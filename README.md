# iris-client-go

Iris (카카오톡 메시지 브릿지)용 Go 클라이언트 라이브러리 SDK입니다.

## 설치 (Installation)

```bash
go get github.com/park285/iris-client-go@latest
```

`v1.0.0`은 공개 표면 축소를 포함한 첫 stable major 릴리스로, 하위 호환성이 깨지는 변경 사항(Breaking Changes — 무소비 facade re-export 및 no-op webhook 옵션 제거)이 있습니다. 업그레이드 전에 [`CHANGELOG.md`](./CHANGELOG.md)의 v1.0.0 항목을 반드시 확인하시기 바랍니다. `v0.11.0` 미만에서 올라오는 경우 [`MIGRATION-v0.11.0.md`](./docs/MIGRATION-v0.11.0.md)도 함께 확인하십시오.

## 빠른 시작 (Quick Start)

### 1. 메시지 발송 (Sending Messages)

```go
import "github.com/park285/iris-client-go/iris"

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

// 일반 파일 발송 (메모리 데이터 예시)
file := iris.NewReplyFileBytes("report.txt", "text/plain", []byte("report body"))
accepted, err := c.SendFile(ctx, "room-id", file,
    iris.WithClientRequestID("report:room-id:2026-07-22"),
)
```

파일 전송은 기존 `iris.Sender`를 확장하지 않는 별도 `iris.FileSender` capability입니다. SDK는
1 byte 이상 30 MiB 이하의 단일 file part를 `multipart/form-data`로 스트리밍하며 전체 파일이나
multipart body를 메모리에 복제하지 않습니다. caller-owned `io.ReaderAt`, path helper의 descriptor
수명, deterministic retry와 `clientRequestId` 계약은 [파일 reply 전송](docs/file-replies.md)을
참조하십시오.

Iris가 structured HTTP error를 반환하면 기존처럼 `errors.As(err, &httpErr)`로
`*iris.HTTPError`를 얻을 수 있습니다. `clientRequestId` 상태처럼 machine-readable code가
필요한 호출부는 `iris.HTTPErrorCode(err)`를 사용하십시오. code가 없거나 공개 token 계약을
벗어난 응답이면 빈 문자열을 반환합니다.

### 2. 웹훅 수신 (Receiving Webhooks)

```go
handler, err := iris.NewWebhookHandler(myMessageHandler,
    valkeydedup.Option(valkeyClient),
)
if err != nil {
    log.Fatalf("웹훅 핸들러 생성 실패: %v", err)
}
defer handler.Close()

http.Handle("/webhook/iris", handler)
```

`WithQueueSize`는 ordering scheduler가 소유하는 전체 pending 상한입니다. 내부 실행 pool은 별도 buffered queue를 만들지 않습니다. 종료 budget이 있는 서비스는 `handler.CloseContext(ctx)`를 사용하면 grace 만료 후 queued callback을 건너뛰고 in-flight handler context를 취소할 수 있습니다. 기존 `Close()`는 무제한 context를 사용하는 호환 wrapper입니다.

HTTP `200 OK`가 메모리 admission이 아니라 durable commit을 의미해야 하는 소비자는 `webhook.MessageAdmitter`를 구현하고 `WithDurableAdmission`을 사용합니다. 이 모드에서는 scheduler와 deduplicator를 건너뛰므로 admitter의 저장소 unique key가 idempotency를 소유합니다.

#### idempotency 소유권과 dedup 상태 계약

| 모드 | idempotency 소유자 | 중복 요청 응답 | 상태 |
|---|---|---|---|
| durable (`WithDurableAdmission`) | admitter의 inbox unique key | admitter 구현이 결정 | 지원 |
| non-durable + `StatefulDeduplicator` | dedup backend의 token-bound 예약 | 선행 요청이 확정 전이면 `503`, 확정 후면 `200` | 지원 |
| non-durable + legacy `Deduplicator` | dedup backend의 set-once 키 | 항상 `200` | **제거 예정 잔여 경로** |

