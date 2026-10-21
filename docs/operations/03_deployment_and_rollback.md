# Deployment, Release Packaging, and Rollback Runbook (T-018 / AT-018)

## 1. Overview & Architecture

Nvuto-Consent employs declarative **Kustomize** environment overlays combined with **immutable container image digests** (`@sha256:...`) pushed to GitHub Container Registry (GHCR).

```
                      +-------------------+
                      |   deploy/base/    |
                      | (API & Worker K8s)|
                      +---------+---------+
                                |
             +------------------+------------------+
             |                                     |
+------------v------------+           +------------v------------+
| deploy/overlays/staging |           |deploy/overlays/production|
| - config.env (staging)  |           | - config.env (prod)     |
| - secrets.env (staging) |           | - secrets.env (prod)    |
| - @sha256:staging-digest|           | - @sha256:prod-digest   |
| - replicas: 2 (API)     |           | - replicas: 3 (API)     |
+-------------------------+           +-------------------------+
```

### Key Principles
1. **Zero Silent Grandfathering**: Every deployment pins exact image digests and config snapshots.
2. **Immutable Digests**: No `:latest` or mutable tags in deployment manifests. All containers run pinned `@sha256:...` digests.
3. **Fail-Closed Secret Validation**: Kustomize `secretGenerator` strictly checks for `secrets.env` and `nats.creds`. If required configuration files are missing, manifest generation fails immediately prior to rollout.
4. **Additive Database Schema Guarantee**: Database schema migrations are strictly forward-only and additive. Rollbacks never downgrade the database; rollbacks strictly revert container image digests to a forward-compatible release.

---

## 2. Platform Infrastructure Dependencies

Before applying Nvuto-Consent manifests, the platform operator must provision and verify the following external infrastructure:

| Component | Minimum Specification | Verification Command / Check |
| :--- | :--- | :--- |
| **YugabyteDB (YSQL)** | Distributed SQL cluster (v2024.1+) across 3 Availability Zones. Database `consent` with TLS enabled (`sslmode=verify-full`). | `ysqlsh -h yb-tserver -U consent_user -d consent -c "SELECT count(*) FROM yb_servers();"` |
| **NATS JetStream** | Clustered JetStream 2.10+ (3 replicas). Stream `NVUTO_CONSENT` covering `nvuto.consent.>`. | `nats stream info NVUTO_CONSENT` |
| **OIDC / Authentik** | OIDC Provider issuing asymmetric RS256/ES256 JWTs with JWKS endpoint reachable by the pods. | `curl -fsS https://auth.nvuto.com/application/o/consent/jwks/` |
| **Ingress Controller** | Ingress controller (e.g. `ingress-nginx`) with TLS certificate from Cert-Manager (`letsencrypt-prod`). Max client body: `1m`. | `kubectl get ingressclass` |
| **Pod Security** | Kubernetes namespace labeled `pod-security.kubernetes.io/enforce: restricted`. | `kubectl get ns nvuto-consent --show-labels` |

---

## 3. Configuration & Secret Management

### 3.1 Non-Sensitive Configuration (`config.env`)
Configured in `deploy/overlays/{environment}/config.env`:
- `authentik-jwks-url`: Public JWKS keys URL.
- `authentik-issuer`: Valid token issuer URL.
- `authentik-audience`: Required audience claim in JWT.
- `nats-url`: Cluster-internal NATS endpoint.
- `nats-stream`: Stream name (`NVUTO_CONSENT`).
- `nats-subject-prefix`: Outbox event prefix (`nvuto.consent`).
- `nats-replicas`: NATS JetStream replica count (`3`).

### 3.2 Secret Configuration (`secrets.env` & `nats.creds`)
Operators must populate these files from the provided `.example` templates prior to building manifests:

```bash
# In staging:
cp deploy/overlays/staging/secrets.env.example deploy/overlays/staging/secrets.env
cp deploy/overlays/staging/nats.creds.example deploy/overlays/staging/nats.creds

# In production:
cp deploy/overlays/production/secrets.env.example deploy/overlays/production/secrets.env
cp deploy/overlays/production/nats.creds.example deploy/overlays/production/nats.creds
```

> [!WARNING]
> Never commit `secrets.env` or `nats.creds` to version control. They are strictly ignored in `.gitignore`. In automated GitOps pipelines (ArgoCD / Flux), inject secrets via **SealedSecrets**, **ExternalSecrets Operator**, or **HashiCorp Vault**.

---

## 4. Promotion & Deployment Workflow

### 4.1 Automated CI/CD Release Flow
1. **Validation**: CI runs `go test -race ./...`, vulnerability scanning, and real YugabyteDB integration tests.
2. **Container Build**: Multi-stage Docker builds create minimal distroless images for both `consent-api` and `consent-worker`.
3. **Registry Publication**: Images are pushed to GitHub Container Registry (`ghcr.io/atlasfsp/nvuto-consent-api` and `consent-worker`).
4. **Digest Pinning**: The CI pipeline extracts the immutable SHA256 digest and updates the overlay:

