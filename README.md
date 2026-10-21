# Nvuto Consent

Shared privacy, communication preference, lawful-basis and consent service for Nvuto Customer Platform.

Owns: consent records, purpose/channel preferences, lawful-basis references, effective dates, proof/evidence, withdrawal, suppression, regional policy, data-subject preference APIs, and consent-change events.

Does not own: customer identity, campaign execution, conversations, legal contracts, or analytics data itself.

Primary consumers: Engage, Service, Contact Center, Customer Portal, CRM, Conversations, AI, Integration Hub.

Functional standard: capture consent with evidence → evaluate purpose/channel decision deterministically → enforce effective/withdrawn state → expose self-service changes → propagate suppression/change events → audit immutable history. No outbound communication workflow may treat missing consent as granted where policy requires affirmative consent.

## Implemented Functional Slice

- Tenant-scoped purpose and lawful-basis registries, including service/marketing category separation, allowed channels, affirmative-consent requirements, and evidence requirements.
- Immutable jurisdictional privacy-notice versions and cryptographically hashed consent receipts containing the accepted notice and collection evidence.
- Governed grant/deny/withdraw/expiry decisions, global/channel/purpose-channel suppression precedence, batch evaluation, and single-use expiring double opt-in confirmation.
- Authentik-compatible RS256/JWKS JWT verification with issuer, audience, tenant, role, and canonical customer-subject enforcement. HMAC and header authentication are explicit development/test modes only.
- YugabyteDB YSQL transactions keep consent state, immutable audit, receipt, and versioned outbox events atomic. The HA-safe worker publishes CloudEvents to NATS JetStream with message-ID deduplication and bounded retry backoff.
- Three-replica API and two-replica worker Kubernetes baselines with disruption budgets and restricted pod security.

## API Surface

Administrative registry writes:

- `POST /v1/purposes`
- `POST /v1/lawful-bases`
- `POST /v1/notices`

Preference and decision operations:

- `POST /v1/consents`
- `POST /v1/consents/{id}/withdraw`
- `POST /v1/suppressions`
- `POST /v1/evaluate`
- `POST /v1/evaluate:batch` (maximum 100 decisions)
- `POST /v1/double-opt-in`
- `POST /v1/double-opt-in/confirm`
- `GET /v1/receipts/{id}`

`subject_id` is the canonical Customer Graph party identifier. A `self_service` JWT must carry a matching `customer_subject_id`; service and administrative roles remain tenant-scoped.

## Observability and Telemetry (T-021)

The API service (`cmd/server`) and the Outbox Worker (`cmd/outbox-worker`) implement structured JSON logging using Go standard library `log/slog`.

### Correlation ID & Context Propagation
- Every incoming HTTP request is assigned a server-generated correlation ID (or adopts the incoming `X-Correlation-ID` header), passed throughout the request context and included in all structured log lines and error responses.
- Handler failures, input rejections, panics, and readiness failures are logged as structured JSON with attributes: `correlation_id`, `method`, `path`, `status`, `kind`, `code`, and sanitized `cause`.

### Worker Telemetry
The outbox worker logs critical batch and lifecycle events in structured JSON:
- `claim_succeeded` / `claim_failed`: Number of rows claimed, lease duration, or error reason.
- `publish_succeeded` / `publish_failed`: Event ID, tenant ID, event type, aggregate ID, attempt count, and error description.
- `complete_succeeded` / `complete_refused` / `complete_failed`: Completion confirmation and lease state.
- `retry_scheduled` / `retry_refused` / `retry_failed`: Fenced exponential backoff scheduling.
- `release_completed` / `release_deadline_reached`: Uncommitted row releases during shutdown or lease expiry.
- `batch_completed`: Number of claimed vs processed records and cycle duration.

### Security and Privacy Boundaries
- **No Token Logging:** Bearer tokens, confirmation tokens, HMAC secrets, and outbox claim tokens are strictly excluded from logs.
- **No Sensitive Evidence Logging:** Evidence payloads, secret fields, and evidence hashes are never logged.
- **Sanitized Errors:** Error causes are sanitized to strip unprintable/control characters and bounded to prevent log injection or denial-of-service.

