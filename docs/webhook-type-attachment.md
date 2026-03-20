# Webhook Payload: `type`, `attachment` fields

## Overview

Webhook payload에 두 개의 optional string 필드가 추가되었습니다.

| Field | JSON key | Go type | Description |
|-------|----------|---------|-------------|
| `Type` | `type` | `string` | KakaoTalk 메시지 타입 코드 (예: `"1"`=텍스트, `"2"`=사진, `"26"`=답장) |
| `Attachment` | `attachment` | `string` | KakaoTalk DB의 복호화된 attachment JSON 원본 |

두 필드 모두 `omitempty` — 값이 없으면 JSON에서 생략됩니다.

## Payload Example

```json
{
  "text": "hello",
  "room": "room-1",
  "sender": "alice",
  "userId": "user-1",
  "type": "1",
  "attachment": "{\"url\":\"https://example.com/img.jpg\",\"w\":640,\"h\":480}"
}
```

Legacy payload (기존 필드만)는 그대로 호환됩니다.

## Go Struct Changes

### WebhookRequest

```go
type WebhookRequest struct {
    // ... existing fields ...
    Type       string `json:"type,omitempty"`
    Attachment string `json:"attachment,omitempty"`
}
```

### MessageJSON

```go
type MessageJSON struct {
    // ... existing fields ...
    Attachment string `json:"attachment,omitempty"`
}
```

`MessageJSON.Type`은 기존에 이미 존재했으나 매핑되지 않던 필드로, 이번 변경에서 `WebhookRequest.Type` 값이 매핑됩니다.

## Pipeline Behavior

| Stage | `Type` | `Attachment` |
|-------|--------|-------------|
| Normalize | `TrimSpace` 적용 | 변경 없음 (원본 보존) |
| Validate | max 256 runes | max 65536 runes (raw length) |
| Build MessageJSON | direct mapping | direct mapping |

### Attachment은 trim하지 않는 이유

`attachment`는 KakaoTalk DB에서 복호화된 JSON 원본입니다.
whitespace를 포함한 원본 데이터를 그대로 전달해야 하므로 normalization에서 제외됩니다.
validation도 raw length 기준으로 수행됩니다 (`validOptionalMax`의 trim 후 측정이 아닌 직접 `RuneCountInString`).

## Consumer Integration

### HandleMessage에서 type 분기

```go
func (h *MyHandler) HandleMessage(ctx context.Context, msg *webhook.Message) {
    if msg.JSON == nil {
        return
    }

    switch msg.JSON.Type {
    case "1":
        // text message
    case "2":
        // photo
    case "26":
        // reply
    default:
        // unknown or unset
    }
}
```

### Attachment 접근

```go
if msg.JSON != nil && msg.JSON.Attachment != "" {
    // attachment is raw JSON string from KakaoTalk
    // parse as needed for your use case
    var att map[string]any
    json.Unmarshal([]byte(msg.JSON.Attachment), &att)
}
```

## Known Type Codes

Iris 서버는 타입 코드에 대한 필터링/검증 없이 KakaoTalk DB 값을 그대로 전달합니다.
알려진 주요 코드:

| Code | Description |
|------|-------------|
| `"1"` | 텍스트 메시지 |
| `"2"` | 사진 |
| `"26"` | 답장 |

이 목록은 비공식이며, KakaoTalk 업데이트에 따라 변경될 수 있습니다.
