# 20 — Open decisions and external dependencies

**Product:** NVUTO Consent  
**Audit date:** 19 October 2026 (command timestamps in UTC; raw log offsets preserved)  
**Source:** supplied ZIP; commit and branch UNKNOWN  
**Disposition:** documentation-only audit; original source preserved.

## State of the decision record

All **14 decisions** below are **OPEN / NOT APPROVED** at handover. They distinguish undocumented requirements from confirmed defects. Recommended resolutions are proposals for the incoming owners, not authorization already obtained. No named individual, provider credential, environment, deadline, launch region or numerical SLO has been invented.

The critical state/authorization failures require correction even while the broader product model is decided. In particular, missing consent must not become affirmative permission, an old DOI capability must not reverse later negative intent, and self-service authorization must bind the requested action to the permitted customer scope. A decision should define safe behavior, not re-label an observed bypass as a passed test.

## Decision index

| Decision | Question / owner | Blocks or changes | Status |
| --- | --- | --- | --- |
| [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01) | Approved launch and frontend scope — Product owner + architecture lead | T-024, T-032; determines T-023 scope | OPEN; owner approval absent |
| [D-02](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-02) | Independent DOI proof and delivery — Product + privacy + identity/delivery owner | T-001, T-002, T-003, T-027; EXT-05 | OPEN; owner approval absent |
| [D-03](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-03) | Consent ordering, effective time and older-record withdrawal — Product/privacy + backend/data lead | T-001, T-006, T-010, T-029 | OPEN; owner approval absent |
| [D-04](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-04) | Policy changes and legacy grants — Privacy owner + product + DBA | T-007, T-008, T-025, T-029 | OPEN; owner approval absent |
| [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05) | Evidence, notice lifecycle and retained proof — Privacy/legal reviewer + data architect | T-004, T-005, T-022, T-027, T-029 | OPEN; owner approval absent |
| [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06) | Role composition and protected suppression — Privacy + identity/security owner | T-002, T-030; role gates across all endpoints | OPEN; owner approval absent |
| [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07) | Initial consumer set and integration ownership — Product + each consumer engineering owner | T-003, T-010, T-023, T-024; INT-01–INT-08 | OPEN; owner approval absent |
| [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08) | Capacity, latency, propagation, availability and recovery budgets — Product + SRE + DBA + consumer owners | T-009, T-012, T-014, T-016, T-020, T-021, T-022, T-023 | OPEN; owner approval absent |
| [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) | Infrastructure and release ownership — Platform/SRE + release owner + DBA | T-015, T-016, T-018–T-022; EXT-03–EXT-11 | OPEN; owner approval absent |
| [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) | Production IdP and canonical Customer Graph mapping — Identity + Customer Graph + security owners | T-014, T-015, T-019, T-023, T-030; EXT-01, EXT-02 | OPEN; owner approval absent |
| [D-11](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-11) | Idempotency replay and retention — API/backend + consumer owner + privacy | T-025; API-04 | OPEN; owner approval absent |
| [D-12](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-12) | Evaluation scope and jurisdiction context — Product/privacy + architecture | T-006, T-032; API-07, API-08 | OPEN; owner approval absent |
| [D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13) | Event order, expiry and consumer reconciliation — API/event architecture + Integration Hub + consumer owners | T-010, T-020, T-023; EXT-04, INT-08 | OPEN; owner approval absent |
| [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) | Regional privacy obligations and evidence handling — Privacy/legal owner + product + data owner | T-005, T-008, T-022, T-029, T-032 | OPEN; owner approval absent |

## D-01

**Approved launch and frontend scope**

| Decision field | Required record |
| --- | --- |
| Question / conflict | Only README and rollback prose are supplied; service vs selected-consumer vs whole-platform launch is undefined. |
| Available choices | Standalone service with contract acceptance; selected named consumers; all eight consumers. A new standalone UI is a separate scope choice. |
| Recommended resolution | Approve one boundary and a requirements baseline. Keep identity/accounts/campaigns outside Consent; identify any actual frontend owner and repository. |
| Accountable role | Product owner + architecture lead; named assignee UNKNOWN |
| Dependent work | T-024, T-032; determines T-023 scope |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

## D-02

**Independent DOI proof and delivery**

| Decision field | Required record |
| --- | --- |
| Question / conflict | The initiator receives the confirmation secret, while communication execution belongs elsewhere. |
| Available choices | Trusted existing NVUTO delivery service; an explicitly approved new delivery adapter; disable DOI-dependent launch flows pending delivery. |
| Recommended resolution | Use an owned trusted delivery path, hide the capability from untrusted initiation responses, bind destination/version and generation, and approve resend/cancel/expiry/abuse behavior. |
| Accountable role | Product + privacy + identity/delivery owner; named assignee UNKNOWN |
| Dependent work | T-001, T-002, T-003, T-027; EXT-05 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

