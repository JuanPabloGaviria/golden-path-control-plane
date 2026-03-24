#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export AUTH_HMAC_SECRET="${AUTH_HMAC_SECRET:-12345678901234567890123456789012}"
export APP_ENV=development
export APP_LOG_LEVEL=INFO
export DATABASE_URL="${DATABASE_URL:-postgres://controlplane:controlplane@localhost:5432/controlplane?sslmode=disable}"
export DATABASE_MAX_OPEN_CONNS="${DATABASE_MAX_OPEN_CONNS:-10}"
export DATABASE_MIN_IDLE_CONNS="${DATABASE_MIN_IDLE_CONNS:-2}"
export DATABASE_MAX_CONN_LIFETIME="${DATABASE_MAX_CONN_LIFETIME:-30m}"
export DATABASE_MAX_CONN_IDLE_TIME="${DATABASE_MAX_CONN_IDLE_TIME:-5m}"
export HTTP_ADDR="${HTTP_ADDR:-:8080}"
export HTTP_READ_TIMEOUT="${HTTP_READ_TIMEOUT:-10s}"
export HTTP_WRITE_TIMEOUT="${HTTP_WRITE_TIMEOUT:-15s}"
export HTTP_IDLE_TIMEOUT="${HTTP_IDLE_TIMEOUT:-60s}"
export SHUTDOWN_TIMEOUT="${SHUTDOWN_TIMEOUT:-20s}"
export WORKER_POLL_INTERVAL="${WORKER_POLL_INTERVAL:-2s}"
export WORKER_BATCH_SIZE="${WORKER_BATCH_SIZE:-5}"
export JOB_LEASE_DURATION="${JOB_LEASE_DURATION:-30s}"
export JOB_MAX_ATTEMPTS="${JOB_MAX_ATTEMPTS:-5}"
export AUTH_MODE="${AUTH_MODE:-hmac}"
export AUTH_AUDIENCE="${AUTH_AUDIENCE:-golden-path-control-plane}"
export AUTH_ISSUER="${AUTH_ISSUER:-golden-path-local}"
export OTEL_SERVICE_NAME="${OTEL_SERVICE_NAME:-golden-path-control-plane}"
export OTEL_EXPORTER_OTLP_ENDPOINT="${OTEL_EXPORTER_OTLP_ENDPOINT:-}"
export PROMETHEUS_NAMESPACE="${PROMETHEUS_NAMESPACE:-goldenpath}"
export CONTROL_PLANE_API_URL="${CONTROL_PLANE_API_URL:-http://localhost:8080}"

cleanup() {
  local exit_code=$?
  docker compose -f "${ROOT_DIR}/deployments/docker-compose.yml" logs --no-color api worker postgres >/tmp/golden-path-compose.log 2>&1 || true
  docker compose -f "${ROOT_DIR}/deployments/docker-compose.yml" down -v >/dev/null 2>&1 || true
  if [[ ${exit_code} -ne 0 && -f /tmp/golden-path-compose.log ]]; then
    cat /tmp/golden-path-compose.log >&2
  fi
}
trap cleanup EXIT

docker compose -f "${ROOT_DIR}/deployments/docker-compose.yml" up -d --build

for _ in $(seq 1 45); do
  if curl -sf "${CONTROL_PLANE_API_URL}/readyz" >/dev/null; then
    break
  fi
  sleep 1
done

curl -sf "${CONTROL_PLANE_API_URL}/readyz" >/dev/null
export CONTROL_PLANE_TOKEN
CONTROL_PLANE_TOKEN="$(cd "${ROOT_DIR}" && go run ./cmd/cli token --subject smoke@example.com --role platform-admin)"

cd "${ROOT_DIR}"
SMOKE_SKIP_RUNTIME_START=1 ./scripts/smoke.sh

echo "compose smoke test passed"
