# iris-client-go

Iris (카카오톡 메시지 브릿지)용 Go 클라이언트 라이브러리 SDK입니다.

## 설치 (Installation)

```bash
go get github.com/park285/iris-client-go/v4@latest
```

현재 지원 major는 `/v4`입니다. v3 소비자는 [`CHANGELOG.md`](./CHANGELOG.md)의 `미출시` 절에 적힌
삭제 표면과 대체 방법을 따르십시오. [`v3.0.0`](./docs/MIGRATION-v3.0.0.md)·[`v2`](./docs/MIGRATION-v2.0.0.md)·
[`v0.11`](./docs/MIGRATION-v0.11.0.md) 마이그레이션 문서는 역사 문서이며 현재 업그레이드 가이드가 아닙니다.

SDK는 환경 변수를 읽지 않습니다. Base URL, bot-control 자격, transport, H3 CA·server name, webhook
비밀키는 모두 옵션으로 넘기며, 각 서비스의 설정 계층이 자기 env를 한 번 읽어 옵션을 조립합니다.

## JSON 계약

SDK의 JSON 실행 경로는 Go 1.27 `encoding/json/v2`를 사용합니다. 디코더는 중복된 object
이름과 잘못된 UTF-8을 거절하고 struct field 이름을 대소문자까지 정확히 일치시킵니다. webhook
request와 HTTP response body는 하나의 완전한 JSON 값이어야 하며, media 응답의 닫힌
경계는 알 수 없는 field도 거절합니다.

typed JSON 응답의 공통 decode 경로는 압축 해제 후 본문 전체를 **16 MiB**로 제한합니다.
정확히 상한인 응답은 허용하고 초과 확인에 필요한 1 byte까지만 추가로 읽습니다.
strict media 경로의 1 MiB 상한과 trailer의 별도 byte/time 제한은 유지합니다.
`WithHTTPClient`로 넣는 client의 transport는 요청 context가 취소되면 응답 본문 Read를 풀어야 합니다. typed 응답 trailer drain의 시간 상한이 그 취소에 기댑니다.
상한 초과는 `errors.Is(err, iris.ErrResponseTooLarge)`로 식별합니다.
공통 decode 경로의 POST 초과 응답은 서버 처리 결과를 확인할 수 없어 `iris.ErrTransport`도 유지하지만
`iris.ErrRetryable`로 분류하지 않고, idempotency key가 있어도 SDK가 자동 재시도하지 않습니다.
이는 이전에 허용하던 16 MiB 초과 typed 응답의 지원을 제한하는 변경입니다.
정책 근거는 `DEC-20260906-sdk-typed-json-response-budget`입니다.

JSON v2 기본값에 따라 nil slice와 map은 각각 `[]`와 `{}`로 인코딩됩니다. `omitempty` field는
JSON 관점에서 빈 값일 때 생략되며, 숫자와 bool의 기존 zero-value 생략 계약은 `omitzero`로
명시되어 있습니다. webhook의 raw JSON 값(`WebhookRequest.EventPayload`·`MessageJSON.EventPayload`·
`MessageContext.EventPayload()`)은 `encoding/json/jsontext.Value`입니다.

## 제네릭 메서드

타입 파라미터가 필요한 메서드는 Go 1.27 제네릭 메서드를 그대로 사용합니다.
`internal/client/transport`의 `doGet[T]`, `doSignedJSON[T]`, `postStrictJSON[T]`, `postJSON[T]`,
`retryPostJSON[T]`가 그렇습니다. 이들을 감싸는 패키지 수준 제네릭 함수 별칭이나 호환 래퍼는
두지 않습니다.

`.golangci.yml`의 `exclusions.rules`와 `exclusions.paths`에는 항목을 추가하지 않습니다.
`generated`와 `warn-unused`를 제외하면 비어 있어야 합니다(DEC-20260824-golangci-suppression-must-be-local).
위반은 먼저 코드로 고치고, 진짜 오탐인 경우에만 해당 줄의 `//nolint`·`#nosec` 주석에 사유를 적어
억제합니다.

