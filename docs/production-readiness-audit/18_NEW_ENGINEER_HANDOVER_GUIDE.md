# 18 — New engineer handover guide

**Audit date:** 19 October 2026. **Scope:** the supplied NVUTO Consent source archive and the retained audit evidence. **Current disposition:** suitable for isolated investigation and remediation; production readiness has not been established.

This guide covers the 20 onboarding topics below. Source references are relative to the repository root. **Observed audit results** are identified explicitly. All setup, database, migration, broker, runtime and container commands offered for a future engineer are **proposed instructions that were not executed during this audit**. The audit did not start Docker, apply SQL, contact a real issuer or consumer, publish to an external broker, or deploy Kubernetes resources. See [report 21](21_AUDIT_EVIDENCE_AND_COMMAND_LOG.md) for exact command arguments, environment isolation and retained logs.

## 1. Understand the product boundary

NVUTO Consent is a shared backend service for consent records, purpose/channel preferences, lawful bases, notice references, evidence, withdrawal, suppression, decisions and change events. Customer identity, campaign execution, conversations, legal contracts and analytics ownership belong elsewhere. The README is the supplied requirements source; no approved PRD, UI specification, launch region list or consumer implementation accompanies it. `README.md:3-11`.

The core invariant is that an outbound workflow must not turn missing required affirmative consent into permission. A useful first review follows one governed write through policy validation, durable evidence, evaluation and eventual event delivery. Several current implementations violate or incompletely establish that invariant: old DOI confirmation can override a later denial, current policy/time semantics are incomplete, and downstream enforcement is unverified. Start with the [executive summary](00_EXECUTIVE_AUDIT_SUMMARY.md), [requirements traceability](02_DOCUMENTATION_REQUIREMENTS_TRACEABILITY.md) and [risk register](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md). Resolve product scope through [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01), rather than treating absent application screens as automatically missing service features.

## 2. Learn the repository organization and reading order

There are **two deployable Go processes**, **eight engineering modules**, **eleven business API operations** and **two operational paths**. The supplied source contains **zero frontend applications, screens, UI routes or consumer clients**. The full original-file inventory is in [report 01](01_REPOSITORY_ARCHITECTURE_MAP.md).

| Order / module | Read first | Responsibility and useful next file |
|---|---|---|
| MOD-01 API | `cmd/server/main.go:23-118,386-455` | Request models, error handling, role gates, routing and startup; then all three server test files. |
| MOD-02 Authentication | `internal/auth/auth.go:16-200` | Claims, JWKS, explicit development modes and exact role matching; then `internal/auth/auth_test.go`. |
| MOD-03 Domain | `internal/domain/model.go:5-151` | Public models, statuses, decision and evidence fields. |
| MOD-04 Service | `internal/service/service.go:17-390` | Registry rules, governed state transitions, evaluation and DOI; then `internal/service/idempotency.go:14-82` and service tests. |
| MOD-05 Storage | `internal/store/memory.go:16-340` and `internal/store/postgres.go:18-396` | Repository contract, memory behavior and YSQL-backed operations; then `internal/store/idempotency.go:14-99`. Memory success does not establish YSQL parity. |
| MOD-06 Worker | `cmd/outbox-worker/main.go:19-234` | Event envelope, SQL leases, publication, retry and process configuration; then its two tests. |
| MOD-07 Migrations | `migrations/001_init.sql`, `002_policy_governance.sql`, `003_governed_idempotency.sql` | Read in numeric order; then `migrations/ROLLBACK.md:1-5` and [report 08](08_DATABASE_AND_DATA_SCHEMA_AUDIT.md). |
| MOD-08 Build/deploy/CI | `go.mod`, `Dockerfile`, `Dockerfile.worker`, `deploy/`, `.github/workflows/ci.yml`, `scripts/ci/start-yugabytedb.sh` | Dependencies, two container images, desired workloads and disposable test infrastructure. |

The package and log names use “Postgres” because the driver speaks that protocol. **YugabyteDB YSQL is the intended production database**, expressly stated in `README.md:45-51`; do not silently replace the target with PostgreSQL when assessing durable or HA behavior.

## 3. Install the right prerequisites and dependencies

For exact audit reproduction, obtain the official **Go 1.25.13** toolchain and select it explicitly. The version is pinned in `go.mod:3`, `.github/workflows/ci.yml:18,46` and both Dockerfiles. The audit downloaded that toolchain into an isolated directory; it did not change the system Go installation. This version is a **reproduction dependency**, not the approved production recommendation: [T-011](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-011) requires a supported patched Go release and dependency remediation, followed by compatibility and vulnerability checks.

The module has three direct dependencies: JWT v5.2.2, pgx v5.9.2 and nats.go v1.47.0; exact direct and indirect versions are in `go.mod:5-22` with checksums in `go.sum`. No Node/npm installation is required. Go tests with `-race` also need a supported race-detector platform and the applicable native C compiler/toolchain. Future container fixtures require Bash, a working Docker daemon, image-pull access, available disk/memory, and unused host ports 5433 and 4222. Starting the API additionally requires port 8080. No measured minimum laptop or production capacity is supplied; [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08) owns capacity targets.

**Proposed portable setup, from the repository root in a fresh Bash shell:**

```bash
set -euo pipefail
test -f go.mod
test -f cmd/server/main.go
export NVUTO_REPO_ROOT="$PWD"
# Set this path to the official Go 1.25.13 binary you installed for reproduction.
: "${NVUTO_GO:?Set NVUTO_GO to the absolute path of the reproduction go binary}"
export GOTOOLCHAIN=local
export GOFLAGS=-mod=readonly
"$NVUTO_GO" version
"$NVUTO_GO" mod download
"$NVUTO_GO" mod verify
```

Do not run `go mod tidy` merely to make setup appear successful: preserve the supplied dependency graph while reproducing findings. Dependency changes belong in T-011 with a reviewed diff and renewed evidence. Module download and verify succeeded in the audit; this portable shell block itself is a future setup instruction.