## D-03

**Consent ordering, effective time and older-record withdrawal**

| Decision field | Required record |
| --- | --- |
| Question / conflict | updated_at chooses current permission; stale tokens restore consent, at can predate grant, and older-row withdrawal supersedes a newer row. |
| Available choices | Only current-time evaluation with explicit unsupported-time errors; a versioned as-of timeline. In either option, define preference-level operation order. |
| Recommended resolution | Immediately prohibit stale challenge restoration. Prefer monotonic preference revision; explicitly choose historical/future support and whether withdrawal targets one record or the current preference. |
| Accountable role | Product/privacy + backend/data lead; named assignee UNKNOWN |
| Dependent work | T-001, T-006, T-010, T-029 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

**Conditional implementation dependency.** The current-only option fits the baseline ticket graph. If historical evaluation is approved, T-006 must own the common immutable consent/policy/suppression timeline foundation before T-007 consumes it for eligibility and T-029 adds investigator history/export. Alternatively, revise the ticket breakdown before implementation. Do not add T-029 as a predecessor of T-006: T-029 already depends on T-006 and that would create a cycle. T-032/D-03 must re-estimate full historical scope; the current4–8 day T-006 band is not a commitment to a complete historical governance engine.

## D-04

**Policy changes and legacy grants**

| Decision field | Required record |
| --- | --- |
| Question / conflict | Purpose/basis edits overwrite policy; existing grants and pending tokens bypass new requirements; blank-notice legacy rows skip checks. |
| Available choices | Immediate re-evaluation/recapture; bounded documented grandfathering by policy version; mixed rules with explicit class eligibility. |
| Recommended resolution | Version policies and record the rule applied. Do not silently grandfather blank evidence. Approve a legacy classification and additive reconciliation plan. |
| Accountable role | Privacy owner + product + DBA; named assignee UNKNOWN |
| Dependent work | T-007, T-008, T-025, T-029 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

## D-05

**Evidence, notice lifecycle and retained proof**

| Decision field | Required record |
| --- | --- |
| Question / conflict | Version-only notice records and limited receipts cannot prove accepted content/full decision; hashes change across timestamp encoding. |
| Available choices | Immutable artifact in Consent; immutable external content address with durable ownership; explicit canonical receipt versions and retention/export policy. |
| Recommended resolution | Bind exact content digest/reference and full relevant decision/provenance to a versioned canonical receipt; keep historic originals and record retirement/supersession separately. |
| Accountable role | Privacy/legal reviewer + data architect; named assignee UNKNOWN |
| Dependent work | T-004, T-005, T-022, T-027, T-029 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

## D-06

**Role composition and protected suppression**

| Decision field | Required record |
| --- | --- |
| Question / conflict | developer has admin authority, mixed roles union privileges, and self-service can clear its own suppression regardless of creator. |
| Available choices | Keep broad tenant role union with an approved issuer policy; separate high-risk privileges and protected administrative suppressions; define explicit precedence. |
| Recommended resolution | Approve all seven roles and mixed-role behavior. Distinguish administrative/legal blocks only if the product requires them, with provenance and negative authorization tests. |
| Accountable role | Privacy + identity/security owner; named assignee UNKNOWN |
| Dependent work | T-002, T-030; role gates across all endpoints |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

**Confirmed composition problem, separate from policy choice.** `cmd/server/main.go:109-118` lets self_service admit a writer route while service/consent_reader separately grants the broader subject bypass. The combination can write another customer within the tenant although neither role alone grants that other-customer write. This is F-030/P1, source-confirmed but not dynamically probed. G-030 remains mandatory until action and scope are authorized together or an explicit, enforced and tested issuer/deployment restriction excludes the affected combinations. The legal/admin suppression ownership choice must also be recorded; that undecided choice does not diminish the mixed-role defect.

## D-07

**Initial consumer set and integration ownership**

| Decision field | Required record |
| --- | --- |
| Question / conflict | Eight consumers are named but no code, environment or launch ownership is supplied. |
| Available choices | One pilot then phased consumers; specified subset; all eight only with per-consumer acceptance. |
| Recommended resolution | Name an accountable owner and staging workflow per launch consumer; document excluded consumers and contract/version compatibility. |
| Accountable role | Product + each consumer engineering owner; named assignee UNKNOWN |
| Dependent work | T-003, T-010, T-023, T-024; INT-01–INT-08 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

