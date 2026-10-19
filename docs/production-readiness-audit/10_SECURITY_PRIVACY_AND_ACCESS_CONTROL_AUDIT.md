# 10 — Security, privacy and access control audit

**Audit date:** 2026-10-19. **Disposition:** production approval is blocked by confirmed consent-state, authorization, evidence-integrity and dependency findings, together with required infrastructure and identity evidence that has not been supplied.

Source references are relative to the retained repository root and refer to the supplied source, not a proposed implementation. This was a source and isolated-test audit; no production credentials, identity provider, customer data, database, broker or cluster were accessed. The complete finding definitions and severity remain in the [risk register](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md); implementation and acceptance IDs remain in the [backlog](16_DETAILED_IMPLEMENTATION_BACKLOG.md). An `F`, `T`, `AT` and `G` with the same numeric suffix refer to the same finding, task, acceptance test and release gate.

## 1. Security conclusion and strength of evidence

The service has a useful security foundation: production JWT verification is explicit, development authentication requires opt-in, most HTTP operations apply tenant and customer boundaries, governed writes use transactions, and receipt/notice/audit tables have mutation-blocking triggers. These controls do not compensate for the confirmed double-opt-in defects. An old challenge can restore permission after a later denial or withdrawal (**F-001, P0**); confirmation omits self-service customer authorization (**F-002, P1**); and self-service initiation discloses the confirmation capability directly (**F-003, P1**). The receipt's stored timestamp can also invalidate its hash (**F-004, P1**). Sources: `internal/auth/auth.go:138-188`; `cmd/server/main.go:109-118,319-362`; `internal/service/service.go:108-117,256-325`; `internal/store/postgres.go:238-286,340-383`; `migrations/002_policy_governance.sql:91-110`.

The audit distinguishes four evidence levels:

| Level | What this audit establishes | Important limit |
|---|---|---|
| Executed counterexample | Local synthetic tests reproduced stale-challenge restoration, cross-subject confirmation, direct-token DOI bypass, timestamp codec hash mismatch, unknown-key JWKS refresh amplification and other cases documented in report 14. | Memory-backed HTTP behavior and a real pgx codec test are not a live YSQL test. |
| Source-confirmed defect | SQL/control flow demonstrates unfenced outbox completion, incomplete readiness, raw internal errors and other implementation gaps. | No production incident, affected customer count or successful external attack is inferred. |
| Executed dependency scan | Exact Go 1.25.13 and govulncheck v1.7.0 found 19 current advisories, ten with symbol traces. | A symbol trace does not prove that the vulnerable runtime configuration or attacker-controlled input is present. |
| Not verified / decision required | Production TLS, IdP claims, broker ACLs, database roles, consumer behavior, retention and restore evidence are absent. | Absence from the archive is not proof that the organization has no such controls elsewhere. |

The existing suite produced **15 PASS / 3 SKIP**, including the same totals under race detection. The three skipped tests require external database/broker services. Twelve additional audit probes produced eleven failing assertions that demonstrate defects and one passing behavior observation. See the [verification report](14_TESTING_QA_AND_VERIFICATION_REPORT.md) for commands, artifacts and exact limitations. These results support a remediation plan, not a production security certification.

## 2. Identity, tenant and customer trust model

### 2.1 Authentication modes

