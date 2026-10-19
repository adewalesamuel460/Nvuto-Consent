# 13 — Non-functional requirements audit

**Audit date:** 19 October 2026. **Assessment:** production acceptance is blocked by demonstrated decision/evidence defects and by missing target-platform, capacity and operational evidence. The repository provides configuration baselines and tests, but it does not provide an approved service-level objective, workload model, recovery target or complete operating contract.

This report separates documented correctness requirements, observed implementation behavior, proposed acceptance methods and decisions that the product/platform owners must make. Numerical availability, latency, throughput, retention, RPO and RTO targets are **DECISION REQUIRED under D-08**, with related retention and regional decisions under D-05/D-14. No percentage uptime or response-time promise is invented here.

Read alongside [database](08_DATABASE_AND_DATA_SCHEMA_AUDIT.md), [security](10_SECURITY_PRIVACY_AND_ACCESS_CONTROL_AUDIT.md), [integrations](11_THIRD_PARTY_INTEGRATIONS_AUDIT.md), [DevOps](12_DEVOPS_INFRASTRUCTURE_AND_DEPLOYMENT_AUDIT.md), [testing](14_TESTING_QA_AND_VERIFICATION_REPORT.md), [risks](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md), [backlog](16_DETAILED_IMPLEMENTATION_BACKLOG.md), [release gates](19_PRODUCTION_GO_LIVE_CHECKLIST.md), and [decisions](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md).

## 1. What the repository actually requires

README describes deterministic purpose/channel decisions, effective/withdrawn state enforcement, suppression/change propagation and immutable audit history. Missing consent must not become permission where affirmative consent is required. It also claims transactional consent/evidence/audit/outbox writes and gives an explicit YugabyteDB production datastore choice (`README.md:5-19,45-51`). Those requirements justify acceptance invariants around correctness, tenant/subject isolation, state durability and auditable evidence.

The same documentation explicitly distinguishes an RF1 CI run from multi-node Production GA evidence. It does not specify traffic levels, operating regions beyond generic jurisdiction handling, latency/error budgets, dependency outage tolerance, backlog recovery targets or legal retention periods (`README.md:45-51`). The wider production-quality targets below are either measurable proposals or outstanding decisions, not previously approved contractual promises. **F-032 / T-032 / AT-032 / G-032** tracks their resolution.

### Quality status overview

| Quality attribute | Observed state | Production implication |
|---|---|---|
| Consent correctness | **BROKEN in executed Memory counterexamples**; corresponding durable paths traced | Stale challenge reverses later denial/withdrawal; pre-grant time query allows; stricter policy behavior undocumented — F-001/F-006/F-007 |
| Evidence integrity | **BROKEN in actual pgx codec probe**; full notice/decision binding incomplete | Receipt hash does not survive timestamp precision change — F-004/F-005 |
| Transactional persistence | Implemented in governed SQL paths; **durable failure cases NOT VERIFIED** | Must prove all-or-nothing state/receipt/audit/outbox/ledger under target YSQL — F-016 |
| Availability and readiness | API/worker replicas and probes configured; **availability NOT VERIFIED** | Readiness does not establish usable auth/schema; worker progress not probed — F-015/F-021 |
| Performance and capacity | No measured workload/load/soak evidence | Batch query count and multi-replica DB pools need capacity budgets — F-012/F-032 |
| Event resilience | Durable outbox, claim transaction, retry and NATS ID exist; lease/ordering incomplete | Stale claim updates and out-of-order events can undermine consumer consistency — F-009/F-010 |
| Security and privacy | Tenant-aware queries and JWT controls exist; reproduced boundary/abuse defects and external transport gaps | F-002/F-003/F-011/F-014/F-019/F-026/F-030 remain relevant |
| Recoverability | Rollback prose exists; backups/PITR/restore not evidenced | RPO/RTO and permission-state reconciliation remain open — F-022 |
| Maintainability/testability | Small separated Go modules; baseline passes runnable tests; durable tests skipped | CI and acceptance coverage incomplete — F-016/F-017/F-018/F-024 |

