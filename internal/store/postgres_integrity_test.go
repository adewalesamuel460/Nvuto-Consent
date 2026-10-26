package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestAT028_CrossTenantReceiptFKViolation_RejectedByDatabase (AT-028) proves that
// composite foreign keys prevent cross-tenant references: inserting a receipt under
// Tenant B referencing a consent ID owned by Tenant A is rejected by PostgreSQL / YSQL.
func TestAT028_CrossTenantReceiptFKViolation_RejectedByDatabase(t *testing.T) {
	databaseURL := integrationDSN(t)
	repo, err := NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	ctx := context.Background()
	run := runID(t)
	tenantA := "tenant-a-" + run
	tenantB := "tenant-b-" + run
	consentID := "con-a-" + run
	receiptID := "rcp-cross-" + run
	now := time.Now().UTC()

	// 1. Insert valid consent belonging to Tenant A
	_, err = repo.Pool().Exec(ctx, `
		INSERT INTO consents(id, tenant_id, subject_id, purpose, channel, legal_basis, status, source, created_at, updated_at)
		VALUES($1, $2, 'cust-1', 'marketing', 'email', 'consent', 'granted', 'portal', $3, $3)
	`, consentID, tenantA, now)
	if err != nil {
		t.Fatalf("failed to insert consent in tenant A: %v", err)
	}

	// 2. Attempt to insert a receipt under Tenant B referencing Tenant A's consent
	_, crossErr := repo.Pool().Exec(ctx, `
		INSERT INTO consent_receipts(id, tenant_id, consent_id, subject_id, purpose, channel, notice_version, jurisdiction, evidence, receipt_hash, issued_at)
		VALUES($1, $2, $3, 'cust-1', 'marketing', 'email', '2026-08', 'NG', '{}'::jsonb, $4, $5)
	`, receiptID, tenantB, consentID, "hash-"+run, now)

	if crossErr == nil {
		t.Fatal("expected cross-tenant receipt insert to fail with foreign key violation, got nil")
	}

	var pgErr *pgconn.PgError
	if !errors.As(crossErr, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got: %T (%v)", crossErr, crossErr)
	}

	// SQLSTATE 23503 = foreign_key_violation
	if pgErr.Code != "23503" {
		t.Fatalf("expected SQLSTATE 23503 (foreign_key_violation), got %s", pgErr.Code)
	}
	if pgErr.ConstraintName != "fk_consent_receipts_consent" {
		t.Fatalf("expected constraint fk_consent_receipts_consent, got %s", pgErr.ConstraintName)
	}
}

// TestAT028_CrossTenantIdempotencyFKViolation_RejectedByDatabase (AT-028) proves
// that the idempotency ledger cannot record a cross-tenant consent reference.
func TestAT028_CrossTenantIdempotencyFKViolation_RejectedByDatabase(t *testing.T) {
	databaseURL := integrationDSN(t)
	repo, err := NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	ctx := context.Background()
	run := runID(t)
	tenantA := "tenant-a-" + run
	tenantB := "tenant-b-" + run
	consentID := "con-a-" + run
	now := time.Now().UTC()

	// 1. Insert consent in Tenant A
	_, err = repo.Pool().Exec(ctx, `
		INSERT INTO consents(id, tenant_id, subject_id, purpose, channel, legal_basis, status, source, created_at, updated_at)
		VALUES($1, $2, 'cust-1', 'marketing', 'email', 'consent', 'granted', 'portal', $3, $3)
	`, consentID, tenantA, now)
	if err != nil {
		t.Fatalf("failed to insert consent in tenant A: %v", err)
	}

	// 2. Attempt to insert idempotency row in Tenant B pointing to Tenant A's consent
	_, crossErr := repo.Pool().Exec(ctx, `
		INSERT INTO consent_idempotency(tenant_id, idempotency_key, request_hash, consent_id, created_at)
		VALUES($1, 'key-1', 'hash-1', $2, $3)
	`, tenantB, consentID, now)

	if crossErr == nil {
		t.Fatal("expected cross-tenant idempotency insert to fail with foreign key violation, got nil")
	}

	var pgErr *pgconn.PgError
	if !errors.As(crossErr, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got: %T (%v)", crossErr, crossErr)
	}

	// SQLSTATE 23503 = foreign_key_violation
	if pgErr.Code != "23503" {
		t.Fatalf("expected SQLSTATE 23503 (foreign_key_violation), got %s", pgErr.Code)
	}
	if pgErr.ConstraintName != "fk_consent_idempotency_consent" {
		t.Fatalf("expected constraint fk_consent_idempotency_consent, got %s", pgErr.ConstraintName)
	}
}

