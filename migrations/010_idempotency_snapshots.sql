-- 010_idempotency_snapshots.sql
-- T-025: Correct idempotency replay semantics with exact response snapshots.
-- Legitimate replays return the stored original_response snapshot without live mutation drift.

-- 1. Ensure request_hash and original_response exist on consent_idempotency
ALTER TABLE consent_idempotency
  ADD COLUMN IF NOT EXISTS request_hash text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS response_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS original_response jsonb NOT NULL DEFAULT '{}'::jsonb;

-- 2. Bidirectional backfill between response_snapshot and original_response for legacy rows
UPDATE consent_idempotency
SET original_response = response_snapshot
WHERE (original_response IS NULL OR original_response = '{}'::jsonb)
  AND response_snapshot IS NOT NULL
  AND response_snapshot <> '{}'::jsonb;

UPDATE consent_idempotency
SET response_snapshot = original_response
WHERE (response_snapshot IS NULL OR response_snapshot = '{}'::jsonb)
  AND original_response IS NOT NULL
  AND original_response <> '{}'::jsonb;

-- 3. Index for retention queries (garbage collection after TTL window)
CREATE INDEX IF NOT EXISTS idx_consent_idempotency_retention
  ON consent_idempotency (created_at ASC);
