# 17 — Production completion roadmap

**Product:** NVUTO Consent  
**Audit date:** 19 October 2026 (command timestamps in UTC; raw log offsets preserved)  
**Source:** supplied ZIP; commit and branch UNKNOWN  
**Disposition:** documentation-only audit; original source preserved.

## Outcome and planning assumptions

The roadmap completes the documented consent service and the **approved launch consumer set**, then demonstrates the required production evidence. It is based on32 concrete tickets, not a generic frontend/backend rewrite. Current recommendation is NO-GO:1 P0,23 P1 and8 P2 findings remain open. The engineer should preserve the audited baseline, approve the missing decisions and deliver incremental reviewed fixes with corresponding acceptance evidence.

Ticket estimates are provisional focused person-days in report16. There is no defensible calendar completion date without team capacity, the launch consumer list, target infrastructure access and decisions about historical evaluation, evidence and retention. New frontend applications, eight unseen consumer codebases or a newly provisioned distributed platform are not included in a small ticket's estimate. T-032 is requirements/decision discovery; T-023 includes discovery plus one pilot, followed by separately estimated consumer work.

## Workstream ownership and concurrency

| Workstream | First tickets | Main responsibility | Dependency coordination |
| --- | --- | --- | --- |
| Consent/data correctness | T-001, T-004; then T-005/T-006/T-007/T-008/T-029 | One preference revision, safe transitions, canonical evidence, policy/legacy history | Share one additive data design and migration order; do not create competing timestamp-only fixes. |
| API/identity security | T-002, T-014, T-027, T-030 | Customer/object authorization, mixed roles, JWKS resilience, input contract, trusted DOI flow | Coordinate claims and delivery with D-02/D-06/D-10 and the state-generation design. |
| Events/runtime reliability | T-009; then T-010/T-012/T-013/T-021 | Fenced worker ownership, version/reconciliation, cancellation, errors and visibility | Event revisions must derive from the same committed preference/policy model; avoid inventing independent ordering. |
| CI/platform/data operations | T-011, T-017; then T-015/T-016/T-018/T-019/T-020/T-022 | Patched reproducible artifacts, required YSQL checks, secure platform, broker and recovery proof | Platform provisioning can start early; final acceptance waits for repaired code/contracts and actual target access. |
| Product/consumer contracts | T-024, T-032; later T-003/T-023 | Scope, read owner, DOI delivery owner, actual consumer journeys and UAT | No new standalone frontend or unrelated identity/payment scope until approved. |

These are proposed responsibilities, not assigned people or a staffing commitment. One engineer can execute sequentially; multiple engineers may work in parallel on disjoint areas, with review coordination for service.go, postgres.go, model.go and migrations.

## Implementation phases and exit criteria

| Phase | Work and concrete outputs | Entry dependencies | Exit evidence |
| --- | --- | --- | --- |
| A — Establish scope and a trustworthy baseline | D-01/D-02/D-03/D-05/D-06/D-07/D-08/D-09/D-10 first; T-032; original tests/probes and exact source commit; isolated DB/broker plan | Supplied repository and owner access | Reviewed scope/decision assignments, recorded baseline, selected consumer pilot, safe reproducible local workflow. |
| B — Correct immediate safety/security and unblock validation | T-001/T-002/T-004/T-009/T-011/T-014/T-017; T-026/T-027/T-030 and T-024 can progress independently | Relevant behavior decisions; source inspection complete | Stale DOI and authorization cases fail safely; canonical receipt design; fenced ownership; patched toolchain; durable tests actually included in CI. |
| C — Complete coherent service contracts | T-003/T-005/T-006/T-007/T-012/T-013/T-015 | Corresponding predecessor tickets; approved delivery/evidence/time/policy/error expectations | Safe recipient proof, canonical notice/evidence, deterministic temporal contract, versioned policy eligibility, cancellable data operations and honest readiness. |
| D — Reconcile legacy/history and event consumers' contract | T-008/T-010/T-025/T-028/T-029/T-031 plus T-024 read owner | Core state/evidence/policy schema, CI foundation | Representative legacy classification/upgrades, full audit reconstruction, idempotency replay contract, no obsolete write bypass, versioned event/read examples. |
| E — Prove the durable and operational system | T-016/T-018/T-019/T-020/T-021; platform preparation may start earlier | Patched images and relevant contracts; target YSQL/broker/cluster access | Actual HTTP/YSQL concurrency, immutable images, enforced TLS/least privilege, broker drift/failover behavior and observable propagation/error budgets. |
| F — Complete selected application integration and recovery | T-022/T-023, with per-consumer child estimates after discovery | State/contract/platform dependencies; real launch applications and isolated restore environment | Actual user/application permission outcomes, withdrawal/suppression persistence/reconciliation, measured restore/rollback within approved budgets. |
| G — Production acceptance | All applicable AT/G records, owner review and residual risk dispositions | Required prior acceptance complete | Source/image/environment-specific release evidence and a documented GO decision. Current audit does not supply that approval. |

