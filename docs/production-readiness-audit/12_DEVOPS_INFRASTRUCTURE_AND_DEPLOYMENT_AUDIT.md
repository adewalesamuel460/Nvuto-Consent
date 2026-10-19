# 12 — DevOps, infrastructure and deployment audit

**Audit date:** 2026-10-19. **Readiness disposition:** the source can be assessed and built in isolation, but a reproducible production release and production-equivalent operating evidence are not established. Confirmed code/configuration gaps must be fixed, and the target infrastructure owners must supply the missing integration, capacity, security and recovery evidence.

Source references are relative to the retained repository root. Kubernetes YAML describes **desired configuration**, not an observed deployment. No Kubernetes cluster, production database, broker account, real IdP, registry release or consumer application was accessed. The existing isolated test and race suites each yielded **15 PASS / 3 SKIP**; skipped external tests and build/scanner details are recorded in the [verification report](14_TESTING_QA_AND_VERIFICATION_REPORT.md).

The [risk register](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md) is authoritative for F-001 through F-032. This report explains operations and evidence needs without recreating the entire backlog. Use the [implementation backlog](16_DETAILED_IMPLEMENTATION_BACKLOG.md), [go-live checklist](19_PRODUCTION_GO_LIVE_CHECKLIST.md) and [decision register](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md) for T/AT/G references and approvals. External dependencies EXT-01 through EXT-12 are inventoried in [report 11](11_THIRD_PARTY_INTEGRATIONS_AUDIT.md).

## 1. What exists and what has been verified

| Area | Supplied implementation | Audit evidence and operational limit |
|---|---|---|
| API process | Go HTTP server on `:8080`, configured timeouts, signal shutdown, pgx store or explicitly enabled memory store. `cmd/server/main.go:413-455`. | Unit/HTTP-memory checks execute. Production-authenticated HTTP→YSQL workflow is not verified. |
| Outbox worker | Separate Go process; SQL claim/retry/completion and synchronous JetStream publication. `cmd/outbox-worker/main.go:46-234`. | Envelope test executes; external worker test skipped. Lease ownership defect is source-confirmed, not an observed production loss incident. |
| Database schema | Three numeric SQL migrations for consent, governance/evidence and idempotency. `migrations/001_init.sql:1-45`; `migrations/002_policy_governance.sql:1-110`; `migrations/003_governed_idempotency.sql:1-11`. | Source reviewed; no live audit migration/upgrade/restore. Exact pgx timestamp codec defect reproduced separately. |
| Container build | Two multi-stage Dockerfiles, static Linux binaries, nonroot distroless runtime. `Dockerfile:1-12`; `Dockerfile.worker:1-11`. | Go builds checked in isolated archive context. Docker/image execution was not available during the audit; no final image digest or scan supplied. |
| Kubernetes | API/worker Deployments, namespace, Service, PDBs and API HPA. `deploy/k8s.yaml:1-129`; `deploy/outbox-worker.yaml:1-83`. | Static definitions reviewed; no applied objects, scheduler placement, rollout, gateway or actual cluster evidence. |
| CI | Go checks and RF1 YSQL/single-server NATS functional job. `.github/workflows/ci.yml:1-122`. | Definition reviewed; audit ran available checks independently. No authenticated GitHub run for a source commit or required-check configuration supplied. |
| Recovery/operations | Short README production caveat and rollback note; process/error logs. `README.md:45-51`; `migrations/ROLLBACK.md:1-5`. | No approved SLO/RPO/RTO, load evidence, backup/restore drill, dashboards or on-call runbooks. |

The original plain `go build` attempt encountered VCS-status access in the parent workspace of the ZIP extraction. The successful archive-specific retry used `-buildvcs=false`; this is an environmental archive limitation, not a product compile defect. It also means the archive does not supply a release commit/provenance identity. Record an actual source commit and immutable artifacts before promotion (**F-018, F-032**; [report 14](14_TESTING_QA_AND_VERIFICATION_REPORT.md)).

## 2. Kubernetes desired-state inventory

### 2.1 API workload