## D-08

**Capacity, latency, propagation, availability and recovery budgets**

| Decision field | Required record |
| --- | --- |
| Question / conflict | No numerical traffic/SLO/RPO/RTO/retention volumes are approved; fixed pools and retry/lease budgets exist. |
| Available choices | Targets based on measured intended load and business harm; explicitly limited pilot capacity; unbounded GA is not a testable target. |
| Recommended resolution | Record peak request/batch rates, tenant cardinality, p95/p99 latency, pool/connection budget, max suppression lag, backlog recovery, error budget, RPO and RTO before load/restore acceptance. |
| Accountable role | Product + SRE + DBA + consumer owners; named assignee UNKNOWN |
| Dependent work | T-009, T-012, T-014, T-016, T-020, T-021, T-022, T-023 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

## D-09

**Infrastructure and release ownership**

| Decision field | Required record |
| --- | --- |
| Question / conflict | Only baseline manifests are supplied; production universe, cluster, registry, secrets and operational ownership are unknown. |
| Available choices | Existing managed platform services with named owners; new provisioned platform under separately estimated infrastructure work. |
| Recommended resolution | Assign target environments and versions, immutable release provenance, least-privilege identities, migration/rollback ownership, alert/on-call and restore responsibility. |
| Accountable role | Platform/SRE + release owner + DBA; named assignee UNKNOWN |
| Dependent work | T-015, T-016, T-018–T-022; EXT-03–EXT-11 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

## D-10

**Production IdP and canonical Customer Graph mapping**

| Decision field | Required record |
| --- | --- |
| Question / conflict | JWT claim names are coded; actual issuer, audience, JWKS, tenant, role and canonical customer mapping are not provided. |
| Available choices | Existing Authentik application/provider with agreed claims; another explicitly compatible approved issuer. |
| Recommended resolution | Use issuer-scoped signed claims, canonical party IDs and tested role mapper. Specify key rotation/cache/outage behavior and token lifetimes; prohibit development modes in production. |
| Accountable role | Identity + Customer Graph + security owners; named assignee UNKNOWN |
| Dependent work | T-014, T-015, T-019, T-023, T-030; EXT-01, EXT-02 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

## D-11

**Idempotency replay and retention**

| Decision field | Required record |
| --- | --- |
| Question / conflict | Creation policy checks occur before ledger lookup; existing key returns current row rather than original response; actor enters fingerprint. |
| Available choices | Exact original response replay; one resource creation with current representation; defined actor/token rotation equivalence. |
| Recommended resolution | Publish one contract, canonicalize semantic inputs, retain authorization checks, define key namespace/lifetime and explain current evaluation to consumers. |
| Accountable role | API/backend + consumer owner + privacy; named assignee UNKNOWN |
| Dependent work | T-025; API-04 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

## D-12

**Evaluation scope and jurisdiction context**

| Decision field | Required record |
| --- | --- |
| Question / conflict | Input exposes at and consent stores scope/evidence/jurisdiction, but the supported policy-context decision matrix is not specified. |
| Available choices | Purpose/channel decision only with documented limitations; explicitly approved scope/context-aware extensions. |
| Recommended resolution | Document actual decision inputs and regional/purpose mapping before adding new policy semantics. Unsupported historical/context requests should have explicit outcomes. |
| Accountable role | Product/privacy + architecture; named assignee UNKNOWN |
| Dependent work | T-006, T-032; API-07, API-08 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

## D-13

**Event order, expiry and consumer reconciliation**

| Decision field | Required record |
| --- | --- |
| Question / conflict | schema_version is not a business revision; delivery can duplicate/reorder, and expiry is computed without emitting an expiry event. |
| Available choices | Consumers perform current authoritative evaluation; revisioned projections with reconciliation; scheduled expiry propagation if specifically required. |
| Recommended resolution | Use versioned tenant-scoped schemas, durable idempotency and monotonic revisions; prove stale grants cannot reverse withdrawal and expiry/policy changes converge. |
| Accountable role | API/event architecture + Integration Hub + consumer owners; named assignee UNKNOWN |
| Dependent work | T-010, T-020, T-023; EXT-04, INT-08 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

## D-14

**Regional privacy obligations and evidence handling**

