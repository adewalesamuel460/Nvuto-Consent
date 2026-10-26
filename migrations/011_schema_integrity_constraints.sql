-- ==============================================================================
-- 011_schema_integrity_constraints.sql
-- ==============================================================================
-- Schema Integrity, Composite Tenant Foreign Keys & State Constraints (T-028 / AT-028)
--
-- Additive migration that enforces:
-- 1. Composite tenant-isolated Foreign Keys (receipts -> consents, idempotency -> consents).
-- 2. Temporal consistency CHECK constraints (expires_at > created_at, withdrawn_at >= created_at).
-- 3. Canonical status value restrictions on consents.
-- 4. Valid JSON structure CHECK constraints on evidence, scope, and response_snapshot.
-- 5. Scope-shape invariants on consent_suppressions.
--
-- Compatible with YugabyteDB YSQL and standard PostgreSQL.
-- Completely idempotent.
-- ==============================================================================

-- 1. Composite unique constraint on consents(tenant_id, id)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'uq_consents_tenant_id'
  ) THEN
    ALTER TABLE consents ADD CONSTRAINT uq_consents_tenant_id UNIQUE (tenant_id, id);
  END IF;
END $$;

-- 2. Drop legacy single-column FK on consent_receipts if present
ALTER TABLE consent_receipts DROP CONSTRAINT IF EXISTS consent_receipts_consent_id_fkey;

-- 3. Composite Foreign Key on consent_receipts referencing consents(tenant_id, id)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_consent_receipts_consent'
  ) THEN
    ALTER TABLE consent_receipts
      ADD CONSTRAINT fk_consent_receipts_consent
      FOREIGN KEY (tenant_id, consent_id) REFERENCES consents(tenant_id, id);
  END IF;
END $$;

-- 4. Composite Foreign Key on consent_idempotency referencing consents(tenant_id, id)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_consent_idempotency_consent'
  ) THEN
    ALTER TABLE consent_idempotency
      ADD CONSTRAINT fk_consent_idempotency_consent
      FOREIGN KEY (tenant_id, consent_id) REFERENCES consents(tenant_id, id);
  END IF;
END $$;

-- 5. Temporal CHECK constraints on consents table
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_consents_temporal_expiry'
  ) THEN
    ALTER TABLE consents
      ADD CONSTRAINT chk_consents_temporal_expiry
      CHECK (expires_at IS NULL OR expires_at > created_at);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_consents_temporal_withdrawn'
  ) THEN
    ALTER TABLE consents
      ADD CONSTRAINT chk_consents_temporal_withdrawn
      CHECK (withdrawn_at IS NULL OR withdrawn_at >= created_at);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_consents_temporal_updated'
  ) THEN
    ALTER TABLE consents
      ADD CONSTRAINT chk_consents_temporal_updated
      CHECK (updated_at >= created_at);
  END IF;
END $$;

-- 6. Status values CHECK constraint on consents table
ALTER TABLE consents DROP CONSTRAINT IF EXISTS consents_status_check;
ALTER TABLE consents DROP CONSTRAINT IF EXISTS chk_consents_status_valid;
ALTER TABLE consents
  ADD CONSTRAINT chk_consents_status_valid
  CHECK (status IN ('granted', 'denied', 'withdrawn', 'expired', 'pending_confirmation'));

-- 7. JSON object type constraints on consents
UPDATE consents SET scope = '{}'::jsonb WHERE scope IS NULL OR scope = 'null'::jsonb;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_consents_evidence_object'
  ) THEN
    ALTER TABLE consents
      ADD CONSTRAINT chk_consents_evidence_object
      CHECK (evidence IS NULL OR jsonb_typeof(evidence) = 'object');
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_consents_scope_object'
  ) THEN
    ALTER TABLE consents
      ADD CONSTRAINT chk_consents_scope_object
      CHECK (scope IS NULL OR jsonb_typeof(scope) IN ('object', 'null'));
  END IF;
END $$;

-- 8. Constraints on consent_receipts
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_consent_receipts_hash_nonempty'
  ) THEN
    ALTER TABLE consent_receipts
      ADD CONSTRAINT chk_consent_receipts_hash_nonempty
      CHECK (length(receipt_hash) > 0);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_consent_receipts_evidence_object'
  ) THEN
    ALTER TABLE consent_receipts
      ADD CONSTRAINT chk_consent_receipts_evidence_object
      CHECK (jsonb_typeof(evidence) = 'object');
  END IF;
END $$;

-- 9. Constraints on consent_idempotency
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_consent_idempotency_hash_nonempty'
  ) THEN
    ALTER TABLE consent_idempotency
      ADD CONSTRAINT chk_consent_idempotency_hash_nonempty
      CHECK (length(request_hash) > 0);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_consent_idempotency_snapshot_object'
  ) THEN
    ALTER TABLE consent_idempotency
      ADD CONSTRAINT chk_consent_idempotency_snapshot_object
      CHECK (jsonb_typeof(response_snapshot) = 'object');
  END IF;
END $$;

-- 10. Constraints on consent_suppressions
ALTER TABLE consent_suppressions DROP CONSTRAINT IF EXISTS consent_suppressions_scope_shape_check;
ALTER TABLE consent_suppressions
  ADD CONSTRAINT consent_suppressions_scope_shape_check
  CHECK (
    (scope = 'global' AND purpose = '' AND channel = '') OR
    (scope = 'channel' AND purpose = '' AND channel <> '') OR
    (scope = 'purpose_channel' AND purpose <> '' AND channel <> '')
  );

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_consent_suppressions_subject_nonempty'
  ) THEN
    ALTER TABLE consent_suppressions
      ADD CONSTRAINT chk_consent_suppressions_subject_nonempty
      CHECK (length(subject_id) > 0);
  END IF;
END $$;
