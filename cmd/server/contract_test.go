package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

func newTestAPI(svc *service.Service, logBuf *bytes.Buffer) http.Handler {
	logger := log.New(logBuf, "", 0)
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
			name:       "omitted required boolean active",
			path:       "/v1/purposes",
			body:       `{"code":"marketing","category":"marketing","allowed_channels":["email"],"requires_affirmative_consent":true,"requires_double_opt_in":false}`,
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
