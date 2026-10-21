package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// legacyTestRepo wraps a base repository to inject legacy consent records directly.
type legacyTestRepo struct {
	store.Repository
	consents map[string]domain.Consent
}

func newLegacyTestRepo(base store.Repository) *legacyTestRepo {
	return &legacyTestRepo{
		Repository: base,
		consents:   make(map[string]domain.Consent),
	}
}

func (r *legacyTestRepo) addLegacy(c domain.Consent) {
	r.consents[c.TenantID+":"+c.SubjectID+":"+c.Purpose+":"+c.Channel] = c
	r.consents[c.TenantID+":"+c.ID] = c
}

func (r *legacyTestRepo) FindEffective(ctx context.Context, tenantID, subjectID, purpose, channel string) (domain.Consent, error) {
	key := tenantID + ":" + subjectID + ":" + purpose + ":" + channel
	if c, ok := r.consents[key]; ok {
		return c, nil
	}
	return r.Repository.FindEffective(ctx, tenantID, subjectID, purpose, channel)
}

func (r *legacyTestRepo) Get(ctx context.Context, tenantID, id string) (domain.Consent, error) {
	key := tenantID + ":" + id
	if c, ok := r.consents[key]; ok {
		return c, nil
	}
	return r.Repository.Get(ctx, tenantID, id)
}

const validTestArtifactHash = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

// setupLegacyTestHarness initializes a test harness with in-memory store, wrapper, and HTTP API handler.
func setupLegacyTestHarness(t *testing.T, tenantID string) (*service.Service, http.Handler, *legacyTestRepo) {
	t.Setenv("ALLOW_DEV_HEADERS", "true")
	baseRepo := store.NewMemory()
	legRepo := newLegacyTestRepo(baseRepo)
	svc := service.New(legRepo)
	handler := newHandler(svc, 5*time.Second, nil)
	return svc, handler, legRepo
}

// TestAT008_SafeLegacyServiceContractGrantEligible verifies that legacy records
// with blank notice_version under non-marketing service categories governed by
// contract or legitimate_interest (where affirmative consent is not required)
// are eligible and evaluate to allowed=true, reason="granted".
func TestAT008_SafeLegacyServiceContractGrantEligible(t *testing.T) {
	tenantID := "tenant-leg-safe"
	svc, handler, repo := setupLegacyTestHarness(t, tenantID)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Configure service_updates purpose (category: service, requires_affirmative_consent: false)
	_, err := svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "service_updates",
		Category:                   "service",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: false,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to register service purpose: %v", err)
	}

	// 2. Configure contract lawful basis (allowed_categories: ["service"], requires_evidence: false)
	_, err = svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "contract",
		AllowedCategories: []string{"service"},
		RequiresEvidence:  false,
		Active:            true,
	})
	if err != nil {
		t.Fatalf("failed to register contract lawful basis: %v", err)
	}

	// 3. Inject legacy consent record into store (blank notice_version, no receipt)
	legacyGrant := domain.Consent{
		ID:            "con-legacy-safe-1",
		TenantID:      tenantID,
		SubjectID:     "cust-safe-100",
		Purpose:       "service_updates",
		Channel:       "email",
		LegalBasis:    "contract",
		Status:        domain.StatusGranted,
		Source:        "legacy-portal",
		NoticeVersion: "", // Blank notice from pre-002 schema
		Version:       1,
		CreatedAt:     now.Add(-48 * time.Hour),
		UpdatedAt:     now.Add(-48 * time.Hour),
	}
	repo.addLegacy(legacyGrant)

	// 4. Programmatic evaluation check
	dec, err := svc.Evaluate(ctx, tenantID, "cust-safe-100", "service_updates", "email", now)
	if err != nil {
		t.Fatalf("Evaluate returned unexpected error: %v", err)
	}
	if !dec.Allowed || dec.Reason != domain.DecisionReasonGranted {
		t.Fatalf("expected safe legacy grant to be allowed=true, reason=granted; got %+v", dec)
	}

	// 5. HTTP API evaluation check: POST /v1/evaluate
	body, _ := json.Marshal(map[string]any{
		"subject_id": "cust-safe-100",
		"purpose":    "service_updates",
		"channel":    "email",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "service-client")
	req.Header.Set("X-Nvuto-Roles", "service")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP /v1/evaluate expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var httpDec domain.SuppressionDecision
	if err := json.Unmarshal(rec.Body.Bytes(), &httpDec); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !httpDec.Allowed || httpDec.Reason != domain.DecisionReasonGranted {
		t.Fatalf("HTTP evaluate expected allowed=true, reason=granted; got %+v", httpDec)
	}
}

