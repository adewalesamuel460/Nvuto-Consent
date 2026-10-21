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

// ErrSubjectForbidden indicates the caller lacks authority for the requested customer subject.
var ErrSubjectForbidden = domain.ErrForbidden.Derive("subject_not_authorized", "subject not authorized")

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
//
// A replay of an already recorded idempotency key returns the exact original
// 201 response snapshot, even if the policy was subsequently updated or the
// consent was subsequently withdrawn (T-025). The caller's subject authorization
// is re-evaluated before returning the snapshot.
func (s *Service) RecordGovernedIdempotent(ctx context.Context, idempotencyKey, tenantID, actorID, subjectID, purposeCode, channel, legalBasis, status, source string, scope map[string]string, expiresAt *time.Time, jurisdiction, noticeVersion string, evidence domain.Evidence, subjectScopes ...SubjectScope) (domain.Consent, error) {
	checkedAt := time.Now().UTC()
	in, err := normalizeGoverned(tenantID, actorID, subjectID, purposeCode, channel, legalBasis, status, source, scope, expiresAt, jurisdiction, noticeVersion, evidence, checkedAt)
	if err != nil {
		return domain.Consent{}, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if err := checkText("idempotency_key", idempotencyKey, maxIdempotencyKey); err != nil {
		return domain.Consent{}, err
	}
	subScope := TenantWide()
	if len(subjectScopes) > 0 {
		subScope = subjectScopes[0]
	}
	if !subScope.Permits(in.SubjectID) {
		return domain.Consent{}, ErrSubjectForbidden
	}

	repo, ok := s.repo.(store.GovernedIdempotencyRepository)
	if !ok {
		return domain.Consent{}, ErrIdempotencyUnsupported
	}

	// Same field normalization as before T-027, so fingerprints of keys
	// stored by earlier releases still replay.
	requestHash := fingerprintGoverned(governedFingerprint{
		TenantID: in.TenantID, ActorID: in.ActorID, SubjectID: in.SubjectID, Purpose: in.Purpose, Channel: in.Channel,
		LegalBasis: in.LegalBasis, Status: in.Status, Source: in.Source, Scope: in.Scope, ExpiresAt: in.ExpiresAt,
		Jurisdiction: in.Jurisdiction, NoticeVersion: in.NoticeVersion, Evidence: in.Evidence,
	})

	// Probe if this idempotency key already exists. A completed key returns
	// the stored response snapshot without re-running policy checks, ensuring
	// replay semantics do not drift when policy updates or consent is withdrawn (T-025).
	if snapshot, storedHash, found, err := repo.FindIdempotencySnapshot(ctx, in.TenantID, idempotencyKey); err != nil {
		return domain.Consent{}, err
	} else if found {
		if storedHash != requestHash {
			return domain.Consent{}, store.ErrConflict
		}
		if !subScope.Permits(snapshot.SubjectID) {
			return domain.Consent{}, ErrSubjectForbidden
		}
		return snapshot, nil
	}

	purpose, notice, err := s.admitGoverned(ctx, in, checkedAt)
	if err != nil {
		return domain.Consent{}, err
	}
	consent, receipt, audit, outbox, err := governedRecords(in, normalizeReceiptTime(time.Now()), purpose, notice)
	if err != nil {
		return domain.Consent{}, err
	}
	result, err := repo.RecordGovernedIdempotent(ctx, idempotencyKey, requestHash, consent, receipt, audit, outbox)
	if err != nil {
		return domain.Consent{}, err
	}
	if !subScope.Permits(result.SubjectID) {
		return domain.Consent{}, ErrSubjectForbidden
	}
	return result, nil
}
