package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// ErrValidation matches every input-validation and policy-rejection error.
var ErrValidation = domain.ErrValidation

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
	audit := domain.AuditEvent{ID: id("aud"), TenantID: tenantID, ActorID: actorID, Action: "consent.purpose.registered", ConsentID: purpose.Code, OccurredAt: purpose.UpdatedAt}
	outbox := domain.OutboxEvent{ID: id("evt"), TenantID: tenantID, EventType: "consent.purpose.changed", AggregateID: purpose.Code, Payload: map[string]any{"category": purpose.Category, "active": purpose.Active}, OccurredAt: purpose.UpdatedAt}
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
	audit := domain.AuditEvent{ID: id("aud"), TenantID: tenantID, ActorID: actorID, Action: "consent.lawful_basis.registered", ConsentID: basis.Code, OccurredAt: basis.UpdatedAt}
	outbox := domain.OutboxEvent{ID: id("evt"), TenantID: tenantID, EventType: "consent.lawful_basis.changed", AggregateID: basis.Code, Payload: map[string]any{"active": basis.Active}, OccurredAt: basis.UpdatedAt}
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
	audit := domain.AuditEvent{ID: id("aud"), TenantID: tenantID, ActorID: actorID, Action: "consent.notice.published", ConsentID: notice.Version, OccurredAt: notice.PublishedAt}
	outbox := domain.OutboxEvent{ID: id("evt"), TenantID: tenantID, EventType: "consent.notice.published", AggregateID: notice.Version, Payload: map[string]any{"jurisdiction": notice.Jurisdiction, "active": notice.Active}, OccurredAt: notice.PublishedAt}
	return s.repo.PutNotice(ctx, notice, audit, outbox)
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
func checkPolicy(ctx context.Context, reader store.PolicyReader, tenantID, purposeCode, channel, basisCode, jurisdiction, noticeVersion string, evidence domain.Evidence, now time.Time) (domain.Purpose, error) {
	purpose, err := reader.GetPurpose(ctx, tenantID, purposeCode)
	if err != nil {
		return domain.Purpose{}, policyLookup(err, "purpose", "is not registered for this tenant")
	}
	if !purpose.Active {
		return domain.Purpose{}, rejected("purpose", "is not active")
	}
	if !includes(purpose.AllowedChannels, channel) {
		return domain.Purpose{}, rejected("channel", "is not allowed for this purpose")
	}
	basis, err := reader.GetLawfulBasis(ctx, tenantID, basisCode)
	if err != nil {
		return domain.Purpose{}, policyLookup(err, "legal_basis", "is not registered for this tenant")
	}
	if !basis.Active || !includes(basis.AllowedCategories, purpose.Category) {
		return domain.Purpose{}, rejected("legal_basis", "is not active or does not cover the purpose category")
	}
	notice, err := reader.GetNotice(ctx, tenantID, jurisdiction, noticeVersion)
	if err != nil {
		return domain.Purpose{}, policyLookup(err, "notice_version", "is not published for this jurisdiction")
	}
	if !notice.Active || notice.PublishedAt.After(now) {
		return domain.Purpose{}, rejected("notice_version", "is not active or not yet published")
	}
	if basis.RequiresEvidence {
		switch {
		case strings.TrimSpace(evidence.Method) == "" || strings.TrimSpace(evidence.ArtifactHash) == "" || evidence.CollectedAt.IsZero():
			return domain.Purpose{}, rejected("evidence", "is required by the lawful basis")
		case evidence.CollectedAt.Before(notice.PublishedAt):
			return domain.Purpose{}, rejected("evidence.collected_at", "must not precede the notice publication")
		case evidence.CollectedAt.After(now):
			return domain.Purpose{}, rejected("evidence.collected_at", "must not be in the future")
		}
	}
	return purpose, nil
}

