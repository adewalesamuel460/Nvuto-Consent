package store

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// validateDatabaseTLSLocal mirrors the connection validation in cmd/server and cmd/outbox-worker.
func validateDatabaseTLSLocal(databaseURL string, allowDev, isProduction bool) error {
	u, err := url.Parse(databaseURL)
	var sslmode string
	if err == nil && u != nil {
		sslmode = strings.ToLower(strings.TrimSpace(u.Query().Get("sslmode")))
	}
	if sslmode == "" {
		for _, part := range strings.Fields(databaseURL) {
			if strings.HasPrefix(strings.ToLower(part), "sslmode=") {
				sslmode = strings.ToLower(strings.TrimPrefix(part, "sslmode="))
				break
			}
		}
	}
	if sslmode == "disable" && !allowDev {
		return errors.New("sslmode=disable is prohibited in production configuration (requires ALLOW_DEV_HEADERS=true)")
	}
	if isProduction && sslmode != "verify-full" && sslmode != "require" && sslmode != "verify-ca" {
		return errors.New("DATABASE_URL requires sslmode=verify-full or sslmode=require in production")
	}
	return nil
}

// TestAT019_ConfigValidation_JWKSAndDatabaseTLS validates that:
// 1. http:// JWKS URLs are rejected at startup unless ALLOW_DEV_HEADERS=true
// 2. sslmode=disable is rejected outside dev configurations
// 3. sslmode=verify-full and sslmode=require are accepted in production
func TestAT019_ConfigValidation_JWKSAndDatabaseTLS(t *testing.T) {
	t.Run("JWKS_HTTP_Scheme_Rejected_When_Dev_Headers_Off", func(t *testing.T) {
		t.Setenv("ALLOW_DEV_HEADERS", "false")
		t.Setenv("JWT_JWKS_URL", "http://auth.example.com/.well-known/jwks.json")
		t.Setenv("JWT_ISSUER", "https://auth.example.com")
		t.Setenv("JWT_AUDIENCE", "nvuto-consent")
		t.Setenv("JWT_HMAC_SECRET", "")
		t.Setenv("ALLOW_HMAC_AUTH", "false")

		err := auth.ValidateConfig()
		if err == nil {
			t.Fatal("expected error rejecting http:// JWKS URL when ALLOW_DEV_HEADERS=false, got nil")
		}
		if !strings.Contains(err.Error(), "JWT_JWKS_URL must use the https:// scheme in production") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("JWKS_HTTP_Scheme_Accepted_When_Dev_Headers_On", func(t *testing.T) {
		t.Setenv("ALLOW_DEV_HEADERS", "true")
		t.Setenv("JWT_JWKS_URL", "http://localhost:8080/.well-known/jwks.json")
		t.Setenv("JWT_ISSUER", "https://auth.example.com")
		t.Setenv("JWT_AUDIENCE", "nvuto-consent")

		err := auth.ValidateConfig()
		if err != nil {
			t.Fatalf("expected http:// JWKS URL to be accepted with ALLOW_DEV_HEADERS=true, got %v", err)
		}
	})

	t.Run("JWKS_HTTPS_Scheme_Accepted_In_Production", func(t *testing.T) {
		t.Setenv("ALLOW_DEV_HEADERS", "false")
		t.Setenv("JWT_JWKS_URL", "https://auth.production.example.com/.well-known/jwks.json")
		t.Setenv("JWT_ISSUER", "https://auth.example.com")
		t.Setenv("JWT_AUDIENCE", "nvuto-consent")

		err := auth.ValidateConfig()
		if err != nil {
			t.Fatalf("expected https:// JWKS URL to be accepted, got %v", err)
		}
	})

	t.Run("Database_SSLMode_Disable_Rejected_When_Dev_Headers_Off", func(t *testing.T) {
		err := validateDatabaseTLSLocal("postgresql://user:pass@localhost:5432/db?sslmode=disable", false, false)
		if err == nil {
			t.Fatal("expected error rejecting sslmode=disable outside development, got nil")
		}
		if !strings.Contains(err.Error(), "sslmode=disable is prohibited") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("Database_SSLMode_Disable_Accepted_When_Dev_Headers_On", func(t *testing.T) {
		err := validateDatabaseTLSLocal("postgresql://user:pass@localhost:5432/db?sslmode=disable", true, false)
		if err != nil {
			t.Fatalf("expected sslmode=disable to be accepted when allowDev=true, got: %v", err)
		}
	})

	t.Run("Database_Production_Requires_SSLMode_Require_Or_VerifyFull", func(t *testing.T) {
		// Plain without sslmode in prod -> rejected
		err := validateDatabaseTLSLocal("postgresql://user:pass@localhost:5432/db", false, true)
		if err == nil || !strings.Contains(err.Error(), "DATABASE_URL requires sslmode=verify-full or sslmode=require") {
			t.Fatalf("expected production sslmode requirement error, got: %v", err)
		}

		// sslmode=require in prod -> accepted
		if err := validateDatabaseTLSLocal("postgresql://user:pass@localhost:5432/db?sslmode=require", false, true); err != nil {
			t.Fatalf("expected sslmode=require to pass in production, got: %v", err)
		}

		// sslmode=verify-full in prod -> accepted
		if err := validateDatabaseTLSLocal("postgresql://user:pass@localhost:5432/db?sslmode=verify-full", false, true); err != nil {
			t.Fatalf("expected sslmode=verify-full to pass in production, got: %v", err)
		}
	})
}

// applyMigrationFile executes a SQL migration file if it exists.
func applyMigrationFile(ctx context.Context, pool *pgxpool.Pool, filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, string(content))
	return err
}

// TestAT019_DatabaseRolePrivilegeEscalationBlocked verifies the least-privilege RBAC matrix
// established by migrations/008_least_privilege_roles.sql:
// 1. consent_api_role CANNOT UPDATE consent_outbox_events (must fail with SQLSTATE 42501).
// 2. consent_worker_role CANNOT INSERT into consents or mutate core tables (must fail with SQLSTATE 42501).
// 3. Neither role can execute DDL (CREATE TABLE) on the schema (must fail with SQLSTATE 42501).
func TestAT019_DatabaseRolePrivilegeEscalationBlocked(t *testing.T) {
	dsn := integrationDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Locate and apply migrations/008_least_privilege_roles.sql
	possiblePaths := []string{
		"../../migrations/008_least_privilege_roles.sql",
		"migrations/008_least_privilege_roles.sql",
		filepath.Join("..", "..", "migrations", "008_least_privilege_roles.sql"),
	}
	var migrationApplied bool
	for _, p := range possiblePaths {
		if err := applyMigrationFile(ctx, pool, p); err == nil {
			migrationApplied = true
			break
		}
	}
	if !migrationApplied {
		t.Log("Note: migration file could not be read directly from disk; assuming migration was already applied by runner")
	}

	t.Run("API_Role_Blocked_From_Updating_Outbox_Events", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)

		// Switch to consent_api_role
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE consent_api_role"); err != nil {
			t.Fatalf("failed to assume consent_api_role: %v", err)
		}

		// Attempting UPDATE on consent_outbox_events MUST fail with SQLSTATE 42501 (permission denied)
		_, updateErr := tx.Exec(ctx, "UPDATE consent_outbox_events SET published_at = now() WHERE id = 'test-unauthorized-id'")
		if updateErr == nil {
			t.Fatal("expected permission denied error when consent_api_role attempts UPDATE on consent_outbox_events, got nil")
		}

		var pgErr *pgconn.PgError
		if !errors.As(updateErr, &pgErr) {
			t.Fatalf("expected PgError, got: %T (%v)", updateErr, updateErr)
		}
		if pgErr.Code != "42501" {
			t.Fatalf("expected SQLSTATE 42501 (insufficient_privilege), got %s (message: %s)", pgErr.Code, pgErr.Message)
		}
		t.Logf("PASS: consent_api_role UPDATE blocked as expected: %s (%s)", pgErr.Code, pgErr.Message)
	})

	t.Run("Worker_Role_Blocked_From_Inserting_Consents", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)

		// Switch to consent_worker_role
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE consent_worker_role"); err != nil {
			t.Fatalf("failed to assume consent_worker_role: %v", err)
		}

		// Attempting INSERT into consents MUST fail with SQLSTATE 42501 (permission denied)
		_, insertErr := tx.Exec(ctx, `
			INSERT INTO consents (id, tenant_id, subject_id, purpose, channel, legal_basis, status, notice_version)
			VALUES ('test-priv-escalate', 'tenant-sec', 'sub-1', 'analytics', 'email', 'consent', 'granted', 'v1.0')
		`)
		if insertErr == nil {
			t.Fatal("expected permission denied error when consent_worker_role attempts INSERT into consents, got nil")
		}

		var pgErr *pgconn.PgError
		if !errors.As(insertErr, &pgErr) {
			t.Fatalf("expected PgError, got: %T (%v)", insertErr, insertErr)
		}
		if pgErr.Code != "42501" {
			t.Fatalf("expected SQLSTATE 42501 (insufficient_privilege), got %s (message: %s)", pgErr.Code, pgErr.Message)
		}
		t.Logf("PASS: consent_worker_role INSERT blocked as expected: %s (%s)", pgErr.Code, pgErr.Message)
	})

	t.Run("Worker_Role_Blocked_From_Updating_Core_Consents", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)

		if _, err := tx.Exec(ctx, "SET LOCAL ROLE consent_worker_role"); err != nil {
			t.Fatalf("failed to assume consent_worker_role: %v", err)
		}

		// Attempting UPDATE on consents MUST fail with SQLSTATE 42501 (permission denied)
		_, updateErr := tx.Exec(ctx, "UPDATE consents SET status = 'withdrawn' WHERE id = 'arbitrary-id'")
		if updateErr == nil {
			t.Fatal("expected permission denied error when consent_worker_role attempts UPDATE on consents, got nil")
		}

		var pgErr *pgconn.PgError
		if !errors.As(updateErr, &pgErr) {
			t.Fatalf("expected PgError, got: %T (%v)", updateErr, updateErr)
		}
		if pgErr.Code != "42501" {
			t.Fatalf("expected SQLSTATE 42501 (insufficient_privilege), got %s (message: %s)", pgErr.Code, pgErr.Message)
		}
		t.Logf("PASS: consent_worker_role UPDATE blocked as expected: %s (%s)", pgErr.Code, pgErr.Message)
	})

	t.Run("Worker_Role_Allowed_To_Read_Consents", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)

		if _, err := tx.Exec(ctx, "SET LOCAL ROLE consent_worker_role"); err != nil {
			t.Fatalf("failed to assume consent_worker_role: %v", err)
		}

		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM consents").Scan(&count); err != nil {
			t.Fatalf("expected consent_worker_role to have SELECT access on consents, got error: %v", err)
		}
		t.Logf("PASS: consent_worker_role SELECT succeeded with count=%d", count)
	})

	t.Run("API_And_Worker_Roles_Blocked_From_DDL_Create_Table", func(t *testing.T) {
		// Test API role DDL restriction
		txAPI, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}
		defer txAPI.Rollback(ctx)

		if _, err := txAPI.Exec(ctx, "SET LOCAL ROLE consent_api_role"); err != nil {
			t.Fatalf("failed to assume consent_api_role: %v", err)
		}

		_, ddlErr := txAPI.Exec(ctx, "CREATE TABLE test_unauthorized_tbl (id text PRIMARY KEY)")
		if ddlErr == nil {
			t.Fatal("expected permission denied on CREATE TABLE by consent_api_role, got nil")
		}
		var pgErr *pgconn.PgError
		if !errors.As(ddlErr, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("expected SQLSTATE 42501 on DDL by consent_api_role, got: %v", ddlErr)
		}
		t.Logf("PASS: consent_api_role DDL CREATE blocked: %s (%s)", pgErr.Code, pgErr.Message)

		// Test Worker role DDL restriction
		txWorker, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}
		defer txWorker.Rollback(ctx)

		if _, err := txWorker.Exec(ctx, "SET LOCAL ROLE consent_worker_role"); err != nil {
			t.Fatalf("failed to assume consent_worker_role: %v", err)
		}

		_, ddlWorkerErr := txWorker.Exec(ctx, "CREATE TABLE test_unauthorized_tbl2 (id text PRIMARY KEY)")
		if ddlWorkerErr == nil {
			t.Fatal("expected permission denied on CREATE TABLE by consent_worker_role, got nil")
		}
		if !errors.As(ddlWorkerErr, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("expected SQLSTATE 42501 on DDL by consent_worker_role, got: %v", ddlWorkerErr)
		}
		t.Logf("PASS: consent_worker_role DDL CREATE blocked: %s (%s)", pgErr.Code, pgErr.Message)
	})
}

