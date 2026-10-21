-- 008: Least-privilege roles and database role-based access control (T-019).
-- Establishes isolated database roles for the API, Outbox Worker, and Migration runner.
-- Enforces least-privilege: blocks outbox claiming by API, blocks consent mutation by worker,
-- and strips all DDL permissions from operational service roles.

-- 1. Create isolated roles idempotently
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'consent_api_role') THEN
    CREATE ROLE consent_api_role NOLOGIN;
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'consent_worker_role') THEN
    CREATE ROLE consent_worker_role NOLOGIN;
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'consent_migration_role') THEN
    CREATE ROLE consent_migration_role NOLOGIN;
  END IF;
END $$;

-- 2. Restrict Schema DDL permissions (No DDL for API or Worker)
REVOKE ALL ON SCHEMA public FROM consent_api_role, consent_worker_role;
REVOKE CREATE ON SCHEMA public FROM consent_api_role, consent_worker_role;
GRANT USAGE ON SCHEMA public TO consent_api_role, consent_worker_role;

GRANT ALL ON SCHEMA public TO consent_migration_role;

-- 3. API Role permissions:
-- Business tables: INSERT, SELECT, UPDATE
GRANT SELECT, INSERT, UPDATE ON consents, consent_receipts, consent_suppressions TO consent_api_role;
GRANT SELECT, INSERT, UPDATE ON consent_purposes, consent_lawful_bases, consent_privacy_notices, consent_preference_state, consent_idempotency TO consent_api_role;
GRANT SELECT, INSERT ON consent_audit_events TO consent_api_role;

-- Outbox table: Strictly INSERT only (API enqueues events within business transactions, never processes)
REVOKE ALL ON consent_outbox_events FROM consent_api_role;
GRANT INSERT ON consent_outbox_events TO consent_api_role;

-- 4. Worker Role permissions:
-- Outbox table: SELECT, UPDATE (for claiming, publishing, fencing, completing, and rescheduling)
REVOKE ALL ON consent_outbox_events FROM consent_worker_role;
GRANT SELECT, UPDATE ON consent_outbox_events TO consent_worker_role;

-- Business tables: Strictly SELECT only (reads consent state to build CloudEvent payloads, cannot mutate)
REVOKE ALL ON consents, consent_receipts, consent_suppressions FROM consent_worker_role;
GRANT SELECT ON consents, consent_receipts, consent_suppressions TO consent_worker_role;
GRANT SELECT ON consent_purposes, consent_lawful_bases, consent_privacy_notices, consent_preference_state, consent_idempotency, consent_audit_events TO consent_worker_role;

-- 5. Migration Role permissions:
-- Full schema DDL & DML capabilities
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO consent_migration_role;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO consent_migration_role;

-- Sequences usage for API and worker
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO consent_api_role, consent_worker_role;
