# 15 — Production blockers and risk register

**Product:** NVUTO Consent  
**Audit date:** 19 October 2026 (command timestamps in UTC; raw log offsets preserved)  
**Source:** supplied ZIP; commit and branch UNKNOWN  
**Disposition:** documentation-only audit; original source preserved.

## Release recommendation and severity method

**NO-GO.** The register contains **32 findings: 1 P0, 23 P1 and 8 P2**. Severity describes potential release impact under the documented service purpose; evidence status describes what was actually established. A P1 evidence gap is not proof that an unseen production environment is broken. It means its mandatory readiness claim cannot be accepted from this archive.

| Severity | Meaning in this audit | Release treatment |
| --- | --- | --- |
| P0 | A reproduced failure can restore consent after later negative intent, undermining a core privacy decision invariant. | Block release of the affected path until correction and regression evidence. |
| P1 | Material authorization, evidence-integrity, decision, delivery, security or mandatory production-verification gap. | Block the applicable launch scope until fixed and verified, or explicitly remove/defer that scope with owner-approved evidence. |
| P2 | Contract, development parity, maintainability, hardening or requirements decision gap. | Close or record an explicit, bounded disposition. A P2 that is a dependency of a mandatory workflow still blocks that workflow. |

These are audit priorities, not CVSS scores or legal conclusions. There are no speculative P0 findings, no readiness percentage, and no claim that every scanner trace is exploitable. All owners below are proposed accountable roles; no individual has been assigned by the user.

## Prioritized index

