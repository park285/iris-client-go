# Tree Restructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 루트 패키지의 모든 Go 코드를 client/, webhook/ 하위 패키지로 이동하여 각 패키지가 자기완결적으로 동작하게 한다.

**Architecture:** 루트의 타입/상수/헬퍼를 소비자 패키지로 이동. 양쪽에서 공유되던 `ResolveToken`은 `strings.TrimSpace()` 직접 호출로 대체. 패키지 간 교차 의존 없이 stdlib만 의존.

**Tech Stack:** Go 1.25, stdlib, golang.org/x/net/http2, github.com/valkey-io/valkey-go

**Spec:** `docs/superpowers/specs/2026-03-20-tree-restructure-design.md`

---

### Task 1: client/ 패키지에 루트 코드 이동

**Files:**
- Create: `client/types.go`
- Create: `client/constants.go`
- Create: `client/normalize.go`
- Create: `client/normalize_test.go`
- Create: `client/types_test.go`
- Modify: `client/options.go` (기존 ClientOption 파일, SendOption 코드 병합)
- Modify: `client/options_test.go` (기존 테스트에 SendOption 테스트 병합)

- [ ] **Step 1: client/types.go 생성**

`ReplyRequest`, `Config`, `DecryptRequest`, `DecryptResponse`를 루트 types.go에서 복사하여 `package client`로 작성.

```go
package client

type ReplyRequest struct {
	Type        string  `json:"type"`
	Room        string  `json:"room"`
	Data        string  `json:"data"`
	ThreadID    *string `json:"threadId,omitempty"`
	ThreadScope *int    `json:"threadScope,omitempty"`
}

type Config struct {
	BotName         string `json:"bot_name"`
	BotHTTPPort     int    `json:"bot_http_port"`
	DBPollingRate   int    `json:"db_polling_rate"`
	MessageSendRate int    `json:"message_send_rate"`
	BotID           int64  `json:"bot_id"`
}

type DecryptRequest struct {
	B64Ciphertext string `json:"b64_ciphertext"`
	UserID        *int64 `json:"user_id,omitempty"`
	Enc           int    `json:"enc"`
}

type DecryptResponse struct {
	PlainText string `json:"plain_text"`
}
```

- [ ] **Step 2: client/constants.go 생성**

루트 constants.go에서 client 전용 상수를 복사.

```go
package client

const (
	PathReply   = "/reply"
	PathReady   = "/ready"
	PathHealth  = "/health"
	PathConfig  = "/config"
	PathDecrypt = "/decrypt"
)

const HeaderBotToken = "X-Bot-Token" //nolint:gosec // HTTP header name, not a credential
```

- [ ] **Step 3: client/options.go에 SendOption 코드 병합**

기존 `client/options.go`의 맨 위에 루트 `options.go`의 `SendOption` 관련 코드를 추가. 기존 `ClientOption` 코드는 그대로 유지.

파일 상단에 추가할 코드:

```go
// SendOption 관련 (루트에서 이동)

type SendOption func(*sendOptions)

type sendOptions struct {
	ThreadID    *string
	ThreadScope *int
}

func WithThreadID(id string) SendOption {
	return func(o *sendOptions) {
		o.ThreadID = &id
	}
}

func WithThreadScope(scope int) SendOption {
	return func(o *sendOptions) {
		o.ThreadScope = &scope
	}
}

func applySendOptions(opts []SendOption) sendOptions {
	var result sendOptions
	for _, opt := range opts {
		opt(&result)
	}
	return result
}

func validateSendOptions(o sendOptions) error {
	if o.ThreadID != nil {
		for _, r := range *o.ThreadID {
			if !unicode.IsDigit(r) {
				return fmt.Errorf("iris: threadId must be numeric, got %q", *o.ThreadID)
			}
		}
	}
	if o.ThreadScope != nil && *o.ThreadScope <= 0 {
		return fmt.Errorf("iris: threadScope must be positive, got %d", *o.ThreadScope)
	}
	if o.ThreadScope != nil && *o.ThreadScope >= 2 && o.ThreadID == nil {
		return errors.New("iris: threadScope >= 2 requires threadId")
	}
	return nil
}
```

**주의:** `ApplySendOptions`/`ValidateSendOptions`는 패키지 내부에서만 사용되므로 unexported (`applySendOptions`/`validateSendOptions`)로 변경.

import에 `"errors"`, `"fmt"`, `"unicode"` 추가.

- [ ] **Step 4: client/normalize.go 생성**

```go
package client

import (
	"strings"
	"unicode"
)

func normalizeReplyThreadID(threadID *string) *string {
	if threadID == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*threadID)
	if trimmed == "" {
		return nil
	}
	for _, r := range trimmed {
		if !unicode.IsDigit(r) {
			return nil
		}
	}
	return &trimmed
}

func normalizeReplyThreadScope(scope *int) *int {
	if scope == nil || *scope <= 0 {
		return nil
	}
	value := *scope
	return &value
}
```