non-durable 모드에서 `WithDeduplicator`로 주입한 backend가 `webhook.StatefulDeduplicator`를 구현하면, 예약(reserve)과 admission 확정(commit)이 분리됩니다. enqueue가 성공해야 `Commit`으로 확정되고, 실패하면 자신의 owner token이 쥔 예약만 `ReleaseReservation`으로 해제한 뒤 `503`을 반환하므로 재전송이 다시 수용됩니다. 선행 요청이 아직 확정 전인 동안 도착한 동시 중복은 `200`이 아니라 `503`을 받아, 원본이 유실된 경우에도 재전송으로 복구할 수 있습니다. `valkeydedup`이 제공하는 backend는 이 계약을 구현하며, owner token을 검증하는 원자적 스크립트로만 키를 확정/삭제하므로 다른 요청의 예약을 지우지 않습니다.

##### legacy stateless 경로는 대등한 선택지가 아닙니다

`IsDuplicate`만 구현한 backend는 예약이 admission보다 먼저 확정되므로, **enqueue 실패 후의 정상 재전송이 중복으로 흡수되어 메시지가 유실됩니다(P1).** 이 경로는 소비자 backend가 아직 상태 계약을 구현하지 못한 동안만 남아 있는 잔여 경로이며, 제거를 전제로 유지됩니다.

- 이 경로로 기동하면 Handler가 `webhook is using a legacy stateless deduplicator ...`를 warn합니다. 기동 로그에 이 줄이 있으면 그 배포에는 P1이 그대로 살아 있습니다.
- `webhook.Deduplicator`는 message dedup 용도에 한해 `Deprecated:`입니다. HMAC nonce 저장소를 구현할 때는 `webhook.NonceStore`를 사용하십시오 — 그 역할은 사용 중단 대상이 아닙니다.
- 제거 조건: 모든 소비자 backend가 `webhook.StatefulDeduplicator`를 구현하고, `valkeydedup` backend의 `LegacyCommittedReads()`가 배포 후 0을 유지할 것. 두 조건이 충족되면 legacy 분기와 `DedupReleaser`, Lua의 `'1'` 드레인 코드를 함께 제거합니다.

  이 카운터를 읽으려면 backend 인스턴스를 붙들고 있어야 합니다. `valkeydedup.Option(client)`은 내부에서 `New(client)`를 인라인으로 만들고 버리므로 노출 경로가 없습니다. 대신 인스턴스를 직접 만들어 넘기십시오.

  ```go
  dedup := valkeydedup.New(valkeyClient)
  handler, err := iris.NewWebhookHandler(msgHandler, webhook.WithDeduplicator(dedup))
  // 진단 엔드포인트나 metric collector에서
  legacy := dedup.LegacyCommittedReads()
  ```

  카운터는 **인스턴스 로컬 `atomic.Uint64`이고 재시작 시 0으로 리셋**됩니다. 따라서 "배포 후 0" 하나만으로는 판정할 수 없습니다. 관측 창의 시작점은 프로세스 기동이 아니라 **마지막 구버전 라이브러리 인스턴스가 퇴역한 시점**입니다 — 롤링 배포 중에는 구버전이 계속 `"1"`을 심고 있고, 그 키가 재전송되지 않으면 아무도 읽지 않은 채 조용히 만료되므로 신버전 카운터가 0이어도 잔량이 없다는 뜻이 아닙니다. 판정 조건은 "**모든 구버전 인스턴스가 내려간 뒤**, 그 시점의 `DedupTTL`보다 오래 연속 가동한 프로세스에서 **모든 인스턴스가** 0"입니다. 어느 인스턴스든 재시작하면 그 인스턴스의 관측 창은 처음부터 다시 시작됩니다.

##### pending TTL 불변식

예약(pending)과 확정(committed)은 TTL을 분리해서 씁니다. 예약은 `WithDedupPendingTTL`(기본 `5s`), 확정은 `WithDedupTTL`(기본 `16m`)을 따르며, `Commit` 시점에 확정 TTL로 교체됩니다.

정상 경로에서 예약이 살아 있는 구간은 reserve~commit(기본 `DedupTimeout` 200ms 두 번 + `EnqueueTimeout` 50ms)뿐이고, pending TTL은 **예약 후 확정 전에 프로세스가 죽었을 때** 그 키가 묶여 있는 최대 시간입니다. 그동안 재전송은 `503`을 받으므로 다음 두 불변식이 성립해야 합니다.

