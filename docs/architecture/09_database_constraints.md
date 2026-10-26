# Database Constraints, Tenant Integrity & Application/Storage Invariant Boundaries

**Document ID:** `DOC-ARCH-009`  
**Task Reference:** `T-028` | **Finding:** `F-028` | **Gate:** `G-028` | **Acceptance Test:** `AT-028`  
**Classification:** Internal Architectural Specification  
**Status:** Approved for Production  

---

## 1. Executive Summary

This architecture specification documents the schema integrity invariants enforced directly by **YugabyteDB (YSQL)** versus those deferred to the **Application Layer** (the Nvuto Go Service).

Under finding **F-028**, database relationships must provide defense-in-depth against data corruption, cross-tenant data leakage, and invalid lifecycle transitions. Even if an application bug, rogue query, or administrative migration script attempts an invalid write, the database schema actively rejects cross-tenant references and illegal state shapes via declarative constraints (`FOREIGN KEY`, `CHECK`, and `UNIQUE`).

---

## 2. Invariant Division Matrix: Database vs. Application Layer

The following matrix delineates the boundary between database-enforced guarantees and application-governed logic:

| Invariant Category | Invariant Rule | Enforced By | Mechanism / Constraint |
| :--- | :--- | :--- | :--- |
| **Tenant Isolation** | Foreign keys across internal entities must stay within the same tenant. | **Database (YSQL)** | Composite Foreign Keys: `(tenant_id, consent_id) REFERENCES consents(tenant_id, id)` on both `consent_receipts` and `consent_idempotency`. |
| **Tenant Isolation** | Data partitioning by tenant ID. | **Both** | Composite primary/unique keys in DB; tenant scoping mandatory in all SQL queries (`WHERE tenant_id = $1`). |
| **Lifecycle Status** | `status` must be one of the canonical values. | **Database (YSQL)** | `CHECK (status IN ('granted', 'denied', 'withdrawn', 'expired', 'pending_confirmation'))` (`chk_consents_status_valid`). |
| **Temporal Logic** | `expires_at` must be strictly after `created_at`. | **Database (YSQL)** | `CHECK (expires_at IS NULL OR expires_at > created_at)` (`chk_consents_temporal_expiry`). |
| **Temporal Logic** | `withdrawn_at` cannot precede `created_at`. | **Database (YSQL)** | `CHECK (withdrawn_at IS NULL OR withdrawn_at >= created_at)` (`chk_consents_temporal_withdrawn`). |
| **Temporal Logic** | `updated_at` cannot precede `created_at`. | **Database (YSQL)** | `CHECK (updated_at >= created_at)` (`chk_consents_temporal_updated`). |
| **Suppression Shape** | `scope` determines required purpose/channel fields. | **Database (YSQL)** | `CHECK ((scope = 'global' AND purpose = '' AND channel = '') OR (scope = 'channel' AND purpose = '' AND channel <> '') OR (scope = 'purpose_channel' AND purpose <> '' AND channel <> ''))`. |
| **Record Immutability** | Cryptographic receipts, audit logs, and notices cannot be updated or deleted. | **Database (YSQL)** | Database triggers: `BEFORE UPDATE OR DELETE ... EXECUTE FUNCTION reject_consent_immutable_change()`. |
| **Cryptographic Evidence** | Receipt hashes must match canonical SHA-256 computation over ordered DTO fields. | **Application** | `service.VerifyReceipt()` hashes canonical JSON representation; DB only enforces `length(receipt_hash) > 0`. |
| **Policy Registry Rules** | Lawful basis evidence requirements and double opt-in triggers. | **Application** | `service.Service.RecordGoverned` verifies purpose and lawful basis active status and channel permissions before writing. |
| **JSON Schema Depth** | Deep validation of JSON evidence properties (`method`, `artifact_hash`, `collected_at`). | **Application** | Go struct validation (`domain.Evidence`); DB ensures `jsonb_typeof(evidence) = 'object'` and `jsonb_typeof(scope) IN ('object', 'null')`. |
| **Subject Identity** | Validation of customer subject IDs (`subject_id`) and actor IDs (`actor_id`). | **Application** | Identity Provider (JWT/OIDC claims); no foreign keys in database. |

---

## 3. Why External References (Subject & Actor IDs) Do Not Have Foreign Keys

