# 08 — Database and data schema audit

**Audit date:** 19 October 2026. **Evidence basis:** the supplied NVUTO Consent ZIP snapshot, source inspection, the executed unchanged baseline tests, separate Memory regression probes, and an executed pgx timestamp-codec probe. **Database release assessment: NOT READY FOR ACCEPTANCE.** The current schema and transaction code implement a useful consent ledger, but the state-ordering and evidence defects must be resolved and the target YugabyteDB behavior must be demonstrated before accepting production use.

This report inventories the actual schema; proposed schema additions are identified explicitly. No database was created, migrated, queried, changed or restored during this audit. Existing production row counts, topology, credentials, backups and operating history were not supplied. A migration file or passing Memory test is not evidence that a production database satisfies its intended guarantees.

Related reports: [backend/API](06_BACKEND_AND_API_AUDIT.md), [non-functional requirements](13_NON_FUNCTIONAL_REQUIREMENTS_AUDIT.md), [testing](14_TESTING_QA_AND_VERIFICATION_REPORT.md), [risk register](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md), [implementation backlog](16_DETAILED_IMPLEMENTATION_BACKLOG.md), [release gates](19_PRODUCTION_GO_LIVE_CHECKLIST.md), [decisions](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md), and [evidence log](21_AUDIT_EVIDENCE_AND_COMMAND_LOG.md).

## 1. Datastore ownership and current implementation

The documented production transactional database is **YugabyteDB YSQL**. The adapter is named `Postgres` and uses pgx because YSQL speaks the PostgreSQL wire protocol; the README explicitly distinguishes this from choosing ordinary PostgreSQL as the production datastore. Production-universe TLS, authentication, placement, backup/PITR and recovery policy are external requirements, not implemented platform infrastructure in this repository (`README.md:45-51`; `internal/store/postgres.go:9-35`).

The API chooses durable storage when `DATABASE_URL` is present. Otherwise it requires explicit `ALLOW_MEMORY_STORE=true` before constructing the non-durable adapter. The supplied production manifest does not set that flag (`cmd/server/main.go:413-430`; `deploy/k8s.yaml:44-64`). Memory is a development/test implementation, and it is neither persistent nor an equivalent substitute for YSQL acceptance testing. Its reproduced composite-key collision and other behavior differences are tracked under **F-026 / T-026 / AT-026 / G-026**.

`store.Repository` has 20 methods, including Ping. A separate optional interface supports durable governed-consent idempotency. Every production read in the adapter uses tenant-scoped parameters; mutations use parameterized SQL rather than interpolating request strings. Governed creation and registry/suppression/withdrawal/confirmation paths use transactions. These controls exist and should be preserved (`internal/store/memory.go:15-35`; `internal/store/idempotency.go:11-13,59-98`; `internal/store/postgres.go:94-103,138-143,146-165,238-286,300-320,344-383`).

### Inventory totals

The schema below is the result intended after migrations 001, 002 and 003, not an inspected deployed schema.

| Object | Count | Interpretation |
|---|---:|---|
| Tables | 9 | All owned by this service's SQL schema |
| Final columns across the nine tables | 90 | Includes columns added by migration 002 |
| Primary keys | 9 | Five global ID keys and four tenant-composite keys |
| Additional inline UNIQUE constraints | 2 | Receipt hash per tenant; logical suppression key |
| Explicitly created indexes | 8 | Includes the unique partial confirmation-token index |
| Foreign keys | 1 | Receipt consent ID references consent ID; tenant is not part of the FK |
| Final CHECK constraints | 2 | Consent status and suppression scope |
| Immutability triggers | 3 | Row UPDATE/DELETE on receipts, notices and audit events |
| Trigger functions | 1 | `reject_consent_immutable_change()` |
| Views / application stored procedures / RLS policies | 0 found | No such definitions in the supplied migrations |

Sources: `migrations/001_init.sql:1-45`; `migrations/002_policy_governance.sql:1-110`; `migrations/003_governed_idempotency.sql:1-11`. NOT NULL declarations and defaults are not counted as separate named constraints in this inventory.

## 2. Complete table and column inventory

### 2.1 `consents` — individual consent records and confirmation state

Sources: `migrations/001_init.sql:1-20`; `migrations/002_policy_governance.sql:1-13,20-22`; corresponding model `internal/domain/model.go:15-39`.

