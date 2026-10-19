CREATE TABLE IF NOT EXISTS consent_idempotency (
  tenant_id text NOT NULL,
  idempotency_key text NOT NULL,
  request_hash text NOT NULL,
  consent_id text NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_consent_idempotency_consent
  ON consent_idempotency(tenant_id,consent_id);
