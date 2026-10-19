# 05 — Frontend audit

**Audit date:** 19 October 2026. **Finding:** the supplied NVUTO Consent repository contains **no frontend application, screen, UI route, component or API client**. Frontend implementation and user-facing integration cannot be assessed from this archive. This is not evidence that a separate NVUTO application lacks a frontend, and it is not an instruction to add an unapproved standalone UI to the Consent service.

The service boundary comes from `README.md:3-11`: Consent owns privacy/preference decisions and evidence; customer identity, campaigns and other consuming application functions are external. The named Customer Portal and other consumers are absent. [D-01](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-01) determines the product/release boundary; [D-07](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-07) determines which consumers must be integrated for the initial launch. [F-023](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-023), [F-024](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-024) and [F-032](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-032) record these evidence and contract gaps.

## 1. Discovery method and complete frontend inventory

The audit inventoried the original archive including `.github/workflows/ci.yml`, inspected both Go entry points and traced the API registration. The original inventory contains 32 files: Go source/tests, module manifests, SQL migrations, README/rollback prose, Dockerfiles, Kubernetes YAML and a CI shell script. No package manifest, browser asset or frontend entry point is present. The full repository map and file evidence are in [01_REPOSITORY_ARCHITECTURE_MAP.md](01_REPOSITORY_ARCHITECTURE_MAP.md) and [21_AUDIT_EVIDENCE_AND_COMMAND_LOG.md](21_AUDIT_EVIDENCE_AND_COMMAND_LOG.md).

| Frontend artifact | Count/status in supplied archive | Evidence and implication |
|---|---|---|
| Frontend applications | 0 | Neither an SPA nor server-rendered UI entry point exists. |
| Screens/pages | 0 | There is no genuine screen-by-screen source corpus. |
| Client-side routes/layouts | 0 | No routing, navigation or dashboard implementation can be traced. |
| UI components, forms, modals and tables | 0 | No buttons, validation messages or fake-success UI can be classified. |
| State-management/data-fetching modules | 0 | No query cache, store, reducer or browser persistence implementation exists. |
| Frontend API clients | 0 | No in-repository call site consumes the 11 business endpoints in `cmd/server/main.go:399-409`. |
| Session/auth guards | 0 | Bearer JWT verification is backend code; it is not evidence of browser login/session handling. |
| Frontend unit/component/browser tests | 0 | Go httptest exercises handlers, not a DOM or browser. |
| Static mock data or UI placeholders | Not applicable | No frontend source was found to inspect for them. Go memory fixtures are backend test doubles, not product UI. |
| Frontend build/lint/start commands | Not applicable | No supported frontend command is available; inventing `npm run dev`, a framework or a folder would mislead the incoming engineer. |

`cmd/server/main.go:386-410` serves health, readiness and JSON business handlers. It does not register HTML pages, static file serving, a frontend asset directory or a server-side template engine. `cmd/outbox-worker/main.go` is a background publisher, not a UI application.

## 2. Screen audit disposition

The current screen inventory is empty. The following are **potential consumer experiences**, included to show which owning repository would be needed to verify the documented workflows. Their names are descriptions, not invented existing screens/routes or approved new frontend scope.

| Potential consumer experience | In-repository UI status | Available Consent capability | Missing evidence / contract dependency |
|---|---|---|---|
| Customer preference center | ABSENT; owner unverified | Record/deny, evaluate, withdraw known ID, set/clear suppression, read known receipt | Approved current-preference/registry read source is missing; no initial load, reload, error, subject binding or user-feedback trace. F-023/F-024; D-01/D-07. |
| Privacy/consent administration | ABSENT; owner unverified | POST purpose, lawful-basis and notice registry writes | No registry GET/list or version-aware edit contract; complete controls and policy-preview behavior not supplied. [F-007](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-007)/F-024/[F-027](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-027)/[F-029](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-029). |
| Double-opt-in initiation and confirmation | ABSENT; delivery owner undecided | Start and confirm endpoints exist | Raw secret is exposed to self-service; other-subject confirmation succeeds with token; stale challenges can override later preferences. [F-001](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-001)/[F-002](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-002)/[F-003](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-003). |
| CRM/contact-center preference capture | ABSENT; actual consumer code unavailable | Governed capture and optional consent-create idempotency | Actual operator mapping, canonical subject lookup, evidence collection and retry state unknown. F-023/[F-025](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-025)/[F-030](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-030). |
| Consent evidence/history view | ABSENT; owner unverified | GET a receipt by known ID | No history/search/export API; receipt content/hash and audit completeness have confirmed defects. [F-004](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-004)/[F-005](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-005)/F-024/F-029. |
| Campaign/conversation pre-send decision feedback | ABSENT; action execution outside this repository | Single/batch Evaluate and change events | No actual UI or send-boundary enforcement evidence. [F-010](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-010)/F-023. |