## 2. Correctness and consistency requirements

For this service, returning an apparently successful but unsafe allow decision is a more serious failure than returning an explicit unavailable result. Operational metrics must therefore distinguish valid denies, unavailable decisions, invalid requests, and incorrect allows. Counting every `allowed:false` response as an error would mismeasure normal product behavior.

### Required decision invariants and acceptance methods

| Invariant / contract | Current evidence | Measurable acceptance | Final references |
|---|---|---|---|
| Later negative intent must not be silently reversed by an older challenge | Both stale-denial and stale-withdrawal sequences reproduced permission restoration in Memory | Run those sequences plus coordinated concurrent confirm/deny/withdraw on YSQL; no stale generation produces an effective grant | F-001 → T-001 → AT-001 → G-001 |
| A decision must apply to the correct tenant and canonical subject | Confirmation omitted the self-service subject constraint; Memory delimiter collision reproduced | Cross-tenant/cross-subject tests cause no state/receipt/audit/outbox mutation and expose no protected object | F-002/T-002/AT-002/G-002; F-026/T-026/AT-026/G-026 |
| A time-specific response must honor its temporal contract | Governed grant reported allowed one hour before creation | Before-grant queries deny; past/future queries follow documented support or explicit rejection; no invented historical result | F-006/T-006/AT-006/G-006; D-03 |
| Policy changes must have explicit effect on old grants and pending confirmations | Existing contract and unconfirmed grants remained allowed after stricter rules | Test affirmative, double opt-in, evidence, active/category/channel changes and races against writes; record approved grandfathering if used | F-007/T-007/AT-007/G-007; D-04 |
| Legacy data must satisfy an explicit eligibility rule | Blank notice_version bypasses all registry rechecks | Classify/reconcile historical rows; legacy fields cannot silently exempt a grant from mandatory policy | F-008/T-008/AT-008/G-008 |
| Receipt verification must be stable after persistence | Actual pgx changed `.123456789` to `.123456`, invalidating the hash | Canonical bytes/hash verify after real YSQL read, JSON export/import and restart; old evidence remains preserved | F-004/T-004/AT-004/G-004 |
| Audit evidence must explain the actual decision and policy | Immutable action records omit old rules and full evidence bindings | Reconstruct accepted notice, decision/basis, policy and suppression history from retained immutable references | F-005/T-005/AT-005/G-005; F-029/T-029/AT-029/G-029 |

Sources: `internal/service/service.go:88-150,181-235,256-325,342-387`; `internal/store/postgres.go:142-143,154-160,307-315,340-383`; `internal/domain/model.go:71-97,132-151`. Executed counterexample details and their environment limits are in [14](14_TESTING_QA_AND_VERIFICATION_REPORT.md). These tests establish the observed behavior, while D-03/D-04 determine the complete intended temporal/grandfathering contract.

### Atomicity and consistency boundaries

Governed writes use SQL transactions to keep the consent row, receipt where required, audit and outbox together. Idempotent governed creation also includes its request ledger. This is implemented, not missing. Acceptance still requires stage-by-stage failure injection, lost-connection/ambiguous-commit recovery, concurrent updates and target YSQL validation (**F-016**; `internal/store/postgres.go:238-286,344-383`; `internal/store/idempotency.go:59-98`).

There is no single current-preference revision that serializes all rows for a subject/purpose/channel. Evaluation reads suppression, consent and policy through separate calls; batch evaluation repeats the whole sequence. Define whether each response must correspond to a consistent policy/state snapshot, and whether a batch is merely a collection of decisions or an atomic snapshot. Fixes must resolve D-03/D-04 and F-001/F-006/F-007 together; a faster cache must not preserve outdated permission after a withdrawal (`internal/service/service.go:237-254,353-384`).