The phases are work packages, not permission to bypass a ticket's predecessors. Some later-phase infrastructure planning can run alongside early code work. The exact dependency waves below and report22 control completion order.

## Dependency waves calculated from the backlog

| Wave | Tickets that can complete after prior dependencies | Purpose |
| --- | --- | --- |
| 1 | [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002), [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [T-014](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-014), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017), [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024), [T-026](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-026), [T-027](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-027), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030), [T-032](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-032) | Independent safety fixes and contract/requirements discovery |
| 2 | [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015) | DOI/evidence/time/runtime/readiness integration |
| 3 | [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013), [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018) | Policy eligibility, stable errors and reproducible packaging |
| 4 | [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021), [T-025](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-025), [T-029](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-029) | Legacy reconciliation, event schemas, secure identities, observability, replay and history |
| 5 | [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020), [T-028](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-028), [T-031](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-031) | Target durable verification, broker policy, constraints and legacy-helper cleanup |
| 6 | [T-022](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-022), [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023) | Restore/recovery and actual selected-consumer acceptance |

The graph contains32 tickets and no dependency cycle. A ticket can start research or write a failing regression before predecessors finish; production completion requires the actual dependency outputs. A phase is not complete because all code merged if its required environment tests remain skipped.

## Conditional scope: historical evaluation

D-03 must choose current-only vs full historical behavior before estimating final T-006 work. The current-only path can reject unsupported `at` values and establish deterministic present-state revisions. A historical path requires the shared immutable consent/policy/suppression timeline inside T-006 (or a restructured earlier foundation ticket) before dependent policy enforcement and investigator work. T-029 cannot be made a predecessor of T-006 without a cycle. T-032 must revise historical-scope estimates and ownership before implementation; the4–8 day baseline T-006 band does not estimate a full historical governance engine.

## Frontend and integration scope

There is no standalone frontend completion phase in this repository because no frontend is supplied or documented as separately owned here. D-01/D-07 and T-024 identify the actual portal/admin application and the authoritative initial-state read source. If approved external frontend work is needed, inspect that repository and create explicit child tickets for real screens, states, accessibility, persistence and error handling. The absence of local screens is not a reason to invent a new framework or account system.

The first consumer pilot should demonstrate a full safety loop: actual authenticated action, current policy/evidence, persisted decision/receipt, worker publication, consumer reconciliation and refusal after denial/withdrawal/suppression/expiry/unavailability. The product owner selects the pilot; Customer Portal and a real send boundary are useful complementary acceptance surfaces, not an assumed committed rollout. All later consumers require their own measured evidence and ownership.

## Definition of production completion

All enabled P0/P1 paths and mandatory dependency gates pass. P2 and scope-dependent work is fixed or explicitly dispositioned with no unresolved mandatory dependency. The release has a known source commit, patched toolchain and immutable image digests, complete applicable contract/regression/durable/consumer tests, safe upgrade and legacy decisions, enforced IdP/TLS/DB/NATS permissions, observable objectives and demonstrated restore/rollback. No missing requirement, unknown environment or skipped mandatory test is converted to a pass.
