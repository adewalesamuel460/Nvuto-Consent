-- 008: Receipt evidence and notice immutability binding (T-005).
-- Additive only: binds accepted notice content hash, version, jurisdiction, and
-- full decision metadata (status, legal basis, policy version, actor) immutably
-- to consent receipts. Implements notice retirement and immutability triggers.

-- T-005: Add notice content, content hash, retirement and supersession flags
-- to consent_privacy_notices.
ALTER TABLE consent_privacy_notices
  ADD COLUMN IF NOT EXISTS content text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS content_hash text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS retired boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS superseded_by text NOT NULL DEFAULT '';

-- T-005: Immutability protection for privacy notices.
-- Content, content_hash, version, and jurisdiction cannot be altered once published.
-- Only active, retired, and superseded_by can be updated during retirement.
CREATE OR REPLACE FUNCTION trg_protect_privacy_notice_immutability()
RETURNS trigger AS $$
BEGIN
  IF (OLD.content <> '' AND NEW.content <> OLD.content) OR
     (OLD.content_hash <> '' AND NEW.content_hash <> OLD.content_hash) OR
     (NEW.version <> OLD.version) OR
     (NEW.jurisdiction <> OLD.jurisdiction) THEN
    RAISE EXCEPTION 'privacy notices are immutable: content, hash, version and jurisdiction cannot be modified once published; publish a new version instead';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_notice_immutability ON consent_privacy_notices;
CREATE TRIGGER trg_notice_immutability
  BEFORE UPDATE ON consent_privacy_notices
  FOR EACH ROW
  EXECUTE FUNCTION trg_protect_privacy_notice_immutability();

-- T-005: Add decision metadata and exact notice content hash to consent_receipts.
ALTER TABLE consent_receipts
  ADD COLUMN IF NOT EXISTS actor_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS consent_status text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS legal_basis text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS notice_content_hash text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS policy_version bigint NOT NULL DEFAULT 1;

-- T-005: Immutability protection for consent_receipts.
-- Receipts are append-only cryptographic evidence and must never be updated.
CREATE OR REPLACE FUNCTION trg_protect_receipt_immutability()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'consent receipts are strictly immutable and cannot be updated';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_receipt_immutability ON consent_receipts;
CREATE TRIGGER trg_receipt_immutability
  BEFORE UPDATE ON consent_receipts
  FOR EACH ROW
  EXECUTE FUNCTION trg_protect_receipt_immutability();
