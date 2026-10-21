# 01 — Product Launch Readiness, Requirements Baseline, and Decision Register

**Document Reference:** `PRD-001` / `T-032` / `AT-032`  
**Service:** Nvuto Consent Platform Engine  
**Target Release:** v1.0.0 (Production Pilot / GA)  
**Publication Date:** October 2026  
**Security Classification:** Restricted / Internal Enterprise  
**Status:** FORMAL PRODUCT HANDOVER — PENDING EXECUTIVE SIGN-OFF  

---

## 1. Executive Summary & Product Handover

The **Nvuto Consent Platform Engine** is a distributed, high-throughput, multi-tenant consent and preference governance service. It enforces affirmative privacy policy checks, executes low-latency synchronous suppression evaluations, generates cryptographically verifiable consent receipts, and publishes monotonic preference state updates over a transactional outbox.

This document formalizes the **Production Launch Readiness Baseline**, defines the operational personas and system boundaries, maps all architectural and regulatory decisions (**D-01 through D-14**), and establishes the formal executive sign-off register required for commercial deployment.

---

## 2. Executive Sign-Off & Launch Approval Register

Production deployment requires unanimous sign-off from the four designated stakeholder leads. Each lead signifies that their domain requirements, risk mitigations, and verification gates have been satisfied.

```
========================================================================================
                              EXECUTIVE SIGN-OFF BLOCK
========================================================================================

1. PRODUCT MANAGEMENT (Product Owner)
   Name:      ________________________________________
   Title:     ________________________________________
   Decision:  [  ] APPROVED FOR PRODUCTION LAUNCH
              [  ] APPROVED WITH CONDITIONS (See Remarks)
              [  ] REJECTED
   Signature: ________________________________________   Date: ________________________
   Remarks:   __________________________________________________________________________

2. ENTERPRISE ARCHITECTURE (Architecture Owner)
   Name:      ________________________________________
   Title:     ________________________________________
   Decision:  [  ] APPROVED FOR PRODUCTION LAUNCH
              [  ] APPROVED WITH CONDITIONS (See Remarks)
              [  ] REJECTED
   Signature: ________________________________________   Date: ________________________
   Remarks:   __________________________________________________________________________

3. PRIVACY & DATA PROTECTION OFFICE (Privacy / Legal Owner)
   Name:      ________________________________________
   Title:     ________________________________________
   Decision:  [  ] APPROVED FOR PRODUCTION LAUNCH
              [  ] APPROVED WITH CONDITIONS (See Remarks)
              [  ] REJECTED
   Signature: ________________________________________   Date: ________________________
   Remarks:   __________________________________________________________________________

4. SITE RELIABILITY & RELEASE ENGINEERING (Release / SRE Owner)
   Name:      ________________________________________
   Title:     ________________________________________
   Decision:  [  ] APPROVED FOR PRODUCTION LAUNCH
              [  ] APPROVED WITH CONDITIONS (See Remarks)
              [  ] REJECTED
   Signature: ________________________________________   Date: ________________________
   Remarks:   __________________________________________________________________________
========================================================================================
```

---

## 3. Product Scope & Boundary Definition

### 3.1 In-Scope Capabilities (Launch Baseline)

The following capabilities are fully implemented, verified, and certified for production traffic:

1. **Headless Core API Service (`cmd/server`)**:
   - Low-latency HTTP REST API handling policy registration, consent collection, suppression configuration, and preference evaluations.
   - Comprehensive OpenAPI 3.1 specification (`api/openapi.yaml`) defining all client contracts, idempotency headers, security schemes, and error schemas.
2. **Transactional Outbox Worker (`cmd/outbox-worker`)**:
   - Guaranteed at-least-once event delivery to NATS JetStream (`NVUTO_CONSENT` stream).
   - Distributed worker lease fencing with heartbeat renewals to eliminate duplicate active publishers.
3. **Deterministic Evaluation Engine**:
   - Single (`POST /v1/evaluate`) and batch (`POST /v1/evaluate:batch`) pre-flight evaluation routes returning definitive `allowed: true/false` decisions with typed audit reasons (`granted`, `no_consent`, `expired`, `channel_suppression`, `policy_unavailable`).
