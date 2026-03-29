# Verification Matrix

Back to the flagship documents:

- [Main README](../README.md)
- [Kubernetes proof surface](../deployments/kubernetes/README.md)

This matrix ties every public claim to a command or a test. If a claim does not appear here, it should not appear in the README as established behavior.

## Claim-To-Proof Map

| Claim | Proof |
| --- | --- |
| API, worker, CLI, migrator, and local OIDC issuer build cleanly | `go build ./cmd/api ./cmd/worker ./cmd/cli ./cmd/migrate ./cmd/devoidc` |
| Config fails fast on invalid or production-unsafe settings | `go test ./internal/config` |
| HMAC and OIDC validation reject bad issuer, audience, expiry, and role claims | `go test ./internal/auth ./internal/api` |
| JSON decoding rejects ambiguous request bodies | `go test ./internal/httpx` |
| Schema lifecycle is explicit and runtime boot refuses an unmigrated database | `go test -tags=integration ./internal/migrations ./internal/app` |
| Readiness scoring and deployment gating survive race checks in real persistence | `go test -race -tags=integration ./...` |
| Direct local runtime flow works with explicit migration and real binaries | `make smoke` |
| Compose runtime works with PostgreSQL and local OIDC/JWKS proof mode | `make smoke-compose` |
| Kubernetes assets are exercised in a real local cluster | `make smoke-kind` |
| Kubernetes manifests render cleanly | `kubectl kustomize deployments/kubernetes/overlays/local-kind` |
| Built artifacts are scanned for reachable Go vulnerabilities | `make vuln` |
| Kubernetes and Docker assets are scanned for high and critical misconfiguration findings | `make scan-config` |
| Built images are scanned for high and critical vulnerabilities | `make scan-image` |

## Reading The Matrix Correctly

- `make smoke` proves the direct local runtime path.
- `make smoke-compose` proves the containerized proof path.
- `make smoke-kind` proves the only Kubernetes deployment shape this repo claims today.
- `make ci` is the broadest gate, but the table above is the more honest map because it keeps each claim attached to its strongest proof.

## Truthfulness Boundaries

### Verified now

- local direct runtime
- Docker Compose runtime
- `kind` runtime
- local OIDC/JWKS auth proof path

### Not claimed now

- managed cloud deployment proof
- enterprise identity provider integration
- managed Postgres backup, failover, or disaster recovery
- production SLO dashboards and alert routing
