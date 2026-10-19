package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
)

// Input contract (T-027). Every write is validated here, independently of the
// transport, before any repository call, so a malformed request is rejected
// with a typed 400 and never reaches persistence. Limits are provisional and
// recorded in one place so an approved policy decision can change them.
const (
	maxIdentifierLen   = 256
	maxReasonLen       = 512
	maxScopeEntries    = 32
	maxScopeValueLen   = 256
	maxListEntries     = 32
	maxConfirmTokenLen = 256
	maxIdempotencyKey  = 200
	maxBatchRequests   = 100

	// MaxConsentLifetime bounds expires_at on a recorded consent.
	MaxConsentLifetime = 10 * 365 * 24 * time.Hour
	// MaxConfirmationLifetime bounds expires_at on a double opt-in challenge.
	MaxConfirmationLifetime = 30 * 24 * time.Hour
)

var (
	codePattern         = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,63}$`)
	scopeKeyPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)
	jurisdictionPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{1,15}$`)
	versionPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)
	methodPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)
	artifactHashPattern = regexp.MustCompile(`^(sha256:[0-9a-f]{64}|sha384:[0-9a-f]{96}|sha512:[0-9a-f]{128})$`)
)

// invalid reports malformed or out-of-bounds input (HTTP 400).
func invalid(field, message string) error {
	return domain.ErrValidation.WithField(field, message)
}

// rejected reports well-formed input refused by tenant policy (HTTP 422).
func rejected(field, message string) error {
	return domain.ErrPolicyRejected.WithField(field, message)
}

// checkText requires a bounded, printable, valid UTF-8 value.
func checkText(field, value string, maxLen int) error {
	switch {
	case value == "":
		return invalid(field, "is required")
	case len(value) > maxLen:
		return invalid(field, fmt.Sprintf("must be at most %d bytes", maxLen))
	case !utf8.ValidString(value):
		return invalid(field, "must be valid UTF-8")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return invalid(field, "must not contain control characters")
		}
	}
	return nil
}

// checkCode validates an already lower-cased registry code (purpose, channel,
// lawful basis, category, source).
func checkCode(field, value string) error {
	if value == "" {
		return invalid(field, "is required")
	}
	if !codePattern.MatchString(value) {
		return invalid(field, "must be 1-64 characters of a-z, 0-9, '_', '.', ':' or '-', starting alphanumeric")
	}
	return nil
}

func checkJurisdiction(field, value string) error {
	if !jurisdictionPattern.MatchString(value) {
		return invalid(field, "must be 2-16 characters of A-Z, 0-9, '_' or '-'")
	}
	return nil
}

func checkNoticeVersion(field, value string) error {
	if !versionPattern.MatchString(value) {
		return invalid(field, "must be 1-64 characters of A-Z, a-z, 0-9, '.', '_', ':' or '-'")
	}
	return nil
}

