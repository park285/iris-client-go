# iris-client-go

Iris (카카오톡 메시지 브릿지)용 Go 클라이언트 라이브러리 SDK입니다.

## 설치 (Installation)

```bash
go get github.com/park285/iris-client-go/v2@latest
```

현재 지원 major는 `/v2`입니다. v1 또는 v0 module path에서 올라오는 경우 먼저
[`v2 마이그레이션 가이드`](./docs/MIGRATION-v2.0.0.md)를 따르고, 최근 변경은
[`CHANGELOG.md`](./CHANGELOG.md)의 `미출시`와 가장 최근 release 섹션을 확인하십시오.
[`v0.11 마이그레이션`](./docs/MIGRATION-v0.11.0.md)은 v0 사이의 이전만 기록한
역사 문서이며 현재 v2 업그레이드 가이드가 아닙니다.

## JSON 계약

SDK의 JSON 실행 경로는 Go 1.27 `encoding/json/v2`를 사용합니다. 디코더는 중복된 object
이름과 잘못된 UTF-8을 거절하고 struct field 이름을 대소문자까지 정확히 일치시킵니다. webhook
request와 HTTP response body는 하나의 완전한 JSON 값이어야 하며, media/reaction 응답의 닫힌
경계는 알 수 없는 field도 거절합니다.

typed JSON 응답의 공통 decode 경로는 압축 해제 후 본문 전체를 **16 MiB**로 제한합니다.
정확히 상한인 응답은 허용하고 초과 확인에 필요한 1 byte까지만 추가로 읽습니다.
raw JSON과 strict media/reaction 경로의 기존 1 MiB 상한 및 trailer의 별도 byte/time
제한은 유지합니다. 상한 초과는 `errors.Is(err, iris.ErrResponseTooLarge)`로 식별합니다.
공통 decode 경로의 POST 초과 응답은 서버 처리 결과를 확인할 수 없어 `iris.ErrTransport`도 유지하지만
`iris.ErrRetryable`로 분류하지 않고, idempotency key가 있어도 SDK가 자동 재시도하지 않습니다.
이는 이전에 허용하던 16 MiB 초과 typed 응답의 지원을 제한하는 변경입니다.
정책 근거는 `DEC-20260906-sdk-typed-json-response-budget`입니다.

v2 기본값에 따라 nil slice와 map은 각각 `[]`와 `{}`로 인코딩됩니다. `omitempty` field는
JSON 관점에서 빈 값일 때 생략되며, 숫자와 bool의 기존 zero-value 생략 계약은 `omitzero`로
명시되어 있습니다. 공개 payload의 `encoding/json.RawMessage` 명명 타입은 호환성을 위해
유지하지만, 해당 값을 처리하는 실행 경로는 v2입니다.

## 제네릭 메서드

타입 파라미터가 필요한 메서드는 Go 1.27 제네릭 메서드를 그대로 사용합니다.
`internal/client/transport`의 `doGet[T]`, `doSignedJSON[T]`, `postStrictJSON[T]`, `postJSON[T]`,
`retryPostJSON[T]`가 그렇습니다. 이들을 감싸는 패키지 수준 제네릭 함수 별칭이나 호환 래퍼는
두지 않습니다.

`.golangci.yml`의 `exclusions.rules`와 `exclusions.paths`에는 항목을 추가하지 않습니다.
`generated`와 `warn-unused`를 제외하면 비어 있어야 합니다(DEC-20260824-golangci-suppression-must-be-local).
위반은 먼저 코드로 고치고, 진짜 오탐인 경우에만 해당 줄의 `//nolint`·`#nosec` 주석에 사유를 적어
억제합니다.

## 빠른 시작 (Quick Start)

### 1. 메시지 발송 (Sending Messages)

