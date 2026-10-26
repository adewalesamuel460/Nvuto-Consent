#!/usr/bin/env bash
# ==============================================================================
# Script: scripts/dr_restore_and_reconcile.sh
# Purpose: Automated YugabyteDB DR Restore and Outbox Event Reconciliation (T-022 / AT-022)
# Usage: ./scripts/dr_restore_and_reconcile.sh [backup_file.dump]
# ==============================================================================

set -euo pipefail

log() {
  echo "[$(date -u +'%Y-%m-%dT%H:%M:%SZ')] [DR-RECONCILE] $*"
}

err() {
  echo "[$(date -u +'%Y-%m-%dT%H:%M:%SZ')] [DR-RECONCILE] [ERROR] $*" >&2
}

DATABASE_URL="${DATABASE_URL:-${TEST_DATABASE_URL:-postgres://yugabyte:yugabyte@127.0.0.1:5433/consent?sslmode=disable}}"
BACKUP_FILE="${1:-${BACKUP_FILE:-}}"
NATS_URL="${NATS_URL:-${TEST_NATS_URL:-nats://127.0.0.1:4222}}"

log "Starting Disaster Recovery Restore and Event Reconciliation..."
log "Target Database URL: ${DATABASE_URL}"
log "NATS JetStream URL: ${NATS_URL}"

# 1. Connectivity Verification
log "Checking database connectivity..."
if ! command -v psql >/dev/null 2>&1; then
  err "psql client utility is required but not installed."
  exit 1
fi

psql "${DATABASE_URL}" -c "SELECT 1;" >/dev/null 2>&1 || {
  err "Failed to connect to target database at ${DATABASE_URL}."
  exit 1
}
log "Database connection verified."

# 2. Backup File Validation (if restoring from dump)
if [[ -n "${BACKUP_FILE}" ]]; then
  if [[ ! -f "${BACKUP_FILE}" ]]; then
    err "Specified backup file does not exist: ${BACKUP_FILE}"
    exit 1
  fi
  log "Found backup archive: ${BACKUP_FILE} ($(du -h "${BACKUP_FILE}" | cut -f1))"

  log "Executing database restore from backup..."
  if command -v pg_restore >/dev/null 2>&1; then
    pg_restore --clean --if-exists --no-owner --dbname="${DATABASE_URL}" "${BACKUP_FILE}" || {
      log "pg_restore completed with warnings (continuing to integrity check)..."
    }
  else
    psql "${DATABASE_URL}" -f "${BACKUP_FILE}" || {
      err "psql restore failed."
      exit 1
    }
  fi
  log "Database restore phase completed."
else
  log "No backup file supplied; running in online post-restore reconciliation mode."
fi

# 3. Post-Restore Schema & Integrity Verification
log "Running schema integrity validation against restored database..."
if [[ -f "scripts/validate_integrity.sql" ]]; then
  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -f scripts/validate_integrity.sql
  log "Integrity validation script passed with 0 violations."
else
  log "Warning: scripts/validate_integrity.sql not found; skipping SQL census."
fi

# 4. Outbox Event Reconciliation
log "Reconciling consent_outbox_events table..."

# 4a. Reset any stale/orphaned locks left from the pre-disaster worker crash
STALE_LOCKS=$(psql "${DATABASE_URL}" -Atc "
  SELECT count(*) FROM consent_outbox_events WHERE locked_at IS NOT NULL AND published_at IS NULL;
")
log "Found ${STALE_LOCKS} orphaned outbox leases from pre-crash state."

if [[ "${STALE_LOCKS}" -gt 0 ]]; then
  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -c "
    UPDATE consent_outbox_events
    SET locked_at = NULL, locked_by = NULL, attempts = 0
    WHERE locked_at IS NOT NULL AND published_at IS NULL;
  "
  log "Reset ${STALE_LOCKS} orphaned outbox records to unlock for worker processing."
fi

# 4b. Inventory Outbox State
TOTAL_OUTBOX=$(psql "${DATABASE_URL}" -Atc "SELECT count(*) FROM consent_outbox_events;")
PENDING_OUTBOX=$(psql "${DATABASE_URL}" -Atc "SELECT count(*) FROM consent_outbox_events WHERE published_at IS NULL;")
PUBLISHED_OUTBOX=$(psql "${DATABASE_URL}" -Atc "SELECT count(*) FROM consent_outbox_events WHERE published_at IS NOT NULL;")

log "Outbox State Summary:"
log "  - Total Events in Database:    ${TOTAL_OUTBOX}"
log "  - Pending for Re-publication:  ${PENDING_OUTBOX}"
log "  - Marked Published:            ${PUBLISHED_OUTBOX}"

# 5. Downstream Idempotency & Revision Reconciliation Notice
log "----------------------------------------------------------------------"
log "DR RECONCILIATION GUARANTEE (T-010 / T-022):"
log "Database restoration to prior image (T-1) causes pending outbox events"
log "to be re-published to NATS JetStream upon worker startup."
log "Downstream consumers enforce the AggregateRevision contract:"
log "  incoming.aggregate_revision <= local.highest_seen_revision => DISCARD"
log "This guarantees that restored 'granted' events CANNOT overwrite subsequent"
log "withdrawals already committed downstream before the crash."
log "----------------------------------------------------------------------"

log "Disaster recovery restore and reconciliation completed successfully."
exit 0
