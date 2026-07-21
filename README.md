# iris-client-go

Iris (카카오톡 메시지 브릿지)용 Go 클라이언트 라이브러리 SDK입니다.

## 설치 (Installation)

```bash
go get github.com/park285/iris-client-go@latest
```

`v0.11.0` 버전으로 마이그레이션할 경우 하위 호환성이 깨지는 변경 사항(Breaking Changes)이 존재하므로, 업그레이드 전에 [`MIGRATION-v0.11.0.md`](./MIGRATION-v0.11.0.md)를 반드시 확인하시기 바랍니다.

## 빠른 시작 (Quick Start)

### 1. 메시지 발송 (Sending Messages)

```go
import "github.com/park285/iris-client-go/iris"

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

### 2. 웹훅 수신 (Receiving Webhooks)

```go
handler, err := iris.NewWebhookHandler(myMessageHandler,
    valkeydedup.Option(valkeyClient),
)
if err != nil {
    log.Fatalf("웹훅 핸들러 생성 실패: %v", err)
}
defer handler.Close()

http.Handle("/webhook/iris", handler)
```

`WithQueueSize`는 ordering scheduler가 소유하는 전체 pending 상한입니다. 내부 실행 pool은 별도 buffered queue를 만들지 않습니다. 종료 budget이 있는 서비스는 `handler.CloseContext(ctx)`를 사용하면 grace 만료 후 queued callback을 건너뛰고 in-flight handler context를 취소할 수 있습니다. 기존 `Close()`는 무제한 context를 사용하는 호환 wrapper입니다.

HTTP `200 OK`가 메모리 admission이 아니라 durable commit을 의미해야 하는 소비자는 `webhook.MessageAdmitter`를 구현하고 `WithDurableAdmission`을 사용합니다. 이 모드에서는 scheduler와 deduplicator를 건너뛰므로 admitter의 저장소 unique key가 idempotency를 소유합니다.

```go
handler, err := iris.NewWebhookHandler(inboxRuntime,
    webhook.WithDurableAdmission(inboxRuntime),
    webhook.WithAdmitTimeout(200 * time.Millisecond),
    webhook.WithWebhookToken("webhook-secret"),
)
```

웹훅 송신 테스트나 smoke 도구에서는 `X-Iris-Message-Id`를 먼저 설정한 뒤 공개 helper로
signature v2 header를 생성할 수 있습니다.

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

`WithAdmitTimeout`은 durable commit에 선택적으로 deadline을 적용합니다. 기본값은 timeout 없음으로 기존 동작을 유지하며, 설정된 deadline이 끝나면 다른 admission 오류와 동일하게 HTTP `503 Service Unavailable`을 반환하므로 발신자가 재시도할 수 있습니다.

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

// HTTP/3 TLS 인증서 핫 리로드
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
    ChatID: chatID,
    Limit:  50,
})

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

---

## 클라이언트 설정 옵션 (Configuration)

```go
c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"), // 또는 IRIS_BASE_URL 환경변수 사용
    iris.WithBotToken("my-token"),              // 또는 IRIS_BOT_TOKEN 환경변수 사용
    iris.WithTimeout(5 * time.Second),
    iris.WithHMACSecret("shared-secret"),
    iris.WithLogger(slog.Default()),
    iris.WithReplyRetry(3),                     // 메시지 전송 실패 시 재시도 횟수
    iris.WithTransport("h3"),                   // 또는 IRIS_TRANSPORT 환경변수 사용
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
)
```

### 1. HTTP/3 전송 설정

Iris API의 기본 전송 프로토콜은 HTTP/3(QUIC)입니다. `IRIS_TRANSPORT` 환경 변수가 누락된 경우 기본적으로 `h3` 전송이 적용되며 이 경우 `https://` 스키마가 포함된 Base URL을 설정해야 합니다.

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

`IRIS_TRANSPORT=h3` 옵션은 `https://` 보안 연결에서만 활성화됩니다. `http3`, `http/3`, `quic` 문자열 역시 `h3`와 동일하게 인식합니다. 레거시 또는 로컬 테스트 목적으로 `http://` 일반 연결을 사용할 경우 `h2c` 전송을 명시해야 하며 유효하지 않은 프로토콜 형식 지정 시 에러가 반환됩니다.