```go
import "github.com/park285/iris-client-go/v2/iris"

c, err := iris.NewClient()
if err != nil {
    log.Fatalf("클라이언트 초기화 실패: %v", err)
}

// 텍스트 메시지 발송
err = c.SendMessage(ctx, "room-id", "Hello, World!",
    iris.WithThreadID("12345"),
)

// 이미지 메시지 발송 (Base64 인코딩 데이터)
err = c.SendImage(ctx, "room-id", base64Img)

// 마크다운 메시지 발송 (텍스트 공유 카드 형태)
resp, err := c.SendMarkdown(ctx, "room-id", "**bold** text")
status, err := c.GetReplyStatus(ctx, resp.RequestID)

// 일반 파일 발송 (메모리 데이터 예시)
file := iris.NewReplyFileBytes("report.txt", "text/plain", []byte("report body"))
accepted, err := c.SendFile(ctx, "room-id", file,
    iris.WithClientRequestID("report:room-id:2026-07-22"),
)
```

파일 전송은 기존 `iris.Sender`를 확장하지 않는 별도 `iris.FileSender` capability입니다. SDK는
1 byte 이상 30 MiB 이하의 단일 file part를 `multipart/form-data`로 스트리밍하며 전체 파일이나
multipart body를 메모리에 복제하지 않습니다. caller-owned `io.ReaderAt`, path helper의 descriptor
수명, deterministic retry와 `clientRequestId` 계약은 [파일 reply 전송](docs/file-replies.md)을
참조하십시오.

Iris가 structured HTTP error를 반환하면 기존처럼 `errors.As(err, &httpErr)`로
`*iris.HTTPError`를 얻을 수 있습니다. 예외가 하나 있습니다. `WithReplyRetry`로 재시도하는 중에 앞선
시도가 결과 불명(transport 오류)으로 끝나면, 마지막 HTTP 오류가 `CLIENT_REQUEST_ID_*` code를 가진
409가 아닌 한 반환 오류는 `iris.ErrTransport`입니다. 이때 마지막 HTTP 오류는 메시지에만 남아
`*iris.HTTPError`와 code를 얻을 수 없습니다(아래 재시도 문단). `clientRequestId` 상태처럼
machine-readable code가 필요한 호출부는 `iris.HTTPErrorCode(err)`를 사용하십시오. code가 없거나
공개 token 계약을 벗어난 응답이면 빈 문자열을 반환합니다.

`409` 중 `CLIENT_REQUEST_ID_FAILED`는 durable queue handoff 이전 실패 — 즉 해당 id로
KakaoTalk 부수효과가 없었음 — 을 뜻하므로, 새 `clientRequestId`(예: `:r1`, `:r2`
generation suffix)로 **같은 payload**를 유한 세대 재전송하는 것이 안전하며 권장 처리입니다.
같은 409군의 `CLIENT_REQUEST_ID_OUTCOME_UNKNOWN`/`PAYLOAD_MISMATCH`/`ALREADY_EXISTS`와
code 없는 409는 이 보장이 없으므로 재발급 없이 종결하십시오.

`iris.WithReplyRetry(n)`은 `/reply`를 최초 요청을 포함해 최대 `n`번 보냅니다. HTTP 429는 항상,
transport 오류는 `clientRequestId`가 있을 때만 같은 id로 재시도하며 5xx와 그 밖의 4xx는 재시도하지
않습니다. 앞선 시도가 transport 오류로 끝나 결과를 알 수 없는데 이후 시도가 위 409 code 판정 없이
429·5xx·4xx로 끝나면, SDK는 마지막 HTTP 오류로 축약하지 않고 `iris.ErrTransport`(결과 불명)로
반환합니다. 마지막 HTTP 오류는 메시지에만 남습니다. `GetReplyStatus`의 `state`는 Iris wire 값
`queued`, `preparing`, `prepared`, `sending`, `handoff_completed`, `outcome_unknown`, `failed`를
그대로 전달합니다. `outcome_unknown`은 발신 실패가 증명되지 않은 상태이므로 `failed`처럼 재전송하지
마십시오(`DEC-20260731-reply-outcome-unknown-fail-closed`).

### 2. 웹훅 수신 (Receiving Webhooks)

```go
handler, err := iris.NewWebhookHandler(myMessageHandler,
    webhook.WithMessageDeduplicator(valkeydedup.NewMessageDeduplicator(valkeyClient)),
    webhook.WithNonceStore(valkeydedup.NewNonceStore(valkeyClient)),
)
if err != nil {
    log.Fatalf("웹훅 핸들러 생성 실패: %v", err)
}
defer handler.Close()

http.Handle("/webhook/iris", handler)
```

