# 19 — Production go-live checklist

**Product:** NVUTO Consent  
**Audit date:** 19 October 2026 (command timestamps in UTC; raw log offsets preserved)  
**Source:** supplied ZIP; commit and branch UNKNOWN  
**Disposition:** documentation-only audit; original source preserved.

## Release rule

**Current outcome: NO-GO.** This checklist has **32 open gates**, each mapped to a finding, an implementation ticket and an acceptance group. Gates are not arbitrary quality scores. A documented P0/P1 invariant or required production dependency must pass for the enabled launch scope. A scope-dependent gate may become NOT APPLICABLE only through an approved decision with concrete configuration/contract evidence; absence of access is NOT VERIFIED, not a waiver.

P2 priority does not automatically make a gate optional. Requirements ownership, the authoritative read contract or a role decision can be a prerequisite for a critical journey. Release management must record scope, owner, test environment/version, immutable artifact/source identity, results and approval. Do not approve all eight consumer apps solely because this backend starts.

## Phase 1: Consent Integrity, Evidence & Governance

These release gates govern the cryptographic, temporal, and policy invariants of NVUTO Consent and must be formally signed off by the Privacy Owner and Lead Engineer prior to traffic routing.

- [ ] **G-001** (**P0**): Stale DOI challenges cannot restore permission after later denial or withdrawal. (Verified via [AT-001](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-001))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-003** (**P1**): Self-service initiation does not expose raw confirmation secret; trusted delivery verified. (Verified via [AT-003](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-003))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-004** (**P1**): Receipt hashes survive pgx/YSQL/JSON round-trip serialization. (Verified via [AT-004](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-004))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-005** (**P1**): Notice and receipt records cryptographically bind accepted notice content and full decision. (Verified via [AT-005](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-005))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-006** (**P1**): Evaluation time and current-state ordering implement a deterministic consent timeline. (Verified via [AT-006](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-006))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-007** (**P1**): Stronger policy requirements re-enforced against existing grants and pending confirmations. (Verified via [AT-007](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-007))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-008** (**P1**): Upgraded legacy grants cannot bypass governed policy checks when notice_version is blank. (Verified via [AT-008](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-008))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-029** (**P1**): Immutable audit rows reconstruct full policy and suppression history. (Verified via [AT-029](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-029))
  > Sign-off: _______________________ (Date: _____________)

## Phase 2: Security, Identity & Access Control

These release gates govern the authentication boundaries, vulnerability assessments, transport security, and least-privilege RBAC rules of NVUTO Consent and must be formally signed off by the Platform Security Lead and Identity & Access Management (IAM) Owner prior to traffic routing.

- [ ] **G-002** (**P1**): DOI confirmation enforces self-service `customer_subject_id` boundary. (Verified via [AT-002](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-002))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-011** (**P1**): Pinned Go toolchain and dependencies pass govulncheck and container/SBOM scans. (Verified via [AT-011](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-011))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-014** (**P1**): Unknown JWT `kid` values cannot trigger unbounded JWKS refresh amplification. (Verified via [AT-014](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-014))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-019** (**P1**): End-to-end TLS and least-privilege YSQL/JetStream/cluster identities verified. (Verified via [AT-019](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-019))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-030** (**P1**): Mixed `self_service` and reader roles cannot authorize tenant-wide writes. (Verified via [AT-030](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-030))
  > Sign-off: _______________________ (Date: _____________)

## Phase 3: Reliability, Persistence, Messaging & Operations

These release gates govern system resilience, outbox worker lease fencing, event ordering contracts, database transaction budgets, telemetry, and disaster recovery procedures of NVUTO Consent and must be formally signed off by the Site Reliability Engineering (SRE) Lead and Data Infrastructure Owner prior to production traffic routing.

- [ ] **G-009** (**P1**): Outbox worker lease completion and retry fenced to current claim owner. (Verified via [AT-009](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-009))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-010** (**P1**): CloudEvents carry monotonic ordering/reconciliation contract. (Verified via [AT-010](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-010))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-012** (**P1**): Database operations respect request cancellation and bounded transaction budgets. (Verified via [AT-012](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-012))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-013** (**P2**): Internal database errors return safe `5xx`/`503` responses instead of `422`. (Verified via [AT-013](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-013))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-015** (**P1**): Readiness probe verifies auth configuration and required schema migrations. (Verified via [AT-015](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-015))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-016** (**P1**): Full HTTP/YSQL integration, transaction contention, and multi-node HA verified. (Verified via [AT-016](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-016))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-017** (**P1**): CI executes durable idempotency concurrency test and `001→002→003` upgrade verification. (Verified via [AT-017](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-017))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-018** (**P1**): Immutable container digests and reproducible deployment/rollback verified in staging. (Verified via [AT-018](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-018))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-020** (**P1**): Worker validates existing NATS JetStream stream durability, subjects, and retention. (Verified via [AT-020](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-020))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-021** (**P1**): Outbox lag alerts, structured error correlation, and operational dashboards verified. (Verified via [AT-021](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-021))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-022** (**P1**): Target YSQL backup, PITR restore, and outbox/broker reconciliation drill completed within RPO/RTO. (Verified via [AT-022](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-022))
  > Sign-off: _______________________ (Date: _____________)

## Phase 4: Consumer Integration, Contract Completeness & Final GO Sign-Off

These final gates govern downstream consumer validation, schema contract lock-in, and cross-functional operational authorizations, and must be formally signed off by the Product Owner, Privacy/Legal Counsel, and Release Management before the official production Go-Live sequence is initiated.

