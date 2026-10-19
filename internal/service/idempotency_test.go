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