No login, registration, password reset, profile setup, payment, billing or account-deletion screens are added to this inventory. The archive does not establish those as Consent-owned requirements. Account identity is explicitly outside this service (`README.md:7`).

## 3. Frontend quality assessment

| Quality dimension requested in the audit brief | Current result | Evidence required before a user-facing claim |
|---|---|---|
| Responsive/mobile/tablet layouts | NOT ASSESSABLE | Actual consumer layout source plus target-device/browser runs. |
| Visual/design consistency | NOT ASSESSABLE | Approved design direction and rendered owning application screens. |
| WCAG 2.2 AA, keyboard and screen-reader behavior | NOT ASSESSABLE | Browser/accessibility evidence from the actual selected UI, with target acceptance agreed by its owner. No accessibility conformance is claimed from JSON API tests. |
| Form usability and validation feedback | NOT ASSESSABLE | Rendered forms, exact request DTO mapping, field errors and failure recovery. Backend currently returns generic validation text, [F-013](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-013)/F-027. |
| Loading, empty, error and permission states | NOT ASSESSABLE | A consumer walkthrough of unavailable service, missing preference, forbidden subject, pending DOI, conflict and expired challenge. |
| Session renewal, token storage and logout | NOT ASSESSABLE | Consumer session architecture and IdP integration under [D-10](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10). Do not infer localStorage, cookies or refresh tokens from the backend. |
| Browser security/CORS | CONTRACT INCOMPLETE | API has no CORS/OPTIONS middleware; same-origin gateway or approved origins must be defined where needed, F-024. |
| Client performance and bundle size | NOT ASSESSABLE | Real build artifacts, bundle analysis and representative user flows. |
| Localization/notice language | UNDOCUMENTED | Approved operating regions, language and notice-artifact requirements under [D-05](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-05)/[D-14](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-14). `NG` test values do not establish production language or legal scope. |
| Navigation/deep links/confirmation links | NOT ASSESSABLE | Approved consumer routes and DOI delivery/return contract under [D-02](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-02). No public link route is implemented here. |
| User-visible persistence after reload | NOT VERIFIED | A real selected consumer reloads state from its approved source following capture/withdrawal/suppression. F-023/F-024. |

## 4. Backend behavior that the eventual consumer must handle

This section gives concrete integration constraints to the incoming frontend engineer without pretending a UI has been implemented.

**Authentication and authorization.** All business calls require configured authentication and an accepted role. A self-service JWT must carry the correct canonical `customer_subject_id`; using the identity-provider `sub` as the customer ID is not equivalent. The backend stores `sub` as the actor. Broader signed roles permit tenant-wide operations. Use the authoritative mapping in [04_USER_PERSONAS_ROLES_AND_PERMISSIONS.md](04_USER_PERSONAS_ROLES_AND_PERMISSIONS.md), and fix API-10 before relying on subject isolation. Source: `internal/auth/auth.go:27-31,131-165`; `cmd/server/main.go:109-118,343-362`.

**Initial preference state.** The existing API has no list/current-preferences endpoint and no registry GET/list. A UI can evaluate known subject/purpose/channel combinations, but the code does not supply the initial vocabulary and full preference/suppression history. Either [T-024](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-024) adds approved read endpoints or the selected consumer supplies and proves an owned projection. A screen must not imply “no preference” merely because a read contract is missing.

