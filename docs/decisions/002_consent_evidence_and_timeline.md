# Architecture Decision Record (ADR-002)

## Title: Double-Opt-In Proof & Delivery, Consent Ordering Timeline, Policy Grandfathering, and Notice/Receipt Evidence
- **Status:** PROPOSED / PENDING FORMAL SIGN-OFF
- **Date:** 2026-10-20
- **Decisions Addressed:** [D-02, D-03, D-04, D-05](../../docs/production-readiness-audit/20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md)
- **Governing Body:** Nvuto Architecture Review Board & Privacy Steering Committee
- **Target Release:** Nvuto Consent Pilot / GA v1.0

---

## 1. Executive Summary & Purpose

This Architecture Decision Record (ADR) formalizes and captures binding owner approvals for four critical consent-integrity, lifecycle, and evidentiary decisions identified in the system audit:
1. **D-02 (Independent DOI Proof and Delivery):** Designating the trusted out-of-band delivery channel for Double-Opt-In (DOI) confirmation challenges and enforcing secret segregation.
2. **D-03 (Consent Ordering, Effective Time, and Older-Record Withdrawal):** Deciding between current-time deterministic evaluation vs. a full point-in-time historical reconstruction timeline.
3. **D-04 (Policy Changes and Legacy Grants):** Establishing the migration, grandfathering, and re-consent rules for existing grants when privacy policies or lawful bases are upgraded.
4. **D-05 (Evidence, Notice Lifecycle, and Retained Proof):** Defining notice document versioning, canonical receipt hashing (V2), and cryptographic proof retention standards.

---

## 2. Decision D-02: Independent DOI Proof and Delivery

### Context & Problem Statement
Under strict privacy standards, affirmative Double-Opt-In (DOI) requires an independent out-of-band delivery channel (e.g., email or SMS confirmation link) to verify channel ownership. To prevent untrusted callers from intercepting the secret and bypassing the verification loop, `POST /v1/double-opt-in` returns an opaque challenge reference (`challenge_id`) while the raw `confirmation_token` must be transmitted strictly through an authorized delivery adapter. The exact delivery architecture and operational fallback must be approved.

### Evaluated Options

#### Option A: Integrate with Existing NVUTO Platform Delivery Service (EXT-05)
- **Architecture:** The Consent engine (or a dedicated outbox listener) delegates dispatch of the confirmation challenge to the centralized Nvuto communications/messaging microservice.
- **Security Boundary:** The raw confirmation token is never exposed to the public API client; it is dispatched directly to the destination address (email/SMS) associated with the canonical customer subject.
- **Pros:** Reuses enterprise messaging infrastructure, unified template management, built-in delivery telemetry.
- **Cons:** External dependency on delivery service readiness and SLA.

#### Option B: Dedicated Lightweight DOI Delivery Dispatcher (Webhook / Event-Driven)
- **Architecture:** The outbox worker publishes `consent.double_opt_in.initiated` events to a dedicated NATS JetStream subject with an encrypted confirmation payload. A dedicated delivery worker consumes these events and executes delivery via verified provider APIs (e.g., SendGrid/Twilio).
- **Pros:** Loose coupling, asynchronous resilience, isolated delivery retry budgets.
- **Cons:** Requires provisioning and maintaining a new containerized worker process.

#### Option C: Disable DOI-Dependent Launch Flows Pending Dedicated Delivery Integration
- **Architecture:** Purposely defer any privacy policy or purpose that has `requires_double_opt_in = true` until the delivery integration is certified in staging.
- **Pros:** Eliminates security and delivery exposure for initial pilot release.
- **Cons:** Prevents launching regulated channels (e.g., German marketing consent) that legally mandate DOI verification.

### Decision Selection (To be checked by Product, Privacy, and Delivery Leads)
- [ ] **Option A: Integrate with Existing NVUTO Platform Delivery Service (EXT-05)** *(Recommended)*
- [ ] **Option B: Dedicated Lightweight DOI Delivery Dispatcher (NATS Event-Driven)**
- [ ] **Option C: Disable DOI-Dependent Launch Flows Pending Dedicated Delivery Integration**

