GO ?= go
GO_TOOLING ?= $(CURDIR)/scripts/ci/go-tooling.sh
GOLANGCI_LINT ?= bash $(GO_TOOLING) golangci-lint
GOVULNCHECK ?= bash $(GO_TOOLING) govulncheck
GOLANGCI_CONFIG ?= .golangci.yml
VALKEY_TEST_ADDR ?=

.PHONY: check-boundaries
check-boundaries:
	bash scripts/check-hmac-boundary.sh
	bash scripts/check-hmac-boundary_test.sh
	bash scripts/check-legacy-removal.sh

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

# Lua 본문은 실제 Valkey에서만 평가된다. 주소가 비면 통합 테스트가 조용히 skip되므로
# 여기서 먼저 실패시키고, -v로 실행/skip 여부가 로그에 남게 한다.
.PHONY: test-valkey
test-valkey:
	@test -n "$(VALKEY_TEST_ADDR)" || { \
		echo "VALKEY_TEST_ADDR is required (e.g. make test-valkey VALKEY_TEST_ADDR=127.0.0.1:6399)" >&2; \
		exit 1; \
	}
	IRIS_CLIENT_VALKEY_TEST_ADDR=$(VALKEY_TEST_ADDR) $(GO) test -race -count=1 -v ./internal/dedup/

.PHONY: vulncheck
vulncheck:
	$(GOVULNCHECK) ./...

.PHONY: build
build: lint
	$(GO) build ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy -diff
