# iris-client-go

Iris (카카오톡 메시지 브릿지)용 Go 클라이언트 라이브러리 SDK입니다.

## 설치 (Installation)

```bash
go get github.com/park285/iris-client-go/v3@latest
```

현재 지원 major는 `/v3`입니다. v2 소비자는 [`v3.0.0 마이그레이션 가이드`](./docs/MIGRATION-v3.0.0.md)를
따르십시오. 이전 기록은 [`v2 마이그레이션 가이드`](./docs/MIGRATION-v2.0.0.md)와
[`CHANGELOG.md`](./CHANGELOG.md)에 있습니다. [`v0.11 마이그레이션`](./docs/MIGRATION-v0.11.0.md)은
역사 문서이며 현재 v3 업그레이드 가이드가 아닙니다.

## JSON 계약

SDK의 JSON 실행 경로는 Go 1.27 `encoding/json/v2`를 사용합니다. 디코더는 중복된 object
이름과 잘못된 UTF-8을 거절하고 struct field 이름을 대소문자까지 정확히 일치시킵니다. webhook
request와 HTTP response body는 하나의 완전한 JSON 값이어야 하며, media 응답의 닫힌
경계는 알 수 없는 field도 거절합니다.

typed JSON 응답의 공통 decode 경로는 압축 해제 후 본문 전체를 **16 MiB**로 제한합니다.
정확히 상한인 응답은 허용하고 초과 확인에 필요한 1 byte까지만 추가로 읽습니다.
strict media 경로의 1 MiB 상한과 trailer의 별도 byte/time 제한은 유지합니다.
`WithHTTPClient`로 넣는 client의 transport는 요청 context가 취소되면 응답 본문 Read를 풀어야 합니다. typed 응답 trailer와 `SendMessage`처럼 본문을 버리는 2xx 응답의 drain 시간 상한이 그 취소에 기댑니다. 버리는 본문의 drain 결과는 2xx 결과를 바꾸지 않습니다.
상한 초과는 `errors.Is(err, iris.ErrResponseTooLarge)`로 식별합니다.
공통 decode 경로의 POST 초과 응답은 서버 처리 결과를 확인할 수 없어 `iris.ErrTransport`도 유지하지만
`iris.ErrRetryable`로 분류하지 않고, idempotency key가 있어도 SDK가 자동 재시도하지 않습니다.
이는 이전에 허용하던 16 MiB 초과 typed 응답의 지원을 제한하는 변경입니다.
정책 근거는 `DEC-20260906-sdk-typed-json-response-budget`입니다.

JSON v2 기본값에 따라 nil slice와 map은 각각 `[]`와 `{}`로 인코딩됩니다. `omitempty` field는
JSON 관점에서 빈 값일 때 생략되며, 숫자와 bool의 기존 zero-value 생략 계약은 `omitzero`로
명시되어 있습니다. 공개 payload의 `encoding/json.RawMessage` 명명 타입은 호환성을 위해
유지하지만, 해당 값을 처리하는 실행 경로는 v2입니다.

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
import "github.com/park285/iris-client-go/v3/iris"

c, err := iris.NewClient()
if err != nil {
    log.Fatalf("클라이언트 초기화 실패: %v", err)
}

// 텍스트 메시지 발송
err = c.SendMessage(ctx, "room-id", "Hello, World!",
    iris.WithThreadID("12345"),
)

// 이미지 메시지 발송 (Base64 인코딩 데이터)
err = c.SendImage(ctx, "room-id", base64Img)

// 마크다운 메시지 발송 (텍스트 공유 카드 형태)
resp, err := c.SendMarkdown(ctx, "room-id", "**bold** text")
status, err := c.GetReplyStatus(ctx, resp.RequestID)

// 카카오톡 26.9.1 네이티브 서식으로 Markdown 변환
resp, err = c.SendRichText(ctx, "room-id", "**굵게** *기울임* ~~취소~~ ||스포일러||")

