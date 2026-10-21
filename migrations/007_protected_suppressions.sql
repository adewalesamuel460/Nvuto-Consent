-- 007: Protected Administrative Suppressions (T-030).
-- Additive only: adds created_by_role column to consent_suppressions to track
-- the authority level under which suppressions were established, preventing
-- self-service users from clearing administrative suppressions.

ALTER TABLE consent_suppressions
  ADD COLUMN IF NOT EXISTS created_by_role text NOT NULL DEFAULT '';
