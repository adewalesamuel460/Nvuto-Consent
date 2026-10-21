package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/service"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// AT-010: Event contracts, revision sequence ordering, and consumer reconciliation tests.

func setupServiceWithRegistries(t *testing.T) (*service.Service, *store.Memory) {
	t.Helper()
	repo := store.NewMemory()
	svc := service.New(repo)
	ctx := context.Background()
	const tenant = "tenant-event-contract"
	const actor = "actor-admin"

	// 1. Purpose for general marketing (direct consent)
	_, err := svc.RegisterPurpose(ctx, tenant, actor, domain.Purpose{
		Code:                       "email_marketing",
		Category:                   "marketing",
		Active:                     true,
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        false,
		Version:                    1,
	})
	if err != nil {
		t.Fatalf("RegisterPurpose marketing failed: %v", err)
	}

	// 2. Purpose requiring double opt-in
	_, err = svc.RegisterPurpose(ctx, tenant, actor, domain.Purpose{
		Code:                       "newsletter_doi",
		Category:                   "marketing",
		Active:                     true,
		AllowedChannels:            []string{"email"},
		RequiresAffirmativeConsent: true,
		RequiresDoubleOptIn:        true,
		Version:                    1,
	})
	if err != nil {
		t.Fatalf("RegisterPurpose doi failed: %v", err)
	}

	// 3. Lawful basis
	_, err = svc.RegisterLawfulBasis(ctx, tenant, actor, domain.LawfulBasis{
		Code:              "consent",
		Active:            true,
		AllowedCategories: []string{"marketing"},
		RequiresEvidence:  true,
		Version:           1,
	})
	if err != nil {
		t.Fatalf("RegisterLawfulBasis failed: %v", err)
	}

	// 4. Privacy notice
	content := "This is notice version 2026.1"
	digest := sha256.Sum256([]byte(content))
	contentHash := "sha256:" + hex.EncodeToString(digest[:])
	_, err = svc.PublishNotice(ctx, tenant, actor, domain.PrivacyNotice{
		Jurisdiction: "GLOBAL",
		Version:      "2026.1",
		ContentHash:  contentHash,
		Active:       true,
		PublishedAt:  time.Now().UTC().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("PublishNotice failed: %v", err)
	}

	return svc, repo
}

func findOutboxEventByType(events []domain.OutboxEvent, eventType string) (domain.OutboxEvent, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].EventType == eventType {
			return events[i], true
		}
	}
	return domain.OutboxEvent{}, false
}

