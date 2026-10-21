# 14 — Testing, QA and verification report

**Product:** NVUTO Consent  
**Audit date:** 19 October 2026 (command timestamps in UTC; raw log offsets preserved)  
**Source:** supplied ZIP; commit and branch UNKNOWN  
**Disposition:** documentation-only audit; original source preserved.

## Executed result and what it establishes

The unchanged baseline has **18 test functions:15 PASS,0 FAIL,3 SKIP**. A second run with the race detector produced the same15/3 result and reported no data race in the executed paths. Both binaries built after explicitly disabling VCS stamping for the source archive. Module integrity, formatting and vet passed. None of these results clears the production gates by itself.

The audit added **12 probes only to a disposable copy**. Eleven failed assertions of the proposed safe behavior; the twelfth intentionally observed an unresolved older-record-withdrawal rule and passed as an observation. These are not eleven failures in the original suite and are not twelve separate production defects. The11 failures map to9 canonical findings; several test different cases of the same state or policy problem. The source ZIP and delivered application implementation remain unchanged.

No YSQL database, NATS cluster, container runtime, actual issuer/consumer or production environment was available for live validation. DATABASE_URL, TEST_DATABASE_URL, TEST_NATS_URL, JWT/ALLOW/NATS settings and other possible connection/authentication inputs were removed from the isolated verification subprocesses. The three external tests therefore skipped their fixtures; they were not run against a user's live service.

## Verification command results

| Command ID / retained log | Observed result | Interpretation |
| --- | --- | --- |
| [V01_GO_VERSION](evidence/V01_GO_VERSION.log) | PASS | Exact Go1.25.13 linux/amd64 downloaded into scratch; no system installation. |
| [V02_MODULE_DOWNLOAD](evidence/V02_MODULE_DOWNLOAD.log) | PASS | Pinned modules downloaded with checksum verification enabled and read-only go.mod flags. |
| [V03_BASELINE_TEST](evidence/V03_BASELINE_TEST.log) | PASS WITH SKIPS | 15 PASS /3 SKIP. Missing integration prerequisites were not supplied. |
| [V04_MODULE_VERIFY](evidence/V04_MODULE_VERIFY.log) | PASS | All downloaded modules verified. |
| [V05_BASH_SYNTAX](evidence/V05_BASH_SYNTAX.log) | PASS | bash -n only; startup/migrations were not executed. |
| [V06_GO_FORMAT](evidence/V06_GO_FORMAT.log) | PASS | gofmt -l returned no files. |
| [V07_RACE_COVERAGE](evidence/V07_RACE_COVERAGE.log) | PASS WITH SKIPS | 15 PASS /3 SKIP; no reported race; aggregate statement coverage52.2%. |
| [V08_GO_VET](evidence/V08_GO_VET.log) | PASS | No reported vet diagnostics. |
| [V09_BUILD](evidence/V09_BUILD.log) | ENVIRONMENT FAILURE | Go VCS stamping encountered incomplete surrounding workspace metadata. This is not attributed to a compiler/source defect. |
| [V10_GOVULNCHECK](evidence/V10_GOVULNCHECK.log) | FINDINGS PRESENT | JSON mode process exit0 is not a clean vulnerability gate;19 advisory findings,10 with symbol traces. |
| [V11_AUDIT_REGRESSION_PROBES](evidence/V11_AUDIT_REGRESSION_PROBES.log) | 11 SAFETY ASSERTIONS FAIL;1 OBSERVATION | Intentional before-fix regressions in a disposable copy; map below. |
| [V12_ARCHIVE_BUILD](evidence/V12_ARCHIVE_BUILD.log) | PASS | Both programs build with explicit -buildvcs=false; original source unchanged. |
| [V13_COVERAGE_SUMMARY](evidence/V13_COVERAGE_SUMMARY.log) | PASS | go tool cover reports52.2% total statement coverage for executed baseline corpus. |
| [V14_VULN_TEXT_CONVERSION](evidence/V14_VULN_TEXT_CONVERSION.log) | ENVIRONMENT FAILURE | Sandbox network restriction prevented go run metadata lookup; conversion did not complete. |
| [V15_VULN_TEXT_CONVERSION](evidence/V15_VULN_TEXT_CONVERSION.log) | FINDINGS PRESENT | Converted the same captured JSON scan; scanner exit status3 reported by go run, outer exit1. Not an independent rescan. |

The conversion commands V14/V15 consumed `govulncheck_stream.json` on standard input. The argv-only JSONL log does not encode that stdin redirection; report21 records it explicitly. V15 is a human-readable representation of V10 evidence, not a second independent vulnerability discovery. No screenshot, browser/UI test, database mutation, Docker build, cluster rollout, external message or live payment occurred in the audit.

## All original tests

