#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
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
export AUTH_MODE="${AUTH_MODE:-oidc}"
export AUTH_AUDIENCE="${AUTH_AUDIENCE:-golden-path-control-plane}"
export AUTH_ISSUER="${AUTH_ISSUER:-http://localhost:9000}"
export AUTH_OIDC_ISSUER_URL="${AUTH_OIDC_ISSUER_URL:-http://localhost:9000}"
export AUTH_OIDC_JWKS_URL="${AUTH_OIDC_JWKS_URL:-http://localhost:9000/keys}"
export OTEL_SERVICE_NAME="${OTEL_SERVICE_NAME:-golden-path-control-plane}"
export OTEL_EXPORTER_OTLP_ENDPOINT="${OTEL_EXPORTER_OTLP_ENDPOINT:-}"
export PROMETHEUS_NAMESPACE="${PROMETHEUS_NAMESPACE:-goldenpath}"
export CONTROL_PLANE_API_URL="${CONTROL_PLANE_API_URL:-http://localhost:8080}"
export OIDC_DEV_ISSUER_URL="${OIDC_DEV_ISSUER_URL:-http://localhost:9000}"
export OIDC_DEV_AUDIENCE="${OIDC_DEV_AUDIENCE:-golden-path-control-plane}"
export OIDC_DEV_ENGINEER_SUBJECT="${OIDC_DEV_ENGINEER_SUBJECT:-owner@example.com}"
export OIDC_DEV_PLATFORM_ADMIN_SUBJECT="${OIDC_DEV_PLATFORM_ADMIN_SUBJECT:-platform-admin@example.com}"

cleanup() {
  local exit_code=$?
  docker compose -f "${ROOT_DIR}/deployments/docker-compose.yml" logs --no-color api worker postgres local-oidc migrate >/tmp/golden-path-compose.log 2>&1 || true
  docker compose -f "${ROOT_DIR}/deployments/docker-compose.yml" down -v >/dev/null 2>&1 || true
  if [[ ${exit_code} -ne 0 && -f /tmp/golden-path-compose.log ]]; then
    cat /tmp/golden-path-compose.log >&2
  fi
}
trap cleanup EXIT

docker compose -f "${ROOT_DIR}/deployments/docker-compose.yml" up -d --build

for _ in $(seq 1 45); do
  if curl -sf "${AUTH_OIDC_ISSUER_URL}/healthz" >/dev/null; then
    break
  fi
  sleep 1
done

curl -sf "${AUTH_OIDC_ISSUER_URL}/healthz" >/dev/null

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
SMOKE_SKIP_MIGRATE=1 SMOKE_SKIP_RUNTIME_START=1 ./scripts/smoke.sh

echo "compose smoke test passed"
