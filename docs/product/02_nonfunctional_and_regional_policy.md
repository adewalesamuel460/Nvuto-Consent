# 02 — Nonfunctional Targets, Capacity SLOs, Regional Policy, and Operating Model

**Document Reference:** `PRD-002` / `T-032` / `AT-032`  
**Service:** Nvuto Consent Platform Engine  
**Target Release:** v1.0.0 (Production Pilot / GA)  
**Publication Date:** October 2026  
**Security Classification:** Restricted / Internal Enterprise  
**Status:** FORMAL PRODUCT HANDOVER — PENDING EXECUTIVE SIGN-OFF  

---

## 1. Executive Summary & Purpose

This document establishes the measurable **Nonfunctional Requirements (NFRs)**, **Service Level Objectives (SLOs)**, **Regional Regulatory Obligations (NDPR Compliance)**, and the **Business Operating Model** for the Nvuto Consent Platform Engine.

Operating as the critical pre-flight gateway for customer communications across the enterprise, the service enforces zero-trust privacy governance under sub-millisecond to low-millisecond latency budgets and guarantees high-availability persistence backed by distributed consensus.

---

## 2. Executive Sign-Off Register (Operational & Regulatory)

Production authorization requires formal verification and sign-off from the designated operational, database, regional legal, and commercial leads:

```
========================================================================================
                     OPERATIONAL & REGULATORY SIGN-OFF BLOCK
========================================================================================

1. PLATFORM RELIABILITY & INFRASTRUCTURE (SRE Lead)
   Name:      ________________________________________
   Title:     ________________________________________
   Decision:  [  ] APPROVED CAPACITY & LATENCY BUDGETS
              [  ] APPROVED WITH CONDITIONS (See Remarks)
              [  ] REJECTED
   Signature: ________________________________________   Date: ________________________
   Remarks:   __________________________________________________________________________

2. DATABASE ARCHITECTURE & CONTINUITY (Lead DBA)
   Name:      ________________________________________
   Title:     ________________________________________
   Decision:  [  ] APPROVED STORAGE, RPO & RTO BUDGETS
              [  ] APPROVED WITH CONDITIONS (See Remarks)
              [  ] REJECTED
   Signature: ________________________________________   Date: ________________________
   Remarks:   __________________________________________________________________________

3. REGIONAL PRIVACY COMPLIANCE (Named Regional Privacy Review Owner / NDPR Counsel)
   Name:      ________________________________________
   Title:     Regional Data Protection Officer (Africa / Nigeria)
   Decision:  [  ] APPROVED NDPR & JURISDICTION COMPLIANCE
              [  ] APPROVED WITH CONDITIONS (See Remarks)
              [  ] REJECTED
   Signature: ________________________________________   Date: ________________________
   Remarks:   __________________________________________________________________________

4. COMMERCIAL & OPERATING FINANCE (Finance / Commercial Lead)
   Name:      ________________________________________
   Title:     Director of Platform Commercials & Operations
   Decision:  [  ] APPROVED OPERATING MODEL & COST RECOVERY
              [  ] APPROVED WITH CONDITIONS (See Remarks)
              [  ] REJECTED
   Signature: ________________________________________   Date: ________________________
   Remarks:   __________________________________________________________________________
========================================================================================
```

---

## 3. Capacity Targets & Service Level Objectives (SLOs)

### 3.1 Peak Traffic & Throughput Capacity

Traffic sizing accounts for steady-state evaluation traffic, peak marketing blast surges (e.g., Engage campaign dispatch), and bulk compliance audits:

| Throughput Metric | Ingress API Route | Pilot Phase Target | Production GA Target | Peak Burst Capacity (10 min) | Measurement Method |
|---|---|---|---|---|---|
| **Synchronous Single Evaluations** | `POST /v1/evaluate` | 1,000 req/sec | 5,000 req/sec | 10,000 req/sec | Prometheus `http_requests_total{path="/v1/evaluate"}` |
| **Synchronous Batch Evaluations** | `POST /v1/evaluate:batch` | 200 req/sec<br>*(20k items/s)* | 500 req/sec<br>*(50k items/s)* | 1,000 req/sec<br>*(100k items/s)* | Prometheus `http_requests_total{path="/v1/evaluate:batch"}` |
| **Preference Writes & Grants** | `POST /v1/consents` | 100 writes/sec | 500 writes/sec | 1,000 writes/sec | Prometheus `http_requests_total{path="/v1/consents",method="POST"}` |
| **Immediate Opt-Outs / Withdrawals** | `POST /v1/consents/{id}/withdraw` | 50 writes/sec | 250 writes/sec | 500 writes/sec | Prometheus `http_requests_total{path=~".*/withdraw"}` |
| **Suppression Updates** | `POST /v1/suppressions` | 50 writes/sec | 200 writes/sec | 500 writes/sec | Prometheus `http_requests_total{path="/v1/suppressions"}` |
| **Double-Opt-In Confirmations** | `POST /v1/double-opt-in/confirm` | 50 writes/sec | 200 writes/sec | 400 writes/sec | Prometheus `http_requests_total{path=~".*/confirm"}` |

