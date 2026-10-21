package store

import (
	"context"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
)

// TestAT021_SimulatedBrokerFailureQuarantinesPoisonEventAfterMaxAttempts proves that:
// 1. A simulated broker failure increments the attempt counter until it hits the maximum (e.g. 5 attempts).
// 2. Once max attempts are exceeded, the event is marked quarantined (DLQ) with timestamp and reason.
// 3. Subsequent ClaimOutbox calls skip the quarantined poison event so the queue is never blocked.
// 4. Other valid pending events in the queue continue to be claimed and processed normally.
func TestAT021_SimulatedBrokerFailureQuarantinesPoisonEventAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	const maxAttempts = 5
	const workerID = "test-worker-obs"

	m, clock := newOutboxMemory(t, "evt-poison", "evt-valid")

	// Verify both events are initially present
	initialClaim, err := m.ClaimOutbox(ctx, workerID, 10, "")
	if err != nil {
		t.Fatalf("initial claim failed: %v", err)
	}
	if len(initialClaim.Events) != 2 {
		t.Fatalf("expected 2 events in initial claim, got %d", len(initialClaim.Events))
	}
	// Release both so we can simulate targeted failure on evt-poison
	if err := m.ReleaseOutboxBatch(ctx, workerID, initialClaim.Events); err != nil {
		t.Fatalf("failed to release initial claim: %v", err)
	}

	// Simulated broker failure loop on evt-poison
	const brokerError = "nats: connection lost: simulated broker failure"
	var poisonToken string

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Claim 1 event: should be evt-poison (earliest OccurredAt)
		claim, err := m.ClaimOutbox(ctx, workerID, 1, "")
		if err != nil {
			t.Fatalf("attempt %d claim failed: %v", attempt+1, err)
		}
		if len(claim.Events) != 1 {
			t.Fatalf("attempt %d expected 1 event, got %d", attempt+1, len(claim.Events))
		}
		item := claim.Events[0]
		if item.ID != "evt-poison" {
			t.Fatalf("attempt %d expected evt-poison, got %s", attempt+1, item.ID)
		}
		if item.Attempts != attempt {
			t.Fatalf("attempt %d expected item.Attempts=%d, got %d", attempt+1, attempt, item.Attempts)
		}

		poisonToken = claim.Token

		// Check if this attempt reaches maxAttempts
		if item.Attempts+1 >= maxAttempts {
			// Hit retry threshold -> quarantine the poison event
			if err := m.QuarantineOutbox(ctx, item.ID, claim.Token, brokerError); err != nil {
				t.Fatalf("attempt %d QuarantineOutbox failed: %v", attempt+1, err)
			}
		} else {
			// Under retry threshold -> retry with exponential backoff
			delay := time.Duration(1<<attempt) * time.Second
			if err := m.RetryOutbox(ctx, item.ID, claim.Token, delay); err != nil {
				t.Fatalf("attempt %d RetryOutbox failed: %v", attempt+1, err)
			}
			// Advance clock past the retry delay so it is eligible for next claim
			clock.Advance(delay + time.Second)
		}
	}

	// Verify quarantine state on the poisoned event
	state := lease(t, m, "evt-poison")
	if !state.Quarantined {
		t.Fatal("expected evt-poison to be marked quarantined=true")
	}
	if state.QuarantinedAt == nil {
		t.Fatal("expected quarantined_at timestamp to be set")
	}
	if state.QuarantineReason != brokerError {
		t.Fatalf("expected quarantine reason %q, got %q", brokerError, state.QuarantineReason)
	}
	if state.PublishedAt != nil {
		t.Fatal("quarantined event must not be marked published")
	}
	if state.Attempts != maxAttempts {
		t.Fatalf("expected exactly %d attempts before quarantine, got %d", maxAttempts, state.Attempts)
	}

	// Verify that subsequent polling skips the quarantined poison event completely
	// and claims the next valid event ("evt-valid") without queue stalling
	nextClaim, err := m.ClaimOutbox(ctx, workerID, 10, "")
	if err != nil {
		t.Fatalf("subsequent claim failed: %v", err)
	}
	if len(nextClaim.Events) != 1 {
		t.Fatalf("expected exactly 1 actionable event (skipping quarantined), got %d", len(nextClaim.Events))
	}
	if nextClaim.Events[0].ID != "evt-valid" {
		t.Fatalf("expected evt-valid to be claimed, got %s", nextClaim.Events[0].ID)
	}

	// Complete evt-valid
	if err := m.CompleteOutbox(ctx, "evt-valid", nextClaim.Token); err != nil {
		t.Fatalf("failed to complete evt-valid: %v", err)
	}

	// Advance clock and verify queue is now completely empty of actionable items
	clock.Advance(time.Hour)
	emptyClaim, err := m.ClaimOutbox(ctx, workerID, 10, "")
	if err != nil {
		t.Fatalf("empty check claim failed: %v", err)
	}
	if len(emptyClaim.Events) != 0 {
		t.Fatalf("expected 0 actionable events, got %d", len(emptyClaim.Events))
	}

	// Verify QuarantinedOutboxCount reports exactly 1 quarantined event
	qCount, err := m.QuarantinedOutboxCount(ctx, "")
	if err != nil {
		t.Fatalf("QuarantinedOutboxCount failed: %v", err)
	}
	if qCount != 1 {
		t.Fatalf("expected 1 quarantined event, got %d", qCount)
	}

	_ = poisonToken
}

