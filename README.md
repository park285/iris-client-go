# iris-client-go

Iris(KakaoTalk 메시지 브릿지)용 Go 클라이언트 라이브러리.

## 설치

```bash
go get github.com/park285/iris-client-go@latest
```

`v0.11.0` 업그레이드 시 breaking change가 있으므로
[`MIGRATION-v0.11.0.md`](./MIGRATION-v0.11.0.md)를 먼저 확인하세요.

## Quick Start

### 메시지 발송

```go
import "github.com/park285/iris-client-go/iris"

c, err := iris.NewClient()

// 텍스트
err = c.SendMessage(ctx, "room-id", "Hello",
    iris.WithThreadID("12345"),
)

// 이미지
err = c.SendImage(ctx, "room-id", base64Img)

// 마크다운 (텍스트 공유 카드)
resp, err := c.SendMarkdown(ctx, "room-id", "**bold** text")
status, err := c.GetReplyStatus(ctx, resp.RequestID)
```

### 웹훅 수신

```go
handler, err := iris.NewWebhookHandler(myMessageHandler,
    iris.WithValkeyDedup(valkeyClient),
)
defer handler.Close()

http.Handle("/webhook/iris", handler)
```

### 관리 API

```go
cfg, err := c.GetConfig(ctx)
health, err := c.GetBridgeHealth(ctx)
rooms, err := c.GetRooms(ctx)
members, err := c.GetMembers(ctx, chatID)
```

### SSE 이벤트 스트림

```go
events, err := c.EventStream(ctx, 0)
for ev := range events {
    fmt.Println(ev.Event, ev.Data)
}
```

### 조회 API

```go
// 채팅방 요약
summary, err := c.QueryRoomSummary(ctx, chatID)

// 멤버 통계
stats, err := c.QueryMemberStats(ctx, iris.QueryMemberStatsRequest{
    ChatID: chatID,
    Limit:  20,
})

// 최근 스레드 목록
threads, err := c.QueryRecentThreads(ctx, chatID)

// 최근 메시지 목록
msgs, err := c.QueryRecentMessages(ctx, iris.QueryRecentMessagesRequest{
    ChatID: chatID,
    Limit:  50,
})
```

## 클라이언트 설정

```go
c, err := iris.NewClient(
    iris.WithBaseURL("http://iris-host:3000"),  // 또는 IRIS_BASE_URL 환경변수
    iris.WithBotToken("my-token"),              // 또는 IRIS_BOT_TOKEN 환경변수
    iris.WithTimeout(5 * time.Second),
    iris.WithHMACSecret("shared-secret"),
    iris.WithLogger(slog.Default()),
    iris.WithReplyRetry(3),                     // 발송 재시도 횟수
    iris.WithTransport("h2c"),                  // 또는 IRIS_TRANSPORT 환경변수
)
```

### 라우트별 비밀키 분리

```go
c, err := iris.NewClient(
    iris.WithBaseURL("http://localhost:3000"),
    iris.WithBotToken("shared-token"),               // 공유 폴백 (하위 호환)
    iris.WithInboundSecret("config-signing-secret"),  // /config 전용
    iris.WithBotControlToken("bot-control-token"),    // /reply, /rooms 등
)
```

`WithHMACSecret`은 모든 라우트에 동일한 비밀키를 사용합니다.
라우트별로 분리하려면 `WithInboundSecret`(설정 조회)과 `WithBotControlToken`(봇 제어)을 사용하세요.

### 웹훅 핸들러 설정

```go
handler, err := iris.NewWebhookHandler(msgHandler,
    iris.WithWebhookToken("webhook-secret"),    // 또는 IRIS_WEBHOOK_TOKEN 환경변수
    iris.WithValkeyDedup(valkeyClient),         // Valkey 기반 중복 제거
    iris.WithDedupTTL(60 * time.Second),
    iris.WithWorkerCount(32),                   // key-ordering worker 수
    iris.WithQueueSize(2000),
    iris.WithHandlerTimeout(30 * time.Second),
    iris.WithMaxBodyBytes(1 << 20),             // 1MB
    iris.WithMetrics(myPrometheusAdapter),
    iris.WithWebhookLogger(slog.Default()),
)
```

### 발송 옵션

```go
err = c.SendMessage(ctx, "room-id", "Hello",
    iris.WithThreadID("12345"),
    iris.WithThreadScope(2),
)
```

## 환경변수

| 변수 | 용도 |
|------|------|
| `IRIS_BASE_URL` | Iris 서버 URL |
| `IRIS_BOT_TOKEN` | 봇 인증 토큰 |
| `IRIS_WEBHOOK_TOKEN` | 웹훅 인증 토큰 |
| `IRIS_TRANSPORT` | 전송 프로토콜 (`h2c`, `http2`, `http1`) |

옵션(`WithBaseURL` 등)이 환경변수보다 우선합니다.

## 패키지 구조

```
iris/      SDK facade -- NewClient, NewWebhookHandler, 모든 타입/옵션 re-export
client/    H2CClient, 타입, SendOption, transport 선택
webhook/   WebhookHandler, 메시지 타입, key-ordering scheduler
dedup/     ValkeyDeduplicator
```

## 기본값

### H2CClient

| 항목 | 값 |
|------|---|
| Client Timeout | 10s |
| Dial Timeout | 3s |
| Idle Connection Timeout | 90s |
| Max Idle Connections | 10 |

### WebhookHandler

| 항목 | 값 |
|------|---|
| Worker Count | 16 |
| Queue Size | 1000 |
| Handler Timeout | 30s |
| Dedup TTL | 60s |
| Max Body Bytes | 1MB |

## 라이선스

MIT
