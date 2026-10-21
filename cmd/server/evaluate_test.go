package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// setupEvaluateFixture creates the purpose, lawful basis, notice and returns a ready service and router.
func setupEvaluateFixture(t *testing.T, repo store.Repository, tenantID string) (*service.Service, http.Handler, domain.PrivacyNotice, domain.Evidence) {
	t.Helper()
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	svc, notice, evidence := setupAT005Fixture(t, repo, tenantID)
	handler := newMux(svc)
	return svc, handler, notice, evidence
}

// TestAT006_EvaluateClockSkewValidation verifies that passing an at parameter
// diverging from current time by more than 1 minute (in past or future) is
// strictly rejected with HTTP 400 Bad Request and ErrUnsupportedHistoricalEvaluation.
func TestAT006_EvaluateClockSkewValidation(t *testing.T) {
	tenantID := "tenant-skew"
	repo := store.NewMemory()
	svc, handler, _, evidence := setupEvaluateFixture(t, repo, tenantID)

	ctx := context.Background()
	now := time.Now().UTC()
	// Record an active consent
	consent, err := svc.RecordGoverned(ctx, tenantID, "actor-admin", "subj-skew-1", "marketing", "email", "consent", "granted", "web", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("failed to record consent: %v", err)
	}
	if consent.Status != domain.StatusGranted {
		t.Fatalf("expected granted status, got %s", consent.Status)
	}
	// Backdate GrantedAt to 45s ago so that nearPast (30s ago) evaluates after grant time
	pastGrant := now.Add(-45 * time.Second)
	consent.GrantedAt = &pastGrant
	repo.SeedConsent(consent)

	t.Run("Deeply in past (> 1 minute) rejected via HTTP", func(t *testing.T) {
		pastTime := now.Add(-2 * time.Minute)
		payload := map[string]any{
			"subject_id": "subj-skew-1",
			"purpose":    "marketing",
			"channel":    "email",
			"at":         pastTime,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
		req.Header.Set("X-Nvuto-Actor-ID", "actor-admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected HTTP 400 for historical query, got %d: %s", w.Code, w.Body.String())
		}
		var errResp errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error body: %v", err)
		}
		if errResp.Error != "unsupported_historical_evaluation" {
			t.Fatalf("expected error code 'unsupported_historical_evaluation', got %q", errResp.Error)
		}
		if errResp.Field != "at" {
			t.Fatalf("expected field 'at', got %q", errResp.Field)
		}
	})

	t.Run("Deeply in past (> 1 minute) rejected via Service method", func(t *testing.T) {
		pastTime := now.Add(-5 * time.Minute)
		_, err := svc.Evaluate(ctx, tenantID, "subj-skew-1", "marketing", "email", pastTime)
		if err == nil {
			t.Fatal("expected error for historical evaluation, got nil")
		}
		if !errors.Is(err, service.ErrUnsupportedHistoricalEvaluation) {
			t.Fatalf("expected ErrUnsupportedHistoricalEvaluation, got %v", err)
		}
		var typed *domain.Error
		if errors.As(err, &typed) {
			if typed.Code() != "unsupported_historical_evaluation" {
				t.Fatalf("expected code 'unsupported_historical_evaluation', got %q", typed.Code())
			}
			if typed.Field() != "at" {
				t.Fatalf("expected field 'at', got %q", typed.Field())
			}
		}
	})

	t.Run("Deeply in future (> 1 minute) rejected via HTTP", func(t *testing.T) {
		futureTime := now.Add(3 * time.Minute)
		payload := map[string]any{
			"subject_id": "subj-skew-1",
			"purpose":    "marketing",
			"channel":    "email",
			"at":         futureTime,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
		req.Header.Set("X-Nvuto-Actor-ID", "actor-admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected HTTP 400 for future query, got %d: %s", w.Code, w.Body.String())
		}
		var errResp errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error body: %v", err)
		}
		if errResp.Error != "unsupported_historical_evaluation" {
			t.Fatalf("expected error code 'unsupported_historical_evaluation', got %q", errResp.Error)
		}
	})

	t.Run("Deeply in future (> 1 minute) rejected via Service method", func(t *testing.T) {
		futureTime := now.Add(10 * time.Minute)
		_, err := svc.Evaluate(ctx, tenantID, "subj-skew-1", "marketing", "email", futureTime)
		if err == nil {
			t.Fatal("expected error for future evaluation, got nil")
		}
		if !errors.Is(err, service.ErrUnsupportedHistoricalEvaluation) {
			t.Fatalf("expected ErrUnsupportedHistoricalEvaluation, got %v", err)
		}
	})

	t.Run("Batch evaluate rejects historical at with field prefix", func(t *testing.T) {
		pastTime := now.Add(-10 * time.Minute)
		payload := map[string]any{
			"requests": []map[string]any{
				{
					"subject_id": "subj-skew-1",
					"purpose":    "marketing",
					"channel":    "email",
					"at":         pastTime,
				},
			},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/v1/evaluate:batch", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
		req.Header.Set("X-Nvuto-Actor-ID", "actor-admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected HTTP 400 for batch historical query, got %d: %s", w.Code, w.Body.String())
		}
		var errResp errorBody
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error body: %v", err)
		}
		if errResp.Error != "unsupported_historical_evaluation" {
			t.Fatalf("expected error code 'unsupported_historical_evaluation', got %q", errResp.Error)
		}
		if errResp.Field != "requests[0].at" {
			t.Fatalf("expected field 'requests[0].at', got %q", errResp.Field)
		}
	})

	t.Run("Valid near-instant at within 1 minute skew accepted", func(t *testing.T) {
		nearPast := time.Now().UTC().Add(-30 * time.Second)
		decision, err := svc.Evaluate(ctx, tenantID, "subj-skew-1", "marketing", "email", nearPast)
		if err != nil {
			t.Fatalf("evaluation within 30s past skew margin must succeed, got %v", err)
		}
		if !decision.Allowed {
			t.Fatalf("expected allowed decision, got %+v", decision)
		}

		nearFuture := time.Now().UTC().Add(30 * time.Second)
		decision, err = svc.Evaluate(ctx, tenantID, "subj-skew-1", "marketing", "email", nearFuture)
		if err != nil {
			t.Fatalf("evaluation within 30s future skew margin must succeed, got %v", err)
		}
		if !decision.Allowed {
			t.Fatalf("expected allowed decision, got %+v", decision)
		}
	})

	t.Run("Omitted at defaults to current time and succeeds", func(t *testing.T) {
		payload := map[string]any{
			"subject_id": "subj-skew-1",
			"purpose":    "marketing",
			"channel":    "email",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
		req.Header.Set("X-Nvuto-Actor-ID", "actor-admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200 for omitted at, got %d: %s", w.Code, w.Body.String())
		}
		var decision domain.SuppressionDecision
		if err := json.Unmarshal(w.Body.Bytes(), &decision); err != nil {
			t.Fatalf("failed to decode decision body: %v", err)
		}
		if !decision.Allowed {
			t.Fatalf("expected allowed decision, got %+v", decision)
		}
	})
}

