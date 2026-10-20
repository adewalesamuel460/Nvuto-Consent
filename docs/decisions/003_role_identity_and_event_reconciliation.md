# Architecture Decision Record (ADR-003)

## Title: Role Composition, Production IdP Mapping, Idempotency Replay, and Event Reconciliation
- **Status:** PROPOSED / PENDING FORMAL SIGN-OFF
- **Date:** 2026-10-20
- **Decisions Addressed:** [D-06, D-10, D-11, D-13](../../docs/production-readiness-audit/20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md)
- **Governing Body:** Nvuto Architecture Review Board & Security/Privacy Steering Committee
- **Target Release:** Nvuto Consent Pilot / GA v1.0

---

## 1. Executive Summary & Purpose

This Architecture Decision Record (ADR) formalizes and captures binding owner approvals for four critical platform security, identity, data consistency, and event distribution decisions identified in the system audit:
1. **D-06 (Role Composition and Protected Suppression):** Defining authorization role hierarchy, mixed-role least-privilege scoping, and protecting legal suppressions against unauthorized self-service clearing.
2. **D-10 (Production IdP and Canonical Customer Graph Mapping):** Certifying the production Identity Provider (IdP) integration, token claims schema, and canonical party identifier mapping from the Customer Graph.
3. **D-11 (Idempotency Replay and Retention):** Establishing exact response replay vs. current representation semantics, actor rotation boundaries, and snapshot retention lifecycles for mutation endpoints.
4. **D-13 (Event Order, Expiry, and Consumer Reconciliation):** Establishing outbox event delivery ordering guarantees, consumer local cache reconciliation patterns, and passive vs. scheduled active consent expiry propagation.

---

## 2. Decision D-06: Role Composition and Protected Suppression

### Context & Problem Statement
The service recognizes seven distinct authorization roles (`privacy_admin`, `consent_admin`, `consent_writer`, `consent_reader`, `service`, `self_service`, and `developer`). Historically, a principal carrying multiple roles (e.g., `self_service` combined with `service` or `consent_reader`) risked unintentionally expanding customer-scoped privileges into tenant-wide write access. Furthermore, the platform must determine whether a data subject acting via `self_service` is permitted to clear suppressions placed administratively (e.g., legal opt-outs, fraud freezes, or compliance blocks).

### Evaluated Options

#### Option A: Strict Decoupled Role Scoping with Protected Administrative Suppressions
- **Architecture:** 
  - Every action is authorized independently against action-specific scope rules. A principal with mixed roles (e.g., `self_service` + `service`) is restricted to their canonical `customer_subject_id` for all write mutations; read-only roles never widen write permissions (T-030).
  - Suppressions carry an immutable `created_by_role` attribute. Suppressions placed by `privacy_admin`, `consent_admin`, or `compliance` are marked *Protected* and **cannot be removed** by a customer acting via `self_service`. Customers may only toggle their own self-managed suppressions.
- **Pros:** Maximum security, eliminates privilege escalation vectors, protects legal and regulatory compliance locks.
- **Cons:** Requires granular role checking and distinct error codes (`suppression_protected`).

#### Option B: Unified Role Hierarchy with Tenant-Wide Override
- **Architecture:**
  - Standard role inheritance where administrative roles strictly dominate. Any principal possessing `consent_writer` or `consent_admin` has tenant-wide write access, regardless of whether `self_service` is also present.
  - Suppressions are treated as uniform blocks that any writer role (or self-service customer for their own identity) can revoke.
- **Pros:** Simpler authorization logic, fewer operational edge cases for customer support agents.
- **Cons:** High risk of accidental cross-customer data writes if customer tokens accidentally receive internal service roles; cannot enforce immutable legal blocks.

#### Option C: Isolated Token Profiles (Dedicated Single-Role Tokens)
- **Architecture:** The Identity Provider enforces that a JWT carries **exactly one** functional role profile (`self_service` tokens can never contain service or admin roles). Tokens with conflicting role unions are rejected at the authentication gateway.
- **Pros:** Completely eliminates role-composition vulnerabilities at the perimeter.
- **Cons:** Shifts governance burden entirely onto IdP configuration; breaks multi-role administrator workflows unless accounts are segregated.