// TestAT008_UnsafeLegacyBlankNoticeMarketingQuarantined verifies that blank-notice
// records CANNOT bypass an affirmative-consent requirement by default.
// A legacy marketing grant lacking notice_version must evaluate to allowed=false,
// reason="policy_requirement_unmet".
func TestAT008_UnsafeLegacyBlankNoticeMarketingQuarantined(t *testing.T) {
	tenantID := "tenant-leg-unsafe"
	svc, handler, repo := setupLegacyTestHarness(t, tenantID)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Configure marketing purpose requiring affirmative consent
	_, err := svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatalf("failed to register marketing purpose: %v", err)
	}

	// 2. Configure consent lawful basis requiring evidence
	_, err = svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "consent",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	})
	if err != nil {
		t.Fatalf("failed to register consent lawful basis: %v", err)
	}

	// 3. Inject legacy grant lacking notice_version and lacking evidence
	unsafeGrant := domain.Consent{
		ID:            "con-legacy-unsafe-1",
		TenantID:      tenantID,
		SubjectID:     "cust-unsafe-200",
		Purpose:       "marketing",
		Channel:       "email",
		LegalBasis:    "consent",
		Status:        domain.StatusGranted,
		Source:        "legacy-import",
		NoticeVersion: "", // Blank notice
		Version:       1,
		CreatedAt:     now.Add(-72 * time.Hour),
		UpdatedAt:     now.Add(-72 * time.Hour),
	}
	repo.addLegacy(unsafeGrant)

	// 4. Programmatic evaluation must strictly deny and quarantine
	dec, err := svc.Evaluate(ctx, tenantID, "cust-unsafe-200", "marketing", "email", now)
	if err != nil {
		t.Fatalf("Evaluate returned unexpected error: %v", err)
	}
	if dec.Allowed || dec.Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("expected blank-notice marketing grant to be denied with policy_requirement_unmet; got %+v", dec)
	}

	// 5. HTTP API evaluation check: POST /v1/evaluate
	body, _ := json.Marshal(map[string]any{
		"subject_id": "cust-unsafe-200",
		"purpose":    "marketing",
		"channel":    "email",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "service-client")
	req.Header.Set("X-Nvuto-Roles", "service")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP /v1/evaluate expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var httpDec domain.SuppressionDecision
	if err := json.Unmarshal(rec.Body.Bytes(), &httpDec); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if httpDec.Allowed || httpDec.Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("HTTP evaluate expected allowed=false, reason=policy_requirement_unmet; got %+v", httpDec)
	}
}

// TestAT008_PolicyTighteningQuarantinesLegacy verifies that if an originally
// un-governed purpose is tightened to require affirmative consent or DOI,
// existing legacy grants are dynamically quarantined.
func TestAT008_PolicyTighteningQuarantinesLegacy(t *testing.T) {
	tenantID := "tenant-leg-tighten"
	svc, _, repo := setupLegacyTestHarness(t, tenantID)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Initial purpose without affirmative consent or evidence requirements
	_, err := svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "notifications",
		Category:                   "service",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: false,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "contract",
		AllowedCategories: []string{"service"},
		RequiresEvidence:  false,
		Active:            true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 2. Inject legacy grant
	legacyGrant := domain.Consent{
		ID:            "con-legacy-tighten",
		TenantID:      tenantID,
		SubjectID:     "cust-tighten-300",
		Purpose:       "notifications",
		Channel:       "email",
		LegalBasis:    "contract",
		Status:        domain.StatusGranted,
		Source:        "legacy",
		NoticeVersion: "",
		Version:       1,
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now.Add(-24 * time.Hour),
	}
	repo.addLegacy(legacyGrant)

	// 3. Before tightening: allowed
	before, err := svc.Evaluate(ctx, tenantID, "cust-tighten-300", "notifications", "email", now)
	if err != nil || !before.Allowed || before.Reason != domain.DecisionReasonGranted {
		t.Fatalf("expected allowed before policy tightening, got %+v (err=%v)", before, err)
	}

	// 4. Policy tightening: require affirmative consent on notifications
	_, err = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "notifications",
		Category:                   "service",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true, // TIGHTENED
		RequiresDoubleOptIn:        false,
		Active:                     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 5. After tightening: dynamically quarantined (no silent bypass)
	after, err := svc.Evaluate(ctx, tenantID, "cust-tighten-300", "notifications", "email", now)
	if err != nil {
		t.Fatal(err)
	}
	if after.Allowed || after.Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Fatalf("expected legacy record quarantined with policy_requirement_unmet after policy tightening, got %+v", after)
	}
}

