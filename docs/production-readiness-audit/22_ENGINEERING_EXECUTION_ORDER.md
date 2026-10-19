# 22 — Engineering execution order

**Product:** NVUTO Consent  
**Audit date:** 19 October 2026 (command timestamps in UTC; raw log offsets preserved)  
**Source:** supplied ZIP; commit and branch UNKNOWN  
**Disposition:** documentation-only audit; original source preserved.

## How to use this sequence

Follow this guide with the detailed backlog (report16), acceptance catalog (report14) and go-live checklist (report19). Each ticket below retains its own exact predecessors and open decisions. Six dependency waves contain all32 tickets. Work within a wave may proceed in parallel when it touches separate code/configuration and the necessary owner decisions are available; completing a wave is not a substitute for the individual evidence requirements.

**Do not deploy the audited baseline.** First obtain the authoritative commit and work in an isolated development branch/environment. Preserve the before-fix audit logs. The original source has not been patched by this audit.

## Step0 — Establish ownership, scope and a repeatable baseline

Read reports00,01,02,18 and21. Run the documented local baseline and isolated probes; understand which current failures are safety defects and which expectations require a decision. Obtain the source commit and approved test access. Start T-032/AT-032 and T-024/AT-024 discovery, and resolve the key decisions needed by your chosen workstream. T-030/AT-030 includes an immediate P1 mixed-role authorization issue; the full role/suppression contract still needs D-06/D-10.

Before changing persisted consent ordering, select D-03 and D-04. Before canonical evidence migration, select D-05. Before DOI delivery, select D-02. Before final platform/consumer acceptance, select D-01/D-07/D-08/D-09/D-10/D-13/D-14. The independent fixes and failure tests do not require guessing these decisions.


## Step1 — Start independent safety and verification work

**Parallel work available.** The following tasks have no unfinished predecessor outside the preceding waves. Product decisions and actual access still apply.

| Ticket / accountable role | Concrete output | Required predecessors | Acceptance / release gate |
| --- | --- | --- | --- |
| [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001) — Backend/data lead + privacy owner | Older double-opt-in challenges can restore permission after a later denial or withdrawal | None; may start independently within an isolated development environment. | [AT-001](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-001), [G-001](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-001) |
| [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002) — API/security engineer | Double-opt-in confirmation omits the self-service customer boundary | None; may start independently within an isolated development environment. | [AT-002](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-002), [G-002](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-002) |
| [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004) — Backend/data engineer | Receipt hashes do not survive the actual pgx timestamp precision round trip | None; may start independently within an isolated development environment. | [AT-004](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-004), [G-004](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-004) |
| [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009) — Worker/backend engineer + SRE | Outbox lease completion and retry are not fenced to the current worker | None; may start independently within an isolated development environment. | [AT-009](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-009), [G-009](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-009) |
| [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011) — Backend maintainer + security engineer | Pinned toolchain and dependencies fail the current vulnerability gate | None; may start independently within an isolated development environment. | [AT-011](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-011), [G-011](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-011) |
| [T-014](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-014) — Identity/security engineer | Unknown JWT key IDs trigger an unbounded JWKS refresh path | None; may start independently within an isolated development environment. | [AT-014](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-014), [G-014](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-014) |
| [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) — CI/backend engineer | CI omits durable idempotency coverage and migration 003 upgrade verification | None; may start independently within an isolated development environment. | [AT-017](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-017), [G-017](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-017) |
| [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024) — API architect + portal/admin owner | Preference discovery, registry reads and a machine-readable client contract are absent | None; may start independently within an isolated development environment. | [AT-024](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-024), [G-024](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-024) |
| [T-026](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-026) — Backend maintainer | Memory-store composite keys collide across tenants and diverge from durable behavior | None; may start independently within an isolated development environment. | [AT-026](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-026), [G-026](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-026) |
| [T-027](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-027) — API/backend engineer | Input normalization and validation leave ambiguous or surprising API behavior | None; may start independently within an isolated development environment. | [AT-027](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-027), [G-027](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-027) |
| [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030) — Identity/security + privacy owner | Mixed self-service and reader roles can authorize tenant-wide writes | None; may start independently within an isolated development environment. | [AT-030](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-030), [G-030](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-030) |
| [T-032](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-032) — Product/architecture + release owner | Approved launch requirements, capacity and regional policy decisions are not supplied | None; may start independently within an isolated development environment. | [AT-032](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-032), [G-032](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-032) |

**Checkpoint before dependent completion.** Have reviewed regression fixes or designs for stale challenges, customer/action scope, receipt serialization, worker ownership, patched dependencies, JWKS refresh and CI omissions. Establish read/identity/requirements owners. Do not concurrently invent different preference or receipt schemas.

**Per-task working loop.** Read its finding and source paths; record any required decision; add or promote the relevant failing regression in a working branch; implement the smallest coherent reviewed change; run its specific acceptance at the correct storage/provider layer; review schema/contract/operational impact; attach evidence and update the gate. Retain the original counterexample unchanged in this audit directory.

## Step2 — Integrate trusted DOI, evidence, time and runtime boundaries

