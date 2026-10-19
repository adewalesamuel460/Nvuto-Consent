# 00 — Executive audit summary

**Product:** NVUTO Consent  
**Audit date:** 19 October 2026 (command timestamps in UTC; raw log offsets preserved)  
**Source:** supplied ZIP; commit and branch UNKNOWN  
**Disposition:** documentation-only audit; original source preserved.

## Decision: NO-GO

NVUTO Consent has a real, compact backend implementation, but it is **not ready for production in its current state**. An older double-opt-in challenge can restore permission after a later denial or withdrawal. Self-service confirmation omits the customer-subject authorization check, and initiation returns the very secret a caller can immediately echo to obtain a grant. Receipt hashes also change after the actual pgx timestamp codec round trip. These are concrete defects, not merely missing deployment evidence.

The audit produced **32 findings, 32 implementation tickets, 32 acceptance groups and 32 release gates**. Priorities are **1 P0, 23 P1 and 8 P2**. No implementation fix, database migration, provider communication, production mutation or deployment was performed. The handover preserves every original source file and adds this audit directory.

## What this product is meant to do

The README describes a shared privacy, communication-preference, lawful-basis and consent service for the NVUTO Customer Platform. It owns purpose/channel decisions, policy references, consent evidence and dates, withdrawal, suppression, self-service preference APIs and change events. Its declared functional sequence is capture with evidence, deterministic evaluation, respect effective/withdrawn state, expose self-service changes, propagate suppression/change events and retain immutable history. Where affirmative consent is required, a consumer must never treat missing consent as permission (`README.md:3-11`).

The named consumers are Engage, Service, Contact Center, Customer Portal, CRM, Conversations, AI and Integration Hub. The service does not own customer identity, campaign execution, conversations, legal contracts or analytics data. No frontend application, account signup/recovery system, payment flow or mobile application is supplied; none is silently added to the approved requirements. Revenue model, launch regions, detailed personas and numerical service objectives are UNKNOWN. Test fixtures using NG do not establish a legal operating region.

## Evidence coverage

| Record type | Count | Meaning |
| --- | --- | --- |
| Original source files | 32 | All supplied32 files inspected; 19 Go files; 10 test files; no Git history supplied |
| Original lines / bytes | 3,958 / 159,241 | Computed from source inventory, not estimated |
| Engineering modules / deployable processes | 8 / 2 | Eight audited code/configuration groupings; one HTTP API and one outbox worker |
| Source-backed requirements / capability records | 22 / 34 | README-derived requirements and individually inventoried implementation capabilities |
| Business / operational endpoints | 11 / 2 | All routes mapped; health endpoints are separate from the business API count |
| Frontend apps / screens / UI routes / client call sites | 0 / 0 / 0 / 0 | Absent from this archive; external products not inspected |
| Persisted tables / numbered migrations | 9 / 3 | YugabyteDB YSQL through pgx; full schema and lifecycle inspected |
| Named consumer integrations / external dependency groups | 8 / 12 | Named and mapped, not runtime-verified integrations |
| Workflow families / recognized role strings | 11 / 7 | Service-facing workflow families and actual JWT role strings |
| Original tests / audit probes | 18 / 12 | 15 original PASS,3 SKIP;11 extra safety assertions FAIL and1 observation recorded |
| Findings / tickets / acceptance groups / gates | 32 / 32 / 32 / 32 | One consistent identifier chain; no orphan P0/P1 finding |
| Reports | 23 | All required00–22 reports included |

The **52.2% statement coverage** measures the executed original Go test corpus under `-coverpkg=./...`; it is not a production-readiness score. Durable database and worker paths largely remain unexecuted in this environment. Evidence logs and coverage data are included under `evidence/`.

## What is built, and what has been verified

The source has two Go entry points, eleven business operations, a JWT/JWKS authentication module, a service layer, Memory and YSQL repository implementations, nine tables, three migrations and a SQL-outbox-to-JetStream worker. Governed creation, withdrawal, registries, suppression and confirmation use SQL transactions to couple state, receipt where relevant, audit and outbox. SQL queries are parameterized and tenant-scoped. Receipt, notice and audit tables have immutability triggers. DOI tokens are cryptographically random, stored hashed, expiring and single-use on the same row. These are meaningful foundations, although source presence is not production acceptance.

Executed verification established:

- The original suite has **15 PASS, 0 FAIL and 3 SKIP**, repeated with the race detector and no reported race in executed tests.
- Module integrity, formatting and `go vet` passed. Both binaries built with `-buildvcs=false`; the ordinary build first encountered the surrounding archive/workspace VCS-stamping limitation, not a compiler defect.
- YAML parsing and shell syntax checks passed. These do not prove valid Kubernetes admission, working images or successful deployment.
- Twelve additional probes ran only in a disposable source copy. Eleven failed the desired-safety assertions; one intentionally recorded the undecided behavior of withdrawing an older consent.

