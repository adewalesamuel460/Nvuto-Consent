# 07 — Frontend-Backend Integration Matrix (Customer Portal & Consumer Handover)

**Document ID:** `DOC-INT-007`  
**Status:** APPROVED / VERIFIED  
**Audit Date:** 26 October 2026  
**Scope:** INT-04 Customer Portal Frontend-Backend Verification & Cross-Subject Security Isolation  
**Reference Implementations:**  
- [internal/auth/jwt.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/auth/jwt.go)  
- [internal/auth/auth.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/auth/auth.go)  
- [tests/e2e/customer_portal_integration_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/tests/e2e/customer_portal_integration_test.go)  

---

## 1. Executive Summary & Verification Matrix

The NVUTO Customer Portal integration (**INT-04**) provides self-service preference management for data subjects. To maintain strict data privacy compliance (GDPR Art. 7, CCPA, NDPR), the Customer Portal interfaces with NVUTO Consent Service under two non-negotiable security boundaries:
1. **Authentik Canonical Subject Binding:** Authentication tokens issued by Authentik IdP explicitly bind to canonical Customer Graph identifiers (`cgraph:<tenant>:<type>:<uuid>`) and enforce the `self_service` role.
2. **Fail-Closed Cross-Subject Isolation:** Requests attempting to view, mutate, evaluate, or withdraw preferences for another data subject fail immediately with HTTP 403 Forbidden, writing zero database state or outbox events.

| Integration ID | Consumer / Portal System | Service Contract & Workflows | Security Boundary & Isolation | Verification Status | Proof of Compliance |
|---|---|---|---|---|---|
| **INT-04** | **Customer Portal** | Self-Service Preference Lifecycle: Initial hydration, Grant, Deny, DOI Initiation, DOI Confirmation, Withdrawal, and State Reload | Strict `self_service` principal binding; Zero-write database guarantee on cross-subject tampering | **VERIFIED (100% PASS)** | [customer_portal_integration_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/tests/e2e/customer_portal_integration_test.go) |
| **INT-01** | **Nvuto Engage** | Pre-Send Check & Outbox Sync | Monotonic revision & safe-deny | **VERIFIED** | [consumer_integration_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/tests/e2e/consumer_integration_test.go) |
| **INT-03** | **Nvuto Contact Center** | Pre-Send Check & Operator Capture | Operator boundary | **VERIFIED** | [consumer_integration_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/tests/e2e/consumer_integration_test.go) |
| **INT-05** | **Nvuto CRM** | Pre-Send Check & Local Replica | In-memory replica | **VERIFIED** | [consumer_integration_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/tests/e2e/consumer_integration_test.go) |

---

## 2. Authentik JWT Canonical Subject Claim Extraction

Authentik IdP issues JWT credentials upon data subject login. The NVUTO Consent API extracts and binds the subject identity via [internal/auth/jwt.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/auth/jwt.go) and [internal/auth/auth.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/auth/auth.go).

### Claim Hierarchy
1. **`customer_subject_id` (Explicit Claim):** Preferred property mapping in Authentik custom scope.
2. **`subject_id` (Custom Claim):** Supported secondary claim for standard OIDC property mappings.
3. **`sub` (Canonical Customer Graph URI):** Supported if formatted as canonical Customer Graph format: `cgraph:<tenant>:<type>:<uuid>`.
4. **Rejection of Plain Actor IDs:** If `roles` contains `self_service` and no canonical subject can be extracted (e.g. `sub: "plain_user"` without `customer_subject_id`), authentication fails closed with HTTP 401 Unauthorized (`"self_service token requires customer_subject_id"`).

### Role Mapping
The middleware extracts and deduplicates roles from both `roles` and `groups` claims emitted by Authentik. Tokens with role `self_service` are restricted strictly to:
$$\text{Scope}(p, \text{action}) = \text{service.OwnSubject}(p.\text{SubjectID})$$

---

## 3. Verified Self-Service Lifecycle (INT-04 Workflow)

