GO ?= go
SHELL := /bin/bash
$(foreach option,GO_TOOLING GOVULNCHECK GOLANGCI_LINT GO_LINT_MATRIX lint_binary \
  GOLANGCI_CONFIG GOLANGCI_LINT_VERSION GOVULNCHECK_VERSION,\
  $(if $(filter-out undefined,$(origin $(option))),$(error $(option) override is unsupported)))
export MODULES
override MODULE_LIST = bash scripts/ci/go-module-dirs.sh $@ "$$MODULES"
override GOWORK := off
export GOWORK
unexport CDPATH

override load_modules = set -e; directories="$$($(MODULE_LIST))"; dirs=(); while IFS= read -r module; do dirs+=("$$module"); done <<< "$$directories"
override run_in_modules = for module in "$${dirs[@]}"; do (cd -- "$$module" && $(1)); done
override run_if_selected = selected="$(1)"; for module in "$${dirs[@]}"; do \
	if [[ "$$module" == "$$selected" ]]; then $(2); break; fi; done
for_each_module = $(load_modules); $(call run_in_modules,$(1))
override resolve_tool = version=$$(scripts/ci/pre-push-gate.sh --$(1)-version); \
  bin=$$(scripts/ci/python-runner.sh -- -I scripts/ci/resolve-go-tool.py --tool $(1) --version "$$version")

.PHONY: lint
lint:
	@$(load_modules); $(call resolve_tool,golangci-lint); \
		scripts/ci/python-runner.sh -- -I scripts/ci/go-lint-matrix.py \
			--golangci-lint "$$bin" --golangci-lint-version "$$version" --module "$${dirs[@]}"

.PHONY: fmt
fmt:
	@$(load_modules); $(call resolve_tool,golangci-lint); config="$$(pwd -P)/.golangci.yml"; \
		$(call run_in_modules,"$$bin" run -c "$$config" --fix ./...)

.PHONY: test
test:
	$(call for_each_module,$(GO) test ./...)

.PHONY: test-adapter-contract
test-adapter-contract:
	bash scripts/ci/check-adapter-contract.sh

.PHONY: test-race
test-race:
	@$(load_modules); $(call run_if_selected,$$(pwd -P)/adapters/valkeydedup,bash scripts/ci/check-adapter-contract.sh); \
		$(call run_in_modules,$(GO) test -race ./...)

.PHONY: test-allocations
test-allocations:
	@$(load_modules); $(call run_if_selected,$$(pwd -P),$(GO) test -run '^TestHMACSignerReusesKeySchedule$$' ./internal/irishmac)

.PHONY: vulncheck
vulncheck:
	@$(load_modules); $(call resolve_tool,govulncheck); \
		$(call run_in_modules,"$$bin" ./...)

.PHONY: build
build: $(if $(wildcard .git),lint)
	$(call for_each_module,$(GO) build ./...)

.PHONY: tidy
tidy:
	$(call for_each_module,$(GO) mod tidy -diff)