### Decision Selection (To be checked by Privacy and Identity/Security Leads)
- [ ] **Option A: Strict Decoupled Role Scoping with Protected Administrative Suppressions** *(Recommended)*
- [ ] **Option B: Unified Role Hierarchy with Tenant-Wide Override**
- [ ] **Option C: Isolated Token Profiles (IdP Single-Role Enforcement)**

### Technical & Security Constraints
1. **Developer Role Deprecation:** The `developer` role must be disabled or strictly rejected in production environments unless explicitly authorized via emergency break-glass procedure.
2. **Canonical Subject Binding:** A `self_service` principal **must** carry a non-empty `customer_subject_id` in their JWT; requests targeting any other `subject_id` must return a typed 403 `subject_not_authorized`.

### Executive Sign-Off (D-06)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Privacy Owner** | ____________________ | Data Protection Office | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Identity / Security Owner** | ____________________ | InfoSec & IAM Architecture | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

**Conditions / Comments:**  
________________________________________________________________________________________________________________________  
________________________________________________________________________________________________________________________

---

## 3. Decision D-10: Production IdP and Canonical Customer Graph Mapping

### Context & Problem Statement
Nvuto Consent does not own user authentication or customer identity. It relies on an external OIDC/OAuth2 Identity Provider (IdP) for RS256 JWT validation via JSON Web Key Sets (JWKS) and on the **Customer Graph** for canonical party identifiers. The production IdP provider, token lifetime, key rotation parameters, and canonical subject mapping must be formally certified.

### Evaluated Options

#### Option A: Authentik Enterprise IdP with Canonical Customer Graph Claims (EXT-01)
- **Architecture:**
  - Deploy Authentik as the authoritative enterprise IdP.
  - JWTs are signed with RS256 keys exposed via a public/internal JWKS endpoint (`JWT_JWKS_URL`).
  - Tokens incorporate custom claims mapped from the Customer Graph:
    - `tenant_id`: Authoritative organizational tenant UUID.
    - `actor_id`: User or service principal identity.
    - `customer_subject_id`: Authoritative Customer Graph party identifier (mandatory for `self_service`).
    - `roles`: String array of assigned RBAC permissions.
- **JWKS Cache Policy:** JWKS refresh throttled to 10s minimum interval, 5-minute cache TTL, and 24-hour maximum stale tolerance (T-014).
- **Pros:** Fully implemented and validated in codebase; supports fine-grained claim mapping; vendor-agnostic OIDC standard.
- **Cons:** Dependent on Authentik deployment availability and key rotation lifecycle.

#### Option B: Alternate Approved OIDC Provider (e.g., Okta, Auth0, Keycloak)
- **Architecture:** Replace Authentik with an alternative OIDC provider adhering to the identical RS256 JWKS and custom claims contract (`tenant_id`, `actor_id`, `roles`, `customer_subject_id`).
- **Pros:** Leverages pre-existing enterprise IAM contracts if already deployed across the organization.
- **Cons:** Requires re-testing token issuance pipelines and claim namespace mappings.

### Decision Selection (To be checked by Identity, Customer Graph, and Security Leads)
- [ ] **Option A: Authentik Enterprise IdP with Canonical Customer Graph Claims** *(Recommended)*
- [ ] **Option B: Alternate Approved OIDC Provider:** __________________________________

### Approved Claim Mapping Specification

```json
{
  "iss": "https://auth.nvuto.com/application/o/consent/",
  "aud": "nvuto-consent-api",
  "sub": "auth-user-987654",
  "tenant_id": "tenant-enterprise-prod",
  "actor_id": "usr-456789",
  "customer_subject_id": "party-canonical-12345",
  "roles": ["self_service"],
  "exp": 1780000000
}
```