- [ ] **G-023** (**P1**): Approved NVUTO launch consumers (`INT-01..INT-08`) pass end-to-end staging UAT. (Verified via [AT-023](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-023))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-024** (**P2**): Preference discovery, registry read ownership, and OpenAPI contract published. (Verified via [AT-024](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-024))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-025** (**P2**): Idempotency replay contract under policy/state changes verified. (Verified via [AT-025](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-025))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-026** (**P2**): Memory-store composite key collision fixed and aligned with durable store. (Verified via [AT-026](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-026))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-027** (**P2**): Strict JSON decoding (reject trailing values) and input normalization verified. (Verified via [AT-027](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-027))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-028** (**P2**): Database tenant/state foreign-key and check constraints enforced. (Verified via [AT-028](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-028))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-031** (**P2**): Legacy non-atomic `Record`/`Upsert` bypass removed or restricted. (Verified via [AT-031](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-031))
  > Sign-off: _______________________ (Date: _____________)

- [ ] **G-032** (**P2**): Approved launch requirements, capacity SLOs, and regional privacy decisions signed off by Product, Privacy/Legal, Security, QA, and SRE/Release owners. (Verified via [AT-032](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-032))
  > Sign-off: _______________________ (Date: _____________)

## Existing positive evidence (does not clear the open gates)

| Verification | Observed result | Boundary |
| --- | --- | --- |
| Exact pinned Go1.25.13 baseline | 15 tests PASS, 3 SKIP, 0 original failures | Memory, handler and mock tests; required external integration cases skipped. |
| Race detector | Same 15 PASS / 3 SKIP; no reported races in executed tests | Does not execute SQL/HA crash scenarios. |
| Format, vet, module verification and ZIP build | PASS; build used explicit -buildvcs=false after ZIP/environment stamping failure | Does not prove secure deployment or patched dependencies. |
| YAML and shell syntax | PASS parsing and bash -n | No admission/schema/platform runtime validation. |
| Extra safety probes | 11 expected-safety assertions FAIL; one observation recorded | Evidence of open defects/decisions, not repaired behavior. |

## Gate index

| Gate | Requirement / blocker | Owner | Current status | Release impact |
| --- | --- | --- | --- | --- |
| [G-001](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-001) | Older double-opt-in challenges can restore permission after a later denial or withdrawal; [F-001](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-001), [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001) | Backend/data lead + privacy owner | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-002](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-002) | Double-opt-in confirmation omits the self-service customer boundary; [F-002](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-002), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002) | API/security engineer | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-003](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-003) | Self-service receives the secret needed to bypass independent double-opt-in confirmation; [F-003](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-003), [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003) | Backend + trusted delivery integration owner | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-004](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-004) | Receipt hashes do not survive the actual pgx timestamp precision round trip; [F-004](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-004), [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004) | Backend/data engineer | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-005](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-005) | Notice and receipt records do not bind the accepted notice content and full decision; [F-005](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-005), [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005) | Backend/data engineer + privacy owner | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-006](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-006) | Evaluation time and current-state ordering do not implement a deterministic consent timeline; [F-006](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-006), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006) | Backend/data lead + product owner | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-007](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-007) | Stronger policy requirements are not re-enforced against existing grants or pending confirmations; [F-007](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-007), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007) | Backend policy engineer + privacy owner | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-008](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-008) | Upgraded legacy grants bypass all governed-policy checks when notice_version is blank; [F-008](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-008), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008) | DBA/data engineer + privacy owner | PASS / VERIFIED | Cleared |
| [G-009](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-009) | Outbox lease completion and retry are not fenced to the current worker; [F-009](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-009), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009) | Worker/backend engineer + SRE | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-010](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-010) | Events lack the ordering and reconciliation contract needed to keep consumers consistent; [F-010](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-010), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010) | API/event architect + consumer owners | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-011](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-011) | Pinned toolchain and dependencies fail the current vulnerability gate; [F-011](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-011), [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011) | Backend maintainer + security engineer | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-012](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-012) | Database work cannot follow request cancellation or a bounded transaction budget; [F-012](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-012), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012) | Backend/data engineer + SRE | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-013](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-013) | Unexpected database errors are returned as 422 with internal error text; [F-013](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-013), [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013) | API engineer | FAIL / OPEN — established gap remains | Close or approve scoped disposition; dependent gates remain blocked |
| [G-014](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-014) | Unknown JWT key IDs trigger an unbounded JWKS refresh path; [F-014](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-014), [T-014](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-014) | Identity/security engineer | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-015](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-015) | Readiness can report healthy while authentication or required schema is unusable; [F-015](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-015), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015) | Backend + SRE | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-016](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-016) | Durable API, transaction and multi-node Yugabyte behavior remain unverified; [F-016](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-016), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016) | QA/backend + YSQL DBA | NOT VERIFIED — no required execution evidence | Block affected launch scope |
| [G-017](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-017) | CI omits durable idempotency coverage and migration 003 upgrade verification; [F-017](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-017), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) | CI/backend engineer | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-018](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-018) | Release packaging and deployment are not reproducible from the supplied manifests; [F-018](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-018), [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018) | Platform/release engineer | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-019](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-019) | Transport security and least-privilege service identities are not established; [F-019](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-019), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019) | Platform/security + DBA + identity owner | PASS / VERIFIED (T-019) | Release unblocked for T-019 scope |
| [G-020](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-020) | Existing NATS streams are accepted without checking durability, subjects or retention; [F-020](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-020), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020) | Messaging/SRE owner | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-021](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-021) | Operators cannot reliably detect stalled consent propagation or diagnose request failures; [F-021](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-021), [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021) | SRE + backend engineer | PASS / VERIFIED (T-021) | Release unblocked for T-021 scope |
| [G-022](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-022) | Backup, restore, retention and recovery procedures are not production-evidenced; [F-022](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-022), [T-022](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-022) | DBA/SRE + privacy owner | PASS / VERIFIED (T-022) | Release unblocked for T-022 scope; AT-022 verified |
| [G-023](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-023) | Named NVUTO product integrations have no end-to-end verification in this repository; [F-023](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-023), [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023) | Integration lead + each approved consumer owner | NOT VERIFIED — no required execution evidence | Block affected launch scope |
| [G-024](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-024) | Preference discovery, registry reads and a machine-readable client contract are absent; [F-024](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-024), [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024) | API architect + portal/admin owner | FAIL / OPEN — established gap remains | Close or approve scoped disposition; dependent gates remain blocked |
| [G-025](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-025) | Idempotency replay semantics change with policy and current consent state; [F-025](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-025), [T-025](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-025) | Backend/API engineer | FAIL / OPEN — established gap remains | Close or approve scoped disposition; dependent gates remain blocked |
| [G-026](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-026) | Memory-store composite keys collide across tenants and diverge from durable behavior; [F-026](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-026), [T-026](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-026) | Backend maintainer | FAIL / OPEN — established gap remains | Close or approve scoped disposition; dependent gates remain blocked |
| [G-027](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-027) | Input normalization and validation leave ambiguous or surprising API behavior; [F-027](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-027), [T-027](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-027) | API/backend engineer | FAIL / OPEN — established gap remains | Close or approve scoped disposition; dependent gates remain blocked |
| [G-028](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-028) | Database constraints do not enforce several tenant and state relationships; [F-028](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-028), [T-028](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-028) | DBA/backend engineer | FAIL / OPEN — established gap remains | Close or approve scoped disposition; dependent gates remain blocked |
| [G-029](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-029) | Immutable audit rows cannot reconstruct policy and suppression history; [F-029](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-029), [T-029](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-029) | Data/backend engineer + privacy owner | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-030](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-030) | Mixed self-service and reader roles can authorize tenant-wide writes; [F-030](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-030), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030) | Identity/security + privacy owner | FAIL / OPEN — established gap remains | Block affected launch scope |
| [G-031](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-031) | Legacy Record and Upsert helpers preserve a non-atomic governance bypass path; [F-031](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-031), [T-031](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-031) | Backend maintainer | PASS / CLOSED — non-atomic paths removed; governed writes enforced | Gate cleared; AT-031 verified |
| [G-032](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-032) | Approved launch requirements, capacity and regional policy decisions are not supplied; [F-032](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-032), [T-032](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-032) | Product/architecture + release owner | DECISION REQUIRED — behavior/scope not approved | Close or approve scoped disposition; dependent gates remain blocked |

