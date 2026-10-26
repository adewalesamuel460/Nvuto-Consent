package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/Atlasfsp/Nvuto-Consent/internal/domain"
	"github.com/jackc/pgx/v5"
)

// LawfulErasureRequest defines the parameters for executing a GDPR (Art. 17)
// or NDPR "Right to be Forgotten" erasure request.
type LawfulErasureRequest struct {
	TenantID  string
	SubjectID string
	ActorID   string
	Reason    string
}

// LawfulErasureResult summarizes the impact of the lawful erasure operation.
type LawfulErasureResult struct {
	ConsentsRedacted    int64
	AuditEventsRedacted int64
	ErasureAuditID      string
	OutboxEventID       string
	OccurredAt          time.Time
}

func lawfulID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

// LawfulRedactSubject executes a privacy-safe redaction and erasure of a subject's
// data across consents, suppressions, and audit payloads while preserving
// the immutable structural envelope of audit records as required by regulatory accountability.
func (p *Postgres) LawfulRedactSubject(ctx context.Context, req LawfulErasureRequest) (LawfulErasureResult, error) {
	tenantID := strings.TrimSpace(req.TenantID)
	subjectID := strings.TrimSpace(req.SubjectID)
	actorID := strings.TrimSpace(req.ActorID)
	reason := strings.TrimSpace(req.Reason)

	if tenantID == "" || subjectID == "" || actorID == "" {
		return LawfulErasureResult{}, domain.ErrValidation.WithField("erasure", "tenant_id, subject_id, and actor_id are required")
	}
	if reason == "" {
		reason = "GDPR Article 17 / NDPR Right to Erasure Request"
	}

	var result LawfulErasureResult
	now := time.Now().UTC()
	auditID := lawfulID("aud_erase")
	outboxID := lawfulID("evt_erase")

	err := p.inTx(ctx, func(tx pgx.Tx) error {
		// 1. Authorize lawful erasure for this transaction session.
		// The trigger reject_consent_audit_mutation allows payload redaction ONLY
		// when nvuto.lawful_erasure_authorized = 'true' and structural metadata is preserved.
		if _, err := tx.Exec(ctx, `SET LOCAL nvuto.lawful_erasure_authorized = 'true'`); err != nil {
			return classify(err)
		}

		// 2. Discover all consent IDs associated with this subject in this tenant.
		rows, err := tx.Query(ctx, `SELECT id FROM consents WHERE tenant_id = $1 AND subject_id = $2`, tenantID, subjectID)
		if err != nil {
			return classify(err)
		}
		var consentIDs []string
		for rows.Next() {
			var cid string
			if err := rows.Scan(&cid); err != nil {
				rows.Close()
				return classify(err)
			}
			consentIDs = append(consentIDs, cid)
		}
		rows.Close()

		// 3. Tombstone active consents: status -> withdrawn, zero evidence and scope.
		redactedEvidence, _ := json.Marshal(map[string]any{
			"method":        "redacted:lawful_erasure",
			"artifact_hash": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			"collected_at":  "1970-01-01T00:00:00Z",
		})
		tag, err := tx.Exec(ctx, `
			UPDATE consents
			SET status = 'withdrawn',
			    withdrawn_at = $3,
			    scope = '{"redacted": "true"}'::jsonb,
			    evidence = $4,
			    confirmation_token_hash = '',
			    updated_at = $3
			WHERE tenant_id = $1 AND subject_id = $2
		`, tenantID, subjectID, now, redactedEvidence)
		if err != nil {
			return classify(err)
		}
		result.ConsentsRedacted = tag.RowsAffected()

		// 4. Deactivate suppressions for this subject.
		if _, err := tx.Exec(ctx, `
			UPDATE consent_suppressions
			SET active = false,
			    reason = 'redacted:lawful_erasure',
			    updated_at = $3
			WHERE tenant_id = $1 AND subject_id = $2
		`, tenantID, subjectID, now); err != nil {
			return classify(err)
		}

		// 5. Redact PII in audit event payloads for these consents.
		// Structural metadata (id, tenant_id, actor_id, action, consent_id, occurred_at, revision)
		// remains 100% immutable. Only before_state and after_state payloads are redacted.
		if len(consentIDs) > 0 {
			tagAudit, err := tx.Exec(ctx, `
				UPDATE consent_audit_events
				SET before_state = '{"redacted": true}'::jsonb,
				    after_state = '{"redacted": true}'::jsonb,
				    changes = '{"redacted": true}'::jsonb
				WHERE tenant_id = $1 AND (consent_id = ANY($2) OR (resource_type = 'consent' AND resource_id = ANY($2)))
			`, tenantID, consentIDs)
			if err != nil {
				return classify(err)
			}
			result.AuditEventsRedacted = tagAudit.RowsAffected()
		}

		// 6. Record immutable audit event and publish outbox event to propagate erasure.
		erasureAudit := domain.AuditEvent{
			ID:           auditID,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "consent.subject.erased",
			ConsentID:    "subject:" + subjectID,
			ResourceType: "subject",
			ResourceID:   subjectID,
			Revision:     1,
			Reason:       reason,
			OccurredAt:   now,
			AfterState:   map[string]any{"subject_id": subjectID, "status": "erased"},
			Changes:      map[string]any{"erased_consents_count": result.ConsentsRedacted},
		}

		erasureOutbox := domain.OutboxEvent{
			ID:                outboxID,
			SchemaVersion:     1,
			TenantID:          tenantID,
			EventType:         "consent.subject.erased",
			AggregateType:     "consent.subject",
			AggregateID:       subjectID,
			AggregateRevision: 1,
			Payload: map[string]any{
				"schema_version": 1,
				"tenant_id":      tenantID,
				"subject_id":     subjectID,
				"action":         "erased",
				"reason":         reason,
			},
			OccurredAt: now,
		}

		if err := appendEffectsTx(ctx, tx, erasureAudit, erasureOutbox); err != nil {
			return err
		}

		result.ErasureAuditID = auditID
		result.OutboxEventID = outboxID
		result.OccurredAt = now
		return nil
	})

	if err != nil {
		return LawfulErasureResult{}, err
	}
	return result, nil
}