### Executive Sign-Off (D-10)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Identity Platform Owner** | ____________________ | IAM / Directory Services | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Customer Graph Owner** | ____________________ | Customer Master Data | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Security Architecture Lead** | ____________________ | Enterprise Cybersecurity | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

---

## 4. Decision D-11: Idempotency Replay and Retention

### Context & Problem Statement
All mutative endpoints (`POST /v1/consents`, `POST /v1/consents/{id}/withdraw`, `POST /v1/suppressions`, `POST /v1/double-opt-in`) accept an `Idempotency-Key` header to ensure safe network retries. When a duplicate key arrives with an identical payload hash, the service must either replay the exact original HTTP response (including original timestamps and status codes) or return the current state of the database record. Furthermore, key collision scopes and cleanup lifecycles must be defined.

### Evaluated Options

#### Option 1: Exact Original Response Snapshot Replay (Current Baseline)
- **Model:**
  - Upon successful initial processing, the complete HTTP response body and status code are serialized into `consent_idempotency.response_snapshot` (added in migration 006).
  - Retried requests matching `(tenant_id, idempotency_key)` with identical payload hash return the cached `response_snapshot` with status `200 OK` or `201 Created`.
  - Retried requests with conflicting payload hashes are rejected with typed `409 Conflict`.
  - Scoped to `(tenant_id, idempotency_key)`: An idempotency key is unique per tenant.
- **Pros:** True RFC-compliant idempotent replay; client receives identical state without side effects; safe for automated network retries.
- **Cons:** Storage overhead in `consent_idempotency` table; requires automated retention purging.

#### Option 2: Current Entity State Re-Fetch (No Snapshot Storage)
- **Model:** The idempotency ledger only records key usage and the created resource ID. Retries re-query the target consent or suppression row and serialize the current database representation.
- **Pros:** Smaller table size; no JSON snapshot storage.
- **Cons:** Non-deterministic if the resource was subsequently withdrawn or modified by a concurrent process; violates exact replay semantics.

#### Option 3: Actor-Bound Idempotency Scoping
- **Model:** Idempotency keys are scoped to `(tenant_id, actor_id, idempotency_key)`. Different actors within the same tenant can reuse the same key without conflict.
- **Pros:** Prevents accidental key collision across distributed client processes.
- **Cons:** Does not protect against duplicate operations if actor tokens rotate mid-session.

### Decision Selection (To be checked by API, Consumer, and Privacy Leads)
- [ ] **Option 1: Exact Original Response Snapshot Replay** *(Recommended)*
- [ ] **Option 2: Current Entity State Re-Fetch**
- [ ] **Option 3: Actor-Bound Idempotency Scoping**

### Idempotency Operational Parameters
- **Snapshot Retention TTL:** [ ______ ] hours before automated cleanup (Default: **72 hours**).
- **In-Flight Lock Timeout:** Maximum duration an in-flight key is locked before considered abandoned: [ ______ ] seconds (Default: **30 seconds**).
- **Scope Boundary:** [x] `(tenant_id, idempotency_key)` &nbsp;&nbsp; [ ] `(tenant_id, actor_id, idempotency_key)`.

### Executive Sign-Off (D-11)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **API / Backend Lead** | ____________________ | Core Services Architecture | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Consumer Platform Lead** | ____________________ | Consumer Engineering | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Privacy Reviewer** | ____________________ | Compliance & Privacy | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

---

## 5. Decision D-13: Event Order, Expiry, and Consumer Reconciliation

### Context & Problem Statement
Nvuto Consent emits CloudEvents to NATS JetStream (`nvuto.consent.>`) whenever consent preferences are granted, withdrawn, or suppressed. However, distributed event brokers can occasionally deliver events out of order, network partitions can delay messages, and consents expire passively when `at > effective_until` without generating a discrete runtime event. The architecture for consumer-side consistency and expiry reconciliation must be approved.

### Evaluated Options

