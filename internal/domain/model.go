package domain

import (
	"context"
	"time"
)

type Status string

const (
	StatusGranted             Status = "granted"
	StatusDenied              Status = "denied"
	StatusWithdrawn           Status = "withdrawn"
	StatusExpired             Status = "expired"
	StatusPendingConfirmation Status = "pending_confirmation"
)

type Consent struct {
	ID                    string            `json:"id"`
	TenantID              string            `json:"tenant_id"`
	SubjectID             string            `json:"subject_id"`
	Purpose               string            `json:"purpose"`
	Channel               string            `json:"channel"`
	LegalBasis            string            `json:"legal_basis"`
	Status                Status            `json:"status"`
	Source                string            `json:"source"`
	Scope                 map[string]string `json:"scope,omitempty"`
	Jurisdiction          string            `json:"jurisdiction,omitempty"`
	NoticeVersion         string            `json:"notice_version,omitempty"`
	ReceiptID             string            `json:"receipt_id,omitempty"`
	Evidence              Evidence          `json:"-"`
	Version               int64             `json:"version"`
	GrantedAt             *time.Time        `json:"granted_at,omitempty"`
	DeniedAt              *time.Time        `json:"denied_at,omitempty"`
	WithdrawnAt           *time.Time        `json:"withdrawn_at,omitempty"`
	ExpiresAt             *time.Time        `json:"expires_at,omitempty"`
	ConfirmationTokenHash string            `json:"-"`
	ConfirmationExpiresAt *time.Time        `json:"confirmation_expires_at,omitempty"`
	ConfirmedAt           *time.Time        `json:"confirmed_at,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
	// PreferenceGeneration is the value of the per-(tenant, subject, purpose,
	// channel) preference generation assigned when this record last expressed
	// intent. The record with the highest generation is the effective one, and
	// a pending challenge is only confirmable while its generation is current.
	PreferenceGeneration int64 `json:"-"`
}

type SuppressionDecision struct {
	Allowed     bool      `json:"allowed"`
	Reason      string    `json:"reason"`
	ConsentID   string    `json:"consent_id,omitempty"`
	ReceiptID   string    `json:"receipt_id,omitempty"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// Evaluation decision reason codes (T-007).
const (
	DecisionReasonGranted                    = "granted"
	DecisionReasonNoConsent                  = "no_consent"
	DecisionReasonExpired                    = "expired"
	DecisionReasonNotYetGranted              = "not_yet_granted"
	DecisionReasonPolicyUnavailable          = "policy_unavailable"
	DecisionReasonPolicyRequirementUnmet     = "policy_requirement_unmet"
	DecisionReasonPurposeInactive            = "purpose_inactive"
	DecisionReasonChannelNotAllowed          = "channel_not_allowed"
	DecisionReasonAffirmativeConsentRequired = "affirmative_consent_required"
	DecisionReasonDoubleOptInRequired        = "double_opt_in_required"
	DecisionReasonEvidenceRequired           = "evidence_required"
	DecisionReasonLawfulBasisInactive        = "lawful_basis_inactive"
	DecisionReasonNoticeInactive             = "notice_inactive"
)

type Purpose struct {
	TenantID                   string    `json:"tenant_id"`
	Code                       string    `json:"code"`
	Category                   string    `json:"category"`
	AllowedChannels            []string  `json:"allowed_channels"`
	RequiresAffirmativeConsent bool      `json:"requires_affirmative_consent"`
	RequiresDoubleOptIn        bool      `json:"requires_double_opt_in"`
	Active                     bool      `json:"active"`
	Version                    int64     `json:"version"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

type LawfulBasis struct {
	TenantID          string    `json:"tenant_id"`
	Code              string    `json:"code"`
	AllowedCategories []string  `json:"allowed_categories"`
	RequiresEvidence  bool      `json:"requires_evidence"`
	Active            bool      `json:"active"`
	Version           int64     `json:"version"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type PrivacyNotice struct {
	TenantID     string    `json:"tenant_id"`
	Version      string    `json:"version"`
	Jurisdiction string    `json:"jurisdiction"`
	PublishedAt  time.Time `json:"published_at"`
	Active       bool      `json:"active"`
	Content      string    `json:"content,omitempty"`
	ContentHash  string    `json:"content_hash,omitempty"`
	Retired      bool      `json:"retired,omitempty"`
	SupersededBy string    `json:"superseded_by,omitempty"`
}

// Evidence is the collection proof attached to a consent decision. Input
// contract (enforced by the service; T-027):
//   - Method: 1-64 characters from [A-Za-z0-9_.:-], starting alphanumeric.
//   - ArtifactHash: "<algorithm>:<lowercase hex digest>" where algorithm is
//     sha256 (64 hex), sha384 (96 hex) or sha512 (128 hex).
//   - CollectedAt: set, not after the server clock, and not before the
//     accepted privacy notice was published.
//
// All three fields are required whenever the lawful basis requires evidence
// and whenever any one of them is supplied; a partial record is rejected.
type Evidence struct {
	Method       string    `json:"method"`
	ArtifactHash string    `json:"artifact_hash"`
	CollectedAt  time.Time `json:"collected_at"`
}

// IsZero reports whether no evidence field was supplied.
func (e Evidence) IsZero() bool {
	return e.Method == "" && e.ArtifactHash == "" && e.CollectedAt.IsZero()
}

type ConsentReceipt struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	ConsentID         string    `json:"consent_id"`
	SubjectID         string    `json:"subject_id"`
	ActorID           string    `json:"actor_id,omitempty"`
	Purpose           string    `json:"purpose"`
	Channel           string    `json:"channel"`
	ConsentStatus     Status    `json:"consent_status,omitempty"`
	LegalBasis        string    `json:"legal_basis,omitempty"`
	NoticeVersion     string    `json:"notice_version"`
	NoticeContentHash string    `json:"notice_content_hash,omitempty"`
	PolicyVersion     int64     `json:"policy_version,omitempty"`
	Jurisdiction      string    `json:"jurisdiction"`
	Evidence          Evidence  `json:"evidence"`
	ReceiptHash       string    `json:"receipt_hash"`
	IssuedAt          time.Time `json:"issued_at"`
	// HashVersion identifies the canonical serialization ReceiptHash was
	// computed over. 0 or 1 is the legacy format; 2 is the canonical v2 format;
	// 3 is the canonical v3 format with full decision metadata and notice content hash.
	HashVersion int `json:"hash_version,omitempty"`
}

type SuppressionScope string

const (
	SuppressionGlobal         SuppressionScope = "global"
	SuppressionChannel        SuppressionScope = "channel"
	SuppressionPurposeChannel SuppressionScope = "purpose_channel"
)

type Suppression struct {
	ID            string           `json:"id"`
	TenantID      string           `json:"tenant_id"`
	SubjectID     string           `json:"subject_id"`
	Scope         SuppressionScope `json:"scope"`
	Purpose       string           `json:"purpose,omitempty"`
	Channel       string           `json:"channel,omitempty"`
	Reason        string           `json:"reason"`
	Active        bool             `json:"active"`
	CreatedByRole string           `json:"created_by_role,omitempty"`
	Version       int64            `json:"version"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

type EvaluationRequest struct {
	SubjectID string     `json:"subject_id"`
	Purpose   string     `json:"purpose"`
	Channel   string     `json:"channel"`
	At        *time.Time `json:"at,omitempty"`
}

type DoubleOptInChallenge struct {
	Consent           Consent `json:"consent"`
	ConfirmationToken string  `json:"-"`
}

// DoubleOptInResponse is the opaque API response returned when a
// double opt-in challenge is initiated (T-003). It strictly omits the
// confirmation token so callers cannot echo it immediately to bypass verification.
type DoubleOptInResponse struct {
	ChallengeID string     `json:"challenge_id"`
	Status      Status     `json:"status"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Consent     Consent    `json:"consent"`
}

// ToResponse converts the internal challenge representation to the opaque
// public API response DTO.
func (c DoubleOptInChallenge) ToResponse() DoubleOptInResponse {
	return DoubleOptInResponse{
		ChallengeID: c.Consent.ID,
		Status:      c.Consent.Status,
		ExpiresAt:   c.Consent.ConfirmationExpiresAt,
		Consent:     c.Consent,
	}
}

type AuditEvent struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenant_id"`
	ActorID       string         `json:"actor_id"`
	Action        string         `json:"action"`
	ConsentID     string         `json:"consent_id"`
	ResourceType  string         `json:"resource_type,omitempty"`
	ResourceID    string         `json:"resource_id,omitempty"`
	Revision      int64          `json:"revision,omitempty"`
	BeforeState   map[string]any `json:"before_state,omitempty"`
	AfterState    map[string]any `json:"after_state,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	Changes       map[string]any `json:"changes,omitempty"`
	OccurredAt    time.Time      `json:"occurred_at"`
}

type CorrelationKey struct{}

// WithCorrelationID stores a correlation identifier in the context.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, CorrelationKey{}, id)
}

// CorrelationID retrieves the correlation identifier from context, or empty string if not set.
func CorrelationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(CorrelationKey{}).(string)
	return id
}

type OutboxEvent struct {
	ID                string         `json:"id"`
	SchemaVersion     int            `json:"schema_version"`
	Source            string         `json:"source"`
	TenantID          string         `json:"tenant_id"`
	EventType         string         `json:"event_type"`
	AggregateType     string         `json:"aggregate_type"`
	AggregateID       string         `json:"aggregate_id"`
	AggregateRevision int64          `json:"aggregate_revision"`
	Payload           map[string]any `json:"payload"`
	OccurredAt        time.Time      `json:"occurred_at"`
	ClaimToken        string         `json:"claim_token,omitempty"`
}

// T-030: Role authority and protection helpers.

// IsAdministrativeRole reports whether role is an administrative authority
// (privacy_admin, consent_admin, developer, or auditor).
func IsAdministrativeRole(role string) bool {
	switch role {
	case "privacy_admin", "consent_admin", "developer", "auditor":
		return true
	default:
		return false
	}
}

// HasAdminAuthority reports whether role has administrative write authority,
// allowing it to manage or revoke administrative suppressions.
func HasAdminAuthority(role string) bool {
	switch role {
	case "privacy_admin", "consent_admin", "developer", "consent_writer":
		return true
	default:
		return false
	}
}

// HighestPrivilegedRole ranks roles by write authority and selects the highest.
func HighestPrivilegedRole(roles []string) string {
	roleRank := map[string]int{
		"privacy_admin":  100,
		"consent_admin":  90,
		"developer":      80,
		"consent_writer": 70,
		"self_service":   50,
		"service":        20,
		"consent_reader": 10,
	}
	bestRole := ""
	bestRank := 0
	for _, r := range roles {
		rank := roleRank[r]
		if rank > bestRank {
			bestRank = rank
			bestRole = r
		}
	}
	if bestRole == "" && len(roles) > 0 {
		return roles[0]
	}
	return bestRole
}
