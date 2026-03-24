GO ?= go
PKGS := ./...
INTEGRATION_DATABASE_URL ?=
TOOLS_BIN := $(CURDIR)/.tools/bin
BUILD_BIN := $(CURDIR)/bin
GOLANGCI_LINT_VERSION := v2.11.4
GOVULNCHECK_VERSION := v1.1.4
GOLANGCI_LINT := $(TOOLS_BIN)/golangci-lint
GOVULNCHECK := $(TOOLS_BIN)/govulncheck

.PHONY: tools fmt check-fmt lint test integration integration-race race preflight contract render-k8s scan-config scan-image smoke smoke-compose smoke-kind build vuln ci

tools: $(GOLANGCI_LINT) $(GOVULNCHECK)

$(TOOLS_BIN):
	mkdir -p $(TOOLS_BIN)

$(GOLANGCI_LINT): | $(TOOLS_BIN)
	GOBIN="$(TOOLS_BIN)" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOVULNCHECK): | $(TOOLS_BIN)
	GOBIN="$(TOOLS_BIN)" $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(BUILD_BIN):
	mkdir -p $(BUILD_BIN)

preflight:
	./scripts/preflight.sh

fmt:
	$(GO) fmt $(PKGS)

check-fmt:
	test -z "$$(gofmt -l .)"

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

test:
	$(GO) test $(PKGS)

integration:
	CONTROL_PLANE_INTEGRATION_DATABASE_URL="$(INTEGRATION_DATABASE_URL)" $(GO) test -tags=integration $(PKGS)

integration-race:
	CONTROL_PLANE_INTEGRATION_DATABASE_URL="$(INTEGRATION_DATABASE_URL)" $(GO) test -race -tags=integration $(PKGS)

race:
	$(GO) test -race $(PKGS)

contract:
	$(GO) test ./internal/api -run TestOpenAPIContractIsValid -count=1

render-k8s:
	kubectl kustomize deployments/kubernetes/overlays/local-kind >/dev/null

scan-config:
	docker run --rm -v "$(CURDIR):/work" -w /work aquasec/trivy:0.62.1 config --severity HIGH,CRITICAL --exit-code 1 .

scan-image:
	docker build --build-arg APP_BIN=api -t golden-path-control-plane-api:scan .
	docker run --rm -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy:0.62.1 image --scanners vuln --severity HIGH,CRITICAL --exit-code 1 golden-path-control-plane-api:scan

smoke:
	./scripts/smoke.sh

smoke-compose:
	./scripts/compose_smoke.sh

smoke-kind:
	./scripts/smoke_kind.sh

build: | $(BUILD_BIN)
	$(GO) build -o "$(BUILD_BIN)/api" ./cmd/api
	$(GO) build -o "$(BUILD_BIN)/worker" ./cmd/worker
	$(GO) build -o "$(BUILD_BIN)/cli" ./cmd/cli
	$(GO) build -o "$(BUILD_BIN)/migrate" ./cmd/migrate
	$(GO) build -o "$(BUILD_BIN)/devoidc" ./cmd/devoidc

vuln: build $(GOVULNCHECK)
	$(GOVULNCHECK) -mode=binary "$(BUILD_BIN)/api"
	$(GOVULNCHECK) -mode=binary "$(BUILD_BIN)/worker"
	$(GOVULNCHECK) -mode=binary "$(BUILD_BIN)/cli"
	$(GOVULNCHECK) -mode=binary "$(BUILD_BIN)/migrate"
	$(GOVULNCHECK) -mode=binary "$(BUILD_BIN)/devoidc"

ci: check-fmt lint test race contract build vuln
