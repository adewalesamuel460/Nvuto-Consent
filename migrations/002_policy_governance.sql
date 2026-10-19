ALTER TABLE consents DROP CONSTRAINT IF EXISTS consents_status_check;
ALTER TABLE consents
  ADD CONSTRAINT consents_status_check
  CHECK (status IN ('granted','denied','withdrawn','expired','pending_confirmation'));

ALTER TABLE consents
  ADD COLUMN IF NOT EXISTS jurisdiction text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS notice_version text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS receipt_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS confirmation_token_hash text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS confirmation_expires_at timestamptz,
  ADD COLUMN IF NOT EXISTS confirmed_at timestamptz;

ALTER TABLE consent_outbox_events
  ADD COLUMN IF NOT EXISTS schema_version integer NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'nvuto/consent',
  ADD COLUMN IF NOT EXISTS aggregate_type text NOT NULL DEFAULT 'consent';

CREATE UNIQUE INDEX IF NOT EXISTS idx_consents_confirmation_token
  ON consents(tenant_id,confirmation_token_hash)
  WHERE confirmation_token_hash <> '';

CREATE TABLE IF NOT EXISTS consent_purposes (
  tenant_id text NOT NULL,
  code text NOT NULL,
  category text NOT NULL,
  allowed_channels jsonb NOT NULL,
  requires_affirmative_consent boolean NOT NULL DEFAULT false,
  requires_double_opt_in boolean NOT NULL DEFAULT false,
  active boolean NOT NULL DEFAULT true,
  version bigint NOT NULL DEFAULT 1,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,code)
);

CREATE TABLE IF NOT EXISTS consent_lawful_bases (
  tenant_id text NOT NULL,
  code text NOT NULL,
  allowed_categories jsonb NOT NULL,
  requires_evidence boolean NOT NULL DEFAULT false,
  active boolean NOT NULL DEFAULT true,
  version bigint NOT NULL DEFAULT 1,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,code)
);

CREATE TABLE IF NOT EXISTS consent_privacy_notices (
  tenant_id text NOT NULL,
  jurisdiction text NOT NULL,
  version text NOT NULL,
  published_at timestamptz NOT NULL,
  active boolean NOT NULL DEFAULT true,
  PRIMARY KEY (tenant_id,jurisdiction,version)
);

CREATE TABLE IF NOT EXISTS consent_receipts (
  id text PRIMARY KEY,
  tenant_id text NOT NULL,
  consent_id text NOT NULL,
  subject_id text NOT NULL,
  purpose text NOT NULL,
  channel text NOT NULL,
  notice_version text NOT NULL,
  jurisdiction text NOT NULL,
  evidence jsonb NOT NULL,
  receipt_hash text NOT NULL,
  issued_at timestamptz NOT NULL,
  UNIQUE (tenant_id,receipt_hash),
  FOREIGN KEY (consent_id) REFERENCES consents(id)
);
CREATE INDEX IF NOT EXISTS idx_consent_receipts_subject
  ON consent_receipts(tenant_id,subject_id,issued_at DESC);

CREATE TABLE IF NOT EXISTS consent_suppressions (
  id text PRIMARY KEY,
  tenant_id text NOT NULL,
  subject_id text NOT NULL,
  scope text NOT NULL CHECK (scope IN ('global','channel','purpose_channel')),
  purpose text NOT NULL DEFAULT '',
  channel text NOT NULL DEFAULT '',
  reason text NOT NULL,
  active boolean NOT NULL DEFAULT true,
  version bigint NOT NULL DEFAULT 1,
  updated_at timestamptz NOT NULL,
  UNIQUE (tenant_id,subject_id,scope,purpose,channel)
);
CREATE INDEX IF NOT EXISTS idx_consent_suppressions_lookup
  ON consent_suppressions(tenant_id,subject_id,active,scope,purpose,channel);

CREATE OR REPLACE FUNCTION reject_consent_immutable_change() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION '% records are immutable', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS consent_receipts_immutable ON consent_receipts;
CREATE TRIGGER consent_receipts_immutable
  BEFORE UPDATE OR DELETE ON consent_receipts
  FOR EACH ROW EXECUTE FUNCTION reject_consent_immutable_change();

DROP TRIGGER IF EXISTS consent_notices_immutable ON consent_privacy_notices;
CREATE TRIGGER consent_notices_immutable
  BEFORE UPDATE OR DELETE ON consent_privacy_notices
  FOR EACH ROW EXECUTE FUNCTION reject_consent_immutable_change();

DROP TRIGGER IF EXISTS consent_audit_immutable ON consent_audit_events;
CREATE TRIGGER consent_audit_immutable
  BEFORE UPDATE OR DELETE ON consent_audit_events
  FOR EACH ROW EXECUTE FUNCTION reject_consent_immutable_change();