## 빠른 시작 (Quick Start)

### 1. 메시지 발송 (Sending Messages)

```go
import "github.com/park285/iris-client-go/v4/iris"

c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"),
    iris.WithBotToken(botToken),
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
)
if err != nil {
    log.Fatalf("클라이언트 초기화 실패: %v", err)
}

// 텍스트 메시지 발송. 텍스트 발송 경로는 SendMessageAccepted 하나다.
accepted, err := c.SendMessageAccepted(ctx, "room-id", "Hello, World!",
    iris.WithThreadID("12345"),
)

// 이미지 메시지 발송. magic byte로 형식을 판별하지 못하면 요청 없이 오류를 반환한다.
accepted, err = c.SendImage(ctx, "room-id", pngBytes)

// 마크다운 메시지 발송 (텍스트 공유 카드 형태)
resp, err := c.SendMarkdown(ctx, "room-id", "**bold** text")
status, err := c.GetReplyStatus(ctx, resp.RequestID)

// 카카오톡 26.9.1 네이티브 서식으로 Markdown 변환
resp, err = c.SendRichText(ctx, "room-id", "**굵게** *기울임* ~~취소~~ ||스포일러||")

// 일반 파일 발송 (메모리 데이터 예시)
file := iris.NewReplyFileBytes("report.txt", "text/plain", []byte("report body"))
accepted, err = c.SendFile(ctx, "room-id", file,
    iris.WithClientRequestID("report:room-id:2026-07-22"),
)
```

파일 전송은 기존 `iris.Sender`를 확장하지 않는 별도 `iris.FileSender` capability입니다. SDK는
1 byte 이상 30 MiB 이하의 단일 file part를 `multipart/form-data`로 스트리밍하며 전체 파일이나
multipart body를 메모리에 복제하지 않습니다. caller-owned `io.ReaderAt`, deterministic retry와
`clientRequestId` 계약은 [파일 reply 전송](docs/file-replies.md)을 참조하십시오.