| Finding | Priority | Category / title | Evidence status | Task / acceptance / gate |
| --- | --- | --- | --- | --- |
| [F-001](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-001) | P0 | Consent state — Older double-opt-in challenges can restore permission after a later denial or withdrawal | CONFIRMED — memory reproduction; durable implementation traced | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [AT-001](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-001), [G-001](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-001) |
| [F-002](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-002) | P1 | Authorization — Double-opt-in confirmation omits the self-service customer boundary | CONFIRMED — HTTP handler reproduction with memory store | [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002), [AT-002](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-002), [G-002](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-002) |
| [F-003](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-003) | P1 | Double opt-in integration — Self-service receives the secret needed to bypass independent double-opt-in confirmation | CONFIRMED — HTTP start and immediate confirmation reproduced; delivery design undecided | [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [AT-003](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-003), [G-003](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-003) |
| [F-004](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-004) | P1 | Evidence integrity — Receipt hashes do not survive the actual pgx timestamp precision round trip | CONFIRMED — pgx codec reproduction; live YSQL round trip not run | [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004), [AT-004](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-004), [G-004](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-004) |
| [F-005](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-005) | P1 | Consent evidence — Notice and receipt records do not bind the accepted notice content and full decision | CONFIRMED — schema and source gap | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [AT-005](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-005), [G-005](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-005) |
| [F-006](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-006) | P1 | Decision engine — Evaluation time and current-state ordering do not implement a deterministic consent timeline | CONFIRMED behavior — time semantics and supersession require decision | [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [AT-006](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-006), [G-006](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-006) |
| [F-007](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-007) | P1 | Policy governance — Stronger policy requirements are not re-enforced against existing grants or pending confirmations | CONFIRMED — memory counterexamples; grandfathering undecided | [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [AT-007](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-007), [G-007](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-007) |
| [F-008](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-008) | P1 | Migration safety — Upgraded legacy grants bypass all governed-policy checks when notice_version is blank | RESOLVED — D-04 approved; category-based legacy eligibility rule and AT-008 tests verified | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [AT-008](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-008), [G-008](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-008) |
| [F-009](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-009) | P1 | Outbox reliability — Outbox lease completion and retry are not fenced to the current worker | CONFIRMED — source defect; failover scenario not executed | [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [AT-009](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-009), [G-009](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-009) |
| [F-010](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-010) | P1 | Event contracts — Events lack the ordering and reconciliation contract needed to keep consumers consistent | CONFIRMED — payload/schema gap; consumer behavior unavailable | [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [AT-010](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-010), [G-010](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-010) |
| [F-011](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-011) | P1 | Dependency security — Pinned toolchain and dependencies fail the current vulnerability gate | CONFIRMED — scanner results; exploitability varies by configuration | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [AT-011](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-011), [G-011](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-011) |
| [F-012](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-012) | P1 | Backend reliability — Database work cannot follow request cancellation or a bounded transaction budget | CONFIRMED — source gap; operational impact not load-tested | [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [AT-012](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-012), [G-012](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-012) |
| [F-013](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-013) | P2 | API error contract — Unexpected database errors are returned as 422 with internal error text | CONFIRMED — source defect | [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013), [AT-013](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-013), [G-013](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-013) |
| [F-014](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-014) | P1 | Authentication availability — Unknown JWT key IDs trigger an unbounded JWKS refresh path | CONFIRMED — five misses caused five local JWKS fetches | [T-014](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-014), [AT-014](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-014), [G-014](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-014) |
| [F-015](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-015) | P1 | Operational readiness — Readiness can report healthy while authentication or required schema is unusable | CONFIRMED — source behavior; cluster not exercised | [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015), [AT-015](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-015), [G-015](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-015) |
| [F-016](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-016) | P1 | Release evidence — Durable API, transaction and multi-node Yugabyte behavior remain unverified | NOT VERIFIED — required infrastructure unavailable in audit | [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [AT-016](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-016), [G-016](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-016) |
| [F-017](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-017) | P1 | CI and migration testing — CI omits durable idempotency coverage and migration 003 upgrade verification | CONFIRMED — workflow and fixture omissions | [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017), [AT-017](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-017), [G-017](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-017) |
| [F-018](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-018) | P1 | Deployment — Release packaging and deployment are not reproducible from the supplied manifests | CONFIRMED — configuration gaps; actual hosting unknown | [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018), [AT-018](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-018), [G-018](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-018) |
| [F-019](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-019) | P1 | Infrastructure security — Transport security and least-privilege service identities are not established | RESOLVED / VERIFIED (T-019) | [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [AT-019](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-019), [G-019](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-019) |
| [F-020](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-020) | P1 | Broker configuration — Existing NATS streams are accepted without checking durability, subjects or retention | CONFIRMED — source gap; broker configuration unknown | [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020), [AT-020](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-020), [G-020](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-020) |
| [F-021](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-021) | P1 | Observability — Operators cannot reliably detect stalled consent propagation or diagnose request failures | RESOLVED — implemented and verified (T-021) | [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021), [AT-021](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-021), [G-021](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-021) |
| [F-022](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-022) | P1 | Data operations — Backup, restore, retention and recovery procedures are not production-evidenced | NOT VERIFIED — required operational policy absent | [T-022](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-022), [AT-022](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-022), [G-022](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-022) |
| [F-023](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-023) | P1 | Platform integration — Named NVUTO product integrations have no end-to-end verification in this repository | NOT VERIFIED — consumer repositories and evidence not supplied | [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023), [AT-023](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-023), [G-023](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-023) |
| [F-024](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-024) | P2 | API completeness — Preference discovery, registry reads and a machine-readable client contract are absent | CONFIRMED absence — new endpoint scope requires decision | [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024), [AT-024](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-024), [G-024](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-024) |
| [F-025](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-025) | P2 | Idempotency contract — Idempotency replay semantics change with policy and current consent state | CONFIRMED — source behavior; intended replay semantics undecided | [T-025](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-025), [AT-025](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-025), [G-025](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-025) |
| [F-026](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-026) | P2 | Development-store correctness — Memory-store composite keys collide across tenants and diverge from durable behavior | CONFIRMED — tenant collision reproduced; production disabled by default | [T-026](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-026), [AT-026](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-026), [G-026](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-026) |
| [F-027](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-027) | P2 | API quality — Input normalization and validation leave ambiguous or surprising API behavior | CONFIRMED — trailing JSON reproduced; other cases source-traced | [T-027](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-027), [AT-027](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-027), [G-027](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-027) |
| [F-028](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-028) | P2 | Schema integrity — Database constraints do not enforce several tenant and state relationships | CONFIRMED — schema gap; no cross-tenant SQL exploit demonstrated | [T-028](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-028), [AT-028](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-028), [G-028](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-028) |
| [F-029](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-029) | P1 | Auditability — Immutable audit rows cannot reconstruct policy and suppression history | CONFIRMED — schema and mutation payload gap | [T-029](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-029), [AT-029](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-029), [G-029](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-029) |
| [F-030](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-030) | P1 | Authorization and permission policy — Mixed self-service and reader roles can authorize tenant-wide writes | CONFIRMED — source-level role composition; live issued combinations NOT VERIFIED | [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030), [AT-030](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-030), [G-030](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-030) |
| [F-031](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-031) | P2 | Maintainability — Legacy Record and Upsert helpers preserve a non-atomic governance bypass path | CONFIRMED — internal-only path, not an active HTTP endpoint | [T-031](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-031), [AT-031](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-031), [G-031](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-031) |
| [F-032](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-032) | P2 | Product handover — Approved launch requirements, capacity and regional policy decisions are not supplied | DECISION REQUIRED — only README/rollback prose exists | [T-032](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-032), [AT-032](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-032), [G-032](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-032) |

## F-001

**Older double-opt-in challenges can restore permission after a later denial or withdrawal**

| Field | Audit record |
| --- | --- |
| Severity / category | P0 / Consent state |
| Status | CONFIRMED — memory reproduction; durable implementation traced |
| Affected module(s) | MOD-04 — Consent service; MOD-05 — Persistence |
| Affected users/roles | Data subjects, writers and all consuming apps |
| Affected APIs | API-04, API-05, API-09, API-10 |
| Database implications | consents, consent_receipts, consent_audit_events, consent_outbox_events |
| Proposed accountable role | Backend/data lead + privacy owner |
| Effort | 4–7 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | [D-03](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-03) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [AT-001](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-001), [G-001](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-001) |

**Description, business and technical impact.** A previously issued challenge remains usable after the subject changes their mind. Confirmation makes the old row newest and evaluation returns allowed=true. Two audit probes reproduced denial and withdrawal being overridden. This breaks the documented withdrawal/effective-state safety rule.

**Evidence and root-cause boundary.** `internal/service/service.go:256-325`; `internal/store/postgres.go:340-383`; `internal/store/postgres.go:142-143`; `internal/store/memory.go:286-314`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Introduce a transactionally controlled preference generation or current-state record keyed by tenant, canonical subject, purpose and channel. Bind every challenge to that generation. Denial, withdrawal and superseding challenges must atomically invalidate prior challenges. Recheck generation, current policy and expiry under the same YSQL transaction before creating receipt/audit/outbox. Preserve historical evidence; migrate additively.

**Measurable acceptance.**

1. Starting a challenge, recording a later denial, and confirming the older challenge never grants permission.
2. An older challenge cannot restore a newer consent after that newer consent is withdrawn.
3. Concurrent confirm/deny/withdraw operations converge to the approved order with one atomic state/audit/outbox outcome.
4. Re-run the two stale-challenge audit probes and equivalent tests on real YSQL.

**Verification.** Memory regressions plus real YSQL concurrency tests for challenge→deny, challenge→new grant→withdraw, simultaneous confirms, retries and stale generation. Assert consent, receipt, audit and outbox counts, not only Allowed. See [AT-001](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-001). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-002

**Double-opt-in confirmation omits the self-service customer boundary**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Authorization |
| Status | CONFIRMED — HTTP handler reproduction with memory store |
| Affected module(s) | MOD-01 — HTTP API; MOD-04 — Consent service |
| Affected users/roles | self_service |
| Affected APIs | API-10 |
| Database implications | consents, consent_receipts |
| Proposed accountable role | API/security engineer |
| Effort | 1–3 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002), [AT-002](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-002), [G-002](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-002) |

**Description, business and technical impact.** A self_service JWT for customer B confirmed customer A's valid token in the same tenant and received A's granted consent. A valid victim challenge token is required; the audit does not claim tokens can be guessed or cross-tenant confirmation succeeds.

**Evidence and root-cause boundary.** `cmd/server/main.go:109-118`; `cmd/server/main.go:343-362`; `internal/service/service.go:305-325`; `README.md:41`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Pass the authenticated principal/customer constraint into a confirmation operation that resolves the pending challenge and enforces object authorization before any mutation. Recheck inside the atomic confirmation path or bind authorization to the locked record. Preserve broader tenant roles deliberately; reject self_service tokens missing customer_subject_id. Avoid leaking receipt or consent data on rejection.

**Measurable acceptance.**

1. Customer B using customer A's challenge token receives a uniform forbidden/not-found result and causes no state, receipt, audit or outbox mutation.
2. Own-subject confirmation and approved tenant-wide writer/admin flows succeed.
3. Cross-tenant, missing-customer-claim and mixed-role cases follow an approved permission matrix.

**Verification.** Authenticated handler tests using production-compatible RS256 tokens for every role/tenant/customer combination; repeat against YSQL and assert rejected requests cause zero writes. See [AT-002](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-002). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-003

**Self-service receives the secret needed to bypass independent double-opt-in confirmation**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Double opt-in integration |
| Status | CONFIRMED — HTTP start and immediate confirmation reproduced; delivery design undecided |
| Affected module(s) | MOD-01 — HTTP API; MOD-03 — Domain contracts; MOD-04 — Consent service |
| Affected users/roles | self_service, trusted delivery integrator |
| Affected APIs | API-09, API-10 |
| Database implications | consents, consent_outbox_events |
| Proposed accountable role | Backend + trusted delivery integration owner |
| Effort | 4–8 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002) |
| Open decisions | [D-02](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-02), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [AT-003](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-003), [G-003](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-003) |

**Description, business and technical impact.** A self-service caller receives confirmation_token and immediately echoes it to obtain allowed=true without any delivery step. The challenge is bound to a channel label, not a verified destination. No message-delivery adapter or trusted delivery contract is present.

**Evidence and root-cause boundary.** `cmd/server/main.go:319-340`; `internal/domain/model.go:127-130`; `internal/service/service.go:271-302`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Decide which trusted NVUTO component owns confirmation delivery. Return an opaque challenge reference/status to untrusted callers; keep the raw secret within an approved delivery path. Bind the challenge to tenant, canonical subject, purpose, channel, verified destination/version, notice and generation. Define resend, cancel, maximum lifetime, retry and abuse controls without exposing tokens in events or logs.

**Measurable acceptance.**

1. A self-service initiation response cannot disclose the confirmation capability.
2. An approved sandbox delivery journey proves the intended destination receives the challenge and confirmation is bound to that destination and subject.
3. Resend/cancel/expiry/replay and partial delivery failures have documented, tested outcomes.

**Verification.** Actual sandbox recipient journey using the approved delivery owner, with response/event/log secret checks, destination mismatch, resend, expiry, cancelled and repeated challenges; no production sending. See [AT-003](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-003). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-004

**Receipt hashes do not survive the actual pgx timestamp precision round trip**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Evidence integrity |
| Status | CONFIRMED — pgx codec reproduction; live YSQL round trip not run |
| Affected module(s) | MOD-04 — Consent service; MOD-05 — Persistence; MOD-07 — Schema lifecycle |
| Affected users/roles | Privacy reviewers, data subjects, consuming apps |
| Affected APIs | API-04, API-10, API-11 |
| Database implications | consent_receipts |
| Proposed accountable role | Backend/data engineer |
| Effort | 2–4 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004), [AT-004](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-004), [G-004](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-004) |

**Description, business and technical impact.** receiptHash hashes a Go IssuedAt with nanoseconds, while the real pgx timestamptz codec changes .123456789 to .123456. Re-hashing that decoded receipt fails. Existing tests only check a nonempty hash, so they miss this defect.

**Evidence and root-cause boundary.** `internal/service/service.go:108-117`; `internal/store/postgres.go:119-122`; `internal/store/postgres.go:289-297`; `migrations/002_policy_governance.sql:57-70`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Define a versioned canonical receipt serialization and timestamp precision before hashing. Persist the exact signed/hashed payload or normalize every hashed field to a stable representation. Add a verifier that distinguishes old and new receipt formats; never rewrite immutable historic receipts to hide mismatches. Decide how existing non-verifiable receipts will be annotated and reconciled.

**Measurable acceptance.**

1. Hash verification succeeds before storage, after a real YSQL read, and after JSON export/import.
2. Nanosecond input, timezone offsets, map ordering and unicode cases produce deterministic documented bytes.
3. Historic receipt remediation preserves original evidence and produces a reviewable reconciliation report.

**Verification.** Canonical-byte unit vectors and actual pgx/YSQL/JSON round trips, timezone/microsecond/unicode/map cases; verify legacy receipts without rewriting originals. See [AT-004](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-004). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-005

**Notice and receipt records do not bind the accepted notice content and full decision**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Consent evidence |
| Status | CONFIRMED — schema and source gap |
| Affected module(s) | MOD-03 — Domain contracts; MOD-04 — Consent service; MOD-07 — Schema lifecycle |
| Affected users/roles | All service users |
| Affected APIs | API-03, API-04, API-10, API-11 |
| Database implications | consent_privacy_notices, consent_receipts |
| Proposed accountable role | Backend/data engineer + privacy owner |
| Effort | 3–6 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004) |
| Open decisions | [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [AT-005](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-005), [G-005](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-005) |

**Description, business and technical impact.** Notices contain only tenant, version, jurisdiction, published_at and active. Receipts carry a notice identifier and supplied evidence hash, but no immutable notice content/hash/reference, consent status, legal basis, policy version or actor. A nonempty hash is not proof of what text and decision were accepted.

**Evidence and root-cause boundary.** `README.md:16`; `internal/domain/model.go:71-97`; `internal/service/service.go:115-117`; `migrations/002_policy_governance.sql:48-71`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Agree the minimum evidence contract with the privacy owner. Store or immutably reference the notice artifact, digest algorithm, content digest, language and version; bind decision/legal-basis/policy version and collection/confirmation facts into the canonical receipt. Define a separate retirement/supersession record because existing notice rows prohibit updates/deletes. Keep legal sufficiency subject to review.

**Measurable acceptance.**

1. An exported receipt can resolve and verify the exact accepted notice artifact and the decision/legal basis represented.
2. A notice identifier cannot silently refer to altered content.
3. Notice retirement preserves existing receipts and has a tested effect on new capture and evaluation under the approved rule.

**Verification.** Notice artifact digest/immutability/retirement tests; end-to-end capture→export→independent verification using exact policy/decision content. See [AT-005](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-005). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-006

**Evaluation time and current-state ordering do not implement a deterministic consent timeline**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Decision engine |
| Status | CONFIRMED behavior — time semantics and supersession require decision |
| Affected module(s) | MOD-01 — HTTP API; MOD-04 — Consent service; MOD-05 — Persistence |
| Affected users/roles | All service users |
| Affected APIs | API-04, API-05, API-07, API-08 |
| Database implications | consents, consent_suppressions |
| Proposed accountable role | Backend/data lead + product owner |
| Effort | 4–8 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001) |
| Open decisions | [D-03](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-03), [D-12](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-12) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [AT-006](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-006), [G-006](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-006) |

**Description, business and technical impact.** A governed grant was reported allowed one hour before it existed when its notice already existed. Evaluation always loads current suppression/latest updated row, and updated_at ties have no deterministic tiebreaker. Withdrawing an older row can supersede a newer grant; that latter rule is undocumented.

**Evidence and root-cause boundary.** `internal/service/service.go:342-387`; `internal/store/postgres.go:142-143`; `internal/store/memory.go:76-92`; `cmd/server/main.go:37-42`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Choose and publish whether at supports historical evaluation, future scheduling, or only present-time decisions. If historical evaluation is supported, query immutable state/policy/suppression intervals as of at and enforce granted/effective boundaries. Otherwise reject unsupported at values explicitly. Use a transactionally sequenced preference revision, not wall-clock ordering alone. Define older-record withdrawal and equal-timestamp behavior. If full historical evaluation is approved, this ticket must own the shared immutable timeline/policy/suppression foundation before dependent policy enforcement and audit export work; re-estimate that expanded scope through T-032/D-03 rather than introducing a dependency cycle.

**Measurable acceptance.**

1. Evaluation never claims permission before the applicable grant/effective time.
2. Past withdrawal/suppression cases return historical results or an explicit unsupported-time error according to the approved contract.
3. Equal timestamps, concurrent writes and older-record withdrawal produce one documented deterministic result in memory and YSQL.

**Verification.** Table-driven current/historical/future-time cases under the approved contract; equal timestamp, older withdrawal, concurrent state revisions, suppression and policy as-of behavior on Memory and YSQL. See [AT-006](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-006). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-007

**Stronger policy requirements are not re-enforced against existing grants or pending confirmations**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Policy governance |
| Status | CONFIRMED — memory counterexamples; grandfathering undecided |
| Affected module(s) | MOD-04 — Consent service |
| Affected users/roles | All service users |
| Affected APIs | API-01, API-02, API-07, API-08, API-10 |
| Database implications | consent_purposes, consent_lawful_bases, consents |
| Proposed accountable role | Backend policy engineer + privacy owner |
| Effort | 3–6 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006) |
| Open decisions | [D-04](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-04) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [AT-007](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-007), [G-007](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-007) |

**Description, business and technical impact.** Changing a purpose to require affirmative consent left a contract-based grant allowed. Requiring double opt-in left an unconfirmed grant allowed. Confirmation also does not re-run policy validation. The current evaluator checks active/channel/category/notice only; no explicit grandfathering rule exists.

**Evidence and root-cause boundary.** `internal/service/service.go:88-105`; `internal/service/service.go:134-136`; `internal/service/service.go:305-325`; `internal/service/service.go:373-385`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Version policy rules and decide when changes invalidate, grandfather or require recapture. Store the policy/evidence provenance used for each decision. Revalidate all applicable requirements at evaluation and confirmation using a consistent policy snapshot, or implement explicitly approved grandfathering with visible reasons and boundaries. Handle policy updates racing with writes.

**Measurable acceptance.**

1. Policy-transition tests cover affirmative, DOI, evidence, active, category and channel changes.
2. A grant incompatible with current mandatory policy is denied or covered by a recorded approved grandfathering rule.
3. Pending confirmation cannot silently bypass policy changes made after initiation.

**Verification.** Policy-version transition matrix for active/channel/category/affirmative/evidence/DOI requirements and confirmation racing with policy update; explicitly test approved grandfathering bounds. See [AT-007](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-007). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-008

**Upgraded legacy grants bypass all governed-policy checks when notice_version is blank**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Migration safety |
| Status | CONFIRMED — migration and evaluator source; deployed legacy volume unknown |
| Affected module(s) | MOD-04 — Consent service; MOD-07 — Schema lifecycle; MOD-08 — Build, CI and deployment |
| Affected users/roles | All service users |
| Affected APIs | API-07, API-08 |
| Database implications | consents |
| Proposed accountable role | DBA/data engineer + privacy owner |
| Effort | 3–6 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007) |
| Open decisions | [D-04](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-04), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) |
| Release blocker status | RESOLVED / CLOSED (Blocks no longer; AT-008 and G-008 verified) |
| Implementation and verification | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [AT-008](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-008), [G-008](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-008) |

**Description, business and technical impact.** Migration 002 retains legacy rows with blank notice_version, and evaluation previously skipped every registry check for those rows. CI explicitly asserted the blank value remained. Existing production data, if any, was not supplied, so affected record counts are unknown.

**Evidence and root-cause boundary.** `migrations/002_policy_governance.sql:6-13`; `.github/workflows/ci.yml:58-62`; `internal/service/service.go:373-387`. The cited implementation and the evidence status above define the established cause.

**Required work.** Create a read-only legacy classification query and obtain policy-owner decisions for each class. Backfill verified evidence/provenance where possible; otherwise quarantine or require recapture rather than assuming consent. Add an explicit legacy eligibility rule to the evaluator. Rehearse the additive migration against representative synthetic legacy data and retain reconciliation evidence.

**Resolution & Verification (2026-10-21).**
1. **Decision D-04 Approved:** Formally adopted Option C in [ADR-002](../decisions/002_consent_evidence_and_timeline.md) signed by Privacy, Product, and DBA leads.
2. **Evaluator Quarantine Rule:** `internal/service/service.go` enforces category eligibility: blank-notice rows under affirmative consent, marketing, DOI, or evidence-mandatory policies are denied with `allowed = false`, `reason = policy_requirement_unmet`. Non-marketing service/contractual grants remain eligible.
3. **Database Migration & Classification:** Added read-only taxonomy query `scripts/sql/legacy_classification.sql`, additive migration `009_legacy_quarantine_reconciliation.sql` creating `view_consent_legacy_classification` and `consent_legacy_reconciliation` audit table, and operational runbook `docs/operations/04_legacy_consent_reconciliation.md`.
4. **Acceptance & Upgrade Tests:** Passed `TestAT008_*` in `cmd/server/legacy_evaluation_test.go` and `TestPostgresLegacyUpgradeEvaluation` in `cmd/server/postgres_integration_test.go`. Verified multi-stage upgrade from 001 through 009 in `.github/workflows/ci.yml` with zero evidence loss. See [AT-008](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-008).

## F-009

**Outbox lease completion and retry are not fenced to the current worker**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Outbox reliability |
| Status | CONFIRMED — source defect; failover scenario not executed |
| Affected module(s) | MOD-06 — Outbox worker |
| Affected users/roles | Operators and all event consumers |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | consent_outbox_events |
| Proposed accountable role | Worker/backend engineer + SRE |
| Effort | 3–6 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [AT-009](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-009), [G-009](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-009) |

**Description, business and technical impact.** A whole batch of 50 events receives a two-minute lease and is published sequentially. After expiry another worker may claim it; complete/retry only use event ID, so a stale worker can mutate another worker's lease. This does not by itself prove permanent event loss, but HA ownership is not established.

**Evidence and root-cause boundary.** `cmd/outbox-worker/main.go:46-79`; `cmd/outbox-worker/main.go:116-134`; `cmd/outbox-worker/main.go:215-231`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Add an unguessable claim generation/token and require it in completion, retry and renewal updates. Bound publish time per event and batch; renew only owned leases or keep batches within the lease budget. Stop claiming during shutdown and define recovery after publish acknowledgement but before SQL completion. Keep NATS message-ID deduplication and require idempotent consumers.

**Measurable acceptance.**

1. A stale worker cannot complete, clear or reschedule a row owned by a newer claim.
2. Slow publish, killed worker and acknowledgement/commit-gap tests preserve eventual delivery without conflicting state transitions.
3. Shutdown releases or safely expires claims within the approved budget.

**Verification.** Two real workers with forced lease expiry, slow/failed broker, worker kill, ack/SQL gap, shutdown, and stale completion/retry attempts; capture event IDs and ownership transitions. See [AT-009](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-009). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-010

**Events lack the ordering and reconciliation contract needed to keep consumers consistent**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Event contracts |
| Status | CLOSED — resolved in T-010; verified via AT-010 / G-010 |
| Affected module(s) | MOD-03 — Domain contracts; MOD-04 — Consent service; MOD-05 — Persistence; MOD-06 — Outbox worker |
| Affected users/roles | All eight named consuming products |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | consent_outbox_events |
| Proposed accountable role | API/event architect + consumer owners |
| Effort | 4–7 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009) |
| Open decisions | [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07), [D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13) |
| Release blocker status | Closed; gate G-010 verified PASS |
| Implementation and verification | [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [AT-010](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-010), [G-010](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-010) |

**Description, business and technical impact.** schema_version is an event-format version, not a preference revision. Events omit an aggregate state version; registry payloads omit key rules; aggregate_type defaults to consent even for registries/suppressions. Multiple workers and retries can publish out of order. Expiry is evaluated on reads without an expiry event. No consumer reconciliation contract is supplied.

**Evidence and root-cause boundary.** `internal/domain/model.go:141-151`; `internal/service/service.go:49-84`; `internal/service/service.go:147-148`; `internal/service/service.go:197-198`; `internal/service/service.go:232-233`; `internal/store/postgres.go:106-115`; `cmd/outbox-worker/main.go:82-113`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Define versioned event schemas with correct aggregate types, tenant/canonical subject, aggregate revision, effective timestamp and enough information or a resolvable read API to reconcile. Choose consumer behavior for out-of-order events, duplicate IDs, expiry, withdrawal and policy changes. Prefer a current-state re-evaluation before communication where safety requires it; do not promise exactly-once delivery from finite broker dedup alone.

**Measurable acceptance.**

1. All eight event families have versioned examples and contract tests.
2. Grant then withdrawal delivered in reverse order cannot leave a consumer permitted to communicate.
3. Replay beyond the dedup window and expiry/policy-change scenarios converge through the documented consumer contract.

**Verification.** Contract fixtures for eight event families/nine concrete types; reverse grant/withdraw order, duplication beyond dedup window, restart/replay, policy change and expiry reconciliation in a consumer harness. See [AT-010](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-010). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-011

**Pinned toolchain and dependencies fail the current vulnerability gate**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Dependency security |
| Status | CONFIRMED — scanner results; exploitability varies by configuration |
| Affected module(s) | MOD-08 — Build, CI and deployment |
| Affected users/roles | Operators, all API users |
| Affected APIs | OPS-01, OPS-02 |
| Database implications | No mandatory table change established; implementation design may add approved metadata |
| Proposed accountable role | Backend maintainer + security engineer |
| Effort | 2–4 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | No product decision needed to establish the defect; routine design review still applies |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [AT-011](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-011), [G-011](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-011) |

**Description, business and technical impact.** govulncheck v1.7.0 with the 2026-10-08 database reported ten symbol-level advisories: nine Go standard-library advisories and one x/text advisory. Nine additional package/module-only findings are not proven called. Text conversion of the same result exited with findings. This is not a claim that all ten are exploitable in the deployed configuration.

**Evidence and root-cause boundary.** `go.mod:3-21`; `Dockerfile:1-8`; `Dockerfile.worker:1-8`; `.github/workflows/ci.yml:18-28`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Upgrade the Go build/CI/container toolchain consistently to a maintained patched release; official advisories identify Go 1.26.9 or the current 1.27.2 line. Upgrade x/text to at least the indicated fixed version v0.41.0, then resolve the compatible dependency graph. Triage module-only findings and scan runtime/container images. Preserve the scanner database date, platform, SBOM and final image digests.

**Measurable acceptance.**

1. The standard CI vulnerability gate exits successfully or has narrowly documented, time-bound security-owner exceptions with configuration evidence.
2. Both API and worker are built from the same approved patched toolchain and pass regression/race checks.
3. Runtime image and transitive-dependency scans are attached to the immutable release artifacts.

**Verification.** Supported-toolchain build, original plus repaired regression suite, race run, text-mode vulnerability scan and final container/SBOM scan with exact versions and image digests. See [AT-011](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-011). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-012

**Database work cannot follow request cancellation or a bounded transaction budget**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Backend reliability |
| Status | CONFIRMED — source gap; operational impact not load-tested |
| Affected module(s) | MOD-01 — HTTP API; MOD-05 — Persistence |
| Affected users/roles | All service users |
| Affected APIs | API-01, API-02, API-03, API-04, API-05, API-06, API-07, API-08, API-09, API-10, API-11 |
| Database implications | All runtime tables |
| Proposed accountable role | Backend/data engineer + SRE |
| Effort | 3–5 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009) |
| Open decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [AT-012](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-012), [G-012](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-012) |

