# Plan G — Multipart Streaming Split (P2.1 only)

**Date:** 2026-05-22 (작성), 2026-05-23 (사용자 승인 — Plan A에서 분리)
**Status:** Implemented
**Origin:** Plan A Task 4 split. Plan A stop rule "streaming 회귀 시 P2.1만 별도 PR로 split" 발동.
**Blocked by:** Plan A core (Tasks 1-3, 5, 6) merge + iris-client-go v0.13.0 published
**Independent of:** Plan B, C, D, E, F

---

## Goal

iris-client-go `SendImage`/`SendMultipleImages` 경로의 multipart body 구성을 `io.Pipe` 기반 streaming으로 전환하되, allocs/op 회귀를 0 또는 감소로 잡는다.

## Background

Plan A Task 4 첫 구현 결과:
- `BenchmarkSendImage_BufferedBaseline`: 19,292,467 ns/op, 25,340,069 B/op, 136 allocs/op
- `BenchmarkSendImage_Streaming`: 9,904,397 ns/op, **223,778 B/op (-99%)**, **174 allocs/op (+28%)**

B/op은 대폭 감소했으나 allocs/op이 risk-gate 정의(`alloc count 50%+ 감소`)와 충돌. 사용자 결정 2026-05-23: Plan A 본PR에서 split.

## Architecture

- 단계 1: 이전 시도(`client/multipart_writer.go`)의 alloc 증가 원인 프로파일링. 추정 원인: pipe writer + multipart writer 양쪽이 작은 chunk마다 새 buffer alloc.
- 단계 2: alloc 절감 전략 (둘 중 하나):
  - (a) `sync.Pool`로 multipart writer + intermediate buffer 재사용
  - (b) pre-sized `bytes.Buffer` + manual boundary write (multipart writer 우회)
  - (c) zero-copy 가능 지점만 streaming, 나머지는 buffered hybrid

## Tech Stack

Go 1.22+, `mime/multipart`, `io`, `sync.Pool`, `runtime/pprof`.

## Execution

`subagent-driven-development` — 단일 worker (iris-client-go) + 1 reviewer.

---

## Success Criteria

1. `BenchmarkSendImage_Streaming` allocs/op ≤ `BenchmarkSendImage_BufferedBaseline` allocs/op (회귀 0)
2. B/op는 baseline 대비 90%+ 감소 유지
3. `SendImage` 회귀 테스트 전부 PASS
4. retry-safe body factory 동작 유지 (두 번째 호출에서 정상 stream)
5. integration test (httptest.Server SendImage) 시 chunked transfer encoding 정상 동작

## File Map

- **Modify:** `client/client.go` SendImage/SendMultipleImages
- **Create:** `client/multipart_writer.go` (Plan A에서 revert됐던 파일 재도입)
- **Create:** `client/multipart_writer_test.go`
- **Create:** `client/multipart_writer_bench_test.go`
- **Modify:** `CHANGELOG.md` (multipart streaming entry 추가)

---

## Task 1 — alloc profiling

- [x] **Step 1.1:** baseline + naive streaming benchmark with `-benchmem` and `-memprofile`
- [x] **Step 1.2:** `go tool pprof -alloc_objects` 분석 — 증가한 38개 allocs/op 출처 식별

> note: RED 재측정은 worktree 위치가 parent `go.work` 아래라 `GOWORK=off`로 실행했다. RoundTripper sink 기준 baseline은 101 allocs/op, naive `io.Pipe` + `mime/multipart` streaming은 464-466 allocs/op였다. `go tool pprof -alloc_objects` 상위 출처는 `mime/multipart.(*Writer).CreatePart`, `mime/multipart.(*Writer).Close`, `net/http` chunked request 처리, `sync.(*Pool).pinSlow`였다.

## Task 2 — fix 적용

- [x] **Step 2.1:** sync.Pool 또는 pre-sized buffer 전략 적용
- [x] **Step 2.2:** benchmark 재측정 — allocs/op 회귀 없음 확인
- [x] **Step 2.3:** retry-safe body factory + integration test PASS

> note: 선택 전략은 (c) hybrid다. `mime/multipart`/`io.Pipe` 대신 manual boundary chunk reader로 metadata/header bytes만 사전 구성하고 image payload는 복사 없이 읽는다. 최종 `BenchmarkSendImage_Streaming`은 70-71 allocs/op, 8,456-13,734 B/op로 기준을 통과했다.

## Task 3 — CHANGELOG 갱신

- [x] **Step 3.1:** Plan A에서 제거된 multipart streaming entry 재추가 — v0.14.0 또는 v0.13.1 entry로

> note: 사용자 결정 미정 항목은 지시대로 v0.13.1로 기록했다.

---

## Validation

```bash
cd /home/kapu/work/iris-stack/iris-client-go
go test ./client -count=1 -race
go test ./client -bench BenchmarkSendImage -benchmem -count=5
go vet ./... && golangci-lint run ./...
```

## Stop Rules

- alloc 절감 전략으로도 회귀 못 잡으면 P2.1 자체 보류 (기존 buffered 유지)
- retry semantics 깨지면 즉시 중단

## Risk Gates

| Gate | Trigger | Mitigation |
|---|---|---|
| **Performance regression** | allocs/op 증가 | sync.Pool 또는 hybrid 전략 |
| **Retry safety** | factory pattern 동작 변경 | retry-safe body 재호출 테스트 |
| **Integration** | httptest.Server chunked transfer 동작 | integration test |