```bash
cd deploy/overlays/staging
kustomize edit set image consent-api=ghcr.io/atlasfsp/nvuto-consent-api@sha256:${API_DIGEST}
kustomize edit set image consent-worker=ghcr.io/atlasfsp/nvuto-consent-worker@sha256:${WORKER_DIGEST}
```

### 4.2 Applying Deployment to Kubernetes

```bash
# 1. Preview rendered manifest
kustomize build deploy/overlays/staging

# 2. Apply rolling update
kubectl apply -k deploy/overlays/staging

# 3. Monitor rollout status
kubectl rollout status deployment/consent-api -n nvuto-consent-staging --timeout=120s
kubectl rollout status deployment/consent-outbox-worker -n nvuto-consent-staging --timeout=120s

# 4. Verify probes and readiness
kubectl get pods -n nvuto-consent-staging -l app=consent-api
curl -fsS http://<INGRESS_OR_SERVICE_IP>/readyz
```

---

## 5. Rollback Procedure

If an application regression, performance degradation, or deployment failure occurs, execute the following procedure immediately.

### 5.1 Step 1: Identify the Known-Good Prior Digest
Retrieve the previous deployment's image digest from git history or cluster deployment history:

```bash
# Check currently running digest
kubectl get deployment consent-api -n nvuto-consent -o jsonpath='{.spec.template.spec.containers[0].image}'

# Inspect previous git commit on main or release tag
git log -n 5 --oneline deploy/overlays/production/kustomization.yaml
```

Example previous known-good digests:
- API: `ghcr.io/atlasfsp/nvuto-consent-api@sha256:7f9a12bc44...`
- Worker: `ghcr.io/atlasfsp/nvuto-consent-worker@sha256:1a8b34de55...`

### 5.2 Step 2: Pin the Prior Digest in Kustomize

```bash
cd deploy/overlays/production

# Revert to prior known-good digests
kustomize edit set image consent-api=ghcr.io/atlasfsp/nvuto-consent-api@sha256:7f9a12bc44...
kustomize edit set image consent-worker=ghcr.io/atlasfsp/nvuto-consent-worker@sha256:1a8b34de55...

cd ../../..
```

### 5.3 Step 3: Apply the Rollback Manifest

```bash
# Apply the reconciled overlay
kubectl apply -k deploy/overlays/production

# Alternatively, trigger rapid rollout undo via kubectl:
kubectl rollout undo deployment/consent-api -n nvuto-consent
kubectl rollout undo deployment/consent-outbox-worker -n nvuto-consent
```

### 5.4 Step 4: Verify Rollback Health

```bash
# Check rollout progress
kubectl rollout status deployment/consent-api -n nvuto-consent
kubectl rollout status deployment/consent-outbox-worker -n nvuto-consent

# Check health and readiness probes
kubectl get pods -n nvuto-consent
kubectl logs -n nvuto-consent -l app=consent-api --tail=50
```

---

## 6. The Additive Database Schema Compatibility Rule

> [!CRITICAL]
> **Database downgrades (`DROP COLUMN`, `DROP TABLE`, or rolling back migrations) are STRICTLY FORBIDDEN in production.**

### Why Database Downgrades Are Prohibited:
1. **Durable Legal Proof**: Consent decisions and receipts created during the incident must remain permanently auditable. Downgrading the schema would corrupt or delete legally binding evidence.
2. **Distributed Transaction Safety**: In YugabyteDB, rolling back DDL across a multi-node cluster risks schema caching conflicts and transaction serialization deadlocks.

### Forward-Compatible Rollback Invariant:
All schema migrations in `migrations/` follow the **Expand/Contract** design:
- New columns are added as `NULL` or with safe defaults.
- Foreign keys and constraints are additive.
- New tables (e.g. `consent_receipts_v3`, audit extensions) are isolated.

**Therefore**: Any prior application version released within the same major schema cycle can safely run against a newer, additive database schema without crashing or rejecting valid operations.

### Schema Compatibility Matrix

| Migration Version | Description | Compatible Rollback Images |
| :--- | :--- | :--- |
| `001_init.sql` | Baseline tables (`consents`, `suppressions`, `outbox`) | All v1.0+ images |
| `002_policy_governance.sql` | Policy registries, categories, channels | All v1.1+ images |
| `003_governed_idempotency.sql` | Idempotency key tracking table | All v1.2+ images |
| `004_preference_generations.sql` | `preference_generation` monotonic version | All v1.3+ images |
| `005_outbox_claim_tokens.sql` | Lease fencing & worker claim tokens | All v1.4+ images |
| `006_immutable_receipt_hash_v2.sql`| Receipt v2 canonical hash | All v1.5+ images |
| `007_protected_suppressions.sql` | `created_by_role` & administrative flags | All v1.6+ images |
| `008_receipt_evidence_binding.sql` | Full decision receipt binding & notice hash | All v1.7+ images |

When rolling back the application, ensure the selected image digest belongs to a version compatible with the current migration level shown above.
