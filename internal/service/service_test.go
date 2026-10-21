package service

import (
	"context"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

const (
	testArtifactHash = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
)

func installCustomPolicyFixture(t *testing.T, svc *Service, tenantID, purposeCode, channel, basisCode, jurisdiction, noticeVersion string, publishedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	_, err := svc.RegisterPurpose(ctx, tenantID, "admin-user", domain.Purpose{
		Code:                       purposeCode,
		Category:                   "marketing",
		AllowedChannels:            []string{channel},
		RequiresAffirmativeConsent: basisCode == "consent",
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("installCustomPolicyFixture purpose: %v", err)
	}
	_, err = svc.RegisterLawfulBasis(ctx, tenantID, "admin-user", domain.LawfulBasis{
		Code:              basisCode,
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	})
	if err != nil {
		t.Fatalf("installCustomPolicyFixture basis: %v", err)
	}
	_, err = svc.PublishNotice(ctx, tenantID, "admin-user", domain.PrivacyNotice{
		Jurisdiction: jurisdiction,
		Version:      noticeVersion,
		PublishedAt:  publishedAt,
		Active:       true,
	})
	if err != nil {
		t.Fatalf("installCustomPolicyFixture notice: %v", err)
	}
}

func TestConsentGrantEvaluateWithdraw(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	now := time.Now().UTC()
	publishedAt := now.Add(-time.Hour)
	installCustomPolicyFixture(t, svc, "tenant-a", "marketing", "whatsapp", "consent", "NG", "2026-08", publishedAt)
	evidence := domain.Evidence{
		Method:       "portal",
		ArtifactHash: testArtifactHash,
		CollectedAt:  publishedAt,
	}

	c, err := svc.RecordGoverned(context.Background(), "tenant-a", "user-1", "cus-1", "marketing", "whatsapp", "consent", "granted", "portal", map[string]string{"brand": "nvuto"}, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.Evaluate(context.Background(), "tenant-a", "cus-1", "marketing", "whatsapp", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allowed || d.ConsentID != c.ID {
		t.Fatalf("expected allowed decision, got %+v", d)
	}
	c2, err := svc.Withdraw(context.Background(), "tenant-a", "user-1", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(c2.Status) != "withdrawn" {
		t.Fatalf("expected withdrawn, got %s", c2.Status)
	}
	d, err = svc.Evaluate(context.Background(), "tenant-a", "cus-1", "marketing", "whatsapp", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != "withdrawn" {
		t.Fatalf("expected suppression after withdrawal, got %+v", d)
	}
	audit := repo.Audit()
	if len(audit) != 5 { // 3 policy publishes + 1 record + 1 withdraw
		t.Fatalf("expected 5 audit events, got %d", len(audit))
	}
	if audit[4].ID == "" || audit[4].ConsentID != c.ID || audit[4].Action != "consent.withdrawn" {
		t.Fatalf("withdrawal audit must reference the consent with its own event ID: %+v", audit[4])
	}
	outbox := repo.Outbox()
	if len(outbox) != 5 {
		t.Fatalf("expected 5 outbox events, got %d", len(outbox))
	}
	if outbox[4].ID == "" || outbox[4].AggregateID != c.ID || outbox[4].EventType != "consent.withdrawn" {
		t.Fatalf("withdrawal outbox must reference the consent with its own event ID: %+v", outbox[4])
	}
}

func TestNoConsentAndExpirySuppress(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	d, err := svc.Evaluate(context.Background(), "tenant-a", "cus-x", "marketing", "email", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != "no_consent" {
		t.Fatalf("unexpected no-consent decision %+v", d)
	}
	now := time.Now().UTC()
	publishedAt := now.Add(-time.Hour)
	installCustomPolicyFixture(t, svc, "tenant-a", "marketing", "email", "consent", "NG", "2026-08", publishedAt)
	evidence := domain.Evidence{
		Method:       "portal",
		ArtifactHash: testArtifactHash,
		CollectedAt:  publishedAt,
	}
	expires := time.Now().UTC().Add(50 * time.Millisecond)
	_, err = svc.RecordGoverned(context.Background(), "tenant-a", "user-1", "cus-x", "marketing", "email", "consent", "granted", "portal", nil, &expires, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	d, err = svc.Evaluate(context.Background(), "tenant-a", "cus-x", "marketing", "email", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != "expired" {
		t.Fatalf("expected expiry suppression, got %+v", d)
	}
}

func TestTenantIsolation(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	now := time.Now().UTC()
	publishedAt := now.Add(-time.Hour)
	installCustomPolicyFixture(t, svc, "tenant-a", "service_updates", "sms", "contract", "NG", "2026-08", publishedAt)
	evidence := domain.Evidence{
		Method:       "admin",
		ArtifactHash: testArtifactHash,
		CollectedAt:  publishedAt,
	}
	_, err := svc.RecordGoverned(context.Background(), "tenant-a", "user-1", "cus-1", "service_updates", "sms", "contract", "granted", "admin", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.Evaluate(context.Background(), "tenant-b", "cus-1", "service_updates", "sms", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != "no_consent" {
		t.Fatalf("cross tenant decision leaked: %+v", d)
	}
}

// TestPolicyTransitionAffirmativeConsent (T-007) verifies that when a purpose is
// updated to require affirmative consent, existing grants with contract legal_basis
// are strictly denied during evaluation rather than silently grandfathered.
func TestPolicyTransitionAffirmativeConsent(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	ctx := context.Background()
	now := time.Now().UTC()
	publishedAt := now.Add(-time.Hour)

	// Step 1: Register purpose with RequiresAffirmativeConsent: false
	_, err := svc.RegisterPurpose(ctx, "tenant-gov", "admin", domain.Purpose{
		Code:                       "analytics",
		Category:                   "marketing",
		AllowedChannels:            []string{"web"},
		RequiresAffirmativeConsent: false,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RegisterLawfulBasis(ctx, "tenant-gov", "admin", domain.LawfulBasis{
		Code:              "contract",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.PublishNotice(ctx, "tenant-gov", "admin", domain.PrivacyNotice{
		Jurisdiction: "NG",
		Version:      "v1",
		PublishedAt:  publishedAt,
		Active:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	evidence := domain.Evidence{
		Method:       "web_terms",
		ArtifactHash: testArtifactHash,
		CollectedAt:  publishedAt,
	}

	// Step 2: Record contract grant
	_, err = svc.RecordGoverned(ctx, "tenant-gov", "admin", "subj-1", "analytics", "web", "contract", "granted", "web", nil, nil, "NG", "v1", evidence)
	if err != nil {
		t.Fatal(err)
	}

	// Step 3: Evaluation allowed before policy tightening
	decision, err := svc.Evaluate(ctx, "tenant-gov", "subj-1", "analytics", "web", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allowed before policy tightening, got %+v", decision)
	}

	// Step 4: Policy transition - require affirmative consent
	_, err = svc.RegisterPurpose(ctx, "tenant-gov", "admin", domain.Purpose{
		Code:                       "analytics",
		Category:                   "marketing",
		AllowedChannels:            []string{"web"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Step 5: Evaluation must now DENY with policy_unavailable (no grandfathering)
	decision, err = svc.Evaluate(ctx, "tenant-gov", "subj-1", "analytics", "web", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || (decision.Reason != domain.DecisionReasonPolicyRequirementUnmet && decision.Reason != "policy_unavailable") {
		t.Fatalf("expected policy_requirement_unmet after tightening affirmative consent, got %+v", decision)
	}
}

// TestPolicyTransitionDoubleOptIn (T-007) verifies that when a purpose is
// updated to require double opt-in, existing unconfirmed direct grants are
// strictly denied during evaluation rather than silently grandfathered.
func TestPolicyTransitionDoubleOptIn(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	ctx := context.Background()
	now := time.Now().UTC()
	publishedAt := now.Add(-time.Hour)

	// Step 1: Register purpose with RequiresDoubleOptIn: false
	_, err := svc.RegisterPurpose(ctx, "tenant-doi", "admin", domain.Purpose{
		Code:                       "newsletter",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RegisterLawfulBasis(ctx, "tenant-doi", "admin", domain.LawfulBasis{
		Code:              "consent",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.PublishNotice(ctx, "tenant-doi", "admin", domain.PrivacyNotice{
		Jurisdiction: "NG",
		Version:      "v1",
		PublishedAt:  publishedAt,
		Active:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	evidence := domain.Evidence{
		Method:       "optin_box",
		ArtifactHash: testArtifactHash,
		CollectedAt:  publishedAt,
	}

	// Step 2: Record direct grant (ConfirmedAt is nil)
	_, err = svc.RecordGoverned(ctx, "tenant-doi", "admin", "subj-2", "newsletter", "email", "consent", "granted", "web", nil, nil, "NG", "v1", evidence)
	if err != nil {
		t.Fatal(err)
	}

	// Step 3: Allowed initially
	decision, err := svc.Evaluate(ctx, "tenant-doi", "subj-2", "newsletter", "email", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allowed before DOI policy transition, got %+v", decision)
	}

	// Step 4: Policy transition - require double opt-in
	_, err = svc.RegisterPurpose(ctx, "tenant-doi", "admin", domain.Purpose{
		Code:                       "newsletter",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        true,
		Active:                     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Step 5: Unconfirmed grant must now be rejected with policy_unavailable
	decision, err = svc.Evaluate(ctx, "tenant-doi", "subj-2", "newsletter", "email", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || (decision.Reason != domain.DecisionReasonPolicyRequirementUnmet && decision.Reason != "policy_unavailable") {
		t.Fatalf("expected policy_requirement_unmet for unconfirmed grant under DOI policy, got %+v", decision)
	}
}

// TestLegacyBlankNoticeVersionQuarantined (T-008) verifies that legacy rows
// with a blank notice_version are rejected with policy_unavailable rather than
// bypassing governed policy checks.
func TestLegacyBlankNoticeVersionQuarantined(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	ctx := context.Background()
	now := time.Now().UTC()
	publishedAt := now.Add(-time.Hour)

	installCustomPolicyFixture(t, svc, "tenant-legacy", "promotions", "sms", "consent", "NG", "2026-08", publishedAt)
	evidence := domain.Evidence{
		Method:       "sms_reply",
		ArtifactHash: testArtifactHash,
		CollectedAt:  publishedAt,
	}

	c, err := svc.RecordGoverned(ctx, "tenant-legacy", "admin", "subj-legacy", "promotions", "sms", "consent", "granted", "sms", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}

	// Verify allowed before tampering
	decision, err := svc.Evaluate(ctx, "tenant-legacy", "subj-legacy", "promotions", "sms", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allowed before blank notice_version simulation, got %+v", decision)
	}

	// Simulate legacy migrated row with blank notice_version
	cStored, err := repo.Get(ctx, "tenant-legacy", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	cStored.NoticeVersion = ""
	// Direct store update of the simulated legacy record
	type consentKey struct{ tenantID, consentID string }
	// We can update through Withdraw or direct method, but let's test evaluation of blank notice_version
	// In the memory store, m.consents is internal to package store, but we can verify via Evaluate
	// by testing an effective record with empty NoticeVersion.
	// Since service_test is in package service, we can use a custom repository or test double to return
	// a Consent with NoticeVersion == "".
	stub := &legacyConsentRepo{Repository: repo, consent: cStored}
	legacySvc := New(stub)

	decision, err = legacySvc.Evaluate(ctx, "tenant-legacy", "subj-legacy", "promotions", "sms", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("expected policy_requirement_unmet for blank notice_version on marketing grant, got %+v", decision)
	}
}

// TestLegacySafeServiceContractRecordEligible (T-008) verifies that legacy rows
// with a blank notice_version under the service category and contract legal basis
// (where affirmative consent and evidence are not required) are eligible and allowed
// under the D-04 Option C grandfathering rule.
func TestLegacySafeServiceContractRecordEligible(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Install service_updates purpose (category: service, requires_affirmative_consent: false, requires_double_opt_in: false)
	_, err := svc.RegisterPurpose(ctx, "tenant-safe", "admin", domain.Purpose{
		Code:                       "service_updates",
		Category:                   "service",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: false,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 2. Install contract lawful basis (allowed_categories: ["service"], requires_evidence: false)
	_, err = svc.RegisterLawfulBasis(ctx, "tenant-safe", "admin", domain.LawfulBasis{
		Code:              "contract",
		AllowedCategories: []string{"service"},
		RequiresEvidence:  false,
		Active:            true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 3. Create a legacy consent record directly with blank NoticeVersion
	legacyGrant := domain.Consent{
		ID:            "con-legacy-safe",
		TenantID:      "tenant-safe",
		SubjectID:     "subj-safe-1",
		Purpose:       "service_updates",
		Channel:       "email",
		LegalBasis:    "contract",
		Status:        domain.StatusGranted,
		Source:        "legacy",
		NoticeVersion: "", // Blank notice version from pre-002 schema
		Version:       1,
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now.Add(-24 * time.Hour),
	}

	stub := &legacyConsentRepo{Repository: repo, consent: legacyGrant}
	legacySvc := New(stub)

	// 4. Evaluate must return allowed=true, reason=granted for safe legacy service contract
	decision, err := legacySvc.Evaluate(ctx, "tenant-safe", "subj-safe-1", "service_updates", "email", now)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.Reason != domain.DecisionReasonGranted {
		t.Fatalf("expected allowed=true, reason=granted for safe legacy service contract, got %+v", decision)
	}
}

type legacyConsentRepo struct {
	store.Repository
	consent domain.Consent
}

func (l *legacyConsentRepo) FindEffective(ctx context.Context, tenantID, subjectID, purpose, channel string) (domain.Consent, error) {
	if l.consent.TenantID == tenantID && l.consent.SubjectID == subjectID {
		return l.consent, nil
	}
	return l.Repository.FindEffective(ctx, tenantID, subjectID, purpose, channel)
}

// TestMemoryStoreTenantDelimiterCollision (T-026) verifies that tenants with
// colon delimiters cannot collide or access each other's data in the memory store.
func TestMemoryStoreTenantDelimiterCollision(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	ctx := context.Background()
	now := time.Now().UTC()
	publishedAt := now.Add(-time.Hour)

	// Tenant A: "tenant"
	// Tenant B: "tenant:child"
	installCustomPolicyFixture(t, svc, "tenant", "updates", "email", "consent", "NG", "v1", publishedAt)
	installCustomPolicyFixture(t, svc, "tenant:child", "updates", "email", "consent", "NG", "v1", publishedAt)

	evidence := domain.Evidence{
		Method:       "web",
		ArtifactHash: testArtifactHash,
		CollectedAt:  publishedAt,
	}

	// Record in tenant:child with subject "user1"
	_, err := svc.RecordGoverned(ctx, "tenant:child", "admin", "user1", "updates", "email", "consent", "granted", "web", nil, nil, "NG", "v1", evidence)
	if err != nil {
		t.Fatal(err)
	}

	// Evaluating under "tenant" for subject "child:user1" (or "user1") must NOT find consent from "tenant:child"
	decision, err := svc.Evaluate(ctx, "tenant", "user1", "updates", "email", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.Reason != "no_consent" {
		t.Fatalf("expected no_consent for tenant, got %+v", decision)
	}

	// Also verify that evaluation under "tenant:child" finds it
	decisionChild, err := svc.Evaluate(ctx, "tenant:child", "user1", "updates", "email", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !decisionChild.Allowed {
		t.Fatalf("expected allowed for tenant:child, got %+v", decisionChild)
	}
}