Outbox commit means an event is available for publication. NATS publish acknowledgement and downstream application are separate stages. The release contract must specify where permission enforcement happens and what happens during stale consumer state. Do not claim exactly-once downstream behavior from the existence of a transaction and NATS message ID (**F-010 / D-07 / D-13**; `cmd/outbox-worker/main.go:102-134`).

## 3. Service-level objectives requiring owner decisions

The following is a concrete specification template for D-08. Owners must choose thresholds and measurement windows before acceptance testing. The formulas and measurements are proposed; every unfilled target remains **DECISION REQUIRED**, not silently waived.

| Measure | Proposed operational definition | Target / decision owner | Evidence to retain |
|---|---|---|---|
| Decision availability | Fraction of eligible authenticated decision requests completed within the agreed latency budget with a valid allow/deny result; explicit unavailable/internal failure counted separately | DECISION REQUIRED, product owner + SRE | Request counts, result/error classes, dependency outages and defined measurement window |
| Evaluation latency | End-to-end p50/p95/p99 for single evaluate, separately by suppressed/no-consent/governed paths | DECISION REQUIRED, product owner + platform engineer | Load results including JWT validation, network, pool wait and database work |
| Batch latency | End-to-end latency for batch sizes 1, representative middle size and maximum 100, with the actual request mix | DECISION REQUIRED, API/consumer owners | Full batch duration, item count, failure policy and query count |
| Mutation latency | Time to durable response for capture/deny/withdraw/suppression/confirm, including receipt/audit/outbox commit | DECISION REQUIRED, product owner + database owner | Per-operation latency and transaction/lock-wait breakdown |
| Withdrawal enforcement delay | Time from accepted durable withdrawal to enforcement by every in-scope communication consumer, not merely publication | DECISION REQUIRED, all launch consumer owners | Correlated commit, event publish/consume and last permitted outbound-action evidence |
| Event propagation lag | Event commit/occurred time to broker acknowledgement and then consumer application, measured separately | DECISION REQUIRED, platform + consumer owners | Oldest unpublished age, delivery/application lag and replay results |
| Backlog recovery | Time to drain an agreed outage backlog while serving ongoing traffic | DECISION REQUIRED, SRE | Arrival/drain rates, retry levels, fairness and consumer catch-up |
| RPO | Maximum acceptable loss of acknowledged durable changes under the approved failure model | DECISION REQUIRED, business/privacy + database owners | Backup/PITR configuration and recoverable commit boundary verified by drill |
| RTO | Time from incident declaration to trustworthy decisions and reconciled propagation | DECISION REQUIRED, business + SRE | Timed restore/failover/reconciliation drill with release decision |
| Capacity and headroom | Sustained/peak request rates, tenant/subject distribution, data volume, event rate and growth horizon | DECISION REQUIRED, product + platform owners | Workload model, connection/storage/CPU/memory budgets, load/soak report |
| Abuse budget | Limits for authenticated/unauthenticated traffic, key misses, batch/confirmation/resend attempts and tenant bursts | DECISION REQUIRED, security + API owners | Measured denial/limiting behavior without legitimate-tenant starvation |
| Retention and replay horizon | Kept history, receipts/notices, audit/outbox and idempotency keys; consumer replay availability | DECISION REQUIRED, privacy + platform owners | Approved retention schedule and tested archive/replay/reconciliation policy |

These measures map principally to **F-012 / F-014 / F-016 / F-020 / F-021 / F-022 / F-023 / F-032** and their same-numbered T/AT/G records. D-05/D-11/D-13/D-14 constrain retention and replay choices; a capacity estimate must not invent or override those choices.

## 4. Current runtime envelope and capacity implications

### Source-defined limits and settings