// TestEventContract_AllEightFamiliesAssertSchema validates that every event family
// emits correct aggregate_type, monotonic aggregate_revision, tenant_id, schema_version,
// and canonical subject_id where applicable across both Go model and serialized JSON payload.
func TestEventContract_AllEightFamiliesAssertSchema(t *testing.T) {
	svc, repo := setupServiceWithRegistries(t)
	ctx := context.Background()
	const tenant = "tenant-event-contract"
	const actor = "actor-admin"
	const subjectID = "customer-sub-900"
	now := time.Now().UTC()

	evidence := domain.Evidence{
		Method:       "web_form",
		ArtifactHash: "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
		CollectedAt:  now,
	}

	// 1. Family: consent.purpose (consent.purpose.changed)
	t.Run("Family 1: consent.purpose", func(t *testing.T) {
		_, err := svc.RegisterPurpose(ctx, tenant, actor, domain.Purpose{
			Code:            "analytics_purpose",
			Category:        "analytics",
			Active:          true,
			AllowedChannels: []string{"web"},
			Version:         2,
		})
		if err != nil {
			t.Fatalf("failed to update purpose: %v", err)
		}
		evt, ok := findOutboxEventByType(repo.Outbox(), "consent.purpose.changed")
		if !ok {
			t.Fatalf("outbox event not found")
		}
		assertEventContract(t, evt, "consent.purpose", 2, tenant, false)
	})

	// 2. Family: consent.lawful_basis (consent.lawful_basis.changed)
	t.Run("Family 2: consent.lawful_basis", func(t *testing.T) {
		_, err := svc.RegisterLawfulBasis(ctx, tenant, actor, domain.LawfulBasis{
			Code:              "legitimate_interest",
			Active:            true,
			AllowedCategories: []string{"analytics"},
			Version:           1,
		})
		if err != nil {
			t.Fatalf("failed to register lawful basis: %v", err)
		}
		evt, ok := findOutboxEventByType(repo.Outbox(), "consent.lawful_basis.changed")
		if !ok {
			t.Fatalf("outbox event not found")
		}
		assertEventContract(t, evt, "consent.lawful_basis", 1, tenant, false)
	})

	// 3. Family: consent.notice (consent.notice.published and consent.notice.retired)
	t.Run("Family 3: consent.notice.published", func(t *testing.T) {
		evt, ok := findOutboxEventByType(repo.Outbox(), "consent.notice.published")
		if !ok {
			t.Fatalf("outbox event not found")
		}
		assertEventContract(t, evt, "consent.notice", 1, tenant, false)
	})

	t.Run("Family 3: consent.notice.retired", func(t *testing.T) {
		legacyDigest := sha256.Sum256([]byte("legacy notice"))
		_, err := svc.PublishNotice(ctx, tenant, actor, domain.PrivacyNotice{
			Jurisdiction: "GLOBAL",
			Version:      "2025.legacy",
			ContentHash:  "sha256:" + hex.EncodeToString(legacyDigest[:]),
			Active:       true,
			PublishedAt:  now.Add(-2 * time.Hour),
		})
		if err != nil {
			t.Fatalf("failed to publish legacy notice: %v", err)
		}
		_, err = svc.RetireNotice(ctx, tenant, actor, "GLOBAL", "2025.legacy", "2026.1")
		if err != nil {
			t.Fatalf("failed to retire notice: %v", err)
		}
		evt, ok := findOutboxEventByType(repo.Outbox(), "consent.notice.retired")
		if !ok {
			t.Fatalf("outbox event not found")
		}
		assertEventContract(t, evt, "consent.notice", 2, tenant, false)
	})

	// 4. Family: consent.preference (consent.granted & consent.denied)
	var grantedConsent domain.Consent
	t.Run("Family 4: consent.preference (granted)", func(t *testing.T) {
		var err error
		grantedConsent, err = svc.RecordGoverned(ctx, tenant, actor, subjectID, "email_marketing", "email", "consent", string(domain.StatusGranted), "web", nil, nil, "GLOBAL", "2026.1", evidence)
		if err != nil {
			t.Fatalf("failed to record grant: %v", err)
		}
		evt, ok := findOutboxEventByType(repo.Outbox(), "consent.granted")
		if !ok {
			t.Fatalf("outbox event not found")
		}
		assertEventContract(t, evt, "consent.preference", 1, tenant, true)
	})

	t.Run("Family 4: consent.preference (denied)", func(t *testing.T) {
		_, err := svc.RecordGoverned(ctx, tenant, actor, "denied-subject", "email_marketing", "email", "consent", string(domain.StatusDenied), "web", nil, nil, "GLOBAL", "2026.1", evidence)
		if err != nil {
			t.Fatalf("failed to record denial: %v", err)
		}
		evt, ok := findOutboxEventByType(repo.Outbox(), "consent.denied")
		if !ok {
			t.Fatalf("outbox event not found")
		}
		assertEventContract(t, evt, "consent.preference", 1, tenant, true)
	})

	// 5. Family: consent.preference (consent.withdrawn)
	t.Run("Family 5: consent.preference (withdrawn)", func(t *testing.T) {
		_, err := svc.Withdraw(ctx, tenant, actor, grantedConsent.ID)
		if err != nil {
			t.Fatalf("failed to withdraw: %v", err)
		}
		evt, ok := findOutboxEventByType(repo.Outbox(), "consent.withdrawn")
		if !ok {
			t.Fatalf("outbox event not found")
		}
		assertEventContract(t, evt, "consent.preference", 2, tenant, true)
	})

	// 6. Family: consent.suppression (consent.suppression.changed)
	t.Run("Family 6: consent.suppression", func(t *testing.T) {
		_, err := svc.SetSuppression(ctx, tenant, actor, domain.Suppression{
			SubjectID: subjectID,
			Scope:     domain.SuppressionGlobal,
			Reason:    "Legal opt out request",
			Active:    true,
		}, "privacy_admin")
		if err != nil {
			t.Fatalf("failed to set suppression: %v", err)
		}
		evt, ok := findOutboxEventByType(repo.Outbox(), "consent.suppression.changed")
		if !ok {
			t.Fatalf("outbox event not found")
		}
		assertEventContract(t, evt, "consent.suppression", 1, tenant, true)
	})

	// 7. Family: consent.double_opt_in (consent.challenge.created)
	var challenge domain.DoubleOptInChallenge
	t.Run("Family 7: consent.double_opt_in (challenge.created)", func(t *testing.T) {
		var err error
		challenge, err = svc.StartDoubleOptIn(ctx, tenant, actor, subjectID, "newsletter_doi", "email", "web", "GLOBAL", "2026.1", evidence, now.Add(24*time.Hour))
		if err != nil {
			t.Fatalf("failed to start double opt-in: %v", err)
		}
		evt, ok := findOutboxEventByType(repo.Outbox(), "consent.challenge.created")
		if !ok {
			t.Fatalf("outbox event not found")
		}
		assertEventContract(t, evt, "consent.double_opt_in", 1, tenant, true)
	})

	// 8. Family: consent.preference (consent.confirmed)
	t.Run("Family 8: consent.preference (confirmed)", func(t *testing.T) {
		scope := service.TenantWide()
		_, err := svc.ConfirmDoubleOptIn(ctx, tenant, actor, scope, challenge.ConfirmationToken)
		if err != nil {
			t.Fatalf("failed to confirm double opt in: %v", err)
		}
		evt, ok := findOutboxEventByType(repo.Outbox(), "consent.confirmed")
		if !ok {
			t.Fatalf("outbox event not found")
		}
		assertEventContract(t, evt, "consent.preference", 1, tenant, true)
	})
}