`APIClient.SendRichText`는 `/reply`에 `type:"richtext"`를 보냅니다.
변환은 Iris가 수행하며, `WithMention`의 대상은 변환된 본문 기준입니다.
기존 `SendMarkdown`과 최소 `Sender` 인터페이스는 변경하지 않습니다.
서버의 RichText 길이·서식·transport 계약은 [Iris README](https://github.com/park285/Iris#카카오-네이티브-서식-richtext)를 따릅니다.

Iris가 structured HTTP error를 반환하면 기존처럼 `errors.As(err, &httpErr)`로
`*iris.HTTPError`를 얻을 수 있습니다. 예외가 하나 있습니다. `WithReplyRetry`로 재시도하는 중에 앞선
시도가 결과 불명(transport 오류)으로 끝나면, 마지막 HTTP 오류가 `CLIENT_REQUEST_ID_*` code를 가진
409가 아닌 한 반환 오류는 `iris.ErrTransport`입니다. 이때 마지막 HTTP 오류는 메시지에만 남아
`*iris.HTTPError`와 code를 얻을 수 없습니다(아래 재시도 문단). `clientRequestId` 상태처럼
machine-readable code가 필요한 호출부는 `iris.HTTPErrorCode(err)`를 사용하십시오. code가 없거나
공개 token 계약을 벗어난 응답이면 빈 문자열을 반환합니다.
code는 SDK가 HTTP 오류 응답을 검증하면서 보존한 값만 사용합니다. `HTTPError.Body`는
잘리거나 redaction된 진단 문자열이며 machine-readable code의 원본이 아닙니다.
직접 만든 `&iris.HTTPError{Body: ...}`에서는 code가 반환되지 않습니다. 테스트에서 code가
필요하면 HTTP fixture에 실제 SDK 요청을 보내 얻은 오류를 사용하십시오.
machine-readable code는 전체 오류 본문이 **64 KiB 이하**이고 읽기 오류 없이 끝난 경우에만
파싱합니다. 정확히 상한은 허용하며 초과 감지용 1 byte와 bounded drain 외에는 더 읽지
않습니다. 초과·잘린 본문·읽기 오류에서는 HTTP status와 최대 512 byte 진단 excerpt만
유지하고 code를 비웁니다. 이를 `CLIENT_REQUEST_ID_FAILED`로 추정하거나 재발급하지 마십시오.

`409` 중 `CLIENT_REQUEST_ID_FAILED`는 durable queue handoff 이전 실패 — 즉 해당 id로
KakaoTalk 부수효과가 없었음 — 을 뜻하므로, 새 `clientRequestId`(예: `:r1`, `:r2`
generation suffix)로 **같은 payload**를 유한 세대 재전송하는 것이 안전하며 권장 처리입니다.
같은 409군의 `CLIENT_REQUEST_ID_OUTCOME_UNKNOWN`/`PAYLOAD_MISMATCH`/`ALREADY_EXISTS`와
code 없는 409는 이 보장이 없으므로 재발급 없이 종결하십시오.

`iris.WithReplyRetry(n)`은 `/reply`를 최초 요청을 포함해 최대 `n`번 보냅니다. HTTP 429는 항상,
transport 오류는 `clientRequestId`가 있을 때만 같은 id로 재시도하며 5xx와 그 밖의 4xx는 재시도하지
않습니다. 전송 전 확정 실패(`iris.ErrNotSent`)도 `clientRequestId`가 있을 때만 재시도하며, H3 egress
거부는 재시도하지 않습니다. 앞선 시도가 transport 오류로 끝나 결과를 알 수 없는데 이후 시도가 위 409 code 판정 없이
429·5xx·4xx로 끝나면, SDK는 마지막 HTTP 오류로 축약하지 않고 `iris.ErrTransport`(결과 불명)로
반환합니다. 마지막 HTTP 오류는 메시지에만 남습니다. `GetReplyStatus`의 `state`는 Iris wire 값
`queued`, `preparing`, `prepared`, `sending`, `handoff_completed`, `outcome_unknown`, `failed`를
그대로 전달합니다. `outcome_unknown`은 발신 실패가 증명되지 않은 상태이므로 `failed`처럼 재전송하지
마십시오(`DEC-20260731-reply-outcome-unknown-fail-closed`).

전송 오류 분류는 다음과 같습니다.

| 오류 | 의미 | `iris.ErrRetryable` |
|---|---|---|
| `iris.ErrTransport`(`*iris.TransportError`) | 요청이 Iris에 도달해 처리됐는지 알 수 없는 결과 불명 | `TransportError.Idempotent`일 때만 참(GET·조회 POST, `clientRequestId`가 있는 reply) |
| `iris.ErrNotSent` | 요청이 나가기 전에 실패가 확정됨(H3 egress 거부, `WithH3DialGuard`·`WithH3DialGuardForBaseURL` 사용 시 dial 전 DNS 해석 실패·취소, http1 transport의 probe 외 요청. guard가 없는 기본 h3 dial의 해석 실패는 `ErrTransport`). `ErrTransport`와 함께 성립하지 않음 | egress 거부는 거짓, 그 밖의 `*TransportError`는 참 |
| `*iris.HTTPError` | Iris가 HTTP 응답을 돌려줌 | 5xx·429는 참(이전과 같음) |

`clientRequestId` 없는 reply의 transport 오류는 결과 불명이지만 같은 요청을 다시 보내면 부수효과가
중복될 수 있어 `ErrRetryable`이 아닙니다. 응답 상한 초과(`iris.ErrResponseTooLarge`)도 `ErrRetryable`이
아닙니다.

`iris.ClassifyReplyStatus(state)`는 wire 상태만 분류하는 순수 함수입니다. 입력을 trim하거나
대소문자·과거 별칭을 정규화하지 않습니다.

| 정확한 wire 입력 | `iris.ReplyStatusClass` 결과 |
|---|---|
| `queued`, `preparing`, `prepared`, `sending` | `iris.ReplyStatusPending` |
| `handoff_completed` | `iris.ReplyStatusHandoffCompleted` |
| `failed` | `iris.ReplyStatusFailed` |
| `outcome_unknown`, 빈 문자열, 그 밖의 모든 값 | `iris.ReplyStatusUnknown` |

Pending은 handoff 완료가 아니며 HandoffCompleted도 수신자 전달 확인은 아닙니다.
Failed 분류만으로 재전송이 안전하다고 판단해서는 안 됩니다. HTTP 오류와 nil snapshot,
polling 간격·예산, 취소·join, 재시도·재발급 정책은 봇 호출부가 계속 소유합니다.
함수는 snapshot을 변경하지 않으므로 세부 wire 상태가 필요한 호출부는 `snapshot.State`를
그대로 사용할 수 있습니다.

### 2. 웹훅 수신 (Receiving Webhooks)

웹훅 수신 경로는 durable admission 하나입니다. `webhook.NewDurableHandler`는 HMAC 검증을 통과한
메시지를 `webhook.MessageAdmitter`에 넘겨 HTTP `200 OK` 전에 durable 저장소에 commit합니다.
메시지 실행(dispatch)과 idempotency는 소비자의 inbox(저장소 unique key)가 소유합니다.

```go
handler, err := webhook.NewDurableHandler(inboxRuntime,
    webhook.WithWebhookToken(webhookSecret), // 필수. SDK는 환경 변수를 읽지 않는다.
    webhook.WithNonceStore(valkeydedup.NewNonceStore(valkeyClient)),
    webhook.WithAdmitTimeout(200 * time.Millisecond),
)
if err != nil {
    log.Fatalf("웹훅 핸들러 생성 실패: %v", err)
}
defer handler.Close()

http.Handle("/webhook/iris", handler)
```

#### inbox 저장 형식

`MessageAdmitter.AdmitMessage(ctx, msg *webhook.Message, body []byte)`의 `body`는 HMAC 검증을 통과한
Iris 원본 webhook 본문(`WebhookRequest` JSON)입니다. inbox에는 이 바이트를 그대로 저장하고, claim할 때
`webhook.DecodeMessage(body)`로 `*webhook.Message`를 복원합니다. 저장 형식의 정본은 Iris wire 계약이며
`webhook.Message`·`MessageJSON`은 JSON 태그가 없는 메모리 표현입니다. `DecodeMessage`는 handler와 같은
decode·검증·정규화 경로를 써서 admission 때 받은 `msg`와 같은 값을 만들며, Iris 계약을 벗어난 본문은
오류를 반환합니다. 이전 SDK가 `webhook.Message`를 JSON으로 직렬화해 저장한 행은 v4로 읽을 수 없으므로
전환 전에 inbox의 비종결 행을 비우십시오.

admitter가 이미 admit된 메시지라는 뜻으로 `webhook.ErrAlreadyAdmitted`(래핑 포함)를 반환하면 handler는 duplicate로 계측하고 `200 OK`를 반환합니다. 그 밖의 admission 오류와 종료 중 수신은 `503 Service Unavailable`입니다. 종료 budget이 있는 서비스는 `handler.CloseContext(ctx)`를 사용하면 grace 만료 후 진행 중인 admission context를 취소할 수 있습니다. `Close()`는 무제한 context로 `CloseContext`를 부르며, `io.Closer` 관례를 따르는 표면이라 폐기 대상이 아닙니다.

#### HMAC nonce 저장소

HMAC replay 방지용 nonce는 `webhook.WithNonceStore`로 명시한 `webhook.SetOnceNonceStore`에
set-once로 기록합니다. `WithNonceStore`는 `SetOnceNonceStore`만 받으므로 set-once 마커가 없는 저장소는
컴파일 시점에 거절됩니다. 저장소가 없으면 생성자가 오류를 반환하며 process-local default나 no-op
nonce store는 없습니다. Valkey 구현은 `valkeydedup.NewNonceStore`입니다. nonce 저장소 오류나
`WithNonceTimeout`(기본 `200ms`) 초과는 admission 전에 `503`으로 끝납니다.

#### HMAC v3-only 계약

receiver는 authority-bound signature v3만 검증하며 v2는 unknown version으로 거절합니다.
`webhooksign.SignRequest`는 v3만 생성합니다. 진단값은 `V3Validated`, `UnknownRejected`,
`MalformedRejected`의 고정 cardinality 필드만 노출합니다.

웹훅 송신 테스트나 smoke 도구에서는 `X-Iris-Message-Id`를 먼저 설정한 뒤 `SignRequest`로
signature v3 header를 생성합니다. `req.Host`가 설정된 경우 URL authority와 canonical parity가
없으면 서명하지 않습니다.

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

서명 version·필수 header·timestamp 창·`X-Iris-Message-Id`·target 검사는 본문을 읽기 전에 끝내며, 여기서
실패한 요청은 본문을 읽지 않고 `401`로 끝납니다. 본문 해시·서명·nonce 검사는 본문을 읽은 뒤 합니다.
본문 읽기 deadline이나 transport timeout은 인증·admission 전에 HTTP `408 Request Timeout`으로 거절합니다. Iris는 이를 설정된 발송 횟수·시간 지평 안에서 재시도하며, 재발송할 때는 같은 `messageId`와 새로운 HMAC nonce로 서명해야 합니다. 잘못된 JSON은 `400`, 본문 크기 초과는 `413`, 인증 실패는 `401`을 유지합니다.

`WithAdmitTimeout`은 durable commit의 deadline입니다. **기본값은 `30s`이며 `0` 이하를 넘겨도 "무제한"이 아니라 이 기본값으로 정규화됩니다.** deadline이 끝나면 다른 admission 오류와 동일하게 HTTP `503 Service Unavailable`을 반환하므로 발신자가 재시도할 수 있습니다. 발신자의 attempt timeout은 Iris runtime profile에 설정된 값입니다(`125s`는 공식 fixture의 예시). admission deadline은 그보다 짧게 설정하여, 저장소가 정체됐을 때 admission goroutine이 요청 context가 끊길 때까지 살아남아 종료(`Close`)까지 지연시키는 대신 빠르게 `503`으로 되돌릴 수 있게 하십시오.

### 3. 조회 API (Query APIs)

```go
rooms, err := c.GetRooms(ctx)
members, err := c.GetMembers(ctx, chatID)

// 멤버 통계 조회
stats, err := c.QueryMemberStats(ctx, iris.QueryMemberStatsRequest{
    ChatID: chatID,
    Limit:  20,
})

// 최근 메시지 내역 조회
msgs, err := c.QueryRecentMessages(ctx, iris.QueryRecentMessagesRequest{
    ChatID:     chatID,
    Limit:      50,
    ChatLogIDs: []string{"500", "300"}, // 선택적 exact filter, 최대 1,000개
})

// 사용자의 최신 이벤트와 다음 older page 조회
events, err := c.GetRoomUserEventsBefore(ctx, chatID, userID, 500, 0)
if len(events) > 0 {
    older, err := c.GetRoomUserEventsBefore(ctx, chatID, userID, 500, events[len(events)-1].ID)
}

for _, msg := range msgs.Messages {
    fmt.Printf("[%d] %s: %s\n", msg.SequenceID, msg.SenderName, msg.Message)
}
```

### 4. BotClient와 readiness

봇 서비스용 최소 인터페이스는 `iris.BotClient`(`Sender` + `Ping`)입니다. `Ping(ctx) error`는 서명 없는
`GET /ready`를 한 번 보내고 결과를 error로 돌려줍니다. 200 외의 응답은 `*iris.HTTPError`(4xx는
`iris.ErrPermanent`, 5xx는 `iris.ErrRetryable`)이고 전송 실패는 `*iris.TransportError`입니다. SDK는 Ping을
다시 시도하거나 로그를 남기지 않으므로 재시도 간격과 로그는 호출자(readiness handler, lifecycle waiter)가
소유합니다.

Base URL을 실행 중에 바꿔야 하는 소비자는 자기 패키지에서 `iris.NewClient`로 새 client를 만들어
교체합니다. SDK는 Base URL 회전 client를 제공하지 않습니다.

---

## 클라이언트 설정 옵션 (Configuration)

```go
c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"), // 필수
    iris.WithBotToken(botToken),                 // 필수. bot-control 서명 자격
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"), // h3에서 필수. 시스템 루트로 넘어가지 않는다
    iris.WithTimeout(5 * time.Second),
    iris.WithReplyRetry(3),                      // 최초 요청을 포함한 최대 시도 횟수
    iris.WithTransport(iris.TransportH3),        // 생략하면 h3
)
```

`NewClient`는 옵션을 한 번만 적용하고, Base URL·bot token 누락, 잘못된 Base endpoint·transport, 없거나
읽을 수 없는 H3 CA를 생성 시점에 오류로 반환합니다. 호출마다 늦게 드러나는 초기화 오류 경로는 없습니다.

### 1. HTTP/3 전송 설정

Iris API의 기본 전송 프로토콜은 HTTP/3(QUIC)입니다. `WithTransport`를 생략하거나 비우면 `h3`이며, 이 경우
`https://` Base URL과 `WithH3CACertFile`이 필요합니다. H3 연결의 dial·handshake·idle 상한은 shared-go
`pkg/h3`와 같은 QUIC 상수로 고정되어 있으며 조정 옵션은 없습니다.

Base endpoint는 `iris.ParseBaseEndpoint`와 모든 client 생성 경로에서 같은 문법으로 검증합니다. 절대 `http`/`https` URL과 host가 필요하며 opaque URL, userinfo, query, fragment는 허용하지 않습니다. 끝의 `/`만 제거하고 `/tenant/iris` 같은 deployment prefix는 보존합니다. 실제 요청 URL에는 고정 API route를 prefix 뒤에 한 번 붙이지만 HMAC canonical target은 계속 `/reply` 같은 API route만 사용합니다.

```go
c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"),
    iris.WithBotToken(botToken),
    iris.WithTransport(iris.TransportH3),
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
    iris.WithH3ServerName("iris-host"),
)
defer c.Close()
```

지원 transport 값은 `iris.TransportH3`(`h3`)와 `iris.TransportHTTP1`(`http1`)뿐이고 이전 별칭은 거절합니다.
소비자가 기동 시 설정 값을 검증할 때는 `iris.ParseTransport(value)`로 SDK와 같은 어휘를 씁니다(앞뒤 공백과
대소문자 무시, 빈 값 거절). `http1`은 Iris HTTP/1 listener가 처리하는 loopback의 `GET /health`·`GET /ready`
probe 전용입니다. SDK가 만든 `http1` transport는 loopback이 아닌 host면 생성 시 거절하고, 그 밖의 route는
서명된 본문을 평문으로 내보내기 전에 `iris.ErrNotSent`로 거절합니다. 테스트에서 임의 HTTP 서버를 붙이려면
`WithHTTPClient`로 호출자 소유 transport를 넘깁니다.

운영 환경에서 H3 egress 대상을 Base URL host로 제한하려면 DNS allowset을 TTL마다 갱신하는 `WithH3DialGuardForBaseURL`을 사용할 수 있습니다. 만료 시 stale allowset이 **허용**하는 dial은 즉시 통과하고 refresh는 뒤에서 끝납니다. stale allowset이 **거부**하는 dial만 그 refresh 결과를 기다렸다 한 번 더 판정하므로, host의 IP가 바뀌어도 TTL 경계의 요청이 egress 거부로 희생되지 않습니다. 어느 경우든 동시 dial은 하나의 refresh를 공유하며, allowset이 아직 유효한 동안의 거부는 DNS를 조회하지 않고 즉시 반환합니다. dial의 context가 먼저 취소되면 기다리지 않고 거부합니다. 초기 DNS 해석에 실패하면 guard를 만들지 않고 오류를 반환합니다. 엉뚱한 host를 allowlist하지 않도록 `WithH3DialGuardForBaseURL`과 `WithBaseURL`에는 반드시 동일한 Base URL을 전달해야 합니다.

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
    iris.WithBotToken(botToken),
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
    dialGuard,
)
```

직접 정책을 구현해야 하는 경우 dial context와 대상 IP를 받는 `WithH3DialGuard`를 사용할 수 있습니다. guard가 에러를 반환하면 연결은 시도되지 않고, 반환 오류는 `iris.ErrNotSent`와 그 guard 에러를 감싸므로 `errors.Is`로 식별할 수 있습니다. 이 거부는 자동 재시도하지 않습니다.

### 2. 서명 비밀키

SDK가 호출하는 Iris 보호 라우트는 모두 BotControl 역할이므로 bot-control 자격 하나로 서명합니다.
Iris의 Inbound 역할(`/config*`)은 SDK가 호출하지 않아 그 비밀키를 받지 않으며, 역할 사이 폴백이 없는
Iris 쪽 계약(`DEC-20260926-stack-iris-client-go-role-secrets`)은 그대로입니다. `Ping`이 쓰는 `GET /ready`는
Iris의 인증 없는 route라 서명하지 않습니다.

```go
c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"),
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
    iris.WithBotToken(botControlToken), // BotControl 역할
)
```

### 3. 웹훅 핸들러 설정 (Webhook Handler Configuration)

```go
import (
    "github.com/park285/iris-client-go/v4/valkeydedup"
    "github.com/park285/iris-client-go/v4/webhook"
)