**Description, business and technical impact.** The Repository interface has no request context; durable methods use context.Background. HTTP timeouts do not cancel those operations. A slow/unavailable database or lock wait can retain work and exhaust fixed pools. Transaction retry behavior for YSQL conflicts is not defined.

**Evidence and root-cause boundary.** `internal/store/memory.go:15-35`; `internal/store/postgres.go:125-143`; `internal/store/postgres.go:238-286`; `internal/store/idempotency.go:59-98`; `cmd/server/main.go:435`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Thread context.Context through handlers, service and repository. Add bounded request, SQL and whole-transaction deadlines with cancellation-aware retries only for approved transient errors. Ensure a retry does not duplicate receipt/audit/outbox effects; record retry limits and backoff. Size API and worker pools against total replica capacity rather than an isolated process.

**Measurable acceptance.**

1. A cancelled/timed-out request promptly cancels pending database work and releases pool resources.
2. Serialization/transaction conflict and lost-response tests either retry safely or return a retryable error without duplicate effects.
3. Load tests at the approved replica count demonstrate bounded connections, latency and queue growth.

**Verification.** Real YSQL request cancellation, lock waits, serialization conflicts, lost responses and approved load; measure released connections and absence of duplicated transactional effects. See [AT-012](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-012). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-013

**Unexpected database errors are returned as 422 with internal error text**