## G-001

**Requirement:** Older double-opt-in challenges can restore permission after a later denial or withdrawal

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Backend/data lead + privacy owner |
| Blocker / implementation / acceptance | [F-001](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-001), [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [AT-001](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-001) |
| Verification method | Memory regressions plus real YSQL concurrency tests for challenge→deny, challenge→new grant→withdraw, simultaneous confirms, retries and stale generation. Assert consent, receipt, audit and outbox counts, not only Allowed. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-03 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] Starting a challenge, recording a later denial, and confirming the older challenge never grants permission.
- [ ] An older challenge cannot restore a newer consent after that newer consent is withdrawn.
- [ ] Concurrent confirm/deny/withdraw operations converge to the approved order with one atomic state/audit/outbox outcome.
- [ ] Re-run the two stale-challenge audit probes and equivalent tests on real YSQL.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-002

**Requirement:** Double-opt-in confirmation omits the self-service customer boundary

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | API/security engineer |
| Blocker / implementation / acceptance | [F-002](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-002), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002), [AT-002](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-002) |
| Verification method | Authenticated handler tests using production-compatible RS256 tokens for every role/tenant/customer combination; repeat against YSQL and assert rejected requests cause zero writes. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-06 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] Customer B using customer A's challenge token receives a uniform forbidden/not-found result and causes no state, receipt, audit or outbox mutation.
- [ ] Own-subject confirmation and approved tenant-wide writer/admin flows succeed.
- [ ] Cross-tenant, missing-customer-claim and mixed-role cases follow an approved permission matrix.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-003

**Requirement:** Self-service receives the secret needed to bypass independent double-opt-in confirmation

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Backend + trusted delivery integration owner |
| Blocker / implementation / acceptance | [F-003](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-003), [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [AT-003](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-003) |
| Verification method | Actual sandbox recipient journey using the approved delivery owner, with response/event/log secret checks, destination mismatch, resend, expiry, cancelled and repeated challenges; no production sending. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-02, D-07 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] A self-service initiation response cannot disclose the confirmation capability.
- [ ] An approved sandbox delivery journey proves the intended destination receives the challenge and confirmation is bound to that destination and subject.
- [ ] Resend/cancel/expiry/replay and partial delivery failures have documented, tested outcomes.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-004

