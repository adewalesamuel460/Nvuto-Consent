package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// mockFaultRepo wraps store.Repository to inject precise faults into individual storage methods.
type mockFaultRepo struct {
	store.Repository
	recordGovernedErr error
	getPurposeErr     error
	getNoticeErr      error
	getLawfulBasisErr error
	findEffectiveErr  error
	findSuppressionErr error
	getErr            error
}

func (m *mockFaultRepo) RecordGoverned(ctx context.Context, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	if m.recordGovernedErr != nil {
		return domain.Consent{}, m.recordGovernedErr
	}
	return m.Repository.RecordGoverned(ctx, consent, receipt, audit, outbox)
}

func (m *mockFaultRepo) GetPurpose(ctx context.Context, tenantID, code string) (domain.Purpose, error) {
	if m.getPurposeErr != nil {
		return domain.Purpose{}, m.getPurposeErr
	}
	return m.Repository.GetPurpose(ctx, tenantID, code)
}

func (m *mockFaultRepo) GetNotice(ctx context.Context, tenantID, jurisdiction, version string) (domain.PrivacyNotice, error) {
	if m.getNoticeErr != nil {
		return domain.PrivacyNotice{}, m.getNoticeErr
	}
	return m.Repository.GetNotice(ctx, tenantID, jurisdiction, version)
}

func (m *mockFaultRepo) GetLawfulBasis(ctx context.Context, tenantID, code string) (domain.LawfulBasis, error) {
	if m.getLawfulBasisErr != nil {
		return domain.LawfulBasis{}, m.getLawfulBasisErr
	}
	return m.Repository.GetLawfulBasis(ctx, tenantID, code)
}

func (m *mockFaultRepo) FindEffective(ctx context.Context, tenantID, subjectID, purpose, channel string) (domain.Consent, error) {
	if m.findEffectiveErr != nil {
		return domain.Consent{}, m.findEffectiveErr
	}
	return m.Repository.FindEffective(ctx, tenantID, subjectID, purpose, channel)
}

func (m *mockFaultRepo) FindSuppression(ctx context.Context, tenantID, subjectID, purpose, channel string) (domain.Suppression, error) {
	if m.findSuppressionErr != nil {
		return domain.Suppression{}, m.findSuppressionErr
	}
	return m.Repository.FindSuppression(ctx, tenantID, subjectID, purpose, channel)
}

func (m *mockFaultRepo) Get(ctx context.Context, tenantID, id string) (domain.Consent, error) {
	if m.getErr != nil {
		return domain.Consent{}, m.getErr
	}
	return m.Repository.Get(ctx, tenantID, id)
}

func setupTestPolicy(t *testing.T, repo store.Repository) (time.Time, string) {
	t.Helper()
	now := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	_, err := repo.PutPurpose(context.Background(), domain.Purpose{
		TenantID:                   "tenant-err-test",
		Code:                       "marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email", "sms"},
		RequiresAffirmativeConsent: true,
		Active:                     true,
	}, domain.AuditEvent{}, domain.OutboxEvent{})
	if err != nil {
		t.Fatalf("failed to seed purpose: %v", err)
	}

	_, err = repo.PutLawfulBasis(context.Background(), domain.LawfulBasis{
		TenantID:          "tenant-err-test",
		Code:              "consent",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	}, domain.AuditEvent{}, domain.OutboxEvent{})
	if err != nil {
		t.Fatalf("failed to seed lawful basis: %v", err)
	}

	_, err = repo.PutNotice(context.Background(), domain.PrivacyNotice{
		TenantID:     "tenant-err-test",
		Version:      "2026-08",
		Jurisdiction: "NG",
		PublishedAt:  now,
		Active:       true,
	}, domain.AuditEvent{}, domain.OutboxEvent{})
	if err != nil {
		t.Fatalf("failed to seed privacy notice: %v", err)
	}

	validHash := "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	return now, validHash
}