// normalizeCodes lower-cases, de-duplicates and validates a list of codes.
// Unlike silently dropping bad entries, an invalid entry rejects the request.
func normalizeCodes(field string, values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, invalid(field, "must contain at least one entry")
	}
	if len(values) > maxListEntries {
		return nil, invalid(field, fmt.Sprintf("must contain at most %d entries", maxListEntries))
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for i, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if err := checkCode(fmt.Sprintf("%s[%d]", field, i), value); err != nil {
			return nil, err
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out, nil
}

// checkEvidenceFormat validates the shape of supplied evidence. Empty evidence
// passes here; whether evidence is required is a lawful-basis policy question
// answered by checkPolicy. A partially supplied record is always rejected.
func checkEvidenceFormat(e domain.Evidence, now time.Time) error {
	if e.IsZero() {
		return nil
	}
	if !methodPattern.MatchString(e.Method) {
		return invalid("evidence.method", "must be 1-64 characters of A-Z, a-z, 0-9, '_', '.', ':' or '-'")
	}
	if !artifactHashPattern.MatchString(e.ArtifactHash) {
		return invalid("evidence.artifact_hash", "must be sha256:, sha384: or sha512: followed by the lowercase hex digest")
	}
	if e.CollectedAt.IsZero() {
		return invalid("evidence.collected_at", "is required")
	}
	if e.CollectedAt.After(now) {
		return invalid("evidence.collected_at", "must not be in the future")
	}
	return nil
}

// checkConsentExpiry requires an optional expiry to be in the future and
// within MaxConsentLifetime, so a grant can never be created already expired.
func checkConsentExpiry(expiresAt *time.Time, now time.Time) error {
	if expiresAt == nil {
		return nil
	}
	if !expiresAt.After(now) {
		return invalid("expires_at", "must be in the future")
	}
	if expiresAt.After(now.Add(MaxConsentLifetime)) {
		return invalid("expires_at", "must be within 10 years")
	}
	return nil
}

func checkChallengeExpiry(expiresAt, now time.Time) error {
	if expiresAt.IsZero() {
		return invalid("expires_at", "is required")
	}
	if !expiresAt.After(now) {
		return invalid("expires_at", "must be in the future")
	}
	if expiresAt.After(now.Add(MaxConfirmationLifetime)) {
		return invalid("expires_at", "must be within 30 days")
	}
	return nil
}

func checkScope(scope map[string]string) error {
	if len(scope) > maxScopeEntries {
		return invalid("scope", fmt.Sprintf("must contain at most %d entries", maxScopeEntries))
	}
	for k, v := range scope {
		if !scopeKeyPattern.MatchString(k) {
			return invalid("scope", "keys must be 1-64 characters of A-Z, a-z, 0-9, '_', '.', ':' or '-'")
		}
		if err := checkText("scope."+k, v, maxScopeValueLen); err != nil {
			return err
		}
	}
	return nil
}

// checkPrincipal validates the authenticated tenant and actor identifiers.
func checkPrincipal(tenantID, actorID string) error {
	if err := checkText("tenant_id", tenantID, maxIdentifierLen); err != nil {
		return err
	}
	return checkText("actor_id", actorID, maxIdentifierLen)
}

// governedInput is a normalized, validated governed consent request.
type governedInput struct {
	TenantID, ActorID, SubjectID string
	Purpose, Channel, LegalBasis string
	Status                       domain.Status
	Source                       string
	Scope                        map[string]string
	ExpiresAt                    *time.Time
	Jurisdiction, NoticeVersion  string
	Evidence                     domain.Evidence
}

func normalizeGoverned(tenantID, actorID, subjectID, purposeCode, channel, legalBasis, status, source string, scope map[string]string, expiresAt *time.Time, jurisdiction, noticeVersion string, evidence domain.Evidence, now time.Time) (governedInput, error) {
	in := governedInput{
		TenantID:      strings.TrimSpace(tenantID),
		ActorID:       strings.TrimSpace(actorID),
		SubjectID:     strings.TrimSpace(subjectID),
		Purpose:       strings.ToLower(strings.TrimSpace(purposeCode)),
		Channel:       strings.ToLower(strings.TrimSpace(channel)),
		LegalBasis:    strings.ToLower(strings.TrimSpace(legalBasis)),
		Status:        domain.Status(strings.ToLower(strings.TrimSpace(status))),
		Source:        strings.ToLower(strings.TrimSpace(source)),
		Scope:         scope,
		ExpiresAt:     expiresAt,
		Jurisdiction:  strings.ToUpper(strings.TrimSpace(jurisdiction)),
		NoticeVersion: strings.TrimSpace(noticeVersion),
		Evidence:      evidence,
	}
	checks := []func() error{
		func() error { return checkPrincipal(in.TenantID, in.ActorID) },
		func() error { return checkText("subject_id", in.SubjectID, maxIdentifierLen) },
		func() error { return checkCode("purpose", in.Purpose) },
		func() error { return checkCode("channel", in.Channel) },
		func() error { return checkCode("legal_basis", in.LegalBasis) },
		func() error {
			if in.Status != domain.StatusGranted && in.Status != domain.StatusDenied {
				return invalid("status", "must be granted or denied")
			}
			return nil
		},
		func() error { return checkCode("source", in.Source) },
		func() error { return checkScope(in.Scope) },
		func() error { return checkConsentExpiry(in.ExpiresAt, now) },
		func() error { return checkJurisdiction("jurisdiction", in.Jurisdiction) },
		func() error { return checkNoticeVersion("notice_version", in.NoticeVersion) },
		func() error { return checkEvidenceFormat(in.Evidence, now) },
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return governedInput{}, err
		}
	}
	return in, nil
}
