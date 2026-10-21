# Operations Runbook: Transport Security & Least-Privilege IAM Architecture (T-019)

- **Finding Traceability:** [F-019](../../docs/production-readiness-audit/15_PRODUCTION_BLOCKERS_AND_RISK_REGISTER.md#f-019)
- **Task ID:** [T-019](../../docs/production-readiness-audit/16_DETAILED_IMPLEMENTATION_BACKLOG.md#t-019)
- **Acceptance Gate:** [AT-019](../../docs/production-readiness-audit/14_TESTING_QA_AND_VERIFICATION_REPORT.md#at-019) / [G-019](../../docs/production-readiness-audit/19_PRODUCTION_GO_LIVE_CHECKLIST.md#g-019)
- **Linked Decisions:** [D-09](../../docs/production-readiness-audit/20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-09) (Database Least-Privilege Roles & Separation of Concerns), [D-10](../../docs/production-readiness-audit/20_OPEN_DECISIONS_AND_EXTERNAL_DEPENDENCIES.md#d-10) (NATS JetStream Authorization & Multi-Tenant Subject Isolation)
- **Status:** APPROVED & OPERATIONAL

---

## 1. Executive Summary & Security Objectives

In high-consequence identity and consent platforms, all data in transit across network boundaries must be encrypted, and all identities operating on persistent storage or message brokers must be bound by the principle of least privilege. Prior to T-019, operational configurations permitted plaintext communication channels and monolithic database credentials capable of unrestricted DDL and cross-component table mutation.

This runbook documents the hardened security architecture implemented across the API service, outbox worker, YugabyteDB YSQL database, and NATS JetStream messaging fabric:

1. **Strict Transport Layer Security (TLS):** Plaintext protocols (`http://`, `nats://`, and `sslmode=disable`) are rejected at startup in production across all trust boundaries.
2. **Database Role-Based Access Control (RBAC):** Three distinct roles (`consent_api_role`, `consent_worker_role`, `consent_migration_role`) isolate operational privileges. The API cannot claim or mark outbox events; the worker cannot mutate consent records or receipts; neither service role has DDL privileges.
3. **NATS JetStream Multi-Tenant Subject Isolation:** Downstream subscriber credentials are mathematically restricted to subjects matching `nvuto.consent.outbox.{tenant_id}.>`, preventing cross-tenant snooping or unauthorized event interception.

---

## 2. Transport Layer Security (TLS) Mandate

TLS is enforced across four distinct trust boundaries:

```
[ External Client ] 
       │ HTTPS / TLS 1.3
       ▼
[ Consent API Service ] ──────────── HTTPS ────────────► [ OIDC / Authentik JWKS Provider ]
       │                                                      (rejects http:// outside dev)
       ├──────────── TLS (sslmode=verify-full) ────────► [ YugabyteDB YSQL Cluster ]
       ▼                                                      (consent_api_role)
[ PostgreSQL Storage ]
       ▲
       ├──────────── TLS (sslmode=verify-full) ───────── [ Outbox Worker Service ]
       │                                                      (consent_worker_role)
       └──────────── TLS (tls://) + mTLS/NKEY ─────────► [ NATS JetStream Cluster ]
                                                              │
                                                              ▼ (Scoped Subject ACLs)
                                                      [ Downstream Tenant Subscribers ]
                                                      (nvuto.consent.outbox.{tenant_id}.>)
```

### 2.1 Ingress & JWKS Provider TLS
- **File Reference:** [internal/auth/auth.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/auth/auth.go)
- **Configuration Parameter:** `JWT_JWKS_URL`
- **Rule:** The OpenID Connect / JWKS endpoint URL must specify the `https://` protocol scheme.
- **Fail-Closed Behavior:** If `JWT_JWKS_URL` starts with `http://` and `ALLOW_DEV_HEADERS=true` is not set, `auth.ValidateConfig()` fails startup immediately:
  ```
  auth configuration invalid: JWT_JWKS_URL must use https:// in production (requires ALLOW_DEV_HEADERS=true for local dev)
  ```

### 2.2 Database (YSQL / PostgreSQL) TLS
- **File References:** [cmd/server/main.go](file:///c:/antigravity/scratch/Nvuto-Consent/cmd/server/main.go), [cmd/outbox-worker/main.go](file:///c:/antigravity/scratch/Nvuto-Consent/cmd/outbox-worker/main.go)
- **Configuration Parameter:** `DATABASE_URL`
- **Enforcement Rules:**
  1. `sslmode=disable` is strictly prohibited unless `ALLOW_DEV_HEADERS=true` is set.
  2. In production (`ENVIRONMENT=production` or `APP_ENV=prod`), `DATABASE_URL` must explicitly specify `sslmode=verify-full` or `sslmode=require` (or `sslmode=verify-ca`).
- **Fail-Closed Behavior:** Startup aborts with an unrecoverable exit code (`os.Exit(1)`) if `sslmode` parameters fail production criteria:
  ```
  storage_configuration_invalid: DATABASE_URL requires sslmode=verify-full or sslmode=require in production
  ```

### 2.3 NATS Messaging Broker TLS
- **File Reference:** [cmd/outbox-worker/main.go](file:///c:/antigravity/scratch/Nvuto-Consent/cmd/outbox-worker/main.go)
- **Configuration Parameter:** `NATS_URL`
- **Enforcement Rules:**
  1. Production deployments require the `tls://` URL scheme (e.g. `tls://nats.messaging.internal:4222`).
  2. Plaintext `nats://` connections are refused at startup unless `ALLOW_INSECURE_NATS=true` is explicitly provided.
  3. Trusted Root CAs may be supplied via `NATS_TLS_CA_FILE` to validate custom or private enterprise certificate authorities.

---

## 3. Database Least-Privilege IAM Roles (008_least_privilege_roles.sql)

Migration [migrations/008_least_privilege_roles.sql](file:///c:/antigravity/scratch/Nvuto-Consent/migrations/008_least_privilege_roles.sql) establishes isolated database roles, completely replacing the legacy single-superuser pattern.

### 3.1 Role Taxonomy & Responsibilities

| Role Name | Granted By / Login | Target Component | Description & Boundaries |
|---|---|---|---|
| **`consent_api_role`** | NOLOGIN (Group / Role) | `cmd/server` (API Service) | Handles client HTTP mutations and evaluations. Enqueues outbox events transactionally. Cannot claim or complete outbox items. |
| **`consent_worker_role`** | NOLOGIN (Group / Role) | `cmd/outbox-worker` (Publisher) | Leases, fences, publishes, and marks outbox events. Reads consent state to build CloudEvent envelopes. Cannot mutate core consents or receipts. |
| **`consent_migration_role`** | NOLOGIN (Admin Role) | CI / CD Migration Runner | Sole role possessing DDL privileges (`CREATE`, `ALTER`, `DROP`) to execute numerical migrations. |

### 3.2 Granular Table Permission Matrix

| Table / Object | `consent_api_role` | `consent_worker_role` | `consent_migration_role` |
|---|---|---|---|
| `consents` | `SELECT`, `INSERT`, `UPDATE` | `SELECT` | `ALL PRIVILEGES` |
| `consent_receipts` | `SELECT`, `INSERT`, `UPDATE`* | `SELECT` | `ALL PRIVILEGES` |
| `consent_suppressions` | `SELECT`, `INSERT`, `UPDATE` | `SELECT` | `ALL PRIVILEGES` |
| `consent_purposes` | `SELECT`, `INSERT`, `UPDATE` | `SELECT` | `ALL PRIVILEGES` |
| `consent_lawful_bases` | `SELECT`, `INSERT`, `UPDATE` | `SELECT` | `ALL PRIVILEGES` |
| `consent_privacy_notices` | `SELECT`, `INSERT`, `UPDATE`* | `SELECT` | `ALL PRIVILEGES` |
| `consent_preference_state` | `SELECT`, `INSERT`, `UPDATE` | `SELECT` | `ALL PRIVILEGES` |
| `consent_idempotency` | `SELECT`, `INSERT`, `UPDATE` | `SELECT` | `ALL PRIVILEGES` |
| `consent_audit_events` | `SELECT`, `INSERT` | `SELECT` | `ALL PRIVILEGES` |
| **`consent_outbox_events`** | **`INSERT` ONLY** | **`SELECT`, `UPDATE`** | `ALL PRIVILEGES` |
| `public` Schema DDL (`CREATE`) | **REVOKED** | **REVOKED** | `ALL PRIVILEGES` |

*\* Note: Runtime immutability triggers (`trg_receipt_immutability` and `trg_notice_immutability`) further guarantee append-only integrity for receipts and published notices.*

### 3.3 Proof of Privilege Defense

1. **Outbox Claim Defense:** If a compromised API service attempts an `UPDATE consent_outbox_events` query to claim or mark an event, PostgreSQL aborts with `SQLSTATE 42501 (permission denied for table consent_outbox_events)`.
2. **Consent Tampering Defense:** If a compromised worker process attempts an `INSERT INTO consents` query, PostgreSQL aborts with `SQLSTATE 42501 (permission denied for table consents)`.
3. **DDL Injection Defense:** If SQL injection occurs on either service, `CREATE TABLE`, `DROP TABLE`, or `ALTER TABLE` commands fail because `CREATE ON SCHEMA public` is revoked from both roles.

---

## 4. NATS JetStream Authorization & Multi-Tenant Subject Isolation

### 4.1 Tenant-Scoped Subject Hierarchy

NVUTO publishes CloudEvents to a hierarchical subject hierarchy formatted as:
```
nvuto.consent.outbox.{tenant_id}.{event_type}
```
For example:
- `nvuto.consent.outbox.tenant-us-retail.consent.granted`
- `nvuto.consent.outbox.tenant-eu-health.consent.withdrawn`

### 4.2 JetStream Stream Scoping
The cluster stream `NVUTO_CONSENT` is configured to ingest all outbox subjects:
- **Stream Name:** `NVUTO_CONSENT`
- **Stream Ingestion Subject Pattern:** `nvuto.consent.outbox.>`
- **Storage:** File-backed (`nats.FileStorage`)
- **Retention:** Limits policy with 3 cluster replicas
- **Deduplication Window:** 10 minutes (`nats.Duplicates(10 * time.Minute)`)

### 4.3 NATS Server Configuration (`auth.conf`)

NATS JetStream accounts and users enforce strict subject boundary segregation:

```hcl
# /etc/nats/auth.conf

accounts {
  # 1. Operational Service Account (nvuto-platform)
  NVUTO_SYSTEM {
    jetstream: enabled
    users [
      {
        # Outbox Worker Publishing Identity
        user: "consent-outbox-worker"
        # NKey seed or certificate authentication
        nkey: "UDWORKER7EXAMPLEPUBKEYXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
        permissions {
          publish {
            # Worker can publish to any tenant outbox partition
            allow: ["nvuto.consent.outbox.>"]
          }
          subscribe {
            # Worker does not consume business events
            deny: ["nvuto.consent.>"]
          }
        }
      }
    ]
  }

  # 2. Isolated Tenant Account: ACME Retail
  TENANT_ACME {
    jetstream: enabled
    users [
      {
        user: "subscriber-acme"
        # Bound to tenant credential bundle (.creds)
        permissions {
          publish {
            deny: [">"] # Downstream subscribers are strictly read-only
          }
          subscribe {
            # Strictly bounded to this tenant's subject tree
            allow: [
              "nvuto.consent.outbox.tenant-acme.>",
              "$JS.API.CONSUMER.>",
              "$JS.API.STREAM.INFO.NVUTO_CONSENT"
            ]
            deny: [
              "nvuto.consent.outbox.*.>",  # Block cross-tenant wildcard snooping
              "nvuto.consent.outbox.tenant-globex.>"
            ]
          }
        }
      }
    ]
  }

  # 3. Isolated Tenant Account: Globex Pharma
  TENANT_GLOBEX {
    jetstream: enabled
    users [
      {
        user: "subscriber-globex"
        permissions {
          publish {
            deny: [">"]
          }
          subscribe {
            allow: [
              "nvuto.consent.outbox.tenant-globex.>",
              "$JS.API.CONSUMER.>",
              "$JS.API.STREAM.INFO.NVUTO_CONSENT"
            ]
            deny: [
              "nvuto.consent.outbox.tenant-acme.>"
            ]
          }
        }
      }
    ]
  }
}
```

### 4.4 Tenant Credential Provisioning Runbook

To provision a new tenant subscriber credential:

```bash
# 1. Create a secure NKey seed pair for the tenant
nk -gen user -pubout > tenant-acme.nkey

# 2. Generate a scoped user JWT with NSC CLI
nsc add user \
  --account TENANT_ACME \
  --name subscriber-acme \
  --deny-pub ">" \
  --allow-sub "nvuto.consent.outbox.tenant-acme.>" \
  --allow-sub "$JS.API.CONSUMER.>"

# 3. Export the combined user .creds bundle
nsc generate creds \
  --account TENANT_ACME \
  --name subscriber-acme > tenant-acme.creds

# 4. Deliver tenant-acme.creds securely to the tenant's secret vault
```

---

## 5. Verification & Acceptance Testing (AT-019)

Acceptance testing for T-019 is automated in:
- [internal/store/postgres_security_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/store/postgres_security_test.go)
- [internal/auth/auth_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/internal/auth/auth_test.go)
- [cmd/server/config_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/cmd/server/config_test.go)
- [cmd/outbox-worker/main_test.go](file:///c:/antigravity/scratch/Nvuto-Consent/cmd/outbox-worker/main_test.go)

### 5.1 Automated Test Execution Command

```bash
go test -v -count=1 \
  ./internal/auth/... \
  ./cmd/server/... \
  ./cmd/outbox-worker/... \
  ./internal/store -run "TestSecurity|TestDatabaseTLS|TestConfigureNATS"
```

### 5.2 Test Matrix Checklist

- [x] **JWKS Plaintext Rejection:** Asserts `JWT_JWKS_URL=http://...` causes `auth.ValidateConfig()` to return an error when `ALLOW_DEV_HEADERS=false`.
- [x] **Database `sslmode=disable` Rejection:** Asserts `DATABASE_URL=...?sslmode=disable` causes server and worker startup validation to fail when `ALLOW_DEV_HEADERS=false`.
- [x] **Database Production TLS Requirement:** Asserts `DATABASE_URL` lacking `sslmode=verify-full` or `sslmode=require` fails startup in production.
- [x] **NATS TLS Scheme Enforcement:** Asserts `NATS_URL=nats://...` fails worker startup when `ALLOW_INSECURE_NATS=false`.
- [x] **API Role Privilege Boundary:** Connected as `consent_api_role`, executing `UPDATE consent_outbox_events` is blocked with PostgreSQL error `42501`.
- [x] **Worker Role Privilege Boundary:** Connected as `consent_worker_role`, executing `INSERT INTO consents` is blocked with PostgreSQL error `42501`.
- [x] **Schema DDL Lockdown:** Connected as `consent_api_role` or `consent_worker_role`, executing `CREATE TABLE` is blocked with PostgreSQL error `42501`.