## What is partial, missing, or unverified

Policy registries, consent transitions, evidence, idempotency, batch evaluation and suppression are implemented but not complete against the documented functional standard. The evaluator does not provide a reliable historical timeline; stronger policy is not automatically re-applied to earlier grants; blank-notice legacy data skips governed checks. Audit immutability preserves rows but not the policy/suppression values that were never recorded. The event contract does not supply a monotonic business revision to reject stale deliveries.

The supplied API has no current-preference collection, registry listing, consent GET/list, suppression listing or public audit-history projection. An external application could own these reads; the owner and contract must be decided before treating self-service/admin journeys as complete. OpenAPI and a maintained consumer/event contract are absent. The independent DOI delivery connection, actual consumer acceptance evidence, operational metrics/alerts and target restore/recovery evidence are missing from the supplied handover. This does not prove that unseen platform repositories lack those components.

All three external database/broker tests were skipped because their prerequisites were unavailable. No actual Authentik application, canonical Customer Graph mapping, YSQL universe, NATS cluster, Kubernetes environment, CI run, consumer app, load test or restore drill was accessed. The database therefore remains **NOT VERIFIED for production**, with additional source-confirmed integrity and migration gaps.

## Highest-priority risks and required work

| Area | Established issue / limit | Finding and delivery references |
| --- | --- | --- |
| Consent integrity | Older challenges override later negative intent; temporal and policy-transition rules remain unsafe or undefined. | [F-001](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-001), [F-006](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-006), [F-007](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-007), [F-008](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-008), [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001) |
| Authorization and DOI proof | Different same-tenant self-service subject can confirm a possessed token; initiating caller can bypass independent delivery; mixed self-service/reader roles can authorize tenant-wide writes. | [F-002](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-002), [F-003](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-003), [F-030](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-030), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002), [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030) |
| Evidence | Actual pgx timestamp precision changes receipt hashes; exact notice/full decision and historical policy/suppression values are not bound. | [F-004](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-004), [F-005](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-005), [F-029](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-029) |
| Event reliability | Lease updates lack owner fencing; stale/duplicate/out-of-order deliveries and existing-stream drift need explicit handling. | [F-009](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-009), [F-010](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-010), [F-020](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-020) |
| Security and runtime | Pinned Go/dependencies have scanner findings; JWKS miss amplification, unbounded DB contexts and weak readiness remain. | [F-011](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-011), [F-012](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-012), [F-014](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-014), [F-015](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-015) |
| Durable verification | YSQL HTTP/concurrency/HA, store idempotency CI and migration003 upgrade are unproven or omitted. | [F-016](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-016), [F-017](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-017) |
| Release/operations | Reproducible artifacts, target TLS/identity, progress alerts and verified backup/recovery are not established. | [F-018](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-018), [F-019](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-019), [F-021](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-021), [F-022](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-022) |
| Platform outcome | None of the eight named consumer integrations has supplied end-to-end evidence. Gate applies to the approved launch set. | [F-023](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-023), [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023), [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07) |

