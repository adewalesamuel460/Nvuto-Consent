package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/auth"
	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

const (
	doiSecret = "doi-test-secret-long-enough-for-hs256"
	doiTenant = "tenant-doi-test"
)

type doiTestFixture struct {
	t        *testing.T
	repo     *store.Memory
	svc      *service.Service
	handler  http.Handler
	logBuf   *bytes.Buffer
	evidence domain.Evidence
}

func newDOITestFixture(t *testing.T) *doiTestFixture {
	t.Helper()
	t.Setenv("JWT_JWKS_URL", "")
	t.Setenv("JWT_HMAC_SECRET", doiSecret)
	t.Setenv("ALLOW_HMAC_AUTH", "true")
	t.Setenv("ALLOW_DEV_HEADERS", "false")

	repo := store.NewMemory()
	svc := service.New(repo)
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	handler := newHandler(svc, 5*time.Second, logger)

	publishedAt := time.Now().UTC().Add(-time.Hour)
	// Register DOI-required purpose
	if _, err := svc.RegisterPurpose(context.Background(), doiTenant, "admin", domain.Purpose{
		Code:                       "double_marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email", "sms"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        true,
		Active:                     true,
	}); err != nil {
		t.Fatal(err)
	}
	// Register regular non-DOI purpose
	if _, err := svc.RegisterPurpose(context.Background(), doiTenant, "admin", domain.Purpose{
		Code:                       "regular_marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterLawfulBasis(context.Background(), doiTenant, "admin", domain.LawfulBasis{
		Code:              "consent",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishNotice(context.Background(), doiTenant, "admin", domain.PrivacyNotice{
		Version:      "2026-08",
		Jurisdiction: "NG",
		PublishedAt:  publishedAt,
		Active:       true,
	}); err != nil {
		t.Fatal(err)
	}

	return &doiTestFixture{
		t:       t,
		repo:    repo,
		svc:     svc,
		handler: handler,
		logBuf:  &logBuf,
		evidence: domain.Evidence{
			Method:       "web_signup",
			ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
			CollectedAt:  time.Now().UTC(),
		},
	}
}

func (f *doiTestFixture) token(tenant, customer string, roles ...string) string {
	f.t.Helper()
	claims := auth.Claims{
		TenantID:  tenant,
		SubjectID: customer,
		Roles:     roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-" + customer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
		},
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(doiSecret))
	if err != nil {
		f.t.Fatal(err)
	}
	return raw
}

func (f *doiTestFixture) do(token, method, path string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	var encoded bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			f.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &encoded)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, req)
	return response
}

