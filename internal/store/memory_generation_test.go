package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
)

const genTenant = "tenant-a"

func genConsent(id string, status domain.Status, updatedAt time.Time) domain.Consent {
	return domain.Consent{
		ID: id, TenantID: genTenant, SubjectID: "cus-1", Purpose: "marketing", Channel: "email",
		LegalBasis: "consent", Status: status, Source: "test", CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}
}

func genChallenge(id, tokenHash string, at time.Time) domain.Consent {
	c := genConsent(id, domain.StatusPendingConfirmation, at)
	expires := at.Add(time.Hour)
	c.ConfirmationTokenHash, c.ConfirmationExpiresAt = tokenHash, &expires
	return c
}

// okFinalizer builds valid side-effect records and counts invocations. It also
// reads policy through the transactional reader, which would deadlock if the
// reader re-acquired the store mutex.
func okFinalizer(calls *int) ConfirmFinalizer {
	return func(ctx context.Context, policy PolicyReader, pending domain.Consent, at time.Time) (domain.ConsentReceipt, domain.AuditEvent, domain.OutboxEvent, error) {
		*calls++
		if _, err := policy.GetPurpose(ctx, pending.TenantID, pending.Purpose); err != nil && !errors.Is(err, ErrNotFound) {
			return domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, err
		}
		return domain.ConsentReceipt{ID: "rcp_" + pending.ID, TenantID: pending.TenantID, ConsentID: pending.ID, IssuedAt: at},
			domain.AuditEvent{ID: "aud_" + pending.ID, TenantID: pending.TenantID, ConsentID: pending.ID},
			domain.OutboxEvent{ID: "evt_" + pending.ID, TenantID: pending.TenantID, AggregateID: pending.ID}, nil
	}
}

func mustRecord(t *testing.T, m *Memory, c domain.Consent) domain.Consent {
	t.Helper()
	stored, err := m.RecordGoverned(context.Background(), c, domain.ConsentReceipt{}, domain.AuditEvent{ID: "aud_" + c.ID}, domain.OutboxEvent{ID: "evt_" + c.ID})
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

type memCounts struct{ audit, outbox, receipts int }

func countsOf(m *Memory) memCounts {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return memCounts{len(m.audit), len(m.outbox), len(m.receipts)}
}

func assertUnconfirmed(t *testing.T, m *Memory, id string, before memCounts) {
	t.Helper()
	if after := countsOf(m); after != before {
		t.Fatalf("rejected confirmation wrote side effects: %+v -> %+v", before, after)
	}
	stored, err := m.Get(context.Background(), genTenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.StatusPendingConfirmation || stored.ReceiptID != "" || stored.ConfirmedAt != nil {
		t.Fatalf("challenge must stay ungranted: %+v", stored)
	}
}

func TestMemoryGenerationAdvancesAndInvalidatesOlderChallenges(t *testing.T) {
	m := NewMemory()
	now := time.Now().UTC()
	first := mustRecord(t, m, genChallenge("con_1", "tok1", now))
	if first.PreferenceGeneration != 1 {
		t.Fatalf("first intent must be generation 1, got %d", first.PreferenceGeneration)
	}
	second := mustRecord(t, m, genChallenge("con_2", "tok2", now))
	if second.PreferenceGeneration != 2 {
		t.Fatalf("superseding challenge must be generation 2, got %d", second.PreferenceGeneration)
	}
	if stored, _ := m.Get(context.Background(), genTenant, "con_1"); stored.ConfirmationTokenHash != "" {
		t.Fatal("superseding challenge must clear the older token")
	}
	denied := mustRecord(t, m, genConsent("con_3", domain.StatusDenied, now))
	if denied.PreferenceGeneration != 3 {
		t.Fatalf("denial must be generation 3, got %d", denied.PreferenceGeneration)
	}
	if stored, _ := m.Get(context.Background(), genTenant, "con_2"); stored.ConfirmationTokenHash != "" {
		t.Fatal("denial must clear the pending token")
	}

	// A different channel is a different preference stream.
	other := genChallenge("con_sms", "tok_sms", now)
	other.Channel = "sms"
	if stored := mustRecord(t, m, other); stored.PreferenceGeneration != 1 {
		t.Fatalf("independent stream must start at 1, got %d", stored.PreferenceGeneration)
	}
	if stored, _ := m.Get(context.Background(), genTenant, "con_3"); stored.PreferenceGeneration != 3 {
		t.Fatal("other streams must not disturb this stream")
	}
}

func TestMemoryConfirmRejectsLegacyGenerationZeroChallenge(t *testing.T) {
	m := NewMemory()
	now := time.Now().UTC()
	legacy := genChallenge("con_legacy", "tok_legacy", now)
	m.consents[consentKey{tenantID: genTenant, consentID: legacy.ID}] = legacy // pre-migration row: generation 0, token intact

	calls := 0
	before := countsOf(m)
	if _, err := m.ConfirmDoubleOptIn(context.Background(), genTenant, "tok_legacy", now, okFinalizer(&calls)); !errors.Is(err, ErrStaleGeneration) || !errors.Is(err, ErrConflict) {
		t.Fatalf("legacy challenge must be rejected as stale, got %v", err)
	}
	if calls != 0 {
		t.Fatal("finalizer must not run for a stale challenge")
	}
	assertUnconfirmed(t, m, legacy.ID, before)
}

func TestMemoryConfirmRejectsStaleGenerationEvenWithIntactToken(t *testing.T) {
	m := NewMemory()
	now := time.Now().UTC()
	pending := mustRecord(t, m, genChallenge("con_old", "tok_old", now))
	// Simulate a later intent whose token sweep did not reach this row.
	m.generations[consentPreferenceKey(pending)] = pending.PreferenceGeneration + 1

	calls := 0
	before := countsOf(m)
	if _, err := m.ConfirmDoubleOptIn(context.Background(), genTenant, "tok_old", now, okFinalizer(&calls)); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale generation must be rejected, got %v", err)
	}
	if calls != 0 {
		t.Fatal("finalizer must not run for a stale challenge")
	}
	assertUnconfirmed(t, m, pending.ID, before)
}

func TestMemoryConfirmChecksExpiryAndStatusBeforeFinalizer(t *testing.T) {
	m := NewMemory()
	now := time.Now().UTC()
	pending := mustRecord(t, m, genChallenge("con_exp", "tok_exp", now))
	calls := 0
	before := countsOf(m)
	if _, err := m.ConfirmDoubleOptIn(context.Background(), genTenant, "tok_exp", pending.ConfirmationExpiresAt.Add(time.Nanosecond), okFinalizer(&calls)); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired challenge must be rejected, got %v", err)
	}
	if _, err := m.ConfirmDoubleOptIn(context.Background(), genTenant, "tok_exp", *pending.ConfirmationExpiresAt, okFinalizer(&calls)); !errors.Is(err, ErrExpired) {
		t.Fatalf("challenge must be expired at its expiry instant, got %v", err)
	}
	if _, err := m.ConfirmDoubleOptIn(context.Background(), "tenant-b", "tok_exp", now, okFinalizer(&calls)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("token must be tenant-scoped, got %v", err)
	}
	if _, err := m.ConfirmDoubleOptIn(context.Background(), genTenant, "", now, okFinalizer(&calls)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty token must never match, got %v", err)
	}
	if calls != 0 {
		t.Fatal("finalizer must not run")
	}
	assertUnconfirmed(t, m, pending.ID, before)
}

