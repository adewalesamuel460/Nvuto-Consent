package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

func TestMemoryCompositeKeys_DelimiterCollisionIsolation(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemory()

	// Tenant A is "tenant:a" with resource ID "1".
	// Tenant B is "tenant" with resource ID "a:1".
	// If keys were naively concatenated with ":", both would result in "tenant:a:1".
	tenantA := "tenant:a"
	tenantB := "tenant"
	now := time.Now().UTC()

	// 1. Consent isolation
	cA := domain.Consent{
		ID:        "1",
		TenantID:  tenantA,
		SubjectID: "sub-1",
		Purpose:   "marketing",
		Channel:   "email",
		Status:    domain.StatusGranted,
		CreatedAt: now,
		UpdatedAt: now,
	}
	cB := domain.Consent{
		ID:        "a:1",
		TenantID:  tenantB,
		SubjectID: "sub-2",
		Purpose:   "analytics",
		Channel:   "sms",
		Status:    domain.StatusGranted,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if _, err := m.RecordGoverned(ctx, cA, domain.ConsentReceipt{ID: "rcp-1", TenantID: tenantA, ConsentID: "1"}, domain.AuditEvent{ID: "aud-1", TenantID: tenantA}, domain.OutboxEvent{ID: "evt-1", TenantID: tenantA}); err != nil {
		t.Fatalf("failed to record consent A: %v", err)
	}
	if _, err := m.RecordGoverned(ctx, cB, domain.ConsentReceipt{ID: "rcp-a:1", TenantID: tenantB, ConsentID: "a:1"}, domain.AuditEvent{ID: "aud-2", TenantID: tenantB}, domain.OutboxEvent{ID: "evt-2", TenantID: tenantB}); err != nil {
		t.Fatalf("failed to record consent B: %v", err)
	}

	gotA, err := m.Get(ctx, tenantA, "1")
	if err != nil || gotA.SubjectID != "sub-1" {
		t.Fatalf("unexpected consent for tenant A: %v, err: %v", gotA, err)
	}
	gotB, err := m.Get(ctx, tenantB, "a:1")
	if err != nil || gotB.SubjectID != "sub-2" {
		t.Fatalf("unexpected consent for tenant B: %v, err: %v", gotB, err)
	}

	// Cross-lookup must return ErrNotFound
	if _, err := m.Get(ctx, tenantA, "a:1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-tenant A lookup, got %v", err)
	}
	if _, err := m.Get(ctx, tenantB, "1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-tenant B lookup, got %v", err)
	}

	// 2. Receipt isolation
	rcpA, err := m.GetReceipt(ctx, tenantA, "rcp-1")
	if err != nil || rcpA.ConsentID != "1" {
		t.Fatalf("unexpected receipt for tenant A: %v, err: %v", rcpA, err)
	}
	if _, err := m.GetReceipt(ctx, tenantA, "rcp-a:1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-tenant receipt lookup, got %v", err)
	}
	if _, err := m.GetReceipt(ctx, tenantB, "rcp-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-tenant receipt lookup, got %v", err)
	}

	// 3. Purpose isolation
	pA := domain.Purpose{TenantID: tenantA, Code: "p1", Category: "marketing", Active: true}
	pB := domain.Purpose{TenantID: tenantB, Code: "a:p1", Category: "security", Active: true}
	if _, err := m.PutPurpose(ctx, pA, domain.AuditEvent{ID: "aud-p1", TenantID: tenantA}, domain.OutboxEvent{ID: "evt-p1", TenantID: tenantA}); err != nil {
		t.Fatalf("failed to put purpose A: %v", err)
	}
	if _, err := m.PutPurpose(ctx, pB, domain.AuditEvent{ID: "aud-p2", TenantID: tenantB}, domain.OutboxEvent{ID: "evt-p2", TenantID: tenantB}); err != nil {
		t.Fatalf("failed to put purpose B: %v", err)
	}

	gotPA, err := m.GetPurpose(ctx, tenantA, "p1")
	if err != nil || gotPA.Category != "marketing" {
		t.Fatalf("unexpected purpose A: %v, err: %v", gotPA, err)
	}
	if _, err := m.GetPurpose(ctx, tenantA, "a:p1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-tenant purpose lookup, got %v", err)
	}
	if _, err := m.GetPurpose(ctx, tenantB, "p1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-tenant purpose lookup, got %v", err)
	}

	// 4. Lawful basis isolation
	bA := domain.LawfulBasis{TenantID: tenantA, Code: "b1", Active: true}
	bB := domain.LawfulBasis{TenantID: tenantB, Code: "a:b1", Active: true}
	if _, err := m.PutLawfulBasis(ctx, bA, domain.AuditEvent{ID: "aud-b1", TenantID: tenantA}, domain.OutboxEvent{ID: "evt-b1", TenantID: tenantA}); err != nil {
		t.Fatalf("failed to put lawful basis A: %v", err)
	}
	if _, err := m.PutLawfulBasis(ctx, bB, domain.AuditEvent{ID: "aud-b2", TenantID: tenantB}, domain.OutboxEvent{ID: "evt-b2", TenantID: tenantB}); err != nil {
		t.Fatalf("failed to put lawful basis B: %v", err)
	}
	gotBA, err := m.GetLawfulBasis(ctx, tenantA, "b1")
	if err != nil || gotBA.Code != "b1" {
		t.Fatalf("unexpected lawful basis A: %v, err: %v", gotBA, err)
	}
	if _, err := m.GetLawfulBasis(ctx, tenantA, "a:b1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-tenant basis lookup, got %v", err)
	}

	// 5. Privacy Notice isolation
	nA := domain.PrivacyNotice{TenantID: tenantA, Jurisdiction: "US:CA", Version: "v1", PublishedAt: now, Active: true}
	nB := domain.PrivacyNotice{TenantID: tenantB, Jurisdiction: "a:US:CA", Version: "v1", PublishedAt: now, Active: true}
	if _, err := m.PutNotice(ctx, nA, domain.AuditEvent{ID: "aud-n1", TenantID: tenantA}, domain.OutboxEvent{ID: "evt-n1", TenantID: tenantA}); err != nil {
		t.Fatalf("failed to put notice A: %v", err)
	}
	if _, err := m.PutNotice(ctx, nB, domain.AuditEvent{ID: "aud-n2", TenantID: tenantB}, domain.OutboxEvent{ID: "evt-n2", TenantID: tenantB}); err != nil {
		t.Fatalf("failed to put notice B: %v", err)
	}
	gotNA, err := m.GetNotice(ctx, tenantA, "US:CA", "v1")
	if err != nil || gotNA.Jurisdiction != "US:CA" {
		t.Fatalf("unexpected notice A: %v, err: %v", gotNA, err)
	}
	if _, err := m.GetNotice(ctx, tenantA, "a:US:CA", "v1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-tenant notice lookup, got %v", err)
	}

	// 6. Suppression isolation
	sA := domain.Suppression{ID: "sup-1", TenantID: tenantA, SubjectID: "sub:1", Scope: domain.SuppressionChannel, Channel: "email", Active: true, Reason: "opt-out"}
	sB := domain.Suppression{ID: "sup-2", TenantID: tenantB, SubjectID: "a:sub:1", Scope: domain.SuppressionChannel, Channel: "email", Active: true, Reason: "opt-out"}
	if _, err := m.SetSuppression(ctx, sA, domain.AuditEvent{ID: "aud-s1", TenantID: tenantA}, domain.OutboxEvent{ID: "evt-s1", TenantID: tenantA}); err != nil {
		t.Fatalf("failed to set suppression A: %v", err)
	}
	if _, err := m.SetSuppression(ctx, sB, domain.AuditEvent{ID: "aud-s2", TenantID: tenantB}, domain.OutboxEvent{ID: "evt-s2", TenantID: tenantB}); err != nil {
		t.Fatalf("failed to set suppression B: %v", err)
	}

	gotSA, err := m.FindSuppression(ctx, tenantA, "sub:1", "", "email")
	if err != nil || gotSA.ID != "sup-1" {
		t.Fatalf("unexpected suppression A: %v, err: %v", gotSA, err)
	}
	if _, err := m.FindSuppression(ctx, tenantA, "a:sub:1", "", "email"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-tenant suppression lookup, got %v", err)
	}
	if _, err := m.FindSuppression(ctx, tenantB, "sub:1", "", "email"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-tenant suppression lookup, got %v", err)
	}

	// 7. Idempotency ledger isolation
	cIdempA := domain.Consent{ID: "c-idemp-1", TenantID: tenantA, SubjectID: "sub-1", Purpose: "p", Channel: "email", Status: domain.StatusGranted, CreatedAt: now, UpdatedAt: now}
	cIdempB := domain.Consent{ID: "c-idemp-2", TenantID: tenantB, SubjectID: "sub-2", Purpose: "p", Channel: "email", Status: domain.StatusGranted, CreatedAt: now, UpdatedAt: now}
	if _, err := m.RecordGovernedIdempotent(ctx, "idemp:1", "hash-a", cIdempA, domain.ConsentReceipt{}, domain.AuditEvent{ID: "aud-i1", TenantID: tenantA}, domain.OutboxEvent{ID: "evt-i1", TenantID: tenantA}); err != nil {
		t.Fatalf("failed to record idempotent A: %v", err)
	}
	if _, err := m.RecordGovernedIdempotent(ctx, "a:idemp:1", "hash-b", cIdempB, domain.ConsentReceipt{}, domain.AuditEvent{ID: "aud-i2", TenantID: tenantB}, domain.OutboxEvent{ID: "evt-i2", TenantID: tenantB}); err != nil {
		t.Fatalf("failed to record idempotent B: %v", err)
	}

	snapA, hashA, okA, err := m.FindIdempotencySnapshot(ctx, tenantA, "idemp:1")
	if err != nil || !okA || hashA != "hash-a" || snapA.ID != "c-idemp-1" {
		t.Fatalf("unexpected snapshot A: ok=%v, hash=%s, snap=%v, err=%v", okA, hashA, snapA, err)
	}
	_, _, okCrossA, _ := m.FindIdempotencySnapshot(ctx, tenantA, "a:idemp:1")
	if okCrossA {
		t.Fatalf("expected cross-tenant snapshot lookup in A to return false")
	}
	_, _, okCrossB, _ := m.FindIdempotencySnapshot(ctx, tenantB, "idemp:1")
	if okCrossB {
		t.Fatalf("expected cross-tenant snapshot lookup in B to return false")
	}
}

