package service

import (
	"context"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

func TestAuditHistoryReconstructionPolicyOverwrite(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)

	// 1. Initial purpose registration (Revision 1)
	p1, err := svc.RegisterPurpose(context.Background(), "tenant-audit", "admin-1", domain.Purpose{
		Code:            "marketing",
		Category:        "marketing",
		AllowedChannels: []string{"email"},
		Active:          true,
	})
	if err != nil {
		t.Fatalf("RegisterPurpose 1 failed: %v", err)
	}
	if p1.Version != 1 {
		t.Fatalf("expected version 1, got %d", p1.Version)
	}

	auditTrail := repo.Audit()
	if len(auditTrail) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(auditTrail))
	}
	ev1 := auditTrail[0]
	if ev1.ResourceType != "purpose" {
		t.Fatalf("expected resource_type purpose, got %q", ev1.ResourceType)
	}
	if ev1.ConsentID != "marketing" {
		t.Fatalf("expected consent_id marketing, got %q", ev1.ConsentID)
	}
	if ev1.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", ev1.Revision)
	}
	if ev1.Changes == nil || ev1.Changes["before"] != nil || ev1.Changes["after"] == nil {
		t.Fatalf("expected before=nil, after!=nil in changes, got %+v", ev1.Changes)
	}

	// 2. Overwrite purpose (Revision 2)
	p2, err := svc.RegisterPurpose(context.Background(), "tenant-audit", "admin-2", domain.Purpose{
		Code:            "marketing",
		Category:        "marketing",
		AllowedChannels: []string{"email", "whatsapp"},
		Active:          true,
	})
	if err != nil {
		t.Fatalf("RegisterPurpose 2 failed: %v", err)
	}
	if p2.Version != 2 {
		t.Fatalf("expected version 2, got %d", p2.Version)
	}

	auditTrail = repo.Audit()
	if len(auditTrail) != 2 {
		t.Fatalf("expected 2 audit events, got %d", len(auditTrail))
	}
	ev2 := auditTrail[1]
	if ev2.ResourceType != "purpose" {
		t.Fatalf("expected resource_type purpose, got %q", ev2.ResourceType)
	}
	if ev2.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", ev2.Revision)
	}
	beforeP, ok := ev2.Changes["before"].(domain.Purpose)
	if !ok || beforeP.Version != 1 || len(beforeP.AllowedChannels) != 1 {
		t.Fatalf("expected before to contain version 1 purpose, got %+v", ev2.Changes["before"])
	}
	afterP, ok := ev2.Changes["after"].(domain.Purpose)
	if !ok || afterP.Version != 2 || len(afterP.AllowedChannels) != 2 {
		t.Fatalf("expected after to contain version 2 purpose, got %+v", ev2.Changes["after"])
	}

	// 3. Initial lawful basis registration (Revision 1)
	b1, err := svc.RegisterLawfulBasis(context.Background(), "tenant-audit", "admin-1", domain.LawfulBasis{
		Code:              "consent",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	})
	if err != nil {
		t.Fatalf("RegisterLawfulBasis 1 failed: %v", err)
	}
	if b1.Version != 1 {
		t.Fatalf("expected lawful basis version 1, got %d", b1.Version)
	}

	// 4. Overwrite lawful basis (Revision 2)
	b2, err := svc.RegisterLawfulBasis(context.Background(), "tenant-audit", "admin-2", domain.LawfulBasis{
		Code:              "consent",
		AllowedCategories: []string{"marketing", "analytics"},
		RequiresEvidence:  false,
		Active:            true,
	})
	if err != nil {
		t.Fatalf("RegisterLawfulBasis 2 failed: %v", err)
	}
	if b2.Version != 2 {
		t.Fatalf("expected lawful basis version 2, got %d", b2.Version)
	}

	auditTrail = repo.Audit()
	evLawful2 := auditTrail[3]
	if evLawful2.ResourceType != "lawful_basis" || evLawful2.Revision != 2 {
		t.Fatalf("expected lawful_basis revision 2, got %+v", evLawful2)
	}
	beforeB, ok := evLawful2.Changes["before"].(domain.LawfulBasis)
	if !ok || beforeB.Version != 1 {
		t.Fatalf("expected before lawful basis version 1, got %+v", evLawful2.Changes["before"])
	}
	afterB, ok := evLawful2.Changes["after"].(domain.LawfulBasis)
	if !ok || afterB.Version != 2 {
		t.Fatalf("expected after lawful basis version 2, got %+v", evLawful2.Changes["after"])
	}
}