| Field | Audit record |
| --- | --- |
| Severity / category | P2 / API error contract |
| Status | CONFIRMED — source defect |
| Affected module(s) | MOD-01 — HTTP API; MOD-04 — Consent service; MOD-05 — Persistence |
| Affected users/roles | All service users |
| Affected APIs | All business APIs |
| Database implications | No mandatory table change established; implementation design may add approved metadata |
| Proposed accountable role | API engineer |
| Effort | 1–3 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012) |
| Open decisions | No product decision needed to establish the defect; routine design review still applies |
| Release blocker status | Conditional: close/defer explicitly; blocks dependent mandatory gates |
| Implementation and verification | [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013), [AT-013](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-013), [G-013](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-013) |

**Description, business and technical impact.** writeError defaults all unrecognized errors to 422 and exposes err.Error(). Infrastructure failures can look like client validation faults and may expose SQL/schema details. Some policy lookups mask infrastructure errors as validation or policy_unavailable, reducing diagnosis quality.

**Evidence and root-cause boundary.** `cmd/server/main.go:59-85`; `internal/store/postgres.go:44-55`; `internal/service/service.go:88-103`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Introduce typed validation, conflict, authorization, unavailable and internal errors. Return stable public codes/statuses and a correlation ID; log sanitized internal causes with that ID. Preserve safe-deny behavior for unavailable policy while distinguishing operational health. Document which failures clients may retry and keep receipt/token material out of logs.

**Measurable acceptance.**

1. Injected database timeout/outage/internal SQL errors return the approved 5xx/503 shape without internal details.
2. Validation remains actionable 4xx and conflicts/replays keep their documented semantics.
3. Client and operator logs can correlate one failure without storing bearer/confirmation secrets.

**Verification.** Dependency timeout/unavailable/SQL error injection and stable status/public error/correlation assertions; ensure operational logs retain sanitized diagnosis and omit capabilities/evidence. See [AT-013](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-013). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-014

**Unknown JWT key IDs trigger an unbounded JWKS refresh path**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Authentication availability |
| Status | CONFIRMED — five misses caused five local JWKS fetches |
| Affected module(s) | MOD-02 — Authentication |
| Affected users/roles | All service users |
| Affected APIs | All authenticated APIs |
| Database implications | No mandatory table change established; implementation design may add approved metadata |
| Proposed accountable role | Identity/security engineer |
| Effort | 2–4 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-014](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-014), [AT-014](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-014), [G-014](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-014) |

**Description, business and technical impact.** Untrusted RS256 token key IDs force a fresh outbound request even while the cache is fresh. Five distinct misses caused five fetches in the audit. No refresh coalescing, cooldown, negative cache or inbound rate limiter exists; invalid requests can amplify load on Consent and the identity provider.

**Evidence and root-cause boundary.** `internal/auth/auth.go:41-47`; `internal/auth/auth.go:95-116`; `internal/auth/auth.go:151-163`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Add per-JWKS singleflight refresh, a bounded negative cache/cooldown and explicit key rotation behavior. Enforce token/header size and request-rate limits at an owned boundary. Validate HTTPS/trusted JWKS configuration and define bounded last-known-good key behavior during outages without accepting unknown keys.

**Measurable acceptance.**

1. Bursts of distinct unknown kids do not produce an unbounded number of JWKS fetches.
2. Legitimate key rotation works within the approved delay, and unknown keys always fail closed.
3. Concurrent refresh, identity-provider outage, expiry and rate-limit tests meet documented budgets.

**Verification.** Local mock JWKS concurrent unknown-kid flood, slow/unavailable/oversized responses and real staging key rotation; demonstrate bounded refresh without accepting invalid signatures. See [AT-014](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-014). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-015

**Readiness can report healthy while authentication or required schema is unusable**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Operational readiness |
| Status | CONFIRMED — source behavior; cluster not exercised |
| Affected module(s) | MOD-01 — HTTP API; MOD-04 — Consent service; MOD-05 — Persistence |
| Affected users/roles | All service users |
| Affected APIs | OPS-01, OPS-02 |
| Database implications | No mandatory table change established; implementation design may add approved metadata |
| Proposed accountable role | Backend + SRE |
| Effort | 1–3 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Open decisions | [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015), [AT-015](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-015), [G-015](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-015) |