func TestMemorySetSuppression_PreservesIDAndUpdatesMetadata(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemory()

	tenantID := "tenant-test"
	subjectID := "user-42"
	originalID := "sup-original-uuid"
	newAttemptedID := "sup-newly-generated-uuid"

	// 1. Initial SetSuppression creates a suppression with version 1
	initial := domain.Suppression{
		ID:            originalID,
		TenantID:      tenantID,
		SubjectID:     subjectID,
		Scope:         domain.SuppressionChannel,
		Channel:       "email",
		Reason:        "user_request",
		Active:        true,
		CreatedByRole: "privacy_admin",
		UpdatedAt:     time.Now().UTC(),
	}
	audit1 := domain.AuditEvent{ID: "aud-1", TenantID: tenantID, Action: "consent.suppression.changed", ConsentID: originalID}
	outbox1 := domain.OutboxEvent{ID: "evt-1", TenantID: tenantID, EventType: "consent.suppression.changed", AggregateID: originalID, Payload: map[string]any{"created_by_role": "privacy_admin"}}

	s1, err := m.SetSuppression(ctx, initial, audit1, outbox1)
	if err != nil {
		t.Fatalf("initial SetSuppression failed: %v", err)
	}
	if s1.ID != originalID {
		t.Fatalf("expected ID %s, got %s", originalID, s1.ID)
	}
	if s1.Version != 1 {
		t.Fatalf("expected version 1, got %d", s1.Version)
	}
	if s1.CreatedByRole != "privacy_admin" {
		t.Fatalf("expected CreatedByRole privacy_admin, got %s", s1.CreatedByRole)
	}

	// 2. Second SetSuppression attempts to overwrite with a different ID and role
	update := domain.Suppression{
		ID:            newAttemptedID,
		TenantID:      tenantID,
		SubjectID:     subjectID,
		Scope:         domain.SuppressionChannel,
		Channel:       "email",
		Reason:        "admin_update",
		Active:        false,
		CreatedByRole: "consent_admin", // should preserve original "privacy_admin"
		UpdatedAt:     time.Now().UTC(),
	}
	audit2 := domain.AuditEvent{ID: "aud-2", TenantID: tenantID, Action: "consent.suppression.changed", ConsentID: newAttemptedID}
	outbox2 := domain.OutboxEvent{ID: "evt-2", TenantID: tenantID, EventType: "consent.suppression.changed", AggregateID: newAttemptedID, Payload: map[string]any{"created_by_role": "consent_admin"}}

	s2, err := m.SetSuppression(ctx, update, audit2, outbox2)
	if err != nil {
		t.Fatalf("updating SetSuppression failed: %v", err)
	}

	// Assert semantic parity with Postgres durable repository contract:
	// a. The original ID must be preserved
	if s2.ID != originalID {
		t.Fatalf("expected preserved ID %s, got %s", originalID, s2.ID)
	}
	// b. The original CreatedByRole must be preserved
	if s2.CreatedByRole != "privacy_admin" {
		t.Fatalf("expected preserved CreatedByRole privacy_admin, got %s", s2.CreatedByRole)
	}
	// c. Version must be incremented to 2
	if s2.Version != 2 {
		t.Fatalf("expected incremented version 2, got %d", s2.Version)
	}

	// d. Audit event must reference the preserved original ID and incremented revision
	audits := m.Audit()
	if len(audits) != 2 {
		t.Fatalf("expected 2 audit events, got %d", len(audits))
	}
	lastAudit := audits[1]
	if lastAudit.ConsentID != originalID {
		t.Fatalf("expected audit ConsentID to reference preserved ID %s, got %s", originalID, lastAudit.ConsentID)
	}
	if lastAudit.Revision != 2 {
		t.Fatalf("expected audit Revision to be 2, got %d", lastAudit.Revision)
	}

	// e. Outbox event must reference the preserved original ID in AggregateID and payload
	outboxes := m.Outbox()
	if len(outboxes) != 2 {
		t.Fatalf("expected 2 outbox events, got %d", len(outboxes))
	}
	lastOutbox := outboxes[1]
	if lastOutbox.AggregateID != originalID {
		t.Fatalf("expected outbox AggregateID to reference preserved ID %s, got %s", originalID, lastOutbox.AggregateID)
	}
	if roleInPayload, ok := lastOutbox.Payload["created_by_role"].(string); !ok || roleInPayload != "privacy_admin" {
		t.Fatalf("expected outbox payload created_by_role to be privacy_admin, got %v", lastOutbox.Payload["created_by_role"])
	}

	// f. T-030 administrative protection check: non-admin cannot deactivate administrative suppression
	// Reactivate as privacy_admin first
	reactivate := domain.Suppression{
		ID:            "sup-3",
		TenantID:      tenantID,
		SubjectID:     subjectID,
		Scope:         domain.SuppressionChannel,
		Channel:       "email",
		Reason:        "admin_reactivate",
		Active:        true,
		CreatedByRole: "privacy_admin",
		UpdatedAt:     time.Now().UTC(),
	}
	if _, err := m.SetSuppression(ctx, reactivate, audit1, outbox1); err != nil {
		t.Fatalf("failed to reactivate: %v", err)
	}

	// Now try to deactivate with self_service
	deactivateBySelfService := domain.Suppression{
		ID:            "sup-4",
		TenantID:      tenantID,
		SubjectID:     subjectID,
		Scope:         domain.SuppressionChannel,
		Channel:       "email",
		Reason:        "self_service_deactivate",
		Active:        false,
		CreatedByRole: "self_service",
		UpdatedAt:     time.Now().UTC(),
	}
	if _, err := m.SetSuppression(ctx, deactivateBySelfService, audit2, outbox2); !errors.Is(err, domain.ErrSuppressionProtected) {
		t.Fatalf("expected ErrSuppressionProtected when self_service deactivates admin suppression, got %v", err)
	}
}