// TestAT008_LegacyTerminalStatesPreserved verifies that historic withdrawn,
// denied, or expired legacy records continue to return their respective
// terminal reasons without being altered.
func TestAT008_LegacyTerminalStatesPreserved(t *testing.T) {
	tenantID := "tenant-leg-terminal"
	svc, _, repo := setupLegacyTestHarness(t, tenantID)
	ctx := context.Background()
	now := time.Now().UTC()

	// Purpose & basis
	_, _ = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:            "updates",
		Category:        "service",
		AllowedChannels: []string{"email"},
		Active:          true,
	})
	_, _ = svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "contract",
		AllowedCategories: []string{"service"},
		Active:            true,
	})

	testCases := []struct {
		name           string
		status         domain.Status
		expectedReason string
	}{
		{"Withdrawn", domain.StatusWithdrawn, "withdrawn"},
		{"Denied", domain.StatusDenied, "denied"},
		{"Expired", domain.StatusExpired, "expired"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			subID := "sub-" + tc.name
			c := domain.Consent{
				ID:            "con-" + tc.name,
				TenantID:      tenantID,
				SubjectID:     subID,
				Purpose:       "updates",
				Channel:       "email",
				LegalBasis:    "contract",
				Status:        tc.status,
				Source:        "legacy",
				NoticeVersion: "",
				Version:       1,
				CreatedAt:     now.Add(-100 * time.Hour),
				UpdatedAt:     now.Add(-100 * time.Hour),
			}
			if tc.status == domain.StatusExpired {
				exp := now.Add(-time.Hour)
				c.ExpiresAt = &exp
				c.Status = domain.StatusGranted
			}
			repo.addLegacy(c)

			dec, err := svc.Evaluate(ctx, tenantID, subID, "updates", "email", now)
			if err != nil {
				t.Fatal(err)
			}
			if dec.Allowed || dec.Reason != tc.expectedReason {
				t.Fatalf("expected allowed=false, reason=%s; got %+v", tc.expectedReason, dec)
			}
		})
	}
}