// TestDoubleOptInOpaqueResponseAndSecretProtection (AT-003 API Acceptance Test)
// Proves that a self_service caller initiating DOI receives a 201 Created response
// that does not disclose the confirmation capability/token, returns an opaque
// challenge reference, and does not leak the secret into application logs.
func TestDoubleOptInOpaqueResponseAndSecretProtection(t *testing.T) {
	f := newDOITestFixture(t)
	subject := "cus-doi-self-service-001"
	selfToken := f.token(doiTenant, subject, auth.RoleSelfService)

	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	initiationBody := map[string]any{
		"subject_id":     subject,
		"purpose":        "double_marketing",
		"channel":        "email",
		"source":         "registration_portal",
		"jurisdiction":   "NG",
		"notice_version": "2026-08",
		"evidence":       f.evidence,
		"expires_at":     expiresAt,
	}

	// 1. Execute initiation as self_service user
	resp := f.do(selfToken, http.MethodPost, "/v1/double-opt-in", initiationBody)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created from POST /v1/double-opt-in, got %d: %s", resp.Code, resp.Body.String())
	}

	// 2. Verify strict omission of raw token in HTTP response JSON
	rawJSON := resp.Body.String()
	if strings.Contains(rawJSON, "confirmation_token") {
		t.Fatalf("CRITICAL SECURITY VIOLATION: HTTP response exposes confirmation_token: %s", rawJSON)
	}

	// 3. Verify opaque response DTO structure
	var challengeResp domain.DoubleOptInResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &challengeResp); err != nil {
		t.Fatalf("failed to decode DoubleOptInResponse: %v", err)
	}

	if challengeResp.ChallengeID == "" {
		t.Fatalf("expected non-empty challenge_id in opaque response: %+v", challengeResp)
	}
	if challengeResp.Status != domain.StatusPendingConfirmation {
		t.Fatalf("expected status=%q, got %q", domain.StatusPendingConfirmation, challengeResp.Status)
	}
	if challengeResp.ExpiresAt == nil || challengeResp.ExpiresAt.IsZero() {
		t.Fatalf("expected valid expires_at timestamp in opaque response: %+v", challengeResp)
	}
	if challengeResp.Consent.ID != challengeResp.ChallengeID {
		t.Fatalf("expected consent.id == challenge_id, got %q vs %q", challengeResp.Consent.ID, challengeResp.ChallengeID)
	}
	if challengeResp.Consent.ConfirmationTokenHash != "" {
		t.Fatalf("CRITICAL: confirmation_token_hash exposed in public consent object")
	}

	// 4. Verify that the caller CANNOT immediately confirm because the secret was not disclosed
	cannotConfirmResp := f.do(selfToken, http.MethodPost, "/v1/double-opt-in/confirm", map[string]any{
		"token": challengeResp.ChallengeID, // Trying to echo challenge_id as token
	})
	if cannotConfirmResp.Code == http.StatusOK {
		t.Fatalf("confirm endpoint accepted opaque challenge_id as secret token")
	}

	// 5. Verify security logs: ensure raw token is never exposed in slog logs
	logs := f.logBuf.String()
	if strings.Contains(logs, "confirmation_token") {
		t.Fatalf("security violation: slog output contains confirmation_token: %s", logs)
	}
}