## 4. Configure an isolated development environment

The programs read environment variables directly; the source provides no dotenv loader or environment-generation wizard. Use a fresh local shell with explicitly selected inputs. Keep production DSNs, bearer tokens, HMAC secrets and NATS credential contents out of terminal transcripts, committed files and this handover. The commands below require any real credentials or issuer configuration to be supplied securely by their owner; they contain no usable production secret.

Choose one authentication mode deliberately. Production-compatible development uses RS256/JWKS with `JWT_JWKS_URL`, `JWT_ISSUER` and `JWT_AUDIENCE`. Local header mode requires `ALLOW_DEV_HEADERS=true`; HMAC requires both `JWT_HMAC_SECRET` and `ALLOW_HMAC_AUTH=true`. JWKS has precedence whenever its URL is nonempty. A failing JWKS branch does not fall through to header authentication. `internal/auth/auth.go:138-188`.

The local database fixture below deliberately has no production credentials and uses `sslmode=disable`, matching the RF1 CI fixture. The script exposes YSQL on host port 5433, and the API binds `:8080` on all interfaces. Run those fixtures only on a disposable host/VM or otherwise isolated local environment with appropriate access restrictions; the application has no `PORT` or bind-address setting to make it loopback-only. `scripts/ci/start-yugabytedb.sh:9-14`; `cmd/server/main.go:435`.

For setup order, prepare dependencies, create the disposable database, apply migrations, build the binaries, then start the selected runtime mode. Run durable test fixtures before starting a standalone worker against the same database, so that it cannot claim test events.

## 5. Start the frontend — not applicable to this repository

There is no frontend entrypoint, package manifest, bundler, browser route, static asset server, component library or frontend test command in the supplied tree. **There is no supported `npm install`, `npm run dev` or frontend URL to provide.** The Go mux registers only the eleven business operations and two operational paths. `cmd/server/main.go:386-410`.

If the assigned work concerns Customer Portal or another actual user interface, obtain that consumer's repository, owning team, runtime and approved integration scope first. [Report 05](05_FRONTEND_AUDIT.md) records the applicable/not-applicable boundaries; [report 07](07_FRONTEND_BACKEND_INTEGRATION_MATRIX.md) records available backend support. [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01) and [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07) determine the launch scope and consumers. The absence of a frontend in this backend service does not prove that a named consumer has no UI elsewhere.

## 6. Start the backend API

The API entrypoint is `cmd/server/main.go:413-455`. It requires `DATABASE_URL` unless `ALLOW_MEMORY_STORE=true`; memory mode is non-durable and has a known tenant-key parity defect ([F-026](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-026)). The recommended durable investigation path uses the disposable YSQL fixture in section 7 and binaries from section 10.

**Proposed authenticated development start**, after an owner supplies an approved development issuer and a dedicated development database:

```bash
: "${DATABASE_URL:?Supply the dedicated development YSQL DSN securely}"
: "${JWT_JWKS_URL:?Supply the approved development issuer JWKS URL}"
: "${JWT_ISSUER:?Supply its exact issuer}"
: "${JWT_AUDIENCE:?Supply the approved Consent audience}"
: "${NVUTO_BUILD_DIR:?Complete the build step first}"
env DATABASE_URL="$DATABASE_URL" \
  JWT_JWKS_URL="$JWT_JWKS_URL" JWT_ISSUER="$JWT_ISSUER" JWT_AUDIENCE="$JWT_AUDIENCE" \
  ALLOW_MEMORY_STORE=false ALLOW_DEV_HEADERS=false ALLOW_HMAC_AUTH=false \
  "$NVUTO_BUILD_DIR/server"
```

**Alternative, proposed isolated local header mode**, solely against the disposable fixture:

```bash
: "${NVUTO_LOCAL_DATABASE_URL:?Create and migrate the disposable YSQL fixture first}"
env -u JWT_JWKS_URL -u JWT_HMAC_SECRET -u JWT_ISSUER -u JWT_AUDIENCE \
  DATABASE_URL="$NVUTO_LOCAL_DATABASE_URL" \
  ALLOW_MEMORY_STORE=false ALLOW_DEV_HEADERS=true ALLOW_HMAC_AUTH=false \
  "$NVUTO_BUILD_DIR/server"
```

Header mode assigns the `developer` role from `X-Nvuto-Tenant-ID` and `X-Nvuto-Actor-ID`; it is not a production user impersonation or authorization test. Keep the server in the foreground and use Ctrl+C to request shutdown. The server's shutdown deadline is 15 seconds. `internal/auth/auth.go:180-186`; `cmd/server/main.go:442-455`.

From another terminal, the following **proposed read-only local checks** use only public health paths:

```bash
curl --fail --silent --show-error http://127.0.0.1:8080/healthz
curl --fail --silent --show-error http://127.0.0.1:8080/readyz
```

Expected response shapes are `{"status":"ok"}` and `{"status":"ready"}`. `/readyz` can return 503 `{"status":"not_ready"}`. A ready response currently establishes only the store readiness check, not correct auth configuration, applied schema, consumer operation or release readiness ([F-015](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-015); `cmd/server/main.go:389-398`; `internal/store/postgres.go:38-41`).

## 7. Start a disposable local YSQL database

**Not executed during the audit.** The supplied helper defaults to `yugabytedb/yugabyte:2026.1.0.1-b1`, starts a single YSQL node, creates the named database and checks that `yb_servers()` returns at least one server. `scripts/ci/start-yugabytedb.sh:4-37`.

The helper **unconditionally runs `docker rm -f` on its chosen container name before creation**. Its default name is `nvuto-consent-yugabyte`. Do not invoke it against a shared, production, existing development or data-bearing container. Choose a fresh name and check for a collision; never rerun this helper to “restart” a container whose data you intend to retain. The container's data is under `/tmp/nvuto-consent-yb-data` inside that container. `scripts/ci/start-yugabytedb.sh:5,9-14`.

**Proposed fresh local fixture:**

