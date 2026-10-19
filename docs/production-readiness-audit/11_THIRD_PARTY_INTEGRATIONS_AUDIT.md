# 11 — Third-party and platform integrations audit

**Audit date:** 2026-10-19. **Conclusion:** the repository implements an HTTP consent service, a pgx persistence adapter and a NATS JetStream outbox publisher. Authentik, Customer Graph and eight NVUTO products are named architectural dependencies, but their production configuration and end-to-end integrations are not supplied. No real external service was contacted for this audit's application tests.

Source references are relative to the retained repository root. The identifiers **EXT-01 through EXT-12** below form the external-dependency inventory used by the [open decisions and external dependencies register](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md). **INT-01 through INT-08** identify the eight named consumer products within EXT-12; they are not eight additional infrastructure dependencies. Owners below are proposed role owners. No named accountable owner or service-level agreement was supplied.

Findings and tasks are canonical: consult the [risk register](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md), [implementation backlog](16_DETAILED_IMPLEMENTATION_BACKLOG.md), [verification report](14_TESTING_QA_AND_VERIFICATION_REPORT.md) and [go-live checklist](19_PRODUCTION_GO_LIVE_CHECKLIST.md) for final F/T/AT/G definitions.

## 1. Operational external-dependency register

| ID and dependency | Evidence / implemented interface | Current state and missing contract | Proposed owner; release references |
|---|---|---|---|
| **EXT-01 — Authentik / identity provider** | JWT RS256/JWKS verification; configured issuer/audience; tenant, actor, customer and role claims. `README.md:3-11,41`; `internal/auth/auth.go:20-32,138-164`; `deploy/k8s.yaml:50-64`. | Adapter implemented; actual provider/version/endpoints/claims, tenant governance, key rotation and role lifecycle **not verified**. Local generated-key tests are not provider integration evidence. | Identity platform owner, with security/backend review. D-06, D-10; F-014, F-015, F-019, F-030. |
| **EXT-02 — Customer Graph canonical subject mapping** | README requires canonical customer subject IDs; API takes `customer_subject_id` claim and request subject IDs. `README.md:41`; `internal/auth/auth.go:27-31`; `cmd/server/main.go:113-118`. | Contract reference only; no Customer Graph client, lookup, reconciliation or merge implementation in this repository. Mapping authority and subject merge/deletion effects **undecided/not supplied**. | Customer Graph owner and identity owner. D-07, D-10, D-12; F-002, F-023, F-030. |
| **EXT-03 — YugabyteDB YSQL** | pgx runtime persistence, three SQL migrations, YSQL CI startup helper. `internal/store/postgres.go:17-41`; `migrations/001_init.sql:1-45`; `migrations/002_policy_governance.sql:1-110`; `migrations/003_governed_idempotency.sql:1-11`; `README.md:45-51`. | Persistence code implemented; actual universe/version/isolation/placement/users/TLS/capacity **not verified**. All external DB tests skipped in the audit. CI is RF1 functional intent, not HA proof. | Database platform owner and backend lead. D-08, D-09; F-004, F-012, F-016, F-017, F-019, F-022, F-028. |
| **EXT-04 — NATS JetStream** | Synchronous publish, CloudEvents envelope, stable message ID, stream creation/check. `cmd/outbox-worker/main.go:82-147,185-203`. | Publisher implemented; actual cluster/version/account/ACL/TLS/stream/consumer configuration **not verified**. Existing stream config is not validated; outbox fencing is defective. | Messaging platform owner. D-08, D-09, D-13; F-009, F-010, F-019, F-020, F-021. |
| **EXT-05 — Trusted double-opt-in delivery owner** | Initiation creates a random challenge and returns a raw token; no transport adapter is implemented. `internal/service/service.go:256-302`; `internal/domain/model.go:127-130`. | **Design and implementation absent** for independent delivery, destination verification, resend/cancel and delivery recovery. Current self-service response bypasses that boundary. | Product-approved trusted delivery/integration owner with privacy and identity review. D-02, D-07; F-001, F-002, F-003. |
| **EXT-06 — Container registry / release artifacts** | Manifests reference GHCR API/worker tags; Dockerfiles build binaries; CI builds local test images. `deploy/k8s.yaml:41`; `deploy/outbox-worker.yaml:36`; `.github/workflows/ci.yml:27-30`. | Image publication, immutable release digests, promotion, signatures/provenance, pull authorization and prior release **not supplied**. Existence of tags was not tested. | Release engineering owner. D-09; F-011, F-018. |
| **EXT-07 — CI execution and required checks** | GitHub Actions workflow declares Go and YSQL jobs. `.github/workflows/ci.yml:1-122`. | Workflow implemented; no actual run for a source commit, branch protection or required-check policy supplied. Durable idempotency and latest upgrade coverage are missing from defined jobs. | Repository/CI owner. F-011, F-016, F-017, F-018. |
| **EXT-08 — Kubernetes platform and gateway/network boundary** | Namespace, API/worker Deployments, Service, disruption budgets and API HPA. `deploy/k8s.yaml:1-129`; `deploy/outbox-worker.yaml:1-83`. | Desired manifests only. Cluster, ingress/gateway, DNS, network policy, autoscaling capacity, TLS boundary and rollout history **not verified**. | Kubernetes/platform owner. D-08, D-09; F-015, F-018, F-019. |
| **EXT-09 — Observability / alerting / on-call** | Basic process/error logging exists. `cmd/server/main.go:424-455`; `cmd/outbox-worker/main.go:216-230`. | No metrics/traces/dashboards/alert routes, worker progress health, poison-event runbook or on-call ownership supplied. | SRE/operations owner with consumer operations. D-08, D-09; F-013, F-015, F-021. |
| **EXT-10 — Secrets and certificate lifecycle** | API references `consent-secrets` and `consent-runtime`; worker mounts `consent-nats-credentials`. `deploy/k8s.yaml:44-64`; `deploy/outbox-worker.yaml:38-56,69-72`. | Required objects/providers, secret generation/distribution/access/rotation and CA lifecycle **not supplied**. No secret values are needed in this handover. | Security platform/secrets owner. D-09, D-10; F-018, F-019. |
| **EXT-11 — Backup / PITR / restore / recovery** | README delegates production recovery to target YSQL; rollback note mentions backup. `README.md:51`; `migrations/ROLLBACK.md:3-5`. | Actual backup/PITR schedules, restore proof, RPO/RTO, immutable archive access, retention and reconciliation with broker history **not verified**. | Database/SRE recovery owner and privacy owner. D-05, D-08, D-09, D-14; F-022. |
| **EXT-12 — Approved launch consumer applications** | Eight product names in README; HTTP route surface and outbound publisher exist. `README.md:9-11`; `cmd/server/main.go:399-409`; `cmd/outbox-worker/main.go:102-113`. | Named consumers only; launch subset, actual clients/subscriptions/UI wiring and communication enforcement **not supplied**. Inventory is below. | Product integration lead plus each launch consumer's owner. D-01, D-07, D-08, D-13; F-010, F-023, F-024. |

