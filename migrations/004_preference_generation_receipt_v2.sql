-- 004: Preference generations (T-001) and versioned receipt hashes (T-004).
-- Additive only: no existing row is rewritten, no column or table is dropped.

-- T-001: one authoritative preference generation per tenant/subject/purpose/channel.
-- Every intent-changing write (grant, deny, withdraw, new double-opt-in
-- challenge) increments the generation inside its own transaction. A pending
-- challenge is confirmable only while its generation is still current.
CREATE TABLE IF NOT EXISTS consent_preference_state (
  tenant_id text NOT NULL,
  subject_id text NOT NULL,
  purpose text NOT NULL,
  channel text NOT NULL,
  generation bigint NOT NULL CHECK (generation > 0),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,subject_id,purpose,channel)
);

-- Existing consent rows receive generation 0. Any write after this migration
-- receives generation >= 1, so new intent always outranks pre-migration rows,
-- and pre-migration rows keep their previous relative (updated_at) order.
ALTER TABLE consents
  ADD COLUMN IF NOT EXISTS preference_generation bigint NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_consents_effective_generation
  ON consents(tenant_id,subject_id,purpose,channel,preference_generation DESC,updated_at DESC);

-- T-004: hash format of each receipt. Existing receipts take the column
-- default 1 (legacy format) without being updated, so the immutability
-- trigger on consent_receipts is not fired and historic evidence is preserved.
-- New receipts are written with hash_version = 2 (canonical v2 bytes).
ALTER TABLE consent_receipts
  ADD COLUMN IF NOT EXISTS hash_version smallint NOT NULL DEFAULT 1;

-- Existing-data validation (run manually before and after deploy; read-only):
--
--   Pending challenges issued before this migration are bound to generation 0
--   and are rejected on confirmation (safe-deny). They must be re-issued:
--     SELECT tenant_id, count(*) FROM consents
--      WHERE status = 'pending_confirmation' AND confirmation_token_hash <> ''
--        AND preference_generation = 0
--      GROUP BY tenant_id;
--
--   Legacy receipts to be reconciled with the receipt verifier:
--     SELECT tenant_id, count(*) FROM consent_receipts
--      WHERE hash_version = 1 GROUP BY tenant_id;
