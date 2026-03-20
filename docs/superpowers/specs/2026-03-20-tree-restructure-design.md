# iris-client-go 트리 구조 재구조화

## 문제

루트 패키지(`package iris`)가 공유 타입 허브가 아닌 단일 소비자 전용 코드의 dumping ground.
실제 양쪽(client/webhook)에서 공유되는 코드는 `ResolveToken`(`strings.TrimSpace`) 하나뿐.

## 결정

루트 패키지의 모든 Go 코드를 제거하고, 각 하위 패키지가 자기 타입/상수/로직을 100% 소유하도록 변경.
외부 소비자가 아직 없음(v0.1.0 pre-release)을 사용자로부터 확인 완료 — breaking change 허용.

## 변경 후 구조

```
iris-client-go/
├── go.mod, README.md, CLAUDE.md
├── client/
│   ├── types.go         ← ReplyRequest, Config, DecryptRequest, DecryptResponse
│   ├── types_test.go    ← ReplyRequest JSON round-trip 테스트
│   ├── constants.go     ← PathReply, PathReady, PathHealth, PathConfig, PathDecrypt, HeaderBotToken
│   ├── options.go       ← SendOption, WithThreadID, WithThreadScope, ApplySendOptions, ValidateSendOptions
│   ├── options_test.go
│   ├── normalize.go     ← NormalizeReplyThreadID, NormalizeReplyThreadScope
│   ├── normalize_test.go
│   ├── h2c_client.go
│   ├── h2c_client_test.go
│   ├── sender.go        ← Sender interface (iris.SendOption → SendOption)
│   ├── admin.go         ← AdminClient interface (iris.Config → Config)
│   ├── transport.go
│   ├── transport_test.go
│   ├── ping.go
│   └── ping_test.go
├── webhook/
│   ├── types.go         ← WebhookRequest, Message, MessageJSON
│   ├── types_test.go    ← WebhookRequest JSON round-trip/unmarshal 테스트
│   ├── constants.go     ← PathWebhook, HeaderIrisToken, HeaderIrisMessageID, DefaultDedupTTL
│   ├── thread.go        ← ResolveThreadID, DedupKey
│   ├── thread_test.go
│   ├── handler.go
│   ├── handler_test.go
│   ├── dedup.go
│   └── metrics.go
└── dedup/
    ├── valkey.go
    └── valkey_test.go
```

## Import 방향

```
client/  ← stdlib + x/net/http2
webhook/ ← stdlib
dedup/   ← webhook.Deduplicator + valkey-go
```

루트 패키지 의존이 완전히 제거됨. 패키지 간 교차 의존 없음.

## 이동 상세

### 루트 → client/

| 원본 | 대상 | 요소 |
|------|------|------|
| types.go | client/types.go | ReplyRequest, Config, DecryptRequest, DecryptResponse |
| types_test.go | client/types_test.go | TestReplyRequestJSON, assertJSONRoundTrip, assertJSONUnmarshal, assertJSONEqual (제네릭 테스트 헬퍼) |
| constants.go | client/constants.go | PathReply, PathReady, PathHealth, PathConfig, PathDecrypt, HeaderBotToken |
| options.go | client/options.go | SendOption, sendOptions, WithThreadID, WithThreadScope, ApplySendOptions, ValidateSendOptions |
| options.go | client/normalize.go | NormalizeReplyThreadID, NormalizeReplyThreadScope |
| options_test.go | client/options_test.go | SendOption/Validate 관련 테스트 (기존 client/options_test.go에 병합) |
| options_test.go | client/normalize_test.go | Normalize 관련 테스트 |

### 루트 → webhook/

| 원본 | 대상 | 요소 |
|------|------|------|
| types.go | webhook/types.go | WebhookRequest, Message, MessageJSON |
| types_test.go | webhook/types_test.go | TestWebhookRequestJSON*, webhookMarshal*, legacyWebhookUnmarshal*, assertJSONRoundTrip, assertJSONUnmarshal, assertJSONEqual (제네릭 헬퍼 복제) |
| constants.go | webhook/constants.go | PathWebhook, HeaderIrisToken, HeaderIrisMessageID, DefaultDedupTTL (기존명 DefaultWebhookDedupTTL → webhook 패키지 내이므로 Webhook prefix 제거) |
| thread.go | webhook/thread.go | ResolveThreadID, DedupKey |
| thread_test.go | webhook/thread_test.go | ResolveThreadID, DedupKey 테스트 |

### ResolveToken 처리

`ResolveToken`은 `strings.TrimSpace()` wrapper. 함수를 완전히 제거하고 호출부에서 직접 inline:
- client/h2c_client.go: `iris.ResolveToken(c.botToken)` → `strings.TrimSpace(c.botToken)`
- webhook/handler.go: `iris.ResolveToken(token)` → `strings.TrimSpace(token)`

별도의 헬퍼 함수(exported/unexported)를 남기지 않음.

### ResolveToken 테스트

thread_test.go의 TestResolveToken은 `strings.TrimSpace` 동작을 검증하는 것이므로 삭제. stdlib 함수에 대한 테스트는 불필요.

### 공개 인터페이스 변경

루트 패키지 제거에 따라 하위 패키지의 공개 인터페이스 시그니처 변경:

| 파일 | 변경 전 | 변경 후 |
|------|---------|---------|
| client/sender.go | `SendMessage(ctx, room, message string, opts ...iris.SendOption) error` | `SendMessage(ctx, room, message string, opts ...SendOption) error` |
| client/admin.go | `GetConfig(ctx) (*iris.Config, error)` | `GetConfig(ctx) (*Config, error)` |
| client/h2c_client.go | `iris.XXX` 참조 전체 | 패키지 내부 참조 (prefix 제거) |
| webhook/handler.go | `iris.XXX` 참조 전체 | 패키지 내부 참조 (prefix 제거) |

### 삭제 대상 (루트)

- types.go, types_test.go
- constants.go
- options.go, options_test.go
- thread.go, thread_test.go

## 기존 코드 변경

- client/ 내 `iris.XXX` 참조 → 패키지 내부 참조로 변경 (prefix 제거)
- client/ 내 `import iris "park285/iris-client-go"` 제거
- webhook/ 내 `iris.XXX` 참조 → 패키지 내부 참조로 변경 (prefix 제거)
- webhook/ 내 `import iris "park285/iris-client-go"` 제거
- dedup/ 변경 없음 (webhook.Deduplicator만 의존)

## README.md 업데이트

현재 README가 `iris.WithThreadID`, `iris.Config` 등 루트 패키지 사용법을 문서화.
변경 후 import path 및 사용 예제를 `client.WithThreadID`, `client.Config` 등으로 수정.

## CLAUDE.md 업데이트

Architecture 섹션의 import direction, packages 설명을 변경된 구조에 맞게 수정.
루트 패키지 설명 제거, 각 하위 패키지 설명에 이동된 타입/상수 반영.

## 완료 검증 기준

1. `grep -r 'park285/iris-client-go"' --include='*.go'` → client/, webhook/ 내 루트 import 0건
2. `grep -r 'iris\.' --include='*.go' client/ webhook/` → `iris.XXX` 잔존 참조 0건
3. 루트 디렉토리에 `.go` 파일 0개 (go.mod, go.sum 제외)
4. `go test ./...` 전체 통과
5. `go vet ./...` 전체 통과
6. `go build ./...` 성공

## 제약조건

- 외부 소비자 없음 (사용자 확인 완료) → breaking change 허용
- import cycle 금지: client/ ↔ webhook/ 교차 불가
- 테스트 전체 통과 필수: `go test ./...` && `go vet ./...`