func TestAuditHistoryReconstructionSuppressionAndWithdrawal(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	installPolicyFixture(t, svc, "tenant-audit")

	// 1. Initial suppression (Revision 1)
	s1, err := svc.SetSuppression(context.Background(), "tenant-audit", "admin-1", domain.Suppression{
		SubjectID: "cus-99",
		Scope:     domain.SuppressionChannel,
		Channel:   "email",
		Reason:    "user_request",
		Active:    true,
	})
	if err != nil {
		t.Fatalf("SetSuppression 1 failed: %v", err)
	}
	if s1.Version != 1 {
		t.Fatalf("expected suppression version 1, got %d", s1.Version)
	}

	// 2. Overwrite suppression (Revision 2)
	s2, err := svc.SetSuppression(context.Background(), "tenant-audit", "admin-2", domain.Suppression{
		SubjectID: "cus-99",
		Scope:     domain.SuppressionChannel,
		Channel:   "email",
		Reason:    "user_unsubscribed",
		Active:    false,
	})
	if err != nil {
		t.Fatalf("SetSuppression 2 failed: %v", err)
	}
	if s2.Version != 2 {
		t.Fatalf("expected suppression version 2, got %d", s2.Version)
	}

	trail := repo.Audit()
	// Find suppression audit events
	var suppressionAudits []domain.AuditEvent
	for _, a := range trail {
		if a.ResourceType == "suppression" {
			suppressionAudits = append(suppressionAudits, a)
		}
	}
	if len(suppressionAudits) != 2 {
		t.Fatalf("expected 2 suppression audit events, got %d", len(suppressionAudits))
	}
	if suppressionAudits[1].Revision != 2 {
		t.Fatalf("expected revision 2 on overwrite, got %d", suppressionAudits[1].Revision)
	}
	beforeSup, ok := suppressionAudits[1].Changes["before"].(domain.Suppression)
	if !ok || beforeSup.Version != 1 || beforeSup.Reason != "user_request" {
		t.Fatalf("expected before suppression with version 1, got %+v", suppressionAudits[1].Changes["before"])
	}
	afterSup, ok := suppressionAudits[1].Changes["after"].(domain.Suppression)
	if !ok || afterSup.Version != 2 || afterSup.Reason != "user_unsubscribed" {
		t.Fatalf("expected after suppression with version 2, got %+v", suppressionAudits[1].Changes["after"])
	}

	// 3. Consent withdrawal captures before and after state
	evidence := domain.Evidence{Method: "preference_center", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()}
	c, err := svc.RecordGoverned(context.Background(), "tenant-audit", "actor-1", "cus-100", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("RecordGoverned failed: %v", err)
	}
	withdrawn, err := svc.Withdraw(context.Background(), "tenant-audit", "actor-2", c.ID)
	if err != nil {
		t.Fatalf("Withdraw failed: %v", err)
	}
	if withdrawn.Version != 2 {
		t.Fatalf("expected withdrawn version 2, got %d", withdrawn.Version)
	}

	trail = repo.Audit()
	var consentAudits []domain.AuditEvent
	for _, a := range trail {
		if a.ResourceType == "consent" && a.ConsentID == c.ID {
			consentAudits = append(consentAudits, a)
		}
	}
	if len(consentAudits) != 2 {
		t.Fatalf("expected 2 consent audits (granted, withdrawn), got %d", len(consentAudits))
	}
	withdrawAudit := consentAudits[1]
	if withdrawAudit.Revision != 2 {
		t.Fatalf("expected withdrawal revision 2, got %d", withdrawAudit.Revision)
	}
	beforeConsent, ok := withdrawAudit.Changes["before"].(domain.Consent)
	if !ok || beforeConsent.Status != domain.StatusGranted {
		t.Fatalf("expected before consent status granted, got %+v", withdrawAudit.Changes["before"])
	}
	afterConsent, ok := withdrawAudit.Changes["after"].(domain.Consent)
	if !ok || afterConsent.Status != domain.StatusWithdrawn {
		t.Fatalf("expected after consent status withdrawn, got %+v", withdrawAudit.Changes["after"])
	}
}