#### Option A: Consumer Authoritative Synchronous Pre-flight Gate
- **Architecture:** 
  - Consumers (e.g., Engage, Contact Center) treat NATS JetStream events merely as cache-invalidation hints.
  - Prior to dispatching any outbound campaign or communication, consumers execute a synchronous pre-flight query against `POST /v1/evaluate` or `POST /v1/evaluate:batch`.
  - Expiry is calculated dynamically at runtime; stale local caches cannot cause illegal communications.
- **Pros:** 100% compliance guarantee; zero risk of sending messages based on delayed JetStream events or unannounced passive expiries.
- **Cons:** Introduces synchronous HTTP latency into consumer outbound dispatch pipelines.

#### Option B: Monotonic Generation-Tracked Projections with Periodic Reconciliation
- **Architecture:**
  - Consumers maintain a local read replica or projected datastore of permissions, updated via JetStream outbox events.
  - Consumers track `preference_generation`: older generation updates are discarded to prevent out-of-order race conditions.
  - Integration Hub runs a daily/hourly reconciliation job querying Consent APIs to repair drifted local projections.
- **Pros:** Sub-millisecond local queries in consumer apps; high campaign throughput.
- **Cons:** Event drift window; requires sophisticated local cache logic in every consumer system.

#### Option C: Scheduled Active Expiry Event Publication
- **Architecture:** A scheduled cron/worker process periodically scans for consents where `effective_until <= now()` and generates explicit `consent.expired` outbox events to JetStream.
- **Pros:** Provides explicit push notifications when grants lapse.
- **Cons:** High write amplification in database during mass expirations; potential delay between legal expiry and event batch execution.

### Decision Selection (To be checked by Event Architecture, Integration Hub, and Consumer Leads)
- [ ] **Option A: Consumer Authoritative Synchronous Pre-flight Gate** *(Recommended for P0 Consumers)*
- [ ] **Option B: Monotonic Generation-Tracked Projections with Periodic Reconciliation**
- [ ] **Option C: Scheduled Active Expiry Event Publication**

### Event Governance Standards
1. **Deduplication Window:** JetStream stream deduplication window is locked to `2 minutes` matching worker lease duration.
2. **Event Payload Policy:** CloudEvent payloads contain entity identifiers, metadata, and status; **no raw credentials or sensitive customer evidence context** are emitted over NATS.
3. **Monotonic Sequence Rule:** Consumers projecting state **must** verify that `new_generation > current_generation` before overwriting local customer preferences.

### Executive Sign-Off (D-13)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Event Architecture Lead** | ____________________ | Messaging & Distributed Systems | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Integration Hub Owner** | ____________________ | Enterprise Integration Services | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Consumer Systems Lead** | ____________________ | Platform Consumer Engineering | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

---

## 6. Consolidated Master Approval Summary

This section consolidates final sign-offs across Decisions D-06, D-10, D-11, and D-13. Once all signatures are recorded, the status of this document advances from `PROPOSED` to `APPROVED`.

| Decision ID | Focus Area | Recommended Resolution | Primary Approvers | Status |
|---|---|---|---|---|
| **D-06** | Role Composition & Suppression | Option A: Decoupled Scoping + Protected Blocks | Privacy Owner, Security Owner | [ ] Pending [ ] Approved |
| **D-10** | Production IdP & Customer Graph | Option A: Authentik RS256/JWKS + Graph Claims | Identity Owner, Customer Graph, Security | [ ] Pending [ ] Approved |
| **D-11** | Idempotency Replay & Retention | Option 1: Exact Response Snapshot (72h TTL) | API Lead, Consumer Lead, Privacy | [ ] Pending [ ] Approved |
| **D-13** | Event Order & Reconciliation | Option A (Sync Gate) / Option B (Monotonic) | Event Architecture, Integration Hub, Consumer | [ ] Pending [ ] Approved |

### Executive Committee Acceptance

| Executive Sponsor | Title | Signature | Date |
|---|---|---|---|
| ________________________________________ | VP of Engineering | ________________________________________ | ____________________ |
| ________________________________________ | Head of Product | ________________________________________ | ____________________ |
| ________________________________________ | General Counsel / Chief Privacy Officer | ________________________________________ | ____________________ |
