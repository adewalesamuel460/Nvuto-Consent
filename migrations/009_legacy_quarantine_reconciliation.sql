-- 009: Legacy Consent Classification and Reconciliation (T-008).
-- Additive only: provides structured reconciliation and classification tracking
-- for legacy consent records (e.g. upgraded 001 rows lacking notice_version or evidence).
-- Enables operational audits without mutating or deleting historic evidence.

-- Track reconciliation decisions, eligibility classifications, and audit counts.
CREATE TABLE IF NOT EXISTS consent_legacy_reconciliation (
  tenant_id text NOT NULL,
  class_code text NOT NULL,
  category text NOT NULL,
  eligibility_decision text NOT NULL, -- 'eligible', 'quarantined', 'terminal'
  rationale text NOT NULL,
  record_count bigint NOT NULL DEFAULT 0,
  reconciled_at timestamptz NOT NULL DEFAULT now(),
  approved_by text NOT NULL DEFAULT '',
  PRIMARY KEY (tenant_id, class_code)
);

-- View for classification of historical consent records without altering source data.
CREATE OR REPLACE VIEW view_consent_legacy_classification AS
SELECT
  c.tenant_id,
  c.id AS consent_id,
  c.subject_id,
  c.purpose,
  c.channel,
  c.legal_basis,
  c.status,
  c.notice_version,
  c.created_at,
  CASE
    WHEN c.status IN ('withdrawn', 'denied', 'expired') THEN 'CLASS_5_TERMINAL_NON_GRANTED'
    WHEN c.notice_version <> '' AND c.receipt_id <> '' THEN 'CLASS_6_GOVERNED_COMPLIANT'
    WHEN c.notice_version = '' AND c.legal_basis = 'contract' AND COALESCE(p.requires_affirmative_consent, false) = false THEN 'CLASS_1_SERVICE_CONTRACT_ELIGIBLE'
    WHEN c.notice_version = '' AND (c.legal_basis IN ('consent', '') OR COALESCE(p.requires_affirmative_consent, true) = true OR COALESCE(p.category, 'marketing') = 'marketing') THEN 'CLASS_2_MARKETING_AFFIRMATIVE_QUARANTINED'
    WHEN COALESCE(lb.requires_evidence, false) = true AND (c.evidence = '{}'::jsonb OR (c.evidence->>'artifact_hash') IS NULL OR (c.evidence->>'artifact_hash') = '') THEN 'CLASS_3_EVIDENCE_UNMET_QUARANTINED'
    WHEN COALESCE(p.requires_double_opt_in, false) = true AND c.confirmed_at IS NULL THEN 'CLASS_4_DOI_UNCONFIRMED_QUARANTINED'
    ELSE 'CLASS_2_MARKETING_AFFIRMATIVE_QUARANTINED'
  END AS classification
FROM consents c
LEFT JOIN consent_purposes p ON p.tenant_id = c.tenant_id AND p.code = c.purpose
LEFT JOIN consent_lawful_bases lb ON lb.tenant_id = c.tenant_id AND lb.code = c.legal_basis;