`WithQueueSize`는 ordering scheduler가 소유하는 전체 pending 상한입니다. 내부 실행 pool은 별도 buffered queue를 만들지 않습니다. 종료 budget이 있는 서비스는 `handler.CloseContext(ctx)`를 사용하면 grace 만료 후 queued callback을 건너뛰고 in-flight handler context를 취소할 수 있습니다. `Close()`는 무제한 context로 `CloseContext`를 부르며, `io.Closer` 관례를 따르는 표면이라 폐기 대상이 아닙니다.

HTTP `200 OK`가 메모리 admission이 아니라 durable commit을 의미해야 하는 소비자는 `webhook.MessageAdmitter`를 구현하고 `WithDurableAdmission`을 사용합니다. 이 모드에서는 scheduler와 deduplicator를 건너뛰므로 admitter의 저장소 unique key가 idempotency를 소유합니다.

#### idempotency와 nonce 역할 분리

HMAC nonce와 message dedup은 별도 public contract입니다. 모든 handler constructor는
`webhook.WithNonceStore`로 명시한 `webhook.SetOnceNonceStore`가 없으면 오류를 반환합니다.
process-local default, message backend 재사용, no-op nonce store는 없습니다.

non-durable handler에서 message dedup이 필요하면 token-bound
`webhook.MessageDeduplicator`를 `webhook.WithMessageDeduplicator`로 주입합니다. `Reserve` 오류나
invalid state는 dispatch 전에 `503`으로 종결하며, 반환된 owner token이 있으면 같은 token으로만
bounded `ReleaseReservation`을 시도합니다. durable handler는 inbox unique key가 idempotency를
소유하므로 message deduplicator를 주입하지 않습니다.

Valkey consumer는 같은 client를 사용하더라도 역할별 constructor를 호출합니다.

```go
messageDeduplicator := valkeydedup.NewMessageDeduplicator(valkeyClient)
nonceStore := valkeydedup.NewNonceStore(valkeyClient)
```

예약 TTL은 `WithDedupPendingTTL`(기본 `5s`), 확정 TTL은 `WithDedupTTL`(기본 `16m`)입니다.
`EnqueueTimeout + 2 × DedupTimeout < DedupPendingTTL`과 sender retry horizon보다 긴 확정 TTL을
유지하십시오.

#### HMAC v3-only 계약

receiver는 authority-bound signature v3만 검증하며 v2는 unknown version으로 거절합니다.
`webhooksign.SignRequest`는 v3만 생성합니다. 진단값은 `V3Validated`, `UnknownRejected`,
`MalformedRejected`의 고정 cardinality 필드만 노출합니다.

```go
handler, err := iris.NewWebhookHandler(inboxRuntime,
    webhook.WithDurableAdmission(inboxRuntime),
    webhook.WithNonceStore(nonceStore),
    webhook.WithAdmitTimeout(200 * time.Millisecond),
    webhook.WithWebhookToken("webhook-secret"),
)
```

웹훅 송신 테스트나 smoke 도구에서는 `X-Iris-Message-Id`를 먼저 설정한 뒤 `SignRequest`로
signature v3 header를 생성합니다. `req.Host`가 설정된 경우 URL authority와 canonical parity가
없으면 서명하지 않습니다.

```go
req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
if err != nil {
    return err
}
req.Header.Set(webhook.HeaderIrisMessageID, messageID)
if err := webhooksign.SignRequest(req, secret, body); err != nil {
    return err
}
```

`WithAdmitTimeout`은 durable commit의 deadline입니다. **기본값은 `30s`이며 `0` 이하를 넘겨도 "무제한"이 아니라 이 기본값으로 정규화됩니다.** deadline이 끝나면 다른 admission 오류와 동일하게 HTTP `503 Service Unavailable`을 반환하므로 발신자가 재시도할 수 있습니다. 기본값을 발신자의 attempt timeout(`125s`)보다 훨씬 짧게 잡은 이유는, 저장소가 정체됐을 때 admission goroutine이 요청 context가 끊길 때까지 살아남아 종료(`Close`)까지 지연시키는 대신 빠르게 `503`으로 되돌리기 위해서입니다.

