package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestIntegration_PostgresTransactionAtomicRollbackOnFault proves all-or-nothing
// transaction atomicity (T-016 / AT-016). When a fault occurs before the outbox event
// is appended, the entire transaction rolls back cleanly, leaving zero orphaned records
// in consents, consent_receipts, consent_audit_events, or consent_preference_state.
func TestIntegration_PostgresTransactionAtomicRollbackOnFault(t *testing.T) {
	databaseURL := integrationDSN(t)
	repo, err := NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	ctx := context.Background()
	run := runID(t)
	tenant := "fault-" + run
	subject := "cus-" + run
	purpose := "marketing"
	channel := "email"
	now := time.Now().UTC()

	// 1. Constraint Violation Fault:
	// Pre-insert an outbox event with a specific ID to force a primary key collision
	// in appendOutboxTx, which executes AFTER insertConsent and insertReceipt.
	poisonOutboxID := "evt-poison-" + run
	preInsertOutbox := domain.OutboxEvent{
		ID:          poisonOutboxID,
		TenantID:    tenant,
		EventType:   "consent.granted",
		AggregateID: "con-pre-" + run,
		Payload:     map[string]any{"pre": true},
		OccurredAt:  now,
	}
	if err := repo.AppendOutbox(ctx, preInsertOutbox); err != nil {
		t.Fatalf("failed to seed collision outbox event: %v", err)
	}

	consentID := "con-failed-" + run
	receiptID := "rcp-failed-" + run
	auditID := "aud-failed-" + run

	failingConsent := domain.Consent{
		ID:            consentID,
		TenantID:      tenant,
		SubjectID:     subject,
		Purpose:       purpose,
		Channel:       channel,
		LegalBasis:    "consent",
		Status:        domain.StatusGranted,
		Source:        "integration",
		Jurisdiction:  "NG",
		NoticeVersion: "2026-08",
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	failingReceipt := domain.ConsentReceipt{
		ID:            receiptID,
		TenantID:      tenant,
		ConsentID:     consentID,
		SubjectID:     subject,
		Purpose:       purpose,
		Channel:       channel,
		NoticeVersion: "2026-08",
		Jurisdiction:  "NG",
		ReceiptHash:   "hash-fault-" + run,
		IssuedAt:      now,
	}
	failingConsent.ReceiptID = receiptID

	failingAudit := domain.AuditEvent{
		ID:         auditID,
		TenantID:   tenant,
		ActorID:    "tester",
		Action:     "consent.granted",
		ConsentID:  consentID,
		OccurredAt: now,
	}

	// This outbox event has the colliding ID
	collidingOutbox := domain.OutboxEvent{
		ID:          poisonOutboxID, // COLLISION!
		TenantID:    tenant,
		EventType:   "consent.granted",
		AggregateID: consentID,
		Payload:     map[string]any{"subject_id": subject},
		OccurredAt:  now,
	}

	// RecordGoverned must fail because appendOutboxTx violates the primary key constraint
	_, recordErr := repo.RecordGoverned(ctx, failingConsent, failingReceipt, failingAudit, collidingOutbox)
	if recordErr == nil {
		t.Fatal("expected RecordGoverned to fail on outbox primary key collision, got nil")
	}

	var pgErr *pgconn.PgError
	if !errors.As(recordErr, &pgErr) && !errors.Is(recordErr, domain.ErrConflict) {
		t.Logf("RecordGoverned returned error as expected: %v", recordErr)
	}

	// Verify All-or-Nothing Atomicity:
	// None of the prior writes (consent, receipt, audit) must exist in the database!
	var consentCount, receiptCount, auditCount, prefCount int
	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM consents WHERE tenant_id=$1 AND id=$2", tenant, consentID).Scan(&consentCount); err != nil {
		t.Fatal(err)
	}
	if consentCount != 0 {
		t.Fatalf("atomic rollback violated: found %d rows in consents for %s", consentCount, consentID)
	}

	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM consent_receipts WHERE tenant_id=$1 AND id=$2", tenant, receiptID).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 0 {
		t.Fatalf("atomic rollback violated: found %d rows in consent_receipts for %s", receiptCount, receiptID)
	}

	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM consent_audit_events WHERE tenant_id=$1 AND id=$2", tenant, auditID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 0 {
		t.Fatalf("atomic rollback violated: found %d rows in consent_audit_events for %s", auditCount, auditID)
	}

	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM consent_preference_state WHERE tenant_id=$1 AND subject_id=$2 AND purpose=$3 AND channel=$4", tenant, subject, purpose, channel).Scan(&prefCount); err != nil {
		t.Fatal(err)
	}
	if prefCount != 0 {
		t.Fatalf("atomic rollback violated: found %d rows in consent_preference_state for stream", prefCount)
	}

	// 2. Context Cancellation Fault:
	// Verify that an explicitly cancelled context causes full rollback without orphaned rows.
	ctxCancelled, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	consentCancelID := "con-cancelled-" + run
	receiptCancelID := "rcp-cancelled-" + run
	auditCancelID := "aud-cancelled-" + run
	outboxCancelID := "evt-cancelled-" + run

	failingConsent.ID = consentCancelID
	failingReceipt.ID = receiptCancelID
	failingReceipt.ConsentID = consentCancelID
	failingConsent.ReceiptID = receiptCancelID
	failingAudit.ID = auditCancelID
	failingAudit.ConsentID = consentCancelID
	collidingOutbox.ID = outboxCancelID
	collidingOutbox.AggregateID = consentCancelID

	_, cancelErr := repo.RecordGoverned(ctxCancelled, failingConsent, failingReceipt, failingAudit, collidingOutbox)
	if cancelErr == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}

	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM consents WHERE tenant_id=$1 AND id=$2", tenant, consentCancelID).Scan(&consentCount); err != nil {
		t.Fatal(err)
	}
	if consentCount != 0 {
		t.Fatalf("cancellation rollback violated: found %d rows in consents", consentCount)
	}
	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM consent_receipts WHERE tenant_id=$1 AND id=$2", tenant, receiptCancelID).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 0 {
		t.Fatalf("cancellation rollback violated: found %d rows in consent_receipts", receiptCount)
	}
}

