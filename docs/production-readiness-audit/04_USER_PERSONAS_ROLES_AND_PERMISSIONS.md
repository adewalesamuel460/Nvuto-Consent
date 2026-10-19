# 04 — User personas, roles and permissions

**Audit date:** 19 October 2026. **Scope:** the supplied 32-file NVUTO Consent source archive. **Assessment:** backend roles are implemented, but the complete production permission model is not approved or comprehensively tested. A self-service confirmation boundary failed an isolated HTTP audit probe. There are no frontend screens or account-management flows in this repository.

This report records current behavior rather than proposing new authority. The canonical risk, implementation and release records are [the finding register](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md), [the detailed backlog](16_DETAILED_IMPLEMENTATION_BACKLOG.md), [the release checklist](19_PRODUCTION_GO_LIVE_CHECKLIST.md) and [the decision register](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md). IDs with the same suffix identify a finding, task, acceptance test and gate: for example, [F-002](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-002) / [T-002](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-002) / [AT-002](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-002) / [G-002](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-002).

## 1. Product and identity boundary

NVUTO Consent is a shared consent, communication-preference and lawful-basis service. It explicitly does not own customer identity, campaign execution, conversations, legal contracts or analytics data. Its documented consumers are Engage, Service, Contact Center, Customer Portal, CRM, Conversations, AI and Integration Hub. These are application names, not automatically backend role names. Source: `README.md:3-11`.

There is no signup, login form, account verification, password reset, profile editor, invitation, MFA enrollment, account deletion or logout endpoint here. The API validates a token issued elsewhere. Absence of those identity features does not constitute a missing approved Consent module. Actual identity-provider configuration, user provisioning, account lifecycle and consumer sessions remain external dependencies under [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01) and [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10); the source archive does not prove that another NVUTO product already implements them.

There is also no native role named `auditor`, `support`, `contact_center_agent`, `crm_admin` or `platform_operator`. A business persona must be deliberately mapped to one of the seven recognized role strings or to a newly approved role through [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030). A job title, UI label or hidden button provides no backend authorization evidence.

## 2. Authentication and principal contract

The production path uses `JWT_JWKS_URL`, `JWT_ISSUER` and `JWT_AUDIENCE`. It accepts RS256, requires an expiration, looks up the JWT `kid` through JWKS and requires nonempty tenant and registered subject claims. The application-specific customer identifier is a separate claim. Source: `internal/auth/auth.go:27-31,131-165`.

| Token/configuration value | Current interpretation | Handover implication |
|---|---|---|
| `tenant_id` | Principal.TenantID; used to scope database operations | The trusted issuer must assign the correct tenant. Business APIs do not accept a selectable tenant independently of the principal. |
| Registered JWT `sub` | Principal.ActorID; audit actor for mutations | The authenticated actor is not necessarily the customer whose preferences are affected. Preserve this distinction for operators and service accounts. |
| `customer_subject_id` | Principal.SubjectID | Required by the documented self-service contract to equal the canonical Customer Graph party identifier. The API does not call Customer Graph to validate or resolve it. |
| `roles` | Exact case-sensitive string array | No case normalization, hierarchy service or role lookup exists. Handler admission and subject authorization independently accept matching roles, which can broaden a mixed token's write scope. |
| `iss`, `aud`, `exp`, `kid` | Production issuer, audience, expiry and signing-key checks | The real provider/claim mapper and rotation behavior need staging evidence; a synthetic JWKS test is insufficient. |
| `JWT_HMAC_SECRET` with `ALLOW_HMAC_AUTH=true` | Explicit HS256 development/test path when JWKS is not configured | It is not the documented production mode. Do not infer production HMAC authorization from passing local tests. |
| `ALLOW_DEV_HEADERS=true` | Development headers supply tenant and actor, then the code grants `developer` | This is explicit development authentication, not trusted production identity. |

When JWKS configuration is present, the JWKS path takes precedence over HMAC and header modes. Invalid JWT authentication is not followed by development-header fallback. Development header mode uses `X-Nvuto-Tenant-ID` and `X-Nvuto-Actor-ID`; it sets the principal customer subject to the actor and assigns `developer`. Source: `internal/auth/auth.go:138-188`.

