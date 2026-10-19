package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/Atlasfsp/Nvuto-Consent/internal/store"
)

// ErrIdempotencyUnsupported is a deployment fault: the configured repository
// cannot provide governed idempotency. It is reported as an internal error.
var ErrIdempotencyUnsupported = domain.ErrInternal.Derive("idempotency_unsupported", "governed consent idempotency is not supported by repository")

type governedFingerprint struct {
	TenantID      string            `json:"tenant_id"`
	ActorID       string            `json:"actor_id"`
	SubjectID     string            `json:"subject_id"`
	Purpose       string            `json:"purpose"`
	Channel       string            `json:"channel"`
	LegalBasis    string            `json:"legal_basis"`
	Status        domain.Status     `json:"status"`
	Source        string            `json:"source"`
	Scope         map[string]string `json:"scope,omitempty"`
	ExpiresAt     *time.Time        `json:"expires_at,omitempty"`
	Jurisdiction  string            `json:"jurisdiction"`
	NoticeVersion string            `json:"notice_version"`
	Evidence      domain.Evidence   `json:"evidence"`
}

func fingerprintGoverned(input governedFingerprint) string {
	encoded, _ := json.Marshal(input)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// RecordGovernedIdempotent records a governed consent at most once per
// idempotency key. The request is validated and admitted exactly like
// RecordGoverned; the fingerprint is computed over the normalized request so
// cosmetic differences (case, whitespace) replay instead of conflicting.
func (s *Service) RecordGovernedIdempotent(ctx context.Context, idempotencyKey, tenantID, actorID, subjectID, purposeCode, channel, legalBasis, status, source string, scope map[string]string, expiresAt *time.Time, jurisdiction, noticeVersion string, evidence domain.Evidence) (domain.Consent, error) {
	checkedAt := time.Now().UTC()
	in, err := normalizeGoverned(tenantID, actorID, subjectID, purposeCode, channel, legalBasis, status, source, scope, expiresAt, jurisdiction, noticeVersion, evidence, checkedAt)
	if err != nil {
		return domain.Consent{}, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if err := checkText("idempotency_key", idempotencyKey, maxIdempotencyKey); err != nil {
		return domain.Consent{}, err
	}
	repo, ok := s.repo.(store.GovernedIdempotencyRepository)
	if !ok {
		return domain.Consent{}, ErrIdempotencyUnsupported
	}
	if err := s.admitGoverned(ctx, in, checkedAt); err != nil {
		return domain.Consent{}, err
	}
	// Same field normalization as before T-027, so fingerprints of keys
	// stored by earlier releases still replay.
	requestHash := fingerprintGoverned(governedFingerprint{
		TenantID: in.TenantID, ActorID: in.ActorID, SubjectID: in.SubjectID, Purpose: in.Purpose, Channel: in.Channel,
		LegalBasis: in.LegalBasis, Status: in.Status, Source: in.Source, Scope: in.Scope, ExpiresAt: in.ExpiresAt,
		Jurisdiction: in.Jurisdiction, NoticeVersion: in.NoticeVersion, Evidence: in.Evidence,
	})
	consent, receipt, audit, outbox, err := governedRecords(in, normalizeReceiptTime(time.Now()))
	if err != nil {
		return domain.Consent{}, err
	}
	return repo.RecordGovernedIdempotent(ctx, idempotencyKey, requestHash, consent, receipt, audit, outbox)
}
