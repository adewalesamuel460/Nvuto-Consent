package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultStatementTimeout is a server-side backstop for any statement whose
// client context fails to bound it. Request contexts normally end work much
// earlier. A DSN that sets statement_timeout explicitly takes precedence.
const defaultStatementTimeout = "15s"

type Postgres struct {
	pool        *pgxpool.Pool
	outboxLease time.Duration
	txRetry     retryPolicy
}

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 25
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 45 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	if _, ok := cfg.ConnConfig.RuntimeParams["statement_timeout"]; !ok {
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = defaultStatementTimeout
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool, outboxLease: OutboxLeaseDuration, txRetry: defaultTxRetry}, nil
}

// SetOutboxLease overrides the outbox lease for claims made through this
// store. It exists so integration tests can let a real lease expire; every
// worker sharing a database must use the same lease.
func (p *Postgres) SetOutboxLease(lease time.Duration) {
	if lease > 0 {
		p.outboxLease = lease
	}
}

// SetTxRetry overrides the transaction retry policy for testing or tuning.
func (p *Postgres) SetTxRetry(attempts int, base, max time.Duration) {
	p.txRetry = retryPolicy{attempts: attempts, base: base, max: max}
}

func (p *Postgres) Close() { p.pool.Close() }

// Pool exposes the underlying connection pool for database-level inspection and integration tests.
func (p *Postgres) Pool() *pgxpool.Pool { return p.pool }

// Ping checks database reachability within the caller's context, capped so
// a readiness probe never hangs on a stalled server.
func (p *Postgres) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return classify(p.pool.Ping(ctx))
}

// VerifySchema verifies that critical database tables from the migrations suite
// (e.g. consent_suppressions and consents) exist and are accessible.
func (p *Postgres) VerifySchema(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var exists bool
	err := p.pool.QueryRow(ctx, `SELECT to_regclass('consent_suppressions') IS NOT NULL AND to_regclass('consents') IS NOT NULL`).Scan(&exists)
	if err != nil {
		return classify(err)
	}
	if !exists {
		return domain.ErrUnavailable.Derive("schema_uninitialized", "required database migrations have not been applied")
	}
	return nil
}

// inTx runs fn in one transaction on the caller's context. Cancellation or
// deadline expiry aborts the in-flight statement, rolls back and releases the
// connection. Transient server aborts (40001/40P01) are retried within the
// retry policy and the context deadline. fn must therefore be re-runnable:
// it may only mutate state through tx and must recompute everything it
// derives from reads. Returning errNoChange rolls back without failing.
func (p *Postgres) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	err := withTxRetry(ctx, p.txRetry, func(ctx context.Context) error { return p.txOnce(ctx, fn) })
	if errors.Is(err, errNoChange) {
		return errNoChange
	}
	return classify(err)
}