```text
EnqueueTimeout + 2 × DedupTimeout < DedupPendingTTL < 발신자에게 남은 재시도 예산
```

앞쪽의 `2 ×`는 reserve와 commit이 각각 자기 `DedupTimeout` context를 받기 때문입니다(commit의 bounded 재시도는 그 하나의 context 안에서 끝납니다). 앞쪽이 깨지면 정상 요청의 예약이 in-flight 중에 만료되어 모든 `Commit`이 lost reservation이 되고, 뒤쪽이 깨지면 확정 전 프로세스 종료 시 재전송이 유실됩니다. 두 경우 모두 기동 시 warn이 남습니다.

- 앞쪽: `webhook enqueue timeout plus the reserve and commit dedup round trips is not shorter than the dedup pending TTL ...`
- 뒤쪽: `webhook dedup pending TTL is not shorter than the shortest wait before the sender's last retry ...`

뒤쪽 warn이 비교하는 것은 **첫 시도부터의 전체 지평이 아니라 남은 예산**입니다. 예약이 남는 시점은 프로세스가 죽은 그 attempt이므로, 그때부터 발신자가 포기할 때까지 남은 시간만이 예약이 만료될 기회입니다. 최악은 마지막 재시도 가능 attempt에서 죽는 경우로, 남은 것은 base `16s`에 `-20%` jitter가 걸린 대기 한 번, 즉 **12.8초**뿐입니다. 라이브러리는 이 값을 상수로 들고 비교합니다. 발신자의 실제 설정은 알 수 없으므로 `delivery.max_attempts`를 낮춘 배포에서는 이 warn이 없어도 예산을 넘을 수 있습니다(아래 참고). 두 warn 모두 **non-durable + `StatefulDeduplicator`** 조합에서만 나옵니다 — durable admitter 모드는 예약을 만들지 않으므로 해당하지 않습니다.

`DedupTTL`보다 큰 pending TTL은 `DedupTTL`로 clamp되며 그 사실도 warn으로 남습니다. clamp는 `DedupTTL`만 기준으로 하므로 clamp를 통과한 값이 여전히 남은 예산을 넘을 수 있습니다(`WithDedupPendingTTL(15*time.Second)`가 그런 경우이며, 이때는 뒤쪽 warn이 잡습니다).

발신자 지평은 Iris webhook worker 기준으로 `503`을 Retry로 분류해 `delivery.max_attempts`(기본 6) 회까지 backoff 1/2/4/8/16초로 재시도한 뒤 dead 처리하는 값입니다. backoff에는 ±20% jitter(`JITTER_PERMILLE = 200`)가 **각 대기마다 독립적으로** 붙으므로 전체 지평은 31초 고정이 아니라 **24.8~37.2초** 구간입니다. 다만 pending TTL이 비교해야 하는 것은 이 전체 지평이 아니라 위의 남은 예산 `12.8s`이고, 기본값 `5s`는 그보다도 충분히 짧습니다.

양방향으로 주의해야 합니다.

- **`DedupPendingTTL`을 늘릴 때:** 전체 지평이 아니라 **남은 예산(12.8초)**과 비교하십시오.
- **발신자의 `delivery.max_attempts`를 줄일 때:** Iris는 이 값에 `>= 1`만 요구하므로(`validation/positive.rs`) 운영자가 `2`로 낮추면 남은 예산이 약 0.8초로 줄어, 기본 `5s` pending TTL이 그것을 **역전**합니다. 이 경우 확정 전 프로세스 종료로 생긴 예약이 만료되기 전에 발신자가 먼저 포기합니다. 발신자 재시도 설정을 낮출 때는 `DedupPendingTTL`도 함께 낮추십시오.

현재 기본값 조합(pending `5s` vs 남은 예산 `12.8s`)은 안전합니다.

##### 확정 TTL은 발신자의 재전송 도달 시각을 덮어야 합니다

