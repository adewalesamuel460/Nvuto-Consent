package store

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
)

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

type idempotencyKey struct {
	tenantID       string
	idempotencyKey string
}

type memoryIdempotencyEntry struct {
	RequestHash      string
	ConsentID        string
	ResponseSnapshot domain.Consent
	OriginalResponse domain.Consent
}

func cloneConsent(c domain.Consent) domain.Consent {
	cp := c
	if c.Scope != nil {
		cp.Scope = make(map[string]string, len(c.Scope))
		for k, v := range c.Scope {
			cp.Scope[k] = v
		}
	}
	if c.GrantedAt != nil {
		t := *c.GrantedAt
		cp.GrantedAt = &t
	}
	if c.DeniedAt != nil {
		t := *c.DeniedAt
		cp.DeniedAt = &t
	}
	if c.WithdrawnAt != nil {
		t := *c.WithdrawnAt
		cp.WithdrawnAt = &t
	}
	if c.ExpiresAt != nil {
		t := *c.ExpiresAt
		cp.ExpiresAt = &t
	}
	if c.ConfirmationExpiresAt != nil {
		t := *c.ConfirmationExpiresAt
		cp.ConfirmationExpiresAt = &t
	}
	if c.ConfirmedAt != nil {
		t := *c.ConfirmedAt
		cp.ConfirmedAt = &t
	}
	return cp
}

func clonePurpose(p domain.Purpose) domain.Purpose {
	cp := p
	if p.AllowedChannels != nil {
		cp.AllowedChannels = make([]string, len(p.AllowedChannels))
		copy(cp.AllowedChannels, p.AllowedChannels)
	}
	return cp
}

func cloneLawfulBasis(b domain.LawfulBasis) domain.LawfulBasis {
	cp := b
	if b.AllowedCategories != nil {
		cp.AllowedCategories = make([]string, len(b.AllowedCategories))
		copy(cp.AllowedCategories, b.AllowedCategories)
	}
	return cp
}

func cloneSuppression(s domain.Suppression) domain.Suppression {
	return s
}

func cloneAuditEvent(e domain.AuditEvent) domain.AuditEvent {
	cp := e
	if e.Changes != nil {
		cp.Changes = cloneMapAny(e.Changes)
	}
	if e.BeforeState != nil {
		cp.BeforeState = cloneMapAny(e.BeforeState)
	}
	if e.AfterState != nil {
		cp.AfterState = cloneMapAny(e.AfterState)
	}
	return cp
}

func cloneOutboxEvent(e domain.OutboxEvent) domain.OutboxEvent {
	cp := e
	if e.Payload != nil {
		cp.Payload = cloneMapAny(e.Payload)
	}
	return cp
}

func cloneMapAny(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	cp := make(map[string]any, len(m))
	for k, v := range m {
		if subMap, ok := v.(map[string]any); ok {
			cp[k] = cloneMapAny(subMap)
		} else if subSlice, ok := v.([]string); ok {
			s := make([]string, len(subSlice))
			copy(s, subSlice)
			cp[k] = s
		} else {
			cp[k] = v
		}
	}
	return cp
}

type Memory struct {
	mu                sync.RWMutex
	consents          map[consentKey]domain.Consent
	audit             []domain.AuditEvent
	outbox            []domain.OutboxEvent
	purposes          map[purposeKey]domain.Purpose
	bases             map[lawfulBasisKey]domain.LawfulBasis
	notices           map[noticeKey]domain.PrivacyNotice
	receipts          map[receiptKey]domain.ConsentReceipt
	suppressions      map[suppressionKey]domain.Suppression
	generations       map[preferenceKey]int64 // current preference generation by preferenceKey
	idempotencyLedger map[idempotencyKey]memoryIdempotencyEntry
	outboxLeases      map[string]*OutboxLeaseState
	clock             func() time.Time
}