---

### 3.2 Latency Budgets & Execution Thresholds

Pre-flight checks directly gate outbound communications. Strict latency budgets prevent communication bottlenecks across consumers:

| Endpoint & Action | Target p95 Latency | Target p99 Latency | Max Allowable SLA Ceiling | Enforcement Mechanism |
|---|---|---|---|---|
| `POST /v1/evaluate` (Read Gate) | **< 5 ms** | **< 15 ms** | 50 ms | Microsecond read lock in store; bounded query execution |
| `POST /v1/evaluate:batch` (up to 100 items) | **< 20 ms** | **< 45 ms** | 100 ms | In-memory evaluation loop; fail-fast validation |
| `POST /v1/consents` (Write Grant) | **< 25 ms** | **< 60 ms** | 150 ms | Single YSQL transaction (consent + receipt + outbox + audit) |
| `POST /v1/consents/{id}/withdraw` (Withdrawal) | **< 15 ms** | **< 40 ms** | 100 ms | Generation increment + direct update + outbox emit |
| `GET /v1/consents` (List Preferences) | **< 15 ms** | **< 35 ms** | 100 ms | Indexed query bounded by `limit <= 100` |
| `GET /v1/receipts/{id}` (Receipt Verification) | **< 8 ms** | **< 20 ms** | 50 ms | Primary key lookup on `tenant_id, receipt_id` |

---

### 3.3 Event Propagation & Invalidation Budgets

Outbox events published via `cmd/outbox-worker` update downstream caches across NATS JetStream subscribers:

```
[Consent Write Committed in YSQL]
                │
                ▼ (Outbox Polling & Claim Lease: Target < 500ms)
[Worker Claims Outbox Event]
                │
                ▼ (NATS JetStream Publish: Target < 200ms)
[Event Published to NVUTO_CONSENT Stream]
                │
                ▼ (Consumer Invalidation: Target < 1000ms)
[Consumer Local Cache Cleared]
```

| Budget Parameter | Target Objective | Warning Alert Threshold | Critical Paging Alert Threshold |
|---|---|---|---|
| **Outbox Claim-to-Publish Lag** | **< 1.0 second** | > 2.0 seconds for 2 min | > 5.0 seconds for 1 min |
| **End-to-End Suppression Lag** | **< 2.0 seconds** | > 5.0 seconds for 2 min | > 10.0 seconds for 1 min |
| **Outbox Worker Lock Lease** | **2 minutes** | Worker heartbeat missed > 30s | Worker heartbeat missed > 60s |
| **Max Unprocessed Outbox Backlog** | **< 500 events** | > 2,000 events | > 5,000 events |

---

### 3.4 Availability, RPO, and RTO Budgets

| Objective | Standard Budget | Operational Target Specification |
|---|---|---|
| **Monthly Service Availability** | **99.95%** | Maximum allowable unplanned downtime: **21.9 minutes / calendar month**. Measured via external synthetic probes querying `/readyz` every 10 seconds. |
| **Error Budget Consumption** | **< 0.05%** | Non-client HTTP 5xx responses must not exceed 0.05% of total ingress requests over any rolling 30-day window. |
| **Recovery Point Objective (RPO)** | **RPO = 0** | **Zero committed transaction loss**. Guaranteed by YugabyteDB distributed Raft consensus (Replication Factor = 3 across 3 distinct Availability Zones). Every committed consent receipt is synchronously replicated to a quorum before client ACK. |
| **Recovery Time Objective (RTO)** | **RTO < 15 minutes** | Automated pod recovery within Kubernetes (< 60s). Multi-node database AZ failover (< 10s). Full regional cluster restore from continuous WAL / PITR backup completed in < 15 minutes. |

---

## 4. Regional Compliance & Legal Framework (NDPR / NDPA 2023)

### 4.1 Operating Jurisdiction: Nigeria (NG)

The Nvuto Consent service is deployed with **NG (Nigeria)** as its primary regulatory jurisdiction, matching the hardcoded test fixtures (`jurisdiction: "NG"`, `notice_version: "2026-08"` / `"2026-10"`).

The regulatory framework governing this jurisdiction is:
1. **Nigeria Data Protection Act 2023 (NDPA)**
2. **Nigeria Data Protection Regulation 2019 (NDPR)**
3. **NDPC General Guidelines on Data Protection in Nigeria**

---

### 4.2 Regional Policy Obligations & Architectural Controls