// TestAT006_EvaluateGrantedAtEffectiveTime proves that evaluation never claims
// permission before the grant's effective time (e.g. if GrantedAt is exactly now,
// evaluating at now - 1s returns denied/not found with reason not_yet_granted).
func TestAT006_EvaluateGrantedAtEffectiveTime(t *testing.T) {
	tenantID := "tenant-effective-time"
	repo := store.NewMemory()
	svc, handler, _, evidence := setupEvaluateFixture(t, repo, tenantID)

	ctx := context.Background()
	consent, err := svc.RecordGoverned(ctx, tenantID, "actor-admin", "subj-eff-1", "marketing", "email", "consent", "granted", "web", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("failed to record consent: %v", err)
	}
	if consent.GrantedAt == nil {
		t.Fatal("consent GrantedAt must not be nil")
	}
	grantTime := *consent.GrantedAt

	t.Run("Evaluating 1s before GrantedAt returns not_yet_granted via Service", func(t *testing.T) {
		beforeGrant := grantTime.Add(-1 * time.Second)
		decision, err := svc.Evaluate(ctx, tenantID, "subj-eff-1", "marketing", "email", beforeGrant)
		if err != nil {
			t.Fatalf("expected successful evaluation query, got error %v", err)
		}
		if decision.Allowed {
			t.Fatalf("evaluation before GrantedAt must NOT claim permission: %+v", decision)
		}
		if decision.Reason != "not_yet_granted" {
			t.Fatalf("expected reason 'not_yet_granted', got %q", decision.Reason)
		}
	})

	t.Run("Evaluating 1s before GrantedAt returns not_yet_granted via HTTP", func(t *testing.T) {
		beforeGrant := grantTime.Add(-1 * time.Second)
		payload := map[string]any{
			"subject_id": "subj-eff-1",
			"purpose":    "marketing",
			"channel":    "email",
			"at":         beforeGrant,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
		req.Header.Set("X-Nvuto-Actor-ID", "actor-admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
		}
		var decision domain.SuppressionDecision
		if err := json.Unmarshal(w.Body.Bytes(), &decision); err != nil {
			t.Fatalf("failed to decode decision body: %v", err)
		}
		if decision.Allowed {
			t.Fatalf("HTTP evaluation before GrantedAt must NOT claim permission: %+v", decision)
		}
		if decision.Reason != "not_yet_granted" {
			t.Fatalf("expected reason 'not_yet_granted', got %q", decision.Reason)
		}
	})

	t.Run("Evaluating at or after GrantedAt returns granted", func(t *testing.T) {
		decisionAt, err := svc.Evaluate(ctx, tenantID, "subj-eff-1", "marketing", "email", grantTime)
		if err != nil {
			t.Fatalf("evaluation at grantTime failed: %v", err)
		}
		if !decisionAt.Allowed || decisionAt.Reason != "granted" {
			t.Fatalf("expected allowed decision at grantTime, got %+v", decisionAt)
		}

		afterGrant := grantTime.Add(2 * time.Second)
		decisionAfter, err := svc.Evaluate(ctx, tenantID, "subj-eff-1", "marketing", "email", afterGrant)
		if err != nil {
			t.Fatalf("evaluation after grantTime failed: %v", err)
		}
		if !decisionAfter.Allowed || decisionAfter.Reason != "granted" {
			t.Fatalf("expected allowed decision after grantTime, got %+v", decisionAfter)
		}
	})
}

// TestAT006_EvaluateDeterministicTieBreaking proves that given two consent records
// with identical updated_at timestamps, the system consistently prioritizes the one
// with the higher preference_generation, then falls back to descending id order.
func TestAT006_EvaluateDeterministicTieBreaking(t *testing.T) {
	tenantID := "tenant-tie"
	fixedTime := time.Now().UTC().Add(-10 * time.Second)
	ctx := context.Background()
	evidence := domain.Evidence{
		Method:       "web_form",
		ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		CollectedAt:  fixedTime,
	}

	t.Run("PreferenceGeneration prioritized over same UpdatedAt", func(t *testing.T) {
		// Older generation is granted, newer generation is withdrawn.
		// Both share the exact same UpdatedAt timestamp.
		cOlder := domain.Consent{
			ID:                   "con_older_gen",
			TenantID:             tenantID,
			SubjectID:            "subj-tie-1",
			Purpose:              "marketing",
			Channel:              "email",
			LegalBasis:           "consent",
			Status:               domain.StatusGranted,
			Source:               "web",
			Jurisdiction:         "NG",
			NoticeVersion:        "2026-08",
			Version:              1,
			PreferenceGeneration: 1,
			GrantedAt:            &fixedTime,
			CreatedAt:            fixedTime,
			UpdatedAt:            fixedTime,
		}
		cNewer := domain.Consent{
			ID:                   "con_newer_gen",
			TenantID:             tenantID,
			SubjectID:            "subj-tie-1",
			Purpose:              "marketing",
			Channel:              "email",
			LegalBasis:           "consent",
			Status:               domain.StatusWithdrawn,
			Source:               "web",
			Jurisdiction:         "NG",
			NoticeVersion:        "2026-08",
			Version:              2,
			PreferenceGeneration: 2,
			WithdrawnAt:          &fixedTime,
			CreatedAt:            fixedTime,
			UpdatedAt:            fixedTime,
		}

		// Test Order A: seed older first, then newer
		repoA := store.NewMemory()
		svcA, _, _, _ := setupEvaluateFixture(t, repoA, tenantID)
		repoA.SeedConsent(cOlder)
		repoA.SeedConsent(cNewer)

		effectiveA, err := repoA.FindEffective(ctx, tenantID, "subj-tie-1", "marketing", "email")
		if err != nil {
			t.Fatalf("FindEffective failed: %v", err)
		}
		if effectiveA.ID != "con_newer_gen" || effectiveA.PreferenceGeneration != 2 {
			t.Fatalf("expected higher preference_generation con_newer_gen, got %+v", effectiveA)
		}
		decisionA, err := svcA.Evaluate(ctx, tenantID, "subj-tie-1", "marketing", "email", time.Now().UTC())
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}
		if decisionA.Allowed || decisionA.Reason != "withdrawn" {
			t.Fatalf("expected withdrawn decision from newer preference_generation, got %+v", decisionA)
		}

		// Test Order B: seed newer first, then older (verifying map iteration independence)
		repoB := store.NewMemory()
		svcB, _, _, _ := setupEvaluateFixture(t, repoB, tenantID)
		repoB.SeedConsent(cNewer)
		repoB.SeedConsent(cOlder)

		effectiveB, err := repoB.FindEffective(ctx, tenantID, "subj-tie-1", "marketing", "email")
		if err != nil {
			t.Fatalf("FindEffective failed: %v", err)
		}
		if effectiveB.ID != "con_newer_gen" || effectiveB.PreferenceGeneration != 2 {
			t.Fatalf("expected higher preference_generation con_newer_gen regardless of insertion order, got %+v", effectiveB)
		}
		decisionB, err := svcB.Evaluate(ctx, tenantID, "subj-tie-1", "marketing", "email", time.Now().UTC())
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}
		if decisionB.Allowed || decisionB.Reason != "withdrawn" {
			t.Fatalf("expected withdrawn decision regardless of insertion order, got %+v", decisionB)
		}
	})

	t.Run("Descending ID breaks ties when PreferenceGeneration and UpdatedAt are identical", func(t *testing.T) {
		// Both records have PreferenceGeneration = 1 and UpdatedAt = fixedTime.
		// "con_omega" > "con_alpha", so "con_omega" must win deterministically.
		cAlpha := domain.Consent{
			ID:                   "con_alpha",
			TenantID:             tenantID,
			SubjectID:            "subj-tie-2",
			Purpose:              "marketing",
			Channel:              "email",
			LegalBasis:           "consent",
			Status:               domain.StatusDenied,
			Source:               "web",
			Jurisdiction:         "NG",
			NoticeVersion:        "2026-08",
			Evidence:             evidence,
			Version:              1,
			PreferenceGeneration: 1,
			DeniedAt:             &fixedTime,
			CreatedAt:            fixedTime,
			UpdatedAt:            fixedTime,
		}
		cOmega := domain.Consent{
			ID:                   "con_omega",
			TenantID:             tenantID,
			SubjectID:            "subj-tie-2",
			Purpose:              "marketing",
			Channel:              "email",
			LegalBasis:           "consent",
			Status:               domain.StatusGranted,
			Source:               "web",
			Jurisdiction:         "NG",
			NoticeVersion:        "2026-08",
			Evidence:             evidence,
			Version:              1,
			PreferenceGeneration: 1,
			GrantedAt:            &fixedTime,
			CreatedAt:            fixedTime,
			UpdatedAt:            fixedTime,
		}

		// Insert alpha first, then omega
		repo1 := store.NewMemory()
		svc1, _, _, _ := setupEvaluateFixture(t, repo1, tenantID)
		repo1.SeedConsent(cAlpha)
		repo1.SeedConsent(cOmega)

		effective1, err := repo1.FindEffective(ctx, tenantID, "subj-tie-2", "marketing", "email")
		if err != nil {
			t.Fatalf("FindEffective failed: %v", err)
		}
		if effective1.ID != "con_omega" {
			t.Fatalf("expected descending ID tie-breaker 'con_omega', got %q", effective1.ID)
		}
		decision1, err := svc1.Evaluate(ctx, tenantID, "subj-tie-2", "marketing", "email", time.Now().UTC())
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}
		if !decision1.Allowed || decision1.Reason != "granted" {
			t.Fatalf("expected granted from winning con_omega, got %+v", decision1)
		}

		// Insert omega first, then alpha
		repo2 := store.NewMemory()
		svc2, _, _, _ := setupEvaluateFixture(t, repo2, tenantID)
		repo2.SeedConsent(cOmega)
		repo2.SeedConsent(cAlpha)

		effective2, err := repo2.FindEffective(ctx, tenantID, "subj-tie-2", "marketing", "email")
		if err != nil {
			t.Fatalf("FindEffective failed: %v", err)
		}
		if effective2.ID != "con_omega" {
			t.Fatalf("expected descending ID tie-breaker 'con_omega' regardless of insertion order, got %q", effective2.ID)
		}
		decision2, err := svc2.Evaluate(ctx, tenantID, "subj-tie-2", "marketing", "email", time.Now().UTC())
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}
		if !decision2.Allowed || decision2.Reason != "granted" {
			t.Fatalf("expected granted from winning con_omega, got %+v", decision2)
		}
	})

	t.Run("Descending ID breaks ties when omega is withdrawn", func(t *testing.T) {
		// "con_omega" is withdrawn, "con_alpha" is granted. Both have identical generation and updated_at.
		cAlpha := domain.Consent{
			ID:                   "con_alpha",
			TenantID:             tenantID,
			SubjectID:            "subj-tie-3",
			Purpose:              "marketing",
			Channel:              "email",
			LegalBasis:           "consent",
			Status:               domain.StatusGranted,
			Source:               "web",
			Jurisdiction:         "NG",
			NoticeVersion:        "2026-08",
			Version:              1,
			PreferenceGeneration: 1,
			GrantedAt:            &fixedTime,
			CreatedAt:            fixedTime,
			UpdatedAt:            fixedTime,
		}
		cOmega := domain.Consent{
			ID:                   "con_omega",
			TenantID:             tenantID,
			SubjectID:            "subj-tie-3",
			Purpose:              "marketing",
			Channel:              "email",
			LegalBasis:           "consent",
			Status:               domain.StatusWithdrawn,
			Source:               "web",
			Jurisdiction:         "NG",
			NoticeVersion:        "2026-08",
			Version:              1,
			PreferenceGeneration: 1,
			WithdrawnAt:          &fixedTime,
			CreatedAt:            fixedTime,
			UpdatedAt:            fixedTime,
		}

		repo := store.NewMemory()
		svc, _, _, _ := setupEvaluateFixture(t, repo, tenantID)
		repo.SeedConsent(cAlpha)
		repo.SeedConsent(cOmega)

		effective, err := repo.FindEffective(ctx, tenantID, "subj-tie-3", "marketing", "email")
		if err != nil {
			t.Fatalf("FindEffective failed: %v", err)
		}
		if effective.ID != "con_omega" {
			t.Fatalf("expected con_omega to win, got %q", effective.ID)
		}
		decision, err := svc.Evaluate(ctx, tenantID, "subj-tie-3", "marketing", "email", time.Now().UTC())
		if err != nil {
			t.Fatalf("Evaluate failed: %v", err)
		}
		if decision.Allowed || decision.Reason != "withdrawn" {
			t.Fatalf("expected withdrawn from winning con_omega, got %+v", decision)
		}
	})

	t.Run("Postgres engine tie-breaking if configured", func(t *testing.T) {
		databaseURL := os.Getenv("TEST_DATABASE_URL")
		if databaseURL == "" {
			t.Skip("TEST_DATABASE_URL is not configured; skipping Postgres tie-breaking verification")
		}
		pgRepo, err := store.NewPostgres(ctx, databaseURL)
		if err != nil {
			t.Fatalf("failed to connect to postgres: %v", err)
		}
		defer pgRepo.Close()

		pgTenant := fmt.Sprintf("tenant-pg-tie-%d", time.Now().UnixNano())
		svcPG, _, _, evidence := setupEvaluateFixture(t, pgRepo, pgTenant)

		// Record first consent
		c1, err := svcPG.RecordGoverned(ctx, pgTenant, "admin", "subj-pg-tie", "marketing", "email", "consent", "granted", "web", nil, nil, "NG", "2026-08", evidence)
		if err != nil {
			t.Fatalf("record c1 failed: %v", err)
		}
		// Withdraw it -> advances generation
		c2, err := svcPG.Withdraw(ctx, pgTenant, "admin", c1.ID)
		if err != nil {
			t.Fatalf("withdraw failed: %v", err)
		}

		effective, err := pgRepo.FindEffective(ctx, pgTenant, "subj-pg-tie", "marketing", "email")
		if err != nil {
			t.Fatalf("postgres FindEffective failed: %v", err)
		}
		if effective.ID != c2.ID || effective.Status != domain.StatusWithdrawn {
			t.Fatalf("expected effective record %s with withdrawn status, got %+v", c2.ID, effective)
		}
	})
}
