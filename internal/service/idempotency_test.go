package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

func TestGovernedConsentIdempotencyReturnsOriginalAndRejectsSemanticReuse(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	installPolicyFixture(t, svc, "tenant-a")
	evidence := domain.Evidence{Method: "preference_center", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()}

	first, err := svc.RecordGovernedIdempotent(context.Background(), "customer360:consent-1", "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "customer360", map[string]string{"source_record_id": "consent-1"}, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.RecordGovernedIdempotent(context.Background(), "customer360:consent-1", "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "customer360", map[string]string{"source_record_id": "consent-1"}, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.ReceiptID != second.ReceiptID {
		t.Fatalf("exact replay must return original consent/receipt: first=%+v second=%+v", first, second)
	}

	_, err = svc.RecordGovernedIdempotent(context.Background(), "customer360:consent-1", "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "denied", "customer360", map[string]string{"source_record_id": "consent-1"}, nil, "NG", "2026-08", evidence)
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("semantic idempotency-key reuse must conflict, got %v", err)
	}
}

func TestGovernedConsentIdempotencyValidatesKey(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	installPolicyFixture(t, svc, "tenant-a")
	evidence := domain.Evidence{Method: "preference_center", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()}
	if _, err := svc.RecordGovernedIdempotent(context.Background(), "", "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "customer360", nil, nil, "NG", "2026-08", evidence); !errors.Is(err, ErrValidation) {
		t.Fatalf("empty key must fail validation, got %v", err)
	}
}

func TestGovernedConsentIdempotencyReplaySnapshotAfterWithdrawalAndPolicyUpdate(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	installPolicyFixture(t, svc, "tenant-a")
	evidence := domain.Evidence{Method: "preference_center", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()}

	// 1. Initial 201 creation
	first, err := svc.RecordGovernedIdempotent(context.Background(), "customer360:consent-snap", "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "customer360", map[string]string{"source_record_id": "snap-1"}, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("initial creation failed: %v", err)
	}
	if first.Status != domain.StatusGranted {
		t.Fatalf("expected status granted, got %v", first.Status)
	}

	// 2. Consent subsequently withdrawn
	withdrawn, err := svc.Withdraw(context.Background(), "tenant-a", "subject-1", first.ID)
	if err != nil {
		t.Fatalf("withdraw failed: %v", err)
	}
	if withdrawn.Status != domain.StatusWithdrawn {
		t.Fatalf("expected withdrawn status, got %v", withdrawn.Status)
	}

	// 3. Replay returns the exact original 201 response snapshot, NOT the withdrawn state
	replayed, err := svc.RecordGovernedIdempotent(context.Background(), "customer360:consent-snap", "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "customer360", map[string]string{"source_record_id": "snap-1"}, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if replayed.Status != domain.StatusGranted {
		t.Fatalf("replay must return original granted status snapshot, got %v", replayed.Status)
	}
	if replayed.ID != first.ID || replayed.ReceiptID != first.ReceiptID {
		t.Fatalf("replay snapshot mismatch: want ID %s, receipt %s; got ID %s, receipt %s", first.ID, first.ReceiptID, replayed.ID, replayed.ReceiptID)
	}

	// 4. Policy is updated to deactivate the purpose
	if _, err := svc.RegisterPurpose(context.Background(), "tenant-a", "admin", domain.Purpose{
		Code: "marketing", Category: "marketing", AllowedChannels: []string{"whatsapp"}, Active: false,
	}); err != nil {
		t.Fatalf("policy update failed: %v", err)
	}

	// 5. Replay STILL succeeds and returns the original snapshot without error
	replayedAfterPolicyUpdate, err := svc.RecordGovernedIdempotent(context.Background(), "customer360:consent-snap", "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "customer360", map[string]string{"source_record_id": "snap-1"}, nil, "NG", "2026-08", evidence)
	if err != nil {
		t.Fatalf("replay after policy change failed: %v", err)
	}
	if replayedAfterPolicyUpdate.Status != domain.StatusGranted {
		t.Fatalf("expected granted status on replay after policy update, got %v", replayedAfterPolicyUpdate.Status)
	}
}

func TestGovernedConsentIdempotencySubjectAuthorizationReevaluated(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	installPolicyFixture(t, svc, "tenant-a")
	evidence := domain.Evidence{Method: "preference_center", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()}

	// Initial creation by authorized customer cus-1
	_, err := svc.RecordGovernedIdempotent(context.Background(), "customer360:consent-auth", "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "customer360", nil, nil, "NG", "2026-08", evidence, OwnSubject("cus-1"))
	if err != nil {
		t.Fatalf("creation failed: %v", err)
	}

	// Replay by caller restricted to cus-2 must be rejected with ErrSubjectForbidden
	_, err = svc.RecordGovernedIdempotent(context.Background(), "customer360:consent-auth", "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "customer360", nil, nil, "NG", "2026-08", evidence, OwnSubject("cus-2"))
	if !errors.Is(err, ErrSubjectForbidden) {
		t.Fatalf("unauthorized customer replay must return ErrSubjectForbidden, got %v", err)
	}

	// Replay by caller restricted to cus-1 succeeds
	replaySelf, err := svc.RecordGovernedIdempotent(context.Background(), "customer360:consent-auth", "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "customer360", nil, nil, "NG", "2026-08", evidence, OwnSubject("cus-1"))
	if err != nil {
		t.Fatalf("authorized customer replay failed: %v", err)
	}
	if replaySelf.SubjectID != "cus-1" {
		t.Fatalf("expected subject cus-1, got %v", replaySelf.SubjectID)
	}

	// Replay by tenant-wide caller succeeds
	replayAdmin, err := svc.RecordGovernedIdempotent(context.Background(), "customer360:consent-auth", "tenant-a", "subject-1", "cus-1", "marketing", "email", "consent", "granted", "customer360", nil, nil, "NG", "2026-08", evidence, TenantWide())
	if err != nil {
		t.Fatalf("tenant-wide replay failed: %v", err)
	}
	if replayAdmin.SubjectID != "cus-1" {
		t.Fatalf("expected subject cus-1, got %v", replayAdmin.SubjectID)
	}
}

func TestGovernedConsentIdempotencyTokenRotationAndWallClockTolerance(t *testing.T) {
	repo := store.NewMemory()
	svc := New(repo)
	installPolicyFixture(t, svc, "tenant-a")
	evidence1 := domain.Evidence{Method: "preference_center", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()}

	// 1. Initial creation by actor-1
	first, err := svc.RecordGovernedIdempotent(context.Background(), "key-rotation-1", "tenant-a", "actor-1", "cus-1", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence1)
	if err != nil {
		t.Fatalf("first creation failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// 2. Replay with different actor (token rotation) and later wall-clock time
	evidence2 := domain.Evidence{Method: "preference_center", ArtifactHash: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", CollectedAt: time.Now().UTC()}
	second, err := svc.RecordGovernedIdempotent(context.Background(), "key-rotation-1", "tenant-a", "actor-rotated-2", "cus-1", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence2)
	if err != nil {
		t.Fatalf("replay with rotated token/wall-clock failed: %v", err)
	}

	if first.ID != second.ID || first.ReceiptID != second.ReceiptID {
		t.Fatalf("replay must return identical original snapshot: first=%+v second=%+v", first, second)
	}
}
