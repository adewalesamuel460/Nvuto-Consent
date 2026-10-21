# Operations Runbook: Legacy Consent Classification & Reconciliation (T-008)

- **Finding Traceability:** [F-008](../../docs/production-readiness-audit/15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-008)
- **Task ID:** [T-008](../../docs/production-readiness-audit/16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008)
- **Acceptance Gate:** [AT-008](../../docs/production-readiness-audit/14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-008) / [G-008](../../docs/production-readiness-audit/19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-008)
- **Linked Decision:** [D-04](../../docs/decisions/002_consent_evidence_and_timeline.md#4-decision-d-04-policy-changes-and-legacy-grants) (Option C: Category-Based Mixed Rules + Explicit Quarantine)
- **Status:** APPROVED & OPERATIONAL

---

## 1. Executive Summary & Problem Context

During the upgrade from schema version `001_init.sql` to `002_policy_governance.sql`, legacy records retained empty `notice_version = ''` strings. Under pre-remediation evaluator code, missing or blank `notice_version` bypassed registry re-evaluations, presenting a critical compliance vulnerability where unevidenced or unconfirmed grants could be treated as valid consent.

In accordance with **Decision D-04 (Option C)**, the platform enforces an explicit, non-permissive legacy eligibility rule:
1. **Zero Silent Grandfathering for Affirmative Consent:** Any consent record lacking an authoritative `notice_version` where the applicable purpose requires affirmative consent, requires double opt-in, or falls under marketing is strictly **quarantined** (`allowed = false`, `reason = "policy_requirement_unmet"`).
2. **Defensible Grandfathering for Service/Contractual Processing:** Legacy records under non-marketing categories (e.g. `service_updates`, `transactional`) governed by `contract` or `legitimate_interest` that do not mandate evidentiary binding remain **eligible** (`allowed = true`, `reason = "granted"`).
3. **Zero Evidence Loss:** No legacy records or evidentiary rows are deleted or rewritten during classification, migration, or evaluation.

---

## 2. Legacy Record Classification Taxonomy

All historical records are partitioned into six mutually exclusive classes:

| Class Code | Category Description | Applicable Legal Basis | Notice Version | Evaluation Disposition | Operational Next Steps |
|---|---|---|---|---|---|
| **CLASS_1_SERVICE_CONTRACT_ELIGIBLE** | Legacy transactional / service communications | `contract`, `legitimate_interest` | `""` (blank) | **ELIGIBLE** (`allowed: true`, `reason: "granted"`) | Retain under D-04 Option C. Valid for transactional notifications. |
| **CLASS_2_MARKETING_AFFIRMATIVE_QUARANTINED** | Marketing communications without notice version | `consent`, empty | `""` (blank) | **QUARANTINED** (`allowed: false`, `reason: "policy_requirement_unmet"`) | Launch affirmative re-consent capture campaign via self-service portal or email challenge. |
| **CLASS_3_EVIDENCE_UNMET_QUARANTINED** | Lawful basis mandates evidence, but artifact hash is missing | `consent` (or policy requiring evidence) | Any | **QUARANTINED** (`allowed: false`, `reason: "policy_requirement_unmet"`) | Backfill verified collection provenance if verifiable; otherwise require fresh capture. |
| **CLASS_4_DOI_UNCONFIRMED_QUARANTINED** | Double-Opt-In required but challenge never confirmed | `consent` | Any | **QUARANTINED** (`allowed: false`, `reason: "policy_requirement_unmet"`) | Re-issue double opt-in challenge token if customer initiates. |
| **CLASS_5_TERMINAL_NON_GRANTED** | Historical negative intent | Any | Any | **TERMINAL** (`allowed: false`, `reason: status`) | Retained immutably for audit defensibility. Non-permissive. |
| **CLASS_6_GOVERNED_COMPLIANT** | Fully governed modern records | Valid | Valid (`notice_version <> ''`) | **GOVERNED** (`allowed: true` if policies satisfied) | Normal governed evaluation lifecycle. |

---

## 3. Policy Owner Decisions & Formal Approvals (D-04)

Under Decision **D-04**, the following executive sign-offs govern this operational runbook:

- **Privacy & Data Protection Officer:** Approved Option C. Blank-notice marketing records must never be grandfathered. Service contracts may continue without customer re-prompting.
- **Product Management:** Approved. Marketing outreach to unverified legacy records is quarantined until explicit affirmative consent is recaptured.
- **Database Administrator / Infrastructure:** Approved. Migration `009_legacy_quarantine_reconciliation.sql` is strictly additive. Read-only queries enforce zero table locks and zero data deletion.

---

## 4. Operational Execution Runbook

### Step 1: Execute Read-Only Classification
Operators execute the standard classification query against the target database:

```bash
# Using ysqlsh / psql:
ysqlsh -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f scripts/sql/legacy_classification.sql
```

Alternatively, query the real-time classification view added in migration `009`:
```sql
SELECT 
    tenant_id,
    classification,
    count(*) AS record_count,
    min(created_at) AS earliest,
    max(created_at) AS latest
FROM view_consent_legacy_classification
GROUP BY tenant_id, classification
ORDER BY tenant_id, classification;
```

### Step 2: Record Reconciliation Sign-Off
Once counts are reviewed, log the verified counts into the audit table:

```sql
INSERT INTO consent_legacy_reconciliation (
    tenant_id,
    class_code,
    category,
    eligibility_decision,
    rationale,
    record_count,
    reconciled_at,
    approved_by
) VALUES 
('tenant-prod', 'CLASS_1', 'service', 'eligible', 'Grandfathered contractual service notifications under D-04', 12450, now(), 'privacy-officer@nvuto.internal'),
('tenant-prod', 'CLASS_2', 'marketing', 'quarantined', 'Unverified legacy marketing lacking notice version; affirmative re-consent required', 84200, now(), 'privacy-officer@nvuto.internal'),
('tenant-prod', 'CLASS_5', 'terminal', 'terminal', 'Historic withdrawn/denied/expired records; immutable audit proof', 15300, now(), 'privacy-officer@nvuto.internal')
ON CONFLICT (tenant_id, class_code) DO UPDATE 
SET record_count = EXCLUDED.record_count,
    reconciled_at = EXCLUDED.reconciled_at,
    approved_by = EXCLUDED.approved_by;
```

---

## 5. Rehearsal Reconciliation Matrix (Synthetic Baseline)

Rehearsal against representative synthetic legacy datasets yielded the following deterministic reconciliation results:

| Tenant | Class Code | Classification | Status | Count | Evaluation Result | Evidence Intact |
|---|---|---|---|---|---|---|
| `tenant-upgrade` | `CLASS_1` | `service_updates / email` | `granted` | 1 | `allowed: true, reason: "granted"` | Yes (100%) |
| `tenant-upgrade` | `CLASS_2` | `marketing / email` | `granted` | 1 | `allowed: false, reason: "policy_requirement_unmet"` | Yes (100%) |
| `tenant-upgrade` | `CLASS_5` | `marketing / email` | `withdrawn` | 1 | `allowed: false, reason: "withdrawn"` | Yes (100%) |
| **Total** | | | | **3** | **1 Eligible, 2 Disallowed** | **Zero deleted** |

---

## 6. Remediation & Recapture Procedures

1. **Quarantined Marketing Records (CLASS_2):**
   - Service users invoking `/v1/evaluate` for these data subjects receive `allowed: false`.
   - The customer journey directs the subject to `/v1/consents` with affirmative privacy notice acceptance (`notice_version: "2026-08"`, verified evidence artifact hash).
   - Once captured, a new versioned grant is issued with an incremented preference generation, establishing compliance without altering the legacy audit record.
2. **Auditing and Verification:**
   - Run `go test -v ./cmd/server -run TestAT008` to verify all evaluation edge cases against mock and live stores.
