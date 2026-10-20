package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
)

// Store errors are the typed domain sentinels, so the HTTP layer maps them to
// stable public statuses (T-013) and errors.Is keeps working everywhere.
var (
	ErrNotFound    = domain.ErrNotFound
	ErrConflict    = domain.ErrConflict
	ErrExpired     = domain.ErrExpired
	ErrUnavailable = domain.ErrUnavailable
)

// ErrStaleGeneration is returned when a double-opt-in challenge is no longer
// bound to the current preference generation because a later grant, denial,
// withdrawal or challenge superseded it. It matches ErrConflict.
var ErrStaleGeneration = domain.ErrConflict.Derive("challenge_superseded", "the double opt-in challenge was superseded by a newer preference")

// contextError classifies a context error: an abandoned caller is a
// cancellation, an exhausted deadline is a retryable unavailability.
func contextError(err error) error {
	if errors.Is(err, context.Canceled) {
		return domain.ErrCanceled.Wrap(err)
	}
	return domain.ErrUnavailable.Wrap(err)
}

// live returns a classified error once ctx is done. Every memory-store method
// checks it before touching state, so a cancelled request never mutates.
func live(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return contextError(err)
	}
	return nil
}

// PolicyReader reads the policy registries. During ConfirmDoubleOptIn it is
// bound to the confirmation transaction, so the policy observed is the policy
// in force when the grant commits.
type PolicyReader interface {
	GetPurpose(ctx context.Context, tenantID, code string) (domain.Purpose, error)
	GetLawfulBasis(ctx context.Context, tenantID, code string) (domain.LawfulBasis, error)
	GetNotice(ctx context.Context, tenantID, jurisdiction, version string) (domain.PrivacyNotice, error)
}

// ConfirmFinalizer runs inside the confirmation transaction after the
// challenge and its preference generation have been locked and verified as
// pending, unexpired and current. It re-validates policy through the supplied
// reader and builds the receipt, audit and outbox records from the locked
// challenge. Returning an error aborts the confirmation with no mutation.
// Implementations must not call back into the repository. A durable store may
// invoke it more than once when it retries a transient transaction conflict;
// each invocation's records belong to a transaction that is rolled back unless
// it commits, so it must not have side effects outside its return values.
type ConfirmFinalizer func(ctx context.Context, policy PolicyReader, pending domain.Consent, at time.Time) (domain.ConsentReceipt, domain.AuditEvent, domain.OutboxEvent, error)

// Repository is the persistence boundary. Every method takes the caller's
// context: cancellation or deadline expiry abandons pending work and leaves
// no partial state (T-012).
type Repository interface {
	Get(ctx context.Context, tenantID, id string) (domain.Consent, error)
	FindEffective(ctx context.Context, tenantID, subjectID, purpose, channel string) (domain.Consent, error)
	PutPurpose(ctx context.Context, purpose domain.Purpose, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Purpose, error)
	GetPurpose(ctx context.Context, tenantID, code string) (domain.Purpose, error)
	PutLawfulBasis(ctx context.Context, basis domain.LawfulBasis, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.LawfulBasis, error)
	GetLawfulBasis(ctx context.Context, tenantID, code string) (domain.LawfulBasis, error)
	PutNotice(ctx context.Context, notice domain.PrivacyNotice, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.PrivacyNotice, error)
	GetNotice(ctx context.Context, tenantID, jurisdiction, version string) (domain.PrivacyNotice, error)
	RecordGoverned(ctx context.Context, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error)
	Withdraw(ctx context.Context, consent domain.Consent, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error)
	GetReceipt(ctx context.Context, tenantID, id string) (domain.ConsentReceipt, error)
	SetSuppression(ctx context.Context, suppression domain.Suppression, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Suppression, error)
	FindSuppression(ctx context.Context, tenantID, subjectID, purpose, channel string) (domain.Suppression, error)
	StartDoubleOptIn(ctx context.Context, consent domain.Consent, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error)
	FindPendingByToken(ctx context.Context, tenantID, tokenHash string) (domain.Consent, error)
	ConfirmDoubleOptIn(ctx context.Context, tenantID, tokenHash string, at time.Time, finalize ConfirmFinalizer) (domain.Consent, error)
	AppendAudit(ctx context.Context, event domain.AuditEvent) error
	AppendOutbox(ctx context.Context, event domain.OutboxEvent) error
	Ping(ctx context.Context) error
}

