package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type logSink struct {
	mu    sync.Mutex
	lines []string
}

func (l *logSink) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *logSink) contains(fragment string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.lines {
		if strings.Contains(line, fragment) {
			return true
		}
	}
	return false
}

func (l *logSink) dump(t *testing.T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.lines {
		t.Log(line)
	}
}

// recorder is a publish function that records every Msg-Id it publishes.
type recorder struct {
	mu        sync.Mutex
	published []string
}

func (r *recorder) publish(_ context.Context, item event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.published = append(r.published, item.ID)
	return nil
}

func (r *recorder) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.published...)
}

type countingLeaser struct {
	store.OutboxLeaser
	claims atomic.Int32
}

func (c *countingLeaser) ClaimOutbox(ctx context.Context, workerID string, limit int, tenantFilter string) (store.OutboxClaim, error) {
	c.claims.Add(1)
	return c.OutboxLeaser.ClaimOutbox(ctx, workerID, limit, tenantFilter)
}

func newOutbox(t *testing.T, count int) (*store.Memory, *fakeClock) {
	t.Helper()
	clock := &fakeClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	repo := store.NewMemory()
	repo.SetClock(clock.Now)
	for i := range count {
		appendEvent(t, repo, fmt.Sprintf("evt-%02d", i), clock.Now().Add(time.Duration(i)))
	}
	return repo, clock
}

func appendEvent(t *testing.T, repo *store.Memory, id string, at time.Time) {
	t.Helper()
	if err := repo.AppendOutbox(context.Background(), domain.OutboxEvent{ID: id, TenantID: "tenant-a", EventType: "consent.granted", AggregateType: "consent", AggregateID: "con-" + id, Payload: map[string]any{"subject_id": "cus-synthetic"}, OccurredAt: at}); err != nil {
		t.Fatal(err)
	}
}

func testWorker(leaser store.OutboxLeaser, publish publishFunc, id string, logs *logSink) *worker {
	w := newWorker(leaser, publish, id)
	w.logf = logs.logf
	w.pollInterval = 5 * time.Millisecond
	return w
}

func leaseOf(t *testing.T, repo *store.Memory, id string) store.OutboxLeaseState {
	t.Helper()
	state, ok := repo.OutboxLease(id)
	if !ok {
		t.Fatalf("unknown event %s", id)
	}
	return state
}

func background() (context.Context, context.Context) {
	return context.Background(), context.Background()
}

func TestWorkerPublishesAndCompletesBatch(t *testing.T) {
	repo, _ := newOutbox(t, 3)
	logs := &logSink{}
	rec := &recorder{}
	w := testWorker(repo, rec.publish, "worker-a", logs)
	soft, hard := background()
	if full := w.cycle(soft, hard); full {
		t.Fatal("a partial batch is not full")
	}
	if got := rec.ids(); len(got) != 3 || got[0] != "evt-00" || got[2] != "evt-02" {
		t.Fatalf("unexpected publications: %v", got)
	}
	for _, id := range rec.ids() {
		if state := leaseOf(t, repo, id); state.PublishedAt == nil || state.ClaimToken != "" || state.LockedAt != nil {
			t.Fatalf("%s must be completed and unlocked: %+v", id, state)
		}
	}
}

