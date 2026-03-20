# iris-client-go

Iris(KakaoTalk 메시지 브릿지)용 Go 클라이언트 라이브러리.

메시지 발신, 웹훅 수신, Iris 서버 관리를 H2C 기반으로 통합 제공합니다.

## 설치

```bash
go get park285/iris-client-go@latest
```

## 패키지 구조

```
iris-client-go/
  (root)     타입, 상수, SendOption, ResolveThreadID, DedupKey
  client/    H2CClient (Sender + AdminClient), 3단계 ping, transport 선택
  webhook/   net/http WebhookHandler, stripe 워커풀, Metrics/Deduplicator 인터페이스
  dedup/     ValkeyDeduplicator (webhook.Deduplicator 구현체)
```

### Import 방향 규칙

```
root      <-- client/     (루트 타입 참조)
          <-- webhook/    (루트 타입 참조)
          <-- dedup/      (webhook.Deduplicator만 참조)

client/ 와 webhook/ 는 서로 참조 금지
```

## 사용법

### 메시지 발송

```go
import (
    iris "park285/iris-client-go"
    "park285/iris-client-go/client"
)

c := client.NewH2CClient("http://iris-host:3000", "bot-token",
    client.WithTransport("h2c"),
    client.WithTimeout(10*time.Second),
)

// 텍스트 메시지
err := c.SendMessage(ctx, "room-id", "Hello",
    iris.WithThreadID("12345"),
    iris.WithThreadScope(2),
)

// 이미지
err = c.SendImage(ctx, "room-id", base64EncodedImage)
```

### 관리 API

```go
// 3단계 probe: /ready -> /health -> OPTIONS /reply
alive := c.Ping(ctx)

cfg, err := c.GetConfig(ctx)

plaintext, err := c.Decrypt(ctx, base64Ciphertext)
```

### 웹훅 수신 (net/http)

```go
import "park285/iris-client-go/webhook"

handler := webhook.NewHandler(ctx, "iris-webhook-token", myMessageHandler, logger,
    webhook.WithWorkerCount(16),
    webhook.WithQueueSize(1000),
    webhook.WithMetrics(myPrometheusAdapter),
    webhook.WithDeduplicator(myValkeyDedup),
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
import "park285/iris-client-go/dedup"

d := dedup.NewValkeyDeduplicator(valkeyClient)

handler := webhook.NewHandler(ctx, token, msgHandler, logger,
    webhook.WithDeduplicator(d),
    webhook.WithDedupTTL(60*time.Second),
)
```

## 핵심 인터페이스

### client.Sender -- 메시지 발신 전용

```go
type Sender interface {
    SendMessage(ctx context.Context, room, message string, opts ...iris.SendOption) error
    SendImage(ctx context.Context, room, imageBase64 string) error
}
```

### client.AdminClient -- 관리/유틸 API

```go
type AdminClient interface {
    Ping(ctx context.Context) bool
    GetConfig(ctx context.Context) (*iris.Config, error)
    Decrypt(ctx context.Context, data string) (string, error)
}
```

`H2CClient`는 `Sender` + `AdminClient` 모두 구현. 소비자는 필요한 인터페이스만 의존:
- settlement-go: `client.Sender`만
- hololive-kakao-bot-go: `client.Sender` + `client.AdminClient`

### webhook.Metrics -- 메트릭 관측점

```go
type Metrics interface {
    ObserveRequest()
    ObserveUnauthorized()
    ObserveBadRequest()
    ObserveDuplicate()
    ObserveEnqueueFailure()
    ObserveAccepted()
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

1. `client.WithTransport("h2c")` -- 명시적 옵션
2. `IRIS_TRANSPORT` 환경변수
3. 기본값: `http://` URL이면 H2C, `https://`이면 HTTP/1.1

지원 값: `h2c`, `http2`, `http1`, `http`, `http/1.1`

## H2CClient 기본 타임아웃

| 항목 | 기본값 |
|------|--------|
| Client Timeout | 10s |
| Dial Timeout | 3s |
| TLS Handshake Timeout | 5s |
| Response Header Timeout | 5s |
| Idle Connection Timeout | 90s |
| Max Idle Connections | 10 |
| H2C Read Idle Timeout | 30s |
| H2C Ping Timeout | 15s |
| H2C Write Byte Timeout | 10s |

## WebhookHandler 기본값

| 항목 | 기본값 |
|------|--------|
| Worker Count | 16 (stripe pool) |
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

루트, `client/`, `webhook/` 패키지는 stdlib + `x/net/http2`만 의존합니다.

## 라이선스

MIT