### 3. 관리 API (Admin APIs)

```go
cfg, err := c.GetConfig(ctx)
health, err := c.GetBridgeHealth(ctx)
rooms, err := c.GetRooms(ctx)
members, err := c.GetMembers(ctx, chatID)

// 설정 업데이트 예시
forwardUnmatched := true
_, err = c.UpdateConfig(ctx, "routes", iris.ConfigUpdateRequest{
    CommandRoutePrefixes: map[string][]string{"chatbot": []string{"!", "/"}},
    EventTypeRoutes:      map[string][]string{"events": []string{"member_nickname_updated"}},
    ForwardUnmatchedMessagesToDefault: &forwardUnmatched,
})

// HTTP/3 TLS 인증서 핫 리로드. v2에서는 iris.WithCertReloadToken에 bot-control 자격과 같은 값을
// 넘겨야 합니다(아래 "v2 폐기 예정 표면과 이관" 참고).
_, err = c.ReloadH3Certificate(ctx) // POST /admin/cert-reload
```
* CAS(Compare-And-Swap) 제어가 필요한 경우 `ConfigUpdateRequest.ExpectedRevision`을 명시하여 설정 변경 시의 충돌을 방지할 수 있습니다.

### 4. SSE 이벤트 스트림 (Server-Sent Events)

```go
events, err := c.EventStream(ctx, 0)
for ev := range events {
    fmt.Printf("이벤트 타입: %s, 데이터: %s\n", ev.Event, ev.Data)
}
```

### 5. 조회 API (Query APIs)

```go
// 채팅방 요약 정보 조회
summary, err := c.QueryRoomSummary(ctx, chatID)

// 멤버 통계 조회
stats, err := c.QueryMemberStats(ctx, iris.QueryMemberStatsRequest{
    ChatID: chatID,
    Limit:  20,
})

// 최근 스레드 목록 조회
threads, err := c.QueryRecentThreads(ctx, chatID)

// 최근 메시지 내역 조회
msgs, err := c.QueryRecentMessages(ctx, iris.QueryRecentMessagesRequest{
    ChatID:     chatID,
    Limit:      50,
    ChatLogIDs: []string{"500", "300"}, // 선택적 exact filter, 최대 1,000개
})

// 사용자의 최신 이벤트와 다음 older page 조회
events, err := c.GetRoomUserEventsBefore(ctx, chatID, userID, 500, 0)
if len(events) > 0 {
    older, err := c.GetRoomUserEventsBefore(ctx, chatID, userID, 500, events[len(events)-1].ID)
}

for _, msg := range msgs.Messages {
    fmt.Printf("[%d] %s: %s\n", msg.SequenceID, msg.SenderName, msg.Message)
}
```

### 6. BotClient 및 RebindingClient

다중 인프라 혹은 동적 환경을 지원하기 위해, 봇 서비스를 위한 최소 인터페이스인 `iris.BotClient` (`Sender` + `Ping` + `GetConfig`) 및 동적으로 Base URL을 핫스왑할 수 있는 `iris.RebindingClient`를 제공합니다.

```go
rc := iris.NewRebindingClient(iris.RebindingClientConfig{
    ResolveBaseURL:  func() (string, error) { return readBaseURL() },
    BotToken:        token,
    ResolveInterval: time.Second,      // URL 또는 resolver 오류 snapshot의 최대 유지 시간
    StaleCloseGrace: 30 * time.Second, // 동적 교체된 이전 클라이언트 연결 정리 유예 시간
})
defer rc.Close()
```

`ResolveInterval`이 `0`이면 각 비동시 호출에서 즉시 Base URL을 다시 확인하는 기존 동작을 유지합니다. 양수이면 interval 안의 호출이 마지막 URL 또는 resolver 오류 snapshot을 공유하고 만료 후 첫 호출이 refresh를 수행합니다. 같은 시점의 동시 호출은 하나의 refresh 결과를 공유합니다.

refresh는 개별 API 호출이 아니라 `RebindingClient`가 소유합니다. refresh를 시작한 호출의 context가 취소되어도 해당 호출만 먼저 반환하며 진행 중인 refresh는 다른 동시 호출과 cache snapshot을 위해 완료됩니다. `Close()`는 대기 중인 호출을 즉시 깨우지만 context를 받지 않는 `ResolveBaseURL` 실행을 강제로 중단할 수는 없으므로 resolver는 유한 시간 안에 반환해야 합니다.

