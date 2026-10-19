# 16 — Detailed implementation backlog

**Product:** NVUTO Consent  
**Audit date:** 19 October 2026 (command timestamps in UTC; raw log offsets preserved)  
**Source:** supplied ZIP; commit and branch UNKNOWN  
**Disposition:** documentation-only audit; original source preserved.

## How to execute this backlog

There are **32 proposed engineering tickets**, one per canonical finding. No implementation in this backlog has been performed. Each ticket identifies the observed behavior, desired outcome, concrete implementation approach, dependencies, effort assumptions, tests and Definition of Done. The ordered dependency graph is in report22; the release decision is governed by report19.

Effort ranges are provisional senior-engineer **person-days of focused work**, including the listed local verification and documentation. They are not elapsed delivery dates or commitments. They assume access to an existing test platform and prompt owner decisions. Provisioning a new YSQL universe, building unseen frontends, vendor procurement, legal review, and eight unknown consumer codebases are not silently included. T-023 is discovery plus one pilot integration; additional consumers need separate estimates after inspection. T-024 is contract/read-owner discovery plus a bounded agreed implementation; a new application requires separate approved scope. T-032 is the explicit requirements-discovery ticket.

Completion dependencies are conservative merge/acceptance prerequisites. Engineers can investigate, design and write isolated failing tests in parallel before all predecessor tickets finish. Coordinate shared changes in service.go/postgres.go and migrations; avoid competing designs for the same preference revision or canonical receipt format.

## Ticket index

| Ticket | Priority / title | Owner role | Effort | Predecessors |
| --- | --- | --- | --- | --- |
| [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001) | P0 — Older double-opt-in challenges can restore permission after a later denial or withdrawal | Backend/data lead + privacy owner | 4–7 days | None; may start independently within an isolated development environment. |
| [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002) | P1 — Double-opt-in confirmation omits the self-service customer boundary | API/security engineer | 1–3 days | None; may start independently within an isolated development environment. |
| [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003) | P1 — Self-service receives the secret needed to bypass independent double-opt-in confirmation | Backend + trusted delivery integration owner | 4–8 days | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002) |
| [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004) | P1 — Receipt hashes do not survive the actual pgx timestamp precision round trip | Backend/data engineer | 2–4 days | None; may start independently within an isolated development environment. |
| [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005) | P1 — Notice and receipt records do not bind the accepted notice content and full decision | Backend/data engineer + privacy owner | 3–6 days | [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004) |
| [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006) | P1 — Evaluation time and current-state ordering do not implement a deterministic consent timeline | Backend/data lead + product owner | 4–8 days | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001) |
| [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007) | P1 — Stronger policy requirements are not re-enforced against existing grants or pending confirmations | Backend policy engineer + privacy owner | 3–6 days | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006) |
| [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008) | P1 — Upgraded legacy grants bypass all governed-policy checks when notice_version is blank | DBA/data engineer + privacy owner | 3–6 days | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007) |
| [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009) | P1 — Outbox lease completion and retry are not fenced to the current worker | Worker/backend engineer + SRE | 3–6 days | None; may start independently within an isolated development environment. |
| [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010) | P1 — Events lack the ordering and reconciliation contract needed to keep consumers consistent | API/event architect + consumer owners | 4–7 days | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009) |
| [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011) | P1 — Pinned toolchain and dependencies fail the current vulnerability gate | Backend maintainer + security engineer | 2–4 days | None; may start independently within an isolated development environment. |
| [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012) | P1 — Database work cannot follow request cancellation or a bounded transaction budget | Backend/data engineer + SRE | 3–5 days | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009) |
| [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013) | P2 — Unexpected database errors are returned as 422 with internal error text | API engineer | 1–3 days | [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012) |
| [T-014](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-014) | P1 — Unknown JWT key IDs trigger an unbounded JWKS refresh path | Identity/security engineer | 2–4 days | None; may start independently within an isolated development environment. |
| [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015) | P1 — Readiness can report healthy while authentication or required schema is unusable | Backend + SRE | 1–3 days | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016) | P1 — Durable API, transaction and multi-node Yugabyte behavior remain unverified | QA/backend + YSQL DBA | 4–8 days | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002), [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) | P1 — CI omits durable idempotency coverage and migration 003 upgrade verification | CI/backend engineer | 2–4 days | None; may start independently within an isolated development environment. |
| [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018) | P1 — Release packaging and deployment are not reproducible from the supplied manifests | Platform/release engineer | 3–6 days | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019) | P1 — Transport security and least-privilege service identities are not established | Platform/security + DBA + identity owner | 3–6 days | [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018) |
| [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020) | P1 — Existing NATS streams are accepted without checking durability, subjects or retention | Messaging/SRE owner | 2–4 days | [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019) |
| [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021) | P1 — Operators cannot reliably detect stalled consent propagation or diagnose request failures | SRE + backend engineer | 3–5 days | [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015) |
| [T-022](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-022) | P1 — Backup, restore, retention and recovery procedures are not production-evidenced | DBA/SRE + privacy owner | 3–7 days | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020) |
| [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023) | P1 — Named NVUTO product integrations have no end-to-end verification in this repository | Integration lead + each approved consumer owner | 4–10 days | [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020), [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030) |
| [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024) | P2 — Preference discovery, registry reads and a machine-readable client contract are absent | API architect + portal/admin owner | 3–6 days | None; may start independently within an isolated development environment. |
| [T-025](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-025) | P2 — Idempotency replay semantics change with policy and current consent state | Backend/API engineer | 2–4 days | [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| [T-026](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-026) | P2 — Memory-store composite keys collide across tenants and diverge from durable behavior | Backend maintainer | 1–3 days | None; may start independently within an isolated development environment. |
| [T-027](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-027) | P2 — Input normalization and validation leave ambiguous or surprising API behavior | API/backend engineer | 2–4 days | None; may start independently within an isolated development environment. |
| [T-028](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-028) | P2 — Database constraints do not enforce several tenant and state relationships | DBA/backend engineer | 2–5 days | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| [T-029](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-029) | P1 — Immutable audit rows cannot reconstruct policy and suppression history | Data/backend engineer + privacy owner | 3–5 days | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007) |
| [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030) | P1 — Mixed self-service and reader roles can authorize tenant-wide writes | Identity/security + privacy owner | 2–4 days | None; may start independently within an isolated development environment. |
| [T-031](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-031) | P2 — Legacy Record and Upsert helpers preserve a non-atomic governance bypass path | Backend maintainer | 1–2 days | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| [T-032](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-032) | P2 — Approved launch requirements, capacity and regional policy decisions are not supplied | Product/architecture + release owner | 2–4 days | None; may start independently within an isolated development environment. |

## T-001

**Older double-opt-in challenges can restore permission after a later denial or withdrawal**

| Planning field | Specification |
| --- | --- |
| Release priority | P0; mandatory for affected scope |
| Finding / acceptance / release gate | [F-001](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-001), [AT-001](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-001), [G-001](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-001) |
| Accountable delivery role | Backend/data lead + privacy owner |
| Complexity / estimate | High; 4–7 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | [D-03](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-03) |
| Affected APIs | API-04, API-05, API-09, API-10 |
| Dependent modules | MOD-04 — Consent service; MOD-05 — Persistence |
| Relevant current files | `internal/service/service.go:256-325`; `internal/store/postgres.go:340-383`; `internal/store/postgres.go:142-143`; `internal/store/memory.go:286-314` |

**Current behavior and evidence of the gap.** A previously issued challenge remains usable after the subject changes their mind. Confirmation makes the old row newest and evaluation returns allowed=true. Two audit probes reproduced denial and withdrawal being overridden. This breaks the documented withdrawal/effective-state safety rule.

**Expected behavior.** Starting a challenge, recording a later denial, and confirming the older challenge never grants permission. An older challenge cannot restore a newer consent after that newer consent is withdrawn. Concurrent confirm/deny/withdraw operations converge to the approved order with one atomic state/audit/outbox outcome. Re-run the two stale-challenge audit probes and equivalent tests on real YSQL.

**Detailed implementation approach.** Introduce a transactionally controlled preference generation or current-state record keyed by tenant, canonical subject, purpose and channel. Bind every challenge to that generation. Denial, withdrawal and superseding challenges must atomically invalidate prior challenges. Recheck generation, current policy and expiry under the same YSQL transaction before creating receipt/audit/outbox. Preserve historical evidence; migrate additively.

**Database work.** Review `consents`, `consent_receipts`, `consent_audit_events`, `consent_outbox_events`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for Data subjects, writers and all consuming apps. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Starting a challenge, recording a later denial, and confirming the older challenge never grants permission.
2. An older challenge cannot restore a newer consent after that newer consent is withdrawn.
3. Concurrent confirm/deny/withdraw operations converge to the approved order with one atomic state/audit/outbox outcome.
4. Re-run the two stale-challenge audit probes and equivalent tests on real YSQL.

**Tests and verification to deliver.** Memory regressions plus real YSQL concurrency tests for challenge→deny, challenge→new grant→withdraw, simultaneous confirms, retries and stale generation. Assert consent, receipt, audit and outbox counts, not only Allowed.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-001](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-001) is closed or explicitly dispositioned by its owner and [G-001](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-001) records the result, source commit, environment, timestamp and reviewer.

## T-002

**Double-opt-in confirmation omits the self-service customer boundary**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-002](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-002), [AT-002](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-002), [G-002](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-002) |
| Accountable delivery role | API/security engineer |
| Complexity / estimate | Medium; 1–3 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06) |
| Affected APIs | API-10 |
| Dependent modules | MOD-01 — HTTP API; MOD-04 — Consent service |
| Relevant current files | `cmd/server/main.go:109-118`; `cmd/server/main.go:343-362`; `internal/service/service.go:305-325`; `README.md:41` |

**Current behavior and evidence of the gap.** A self_service JWT for customer B confirmed customer A's valid token in the same tenant and received A's granted consent. A valid victim challenge token is required; the audit does not claim tokens can be guessed or cross-tenant confirmation succeeds.

**Expected behavior.** Customer B using customer A's challenge token receives a uniform forbidden/not-found result and causes no state, receipt, audit or outbox mutation. Own-subject confirmation and approved tenant-wide writer/admin flows succeed. Cross-tenant, missing-customer-claim and mixed-role cases follow an approved permission matrix.