func TestMemoryDecoupling_MutableReferenceIsolation(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemory()

	now := time.Now().UTC()
	grantedAt := now
	consent := domain.Consent{
		ID:        "c-decouple",
		TenantID:  "tenant-decouple",
		SubjectID: "user-1",
		Purpose:   "marketing",
		Channel:   "email",
		Status:    domain.StatusGranted,
		Scope:     map[string]string{"tier": "gold"},
		GrantedAt: &grantedAt,
		CreatedAt: now,
		UpdatedAt: now,
	}

	recorded, err := m.RecordGoverned(ctx, consent, domain.ConsentReceipt{}, domain.AuditEvent{ID: "aud-d", Changes: map[string]any{"initial": "val"}}, domain.OutboxEvent{ID: "evt-d", Payload: map[string]any{"event": "one"}})
	if err != nil {
		t.Fatalf("RecordGoverned failed: %v", err)
	}

	// Mutate the returned record
	recorded.Scope["tier"] = "hacked"
	recorded.Scope["leak"] = "added"
	*recorded.GrantedAt = time.Unix(0, 0)

	// Mutate the input struct
	consent.Scope["tier"] = "mutated_input"

	// Fetch fresh copy from store
	fetched, err := m.Get(ctx, "tenant-decouple", "c-decouple")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if fetched.Scope["tier"] != "gold" {
		t.Fatalf("expected stored scope tier to remain 'gold', got '%s'", fetched.Scope["tier"])
	}
	if _, exists := fetched.Scope["leak"]; exists {
		t.Fatalf("expected stored scope not to contain leaked key")
	}
	if fetched.GrantedAt.Equal(time.Unix(0, 0)) {
		t.Fatalf("expected stored GrantedAt pointer not to be mutated by caller")
	}

	// Test Purpose slice decoupling
	purpose := domain.Purpose{
		TenantID:        "tenant-decouple",
		Code:            "p-decouple",
		AllowedChannels: []string{"email", "sms"},
		Active:          true,
	}
	storedP, err := m.PutPurpose(ctx, purpose, domain.AuditEvent{ID: "aud-p"}, domain.OutboxEvent{ID: "evt-p"})
	if err != nil {
		t.Fatalf("PutPurpose failed: %v", err)
	}
	storedP.AllowedChannels[0] = "mutated"
	purpose.AllowedChannels[0] = "mutated_input"

	fetchedP, err := m.GetPurpose(ctx, "tenant-decouple", "p-decouple")
	if err != nil {
		t.Fatalf("GetPurpose failed: %v", err)
	}
	if fetchedP.AllowedChannels[0] != "email" {
		t.Fatalf("expected stored AllowedChannels[0] to remain 'email', got '%s'", fetchedP.AllowedChannels[0])
	}

	// Test LawfulBasis slice decoupling
	basis := domain.LawfulBasis{
		TenantID:          "tenant-decouple",
		Code:              "b-decouple",
		AllowedCategories: []string{"cat1", "cat2"},
		Active:            true,
	}
	storedB, err := m.PutLawfulBasis(ctx, basis, domain.AuditEvent{ID: "aud-b"}, domain.OutboxEvent{ID: "evt-b"})
	if err != nil {
		t.Fatalf("PutLawfulBasis failed: %v", err)
	}
	storedB.AllowedCategories[0] = "mutated"
	basis.AllowedCategories[0] = "mutated_input"

	fetchedB, err := m.GetLawfulBasis(ctx, "tenant-decouple", "b-decouple")
	if err != nil {
		t.Fatalf("GetLawfulBasis failed: %v", err)
	}
	if fetchedB.AllowedCategories[0] != "cat1" {
		t.Fatalf("expected stored AllowedCategories[0] to remain 'cat1', got '%s'", fetchedB.AllowedCategories[0])
	}

	// Test Audit & Outbox slice/map decoupling
	audits := m.Audit()
	audits[0].Changes["initial"] = "mutated_audit"
	if m.Audit()[0].Changes["initial"] != "val" {
		t.Fatalf("Audit() returned mutable reference to internal changes map")
	}

	outboxes := m.Outbox()
	outboxes[0].Payload["event"] = "mutated_outbox"
	if m.Outbox()[0].Payload["event"] != "one" {
		t.Fatalf("Outbox() returned mutable reference to internal payload map")
	}
}