| Mode | Implemented requirements | Security interpretation |
|---|---|---|
| Configured JWKS | `JWT_JWKS_URL`, nonempty `JWT_ISSUER` and `JWT_AUDIENCE`; RS256 only; nonempty `kid`; required expiration; validated issuer/audience; nonempty `tenant_id` and JWT `sub`. | Intended production path. The API trusts the configured issuer to mint accurate tenant, actor, customer and role claims. See `internal/auth/auth.go:131-164`. |
| HMAC | Both a nonempty `JWT_HMAC_SECRET` and `ALLOW_HMAC_AUTH=true`; HS256 only; required expiration and tenant/actor. | Explicit alternate mode. It does not enforce the JWKS branch's issuer/audience requirements. Production use requires an explicit identity decision; it is not enabled in the supplied API manifest. See `internal/auth/auth.go:166-179`; `deploy/k8s.yaml:44-64`. |
| Development headers | `ALLOW_DEV_HEADERS=true`; caller supplies `X-Nvuto-Tenant-ID` and `X-Nvuto-Actor-ID`; actor becomes customer subject and role becomes `developer`. | Deliberately trusted local/test mode, not an unconditional authentication bypass. There is no stage guard in the process that prevents an operator enabling it in production. See `internal/auth/auth.go:180-188`. |
| No usable mode | Returns an authentication configuration error. | Fails closed on business routes. Readiness does not presently detect this condition. See `internal/auth/auth.go:188`; `cmd/server/main.go:392-398`. |

Configured JWKS takes precedence: a failed JWKS token does not fall through to HMAC or headers. Bearer parsing is case-insensitive for the scheme, and role comparison is exact and case-sensitive. Tokens are not minted by this service. There are no signup, password recovery, session issuance or OAuth provider endpoints in the route set. Those remain external identity responsibilities under **EXT-01 / D-10**, consistent with `README.md:3-11,41` and `cmd/server/main.go:399-409`.

The code verifies a token's claims; it does not query tenant membership, retrieve canonical Customer Graph identity or independently establish that `customer_subject_id` belongs to the actor. That mapping must be protected in the identity/Customer Graph contract. A maliciously configured trusted issuer can authorize arbitrary tenants or roles; this is an administrative trust boundary, not a token-signature bypass. Sources: `internal/auth/auth.go:20-32,131-164`; `README.md:41`.

### 2.2 Existing role matrix

This matrix describes current behavior for each role individually. Mixed-role composition introduces the **F-030, P1** write-authorization defect described below; the full production permission policy also requires **D-06 / D-10** approval. “Tenant-wide” means the authenticated tenant; it does not grant cross-tenant SQL access.

| JWT role | Purpose / lawful-basis / notice writes | Consent, withdrawal, suppression and DOI writes | Evaluate / batch / receipt reads | Subject restriction currently applied |
|---|---|---|---|---|
| `privacy_admin` | Yes | Yes | Yes | Tenant-wide |
| `consent_admin` | Yes | Yes | Yes | Tenant-wide |
| `developer` | Yes | Yes | Yes | Tenant-wide, including when supplied in a valid production JWT |
| `consent_writer` | No | Yes | Yes | Tenant-wide |
| `consent_reader` | No | No | Yes | Tenant-wide |
| `service` | No | No | Yes | Tenant-wide; this role does not authorize writes |
| `self_service` | No | Yes | Yes | Own `customer_subject_id` on checked operations; DOI confirmation is the exception in F-002 |

Source: role groups and `subjectAuthorized`, `cmd/server/main.go:109-118`; individual handler checks, `cmd/server/main.go:120-384`. Missing `customer_subject_id` is not globally rejected during authentication, but checked subject operations reject it when the principal is restricted to `self_service`. Receipt and withdrawal handlers first resolve the object within the authenticated tenant and then apply the customer boundary (`cmd/server/main.go:211-239,365-384`).

**F-030 — Mixed self-service and reader roles can authorize tenant-wide writes (P1).** A valid token containing `self_service` plus either `service` or `consent_reader` passes writer admission through `self_service`, then bypasses the customer restriction because `subjectAuthorized` treats the read role as tenant-wide. Neither role alone permits writing another customer's records, but the combination can authorize those writes within the tenant. This is a source-confirmed authorization-composition defect at `cmd/server/main.go:109-118`, with affected handler use at `cmd/server/main.go:180-209,211-239,241-263,319-341`. No additional dynamic probe was run, and actual production issuance of these claim combinations is unverified.