// Slow publish: worker A's lease expires mid-publish and worker B re-claims,
// publishes and completes. A's late completion must be refused and must not
// alter B's result.
func TestSlowPublisherCannotCompleteRowReclaimedByNewerWorker(t *testing.T) {
	repo, clock := newOutbox(t, 1)
	logsA, logsB := &logSink{}, &logSink{}
	started, unblock := make(chan struct{}), make(chan struct{})
	slowPublish := func(ctx context.Context, item event) error {
		close(started)
		<-unblock
		return nil
	}
	workerA := testWorker(repo, slowPublish, "worker-a", logsA)
	rec := &recorder{}
	workerB := testWorker(repo, rec.publish, "worker-b", logsB)

	done := make(chan struct{})
	go func() {
		defer close(done)
		soft, hard := background()
		workerA.cycle(soft, hard)
	}()
	<-started
	tokenA := leaseOf(t, repo, "evt-00").ClaimToken

	clock.Advance(store.OutboxLeaseDuration + time.Second)
	soft, hard := background()
	workerB.cycle(soft, hard)
	completedByB := leaseOf(t, repo, "evt-00")
	if completedByB.PublishedAt == nil || len(rec.ids()) != 1 {
		t.Fatalf("worker B must re-claim and complete the expired lease: %+v", completedByB)
	}

	clock.Advance(time.Minute)
	close(unblock)
	<-done
	after := leaseOf(t, repo, "evt-00")
	if !after.PublishedAt.Equal(*completedByB.PublishedAt) || after.Attempts != 2 || after.ClaimToken != "" {
		t.Fatalf("stale completion altered the row: before=%+v after=%+v", completedByB, after)
	}
	if tokenA == "" || !logsA.contains("complete evt-00 refused") {
		logsA.dump(t)
		t.Fatal("worker A must log the refused stale completion")
	}
}

// A stale worker whose publish fails must not reschedule or release a row that
// a newer claim currently owns.
func TestStaleWorkerCannotRescheduleOrReleaseNewerClaim(t *testing.T) {
	repo, clock := newOutbox(t, 2)
	logs := &logSink{}
	started, unblock := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	failingSlowPublish := func(ctx context.Context, item event) error {
		if calls.Add(1) == 1 {
			close(started)
			<-unblock
			return errors.New("broker unavailable")
		}
		return nil
	}
	stale := testWorker(repo, failingSlowPublish, "worker-a", logs)
	done := make(chan struct{})
	go func() {
		defer close(done)
		soft, hard := background()
		stale.cycle(soft, hard)
	}()
	<-started

	clock.Advance(store.OutboxLeaseDuration + time.Second)
	newer, err := repo.ClaimOutbox(context.Background(), "worker-b", 10, "")
	if err != nil || len(newer.Events) != 2 {
		t.Fatalf("worker B must re-claim both rows: %+v %v", newer, err)
	}
	owned := map[string]store.OutboxLeaseState{"evt-00": leaseOf(t, repo, "evt-00"), "evt-01": leaseOf(t, repo, "evt-01")}

	close(unblock)
	<-done
	for id, before := range owned {
		after := leaseOf(t, repo, id)
		if after.ClaimToken != newer.Token || after.LockedBy != "worker-b" || after.PublishedAt != nil ||
			!after.NextAttemptAt.Equal(before.NextAttemptAt) || after.Attempts != before.Attempts {
			t.Fatalf("stale worker disturbed %s: before=%+v after=%+v", id, before, after)
		}
	}
	if !logs.contains("retry scheduling evt-00 refused") || !logs.contains("complete evt-01 refused") {
		logs.dump(t)
		t.Fatal("stale retry and completion must be refused")
	}
	for _, item := range newer.Events {
		if err := repo.CompleteOutbox(context.Background(), item.ID, newer.Token); err != nil {
			t.Fatalf("the newer claim keeps full ownership: %v", err)
		}
	}
}