**Detailed implementation approach.** Pass the authenticated principal/customer constraint into a confirmation operation that resolves the pending challenge and enforces object authorization before any mutation. Recheck inside the atomic confirmation path or bind authorization to the locked record. Preserve broader tenant roles deliberately; reject self_service tokens missing customer_subject_id. Avoid leaking receipt or consent data on rejection.

**Database work.** Review `consents`, `consent_receipts`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for self_service. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Customer B using customer A's challenge token receives a uniform forbidden/not-found result and causes no state, receipt, audit or outbox mutation.
2. Own-subject confirmation and approved tenant-wide writer/admin flows succeed.
3. Cross-tenant, missing-customer-claim and mixed-role cases follow an approved permission matrix.

**Tests and verification to deliver.** Authenticated handler tests using production-compatible RS256 tokens for every role/tenant/customer combination; repeat against YSQL and assert rejected requests cause zero writes.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-002](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-002) is closed or explicitly dispositioned by its owner and [G-002](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-002) records the result, source commit, environment, timestamp and reviewer.

## T-003

**Self-service receives the secret needed to bypass independent double-opt-in confirmation**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-003](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-003), [AT-003](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-003), [G-003](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-003) |
| Accountable delivery role | Backend + trusted delivery integration owner |
| Complexity / estimate | High; 4–8 provisional person-days |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002) |
| Product/architecture decisions | [D-02](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-02), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07) |
| Affected APIs | API-09, API-10 |
| Dependent modules | MOD-01 — HTTP API; MOD-03 — Domain contracts; MOD-04 — Consent service |
| Relevant current files | `cmd/server/main.go:319-340`; `internal/domain/model.go:127-130`; `internal/service/service.go:271-302` |

**Current behavior and evidence of the gap.** A self-service caller receives confirmation_token and immediately echoes it to obtain allowed=true without any delivery step. The challenge is bound to a channel label, not a verified destination. No message-delivery adapter or trusted delivery contract is present.

**Expected behavior.** A self-service initiation response cannot disclose the confirmation capability. An approved sandbox delivery journey proves the intended destination receives the challenge and confirmation is bound to that destination and subject. Resend/cancel/expiry/replay and partial delivery failures have documented, tested outcomes.

**Detailed implementation approach.** Decide which trusted NVUTO component owns confirmation delivery. Return an opaque challenge reference/status to untrusted callers; keep the raw secret within an approved delivery path. Bind the challenge to tenant, canonical subject, purpose, channel, verified destination/version, notice and generation. Define resend, cancel, maximum lifetime, retry and abuse controls without exposing tokens in events or logs.

**Database work.** Review `consents`, `consent_outbox_events`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for self_service, trusted delivery integrator. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. A self-service initiation response cannot disclose the confirmation capability.
2. An approved sandbox delivery journey proves the intended destination receives the challenge and confirmation is bound to that destination and subject.
3. Resend/cancel/expiry/replay and partial delivery failures have documented, tested outcomes.

**Tests and verification to deliver.** Actual sandbox recipient journey using the approved delivery owner, with response/event/log secret checks, destination mismatch, resend, expiry, cancelled and repeated challenges; no production sending.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-003](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-003) is closed or explicitly dispositioned by its owner and [G-003](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-003) records the result, source commit, environment, timestamp and reviewer.

## T-004

**Receipt hashes do not survive the actual pgx timestamp precision round trip**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-004](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-004), [AT-004](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-004), [G-004](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-004) |
| Accountable delivery role | Backend/data engineer |
| Complexity / estimate | Medium; 2–4 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05) |
| Affected APIs | API-04, API-10, API-11 |
| Dependent modules | MOD-04 — Consent service; MOD-05 — Persistence; MOD-07 — Schema lifecycle |
| Relevant current files | `internal/service/service.go:108-117`; `internal/store/postgres.go:119-122`; `internal/store/postgres.go:289-297`; `migrations/002_policy_governance.sql:57-70` |

**Current behavior and evidence of the gap.** receiptHash hashes a Go IssuedAt with nanoseconds, while the real pgx timestamptz codec changes .123456789 to .123456. Re-hashing that decoded receipt fails. Existing tests only check a nonempty hash, so they miss this defect.

**Expected behavior.** Hash verification succeeds before storage, after a real YSQL read, and after JSON export/import. Nanosecond input, timezone offsets, map ordering and unicode cases produce deterministic documented bytes. Historic receipt remediation preserves original evidence and produces a reviewable reconciliation report.

**Detailed implementation approach.** Define a versioned canonical receipt serialization and timestamp precision before hashing. Persist the exact signed/hashed payload or normalize every hashed field to a stable representation. Add a verifier that distinguishes old and new receipt formats; never rewrite immutable historic receipts to hide mismatches. Decide how existing non-verifiable receipts will be annotated and reconciled.

**Database work.** Review `consent_receipts`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for Privacy reviewers, data subjects, consuming apps. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Hash verification succeeds before storage, after a real YSQL read, and after JSON export/import.
2. Nanosecond input, timezone offsets, map ordering and unicode cases produce deterministic documented bytes.
3. Historic receipt remediation preserves original evidence and produces a reviewable reconciliation report.

**Tests and verification to deliver.** Canonical-byte unit vectors and actual pgx/YSQL/JSON round trips, timezone/microsecond/unicode/map cases; verify legacy receipts without rewriting originals.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-004](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-004) is closed or explicitly dispositioned by its owner and [G-004](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-004) records the result, source commit, environment, timestamp and reviewer.

## T-005

**Notice and receipt records do not bind the accepted notice content and full decision**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-005](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-005), [AT-005](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-005), [G-005](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-005) |
| Accountable delivery role | Backend/data engineer + privacy owner |
| Complexity / estimate | High; 3–6 provisional person-days |
| Dependencies | [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004) |
| Product/architecture decisions | [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) |
| Affected APIs | API-03, API-04, API-10, API-11 |
| Dependent modules | MOD-03 — Domain contracts; MOD-04 — Consent service; MOD-07 — Schema lifecycle |
| Relevant current files | `README.md:16`; `internal/domain/model.go:71-97`; `internal/service/service.go:115-117`; `migrations/002_policy_governance.sql:48-71` |

**Current behavior and evidence of the gap.** Notices contain only tenant, version, jurisdiction, published_at and active. Receipts carry a notice identifier and supplied evidence hash, but no immutable notice content/hash/reference, consent status, legal basis, policy version or actor. A nonempty hash is not proof of what text and decision were accepted.

**Expected behavior.** An exported receipt can resolve and verify the exact accepted notice artifact and the decision/legal basis represented. A notice identifier cannot silently refer to altered content. Notice retirement preserves existing receipts and has a tested effect on new capture and evaluation under the approved rule.

**Detailed implementation approach.** Agree the minimum evidence contract with the privacy owner. Store or immutably reference the notice artifact, digest algorithm, content digest, language and version; bind decision/legal-basis/policy version and collection/confirmation facts into the canonical receipt. Define a separate retirement/supersession record because existing notice rows prohibit updates/deletes. Keep legal sufficiency subject to review.

**Database work.** Review `consent_privacy_notices`, `consent_receipts`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. An exported receipt can resolve and verify the exact accepted notice artifact and the decision/legal basis represented.
2. A notice identifier cannot silently refer to altered content.
3. Notice retirement preserves existing receipts and has a tested effect on new capture and evaluation under the approved rule.

**Tests and verification to deliver.** Notice artifact digest/immutability/retirement tests; end-to-end capture→export→independent verification using exact policy/decision content.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-005](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-005) is closed or explicitly dispositioned by its owner and [G-005](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-005) records the result, source commit, environment, timestamp and reviewer.

## T-006

**Evaluation time and current-state ordering do not implement a deterministic consent timeline**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-006](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-006), [AT-006](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-006), [G-006](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-006) |
| Accountable delivery role | Backend/data lead + product owner |
| Complexity / estimate | High; 4–8 provisional person-days |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001) |
| Product/architecture decisions | [D-03](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-03), [D-12](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-12) |
| Affected APIs | API-04, API-05, API-07, API-08 |
| Dependent modules | MOD-01 — HTTP API; MOD-04 — Consent service; MOD-05 — Persistence |
| Relevant current files | `internal/service/service.go:342-387`; `internal/store/postgres.go:142-143`; `internal/store/memory.go:76-92`; `cmd/server/main.go:37-42` |

**Current behavior and evidence of the gap.** A governed grant was reported allowed one hour before it existed when its notice already existed. Evaluation always loads current suppression/latest updated row, and updated_at ties have no deterministic tiebreaker. Withdrawing an older row can supersede a newer grant; that latter rule is undocumented.

**Expected behavior.** Evaluation never claims permission before the applicable grant/effective time. Past withdrawal/suppression cases return historical results or an explicit unsupported-time error according to the approved contract. Equal timestamps, concurrent writes and older-record withdrawal produce one documented deterministic result in memory and YSQL.

**Detailed implementation approach.** Choose and publish whether at supports historical evaluation, future scheduling, or only present-time decisions. If historical evaluation is supported, query immutable state/policy/suppression intervals as of at and enforce granted/effective boundaries. Otherwise reject unsupported at values explicitly. Use a transactionally sequenced preference revision, not wall-clock ordering alone. Define older-record withdrawal and equal-timestamp behavior. If full historical evaluation is approved, this ticket must own the shared immutable timeline/policy/suppression foundation before dependent policy enforcement and audit export work; re-estimate that expanded scope through T-032/D-03 rather than introducing a dependency cycle.

**Database work.** Review `consents`, `consent_suppressions`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Evaluation never claims permission before the applicable grant/effective time.
2. Past withdrawal/suppression cases return historical results or an explicit unsupported-time error according to the approved contract.
3. Equal timestamps, concurrent writes and older-record withdrawal produce one documented deterministic result in memory and YSQL.

**Tests and verification to deliver.** Table-driven current/historical/future-time cases under the approved contract; equal timestamp, older withdrawal, concurrent state revisions, suppression and policy as-of behavior on Memory and YSQL.

**Scope/estimate condition.** The 4–8 day range assumes a bounded approved time/revision contract. If D-03 approves full historical evaluation, allocate the shared immutable consent/policy/suppression timeline foundation inside this ticket before T-007/T-029 and re-estimate through T-032. Do not introduce a dependency cycle by waiting on T-029.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-006](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-006) is closed or explicitly dispositioned by its owner and [G-006](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-006) records the result, source commit, environment, timestamp and reviewer.