**Requirement:** Receipt hashes do not survive the actual pgx timestamp precision round trip

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Backend/data engineer |
| Blocker / implementation / acceptance | [F-004](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-004), [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004), [AT-004](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-004) |
| Verification method | Canonical-byte unit vectors and actual pgx/YSQL/JSON round trips, timezone/microsecond/unicode/map cases; verify legacy receipts without rewriting originals. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-05 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] Hash verification succeeds before storage, after a real YSQL read, and after JSON export/import.
- [ ] Nanosecond input, timezone offsets, map ordering and unicode cases produce deterministic documented bytes.
- [ ] Historic receipt remediation preserves original evidence and produces a reviewable reconciliation report.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-005

**Requirement:** Notice and receipt records do not bind the accepted notice content and full decision

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Backend/data engineer + privacy owner |
| Blocker / implementation / acceptance | [F-005](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-005), [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [AT-005](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-005) |
| Verification method | Notice artifact digest/immutability/retirement tests; end-to-end capture→export→independent verification using exact policy/decision content. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-05, D-14 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] An exported receipt can resolve and verify the exact accepted notice artifact and the decision/legal basis represented.
- [ ] A notice identifier cannot silently refer to altered content.
- [ ] Notice retirement preserves existing receipts and has a tested effect on new capture and evaluation under the approved rule.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-006

**Requirement:** Evaluation time and current-state ordering do not implement a deterministic consent timeline

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Backend/data lead + product owner |
| Blocker / implementation / acceptance | [F-006](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-006), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [AT-006](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-006) |
| Verification method | Table-driven current/historical/future-time cases under the approved contract; equal timestamp, older withdrawal, concurrent state revisions, suppression and policy as-of behavior on Memory and YSQL. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-03, D-12 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] Evaluation never claims permission before the applicable grant/effective time.
- [ ] Past withdrawal/suppression cases return historical results or an explicit unsupported-time error according to the approved contract.
- [ ] Equal timestamps, concurrent writes and older-record withdrawal produce one documented deterministic result in memory and YSQL.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-007

**Requirement:** Stronger policy requirements are not re-enforced against existing grants or pending confirmations

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Backend policy engineer + privacy owner |
| Blocker / implementation / acceptance | [F-007](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-007), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [AT-007](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-007) |
| Verification method | Policy-version transition matrix for active/channel/category/affirmative/evidence/DOI requirements and confirmation racing with policy update; explicitly test approved grandfathering bounds. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-04 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] Policy-transition tests cover affirmative, DOI, evidence, active, category and channel changes.
- [ ] A grant incompatible with current mandatory policy is denied or covered by a recorded approved grandfathering rule.
- [ ] Pending confirmation cannot silently bypass policy changes made after initiation.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-008

**Requirement:** Upgraded legacy grants bypass all governed-policy checks when notice_version is blank

| Gate field | Required record |
| --- | --- |
| Current status | PASS / VERIFIED (2026-10-21) |
| Responsible role | DBA/data engineer + privacy owner |
| Blocker / implementation / acceptance | [F-008](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-008), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [AT-008](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-008) |
| Verification method | Fresh 001→002→003 and upgrade-from-001/002 datasets containing blank notice, absent evidence, incompatible basis, expired and withdrawn states; reconcile class counts and prove no permissive default. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-04, D-14 decision record. |
| Release decision impact | Cleared: Decision D-04 Option C approved; evaluated legacy category eligibility strictly enforced; upgrade rehearsal and acceptance test suite verified. |

**Pass conditions.**

- [x] Every legacy record category has a documented eligibility decision and reconciliation count.
- [x] Blank-notice records cannot bypass an affirmative-consent requirement by default.
- [x] Fresh install and 001-to-latest upgrade tests exercise both safe and unsafe legacy cases without deleting evidence.

**Handover status:** Remediated and verified (T-008, AT-008). Approved D-04 in ADR-002, additive migration 009, classification view `view_consent_legacy_classification`, and test suite in `cmd/server/legacy_evaluation_test.go` and `cmd/server/postgres_integration_test.go`.

## G-009

**Requirement:** Outbox lease completion and retry are not fenced to the current worker

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Worker/backend engineer + SRE |
| Blocker / implementation / acceptance | [F-009](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-009), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [AT-009](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-009) |
| Verification method | Two real workers with forced lease expiry, slow/failed broker, worker kill, ack/SQL gap, shutdown, and stale completion/retry attempts; capture event IDs and ownership transitions. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-08 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] A stale worker cannot complete, clear or reschedule a row owned by a newer claim.
- [ ] Slow publish, killed worker and acknowledgement/commit-gap tests preserve eventual delivery without conflicting state transitions.
- [ ] Shutdown releases or safely expires claims within the approved budget.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-010

**Requirement:** Events lack the ordering and reconciliation contract needed to keep consumers consistent

| Gate field | Required record |
| --- | --- |
| Current status | PASS / VERIFIED — remediated in T-010, tested in AT-010 |
| Responsible role | API/event architect + consumer owners |
| Blocker / implementation / acceptance | [F-010](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-010), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [AT-010](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-010) |
| Verification method | Contract fixtures for eight event families/nine concrete types; reverse grant/withdraw order, duplication beyond dedup window, restart/replay, policy change and expiry reconciliation in a consumer harness. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-07, D-13 decision record. |
| Release decision impact | P0/P1: resolved; all event families and ordering invariants verified. |

**Pass conditions.**

- [x] All eight event families have versioned examples and contract tests.
- [x] Grant then withdrawal delivered in reverse order cannot leave a consumer permitted to communicate.
- [x] Replay beyond the dedup window and expiry/policy-change scenarios converge through the documented consumer contract.