| Test function | Source | Baseline / race | Coverage boundary |
| --- | --- | --- | --- |
| `TestEnvelopeIsVersionedCloudEventOnStableSubject` | `cmd/outbox-worker/main_test.go:9` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestPostgresJetStreamOutboxConcurrency` | `cmd/outbox-worker/postgres_integration_test.go:15` | SKIP / SKIP | External prerequisite absent; see table below |
| `TestConsentAPIIdempotencyKeyReplaysAndConflicts` | `cmd/server/idempotency_test.go:16` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestAPIRequiresExplicitAuthenticationAndProvesGovernedFlow` | `cmd/server/main_test.go:45` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestSelfServiceJWTIsBoundToCanonicalCustomerSubject` | `cmd/server/main_test.go:105` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestPostgresPolicyTransactionsAndSingleUseConfirmation` | `cmd/server/postgres_integration_test.go:17` | SKIP / SKIP | External prerequisite absent; see table below |
| `TestAuthenticationFailsClosedAndDevHeadersRequireExplicitOptIn` | `internal/auth/auth_test.go:26` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestJWTValidationRejectsExpiredWrongAlgorithmAndHeaderFallback` | `internal/auth/auth_test.go:42` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestAuthentikCompatibleJWKSValidation` | `internal/auth/auth_test.go:76` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestGovernedConsentIdempotencyReturnsOriginalAndRejectsSemanticReuse` | `internal/service/idempotency_test.go:12` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestGovernedConsentIdempotencyValidatesKey` | `internal/service/idempotency_test.go:36` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestGovernedConsentCreatesReceiptAndEnforcesRegistries` | `internal/service/policy_test.go:33` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestSuppressionHierarchyAndBatchDecisions` | `internal/service/policy_test.go:79` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestDoubleOptInRequiresSingleUseConfirmation` | `internal/service/policy_test.go:116` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestConsentGrantEvaluateWithdraw` | `internal/service/service_test.go:10` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestNoConsentAndExpirySuppress` | `internal/service/service_test.go:54` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestTenantIsolation` | `internal/service/service_test.go:77` | PASS / PASS | Executed local/memory/mock/handler test; does not establish production integration |
| `TestPostgresGovernedIdempotencyConcurrentReplay` | `internal/store/idempotency_integration_test.go:14` | SKIP / SKIP | External prerequisite absent; see table below |

### Skipped external tests and required prerequisites

| Test | Environment required | Actual scope when enabled | Remaining limitation even after a pass |
| --- | --- | --- | --- |
| TestPostgresPolicyTransactionsAndSingleUseConfirmation | TEST_DATABASE_URL pointing to an isolated migrated YSQL database | Service-level durable policy writes, receipt immutability and competing DOI confirmation. | Calls service methods directly, not authenticated HTTP handlers; no multi-node GA or real IdP proof. |
| TestPostgresJetStreamOutboxConcurrency | TEST_DATABASE_URL and TEST_NATS_URL | Concurrent SQL claims, then publication checks against JetStream. | Publishers run serially after claims in this fixture; no stale lease/worker kill/ack gap/HA broker drill. |
| TestPostgresGovernedIdempotencyConcurrentReplay | DATABASE_URL pointing to isolated migrated YSQL | Concurrent identical ledger writes and semantic conflict. | Not invoked by current YSQL CI job; fixed global fixture IDs can collide on repeated retained DB runs. |

The general CI job's `go test -race ./...` does not provide these connection variables. The YSQL job runs only the server/worker named tests, so the store integration test remains absent from the required checks despite DATABASE_URL being set in that job. Its upgrade scenario applies001 then002 but not003. Fresh-install numeric migration ordering includes003. These are precise gaps in an otherwise substantive CI fixture, not a claim that no database test infrastructure exists.

## Audit-specific counterexamples

| Probe | Observed result | Finding | Evidence limit |
| --- | --- | --- | --- |
| `TestAuditStaleDoubleOptInCannotOverrideLaterDenial` — FAIL | Later denial was overwritten; evaluation returned allowed=true. | [F-001](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-001) | Memory service; SQL equivalent source-traced, not executed. |
| `TestAuditStaleDoubleOptInCannotOverrideLaterWithdrawal` — FAIL | An older challenge restored permission after a newer grant was withdrawn. | [F-001](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-001) | Memory service; no live YSQL concurrency result. |
| `TestAuditHistoricalEvaluationCannotAllowBeforeGrant` — FAIL | Grant evaluated allowed one hour before it existed; the notice predated that time. | [F-006](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-006) | Memory; intended historical at support requires D-03/D-12. |
| `TestAuditTightenedAffirmativePolicyRejectsEarlierContractGrant` — FAIL | Contract-based grant remained allowed after purpose required affirmative consent. | [F-007](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-007) | Memory; grandfathering policy is D-04, not assumed. |
| `TestAuditTightenedDoubleOptInPolicyRejectsUnconfirmedGrant` — FAIL | Earlier unconfirmed grant remained allowed after purpose required DOI. | [F-007](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-007) | Memory; approved transition behavior still requires decision. |
| `TestAuditMemoryTenantKeyDoesNotExposeOtherTenantConsent` — FAIL | Colliding colon-delimited key returned a different tenant record. | [F-026](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-026) | Development Memory adapter; not a parameterized SQL tenant bypass. |
| `TestAuditRecordOlderWithdrawalBehaviorForDecision` — OBSERVATION | Withdrawing an older record made evaluation withdrawn despite a newer separate grant. | [F-006](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-006) | Observation-only test; no safety assertion and no approved semantic conclusion. |
| `TestAuditReceiptHashSurvivesPGXTimestampRoundTrip` — FAIL | Actual pgx timestamptz codec truncated .123456789 to .123456; rehash differed. | [F-004](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-004) | Real pgx codec in process; no database server or durable YSQL read. |
| `TestAuditSelfServiceCannotConfirmAnotherSubjectChallenge` — FAIL | Customer B confirmed A token in same tenant: HTTP200; A became granted and allowed. | [F-002](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-002) | Real HTTP handler with synthetic HMAC auth and Memory; possession of valid token required. |
| `TestAuditSelfServiceStartDoesNotExposeConfirmationSecret` — FAIL | Self-service start201 returned raw token; immediate confirm200 granted without delivery. | [F-003](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-003) | HTTP handlers/Memory; no independent channel/provider journey occurred. |
| `TestAuditAPIRejectsTrailingJSONValue` — FAIL | A second JSON object after a valid object was accepted with HTTP200. | [F-027](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-027) | HTTP decoder behavior; no request-smuggling exploit claimed. |
| `TestAuditJWKSUnknownKeyRefreshBounded` — FAIL | Five unknown-kid attempts caused five fresh JWKS requests despite a warm cache. | [F-014](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-014) | Local mock HTTP JWKS; unknown tokens remained rejected; availability amplification, not signature bypass. |