| Setting | Current value | Evidence / implication |
|---|---|---|
| HTTP header/read/write/idle timeouts | 5s / 15s / 15s / 60s | `cmd/server/main.go:435`; these are transport settings, not database operation deadlines |
| API shutdown budget | 15s | `cmd/server/main.go:452-455`; unfinished background-context DB calls can outlive request handling |
| JSON body reader | 1 MiB | `cmd/server/main.go:78-85`; field cardinality and semantic limits still need definition |
| Batch size | 1–100 requests | `internal/service/service.go:237-240` |
| API DB pool | Max 25, min 2 per API pod; lifetime 45m; idle 5m | `internal/store/postgres.go:22-25`; not configurable through an explicit application budget |
| DB Ping | 2s timeout | `internal/store/postgres.go:38-41`; checks connectivity only |
| API replicas / autoscaling | 3 baseline; HPA 3–20 at CPU 70% | `deploy/k8s.yaml:14,121-129`; no request/backlog/DB-pressure metric configured |
| API resources | Request 100m CPU/128Mi; limit 1 CPU/512Mi | `deploy/k8s.yaml:75-81`; declarative sizing, not observed adequacy |
| Worker replicas / resources | 2; request 50m CPU/64Mi; limit 500m CPU/256Mi | `deploy/outbox-worker.yaml:7,57-63` |
| Worker cadence / batch | 1-second ticker; 50 events; sequential publish | `cmd/outbox-worker/main.go:206,215,220-231` |
| Outbox lease | 2 minutes | `cmd/outbox-worker/main.go:54`; no renewal or ownership fence |
| Retry backoff | Exponential, capped at 900s | `cmd/outbox-worker/main.go:127-134`; no jitter or terminal quarantine in this loop |
| New JetStream configuration | File storage, LimitsPolicy, 10-minute duplicate window, default 3 replicas | `cmd/outbox-worker/main.go:137-169`; existing stream settings are accepted without comparison |
| JWKS fetch/cache | 5s fetch timeout; positive cache 5 minutes | `internal/auth/auth.go:41-47,95-116`; misses can trigger repeated fetches |

### Database connection budget

The API pool maximum multiplied by the declared HPA maximum is:

**25 connections per API pod × 20 API pods = 500 theoretical API connections.**

At the three-pod baseline, the corresponding configured maximum is 75. These figures are arithmetic inputs from `internal/store/postgres.go:22-25` and `deploy/k8s.yaml:14,121-122`; they are not measured open connections, supported YSQL capacity, or an application throughput guarantee. Worker pools, administrators, migrations, other NVUTO applications and rollout headroom are additional. The API rollout permits one surge pod, and terminating pods may take time to release connections (`deploy/k8s.yaml:15-19`).

The capacity model must therefore budget **all** clients, bound pool acquisition and transaction duration, and confirm the target universe can sustain the resulting concurrency. The worker creates its own pgx pool directly, so its effective connection settings must be inspected in the chosen deployment instead of assuming the API's fixed 25 applies to it (`cmd/outbox-worker/main.go:177-183`). D-08/D-09 own this model; **T-012 / T-016** supply implementation and test evidence.

### Query amplification and hot data

Current allowed governed evaluation executes up to five sequential lookups: suppression, effective consent, purpose, basis and notice. A maximum 100-item batch can therefore make about **500 database round trips**. A suppression match may complete after one lookup, and no-consent after two; workload mix materially changes cost. This is derived from source, not a latency benchmark (`internal/service/service.go:237-254,353-384`).

Load tests must vary tenant size, hot subjects, historical rows per preference, suppression scope overlap, policy update frequency, batch size and data/evidence size. Check YSQL query plans for the effective index and pending outbox predicates before changing indexes or adding caches (`migrations/001_init.sql:19-20,45`; `migrations/002_policy_governance.sql:88-89`). Any cache needs explicit withdrawal/policy invalidation semantics and a measured maximum staleness compatible with D-08/D-13. There is no supplied evidence that a cache is necessary or safe today.

### Worker throughput and lease budget