None of these entries implies that infrastructure must be implemented inside this repository. An external team may own a fully adequate service. The release requirement is a concrete owner, approved interface/configuration and reproducible evidence that the exact application release works with it.

## 2. Consumer inventory and required journeys

`README.md:9-11` names these eight products. It does not include consumer source, deployed versions, authenticated client configuration, subscriptions or UAT results. The suggested journeys below are **acceptance planning**, not claims about implemented screens or current customer behavior. D-01/D-07 must identify the launch set; products outside that set can be explicitly deferred.

| Consumer ID | Product | Suggested launch contract / journey to validate if included | Implementation and owner state |
|---|---|---|---|
| **INT-01** | Engage | An attempted communication evaluates the correct canonical subject, purpose and channel; withdrawal/suppression/expiry and unavailable Consent prevent disallowed sends. | No client/UAT supplied; proposed Engage engineering/product owner. |
| **INT-02** | Service | An approved service workflow uses its intended lawful purpose/channel and role; it does not infer permission for another purpose from a generic grant. | No client/UAT supplied; proposed Service owner. |
| **INT-03** | Contact Center | An agent workflow records only authorized changes and checks the subject's effective restrictions before the approved contact action. | No agent UI/integration supplied; proposed Contact Center owner. |
| **INT-04** | Customer Portal | A self-service principal loads the correct customer's preferences, completes the approved confirmation journey and sees persisted changes after reload/restart. | No portal source or authoritative preference-read projection supplied; proposed Portal owner. |
| **INT-05** | CRM | Canonical subject mapping is consistent with Customer Graph and CRM does not overwrite a newer withdrawal with a delayed event or stale cached state. | No mapping/subscription supplied; proposed CRM owner. |
| **INT-06** | Conversations | The specific approved conversation action observes channel restrictions, later withdrawal and provider/API failure behavior. | No outbound adapter or UAT supplied; proposed Conversations owner. |
| **INT-07** | AI | The intended AI use of consent data and any communication/action are approved, purpose-bound and tenant-scoped; the model/product name alone grants no broad processing permission. | No AI data-flow or enforcement implementation supplied; proposed AI product/privacy owner. |
| **INT-08** | Integration Hub | Replay, duplicate handling, event ordering and tenant-scoped distribution preserve current permission across downstream integrations. | No integration routes, consumer code or reconciliation proof supplied; proposed Integration Hub owner. |