func NewMemory() *Memory {
	return &Memory{
		consents:          map[consentKey]domain.Consent{},
		purposes:          map[purposeKey]domain.Purpose{},
		bases:             map[lawfulBasisKey]domain.LawfulBasis{},
		notices:           map[noticeKey]domain.PrivacyNotice{},
		receipts:          map[receiptKey]domain.ConsentReceipt{},
		suppressions:      map[suppressionKey]domain.Suppression{},
		generations:       map[preferenceKey]int64{},
		idempotencyLedger: map[idempotencyKey]memoryIdempotencyEntry{},
		outboxLeases:      map[string]*OutboxLeaseState{},
		clock:             time.Now,
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
	return cloneConsent(c), nil
}

// SeedConsent inserts or overwrites a consent record directly for deterministic testing.
func (m *Memory) SeedConsent(c domain.Consent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consents[consentKey{tenantID: c.TenantID, consentID: c.ID}] = cloneConsent(c)
}

func consentIsBetter(c, best domain.Consent) bool {
	if c.PreferenceGeneration != best.PreferenceGeneration {
		return c.PreferenceGeneration > best.PreferenceGeneration
	}
	if !c.UpdatedAt.Equal(best.UpdatedAt) {
		return c.UpdatedAt.After(best.UpdatedAt)
	}
	return c.ID > best.ID
}

// FindEffective returns the record carrying the highest preference generation;
// updated_at and id break ties (pre-generation records all have generation 0).
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
		if !found || consentIsBetter(c, best) {
			best = c
			found = true
		}
	}
	if !found {
		return domain.Consent{}, ErrNotFound
	}
	return cloneConsent(best), nil
}

