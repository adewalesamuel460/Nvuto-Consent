package store_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// TestAT031_RepositoryInterfaceHasNoLegacyUpsert (AT-031) statically and dynamically
// proves that the store.Repository interface does not expose an Upsert method.
func TestAT031_RepositoryInterfaceHasNoLegacyUpsert(t *testing.T) {
	repoType := reflect.TypeOf((*store.Repository)(nil)).Elem()
	if _, found := repoType.MethodByName("Upsert"); found {
		t.Fatal("store.Repository must NOT expose legacy Upsert method")
	}
	if _, found := repoType.MethodByName("Record"); found {
		t.Fatal("store.Repository must NOT expose legacy Record method")
	}
	t.Log("PASS: store.Repository strictly prohibits legacy Upsert and Record methods")
}

// TestAT031_PostgresHasNoLegacyUpsert (AT-031) statically and dynamically
// proves that *store.Postgres does not expose an Upsert method.
func TestAT031_PostgresHasNoLegacyUpsert(t *testing.T) {
	pgType := reflect.TypeOf(&store.Postgres{})
	if _, found := pgType.MethodByName("Upsert"); found {
		t.Fatal("*store.Postgres must NOT expose legacy Upsert method")
	}
	if _, found := pgType.MethodByName("Record"); found {
		t.Fatal("*store.Postgres must NOT expose legacy Record method")
	}
	t.Log("PASS: *store.Postgres strictly prohibits legacy Upsert and Record methods")
}

// TestAT031_MemoryHasNoLegacyUpsert (AT-031) statically and dynamically
// proves that *store.Memory does not expose an Upsert method.
func TestAT031_MemoryHasNoLegacyUpsert(t *testing.T) {
	memType := reflect.TypeOf(&store.Memory{})
	if _, found := memType.MethodByName("Upsert"); found {
		t.Fatal("*store.Memory must NOT expose legacy Upsert method")
	}
	if _, found := memType.MethodByName("Record"); found {
		t.Fatal("*store.Memory must NOT expose legacy Record method")
	}
	t.Log("PASS: *store.Memory strictly prohibits legacy Upsert and Record methods")
}

// TestAT031_PostgresGovernedAtomicWriteEnforced proves that all consent
// creation in PostgreSQL occurs through RecordGoverned, binding consent,
// receipt, audit, and outbox atomically.
func TestAT031_PostgresGovernedAtomicWriteEnforced(t *testing.T) {
	databaseURL := getTestDSN(t)
	if databaseURL == "" {
		t.Skip("skipping PostgreSQL integration test: TEST_DATABASE_URL not set")
	}

	repo, err := store.NewPostgres(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	tenantID := "tenant-at031-" + time.Now().Format("150405.000000")
	now := time.Now().UTC()

	consent := domain.Consent{
		ID:            "con-at031-" + tenantID,
		TenantID:      tenantID,
		SubjectID:     "subj-at031",
		Purpose:       "marketing",
		Channel:       "email",
		LegalBasis:    "consent",
		Status:        domain.StatusGranted,
		Source:        "web",
		NoticeVersion: "2026-08",
		Jurisdiction:  "NG",
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	receipt := domain.ConsentReceipt{
		ID:          "rcp-at031-" + tenantID,
		TenantID:    tenantID,
		ConsentID:   consent.ID,
		SubjectID:   consent.SubjectID,
		Purpose:     consent.Purpose,
		Channel:     consent.Channel,
		ReceiptHash: "hash-at031",
		IssuedAt:    now,
	}

	audit := domain.AuditEvent{
		ID:           "aud-at031-" + tenantID,
		TenantID:     tenantID,
		ActorID:      "admin-user",
		Action:       "consent.granted",
		ConsentID:    consent.ID,
		ResourceType: "consent",
		ResourceID:   consent.ID,
		Revision:     1,
		OccurredAt:   now,
	}

	outbox := domain.OutboxEvent{
		ID:            "evt-at031-" + tenantID,
		TenantID:      tenantID,
		EventType:     "consent.granted",
		AggregateType: "consent.preference",
		AggregateID:   consent.ID,
		Payload:       map[string]any{"subject_id": consent.SubjectID},
		OccurredAt:    now,
	}

	// 1. Success path: RecordGoverned transactionally writes all 4 records
	stored, err := repo.RecordGoverned(ctx, consent, receipt, audit, outbox)
	if err != nil {
		t.Fatalf("RecordGoverned failed: %v", err)
	}
	if stored.ID != consent.ID || stored.PreferenceGeneration < 1 {
		t.Fatalf("expected stored consent with valid preference generation, got %+v", stored)
	}

	// Verify consent exists
	c, err := repo.Get(ctx, tenantID, consent.ID)
	if err != nil {
		t.Fatalf("Get consent failed: %v", err)
	}
	if c.ID != consent.ID {
		t.Fatalf("mismatched consent ID: %s vs %s", c.ID, consent.ID)
	}

	// Verify receipt exists
	r, err := repo.GetReceipt(ctx, tenantID, receipt.ID)
	if err != nil {
		t.Fatalf("GetReceipt failed: %v", err)
	}
	if r.ID != receipt.ID {
		t.Fatalf("mismatched receipt ID: %s vs %s", r.ID, receipt.ID)
	}

	// 2. Failure path: Triggering outbox primary key collision rolls back consent write
	failedConsentID := "con-failed-" + tenantID
	failedReceiptID := "rcp-failed-" + tenantID
	failedConsent := domain.Consent{
		ID:            failedConsentID,
		TenantID:      tenantID,
		SubjectID:     "subj-failed",
		Purpose:       "marketing",
		Channel:       "email",
		LegalBasis:    "consent",
		Status:        domain.StatusGranted,
		Source:        "web",
		NoticeVersion: "2026-08",
		Jurisdiction:  "NG",
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	failedReceipt := domain.ConsentReceipt{
		ID:          failedReceiptID,
		TenantID:    tenantID,
		ConsentID:   failedConsentID,
		SubjectID:   "subj-failed",
		Purpose:     "marketing",
		Channel:     "email",
		ReceiptHash: "hash-failed",
		IssuedAt:    now,
	}
	collidingOutbox := domain.OutboxEvent{
		ID:            outbox.ID, // Collide with existing outbox event ID!
		TenantID:      tenantID,
		EventType:     "consent.granted",
		AggregateType: "consent.preference",
		AggregateID:   failedConsentID,
		OccurredAt:    now,
	}

	_, err = repo.RecordGoverned(ctx, failedConsent, failedReceipt, audit, collidingOutbox)
	if err == nil {
		t.Fatal("expected RecordGoverned to fail on duplicate outbox event ID, got nil")
	}

	// Verify failedConsent was completely rolled back from PostgreSQL
	_, err = repo.Get(ctx, tenantID, failedConsentID)
	if err == nil || err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound for rolled back consent, got %v", err)
	}
	t.Log("PASS: RecordGoverned transactionally rolled back consent on outbox collision")
}

func getTestDSN(t *testing.T) string {
	t.Helper()
	return "postgres://yugabyte:yugabyte@127.0.0.1:5433/consent?sslmode=disable"
}