| Legal Obligation | NDPR / NDPA Reference | Architectural Enforcement in Nvuto Consent |
|---|---|---|
| **Affirmative, Freely Given Consent** | NDPA 2023 Sec 26(1)<br>NDPR Reg 2.1(a) | Enforced via `requires_affirmative_consent = true`. Pre-ticked boxes, implied consent, and blanket lawful basis reuse are strictly rejected by the policy engine. |
| **Independent Verification for Sensitive Data** | NDPA 2023 Sec 30<br>NDPR Reg 2.2(d) | Double-Opt-In (DOI) support via `requires_double_opt_in = true`. Generates single-use cryptographic challenges requiring explicit customer activation. |
| **Clear & Transparent Privacy Notice** | NDPA 2023 Sec 27<br>NDPR Reg 2.1(b) | Privacy notices are versioned, jurisdiction-bound (`jurisdiction = "NG"`), and immutable once published. Every consent grant permanently binds the exact published version. |
| **Unconditional Right to Withdraw** | NDPA 2023 Sec 35<br>NDPR Reg 2.1(d) | **Opt-Out Resilience (T-027)**: A data subject can withdraw consent or record a denial at any time. Withdrawals and denials bypass policy availability checks, ensuring opt-outs cannot be blocked by retired notices, inactive purposes, or missing evidence. |
| **Cryptographic Proof of Consent** | NDPA 2023 Sec 26(2) | Generates V2 canonical consent receipts containing collection method, artifact SHA-256 hash, notice version, and microsecond UTC timestamp, verifiable offline. |
| **Right to Erasure vs. Statutory Audit** | NDPA 2023 Sec 34<br>NDPR Reg 2.1(d) | Erasure requests transition consent status to `withdrawn` and clear identifiable subject tokens. Audit events and cryptographic receipts are retained under immutable legal proof exemptions for statutory periods (5 years). |
| **Data Localization & Cross-Border Transfer** | NDPA 2023 Sec 41–43 | Dedicated tenant database storage hosted within domestic or certified adequate sovereign regions. Database connections enforce mutual TLS 1.3 (`verify-full`). |

---

### 4.3 Named Regional Privacy Review Owner

```
========================================================================================
                      REGIONAL PRIVACY REVIEW OWNER ATTESTATION
========================================================================================
Designated Regional DPO:  ______________________________________________________
Professional Title:       Regional Data Protection Officer (Africa Region)
Legal Jurisdiction:       Federal Republic of Nigeria (NG)
Supervisory Authority:    Nigeria Data Protection Commission (NDPC)

Attestation Statement:
"I have reviewed the policy governance architecture, opt-out resilience pathways,
notice versioning, and receipt verification mechanisms of the Nvuto Consent Engine.
I certify that the technical controls align with the legal standards of the Nigeria
Data Protection Act 2023 and NDPR requirements for affirmative consent and unconditional
withdrawal."

Signature: _________________________________________   Date: ___________________________
========================================================================================
```

---

## 5. Commercial & Operating Model (Cost & Resource Allocation)

### 5.1 Operating Context

The Nvuto Consent Platform Engine operates as a shared foundational utility across enterprise consumers (`Engage`, `Service`, `Contact Center`, `Customer Portal`, `CRM`, `Conversations`, `AI`, `Integration Hub`). To maintain fiscal accountability and fair resource distribution, operational costs are allocated across consumers based on consumption telemetry.

---

### 5.2 Unit Economics & Cost Allocation Schedule

| Resource Dimension | Consumption Unit | Unit Cost Baseline | Primary Consumer Cost Driver |
|---|---|---|---|
| **Evaluation Compute** | 1,000,000 Evaluations | $1.20 | Outbound Marketing Campaigns (Engage) |
| **State Mutation & Receipts** | 100,000 Consent Writes | $4.50 | Customer Portal & Onboarding Flows |
| **Outbox Event Streaming** | 1,000,000 JetStream Events | $0.40 | Real-Time Cache Invalidation Subscribers |
| **Cryptographic Storage** | 100,000 Receipts / Year | $0.85 | Regulatory Audit Ledger & YSQL Storage |
| **Dedicated Tenant Isolation** | Per Dedicated Cluster / Month | $450.00 | High-Compliance Enterprise Tenants |

---

### 5.3 Consumer Chargeback & Metering Framework

1. **Telemetry Collection**: Every HTTP request and outbox event is tagged with `tenant_id` and consumer identity (`actor_id` or `X-Nvuto-Consumer`).
2. **Monthly Reconciliation**: SRE and FinOps export monthly usage summaries from Prometheus metrics (`http_requests_total` grouped by tenant and consumer) to reconcile operational expenses.
3. **Capacity Reservation**: Consumers forecasting high-volume burst events (e.g., promotional campaigns exceeding 5,000 evaluations/sec) must submit capacity reservation notices 5 business days in advance to ensure dynamic HPA scaling.

---

## 6. Document History & Revision Control

| Revision | Date | Author / Role | Summary of Changes |
|---|---|---|---|
| **1.0.0** | 2026-10-21 | Product & Architecture Working Group | Initial formal baseline for T-032 / AT-032 product handover. Formalized capacity SLOs, NDPR regional obligations, and commercial operating model. |
