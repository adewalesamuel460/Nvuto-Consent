package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// mockHealthRepo allows simulating storage reachability and schema check failures.
type mockHealthRepo struct {
	store.Repository
	pingErr         error
	verifySchemaErr error
}

func (m *mockHealthRepo) Ping(ctx context.Context) error {
	if m.pingErr != nil {
		return m.pingErr
	}
	return nil
}

func (m *mockHealthRepo) VerifySchema(ctx context.Context) error {
	if m.verifySchemaErr != nil {
		return m.verifySchemaErr
	}
	return nil
}

// TestAT015_HealthEndpointsMethodRestriction verifies that /healthz and /readyz
// strictly accept only HTTP GET and reject all other methods (POST, CONNECT,
// PUT, DELETE, PATCH) with 405 Method Not Allowed and the Allow: GET header.
func TestAT015_HealthEndpointsMethodRestriction(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	repo := store.NewMemory()
	svc := service.New(repo)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := newHandler(svc, 0, logger)

	rejectedMethods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		"CONNECT",
	}

	endpoints := []string{"/healthz", "/readyz"}

	for _, endpoint := range endpoints {
		for _, method := range rejectedMethods {
			t.Run(endpoint+"_"+method, func(t *testing.T) {
				req := httptest.NewRequest(method, endpoint, nil)
				resp := httptest.NewRecorder()
				handler.ServeHTTP(resp, req)

				if resp.Code != http.StatusMethodNotAllowed {
					t.Fatalf("expected 405 Method Not Allowed for %s %s, got %d: %s",
						method, endpoint, resp.Code, resp.Body.String())
				}
				if allow := resp.Header().Get("Allow"); allow != http.MethodGet {
					t.Fatalf("expected Allow: %s, got %q", http.MethodGet, allow)
				}
			})
		}
	}

	// Verify GET succeeds
	t.Run("healthz_GET_ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for GET /healthz, got %d: %s", resp.Code, resp.Body.String())
		}
		var body map[string]string
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["status"] != "ok" {
			t.Fatalf("expected status: ok, got %v", body["status"])
		}
	})

	t.Run("readyz_GET_ready", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for GET /readyz, got %d: %s", resp.Code, resp.Body.String())
		}
		var body map[string]string
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["status"] != "ready" {
			t.Fatalf("expected status: ready, got %v", body["status"])
		}
	})
}

// TestAT015_HealthzLivenessIndependentOfDependencies proves that /healthz returns
// 200 OK even when database ping and schema checks are failing. Liveness reflects
// process liveness and must not fail on routine dependency outages.
func TestAT015_HealthzLivenessIndependentOfDependencies(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	mockRepo := &mockHealthRepo{
		Repository:      store.NewMemory(),
		pingErr:         domain.ErrUnavailable.Derive("db_down", "connection refused"),
		verifySchemaErr: domain.ErrUnavailable.Derive("schema_broken", "relation does not exist"),
	}
	svc := service.New(mockRepo)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := newHandler(svc, 0, logger)

	// Liveness probe (/healthz) must still be 200 OK
	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthResp := httptest.NewRecorder()
	handler.ServeHTTP(healthResp, healthReq)

	if healthResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /healthz during DB outage, got %d: %s",
			healthResp.Code, healthResp.Body.String())
	}
	var healthBody map[string]string
	if err := json.Unmarshal(healthResp.Body.Bytes(), &healthBody); err != nil {
		t.Fatal(err)
	}
	if healthBody["status"] != "ok" {
		t.Fatalf("expected status: ok, got %v", healthBody["status"])
	}

	// Readiness probe (/readyz) must fail with 503
	readyReq := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	readyResp := httptest.NewRecorder()
	handler.ServeHTTP(readyResp, readyReq)

	if readyResp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable for /readyz during DB outage, got %d: %s",
			readyResp.Code, readyResp.Body.String())
	}
	var readyBody map[string]string
	if err := json.Unmarshal(readyResp.Body.Bytes(), &readyBody); err != nil {
		t.Fatal(err)
	}
	if readyBody["status"] != "not_ready" {
		t.Fatalf("expected status: not_ready, got %v", readyBody["status"])
	}
}

// TestAT015_ReadyzFailsWhenSchemaCheckFails proves that /readyz returns 503 if
// the database is reachable (Ping succeeds) but the schema verification check fails.
func TestAT015_ReadyzFailsWhenSchemaCheckFails(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	mockRepo := &mockHealthRepo{
		Repository:      store.NewMemory(),
		pingErr:         nil, // Ping succeeds
		verifySchemaErr: domain.ErrUnavailable.Derive("schema_uninitialized", "required database migrations have not been applied"),
	}
	svc := service.New(mockRepo)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := newHandler(svc, 0, logger)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable when schema check fails, got %d: %s",
			resp.Code, resp.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "not_ready" {
		t.Fatalf("expected status: not_ready, got %v", body["status"])
	}
}

// TestAT015_ReadyzFailsWhenAuthUnconfigured proves that /readyz returns 503 if
// authentication is completely unconfigured.
func TestAT015_ReadyzFailsWhenAuthUnconfigured(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "false")
	t.Setenv("JWT_HMAC_SECRET", "")
	t.Setenv("JWT_JWKS_URL", "")

	repo := store.NewMemory()
	svc := service.New(repo)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := newHandler(svc, 0, logger)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when auth is unconfigured, got %d: %s", resp.Code, resp.Body.String())
	}
}

// TestAT015_StrictStartupValidation verifies that startup validation rejects
// missing or malformed configuration.
func TestAT015_StrictStartupValidation(t *testing.T) {
	t.Run("auth validation rejects missing configuration", func(t *testing.T) {
		t.Setenv("ALLOW_DEV_HEADERS", "false")
		t.Setenv("JWT_HMAC_SECRET", "")
		t.Setenv("JWT_JWKS_URL", "")
		if err := ValidateAuthConfig(); err == nil {
			t.Fatal("expected ValidateAuthConfig to fail when no auth method is enabled")
		}
	})

	t.Run("storage validation rejects malformed DATABASE_URL", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "://invalid-url-not-parsable")
		t.Setenv("ALLOW_DEV_HEADERS", "true")
		err := validateStorageConfig()
		if err == nil {
			t.Fatal("expected error for malformed DATABASE_URL, got nil")
		}
		if !errors.Is(err, err) && err.Error() == "" {
			t.Fatal("expected non-empty error")
		}
	})

	t.Run("storage validation rejects memory store in production", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")
		t.Setenv("ALLOW_MEMORY_STORE", "true")
		t.Setenv("ENVIRONMENT", "production")
		t.Setenv("ALLOW_DEV_HEADERS", "true")
		err := validateStorageConfig()
		if err == nil || !strings.Contains(err.Error(), "ALLOW_MEMORY_STORE=true is prohibited in production configuration") {
			t.Fatalf("expected production memory rejection, got: %v", err)
		}
	})
}