`DedupTTL`의 기본값이 `16m`인 것은 발신자의 **backoff 합이 아니라 절대 전송 상한**을 기준으로 잡았기 때문입니다. Iris는 attempt마다 응답을 `delivery.request_timeout_ms`(기본 `125s`)까지 기다리고, breaker `Deferred`도 `max_attempts`를 소비합니다. 각 attempt 사이의 최대 wait를 retry cap과 breaker cooldown 중 큰 값으로 잡으면 기본 profile의 상한은 `6 × 125s + 5 × 30s = 900s`입니다. 확정 TTL은 이 경계보다 엄격히 길어야 하므로 기본값은 60초 여유를 둔 `16m`입니다.

확정 TTL이 그보다 짧으면 늦게 도착한 재전송이 **이미 만료된 키**를 만나 같은 메시지를 다시 처리합니다. 응답 outbox의 UNIQUE 제약은 중복 *발송*은 막지만 처리 부작용(게임 상태 변경, 카운터 차감 등)은 막지 못하므로, 이 경로는 소비자 쪽에서 흡수되지 않습니다.

`WithDedupTTL`로 지정한 값이 이 전송 상한 이하이면 기동 시 warn이 남습니다.

```text
webhook dedup TTL is shorter than the arrival of the sender's last retransmission ...
```

값을 명시 설정한 배포에서 이 warn을 없애는 지점은 **이 라이브러리가 아니라 설정 경로**입니다. 실효값은 worker profile의 `receive.dedup_ttl_ms`에서 흘러오고, 기본값 `16m`의 소유자는 `shared-go/pkg/workerconfig`의 `Receive.DedupTTL`입니다. 소비자는 그 값을 그대로 `webhook.WithDedupTTL`로 넘기므로(`hololive-bot`의 `hololive-shared/pkg/config/settings/config_env_loaders.go`, `chat-bot-go-kakao`의 `internal/config/load_bot_webhook.go`), 기존 profile에 더 짧은 값을 명시한 배포는 이를 `900s`보다 크게 올려야 합니다. 이 경고도 **non-durable 경로에서 Noop이 아닌 dedup backend를 주입한 경우에만** 나옵니다 — durable admitter는 `handleDedupKey`를 거치지 않아 `DedupTTL`을 쓰지 않고, Noop backend는 키를 남기지 않습니다. 앞의 두 예약 관련 warn과 달리 legacy stateless backend에도 적용됩니다. `IsDuplicate`가 같은 `DedupTTL`로 키를 심기 때문입니다.

확정(`Commit`)이 일시적으로 실패하면 bounded 재시도 후에도 그 키는 pending으로 남습니다. 이 경우 메시지는 이미 처리되었지만 재전송은 예약 만료까지 `503`을 받으므로, 확정 실패 warn 로그(`dedupKeyHash` 포함)를 모니터링하십시오.

##### pending `503` 관측

확정 전 예약 때문에 되돌린 `503`은 중복(`ObserveDuplicate`)에도 enqueue 거절(`ObserveEnqueueFailure`)에도 잡히지 않습니다. `Commit`이 계속 실패해 모든 재전송이 `503`이 되는 상태는 이 값으로만 보이므로, 반드시 어느 한쪽으로 노출하십시오.

1. `handler.DedupPendingRejectedCount()` — 기존 `ReceiveDiagnostics`의 public struct/JSON shape를
   바꾸지 않는 additive accessor입니다. JSON endpoint에서는 반환값을
   `dedupPendingRejectedCount` 필드로 명시해 노출하십시오.
2. `webhook.DedupPendingObserver` — metric으로 바로 올리려면 `WithMetrics`로 주입하는 값에 `ObserveDedupPendingRejected()`를 추가하십시오. `Metrics` 인터페이스는 바뀌지 않으므로 기존 구현은 그대로 컴파일되고, 구현하지 않으면 호출되지 않습니다.

```go
type Metrics struct { /* 기존 webhook.Metrics 구현 */ }

func (m *Metrics) ObserveDedupPendingRejected() {
    m.dedupPendingRejected.Inc()
}

var _ webhook.DedupPendingObserver = (*Metrics)(nil)
```

##### 롤링 배포 주의

