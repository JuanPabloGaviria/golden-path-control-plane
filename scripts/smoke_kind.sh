#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLUSTER_NAME="${KIND_CLUSTER_NAME:-golden-path}"
NAMESPACE="${KIND_NAMESPACE:-golden-path}"
API_URL="${CONTROL_PLANE_API_URL:-http://localhost:8080}"
OIDC_URL="${AUTH_OIDC_ISSUER_URL:-http://localhost:9000}"

API_PORT_FORWARD_PID=""
OIDC_PORT_FORWARD_PID=""

cleanup() {
  local exit_code=$?

  if command -v kubectl >/dev/null 2>&1 && kubectl config current-context >/dev/null 2>&1; then
    kubectl -n "${NAMESPACE}" get pods >/tmp/golden-path-kind-pods.log 2>/dev/null || true
    kubectl -n "${NAMESPACE}" get events --sort-by=.metadata.creationTimestamp >/tmp/golden-path-kind-events.log 2>/dev/null || true
    kubectl -n "${NAMESPACE}" describe pods >/tmp/golden-path-kind-describe.log 2>/dev/null || true
    kubectl -n "${NAMESPACE}" logs deploy/control-plane-api >/tmp/golden-path-kind-api.log 2>/dev/null || true
    kubectl -n "${NAMESPACE}" logs deploy/control-plane-worker >/tmp/golden-path-kind-worker.log 2>/dev/null || true
    kubectl -n "${NAMESPACE}" logs deploy/local-oidc >/tmp/golden-path-kind-oidc.log 2>/dev/null || true
    kubectl -n "${NAMESPACE}" logs job/control-plane-migrate >/tmp/golden-path-kind-migrate.log 2>/dev/null || true
  fi

  if [[ -n "${API_PORT_FORWARD_PID}" ]]; then
    kill "${API_PORT_FORWARD_PID}" >/dev/null 2>&1 || true
    wait "${API_PORT_FORWARD_PID}" 2>/dev/null || true
  fi
  if [[ -n "${OIDC_PORT_FORWARD_PID}" ]]; then
    kill "${OIDC_PORT_FORWARD_PID}" >/dev/null 2>&1 || true
    wait "${OIDC_PORT_FORWARD_PID}" 2>/dev/null || true
  fi

  kind delete cluster --name "${CLUSTER_NAME}" >/dev/null 2>&1 || true

  if [[ ${exit_code} -ne 0 ]]; then
    [[ -f /tmp/golden-path-kind-pods.log ]] && cat /tmp/golden-path-kind-pods.log >&2
    [[ -f /tmp/golden-path-kind-events.log ]] && cat /tmp/golden-path-kind-events.log >&2
    [[ -f /tmp/golden-path-kind-describe.log ]] && cat /tmp/golden-path-kind-describe.log >&2
    [[ -f /tmp/golden-path-kind-api.log ]] && cat /tmp/golden-path-kind-api.log >&2
    [[ -f /tmp/golden-path-kind-worker.log ]] && cat /tmp/golden-path-kind-worker.log >&2
    [[ -f /tmp/golden-path-kind-oidc.log ]] && cat /tmp/golden-path-kind-oidc.log >&2
    [[ -f /tmp/golden-path-kind-migrate.log ]] && cat /tmp/golden-path-kind-migrate.log >&2
  fi
}
trap cleanup EXIT

"${ROOT_DIR}/scripts/preflight.sh"

kind delete cluster --name "${CLUSTER_NAME}" >/dev/null 2>&1 || true
kind create cluster --name "${CLUSTER_NAME}"

cd "${ROOT_DIR}"

docker build --build-arg APP_BIN=api -t golden-path-control-plane-api:kind .
docker build --build-arg APP_BIN=worker -t golden-path-control-plane-worker:kind .
docker build --build-arg APP_BIN=migrate -t golden-path-control-plane-migrate:kind .
docker build --build-arg APP_BIN=devoidc -t golden-path-control-plane-devoidc:kind .

kind load docker-image --name "${CLUSTER_NAME}" \
  golden-path-control-plane-api:kind \
  golden-path-control-plane-worker:kind \
  golden-path-control-plane-migrate:kind \
  golden-path-control-plane-devoidc:kind

kubectl apply -k "${ROOT_DIR}/deployments/kubernetes/overlays/local-kind"

kubectl -n "${NAMESPACE}" rollout status deployment/postgres --timeout=180s
kubectl -n "${NAMESPACE}" rollout status deployment/local-oidc --timeout=180s
kubectl -n "${NAMESPACE}" wait --for=condition=complete job/control-plane-migrate --timeout=180s
kubectl -n "${NAMESPACE}" rollout status deployment/control-plane-api --timeout=180s
kubectl -n "${NAMESPACE}" rollout status deployment/control-plane-worker --timeout=180s

kubectl -n "${NAMESPACE}" port-forward service/control-plane-api 8080:80 >/tmp/golden-path-kind-api-portforward.log 2>&1 &
API_PORT_FORWARD_PID=$!
kubectl -n "${NAMESPACE}" port-forward service/local-oidc 9000:9000 >/tmp/golden-path-kind-oidc-portforward.log 2>&1 &
OIDC_PORT_FORWARD_PID=$!

for _ in $(seq 1 45); do
  if curl -sf "${API_URL}/readyz" >/dev/null && curl -sf "${OIDC_URL}/healthz" >/dev/null; then
    break
  fi
  sleep 1
done

curl -sf "${API_URL}/readyz" >/dev/null
curl -sf "${OIDC_URL}/healthz" >/dev/null

export APP_ENV=development
export APP_LOG_LEVEL=INFO
export AUTH_MODE=oidc
export AUTH_AUDIENCE="${AUTH_AUDIENCE:-golden-path-control-plane}"
export AUTH_ISSUER="${AUTH_ISSUER:-http://localhost:9000}"
export AUTH_OIDC_ISSUER_URL="${AUTH_OIDC_ISSUER_URL:-http://localhost:9000}"
export AUTH_OIDC_JWKS_URL="${AUTH_OIDC_JWKS_URL:-http://localhost:9000/keys}"
export CONTROL_PLANE_API_URL="${API_URL}"
export CONTROL_PLANE_TOKEN
CONTROL_PLANE_TOKEN="$(go run ./cmd/cli token --subject smoke@example.com --role platform-admin)"

SMOKE_SKIP_RUNTIME_START=1 SMOKE_SKIP_MIGRATE=1 ./scripts/smoke.sh

echo "kind smoke test passed"
