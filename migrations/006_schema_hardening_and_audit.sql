-- 006: Database Schema Constraints (T-028), Audit History Reconstruction (T-029),
-- and Idempotency Replay Semantics (T-025).
-- Additive only: enforces missing tenant isolation and state relationships,
-- enables immutable audit reconstruction with typed before/after changes, and
-- provides exact response snapshots for governed idempotency replays.

-- T-028: Composite unique constraint on consents(tenant_id, id) to support
-- tenant-isolated composite foreign keys.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'uq_consents_tenant_id'
  ) THEN
    ALTER TABLE consents ADD CONSTRAINT uq_consents_tenant_id UNIQUE (tenant_id, id);
  END IF;
END $$;

-- T-028: Enforce tenant isolation on consent_receipts by linking via
-- (tenant_id, consent_id) instead of just consent_id.
ALTER TABLE consent_receipts
  DROP CONSTRAINT IF EXISTS consent_receipts_consent_id_fkey;

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

-- T-028: Enforce composite foreign key on consent_idempotency to ensure tenant isolation.
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

-- T-028: CHECK constraints to enforce valid state shapes on consent_suppressions.
-- Global scope requires blank purpose and channel.
-- Channel scope requires a channel but blank purpose.
-- Purpose_channel scope requires both purpose and channel.
ALTER TABLE consent_suppressions
  DROP CONSTRAINT IF EXISTS consent_suppressions_scope_shape_check;

ALTER TABLE consent_suppressions
  ADD CONSTRAINT consent_suppressions_scope_shape_check
  CHECK (
    (scope = 'global' AND purpose = '' AND channel = '') OR
    (scope = 'channel' AND purpose = '' AND channel <> '') OR
    (scope = 'purpose_channel' AND purpose <> '' AND channel <> '')
  );

-- T-029: Alter consent_audit_events to include typed resource identifiers,
-- revision numbers, and before/after changes.
ALTER TABLE consent_audit_events
  ADD COLUMN IF NOT EXISTS resource_type text NOT NULL DEFAULT 'consent',
  ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS changes jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_consent_audit_resource
  ON consent_audit_events(tenant_id, resource_type, consent_id, revision);

-- T-025: Store exact original response snapshot on consent_idempotency.
ALTER TABLE consent_idempotency
  ADD COLUMN IF NOT EXISTS response_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;