**Capture and retries.** API-04 accepts only granted/denied input states, performs active policy/notice/evidence checks and returns 201 with a consent/receipt ID. The optional Idempotency-Key is unique within tenant, includes actor in its request fingerprint and currently returns the latest state of the original resource on replay. Do not treat a replayed create response as present permission; Evaluate is the decision endpoint. F-025/[D-11](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-11) must settle replay semantics before a maintained SDK is issued. Source: `cmd/server/main.go:180-208`; `internal/service/idempotency.go:39-81`.

**Denial and withdrawal.** Normal deny decisions use HTTP 200 with `allowed=false`; a successful HTTP response never by itself authorizes a communication. Withdrawal uses a known consent ID. Direct denied capture currently requires the same policy/evidence checks as grant, so error handling during policy changes must preserve the approved opt-out experience. F-001/[F-006](15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-006)/F-027 must resolve stale intent, timing and negative-preference rules. Source: `internal/service/service.go:121-149,181-199,342-387`.

**Double opt-in.** The current start endpoint returns a redeemable raw token to the initiating self-service client. It is not an approved browser delivery pattern. [T-003](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-003) must provide a trusted delivery design and public challenge status before a consumer builds around the response. Browser logs, analytics, URLs and user-visible errors must not expose the capability. The confirmation route requires bearer authentication; no unauthenticated email-link route exists. Source: `cmd/server/main.go:319-362`; `internal/domain/model.go:127-130`.

**Errors and asynchronous completion.** The server currently uses 400 for malformed JSON, 401/403 for access errors, 404 for missing records, 409 for conflicts, 410 for expired challenges and generally 422 for all other errors, including some infrastructure errors. A mutation succeeds when its local transaction commits; it does not prove remote consumers processed its event. The actual consumer must distinguish saved preference from propagation state according to [D-08](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-08)/[D-13](20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-13), not display an unverified global-success claim. Source: `cmd/server/main.go:59-85`; `internal/store/postgres.go:238-259`; `cmd/outbox-worker/main.go:102-134`.

## 5. Required next work without expanding scope

1. Resolve D-01/D-07 with the product owner: identify which real customer/operator experiences are part of the initial launch and which application repositories own them.
2. Obtain those repositories and their approved routes/design requirements. Record each UI source path and current API call site in [T-023](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-023) rather than inventing screens in this report.
3. Resolve the read/projection contract through T-024 and the claims/permissions contract through [T-030](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-030). Confirm same-origin gateway or CORS ownership for each browser client.
4. Complete the consent/DOI/evidence fixes before a client freezes incompatible assumptions: [T-001](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-001)–[T-008](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-008), [T-010](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-010), [T-013](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-013), [T-025](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-025) and [T-027](16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-027) as applicable.
5. Execute real browser-to-API-to-YSQL and event-consumer flows with approved synthetic data. Include reload persistence, cancellation/failure, all disallowed states, permissions and external dependency outages.
6. Perform UI quality/accessibility/performance testing in the owning repositories against approved targets. No frontend tests were executed in this audit because no frontend source exists.

The unchanged backend baseline produced 15 PASS / 3 SKIP, also under race detection; these are Go tests, not browser acceptance. The separate 11 FAIL + 1 OBSERVATION audit probe result includes API and service defects but no UI rendering tests. Exact evidence is centralized in [14_TESTING_QA_AND_VERIFICATION_REPORT.md](14_TESTING_QA_AND_VERIFICATION_REPORT.md).

## 6. Release disposition

There is no in-repository frontend to label complete, broken or inaccessible. **Frontend-to-service launch acceptance is NOT VERIFIED.** [G-023](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-023) requires each approved launch consumer’s real integration/UAT record; [G-024](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-024) requires its authorized data source and maintained API contract; [G-030](19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-030) requires actual identity-role mapping. D-01/D-07 may approve an API-only first release or a selected set of consumer experiences, but neither decision removes the confirmed backend consent and authorization blockers.

Use [07_FRONTEND_BACKEND_INTEGRATION_MATRIX.md](07_FRONTEND_BACKEND_INTEGRATION_MATRIX.md) for the eleven workflow IDs and [09_APPLICATION_USER_FLOW_AUDIT.md](09_APPLICATION_USER_FLOW_AUDIT.md) for success, failure and edge paths. They mark external components as absent/unverified rather than assuming a functioning screen.
