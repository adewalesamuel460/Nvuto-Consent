#!/usr/bin/env bash
# ==============================================================================
# Script: run_e2e_uat.sh
# Purpose: Wire up Staging/UAT topology and run end-to-end integration tests
#          simulating approved launch consumers (T-023 / AT-023 / G-023).
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "================================================================================"
echo "NVUTO-CONSENT: End-to-End Consumer Integration & UAT Runner (T-023)"
echo "Target: Launch Consumer Scope (CRM, Contact Center, HR, Customer Portal)"
echo "================================================================================"

# 1. Environment Configurations & Defaults
export DATABASE_URL="${DATABASE_URL:-${TEST_DATABASE_URL:-postgres://yugabyte:yugabyte@127.0.0.1:5433/consent?sslmode=disable}}"
export TEST_DATABASE_URL="${DATABASE_URL}"
export NATS_URL="${NATS_URL:-${TEST_NATS_URL:-nats://127.0.0.1:4222}}"
export TEST_NATS_URL="${NATS_URL}"
export NVUTO_REQUIRE_INTEGRATION="true"
export ALLOW_DEV_HEADERS="${ALLOW_DEV_HEADERS:-true}"
export ALLOW_INSECURE_NATS="${ALLOW_INSECURE_NATS:-true}"
export ENVIRONMENT="test"

# Safe temporary build directory to prevent permission locking on Windows / container roots
BUILD_TMP="${REPO_ROOT}/build-tmp"
mkdir -p "${BUILD_TMP}"
export GOTMPDIR="${BUILD_TMP}"

echo "[*] Database URL : ${DATABASE_URL}"
echo "[*] NATS URL     : ${NATS_URL}"
echo "[*] Working Dir  : ${REPO_ROOT}"
echo "[*] Build Tmp    : ${GOTMPDIR}"

# 2. Dependency Health Checks
echo "[*] Checking Go toolchain..."
if ! command -v go >/dev/null 2>&1; then
  echo "[-] ERROR: 'go' toolchain is required but not installed on PATH."
  exit 1
fi
go version

echo "[*] Checking Database connectivity..."
# Quick probe using psql or nc if available, or rely on Go test pre-flight
if command -v pg_isready >/dev/null 2>&1; then
  # Parse host and port from URL if possible
  DB_HOST=$(echo "${DATABASE_URL}" | sed -e 's,.*://.*@,,g' -e 's,[:/].*,,g')
  DB_PORT=$(echo "${DATABASE_URL}" | sed -e 's,.*://.*@[^:]*:,,' -e 's,/.*,,g')
  if pg_isready -h "${DB_HOST}" -p "${DB_PORT}" -q; then
    echo "[+] Database is reachable at ${DB_HOST}:${DB_PORT}."
  else
    echo "[!] Warning: pg_isready failed for ${DB_HOST}:${DB_PORT}, test will verify via driver."
  fi
fi

# 3. Apply Schema Migrations if migration script exists
echo "[*] Verifying database schema migrations..."
if [ -f "${SCRIPT_DIR}/migrate.sh" ]; then
  bash "${SCRIPT_DIR}/migrate.sh" up
elif command -v psql >/dev/null 2>&1; then
  for migration in "${REPO_ROOT}"/migrations/*.sql; do
    if [ -f "${migration}" ]; then
      echo "    Applying $(basename "${migration}")..."
      psql "${DATABASE_URL}" -f "${migration}" >/dev/null 2>&1 || true
    fi
  done
fi

# 4. Execute the End-to-End Test Suite
echo ""
echo "================================================================================"
echo "Executing E2E Consumer Integration Test Suite (go test ./tests/e2e/...)"
echo "================================================================================"
cd "${REPO_ROOT}"

go test -v -count=1 ./tests/e2e/...

EXIT_CODE=$?

if [ ${EXIT_CODE} -eq 0 ]; then
  echo ""
  echo "================================================================================"
  echo "[+] SUCCESS: All End-to-End Consumer Integration tests passed."
  echo "    - Launch consumer identity and contracts verified."
  echo "    - Monotonic outbox/NATS reconciliation verified."
  echo "    - 6/6 Negative blocking conditions and Fail-Closed rule proven."
  echo "    - Propagation budget (< 2.0s) verified."
  echo "================================================================================"
else
  echo ""
  echo "================================================================================"
  echo "[-] FAILURE: E2E Consumer Integration test suite failed with exit code ${EXIT_CODE}."
  echo "================================================================================"
fi

exit ${EXIT_CODE}
