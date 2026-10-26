# Production YugabyteDB Backup, Restore, PITR, and Retention Runbook

**Document ID:** `DOC-OPS-009`  
**Task Reference:** `T-022` | **Finding:** `F-022` | **Gate:** `G-022` | **Acceptance Test:** `AT-022`  
**Classification:** Internal Operational Runbook & Disaster Recovery (DR) Policy  
**Status:** Approved for Production  

---

## 1. Executive Summary & Recovery Objectives

This operational runbook defines the disaster recovery (DR), backup, point-in-time recovery (PITR), retention, and post-restore event reconciliation standards for the **Nvuto Consent Service** on **YugabyteDB (YSQL)**.

Under data protection regulations (GDPR Article 32, NDPR Section 2.1) and enterprise availability contracts, consent and privacy management systems require strict recovery guarantees:

| Metric | Production Target | Operational Guarantee & Mechanism |
| :--- | :--- | :--- |
| **Recovery Point Objective (RPO)** | **$\le 15$ minutes** (Snapshot)<br/>**$< 1$ minute** (PITR) | Distributed YugabyteDB snapshots taken every 15 minutes. Continuous Raft-based WAL replication and Point-In-Time Recovery (PITR) window enables recovery to any microsecond within the retention window. |
| **Recovery Time Objective (RTO)** | **$\le 60$ minutes** (Full Regional Failover)<br/>**$\le 15$ minutes** (In-Region Point Restore) | Automated orchestration using `yb-admin` and declarative Kubernetes manifests. Restores database state, re-synchronizes outbox offsets, and restarts API services. |
| **Reconciliation Invariant** | **Zero Consent Resurrection** | Restoring a prior database image ($T-1$) where a consent was "granted" **never** resurrects permission downstream if the subject withdrew consent prior to the crash ($T-0$). Guaranteed by the downstream `AggregateRevision` idempotency contract. |
| **Audit Immutability Invariant** | **Append-Only Preservation** | Database triggers prevent deletion or unauthorized modification of audit trails, while supporting GDPR/NDPR lawful erasure through audited payload redaction. |

---

## 2. YugabyteDB Distributed Snapshot & PITR Procedures

YugabyteDB coordinates storage across distributed tablet servers using Raft consensus. Backups must be performed at the storage engine level via `yb-admin` distributed snapshots or logically via `ysql_dump`.

### 2.1 Distributed Snapshots via `yb-admin`

Distributed snapshots provide consistent point-in-time distributed snapshots across all tablets without locking database writes.

#### Step 1: Create a Distributed Database Snapshot
```bash
yb-admin -master_addresses "${YB_MASTER_ADDRESSES}" \
  create_database_snapshot ysql.consent
```
*Output: Snapshot UUID (e.g., `b817924e-3721-4d32-8df7-71c890123456`)*

#### Step 2: Export Snapshot Metadata to Object Storage
```bash
SNAPSHOT_ID="b817924e-3721-4d32-8df7-71c890123456"
yb-admin -master_addresses "${YB_MASTER_ADDRESSES}" \
  export_snapshot "${SNAPSHOT_ID}" "/mnt/backup/metadata_${SNAPSHOT_ID}.json"

# Transfer snapshot tablet files to encrypted cloud storage (S3 / GCS)
aws s3 sync /mnt/disk*/yb-data/tserver/data/rocksdb/table-*/snapshots/ \
  "s3://${BACKUP_BUCKET}/snapshots/${SNAPSHOT_ID}/" \
  --sse aws:kms \
  --sse-kms-key-id "${KMS_KEY_ARN}"
```

#### Step 3: Restore a Distributed Snapshot
```bash
# 1. Quiesce incoming API traffic
kubectl scale deployment nvuto-consent-api --replicas=0 -n nvuto-consent
kubectl scale deployment nvuto-consent-worker --replicas=0 -n nvuto-consent

# 2. Restore distributed snapshot
yb-admin -master_addresses "${YB_MASTER_ADDRESSES}" \
  restore_snapshot "${SNAPSHOT_ID}"

# 3. Verify restore completion
yb-admin -master_addresses "${YB_MASTER_ADDRESSES}" \
  list_snapshots
```

### 2.2 Point-In-Time Recovery (PITR)

YugabyteDB PITR allows rolling back a keyspace or database to an arbitrary timestamp with microsecond granularity within the retention window.

