#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_URL="${CONTROL_PLANE_API_URL:-http://localhost:8080}"

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL must be set for smoke tests" >&2
  exit 1
fi

cleanup() {
  if [[ -n "${API_PID:-}" ]]; then
    kill "${API_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${WORKER_PID:-}" ]]; then
    kill "${WORKER_PID}" >/dev/null 2>&1 || true
  fi
  rm -f "${SERVICE_FILE:-}"
}
trap cleanup EXIT

cd "${ROOT_DIR}"

go run ./cmd/api >/tmp/golden-path-api.log 2>&1 &
API_PID=$!

go run ./cmd/worker >/tmp/golden-path-worker.log 2>&1 &
WORKER_PID=$!

for _ in $(seq 1 30); do
  if curl -sf "${API_URL}/readyz" >/dev/null; then
    break
  fi
  sleep 1
done

curl -sf "${API_URL}/readyz" >/dev/null

export CONTROL_PLANE_API_URL="${API_URL}"
export CONTROL_PLANE_TOKEN
CONTROL_PLANE_TOKEN="$(go run ./cmd/cli token --subject smoke@example.com --role platform-admin)"
export CONTROL_PLANE_TOKEN

SERVICE_FILE="$(mktemp)"
cat > "${SERVICE_FILE}" <<'JSON'
{
  "name": "smoke-service",
  "description": "Smoke-test service",
  "owner_email": "owner@example.com",
  "repository_url": "https://github.com/example/smoke-service",
  "runbook_url": "https://example.com/runbook",
  "health_endpoint_url": "https://example.com/healthz",
  "observability_url": "https://example.com/dashboard",
  "deployment_pipeline": "github-actions",
  "has_ci": true,
  "has_tracing": true,
  "has_metrics": true,
  "language": "go",
  "tier": 1,
  "lifecycle": "production",
  "slo_policy": {
    "availability_target_percent": 99.9,
    "latency_target_milliseconds": 250,
    "window": "30d"
  }
}
JSON

SERVICE_RESPONSE="$(go run ./cmd/cli register-service --file "${SERVICE_FILE}")"
SERVICE_ID="$(printf '%s' "${SERVICE_RESPONSE}" | grep -oE '[0-9a-f-]{36}' | head -n 1)"

go run ./cmd/cli queue-evaluation --service-id "${SERVICE_ID}" >/tmp/golden-path-job.json
sleep 3

SCORECARD_RESPONSE="$(go run ./cmd/cli scorecard --service-id "${SERVICE_ID}")"
printf '%s' "${SCORECARD_RESPONSE}" | grep -q '"state":"ready"'

CANDIDATE_RESPONSE="$(go run ./cmd/cli create-candidate --service-id "${SERVICE_ID}" --environment production --version v1.0.0 --commit-sha abc123 --requested-by owner@example.com)"
CANDIDATE_ID="$(printf '%s' "${CANDIDATE_RESPONSE}" | grep -oE '[0-9a-f-]{36}' | head -n 1)"

EVALUATE_RESPONSE="$(go run ./cmd/cli evaluate-candidate --candidate-id "${CANDIDATE_ID}")"
printf '%s' "${EVALUATE_RESPONSE}" | grep -q '"status":"approved"'

echo "smoke test passed"