```bash
docker info >/dev/null
export NVUTO_LOCAL_RUN_ID="nvuto-consent-$(date -u +%Y%m%dT%H%M%SZ)-$$"
export YUGABYTEDB_CONTAINER="${NVUTO_LOCAL_RUN_ID}-yb"
if docker container inspect "$YUGABYTEDB_CONTAINER" >/dev/null 2>&1; then
  printf 'Refusing to replace existing container %s\n' "$YUGABYTEDB_CONTAINER" >&2
  exit 1
fi
export YUGABYTEDB_IMAGE='yugabytedb/yugabyte:2026.1.0.1-b1'
export YUGABYTEDB_STARTUP_TIMEOUT_SECONDS=300
bash scripts/ci/start-yugabytedb.sh consent
export NVUTO_LOCAL_DATABASE_URL='postgres://yugabyte@127.0.0.1:5433/consent?sslmode=disable'
```

The fixed `consent` argument matches the migration loop below and avoids interpolating an arbitrary database identifier into the helper's SQL. Confirm that port 5433 is unused before starting; the helper does not offer a host-port variable. Its startup timeout defaults to 300 seconds and can emit YSQL/yb-ctl diagnostics on failure. Do not resolve a port collision by removing an unfamiliar container. A successful single-node start is a functional fixture, not a YugabyteDB multi-node availability or recovery qualification.

## 8. Apply migrations safely and understand rollback limits

**Not executed during the audit.** On the freshly created, isolated `consent` database only, use the source CI loop. With `set -euo pipefail` from section 3, a failed command stops the setup; `ON_ERROR_STOP=1` prevents continuing silently within a failed SQL file.

```bash
: "${YUGABYTEDB_CONTAINER:?Select the fresh disposable container from section 7}"
for migration in migrations/[0-9][0-9][0-9]_*.sql; do
  docker exec -i "$YUGABYTEDB_CONTAINER" bin/ysqlsh \
    -h 127.0.0.1 -U yugabyte -d consent -v ON_ERROR_STOP=1 < "$migration"
done
```

This applies the supplied three migrations in order: **001** base consent/audit/outbox, **002** governed registries/notices/receipts/suppression/DOI and outbox evolution, **003** governed idempotency ledger. `.github/workflows/ci.yml:51-55`; `migrations/001_init.sql:1-45`; `migrations/002_policy_governance.sql:1-110`; `migrations/003_governed_idempotency.sql:1-11`.

The source has no migration CLI, version-tracking table, migration job or automatically verified rollback. The existing CI upgrade example exercises 001→002 and preserves a granted legacy row with blank notice version; it does not exercise 003 on that upgrade database. `UPGRADE_DATABASE_URL` is declared but unused. These are concrete gaps in [F-008](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-008) and [F-017](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-017), not evidence of a completed production upgrade.

Do not copy this empty-install loop into a production maintenance session. An existing-data upgrade needs the schema/data inventory, explicit migration owner, backup/restore proof, the approved legacy eligibility policy and a tested application compatibility plan. `migrations/ROLLBACK.md:3-5` proposes retaining additive evidence tables and rolling back the application; no compatible prior image or recovery drill was supplied. Its “PostgreSQL backup” wording must be interpreted against the actual YSQL target, with the platform owner's verified recovery procedure. See [report 08](08_DATABASE_AND_DATA_SCHEMA_AUDIT.md), [report 12](12_DEVOPS_INFRASTRUCTURE_AND_DEPLOYMENT_AUDIT.md) and [D-04](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-04)/[D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05).

## 9. Run tests and distinguish pass, skip and reproduced defect

### Observed audit commands and results

The following commands ran in an isolated copy using the absolute Go 1.25.13 binary recorded in [report 21](21_AUDIT_EVIDENCE_AND_COMMAND_LOG.md). Database, test, broker and authentication connection inputs were removed; `GOFLAGS=-mod=readonly` and `GOTOOLCHAIN=local` were set. The `go` spelling below abbreviates that recorded absolute executable, not a claim about the host's default Go.