#### Step 1: Enable PITR Schedule (Infrastructure Setup)
```bash
# Create a snapshot schedule with 7-day retention and 15-minute interval
yb-admin -master_addresses "${YB_MASTER_ADDRESSES}" \
  create_snapshot_schedule ysql.consent 15 10080
```

#### Step 2: Execute Point-In-Time Recovery to Specific Timestamp
```bash
# Target recovery time formatted in microseconds UTC
RESTORE_TIME="2026-10-26 09:45:00.000000"

yb-admin -master_addresses "${YB_MASTER_ADDRESSES}" \
  restore_snapshot_schedule "${SCHEDULE_ID}" "${RESTORE_TIME}"
```

### 2.3 Logical Backup via `ysql_dump`

For cross-version upgrades, schema verification, and isolated sandbox testing:

```bash
# Consistent logical dump using PostgreSQL custom compressed format
PGPASSWORD="${DB_PASSWORD}" ysql_dump \
  -h "${DB_HOST}" \
  -p 5433 \
  -U yugabyte \
  -d consent \
  --format=custom \
  --blobs \
  --verbose \
  --file="/backups/consent_$(date +%Y%m%d_%H%M%S).dump"

# Restore from logical dump
PGPASSWORD="${DB_PASSWORD}" pg_restore \
  -h "${RESTORE_DB_HOST}" \
  -p 5433 \
  -U yugabyte \
  -d consent \
  --clean \
  --if-exists \
  --no-owner \
  --verbose \
  "/backups/consent_20261026_094500.dump"
```

---

## 3. Backup Retention, Encryption & Access Governance

### 3.1 Retention Tiering Schedule

To balance rapid disaster recovery against long-term compliance mandates (GDPR Art. 30, NDPR audit requirements):

| Tier | Type | Frequency | Retention Window | Storage Class | Immutability Protection |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Hot PITR** | WAL & Tablet Logs | Continuous | **7 Days** | NVMe / High-IOPS Cloud Disk | YB internal Raft WAL retention |
| **Warm Daily** | Distributed Snapshot | Daily at 01:00 UTC | **30 Days** | Cloud Object Storage (Standard) | AWS S3 Object Lock (Compliance Mode) / GCS Bucket Lock |
| **Cold Monthly** | Compressed Logical Dump | 1st of each month | **365 Days (1 Year)** | Glacier Flexible / Archive | KMS Encrypted, Object Lock 365 days |

### 3.2 Encryption Standards
1. **At Rest:** All backup archives and distributed snapshots MUST be encrypted using AES-256-GCM via Customer-Managed Keys (CMK) in AWS KMS or GCP Cloud KMS (`--sse aws:kms`).
2. **In Transit:** All replication and backup transfer traffic must use TLS 1.3 (`verify-full`).
3. **WORM Compliance:** Backup buckets enforce Write-Once-Read-Many (WORM) Object Lock. No principal, including root or cloud admin, can delete or overwrite backup objects until the retention period expires.

### 3.3 Access Control & Least Privilege
- **Backup Principal:** Automated backup CronJobs assume `arn:aws:iam::123456789012:role/nvuto-backup-agent` with permissions restricted to `s3:PutObject` and `kms:GenerateDataKey`.
- **Restore Principal:** Production restore execution requires dual-authorization (Break-Glass protocol) assuming `arn:aws:iam::123456789012:role/nvuto-dr-restore-agent`.

---

## 4. Post-Restore Outbox Reconciliation Strategy

### 4.1 The $T-1$ Prior-Image Reversion Hazard

When a database is restored from a backup taken at $T-1$ after a crash at $T-0$:
```
Timeline:
  T-1 (Snapshot Taken):   Consent C1 status = "granted" (Generation 1).
  T-0.5 (Live Operation): Customer withdraws consent.
                          DB updates C1 status = "withdrawn" (Generation 2).
                          Outbox publishes Generation 2 to NATS JetStream.
                          Downstream services receive Gen 2 and halt marketing.
  T-0 (Catastrophic Crash): Primary DB storage lost.
  DR Restore to T-1:      Database reverts to state at T-1!
                          In DB: C1 status is once again "granted" (Generation 1)!
                          Outbox row for Gen 1 may still be pending or reset.
```

**The Threat:** If the outbox worker re-publishes the restored Generation 1 ("granted") event, an un-guarded downstream consumer might process it and erroneously re-enable marketing communication, causing severe regulatory and privacy violations.

### 4.2 The Reconciliation Contract (T-010 / T-022)

