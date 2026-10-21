package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// TestAT007_DoubleOptInTightening verifies that a consent granted when
// RequiresDoubleOptIn=false correctly evaluates to allowed=false with
// reason "policy_requirement_unmet" after the purpose is updated to RequiresDoubleOptIn=true.
func TestAT007_DoubleOptInTightening(t *testing.T) {
	tenantID := "tenant-doi-tighten"
	repo := store.NewMemory()
	svc, handler, _, evidence := setupEvaluateFixture(t, repo, tenantID)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Register purpose with RequiresDoubleOptIn = false
	_, err := svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "newsletter",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to register purpose: %v", err)
	}

	// 2. Record direct grant (status: granted, confirmed_at: nil)
	consent, err := svc.RecordGoverned(ctx, tenantID, "admin", "subj-doi-1", "newsletter", "email", "consent", "granted", "web", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("failed to record grant: %v", err)
	}
	if consent.Status != domain.StatusGranted || consent.ConfirmedAt != nil {
		t.Fatalf("expected direct unconfirmed grant: %+v", consent)
	}

	// 3. Evaluation must return allowed=true before policy tightening
	decision, err := svc.Evaluate(ctx, tenantID, "subj-doi-1", "newsletter", "email", now)
	if err != nil || !decision.Allowed || decision.Reason != domain.DecisionReasonGranted {
		t.Fatalf("expected allowed=true, reason=granted before tightening, got %+v (err=%v)", decision, err)
	}

	// Also verify via HTTP POST /v1/evaluate
	evalReqBody, _ := json.Marshal(map[string]any{
		"subject_id": "subj-doi-1",
		"purpose":    "newsletter",
		"channel":    "email",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(evalReqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP evaluate failed: %d %s", w.Code, w.Body.String())
	}
	var httpDecision domain.SuppressionDecision
	_ = json.Unmarshal(w.Body.Bytes(), &httpDecision)
	if !httpDecision.Allowed || httpDecision.Reason != domain.DecisionReasonGranted {
		t.Fatalf("HTTP decision before tightening expected allowed=true: %+v", httpDecision)
	}

	// 4. Policy Transition: Update purpose to RequiresDoubleOptIn = true
	_, err = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "newsletter",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        true,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to update purpose: %v", err)
	}

	// 5. Strict Re-evaluation: No silent grandfathering. Must return allowed=false, reason=policy_requirement_unmet
	decisionAfter, err := svc.Evaluate(ctx, tenantID, "subj-doi-1", "newsletter", "email", now)
	if err != nil {
		t.Fatalf("evaluation returned unexpected error: %v", err)
	}
	if decisionAfter.Allowed {
		t.Fatalf("expected allowed=false after tightening DOI policy, got %+v", decisionAfter)
	}
	if decisionAfter.Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("expected reason %q, got %q", domain.DecisionReasonPolicyRequirementUnmet, decisionAfter.Reason)
	}

	// Verify rejection via HTTP POST /v1/evaluate as well
	reqAfter := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(evalReqBody))
	reqAfter.Header.Set("Content-Type", "application/json")
	reqAfter.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	reqAfter.Header.Set("X-Nvuto-Actor-ID", "admin")
	wAfter := httptest.NewRecorder()
	handler.ServeHTTP(wAfter, reqAfter)
	if wAfter.Code != http.StatusOK {
		t.Fatalf("HTTP evaluate returned %d: %s", wAfter.Code, wAfter.Body.String())
	}
	var httpDecisionAfter domain.SuppressionDecision
	_ = json.Unmarshal(wAfter.Body.Bytes(), &httpDecisionAfter)
	if httpDecisionAfter.Allowed {
		t.Fatalf("HTTP evaluate must deny after tightening DOI policy: %+v", httpDecisionAfter)
	}
	if httpDecisionAfter.Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("HTTP evaluate expected reason %q, got %q", domain.DecisionReasonPolicyRequirementUnmet, httpDecisionAfter.Reason)
	}
}