| Observed command | Result and boundary |
|---|---|
| `go version`; `go mod download`; `go mod verify` | Go 1.25.13 linux/amd64; dependency download and integrity verification passed. V01/V02/V04. |
| `go test -json -count=1 ./...` | **15 PASS / 3 SKIP**, zero original-test failures. V03. |
| `go test -race -json -count=1 -coverpkg=./... -coverprofile=/workspace/scratch/ad3a679dbacf/audit-work/baseline_coverage.out ./...` | Same **15 PASS / 3 SKIP**; no reported race in executed paths; aggregate statement coverage **52.2%**. V07/V13. |
| `gofmt -l .`; `go vet ./...`; `bash -n scripts/ci/start-yugabytedb.sh` | Passed; syntax checking did not start the script. V05/V06/V08. |
| `go test -json -count=1 -run TestAudit ./cmd/server ./internal/service ./internal/auth` | Separate disposable probe copy: **11 failed safety assertions + 1 observation**, not modifications to the baseline suite. V11. |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 -json ./...` | Findings present despite JSON-mode exit 0. The retained scan contains 19 advisory findings, 10 with symbol traces; text conversion reports a failing finding status. V10/V15; [report 10](10_SECURITY_PRIVACY_AND_ACCESS_CONTROL_AUDIT.md). |

### Proposed portable baseline reproduction

Use a disposable source copy and clear inherited connection/authentication variables before testing. This reproduces the audit's dependency-free boundary; it deliberately leaves the three external tests skipped.

```bash
(
  unset DATABASE_URL TEST_DATABASE_URL TEST_NATS_URL
  unset JWT_JWKS_URL JWT_ISSUER JWT_AUDIENCE JWT_HMAC_SECRET
  unset ALLOW_DEV_HEADERS ALLOW_HMAC_AUTH ALLOW_MEMORY_STORE ALLOW_INSECURE_NATS
  unset NATS_URL NATS_CREDS_FILE NATS_TLS_CA_FILE NATS_STREAM NATS_SUBJECT_PREFIX NATS_REPLICAS
  "$NVUTO_GO" test -json -count=1 ./...
  "$NVUTO_GO" test -race -count=1 ./...
  "$NVUTO_GO" vet ./...
)
```

### Proposed durable tests — require isolated writable services

All three tests below were **SKIPPED** in the audit. They write fixtures, and the worker test creates/deletes a test JetStream stream. Run them once against a **fresh migrated fixture**, with the standalone API and worker stopped. The store test uses fixed globally keyed IDs such as `con-0`, `rcp-0` and `evt-0`; changing only its tenant is insufficient for safe repeated runs. A repeated test needs a new disposable database or a separately reviewed fixture repair under T-017. `internal/store/idempotency_integration_test.go:25-46`.

| Test | Required variables | What it actually exercises |
|---|---|---|
| `TestPostgresPolicyTransactionsAndSingleUseConfirmation` | `TEST_DATABASE_URL` | Service→YSQL transactions/DOI concurrency, not a real HTTP/IdP/consumer chain. `cmd/server/postgres_integration_test.go:17-83`. |
| `TestPostgresJetStreamOutboxConcurrency` | `TEST_DATABASE_URL`, `TEST_NATS_URL` | SQL claims and broker publication through test code. It connects directly with `nats.Connect(natsURL)`, not worker production credential options. `cmd/outbox-worker/postgres_integration_test.go:15-117`. |
| `TestPostgresGovernedIdempotencyConcurrentReplay` | `DATABASE_URL` | Durable concurrent replay/conflict contract with fixed global fixture IDs. `internal/store/idempotency_integration_test.go:14-73`. |

After the local YSQL/migrations and **local test broker** in section 11 are ready, these are proposed commands. Use a clean test shell as above, then set only the three fixture variables:

```bash
(
  unset JWT_JWKS_URL JWT_ISSUER JWT_AUDIENCE JWT_HMAC_SECRET
  unset ALLOW_DEV_HEADERS ALLOW_HMAC_AUTH ALLOW_MEMORY_STORE ALLOW_INSECURE_NATS
  unset NATS_URL NATS_CREDS_FILE NATS_TLS_CA_FILE NATS_STREAM NATS_SUBJECT_PREFIX NATS_REPLICAS
  export DATABASE_URL="$NVUTO_LOCAL_DATABASE_URL"
  export TEST_DATABASE_URL="$NVUTO_LOCAL_DATABASE_URL"
  export TEST_NATS_URL="$NVUTO_LOCAL_TEST_NATS_URL"
  "$NVUTO_GO" test -race ./cmd/server -run '^TestPostgresPolicyTransactionsAndSingleUseConfirmation$' -count=1 -v
  "$NVUTO_GO" test -race ./cmd/outbox-worker -run '^TestPostgresJetStreamOutboxConcurrency$' -count=1 -v
  "$NVUTO_GO" test -race ./internal/store -run '^TestPostgresGovernedIdempotencyConcurrentReplay$' -count=1 -v
)
```

Do not follow that sequence with an indiscriminate second `go test ./...` against the same fixture: it would rerun the fixed-ID store test. The source CI selects the first two durable tests but omits the third. Its RF1/single-broker design does not establish multi-node YSQL or broker failover safety. [F-016](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-016)/F-017 remain open until the required durable evidence exists.

For defect reproduction, use [report 14's isolated-copy procedure](14_TESTING_QA_AND_VERIFICATION_REPORT.md#portable-reproduction-of-the-audit-probes). Probe sources are retained under `evidence/probes/` as **`.go.txt`** so documentation does not add mixed-package Go sources to `go test ./...`. Copy each into its specified package only in a disposable copy. The 12 probes map to nine canonical findings plus one open-rule observation: memory/httptest checks do not prove SQL execution, the receipt probe exercises the real pgx timestamp codec without a database, and the JWKS probe uses a local HTTP server without a real issuer. F-030's mixed-role behavior is source-confirmed and has **no additional dynamic probe**. Full expected/observed cases and future AT-001..AT-032 are in report 14.

## 10. Build the two programs and understand archive provenance

**Observed:** the initial plain build failed with `error obtaining VCS status: exit status 128` because the ZIP extraction sat within an incomplete surrounding workspace. An otherwise identical build with `-buildvcs=false` successfully produced both programs. This is an archive environment issue, not a source compile defect. The archive still has no known commit or branch. [V09](evidence/V09_BUILD.log); [V12](evidence/V12_ARCHIVE_BUILD.log).

**Proposed portable archive build:**

```bash
export NVUTO_BUILD_DIR="$(mktemp -d -t nvuto-consent-build-XXXXXXXX)"
"$NVUTO_GO" build -buildvcs=false -o "$NVUTO_BUILD_DIR/" ./cmd/server ./cmd/outbox-worker
```

The resulting executables are `$NVUTO_BUILD_DIR/server` and `$NVUTO_BUILD_DIR/outbox-worker`. The application has no separate generated frontend build, migration binary or SDK generation step.

The CI also defines the following local container builds; **neither was run in the audit**:

```bash
docker build -t nvuto-consent:test .
docker build -f Dockerfile.worker -t nvuto-consent-worker:test .
```

Both Dockerfiles use a Go 1.25.13 Alpine build stage with `CGO_ENABLED=0`, Linux output and a nonroot distroless runtime. They do not package/run database migrations. `Dockerfile:1-12`; `Dockerfile.worker:1-11`. Container builds, image scans, a source commit, SBOM/provenance and immutable image digests remain release work under [T-018](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-018); a successful local archive binary is insufficient provenance for promotion.

## 11. Understand service communication and start the worker

The API synchronously calls its repository; governed durable mutations commit consent/evidence, audit and outbox effects together. A separate worker claims outbox rows from YSQL, publishes versioned CloudEvents to JetStream, then records completion or retry. Consumers must independently implement delivery handling and actual outbound enforcement. The API does not call the eight named consumers directly and does not deliver DOI email/SMS. `internal/store/postgres.go:238-259,353-389`; `cmd/outbox-worker/main.go:46-145`; [report 06](06_BACKEND_AND_API_AUDIT.md#6-event-contract-available-to-consumers).

**Proposed credentialed development worker:** use a dedicated development database plus a broker/stream assigned by the messaging owner. Obtain the credentials file and any custom CA securely. The worker may create the named stream, so this must be an authorized development account/namespace, not a discovered shared account.

```bash
: "${DATABASE_URL:?Supply the dedicated development YSQL DSN securely}"
: "${NATS_URL:?Supply the approved development broker URL}"
: "${NATS_CREDS_FILE:?Supply the securely provisioned credentials file path}"
: "${NATS_STREAM:?Supply the assigned development stream name}"
: "${NATS_SUBJECT_PREFIX:?Supply the assigned development subject prefix}"
test -r "$NATS_CREDS_FILE"
env DATABASE_URL="$DATABASE_URL" NATS_URL="$NATS_URL" \
  NATS_CREDS_FILE="$NATS_CREDS_FILE" NATS_TLS_CA_FILE="${NATS_TLS_CA_FILE:-}" \
  NATS_STREAM="$NATS_STREAM" NATS_SUBJECT_PREFIX="$NATS_SUBJECT_PREFIX" \
  NATS_REPLICAS="${NATS_REPLICAS:-3}" ALLOW_INSECURE_NATS=false \
  "$NVUTO_BUILD_DIR/outbox-worker"