| Setting | Supplied value | Implication / verification needed |
|---|---|---|
| Namespace | `nvuto-consent`, restricted Pod Security enforcement label. `deploy/k8s.yaml:1-6`. | Useful baseline; actual admission policy/enforcement not inspected. |
| Replicas and rollout | Three replicas; rolling update `maxUnavailable: 0`, `maxSurge: 1`. Lines 14-19. | Availability intent only. Requires spare node/resource capacity and correct readiness. |
| Placement | Hostname topology spread, max skew 1, `ScheduleAnyway`. Lines 32-38. | Soft distribution, not proof of multi-zone placement or failure-domain isolation. |
| Image | `ghcr.io/atlasfsp/nvuto-consent:latest`, no explicit pull policy/digest. Line 41. | Mutable artifact; exact deployed release unknown. F-018. |
| Network | Container 8080; Service port 80→8080 with default ClusterIP type. Lines 42-43,88-98. | No external gateway/Ingress, DNS or TLS boundary supplied. EXT-08/F-019. |
| Probes | `/readyz` every five seconds; `/healthz` every ten seconds. Lines 65-74. | Readiness currently only DB ping. No startup probe or schema/auth compatibility gate. F-015. |
| Resources | Request 100m CPU / 128Mi; limit 1 CPU / 512Mi. Lines 75-81. | Starting resource settings, not measured capacity or latency evidence. D-08. |
| Security | Nonroot, RuntimeDefault seccomp, no privilege escalation, read-only root FS, all capabilities dropped. Lines 28-31,82-86. | Positive controls to retain; runtime/image behavior still needs staging verification. |
| Disruption budget | `minAvailable: 2`. Lines 100-109. | A policy setting, not proof that a node/zone failure preserves application service. |
| Autoscaling | CPU HPA minimum 3, maximum 20, target 70% utilization. Lines 111-129. | No load-derived scaling threshold or database capacity budget supplied. |

### 2.2 Worker workload

| Setting | Supplied value | Implication / verification needed |
|---|---|---|
| Replicas and rollout | Two replicas; max unavailable 0, surge 1. `deploy/outbox-worker.yaml:7-12`. | Concurrency is intended, but unfenced SQL leases prevent claiming proven HA safety. F-009. |
| Placement | Preferred hostname anti-affinity. Lines 25-33. | Preference, not a mandatory distinct-node/zone guarantee. |
| Image | `ghcr.io/atlasfsp/nvuto-consent:latest-worker`. Line 36. | Mutable non-literal-latest tag; cached-node differences are possible. F-018. |
| Database credentials | Same `consent-secrets/database-url` reference as API. Lines 38-42. | No separate least-privilege worker role demonstrated. F-019. |
| Messaging | `nats://nats.nvuto-messaging.svc.cluster.local:4222`; stream `NVUTO_CONSENT`; prefix `nvuto.consent`; replicas `3`. Lines 43-50. | An external broker DNS name and requested stream settings, not a cluster definition or evidence of TLS/replication. |
| Credentials | Read-only mounted `consent-nats-credentials`; file `/var/run/secrets/nvuto/nats/consent.creds`. Lines 51-56,69-72. | Secret object/content/lifecycle not supplied. No custom CA setting/mount is wired here. |
| Resources and security | Request 50m CPU / 64Mi; limit 500m CPU / 256Mi; nonroot/seccomp/read-only/capability controls. Lines 21-24,57-68. | Baseline only; no delivery-throughput measurement. |
| Disruption budget / health | PDB `minAvailable: 1`; no worker readiness/liveness/startup probes, no worker HPA, no explicit termination-grace value. Lines 1-83. | A running process can make no delivery progress without an orchestration-level signal. F-015/F-021. |

Neither manifest defines the referenced Secrets/ConfigMap, YSQL universe, NATS cluster/account, registry publication, ingress/gateway, network policies, workload service-account separation, monitoring stack or backup jobs. Those can be externally owned, but the release needs explicit provisioning inputs and owners (**EXT-03, EXT-04, EXT-06, EXT-08 through EXT-11**).