Every claimed batch can contain 50 events sharing a two-minute lease, processed sequentially. Ignoring SQL/claim overhead, average processing above **2.4 seconds per event** would already exceed the lease for a full batch. This is arithmetic illustrating the design risk, not an observed production publish delay. Completion/retry update by ID without checking the claim generation; a slow old worker can interfere after another worker reclaims the event (**F-009**; `cmd/outbox-worker/main.go:46-79,116-134,215-231`).

Define a bounded publish/complete budget, fencing and lease renewal or safe batch sizing before increasing replicas. Backlog recovery requires measured service rate above ongoing arrival rate with agreed headroom, and fairness across tenants. CPU HPA on the API does not measure worker backlog or downstream enforcement lag. `PublishAsyncMaxPending(256)` does not establish 256-way publication: the loop actually calls synchronous PublishMsg (`cmd/outbox-worker/main.go:102-113,199,220-231`).

## 5. Availability, failure handling and resilience

### Request lifecycle and dependency failure

Repository methods accept no context, and durable reads/writes use `context.Background()`. Therefore client disconnection, HTTP timeout and shutdown do not establish a bounded database operation. The only explicit database deadline is Ping. Resolve **F-012** by carrying request context through service/repository, defining transaction/statement/lock/pool-acquisition budgets, and testing uncertain commits against the replay contract (`internal/store/memory.go:15-35`; `internal/store/postgres.go:125-143,238-286`; `internal/store/idempotency.go:59-98`).

Readiness reports ready when Ping succeeds; it does not validate required schema or usable auth configuration. Liveness reports process response. Both can remain positive while real endpoints fail. Worker manifests have no progress/readiness probe, so process survival is not evidence that withdrawals are propagating. Correct health semantics must balance dependency recovery with avoiding endless crash loops, and should be explicitly owned under **D-09 / T-015 / T-021** (`cmd/server/main.go:386-398`; `deploy/k8s.yaml:65-74`; `deploy/outbox-worker.yaml:34-72`).

Unexpected database errors currently fall into a 422 response with internal error text. This obscures operational failure, exposes details and may prevent consumers from retrying appropriately. Define stable public error classes and correlation IDs, with internal diagnostics separated from client messages: **F-013 / T-013 / AT-013 / G-013** (`cmd/server/main.go:59-85`; `internal/store/postgres.go:44-55`).

### Failure-mode acceptance matrix

The following are required test designs or owner decisions, not executed fault-test results.

| Fault | Current concern | Acceptance result to demonstrate | References |
|---|---|---|---|
| API client disconnect / slow DB / lock wait | Database work ignores request context | Work ends within the approved bound; pool recovers; ambiguous mutation can be resolved without duplicate evidence | F-012; AT-012; D-08/D-11 |
| Missing schema / invalid IdP settings | Ping-only readiness can route traffic to unusable API | Startup/readiness blocks incompatible configuration and exposes an actionable internal diagnostic | F-015; AT-015 |
| Unknown-kid traffic / JWKS outage | Five misses reproduced five upstream fetches; no bounded miss refresh path | Singleflight/bounded refresh and miss controls contain load; valid cached keys/rotation follow approved policy | F-014; AT-014; D-10 |
| YSQL leader/node loss or transaction conflict | Only source and RF1 CI definition available; no audit universe | Approved isolation/retry behavior, durable committed state, bounded recovery and no stale permission reversal | F-016; AT-016; D-08/D-09 |
| Worker killed after broker acknowledgement before SQL completion | Event may be published again on retry | Durable consumer deduplication/reconciliation and safe eventual propagation | F-009/F-010; AT-009, AT-010 |
| Worker paused beyond lease while another claims | Completion/retry not fenced | Old owner cannot mutate the newer claim; controlled eventual delivery demonstrated | F-009; AT-009 |
| Duplicate / reversed-order grant and withdrawal events | No aggregate business revision in event | Consumer cannot overwrite newer negative intent with older grant; current-state recheck/reconciliation follows contract | F-010; AT-010; D-13 |
| Broker unavailable or stream mismatched | New stream defaults exist but existing config trusted | Visible failure/backlog; expected stream durability/subjects/retention checked; recovery meets agreed lag | F-020/F-021; AT-020, AT-021 |
| Poison event or prolonged broker outage | Indefinite retry with no terminal/quarantine administration | Approved quarantine/replay procedure preserves evidence and does not starve healthy tenants | F-020/F-021; D-08/D-13 |
| Backup restore behind latest withdrawal | Row restoration alone may revive older grant | Accepted changes reconciled before enforcement resumes; measured recovery boundary and current deny state proved | F-022; AT-022 |