Bind an operation's permitted customer scope to the authority that grants that operation, and test both mixed-role combinations against own-subject and other-subject writes under **T-030 / AT-030**. **G-030 is release-blocking** until the defect is corrected or concrete, enforced issuer/deployment configuration proves that the unsafe combinations cannot occur. D-06/D-10 must still approve the complete role matrix, mixed-role policy, development-role disposition and suppression authority; a policy decision alone is not evidence that the current combination is excluded.

A self-service caller can currently clear their own suppression with `active=false`. There is no provenance or authority class that distinguishes a personal preference from an administrative or legal block. Whether this is permitted is a product/privacy decision; the source-confirmed fact is the broad permission and absent distinction, not a proven violation of an approved policy. Required acceptance is an approved seven-role matrix, tests for mixed roles and missing claims, and protected suppression behavior if the policy requires that class (**AT-030**).

### 2.3 Tenant isolation strengths and limits

Durable reads and writes use tenant parameters, and API-created objects obtain their tenant and actor from the authenticated principal. This is a useful application-layer isolation control. The SQL uses bound parameters for runtime values; this review did not identify a demonstrated runtime SQL-injection path. Sources: `cmd/server/main.go:120-384`; `internal/store/postgres.go:125-160,238-383`.

Database relationships are not uniformly tenant-constrained: the receipt foreign key includes `consent_id` but not tenant; the idempotency ledger and several registry references lack database foreign keys. **F-028** records this defense-in-depth gap without claiming a cross-tenant SQL exploit. The opt-in memory implementation has a separate reproduced composite-key collision across delimiter-rich tenant/ID values (**F-026**). Production rejects memory mode by default; this issue still matters to development/test fidelity and any explicitly enabled deployment. Sources: `migrations/002_policy_governance.sql:57-73`; `migrations/003_governed_idempotency.sql:1-11`; `internal/store/memory.go:60-74,145-168,218-235`; `cmd/server/main.go:425-430`.

Broker isolation is a different boundary. The worker publishes every tenant into a common event-type subject namespace and includes tenant metadata in headers and the envelope. Metadata does not enforce subscription authorization. Consumer-account and tenant-isolation design is an **F-019 / EXT-04 / EXT-12** gate; an actual cross-tenant broker exposure was not tested (`cmd/outbox-worker/main.go:82-113`; `deploy/outbox-worker.yaml:43-50`).

## 3. Abuse paths that require remediation

### 3.1 Confirmation authorization and independent delivery

The confirmation route accepts a writer principal and the submitted token, then calls the service with tenant, actor and token. It never calls `subjectAuthorized`. A synthetic HTTP test used customer B's self-service identity to confirm customer A's valid challenge in the same tenant and received A's granted consent. The precondition is possession of a valid victim token; the audit does not claim token guessing or cross-tenant confirmation. Enforce object authorization on the pending record within the atomic confirmation path, without leaking the resolved consent or receipt on rejection (**F-002 / T-002 / AT-002**; `cmd/server/main.go:343-362`; `internal/store/postgres.go:340-383`).

Initiation also returns `confirmation_token` in its public response. The self-service caller can immediately echo it, producing an allowed decision without an independent delivery action. The random token is hashed for storage, which is useful protection against direct recovery from the database, but returning the secret to the initiating untrusted caller defeats the intended evidence boundary. The challenge identifies a channel label; the model does not bind a verified contact destination or destination version. Select the trusted delivery owner (**EXT-05 / D-02**), return an opaque reference to self-service, and prove a complete sandbox delivery/confirmation journey (**F-003 / AT-003**). Sources: `internal/domain/model.go:127-130`; `internal/service/service.go:271-302`; `cmd/server/main.go:319-340`.