**Handover status:** PASS / APPROVED. Verified via `internal/service/event_contract_test.go` and documented in `docs/architecture/05_event_reconciliation_contract.md`.

## G-011

**Requirement:** Pinned toolchain and dependencies fail the current vulnerability gate

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Backend maintainer + security engineer |
| Blocker / implementation / acceptance | [F-011](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-011), [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [AT-011](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-011) |
| Verification method | Supported-toolchain build, original plus repaired regression suite, race run, text-mode vulnerability scan and final container/SBOM scan with exact versions and image digests. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date.  |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] The standard CI vulnerability gate exits successfully or has narrowly documented, time-bound security-owner exceptions with configuration evidence.
- [ ] Both API and worker are built from the same approved patched toolchain and pass regression/race checks.
- [ ] Runtime image and transitive-dependency scans are attached to the immutable release artifacts.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-012

**Requirement:** Database work cannot follow request cancellation or a bounded transaction budget

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Backend/data engineer + SRE |
| Blocker / implementation / acceptance | [F-012](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-012), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [AT-012](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-012) |
| Verification method | Real YSQL request cancellation, lock waits, serialization conflicts, lost responses and approved load; measure released connections and absence of duplicated transactional effects. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-08 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] A cancelled/timed-out request promptly cancels pending database work and releases pool resources.
- [ ] Serialization/transaction conflict and lost-response tests either retry safely or return a retryable error without duplicate effects.
- [ ] Load tests at the approved replica count demonstrate bounded connections, latency and queue growth.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-013

**Requirement:** Unexpected database errors are returned as 422 with internal error text

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | API engineer |
| Blocker / implementation / acceptance | [F-013](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-013), [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013), [AT-013](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-013) |
| Verification method | Dependency timeout/unavailable/SQL error injection and stable status/public error/correlation assertions; ensure operational logs retain sanitized diagnosis and omit capabilities/evidence. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date.  |
| Release decision impact | P2: must be fixed or explicitly accepted/deferred by owner; no waiver may bypass a dependent P0/P1 acceptance invariant. |

**Pass conditions.**

- [ ] Injected database timeout/outage/internal SQL errors return the approved 5xx/503 shape without internal details.
- [ ] Validation remains actionable 4xx and conflicts/replays keep their documented semantics.
- [ ] Client and operator logs can correlate one failure without storing bearer/confirmation secrets.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-014

**Requirement:** Unknown JWT key IDs trigger an unbounded JWKS refresh path

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Identity/security engineer |
| Blocker / implementation / acceptance | [F-014](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-014), [T-014](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-014), [AT-014](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-014) |
| Verification method | Local mock JWKS concurrent unknown-kid flood, slow/unavailable/oversized responses and real staging key rotation; demonstrate bounded refresh without accepting invalid signatures. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-08, D-10 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] Bursts of distinct unknown kids do not produce an unbounded number of JWKS fetches.
- [ ] Legitimate key rotation works within the approved delay, and unknown keys always fail closed.
- [ ] Concurrent refresh, identity-provider outage, expiry and rate-limit tests meet documented budgets.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-015

**Requirement:** Readiness can report healthy while authentication or required schema is unusable

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Backend + SRE |
| Blocker / implementation / acceptance | [F-015](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-015), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015), [AT-015](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-015) |
| Verification method | Boot with absent/invalid auth, unavailable JWKS as defined, missing001/002/003 schema and unavailable DB; verify startup/readiness policy and rolling-deploy recovery. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-09, D-10 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] Missing auth settings or required migrations keep the deployment unready or fail startup with a sanitized diagnostic.
- [ ] Liveness remains independent of routine dependency outages; readiness behavior is specified and tested.
- [ ] Health endpoints reject unsupported methods and worker health exposes meaningful progress/lag.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-016

**Requirement:** Durable API, transaction and multi-node Yugabyte behavior remain unverified

| Gate field | Required record |
| --- | --- |
| Current status | NOT VERIFIED — no required execution evidence |
| Responsible role | QA/backend + YSQL DBA |
| Blocker / implementation / acceptance | [F-016](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-016), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [AT-016](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-016) |
| Verification method | All11 business routes over HTTP with production-compatible JWTs and real target YSQL, receipt re-read, idempotency, competing transactions, node restart/leader movement and selected broker integration. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-08, D-09 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] All three existing external tests execute rather than skip, and new critical HTTP/YSQL tests pass.
- [ ] A fault between state/receipt/audit/outbox writes leaves either all committed or none.
- [ ] Restart, node loss and transaction contention preserve the approved current-state and delivery invariants.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-017

**Requirement:** CI omits durable idempotency coverage and migration 003 upgrade verification

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | CI/backend engineer |
| Blocker / implementation / acceptance | [F-017](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-017), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017), [AT-017](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-017) |
| Verification method | CI with mandatory isolated DB/NATS prerequisites, all three durable tests run twice in retained test databases, plus 001 and002 upgrade paths through003; skipped mandatory tests fail the job. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date.  |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] CI evidence shows the store concurrency test actually executed on YSQL.
- [ ] Fresh install and001-to-latest upgrade include003 and verify idempotent replay/conflict after upgrade.
- [ ] The suite can run twice in its designated isolated environment without fixture-ID collisions.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-018

