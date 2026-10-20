# Architecture Decision Record (ADR-004)

## Title: Operational Budgets, Infrastructure Ownership, and External Dependency Provisioning
- **Status:** PROPOSED / PENDING FORMAL SIGN-OFF
- **Date:** 2026-10-20
- **Decisions Addressed:** [D-08, D-09, EXT-01 through EXT-12](../../docs/production-readiness-audit/20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md)
- **Governing Body:** Nvuto Architecture Review Board & Site Reliability / Platform Operations Committee
- **Target Release:** Nvuto Consent Pilot / GA v1.0

---

## 1. Executive Summary & Purpose

This Architecture Decision Record (ADR) formalizes and captures binding owner approvals for the operational, infrastructural, and external provisioning requirements needed for production deployment of the **Nvuto Consent** service:
1. **D-08 (Capacity, Latency, Propagation, Availability, and Recovery Budgets):** Establishing numerical Service Level Objectives (SLOs), latency thresholds, maximum allowable suppression propagation lag, connection pool ceilings, and disaster recovery targets (RPO/RTO).
2. **D-09 (Infrastructure and Release Ownership):** Deciding between shared enterprise platform hosting vs. dedicated isolated infrastructure, assigning operational maintenance ownership across Kubernetes, YugabyteDB, NATS JetStream, and GitOps release pipelines.
3. **External Dependency Provisioning Matrix (EXT-01 through EXT-12):** Establishing an executive-level status and delivery registry across all 12 platform dependencies, specifying accountable engineering owners, staging readiness dates, and verification gates.

---

## 2. Decision D-08: Capacity, Latency, Propagation, Availability, and Recovery Budgets

### Context & Problem Statement
The codebase contains internal constants (e.g., 25-connection pool size per pod, 10s request timeouts, 2-minute claim leases, 30s shutdown grace), but no authoritative numerical Service Level Indicators (SLIs), Service Level Objectives (SLOs), or business continuity recovery targets have been formally authorized. Without approved budgets, capacity planning, horizontal autoscaling (HPA 3–20 pods), and production readiness gates cannot be measured or certified.

### Evaluated Options

#### Option A: Production Scale Targets Based on Measured Intended Enterprise Load
- **Scope:** Define enterprise-grade SLOs sized to support all platform consumers across multi-region production traffic. Sized for peak marketing broadcast surges (e.g., Black Friday campaigns) and low-latency synchronous pre-flight gates.
- **Pros:** Guarantees consumer SLA compliance, prevents database connection exhaustion under load, establishes clear on-call escalation thresholds.
- **Cons:** Requires rigorous performance benchmarking and load testing against multi-node YSQL and NATS clusters.

#### Option B: Explicitly Constrained Pilot Capacity (Staging / Controlled Pilot Release)
- **Scope:** Define bounded capacity targets for an initial pilot release restricted to 1–2 consumer systems (e.g., Engage marketing pilot and Customer Portal). Scale bounds are strictly capped (e.g., max 5 pods, 125 DB connections) with lower throughput requirements.
- **Pros:** Lower initial infrastructure footprint; reduces operational risk while validating real-world database access patterns.
- **Cons:** Requires a secondary benchmarking and capacity expansion exercise before onboarding the remaining platform consumers.

### Decision Selection (To be checked by Product, SRE, DBA, and Consumer Leads)
- [ ] **Option A: Production Scale Targets Based on Measured Intended Enterprise Load** *(Recommended)*
- [ ] **Option B: Explicitly Constrained Pilot Capacity (Staging / Pilot Release)**

---

### Service Level Objectives (SLO) & Operational Budget Target Matrix

*Please fill in the approved target metrics below:*

