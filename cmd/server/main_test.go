package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

func request(t *testing.T, handler http.Handler, method, path string, body any, authenticate bool) *httptest.ResponseRecorder {
	t.Helper()
	var encoded bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &encoded)
	req.Header.Set("Content-Type", "application/json")
	if authenticate {
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-api")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-api")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAPIRequiresExplicitAuthenticationAndProvesGovernedFlow(t *testing.T) {
	t.Setenv("JWT_HMAC_SECRET", "")
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	handler := newMux(service.New(store.NewMemory()))

	response := request(t, handler, http.MethodPost, "/v1/evaluate", map[string]any{"subject_id": "cus-1", "purpose": "marketing", "channel": "email"}, false)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing authentication returned %d: %s", response.Code, response.Body.String())
	}

	publishedAt := time.Now().UTC()
	requests := []struct {
		path string
		body any
	}{
		{"/v1/purposes", map[string]any{"code": "marketing", "category": "marketing", "allowed_channels": []string{"email"}, "requires_affirmative_consent": true, "requires_double_opt_in": false, "active": true}},
		{"/v1/lawful-bases", map[string]any{"code": "consent", "allowed_categories": []string{"marketing"}, "requires_evidence": true, "active": true}},
		{"/v1/notices", map[string]any{"version": "2026-08", "jurisdiction": "NG", "published_at": publishedAt, "active": true}},
	}
	for _, input := range requests {
		response = request(t, handler, http.MethodPost, input.path, input.body, true)
		if response.Code != http.StatusCreated {
			t.Fatalf("%s returned %d: %s", input.path, response.Code, response.Body.String())
		}
	}

	response = request(t, handler, http.MethodPost, "/v1/consents", map[string]any{
		"subject_id": "cus-1", "purpose": "marketing", "channel": "email", "legal_basis": "consent", "status": "granted", "source": "preference_center", "jurisdiction": "NG", "notice_version": "2026-08",
		"evidence": map[string]any{"method": "preference_center", "artifact_hash": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", "collected_at": publishedAt},
	}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("governed consent returned %d: %s", response.Code, response.Body.String())
	}
	consent := decodeResponse(t, response)
	receiptID, _ := consent["receipt_id"].(string)
	if receiptID == "" {
		t.Fatalf("consent response lacked receipt: %v", consent)
	}

	response = request(t, handler, http.MethodGet, "/v1/receipts/"+receiptID, nil, true)
	if response.Code != http.StatusOK || decodeResponse(t, response)["receipt_hash"] == "" {
		t.Fatalf("receipt endpoint returned %d: %s", response.Code, response.Body.String())
	}

	response = request(t, handler, http.MethodPost, "/v1/evaluate", map[string]any{"subject_id": "cus-1", "purpose": "marketing", "channel": "email"}, true)
	if response.Code != http.StatusOK || decodeResponse(t, response)["allowed"] != true {
		t.Fatalf("evaluation returned %d: %s", response.Code, response.Body.String())
	}

	response = request(t, handler, http.MethodPost, "/v1/suppressions", map[string]any{"subject_id": "cus-1", "scope": "channel", "channel": "email", "reason": "opt_out", "active": true}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("suppression returned %d: %s", response.Code, response.Body.String())
	}
	response = request(t, handler, http.MethodPost, "/v1/evaluate:batch", map[string]any{"requests": []map[string]any{{"subject_id": "cus-1", "purpose": "marketing", "channel": "email"}}}, true)
	decisions, _ := decodeResponse(t, response)["decisions"].([]any)
	if response.Code != http.StatusOK || len(decisions) != 1 || decisions[0].(map[string]any)["reason"] != "channel_suppression" {
		t.Fatalf("batch evaluation returned %d: %s", response.Code, response.Body.String())
	}
}

func TestSelfServiceJWTIsBoundToCanonicalCustomerSubject(t *testing.T) {
	const secret = "self-service-test-secret"
	t.Setenv("JWT_HMAC_SECRET", secret)
	t.Setenv("ALLOW_HMAC_AUTH", "true")
	t.Setenv("ALLOW_DEV_HEADERS", "false")
	claims := auth.Claims{
		TenantID:  "tenant-self",
		SubjectID: "cus-self",
		Roles:     []string{"self_service"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "authentik-user",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	handler := newMux(service.New(store.NewMemory()))
	evaluate := func(subjectID string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"subject_id": subjectID, "purpose": "marketing", "channel": "email"})
		req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	if response := evaluate("cus-other"); response.Code != http.StatusForbidden {
		t.Fatalf("cross-subject self-service returned %d: %s", response.Code, response.Body.String())
	}
	if response := evaluate("cus-self"); response.Code != http.StatusOK || decodeResponse(t, response)["reason"] != "no_consent" {
		t.Fatalf("own-subject self-service returned %d: %s", response.Code, response.Body.String())
	}
}