**AT-023 / G-023** requires actual staging evidence for every approved launch consumer. Each record should identify consumer/source versions, claim mapping, endpoint/event schema version, sanitized configuration, actions performed, durable state assertions and final user/communication outcome. Negative cases include no consent, denial, withdrawal, suppression, expiry, invalid/mismatched identity and unavailable Consent. A service binary build or isolated publisher test cannot establish these outcomes.

## 3. Identity and Customer Graph integration contract

The verified JWT is the principal source. Production configuration must settle the exact issuer/audience/JWKS URL, array-of-strings role mapping, tenant claim authority, actor identity and canonical customer subject mapping. A `self_service` identity must not choose another customer's claim. Whether privileged roles may coexist with self-service, whether `developer` is ever minted outside tests, and who may clear administrative suppressions remain D-06/D-10 decisions. Current behavior is documented in the [security report](10_SECURITY_PRIVACY_AND_ACCESS_CONTROL_AUDIT.md) and `cmd/server/main.go:109-118,241-258`.

Customer Graph is referenced as the canonical-ID owner but is not called by this service. An integration contract must state who resolves IDs before requests, how tenant membership is guaranteed, and what happens when subjects merge, split or are deleted. The repository alone cannot determine whether historical receipts move, remain linked to the original ID, or are resolved through an external alias history. This is a D-07/D-12 decision with privacy review; do not invent a migration or use an email address as a canonical ID without that agreement (`README.md:41`; `internal/domain/model.go:15-39`).

Provider acceptance must include actual staging tokens for all seven roles, wrong issuer/audience, missing/expired claims, unknown and rotated keys, revoked/changed role behavior under the approved token lifetime, and mixed-role cases. JWT library tests already prove some mechanics, while **F-014** requires bounded key refresh. No IdP administration, user provisioning or external token issuance was performed during this audit (`internal/auth/auth_test.go:26-131`).

## 4. HTTP consumer contract

The current mux exposes eleven business operations: three registry writes; governed consent capture; withdrawal; suppression change; single and batch evaluation; double-opt-in start and confirm; and receipt retrieval. It also exposes health/readiness. There is no OpenAPI document or SDK in the supplied archive. There is no subject preference list, registry list, consent-by-ID GET, suppression list or audit export route. This does not require all such endpoints to be added: an external projection can own reads if it is documented and tested (**F-024 / D-01 / D-07**; `cmd/server/main.go:399-409`).

The launch contract must specify:

- Canonical subject and scope mapping, role requirements and negative authorization behavior for each operation.
- Stable typed public errors and retry rules. Current unexpected infrastructure failures can be returned as 422 with internal text (**F-013**), so clients must not treat the current mapping as an approved long-term contract.
- Idempotency for governed capture: whether the guarantee is one resource creation or exact original-response replay, its tenant/actor namespace, retention, and behavior after withdrawal or policy change. The present ledger returns current consent state and creation validation happens before replay lookup (**F-025 / D-11**; `internal/service/idempotency.go:17-30,55-81`; `internal/store/idempotency.go:27-41,67-83`).
- Whether evaluation supports historical/future `at`, and how scope/jurisdiction affect the answer. Current latest-row semantics do not provide a reliable historical timeline (**F-006 / D-03 / D-12**).
- Browser origin/session/gateway behavior if a browser consumer calls the API. No CORS or browser-auth bridge is implemented in the route setup; an owned gateway or same-origin integration may supply it. The absent frontend is not evidence of a broken existing screen (**F-024**).
- Availability and propagation budgets approved under D-08, including how a consumer behaves when it cannot obtain a fresh decision. The audit supplies no measured production latency or acceptable stale-permission interval.