| Column | SQL type | Nullability / default | Meaning and current limitation |
|---|---|---|---|
| `id` | text | Primary key | Globally unique consent-record identifier |
| `tenant_id` | text | NOT NULL | Tenant claim copied into stored record |
| `subject_id` | text | NOT NULL | Canonical Customer Graph party identifier; no locally owned subject table |
| `purpose` | text | NOT NULL | Purpose code; no FK to purpose registry |
| `channel` | text | NOT NULL | Normalized channel label |
| `legal_basis` | text | NOT NULL, empty default | Lawful-basis code; no FK to registry |
| `status` | text | NOT NULL, CHECK | Final allowed values: granted, denied, withdrawn, expired, pending_confirmation |
| `source` | text | NOT NULL | Originating application or capture source |
| `scope` | jsonb | NOT NULL, `{}` default | String map at service boundary; database does not enforce object/string-value shape |
| `version` | bigint | NOT NULL, 1 default | Version of this row, not a sequenced version of the entire logical preference |
| `granted_at` | timestamptz | Nullable | Grant time; not checked against requested evaluation time |
| `denied_at` | timestamptz | Nullable | Denial time |
| `withdrawn_at` | timestamptz | Nullable | Withdrawal time |
| `expires_at` | timestamptz | Nullable | Final consent expiry; evaluation reads this value |
| `created_at` | timestamptz | NOT NULL | Application-generated creation time |
| `updated_at` | timestamptz | NOT NULL | Sole current effective-record ordering field |
| `jurisdiction` | text | NOT NULL, empty default | Added in 002; normalized label, not an implemented legal-policy engine |
| `notice_version` | text | NOT NULL, empty default | Added in 002; empty legacy value bypasses current registry rechecks |
| `receipt_id` | text | NOT NULL, empty default | Application-maintained receipt pointer without an FK |
| `evidence` | jsonb | NOT NULL, `{}` default | Method, artifact hash and collection time; omitted from ordinary consent JSON |
| `confirmation_token_hash` | text | NOT NULL, empty default | SHA-256 hash of confirmation token; cleared after successful confirmation |
| `confirmation_expires_at` | timestamptz | Nullable | Expiry of pending challenge, distinct from final consent expiry |
| `confirmed_at` | timestamptz | Nullable | Confirmation time |

There is no unique current-preference row for `(tenant_id, subject_id, purpose, channel)`. Grant/deny/start operations normally create separate consent IDs, and later state changes update those rows. The latest `updated_at` becomes authoritative, without a deterministic tie-break. This design enables the reproduced stale-confirmation reversal and inconsistent temporal behavior: **F-001** and **F-006** (`internal/service/service.go:121-150,256-325,342-387`; `internal/store/postgres.go:142-143`).

### 2.2 `consent_audit_events` — immutable action records

Sources: `migrations/001_init.sql:22-30`; `migrations/002_policy_governance.sql:107-110`; `internal/domain/model.go:132-139`.

| Column | Type | Constraint / purpose |
|---|---|---|
| `id` | text | Global primary key |
| `tenant_id` | text | NOT NULL |
| `actor_id` | text | NOT NULL; actor performing the operation |
| `action` | text | NOT NULL; action string such as consent.granted |
| `consent_id` | text | NOT NULL; overloaded with purpose/basis code, notice version or suppression ID for those actions |
| `occurred_at` | timestamptz | NOT NULL |

The overloaded `consent_id` cannot safely become a consent-only FK without first changing the event model. Immutability prevents row edits through ordinary UPDATE/DELETE, but the record does not preserve before/after policy or suppression configuration. Therefore immutable action existence does not establish a reconstructible policy history: **F-029** (`internal/service/service.go:58-59,71-72,83-84,232-233`).

### 2.3 `consent_outbox_events` — durable event and delivery ledger

Sources: `migrations/001_init.sql:32-45`; `migrations/002_policy_governance.sql:15-18`; `cmd/outbox-worker/main.go:20-31,46-79,116-146`.

| Column | Type | Nullability / default | Purpose |
|---|---|---|---|
| `id` | text | Primary key | Event ID also used as NATS message ID |
| `tenant_id` | text | NOT NULL | Event tenant |
| `event_type` | text | NOT NULL | Business event name |
| `aggregate_id` | text | NOT NULL | Referenced consent, registry entry or suppression |
| `payload` | jsonb | NOT NULL, `{}` | Event-specific data |
| `occurred_at` | timestamptz | NOT NULL | Application event time |
| `published_at` | timestamptz | Nullable | SQL completion marker following publish acknowledgement |
| `attempts` | integer | NOT NULL, 0 | Incremented when claimed |
| `next_attempt_at` | timestamptz | NOT NULL, now() | Retry eligibility time |
| `locked_at` | timestamptz | Nullable | Claim timestamp; currently treated as a two-minute lease |
| `locked_by` | text | Nullable | Worker hostname; completion/retry do not enforce it |
| `schema_version` | integer | NOT NULL, 1 | Event-format version, not business-state revision |
| `source` | text | NOT NULL, nvuto/consent | CloudEvent source |
| `aggregate_type` | text | NOT NULL, consent | Default also used for registry/suppression events unless explicitly set |