**Description, business and technical impact.** Readiness checks only database Ping. Missing/invalid authentication configuration or a reachable database missing migration tables still yields ready. Health endpoints accept every method, including CONNECT, which matters to a scanner-identified standard-library precondition.

**Evidence and root-cause boundary.** `cmd/server/main.go:386-398`; `cmd/server/main.go:413-435`; `internal/service/service.go:390`; `internal/store/postgres.go:38-41`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Validate required auth mode, issuer/audience/JWKS transport, production store and configuration at startup. Verify a compatible recorded schema version before readiness without making every probe depend on a live IdP call. Restrict probe methods and define API/worker startup/readiness/liveness meanings. Fail deployment on unusable configuration.

**Measurable acceptance.**

1. Missing auth settings or required migrations keep the deployment unready or fail startup with a sanitized diagnostic.
2. Liveness remains independent of routine dependency outages; readiness behavior is specified and tested.
3. Health endpoints reject unsupported methods and worker health exposes meaningful progress/lag.

**Verification.** Boot with absent/invalid auth, unavailable JWKS as defined, missing001/002/003 schema and unavailable DB; verify startup/readiness policy and rolling-deploy recovery. See [AT-015](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-015). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-016

**Durable API, transaction and multi-node Yugabyte behavior remain unverified**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Release evidence |
| Status | NOT VERIFIED — required infrastructure unavailable in audit |
| Affected module(s) | MOD-01 — HTTP API; MOD-05 — Persistence; MOD-06 — Outbox worker |
| Affected users/roles | All service users |
| Affected APIs | API-01, API-02, API-03, API-04, API-05, API-06, API-07, API-08, API-09, API-10, API-11 |
| Database implications | All runtime tables |
| Proposed accountable role | QA/backend + YSQL DBA |
| Effort | 4–8 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002), [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Open decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [AT-016](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-016), [G-016](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-016) |

**Description, business and technical impact.** All three external-integration tests skipped in the audit. The server database test calls Service directly, not authenticated HTTP. No current CI run artifact, production-equivalent YSQL universe, failover evidence or real provider credentials was supplied. Passing memory tests cannot establish durable production behavior.

**Evidence and root-cause boundary.** `README.md:45-51`; `cmd/server/postgres_integration_test.go:17-83`; `internal/store/idempotency_integration_test.go:14-73`; `cmd/outbox-worker/postgres_integration_test.go:15-120`. The verified gap is the absence of the specified contract, ownership or execution evidence; this record does not assert an unseen integration or environment has failed.

**Required work.** Provision an explicitly isolated target-version YSQL environment and NATS test account. Run every database test plus authenticated HTTP-to-YSQL workflows, transaction fault injection, restarts, concurrent writes and receipt round trips. Separately run multi-node placement/failure tests on the approved production topology. Record exact versions, sanitized config, timestamps, test logs and database assertions.

**Measurable acceptance.**

1. All three existing external tests execute rather than skip, and new critical HTTP/YSQL tests pass.
2. A fault between state/receipt/audit/outbox writes leaves either all committed or none.
3. Restart, node loss and transaction contention preserve the approved current-state and delivery invariants.

**Verification.** All11 business routes over HTTP with production-compatible JWTs and real target YSQL, receipt re-read, idempotency, competing transactions, node restart/leader movement and selected broker integration. See [AT-016](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-016). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-017

**CI omits durable idempotency coverage and migration 003 upgrade verification**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / CI and migration testing |
| Status | CONFIRMED — workflow and fixture omissions |
| Affected module(s) | MOD-05 — Persistence; MOD-07 — Schema lifecycle; MOD-08 — Build, CI and deployment |
| Affected users/roles | All service users |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | consent_idempotency, consents, consent_receipts, consent_outbox_events |
| Proposed accountable role | CI/backend engineer |
| Effort | 2–4 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | No product decision needed to establish the defect; routine design review still applies |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017), [AT-017](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-017), [G-017](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-017) |

**Description, business and technical impact.** The general test job has no database. The YSQL job selects only server and worker tests, leaving the store idempotency integration test skipped everywhere. The upgrade path stops at002. That test also uses globally fixed record IDs despite a unique tenant, making reruns unsafe against a reused test database.

**Evidence and root-cause boundary.** `.github/workflows/ci.yml:20-28`; `.github/workflows/ci.yml:51-72`; `internal/store/idempotency_integration_test.go:14-46`; `migrations/003_governed_idempotency.sql:1-11`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Run every integration package against a deliberately isolated YSQL database using one consistent test-DSN contract. Fail the required job when prerequisites are missing rather than silently skipping. Upgrade001 through every current migration including003; exercise persisted state and idempotency after upgrade. Use run-unique global fixture IDs and preserve machine-readable test artifacts.

**Measurable acceptance.**

1. CI evidence shows the store concurrency test actually executed on YSQL.
2. Fresh install and001-to-latest upgrade include003 and verify idempotent replay/conflict after upgrade.
3. The suite can run twice in its designated isolated environment without fixture-ID collisions.

**Verification.** CI with mandatory isolated DB/NATS prerequisites, all three durable tests run twice in retained test databases, plus 001 and002 upgrade paths through003; skipped mandatory tests fail the job. See [AT-017](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-017). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-018

**Release packaging and deployment are not reproducible from the supplied manifests**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Deployment |
| Status | CONFIRMED — configuration gaps; actual hosting unknown |
| Affected module(s) | MOD-08 — Build, CI and deployment |
| Affected users/roles | Release engineer and platform operator |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | No mandatory table change established; implementation design may add approved metadata |
| Proposed accountable role | Platform/release engineer |
| Effort | 3–6 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Open decisions | [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018), [AT-018](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-018), [G-018](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-018) |

**Description, business and technical impact.** The API uses :latest and the worker :latest-worker without explicit pull policy/digests. CI builds test images but does not publish the referenced release images. Required ConfigMap, Secrets, database, NATS and ingress ownership are external and unspecified. The manifests are a baseline, not a complete release procedure.

**Evidence and root-cause boundary.** `deploy/k8s.yaml:39-64`; `deploy/outbox-worker.yaml:34-56`; `.github/workflows/ci.yml:27-30`; `Dockerfile:1-10`; `Dockerfile.worker:1-10`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Define environment overlays or equivalent deployment inputs, immutable image digests, release provenance and an owned artifact publication/promotion flow. Supply secure references/templates for required configuration and document external infrastructure dependencies. Rehearse rollout and image rollback with additive schema compatibility. Do not embed real secret values in the repository.

**Measurable acceptance.**

1. A new authorized engineer can deploy the same exact API/worker digests in staging using the documented inputs.
2. Missing configuration is detected before rollout; API and worker run the intended release after rolling update.
3. Rollback uses a known prior digest and passes current-schema compatibility checks.

**Verification.** Build both immutable images from recorded commit; validate rendered deployment inputs, roll out in staging, record actual digests/pull behavior, then roll back to previous compatible image. See [AT-018](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-018). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-019

**Transport security and least-privilege service identities are not established**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Infrastructure security |
| Status | RESOLVED / VERIFIED (T-019) |
| Affected module(s) | MOD-01 — HTTP API; MOD-02 — Authentication; MOD-06 — Outbox worker; MOD-08 — Build, CI and deployment |
| Affected users/roles | Security/platform operators |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | Least-privilege roles (`consent_api_role`, `consent_worker_role`, `consent_migration_role`) added via `migrations/008_least_privilege_roles.sql` |
| Proposed accountable role | Platform/security + DBA + identity owner |
| Effort | 3–6 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018) |
| Open decisions | [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| Release blocker status | Release unblocked for T-019 scope |
| Implementation and verification | [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [AT-019](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-019), [G-019](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-019) |

**Description, business and technical impact.** JWKS accepts configurable HTTP URLs; API serves plaintext behind an unspecified boundary; NATS credentials do not themselves require TLS. The manifest uses nats:// and no CA setting. Database TLS/roles and broker tenant ACLs are not supplied. This is not evidence that actual production traffic is plaintext.

**Evidence and root-cause boundary.** `internal/auth/auth.go:41-47`; `cmd/server/main.go:435-439`; `cmd/outbox-worker/main.go:185-194`; `deploy/outbox-worker.yaml:38-56`; `README.md:47-51`. The verified gap is the absence of the specified contract, ownership or execution evidence; this record does not assert an unseen integration or environment has failed.

**Required work.** Specify and verify every trust boundary: ingress/API, API/JWKS, API-worker/YSQL and worker/NATS. Require authenticated TLS or an approved equivalent channel; verify hostname/CA behavior. Separate migration, API and worker database roles; restrict broker subjects/stream administration and tenant consumer access. Reject development modes in production deployment policy.

**Measurable acceptance.**

1. Staging handshakes prove certificate/hostname validation and fail on a wrong CA or downgraded endpoint.
2. The API cannot modify outbox delivery bookkeeping/DDL and the worker cannot mutate consent/receipts except as explicitly needed by its role.
3. Tenant consumer credentials cannot subscribe to another tenant's data; header tenant metadata alone is not treated as an ACL.

**Verification.** Successful trusted TLS and negative invalid CA/hostname/plaintext attempts on every required hop; least-privilege SQL/NATS/cluster authorization and credentials/certificate rotation drill. See [AT-019](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-019). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-020

**Existing NATS streams are accepted without checking durability, subjects or retention**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Broker configuration |
| Status | CONFIRMED — source gap; broker configuration unknown |
| Affected module(s) | MOD-06 — Outbox worker; MOD-08 — Build, CI and deployment |
| Affected users/roles | Messaging/platform operators |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | No mandatory table change established; implementation design may add approved metadata |
| Proposed accountable role | Messaging/SRE owner |
| Effort | 2–4 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019) |
| Open decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020), [AT-020](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-020), [G-020](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-020) |

