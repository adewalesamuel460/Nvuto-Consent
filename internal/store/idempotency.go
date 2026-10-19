package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/jackc/pgx/v5"
)

type GovernedIdempotencyRepository interface {
	RecordGovernedIdempotent(ctx context.Context, idempotencyKey, requestHash string, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error)
}

type memoryIdempotencyEntry struct {
	RequestHash string
	ConsentID   string
}

var memoryIdempotency sync.Map // map[*Memory]map[string]memoryIdempotencyEntry; guarded by Memory.mu

func memoryLedger(m *Memory) map[string]memoryIdempotencyEntry {
	ledger, _ := memoryIdempotency.LoadOrStore(m, map[string]memoryIdempotencyEntry{})
	return ledger.(map[string]memoryIdempotencyEntry)
}

func (m *Memory) RecordGovernedIdempotent(ctx context.Context, idempotencyKey, requestHash string, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	if err := live(ctx); err != nil {
		return domain.Consent{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	ledger := memoryLedger(m)
	ledgerKey := key(consent.TenantID, idempotencyKey)
	if existing, ok := ledger[ledgerKey]; ok {
		if existing.RequestHash != requestHash {
			return domain.Consent{}, ErrConflict
		}
		stored, ok := m.consents[key(consent.TenantID, existing.ConsentID)]
		if !ok {
			return domain.Consent{}, ErrConflict
		}
		return stored, nil
	}
	if _, exists := m.consents[key(consent.TenantID, consent.ID)]; exists {
		return domain.Consent{}, ErrConflict
	}
	if receipt.ID != "" {
		if _, exists := m.receipts[key(receipt.TenantID, receipt.ID)]; exists {
			return domain.Consent{}, ErrConflict
		}
	}
	consent.PreferenceGeneration = m.advanceGenerationLocked(consent)
	if receipt.ID != "" {
		m.receipts[key(receipt.TenantID, receipt.ID)] = receipt
	}
	m.consents[key(consent.TenantID, consent.ID)] = consent
	m.audit = append(m.audit, audit)
	m.outbox = append(m.outbox, outbox)
	ledger[ledgerKey] = memoryIdempotencyEntry{RequestHash: requestHash, ConsentID: consent.ID}
	return consent, nil
}

// RecordGovernedIdempotent claims the idempotency key and records the consent
// in one transaction. A transient abort re-runs the whole transaction, so the
// key claim and the consent/receipt/audit/outbox rows commit together or not
// at all; a replay of an already committed key returns the stored consent and
// rolls back without writing.
func (p *Postgres) RecordGovernedIdempotent(ctx context.Context, idempotencyKey, requestHash string, consent domain.Consent, receipt domain.ConsentReceipt, audit domain.AuditEvent, outbox domain.OutboxEvent) (domain.Consent, error) {
	var result domain.Consent
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		var storedHash, storedConsentID string
		err := tx.QueryRow(ctx, `INSERT INTO consent_idempotency(tenant_id,idempotency_key,request_hash,consent_id,created_at)
		VALUES($1,$2,$3,$4,$5)
		ON CONFLICT(tenant_id,idempotency_key) DO UPDATE SET idempotency_key=excluded.idempotency_key
		RETURNING request_hash,consent_id`, consent.TenantID, idempotencyKey, requestHash, consent.ID, time.Now().UTC()).Scan(&storedHash, &storedConsentID)
		if err != nil {
			return classify(err)
		}
		if storedHash != requestHash {
			return ErrConflict
		}
		if storedConsentID != consent.ID {
			existing, err := scanConsent(tx.QueryRow(ctx, `SELECT `+consentColumns+` FROM consents WHERE tenant_id=$1 AND id=$2`, consent.TenantID, storedConsentID))
			if err != nil {
				return err
			}
			result = existing
			return errNoChange
		}
		result, err = recordGovernedTx(ctx, tx, consent, receipt, audit, outbox)
		return err
	})
	if err != nil && !errors.Is(err, errNoChange) {
		return domain.Consent{}, err
	}
	return result, nil
}
