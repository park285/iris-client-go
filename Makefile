GO ?= go
GO_TOOLING ?= $(CURDIR)/scripts/ci/go-tooling.sh
GOLANGCI_LINT ?= bash $(GO_TOOLING) golangci-lint
GOVULNCHECK ?= bash $(GO_TOOLING) govulncheck
GOLANGCI_CONFIG ?= .golangci.yml

.PHONY: check-boundaries
check-boundaries:
	bash scripts/check-hmac-boundary.sh
	bash scripts/check-hmac-boundary_test.sh

.PHONY: lint
lint: check-boundaries
	$(GOLANGCI_LINT) run -c $(GOLANGCI_CONFIG) ./...

.PHONY: fmt
fmt:
	$(GOLANGCI_LINT) run -c $(GOLANGCI_CONFIG) --fix ./...

.PHONY: test
test:
	$(GO) test ./...

.PHONY: test-race
test-race:
	$(GO) test -race -count=1 ./...

.PHONY: vulncheck
vulncheck:
	$(GOVULNCHECK) ./...

.PHONY: build
build: lint
	$(GO) build ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy -diff
