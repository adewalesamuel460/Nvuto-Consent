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
	// Every global primary key (consents, receipts, audit and outbox ids) is
	// run-unique, so the test can be repeated against the same database.
	candidate := func(suffix string) (domain.Consent, domain.ConsentReceipt, domain.AuditEvent, domain.OutboxEvent) {
		id := run + "-" + suffix
		consent := domain.Consent{ID: "con-" + id, TenantID: tenant, SubjectID: "cus-1", Purpose: "marketing", Channel: "email", LegalBasis: "consent", Status: domain.StatusGranted, Source: "customer360", Scope: map[string]string{"source_record_id": "local-1"}, Jurisdiction: "NG", NoticeVersion: "2026-08", Version: 1, CreatedAt: now, UpdatedAt: now}
		consent.GrantedAt = &now
		receipt := domain.ConsentReceipt{ID: "rcp-" + id, TenantID: tenant, ConsentID: consent.ID, SubjectID: consent.SubjectID, Purpose: consent.Purpose, Channel: consent.Channel, NoticeVersion: consent.NoticeVersion, Jurisdiction: consent.Jurisdiction, ReceiptHash: "hash-" + id, IssuedAt: now}
		consent.ReceiptID = receipt.ID
		audit := domain.AuditEvent{ID: "aud-" + id, TenantID: tenant, ActorID: "svc-customer360", Action: "consent.granted", ConsentID: consent.ID, OccurredAt: now}
		outbox := domain.OutboxEvent{ID: "evt-" + id, TenantID: tenant, EventType: "consent.granted", AggregateID: consent.ID, Payload: map[string]any{"subject_id": consent.SubjectID}, OccurredAt: now}
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
				return repo.RecordGovernedIdempotent(context.Background(), "customer360:local-1", "request-hash-a", consent, receipt, audit, outbox)
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
	if _, err := repo.RecordGovernedIdempotent(context.Background(), "customer360:local-1", "request-hash-b", conflicting, receipt, audit, outbox); !errors.Is(err, ErrConflict) {
		t.Fatalf("semantic key reuse must conflict, got %v", err)
	}
	assertTenantRowCounts(t, repo, tenant, 1)

	// A later exact replay with fresh candidate IDs still returns the original.
	replay, receipt, audit, outbox := candidate("late-replay")
	stored, err := repo.RecordGovernedIdempotent(context.Background(), "customer360:local-1", "request-hash-a", replay, receipt, audit, outbox)
	if err != nil || stored.ID != winner {
		t.Fatalf("late exact replay must return the original record: stored=%+v err=%v", stored, err)
	}
	assertTenantRowCounts(t, repo, tenant, 1)
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