Old pending challenges also remain valid after a later preference change. Confirmation makes their row current under latest-`updated_at` evaluation and can override a denial or withdrawal. This is the P0 state defect **F-001**, independently of token entropy or token delivery. Bind challenges to a transactionally controlled preference generation and invalidate superseded challenges atomically. Acceptance must cover both memory and YSQL, concurrent confirm/deny/withdraw, and no partial receipt/audit/outbox write (**AT-001**). Source: `internal/service/service.go:256-325`; `internal/store/postgres.go:142-143,340-383`.

### 3.2 JWKS availability and key lifecycle

JWKS fetching has a five-second HTTP timeout, a one-MiB reader, RSA signing-key selection and a five-minute positive cache. Those controls bound one fetch, not the number of fetches. Every unknown key ID triggers a fresh request, even with a warm cache, and concurrent requests are not coalesced. Five synthetic unknown key IDs caused five local JWKS requests in the executed probe. No signing authority is needed to trigger this path. An exposed API can amplify unauthenticated traffic against itself and the configured IdP (**F-014**; `internal/auth/auth.go:41-117,151-163`).

The correction needs a per-issuer refresh coordinator, bounded global miss cooldown/negative cache, failure backoff and explicit rotation behavior. A per-key cache alone is insufficient because an attacker can vary the key ID. Known valid cached keys should continue working under the approved outage policy, and unknown keys must continue failing closed. The acceptance cases must bound outbound requests under concurrent unique misses and prove legitimate new-key adoption and retired-key handling within the approved delay (**AT-014 / D-08 / D-10**).

The three existing auth tests cover fail-closed/dev mode, HMAC success/expiry/algorithm/fallback, and local RS256 success plus wrong issuer. They do not establish actual Authentik claim mapping, key rotation, wrong audience, missing expiry, missing tenant/customer claims, concurrent refresh or provider outage behavior. Library enforcement may already reject some negative cases; missing tests are not evidence of universal acceptance. Source: `internal/auth/auth_test.go:26-131`.

### 3.3 Input, error and resource boundaries

JSON bodies are capped at one MiB and unknown fields are rejected, but one successful decoder call permits a trailing second JSON value; the audit reproduced that behavior. Semantic bounds for evidence hashes, expiry relationships, challenge lifetime and identifiers are incomplete (**F-027**). Unexpected database errors become HTTP 422 with raw `err.Error()`, which can expose SQL/schema detail and mislead clients about retryability (**F-013**). Sources: `cmd/server/main.go:59-85`; `internal/service/service.go:79-103,131-139,202-234,264-265`.

HTTP timeouts do not cancel database work whose repository methods use `context.Background`. Authentication and broker/DB outages can therefore interact with fixed connection pools and missing rate limits. Thread request context and bounded transaction budgets through the implementation; emit sanitized operational errors and correlation IDs; preserve safe denial while distinguishing unavailable policy from malformed input (**F-012, F-013, F-014**). Sources: `internal/store/postgres.go:125-143,238-286`; `cmd/server/main.go:435`.

## 4. Privacy data and evidence integrity

| Data class in source | Storage or transfer | Required decision/control |
|---|---|---|
| Tenant, actor and canonical customer identifiers | JWT claims; consent/audit tables; event metadata/payloads | Trusted claim mapping, role-scoped access, log/metric cardinality limits and consumer minimization. |
| Purpose, lawful basis, channel, decision, suppression reason and time | Consent, registry and suppression tables; selected values in events | Treat as privacy-relevant records; classification and retention require D-12/D-14 approval. A particular legal classification is not inferred from field names. |
| Notice version, jurisdiction and evidence method/hash/time | Notices and receipts; consent evidence stored but omitted from ordinary consent JSON | Preserve exact notice/evidence provenance. A supplied hash does not establish that an artifact exists or is trustworthy. |
| Confirmation token | Raw initiation response; hashed pending state | Restrict the raw capability to trusted delivery, define expiry/resend/cancel, and keep it out of logs/events. |
| Database/NATS credentials and certificates | Environment references and mounted files | Secret ownership, least privilege, rotation and verified transport; no real production values were supplied. |

