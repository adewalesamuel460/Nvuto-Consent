-- 010: Outbox poison-event quarantine and dead letter queue (T-021).
-- Adds quarantine columns to consent_outbox_events to isolate unpublishable
-- events without blocking queue delivery or losing evidence.

ALTER TABLE consent_outbox_events
  ADD COLUMN IF NOT EXISTS quarantined boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS quarantined_at timestamptz,
  ADD COLUMN IF NOT EXISTS quarantine_reason text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_consent_outbox_quarantined
  ON consent_outbox_events(tenant_id, quarantined_at)
  WHERE quarantined = true;