## Runtime Configuration

**YugabyteDB YSQL is the production transactional database for Nvuto Consent.** The service uses a PostgreSQL-compatible driver and connection URI because YSQL speaks the PostgreSQL wire protocol, but PostgreSQL is not the intended production datastore.

### Environment Variables

#### API Service (`cmd/server`)

| Variable | Description | Default | Allowed Values / Format | Required |
|---|---|---|---|---|
| `DATABASE_URL` | YSQL connection string (e.g., `postgresql://user:pass@host:5433/db?sslmode=verify-full`) | None | Valid PostgreSQL/YSQL URI | Required (unless `ALLOW_MEMORY_STORE=true`) |
| `ALLOW_MEMORY_STORE` | Opt-in to ephemeral in-memory storage (dev/testing only) | `false` | `true`, `false` | Optional |
| `REQUEST_TIMEOUT` | Individual HTTP request execution deadline before typed 503 | `10s` | Go duration string (e.g., `5s`, `10s`, `30s`) | Optional |
| `JWT_JWKS_URL` | Authentik / OIDC JWKS endpoint for RS256 token verification | None | Valid HTTP/HTTPS URL | Required in Production |
| `JWT_ISSUER` | Expected JWT `iss` claim | None | Non-empty string | Required if `JWT_JWKS_URL` set |
| `JWT_AUDIENCE` | Expected JWT `aud` claim | None | Non-empty string | Required if `JWT_JWKS_URL` set |
| `ALLOW_HMAC_AUTH` | Opt-in to HS256 HMAC authentication (dev/testing only) | `false` | `true`, `false` | Optional |
| `JWT_HMAC_SECRET` | Shared secret for HS256 verification (requires `ALLOW_HMAC_AUTH=true`) | None | Non-empty string | Optional |
| `ALLOW_DEV_HEADERS` | Opt-in to `X-Nvuto-Tenant-ID` / `X-Nvuto-Actor-ID` headers without tokens | `false` | `true`, `false` | Optional (dev only) |

#### Outbox Worker (`cmd/outbox-worker`)

| Variable | Description | Default | Allowed Values / Format | Required |
|---|---|---|---|---|
| `DATABASE_URL` | YSQL connection string for outbox claiming and fencing | None | Valid PostgreSQL/YSQL URI | Required |
| `NATS_URL` | NATS JetStream server connection URL | None | Valid NATS URI (e.g., `nats://host:4222`) | Required |
| `NATS_STREAM` | JetStream stream name for outbox events | `NVUTO_CONSENT` | Non-empty string | Optional |
| `NATS_SUBJECT_PREFIX` | Subject prefix for published CloudEvents | `nvuto.consent` | Non-empty string | Optional |
| `NATS_REPLICAS` | Number of JetStream stream replicas | `3` | Integer between `1` and `5` | Optional |
| `NATS_CREDS_FILE` | Path to NATS user credentials file | None | Valid file path | Required (unless `ALLOW_INSECURE_NATS=true`) |
| `ALLOW_INSECURE_NATS` | Opt-in to unauthenticated NATS connection (local dev only) | `false` | `true`, `false` | Optional |
| `NATS_TLS_CA_FILE` | Path to custom Root CA certificate for NATS TLS connection | None | Valid file path | Optional |
| `OUTBOX_SHUTDOWN_GRACE` | Grace period to drain publishes before releasing remaining claims | `30s` | Duration between `1s` and `5m` | Optional |
| `OUTBOX_PUBLISH_TIMEOUT` | Batch publish timeout budget before releasing remaining claims | `15s` | Duration between `1s` and `1m` | Optional |

## Release Packaging & Deployment (T-018)

Nvuto-Consent provides declarative **Kustomize** deployment overlays with **immutable container image digests** (`@sha256:...`) pushed to GitHub Container Registry (GHCR).

Detailed deployment, rollback, and platform dependency operations are documented in the [Deployment and Rollback Runbook](docs/operations/03_deployment_and_rollback.md).

### Kustomize Directory Layout