handler, err := webhook.NewDurableHandler(inboxRuntime,
    webhook.WithWebhookToken(webhookSecret),          // 필수
    webhook.WithNonceStore(valkeydedup.NewNonceStore(valkeyClient)),
    webhook.WithNonceTimeout(200 * time.Millisecond), // nonce 저장소 왕복 상한
    webhook.WithAdmitTimeout(200 * time.Millisecond), // durable commit 상한
    webhook.WithMaxBodyBytes(1 << 20),                // 최대 요청 크기 (1MB)
    webhook.WithContext(ctx),                         // 수명 context의 값만 물려받고 취소는 물려받지 않음
    webhook.WithMetrics(myPrometheusAdapter),
    webhook.WithWebhookLogger(slog.Default()),
)
```

* 웹훅 메시지 스키마(`webhook.Message`/`webhook.MessageJSON`)와 생성자·옵션은 모두 `webhook` 패키지에 있으며 `iris` 패키지는 웹훅 생성자를 두지 않습니다. `NewDurableHandler`는 nil admitter를 옵션 실행 전에 거절하고, 옵션을 순서대로 한 번 적용한 뒤(같은 옵션은 마지막 값) 인증·nonce 설정을 검증하고 활성화합니다. `WithWebhookToken`이 비면 생성이 실패합니다. 0 이하의 timeout·크기는 기본값으로 정규화합니다.
* optional `sourceCreatedAtMs`는 `WebhookRequest.SourceCreatedAtMS`로 decode되고 `MessageJSON.SourceCreatedAtMS`까지 그대로 전달됩니다. 값은 원본 Kakao row의 초 단위 시각을 millisecond로 표현한 계측 입력이며 message identity나 ordering key가 아닙니다.
* 메시지 순서와 실행은 소비자의 durable inbox가 소유합니다. handler는 in-memory 큐나 scheduler를 두지 않습니다.

---

## v4 공개 계약

- Webhook body의 `messageId`와 `X-Iris-Message-Id`는 모두 필수이고 같은 값이어야 합니다.
  `WebhookMention.userId`는 비어 있지 않은 문자열만 받습니다.
- `webhook.Message`의 본문과 방 정본은 `Msg`·`Room`입니다. `MessageJSON`은 식별자와
  이벤트 metadata만 담습니다. `NewMessageContext(msg).RoomID()`도 이 정본을 읽습니다.
- inbox 저장 형식은 Iris 원본 webhook 본문이며 `webhook.DecodeMessage`로 복원합니다.
- SDK는 환경 변수를 읽지 않습니다. 모든 설정과 비밀키는 옵션으로 받습니다.
- Iris가 반드시 거절하는 요청(인증 전 `400`인 decode query key 반복, 판별되지 않거나 허용되지 않는
  이미지 content type)은 SDK가 서명·전송 전에 오류로 끝냅니다.
- **배포 순서:** v4는 `WithThreadScope`를 삭제해 reply에 `threadScope`를 보내지 않습니다. v4 소비자는
  semantic hash의 `threadScope`를 정규화한 Iris를 먼저 배포한 뒤에만 배포합니다. 그럴 수 없으면
  hololive·twentyq·chat-bot-go-kakao의 reply outbox를 drain한 뒤 배포합니다. 정규화 전 Iris에서 이전 SDK가
  `threadScope` 2로 admit한 미종결 `clientRequestId`를 v4가 생략해 재전송하면 `CLIENT_REQUEST_ID_PAYLOAD_MISMATCH`가 됩니다.

## Iris wire 상수

소비자가 KakaoTalk·Iris wire 값을 리터럴로 복제하지 않도록 다음 상수를 공개합니다. 값의 정본은 Iris
코드·테스트입니다.

| 상수 | 값 | 정본 |
|---|---|---|
| `webhook.MessageTypeText`·`MessageTypePhoto`·`MessageTypeFile`·`MessageTypeReply`·`MessageTypeMultiPhoto` | `"1"`·`"2"`·`"18"`·`"26"`·`"27"` | Iris webhook payload와 참조 context |
| `webhook.RoomTypeDirectChat`·`RoomTypeMultiChat` | `"DirectChat"`·`"MultiChat"` | Iris read model room type |
| `webhook.OpenChatRoomTypePrefix` | `"O"` | 오픈채팅 room type 접두(예: `OD`, `OM`) |
| `webhook.MaxReferencedImages`·`MaxReferencedImageURLBytes` | `10`·`2048` | Iris `referenced_context/images.rs` |
| `iris.ReplyAdmissionRetention` | `168h` | Iris `DEFAULT_REPLY_ADMISSION_RETENTION` |
| `iris.ReplyAutomaticReplayHorizon` | `144h`(보존 − `iris.ReplyReplayHorizonMargin` `24h`) | 저장된 `clientRequestId`로 자동 재발송을 허용하는 상한 |

---

## 라이브러리 구조 (Directory Layout)

```text
iris/              # SDK Facade - 외부 노출용 엔트리 포인트 (NewClient, ParseTransport 등)
webhook/           # durable WebhookHandler, DecodeMessage와 메시지 스키마 정의
webhooksign/       # Webhook signature v3 요청 header 생성 helper
valkeydedup/       # Valkey 기반 webhook HMAC nonce store
internal/client/   # transport/signing/multipart/query/common 내부 구현
```

---

## 라이선스 (License)

Apache License 2.0 — [LICENSE](LICENSE)