Claiming uses a transaction and `FOR UPDATE SKIP LOCKED`. The subsequent publish and SQL completion cannot be one atomic transaction with NATS. Completion and retry lack a claim-generation predicate, so an expired worker can alter a newer claim: **F-009**. Business-state revisions and consumer reconciliation are also absent: **F-010**. Do not interpret `published_at` as proof that every downstream application has applied the withdrawal or that an event was delivered exactly once.

### 2.4 `consent_purposes` — mutable purpose registry

Sources: `migrations/002_policy_governance.sql:24-35`; `internal/domain/model.go:49-59`; `internal/store/postgres.go:146-176`.

| Column | Type | Nullability / default |
|---|---|---|
| `tenant_id` | text | NOT NULL; first PK component |
| `code` | text | NOT NULL; second PK component |
| `category` | text | NOT NULL |
| `allowed_channels` | jsonb | NOT NULL |
| `requires_affirmative_consent` | boolean | NOT NULL, false |
| `requires_double_opt_in` | boolean | NOT NULL, false |
| `active` | boolean | NOT NULL, true |
| `version` | bigint | NOT NULL, 1 |
| `updated_at` | timestamptz | NOT NULL |

The upsert replaces the current rule fields and increments a version; it does not retain the previous version as a separate record. Captured consent does not record that policy version. This matters to historical evaluation, stricter policy rollout and audit reconstruction: **F-006 / F-007 / F-029**. The service normalizes/deduplicates channels, but the database has no JSON array/nonempty-value constraints (`internal/service/service.go:27-61`).

### 2.5 `consent_lawful_bases` — mutable lawful-basis registry

Sources: `migrations/002_policy_governance.sql:37-46`; `internal/domain/model.go:61-69`; `internal/store/postgres.go:179-209`.

| Column | Type | Nullability / default |
|---|---|---|
| `tenant_id` | text | NOT NULL; first PK component |
| `code` | text | NOT NULL; second PK component |
| `allowed_categories` | jsonb | NOT NULL |
| `requires_evidence` | boolean | NOT NULL, false |
| `active` | boolean | NOT NULL, true |
| `version` | bigint | NOT NULL, 1 |
| `updated_at` | timestamptz | NOT NULL |

A basis must be active and allow the purpose category at governed capture. If RequiresEvidence is true, capture checks nonempty method/hash and collection time relative to notice publication and now. Evaluation does not repeat the evidence requirement after a later policy change. Approved grandfathering and policy-revision behavior remain **D-04**, with implementation **T-007** (`internal/service/service.go:88-105,373-385`).

### 2.6 `consent_privacy_notices` — immutable notice identifiers

Sources: `migrations/002_policy_governance.sql:48-55,102-105`; `internal/domain/model.go:71-77`; `internal/store/postgres.go:212-236`.

| Column | Type | Constraint / default |
|---|---|---|
| `tenant_id` | text | NOT NULL; PK component |
| `jurisdiction` | text | NOT NULL; PK component |
| `version` | text | NOT NULL; PK component |
| `published_at` | timestamptz | NOT NULL |
| `active` | boolean | NOT NULL, true |

There is no stored accepted notice text, content digest, immutable external reference, language, or retirement record. The active flag cannot change through the current insert-only API or ordinary UPDATE because the row is immutable. Decide whether notice artifacts are owned here or immutably referenced from another component, and how supersession/retirement works without altering accepted evidence: **D-05 / D-14**, **F-005 / T-005**. This report does not determine legal sufficiency in a jurisdiction.

### 2.7 `consent_receipts` — hashed evidence records

Sources: `migrations/002_policy_governance.sql:57-73,97-100`; `internal/domain/model.go:85-97`; `internal/service/service.go:108-117`.