**Description, business and technical impact.** ensureStream returns immediately for any existing stream name. It does not verify subject coverage, storage, replicas, dedup window or retention. Creation has finite10-minute dedup but no explicit size/age/message limits. A stale/misconfigured stream can contradict the documented three-replica baseline.

**Evidence and root-cause boundary.** `cmd/outbox-worker/main.go:137-146`; `cmd/outbox-worker/main.go:155-169`; `deploy/outbox-worker.yaml:45-50`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Move stream ownership to a controlled provisioning role or validate its configuration against the approved contract at startup. Define retention/capacity/discard policy, dedup horizon, subject permissions and safe change procedures. Alert on configuration drift and publish rejections; avoid silently altering shared broker resources from an application worker.

**Measurable acceptance.**

1. An existing mismatched stream produces an actionable blocked-startup/rollout result.
2. Approved retention/replica/storage/subject settings are verified in staging and attached to release evidence.
3. Capacity exhaustion and retry beyond dedup horizon follow the documented at-least-once consumer contract.

**Verification.** Start against pre-existing stream with wrong subject/replicas/retention/dedup limits, plus broker-node failure and restart; prove explicit drift behavior and retained acknowledged messages. See [AT-020](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-020). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-021

**Operators cannot reliably detect stalled consent propagation or diagnose request failures**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Observability |
| Status | RESOLVED — implemented and verified in T-021 |
| Affected module(s) | MOD-01 — HTTP API; MOD-06 — Outbox worker; MOD-08 — Build, CI and deployment |
| Affected users/roles | SRE, support, privacy operations |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | Added outbox quarantine schema (quarantined, quarantined_at, quarantine_reason) |
| Proposed accountable role | SRE + backend engineer |
| Effort | 3–5 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015) |
| Open decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021), [AT-021](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-021), [G-021](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-021) |

**Description, business and technical impact.** Logging is basic process/error text. There are no request traces, API latency/error metrics, worker progress probes, outbox-age alerts or dashboards. Worker claim/publish failure loops can persist without an owned operational signal; retry attempts never enter a bounded quarantine/review workflow.

**Evidence and root-cause boundary.** `cmd/outbox-worker/main.go:206-231`; `cmd/server/main.go:59-98`; `deploy/outbox-worker.yaml:34-72`; `deploy/k8s.yaml:65-81`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Instrument request counts/latency, decision reasons, auth failures, database pool saturation, JWKS refresh and outbox lag/attempts/publish acknowledgements. Add worker progress/health signals, dashboards, alert routing and runbooks with privacy-safe labels. Define poison-event handling and manual replay authorization while preserving durable evidence.

**Measurable acceptance.**

1. Simulated database/broker/IdP failures and a poison event trigger the expected operator signal with a traceable cause.
2. Dashboard distinguishes API health from consumer propagation lag.
3. Operators can quarantine/replay an event safely without exposing tokens or recording unbounded subject identifiers in metrics.

**Verification.** Trigger request failure, DB saturation, broker outage, oldest-event growth, poison event and stopped worker; prove alert delivery/ownership and correlation-based diagnosis within approved targets. See [AT-021](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-021). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-022

**Backup, restore, retention and recovery procedures are not production-evidenced**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Data operations |
| Status | NOT VERIFIED — required operational policy absent |
| Affected module(s) | MOD-07 — Schema lifecycle |
| Affected users/roles | DBA/SRE/privacy owner |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | All durable state and broker events |
| Proposed accountable role | DBA/SRE + privacy owner |
| Effort | 3–7 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020) |
| Open decisions | [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-022](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-022), [AT-022](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-022), [G-022](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-022) |

**Description, business and technical impact.** README delegates TLS, placement, backup/PITR and recovery to the target YSQL universe. No actual backup schedule, successful restore, recovery objectives or retention policy is present. ROLLBACK refers to a PostgreSQL backup despite YSQL being the intended production system; compatibility of the prior image is asserted but untested here.

**Evidence and root-cause boundary.** `README.md:51`; `migrations/ROLLBACK.md:3-5`; `migrations/002_policy_governance.sql:91-110`. The verified gap is the absence of the specified contract, ownership or execution evidence; this record does not assert an unseen integration or environment has failed.

**Required work.** Approve retention/RPO/RTO and assign owners. Produce a target-YSQL backup/PITR and restore runbook, with encrypted access-controlled evidence storage and tests. Reconcile restored consent/receipt/audit/idempotency/outbox state with already delivered broker events. Define lawful evidence retention/deletion procedures that do not bypass immutable triggers ad hoc.

**Measurable acceptance.**

1. A restore into an isolated target-version universe meets the approved RPO/RTO and verifies representative receipts, decisions and outbox replay.
2. A prior-image rollback and forward-correction drill retain evidence and do not resurrect withdrawn permission.
3. Retention/deletion and recovery responsibilities are approved and documented; no legal-compliance claim rests on schema alone.

**Verification.** Isolated target-YSQL backup/PITR restore, receipt/hash and preference verification, idempotency/outbox/broker reconciliation and prior-image rollback/forward-correction; measure agreed RPO/RTO. See [AT-022](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-022). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-023

**Named NVUTO product integrations have no end-to-end verification in this repository**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Platform integration |
| Status | NOT VERIFIED — consumer repositories and evidence not supplied |
| Affected module(s) | MOD-01 — HTTP API; MOD-06 — Outbox worker |
| Affected users/roles | All launch product teams |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | No mandatory table change established; implementation design may add approved metadata |
| Proposed accountable role | Integration lead + each approved consumer owner |
| Effort | 4–10 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020), [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030) |
| Open decisions | [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07), [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023), [AT-023](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-023), [G-023](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-023) |

**Description, business and technical impact.** Engage, Service, Contact Center, Customer Portal, CRM, Conversations, AI and Integration Hub are named consumers only. No actual client, frontend, subscription, contact delivery or fail-closed communication test is supplied. Building this service does not prove those products respect consent.

**Evidence and root-cause boundary.** `README.md:9-11`; `cmd/server/main.go:399-409`; `cmd/outbox-worker/main.go:102-113`. The verified gap is the absence of the specified contract, ownership or execution evidence; this record does not assert an unseen integration or environment has failed.

**Required work.** Approve the launch consumer set and assign an owner to each integration. Publish identity, HTTP and event contracts, map canonical Customer Graph IDs, and run each launch workflow from actual user/application action through YSQL and messaging to the consumer outcome. Define timeout/error handling and the pre-send consent check. Non-launch consumers remain explicitly deferred.

**Measurable acceptance.**

1. Each approved launch consumer has a reproducible integration/UAT record using real staging wiring.
2. No-consent, denial, withdrawal, suppression, expiry and service-unavailable cases prevent disallowed communication.
3. A subject change is reflected after page reload and consumer restart, within the approved propagation budget.

**Verification.** Per approved consumer, actual staging user/app action→JWT→API→YSQL→event→consumer outcome; denied/missing/withdrawn/suppressed/expired/unavailable cases and restart/page-reload persistence. See [AT-023](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-023). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-024

**Preference discovery, registry reads and a machine-readable client contract are absent**

| Field | Audit record |
| --- | --- |
| Severity / category | P2 / API completeness |
| Status | CONFIRMED absence — new endpoint scope requires decision |
| Affected module(s) | MOD-01 — HTTP API; MOD-03 — Domain contracts |
| Affected users/roles | Portal/admin frontend and client engineers |
| Affected APIs | All business APIs; proposed read APIs require approval |
| Database implications | No mandatory table change established; implementation design may add approved metadata |
| Proposed accountable role | API architect + portal/admin owner |
| Effort | 3–6 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07) |
| Release blocker status | Conditional: close/defer explicitly; blocks dependent mandatory gates |
| Implementation and verification | [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024), [AT-024](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-024), [G-024](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-024) |

**Description, business and technical impact.** The service exposes writes, evaluation and receipt-by-ID, but no subject preference list, consent-by-ID GET, registry list, suppression list or audit export route. External portals may rely on another owned projection; none is provided. OpenAPI/SDK examples and errors/pagination contracts are also absent.

**Evidence and root-cause boundary.** `README.md:5-11`; `cmd/server/main.go:386-410`; `internal/domain/model.go:15-125`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Choose the authoritative read owner for the self-service/admin experiences. If Consent owns it, specify tenant/subject-authorized read APIs with bounded pagination and exact frontend DTOs; otherwise document and prove the external projection. Publish an OpenAPI contract for all11 existing operations and approved additions, including error/retry/idempotency and CORS/gateway requirements.