// admitGoverned applies the registry policy and the grant rules that depend
// on the purpose. A grant may only be recorded when every rule passes.
func (s *Service) admitGoverned(ctx context.Context, in governedInput, now time.Time) error {
	purpose, err := checkPolicy(ctx, s.repo, in.TenantID, in.Purpose, in.Channel, in.LegalBasis, in.Jurisdiction, in.NoticeVersion, in.Evidence, now)
	if err != nil {
		return err
	}
	if in.Status == domain.StatusGranted && purpose.RequiresDoubleOptIn {
		return rejected("status", "this purpose requires double opt-in confirmation")
	}
	if in.Status == domain.StatusGranted && purpose.RequiresAffirmativeConsent && in.LegalBasis != "consent" {
		return rejected("legal_basis", "this purpose requires the consent lawful basis for a grant")
	}
	return nil
}

// governedRecords builds the consent, receipt, audit and outbox records for a
// validated, admitted request.
func governedRecords(in governedInput, now time.Time) (domain.Consent, domain.ConsentReceipt, domain.AuditEvent, domain.OutboxEvent, error) {
	consent := domain.Consent{ID: id("con"), TenantID: in.TenantID, SubjectID: in.SubjectID, Purpose: in.Purpose, Channel: in.Channel, LegalBasis: in.LegalBasis, Status: in.Status, Source: in.Source, Scope: in.Scope, Jurisdiction: in.Jurisdiction, NoticeVersion: in.NoticeVersion, Evidence: in.Evidence, Version: 1, ExpiresAt: in.ExpiresAt, CreatedAt: now, UpdatedAt: now}
	if in.Status == domain.StatusGranted {
		consent.GrantedAt = &now
	} else {
		consent.DeniedAt = &now
	}
	receipt, err := makeReceipt(consent, in.Evidence, now)
	if err != nil {
		return domain.Consent{}, domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, domain.ErrInternal.Wrap(err)
	}
	consent.ReceiptID = receipt.ID
	audit := domain.AuditEvent{ID: id("aud"), TenantID: in.TenantID, ActorID: in.ActorID, Action: "consent." + string(in.Status), ConsentID: consent.ID, OccurredAt: now}
	outbox := domain.OutboxEvent{ID: id("evt"), TenantID: in.TenantID, EventType: "consent." + string(in.Status), AggregateID: consent.ID, Payload: map[string]any{"subject_id": in.SubjectID, "purpose": in.Purpose, "channel": in.Channel, "receipt_id": receipt.ID, "notice_version": in.NoticeVersion}, OccurredAt: now}
	return consent, receipt, audit, outbox, nil
}

func (s *Service) RecordGoverned(ctx context.Context, tenantID, actorID, subjectID, purposeCode, channel, legalBasis, status, source string, scope map[string]string, expiresAt *time.Time, jurisdiction, noticeVersion string, evidence domain.Evidence) (domain.Consent, error) {
	checkedAt := time.Now().UTC()
	in, err := normalizeGoverned(tenantID, actorID, subjectID, purposeCode, channel, legalBasis, status, source, scope, expiresAt, jurisdiction, noticeVersion, evidence, checkedAt)
	if err != nil {
		return domain.Consent{}, err
	}
	if err := s.admitGoverned(ctx, in, checkedAt); err != nil {
		return domain.Consent{}, err
	}
	consent, receipt, audit, outbox, err := governedRecords(in, normalizeReceiptTime(time.Now()))
	if err != nil {
		return domain.Consent{}, err
	}
	return s.repo.RecordGoverned(ctx, consent, receipt, audit, outbox)
}

