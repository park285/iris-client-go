# Plan B — Consumer Error Migration (chat-bot-go-kakao + hololive-bot)

**Date:** 2026-05-22
**Status:** Proposed
**Blocked by:** Plan A (sentinel + typed errors must exist)
**Parallel with:** Plan C, Plan D (different files)

---

## Goal

iris-client-go가 Plan A에서 export한 sentinel/typed error를 두 consumer가 채택하여 (1) retry 정책을 정확하게 분류하고 (2) 영구 실패는 즉시 surface하며 (3) failure reason 라벨을 표준화한다.

## Architecture

- **cbgk:** `iris_adapter.go:205`의 `isTransientIrisReplyTransportError(err)` 자체 분류기를 `errors.Is(err, iris.ErrRetryable)`로 대체. auth 실패는 retry하지 않고 즉시 반환. rate-limit은 명시적 backoff 적용.
- **hololive:** `dispatcher_send_flow.go:99`의 `deliveryFailureReason(err)` 분류 함수를 확장하여 iris sentinel(`ErrAuthFailed`, `ErrRateLimited`, `ErrTransport`)을 라벨링. dispatcher의 retry 결정에도 sentinel 사용.
- 두 consumer 모두 변경은 internal helper에 한정. 외부 시그니처 변경 없음.

## Tech Stack

Go 1.22+, `errors.Is`/`errors.As`, iris-client-go v0.NEXT (Plan A 산출).

## Execution

`subagent-driven-development`. cbgk wave와 hololive wave는 독립이므로 worktree로 병렬 실행 가능. 두 wave는 같은 iris-client-go 버전(Plan A 산출)을 사용한다.

---

## Success Criteria

1. cbgk `isTransientIrisReplyTransportError`가 제거되고 `errors.Is(err, iris.ErrRetryable)`로 대체됨.
2. cbgk `withTransientReplyRetry`가 `iris.ErrAuthFailed`/`iris.ErrPermanent`에 대해 retry하지 않고 즉시 반환함.
3. cbgk 신규 단위 테스트: 401/403 응답 시 1회만 호출되고 즉시 ErrAuthFailed surface. 503 응답 시 retry 동작.
4. hololive `deliveryFailureReason`가 `auth`, `rate-limited`, `transport`, `http-permanent` 라벨을 반환.
5. hololive 신규 단위 테스트: sentinel별 라벨 매핑 검증.
6. 두 레포 `go test ./... -race` 통과.
7. metrics/log에서 retry 카운트 패턴이 변경됨을 production 관측으로 확인(별도 runbook).

## File Map (cbgk)

- **Modify:** `chat-bot-go-kakao/internal/bot/iris_adapter.go:194-219` — `withTransientReplyRetry` 분류 로직 교체.
- **Modify/Delete:** `chat-bot-go-kakao/internal/bot/iris_adapter.go` — `isTransientIrisReplyTransportError` 제거(또는 thin wrapper로 1버전 유지).
- **Test:** `chat-bot-go-kakao/internal/bot/iris_adapter_test.go` — sentinel 분류 시나리오 추가.

## File Map (hololive)

- **Modify:** `hololive-bot/hololive/hololive-shared/pkg/service/youtube/outbox/internal/delivery/dispatcher_send_flow.go:99-104` — `deliveryFailureReason` 확장.
- **Modify:** `hololive-bot/hololive/hololive-shared/pkg/service/youtube/outbox/internal/delivery/dispatcher_send.go:298,499` — sentinel 기반 retry 결정 추가.
- **Test:** `hololive-bot/hololive/hololive-shared/pkg/service/youtube/outbox/internal/delivery/dispatcher_send_flow_test.go` (없으면 생성) — sentinel→라벨 매핑.

---

## Task 1 (cbgk) — withTransientReplyRetry sentinel 채택

**Files:**
- Modify: `chat-bot-go-kakao/internal/bot/iris_adapter.go`
- Test: `chat-bot-go-kakao/internal/bot/iris_adapter_test.go`