## T-007

**Stronger policy requirements are not re-enforced against existing grants or pending confirmations**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-007](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-007), [AT-007](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-007), [G-007](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-007) |
| Accountable delivery role | Backend policy engineer + privacy owner |
| Complexity / estimate | High; 3–6 provisional person-days |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006) |
| Product/architecture decisions | [D-04](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-04) |
| Affected APIs | API-01, API-02, API-07, API-08, API-10 |
| Dependent modules | MOD-04 — Consent service |
| Relevant current files | `internal/service/service.go:88-105`; `internal/service/service.go:134-136`; `internal/service/service.go:305-325`; `internal/service/service.go:373-385` |

**Current behavior and evidence of the gap.** Changing a purpose to require affirmative consent left a contract-based grant allowed. Requiring double opt-in left an unconfirmed grant allowed. Confirmation also does not re-run policy validation. The current evaluator checks active/channel/category/notice only; no explicit grandfathering rule exists.

**Expected behavior.** Policy-transition tests cover affirmative, DOI, evidence, active, category and channel changes. A grant incompatible with current mandatory policy is denied or covered by a recorded approved grandfathering rule. Pending confirmation cannot silently bypass policy changes made after initiation.

**Detailed implementation approach.** Version policy rules and decide when changes invalidate, grandfather or require recapture. Store the policy/evidence provenance used for each decision. Revalidate all applicable requirements at evaluation and confirmation using a consistent policy snapshot, or implement explicitly approved grandfathering with visible reasons and boundaries. Handle policy updates racing with writes.

**Database work.** Review `consent_purposes`, `consent_lawful_bases`, `consents`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Policy-transition tests cover affirmative, DOI, evidence, active, category and channel changes.
2. A grant incompatible with current mandatory policy is denied or covered by a recorded approved grandfathering rule.
3. Pending confirmation cannot silently bypass policy changes made after initiation.

**Tests and verification to deliver.** Policy-version transition matrix for active/channel/category/affirmative/evidence/DOI requirements and confirmation racing with policy update; explicitly test approved grandfathering bounds.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-007](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-007) is closed or explicitly dispositioned by its owner and [G-007](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-007) records the result, source commit, environment, timestamp and reviewer.

## T-008

**Upgraded legacy grants bypass all governed-policy checks when notice_version is blank**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-008](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-008), [AT-008](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-008), [G-008](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-008) |
| Accountable delivery role | DBA/data engineer + privacy owner |
| Complexity / estimate | High; 3–6 provisional person-days |
| Dependencies | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007) |
| Product/architecture decisions | [D-04](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-04), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) |
| Affected APIs | API-07, API-08 |
| Dependent modules | MOD-04 — Consent service; MOD-07 — Schema lifecycle; MOD-08 — Build, CI and deployment |
| Relevant current files | `migrations/002_policy_governance.sql:6-13`; `.github/workflows/ci.yml:58-62`; `internal/service/service.go:373-387` |

**Current behavior and evidence of the gap.** Migration 002 retains legacy rows with blank notice_version, and evaluation skips every registry check for those rows. CI explicitly asserts the blank value remains. Existing production data, if any, was not supplied, so affected record counts are unknown.

**Expected behavior.** Every legacy record category has a documented eligibility decision and reconciliation count. Blank-notice records cannot bypass an affirmative-consent requirement by default. Fresh install and 001-to-latest upgrade tests exercise both safe and unsafe legacy cases without deleting evidence.

**Detailed implementation approach.** Create a read-only legacy classification query and obtain policy-owner decisions for each class. Backfill verified evidence/provenance where possible; otherwise quarantine or require recapture rather than assuming consent. Add an explicit legacy eligibility rule to the evaluator. Rehearse the additive migration against representative synthetic legacy data and retain reconciliation evidence.

**Database work.** Review `consents`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Every legacy record category has a documented eligibility decision and reconciliation count.
2. Blank-notice records cannot bypass an affirmative-consent requirement by default.
3. Fresh install and 001-to-latest upgrade tests exercise both safe and unsafe legacy cases without deleting evidence.

**Tests and verification to deliver.** Fresh 001→002→003 and upgrade-from-001/002 datasets containing blank notice, absent evidence, incompatible basis, expired and withdrawn states; reconcile class counts and prove no permissive default.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-008](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-008) is closed or explicitly dispositioned by its owner and [G-008](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-008) records the result, source commit, environment, timestamp and reviewer.

## T-009

**Outbox lease completion and retry are not fenced to the current worker**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-009](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-009), [AT-009](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-009), [G-009](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-009) |
| Accountable delivery role | Worker/backend engineer + SRE |
| Complexity / estimate | High; 3–6 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08) |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | MOD-06 — Outbox worker |
| Relevant current files | `cmd/outbox-worker/main.go:46-79`; `cmd/outbox-worker/main.go:116-134`; `cmd/outbox-worker/main.go:215-231` |

**Current behavior and evidence of the gap.** A whole batch of 50 events receives a two-minute lease and is published sequentially. After expiry another worker may claim it; complete/retry only use event ID, so a stale worker can mutate another worker's lease. This does not by itself prove permanent event loss, but HA ownership is not established.

**Expected behavior.** A stale worker cannot complete, clear or reschedule a row owned by a newer claim. Slow publish, killed worker and acknowledgement/commit-gap tests preserve eventual delivery without conflicting state transitions. Shutdown releases or safely expires claims within the approved budget.

**Detailed implementation approach.** Add an unguessable claim generation/token and require it in completion, retry and renewal updates. Bound publish time per event and batch; renew only owned leases or keep batches within the lease budget. Stop claiming during shutdown and define recovery after publish acknowledgement but before SQL completion. Keep NATS message-ID deduplication and require idempotent consumers.

**Database work.** Review `consent_outbox_events`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for Operators and all event consumers. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. A stale worker cannot complete, clear or reschedule a row owned by a newer claim.
2. Slow publish, killed worker and acknowledgement/commit-gap tests preserve eventual delivery without conflicting state transitions.
3. Shutdown releases or safely expires claims within the approved budget.

**Tests and verification to deliver.** Two real workers with forced lease expiry, slow/failed broker, worker kill, ack/SQL gap, shutdown, and stale completion/retry attempts; capture event IDs and ownership transitions.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-009](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-009) is closed or explicitly dispositioned by its owner and [G-009](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-009) records the result, source commit, environment, timestamp and reviewer.

## T-010

**Events lack the ordering and reconciliation contract needed to keep consumers consistent**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-010](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-010), [AT-010](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-010), [G-010](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-010) |
| Accountable delivery role | API/event architect + consumer owners |
| Complexity / estimate | High; 4–7 provisional person-days |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009) |
| Product/architecture decisions | [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07), [D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13) |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | MOD-03 — Domain contracts; MOD-04 — Consent service; MOD-05 — Persistence; MOD-06 — Outbox worker |
| Relevant current files | `internal/domain/model.go:141-151`; `internal/service/service.go:49-84`; `internal/service/service.go:147-148`; `internal/service/service.go:197-198`; `internal/service/service.go:232-233`; `internal/store/postgres.go:106-115`; `cmd/outbox-worker/main.go:82-113` |

**Current behavior and evidence of the gap.** schema_version is an event-format version, not a preference revision. Events omit an aggregate state version; registry payloads omit key rules; aggregate_type defaults to consent even for registries/suppressions. Multiple workers and retries can publish out of order. Expiry is evaluated on reads without an expiry event. No consumer reconciliation contract is supplied.

**Expected behavior.** All eight event families have versioned examples and contract tests. Grant then withdrawal delivered in reverse order cannot leave a consumer permitted to communicate. Replay beyond the dedup window and expiry/policy-change scenarios converge through the documented consumer contract.

**Detailed implementation approach.** Define versioned event schemas with correct aggregate types, tenant/canonical subject, aggregate revision, effective timestamp and enough information or a resolvable read API to reconcile. Choose consumer behavior for out-of-order events, duplicate IDs, expiry, withdrawal and policy changes. Prefer a current-state re-evaluation before communication where safety requires it; do not promise exactly-once delivery from finite broker dedup alone.

**Database work.** Review `consent_outbox_events`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for All eight named consuming products. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. All eight event families have versioned examples and contract tests.
2. Grant then withdrawal delivered in reverse order cannot leave a consumer permitted to communicate.
3. Replay beyond the dedup window and expiry/policy-change scenarios converge through the documented consumer contract.

**Tests and verification to deliver.** Contract fixtures for eight event families/nine concrete types; reverse grant/withdraw order, duplication beyond dedup window, restart/replay, policy change and expiry reconciliation in a consumer harness.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-010](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-010) is closed or explicitly dispositioned by its owner and [G-010](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-010) records the result, source commit, environment, timestamp and reviewer.

## T-011

**Pinned toolchain and dependencies fail the current vulnerability gate**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-011](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-011), [AT-011](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-011), [G-011](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-011) |
| Accountable delivery role | Backend maintainer + security engineer |
| Complexity / estimate | Medium; 2–4 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | None identified as a prerequisite |
| Affected APIs | OPS-01, OPS-02 |
| Dependent modules | MOD-08 — Build, CI and deployment |
| Relevant current files | `go.mod:3-21`; `Dockerfile:1-8`; `Dockerfile.worker:1-8`; `.github/workflows/ci.yml:18-28` |

**Current behavior and evidence of the gap.** govulncheck v1.7.0 with the 2026-10-08 database reported ten symbol-level advisories: nine Go standard-library advisories and one x/text advisory. Nine additional package/module-only findings are not proven called. Text conversion of the same result exited with findings. This is not a claim that all ten are exploitable in the deployed configuration.

**Expected behavior.** The standard CI vulnerability gate exits successfully or has narrowly documented, time-bound security-owner exceptions with configuration evidence. Both API and worker are built from the same approved patched toolchain and pass regression/race checks. Runtime image and transitive-dependency scans are attached to the immutable release artifacts.