// TestAT029_PostgresAuditImmutabilityTriggerAndRoleRBAC verifies T-029:
// 1. Direct UPDATE or DELETE on consent_audit_events is blocked by trg_consent_audit_immutable.
// 2. consent_api_role is restricted from mutating existing audit rows.
// 3. consent_worker_role is restricted from mutating or inserting audit rows.
func TestAT029_PostgresAuditImmutabilityTriggerAndRoleRBAC(t *testing.T) {
	dsn := integrationDSN(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	testEventID := "aud-sec-" + runID(t)
	testTenantID := "tenant-sec-" + runID(t)

	// Ensure the migration has been applied
	_ = applyMigrationFile(ctx, pool, filepath.Join("..", "..", "migrations", "008_least_privilege_roles.sql"))
	_ = applyMigrationFile(ctx, pool, filepath.Join("..", "..", "migrations", "009_audit_history_enhancement.sql"))

	// Insert an initial audit event
	_, err = pool.Exec(ctx, `
		INSERT INTO consent_audit_events (
			id, tenant_id, actor_id, action, consent_id, resource_type, resource_id,
			revision, before_state, after_state, correlation_id, reason, occurred_at
		) VALUES (
			$1, $2, 'actor-admin', 'created', 'con-init', 'suppression', 'test-subj:global:*',
			1, '{}'::jsonb, '{"active":true}'::jsonb, 'corr-init', 'initial audit', now()
		)
	`, testEventID, testTenantID)
	if err != nil {
		t.Fatalf("failed to seed test audit event: %v", err)
	}

	t.Run("Trigger_Blocks_Direct_UPDATE", func(t *testing.T) {
		_, updateErr := pool.Exec(ctx, "UPDATE consent_audit_events SET reason = 'tampered' WHERE id = $1", testEventID)
		if updateErr == nil {
			t.Fatal("expected immutability trigger to block UPDATE on consent_audit_events, got nil")
		}
		if !strings.Contains(updateErr.Error(), "append-only") && !strings.Contains(updateErr.Error(), "reject_consent_audit_mutation") && !strings.Contains(updateErr.Error(), "immutable") {
			t.Fatalf("unexpected error message on UPDATE: %v", updateErr)
		}
		t.Logf("PASS: immutability trigger successfully rejected UPDATE: %v", updateErr)
	})

	t.Run("Trigger_Blocks_Direct_DELETE", func(t *testing.T) {
		_, deleteErr := pool.Exec(ctx, "DELETE FROM consent_audit_events WHERE id = $1", testEventID)
		if deleteErr == nil {
			t.Fatal("expected immutability trigger to block DELETE on consent_audit_events, got nil")
		}
		if !strings.Contains(deleteErr.Error(), "append-only") && !strings.Contains(deleteErr.Error(), "reject_consent_audit_mutation") && !strings.Contains(deleteErr.Error(), "immutable") {
			t.Fatalf("unexpected error message on DELETE: %v", deleteErr)
		}
		t.Logf("PASS: immutability trigger successfully rejected DELETE: %v", deleteErr)
	})

	t.Run("API_Role_Blocked_From_Mutating_Audit_Events", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)

		if _, err := tx.Exec(ctx, "SET LOCAL ROLE consent_api_role"); err != nil {
			t.Fatalf("failed to assume consent_api_role: %v", err)
		}

		_, updateErr := tx.Exec(ctx, "UPDATE consent_audit_events SET reason = 'tampered' WHERE id = $1", testEventID)
		if updateErr == nil {
			t.Fatal("expected error when consent_api_role attempts UPDATE on consent_audit_events, got nil")
		}
		t.Logf("PASS: consent_api_role UPDATE blocked: %v", updateErr)
	})

	t.Run("Worker_Role_Blocked_From_Inserting_Audit_Events", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)

		if _, err := tx.Exec(ctx, "SET LOCAL ROLE consent_worker_role"); err != nil {
			t.Fatalf("failed to assume consent_worker_role: %v", err)
		}

		_, insertErr := tx.Exec(ctx, `
			INSERT INTO consent_audit_events (id, tenant_id, actor_id, action, consent_id, resource_type, resource_id, revision)
			VALUES ('worker-test-aud', $1, 'worker', 'created', 'con-1', 'consent', 'res-1', 1)
		`, testTenantID)
		if insertErr == nil {
			t.Fatal("expected permission denied when consent_worker_role attempts INSERT into consent_audit_events, got nil")
		}
		var pgErr *pgconn.PgError
		if errors.As(insertErr, &pgErr) && pgErr.Code != "42501" {
			t.Fatalf("expected SQLSTATE 42501, got %s: %v", pgErr.Code, insertErr)
		}
		t.Logf("PASS: consent_worker_role INSERT blocked: %v", insertErr)
	})
}