The code does not globally reject a self-service JWT missing `customer_subject_id`: most handlers reject it when `subjectAuthorized` runs. Confirmation currently omits that check. F-002/T-002 covers enforcing the customer constraint before mutation, including the missing-claim case. See `cmd/server/main.go:113-118,343-362`.

## 3. Personas supported by observed capabilities

Descriptions below are evidence-based role interpretations. The repository has no formal persona research, approved organizational hierarchy or PRD defining user demographics, monetization, operating regions or account onboarding.

| Persona | Main objective and available modules | Backend role mapping | Onboarding and lifecycle | Dashboard, notifications and relationships |
|---|---|---|---|---|
| Privacy or consent administrator | Configure purpose, basis and notice registries; perform subject mutations; inspect decisions and receipts | `privacy_admin` or `consent_admin`; identical permissions in current code | Externally provisioned identity with approved tenant and roles. No account-creation or approval workflow here. | No admin dashboard. Registry writes publish change events. May act on any subject within the token tenant. No separation of duties between the two admin roles is implemented. |
| Authorized collecting application or operator | Record granted/denied preferences, withdraw consent, set/clear suppression and orchestrate DOI | `consent_writer` | Upstream operator/service identity and canonical customer mapping required. Whether it represents a human or a client credential is outside this implementation. | No CRM/contact-center UI. Can affect any subject within tenant; cannot maintain registries. Actor is recorded separately from affected customer. |
| Decision-consuming application | Evaluate a purpose/channel decision before the application performs its action; retrieve a known receipt | `service` or `consent_reader` | Trusted external issuer provisions the tenant-bound principal. The API does not implement OAuth grant flows or client registration. | No application dashboard here. Both roles can read decisions and receipts for any subject within tenant. Neither can start/confirm DOI or write preferences. |
| Customer or data subject | View and change personal preferences through an owning consumer such as Customer Portal | `self_service` | External identity must be bound to `customer_subject_id`. Actual registration, login, recovery, session refresh and party linking are unverified. | No preference center. Most writes and reads are constrained to own subject; confirmation is defective. Notification/delivery owner for DOI is undecided under [D-02](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-02). |
| Development integrator | Exercise and integrate local APIs | `developer` | Explicit local auth mode or an issuer-signed role. No built-in environment check restricts this JWT role to development. | Broad tenant privileges. Production issuance must be governed by [D-06](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-06)/D-10 rather than assumed impossible. |
| Platform operator | Deploy processes, configure dependencies and investigate health/lag | No JWT platform role is defined | Infrastructure access is outside API RBAC and requires deployment/database/broker ownership | Public health/readiness probes exist. Metrics, worker progress, infrastructure permissions and incident routing are incomplete; see [F-015](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-015), [F-018](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-018)–[F-022](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-022). |

Source for role capabilities: `cmd/server/main.go:109-118` and registered handlers `120-409`. Documented identity exclusion and consumer names: `README.md:7-9,41`.

## 4. Exact current permission matrix

“Yes” means the handler admits that role, subject to authentication, tenant scoping, input validation and any subject check. It does not mean the function is production-ready. API IDs are defined in [the backend/API inventory](06_BACKEND_AND_API_AUDIT.md).

| Role | Registry writes API-01–03 | Record/withdraw/suppress API-04–06 | Evaluate single/batch API-07–08 | Start/confirm DOI API-09–10 | Receipt read API-11 | Subject scope |
|---|---|---|---|---|---|---|
| `privacy_admin` | Yes | Yes | Yes | Yes | Yes | Any subject in principal tenant |
| `consent_admin` | Yes | Yes | Yes | Yes | Yes | Any subject in principal tenant |
| `consent_writer` | No | Yes | Yes | Yes | Yes | Any subject in principal tenant |
| `consent_reader` | No | No | Yes | No | Yes | Any subject in principal tenant |
| `service` | No | No | Yes | No | Yes | Any subject in principal tenant |
| `self_service` | No | Own subject | Own subject | Start: own subject; confirm: **tenant and token only** | Own subject | Canonical customer constraint is missing from API-10 |
| `developer` | Yes | Yes | Yes | Yes | Yes | Any subject in principal tenant |
| Authenticated with no recognized role | No | No | No | No | No | Business route returns 403 |
| Unauthenticated | No | No | No | No | No | Business route returns 401; probes are public |