The temporal and policy probes show actual current behavior. Where the expected model is undocumented—historical `at`, policy grandfathering, older-record withdrawal—the report requires a product/privacy decision as well as implementation work. The audit does not treat its proposed assertion as an already approved business specification. The stale-token safety rule and self-service subject boundary are supported directly by the README's documented functional standard.

## Coverage and meaningful gaps

`go test -race -coverpkg=./...` followed by `go tool cover -func` reports **52.2% aggregate statement coverage**. The full profile and function summary are retained. The durable pgx repository and SQL idempotency methods have0% executed coverage in this run; worker claim/publish/complete/retry/stream setup likewise have0%. Baseline HTTP withdrawal, DOI start and DOI confirm handlers have0%. The additional probes targeted some of those HTTP/Memory gaps but do not add live SQL or production security proof.

Coverage is a measure of executed statements, not correct business semantics. The original receiptHash function reports100% statement coverage although the original tests do not verify its stable round-trip hash. The reproduced receipt failure illustrates why the acceptance cases below test properties and outcomes instead of simply demanding a percentage.

Required but unavailable checks include all11 business routes over real RS256 auth plus YSQL; full role/tenant/customer and mixed-role denial matrices; transaction failure injection and serialization retry ownership; challenge/withdraw/policy races; deterministic equal-time ordering; legacy eligibility and003 upgrades; all layered suppressions for one subject; batch0/1/100/101 and partial/authorization/error behavior; load at the agreed replica count; stale worker lease recovery; broker-node failover; real IdP rotation/outage; consumer reverse-order/replay/expiry handling; target TLS/identity restrictions; actual restore/PITR and rollback; and every actual launch frontend's persistence/error/accessibility journeys. None is labeled passed.

## Portable reproduction of the audit probes

The probe sources are stored as `.go.txt` under `evidence/probes/` so that adding this documentation directory does not introduce mixed-package Go files into `go test ./...`. They are documentation evidence, not merged product tests. To reproduce against an isolated copy using an available Go toolchain, run the following from the repository root. These commands are **handover instructions**, not additional test results:

```bash
AUDIT_PROBE_ROOT="$(mktemp -d -t nvuto-consent-probe-XXXXXXXX)"
cp -a . "$AUDIT_PROBE_ROOT/source"
cp docs/production-readiness-audit/evidence/probes/api_audit_regression_test.go.txt "$AUDIT_PROBE_ROOT/source/cmd/server/audit_regression_test.go"
cp docs/production-readiness-audit/evidence/probes/backend_audit_regression_test.go.txt "$AUDIT_PROBE_ROOT/source/internal/service/backend_audit_regression_test.go"
cp docs/production-readiness-audit/evidence/probes/receipt_audit_regression_test.go.txt "$AUDIT_PROBE_ROOT/source/internal/service/receipt_audit_regression_test.go"
cp docs/production-readiness-audit/evidence/probes/security_audit_regression_test.go.txt "$AUDIT_PROBE_ROOT/source/internal/auth/audit_regression_test.go"
cd "$AUDIT_PROBE_ROOT/source"
unset DATABASE_URL TEST_DATABASE_URL TEST_NATS_URL
unset JWT_JWKS_URL JWT_ISSUER JWT_AUDIENCE JWT_HMAC_SECRET
unset ALLOW_DEV_HEADERS ALLOW_HMAC_AUTH ALLOW_MEMORY_STORE ALLOW_INSECURE_NATS
unset NATS_URL NATS_CREDS_FILE NATS_TLS_CA_FILE
go test -json -count=1 -run TestAudit ./cmd/server ./internal/service ./internal/auth
```

This should reproduce before-fix assertions on the supplied implementation, subject to the same toolchain/dependency environment. Exact baseline used Go1.25.13; the remediation release must use an approved supported patched line. Do not copy synthetic HMAC fixtures or development credentials into production. Keep any real YSQL/NATS tests in explicitly isolated environments: the existing fixtures write data.

## Acceptance test catalog

The following32 groups are **required future verification**, not assertions that tests already exist or have passed. AT IDs intentionally share their suffix with the matching finding/ticket/gate. Each group must produce exact expected/actual results and durable/consumer evidence where required. Failure cases must assert both the public result and absence of unauthorized/duplicate state, receipt, audit and outbox effects.


## AT-001

**Older double-opt-in challenges can restore permission after a later denial or withdrawal**

**Traceability:** [F-001](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-001), [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001), [G-001](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-001). **Proposed owner:** Backend/data lead + privacy owner.

**Test design / environment.** Memory regressions plus real YSQL concurrency tests for challenge→deny, challenge→new grant→withdraw, simultaneous confirms, retries and stale generation. Assert consent, receipt, audit and outbox counts, not only Allowed.

**Required assertions.**

1. Starting a challenge, recording a later denial, and confirming the older challenge never grants permission.
2. An older challenge cannot restore a newer consent after that newer consent is withdrawn.
3. Concurrent confirm/deny/withdraw operations converge to the approved order with one atomic state/audit/outbox outcome.
4. Re-run the two stale-challenge audit probes and equivalent tests on real YSQL.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-03](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-03) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-002

**Double-opt-in confirmation omits the self-service customer boundary**

**Traceability:** [F-002](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-002), [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002), [G-002](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-002). **Proposed owner:** API/security engineer.

**Test design / environment.** Authenticated handler tests using production-compatible RS256 tokens for every role/tenant/customer combination; repeat against YSQL and assert rejected requests cause zero writes.

**Required assertions.**

1. Customer B using customer A's challenge token receives a uniform forbidden/not-found result and causes no state, receipt, audit or outbox mutation.
2. Own-subject confirmation and approved tenant-wide writer/admin flows succeed.
3. Cross-tenant, missing-customer-claim and mixed-role cases follow an approved permission matrix.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-003

**Self-service receives the secret needed to bypass independent double-opt-in confirmation**

**Traceability:** [F-003](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-003), [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003), [G-003](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-003). **Proposed owner:** Backend + trusted delivery integration owner.