Kubernetes gives a literal `:latest` image the default `Always` pull policy, while a different tag such as `:latest-worker` defaults to `IfNotPresent` when omitted. Both remain mutable; the worker additionally risks retaining different cached tag contents across nodes. Use approved immutable image digests for both processes and an owned promotion flow (**F-018 / AT-018**). Primary source checked 2026-10-19: [Kubernetes image names and pull policy](https://kubernetes.io/docs/concepts/containers/images/).

## 3. Complete environment-variable and configuration inventory

There are **21 named variables directly referenced in application, test, shell or CI configuration**: 20 are read by Go or the shell helper, and `UPGRADE_DATABASE_URL` is a CI-only declaration that is never read. This count does not imply 21 active production settings. Standard inherited library/tool variables are discussed separately below.

| # | Variable | Reader; requirement/default | Supplied deployment / operational treatment |
|---|---|---|---|
| 1 | `DATABASE_URL` | API `cmd/server/main.go:417-427`; worker `cmd/outbox-worker/main.go:150-153`; store integration test `internal/store/idempotency_integration_test.go:14-18`. Required for worker; API requires it unless memory explicitly enabled. | API/worker share Secret `consent-secrets/database-url` (`deploy/k8s.yaml:45-49`; worker manifest 38-42). CI sets localhost YSQL DSN with `sslmode=disable` (CI:36). Treat DSN as secret-capable, require production TLS/roles; do not use production DB for integration tests. |
| 2 | `ALLOW_MEMORY_STORE` | API `cmd/server/main.go:426-430`; only case-insensitive `true` opts into non-durable storage when DB URL is absent. Default disabled. | Not set in production manifest; CI sets `false` (CI:40). Enforce disabled in production deployment policy; memory mode loses state on restart and has F-026 parity defects. |
| 3 | `JWT_JWKS_URL` | Auth `internal/auth/auth.go:139-164`; nonempty selects JWKS branch ahead of other modes. | ConfigMap `consent-runtime/authentik-jwks-url` (API manifest:50-54). Value and actual HTTPS endpoint not supplied; startup validation/rotation needed. |
| 4 | `JWT_ISSUER` | Auth:142-145,153; required nonempty in JWKS mode. | ConfigMap key `authentik-issuer` (API manifest:55-59). Must match approved provider issuer exactly. |
| 5 | `JWT_AUDIENCE` | Auth:143-145,153; required nonempty in JWKS mode. | ConfigMap key `authentik-audience` (API manifest:60-64). Define allowed token consumers with identity owner. |
| 6 | `JWT_HMAC_SECRET` | Auth:140,166-177; nonempty secret plus opt-in required for HMAC branch. | Not in production manifest. Secret-capable local/test alternative; does not add issuer/audience validation to that branch. |
| 7 | `ALLOW_HMAC_AUTH` | Auth:166; case-insensitive `true`, default disabled. | Not in production manifest. Production disposition must follow D-10; do not accidentally enable through shared env injection. |
| 8 | `ALLOW_DEV_HEADERS` | Auth:180-186; case-insensitive `true`, default disabled. | Not in production manifest; CI sets `true` (CI:41). Grants developer identity from supplied headers and must be rejected by approved production policy. |
| 9 | `NATS_URL` | Worker:151-153; required, no application default. | Worker manifest:43-44 supplies internal `nats://` URL. Actual endpoint/account/TLS topology not verified. URL may carry sensitive credentials depending on configuration. |
| 10 | `NATS_STREAM` | Worker:155-158; default `NVUTO_CONSENT`. | Worker manifest:45-46 sets same. Existing stream config is not validated; F-020. |
| 11 | `NATS_SUBJECT_PREFIX` | Worker:159-162; default `nvuto.consent`; trailing dot trimmed by subject builder. | Worker manifest:47-48 sets same. No tenant component in default subjects; ACL/consumer contract external. |
| 12 | `NATS_REPLICAS` | Worker:163-169; default 3; parse must be integer 1 through 5. | Worker manifest:49-50 sets `3`. Used when creating a stream; does not validate/change existing stream replicas. |
| 13 | `NATS_CREDS_FILE` | Worker:186-190; required unless insecure opt-in is true. | Manifest:51-56 points to mounted `consent.creds`. Define file key/mount/access and credential rotation; filename is not proof of valid credentials. |
| 14 | `ALLOW_INSECURE_NATS` | Worker:188-190; case-insensitive `true` permits missing credentials file; default disabled. | Not set in production worker manifest. The name concerns the credentials requirement; it is not an explicit TLS-off/TLS-on switch. |
| 15 | `NATS_TLS_CA_FILE` | Worker:191-192; optional custom root CA file passed to `nats.RootCAs`. | Not wired in supplied worker manifest. In pinned client, RootCAs enables secure mode; TLS may also be required by server. Define explicit transport/certificate policy. |
| 16 | `TEST_DATABASE_URL` | Server external test `cmd/server/postgres_integration_test.go:17-21`; worker external test `cmd/outbox-worker/postgres_integration_test.go:15-20`; skip when absent. | CI:37 supplies isolated consent DB. The store idempotency test uses a different variable, `DATABASE_URL`; unify required integration-test contract under F-017. |
| 17 | `TEST_NATS_URL` | Worker external test:17-20; external test skips without DB or NATS URL. | CI:38 supplies localhost broker. Test connects directly without production credentials/TLS options; not production security evidence. |
| 18 | `YUGABYTEDB_CONTAINER` | Shell helper `scripts/ci/start-yugabytedb.sh:5`; default `nvuto-consent-yugabyte`; also referenced by CI commands. | CI:35 sets same. Script force-removes the named container before creation; use only a deliberately isolated CI runner, never a production/reused service container. |
| 19 | `YUGABYTEDB_IMAGE` | Shell helper:6; default `yugabytedb/yugabyte:2026.1.0.1-b1`. | Not overridden in current CI. A test image pin, not a declaration of actual production database version. |
| 20 | `YUGABYTEDB_STARTUP_TIMEOUT_SECONDS` | Shell helper:7,14,22-30; default 300. | No override in current CI. Bounds cluster-start/readiness polling in fixture setup, not runtime SQL latency. |
| 21 | `UPGRADE_DATABASE_URL` | Declared `.github/workflows/ci.yml:39`; **unused** by source/script/workflow commands. | Upgrade commands directly select `consent_upgrade` (CI:58-62). Remove ambiguity or wire a consistent contract; declaration does not execute upgrade tests. |

Authentication settings are read during each request rather than validated once at startup. Other runtime worker settings are read at process startup. Kubernetes environment injection and mounted-secret rotation behavior must be documented; no hot-reload guarantee is implemented. Sources: `internal/auth/auth.go:138-188`; `cmd/outbox-worker/main.go:149-203`.

Library defaults may also respond to inherited environment: Go's default HTTP transport honors proxy variables such as `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY`; pgx can interpret supported connection/service/password environment; build tools have their own configuration. These are **not additional application-defined variables** and are not included in the 21 count. Capture an allowlisted, sanitized effective configuration for release evidence. In particular, the JWKS client uses the default transport (`internal/auth/auth.go:46-47`; audited Go toolchain `src/net/http/transport.go:40-55,76-78`).

## 4. Fixed runtime budgets and capacity assumptions

| Control | Current value / behavior | Operational consequence |
|---|---|---|
| API address and HTTP limits | `:8080`; read-header 5s, read 15s, write 15s, idle 60s; shutdown deadline 15s. `cmd/server/main.go:435,442-455`. | Useful network/process budgets, but they do not cancel `context.Background` repository work. F-012. |
| JSON input and batch size | One-MiB body; unknown fields disallowed; batch evaluation 1-100 items. `cmd/server/main.go:78-85`; `internal/service/service.go:237-245`. | Per-request bounds exist. Exactly-one-JSON-document and semantic input limits need F-027. |
| API DB pool | Maximum 25/minimum 2 connections per process; max lifetime 45m, idle 5m. `internal/store/postgres.go:22-25`. | Overrides these parsed pool fields even if a DSN tries to tune them. At 20 API replicas the configured upper bound is 500 API connections, plus worker/migration/operations connections and rollout surge. This is arithmetic, not a measured safe YSQL budget. |
| Worker DB pool | `pgxpool.New` using DSN/client defaults, separate from `NewPostgres`. `cmd/outbox-worker/main.go:177`. | Do not assume worker inherits API's 25/2 configuration. Include its actual pool settings in capacity evidence. |
| Readiness DB check | Ping with two-second context. `internal/store/postgres.go:38-41`. | Bounds a ping, not schema/auth/business readiness. |
| JWKS fetch/cache | Five-second HTTP client timeout, one-MiB reader, five-minute positive cache. `internal/auth/auth.go:41-117`. | Miss traffic is not bounded by cache age; F-014 can amplify IdP load. |
| Outbox poll and batch | One-second ticker; up to 50 events, all tenants, serial publication. `cmd/outbox-worker/main.go:206,215-231`. | Throughput depends on sync acknowledgment/DB latency, not an assumed 50 events/sec. No per-tenant runtime partition or fairness budget is configured. |
| Claim lease | Two minutes for every row in batch, hostname-based owner marker. Worker:54,71-74,171-174. | No renewal/claim token; later rows can outlive lease before processing. F-009. |
| NATS client | Connect timeout 10s; unlimited reconnect attempts; sync publication. Worker:185,199,221. | `PublishAsyncMaxPending(256)` is configured but publication uses synchronous `PublishMsg`, so it does not create 256 concurrent publishing slots. |
| Publish/retry horizon | Pinned client default JetStream request wait 5s; retry exponential, capped 900s; new-stream dedup window 10m. Worker:127-145; pinned `nats.go@v1.47.0/js.go:298-315,534-553`. | 50 serial five-second waits can approach 250s before DB overhead, exceeding the 120s lease. The 15m retry cap can exceed the 10m broker duplicate window. |

D-08 must approve request volume, tenant/customer/event cardinality, latency/availability SLO, propagation budget, maximum tolerated backlog, RPO/RTO and target failure topology. No load test, size estimate or benchmark in the supplied evidence validates the current pool, pod resource, HPA or batch settings. Test total connection and memory budgets at maximum replicas plus surge, then tune based on measured demand (**F-012, F-016, F-021, F-032**).

YSQL transaction retry behavior depends on transaction/isolation/error conditions and client behavior. The repository uses default transaction options and does not define a bounded retry policy for serialization/conflict failures. This is not a claim that every transaction needs an application retry or that YSQL has failed. Define whole-transaction retry semantics for the approved target configuration, with idempotent receipt/audit/outbox effects and cancellation-aware limits (**F-012 / AT-012**). Primary sources checked 2026-10-19: [YSQL transaction retries](https://docs.yugabyte.com/stable/develop/learn/transactions/transactions-retries-ysql/) and [YSQL read-committed behavior](https://docs.yugabyte.com/stable/architecture/transactions/read-committed/).

## 5. Outbox correctness, failure recovery and health

### 5.1 Useful foundations

Claiming occurs inside a database transaction using `FOR UPDATE SKIP LOCKED`; rows are marked leased and attempts incremented before commit. The worker then publishes outside the claim transaction, uses stable event IDs for broker deduplication, and marks a row published only after a synchronous publish returns success. On publication failure it keeps the event durable and schedules exponential retry. Sources: `cmd/outbox-worker/main.go:46-79,102-134`.

These mechanics avoid holding the claim transaction open across broker I/O and support recovery. They do not establish exclusive ownership after a lease expires, end-to-end ordering, or exactly-once processing. Broker acknowledgment is not consumer acknowledgment; consumer deduplication/reconciliation remains **F-010 / EXT-12**. Primary references checked 2026-10-19: [NATS publishing](https://docs.nats.io/learn/jetstream/publishing) and [delivery and acknowledgment](https://docs.nats.io/learn/jetstream/delivery-and-acknowledgment).

### 5.2 Confirmed lease and shutdown gap — F-009

`complete` updates by event ID and `published_at IS NULL`; it does not check `locked_by` or a claim generation. Its “lost ownership” error checks affected-row count, not actual ownership. `retry` likewise clears and reschedules a currently leased row by ID without an ownership condition. A stale worker A can therefore mutate a row newly claimed by B after the two-minute lease expires. The entire 50-event batch is leased together and published serially without renewal. Source: `cmd/outbox-worker/main.go:54,71-74,116-134,215-231`.

The reproducible isolated scenario is: A claims an event; its lease expires; B reclaims; A calls completion or retry. Current SQL can complete or clear/reschedule B's claim. This is a confirmed control-flow/SQL defect; the audit did not execute a real broker/DB failover or demonstrate permanent event loss. Likely failure modes include duplicate publishers, confused retry scheduling and longer propagation delays. A successful old publication followed by a completion does not by itself mean that an event disappeared.

On signal, the outer context is canceled, but the inner batch does not check cancellation before each publish and `PublishMsg` receives no request context. Later SQL completion/retry calls use the canceled context, while more broker calls can still occur. The process defers a connection `Close`, not an explicit drain, and the manifest supplies no explicit termination budget. Source: `cmd/outbox-worker/main.go:175-198,209-231`; `deploy/outbox-worker.yaml:1-83`.

**T-009 / AT-009** must add an unguessable claim token/generation checked by complete/retry/renew, bounded publish/batch deadlines or safe lease renewal, and stop-claiming/drain/recovery semantics. Tests must cover stale mutation rejection, slow publication, process kill, publish-acknowledgment/SQL-completion gaps and shutdown within the approved D-08 budget. Hostname alone is insufficient across retries and restarted processes.

### 5.3 Stream and failure-state control — F-020/F-021

`ensureStream` returns success for any existing stream name without checking subjects, storage type, replica count, dedup window or retention. New streams request file storage, LimitsPolicy, replicas and ten-minute deduplication but no explicit byte/message/age limits. Account/server defaults may constrain the stream; their actual values are unknown. Source: `cmd/outbox-worker/main.go:137-169`.

Provision streams through an approved messaging role or validate them without silently modifying shared resources at worker startup. Define storage/capacity/discard policy and limits based on D-08, and verify drift and publish-rejection behavior under **AT-020**. Do not confuse `NATS_REPLICAS=3` with evidence that an existing stream actually has three replicas. Primary reference checked 2026-10-19: [NATS stream configuration and limits](https://docs.nats.io/learn/jetstream/shaping-the-stream).

Retries are unbounded in count and there is no quarantine/review/replay workflow for an event that always fails. Claims can loop on DB failure; reconnect is unlimited; logs alone do not provide a delivery SLO. Under **AT-021**, operators need backlog/oldest-age/attempt-distribution signals, a safe poison-event investigation path and authorized idempotent replay. A manual “mark published” workaround must not substitute for verified delivery.

## 6. Startup, readiness and observability

The API validates its storage connection at startup but checks authentication configuration only during requests. `/readyz` invokes `Service.Ready`, which pings the DB; it does not verify migrations, table usability or issuer/audience configuration. A reachable empty database or missing auth can therefore coexist with a ready response. `/healthz` always reports success. Both handlers accept every method, including CONNECT; this matters to a conditional standard-library advisory explained in [report 10](10_SECURITY_PRIVACY_AND_ACCESS_CONTROL_AUDIT.md). Source: `cmd/server/main.go:389-398,413-439`; `internal/service/service.go:390`; `internal/store/postgres.go:38-41`.

**F-015 / AT-015** requires startup validation for production auth/store/config, a compatible recorded-schema gate and explicit probe methods. Define readiness around ability to serve the approved workload, without making every probe issue an IdP fetch. Keep liveness distinct from transient dependency availability to avoid a restart loop during a recoverable outage. A correctly configured authenticated smoke request must precede promotion.

The worker has no metrics endpoint or progress probe. Neither binary implements a structured request correlation/trace layer, latency/error/decision metric set, database-pool telemetry, JWKS refresh metrics, outbox-age metrics, dashboards or alert routing. Logs report process and error text, and public unknown errors can include raw internals. Source: `cmd/server/main.go:59-76,424-455`; `cmd/outbox-worker/main.go:208-230`.

**Proposed minimum operational signals, subject to D-08/D-09 approval:**

| Signal | Purpose | Privacy and action requirement |
|---|---|---|
| Request count/latency/status and stable decision reason | Distinguish normal denials from application/unavailable errors and SLO degradation. | Bounded labels; no raw subject, bearer token, evidence or confirmation secret in metrics. |
| DB pool usage/wait, transaction duration/retry/timeout | Detect connection exhaustion, contention and ineffective cancellation. | Correlate sanitized errors; avoid raw SQL parameters. |
| JWKS refresh/miss/failure/age | Detect amplification, rotation failure or IdP outage. | Alert on configured budgets; never log token contents. |
| Outbox pending count/oldest age/attempts/claim and publish outcomes | Separate API health from eventual propagation health. | Expose bounded tenant classification only if approved; avoid unbounded per-subject labels. |
| Consumer lag/reconciliation and pre-send denial outcomes | Demonstrate downstream enforcement and propagation SLO. | Owned by each launch consumer; worker publish success cannot substitute. |
| Stream config/capacity, backup freshness and restore results | Detect drift and recovery failures before a customer incident. | Alerts need named rotation/on-call ownership, escalation and runbook links. |

No alert threshold in this table is asserted as an approved target. **AT-021** calls for staged IdP/DB/broker/poison-event failures that trigger the expected signal and recovery action, with evidence of operator interpretation and privacy-safe logs.

## 7. CI audit: implemented checks and exact omissions

The workflow runs on pushes to main and pull requests with `contents: read` permissions. Both jobs use `ubuntu-latest`, `actions/checkout@v4` and `actions/setup-go@v5`; Go is pinned to 1.25.13 and setup cache is disabled. These are source facts, not evidence of branch protection or a passing run (`.github/workflows/ci.yml:3-19,32-48`).

| Workflow stage | What the definition does | Coverage limit / required correction |
|---|---|---|
| Module/format/vet | Download modules, require empty gofmt output, run vet. CI:20-23. | Useful baseline; module verification was separately executed in audit. Does not prove deployed supply-chain identity. |
| Vulnerability gate | `go run ...govulncheck@v1.7.0 ./...` in ordinary text mode. CI:24-25. | Current pinned graph fails recorded scan with ten symbol advisories. JSON process exit 0 is not a pass. F-011. |
| General tests | `go test -race ./...`. CI:26. | This job has no DB/NATS env; all external integration tests skip. |
| Binary/image builds | Build API/worker and two local `:test` images. CI:27-30. | No publish/sign/SBOM/provenance/promotion/deployment or immutable release pinning. F-018. |
| YSQL fixture | Shell helper starts one RF1 cluster container; CI uses passwordless local YSQL with TLS disabled. CI:34-50; script:9-37. | Appropriate isolated functional fixture intent; not production TLS/auth/HA proof. Do not apply fixture credentials/topology to production. |
| Fresh install | Numeric migration glob applies every current migration. CI:51-55. | Includes 003 on fresh install; this is distinct from the incomplete upgrade step. |
| Upgrade | Creates `consent_upgrade`, applies 001, inserts legacy row, applies only 002, checks `granted:` with blank notice. CI:56-62. | Does not apply/verify 003 or post-upgrade idempotency. It intentionally retains the blank-notice legacy case of F-008. `UPGRADE_DATABASE_URL` is unused. F-017. |
| Server database test | Runs only `TestPostgresPolicyTransactionsAndSingleUseConfirmation`. CI:63-64. | Test calls Service directly, not authenticated HTTP. It does not establish actual IdP or complete durable route authorization. |
| Worker database/broker test | Starts one `nats:2.10-alpine -js`; runs only `TestPostgresJetStreamOutboxConcurrency`. CI:65-72. | Test uses a one-replica stream and checks concurrent claims, then serial publishes. Name “HA-safe” overstates demonstrated failure/replication coverage. |
| Durable idempotency test | No selected execution in YSQL job. | `internal/store/idempotency_integration_test.go` is never run with a DB by defined jobs. Its `DATABASE_URL` gate differs from the other tests, and globally fixed fixture IDs collide on reuse. F-017. |
| HTTP functional smoke | Starts API, readiness-polls, uses development headers for registry/grant/evaluate/suppression/batch/DOI/reuse/withdraw/receipt/outbox assertions. CI:73-115. | Useful development-identity HTTP coverage if executed. Receipt check asserts nonempty hash, not re-verification; no self-service authorization or independent DOI delivery proof. |
| Topology check | `yb_servers()` count at least one. CI:116-119. | Confirms a YSQL topology endpoint for RF1 fixture, not node loss, multi-zone safety or quorum behavior. |
| Failure artifact | Cat API log on failure. CI:120-122. | No durable machine-readable test/coverage/scan/manifest evidence publication, dashboard or consumer UAT. |

The worker integration test creates 30 events, runs two concurrent claimers with limits of 20, verifies unique combined claims, then publishes the combined list serially and fetches/acknowledges the messages. It does not exercise concurrent publication after lease expiry, slow-ack fencing, broker node loss, stream misconfiguration, replay beyond deduplication or production credentials/TLS. Source: `cmd/outbox-worker/postgres_integration_test.go:15-120`.

The store idempotency test creates a unique tenant but uses global IDs such as `con-0`, `rcp-0`, `aud-0` and `evt-0`. Since those table IDs are global primary keys, unique tenant alone does not isolate reruns. It has no corresponding cleanup. **AT-017** requires run-unique IDs, a consistent isolated DSN contract, actual execution of every external package, fresh/001-to-latest migration coverage including 003, and a second successful run in the designated reusable test environment. Sources: `internal/store/idempotency_integration_test.go:14-46`; `migrations/001_init.sql:1-45`; `migrations/002_policy_governance.sql:57-73`.

The audit executed module download/verification, format, shell syntax, vet, unit/race checks, builds and the pinned scanner as documented in report 14. It did **not** run the Docker-backed workflow, real YSQL/NATS tests or a GitHub-hosted pipeline. The three skips are **F-016 evidence gaps**, not failing database assertions. Adding the critical regression probes to appropriate durable/HTTP tests remains required; unchanged passing unit counts cannot close those findings.

## 8. Toolchain and release artifact policy

Both Dockerfiles use Go 1.25.13 Alpine builders and copy only the resulting static Linux binary into `gcr.io/distroless/static-debian12:nonroot`. Build flags disable CGO, set GOOS Linux, trim paths and strip symbol/debug size. Migrations and operator tooling are not copied into the runtime image. The release process therefore needs a separately identified migration artifact or checkout at the exact release revision, not an assumption that the running container can apply schema. Sources: `Dockerfile:1-12`; `Dockerfile.worker:1-11`.

The current Go pin exists but is out of support as of this audit under Go's two-current-major policy. Official releases on 2026-10-08 include Go 1.26.9 and 1.27.2. The exact scan found **19 advisories, ten symbol-level and nine package/module-only**; not all have exploitable runtime conditions in this application. The security report provides the full contextual table. Patch the supported toolchain consistently in all four source/configuration locations, update `x/text` to the indicated fixed version v0.41.0 or a compatible later version, triage the transitive graph and scan final runtime images (**F-011 / AT-011**). Primary source checked 2026-10-19: [Go release history/support policy](https://go.dev/doc/devel/release).

Required release evidence under **F-018** is an actual source commit, exact API/worker digests, builder/runtime identities, module/SBOM inventory, scan database/version/result, test artifacts, environment configuration references, migration version/checksums and a known prior digest. GitHub action major tags, moving runner image and mutable container tags should be evaluated under the organization's approved supply-chain policy. No malicious artifact or compromise was observed; provenance is missing evidence.

## 9. Migration, rollout and recovery procedure to make concrete

This is a **proposed controlled release sequence**, not an executed deployment or authorization to mutate live infrastructure. D-08/D-09 and the go-live gates must set owners and budgets before it becomes an operational runbook.

1. **Approve release inputs.** Record launch scope/consumers, identity claims/roles, state/temporal/policy/evidence decisions, SLO/capacity/RPO/RTO, topology, TLS/roles and external owners. Reject missing configuration before rollout. Resolve P0/P1 gates and document any permitted exception in the central register.
2. **Build one identifiable release.** Produce API/worker binaries and images from the same approved patched source/toolchain, scan them, preserve test/provenance evidence and promote exact digests. Publish the numeric migration set from that same source identity.
3. **Rehearse on isolated target-version infrastructure.** Run all external tests, authenticated HTTP→YSQL flows, current regression cases, fresh install and 001→latest upgrade, contention/cancellation, worker/broker failure and certificate/permission checks. Record actual target configuration and results under AT-016/AT-017/AT-019.
4. **Classify and protect existing evidence.** If any legacy records exist, obtain read-only counts and approved eligibility decisions under F-008; do not assume blank notice grants are safe. Verify a recoverable target-YSQL backup and recovery prerequisites before schema changes. No actual existing production record count was available in this audit.
5. **Apply additive schema through a restricted migration role.** Define migration locking/version tracking, compatibility window and failure handling; validate required schema before API readiness. Test any new constraints against existing-data checks and target YSQL insertion/transaction semantics. The current application has no startup migration runner.
6. **Roll out and verify the exact processes.** Use capacity-aware rolling updates, dependency-complete readiness and authenticated smoke checks. Confirm actual image digests, schema version and broker stream configuration. Observe errors, DB pool/latency, pending outbox age and consumer propagation while controlled journeys execute.
7. **Promote only on business outcomes.** Verify receipt hashes after storage, correct denial/withdrawal/DOI behavior, consumer restart/replay and disallowed-communication prevention. A healthy pod or empty outbox alone does not prove that a launch consumer enforced consent.
8. **Retain rollback and forward-correction evidence.** Keep the known prior image and an approved compatibility matrix. Roll back application images only when demonstrated compatible with retained schema/evidence. Use an approved forward correction for schema defects; do not destroy evidence to make an older binary start.

`migrations/ROLLBACK.md:3` asserts that the previous API can read/write legacy columns after migration 002. No previous image, source history or compatibility test was supplied, so that claim remains unverified. The note also says to take a “PostgreSQL backup” (`ROLLBACK.md:5`), whereas README specifies production YugabyteDB YSQL. Replace that ambiguity with the exact target-YSQL backup/PITR/restore method, ownership and tested versions; PostgreSQL protocol compatibility is not a demonstrated recovery procedure (**F-022**).

Recovery must reconcile the complete state boundary: consents, receipts, immutable audit history, policies/notices, suppressions, idempotency ledger and outbox, plus broker/consumer events already delivered after the restored point. A restored database can legitimately need replay, but must not resurrect a withdrawn permission or allow an old challenge to grant again. Test receipt verification, event duplication, consumer projection rebuild and the approved restore cutover behavior. Retention/deletion must be privacy-approved and preserve evidence; mutation-blocking triggers are not a backup or legal retention policy (`migrations/002_policy_governance.sql:91-110`; **D-05, D-08, D-14 / AT-022**).

## 10. Operations completion and go-live evidence

| Outcome needed before production approval | Canonical acceptance / gate | Required evidence and proposed owner |
|---|---|---|
| Fenced, bounded and recoverable outbox publication | AT-009 / G-009; AT-010 / G-010 | Slow/expired lease, stale worker, kill, lost acknowledgment/completion, shutdown and consumer reordering/replay tests. Backend/messaging owners. |
| Patched reproducible release | AT-011 / G-011; AT-018 / G-018 | Supported common toolchain, final image digests/scans/provenance, staging deployment/rollback of those digests. Release/security owners. |
| Bounded backend work and truthful health | AT-012 / G-012; AT-013 / G-013; AT-015 / G-015 | Cancellation/timeouts, typed sanitized errors, auth/schema startup gate and meaningful API/worker probes. Backend/SRE owners. |
| Durable and upgrade coverage actually runs | AT-016 / G-016; AT-017 / G-017 | No skipped required external tests, HTTP/YSQL atomicity and repeatable latest migration/idempotency tests, separate multi-node failure evidence. QA/database owners. |
| Verified infrastructure trust and broker configuration | AT-019 / G-019; AT-020 / G-020 | TLS failure/rotation tests, least-privilege roles and tenant ACLs, actual stream config/limits/capacity tests. Platform/security/messaging owners. |
| Observable service and recoverable evidence | AT-021 / G-021; AT-022 / G-022 | Dashboards/alerts/on-call/runbooks, poison replay drill, approved RPO/RTO/retention and isolated target-YSQL restore. SRE/database/privacy owners. |
| Approved capacity and operating scope | AT-023 / G-023; AT-032 / G-032 | Launch consumer outcomes, measured capacity/failure topology, signed scope/decision ownership and source identity. Product/architecture/release owners. |

The proposed role owners above remain unassigned until accepted. D-08 (SLO/RPO/RTO/capacity), D-09 (infrastructure/operations), D-10 (identity claims) and D-13 (events), together with the state/evidence/privacy decisions, are prerequisites to choosing defensible operating limits. Track closure in [report 19](19_PRODUCTION_GO_LIVE_CHECKLIST.md) and [report 20](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md), using executed artifacts from [report 14](14_TESTING_QA_AND_VERIFICATION_REPORT.md) and the canonical severity/work in [reports 15](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md) and [16](16_DETAILED_IMPLEMENTATION_BACKLOG.md).