| SLI / Performance Metric | Measurement Boundary / Method | Target Metric (Pilot / Launch) | Production Target (Full GA) | Alert Warning / Page Threshold |
|---|---|---|---|---|
| **Peak Synchronous Read Rate** | Calls to `POST /v1/evaluate` | [ ______ ] req/sec | [ ______ ] req/sec | > [ ______ ] req/sec |
| **Peak Synchronous Batch Rate** | Calls to `POST /v1/evaluate:batch` | [ ______ ] req/sec | [ ______ ] req/sec | > [ ______ ] req/sec |
| **Peak Preference Write Rate** | `POST /v1/consents`, `/suppressions` | [ ______ ] req/sec | [ ______ ] req/sec | > [ ______ ] req/sec |
| **Read Latency (p95)** | Server execution time `/v1/evaluate` | < [ ______ ] ms (Default: 5ms) | < [ ______ ] ms (Default: 5ms) | > [ ______ ] ms for 5 min |
| **Read Latency (p99)** | Server execution time `/v1/evaluate` | < [ ______ ] ms (Default: 15ms) | < [ ______ ] ms (Default: 15ms) | > [ ______ ] ms for 5 min |
| **Write Latency (p95)** | Transaction commit time for grants | < [ ______ ] ms (Default: 50ms) | < [ ______ ] ms (Default: 30ms) | > [ ______ ] ms for 5 min |
| **Max Suppression Lag** | Withdrawal commit → JetStream publish | < [ ______ ] sec (Default: 2s) | < [ ______ ] sec (Default: 1s) | > [ ______ ] sec |
| **Consumer Invalidation Lag** | JetStream publish → Consumer cache clear | < [ ______ ] sec (Default: 5s) | < [ ______ ] sec (Default: 2s) | > [ ______ ] sec |
| **Monthly Service Availability** | Percentage of non-5xx probe responses | [ ______ ] % (e.g., 99.9%) | [ ______ ] % (e.g., 99.95%) | Error budget drop > 10%/hr |
| **Recovery Point Objective (RPO)** | Maximum allowable data loss in disaster | [ ______ ] (Default: 0 min) | [ ______ ] (Default: 0 min) | Backup lag > 15 min |
| **Recovery Time Objective (RTO)** | Time to restore service from disaster | < [ ______ ] min (Default: 30m) | < [ ______ ] min (Default: 15m) | Failover delay > 10 min |
| **Total YSQL Connection Budget** | Combined API + Worker active connections | Max [ ______ ] connections | Max [ ______ ] connections | > 80% pool utilization |

---

### Executive Sign-Off (D-08)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Product Owner** | ____________________ | Product Management | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Site Reliability Engineer (SRE Lead)** | ____________________ | Infrastructure & Reliability | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Database Administrator (DBA Lead)** | ____________________ | Database Architecture | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Consumer Platform Owner** | ____________________ | Enterprise Applications | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

**Conditions / Operational Notes:**  
________________________________________________________________________________________________________________________  
________________________________________________________________________________________________________________________

---

## 3. Decision D-09: Infrastructure and Release Ownership

### Context & Problem Statement
The Nvuto Consent deployment manifests (`deploy/k8s.yaml` and `deploy/outbox-worker.yaml`) declare Kubernetes workloads, PodDisruptionBudgets, and HorizontalPodAutoscalers, but external infrastructure hosting, provisioning, TLS boundaries, and deployment release pipelines must have designated operational owners.

### Evaluated Options

#### Option 1: Shared Enterprise Managed Platform Infrastructure (Recommended)
- **Architecture:** 
  - Host Consent workloads on existing enterprise Kubernetes clusters (`nvuto-prod-platform`) under the isolated `nvuto-consent` namespace with Restricted Pod Security Standards.
  - Connect to the centralized enterprise YugabyteDB YSQL universe via TLS, utilizing dedicated database roles with least-privilege table grants.
  - Utilize the enterprise NATS JetStream messaging cluster with mTLS / credentials authentication.
  - Secrets and credentials managed via enterprise HashiCorp Vault with automated rotation.
- **Pros:** Lower operational overhead, unified monitoring, reuses enterprise backup/PITR and SRE on-call rotation.
- **Cons:** Dependent on shared cluster capacity, noisier neighbor risks if noisy consumer workloads spike.

#### Option 2: Dedicated Isolated Consent Platform Infrastructure
- **Architecture:** 
  - Provision a dedicated standalone Kubernetes cluster, dedicated YugabyteDB universe, and standalone NATS cluster exclusively for the Consent domain.
- **Pros:** Complete physical and logical isolation; zero risk from noisy neighbors; bespoke scaling.
- **Cons:** Substantially higher infrastructure costs; duplicated SRE maintenance and monitoring overhead.

### Decision Selection (To be checked by Platform, Release, and DBA Leads)
- [ ] **Option 1: Shared Enterprise Managed Platform Infrastructure** *(Recommended)*
- [ ] **Option 2: Dedicated Isolated Consent Platform Infrastructure**

---

### Operational RACI Responsibility Matrix

| Infrastructure Domain | Responsible (R) | Accountable (A) | Consulted (C) | Informed (I) |
|---|---|---|---|---|
| **Kubernetes Compute & Ingress** | Platform SRE | Platform Lead | Backend Lead | Product Owner |
| **YugabyteDB Operations & Sizing** | Database Administrator | DBA Lead | SRE Lead | Backend Lead |
| **NATS JetStream Cluster & Streams** | Messaging Team | Messaging Lead | SRE Lead | Consumer Leads |
| **CI/CD Build & Promotion Pipelines** | Release Engineering | Release Owner | Security Lead | Development Team |
| **Secrets & Certificate Lifecycle** | Security Operations | InfoSec Officer | SRE Lead | Backend Lead |
| **Database Backup, PITR & Drills** | Database Operations | DBA Lead | Privacy Officer | Legal Counsel |
| **On-Call Paging & Incident Response** | Primary Consent On-Call | SRE Lead | Backend Lead | Customer Support |

---

### Executive Sign-Off (D-09)