The API has three replicas, a minimum-two PDB, and hostname spread preference; worker has two replicas, minimum-one PDB and preferred host anti-affinity. These are useful declarative baselines. They do not prove node/zone survival, sufficient capacity, broker/DB quorum placement or application-level failover. The API spread uses `ScheduleAnyway`, and worker placement is preferred, so neither proves strict independent-failure placement (`deploy/k8s.yaml:14,32-38,100-109`; `deploy/outbox-worker.yaml:7,25-33,74-83`).

NATS credentials and optional CA loading exist, but the manifest alone does not establish end-to-end TLS/least privilege. Conversely, `nats://` alone is not proof of plaintext: server-required TLS or configured CA can change negotiated transport. Verify actual endpoints, identity scopes, certificate validation and rotation rather than inferring deployment security from a URI (**F-019**, [security report](10_SECURITY_PRIVACY_AND_ACCESS_CONTROL_AUDIT.md)).

## 7. Event delivery, ordering and consumer consistency

The event contract must specify format compatibility, tenant/subject identity, aggregate identity, a business-state revision, effective time and the data or read operation needed for reconciliation. `schema_version` is currently only the payload format version. Registry payloads omit material rules; aggregate_type defaults to consent; state revisions are absent (**F-010**; `internal/domain/model.go:141-151`; `internal/service/service.go:49-84,147-148,197-198,232-233`; `internal/store/postgres.go:106-115`).

The configured ten-minute duplicate window is shorter than the maximum individual retry delay of fifteen minutes. Broker duplicate suppression must not be the sole safety mechanism; consumer idempotency and ordering must tolerate replay beyond the window and after recovery (`cmd/outbox-worker/main.go:127-145`). This does not prove that a duplicate has already occurred or caused a real unauthorized communication; those consumers were not supplied. **F-023 / D-07** requires end-to-end verification in each approved launch consumer.

Expiry is evaluated when reading a consent but does not emit an automatic expiry event. Decide whether consumers always call Evaluate before sending, schedule their own expiry safely, or require service-generated state changes. Do not silently promise continuous expiry propagation from the existing outbox. **D-13 / T-010** must also define replay horizon, stale-event rejection, consumer lag limits and reconciliation after policy changes or restore.

## 8. Observability and operational diagnosis

Current request handlers lack a complete structured request/result logging and metrics layer; worker logs errors but does not publish progress/backlog health. Consequently operators cannot establish the service-level measures in section 3 from this repository alone. **F-021 / T-021 / AT-021 / G-021** should produce a minimum observable contract, with owners and alert thresholds chosen under D-08/D-09 (`cmd/server/main.go:59-98`; `cmd/outbox-worker/main.go:206-231`; `deploy/outbox-worker.yaml:34-72`).