| Column | Type | Constraint / purpose |
|---|---|---|
| `id` | text | Global primary key |
| `tenant_id` | text | NOT NULL |
| `consent_id` | text | NOT NULL; FK to consents.id |
| `subject_id` | text | NOT NULL |
| `purpose` | text | NOT NULL |
| `channel` | text | NOT NULL |
| `notice_version` | text | NOT NULL |
| `jurisdiction` | text | NOT NULL |
| `evidence` | jsonb | NOT NULL; method/artifact hash/collected_at |
| `receipt_hash` | text | NOT NULL; unique with tenant_id |
| `issued_at` | timestamptz | NOT NULL |

Receipt hash is unkeyed SHA-256 over the Go JSON after blanking its own hash field. It is not a digital signature. An actual pgx binary timestamp encode/decode probe transformed `2026-10-19T12:00:00.123456789Z` into `2026-10-19T12:00:00.123456Z`; the fetched-form receipt no longer produced its original hash. **This precision defect is reproduced; a live YSQL round trip was not run.** The schema and adapter use that timestamp path (`internal/store/postgres.go:119-122,289-297`). See **F-004 / T-004 / AT-004 / G-004** and the [test report](14_TESTING_QA_AND_VERIFICATION_REPORT.md).

The receipt also lacks status, legal basis, scope, expiry, consent/policy revision and actor. Repairing timestamp precision alone does not define the complete proof contract. Canonical serialization, retained original evidence and enrichment must be designed together under **F-004 / F-005**; existing immutable receipts must not be rewritten to conceal a verification discrepancy.

### 2.8 `consent_suppressions` — current opt-out scopes

Sources: `migrations/002_policy_governance.sql:75-89`; `internal/domain/model.go:99-118`; `internal/store/postgres.go:300-333`.

| Column | Type | Nullability / default |
|---|---|---|
| `id` | text | Global primary key |
| `tenant_id` | text | NOT NULL |
| `subject_id` | text | NOT NULL |
| `scope` | text | NOT NULL; CHECK global/channel/purpose_channel |
| `purpose` | text | NOT NULL, empty |
| `channel` | text | NOT NULL, empty |
| `reason` | text | NOT NULL |
| `active` | boolean | NOT NULL, true |
| `version` | bigint | NOT NULL, 1 |
| `updated_at` | timestamptz | NOT NULL |

Unique key: `(tenant_id,subject_id,scope,purpose,channel)`. Service validation requires no purpose/channel for global, channel only for channel scope, and both for purpose_channel. Those shape rules are not SQL constraints. Existing row content is overwritten on update, so the current table cannot reconstruct activation/deactivation intervals or previous reasons. Suppression-clearing authority and retained history are separate decisions under **D-06** and **D-05**, tracked by **F-030** and **F-029** (`internal/service/service.go:202-235`).

### 2.9 `consent_idempotency` — governed capture replay ledger

Sources: `migrations/003_governed_idempotency.sql:1-11`; `internal/store/idempotency.go:59-98`; `internal/service/idempotency.go:17-81`.

| Column | Type | Constraint / purpose |
|---|---|---|
| `tenant_id` | text | NOT NULL; first PK component |
| `idempotency_key` | text | NOT NULL; second PK component |
| `request_hash` | text | NOT NULL; normalized semantic fingerprint |
| `consent_id` | text | NOT NULL; stored outcome identifier, no FK |
| `created_at` | timestamptz | NOT NULL |

The ledger is inserted/serialized in the same transaction as governed state, receipt, audit and outbox. Exact matching retries return the current consent with the original ID; conflicting fingerprints fail. The service checks current policy before reaching the ledger, and there is no persisted original response snapshot or expiry policy. Consequently retry behavior can change after policy changes or withdrawal. **D-11 / F-025** must establish the replay and retention contract before cleanup or response changes are implemented.

## 3. Relationships, indexes and database enforcement

### 3.1 Actual and application-maintained relationships

| Relationship | Enforcement today | Assessment |
|---|---|---|
| Receipt consent_id → consent id | Actual FK on ID only | Prevents missing consent ID, but permits mismatched receipt tenant/subject if a writer bypasses service construction |
| Consent receipt_id → receipt id | Application-maintained string pointer | No FK or one-to-one constraint; receipt creation is transactional in governed paths |
| Consent purpose / basis / notice → tenant registries | Service lookups | No database FK; legacy blanks must be considered before adding constraints |
| Idempotency consent_id → consent id | Same-transaction application write | No FK; no automatic protection if external/manual data changes create an orphan |
| Audit/outbox aggregate references → various entities | Polymorphic strings | Cannot be enforced as consent-only references without an explicit aggregate model |
| Tenant/subject → external identity/customer systems | External identifiers | Absence of local identity tables is consistent with README scope; external integrity contract remains required |