func (p *Postgres) txOnce(ctx context.Context, fn func(pgx.Tx) error) (err error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			rollback(ctx, tx)
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type rowScanner interface{ Scan(...any) error }

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const consentColumns = `id,tenant_id,subject_id,purpose,channel,legal_basis,status,source,scope,jurisdiction,notice_version,receipt_id,evidence,version,granted_at,denied_at,withdrawn_at,expires_at,confirmation_token_hash,confirmation_expires_at,confirmed_at,created_at,updated_at,preference_generation`

func scanConsent(row rowScanner) (domain.Consent, error) {
	var consent domain.Consent
	var status string
	var scope, evidence []byte
	err := row.Scan(
		&consent.ID, &consent.TenantID, &consent.SubjectID, &consent.Purpose, &consent.Channel,
		&consent.LegalBasis, &status, &consent.Source, &scope, &consent.Jurisdiction,
		&consent.NoticeVersion, &consent.ReceiptID, &evidence, &consent.Version, &consent.GrantedAt,
		&consent.DeniedAt, &consent.WithdrawnAt, &consent.ExpiresAt, &consent.ConfirmationTokenHash,
		&consent.ConfirmationExpiresAt, &consent.ConfirmedAt, &consent.CreatedAt, &consent.UpdatedAt,
		&consent.PreferenceGeneration,
	)
	if err != nil {
		return domain.Consent{}, classify(err)
	}
	consent.Status = domain.Status(status)
	_ = json.Unmarshal(scope, &consent.Scope)
	_ = json.Unmarshal(evidence, &consent.Evidence)
	return consent, nil
}

func insertConsent(ctx context.Context, tx pgx.Tx, consent domain.Consent) error {
	scope, _ := json.Marshal(consent.Scope)
	evidence, _ := json.Marshal(consent.Evidence)
	_, err := tx.Exec(ctx, `INSERT INTO consents(`+consentColumns+`)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
		consent.ID, consent.TenantID, consent.SubjectID, consent.Purpose, consent.Channel, consent.LegalBasis,
		string(consent.Status), consent.Source, scope, consent.Jurisdiction, consent.NoticeVersion, consent.ReceiptID,
		evidence, consent.Version, consent.GrantedAt, consent.DeniedAt, consent.WithdrawnAt, consent.ExpiresAt,
		consent.ConfirmationTokenHash, consent.ConfirmationExpiresAt, consent.ConfirmedAt, consent.CreatedAt, consent.UpdatedAt,
		consent.PreferenceGeneration)
	return classify(err)
}

// advanceGenerationTx increments the preference generation for the consent's
// (tenant, subject, purpose, channel) stream and invalidates every pending
// challenge bound to an older generation, all inside tx. The upsert takes the
// row lock on consent_preference_state that serializes every intent change and
// confirmation for that stream until tx ends.
func advanceGenerationTx(ctx context.Context, tx pgx.Tx, c domain.Consent, at time.Time) (int64, error) {
	var generation int64
	err := tx.QueryRow(ctx, `INSERT INTO consent_preference_state(tenant_id,subject_id,purpose,channel,generation,updated_at)
		VALUES($1,$2,$3,$4,1,$5)
		ON CONFLICT(tenant_id,subject_id,purpose,channel) DO UPDATE
		SET generation=consent_preference_state.generation+1,updated_at=excluded.updated_at
		RETURNING generation`, c.TenantID, c.SubjectID, c.Purpose, c.Channel, at).Scan(&generation)
	if err != nil {
		return 0, classify(err)
	}
	_, err = tx.Exec(ctx, `UPDATE consents SET confirmation_token_hash='',version=version+1
		WHERE tenant_id=$1 AND subject_id=$2 AND purpose=$3 AND channel=$4
		AND status='pending_confirmation' AND confirmation_token_hash<>'' AND preference_generation<$5`,
		c.TenantID, c.SubjectID, c.Purpose, c.Channel, generation)
	return generation, classify(err)
}

func appendAuditTx(ctx context.Context, tx pgx.Tx, event domain.AuditEvent) error {
	resourceType := event.ResourceType
	if resourceType == "" {
		resourceType = "consent"
	}
	revision := event.Revision
	if revision <= 0 {
		revision = 1
	}
	changes := event.Changes
	if changes == nil {
		changes = map[string]any{}
	}
	changesJSON, _ := json.Marshal(changes)
	_, err := tx.Exec(ctx, `INSERT INTO consent_audit_events(id,tenant_id,actor_id,action,consent_id,occurred_at,resource_type,revision,changes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, event.ID, event.TenantID, event.ActorID, event.Action, event.ConsentID, event.OccurredAt, resourceType, revision, changesJSON)
	return classify(err)
}

func appendOutboxTx(ctx context.Context, tx pgx.Tx, event domain.OutboxEvent) error {
	event = normalizeOutbox(event)
	payload, _ := json.Marshal(event.Payload)
	_, err := tx.Exec(ctx, `INSERT INTO consent_outbox_events(id,schema_version,source,tenant_id,event_type,aggregate_type,aggregate_id,payload,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, event.ID, event.SchemaVersion, event.Source, event.TenantID, event.EventType, event.AggregateType, event.AggregateID, payload, event.OccurredAt)
	return classify(err)
}

func normalizeOutbox(event domain.OutboxEvent) domain.OutboxEvent {
	if event.SchemaVersion == 0 {
		event.SchemaVersion = 1
	}
	if event.Source == "" {
		event.Source = "nvuto/consent"
	}
	if event.AggregateType == "" {
		event.AggregateType = "consent"
	}
	return event
}

func insertReceipt(ctx context.Context, tx pgx.Tx, receipt domain.ConsentReceipt) error {
	evidence, _ := json.Marshal(receipt.Evidence)
	hashVersion := receipt.HashVersion
	if hashVersion == 0 {
		hashVersion = 1 // unversioned receipts are, by definition, legacy format
	}
	policyVersion := receipt.PolicyVersion
	if policyVersion <= 0 {
		policyVersion = 1
	}
	_, err := tx.Exec(ctx, `INSERT INTO consent_receipts(id,tenant_id,consent_id,subject_id,purpose,channel,notice_version,jurisdiction,evidence,receipt_hash,issued_at,hash_version,actor_id,consent_status,legal_basis,notice_content_hash,policy_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, receipt.ID, receipt.TenantID, receipt.ConsentID, receipt.SubjectID, receipt.Purpose, receipt.Channel, receipt.NoticeVersion, receipt.Jurisdiction, evidence, receipt.ReceiptHash, receipt.IssuedAt, hashVersion, receipt.ActorID, string(receipt.ConsentStatus), receipt.LegalBasis, receipt.NoticeContentHash, policyVersion)
	return classify(err)
}

// appendEffectsTx writes the audit and outbox records that accompany every
// state change, in the same transaction as the change itself.
func appendEffectsTx(ctx context.Context, tx pgx.Tx, audit domain.AuditEvent, outbox domain.OutboxEvent) error {
	if err := appendAuditTx(ctx, tx, audit); err != nil {
		return err
	}
	return appendOutboxTx(ctx, tx, outbox)
}

func (p *Postgres) Get(ctx context.Context, tenantID, id string) (domain.Consent, error) {
	return scanConsent(p.pool.QueryRow(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND id=$2`, tenantID, id))
}

// FindEffective returns the record carrying the highest preference generation;
// updated_at only breaks ties (pre-migration records all have generation 0), so
// wall-clock skew between writers can no longer reorder intent.
func (p *Postgres) FindEffective(ctx context.Context, tenantID, subjectID, purpose, channel string) (domain.Consent, error) {
	return scanConsent(p.pool.QueryRow(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND subject_id=$2 AND purpose=$3 AND channel=$4 ORDER BY preference_generation DESC, updated_at DESC, id DESC LIMIT 1`, tenantID, subjectID, purpose, channel))
}

func (p *Postgres) ListConsents(ctx context.Context, tenantID, subjectID string, limit, offset int) ([]domain.Consent, int, error) {
	var total int
	err := p.pool.QueryRow(ctx, `SELECT count(*) FROM consents WHERE tenant_id=$1 AND subject_id=$2`, tenantID, subjectID).Scan(&total)
	if err != nil {
		return nil, 0, classify(err)
	}
	if total == 0 || offset >= total {
		return []domain.Consent{}, total, nil
	}
	rows, err := p.pool.Query(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND subject_id=$2 ORDER BY preference_generation DESC, updated_at DESC, id DESC LIMIT $3 OFFSET $4`, tenantID, subjectID, limit, offset)
	if err != nil {
		return nil, 0, classify(err)
	}
	defer rows.Close()
	items := make([]domain.Consent, 0, limit)
	for rows.Next() {
		item, err := scanConsent(rows)
		if err != nil {
			return nil, 0, classify(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, classify(err)
	}
	return items, total, nil
}

func (p *Postgres) PutPurpose(ctx context.Context, purpose domain.Purpose, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Purpose, error) {
	channels, _ := json.Marshal(purpose.AllowedChannels)
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		var before any
		prev, err := getPurpose(ctx, tx, purpose.TenantID, purpose.Code, " FOR UPDATE")
		if err == nil {
			before = prev
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		err = tx.QueryRow(ctx, `INSERT INTO consent_purposes(tenant_id,code,category,allowed_channels,requires_affirmative_consent,requires_double_opt_in,active,version,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,1,$8)
		ON CONFLICT(tenant_id,code) DO UPDATE SET category=excluded.category,allowed_channels=excluded.allowed_channels,requires_affirmative_consent=excluded.requires_affirmative_consent,requires_double_opt_in=excluded.requires_double_opt_in,active=excluded.active,version=consent_purposes.version+1,updated_at=excluded.updated_at RETURNING version`, purpose.TenantID, purpose.Code, purpose.Category, channels, purpose.RequiresAffirmativeConsent, purpose.RequiresDoubleOptIn, purpose.Active, purpose.UpdatedAt).Scan(&purpose.Version)
		if err != nil {
			return classify(err)
		}
		if audit.ResourceType == "" {
			audit.ResourceType = "purpose"
		}
		if audit.ConsentID == "" {
			audit.ConsentID = purpose.Code
		}
		audit.Revision = purpose.Version
		audit.Changes = map[string]any{"before": before, "after": purpose}
		return appendEffectsTx(ctx, tx, audit, outbox)
	})
	if err != nil {
		return domain.Purpose{}, err
	}
	return purpose, nil
}

// The policy readers accept a lock clause so the confirmation transaction can
// read with FOR SHARE: a concurrent deactivation then cannot commit between the
// policy check and the grant.
func getPurpose(ctx context.Context, q querier, tenantID, code, lock string) (domain.Purpose, error) {
	var purpose domain.Purpose
	var channels []byte
	err := q.QueryRow(ctx, `SELECT tenant_id,code,category,allowed_channels,requires_affirmative_consent,requires_double_opt_in,active,version,updated_at FROM consent_purposes WHERE tenant_id=$1 AND code=$2`+lock, tenantID, code).Scan(&purpose.TenantID, &purpose.Code, &purpose.Category, &channels, &purpose.RequiresAffirmativeConsent, &purpose.RequiresDoubleOptIn, &purpose.Active, &purpose.Version, &purpose.UpdatedAt)
	if err != nil {
		return domain.Purpose{}, classify(err)
	}
	_ = json.Unmarshal(channels, &purpose.AllowedChannels)
	return purpose, nil
}

func getLawfulBasis(ctx context.Context, q querier, tenantID, code, lock string) (domain.LawfulBasis, error) {
	var basis domain.LawfulBasis
	var categories []byte
	err := q.QueryRow(ctx, `SELECT tenant_id,code,allowed_categories,requires_evidence,active,version,updated_at FROM consent_lawful_bases WHERE tenant_id=$1 AND code=$2`+lock, tenantID, code).Scan(&basis.TenantID, &basis.Code, &categories, &basis.RequiresEvidence, &basis.Active, &basis.Version, &basis.UpdatedAt)
	if err != nil {
		return domain.LawfulBasis{}, classify(err)
	}
	_ = json.Unmarshal(categories, &basis.AllowedCategories)
	return basis, nil
}

func getNotice(ctx context.Context, q querier, tenantID, jurisdiction, version, lock string) (domain.PrivacyNotice, error) {
	var notice domain.PrivacyNotice
	err := q.QueryRow(ctx, `SELECT tenant_id,version,jurisdiction,published_at,active,content,content_hash,retired,superseded_by FROM consent_privacy_notices WHERE tenant_id=$1 AND jurisdiction=$2 AND version=$3`+lock, tenantID, jurisdiction, version).Scan(&notice.TenantID, &notice.Version, &notice.Jurisdiction, &notice.PublishedAt, &notice.Active, &notice.Content, &notice.ContentHash, &notice.Retired, &notice.SupersededBy)
	if err != nil {
		return domain.PrivacyNotice{}, classify(err)
	}
	return notice, nil
}

func (p *Postgres) GetPurpose(ctx context.Context, tenantID, code string) (domain.Purpose, error) {
	return getPurpose(ctx, p.pool, tenantID, code, "")
}

func (p *Postgres) ListPurposes(ctx context.Context, tenantID string, limit, offset int) ([]domain.Purpose, int, error) {
	var total int
	err := p.pool.QueryRow(ctx, `SELECT count(*) FROM consent_purposes WHERE tenant_id=$1`, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, classify(err)
	}
	if total == 0 || offset >= total {
		return []domain.Purpose{}, total, nil
	}
	rows, err := p.pool.Query(ctx, `SELECT tenant_id,code,category,allowed_channels,requires_affirmative_consent,requires_double_opt_in,active,version,updated_at FROM consent_purposes WHERE tenant_id=$1 ORDER BY code ASC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, classify(err)
	}
	defer rows.Close()
	items := make([]domain.Purpose, 0, limit)
	for rows.Next() {
		var purpose domain.Purpose
		var channels []byte
		if err := rows.Scan(&purpose.TenantID, &purpose.Code, &purpose.Category, &channels, &purpose.RequiresAffirmativeConsent, &purpose.RequiresDoubleOptIn, &purpose.Active, &purpose.Version, &purpose.UpdatedAt); err != nil {
			return nil, 0, classify(err)
		}
		_ = json.Unmarshal(channels, &purpose.AllowedChannels)
		items = append(items, purpose)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, classify(err)
	}
	return items, total, nil
}

func (p *Postgres) PutLawfulBasis(ctx context.Context, basis domain.LawfulBasis, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.LawfulBasis, error) {
	categories, _ := json.Marshal(basis.AllowedCategories)
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		var before any
		prev, err := getLawfulBasis(ctx, tx, basis.TenantID, basis.Code, " FOR UPDATE")
		if err == nil {
			before = prev
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		err = tx.QueryRow(ctx, `INSERT INTO consent_lawful_bases(tenant_id,code,allowed_categories,requires_evidence,active,version,updated_at) VALUES($1,$2,$3,$4,$5,1,$6)
		ON CONFLICT(tenant_id,code) DO UPDATE SET allowed_categories=excluded.allowed_categories,requires_evidence=excluded.requires_evidence,active=excluded.active,version=consent_lawful_bases.version+1,updated_at=excluded.updated_at RETURNING version`, basis.TenantID, basis.Code, categories, basis.RequiresEvidence, basis.Active, basis.UpdatedAt).Scan(&basis.Version)
		if err != nil {
			return classify(err)
		}
		if audit.ResourceType == "" {
			audit.ResourceType = "lawful_basis"
		}
		if audit.ConsentID == "" {
			audit.ConsentID = basis.Code
		}
		audit.Revision = basis.Version
		audit.Changes = map[string]any{"before": before, "after": basis}
		return appendEffectsTx(ctx, tx, audit, outbox)
	})
	if err != nil {
		return domain.LawfulBasis{}, err
	}
	return basis, nil
}

func (p *Postgres) GetLawfulBasis(ctx context.Context, tenantID, code string) (domain.LawfulBasis, error) {
	return getLawfulBasis(ctx, p.pool, tenantID, code, "")
}

func (p *Postgres) ListLawfulBases(ctx context.Context, tenantID string, limit, offset int) ([]domain.LawfulBasis, int, error) {
	var total int
	err := p.pool.QueryRow(ctx, `SELECT count(*) FROM consent_lawful_bases WHERE tenant_id=$1`, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, classify(err)
	}
	if total == 0 || offset >= total {
		return []domain.LawfulBasis{}, total, nil
	}
	rows, err := p.pool.Query(ctx, `SELECT tenant_id,code,allowed_categories,requires_evidence,active,version,updated_at FROM consent_lawful_bases WHERE tenant_id=$1 ORDER BY code ASC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, classify(err)
	}
	defer rows.Close()
	items := make([]domain.LawfulBasis, 0, limit)
	for rows.Next() {
		var basis domain.LawfulBasis
		var categories []byte
		if err := rows.Scan(&basis.TenantID, &basis.Code, &categories, &basis.RequiresEvidence, &basis.Active, &basis.Version, &basis.UpdatedAt); err != nil {
			return nil, 0, classify(err)
		}
		_ = json.Unmarshal(categories, &basis.AllowedCategories)
		items = append(items, basis)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, classify(err)
	}
	return items, total, nil
}

func (p *Postgres) PutNotice(ctx context.Context, notice domain.PrivacyNotice, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.PrivacyNotice, error) {
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO consent_privacy_notices(tenant_id,jurisdiction,version,published_at,active,content,content_hash,retired,superseded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, notice.TenantID, notice.Jurisdiction, notice.Version, notice.PublishedAt, notice.Active, notice.Content, notice.ContentHash, notice.Retired, notice.SupersededBy); err != nil {
			return classify(err)
		}
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
		return appendEffectsTx(ctx, tx, audit, outbox)
	})
	if err != nil {
		return domain.PrivacyNotice{}, err
	}
	return notice, nil
}

func (p *Postgres) GetNotice(ctx context.Context, tenantID, jurisdiction, version string) (domain.PrivacyNotice, error) {
	return getNotice(ctx, p.pool, tenantID, jurisdiction, version, "")
}

func (p *Postgres) ListNotices(ctx context.Context, tenantID string, limit, offset int) ([]domain.PrivacyNotice, int, error) {
	var total int
	err := p.pool.QueryRow(ctx, `SELECT count(*) FROM consent_privacy_notices WHERE tenant_id=$1`, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, classify(err)
	}
	if total == 0 || offset >= total {
		return []domain.PrivacyNotice{}, total, nil
	}
	rows, err := p.pool.Query(ctx, `SELECT tenant_id,version,jurisdiction,published_at,active,content,content_hash,retired,superseded_by FROM consent_privacy_notices WHERE tenant_id=$1 ORDER BY published_at DESC, jurisdiction ASC, version DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, classify(err)
	}
	defer rows.Close()
	items := make([]domain.PrivacyNotice, 0, limit)
	for rows.Next() {
		var notice domain.PrivacyNotice
		if err := rows.Scan(&notice.TenantID, &notice.Version, &notice.Jurisdiction, &notice.PublishedAt, &notice.Active, &notice.Content, &notice.ContentHash, &notice.Retired, &notice.SupersededBy); err != nil {
			return nil, 0, classify(err)
		}
		items = append(items, notice)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, classify(err)
	}
	return items, total, nil
}

func (p *Postgres) RetireNotice(ctx context.Context, tenantID, jurisdiction, version, supersededBy string, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.PrivacyNotice, error) {
	var notice domain.PrivacyNotice
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		var before domain.PrivacyNotice
		err := tx.QueryRow(ctx, `SELECT tenant_id,version,jurisdiction,published_at,active,content,content_hash,retired,superseded_by FROM consent_privacy_notices WHERE tenant_id=$1 AND jurisdiction=$2 AND version=$3 FOR UPDATE`, tenantID, jurisdiction, version).Scan(&before.TenantID, &before.Version, &before.Jurisdiction, &before.PublishedAt, &before.Active, &before.Content, &before.ContentHash, &before.Retired, &before.SupersededBy)
		if err != nil {
			return classify(err)
		}
		notice = before
		notice.Active = false
		notice.Retired = true
		notice.SupersededBy = supersededBy
		tag, err := tx.Exec(ctx, `UPDATE consent_privacy_notices SET active=false, retired=true, superseded_by=$4 WHERE tenant_id=$1 AND jurisdiction=$2 AND version=$3`, tenantID, jurisdiction, version, supersededBy)
		if err != nil {
			return classify(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if audit.ResourceType == "" {
			audit.ResourceType = "privacy_notice"
		}
		if audit.ConsentID == "" {
			audit.ConsentID = notice.Jurisdiction + "/" + notice.Version
		}
		if audit.Revision <= 0 {
			audit.Revision = 2
		}
		audit.Changes = map[string]any{"before": before, "after": notice}
		return appendEffectsTx(ctx, tx, audit, outbox)
	})
	if err != nil {
		return domain.PrivacyNotice{}, err
	}
	return notice, nil
}

// pgPolicyReader reads policy inside the confirmation transaction with shared
// row locks.
type pgPolicyReader struct{ tx pgx.Tx }

func (r pgPolicyReader) GetPurpose(ctx context.Context, tenantID, code string) (domain.Purpose, error) {
	return getPurpose(ctx, r.tx, tenantID, code, " FOR SHARE")
}

func (r pgPolicyReader) GetLawfulBasis(ctx context.Context, tenantID, code string) (domain.LawfulBasis, error) {
	return getLawfulBasis(ctx, r.tx, tenantID, code, " FOR SHARE")
}

func (r pgPolicyReader) GetNotice(ctx context.Context, tenantID, jurisdiction, version string) (domain.PrivacyNotice, error) {
	return getNotice(ctx, r.tx, tenantID, jurisdiction, version, " FOR SHARE")
}

// recordGovernedTx advances the stream generation and writes the consent,
// optional receipt, audit and outbox records. It returns the stored consent.
func recordGovernedTx(ctx context.Context, tx pgx.Tx, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	generation, err := advanceGenerationTx(ctx, tx, consent, consent.UpdatedAt)
	if err != nil {
		return domain.Consent{}, err
	}
	consent.PreferenceGeneration = generation
	if err := insertConsent(ctx, tx, consent); err != nil {
		return domain.Consent{}, err
	}
	if receipt.ID != "" {
		if err := insertReceipt(ctx, tx, receipt); err != nil {
			return domain.Consent{}, err
		}
	}
	if audit.ResourceType == "" {
		audit.ResourceType = "consent"
	}
	if audit.Revision <= 0 {
		audit.Revision = 1
	}
	if audit.Changes == nil {
		audit.Changes = map[string]any{"before": nil, "after": consent}
	}
	if outbox.Payload != nil {
		outbox.Payload["generation"] = generation
		outbox.Payload["preference_generation"] = generation
	}
	if err := appendEffectsTx(ctx, tx, audit, outbox); err != nil {
		return domain.Consent{}, err
	}
	return consent, nil
}

func (p *Postgres) RecordGoverned(ctx context.Context, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	var stored domain.Consent
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		stored, err = recordGovernedTx(ctx, tx, consent, receipt, audit, outbox)
		return err
	})
	if err != nil {
		return domain.Consent{}, err
	}
	return stored, nil
}

