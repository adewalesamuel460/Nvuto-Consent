package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/jackc/pgx/v5"
)

type GovernedIdempotencyRepository interface {
	RecordGovernedIdempotent(ctx context.Context, idempotencyKey, requestHash string, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error)
	FindIdempotencySnapshot(ctx context.Context, tenantID, idempotencyKey string) (domain.Consent, string, bool, error)
}

func (m *Memory) FindIdempotencySnapshot(ctx context.Context, tenantID, idempKey string) (domain.Consent, string, bool, error) {
	if err := live(ctx); err != nil {
		return domain.Consent{}, "", false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	lKey := idempotencyKey{tenantID: tenantID, idempotencyKey: idempKey}
	entry, ok := m.idempotencyLedger[lKey]
	if !ok {
		return domain.Consent{}, "", false, nil
	}
	snap := entry.OriginalResponse
	if snap.ID == "" {
		snap = entry.ResponseSnapshot
	}
	return cloneConsent(snap), entry.RequestHash, true, nil
}

func (m *Memory) RecordGovernedIdempotent(ctx context.Context, idempKey, requestHash string, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	if err := live(ctx); err != nil {
		return domain.Consent{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	lKey := idempotencyKey{tenantID: consent.TenantID, idempotencyKey: idempKey}
	if existing, ok := m.idempotencyLedger[lKey]; ok {
		if existing.RequestHash != requestHash {
			return domain.Consent{}, ErrConflict
		}
		snap := existing.OriginalResponse
		if snap.ID == "" {
			snap = existing.ResponseSnapshot
		}
		return cloneConsent(snap), nil
	}
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
	m.audit = append(m.audit, cloneAuditEvent(audit))
	m.outbox = append(m.outbox, cloneOutboxEvent(outbox))
	m.idempotencyLedger[lKey] = memoryIdempotencyEntry{
		RequestHash:      requestHash,
		ConsentID:        consent.ID,
		ResponseSnapshot: clonedConsent,
		OriginalResponse: clonedConsent,
	}
	return cloneConsent(consent), nil
}

func (p *Postgres) FindIdempotencySnapshot(ctx context.Context, tenantID, idempKey string) (domain.Consent, string, bool, error) {
	var storedHash, storedConsentID string
	var snapshotBytes []byte
	err := p.pool.QueryRow(ctx, `
		SELECT request_hash, consent_id, COALESCE(NULLIF(original_response, '{}'::jsonb), response_snapshot)
		FROM consent_idempotency
		WHERE tenant_id=$1 AND idempotency_key=$2
	`, tenantID, idempKey).Scan(&storedHash, &storedConsentID, &snapshotBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Consent{}, "", false, nil
		}
		return domain.Consent{}, "", false, classify(err)
	}
	var snapshot domain.Consent
	if len(snapshotBytes) > 0 && string(snapshotBytes) != "{}" {
		if err := json.Unmarshal(snapshotBytes, &snapshot); err != nil {
			return domain.Consent{}, "", false, classify(err)
		}
	}
	if snapshot.ID == "" && storedConsentID != "" {
		existing, err := scanConsent(p.pool.QueryRow(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND id=$2`, tenantID, storedConsentID))
		if err != nil {
			return domain.Consent{}, "", false, err
		}
		snapshot = existing
	}
	return snapshot, storedHash, true, nil
}

// RecordGovernedIdempotent claims the idempotency key and records the consent
// in one transaction. A transient abort re-runs the whole transaction, so the
// key claim and the consent/receipt/audit/outbox rows commit together or not
// at all; a replay of an already committed key returns the stored response snapshot
// and rolls back without writing.
func (p *Postgres) RecordGovernedIdempotent(ctx context.Context, idempotencyKey, requestHash string, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	var result domain.Consent
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		var storedHash, storedConsentID string
		var snapshotBytes []byte
		err := tx.QueryRow(ctx, `
			SELECT request_hash, consent_id, COALESCE(NULLIF(original_response, '{}'::jsonb), response_snapshot)
			FROM consent_idempotency
			WHERE tenant_id=$1 AND idempotency_key=$2
			FOR UPDATE
		`, consent.TenantID, idempotencyKey).Scan(&storedHash, &storedConsentID, &snapshotBytes)
		if err == nil {
			if storedHash != requestHash {
				return ErrConflict
			}
			var snapshot domain.Consent
			if len(snapshotBytes) > 0 && string(snapshotBytes) != "{}" {
				if err := json.Unmarshal(snapshotBytes, &snapshot); err != nil {
					return classify(err)
				}
			}
			if snapshot.ID == "" && storedConsentID != "" {
				existing, err := scanConsent(tx.QueryRow(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND id=$2`, consent.TenantID, storedConsentID))
				if err != nil {
					return err
				}
				snapshot = existing
			}
			result = snapshot
			return errNoChange
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return classify(err)
		}

		result, err = recordGovernedTx(ctx, tx, consent, receipt, audit, outbox)
		if err != nil {
			return err
		}
		snapJSON, err := json.Marshal(result)
		if err != nil {
			return domain.ErrInternal.Wrap(err)
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO consent_idempotency(tenant_id, idempotency_key, request_hash, consent_id, response_snapshot, original_response, created_at)
			VALUES($1, $2, $3, $4, $5, $5, $6)
			ON CONFLICT(tenant_id, idempotency_key) DO UPDATE SET idempotency_key=excluded.idempotency_key
			RETURNING request_hash, consent_id, COALESCE(NULLIF(original_response, '{}'::jsonb), response_snapshot)
		`, consent.TenantID, idempotencyKey, requestHash, result.ID, snapJSON, time.Now().UTC()).Scan(&storedHash, &storedConsentID, &snapshotBytes)
		if err != nil {
			return classify(err)
		}
		if storedHash != requestHash {
			return ErrConflict
		}
		if storedConsentID != result.ID {
			var snapshot domain.Consent
			if len(snapshotBytes) > 0 && string(snapshotBytes) != "{}" {
				if err := json.Unmarshal(snapshotBytes, &snapshot); err != nil {
					return classify(err)
				}
			}
			if snapshot.ID == "" && storedConsentID != "" {
				existing, err := scanConsent(tx.QueryRow(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND id=$2`, consent.TenantID, storedConsentID))
				if err != nil {
					return err
				}
				snapshot = existing
			}
			result = snapshot
			return errNoChange
		}
		return nil
	})
	if err != nil && !errors.Is(err, errNoChange) {
		return domain.Consent{}, err
	}
	return result, nil
}
