# Architecture Decision Record (ADR-001)

## Title: Launch Scope, Consumer Integration, Evaluation Context, and Regional Privacy Obligations
- **Status:** PROPOSED / PENDING FORMAL SIGN-OFF
- **Date:** 2026-10-20
- **Decisions Addressed:** [D-01, D-07, D-12, D-14](../../docs/production-readiness-audit/20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md)
- **Governing Body:** Nvuto Architecture Review Board & Privacy Steering Committee
- **Target Release:** Nvuto Consent Pilot / GA v1.0

---

## 1. Executive Summary & Purpose

This Architecture Decision Record (ADR) formalizes and captures binding owner approvals for the four foundational governance and scope decisions required prior to production rollout of the **Nvuto Consent** service:
1. **D-01 (Approved Launch & Frontend Scope):** Defining service boundaries, UI ownership, and deployment tiers.
2. **D-07 (Initial Consumer Set & Integration Ownership):** Designating launch eligibility, integration patterns, and engineering ownership across the eight named platform consumers.
3. **D-12 (Evaluation Scope & Jurisdiction Context):** Establishing whether evaluation decisions remain purpose/channel-driven or incorporate runtime dynamic jurisdiction extensions.
4. **D-14 (Regional Privacy Obligations & Evidence Handling):** Establishing applicable regulatory frameworks, lawful basis requirements, evidence retention timelines, and Right-to-Delete (erasure) compliance boundaries.

---

## 2. Decision D-01: Approved Launch and Frontend Scope

### Context & Problem Statement
The Nvuto Consent service is designed as the centralized engine for consent, preference, and lawful-basis evaluation. However, the exact launch perimeter—whether deployed as a headless core API, integrated with select high-priority consumers, or mandated platform-wide with a dedicated administrative UI—requires executive alignment.

### Evaluated Options

#### Option A: Headless Core API Service (Standalone Backend)
- **Scope:** Deploy high-availability API (`cmd/server`) and Outbox Worker (`cmd/outbox-worker`) without a dedicated user interface.
- **Frontend / Interaction:** Administrative configuration (purposes, lawful bases, privacy notices) is performed via CI/CD automation, Terraform, or internal administrative API tooling. Data-subject self-service preference management is delegated to existing frontend applications (e.g., Customer Portal).
- **Pros:** Fast time-to-market, clean domain separation, zero frontend codebase bloat in Consent repository.
- **Cons:** Dependent on consumer applications or script tooling for notice and purpose provisioning.

#### Option B: Phased Consumer Rollout (Pilot Launch)
- **Scope:** Headless API deployment integrated exclusively with 2–3 pilot consumer workflows (e.g., Engage marketing campaigns and Customer Portal self-service).
- **Frontend / Interaction:** No dedicated administrative UI; pilot consumers validate the API contract and outbox event consumption in staging before general rollout.
- **Pros:** Controlled blast radius, allows validation of JetStream deduplication and YSQL latency under real consumer workloads.
- **Cons:** Other platform consumers remain on legacy or unmonitored communication paths during the pilot phase.

#### Option C: Full Platform Big-Bang Launch with Dedicated Administrative UI
- **Scope:** Simultaneous launch across all 8 platform consumers, backed by a newly developed administrative UI for privacy officers.
- **Pros:** Immediate enterprise-wide enforcement across all communication channels.
- **Cons:** High coordination overhead, delayed release, significantly increased delivery risk.

### Decision Selection (To be checked by Product & Architecture Leads)
- [ ] **Option A: Headless Core API Service (Standalone Backend)**
- [ ] **Option B: Phased Consumer Rollout (Pilot Launch)** *(Recommended)*
- [ ] **Option C: Full Platform Big-Bang Launch**

### Implementation Boundaries & Constraints
1. **Identity & Campaign Separation:** Nvuto Consent **does not own** customer identity (owned by Customer Graph) or outbound campaign dispatch (owned by Engage).
2. **Repository Perimeter:** No frontend web application code shall be committed to the `Nvuto-Consent` repository without an explicit architectural amendment.

### Executive Sign-Off (D-01)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Product Owner** | ____________________ | Product Management | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Architecture Lead** | ____________________ | Enterprise Architecture | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

**Conditions / Comments:**  
________________________________________________________________________________________________________________________  
________________________________________________________________________________________________________________________

---

## 3. Decision D-07: Initial Consumer Set and Integration Ownership

### Context & Problem Statement
Nvuto platform architecture references eight primary consumer systems. To ensure operational stability, message deduplication, and SLA compliance, each consumer must have an identified engineering owner, a declared integration pattern, and a formal scope determination (In Scope for Launch vs. Deferred).

### Integration Patterns
- **Pattern 1: Synchronous Pre-flight Gate:** Consumer queries `POST /v1/evaluate` or `POST /v1/evaluate:batch` synchronously prior to dispatching communication.
- **Pattern 2: Asynchronous Event Streaming:** Consumer subscribes to NATS JetStream `NVUTO_CONSENT` (`nvuto.consent.>`) to maintain a local projected cache of preferences/suppressions.
- **Pattern 3: Hybrid (Synchronous Evaluation + Asynchronous Invalidation):** Consumer evaluates synchronously for high-stakes flows and listens to JetStream for proactive cache invalidation.

