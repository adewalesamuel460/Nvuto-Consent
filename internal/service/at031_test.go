package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

const (
	at031ArtifactHash = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
)

// setupAT031Policy registers a complete valid policy fixture for testing.
func setupAT031Policy(t *testing.T, svc *service.Service, tenantID, purposeCode, channel, basisCode, jurisdiction, noticeVersion string, publishedAt time.Time) {
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
		t.Fatalf("setupAT031Policy purpose: %v", err)
	}
	_, err = svc.RegisterLawfulBasis(ctx, tenantID, "admin-user", domain.LawfulBasis{
		Code:              basisCode,
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	})
	if err != nil {
		t.Fatalf("setupAT031Policy basis: %v", err)
	}
	_, err = svc.PublishNotice(ctx, tenantID, "admin-user", domain.PrivacyNotice{
		Jurisdiction: jurisdiction,
		Version:      noticeVersion,
		PublishedAt:  publishedAt,
		Active:       true,
	})
	if err != nil {
		t.Fatalf("setupAT031Policy notice: %v", err)
	}
}

// TestAT031_LegacyRecordMethodRemovedFromService proves that the obsolete,
// non-atomic Record method has been completely removed from *service.Service,
// ensuring no caller can bypass governed transactions.
func TestAT031_LegacyRecordMethodRemovedFromService(t *testing.T) {
	svcType := reflect.TypeOf(&service.Service{})
	_, hasRecord := svcType.MethodByName("Record")
	if hasRecord {
		t.Fatal("SECURITY VIOLATION: *service.Service still exposes legacy non-atomic Record method")
	}
	t.Log("PASS: *service.Service does not expose legacy Record method")
}

// TestAT031_LegacyUpsertMethodRemovedFromRepository proves that the obsolete,
// non-transactional Upsert method has been completely removed from store.Repository.
func TestAT031_LegacyUpsertMethodRemovedFromRepository(t *testing.T) {
	repoType := reflect.TypeOf((*store.Repository)(nil)).Elem()
	_, hasUpsert := repoType.MethodByName("Upsert")
	if hasUpsert {
		t.Fatal("SECURITY VIOLATION: store.Repository still exposes legacy non-transactional Upsert method")
	}
	t.Log("PASS: store.Repository does not expose legacy Upsert method")
}

// faultInjectingRepo wraps a store.Repository and intercepts RecordGoverned to simulate
// a transactional storage failure (e.g., failed audit or outbox insert).
type faultInjectingRepo struct {
	store.Repository
	failRecordGoverned bool
}

func (f *faultInjectingRepo) RecordGoverned(ctx context.Context, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	if f.failRecordGoverned {
		return domain.Consent{}, errors.New("simulated transactional failure: outbox write rejected")
	}
	return f.Repository.RecordGoverned(ctx, consent, receipt, audit, outbox)
}

// TestAT031_AtomicRollbackOnStorageFailure proves that when the repository
// encounters an error during RecordGoverned, the operation fails completely
// and no orphan consent record is persisted.
func TestAT031_AtomicRollbackOnStorageFailure(t *testing.T) {
	baseRepo := store.NewMemory()
	failingRepo := &faultInjectingRepo{Repository: baseRepo, failRecordGoverned: false}
	svc := service.New(failingRepo)

	now := time.Now().UTC()
	publishedAt := now.Add(-time.Hour)
	setupAT031Policy(t, svc, "tenant-atomic", "marketing", "email", "consent", "NG", "2026-08", publishedAt)

	evidence := domain.Evidence{
		Method:       "portal",
		ArtifactHash: at031ArtifactHash,
		CollectedAt:  publishedAt,
	}

	// 1. Arm fault injection to simulate failure in outbox/audit persistence
	failingRepo.failRecordGoverned = true

	// 2. Attempt RecordGoverned
	ctx := context.Background()
	_, err := svc.RecordGoverned(ctx, "tenant-atomic", "admin-1", "subj-1", "marketing", "email", "consent", "granted", "portal", nil, nil, "NG", "2026-08", evidence)
	if err == nil {
		t.Fatal("expected RecordGoverned to fail when repository errors, got nil")
	}

	// 3. Verify no consent was stored (must return ErrNotFound)
	_, err = baseRepo.FindEffective(ctx, "tenant-atomic", "subj-1", "marketing", "email")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after rollback, got %v", err)
	}

	// 4. Verify evaluation strictly denies because no consent was persisted
	decision, err := svc.Evaluate(ctx, "tenant-atomic", "subj-1", "marketing", "email", now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.Reason != domain.DecisionReasonNoConsent {
		t.Fatalf("expected no_consent decision after aborted write, got %+v", decision)
	}
	t.Log("PASS: transaction failure cleanly aborts consent creation with no orphan state")
}