// Withdraw records the newest intent for the consent's preference stream: it
// advances the generation and stamps the withdrawn record with it, so the
// withdrawal is effective and every older pending challenge is invalidated.
func (p *Postgres) Withdraw(ctx context.Context, consent domain.Consent, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	var result domain.Consent
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		// Lock order is preference state -> consent row, matching every other
		// writer, so read the stream key without a lock first.
		probe, err := scanConsent(tx.QueryRow(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND id=$2`, consent.TenantID, consent.ID))
		if err != nil {
			return err
		}
		generation, err := advanceGenerationTx(ctx, tx, probe, consent.UpdatedAt)
		if err != nil {
			return err
		}
		current, err := scanConsent(tx.QueryRow(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, consent.TenantID, consent.ID))
		if err != nil {
			return err
		}
		if current.Status == domain.StatusWithdrawn {
			result = current
			return errNoChange // rollback discards the generation advance
		}
		updated := consent
		updated.Version = current.Version + 1
		updated.ConfirmationTokenHash = ""
		updated.PreferenceGeneration = generation
		if _, err := tx.Exec(ctx, `UPDATE consents SET status='withdrawn',withdrawn_at=$3,version=$4,updated_at=$5,confirmation_token_hash='',preference_generation=$6 WHERE tenant_id=$1 AND id=$2`, updated.TenantID, updated.ID, updated.WithdrawnAt, updated.Version, updated.UpdatedAt, generation); err != nil {
			return classify(err)
		}
		if audit.ResourceType == "" {
			audit.ResourceType = "consent"
		}
		audit.ConsentID = updated.ID
		audit.Revision = updated.Version
		audit.Changes = map[string]any{"before": current, "after": updated}
		if err := appendEffectsTx(ctx, tx, audit, outbox); err != nil {
			return err
		}
		result = updated
		return nil
	})
	if err != nil && !errors.Is(err, errNoChange) {
		return domain.Consent{}, err
	}
	return result, nil
}

func (p *Postgres) GetReceipt(ctx context.Context, tenantID, id string) (domain.ConsentReceipt, error) {
	var receipt domain.ConsentReceipt
	var evidence []byte
	var hashVersion int16
	var consentStatus string
	err := p.pool.QueryRow(ctx, `SELECT id,tenant_id,consent_id,subject_id,purpose,channel,notice_version,jurisdiction,evidence,receipt_hash,issued_at,hash_version,actor_id,consent_status,legal_basis,notice_content_hash,policy_version FROM consent_receipts WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&receipt.ID, &receipt.TenantID, &receipt.ConsentID, &receipt.SubjectID, &receipt.Purpose, &receipt.Channel, &receipt.NoticeVersion, &receipt.Jurisdiction, &evidence, &receipt.ReceiptHash, &receipt.IssuedAt, &hashVersion, &receipt.ActorID, &consentStatus, &receipt.LegalBasis, &receipt.NoticeContentHash, &receipt.PolicyVersion)
	if err != nil {
		return domain.ConsentReceipt{}, classify(err)
	}
	_ = json.Unmarshal(evidence, &receipt.Evidence)
	receipt.ConsentStatus = domain.Status(consentStatus)
	receipt.HashVersion = int(hashVersion)
	receipt.IssuedAt = receipt.IssuedAt.UTC()
	return receipt, nil
}

func (p *Postgres) SetSuppression(ctx context.Context, suppression domain.Suppression, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Suppression, error) {
	var stored domain.Suppression
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		var before any
		var prev domain.Suppression
		var prevScope string
		err := tx.QueryRow(ctx, `SELECT id,tenant_id,subject_id,scope,purpose,channel,reason,active,created_by_role,version,updated_at FROM consent_suppressions WHERE tenant_id=$1 AND subject_id=$2 AND scope=$3 AND purpose=$4 AND channel=$5 FOR UPDATE`, suppression.TenantID, suppression.SubjectID, string(suppression.Scope), suppression.Purpose, suppression.Channel).Scan(&prev.ID, &prev.TenantID, &prev.SubjectID, &prevScope, &prev.Purpose, &prev.Channel, &prev.Reason, &prev.Active, &prev.CreatedByRole, &prev.Version, &prev.UpdatedAt)
		if err == nil {
			prev.Scope = domain.SuppressionScope(prevScope)
			before = prev
			// T-030: Protect administrative suppressions from revocation by self_service.
			if prev.Active && !suppression.Active && domain.IsAdministrativeRole(prev.CreatedByRole) && !domain.HasAdminAuthority(suppression.CreatedByRole) {
				return domain.ErrSuppressionProtected
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return classify(err)
		}

		stored = suppression
		err = tx.QueryRow(ctx, `INSERT INTO consent_suppressions(id,tenant_id,subject_id,scope,purpose,channel,reason,active,created_by_role,version,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,1,$10)
		ON CONFLICT(tenant_id,subject_id,scope,purpose,channel) DO UPDATE SET
			reason=excluded.reason,
			active=excluded.active,
			created_by_role=CASE WHEN consent_suppressions.created_by_role <> '' THEN consent_suppressions.created_by_role ELSE excluded.created_by_role END,
			version=consent_suppressions.version+1,
			updated_at=excluded.updated_at
		RETURNING id,created_by_role,version`, suppression.ID, suppression.TenantID, suppression.SubjectID, string(suppression.Scope), suppression.Purpose, suppression.Channel, suppression.Reason, suppression.Active, suppression.CreatedByRole, suppression.UpdatedAt).Scan(&stored.ID, &stored.CreatedByRole, &stored.Version)
		if err != nil {
			return classify(err)
		}
		audit, outbox := audit, outbox
		if audit.ResourceType == "" {
			audit.ResourceType = "suppression"
		}
		audit.ConsentID = stored.ID
		audit.Revision = stored.Version
		audit.Changes = map[string]any{"before": before, "after": stored}
		outbox.AggregateID = stored.ID
		return appendEffectsTx(ctx, tx, audit, outbox)
	})
	if err != nil {
		return domain.Suppression{}, err
	}
	return stored, nil
}