구버전과 신버전이 같은 Valkey를 공유하는 동안에는 두 방향의 비대칭이 있습니다.

- **정방향(구 → 신):** 구버전이 남긴 `"1"` 값을 신버전은 확정으로 읽어 기존과 같이 `200`으로 흡수합니다. 안전합니다. 이 해석은 별도 코드로 계상되며 `LegacyCommittedReads()`로 잔량을 관측할 수 있습니다.
- **역방향(신 → 구):** 신버전이 만든 pending 예약 키를 구버전은 `SET NX` 실패로만 인식해 **`200`으로 흡수**합니다. 즉 롤백 또는 혼재 구간에서는 원래의 재전송 유실(P1)이 그 키에 대해 다시 나타날 수 있습니다.

롤백 런북에는 pending 키 드레인 단계를 포함하십시오. 둘 중 하나면 충분합니다.

1. 구버전을 올리기 전에 `DedupPendingTTL`(기본 `5s`)만큼 대기해 모든 pending 예약을 자연 만료시킵니다. 확정된 키(`"c"`)는 남아도 구버전이 `SET NX` 실패로 읽어 기존과 같이 `200`으로 흡수하므로 문제되지 않습니다.
2. 즉시 롤백해야 하면 먼저 ingress와 모든 신버전 writer를 quiesce한 뒤 `iris:msg:*` 중 **값이 `p:`로 시작하는 키만** 삭제합니다. 확정 키(`"c"`)나 구버전 값(`"1"`)을 함께 지우면 이미 처리된 메시지의 재전송이 다시 처리됩니다. 아래 Lua는 `GET`과 조건부 `DEL`을 한 원자 연산으로 수행하므로, 별도 `GET` 뒤 상태가 바뀐 키를 지우는 TOCTOU 경로를 만들지 않습니다. `SCAN`은 key discovery만 담당하므로 writer quiesce를 생략해서는 안 됩니다.

```bash
# ingress와 신버전 writer를 먼저 quiesce한 상태에서 실행
valkey-cli --scan --pattern 'iris:msg:*' | while IFS= read -r key; do
  valkey-cli --raw EVAL \
    'local v=redis.call("GET",KEYS[1]); if v and string.sub(v,1,2)=="p:" then return redis.call("DEL",KEYS[1]) end; return 0' \
    1 "$key"
done
```

##### Valkey Lua 계약 통합 테스트

`internal/dedup`의 reserve/commit/release는 Lua 스크립트가 원자성과 소유권 검증을 소유하므로, 스크립트 본문은 실제 Valkey 인스턴스에 대해서만 검증할 수 있습니다. `make test`/`make test-race`는 이 경로를 실행하지 않고 skip하므로, 전용 타깃으로 돌립니다.

```bash
docker run --rm -d --name valkey-lua-test -p 127.0.0.1:6399:6379 \
  valkey/valkey@sha256:ee91f7a174ac4d6a6b0685b3a60e321f0a9dbbb691f9b0e285be2ba1d1be8328 # 9.1.1-alpine3.24
make test-valkey VALKEY_TEST_ADDR=127.0.0.1:6399
docker rm -f valkey-lua-test
```

`VALKEY_TEST_ADDR`가 비면 타깃이 즉시 실패하므로, 통합 테스트가 조용히 skip된 채 초록으로 끝나지 않습니다. CI에서는 `ci.yml`의 `dedup-contract` job이 valkey service 컨테이너를 띄워 같은 타깃을 실행합니다.

커버 항목은 reserve의 배타성과 token 기록, 같은 token 재전송의 self-idempotency, commit의 token 검증·확정 TTL 교체(상·하한), release의 compare-and-delete, foreign token 거부, 예약 만료 후 재예약 vs 늦게 도착한 commit 경계, 구버전 `"1"` 값의 확정 해석과 그 계상, 미상 값의 오류(fail-open) 처리입니다.

#### nonce cache와 message dedup 분리

HMAC replay 방지용 nonce cache와 message dedup은 키 공간이 겹치지 않는 별개의 역할입니다. nonce는 set-once fail-closed(저장 실패 시 요청 거부)로만 동작하며 상태 계약의 영향을 받지 않습니다. 이 역할의 계약 타입은 `webhook.NonceStore`이고, message dedup의 `webhook.Deduplicator`와 달리 사용 중단 대상이 아닙니다.

