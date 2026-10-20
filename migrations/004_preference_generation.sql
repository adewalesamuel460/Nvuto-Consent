-- 004_preference_generation.sql: Preference generation tracking (T-001).
-- Additive only: introduces consent_preference_state and ties generations to consents table.

-- Authoritative preference generation table keyed by (tenant_id, subject_id, purpose, channel).
-- Increments monotonically on every grant, denial, withdrawal, or new double-opt-in challenge.
CREATE TABLE IF NOT EXISTS consent_preference_state (
  tenant_id text NOT NULL,
  subject_id text NOT NULL,
  purpose text NOT NULL,
  channel text NOT NULL,
  generation bigint NOT NULL CHECK (generation > 0),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, subject_id, purpose, channel)
);

-- Tie preference_generation to the consents table.
-- Existing consent rows receive generation 0. Any write after this migration
-- receives generation >= 1, so pending challenges are bound to the specific generation
-- they were created under and are invalidated when a superseding intent advances the generation.
ALTER TABLE consents
  ADD COLUMN IF NOT EXISTS preference_generation bigint NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_consents_effective_generation
  ON consents(tenant_id, subject_id, purpose, channel, preference_generation DESC, updated_at DESC);
