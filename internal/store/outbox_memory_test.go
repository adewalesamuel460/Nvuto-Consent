package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newOutboxMemory(t *testing.T, ids ...string) (*Memory, *testClock) {
	t.Helper()
	clock := &testClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	m := NewMemory()
	m.SetClock(clock.Now)
	for i, id := range ids {
		if err := m.AppendOutbox(context.Background(), domain.OutboxEvent{ID: id, TenantID: "tenant-a", EventType: "consent.granted", AggregateID: fmt.Sprintf("con-%d", i), Payload: map[string]any{"n": i}, OccurredAt: clock.now.Add(time.Duration(i))}); err != nil {
			t.Fatal(err)
		}
	}
	return m, clock
}

func mustClaim(t *testing.T, m *Memory, worker string, limit int) OutboxClaim {
	t.Helper()
	claim, err := m.ClaimOutbox(context.Background(), worker, limit, "")
	if err != nil {
		t.Fatal(err)
	}
	return claim
}

func lease(t *testing.T, m *Memory, id string) OutboxLeaseState {
	t.Helper()
	state, ok := m.OutboxLease(id)
	if !ok {
		t.Fatalf("unknown outbox event %s", id)
	}
	return state
}

func TestOutboxClaimTokensAreUnguessableAndUnique(t *testing.T) {
	m, clock := newOutboxMemory(t, "evt-1")
	pattern := regexp.MustCompile(`^[0-9a-f]{32}$`)
	seen := map[string]bool{}
	for range 50 {
		claim := mustClaim(t, m, "w", 1)
		if !pattern.MatchString(claim.Token) || seen[claim.Token] {
			t.Fatalf("claim token must be fresh 128-bit hex: %q", claim.Token)
		}
		seen[claim.Token] = true
		clock.Advance(OutboxLeaseDuration + time.Second)
	}
}

// AT-009 assertion 1: a stale worker cannot complete, clear or reschedule a
// row owned by a newer claim.
func TestStaleClaimCannotCompleteRetryOrReleaseReclaimedRow(t *testing.T) {
	ctx := context.Background()
	m, clock := newOutboxMemory(t, "evt-1")
	stale := mustClaim(t, m, "worker-a", 10)
	if len(stale.Events) != 1 {
		t.Fatalf("expected one claimed row, got %d", len(stale.Events))
	}

	clock.Advance(OutboxLeaseDuration - time.Nanosecond)
	if early := mustClaim(t, m, "worker-b", 10); len(early.Events) != 0 {
		t.Fatal("a live lease must not be re-claimed")
	}
	clock.Advance(time.Nanosecond)
	if atExpiry := mustClaim(t, m, "worker-b", 10); len(atExpiry.Events) != 0 {
		t.Fatal("a lease is live until strictly after its duration")
	}
	clock.Advance(time.Nanosecond)
	fresh := mustClaim(t, m, "worker-b", 10)
	if len(fresh.Events) != 1 || fresh.Token == stale.Token || fresh.Events[0].Attempts != 1 {
		t.Fatalf("expired lease must be re-claimed under a new token: %+v", fresh)
	}
	owned := lease(t, m, "evt-1")

	for name, write := range map[string]func() error{
		"complete": func() error { return m.CompleteOutbox(ctx, "evt-1", stale.Token) },
		"retry":    func() error { return m.RetryOutbox(ctx, "evt-1", stale.Token, time.Hour) },
		"release":  func() error { return m.ReleaseOutbox(ctx, "evt-1", stale.Token) },
		"empty":    func() error { return m.CompleteOutbox(ctx, "evt-1", "") },
	} {
		if err := write(); !errors.Is(err, ErrLeaseLost) || !errors.Is(err, ErrConflict) {
			t.Fatalf("stale %s must be refused with ErrLeaseLost, got %v", name, err)
		}
		if got := lease(t, m, "evt-1"); got.ClaimToken != owned.ClaimToken || got.LockedBy != "worker-b" || got.PublishedAt != nil ||
			!got.NextAttemptAt.Equal(owned.NextAttemptAt) || got.Attempts != owned.Attempts {
			t.Fatalf("stale %s disturbed the newer claim: before=%+v after=%+v", name, owned, got)
		}
	}

	if err := m.CompleteOutbox(ctx, "evt-1", fresh.Token); err != nil {
		t.Fatal(err)
	}
	done := lease(t, m, "evt-1")
	if done.PublishedAt == nil || done.ClaimToken != "" || done.LockedAt != nil || done.Attempts != 2 {
		t.Fatalf("owner completion must publish and clear the lease: %+v", done)
	}
	if err := m.CompleteOutbox(ctx, "evt-1", fresh.Token); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("completion is single-use, got %v", err)
	}
	if err := m.CompleteOutbox(ctx, "evt-1", stale.Token); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale completion after publication must be refused, got %v", err)
	}
	clock.Advance(24 * time.Hour)
	if again := mustClaim(t, m, "worker-c", 10); len(again.Events) != 0 {
		t.Fatal("published rows are never claimed again")
	}
}