두 역할을 분리해 운영하려면 `webhook.WithNonceCache`로 nonce 저장소를 명시적으로 주입하십시오. 지정하지 않으면 Noop이 아닌 dedup backend가 nonce cache로 재사용되며, 이 암묵적 fallback은 호환을 위해 유지됩니다. 이때 backend의 `IsDuplicate`가 실제 set-once가 아니면 replay 보호가 조용히 fail-open되므로 Handler가 기동 시 warn합니다. backend가 set-once임을 스스로 보장한다면 `webhook.SetOnceNonceStore`(마커 메서드 `SetOnceNonce()`)를 구현해 이 warn을 없앨 수 있습니다 — `valkeydedup` backend는 `SET NX` 단일 왕복이므로 이미 구현하고 있습니다.

`WithNonceCache`의 함수 시그니처는 v1 함수 값 호환성을 위해
`func(webhook.Deduplicator) webhook.HandlerOption`으로 유지됩니다. `NonceStore`는 같은
`IsDuplicate` 메서드 집합을 가지므로 `NonceStore` interface 변수와 concrete backend도 그대로
인자로 전달할 수 있습니다.

`valkeydedup.Option`은 message dedup과 nonce cache를 같은 backend로 **명시적으로** 함께 설정합니다. 직접 option을 조립하는 소비자는 `WithDeduplicator`와 `WithNonceCache`를 항상 한 쌍으로 전달하십시오. 암묵적 fallback 제거 조건은 지원 중인 모든 소비자가 이 명시 경로로 배포되고, 한 major deprecation window 동안 fallback warning이 0을 유지하는 것입니다. 그 조건 전에는 외부 consumer 호환을 위해 fallback을 남기되 신규 stack 호출부에서는 사용하지 않습니다.

```go
handler, err := iris.NewWebhookHandler(inboxRuntime,
    webhook.WithDurableAdmission(inboxRuntime),
    webhook.WithAdmitTimeout(200 * time.Millisecond),
    webhook.WithWebhookToken("webhook-secret"),
)
```

웹훅 송신 테스트나 smoke 도구에서는 `X-Iris-Message-Id`를 먼저 설정한 뒤 공개 helper로
signature v2 header를 생성할 수 있습니다.

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

`WithAdmitTimeout`은 durable commit의 deadline입니다. **기본값은 `30s`이며 `0` 이하를 넘겨도 "무제한"이 아니라 이 기본값으로 정규화됩니다.** deadline이 끝나면 다른 admission 오류와 동일하게 HTTP `503 Service Unavailable`을 반환하므로 발신자가 재시도할 수 있습니다. 기본값을 발신자의 attempt timeout(`125s`)보다 훨씬 짧게 잡은 이유는, 저장소가 정체됐을 때 admission goroutine이 요청 context가 끊길 때까지 살아남아 종료(`Close`)까지 지연시키는 대신 빠르게 `503`으로 되돌리기 위해서입니다.

### 3. 관리 API (Admin APIs)

```go
cfg, err := c.GetConfig(ctx)
health, err := c.GetBridgeHealth(ctx)
rooms, err := c.GetRooms(ctx)
members, err := c.GetMembers(ctx, chatID)

// 설정 업데이트 예시
forwardUnmatched := true
_, err = c.UpdateConfig(ctx, "routes", iris.ConfigUpdateRequest{
    CommandRoutePrefixes: map[string][]string{"chatbot": []string{"!", "/"}},
    EventTypeRoutes:      map[string][]string{"events": []string{"member_nickname_updated"}},
    ForwardUnmatchedMessagesToDefault: &forwardUnmatched,
})

// HTTP/3 TLS 인증서 핫 리로드
_, err = c.ReloadH3Certificate(ctx) // POST /admin/cert-reload
```
* CAS(Compare-And-Swap) 제어가 필요한 경우 `ConfigUpdateRequest.ExpectedRevision`을 명시하여 설정 변경 시의 충돌을 방지할 수 있습니다.