The vulnerability scan identified **19 advisory findings:10 with symbol traces and9 package/module-only findings**. The10 are nine standard-library advisories and one x/text advisory. Some paths require HTTP/2 server configuration or a different precis profile not shown in this application. The result establishes a maintenance/security gate, not19 demonstrated exploits. The exact versions, conditional reachability and primary sources are in report10. Go1.25 is outside its supported release window on the audit date; patched supported Go1.26.9/1.27.2 lines are identified by the official release history and advisories. See [Go release history](https://go.dev/doc/devel/release) and the captured scan evidence.

## Recommended ownership and order

1. Approve the launch boundary and decision register; recover the authoritative source commit and missing upstream requirements. Assign product/privacy, backend/data, identity/security, SRE/DBA and consumer owners.
2. Start independent safety work: T-001/T-002/T-004/T-009/T-011/T-014/T-017, with T-024/T-030/T-032 discovery in parallel.
3. Build one coherent preference revision, time and policy model; complete independent DOI proof, canonical evidence and historical governance before migrating legacy data.
4. Stabilize request cancellation, errors, health, event schemas and worker fencing. Prove durable behavior against the target YSQL version, not only Memory or PostgreSQL-compatible syntax.
5. Produce immutable release artifacts and verified infrastructure, TLS/least-privilege identities, stream durability, observability and recovery procedures.
6. Execute actual selected-consumer journeys, including refusal to communicate after denial, withdrawal, suppression, expiry and service failure. Complete all applicable release gates with source/environment-specific evidence.

Workstreams and exact predecessor tickets are in report22. Estimates in report16 are bounded person-day planning ranges, not calendar promises; unseen consumer/frontend and new infrastructure scope needs discovery.

## What production-ready means here

Production acceptance requires correct and deterministic permission decisions; no cross-subject authorization or DOI proof bypass; verifiable, durable consent/notice/audit evidence; safe fresh and legacy migrations; atomic YSQL effects under concurrency and recovery; consumers that cannot restore permission from stale events; patched and reviewed release artifacts; enforced identities/TLS; measured operational objectives; and demonstrated restore/rollback. Approved scope, numerical targets and legal/evidence decisions must be explicit. **A green unit suite or healthy pod alone does not meet this standard.**

## Documents and evidence

Start with this summary, report15 for all risks, report16 for executable tickets, report22 for execution order and report19 for release acceptance. Report21 records provenance, commands, exact results and limitations. All23 reports are indexed below, and the adjacent `evidence/` directory contains inventories, raw verification logs, coverage, scanner results and audit probe source as non-executable `.go.txt` evidence.

| Report | Purpose |
| --- | --- |
| [00_EXECUTIVE_AUDIT_SUMMARY.md](00_EXECUTIVE_AUDIT_SUMMARY.md) | Executive audit summary |
| [01_REPOSITORY_ARCHITECTURE_MAP.md](01_REPOSITORY_ARCHITECTURE_MAP.md) | Repository architecture map |
| [02_DOCUMENTATION_REQUIREMENTS_TRACEABILITY.md](02_DOCUMENTATION_REQUIREMENTS_TRACEABILITY.md) | Documentation requirements traceability |
| [03_COMPLETE_PRODUCT_FEATURE_AUDIT.md](03_COMPLETE_PRODUCT_FEATURE_AUDIT.md) | Complete product feature audit |
| [04_USER_PERSONAS_ROLES_AND_PERMISSIONS.md](04_USER_PERSONAS_ROLES_AND_PERMISSIONS.md) | User personas roles and permissions |
| [05_FRONTEND_AUDIT.md](05_FRONTEND_AUDIT.md) | Frontend audit |
| [06_BACKEND_AND_API_AUDIT.md](06_BACKEND_AND_API_AUDIT.md) | Backend and api audit |
| [07_FRONTEND_BACKEND_INTEGRATION_MATRIX.md](07_FRONTEND_BACKEND_INTEGRATION_MATRIX.md) | Frontend backend integration matrix |
| [08_DATABASE_AND_DATA_SCHEMA_AUDIT.md](08_DATABASE_AND_DATA_SCHEMA_AUDIT.md) | Database and data schema audit |
| [09_APPLICATION_USER_FLOW_AUDIT.md](09_APPLICATION_USER_FLOW_AUDIT.md) | Application user flow audit |
| [10_SECURITY_PRIVACY_AND_ACCESS_CONTROL_AUDIT.md](10_SECURITY_PRIVACY_AND_ACCESS_CONTROL_AUDIT.md) | Security privacy and access control audit |
| [11_THIRD_PARTY_INTEGRATIONS_AUDIT.md](11_THIRD_PARTY_INTEGRATIONS_AUDIT.md) | Third party integrations audit |
| [12_DEVOPS_INFRASTRUCTURE_AND_DEPLOYMENT_AUDIT.md](12_DEVOPS_INFRASTRUCTURE_AND_DEPLOYMENT_AUDIT.md) | Devops infrastructure and deployment audit |
| [13_NON_FUNCTIONAL_REQUIREMENTS_AUDIT.md](13_NON_FUNCTIONAL_REQUIREMENTS_AUDIT.md) | Non functional requirements audit |
| [14_TESTING_QA_AND_VERIFICATION_REPORT.md](14_TESTING_QA_AND_VERIFICATION_REPORT.md) | Testing qa and verification report |
| [15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md) | Production blockers and risk register |
| [16_DETAILED_IMPLEMENTATION_BACKLOG.md](16_DETAILED_IMPLEMENTATION_BACKLOG.md) | Detailed implementation backlog |
| [17_PRODUCTION_COMPLETION_ROADMAP.md](17_PRODUCTION_COMPLETION_ROADMAP.md) | Production completion roadmap |
| [18_NEW_ENGINEER_HANDOVER_GUIDE.md](18_NEW_ENGINEER_HANDOVER_GUIDE.md) | New engineer handover guide |
| [19_PRODUCTION_GO_LIVE_CHECKLIST.md](19_PRODUCTION_GO_LIVE_CHECKLIST.md) | Production go live checklist |
| [20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md) | Open decisions and external dependencies |
| [21_AUDIT_EVIDENCE_AND_COMMAND_LOG.md](21_AUDIT_EVIDENCE_AND_COMMAND_LOG.md) | Audit evidence and command log |
| [22_ENGINEERING_EXECUTION_ORDER.md](22_ENGINEERING_EXECUTION_ORDER.md) | Engineering execution order |