// Memory-store typed composite keys (T-026). In-memory maps use distinct
// structs as map keys rather than concatenated strings, preventing delimiter
// collisions (e.g. "tenant" with ID "child:1" vs "tenant:child" with ID "1").
type consentKey struct {
	tenantID  string
	consentID string
}

type purposeKey struct {
	tenantID string
	code     string
}

type lawfulBasisKey struct {
	tenantID string
	code     string
}

type noticeKey struct {
	tenantID     string
	jurisdiction string
	version      string
}

type receiptKey struct {
	tenantID  string
	receiptID string
}

type suppressionKey struct {
	tenantID  string
	subjectID string
	scope     domain.SuppressionScope
	purpose   string
	channel   string
}

type preferenceKey struct {
	tenantID  string
	subjectID string
	purpose   string
	channel   string
}

type Memory struct {
	mu           sync.RWMutex
	consents     map[consentKey]domain.Consent
	audit        []domain.AuditEvent
	outbox       []domain.OutboxEvent
	purposes     map[purposeKey]domain.Purpose
	bases        map[lawfulBasisKey]domain.LawfulBasis
	notices      map[noticeKey]domain.PrivacyNotice
	receipts     map[receiptKey]domain.ConsentReceipt
	suppressions map[suppressionKey]domain.Suppression
	generations  map[preferenceKey]int64 // current preference generation by preferenceKey
	outboxLeases map[string]*OutboxLeaseState
	clock        func() time.Time
}

func NewMemory() *Memory {
	return &Memory{
		consents:     map[consentKey]domain.Consent{},
		purposes:     map[purposeKey]domain.Purpose{},
		bases:        map[lawfulBasisKey]domain.LawfulBasis{},
		notices:      map[noticeKey]domain.PrivacyNotice{},
		receipts:     map[receiptKey]domain.ConsentReceipt{},
		suppressions: map[suppressionKey]domain.Suppression{},
		generations:  map[preferenceKey]int64{},
		outboxLeases: map[string]*OutboxLeaseState{},
		clock:        time.Now,
	}
}

func consentPreferenceKey(c domain.Consent) preferenceKey {
	return preferenceKey{
		tenantID:  c.TenantID,
		subjectID: c.SubjectID,
		purpose:   c.Purpose,
		channel:   c.Channel,
	}
}

// advanceGenerationLocked increments the preference generation for the
// consent's preference stream and invalidates every pending challenge bound to
// an older generation. Caller must hold m.mu for writing and must only call it
// once the surrounding operation is certain to succeed.
func (m *Memory) advanceGenerationLocked(c domain.Consent) int64 {
	k := consentPreferenceKey(c)
	generation := m.generations[k] + 1
	m.generations[k] = generation
	for id, existing := range m.consents {
		if consentPreferenceKey(existing) == k && existing.Status == domain.StatusPendingConfirmation &&
			existing.ConfirmationTokenHash != "" && existing.PreferenceGeneration < generation {
			existing.ConfirmationTokenHash = ""
			existing.Version++
			m.consents[id] = existing
		}
	}
	return generation
}