// Record is the legacy ungoverned write path, kept for existing callers. It
// applies the identifier and expiry bounds but no policy registry checks.
func (s *Service) Record(ctx context.Context, tenantID, actorID, subjectID, purpose, channel, legalBasis, status, source string, scope map[string]string, expiresAt *time.Time) (domain.Consent, error) {
	now := time.Now().UTC()
	tenantID, actorID, subjectID = strings.TrimSpace(tenantID), strings.TrimSpace(actorID), strings.TrimSpace(subjectID)
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	channel = strings.ToLower(strings.TrimSpace(channel))
	legalBasis = strings.ToLower(strings.TrimSpace(legalBasis))
	source = strings.ToLower(strings.TrimSpace(source))
	st := domain.Status(strings.ToLower(strings.TrimSpace(status)))
	for _, err := range []error{
		checkPrincipal(tenantID, actorID),
		checkText("subject_id", subjectID, maxIdentifierLen),
		checkCode("purpose", purpose),
		checkCode("channel", channel),
		checkCode("source", source),
		checkScope(scope),
		checkConsentExpiry(expiresAt, now),
	} {
		if err != nil {
			return domain.Consent{}, err
		}
	}
	if st != domain.StatusGranted && st != domain.StatusDenied {
		return domain.Consent{}, invalid("status", "must be granted or denied")
	}
	if st == domain.StatusGranted {
		if err := checkCode("legal_basis", legalBasis); err != nil {
			return domain.Consent{}, err
		}
	}
	c := domain.Consent{ID: id("con"), TenantID: tenantID, SubjectID: subjectID, Purpose: purpose, Channel: channel, LegalBasis: legalBasis, Status: st, Source: source, Scope: scope, Version: 1, ExpiresAt: expiresAt, CreatedAt: now, UpdatedAt: now}
	if st == domain.StatusGranted {
		c.GrantedAt = &now
	} else {
		c.DeniedAt = &now
	}
	out, err := s.repo.Upsert(ctx, c)
	if err != nil {
		return domain.Consent{}, err
	}
	_ = s.repo.AppendAudit(ctx, domain.AuditEvent{ID: id("aud"), TenantID: tenantID, ActorID: actorID, Action: "consent." + string(st), ConsentID: c.ID, OccurredAt: now})
	_ = s.repo.AppendOutbox(ctx, domain.OutboxEvent{ID: id("evt"), TenantID: tenantID, EventType: "consent." + string(st), AggregateID: c.ID, Payload: map[string]any{"subject_id": subjectID, "purpose": c.Purpose, "channel": c.Channel, "legal_basis": c.LegalBasis}, OccurredAt: now})
	return out, nil
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
	c.Status = domain.StatusWithdrawn
	c.WithdrawnAt = &now
	c.UpdatedAt = now
	c.Version++
	audit := domain.AuditEvent{ID: id("aud"), TenantID: tenantID, ActorID: actorID, Action: "consent.withdrawn", ConsentID: consentID, OccurredAt: now}
	outbox := domain.OutboxEvent{ID: id("evt"), TenantID: tenantID, EventType: "consent.withdrawn", AggregateID: consentID, Payload: map[string]any{"subject_id": c.SubjectID, "purpose": c.Purpose, "channel": c.Channel}, OccurredAt: now}
	return s.repo.Withdraw(ctx, c, audit, outbox)
}