| Decision field | Required record |
| --- | --- |
| Question / conflict | The fixture jurisdiction NG is not a launch-region approval. No applicable laws, lawful-basis determination, retention or rights workflows are supplied. |
| Available choices | Named initial regions with privacy review; constrained pilot jurisdiction; broader region support after explicit analysis. |
| Recommended resolution | Have the authorized privacy/legal owner identify applicable requirements, approve evidence/retention/right-to-delete vs immutable-history handling, and map each obligation to acceptance evidence. |
| Accountable role | Privacy/legal owner + product + data owner; named assignee UNKNOWN |
| Dependent work | T-005, T-008, T-022, T-029, T-032 |
| Current status | OPEN / NOT APPROVED |
| Evidence required to close | An owner-approved record of selected behavior/scope, rationale, rejected alternatives, affected requirements and contracts, measurable acceptance, implementation ownership and release applicability. |
| If unresolved | The affected design, estimate and release gate remain blocked or explicitly unverified. Work on independent defects and evidence collection may continue. |

**Legal evidence boundary.** This audit does not determine that NG fixtures establish Nigerian launch scope, certify compliance with any law, or prescribe a legal basis. The authorized privacy/legal owner must identify applicable rules and translate them into reviewed technical acceptance. The source audit establishes evidence limitations and data-lifecycle questions, not a legal opinion.

## External dependency and access register

The12 groups below are the same EXT identifiers as report11. They separate a declared interface from an available, working integration. An owner or configuration name in a document is not proof of provisioned access. Supply real credentials through the approved secrets channel; do not paste passwords, NATS credentials, JWT signing material or customer evidence into these reports or issue bodies.

| ID / dependency | Required input or proof | Proposed owner / present status | Tickets and decisions |
| --- | --- | --- | --- |
| EXT-01 — Authentik / compatible identity provider | Issuer, audience, JWKS endpoint, signed tenant/actor/roles/customer claims, rotation/outage policy | Identity/security owner; Implementation exists; actual provider NOT VERIFIED | [T-014](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-014), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030); [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| EXT-02 — Customer Graph canonical subject mapping | Authoritative canonical party IDs, tenant relationship and approved mapping into customer_subject_id | Customer Graph + identity + consumer owners; Claim field exists; identity mapping NOT VERIFIED | [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002), [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030); [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| EXT-03 — YugabyteDB YSQL universe | Exact target version/topology, isolated test DB, TLS trust, least-privilege role, placement/connection limits | DBA/platform owner; SQL adapter/migrations exist; live durable target NOT VERIFIED | [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-022](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-022), [T-028](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-028); [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) |
| EXT-04 — NATS JetStream cluster | Broker/stream/account, subjects, replicas, retention/dedup limits, credentials, TLS, subscriptions and replay ownership | Messaging/SRE + consumer owners; Publisher source exists; real broker/consumer acceptance NOT VERIFIED | [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020), [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021), [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023); [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13) |
| EXT-05 — Trusted DOI delivery component | Named service that receives protected capability, verified recipient binding, sandbox send/confirm and retry/resend/cancel contract | Delivery product/engineering owner; Delivery connection ABSENT from supplied service; owner UNKNOWN | [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023), [T-027](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-027); [D-02](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-02), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07) |
| EXT-06 — Container registry and release artifacts | Authoritative repository/commit, image registry paths, immutable digests, provenance/SBOM, prior rollback image | Release/platform owner; Docker recipes present; published image identity NOT VERIFIED | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018); [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) |
| EXT-07 — CI execution and required checks | Repository access, workflow runner, required branch checks, retained logs, DB/broker test isolation and scan policy | CI/repository owner; Two jobs defined; no actual successful CI run supplied | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017), [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018); [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) |
| EXT-08 — Kubernetes platform and gateway | Cluster/context/namespace, admission policy, network/TLS gateway, scheduling, resource quotas and rollout permissions | Platform/SRE owner; Desired manifests present; referenced external inputs and actual runtime NOT VERIFIED | [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015), [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021); [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) |
| EXT-09 — Observability, alerting and on-call | Metrics/log/trace destination, request/event correlation, backlog/lag alerts, dashboards, notification routing and incident owner | SRE/on-call + backend owner; Basic logs only in supplied code; integrated monitoring NOT VERIFIED | [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013), [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021); [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) |
| EXT-10 — Secrets and certificate lifecycle | Secure provision/rotation for DB, broker, IdP configuration and trust material; least-privilege identities | Security/platform + service owners; Secret references/options exist; production values/policies NOT PROVIDED | [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030); [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| EXT-11 — Backup, PITR, restore and recovery | Target-universe backup access, isolated restore environment, RPO/RTO, evidence retention and broker/outbox reconciliation | DBA/SRE + privacy owner; README requirement exists; successful drill/runbook NOT VERIFIED | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020), [T-022](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-022), [T-029](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-029); [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) |
| EXT-12 — Approved launch consumer applications | Selected repositories/builds and accountable owners, current API/event call sites, UI/processing actions and UAT environment | Product + integration lead + consumer owners; Eight names supplied; zero actual consumer integrations verified | [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023), [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030), [T-032](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-032); [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07), [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10), [D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13) |