**Measurable acceptance.**

1. A portal can load the approved current preferences and necessary policy/notice data through a documented authorized source.
2. Contract tests validate request/response models for every launch endpoint, including negative authorization cases.
3. Pagination/filtering and origin/session behavior are explicitly specified where needed; absent frontend is not misrepresented as an implemented screen.

**Verification.** Contract conformance for existing11 operations and approved reads/projection; tenant/subject authorization, current-state initial load, bounded pagination/filtering and client error/retry handling. See [AT-024](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-024). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-025

**Idempotency replay semantics change with policy and current consent state**

| Field | Audit record |
| --- | --- |
| Severity / category | P2 / Idempotency contract |
| Status | CONFIRMED — source behavior; intended replay semantics undecided |
| Affected module(s) | MOD-04 — Consent service; MOD-05 — Persistence |
| Affected users/roles | All service users |
| Affected APIs | API-04 |
| Database implications | consent_idempotency, consents |
| Proposed accountable role | Backend/API engineer |
| Effort | 2–4 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Open decisions | [D-11](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-11) |
| Release blocker status | Conditional: close/defer explicitly; blocks dependent mandatory gates |
| Implementation and verification | [T-025](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-025), [AT-025](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-025), [G-025](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-025) |

**Description, business and technical impact.** Policy is revalidated before looking up the key, so a legitimate replay can fail after a policy change. The ledger returns the current consent row, not the original response, so a withdrawn record can replace the original granted response. The fingerprint includes actor and uses timestamp representations; retention is undefined.

**Evidence and root-cause boundary.** `internal/service/idempotency.go:17-30`; `internal/service/idempotency.go:55-81`; `internal/store/idempotency.go:27-41`; `internal/store/idempotency.go:67-83`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Specify whether a key guarantees original response replay or only one resource creation, how actor/token rotation affects it, and the retention/security namespace. Resolve authorized existing-key replay before current-policy creation checks where the approved contract requires it. Canonicalize semantic times and store a response snapshot if exact replay is promised; do not allow replay to bypass current subject authorization.

**Measurable acceptance.**

1. Same-key replay after withdrawal, policy change, actor rotation and time-offset normalization matches the published contract.
2. Changed semantic payload returns409 and concurrent identical writes create exactly one consent/receipt/audit/outbox set on YSQL.
3. Retention, cleanup and consumer use of evaluation instead of stale create-response status are documented.

**Verification.** Identical-key replay after policy/actor/token/time representation/resource status changes, semantic conflict409, simultaneous writes and retention boundary on real YSQL. See [AT-025](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-025). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-026

**Memory-store composite keys collide across tenants and diverge from durable behavior**

| Field | Audit record |
| --- | --- |
| Severity / category | P2 / Development-store correctness |
| Status | CONFIRMED — tenant collision reproduced; production disabled by default |
| Affected module(s) | MOD-01 — HTTP API; MOD-05 — Persistence |
| Affected users/roles | Developers and local test tenants |
| Affected APIs | Development-mode APIs |
| Database implications | No mandatory table change established; implementation design may add approved metadata |
| Proposed accountable role | Backend maintainer |
| Effort | 1–3 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | No product decision needed to establish the defect; routine design review still applies |
| Release blocker status | Conditional: close/defer explicitly; blocks dependent mandatory gates |
| Implementation and verification | [T-026](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-026), [AT-026](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-026), [G-026](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-026) |

**Description, business and technical impact.** Unescaped colon concatenation permits a lookup in tenant audit-parent to return a record from tenant audit-parent:child when the ID is prefixed accordingly. This was reproduced through Service. Memory suppression updates also preserve the old ID but do not repair audit/outbox IDs, and memory events skip durable normalization.

**Evidence and root-cause boundary.** `internal/store/memory.go:60-74`; `internal/store/memory.go:145-168`; `internal/store/memory.go:218-235`; `internal/store/idempotency.go:20-41`; `cmd/server/main.go:425-430`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Use typed composite keys/nested maps and defensive tenant checks. Make memory suppression/event/immutability semantics match the public repository contract, clone mutable data where required, and keep the idempotency ledger owned by each Memory instance. Retain the explicit development opt-in and enforce its prohibition in approved production configuration.

**Measurable acceptance.**

1. Delimiter-rich tenant/subject/code/ID combinations cannot cross boundaries.
2. Memory and YSQL contract tests agree on suppression IDs, event metadata, concurrency and immutable values.
3. Production configuration tests reject ALLOW_MEMORY_STORE.

**Verification.** Typed-key collision property cases and shared adapter contract suite, suppression event IDs, isolation across Memory instances and mutable-map aliasing; deployment ban on Memory. See [AT-026](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-026). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-027

**Input normalization and validation leave ambiguous or surprising API behavior**

| Field | Audit record |
| --- | --- |
| Severity / category | P2 / API quality |
| Status | CONFIRMED — trailing JSON reproduced; other cases source-traced |
| Affected module(s) | MOD-01 — HTTP API; MOD-04 — Consent service |
| Affected users/roles | All service users |
| Affected APIs | All JSON-consuming business APIs |
| Database implications | No mandatory table change established; implementation design may add approved metadata |
| Proposed accountable role | API/backend engineer |
| Effort | 2–4 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | [D-02](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-02), [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05) |
| Release blocker status | Conditional: close/defer explicitly; blocks dependent mandatory gates |
| Implementation and verification | [T-027](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-027), [AT-027](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-027), [G-027](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-027) |

**Description, business and technical impact.** A valid JSON object followed by a second object is accepted. Boolean active defaults false on API-decoded structs despite SQL defaults true; evidence hash format is not checked; consent expiry may precede creation; challenge lifetime and many identifiers have no semantic maximum. Denial capture also requires the same current policy/evidence checks as grant.

**Evidence and root-cause boundary.** `cmd/server/main.go:78-85`; `internal/service/service.go:79-103`; `internal/service/service.go:131-139`; `internal/service/service.go:202-234`; `internal/service/service.go:264-265`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Define an explicit input contract. Require exactly one JSON document; implement approved content-type/body-size behavior and field bounds. Distinguish omitted booleans from explicit false where needed. Validate evidence format and timestamp relationships. Decide how a subject can deny/withdraw when policy is unavailable without accidentally authorizing consent.

**Measurable acceptance.**

1. Trailing JSON and malformed/oversized/wrong-type inputs return stable4xx without mutation.
2. Omitted active, invalid expiry and invalid evidence produce documented outcomes rather than accidental defaults.
3. Deny/withdraw error paths allow the approved opt-out journey under policy-change/unavailable conditions.

**Verification.** Exactly one JSON document, body limits/types/content type, omitted booleans, evidence/time bounds, normalization and safe opt-out failure journeys; assert no mutation on rejected requests. See [AT-027](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-027). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-028

**Database constraints do not enforce several tenant and state relationships**

| Field | Audit record |
| --- | --- |
| Severity / category | P2 / Schema integrity |
| Status | CONFIRMED — schema gap; no cross-tenant SQL exploit demonstrated |
| Affected module(s) | MOD-07 — Schema lifecycle |
| Affected users/roles | All service users |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | consents, consent_receipts, consent_idempotency, consent_suppressions |
| Proposed accountable role | DBA/backend engineer |
| Effort | 2–5 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Open decisions | No product decision needed to establish the defect; routine design review still applies |
| Release blocker status | Conditional: close/defer explicitly; blocks dependent mandatory gates |
| Implementation and verification | [T-028](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-028), [AT-028](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-028), [G-028](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-028) |

**Description, business and technical impact.** Only receipt.consent_id has a foreign key, and it does not include tenant. Receipt_id on consent and consent_id on idempotency have no FK; registry references are logical. Suppression field shapes, evidence JSON structure and temporal/state relations are mostly enforced only in service code. SQL queries do use tenant parameters.

**Evidence and root-cause boundary.** `migrations/001_init.sql:1-45`; `migrations/002_policy_governance.sql:24-89`; `migrations/003_governed_idempotency.sql:1-11`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Define invariant ownership and add appropriate composite tenant keys/FKs/checks after read-only data validation. Avoid inappropriate FKs for intentionally external subjects or polymorphic audit targets. Reconcile existing violations additively, test insertion ordering/deferrability on YSQL, and document application-vs-database guarantees.

**Measurable acceptance.**

1. Representative cross-tenant receipt/idempotency reference attempts and invalid state shapes are rejected by the approved layer.
2. Existing-data validation reports zero unexplained violations before constraints are enforced.
3. Fresh install and upgrade preserve performance and transactional write order under YSQL.

**Verification.** Read-only violation census, representative direct invalid inserts, composite tenant references, constrained transaction ordering and post-upgrade write/performance tests on YSQL. See [AT-028](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-028). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-029

**Immutable audit rows cannot reconstruct policy and suppression history**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Auditability |
| Status | CONFIRMED — schema and mutation payload gap |
| Affected module(s) | MOD-03 — Domain contracts; MOD-04 — Consent service; MOD-05 — Persistence |
| Affected users/roles | Privacy administrator, auditor, authorized support |
| Affected APIs | API-01, API-02, API-06, Dependent evaluation and authorized history/export workflows |
| Database implications | consent_audit_events, consent_purposes, consent_lawful_bases, consent_suppressions |
| Proposed accountable role | Data/backend engineer + privacy owner |
| Effort | 3–5 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007) |
| Open decisions | [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-029](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-029), [AT-029](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-029), [G-029](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-029) |

