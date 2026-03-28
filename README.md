# iris-client-go

Iris(KakaoTalk 메시지 브릿지)용 Go 클라이언트 라이브러리.

메시지 발신, 웹훅 수신, Iris 서버 관리를 H2C 기반으로 통합 제공합니다.

## 설치

```bash
go get github.com/park285/iris-client-go@latest
```

## 패키지 구조

```
iris-client-go/
  iris/      SDK facade -- NewClient, NewWebhookHandler, 모든 타입/옵션 re-export
  client/    H2CClient (Sender + AdminClient + RoomClient + EventStreamClient), 타입, 상수, SendOption, 3단계 ping, transport 선택
  webhook/   net/http WebhookHandler, 타입, 상수, ResolveThreadID, DedupKey, key-ordering scheduler
  dedup/     ValkeyDeduplicator (webhook.Deduplicator 구현체)
```

### Import 방향 규칙

```
client/  <- stdlib + x/net/http2
webhook/ <- stdlib
dedup/   <- webhook.Deduplicator + valkey-go
```

## 환경변수

| 변수 | 용도 | 필수 |
|------|------|------|
| `IRIS_BASE_URL` | Iris 서버 URL | NewClient 사용 시 |
| `IRIS_BOT_TOKEN` | 봇 인증 토큰 | NewClient 사용 시 |
| `IRIS_WEBHOOK_TOKEN` | 웹훅 인증 토큰 | NewWebhookHandler 사용 시 |

옵션(`WithBaseURL` 등)이 환경변수보다 우선합니다.

## 사용법

### 메시지 발송

```go
import "github.com/park285/iris-client-go/iris"

// IRIS_BASE_URL, IRIS_BOT_TOKEN 환경변수에서 자동 읽기
c, err := iris.NewClient()

// 또는 옵션 override
c, err := iris.NewClient(
    iris.WithBaseURL("http://iris-host:3000"),
    iris.WithTimeout(5 * time.Second),
)

// 텍스트 메시지
err = c.SendMessage(ctx, "room-id", "Hello",
    iris.WithThreadID("12345"),
    iris.WithThreadScope(2),
)

// 이미지
err = c.SendImage(ctx, "room-id", base64EncodedImage)

// 여러 장 이미지
err = c.SendMultipleImages(ctx, "room-id", []string{base64Image1, base64Image2})

// 마크다운 메시지 (텍스트 공유 카드)
resp, err := c.SendMarkdown(ctx, "room-id", "**bold** text")

// 발송 상태 조회
status, err := c.GetReplyStatus(ctx, resp.RequestID)
```

### 관리 API

```go
// Ping: 기본 3단계 probe (/ready -> /health -> OPTIONS /reply)
// 성공한 endpoint는 캐시하여 이후 호출에서 fallback 생략
alive := c.Ping(ctx)

cfg, err := c.GetConfig(ctx)

updateResp, err := c.UpdateConfig(ctx, "configName", iris.ConfigUpdateRequest{Value: "newValue"})

health, err := c.GetBridgeHealth(ctx)

queryResp, err := c.Query(ctx, iris.QueryRequest{SQL: "SELECT * FROM chat_logs LIMIT 10"})

plaintext, err := c.Decrypt(ctx, base64Ciphertext)
```

### 웹훅 수신 (net/http)

```go
import "github.com/park285/iris-client-go/iris"

// IRIS_WEBHOOK_TOKEN 환경변수에서 자동 읽기
handler, err := iris.NewWebhookHandler(myMessageHandler)

// 또는 옵션 override
handler, err := iris.NewWebhookHandler(myMessageHandler,
    iris.WithValkeyDedup(valkeyClient),
    iris.WithWorkerCount(32),
    iris.WithMetrics(myPrometheusAdapter),
)
defer handler.Close()

http.Handle("/webhook/iris", handler)
```

### 웹훅 수신 (gin)

```go
r := gin.Default()
r.POST("/webhook/iris", gin.WrapH(handler))
```

### Valkey 기반 중복 제거

```go
import "github.com/park285/iris-client-go/iris"

handler, err := iris.NewWebhookHandler(msgHandler,
    iris.WithValkeyDedup(valkeyClient),
    iris.WithDedupTTL(60 * time.Second),
)
```

### 방/이벤트 API

```go
// 방 목록 조회
rooms, err := c.GetRooms(ctx)

// 멤버 조회
members, err := c.GetMembers(ctx, chatID)

// 실시간 이벤트 스트림 (SSE)
events, err := c.EventStream(ctx, 0)
for ev := range events {
    fmt.Println(ev.ID, string(ev.Data))
}
```

## 핵심 인터페이스

### client.Sender -- 메시지 발신 전용

```go
type Sender interface {
    SendMessage(ctx context.Context, room, message string, opts ...SendOption) error
    SendImage(ctx context.Context, room, imageBase64 string, opts ...SendOption) error
    SendMultipleImages(ctx context.Context, room string, imageBase64s []string, opts ...SendOption) error
    SendMarkdown(ctx context.Context, room, markdown string, opts ...SendOption) (*ReplyAcceptedResponse, error)
    GetReplyStatus(ctx context.Context, requestID string) (*ReplyStatusSnapshot, error)
}
```