### Consumer Inclusion & Ownership Matrix

| Consumer System | Function / Role | Scope Status | Approved Integration Pattern | Designated Engineering Owner | Target Staging Date | Sign-Off Signature |
|---|---|---|---|---|---|---|
| **1. Engage** | Outbound marketing campaigns, newsletters, SMS broadcasts | [ ] In Scope (P0)<br>[ ] Deferred | [ ] Synchronous<br>[ ] JetStream Stream<br>[ ] Hybrid | ____________________ | ____________ | ____________________ |
| **2. Service** | Transactional service alerts, ticket notifications | [ ] In Scope (P0)<br>[ ] Deferred | [ ] Synchronous<br>[ ] JetStream Stream<br>[ ] Hybrid | ____________________ | ____________ | ____________________ |
| **3. Contact Center** | Telephony, outbound agent dialing, IVR | [ ] In Scope (P1)<br>[ ] Deferred | [ ] Synchronous<br>[ ] JetStream Stream<br>[ ] Hybrid | ____________________ | ____________ | ____________________ |
| **4. Customer Portal** | End-user self-service preference management & opt-outs | [ ] In Scope (P0)<br>[ ] Deferred | [ ] Synchronous<br>[ ] JetStream Stream<br>[ ] Hybrid | ____________________ | ____________ | ____________________ |
| **5. CRM** | Customer relationship profiles, sales outreach | [ ] In Scope (P1)<br>[ ] Deferred | [ ] Synchronous<br>[ ] JetStream Stream<br>[ ] Hybrid | ____________________ | ____________ | ____________________ |
| **6. Conversations** | Real-time chat, WhatsApp, customer messaging | [ ] In Scope (P1)<br>[ ] Deferred | [ ] Synchronous<br>[ ] JetStream Stream<br>[ ] Hybrid | ____________________ | ____________ | ____________________ |
| **7. AI / Agents** | Automated conversational AI, generative agents | [ ] In Scope (P2)<br>[ ] Deferred | [ ] Synchronous<br>[ ] JetStream Stream<br>[ ] Hybrid | ____________________ | ____________ | ____________________ |
| **8. Integration Hub** | Third-party webhooks, external partner exports | [ ] In Scope (P1)<br>[ ] Deferred | [ ] Synchronous<br>[ ] JetStream Stream<br>[ ] Hybrid | ____________________ | ____________ | ____________________ |

### Engineering Lead Approval (D-07)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Platform Engineering Lead** | ____________________ | Core Platform Engineering | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

---

## 4. Decision D-12: Evaluation Scope and Jurisdiction Context

### Context & Problem Statement
Currently, `POST /v1/evaluate` resolves consent using `(tenant_id, subject_id, purpose, channel, at)`. The system verifies affirmative consent requirements, active status, effective timestamps, notice version compliance, and suppression rules. We must formalize whether runtime evaluation requires dynamic jurisdiction query parameters (e.g., passing `jurisdiction=NG` or `jurisdiction=EU` at evaluation time) or whether jurisdiction remains bound strictly to the accepted `privacy_notice` version linked during consent capture.

### Evaluated Options

#### Option 1: Deterministic Purpose/Channel/Notice Resolution (Current Architecture)
- **Model:** Evaluation relies deterministically on `(tenant_id, subject_id, purpose, channel)`. The applicable jurisdiction is derived directly from the governing `privacy_notice` version linked to the consent grant.
- **Evaluation Semantics:**
  - If a purpose requires affirmative consent, an active, unwithdrawn grant with a valid notice version must exist.
  - If a suppression exists (global, channel, or purpose-channel), evaluation returns `granted: false` with the suppression reason.
- **Pros:** High performance (<5ms p99), cache-friendly, simple integration contract for consumers.
- **Cons:** Consumers cannot request dynamic cross-border evaluation on the fly without updating the subject's notice/grant.

#### Option 2: Dynamic Multi-Jurisdictional Context Evaluation
- **Model:** Consumers pass client location/jurisdiction dynamically in the evaluation payload (`{"subject_id": "...", "purpose": "...", "jurisdiction": "NG"}`).
- **Pros:** Accommodates transient travellers or dynamic multi-region compliance rules.
- **Cons:** Increases evaluation latency, requires complex regional rule tables, and risks non-deterministic compliance outcomes.

### Decision Selection (To be checked by Product & Privacy Leads)
- [ ] **Option 1: Deterministic Purpose/Channel/Notice Resolution (Current Architecture)** *(Recommended)*
- [ ] **Option 2: Dynamic Multi-Jurisdictional Context Evaluation**

### Historical & Future Evaluation Policy (`at` parameter)
- [ ] **Current-time Evaluation Only:** If `at` is omitted or zero, evaluate at current UTC time. If `at` is supplied in the past, evaluate historical state (supported up to audit retention limit).
- [ ] **Future Evaluation Prohibited:** Requests with `at > now() + 5 minutes` must return a typed 400 validation error (`invalid_timestamp`).

