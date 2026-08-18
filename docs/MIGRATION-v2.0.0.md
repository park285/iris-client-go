# iris-client-go v2 migration

`v2.0.0`은 webhook signature와 dedup/nonce 역할을 하나의 canonical path로 축소하는 major
release입니다.

## Module path

모든 import에 `/v2`를 추가합니다.

```go
import (
    "github.com/park285/iris-client-go/v2/iris"
    "github.com/park285/iris-client-go/v2/valkeydedup"
    "github.com/park285/iris-client-go/v2/webhook"
)
```

## Nonce store

모든 handler는 `webhook.WithNonceStore`로 explicit `webhook.SetOnceNonceStore`를 받아야 합니다.
누락하거나 set-once marker가 없는 store를 넘기면 constructor가
`webhook.ErrNonceStoreRequired`를 반환합니다.

```go
nonceStore := valkeydedup.NewNonceStore(valkeyClient)
handler, err := iris.NewDurableWebhookHandler(admitter,
    webhook.WithNonceStore(nonceStore),
)
```

## Message dedup

durable admission consumer는 inbox unique key가 idempotency를 소유하므로 message deduplicator를
주입하지 않습니다. non-durable consumer만 token-bound backend를 명시합니다.

```go
handler, err := iris.NewWebhookHandler(messageHandler,
    webhook.WithMessageDeduplicator(valkeydedup.NewMessageDeduplicator(valkeyClient)),
    webhook.WithNonceStore(valkeydedup.NewNonceStore(valkeyClient)),
)
```

`Reserve` 오류는 더 이상 dispatch를 계속하지 않습니다. 반환 token이 있으면 같은 token으로
`ReleaseReservation`을 시도하고 HTTP `503`을 반환합니다.

## HMAC

`webhooksign.SignRequest`는 authority-bound v3만 생성합니다. `SignRequestV3`와 v2 constant/helper는
제거됐습니다. receiver는 v2를 unknown version으로 거절합니다.

## Removed symbols

- `webhook.Deduplicator`
- `webhook.DedupReleaser`
- `webhook.StatefulDeduplicator`
- `webhook.WithDeduplicator`
- `webhook.WithNonceCache`
- `webhook.NoopDeduplicator`
- `webhook.SignatureVersionV2`
- `webhooksign.SignRequestV3`
- `valkeydedup.New`
- `valkeydedup.Option`