func (p *Postgres) FindSuppression(ctx context.Context, tenantID, subjectID, purpose, channel string) (domain.Suppression, error) {
	var suppression domain.Suppression
	var scope string
	err := p.pool.QueryRow(ctx, `SELECT id,tenant_id,subject_id,scope,purpose,channel,reason,active,created_by_role,version,updated_at FROM consent_suppressions
		WHERE tenant_id=$1 AND subject_id=$2 AND active=true AND (scope='global' OR (scope='channel' AND channel=$4) OR (scope='purpose_channel' AND purpose=$3 AND channel=$4))
		ORDER BY CASE scope WHEN 'global' THEN 3 WHEN 'channel' THEN 2 ELSE 1 END DESC, updated_at DESC, id DESC LIMIT 1`, tenantID, subjectID, purpose, channel).Scan(&suppression.ID, &suppression.TenantID, &suppression.SubjectID, &scope, &suppression.Purpose, &suppression.Channel, &suppression.Reason, &suppression.Active, &suppression.CreatedByRole, &suppression.Version, &suppression.UpdatedAt)
	if err != nil {
		return domain.Suppression{}, classify(err)
	}
	suppression.Scope = domain.SuppressionScope(scope)
	return suppression, nil
}

func (p *Postgres) ListSuppressions(ctx context.Context, tenantID, subjectID string, limit, offset int) ([]domain.Suppression, int, error) {
	var total int
	err := p.pool.QueryRow(ctx, `SELECT count(*) FROM consent_suppressions WHERE tenant_id=$1 AND subject_id=$2`, tenantID, subjectID).Scan(&total)
	if err != nil {
		return nil, 0, classify(err)
	}
	if total == 0 || offset >= total {
		return []domain.Suppression{}, total, nil
	}
	rows, err := p.pool.Query(ctx, `SELECT id,tenant_id,subject_id,scope,purpose,channel,reason,active,created_by_role,version,updated_at FROM consent_suppressions WHERE tenant_id=$1 AND subject_id=$2 ORDER BY updated_at DESC, id DESC LIMIT $3 OFFSET $4`, tenantID, subjectID, limit, offset)
	if err != nil {
		return nil, 0, classify(err)
	}
	defer rows.Close()
	items := make([]domain.Suppression, 0, limit)
	for rows.Next() {
		var sup domain.Suppression
		var scope string
		if err := rows.Scan(&sup.ID, &sup.TenantID, &sup.SubjectID, &scope, &sup.Purpose, &sup.Channel, &sup.Reason, &sup.Active, &sup.CreatedByRole, &sup.Version, &sup.UpdatedAt); err != nil {
			return nil, 0, classify(err)
		}
		sup.Scope = domain.SuppressionScope(scope)
		items = append(items, sup)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, classify(err)
	}
	return items, total, nil
}