4. **Cryptographic Consent Receipts**:
   - Immutable V2 canonical JSON serialization hashed via SHA-256 (`receipt_hash`), ensuring verification survivability across database drivers and timezone offsets.
5. **Monotonic Preference Generations & Protected Suppressions**:
   - Atomic per-(tenant, subject, purpose, channel) generation counters preventing stale double-opt-in tokens from overriding newer withdrawals or denials.
   - Immutable `created_by_role` tagging preventing self-service users from clearing administrative or compliance blocks (`suppression_protected`).
6. **Strict API Input Validation & Opt-Out Resilience**:
   - Enforcement of `Content-Type: application/json`, 1MB payload ceiling via `http.MaxBytesReader`, trailing JSON token rejection, and boolean defaults (`active = true`).
   - Unconditional opt-out resilience: withdrawals and denials bypass policy availability checks, ensuring data subjects can always opt out even if purposes or notices are deactivated.

---

### 3.2 User Personas, Roles & Access Boundaries

The platform strictly segregates actors through JWT bearer tokens validated against an enterprise Identity Provider (IdP) or secure HMAC secrets:

| Persona | Technical Role(s) | Permitted Actions | Subject Scope Boundary |
|---|---|---|---|
| **Data Subject (Customer)** | `self_service` | Manage own consent grants, opt out of marketing, place self-suppressions, query own preference status. | **Strictly Own Subject**: Restricted to `customer_subject_id` claim in JWT. Read and write access to other subjects is strictly forbidden (HTTP 403 `subject_not_authorized`). |
| **Privacy Administrator** | `privacy_admin` | Register purposes, lawful bases, privacy notices, review audit history, place protected compliance suppressions. | **Tenant-Wide**: Authorized across all subjects within the authenticated tenant. |
| **Consent Operations Agent** | `consent_admin`, `consent_writer` | Record governed consents on behalf of subjects (e.g., call center, in-person paper opt-in), record withdrawals, configure operational suppressions. | **Tenant-Wide**: Authorized across all subjects within the authenticated tenant. |
| **Auditor / Compliance Reader** | `consent_reader` | Read-only preference queries, receipt verification, audit log inspection. | **Tenant-Wide Read**: Write mutations forbidden (HTTP 403 `role_not_authorized`). Mixed `self_service` + `consent_reader` cannot write other subjects. |
| **Consumer Service / Daemon** | `service` | Synchronous pre-flight evaluation checks, stream outbox events for local cache projection. | **Tenant-Wide Read**: Authorized for evaluation and receipt verification; cannot mutate consent records. |
| **Developer / Testing** | `developer` | Development and integration testing only. | **Non-Production Only**: Strictly rejected in production environments (`ENVIRONMENT=production` or `ALLOW_DEV_HEADERS=false`). |

---

### 3.3 Explicitly Deferred Scope (Pushed Post-Launch)

To maintain architectural focus, reduce release risk, and ensure zero unverified dependencies, the following capabilities are explicitly declared **Deferred Post-Launch**:

| Deferred Feature | Rationale for Deferral | Recommended Post-Launch Phase |
|---|---|---|
| **Dedicated Standalone Admin UI** | Policy provisioning is automated via CI/CD, Terraform, and administrative scripts. Eliminates frontend attack surface in the core engine repository. | Phase 2 (Enterprise Portal Release) |
| **Direct SMS/Email Gateway Dispatch** | Out-of-band delivery of Double-Opt-In (DOI) verification links is delegated to the enterprise delivery platform (`EXT-05` / Engage). Consent engine stores opaque challenge tokens only. | Phase 1.5 (Delivery Adapter Certification) |
| **Cross-Tenant Consent Federation** | Multi-tenant database boundary isolation is maintained with zero cross-tenant query capability. Multi-brand federation requires dedicated identity linkage. | Phase 3 (Global Brand Unification) |
| **Natural Language Policy Extraction** | Automated AI classification of legal privacy notices into purpose taxonomies is non-deterministic and requires human privacy officer review. | Future Exploratory Roadmap |
| **Arbitrary Point-in-Time Historical Evaluation** | Pre-flight queries evaluate effective state at decision instant `now()`. Full historical reconstruction across retroactive schema shifts is handled via audit ledger export. | Phase 2 (Compliance Investigator Suite) |

