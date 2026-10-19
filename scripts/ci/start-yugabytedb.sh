#!/usr/bin/env bash
set -euo pipefail

database="${1:?usage: start-yugabytedb.sh <database>}"
container="${YUGABYTEDB_CONTAINER:-nvuto-consent-yugabyte}"
image="${YUGABYTEDB_IMAGE:-yugabytedb/yugabyte:2026.1.0.1-b1}"
timeout_seconds="${YUGABYTEDB_STARTUP_TIMEOUT_SECONDS:-300}"

docker rm -f "$container" >/dev/null 2>&1 || true
docker run -d --name "$container" -p 5433:5433 "$image" sleep infinity >/dev/null

if ! docker exec "$container" bin/yb-ctl \
  --data_dir /tmp/nvuto-consent-yb-data \
  --timeout-processes-running-sec "$timeout_seconds" \
  create --listen_ip=0.0.0.0; then
  echo "yb-ctl failed to create the RF1 YugabyteDB cluster" >&2
  docker exec "$container" bin/yb-ctl --data_dir /tmp/nvuto-consent-yb-data status >&2 || true
  docker logs "$container" >&2 || true
  exit 1
fi

deadline=$((SECONDS + timeout_seconds))
while (( SECONDS < deadline )); do
  if docker exec "$container" postgres/bin/pg_isready -h 127.0.0.1 -p 5433 -U yugabyte >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

docker exec "$container" postgres/bin/pg_isready -h 127.0.0.1 -p 5433 -U yugabyte
docker exec "$container" bin/ysqlsh -h 127.0.0.1 -U yugabyte -d yugabyte -v ON_ERROR_STOP=1 -c "CREATE DATABASE ${database};"
servers="$(docker exec "$container" bin/ysqlsh -h 127.0.0.1 -U yugabyte -d "$database" -Atc 'select count(*) from yb_servers();')"
if ! [[ "$servers" =~ ^[1-9][0-9]*$ ]]; then
  echo "YugabyteDB topology verification failed: yb_servers() returned '$servers'" >&2
  exit 1
fi
printf 'YugabyteDB YSQL ready: database=%s servers=%s\n' "$database" "$servers"
