# Migration Guide: v0.10.x -> v0.11.0

`v0.11.0`은 `/home/kapu/gemini/Iris` 서버 계약에 SDK surface를 맞춘 정리 릴리즈입니다.
이번 버전에는 의도된 breaking change가 포함됩니다.

## 요약

- raw `Query()` API 제거
- raw `Decrypt()` API 제거
- `iris/preset` 패키지 제거
- webhook payload 타입에서 `senderRole` 제거
- typed query API를 primary surface로 승격
- route-separated HMAC secret 지원 추가
- SSE event envelope 파서가 `event:` 필드와 comment frame을 처리하도록 정렬

## Breaking Changes

### 1. `Query()` 제거

다음 API는 더 이상 제공되지 않습니다.

```go
result, err := c.Query(ctx, client.QueryRequest{...})
```

대신 typed query API를 사용해야 합니다.

```go
summary, err := c.QueryRoomSummary(ctx, chatID)

stats, err := c.QueryMemberStats(ctx, iris.QueryMemberStatsRequest{
    ChatID: chatID,
    Limit:  20,
})

threads, err := c.QueryRecentThreads(ctx, chatID)

messages, err := c.QueryRecentMessages(ctx, iris.QueryRecentMessagesRequest{
    ChatID: chatID,
    Limit:  50,
})
```

### 2. `Decrypt()` 제거

다음 API는 더 이상 제공되지 않습니다.

```go
plain, err := c.Decrypt(ctx, ciphertext)
```

정본 서버인 Iris는 `/decrypt`를 public SDK 계약으로 더 이상 제공하지 않습니다.
복호화가 필요하면 서버가 제공하는 typed API나 room/message 조회 결과를 사용해야 합니다.

### 3. `iris/preset` 제거

다음 패키지는 제거됐습니다.

```go
import "github.com/park285/iris-client-go/iris/preset"
```

기존:

```go
opts := preset.ClientOptions(preset.ClientConfig{
    Timeout: 5 * time.Second,
})
client := iris.NewH2CClient(baseURL, token, opts...)
```

변경:

```go
client := iris.NewH2CClient(
    baseURL,
    token,
    iris.WithTimeout(5*time.Second),
)
```

기존:

```go
handler := iris.NewHandler(
    ctx,
    token,
    msgHandler,
    logger,
    preset.WebhookOptions(preset.WebhookConfig{
        WorkerCount: 32,
        QueueSize:   2000,
    })...,
)
```

변경:

```go
handler := iris.NewHandler(
    ctx,
    token,
    msgHandler,
    logger,
    iris.WithWorkerCount(32),
    iris.WithQueueSize(2000),
)
```

### 4. webhook `senderRole` 제거

Iris 서버의 실제 webhook payload는 `senderRole`을 직렬화하지 않습니다.
따라서 SDK의 다음 필드는 제거됐습니다.

- `webhook.WebhookRequest.SenderRole`
- `webhook.MessageJSON.SenderRole`
- `iris.WebhookRequest.SenderRole`
- `iris.MessageJSON.SenderRole`

unknown field는 계속 무시되므로, 외부 서버가 임의로 `senderRole`을 넣어도 decode는 깨지지 않습니다.
다만 SDK 타입에는 더 이상 노출되지 않습니다.

## 새 기본 사용 방식

### Typed query 사용

```go
summary, err := c.QueryRoomSummary(ctx, 12345)
stats, err := c.QueryMemberStats(ctx, iris.QueryMemberStatsRequest{
    ChatID: 12345,
    Limit:  20,
})
threads, err := c.QueryRecentThreads(ctx, 12345)
events, err := c.GetRoomEvents(ctx, 12345, 50, 0)
```

### Route-separated secret 사용

기존처럼 단일 shared secret도 계속 가능합니다.

```go
c := iris.NewH2CClient(
    baseURL,
    botToken,
    iris.WithHMACSecret("shared-secret"),
)
```

라우트별 비밀키를 분리하려면 아래처럼 설정합니다.

```go
c := iris.NewH2CClient(
    baseURL,
    botToken,
    iris.WithInboundSecret("config-secret"),
    iris.WithBotControlToken("bot-control-secret"),
)
```

- `WithInboundSecret`: `/config` 계열
- `WithBotControlToken`: `/reply`, `/rooms`, `/events`, `/query/*`
- 둘 다 없으면 `WithHMACSecret`
- `WithHMACSecret`도 없으면 `botToken`이 shared secret로 폴백

### SSE consumer 업데이트

이제 raw event는 `event` 필드를 함께 가집니다.

```go
events, err := c.EventStream(ctx, 0)
for ev := range events {
    fmt.Println(ev.ID, ev.Event, string(ev.Data))
}
```

SDK는 아래를 서버 계약대로 처리합니다.

- `id:`
- `event:`
- `data:`
- comment frame (`: connected`, `: keepalive`) 무시

## 치환표

| 이전 | 이후 |
|------|------|
| `Query()` | `QueryRoomSummary`, `QueryMemberStats`, `QueryRecentThreads`, `QueryRecentMessages` |
| `Decrypt()` | 제거, typed API 사용 |
| `preset.ClientOptions(...)` | `iris.With*` 옵션 직접 사용 |
| `preset.WebhookOptions(...)` | `iris.With*` webhook 옵션 직접 사용 |
| `senderRole` 필드 접근 | 제거 |
| shared secret only | `WithHMACSecret` 또는 route-separated secret |

## 업그레이드 체크리스트

- `Query(` 호출 제거
- `Decrypt(` 호출 제거
- `iris/preset` import 제거
- `senderRole` 필드 접근 제거
- SSE consumer가 `RawSSEEvent.Event`를 사용하도록 갱신
- 필요 시 `WithInboundSecret` / `WithBotControlToken`으로 secret 분리
- `go test ./...` 재실행

## 검증 권장 명령

```bash
go test ./...
```