The method guard executes before the authentication check. An unsupported method can therefore return 405 rather than 401. OPS-01 `/healthz` and OPS-02 `/readyz` have no authentication or method guard and currently accept any HTTP method. Source: `cmd/server/main.go:101-111,120-127,386-398`.

### Subject authorization at each boundary

| Operation | Current object authorization | Evidence | Assessment |
|---|---|---|---|
| API-04 record | Compare request `subject_id` with canonical subject for a solely self-service caller, before service normalization | `cmd/server/main.go:184-203` | PRESENT; full negative-role/tenant matrix unverified |
| API-05 withdraw | Load consent within tenant, compare stored subject, then withdraw | `cmd/server/main.go:215-234` | PRESENT; test through durable HTTP required |
| API-06 suppression | Compare request subject before update | `cmd/server/main.go:245-257` | PRESENT; protected administrative suppression authority undecided |
| API-07 evaluate | Compare request subject | `cmd/server/main.go:269-285` | Existing own/other-subject handler test PASS |
| API-08 batch | Compare every requested subject; one unauthorized item rejects whole request | `cmd/server/main.go:297-311` | PRESENT; mixed-item boundary test absent from original corpus |
| API-09 DOI start | Compare request subject, then return full challenge including secret | `cmd/server/main.go:323-340` | Subject check PRESENT; token-delivery design BROKEN, [F-003](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-003) |
| API-10 DOI confirm | Check writer role; service finds token within tenant; no customer comparison | `cmd/server/main.go:347-362`; `internal/service/service.go:305-325` | BROKEN, F-002, isolated HTTP reproduction |
| API-11 receipt | Load receipt within tenant, then compare stored subject before response | `cmd/server/main.go:369-383` | PRESENT; actual receipt hash integrity separately fails [F-004](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-004) |

Read-before-subject-check can distinguish an existing other-subject ID (403) from an absent ID (404), but the handler does not return the protected record body on rejection. Choose consistent rejection semantics in T-002/T-030; no broad data disclosure or cross-tenant SQL exploit is inferred from that distinction.

## 5. Permission ambiguities requiring decisions

### Mixed roles and the `developer` role

`subjectAuthorized` immediately permits any principal containing `privacy_admin`, `consent_admin`, `consent_writer`, `consent_reader`, `service` or `developer`. Only a principal without any of those broader roles is restricted by `self_service`. The handler and object checks compose independently: a token containing both `self_service` and `service` or `consent_reader` passes a writer handler because of self_service, then bypasses its customer restriction because of the other role. It can therefore perform tenant-wide subject writes even though service/consent_reader alone are read-only. Registry writes still require an administrative role. This source-confirmed composition is part of F-030; no unapproved role assignment or live exploit is claimed. D-06/D-10 must explicitly approve or change it. Source: `cmd/server/main.go:109-118`.

`developer` is an accepted role string in production JWTs if the issuer grants it. The explicit environment flags gate development **authentication modes**, not acceptance of that role from a valid production token. T-030 must align actual identity-provider mappings, application policy and tests. Do not describe the developer JWT role as already confined to local use.

### Suppression-clearing authority

Any authorized writer, including a customer acting on their own subject, can POST an existing suppression natural key with `active=false`. The model contains reason but no creator/owner/protected category; clearing authority does not depend on who created it. A self-service caller can therefore clear an own-subject global/channel/purpose-channel suppression created by an administrator. Whether this is permitted customer preference management or an unacceptable override depends on D-06. It is a **DECISION REQUIRED** finding [F-030](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-030), not a proven violation of a rule absent from the repository. Source: `cmd/server/main.go:241-258`; `internal/service/service.go:202-234`; `internal/store/postgres.go:300-320`.