클라이언트 초기화 오류는 원본 Base URL을 포함하지 않고 정제된 parse/transport 오류를 감쌉니다. 사용자 제공 resolver가 직접 반환하는 오류의 내용은 resolver가 정제해야 합니다.

---

## 클라이언트 설정 옵션 (Configuration)

```go
c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"), // 또는 IRIS_BASE_URL 환경변수 사용
    iris.WithBotToken("my-token"),              // 또는 IRIS_BOT_TOKEN 환경변수 사용
    iris.WithTimeout(5 * time.Second),
    iris.WithInboundSecret("config-signing-secret"), // /config* 호출이 있을 때만 필요
    iris.WithLogger(slog.Default()),
    iris.WithReplyRetry(3),                     // 최초 요청을 포함한 최대 시도 횟수
    iris.WithTransport("h3"),                   // 또는 IRIS_TRANSPORT 환경변수 사용
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
)
```

### 1. HTTP/3 전송 설정

Iris API의 기본 전송 프로토콜은 HTTP/3(QUIC)입니다. `IRIS_TRANSPORT` 환경 변수가 누락된 경우 기본적으로 `h3` 전송이 적용되며 이 경우 `https://` 스키마가 포함된 Base URL을 설정해야 합니다.

Base endpoint는 `iris.ParseBaseEndpoint`와 모든 client 생성 경로에서 같은 문법으로 검증합니다. 절대 `http`/`https` URL과 host가 필요하며 opaque URL, userinfo, query, fragment는 허용하지 않습니다. 끝의 `/`만 제거하고 `/tenant/iris` 같은 deployment prefix는 보존합니다. 실제 요청 URL에는 고정 API route를 prefix 뒤에 한 번 붙이지만 HMAC canonical target은 계속 `/reply` 같은 API route만 사용합니다.

```go
c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"),
    iris.WithBotToken("my-token"),
    iris.WithTransport("h3"),
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
    iris.WithH3ServerName("iris-host"),
)
defer c.Close()
```

`IRIS_TRANSPORT=h3` 옵션은 `https://` 보안 연결에서만 활성화됩니다. 정본 값은 `h3`와 `http1`입니다. `http3`, `http/3`, `quic`(=`h3`)과 `http`, `http/1.1`(=`http1`) 별칭은 v2에서 계속 인식하지만 폐기 예정이며 다음 coordinated major에서 거절합니다. 현재 Iris runtime에서 `http1`은 loopback의 `GET /health`, `GET /ready` probe와 transport 단위 테스트에만 사용합니다. config, reply, query, diagnostics와 SSE를 포함한 보호 메서드는 `h3`와 `https://` Base URL이 필요합니다. 그 밖의 전송 값은 지원하지 않습니다.

운영 환경에서 H3 egress 대상을 Base URL host로 제한하려면 DNS allowset을 TTL마다 갱신하는 `WithH3DialGuardForBaseURL`을 사용할 수 있습니다. 만료 시 stale allowset이 **허용**하는 dial은 즉시 통과하고 refresh는 뒤에서 끝납니다. stale allowset이 **거부**하는 dial만 그 refresh 결과를 기다렸다 한 번 더 판정하므로, host의 IP가 바뀌어도 TTL 경계의 요청이 `ErrH3EgressDenied`로 희생되지 않습니다. 어느 경우든 동시 dial은 하나의 refresh를 공유하며, allowset이 아직 유효한 동안의 거부는 DNS를 조회하지 않고 즉시 반환합니다. dial의 context가 먼저 취소되면 기다리지 않고 거부합니다. 초기 DNS 해석 실패는 기본적으로 오류를 반환하며 `WithH3DialGuardLenientInit`을 지정하면 deny-all 상태로 기동한 뒤 TTL이 만료된 첫 dial이 refresh를 수행해 자가회복합니다. 엉뚱한 host를 allowlist하지 않도록 `WithH3DialGuardForBaseURL`과 `WithBaseURL`에는 반드시 동일한 Base URL을 전달해야 합니다.

