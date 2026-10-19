# Nvuto Consent

Shared privacy, communication preference, lawful-basis and consent service for Nvuto Customer Platform.

Owns: consent records, purpose/channel preferences, lawful-basis references, effective dates, proof/evidence, withdrawal, suppression, regional policy, data-subject preference APIs, and consent-change events.

Does not own: customer identity, campaign execution, conversations, legal contracts, or analytics data itself.

Primary consumers: Engage, Service, Contact Center, Customer Portal, CRM, Conversations, AI, Integration Hub.

Functional standard: capture consent with evidence → evaluate purpose/channel decision deterministically → enforce effective/withdrawn state → expose self-service changes → propagate suppression/change events → audit immutable history. No outbound communication workflow may treat missing consent as granted where policy requires affirmative consent.

## Implemented functional slice

- Tenant-scoped purpose and lawful-basis registries, including service/marketing category separation, allowed channels, affirmative-consent requirements, and evidence requirements.
- Immutable jurisdictional privacy-notice versions and cryptographically hashed consent receipts containing the accepted notice and collection evidence.
- Governed grant/deny/withdraw/expiry decisions, global/channel/purpose-channel suppression precedence, batch evaluation, and single-use expiring double opt-in confirmation.
- Authentik-compatible RS256/JWKS JWT verification with issuer, audience, tenant, role, and canonical customer-subject enforcement. HMAC and header authentication are explicit development/test modes only.
- YugabyteDB YSQL transactions keep consent state, immutable audit, receipt, and versioned outbox events atomic. The HA-safe worker publishes CloudEvents to NATS JetStream with message-ID deduplication and bounded retry backoff.
- Three-replica API and two-replica worker Kubernetes baselines with disruption budgets and restricted pod security.

## API surface

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

## Runtime configuration

**YugabyteDB YSQL is the production transactional database for Nvuto Consent.** The service may use a PostgreSQL-compatible driver and connection URI because YSQL speaks the PostgreSQL wire protocol, but PostgreSQL is not the intended production datastore.

The API requires `DATABASE_URL` and production authentication via `JWT_JWKS_URL`, `JWT_ISSUER`, and `JWT_AUDIENCE`. HMAC requires the explicit `ALLOW_HMAC_AUTH=true` development opt-in; header authentication likewise requires `ALLOW_DEV_HEADERS=true`, which is never enabled in the deployment manifest. The outbox worker requires `DATABASE_URL`, `NATS_URL`, and a `NATS_CREDS_FILE`; insecure NATS also requires an explicit local-only opt-in. `NATS_STREAM`, `NATS_SUBJECT_PREFIX`, and the three-replica stream baseline are documented in the deployment.

Migrations are applied in numeric order. See [migrations/ROLLBACK.md](migrations/ROLLBACK.md) for the additive migration rollback strategy. CI boots a real YugabyteDB RF1 node, verifies `yb_servers()`, proves empty install and previous-schema upgrade, executes policy transactions and double-opt-in concurrency against YSQL, proves immutable receipts and persisted grant/evaluate/withdraw behavior, and proves concurrent JetStream outbox publication while using YugabyteDB as the durable ledger.

Production environments must use the target YugabyteDB universe's TLS, authentication, placement, backup/PITR and recovery policy. A green RF1 CI run is repository evidence, not multi-node Production GA evidence.
