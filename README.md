# golden-path-control-plane

`golden-path-control-plane` is a Go-based internal developer platform backend for service onboarding and release readiness.

It focuses on one reviewer-grade flow:

1. Register a service with ownership, operational metadata, and SLO policy.
2. Enqueue a readiness evaluation.
3. Run deterministic golden-path checks in an async worker.
4. Inspect the latest readiness scorecard.
5. Create and evaluate a deployment candidate against the latest readiness state.

This repository does not claim production readiness. It is a strong v1 control plane with honest boundaries, local operability, Kubernetes-ready deployment assets, CI gates, and a runtime-exercised critical flow.

## Requirements

- Go `1.25.8`
- PostgreSQL `16+` for local API, worker, integration tests, and smoke flow
- Docker for [deployments/docker-compose.yml](./deployments/docker-compose.yml) and [scripts/compose_smoke.sh](./scripts/compose_smoke.sh)

## Architecture

- `cmd/api`: HTTP API, health, readiness, metrics.
- `cmd/worker`: async job processor backed by PostgreSQL.
- `cmd/cli`: small operator and developer CLI.
- `internal/app`: use cases and orchestration.
- `internal/auth`: JWT validation for local HMAC and OIDC.
- `internal/config`: fail-fast config contract.
- `internal/domain`: domain models and validation.
- `internal/httpx`: HTTP helpers, middleware, and error envelopes.
- `internal/migrations`: embedded schema migrations.
- `internal/observability`: logging, tracing, and metrics.
- `internal/platformchecks`: deterministic readiness rules.
- `internal/postgres`: PostgreSQL persistence and job queue semantics.

## Config

The committed contract lives in [`.env.example`](./.env.example).

- `.env.example` is documentation only.
- `.env` is ignored and must never be committed.
- The binaries read configuration from environment variables.
- Invalid or placeholder configuration fails boot.
- Secrets are redacted from diagnostics.

Load local configuration with shell environment export, for example:

```bash
set -a
source .env
set +a
```

## Quickstart

1. Start PostgreSQL.
2. Export environment variables from `.env.example`.
3. Run `go run ./cmd/api`.
4. Run `go run ./cmd/worker`.
5. Use `go run ./cmd/cli --help`.

For a full local smoke flow, ensure `DATABASE_URL` points to a running PostgreSQL instance and run `make smoke`.

For a Docker-backed proof that exercises PostgreSQL, API, and worker in containers, run `make smoke-compose`.

For a containerized local stack, use [deployments/docker-compose.yml](./deployments/docker-compose.yml).

## Quality Gates

- formatting: `make fmt`
- lint: `make lint`
- unit tests: `make test`
- integration tests: `make integration INTEGRATION_DATABASE_URL=...`
- race tests: `make race`
- build: `make build`
- vulnerability scan: `make vuln`

## Public API

- `POST /v1/services`
- `PATCH /v1/services/{service_id}`
- `POST /v1/services/{service_id}/evaluations`
- `GET /v1/services/{service_id}/scorecard`
- `POST /v1/deployment-candidates`
- `POST /v1/deployment-candidates/{candidate_id}/evaluate`
- `GET /v1/deployment-candidates/{candidate_id}`
- `GET /v1/audit-events`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
