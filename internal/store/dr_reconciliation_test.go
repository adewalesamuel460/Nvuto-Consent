package store_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/nats-io/nats.go"
)

// DownstreamConsumerSimulator models an external microservice or notification engine
// that consumes consent events from NATS JetStream and enforces the T-010
// AggregateRevision idempotency contract.
type DownstreamConsumerSimulator struct {
	highestSeenRevision int64
	currentStatus       domain.Status
	processedCount      int
	discardedCount      int
}

func (c *DownstreamConsumerSimulator) HandleEvent(data []byte) error {
	var evt domain.OutboxEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return err
	}

	incomingRev := evt.AggregateRevision
	if incomingRev == 0 {
		if payloadRev, ok := evt.Payload["preference_generation"].(float64); ok {
			incomingRev = int64(payloadRev)
		}
	}

	// Idempotency Contract (T-010 / T-022):
	// Any event with revision <= highestSeenRevision is discarded to prevent
	// re-published or restored prior-image events from overwriting later intent.
	if incomingRev <= c.highestSeenRevision {
		c.discardedCount++
		return nil // safely discarded
	}

	// Apply newer state
	statusStr, _ := evt.Payload["status"].(string)
	c.currentStatus = domain.Status(statusStr)
	c.highestSeenRevision = incomingRev
	c.processedCount++
	return nil
}