**주의:** 이 함수들은 패키지 내부에서만 사용되므로 unexported로 변경.

- [ ] **Step 5: client/types_test.go 생성**

루트 `types_test.go`에서 `ReplyRequest` 관련 테스트 + 제네릭 헬퍼를 복사.

```go
package client

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestReplyRequestJSON(t *testing.T) {
	threadID := "12345"
	threadScope := 2

	tests := []struct {
		name      string
		input     ReplyRequest
		wantJSON  string
		wantRound ReplyRequest
	}{
		{
			name:     "omit empty optional fields",
			input:    ReplyRequest{Type: "text", Room: "room-a", Data: "hello"},
			wantJSON: `{"type":"text","room":"room-a","data":"hello"}`,
			wantRound: ReplyRequest{Type: "text", Room: "room-a", Data: "hello"},
		},
		{
			name:     "include optional thread fields",
			input:    ReplyRequest{Type: "text", Room: "room-a", Data: "hello", ThreadID: &threadID, ThreadScope: &threadScope},
			wantJSON: `{"type":"text","room":"room-a","data":"hello","threadId":"12345","threadScope":2}`,
			wantRound: ReplyRequest{Type: "text", Room: "room-a", Data: "hello", ThreadID: &threadID, ThreadScope: &threadScope},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONRoundTrip(t, tt.input, tt.wantJSON, tt.wantRound, "ReplyRequest")
		})
	}
}

func assertJSONRoundTrip[T any](t *testing.T, input T, wantJSON string, wantRound T, label string) {
	t.Helper()
	gotJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if string(gotJSON) != wantJSON {
		t.Fatalf("json.Marshal() = %s, want %s", gotJSON, wantJSON)
	}
	var got T
	if err := json.Unmarshal(gotJSON, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	assertJSONEqual(t, got, wantRound, label)
}

func assertJSONEqual[T any](t *testing.T, got, want T, label string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %+v, want %+v", label, got, want)
	}
}
```

- [ ] **Step 6: client/options_test.go에 SendOption 테스트 병합**

기존 `client/options_test.go`에 루트 `options_test.go`의 SendOption/Validate 관련 테스트를 추가.
함수명을 unexported 버전에 맞게 변경: `ApplySendOptions` → `applySendOptions`, `ValidateSendOptions` → `validateSendOptions`.

- [ ] **Step 7: client/normalize_test.go 생성**

루트 `options_test.go`의 `TestNormalizeReplyThreadID`, `TestNormalizeReplyThreadScope` 테스트를 복사.
함수명을 unexported 버전에 맞게 변경: `NormalizeReplyThreadID` → `normalizeReplyThreadID` 등.
**반드시 포함할 테스트 헬퍼 함수** (루트 `options_test.go` 250-268행):
- `stringPtr(s string) *string`
- `equalStringPtr(got, want *string) bool`
- `equalIntPtr(got, want *int) bool`

이 헬퍼들은 normalize 테스트에서 포인터 비교에 사용됨.

- [ ] **Step 8: client/ 기존 파일의 iris 참조 제거**

다음 파일들에서 `import iris "park285/iris-client-go"` 제거하고, `iris.XXX` prefix를 제거:

**client/h2c_client.go:**
- `iris "park285/iris-client-go"` import 제거
- `iris.ApplySendOptions` → `applySendOptions`
- `iris.ValidateSendOptions` → `validateSendOptions`
- `iris.ReplyRequest` → `ReplyRequest`
- `iris.NormalizeReplyThreadID` → `normalizeReplyThreadID`
- `iris.NormalizeReplyThreadScope` → `normalizeReplyThreadScope`
- `iris.PathReply` → `PathReply`
- `iris.PathConfig` → `PathConfig`
- `iris.PathDecrypt` → `PathDecrypt`
- `iris.Config` → `Config`
- `iris.DecryptRequest` → `DecryptRequest`
- `iris.DecryptResponse` → `DecryptResponse`
- `iris.ResolveToken(c.botToken)` → `strings.TrimSpace(c.botToken)`
- `iris.HeaderBotToken` → `HeaderBotToken`

**client/sender.go:**
- `iris "park285/iris-client-go"` import 제거
- `iris.SendOption` → `SendOption`

**client/admin.go:**
- `iris "park285/iris-client-go"` import 제거
- `*iris.Config` → `*Config`

**client/ping.go:**
- `iris "park285/iris-client-go"` import 제거
- `iris.PathReady` → `PathReady`
- `iris.PathHealth` → `PathHealth`
- `iris.PathReply` → `PathReply`