```

This does not provision broker credentials, guarantee TLS or validate an existing stream's replica/retention settings. Those depend on the approved broker configuration and [F-019](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-019)/[F-020](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-020). The default requested replicas is three, with accepted range 1–5. `cmd/outbox-worker/main.go:149-203`.

**Separate proposed unauthenticated local test fixture:** the following adapts CI's single-server `nats:2.10-alpine -js` command to a fresh container name and a loopback-only published port. No real credentials are used. Run this only for the disposable local session, with port 4222 free:

```bash
: "${NVUTO_LOCAL_RUN_ID:?Complete the disposable fixture setup first}"
export NVUTO_LOCAL_NATS_CONTAINER="${NVUTO_LOCAL_RUN_ID}-nats"
docker run --rm -d --name "$NVUTO_LOCAL_NATS_CONTAINER" \
  -p 127.0.0.1:4222:4222 nats:2.10-alpine -js
export NVUTO_LOCAL_TEST_NATS_URL='nats://127.0.0.1:4222'
docker logs "$NVUTO_LOCAL_NATS_CONTAINER"
```

Wait for broker startup before the durable worker test. CI polls the local TCP port for up to 30 seconds; that checks connectivity, not the final security or stream contract. `.github/workflows/ci.yml:65-72`. The original integration test does not load `NATS_CREDS_FILE`, so its existing plain local-broker path must not be presented as credentialed integration coverage.

For a **bounded, optional local worker demonstration after fixture tests**, GNU `timeout` limits this foreground run to 60 seconds; timeout exit 124 is expected when the limit ends it. This is a helper command, not a verified worker shutdown test:

```bash
timeout --signal=TERM --kill-after=5s 60s \
  env -u NATS_CREDS_FILE -u NATS_TLS_CA_FILE \
  DATABASE_URL="$NVUTO_LOCAL_DATABASE_URL" NATS_URL="$NVUTO_LOCAL_TEST_NATS_URL" \
  NATS_STREAM=NVUTO_CONSENT_DEV NATS_SUBJECT_PREFIX=nvuto.consent.dev \
  NATS_REPLICAS=1 ALLOW_INSECURE_NATS=true \
  "$NVUTO_BUILD_DIR/outbox-worker"
