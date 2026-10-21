package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// setupAT005Fixture initializes policy registry with explicit notice content.
func setupAT005Fixture(t *testing.T, repo store.Repository, tenantID string) (*service.Service, domain.PrivacyNotice, domain.Evidence) {
	t.Helper()
	svc := service.New(repo)
	ctx := context.Background()

	// Register Purpose
	if _, err := svc.RegisterPurpose(ctx, tenantID, "admin", domain.Purpose{
		Code:                       "marketing",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Active:                     true,
	}); err != nil && !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}

	// Register Lawful Basis
	if _, err := svc.RegisterLawfulBasis(ctx, tenantID, "admin", domain.LawfulBasis{
		Code:              "consent",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	}); err != nil && !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}

	// Publish Notice with explicit content
	noticeContent := "Terms and Privacy Notice 2026-08: Complete consent policy for NG jurisdiction."
	digest := sha256.Sum256([]byte(noticeContent))
	expectedHash := "sha256:" + hex.EncodeToString(digest[:])

	notice, err := svc.PublishNotice(ctx, tenantID, "admin", domain.PrivacyNotice{
		Version:      "2026-08",
		Jurisdiction: "NG",
		PublishedAt:  time.Now().UTC().Add(-2 * time.Hour),
		Active:       true,
		Content:      noticeContent,
	})
	if err != nil && !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	if notice.ContentHash != "" && notice.ContentHash != expectedHash {
		t.Fatalf("expected content_hash %s, got %s", expectedHash, notice.ContentHash)
	}

	evidence := domain.Evidence{
		Method:       "web_form",
		ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		CollectedAt:  time.Now().UTC().Add(-time.Hour),
	}

	return svc, notice, evidence
}

