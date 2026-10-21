package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
)

func TestPostgresGovernedIdempotencyConcurrentReplay(t *testing.T) {
	databaseURL := integrationDSN(t)
	repo, err := NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	now := time.Now().UTC()
	run := runID(t)
	tenant := "idem-" + run
	subject := "cus-" + run
	idemKey := "customer360:local-" + run
	reqHashA := "request-hash-a-" + run
	reqHashB := "request-hash-b-" + run

	// Every global primary key (consents, receipts, audit and outbox ids),
	// tenant, subject, and idempotency key is run-unique, so the test can be
	// repeated sequentially or concurrently against the same database without
	// collision.
	candidate := func(suffix string) (domain.Consent, domain.ConsentReceipt, domain.AuditEvent, domain.OutboxEvent) {
		id := run + "-" + suffix
		consent := domain.Consent{
			ID:            "con-" + id,
			TenantID:      tenant,
			SubjectID:     subject,
			Purpose:       "marketing",
			Channel:       "email",
			LegalBasis:    "consent",
			Status:        domain.StatusGranted,
			Source:        "customer360",
			Scope:         map[string]string{"source_record_id": "local-" + run},
			Jurisdiction:  "NG",
			NoticeVersion: "2026-08",
			Version:       1,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		consent.GrantedAt = &now
		receipt := domain.ConsentReceipt{
			ID:            "rcp-" + id,
			TenantID:      tenant,
			ConsentID:     consent.ID,
			SubjectID:     consent.SubjectID,
			Purpose:       consent.Purpose,
			Channel:       consent.Channel,
			NoticeVersion: consent.NoticeVersion,
			Jurisdiction:  consent.Jurisdiction,
			ReceiptHash:   "hash-" + id,
			IssuedAt:      now,
		}
		consent.ReceiptID = receipt.ID
		audit := domain.AuditEvent{
			ID:         "aud-" + id,
			TenantID:   tenant,
			ActorID:    "svc-customer360",
			Action:     "consent.granted",
			ConsentID:  consent.ID,
			OccurredAt: now,
		}
		outbox := domain.OutboxEvent{
			ID:          "evt-" + id,
			TenantID:    tenant,
			EventType:   "consent.granted",
			AggregateID: consent.ID,
			Payload:     map[string]any{"subject_id": consent.SubjectID},
			OccurredAt:  now,
		}
		return consent, receipt, audit, outbox
	}

	var wg sync.WaitGroup
	results := make(chan domain.Consent, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		suffix := fmt.Sprintf("%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			consent, receipt, audit, outbox := candidate(suffix)
			result, err := retryTransient(func() (domain.Consent, error) {
				return repo.RecordGovernedIdempotent(context.Background(), idemKey, reqHashA, consent, receipt, audit, outbox)
			})
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent exact replay failed: %v", err)
		}
	}
	var winner string
	for result := range results {
		if winner == "" {
			winner = result.ID
		}
		if result.ID != winner || result.ReceiptID == "" {
			t.Fatalf("replays must converge on one original record: winner=%q result=%+v", winner, result)
		}
	}
	assertTenantRowCounts(t, repo, tenant, 1)

	conflicting, receipt, audit, outbox := candidate("conflict")
	if _, err := repo.RecordGovernedIdempotent(context.Background(), idemKey, reqHashB, conflicting, receipt, audit, outbox); !errors.Is(err, ErrConflict) {
		t.Fatalf("semantic key reuse must conflict, got %v", err)
	}
	assertTenantRowCounts(t, repo, tenant, 1)

	// A later exact replay with fresh candidate IDs still returns the original.
	replay, receipt, audit, outbox := candidate("late-replay")
	stored, err := repo.RecordGovernedIdempotent(context.Background(), idemKey, reqHashA, replay, receipt, audit, outbox)
	if err != nil || stored.ID != winner {
		t.Fatalf("late exact replay must return the original record: stored=%+v err=%v", stored, err)
	}
	if stored.Status != domain.StatusGranted {
		t.Fatalf("snapshot status must be granted, got %v", stored.Status)
	}
	assertTenantRowCounts(t, repo, tenant, 1)

	// Verify FindIdempotencySnapshot returns the stored snapshot
	snap, snapHash, found, err := repo.FindIdempotencySnapshot(context.Background(), tenant, idemKey)
	if err != nil || !found || snapHash != reqHashA || snap.ID != winner {
		t.Fatalf("FindIdempotencySnapshot failed: snap=%+v hash=%s found=%v err=%v", snap, snapHash, found, err)
	}
	if snap.Status != domain.StatusGranted {
		t.Fatalf("FindIdempotencySnapshot status must be granted, got %v", snap.Status)
	}

	// Withdraw the consent in storage
	withdrawnCandidate := stored
	withdrawnCandidate.Status = domain.StatusWithdrawn
	withdrawnCandidate.WithdrawnAt = &now
	withdrawnCandidate.UpdatedAt = now
	withdrawAudit := domain.AuditEvent{ID: "aud-withdraw-" + run, TenantID: tenant, ActorID: "svc", Action: "consent.withdrawn", ConsentID: winner, OccurredAt: now}
	withdrawOutbox := domain.OutboxEvent{ID: "evt-withdraw-" + run, TenantID: tenant, EventType: "consent.withdrawn", AggregateID: winner, Payload: map[string]any{}, OccurredAt: now}
	if _, err := repo.Withdraw(context.Background(), withdrawnCandidate, withdrawAudit, withdrawOutbox); err != nil {
		t.Fatalf("Withdraw failed: %v", err)
	}

	// Idempotent replay must STILL return the exact original snapshot with StatusGranted (T-025)
	replayedAfterWithdraw, err := repo.RecordGovernedIdempotent(context.Background(), idemKey, reqHashA, replay, receipt, audit, outbox)
	if err != nil {
		t.Fatalf("replay after withdrawal failed: %v", err)
	}
	if replayedAfterWithdraw.Status != domain.StatusGranted {
		t.Fatalf("replay after withdrawal must return original snapshot with granted status, got %v", replayedAfterWithdraw.Status)
	}
}

func TestMemoryGovernedIdempotencyConcurrentReplay(t *testing.T) {
	repo := NewMemory()
	now := time.Now().UTC()
	run := runID(t)
	tenant := "idem-mem-" + run
	subject := "cus-mem-" + run
	idemKey := "customer360:mem-" + run
	reqHashA := "req-hash-a-" + run
	reqHashB := "req-hash-b-" + run

	candidate := func(suffix string) (domain.Consent, domain.ConsentReceipt, domain.AuditEvent, domain.OutboxEvent) {
		id := run + "-" + suffix
		consent := domain.Consent{
			ID:            "con-" + id,
			TenantID:      tenant,
			SubjectID:     subject,
			Purpose:       "marketing",
			Channel:       "email",
			LegalBasis:    "consent",
			Status:        domain.StatusGranted,
			Source:        "customer360",
			Scope:         map[string]string{"source_record_id": "local-" + run},
			Jurisdiction:  "NG",
			NoticeVersion: "2026-08",
			Version:       1,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		consent.GrantedAt = &now
		receipt := domain.ConsentReceipt{
			ID:            "rcp-" + id,
			TenantID:      tenant,
			ConsentID:     consent.ID,
			SubjectID:     consent.SubjectID,
			Purpose:       consent.Purpose,
			Channel:       consent.Channel,
			NoticeVersion: consent.NoticeVersion,
			Jurisdiction:  consent.Jurisdiction,
			ReceiptHash:   "hash-" + id,
			IssuedAt:      now,
		}
		consent.ReceiptID = receipt.ID
		audit := domain.AuditEvent{
			ID:         "aud-" + id,
			TenantID:   tenant,
			ActorID:    "svc",
			Action:     "consent.granted",
			ConsentID:  consent.ID,
			OccurredAt: now,
		}
		outbox := domain.OutboxEvent{
			ID:          "evt-" + id,
			TenantID:    tenant,
			EventType:   "consent.granted",
			AggregateID: consent.ID,
			Payload:     map[string]any{"subject_id": consent.SubjectID},
			OccurredAt:  now,
		}
		return consent, receipt, audit, outbox
	}

	var wg sync.WaitGroup
	results := make(chan domain.Consent, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		suffix := fmt.Sprintf("%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			consent, receipt, audit, outbox := candidate(suffix)
			result, err := repo.RecordGovernedIdempotent(context.Background(), idemKey, reqHashA, consent, receipt, audit, outbox)
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent replay failed: %v", err)
		}
	}
	var winner string
	for result := range results {
		if winner == "" {
			winner = result.ID
		}
		if result.ID != winner || result.ReceiptID == "" {
			t.Fatalf("replays must converge on one original record: winner=%q result=%+v", winner, result)
		}
	}

	conflicting, receipt, audit, outbox := candidate("conflict")
	if _, err := repo.RecordGovernedIdempotent(context.Background(), idemKey, reqHashB, conflicting, receipt, audit, outbox); !errors.Is(err, ErrConflict) {
		t.Fatalf("semantic key reuse must conflict, got %v", err)
	}

	replay, receipt, audit, outbox := candidate("late-replay")
	stored, err := repo.RecordGovernedIdempotent(context.Background(), idemKey, reqHashA, replay, receipt, audit, outbox)
	if err != nil || stored.ID != winner {
		t.Fatalf("late exact replay must return original: stored=%+v err=%v", stored, err)
	}
	if stored.Status != domain.StatusGranted {
		t.Fatalf("snapshot status must be granted, got %v", stored.Status)
	}

	snap, snapHash, found, err := repo.FindIdempotencySnapshot(context.Background(), tenant, idemKey)
	if err != nil || !found || snapHash != reqHashA || snap.ID != winner {
		t.Fatalf("FindIdempotencySnapshot failed: snap=%+v hash=%s found=%v err=%v", snap, snapHash, found, err)
	}
}

// assertTenantRowCounts proves rejected or replayed writes left no hidden
// consent, receipt, audit, outbox or idempotency rows behind.
func assertTenantRowCounts(t *testing.T, repo *Postgres, tenant string, want int) {
	t.Helper()
	for _, table := range []string{"consents", "consent_receipts", "consent_audit_events", "consent_outbox_events", "consent_idempotency"} {
		var count int
		if err := repo.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table+` WHERE tenant_id=$1`, tenant).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("%s has %d rows for %s, want %d", table, count, tenant, want)
		}
	}
}