```go
baseURL := "https://iris-host:31001"
dialGuard, err := iris.WithH3DialGuardForBaseURL(
    ctx,
    baseURL,
    iris.WithH3DialGuardTTL(time.Minute),
    iris.WithH3DialGuardResolveTimeout(5*time.Second),
    iris.WithH3DialGuardLogger(logger),
)
if err != nil {
    return err
}
c, err := iris.NewClient(
    iris.WithBaseURL(baseURL),
    iris.WithTransport("h3"),
    dialGuard,
)
```

직접 정책을 구현해야 하는 경우 기존 `WithH3DialGuard` 또는 context 값을 받는 `WithH3DialGuardContext`를 사용할 수 있습니다. guard가 에러를 반환하면 연결은 시도되지 않고 `iris.IsH3EgressDenied(err)`로 분류할 수 있습니다.

### 2. 역할별 비밀키

Iris 서버는 Inbound(`/config*`)와 BotControl(그 밖의 보호 라우트, `/admin/cert-reload` 포함) 두
역할만 두고 역할 사이 폴백이 없습니다(`DEC-20260926-stack-iris-client-go-role-secrets`). SDK의 정본
설정은 두 값입니다.

```go
c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"),
    iris.WithTransport("h3"),
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
    iris.WithBotToken("bot-control-token"),          // BotControl 역할(IRIS_BOT_TOKEN, NewAPIClient의 botToken 인자)
    iris.WithInboundSecret("config-signing-secret"), // Inbound 역할. /config* 호출에서만 필요
)
```

`WithHMACSecret`, `WithBotControlToken`, `WithCertReloadToken`은 폐기 예정입니다. 이관 방법은 아래
"v2 폐기 예정 표면과 이관"에 있습니다.

### 3. 웹훅 핸들러 설정 (Webhook Handler Configuration)

```go
import (
    "github.com/park285/iris-client-go/v2/iris"
    "github.com/park285/iris-client-go/v2/valkeydedup"
    "github.com/park285/iris-client-go/v2/webhook"
)

handler, err := iris.NewWebhookHandler(msgHandler,
    webhook.WithWebhookToken("webhook-secret"),  // 또는 IRIS_WEBHOOK_TOKEN 환경변수 사용
    webhook.WithMessageDeduplicator(valkeydedup.NewMessageDeduplicator(valkeyClient)),
    webhook.WithNonceStore(valkeydedup.NewNonceStore(valkeyClient)),
    webhook.WithDedupTTL(16 * time.Minute),
    webhook.WithWorkerCount(32),                 // Key-ordering 동시성 워커 개수
    webhook.WithQueueSize(2000),
    webhook.WithHandlerTimeout(30 * time.Second),
    webhook.WithMaxBodyBytes(1 << 20),           // 최대 요청 크기 (1MB)
    webhook.WithMetrics(myPrometheusAdapter),
    webhook.WithWebhookLogger(slog.Default()),
)
```

* 웹훅 메시지 스키마(`webhook.Message`/`webhook.MessageJSON`)와 핸들러 옵션(`webhook.WithXxx`)은 `webhook` 패키지에서 직접 import합니다. SDK 진입점인 `iris.NewWebhookHandler`(환경변수 해석·검증 포함)는 `iris` 패키지에 유지되며 Valkey 구현은 `valkeydedup.NewMessageDeduplicator`와 `valkeydedup.NewNonceStore`로 역할을 분리합니다.
* `iris.NewWebhookHandler`와 `iris.NewDurableWebhookHandler`는 각각 `webhook.NewSDKHandler`와 `webhook.NewSDKDurableHandler`의 생성 경로를 사용합니다. 옵션은 순서대로 한 번 적용하고 인증·nonce 검증 뒤 활성화합니다. durable 생성자의 명시적 admitter는 `WithDurableAdmission` 옵션보다 우선합니다. 기존 `webhook.NewHandler`·`NewDurableHandler`는 명시적 context/token/logger를 사용하며 SDK 전용 옵션으로 이를 바꾸지 않습니다. `ResolveSDKConfig`는 환경 해석이나 활성화 없이 옵션을 독립적으로 한 번 적용하는 설정 snapshot입니다.
* optional `sourceCreatedAtMs`는 `WebhookRequest.SourceCreatedAtMS`로 decode되고 durable handler용 `MessageJSON.SourceCreatedAtMS`까지 그대로 전달됩니다. 값은 원본 Kakao row의 초 단위 시각을 millisecond로 표현한 계측 입력이며 message identity나 ordering key가 아닙니다.
* **메시지 순서 보장:** in-memory 모드에서는 기본적으로 동일한 채팅방 또는 동일 스레드 내의 메시지가 순차 처리됩니다. 자체적인 durable scheduler나 분산 큐가 순서를 소유하는 경우 `webhook.WithDurableAdmission`을 사용하거나 `webhook.WithOrderingMode(webhook.OrderingModeNone)`로 in-memory ordering을 끌 수 있습니다.

