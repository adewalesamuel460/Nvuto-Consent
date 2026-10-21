package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

const contractSecret = "contract-test-secret-long-enough-for-hs256"

func makeContractToken(tenant, customer string, roles ...string) string {
	claims := auth.Claims{
		TenantID:  tenant,
		SubjectID: customer,
		Roles:     roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-" + customer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(contractSecret))
	if err != nil {
		panic(err)
	}
	return raw
}

func newTestAPI(svc *service.Service, logBuf *bytes.Buffer) http.Handler {
	logger := slog.New(slog.NewJSONHandler(logBuf, nil))
	return newHandler(svc, 5*time.Second, logger)
}

// Test (3): Trailing JSON and malformed input rejection (T-027).
func TestTrailingJSONAndMalformedInputRejection(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	repo := store.NewMemory()
	svc := service.New(repo)
	var logBuf bytes.Buffer
	handler := newTestAPI(svc, &logBuf)

	validArtifactHash := "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

	cases := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "trailing JSON object",
			path:       "/v1/consents",
			body:       `{"subject_id":"cus-1","purpose":"marketing","channel":"email","legal_basis":"consent","status":"granted","source":"portal","jurisdiction":"NG","notice_version":"2026-08","evidence":{"method":"ci","artifact_hash":"` + validArtifactHash + `","collected_at":"2026-08-14T00:00:00Z"}} {"trailing":"object"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "trailing_data",
		},
		{
			name:       "trailing closing brace",
			path:       "/v1/consents",
			body:       `{"subject_id":"cus-1","purpose":"marketing","channel":"email","legal_basis":"consent","status":"granted","source":"portal","jurisdiction":"NG","notice_version":"2026-08","evidence":{"method":"ci","artifact_hash":"` + validArtifactHash + `","collected_at":"2026-08-14T00:00:00Z"}}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "trailing_data",
		},
		{
			name:       "trailing literal null",
			path:       "/v1/consents",
			body:       `{"subject_id":"cus-1","purpose":"marketing","channel":"email","legal_basis":"consent","status":"granted","source":"portal","jurisdiction":"NG","notice_version":"2026-08","evidence":{"method":"ci","artifact_hash":"` + validArtifactHash + `","collected_at":"2026-08-14T00:00:00Z"}} null`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "trailing_data",
		},
		{
			name:       "duplicate key",
			path:       "/v1/purposes",
			body:       `{"code":"marketing","category":"marketing","allowed_channels":["email"],"requires_affirmative_consent":true,"requires_double_opt_in":false,"active":true,"code":"duplicate"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "duplicate_field",
		},
		{
			name:       "case-variant duplicate key",
			path:       "/v1/purposes",
			body:       `{"code":"marketing","category":"marketing","allowed_channels":["email"],"requires_affirmative_consent":true,"requires_double_opt_in":false,"active":true,"Active":false}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "duplicate_field",
		},
		{
			name:       "top-level null body",
			path:       "/v1/purposes",
			body:       `null`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_json",
		},
		{
			name:       "top-level array body",
			path:       "/v1/purposes",
			body:       `[{"code":"marketing"}]`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_json",
		},
		{
			name:       "omitted required boolean requires_affirmative_consent",
			path:       "/v1/purposes",
			body:       `{"code":"marketing","category":"marketing","allowed_channels":["email"],"requires_double_opt_in":false}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "required_field",
		},
		{
			name:       "omitted required boolean requires_double_opt_in",
			path:       "/v1/purposes",
			body:       `{"code":"marketing","category":"marketing","allowed_channels":["email"],"requires_affirmative_consent":true,"active":true}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "required_field",
		},
		{
			name:       "unknown field disallowed",
			path:       "/v1/purposes",
			body:       `{"code":"marketing","category":"marketing","allowed_channels":["email"],"requires_affirmative_consent":true,"requires_double_opt_in":false,"active":true,"unrecognized":123}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "unknown_field",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			auditBefore := len(repo.Audit())
			outboxBefore := len(repo.Outbox())

			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
			req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status got %d, want %d: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			var body errorBody
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to decode error body: %v (raw: %s)", err, w.Body.String())
			}
			if body.Error != tc.wantCode {
				t.Errorf("error code got %q, want %q", body.Error, tc.wantCode)
			}
			if body.CorrelationID == "" {
				t.Errorf("expected correlation_id in response body")
			}
			if w.Header().Get(correlationHeader) != body.CorrelationID {
				t.Errorf("header %s (%s) does not match body correlation_id (%s)",
					correlationHeader, w.Header().Get(correlationHeader), body.CorrelationID)
			}

			if len(repo.Audit()) != auditBefore || len(repo.Outbox()) != outboxBefore {
				t.Errorf("malformed request caused state mutation: audit %d->%d, outbox %d->%d",
					auditBefore, len(repo.Audit()), outboxBefore, len(repo.Outbox()))
			}
		})
	}
}

// Test (1): Client disconnection / timeout (T-012).
func TestClientDisconnectionCancelsWork(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	repo := store.NewMemory()
	svc := service.New(repo)
	var logBuf bytes.Buffer
	handler := newTestAPI(svc, &logBuf)

	validArtifactHash := "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	body := `{"subject_id":"cus-1","purpose":"marketing","channel":"email","legal_basis":"consent","status":"granted","source":"portal","jurisdiction":"NG","notice_version":"2026-08","evidence":{"method":"ci","artifact_hash":"` + validArtifactHash + `","collected_at":"2026-08-14T00:00:00Z"}}`

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already disconnected

	req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
	req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Since client was cancelled, handler records 499 (Client Closed Request)
	if w.Code != statusClientClosedRequest {
		t.Fatalf("expected status %d for cancelled request, got %d: %s", statusClientClosedRequest, w.Code, w.Body.String())
	}

	// Verify no mutations happened
	if len(repo.Audit()) != 0 || len(repo.Outbox()) != 0 {
		t.Fatalf("cancelled request mutated store: audit=%d, outbox=%d", len(repo.Audit()), len(repo.Outbox()))
	}
}

// Fault injecting repository wrapper to simulate DB outages and errors.
type faultInjectingRepo struct {
	store.Repository
	recordGovernedErr error
	pingErr           error
}

func (f *faultInjectingRepo) RecordGoverned(ctx context.Context, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	if f.recordGovernedErr != nil {
		return domain.Consent{}, f.recordGovernedErr
	}
	return f.Repository.RecordGoverned(ctx, consent, receipt, audit, outbox)
}

func (f *faultInjectingRepo) Ping(ctx context.Context) error {
	if f.pingErr != nil {
		return f.pingErr
	}
	return f.Repository.Ping(ctx)
}

// Test (2): Injected database timeout/outage verifying safe error handling (T-013).
func TestInjectedDatabaseTimeoutAndSafeErrorHandling(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	baseRepo := store.NewMemory()

	// Seed policy registries so admission passes and the DB write is reached
	publishedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	_, _ = baseRepo.PutPurpose(context.Background(), domain.Purpose{TenantID: "tenant-test", Code: "marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, Active: true}, domain.AuditEvent{}, domain.OutboxEvent{})
	_, _ = baseRepo.PutLawfulBasis(context.Background(), domain.LawfulBasis{TenantID: "tenant-test", Code: "consent", AllowedCategories: []string{"marketing"}, RequiresEvidence: true, Active: true}, domain.AuditEvent{}, domain.OutboxEvent{})
	_, _ = baseRepo.PutNotice(context.Background(), domain.PrivacyNotice{TenantID: "tenant-test", Version: "2026-08", Jurisdiction: "NG", PublishedAt: publishedAt, Active: true}, domain.AuditEvent{}, domain.OutboxEvent{})

	validArtifactHash := "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	reqBody := `{"subject_id":"cus-1","purpose":"marketing","channel":"email","legal_basis":"consent","status":"granted","source":"portal","jurisdiction":"NG","notice_version":"2026-08","evidence":{"method":"ci","artifact_hash":"` + validArtifactHash + `","collected_at":"` + publishedAt.Format(time.RFC3339) + `"}}`

	t.Run("unexpected raw database error returns 500 without leaking raw SQL", func(t *testing.T) {
		rawSQLError := errors.New("pq: relation \"consent_records\" does not exist at character 42")
		faultRepo := &faultInjectingRepo{Repository: baseRepo, recordGovernedErr: rawSQLError}
		svc := service.New(faultRepo)

		var logBuf bytes.Buffer
		handler := newTestAPI(svc, &logBuf)

		req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Must be 500 Internal Server Error (NEVER 422!)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 for raw database error, got %d: %s", w.Code, w.Body.String())
		}

		var body errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body.Error != "internal_error" {
			t.Errorf("error code got %q, want internal_error", body.Error)
		}

		// Verify NO SQL/schema leakage to the client
		bodyStr := w.Body.String()
		for _, leak := range []string{"pq:", "consent_records", "character 42"} {
			if strings.Contains(bodyStr, leak) {
				t.Fatalf("database error leaked to client: contains %q in body: %s", leak, bodyStr)
			}
		}

		// Verify correlation ID in header matches body
		cid := w.Header().Get(correlationHeader)
		if cid == "" || cid != body.CorrelationID {
			t.Fatalf("correlation header %q does not match body correlation_id %q", cid, body.CorrelationID)
		}

		// Verify internal cause was safely logged with the correlation ID
		logOutput := logBuf.String()
		if !strings.Contains(logOutput, cid) {
			t.Errorf("log output does not contain correlation ID %s: %s", cid, logOutput)
		}
		if !strings.Contains(logOutput, "relation") {
			t.Errorf("log output does not contain sanitized internal cause: %s", logOutput)
		}
	})

	t.Run("database timeout returns 503 unavailable with Retry-After", func(t *testing.T) {
		timeoutErr := domain.ErrUnavailable.Wrap(context.DeadlineExceeded)
		faultRepo := &faultInjectingRepo{Repository: baseRepo, recordGovernedErr: timeoutErr}
		svc := service.New(faultRepo)

		var logBuf bytes.Buffer
		handler := newTestAPI(svc, &logBuf)

		req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 for DB timeout, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Retry-After") != "1" {
			t.Errorf("expected Retry-After: 1 header, got %q", w.Header().Get("Retry-After"))
		}

		var body errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body.Error != "unavailable" {
			t.Errorf("error code got %q, want unavailable", body.Error)
		}
	})

	t.Run("database contention returns 503 contention with Retry-After", func(t *testing.T) {
		faultRepo := &faultInjectingRepo{Repository: baseRepo, recordGovernedErr: domain.ErrContention}
		svc := service.New(faultRepo)

		var logBuf bytes.Buffer
		handler := newTestAPI(svc, &logBuf)

		req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 for contention, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Retry-After") != "1" {
			t.Errorf("expected Retry-After: 1 header, got %q", w.Header().Get("Retry-After"))
		}

		var body errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body.Error != "contention" {
			t.Errorf("error code got %q, want contention", body.Error)
		}
	})
}

// AT-024: Test OpenAPI 3.1 Specification structure, endpoints, schemas and security.
func TestOpenAPISpecStructureAndEndpoints(t *testing.T) {
	specPaths := []string{"../../api/openapi.yaml", "api/openapi.yaml", "../api/openapi.yaml"}
	var specData []byte
	var err error
	for _, p := range specPaths {
		specData, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("failed to read api/openapi.yaml: %v", err)
	}

	content := string(specData)
	if !strings.HasPrefix(content, "openapi: 3.1.0") {
		t.Errorf("expected openapi: 3.1.0 version header, got: %s", content[:min(30, len(content))])
	}

	requiredEndpoints := []string{
		"/healthz:",
		"/readyz:",
		"/v1/purposes:",
		"/v1/lawful-bases:",
		"/v1/notices:",
		"/v1/consents:",
		"/v1/consents/{id}/withdraw:",
		"/v1/suppressions:",
		"/v1/evaluate:",
		"/v1/evaluate:batch:",
		"/v1/double-opt-in:",
		"/v1/double-opt-in/confirm:",
		"/v1/receipts/{id}:",
	}
	for _, ep := range requiredEndpoints {
		if !strings.Contains(content, ep) {
			t.Errorf("openapi.yaml missing required endpoint %s", ep)
		}
	}

	requiredSchemas := []string{
		"Consent:",
		"ConsentListResponse:",
		"Suppression:",
		"SuppressionListResponse:",
		"Purpose:",
		"PurposeListResponse:",
		"LawfulBasis:",
		"LawfulBasisListResponse:",
		"PrivacyNotice:",
		"NoticeListResponse:",
		"Evidence:",
		"ConsentReceipt:",
		"SuppressionDecision:",
		"ErrorResponse:",
	}
	for _, sc := range requiredSchemas {
		if !strings.Contains(content, sc) {
			t.Errorf("openapi.yaml missing required schema %s", sc)
		}
	}

	requiredSecurity := []string{
		"BearerAuth:",
		"DevHeaders:",
	}
	for _, sec := range requiredSecurity {
		if !strings.Contains(content, sec) {
			t.Errorf("openapi.yaml missing required security scheme %s", sec)
		}
	}

	requiredHeaders := []string{
		"Idempotency-Key",
		"X-Request-Hash",
		"X-Correlation-ID",
	}
	for _, h := range requiredHeaders {
		if !strings.Contains(content, h) {
			t.Errorf("openapi.yaml missing header definition %s", h)
		}
	}
}

// AT-024: Test authorized preference discovery and prove negative authorization cases.
func TestAuthorizedPreferenceDiscoveryAndNegativeAuthz(t *testing.T) {
	t.Setenv("JWT_JWKS_URL", "")
	t.Setenv("JWT_HMAC_SECRET", contractSecret)
	t.Setenv("ALLOW_HMAC_AUTH", "true")
	t.Setenv("ALLOW_DEV_HEADERS", "false")
	repo := store.NewMemory()
	svc := service.New(repo)
	var logBuf bytes.Buffer
	handler := newTestAPI(svc, &logBuf)

	tenantID := "tenant-alpha"
	now := time.Now().UTC().Truncate(time.Second)

	tokenB := makeContractToken(tenantID, "cus-b", "self_service")
	tokenReader := makeContractToken(tenantID, "reader-1", "consent_reader")

	// Seed consent for Customer A
	_, err := repo.RecordGoverned(context.Background(),
		domain.Consent{
			ID: "con_a1", TenantID: tenantID, SubjectID: "cus-a", Purpose: "marketing", Channel: "email",
			LegalBasis: "consent", Status: domain.StatusGranted, Source: "test", Version: 1, CreatedAt: now, UpdatedAt: now,
		},
		domain.ConsentReceipt{ID: "rcp_a1", TenantID: tenantID, ConsentID: "con_a1", SubjectID: "cus-a", IssuedAt: now},
		domain.AuditEvent{ID: "aud_a1", TenantID: tenantID, ActorID: "admin", Action: "consent.granted", ConsentID: "con_a1", OccurredAt: now},
		domain.OutboxEvent{ID: "evt_a1", TenantID: tenantID, EventType: "consent.granted", AggregateID: "con_a1", OccurredAt: now},
	)
	if err != nil {
		t.Fatalf("failed to seed consent A: %v", err)
	}

	// Seed suppression for Customer A
	_, err = repo.SetSuppression(context.Background(),
		domain.Suppression{
			ID: "sup_a1", TenantID: tenantID, SubjectID: "cus-a", Scope: domain.SuppressionGlobal,
			Reason: "opt_out", Active: true, CreatedByRole: "self_service", Version: 1, UpdatedAt: now,
		},
		domain.AuditEvent{ID: "aud_sa1", TenantID: tenantID, ActorID: "cus-a", Action: "suppression.changed", ConsentID: "sup_a1", OccurredAt: now},
		domain.OutboxEvent{ID: "evt_sa1", TenantID: tenantID, EventType: "suppression.changed", AggregateID: "sup_a1", OccurredAt: now},
	)
	if err != nil {
		t.Fatalf("failed to seed suppression A: %v", err)
	}

	// Seed consent for Customer B
	_, err = repo.RecordGoverned(context.Background(),
		domain.Consent{
			ID: "con_b1", TenantID: tenantID, SubjectID: "cus-b", Purpose: "marketing", Channel: "email",
			LegalBasis: "consent", Status: domain.StatusGranted, Source: "test", Version: 1, CreatedAt: now, UpdatedAt: now,
		},
		domain.ConsentReceipt{ID: "rcp_b1", TenantID: tenantID, ConsentID: "con_b1", SubjectID: "cus-b", IssuedAt: now},
		domain.AuditEvent{ID: "aud_b1", TenantID: tenantID, ActorID: "admin", Action: "consent.granted", ConsentID: "con_b1", OccurredAt: now},
		domain.OutboxEvent{ID: "evt_b1", TenantID: tenantID, EventType: "consent.granted", AggregateID: "con_b1", OccurredAt: now},
	)
	if err != nil {
		t.Fatalf("failed to seed consent B: %v", err)
	}

	// Seed suppression for Customer B
	_, err = repo.SetSuppression(context.Background(),
		domain.Suppression{
			ID: "sup_b1", TenantID: tenantID, SubjectID: "cus-b", Scope: domain.SuppressionGlobal,
			Reason: "opt_out", Active: true, CreatedByRole: "self_service", Version: 1, UpdatedAt: now,
		},
		domain.AuditEvent{ID: "aud_sb1", TenantID: tenantID, ActorID: "cus-b", Action: "suppression.changed", ConsentID: "sup_b1", OccurredAt: now},
		domain.OutboxEvent{ID: "evt_sb1", TenantID: tenantID, EventType: "suppression.changed", AggregateID: "sup_b1", OccurredAt: now},
	)
	if err != nil {
		t.Fatalf("failed to seed suppression B: %v", err)
	}

	t.Run("Customer B querying Customer A consents returns 403 Forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/consents?subject_id=cus-a", nil)
		req.Header.Set("Authorization", "Bearer "+tokenB)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected status 403 Forbidden, got %d: %s", w.Code, w.Body.String())
		}
		var body errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to parse error: %v", err)
		}
		if body.Error != "subject_not_authorized" {
			t.Errorf("expected error subject_not_authorized, got %q", body.Error)
		}
	})

	t.Run("Customer B querying Customer A suppressions returns 403 Forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/suppressions?subject_id=cus-a", nil)
		req.Header.Set("Authorization", "Bearer "+tokenB)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected status 403 Forbidden, got %d: %s", w.Code, w.Body.String())
		}
		var body errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to parse error: %v", err)
		}
		if body.Error != "subject_not_authorized" {
			t.Errorf("expected error subject_not_authorized, got %q", body.Error)
		}
	})

	t.Run("Customer B querying own preferences returns 200 OK", func(t *testing.T) {
		// Consents
		req := httptest.NewRequest(http.MethodGet, "/v1/consents?subject_id=cus-b", nil)
		req.Header.Set("Authorization", "Bearer "+tokenB)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 OK, got %d: %s", w.Code, w.Body.String())
		}
		var resp listResponse[domain.Consent]
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode consents response: %v", err)
		}
		if resp.Total != 1 || len(resp.Items) != 1 || resp.Items[0].SubjectID != "cus-b" {
			t.Errorf("unexpected consents response: %+v", resp)
		}

		// Suppressions
		reqSup := httptest.NewRequest(http.MethodGet, "/v1/suppressions?subject_id=cus-b", nil)
		reqSup.Header.Set("Authorization", "Bearer "+tokenB)
		wSup := httptest.NewRecorder()

		handler.ServeHTTP(wSup, reqSup)

		if wSup.Code != http.StatusOK {
			t.Fatalf("expected status 200 OK, got %d: %s", wSup.Code, wSup.Body.String())
		}
		var respSup listResponse[domain.Suppression]
		if err := json.Unmarshal(wSup.Body.Bytes(), &respSup); err != nil {
			t.Fatalf("failed to decode suppressions response: %v", err)
		}
		if respSup.Total != 1 || len(respSup.Items) != 1 || respSup.Items[0].SubjectID != "cus-b" {
			t.Errorf("unexpected suppressions response: %+v", respSup)
		}
	})

	t.Run("Missing subject_id on preferences returns 400 Bad Request", func(t *testing.T) {
		for _, endpoint := range []string{"/v1/consents", "/v1/suppressions"} {
			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			req.Header.Set("Authorization", "Bearer "+tokenReader)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("endpoint %s expected status 400, got %d: %s", endpoint, w.Code, w.Body.String())
			}
			var body errorBody
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to parse error: %v", err)
			}
			if body.Error != "required_field" || body.Field != "subject_id" {
				t.Errorf("expected required_field for subject_id, got %q (field %q)", body.Error, body.Field)
			}
		}
	})

	t.Run("Consent reader role can read tenant-wide subjects", func(t *testing.T) {
		for _, sub := range []string{"cus-a", "cus-b"} {
			req := httptest.NewRequest(http.MethodGet, "/v1/consents?subject_id="+sub, nil)
			req.Header.Set("Authorization", "Bearer "+tokenReader)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected status 200 OK for %s, got %d: %s", sub, w.Code, w.Body.String())
			}

			reqSup := httptest.NewRequest(http.MethodGet, "/v1/suppressions?subject_id="+sub, nil)
			reqSup.Header.Set("Authorization", "Bearer "+tokenReader)
			wSup := httptest.NewRecorder()

			handler.ServeHTTP(wSup, reqSup)

			if wSup.Code != http.StatusOK {
				t.Fatalf("expected status 200 OK for %s, got %d: %s", sub, wSup.Code, wSup.Body.String())
			}
		}
	})
}

// AT-024: Test bounded pagination across preferences and policy registries.
func TestPreferenceDiscoveryAndRegistryPagination(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	repo := store.NewMemory()
	svc := service.New(repo)
	var logBuf bytes.Buffer
	handler := newTestAPI(svc, &logBuf)

	tenantID := "tenant-pagination"
	subjectID := "cus-pagination"
	now := time.Now().UTC().Truncate(time.Second)

	// Seed 25 consents
	for i := 0; i < 25; i++ {
		cid := fmt.Sprintf("con_p_%02d", i)
		ts := now.Add(time.Duration(i) * time.Minute)
		_, _ = repo.RecordGoverned(context.Background(),
			domain.Consent{
				ID: cid, TenantID: tenantID, SubjectID: subjectID, Purpose: "marketing", Channel: fmt.Sprintf("ch_%02d", i),
				LegalBasis: "consent", Status: domain.StatusGranted, Source: "seed", Version: 1, CreatedAt: ts, UpdatedAt: ts,
			},
			domain.ConsentReceipt{ID: "rcp_" + cid, TenantID: tenantID, ConsentID: cid, SubjectID: subjectID, IssuedAt: ts},
			domain.AuditEvent{ID: "aud_" + cid, TenantID: tenantID, ActorID: "admin", Action: "consent.granted", ConsentID: cid, OccurredAt: ts},
			domain.OutboxEvent{ID: "evt_" + cid, TenantID: tenantID, EventType: "consent.granted", AggregateID: cid, OccurredAt: ts},
		)
	}

	// Seed 25 suppressions
	for i := 0; i < 25; i++ {
		sid := fmt.Sprintf("sup_p_%02d", i)
		ts := now.Add(time.Duration(i) * time.Minute)
		_, _ = repo.SetSuppression(context.Background(),
			domain.Suppression{
				ID: sid, TenantID: tenantID, SubjectID: subjectID, Scope: domain.SuppressionChannel, Channel: fmt.Sprintf("ch_%02d", i),
				Reason: "opt_out", Active: true, CreatedByRole: "privacy_admin", Version: 1, UpdatedAt: ts,
			},
			domain.AuditEvent{ID: "aud_" + sid, TenantID: tenantID, ActorID: "admin", Action: "suppression.changed", ConsentID: sid, OccurredAt: ts},
			domain.OutboxEvent{ID: "evt_" + sid, TenantID: tenantID, EventType: "suppression.changed", AggregateID: sid, OccurredAt: ts},
		)
	}

	// Seed 25 purposes
	for i := 0; i < 25; i++ {
		code := fmt.Sprintf("purpose_%02d", i)
		_, _ = repo.PutPurpose(context.Background(),
			domain.Purpose{TenantID: tenantID, Code: code, Category: "cat", AllowedChannels: []string{"email"}, Active: true, Version: 1, UpdatedAt: now},
			domain.AuditEvent{ID: "aud_" + code, TenantID: tenantID, ActorID: "admin", Action: "purpose.registered", ConsentID: code, OccurredAt: now},
			domain.OutboxEvent{ID: "evt_" + code, TenantID: tenantID, EventType: "purpose.changed", AggregateID: code, OccurredAt: now},
		)
	}

	// Seed 25 lawful bases
	for i := 0; i < 25; i++ {
		code := fmt.Sprintf("basis_%02d", i)
		_, _ = repo.PutLawfulBasis(context.Background(),
			domain.LawfulBasis{TenantID: tenantID, Code: code, AllowedCategories: []string{"cat"}, Active: true, Version: 1, UpdatedAt: now},
			domain.AuditEvent{ID: "aud_" + code, TenantID: tenantID, ActorID: "admin", Action: "lawful_basis.registered", ConsentID: code, OccurredAt: now},
			domain.OutboxEvent{ID: "evt_" + code, TenantID: tenantID, EventType: "lawful_basis.changed", AggregateID: code, OccurredAt: now},
		)
	}

	// Seed 25 notices
	for i := 0; i < 25; i++ {
		ver := fmt.Sprintf("v%02d", i)
		ts := now.Add(-time.Duration(25-i) * time.Hour)
		_, _ = repo.PutNotice(context.Background(),
			domain.PrivacyNotice{TenantID: tenantID, Jurisdiction: "US", Version: ver, PublishedAt: ts, Active: true},
			domain.AuditEvent{ID: "aud_" + ver, TenantID: tenantID, ActorID: "admin", Action: "notice.published", ConsentID: ver, OccurredAt: ts},
			domain.OutboxEvent{ID: "evt_" + ver, TenantID: tenantID, EventType: "notice.published", AggregateID: ver, OccurredAt: ts},
		)
	}

	t.Run("Consents pagination works with limit and offset", func(t *testing.T) {
		// Page 1: limit 10, offset 0
		req := httptest.NewRequest(http.MethodGet, "/v1/consents?subject_id="+subjectID+"&limit=10&offset=0", nil)
		req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
		req.Header.Set("X-Nvuto-Actor-ID", "admin")
		req.Header.Set("X-Nvuto-Roles", "consent_reader")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("page 1 failed: status %d: %s", w.Code, w.Body.String())
		}
		var p1 listResponse[domain.Consent]
		if err := json.Unmarshal(w.Body.Bytes(), &p1); err != nil {
			t.Fatalf("decode page 1 failed: %v", err)
		}
		if p1.Total != 25 || len(p1.Items) != 10 || p1.Limit != 10 || p1.Offset != 0 {
			t.Fatalf("page 1 unexpected: total=%d, items=%d", p1.Total, len(p1.Items))
		}

		// Page 2: limit 10, offset 10
		req2 := httptest.NewRequest(http.MethodGet, "/v1/consents?subject_id="+subjectID+"&limit=10&offset=10", nil)
		req2.Header.Set("X-Nvuto-Tenant-ID", tenantID)
		req2.Header.Set("X-Nvuto-Actor-ID", "admin")
		req2.Header.Set("X-Nvuto-Roles", "consent_reader")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("page 2 failed: status %d: %s", w2.Code, w2.Body.String())
		}
		var p2 listResponse[domain.Consent]
		if err := json.Unmarshal(w2.Body.Bytes(), &p2); err != nil {
			t.Fatalf("decode page 2 failed: %v", err)
		}
		if p2.Total != 25 || len(p2.Items) != 10 || p2.Limit != 10 || p2.Offset != 10 {
			t.Fatalf("page 2 unexpected: total=%d, items=%d", p2.Total, len(p2.Items))
		}

		// Verify no overlap between page 1 and page 2
		seen := make(map[string]bool)
		for _, item := range p1.Items {
			seen[item.ID] = true
		}
		for _, item := range p2.Items {
			if seen[item.ID] {
				t.Errorf("consent ID %s appeared on both page 1 and page 2", item.ID)
			}
		}

		// Page 3: limit 10, offset 20 (only 5 remain)
		req3 := httptest.NewRequest(http.MethodGet, "/v1/consents?subject_id="+subjectID+"&limit=10&offset=20", nil)
		req3.Header.Set("X-Nvuto-Tenant-ID", tenantID)
		req3.Header.Set("X-Nvuto-Actor-ID", "admin")
		req3.Header.Set("X-Nvuto-Roles", "consent_reader")
		w3 := httptest.NewRecorder()
		handler.ServeHTTP(w3, req3)
		var p3 listResponse[domain.Consent]
		_ = json.Unmarshal(w3.Body.Bytes(), &p3)
		if p3.Total != 25 || len(p3.Items) != 5 {
			t.Fatalf("page 3 unexpected: total=%d, items=%d (want 5)", p3.Total, len(p3.Items))
		}

		// Beyond total: offset 30
		req4 := httptest.NewRequest(http.MethodGet, "/v1/consents?subject_id="+subjectID+"&limit=10&offset=30", nil)
		req4.Header.Set("X-Nvuto-Tenant-ID", tenantID)
		req4.Header.Set("X-Nvuto-Actor-ID", "admin")
		req4.Header.Set("X-Nvuto-Roles", "consent_reader")
		w4 := httptest.NewRecorder()
		handler.ServeHTTP(w4, req4)
		var p4 listResponse[domain.Consent]
		_ = json.Unmarshal(w4.Body.Bytes(), &p4)
		if p4.Total != 25 || len(p4.Items) != 0 {
			t.Fatalf("page 4 unexpected: total=%d, items=%d (want 0)", p4.Total, len(p4.Items))
		}
	})

	t.Run("Default limit of 20 applies when unspecified", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/purposes", nil)
		req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
		req.Header.Set("X-Nvuto-Actor-ID", "admin")
		req.Header.Set("X-Nvuto-Roles", "consent_reader")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("purposes default pagination failed: %d: %s", w.Code, w.Body.String())
		}
		var resp listResponse[domain.Purpose]
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Limit != 20 || resp.Offset != 0 || len(resp.Items) != 20 || resp.Total != 25 {
			t.Fatalf("default pagination unexpected: %+v", resp)
		}
	})

	t.Run("Invalid pagination parameters rejected with 400 Bad Request", func(t *testing.T) {
		invalidCases := []struct {
			query string
			field string
		}{
			{"limit=-1", "limit"},
			{"limit=0", "limit"},
			{"limit=101", "limit"},
			{"limit=abc", "limit"},
			{"offset=-1", "offset"},
			{"offset=foo", "offset"},
		}
		for _, tc := range invalidCases {
			req := httptest.NewRequest(http.MethodGet, "/v1/purposes?"+tc.query, nil)
			req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
			req.Header.Set("X-Nvuto-Actor-ID", "admin")
			req.Header.Set("X-Nvuto-Roles", "consent_reader")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("query %s expected 400, got %d", tc.query, w.Code)
			}
			var body errorBody
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			if body.Error != "invalid_type" || body.Field != tc.field {
				t.Errorf("query %s expected invalid_type for field %s, got error=%s field=%s",
					tc.query, tc.field, body.Error, body.Field)
			}
		}
	})
}

// AT-024: Test Response DTO contract conformance against OpenAPI schemas.
func TestResponseDTOContractConformance(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	repo := store.NewMemory()
	svc := service.New(repo)
	var logBuf bytes.Buffer
	handler := newTestAPI(svc, &logBuf)

	tenantID := "tenant-dto"
	subjectID := "cus-dto"
	now := time.Now().UTC().Truncate(time.Second)

	// Seed one of each entity
	_, _ = repo.PutPurpose(context.Background(),
		domain.Purpose{TenantID: tenantID, Code: "marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, RequiresDoubleOptIn: false, Active: true, Version: 1, UpdatedAt: now},
		domain.AuditEvent{}, domain.OutboxEvent{},
	)
	_, _ = repo.PutLawfulBasis(context.Background(),
		domain.LawfulBasis{TenantID: tenantID, Code: "consent", AllowedCategories: []string{"marketing"}, RequiresEvidence: true, Active: true, Version: 1, UpdatedAt: now},
		domain.AuditEvent{}, domain.OutboxEvent{},
	)
	_, _ = repo.PutNotice(context.Background(),
		domain.PrivacyNotice{TenantID: tenantID, Version: "2026-08", Jurisdiction: "NG", PublishedAt: now, Active: true},
		domain.AuditEvent{}, domain.OutboxEvent{},
	)
	_, _ = repo.RecordGoverned(context.Background(),
		domain.Consent{
			ID: "con_dto_1", TenantID: tenantID, SubjectID: subjectID, Purpose: "marketing", Channel: "email",
			LegalBasis: "consent", Status: domain.StatusGranted, Source: "portal", Version: 1, CreatedAt: now, UpdatedAt: now,
		},
		domain.ConsentReceipt{ID: "rcp_dto_1", TenantID: tenantID, ConsentID: "con_dto_1", SubjectID: subjectID, IssuedAt: now},
		domain.AuditEvent{}, domain.OutboxEvent{},
	)
	_, _ = repo.SetSuppression(context.Background(),
		domain.Suppression{
			ID: "sup_dto_1", TenantID: tenantID, SubjectID: subjectID, Scope: domain.SuppressionGlobal,
			Reason: "user_optout", Active: true, CreatedByRole: "privacy_admin", Version: 1, UpdatedAt: now,
		},
		domain.AuditEvent{}, domain.OutboxEvent{},
	)

	headers := map[string]string{
		"X-Nvuto-Tenant-ID": tenantID,
		"X-Nvuto-Actor-ID":  "admin",
		"X-Nvuto-Roles":     "consent_reader",
	}

	validateEnvelope := func(t *testing.T, raw []byte) map[string]any {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("failed to decode JSON map: %v", err)
		}
		for _, key := range []string{"items", "total", "limit", "offset"} {
			if _, ok := m[key]; !ok {
				t.Errorf("missing envelope key: %s", key)
			}
		}
		return m
	}

	t.Run("GET /v1/consents conforms to ConsentListResponse", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/consents?subject_id="+subjectID, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status got %d: %s", w.Code, w.Body.String())
		}
		m := validateEnvelope(t, w.Body.Bytes())
		items := m["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		consent := items[0].(map[string]any)
		for _, field := range []string{"id", "tenant_id", "subject_id", "purpose", "channel", "legal_basis", "status", "source", "version", "created_at", "updated_at"} {
			if _, ok := consent[field]; !ok {
				t.Errorf("consent record missing schema field %q", field)
			}
		}
	})

	t.Run("GET /v1/suppressions conforms to SuppressionListResponse", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/suppressions?subject_id="+subjectID, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status got %d: %s", w.Code, w.Body.String())
		}
		m := validateEnvelope(t, w.Body.Bytes())
		items := m["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		sup := items[0].(map[string]any)
		for _, field := range []string{"id", "tenant_id", "subject_id", "scope", "reason", "active", "version", "updated_at"} {
			if _, ok := sup[field]; !ok {
				t.Errorf("suppression record missing schema field %q", field)
			}
		}
	})

	t.Run("GET /v1/purposes conforms to PurposeListResponse", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/purposes", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status got %d: %s", w.Code, w.Body.String())
		}
		m := validateEnvelope(t, w.Body.Bytes())
		items := m["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		p := items[0].(map[string]any)
		for _, field := range []string{"tenant_id", "code", "category", "allowed_channels", "requires_affirmative_consent", "requires_double_opt_in", "active", "version", "updated_at"} {
			if _, ok := p[field]; !ok {
				t.Errorf("purpose record missing schema field %q", field)
			}
		}
	})

	t.Run("GET /v1/lawful-bases conforms to LawfulBasisListResponse", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/lawful-bases", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status got %d: %s", w.Code, w.Body.String())
		}
		m := validateEnvelope(t, w.Body.Bytes())
		items := m["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		b := items[0].(map[string]any)
		for _, field := range []string{"tenant_id", "code", "allowed_categories", "requires_evidence", "active", "version", "updated_at"} {
			if _, ok := b[field]; !ok {
				t.Errorf("lawful basis record missing schema field %q", field)
			}
		}
	})

	t.Run("GET /v1/notices conforms to NoticeListResponse", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/notices", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status got %d: %s", w.Code, w.Body.String())
		}
		m := validateEnvelope(t, w.Body.Bytes())
		items := m["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		n := items[0].(map[string]any)
		for _, field := range []string{"tenant_id", "version", "jurisdiction", "published_at", "active"} {
			if _, ok := n[field]; !ok {
				t.Errorf("privacy notice record missing schema field %q", field)
			}
		}
	})
}
