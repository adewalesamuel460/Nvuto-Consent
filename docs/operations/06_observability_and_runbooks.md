# Operational Observability, Alerts & Incident Runbooks

**Document ID:** `DOC-OPS-006`  
**Task Reference:** `T-021` | **Finding:** `F-021` | **Gate:** `G-021` | **Acceptance Test:** `AT-021`  
**Classification:** Internal Operational Guide  
**Status:** Approved for Production  

---

## 1. Executive Summary

This runbook establishes operational observability standards for the Nvuto Consent Service API and Transactional Outbox Worker. It specifies the telemetry architecture, Prometheus metric schemas, alert thresholds, Grafana dashboard panels, and the exact procedures an operator must follow to safely inspect, diagnose, and replay poison events isolated in the outbox Dead Letter Queue (DLQ).

---

## 2. Privacy-Safe Telemetry Design

In strict compliance with privacy regulations (GDPR Art. 25 & 32, NDPR, CCPA), the operational telemetry layer is engineered under a zero-privacy-leakage constraint:

1. **Strictly Prohibited Metric Labels:**
   - Customer Subject Identifiers (`subject_id`)
   - Bearer / OIDC Access Tokens (`Authorization` header contents)
   - Double Opt-In Confirmation Secrets & Tokens (`token_hash`, plaintext tokens)
   - Raw DSN connection credentials or database passwords
2. **Bounded Low-Cardinality Labels Only:**
   - `tenant_id`: Bounded customer tenant identifier (e.g. `tenant-a`, `tenant-b`).
   - `endpoint`: Normalized route template (e.g. `/v1/consents/:id`, `/v1/evaluate`).
   - `status`: HTTP status code string (`200`, `400`, `401`, `403`, `415`, `500`).
   - `event_type`: Predefined CloudEvent taxonomy (e.g. `consent.granted`, `consent.withdrawn`).
   - `reason`: Fixed categorical reason (e.g. `opt_in_required`, `double_opt_in_pending`, `suppressed`).

---

## 3. Metrics Catalog

### 3.1 API & Auth Metrics

| Metric Name | Type | Labels | Description |
| :--- | :--- | :--- | :--- |
| `nvuto_consent_http_requests_total` | Counter | `endpoint`, `method`, `status`, `tenant_id` | Cumulative HTTP requests served by the API. |
| `nvuto_consent_http_request_duration_seconds` | Histogram | `endpoint`, `method`, `status` | Request execution latency distribution. |
| `nvuto_consent_decision_evaluations_total` | Counter | `tenant_id`, `purpose`, `channel`, `allowed`, `reason` | Consent policy decisions served to upstream systems. |
| `nvuto_consent_auth_failures_total` | Counter | `tenant_id`, `reason` | Authentication and authorization rejections. |
| `nvuto_consent_auth_jwks_refresh_total` | Counter | `status` | IdP JWKS public key set refresh attempts (`success`, `failure`, `throttled`). |
| `nvuto_consent_db_pool_connections` | Gauge | `state` | Database connection pool stats (`acquired`, `idle`, `total`, `max`). |

### 3.2 Outbox & Propagation Metrics

| Metric Name | Type | Labels | Description |
| :--- | :--- | :--- | :--- |
| `nvuto_consent_outbox_publish_attempts_total` | Counter | `tenant_id`, `event_type` | Total publishing attempts to NATS JetStream. |
| `nvuto_consent_outbox_publish_success_total` | Counter | `tenant_id`, `event_type` | JetStream-acknowledged publications. |
| `nvuto_consent_outbox_publish_failures_total` | Counter | `tenant_id`, `event_type`, `reason` | Publish errors encountering transient or terminal broker faults. |
| `nvuto_consent_outbox_lag_seconds` | Gauge | `tenant_id` | Age in seconds of the oldest unpublished, non-quarantined event. |
| `nvuto_consent_outbox_quarantined_events_total` | Counter | `tenant_id`, `event_type` | Count of poison events isolated into DLQ. |
| `nvuto_consent_outbox_pending_events_count` | Gauge | `tenant_id` | Current backlog depth of actionable outbox records. |

---

## 4. Grafana Dashboard Interpretation

The dashboard template is located at `deploy/observability/grafana-dashboard.json` (`UID: nvuto-consent-observability`). It is structured into two dedicated operational sections:

### Section 1: API Health, Latency & Security
- **API Request Rate by HTTP Status:** Visualizes traffic volume and 4xx/5xx ratios. A sudden spike in 5xx indicates upstream database contention or service crashes.
- **API Latency Percentiles (p50, p95, p99):** Tracks user-perceived responsiveness. p99 > 500ms indicates slow queries or database pool starvation.
- **Auth Failures:** Surfaces brute-force attempts, expired credentials, or role misconfigurations.
- **DB Pool Saturation:** Displays active (`acquired`) connections vs `max`. A ratio > 80% requires scaling pool limits or optimizing transaction lifetimes.
- **Evaluation Decisions:** Confirms that consent decisions are properly evaluating across lawful bases and active notices.