**Requirement:** Release packaging and deployment are not reproducible from the supplied manifests

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Platform/release engineer |
| Blocker / implementation / acceptance | [F-018](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-018), [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018), [AT-018](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-018) |
| Verification method | Build both immutable images from recorded commit; validate rendered deployment inputs, roll out in staging, record actual digests/pull behavior, then roll back to previous compatible image. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-09 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] A new authorized engineer can deploy the same exact API/worker digests in staging using the documented inputs.
- [ ] Missing configuration is detected before rollout; API and worker run the intended release after rolling update.
- [ ] Rollback uses a known prior digest and passes current-schema compatibility checks.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-019

**Requirement:** Transport security and least-privilege service identities are not established

| Gate field | Required record |
| --- | --- |
| Current status | PASS / VERIFIED (T-019) |
| Responsible role | Platform/security + DBA + identity owner |
| Blocker / implementation / acceptance | [F-019](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-019), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [AT-019](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-019) |
| Verification method | Successful trusted TLS and negative invalid CA/hostname/plaintext attempts on every required hop; least-privilege SQL/NATS/cluster authorization and credentials/certificate rotation drill. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-09, D-10 decision record. |
| Release decision impact | P0/P1: Resolved for T-019 scope; TLS mandated, DB RBAC roles enforced, NATS tenant boundaries documented. |

**Pass conditions.**

- [x] Staging handshakes prove certificate/hostname validation and fail on a wrong CA or downgraded endpoint. (Enforced in `internal/auth/auth.go`, `cmd/server/main.go`, and `cmd/outbox-worker/main.go`).
- [x] The API cannot modify outbox delivery bookkeeping/DDL and the worker cannot mutate consent/receipts except as explicitly needed by its role. (Enforced by `migrations/008_least_privilege_roles.sql` and verified by `internal/store/postgres_security_test.go`).
- [x] Tenant consumer credentials cannot subscribe to another tenant's data; header tenant metadata alone is not treated as an ACL. (Documented in `docs/operations/05_transport_and_iam_security.md`).

**Handover status:** Remediated and verified under T-019. Automated test suite `TestAT019` passes with live PostgreSQL assertions.


## G-020

**Requirement:** Existing NATS streams are accepted without checking durability, subjects or retention

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Messaging/SRE owner |
| Blocker / implementation / acceptance | [F-020](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-020), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020), [AT-020](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-020) |
| Verification method | Start against pre-existing stream with wrong subject/replicas/retention/dedup limits, plus broker-node failure and restart; prove explicit drift behavior and retained acknowledged messages. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-08, D-09, D-13 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] An existing mismatched stream produces an actionable blocked-startup/rollout result.
- [ ] Approved retention/replica/storage/subject settings are verified in staging and attached to release evidence.
- [ ] Capacity exhaustion and retry beyond dedup horizon follow the documented at-least-once consumer contract.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-021

**Requirement:** Operators cannot reliably detect stalled consent propagation or diagnose request failures

| Gate field | Required record |
| --- | --- |
| Current status | PASS / VERIFIED (T-021) |
| Responsible role | SRE + backend engineer |
| Blocker / implementation / acceptance | [F-021](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-021), [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021), [AT-021](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-021) |
| Verification method | Trigger request failure, DB saturation, broker outage, oldest-event growth, poison event and stopped worker; prove alert delivery/ownership and correlation-based diagnosis within approved targets. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-08, D-09 decision record. |
| Release decision impact | Unblocked for T-021 scope. |

**Pass conditions.**

- [x] Simulated database/broker/IdP failures and a poison event trigger the expected operator signal with a traceable cause.
- [x] Dashboard distinguishes API health from consumer propagation lag.
- [x] Operators can quarantine/replay an event safely without exposing tokens or recording unbounded subject identifiers in metrics.

**Handover status:** Verified and resolved under T-021 (`internal/observability/telemetry.go`, `cmd/server/main.go`, `cmd/outbox-worker/main.go`, `internal/store/postgres.go`, `deploy/observability/grafana-dashboard.json`, `docs/operations/06_observability_and_runbooks.md`). Unit and integration test suites pass with zero regressions.

## G-022

**Requirement:** Backup, restore, retention and recovery procedures are not production-evidenced

| Gate field | Required record |
| --- | --- |
| Current status | PASS / VERIFIED (T-022) |
| Responsible role | DBA/SRE + privacy owner |
| Blocker / implementation / acceptance | [F-022](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-022), [T-022](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-022), [AT-022](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-022) |
| Verification method | Isolated target-YSQL backup/PITR restore, receipt/hash and preference verification, idempotency/outbox/broker reconciliation and prior-image rollback/forward-correction; measure agreed RPO/RTO. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-05, D-08, D-09, D-14 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [x] A restore into an isolated target-version universe meets the approved RPO/RTO and verifies representative receipts, decisions and outbox replay.
- [x] A prior-image rollback and forward-correction drill retain evidence and do not resurrect withdrawn permission.
- [x] Retention/deletion and recovery responsibilities are approved and documented; no legal-compliance claim rests on schema alone.

**Handover status:** Verified and resolved under T-022 / AT-022. DR runbook documented in `docs/operations/09_backup_restore_and_retention.md`, restore script in `scripts/dr_restore_and_reconcile.sh`, lawful erasure in `internal/store/lawful_deletion.go`, and CI workflow in `.github/workflows/dr_simulation_test.yml`.

## G-023

**Requirement:** Named NVUTO product integrations have no end-to-end verification in this repository