// TestAT008_BatchEvaluationLegacyAndGoverned verifies batch evaluation with
// a mixture of safe legacy, unsafe legacy, withdrawn legacy, and modern governed records.
func TestAT008_BatchEvaluationLegacyAndGoverned(t *testing.T) {
	tenantID := "tenant-leg-batch"
	svc, handler, repo := setupLegacyTestHarness(t, tenantID)
	ctx := context.Background()
	now := time.Now().UTC()
	publishedAt := now.Add(-10 * time.Hour)

	// Configure purposes
	_, _ = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "service_updates",
		Category:                   "service",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: false,
		Active:                     true,
	})
	_, _ = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		Active:                     true,
	})
	_, _ = svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "contract",
		AllowedCategories: []string{"service"},
		Active:            true,
	})
	_, _ = svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "consent",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	})
	_, err := svc.PublishNotice(ctx, tenantID, "admin", domain.PrivacyNotice{
		Version:      "2026-08",
		Jurisdiction: "NG",
		PublishedAt:  publishedAt,
		Active:       true,
		Content:      "Notice Content",
	})
	if err != nil {
		t.Fatalf("failed to publish notice: %v", err)
	}

	// 1. Safe legacy: service_updates / contract
	repo.addLegacy(domain.Consent{
		ID: "con-1", TenantID: tenantID, SubjectID: "sub-1", Purpose: "service_updates", Channel: "email",
		LegalBasis: "contract", Status: domain.StatusGranted, Source: "legacy", NoticeVersion: "", Version: 1,
		CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour),
	})

	// 2. Unsafe legacy: marketing / consent (blank notice)
	repo.addLegacy(domain.Consent{
		ID: "con-2", TenantID: tenantID, SubjectID: "sub-2", Purpose: "marketing", Channel: "email",
		LegalBasis: "consent", Status: domain.StatusGranted, Source: "legacy", NoticeVersion: "", Version: 1,
		CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour),
	})

	// 3. Terminal legacy: withdrawn
	repo.addLegacy(domain.Consent{
		ID: "con-3", TenantID: tenantID, SubjectID: "sub-3", Purpose: "service_updates", Channel: "email",
		LegalBasis: "contract", Status: domain.StatusWithdrawn, Source: "legacy", NoticeVersion: "", Version: 2,
		CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-12 * time.Hour),
	})

	// 4. Modern governed: marketing / consent with valid notice & evidence
	evidence := domain.Evidence{
		Method:       "web_form",
		ArtifactHash: validTestArtifactHash,
		CollectedAt:  publishedAt.Add(time.Hour),
	}
	_, err = svc.RecordGoverned(ctx, tenantID, "admin", "sub-4", "marketing", "email", "consent", "granted", "web", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("failed to record modern governed grant: %v", err)
	}

	// Execute Batch via HTTP POST /v1/evaluate:batch
	batchBody, _ := json.Marshal(map[string]any{
		"requests": []map[string]any{
			{"subject_id": "sub-1", "purpose": "service_updates", "channel": "email"},
			{"subject_id": "sub-2", "purpose": "marketing", "channel": "email"},
			{"subject_id": "sub-3", "purpose": "service_updates", "channel": "email"},
			{"subject_id": "sub-4", "purpose": "marketing", "channel": "email"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate:batch", bytes.NewReader(batchBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nvuto-Tenant-ID", tenantID)
	req.Header.Set("X-Nvuto-Actor-ID", "service-client")
	req.Header.Set("X-Nvuto-Roles", "service")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP batch evaluate failed: %d %s", rec.Code, rec.Body.String())
	}

	var batchResp struct {
		Decisions []domain.SuppressionDecision `json:"decisions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &batchResp); err != nil {
		t.Fatalf("failed to decode batch response: %v", err)
	}
	if len(batchResp.Decisions) != 4 {
		t.Fatalf("expected 4 decisions, got %d", len(batchResp.Decisions))
	}

	// Decision 1: Safe legacy -> allowed=true, reason=granted
	if !batchResp.Decisions[0].Allowed || batchResp.Decisions[0].Reason != domain.DecisionReasonGranted {
		t.Errorf("decision 0 (safe legacy) expected allowed=true, reason=granted; got %+v", batchResp.Decisions[0])
	}

	// Decision 2: Unsafe legacy -> allowed=false, reason=policy_requirement_unmet
	if batchResp.Decisions[1].Allowed || batchResp.Decisions[1].Reason != domain.DecisionReasonPolicyRequirementUnmet {
		t.Errorf("decision 1 (unsafe legacy) expected allowed=false, reason=policy_requirement_unmet; got %+v", batchResp.Decisions[1])
	}

	// Decision 3: Withdrawn legacy -> allowed=false, reason=withdrawn
	if batchResp.Decisions[2].Allowed || batchResp.Decisions[2].Reason != "withdrawn" {
		t.Errorf("decision 2 (withdrawn legacy) expected allowed=false, reason=withdrawn; got %+v", batchResp.Decisions[2])
	}

	// Decision 4: Modern governed -> allowed=true, reason=granted
	if !batchResp.Decisions[3].Allowed || batchResp.Decisions[3].Reason != domain.DecisionReasonGranted {
		t.Errorf("decision 3 (modern governed) expected allowed=true, reason=granted; got %+v", batchResp.Decisions[3])
	}
}

// TestAT008_ZeroEvidenceLoss verifies that evaluating legacy records never
// mutates, overwrites, or deletes records from the repository.
func TestAT008_ZeroEvidenceLoss(t *testing.T) {
	tenantID := "tenant-leg-immutability"
	svc, _, repo := setupLegacyTestHarness(t, tenantID)
	ctx := context.Background()
	now := time.Now().UTC()

	_, _ = svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code: "marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, Active: true,
	})
	_, _ = svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code: "consent", AllowedCategories: []string{"marketing"}, RequiresEvidence: true, Active: true,
	})

	originalGrant := domain.Consent{
		ID:            "con-immutability-1",
		TenantID:      tenantID,
		SubjectID:     "cust-immutable-999",
		Purpose:       "marketing",
		Channel:       "email",
		LegalBasis:    "consent",
		Status:        domain.StatusGranted,
		Source:        "legacy-original-source",
		NoticeVersion: "",
		Version:       1,
		CreatedAt:     now.Add(-100 * time.Hour),
		UpdatedAt:     now.Add(-100 * time.Hour),
	}
	repo.addLegacy(originalGrant)

	// Evaluate 10 times consecutively
	for i := 0; i < 10; i++ {
		_, err := svc.Evaluate(ctx, tenantID, "cust-immutable-999", "marketing", "email", now)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Fetch record back from store
	stored, err := repo.Get(ctx, tenantID, "con-immutability-1")
	if err != nil {
		t.Fatalf("failed to retrieve grant after evaluation: %v", err)
	}

	// Verify all fields remain completely intact
	if stored.ID != originalGrant.ID ||
		stored.SubjectID != originalGrant.SubjectID ||
		stored.Status != originalGrant.Status ||
		stored.Source != originalGrant.Source ||
		stored.NoticeVersion != "" ||
		stored.Version != 1 ||
		!stored.CreatedAt.Equal(originalGrant.CreatedAt) ||
		!stored.UpdatedAt.Equal(originalGrant.UpdatedAt) {
		t.Fatalf("legacy record was mutated by evaluation! Original: %+v, Stored: %+v", originalGrant, stored)
	}
}