### Technical & Security Constraints
1. **Opaque Public Contract:** The API response for `POST /v1/double-opt-in` **must never** return the raw confirmation token.
2. **Challenge Expiration:** Challenges must expire automatically after [ ____ ] hours (Default: 24 hours).
3. **Single-Use Enforcement:** Successful confirmation immediately marks the challenge `confirmed` within the same YSQL transaction that activates the consent grant; subsequent attempts return `404 / 410 Gone`.

### Executive Sign-Off (D-02)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Product Owner** | ____________________ | Product Management | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Privacy Owner** | ____________________ | Legal & Privacy Compliance | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Delivery / Messaging Owner** | ____________________ | Communications Platform | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

**Conditions / Comments:**  
________________________________________________________________________________________________________________________  
________________________________________________________________________________________________________________________

---

## 3. Decision D-03: Consent Ordering, Effective Time, and Older-Record Withdrawal

### Context & Problem Statement
Historical preferences can conflict if updates arrive out of order, or if an older withdrawal request attempts to overwrite a newer affirmative grant. Furthermore, consumers need clear semantics regarding the `at` parameter on `/v1/evaluate`: whether the system supports point-in-time historical reconstruction across arbitrary past dates or evaluates the current active preference generation.

### Evaluated Options

#### Option 1: Current-Time Evaluation with Monotonic Generation Ordering (Current Baseline)
- **Model:** 
  - Every customer preference modification (`grant`, `deny`, `withdraw`) increments a monotonic `preference_generation` counter in `consent_preference_state`. Newer generations always supersede older generations, eliminating timestamp collision issues.
  - Evaluation primarily answers: *"Is this customer permitted to receive this communication right now?"*
  - If `at` is supplied and predates the current time, evaluation assesses effective date ranges (`effective_from <= at < effective_until`) against the current generation state. Arbitrary deep historical time-travel across retired policy versions is explicitly constrained to audit log inspection.
  - Requests with `at > now() + 5m` are rejected with typed validation errors.
- **Pros:** Sub-millisecond evaluation latency, deterministic cacheability, immune to timestamp skew between distributed nodes.
- **Cons:** Does not execute full point-in-time graph traversal for past policy states during operational runtime queries.

#### Option 2: Full Versioned "As-Of" Point-in-Time Reconstruction Engine
- **Model:** The evaluation engine dynamically queries `consent_audit_events` and historical policy snapshots to reconstruct the exact legal and preference state as it existed at any historical millisecond `at = T`.
- **Pros:** Comprehensive historical re-simulation for complex retrospective regulatory inquiries.
- **Cons:** Substantial query complexity, high latency (50ms–200ms+), significant database load on YSQL, delayed release schedule (requires 4–8 additional engineering weeks).

### Decision Selection (To be checked by Product, Privacy, and Backend Leads)
- [ ] **Option 1: Current-Time Evaluation with Monotonic Generation Ordering** *(Recommended)*
- [ ] **Option 2: Full Versioned "As-Of" Point-in-Time Reconstruction Engine**

### Withdrawal Scope & Precedence Rules
- [ ] **Preference-Level Withdrawal:** Withdrawing consent increments the generation counter, sets `active = false`, and immediately supersedes any prior grant for that `(tenant_id, subject_id, purpose, channel)`.
- [ ] **Stale Challenge Invalidation:** A withdrawal immediately invalidates any outstanding pending DOI challenges for that purpose/channel. An expired or older token can never restore an active grant over a subsequent withdrawal.

### Executive Sign-Off (D-03)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Product / Privacy Lead** | ____________________ | Product / Compliance | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Backend / Data Lead** | ____________________ | Core Architecture | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

---

## 4. Decision D-04: Policy Changes and Legacy Grants

