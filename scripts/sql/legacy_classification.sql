-- =============================================================================
-- Read-Only Legacy Consent Classification Query (T-008 / F-008 / D-04)
-- =============================================================================
-- Description:
--   Performs a non-destructive, read-only classification of all legacy and
--   historical consent records (pre-002 schema rows with blank notice_version,
--   missing evidence, or unconfirmed DOI challenges).
--
-- Categories / Dispositions (under Decision D-04 Option C):
--   1. CLASS_1_SERVICE_CONTRACT_ELIGIBLE:
--      Grandfathered legacy service/contractual grants (non-marketing, lawful
--      basis contract/legitimate_interest, no affirmative consent/evidence required).
--      Eligible for continued delivery.
--
--   2. CLASS_2_MARKETING_AFFIRMATIVE_QUARANTINED:
--      Unsafe marketing or affirmative-consent grants lacking a valid notice_version.
--      Quarantined (allowed = false, reason = policy_requirement_unmet); requires
--      affirmative re-consent recapture campaign.
--
--   3. CLASS_3_EVIDENCE_UNMET_QUARANTINED:
--      Records under lawful bases requiring evidentiary binding where evidence
--      is empty or missing artifact_hash. Quarantined until evidence backfilled.
--
--   4. CLASS_4_DOI_UNCONFIRMED_QUARANTINED:
--      Records under Double-Opt-In policies that were never confirmed. Quarantined.
--
--   5. CLASS_5_TERMINAL_NON_GRANTED:
--      Historic negative consent states (denied, withdrawn, expired). Non-permissive.
--
--   6. CLASS_6_GOVERNED_COMPLIANT:
--      Fully governed modern records with valid notice_version and receipt_id.
-- =============================================================================

-- Query 1: Detailed Class Aggregation by Tenant, Purpose, Channel, and Legal Basis
SELECT
    c.tenant_id,
    c.purpose,
    COALESCE(p.category, 'unregistered') AS purpose_category,
    c.channel,
    c.legal_basis,
    c.status,
    CASE WHEN c.notice_version = '' THEN '(blank)' ELSE c.notice_version END AS notice_version,
    CASE
        WHEN c.status IN ('withdrawn', 'denied', 'expired') THEN 'CLASS_5_TERMINAL_NON_GRANTED'
        WHEN c.notice_version <> '' AND c.receipt_id <> '' THEN 'CLASS_6_GOVERNED_COMPLIANT'
        WHEN c.notice_version = '' AND c.legal_basis = 'contract' AND COALESCE(p.requires_affirmative_consent, false) = false THEN 'CLASS_1_SERVICE_CONTRACT_ELIGIBLE'
        WHEN c.notice_version = '' AND (c.legal_basis IN ('consent', '') OR COALESCE(p.requires_affirmative_consent, true) = true OR COALESCE(p.category, 'marketing') = 'marketing') THEN 'CLASS_2_MARKETING_AFFIRMATIVE_QUARANTINED'
        WHEN COALESCE(lb.requires_evidence, false) = true AND (c.evidence = '{}'::jsonb OR (c.evidence->>'artifact_hash') IS NULL OR (c.evidence->>'artifact_hash') = '') THEN 'CLASS_3_EVIDENCE_UNMET_QUARANTINED'
        WHEN COALESCE(p.requires_double_opt_in, false) = true AND c.confirmed_at IS NULL THEN 'CLASS_4_DOI_UNCONFIRMED_QUARANTINED'
        ELSE 'CLASS_2_MARKETING_AFFIRMATIVE_QUARANTINED'
    END AS classification,
    count(*) AS record_count,
    min(c.created_at) AS earliest_record,
    max(c.created_at) AS latest_record
FROM consents c
LEFT JOIN consent_purposes p ON p.tenant_id = c.tenant_id AND p.code = c.purpose
LEFT JOIN consent_lawful_bases lb ON lb.tenant_id = c.tenant_id AND lb.code = c.legal_basis
GROUP BY
    c.tenant_id,
    c.purpose,
    p.category,
    p.requires_affirmative_consent,
    p.requires_double_opt_in,
    c.channel,
    c.legal_basis,
    lb.requires_evidence,
    c.status,
    c.notice_version,
    c.receipt_id,
    c.evidence,
    c.confirmed_at
ORDER BY c.tenant_id, classification, record_count DESC;

-- Query 2: Executive Summary & Reconciliation Totals
SELECT
    CASE
        WHEN c.status IN ('withdrawn', 'denied', 'expired') THEN 'CLASS_5_TERMINAL_NON_GRANTED'
        WHEN c.notice_version <> '' AND c.receipt_id <> '' THEN 'CLASS_6_GOVERNED_COMPLIANT'
        WHEN c.notice_version = '' AND c.legal_basis = 'contract' AND COALESCE(p.requires_affirmative_consent, false) = false THEN 'CLASS_1_SERVICE_CONTRACT_ELIGIBLE'
        WHEN c.notice_version = '' AND (c.legal_basis IN ('consent', '') OR COALESCE(p.requires_affirmative_consent, true) = true OR COALESCE(p.category, 'marketing') = 'marketing') THEN 'CLASS_2_MARKETING_AFFIRMATIVE_QUARANTINED'
        WHEN COALESCE(lb.requires_evidence, false) = true AND (c.evidence = '{}'::jsonb OR (c.evidence->>'artifact_hash') IS NULL OR (c.evidence->>'artifact_hash') = '') THEN 'CLASS_3_EVIDENCE_UNMET_QUARANTINED'
        WHEN COALESCE(p.requires_double_opt_in, false) = true AND c.confirmed_at IS NULL THEN 'CLASS_4_DOI_UNCONFIRMED_QUARANTINED'
        ELSE 'CLASS_2_MARKETING_AFFIRMATIVE_QUARANTINED'
    END AS classification,
    CASE
        WHEN c.status IN ('withdrawn', 'denied', 'expired') THEN 'TERMINAL'
        WHEN c.notice_version <> '' AND c.receipt_id <> '' THEN 'ELIGIBLE'
        WHEN c.notice_version = '' AND c.legal_basis = 'contract' AND COALESCE(p.requires_affirmative_consent, false) = false THEN 'ELIGIBLE (Grandfathered Service)'
        ELSE 'QUARANTINED (Recapture Required)'
    END AS evaluation_disposition,
    count(*) AS total_records,
    round(100.0 * count(*) / sum(count(*)) OVER (), 2) AS percentage
FROM consents c
LEFT JOIN consent_purposes p ON p.tenant_id = c.tenant_id AND p.code = c.purpose
LEFT JOIN consent_lawful_bases lb ON lb.tenant_id = c.tenant_id AND lb.code = c.legal_basis
GROUP BY 1, 2
ORDER BY classification;