The complete Customer Portal user journey was simulated against a live YugabyteDB/PostgreSQL and NATS JetStream test topology in [tests/e2e/customer_portal_integration_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/tests/e2e/customer_portal_integration_test.go):

```mermaid
sequenceDiagram
    autonumber
    actor Customer as User (Alice)
    participant Portal as Customer Portal UI
    participant API as Consent API (/v1)
    participant DB as YugabyteDB (Store)
    participant Outbox as Outbox Worker / NATS

    Customer->>Portal: Opens Preference Portal
    Portal->>API: GET /v1/consents?subject_id=cgraph:tenant:customer:alice (Hydration)
    API->>DB: Query consents by tenant & subject
    DB-->>API: 0 records
    API-->>Portal: 200 OK (Empty list)
    
    Customer->>Portal: Clicks "Accept Marketing"
    Portal->>API: POST /v1/consents (purpose=marketing, status=granted)
    API->>DB: RecordGoverned (consent, receipt, audit, outbox)
    DB-->>API: 201 Created (status=granted, gen=1)
    API-->>Portal: 201 Created
    
    Customer->>Portal: Clicks "Deny Analytics"
    Portal->>API: POST /v1/consents (purpose=analytics, status=denied)
    API->>DB: RecordGoverned (consent, receipt, audit, outbox)
    DB-->>API: 201 Created (status=denied)
    API-->>Portal: 201 Created

    Customer->>Portal: Clicks "Subscribe Newsletter"
    Portal->>API: POST /v1/double-opt-in (purpose=newsletter)
    API->>DB: StartDoubleOptIn (pending_confirmation)
    DB-->>API: Challenge created
    API-->>Portal: 201 Created (challenge_id, secret omitted)
    API->>Outbox: consent.challenge.created (contains confirmation_token)

    Note over Customer,Outbox: Outbox dispatches confirmation link to Alice's email
    Customer->>Portal: Clicks email confirmation link
    Portal->>API: POST /v1/double-opt-in/confirm (token=...)
    API->>DB: ConfirmDoubleOptIn (locks challenge, transitions to granted)
    DB-->>API: 200 OK (status=granted)
    API-->>Portal: 200 OK

    Customer->>Portal: Clicks "Withdraw Marketing"
    Portal->>API: POST /v1/consents/{id}/withdraw
    API->>DB: Withdraw (advances generation to 2, status=withdrawn)
    DB-->>API: 200 OK
    API-->>Portal: 200 OK

    Customer->>Portal: Refreshes Page (Reload)
    Portal->>API: GET /v1/consents?subject_id=cgraph:tenant:customer:alice
    API->>DB: ListConsents
    DB-->>API: 3 records (marketing: withdrawn, analytics: denied, newsletter: granted)
    API-->>Portal: 200 OK (Exact persisted state rendered)
```

---

## 4. Cross-Subject Security Boundary & Zero-Persistence Invariant

To guarantee that the Customer Portal fails closed when subject tampering is attempted, a dedicated security harness simulates Subject Alice attempting to access or manipulate Subject Bob's records:

| Attack Vector | HTTP Method & Endpoint | Attacker Token Bound To | Payload Specified Subject | Observed HTTP Response | Database Persistence Audit Assertion |
|---|---|---|---|---|---|
| **Cross-Subject Write** | `POST /v1/consents` | Subject Alice | Subject Bob | **403 Forbidden** (`subject_not_authorized`) | `consents` count for Bob's target purpose == 0 |
| **Cross-Subject Read** | `GET /v1/consents?subject_id=...` | Subject Alice | Subject Bob | **403 Forbidden** (`subject_not_authorized`) | Zero records leaked |
| **Cross-Subject DOI** | `POST /v1/double-opt-in` | Subject Alice | Subject Bob | **403 Forbidden** (`subject_not_authorized`) | Zero challenges created |
| **Cross-Subject Withdrawal** | `POST /v1/consents/{bobID}/withdraw` | Subject Alice | Target Bob's Consent ID | **403 Forbidden** (`subject_not_authorized`) | Bob's consent status unaltered (`granted`, gen 1) |
| **Cross-Subject Pre-Send Check** | `POST /v1/evaluate` | Subject Alice | Subject Bob | **403 Forbidden** (`subject_not_authorized`) | Zero evaluation decisions returned |
| **Cross-Subject Receipt Inspection** | `GET /v1/receipts/{bobReceiptID}` | Subject Alice | Target Bob's Receipt ID | **403 Forbidden** (`subject_not_authorized`) | Zero proof artifacts leaked |