In relational design, natural foreign keys are common. However, `subject_id` (the end-customer whose consent is recorded) and `actor_id` (the user or service principal performing the action) **intentionally do not have database foreign keys** in the Nvuto Consent schema:

### 3.1 Decoupled Identity Architecture
Nvuto Consent is an independent microservice responsible for consent lifecycle management. Customer identities (`subject_id`) reside in external Identity Providers (IdPs) or Customer Relationship Management (CRM) systems (e.g. Auth0, Okta, Keycloak, Ping, or internal user databases). Actor identities (`actor_id`) reside in corporate directory services. Introducing a foreign key would require tight database-level coupling to an external identity table, violating microservice boundaries and preventing multi-tenant SaaS adoption.

### 3.2 Right to Erasure & GDPR Article 17 (Data Minimization)
Under GDPR Art. 17 and CCPA, a customer may exercise their Right to Be Forgotten in an upstream identity service. If Nvuto Consent had a strict database foreign key referencing a `users` table:
- Deleting the user in the upstream table would either fail (referential integrity block) or cascade delete the consent record (`ON DELETE CASCADE`).
- Deleting the consent record or audit log destroys the legally required proof of compliance (GDPR Art. 7(1): *"the controller shall be able to demonstrate that the data subject has consented"*).
- By maintaining `subject_id` as a decoupled text identifier, upstream identity systems can pseudonymize or delete user profiles while the consent service preserves the immutable compliance trail.

### 3.3 Zero Cross-Database Distributed Transactions
In a cloud-native architecture, user identities are managed across distributed services. Imposing foreign keys would require cross-database distributed joins and locking, severely degrading write throughput and creating single points of failure.

---

## 4. YSQL Constraint Catalog (Migration 011)

Migration `011_schema_integrity_constraints.sql` introduces the following constraints:

### 4.1 Composite Unique Key
```sql
ALTER TABLE consents 
  ADD CONSTRAINT uq_consents_tenant_id UNIQUE (tenant_id, id);
```
- **Rationale:** Acts as the parent key for all composite foreign keys. Guarantees that `(tenant_id, id)` is globally unique within the table.

### 4.2 Composite Tenant Foreign Keys
```sql
-- Link consent_receipts to parent consent within the same tenant
ALTER TABLE consent_receipts
  ADD CONSTRAINT fk_consent_receipts_consent
  FOREIGN KEY (tenant_id, consent_id) REFERENCES consents(tenant_id, id);

-- Link consent_idempotency to parent consent within the same tenant
ALTER TABLE consent_idempotency
  ADD CONSTRAINT fk_consent_idempotency_consent
  FOREIGN KEY (tenant_id, consent_id) REFERENCES consents(tenant_id, id);
```
- **Rationale:** Prevents cross-tenant pointer errors. If Tenant B attempts to insert a receipt or idempotency record referencing a consent belonging to Tenant A, PostgreSQL / YSQL immediately aborts with SQLSTATE `23503` (`foreign_key_violation`).

### 4.3 Temporal and Status Check Constraints
```sql
-- Expiry must be after creation
ALTER TABLE consents
  ADD CONSTRAINT chk_consents_temporal_expiry
  CHECK (expires_at IS NULL OR expires_at > created_at);

-- Withdrawal cannot precede creation
ALTER TABLE consents
  ADD CONSTRAINT chk_consents_temporal_withdrawn
  CHECK (withdrawn_at IS NULL OR withdrawn_at >= created_at);

-- Valid status values
ALTER TABLE consents
  ADD CONSTRAINT chk_consents_status_valid
  CHECK (status IN ('granted', 'denied', 'withdrawn', 'expired', 'pending_confirmation'));
```
- **Rationale:** Defends against invalid time-travel or corrupted records injected by manual SQL commands or faulty external integrations.

---

## 5. Pre-Enforcement Data Validation (`scripts/validate_integrity.sql`)

Before applying migration 011 in an existing environment, operators execute `scripts/validate_integrity.sql`:

```bash
docker exec -i yugabyte-node bin/ysqlsh -U yugabyte -d consent < scripts/validate_integrity.sql
```

The script runs in a `READ ONLY` transaction and reports a summary census across 13 integrity checks. Migration 011 should only proceed if all violation counts return `0`.