The receipt FK and source-level enforcement gaps are **F-028**. No cross-tenant SQL exploit was demonstrated: the audited adapter filters tenant explicitly and service constructors set matching tenant metadata. Strengthen relationships deliberately, without claiming that adding an FK alone establishes authorization or that RLS is an already approved architectural requirement (`migrations/002_policy_governance.sql:69-70`; `internal/store/postgres.go:138-143,232-235,289-297`; `README.md:7,41`).

### 3.2 All eight explicit indexes

| Index | Definition | Query supported / remaining verification |
|---|---|---|
| `idx_consents_effective` | tenant_id, subject_id, purpose, channel, updated_at DESC | Current effective-record lookup; missing total state order remains F-006 |
| `idx_consents_status` | tenant_id, status, updated_at DESC | Available for status-oriented access; no such public list endpoint is present |
| `idx_consent_audit_tenant_time` | tenant_id, occurred_at DESC | Tenant/time audit retrieval; no public audit-search API is present |
| `idx_consent_outbox_pending` | next_attempt_at, occurred_at WHERE published_at IS NULL | Pending/retry scan; actual claim additionally filters lease age and orders occurred_at,id, so target plans need measurement |
| `idx_consents_confirmation_token` | UNIQUE tenant_id, confirmation_token_hash WHERE hash nonempty | Token lookup and token uniqueness within a tenant |
| `idx_consent_receipts_subject` | tenant_id, subject_id, issued_at DESC | Subject receipt retrieval; current HTTP API reads by receipt ID |
| `idx_consent_suppressions_lookup` | tenant_id, subject_id, active, scope, purpose, channel | Suppression precedence lookup; verify OR predicates and cardinalities at target scale |
| `idx_consent_idempotency_consent` | tenant_id, consent_id | Ledger-to-consent diagnostic/reconciliation lookup |

Sources: `migrations/001_init.sql:19-20,30,45`; `migrations/002_policy_governance.sql:20-22,72-73,88-89`; `migrations/003_governed_idempotency.sql:10-11`. Primary and inline unique constraints also entail database index structures, but are not double-counted as explicit CREATE INDEX statements. No EXPLAIN/latency/load evidence was obtained, so this report does not declare any index adequate or redundant for the production universe.

### 3.3 Immutability and missing state constraints

`reject_consent_immutable_change()` raises an exception for UPDATE/DELETE. Three triggers attach it to receipts, privacy notices and audit events (`migrations/002_policy_governance.sql:91-110`). That protects ordinary row modification. It does not replace production ownership/privilege restrictions, backups or controls over privileged DDL/TRUNCATE. No least-privilege database role provisioning is included in the migrations; external controls remain **F-019 / D-09**.

Constraints do not currently enforce positive versions, nonempty identifiers, JSON object/array shapes, valid suppression field combinations, consistent pending-token/expiry state, or tenant-consistent receipt linkage. Scan helpers silently ignore JSON unmarshal errors. The resulting corruption path has not been exercised against a database; it is a concrete integrity gap to address under **T-028**, with decode-error visibility linked to **T-013** (`internal/store/postgres.go:62-79,168-176,201-209,289-297`).

## 4. Transaction and state-lifecycle assessment

| Operation | Writes and atomic boundary | Current qualification |
|---|---|---|
| Governed grant/deny | New consent + receipt + audit + outbox in one SQL transaction | Atomic path exists; receipt verification and policy/current-state semantics need repair |
| Governed creation with key | Same four objects + idempotency ledger in one transaction | Correct deduplication structure exists; real YSQL concurrency/rollback and replay contract unverified |
| Purpose/basis upsert | Registry update + audit + outbox in one transaction | Old configuration is not retained as a policy revision |
| Notice publish | Immutable notice insert + audit + outbox in one transaction | Duplicate tuple conflicts; retirement/content evidence undefined |
| Withdrawal | Named consent locked/updated + audit + outbox | Repeat of already-withdrawn row returns without new events; preference-wide ordering remains defective |
| Suppression update | Logical scope upsert + audit + outbox | Stable durable ID/version; history overwritten; clearing authority undecided |
| Double-opt-in start | Pending consent + audit + outbox | No receipt yet; several pending rows can exist for one preference |
| Double-opt-in confirm | Token row locked/updated + receipt + audit + outbox | Single-use check exists; no current-preference generation or policy revalidation |
| Legacy Record | Upsert then separate audit/outbox calls | Errors from side-effect appends ignored; internal-only path, not current HTTP creation |