**Detailed implementation approach.** Upgrade the Go build/CI/container toolchain consistently to a maintained patched release; official advisories identify Go 1.26.9 or the current 1.27.2 line. Upgrade x/text to at least the indicated fixed version v0.41.0, then resolve the compatible dependency graph. Triage module-only findings and scan runtime/container images. Preserve the scanner database date, platform, SBOM and final image digests.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for Operators, all API users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. The standard CI vulnerability gate exits successfully or has narrowly documented, time-bound security-owner exceptions with configuration evidence.
2. Both API and worker are built from the same approved patched toolchain and pass regression/race checks.
3. Runtime image and transitive-dependency scans are attached to the immutable release artifacts.

**Tests and verification to deliver.** Supported-toolchain build, original plus repaired regression suite, race run, text-mode vulnerability scan and final container/SBOM scan with exact versions and image digests.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-011](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-011) is closed or explicitly dispositioned by its owner and [G-011](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-011) records the result, source commit, environment, timestamp and reviewer.

## T-012

**Database work cannot follow request cancellation or a bounded transaction budget**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-012](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-012), [AT-012](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-012), [G-012](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-012) |
| Accountable delivery role | Backend/data engineer + SRE |
| Complexity / estimate | Medium; 3–5 provisional person-days |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009) |
| Product/architecture decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08) |
| Affected APIs | API-01, API-02, API-03, API-04, API-05, API-06, API-07, API-08, API-09, API-10, API-11 |
| Dependent modules | MOD-01 — HTTP API; MOD-05 — Persistence |
| Relevant current files | `internal/store/memory.go:15-35`; `internal/store/postgres.go:125-143`; `internal/store/postgres.go:238-286`; `internal/store/idempotency.go:59-98`; `cmd/server/main.go:435` |

**Current behavior and evidence of the gap.** The Repository interface has no request context; durable methods use context.Background. HTTP timeouts do not cancel those operations. A slow/unavailable database or lock wait can retain work and exhaust fixed pools. Transaction retry behavior for YSQL conflicts is not defined.

**Expected behavior.** A cancelled/timed-out request promptly cancels pending database work and releases pool resources. Serialization/transaction conflict and lost-response tests either retry safely or return a retryable error without duplicate effects. Load tests at the approved replica count demonstrate bounded connections, latency and queue growth.

**Detailed implementation approach.** Thread context.Context through handlers, service and repository. Add bounded request, SQL and whole-transaction deadlines with cancellation-aware retries only for approved transient errors. Ensure a retry does not duplicate receipt/audit/outbox effects; record retry limits and backoff. Size API and worker pools against total replica capacity rather than an isolated process.

**Database work.** Review `All runtime tables`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. A cancelled/timed-out request promptly cancels pending database work and releases pool resources.
2. Serialization/transaction conflict and lost-response tests either retry safely or return a retryable error without duplicate effects.
3. Load tests at the approved replica count demonstrate bounded connections, latency and queue growth.

**Tests and verification to deliver.** Real YSQL request cancellation, lock waits, serialization conflicts, lost responses and approved load; measure released connections and absence of duplicated transactional effects.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-012](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-012) is closed or explicitly dispositioned by its owner and [G-012](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-012) records the result, source commit, environment, timestamp and reviewer.

## T-013

**Unexpected database errors are returned as 422 with internal error text**

| Planning field | Specification |
| --- | --- |
| Release priority | P2; scope/deferral decision required; may block dependent tickets |
| Finding / acceptance / release gate | [F-013](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-013), [AT-013](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-013), [G-013](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-013) |
| Accountable delivery role | API engineer |
| Complexity / estimate | Medium; 1–3 provisional person-days |
| Dependencies | [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012) |
| Product/architecture decisions | None identified as a prerequisite |
| Affected APIs | All business APIs |
| Dependent modules | MOD-01 — HTTP API; MOD-04 — Consent service; MOD-05 — Persistence |
| Relevant current files | `cmd/server/main.go:59-85`; `internal/store/postgres.go:44-55`; `internal/service/service.go:88-103` |

**Current behavior and evidence of the gap.** writeError defaults all unrecognized errors to 422 and exposes err.Error(). Infrastructure failures can look like client validation faults and may expose SQL/schema details. Some policy lookups mask infrastructure errors as validation or policy_unavailable, reducing diagnosis quality.

**Expected behavior.** Injected database timeout/outage/internal SQL errors return the approved 5xx/503 shape without internal details. Validation remains actionable 4xx and conflicts/replays keep their documented semantics. Client and operator logs can correlate one failure without storing bearer/confirmation secrets.

**Detailed implementation approach.** Introduce typed validation, conflict, authorization, unavailable and internal errors. Return stable public codes/statuses and a correlation ID; log sanitized internal causes with that ID. Preserve safe-deny behavior for unavailable policy while distinguishing operational health. Document which failures clients may retry and keep receipt/token material out of logs.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Injected database timeout/outage/internal SQL errors return the approved 5xx/503 shape without internal details.
2. Validation remains actionable 4xx and conflicts/replays keep their documented semantics.
3. Client and operator logs can correlate one failure without storing bearer/confirmation secrets.

**Tests and verification to deliver.** Dependency timeout/unavailable/SQL error injection and stable status/public error/correlation assertions; ensure operational logs retain sanitized diagnosis and omit capabilities/evidence.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-013](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-013) is closed or explicitly dispositioned by its owner and [G-013](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-013) records the result, source commit, environment, timestamp and reviewer.

## T-014

**Unknown JWT key IDs trigger an unbounded JWKS refresh path**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-014](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-014), [AT-014](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-014), [G-014](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-014) |
| Accountable delivery role | Identity/security engineer |
| Complexity / estimate | Medium; 2–4 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| Affected APIs | All authenticated APIs |
| Dependent modules | MOD-02 — Authentication |
| Relevant current files | `internal/auth/auth.go:41-47`; `internal/auth/auth.go:95-116`; `internal/auth/auth.go:151-163` |

**Current behavior and evidence of the gap.** Untrusted RS256 token key IDs force a fresh outbound request even while the cache is fresh. Five distinct misses caused five fetches in the audit. No refresh coalescing, cooldown, negative cache or inbound rate limiter exists; invalid requests can amplify load on Consent and the identity provider.

**Expected behavior.** Bursts of distinct unknown kids do not produce an unbounded number of JWKS fetches. Legitimate key rotation works within the approved delay, and unknown keys always fail closed. Concurrent refresh, identity-provider outage, expiry and rate-limit tests meet documented budgets.

**Detailed implementation approach.** Add per-JWKS singleflight refresh, a bounded negative cache/cooldown and explicit key rotation behavior. Enforce token/header size and request-rate limits at an owned boundary. Validate HTTPS/trusted JWKS configuration and define bounded last-known-good key behavior during outages without accepting unknown keys.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Bursts of distinct unknown kids do not produce an unbounded number of JWKS fetches.
2. Legitimate key rotation works within the approved delay, and unknown keys always fail closed.
3. Concurrent refresh, identity-provider outage, expiry and rate-limit tests meet documented budgets.

**Tests and verification to deliver.** Local mock JWKS concurrent unknown-kid flood, slow/unavailable/oversized responses and real staging key rotation; demonstrate bounded refresh without accepting invalid signatures.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-014](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-014) is closed or explicitly dispositioned by its owner and [G-014](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-014) records the result, source commit, environment, timestamp and reviewer.

## T-015

**Readiness can report healthy while authentication or required schema is unusable**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-015](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-015), [AT-015](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-015), [G-015](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-015) |
| Accountable delivery role | Backend + SRE |
| Complexity / estimate | Medium; 1–3 provisional person-days |
| Dependencies | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Product/architecture decisions | [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| Affected APIs | OPS-01, OPS-02 |
| Dependent modules | MOD-01 — HTTP API; MOD-04 — Consent service; MOD-05 — Persistence |
| Relevant current files | `cmd/server/main.go:386-398`; `cmd/server/main.go:413-435`; `internal/service/service.go:390`; `internal/store/postgres.go:38-41` |

**Current behavior and evidence of the gap.** Readiness checks only database Ping. Missing/invalid authentication configuration or a reachable database missing migration tables still yields ready. Health endpoints accept every method, including CONNECT, which matters to a scanner-identified standard-library precondition.

**Expected behavior.** Missing auth settings or required migrations keep the deployment unready or fail startup with a sanitized diagnostic. Liveness remains independent of routine dependency outages; readiness behavior is specified and tested. Health endpoints reject unsupported methods and worker health exposes meaningful progress/lag.

**Detailed implementation approach.** Validate required auth mode, issuer/audience/JWKS transport, production store and configuration at startup. Verify a compatible recorded schema version before readiness without making every probe depend on a live IdP call. Restrict probe methods and define API/worker startup/readiness/liveness meanings. Fail deployment on unusable configuration.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Missing auth settings or required migrations keep the deployment unready or fail startup with a sanitized diagnostic.
2. Liveness remains independent of routine dependency outages; readiness behavior is specified and tested.
3. Health endpoints reject unsupported methods and worker health exposes meaningful progress/lag.

**Tests and verification to deliver.** Boot with absent/invalid auth, unavailable JWKS as defined, missing001/002/003 schema and unavailable DB; verify startup/readiness policy and rolling-deploy recovery.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-015](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-015) is closed or explicitly dispositioned by its owner and [G-015](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-015) records the result, source commit, environment, timestamp and reviewer.

## T-016

**Durable API, transaction and multi-node Yugabyte behavior remain unverified**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-016](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-016), [AT-016](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-016), [G-016](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-016) |
| Accountable delivery role | QA/backend + YSQL DBA |
| Complexity / estimate | High; 4–8 provisional person-days |
| Dependencies | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002), [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Product/architecture decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) |
| Affected APIs | API-01, API-02, API-03, API-04, API-05, API-06, API-07, API-08, API-09, API-10, API-11 |
| Dependent modules | MOD-01 — HTTP API; MOD-05 — Persistence; MOD-06 — Outbox worker |
| Relevant current files | `README.md:45-51`; `cmd/server/postgres_integration_test.go:17-83`; `internal/store/idempotency_integration_test.go:14-73`; `cmd/outbox-worker/postgres_integration_test.go:15-120` |

**Current behavior and evidence of the gap.** All three external-integration tests skipped in the audit. The server database test calls Service directly, not authenticated HTTP. No current CI run artifact, production-equivalent YSQL universe, failover evidence or real provider credentials was supplied. Passing memory tests cannot establish durable production behavior.