- [ ] **Step 1.1: failing test 작성**

```go
func TestWithTransientReplyRetry_RetriesOnIrisErrRetryable(t *testing.T) {
	var attempts int32
	sender := newTestSender(t, func() error {
		atomic.AddInt32(&attempts, 1)
		return fmt.Errorf("wrap: %w", &iris.HTTPError{StatusCode: 503, URL: "x"})
	})

	err := sender.withTransientReplyRetry(context.Background(), sender.testSend)
	require.Error(t, err)
	require.GreaterOrEqual(t, atomic.LoadInt32(&attempts), int32(2), "expected retry on ErrRetryable")
}

func TestWithTransientReplyRetry_DoesNotRetryOnIrisErrAuthFailed(t *testing.T) {
	var attempts int32
	sender := newTestSender(t, func() error {
		atomic.AddInt32(&attempts, 1)
		return fmt.Errorf("wrap: %w", &iris.HTTPError{StatusCode: 401, URL: "x"})
	})

	err := sender.withTransientReplyRetry(context.Background(), sender.testSend)
	require.Error(t, err)
	require.True(t, errors.Is(err, iris.ErrAuthFailed))
	require.Equal(t, int32(1), atomic.LoadInt32(&attempts), "401 must not retry")
}

func TestWithTransientReplyRetry_DoesNotRetryOnIrisErrPermanent(t *testing.T) {
	var attempts int32
	sender := newTestSender(t, func() error {
		atomic.AddInt32(&attempts, 1)
		return fmt.Errorf("wrap: %w", &iris.HTTPError{StatusCode: 400, URL: "x"})
	})

	err := sender.withTransientReplyRetry(context.Background(), sender.testSend)
	require.Error(t, err)
	require.True(t, errors.Is(err, iris.ErrPermanent))
	require.Equal(t, int32(1), atomic.LoadInt32(&attempts), "400 must not retry")
}
```

`newTestSender`는 기존 test helper 패턴을 따라 작성. iris module 경로는 `github.com/park285/iris-client-go/iris`.

- [ ] **Step 1.2: run, confirm RED**

Run: `cd /home/kapu/work/iris-stack/chat-bot-go-kakao && go test ./internal/bot -run TestWithTransientReplyRetry -v`
Expected: FAIL — `iris.ErrAuthFailed`/`iris.ErrPermanent` 미존재 (Plan A 이전이면) 또는 retry 로직이 sentinel 인지 못 함.

> **Stop rule:** Plan A 미배포면 여기서 중단. go.mod 업데이트로 새 SDK 버전 확인 후 진행.

- [ ] **Step 1.3: iris_adapter.go:205 로직 교체**

```go
// 기존
if !isTransientIrisReplyTransportError(err) || attempt == policy.maxAttempts {
	return fmt.Errorf("send iris reply attempt %d: %w", attempt, err)
}

// 신규
if !errors.Is(err, iris.ErrRetryable) || attempt == policy.maxAttempts {
	return fmt.Errorf("send iris reply attempt %d: %w", attempt, err)
}
```

- [ ] **Step 1.4: `isTransientIrisReplyTransportError` 정의 처리**

전체 호출처를 grep. 다른 곳에서 쓰지 않으면 삭제. 쓰면 본문을 `errors.Is(err, iris.ErrRetryable)`로 바꾸고 1버전 후 제거 예정 주석 추가.

- [ ] **Step 1.5: run, confirm GREEN + 기존 회귀 없음**

Run: `cd /home/kapu/work/iris-stack/chat-bot-go-kakao && go test ./internal/bot -count=1 -race`
Expected: all PASS.

- [ ] **Step 1.6: pre-commit gate**

Run: `cd /home/kapu/work/iris-stack/chat-bot-go-kakao && ./scripts/pre-commit-go-checks.sh`
Expected: clean.

---

## Task 2 (cbgk) — `confirmAcceptedDelivery` permanent 실패 즉시 surface

