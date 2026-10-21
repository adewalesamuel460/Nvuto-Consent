-- 009: Audit History Enhancement and Append-Only Immutability (T-029).
-- Additive only: enriches consent_audit_events with typed resource identifiers,
-- resource revision numbers, full before/after states, correlation identifiers,
-- and caller mutation reasons.
-- Enforces append-only immutability via a PostgreSQL trigger and strict least-privilege role restrictions.

-- 1. Schema Additions: add audit enrichment columns idempotently.
ALTER TABLE consent_audit_events
  ADD COLUMN IF NOT EXISTS resource_type text NOT NULL DEFAULT 'consent',
  ADD COLUMN IF NOT EXISTS resource_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS before_state jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS after_state jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS correlation_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS reason text NOT NULL DEFAULT '';

-- 2. Backfill resource_id from consent_id for existing audit records.
UPDATE consent_audit_events
  SET resource_id = consent_id
  WHERE resource_id = '' AND consent_id <> '';

-- 3. Composite index for authorized audit exports and chronological history reconstruction.
CREATE INDEX IF NOT EXISTS idx_consent_audit_export
  ON consent_audit_events(tenant_id, resource_type, resource_id, revision ASC, occurred_at ASC);

-- 4. Immutability Enforcement: Trigger rejecting any UPDATE or DELETE on consent_audit_events.
CREATE OR REPLACE FUNCTION reject_consent_audit_mutation()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'consent_audit_events is append-only: updates and deletes are forbidden';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_consent_audit_immutable ON consent_audit_events;

CREATE TRIGGER trg_consent_audit_immutable
BEFORE UPDATE OR DELETE ON consent_audit_events
FOR EACH ROW
EXECUTE FUNCTION reject_consent_audit_mutation();

-- 5. Least-Privilege Role Hardening: ensure operational application roles
-- (consent_api_role, consent_worker_role) can NEVER update, delete, or truncate audit records.
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'consent_api_role') THEN
    REVOKE UPDATE, DELETE, TRUNCATE ON consent_audit_events FROM consent_api_role;
    GRANT SELECT, INSERT ON consent_audit_events TO consent_api_role;
  END IF;
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'consent_worker_role') THEN
    REVOKE UPDATE, DELETE, TRUNCATE ON consent_audit_events FROM consent_worker_role;
    GRANT SELECT ON consent_audit_events TO consent_worker_role;
  END IF;
END $$;