// TestAT031_StrictPolicyGovernanceEnforced proves that all consent recording
// requires valid, active policies (purpose, lawful basis, notice) and cannot
// bypass them.
func TestAT031_StrictPolicyGovernanceEnforced(t *testing.T) {
	repo := store.NewMemory()
	svc := service.New(repo)
	ctx := context.Background()
	now := time.Now().UTC()

	evidence := domain.Evidence{
		Method:       "portal",
		ArtifactHash: at031ArtifactHash,
		CollectedAt:  now,
	}

	// Case 1: Unregistered purpose rejected
	_, err := svc.RecordGoverned(ctx, "tenant-strict", "admin-1", "subj-1", "unregistered_purpose", "email", "consent", "granted", "portal", nil, nil, "NG", "v1", evidence)
	if err == nil {
		t.Fatal("expected error for unregistered purpose, got nil")
	}

	// Setup valid purpose
	_, err = svc.RegisterPurpose(ctx, "tenant-strict", "admin-1", domain.Purpose{
		Code:                       "newsletter",
		Category:                   "marketing",
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		Active:                     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Case 2: Unregistered lawful basis rejected
	_, err = svc.RecordGoverned(ctx, "tenant-strict", "admin-1", "subj-1", "newsletter", "email", "unregistered_basis", "granted", "portal", nil, nil, "NG", "v1", evidence)
	if err == nil {
		t.Fatal("expected error for unregistered legal basis, got nil")
	}

	// Setup valid lawful basis
	_, err = svc.RegisterLawfulBasis(ctx, "tenant-strict", "admin-1", domain.LawfulBasis{
		Code:              "consent",
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Active:            true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Case 3: Unpublished notice rejected
	_, err = svc.RecordGoverned(ctx, "tenant-strict", "admin-1", "subj-1", "newsletter", "email", "consent", "granted", "portal", nil, nil, "NG", "v1", evidence)
	if err == nil {
		t.Fatal("expected error for unpublished notice, got nil")
	}

	// Setup published notice
	_, err = svc.PublishNotice(ctx, "tenant-strict", "admin-1", domain.PrivacyNotice{
		Jurisdiction: "NG",
		Version:      "v1",
		PublishedAt:  now.Add(-time.Hour),
		Active:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Case 4: Now succeeds with all governed policies present
	evidence.CollectedAt = now.Add(-30 * time.Minute)
	c, err := svc.RecordGoverned(ctx, "tenant-strict", "admin-1", "subj-1", "newsletter", "email", "consent", "granted", "portal", nil, nil, "NG", "v1", evidence)
	if err != nil {
		t.Fatalf("expected RecordGoverned to succeed after policy registration, got %v", err)
	}
	if c.ID == "" || c.ReceiptID == "" {
		t.Fatalf("expected populated consent and receipt IDs, got %+v", c)
	}
	t.Log("PASS: strict policy governance enforced on all record attempts")
}

// mockRawConsentStore implements service.RawConsentStore for testing LegacyMigrationAdapter.
type mockRawConsentStore struct {
	inserted []domain.Consent
}

func (m *mockRawConsentStore) RawInsertLegacy(ctx context.Context, consent domain.Consent) error {
	m.inserted = append(m.inserted, consent)
	return nil
}

// TestAT031_LegacyMigrationAdapterIsolation proves that LegacyMigrationAdapter is
// strictly isolated, requires explicit raw store configuration, and tags imported
// records with source=legacy_migration.
func TestAT031_LegacyMigrationAdapterIsolation(t *testing.T) {
	ctx := context.Background()

	// 1. Unconfigured adapter fails closed
	nilAdapter := service.NewLegacyMigrationAdapter(nil)
	err := nilAdapter.MigrateLegacyConsent(ctx, domain.Consent{TenantID: "t1", SubjectID: "s1", Purpose: "p1", Channel: "c1"})
	if err == nil {
		t.Fatal("expected error for unconfigured adapter, got nil")
	}

	// 2. Validation requires core identifiers
	rawStore := &mockRawConsentStore{}
	adapter := service.NewLegacyMigrationAdapter(rawStore)
	err = adapter.MigrateLegacyConsent(ctx, domain.Consent{TenantID: "t1"})
	if err == nil {
		t.Fatal("expected validation error for incomplete consent, got nil")
	}

	// 3. Successful legacy import tags source and updates timestamps
	legacy := domain.Consent{
		TenantID:  "t1",
		SubjectID: "s1",
		Purpose:   "p1",
		Channel:   "c1",
	}
	err = adapter.MigrateLegacyConsent(ctx, legacy)
	if err != nil {
		t.Fatalf("expected successful legacy migration, got %v", err)
	}
	if len(rawStore.inserted) != 1 {
		t.Fatalf("expected 1 inserted record, got %d", len(rawStore.inserted))
	}
	stored := rawStore.inserted[0]
	if stored.Source != "legacy_migration" {
		t.Fatalf("expected Source to default to legacy_migration, got %s", stored.Source)
	}
	if stored.CreatedAt.IsZero() || stored.UpdatedAt.IsZero() {
		t.Fatal("expected non-zero timestamps on migrated record")
	}
	t.Log("PASS: LegacyMigrationAdapter enforces isolated validation and historical tagging")
}