If policy distinguishes administrative/legal blocks from voluntary customer opt-outs, T-030 must model that distinction, provenance and role enforcement. UI hiding alone is insufficient. If all suppressions are user preferences, document that meaning and avoid treating them as non-overridable legal controls.

### Receipt and audit access

Reader/service roles can retrieve any known receipt within their tenant, including collection method and artifact hash. There is no narrower permission for receipt evidence, no audit-history export route and no role-specific redaction. [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05)/D-06 must define authorized investigator access and evidence handling. [F-024](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-024) covers the read contract; [F-029](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-029) covers history reconstruction. No new `auditor` role is present until approved and implemented.

## 6. Observed test evidence and its limits

The unchanged baseline suite produced **15 PASS / 3 SKIP**, also under race detection. The three skipped suites require external database/broker infrastructure. The separate audit probes produced **11 failing safety assertions plus one behavioral observation**; they were run in a disposable copy without modifying implementation. See [the QA report](14_TESTING_QA_AND_VERIFICATION_REPORT.md) and [command/evidence log](21_AUDIT_EVIDENCE_AND_COMMAND_LOG.md).

| Evidence | Observed result | What it establishes and what it does not |
|---|---|---|
| `TestAuthenticationFailsClosedAndDevHeadersRequireExplicitOptIn`, `internal/auth/auth_test.go:26-40` | PASS in original suite | Unconfigured auth rejects and explicit development headers work in the fixture. No production deployment proof. |
| `TestJWTValidationRejectsExpiredWrongAlgorithmAndHeaderFallback`, `internal/auth/auth_test.go:42-74` | PASS | Selected HMAC expiration/algorithm and fallback behavior; not the full RS256 negative matrix. |
| `TestAuthentikCompatibleJWKSValidation`, `internal/auth/auth_test.go:76-131` | PASS | Synthetic local RSA/JWKS valid-token and wrong-issuer cases. No live Authentik, key-rotation or role mapper proof. |
| `TestSelfServiceJWTIsBoundToCanonicalCustomerSubject`, `cmd/server/main_test.go:105-138` | PASS | Solely self-service HMAC token can evaluate own subject and receives 403 on other subject. Other subject-sensitive handlers are not tested here. |
| `TestAuditSelfServiceCannotConfirmAnotherSubjectChallenge` | FAIL safety assertion | Synthetic B self-service JWT confirmed A's valid token: HTTP 200 and A allowed=true. A valid victim token and same tenant were required. No token guessing, cross-tenant confirmation or live YSQL claim. F-002. |
| `TestAuditSelfServiceStartDoesNotExposeConfirmationSecret` | FAIL safety assertion | Own-subject start 201 exposes token; immediate confirm 200 yields allowed=true without independent delivery. No external messaging was sent. F-003. |
| `TestAuditJWKSUnknownKeyRefreshBounded` | FAIL safety assertion | Five distinct unknown key IDs caused five local JWKS fetches. It demonstrates refresh amplification, not a live service outage. [F-014](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-014). |

## 7. Required role acceptance before release

T-002 and T-030 must produce a signed matrix matching the actual issuer configuration, then exercise all seven roles across all eleven business operations. The matrix must include own subject, other subject in same tenant, other tenant, missing customer claim, no role, unknown role, mixed role and expired/invalid token cases. Rejection must be proven to leave consent, receipt, audit and outbox unchanged where mutation was attempted.

AT-002/G-002 must specifically pass API-10 authorization inside the atomic confirmation path. [AT-003](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-003)/[G-003](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-003) must prove that an untrusted initiator cannot obtain the confirmation capability directly. [AT-030](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-030)/[G-030](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-030) must record the approved suppression-clearing and mixed-role rules. Production-equivalent authenticated HTTP/YSQL execution belongs to [AT-016](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-016)/[G-016](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-016); selected consumer sessions and user outcomes belong to [AT-023](14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-023)/[G-023](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-023).

Identity onboarding is complete for this service only when D-10 supplies approved claim mappings, the canonical subject relationship and lifecycle ownership, and D-06 records who receives each permission. These decisions should be carried into the owning applications, rather than creating account-management scope that `README.md:7` explicitly excludes.
