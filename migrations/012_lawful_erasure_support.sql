-- Migration 012: Additive lawful erasure support for GDPR/NDPR Right to be Forgotten
-- Enables privacy_admin to redact personal data (before_state, after_state) in consent_audit_events
-- without compromising structural audit immutability (event_id, timestamps, action, actor remain immutable).

CREATE OR REPLACE FUNCTION reject_consent_audit_mutation()
RETURNS TRIGGER AS $$
BEGIN
  -- Strict exception: Allow lawful erasure only when the session setting is explicitly authorized.
  IF current_setting('nvuto.lawful_erasure_authorized', true) = 'true' THEN
    IF TG_OP = 'UPDATE' THEN
      -- Invariant: Structural event metadata must NOT be modified.
      IF NEW.id = OLD.id AND
         NEW.tenant_id = OLD.tenant_id AND
         NEW.actor_id = OLD.actor_id AND
         NEW.action = OLD.action AND
         NEW.consent_id = OLD.consent_id AND
         NEW.occurred_at = OLD.occurred_at AND
         NEW.revision = OLD.revision THEN
        RETURN NEW;
      END IF;
      RAISE EXCEPTION 'lawful erasure may only redact state payloads, structural metadata is immutable';
    END IF;
  END IF;

  RAISE EXCEPTION 'consent_audit_events records are immutable';
END;
$$ LANGUAGE plpgsql;

-- Ensure the trigger uses reject_consent_audit_mutation
DROP TRIGGER IF EXISTS trg_consent_audit_immutable ON consent_audit_events;
CREATE TRIGGER trg_consent_audit_immutable
  BEFORE UPDATE OR DELETE ON consent_audit_events
  FOR EACH ROW EXECUTE FUNCTION reject_consent_audit_mutation();

DROP TRIGGER IF EXISTS consent_audit_immutable ON consent_audit_events;
CREATE TRIGGER consent_audit_immutable
  BEFORE UPDATE OR DELETE ON consent_audit_events
  FOR EACH ROW EXECUTE FUNCTION reject_consent_audit_mutation();