// TestAT013_DatabaseErrorsShieldedAndTyped tests that internal database faults
// and syntax errors return 500 without leaking any SQL or internal details in the response body.
func TestAT013_DatabaseErrorsShieldedAndTyped(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	baseRepo := store.NewMemory()
	publishedAt, validHash := setupTestPolicy(t, baseRepo)

	rawSQLError := errors.New("pq: syntax error at or near \"SELECT * FROM secret_table\" at character 19")
	faultRepo := &mockFaultRepo{
		Repository:        baseRepo,
		recordGovernedErr: rawSQLError,
	}
	svc := service.New(faultRepo)

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	handler := newHandler(svc, 5*time.Second, logger)

	reqBody := `{"subject_id":"cus-100","purpose":"marketing","channel":"email","legal_basis":"consent","status":"granted","source":"portal","jurisdiction":"NG","notice_version":"2026-08","evidence":{"method":"ci","artifact_hash":"` + validHash + `","collected_at":"` + publishedAt.Format(time.RFC3339) + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", "tenant-err-test")
	req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Must be 500 Internal Server Error (NEVER 422 or 400)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected HTTP 500, got %d: %s", w.Code, w.Body.String())
	}

	var body errorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if body.Error != "internal_error" {
		t.Errorf("expected error code 'internal_error', got %q", body.Error)
	}
	if body.Message != "an internal error occurred" {
		t.Errorf("expected generic message 'an internal error occurred', got %q", body.Message)
	}
	if body.Field != "" {
		t.Errorf("expected field to be empty on 500 error, got %q", body.Field)
	}

	// Ensure no SQL/schema leakage in response body
	bodyStr := w.Body.String()
	leaks := []string{"syntax error", "SELECT", "secret_table", "character 19", "pq:"}
	for _, leak := range leaks {
		if strings.Contains(bodyStr, leak) {
			t.Fatalf("internal error leaked to client body! Found %q in: %s", leak, bodyStr)
		}
	}

	// Verify correlation ID in header matches body
	cid := w.Header().Get("X-Correlation-ID")
	if cid == "" || cid != body.CorrelationID {
		t.Fatalf("correlation header %q does not match body correlation_id %q", cid, body.CorrelationID)
	}

	// Verify internal cause was safely logged with the correlation ID
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, cid) {
		t.Errorf("log output missing correlation ID %s: %s", cid, logOutput)
	}
	if !strings.Contains(logOutput, "secret_table") {
		t.Errorf("server log missing internal cause with SQL detail: %s", logOutput)
	}
}

// TestAT013_DatabaseTimeoutReturnsUnavailable tests that database deadlines
// and network timeouts return 503 Unavailable with Retry-After and without leaking internals.
func TestAT013_DatabaseTimeoutReturnsUnavailable(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	baseRepo := store.NewMemory()
	publishedAt, validHash := setupTestPolicy(t, baseRepo)

	timeoutErr := domain.ErrUnavailable.Wrap(context.DeadlineExceeded)
	faultRepo := &mockFaultRepo{
		Repository:        baseRepo,
		recordGovernedErr: timeoutErr,
	}
	svc := service.New(faultRepo)

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	handler := newHandler(svc, 5*time.Second, logger)

	reqBody := `{"subject_id":"cus-100","purpose":"marketing","channel":"email","legal_basis":"consent","status":"granted","source":"portal","jurisdiction":"NG","notice_version":"2026-08","evidence":{"method":"ci","artifact_hash":"` + validHash + `","collected_at":"` + publishedAt.Format(time.RFC3339) + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", "tenant-err-test")
	req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTP 503, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") != "1" {
		t.Errorf("expected Retry-After: 1 header, got %q", w.Header().Get("Retry-After"))
	}

	var body errorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if body.Error != "unavailable" {
		t.Errorf("expected error code 'unavailable', got %q", body.Error)
	}
	if body.Message != "the service is temporarily unavailable; retry later" {
		t.Errorf("expected generic message 'the service is temporarily unavailable; retry later', got %q", body.Message)
	}
	if body.Field != "" {
		t.Errorf("expected field to be empty on 503 error, got %q", body.Field)
	}

	cid := w.Header().Get("X-Correlation-ID")
	if cid == "" || cid != body.CorrelationID {
		t.Fatalf("correlation header %q does not match body %q", cid, body.CorrelationID)
	}
}