// LawfulRedactSubject provides in-memory simulation of lawful erasure.
func (m *Memory) LawfulRedactSubject(ctx context.Context, req LawfulErasureRequest) (LawfulErasureResult, error) {
	if err := live(ctx); err != nil {
		return LawfulErasureResult{}, err
	}
	tenantID := strings.TrimSpace(req.TenantID)
	subjectID := strings.TrimSpace(req.SubjectID)
	actorID := strings.TrimSpace(req.ActorID)
	reason := strings.TrimSpace(req.Reason)

	if tenantID == "" || subjectID == "" || actorID == "" {
		return LawfulErasureResult{}, domain.ErrValidation.WithField("erasure", "tenant_id, subject_id, and actor_id are required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	var consentsRedacted int64
	var consentIDs []string

	for key, c := range m.consents {
		if c.TenantID == tenantID && c.SubjectID == subjectID {
			consentIDs = append(consentIDs, c.ID)
			c.Status = domain.StatusWithdrawn
			c.WithdrawnAt = &now
			c.Scope = map[string]string{"redacted": "true"}
			c.Evidence = domain.Evidence{Method: "redacted:lawful_erasure"}
			m.consents[key] = c
			consentsRedacted++
		}
	}

	for key, s := range m.suppressions {
		if s.TenantID == tenantID && s.SubjectID == subjectID {
			s.Active = false
			s.Reason = "redacted:lawful_erasure"
			m.suppressions[key] = s
		}
	}

	var auditRedacted int64
	for i, aud := range m.audit {
		if aud.TenantID == tenantID {
			for _, cid := range consentIDs {
				if aud.ConsentID == cid || aud.ResourceID == cid {
					aud.BeforeState = map[string]any{"redacted": true}
					aud.AfterState = map[string]any{"redacted": true}
					aud.Changes = map[string]any{"redacted": true}
					m.audit[i] = aud
					auditRedacted++
					break
				}
			}
		}
	}

	auditID := lawfulID("aud_erase")
	outboxID := lawfulID("evt_erase")

	m.audit = append(m.audit, domain.AuditEvent{
		ID:           auditID,
		TenantID:     tenantID,
		ActorID:      actorID,
		Action:       "consent.subject.erased",
		ConsentID:    "subject:" + subjectID,
		ResourceType: "subject",
		ResourceID:   subjectID,
		Revision:     1,
		Reason:       reason,
		OccurredAt:   now,
		AfterState:   map[string]any{"subject_id": subjectID, "status": "erased"},
		Changes:      map[string]any{"erased_consents_count": consentsRedacted},
	})

	m.outbox = append(m.outbox, domain.OutboxEvent{
		ID:                outboxID,
		SchemaVersion:     1,
		TenantID:          tenantID,
		EventType:         "consent.subject.erased",
		AggregateType:     "consent.subject",
		AggregateID:       subjectID,
		AggregateRevision: 1,
		Payload: map[string]any{
			"schema_version": 1,
			"tenant_id":      tenantID,
			"subject_id":     subjectID,
			"action":         "erased",
			"reason":         reason,
		},
		OccurredAt: now,
	})

	return LawfulErasureResult{
		ConsentsRedacted:    consentsRedacted,
		AuditEventsRedacted: auditRedacted,
		ErasureAuditID:      auditID,
		OutboxEventID:       outboxID,
		OccurredAt:          now,
	}, nil
}