**Expected behavior.** All three existing external tests execute rather than skip, and new critical HTTP/YSQL tests pass. A fault between state/receipt/audit/outbox writes leaves either all committed or none. Restart, node loss and transaction contention preserve the approved current-state and delivery invariants.

**Detailed implementation approach.** Provision an explicitly isolated target-version YSQL environment and NATS test account. Run every database test plus authenticated HTTP-to-YSQL workflows, transaction fault injection, restarts, concurrent writes and receipt round trips. Separately run multi-node placement/failure tests on the approved production topology. Record exact versions, sanitized config, timestamps, test logs and database assertions.

**Database work.** Review `All runtime tables`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. All three existing external tests execute rather than skip, and new critical HTTP/YSQL tests pass.
2. A fault between state/receipt/audit/outbox writes leaves either all committed or none.
3. Restart, node loss and transaction contention preserve the approved current-state and delivery invariants.

**Tests and verification to deliver.** All11 business routes over HTTP with production-compatible JWTs and real target YSQL, receipt re-read, idempotency, competing transactions, node restart/leader movement and selected broker integration.

**Estimate dependency.** The stated range assumes access to the target staging services and platform owners. Missing provisioning, provider approvals, quotas or infrastructure design require separate discovery estimates; do not substitute local Memory or single-node fixtures for the required target evidence.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-016](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-016) is closed or explicitly dispositioned by its owner and [G-016](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-016) records the result, source commit, environment, timestamp and reviewer.

## T-017

**CI omits durable idempotency coverage and migration 003 upgrade verification**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-017](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-017), [AT-017](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-017), [G-017](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-017) |
| Accountable delivery role | CI/backend engineer |
| Complexity / estimate | Medium; 2–4 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | None identified as a prerequisite |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | MOD-05 — Persistence; MOD-07 — Schema lifecycle; MOD-08 — Build, CI and deployment |
| Relevant current files | `.github/workflows/ci.yml:20-28`; `.github/workflows/ci.yml:51-72`; `internal/store/idempotency_integration_test.go:14-46`; `migrations/003_governed_idempotency.sql:1-11` |

**Current behavior and evidence of the gap.** The general test job has no database. The YSQL job selects only server and worker tests, leaving the store idempotency integration test skipped everywhere. The upgrade path stops at002. That test also uses globally fixed record IDs despite a unique tenant, making reruns unsafe against a reused test database.

**Expected behavior.** CI evidence shows the store concurrency test actually executed on YSQL. Fresh install and001-to-latest upgrade include003 and verify idempotent replay/conflict after upgrade. The suite can run twice in its designated isolated environment without fixture-ID collisions.

**Detailed implementation approach.** Run every integration package against a deliberately isolated YSQL database using one consistent test-DSN contract. Fail the required job when prerequisites are missing rather than silently skipping. Upgrade001 through every current migration including003; exercise persisted state and idempotency after upgrade. Use run-unique global fixture IDs and preserve machine-readable test artifacts.

**Database work.** Review `consent_idempotency`, `consents`, `consent_receipts`, `consent_outbox_events`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. CI evidence shows the store concurrency test actually executed on YSQL.
2. Fresh install and001-to-latest upgrade include003 and verify idempotent replay/conflict after upgrade.
3. The suite can run twice in its designated isolated environment without fixture-ID collisions.

**Tests and verification to deliver.** CI with mandatory isolated DB/NATS prerequisites, all three durable tests run twice in retained test databases, plus 001 and002 upgrade paths through003; skipped mandatory tests fail the job.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-017](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-017) is closed or explicitly dispositioned by its owner and [G-017](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-017) records the result, source commit, environment, timestamp and reviewer.

## T-018

**Release packaging and deployment are not reproducible from the supplied manifests**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-018](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-018), [AT-018](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-018), [G-018](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-018) |
| Accountable delivery role | Platform/release engineer |
| Complexity / estimate | High; 3–6 provisional person-days |
| Dependencies | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Product/architecture decisions | [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | MOD-08 — Build, CI and deployment |
| Relevant current files | `deploy/k8s.yaml:39-64`; `deploy/outbox-worker.yaml:34-56`; `.github/workflows/ci.yml:27-30`; `Dockerfile:1-10`; `Dockerfile.worker:1-10` |

**Current behavior and evidence of the gap.** The API uses :latest and the worker :latest-worker without explicit pull policy/digests. CI builds test images but does not publish the referenced release images. Required ConfigMap, Secrets, database, NATS and ingress ownership are external and unspecified. The manifests are a baseline, not a complete release procedure.

**Expected behavior.** A new authorized engineer can deploy the same exact API/worker digests in staging using the documented inputs. Missing configuration is detected before rollout; API and worker run the intended release after rolling update. Rollback uses a known prior digest and passes current-schema compatibility checks.

**Detailed implementation approach.** Define environment overlays or equivalent deployment inputs, immutable image digests, release provenance and an owned artifact publication/promotion flow. Supply secure references/templates for required configuration and document external infrastructure dependencies. Rehearse rollout and image rollback with additive schema compatibility. Do not embed real secret values in the repository.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for Release engineer and platform operator. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. A new authorized engineer can deploy the same exact API/worker digests in staging using the documented inputs.
2. Missing configuration is detected before rollout; API and worker run the intended release after rolling update.
3. Rollback uses a known prior digest and passes current-schema compatibility checks.

**Tests and verification to deliver.** Build both immutable images from recorded commit; validate rendered deployment inputs, roll out in staging, record actual digests/pull behavior, then roll back to previous compatible image.

**Estimate dependency.** The stated range assumes access to the target staging services and platform owners. Missing provisioning, provider approvals, quotas or infrastructure design require separate discovery estimates; do not substitute local Memory or single-node fixtures for the required target evidence.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-018](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-018) is closed or explicitly dispositioned by its owner and [G-018](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-018) records the result, source commit, environment, timestamp and reviewer.

## T-019

**Transport security and least-privilege service identities are not established**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-019](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-019), [AT-019](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-019), [G-019](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-019) |
| Accountable delivery role | Platform/security + DBA + identity owner |
| Complexity / estimate | High; 3–6 provisional person-days |
| Dependencies | [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018) |
| Product/architecture decisions | [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | MOD-01 — HTTP API; MOD-02 — Authentication; MOD-06 — Outbox worker; MOD-08 — Build, CI and deployment |
| Relevant current files | `internal/auth/auth.go:41-47`; `cmd/server/main.go:435-439`; `cmd/outbox-worker/main.go:185-194`; `deploy/outbox-worker.yaml:38-56`; `README.md:47-51` |

**Current behavior and evidence of the gap.** JWKS accepts configurable HTTP URLs; API serves plaintext behind an unspecified boundary; NATS credentials do not themselves require TLS. The manifest uses nats:// and no CA setting. Database TLS/roles and broker tenant ACLs are not supplied. This is not evidence that actual production traffic is plaintext.

**Expected behavior.** Staging handshakes prove certificate/hostname validation and fail on a wrong CA or downgraded endpoint. The API cannot modify outbox delivery bookkeeping/DDL and the worker cannot mutate consent/receipts except as explicitly needed by its role. Tenant consumer credentials cannot subscribe to another tenant's data; header tenant metadata alone is not treated as an ACL.

**Detailed implementation approach.** Specify and verify every trust boundary: ingress/API, API/JWKS, API-worker/YSQL and worker/NATS. Require authenticated TLS or an approved equivalent channel; verify hostname/CA behavior. Separate migration, API and worker database roles; restrict broker subjects/stream administration and tenant consumer access. Reject development modes in production deployment policy.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for Security/platform operators. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Staging handshakes prove certificate/hostname validation and fail on a wrong CA or downgraded endpoint.
2. The API cannot modify outbox delivery bookkeeping/DDL and the worker cannot mutate consent/receipts except as explicitly needed by its role.
3. Tenant consumer credentials cannot subscribe to another tenant's data; header tenant metadata alone is not treated as an ACL.

**Tests and verification to deliver.** Successful trusted TLS and negative invalid CA/hostname/plaintext attempts on every required hop; least-privilege SQL/NATS/cluster authorization and credentials/certificate rotation drill.

**Estimate dependency.** The stated range assumes access to the target staging services and platform owners. Missing provisioning, provider approvals, quotas or infrastructure design require separate discovery estimates; do not substitute local Memory or single-node fixtures for the required target evidence.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-019](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-019) is closed or explicitly dispositioned by its owner and [G-019](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-019) records the result, source commit, environment, timestamp and reviewer.

## T-020

**Existing NATS streams are accepted without checking durability, subjects or retention**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-020](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-020), [AT-020](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-020), [G-020](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-020) |
| Accountable delivery role | Messaging/SRE owner |
| Complexity / estimate | Medium; 2–4 provisional person-days |
| Dependencies | [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019) |
| Product/architecture decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13) |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | MOD-06 — Outbox worker; MOD-08 — Build, CI and deployment |
| Relevant current files | `cmd/outbox-worker/main.go:137-146`; `cmd/outbox-worker/main.go:155-169`; `deploy/outbox-worker.yaml:45-50` |

**Current behavior and evidence of the gap.** ensureStream returns immediately for any existing stream name. It does not verify subject coverage, storage, replicas, dedup window or retention. Creation has finite10-minute dedup but no explicit size/age/message limits. A stale/misconfigured stream can contradict the documented three-replica baseline.

**Expected behavior.** An existing mismatched stream produces an actionable blocked-startup/rollout result. Approved retention/replica/storage/subject settings are verified in staging and attached to release evidence. Capacity exhaustion and retry beyond dedup horizon follow the documented at-least-once consumer contract.

**Detailed implementation approach.** Move stream ownership to a controlled provisioning role or validate its configuration against the approved contract at startup. Define retention/capacity/discard policy, dedup horizon, subject permissions and safe change procedures. Alert on configuration drift and publish rejections; avoid silently altering shared broker resources from an application worker.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for Messaging/platform operators. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. An existing mismatched stream produces an actionable blocked-startup/rollout result.
2. Approved retention/replica/storage/subject settings are verified in staging and attached to release evidence.
3. Capacity exhaustion and retry beyond dedup horizon follow the documented at-least-once consumer contract.

**Tests and verification to deliver.** Start against pre-existing stream with wrong subject/replicas/retention/dedup limits, plus broker-node failure and restart; prove explicit drift behavior and retained acknowledged messages.