Sources: `internal/store/postgres.go:146-165,179-198,212-229,238-286,300-320,336-383`; `internal/store/idempotency.go:59-98`; `internal/service/service.go:152-178`. Governed transaction existence must not be confused with executed rollback/failure-injection proof. The three database/broker integration suites skipped in this audit; **F-016 / T-016 / AT-016 / G-016** remains open.

### Priority data correctness failures

1. **Stale intent:** older pending challenge A can become newest and grant permission after a later deny or a newer grant/withdrawal. Both Memory sequences reproduced; durable source uses the same row-local ordering. Additive preference-generation design and race tests belong to **T-001**.
2. **Time:** `at` affects expiry/notice publication comparisons but does not select historical consent/suppression/policy state or enforce grant lower bounds. The pre-grant allow counterexample reproduced. Resolve **D-03** and implement **T-006** before claiming historical evaluation.
3. **Policy evolution:** existing weaker grants and pending challenges can survive stricter rules without an explicit grandfathering record. Two Memory cases reproduced; governed-vs-legacy eligibility is **D-04 / T-007 / T-008**.
4. **Legacy migration:** 002 gives old rows blank notice_version; the evaluator skips all registry checks for those rows. CI confirms retention of the blank value, not its eligibility. Actual affected volume is UNKNOWN (**F-008**; `migrations/002_policy_governance.sql:6-13`; `.github/workflows/ci.yml:58-62`; `internal/service/service.go:373-387`).
5. **Evidence:** hash precision fails in the actual driver codec, and the proof model lacks binding to exact notice content and the complete decision. Repair under **T-004 / T-005** while preserving original evidence.

The `expired` status is valid SQL data, but the service has no materialization job that sets expired or emits a corresponding event. Evaluation returns an expired reason while the row can remain granted. Double-opt-in `expires_at` input becomes challenge expiry only; a confirmed consent has no final expiry supplied by that start flow. These are contracts to resolve under **D-02 / D-13**, rather than assuming a missing expiry scheduler is an independently approved feature (`internal/service/service.go:278-295,367-372`; `internal/domain/model.go:33-36`).

## 5. Migration, compatibility and safe data reconciliation

### Migration sequence

| Migration | Intended change | Acceptance limitation |
|---|---|---|
| `001_init.sql` | Consent, audit and outbox tables; initial four-state check; indexes | Actual fresh installation not executed during this audit |
| `002_policy_governance.sql` | Wider consent status; governance/token/evidence columns; outbox envelope columns; five tables; token index; three immutable triggers | Retains blank legacy policy context; temporarily drops then recreates status check; no explicit transaction wrapper |
| `003_governed_idempotency.sql` | Tenant-scoped replay ledger and consent lookup index | Durable store test and 003 upgrade verification are omitted from current durable CI stage: F-017 |

The API does not apply migrations and readiness checks only database Ping, not required tables/columns/triggers or a schema version (`cmd/server/main.go:392-398,413-435`; `internal/service/service.go:390`; `internal/store/postgres.go:38-41`). No migration ledger/checksum/locking mechanism is provided by these scripts. The SQL contains replay-tolerant IF EXISTS/IF NOT EXISTS statements, but these do not prove schema equivalence or make interrupted application safe. Production migration ownership and tooling are **D-09 / F-018**; readiness protection is **F-015**.

The rollback document recommends application rollback while preserving the additive schema and evidence. For schema defects it describes pausing registry writes/outbox publication, taking a verified backup, making a forward correction, validating counts and constraints, then resuming. It does not specify a 003 rollback process or prove older binaries understand pending/governed data (`migrations/ROLLBACK.md:3-5`). The actual image/schema compatibility matrix must be rehearsed, not inferred from the document alone.

### Required migration and reconciliation evidence

The incoming engineer should execute the following through an owned, isolated target-version environment and retain results in the test/release evidence package. These are proposed acceptance activities; none ran in this audit.

