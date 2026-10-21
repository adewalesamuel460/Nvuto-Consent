package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// ErrValidation matches every input-validation and policy-rejection error.
var (
	ErrValidation                      = domain.ErrValidation
	ErrUnsupportedHistoricalEvaluation = domain.ErrUnsupportedHistoricalEvaluation
)

const maxEvaluationClockSkew = time.Minute

type Service struct{ repo store.Repository }

func New(repo store.Repository) *Service { return &Service{repo: repo} }

// SubjectScope constrains which canonical customer subjects an operation may
// act on. The zero value permits no subject, so a caller that forgets to
// establish a scope fails closed.
type SubjectScope struct {
	tenantWide bool
	subjectID  string
}

// TenantWide permits every subject in the tenant. Callers must already have
// verified tenant-wide authority for the requested action.
func TenantWide() SubjectScope { return SubjectScope{tenantWide: true} }

// OwnSubject permits only the given canonical customer subject. An empty
// subject permits nothing.
func OwnSubject(subjectID string) SubjectScope {
	return SubjectScope{subjectID: strings.TrimSpace(subjectID)}
}

// Permits reports whether the scope covers subjectID.
func (s SubjectScope) Permits(subjectID string) bool {
	if s.tenantWide {
		return true
	}
	subjectID = strings.TrimSpace(subjectID)
	return s.subjectID != "" && subjectID != "" && s.subjectID == subjectID
}