**client/h2c_client_test.go:**
- `iris "park285/iris-client-go"` import 제거
- `iris.ReplyRequest` → `ReplyRequest`
- `iris.WithThreadID` → `WithThreadID`
- `iris.WithThreadScope` → `WithThreadScope`
- `iris.PathConfig` → `PathConfig`
- `iris.Config` → `Config`
- `iris.PathDecrypt` → `PathDecrypt`
- `iris.DecryptRequest` → `DecryptRequest`
- `iris.DecryptResponse` → `DecryptResponse`
- `iris.PathReply` → `PathReply`
- `iris.HeaderBotToken` → `HeaderBotToken`

**client/ping_test.go:**
- `iris "park285/iris-client-go"` import 제거
- `iris.PathReady` → `PathReady`
- `iris.PathHealth` → `PathHealth`
- `iris.PathReply` → `PathReply`

- [ ] **Step 9: client/ 테스트 실행**

Run: `go test ./client/ -v -count=1`
Expected: ALL PASS

- [ ] **Step 10: Commit**

```bash
git add client/
git commit -m "refactor: move root types/constants/options/helpers to client/"
```

---

### Task 2: webhook/ 패키지에 루트 코드 이동

**Files:**
- Create: `webhook/types.go`
- Create: `webhook/types_test.go`
- Create: `webhook/constants.go`
- Create: `webhook/thread.go`
- Create: `webhook/thread_test.go`
- Modify: `webhook/handler.go`
- Modify: `webhook/handler_test.go`

- [ ] **Step 1: webhook/types.go 생성**

```go
package webhook

type WebhookRequest struct {
	Route       string `json:"route,omitempty"`
	MessageID   string `json:"messageId,omitempty"`
	SourceLogID int64  `json:"sourceLogId,omitempty"`
	Text        string `json:"text"`
	Room        string `json:"room"`
	Sender      string `json:"sender"`
	UserID      string `json:"userId"`
	ChatLogID   string `json:"chatLogId,omitempty"`
	RoomType    string `json:"roomType,omitempty"`
	RoomLinkID  string `json:"roomLinkId,omitempty"`
	ThreadID    string `json:"threadId,omitempty"`
	ThreadScope *int   `json:"threadScope,omitempty"`
}

type Message struct {
	Msg    string       `json:"msg"`
	Room   string       `json:"room"`
	Sender *string      `json:"sender,omitempty"`
	JSON   *MessageJSON `json:"json,omitempty"`
}

type MessageJSON struct {
	UserID      string  `json:"user_id,omitempty"`
	Message     string  `json:"message,omitempty"`
	ChatID      string  `json:"chat_id,omitempty"`
	Type        string  `json:"type,omitempty"`
	Route       string  `json:"route,omitempty"`
	MessageID   string  `json:"message_id,omitempty"`
	ChatLogID   string  `json:"chat_log_id,omitempty"`
	RoomType    string  `json:"room_type,omitempty"`
	RoomLinkID  string  `json:"room_link_id,omitempty"`
	SourceLogID *int64  `json:"source_log_id,omitempty"`
	ThreadID    *string `json:"thread_id,omitempty"`
	ThreadScope *int    `json:"thread_scope,omitempty"`
}
```

- [ ] **Step 2: webhook/constants.go 생성**

```go
package webhook

import "time"

const (
	PathWebhook          = "/webhook/iris"
	HeaderIrisToken      = "X-Iris-Token"
	HeaderIrisMessageID  = "X-Iris-Message-Id"
)

const DefaultDedupTTL = 60 * time.Second
```

**주의:** `DefaultWebhookDedupTTL` → `DefaultDedupTTL`로 rename (webhook 패키지 내이므로 Webhook prefix 불필요).

- [ ] **Step 3: webhook/thread.go 생성**

루트 `thread.go`에서 `ResolveThreadID`, `DedupKey`를 복사. `ResolveToken`은 제외.

```go
package webhook

import "strings"

func ResolveThreadID(req *WebhookRequest) string {
	if req == nil {
		return ""
	}
	if id := strings.TrimSpace(req.ThreadID); id != "" {
		return id
	}
	chatLogID := strings.TrimSpace(req.ChatLogID)
	if chatLogID == "" {
		return ""
	}
	roomType := strings.TrimSpace(req.RoomType)
	roomLinkID := strings.TrimSpace(req.RoomLinkID)
	if strings.EqualFold(roomType, "OD") || roomLinkID != "" {
		return chatLogID
	}
	return ""
}

func DedupKey(messageID string) string {
	id := strings.TrimSpace(messageID)
	if id == "" {
		return ""
	}
	return "iris:msg:{" + id + "}"
}
```

- [ ] **Step 4: webhook/thread_test.go 생성**

루트 `thread_test.go`에서 `TestResolveThreadID`, `TestDedupKey` 테스트를 복사.
`TestResolveToken` 테스트는 제외 (stdlib 함수 테스트 불필요).