Sources: `internal/auth/auth.go:20-32`; `internal/domain/model.go:15-151`; `deploy/k8s.yaml:44-64`; `deploy/outbox-worker.yaml:38-56,69-72`.

The receipt hash is not presently stable across storage. The actual pgx timestamptz codec changes nanosecond precision to microseconds; the executed audit case changed `.123456789` to `.123456`, and re-hashing the decoded receipt failed. Define versioned canonical bytes and normalized time precision, persist or recover those exact bytes, and test a real YSQL round trip and export/import. Preserve historical records and annotate any remediation rather than rewriting immutable evidence (**F-004 / AT-004**; `internal/service/service.go:108-117`; `internal/store/postgres.go:119-122,289-297`).

Notice rows hold identifiers and metadata, not the accepted notice content or an immutable content digest. Receipts lack a complete decision/legal-basis/policy-version/actor binding. Audit rows retain actor/action/resource-like ID/time but not complete policy or suppression before/after history. Mutation-blocking triggers cannot reconstruct information never recorded. **F-005 and F-029** require an agreed evidence contract, typed resource/revision history, authorized export, and privacy-owner review (**D-05 / D-14**). Sources: `internal/domain/model.go:71-97,132-139`; `internal/store/postgres.go:154-160,307-315`; `migrations/002_policy_governance.sql:48-73,91-110`.

The row triggers block ordinary updates/deletes to receipt, notice and audit rows. They are not cryptographic attestation, protection against a database owner changing DDL, or a retention program. Backup/PITR, restore validation, at-rest encryption, archive access, lawful retention/deletion and incident responsibilities are **not verified**. No jurisdictional compliance conclusion follows from `NG` test values. See **F-022, F-032**, the [operations report](12_DEVOPS_INFRASTRUCTURE_AND_DEPLOYMENT_AUDIT.md), and **D-08 / D-12 / D-14** in the [decision register](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md).

## 5. Transport, secrets and infrastructure identity

| Connection | Source behavior | Required production evidence |
|---|---|---|
| Client/gateway → API | HTTP `ListenAndServe` on `:8080`; ClusterIP Service 80→8080; no gateway/Ingress in supplied files. | Named TLS termination and internal-hop policy, supported methods/origins, authentication forwarding, body/header/rate limits. |
| API → JWKS | Configurable URL with ordinary HTTP client and redirects; no HTTPS-only validation. | Approved HTTPS issuer/key endpoint and redirect policy, certificate validation and rotation/outage tests. |
| API/worker → YSQL | DSN accepted as provided; no application TLS enforcement or separate role provisioning. | Verified certificate/hostname behavior and separate migration, API and worker database permissions. |
| Worker → NATS | Credentials required by default; optional CA file; supplied URL uses `nats://`. | Explicit TLS and account/subject permissions, controlled stream administration and certificate rotation. |
| Broker → consumers | Tenant extension/header in a common stream namespace. | Authorized consumer accounts, tenant isolation or explicitly approved platform-wide access, safe replay and delivery policy. |

Sources: `cmd/server/main.go:435-439`; `internal/auth/auth.go:41-47`; `internal/store/postgres.go:17-30`; `cmd/outbox-worker/main.go:185-194`; `deploy/k8s.yaml:88-98`; `deploy/outbox-worker.yaml:38-56`.