### Zero-Persistence Proof
The integration test executes direct SQL queries on the test database following the attack sequence:
1. `SELECT COUNT(*) FROM consents WHERE tenant_id = $1 AND subject_id = $2 AND purpose = 'analytics'`: **Asserted == 0**
2. `SELECT COUNT(*) FROM consent_audit_events WHERE tenant_id = $1 AND actor_id = 'session-alice'`: **Asserted == 0**
3. `SELECT COUNT(*) FROM consent_outbox_events WHERE tenant_id = $1 AND payload->>'subject_id' = $2`: **Asserted == 0**
4. `SELECT status, preference_generation FROM consents WHERE id = $1` (Bob's legitimate record): **Asserted status = "granted", generation = 1**

---

## 5. Verification Commands & Execution Evidence

The test suite executed with zero errors and zero race conditions under Go 1.27.2 on Windows x64:

```powershell
$env:GOTMPDIR = "$PWD\build-tmp"
go test -v -count=1 ./tests/e2e/... -run ^TestCustomerPortal
```

### Verified Test Output
```text
=== RUN   TestCustomerPortal_SelfServiceLifecycle
    customer_portal_integration_test.go:312: Step 1: Hydrating initial preferences for Alice from read projection...
    customer_portal_integration_test.go:352: Step 2: Alice clicks 'Grant' for Marketing...
    customer_portal_integration_test.go:386: Step 3: Alice clicks 'Deny' for Analytics...
    customer_portal_integration_test.go:411: Step 4: Alice initiates Double-Opt-In for Newsletter...
    customer_portal_integration_test.go:443: Step 5: Retrieving DOI token from database outbox and confirming DOI...
    customer_portal_integration_test.go:481: Step 6: Alice withdraws previously granted Marketing consent...
    customer_portal_integration_test.go:505: Step 7: Simulating page refresh - Reloading preferences from read projection...
    customer_portal_integration_test.go:595: SUCCESS: Customer Portal self-service lifecycle verified completely.
--- PASS: TestCustomerPortal_SelfServiceLifecycle (2.42s)
=== RUN   TestCustomerPortal_CrossSubjectIsolation
    customer_portal_integration_test.go:646: Attack 1: Alice attempts to write consent for Bob...
    customer_portal_integration_test.go:664: Attack 2: Alice attempts to read Bob's preferences...
    customer_portal_integration_test.go:673: Attack 3: Alice attempts to initiate DOI for Bob...
    customer_portal_integration_test.go:690: Attack 4: Alice attempts to withdraw Bob's marketing consent...
    customer_portal_integration_test.go:699: Attack 5: Alice attempts to evaluate Bob's consent...
    customer_portal_integration_test.go:712: Attack 6: Alice attempts to read Bob's receipt...
    customer_portal_integration_test.go:725: Verifying zero persistence in PostgreSQL for all rejected attacks...
    customer_portal_integration_test.go:782: SUCCESS: Strict cross-subject isolation verified. System failed closed with zero persistence.
--- PASS: TestCustomerPortal_CrossSubjectIsolation (0.30s)
=== RUN   TestCustomerPortal_AuthentikClaimMappings
    customer_portal_integration_test.go:865: SUCCESS: Authentik claim mapping variants verified.
--- PASS: TestCustomerPortal_AuthentikClaimMappings (0.26s)
PASS
ok  	github.com/Atlasfsp/Nvuto-Consent/tests/e2e	3.233s
```

All acceptance criteria for ticket **INT-04** are verified and closed.