**Estimate dependency.** The stated range assumes access to the target staging services and platform owners. Missing provisioning, provider approvals, quotas or infrastructure design require separate discovery estimates; do not substitute local Memory or single-node fixtures for the required target evidence.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-020](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-020) is closed or explicitly dispositioned by its owner and [G-020](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-020) records the result, source commit, environment, timestamp and reviewer.

## T-021

**Operators cannot reliably detect stalled consent propagation or diagnose request failures**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-021](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-021), [AT-021](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-021), [G-021](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-021) |
| Accountable delivery role | SRE + backend engineer |
| Complexity / estimate | Medium; 3–5 provisional person-days |
| Dependencies | [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015) |
| Product/architecture decisions | [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | MOD-01 — HTTP API; MOD-06 — Outbox worker; MOD-08 — Build, CI and deployment |
| Relevant current files | `cmd/outbox-worker/main.go:206-231`; `cmd/server/main.go:59-98`; `deploy/outbox-worker.yaml:34-72`; `deploy/k8s.yaml:65-81` |

**Current behavior and evidence of the gap.** Logging is basic process/error text. There are no request traces, API latency/error metrics, worker progress probes, outbox-age alerts or dashboards. Worker claim/publish failure loops can persist without an owned operational signal; retry attempts never enter a bounded quarantine/review workflow.

**Expected behavior.** Simulated database/broker/IdP failures and a poison event trigger the expected operator signal with a traceable cause. Dashboard distinguishes API health from consumer propagation lag. Operators can quarantine/replay an event safely without exposing tokens or recording unbounded subject identifiers in metrics.

**Detailed implementation approach.** Instrument request counts/latency, decision reasons, auth failures, database pool saturation, JWKS refresh and outbox lag/attempts/publish acknowledgements. Add worker progress/health signals, dashboards, alert routing and runbooks with privacy-safe labels. Define poison-event handling and manual replay authorization while preserving durable evidence.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for SRE, support, privacy operations. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Simulated database/broker/IdP failures and a poison event trigger the expected operator signal with a traceable cause.
2. Dashboard distinguishes API health from consumer propagation lag.
3. Operators can quarantine/replay an event safely without exposing tokens or recording unbounded subject identifiers in metrics.

**Tests and verification to deliver.** Trigger request failure, DB saturation, broker outage, oldest-event growth, poison event and stopped worker; prove alert delivery/ownership and correlation-based diagnosis within approved targets.

**Estimate dependency.** The stated range assumes access to the target staging services and platform owners. Missing provisioning, provider approvals, quotas or infrastructure design require separate discovery estimates; do not substitute local Memory or single-node fixtures for the required target evidence.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-021](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-021) is closed or explicitly dispositioned by its owner and [G-021](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-021) records the result, source commit, environment, timestamp and reviewer.

## T-022

**Backup, restore, retention and recovery procedures are not production-evidenced**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-022](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-022), [AT-022](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-022), [G-022](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-022) |
| Accountable delivery role | DBA/SRE + privacy owner |
| Complexity / estimate | High; 3–7 provisional person-days |
| Dependencies | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020) |
| Product/architecture decisions | [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | MOD-07 — Schema lifecycle |
| Relevant current files | `README.md:51`; `migrations/ROLLBACK.md:3-5`; `migrations/002_policy_governance.sql:91-110` |

**Current behavior and evidence of the gap.** README delegates TLS, placement, backup/PITR and recovery to the target YSQL universe. No actual backup schedule, successful restore, recovery objectives or retention policy is present. ROLLBACK refers to a PostgreSQL backup despite YSQL being the intended production system; compatibility of the prior image is asserted but untested here.

**Expected behavior.** A restore into an isolated target-version universe meets the approved RPO/RTO and verifies representative receipts, decisions and outbox replay. A prior-image rollback and forward-correction drill retain evidence and do not resurrect withdrawn permission. Retention/deletion and recovery responsibilities are approved and documented; no legal-compliance claim rests on schema alone.

**Detailed implementation approach.** Approve retention/RPO/RTO and assign owners. Produce a target-YSQL backup/PITR and restore runbook, with encrypted access-controlled evidence storage and tests. Reconcile restored consent/receipt/audit/idempotency/outbox state with already delivered broker events. Define lawful evidence retention/deletion procedures that do not bypass immutable triggers ad hoc.

**Database work.** Review `All durable state and broker events`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for DBA/SRE/privacy owner. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. A restore into an isolated target-version universe meets the approved RPO/RTO and verifies representative receipts, decisions and outbox replay.
2. A prior-image rollback and forward-correction drill retain evidence and do not resurrect withdrawn permission.
3. Retention/deletion and recovery responsibilities are approved and documented; no legal-compliance claim rests on schema alone.

**Tests and verification to deliver.** Isolated target-YSQL backup/PITR restore, receipt/hash and preference verification, idempotency/outbox/broker reconciliation and prior-image rollback/forward-correction; measure agreed RPO/RTO.

**Estimate dependency.** The stated range assumes access to the target staging services and platform owners. Missing provisioning, provider approvals, quotas or infrastructure design require separate discovery estimates; do not substitute local Memory or single-node fixtures for the required target evidence.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-022](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-022) is closed or explicitly dispositioned by its owner and [G-022](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-022) records the result, source commit, environment, timestamp and reviewer.

## T-023

**Named NVUTO product integrations have no end-to-end verification in this repository**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-023](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-023), [AT-023](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-023), [G-023](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-023) |
| Accountable delivery role | Integration lead + each approved consumer owner |
| Complexity / estimate | High; 4–10 provisional person-days |
| Dependencies | [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020), [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030) |
| Product/architecture decisions | [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07), [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | MOD-01 — HTTP API; MOD-06 — Outbox worker |
| Relevant current files | `README.md:9-11`; `cmd/server/main.go:399-409`; `cmd/outbox-worker/main.go:102-113` |

**Current behavior and evidence of the gap.** Engage, Service, Contact Center, Customer Portal, CRM, Conversations, AI and Integration Hub are named consumers only. No actual client, frontend, subscription, contact delivery or fail-closed communication test is supplied. Building this service does not prove those products respect consent.

**Expected behavior.** Each approved launch consumer has a reproducible integration/UAT record using real staging wiring. No-consent, denial, withdrawal, suppression, expiry and service-unavailable cases prevent disallowed communication. A subject change is reflected after page reload and consumer restart, within the approved propagation budget.

**Detailed implementation approach.** Approve the launch consumer set and assign an owner to each integration. Publish identity, HTTP and event contracts, map canonical Customer Graph IDs, and run each launch workflow from actual user/application action through YSQL and messaging to the consumer outcome. Define timeout/error handling and the pre-send consent check. Non-launch consumers remain explicitly deferred.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for All launch product teams. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Each approved launch consumer has a reproducible integration/UAT record using real staging wiring.
2. No-consent, denial, withdrawal, suppression, expiry and service-unavailable cases prevent disallowed communication.
3. A subject change is reflected after page reload and consumer restart, within the approved propagation budget.

**Tests and verification to deliver.** Per approved consumer, actual staging user/app action→JWT→API→YSQL→event→consumer outcome; denied/missing/withdrawn/suppressed/expired/unavailable cases and restart/page-reload persistence.

**Discovery boundary.** First obtain each selected consumer repository, send/processing boundary, identity mapping, API client and subscription implementation. Estimate and create a concrete per-consumer child work item after inspection. The 4–10 day range covers contract discovery and one pilot with a ready test environment; it does not estimate all eight unseen applications.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-023](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-023) is closed or explicitly dispositioned by its owner and [G-023](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-023) records the result, source commit, environment, timestamp and reviewer.

## T-024

**Preference discovery, registry reads and a machine-readable client contract are absent**

| Planning field | Specification |
| --- | --- |
| Release priority | P2; scope/deferral decision required; may block dependent tickets |
| Finding / acceptance / release gate | [F-024](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-024), [AT-024](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-024), [G-024](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-024) |
| Accountable delivery role | API architect + portal/admin owner |
| Complexity / estimate | High; 3–6 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07) |
| Affected APIs | All business APIs; proposed read APIs require approval |
| Dependent modules | MOD-01 — HTTP API; MOD-03 — Domain contracts |
| Relevant current files | `README.md:5-11`; `cmd/server/main.go:386-410`; `internal/domain/model.go:15-125` |

**Current behavior and evidence of the gap.** The service exposes writes, evaluation and receipt-by-ID, but no subject preference list, consent-by-ID GET, registry list, suppression list or audit export route. External portals may rely on another owned projection; none is provided. OpenAPI/SDK examples and errors/pagination contracts are also absent.

**Expected behavior.** A portal can load the approved current preferences and necessary policy/notice data through a documented authorized source. Contract tests validate request/response models for every launch endpoint, including negative authorization cases. Pagination/filtering and origin/session behavior are explicitly specified where needed; absent frontend is not misrepresented as an implemented screen.

**Detailed implementation approach.** Choose the authoritative read owner for the self-service/admin experiences. If Consent owns it, specify tenant/subject-authorized read APIs with bounded pagination and exact frontend DTOs; otherwise document and prove the external projection. Publish an OpenAPI contract for all11 existing operations and approved additions, including error/retry/idempotency and CORS/gateway requirements.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for Portal/admin frontend and client engineers. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. A portal can load the approved current preferences and necessary policy/notice data through a documented authorized source.
2. Contract tests validate request/response models for every launch endpoint, including negative authorization cases.
3. Pagination/filtering and origin/session behavior are explicitly specified where needed; absent frontend is not misrepresented as an implemented screen.

**Tests and verification to deliver.** Contract conformance for existing11 operations and approved reads/projection; tenant/subject authorization, current-state initial load, bounded pagination/filtering and client error/retry handling.

**Discovery boundary.** Determine whether an existing Customer Portal/admin projection already owns initial reads. If it does, document and test that contract; if it does not, approve the smallest required read API. No new frontend framework or account system is authorized by this ticket.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-024](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-024) is closed or explicitly dispositioned by its owner and [G-024](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-024) records the result, source commit, environment, timestamp and reviewer.

## T-025

**Idempotency replay semantics change with policy and current consent state**