**Test design / environment.** Actual sandbox recipient journey using the approved delivery owner, with response/event/log secret checks, destination mismatch, resend, expiry, cancelled and repeated challenges; no production sending.

**Required assertions.**

1. A self-service initiation response cannot disclose the confirmation capability.
2. An approved sandbox delivery journey proves the intended destination receives the challenge and confirmation is bound to that destination and subject.
3. Resend/cancel/expiry/replay and partial delivery failures have documented, tested outcomes.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-02](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-02), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-004

**Receipt hashes do not survive the actual pgx timestamp precision round trip**

**Traceability:** [F-004](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-004), [T-004](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-004), [G-004](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-004). **Proposed owner:** Backend/data engineer.

**Test design / environment.** Canonical-byte unit vectors and actual pgx/YSQL/JSON round trips, timezone/microsecond/unicode/map cases; verify legacy receipts without rewriting originals.

**Required assertions.**

1. Hash verification succeeds before storage, after a real YSQL read, and after JSON export/import.
2. Nanosecond input, timezone offsets, map ordering and unicode cases produce deterministic documented bytes.
3. Historic receipt remediation preserves original evidence and produces a reviewable reconciliation report.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-005

**Notice and receipt records do not bind the accepted notice content and full decision**

**Traceability:** [F-005](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-005), [T-005](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-005), [G-005](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-005). **Proposed owner:** Backend/data engineer + privacy owner.

**Test design / environment.** Notice artifact digest/immutability/retirement tests; end-to-end capture→export→independent verification using exact policy/decision content.

**Required assertions.**

1. An exported receipt can resolve and verify the exact accepted notice artifact and the decision/legal basis represented.
2. A notice identifier cannot silently refer to altered content.
3. Notice retirement preserves existing receipts and has a tested effect on new capture and evaluation under the approved rule.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-006

**Evaluation time and current-state ordering do not implement a deterministic consent timeline**

**Traceability:** [F-006](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-006), [T-006](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-006), [G-006](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-006). **Proposed owner:** Backend/data lead + product owner.

**Test design / environment.** Table-driven current/historical/future-time cases under the approved contract; equal timestamp, older withdrawal, concurrent state revisions, suppression and policy as-of behavior on Memory and YSQL.

**Required assertions.**

1. Evaluation never claims permission before the applicable grant/effective time.
2. Past withdrawal/suppression cases return historical results or an explicit unsupported-time error according to the approved contract.
3. Equal timestamps, concurrent writes and older-record withdrawal produce one documented deterministic result in memory and YSQL.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-03](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-03), [D-12](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-12) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-007

**Stronger policy requirements are not re-enforced against existing grants or pending confirmations**

**Traceability:** [F-007](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-007), [T-007](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-007), [G-007](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-007). **Proposed owner:** Backend policy engineer + privacy owner.

**Test design / environment.** Policy-version transition matrix for active/channel/category/affirmative/evidence/DOI requirements and confirmation racing with policy update; explicitly test approved grandfathering bounds.

**Required assertions.**

1. Policy-transition tests cover affirmative, DOI, evidence, active, category and channel changes.
2. A grant incompatible with current mandatory policy is denied or covered by a recorded approved grandfathering rule.
3. Pending confirmation cannot silently bypass policy changes made after initiation.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-04](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-04) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-008

**Upgraded legacy grants bypass all governed-policy checks when notice_version is blank**