### client.AdminClient -- 관리/유틸 API

```go
type AdminClient interface {
    Ping(ctx context.Context) bool
    GetConfig(ctx context.Context) (*ConfigResponse, error)
    UpdateConfig(ctx context.Context, name string, req ConfigUpdateRequest) (*ConfigUpdateResponse, error)
    Decrypt(ctx context.Context, data string) (string, error)
    GetBridgeHealth(ctx context.Context) (*BridgeHealthResult, error)
    Query(ctx context.Context, req QueryRequest) (*QueryResponse, error)
}
```

### client.RoomClient -- 방/멤버/통계 API

```go
type RoomClient interface {
    GetRooms(ctx context.Context) (*RoomListResponse, error)
    GetMembers(ctx context.Context, chatID int64) (*MemberListResponse, error)
    GetRoomInfo(ctx context.Context, chatID int64) (*RoomInfoResponse, error)
    GetRoomStats(ctx context.Context, chatID int64, opts RoomStatsOptions) (*StatsResponse, error)
    GetMemberActivity(ctx context.Context, chatID, userID int64, period string) (*MemberActivityResponse, error)
}
```

### client.EventStreamClient -- SSE 이벤트 스트림 API

```go
type EventStreamClient interface {
    EventStream(ctx context.Context, lastEventID int64) (<-chan RawSSEEvent, error)
}
```

`H2CClient`는 `Sender`, `AdminClient`, `RoomClient`, `EventStreamClient`를 모두 구현. 봇 코드가 공통으로 의존할 상위 인터페이스도 제공:

```go
// 봇 코드가 공통으로 의존할 상위 인터페이스
type Client interface {
    Sender
    AdminClient
}

// 모든 Iris 기능을 포함하는 확장 인터페이스
type FullClient interface {
    Sender
    AdminClient
    RoomClient
    EventStreamClient
}
```

소비자는 필요한 범위만 의존:
- 일반 봇/자동응답기: `iris.Client`
- 운영 도구/관리 콘솔: `iris.FullClient`
- 발신 전용 워커: `client.Sender`

### webhook.Metrics -- 메트릭 관측점

```go
type Metrics interface {
    ObserveRequest()
    ObserveUnauthorized()
    ObserveBadRequest()
    ObserveDuplicate()
    ObserveEnqueueFailure()
    ObserveAccepted()
    ObserveDecodeLatency(d time.Duration)
    ObserveDedupLatency(d time.Duration)
    ObserveEnqueueWait(d time.Duration)
    ObserveQueueDepth(depth int)
    ObserveHandlerDuration(d time.Duration)
}
```

Prometheus 등 원하는 구현체를 주입. 기본값: `NoopMetrics`.

### webhook.Deduplicator -- 중복 메시지 검사

```go
type Deduplicator interface {
    IsDuplicate(ctx context.Context, key string, ttl time.Duration) (bool, error)
}
```

기본값: `NoopDeduplicator` (중복 검사 비활성화).

## Transport 선택

우선순위:

1. `client.WithHTTPClient(c)` -- 완전 커스텀 HTTP 클라이언트
2. `client.WithRoundTripper(rt)` -- 커스텀 transport (otelhttp, circuit breaker 등)
3. `client.WithTransport("h2c")` -- 명시적 프로토콜
4. `IRIS_TRANSPORT` 환경변수
5. 기본값: `http://` URL이면 H2C, `https://`이면 HTTP/1.1

지원 프로토콜 값: `h2c`, `http2`, `http1`, `http`, `http/1.1`

## H2CClient 기본 타임아웃

| 항목 | 기본값 |
|------|--------|
| Client Timeout | 10s |
| Dial Timeout | 3s |
| TLS Handshake Timeout | 5s |
| Response Header Timeout | 5s |
| Idle Connection Timeout | 90s |
| Max Idle Connections | 10 |
| Ping Probe Timeout | 5s |
| H2C Read Idle Timeout | 30s |
| H2C Ping Timeout | 15s |
| H2C Write Byte Timeout | 10s |

## WebhookHandler 기본값

| 항목 | 기본값 |
|------|--------|
| Worker Count | 16 (key-ordering scheduler) |
| Queue Size | 1000 |
| Enqueue Timeout | 50ms |
| Handler Timeout | 30s |
| Dedup TTL | 60s |
| Dedup Timeout | 200ms |
| Max Body Bytes | 1MB |

## 외부 의존성

| 패키지 | 사용 위치 | 용도 |
|--------|----------|------|
| `golang.org/x/net/http2` | `client/` | H2C transport |
| `github.com/valkey-io/valkey-go` | `dedup/` only | ValkeyDeduplicator |

`client/`는 stdlib + `x/net/http2`, `webhook/`는 stdlib만 의존합니다.

## 라이선스

MIT
