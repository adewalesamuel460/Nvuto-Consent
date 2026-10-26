-- ==============================================================================
-- scripts/validate_integrity.sql
-- ==============================================================================
-- Read-Only Schema Integrity & Data Sanity Audit (T-028 / AT-028)
--
-- This script inspects the Nvuto Consent database to detect any data anomalies,
-- orphaned records, cross-tenant foreign key violations, invalid temporal shapes,
-- or malformed JSON structures before applying migration 011_schema_integrity_constraints.sql.
--
-- Execution is completely read-only.
-- Safe to execute concurrently against production or staging databases.
-- ==============================================================================

BEGIN TRANSACTION READ ONLY;

-- 1. Summary Census of Integrity Violations
SELECT 
  'consents' AS table_name,
  'temporal_expiry_violation' AS check_name,
  count(*) AS violation_count
FROM consents
WHERE expires_at IS NOT NULL AND expires_at <= created_at

UNION ALL

SELECT 
  'consents',
  'temporal_withdrawn_violation',
  count(*)
FROM consents
WHERE withdrawn_at IS NOT NULL AND withdrawn_at < created_at

UNION ALL

SELECT 
  'consents',
  'invalid_status_value',
  count(*)
FROM consents
WHERE status NOT IN ('granted', 'denied', 'withdrawn', 'expired', 'pending_confirmation')

UNION ALL

SELECT 
  'consents',
  'malformed_json_evidence_or_scope',
  count(*)
FROM consents
WHERE (evidence IS NOT NULL AND jsonb_typeof(evidence) <> 'object')
   OR (scope IS NOT NULL AND jsonb_typeof(scope) NOT IN ('object', 'null'))

UNION ALL

SELECT 
  'consent_receipts',
  'orphaned_receipt_no_consent',
  count(*)
FROM consent_receipts cr
WHERE NOT EXISTS (
  SELECT 1 FROM consents c WHERE c.id = cr.consent_id
)

UNION ALL

SELECT 
  'consent_receipts',
  'cross_tenant_fk_violation',
  count(*)
FROM consent_receipts cr
WHERE NOT EXISTS (
  SELECT 1 FROM consents c WHERE c.tenant_id = cr.tenant_id AND c.id = cr.consent_id
) AND EXISTS (
  SELECT 1 FROM consents c WHERE c.id = cr.consent_id
)

UNION ALL

SELECT 
  'consent_receipts',
  'malformed_evidence_or_empty_hash',
  count(*)
FROM consent_receipts
WHERE jsonb_typeof(evidence) <> 'object' OR receipt_hash IS NULL OR length(receipt_hash) = 0

UNION ALL

SELECT 
  'consent_idempotency',
  'orphaned_idempotency_no_consent',
  count(*)
FROM consent_idempotency ci
WHERE NOT EXISTS (
  SELECT 1 FROM consents c WHERE c.id = ci.consent_id
)

UNION ALL

SELECT 
  'consent_idempotency',
  'cross_tenant_fk_violation',
  count(*)
FROM consent_idempotency ci
WHERE NOT EXISTS (
  SELECT 1 FROM consents c WHERE c.tenant_id = ci.tenant_id AND c.id = ci.consent_id
) AND EXISTS (
  SELECT 1 FROM consents c WHERE c.id = ci.consent_id
)

UNION ALL

SELECT 
  'consent_idempotency',
  'empty_request_hash_or_key',
  count(*)
FROM consent_idempotency
WHERE request_hash IS NULL OR length(request_hash) = 0 OR idempotency_key IS NULL OR length(idempotency_key) = 0

UNION ALL

SELECT 
  'consent_suppressions',
  'invalid_scope_value',
  count(*)
FROM consent_suppressions
WHERE scope NOT IN ('global', 'channel', 'purpose_channel')

UNION ALL

SELECT 
  'consent_suppressions',
  'scope_field_shape_violation',
  count(*)
FROM consent_suppressions
WHERE NOT (
  (scope = 'global' AND purpose = '' AND channel = '') OR
  (scope = 'channel' AND purpose = '' AND channel <> '') OR
  (scope = 'purpose_channel' AND purpose <> '' AND channel <> '')
)

UNION ALL

SELECT 
  'consent_suppressions',
  'empty_subject_id',
  count(*)
FROM consent_suppressions
WHERE subject_id IS NULL OR length(subject_id) = 0;

COMMIT;