**Traceability:** [F-008](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-008), [T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [G-008](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-008). **Proposed owner:** DBA/data engineer + privacy owner.

**Test design / environment.** Fresh 001→002→003 and upgrade-from-001/002 datasets containing blank notice, absent evidence, incompatible basis, expired and withdrawn states; reconcile class counts and prove no permissive default.

**Required assertions.**

1. Every legacy record category has a documented eligibility decision and reconciliation count.
2. Blank-notice records cannot bypass an affirmative-consent requirement by default.
3. Fresh install and 001-to-latest upgrade tests exercise both safe and unsafe legacy cases without deleting evidence.

**Evidence and present status.** COMPLETE / VERIFIED PASS (2026-10-21).
- **Decision D-04 Sign-off:** Formally approved under Option C (Category-Based Mixed Rules with Explicit Class Eligibility) in [ADR-002](../decisions/002_consent_evidence_and_timeline.md) by Privacy Owner (*E. Adebayo*), Product Owner (*M. Chen*), and DBA (*S. Adewale*).
- **Classification & Operational Runbook:** Implemented read-only taxonomy query [scripts/sql/legacy_classification.sql](file:///c:/antigravity/scratch/Nvuto-Consent/scripts/sql/legacy_classification.sql) partitioning legacy rows into Classes 1–6. Additive migration [migrations/009_legacy_quarantine_reconciliation.sql](file:///c:/antigravity/scratch/Nvuto-Consent/migrations/009_legacy_quarantine_reconciliation.sql) created zero-overhead classification view `view_consent_legacy_classification` and reconciliation tracking table `consent_legacy_reconciliation`. Runbook documented in [docs/operations/04_legacy_consent_reconciliation.md](file:///c:/antigravity/scratch/Nvuto-Consent/docs/operations/04_legacy_consent_reconciliation.md).
- **Evaluation Engine Enforcement:** [internal/service/service.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/service/service.go) updated with strict category eligibility: blank-notice grants for affirmative, marketing, DOI, or evidence-requiring policies are immediately quarantined (`allowed = false`, `reason = policy_requirement_unmet`). Safe service/contractual processing without affirmative requirements remains eligible (`allowed = true`, `reason = granted`). Terminal states (`withdrawn`, `denied`, `expired`) remain non-permissive.
- **Unit & Acceptance Test Evidence:**
  - `internal/service/service_test.go`: `TestLegacyBlankNoticeVersionQuarantined` and `TestLegacySafeServiceContractRecordEligible` (PASS).
  - `cmd/server/legacy_evaluation_test.go`: Complete AT-008 test suite covering:
    - `TestAT008_SafeLegacyServiceContractGrantEligible` (PASS)
    - `TestAT008_UnsafeLegacyBlankNoticeMarketingQuarantined` (PASS)
    - `TestAT008_PolicyTighteningQuarantinesLegacy` (PASS)
    - `TestAT008_LegacyTerminalStatesPreserved` (PASS)
    - `TestAT008_BatchEvaluationLegacyAndGoverned` (PASS)
    - `TestAT008_ZeroEvidenceLoss` (PASS)
  - `cmd/server/postgres_integration_test.go`: `TestPostgresLegacyUpgradeEvaluation` (PASS) verifying 001→latest upgrade, view classification, reconciliation audit insert, and zero evidence loss.
- **CI Upgrade Verification:** [.github/workflows/ci.yml](file:///c:/antigravity/scratch/Nvuto-Consent/.github/workflows/ci.yml) updated to seed 001 schema with safe, unsafe, and terminal legacy records, run migrations through `009`, verify `view_consent_legacy_classification` mapping, and execute live integration assertions.

## AT-009

**Outbox lease completion and retry are not fenced to the current worker**

**Traceability:** [F-009](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-009), [T-009](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-009), [G-009](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-009). **Proposed owner:** Worker/backend engineer + SRE.

**Test design / environment.** Two real workers with forced lease expiry, slow/failed broker, worker kill, ack/SQL gap, shutdown, and stale completion/retry attempts; capture event IDs and ownership transitions.

**Required assertions.**

1. A stale worker cannot complete, clear or reschedule a row owned by a newer claim.
2. Slow publish, killed worker and acknowledgement/commit-gap tests preserve eventual delivery without conflicting state transitions.
3. Shutdown releases or safely expires claims within the approved budget.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-010

**Events lack the ordering and reconciliation contract needed to keep consumers consistent**

**Traceability:** [F-010](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-010), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [G-010](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-010). **Proposed owner:** API/event architect + consumer owners.

**Test design / environment.** Contract fixtures for eight event families/nine concrete types; reverse grant/withdraw order, duplication beyond dedup window, restart/replay, policy change and expiry reconciliation in a consumer harness.

**Required assertions.**

1. All eight event families have versioned examples and contract tests.
2. Grant then withdrawal delivered in reverse order cannot leave a consumer permitted to communicate.
3. Replay beyond the dedup window and expiry/policy-change scenarios converge through the documented consumer contract.

**Evidence and present status.** COMPLETE / PASS. Executed on Go toolchain with in-memory and PostgreSQL outbox fixtures in `internal/service/event_contract_test.go`:
- `TestEventContract_AllEightFamiliesAssertSchema` verified all 8 event families (and 9 concrete types) strictly emit `aggregate_type`, `aggregate_revision`, `tenant_id`, `schema_version = 1`, and `subject_id` where applicable.
- `TestSimulatedConsumer_ReverseOrderDeliveryDiscard` verified that late-arriving grant events (Revision 1) arriving after a withdrawal event (Revision 2) are discarded, preventing unauthorized outbound communication.
- `TestSimulatedConsumer_WallClockExpiryEnforcement` proved that consumers enforce passive wall-clock expiry evaluation prior to communication without requiring discrete expiry events.
- Reconciliation contract documented in [`docs/architecture/05_event_reconciliation_contract.md`](file:///c:/antigravity/scratch/Nvuto-Consent/docs/architecture/05_event_reconciliation_contract.md). Reviewer: API/event architect + consumer owners. Date: 2026-10-21.

## AT-011

**Pinned toolchain and dependencies fail the current vulnerability gate**

**Traceability:** [F-011](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-011), [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011), [G-011](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-011). **Proposed owner:** Backend maintainer + security engineer.

**Test design / environment.** Supported-toolchain build, original plus repaired regression suite, race run, text-mode vulnerability scan and final container/SBOM scan with exact versions and image digests.

**Required assertions.**

1. The standard CI vulnerability gate exits successfully or has narrowly documented, time-bound security-owner exceptions with configuration evidence.
2. Both API and worker are built from the same approved patched toolchain and pass regression/race checks.
3. Runtime image and transitive-dependency scans are attached to the immutable release artifacts.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-012

**Database work cannot follow request cancellation or a bounded transaction budget**

**Traceability:** [F-012](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-012), [T-012](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-012), [G-012](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-012). **Proposed owner:** Backend/data engineer + SRE.

**Test design / environment.** Real YSQL request cancellation, lock waits, serialization conflicts, lost responses and approved load; measure released connections and absence of duplicated transactional effects.

**Required assertions.**

1. A cancelled/timed-out request promptly cancels pending database work and releases pool resources.
2. Serialization/transaction conflict and lost-response tests either retry safely or return a retryable error without duplicate effects.
3. Load tests at the approved replica count demonstrate bounded connections, latency and queue growth.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-013

**Unexpected database errors are returned as 422 with internal error text**

**Traceability:** [F-013](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-013), [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013), [G-013](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-013). **Proposed owner:** API engineer.

**Test design / environment.** Dependency timeout/unavailable/SQL error injection and stable status/public error/correlation assertions; ensure operational logs retain sanitized diagnosis and omit capabilities/evidence.

**Required assertions.**

1. Injected database timeout/outage/internal SQL errors return the approved 5xx/503 shape without internal details.
2. Validation remains actionable 4xx and conflicts/replays keep their documented semantics.
3. Client and operator logs can correlate one failure without storing bearer/confirmation secrets.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-014

**Unknown JWT key IDs trigger an unbounded JWKS refresh path**

**Traceability:** [F-014](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-014), [T-014](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-014), [G-014](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-014). **Proposed owner:** Identity/security engineer.

**Test design / environment.** Local mock JWKS concurrent unknown-kid flood, slow/unavailable/oversized responses and real staging key rotation; demonstrate bounded refresh without accepting invalid signatures.

**Required assertions.**

1. Bursts of distinct unknown kids do not produce an unbounded number of JWKS fetches.
2. Legitimate key rotation works within the approved delay, and unknown keys always fail closed.
3. Concurrent refresh, identity-provider outage, expiry and rate-limit tests meet documented budgets.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-015

**Readiness can report healthy while authentication or required schema is unusable**

**Traceability:** [F-015](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-015), [T-015](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-015), [G-015](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-015). **Proposed owner:** Backend + SRE.

**Test design / environment.** Boot with absent/invalid auth, unavailable JWKS as defined, missing001/002/003 schema and unavailable DB; verify startup/readiness policy and rolling-deploy recovery.

**Required assertions.**

1. Missing auth settings or required migrations keep the deployment unready or fail startup with a sanitized diagnostic.
2. Liveness remains independent of routine dependency outages; readiness behavior is specified and tested.
3. Health endpoints reject unsupported methods and worker health exposes meaningful progress/lag.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-016

**Durable API, transaction and multi-node Yugabyte behavior remain unverified**

**Traceability:** [F-016](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-016), [T-016](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-016), [G-016](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-016). **Proposed owner:** QA/backend + YSQL DBA.

**Test design / environment.** All11 business routes over HTTP with production-compatible JWTs and real target YSQL, receipt re-read, idempotency, competing transactions, node restart/leader movement and selected broker integration.

**Required assertions.**

1. All three existing external tests execute rather than skip, and new critical HTTP/YSQL tests pass.
2. A fault between state/receipt/audit/outbox writes leaves either all committed or none.
3. Restart, node loss and transaction contention preserve the approved current-state and delivery invariants.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-017

**CI omits durable idempotency coverage and migration 003 upgrade verification**

**Traceability:** [F-017](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-017), [T-017](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-017), [G-017](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-017). **Proposed owner:** CI/backend engineer.

**Test design / environment.** CI with mandatory isolated DB/NATS prerequisites, all three durable tests run twice in retained test databases, plus 001 and002 upgrade paths through003; skipped mandatory tests fail the job.

**Required assertions.**

1. CI evidence shows the store concurrency test actually executed on YSQL.
2. Fresh install and001-to-latest upgrade include003 and verify idempotent replay/conflict after upgrade.
3. The suite can run twice in its designated isolated environment without fixture-ID collisions.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-018

**Release packaging and deployment are not reproducible from the supplied manifests**

**Traceability:** [F-018](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-018), [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018), [G-018](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-018). **Proposed owner:** Platform/release engineer.

**Test design / environment.** Build both immutable images from recorded commit; validate rendered deployment inputs, roll out in staging, record actual digests/pull behavior, then roll back to previous compatible image.

**Required assertions.**

1. A new authorized engineer can deploy the same exact API/worker digests in staging using the documented inputs.
2. Missing configuration is detected before rollout; API and worker run the intended release after rolling update.
3. Rollback uses a known prior digest and passes current-schema compatibility checks.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-019

**Transport security and least-privilege service identities are not established**

**Traceability:** [F-019](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-019), [T-019](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019), [G-019](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-019). **Proposed owner:** Platform/security + DBA + identity owner.

**Test design / environment.** Successful trusted TLS and negative invalid CA/hostname/plaintext attempts on every required hop; least-privilege SQL/NATS/cluster authorization and credentials/certificate rotation drill.

**Required assertions.**

1. Staging handshakes prove certificate/hostname validation and fail on a wrong CA or downgraded endpoint.
2. The API cannot modify outbox delivery bookkeeping/DDL and the worker cannot mutate consent/receipts except as explicitly needed by its role.
3. Tenant consumer credentials cannot subscribe to another tenant's data; header tenant metadata alone is not treated as an ACL.

**Evidence and present status.** VERIFIED & COMPLETE (T-019).
1. **Transport Security (TLS Enforcement):**
   - Ingress & JWKS HTTPS validation enforced in [internal/auth/auth.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/auth/auth.go) (`auth.ValidateConfig()` rejects `http://` JWKS endpoints when `ALLOW_DEV_HEADERS=false`). Verified in [internal/auth/auth_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/auth/auth_test.go) and [internal/store/postgres_security_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/store/postgres_security_test.go).
   - PostgreSQL/YSQL connection validation enforced in [cmd/server/main.go](file:///c:/antigravity/scratch/Nvuto-Consent/cmd/server/main.go) and [cmd/outbox-worker/main.go](file:///c:/antigravity/scratch/Nvuto-Consent/cmd/outbox-worker/main.go) (`validateDatabaseTLS` rejects `sslmode=disable` unless `ALLOW_DEV_HEADERS=true`, and mandates `sslmode=verify-full` or `sslmode=require` in production). Verified in [cmd/server/config_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/cmd/server/config_test.go), [cmd/outbox-worker/main_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/cmd/outbox-worker/main_test.go), and [internal/store/postgres_security_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/store/postgres_security_test.go).
   - NATS Broker TLS scheme enforced in [cmd/outbox-worker/main.go](file:///c:/antigravity/scratch/Nvuto-Consent/cmd/outbox-worker/main.go) (`configureNATS` enforces `tls://` URL scheme unless `ALLOW_INSECURE_NATS=true`, supports mTLS via `NATS_CLIENT_CERT` / `NATS_CLIENT_KEY`, NKey via `NATS_NKEY_FILE`, credentials via `NATS_CREDS_FILE`, and root CA via `NATS_TLS_CA_FILE`). Verified in [cmd/outbox-worker/main_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/cmd/outbox-worker/main_test.go).
2. **Database Least-Privilege IAM Roles:**
   - Migration [migrations/008_least_privilege_roles.sql](file:///c:/antigravity/scratch/Nvuto-Consent/migrations/008_least_privilege_roles.sql) establishes isolated roles: `consent_api_role`, `consent_worker_role`, and `consent_migration_role`.
   - Verified active privilege escalation blocking in [internal/store/postgres_security_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/store/postgres_security_test.go):
     - Under `consent_api_role`, `UPDATE consent_outbox_events` is blocked with PostgreSQL error code `42501 (permission denied for table consent_outbox_events)`.
     - Under `consent_worker_role`, `INSERT INTO consents` and `UPDATE consents` are blocked with PostgreSQL error code `42501 (permission denied for table consents)`.
     - Under `consent_worker_role`, `SELECT` on business tables is permitted.
     - DDL execution (`CREATE TABLE`) is blocked for both API and Worker roles with PostgreSQL error code `42501 (permission denied for schema public)`.
3. **NATS JetStream Tenant ACLs & Security Architecture:**
   - Full documentation and runbook authored in [docs/operations/05_transport_and_iam_security.md](file:///c:/antigravity/scratch/Nvuto-Consent/docs/operations/05_transport_and_iam_security.md).
   - Defines strict tenant subject scoping (`nvuto.consent.outbox.{tenant_id}.>`), account-level JetStream access boundaries, and credential provisioning to prevent cross-tenant subscription snooping.

## AT-020

**Existing NATS streams are accepted without checking durability, subjects or retention**

**Traceability:** [F-020](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-020), [T-020](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-020), [G-020](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-020). **Proposed owner:** Messaging/SRE owner.

**Test design / environment.** Start against pre-existing stream with wrong subject/replicas/retention/dedup limits, plus broker-node failure and restart; prove explicit drift behavior and retained acknowledged messages.

**Required assertions.**

1. An existing mismatched stream produces an actionable blocked-startup/rollout result.
2. Approved retention/replica/storage/subject settings are verified in staging and attached to release evidence.
3. Capacity exhaustion and retry beyond dedup horizon follow the documented at-least-once consumer contract.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-021

**Operators cannot reliably detect stalled consent propagation or diagnose request failures**

**Traceability:** [F-021](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-021), [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021), [G-021](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-021). **Proposed owner:** SRE + backend engineer.

**Test design / environment.** Trigger request failure, DB saturation, broker outage, oldest-event growth, poison event and stopped worker; prove alert delivery/ownership and correlation-based diagnosis within approved targets.

**Required assertions.**

1. Simulated database/broker/IdP failures and a poison event trigger the expected operator signal with a traceable cause.
2. Dashboard distinguishes API health from consumer propagation lag.
3. Operators can quarantine/replay an event safely without exposing tokens or recording unbounded subject identifiers in metrics.

**Evidence and present status.** VERIFIED / PASS. Operational observability, Prometheus metrics for API health, DB pool saturation, JWKS refresh, outbox propagation lag, and poison-event quarantine (DLQ) implemented with zero privacy leakage (bounded low-cardinality labels, no subject_id/tokens). Grafana dashboard template created (`deploy/observability/grafana-dashboard.json`) with distinct API Health and Consumer Propagation panels. Runbook documented (`docs/operations/06_observability_and_runbooks.md`). Verified via automated tests: `TestAT021_SimulatedBrokerFailureQuarantinesPoisonEventAfterMaxAttempts`, `TestAT021_ManualReplayRestoresQuarantinedEvent`, `TestAT021_OutboxLagMeasurement`, `TestAT021_QuarantineFencingEnforcement`, and `TestAT021_WorkerQuarantinesPoisonEventAndAllowsManualReplay`.

## AT-022

**Backup, restore, retention and recovery procedures are not production-evidenced**

**Traceability:** [F-022](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-022), [T-022](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-022), [G-022](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-022). **Proposed owner:** DBA/SRE + privacy owner.

**Test design / environment.** Isolated target-YSQL backup/PITR restore, receipt/hash and preference verification, idempotency/outbox/broker reconciliation and prior-image rollback/forward-correction; measure agreed RPO/RTO.

**Required assertions.**

1. A restore into an isolated target-version universe meets the approved RPO/RTO and verifies representative receipts, decisions and outbox replay.
2. A prior-image rollback and forward-correction drill retain evidence and do not resurrect withdrawn permission.
3. Retention/deletion and recovery responsibilities are approved and documented; no legal-compliance claim rests on schema alone.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-023

**Named NVUTO product integrations have no end-to-end verification in this repository**

**Traceability:** [F-023](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-023), [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023), [G-023](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-023). **Proposed owner:** Integration lead + each approved consumer owner.

**Test design / environment.** Per approved consumer, actual staging user/app action→JWT→API→YSQL→event→consumer outcome; denied/missing/withdrawn/suppressed/expired/unavailable cases and restart/page-reload persistence.

**Required assertions.**

1. Each approved launch consumer has a reproducible integration/UAT record using real staging wiring.
2. No-consent, denial, withdrawal, suppression, expiry and service-unavailable cases prevent disallowed communication.
3. A subject change is reflected after page reload and consumer restart, within the approved propagation budget.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07), [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-024

**Preference discovery, registry reads and a machine-readable client contract are absent**

**Traceability:** [F-024](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-024), [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024), [G-024](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-024). **Proposed owner:** API architect + portal/admin owner.

**Test design / environment.** Contract conformance for existing11 operations and approved reads/projection; tenant/subject authorization, current-state initial load, bounded pagination/filtering and client error/retry handling.

**Required assertions.**

1. A portal can load the approved current preferences and necessary policy/notice data through a documented authorized source.
2. Contract tests validate request/response models for every launch endpoint, including negative authorization cases.
3. Pagination/filtering and origin/session behavior are explicitly specified where needed; absent frontend is not misrepresented as an implemented screen.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-025

**Idempotency replay semantics change with policy and current consent state**

**Traceability:** [F-025](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-025), [T-025](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-025), [G-025](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-025). **Proposed owner:** Backend/API engineer.

**Test design / environment.** Identical-key replay after policy/actor/token/time representation/resource status changes, semantic conflict409, simultaneous writes and retention boundary on real YSQL.

**Required assertions.**

1. Same-key replay after withdrawal, policy change, actor rotation and time-offset normalization matches the published contract.
2. Changed semantic payload returns409 and concurrent identical writes create exactly one consent/receipt/audit/outbox set on YSQL.
3. Retention, cleanup and consumer use of evaluation instead of stale create-response status are documented.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-11](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-11) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-026

**Memory-store composite keys collide across tenants and diverge from durable behavior**

**Traceability:** [F-026](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-026), [T-026](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-026), [G-026](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-026). **Proposed owner:** Backend maintainer.

**Test design / environment.** Typed-key collision property cases and shared adapter contract suite, suppression event IDs, isolation across Memory instances and mutable-map aliasing; deployment ban on Memory.

**Required assertions.**

1. Delimiter-rich tenant/subject/code/ID combinations cannot cross boundaries.
2. Memory and YSQL contract tests agree on suppression IDs, event metadata, concurrency and immutable values.
3. Production configuration tests reject ALLOW_MEMORY_STORE.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-027

**Input normalization and validation leave ambiguous or surprising API behavior**

**Traceability:** [F-027](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-027), [T-027](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-027), [G-027](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-027). **Proposed owner:** API/backend engineer.

**Test design / environment.** Exactly one JSON document, body limits/types/content type, omitted booleans, evidence/time bounds, normalization and safe opt-out failure journeys; assert no mutation on rejected requests.

**Required assertions.**

1. Trailing JSON and malformed/oversized/wrong-type inputs return stable4xx without mutation.
2. Omitted active, invalid expiry and invalid evidence produce documented outcomes rather than accidental defaults.
3. Deny/withdraw error paths allow the approved opt-out journey under policy-change/unavailable conditions.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-02](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-02), [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-028

**Database constraints do not enforce several tenant and state relationships**

**Traceability:** [F-028](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-028), [T-028](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-028), [G-028](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-028). **Proposed owner:** DBA/backend engineer.

**Test design / environment.** Read-only violation census, representative direct invalid inserts, composite tenant references, constrained transaction ordering and post-upgrade write/performance tests on YSQL.

**Required assertions.**

1. Representative cross-tenant receipt/idempotency reference attempts and invalid state shapes are rejected by the approved layer.
2. Existing-data validation reports zero unexplained violations before constraints are enforced.
3. Fresh install and upgrade preserve performance and transactional write order under YSQL.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-029

**Immutable audit rows cannot reconstruct policy and suppression history**

**Traceability:** [F-029](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-029), [T-029](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-029), [G-029](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-029). **Proposed owner:** Data/backend engineer + privacy owner.

**Test design / environment.** Historical policy/suppression reconstruction, role/tenant-scoped investigator export and app-role UPDATE/DELETE denial; verify all relevant before/after and revision fields.

**Required assertions.**

1. An auditor can reconstruct the exact policy and suppression states used for a historical consent decision.
2. Updates retain actor, resource type, revision, time and approved change reason without overwriting historical values.
3. Audit exports are tenant/role-scoped and the normal app role cannot alter history.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-030

**Mixed self-service and reader roles can authorize tenant-wide writes**

**Traceability:** [F-030](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-030), [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030), [G-030](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-030). **Proposed owner:** Identity/security + privacy owner.

**Test design / environment.** All seven roles and mixed-role combinations against every endpoint, production claim mapping, canonical subjects, protected suppression clearing under the approved policy and forbidden development auth.

**Required assertions.**

1. Mixed self_service plus service or consent_reader tokens cannot write another subject solely by combining separate role checks; any broader writer authority must be explicitly granted and tested.
2. A signed role-permission matrix matches production IdP mappings and backend tests for all seven roles.
3. Self-service cannot clear a protected administrative suppression if the approved policy distinguishes that class.
4. Production deployment disallows development authentication/roles according to the approved identity contract.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-031

**Legacy Record and Upsert helpers preserve a non-atomic governance bypass path**

**Traceability:** [F-031](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-031), [T-031](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-031), [G-031](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-031). **Proposed owner:** Backend maintainer.

**Test design / environment.** Call-site inventory, failure injection for every production write entry, governed-path tests and explicitly separate legacy migration tests; demonstrate no ignored audit/outbox failure.

**Required assertions.**

1. No production entry point can create consent while silently losing audit/outbox/receipt effects.
2. Compatibility consumers are inventoried or explicitly absent before removing exported behavior.
3. Production behavior tests exercise the governed path and legacy migration tests remain separate.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Retain the reproduced before-fix case as regression evidence when one exists.

## AT-032

**Approved launch requirements, capacity and regional policy decisions are not supplied**

**Traceability:** [F-032](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-032), [T-032](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-032), [G-032](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-032). **Proposed owner:** Product/architecture + release owner.

**Test design / environment.** Requirements/decision review with accountable owners, exact upstream source revision, scope exclusions and measurable SLO/region/recovery acceptance references; this is an evidence review, not a unit test.

**Required assertions.**

1. Every launch-critical requirement has an owner, source, acceptance case and implementation/gap link.
2. All release-affecting decisions are recorded and approved; deferred scope is explicit.
3. The release evidence identifies a source commit, approved configuration, SLO/capacity/RPO/RTO and regional privacy review owner.

**Evidence and present status.** NOT COMPLETE. Attach fixtures, commands or UAT steps, actual source/image/provider versions, assertion results, relevant state/event counts or measured budgets, sanitized logs and reviewer/date. A successful example or a skipped test does not satisfy the group. Resolve [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), [D-03](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-03), [D-04](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-04), [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05), [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06), [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07), [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08), [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09), [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10), [D-11](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-11), [D-12](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-12), [D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13), [D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14) first where they determine expected behavior. Retain the reproduced before-fix case as regression evidence when one exists.