**Parallel work available.** The following tasks have no unfinished predecessor outside the preceding waves. Product decisions and actual access still apply.

| Ticket / accountable role | Concrete output | Required predecessors | Acceptance / release gate |
| --- | --- | --- | --- |
| [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003) — Backend + trusted delivery integration owner | Self-service receives the secret needed to bypass independent double-opt-in confirmation | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002) | [AT-003](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-003), [G-003](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-003) |
| [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005) — Backend/data engineer + privacy owner | Notice and receipt records do not bind the accepted notice content and full decision | [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004) | [AT-005](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-005), [G-005](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-005) |
| [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006) — Backend/data lead + product owner | Evaluation time and current-state ordering do not implement a deterministic consent timeline | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001) | [AT-006](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-006), [G-006](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-006) |
| [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012) — Backend/data engineer + SRE | Database work cannot follow request cancellation or a bounded transaction budget | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009) | [AT-012](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-012), [G-012](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-012) |
| [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015) — Backend + SRE | Readiness can report healthy while authentication or required schema is unusable | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) | [AT-015](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-015), [G-015](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-015) |

**Checkpoint before dependent completion.** Bind delivery to the safe generation/authorization model; establish exact notice proof and supported evaluation time; thread cancellation and make auth/schema readiness meaningful. The historical option must include the shared immutable timeline foundation in T-006 and receive a revised estimate before dependent work.

**Per-task working loop.** Read its finding and source paths; record any required decision; add or promote the relevant failing regression in a working branch; implement the smallest coherent reviewed change; run its specific acceptance at the correct storage/provider layer; review schema/contract/operational impact; attach evidence and update the gate. Retain the original counterexample unchanged in this audit directory.

## Step3 — Apply policy transitions and produce stable runtime/release contracts

**Parallel work available.** The following tasks have no unfinished predecessor outside the preceding waves. Product decisions and actual access still apply.

| Ticket / accountable role | Concrete output | Required predecessors | Acceptance / release gate |
| --- | --- | --- | --- |
| [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007) — Backend policy engineer + privacy owner | Stronger policy requirements are not re-enforced against existing grants or pending confirmations | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006) | [AT-007](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-007), [G-007](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-007) |
| [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013) — API engineer | Unexpected database errors are returned as 422 with internal error text | [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012) | [AT-013](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-013), [G-013](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-013) |
| [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018) — Platform/release engineer | Release packaging and deployment are not reproducible from the supplied manifests | [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) | [AT-018](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-018), [G-018](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-018) |

**Checkpoint before dependent completion.** Policy changes follow explicit versioned eligibility; clients receive distinguishable safe errors; release packaging uses the patched tested components. Claims of successful deployment still wait for actual target evidence.

**Per-task working loop.** Read its finding and source paths; record any required decision; add or promote the relevant failing regression in a working branch; implement the smallest coherent reviewed change; run its specific acceptance at the correct storage/provider layer; review schema/contract/operational impact; attach evidence and update the gate. Retain the original counterexample unchanged in this audit directory.

## Step4 — Reconcile legacy data and complete event, identity and history behavior

**Parallel work available.** The following tasks have no unfinished predecessor outside the preceding waves. Product decisions and actual access still apply.

| Ticket / accountable role | Concrete output | Required predecessors | Acceptance / release gate |
| --- | --- | --- | --- |
| [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008) — DBA/data engineer + privacy owner | Upgraded legacy grants bypass all governed-policy checks when notice_version is blank | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007) | [AT-008](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-008), [G-008](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-008) |
| [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010) — API/event architect + consumer owners | Events lack the ordering and reconciliation contract needed to keep consumers consistent | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009) | [AT-010](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-010), [G-010](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-010) |
| [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019) — Platform/security + DBA + identity owner | Transport security and least-privilege service identities are not established | [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018) | [AT-019](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-019), [G-019](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-019) |
| [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021) — SRE + backend engineer | Operators cannot reliably detect stalled consent propagation or diagnose request failures | [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015) | [AT-021](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-021), [G-021](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-021) |
| [T-025](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-025) — Backend/API engineer | Idempotency replay semantics change with policy and current consent state | [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) | [AT-025](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-025), [G-025](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-025) |
| [T-029](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-029) — Data/backend engineer + privacy owner | Immutable audit rows cannot reconstruct policy and suppression history | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007) | [AT-029](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-029), [G-029](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-029) |

**Checkpoint before dependent completion.** Legacy row classes have approved eligibility and migrations; events carry reconciliation semantics; service identities/TLS are enforced; critical failures/lag are observable; idempotency and history match approved contracts.

**Per-task working loop.** Read its finding and source paths; record any required decision; add or promote the relevant failing regression in a working branch; implement the smallest coherent reviewed change; run its specific acceptance at the correct storage/provider layer; review schema/contract/operational impact; attach evidence and update the gate. Retain the original counterexample unchanged in this audit directory.

## Step5 — Prove target persistence, stream policy and compatibility

**Parallel work available.** The following tasks have no unfinished predecessor outside the preceding waves. Product decisions and actual access still apply.