func assertEventContract(t *testing.T, evt domain.OutboxEvent, expectedAggregateType string, expectedMinRev int64, expectedTenant string, requireSubject bool) {
	t.Helper()

	// 1. Struct field assertions
	if evt.AggregateType != expectedAggregateType {
		t.Errorf("expected AggregateType %q, got %q", expectedAggregateType, evt.AggregateType)
	}
	if evt.AggregateRevision < expectedMinRev {
		t.Errorf("expected AggregateRevision >= %d, got %d", expectedMinRev, evt.AggregateRevision)
	}
	if evt.TenantID != expectedTenant {
		t.Errorf("expected TenantID %q, got %q", expectedTenant, evt.TenantID)
	}
	if evt.SchemaVersion != 1 {
		t.Errorf("expected SchemaVersion 1, got %d", evt.SchemaVersion)
	}

	// 2. Serialized JSON payload assertions
	payloadBytes, err := json.Marshal(evt.Payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	var payloadMap map[string]any
	if err := json.Unmarshal(payloadBytes, &payloadMap); err != nil {
		t.Fatalf("failed to unmarshal payload JSON: %v", err)
	}

	// schema_version
	if sv, ok := payloadMap["schema_version"].(float64); !ok || int(sv) != 1 {
		t.Errorf("payload missing or invalid schema_version: %v", payloadMap["schema_version"])
	}

	// aggregate_type
	if at, ok := payloadMap["aggregate_type"].(string); !ok || at != expectedAggregateType {
		t.Errorf("payload missing or invalid aggregate_type: expected %q, got %v", expectedAggregateType, payloadMap["aggregate_type"])
	}

	// aggregate_revision
	if ar, ok := payloadMap["aggregate_revision"].(float64); !ok || int64(ar) < expectedMinRev {
		t.Errorf("payload missing or invalid aggregate_revision: expected >= %d, got %v", expectedMinRev, payloadMap["aggregate_revision"])
	}

	// tenant_id
	if tid, ok := payloadMap["tenant_id"].(string); !ok || tid != expectedTenant {
		t.Errorf("payload missing or invalid tenant_id: expected %q, got %v", expectedTenant, payloadMap["tenant_id"])
	}

	// subject_id (when required)
	if requireSubject {
		if sid, ok := payloadMap["subject_id"].(string); !ok || sid == "" {
			t.Errorf("payload missing or empty subject_id")
		}
	}
}

// SimulatedConsumer represents a downstream platform consumer (e.g. Engage INT-01 or Conversations INT-06)
// maintaining a local cache indexed by (tenant_id, subject_id, purpose, channel).
type SimulatedConsumer struct {
	storedRevisions map[string]int64
	storedStatus    map[string]string
	storedExpiresAt map[string]time.Time
	discardedEvents []string
}

func newSimulatedConsumer() *SimulatedConsumer {
	return &SimulatedConsumer{
		storedRevisions: make(map[string]int64),
		storedStatus:    make(map[string]string),
		storedExpiresAt: make(map[string]time.Time),
	}
}

func (c *SimulatedConsumer) key(tenantID, subjectID, purpose, channel string) string {
	return tenantID + "/" + subjectID + "/" + purpose + "/" + channel
}

// HandleEvent applies the monotonic ordering rule from ARC-005:
// Discards any incoming event where aggregate_revision <= local stored revision.
func (c *SimulatedConsumer) HandleEvent(evt domain.OutboxEvent) bool {
	tenantID := evt.TenantID
	subjectID, _ := evt.Payload["subject_id"].(string)
	purpose, _ := evt.Payload["purpose"].(string)
	channel, _ := evt.Payload["channel"].(string)
	k := c.key(tenantID, subjectID, purpose, channel)

	incomingRev := evt.AggregateRevision
	storedRev := c.storedRevisions[k]

	// ARC-005 Ordering Invariant:
	// If incomingRev <= storedRev, event MUST be discarded as stale/out-of-order.
	if incomingRev <= storedRev {
		c.discardedEvents = append(c.discardedEvents, evt.ID)
		return false
	}

	// Apply mutation
	c.storedRevisions[k] = incomingRev
	if status, ok := evt.Payload["status"].(string); ok {
		c.storedStatus[k] = status
	}
	if expStr, ok := evt.Payload["expires_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, expStr); err == nil {
			c.storedExpiresAt[k] = t
		}
	}
	return true
}

