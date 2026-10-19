CREATE TABLE IF NOT EXISTS consents (
  id text PRIMARY KEY,
  tenant_id text NOT NULL,
  subject_id text NOT NULL,
  purpose text NOT NULL,
  channel text NOT NULL,
  legal_basis text NOT NULL DEFAULT '',
  status text NOT NULL CHECK (status IN ('granted','denied','withdrawn','expired')),
  source text NOT NULL,
  scope jsonb NOT NULL DEFAULT '{}'::jsonb,
  version bigint NOT NULL DEFAULT 1,
  granted_at timestamptz,
  denied_at timestamptz,
  withdrawn_at timestamptz,
  expires_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_consents_effective ON consents(tenant_id,subject_id,purpose,channel,updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_consents_status ON consents(tenant_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS consent_audit_events (
  id text PRIMARY KEY,
  tenant_id text NOT NULL,
  actor_id text NOT NULL,
  action text NOT NULL,
  consent_id text NOT NULL,
  occurred_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_consent_audit_tenant_time ON consent_audit_events(tenant_id,occurred_at DESC);

CREATE TABLE IF NOT EXISTS consent_outbox_events (
  id text PRIMARY KEY,
  tenant_id text NOT NULL,
  event_type text NOT NULL,
  aggregate_id text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  occurred_at timestamptz NOT NULL,
  published_at timestamptz,
  attempts integer NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  locked_at timestamptz,
  locked_by text
);
CREATE INDEX IF NOT EXISTS idx_consent_outbox_pending ON consent_outbox_events(next_attempt_at,occurred_at) WHERE published_at IS NULL;