```go
package webhook

import "testing"

// TestResolveThreadID 및 TestDedupKey를 루트 thread_test.go에서 그대로 복사.
// WebhookRequest 타입 참조에서 iris. prefix만 제거.
```

- [ ] **Step 5: webhook/types_test.go 생성**

루트 `types_test.go`에서 WebhookRequest 관련 테스트 + 제네릭 헬퍼를 복사.

포함할 테스트:
- `TestWebhookRequestJSONMarshalLegacyCompatibility`
- `TestWebhookRequestJSONMarshalWithOptionalFields`
- `TestWebhookRequestJSONUnmarshalLegacy`
- `webhookMarshalLegacyCase`, `webhookMarshalOptionalFieldsCase`, `legacyWebhookUnmarshalCase` 헬퍼
- `assertJSONRoundTrip`, `assertJSONUnmarshal`, `assertJSONEqual` 제네릭 헬퍼

- [ ] **Step 6: webhook/ 기존 파일의 iris 참조 제거**

**webhook/handler.go:**
- `iris "park285/iris-client-go"` import 제거
- `iris.Message` → `Message` (모든 곳)
- `iris.MessageJSON` → `MessageJSON` (모든 곳)
- `iris.WebhookRequest` → `WebhookRequest` (모든 곳)
- `iris.ResolveToken(token)` → `strings.TrimSpace(token)` (line 92)
- `iris.HeaderIrisToken` → `HeaderIrisToken` (line 278)
- `iris.DedupKey(...)` → `DedupKey(...)` (line 310)
- `iris.HeaderIrisMessageID` → `HeaderIrisMessageID` (line 310)
- `iris.ResolveThreadID(...)` → `ResolveThreadID(...)` (line 576)
- `iris.DefaultWebhookDedupTTL` → `DefaultDedupTTL` (lines 665, 689)

**webhook/handler_test.go:**
- `iris "park285/iris-client-go"` import 제거
- `iris.Message` → `Message` (모든 곳)
- `iris.MessageJSON` → `MessageJSON` (모든 곳)
- `iris.HeaderIrisToken` → `HeaderIrisToken` (모든 곳)
- `iris.HeaderIrisMessageID` → `HeaderIrisMessageID` (모든 곳)

- [ ] **Step 7: webhook/ 테스트 실행**

Run: `go test ./webhook/ -v -count=1`
Expected: ALL PASS

- [ ] **Step 8: Commit**

```bash
git add webhook/
git commit -m "refactor: move root types/constants/helpers to webhook/"
```

---

### Task 3: 루트 Go 파일 삭제 및 정리

**Files:**
- Delete: `types.go`, `types_test.go`
- Delete: `constants.go`
- Delete: `options.go`, `options_test.go`
- Delete: `thread.go`, `thread_test.go`

- [ ] **Step 1: 루트 Go 파일 전체 삭제**

```bash
rm types.go types_test.go constants.go options.go options_test.go thread.go thread_test.go
```

- [ ] **Step 2: 전체 빌드 및 테스트**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: ALL PASS, no errors

- [ ] **Step 3: 완료 검증**

```bash
# 루트에 .go 파일 없음
ls *.go 2>/dev/null | wc -l  # Expected: 0

# 루트 import 잔존 없음
grep -r 'park285/iris-client-go"' --include='*.go' client/ webhook/ dedup/ | wc -l  # Expected: 0

# iris. prefix 잔존 없음 (테스트 파일 포함)
grep -rn 'iris\.' --include='*.go' client/ webhook/ | head -5  # Expected: 0
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: remove empty root package"
```

---

### Task 4: README.md 및 CLAUDE.md 업데이트

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: README.md 업데이트**

주요 변경:
- 패키지 구조 섹션: root 행 제거, client/webhook에 이동된 내용 반영
- Import 방향 규칙: root 의존 제거, 각 패키지 stdlib만 의존
- 사용법 예제: `iris "park285/iris-client-go"` import 제거
  - `iris.WithThreadID` → `client.WithThreadID`
  - `iris.WithThreadScope` → `client.WithThreadScope`
- 핵심 인터페이스 섹션:
  - `Sender` 시그니처: `iris.SendOption` → `SendOption`
  - `AdminClient` 시그니처: `*iris.Config` → `*Config`
- 외부 의존성: "루트" 언급 제거

- [ ] **Step 2: CLAUDE.md 업데이트**

Architecture 섹션 변경:
- Import direction: root 의존 제거
- Packages 설명: root 패키지 설명 제거, client/webhook에 이동된 타입/상수 반영
- Key patterns: `DefaultWebhookDedupTTL` → `DefaultDedupTTL` (webhook 패키지)

- [ ] **Step 3: 최종 테스트**

Run: `go test ./... -count=1 && go vet ./...`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: update README and CLAUDE.md for new package structure"
```