### Executive Sign-Off (D-12)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Lead Architect** | ____________________ | Solution Architecture | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Privacy Officer** | ____________________ | Legal & Regulatory Compliance | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

---

## 5. Decision D-14: Regional Privacy Obligations and Evidence Handling

### Context & Problem Statement
Privacy legislation (e.g., Nigeria Data Protection Act [NDPA 2023], GDPR, CCPA/CPRA) imposes distinct requirements on lawful basis validity, proof of affirmative consent, audit trail integrity, and data subject rights. This section records legal determination of launch jurisdictions, retention periods, and the critical tension between the **Right-to-Delete (Erasure)** and **Immutable Audit Receipts**.

### 5.1 Target Launch Jurisdictions

Please select all jurisdictions authorized for initial release:
- [ ] **Nigeria (NDPA / NDPR):** Baseline launch jurisdiction.
- [ ] **European Union / United Kingdom (GDPR / UK-GDPR):** Requires strict affirmative opt-in and granular lawful basis.
- [ ] **United States - California (CCPA / CPRA):** Requires opt-out suppression compliance and "Do Not Sell/Share" support.
- [ ] **Other Jurisdiction(s):** ____________________________________________________________________

---

### 5.2 Retention Schedule and Archival Policy

Define the required retention periods for consent records, receipts, and audit history:

| Data Category | Target Retention Period | Mandatory Destruction / Archival Mechanism |
|---|---|---|
| **Active Consent Grants** | Active lifetime + [ ____ ] years post-withdrawal | Soft-delete / status flag `withdrawn` |
| **Consent Receipts (Proof)** | [ ____ ] years from generation | Append-only YSQL table (`consent_receipts`) |
| **Audit Events Log** | [ ____ ] years from occurrence | Partitioned immutable table (`consent_audit_events`) |
| **Outbox Events (NATS)** | [ ____ ] hours/days post-publish | JetStream retention purge (default: 7 days) |
| **Idempotency Snapshots** | [ ____ ] hours/days from creation | Periodic cleanup (default: 72 hours) |

---

### 5.3 Right-to-Delete (Erasure) vs. Immutable Audit Policy

When a data subject submits an Article 17 / NDPA erasure request ("Right to be Forgotten"):

**Legal Determination of Technical Architecture:**
- [ ] **Standard Option (Suppression Tombstoning):** The customer's active consent grants are withdrawn/erased, but an immutable tombstone record is created in `consent_suppressions` (`scope='global'`) referencing an opaque irreversible pseudonym or hash of the party ID. This prevents accidental re-subscription or subsequent unauthorized marketing while fulfilling erasure requirements.
- [ ] **Cryptographic Erasure (Zero-Knowledge Shredding):** Customer subject IDs are keyed via a tenant encryption salt. Erasure destroys the key, rendering historic receipts and audit log entries cryptographically unreadable.

**Mandatory Legal Boundary:**  
Does Legal/Privacy confirm that keeping hashed consent receipts (`consent_receipts.receipt_hash`) and audit logs for regulatory defense complies with applicable privacy laws?  
- [ ] **YES — Compliance defense exemption applies.** Audit trails and cryptographic proofs are retained in accordance with statutory limitation periods.
- [ ] **NO — Full destructive purging required.** Custom purging workflows must be specified before GA.

---

### 5.4 Privacy & Legal Sign-Off (D-14)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Data Protection Officer (DPO)** | ____________________ | Data Privacy & Governance | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Chief Legal Counsel** | ____________________ | Legal & Regulatory Affairs | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

**Legal Conditions & Regulatory Notes:**  
________________________________________________________________________________________________________________________  
________________________________________________________________________________________________________________________  
________________________________________________________________________________________________________________________

---

## 6. Consolidated Master Approval Summary

This section consolidates final approval across all four architectural decisions. Once fully executed, this record will be committed to the repository and change status from `PROPOSED` to `APPROVED`.

| Decision ID | Area | Recommended Option | Approver Roles | Sign-Off Status |
|---|---|---|---|---|
| **D-01** | Launch & Frontend Scope | Option B (Phased Pilot) | Product Owner + Architecture Lead | [ ] Pending [ ] Approved |
| **D-07** | Consumer Integration | Hybrid Pattern / Engage & Portal Pilot | Platform Engineering Lead | [ ] Pending [ ] Approved |
| **D-12** | Evaluation Context | Option 1 (Deterministic Notice-Bound) | Lead Architect + Privacy Officer | [ ] Pending [ ] Approved |
| **D-14** | Regional Privacy & Evidence | NDPA/NDPR + Suppression Tombstoning | DPO + Legal Counsel | [ ] Pending [ ] Approved |

### Executive Committee Acceptance

| Executive Sponsor | Title | Signature | Date |
|---|---|---|---|
| ________________________________________ | VP of Engineering | ________________________________________ | ____________________ |
| ________________________________________ | Head of Product | ________________________________________ | ____________________ |
| ________________________________________ | General Counsel / Chief Privacy Officer | ________________________________________ | ____________________ |