| Signal to add or expose | Purpose | Cardinality / privacy rule |
|---|---|---|
| Request count, duration and status/error class by route/operation | Separate valid denies, validation errors, auth rejection and unavailable service | Do not label metrics with raw subject IDs, tokens or receipt evidence |
| Decision reason and policy/revision context in restricted diagnostics | Explain why permission was granted or suppressed | Use approved audit correlation; avoid unrestricted customer profiling logs |
| Pool acquisition time, in-use/idle connections, transaction/lock duration and cancellations | Detect connection exhaustion and stalled work | Aggregate by bounded service/operation dimensions |
| Auth mode, JWT rejection category, JWKS cache/miss/fetch/concurrency/error measures | Detect unusable auth and miss amplification | Never record raw JWTs, confirmation tokens or key material |
| Pending outbox count and oldest age, attempt distribution and stale-lease/fence rejections | Detect delayed withdrawals and recovery problems | Prefer aggregate and bounded tenant diagnostics with access controls |
| Broker publish acknowledgement, failure/reconnect, stream config drift and consumer application lag | Distinguish database commit, publish and actual enforcement | Correlate event IDs; avoid exposing full payloads by default |
| Receipt verification failures and reconciliation counts | Detect canonicalization/data corruption and preserve investigation evidence | Protect notice/evidence/subject details; retain immutable originals |
| Backup age, recoverable boundary and last successful restore drill | Establish whether RPO/RTO claims have current evidence | Access restricted to operators and designated reviewers |

Alert thresholds require workload baselines. The release must nevertheless demonstrate that a deliberately stalled worker, rejected publish, schema mismatch and exhausted pool trigger the intended signals and runbooks. A dashboard displaying zero errors while no events are processed is not acceptable propagation monitoring.

## 9. Recoverability, data integrity and privacy

Recoverability concerns both the database and the consuming systems' interpretation of permission. RPO/RTO must identify which events count as acknowledged, whether pending publications are retained, and how post-backup withdrawals are restored before communication resumes. Rehearse the combination of database restore, receipt verification, outbox replay, consumer deduplication and safe re-evaluation. No backup/PITR configuration or timed restore evidence was supplied: **F-022** (`README.md:51`; `migrations/ROLLBACK.md:3-5`).

The database contains canonical subject references, consent decisions and collection evidence; these are privacy-sensitive even when no email address is stored. Immutability is useful evidence protection but does not settle retention, legal hold, erasure, regional residency or lawful-basis policy. **D-05/D-12/D-14** require actual regions, controller/processor responsibilities, notice/evidence ownership and retention rules to be approved by the appropriate owner. The service's jurisdiction string and registry categories do not demonstrate jurisdiction-specific compliance (`internal/domain/model.go:15-39,61-97`; `migrations/002_policy_governance.sql:24-71`).

Privacy acceptance must verify tenant/subject authorization, trusted confirmation delivery, suppression-clearing authority, least-privilege database/NATS access, encrypted transport, protected backup/export access, and absence of raw secrets in routine responses/logs. Tests need to include the reproduced self-service confirmation boundary and Memory collision while clearly identifying which deployment modes are in scope (**F-002/F-003/F-019/F-026/F-030**). Compliance certification or legal sufficiency is not claimed by this audit.

## 10. Maintainability, compatibility and testability

The code separates HTTP handling, domain models, service rules, repositories and worker publication, and both adapters implement the repository contract. This is a useful maintenance baseline. However, equivalent method signatures do not guarantee equivalent behavior: Memory key collisions, event references and concurrent state updates differ from durable storage. Add shared behavior tests as part of **T-026**, without using Memory success to certify SQL transactions (`internal/store/memory.go:60-74,190-235`; `internal/store/postgres.go:261-286,300-320`).

The legacy exported Record/Upsert path can create state without atomic audit/outbox handling if reused. It is not the active HTTP creation path, but it is a maintenance hazard for the incoming engineer. Remove, constrain or explicitly document that obsolete path under **F-031 / T-031**, with failure-injection tests appropriate to its retained role (`internal/service/service.go:152-178`; `cmd/server/main.go:200-202`).

Approved client contracts and discovery APIs are not supplied in machine-readable form. Event/schema changes, canonical receipt versions and idempotency behavior need compatibility rules and contract tests with actual consumer owners. New read/list endpoints are scope decisions, not automatic NFR additions: **D-01/D-07/D-11**, **F-024/F-025**.

