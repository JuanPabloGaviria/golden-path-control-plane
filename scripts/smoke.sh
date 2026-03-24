#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_URL="${CONTROL_PLANE_API_URL:-http://localhost:8080}"
RUN_ID="$(date +%s)"
SMOKE_SKIP_RUNTIME_START="${SMOKE_SKIP_RUNTIME_START:-0}"
SMOKE_SKIP_MIGRATE="${SMOKE_SKIP_MIGRATE:-0}"

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL must be set for smoke tests" >&2
  exit 1
fi

cleanup() {
  local exit_code=$?
  if [[ -n "${API_PID:-}" ]]; then
    kill "${API_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${WORKER_PID:-}" ]]; then
    kill "${WORKER_PID}" >/dev/null 2>&1 || true
  fi
  rm -f "${SERVICE_FILE:-}"
  if [[ ${exit_code} -ne 0 ]]; then
    [[ -f /tmp/golden-path-migrate.log ]] && cat /tmp/golden-path-migrate.log >&2
    [[ -f /tmp/golden-path-api.log ]] && cat /tmp/golden-path-api.log >&2
    [[ -f /tmp/golden-path-worker.log ]] && cat /tmp/golden-path-worker.log >&2
  fi
}
trap cleanup EXIT

cd "${ROOT_DIR}"

if [[ "${SMOKE_SKIP_MIGRATE}" != "1" ]]; then
  go run ./cmd/migrate >/tmp/golden-path-migrate.log 2>&1
fi

if [[ "${SMOKE_SKIP_RUNTIME_START}" != "1" ]]; then
  go run ./cmd/api >/tmp/golden-path-api.log 2>&1 &
  API_PID=$!

  go run ./cmd/worker >/tmp/golden-path-worker.log 2>&1 &
  WORKER_PID=$!
fi

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
  "name": "__SERVICE_NAME__",
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

sed -i.bak "s/__SERVICE_NAME__/smoke-service-${RUN_ID}/" "${SERVICE_FILE}"
rm -f "${SERVICE_FILE}.bak"

SERVICE_RESPONSE="$(go run ./cmd/cli register-service --file "${SERVICE_FILE}")"
SERVICE_ID="$(printf '%s' "${SERVICE_RESPONSE}" | grep -o '"id":"[^"]*"' | head -n 1 | cut -d '"' -f4)"
if [[ -z "${SERVICE_ID}" ]]; then
  echo "failed to extract service ID from response: ${SERVICE_RESPONSE}" >&2
  exit 1
fi

go run ./cmd/cli queue-evaluation --service-id "${SERVICE_ID}" >/tmp/golden-path-job.json

for _ in $(seq 1 30); do
  if SCORECARD_RESPONSE="$(go run ./cmd/cli scorecard --service-id "${SERVICE_ID}" 2>/dev/null)"; then
    if printf '%s' "${SCORECARD_RESPONSE}" | grep -q '"state":"ready"'; then
      break
    fi
  fi
  sleep 1
done

SCORECARD_RESPONSE="$(go run ./cmd/cli scorecard --service-id "${SERVICE_ID}")"
printf '%s' "${SCORECARD_RESPONSE}" | grep -q '"state":"ready"'

CANDIDATE_RESPONSE="$(go run ./cmd/cli create-candidate --service-id "${SERVICE_ID}" --environment production --version v1.0.0 --commit-sha abc123 --requested-by owner@example.com)"
CANDIDATE_ID="$(printf '%s' "${CANDIDATE_RESPONSE}" | grep -o '"id":"[^"]*"' | head -n 1 | cut -d '"' -f4)"
if [[ -z "${CANDIDATE_ID}" ]]; then
  echo "failed to extract candidate ID from response: ${CANDIDATE_RESPONSE}" >&2
  exit 1
fi

EVALUATE_RESPONSE="$(go run ./cmd/cli evaluate-candidate --candidate-id "${CANDIDATE_ID}")"
printf '%s' "${EVALUATE_RESPONSE}" | grep -q '"status":"approved"'

echo "smoke test passed"