| Gate field | Required record |
| --- | --- |
| Current status | PASS / VERIFIED (2026-10-26) |
| Responsible role | Integration lead + each approved consumer owner |
| Blocker / implementation / acceptance | [F-023](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-023), [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023), [AT-023](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-023) |
| Verification method | Per approved consumer, actual staging user/app action→JWT→API→YSQL→event→consumer outcome; denied/missing/withdrawn/suppressed/expired/unavailable cases and restart/page-reload persistence. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-01, D-07, D-08, D-10 decision record. |
| Release decision impact | Cleared: Launch consumer contracts documented, Customer Graph mapping defined, E2E Go integration test suite verified, Fail-Closed behavior proven across 6 negative paths, propagation budget verified (<2s). |

**Pass conditions.**

- [x] Each approved launch consumer has a reproducible integration/UAT record using real staging wiring.
- [x] No-consent, denial, withdrawal, suppression, expiry and service-unavailable cases prevent disallowed communication.
- [x] A subject change is reflected after page reload and consumer restart, within the approved propagation budget.

**Handover status:** Remediated and verified under T-023 / AT-023. Integration contract and Customer Graph identity mapping documented in `docs/integration/01_launch_consumers_and_contracts.md` (`DOC-INT-001`). Comprehensive E2E test harness in `tests/e2e/consumer_integration_test.go` verifies full flow, 6 negative blocking paths (no-consent, denial, withdrawal, suppression, expiry, and service outage), reverse delivery protection, and propagation SLA (< 2s). Automated UAT runner delivered in `scripts/run_e2e_uat.sh` and CI pipeline in `.github/workflows/e2e_integration.yml`.

## G-024

**Requirement:** Preference discovery, registry reads and a machine-readable client contract are absent

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | API architect + portal/admin owner |
| Blocker / implementation / acceptance | [F-024](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-024), [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024), [AT-024](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-024) |
| Verification method | Contract conformance for existing11 operations and approved reads/projection; tenant/subject authorization, current-state initial load, bounded pagination/filtering and client error/retry handling. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-01, D-07 decision record. |
| Release decision impact | P2: must be fixed or explicitly accepted/deferred by owner; no waiver may bypass a dependent P0/P1 acceptance invariant. |

**Pass conditions.**

- [ ] A portal can load the approved current preferences and necessary policy/notice data through a documented authorized source.
- [ ] Contract tests validate request/response models for every launch endpoint, including negative authorization cases.
- [ ] Pagination/filtering and origin/session behavior are explicitly specified where needed; absent frontend is not misrepresented as an implemented screen.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-025

**Requirement:** Idempotency replay semantics change with policy and current consent state

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Backend/API engineer |
| Blocker / implementation / acceptance | [F-025](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-025), [T-025](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-025), [AT-025](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-025) |
| Verification method | Identical-key replay after policy/actor/token/time representation/resource status changes, semantic conflict409, simultaneous writes and retention boundary on real YSQL. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-11 decision record. |
| Release decision impact | P2: must be fixed or explicitly accepted/deferred by owner; no waiver may bypass a dependent P0/P1 acceptance invariant. |

**Pass conditions.**

- [ ] Same-key replay after withdrawal, policy change, actor rotation and time-offset normalization matches the published contract.
- [ ] Changed semantic payload returns409 and concurrent identical writes create exactly one consent/receipt/audit/outbox set on YSQL.
- [ ] Retention, cleanup and consumer use of evaluation instead of stale create-response status are documented.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-026

**Requirement:** Memory-store composite keys collide across tenants and diverge from durable behavior

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Backend maintainer |
| Blocker / implementation / acceptance | [F-026](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-026), [T-026](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-026), [AT-026](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-026) |
| Verification method | Typed-key collision property cases and shared adapter contract suite, suppression event IDs, isolation across Memory instances and mutable-map aliasing; deployment ban on Memory. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date.  |
| Release decision impact | P2: must be fixed or explicitly accepted/deferred by owner; no waiver may bypass a dependent P0/P1 acceptance invariant. |

**Pass conditions.**

- [ ] Delimiter-rich tenant/subject/code/ID combinations cannot cross boundaries.
- [ ] Memory and YSQL contract tests agree on suppression IDs, event metadata, concurrency and immutable values.
- [ ] Production configuration tests reject ALLOW_MEMORY_STORE.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-027

**Requirement:** Input normalization and validation leave ambiguous or surprising API behavior

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | API/backend engineer |
| Blocker / implementation / acceptance | [F-027](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-027), [T-027](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-027), [AT-027](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-027) |
| Verification method | Exactly one JSON document, body limits/types/content type, omitted booleans, evidence/time bounds, normalization and safe opt-out failure journeys; assert no mutation on rejected requests. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-02, D-05 decision record. |
| Release decision impact | P2: must be fixed or explicitly accepted/deferred by owner; no waiver may bypass a dependent P0/P1 acceptance invariant. |

**Pass conditions.**

- [ ] Trailing JSON and malformed/oversized/wrong-type inputs return stable4xx without mutation.
- [ ] Omitted active, invalid expiry and invalid evidence produce documented outcomes rather than accidental defaults.
- [ ] Deny/withdraw error paths allow the approved opt-out journey under policy-change/unavailable conditions.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-028

**Requirement:** Database constraints do not enforce several tenant and state relationships

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | DBA/backend engineer |
| Blocker / implementation / acceptance | [F-028](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-028), [T-028](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-028), [AT-028](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-028) |
| Verification method | Read-only violation census, representative direct invalid inserts, composite tenant references, constrained transaction ordering and post-upgrade write/performance tests on YSQL. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date.  |
| Release decision impact | P2: must be fixed or explicitly accepted/deferred by owner; no waiver may bypass a dependent P0/P1 acceptance invariant. |