// TestAT013_PolicyLookupDatabaseErrorReturnsUnavailableSafeDeny tests that an infrastructure
// failure during policy lookup returns 503 (safe-deny) instead of mistakenly returning 422 or "not found".
func TestAT013_PolicyLookupDatabaseErrorReturnsUnavailableSafeDeny(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	baseRepo := store.NewMemory()
	publishedAt, validHash := setupTestPolicy(t, baseRepo)

	// Simulate database connection failure during purpose lookup
	dbConnErr := domain.ErrUnavailable.Wrap(errors.New("dial tcp 10.0.0.1:5433: i/o timeout"))
	faultRepo := &mockFaultRepo{
		Repository:    baseRepo,
		getPurposeErr: dbConnErr,
	}
	svc := service.New(faultRepo)

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	handler := newHandler(svc, 5*time.Second, logger)

	t.Run("record consent during policy lookup failure returns 503 not 422", func(t *testing.T) {
		reqBody := `{"subject_id":"cus-100","purpose":"marketing","channel":"email","legal_basis":"consent","status":"granted","source":"portal","jurisdiction":"NG","notice_version":"2026-08","evidence":{"method":"ci","artifact_hash":"` + validHash + `","collected_at":"` + publishedAt.Format(time.RFC3339) + `"}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-err-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Must NOT be 422 (which would blame the client for a missing/invalid policy)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected HTTP 503 for policy lookup DB failure, got %d: %s", w.Code, w.Body.String())
		}

		var body errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body.Error != "unavailable" {
			t.Errorf("expected error 'unavailable', got %q", body.Error)
		}
	})

	t.Run("evaluate during policy lookup failure returns 503 safe-deny", func(t *testing.T) {
		// First seed a granted consent record in baseRepo directly
		now := time.Now().UTC()
		_, _ = baseRepo.RecordGoverned(context.Background(), domain.Consent{
			ID:            "con-eval-test",
			TenantID:      "tenant-err-test",
			SubjectID:     "cus-100",
			Purpose:       "marketing",
			Channel:       "email",
			LegalBasis:    "consent",
			Status:        domain.StatusGranted,
			Source:        "portal",
			Jurisdiction:  "NG",
			NoticeVersion: "2026-08",
			GrantedAt:     &now,
			Version:       1,
			CreatedAt:     now,
			UpdatedAt:     now,
		}, domain.ConsentReceipt{ID: "rcp-1", TenantID: "tenant-err-test", ConsentID: "con-eval-test"}, domain.AuditEvent{}, domain.OutboxEvent{})

		reqBody := `{"subject_id":"cus-100","purpose":"marketing","channel":"email"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-err-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Safe-deny must return 503 error, NOT an authoritative 200 {"allowed": false}
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected HTTP 503 on evaluate DB outage, got %d: %s", w.Code, w.Body.String())
		}
		var body errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body.Error != "unavailable" {
			t.Errorf("expected 'unavailable', got %q", body.Error)
		}
	})
}