// TestIntegration_PostgresHighConcurrencyContention simulates 50 concurrent writers
// hitting the exact same subject/purpose/channel stream simultaneously (T-016 / AT-016).
// It proves YSQL handles row-level contention safely without deadlocks, corruption, or lost updates.
func TestIntegration_PostgresHighConcurrencyContention(t *testing.T) {
	databaseURL := integrationDSN(t)
	repo, err := NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	ctx := context.Background()
	run := runID(t)
	tenant := "concur-" + run
	subject := "cus-contention-" + run
	purpose := "marketing"
	channel := "email"

	const concurrency = 50
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			now := time.Now().UTC()
			id := fmt.Sprintf("%s-%02d", run, idx)

			status := domain.StatusGranted
			if idx%2 == 1 {
				status = domain.StatusDenied
			}

			consent := domain.Consent{
				ID:            "con-" + id,
				TenantID:      tenant,
				SubjectID:     subject,
				Purpose:       purpose,
				Channel:       channel,
				LegalBasis:    "consent",
				Status:        status,
				Source:        "integration_concurrency",
				Jurisdiction:  "NG",
				NoticeVersion: "2026-08",
				Version:       1,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			receipt := domain.ConsentReceipt{
				ID:            "rcp-" + id,
				TenantID:      tenant,
				ConsentID:     consent.ID,
				SubjectID:     subject,
				Purpose:       purpose,
				Channel:       channel,
				NoticeVersion: "2026-08",
				Jurisdiction:  "NG",
				ReceiptHash:   "hash-" + id,
				IssuedAt:      now,
			}
			consent.ReceiptID = receipt.ID

			audit := domain.AuditEvent{
				ID:         "aud-" + id,
				TenantID:   tenant,
				ActorID:    fmt.Sprintf("writer-%02d", idx),
				Action:     "consent." + string(status),
				ConsentID:  consent.ID,
				OccurredAt: now,
			}
			outbox := domain.OutboxEvent{
				ID:          "evt-" + id,
				TenantID:    tenant,
				EventType:   "consent." + string(status),
				AggregateID: consent.ID,
				Payload:     map[string]any{"subject_id": subject, "idx": idx},
				OccurredAt:  now,
			}

			<-startBarrier // Synchronize all 50 goroutines to strike at the exact same instant

			_, err := retryTransient(func() (domain.Consent, error) {
				return repo.RecordGoverned(context.Background(), consent, receipt, audit, outbox)
			})
			errs <- err
		}()
	}

	close(startBarrier) // Fire all 50 writers concurrently
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent writer failed: %v", err)
		}
	}

	// 1. Verify that exactly 50 consents were recorded
	var consentCount int
	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM consents WHERE tenant_id=$1 AND subject_id=$2", tenant, subject).Scan(&consentCount); err != nil {
		t.Fatal(err)
	}
	if consentCount != concurrency {
		t.Fatalf("expected %d consents recorded, got %d", concurrency, consentCount)
	}

	// 2. Verify that preference generation strictly reached 50 without lost increments
	var finalGen int64
	if err := repo.pool.QueryRow(ctx, "SELECT generation FROM consent_preference_state WHERE tenant_id=$1 AND subject_id=$2 AND purpose=$3 AND channel=$4", tenant, subject, purpose, channel).Scan(&finalGen); err != nil {
		t.Fatal(err)
	}
	if finalGen != int64(concurrency) {
		t.Fatalf("preference generation must equal %d, got %d", concurrency, finalGen)
	}

	// 3. Verify that exactly 50 receipts, 50 audit events, and 50 outbox events exist
	var rcpCount, audCount, outCount int
	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM consent_receipts WHERE tenant_id=$1 AND subject_id=$2", tenant, subject).Scan(&rcpCount); err != nil {
		t.Fatal(err)
	}
	if rcpCount != concurrency {
		t.Fatalf("expected %d receipts, got %d", concurrency, rcpCount)
	}

	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM consent_audit_events WHERE tenant_id=$1", tenant).Scan(&audCount); err != nil {
		t.Fatal(err)
	}
	if audCount != concurrency {
		t.Fatalf("expected %d audit events, got %d", concurrency, audCount)
	}

	if err := repo.pool.QueryRow(ctx, "SELECT count(*) FROM consent_outbox_events WHERE tenant_id=$1", tenant).Scan(&outCount); err != nil {
		t.Fatal(err)
	}
	if outCount != concurrency {
		t.Fatalf("expected %d outbox events, got %d", concurrency, outCount)
	}

	// 4. Verify highest generation consent row matches latest generation
	var topGen int64
	if err := repo.pool.QueryRow(ctx, "SELECT preference_generation FROM consents WHERE tenant_id=$1 AND subject_id=$2 ORDER BY preference_generation DESC LIMIT 1", tenant, subject).Scan(&topGen); err != nil {
		t.Fatal(err)
	}
	if topGen != int64(concurrency) {
		t.Fatalf("top consent preference_generation must be %d, got %d", concurrency, topGen)
	}
}