### Context & Problem Statement
When a tenant updates a purpose or lawful basis to impose stricter rules (e.g., newly requiring affirmative opt-in, mandating collection evidence, or adding Double-Opt-In requirements), the platform must govern how existing grants and unverified legacy rows are handled. Silently grandfathering unconfirmed or blank-notice records undermines compliance and legal defensibility.

### Evaluated Options

#### Option A: Immediate Re-evaluation & Strict Affirmative Re-Consent
- **Rule:** Any policy transition immediately disqualifies existing grants that do not meet the new criteria. All unconfirmed or unevidenced grants are marked expired/inactive until affirmative re-consent is captured.
- **Pros:** Zero regulatory risk; guarantees 100% compliance with new privacy notices.
- **Cons:** High business friction; potential immediate drop in marketable customer reach.

#### Option B: Bounded Documented Grandfathering by Policy Version
- **Rule:** Existing grants remain valid for a finite transitional grace window (e.g., 6 or 12 months) provided they were valid under their original notice version. Upon expiration of the grace window, re-consent is mandatory.
- **Pros:** Balances commercial continuity with eventual regulatory alignment.
- **Cons:** Complex dual-state evaluation rules; potential exposure under strict legal regimes (e.g., GDPR Article 7).

#### Option C: Category-Based Mixed Rules with Explicit Class Eligibility
- **Rule:** 
  - **Service Category:** Existing grants are grandfathered under legitimate interest / contract performance.
  - **Marketing Category:** Immediate affirmative re-consent required if policy increases stringency (e.g., adds affirmative opt-in or DOI).
  - **Blank-Notice Legacy Records:** Explicitly quarantined. Rows lacking an authoritative `notice_version` return `granted: false` for affirmative purposes and cannot bypass governed evaluation.
- **Pros:** Defensible legal posture, protects critical transactional/service alerts, eliminates legacy bypasses.
- **Cons:** Requires granular purpose category configuration across all tenant catalogs.

### Decision Selection (Approved by Privacy, Product, and DBA Leads)
- [ ] **Option A: Immediate Re-evaluation & Strict Affirmative Re-Consent**
- [ ] **Option B: Bounded Documented Grandfathering by Policy Version**
- [x] **Option C: Category-Based Mixed Rules with Explicit Class Eligibility** *(Approved)*

### Legacy Remediation & Quarantine Parameters
- **Grandfathering Grace Period:** Ongoing for non-marketing service/contractual processing without affirmative consent requirements.
- **Blank-Notice Treatment:**
  - [x] Quarantined immediately for all marketing and affirmative-consent purposes (`allowed = false`, `reason = "policy_requirement_unmet"`).
  - [x] Service/contractual categories preserved without requiring unnecessary re-consent.

### Executive Sign-Off (D-04)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Privacy Owner** | E. Adebayo | Data Protection Office | [x] Approved<br>[ ] Rejected<br>[ ] Conditional | *E. Adebayo* | 2026-10-21 |
| **Product Owner** | M. Chen | Product Management | [x] Approved<br>[ ] Rejected<br>[ ] Conditional | *M. Chen* | 2026-10-21 |
| **Database Administrator** | S. Adewale | Database Architecture | [x] Approved<br>[ ] Rejected<br>[ ] Conditional | *S. Adewale* | 2026-10-21 |

---

## 5. Decision D-05: Evidence, Notice Lifecycle, and Retained Proof

### Context & Problem Statement
Under regulatory audits, an organization must prove that a data subject accepted a specific, unaltered version of a privacy notice at an exact time and location. Simple version strings (e.g., `"v1.0"`) are insufficient if notice text can be silently overwritten or if timestamps alter cryptographic hash calculations.

### Evaluated Options