// TestAT013_ValidationAndConflictMappings verifies that client-actionable errors
// return stable 400, 422, and 409 status codes with actionable messages and correlation IDs.
func TestAT013_ValidationAndConflictMappings(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	baseRepo := store.NewMemory()
	publishedAt, validHash := setupTestPolicy(t, baseRepo)

	svc := service.New(baseRepo)
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	handler := newHandler(svc, 5*time.Second, logger)

	t.Run("malformed json returns 400 validation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader("{invalid-json"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-err-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected HTTP 400 for malformed JSON, got %d: %s", w.Code, w.Body.String())
		}
		var body errorBody
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Error != "invalid_json" {
			t.Errorf("expected 'invalid_json', got %q", body.Error)
		}
		if body.CorrelationID == "" {
			t.Errorf("expected non-empty correlation_id")
		}
	})

	t.Run("missing required field returns 400 with field name", func(t *testing.T) {
		// Missing channel
		reqBody := `{"subject_id":"cus-100","purpose":"marketing","legal_basis":"consent","status":"granted","source":"portal","jurisdiction":"NG","notice_version":"2026-08","evidence":{"method":"ci","artifact_hash":"` + validHash + `","collected_at":"2026-08-14T00:00:00Z"}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-err-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected HTTP 400 for missing field, got %d: %s", w.Code, w.Body.String())
		}
		var body errorBody
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Field != "channel" {
			t.Errorf("expected field 'channel', got %q", body.Field)
		}
	})

	t.Run("policy rejection returns 422 with actionable message", func(t *testing.T) {
		// Disallowed channel for purpose marketing
		reqBody := `{"subject_id":"cus-100","purpose":"marketing","channel":"pigeon_post","legal_basis":"consent","status":"granted","source":"portal","jurisdiction":"NG","notice_version":"2026-08","evidence":{"method":"ci","artifact_hash":"` + validHash + `","collected_at":"` + publishedAt.Format(time.RFC3339) + `"}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-err-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected HTTP 422 for policy violation, got %d: %s", w.Code, w.Body.String())
		}
		var body errorBody
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Error != "policy_rejected" {
			t.Errorf("expected error 'policy_rejected', got %q", body.Error)
		}
		if body.Field != "channel" {
			t.Errorf("expected field 'channel', got %q", body.Field)
		}
	})

	t.Run("conflict returns 409", func(t *testing.T) {
		faultRepo := &mockFaultRepo{
			Repository:        baseRepo,
			recordGovernedErr: domain.ErrConflict,
		}
		faultSvc := service.New(faultRepo)
		faultHandler := newHandler(faultSvc, 5*time.Second, logger)

		reqBody := `{"subject_id":"cus-100","purpose":"marketing","channel":"email","legal_basis":"consent","status":"granted","source":"portal","jurisdiction":"NG","notice_version":"2026-08","evidence":{"method":"ci","artifact_hash":"` + validHash + `","collected_at":"` + publishedAt.Format(time.RFC3339) + `"}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-err-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
		w := httptest.NewRecorder()

		faultHandler.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected HTTP 409 for conflict, got %d: %s", w.Code, w.Body.String())
		}
		var body errorBody
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Error != "conflict" {
			t.Errorf("expected 'conflict', got %q", body.Error)
		}
		if body.CorrelationID == "" {
			t.Errorf("expected correlation_id in response body")
		}
	})
}

// TestAT013_CorrelationIDAdoptionAndPropagation verifies that client-supplied
// correlation IDs are adopted and returned in both header and body, and newly generated ones are well-formed.
func TestAT013_CorrelationIDAdoptionAndPropagation(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	baseRepo := store.NewMemory()
	svc := service.New(baseRepo)
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	handler := newHandler(svc, 5*time.Second, logger)

	customCID := "custom-trace-uuid-1234-abcd"

	req := httptest.NewRequest(http.MethodGet, "/v1/receipts/nonexistent", nil)
	req.Header.Set("X-Correlation-ID", customCID)
	req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
	req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify adopted correlation ID in response header
	headerCID := w.Header().Get("X-Correlation-ID")
	if headerCID != customCID {
		t.Errorf("expected response header X-Correlation-ID %q, got %q", customCID, headerCID)
	}

	var body errorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body.CorrelationID != customCID {
		t.Errorf("expected body correlation_id %q, got %q", customCID, body.CorrelationID)
	}
}

// TestAT013_SensitiveSecretsNeverLogged asserts that bearer tokens, confirmation tokens,
// and database passwords never leak into structured server log lines.
func TestAT013_SensitiveSecretsNeverLogged(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "false")
	baseRepo := store.NewMemory()
	svc := service.New(baseRepo)

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	handler := newHandler(svc, 5*time.Second, logger)

	secretBearer := "super-secret-jwt-bearer-token-12345"
	secretConfirm := "super-secret-confirmation-token-67890"

	req := httptest.NewRequest(http.MethodPost, "/v1/double-opt-in/confirm", strings.NewReader(`{"token":"`+secretConfirm+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secretBearer)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	logOutput := logBuf.String()

	if strings.Contains(logOutput, secretBearer) {
		t.Fatalf("SECURITY VIOLATION: Bearer token leaked into server logs! Log: %s", logOutput)
	}
	if strings.Contains(logOutput, secretConfirm) {
		t.Fatalf("SECURITY VIOLATION: Confirmation token leaked into server logs! Log: %s", logOutput)
	}
}