// 일반 파일 발송 (메모리 데이터 예시)
file := iris.NewReplyFileBytes("report.txt", "text/plain", []byte("report body"))
accepted, err := c.SendFile(ctx, "room-id", file,
    iris.WithClientRequestID("report:room-id:2026-07-22"),
)
```

파일 전송은 기존 `iris.Sender`를 확장하지 않는 별도 `iris.FileSender` capability입니다. SDK는
1 byte 이상 30 MiB 이하의 단일 file part를 `multipart/form-data`로 스트리밍하며 전체 파일이나
multipart body를 메모리에 복제하지 않습니다. caller-owned `io.ReaderAt`, deterministic retry와
`clientRequestId` 계약은 [파일 reply 전송](docs/file-replies.md)을 참조하십시오.

`APIClient.SendRichText`와 `RebindingClient.SendRichText`는 `/reply`에 `type:"richtext"`를 보냅니다.
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
않습니다. 앞선 시도가 transport 오류로 끝나 결과를 알 수 없는데 이후 시도가 위 409 code 판정 없이
429·5xx·4xx로 끝나면, SDK는 마지막 HTTP 오류로 축약하지 않고 `iris.ErrTransport`(결과 불명)로
반환합니다. 마지막 HTTP 오류는 메시지에만 남습니다. `GetReplyStatus`의 `state`는 Iris wire 값
`queued`, `preparing`, `prepared`, `sending`, `handoff_completed`, `outcome_unknown`, `failed`를
그대로 전달합니다. `outcome_unknown`은 발신 실패가 증명되지 않은 상태이므로 `failed`처럼 재전송하지
마십시오(`DEC-20260731-reply-outcome-unknown-fail-closed`).

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
    webhook.WithWebhookToken("webhook-secret"), // 또는 IRIS_WEBHOOK_TOKEN 환경변수 사용
    webhook.WithNonceStore(valkeydedup.NewNonceStore(valkeyClient)),
    webhook.WithAdmitTimeout(200 * time.Millisecond),
)
if err != nil {
    log.Fatalf("웹훅 핸들러 생성 실패: %v", err)
}
defer handler.Close()

http.Handle("/webhook/iris", handler)
```

admitter가 이미 admit된 메시지라는 뜻으로 `webhook.ErrAlreadyAdmitted`(래핑 포함)를 반환하면 handler는 duplicate로 계측하고 `200 OK`를 반환합니다. 그 밖의 admission 오류와 종료 중 수신은 `503 Service Unavailable`입니다. 종료 budget이 있는 서비스는 `handler.CloseContext(ctx)`를 사용하면 grace 만료 후 진행 중인 admission context를 취소할 수 있습니다. `Close()`는 무제한 context로 `CloseContext`를 부르며, `io.Closer` 관례를 따르는 표면이라 폐기 대상이 아닙니다.

#### HMAC nonce 저장소

HMAC replay 방지용 nonce는 `webhook.WithNonceStore`로 명시한 `webhook.SetOnceNonceStore`에
set-once로 기록합니다. 이 저장소가 없으면 생성자가 오류를 반환하며 process-local default나 no-op
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

### 4. BotClient 및 RebindingClient

다중 인프라 혹은 동적 환경을 지원하기 위해, 봇 서비스를 위한 최소 인터페이스인 `iris.BotClient` (`Sender` + `Ping`) 및 동적으로 Base URL을 핫스왑할 수 있는 `iris.RebindingClient`를 제공합니다.

```go
rc := iris.NewRebindingClient(iris.RebindingClientConfig{
    ResolveBaseURL:  func() (string, error) { return readBaseURL() },
    BotToken:        token,
    ResolveInterval: time.Second,      // URL 또는 resolver 오류 snapshot의 최대 유지 시간
    StaleCloseGrace: 30 * time.Second, // 동적 교체된 이전 클라이언트 연결 정리 유예 시간
})
defer rc.Close()
```

`ResolveInterval`이 `0`이면 각 비동시 호출에서 즉시 Base URL을 다시 확인하는 기존 동작을 유지합니다. 양수이면 interval 안의 호출이 마지막 URL 또는 resolver 오류 snapshot을 공유하고 만료 후 첫 호출이 refresh를 수행합니다. 같은 시점의 동시 호출은 하나의 refresh 결과를 공유합니다.

refresh는 개별 API 호출이 아니라 `RebindingClient`가 소유합니다. refresh를 시작한 호출의 context가 취소되어도 해당 호출만 먼저 반환하며 진행 중인 refresh는 다른 동시 호출과 cache snapshot을 위해 완료됩니다. `Close()`는 대기 중인 호출을 즉시 깨우지만 context를 받지 않는 `ResolveBaseURL` 실행을 강제로 중단할 수는 없으므로 resolver는 유한 시간 안에 반환해야 합니다.

클라이언트 초기화 오류는 원본 Base URL을 포함하지 않고 정제된 parse/transport 오류를 감쌉니다. 사용자 제공 resolver가 직접 반환하는 오류의 내용은 resolver가 정제해야 합니다.