// CanCommunicate evaluates whether an outbound message may be dispatched to the subject.
func (c *SimulatedConsumer) CanCommunicate(tenantID, subjectID, purpose, channel string, at time.Time) bool {
	k := c.key(tenantID, subjectID, purpose, channel)
	status := c.storedStatus[k]
	if status != string(domain.StatusGranted) {
		return false // withdrawn, denied, or unknown defaults to safe-deny
	}
	// Check passive wall-clock expiry rule
	if exp, ok := c.storedExpiresAt[k]; ok && !exp.IsZero() {
		if at.After(exp) {
			return false
		}
	}
	return true
}

// TestSimulatedConsumer_ReverseOrderDeliveryDiscard verifies that receiving a consent.granted
// event (Revision 1) AFTER a consent.withdrawn event (Revision 2) successfully discards the
// stale grant, preventing an illegal outbound communication.
func TestSimulatedConsumer_ReverseOrderDeliveryDiscard(t *testing.T) {
	consumer := newSimulatedConsumer()
	const tenant = "tenant-safe-deny"
	const subject = "user-12345"
	const purpose = "email_marketing"
	const channel = "email"

	// Event 1: Customer grants consent at T0 (Revision 1)
	eventGrantRev1 := domain.OutboxEvent{
		ID:                "evt_grant_rev1",
		SchemaVersion:     1,
		TenantID:          tenant,
		EventType:         "consent.granted",
		AggregateType:     "consent.preference",
		AggregateID:       "con_12345",
		AggregateRevision: 1,
		Payload: map[string]any{
			"schema_version":     1,
			"aggregate_type":     "consent.preference",
			"aggregate_revision": int64(1),
			"tenant_id":          tenant,
			"subject_id":         subject,
			"purpose":            purpose,
			"channel":            channel,
			"status":             string(domain.StatusGranted),
		},
		OccurredAt: time.Now().UTC().Add(-10 * time.Minute),
	}

	// Event 2: Customer withdraws consent at T1 (Revision 2)
	eventWithdrawRev2 := domain.OutboxEvent{
		ID:                "evt_withdraw_rev2",
		SchemaVersion:     1,
		TenantID:          tenant,
		EventType:         "consent.withdrawn",
		AggregateType:     "consent.preference",
		AggregateID:       "con_12345",
		AggregateRevision: 2,
		Payload: map[string]any{
			"schema_version":     1,
			"aggregate_type":     "consent.preference",
			"aggregate_revision": int64(2),
			"tenant_id":          tenant,
			"subject_id":         subject,
			"purpose":            purpose,
			"channel":            channel,
			"status":             string(domain.StatusWithdrawn),
		},
		OccurredAt: time.Now().UTC().Add(-5 * time.Minute),
	}

	// STEP 1: Simulate network partition / delay causing REVERSE ORDER DELIVERY.
	// Event 2 (Withdrawal, Rev 2) arrives FIRST:
	applied := consumer.HandleEvent(eventWithdrawRev2)
	if !applied {
		t.Fatalf("expected withdrawal event to be applied")
	}

	// Confirm consumer state is "withdrawn"
	if consumer.CanCommunicate(tenant, subject, purpose, channel, time.Now().UTC()) {
		t.Fatalf("communication must be blocked after withdrawal")
	}

	// STEP 2: Event 1 (Grant, Rev 1) arrives LATE:
	appliedLate := consumer.HandleEvent(eventGrantRev1)
	if appliedLate {
		t.Fatalf("expected late grant event (Rev 1) to be DISCARDED, but it was applied!")
	}

	// Check that the discard was recorded
	if len(consumer.discardedEvents) != 1 || consumer.discardedEvents[0] != "evt_grant_rev1" {
		t.Errorf("expected evt_grant_rev1 to be in discarded list, got %v", consumer.discardedEvents)
	}

	// CRITICAL COMPLIANCE ASSERTION:
	// Communication MUST STILL BE BLOCKED. The late grant did not resurrect permission.
	if consumer.CanCommunicate(tenant, subject, purpose, channel, time.Now().UTC()) {
		t.Fatalf("CRITICAL PRIVACY VIOLATION: Stale grant event revived a withdrawn consent!")
	}

	// STEP 3: Duplicate delivery replay test beyond broker dedup window.
	// Re-sending Event 2 (Withdrawal, Rev 2) must be treated idempotently as discarded.
	appliedDup := consumer.HandleEvent(eventWithdrawRev2)
	if appliedDup {
		t.Fatalf("expected duplicate event with same revision to be discarded")
	}
	if len(consumer.discardedEvents) != 2 {
		t.Errorf("expected 2 discarded events, got %d", len(consumer.discardedEvents))
	}
}