**Description, business and technical impact.** Audit rows contain ID, tenant, actor, action, consent_id and time only. Mutable registries and suppression rows overwrite values; audit rows do not retain before/after rules or resource type/revision. A row-level immutability trigger prevents changes but does not make missing history reconstructable.

**Evidence and root-cause boundary.** `internal/domain/model.go:132-139`; `internal/service/service.go:58-59`; `internal/service/service.go:71-72`; `internal/service/service.go:232-233`; `internal/store/postgres.go:154-160`; `internal/store/postgres.go:307-315`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Add typed resource identifiers, immutable policy/suppression versions or complete before/after change records, correlation/source and reason where approved. Provide an authorized export/query path or owned external audit projection. Apply least privilege to the append-only tables and define redaction/retention with the privacy owner.

**Measurable acceptance.**

1. An auditor can reconstruct the exact policy and suppression states used for a historical consent decision.
2. Updates retain actor, resource type, revision, time and approved change reason without overwriting historical values.
3. Audit exports are tenant/role-scoped and the normal app role cannot alter history.

**Verification.** Historical policy/suppression reconstruction, role/tenant-scoped investigator export and app-role UPDATE/DELETE denial; verify all relevant before/after and revision fields. See [AT-029](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-029). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-030

**Mixed self-service and reader roles can authorize tenant-wide writes**

| Field | Audit record |
| --- | --- |
| Severity / category | P1 / Authorization and permission policy |
| Status | CONFIRMED — source-level role composition; live issued combinations NOT VERIFIED |
| Affected module(s) | MOD-01 — HTTP API; MOD-02 — Authentication |
| Affected users/roles | Identity owner, privacy owner, all seven JWT roles |
| Affected APIs | All authenticated APIs |
| Database implications | consent_suppressions |
| Proposed accountable role | Identity/security + privacy owner |
| Effort | 2–4 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| Release blocker status | Blocks current affected launch path |
| Implementation and verification | [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030), [AT-030](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-030), [G-030](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-030) |

**Description, business and technical impact.** The developer role is accepted in production JWTs if issued, service is read-only, and privileged roles override a self_service restriction on a mixed-role token. Specifically, self_service plus service or consent_reader admits writes through self_service and then bypasses the subject restriction through the read role, allowing tenant-wide subject writes; this is source-traced, not dynamically probed. A self_service user may set active=false on their own global/channel suppression regardless of who created it. No finer policy distinguishes user preferences from administrative/legal suppression.

**Evidence and root-cause boundary.** `cmd/server/main.go:109-118`; `cmd/server/main.go:241-258`; `internal/auth/auth.go:27-31`; `internal/auth/auth.go:180-199`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Authorize the requested action and subject scope together, preventing a tenant-wide reader role from expanding a self-service writer into tenant-wide writes. Until fixed, any proposed deployment exclusion must be enforced and tested in the issuer/configuration, not assumed. Approve exact Authentik role/claim mappings and suppression authority. Decide whether to separate user preference opt-outs from administrative/legal blocks and how mixed roles behave. Add provenance/owner fields only where that approved rule requires them; test every role and mixed-role combination against every operation and resource boundary, including self_service plus service or consent_reader. Keep identity account signup/recovery external as README states.

**Measurable acceptance.**

1. Mixed self_service plus service or consent_reader tokens cannot write another subject solely by combining separate role checks; any broader writer authority must be explicitly granted and tested.
2. A signed role-permission matrix matches production IdP mappings and backend tests for all seven roles.
3. Self-service cannot clear a protected administrative suppression if the approved policy distinguishes that class.
4. Production deployment disallows development authentication/roles according to the approved identity contract.

**Verification.** All seven roles and mixed-role combinations against every endpoint, production claim mapping, canonical subjects, protected suppression clearing under the approved policy and forbidden development auth. See [AT-030](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-030). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-031

**Legacy Record and Upsert helpers preserve a non-atomic governance bypass path**

| Field | Audit record |
| --- | --- |
| Severity / category | P2 / Maintainability |
| Status | CONFIRMED — internal-only path, not an active HTTP endpoint |
| Affected module(s) | MOD-04 — Consent service; MOD-05 — Persistence |
| Affected users/roles | All service users |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | consents, consent_audit_events, consent_outbox_events |
| Proposed accountable role | Backend maintainer |
| Effort | 1–2 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Open decisions | No product decision needed to establish the defect; routine design review still applies |
| Release blocker status | Conditional: close/defer explicitly; blocks dependent mandatory gates |
| Implementation and verification | [T-031](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-031), [AT-031](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-031), [G-031](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-031) |

**Description, business and technical impact.** The legacy Record helper writes consent then independently appends audit/outbox while ignoring their errors. It bypasses governed policy/receipt rules. Current HTTP /v1/consents uses RecordGoverned, so this is not an identified public-route transaction bypass; future reuse and legacy tests can hide risk.

**Evidence and root-cause boundary.** `internal/service/service.go:152-178`; `internal/store/postgres.go:125-135`; `internal/store/memory.go:15-35`; `internal/service/service_test.go:10-90`. The cited implementation and the evidence status above define the established cause. Where runtime reproduction is absent, concurrency, production impact and exploitability remain unverified beyond the source trace.

**Required work.** Remove, restrict or explicitly deprecate the legacy helper and unsafe Upsert surface after confirming any external Go consumers. Route required compatibility behavior through one atomic governed implementation or an explicitly isolated migration adapter. Align tests with production-reachable paths; retain a clear legacy-data migration contract.

**Measurable acceptance.**

1. No production entry point can create consent while silently losing audit/outbox/receipt effects.
2. Compatibility consumers are inventoried or explicitly absent before removing exported behavior.
3. Production behavior tests exercise the governed path and legacy migration tests remain separate.

**Verification.** Call-site inventory, failure injection for every production write entry, governed-path tests and explicitly separate legacy migration tests; demonstrate no ignored audit/outbox failure. See [AT-031](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-031). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## F-032

**Approved launch requirements, capacity and regional policy decisions are not supplied**

| Field | Audit record |
| --- | --- |
| Severity / category | P2 / Product handover |
| Status | DECISION REQUIRED — only README/rollback prose exists |
| Affected module(s) | Cross-cutting product/operational requirements across MOD-01–MOD-08 |
| Affected users/roles | Product, architecture, privacy and release owners |
| Affected APIs | No direct route; affects runtime, delivery, infrastructure or release evidence |
| Database implications | No mandatory table change established; implementation design may add approved metadata |
| Proposed accountable role | Product/architecture + release owner |
| Effort | 2–4 senior-engineer person-days, provisional; see ticket assumptions |
| Dependencies | None; may start independently within an isolated development environment. |
| Open decisions | [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), [D-03](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-03), [D-04](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-04), [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07), [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10), [D-11](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-11), [D-12](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-12), [D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) |
| Release blocker status | Conditional: close/defer explicitly; blocks dependent mandatory gates |
| Implementation and verification | [T-032](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-032), [AT-032](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-032), [G-032](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-032) |

**Description, business and technical impact.** The attached audit brief expects detailed PRDs/TRDs/flows; the32-file archive contains none. Revenue model, operating regions, exact personas, data scope, capacity/SLOs and acceptance ownership are undocumented. NG appears in tests; it does not establish applicable legal obligations. There is no Git history/commit in the ZIP.

**Evidence and root-cause boundary.** `README.md:1-51`; `migrations/ROLLBACK.md:1-5`. The verified gap is the absence of the specified contract, ownership or execution evidence; this record does not assert an unseen integration or environment has failed.

**Required work.** Review this evidence-based reconstruction with product/privacy/architecture owners, approve or amend the scope and decision register, recover any missing upstream specifications and record the exact source commit. Resolve requirements before expanding frontend, authentication, payment or geographic functionality. Define release owners and measurable nonfunctional targets with explicit approvals.

**Measurable acceptance.**

1. Every launch-critical requirement has an owner, source, acceptance case and implementation/gap link.
2. All release-affecting decisions are recorded and approved; deferred scope is explicit.
3. The release evidence identifies a source commit, approved configuration, SLO/capacity/RPO/RTO and regional privacy review owner.

**Verification.** Requirements/decision review with accountable owners, exact upstream source revision, scope exclusions and measurable SLO/region/recovery acceptance references; this is an evidence review, not a unit test. See [AT-032](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-032). Execution order follows the predecessor tickets above and [the implementation sequence](22_ENGINEERING_EXECUTION_ORDER.md).

## Residual risks and scope discipline

The original local suite passing does not clear any of the durable YSQL, provider, consumer, security or operational gates. Every current finding remains open at handover; the audit made no implementation fix. The Memory-specific tenant collision is not attributed to parameterized YSQL queries. The internal legacy `Record` issue is not a current HTTP route bypass. Receipt codec failure is not described as a live database test. Missing frontend source is not an approved requirement to build a standalone UI.

The incoming engineer should update finding status only after attaching the named acceptance evidence and obtaining any required product/privacy decision. A signed decision can settle an ambiguous behavior; it cannot retroactively make a reproduced result disappear. Preserve the before-fix probes and source identity for regression review.
