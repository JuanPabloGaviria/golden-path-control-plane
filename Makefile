GO ?= go
PKGS := ./...
INTEGRATION_DATABASE_URL ?=

.PHONY: fmt lint test integration integration-race race preflight smoke smoke-compose smoke-kind build vuln ci

preflight:
	./scripts/preflight.sh

fmt:
	$(GO) fmt $(PKGS)

lint:
	golangci-lint run

test:
	$(GO) test $(PKGS)

integration:
	CONTROL_PLANE_INTEGRATION_DATABASE_URL="$(INTEGRATION_DATABASE_URL)" $(GO) test -tags=integration $(PKGS)

integration-race:
	CONTROL_PLANE_INTEGRATION_DATABASE_URL="$(INTEGRATION_DATABASE_URL)" $(GO) test -race -tags=integration $(PKGS)

race:
	$(GO) test -race $(PKGS)

smoke:
	./scripts/smoke.sh

smoke-compose:
	./scripts/compose_smoke.sh

smoke-kind:
	./scripts/smoke_kind.sh

build:
	$(GO) build ./cmd/api
	$(GO) build ./cmd/worker
	$(GO) build ./cmd/cli
	$(GO) build ./cmd/migrate
	$(GO) build ./cmd/devoidc

vuln:
	govulncheck ./...

ci: fmt lint test race build vuln