// TestAT021_ManualReplayRestoresQuarantinedEvent proves that:
// 1. A manual operator replay method (ReplayOutbox) safely restores a quarantined event.
// 2. The event's quarantine flag is cleared, quarantined_at is reset, reason is cleared, and attempts reset to 0.
// 3. The replayed event is immediately actionable and claimed by subsequent polling.
// 4. Once the broker issue is resolved, publication completes successfully.
func TestAT021_ManualReplayRestoresQuarantinedEvent(t *testing.T) {
	ctx := context.Background()
	const workerID = "test-worker-replay"

	m, clock := newOutboxMemory(t, "evt-poison-replay")

	// 1. Claim and quarantine the event
	claim, err := m.ClaimOutbox(ctx, workerID, 1, "")
	if err != nil || len(claim.Events) != 1 {
		t.Fatalf("failed to claim: %v", err)
	}
	if err := m.QuarantineOutbox(ctx, "evt-poison-replay", claim.Token, "broker timeout"); err != nil {
		t.Fatalf("failed to quarantine: %v", err)
	}

	// 2. Verify it cannot be claimed while quarantined
	clock.Advance(10 * time.Minute)
	blockedClaim, err := m.ClaimOutbox(ctx, workerID, 1, "")
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if len(blockedClaim.Events) != 0 {
		t.Fatalf("expected 0 claimed events while quarantined, got %d", len(blockedClaim.Events))
	}

	// 3. Operator executes manual replay
	if err := m.ReplayOutbox(ctx, "evt-poison-replay"); err != nil {
		t.Fatalf("ReplayOutbox failed: %v", err)
	}

	// 4. Verify the quarantine state was safely cleared and attempts reset
	state := lease(t, m, "evt-poison-replay")
	if state.Quarantined {
		t.Fatal("expected Quarantined=false after replay")
	}
	if state.QuarantinedAt != nil {
		t.Fatal("expected QuarantinedAt=nil after replay")
	}
	if state.QuarantineReason != "" {
		t.Fatalf("expected empty QuarantineReason after replay, got %q", state.QuarantineReason)
	}
	if state.Attempts != 0 {
		t.Fatalf("expected Attempts reset to 0 after replay, got %d", state.Attempts)
	}

	// 5. Subsequent ClaimOutbox successfully claims the restored event
	restoredClaim, err := m.ClaimOutbox(ctx, workerID, 1, "")
	if err != nil {
		t.Fatalf("ClaimOutbox after replay failed: %v", err)
	}
	if len(restoredClaim.Events) != 1 || restoredClaim.Events[0].ID != "evt-poison-replay" {
		t.Fatalf("expected to claim restored event evt-poison-replay, got %+v", restoredClaim.Events)
	}

	// 6. Complete publication
	if err := m.CompleteOutbox(ctx, "evt-poison-replay", restoredClaim.Token); err != nil {
		t.Fatalf("failed to complete restored event: %v", err)
	}

	// Verify published
	finalState := lease(t, m, "evt-poison-replay")
	if finalState.PublishedAt == nil {
		t.Fatal("expected PublishedAt to be set after completion")
	}
}

