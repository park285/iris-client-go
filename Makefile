GO ?= go
GO_TOOLING ?= $(CURDIR)/scripts/ci/go-tooling.sh
GOLANGCI_LINT ?= bash "$(abspath $(GO_TOOLING))" golangci-lint
GOVULNCHECK ?= bash "$(abspath $(GO_TOOLING))" govulncheck
GOLANGCI_CONFIG ?= .golangci.yml
MODULES ?= . $(patsubst %/go.mod,%,$(wildcard adapters/valkeydedup/go.mod))
export GOWORK := off

define for_each_module
set -e; for module in $(MODULES); do (cd "$$module" && $(1)); done
endef

.PHONY: lint
lint:
	$(call for_each_module,$(GOLANGCI_LINT) run -c "$(abspath $(GOLANGCI_CONFIG))" ./...)

.PHONY: fmt
fmt:
	$(call for_each_module,$(GOLANGCI_LINT) run -c "$(abspath $(GOLANGCI_CONFIG))" --fix ./...)

.PHONY: test
test:
	$(call for_each_module,$(GO) test ./...)

.PHONY: test-adapter-contract
test-adapter-contract:
ifneq ($(filter adapters/valkeydedup,$(MODULES)),)
	bash scripts/ci/check-adapter-contract.sh
endif

.PHONY: test-race
test-race: test-adapter-contract
	$(call for_each_module,$(GO) test -race -count=1 ./...)

# race 빌드에서 제외되는 서명기 할당 상한을 검증한다.
.PHONY: test-allocations
test-allocations:
ifneq ($(filter .,$(MODULES)),)
	$(GO) test -count=1 -run '^TestHMACSignerReusesKeySchedule$$' ./internal/irishmac
endif

.PHONY: vulncheck
vulncheck:
	$(call for_each_module,$(GOVULNCHECK) ./...)

.PHONY: build
build: lint
	$(call for_each_module,$(GO) build ./...)

.PHONY: tidy
tidy:
	$(call for_each_module,$(GO) mod tidy -diff)