func TestSuppressionScopeShapeConstraints(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)

	// Valid global scope (purpose and channel blank)
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{
		SubjectID: "cus-1",
		Scope:     domain.SuppressionGlobal,
		Reason:    "opt-out-all",
		Active:    true,
	}); err != nil {
		t.Fatalf("valid global suppression failed: %v", err)
	}

	// Invalid global scope (names purpose)
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{
		SubjectID: "cus-1",
		Scope:     domain.SuppressionGlobal,
		Purpose:   "marketing",
		Reason:    "invalid",
		Active:    true,
	}); err == nil {
		t.Fatalf("global suppression with purpose must fail")
	}

	// Invalid global scope (names channel)
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{
		SubjectID: "cus-1",
		Scope:     domain.SuppressionGlobal,
		Channel:   "email",
		Reason:    "invalid",
		Active:    true,
	}); err == nil {
		t.Fatalf("global suppression with channel must fail")
	}

	// Valid channel scope (channel present, purpose blank)
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{
		SubjectID: "cus-1",
		Scope:     domain.SuppressionChannel,
		Channel:   "email",
		Reason:    "no-email",
		Active:    true,
	}); err != nil {
		t.Fatalf("valid channel suppression failed: %v", err)
	}

	// Invalid channel scope (missing channel)
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{
		SubjectID: "cus-1",
		Scope:     domain.SuppressionChannel,
		Reason:    "invalid",
		Active:    true,
	}); err == nil {
		t.Fatalf("channel suppression without channel must fail")
	}

	// Invalid channel scope (names purpose)
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{
		SubjectID: "cus-1",
		Scope:     domain.SuppressionChannel,
		Channel:   "email",
		Purpose:   "marketing",
		Reason:    "invalid",
		Active:    true,
	}); err == nil {
		t.Fatalf("channel suppression with purpose must fail")
	}

	// Valid purpose_channel scope (both purpose and channel present)
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{
		SubjectID: "cus-1",
		Scope:     domain.SuppressionPurposeChannel,
		Purpose:   "marketing",
		Channel:   "email",
		Reason:    "no-marketing-email",
		Active:    true,
	}); err != nil {
		t.Fatalf("valid purpose_channel suppression failed: %v", err)
	}

	// Invalid purpose_channel scope (missing purpose)
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{
		SubjectID: "cus-1",
		Scope:     domain.SuppressionPurposeChannel,
		Channel:   "email",
		Reason:    "invalid",
		Active:    true,
	}); err == nil {
		t.Fatalf("purpose_channel suppression without purpose must fail")
	}

	// Invalid purpose_channel scope (missing channel)
	if _, err := svc.SetSuppression(context.Background(), "tenant-a", "admin", domain.Suppression{
		SubjectID: "cus-1",
		Scope:     domain.SuppressionPurposeChannel,
		Purpose:   "marketing",
		Reason:    "invalid",
		Active:    true,
	}); err == nil {
		t.Fatalf("purpose_channel suppression without channel must fail")
	}
}