// StartDoubleOptIn inserts the pending challenge bound to a freshly advanced
// generation, which atomically supersedes any earlier pending challenge.
func (p *Postgres) StartDoubleOptIn(ctx context.Context, consent domain.Consent, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	return p.RecordGoverned(ctx, consent, domain.ConsentReceipt{}, audit, outbox)
}

func (p *Postgres) FindPendingByToken(ctx context.Context, tenantID, tokenHash string) (domain.Consent, error) {
	if tokenHash == "" {
		return domain.Consent{}, ErrNotFound
	}
	return scanConsent(p.pool.QueryRow(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND confirmation_token_hash=$2 AND status='pending_confirmation'`, tenantID, tokenHash))
}

// ConfirmDoubleOptIn grants a pending challenge only if, inside one
// transaction holding the preference-state row lock and the challenge row
// lock, the challenge is pending, unexpired, bound to the current generation,
// and the finalizer accepts the policy read under that same transaction.
func (p *Postgres) ConfirmDoubleOptIn(ctx context.Context, tenantID, tokenHash string, at time.Time, finalize ConfirmFinalizer) (domain.Consent, error) {
	if tokenHash == "" || finalize == nil {
		return domain.Consent{}, ErrNotFound
	}
	var granted domain.Consent
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		// 1. Identify the challenge (unlocked) to learn its preference stream.
		probe, err := scanConsent(tx.QueryRow(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND confirmation_token_hash=$2`, tenantID, tokenHash))
		if err != nil {
			return err
		}
		// 2. Lock the preference state first (same order as every writer).
		var current int64
		err = tx.QueryRow(ctx, `SELECT generation FROM consent_preference_state WHERE tenant_id=$1 AND subject_id=$2 AND purpose=$3 AND channel=$4 FOR UPDATE`,
			probe.TenantID, probe.SubjectID, probe.Purpose, probe.Channel).Scan(&current)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return classify(err)
		}
		// 3. Lock and re-read the challenge; a concurrent writer may have
		// invalidated the token between the probe and the state lock.
		consent, err := scanConsent(tx.QueryRow(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND id=$2 AND confirmation_token_hash=$3 FOR UPDATE`, tenantID, probe.ID, tokenHash))
		if err != nil {
			return err
		}
		if consent.Status != domain.StatusPendingConfirmation {
			return ErrConflict
		}
		if consent.ConfirmationExpiresAt == nil || !consent.ConfirmationExpiresAt.After(at) {
			return ErrExpired
		}
		if consent.PreferenceGeneration == 0 || consent.PreferenceGeneration != current {
			return ErrStaleGeneration
		}
		// 4. Re-validate policy and build the evidence records under this tx.
		// A retried attempt calls finalize again, so its records (and IDs)
		// are rebuilt from the re-locked row; earlier attempts left nothing.
		receipt, audit, outbox, err := finalize(ctx, pgPolicyReader{tx: tx}, consent, at)
		if err != nil {
			return err
		}
		if receipt.ID == "" || receipt.TenantID != tenantID || receipt.ConsentID != consent.ID {
			return ErrConflict
		}
		// 5. Apply; the guarded UPDATE is a final compare-and-set.
		tag, err := tx.Exec(ctx, `UPDATE consents SET status='granted',granted_at=$3,confirmed_at=$3,receipt_id=$4,confirmation_token_hash='',version=version+1,updated_at=$3
		WHERE tenant_id=$1 AND id=$2 AND status='pending_confirmation' AND preference_generation=$5`, tenantID, consent.ID, at, receipt.ID, current)
		if err != nil {
			return classify(err)
		}
		if tag.RowsAffected() != 1 {
			return ErrStaleGeneration
		}
		before := consent
		consent.Status = domain.StatusGranted
		consent.GrantedAt = &at
		consent.ConfirmedAt = &at
		consent.ReceiptID = receipt.ID
		consent.ConfirmationTokenHash = ""
		consent.Version++
		consent.UpdatedAt = at
		if err := insertReceipt(ctx, tx, receipt); err != nil {
			return err
		}
		if audit.ResourceType == "" {
			audit.ResourceType = "consent"
		}
		audit.ConsentID = consent.ID
		audit.Revision = consent.Version
		audit.Changes = map[string]any{"before": before, "after": consent}
		if err := appendEffectsTx(ctx, tx, audit, outbox); err != nil {
			return err
		}
		granted = consent
		return nil
	})
	if err != nil {
		return domain.Consent{}, err
	}
	return granted, nil
}

func (p *Postgres) AppendAudit(ctx context.Context, event domain.AuditEvent) error {
	resourceType := event.ResourceType
	if resourceType == "" {
		resourceType = "consent"
	}
	revision := event.Revision
	if revision <= 0 {
		revision = 1
	}
	changes := event.Changes
	if changes == nil {
		changes = map[string]any{}
	}
	changesJSON, _ := json.Marshal(changes)
	_, err := p.pool.Exec(ctx, `INSERT INTO consent_audit_events(id,tenant_id,actor_id,action,consent_id,occurred_at,resource_type,revision,changes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, event.ID, event.TenantID, event.ActorID, event.Action, event.ConsentID, event.OccurredAt, resourceType, revision, changesJSON)
	return classify(err)
}

func (p *Postgres) AppendOutbox(ctx context.Context, event domain.OutboxEvent) error {
	event = normalizeOutbox(event)
	payload, _ := json.Marshal(event.Payload)
	_, err := p.pool.Exec(ctx, `INSERT INTO consent_outbox_events(id,schema_version,source,tenant_id,event_type,aggregate_type,aggregate_id,payload,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, event.ID, event.SchemaVersion, event.Source, event.TenantID, event.EventType, event.AggregateType, event.AggregateID, payload, event.OccurredAt)
	return classify(err)
}

// ClaimOutbox locks due rows with FOR UPDATE SKIP LOCKED and stamps them with a
// fresh claim token in the same transaction. A row is claimable when it is
// unpublished, due, and either unlocked or locked longer than the lease ago
// (by database time).
func (p *Postgres) ClaimOutbox(ctx context.Context, workerID string, limit int, tenantFilter string) (OutboxClaim, error) {
	claim := OutboxClaim{Lease: p.outboxLease}
	if limit <= 0 {
		return claim, nil
	}
	token, err := newClaimToken()
	if err != nil {
		return OutboxClaim{}, err
	}
	claim.Token = token
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return OutboxClaim{}, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id,schema_version,source,tenant_id,event_type,aggregate_type,aggregate_id,payload,occurred_at,attempts
		FROM consent_outbox_events
		WHERE published_at IS NULL AND next_attempt_at<=now()
		AND (locked_at IS NULL OR locked_at<now()-($3::bigint*interval '1 millisecond'))
		AND ($2='' OR tenant_id=$2)
		ORDER BY occurred_at,id LIMIT $1 FOR UPDATE SKIP LOCKED`, limit, tenantFilter, p.outboxLease.Milliseconds())
	if err != nil {
		return OutboxClaim{}, err
	}
	claim.Events = make([]OutboxRecord, 0, limit)
	ids := make([]string, 0, limit)
	for rows.Next() {
		var item OutboxRecord
		if err := rows.Scan(&item.ID, &item.SchemaVersion, &item.Source, &item.TenantID, &item.EventType, &item.AggregateType, &item.AggregateID, &item.Payload, &item.OccurredAt, &item.Attempts); err != nil {
			rows.Close()
			return OutboxClaim{}, err
		}
		claim.Events = append(claim.Events, item)
		ids = append(ids, item.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return OutboxClaim{}, err
	}
	if len(ids) > 0 {
		command, err := tx.Exec(ctx, `UPDATE consent_outbox_events
			SET locked_at=now(),locked_by=$1,claim_token=$2,attempts=attempts+1
			WHERE id=ANY($3) AND published_at IS NULL`, workerID, token, ids)
		if err != nil {
			return OutboxClaim{}, err
		}
		if command.RowsAffected() != int64(len(ids)) {
			return OutboxClaim{}, fmt.Errorf("%w: claimed %d of %d locked outbox rows", ErrConflict, command.RowsAffected(), len(ids))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return OutboxClaim{}, err
	}
	return claim, nil
}

// fencedOutboxUpdate applies set to the row only while token still owns it.
func (p *Postgres) fencedOutboxUpdate(ctx context.Context, set, id, token string, args ...any) error {
	if id == "" || token == "" {
		return ErrLeaseLost
	}
	command, err := p.pool.Exec(ctx, `UPDATE consent_outbox_events SET `+set+`
		WHERE id=$1 AND claim_token=$2 AND published_at IS NULL`, append([]any{id, token}, args...)...)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (p *Postgres) CompleteOutbox(ctx context.Context, id, token string) error {
	return p.fencedOutboxUpdate(ctx, `published_at=now(),locked_at=NULL,locked_by=NULL,claim_token=NULL`, id, token)
}

func (p *Postgres) RetryOutbox(ctx context.Context, id, token string, delay time.Duration) error {
	return p.fencedOutboxUpdate(ctx, `locked_at=NULL,locked_by=NULL,claim_token=NULL,next_attempt_at=now()+($3::bigint*interval '1 millisecond')`, id, token, max(delay, 0).Milliseconds())
}

func (p *Postgres) ReleaseOutbox(ctx context.Context, id, token string) error {
	return p.fencedOutboxUpdate(ctx, `locked_at=NULL,locked_by=NULL,claim_token=NULL,attempts=GREATEST(attempts-1,0)`, id, token)
}

func (p *Postgres) RescheduleOutbox(ctx context.Context, id, token string, delay time.Duration) error {
	return p.RetryOutbox(ctx, id, token, delay)
}

func (p *Postgres) ReleaseOutboxBatch(ctx context.Context, workerID string, events []OutboxRecord) error {
	if len(events) == 0 {
		return nil
	}
	ids := make([]string, 0, len(events))
	for _, e := range events {
		if e.ID != "" {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := p.pool.Exec(ctx, `UPDATE consent_outbox_events
		SET locked_at=NULL,locked_by=NULL,claim_token=NULL,attempts=GREATEST(attempts-1,0)
		WHERE id=ANY($1) AND published_at IS NULL AND (locked_by=$2 OR $2='')`, ids, workerID)
	return err
}
