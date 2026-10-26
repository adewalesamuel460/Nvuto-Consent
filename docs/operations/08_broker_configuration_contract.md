# Production NATS JetStream Broker Configuration & Retention Contract

**Document ID:** `DOC-OPS-008`  
**Task Reference:** `T-020` | **Finding:** `F-020` | **Gate:** `G-020` | **Acceptance Test:** `AT-020`  
**Classification:** Internal Technical Specification & Contract  
**Status:** Approved for Production  

---

## 1. Executive Summary

This document defines the production infrastructure contract and capacity limits for the **NATS JetStream** messaging layer used by the Nvuto Consent Outbox Worker. 

Under finding **F-020**, application workers must not silently create or mutate shared messaging infrastructure at runtime. The messaging broker topology is owned exclusively by infrastructure-as-code (Terraform) and platform operations. At startup, the outbox worker verifies that the active stream strictly conforms to this contract; if configuration drift is detected, the worker logs an actionable alert and immediately halts startup.

---

## 2. Approved Production Stream Contract

The approved stream configuration guarantees high availability across Availability Zones, durable persistence on disk, bounded message retention, and protection against message duplication during worker failover.

### 2.1 Specification Parameters

| Parameter | Approved Production Value | Justification & Operational Invariant |
| :--- | :--- | :--- |
| **Stream Name** | `NVUTO_CONSENT` (or `NVUTO_OUTBOX`) | Canonical stream identifier for privacy consent lifecycle events. |
| **Subjects** | `nvuto.consent.>` | Wildcard namespace covering all consent aggregate events (`nvuto.consent.granted`, `nvuto.consent.withdrawn`, etc.). |
| **Storage Type** | `FileStorage` (`1`) | Persistent NVMe disk-backed storage. In-memory storage (`MemoryStorage`) is strictly prohibited in production to prevent data loss on broker restarts. |
| **Replicas** | `3` | Multi-AZ Raft quorum replication (RF=3). Survives loss of a full AZ or node. Replicas < 3 is permitted ONLY in local development environments. |
| **Retention Policy** | `LimitsPolicy` | Messages are retained until any configured limit (`MaxAge`, `MaxBytes`, `MaxMsgs`) is reached, allowing multiple independent consumer groups to process at their own pace. |
| **Deduplication Window** | `10 minutes` (`10m`) | Deduplication horizon keyed by `Nats-Msg-Id` (the outbox event UUID). Safely exceeds the worker lease duration (2 minutes) by 5x to absorb worker crash/recovery cycles. |
| **Max Message Age (`MaxAge`)** | `7 days` (`168h`) | Bounded retention buffer. Provides operational headroom for downstream subscriber recovery during extended outages. |
| **Max Stream Bytes (`MaxBytes`)** | `50 GB` (`53687091200`) | Hard capacity ceiling. At ~1.5 KB per CloudEvent, accommodates >33 million events. |
| **Max Message Size (`MaxMsgSize`)** | `1 MB` (`1048576`) | Rejects oversized payloads at the broker edge. Standard consent CloudEvents average 1.5 KB. |
| **Discard Policy** | `DiscardOld` | When `MaxBytes` or `MaxAge` is reached, oldest acknowledged/unacknowledged messages are evicted, preserving real-time ingestion throughput. |

---

## 3. Terraform / Infrastructure as Code Contract

The production stream must be provisioned and managed via Terraform using the official NATS provider:

```hcl
resource "nats_jetstream_stream" "consent_outbox" {
  name        = "NVUTO_CONSENT"
  description = "Transactional outbox events for Nvuto Consent domain events (RF=3)"
  subjects    = ["nvuto.consent.>"]
  
  # Durability and High Availability
  storage     = "file"
  replicas    = 3
  
  # Retention and Capacity Limits
  retention   = "limits"
  max_age     = 604800     # 7 days in seconds
  max_bytes   = 53687091200 # 50 GB
  max_msg_size = 1048576   # 1 MB
  discard     = "old"
  
  # Message Deduplication Window
  duplicate_window = 600   # 10 minutes in seconds (600s)
}
```

---

## 4. Consumer Invariant: At-Least-Once Delivery & Deduplication Horizon

### 4.1 Bounded Broker Deduplication (The 10-Minute Horizon)