---

## v2 폐기 예정 표면과 이관 (Deprecations)

아래 표면은 v2에서 그대로 동작하지만 다음 coordinated major에서 삭제하거나 거절합니다
(`DEC-20260926-stack-iris-client-go-compat-surface-retirement`,
`DEC-20260926-stack-iris-client-go-role-secrets`,
`DEC-20260825-iris-client-go-public-surface-major-only`). 심볼에는 godoc `Deprecated:`가 붙어 있어
staticcheck `SA1019`가 사용처를 알려 줍니다. 동작(wire 입력, 폴백)은 해당 godoc에 폐기 예정으로
적혀 있습니다. v2에서 공개 심볼을 지우지 않습니다.

Iris가 이미 서버 쪽을 삭제해 새 Iris에서는 실패하는 표면입니다. 먼저 옮기십시오.

| v2 표면 | 새 Iris에서의 결과 | 이관 |
|------|------|------|
| `SendKaringHololive`, `iris.KaringHololiveRequest`, `iris.PathKaringHololive` (`APIClient`, `RebindingClient`, `KaringClient`) | `/karing/hololive` 삭제, 404 `*HTTPError` | `SendKaringContentList`로 보내고 `Stream`·`Streams`를 `KaringContentListRequest.Items`로 옮깁니다. 한 항목도 길이 1인 목록입니다. 나머지 필드는 같습니다. |
| `KaringContentListRequest.Item` | wire key `item` 삭제, 400 | `Items: []iris.KaringContentItem{item}` |
| `KaringDryRunResponse.StreamCount` | Iris가 보내지 않아 `nil` | `ItemCount`를 읽습니다. 이전 Iris가 보내던 값도 `ItemCount`와 같았습니다. |
| `GetNativeCoreDiagnostics`, `iris.NativeCoreDiagnostics` (`APIClient`, `RebindingClient`, `iris.Client`) | `/diagnostics/native-core` 삭제, 404 `*HTTPError` | `GetRuntimeDiagnostics` 응답의 `nativeCore` 객체를 읽습니다. |

Karing 요청의 `clientRequestId`는 이제 Iris 정본 이름 `client_request_id`로 보냅니다. Iris는 두 이름을
모두 받아 왔으므로(c3069c08부터) 이 변경만으로 이전 Iris와의 호환이 깨지지 않습니다.
`KaringDryRunResponse`는 현재 Iris의 camelCase dry-run 응답(`dryRun`, `templateArgs`, `itemCount` 등)을
정본으로 읽고, 이전 Iris의 snake_case dry-run 응답과 `stream_count`·`streamCount`는 정본 key가 없을
때만 읽습니다. 이 호환 입력은 다음 coordinated major에서 삭제합니다.

자격 이름은 서버의 두 역할에 맞춥니다.