// Killed worker: rows claimed by a worker that dies (before publishing, or
// after the broker ack but before the SQL completion) are re-claimed after
// the lease and published under the same Msg-Id, so delivery is eventual and
// JetStream de-duplicates the ack/commit-gap replay.
func TestKilledWorkerRowsAreRedeliveredAfterLeaseExpiry(t *testing.T) {
	repo, clock := newOutbox(t, 2)
	logs := &logSink{}
	rec := &recorder{}

	killed, err := repo.ClaimOutbox(context.Background(), "worker-killed", 10, "")
	if err != nil || len(killed.Events) != 2 {
		t.Fatalf("setup claim failed: %+v %v", killed, err)
	}
	// evt-00 was acknowledged by the broker before the crash; evt-01 was not.
	if err := rec.publish(context.Background(), killed.Events[0]); err != nil {
		t.Fatal(err)
	}

	survivor := testWorker(repo, rec.publish, "worker-survivor", logs)
	soft, hard := background()
	survivor.cycle(soft, hard)
	if len(rec.ids()) != 1 {
		t.Fatal("rows under a live lease must not be taken over")
	}

	clock.Advance(store.OutboxLeaseDuration + time.Second)
	survivor.cycle(soft, hard)
	got := rec.ids()
	if len(got) != 3 || got[0] != "evt-00" || got[1] != "evt-00" || got[2] != "evt-01" {
		t.Fatalf("expected the acked event to be replayed under the same Msg-Id and the other delivered: %v", got)
	}
	for _, id := range []string{"evt-00", "evt-01"} {
		if state := leaseOf(t, repo, id); state.PublishedAt == nil || state.Attempts != 2 {
			t.Fatalf("%s must be completed by the survivor: %+v", id, state)
		}
	}
	if err := repo.CompleteOutbox(context.Background(), "evt-00", killed.Token); !errors.Is(err, store.ErrLeaseLost) {
		t.Fatalf("a revived killed worker must stay fenced, got %v", err)
	}
}

func TestDuplicateWindowCoversLeaseRedelivery(t *testing.T) {
	// Redelivery after a lost completion happens at most one lease plus one
	// publish and poll later; it must land inside the JetStream window.
	if worst := store.OutboxLeaseDuration + defaultPublishTimeout + defaultPollInterval; worst >= jetStreamDuplicateWindow {
		t.Fatalf("duplicate window %s does not cover redelivery after %s", jetStreamDuplicateWindow, worst)
	}
	if leaseSafetyMargin <= defaultPublishTimeout || leaseSafetyMargin >= store.OutboxLeaseDuration {
		t.Fatal("lease margin must exceed one publish timeout and leave a positive batch budget")
	}
}

func TestPublishFailureSchedulesFencedRetry(t *testing.T) {
	repo, clock := newOutbox(t, 1)
	logs := &logSink{}
	w := testWorker(repo, func(context.Context, event) error { return errors.New("nats: no responders") }, "w", logs)
	soft, hard := background()
	w.cycle(soft, hard)
	state := leaseOf(t, repo, "evt-00")
	if state.PublishedAt != nil || state.LockedAt != nil || state.ClaimToken != "" || state.Attempts != 1 ||
		!state.NextAttemptAt.Equal(clock.Now().Add(retryDelay(0))) {
		t.Fatalf("failed publish must unlock and back off: %+v", state)
	}
}