```

Use a separate terminal for the timeout command so its expected nonzero exit does not interrupt other setup under `set -e`. Stop only your recorded disposable containers when finished, using `docker stop "$NVUTO_LOCAL_NATS_CONTAINER"` and `docker stop "$YUGABYTEDB_CONTAINER"`. Stopping the `--rm` NATS fixture removes that deliberately disposable broker. The YSQL helper must not be rerun against retained data. None of these local fixtures validates lease fencing, reverse-order delivery, consumer deduplication, expiry propagation or HA ([F-009](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-009)/[F-010](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-010)).

## 12. Find and use the exact API contracts

The maintained handover contract is [report 06, endpoint inventory and schemas](06_BACKEND_AND_API_AUDIT.md#3-exact-active-endpoint-inventory), backed by the handler and domain source. The repository supplies no OpenAPI document, generated client or consumer contract-test suite ([F-024](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-024)). All eleven business operations require an authenticated tenant principal; roles further restrict them.

| Contract ID | Method and path | Operation |
|---|---|---|
| API-01 | `POST /v1/purposes` | Upsert purpose policy. |
| API-02 | `POST /v1/lawful-bases` | Upsert lawful-basis reference. |
| API-03 | `POST /v1/notices` | Insert an immutable notice version. |
| API-04 | `POST /v1/consents` | Governed grant/deny, with optional `Idempotency-Key`. |
| API-05 | `POST /v1/consents/{id}/withdraw` | Withdraw the identified consent. |
| API-06 | `POST /v1/suppressions` | Set/clear a global, channel or purpose/channel suppression. |
| API-07 | `POST /v1/evaluate` | Obtain one authorization decision. |
| API-08 | `POST /v1/evaluate:batch` | Evaluate 1–100 requests. |
| API-09 | `POST /v1/double-opt-in` | Create pending confirmation and currently return a raw token. |
| API-10 | `POST /v1/double-opt-in/confirm` | Consume the token and confirm. |
| API-11 | `GET /v1/receipts/{id}` | Retrieve an authorized receipt. |

`cmd/server/main.go:120-383,399-409` is the active dispatch/handler source. OPS-01 `/healthz` and OPS-02 `/readyz` are public and currently accept every HTTP method, although normal callers should use GET. There is no consent-history, current-preference list, registry list, suppression list or audit-history read API.

The handler uses JSON, a one-MiB request-body limit and unknown-field rejection, but currently accepts a second trailing JSON value. Common explicit responses include 400 decode failure, 401 authentication failure, 403 role/subject denial, 404 missing record, 405 wrong method, 409 conflict, 410 expired challenge and 422 validation/default error. Unexpected storage errors also default to 422 with raw text; do not treat all 422 responses as safe user-correctable input errors. Evaluation can return HTTP 200 with `allowed=false`, which is a normal decision. `cmd/server/main.go:59-107`; [F-013](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-013)/[F-027](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-027).

Only API-04 supports the idempotency header. Preserve a key and the semantic request for a retry; changed content can conflict. Current replay validates live policy first and returns the current consent row, so it is not a byte-identical original-response guarantee. Actor identity and timestamp representation also matter. Resolve the contract in [D-11](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-11) and [F-025](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-025) before implementing consumer retry assumptions.

## 13. Understand identities, roles and permissions

`subject_id` denotes the canonical Customer Graph party, not a login name. JWT `sub` identifies the actor, while `customer_subject_id` supplies the self-service customer binding. Tenant and claim provisioning belong to the external identity/customer owners. Role strings are exact and case-sensitive. `README.md:41`; `internal/auth/auth.go:16-31,119-135,190-200`.

| Role | Current route permissions and boundary |
|---|---|
| `privacy_admin` | Registry writes, preference writes and decisions/receipts across the token's tenant. |
| `consent_admin` | Same current route sets as privacy administrator. |
| `consent_writer` | Preference/DOI/suppression writes and decisions/receipts across the token's tenant; no registry writes. |
| `consent_reader` | Decisions and receipts; no writer route by this role alone. |
| `service` | Decisions and receipts; no writer route by this role alone. |
| `self_service` | Preference/DOI/suppression writes and decisions/receipts, intended to be restricted to the matching canonical subject. |
| `developer` | Same broad route sets as administrator. A JWT carrying this role is not separately restricted by a development-mode flag. |

`cmd/server/main.go:109-118` defines these sets. Two known exceptions materially affect integration: DOI confirmation omits the customer boundary ([F-002](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-002)), and a mixed `self_service` plus `consent_reader` or `service` token can enter a writer route through the first role and bypass own-subject restriction through the second. This is **P1 F-030, confirmed from source; actual issued combinations are NOT VERIFIED**, with no new dynamic probe. [G-030](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-030) remains mandatory until scope is fixed or a concrete deployment/IdP contract excludes unsafe combinations with negative gates. Broad suppression-clearing authority remains [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06). Use [report 04](04_USER_PERSONAS_ROLES_AND_PERMISSIONS.md) and [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) before provisioning client roles.

## 14. Learn the eleven workflows and their edge cases

Each workflow in [report 09](09_APPLICATION_USER_FLOW_AUDIT.md) includes success/failure/edge behavior, RBAC, storage, integration dependencies and finding/test mappings. Preserve these IDs in tickets and consumer UAT evidence.

| Workflow | Name | First issue to understand |
|---|---|---|
| [WF-01](09_APPLICATION_USER_FLOW_AUDIT.md#wf-01) | Authenticate and map subject | Canonical identity and safe mixed-role issuance. |
| [WF-02](09_APPLICATION_USER_FLOW_AUDIT.md#wf-02) | Maintain purpose/basis/notice | Mutable policy versus immutable notice versions; no list/read workflow. |
| [WF-03](09_APPLICATION_USER_FLOW_AUDIT.md#wf-03) | Capture or deny preference | Policy/evidence validation and retry semantics; returned receipt is not independently verified proof. |
| [WF-04](09_APPLICATION_USER_FLOW_AUDIT.md#wf-04) | Load and edit existing preferences | Current-preference discovery API and actual owning UI are absent/unverified; scope decision required. |
| [WF-05](09_APPLICATION_USER_FLOW_AUDIT.md#wf-05) | Withdraw consent | Client needs a retained consent ID; old-record and newer-state ordering require explicit semantics. |
| [WF-06](09_APPLICATION_USER_FLOW_AUDIT.md#wf-06) | DOI collection and confirmation | Raw-token self-confirmation, cross-subject confirmation and old-challenge resurrection. |
| [WF-07](09_APPLICATION_USER_FLOW_AUDIT.md#wf-07) | Set/clear layered suppression | Scope precedence, clearing authority and missing before/after audit history. |
| [WF-08](09_APPLICATION_USER_FLOW_AUDIT.md#wf-08) | Authorize one outbound action | Missing affirmative consent must fail closed; historical `at` is not a reconstructed timeline. |
| [WF-09](09_APPLICATION_USER_FLOW_AUDIT.md#wf-09) | Batch authorization | 1–100 boundary, customer authorization and consumer interpretation of each denial. |
| [WF-10](09_APPLICATION_USER_FLOW_AUDIT.md#wf-10) | Retrieve evidence and investigate history | Receipt retrieval exists; accepted-content binding, hash durability and full audit retrieval remain incomplete. |
| [WF-11](09_APPLICATION_USER_FLOW_AUDIT.md#wf-11) | Publish and consume a change | SQL completion is not consumer enforcement; ordering, expiry, retries and reconciliation need proof. |

Read the policy and transition source in `internal/service/service.go:51-390` with the matching tests. Use proposed acceptance groups in [report 14](14_TESTING_QA_AND_VERIFICATION_REPORT.md), not only happy-path curl results, to assess an implementation change.

## 15. Locate the complete environment-variable contract

[Report 12, section 3](12_DEVOPS_INFRASTRUCTURE_AND_DEPLOYMENT_AUDIT.md#3-complete-environment-variable-and-configuration-inventory) is the full per-variable inventory with defaults, readers, manifests, sensitivity and validation. There are **21 named source/workflow variables: 20 actively read by Go or the shell helper, plus one unused CI declaration**. This is not a claim that all 21 are production runtime settings.

| Group | Exact variables | Source |
|---|---|---|
| API/store (2) | `DATABASE_URL`, `ALLOW_MEMORY_STORE` | `cmd/server/main.go:417-430`; `DATABASE_URL` also serves worker and store test. |
| Authentication (6) | `JWT_JWKS_URL`, `JWT_ISSUER`, `JWT_AUDIENCE`, `JWT_HMAC_SECRET`, `ALLOW_HMAC_AUTH`, `ALLOW_DEV_HEADERS` | `internal/auth/auth.go:138-188`. |
| Worker messaging (7) | `NATS_URL`, `NATS_STREAM`, `NATS_SUBJECT_PREFIX`, `NATS_REPLICAS`, `NATS_CREDS_FILE`, `ALLOW_INSECURE_NATS`, `NATS_TLS_CA_FILE` | `cmd/outbox-worker/main.go:150-192`. |
| Dedicated test selectors (2) | `TEST_DATABASE_URL`, `TEST_NATS_URL` | Server/worker external tests; the third durable test uses `DATABASE_URL`. |
| YSQL fixture (3) | `YUGABYTEDB_CONTAINER`, `YUGABYTEDB_IMAGE`, `YUGABYTEDB_STARTUP_TIMEOUT_SECONDS` | `scripts/ci/start-yugabytedb.sh:5-7`. |
| CI-only unused declaration (1) | `UPGRADE_DATABASE_URL` | `.github/workflows/ci.yml:39`; no code/script reads it. |

The guide's `NVUTO_*` variables are local shell conveniences, not application configuration. Likewise `GOTOOLCHAIN`, `GOFLAGS`, cache paths and inherited proxy/pgx settings are tool/library behavior outside the 21 application/test/helper inventory. Authentication settings are read per request; worker configuration is read at startup. No general credential hot-reload guarantee exists. Configure production transport, secret injection/rotation and connection budgets through the owning platform team and the full report 12 contract.

## 16. Identify integration owners and missing evidence

The README names eight primary consumers. **All eight are NOT VERIFIED integrations**: no consumer source, deployed endpoint, service account, contract result or end-to-end user journey was supplied. `README.md:9`; [report 07](07_FRONTEND_BACKEND_INTEGRATION_MATRIX.md); [report 11](11_THIRD_PARTY_INTEGRATIONS_AUDIT.md).

| ID | Named consumer | ID | Named consumer |
|---|---|---|---|
| INT-01 | Engage | INT-05 | CRM |
| INT-02 | Service | INT-06 | Conversations |
| INT-03 | Contact Center | INT-07 | AI |
| INT-04 | Customer Portal | INT-08 | Integration Hub |

Obtain the selected launch set and named owners under D-07. Each selected consumer needs its real repository/version, identity mapping, permitted role set, retry and decision contract, event handling if applicable, and evidence that denial/withdrawal/suppression prevents the actual outbound action. The table does not assume that every consumer uses every endpoint or receives events.

Infrastructure dependencies EXT-01..EXT-12 are fully inventoried in report 11. The immediate development dependencies are the Go/module supply chain, YSQL, JetStream, the issuer/JWKS service and canonical Customer Graph identity. Production additionally needs registry/release, cluster/gateway/TLS, secrets/permissions, monitoring and recovery owners. DOI delivery and independently demonstrated channel proof require [D-02](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-02); a returned token is not evidence of delivery. These are external dependencies to resolve, not services secretly implemented by this repository.

## 17. Diagnose failures using observed evidence and source behavior

| Symptom | Evidence-based interpretation and next step |
|---|---|
| `error obtaining VCS status: exit status 128` | Observed V09 archive/workspace issue. Use the explicit archive build in section 10; preserve the lack of commit provenance in release records. |
| External tests report SKIP | Observed V03/V07. Check the exact gate names in section 9; supplying only `TEST_DATABASE_URL` does not enable the store test, and the worker also needs `TEST_NATS_URL`. A skip is not a pass. |
| `DATABASE_URL is required unless ALLOW_MEMORY_STORE=true` | API startup guard, `cmd/server/main.go:426-430`. Supply the deliberately selected database; do not turn on memory to conceal a durable environment failure. |
| `postgres unavailable: ...` or `using PostgreSQL storage` | API's generic pgx naming, `cmd/server/main.go:417-424`. Check the selected YSQL DSN and fixture readiness; this log does not identify the actual database engine. |
| Ready response but business request fails | Auth is checked per request; readiness does not verify auth or schema. Check migration state and issuer/audience configuration; F-015 remains open. |
| 401 `authentication is not configured` / `JWKS authentication requires issuer and audience` | Explicit auth branches, `internal/auth/auth.go:139-145,187`. Check the selected mode and required claims; do not bypass an issuer failure by enabling development auth in a deployed environment. |
| 403 `role not authorized` / customer denial | Route role gate and subject comparison. Compare exact role strings, token tenant and canonical `customer_subject_id`; account for F-002/F-030 before concluding the matrix is secure. |
| Worker says `DATABASE_URL and NATS_URL are required` or requires a credentials file | Startup checks at `cmd/outbox-worker/main.go:150-153,186-190`. Securely supply the assigned inputs; insecure mode belongs only to the bounded disposable fixture. |
| `NATS_REPLICAS must be between 1 and 5` or stream creation fails locally | Parse guard at worker lines 163-169; single-server fixture uses 1. Existing-stream acceptance does not validate its settings. |
| YSQL startup timeout or port conflict | Use the helper's emitted diagnostics and inspect only the recorded fixture. Do not rerun its force-removal path against a retained/shared container. |
| Duplicate keys on a repeated store integration test | Inspect its fixed global IDs and whether the database is fresh; F-017. Do not delete unfamiliar rows to get a green result. |
| Scanner JSON process exits 0 while finding records exist | Observed V10. Inspect report 10 and the captured text conversion; use the actual vulnerability gate, not JSON exporter exit status alone. |
| 200 `allowed=false`, or 422 after a DB problem | Denial can be normal; current 422 can also conceal storage failure. Read the decision reason and sanitized server evidence, then trace the service/store call; F-013. |

The programs provide process/error logs but no complete request correlation, delivery-lag dashboard or readiness/alert package. Avoid pasting tokens, raw proof, credential contents or customer data into incident notes. A useful bug report records the selected source/version, sanitized configuration mode, endpoint/test, tenant/subject relationships using synthetic identifiers, expected result, actual result and relevant state/event counts. See [T-021](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-021) for the missing operational evidence.

## 18. Navigate the handover and preserve evidence

| Question | Read |
|---|---|
| What is this service and what exists? | [00 executive summary](00_EXECUTIVE_AUDIT_SUMMARY.md), [01 architecture](01_REPOSITORY_ARCHITECTURE_MAP.md), [02 requirements](02_DOCUMENTATION_REQUIREMENTS_TRACEABILITY.md), [03 feature audit](03_COMPLETE_PRODUCT_FEATURE_AUDIT.md). |
| Which actor can do what, and through which client/API/flow? | [04 roles](04_USER_PERSONAS_ROLES_AND_PERMISSIONS.md), [05 frontend scope](05_FRONTEND_AUDIT.md), [06 backend/API](06_BACKEND_AND_API_AUDIT.md), [07 integration matrix](07_FRONTEND_BACKEND_INTEGRATION_MATRIX.md), [09 workflows](09_APPLICATION_USER_FLOW_AUDIT.md). |
| What constrains data, security, integrations and operations? | [08 schema](08_DATABASE_AND_DATA_SCHEMA_AUDIT.md), [10 security](10_SECURITY_PRIVACY_AND_ACCESS_CONTROL_AUDIT.md), [11 dependencies](11_THIRD_PARTY_INTEGRATIONS_AUDIT.md), [12 DevOps](12_DEVOPS_INFRASTRUCTURE_AND_DEPLOYMENT_AUDIT.md), [13 non-functional requirements](13_NON_FUNCTIONAL_REQUIREMENTS_AUDIT.md). |
| What was actually tested? | [14 verification and acceptance tests](14_TESTING_QA_AND_VERIFICATION_REPORT.md), [21 command log](21_AUDIT_EVIDENCE_AND_COMMAND_LOG.md), retained `evidence/` logs and `.go.txt` probes. |
| What must change, who decides, and what blocks release? | [15 risks](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md), [16 implementation backlog](16_DETAILED_IMPLEMENTATION_BACKLOG.md), [19 go-live gates](19_PRODUCTION_GO_LIVE_CHECKLIST.md), [20 decisions/dependencies](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md). |

F-001..F-032, T-001..T-032, AT-001..AT-032 and G-001..G-032 share suffixes for the same finding/work/acceptance/gate chain. RQ-001..RQ-022 are source requirements; WF-01..WF-11 are workflows; API-01..API-11 and OPS-01..OPS-02 are contracts; INT-01..INT-08 are named consumers. Do not renumber these independently or substitute provisional audit IDs.

Report 21's command metadata is normalized to UTC. Raw Go JSON logs preserve their original timestamp offsets, including `+01:00`; the differing display offset is not a second run or evidence alteration. Retain originals when adding a human-readable summary. An argv-only record also does not encode stdin redirection; report 21 explains the captured vulnerability JSON consumed by V14/V15. V15 is a conversion of V10, not another independent scan.

## 19. Understand deployment intent without treating it as a deployment result

The supplied manifests describe three API replicas, two worker replicas, disruption budgets, restricted/nonroot security settings and API autoscaling. They reference external configuration and secrets but do not provision a YSQL universe, a broker account/cluster, a gateway, TLS policy, monitoring or backup infrastructure. Mutable `:latest` and `:latest-worker` images do not identify a reproducible release. `deploy/k8s.yaml:1-129`; `deploy/outbox-worker.yaml:1-83`; [report 12](12_DEVOPS_INFRASTRUCTURE_AND_DEPLOYMENT_AUDIT.md).

The CI definition contains Go checks, Docker builds, RF1 YSQL setup, an empty install, a partial upgrade and local NATS tests. The audit did not observe an authenticated CI run for a source commit or any deployment. README claims about CI and HA are therefore read as declared implementation intent, qualified by the missing/defective cases in reports 12 and 14. `.github/workflows/ci.yml:11-122`; `README.md:19-21,49-51`.

Before any environment promotion, the owners must provide immutable artifact identities, secrets/configuration, supported patched runtime, target transport and permissions, complete migration evidence, correct readiness, required negative security/state tests, observability and a tested recovery/rollback procedure. [D-09](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) establishes infrastructure/release/operations ownership. This guide does not supply a `kubectl apply` production shortcut: the approved, concrete release and gate evidence must exist first.

## 20. Validate readiness and choose the first engineering work

Use [report 19](19_PRODUCTION_GO_LIVE_CHECKLIST.md) as the release checklist and [report 16](16_DETAILED_IMPLEMENTATION_BACKLOG.md) as the work definition. Local health, a compiled binary, 15 passing baseline tests, RF1 fixture success or a broker acknowledgment cannot individually satisfy production readiness. Each relevant gate needs its exact expected behavior, actual evidence, target version/environment and named reviewer.

The first engineering pass should address the reproduced consent/DOI boundary and ordering defects, receipt durability/evidence, and mixed-role write authorization; align approved time/policy/legacy rules; then prove the durable invariants and worker lease/event behavior on the target infrastructure. Supported dependency remediation, bounded DB/JWKS work, release identity and operational controls are also mandatory work, not optional polish. The risk/backlog documents own severity and dependency order; this guide deliberately does not create a competing priority table.

Before treating the handover as understood, a new engineer should be able to trace a governed grant and withdrawal from authenticated handler through service and transaction, explain what the current receipt does and does not prove, identify the three skipped-test prerequisites, reproduce the archive build and isolated probes, name the chosen consumer/identity/operations owners, and attach evidence to the matching AT/G entry. Scope/region decisions, real consumer journeys, multi-node resilience, restore/PITR and issued-role combinations remain unverified until their owners produce the evidence specified in the linked reports.