// TestSimulatedConsumer_WallClockExpiryEnforcement asserts that consumers enforce
// passive expiry evaluation prior to outbound send even if no expiry event was received.
func TestSimulatedConsumer_WallClockExpiryEnforcement(t *testing.T) {
	consumer := newSimulatedConsumer()
	const tenant = "tenant-expiry"
	const subject = "user-expired"
	const purpose = "promo"
	const channel = "sms"

	now := time.Now().UTC()
	expiryTime := now.Add(-10 * time.Minute) // Already expired 10 minutes ago

	eventGrantExpired := domain.OutboxEvent{
		ID:                "evt_grant_exp",
		SchemaVersion:     1,
		TenantID:          tenant,
		EventType:         "consent.granted",
		AggregateType:     "consent.preference",
		AggregateID:       "con_exp_1",
		AggregateRevision: 1,
		Payload: map[string]any{
			"schema_version":     1,
			"aggregate_type":     "consent.preference",
			"aggregate_revision": int64(1),
			"tenant_id":          tenant,
			"subject_id":         subject,
			"purpose":            purpose,
			"channel":            channel,
			"status":             string(domain.StatusGranted),
			"expires_at":         expiryTime.Format(time.RFC3339),
		},
		OccurredAt: now.Add(-1 * time.Hour),
	}

	consumer.HandleEvent(eventGrantExpired)

	// Attempting to send NOW must be refused because wall-clock time has passed expires_at
	if consumer.CanCommunicate(tenant, subject, purpose, channel, now) {
		t.Fatalf("communication must be blocked when current time exceeds expires_at")
	}
}