// TestAT007_PendingDOIConfirmationPurposeDeactivated verifies that if the underlying purpose
// is deactivated before confirmation occurs:
// 1. Pending confirmation fails (rejected).
// 2. Evaluation returns allowed=false.
func TestAT007_PendingDOIConfirmationPurposeDeactivated(t *testing.T) {
	tenantID := "tenant-doi-deact"
	repo := store.NewMemory()
	svc, handler, _, evidence := setupEvaluateFixture(t, repo, tenantID)
	ctx := context.Background()

	// 1. Purpose with RequiresDoubleOptIn = true, Active = true
	_, err := svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "promotions",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        true,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to register purpose: %v", err)
	}

	// 2. Start Double Opt-In challenge
	challenge, err := svc.StartDoubleOptIn(ctx, tenantID, "admin", "subj-deact-1", "promotions", "email", "web", "NG", "2026-08", evidence, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("failed to start DOI: %v", err)
	}
	if challenge.ConfirmationToken == "" {
		t.Fatal("expected confirmation token from start challenge")
	}

	// 3. Before confirmation, evaluation must be denied (pending_confirmation)
	decBefore, err := svc.Evaluate(ctx, tenantID, "subj-deact-1", "promotions", "email", time.Now().UTC())
	if err != nil || decBefore.Allowed {
		t.Fatalf("pending DOI must not grant permission: %+v", decBefore)
	}

	// 4. Deactivate purpose
	_, err = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "promotions",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        true,
		Active:                     false, // Deactivated!
	})
	if err != nil {
		t.Fatalf("failed to deactivate purpose: %v", err)
	}

	// 5. Attempt confirmation via HTTP POST /v1/double-opt-in/confirm
	confirmBody, _ := json.Marshal(map[string]any{
		"token": challenge.ConfirmationToken,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/double-opt-in/confirm", bytes.NewReader(confirmBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Must reject confirmation with 422 Unprocessable Entity
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422 for confirming deactivated purpose, got %d: %s", w.Code, w.Body.String())
	}

	// Attempt confirmation directly via Service method
	_, err = svc.ConfirmDoubleOptIn(ctx, tenantID, "admin", service.TenantWide(), challenge.ConfirmationToken)
	if err == nil {
		t.Fatal("expected ConfirmDoubleOptIn to fail for deactivated purpose, got nil")
	}
	if !errors.Is(err, service.ErrValidation) && !errors.Is(err, domain.ErrPolicyRejected) {
		t.Fatalf("expected ErrPolicyRejected / ErrValidation, got %v", err)
	}

	// 6. Verify evaluation continues to return allowed=false
	decAfter, err := svc.Evaluate(ctx, tenantID, "subj-deact-1", "promotions", "email", time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if decAfter.Allowed {
		t.Fatalf("evaluation of pending consent with deactivated purpose must be denied: %+v", decAfter)
	}
}

// TestAT007_PendingDOIConfirmationChannelRemoved verifies that if the channel
// is removed from the purpose's allowed channels between DOI initiation and confirmation,
// confirmation is rejected and evaluation remains denied.
func TestAT007_PendingDOIConfirmationChannelRemoved(t *testing.T) {
	tenantID := "tenant-doi-chan-rem"
	repo := store.NewMemory()
	svc, handler, _, evidence := setupEvaluateFixture(t, repo, tenantID)
	ctx := context.Background()

	// 1. Purpose initially allows both email and sms
	_, err := svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "alerts",
		Category:                   "marketing",
		AllowedChannels:            []string{"email", "sms"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        true,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to register purpose: %v", err)
	}

	// 2. Start DOI challenge for channel "sms"
	challenge, err := svc.StartDoubleOptIn(ctx, tenantID, "admin", "subj-sms-1", "alerts", "sms", "web", "NG", "2026-08", evidence, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("failed to start DOI: %v", err)
	}

	// 3. Update purpose to remove "sms" from AllowedChannels
	_, err = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "alerts",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"}, // sms removed!
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        true,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to update purpose channels: %v", err)
	}

	// 4. Confirmation via HTTP must fail
	confirmBody, _ := json.Marshal(map[string]any{
		"token": challenge.ConfirmationToken,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/double-opt-in/confirm", bytes.NewReader(confirmBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422 for removed channel confirmation, got %d: %s", w.Code, w.Body.String())
	}

	// Confirmation via Service must fail
	_, err = svc.ConfirmDoubleOptIn(ctx, tenantID, "admin", service.TenantWide(), challenge.ConfirmationToken)
	if err == nil {
		t.Fatal("expected ConfirmDoubleOptIn to fail for removed channel, got nil")
	}

	// Evaluation must return allowed=false
	dec, err := svc.Evaluate(ctx, tenantID, "subj-sms-1", "alerts", "sms", time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if dec.Allowed {
		t.Fatalf("expected allowed=false for removed channel, got %+v", dec)
	}
}

// TestAT007_AffirmativeConsentTightening verifies that when a purpose is updated
// to require affirmative consent, existing grants captured under lesser lawful bases
// (such as contract) are strictly denied upon evaluation with policy_requirement_unmet.
func TestAT007_AffirmativeConsentTightening(t *testing.T) {
	tenantID := "tenant-affirm-tighten"
	repo := store.NewMemory()
	svc, handler, _, evidence := setupEvaluateFixture(t, repo, tenantID)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Register lawful basis "contract"
	_, err := svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "contract",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	})
	if err != nil {
		t.Fatalf("failed to register lawful basis: %v", err)
	}

	// 2. Register purpose with RequiresAffirmativeConsent = false
	_, err = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "analytics",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: false,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to register purpose: %v", err)
	}

	// 3. Record grant under legal basis "contract"
	consent, err := svc.RecordGoverned(ctx, tenantID, "admin", "subj-aff-1", "analytics", "email", "contract", "granted", "web", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("failed to record grant: %v", err)
	}
	if consent.Status != domain.StatusGranted {
		t.Fatalf("expected granted status: %+v", consent)
	}

	// 4. Initially allowed
	decBefore, err := svc.Evaluate(ctx, tenantID, "subj-aff-1", "analytics", "email", now)
	if err != nil || !decBefore.Allowed || decBefore.Reason != domain.DecisionReasonGranted {
		t.Fatalf("expected allowed before tightening: %+v (err=%v)", decBefore, err)
	}

	// 5. Policy Transition: update purpose to require affirmative consent
	_, err = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "analytics",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true, // Now required!
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to update purpose: %v", err)
	}

	// 6. Evaluation must strictly deny the lesser legal basis (no grandfathering)
	decAfter, err := svc.Evaluate(ctx, tenantID, "subj-aff-1", "analytics", "email", now)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if decAfter.Allowed {
		t.Fatalf("lesser legal basis must NOT be grandfathered when affirmative consent is required: %+v", decAfter)
	}
	if decAfter.Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("expected reason %q, got %q", domain.DecisionReasonPolicyRequirementUnmet, decAfter.Reason)
	}

	// Verify via HTTP as well
	evalReqBody, _ := json.Marshal(map[string]any{
		"subject_id": "subj-aff-1",
		"purpose":    "analytics",
		"channel":    "email",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(evalReqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP evaluate failed: %d %s", w.Code, w.Body.String())
	}
	var httpDec domain.SuppressionDecision
	_ = json.Unmarshal(w.Body.Bytes(), &httpDec)
	if httpDec.Allowed || httpDec.Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("HTTP evaluate expected allowed=false, reason=policy_requirement_unmet, got %+v", httpDec)
	}
}