| v2 표면 | 이관 |
|------|------|
| `iris.WithHMACSecret`와 역할별 값이 없을 때의 공유 비밀 폴백 | `/config*` 서명 값은 `WithInboundSecret`, bot-control 서명 값은 `NewAPIClient`의 `botToken` 인자(`NewClient`는 `WithBotToken` 또는 `IRIS_BOT_TOKEN`)로 옮깁니다. |
| `iris.WithBotControlToken` | 넘기던 값을 `botToken` 인자(`WithBotToken`·`IRIS_BOT_TOKEN`)로 옮깁니다. |
| `iris.WithCertReloadToken`, `iris.ErrCertReloadTokenRequired` | v2의 `ReloadH3Certificate`는 이 옵션이 필요하므로 bot-control 자격과 같은 값을 넘깁니다. major에서 `ReloadH3Certificate`가 bot-control 자격으로 서명하므로 그때 이 옵션 호출만 지웁니다. 오류 문자열은 v2에서 바뀌지 않습니다. |
| `IRIS_TRANSPORT`·`WithTransport` 별칭(`http3`, `http/3`, `quic`, `http`, `http/1.1`) | `h3` 또는 `http1`을 씁니다. |

webhook 수신 쪽은 한 모델로 줄이는 동작입니다.

| v2 동작 | 이관 |
|------|------|
| `MessageContext.StableMessageIdentity`의 messageId→sourceLogId→chatLogId 폴백 체인 | `MessageContext.MessageID`를 씁니다. `Handler`가 만든 메시지에는 항상 있습니다. 반환 문자열을 저장해 왔다면 저장 키를 messageId 기준으로 먼저 옮깁니다. |
| `webhook.Message`의 `Msg`·`Room`과 `JSON.Message`·`JSON.ChatID` 이중 필드, `NewMessageContext`의 JSON 우선 폴백 | 두 필드 사이 폴백을 직접 구현하지 말고 `NewMessageContext(msg).Text()`·`RoomID()`로 읽습니다. 남길 필드는 major에서 정합니다. |
| body `messageId`가 비면 `X-Iris-Message-Id` header 값으로 채우는 동작 | 직접 서명해 보내는 테스트·smoke 도구는 body `messageId`도 채웁니다. Iris는 두 곳에 같은 값을 항상 보냅니다. |
| webhook 비밀키의 token(`NewHandler` 인자, `WithWebhookToken`, `IRIS_WEBHOOK_TOKEN`)과 `WithWebhookSecret` 두 이름, secret이 없을 때 token으로 가는 폴백 | 두 이름 중 하나만 씁니다. 남길 이름은 major에서 정합니다. |
| `NewHandler`·`NewDurableHandler`가 `WithContext`·`WithWebhookToken`·`WithWebhookLogger`를 조용히 무시하는 동작 | 직접 경로에서는 인자로만 넘기고, 옵션으로 넘기려면 `iris.NewWebhookHandler`(SDK 경로)를 씁니다. |
| `WebhookMention`의 `user_id` 키와 숫자 `userId` 입력 | Iris는 문자열 `userId`만 보냅니다. 삭제 전 조건은 ChatBotGo webhook inbox 잔존 payload에 두 입력이 0건인 것입니다. |

`webhook.Handler.Close()`는 폐기 대상이 아닙니다.

---

## 환경 변수 (Environment Variables)

| 환경 변수 | 설명 |
|------|------|
| `IRIS_BASE_URL` | Iris 백엔드 서버 Base URL |
| `IRIS_BOT_TOKEN` | 봇 호출 API 인증용 Bearer 토큰 |
| `IRIS_WEBHOOK_TOKEN` | 웹훅 유효성 검증용 인바운드 인증 토큰 |
| `IRIS_TRANSPORT` | 메시지 전송용 프로토콜 (`h3` [기본값], `http1` 지원) |

* 코드 상에서 옵션 함수(`WithBaseURL` 등)로 주입된 값이 환경 변수로 로드된 값보다 항상 우선하여 적용됩니다.

---

## 라이브러리 구조 (Directory Layout)

```text
iris/              # SDK Facade - 외부 노출용 엔트리 포인트 (NewClient, NewWebhookHandler 등)
webhook/           # WebhookHandler, 메시지 스키마 정의 및 순차 스케줄러 큐
webhooksign/       # Webhook signature v3 요청 header 생성 helper
valkeydedup/       # Valkey 기반 메시지 중복 제거 public wrapper
internal/client/   # transport/signing/SSE/multipart/rebind/query/common 내부 구현
internal/dedup/    # Valkey 기반 메시지 중복 제거 구현체
```

---

## 라이선스 (License)

Apache License 2.0 — [LICENSE](LICENSE)
