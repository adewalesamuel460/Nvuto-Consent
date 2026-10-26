package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// transientPG reports YSQL serialization/deadlock aborts that callers are
// expected to retry under snapshot isolation.
func transientPG(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && (pe.Code == "40001" || pe.Code == "40P01")
}

func retryTransient[T any](t *testing.T, op func() (T, error)) (T, error) {
	t.Helper()
	for attempt := 0; ; attempt++ {
		value, err := op()
		if err == nil || !transientPG(err) || attempt >= 20 {
			return value, err
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
}

// assertStoredReceiptVerifies checks the T-004 hash after a real YSQL read and
// after a JSON export/import of the read value.
func assertStoredReceiptVerifies(t *testing.T, svc *service.Service, tenantID, receiptID string) {
	t.Helper()
	receipt, err := svc.Receipt(context.Background(), tenantID, receiptID)
	if err != nil {
		t.Fatal(err)
	}
	if v := service.VerifyReceipt(receipt); v.Status != service.ReceiptHashVerified || receipt.HashVersion != service.ReceiptHashVersionCanonical {
		t.Fatalf("receipt %s must verify after a YSQL read: %+v (issued_at=%s)", receiptID, v, receipt.IssuedAt.Format(time.RFC3339Nano))
	}
	exported, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var imported domain.ConsentReceipt
	if err := json.Unmarshal(exported, &imported); err != nil {
		t.Fatal(err)
	}
	if v := service.VerifyReceipt(imported); v.Status != service.ReceiptHashVerified {
		t.Fatalf("receipt %s must verify after JSON export/import: %+v", receiptID, v)
	}
}

func TestPostgresPolicyTransactionsAndSingleUseConfirmation(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	repo, err := store.NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	svc := service.New(repo)
	tenantID := fmt.Sprintf("tenant-pg-%d", time.Now().UnixNano())
	now := time.Now().UTC()

	if _, err := svc.RegisterPurpose(context.Background(), tenantID, "admin", domain.Purpose{Code: "marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterPurpose(context.Background(), tenantID, "admin", domain.Purpose{Code: "double_marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, RequiresDoubleOptIn: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterLawfulBasis(context.Background(), tenantID, "admin", domain.LawfulBasis{Code: "consent", AllowedCategories: []string{"marketing"}, RequiresEvidence: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishNotice(context.Background(), tenantID, "admin", domain.PrivacyNotice{Version: "2026-08", Jurisdiction: "NG", PublishedAt: now, Active: true}); err != nil {
		t.Fatal(err)
	}
	evidence := domain.Evidence{Method: "integration", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: now}
	consent, err := svc.RecordGoverned(context.Background(), tenantID, "admin", "cus-1", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := svc.Receipt(context.Background(), tenantID, consent.ReceiptID)
	if err != nil || receipt.ConsentID != consent.ID || receipt.ReceiptHash == "" {
		t.Fatalf("durable receipt mismatch: receipt=%+v err=%v", receipt, err)
	}
	if _, err := svc.SetSuppression(context.Background(), tenantID, "admin", domain.Suppression{SubjectID: "cus-1", Scope: domain.SuppressionPurposeChannel, Purpose: "marketing", Channel: "email", Reason: "integration", Active: true}); err != nil {
		t.Fatal(err)
	}
	decision, err := svc.Evaluate(context.Background(), tenantID, "cus-1", "marketing", "email", time.Time{})
	if err != nil || decision.Reason != "purpose_channel_suppression" {
		t.Fatalf("durable suppression mismatch: decision=%+v err=%v", decision, err)
	}

	challenge, err := svc.StartDoubleOptIn(context.Background(), tenantID, "admin", "cus-2", "double_marketing", "email", "portal", "NG", "2026-08", evidence, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := svc.ConfirmDoubleOptIn(context.Background(), tenantID, "admin", service.TenantWide(), challenge.ConfirmationToken); err == nil {
				successes.Add(1)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatalf("confirmation token succeeded %d times, want exactly one", successes.Load())
	}
	decision, err = svc.Evaluate(context.Background(), tenantID, "cus-2", "double_marketing", "email", time.Time{})
	if err != nil || !decision.Allowed || decision.ReceiptID == "" {
		t.Fatalf("confirmed consent unavailable: decision=%+v err=%v", decision, err)
	}

	// T-004: governed and double-opt-in receipts verify after a real read.
	assertStoredReceiptVerifies(t, svc, tenantID, consent.ReceiptID)
	assertStoredReceiptVerifies(t, svc, tenantID, decision.ReceiptID)

	// T-001: challenge -> later denial -> confirming the older challenge fails.
	stale, err := svc.StartDoubleOptIn(context.Background(), tenantID, "admin", "cus-3", "double_marketing", "email", "portal", "NG", "2026-08", evidence, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordGoverned(context.Background(), tenantID, "admin", "cus-3", "double_marketing", "email", "consent", "denied", "portal", nil, nil, "NG", "2026-08", evidence); err != nil {
		t.Fatal(err)
	}
	if confirmed, err := svc.ConfirmDoubleOptIn(context.Background(), tenantID, "admin", service.TenantWide(), stale.ConfirmationToken); err == nil {
		t.Fatalf("stale challenge overrode a later denial: %+v", confirmed)
	}
	if decision, err := svc.Evaluate(context.Background(), tenantID, "cus-3", "double_marketing", "email", time.Time{}); err != nil || decision.Allowed || decision.Reason != string(domain.StatusDenied) {
		t.Fatalf("later denial must remain effective: decision=%+v err=%v", decision, err)
	}

	// T-001: challenge1 -> challenge2 -> confirm2 -> withdraw -> confirm1 fails.
	older, err := svc.StartDoubleOptIn(context.Background(), tenantID, "admin", "cus-4", "double_marketing", "email", "portal", "NG", "2026-08", evidence, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	newer, err := svc.StartDoubleOptIn(context.Background(), tenantID, "admin", "cus-4", "double_marketing", "email", "portal", "NG", "2026-08", evidence, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	granted, err := svc.ConfirmDoubleOptIn(context.Background(), tenantID, "admin", service.TenantWide(), newer.ConfirmationToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Withdraw(context.Background(), tenantID, "admin", granted.ID); err != nil {
		t.Fatal(err)
	}
	if confirmed, err := svc.ConfirmDoubleOptIn(context.Background(), tenantID, "admin", service.TenantWide(), older.ConfirmationToken); err == nil {
		t.Fatalf("superseded challenge restored a withdrawn consent: %+v", confirmed)
	}
	if decision, err := svc.Evaluate(context.Background(), tenantID, "cus-4", "double_marketing", "email", time.Time{}); err != nil || decision.Allowed || decision.Reason != string(domain.StatusWithdrawn) {
		t.Fatalf("withdrawal must remain effective: decision=%+v err=%v", decision, err)
	}

	// T-001: concurrent confirm and later denial always converge to denied.
	for i := range 5 {
		subject := fmt.Sprintf("cus-race-%d", i)
		racing, err := svc.StartDoubleOptIn(context.Background(), tenantID, "admin", subject, "double_marketing", "email", "portal", "NG", "2026-08", evidence, now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		var confirmed domain.Consent
		var confirmErr, denyErr error
		var race sync.WaitGroup
		race.Add(2)
		go func() {
			defer race.Done()
			confirmed, confirmErr = retryTransient(t, func() (domain.Consent, error) {
				return svc.ConfirmDoubleOptIn(context.Background(), tenantID, "admin", service.TenantWide(), racing.ConfirmationToken)
			})
		}()
		go func() {
			defer race.Done()
			_, denyErr = retryTransient(t, func() (domain.Consent, error) {
				return svc.RecordGoverned(context.Background(), tenantID, "admin", subject, "double_marketing", "email", "consent", "denied", "portal", nil, nil, "NG", "2026-08", evidence)
			})
		}()
		race.Wait()
		if denyErr != nil {
			t.Fatalf("%s: denial failed: %v", subject, denyErr)
		}
		if confirmErr == nil {
			assertStoredReceiptVerifies(t, svc, tenantID, confirmed.ReceiptID)
		} else if transientPG(confirmErr) {
			t.Fatalf("%s: confirmation kept aborting: %v", subject, confirmErr)
		}
		if decision, err := svc.Evaluate(context.Background(), tenantID, subject, "double_marketing", "email", time.Time{}); err != nil || decision.Allowed || decision.Reason != string(domain.StatusDenied) {
			t.Fatalf("%s: denial must win regardless of ordering: decision=%+v err=%v", subject, decision, err)
		}
	}
}

// TestPostgresLegacyUpgradeEvaluation (AT-008) verifies against PostgreSQL/YugabyteDB
// that upgraded legacy records with blank notice_version are properly handled:
//  1. Safe legacy service/contractual grants evaluate to allowed=true, reason=granted.
//  2. Unsafe legacy affirmative/marketing grants evaluate to allowed=false, reason=policy_requirement_unmet.
//  3. Terminal states (withdrawn) evaluate to allowed=false, reason=withdrawn.
//  4. view_consent_legacy_classification correctly categorizes legacy rows.
//  5. No records or evidence are deleted or mutated.
func TestPostgresLegacyUpgradeEvaluation(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		if os.Getenv("NVUTO_REQUIRE_INTEGRATION") == "true" {
			t.Fatal("TEST_DATABASE_URL (or DATABASE_URL) is required when NVUTO_REQUIRE_INTEGRATION=true")
		}
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	ctx := context.Background()
	repo, err := store.NewPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	svc := service.New(repo)
	tenantID := fmt.Sprintf("tenant-legacy-pg-%d", time.Now().UnixNano())
	now := time.Now().UTC()

	// 1. Configure purpose registries
	_, err = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "service_updates",
		Category:                   "service",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: false,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to register service purpose: %v", err)
	}
	_, err = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to register marketing purpose: %v", err)
	}

	// 2. Configure lawful bases
	_, err = svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "contract",
		AllowedCategories: []string{"service"},
		RequiresEvidence:  false,
		Active:            true,
	})
	if err != nil {
		t.Fatalf("failed to register contract basis: %v", err)
	}
	_, err = svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "consent",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	})
	if err != nil {
		t.Fatalf("failed to register consent basis: %v", err)
	}

	// 3. Insert synthetic pre-002 legacy records into PostgreSQL (notice_version='')
	safeID := fmt.Sprintf("con-safe-%d", time.Now().UnixNano())
	unsafeID := fmt.Sprintf("con-unsafe-%d", time.Now().UnixNano())
	withdrawnID := fmt.Sprintf("con-withdrawn-%d", time.Now().UnixNano())

	// Safe legacy: service_updates / contract / status=granted / notice_version=''
	_, err = repo.Pool().Exec(ctx, `
		INSERT INTO consents(id, tenant_id, subject_id, purpose, channel, legal_basis, status, source, notice_version, created_at, updated_at)
		VALUES($1, $2, 'cust-safe-pg', 'service_updates', 'email', 'contract', 'granted', 'legacy', '', $3, $3)
	`, safeID, tenantID, now.Add(-48*time.Hour))
	if err != nil {
		t.Fatalf("failed to insert safe legacy record: %v", err)
	}

	// Unsafe legacy: marketing / consent / status=granted / notice_version='' / empty evidence
	_, err = repo.Pool().Exec(ctx, `
		INSERT INTO consents(id, tenant_id, subject_id, purpose, channel, legal_basis, status, source, notice_version, evidence, created_at, updated_at)
		VALUES($1, $2, 'cust-unsafe-pg', 'marketing', 'email', 'consent', 'granted', 'legacy', '', '{}'::jsonb, $3, $3)
	`, unsafeID, tenantID, now.Add(-48*time.Hour))
	if err != nil {
		t.Fatalf("failed to insert unsafe legacy record: %v", err)
	}

	// Terminal legacy: service_updates / contract / status=withdrawn / notice_version=''
	_, err = repo.Pool().Exec(ctx, `
		INSERT INTO consents(id, tenant_id, subject_id, purpose, channel, legal_basis, status, source, notice_version, created_at, updated_at, withdrawn_at)
		VALUES($1, $2, 'cust-withdrawn-pg', 'service_updates', 'email', 'contract', 'withdrawn', 'legacy', '', $3, $3, $3)
	`, withdrawnID, tenantID, now.Add(-48*time.Hour))
	if err != nil {
		t.Fatalf("failed to insert withdrawn legacy record: %v", err)
	}

	// 4. Evaluate safe legacy record: must be allowed=true, reason=granted
	safeDec, err := svc.Evaluate(ctx, tenantID, "cust-safe-pg", "service_updates", "email", now)
	if err != nil {
		t.Fatalf("Evaluate safe legacy returned error: %v", err)
	}
	if !safeDec.Allowed || safeDec.Reason != domain.DecisionReasonGranted {
		t.Fatalf("expected safe legacy grant allowed=true, reason=granted; got %+v", safeDec)
	}

	// 5. Evaluate unsafe legacy record: must be allowed=false, reason=policy_requirement_unmet
	unsafeDec, err := svc.Evaluate(ctx, tenantID, "cust-unsafe-pg", "marketing", "email", now)
	if err != nil {
		t.Fatalf("Evaluate unsafe legacy returned error: %v", err)
	}
	if unsafeDec.Allowed || unsafeDec.Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("expected unsafe legacy grant allowed=false, reason=policy_requirement_unmet; got %+v", unsafeDec)
	}

	// 6. Evaluate withdrawn legacy record: must be allowed=false, reason=withdrawn
	withdrawnDec, err := svc.Evaluate(ctx, tenantID, "cust-withdrawn-pg", "service_updates", "email", now)
	if err != nil {
		t.Fatalf("Evaluate withdrawn legacy returned error: %v", err)
	}
	if withdrawnDec.Allowed || withdrawnDec.Reason != "withdrawn" {
		t.Fatalf("expected withdrawn legacy grant allowed=false, reason=withdrawn; got %+v", withdrawnDec)
	}

	// 7. Verify view_consent_legacy_classification view in DB
	rows, err := repo.Pool().Query(ctx, `
		SELECT consent_id, classification FROM view_consent_legacy_classification
		WHERE tenant_id = $1 ORDER BY consent_id
	`, tenantID)
	if err != nil {
		t.Fatalf("querying view_consent_legacy_classification failed: %v", err)
	}
	defer rows.Close()

	classifications := make(map[string]string)
	for rows.Next() {
		var cid, class string
		if err := rows.Scan(&cid, &class); err != nil {
			t.Fatal(err)
		}
		classifications[cid] = class
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if classifications[safeID] != "CLASS_1_SERVICE_CONTRACT_ELIGIBLE" {
		t.Errorf("expected safeID classification CLASS_1_SERVICE_CONTRACT_ELIGIBLE, got %s", classifications[safeID])
	}
	if classifications[unsafeID] != "CLASS_2_MARKETING_AFFIRMATIVE_QUARANTINED" {
		t.Errorf("expected unsafeID classification CLASS_2_MARKETING_AFFIRMATIVE_QUARANTINED, got %s", classifications[unsafeID])
	}
	if classifications[withdrawnID] != "CLASS_5_TERMINAL_NON_GRANTED" {
		t.Errorf("expected withdrawnID classification CLASS_5_TERMINAL_NON_GRANTED, got %s", classifications[withdrawnID])
	}

	// 8. Record audit reconciliation in consent_legacy_reconciliation
	_, err = repo.Pool().Exec(ctx, `
		INSERT INTO consent_legacy_reconciliation(tenant_id, class_code, category, eligibility_decision, rationale, record_count, approved_by)
		VALUES($1, 'CLASS_1', 'service', 'eligible', 'Grandfathered contractual service notifications under D-04', 1, 'ci-audit')
		ON CONFLICT (tenant_id, class_code) DO NOTHING
	`, tenantID)
	if err != nil {
		t.Fatalf("failed to insert reconciliation record: %v", err)
	}

	// 9. Verify zero evidence loss / row count unchanged
	var count int
	err = repo.Pool().QueryRow(ctx, "SELECT count(*) FROM consents WHERE tenant_id = $1", tenantID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected exactly 3 records in consents (zero deletion), got %d", count)
	}
}

// TestIntegration_PostgresAuthenticatedHTTPWorkflows proves end-to-end HTTP-to-YSQL
// workflows against the real PostgreSQL / YugabyteDB backend (T-016 / AT-016).
// It tests authenticated HTTP requests (POST /v1/consents, GET /v1/consents, POST /v1/evaluate,
// GET /v1/receipts, POST /v1/consents/{id}/withdraw) using valid HMAC JWTs, asserting status codes,
// canonical receipt hashes, and database states.
func TestIntegration_PostgresAuthenticatedHTTPWorkflows(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatal("TEST_DATABASE_URL is required when NVUTO_REQUIRE_INTEGRATION=true")
		}
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	repo, err := store.NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	svc := service.New(repo)
	tenantID := fmt.Sprintf("tenant-http-%d", time.Now().UnixNano())
	now := time.Now().UTC()

	// 1. Configure HMAC authentication
	hmacSecret := "test-secret-at016-e2e-workflows-32b!"
	t.Setenv("JWT_JWKS_URL", "")
	t.Setenv("JWT_HMAC_SECRET", hmacSecret)
	t.Setenv("ALLOW_HMAC_AUTH", "true")
	t.Setenv("ALLOW_DEV_HEADERS", "false")

	// Stand up actual HTTP server listening on a loopback TCP socket
	ts := httptest.NewServer(newMux(svc))
	defer ts.Close()
	client := ts.Client()

	signToken := func(actor, customerSubject string, roles ...string) string {
		claims := auth.Claims{
			TenantID:  tenantID,
			SubjectID: customerSubject,
			Roles:     roles,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "actor-" + actor,
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			},
		}
		raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(hmacSecret))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	adminToken := signToken("admin-1", "", auth.RolePrivacyAdmin)
	writerToken := signToken("writer-1", "", auth.RoleConsentWriter)
	readerToken := signToken("reader-1", "", auth.RoleConsentReader)

	doHTTP := func(method, path, token string, body any) (*http.Response, []byte) {
		var r io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			r = bytes.NewReader(encoded)
		}
		req, err := http.NewRequest(method, ts.URL+path, r)
		if err != nil {
			t.Fatal(err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp, respBytes
	}

	// 2. Administrative Setup: register purpose, lawful basis, notice via HTTP
	resp, _ := doHTTP(http.MethodPost, "/v1/purposes", adminToken, map[string]any{
		"code":                         "marketing",
		"category":                     "marketing",
		"allowed_channels":             []string{"email"},
		"requires_affirmative_consent": true,
		"requires_double_opt_in":       false,
		"active":                       true,
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/purposes failed with status %d", resp.StatusCode)
	}

	resp, _ = doHTTP(http.MethodPost, "/v1/lawful-bases", adminToken, map[string]any{
		"code":               "consent",
		"allowed_categories": []string{"marketing"},
		"requires_evidence":  true,
		"active":             true,
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/lawful-bases failed with status %d", resp.StatusCode)
	}

	resp, _ = doHTTP(http.MethodPost, "/v1/notices", adminToken, map[string]any{
		"version":      "2026-08",
		"jurisdiction": "NG",
		"published_at": now.Format(time.RFC3339Nano),
		"active":       true,
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/notices failed with status %d", resp.StatusCode)
	}

	// 3. Create Consent via POST /v1/consents
	subjectID := "cus-http-workflow-1"
	createPayload := map[string]any{
		"subject_id":     subjectID,
		"purpose":        "marketing",
		"channel":        "email",
		"legal_basis":    "consent",
		"status":         "granted",
		"source":         "portal",
		"jurisdiction":   "NG",
		"notice_version": "2026-08",
		"evidence": map[string]any{
			"method":        "preference_center",
			"artifact_hash": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
			"collected_at":  now.Format(time.RFC3339Nano),
		},
	}
	resp, bodyBytes := doHTTP(http.MethodPost, "/v1/consents", writerToken, createPayload)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /v1/consents failed: %d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var createdConsent domain.Consent
	if err := json.Unmarshal(bodyBytes, &createdConsent); err != nil {
		t.Fatalf("unmarshaling created consent failed: %v", err)
	}
	if createdConsent.ID == "" || createdConsent.ReceiptID == "" || createdConsent.Status != domain.StatusGranted {
		t.Fatalf("invalid created consent payload: %+v", createdConsent)
	}

	// 4. List Consents via GET /v1/consents?subject_id=...
	resp, bodyBytes = doHTTP(http.MethodGet, "/v1/consents?subject_id="+subjectID, readerToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/consents failed: %d body=%s", resp.StatusCode, string(bodyBytes))
	}
	var listResp struct {
		Items []domain.Consent `json:"items"`
	}
	if err := json.Unmarshal(bodyBytes, &listResp); err != nil {
		t.Fatalf("unmarshaling consent list failed: %v", err)
	}
	if len(listResp.Items) != 1 || listResp.Items[0].ID != createdConsent.ID {
		t.Fatalf("GET /v1/consents did not return created consent: %+v", listResp.Items)
	}

	// 5. Evaluate Consent via POST /v1/evaluate
	evalPayload := map[string]any{
		"subject_id": subjectID,
		"purpose":    "marketing",
		"channel":    "email",
	}
	resp, bodyBytes = doHTTP(http.MethodPost, "/v1/evaluate", readerToken, evalPayload)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/evaluate failed: %d body=%s", resp.StatusCode, string(bodyBytes))
	}
	var decision struct {
		Allowed   bool   `json:"allowed"`
		Reason    string `json:"reason"`
		ReceiptID string `json:"receipt_id"`
	}
	if err := json.Unmarshal(bodyBytes, &decision); err != nil {
		t.Fatalf("unmarshaling evaluate decision failed: %v", err)
	}
	if !decision.Allowed || decision.Reason != "granted" || decision.ReceiptID != createdConsent.ReceiptID {
		t.Fatalf("unexpected evaluate decision: %+v", decision)
	}

	// 6. Fetch Receipt via GET /v1/receipts/{receipt_id} and assert cryptographic verification
	resp, bodyBytes = doHTTP(http.MethodGet, "/v1/receipts/"+createdConsent.ReceiptID, readerToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/receipts/%s failed: %d body=%s", createdConsent.ReceiptID, resp.StatusCode, string(bodyBytes))
	}
	var receipt domain.ConsentReceipt
	if err := json.Unmarshal(bodyBytes, &receipt); err != nil {
		t.Fatalf("unmarshaling receipt failed: %v", err)
	}
	if receipt.ConsentID != createdConsent.ID || receipt.ReceiptHash == "" {
		t.Fatalf("invalid receipt returned: %+v", receipt)
	}
	verification := service.VerifyReceipt(receipt)
	if verification.Status != service.ReceiptHashVerified || receipt.HashVersion != service.ReceiptHashVersionCanonical {
		t.Fatalf("receipt failed canonical cryptographic verification: %+v", verification)
	}

	// 7. Verify Database State in YSQL directly
	ctx := context.Background()
	var dbStatus string
	var dbGen int64
	err = repo.Pool().QueryRow(ctx, "SELECT status, preference_generation FROM consents WHERE tenant_id=$1 AND id=$2", tenantID, createdConsent.ID).Scan(&dbStatus, &dbGen)
	if err != nil {
		t.Fatalf("database query for consent failed: %v", err)
	}
	if dbStatus != string(domain.StatusGranted) || dbGen < 1 {
		t.Fatalf("database row state mismatch: status=%s generation=%d", dbStatus, dbGen)
	}

	var rcpCount, audCount, outCount int
	if err := repo.Pool().QueryRow(ctx, "SELECT count(*) FROM consent_receipts WHERE tenant_id=$1 AND consent_id=$2", tenantID, createdConsent.ID).Scan(&rcpCount); err != nil || rcpCount != 1 {
		t.Fatalf("expected 1 receipt row in DB, got %d (err: %v)", rcpCount, err)
	}
	if err := repo.Pool().QueryRow(ctx, "SELECT count(*) FROM consent_audit_events WHERE tenant_id=$1 AND consent_id=$2", tenantID, createdConsent.ID).Scan(&audCount); err != nil || audCount < 1 {
		t.Fatalf("expected at least 1 audit event in DB, got %d (err: %v)", audCount, err)
	}
	if err := repo.Pool().QueryRow(ctx, "SELECT count(*) FROM consent_outbox_events WHERE tenant_id=$1 AND aggregate_id=$2", tenantID, createdConsent.ID).Scan(&outCount); err != nil || outCount < 1 {
		t.Fatalf("expected at least 1 outbox event in DB, got %d (err: %v)", outCount, err)
	}

	// 8. Withdraw Consent via POST /v1/consents/{id}/withdraw
	resp, bodyBytes = doHTTP(http.MethodPost, "/v1/consents/"+createdConsent.ID+"/withdraw", writerToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/consents/%s/withdraw failed: %d body=%s", createdConsent.ID, resp.StatusCode, string(bodyBytes))
	}
	var withdrawnConsent domain.Consent
	if err := json.Unmarshal(bodyBytes, &withdrawnConsent); err != nil {
		t.Fatalf("unmarshaling withdrawn consent failed: %v", err)
	}
	if withdrawnConsent.Status != domain.StatusWithdrawn {
		t.Fatalf("expected status withdrawn, got %s", withdrawnConsent.Status)
	}

	// 9. Re-evaluate via HTTP: must now be rejected with reason=withdrawn
	resp, bodyBytes = doHTTP(http.MethodPost, "/v1/evaluate", readerToken, evalPayload)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/evaluate after withdraw failed: %d body=%s", resp.StatusCode, string(bodyBytes))
	}
	var afterDecision struct {
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(bodyBytes, &afterDecision); err != nil {
		t.Fatalf("unmarshaling evaluate decision failed: %v", err)
	}
	if afterDecision.Allowed || afterDecision.Reason != "withdrawn" {
		t.Fatalf("expected evaluate after withdrawal to be allowed=false, reason=withdrawn; got %+v", afterDecision)
	}
}

func TestIntegration_PostgresPolicyTransactionsAndSingleUseConfirmation(t *testing.T) {
	TestPostgresPolicyTransactionsAndSingleUseConfirmation(t)
}

func TestIntegration_PostgresLegacyUpgradeEvaluation(t *testing.T) {
	TestPostgresLegacyUpgradeEvaluation(t)
}