| Planning field | Specification |
| --- | --- |
| Release priority | P2; scope/deferral decision required; may block dependent tickets |
| Finding / acceptance / release gate | [F-025](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-025), [AT-025](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-025), [G-025](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-025) |
| Accountable delivery role | Backend/API engineer |
| Complexity / estimate | Medium; 2–4 provisional person-days |
| Dependencies | [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Product/architecture decisions | [D-11](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-11) |
| Affected APIs | API-04 |
| Dependent modules | MOD-04 — Consent service; MOD-05 — Persistence |
| Relevant current files | `internal/service/idempotency.go:17-30`; `internal/service/idempotency.go:55-81`; `internal/store/idempotency.go:27-41`; `internal/store/idempotency.go:67-83` |

**Current behavior and evidence of the gap.** Policy is revalidated before looking up the key, so a legitimate replay can fail after a policy change. The ledger returns the current consent row, not the original response, so a withdrawn record can replace the original granted response. The fingerprint includes actor and uses timestamp representations; retention is undefined.

**Expected behavior.** Same-key replay after withdrawal, policy change, actor rotation and time-offset normalization matches the published contract. Changed semantic payload returns409 and concurrent identical writes create exactly one consent/receipt/audit/outbox set on YSQL. Retention, cleanup and consumer use of evaluation instead of stale create-response status are documented.

**Detailed implementation approach.** Specify whether a key guarantees original response replay or only one resource creation, how actor/token rotation affects it, and the retention/security namespace. Resolve authorized existing-key replay before current-policy creation checks where the approved contract requires it. Canonicalize semantic times and store a response snapshot if exact replay is promised; do not allow replay to bypass current subject authorization.

**Database work.** Review `consent_idempotency`, `consents`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Same-key replay after withdrawal, policy change, actor rotation and time-offset normalization matches the published contract.
2. Changed semantic payload returns409 and concurrent identical writes create exactly one consent/receipt/audit/outbox set on YSQL.
3. Retention, cleanup and consumer use of evaluation instead of stale create-response status are documented.

**Tests and verification to deliver.** Identical-key replay after policy/actor/token/time representation/resource status changes, semantic conflict409, simultaneous writes and retention boundary on real YSQL.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-025](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-025) is closed or explicitly dispositioned by its owner and [G-025](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-025) records the result, source commit, environment, timestamp and reviewer.

## T-026

**Memory-store composite keys collide across tenants and diverge from durable behavior**

| Planning field | Specification |
| --- | --- |
| Release priority | P2; scope/deferral decision required; may block dependent tickets |
| Finding / acceptance / release gate | [F-026](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-026), [AT-026](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-026), [G-026](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-026) |
| Accountable delivery role | Backend maintainer |
| Complexity / estimate | Medium; 1–3 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | None identified as a prerequisite |
| Affected APIs | Development-mode APIs |
| Dependent modules | MOD-01 — HTTP API; MOD-05 — Persistence |
| Relevant current files | `internal/store/memory.go:60-74`; `internal/store/memory.go:145-168`; `internal/store/memory.go:218-235`; `internal/store/idempotency.go:20-41`; `cmd/server/main.go:425-430` |

**Current behavior and evidence of the gap.** Unescaped colon concatenation permits a lookup in tenant audit-parent to return a record from tenant audit-parent:child when the ID is prefixed accordingly. This was reproduced through Service. Memory suppression updates also preserve the old ID but do not repair audit/outbox IDs, and memory events skip durable normalization.

**Expected behavior.** Delimiter-rich tenant/subject/code/ID combinations cannot cross boundaries. Memory and YSQL contract tests agree on suppression IDs, event metadata, concurrency and immutable values. Production configuration tests reject ALLOW_MEMORY_STORE.

**Detailed implementation approach.** Use typed composite keys/nested maps and defensive tenant checks. Make memory suppression/event/immutability semantics match the public repository contract, clone mutable data where required, and keep the idempotency ledger owned by each Memory instance. Retain the explicit development opt-in and enforce its prohibition in approved production configuration.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for Developers and local test tenants. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Delimiter-rich tenant/subject/code/ID combinations cannot cross boundaries.
2. Memory and YSQL contract tests agree on suppression IDs, event metadata, concurrency and immutable values.
3. Production configuration tests reject ALLOW_MEMORY_STORE.

**Tests and verification to deliver.** Typed-key collision property cases and shared adapter contract suite, suppression event IDs, isolation across Memory instances and mutable-map aliasing; deployment ban on Memory.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-026](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-026) is closed or explicitly dispositioned by its owner and [G-026](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-026) records the result, source commit, environment, timestamp and reviewer.

## T-027

**Input normalization and validation leave ambiguous or surprising API behavior**

| Planning field | Specification |
| --- | --- |
| Release priority | P2; scope/deferral decision required; may block dependent tickets |
| Finding / acceptance / release gate | [F-027](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-027), [AT-027](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-027), [G-027](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-027) |
| Accountable delivery role | API/backend engineer |
| Complexity / estimate | Medium; 2–4 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | [D-02](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-02), [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05) |
| Affected APIs | All JSON-consuming business APIs |
| Dependent modules | MOD-01 — HTTP API; MOD-04 — Consent service |
| Relevant current files | `cmd/server/main.go:78-85`; `internal/service/service.go:79-103`; `internal/service/service.go:131-139`; `internal/service/service.go:202-234`; `internal/service/service.go:264-265` |

**Current behavior and evidence of the gap.** A valid JSON object followed by a second object is accepted. Boolean active defaults false on API-decoded structs despite SQL defaults true; evidence hash format is not checked; consent expiry may precede creation; challenge lifetime and many identifiers have no semantic maximum. Denial capture also requires the same current policy/evidence checks as grant.

**Expected behavior.** Trailing JSON and malformed/oversized/wrong-type inputs return stable4xx without mutation. Omitted active, invalid expiry and invalid evidence produce documented outcomes rather than accidental defaults. Deny/withdraw error paths allow the approved opt-out journey under policy-change/unavailable conditions.

**Detailed implementation approach.** Define an explicit input contract. Require exactly one JSON document; implement approved content-type/body-size behavior and field bounds. Distinguish omitted booleans from explicit false where needed. Validate evidence format and timestamp relationships. Decide how a subject can deny/withdraw when policy is unavailable without accidentally authorizing consent.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Trailing JSON and malformed/oversized/wrong-type inputs return stable4xx without mutation.
2. Omitted active, invalid expiry and invalid evidence produce documented outcomes rather than accidental defaults.
3. Deny/withdraw error paths allow the approved opt-out journey under policy-change/unavailable conditions.

**Tests and verification to deliver.** Exactly one JSON document, body limits/types/content type, omitted booleans, evidence/time bounds, normalization and safe opt-out failure journeys; assert no mutation on rejected requests.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-027](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-027) is closed or explicitly dispositioned by its owner and [G-027](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-027) records the result, source commit, environment, timestamp and reviewer.

## T-028

**Database constraints do not enforce several tenant and state relationships**

| Planning field | Specification |
| --- | --- |
| Release priority | P2; scope/deferral decision required; may block dependent tickets |
| Finding / acceptance / release gate | [F-028](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-028), [AT-028](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-028), [G-028](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-028) |
| Accountable delivery role | DBA/backend engineer |
| Complexity / estimate | Medium; 2–5 provisional person-days |
| Dependencies | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Product/architecture decisions | None identified as a prerequisite |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | MOD-07 — Schema lifecycle |
| Relevant current files | `migrations/001_init.sql:1-45`; `migrations/002_policy_governance.sql:24-89`; `migrations/003_governed_idempotency.sql:1-11` |

**Current behavior and evidence of the gap.** Only receipt.consent_id has a foreign key, and it does not include tenant. Receipt_id on consent and consent_id on idempotency have no FK; registry references are logical. Suppression field shapes, evidence JSON structure and temporal/state relations are mostly enforced only in service code. SQL queries do use tenant parameters.

**Expected behavior.** Representative cross-tenant receipt/idempotency reference attempts and invalid state shapes are rejected by the approved layer. Existing-data validation reports zero unexplained violations before constraints are enforced. Fresh install and upgrade preserve performance and transactional write order under YSQL.

**Detailed implementation approach.** Define invariant ownership and add appropriate composite tenant keys/FKs/checks after read-only data validation. Avoid inappropriate FKs for intentionally external subjects or polymorphic audit targets. Reconcile existing violations additively, test insertion ordering/deferrability on YSQL, and document application-vs-database guarantees.

**Database work.** Review `consents`, `consent_receipts`, `consent_idempotency`, `consent_suppressions`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Representative cross-tenant receipt/idempotency reference attempts and invalid state shapes are rejected by the approved layer.
2. Existing-data validation reports zero unexplained violations before constraints are enforced.
3. Fresh install and upgrade preserve performance and transactional write order under YSQL.

**Tests and verification to deliver.** Read-only violation census, representative direct invalid inserts, composite tenant references, constrained transaction ordering and post-upgrade write/performance tests on YSQL.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-028](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-028) is closed or explicitly dispositioned by its owner and [G-028](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-028) records the result, source commit, environment, timestamp and reviewer.

## T-029

**Immutable audit rows cannot reconstruct policy and suppression history**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-029](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-029), [AT-029](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-029), [G-029](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-029) |
| Accountable delivery role | Data/backend engineer + privacy owner |
| Complexity / estimate | Medium; 3–5 provisional person-days |
| Dependencies | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007) |
| Product/architecture decisions | [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) |
| Affected APIs | API-01, API-02, API-06, Dependent evaluation and authorized history/export workflows |
| Dependent modules | MOD-03 — Domain contracts; MOD-04 — Consent service; MOD-05 — Persistence |
| Relevant current files | `internal/domain/model.go:132-139`; `internal/service/service.go:58-59`; `internal/service/service.go:71-72`; `internal/service/service.go:232-233`; `internal/store/postgres.go:154-160`; `internal/store/postgres.go:307-315` |

**Current behavior and evidence of the gap.** Audit rows contain ID, tenant, actor, action, consent_id and time only. Mutable registries and suppression rows overwrite values; audit rows do not retain before/after rules or resource type/revision. A row-level immutability trigger prevents changes but does not make missing history reconstructable.

**Expected behavior.** An auditor can reconstruct the exact policy and suppression states used for a historical consent decision. Updates retain actor, resource type, revision, time and approved change reason without overwriting historical values. Audit exports are tenant/role-scoped and the normal app role cannot alter history.