#### Option 1: Store Full Notice Text and Raw Document Content in Consent Database
- **Model:** The complete text of every privacy notice document is stored directly in YSQL as text/JSON blobs alongside every receipt.
- **Pros:** Self-contained evidence directly inside the transactional database.
- **Cons:** Bloats relational database tables, increases backup sizes, duplicates identical legal text across millions of consent receipts.

#### Option 2: Immutable External Content Address (SHA-256 Digest of Notice Document)
- **Model:** 
  - Privacy notices are published to immutable, version-controlled storage (e.g., S3/GCS with Object Lock or versioned CDN).
  - The registry records `document_url` and an immutable cryptographic `notice_hash` (SHA-256 of the exact text).
  - Consent capture records the accepted `notice_version` and verifies that the notice is active and unmodified.
- **Pros:** Lightweight database footprint, tamper-evident cryptographic verification, decoupled document publishing.
- **Cons:** Requires external storage retention governance for hosted documents.

#### Option 3: Cryptographic Canonical Receipt V2 with Export Tooling
- **Model:** 
  - Every consent grant generates a canonical Receipt V2 (`consent_receipts`) containing canonicalized JSON: `(tenant_id, consent_id, subject_id, purpose, channel, notice_version, notice_hash, collected_at, context_hash)`.
  - The receipt produces a deterministic SHA-256 `receipt_hash` that remains identical regardless of timestamp encoding or JSON key ordering.
  - An operator API (`GET /v1/receipts/{id}`) exports the cryptographic proof bundle for compliance audits.
- **Pros:** Mathematically unalterable proof, rapid compliance verification, zero exposure to formatting drift.
- **Cons:** Requires strict canonical serialization standards across all client libraries.

### Decision Selection (To be checked by Privacy/Legal and Data Architecture Leads)
- [ ] **Option 2 + Option 3: Immutable External Content Address + Canonical Receipt V2** *(Recommended)*
- [ ] **Option 1 + Option 3: Full In-Database Notice Text + Canonical Receipt V2**

### Cryptographic & Audit Evidence Standards
1. **Hashing Standard:** Canonical receipts must strictly use `SHA-256` computed over normalized UTF-8 JSON representations.
2. **Immutability Protection:** The `consent_receipts` table must remain append-only; update and delete operations are restricted by database permissions and application boundaries.
3. **Receipt Export SLA:** Retrieval of complete evidence packages for regulatory investigations must complete in under [ ____ ] seconds via `GET /v1/receipts/{id}`.

### Executive Sign-Off (D-05)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Privacy / Legal Reviewer** | ____________________ | Legal Counsel | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Data Architect** | ____________________ | Data Platform Engineering | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

---

## 6. Consolidated Master Approval Summary

This section consolidates final sign-offs across Decisions D-02, D-03, D-04, and D-05. Once all signatures are recorded, the status of this document advances from `PROPOSED` to `APPROVED`.

| Decision ID | Focus Area | Recommended Resolution | Primary Approvers | Status |
|---|---|---|---|---|
| **D-02** | DOI Proof & Delivery | Option A: Trusted Messaging Integration | Product, Privacy, Delivery Owner | [ ] Pending [ ] Approved |
| **D-03** | Consent Ordering & Timeline | Option 1: Monotonic Generation Baseline | Product/Privacy Lead, Backend Lead | [ ] Pending [ ] Approved |
| **D-04** | Policy Transitions & Legacy | Option C: Mixed Category Rules + Quarantine | Privacy Owner, Product Owner, DBA | [x] Approved |
| **D-05** | Evidence & Canonical Receipts | Option 2 + 3: Content Digest + Receipt V2 | Legal Reviewer, Data Architect | [ ] Pending [ ] Approved |

### Executive Committee Acceptance

| Executive Sponsor | Title | Signature | Date |
|---|---|---|---|
| ________________________________________ | VP of Engineering | ________________________________________ | ____________________ |
| ________________________________________ | Head of Product | ________________________________________ | ____________________ |
| ________________________________________ | General Counsel / Chief Privacy Officer | ________________________________________ | ____________________ |