**F-019 is a source-enforcement/evidence gap, not proof of plaintext production traffic.** The pinned NATS client can upgrade a `nats://` connection when the server requires TLS; providing `RootCAs` also enables secure mode. A credentials file authenticates the connection but does not independently prove encryption. No server configuration or live handshake was inspected. Production acceptance needs wrong-CA/hostname/downgrade failures on each relevant boundary, and permission tests that deny unnecessary DDL, delivery-ledger mutations or cross-tenant subscriptions. Primary reference, checked 2026-10-19: [NATS encryption and TLS](https://docs.nats.io/learn/security/encryption). Pinned client evidence: `github.com/nats-io/nats.go@v1.47.0/nats.go:965-991,2211-2253,2614-2637` in the isolated module cache.

Pods already request nonroot execution, seccomp, no privilege escalation, dropped capabilities and a read-only root filesystem. Retain these controls. The referenced Secrets/ConfigMap, secret lifecycle, workload identities, network policies and image provenance remain external or absent from the supplied definitions (**F-018, F-019**; `deploy/k8s.yaml:1-6,28-31,44-64,82-86`; `deploy/outbox-worker.yaml:21-24,38-56,64-72`).

## 6. Dependency security: all 19 current advisories

### 6.1 Method and support status

The scan used **Go 1.25.13**, **govulncheck v1.7.0**, source/symbol analysis, and the official vulnerability database last modified **2026-10-08T22:31:09Z**. Its result contains **19 unique current advisories: ten with symbol traces and nine at package/module level only**. Historical advisory records carried in the JSON stream are not additional current findings. The JSON command exited 0 because it emitted machine-readable results; text conversion of those results reported `exit status 3`, with the `go run` wrapper exiting 1. The normal CI text gate therefore cannot be called green from the JSON process status. Evidence and conversion logs are catalogued in [report 14](14_TESTING_QA_AND_VERIFICATION_REPORT.md).

Go 1.25.13 exists and was used successfully for isolated checks. It is nevertheless outside the two-current-major-release support window on the audit date: the official release history lists Go 1.27.0 on 2026-08-19 and patched releases Go **1.26.9** and **1.27.2** on 2026-10-08. Update `go.mod`, CI setup and both Docker builder stages together under **F-011 / T-011**. Primary source checked 2026-10-19: [Go release history and support policy](https://go.dev/doc/devel/release). Pins: `go.mod:3`; `.github/workflows/ci.yml:18,46`; `Dockerfile:1`; `Dockerfile.worker:1`.

The following table contextualizes every finding. “Symbol” is the scanner's evidence level, not a declaration that an exploit was executed. The API currently runs a plaintext HTTP/1 server, with no `ListenAndServeTLS`, h2c server or HTTP/2 server setup. In contrast, an HTTPS JWKS request through Go's default transport can use HTTP/2. The distinction changes which advisory preconditions may be present (`cmd/server/main.go:435-439`; `internal/auth/auth.go:41-47`). All linked advisories are primary Go vulnerability database pages, assessed 2026-10-19.

| Advisory | Scan level; affected pin | Fixed version indicated | Application context and disposition |
|---|---|---|---|
| [GO-2026-6603](https://pkg.go.dev/vuln/GO-2026-6603) — HTTP/2 trailer memory exhaustion | Symbol; Go 1.25.13 | Go 1.26.9 / 1.27.2 | HTTP/2 **server** precondition is not enabled by current API startup. Patch toolchain; do not report demonstrated API exhaustion. |
| [GO-2026-6605](https://pkg.go.dev/vuln/GO-2026-6605) — HTTP/1 client CONNECT rejection desynchronization | Symbol; Go 1.25.13 | Go 1.26.9 / 1.27.2 | JWKS uses GET with nil body. No application CONNECT request with the vulnerable body behavior was found; a `Client.Do` trace alone is insufficient. |
| [GO-2026-6607](https://pkg.go.dev/vuln/GO-2026-6607) — malformed ECH outer references | Symbol; Go 1.25.13 | Go 1.26.9 / 1.27.2 | Vulnerable TLS-server/ECH configuration is not established; the API does not run a TLS server. TLS-client use does not establish this server condition. |
| [GO-2026-6608](https://pkg.go.dev/vuln/GO-2026-6608) — MIME memory-limit bypass | Symbol; Go 1.25.13 | Go 1.26.9 / 1.27.2 | API accepts JSON, not multipart. The specific low-remaining-memory multipart/header context is not shown by the generic MIME-header trace. |
| [GO-2026-6610](https://pkg.go.dev/vuln/GO-2026-6610) — malformed HTTP/2 response framing headers | Symbol; Go 1.25.13 | Go 1.26.9 / 1.27.2 | JWKS may be an HTTP/2 client, but its response headers are not forwarded into HTTP/1 responses. No reverse-proxy smuggling path is demonstrated. |
| [GO-2026-6611](https://pkg.go.dev/vuln/GO-2026-6611) — repeated initial-window settings cause CPU use | Symbol; Go 1.25.13 | Go 1.26.9 / 1.27.2 | Potential conditional path through an HTTPS/HTTP2 JWKS peer, including a malicious or compromised upstream. Current API does not enable the server variant. Patch and exercise bounded IdP failure behavior. |
| [GO-2026-6612](https://pkg.go.dev/vuln/GO-2026-6612) — HTTP/2 server flow-control refund | Symbol; Go 1.25.13 | Go 1.26.9 / 1.27.2 | Current server configuration does not enable HTTP/2. Patch before any future TLS/h2c server change. |
| [GO-2026-6613](https://pkg.go.dev/vuln/GO-2026-6613) — HTTP/1 server 2xx CONNECT desynchronization | Symbol; Go 1.25.13 | Go 1.26.9 / 1.27.2 | Health handlers accept all methods and can return 200 to CONNECT without hijacking. This application-side precondition exists; gateway forwarding/intermediary exploit conditions are unknown. Restrict health methods under F-015 and patch; request smuggling was not executed. |
| [GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617) — HTTP/2 HPACK encoder race | Symbol; Go 1.25.13 | Go 1.26.9 / 1.27.2 | HTTP/2-server path is not enabled by current startup. No observed application crash. |
| [GO-2026-6629](https://pkg.go.dev/vuln/GO-2026-6629) — PRECIS crafted-input panic | Symbol; `x/text` v0.39.0 | v0.41.0 | Advisory concerns the Nickname profile/short destination behavior. pgx SCRAM uses `precis.OpaqueString.String(config.Password)`, not Nickname; the generic profile trace does not establish the triggering input/profile. Upgrade and verify dependency compatibility. |
| [GO-2026-5841](https://pkg.go.dev/vuln/GO-2026-5841) — S2 out-of-bounds read | Package/module only; `klauspost/compress` v1.18.0 | v1.18.7 | No called vulnerable symbol reported. Triage transitive usage and upgrade compatible graph; no S2 exploit path was proved. |
| [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) — unmaintained unsafe OpenPGP package | Package/module only; `x/crypto` v0.52.0 | No fixed version | No application OpenPGP use found. Do not claim a version bump fixes this advisory; keep unused or replace with an approved maintained design if ever required. |
| [GO-2026-6303](https://pkg.go.dev/vuln/GO-2026-6303) — SSH source-address option bypass | Package/module only; `x/crypto` v0.52.0 | v0.55.0 | No SSH server/auth callback in application source. Upgrade/record non-applicability rather than calling it exposed. |
| [GO-2026-6354](https://pkg.go.dev/vuln/GO-2026-6354) — undecided SSH channel deadlock | Package/module only; `x/crypto` v0.52.0 | v0.56.0 | No application SSH channel handling. No called vulnerable symbol reported. |
| [GO-2026-6355](https://pkg.go.dev/vuln/GO-2026-6355) — established SSH channel deadlock | Package/module only; `x/crypto` v0.52.0 | v0.56.0 | Same absent SSH runtime context; retain in dependency triage. |
| [GO-2026-6599](https://pkg.go.dev/vuln/GO-2026-6599) — consecutive template expression context | Package/module only; Go 1.25.13 | Go 1.26.9 / 1.27.2 | No HTML template rendering in this JSON API or worker. |
| [GO-2026-6600](https://pkg.go.dev/vuln/GO-2026-6600) — template `yield` regexp context | Package/module only; Go 1.25.13 | Go 1.26.9 / 1.27.2 | No HTML template rendering path. |
| [GO-2026-6604](https://pkg.go.dev/vuln/GO-2026-6604) — Windows root junction escape | Package/module only; Go 1.25.13 | Go 1.26.9 / 1.27.2 | Audit/build target is Linux and no `os.Root` application operation was found; Windows-specific condition is absent. |
| [GO-2026-6609](https://pkg.go.dev/vuln/GO-2026-6609) — unbounded parsed Range header | Package/module only; Go 1.25.13 | Go 1.26.9 / 1.27.2 | No `ServeContent`, `ServeFile` or file-server route was found. Ordinary JSON serving is not evidence of the vulnerable Range parser being used. |

The pgx profile check is source-backed at `github.com/jackc/pgx/v5@v5.9.2/pgconn/auth_scram.go:195-201` in the isolated module cache. The Go header-parser distinction is at the audited toolchain's `src/net/textproto/reader.go:506-515`. These checks explain reachability limits; they are not a substitute for patching a supported toolchain.

**AT-011** requires a passing standard vulnerability gate or narrowly scoped, time-limited security-owner exceptions supported by configuration evidence, consistent API/worker toolchains, regression/race results, and scans attached to the final immutable images. Re-scan at implementation and release time because the table is an audit-date snapshot. No container-runtime scan, full license review, SBOM or image provenance was supplied; see the [dependency inventory](11_THIRD_PARTY_INTEGRATIONS_AUDIT.md) and [operations report](12_DEVOPS_INFRASTRUCTURE_AND_DEPLOYMENT_AUDIT.md).

## 7. Security acceptance and accountable decisions

This report does not create a second backlog. The release-critical work is already tracked in the canonical register:

| Security outcome | Canonical acceptance/gate references | Proposed accountable role; approval status |
|---|---|---|
| A subject cannot be re-authorized by a stale or another customer's challenge; confirmation proves the approved independent step. | AT-001 / G-001, AT-002 / G-002, AT-003 / G-003 | Backend lead, identity owner and trusted-delivery owner; named owners not supplied. D-02, D-03, D-06. |
| Stored/exported evidence is verifiable and reconstructs the approved notice, policy and suppression state. | AT-004 / G-004, AT-005 / G-005, AT-029 / G-029 | Privacy owner and data/backend lead; D-05, D-14 unresolved. |
| Authentication remains bounded under untrusted traffic and actual provider rotation. | AT-014 / G-014, AT-015 / G-015, AT-030 / G-030 | Identity/platform owners; D-08, D-09, D-10 unresolved. |
| Deployed identities and channels meet the approved least-privilege model. | AT-019 / G-019, AT-022 / G-022 | Security, platform and database owners; deployment evidence not supplied. |
| Supported dependencies and runtime artifacts satisfy the release scan policy. | AT-011 / G-011, AT-018 / G-018 | Security/release owner; final patched artifacts not yet built. |
| Actual launch products honor negative consent and dependency failures. | AT-010 / G-010, AT-023 / G-023 | Each approved consumer owner; D-01, D-07, D-13 unresolved. |

Use the [testing report](14_TESTING_QA_AND_VERIFICATION_REPORT.md) to distinguish executed and proposed tests, the [risk register](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md) for severity and disposition, the [backlog](16_DETAILED_IMPLEMENTATION_BACKLOG.md) for exact work packages, the [go-live checklist](19_PRODUCTION_GO_LIVE_CHECKLIST.md) for acceptance evidence, and the [decision/dependency register](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md) for approvals. None of these unresolved decisions authorizes silently broadening roles, weakening denial behavior or asserting legal compliance.