func (m *Memory) Get(ctx context.Context, t, id string) (domain.Consent, error) {
	if err := live(ctx); err != nil {
		return domain.Consent{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.consents[consentKey{tenantID: t, consentID: id}]
	if !ok {
		return domain.Consent{}, ErrNotFound
	}
	return c, nil
}

// FindEffective returns the record carrying the highest preference generation;
// updated_at only breaks ties (pre-generation records all have generation 0).
func (m *Memory) FindEffective(ctx context.Context, t, s, p, ch string) (domain.Consent, error) {
	if err := live(ctx); err != nil {
		return domain.Consent{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var best domain.Consent
	found := false
	for _, c := range m.consents {
		if c.TenantID != t || c.SubjectID != s || c.Purpose != p || c.Channel != ch {
			continue
		}
		if !found || c.PreferenceGeneration > best.PreferenceGeneration ||
			(c.PreferenceGeneration == best.PreferenceGeneration && c.UpdatedAt.After(best.UpdatedAt)) {
			best = c
			found = true
		}
	}
	if !found {
		return domain.Consent{}, ErrNotFound
	}
	return best, nil
}

func (m *Memory) PutPurpose(ctx context.Context, purpose domain.Purpose, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Purpose, error) {
	if err := live(ctx); err != nil {
		return domain.Purpose{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := purposeKey{tenantID: purpose.TenantID, code: purpose.Code}
	var before any
	if current, ok := m.purposes[k]; ok {
		before = current
		purpose.Version = current.Version + 1
	} else {
		purpose.Version = 1
	}
	m.purposes[k] = purpose
	if audit.ResourceType == "" {
		audit.ResourceType = "purpose"
	}
	if audit.ConsentID == "" {
		audit.ConsentID = purpose.Code
	}
	audit.Revision = purpose.Version
	audit.Changes = map[string]any{"before": before, "after": purpose}
	m.audit = append(m.audit, audit)
	m.outbox = append(m.outbox, outbox)
	return purpose, nil
}

func (m *Memory) GetPurpose(ctx context.Context, tenantID, code string) (domain.Purpose, error) {
	if err := live(ctx); err != nil {
		return domain.Purpose{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getPurposeLocked(tenantID, code)
}

func (m *Memory) getPurposeLocked(tenantID, code string) (domain.Purpose, error) {
	purpose, ok := m.purposes[purposeKey{tenantID: tenantID, code: code}]
	if !ok {
		return domain.Purpose{}, ErrNotFound
	}
	return purpose, nil
}

func (m *Memory) PutLawfulBasis(ctx context.Context, basis domain.LawfulBasis, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.LawfulBasis, error) {
	if err := live(ctx); err != nil {
		return domain.LawfulBasis{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := lawfulBasisKey{tenantID: basis.TenantID, code: basis.Code}
	var before any
	if current, ok := m.bases[k]; ok {
		before = current
		basis.Version = current.Version + 1
	} else {
		basis.Version = 1
	}
	m.bases[k] = basis
	if audit.ResourceType == "" {
		audit.ResourceType = "lawful_basis"
	}
	if audit.ConsentID == "" {
		audit.ConsentID = basis.Code
	}
	audit.Revision = basis.Version
	audit.Changes = map[string]any{"before": before, "after": basis}
	m.audit = append(m.audit, audit)
	m.outbox = append(m.outbox, outbox)
	return basis, nil
}

func (m *Memory) GetLawfulBasis(ctx context.Context, tenantID, code string) (domain.LawfulBasis, error) {
	if err := live(ctx); err != nil {
		return domain.LawfulBasis{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getLawfulBasisLocked(tenantID, code)
}

func (m *Memory) getLawfulBasisLocked(tenantID, code string) (domain.LawfulBasis, error) {
	basis, ok := m.bases[lawfulBasisKey{tenantID: tenantID, code: code}]
	if !ok {
		return domain.LawfulBasis{}, ErrNotFound
	}
	return basis, nil
}

func (m *Memory) PutNotice(ctx context.Context, notice domain.PrivacyNotice, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.PrivacyNotice, error) {
	if err := live(ctx); err != nil {
		return domain.PrivacyNotice{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := noticeKey{tenantID: notice.TenantID, jurisdiction: notice.Jurisdiction, version: notice.Version}
	if _, exists := m.notices[k]; exists {
		return domain.PrivacyNotice{}, ErrConflict
	}
	m.notices[k] = notice
	if audit.ResourceType == "" {
		audit.ResourceType = "privacy_notice"
	}
	if audit.ConsentID == "" {
		audit.ConsentID = notice.Jurisdiction + "/" + notice.Version
	}
	if audit.Revision <= 0 {
		audit.Revision = 1
	}
	audit.Changes = map[string]any{"before": nil, "after": notice}
	m.audit = append(m.audit, audit)
	m.outbox = append(m.outbox, outbox)
	return notice, nil
}

func (m *Memory) GetNotice(ctx context.Context, tenantID, jurisdiction, version string) (domain.PrivacyNotice, error) {
	if err := live(ctx); err != nil {
		return domain.PrivacyNotice{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getNoticeLocked(tenantID, jurisdiction, version)
}

func (m *Memory) getNoticeLocked(tenantID, jurisdiction, version string) (domain.PrivacyNotice, error) {
	notice, ok := m.notices[noticeKey{tenantID: tenantID, jurisdiction: jurisdiction, version: version}]
	if !ok {
		return domain.PrivacyNotice{}, ErrNotFound
	}
	return notice, nil
}

// memoryPolicyReader reads the registries while the caller already holds m.mu,
// giving the confirmation finalizer the same isolation a database transaction
// provides.
type memoryPolicyReader struct{ m *Memory }

func (r memoryPolicyReader) GetPurpose(_ context.Context, tenantID, code string) (domain.Purpose, error) {
	return r.m.getPurposeLocked(tenantID, code)
}

func (r memoryPolicyReader) GetLawfulBasis(_ context.Context, tenantID, code string) (domain.LawfulBasis, error) {
	return r.m.getLawfulBasisLocked(tenantID, code)
}

func (r memoryPolicyReader) GetNotice(_ context.Context, tenantID, jurisdiction, version string) (domain.PrivacyNotice, error) {
	return r.m.getNoticeLocked(tenantID, jurisdiction, version)
}

func (m *Memory) RecordGoverned(ctx context.Context, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	if err := live(ctx); err != nil {
		return domain.Consent{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Validate everything before mutating: the memory store has no rollback.
	cKey := consentKey{tenantID: consent.TenantID, consentID: consent.ID}
	if _, exists := m.consents[cKey]; exists {
		return domain.Consent{}, ErrConflict
	}
	if receipt.ID != "" {
		rKey := receiptKey{tenantID: receipt.TenantID, receiptID: receipt.ID}
		if _, exists := m.receipts[rKey]; exists {
			return domain.Consent{}, ErrConflict
		}
	}
	consent.PreferenceGeneration = m.advanceGenerationLocked(consent)
	if receipt.ID != "" {
		m.receipts[receiptKey{tenantID: receipt.TenantID, receiptID: receipt.ID}] = receipt
	}
	m.consents[cKey] = consent
	if audit.ResourceType == "" {
		audit.ResourceType = "consent"
	}
	if audit.Revision <= 0 {
		audit.Revision = 1
	}
	if audit.Changes == nil {
		audit.Changes = map[string]any{"before": nil, "after": consent}
	}
	m.audit = append(m.audit, audit)
	m.outbox = append(m.outbox, outbox)
	return consent, nil
}

// Withdraw records the newest intent for the consent's preference stream, so it
// advances the generation and stamps the withdrawn record with it. Withdrawing
// any record therefore makes "withdrawn" the effective state (safe-deny).
func (m *Memory) Withdraw(ctx context.Context, consent domain.Consent, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	if err := live(ctx); err != nil {
		return domain.Consent{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := consentKey{tenantID: consent.TenantID, consentID: consent.ID}
	current, exists := m.consents[k]
	if !exists {
		return domain.Consent{}, ErrNotFound
	}
	if current.Status == domain.StatusWithdrawn {
		return current, nil
	}
	var before any = current
	generation := m.advanceGenerationLocked(current)
	current = m.consents[k] // advanceGenerationLocked may have invalidated its token
	consent.Version = current.Version + 1
	consent.ConfirmationTokenHash = ""
	consent.PreferenceGeneration = generation
	m.consents[k] = consent
	if audit.ResourceType == "" {
		audit.ResourceType = "consent"
	}
	audit.ConsentID = consent.ID
	audit.Revision = consent.Version
	audit.Changes = map[string]any{"before": before, "after": consent}
	m.audit = append(m.audit, audit)
	m.outbox = append(m.outbox, outbox)
	return consent, nil
}

func (m *Memory) GetReceipt(ctx context.Context, tenantID, id string) (domain.ConsentReceipt, error) {
	if err := live(ctx); err != nil {
		return domain.ConsentReceipt{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	receipt, ok := m.receipts[receiptKey{tenantID: tenantID, receiptID: id}]
	if !ok {
		return domain.ConsentReceipt{}, ErrNotFound
	}
	return receipt, nil
}

func (m *Memory) SetSuppression(ctx context.Context, suppression domain.Suppression, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Suppression, error) {
	if err := live(ctx); err != nil {
		return domain.Suppression{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := suppressionKey{
		tenantID:  suppression.TenantID,
		subjectID: suppression.SubjectID,
		scope:     suppression.Scope,
		purpose:   suppression.Purpose,
		channel:   suppression.Channel,
	}
	var before any
	if current, ok := m.suppressions[k]; ok {
		before = current
		suppression.ID = current.ID
		suppression.Version = current.Version + 1
	} else {
		suppression.Version = 1
	}
	m.suppressions[k] = suppression
	if audit.ResourceType == "" {
		audit.ResourceType = "suppression"
	}
	audit.ConsentID = suppression.ID
	audit.Revision = suppression.Version
	audit.Changes = map[string]any{"before": before, "after": suppression}
	m.audit = append(m.audit, audit)
	m.outbox = append(m.outbox, outbox)
	return suppression, nil
}

func (m *Memory) FindSuppression(ctx context.Context, tenantID, subjectID, purpose, channel string) (domain.Suppression, error) {
	if err := live(ctx); err != nil {
		return domain.Suppression{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var best domain.Suppression
	bestRank := 0
	for _, suppression := range m.suppressions {
		if suppression.TenantID != tenantID || suppression.SubjectID != subjectID || !suppression.Active {
			continue
		}
		rank := 0
		switch suppression.Scope {
		case domain.SuppressionGlobal:
			rank = 3
		case domain.SuppressionChannel:
			if suppression.Channel == channel {
				rank = 2
			}
		case domain.SuppressionPurposeChannel:
			if suppression.Purpose == purpose && suppression.Channel == channel {
				rank = 1
			}
		}
		if rank > bestRank {
			best = suppression
			bestRank = rank
		}
	}
	if bestRank == 0 {
		return domain.Suppression{}, ErrNotFound
	}
	return best, nil
}

// StartDoubleOptIn inserts the pending challenge bound to a freshly advanced
// generation, which atomically supersedes any earlier pending challenge.
func (m *Memory) StartDoubleOptIn(ctx context.Context, consent domain.Consent, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	return m.RecordGoverned(ctx, consent, domain.ConsentReceipt{}, audit, outbox)
}

func (m *Memory) FindPendingByToken(ctx context.Context, tenantID, tokenHash string) (domain.Consent, error) {
	if err := live(ctx); err != nil {
		return domain.Consent{}, err
	}
	if tokenHash == "" {
		return domain.Consent{}, ErrNotFound
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, consent := range m.consents {
		if consent.TenantID == tenantID && consent.Status == domain.StatusPendingConfirmation && consent.ConfirmationTokenHash == tokenHash {
			return consent, nil
		}
	}
	return domain.Consent{}, ErrNotFound
}

// ConfirmDoubleOptIn grants a pending challenge only if, under one critical
// section, it is still pending, unexpired, bound to the current preference
// generation, and the finalizer accepts current policy.
func (m *Memory) ConfirmDoubleOptIn(ctx context.Context, tenantID, tokenHash string, at time.Time, finalize ConfirmFinalizer) (domain.Consent, error) {
	if err := live(ctx); err != nil {
		return domain.Consent{}, err
	}
	if tokenHash == "" || finalize == nil {
		return domain.Consent{}, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var k consentKey
	var consent domain.Consent
	found := false
	for candidateKey, candidate := range m.consents {
		if candidate.TenantID == tenantID && candidate.ConfirmationTokenHash == tokenHash {
			k, consent, found = candidateKey, candidate, true
			break
		}
	}
	if !found {
		return domain.Consent{}, ErrNotFound
	}
	if consent.Status != domain.StatusPendingConfirmation {
		return domain.Consent{}, ErrConflict
	}
	if consent.ConfirmationExpiresAt == nil || !consent.ConfirmationExpiresAt.After(at) {
		return domain.Consent{}, ErrExpired
	}
	current := m.generations[consentPreferenceKey(consent)]
	if consent.PreferenceGeneration == 0 || consent.PreferenceGeneration != current {
		return domain.Consent{}, ErrStaleGeneration
	}
	receipt, audit, outbox, err := finalize(ctx, memoryPolicyReader{m: m}, consent, at)
	if err != nil {
		return domain.Consent{}, err
	}
	if receipt.ID == "" || receipt.TenantID != tenantID || receipt.ConsentID != consent.ID {
		return domain.Consent{}, ErrConflict
	}
	if _, exists := m.receipts[receiptKey{tenantID: tenantID, receiptID: receipt.ID}]; exists {
		return domain.Consent{}, ErrConflict
	}
	// Last cancellation point: a caller that left during finalization gets no
	// grant, mirroring a database transaction that never commits.
	if err := live(ctx); err != nil {
		return domain.Consent{}, err
	}
	before := consent
	consent.Status = domain.StatusGranted
	consent.ConfirmedAt = &at
	consent.GrantedAt = &at
	consent.UpdatedAt = at
	consent.Version++
	consent.ConfirmationTokenHash = ""
	consent.ReceiptID = receipt.ID
	m.consents[k] = consent
	m.receipts[receiptKey{tenantID: tenantID, receiptID: receipt.ID}] = receipt
	if audit.ResourceType == "" {
		audit.ResourceType = "consent"
	}
	audit.ConsentID = consent.ID
	audit.Revision = consent.Version
	audit.Changes = map[string]any{"before": before, "after": consent}
	m.audit = append(m.audit, audit)
	m.outbox = append(m.outbox, outbox)
	return consent, nil
}
func (m *Memory) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	if err := live(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.ResourceType == "" {
		e.ResourceType = "consent"
	}
	if e.Revision <= 0 {
		e.Revision = 1
	}
	if e.Changes == nil {
		e.Changes = map[string]any{}
	}
	m.audit = append(m.audit, e)
	return nil
}
func (m *Memory) AppendOutbox(ctx context.Context, e domain.OutboxEvent) error {
	if err := live(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outbox = append(m.outbox, e)
	return nil
}
func (m *Memory) Ping(ctx context.Context) error { return live(ctx) }
func (m *Memory) Audit() []domain.AuditEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.AuditEvent(nil), m.audit...)
}
func (m *Memory) Outbox() []domain.OutboxEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.OutboxEvent(nil), m.outbox...)
}

// OutboxLeaseState is the lease/publication state the memory store tracks
// per outbox event, mirroring the consent_outbox_events lease columns.
type OutboxLeaseState struct {
	PublishedAt   *time.Time
	Attempts      int
	NextAttemptAt time.Time
	LockedAt      *time.Time
	LockedBy      string
	ClaimToken    string
}

// SetClock replaces the store clock used for outbox lease timing (tests).
func (m *Memory) SetClock(clock func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clock = clock
}

// OutboxLease returns a copy of the lease state of outbox event id.
func (m *Memory) OutboxLease(id string) (OutboxLeaseState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.outboxLeases[id]
	if !ok {
		for _, event := range m.outbox {
			if event.ID == id {
				return OutboxLeaseState{}, true
			}
		}
		return OutboxLeaseState{}, false
	}
	return *state, true
}

func (m *Memory) outboxLeaseLocked(id string) *OutboxLeaseState {
	state, ok := m.outboxLeases[id]
	if !ok {
		state = &OutboxLeaseState{}
		m.outboxLeases[id] = state
	}
	return state
}

func (m *Memory) ClaimOutbox(ctx context.Context, workerID string, limit int, tenantFilter string) (OutboxClaim, error) {
	claim := OutboxClaim{Lease: OutboxLeaseDuration}
	if err := ctx.Err(); err != nil {
		return OutboxClaim{}, err
	}
	if limit <= 0 {
		return claim, nil
	}
	token, err := newClaimToken()
	if err != nil {
		return OutboxClaim{}, err
	}
	claim.Token = token
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock()
	candidates := make([]domain.OutboxEvent, 0, len(m.outbox))
	for _, event := range m.outbox {
		if tenantFilter != "" && event.TenantID != tenantFilter {
			continue
		}
		state := m.outboxLeases[event.ID]
		if state != nil && (state.PublishedAt != nil || state.NextAttemptAt.After(now) ||
			(state.LockedAt != nil && !state.LockedAt.Before(now.Add(-OutboxLeaseDuration)))) {
			continue
		}
		candidates = append(candidates, event)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if !candidates[i].OccurredAt.Equal(candidates[j].OccurredAt) {
			return candidates[i].OccurredAt.Before(candidates[j].OccurredAt)
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	for _, event := range candidates {
		event = normalizeOutbox(event)
		payload, _ := json.Marshal(event.Payload)
		state := m.outboxLeaseLocked(event.ID)
		claim.Events = append(claim.Events, OutboxRecord{
			ID: event.ID, SchemaVersion: event.SchemaVersion, Source: event.Source, TenantID: event.TenantID,
			EventType: event.EventType, AggregateType: event.AggregateType, AggregateID: event.AggregateID,
			Payload: payload, OccurredAt: event.OccurredAt, Attempts: state.Attempts,
		})
		lockedAt := now
		state.LockedAt, state.LockedBy, state.ClaimToken = &lockedAt, workerID, token
		state.Attempts++
	}
	return claim, nil
}

// fencedOutboxStateLocked returns the row state only while token owns it.
func (m *Memory) fencedOutboxStateLocked(id, token string) (*OutboxLeaseState, error) {
	state, ok := m.outboxLeases[id]
	if id == "" || token == "" || !ok || state.PublishedAt != nil || state.ClaimToken != token {
		return nil, ErrLeaseLost
	}
	return state, nil
}

func (m *Memory) CompleteOutbox(ctx context.Context, id, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.fencedOutboxStateLocked(id, token)
	if err != nil {
		return err
	}
	now := m.clock()
	state.PublishedAt, state.LockedAt, state.LockedBy, state.ClaimToken = &now, nil, "", ""
	return nil
}

func (m *Memory) RetryOutbox(ctx context.Context, id, token string, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.fencedOutboxStateLocked(id, token)
	if err != nil {
		return err
	}
	state.LockedAt, state.LockedBy, state.ClaimToken = nil, "", ""
	state.NextAttemptAt = m.clock().Add(max(delay, 0))
	return nil
}

func (m *Memory) ReleaseOutbox(ctx context.Context, id, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.fencedOutboxStateLocked(id, token)
	if err != nil {
		return err
	}
	state.LockedAt, state.LockedBy, state.ClaimToken = nil, "", ""
	state.Attempts = max(state.Attempts-1, 0)
	return nil
}