운영 환경에서 H3 egress 대상을 Base URL host로 제한하려면 DNS allowset을 TTL마다 갱신하는 `WithH3DialGuardForBaseURL`을 사용할 수 있습니다. 만료 시 다른 dial은 stale allowset으로 즉시 판정하고 하나의 background refresh만 수행합니다. 초기 DNS 해석 실패는 기본적으로 오류를 반환하며 `WithH3DialGuardLenientInit`을 지정하면 deny-all 상태로 기동한 뒤 TTL 만료 시 자가회복합니다. 엉뚱한 host를 allowlist하지 않도록 `WithH3DialGuardForBaseURL`과 `WithBaseURL`에는 반드시 동일한 Base URL을 전달해야 합니다.

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

### 2. 엔드포인트별 비밀키(Token) 분리 권장

보안 강화를 위해 모든 API 엔드포인트에 단일 토큰(`WithHMACSecret`)을 적용하는 대신, API 역할별로 전용 비밀 토큰을 지정할 수 있습니다.

```go
c, err := iris.NewClient(
    iris.WithBaseURL("http://localhost:3000"),
    iris.WithBotToken("shared-token"),               // 공유 폴백 키 (하위 호환 유지)
    iris.WithInboundSecret("config-signing-secret"),  // /config 전용
    iris.WithBotControlToken("bot-control-token"),    // /reply, /rooms 등 제어 API 전용
)
```

### 3. 웹훅 핸들러 설정 (Webhook Handler Configuration)

```go
import (
    "github.com/park285/iris-client-go/iris"
    "github.com/park285/iris-client-go/valkeydedup"
    "github.com/park285/iris-client-go/webhook"
)

handler, err := iris.NewWebhookHandler(msgHandler,
    webhook.WithWebhookToken("webhook-secret"),  // 또는 IRIS_WEBHOOK_TOKEN 환경변수 사용
    valkeydedup.Option(valkeyClient),            // Valkey 기반의 분산 중복 제거 필터
    webhook.WithDedupTTL(60 * time.Second),
    webhook.WithWorkerCount(32),                 // Key-ordering 동시성 워커 개수
    webhook.WithQueueSize(2000),
    webhook.WithHandlerTimeout(30 * time.Second),
    webhook.WithMaxBodyBytes(1 << 20),           // 최대 요청 크기 (1MB)
    webhook.WithMetrics(myPrometheusAdapter),
    webhook.WithWebhookLogger(slog.Default()),
)
```

* 웹훅 메시지 스키마(`webhook.Message`/`webhook.MessageJSON`)와 핸들러 옵션(`webhook.WithXxx`)은 `webhook` 패키지에서 직접 import합니다. SDK 진입점인 `iris.NewWebhookHandler`(환경변수 해석·검증 포함)는 `iris` 패키지에 유지되며 Valkey 기반 중복 제거 필터는 `github.com/park285/iris-client-go/valkeydedup` 서브패키지(`valkeydedup.Option`/`valkeydedup.New`)로 분리되어 valkey-go를 쓰지 않는 소비자의 바이너리에 링크되지 않습니다.
* **메시지 순서 보장:** in-memory 모드에서는 기본적으로 동일한 채팅방 또는 동일 스레드 내의 메시지가 순차 처리됩니다. 자체적인 durable scheduler나 분산 큐가 순서를 소유하는 경우 `webhook.WithDurableAdmission`을 사용하거나 `webhook.WithOrderingMode(webhook.OrderingModeNone)`로 in-memory ordering을 끌 수 있습니다.

---

## 환경 변수 (Environment Variables)

| 환경 변수 | 설명 |
|------|------|
| `IRIS_BASE_URL` | Iris 백엔드 서버 Base URL |
| `IRIS_BOT_TOKEN` | 봇 호출 API 인증용 Bearer 토큰 |
| `IRIS_WEBHOOK_TOKEN` | 웹훅 유효성 검증용 인바운드 인증 토큰 |
| `IRIS_TRANSPORT` | 메시지 전송용 프로토콜 (`h3` [기본값], `h2c`, `http2`, `http1` 지원) |

* 코드 상에서 옵션 함수(`WithBaseURL` 등)로 주입된 값이 환경 변수로 로드된 값보다 항상 우선하여 적용됩니다.

---

## 라이브러리 구조 (Directory Layout)

```text
iris/              # SDK Facade - 외부 노출용 엔트리 포인트 (NewClient, NewWebhookHandler 등)
webhook/           # WebhookHandler, 메시지 스키마 정의 및 순차 스케줄러 큐
webhooksign/       # Webhook signature v2 요청 header 생성 helper
valkeydedup/       # Valkey 기반 메시지 중복 제거 public wrapper
internal/client/   # transport/signing/SSE/multipart/rebind/query/common 내부 구현
internal/dedup/    # Valkey 기반 메시지 중복 제거 구현체
```

---

## 라이선스 (License)

Apache License 2.0 — [LICENSE](LICENSE)