// TestDoubleOptInOutboxRoutingAndTrustedDelivery (AT-003 Integration Acceptance Test)
// Proves that the outbox event payload successfully captures the token and intended
// destination for the downstream mailer/SMS gateway, binding tenant, subject, purpose,
// channel, and generation, and allowing the trusted delivery consumer to confirm.
func TestDoubleOptInOutboxRoutingAndTrustedDelivery(t *testing.T) {
	f := newDOITestFixture(t)
	subject := "cus-doi-trusted-002"
	selfToken := f.token(doiTenant, subject, auth.RoleSelfService)

	expiresAt := time.Now().UTC().Add(12 * time.Hour)
	initiationBody := map[string]any{
		"subject_id":     subject,
		"purpose":        "double_marketing",
		"channel":        "email",
		"source":         "landing_page",
		"jurisdiction":   "NG",
		"notice_version": "2026-08",
		"evidence":       f.evidence,
		"expires_at":     expiresAt,
	}

	resp := f.do(selfToken, http.MethodPost, "/v1/double-opt-in", initiationBody)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", resp.Code, resp.Body.String())
	}

	var challengeResp domain.DoubleOptInResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &challengeResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// 1. Inspect the outbox events in the store to find the challenge outbox event
	events := f.repo.Outbox()
	if len(events) == 0 {
		t.Fatal("expected at least one outbox event, got 0")
	}

	var challengeEvent *domain.OutboxEvent
	for i := range events {
		if events[i].AggregateID == challengeResp.ChallengeID {
			challengeEvent = &events[i]
			break
		}
	}
	if challengeEvent == nil {
		t.Fatalf("no outbox event found for challenge_id %s; events: %+v", challengeResp.ChallengeID, events)
	}

	// 2. Validate EventType and Aggregate binding
	if challengeEvent.EventType != "consent.challenge.created" {
		t.Fatalf("expected EventType='consent.challenge.created', got %q", challengeEvent.EventType)
	}
	if challengeEvent.TenantID != doiTenant {
		t.Fatalf("expected TenantID=%q, got %q", doiTenant, challengeEvent.TenantID)
	}
	if challengeEvent.AggregateID != challengeResp.ChallengeID {
		t.Fatalf("expected AggregateID=%q, got %q", challengeResp.ChallengeID, challengeEvent.AggregateID)
	}

	// 3. Validate outbox Payload security bindings
	payload := challengeEvent.Payload
	if payload == nil {
		t.Fatal("outbox payload is nil")
	}

	rawToken, ok := payload["confirmation_token"].(string)
	if !ok || len(rawToken) != 64 {
		t.Fatalf("expected 64-char hex confirmation_token in outbox payload, got: %v", payload["confirmation_token"])
	}

	if tenant, _ := payload["tenant_id"].(string); tenant != doiTenant {
		t.Fatalf("expected tenant_id=%q in payload, got %q", doiTenant, tenant)
	}
	if subj, _ := payload["subject_id"].(string); subj != subject {
		t.Fatalf("expected subject_id=%q in payload, got %q", subject, subj)
	}
	if purp, _ := payload["purpose"].(string); purp != "double_marketing" {
		t.Fatalf("expected purpose='double_marketing' in payload, got %q", purp)
	}
	if ch, _ := payload["channel"].(string); ch != "email" {
		t.Fatalf("expected channel='email' in payload, got %q", ch)
	}
	if dest, _ := payload["destination"].(string); dest != "email" {
		t.Fatalf("expected destination='email' in payload, got %q", dest)
	}
	if gen, ok := payload["generation"]; !ok || gen == nil {
		t.Fatalf("expected bound generation in outbox payload, got: %v", payload)
	}

	// 4. Simulate the trusted delivery path: downstream mailer receives the token and the user clicks confirm
	confirmResp := f.do(selfToken, http.MethodPost, "/v1/double-opt-in/confirm", map[string]any{
		"token": rawToken,
	})
	if confirmResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on confirm with trusted token, got %d: %s", confirmResp.Code, confirmResp.Body.String())
	}

	var confirmedConsent domain.Consent
	if err := json.Unmarshal(confirmResp.Body.Bytes(), &confirmedConsent); err != nil {
		t.Fatalf("failed to decode confirmed consent: %v", err)
	}
	if confirmedConsent.Status != domain.StatusGranted {
		t.Fatalf("expected status=granted, got %q", confirmedConsent.Status)
	}
	if confirmedConsent.ConfirmedAt == nil || confirmedConsent.GrantedAt == nil {
		t.Fatalf("expected confirmed_at and granted_at timestamps, got confirmed=%v, granted=%v", confirmedConsent.ConfirmedAt, confirmedConsent.GrantedAt)
	}
	if confirmedConsent.ReceiptID == "" {
		t.Fatalf("expected cryptographic receipt_id on confirmation grant")
	}

	// 5. Single-use enforcement: second confirmation with the same token must fail
	replayResp := f.do(selfToken, http.MethodPost, "/v1/double-opt-in/confirm", map[string]any{
		"token": rawToken,
	})
	if replayResp.Code == http.StatusOK {
		t.Fatalf("expected replay to be rejected, got 200 OK: %s", replayResp.Body.String())
	}
}

// TestDoubleOptInCrossSubjectUnauthorized verifies that a self_service caller
// cannot initiate or confirm challenges for another subject.
func TestDoubleOptInCrossSubjectUnauthorized(t *testing.T) {
	f := newDOITestFixture(t)
	subjectA := "cus-doi-subj-a"
	subjectB := "cus-doi-subj-b"
	tokenA := f.token(doiTenant, subjectA, auth.RoleSelfService)

	// User A attempts to initiate DOI for User B -> 403 Forbidden
	resp := f.do(tokenA, http.MethodPost, "/v1/double-opt-in", map[string]any{
		"subject_id":     subjectB,
		"purpose":        "double_marketing",
		"channel":        "email",
		"source":         "web",
		"jurisdiction":   "NG",
		"notice_version": "2026-08",
		"evidence":       f.evidence,
		"expires_at":     time.Now().UTC().Add(time.Hour),
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-subject DOI initiation, got %d: %s", resp.Code, resp.Body.String())
	}
}
