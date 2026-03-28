# iris-client-go

Iris(KakaoTalk 메시지 브릿지)용 Go 클라이언트 라이브러리.

## 설치

```bash
go get github.com/park285/iris-client-go@latest
```

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