// TestAT028_TemporalExpiryBeforeCreatedAt_RejectedByDatabase (AT-028) proves that
// temporal check constraints in PostgreSQL / YSQL prevent inserting consents where
// expires_at <= created_at, even if bypassing the application validation layer.
func TestAT028_TemporalExpiryBeforeCreatedAt_RejectedByDatabase(t *testing.T) {
	databaseURL := integrationDSN(t)
	repo, err := NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	ctx := context.Background()
	run := runID(t)
	tenant := "tenant-temporal-" + run
	consentID := "con-expired-" + run
	now := time.Now().UTC()
	invalidExpiry := now.Add(-2 * time.Hour) // expires_at before created_at!

	// Attempt raw SQL insert with invalid temporal relation
	_, insertErr := repo.Pool().Exec(ctx, `
		INSERT INTO consents(id, tenant_id, subject_id, purpose, channel, legal_basis, status, source, created_at, updated_at, expires_at)
		VALUES($1, $2, 'cust-temp', 'marketing', 'email', 'consent', 'granted', 'portal', $3, $3, $4)
	`, consentID, tenant, now, invalidExpiry)

	if insertErr == nil {
		t.Fatal("expected insert with expires_at <= created_at to fail with check constraint violation, got nil")
	}

	var pgErr *pgconn.PgError
	if !errors.As(insertErr, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got: %T (%v)", insertErr, insertErr)
	}

	// SQLSTATE 23514 = check_violation
	if pgErr.Code != "23514" {
		t.Fatalf("expected SQLSTATE 23514 (check_violation), got %s", pgErr.Code)
	}
	if pgErr.ConstraintName != "chk_consents_temporal_expiry" {
		t.Fatalf("expected constraint chk_consents_temporal_expiry, got %s", pgErr.ConstraintName)
	}
}

// TestAT028_TemporalWithdrawnBeforeCreatedAt_RejectedByDatabase (AT-028) proves that
// withdrawn_at < created_at is rejected by the database.
func TestAT028_TemporalWithdrawnBeforeCreatedAt_RejectedByDatabase(t *testing.T) {
	databaseURL := integrationDSN(t)
	repo, err := NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	ctx := context.Background()
	run := runID(t)
	tenant := "tenant-withdrawn-" + run
	consentID := "con-withdrawn-" + run
	now := time.Now().UTC()
	invalidWithdrawn := now.Add(-10 * time.Minute)

	_, insertErr := repo.Pool().Exec(ctx, `
		INSERT INTO consents(id, tenant_id, subject_id, purpose, channel, legal_basis, status, source, created_at, updated_at, withdrawn_at)
		VALUES($1, $2, 'cust-temp', 'marketing', 'email', 'consent', 'withdrawn', 'portal', $3, $3, $4)
	`, consentID, tenant, now, invalidWithdrawn)

	if insertErr == nil {
		t.Fatal("expected insert with withdrawn_at < created_at to fail, got nil")
	}

	var pgErr *pgconn.PgError
	if !errors.As(insertErr, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got: %T (%v)", insertErr, insertErr)
	}

	if pgErr.Code != "23514" {
		t.Fatalf("expected SQLSTATE 23514 (check_violation), got %s", pgErr.Code)
	}
	if pgErr.ConstraintName != "chk_consents_temporal_withdrawn" {
		t.Fatalf("expected constraint chk_consents_temporal_withdrawn, got %s", pgErr.ConstraintName)
	}
}

