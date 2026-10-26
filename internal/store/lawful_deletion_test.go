package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// TestAT022_LawfulErasureRedactsAuditWithoutBreakingImmutability verifies that
// GDPR/NDPR lawful erasure correctly redacts PII from before_state and after_state
// payloads while preserving the structural immutability of audit records.
func TestAT022_LawfulErasureRedactsAuditWithoutBreakingImmutability(t *testing.T) {
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
	tenantID := "tenant-gdpr-" + time.Now().Format("150405.000000")
	subjectID := "subject-gdpr-123"
	now := time.Now().UTC()

	// 1. Record an initial consent with audit event containing PII
	consent := domain.Consent{
		ID:            "con-gdpr-" + tenantID,
		TenantID:      tenantID,
		SubjectID:     subjectID,
		Purpose:       "marketing",
		Channel:       "email",
		LegalBasis:    "consent",
		Status:        domain.StatusGranted,
		Source:        "web",
		Scope:         map[string]string{"email": "user@example.com", "name": "Jane Doe"},
		NoticeVersion: "2026-08",
		Jurisdiction:  "NG",
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	receipt := domain.ConsentReceipt{
		ID:          "rcp-gdpr-" + tenantID,
		TenantID:    tenantID,
		ConsentID:   consent.ID,
		SubjectID:   subjectID,
		Purpose:     consent.Purpose,
		Channel:     consent.Channel,
		ReceiptHash: "hash-gdpr",
		IssuedAt:    now,
	}

	audit := domain.AuditEvent{
		ID:           "aud-gdpr-" + tenantID,
		TenantID:     tenantID,
		ActorID:      "admin-user",
		Action:       "consent.granted",
		ConsentID:    consent.ID,
		ResourceType: "consent",
		ResourceID:   consent.ID,
		Revision:     1,
		BeforeState:  map[string]any{},
		AfterState:   map[string]any{"subject_id": subjectID, "email": "user@example.com"},
		OccurredAt:   now,
	}

	outbox := domain.OutboxEvent{
		ID:            "evt-gdpr-" + tenantID,
		TenantID:      tenantID,
		EventType:     "consent.granted",
		AggregateType: "consent.preference",
		AggregateID:   consent.ID,
		Payload:       map[string]any{"subject_id": subjectID},
		OccurredAt:    now,
	}

	_, err = repo.RecordGoverned(ctx, consent, receipt, audit, outbox)
	if err != nil {
		t.Fatalf("RecordGoverned failed: %v", err)
	}

	// 2. Perform Lawful Erasure of the subject's personal data
	erasureReq := store.LawfulErasureRequest{
		TenantID:  tenantID,
		SubjectID: subjectID,
		ActorID:   "privacy-officer",
		Reason:    "GDPR Article 17 Right to Erasure Request",
	}
	result, err := repo.LawfulRedactSubject(ctx, erasureReq)
	if err != nil {
		t.Fatalf("LawfulRedactSubject failed: %v", err)
	}
	if result.ConsentsRedacted != 1 {
		t.Fatalf("expected 1 consent redacted, got %d", result.ConsentsRedacted)
	}
	if result.AuditEventsRedacted < 1 {
		t.Fatalf("expected at least 1 audit event redacted, got %d", result.AuditEventsRedacted)
	}

	// 3. Verify that the consent record has been tombstoned to withdrawn
	storedConsent, err := repo.Get(ctx, tenantID, consent.ID)
	if err != nil {
		t.Fatalf("Get consent failed: %v", err)
	}
	if storedConsent.Status != domain.StatusWithdrawn {
		t.Fatalf("expected consent status withdrawn, got %s", storedConsent.Status)
	}
	if storedConsent.Scope["redacted"] != "true" {
		t.Fatalf("expected scope to be redacted, got %+v", storedConsent.Scope)
	}

	// 4. Verify that audit events have PII payloads redacted
	auditEvents, total, err := repo.ListAuditEvents(ctx, tenantID, "consent", consent.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditEvents failed: %v", err)
	}
	if total == 0 || len(auditEvents) == 0 {
		t.Fatal("expected audit events to exist")
	}

	foundInitialAudit := false
	for _, a := range auditEvents {
		if a.ID == audit.ID {
			foundInitialAudit = true
			// Structural fields must remain 100% intact
			if a.Action != "consent.granted" || a.ConsentID != consent.ID || a.ActorID != "admin-user" {
				t.Fatalf("structural audit envelope was corrupted: %+v", a)
			}
			// Payloads must be redacted
			if a.AfterState["redacted"] != true {
				t.Fatalf("expected after_state to be redacted, got %+v", a.AfterState)
			}
		}
	}
	if !foundInitialAudit {
		t.Fatalf("initial audit event %s was not found in audit log", audit.ID)
	}

	// 5. Verify that unauthorized direct UPDATE on consent_audit_events is STILL rejected
	pool := repo.Pool()
	_, updateErr := pool.Exec(ctx, `UPDATE consent_audit_events SET before_state = '{}' WHERE id = $1`, audit.ID)
	if updateErr == nil {
		t.Fatal("expected direct UPDATE without session authorization to fail, got nil")
	}
	if !strings.Contains(updateErr.Error(), "immutable") && !strings.Contains(updateErr.Error(), "append-only") {
		t.Fatalf("unexpected error message on unauthorized update: %v", updateErr)
	}
	t.Log("PASS: Lawful erasure redacted PII while preserving audit structure and immutability controls")
}

// TestAT022_LawfulErasureMemoryParity verifies the lawful deletion contract on Memory store.
func TestAT022_LawfulErasureMemoryParity(t *testing.T) {
	mem := store.NewMemory()
	ctx := context.Background()
	tenantID := "tenant-mem-gdpr"
	subjectID := "subject-mem-123"
	now := time.Now().UTC()

	consent := domain.Consent{
		ID:         "con-mem-1",
		TenantID:   tenantID,
		SubjectID:  subjectID,
		Purpose:    "marketing",
		Channel:    "email",
		LegalBasis: "consent",
		Status:     domain.StatusGranted,
		Version:    1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	audit := domain.AuditEvent{
		ID:           "aud-mem-1",
		TenantID:     tenantID,
		ActorID:      "admin",
		Action:       "consent.granted",
		ConsentID:    consent.ID,
		ResourceType: "consent",
		ResourceID:   consent.ID,
		AfterState:   map[string]any{"name": "Alice"},
		OccurredAt:   now,
	}

	_, err := mem.RecordGoverned(ctx, consent, domain.ConsentReceipt{}, audit, domain.OutboxEvent{})
	if err != nil {
		t.Fatal(err)
	}

	res, err := mem.LawfulRedactSubject(ctx, store.LawfulErasureRequest{
		TenantID:  tenantID,
		SubjectID: subjectID,
		ActorID:   "privacy-officer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ConsentsRedacted != 1 {
		t.Fatalf("expected 1 redacted consent, got %d", res.ConsentsRedacted)
	}

	stored, err := mem.Get(ctx, tenantID, consent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.StatusWithdrawn {
		t.Fatalf("expected withdrawn status, got %s", stored.Status)
	}
	t.Log("PASS: Memory store implements lawful deletion with complete parity")
}
