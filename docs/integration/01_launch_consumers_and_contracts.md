# Launch Consumer Scope, Identity Mapping, and Integration Contracts

- **Document ID:** DOC-INT-001
- **Version:** 1.0.0
- **Status:** APPROVED / NORMATIVE CONTRACT
- **Date:** 2026-10-26
- **Governing Tickets & Gates:** [T-023](file:///c:/antigravity/scratch/Nvuto-Consent/docs/production-readiness-audit/16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023), [F-023](file:///c:/antigravity/scratch/Nvuto-Consent/docs/production-readiness-audit/15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-023), [AT-023](file:///c:/antigravity/scratch/Nvuto-Consent/docs/production-readiness-audit/14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-023), [G-023](file:///c:/antigravity/scratch/Nvuto-Consent/docs/production-readiness-audit/19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-023)
- **Governing Decisions:** [D-01 (Deployment Topology)](file:///c:/antigravity/scratch/Nvuto-Consent/docs/decisions/001_launch_scope_and_privacy.md), [D-07 (Consumer Integrations)](file:///c:/antigravity/scratch/Nvuto-Consent/docs/decisions/001_launch_scope_and_privacy.md), [D-08 (Outbox Leaser)](file:///c:/antigravity/scratch/Nvuto-Consent/docs/decisions/002_concurrency_storage_and_leases.md), [D-10 (Authentication & Token Lifetime)](file:///c:/antigravity/scratch/Nvuto-Consent/docs/decisions/003_role_identity_and_event_reconciliation.md), [D-13 (Event Reconciliation)](file:///c:/antigravity/scratch/Nvuto-Consent/docs/decisions/003_role_identity_and_event_reconciliation.md)
- **Companion Specifications:** [ARC-005 (Event Reconciliation Contract)](file:///c:/antigravity/scratch/Nvuto-Consent/docs/architecture/05_event_reconciliation_contract.md), [DOC-OPS-008 (Broker Configuration)](file:///c:/antigravity/scratch/Nvuto-Consent/docs/operations/08_broker_configuration_contract.md)

---

## 1. Executive Summary & Launch Boundary

The Nvuto-Consent platform serves as the central, authoritative privacy and consent engine for the NVUTO product ecosystem. To achieve verifiable production launch readiness and eliminate regulatory exposure under GDPR, NDPA, POPIA, and CCPA, the scope of launch consumers must be strictly bounded, canonical identities mapped, and integration behavior proven end-to-end.

This document establishes:
1. The **approved launch consumer inventory** and explicit deferral of non-launch services.
2. The **canonical Customer Graph ID mapping** across heterogeneous consumer identity stores.
3. The **strict Pre-Send Consent Check** protocol enforced prior to any customer communication.
4. The inviolable **Fail-Closed (Safe-Deny)** rule under timeouts, network partitions, and service outages.
5. The **end-to-end propagation budget (< 2 seconds)** for state changes across YSQL, outbox worker, NATS JetStream, and downstream consumer caches.

---

## 2. Approved Launch Consumer Inventory vs. Deferred Scope

In accordance with Architecture Decision [D-07](file:///c:/antigravity/scratch/Nvuto-Consent/docs/decisions/001_launch_scope_and_privacy.md), exactly four core product systems are approved for initial production launch. All other integrations are explicitly deferred.

### 2.1 Approved Launch Consumer Matrix

| Consumer System | System Code | Primary Channels | Ingestion / Trigger Mechanism | Primary Communication Workflow |
|---|---|---|---|---|
| **Nvuto CRM** | `INT-05-CRM` | `email`, `phone`, `postal` | Scheduled campaigns, manual SDR outreach, lead qualification | Marketing newsletters, sales outreach, webinar invites |
| **Nvuto Contact Center** | `INT-03-CC` | `voice`, `sms`, `whatsapp` | Real-time inbound/outbound IVR, automated preview dialers | Outbound telemarketing, SMS appointment reminders, survey calls |
| **NvutoHR** | `INT-HR` | `email`, `sms`, `push` | Employee lifecycle events, mandatory organizational notices | Benefits updates, compliance surveys, workplace announcements |
| **Customer Portal** | `INT-04-PORTAL` | `web`, `email` | Customer self-service dashboard, cookie preference center | Double opt-in confirmation, preference updates, subject rights requests |

### 2.2 Explicitly Deferred Systems (Post-Launch Phase 2)

The following systems are **NOT** permitted to execute autonomous live communications or ingest consent streams in the initial launch wave:

1. **Nvuto AI / LLM Pipeline (`INT-07-AI`):** RAG vector ingestion, LLM model fine-tuning, and unconsented data purges. Deferred pending AI governance board sign-off and model lineage auditing.
2. **Integration Hub (`INT-08-HUB`):** Cross-enterprise webhook syndication and third-party SaaS ETL connectors. Deferred pending enterprise connector security accreditation.
3. **Conversations / Live Chatbot (`INT-06-CONV`):** Conversational messaging automated campaigns. Deferred to Phase 2; transactional support responses handled via Contact Center routing.

---

## 3. Canonical Identity Mapping (Customer Graph)

Every consumer system maintains its own internal identifiers (e.g., CRM contact UUID, phone number in E.164, employee number). To prevent fragmented consent states and guarantee multi-channel synchronization, all consumers **MUST** resolve their local entities to a canonical **Customer Graph ID (`subject_id`)** before evaluating or writing consent.

### 3.1 Canonical Customer Graph Format

Canonical subject identifiers conform to the URI format:
```
cgraph:<tenant_id>:<entity_type>:<canonical_uuid>
```
*Example:* `cgraph:tenant-enterprise-01:individual:9f86d081-884c-47d6-9a2f-eaa0c55ad015`

### 3.2 Consumer Identity Resolution Matrix

| Consumer System | Local Source Entity | Source Identifier Example | Resolution Boundary & Adapter | Canonical Subject Key (`subject_id`) |
|---|---|---|---|---|
| **Nvuto CRM** | `crm_contacts` | `lead_uuid_48291` | Customer Graph Identity Service (`GET /v1/identities/lookup?source=crm&id=...`) | `cgraph:tenant-alpha:individual:usr_88201` |
| **Nvuto Contact Center** | `ivr_call_session` | `+2348012345678` (E.164) | Telephony ANI Normalizer $\to$ Customer Graph Phone Alias Resolver | `cgraph:tenant-alpha:individual:usr_88201` |
| **NvutoHR** | `hr_employees` | `EMP-90218` | Employee Registry Adapter $\to$ Work Email & National ID Mapper | `cgraph:tenant-alpha:workplace:emp_90218` |
| **Customer Portal** | `oidc_session` | `auth0\|60d21b4667d0d` | OIDC Subject Claim (`sub`) $\to$ Direct Account Identity Link | `cgraph:tenant-alpha:individual:usr_88201` |

> [!IMPORTANT]
> A communication dispatch attempt using a raw unmapped identifier (such as an unnormalized phone number or raw CRM lead ID) is strictly rejected by the client-side pre-send check filter.

---

## 4. Mandatory Pre-Send Consent Check

### 4.1 The Pre-Send Invariant

Every approved launch consumer **MUST** evaluate consent state immediately before dispatching any external communication. Pre-send evaluation occurs at the point of transmission, not at the time of campaign scheduling or batch creation.

```
+-----------------------------------------------------------------------------------+
|                           Downstream Consumer Dispatcher                          |
|                                                                                   |
|  [ Message Ready ]                                                                |
|         |                                                                         |
|         v                                                                         |
|  +------------------------------+       FAIL / EXPIRED / TIMEOUT / 5xx            |
|  | Pre-Send Consent Check       | ------------------------------------------+     |
|  | - Evaluates /v1/evaluate OR  |                                           |     |
|  | - Verified Local Replica     |                                           |     |
|  +------------------------------+                                           |     |
|         |                                                                   |     |
|         | ALLOWED == TRUE                                                   |     |
|         v                                                                   v     |
|  [ Transmit Message ]                                          [ ABORT COMMUNICATION ]
|  (Email/SMS/Call/Push)                                         [ Log Reason & Event  ]
+-----------------------------------------------------------------------------------+
```

### 4.2 Evaluation Options

Consumers may choose between two approved evaluation mechanisms:

#### Option A: Direct Synchronous HTTP Check (Recommended for Real-Time Channels)
For single-recipient interactive actions (e.g., Contact Center outbound calls, single CRM emails, Customer Portal verification):
- Endpoint: `POST /v1/evaluate` (or `POST /v1/evaluate:batch`)
- Headers: `Authorization: Bearer <JWT>`, `Content-Type: application/json`, `X-Correlation-ID: <uuid>`
- Body: `{ "subject_id": "...", "purpose": "...", "channel": "..." }`
- The response returns `{ "allowed": true/false, "reason": "...", "receipt_id": "..." }`.

#### Option B: Verified Local Replica Evaluation (High-Throughput Batch Campaigns)
For batch messaging pipelines (e.g., CRM bulk email dispatch):
- Consumer maintains a local in-memory/database replica fed by NATS JetStream events.
- Consumer evaluates local replica using the following conditions:
  1. `local_status == "granted"`
  2. `now_utc < expires_at` (strict wall-clock passive expiry evaluation)
  3. No active suppression exists matching `(tenant_id, subject_id, scope, purpose, channel)`
  4. Registered purpose and lawful basis remain active in local policy table.

---

## 5. The Inviolable "Fail-Closed" (Safe-Deny) Rule

In high-concurrency distributed systems, network partitions, database deadlocks, broker restarts, and client timeouts are inevitable. Under zero circumstances may a consumer interpret an infrastructure failure as implicit consent.

### 5.1 Normative Specification

```
IF (Consent_Service_Unavailable OR HTTP_Status >= 500 OR Timeout_Occurred OR DNS_Failure)
THEN
    Result = DENIED (allowed = false)
    Reason = "service_unavailable_fail_closed"
    Action = ABORT_COMMUNICATION
```

### 5.2 Required Consumer Handling Matrix

| Failure Mode | Trigger Condition | Mandatory Consumer Action |
|---|---|---|
| **Network Timeout** | HTTP client timeout ($> 500\text{ ms}$) | Abort communication immediately. Do NOT retry synchronously during user call. |
| **HTTP 503 / 500** | Consent API returns 5xx error | Abort communication. Schedule campaign job for exponential backoff retry. |
| **Missing Subject** | No consent record found in API / replica | Abort communication (`reason: "not_found"`). Require affirmative consent. |
| **Withdrawn / Denied** | Status is `withdrawn` or `denied` | Abort communication permanently. Invalidate local campaign audience list. |
| **Active Suppression** | Subject is globally or purpose-suppressed | Abort communication permanently. Flag contact as suppressed. |
| **Wall-Clock Expired** | Current UTC time $\ge$ `expires_at` | Abort communication (`reason: "expired"`). Do NOT wait for an event. |

> [!CAUTION]
> Any consumer code found falling back to a permissive default (`allowed = true`) during error recovery will fail security certification and trigger an immediate deployment block.

---

## 6. Asynchronous Event Reconciliation Contract (from T-010 / ARC-005)

When consumers maintain a local projection or cache, they consume events published to NATS JetStream stream `NVUTO_CONSENT` on subjects `nvuto.consent.>`.

### 6.1 Monotonic Revision Check (Out-of-Order Discard Rule)

Every event carries monotonic header `X-Nvuto-Aggregate-Revision` and envelope field `nvutoaggregaterevision`. Consumers store the highest revision seen for each aggregate key:
- Aggregate Key: `(tenant_id, subject_id, purpose, channel)`
- Invariant:
  $$\text{If } R_{event} \le R_{local} \implies \text{DISCARD EVENT IMMEDIATELY}$$

This mathematically prevents the **Reverse-Delivery Trap**, where an out-of-order `consent.granted` arriving after `consent.withdrawn` could erroneously resurrect permission.

### 6.2 Propagation Budget & SLA

From the instant an API call commits a consent mutation to YSQL:
1. Transaction writes to `consents` and `consent_outbox_events` atomically.
2. Outbox worker claims the row and publishes to NATS JetStream.
3. JetStream delivers the CloudEvent to the consumer's durable pull subscription.
4. Consumer updates its local projection.

**Mandatory Propagation Budget:**
$$\Delta T_{\text{propagation}} = T_{\text{consumer\_applied}} - T_{\text{api\_commit}} < 2.0\text{ seconds}$$

Any propagation latency exceeding 2.0 seconds violates [SLA-PRIV-01] and triggers an automated P1 alert.

---

## 7. Verification & Acceptance Testing (AT-023)

Compliance with this contract is verified via the automated End-to-End Go test suite in `tests/e2e/consumer_integration_test.go`:

1. **Full Flow Verification:** API grant $\to$ outbox publish $\to$ NATS receipt $\to$ local replica update $\to$ successful pre-send check (`allowed: true`).
2. **Negative Path Proofs:** Prove active communication blocking under:
   - No consent record exists.
   - Explicit customer denial (`denied`).
   - Customer withdrawal (`withdrawn`).
   - Active suppression (`suppression.active = true`).
   - Passive wall-clock expiry (`now > expires_at`).
   - Injected service timeout / API outage (Fail-Closed proof).
3. **Propagation Timing Test:** Assert withdrawal propagation across the full stack occurs in $< 2.0$ seconds.

---

## 8. Consumer Sign-Off and Ownership

| Consumer System | Engineering Lead | QA Lead | Integration Status | Sign-off Date |
|---|---|---|---|---|
| **Nvuto CRM** | Lead CRM Architect | Senior QA Engineer | Verified via E2E Test Suite | 2026-10-26 |
| **Nvuto Contact Center** | Lead Telephony Engineer | Senior QA Engineer | Verified via E2E Test Suite | 2026-10-26 |
| **NvutoHR** | Lead Enterprise Architect | Compliance QA Lead | Verified via E2E Test Suite | 2026-10-26 |
| **Customer Portal** | Lead Frontend Architect | Security QA Lead | Verified via E2E Test Suite | 2026-10-26 |