| Check | Required evidence | References |
|---|---|---|
| Empty install | Schema inventory matches all nine tables, columns, constraints, indexes and triggers; API refuses incompatible schema | T-015, T-016, T-017; AT-015, AT-016, AT-017 |
| Populated 001 → latest | Legacy granted/denied/withdrawn/expired cases remain preserved; eligibility explicitly classified; counts reconcile | T-008, T-017; AT-008, AT-017 |
| Populated 002 → 003 | Existing receipt/policy/outbox state remains unchanged; idempotency becomes available; repeated upgrade safe by chosen migration tool | T-017; AT-017 |
| Policy/state-model migration | Old rows deterministically map to chosen preference generation/history model; pending stale tokens cannot regain permission | T-001, T-006, T-007, T-029 |
| Constraint hardening | Historical invalid shapes/orphans are classified before enforcement; valid old data remains readable | T-028; AT-028 |
| Canonical receipt migration | Old receipt bytes/hashes retained; format version and discrepancy reconciliation are explicit; new hashes verify after real reads | T-004, T-005; AT-004, AT-005 |
| Interrupted/repeated migration | Chosen tool detects partial/version-mismatched state and prevents ambiguous readiness; migration ownership serialized | T-015, T-017, T-018 |
| Application rollback | Exact known image/schema pairs can read necessary history and enforce withdrawals after rollback | T-018, T-022; AT-018, AT-022 |

An initial **read-only, unexecuted** legacy classification can be specified from existing columns as follows. Run it only against the authorized environment, and protect tenant-level results as operational data:

```sql
SELECT tenant_id, status, COUNT(*) AS record_count
FROM consents
WHERE notice_version = ''
GROUP BY tenant_id, status
ORDER BY tenant_id, status;
```

The count is a discovery input, not an eligibility verdict. A nonempty notice version also does not prove evidence correctness. Classification must assess linked policy/notice/evidence, purpose rules and approved regional handling, then record a reviewable decision under **D-04 / D-05 / D-14**. No arbitrary backfill values, mass grant changes, receipt rewrites or destructive cleanup are authorized by this report.

### YSQL-specific verification boundary

Testing must use the chosen YSQL version and production-relevant isolation, placement, connection and TLS settings. The CI RF1 node described in README cannot establish multi-node failover, universe capacity or Production GA behavior (`README.md:49-51`). Specifically verify concurrent upserts and row locks, `FOR UPDATE SKIP LOCKED`, unique conflicts, transactional receipt/audit/outbox rollback, timestamp encoding, trigger behavior, and representative query plans on the target platform (**F-016 / F-017**).

The adapter does not retry SQLSTATE 40001/40P01; `mapPG` only special-cases no-row and unique-violation 23505. Whether the target isolation/workload requires application retries must be measured and documented. Do not assert either that all YSQL conflicts will succeed automatically or that a particular conflict failure was reproduced (`internal/store/postgres.go:44-55`; transaction bodies at `238-286,300-383`). Retry ownership must respect uncertain commits and **D-11** idempotency semantics.

## 6. Data lifecycle, retention and recovery

| Data class | Current lifecycle | Decision / missing operating evidence |
|---|---|---|
| Consent rows | New rows for capture/challenge; later updates for confirmation/withdrawal; no deletion path | Required history and expiry semantics: D-03/D-05/D-13 |
| Receipts and notices | Insert only; UPDATE/DELETE rejected | Retention, exact artifact ownership, legal hold and retirement: D-05/D-14 |
| Audit events | Append only through service; immutable rows | Retention and reconstructible policy/suppression history: F-029 |
| Policy/suppression state | Current rows overwritten with version increment | Immutable revisions/intervals or approved reduced history contract: T-007/T-029 |
| Confirmation tokens | Hashed; cleared on success; expired hashes may remain | TTL cap, resend/supersession/cancel and expiry cleanup: D-02/T-001/T-003 |
| Outbox | Retained after publication; attempts/lease state updated | Archive/purge/replay horizon, poison-event handling, broker/consumer reconciliation: D-05/D-13/F-009/F-010/F-020 |
| Idempotency ledger | Retained without expiry | Key retention must align with accepted retry and data-retention contract: D-11/F-025 |

No application retention job, database backup definition, PITR policy, recovery automation or timed restore result is included. This is **F-022 / T-022 / AT-022 / G-022**, with **D-08** requiring named RPO/RTO and **D-09** requiring operational ownership. It does not prove that the organization has no external backups; it means the audit has no acceptable evidence of them.

Recovery must reconcile permission state, not only row counts. A restore from before a withdrawal can expose an earlier grant unless the restored ledger is reconciled with later accepted changes and consumer state before enforcement resumes. The release evidence must therefore show restoration of consent, receipt, audit, policy/suppression revisions, pending outbox and idempotency together; verification of receipt hashes; replay/deduplication of missing events; and a documented point at which current decisions become safe. The exact tool/commands depend on the owned universe and must not be fabricated from this ZIP (**F-001 / F-010 / F-022 / F-029**).

## 7. Sizing, query cost and capacity evidence