// TestAT007_EvidenceRequirementTightening verifies that when a lawful basis is updated
// to require evidence (RequiresEvidence=true), older grants that lack a valid evidence hash
// strictly fail evaluation with policy_requirement_unmet.
func TestAT007_EvidenceRequirementTightening(t *testing.T) {
	tenantID := "tenant-evid-tighten"
	repo := store.NewMemory()
	svc, handler, _, _ := setupEvaluateFixture(t, repo, tenantID)
	ctx := context.Background()

	// 1. Register lawful basis "legitimate_interest" with RequiresEvidence = false
	_, err := svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "legitimate_interest",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  false, // Evidence NOT required initially
		Active:            true,
	})
	if err != nil {
		t.Fatalf("failed to register lawful basis: %v", err)
	}

	// 2. Register purpose
	_, err = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "direct_mail",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: false,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to register purpose: %v", err)
	}

	// 3. Record grant with empty evidence
	emptyEvidence := domain.Evidence{}
	consent, err := svc.RecordGoverned(ctx, tenantID, "admin", "subj-evid-1", "direct_mail", "email", "legitimate_interest", "granted", "web", nil, nil, "NG", "2026-08", emptyEvidence)
	if err != nil {
		t.Fatalf("failed to record grant without evidence: %v", err)
	}
	if consent.Status != domain.StatusGranted {
		t.Fatalf("expected granted status: %+v", consent)
	}

	// 4. Initially allowed because lawful basis did not require evidence
	decBefore, err := svc.Evaluate(ctx, tenantID, "subj-evid-1", "direct_mail", "email", time.Now().UTC())
	if err != nil || !decBefore.Allowed || decBefore.Reason != domain.DecisionReasonGranted {
		t.Fatalf("expected allowed before evidence tightening: %+v (err=%v)", decBefore, err)
	}

	// 5. Policy Transition: Lawful basis updated to RequiresEvidence = true
	_, err = svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "legitimate_interest",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true, // Evidence now strictly required!
		Active:            true,
	})
	if err != nil {
		t.Fatalf("failed to update lawful basis: %v", err)
	}

	// 6. Strict Re-evaluation: Grant lacking valid evidence must be denied
	decAfter, err := svc.Evaluate(ctx, tenantID, "subj-evid-1", "direct_mail", "email", time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if decAfter.Allowed {
		t.Fatalf("grant lacking required evidence must be denied after policy transition: %+v", decAfter)
	}
	if decAfter.Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("expected reason %q, got %q", domain.DecisionReasonPolicyRequirementUnmet, decAfter.Reason)
	}

	// Verify via HTTP POST /v1/evaluate
	evalReqBody, _ := json.Marshal(map[string]any{
		"subject_id": "subj-evid-1",
		"purpose":    "direct_mail",
		"channel":    "email",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(evalReqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP evaluate failed: %d %s", w.Code, w.Body.String())
	}
	var httpDec domain.SuppressionDecision
	_ = json.Unmarshal(w.Body.Bytes(), &httpDec)
	if httpDec.Allowed || httpDec.Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("HTTP evaluate expected allowed=false, reason=policy_requirement_unmet, got %+v", httpDec)
	}
}