**Files:**
- Modify: `chat-bot-go-kakao/internal/bot/iris_adapter.go:158-180`
- Test: 동일 test 파일

- [ ] **Step 2.1: failing test — accepted=false 시 retry 안 함, accepted=true이지만 후속 poll에서 ErrAuthFailed면 retry 안 함**

```go
func TestConfirmAcceptedDelivery_AuthFailureDoesNotRetry(t *testing.T) {
	// pollResult가 iris.ErrAuthFailed를 반환하도록 fake 구성
	// withTransientReplyRetry 한 번만 호출되는지 검증
}
```

- [ ] **Step 2.2: RED → 구현 → GREEN**

`waitForIrisReplyDelivery` 내부 또는 그 결과를 받아서 errors.Is로 분류, retry loop가 이를 존중하게 함.

---

## Task 3 (hololive) — `deliveryFailureReason` sentinel 라벨 확장

**Files:**
- Modify: `hololive-bot/hololive/hololive-shared/pkg/service/youtube/outbox/internal/delivery/dispatcher_send_flow.go:99-104`
- Test: `dispatcher_send_flow_test.go` (없으면 생성)

- [ ] **Step 3.1: failing test 작성**

```go
package delivery

import (
	"errors"
	"fmt"
	"testing"

	"github.com/park285/iris-client-go/iris"
)

func TestDeliveryFailureReason_ClassifiesIrisSentinels(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"auth", fmt.Errorf("wrap: %w", &iris.HTTPError{StatusCode: 401}), "auth"},
		{"rate-limited", fmt.Errorf("wrap: %w", &iris.HTTPError{StatusCode: 429}), "rate-limited"},
		{"transport", fmt.Errorf("wrap: %w", &iris.TransportError{Op: "dial", Err: errors.New("conn refused")}), "transport"},
		{"http-permanent", fmt.Errorf("wrap: %w", &iris.HTTPError{StatusCode: 400}), "http-permanent"},
		{"dedupe-key", ErrDeliveryDedupeKeyRequired, "dedupe key"},
		{"generic", errors.New("boom"), "send message"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deliveryFailureReason(tc.err)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3.2: run, confirm RED**

Run: `cd /home/kapu/work/iris-stack/hololive-bot && go test ./hololive/hololive-shared/pkg/service/youtube/outbox/internal/delivery -run TestDeliveryFailureReason -v`

- [ ] **Step 3.3: implement deliveryFailureReason 확장**

```go
func deliveryFailureReason(err error) string {
	if errors.Is(err, ErrDeliveryDedupeKeyRequired) {
		return "dedupe key"
	}
	switch {
	case errors.Is(err, iris.ErrAuthFailed):
		return "auth"
	case errors.Is(err, iris.ErrRateLimited):
		return "rate-limited"
	case errors.Is(err, iris.ErrTransport):
		return "transport"
	case errors.Is(err, iris.ErrPermanent):
		return "http-permanent"
	}
	return "send message"
}
```

`iris` import 추가 필요.

- [ ] **Step 3.4: GREEN**

Expected: 모든 케이스 PASS.

---

## Task 4 (hololive) — dispatcher retry 결정에 sentinel 사용

**Files:**
- Modify: `dispatcher_send.go` (sendDeliveryMessage 호출 결과 분류)
- Test: 기존 dispatcher 테스트 보강

- [ ] **Step 4.1: failing test — auth 실패 시 retry queue에 다시 들어가지 않음**

기존 `dispatcher_send_test.go` 패턴 확인 후, auth 실패가 발생하면 outbox row가 `permanent_failure` 상태로 즉시 전이되는지 검증.

> 사전 조건: dispatcher의 retry 모델이 별도 retry queue/state machine이라면 이 task는 분리. 단순 in-process retry라면 통합.

- [ ] **Step 4.2: 구현** — `sendDeliveryMessage` 결과를 `errors.Is(err, iris.ErrPermanent)` 분기로 처리.

- [ ] **Step 4.3: GREEN + 기존 dispatcher 테스트 회귀 없음**

Run: `cd /home/kapu/work/iris-stack/hololive-bot && go test ./hololive/hololive-shared/pkg/service/youtube/outbox/... -count=1 -race`

---

## Task 5 — 두 레포 go.mod 동기화

**Files:**
- Modify: `chat-bot-go-kakao/go.mod` (iris-client-go 버전 bump)
- Modify: `hololive-bot/go.mod`, `hololive-bot/hololive/*/go.mod` (각 모듈)

- [ ] **Step 5.1:** Plan A에서 결정된 새 버전(v0.NEXT)으로 `go get github.com/park285/iris-client-go@v0.NEXT`. `go mod tidy`.

- [ ] **Step 5.2:** `go.work`이 local path resolve 중이면 그대로. 별도 commit으로 분리하지 않음(같은 PR 안에서 처리).

- [ ] **Step 5.3:** `go build ./...` 양쪽 통과.

---

## Validation

```bash
# cbgk
cd /home/kapu/work/iris-stack/chat-bot-go-kakao
go test ./... -count=1 -race
./scripts/pre-commit-go-checks.sh

# hololive
cd /home/kapu/work/iris-stack/hololive-bot
go test ./... -count=1 -race
./build-all.sh --no-bump
```

Expected: 양쪽 모두 PASS.

## Deploy Ordering (risk-gate #4)

배포 순서 강제:

1. **iris-client-go release tag** (v0.NEXT) push — Plan A 산출. 이게 먼저 published 되어야 cbgk/hololive의 `go mod tidy`가 성공.
2. **cbgk PR merge & deploy** — chatbotgo 서비스 무중단 재시작. retry 분류 변경이 들어감.
3. **hololive PR merge & deploy** — youtube-producer/alarm-worker 재시작.

단계 2와 3 사이는 24시간 이상 권장(cbgk soak window). 동시 배포는 retry behavior 변경이 두 서비스에서 동시 발현되어 디버깅 곤란.

## Operations Monitor (risk-gate #5)

배포 후 1주 soak. 다음 metric 관측 owner: **사용자님 또는 지정 운영자**.

- `iris_reply_retry_attempts_total` (cbgk): 401/400 발생 빈도 — retry count가 attempt당 1로 떨어지는지.
- `iris_reply_4xx_surface_total` (cbgk): 영구 실패가 즉시 surface되는지(retry 전부 소진 후 surface가 아니라).
- hololive `delivery_failure_reason{reason="auth|rate-limited|transport|http-permanent"}` cardinality: 신규 라벨이 정상 발현되는지, "send message" 라벨이 줄어드는지.
- 비정상 시 24시간 내 rollback 결정. 별도 incident channel 사용.

## Stop Rules

- **Plan A 미배포:** 새 sentinel 미존재면 즉시 중단. Plan A 완료 확인 후 재시작.
- **기존 retry 횟수가 의도적으로 401에도 retry하던 의존성 발견:** production behavior 변경이 의도와 다르면 Task 1 결정 재검토 후 별도 PR로 retry 정책 조정.
- **hololive dispatcher가 별도 retry queue를 가짐:** Task 4 단순화 가정이 깨지면 task를 retry-queue 모델용 별도 task로 분리.

## Risk Gates

| Gate | Trigger | Mitigation |
|---|---|---|
| **Retry behavior change** | 401/400에서 retry 안 함으로 변경 | production metric 비교(retry count, 4xx surface count). 1주 soak. |
| **Cross-repo dep bump** | iris-client-go 새 버전 채택 | 두 레포 동시 PR. go.work이 있어 local build OK이나 CI는 release 버전을 봄. 미배포 시 break. |
| **Failure label cardinality** | metrics 라벨 `auth`/`rate-limited`/`transport`/`http-permanent` 추가 | 기존 라벨과 cardinality 충돌 없음 확인. 대시보드 split 필요 시 follow-up. |
