# Downstream Consumer Event Reconciliation Contract

- **Document ID:** ARC-005
- **Version:** 1.0.0
- **Status:** APPROVED / NORMATIVE SPECIFICATION
- **Date:** 2026-10-21
- **Governing Tickets & Gates:** [T-010](file:///c:/antigravity/scratch/Nvuto-Consent/docs/production-readiness-audit/16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [F-010](file:///c:/antigravity/scratch/Nvuto-Consent/docs/production-readiness-audit/15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-010), [AT-010](file:///c:/antigravity/scratch/Nvuto-Consent/docs/production-readiness-audit/14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-010), [G-010](file:///c:/antigravity/scratch/Nvuto-Consent/docs/production-readiness-audit/19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-010)
- **Governing Decisions:** [D-07 (Consumer Integrations)](file:///c:/antigravity/scratch/Nvuto-Consent/docs/decisions/001_launch_scope_and_privacy.md), [D-13 (Event Order & Reconciliation)](file:///c:/antigravity/scratch/Nvuto-Consent/docs/decisions/003_role_identity_and_event_reconciliation.md)
- **Target Audience:** Engineering teams building and maintaining downstream consumer products (`INT-01` through `INT-08`), platform architects, and site reliability engineers.

---

## 1. Executive Summary & Purpose

The Nvuto-Consent platform employs a transactional outbox pattern to publish state mutations asynchronously to NATS JetStream. In a distributed, multi-region environment with asynchronous retries, message replay, and network jitter:
1. Message delivery order across consumers **cannot** be guaranteed solely by the transport broker.
2. The NATS JetStream broker deduplication window (`jetStreamDuplicateWindow = 10 minutes`) is finite and handles only transient worker retries and lease handovers.
3. Consents lapse passively on wall-clock expiry without emitting dynamic event streams.

This document defines the **binding consumer reconciliation contract** that all eight downstream consumers (`INT-01` through `INT-08`) **MUST** implement. Adherence to this contract guarantees that no consumer ever retains stale permission, communicates with a customer who has withdrawn consent, or violates regional privacy regulations (GDPR, NDPA, POPIA, CCPA).

---

## 2. Event Envelope & Transport Architecture

All events are published as [CloudEvents v1.0](https://cloudevents.io/) JSON payloads over NATS JetStream on subject hierarchy:
```
nvuto.consent.<event_type_suffix>
```

### 2.1 Transport Headers

Every NATS message published by `outbox-worker` contains:

| Header Name | Value Description | Purpose |
|---|---|---|
| `Nats-Msg-Id` | Unique Outbox Event UUID (`evt_...`) | JetStream 10-minute broker-level deduplication window. |
| `Content-Type` | `application/cloudevents+json` | MIME type specification. |
| `X-Nvuto-Tenant-ID` | Canonical Tenant Identifier (`tenant_...`) | Multi-tenant filtering and routing. |
| `X-Nvuto-Aggregate-Type` | Aggregate domain entity family (e.g., `consent.preference`) | Protocol filtering without JSON parsing. |
| `X-Nvuto-Aggregate-Revision` | Monotonic int64 string (e.g., `"2"`) | Fast transport-level stale event discard. |

### 2.2 CloudEvent Envelope Schema

```json
{
  "specversion": "1.0",
  "id": "evt_01JABC1234567890DEF",
  "source": "nvuto/consent",
  "type": "consent.withdrawn",
  "subject": "consent.preference/con_01JABC1234567890CON",
  "time": "2026-10-21T16:00:00Z",
  "datacontenttype": "application/json",
  "nvutoschemaversion": 1,
  "nvutotenantid": "tenant-enterprise-alpha",
  "nvutoaggregatetype": "consent.preference",
  "nvutoaggregaterevision": 2,
  "data": { ... }
}
```

---

## 3. The Eight Event Families & Nine Concrete Types

The service emits nine concrete event types across eight event families. Each event payload strictly includes `schema_version: 1`, `aggregate_type`, `aggregate_revision`, `tenant_id`, and canonical `subject_id` where applicable.

| # | Event Family | Concrete Event Type | Aggregate Type | Aggregate ID | Aggregate Revision Source | Canonical Subject ID |
|---|---|---|---|---|---|---|
| 1 | Purpose Governance | `consent.purpose.changed` | `consent.purpose` | `purpose.code` | Purpose Version (`1, 2, ...`) | N/A (Registry Entity) |
| 2 | Lawful Basis | `consent.lawful_basis.changed` | `consent.lawful_basis` | `lawful_basis.code` | Basis Version (`1, 2, ...`) | N/A (Registry Entity) |
| 3 | Privacy Notice | `consent.notice.published`<br>`consent.notice.retired` | `consent.notice` | `notice.version` | Notice Lifecycle (1 = Published, 2 = Retired) | N/A (Registry Entity) |
| 4 | Preference Capture | `consent.granted`<br>`consent.denied` | `consent.preference` | `consent.id` | Preference Generation (`preference_generation`) | Mandatory (`subject_id`) |
| 5 | Preference Withdrawal | `consent.withdrawn` | `consent.preference` | `consent.id` | Preference Generation (`preference_generation`) | Mandatory (`subject_id`) |
| 6 | Suppression Governance | `consent.suppression.changed` | `consent.suppression` | `suppression.id` | Suppression Version (`suppression.version`) | Mandatory (`subject_id`) |
| 7 | Double Opt-In Challenge | `consent.challenge.created` | `consent.double_opt_in` | `consent.id` | Preference Generation (`preference_generation`) | Mandatory (`subject_id`) |
| 8 | Double Opt-In Confirmation | `consent.confirmed` | `consent.preference` | `consent.id` | Preference Generation (`preference_generation`) | Mandatory (`subject_id`) |

---

## 4. Downstream Consumers Matrix (INT-01 through INT-08)

All eight named downstream consumers consume events via dedicated, durable JetStream pull-subscriptions:

| Interface ID | Consumer System | Subscribed Event Families | Critical Consumer Action | Mandatory Pre-Send Check |
|---|---|---|---|---|
| **INT-01** | **Engage** (Marketing & Campaigns) | `consent.preference`, `consent.suppression`, `consent.purpose` | Invalidate audience lists; update local permission cache. | **Synchronous `POST /v1/evaluate` or batch evaluate** immediately before dispatching any marketing communication. |
| **INT-02** | **Service** (Customer Support) | `consent.preference`, `consent.suppression` | Update customer preference banner; enforce agent communication boundaries. | Invalidate operator view; check channel eligibility before agent outbound action. |
| **INT-03** | **Contact Center** (Telephony/IVR) | `consent.suppression`, `consent.preference` | Invalidate dialer lists; enforce phone suppression. | Synchronous pre-dial verification against active suppressions. |
| **INT-04** | **Customer Portal** (Self-Service) | `consent.preference`, `consent.notice`, `consent.double_opt_in` | Update customer dashboard; show active notices; refresh status after token confirmation. | Read authoritative preferences from `GET /v1/consents/{id}` or `POST /v1/evaluate`. |
| **INT-05** | **CRM** (Sales & Account Management) | `consent.preference`, `consent.suppression` | Synchronize customer opt-in/opt-out status with external CRM entities. | Filter exports and batch syncs; discard stale updates. |
| **INT-06** | **Conversations** (Chat & Messaging) | `consent.preference`, `consent.suppression` | Immediate message blocking on withdrawal or suppression. | Pre-flight check prior to sending conversational notifications (SMS/WhatsApp). |
| **INT-07** | **AI** (LLM/Model Ingestion & RAG) | `consent.purpose`, `consent.preference` | Purge unconsented data from AI training sets and contextual embeddings. | Model pipeline gating; verify specific AI purpose consent. |
| **INT-08** | **Integration Hub** (Enterprise ESB) | All 8 Families | Reliable message routing, cross-system ETL, and dead-letter handling. | Enforce ordering and dedup before dispatching to external third parties. |

---

## 5. Invariant Ordering & Reconciliation Rules

### Rule 1: Monotonic Revision Check (The Out-of-Order Discard Invariant)

Every consumer maintaining a projection or local cache for any aggregate MUST store the latest `aggregate_revision` alongside the aggregate key:
- For `consent.preference`: Key is `(tenant_id, subject_id, purpose, channel)`.
- For `consent.suppression`: Key is `(tenant_id, subject_id, scope, purpose, channel)`.
- For `consent.purpose`: Key is `(tenant_id, purpose_code)`.
- For `consent.lawful_basis`: Key is `(tenant_id, basis_code)`.
- For `consent.notice`: Key is `(tenant_id, jurisdiction, version)`.

#### Discard Condition
When receiving an event with revision $R_{event}$ for aggregate key $K$ having locally stored revision $R_{local}$:
$$\text{If } R_{event} \le R_{local} \implies \mathbf{DISCARD\ EVENT\ IMMEDIATELY}$$

The event **MUST NOT** mutate local state, trigger downstream notifications, or update cached permissions. It must be acknowledged to the broker and logged as a stale discard:
```json
{
  "log_event": "stale_event_discarded",
  "aggregate_key": "tenant-alpha/cus-101/email_marketing/email",
  "event_id": "evt_01JGRANT1111",
  "incoming_revision": 1,
  "stored_revision": 2,
  "action": "no_op"
}
```

### Rule 2: The Reverse-Delivery Trap Protection

Consider this common network anomaly:
1. At $T_0$, customer grants consent $\implies$ Event $E_1$ (`consent.granted`, `aggregate_revision = 1`).
2. At $T_1$, customer withdraws consent $\implies$ Event $E_2$ (`consent.withdrawn`, `aggregate_revision = 2`).
3. Due to network partition or worker retry delay, consumer receives $E_2$ first:
   - Consumer sets state for `(customer, email_marketing)` = `withdrawn`, stores revision = `2`.
4. Later, $E_1$ arrives:
   - Without Rule 1, consumer would set state = `granted`, illegally spamming the customer!
   - Under Rule 1, consumer inspects $R_{event} = 1 \le R_{local} = 2$.
   - **$E_1$ is discarded.** State remains `withdrawn` (Safe-Deny preserved).

### Rule 3: Broker Deduplication vs. Consumer Infinite Idempotency

- **JetStream Broker Deduplication:** Leverages `Nats-Msg-Id: <event_id>` with a configured 10-minute duplicate window. If a worker crashes mid-lease and re-publishes $E_1$, JetStream silently drops the duplicate within 10 minutes.
- **Consumer Monotonic Idempotency:** JetStream cannot protect against replay after 11 minutes or manual stream replay during disaster recovery. Consumers **must not** rely on broker deduplication for business consistency. Rule 1 provides infinite mathematical idempotency.

---

## 6. Wall-Clock Expiry & Pre-Send Evaluation Rule

### Rule 4: Passive Expiry Evaluation (No Wall-Clock Events)

1. **Zero Expiry Events:** The Nvuto-Consent system **DOES NOT** emit events on wall-clock expiry. Emitting millions of events when consents expire at midnight causes massive database and broker write spikes.
2. **Current-State Evaluation Requirement:**
   - Consumers sending outbound messages (especially **INT-01 Engage**, **INT-03 Contact Center**, and **INT-06 Conversations**) **MUST NOT** rely solely on the last received event to determine whether a grant is currently valid.
   - Consumers **MUST** either:
     a. Perform a synchronous HTTP evaluation against `POST /v1/evaluate` or `POST /v1/evaluate:batch` immediately before dispatch, OR
     b. Verify locally that `current_time < expires_at` AND that the associated purpose remains active.

### Rule 5: Safe-Deny on Failure or Uncertainty

If the Consent API is unreachable, times out, or returns a 5xx error, consumers **MUST DEFAULT TO SAFE-DENY**:
- Do **NOT** send the communication.
- Defer or retry the campaign execution.
- Never interpret a network timeout or system failure as consent.

---

## 7. Stream Replay & Disaster Recovery Reconciliation

When a consumer needs to rebuild its datastore from scratch (e.g., following database corruption or cache loss):
1. **Replay from Stream Start:** Consumer attaches a new JetStream consumer starting from `DeliverAll`.
2. **Sequential Revision Processing:** The consumer applies Rule 1. As the stream replays in arbitrary order across partitions, applying `aggregate_revision > local_revision` guarantees eventual convergence to the identical mathematical state.
3. **API Cross-Reconciliation:** High-volume consumers (e.g., INT-05 CRM, INT-08 Integration Hub) should execute periodic nightly reconciliation batches comparing local hashes against `POST /v1/evaluate:batch` to detect and repair any silent drift.

---

## 8. Consumer Implementation Checklist

Prior to receiving production event traffic, every consumer engineering lead MUST verify:

- [ ] Consumer stores `aggregate_revision` for every cached or projected consent record.
- [ ] Monotonic check (`event.aggregate_revision <= local.aggregate_revision`) drops stale events.
- [ ] Reverse-order delivery test (withdrawal arriving before grant) leaves permission revoked.
- [ ] Duplicate event replay beyond 10 minutes produces no side-effects.
- [ ] Wall-clock expiry is evaluated dynamically before sending any outbound communication.
- [ ] Outbound delivery fails closed (safe-deny) if Consent API is unreachable.
- [ ] No raw confirmation tokens or sensitive evidence hashes are logged in consumer output.
