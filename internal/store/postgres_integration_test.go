package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestAT012_BoundedConnectionPool verifies that NewPostgres sets explicit bounds
// on MaxConns, MinConns, MaxConnIdleTime, and MaxConnLifetime.
func TestAT012_BoundedConnectionPool(t *testing.T) {
	t.Run("PoolConfigBounds", func(t *testing.T) {
		databaseURL := integrationDSN(t)
		repo, err := NewPostgres(context.Background(), databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()

		cfg := repo.Pool().Config()
		if cfg.MaxConns != 25 {
			t.Errorf("expected MaxConns=25, got %d", cfg.MaxConns)
		}
		if cfg.MinConns != 2 {
			t.Errorf("expected MinConns=2, got %d", cfg.MinConns)
		}
		if cfg.MaxConnLifetime != 45*time.Minute {
			t.Errorf("expected MaxConnLifetime=45m, got %v", cfg.MaxConnLifetime)
		}
		if cfg.MaxConnIdleTime != 5*time.Minute {
			t.Errorf("expected MaxConnIdleTime=5m, got %v", cfg.MaxConnIdleTime)
		}
	})
}

// TestAT012_ContextCancellationAbortsPostgresQuery proves that a cancelled
// context.Context immediately aborts an in-progress Postgres query and returns
// a typed context error (context.Canceled or context.DeadlineExceeded).
func TestAT012_ContextCancellationAbortsPostgresQuery(t *testing.T) {
	t.Run("MemoryStore_LiveCheck", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := live(ctx)
		if !errors.Is(err, context.Canceled) && domain.KindOf(err) != domain.KindCanceled {
			t.Fatalf("expected KindCanceled from live(cancelled ctx), got %v", err)
		}

		deadlineCtx, deadlineCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer deadlineCancel()

		errDeadline := live(deadlineCtx)
		if !errors.Is(errDeadline, context.DeadlineExceeded) && domain.KindOf(errDeadline) != domain.KindUnavailable {
			t.Fatalf("expected KindUnavailable from live(expired ctx), got %v", errDeadline)
		}

		// Verify Memory store methods abort immediately on cancelled context
		mem := NewMemory()
		_, errMem := mem.Get(ctx, "tenant", "id")
		if !errors.Is(errMem, context.Canceled) && domain.KindOf(errMem) != domain.KindCanceled {
			t.Fatalf("expected Memory.Get to abort on cancelled ctx, got %v", errMem)
		}
	})

	t.Run("Postgres_PreCancelledContext", func(t *testing.T) {
		databaseURL := integrationDSN(t)
		repo, err := NewPostgres(context.Background(), databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		start := time.Now()
		err = repo.inTx(ctx, func(tx pgx.Tx) error {
			_, execErr := tx.Exec(ctx, "SELECT 1")
			return execErr
		})
		elapsed := time.Since(start)

		if elapsed > 500*time.Millisecond {
			t.Fatalf("query was not aborted immediately: elapsed=%v", elapsed)
		}
		if !errors.Is(err, context.Canceled) && domain.KindOf(err) != domain.KindCanceled {
			t.Fatalf("expected context.Canceled or KindCanceled, got %v", err)
		}
	})

	t.Run("Postgres_InFlightCancellationAbortsQuery", func(t *testing.T) {
		databaseURL := integrationDSN(t)
		repo, err := NewPostgres(context.Background(), databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(30 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		var dummy int
		queryErr := repo.Pool().QueryRow(ctx, "SELECT 1 FROM pg_sleep(5)").Scan(&dummy)
		elapsed := time.Since(start)

		if elapsed > 1500*time.Millisecond {
			t.Fatalf("in-flight cancellation did not abort query promptly: elapsed=%v", elapsed)
		}
		classified := classify(queryErr)
		if !errors.Is(queryErr, context.Canceled) && domain.KindOf(classified) != domain.KindCanceled {
			t.Fatalf("expected query to abort with context.Canceled, got %v (classified: %v)", queryErr, classified)
		}
	})

	t.Run("Postgres_DeadlineExceededAbortsQuery", func(t *testing.T) {
		databaseURL := integrationDSN(t)
		repo, err := NewPostgres(context.Background(), databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()

		start := time.Now()
		var dummy int
		queryErr := repo.Pool().QueryRow(ctx, "SELECT 1 FROM pg_sleep(5)").Scan(&dummy)
		elapsed := time.Since(start)

		if elapsed > 1500*time.Millisecond {
			t.Fatalf("query did not abort on deadline exceeded: elapsed=%v", elapsed)
		}
		classified := classify(queryErr)
		if !errors.Is(queryErr, context.DeadlineExceeded) && domain.KindOf(classified) != domain.KindUnavailable {
			t.Fatalf("expected DeadlineExceeded / KindUnavailable, got %v (classified: %v)", queryErr, classified)
		}
	})
}

// TestAT012_SerializationRetryWithoutDuplicates proves that YSQL serialization
// anomalies (SQLSTATE 40001) are caught by the retry loop, retried with backoff
// obeying ctx.Done(), and commit without leaking duplicate audit or outbox records.
func TestAT012_SerializationRetryWithoutDuplicates(t *testing.T) {
	t.Run("RetryLoop_Catches40001_ResolvesWithoutDuplicates", func(t *testing.T) {
		ctx := context.Background()
		policy := retryPolicy{attempts: 4, base: 10 * time.Millisecond, max: 50 * time.Millisecond}

		var attempts int
		var auditEvents []string
		var outboxEvents []string

		err := withTxRetry(ctx, policy, func(ctx context.Context) error {
			attempts++
			// Simulate uncommitted writes in transaction attempt
			localAudit := fmt.Sprintf("audit-attempt-%d", attempts)
			localOutbox := fmt.Sprintf("outbox-attempt-%d", attempts)

			if attempts < 3 {
				// Simulating YugabyteDB SQLSTATE 40001 serialization anomaly
				// on the first 2 attempts. Transaction rolls back, discarding uncommitted writes.
				return &pgconn.PgError{Code: "40001", Message: "snapshot isolation transaction conflict"}
			}

			// Third attempt succeeds and commits writes atomically
			auditEvents = append(auditEvents, localAudit)
			outboxEvents = append(outboxEvents, localOutbox)
			return nil
		})

		if err != nil {
			t.Fatalf("expected withTxRetry to resolve 40001 conflict, got %v", err)
		}
		if attempts != 3 {
			t.Fatalf("expected exactly 3 attempts, got %d", attempts)
		}
		if len(auditEvents) != 1 || len(outboxEvents) != 1 {
			t.Fatalf("duplicate records leaked on retry: audits=%v, outbox=%v", auditEvents, outboxEvents)
		}
	})

	t.Run("RetryLoop_AbortsImmediatelyWhenContextCancelledDuringBackoff", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		policy := retryPolicy{attempts: 5, base: 500 * time.Millisecond, max: time.Second}

		var attempts int
		go func() {
			time.Sleep(30 * time.Millisecond)
			cancel() // Cancel while withTxRetry is in backoff timer
		}()

		start := time.Now()
		err := withTxRetry(ctx, policy, func(ctx context.Context) error {
			attempts++
			return &pgconn.PgError{Code: "40001", Message: "serialization failure"}
		})
		elapsed := time.Since(start)

		if elapsed > 300*time.Millisecond {
			t.Fatalf("withTxRetry did not abort immediately upon ctx.Done(): elapsed=%v", elapsed)
		}
		if !errors.Is(err, context.Canceled) && domain.KindOf(err) != domain.KindCanceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
		if attempts != 1 {
			t.Fatalf("expected 1 attempt before cancellation, got %d", attempts)
		}
	})

	t.Run("RetryLoop_ExhaustionReturnsContention", func(t *testing.T) {
		ctx := context.Background()
		policy := retryPolicy{attempts: 3, base: 5 * time.Millisecond, max: 10 * time.Millisecond}

		var attempts int
		err := withTxRetry(ctx, policy, func(ctx context.Context) error {
			attempts++
			return &pgconn.PgError{Code: "40001", Message: "persistent conflict"}
		})

		if attempts != 3 {
			t.Fatalf("expected exactly 3 attempts before exhaustion, got %d", attempts)
		}
		classified := classify(err)
		if !errors.Is(classified, domain.ErrContention) {
			t.Fatalf("expected ErrContention on 40001 exhaustion, got %v", classified)
		}
	})

	t.Run("Postgres_InTx_RollbackOn40001LeavesNoDuplicateRows", func(t *testing.T) {
		databaseURL := integrationDSN(t)
		repo, err := NewPostgres(context.Background(), databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()

		ctx := context.Background()
		run := runID(t)
		tenant := "retry-" + run
		auditID := "aud-retry-" + run
		outboxID := "out-retry-" + run
		now := time.Now().UTC()

		var attempts int
		err = repo.inTx(ctx, func(tx pgx.Tx) error {
			attempts++
			// Insert audit and outbox in this transaction
			err := appendEffectsTx(ctx, tx, domain.AuditEvent{
				ID:           auditID,
				TenantID:     tenant,
				ActorID:      "actor",
				Action:       "consent.granted",
				ConsentID:    "con-" + run,
				OccurredAt:   now,
				ResourceType: "consent",
				Revision:     1,
			}, domain.OutboxEvent{
				ID:          outboxID,
				TenantID:    tenant,
				EventType:   "consent.granted",
				AggregateID: "con-" + run,
				OccurredAt:  now,
			})
			if err != nil {
				return err
			}

			// Inject 40001 serialization failure on attempt 1
			if attempts == 1 {
				return &pgconn.PgError{Code: "40001", Message: "simulated serialization conflict"}
			}
			return nil
		})

		if err != nil {
			t.Fatalf("inTx failed: %v", err)
		}
		if attempts != 2 {
			t.Fatalf("expected 2 attempts, got %d", attempts)
		}

		// Verify that exactly 1 copy of audit and outbox was committed and no duplicates were left by attempt 1
		var auditCount int
		if err := repo.Pool().QueryRow(ctx, "SELECT count(*) FROM consent_audit_events WHERE tenant_id=$1 AND id=$2", tenant, auditID).Scan(&auditCount); err != nil {
			t.Fatal(err)
		}
		if auditCount != 1 {
			t.Fatalf("expected exactly 1 audit row, got %d", auditCount)
		}

		var outboxCount int
		if err := repo.Pool().QueryRow(ctx, "SELECT count(*) FROM consent_outbox_events WHERE tenant_id=$1 AND id=$2", tenant, outboxID).Scan(&outboxCount); err != nil {
			t.Fatal(err)
		}
		if outboxCount != 1 {
			t.Fatalf("expected exactly 1 outbox row, got %d", outboxCount)
		}
	})

	t.Run("Postgres_ConcurrentWriters_ResolveSafely", func(t *testing.T) {
		databaseURL := integrationDSN(t)
		repo, err := NewPostgres(context.Background(), databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()

		ctx := context.Background()
		run := runID(t)
		tenant := "race-" + run
		subject := "subject-" + run
		purpose := "marketing"
		channel := "email"

		now := time.Now().UTC()
		var wait sync.WaitGroup
		var successCount atomic.Int32
		var errorsList sync.Mutex
		var capturedErrors []error

		concurrency := 6
		for i := 0; i < concurrency; i++ {
			wait.Add(1)
			go func(idx int) {
				defer wait.Done()
				consentID := fmt.Sprintf("con-%s-%d", run, idx)
				auditID := fmt.Sprintf("aud-%s-%d", run, idx)
				outboxID := fmt.Sprintf("out-%s-%d", run, idx)

				consent := domain.Consent{
					ID:            consentID,
					TenantID:      tenant,
					SubjectID:     subject,
					Purpose:       purpose,
					Channel:       channel,
					LegalBasis:    "consent",
					Status:        domain.StatusGranted,
					Source:        "test",
					Jurisdiction:  "NG",
					NoticeVersion: "2026-08",
					Version:       1,
					CreatedAt:     now,
					UpdatedAt:     now.Add(time.Duration(idx) * time.Millisecond),
				}
				audit := domain.AuditEvent{
					ID:           auditID,
					TenantID:     tenant,
					ActorID:      "tester",
					Action:       "consent.granted",
					ConsentID:    consentID,
					OccurredAt:   now,
					ResourceType: "consent",
					Revision:     1,
				}
				outbox := domain.OutboxEvent{
					ID:          outboxID,
					TenantID:    tenant,
					EventType:   "consent.granted",
					AggregateID: consentID,
					OccurredAt:  now,
				}

				_, err := repo.RecordGoverned(ctx, consent, domain.ConsentReceipt{}, audit, outbox)
				if err != nil {
					errorsList.Lock()
					capturedErrors = append(capturedErrors, err)
					errorsList.Unlock()
				} else {
					successCount.Add(1)
				}
			}(i)
		}

		wait.Wait()
		if len(capturedErrors) > 0 {
			t.Fatalf("concurrent RecordGoverned failed with errors: %v", capturedErrors)
		}
		if int(successCount.Load()) != concurrency {
			t.Fatalf("expected all %d concurrent writes to succeed, got %d", concurrency, successCount.Load())
		}

		// Verify every audit event and outbox event is present exactly once
		var totalAudit int
		if err := repo.Pool().QueryRow(ctx, "SELECT count(*) FROM consent_audit_events WHERE tenant_id=$1", tenant).Scan(&totalAudit); err != nil {
			t.Fatal(err)
		}
		if totalAudit != concurrency {
			t.Fatalf("expected %d audit records, got %d", concurrency, totalAudit)
		}

		var totalOutbox int
		if err := repo.Pool().QueryRow(ctx, "SELECT count(*) FROM consent_outbox_events WHERE tenant_id=$1", tenant).Scan(&totalOutbox); err != nil {
			t.Fatal(err)
		}
		if totalOutbox != concurrency {
			t.Fatalf("expected %d outbox records, got %d", concurrency, totalOutbox)
		}
	})
}

func TestIntegration_AT012_BoundedConnectionPool(t *testing.T) {
	TestAT012_BoundedConnectionPool(t)
}

func TestIntegration_AT012_ContextCancellationAbortsPostgresQuery(t *testing.T) {
	TestAT012_ContextCancellationAbortsPostgresQuery(t)
}

func TestIntegration_AT012_SerializationRetryWithoutDuplicates(t *testing.T) {
	TestAT012_SerializationRetryWithoutDuplicates(t)
}