// TestAT028_InvalidStatusValue_RejectedByDatabase (AT-028) proves that an invalid
// lifecycle status value is rejected by the database.
func TestAT028_InvalidStatusValue_RejectedByDatabase(t *testing.T) {
	databaseURL := integrationDSN(t)
	repo, err := NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	ctx := context.Background()
	run := runID(t)
	tenant := "tenant-status-" + run
	consentID := "con-status-" + run
	now := time.Now().UTC()

	_, insertErr := repo.Pool().Exec(ctx, `
		INSERT INTO consents(id, tenant_id, subject_id, purpose, channel, legal_basis, status, source, created_at, updated_at)
		VALUES($1, $2, 'cust-temp', 'marketing', 'email', 'consent', 'arbitrary_status', 'portal', $3, $3)
	`, consentID, tenant, now)

	if insertErr == nil {
		t.Fatal("expected insert with invalid status to fail with check constraint violation, got nil")
	}

	var pgErr *pgconn.PgError
	if !errors.As(insertErr, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got: %T (%v)", insertErr, insertErr)
	}

	if pgErr.Code != "23514" {
		t.Fatalf("expected SQLSTATE 23514, got %s", pgErr.Code)
	}
	if pgErr.ConstraintName != "chk_consents_status_valid" {
		t.Fatalf("expected constraint chk_consents_status_valid, got %s", pgErr.ConstraintName)
	}
}

// TestAT028_SuppressionScopeShape_RejectedByDatabase (AT-028) proves that invalid
// scope shapes (e.g. global scope with non-empty channel) are rejected by DB CHECK.
func TestAT028_SuppressionScopeShape_RejectedByDatabase(t *testing.T) {
	databaseURL := integrationDSN(t)
	repo, err := NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	ctx := context.Background()
	run := runID(t)
	tenant := "tenant-supp-" + run
	suppID := "sup-invalid-" + run
	now := time.Now().UTC()

	// Global scope with channel='email' violates consent_suppressions_scope_shape_check
	_, insertErr := repo.Pool().Exec(ctx, `
		INSERT INTO consent_suppressions(id, tenant_id, subject_id, scope, purpose, channel, reason, updated_at)
		VALUES($1, $2, 'cust-supp', 'global', '', 'email', 'test_reason', $3)
	`, suppID, tenant, now)

	if insertErr == nil {
		t.Fatal("expected insert with invalid suppression scope shape to fail, got nil")
	}

	var pgErr *pgconn.PgError
	if !errors.As(insertErr, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got: %T (%v)", insertErr, insertErr)
	}

	if pgErr.Code != "23514" {
		t.Fatalf("expected SQLSTATE 23514, got %s", pgErr.Code)
	}
	if pgErr.ConstraintName != "consent_suppressions_scope_shape_check" {
		t.Fatalf("expected constraint consent_suppressions_scope_shape_check, got %s", pgErr.ConstraintName)
	}
}

// TestIntegration_AT028_DatabaseConstraintsAndIntegrity matches the -run Integration filter.
func TestIntegration_AT028_DatabaseConstraintsAndIntegrity(t *testing.T) {
	TestAT028_CrossTenantReceiptFKViolation_RejectedByDatabase(t)
	TestAT028_CrossTenantIdempotencyFKViolation_RejectedByDatabase(t)
	TestAT028_TemporalExpiryBeforeCreatedAt_RejectedByDatabase(t)
	TestAT028_TemporalWithdrawnBeforeCreatedAt_RejectedByDatabase(t)
	TestAT028_InvalidStatusValue_RejectedByDatabase(t)
	TestAT028_SuppressionScopeShape_RejectedByDatabase(t)
}