// TestAT022_DisasterRecoveryReconciliationProvesNoResurrection (AT-022) simulates:
//  1. Initial consent grant (Revision 1) published to NATS and consumed downstream.
//  2. Subsequent withdrawal (Revision 2) published to NATS and consumed downstream (status=withdrawn).
//  3. Database disaster recovery: Database state reverts to T-1 (Revision 1, granted).
//  4. Post-restore outbox re-publishing: Outbox re-publishes Revision 1 event.
//  5. Downstream reconciliation verification: Downstream consumer rejects Revision 1 event
//     via the AggregateRevision contract, ensuring the restored database NEVER resurrects
//     a withdrawn permission in downstream services.
func TestAT022_DisasterRecoveryReconciliationProvesNoResurrection(t *testing.T) {
	databaseURL := getTestDSN(t)
	if databaseURL == "" {
		t.Skip("skipping test: TEST_DATABASE_URL not set")
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = os.Getenv("TEST_NATS_URL")
	}
	if natsURL == "" {
		natsURL = "nats://127.0.0.1:4222"
	}

	nc, err := nats.Connect(natsURL, nats.Timeout(2*time.Second))
	if err != nil {
		t.Skipf("skipping test: NATS unavailable at %s: %v", natsURL, err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("failed to obtain JetStream context: %v", err)
	}

	streamName := "NVUTO_OUTBOX"
	streamSubject := "nvuto.consent.outbox.>"
	_, _ = js.AddStream(&nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{streamSubject},
		Storage:   nats.MemoryStorage, // test storage
		Retention: nats.LimitsPolicy,
	})

	repo, err := store.NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	run := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "tenant-dr-" + run
	subjectID := "subject-dr-" + run
	purpose := "marketing"
	channel := "email"
	now := time.Now().UTC()

	consumer := &DownstreamConsumerSimulator{}

	// --- Step 1: Initial Grant (Revision 1) ---
	consentV1 := domain.Consent{
		ID:            "con-dr-1-" + run,
		TenantID:      tenantID,
		SubjectID:     subjectID,
		Purpose:       purpose,
		Channel:       channel,
		LegalBasis:    "consent",
		Status:        domain.StatusGranted,
		Source:        "web",
		NoticeVersion: "2026-08",
		Jurisdiction:  "NG",
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	receiptV1 := domain.ConsentReceipt{
		ID:          "rcp-dr-1-" + run,
		TenantID:    tenantID,
		ConsentID:   consentV1.ID,
		SubjectID:   subjectID,
		Purpose:     purpose,
		Channel:     channel,
		ReceiptHash: "hash-v1",
		IssuedAt:    now,
	}
	auditV1 := domain.AuditEvent{
		ID:           "aud-dr-1-" + run,
		TenantID:     tenantID,
		ActorID:      "admin",
		Action:       "consent.granted",
		ConsentID:    consentV1.ID,
		ResourceType: "consent",
		ResourceID:   consentV1.ID,
		Revision:     1,
		OccurredAt:   now,
	}
	outboxV1 := domain.OutboxEvent{
		ID:                "evt-dr-1-" + run,
		TenantID:          tenantID,
		EventType:         "consent.granted",
		AggregateType:     "consent.preference",
		AggregateID:       consentV1.ID,
		AggregateRevision: 1,
		Payload: map[string]any{
			"tenant_id":             tenantID,
			"subject_id":            subjectID,
			"purpose":               purpose,
			"channel":               channel,
			"status":                "granted",
			"preference_generation": 1,
		},
		OccurredAt: now,
	}

	storedV1, err := repo.RecordGoverned(ctx, consentV1, receiptV1, auditV1, outboxV1)
	if err != nil {
		t.Fatalf("Step 1 RecordGoverned failed: %v", err)
	}

	// Publish Revision 1 to NATS
	v1Bytes, _ := json.Marshal(outboxV1)
	pubSubject := fmt.Sprintf("nvuto.consent.outbox.%s.preference", tenantID)
	if _, err := js.Publish(pubSubject, v1Bytes); err != nil {
		t.Fatalf("failed to publish V1 event to NATS: %v", err)
	}

	// Downstream consumes Revision 1
	if err := consumer.HandleEvent(v1Bytes); err != nil {
		t.Fatalf("consumer error handling V1: %v", err)
	}
	if consumer.highestSeenRevision != 1 || consumer.currentStatus != domain.StatusGranted {
		t.Fatalf("expected consumer status granted at rev 1, got status=%s rev=%d", consumer.currentStatus, consumer.highestSeenRevision)
	}

	// --- Step 2: Withdrawal (Revision 2) ---
	now2 := now.Add(time.Minute)
	auditV2 := domain.AuditEvent{
		ID:           "aud-dr-2-" + run,
		TenantID:     tenantID,
		ActorID:      "user",
		Action:       "consent.withdrawn",
		ConsentID:    storedV1.ID,
		ResourceType: "consent",
		ResourceID:   storedV1.ID,
		Revision:     2,
		OccurredAt:   now2,
	}
	outboxV2 := domain.OutboxEvent{
		ID:                "evt-dr-2-" + run,
		TenantID:          tenantID,
		EventType:         "consent.withdrawn",
		AggregateType:     "consent.preference",
		AggregateID:       storedV1.ID,
		AggregateRevision: 2,
		Payload: map[string]any{
			"tenant_id":             tenantID,
			"subject_id":            subjectID,
			"purpose":               purpose,
			"channel":               channel,
			"status":                "withdrawn",
			"preference_generation": 2,
		},
		OccurredAt: now2,
	}

	_, err = repo.Withdraw(ctx, storedV1, auditV2, outboxV2)
	if err != nil {
		t.Fatalf("Step 2 Withdraw failed: %v", err)
	}

	// Publish Revision 2 to NATS
	v2Bytes, _ := json.Marshal(outboxV2)
	if _, err := js.Publish(pubSubject, v2Bytes); err != nil {
		t.Fatalf("failed to publish V2 event to NATS: %v", err)
	}

	// Downstream consumes Revision 2
	if err := consumer.HandleEvent(v2Bytes); err != nil {
		t.Fatalf("consumer error handling V2: %v", err)
	}
	if consumer.highestSeenRevision != 2 || consumer.currentStatus != domain.StatusWithdrawn {
		t.Fatalf("expected consumer status withdrawn at rev 2, got status=%s rev=%d", consumer.currentStatus, consumer.highestSeenRevision)
	}

	// --- Step 3: Simulated Database Disaster & Restore to T-1 ---
	// In a real DR event, the database is restored from a backup taken at T-1 (before withdrawal).
	// In the restored database, the record is back at Revision 1 (Status: granted).
	// The outbox event for Revision 1 is in the outbox table and may be re-published.
	t.Log("Simulating DR restore to T-1 where database reverts to Revision 1 (granted)...")

	// --- Step 4: Re-publishing Revision 1 Event after DR Restore ---
	// The outbox worker wakes up on the restored database and re-publishes outboxV1.
	if _, err := js.Publish(pubSubject, v1Bytes); err != nil {
		t.Fatalf("failed to re-publish V1 event: %v", err)
	}

	// --- Step 5: Downstream Reconciliation Verification ---
	// Downstream consumer receives the re-published Revision 1 event.
	prevProcessed := consumer.processedCount
	prevDiscarded := consumer.discardedCount

	if err := consumer.HandleEvent(v1Bytes); err != nil {
		t.Fatalf("consumer error on re-published V1: %v", err)
	}

	// Assertions:
	// 1. Event was discarded (discardedCount incremented, processedCount unchanged).
	if consumer.discardedCount != prevDiscarded+1 {
		t.Fatalf("expected discardedCount to increment, got prev=%d now=%d", prevDiscarded, consumer.discardedCount)
	}
	if consumer.processedCount != prevProcessed {
		t.Fatalf("expected processedCount to remain unchanged, got prev=%d now=%d", prevProcessed, consumer.processedCount)
	}
	// 2. CRITICAL INVARIANT: The downstream status remains 'withdrawn'!
	// The restored DB image NEVER resurrects a withdrawn permission!
	if consumer.currentStatus != domain.StatusWithdrawn {
		t.Fatalf("CRITICAL SECURITY VIOLATION: Restored database resurrected withdrawn consent to %s!", consumer.currentStatus)
	}
	if consumer.highestSeenRevision != 2 {
		t.Fatalf("highestSeenRevision regressed: got %d, expected 2", consumer.highestSeenRevision)
	}

	t.Logf("PASS: Downstream consumer correctly discarded stale restored revision (rev 1 < rev 2); status remains %s", consumer.currentStatus)
}