The executed vulnerability scan did not satisfy the security gate for the pinned toolchain/dependencies. Resolve **F-011** through aligned source/CI/container versions, documented advisory triage and rescanning of actual release artifacts. Do not equate every scanner symbol path with a proven exploitable endpoint. Mutable release-image references and incomplete release packaging also prevent reliable provenance/rollback evidence: **F-018** (`go.mod:3-21`; `Dockerfile:1-10`; `Dockerfile.worker:1-10`; `deploy/k8s.yaml:41`; `deploy/outbox-worker.yaml:36`). Detailed scan and packaging evidence belong to the testing/security/DevOps reports.

### Test evidence and remaining quality gates

The unchanged repository baseline had **18 named tests: 15 passed, 0 failed, 3 integration tests skipped**. The race run reported the same pass/skip result. Statement coverage was **52.2% overall**, with durable adapter methods unexecuted because the database/broker prerequisites were absent. Those are observed test results, not readiness percentages. Audit-only safety assertions exposed defects that the passing baseline did not test; their failures are documented separately in [14](14_TESTING_QA_AND_VERIFICATION_REPORT.md).

The next acceptance suite needs more than a coverage threshold. It must prove important negative and concurrent cases: transaction rollback at every stage; stale challenge supersession; before-grant time queries; policy changes; correct tenant/subject boundaries; receipt hash round trips; real YSQL lock/conflict behavior; worker ownership expiry; duplicate/out-of-order events; canceled requests; startup/readiness failures; restore and downstream reconciliation. CI must run the durable idempotency test and migration 003 upgrade coverage, and integration fixtures must use unique run IDs so repeat runs do not collide (**F-016 / F-017**).

## 11. Scope limits and non-applicable checks

No frontend application is supplied, and this repository does not own account creation, password reset, campaign execution or message delivery UI. Browser rendering, accessibility, responsive layouts and client bundle metrics cannot be assessed as service implementation qualities here. They remain the responsibility of whichever consumer applications are in the approved D-01/D-07 launch scope; their absence is not proof a new frontend must be built in this repository (`README.md:7-11`; `cmd/server/main.go:399-409`).

Localized notice content and user-facing consent language still matter to the consumer/evidence contract, but the supported languages/regions are unspecified. Record these under D-05/D-12/D-14. This report does not impose a WCAG certification, a particular legal regime, an uptime promise or a retention duration without a confirmed owner requirement.

## 12. Release decision and required ownership

Production acceptance requires the correctness/evidence defects to be closed with their AT evidence, numerical and temporal/retention contracts to be approved, and target-platform reliability/recovery/integration evidence to exist. The [go-live checklist](19_PRODUCTION_GO_LIVE_CHECKLIST.md) is the authority for G-001 through G-032; the [backlog](16_DETAILED_IMPLEMENTATION_BACKLOG.md) carries detailed work and estimates.

| Ownership decision | Required outcome before acceptance | Related final records |
|---|---|---|
| Product/privacy: D-03/D-04/D-05/D-12/D-14 | Defined state timeline, grandfathering, complete evidence and regional/retention requirements | F-001/F-004/F-005/F-006/F-007/F-008/F-029/F-032 |
| Platform/SRE: D-08/D-09 | Approved workload/SLO/RPO/RTO; bounded operations; tested placement, capacity, health, monitoring and recovery | F-009/F-012/F-015/F-016/F-018/F-019/F-020/F-021/F-022 |
| Identity/security: D-02/D-06/D-10 | Trusted confirmation proof, claim mapping, object boundaries, role/suppression authority and bounded authentication behavior | F-002/F-003/F-011/F-014/F-019/F-030 |
| Consumer/API owners: D-01/D-07/D-11/D-13 | Explicit launch integrations, replay/ordering/expiry and client compatibility contracts; actual enforcement tests | F-010/F-023/F-024/F-025/F-032 |

A replica count, a nonempty receipt hash, a healthy database Ping or passing unit tests cannot substitute for these outcomes. The implementation sequence should close the state/evidence and trust-boundary work first, then prove durable behavior and consumer enforcement, and finally execute the approved load, outage and restore acceptance program.
