# 21 — Audit evidence and command log

**Product:** NVUTO Consent  
**Audit date:** 19 October 2026 (command timestamps in UTC; raw log offsets preserved)  
**Source:** supplied ZIP; commit and branch UNKNOWN  
**Disposition:** documentation-only audit; original source preserved.

## Provenance and scope

| Field | Recorded value |
| --- | --- |
| Product / supplied folder | Nvuto Consent / Nvuto-Consent-main |
| Original archive | Nvuto-Consent-main (1)(1).zip |
| Archive SHA-256 | `d8888747971b7e8d876ae1d935b9559699164cc8c63c8fa3a154df1a007dfe99` |
| Source revision / commit | **UNKNOWN** — no .git/history/commit manifest supplied |
| Branch | **UNKNOWN** — folder suffix-main is not evidence |
| Audit date | 19 October2026; verification timestamps below are UTC |
| Original inventory |32 files;19 Go files;10 test files;18 original Test functions |
| Original bytes / line count |159,241 bytes / 3,958 lines |
| Source inspection | Entire supplied32-file repository, with parallel API/product, backend/data and security/operations reviews |
| Verification scope | Isolated unchanged baseline plus separate audit-probe copy; no product source fix |
| Implementation/migration/deployment | None performed |

`source_inventory.json` records every original file's path, bytes, line count and SHA-256. The final package comparison in `package_validation.json` checks the delivered source files against this inventory and the preserved extracted baseline. Only `docs/production-readiness-audit/` is added in the handover. No original PRD/TRD or rollback documentation was overwritten. The audit instruction attachment defined the work, but no missing requirements document was fabricated.

## Toolchain provenance and execution boundary

Go, Docker and psql were not initially installed in the audit shell. A local Go1.25.13 linux/amd64 toolchain was fetched from the official Go distribution to reproduce the exact pinned build. Its archive is59,908,810 bytes and SHA-256 `39042a078ea9ceebe3ecda4a7188f0f5b96e14a071d27923ba7f40b456e85ae3`. It was extracted only into the disposable audit workspace. The SDK, module cache and binaries are not part of the delivered handover.