---

## 클라이언트 설정 옵션 (Configuration)

```go
c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"), // 또는 IRIS_BASE_URL 환경변수 사용
    iris.WithBotToken("my-token"),              // 또는 IRIS_BOT_TOKEN 환경변수 사용
    iris.WithTimeout(5 * time.Second),
    iris.WithLogger(slog.Default()),
    iris.WithReplyRetry(3),                     // 최초 요청을 포함한 최대 시도 횟수
    iris.WithTransport("h3"),                   // 또는 IRIS_TRANSPORT 환경변수 사용
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
)
```

### 1. HTTP/3 전송 설정

Iris API의 기본 전송 프로토콜은 HTTP/3(QUIC)입니다. `IRIS_TRANSPORT` 환경 변수가 누락된 경우 기본적으로 `h3` 전송이 적용되며 이 경우 `https://` 스키마가 포함된 Base URL을 설정해야 합니다.

Base endpoint는 `iris.ParseBaseEndpoint`와 모든 client 생성 경로에서 같은 문법으로 검증합니다. 절대 `http`/`https` URL과 host가 필요하며 opaque URL, userinfo, query, fragment는 허용하지 않습니다. 끝의 `/`만 제거하고 `/tenant/iris` 같은 deployment prefix는 보존합니다. 실제 요청 URL에는 고정 API route를 prefix 뒤에 한 번 붙이지만 HMAC canonical target은 계속 `/reply` 같은 API route만 사용합니다.

```go
c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"),
    iris.WithBotToken("my-token"),
    iris.WithTransport("h3"),
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
    iris.WithH3ServerName("iris-host"),
)
defer c.Close()
```

`IRIS_TRANSPORT=h3` 옵션은 `https://` 보안 연결에서만 활성화됩니다. 지원 값은 `h3`와 `http1`뿐입니다. 이전 별칭은 거절합니다. 현재 Iris runtime에서 `http1`은 loopback의 `GET /health`, `GET /ready` probe와 transport 단위 테스트에만 사용합니다. reply와 query를 포함한 보호 메서드는 `h3`와 `https://` Base URL이 필요합니다. 그 밖의 전송 값은 지원하지 않습니다.

운영 환경에서 H3 egress 대상을 Base URL host로 제한하려면 DNS allowset을 TTL마다 갱신하는 `WithH3DialGuardForBaseURL`을 사용할 수 있습니다. 만료 시 stale allowset이 **허용**하는 dial은 즉시 통과하고 refresh는 뒤에서 끝납니다. stale allowset이 **거부**하는 dial만 그 refresh 결과를 기다렸다 한 번 더 판정하므로, host의 IP가 바뀌어도 TTL 경계의 요청이 egress 거부로 희생되지 않습니다. 어느 경우든 동시 dial은 하나의 refresh를 공유하며, allowset이 아직 유효한 동안의 거부는 DNS를 조회하지 않고 즉시 반환합니다. dial의 context가 먼저 취소되면 기다리지 않고 거부합니다. 초기 DNS 해석 실패는 기본적으로 오류를 반환하며 `WithH3DialGuardLenientInit`을 지정하면 deny-all 상태로 기동한 뒤 TTL이 만료된 첫 dial이 refresh를 수행해 자가회복합니다. 엉뚱한 host를 allowlist하지 않도록 `WithH3DialGuardForBaseURL`과 `WithBaseURL`에는 반드시 동일한 Base URL을 전달해야 합니다.

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
    iris.WithTransport("h3"),
    dialGuard,
)
```

직접 정책을 구현해야 하는 경우 dial context와 대상 IP를 받는 `WithH3DialGuard`를 사용할 수 있습니다. guard가 에러를 반환하면 연결은 시도되지 않고, 반환 오류는 그 guard 에러를 감싸므로 `errors.Is`로 식별할 수 있습니다. 이 거부는 자동 재시도하지 않습니다.

### 2. 서명 비밀키

SDK가 호출하는 Iris 보호 라우트는 모두 BotControl 역할이므로 bot-control 자격 하나로 서명합니다.
Iris의 Inbound 역할(`/config*`)은 SDK가 호출하지 않아 그 비밀키를 받지 않으며, 역할 사이 폴백이 없는
Iris 쪽 계약(`DEC-20260926-stack-iris-client-go-role-secrets`)은 그대로입니다.

```go
c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"),
    iris.WithTransport("h3"),
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
    iris.WithBotToken("bot-control-token"), // BotControl 역할(IRIS_BOT_TOKEN, NewAPIClient의 botToken 인자)
)
```

### 3. 웹훅 핸들러 설정 (Webhook Handler Configuration)

```go
import (
    "github.com/park285/iris-client-go/v3/valkeydedup"
    "github.com/park285/iris-client-go/v3/webhook"
)