The outbox worker assigns each event's unique database UUID to the `Nats-Msg-Id` header:
- When an outbox worker publishes an event, NATS JetStream indexes the `Nats-Msg-Id`.
- If the worker crashes before marking the outbox row complete in PostgreSQL, the lease expires after 2 minutes.
- A survivor worker re-claims the row and re-publishes it under the identical `Nats-Msg-Id`.
- Because the re-publication occurs well within the **10-minute deduplication window**, JetStream recognizes the ID, suppresses the duplicate, and returns a synthetic ACK.

### 4.2 Handling Deliveries Beyond the 10-Minute Horizon (T-010)

If an event is published or replayed after more than 10 minutes:
- Examples: An event quarantined in the DLQ is manually replayed by an operator hours later, or a massive backlog is drained following disaster recovery.
- NATS JetStream has expired the 10-minute `Nats-Msg-Id` cache and **will accept and deliver the event as a new message**.
- **Downstream Consumer Contract (Mandatory):**
  - All consumer microservices and webhooks **must not rely exclusively on broker deduplication**.
  - Consumers must enforce aggregate revision fencing using the CloudEvent metadata headers stamped by the outbox worker:
    - `X-Nvuto-Tenant-ID`: Customer tenant isolation.
    - `X-Nvuto-Aggregate-Type`: Target domain aggregate (e.g. `consent`).
    - `X-Nvuto-Aggregate-Revision`: Monotonically increasing generation number (`preference_generation`).
  - If a consumer receives an event where `AggregateRevision <= CurrentState.Revision`, the consumer MUST treat the event as an idempotent duplicate and discard it without re-applying side effects.

---

## 5. Worker Startup Validation & Drift Detection

At worker startup, `validateStreamContract` in `cmd/outbox-worker/main.go` executes the following check:

```
[Worker Startup]
       │
       ▼
[Fetch StreamInfo("NVUTO_CONSENT")]
       │
       ├── Stream Does Not Exist ──► [In Dev: Create Stream] ──► [In Prod: Fail Closed]
       │
       └── Stream Exists
               │
               ▼
       [Verify Configuration vs Production Contract]
               ├── Storage == FileStorage
               ├── Replicas >= 3 (unless local dev mode)
               ├── Subjects cover nvuto.consent.>
               ├── Duplicates == 10 minutes
               └── Retention == LimitsPolicy
               │
               ├── Match ──► [Log INFO & Bind Worker]
               │
               └── Drift Detected
                       │
                       ▼
               [Log Structured ERROR: nats_stream_configuration_drift_detected]
               [Halt Execution: os.Exit(1)]
               (Do NOT call js.UpdateStream)
```

### 5.1 Drift Detection Actionable Log Format

When drift occurs, the worker emits structured JSON to stderr and exits:

```json
{
  "time": "2026-10-26T10:00:00Z",
  "level": "ERROR",
  "msg": "nats_stream_configuration_drift_detected",
  "stream": "NVUTO_CONSENT",
  "expected_subject": "nvuto.consent.>",
  "expected_replicas": 3,
  "expected_duplicates": 600000000000,
  "expected_storage": "FileStorage",
  "remediation": "reconcile NATS stream via Terraform/IaC; worker will not mutate shared broker state",
  "error": "stream \"NVUTO_CONSENT\" storage type mismatch: got MemoryStorage, want FileStorage (FileStorage required)"
}
```

---

## 6. Operational Runbook & Verification Commands

### 6.1 Inspecting Active Stream Configuration via NATS CLI

```bash
# Check stream configuration and health
nats stream info NVUTO_CONSENT

# Check storage utilization, message counts, and consumer lag
nats stream report
```

Expected output:
```
Information for Stream NVUTO_CONSENT created 2026-10-26 08:00:00

Configuration:
             Subjects: nvuto.consent.>
         Storage Type: File
             Replicas: 3
            Retention: Limits
             Discard: Old
              Max Age: 7d 0h 0m 0s
            Max Bytes: 50.00 GiB
         Max Msg Size: 1.00 MiB
           Duplicates: 10m0s
```

### 6.2 Remediating Configuration Drift

If the outbox worker fails startup with `nats_stream_configuration_drift_detected`:
1. Check the drift error message in the worker pod logs.
2. Review the active Terraform state for `nats_jetstream_stream.consent_outbox`.
3. If the stream was created manually or with dev parameters, execute Terraform apply to bring the stream into compliance:
   ```bash
   terraform plan -target=nats_jetstream_stream.consent_outbox
   terraform apply -target=nats_jetstream_stream.consent_outbox
   ```
4. Restart the outbox worker deployment once the stream configuration matches the contract.