Official source for toolchain availability: [Go downloads](https://go.dev/dl/). The module graph was downloaded with read-only module settings and verified. Verification used isolated GOCACHE/GOMODCACHE/GOPATH, GOTOOLCHAIN=local, GOENV=off, GOWORK=off and official Go proxy/checksum settings. Real database/test/NATS/authentication connection variables were removed before the verification subprocesses. Tests use synthetic subjects, tokens and evidence.

The default sandbox prevented network/module fetch and localhost test sockets. Necessary official toolchain/module fetches and local mocked-HTTP tests were run with approved execution access. No automatic approval rejection occurred and no unsafe workaround was used. V14 records one environment network failure before the successful conversion in V15. No databases or containers were created; no migration, live user action, external delivery or deployment was run.

## Exact verification command log

These are the15 individually captured verification commands. Each row gives exact argv in the original isolated workspace, UTC start, outer process status and an unmodified output log. Read-only source discovery used archive listing, rg file/text search, nl/sed/cat inspection and Python inventory/YAML/JSON analysis; those inspection families are documented here but the audit did not install a shell-wide recorder of every exploratory read. The retained command log is therefore the verification-command record, not a claim to be a complete terminal transcript.

| ID / UTC start | Working directory and exact command | Exit / result | Log |
| --- | --- | --- | --- |
| V01_GO_VERSION / 2026-10-19T12:35:26.451190+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go version` | 0 / PASS | [V01_GO_VERSION.log](evidence/V01_GO_VERSION.log) |
| V02_MODULE_DOWNLOAD / 2026-10-19T12:35:33.889772+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go mod download` | 0 / PASS | [V02_MODULE_DOWNLOAD.log](evidence/V02_MODULE_DOWNLOAD.log) |
| V03_BASELINE_TEST / 2026-10-19T12:36:29.906140+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go test -json -count=1 ./...` | 0 / PASS WITH SKIPS | [V03_BASELINE_TEST.log](evidence/V03_BASELINE_TEST.log) |
| V04_MODULE_VERIFY / 2026-10-19T12:36:22.609866+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go mod verify` | 0 / PASS | [V04_MODULE_VERIFY.log](evidence/V04_MODULE_VERIFY.log) |
| V05_BASH_SYNTAX / 2026-10-19T12:37:26.242482+00:00 | `audit-work/baseline-repo` — `bash -n scripts/ci/start-yugabytedb.sh` | 0 / PASS | [V05_BASH_SYNTAX.log](evidence/V05_BASH_SYNTAX.log) |
| V06_GO_FORMAT / 2026-10-19T12:37:26.535257+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/gofmt -l .` | 0 / PASS | [V06_GO_FORMAT.log](evidence/V06_GO_FORMAT.log) |
| V07_RACE_COVERAGE / 2026-10-19T12:37:54.015822+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go test -race -json -count=1 -coverpkg=./... -coverprofile=/workspace/scratch/ad3a679dbacf/audit-work/baseline_coverage.out ./...` | 0 / PASS WITH SKIPS | [V07_RACE_COVERAGE.log](evidence/V07_RACE_COVERAGE.log) |
| V08_GO_VET / 2026-10-19T12:37:49.174778+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go vet ./...` | 0 / PASS | [V08_GO_VET.log](evidence/V08_GO_VET.log) |
| V09_BUILD / 2026-10-19T12:37:54.633960+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go build -o /workspace/scratch/ad3a679dbacf/audit-work/binaries/ ./cmd/server ./cmd/outbox-worker` | 1 / ENVIRONMENT FAILURE | [V09_BUILD.log](evidence/V09_BUILD.log) |
| V10_GOVULNCHECK / 2026-10-19T12:37:55.797531+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 -json ./...` | 0 / FINDINGS PRESENT | [V10_GOVULNCHECK.log](evidence/V10_GOVULNCHECK.log) |
| V11_AUDIT_REGRESSION_PROBES / 2026-10-19T12:39:16.960449+00:00 | `audit-work/probe-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go test -json -count=1 -run TestAudit ./cmd/server ./internal/service ./internal/auth` | 1 / 11 SAFETY ASSERTIONS FAIL;1 OBSERVATION | [V11_AUDIT_REGRESSION_PROBES.log](evidence/V11_AUDIT_REGRESSION_PROBES.log) |
| V12_ARCHIVE_BUILD / 2026-10-19T12:40:26.139795+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go build -buildvcs=false -o /workspace/scratch/ad3a679dbacf/audit-work/binaries/ ./cmd/server ./cmd/outbox-worker` | 0 / PASS | [V12_ARCHIVE_BUILD.log](evidence/V12_ARCHIVE_BUILD.log) |
| V13_COVERAGE_SUMMARY / 2026-10-19T12:40:28.294418+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go tool cover -func=/workspace/scratch/ad3a679dbacf/audit-work/baseline_coverage.out` | 0 / PASS | [V13_COVERAGE_SUMMARY.log](evidence/V13_COVERAGE_SUMMARY.log) |
| V14_VULN_TEXT_CONVERSION / 2026-10-19T12:41:28.537525+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 -mode=convert` | 1 / ENVIRONMENT FAILURE | [V14_VULN_TEXT_CONVERSION.log](evidence/V14_VULN_TEXT_CONVERSION.log) |
| V15_VULN_TEXT_CONVERSION / 2026-10-19T12:42:20.207682+00:00 | `audit-work/baseline-repo` — `/workspace/scratch/ad3a679dbacf/audit-work/toolchain/go/bin/go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 -mode=convert` | 1 / FINDINGS PRESENT | [V15_VULN_TEXT_CONVERSION.log](evidence/V15_VULN_TEXT_CONVERSION.log) |

**Additional input:** V14 and V15 passed the contents of `audit-work/govulncheck_stream.json` to standard input. That file contains the parsed JSON scan records from V10 without the Go bootstrap output. V15 ran `go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 -mode=convert` against those records. The child scanner reported exit status3 for findings; `go run` returned1. V10 JSON mode returned0 despite findings. This interpretation does not assert a flaw in the repository's existing text-mode CI scanner command.

**Archive build:** V09 failed obtaining VCS status from surrounding workspace metadata. V12 explicitly used `-buildvcs=false` and built both commands. The source ZIP lacks a commit, so this flag is an appropriate reproducibility adjustment for the audit, not a product fix. Production artifacts still need a known source commit and provenance.

## Additional static and packaging evidence

`static_config_check.json` records successful YAML parsing: one workflow document, five API Kubernetes objects and two worker objects. It does not validate Kubernetes API schemas, admission policy, referenced secrets/configuration, image availability, scheduling or runtime readiness. `bash -n` checked shell syntax without running the helper. Source inventory and reference/count consistency checks are retained in `package_validation.json`; report22's dependency graph and reports14–19's ID chains are validated before final packaging.

## Evidence file index

| Artifact | Purpose / interpretation |
| --- | --- |
| [source_inventory.json](evidence/source_inventory.json) | All32 original files and individual hashes/line counts |
| [audit_catalog.json](evidence/audit_catalog.json) | Machine-readable34 features,22 requirements,8 modules,32 finding/ticket/test/gate chains and14 decisions |
| [canonical_findings.json](evidence/canonical_findings.json) | Canonical finding details and conservative task dependencies |
| [test_inventory.json](evidence/test_inventory.json) | 18 named original tests with source locations and external prerequisite flags |
| [command_log.jsonl](evidence/command_log.jsonl) | 15 core verification commands, timestamps, argv, exits and environment summary |
| [static_config_check.json](evidence/static_config_check.json) | YAML syntax/object inventory only |
| [baseline_coverage.out](evidence/baseline_coverage.out) | Original race-run statement coverage profile; not readiness score |
| [V13_COVERAGE_SUMMARY.log](evidence/V13_COVERAGE_SUMMARY.log) | Per-function coverage and52.2% total |
| [V10_GOVULNCHECK.log](evidence/V10_GOVULNCHECK.log) | Raw JSON-mode tool output, including historical advisory records; do not count every OSV record as a finding |
| [govulncheck_stream.json](evidence/govulncheck_stream.json) | Machine-readable scan record stream used by text converter |
| [vulnerability_summary.json](evidence/vulnerability_summary.json) | 19 actual advisory IDs and symbol/package/module classification |
| [V15_VULN_TEXT_CONVERSION.log](evidence/V15_VULN_TEXT_CONVERSION.log) | Human-readable same-scan findings and call-path evidence |
| [V11_AUDIT_REGRESSION_PROBES.log](evidence/V11_AUDIT_REGRESSION_PROBES.log) | Twelve extra before-fix probe outcomes |
| [probes/](evidence/probes/) | Four .go.txt source files; copy only into a disposable checkout per report14 |
| [package_validation.json](evidence/package_validation.json) | Final source preservation, source-citation/ID/link/coverage consistency and report inventory validation |

## Evidence strength and limitations

| Evidence class | What it can support | What it cannot support |
| --- | --- | --- |
| Source trace | Active routes/call paths, SQL predicates, schema fields, config and missing artifacts in this archive. | Successful deployment, actual provider configuration, affected live row counts or unseen client behavior. |
| Local original tests | The exact memory/mock/handler examples executed under pinned Go. | Exhaustive workflow correctness, durable SQL, multi-node failover or external subject mapping. |
| Audit probes | The recorded counterexample on unchanged implementation under the specified Memory/HTTP/codec/mock boundary. | Unexecuted production impact or an undocumented product rule being pre-approved. |
| Vulnerability scan | Advisory presence, installed versions and conservative call traces on linux/amd64 as of recorded DB date. | Ten or nineteen proven app exploits; deployment-dependent paths require individual triage. |
| CI/manifests | Intended steps and desired platform baseline. | Passed CI checks, current image provenance or actual cluster health. |
| No supplied artifact | Absence within the archive. | Absence from the entire NVUTO platform or a new approved scope requirement. |

The Go vulnerability database timestamp recorded by govulncheck is2026-10-08T22:31:09Z. The scan uses govulncheckv1.7.0 and Go1.25.13 on linux/amd64. It contains19 advisory findings, of which10 have symbol traces; historical OSV records and repeated trace entries are not extra findings. Report10 provides contextual exploitability analysis and primary links. No active penetration test, exploit against a live service, malicious message or sensitive data extraction was conducted.

Verification blockers are missing Docker/YSQL/NATS infrastructure, provider/application repositories and staging credentials, production cluster/registry/identity/telemetry access, approved SLO/region/retention decisions and authoritative Git history. Report20 names the dependencies and owner roles. We did not substitute guessed credentials, Memory results, generic PostgreSQL or one-node CI for the unavailable production evidence.

## Retained before-fix baseline and future updates

Keep the ZIP digest, per-file manifest, original logs and audit probes together with the incoming engineer's implementation work. When a ticket is completed, add after-fix results with a new source commit, toolchain/image versions, environment and timestamp; do not edit the old logs to make them appear passed. Changes to policy/time/role semantics require the associated decision record and revised acceptance cases. The handover itself is an audit record, not a signed production approval.