### Section 2: Consumer Propagation & Outbox Reliability
- **Outbox Propagation Lag (Seconds):** Measures delay between transactional commit and NATS publication. Normal operations stay under 5 seconds.
- **Publish Acks vs Retry Rates:** Visualizes message throughput. Failures trigger bounded exponential backoff (`2^attempts` seconds, capped at 15 minutes).
- **Poison Events Quarantined:** Displays poison events diverted to the Dead Letter Queue. **Any non-zero count requires operator investigation.**

---

## 5. Poison Event Quarantine (DLQ) Architecture

When a transient broker issue occurs, the worker retries the event with bounded backoff. However, if an event fails continuously due to terminal issues (e.g. subject routing rules rejected, broker payload limit exceeded, or unrecoverable network partition) and reaches `OUTBOX_MAX_ATTEMPTS` (default: 5):

1. **Automatic Isolation:** The worker executes `QuarantineOutbox(ctx, event.ID, claim.Token, reason)`.
2. **Non-Blocking Queue:** The event row is stamped with `quarantined = true`, `quarantined_at = now()`, and `quarantine_reason = error_message`.
3. **Queue Skipping:** Subsequent `ClaimOutbox` queries execute `WHERE (quarantined IS NULL OR quarantined = false)` (or filter `!state.Quarantined`), ensuring downstream valid events are never held back by a single bad record.
4. **Metric & Alert Generation:** `nvuto_consent_outbox_quarantined_events_total` increments, alerting the on-call engineer.

---

## 6. Incident Runbook: Investigating & Replaying Quarantined Events

### Phase 1: Detection & Triage
When the `OutboxPoisonEventQuarantined` alert fires:
1. Open the Grafana dashboard and identify the affected `tenant_id` and `event_type`.
2. Access the database replica or primary to inspect the quarantined records:

```sql
SELECT id, tenant_id, event_type, aggregate_type, aggregate_id, 
       aggregate_revision, attempts, quarantined_at, quarantine_reason
FROM consent_outbox_events
WHERE quarantined = true
ORDER BY quarantined_at DESC
LIMIT 50;
```

### Phase 2: Root Cause Diagnosis
Inspect the `quarantine_reason` column:
- **`nats: no responders` / `stream not found`:** Verify that the JetStream stream `NVUTO_CONSENT` exists and subjects are bound.
- **`nats: message size exceeds maximum`:** The event payload exceeds NATS subject limit. Verify audit evidence payload size.
- **Network / DNS timeout:** Temporary broker partition during worker attempts.

### Phase 3: Remediating Underlying Cause
- If broker subjects were missing: restore or create the subject in JetStream.
- If NATS cluster had a quorum loss: verify cluster recovery and test manual connectivity via `nats sub "nvuto.consent.>"`.

### Phase 4: Safe Manual Event Replay
Once the underlying root cause is resolved, the operator resets the quarantined event back into the actionable processing pipeline using the store's replay mechanism:

#### Option A: Programmatic Store Replay
```go
err := repo.ReplayOutbox(ctx, eventID)
if err != nil {
    log.Fatalf("failed to replay event: %v", err)
}
```

#### Option B: SQL Operator Console Command
```sql
-- Reset quarantined event back to actionable state
UPDATE consent_outbox_events
SET quarantined = false,
    quarantined_at = NULL,
    quarantine_reason = '',
    attempts = 0,
    next_attempt_at = now(),
    locked_at = NULL,
    locked_by = NULL,
    claim_token = NULL
WHERE id = 'TARGET_EVENT_ID'
  AND published_at IS NULL
  AND quarantined = true;
```

#### Option C: Bulk Tenant Replay (Following Cluster Outage)
```sql
UPDATE consent_outbox_events
SET quarantined = false,
    quarantined_at = NULL,
    quarantine_reason = '',
    attempts = 0,
    next_attempt_at = now(),
    locked_at = NULL,
    locked_by = NULL,
    claim_token = NULL
WHERE tenant_id = 'tenant-a'
  AND published_at IS NULL
  AND quarantined = true;
```

### Phase 5: Verification & Post-Mortem
1. Observe the worker logs for `complete_succeeded` on the replayed `event_id`.
2. Verify that `nvuto_consent_outbox_lag_seconds` drops back to baseline (< 5s).
3. Confirm downstream consumers have processed the event revision idempotently using `Msg-Id` and `aggregate_revision`.
4. Close the incident ticket with the root cause and mitigation summary.