The trusted DOI delivery contract needs additional destination binding, token confidentiality, resend/cancel/expiry and partial-delivery recovery. Neither an email/SMS provider nor a webhook delivery adapter is present. **EXT-05** must select an approved owning component before adding a vendor SDK. The current token disclosure is a confirmed defect requiring **AT-003**, not evidence that any external provider failed (`internal/service/service.go:256-302`).

## 5. NATS and event integration

### 5.1 Implemented envelope and delivery behavior

The worker creates a CloudEvents 1.0 JSON envelope with event ID, source, type, aggregate subject, time, content type, schema version, tenant extension and payload. It also sends `Nats-Msg-Id` equal to the durable event ID and a tenant header. The database row is marked published only after synchronous `PublishMsg` returns successfully. Governed mutation paths append the outbox event inside the same database transaction as domain/audit/evidence writes. Sources: `cmd/outbox-worker/main.go:33-44,87-125`; `internal/store/postgres.go:238-286,300-320,344-383`.

Those are useful at-least-once delivery foundations. A broker publish acknowledgment is evidence of broker acceptance/persistence according to its configuration, not evidence that a consumer applied the event or prevented a communication. If the acknowledgment or later database completion is lost, replay can occur. Message-ID deduplication is finite: the created stream uses a ten-minute duplicate window, while retry delays can reach fifteen minutes and outages can last longer. Exactly-once end-to-end processing is not established. Primary sources checked 2026-10-19: [NATS publishing](https://docs.nats.io/learn/jetstream/publishing) and [delivery and acknowledgment](https://docs.nats.io/learn/jetstream/delivery-and-acknowledgment). Local evidence: `cmd/outbox-worker/main.go:108-145`.

The existing stream is accepted by name without validating configuration. New streams request file storage, LimitsPolicy retention, the configured subject wildcard, replica count and deduplication window, but no explicit maximum age/messages/bytes. Actual effective cluster/account limits are external and unknown. Stream ownership, retention/capacity/discard policy and drift detection belong to **EXT-04 / F-020**. Primary reference checked 2026-10-19: [NATS stream configuration and limits](https://docs.nats.io/learn/jetstream/shaping-the-stream).

### 5.2 Event inventory: eight families, nine concrete types

“Granted” and “denied” are the two concrete types in the capture family. This grouping matches the eight-family acceptance wording in **AT-010**. Subjects below use the default prefix `nvuto.consent`; the worker removes the leading `consent.` from event type and appends the suffix (`cmd/outbox-worker/main.go:82-85,159-161`).

| Family | Concrete event type → default NATS subject | Current payload fields | Source and integration gap |
|---|---|---|---|
| Purpose change | `consent.purpose.changed` → `nvuto.consent.purpose.changed` | `category`, `active` | `internal/service/service.go:59`. Aggregate ID is purpose code; payload omits allowed channels and affirmative/DOI/evidence requirements. |
| Lawful-basis change | `consent.lawful_basis.changed` → `nvuto.consent.lawful_basis.changed` | `active` | `internal/service/service.go:72`. Aggregate ID is basis code; payload omits complete rule/category information. |
| Notice publication | `consent.notice.published` → `nvuto.consent.notice.published` | `jurisdiction`, `active` | `internal/service/service.go:84`. Aggregate ID is notice version; no notice artifact/digest in the notice model. |
| Governed capture | `consent.granted` / `consent.denied` → `nvuto.consent.granted` / `nvuto.consent.denied` | `subject_id`, `purpose`, `channel`, `receipt_id`, `notice_version` | `internal/service/service.go:148`; `internal/service/idempotency.go:80`. No preference revision, expiry, full scope or complete policy snapshot. |
| Withdrawal | `consent.withdrawn` → `nvuto.consent.withdrawn` | `subject_id`, `purpose`, `channel` | `internal/service/service.go:198`. No current preference revision or reconciliation reference beyond consent aggregate ID. |
| Suppression change | `consent.suppression.changed` → `nvuto.consent.suppression.changed` | `subject_id`, `scope`, `purpose`, `channel`, `active` | `internal/service/service.go:233`. No suppression state version or authority/provenance class in event. |
| Pending confirmation | `consent.pending_confirmation` → `nvuto.consent.pending_confirmation` | `subject_id`, `purpose`, `channel`, `notice_version` | `internal/service/service.go:297`. It contains no raw token or destination; it is not an implemented delivery command. |
| Confirmation | `consent.confirmed` → `nvuto.consent.confirmed` | `subject_id`, `purpose`, `channel`, `receipt_id` | `internal/service/service.go:324`. No supersession/preference generation or effective expiry. |

Durable normalization defaults `schema_version` to 1, `source` to `nvuto/consent`, and `aggregate_type` to `consent` even for registries and suppressions. The CloudEvents subject therefore does not accurately type those resources. A schema version describes event format, not the latest business-state revision. Source: `internal/store/postgres.go:106-115`; `internal/domain/model.go:141-151`; `cmd/outbox-worker/main.go:87-100`.

An older internal `Record` helper emits capture events with a different payload including `legal_basis` and no governed receipt. It is not the current HTTP capture route, but represents compatibility/event-shape risk if reused; **F-031** requires inventory and restriction/removal of that path (`internal/service/service.go:152-178`; `cmd/server/main.go:180-209`). Consumers should not infer that identical event type strings already guarantee one complete stable schema across all internal paths.

### 5.3 Ordering, replay and reconciliation acceptance

Two workers can publish separate batches concurrently. A failed earlier event moves to a later retry while a newer event can publish first. Sorting claims by occurrence time does not enforce end-to-end per-preference order. The outbox also lacks a claim-generation fence (**F-009**). A consumer that treats the most recently received event as permission can therefore process withdrawal followed by an older grant and re-enable communication. No actual consumer implementation was supplied, so that harmful consumer behavior is a reproduction condition, not an observed downstream incident. Sources: `cmd/outbox-worker/main.go:46-79,116-134,215-231`; `deploy/outbox-worker.yaml:7`.

**T-010 / AT-010** should establish one approved reconciliation contract under D-13:

1. Specify the actual aggregate/projection key, including tenant and canonical subject where relevant, and assign a transactional monotonic revision that spans superseding consent rows.
2. Publish versioned schemas/examples for all eight families and correct aggregate resource types. Include enough state or an authorized read/evaluation reference for reconstruction.
3. Require consumers to handle duplicate IDs and out-of-order revisions, including replay beyond the ten-minute broker window. A delayed grant must not override a later withdrawal.
4. Define expiry and policy-change handling. Expiry is currently evaluated on reads; no expiry worker or expiry event exists. A cached grant cannot remain valid indefinitely because no event arrived (`internal/service/service.go:342-387`).
5. Decide when a consumer must re-evaluate immediately before a communication, and demonstrate safe behavior during API/broker outages within D-08's approved budgets.
6. Test consumer restart, projection rebuild, poison payload, event schema evolution, lost publish acknowledgment and database completion failure with reproducible state assertions.

These requirements complement broker durability. They do not assume a particular NATS consumer mode or invent consumer configuration that is not present.

## 6. Complete application dependency pins

The module is `github.com/Atlasfsp/Nvuto-Consent`, with `go 1.25.13`. There are **three direct and ten indirect `go.mod` requirements**. Module download and checksum verification succeeded in the isolated audit. That checks availability/integrity against Go's module mechanisms; it does not prove license suitability, continued support, absence of vulnerabilities, or successful production interoperability. Sources: `go.mod:1-22`; `go.sum:1-40`; [verification report](14_TESTING_QA_AND_VERIFICATION_REPORT.md).

| Module | Exact pin and declaration | Runtime responsibility / state | Proposed maintenance owner |
|---|---|---|---|
| `github.com/golang-jwt/jwt/v5` | v5.2.2; direct, `go.mod:6` | JWT parsing/verification; local auth tests execute. No current advisory attributed to this module by the recorded scan; not a general security certification. | Backend identity maintainer. |
| `github.com/jackc/pgx/v5` | v5.9.2; direct, `go.mod:7` | YSQL PostgreSQL-protocol driver/pool; codec exercised, external DB integration not executed. | Backend/database maintainer. |
| `github.com/nats-io/nats.go` | v1.47.0; direct, `go.mod:8` | JetStream client and credentials/TLS integration; envelope unit test executes, external broker test skipped. | Messaging/backend maintainer. |
| `github.com/jackc/pgpassfile` | v1.0.0; indirect, `go.mod:12` | pgx password-file support in dependency graph. No application-specific direct use claimed. | Backend/database maintainer. |
| `github.com/jackc/pgservicefile` | v0.0.0-20240606120523-5a60cdf6a761; indirect, `go.mod:13` | pgx service-file configuration support. No deployment use supplied. | Backend/database maintainer. |
| `github.com/jackc/puddle/v2` | v2.2.2; indirect, `go.mod:14` | Resource pooling in pgx dependency graph. | Backend/database maintainer. |
| `github.com/klauspost/compress` | v1.18.0; indirect, `go.mod:15` | Compression dependency in NATS graph; GO-2026-5841 package/module finding, fixed v1.18.7; called vulnerable S2 path not established. | Messaging maintainer with security review. |
| `github.com/nats-io/nkeys` | v0.4.11; indirect, `go.mod:16` | NATS key/credential support. Actual credentials and account policy not supplied. | Messaging/security maintainer. |
| `github.com/nats-io/nuid` | v1.0.1; indirect, `go.mod:17` | NATS unique-ID support. | Messaging maintainer. |
| `golang.org/x/crypto` | v0.52.0; indirect, `go.mod:18` | Cryptographic support in dependency graph; four package/module advisories for unused OpenPGP/SSH contexts. SSH fixes indicated by v0.55.0/v0.56.0; OpenPGP advisory has no fixed version. | Backend/security maintainer. |
| `golang.org/x/sync` | v0.21.0; indirect, `go.mod:19` | Concurrency helpers in dependency graph; it does not mean application JWKS refresh is coalesced. | Backend maintainer. |
| `golang.org/x/sys` | v0.45.0; indirect, `go.mod:20` | Operating-system support in dependency graph. | Backend/release maintainer. |
| `golang.org/x/text` | v0.39.0; indirect, `go.mod:21` | Text/PRECIS handling, including pgx SCRAM; GO-2026-6629 symbol finding, fixed v0.41.0; triggering Nickname profile not established in this app. | Backend/database maintainer with security review. |

All maintenance ownership assignments are proposals, not evidence of an existing team agreement. The [security report](10_SECURITY_PRIVACY_AND_ACCESS_CONTROL_AUDIT.md) contains the full **19-advisory table**, contextual reachability and **F-011** remediation. Go 1.25.13 must be replaced consistently with a maintained patched line; as checked on 2026-10-19 the official fixes are Go 1.26.9 or 1.27.2. Re-resolve and scan the compatible module graph, rather than assuming that editing one indirect requirement closes every finding. Primary source: [Go release history](https://go.dev/doc/devel/release).

`go.sum` also retains checksum entries for `go-spew` v1.1.1 (and v1.1.0 go.mod), `go-difflib` v1.0.0, `objx` v0.1.0 go.mod, `testify` v1.11.1 (plus older v1.3.0/v1.7.0 go.mod entries), `check.v1` v0.0.0-20161208181325-20d25e280405 go.mod, and `yaml.v3` v3.0.1 (plus v3.0.0-20200313102051-9f266ea9e77c go.mod). These are checksum/test-graph records, not additional direct application requirements or proof that each package is linked into the production binaries (`go.sum:1-3,22-28,37-40`). A final binary/image SBOM should establish shipped components.

## 7. Build, platform and supply-chain dependencies

| Artifact/tool | Exact supplied reference | State / gap and proposed owner |
|---|---|---|
| Go language/toolchain | `go 1.25.13`; CI `go-version: 1.25.13` | Exact version exists and was audited; unsupported line on audit date and current vulnerability findings. Backend/release owner; F-011. |
| API and worker builder | `golang:1.25.13-alpine` | Mutable tag, not digest. Both Dockerfiles align with current Go pin; both need coordinated update. Release owner. `Dockerfile:1`; `Dockerfile.worker:1`. |
| API and worker runtime | `gcr.io/distroless/static-debian12:nonroot` | Nonroot runtime is a positive control; tag digest and image vulnerability/provenance evidence not supplied. Release/security owner. Both Dockerfiles:8-10. |
| API deployment image | `ghcr.io/atlasfsp/nvuto-consent:latest` | Mutable release tag; no supplied publication/promotion evidence. EXT-06. `deploy/k8s.yaml:41`. |
| Worker deployment image | `ghcr.io/atlasfsp/nvuto-consent:latest-worker` | Mutable tag with different default pull semantics from literal `latest`; no exact digest. EXT-06. `deploy/outbox-worker.yaml:36`. |
| Yugabyte CI image | `yugabytedb/yugabyte:2026.1.0.1-b1` | CI default, not a verified production universe version. EXT-03/EXT-07. `scripts/ci/start-yugabytedb.sh:6`. |
| NATS CI image | `nats:2.10-alpine` | Single-server JetStream CI fixture; mutable minor tag, not production cluster evidence. EXT-04/EXT-07. `.github/workflows/ci.yml:67`. |
| Vulnerability scanner | `golang.org/x/vuln/cmd/govulncheck@v1.7.0` | Version-pinned scanner in CI; source database is time-varying. Record DB date and actual exit semantics. EXT-07/security owner. `.github/workflows/ci.yml:25`. |
| Checkout action | `actions/checkout@v4` | Major tag, not immutable action commit. CI owner; approved supply-chain policy required. `.github/workflows/ci.yml:15,43`. |
| Go setup action | `actions/setup-go@v5` | Major tag, not immutable action commit. CI owner. `.github/workflows/ci.yml:16,44`. |
| Runner | `ubuntu-latest` | Moving hosted runner image; exact release environment not preserved in source. CI owner. `.github/workflows/ci.yml:13,33`. |
| CI command dependencies | Bash, Docker, `ysqlsh`, `pg_isready`, curl, netcat, Python 3 and Go tools | Used by scripts/workflow; no independent versions pinned beyond image/runner context. Document actual runner/image versions in evidence. EXT-07. `scripts/ci/start-yugabytedb.sh:1-37`; `.github/workflows/ci.yml:50-122`. |

Kubernetes defaults literal `:latest` to `Always` pull policy and other tags to `IfNotPresent` when the policy is omitted. Thus `latest-worker` is not equivalent to `latest`; mixed cached worker revisions are a realistic deployment risk. Use immutable digest references and an owned promotion process (**F-018**). Primary source checked 2026-10-19: [Kubernetes container images and pull policy](https://kubernetes.io/docs/concepts/containers/images/).

No current container scan, artifact signature, SBOM, dependency license review, registry access policy, branch protection evidence or source commit history was included. These are evidence/ownership gaps; no registry compromise or malicious dependency was observed. The supplied archive contains application source and definitions, not a complete software-supply-chain attestation.

## 8. Integration completion criteria

The release owner should obtain one reviewable evidence packet per approved dependency/consumer, using the shared canonical gates instead of creating parallel task IDs:

| Completion outcome | Canonical references | Evidence to attach |
|---|---|---|
| Actual identity/customer mapping and roles agree with backend enforcement. | T-002 / AT-002 / G-002; T-014 / AT-014 / G-014; T-030 / AT-030 / G-030 | Sanitized provider settings, claim examples, role/subject/tenant negatives and rotation/outage results. |
| Independent DOI delivery is real and confidential. | T-003 / AT-003 / G-003 | Approved EXT-05 contract, sandbox destination journey, resend/cancel/expiry and failure recovery. |
| Durable writes and broker delivery survive failure under the approved topology. | T-009 / AT-009 / G-009; T-016 / AT-016 / G-016; T-020 / AT-020 / G-020 | YSQL/NATS versions/config, atomicity/fencing/failover tests, stream validation and capacity behavior. |
| Consumers converge safely under duplicates, reordering and expiry. | T-010 / AT-010 / G-010; T-023 / AT-023 / G-023; T-024 / AT-024 / G-024 | Versioned event/HTTP contracts, authorized read owner, actual launch-product UAT, restart/replay/pre-send checks. |
| Release artifacts and operational dependencies are reproducible. | T-011 / AT-011 / G-011; T-018 / AT-018 / G-018; T-019 / AT-019 / G-019; T-022 / AT-022 / G-022 | Patched image digests/scans, deployment inputs, TLS/permission results, recovery drill and accountable owners. |

Use [report 12](12_DEVOPS_INFRASTRUCTURE_AND_DEPLOYMENT_AUDIT.md) for all configuration variables and operational mechanics, [report 14](14_TESTING_QA_AND_VERIFICATION_REPORT.md) for executed evidence, [report 15](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md) for risk disposition, [report 16](16_DETAILED_IMPLEMENTATION_BACKLOG.md) for implementation acceptance, [report 19](19_PRODUCTION_GO_LIVE_CHECKLIST.md) for release gates, and [report 20](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md) for the approvals and owners needed to turn these external dependencies into verified integrations.