No production row counts, tenant distribution, transaction rate, evidence-size distribution or latency measurements were supplied. Database size and throughput targets are **DECISION REQUIRED: D-08**. Use measured representative row/index sizes and the following source-derived write amplification when building the capacity model:

| Operation | New logical rows on first success | Updates / caveats |
|---|---:|---|
| Governed grant/deny | 4 | Consent, receipt, audit, outbox |
| Governed grant/deny with new key | 5 | Above plus idempotency ledger; exact replay should not create more |
| Double-opt-in start | 3 | Pending consent, audit, outbox |
| Double-opt-in confirm | 3 | Receipt, audit, outbox; existing consent updated |
| First withdrawal | 2 | Audit/outbox; existing consent updated |
| Registry/suppression create | 3 | Registry/state row, audit, outbox |
| Registry/suppression update | 2 | Audit/outbox plus current row update |

These are application row counts, not storage-byte multipliers. Include primary/secondary indexes, JSON and text sizes, retained history, updates/compaction, replication, backup/PITR, outbox backlog and broker storage when sizing the actual universe. Retention assumptions must match D-05/D-11/D-13; do not reduce evidence retention merely to meet an invented storage budget.

An allowed governed evaluation can perform five sequential lookups: suppression, effective consent, purpose, lawful basis and notice. A 100-item batch can therefore require about 500 database round trips. That is a static upper-path operation count, not a measured latency result (`internal/service/service.go:237-254,353-384`). Query-plan/load tests should include hot subjects, large tenants, many historical consent rows, mixed suppression scopes and a growing pending outbox; optimization must preserve policy/withdrawal consistency.

The API adapter's configured pool maximum is 25 per pod. The HPA permits 20 API pods, giving **25 × 20 = 500 theoretical API connections** at that steady replica setting, before worker pools, rollout surge, migrations, operators and other clients. The baseline three API replicas could have a configured maximum of 75. These figures are arithmetic capacity inputs, not proof the universe supports 500 connections or that the application actually opens them (`internal/store/postgres.go:22-25`; `deploy/k8s.yaml:14-19,121-129`). The worker creates a separate pgx pool whose effective budget must be measured (`cmd/outbox-worker/main.go:177-183`). See the [NFR report](13_NON_FUNCTIONAL_REQUIREMENTS_AUDIT.md) for workload and cancellation requirements.

## 8. Database acceptance and engineer handover

Do not close these findings merely because SQL applies or unit tests pass. The detailed ticket definitions remain in [16](16_DETAILED_IMPLEMENTATION_BACKLOG.md); this table identifies the database evidence needed to close them without duplicating the backlog.

| Concern | Final finding / task | Acceptance / gate |
|---|---|---|
| No stale intent reversal; coherent current preference ordering | F-001 / T-001; F-006 / T-006 | AT-001 / G-001; AT-006 / G-006 |
| Canonical, verifiable, complete receipt/notice evidence | F-004 / T-004; F-005 / T-005 | AT-004 / G-004; AT-005 / G-005 |
| Policy transitions and reconciled legacy eligibility | F-007 / T-007; F-008 / T-008 | AT-007 / G-007; AT-008 / G-008 |
| Fenced leases and ordered/reconcilable event application | F-009 / T-009; F-010 / T-010 | AT-009 / G-009; AT-010 / G-010 |
| Bounded database work and schema-aware readiness | F-012 / T-012; F-015 / T-015 | AT-012 / G-012; AT-015 / G-015 |
| Real YSQL tests and complete migration CI | F-016 / T-016; F-017 / T-017 | AT-016 / G-016; AT-017 / G-017 |
| Retention, backup/PITR and safe recovery | F-022 / T-022 | AT-022 / G-022 |
| Tenant/state constraints and immutable change history | F-028 / T-028; F-029 / T-029 | AT-028 / G-028; AT-029 / G-029 |
| Explicit replay and development-adapter behavior | F-025 / T-025; F-026 / T-026 | AT-025 / G-025; AT-026 / G-026 |
| Obsolete non-atomic internal write path | F-031 / T-031 | AT-031 / G-031 |

The next schema plan should resolve D-03/D-04/D-05 together so current-state generation, policy history and canonical evidence agree. Database/consumer recovery design depends on D-07/D-08/D-09/D-13; regional retention/evidence decisions depend on D-14. Start with the [execution order](22_ENGINEERING_EXECUTION_ORDER.md), retain every migration and verification result, and require the corresponding gate evidence before releasing the database-backed service.