```
deploy/
├── base/                              # Shared base manifests
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── api.yaml                       # API Deployment, Service, PDB, HPA
│   └── worker.yaml                    # Worker Deployment, PDB
└── overlays/                          # Environment-specific overlays
    ├── staging/
    │   ├── kustomization.yaml
    │   ├── config.env                 # Non-sensitive runtime configuration
    │   ├── secrets.env.example        # Secret template (DATABASE_URL, JWT secret)
    │   ├── nats.creds.example         # NATS user credentials template
    │   └── patches/replicas.yaml      # Staging scaling limits (API: 2, Worker: 1)
    └── production/
        ├── kustomization.yaml
        ├── config.env                 # Production runtime configuration
        ├── secrets.env.example
        ├── nats.creds.example
        └── patches/replicas.yaml      # Production scaling limits (API: 3, Worker: 2)
```

### Immutable Image Digests (Mandatory for Production)

Production deployments **must** specify immutable image digests (SHA-256) rather than mutable `:latest` or floating release tags. 

Using mutable tags in production introduces deployment drift, indeterminate rollback states, inconsistent image versions across horizontally autoscaled nodes, and container supply-chain vulnerabilities.

#### Pinned Overlays & Automated CI Promotion

During CI release builds, `kustomize edit set image` updates the target overlay with the immutable SHA256 digest:

```bash
cd deploy/overlays/staging
kustomize edit set image consent-api=ghcr.io/atlasfsp/nvuto-consent-api@sha256:${API_DIGEST}
kustomize edit set image consent-worker=ghcr.io/atlasfsp/nvuto-consent-worker@sha256:${WORKER_DIGEST}
```

#### Validating Manifests Locally

```bash
# 1. Provide environment credentials from templates (fail-closed if missing)
cp deploy/overlays/staging/secrets.env.example deploy/overlays/staging/secrets.env
cp deploy/overlays/staging/nats.creds.example deploy/overlays/staging/nats.creds

# 2. Render and inspect validated manifests
kustomize build deploy/overlays/staging
```

## Database Migrations

Database migrations are strictly additive and backward-compatible (expand/contract pattern). They must be executed in numerical sequence against the target YSQL universe before deploying updated application binaries.

### Applied Schema Scripts

1. `001_init.sql` — Initial schema establishing tenants, purposes, lawful bases, privacy notices, consents, and outbox tables.
2. `002_policy_governance.sql` — Notice versioning, immutable receipts, double-opt-in challenge tracking, and suppressions.
3. `003_governed_idempotency.sql` — Idempotency key tracking table with payload hashing.
4. `004_preference_generation_receipt_v2.sql` — Preference generation state (`consent_preference_state`), monotonic generation counter on consents, and SHA-256 canonical receipt hashing v2.
5. `005_outbox_fencing.sql` — Outbox worker fencing with `claim_token` UUID leases and the `consent_outbox_fence_untokened_writes` trigger to prevent split-brain delivery.
6. `006_schema_hardening_and_audit.sql` — Schema constraints hardening:
   - Composite foreign keys on `consent_receipts` and `consent_idempotency` enforcing tenant isolation.
   - `CHECK` constraints on `consent_suppressions` validating scope shapes (global, channel, purpose-channel).
   - Audit event reconstruction fields (`resource_type`, `revision`, `changes`) and idempotency response snapshots.

### Applying Migrations

Execute all migration scripts using standard `psql` (or YSQL CLI) ensuring `ON_ERROR_STOP=1`:

```bash
#!/usr/bin/env bash
set -euo pipefail

echo "Applying migrations to: ${DATABASE_URL}"
for file in $(ls migrations/*.sql | sort -V); do
  echo "Executing ${file}..."
  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -f "${file}"
done
echo "All migrations applied successfully."
```

### Rollback Strategy

Refer to [migrations/ROLLBACK.md](migrations/ROLLBACK.md) for incident recovery procedures. Because all migrations (`001` through `006`) are additive:
- **Application Rollbacks:** Reverting application binaries to a prior compatible image digest is schema-safe. Pre-006 binaries continue operating correctly against the hardened schema.
- **Never Drop Tables or Columns During Incidents:** Dropping columns or triggers destroys compliance evidence and can cause lease errors. Forward corrective migrations must be used if schema alterations are required.