func TestLeaseBudgetExhaustionReleasesRemainingRows(t *testing.T) {
	repo, _ := newOutbox(t, 5)
	logs := &logSink{}
	slow := func(ctx context.Context, item event) error {
		select {
		case <-time.After(30 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	w := testWorker(repo, slow, "w", logs)
	w.leaseMargin = store.OutboxLeaseDuration - 50*time.Millisecond // 50ms batch budget
	soft, hard := background()
	w.cycle(soft, hard)
	published, released := 0, 0
	for i := range 5 {
		state := leaseOf(t, repo, fmt.Sprintf("evt-%02d", i))
		switch {
		case state.PublishedAt != nil:
			published++
		case state.LockedAt == nil && state.ClaimToken == "" && state.Attempts == 0:
			released++
		default:
			t.Fatalf("evt-%02d left in an unsettled state: %+v", i, state)
		}
	}
	if released == 0 || published+released != 5 || !logs.contains("lease budget exhausted") {
		logs.dump(t)
		t.Fatalf("budget exhaustion must release the remainder: published=%d released=%d", published, released)
	}
}

// AT-009 assertion 3: after SIGTERM the worker claims nothing new and the batch
// in flight drains when it fits the grace.
func TestShutdownStopsClaimingAndDrainsInFlightBatch(t *testing.T) {
	repo, clock := newOutbox(t, 5)
	logs := &logSink{}
	leaser := &countingLeaser{OutboxLeaser: repo}
	first := make(chan struct{})
	var once sync.Once
	rec := &recorder{}
	publish := func(ctx context.Context, item event) error {
		once.Do(func() { close(first) })
		time.Sleep(10 * time.Millisecond)
		return rec.publish(ctx, item)
	}
	w := testWorker(leaser, publish, "w", logs)
	w.batchSize = 5
	w.shutdownGrace = 2 * time.Second

	stop, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.run(stop) }()
	<-first
	cancel()
	appendEvent(t, repo, "evt-late", clock.Now().Add(time.Hour))
	select {
	case <-done:
	case <-time.After(w.shutdownGrace):
		t.Fatal("worker did not stop within the shutdown grace")
	}
	if got := len(rec.ids()); got != 5 {
		t.Fatalf("in-flight batch must drain completely, published %d", got)
	}
	if claims := leaser.claims.Load(); claims != 1 {
		t.Fatalf("no batch may be claimed after stop, claimed %d times", claims)
	}
	if state := leaseOf(t, repo, "evt-late"); state.LockedAt != nil || state.Attempts != 0 {
		t.Fatalf("events arriving after stop must not be claimed: %+v", state)
	}
}

// AT-009 assertion 3: a batch that cannot drain within the grace has its
// unpublished rows released before the grace ends, so another worker can
// claim them immediately rather than after lease expiry.
func TestShutdownReleasesUnpublishedRowsWithinGrace(t *testing.T) {
	repo, _ := newOutbox(t, 5)
	logs := &logSink{}
	first := make(chan struct{})
	var once sync.Once
	hung := func(ctx context.Context, item event) error {
		once.Do(func() { close(first) })
		<-ctx.Done()
		return ctx.Err()
	}
	w := testWorker(repo, hung, "w", logs)
	w.shutdownGrace = 300 * time.Millisecond

	stop, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.run(stop) }()
	<-first
	signalled := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker hung past the shutdown grace")
	}
	if elapsed := time.Since(signalled); elapsed > w.shutdownGrace+150*time.Millisecond {
		t.Fatalf("shutdown took %s, grace is %s", elapsed, w.shutdownGrace)
	}
	for i := range 5 {
		if state := leaseOf(t, repo, fmt.Sprintf("evt-%02d", i)); state.LockedAt != nil || state.ClaimToken != "" || state.PublishedAt != nil || state.Attempts != 0 {
			logs.dump(t)
			t.Fatalf("evt-%02d must be released unpublished: %+v", i, state)
		}
	}
	next, err := repo.ClaimOutbox(context.Background(), "replacement", 10, "")
	if err != nil || len(next.Events) != 5 {
		t.Fatalf("released rows must be claimable at once: %d %v", len(next.Events), err)
	}
}

func TestShutdownWhileIdleReturnsPromptly(t *testing.T) {
	repo, _ := newOutbox(t, 0)
	w := testWorker(repo, (&recorder{}).publish, "w", &logSink{})
	w.pollInterval = time.Hour
	stop, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.run(stop) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("idle worker must stop without waiting for the next poll")
	}
}

func TestRetryDelayIsBoundedExponential(t *testing.T) {
	for attempts, want := range map[int]time.Duration{0: 2 * time.Second, 1: 4 * time.Second, 8: 512 * time.Second, 9: 900 * time.Second, 50: 900 * time.Second} {
		if got := retryDelay(attempts); got != want {
			t.Errorf("retryDelay(%d)=%s want %s", attempts, got, want)
		}
	}
}

func TestDurationEnvValidation(t *testing.T) {
	t.Setenv("OUTBOX_TEST_DURATION", "")
	if got, err := durationEnv("OUTBOX_TEST_DURATION", 25*time.Second, time.Second, time.Minute); err != nil || got != 25*time.Second {
		t.Fatalf("default not applied: %s %v", got, err)
	}
	for _, bad := range []string{"0s", "2m", "soon"} {
		t.Setenv("OUTBOX_TEST_DURATION", bad)
		if _, err := durationEnv("OUTBOX_TEST_DURATION", time.Second, time.Second, time.Minute); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}
