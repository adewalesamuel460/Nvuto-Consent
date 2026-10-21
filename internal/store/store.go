package store

import (
	"context"
	"errors"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
)

// Store errors are the typed domain sentinels, so the HTTP layer maps them to
// stable public statuses (T-013) and errors.Is keeps working everywhere.
var (
	ErrNotFound             = domain.ErrNotFound
	ErrConflict             = domain.ErrConflict
	ErrExpired              = domain.ErrExpired
	ErrUnavailable          = domain.ErrUnavailable
	ErrSuppressionProtected = domain.ErrSuppressionProtected
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

// live returns a classified error once ctx is done. Every store method
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
	ListConsents(ctx context.Context, tenantID, subjectID string, limit, offset int) ([]domain.Consent, int, error)
	PutPurpose(ctx context.Context, purpose domain.Purpose, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Purpose, error)
	GetPurpose(ctx context.Context, tenantID, code string) (domain.Purpose, error)
	ListPurposes(ctx context.Context, tenantID string, limit, offset int) ([]domain.Purpose, int, error)
	PutLawfulBasis(ctx context.Context, basis domain.LawfulBasis, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.LawfulBasis, error)
	GetLawfulBasis(ctx context.Context, tenantID, code string) (domain.LawfulBasis, error)
	ListLawfulBases(ctx context.Context, tenantID string, limit, offset int) ([]domain.LawfulBasis, int, error)
	PutNotice(ctx context.Context, notice domain.PrivacyNotice, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.PrivacyNotice, error)
	GetNotice(ctx context.Context, tenantID, jurisdiction, version string) (domain.PrivacyNotice, error)
	ListNotices(ctx context.Context, tenantID string, limit, offset int) ([]domain.PrivacyNotice, int, error)
	RetireNotice(ctx context.Context, tenantID, jurisdiction, version, supersededBy string, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.PrivacyNotice, error)
	RecordGoverned(ctx context.Context, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error)
	Withdraw(ctx context.Context, consent domain.Consent, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error)
	GetReceipt(ctx context.Context, tenantID, id string) (domain.ConsentReceipt, error)
	SetSuppression(ctx context.Context, suppression domain.Suppression, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Suppression, error)
	FindSuppression(ctx context.Context, tenantID, subjectID, purpose, channel string) (domain.Suppression, error)
	ListSuppressions(ctx context.Context, tenantID, subjectID string, limit, offset int) ([]domain.Suppression, int, error)
	StartDoubleOptIn(ctx context.Context, consent domain.Consent, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error)
	FindPendingByToken(ctx context.Context, tenantID, tokenHash string) (domain.Consent, error)
	ConfirmDoubleOptIn(ctx context.Context, tenantID, tokenHash string, at time.Time, finalize ConfirmFinalizer) (domain.Consent, error)
	AppendAudit(ctx context.Context, event domain.AuditEvent) error
	AppendOutbox(ctx context.Context, event domain.OutboxEvent) error
	Ping(ctx context.Context) error
	VerifySchema(ctx context.Context) error
}
