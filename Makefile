GO ?= go
GOLANGCI_LINT ?= golangci-lint
GOLANGCI_CONFIG ?= .golangci.yml

.PHONY: lint
lint:
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

.PHONY: perf-smoke
perf-smoke:
	$(GO) test -run='^$$' -bench='Benchmark(NewSignedRequestHMACSmallJSON|Sha256HexBytesEmpty|SchedulerShardIndex|SendImage_Streaming|ParseSSEStreamRoomEvents)' -benchmem -benchtime=100ms ./...

.PHONY: vulncheck
vulncheck:
	govulncheck ./...

.PHONY: build
build: lint
	$(GO) build ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy
