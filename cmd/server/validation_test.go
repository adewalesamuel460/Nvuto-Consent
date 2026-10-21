package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// AT-027 / T-027: Strict Request Parsing, Media Types, and Body Bounds.
func TestStrictRequestParsingAndMediaTypes(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	repo := store.NewMemory()
	svc := service.New(repo)
	handler := newMux(svc)

	const validHash = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

	t.Run("Trailing JSON rejected without mutating state", func(t *testing.T) {
		payload := `{"code":"newsletter","category":"marketing","allowed_channels":["email"],"requires_affirmative_consent":true,"requires_double_opt_in":false} {"trailing":true}`
		req := httptest.NewRequest(http.MethodPost, "/v1/purposes", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-admin")
		req.Header.Set("X-Nvuto-Roles", "privacy_admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("trailing JSON returned %d, want 400: %s", w.Code, w.Body.String())
		}
		var errResp errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error body: %v", err)
		}
		if errResp.Error != "trailing_data" {
			t.Fatalf("error code = %q, want trailing_data", errResp.Error)
		}
		purposes, total, err := svc.ListPurposes(context.Background(), "tenant-test", 10, 0)
		if err != nil || total != 0 || len(purposes) != 0 {
			t.Fatalf("state mutated on rejected trailing JSON: total=%d, purposes=%v", total, purposes)
		}
	})

	t.Run("Malformed JSON bodies rejected without mutating state", func(t *testing.T) {
		malformedCases := []struct {
			name     string
			body     string
			wantCode string
		}{
			{"truncated JSON", `{"code":"newslett`, "invalid_json"},
			{"plain text", `not a json object`, "invalid_json"},
			{"top-level array", `[{"code":"test"}]`, "invalid_json"},
			{"top-level null", `null`, "invalid_json"},
		}

		for _, tc := range malformedCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "/v1/purposes", strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
				req.Header.Set("X-Nvuto-Actor-ID", "actor-admin")
				req.Header.Set("X-Nvuto-Roles", "privacy_admin")
				w := httptest.NewRecorder()

				handler.ServeHTTP(w, req)

				if w.Code != http.StatusBadRequest {
					t.Fatalf("%s returned %d, want 400: %s", tc.name, w.Code, w.Body.String())
				}
				var errResp errorBody
				_ = json.Unmarshal(w.Body.Bytes(), &errResp)
				if errResp.Error != tc.wantCode {
					t.Fatalf("%s error code = %q, want %q", tc.name, errResp.Error, tc.wantCode)
				}
			})
		}
	})

	t.Run("Oversized payload rejected without mutating state", func(t *testing.T) {
		// maxBodyBytes is 1MB (1048576 bytes). Send 1MB + 2KB of payload.
		oversized := `{"code":"news","category":"marketing","allowed_channels":["email"],"requires_affirmative_consent":true,"requires_double_opt_in":false,"pad":"` + strings.Repeat("A", 1048576+2048) + `"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/purposes", strings.NewReader(oversized))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-admin")
		req.Header.Set("X-Nvuto-Roles", "privacy_admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("oversized payload returned %d, want 413: %s", w.Code, w.Body.String())
		}
		var errResp errorBody
		_ = json.Unmarshal(w.Body.Bytes(), &errResp)
		if errResp.Error != "body_too_large" {
			t.Fatalf("error code = %q, want body_too_large", errResp.Error)
		}
	})

	t.Run("Content-Type enforcement on mutation endpoints", func(t *testing.T) {
		mutationEndpoints := []struct {
			name  string
			path  string
			body  string
			roles string
		}{
			{"create purpose", "/v1/purposes", `{"code":"news","category":"marketing","allowed_channels":["email"],"requires_affirmative_consent":true,"requires_double_opt_in":false}`, "privacy_admin"},
			{"create lawful basis", "/v1/lawful-bases", `{"code":"legit","allowed_categories":["marketing"],"requires_evidence":false}`, "privacy_admin"},
			{"create notice", "/v1/notices", `{"version":"2026-10","jurisdiction":"NG","published_at":"2026-10-01T00:00:00Z"}`, "privacy_admin"},
			{"create consent", "/v1/consents", `{"subject_id":"cus-ct-1","purpose":"news","channel":"email","legal_basis":"legit","status":"granted","source":"web","jurisdiction":"NG","notice_version":"2026-10"}`, "consent_writer"},
			{"create suppression", "/v1/suppressions", `{"subject_id":"cus-ct-1","scope":"global","reason":"opt_out"}`, "consent_writer"},
			{"double opt-in challenge", "/v1/double-opt-in", `{"subject_id":"cus-ct-1","purpose":"news","channel":"email","source":"web","jurisdiction":"NG","notice_version":"2026-10","evidence":{"method":"form","artifact_hash":"` + validHash + `","collected_at":"2026-10-02T00:00:00Z"},"expires_at":"2026-11-01T00:00:00Z"}`, "consent_writer"},
		}

		for _, me := range mutationEndpoints {
			t.Run(me.name+" missing Content-Type", func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, me.path, strings.NewReader(me.body))
				req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
				req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
				req.Header.Set("X-Nvuto-Roles", me.roles)
				w := httptest.NewRecorder()

				handler.ServeHTTP(w, req)

				if w.Code != http.StatusUnsupportedMediaType {
					t.Fatalf("%s without Content-Type returned %d, want 415: %s", me.name, w.Code, w.Body.String())
				}
				var errResp errorBody
				_ = json.Unmarshal(w.Body.Bytes(), &errResp)
				if errResp.Error != "unsupported_media_type" {
					t.Fatalf("%s error = %q, want unsupported_media_type", me.name, errResp.Error)
				}
			})

			t.Run(me.name+" invalid Content-Type text/plain", func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, me.path, strings.NewReader(me.body))
				req.Header.Set("Content-Type", "text/plain")
				req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
				req.Header.Set("X-Nvuto-Actor-ID", "actor-test")
				req.Header.Set("X-Nvuto-Roles", me.roles)
				w := httptest.NewRecorder()

				handler.ServeHTTP(w, req)

				if w.Code != http.StatusUnsupportedMediaType {
					t.Fatalf("%s with text/plain returned %d, want 415: %s", me.name, w.Code, w.Body.String())
				}
			})
		}
	})

	t.Run("Valid Content-Type with charset accepted", func(t *testing.T) {
		payload := `{"code":"blog","category":"marketing","allowed_channels":["email"],"requires_affirmative_consent":true,"requires_double_opt_in":false}`
		req := httptest.NewRequest(http.MethodPost, "/v1/purposes", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("X-Nvuto-Tenant-ID", "tenant-test")
		req.Header.Set("X-Nvuto-Actor-ID", "actor-admin")
		req.Header.Set("X-Nvuto-Roles", "privacy_admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("application/json; charset=utf-8 returned %d, want 201: %s", w.Code, w.Body.String())
		}
	})
}

// AT-027 / T-027: Boolean Defaults (active defaults to true when omitted).
func TestBooleanDefaultsActiveFlag(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	repo := store.NewMemory()
	svc := service.New(repo)
	handler := newMux(svc)

	tenant := "tenant-bool-test"

	t.Run("Purpose creation defaults active to true when omitted", func(t *testing.T) {
		body := `{"code":"telemetry","category":"analytics","allowed_channels":["email"],"requires_affirmative_consent":true,"requires_double_opt_in":false}`
		req := httptest.NewRequest(http.MethodPost, "/v1/purposes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenant)
		req.Header.Set("X-Nvuto-Actor-ID", "admin")
		req.Header.Set("X-Nvuto-Roles", "privacy_admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("create purpose without active returned %d: %s", w.Code, w.Body.String())
		}
		var created domain.Purpose
		if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		if !created.Active {
			t.Fatalf("expected created.Active to be true, got false")
		}

		stored, err := repo.GetPurpose(context.Background(), tenant, "telemetry")
		if err != nil || !stored.Active {
			t.Fatalf("stored purpose Active = %v, err = %v, want true", stored.Active, err)
		}
	})

	t.Run("Purpose creation respects explicit active false", func(t *testing.T) {
		body := `{"code":"experiments","category":"analytics","allowed_channels":["email"],"requires_affirmative_consent":true,"requires_double_opt_in":false,"active":false}`
		req := httptest.NewRequest(http.MethodPost, "/v1/purposes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenant)
		req.Header.Set("X-Nvuto-Actor-ID", "admin")
		req.Header.Set("X-Nvuto-Roles", "privacy_admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("create purpose with active=false returned %d: %s", w.Code, w.Body.String())
		}
		var created domain.Purpose
		_ = json.Unmarshal(w.Body.Bytes(), &created)
		if created.Active {
			t.Fatalf("expected created.Active to be false, got true")
		}

		stored, err := repo.GetPurpose(context.Background(), tenant, "experiments")
		if err != nil || stored.Active {
			t.Fatalf("stored purpose Active = %v, err = %v, want false", stored.Active, err)
		}
	})

	t.Run("Notice creation defaults active to true when omitted", func(t *testing.T) {
		body := `{"version":"2026-v1","jurisdiction":"NG","published_at":"2026-10-01T00:00:00Z"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/notices", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenant)
		req.Header.Set("X-Nvuto-Actor-ID", "admin")
		req.Header.Set("X-Nvuto-Roles", "privacy_admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("create notice without active returned %d: %s", w.Code, w.Body.String())
		}
		var created domain.PrivacyNotice
		_ = json.Unmarshal(w.Body.Bytes(), &created)
		if !created.Active {
			t.Fatalf("expected created notice Active to be true, got false")
		}

		stored, err := repo.GetNotice(context.Background(), tenant, "NG", "2026-v1")
		if err != nil || !stored.Active {
			t.Fatalf("stored notice Active = %v, err = %v, want true", stored.Active, err)
		}
	})

	t.Run("Notice creation respects explicit active false", func(t *testing.T) {
		body := `{"version":"2026-v2","jurisdiction":"NG","published_at":"2026-10-01T00:00:00Z","active":false}`
		req := httptest.NewRequest(http.MethodPost, "/v1/notices", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenant)
		req.Header.Set("X-Nvuto-Actor-ID", "admin")
		req.Header.Set("X-Nvuto-Roles", "privacy_admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("create notice with active=false returned %d: %s", w.Code, w.Body.String())
		}
		var created domain.PrivacyNotice
		_ = json.Unmarshal(w.Body.Bytes(), &created)
		if created.Active {
			t.Fatalf("expected created notice Active to be false, got true")
		}
	})

	t.Run("Lawful basis creation defaults active to true when omitted", func(t *testing.T) {
		body := `{"code":"public_interest","allowed_categories":["analytics"],"requires_evidence":false}`
		req := httptest.NewRequest(http.MethodPost, "/v1/lawful-bases", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenant)
		req.Header.Set("X-Nvuto-Actor-ID", "admin")
		req.Header.Set("X-Nvuto-Roles", "privacy_admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("create lawful basis without active returned %d: %s", w.Code, w.Body.String())
		}
		var created domain.LawfulBasis
		_ = json.Unmarshal(w.Body.Bytes(), &created)
		if !created.Active {
			t.Fatalf("expected created lawful basis Active to be true, got false")
		}
	})

	t.Run("Suppression creation defaults active to true when omitted", func(t *testing.T) {
		body := `{"subject_id":"sub-defaults-test","scope":"global","reason":"user_opt_out"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/suppressions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenant)
		req.Header.Set("X-Nvuto-Actor-ID", "admin")
		req.Header.Set("X-Nvuto-Roles", "privacy_admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("create suppression without active returned %d: %s", w.Code, w.Body.String())
		}
		var created domain.Suppression
		_ = json.Unmarshal(w.Body.Bytes(), &created)
		if !created.Active {
			t.Fatalf("expected created suppression Active to be true, got false")
		}
	})
}

// AT-027 / T-027: Opt-Out Resilience (Denial and Withdrawal Resilience).
func TestOptOutResilience(t *testing.T) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	repo := store.NewMemory()
	svc := service.New(repo)
	handler := newMux(svc)

	tenant := "tenant-optout"
	actor := "admin-actor"
	validHash := "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

	// 1. Initial setup: active purpose, lawful basis, notice.
	pubTime := time.Now().UTC().Add(-time.Hour)
	if _, err := svc.RegisterPurpose(context.Background(), tenant, actor, domain.Purpose{
		Code:                       "direct_marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		Active:                     true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterLawfulBasis(context.Background(), tenant, actor, domain.LawfulBasis{
		Code:              "consent",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishNotice(context.Background(), tenant, actor, domain.PrivacyNotice{
		Version:      "2026-10",
		Jurisdiction: "NG",
		PublishedAt:  pubTime,
		Active:       true,
	}); err != nil {
		t.Fatal(err)
	}

	// 2. Record granted consent for cus-withdraw-1.
	evidence := domain.Evidence{
		Method:       "opt_in_form",
		ArtifactHash: validHash,
		CollectedAt:  pubTime.Add(time.Minute),
	}
	grantConsent, err := svc.RecordGoverned(context.Background(), tenant, actor, "cus-withdraw-1", "direct_marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-10", evidence)
	if err != nil {
		t.Fatalf("recording initial grant failed: %v", err)
	}
	if grantConsent.Status != domain.StatusGranted {
		t.Fatalf("grant status = %v, want granted", grantConsent.Status)
	}

	// 3. Deactivate the purpose: active = false.
	if _, err := svc.RegisterPurpose(context.Background(), tenant, actor, domain.Purpose{
		Code:                       "direct_marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		Active:                     false,
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("User can successfully withdraw consent for deactivated purpose (active = false)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/consents/%s/withdraw", grantConsent.ID), nil)
		req.Header.Set("X-Nvuto-Tenant-ID", tenant)
		req.Header.Set("X-Nvuto-Actor-ID", "actor-user")
		req.Header.Set("X-Nvuto-Roles", "consent_writer")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("withdraw for deactivated purpose returned %d: %s", w.Code, w.Body.String())
		}
		var withdrawn domain.Consent
		if err := json.Unmarshal(w.Body.Bytes(), &withdrawn); err != nil {
			t.Fatal(err)
		}
		if withdrawn.Status != domain.StatusWithdrawn {
			t.Fatalf("withdrawn consent status = %q, want %q", withdrawn.Status, domain.StatusWithdrawn)
		}

		// Evaluation must now fail closed with withdrawn status.
		decision, err := svc.Evaluate(context.Background(), tenant, "cus-withdraw-1", "direct_marketing", "email", time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if decision.Allowed {
			t.Fatalf("evaluation after withdrawal must deny permission, got allowed=true")
		}
	})

	t.Run("Recording a denial succeeds when purpose is deactivated", func(t *testing.T) {
		// Purpose 'direct_marketing' is active = false.
		body := fmt.Sprintf(`{
			"subject_id": "cus-denial-1",
			"purpose": "direct_marketing",
			"channel": "email",
			"legal_basis": "consent",
			"status": "denied",
			"source": "privacy_center",
			"jurisdiction": "NG",
			"notice_version": "2026-10"
		}`)

		req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenant)
		req.Header.Set("X-Nvuto-Actor-ID", "actor-user")
		req.Header.Set("X-Nvuto-Roles", "consent_writer")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("recording denial for deactivated purpose returned %d: %s", w.Code, w.Body.String())
		}
		var denied domain.Consent
		if err := json.Unmarshal(w.Body.Bytes(), &denied); err != nil {
			t.Fatal(err)
		}
		if denied.Status != domain.StatusDenied {
			t.Fatalf("denial status = %q, want denied", denied.Status)
		}

		decision, err := svc.Evaluate(context.Background(), tenant, "cus-denial-1", "direct_marketing", "email", time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if decision.Allowed {
			t.Fatalf("evaluation after denial must deny permission")
		}
	})

	t.Run("Recording a denial succeeds when notice is deactivated and evidence is missing/invalid", func(t *testing.T) {
		// Publish inactive notice 2026-inactive with active: false.
		if _, err := svc.PublishNotice(context.Background(), tenant, actor, domain.PrivacyNotice{
			Version:      "2026-inactive",
			Jurisdiction: "NG",
			PublishedAt:  pubTime,
			Active:       false,
		}); err != nil {
			t.Fatal(err)
		}

		// Denial with missing evidence and deactivated notice.
		bodyMissingEvidence := `{
			"subject_id": "cus-denial-2",
			"purpose": "direct_marketing",
			"channel": "email",
			"legal_basis": "consent",
			"status": "denied",
			"source": "privacy_center",
			"jurisdiction": "NG",
			"notice_version": "2026-inactive"
		}`

		req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(bodyMissingEvidence))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenant)
		req.Header.Set("X-Nvuto-Actor-ID", "actor-user")
		req.Header.Set("X-Nvuto-Roles", "consent_writer")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("recording denial with missing evidence returned %d: %s", w.Code, w.Body.String())
		}

		// Denial with invalid artifact hash format and deactivated notice.
		bodyInvalidEvidence := `{
			"subject_id": "cus-denial-3",
			"purpose": "direct_marketing",
			"channel": "email",
			"legal_basis": "consent",
			"status": "denied",
			"source": "privacy_center",
			"jurisdiction": "NG",
			"notice_version": "2026-inactive",
			"evidence": {
				"method": "bad_method",
				"artifact_hash": "not-a-valid-sha256-hash",
				"collected_at": "2099-01-01T00:00:00Z"
			}
		}`

		req = httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(bodyInvalidEvidence))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenant)
		req.Header.Set("X-Nvuto-Actor-ID", "actor-user")
		req.Header.Set("X-Nvuto-Roles", "consent_writer")
		w = httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("recording denial with invalid evidence returned %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Grant is strictly rejected under deactivated policy or invalid evidence", func(t *testing.T) {
		// Attempting a grant under deactivated purpose/notice or invalid evidence must be rejected.
		grantBody := `{
			"subject_id": "cus-grant-fail",
			"purpose": "direct_marketing",
			"channel": "email",
			"legal_basis": "consent",
			"status": "granted",
			"source": "privacy_center",
			"jurisdiction": "NG",
			"notice_version": "2026-10",
			"evidence": {
				"method": "form",
				"artifact_hash": "not-valid-hash",
				"collected_at": "2026-10-01T00:00:00Z"
			}
		}`

		req := httptest.NewRequest(http.MethodPost, "/v1/consents", strings.NewReader(grantBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenant)
		req.Header.Set("X-Nvuto-Actor-ID", "actor-user")
		req.Header.Set("X-Nvuto-Roles", "consent_writer")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code == http.StatusCreated {
			t.Fatalf("grant under deactivated policy and invalid evidence must not succeed, got 201")
		}
	})
}