// TestAT005_ExportedReceiptResolvesAndVerifiesNoticeAndDecision proves that an exported receipt
// can cryptographically resolve and verify the exact accepted notice artifact and full decision metadata (T-005).
func TestAT005_ExportedReceiptResolvesAndVerifiesNoticeAndDecision(t *testing.T) {
	repo := store.NewMemory()
	tenantID := "tenant-at005-verify"
	svc, notice, evidence := setupAT005Fixture(t, repo, tenantID)
	ctx := context.Background()

	// 1. Record governed consent decision
	actorID := "admin-actor-101"
	subjectID := "sub-user-202"
	consent, err := svc.RecordGoverned(ctx, tenantID, actorID, subjectID, "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("RecordGoverned failed: %v", err)
	}
	if consent.ReceiptID == "" {
		t.Fatal("expected receipt_id on recorded consent")
	}

	// 2. Fetch receipt from service
	originalReceipt, err := svc.Receipt(ctx, tenantID, consent.ReceiptID)
	if err != nil {
		t.Fatalf("Receipt failed: %v", err)
	}

	// Verify receipt embeds full decision metadata
	if originalReceipt.ActorID != actorID {
		t.Errorf("expected ActorID %s, got %s", actorID, originalReceipt.ActorID)
	}
	if originalReceipt.SubjectID != subjectID {
		t.Errorf("expected SubjectID %s, got %s", subjectID, originalReceipt.SubjectID)
	}
	if originalReceipt.ConsentStatus != domain.StatusGranted {
		t.Errorf("expected ConsentStatus %s, got %s", domain.StatusGranted, originalReceipt.ConsentStatus)
	}
	if originalReceipt.LegalBasis != "consent" {
		t.Errorf("expected LegalBasis 'consent', got %s", originalReceipt.LegalBasis)
	}
	if originalReceipt.PolicyVersion <= 0 {
		t.Errorf("expected positive PolicyVersion, got %d", originalReceipt.PolicyVersion)
	}
	if originalReceipt.NoticeContentHash != notice.ContentHash {
		t.Errorf("expected NoticeContentHash %s, got %s", notice.ContentHash, originalReceipt.NoticeContentHash)
	}

	// 3. Export receipt to JSON and import into a detached struct
	exportedJSON, err := json.Marshal(originalReceipt)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var importedReceipt domain.ConsentReceipt
	if err := json.Unmarshal(exportedJSON, &importedReceipt); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// 4. Resolve and verify the exported receipt artifact against the repository
	verification, resolvedNotice, err := svc.ResolveAndVerifyReceipt(ctx, importedReceipt)
	if err != nil {
		t.Fatalf("ResolveAndVerifyReceipt failed: %v", err)
	}
	if verification.Status != service.ReceiptHashVerified || !verification.Trusted() {
		t.Fatalf("expected verification to be verified, got: %+v", verification)
	}
	if resolvedNotice.Version != "2026-08" || resolvedNotice.Jurisdiction != "NG" {
		t.Fatalf("resolved notice mismatch: version=%s jurisdiction=%s", resolvedNotice.Version, resolvedNotice.Jurisdiction)
	}
	if resolvedNotice.ContentHash != importedReceipt.NoticeContentHash {
		t.Fatalf("resolved notice content hash %s does not match receipt notice_content_hash %s", resolvedNotice.ContentHash, importedReceipt.NoticeContentHash)
	}

	// 5. Prove tampering with any full decision metadata field invalidates the receipt hash
	tamperTests := map[string]func(r domain.ConsentReceipt) domain.ConsentReceipt{
		"tamper status": func(r domain.ConsentReceipt) domain.ConsentReceipt {
			r.ConsentStatus = domain.StatusDenied
			return r
		},
		"tamper legal_basis": func(r domain.ConsentReceipt) domain.ConsentReceipt {
			r.LegalBasis = "legitimate_interest"
			return r
		},
		"tamper actor_id": func(r domain.ConsentReceipt) domain.ConsentReceipt {
			r.ActorID = "attacker"
			return r
		},
		"tamper subject_id": func(r domain.ConsentReceipt) domain.ConsentReceipt {
			r.SubjectID = "other-subject"
			return r
		},
		"tamper policy_version": func(r domain.ConsentReceipt) domain.ConsentReceipt {
			r.PolicyVersion = 999
			return r
		},
		"tamper notice_content_hash": func(r domain.ConsentReceipt) domain.ConsentReceipt {
			r.NoticeContentHash = "sha256:tamperedhash"
			return r
		},
		"tamper notice_version": func(r domain.ConsentReceipt) domain.ConsentReceipt {
			r.NoticeVersion = "2026-09"
			return r
		},
		"tamper jurisdiction": func(r domain.ConsentReceipt) domain.ConsentReceipt {
			r.Jurisdiction = "GH"
			return r
		},
	}

	for name, tamper := range tamperTests {
		t.Run(name, func(t *testing.T) {
			tampered := tamper(importedReceipt)
			v := service.VerifyReceipt(tampered)
			if v.Status != service.ReceiptHashMismatch || v.Trusted() {
				t.Fatalf("expected ReceiptHashMismatch for %s, got: %+v", name, v)
			}
		})
	}
}

