# Verification Matrix

This repository claims only what is exercised locally or in CI-equivalent commands.

| Claim | Proof |
| --- | --- |
| API, worker, CLI, migrator, and local OIDC issuer build cleanly | `go build ./cmd/api ./cmd/worker ./cmd/cli ./cmd/migrate ./cmd/devoidc` |
| Config fails fast on invalid or production-unsafe settings | `go test ./internal/config` |
| HMAC and OIDC auth validation reject bad issuer, audience, expiry, and role claims | `go test ./internal/auth ./internal/api` |
| JSON decoding rejects ambiguous multi-document bodies | `go test ./internal/httpx` |
| Database schema lifecycle is explicit and runtime bootstrap refuses an unmigrated database | `go test -tags=integration ./internal/migrations ./internal/app` |
| Core control-plane flow is race-checked in real persistence | `go test -race -tags=integration ./...` |
| Direct local runtime flow works with explicit migration and local binaries | `make smoke` |
| Containerized runtime proof works with OIDC and PostgreSQL in Docker Compose | `make smoke-compose` |
| Kubernetes deployment assets are not theater and work in a real local cluster | `make smoke-kind` |
| Kubernetes manifests render cleanly | `kubectl kustomize deployments/kubernetes/overlays/local-kind` |
| Dependency vulnerabilities are checked in Go modules | `govulncheck ./...` |

## Truthfulness Boundaries

- Verified now:
  - Local direct runtime
  - Docker Compose runtime
  - kind-based Kubernetes runtime
  - OIDC/JWKS validation path using the local proof issuer
- Not claimed yet:
  - Managed cloud deployment proof
  - External enterprise identity provider integration
  - Managed Postgres operations, backup, or disaster recovery
  - Production SLO dashboards and alerting