func TestMemoryIdempotencyLedger_InstanceOwnership(t *testing.T) {
	ctx := context.Background()
	m1 := store.NewMemory()
	m2 := store.NewMemory()

	tenantID := "tenant-instance"
	idempKey := "key-instance-1"
	consent := domain.Consent{
		ID:        "c-instance-1",
		TenantID:  tenantID,
		SubjectID: "user-1",
		Purpose:   "marketing",
		Channel:   "email",
		Status:    domain.StatusGranted,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if _, err := m1.RecordGovernedIdempotent(ctx, idempKey, "hash-1", consent, domain.ConsentReceipt{}, domain.AuditEvent{ID: "aud-1"}, domain.OutboxEvent{ID: "evt-1"}); err != nil {
		t.Fatalf("m1.RecordGovernedIdempotent failed: %v", err)
	}

	// m1 must have the snapshot
	_, _, ok1, err := m1.FindIdempotencySnapshot(ctx, tenantID, idempKey)
	if err != nil || !ok1 {
		t.Fatalf("expected m1 to have snapshot: ok=%v, err=%v", ok1, err)
	}

	// m2 MUST NOT have the snapshot because each Memory instance owns its own ledger
	_, _, ok2, err := m2.FindIdempotencySnapshot(ctx, tenantID, idempKey)
	if err != nil || ok2 {
		t.Fatalf("expected m2 to have NO snapshot (isolated instance): ok=%v, err=%v", ok2, err)
	}
}