| Role | Designee Name | Title / Organization | Decision | Signature | Date |
|---|---|---|---|---|---|
| **Platform / SRE Lead** | ____________________ | Platform Infrastructure | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Release Engineering Owner** | ____________________ | DevOps / Release Systems | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |
| **Database Administrator (DBA)** | ____________________ | Database Platform | [ ] Approved<br>[ ] Rejected<br>[ ] Conditional | ____________________ | ____________ |

**Conditions / Operational Notes:**  
________________________________________________________________________________________________________________________  
________________________________________________________________________________________________________________________

---

## 4. Master External Dependency Provisioning Matrix (EXT-01 through EXT-12)

This matrix establishes formal accountability, provisioning inputs, and target delivery dates for all twelve external dependencies required by Nvuto Consent. Each designated owner must verify that their component satisfies the stated requirements.

| ID | External Dependency | Required Provisioning Input / Integration Boundary | Designated Engineering Owner | Target Staging Readiness Date | Current Status |
|---|---|---|---|---|---|
| **EXT-01** | **Authentik / Compatible IdP** | Verified issuer URL, audience string, internal/public JWKS endpoint, custom claim mappers (`tenant_id`, `actor_id`, `customer_subject_id`, `roles`). | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |
| **EXT-02** | **Customer Graph Canonical Mapping** | Authoritative Customer Graph party identifier resolution service and tenant association rules mapped to `customer_subject_id`. | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |
| **EXT-03** | **YugabyteDB YSQL Universe** | Target universe topology (RF3), YSQL endpoint, TLS certificates (`sslmode=verify-full`), least-privilege DB role, migrations 001–006 executed. | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |
| **EXT-04** | **NATS JetStream Cluster** | NATS broker URL, JetStream stream `NVUTO_CONSENT` provisioned with 3 replicas, deduplication window (2m), user `.creds` file provisioned. | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |
| **EXT-05** | **Trusted DOI Delivery Component** | Authorized communications adapter to consume DOI challenge events and dispatch out-of-band email/SMS confirmation links without token leakage. | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |
| **EXT-06** | **Container Registry & Artifacts** | GHCR repository access, automated container builds, immutable `@sha256:` digest generation, SBOM/provenance attestation, vulnerability scanner gates. | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |
| **EXT-07** | **CI Execution & Validation Gates** | GitHub Actions runners configured with isolated YSQL RF1 node and NATS test broker, required branch protection rules, automated test execution. | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |
| **EXT-08** | **Kubernetes Platform & Gateway** | Namespace `nvuto-consent` labeled with `pod-security.kubernetes.io/enforce: restricted`, Ingress/Gateway routing, TLS certificate manager, HPA/PDB active. | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |
| **EXT-09** | **Observability & Alerting Stack** | Centralized log sink for structured JSON `slog` events, metrics collection (Prometheus), latency/error rate dashboards (Grafana), PagerDuty alerting rules. | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |
| **EXT-10** | **Secrets & Certificate Lifecycle** | Production Secret distribution (`consent-secrets`, `consent-runtime`, `consent-nats-credentials`), TLS root CA distribution, Vault rotation policies. | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |
| **EXT-11** | **Backup, PITR, & Recovery** | Automated snapshot policy for YSQL universe, Point-In-Time Recovery (PITR) verified in isolated recovery namespace, outbox reconciliation runbook. | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |
| **EXT-12** | **Launch Consumer Applications** | Verified integration test suites for approved launch consumers (e.g., Engage marketing campaigns, Customer Portal self-service) validating real API contracts. | ____________________ | ____________ | [ ] Ready<br>[ ] In Progress<br>[ ] Blocked |

---

## 5. Consolidated Master Approval Summary

This section consolidates final approvals across Decisions D-08, D-09, and the External Dependency Provisioning Matrix (EXT-01 to EXT-12). Once all signatures are recorded, the status of this document advances from `PROPOSED` to `APPROVED`.

| Section | Focus Area | Recommended Resolution | Primary Approvers | Status |
|---|---|---|---|---|
| **D-08** | Operational Budgets & SLOs | Option A: Full Enterprise Production Budgets | Product, SRE, DBA, Consumer Leads | [ ] Pending [ ] Approved |
| **D-09** | Infrastructure & Ownership | Option 1: Shared Enterprise Managed Platform | Platform SRE, Release Owner, DBA | [ ] Pending [ ] Approved |
| **EXT-01..12** | External Dependency Matrix | Full 12-Component Provisioning Plan | Respective Designated Component Owners | [ ] Pending [ ] Approved |

### Executive Committee Acceptance

| Executive Sponsor | Title | Signature | Date |
|---|---|---|---|
| ________________________________________ | VP of Infrastructure & Operations | ________________________________________ | ____________________ |
| ________________________________________ | VP of Engineering | ________________________________________ | ____________________ |
| ________________________________________ | Head of Product | ________________________________________ | ____________________ |
| ________________________________________ | General Counsel / Chief Privacy Officer | ________________________________________ | ____________________ |