handler, err := webhook.NewDurableHandler(inboxRuntime,
    webhook.WithWebhookToken("webhook-secret"),       // 또는 IRIS_WEBHOOK_TOKEN 환경변수 사용
    webhook.WithNonceStore(valkeydedup.NewNonceStore(valkeyClient)),
    webhook.WithNonceTimeout(200 * time.Millisecond), // nonce 저장소 왕복 상한
    webhook.WithAdmitTimeout(200 * time.Millisecond), // durable commit 상한
    webhook.WithMaxBodyBytes(1 << 20),                // 최대 요청 크기 (1MB)
    webhook.WithContext(ctx),                         // 수명 context의 값만 물려받고 취소는 물려받지 않음
    webhook.WithMetrics(myPrometheusAdapter),
    webhook.WithWebhookLogger(slog.Default()),
)
```

* 웹훅 메시지 스키마(`webhook.Message`/`webhook.MessageJSON`)와 생성자·옵션은 모두 `webhook` 패키지에 있으며 `iris` 패키지는 웹훅 생성자를 두지 않습니다. `NewDurableHandler`는 nil admitter를 옵션 실행 전에 거절하고, 옵션을 순서대로 한 번 적용한 뒤(같은 옵션은 마지막 값) 인증·nonce 설정을 검증하고 활성화합니다. `WithWebhookToken`이 비면 옵션 적용 뒤 `IRIS_WEBHOOK_TOKEN`을 읽습니다. 0 이하의 timeout·크기는 기본값으로 정규화합니다.
* optional `sourceCreatedAtMs`는 `WebhookRequest.SourceCreatedAtMS`로 decode되고 `MessageJSON.SourceCreatedAtMS`까지 그대로 전달됩니다. 값은 원본 Kakao row의 초 단위 시각을 millisecond로 표현한 계측 입력이며 message identity나 ordering key가 아닙니다.
* 메시지 순서와 실행은 소비자의 durable inbox가 소유합니다. handler는 in-memory 큐나 scheduler를 두지 않습니다.

---

## v3 공개 계약

- Webhook body의 `messageId`와 `X-Iris-Message-Id`는 모두 필수이고 같은 값이어야 합니다.
  `WebhookMention.userId`는 문자열만 받습니다.
- `webhook.Message`의 본문과 방 정본은 `Msg`·`Room`입니다. `MessageJSON`은 식별자와
  이벤트 metadata만 담습니다. `NewMessageContext(msg).RoomID()`도 이 정본을 읽습니다.
- Webhook HMAC 비밀키는 `webhook.NewDurableHandler`의 `WithWebhookToken` 또는 `IRIS_WEBHOOK_TOKEN`으로
  지정합니다.

삭제한 공개 API와 입력별 이관은 [`v3.0.0 마이그레이션 가이드`](./docs/MIGRATION-v3.0.0.md)를 참조하십시오.

---

## 환경 변수 (Environment Variables)

| 환경 변수 | 설명 |
|------|------|
| `IRIS_BASE_URL` | Iris 백엔드 서버 Base URL |
| `IRIS_BOT_TOKEN` | 봇 호출 API 인증용 Bearer 토큰 |
| `IRIS_WEBHOOK_TOKEN` | 웹훅 유효성 검증용 인바운드 인증 토큰 |
| `IRIS_TRANSPORT` | 메시지 전송용 프로토콜 (`h3` [기본값], `http1` 지원) |

* 코드 상에서 옵션 함수(`WithBaseURL` 등)로 주입된 값이 환경 변수로 로드된 값보다 항상 우선하여 적용됩니다.

---

## 라이브러리 구조 (Directory Layout)

```text
iris/              # SDK Facade - 외부 노출용 엔트리 포인트 (NewClient, NewAPIClient 등)
webhook/           # durable WebhookHandler와 메시지 스키마 정의
webhooksign/       # Webhook signature v3 요청 header 생성 helper
valkeydedup/       # Valkey 기반 webhook HMAC nonce store
internal/client/   # transport/signing/multipart/rebind/query/common 내부 구현
```

---

## 라이선스 (License)

Apache License 2.0 — [LICENSE](LICENSE)