### 4. SSE 이벤트 스트림 (Server-Sent Events)

```go
events, err := c.EventStream(ctx, 0)
for ev := range events {
    fmt.Printf("이벤트 타입: %s, 데이터: %s\n", ev.Event, ev.Data)
}
```

### 5. 조회 API (Query APIs)

```go
// 채팅방 요약 정보 조회
summary, err := c.QueryRoomSummary(ctx, chatID)

// 멤버 통계 조회
stats, err := c.QueryMemberStats(ctx, iris.QueryMemberStatsRequest{
    ChatID: chatID,
    Limit:  20,
})

// 최근 스레드 목록 조회
threads, err := c.QueryRecentThreads(ctx, chatID)

// 최근 메시지 내역 조회
msgs, err := c.QueryRecentMessages(ctx, iris.QueryRecentMessagesRequest{
    ChatID: chatID,
    Limit:  50,
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

### 6. BotClient 및 RebindingClient

다중 인프라 혹은 동적 환경을 지원하기 위해, 봇 서비스를 위한 최소 인터페이스인 `iris.BotClient` (`Sender` + `Ping` + `GetConfig`) 및 동적으로 Base URL을 핫스왑할 수 있는 `iris.RebindingClient`를 제공합니다.

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

---

## 클라이언트 설정 옵션 (Configuration)

```go
c, err := iris.NewClient(
    iris.WithBaseURL("https://iris-host:31001"), // 또는 IRIS_BASE_URL 환경변수 사용
    iris.WithBotToken("my-token"),              // 또는 IRIS_BOT_TOKEN 환경변수 사용
    iris.WithTimeout(5 * time.Second),
    iris.WithHMACSecret("shared-secret"),
    iris.WithLogger(slog.Default()),
    iris.WithReplyRetry(3),                     // 최초 요청을 포함한 최대 시도 횟수
    iris.WithTransport("h3"),                   // 또는 IRIS_TRANSPORT 환경변수 사용
    iris.WithH3CACertFile("/run/iris/h3-ca.crt"),
)
```

### 1. HTTP/3 전송 설정

Iris API의 기본 전송 프로토콜은 HTTP/3(QUIC)입니다. `IRIS_TRANSPORT` 환경 변수가 누락된 경우 기본적으로 `h3` 전송이 적용되며 이 경우 `https://` 스키마가 포함된 Base URL을 설정해야 합니다.

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

`IRIS_TRANSPORT=h3` 옵션은 `https://` 보안 연결에서만 활성화됩니다. `http3`, `http/3`, `quic` 문자열 역시 `h3`와 동일하게 인식합니다. 레거시 또는 로컬 테스트 목적으로 `http://` 일반 연결을 사용할 경우 `h2c` 전송을 명시해야 하며 유효하지 않은 프로토콜 형식 지정 시 에러가 반환됩니다.

운영 환경에서 H3 egress 대상을 Base URL host로 제한하려면 DNS allowset을 TTL마다 갱신하는 `WithH3DialGuardForBaseURL`을 사용할 수 있습니다. 만료 시 다른 dial은 stale allowset으로 즉시 판정하고 하나의 background refresh만 수행합니다. 초기 DNS 해석 실패는 기본적으로 오류를 반환하며 `WithH3DialGuardLenientInit`을 지정하면 deny-all 상태로 기동한 뒤 TTL 만료 시 자가회복합니다. 엉뚱한 host를 allowlist하지 않도록 `WithH3DialGuardForBaseURL`과 `WithBaseURL`에는 반드시 동일한 Base URL을 전달해야 합니다.

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

직접 정책을 구현해야 하는 경우 기존 `WithH3DialGuard` 또는 context 값을 받는 `WithH3DialGuardContext`를 사용할 수 있습니다. guard가 에러를 반환하면 연결은 시도되지 않고 `iris.IsH3EgressDenied(err)`로 분류할 수 있습니다.

### 2. 엔드포인트별 비밀키(Token) 분리 권장

보안 강화를 위해 모든 API 엔드포인트에 단일 토큰(`WithHMACSecret`)을 적용하는 대신, API 역할별로 전용 비밀 토큰을 지정할 수 있습니다.