// TestAT005_AlteringNoticeRowInvalidatesReceiptVerification proves that directly altering a notice row
// (or tampered content hash) causes receipt verification to fail with ReceiptHashMismatch (T-005).
func TestAT005_AlteringNoticeRowInvalidatesReceiptVerification(t *testing.T) {
	repo := store.NewMemory()
	tenantID := "tenant-at005-alter"
	svc, notice, evidence := setupAT005Fixture(t, repo, tenantID)
	ctx := context.Background()

	consent, err := svc.RecordGoverned(ctx, tenantID, "admin", "sub-alter-1", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}

	receipt, err := svc.Receipt(ctx, tenantID, consent.ReceiptID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify clean initially
	v, err := svc.VerifyReceipt(ctx, tenantID, receipt.ID)
	if err != nil || v.Status != service.ReceiptHashVerified {
		t.Fatalf("initial verification should succeed, got: %+v, err=%v", v, err)
	}

	// Now simulate an altered notice row in the repository by re-saving altered notice content/hash
	alteredNotice := notice
	alteredNotice.Content = "TAMPERED AND ALTERED POLICY CONTENT"
	alteredNoticeDigest := sha256.Sum256([]byte(alteredNotice.Content))
	alteredNotice.ContentHash = "sha256:" + hex.EncodeToString(alteredNoticeDigest[:])

	// Verify using VerifyReceiptArtifact with the altered notice
	artifactVerification := service.VerifyReceiptArtifact(receipt, alteredNotice)
	if artifactVerification.Status != service.ReceiptHashMismatch || artifactVerification.Trusted() {
		t.Fatalf("altering notice content hash must fail verification, got: %+v", artifactVerification)
	}
	if !strings.Contains(artifactVerification.Detail, "notice_content_hash") {
		t.Fatalf("expected detail mentioning notice_content_hash mismatch, got: %s", artifactVerification.Detail)
	}
}

// TestAT005_RetiringNoticePreservesReceiptsAndControlsNewConsents proves that retiring a notice
// preserves existing receipts (they still verify) and strictly blocks new consents under the retired version (T-005).
func TestAT005_RetiringNoticePreservesReceiptsAndControlsNewConsents(t *testing.T) {
	repo := store.NewMemory()
	tenantID := "tenant-at005-retire"
	svc, notice, evidence := setupAT005Fixture(t, repo, tenantID)
	ctx := context.Background()

	// 1. Grant consent under active notice v1 ("2026-08")
	consent1, err := svc.RecordGoverned(ctx, tenantID, "admin", "sub-retire-1", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("initial grant failed: %v", err)
	}

	// Verify receipt1 verifies cleanly
	v1, err := svc.VerifyReceipt(ctx, tenantID, consent1.ReceiptID)
	if err != nil || v1.Status != service.ReceiptHashVerified {
		t.Fatalf("receipt1 should verify, got %+v, err=%v", v1, err)
	}

	// 2. Retire notice version "2026-08" with superseded_by "2026-09"
	retiredNotice, err := svc.RetireNotice(ctx, tenantID, "admin", "NG", "2026-08", "2026-09")
	if err != nil {
		t.Fatalf("RetireNotice failed: %v", err)
	}
	if !retiredNotice.Retired {
		t.Fatal("expected notice to be marked retired")
	}
	if retiredNotice.Active {
		t.Fatal("expected retired notice to be inactive")
	}
	if retiredNotice.SupersededBy != "2026-09" {
		t.Fatalf("expected superseded_by '2026-09', got %s", retiredNotice.SupersededBy)
	}

	// 3. Existing receipt MUST still resolve and verify successfully
	receipt1, err := svc.Receipt(ctx, tenantID, consent1.ReceiptID)
	if err != nil {
		t.Fatal(err)
	}
	vAfterRetire, resolvedNotice, err := svc.ResolveAndVerifyReceipt(ctx, receipt1)
	if err != nil || vAfterRetire.Status != service.ReceiptHashVerified {
		t.Fatalf("existing receipt must still verify after notice retirement, got %+v, err=%v", vAfterRetire, err)
	}
	if resolvedNotice.ContentHash != notice.ContentHash {
		t.Fatalf("resolved notice hash %s does not match original %s", resolvedNotice.ContentHash, notice.ContentHash)
	}

	// 4. Attempting to record NEW consent under retired notice "2026-08" MUST be rejected
	_, err = svc.RecordGoverned(ctx, tenantID, "admin", "sub-retire-2", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err == nil {
		t.Fatal("expected error when attempting to grant consent under a retired notice version")
	}
	if !errors.Is(err, service.ErrValidation) {
		t.Fatalf("expected ErrValidation policy rejection, got: %v", err)
	}

	// 5. Publish superseding notice version "2026-09" and record new consent under it
	v2NoticeContent := "Terms and Privacy Notice 2026-09: Updated policy for NG jurisdiction."
	v2Notice, err := svc.PublishNotice(ctx, tenantID, "admin", domain.PrivacyNotice{
		Version:      "2026-09",
		Jurisdiction: "NG",
		PublishedAt:  time.Now().UTC().Add(-time.Hour),
		Active:       true,
		Content:      v2NoticeContent,
	})
	if err != nil {
		t.Fatalf("PublishNotice v2 failed: %v", err)
	}

	v2Evidence := domain.Evidence{
		Method:       "web_form",
		ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		CollectedAt:  time.Now().UTC(),
	}
	consent2, err := svc.RecordGoverned(ctx, tenantID, "admin", "sub-retire-2", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-09", v2Evidence)
	if err != nil {
		t.Fatalf("grant under new notice version 2026-09 failed: %v", err)
	}
	receipt2, err := svc.Receipt(ctx, tenantID, consent2.ReceiptID)
	if err != nil {
		t.Fatal(err)
	}
	if receipt2.NoticeVersion != "2026-09" || receipt2.NoticeContentHash != v2Notice.ContentHash {
		t.Fatalf("receipt2 does not bind to notice v2: version=%s hash=%s", receipt2.NoticeVersion, receipt2.NoticeContentHash)
	}
	v2ReceiptVerification, _, err := svc.ResolveAndVerifyReceipt(ctx, receipt2)
	if err != nil || v2ReceiptVerification.Status != service.ReceiptHashVerified {
		t.Fatalf("receipt2 must verify, got %+v, err=%v", v2ReceiptVerification, err)
	}
}

// TestAT005_PostgresImmutabilityTriggers verifies PostgreSQL DB triggers prevent direct updates to notice content/hash and receipts.
func TestAT005_PostgresImmutabilityTriggers(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
			t.Fatal("TEST_DATABASE_URL (or DATABASE_URL) is required when NVUTO_REQUIRE_INTEGRATION=true")
		}
		t.Skip("TEST_DATABASE_URL / DATABASE_URL not configured")
	}

	repo, err := store.NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })

	tenantID := "tenant-at005-pg-immutability"
	svc, _, evidence := setupAT005Fixture(t, repo, tenantID)
	ctx := context.Background()

	consent, err := svc.RecordGoverned(ctx, tenantID, "admin", "sub-pg-immutability", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Verify receipt verifies after real DB read
	verification, err := svc.VerifyReceipt(ctx, tenantID, consent.ReceiptID)
	if err != nil || verification.Status != service.ReceiptHashVerified {
		t.Fatalf("stored receipt must verify in DB: %+v", verification)
	}

	// 2. Direct attempt to alter notice content in PostgreSQL should be blocked by trigger
	rawPool := repo.Pool()
	_, err = rawPool.Exec(ctx, `UPDATE consent_privacy_notices SET content = 'ALTERED CONTENT' WHERE tenant_id=$1 AND jurisdiction='NG' AND version='2026-08'`, tenantID)
	if err == nil {
		t.Fatal("expected PostgreSQL trigger trg_notice_immutability to block direct UPDATE on notice content")
	}
	if !strings.Contains(err.Error(), "privacy notices are immutable") {
		t.Fatalf("expected trigger exception 'privacy notices are immutable', got: %v", err)
	}

	// 3. Direct attempt to alter consent receipt row in PostgreSQL should be blocked by trigger
	_, err = rawPool.Exec(ctx, `UPDATE consent_receipts SET consent_status = 'denied' WHERE tenant_id=$1 AND id=$2`, tenantID, consent.ReceiptID)
	if err == nil {
		t.Fatal("expected PostgreSQL trigger trg_receipt_immutability to block direct UPDATE on receipt")
	}
	if !strings.Contains(err.Error(), "consent receipts are strictly immutable") {
		t.Fatalf("expected trigger exception 'consent receipts are strictly immutable', got: %v", err)
	}
}