The system protects against this failure mode through two complementary architectural invariants:

1. **Downstream `AggregateRevision` Invariant (Contract T-010):**
   - Every outbox event published to NATS carries `AggregateRevision` (which maps directly to `preference_generation`).
   - Every downstream consumer maintains a local high-water mark: `highest_seen_revision`.
   - **Contract Rule:** If `incoming_event.aggregate_revision <= local.highest_seen_revision`, the event **MUST BE DISCARDED**.
   - Because downstream already processed Generation 2 ("withdrawn") at $T-0.5$, any re-published Generation 1 ("granted") event from the restored database is safely dropped as a superseded stale revision.

2. **Database Outbox Reconciliation Procedure (Runbook Step):**
   - Before un-pausing the outbox worker on a restored database, operators execute [`scripts/dr_restore_and_reconcile.sh`](file:///c:/antigravity/scratch/Nvuto-Consent/scripts/dr_restore_and_reconcile.sh).
   - The script inspects `consent_outbox_events` and clears expired locks (`locked_by = NULL, locked_at = NULL`).
   - The worker resumes streaming. Downstream consumers discard the redundant events without state regression.

---

## 5. Lawful Deletion vs. Audit Immutability (GDPR Art. 17 / NDPR)

### 5.1 The Regulatory Paradox

- **Audit Requirement:** Financial, privacy, and healthcare regulations (GDPR Art. 5(2), HIPAA, SOC 2) require immutable, append-only audit trails (`consent_audit_events`) that cannot be modified by administrators.
- **Right to Erasure:** GDPR Article 17 and NDPR Section 3.1 grant data subjects the "Right to be Forgotten" (erasure of personal data).
- **Resolution:** Physical `DELETE` of audit rows violates accountability and proves nothing about what occurred. The approved legal and architectural standard is **Cryptographic Redaction with Preserved Structural Envelopes**.

### 5.2 Two-Tier Lawful Erasure Procedure

Implemented in [`internal/store/lawful_deletion.go`](file:///c:/antigravity/scratch/Nvuto-Consent/internal/store/lawful_deletion.go) and enabled by migration `012_lawful_erasure_support.sql`:

1. **Tombstone Active State:**
   - Target consents for `(tenant_id, subject_id)` are updated to `status = 'withdrawn'`.
   - Personal attributes in `scope` and `evidence` are redacted: `scope = '{"redacted": "true"}'::jsonb`.
   - Suppressions for the subject are deactivated (`active = false`).

2. **Redact Audit Payloads under Audited Authorization:**
   - The session sets `SET LOCAL nvuto.lawful_erasure_authorized = 'true'`.
   - Trigger `trg_consent_audit_immutable` verifies that structural envelope metadata (`id`, `tenant_id`, `actor_id`, `action`, `consent_id`, `occurred_at`, `revision`) is **NOT** modified.
   - Only PII payloads (`before_state`, `after_state`, `changes`) are updated to `{"redacted": true}`.

3. **Append Immutable Erasure Proof:**
   - An immutable audit record is appended to `consent_audit_events`:
     - `action = 'consent.subject.erased'`
     - `resource_type = 'subject'`
     - `resource_id = subject_id`
     - `actor_id = privacy_officer_id`
     - `reason = 'GDPR Article 17 Right to Erasure Request'`
   - An outbox event `consent.subject.erased` is published so all downstream data processors purge cached personal data.

---

## 6. Disaster Recovery Verification & Drill Checklist

Prior to production sign-off, operators must verify the DR capability by executing the automated test suite:

- [x] **RPO/RTO Defined:** RPO $\le 15$ min, RTO $\le 60$ min.
- [x] **YSQL Snapshot / PITR Commands Documented:** Section 2.
- [x] **WORM Retention & KMS Encryption Specified:** Section 3.
- [x] **Downstream Reconciliation Proved:** Verified by `TestAT022_DisasterRecoveryReconciliationProvesNoResurrection` in `internal/store/dr_reconciliation_test.go`.
- [x] **Lawful Erasure Verified:** Verified by `TestAT022_LawfulErasureRedactsAuditWithoutBreakingImmutability` in `internal/store/lawful_deletion_test.go`.
- [x] **Automated DR Script:** Delivered in `scripts/dr_restore_and_reconcile.sh`.
- [x] **CI Disaster Simulation Pipeline:** Delivered in `.github/workflows/dr_simulation_test.yml`.