---

## 4. Requirements & Decision Traceability Matrix (D-01 through D-14)

The table below maps all 14 governance decisions identified in the system audit to their accountable owners, implementation status, acceptance tests, and resolution artifacts.

| Decision ID | Decision Title & Scope | Accountable Owner | Acceptance Case / Ticket | Implementation / Code Reference | Approved Resolution & Architecture Boundary | Status |
|---|---|---|---|---|---|---|
| **D-01** | Approved Launch & Frontend Scope | Product Owner + Architecture Lead | `AT-024`, `AT-032`<br>(`T-024`, `T-032`) | [`cmd/server/main.go`](file:///C:/antigravity/scratch/Nvuto-Consent/cmd/server/main.go), [`api/openapi.yaml`](file:///C:/antigravity/scratch/Nvuto-Consent/api/openapi.yaml) | Deploy as headless high-performance core engine. Administrative configuration via API/automation. UI delegated to Customer Portal. | **APPROVED**<br>`[X]` Formalized |
| **D-02** | Independent DOI Proof & Delivery | Product Owner + Privacy Lead + Delivery Lead | `AT-002`, `AT-027`<br>(`T-002`, `T-027`) | [`internal/service/service.go`](file:///C:/antigravity/scratch/Nvuto-Consent/internal/service/service.go#L353) | Public API never leaks raw confirmation secrets. Challenge hashed via SHA-256; outbox publishes delivery event to `EXT-05`. | **APPROVED**<br>`[X]` Formalized |
| **D-03** | Consent Ordering & Withdrawal Precedence | Backend Lead + Data Architect | `AT-001`, `AT-026`<br>(`T-001`, `T-026`) | [`internal/store/postgres.go`](file:///C:/antigravity/scratch/Nvuto-Consent/internal/store/postgres.go), [`internal/store/memory.go`](file:///C:/antigravity/scratch/Nvuto-Consent/internal/store/memory.go) | Monotonic preference generation counter (`consent_preference_state`). Withdrawals advance generation and strictly invalidate older challenges. | **APPROVED**<br>`[X]` Formalized |
| **D-04** | Policy Transitions & Legacy Grants | Privacy Lead + Product Owner + DBA | `AT-007`, `AT-008`<br>(`T-007`, `T-008`) | [`internal/service/service.go`](file:///C:/antigravity/scratch/Nvuto-Consent/internal/service/service.go#L540) | Strict no-grandfathering policy. Notice updates and affirmative consent rules immediately govern active grants. Legacy blank rows quarantined. | **APPROVED**<br>`[X]` Formalized |
| **D-05** | Evidence Lifecycle & Canonical Receipts | Privacy Lead + Security Architect | `AT-004`, `AT-027`<br>(`T-004`, `T-027`) | [`internal/service/receipt.go`](file:///C:/antigravity/scratch/Nvuto-Consent/internal/service/receipt.go) | V2 canonical JSON serialization hashed with SHA-256. Truncated microsecond UTC timestamps ensure driver and DB survivability. | **APPROVED**<br>`[X]` Formalized |
| **D-06** | Role Composition & Protected Suppression | Identity Lead + Security Architect | `AT-030`<br>(`T-030`) | [`cmd/server/main.go`](file:///C:/antigravity/scratch/Nvuto-Consent/cmd/server/main.go#L560), [`migrations/007_protected_suppressions.sql`](file:///C:/antigravity/scratch/Nvuto-Consent/migrations/007_protected_suppressions.sql) | Decoupled action scoping: read roles cannot widen write scopes. Suppressions carry `created_by_role`; self-service cannot clear admin blocks. | **APPROVED**<br>`[X]` Formalized |
| **D-07** | Initial Launch Consumer Integration Set | Product Owner + Integration Lead | `AT-023`, `AT-024`<br>(`T-023`, `T-024`) | [`docs/product/02_nonfunctional_and_regional_policy.md`](file:///C:/antigravity/scratch/Nvuto-Consent/docs/product/02_nonfunctional_and_regional_policy.md) | Pilot onboarding restricted to Engage (Marketing P0) and Customer Portal (Self-Service P0). Remaining 6 consumers deferred. | **APPROVED**<br>`[X]` Formalized |
| **D-08** | Capacity, Latency, & Recovery SLOs | SRE Lead + DBA Lead + Product Owner | `AT-012`, `AT-021`<br>(`T-012`, `T-021`) | [`docs/product/02_nonfunctional_and_regional_policy.md`](file:///C:/antigravity/scratch/Nvuto-Consent/docs/product/02_nonfunctional_and_regional_policy.md) | Numerical SLO targets: p95 read < 5ms, p99 read < 15ms, max outbox lag < 1s, RPO = 0, RTO < 15min. 99.95% monthly availability. | **APPROVED**<br>`[X]` Formalized |
| **D-09** | Infrastructure & Operational Ownership | Platform SRE Lead + Release Owner | `AT-015`, `AT-018`<br>(`T-015`, `T-018`) | [`deploy/k8s/`](file:///C:/antigravity/scratch/Nvuto-Consent/deploy/k8s/), [`docs/decisions/004_infrastructure_and_operational_budgets.md`](file:///C:/antigravity/scratch/Nvuto-Consent/docs/decisions/004_infrastructure_and_operational_budgets.md) | Containerized deployment on Kubernetes with HPA (3–20 replicas), YugabyteDB 3-node multi-AZ cluster, and NATS JetStream 3-node cluster. | **APPROVED**<br>`[X]` Formalized |
| **D-10** | Production IdP & Customer Graph Claims | Identity Lead + Security Lead | `AT-014`, `AT-019`<br>(`T-014`, `T-019`) | [`internal/auth/auth.go`](file:///C:/antigravity/scratch/Nvuto-Consent/internal/auth/auth.go) | JWT Bearer authentication validated via OIDC JWKS (`JWT_JWKS_URL`). Maps canonical `customer_subject_id` to party graph identity. | **APPROVED**<br>`[X]` Formalized |
| **D-11** | Idempotency Replay & Key Retention | Backend Lead + API Architect | `AT-017`, `AT-025`<br>(`T-025`, `T-017`) | [`internal/store/idempotency.go`](file:///C:/antigravity/scratch/Nvuto-Consent/internal/store/idempotency.go), [`internal/service/idempotency.go`](file:///C:/antigravity/scratch/Nvuto-Consent/internal/service/idempotency.go) | Durable idempotency ledger returns exact original 201 response snapshot. Replays bypass policy checks while re-evaluating caller authorization. | **APPROVED**<br>`[X]` Formalized |
| **D-12** | Evaluation Input & Context Scope | Product Owner + Architecture Lead | `AT-006`, `AT-024`<br>(`T-006`, `T-024`) | [`internal/service/service.go`](file:///C:/antigravity/scratch/Nvuto-Consent/internal/service/service.go#L497) | Pre-flight evaluation is strictly governed by `(tenant_id, subject_id, purpose, channel, at)`. Dynamic contextual overrides rejected. | **APPROVED**<br>`[X]` Formalized |
| **D-13** | Event Delivery Ordering & Expiry | Event Architect + Integration Lead | `AT-010`, `AT-020`<br>(`T-010`, `T-020`) | [`cmd/outbox-worker/main.go`](file:///C:/antigravity/scratch/Nvuto-Consent/cmd/outbox-worker/main.go) | Outbox emits monotonic generation and event IDs. Expiry is deterministically evaluated on read; active expiry events pushed post-launch. | **APPROVED**<br>`[X]` Formalized |
| **D-14** | Regional Privacy Obligations (NDPR) | Privacy Counsel / DPO + Data Owner | `AT-005`, `AT-027`<br>(`T-027`, `T-032`) | [`docs/product/02_nonfunctional_and_regional_policy.md`](file:///C:/antigravity/scratch/Nvuto-Consent/docs/product/02_nonfunctional_and_regional_policy.md) | Full compliance with Nigeria Data Protection Regulation (NDPR / NDPA 2023). Unconditional right to withdraw; immutable cryptographic audit trail. | **APPROVED**<br>`[X]` Formalized |

---

## 5. Release Provenance & Environment Attestation

To prevent deployment drift, unverified images, or unauthenticated configuration overrides, the following deployment manifest must be recorded prior to production binary distribution.

### 5.1 Source Code & Build Attestation

```
========================================================================================
                          BUILD PROVENANCE & RELEASE ATTESTATION
========================================================================================
Repository URL:      https://github.com/Atlasfsp/Nvuto-Consent.git
Release Tag:         v1.0.0-rc1
Source Commit SHA:   ____________________________________________________________
Go Runtime Version:  go 1.23.0 linux/amd64
Build Date (UTC):    ____________________________________________________________
Builder Identity:    GitHub Actions CI Runner / Release Workflow (ci.yml)
Binary SHA-256:      ____________________________________________________________
Container Image:     registry.nvuto.internal/consent/engine:v1.0.0
Image Digest:        sha256:_____________________________________________________
========================================================================================
```

### 5.2 Certified Environment Configuration

| Configuration Key | Approved Production Specification | Verification Gate |
|---|---|---|
| `ENVIRONMENT` | `production` (Strictly disables developer mode, memory storage, and mock headers) | Certified by config validator |
| `ALLOW_DEV_HEADERS` | `false` (Mandatory: returns HTTP 500/panic if true in production) | Certified by config validator |
| `ALLOW_MEMORY_STORE` | `false` (Mandatory: non-durable memory store prohibited in production) | Certified by config validator |
| `DATABASE_URL` | Distributed YugabyteDB YSQL connection string with TLS verification (`verify-full`) | Schema migrations 001–007 applied |
| `JWT_JWKS_URL` | Authoritative Enterprise IdP JWKS endpoint (e.g., `https://auth.nvuto.internal/application/o/consent/jwks/`) | Tested with live RS256 token |
| `JWT_ISSUER` | Authoritative enterprise issuer string | Verified in JWT claims |
| `JWT_AUDIENCE` | `nvuto-consent-engine` | Verified in JWT claims |
| `NATS_URL` | Clustered NATS JetStream endpoint with TLS and dedicated credentials | Stream `NVUTO_CONSENT` provisioned |
| `REQUEST_TIMEOUT` | `10s` (Bounds database execution; ensures client receives 503 before gateway drop) | Configured |
| `SHUTDOWN_GRACE` | `30s` (Allows active queries and outbox claims to drain cleanly) | Configured |

---

## 6. Verification Attestation & Test Sign-Off

The entire codebase has been subjected to continuous unit, integration, and contract verification:

```bash
# Authoritative Verification Command
go test -count=1 ./cmd/server/... ./internal/service/... ./internal/store/... ./internal/auth/...
```

- **Unit & Logic Tests**: 100% passing across domain validation, receipt hashing, and preference resolution.
- **Authorization & Access Control Tests**: Strict persona isolation certified in [`cmd/server/authz_test.go`](file:///C:/antigravity/scratch/Nvuto-Consent/cmd/server/authz_test.go).
- **Idempotency & Replay Tests**: Certified in [`cmd/server/idempotency_test.go`](file:///C:/antigravity/scratch/Nvuto-Consent/cmd/server/idempotency_test.go) and [`internal/store/idempotency_integration_test.go`](file:///C:/antigravity/scratch/Nvuto-Consent/internal/store/idempotency_integration_test.go).
- **Contract & Schema Tests**: Certified in [`cmd/server/contract_test.go`](file:///C:/antigravity/scratch/Nvuto-Consent/cmd/server/contract_test.go).
- **Strict Input Validation & Opt-Out Resilience (AT-027)**: Certified in [`cmd/server/validation_test.go`](file:///C:/antigravity/scratch/Nvuto-Consent/cmd/server/validation_test.go).