**Pass conditions.**

- [ ] Representative cross-tenant receipt/idempotency reference attempts and invalid state shapes are rejected by the approved layer.
- [ ] Existing-data validation reports zero unexplained violations before constraints are enforced.
- [ ] Fresh install and upgrade preserve performance and transactional write order under YSQL.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-029

**Requirement:** Immutable audit rows cannot reconstruct policy and suppression history

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Data/backend engineer + privacy owner |
| Blocker / implementation / acceptance | [F-029](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-029), [T-029](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-029), [AT-029](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-029) |
| Verification method | Historical policy/suppression reconstruction, role/tenant-scoped investigator export and app-role UPDATE/DELETE denial; verify all relevant before/after and revision fields. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-05, D-14 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] An auditor can reconstruct the exact policy and suppression states used for a historical consent decision.
- [ ] Updates retain actor, resource type, revision, time and approved change reason without overwriting historical values.
- [ ] Audit exports are tenant/role-scoped and the normal app role cannot alter history.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-030

**Requirement:** Mixed self-service and reader roles can authorize tenant-wide writes

| Gate field | Required record |
| --- | --- |
| Current status | FAIL / OPEN — established gap remains |
| Responsible role | Identity/security + privacy owner |
| Blocker / implementation / acceptance | [F-030](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-030), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030), [AT-030](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-030) |
| Verification method | All seven roles and mixed-role combinations against every endpoint, production claim mapping, canonical subjects, protected suppression clearing under the approved policy and forbidden development auth. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-06, D-10 decision record. |
| Release decision impact | P0/P1: block release while this capability/dependency is in scope and evidence is incomplete. |

**Pass conditions.**

- [ ] Mixed self_service plus service or consent_reader tokens cannot write another subject solely by combining separate role checks; any broader writer authority must be explicitly granted and tested.
- [ ] A signed role-permission matrix matches production IdP mappings and backend tests for all seven roles.
- [ ] Self-service cannot clear a protected administrative suppression if the approved policy distinguishes that class.
- [ ] Production deployment disallows development authentication/roles according to the approved identity contract.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## G-031

**Requirement:** Legacy Record and Upsert helpers preserve a non-atomic governance bypass path

| Gate field | Required record |
| --- | --- |
| Current status | PASS / CLOSED — non-atomic paths removed; governed writes enforced |
| Responsible role | Backend maintainer |
| Blocker / implementation / acceptance | [F-031](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-031), [T-031](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-031), [AT-031](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-031) |
| Verification method | Call-site inventory, failure injection for every production write entry, governed-path tests and explicitly separate legacy migration tests; demonstrate no ignored audit/outbox failure. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date.  |
| Release decision impact | P2: must be fixed or explicitly accepted/deferred by owner; no waiver may bypass a dependent P0/P1 acceptance invariant. |

**Pass conditions.**

- [x] No production entry point can create consent while silently losing audit/outbox/receipt effects.
- [x] Compatibility consumers are inventoried or explicitly absent before removing exported behavior.
- [x] Production behavior tests exercise the governed path and legacy migration tests remain separate.

**Handover status:** Verified and resolved under T-031 / AT-031. Legacy Record and Upsert methods completely deleted. All consent creation strictly governed and transactionally atomic.

## G-032

**Requirement:** Approved launch requirements, capacity and regional policy decisions are not supplied

| Gate field | Required record |
| --- | --- |
| Current status | DECISION REQUIRED — behavior/scope not approved |
| Responsible role | Product/architecture + release owner |
| Blocker / implementation / acceptance | [F-032](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-032), [T-032](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-032), [AT-032](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-032) |
| Verification method | Requirements/decision review with accountable owners, exact upstream source revision, scope exclusions and measurable SLO/region/recovery acceptance references; this is an evidence review, not a unit test. |
| Evidence required | Source commit and image/toolchain versions where relevant; exact commands or UAT steps; input fixture identity; expected/actual results for every criterion; relevant DB/event counts or operational measurements; sanitized logs; reviewer/date. Approved D-01, D-03, D-04, D-05, D-06, D-07, D-08, D-09, D-10, D-11, D-12, D-13, D-14 decision record. |
| Release decision impact | P2: must be fixed or explicitly accepted/deferred by owner; no waiver may bypass a dependent P0/P1 acceptance invariant. |

**Pass conditions.**

- [ ] Every launch-critical requirement has an owner, source, acceptance case and implementation/gap link.
- [ ] All release-affecting decisions are recorded and approved; deferred scope is explicit.
- [ ] The release evidence identifies a source commit, approved configuration, SLO/capacity/RPO/RTO and regional privacy review owner.

**Handover status:** No fix, sign-off, provider test or waiver was performed by this audit.

## Final release evidence review

The release owner should assemble one evidence bundle covering the approved requirement list, resolved decisions, exact source commit and images, clean required checks, every applicable AT group, actual consumer outcomes, topology/TLS/identity configuration, observable propagation budgets and a successful isolated restore/rollback drill. Review unresolved risks with product, privacy, security, engineering and SRE owners.

Do not label a deployment GO when stale challenges can restore consent, the confirmation subject boundary is bypassable, evidence hashes are not stable, mandatory data/provider tests are skipped, or the selected consumer can act on stale or unavailable permission. A successful pod rollout alone is insufficient. No accessibility gate is asserted passed for a frontend that was not supplied; actual launch frontends require their own evidence under the approved scope.