func TestMemoryConfirmFinalizerFailureLeavesNoMutation(t *testing.T) {
	m := NewMemory()
	now := time.Now().UTC()
	pending := mustRecord(t, m, genChallenge("con_fin", "tok_fin", now))
	policyRejected := errors.New("policy changed")

	before := countsOf(m)
	failing := func(context.Context, PolicyReader, domain.Consent, time.Time) (domain.ConsentReceipt, domain.AuditEvent, domain.OutboxEvent, error) {
		return domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, policyRejected
	}
	if _, err := m.ConfirmDoubleOptIn(context.Background(), genTenant, "tok_fin", now, failing); !errors.Is(err, policyRejected) {
		t.Fatalf("finalizer error must propagate, got %v", err)
	}
	assertUnconfirmed(t, m, pending.ID, before)

	foreignReceipt := func(_ context.Context, _ PolicyReader, p domain.Consent, at time.Time) (domain.ConsentReceipt, domain.AuditEvent, domain.OutboxEvent, error) {
		return domain.ConsentReceipt{ID: "rcp_x", TenantID: p.TenantID, ConsentID: "con_other"}, domain.AuditEvent{}, domain.OutboxEvent{}, nil
	}
	if _, err := m.ConfirmDoubleOptIn(context.Background(), genTenant, "tok_fin", now, foreignReceipt); !errors.Is(err, ErrConflict) {
		t.Fatalf("receipt bound to another consent must be refused, got %v", err)
	}
	assertUnconfirmed(t, m, pending.ID, before)

	// The challenge is still confirmable afterwards, exactly once.
	calls := 0
	granted, err := m.ConfirmDoubleOptIn(context.Background(), genTenant, "tok_fin", now, okFinalizer(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if granted.Status != domain.StatusGranted || granted.ReceiptID != "rcp_con_fin" || granted.ConfirmationTokenHash != "" || granted.PreferenceGeneration != pending.PreferenceGeneration {
		t.Fatalf("unexpected grant: %+v", granted)
	}
	if _, err := m.ConfirmDoubleOptIn(context.Background(), genTenant, "tok_fin", now, okFinalizer(&calls)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("confirmation must be single-use, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("finalizer must run exactly once, ran %d times", calls)
	}
}

func TestMemoryFindEffectivePrefersGenerationOverTimestamp(t *testing.T) {
	m := NewMemory()
	now := time.Now().UTC()
	// The later intent carries an older updated_at (clock skew, backdated
	// evidence); the generation, not the timestamp, decides.
	mustRecord(t, m, genConsent("con_grant", domain.StatusGranted, now.Add(time.Hour)))
	denied := mustRecord(t, m, genConsent("con_deny", domain.StatusDenied, now))
	effective, err := m.FindEffective(context.Background(), genTenant, "cus-1", "marketing", "email")
	if err != nil {
		t.Fatal(err)
	}
	if effective.ID != denied.ID {
		t.Fatalf("highest generation must be effective, got %+v", effective)
	}

	// Withdrawing the older grant is newer intent and becomes effective.
	grant, _ := m.Get(context.Background(), genTenant, "con_grant")
	withdrawn := grant
	withdrawn.Status = domain.StatusWithdrawn
	if _, err := m.Withdraw(context.Background(), withdrawn, domain.AuditEvent{}, domain.OutboxEvent{}); err != nil {
		t.Fatal(err)
	}
	if effective, _ := m.FindEffective(context.Background(), genTenant, "cus-1", "marketing", "email"); effective.ID != "con_grant" || effective.Status != domain.StatusWithdrawn || effective.PreferenceGeneration != 3 {
		t.Fatalf("withdrawal must be the effective state: %+v", effective)
	}
}

func TestMemoryWithdrawInvalidatesPendingChallenge(t *testing.T) {
	m := NewMemory()
	now := time.Now().UTC()
	pending := mustRecord(t, m, genChallenge("con_wd", "tok_wd", now))
	withdrawn := pending
	withdrawn.Status = domain.StatusWithdrawn
	if _, err := m.Withdraw(context.Background(), withdrawn, domain.AuditEvent{}, domain.OutboxEvent{}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if _, err := m.ConfirmDoubleOptIn(context.Background(), genTenant, "tok_wd", now, okFinalizer(&calls)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("withdrawn challenge must not be confirmable, got %v", err)
	}
	if calls != 0 {
		t.Fatal("finalizer must not run")
	}
	// Re-withdrawing is idempotent and does not advance the generation.
	before := m.generations[consentPreferenceKey(pending)]
	if _, err := m.Withdraw(context.Background(), withdrawn, domain.AuditEvent{}, domain.OutboxEvent{}); err != nil {
		t.Fatal(err)
	}
	if after := m.generations[consentPreferenceKey(pending)]; after != before {
		t.Fatalf("idempotent withdrawal advanced the generation: %d -> %d", before, after)
	}
}

func TestMemoryIdempotentReplayDoesNotAdvanceGeneration(t *testing.T) {
	m := NewMemory()
	now := time.Now().UTC()
	pending := mustRecord(t, m, genChallenge("con_pending", "tok_p", now))
	first, err := m.RecordGovernedIdempotent(context.Background(), "idem-1", "hash-1", genConsent("con_idem", domain.StatusDenied, now), domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{})
	if err != nil {
		t.Fatal(err)
	}
	if first.PreferenceGeneration != pending.PreferenceGeneration+1 {
		t.Fatalf("first write must advance the generation: %d", first.PreferenceGeneration)
	}
	if stored, _ := m.Get(context.Background(), genTenant, pending.ID); stored.ConfirmationTokenHash != "" {
		t.Fatal("idempotent denial must invalidate the pending challenge")
	}
	replay, err := m.RecordGovernedIdempotent(context.Background(), "idem-1", "hash-1", genConsent("con_idem_retry", domain.StatusDenied, now), domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{})
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != first.ID || replay.PreferenceGeneration != first.PreferenceGeneration || m.generations[consentPreferenceKey(first)] != first.PreferenceGeneration {
		t.Fatalf("replay must return the original record without advancing: first=%+v replay=%+v", first, replay)
	}
	if _, err := m.RecordGovernedIdempotent(context.Background(), "idem-1", "hash-other", genConsent("con_idem_x", domain.StatusGranted, now), domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("key reuse with a different request must conflict, got %v", err)
	}
	if m.generations[consentPreferenceKey(first)] != first.PreferenceGeneration {
		t.Fatal("conflicting replay must not advance the generation")
	}
}