func (m *Memory) ListConsents(ctx context.Context, tenantID, subjectID string, limit, offset int) ([]domain.Consent, int, error) {
	if err := live(ctx); err != nil {
		return nil, 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var matches []domain.Consent
	for _, c := range m.consents {
		if c.TenantID == tenantID && c.SubjectID == subjectID {
			matches = append(matches, c)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].PreferenceGeneration != matches[j].PreferenceGeneration {
			return matches[i].PreferenceGeneration > matches[j].PreferenceGeneration
		}
		if !matches[i].UpdatedAt.Equal(matches[j].UpdatedAt) {
			return matches[i].UpdatedAt.After(matches[j].UpdatedAt)
		}
		return matches[i].ID > matches[j].ID
	})
	total := len(matches)
	if offset >= total {
		return []domain.Consent{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	res := make([]domain.Consent, end-offset)
	for i, c := range matches[offset:end] {
		res[i] = cloneConsent(c)
	}
	return res, total, nil
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
		before = clonePurpose(current)
		purpose.Version = current.Version + 1
	} else {
		purpose.Version = 1
	}
	clonedPurpose := clonePurpose(purpose)
	m.purposes[k] = clonedPurpose
	if audit.ResourceType == "" {
		audit.ResourceType = "purpose"
	}
	if audit.ConsentID == "" {
		audit.ConsentID = purpose.Code
	}
	if audit.ResourceID == "" {
		audit.ResourceID = audit.ConsentID
	}
	audit.Revision = purpose.Version
	if audit.CorrelationID == "" {
		audit.CorrelationID = domain.CorrelationID(ctx)
	}
	if audit.BeforeState == nil && before != nil {
		audit.BeforeState = toMap(before)
	}
	if audit.AfterState == nil {
		audit.AfterState = toMap(clonedPurpose)
	}
	audit.Changes = map[string]any{"before": before, "after": clonedPurpose}
	m.audit = append(m.audit, cloneAuditEvent(audit))
	m.outbox = append(m.outbox, cloneOutboxEvent(outbox))
	return clonePurpose(purpose), nil
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
	return clonePurpose(purpose), nil
}

func (m *Memory) ListPurposes(ctx context.Context, tenantID string, limit, offset int) ([]domain.Purpose, int, error) {
	if err := live(ctx); err != nil {
		return nil, 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var matches []domain.Purpose
	for _, p := range m.purposes {
		if p.TenantID == tenantID {
			matches = append(matches, p)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Code < matches[j].Code
	})
	total := len(matches)
	if offset >= total {
		return []domain.Purpose{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	res := make([]domain.Purpose, end-offset)
	for i, p := range matches[offset:end] {
		res[i] = clonePurpose(p)
	}
	return res, total, nil
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
		before = cloneLawfulBasis(current)
		basis.Version = current.Version + 1
	} else {
		basis.Version = 1
	}
	clonedBasis := cloneLawfulBasis(basis)
	m.bases[k] = clonedBasis
	if audit.ResourceType == "" {
		audit.ResourceType = "lawful_basis"
	}
	if audit.ConsentID == "" {
		audit.ConsentID = basis.Code
	}
	if audit.ResourceID == "" {
		audit.ResourceID = audit.ConsentID
	}
	audit.Revision = basis.Version
	if audit.CorrelationID == "" {
		audit.CorrelationID = domain.CorrelationID(ctx)
	}
	if audit.BeforeState == nil && before != nil {
		audit.BeforeState = toMap(before)
	}
	if audit.AfterState == nil {
		audit.AfterState = toMap(clonedBasis)
	}
	audit.Changes = map[string]any{"before": before, "after": clonedBasis}
	m.audit = append(m.audit, cloneAuditEvent(audit))
	m.outbox = append(m.outbox, cloneOutboxEvent(outbox))
	return cloneLawfulBasis(basis), nil
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
	return cloneLawfulBasis(basis), nil
}

func (m *Memory) ListLawfulBases(ctx context.Context, tenantID string, limit, offset int) ([]domain.LawfulBasis, int, error) {
	if err := live(ctx); err != nil {
		return nil, 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var matches []domain.LawfulBasis
	for _, b := range m.bases {
		if b.TenantID == tenantID {
			matches = append(matches, b)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Code < matches[j].Code
	})
	total := len(matches)
	if offset >= total {
		return []domain.LawfulBasis{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	res := make([]domain.LawfulBasis, end-offset)
	for i, b := range matches[offset:end] {
		res[i] = cloneLawfulBasis(b)
	}
	return res, total, nil
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
	if audit.ResourceID == "" {
		audit.ResourceID = audit.ConsentID
	}
	if audit.Revision <= 0 {
		audit.Revision = 1
	}
	if audit.CorrelationID == "" {
		audit.CorrelationID = domain.CorrelationID(ctx)
	}
	if audit.AfterState == nil {
		audit.AfterState = toMap(notice)
	}
	audit.Changes = map[string]any{"before": nil, "after": notice}
	m.audit = append(m.audit, cloneAuditEvent(audit))
	m.outbox = append(m.outbox, cloneOutboxEvent(outbox))
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

func (m *Memory) ListNotices(ctx context.Context, tenantID string, limit, offset int) ([]domain.PrivacyNotice, int, error) {
	if err := live(ctx); err != nil {
		return nil, 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var matches []domain.PrivacyNotice
	for _, n := range m.notices {
		if n.TenantID == tenantID {
			matches = append(matches, n)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if !matches[i].PublishedAt.Equal(matches[j].PublishedAt) {
			return matches[i].PublishedAt.After(matches[j].PublishedAt)
		}
		if matches[i].Jurisdiction != matches[j].Jurisdiction {
			return matches[i].Jurisdiction < matches[j].Jurisdiction
		}
		return matches[i].Version > matches[j].Version
	})
	total := len(matches)
	if offset >= total {
		return []domain.PrivacyNotice{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	res := make([]domain.PrivacyNotice, end-offset)
	copy(res, matches[offset:end])
	return res, total, nil
}

func (m *Memory) RetireNotice(ctx context.Context, tenantID, jurisdiction, version, supersededBy string, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.PrivacyNotice, error) {
	if err := live(ctx); err != nil {
		return domain.PrivacyNotice{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := noticeKey{tenantID: tenantID, jurisdiction: jurisdiction, version: version}
	current, ok := m.notices[k]
	if !ok {
		return domain.PrivacyNotice{}, ErrNotFound
	}
	before := current
	current.Active = false
	current.Retired = true
	current.SupersededBy = supersededBy
	m.notices[k] = current

	if audit.ResourceType == "" {
		audit.ResourceType = "privacy_notice"
	}
	if audit.ConsentID == "" {
		audit.ConsentID = current.Jurisdiction + "/" + current.Version
	}
	if audit.ResourceID == "" {
		audit.ResourceID = audit.ConsentID
	}
	if audit.Revision <= 0 {
		audit.Revision = 2
	}
	if audit.CorrelationID == "" {
		audit.CorrelationID = domain.CorrelationID(ctx)
	}
	if audit.BeforeState == nil {
		audit.BeforeState = toMap(before)
	}
	if audit.AfterState == nil {
		audit.AfterState = toMap(current)
	}
	audit.Changes = map[string]any{"before": before, "after": current}
	m.audit = append(m.audit, cloneAuditEvent(audit))
	m.outbox = append(m.outbox, cloneOutboxEvent(outbox))
	return current, nil
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
	clonedConsent := cloneConsent(consent)
	m.consents[cKey] = clonedConsent
	if audit.ResourceType == "" {
		audit.ResourceType = "consent"
	}
	if audit.ResourceID == "" {
		audit.ResourceID = consent.ID
	}
	if audit.Revision <= 0 {
		audit.Revision = 1
	}
	if audit.CorrelationID == "" {
		audit.CorrelationID = domain.CorrelationID(ctx)
	}
	if audit.AfterState == nil {
		audit.AfterState = toMap(clonedConsent)
	}
	if audit.Changes == nil {
		audit.Changes = map[string]any{"before": nil, "after": clonedConsent}
	}
	if outbox.Payload != nil {
		outbox.Payload["generation"] = consent.PreferenceGeneration
		outbox.Payload["preference_generation"] = consent.PreferenceGeneration
		outbox.Payload["aggregate_revision"] = consent.PreferenceGeneration
	}
	outbox.AggregateRevision = consent.PreferenceGeneration
	m.audit = append(m.audit, cloneAuditEvent(audit))
	m.outbox = append(m.outbox, cloneOutboxEvent(outbox))
	return cloneConsent(consent), nil
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
		return cloneConsent(current), nil
	}
	before := cloneConsent(current)
	generation := m.advanceGenerationLocked(current)
	current = m.consents[k] // advanceGenerationLocked may have invalidated its token
	consent.Version = current.Version + 1
	consent.ConfirmationTokenHash = ""
	consent.PreferenceGeneration = generation
	clonedConsent := cloneConsent(consent)
	m.consents[k] = clonedConsent
	if audit.ResourceType == "" {
		audit.ResourceType = "consent"
	}
	audit.ConsentID = consent.ID
	audit.ResourceID = consent.ID
	audit.Revision = consent.Version
	if audit.CorrelationID == "" {
		audit.CorrelationID = domain.CorrelationID(ctx)
	}
	if audit.BeforeState == nil {
		audit.BeforeState = toMap(before)
	}
	if audit.AfterState == nil {
		audit.AfterState = toMap(clonedConsent)
	}
	audit.Changes = map[string]any{"before": before, "after": clonedConsent}
	if outbox.Payload != nil {
		outbox.Payload["generation"] = generation
		outbox.Payload["preference_generation"] = generation
		outbox.Payload["aggregate_revision"] = generation
		outbox.Payload["schema_version"] = 1
		outbox.Payload["aggregate_type"] = "consent.preference"
		outbox.Payload["tenant_id"] = consent.TenantID
	}
	outbox.AggregateType = "consent.preference"
	outbox.AggregateRevision = generation
	m.audit = append(m.audit, cloneAuditEvent(audit))
	m.outbox = append(m.outbox, cloneOutboxEvent(outbox))
	return cloneConsent(consent), nil
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
		before = cloneSuppression(current)
		// T-030: Protect administrative suppressions from revocation by self_service.
		if current.Active && !suppression.Active && domain.IsAdministrativeRole(current.CreatedByRole) && !domain.HasAdminAuthority(suppression.CreatedByRole) {
			return domain.Suppression{}, domain.ErrSuppressionProtected
		}
		// T-026: Preserve the old ID and CreatedByRole matching durable postgres store.
		suppression.ID = current.ID
		if current.CreatedByRole != "" {
			suppression.CreatedByRole = current.CreatedByRole
		}
		suppression.Version = current.Version + 1
	} else {
		suppression.Version = 1
	}
	clonedSuppression := cloneSuppression(suppression)
	m.suppressions[k] = clonedSuppression
	if audit.ResourceType == "" {
		audit.ResourceType = "suppression"
	}
	audit.ConsentID = suppression.ID
	audit.ResourceID = suppression.ID
	audit.Revision = suppression.Version
	if audit.Reason == "" {
		audit.Reason = suppression.Reason
	}
	if audit.CorrelationID == "" {
		audit.CorrelationID = domain.CorrelationID(ctx)
	}
	if audit.BeforeState == nil && before != nil {
		audit.BeforeState = toMap(before)
	}
	if audit.AfterState == nil {
		audit.AfterState = toMap(clonedSuppression)
	}
	audit.Changes = map[string]any{"before": before, "after": clonedSuppression}
	// T-026: Ensure outbox AggregateID references the preserved suppression ID.
	outbox.AggregateID = suppression.ID
	outbox.AggregateType = "consent.suppression"
	outbox.AggregateRevision = suppression.Version
	if outbox.Payload != nil {
		clonedPayload := cloneMapAny(outbox.Payload)
		if _, hasRole := clonedPayload["created_by_role"]; hasRole {
			clonedPayload["created_by_role"] = suppression.CreatedByRole
		}
		clonedPayload["schema_version"] = 1
		clonedPayload["aggregate_type"] = "consent.suppression"
		clonedPayload["aggregate_revision"] = suppression.Version
		clonedPayload["tenant_id"] = suppression.TenantID
		clonedPayload["subject_id"] = suppression.SubjectID
		clonedPayload["version"] = suppression.Version
		outbox.Payload = clonedPayload
	}
	m.audit = append(m.audit, cloneAuditEvent(audit))
	m.outbox = append(m.outbox, cloneOutboxEvent(outbox))
	return cloneSuppression(suppression), nil
}

func suppressionIsBetter(s, best domain.Suppression, rank, bestRank int) bool {
	if rank != bestRank {
		return rank > bestRank
	}
	if !s.UpdatedAt.Equal(best.UpdatedAt) {
		return s.UpdatedAt.After(best.UpdatedAt)
	}
	return s.ID > best.ID
}

func (m *Memory) FindSuppression(ctx context.Context, tenantID, subjectID, purpose, channel string) (domain.Suppression, error) {
	if err := live(ctx); err != nil {
		return domain.Suppression{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var best domain.Suppression
	bestRank := 0
	found := false
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
		if rank > 0 && (!found || suppressionIsBetter(suppression, best, rank, bestRank)) {
			best = suppression
			bestRank = rank
			found = true
		}
	}
	if !found {
		return domain.Suppression{}, ErrNotFound
	}
	return cloneSuppression(best), nil
}

func (m *Memory) ListSuppressions(ctx context.Context, tenantID, subjectID string, limit, offset int) ([]domain.Suppression, int, error) {
	if err := live(ctx); err != nil {
		return nil, 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var matches []domain.Suppression
	for _, s := range m.suppressions {
		if s.TenantID == tenantID && s.SubjectID == subjectID {
			matches = append(matches, s)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if !matches[i].UpdatedAt.Equal(matches[j].UpdatedAt) {
			return matches[i].UpdatedAt.After(matches[j].UpdatedAt)
		}
		return matches[i].ID > matches[j].ID
	})
	total := len(matches)
	if offset >= total {
		return []domain.Suppression{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	res := make([]domain.Suppression, end-offset)
	for i, s := range matches[offset:end] {
		res[i] = cloneSuppression(s)
	}
	return res, total, nil
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
			return cloneConsent(consent), nil
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
	before := cloneConsent(consent)
	consent.Status = domain.StatusGranted
	consent.ConfirmedAt = &at
	consent.GrantedAt = &at
	consent.UpdatedAt = at
	consent.Version++
	consent.ConfirmationTokenHash = ""
	consent.ReceiptID = receipt.ID
	clonedConsent := cloneConsent(consent)
	m.consents[k] = clonedConsent
	m.receipts[receiptKey{tenantID: tenantID, receiptID: receipt.ID}] = receipt
	if audit.ResourceType == "" {
		audit.ResourceType = "consent"
	}
	audit.ConsentID = consent.ID
	audit.ResourceID = consent.ID
	audit.Revision = consent.Version
	if audit.CorrelationID == "" {
		audit.CorrelationID = domain.CorrelationID(ctx)
	}
	if audit.BeforeState == nil {
		audit.BeforeState = toMap(before)
	}
	if audit.AfterState == nil {
		audit.AfterState = toMap(clonedConsent)
	}
	audit.Changes = map[string]any{"before": before, "after": clonedConsent}
	if outbox.Payload != nil {
		outbox.Payload["generation"] = consent.PreferenceGeneration
		outbox.Payload["preference_generation"] = consent.PreferenceGeneration
		outbox.Payload["aggregate_revision"] = consent.PreferenceGeneration
		outbox.Payload["schema_version"] = 1
		outbox.Payload["aggregate_type"] = "consent.preference"
		outbox.Payload["tenant_id"] = consent.TenantID
	}
	outbox.AggregateType = "consent.preference"
	outbox.AggregateRevision = consent.PreferenceGeneration
	m.audit = append(m.audit, cloneAuditEvent(audit))
	m.outbox = append(m.outbox, cloneOutboxEvent(outbox))
	return cloneConsent(consent), nil
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
	if e.ResourceID == "" {
		e.ResourceID = e.ConsentID
	}
	if e.Revision <= 0 {
		e.Revision = 1
	}
	if e.CorrelationID == "" {
		e.CorrelationID = domain.CorrelationID(ctx)
	}
	if e.BeforeState == nil {
		e.BeforeState = map[string]any{}
	}
	if e.AfterState == nil {
		e.AfterState = map[string]any{}
	}
	if e.Changes == nil {
		e.Changes = map[string]any{"before": e.BeforeState, "after": e.AfterState}
	}
	m.audit = append(m.audit, cloneAuditEvent(e))
	return nil
}

func (m *Memory) ListAuditEvents(ctx context.Context, tenantID, resourceType, resourceID string, limit, offset int) ([]domain.AuditEvent, int, error) {
	if err := live(ctx); err != nil {
		return nil, 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matched []domain.AuditEvent
	for _, a := range m.audit {
		if a.TenantID != tenantID {
			continue
		}
		if resourceType != "" && a.ResourceType != resourceType {
			continue
		}
		if resourceID != "" {
			targetID := a.ResourceID
			if targetID == "" {
				targetID = a.ConsentID
			}
			if targetID != resourceID && a.ConsentID != resourceID {
				continue
			}
		}
		matched = append(matched, cloneAuditEvent(a))
	}

	total := len(matched)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if offset >= total {
		return []domain.AuditEvent{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matched[offset:end], total, nil
}

func (m *Memory) AppendOutbox(ctx context.Context, e domain.OutboxEvent) error {
	if err := live(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outbox = append(m.outbox, cloneOutboxEvent(e))
	return nil
}

func (m *Memory) Ping(ctx context.Context) error { return live(ctx) }

func (m *Memory) VerifySchema(ctx context.Context) error { return live(ctx) }

func (m *Memory) Audit() []domain.AuditEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]domain.AuditEvent, len(m.audit))
	for i, a := range m.audit {
		res[i] = cloneAuditEvent(a)
	}
	return res
}

func (m *Memory) Outbox() []domain.OutboxEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]domain.OutboxEvent, len(m.outbox))
	for i, o := range m.outbox {
		res[i] = cloneOutboxEvent(o)
	}
	return res
}

// OutboxLeaseState is the lease/publication state the memory store tracks
// per outbox event, mirroring the consent_outbox_events lease columns.
type OutboxLeaseState struct {
	PublishedAt      *time.Time
	Attempts         int
	NextAttemptAt    time.Time
	LockedAt         *time.Time
	LockedBy         string
	ClaimToken       string
	Quarantined      bool
	QuarantinedAt    *time.Time
	QuarantineReason string
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
		if state != nil && (state.PublishedAt != nil || state.Quarantined || state.NextAttemptAt.After(now) ||
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
		rev := event.AggregateRevision
		if rev <= 0 && event.Payload != nil {
			if r, ok := event.Payload["aggregate_revision"].(int64); ok && r > 0 {
				rev = r
			} else if r, ok := event.Payload["preference_generation"].(int64); ok && r > 0 {
				rev = r
			}
		}
		if rev <= 0 {
			rev = 1
		}
		claim.Events = append(claim.Events, OutboxRecord{
			ID: event.ID, SchemaVersion: event.SchemaVersion, Source: event.Source, TenantID: event.TenantID,
			EventType: event.EventType, AggregateType: event.AggregateType, AggregateID: event.AggregateID,
			AggregateRevision: rev, Payload: payload, OccurredAt: event.OccurredAt, Attempts: state.Attempts,
			Quarantined: state.Quarantined, QuarantinedAt: state.QuarantinedAt, QuarantineReason: state.QuarantineReason,
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

func (m *Memory) RescheduleOutbox(ctx context.Context, id, token string, delay time.Duration) error {
	return m.RetryOutbox(ctx, id, token, delay)
}

func (m *Memory) ReleaseOutboxBatch(ctx context.Context, workerID string, events []OutboxRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range events {
		state, ok := m.outboxLeases[item.ID]
		if ok && state.PublishedAt == nil && (workerID == "" || state.LockedBy == workerID) {
			state.LockedAt, state.LockedBy, state.ClaimToken = nil, "", ""
			state.Attempts = max(state.Attempts-1, 0)
		}
	}
	return nil
}

func (m *Memory) QuarantineOutbox(ctx context.Context, id, token string, reason string) error {
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
	state.Quarantined = true
	state.QuarantinedAt = &now
	state.QuarantineReason = reason
	state.LockedAt, state.LockedBy, state.ClaimToken = nil, "", ""
	return nil
}

func (m *Memory) ReplayOutbox(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.outboxLeases[id]
	if !ok {
		found := false
		for _, e := range m.outbox {
			if e.ID == id {
				found = true
				break
			}
		}
		if !found {
			return ErrNotFound
		}
		return nil
	}
	if !state.Quarantined {
		return nil
	}
	now := m.clock()
	state.Quarantined = false
	state.QuarantinedAt = nil
	state.QuarantineReason = ""
	state.Attempts = 0
	state.NextAttemptAt = now
	state.LockedAt = nil
	state.LockedBy = ""
	state.ClaimToken = ""
	return nil
}

func (m *Memory) OutboxLag(ctx context.Context, tenantFilter string) (time.Duration, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := m.clock()
	var oldest time.Time
	for _, event := range m.outbox {
		if tenantFilter != "" && event.TenantID != tenantFilter {
			continue
		}
		state := m.outboxLeases[event.ID]
		if state != nil && (state.PublishedAt != nil || state.Quarantined) {
			continue
		}
		if oldest.IsZero() || event.OccurredAt.Before(oldest) {
			oldest = event.OccurredAt
		}
	}
	if oldest.IsZero() {
		return 0, nil
	}
	lag := now.Sub(oldest)
	if lag < 0 {
		return 0, nil
	}
	return lag, nil
}

func (m *Memory) QuarantinedOutboxCount(ctx context.Context, tenantFilter string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var count int64
	for _, event := range m.outbox {
		if tenantFilter != "" && event.TenantID != tenantFilter {
			continue
		}
		state := m.outboxLeases[event.ID]
		if state != nil && state.Quarantined {
			count++
		}
	}
	return count, nil
}
