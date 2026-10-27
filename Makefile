GO ?= go
GO_TOOLING ?= $(CURDIR)/scripts/ci/go-tooling.sh
GOLANGCI_LINT ?= bash $(GO_TOOLING) golangci-lint
GOVULNCHECK ?= bash $(GO_TOOLING) govulncheck
GOLANGCI_CONFIG ?= .golangci.yml

.PHONY: check-boundaries
check-boundaries:
	bash scripts/check-hmac-boundary.sh
	bash scripts/ci/check-otel-api-only.sh

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

# race 빌드에서 제외되는 서명기 할당 상한을 검증한다.
.PHONY: test-allocations
test-allocations:
	$(GO) test -count=1 -run '^TestHMACSignerReusesKeySchedule$$' ./internal/client/signing

.PHONY: vulncheck
vulncheck:
	$(GOVULNCHECK) ./...

.PHONY: build
build: lint
	$(GO) build ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy -diff