// TestAT021_OutboxLagMeasurement verifies calculation of propagation lag
// and confirms lag shifts properly when oldest events are quarantined or published.
func TestAT021_OutboxLagMeasurement(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)}
	m := NewMemory()
	m.SetClock(clock.Now)

	// Add 3 events at t0, t0 + 10s, t0 + 20s
	t0 := clock.now
	events := []domain.OutboxEvent{
		{ID: "evt-lag-1", TenantID: "tenant-a", EventType: "consent.granted", AggregateID: "con-1", OccurredAt: t0},
		{ID: "evt-lag-2", TenantID: "tenant-a", EventType: "consent.granted", AggregateID: "con-2", OccurredAt: t0.Add(10 * time.Second)},
		{ID: "evt-lag-3", TenantID: "tenant-a", EventType: "consent.granted", AggregateID: "con-3", OccurredAt: t0.Add(20 * time.Second)},
	}
	for _, e := range events {
		if err := m.AppendOutbox(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	// Advance clock to t0 + 35s
	clock.Advance(35 * time.Second)

	// Oldest active is evt-lag-1 (occurred at t0). Lag should be 35s.
	lag, err := m.OutboxLag(ctx, "")
	if err != nil {
		t.Fatalf("OutboxLag failed: %v", err)
	}
	if lag != 35*time.Second {
		t.Fatalf("expected lag of 35s, got %v", lag)
	}

	// Quarantine evt-lag-1
	claim1, err := m.ClaimOutbox(ctx, "w1", 1, "")
	if err != nil || len(claim1.Events) != 1 {
		t.Fatalf("claim failed: %v", err)
	}
	if err := m.QuarantineOutbox(ctx, "evt-lag-1", claim1.Token, "poison payload"); err != nil {
		t.Fatalf("quarantine failed: %v", err)
	}

	// Oldest non-quarantined is now evt-lag-2 (occurred at t0 + 10s).
	// Current time is t0 + 35s. Lag should be 25s.
	lagAfterQuarantine, err := m.OutboxLag(ctx, "")
	if err != nil {
		t.Fatalf("OutboxLag failed: %v", err)
	}
	if lagAfterQuarantine != 25*time.Second {
		t.Fatalf("expected lag of 25s after quarantining oldest, got %v", lagAfterQuarantine)
	}

	// Complete evt-lag-2
	claim2, err := m.ClaimOutbox(ctx, "w1", 1, "")
	if err != nil || len(claim2.Events) != 1 {
		t.Fatalf("claim failed: %v", err)
	}
	if err := m.CompleteOutbox(ctx, "evt-lag-2", claim2.Token); err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	// Oldest remaining active is evt-lag-3 (occurred at t0 + 20s).
	// Current time is t0 + 35s. Lag should be 15s.
	lagRemaining, err := m.OutboxLag(ctx, "")
	if err != nil {
		t.Fatalf("OutboxLag failed: %v", err)
	}
	if lagRemaining != 15*time.Second {
		t.Fatalf("expected lag of 15s, got %v", lagRemaining)
	}

	// Complete evt-lag-3
	claim3, err := m.ClaimOutbox(ctx, "w1", 1, "")
	if err != nil || len(claim3.Events) != 1 {
		t.Fatalf("claim failed: %v", err)
	}
	if err := m.CompleteOutbox(ctx, "evt-lag-3", claim3.Token); err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	// All active non-quarantined events completed. Lag should be 0.
	lagZero, err := m.OutboxLag(ctx, "")
	if err != nil {
		t.Fatalf("OutboxLag failed: %v", err)
	}
	if lagZero != 0 {
		t.Fatalf("expected lag of 0s when no active pending events exist, got %v", lagZero)
	}
}

// TestAT021_QuarantineFencingEnforcement verifies fencing invariants on quarantine:
// an expired or invalid claim token cannot quarantine a row owned by a newer lease.
func TestAT021_QuarantineFencingEnforcement(t *testing.T) {
	ctx := context.Background()
	m, clock := newOutboxMemory(t, "evt-fence")

	staleClaim, err := m.ClaimOutbox(ctx, "worker-1", 1, "")
	if err != nil || len(staleClaim.Events) != 1 {
		t.Fatalf("claim failed: %v", err)
	}

	// Advance clock past lease expiry
	clock.Advance(OutboxLeaseDuration + time.Second)

	// Newer worker claims the row
	newerClaim, err := m.ClaimOutbox(ctx, "worker-2", 1, "")
	if err != nil || len(newerClaim.Events) != 1 {
		t.Fatalf("newer claim failed: %v", err)
	}

	// Stale worker attempts to quarantine: must be rejected with ErrLeaseLost
	err = m.QuarantineOutbox(ctx, "evt-fence", staleClaim.Token, "late poison detection")
	if err == nil || !(err == ErrLeaseLost || err == ErrConflict) {
		t.Fatalf("expected ErrLeaseLost for stale quarantine, got %v", err)
	}

	// Newer worker successfully quarantines
	if err := m.QuarantineOutbox(ctx, "evt-fence", newerClaim.Token, "confirmed poison"); err != nil {
		t.Fatalf("newer claim quarantine failed: %v", err)
	}

	// Replaying non-existent event returns ErrNotFound
	if err := m.ReplayOutbox(ctx, "non-existent-id"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound replaying non-existent ID, got %v", err)
	}
}
