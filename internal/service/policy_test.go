package service

import (
	"context"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

func installPolicyFixture(t *testing.T, svc *Service, tenant string) {
	t.Helper()
	if _, err := svc.RegisterPurpose(context.Background(), tenant, "admin", domain.Purpose{Code: "marketing", Category: "marketing", AllowedChannels: []string{"email", "whatsapp"}, RequiresAffirmativeConsent: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterPurpose(context.Background(), tenant, "admin", domain.Purpose{Code: "double_marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, RequiresDoubleOptIn: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterPurpose(context.Background(), tenant, "admin", domain.Purpose{Code: "service_updates", Category: "service", AllowedChannels: []string{"sms", "email"}, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterLawfulBasis(context.Background(), tenant, "admin", domain.LawfulBasis{Code: "consent", AllowedCategories: []string{"marketing", "service"}, RequiresEvidence: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterLawfulBasis(context.Background(), tenant, "admin", domain.LawfulBasis{Code: "contract", AllowedCategories: []string{"service"}, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishNotice(context.Background(), tenant, "admin", domain.PrivacyNotice{Version: "2026-08", Jurisdiction: "NG", PublishedAt: time.Now().UTC().Add(-time.Hour), Active: true}); err != nil {
		t.Fatal(err)
	}
}

func TestGovernedConsentCreatesReceiptAndEnforcesRegistries(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	installPolicyFixture(t, svc, "tenant-a")
	evidence := domain.Evidence{Method: "preference_center", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()}
	consent, err := svc.RecordGoverned(context.Background(), "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}
	if consent.ReceiptID == "" || consent.NoticeVersion != "2026-08" || consent.Jurisdiction != "NG" {
		t.Fatalf("governed consent lost receipt policy context: %+v", consent)
	}
	receipt, err := repo.GetReceipt(context.Background(), "tenant-a", consent.ReceiptID)
	if err != nil || receipt.ConsentID != consent.ID || receipt.Evidence.ArtifactHash != "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" {
		t.Fatalf("immutable receipt missing evidence: receipt=%+v err=%v", receipt, err)
	}
	decision, err := svc.Evaluate(context.Background(), "tenant-a", "cus-1", "marketing", "email", time.Time{})
	if err != nil || !decision.Allowed || decision.Reason != "granted" || decision.ReceiptID != receipt.ID {
		t.Fatalf("governed decision failed: decision=%+v err=%v", decision, err)
	}
	if _, err := svc.RecordGoverned(context.Background(), "tenant-a", "subject-1", "cus-2", "marketing", "sms", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence); err == nil {
		t.Fatal("purpose channel registry must reject unsupported channel")
	}
	if _, err := svc.RecordGoverned(context.Background(), "tenant-a", "subject-1", "cus-2", "marketing", "email", "contract", "granted", "portal", nil, nil, "NG", "2026-08", evidence); err == nil {
		t.Fatal("lawful basis category mismatch must be rejected")
	}
	if _, err := svc.RecordGoverned(context.Background(), "tenant-a", "subject-1", "cus-2", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", domain.Evidence{}); err == nil {
		t.Fatal("evidence-required lawful basis must reject missing evidence")
	}
	futureEvidence := evidence
	futureEvidence.CollectedAt = time.Now().UTC().Add(time.Hour)
	if _, err := svc.RecordGoverned(context.Background(), "tenant-a", "subject-1", "cus-2", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", futureEvidence); err == nil {
		t.Fatal("future-dated evidence must be rejected")
	}
	if _, err := svc.RecordGoverned(context.Background(), "tenant-b", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence); err == nil {
		t.Fatal("tenant without policy registry must fail closed")
	}
	if _, err := svc.RegisterPurpose(context.Background(), "tenant-a", "admin", domain.Purpose{Code: "marketing", Category: "marketing", AllowedChannels: []string{"email"}, RequiresAffirmativeConsent: true, Active: false}); err != nil {
		t.Fatal(err)
	}
	decision, err = svc.Evaluate(context.Background(), "tenant-a", "cus-1", "marketing", "email", time.Time{})
	if err != nil || decision.Allowed || (decision.Reason != domain.DecisionReasonPurposeInactive && decision.Reason != "policy_unavailable") {
		t.Fatalf("deactivated purpose must fail closed: decision=%+v err=%v", decision, err)
	}
}

func TestSuppressionHierarchyAndBatchDecisions(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	installPolicyFixture(t, svc, "tenant-a")
	evidence := domain.Evidence{Method: "agent", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()}
	for _, subject := range []string{"global", "channel", "purpose", "allowed"} {
		if _, err := svc.RecordGoverned(context.Background(), "tenant-a", "admin", subject, "marketing", "email", "consent", "granted", "admin", nil, nil, "NG", "2026-08", evidence); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{SubjectID: "global", Scope: domain.SuppressionGlobal, Reason: "privacy_request", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{SubjectID: "channel", Scope: domain.SuppressionChannel, Channel: "email", Reason: "channel_opt_out", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{SubjectID: "purpose", Scope: domain.SuppressionPurposeChannel, Purpose: "marketing", Channel: "email", Reason: "purpose_opt_out", Active: true}); err != nil {
		t.Fatal(err)
	}
	requests := []domain.EvaluationRequest{
		{SubjectID: "global", Purpose: "marketing", Channel: "email"},
		{SubjectID: "channel", Purpose: "marketing", Channel: "email"},
		{SubjectID: "purpose", Purpose: "marketing", Channel: "email"},
		{SubjectID: "allowed", Purpose: "marketing", Channel: "email"},
	}
	decisions, err := svc.BatchEvaluate(context.Background(), "tenant-a", requests)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"global_suppression", "channel_suppression", "purpose_channel_suppression", "granted"}
	for i := range decisions {
		if decisions[i].Reason != want[i] || decisions[i].Allowed != (i == 3) {
			t.Fatalf("suppression precedence mismatch at %d: %+v", i, decisions[i])
		}
	}
}

func TestDoubleOptInRequiresSingleUseConfirmation(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	installPolicyFixture(t, svc, "tenant-a")
	evidence := domain.Evidence{Method: "signup_form", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()}
	pending, err := svc.StartDoubleOptIn(context.Background(), "tenant-a", "subject-1", "cus-double", "double_marketing", "email", "portal", "NG", "2026-08", evidence, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if pending.ConfirmationToken == "" || pending.Consent.Status != domain.StatusPendingConfirmation {
		t.Fatalf("double opt-in did not create pending challenge: %+v", pending)
	}
	decision, _ := svc.Evaluate(context.Background(), "tenant-a", "cus-double", "double_marketing", "email", time.Time{})
	if decision.Allowed || decision.Reason != "pending_confirmation" {
		t.Fatalf("pending consent must suppress: %+v", decision)
	}
	if _, err := svc.ConfirmDoubleOptIn(context.Background(), "tenant-a", "subject-1", TenantWide(), "wrong-token"); err == nil {
		t.Fatal("wrong confirmation token must fail")
	}
	confirmed, err := svc.ConfirmDoubleOptIn(context.Background(), "tenant-a", "subject-1", TenantWide(), pending.ConfirmationToken)
	if err != nil || confirmed.Status != domain.StatusGranted || confirmed.ReceiptID == "" || confirmed.ConfirmedAt == nil {
		t.Fatalf("double opt-in confirmation failed: consent=%+v err=%v", confirmed, err)
	}
	if _, err := svc.ConfirmDoubleOptIn(context.Background(), "tenant-a", "subject-1", TenantWide(), pending.ConfirmationToken); err == nil {
		t.Fatal("confirmation token must be single use")
	}
	decision, _ = svc.Evaluate(context.Background(), "tenant-a", "cus-double", "double_marketing", "email", time.Time{})
	if !decision.Allowed || decision.Reason != "granted" {
		t.Fatalf("confirmed consent should allow: %+v", decision)
	}
}
