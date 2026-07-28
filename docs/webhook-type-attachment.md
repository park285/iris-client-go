# Webhook Payload: `type`, `attachment` fields

> 현행화 기준: 2026-07-28, Iris `webhook_payload/mod.rs`·`attachment.rs`·`referenced_context.rs`.

## Overview

Webhook payload의 optional string 필드 두 개의 계약입니다.

| Field | JSON key | Go type | Description |
|-------|----------|---------|-------------|
| `Type` | `type` | `string` | webhook 메시지 타입 식별자. 일반 메시지에서는 KakaoTalk 타입 코드 문자열(예: `"1"`=텍스트, `"2"`=사진, `"26"`=답장), semantic 이벤트에서는 `member_nickname_updated` |
| `Attachment` | `attachment` | `string` | Iris가 allowlist로 정제한 attachment metadata JSON 문자열. opt-in(`include_attachment_payload`)일 때만 전달 |

두 필드 모두 `omitempty` — 값이 없으면 JSON에서 생략됩니다.

semantic 이벤트는 `member_nickname_updated` 하나입니다.

## Attachment 정제 규칙 (Iris 서버 측)

Iris는 attachment 원본을 전달하지 않습니다.

- opt-in: Iris의 global wildcard 또는 route allowlist가 설정된 경우에만 `attachment`가 포함됩니다.
- 대상 타입: `type ∈ {"1","2","23","26"}`만 attachment를 전달합니다.
- allowlist 키: `mediaType`, `mimeType`, `type`, `width`, `height`, `size`, `duration`.
- 타입 `1`/`26`(답장·참조)은 source row를 찾으면 `src_type ∈ {"1","2","27","71"}`의 canonical context를 전달할 수 있습니다.
- canonical reference 키는 `src_type`, `srcLogId`, bounded `src_message`이며, `src_type="2"`는 URL만 남긴 `src_attachment`, `src_type="27"`/`"71"`은 ordered `src_images`와 `src_images_truncated`를 사용할 수 있습니다.
- reference URL은 credential이 없는 absolute HTTPS URI, 최대 2,048 bytes, 중복 없는 최대 10개만 허용됩니다. 유효한 signed URL 문자열은 정규화하지 않습니다.
- 일반 attachment의 URL, 파일 경로, raw blob과 type-71의 raw `C`/`P`·button/link·device path·임의 중첩 필드는 전달되지 않습니다.

Reference URL은 attachment opt-in route에만 노출되는 예외입니다. Iris는 `src_attachment`, `src_images`와 truncation marker를 durable room events/facts, direct query, SSE에서 제거하며 과거 row도 read/publish 경계에서 비파괴 redaction합니다. Consumer는 다운로드 전에 host allowlist, redirect target, content type과 aggregate size를 추가 검증해야 합니다.

## Payload Example

```json
{
  "text": "hello",
  "room": "room-1",
  "sender": "alice",
  "userId": "user-1",
  "type": "2",
  "attachment": "{\"mediaType\":\"image\",\"mimeType\":\"image/jpeg\",\"width\":640,\"height\":480,\"size\":204800}"
}
```

Legacy payload (기존 필드만)는 그대로 호환됩니다.

## Go Struct Mapping

`WebhookRequest.Type`/`Attachment` → `MessageJSON.Type`/`Attachment`로 direct mapping됩니다.

```go
type WebhookRequest struct {
    // ... existing fields ...
    Type       string `json:"type,omitempty"`
    Attachment string `json:"attachment,omitempty"`
}
```

## Pipeline Behavior (클라이언트 핸들러)

| Stage | `Type` | `Attachment` |
|-------|--------|-------------|
| Normalize | `TrimSpace` 적용 | 변경 없음 (수신값 보존) |
| Validate | max 256 runes | max 65536 runes (raw length) |
| Build MessageJSON | direct mapping | direct mapping |

### Attachment은 trim하지 않는 이유

`attachment`는 Iris가 정제해 직렬화한 JSON 문자열입니다.
whitespace를 포함한 수신값을 그대로 전달해야 하므로 normalization에서 제외됩니다.
validation도 raw length 기준으로 수행됩니다 (`validOptionalMax`의 trim 후 측정이 아닌 직접 `RuneCountInString`).

## Consumer Integration

### HandleMessage에서 type 분기

```go
func (h *MyHandler) HandleMessage(ctx context.Context, msg *webhook.Message) {
    if msg.JSON == nil {
        return
    }

    switch msg.JSON.Type {
    case "member_nickname_updated":
        // semantic event — EventPayload에 MemberNicknameUpdatedEvent JSON
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
    // attachment is the sanitized metadata JSON string from Iris
    var att map[string]any
    json.Unmarshal([]byte(msg.JSON.Attachment), &att)
}
```

## Known Type Values

### Semantic event type

| Value | Description |
|-------|-------------|
| `"member_nickname_updated"` | 닉네임 변경 이벤트 (유일한 semantic 타입). `previousDisplayName`/`currentDisplayName`은 `eventPayload`에 포함 |

### KakaoTalk message type code

일반 채팅 webhook에서는 KakaoTalk DB 타입 코드가 들어옵니다. 알려진 값:

| Code | Description |
|------|-------------|
| `"1"` | 텍스트 메시지 |
| `"2"` | 사진 |
| `"3"` | 동영상 |
| `"5"` | 지도 |
| `"6"` | 음성 |
| `"12"` | 음악 |
| `"13"` | 이모티콘 |
| `"14"` | 스티커 |
| `"15"` | 파일 |
| `"16"` | URL |
| `"23"` | 앨범 |
| `"26"` | 답장 |
| `"27"` | 오픈채널 포스트 |
| `"71"` | 라이브톡 |

이 목록은 비공식이며 KakaoTalk 업데이트에 따라 변경될 수 있습니다.