| Ticket / accountable role | Concrete output | Required predecessors | Acceptance / release gate |
| --- | --- | --- | --- |
| [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016) — QA/backend + YSQL DBA | Durable API, transaction and multi-node Yugabyte behavior remain unverified | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002), [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) | [AT-016](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-016), [G-016](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-016) |
| [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020) — Messaging/SRE owner | Existing NATS streams are accepted without checking durability, subjects or retention | [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019) | [AT-020](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-020), [G-020](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-020) |
| [T-028](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-028) — DBA/backend engineer | Database constraints do not enforce several tenant and state relationships | [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) | [AT-028](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-028), [G-028](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-028) |
| [T-031](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-031) — Backend maintainer | Legacy Record and Upsert helpers preserve a non-atomic governance bypass path | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017) | [AT-031](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-031), [G-031](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-031) |

**Checkpoint before dependent completion.** Execute the repaired full HTTP/YSQL corpus, test stream drift/HA behavior, validate tenant/state constraints and remove/restrict internal legacy bypasses. Skipped required fixtures are failures of the acceptance process, not passes.

**Per-task working loop.** Read its finding and source paths; record any required decision; add or promote the relevant failing regression in a working branch; implement the smallest coherent reviewed change; run its specific acceptance at the correct storage/provider layer; review schema/contract/operational impact; attach evidence and update the gate. Retain the original counterexample unchanged in this audit directory.

## Step6 — Demonstrate recovery and actual launch-consumer outcomes

**Parallel work available.** The following tasks have no unfinished predecessor outside the preceding waves. Product decisions and actual access still apply.

| Ticket / accountable role | Concrete output | Required predecessors | Acceptance / release gate |
| --- | --- | --- | --- |
| [T-022](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-022) — DBA/SRE + privacy owner | Backup, restore, retention and recovery procedures are not production-evidenced | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020) | [AT-022](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-022), [G-022](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-022) |
| [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023) — Integration lead + each approved consumer owner | Named NVUTO product integrations have no end-to-end verification in this repository | [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020), [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030) | [AT-023](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-023), [G-023](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-023) |

**Checkpoint before dependent completion.** Restore into an isolated target universe and reconcile SQL/outbox/broker state; execute every approved consumer action through actual staging wiring. Retain measured RPO/RTO/propagation results and user-facing persistence/error evidence.

**Per-task working loop.** Read its finding and source paths; record any required decision; add or promote the relevant failing regression in a working branch; implement the smallest coherent reviewed change; run its specific acceptance at the correct storage/provider layer; review schema/contract/operational impact; attach evidence and update the gate. Retain the original counterexample unchanged in this audit directory.

## Step7 — Assemble and review the release evidence

Review every applicable G-001 through G-032 against its matching AT-001 through AT-032 result and requirement/workflow. Every P0/P1 gate must be resolved for the enabled launch scope. A scoped exclusion must identify the omitted feature/consumer, enforced configuration, ownership and remaining risk; lack of access is not exclusion evidence. Record any residual P2 deferral and ensure no required ticket depends on it.

The final release packet must identify the source commit, CI run, patched toolchain, image digests, actual environment and service versions/topology, migration validation, IdP claims/roles/TLS grants, end-to-end consumer outcomes, alert/incident ownership and successful restore/rollback. Confirm no old or mixed-role path can restore unauthorized permission and no stale grant event leaves a consumer allowed after withdrawal. Product, privacy/security, engineering/QA and SRE/release owners should record their actual approval according to the organization's process. This audit provides no production deployment approval.

## Safe parallelization and merge constraints

| Shared area | Coordination rule |
| --- | --- |
| service.go, domain models and postgres.go | The state/evidence owner coordinates T-001/T-004/T-005/T-006/T-007/T-008/T-029. Use one versioned contract and migration sequence. |
| Request principal/role/subject checks | Coordinate T-002/T-030 with T-003/T-024. Authorize action and subject together; do not let a broad reader expand a narrow writer. |
| Outbox schema and worker | Coordinate T-009/T-010/T-020. Claim fencing, business revision and consumer ordering are distinct guarantees. |
| Repository contexts and transactions | T-012 touches multiple APIs/storage operations. Rebase its interface changes with state/evidence work and verify retries do not duplicate effects. |
| CI/toolchain/release | T-011/T-017 can begin immediately; T-015/T-018/T-019/T-016 must test the actual resulting images and target environments. |
| Consumer/read/delivery code outside archive | T-023/T-024 discovery identifies ownership and effort before new scope. Per-consumer work can run in parallel once shared contracts stabilize. |

## Stop conditions during implementation

Pause a particular migration or rollout if tenant/evidence invariants fail, a required state decision remains unresolved, rollback/recovery loses withdrawal state, or tests would target an unapproved/shared database. Continue independent read-only design and isolated tests. No step in this document authorizes destructive schema rollback, production-data test fixtures, live marketing delivery or deployment without the actual release process.

The production endpoint for this sequence is an evidence-backed GO decision, not simply completion of code changes. Until the required gates are met, the current decision remains **NO-GO**.
