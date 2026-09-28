# iris-client-go v3.0.0 migration

v3는 Iris의 현행 입력·인증 계약에 맞춰 v2 호환 표면을 제거합니다. Go 1.27.1과 기존 외부 의존성 버전은 유지합니다. HMAC signature v3, strict `encoding/json/v2`, H3, `clientRequestId`의 `outcome_unknown`·재발급 판정은 유지합니다.

## Module path

`go.mod`와 모든 SDK import를 `github.com/park285/iris-client-go/v3`으로 바꿉니다. `iris`, `webhook`, `webhooksign`, `valkeydedup` 및 내부 테스트의 import도 같은 major를 사용합니다.

```go
import (
    "github.com/park285/iris-client-go/v3/iris"
    "github.com/park285/iris-client-go/v3/webhook"
)
```

## 인증과 전송

`iris.NewClient(iris.WithBotToken(botToken), iris.WithInboundSecret(inboundSecret), ...)`에서 bot token은 BotControl 라우트와 `/admin/cert-reload`를 서명합니다. `iris.NewAPIClient(baseURL, botToken, ...)`도 동일합니다. `/config*`는 `WithInboundSecret`이 없으면 요청 전에 `ErrInboundSecretRequired`로 실패합니다. `WithHMACSecret`, `WithBotControlToken`, `WithCertReloadToken`, `ErrCertReloadTokenRequired`는 삭제했습니다. `ReloadH3Certificate` 호출은 cert-reload 전용 옵션 없이 bot token으로 서명합니다.

`WithTransport`와 `IRIS_TRANSPORT`는 `h3` 또는 `http1`만 받습니다. 이전 `http3`, `http/3`, `quic`, `http`, `http/1.1` 별칭은 초기화 오류가 됩니다. 기본값 `h3`와 HTTPS 요구는 그대로입니다.

Webhook HMAC 키의 정본은 `webhook.WithWebhookToken(token)` 또는 `IRIS_WEBHOOK_TOKEN`입니다. `WithWebhookSecret`과 `SDKConfig.Secret`는 삭제했습니다. 직접 생성자 `webhook.NewHandler`·`NewDurableHandler`는 기존 token 인자를 쓰며, 이 생성자에 SDK 전용 `WithWebhookToken`·`WithWebhookLogger`·`WithContext`를 넣으면 오류를 반환합니다. `webhook.Handler.Close()`는 계속 사용합니다.

## Webhook 메시지

Iris가 보내는 body `messageId`와 `X-Iris-Message-Id` header를 모두 채우고 같은 값으로 서명하십시오. body 값이 빠진 요청은 유효한 서명이 있어도 HTTP 400입니다. 멘션은 문자열 `userId`만 받으며 숫자 `userId` 또는 `user_id`는 거절합니다. HMAC v3와 nonce set-once 검증 순서는 유지합니다.

`webhook.Message`의 본문·방 정본은 `Msg`·`Room`입니다. 중복된 `MessageJSON.Message`·`ChatID`가 삭제되었으므로 저장 payload 생성, 구조체 리터럴, fallback 읽기를 수정하십시오. `webhook.NewMessageContext(msg).Text()`·`RoomID()`는 이 두 정본 필드를 읽고, `MessageID()`는 `MessageJSON.MessageID`를 읽습니다. `StableMessageIdentity()`와 sourceLogId/chatLogId 폴백은 삭제했습니다. 저장 키를 옮길 때 기존 prefix·fallback 값을 `messageId` 기준으로 명시적으로 이관하십시오. `MessageJSON`의 나머지 식별자·이벤트 metadata 필드는 유지합니다.

## Karing, 진단, 설정

`SendKaringHololive`, `KaringHololiveRequest`, `PathKaringHololive`를 삭제했습니다. `SendKaringContentList`에 `KaringContentListRequest.Items`를 넘기십시오. 단일 항목도 길이 1인 목록입니다. `KaringContentListRequest.Item`도 삭제했습니다. 요청의 `client_request_id` 정본은 유지합니다.

`KaringDryRunResponse`는 `dryRun`, `receiverName`, `templateId`, `itemCount`, `templateArgs` 정본 키를 인코딩·디코딩하며 snake_case 응답 별칭과 `streamCount`를 거절합니다. 공개 `StreamCount` 필드는 삭제했습니다.

`GetNativeCoreDiagnostics`, `NativeCoreDiagnostics`와 `/diagnostics/native-core` 호환 경로는 삭제했습니다. `GetRuntimeDiagnostics` 응답의 `nativeCore`를 읽으십시오. `ConfigState.WebEndpoint`도 삭제했습니다. `ConfigResponse.User`·`Applied`와 `ConfigUpdateResponse.User`·`RuntimeApplied`에서 `Webhooks["default"]`를 읽으십시오.

## 검증

소비자 코드를 바꾼 뒤 `GOWORK=off go test ./...`와 해당 서비스의 build·계약 gate를 실행하십시오. v3 tag·immutable release 검증이 끝난 뒤 소비자의 module pin을 새 버전으로 옮기십시오. v2와 v3는 Go module에서 서로 다른 import path입니다.