func (s *Service) SetSuppression(ctx context.Context, tenantID, actorID string, suppression domain.Suppression) (domain.Suppression, error) {
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
	now := time.Now().UTC()
	suppression.ID = id("sup")
	suppression.TenantID = tenantID
	suppression.UpdatedAt = now
	audit := domain.AuditEvent{ID: id("aud"), TenantID: tenantID, ActorID: actorID, Action: "consent.suppression.changed", ConsentID: suppression.ID, OccurredAt: now}
	outbox := domain.OutboxEvent{ID: id("evt"), TenantID: tenantID, EventType: "consent.suppression.changed", AggregateID: suppression.ID, Payload: map[string]any{"subject_id": suppression.SubjectID, "scope": suppression.Scope, "purpose": suppression.Purpose, "channel": suppression.Channel, "active": suppression.Active}, OccurredAt: now}
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
				return nil, invalid(fmt.Sprintf("requests[%d].%s", i, typed.Field()), typed.Message())
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
	purpose, err := checkPolicy(ctx, s.repo, in.TenantID, in.Purpose, in.Channel, "consent", in.Jurisdiction, in.NoticeVersion, in.Evidence, now)
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
	audit := domain.AuditEvent{ID: id("aud"), TenantID: in.TenantID, ActorID: in.ActorID, Action: "consent.pending_confirmation", ConsentID: consent.ID, OccurredAt: now}
	outbox := domain.OutboxEvent{ID: id("evt"), TenantID: in.TenantID, EventType: "consent.pending_confirmation", AggregateID: consent.ID, Payload: map[string]any{"subject_id": in.SubjectID, "purpose": in.Purpose, "channel": in.Channel, "notice_version": in.NoticeVersion}, OccurredAt: now}
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
	finalize := func(ctx context.Context, policy store.PolicyReader, locked domain.Consent, at time.Time) (domain.ConsentReceipt, domain.AuditEvent, domain.OutboxEvent, error) {
		if !scope.Permits(locked.SubjectID) {
			return domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, store.ErrNotFound
		}
		purpose, err := checkPolicy(ctx, policy, tenantID, locked.Purpose, locked.Channel, locked.LegalBasis, locked.Jurisdiction, locked.NoticeVersion, locked.Evidence, at)
		if err != nil {
			return domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, err
		}
		if !purpose.RequiresDoubleOptIn {
			return domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, rejected("purpose", "no longer uses double opt-in")
		}
		receipt, err := makeReceipt(locked, locked.Evidence, at)
		if err != nil {
			return domain.ConsentReceipt{}, domain.AuditEvent{}, domain.OutboxEvent{}, domain.ErrInternal.Wrap(err)
		}
		audit := domain.AuditEvent{ID: id("aud"), TenantID: tenantID, ActorID: actorID, Action: "consent.confirmed", ConsentID: locked.ID, OccurredAt: at}
		outbox := domain.OutboxEvent{ID: id("evt"), TenantID: tenantID, EventType: "consent.confirmed", AggregateID: locked.ID, Payload: map[string]any{"subject_id": locked.SubjectID, "purpose": locked.Purpose, "channel": locked.Channel, "receipt_id": receipt.ID}, OccurredAt: at}
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
	if at.IsZero() {
		at = time.Now().UTC()
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
		return domain.SuppressionDecision{Allowed: false, Reason: "no_consent", EvaluatedAt: at}, nil
	}
	if err != nil {
		return domain.SuppressionDecision{}, err
	}
	if c.ExpiresAt != nil && !c.ExpiresAt.After(at) {
		return domain.SuppressionDecision{Allowed: false, Reason: "expired", ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}
	if c.Status != domain.StatusGranted {
		return domain.SuppressionDecision{Allowed: false, Reason: string(c.Status), ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
	}
	if c.NoticeVersion != "" {
		unavailable := domain.SuppressionDecision{Allowed: false, Reason: "policy_unavailable", ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}
		registeredPurpose, err := s.repo.GetPurpose(ctx, tenantID, purpose)
		if deny, failure := policyDecision(err); failure != nil {
			return domain.SuppressionDecision{}, failure
		} else if deny || !registeredPurpose.Active || !includes(registeredPurpose.AllowedChannels, channel) {
			return unavailable, nil
		}
		basis, err := s.repo.GetLawfulBasis(ctx, tenantID, c.LegalBasis)
		if deny, failure := policyDecision(err); failure != nil {
			return domain.SuppressionDecision{}, failure
		} else if deny || !basis.Active || !includes(basis.AllowedCategories, registeredPurpose.Category) {
			return unavailable, nil
		}
		notice, err := s.repo.GetNotice(ctx, tenantID, c.Jurisdiction, c.NoticeVersion)
		if deny, failure := policyDecision(err); failure != nil {
			return domain.SuppressionDecision{}, failure
		} else if deny || !notice.Active || notice.PublishedAt.After(at) {
			return unavailable, nil
		}
	}
	return domain.SuppressionDecision{Allowed: true, Reason: "granted", ConsentID: c.ID, ReceiptID: c.ReceiptID, EvaluatedAt: at}, nil
}

func (s *Service) Ready(ctx context.Context) error { return s.repo.Ping(ctx) }