// TestAT007_ChannelCapabilityRemoval verifies that when a channel is removed from
// a purpose's AllowedChannels, older grants recorded for that channel evaluate to
// allowed=false with reason "channel_not_allowed".
func TestAT007_ChannelCapabilityRemoval(t *testing.T) {
	tenantID := "tenant-chan-remove"
	repo := store.NewMemory()
	svc, handler, _, evidence := setupEvaluateFixture(t, repo, tenantID)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Purpose allows email and push
	_, err := svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "updates",
		Category:                   "marketing",
		AllowedChannels:            []string{"email", "push"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to register purpose: %v", err)
	}

	// 2. Grant recorded for channel "push"
	_, err = svc.RecordGoverned(ctx, tenantID, "admin", "subj-chan-1", "updates", "push", "consent", "granted", "web", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("failed to record grant: %v", err)
	}

	// 3. Initially allowed for "push"
	decBefore, err := svc.Evaluate(ctx, tenantID, "subj-chan-1", "updates", "push", now)
	if err != nil || !decBefore.Allowed || decBefore.Reason != domain.DecisionReasonGranted {
		t.Fatalf("expected allowed for push initially: %+v (err=%v)", decBefore, err)
	}

	// 4. Policy Transition: Remove "push" from AllowedChannels
	_, err = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "updates",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"}, // push removed!
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to update purpose channels: %v", err)
	}

	// 5. Strict Re-evaluation: Evaluation for "push" must return allowed=false, reason=channel_not_allowed
	decAfter, err := svc.Evaluate(ctx, tenantID, "subj-chan-1", "updates", "push", now)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if decAfter.Allowed {
		t.Fatalf("expected allowed=false for removed channel, got %+v", decAfter)
	}
	if decAfter.Reason != domain.DecisionReasonChannelNotAllowed {
		t.Fatalf("expected reason %q, got %q", domain.DecisionReasonChannelNotAllowed, decAfter.Reason)
	}

	// Verify via HTTP POST /v1/evaluate
	evalReqBody, _ := json.Marshal(map[string]any{
		"subject_id": "subj-chan-1",
		"purpose":    "updates",
		"channel":    "push",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(evalReqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP evaluate failed: %d %s", w.Code, w.Body.String())
	}
	var httpDec domain.SuppressionDecision
	_ = json.Unmarshal(w.Body.Bytes(), &httpDec)
	if httpDec.Allowed || httpDec.Reason != domain.DecisionReasonChannelNotAllowed {
		t.Fatalf("HTTP evaluate expected allowed=false, reason=channel_not_allowed, got %+v", httpDec)
	}
}

// TestAT007_PurposeDeactivationAgainstExistingGrant verifies that deactivating
// a purpose immediately suppresses older grants during evaluation with reason "purpose_inactive".
func TestAT007_PurposeDeactivationAgainstExistingGrant(t *testing.T) {
	tenantID := "tenant-purp-deact"
	repo := store.NewMemory()
	svc, handler, _, evidence := setupEvaluateFixture(t, repo, tenantID)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Purpose initially active
	_, err := svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "offers",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to register purpose: %v", err)
	}

	// 2. Grant recorded
	_, err = svc.RecordGoverned(ctx, tenantID, "admin", "subj-off-1", "offers", "email", "consent", "granted", "web", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("failed to record grant: %v", err)
	}

	// 3. Initially allowed
	decBefore, err := svc.Evaluate(ctx, tenantID, "subj-off-1", "offers", "email", now)
	if err != nil || !decBefore.Allowed || decBefore.Reason != domain.DecisionReasonGranted {
		t.Fatalf("expected allowed before deactivation: %+v (err=%v)", decBefore, err)
	}

	// 4. Policy Transition: Purpose deactivated
	_, err = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "offers",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     false, // Deactivated!
	})
	if err != nil {
		t.Fatalf("failed to deactivate purpose: %v", err)
	}

	// 5. Strict Re-evaluation: Must return allowed=false, reason=purpose_inactive
	decAfter, err := svc.Evaluate(ctx, tenantID, "subj-off-1", "offers", "email", now)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if decAfter.Allowed {
		t.Fatalf("expected allowed=false for deactivated purpose, got %+v", decAfter)
	}
	if decAfter.Reason != domain.DecisionReasonPurposeInactive {
		t.Fatalf("expected reason %q, got %q", domain.DecisionReasonPurposeInactive, decAfter.Reason)
	}

	// Verify via HTTP POST /v1/evaluate
	evalReqBody, _ := json.Marshal(map[string]any{
		"subject_id": "subj-off-1",
		"purpose":    "offers",
		"channel":    "email",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(evalReqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP evaluate failed: %d %s", w.Code, w.Body.String())
	}
	var httpDec domain.SuppressionDecision
	_ = json.Unmarshal(w.Body.Bytes(), &httpDec)
	if httpDec.Allowed || httpDec.Reason != domain.DecisionReasonPurposeInactive {
		t.Fatalf("HTTP evaluate expected allowed=false, reason=purpose_inactive, got %+v", httpDec)
	}
}

// TestAT007_BatchEvaluationPolicyReenforcement verifies that batch evaluation
// (/v1/evaluate:batch and svc.BatchEvaluate) accurately enforces dynamic policy rules
// across multiple subjects and channels.
func TestAT007_BatchEvaluationPolicyReenforcement(t *testing.T) {
	tenantID := "tenant-batch-reenforce"
	repo := store.NewMemory()
	svc, handler, _, evidence := setupEvaluateFixture(t, repo, tenantID)
	ctx := context.Background()

	// Setup purpose 1: active, allows email
	_, _ = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "valid_marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	// Setup purpose 2: starts active, then deactivated after grant
	_, _ = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "inactive_marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	// Setup purpose 3: requires double opt in initially false, then tightened
	_, _ = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "doi_marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})

	// Grant for subject 1 (valid)
	_, _ = svc.RecordGoverned(ctx, tenantID, "admin", "batch-1", "valid_marketing", "email", "consent", "granted", "web", nil, nil, "NG", "2026-08", evidence)
	// Grant for subject 2 (while active)
	_, _ = svc.RecordGoverned(ctx, tenantID, "admin", "batch-2", "inactive_marketing", "email", "consent", "granted", "web", nil, nil, "NG", "2026-08", evidence)
	// Direct unconfirmed grant for subject 3 (while DOI not required)
	_, _ = svc.RecordGoverned(ctx, tenantID, "admin", "batch-3", "doi_marketing", "email", "consent", "granted", "web", nil, nil, "NG", "2026-08", evidence)

	// Now apply policy transitions:
	// Deactivate purpose 2
	_, _ = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "inactive_marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     false, // Deactivated!
	})
	// Require DOI on purpose 3
	_, _ = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "doi_marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        true, // Tightened!
		Active:                     true,
	})

	batchRequests := []domain.EvaluationRequest{
		{SubjectID: "batch-1", Purpose: "valid_marketing", Channel: "email"},
		{SubjectID: "batch-2", Purpose: "inactive_marketing", Channel: "email"},
		{SubjectID: "batch-3", Purpose: "doi_marketing", Channel: "email"},
	}

	// 1. Service BatchEvaluate
	decisions, err := svc.BatchEvaluate(ctx, tenantID, batchRequests)
	if err != nil {
		t.Fatalf("BatchEvaluate failed: %v", err)
	}
	if len(decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(decisions))
	}
	// Subject 1: allowed
	if !decisions[0].Allowed || decisions[0].Reason != domain.DecisionReasonGranted {
		t.Fatalf("expected subject 1 allowed, got %+v", decisions[0])
	}
	// Subject 2: purpose_inactive
	if decisions[1].Allowed || decisions[1].Reason != domain.DecisionReasonPurposeInactive {
		t.Fatalf("expected subject 2 denied with purpose_inactive, got %+v", decisions[1])
	}
	// Subject 3: policy_requirement_unmet
	if decisions[2].Allowed || decisions[2].Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("expected subject 3 denied with policy_requirement_unmet, got %+v", decisions[2])
	}

	// 2. HTTP Batch Evaluation
	body, _ := json.Marshal(map[string]any{
		"requests": batchRequests,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate:batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP batch evaluate failed: %d %s", w.Code, w.Body.String())
	}
	var batchResp struct {
		Decisions []domain.SuppressionDecision `json:"decisions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &batchResp); err != nil {
		t.Fatalf("failed to decode HTTP batch response: %v", err)
	}
	httpDecisions := batchResp.Decisions
	if len(httpDecisions) != 3 {
		t.Fatalf("expected 3 HTTP batch decisions, got %d (body: %s)", len(httpDecisions), w.Body.String())
	}
	if !httpDecisions[0].Allowed || httpDecisions[0].Reason != domain.DecisionReasonGranted {
		t.Fatalf("HTTP subject 1 mismatch: %+v", httpDecisions[0])
	}
	if httpDecisions[1].Allowed || httpDecisions[1].Reason != domain.DecisionReasonPurposeInactive {
		t.Fatalf("HTTP subject 2 mismatch: %+v", httpDecisions[1])
	}
	if httpDecisions[2].Allowed || httpDecisions[2].Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("HTTP subject 3 mismatch: %+v", httpDecisions[2])
	}
}