**Detailed implementation approach.** Add typed resource identifiers, immutable policy/suppression versions or complete before/after change records, correlation/source and reason where approved. Provide an authorized export/query path or owned external audit projection. Apply least privilege to the append-only tables and define redaction/retention with the privacy owner.

**Database work.** Review `consent_audit_events`, `consent_purposes`, `consent_lawful_bases`, `consent_suppressions`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for Privacy administrator, auditor, authorized support. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. An auditor can reconstruct the exact policy and suppression states used for a historical consent decision.
2. Updates retain actor, resource type, revision, time and approved change reason without overwriting historical values.
3. Audit exports are tenant/role-scoped and the normal app role cannot alter history.

**Tests and verification to deliver.** Historical policy/suppression reconstruction, role/tenant-scoped investigator export and app-role UPDATE/DELETE denial; verify all relevant before/after and revision fields.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-029](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-029) is closed or explicitly dispositioned by its owner and [G-029](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-029) records the result, source commit, environment, timestamp and reviewer.

## T-030

**Mixed self-service and reader roles can authorize tenant-wide writes**

| Planning field | Specification |
| --- | --- |
| Release priority | P1; mandatory for affected scope |
| Finding / acceptance / release gate | [F-030](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-030), [AT-030](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-030), [G-030](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-030) |
| Accountable delivery role | Identity/security + privacy owner |
| Complexity / estimate | Medium; 2–4 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) |
| Affected APIs | All authenticated APIs |
| Dependent modules | MOD-01 — HTTP API; MOD-02 — Authentication |
| Relevant current files | `cmd/server/main.go:109-118`; `cmd/server/main.go:241-258`; `internal/auth/auth.go:27-31`; `internal/auth/auth.go:180-199` |

**Current behavior and evidence of the gap.** The developer role is accepted in production JWTs if issued, service is read-only, and privileged roles override a self_service restriction on a mixed-role token. Specifically, self_service plus service or consent_reader admits writes through self_service and then bypasses the subject restriction through the read role, allowing tenant-wide subject writes; this is source-traced, not dynamically probed. A self_service user may set active=false on their own global/channel suppression regardless of who created it. No finer policy distinguishes user preferences from administrative/legal suppression.

**Expected behavior.** Mixed self_service plus service or consent_reader tokens cannot write another subject solely by combining separate role checks; any broader writer authority must be explicitly granted and tested. A signed role-permission matrix matches production IdP mappings and backend tests for all seven roles. Self-service cannot clear a protected administrative suppression if the approved policy distinguishes that class. Production deployment disallows development authentication/roles according to the approved identity contract.

**Detailed implementation approach.** Authorize the requested action and subject scope together, preventing a tenant-wide reader role from expanding a self-service writer into tenant-wide writes. Until fixed, any proposed deployment exclusion must be enforced and tested in the issuer/configuration, not assumed. Approve exact Authentik role/claim mappings and suppression authority. Decide whether to separate user preference opt-outs from administrative/legal blocks and how mixed roles behave. Add provenance/owner fields only where that approved rule requires them; test every role and mixed-role combination against every operation and resource boundary, including self_service plus service or consent_reader. Keep identity account signup/recovery external as README states.

**Database work.** Review `consent_suppressions`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for Identity owner, privacy owner, all seven JWT roles. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. This ticket must preserve safe-deny behavior on uncertainty. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Mixed self_service plus service or consent_reader tokens cannot write another subject solely by combining separate role checks; any broader writer authority must be explicitly granted and tested.
2. A signed role-permission matrix matches production IdP mappings and backend tests for all seven roles.
3. Self-service cannot clear a protected administrative suppression if the approved policy distinguishes that class.
4. Production deployment disallows development authentication/roles according to the approved identity contract.

**Tests and verification to deliver.** All seven roles and mixed-role combinations against every endpoint, production claim mapping, canonical subjects, protected suppression clearing under the approved policy and forbidden development auth.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-030](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-030) is closed or explicitly dispositioned by its owner and [G-030](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-030) records the result, source commit, environment, timestamp and reviewer.

## T-031

**Legacy Record and Upsert helpers preserve a non-atomic governance bypass path**

| Planning field | Specification |
| --- | --- |
| Release priority | P2; scope/deferral decision required; may block dependent tickets |
| Finding / acceptance / release gate | [F-031](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-031), [AT-031](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-031), [G-031](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-031) |
| Accountable delivery role | Backend maintainer |
| Complexity / estimate | Low–medium; 1–2 provisional person-days |
| Dependencies | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) |
| Product/architecture decisions | None identified as a prerequisite |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | MOD-04 — Consent service; MOD-05 — Persistence |
| Relevant current files | `internal/service/service.go:152-178`; `internal/store/postgres.go:125-135`; `internal/store/memory.go:15-35`; `internal/service/service_test.go:10-90` |

**Current behavior and evidence of the gap.** The legacy Record helper writes consent then independently appends audit/outbox while ignoring their errors. It bypasses governed policy/receipt rules. Current HTTP /v1/consents uses RecordGoverned, so this is not an identified public-route transaction bypass; future reuse and legacy tests can hide risk.

**Expected behavior.** No production entry point can create consent while silently losing audit/outbox/receipt effects. Compatibility consumers are inventoried or explicitly absent before removing exported behavior. Production behavior tests exercise the governed path and legacy migration tests remain separate.

**Detailed implementation approach.** Remove, restrict or explicitly deprecate the legacy helper and unsafe Upsert surface after confirming any external Go consumers. Route required compatibility behavior through one atomic governed implementation or an explicitly isolated migration adapter. Align tests with production-reachable paths; retain a clear legacy-data migration contract.

**Database work.** Review `consents`, `consent_audit_events`, `consent_outbox_events`. Any new columns/constraints/version records are **proposed additions**, not files that already exist. Produce an additive migration and existing-data validation plan where the accepted design changes persisted state. Verify write ordering, tenant boundaries, receipt/audit immutability, upgrade and rollback compatibility on target YSQL.

**Security considerations.** Preserve tenant and canonical-subject authorization for All service users. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. No production entry point can create consent while silently losing audit/outbox/receipt effects.
2. Compatibility consumers are inventoried or explicitly absent before removing exported behavior.
3. Production behavior tests exercise the governed path and legacy migration tests remain separate.

**Tests and verification to deliver.** Call-site inventory, failure injection for every production write entry, governed-path tests and explicitly separate legacy migration tests; demonstrate no ignored audit/outbox failure.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-031](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-031) is closed or explicitly dispositioned by its owner and [G-031](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-031) records the result, source commit, environment, timestamp and reviewer.

## T-032

**Approved launch requirements, capacity and regional policy decisions are not supplied**

| Planning field | Specification |
| --- | --- |
| Release priority | P2; scope/deferral decision required; may block dependent tickets |
| Finding / acceptance / release gate | [F-032](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-032), [AT-032](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-032), [G-032](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-032) |
| Accountable delivery role | Product/architecture + release owner |
| Complexity / estimate | Medium; 2–4 provisional person-days |
| Dependencies | None; may start independently within an isolated development environment. |
| Product/architecture decisions | [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), [D-03](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-03), [D-04](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-04), [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07), [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10), [D-11](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-11), [D-12](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-12), [D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) |
| Affected APIs | No directly modified API required; verify impacted runtime/integration contract |
| Dependent modules | Cross-cutting product/operational requirements across MOD-01–MOD-08 |
| Relevant current files | `README.md:1-51`; `migrations/ROLLBACK.md:1-5` |

**Current behavior and evidence of the gap.** The attached audit brief expects detailed PRDs/TRDs/flows; the32-file archive contains none. Revenue model, operating regions, exact personas, data scope, capacity/SLOs and acceptance ownership are undocumented. NG appears in tests; it does not establish applicable legal obligations. There is no Git history/commit in the ZIP.

**Expected behavior.** Every launch-critical requirement has an owner, source, acceptance case and implementation/gap link. All release-affecting decisions are recorded and approved; deferred scope is explicit. The release evidence identifies a source commit, approved configuration, SLO/capacity/RPO/RTO and regional privacy review owner.

**Detailed implementation approach.** Review this evidence-based reconstruction with product/privacy/architecture owners, approve or amend the scope and decision register, recover any missing upstream specifications and record the exact source commit. Resolve requirements before expanding frontend, authentication, payment or geographic functionality. Define release owners and measurable nonfunctional targets with explicit approvals.

**Database work.** No schema migration is prescribed merely to close this ticket. If the accepted design requires new persisted metadata, propose its schema, ownership, lifecycle and YSQL migration explicitly before implementation.

**Security considerations.** Preserve tenant and canonical-subject authorization for Product, architecture, privacy and release owners. Keep raw DOI capabilities, authentication material and unnecessary personal evidence out of logs and shared test artifacts. Rejected or retried actions must not create hidden state, receipts, audit or outbox duplicates. Use synthetic subjects and approved isolated environments for the required tests.

**Acceptance criteria.**

1. Every launch-critical requirement has an owner, source, acceptance case and implementation/gap link.
2. All release-affecting decisions are recorded and approved; deferred scope is explicit.
3. The release evidence identifies a source commit, approved configuration, SLO/capacity/RPO/RTO and regional privacy review owner.

**Tests and verification to deliver.** Requirements/decision review with accountable owners, exact upstream source revision, scope exclusions and measurable SLO/region/recovery acceptance references; this is an evidence review, not a unit test.

**Discovery deliverable.** Produce an owner-reviewed requirements and decision record, recover upstream PRDs/TRDs if they exist, obtain the authoritative commit/release baseline, and replace unapproved numerical/region assumptions with documented targets. Remaining uncertain tasks must be re-estimated before scheduling.

**Definition of Done.**

1. The accepted behavior and any linked decision are recorded, with a reviewed code/configuration or documentation change tied to this ticket.
2. Every criterion above has a reproducible result and evidence artifact; required integration cases are executed, not skipped.
3. Relevant original tests and repaired audit regressions pass on the chosen patched toolchain; durable changes have target-YSQL evidence and migration/recovery review.
4. Contracts, setup/runbook instructions and operational implications are updated. No unsupported scope or unverified production claim is introduced.
5. [F-032](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-032) is closed or explicitly dispositioned by its owner and [G-032](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-032) records the result, source commit, environment, timestamp and reviewer.