## Consumer sub-register

| ID | Named consumer | Current readiness | Required scope disposition |
| --- | --- | --- | --- |
| INT-01 | Engage | NOT VERIFIED; no repository/client/subscription/UAT supplied | D-01/D-07 select launch inclusion. If included, complete T-023/AT-023 with actual application action and permission outcome; otherwise record explicit deferral. |
| INT-02 | Service | NOT VERIFIED; no repository/client/subscription/UAT supplied | D-01/D-07 select launch inclusion. If included, complete T-023/AT-023 with actual application action and permission outcome; otherwise record explicit deferral. |
| INT-03 | Contact Center | NOT VERIFIED; no repository/client/subscription/UAT supplied | D-01/D-07 select launch inclusion. If included, complete T-023/AT-023 with actual application action and permission outcome; otherwise record explicit deferral. |
| INT-04 | Customer Portal | NOT VERIFIED; no repository/client/subscription/UAT supplied | D-01/D-07 select launch inclusion. If included, complete T-023/AT-023 with actual application action and permission outcome; otherwise record explicit deferral. |
| INT-05 | CRM | NOT VERIFIED; no repository/client/subscription/UAT supplied | D-01/D-07 select launch inclusion. If included, complete T-023/AT-023 with actual application action and permission outcome; otherwise record explicit deferral. |
| INT-06 | Conversations | NOT VERIFIED; no repository/client/subscription/UAT supplied | D-01/D-07 select launch inclusion. If included, complete T-023/AT-023 with actual application action and permission outcome; otherwise record explicit deferral. |
| INT-07 | AI | NOT VERIFIED; no repository/client/subscription/UAT supplied | D-01/D-07 select launch inclusion. If included, complete T-023/AT-023 with actual application action and permission outcome; otherwise record explicit deferral. |
| INT-08 | Integration Hub | NOT VERIFIED; no repository/client/subscription/UAT supplied | D-01/D-07 select launch inclusion. If included, complete T-023/AT-023 with actual application action and permission outcome; otherwise record explicit deferral. |

These are consumer-specific interfaces, not8 new applications the engineer should build from scratch. Report07 describes the distinct integration acceptance areas. Customer Portal needs authorized initial state and safe preference/DOI/reload behavior; communication consumers need a verified decision at their actual send boundary; Integration Hub needs version/tenant/replay/reconciliation behavior. The exact AI processing purpose and Service/Contact Center policy mapping require approval rather than guessed generic consent semantics.

## Prerequisite information for the incoming engineer

1. Recover the authoritative Git repository/commit and any original PRDs/TRDs. The ZIP folder name does not establish a branch. Preserve this archive's hashes as the audited baseline.
2. Assign the product/privacy, architecture/backend, identity, DBA/messaging, platform/release, QA and consumer owners. Confirm which approvals are required by the actual organization.
3. Obtain isolated test-environment access and version/topology details for YSQL and NATS before running the existing mutation fixtures. Ordinary PostgreSQL compatibility is not target-YSQL acceptance.
4. Obtain the approved IdP claim contract and Customer Graph mapping, including allowed role combinations, token lifetime and key rotation. Use synthetic test subjects and negative authorization cases.
5. Define operational budgets, rollout, release provenance, migration and incident/recovery ownership before scheduling GA verification. Fixed code constants are not approved SLOs.
6. Identify whether existing external projections and delivery services satisfy read/bootstrap and DOI proof requirements; estimate missing approved work only after inspection.

## Assumptions explicitly rejected

The audit does not assume a hidden frontend is broken, that all8 named consumers must launch together, that the test jurisdiction is the operating region, that PostgreSQL is the production database, that mutable image tags identify a reproducible release, that development role flags are safely configured in production, or that passed local tests prove restored/replicated data. No unknown is marked resolved.

## Updating this register

For each decision or dependency, record the actual owner, date, source/approval reference, selected contract/environment version, verification link and gate impact. Update requirement/feature/workflow matrices and the matching backlog ticket together. If new evidence expands scope, add traceability before estimating work; do not silently overwrite the README-derived baseline or relax a safety acceptance condition.