func TestOutboxRetryDefersAndReleaseRestoresImmediately(t *testing.T) {
	ctx := context.Background()
	m, clock := newOutboxMemory(t, "evt-retry", "evt-release")
	claim := mustClaim(t, m, "w", 10)
	if len(claim.Events) != 2 || claim.Events[0].ID != "evt-retry" {
		t.Fatalf("claims must follow occurred_at order: %+v", claim.Events)
	}
	if err := m.RetryOutbox(ctx, "evt-retry", claim.Token, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := m.ReleaseOutbox(ctx, "evt-release", claim.Token); err != nil {
		t.Fatal(err)
	}
	if released := lease(t, m, "evt-release"); released.Attempts != 0 || released.LockedAt != nil || released.ClaimToken != "" {
		t.Fatalf("release must unlock without counting an attempt: %+v", released)
	}
	next := mustClaim(t, m, "w", 10)
	if len(next.Events) != 1 || next.Events[0].ID != "evt-release" {
		t.Fatalf("released row is claimable at once, retried row is deferred: %+v", next.Events)
	}
	clock.Advance(5 * time.Second)
	later := mustClaim(t, m, "w", 10)
	if len(later.Events) != 1 || later.Events[0].ID != "evt-retry" || later.Events[0].Attempts != 1 {
		t.Fatalf("retried row must be due after its delay with one prior attempt: %+v", later.Events)
	}
}

func TestOutboxClaimHonoursLimitAndTenantFilter(t *testing.T) {
	m, _ := newOutboxMemory(t, "evt-1", "evt-2", "evt-3")
	if err := m.AppendOutbox(context.Background(), domain.OutboxEvent{ID: "evt-b", TenantID: "tenant-b", EventType: "consent.granted", OccurredAt: time.Unix(0, 0)}); err != nil {
		t.Fatal(err)
	}
	first, err := m.ClaimOutbox(context.Background(), "w", 2, "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 2 || first.Events[0].ID != "evt-1" || first.Events[1].ID != "evt-2" {
		t.Fatalf("unexpected first batch: %+v", first.Events)
	}
	second, _ := m.ClaimOutbox(context.Background(), "w", 10, "tenant-a")
	if len(second.Events) != 1 || second.Events[0].ID != "evt-3" {
		t.Fatalf("concurrent claims must not overlap: %+v", second.Events)
	}
	if empty, _ := m.ClaimOutbox(context.Background(), "w", 0, ""); empty.Token != "" || len(empty.Events) != 0 {
		t.Fatal("a zero limit claims nothing")
	}
	if cancelled, err := func() (OutboxClaim, error) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return m.ClaimOutbox(ctx, "w", 10, "")
	}(); err == nil || len(cancelled.Events) != 0 {
		t.Fatal("a cancelled claim must not lock rows")
	}
	if state := lease(t, m, "evt-b"); state.LockedAt != nil {
		t.Fatal("cancelled claim locked a row")
	}
}

func TestConcurrentOutboxClaimsNeverShareRows(t *testing.T) {
	ids := make([]string, 40)
	for i := range ids {
		ids[i] = fmt.Sprintf("evt-%02d", i)
	}
	m, _ := newOutboxMemory(t, ids...)
	var mu sync.Mutex
	owner := map[string]string{}
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				claim, err := m.ClaimOutbox(context.Background(), fmt.Sprintf("w%d", w), 3, "")
				if err != nil || len(claim.Events) == 0 {
					return
				}
				mu.Lock()
				for _, item := range claim.Events {
					if previous, taken := owner[item.ID]; taken {
						t.Errorf("%s claimed by %s and %s", item.ID, previous, claim.Token)
					}
					owner[item.ID] = claim.Token
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(owner) != len(ids) {
		t.Fatalf("claimed %d of %d rows", len(owner), len(ids))
	}
}