```go
c, err := iris.NewClient(
    iris.WithBaseURL("http://localhost:3000"),
    iris.WithBotToken("shared-token"),               // 공유 폴백 키 (하위 호환 유지)
    iris.WithInboundSecret("config-signing-secret"),  // /config 전용
    iris.WithBotControlToken("bot-control-token"),    // /reply, /rooms 등 제어 API 전용
    iris.WithCertReloadToken("cert-reload-token"),    // /admin/cert-reload 전용
)
```

### 3. 웹훅 핸들러 설정 (Webhook Handler Configuration)

```go
import (
    "github.com/park285/iris-client-go/iris"
    "github.com/park285/iris-client-go/valkeydedup"
    "github.com/park285/iris-client-go/webhook"
)

handler, err := iris.NewWebhookHandler(msgHandler,
    webhook.WithWebhookToken("webhook-secret"),  // 또는 IRIS_WEBHOOK_TOKEN 환경변수 사용
    valkeydedup.Option(valkeyClient),            // Valkey 기반의 분산 중복 제거 필터
    webhook.WithDedupTTL(16 * time.Minute),
    webhook.WithWorkerCount(32),                 // Key-ordering 동시성 워커 개수
    webhook.WithQueueSize(2000),
    webhook.WithHandlerTimeout(30 * time.Second),
    webhook.WithMaxBodyBytes(1 << 20),           // 최대 요청 크기 (1MB)
    webhook.WithMetrics(myPrometheusAdapter),
    webhook.WithWebhookLogger(slog.Default()),
)
```

* 웹훅 메시지 스키마(`webhook.Message`/`webhook.MessageJSON`)와 핸들러 옵션(`webhook.WithXxx`)은 `webhook` 패키지에서 직접 import합니다. SDK 진입점인 `iris.NewWebhookHandler`(환경변수 해석·검증 포함)는 `iris` 패키지에 유지되며 Valkey 기반 중복 제거 필터는 `github.com/park285/iris-client-go/valkeydedup` 서브패키지(`valkeydedup.Option`/`valkeydedup.New`)로 분리되어 valkey-go를 쓰지 않는 소비자의 바이너리에 링크되지 않습니다.
* **메시지 순서 보장:** in-memory 모드에서는 기본적으로 동일한 채팅방 또는 동일 스레드 내의 메시지가 순차 처리됩니다. 자체적인 durable scheduler나 분산 큐가 순서를 소유하는 경우 `webhook.WithDurableAdmission`을 사용하거나 `webhook.WithOrderingMode(webhook.OrderingModeNone)`로 in-memory ordering을 끌 수 있습니다.

---

## 환경 변수 (Environment Variables)

| 환경 변수 | 설명 |
|------|------|
| `IRIS_BASE_URL` | Iris 백엔드 서버 Base URL |
| `IRIS_BOT_TOKEN` | 봇 호출 API 인증용 Bearer 토큰 |
| `IRIS_WEBHOOK_TOKEN` | 웹훅 유효성 검증용 인바운드 인증 토큰 |
| `IRIS_TRANSPORT` | 메시지 전송용 프로토콜 (`h3` [기본값], `h2c`, `http2`, `http1` 지원) |

* 코드 상에서 옵션 함수(`WithBaseURL` 등)로 주입된 값이 환경 변수로 로드된 값보다 항상 우선하여 적용됩니다.

---

## 라이브러리 구조 (Directory Layout)

```text
iris/              # SDK Facade - 외부 노출용 엔트리 포인트 (NewClient, NewWebhookHandler 등)
webhook/           # WebhookHandler, 메시지 스키마 정의 및 순차 스케줄러 큐
webhooksign/       # Webhook signature v2 요청 header 생성 helper
valkeydedup/       # Valkey 기반 메시지 중복 제거 public wrapper
internal/client/   # transport/signing/SSE/multipart/rebind/query/common 내부 구현
internal/dedup/    # Valkey 기반 메시지 중복 제거 구현체
```

---

## 라이선스 (License)

Apache License 2.0 — [LICENSE](LICENSE)