func id(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func includes(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (s *Service) RegisterPurpose(ctx context.Context, tenantID, actorID string, purpose domain.Purpose) (domain.Purpose, error) {
	tenantID, actorID = strings.TrimSpace(tenantID), strings.TrimSpace(actorID)
	purpose.Code = strings.ToLower(strings.TrimSpace(purpose.Code))
	purpose.Category = strings.ToLower(strings.TrimSpace(purpose.Category))
	if err := checkPrincipal(tenantID, actorID); err != nil {
		return domain.Purpose{}, err
	}
	if err := checkCode("code", purpose.Code); err != nil {
		return domain.Purpose{}, err
	}
	if err := checkCode("category", purpose.Category); err != nil {
		return domain.Purpose{}, err
	}
	channels, err := normalizeCodes("allowed_channels", purpose.AllowedChannels)
	if err != nil {
		return domain.Purpose{}, err
	}
	purpose.AllowedChannels = channels
	purpose.TenantID = tenantID
	purpose.UpdatedAt = time.Now().UTC()
	rev := purpose.Version
	if rev <= 0 {
		rev = 1
	}
	var before any
	var beforeState map[string]any
	if existing, err := s.repo.GetPurpose(ctx, tenantID, purpose.Code); err == nil && existing.Code != "" {
		before = existing
		beforeState = toMap(existing)
	}
	afterState := toMap(purpose)
	audit := domain.AuditEvent{
		ID:            id("aud"),
		TenantID:      tenantID,
		ActorID:       actorID,
		Action:        "consent.purpose.registered",
		ConsentID:     purpose.Code,
		ResourceID:    purpose.Code,
		OccurredAt:    purpose.UpdatedAt,
		ResourceType:  "purpose",
		Revision:      rev,
		CorrelationID: domain.CorrelationID(ctx),
		BeforeState:   beforeState,
		AfterState:    afterState,
		Changes:       map[string]any{"before": before, "after": purpose},
	}
	outbox := domain.OutboxEvent{
		ID:                id("evt"),
		SchemaVersion:     1,
		TenantID:          tenantID,
		EventType:         "consent.purpose.changed",
		AggregateType:     "consent.purpose",
		AggregateID:       purpose.Code,
		AggregateRevision: rev,
		Payload: map[string]any{
			"tenant_id":                    tenantID,
			"code":                         purpose.Code,
			"category":                     purpose.Category,
			"active":                       purpose.Active,
			"allowed_channels":             purpose.AllowedChannels,
			"requires_affirmative_consent": purpose.RequiresAffirmativeConsent,
			"requires_double_opt_in":       purpose.RequiresDoubleOptIn,
			"aggregate_type":               "consent.purpose",
			"aggregate_revision":           rev,
			"schema_version":               1,
		},
		OccurredAt: purpose.UpdatedAt,
	}
	return s.repo.PutPurpose(ctx, purpose, audit, outbox)
}

func (s *Service) RegisterLawfulBasis(ctx context.Context, tenantID, actorID string, basis domain.LawfulBasis) (domain.LawfulBasis, error) {
	tenantID, actorID = strings.TrimSpace(tenantID), strings.TrimSpace(actorID)
	basis.Code = strings.ToLower(strings.TrimSpace(basis.Code))
	if err := checkPrincipal(tenantID, actorID); err != nil {
		return domain.LawfulBasis{}, err
	}
	if err := checkCode("code", basis.Code); err != nil {
		return domain.LawfulBasis{}, err
	}
	categories, err := normalizeCodes("allowed_categories", basis.AllowedCategories)
	if err != nil {
		return domain.LawfulBasis{}, err
	}
	basis.AllowedCategories = categories
	basis.TenantID = tenantID
	basis.UpdatedAt = time.Now().UTC()
	basisRev := basis.Version
	if basisRev <= 0 {
		basisRev = 1
	}
	var before any
	var beforeState map[string]any
	if existing, err := s.repo.GetLawfulBasis(ctx, tenantID, basis.Code); err == nil && existing.Code != "" {
		before = existing
		beforeState = toMap(existing)
	}
	afterState := toMap(basis)
	audit := domain.AuditEvent{
		ID:            id("aud"),
		TenantID:      tenantID,
		ActorID:       actorID,
		Action:        "consent.lawful_basis.registered",
		ConsentID:     basis.Code,
		ResourceID:    basis.Code,
		OccurredAt:    basis.UpdatedAt,
		ResourceType:  "lawful_basis",
		Revision:      basisRev,
		CorrelationID: domain.CorrelationID(ctx),
		BeforeState:   beforeState,
		AfterState:    afterState,
		Changes:       map[string]any{"before": before, "after": basis},
	}
	outbox := domain.OutboxEvent{
		ID:                id("evt"),
		SchemaVersion:     1,
		TenantID:          tenantID,
		EventType:         "consent.lawful_basis.changed",
		AggregateType:     "consent.lawful_basis",
		AggregateID:       basis.Code,
		AggregateRevision: basisRev,
		Payload: map[string]any{
			"tenant_id":          tenantID,
			"code":               basis.Code,
			"active":             basis.Active,
			"allowed_categories": basis.AllowedCategories,
			"requires_evidence":  basis.RequiresEvidence,
			"aggregate_type":     "consent.lawful_basis",
			"aggregate_revision": basisRev,
			"schema_version":     1,
		},
		OccurredAt: basis.UpdatedAt,
	}
	return s.repo.PutLawfulBasis(ctx, basis, audit, outbox)
}

func (s *Service) PublishNotice(ctx context.Context, tenantID, actorID string, notice domain.PrivacyNotice) (domain.PrivacyNotice, error) {
	tenantID, actorID = strings.TrimSpace(tenantID), strings.TrimSpace(actorID)
	notice.Version = strings.TrimSpace(notice.Version)
	notice.Jurisdiction = strings.ToUpper(strings.TrimSpace(notice.Jurisdiction))
	if err := checkPrincipal(tenantID, actorID); err != nil {
		return domain.PrivacyNotice{}, err
	}
	if err := checkNoticeVersion("version", notice.Version); err != nil {
		return domain.PrivacyNotice{}, err
	}
	if err := checkJurisdiction("jurisdiction", notice.Jurisdiction); err != nil {
		return domain.PrivacyNotice{}, err
	}
	if notice.PublishedAt.IsZero() {
		return domain.PrivacyNotice{}, invalid("published_at", "is required")
	}
	if notice.PublishedAt.After(time.Now().UTC()) {
		return domain.PrivacyNotice{}, invalid("published_at", "must not be in the future")
	}
	notice.TenantID = tenantID
	if notice.ContentHash == "" {
		if notice.Content != "" {
			digest := sha256.Sum256([]byte(notice.Content))
			notice.ContentHash = "sha256:" + hex.EncodeToString(digest[:])
		} else {
			digest := sha256.Sum256([]byte(notice.TenantID + ":" + notice.Jurisdiction + ":" + notice.Version))
			notice.ContentHash = "sha256:" + hex.EncodeToString(digest[:])
		}
	} else if notice.Content != "" {
		digest := sha256.Sum256([]byte(notice.Content))
		expected := "sha256:" + hex.EncodeToString(digest[:])
		if !hashesEqual(notice.ContentHash, expected) && !hashesEqual(notice.ContentHash, hex.EncodeToString(digest[:])) {
			return domain.PrivacyNotice{}, invalid("content_hash", "does not match sha256 of notice content")
		}
	}
	var beforeState map[string]any
	if existing, err := s.repo.GetNotice(ctx, tenantID, notice.Jurisdiction, notice.Version); err == nil {
		beforeState = toMap(existing)
	}
	afterState := toMap(notice)
	audit := domain.AuditEvent{
		ID:            id("aud"),
		TenantID:      tenantID,
		ActorID:       actorID,
		Action:        "consent.notice.published",
		ConsentID:     notice.Version,
		ResourceID:    notice.Jurisdiction + "/" + notice.Version,
		OccurredAt:    notice.PublishedAt,
		ResourceType:  "privacy_notice",
		Revision:      1,
		CorrelationID: domain.CorrelationID(ctx),
		BeforeState:   beforeState,
		AfterState:    afterState,
		Changes:       map[string]any{"before": beforeState, "after": afterState},
	}
	outbox := domain.OutboxEvent{
		ID:                id("evt"),
		SchemaVersion:     1,
		TenantID:          tenantID,
		EventType:         "consent.notice.published",
		AggregateType:     "consent.notice",
		AggregateID:       notice.Version,
		AggregateRevision: 1,
		Payload: map[string]any{
			"tenant_id":          tenantID,
			"jurisdiction":       notice.Jurisdiction,
			"version":            notice.Version,
			"content_hash":       notice.ContentHash,
			"active":             notice.Active,
			"published_at":       notice.PublishedAt,
			"aggregate_type":     "consent.notice",
			"aggregate_revision": int64(1),
			"schema_version":     1,
		},
		OccurredAt: notice.PublishedAt,
	}
	return s.repo.PutNotice(ctx, notice, audit, outbox)
}

func (s *Service) RetireNotice(ctx context.Context, tenantID, actorID, jurisdiction, version, supersededBy string) (domain.PrivacyNotice, error) {
	tenantID, actorID = strings.TrimSpace(tenantID), strings.TrimSpace(actorID)
	jurisdiction = strings.ToUpper(strings.TrimSpace(jurisdiction))
	version = strings.TrimSpace(version)
	supersededBy = strings.TrimSpace(supersededBy)

	if err := checkPrincipal(tenantID, actorID); err != nil {
		return domain.PrivacyNotice{}, err
	}
	if err := checkJurisdiction("jurisdiction", jurisdiction); err != nil {
		return domain.PrivacyNotice{}, err
	}
	if err := checkNoticeVersion("version", version); err != nil {
		return domain.PrivacyNotice{}, err
	}
	if supersededBy != "" {
		if err := checkNoticeVersion("superseded_by", supersededBy); err != nil {
			return domain.PrivacyNotice{}, err
		}
	}

	existing, _ := s.repo.GetNotice(ctx, tenantID, jurisdiction, version)
	beforeState := toMap(existing)
	retired := existing
	retired.Active = false
	retired.Retired = true
	retired.SupersededBy = supersededBy
	afterState := toMap(retired)

	audit := domain.AuditEvent{
		ID:            id("aud"),
		TenantID:      tenantID,
		ActorID:       actorID,
		Action:        "consent.notice.retired",
		ConsentID:     jurisdiction + "/" + version,
		ResourceID:    jurisdiction + "/" + version,
		OccurredAt:    time.Now().UTC(),
		ResourceType:  "privacy_notice",
		Revision:      2,
		CorrelationID: domain.CorrelationID(ctx),
		Reason:        "superseded_by:" + supersededBy,
		BeforeState:   beforeState,
		AfterState:    afterState,
		Changes:       map[string]any{"before": beforeState, "after": afterState},
	}
	outbox := domain.OutboxEvent{
		ID:                id("evt"),
		SchemaVersion:     1,
		TenantID:          tenantID,
		EventType:         "consent.notice.retired",
		AggregateType:     "consent.notice",
		AggregateID:       version,
		AggregateRevision: 2,
		Payload: map[string]any{
			"tenant_id":          tenantID,
			"jurisdiction":       jurisdiction,
			"version":            version,
			"retired":            true,
			"superseded_by":      supersededBy,
			"aggregate_type":     "consent.notice",
			"aggregate_revision": int64(2),
			"schema_version":     1,
		},
		OccurredAt: time.Now().UTC(),
	}
	return s.repo.RetireNotice(ctx, tenantID, jurisdiction, version, supersededBy, audit, outbox)
}

// policyLookup separates "the policy record does not exist" (a 422 policy
// rejection naming field) from infrastructure failures, which are returned
// unchanged so they surface as 503/500 instead of masquerading as client
// errors (T-013).
func policyLookup(err error, field, message string) error {
	if errors.Is(err, store.ErrNotFound) {
		return rejected(field, message)
	}
	return err
}

// checkPolicy validates a consent against the purpose, lawful basis and notice
// registries as seen through reader at instant now. The confirmation path
// passes a reader bound to its transaction so the policy that is checked is the
// policy in force when the grant commits.
func checkPolicy(ctx context.Context, reader store.PolicyReader, tenantID, purposeCode, channel, basisCode, jurisdiction, noticeVersion string, evidence domain.Evidence, now time.Time) (domain.Purpose, domain.PrivacyNotice, error) {
	purpose, err := reader.GetPurpose(ctx, tenantID, purposeCode)
	if err != nil {
		return domain.Purpose{}, domain.PrivacyNotice{}, policyLookup(err, "purpose", "is not registered for this tenant")
	}
	if !purpose.Active {
		return domain.Purpose{}, domain.PrivacyNotice{}, rejected("purpose", "is not active")
	}
	if !includes(purpose.AllowedChannels, channel) {
		return domain.Purpose{}, domain.PrivacyNotice{}, rejected("channel", "is not allowed for this purpose")
	}
	basis, err := reader.GetLawfulBasis(ctx, tenantID, basisCode)
	if err != nil {
		return domain.Purpose{}, domain.PrivacyNotice{}, policyLookup(err, "legal_basis", "is not registered for this tenant")
	}
	if !basis.Active || !includes(basis.AllowedCategories, purpose.Category) {
		return domain.Purpose{}, domain.PrivacyNotice{}, rejected("legal_basis", "is not active or does not cover the purpose category")
	}
	notice, err := reader.GetNotice(ctx, tenantID, jurisdiction, noticeVersion)
	if err != nil {
		return domain.Purpose{}, domain.PrivacyNotice{}, policyLookup(err, "notice_version", "is not published for this jurisdiction")
	}
	if !notice.Active || notice.Retired || notice.PublishedAt.After(now) {
		return domain.Purpose{}, domain.PrivacyNotice{}, rejected("notice_version", "is not active, retired, or not yet published")
	}
	if basis.RequiresEvidence {
		switch {
		case strings.TrimSpace(evidence.Method) == "" || strings.TrimSpace(evidence.ArtifactHash) == "" || evidence.CollectedAt.IsZero():
			return domain.Purpose{}, domain.PrivacyNotice{}, rejected("evidence", "is required by the lawful basis")
		case evidence.CollectedAt.Before(notice.PublishedAt):
			return domain.Purpose{}, domain.PrivacyNotice{}, rejected("evidence.collected_at", "must not precede the notice publication")
		case evidence.CollectedAt.After(now):
			return domain.Purpose{}, domain.PrivacyNotice{}, rejected("evidence.collected_at", "must not be in the future")
		}
	}
	return purpose, notice, nil
}

// admitGoverned applies the registry policy and the grant rules that depend
// on the purpose. A grant may only be recorded when every rule passes.
// Opt-outs (denials) bypass strict policy availability checks so a subject cannot
// be blocked from opting out or denying consent (T-027).
func (s *Service) admitGoverned(ctx context.Context, in governedInput, now time.Time) (domain.Purpose, domain.PrivacyNotice, error) {
	if in.Status == domain.StatusDenied {
		var notice domain.PrivacyNotice
		if in.NoticeVersion != "" && in.Jurisdiction != "" {
			notice, _ = s.repo.GetNotice(ctx, in.TenantID, in.Jurisdiction, in.NoticeVersion)
		}
		return domain.Purpose{}, notice, nil
	}
	purpose, notice, err := checkPolicy(ctx, s.repo, in.TenantID, in.Purpose, in.Channel, in.LegalBasis, in.Jurisdiction, in.NoticeVersion, in.Evidence, now)
	if err != nil {
		return domain.Purpose{}, domain.PrivacyNotice{}, err
	}
	if in.Status == domain.StatusGranted && purpose.RequiresDoubleOptIn {
		return domain.Purpose{}, domain.PrivacyNotice{}, rejected("status", "this purpose requires double opt-in confirmation")
	}
	if in.Status == domain.StatusGranted && purpose.RequiresAffirmativeConsent && in.LegalBasis != "consent" {
		return domain.Purpose{}, domain.PrivacyNotice{}, rejected("legal_basis", "this purpose requires the consent lawful basis for a grant")
	}
	return purpose, notice, nil
}

// governedRecords builds the consent, receipt, audit and outbox records for a
// validated, admitted request.
func governedRecords(in governedInput, now time.Time, purpose domain.Purpose, notice domain.PrivacyNotice) (domain.Consent, domain.ConsentReceipt, domain.AuditEvent, domain.OutboxEvent, error) {
	consent := domain.Consent{ID: id("con"), TenantID: in.TenantID, SubjectID: in.SubjectID, Purpose: in.Purpose, Channel: in.Channel, LegalBasis: in.LegalBasis, Status: in.Status, Source: in.Source, Scope: in.Scope, Jurisdiction: in.Jurisdiction, NoticeVersion: in.NoticeVersion, Evidence: in.Evidence, Version: 1, ExpiresAt: in.ExpiresAt, CreatedAt: now, UpdatedAt: now}
	if in.Status == domain.StatusGranted {
		consent.GrantedAt = &now
	} else {
		consent.DeniedAt = &now
	}
	meta := ReceiptMetadata{
		ActorID:           in.ActorID,
		ConsentStatus:     in.Status,
		LegalBasis:        in.LegalBasis,
		NoticeContentHash: notice.ContentHash,
		PolicyVersion:     purpose.Version,
	}
	receipt, err := makeReceipt(consent, in.Evidence, now, meta)
	if err != nil {
		return domain.Consent{}, domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, domain.ErrInternal.Wrap(err)
	}
	consent.ReceiptID = receipt.ID
	audit := domain.AuditEvent{
		ID:           id("aud"),
		TenantID:     in.TenantID,
		ActorID:      in.ActorID,
		Action:       "consent." + string(in.Status),
		ConsentID:    consent.ID,
		ResourceID:   consent.ID,
		OccurredAt:   now,
		ResourceType: "consent",
		Revision:     1,
		AfterState:   toMap(consent),
		Changes:      map[string]any{"before": nil, "after": consent},
	}
	outbox := domain.OutboxEvent{
		ID:                id("evt"),
		SchemaVersion:     1,
		TenantID:          in.TenantID,
		EventType:         "consent." + string(in.Status),
		AggregateType:     "consent.preference",
		AggregateID:       consent.ID,
		AggregateRevision: 1,
		Payload: map[string]any{
			"tenant_id":             in.TenantID,
			"subject_id":            in.SubjectID,
			"purpose":               in.Purpose,
			"channel":               in.Channel,
			"status":                string(in.Status),
			"legal_basis":           in.LegalBasis,
			"receipt_id":            receipt.ID,
			"notice_version":        in.NoticeVersion,
			"preference_generation": int64(1),
			"aggregate_type":        "consent.preference",
			"aggregate_revision":    int64(1),
			"schema_version":        1,
		},
		OccurredAt: now,
	}
	return consent, receipt, audit, outbox, nil
}

func (s *Service) RecordGoverned(ctx context.Context, tenantID, actorID, subjectID, purposeCode, channel, legalBasis, status, source string, scope map[string]string, expiresAt *time.Time, jurisdiction, noticeVersion string, evidence domain.Evidence) (domain.Consent, error) {
	checkedAt := time.Now().UTC()
	in, err := normalizeGoverned(tenantID, actorID, subjectID, purposeCode, channel, legalBasis, status, source, scope, expiresAt, jurisdiction, noticeVersion, evidence, checkedAt)
	if err != nil {
		return domain.Consent{}, err
	}
	purpose, notice, err := s.admitGoverned(ctx, in, checkedAt)
	if err != nil {
		return domain.Consent{}, err
	}
	consent, receipt, audit, outbox, err := governedRecords(in, normalizeReceiptTime(time.Now()), purpose, notice)
	if err != nil {
		return domain.Consent{}, err
	}
	return s.repo.RecordGoverned(ctx, consent, receipt, audit, outbox)
}

// Withdraw requires no policy lookup, so the opt-out remains available while
// the purpose, lawful basis or notice registries are retired or unavailable.
func (s *Service) Withdraw(ctx context.Context, tenantID, actorID, consentID string) (domain.Consent, error) {
	tenantID, actorID, consentID = strings.TrimSpace(tenantID), strings.TrimSpace(actorID), strings.TrimSpace(consentID)
	if err := checkPrincipal(tenantID, actorID); err != nil {
		return domain.Consent{}, err
	}
	if err := checkText("consent_id", consentID, maxIdentifierLen); err != nil {
		return domain.Consent{}, err
	}
	c, err := s.repo.Get(ctx, tenantID, consentID)
	if err != nil {
		return domain.Consent{}, err
	}
	if c.Status == domain.StatusWithdrawn {
		return c, nil
	}
	now := time.Now().UTC()
	before := c
	beforeState := toMap(c)
	c.Status = domain.StatusWithdrawn
	c.WithdrawnAt = &now
	c.UpdatedAt = now
	c.Version++
	audit := domain.AuditEvent{
		ID:            id("aud"),
		TenantID:      tenantID,
		ActorID:       actorID,
		Action:        "consent.withdrawn",
		ConsentID:     consentID,
		ResourceID:    consentID,
		OccurredAt:    now,
		ResourceType:  "consent",
		Revision:      c.Version,
		CorrelationID: domain.CorrelationID(ctx),
		BeforeState:   beforeState,
		AfterState:    toMap(c),
		Changes:       map[string]any{"before": before, "after": c},
	}
	outbox := domain.OutboxEvent{
		ID:                id("evt"),
		SchemaVersion:     1,
		TenantID:          tenantID,
		EventType:         "consent.withdrawn",
		AggregateType:     "consent.preference",
		AggregateID:       consentID,
		AggregateRevision: c.Version,
		Payload: map[string]any{
			"tenant_id":             tenantID,
			"subject_id":            c.SubjectID,
			"purpose":               c.Purpose,
			"channel":               c.Channel,
			"status":                string(domain.StatusWithdrawn),
			"preference_generation": c.Version,
			"aggregate_type":        "consent.preference",
			"aggregate_revision":    c.Version,
			"schema_version":        1,
		},
		OccurredAt: now,
	}
	return s.repo.Withdraw(ctx, c, audit, outbox)
}

func (s *Service) SetSuppression(ctx context.Context, tenantID, actorID string, suppression domain.Suppression, callerRoles ...string) (domain.Suppression, error) {
	tenantID = strings.TrimSpace(tenantID)
	actorID = strings.TrimSpace(actorID)
	suppression.SubjectID = strings.TrimSpace(suppression.SubjectID)
	suppression.Purpose = strings.ToLower(strings.TrimSpace(suppression.Purpose))
	suppression.Channel = strings.ToLower(strings.TrimSpace(suppression.Channel))
	suppression.Reason = strings.TrimSpace(suppression.Reason)
	if err := checkPrincipal(tenantID, actorID); err != nil {
		return domain.Suppression{}, err
	}
	if err := checkText("subject_id", suppression.SubjectID, maxIdentifierLen); err != nil {
		return domain.Suppression{}, err
	}
	if err := checkText("reason", suppression.Reason, maxReasonLen); err != nil {
		return domain.Suppression{}, err
	}
	switch suppression.Scope {
	case domain.SuppressionGlobal:
		if suppression.Purpose != "" || suppression.Channel != "" {
			return domain.Suppression{}, invalid("scope", "a global suppression must not name a purpose or channel")
		}
	case domain.SuppressionChannel:
		if suppression.Purpose != "" {
			return domain.Suppression{}, invalid("purpose", "must be empty for a channel suppression")
		}
		if err := checkCode("channel", suppression.Channel); err != nil {
			return domain.Suppression{}, err
		}
	case domain.SuppressionPurposeChannel:
		if err := checkCode("purpose", suppression.Purpose); err != nil {
			return domain.Suppression{}, err
		}
		if err := checkCode("channel", suppression.Channel); err != nil {
			return domain.Suppression{}, err
		}
	default:
		return domain.Suppression{}, invalid("scope", "must be global, channel or purpose_channel")
	}

	// T-030: Capture the caller's highest privileged role in created_by_role.
	if suppression.CreatedByRole == "" {
		if len(callerRoles) > 0 {
			suppression.CreatedByRole = domain.HighestPrivilegedRole(callerRoles)
		} else {
			suppression.CreatedByRole = "privacy_admin"
		}
	}

	now := time.Now().UTC()
	suppression.ID = id("sup")
	suppression.TenantID = tenantID
	suppression.UpdatedAt = now
	audit := domain.AuditEvent{
		ID:            id("aud"),
		TenantID:      tenantID,
		ActorID:       actorID,
		Action:        "consent.suppression.changed",
		ConsentID:     suppression.ID,
		ResourceID:    suppression.ID,
		OccurredAt:    now,
		ResourceType:  "suppression",
		CorrelationID: domain.CorrelationID(ctx),
		Reason:        suppression.Reason,
		AfterState:    toMap(suppression),
	}
	outbox := domain.OutboxEvent{
		ID:                id("evt"),
		SchemaVersion:     1,
		TenantID:          tenantID,
		EventType:         "consent.suppression.changed",
		AggregateType:     "consent.suppression",
		AggregateID:       suppression.ID,
		AggregateRevision: 1,
		Payload: map[string]any{
			"schema_version":     1,
			"aggregate_type":     "consent.suppression",
			"aggregate_revision": int64(1),
			"tenant_id":          tenantID,
			"subject_id":         suppression.SubjectID,
			"scope":              suppression.Scope,
			"purpose":            suppression.Purpose,
			"channel":            suppression.Channel,
			"active":             suppression.Active,
			"created_by_role":    suppression.CreatedByRole,
		},
		OccurredAt: now,
	}
	return s.repo.SetSuppression(ctx, suppression, audit, outbox)
}

// BatchEvaluate evaluates up to 100 requests. It stops at the first error,
// including cancellation of ctx, and never returns partial decisions.
func (s *Service) BatchEvaluate(ctx context.Context, tenantID string, requests []domain.EvaluationRequest) ([]domain.SuppressionDecision, error) {
	if err := checkText("tenant_id", strings.TrimSpace(tenantID), maxIdentifierLen); err != nil {
		return nil, err
	}
	if len(requests) == 0 || len(requests) > maxBatchRequests {
		return nil, invalid("requests", fmt.Sprintf("must contain 1-%d entries", maxBatchRequests))
	}
	decisions := make([]domain.SuppressionDecision, len(requests))
	for i, request := range requests {
		at := time.Time{}
		if request.At != nil {
			at = *request.At
		}
		decision, err := s.Evaluate(ctx, tenantID, request.SubjectID, request.Purpose, request.Channel, at)
		if err != nil {
			var typed *domain.Error
			if errors.As(err, &typed) && typed.Kind() == domain.KindValidation && typed.Field() != "" {
				return nil, typed.WithField(fmt.Sprintf("requests[%d].%s", i, typed.Field()), typed.Message())
			}
			return nil, err
		}
		decisions[i] = decision
	}
	return decisions, nil
}

func (s *Service) StartDoubleOptIn(ctx context.Context, tenantID, actorID, subjectID, purposeCode, channel, source, jurisdiction, noticeVersion string, evidence domain.Evidence, expiresAt time.Time) (domain.DoubleOptInChallenge, error) {
	now := time.Now().UTC()
	in, err := normalizeGoverned(tenantID, actorID, subjectID, purposeCode, channel, "consent", string(domain.StatusGranted), source, nil, nil, jurisdiction, noticeVersion, evidence, now)
	if err != nil {
		return domain.DoubleOptInChallenge{}, err
	}
	if err := checkChallengeExpiry(expiresAt, now); err != nil {
		return domain.DoubleOptInChallenge{}, err
	}
	purpose, _, err := checkPolicy(ctx, s.repo, in.TenantID, in.Purpose, in.Channel, "consent", in.Jurisdiction, in.NoticeVersion, in.Evidence, now)
	if err != nil {
		return domain.DoubleOptInChallenge{}, err
	}
	if !purpose.RequiresDoubleOptIn {
		return domain.DoubleOptInChallenge{}, rejected("purpose", "does not use double opt-in; record the consent directly")
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return domain.DoubleOptInChallenge{}, domain.ErrInternal.Wrap(err)
	}
	token := hex.EncodeToString(rawToken)
	tokenHash := sha256.Sum256([]byte(token))
	expiresAt = expiresAt.UTC()
	consent := domain.Consent{
		ID:                    id("con"),
		TenantID:              in.TenantID,
		SubjectID:             in.SubjectID,
		Purpose:               in.Purpose,
		Channel:               in.Channel,
		LegalBasis:            "consent",
		Status:                domain.StatusPendingConfirmation,
		Source:                in.Source,
		Jurisdiction:          in.Jurisdiction,
		NoticeVersion:         in.NoticeVersion,
		Evidence:              in.Evidence,
		Version:               1,
		ConfirmationTokenHash: hex.EncodeToString(tokenHash[:]),
		ConfirmationExpiresAt: &expiresAt,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	audit := domain.AuditEvent{
		ID:            id("aud"),
		TenantID:      in.TenantID,
		ActorID:       in.ActorID,
		Action:        "consent.pending_confirmation",
		ConsentID:     consent.ID,
		ResourceID:    consent.ID,
		OccurredAt:    now,
		ResourceType:  "consent",
		Revision:      1,
		CorrelationID: domain.CorrelationID(ctx),
		AfterState:    toMap(consent),
		Changes:       map[string]any{"before": nil, "after": consent},
	}
	outbox := domain.OutboxEvent{
		ID:                id("evt"),
		SchemaVersion:     1,
		TenantID:          in.TenantID,
		EventType:         "consent.challenge.created",
		AggregateType:     "consent.double_opt_in",
		AggregateID:       consent.ID,
		AggregateRevision: int64(1),
		Payload: map[string]any{
			"schema_version":        1,
			"aggregate_type":        "consent.double_opt_in",
			"aggregate_revision":    int64(1),
			"tenant_id":             in.TenantID,
			"subject_id":            in.SubjectID,
			"purpose":               in.Purpose,
			"channel":               in.Channel,
			"destination":           in.Channel,
			"notice_version":        in.NoticeVersion,
			"confirmation_token":    token,
			"expires_at":            expiresAt,
			"generation":            int64(1),
			"preference_generation": int64(1),
		},
		OccurredAt: now,
	}
	stored, err := s.repo.StartDoubleOptIn(ctx, consent, audit, outbox)
	if err != nil {
		return domain.DoubleOptInChallenge{}, err
	}
	return domain.DoubleOptInChallenge{Consent: stored, ConfirmationToken: token}, nil
}

// ConfirmDoubleOptIn redeems a DOI challenge token. The challenge must belong to
// a subject within scope; otherwise ErrNotFound is returned, indistinguishable
// from an unknown token, and no state, receipt, audit or outbox is written.
//
// The grant is decided inside the repository's confirmation transaction: the
// challenge must still be pending, unexpired and bound to the current
// preference generation (a later grant, denial, withdrawal or challenge makes
// it stale), and the purpose, lawful basis and notice are re-validated there
// before the receipt, audit and outbox records are built from the locked row.
func (s *Service) ConfirmDoubleOptIn(ctx context.Context, tenantID, actorID string, scope SubjectScope, token string) (domain.Consent, error) {
	tenantID = strings.TrimSpace(tenantID)
	actorID = strings.TrimSpace(actorID)
	token = strings.TrimSpace(token)
	if err := checkPrincipal(tenantID, actorID); err != nil {
		return domain.Consent{}, err
	}
	if err := checkText("token", token, maxConfirmTokenLen); err != nil {
		return domain.Consent{}, err
	}
	tokenHash := sha256.Sum256([]byte(token))
	encodedHash := hex.EncodeToString(tokenHash[:])
	pending, err := s.repo.FindPendingByToken(ctx, tenantID, encodedHash)
	if err != nil {
		return domain.Consent{}, err
	}
	// Fast pre-check; repeated against the locked row in the finalizer.
	if !scope.Permits(pending.SubjectID) {
		return domain.Consent{}, store.ErrNotFound
	}
	// Re-validate current policy for target purpose before initiating confirmation.
	currentPurpose, err := s.repo.GetPurpose(ctx, tenantID, pending.Purpose)
	if err != nil {
		return domain.Consent{}, policyLookup(err, "purpose", "is not registered for this tenant")
	}
	if !currentPurpose.Active {
		return domain.Consent{}, rejected("purpose", "is not active")
	}
	if !includes(currentPurpose.AllowedChannels, pending.Channel) {
		return domain.Consent{}, rejected("channel", "is not allowed for this purpose")
	}
	if !currentPurpose.RequiresDoubleOptIn {
		return domain.Consent{}, rejected("purpose", "no longer uses double opt-in")
	}
	if currentPurpose.RequiresAffirmativeConsent && pending.LegalBasis != "consent" {
		return domain.Consent{}, rejected("legal_basis", "this purpose requires the consent lawful basis")
	}
	finalize := func(ctx context.Context, policy store.PolicyReader, locked domain.Consent, at time.Time) (domain.ConsentReceipt, domain.AuditEvent, domain.OutboxEvent, error) {
		if !scope.Permits(locked.SubjectID) {
			return domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, store.ErrNotFound
		}
		purpose, notice, err := checkPolicy(ctx, policy, tenantID, locked.Purpose, locked.Channel, locked.LegalBasis, locked.Jurisdiction, locked.NoticeVersion, locked.Evidence, at)
		if err != nil {
			return domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, err
		}
		if !purpose.RequiresDoubleOptIn {
			return domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, rejected("purpose", "no longer uses double opt-in")
		}
		if purpose.RequiresAffirmativeConsent && locked.LegalBasis != "consent" {
			return domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, rejected("legal_basis", "this purpose requires the consent lawful basis")
		}
		receipt, err := makeReceipt(locked, locked.Evidence, at, ReceiptMetadata{
			ActorID:           actorID,
			ConsentStatus:     domain.StatusGranted,
			LegalBasis:        locked.LegalBasis,
			NoticeContentHash: notice.ContentHash,
			PolicyVersion:     purpose.Version,
		})
		if err != nil {
			return domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, domain.ErrInternal.Wrap(err)
		}
		audit := domain.AuditEvent{
			ID:            id("aud"),
			TenantID:      tenantID,
			ActorID:       actorID,
			Action:        "consent.confirmed",
			ConsentID:     locked.ID,
			ResourceID:    locked.ID,
			OccurredAt:    at,
			ResourceType:  "consent",
			Revision:      locked.Version + 1,
			CorrelationID: domain.CorrelationID(ctx),
			BeforeState:   toMap(locked),
		}
		confRev := locked.PreferenceGeneration
		if confRev <= 0 {
			confRev = locked.Version + 1
		}
		outbox := domain.OutboxEvent{
			ID:                id("evt"),
			SchemaVersion:     1,
			TenantID:          tenantID,
			EventType:         "consent.confirmed",
			AggregateType:     "consent.preference",
			AggregateID:       locked.ID,
			AggregateRevision: confRev,
			Payload: map[string]any{
				"schema_version":        1,
				"aggregate_type":        "consent.preference",
				"aggregate_revision":    confRev,
				"preference_generation": confRev,
				"tenant_id":             tenantID,
				"subject_id":            locked.SubjectID,
				"purpose":               locked.Purpose,
				"channel":               locked.Channel,
				"status":                string(domain.StatusGranted),
				"receipt_id":            receipt.ID,
			},
			OccurredAt: at,
		}
		return receipt, audit, outbox, nil
	}
	// Full precision: truncating before the policy comparison could make
	// evidence collected within the same microsecond appear future-dated.
	// makeReceipt normalizes the receipt's own timestamp.
	return s.repo.ConfirmDoubleOptIn(ctx, tenantID, encodedHash, time.Now().UTC(), finalize)
}

func (s *Service) Receipt(ctx context.Context, tenantID, receiptID string) (domain.ConsentReceipt, error) {
	tenantID, receiptID = strings.TrimSpace(tenantID), strings.TrimSpace(receiptID)
	if err := checkText("tenant_id", tenantID, maxIdentifierLen); err != nil {
		return domain.ConsentReceipt{}, err
	}
	if err := checkText("receipt_id", receiptID, maxIdentifierLen); err != nil {
		return domain.ConsentReceipt{}, err
	}
	return s.repo.GetReceipt(ctx, tenantID, receiptID)
}

func (s *Service) Consent(ctx context.Context, tenantID, consentID string) (domain.Consent, error) {
	tenantID, consentID = strings.TrimSpace(tenantID), strings.TrimSpace(consentID)
	if err := checkText("tenant_id", tenantID, maxIdentifierLen); err != nil {
		return domain.Consent{}, err
	}
	if err := checkText("consent_id", consentID, maxIdentifierLen); err != nil {
		return domain.Consent{}, err
	}
	return s.repo.Get(ctx, tenantID, consentID)
}

// policyDecision maps a registry lookup during evaluation. A missing or
// inactive policy is a definitive deny ("policy_unavailable"). An
// infrastructure failure is returned as an error: the caller still gets no
// permission (safe deny), but as a retryable 503 rather than a decision that
// looks authoritative.
func policyDecision(err error) (deny bool, failure error) {
	if err == nil {
		return false, nil
	}
	if errors.Is(err, store.ErrNotFound) {
		return true, nil
	}
	return false, err
}

func (s *Service) Evaluate(ctx context.Context, tenantID, subjectID, purpose, channel string, at time.Time) (domain.SuppressionDecision, error) {
	tenantID = strings.TrimSpace(tenantID)
	subjectID = strings.TrimSpace(subjectID)
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	channel = strings.ToLower(strings.TrimSpace(channel))
	for _, err := range []error{
		checkText("tenant_id", tenantID, maxIdentifierLen),
		checkText("subject_id", subjectID, maxIdentifierLen),
		checkCode("purpose", purpose),
		checkCode("channel", channel),
	} {
		if err != nil {
			return domain.SuppressionDecision{}, err
		}
	}
	now := time.Now().UTC()
	if !at.IsZero() {
		at = at.UTC()
		skew := now.Sub(at)
		if skew < 0 {
			skew = -skew
		}
		if skew > maxEvaluationClockSkew {
			return domain.SuppressionDecision{}, ErrUnsupportedHistoricalEvaluation.WithField("at", "evaluation timestamp diverges from current time by more than allowed clock-skew margin (1 minute)")
		}
	} else {
		at = now
	}
	suppression, err := s.repo.FindSuppression(ctx, tenantID, subjectID, purpose, channel)
	if err == nil {
		return domain.SuppressionDecision{Allowed: false, Reason: string(suppression.Scope) + "_suppression", EvaluatedAt: at}, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return domain.SuppressionDecision{}, err
	}
	c, err := s.repo.FindEffective(ctx, tenantID, subjectID, purpose, channel)
	if errors.Is(err, store.ErrNotFound) {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonNoConsent, EvaluatedAt: at}, nil
	}
	if err != nil {
		return domain.SuppressionDecision{}, err
	}
	if c.ExpiresAt != nil && !c.ExpiresAt.After(at) {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonExpired, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}
	if c.Status != domain.StatusGranted {
		return domain.SuppressionDecision{Allowed: false, Reason: string(c.Status), ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}
	if c.GrantedAt != nil && c.GrantedAt.After(at) {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonNotYetGranted, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}
	registeredPurpose, err := s.repo.GetPurpose(ctx, tenantID, purpose)
	if deny, failure := policyDecision(err); failure != nil {
		return domain.SuppressionDecision{}, failure
	} else if deny {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonPolicyUnavailable, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	} else if !registeredPurpose.Active {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonPurposeInactive, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	} else if !includes(registeredPurpose.AllowedChannels, channel) {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonChannelNotAllowed, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}
	// Policy transitions (T-007): stronger policy requirements (affirmative
	// consent or Double-Opt-In) strictly apply to existing grants; unconfirmed
	// or non-consent grants must not be grandfathered.
	if registeredPurpose.RequiresAffirmativeConsent && c.LegalBasis != "consent" {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonPolicyRequirementUnmet, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}
	if registeredPurpose.RequiresDoubleOptIn && c.ConfirmedAt == nil {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonPolicyRequirementUnmet, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}
	basis, err := s.repo.GetLawfulBasis(ctx, tenantID, c.LegalBasis)
	if deny, failure := policyDecision(err); failure != nil {
		return domain.SuppressionDecision{}, failure
	} else if deny {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonPolicyUnavailable, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	} else if !basis.Active || !includes(basis.AllowedCategories, registeredPurpose.Category) {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonPolicyRequirementUnmet, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}
	if basis.RequiresEvidence && (c.Evidence.IsZero() || strings.TrimSpace(c.Evidence.ArtifactHash) == "") {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonPolicyRequirementUnmet, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}

	// Legacy Record Eligibility Rule (T-008 / D-04 Option C):
	// Blank-notice records cannot bypass an affirmative-consent requirement by default.
	// When notice_version is empty:
	// If the purpose requires affirmative consent, requires DOI, is in the marketing category,
	// or the legal basis requires evidence or is "consent", the legacy record is quarantined (denied).
	// Only grandfathered non-marketing service/contractual processing (e.g. contract or legitimate_interest)
	// that does not require affirmative consent or evidence is considered eligible without a published notice.
	if c.NoticeVersion == "" {
		if registeredPurpose.RequiresAffirmativeConsent ||
			registeredPurpose.RequiresDoubleOptIn ||
			registeredPurpose.Category == "marketing" ||
			basis.RequiresEvidence ||
			c.LegalBasis == "consent" {
			return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonPolicyRequirementUnmet, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
		}
		// Eligible legacy service/contractual grant
		return domain.SuppressionDecision{Allowed: true, Reason: domain.DecisionReasonGranted, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}

	notice, err := s.repo.GetNotice(ctx, tenantID, c.Jurisdiction, c.NoticeVersion)
	if deny, failure := policyDecision(err); failure != nil {
		return domain.SuppressionDecision{}, failure
	} else if deny || !notice.Active || notice.Retired || notice.PublishedAt.After(at) {
		return domain.SuppressionDecision{Allowed: false, Reason: domain.DecisionReasonPolicyUnavailable, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}
	return domain.SuppressionDecision{Allowed: true, Reason: domain.DecisionReasonGranted, ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
}

func (s *Service) Ready(ctx context.Context) error {
	if err := s.repo.Ping(ctx); err != nil {
		return err
	}
	return s.repo.VerifySchema(ctx)
}

func (s *Service) ListConsents(ctx context.Context, tenantID, subjectID string, limit, offset int) ([]domain.Consent, int, error) {
	tenantID = strings.TrimSpace(tenantID)
	subjectID = strings.TrimSpace(subjectID)
	if err := checkText("tenant_id", tenantID, maxIdentifierLen); err != nil {
		return nil, 0, err
	}
	if err := checkText("subject_id", subjectID, maxIdentifierLen); err != nil {
		return nil, 0, err
	}
	limit, offset = sanitizePagination(limit, offset)
	return s.repo.ListConsents(ctx, tenantID, subjectID, limit, offset)
}

func (s *Service) ListSuppressions(ctx context.Context, tenantID, subjectID string, limit, offset int) ([]domain.Suppression, int, error) {
	tenantID = strings.TrimSpace(tenantID)
	subjectID = strings.TrimSpace(subjectID)
	if err := checkText("tenant_id", tenantID, maxIdentifierLen); err != nil {
		return nil, 0, err
	}
	if err := checkText("subject_id", subjectID, maxIdentifierLen); err != nil {
		return nil, 0, err
	}
	limit, offset = sanitizePagination(limit, offset)
	return s.repo.ListSuppressions(ctx, tenantID, subjectID, limit, offset)
}

func (s *Service) ListPurposes(ctx context.Context, tenantID string, limit, offset int) ([]domain.Purpose, int, error) {
	tenantID = strings.TrimSpace(tenantID)
	if err := checkText("tenant_id", tenantID, maxIdentifierLen); err != nil {
		return nil, 0, err
	}
	limit, offset = sanitizePagination(limit, offset)
	return s.repo.ListPurposes(ctx, tenantID, limit, offset)
}

func (s *Service) ListLawfulBases(ctx context.Context, tenantID string, limit, offset int) ([]domain.LawfulBasis, int, error) {
	tenantID = strings.TrimSpace(tenantID)
	if err := checkText("tenant_id", tenantID, maxIdentifierLen); err != nil {
		return nil, 0, err
	}
	limit, offset = sanitizePagination(limit, offset)
	return s.repo.ListLawfulBases(ctx, tenantID, limit, offset)
}

func (s *Service) ListNotices(ctx context.Context, tenantID string, limit, offset int) ([]domain.PrivacyNotice, int, error) {
	tenantID = strings.TrimSpace(tenantID)
	if err := checkText("tenant_id", tenantID, maxIdentifierLen); err != nil {
		return nil, 0, err
	}
	limit, offset = sanitizePagination(limit, offset)
	return s.repo.ListNotices(ctx, tenantID, limit, offset)
}

func (s *Service) ListAuditEvents(ctx context.Context, tenantID, resourceType, resourceID string, limit, offset int) ([]domain.AuditEvent, int, error) {
	tenantID = strings.TrimSpace(tenantID)
	if err := checkText("tenant_id", tenantID, maxIdentifierLen); err != nil {
		return nil, 0, err
	}
	limit, offset = sanitizePagination(limit, offset)
	return s.repo.ListAuditEvents(ctx, tenantID, resourceType, resourceID, limit, offset)
}

func toMap(v any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{}
	}
	if m == nil {
		return map[string]any{}
	}
	return m
}
