# 변경 이력

이 문서는 실제 Git tag를 기준으로 작성합니다. 기존 상세 기록은 모두 보존해 한국어로
옮겼고, 기록이 없던 릴리즈는 해당 tag 범위의 commit으로 보완했습니다.

## Unreleased

### 변경

- 동작하지 않는 `webhook.DedupModeBeforeDecode`, `webhook.WithDedupMode`, token-only 인증용
  `webhook.HeaderIrisToken`을 다음 major release 제거 예정으로 deprecated 표시했습니다.

### 수정

- SSE 재연결 실패가 반복될 때 오류와 시도 횟수를 남기되 동일 오류 로그를 억제하도록 했습니다.

### 내부

- 웹훅 핸들러의 사용되지 않는 token byte 사본을 제거했습니다.

## v0.32.0 - 2026-07-18

### 추가

- 웹훅 송신자가 `X-Iris-Message-Id`를 설정한 요청에 signature v2 header를 생성할 수 있도록
  `webhooksign.SignRequest(req *http.Request, secret string, body []byte) error`를 공개했습니다.

### 변경

- `internal/client`의 flat 구현을 transport, signing, SSE, multipart, rebind, query, common 책임
  경계로 재구성하고 공개 `iris` API alias와 동작을 유지했습니다.
- `webhook/handler.go`를 같은 `webhook` 패키지 안에서 options, validation, dispatch 책임 파일로
  분리했습니다.

## v0.31.0 - 2026-07-17

### 제거 (호환성 변경)

- deprecated no-op `webhook.WithRequireHMAC`를 제거했습니다. HMAC 검증은 항상
  필수이며 이 option은 v0.29.0부터 값을 무시해 왔습니다.

## v0.30.0 - 2026-07-17

### 추가

- webhook signature v2를 추가했습니다. `X-Iris-Signature-Version: v2`는 canonical HMAC에
  `X-Iris-Message-Id`를 결합하여 message identity 변경도 서명을 무효화합니다.
- `HeaderIrisSignatureVersion`, `SignatureVersionV2`를 공개하고 Iris와 byte-identical한 v2
  contract vector를 추가했습니다.

### 수정

- dedup 예약(SET-NX) 후 enqueue가 실패(queue full, shutdown)하면 예약을 best-effort로
  해제해 Iris 재시도가 duplicate로 흡수되어 메시지가 영구 유실되는 문제를 고쳤습니다.
  선택적 `webhook.DedupReleaser` 인터페이스를 추가했고 내장 `valkeydedup` 구현이 이를
  구현합니다. 기존 `Deduplicator` 계약은 그대로입니다.
- webhook 수신은 v2 signature만 허용하며 인증된 header identity만 body에 없는 message ID를
  보완할 수 있습니다. body와 header의 message ID 불일치, 중복 header, 길이·문자 집합
  위반은 fail-closed 처리합니다.
- benchmark evidence reader를 strict mode로 전환하고 fixture helper의 ShellCheck 경고를
  해소했습니다.

## v0.29.0 - 2026-07-12

### 추가

- `webhook.Handler`에 durable admission과 bounded shutdown을 추가했습니다. HTTP 200 전에
  `MessageAdmitter`가 durable store에 commit할 수 있고, 취소·panic·종료 경계에서도 승인된
  work의 ownership을 보존합니다.
- `webhook.Handler`에 HMAC 검증을 필수화했습니다. Iris 서명 header 네 개
  (`X-Iris-Timestamp`, `X-Iris-Nonce`, `X-Iris-Signature`, `X-Iris-Body-Sha256`)를 요구하고
  canonical request, body hash, replay window, nonce single-use를 포함한 HMAC-SHA256 전체
  검증을 수행합니다. token-only webhook은 거부합니다. 새 option은
  `webhook.WithWebhookSecret`, deprecated no-op `webhook.WithRequireHMAC`,
  `webhook.WithReplayWindow`, `webhook.WithNonceCache`입니다.
  - **호환성 변경:** consumer가 이 SDK 계약으로 올라가기 전에 Iris runtime이 서명된 outbound
    webhook을 보내야 합니다. `webhook.WithRequireHMAC(false)`는 source compatibility만 위해
    유지하며 token fallback을 다시 활성화하지 않습니다.
  - **downgrade 방지:** signature header가 하나라도 있는 request는 반드시 signed request로
    검증합니다. 유효한 token이 함께 있어도 일부만 있거나 잘못된 signature는 `401`을
    반환하며 token auth로 낮추지 않습니다.
  - **nonce store:** 기본적으로 process-local memory cache에서 nonce single-use를 추적합니다.
    `webhook.WithDeduplicator` backend를 설정하고 `WithNonceCache`를 지정하지 않으면 별도
    keyspace를 사용해 해당 backend를 nonce 저장소로 공유합니다. memory cache는 instance마다
    분리되고 restart 시 사라지므로 replica 사이에 replay protection을 공유하지 않으며 process
    restart 때 초기화됩니다. multi-instance 배포에서는 `WithNonceCache`로 shared external
    store를 주입해야 합니다. 외부 nonce store의 error나 timeout은 fail-closed `401`로
    처리합니다.

### 변경

- `webhook.NewHandler`의 ctx 취소는 더 이상 handler 실행에 전파되지 않습니다. 실행
  context는 ctx의 값만 보존하며(`context.WithoutCancel`), 취소는 `Close`/`CloseContext`로만
  발생합니다.
- `RebindingClientConfig.ResolveInterval`이 resolved Base URL 또는 resolver error snapshot의
  재사용 시간을 제어합니다. interval이 0이어도 concurrent refresh는 single-flight이며,
  refresh leader를 포함한 각 caller가 자신의 context cancellation으로 반환할 수 있습니다.
  공개 field 추가로 외부의 unkeyed `RebindingClientConfig` literal은 수정이 필요할 수 있으며
  keyed literal은 source-compatible합니다.

### CI

- benchmark baseline 검증과 fixture의 Git 환경 격리, Go 검증 도구·worktree 경계를
  강화했습니다.

## v0.28.0 - 2026-07-04

### 추가

- webhook HMAC dual-accept 검증을 추가했습니다. signature header가 있는 request는 HMAC을
  검증하고, rollout 기간의 unsigned request에는 기존 token 경로를 유지했습니다.
- payload schema parity test와 Iris SSOT signature vector 복사본을 추가했습니다.

### 수정

- `*NoopDeduplicator`도 nonce 공유 판단에서 올바르게 감지하고 partial signature,
  anti-downgrade, replay, body-hash 경계를 보강했습니다.

### CI

- 전체 local pre-push gate와 worktree-compatible benchmark gate를 추가했습니다.

## v0.27.0 - 2026-07-04

### 제거 (호환성 변경)

- stack 내부 consumer가 없던 public interface `iris.FullClient`, `iris.ClosableClient`,
  `iris.ClosableFullClient`, `iris.AdminClient`, `iris.CertReloadClient`, `iris.RoomClient`,
  `iris.RoomEventsByTypeClient`, `iris.RoomUserEventsByTypeClient`,
  `iris.LatestRoomUserEventsByTypeClient`, `iris.NicknameHistorySearchClient`,
  `iris.EventStreamClient`, `iris.QueryClient`와 내부 backing interface·assertion을
  제거했습니다. 지원 interface는 `iris.Client`, `iris.BotClient`, `iris.Sender`,
  `iris.KaringClient`입니다.
- typed runtime diagnostics decode API인 `iris.RuntimeDiagnostics`,
  `iris.RuntimeWorkersDiagnostics`, `iris.RuntimeWorkerDiagnostics`,
  `iris.IrisBotWebhookPipelineDiagnostics`, `iris.IrisWebhookDeliveryDiagnostics`,
  `iris.BotWebhookReceiveDiagnostics`, `iris.BotPoolExpectedDiagnostics`,
  `iris.IrisBotWebhookWorkerProfile`, `iris.IrisWebhookDeliveryWorkerProfile`,
  `iris.BotWebhookReceiveWorkerProfile`, `iris.BotPoolWorkerProfile`,
  `iris.IrisBotWebhookWorkerProfileValidation`, `iris.DecodeRuntimeDiagnostics`,
  `iris.DecodeIrisBotWebhookPipelineDiagnostics`,
  `iris.ErrRuntimeWorkerProfileDiagnosticsMissing`와 내부 typed decode helper,
  `H2CClient.GetIrisBotWebhookPipelineDiagnostics`를 제거했습니다. runtime diagnostics가 필요한
  consumer는 `GetRuntimeDiagnostics`를 호출하고 자신의 경계에서 raw JSON을 decode해야 합니다.
- `webhook.WithAutoWorkerCount`를 제거했습니다. 기본 worker count를 덮어쓸 때는
  `webhook.WithWorkerCount(n)`을 명시해야 합니다.

## v0.26.0 - 2026-07-03

### 제거 (호환성 변경)

- `iris.WithValkeyDedup`, `iris.NewValkeyDeduplicator`, `iris.ValkeyDeduplicator`를 `iris`
  package에서 제거했습니다. `iris`의 package-level import가 Valkey를 사용하지 않는
  twentyq-bot 같은 binary에도 `github.com/valkey-io/valkey-go`를 연결하던 문제를
  해소했습니다. Valkey deduplication API는 public subpackage
  `github.com/park285/iris-client-go/valkeydedup`으로 이동했습니다.
  - `iris.WithValkeyDedup(client)` → `valkeydedup.Option(client)`
  - `iris.NewValkeyDeduplicator(client)` → `valkeydedup.New(client)`
  - `iris.ValkeyDeduplicator` → `valkeydedup.Deduplicator`
  구현은 의도적으로 internal인 `internal/dedup`에 유지합니다. `valkeydedup`은 얇은 public
  wrapper이며 `New`는 `*valkeydedup.Deduplicator`를 반환하고 `Option`은
  `webhook.WithDeduplicator(New(client))`에 위임합니다.

## v0.25.0 - 2026-07-03

### 제거 (호환성 변경)

- `webhook.WithRequireHTTP2`, `webhook.HandlerOptions.RequireHTTP2`, handler의 HTTP/2-only
  protocol gate와 `505 HTTP Version Not Supported` 경로를 제거했습니다. 이 gate는 HTTP/3
  전환 전에 만들어져 활성화 시 H3 delivery(`ProtoMajor == 3`)를 거부했으며 stack consumer는
  사용하지 않았습니다. handler는 이제 server transport가 협상한 모든 HTTP version을
  허용합니다.
- legacy single shared-token fallback helper인 `iris.ResolveToken`, `iris.ResolveTokens`를
  제거했습니다. consumer는 `WithBotToken`과 `WithWebhookToken`으로 role별 token을
  주입해야 하며, stack 전체에 이 helper의 caller는 없었습니다.

### 변경 (호환성 변경)

- inbound-role request signing(`GetConfig`, `UpdateConfig`, 기타 `/config*` route)이 bot
  token으로 암묵적으로 fallback하지 않습니다. Iris server는 `/config*`를 inbound-role
  secret으로만 검증하므로 이전 fallback은 진단하기 어려운 `401`을 만들었습니다. 이제
  `WithInboundSecret` 또는 모든 route용 `WithHMACSecret`이 없으면 request 전송 전에 새
  sentinel `iris.ErrInboundSecretRequired`로 fail-closed합니다. webhook/reply만 사용하는
  bot-control client에는 영향이 없습니다.

## v0.24.0 - 2026-07-02

### 제거 (호환성 변경)

- public `webhook` package의 backward-compat `iris` facade re-export를 제거했습니다.
  `webhook`이 message schema, handler option, raw handler의 canonical import path입니다.
  consumer는 `github.com/park285/iris-client-go/webhook`을 직접 import하고 해당 `iris.*`
  symbol을 `webhook.*`으로 옮겨야 합니다.
  - type: `iris.Message`, `iris.MessageJSON`, `iris.WebhookHandler`, `iris.MessageHandler`,
    `iris.HandlerOption`, `iris.HandlerOptions`, `iris.WebhookRequest`, `iris.WebhookMention`,
    `iris.Metrics`, `iris.NoopMetrics`, `iris.Deduplicator`, `iris.NoopDeduplicator`,
    `iris.TaskPool`, `iris.WebhookOrderingMode`, `iris.WebhookReceiveDiagnostics`,
    `iris.WebhookSDKConfig`, `iris.WebhookDedupMode` → `webhook.Message`,
    `webhook.MessageJSON`, `webhook.Handler`, `webhook.MessageHandler`,
    `webhook.HandlerOption`, `webhook.HandlerOptions`, `webhook.WebhookRequest`,
    `webhook.WebhookMention`, `webhook.Metrics`, `webhook.NoopMetrics`,
    `webhook.Deduplicator`, `webhook.NoopDeduplicator`, `webhook.TaskPool`,
    `webhook.OrderingMode`, `webhook.ReceiveDiagnostics`, `webhook.SDKConfig`,
    `webhook.DedupMode`
  - constant: `iris.PathWebhook`, `iris.HeaderIrisToken`, `iris.HeaderIrisMessageID`,
    `iris.HeaderIrisRoute`, `iris.DefaultDedupTTL`, `iris.WebhookOrderingModeKey/None`,
    `iris.WebhookDedupModeBeforeDecode/AfterDecode` → 대응하는 `webhook.*` constant
    (`webhook.OrderingModeKey/None`, `webhook.DedupModeBeforeDecode/AfterDecode` 등)
  - function·variable: `iris.NewHandler`, `iris.WithWebhookOrderingMode`,
    `iris.WithDedupMode`, `iris.ResolveWebhookSDKConfig`와 webhook option re-export
    (`iris.WithWebhookToken`, `iris.WithWebhookLogger`, `iris.WithContext`,
    `iris.WithMetrics`, `iris.WithDeduplicator`, `iris.WithTaskPool`,
    `iris.WithWorkerCount`, `iris.WithQueueSize`, `iris.WithEnqueueTimeout`,
    `iris.WithHandlerTimeout`, `iris.WithRequireHTTP2`, `iris.WithDedupTTL`,
    `iris.WithDedupTimeout`, `iris.WithMaxBodyBytes`, `iris.WithAutoWorkerCount`,
    `iris.ResolveThreadID`, `iris.DedupKey`) → 대응하는 `webhook.*` symbol
    (`webhook.WithOrderingMode`, `webhook.WithDedupMode`, `webhook.NewHandler`,
    `webhook.ResolveSDKConfig`, `webhook.WithWebhookToken` 등)
- `KaringContentItem`의 alias였던 `KaringHololiveStream` type alias
  (`iris.KaringHololiveStream`, 내부 `client.KaringHololiveStream`)를 제거했습니다.
  `iris.KaringContentItem`을 사용해야 하며 `KaringHololiveRequest.Stream`/`.Streams`는 각각
  `*KaringContentItem`/`[]KaringContentItem`입니다.

### 참고

- `iris` package는 SDK entry point로 유지됩니다. `iris.NewClient`, env를 해석하고
  `webhook.HandlerOption`을 받는 `iris.NewWebhookHandler`, 당시의 Valkey dedup API, 모든
  `client` 기반 re-export는 유지했습니다. 실제 type은 compiler가 경계를 강제하는
  `internal/client`에 있고 HMAC signer는 비공개 file scope에 있으므로 `iris` alias는
  backward-compat shim이 아니라 유일한 public API입니다.

### 성능

- SSE event-stream parser를 처음부터 끝까지 `[]byte`로 처리하도록 바꿨습니다.
  `scanner.Bytes()`로 line을 소비하고 reusable buffer에 data line을 누적하며 event당 한 번만
  `bytes.Clone`으로 할당합니다. event ID는 sign·overflow 의미가 `strconv.ParseInt`와 같은
  zero-allocation `[]byte` parser로 처리합니다. 100-event room-event hot path는
  402→204 allocs/op, 18,522→10,689 B/op, 32,387→17,659 ns/op로 줄었습니다. allocation-budget
  test와 `perf-smoke` benchmark gate가 이 예산을 보호합니다.
- secret별 HMAC signer state를 `sync.Pool`의 `hash.Hash`로 pooling하여 request signing마다
  key schedule을 다시 계산하지 않게 했고 fallback retry backoff에 half-jitter
  (`[base/2, base]`)를 추가했습니다. `Retry-After`가 있으면 계속 우선합니다.
- 기본 `MaxConnsPerHost`를 32로 높였습니다.

### 수정

- signing 뒤 pooled HMAC hash를 항상 pool에 돌려주고 `Get` type assertion을 검사합니다.
  외부 값이 들어오면 panic 대신 새 HMAC state로 fallback합니다.

### 내부

- call별 signing helper `signIrisRequest`, `signIrisRequestWithBodySHA256`를 test-only code로
  옮겼습니다. production signing은 prebuilt secret별 signer cache만 사용합니다.
- lock 경로의 retry-after delay bound test를 추가했습니다.

### CI

- concurrency group과 `cancel-in-progress`, job timeout, full-SHA action pin을 적용하고 stack
  canonical `check-workflow-secrets` checker와 profile auto-detection을 채택했습니다.

## v0.23.0 - 2026-06-26

### 추가

- raw runtime diagnostics를 typed worker-profile 구조로 decode하는 helper와 diagnostics
  client API를 추가했습니다.

## v0.22.0 - 2026-06-23

### 변경

- certificate reload route에 전용 token을 필수로 요구하도록 인증 역할을 분리했습니다.

## v0.21.1 - 2026-06-22

### 수정

- diagnostics exporter의 빈 host 형식 `:port`를 non-loopback으로 분류하여 외부 노출을
  fail-closed 처리했습니다.

## v0.21.0 - 2026-06-22

### 변경

- dependency minor version을 갱신했습니다.

## v0.20.0 - 2026-06-21

### 변경 (호환성 변경)

- webhook deduplication을 decode 뒤 canonical body `messageId` 기준으로 옮겨 header spoof를
  차단했습니다.

### 보안

- cross-host redirect의 POST replay, 무제한 raw JSON·SSE·ping read, 큰 `EventPayload`, 빈 CA,
  공백 token constructor 우회를 차단했습니다.
- diagnostics exporter `/metrics`를 loopback bind와 bearer 인증으로 보호했습니다.

## v0.19.0 - 2026-06-20

### 추가

- 사용자 event 최신 조회 option, nickname exact-search method, 검색 결과 truncation signal을
  추가했습니다.

## v0.18.0 - 2026-06-18

### 추가

- pinned H3 CA file hot reload, webhook scheduler ordering mode, runtime diagnostics exporter,
  image MIME 지정과 profile refresh 조회를 추가했습니다.

### 변경

- `iris.go` god-file을 client, webhook, errors 파일로 분리하면서 1:1 alias를 보존했습니다.
- HMAC contract vector를 Iris SSOT 12개 case와 byte-identical하게 동기화하고 signer helper
  boundary와 benchmark regression gate를 추가했습니다.
- CI를 public fast gate와 local heavy gate로 분리하고 action pin, concurrency, timeout을
  강화했습니다.

### 수정

- 외부 `TaskPool` rejection 시 webhook scheduler `Close`가 멈추지 않도록 했고 인증·query·image
  admission과 malformed query 처리를 fail-closed로 강화했습니다.

## v0.17.0 - 2026-06-10

### 추가

- 최소 bot-consumer interface인 `iris.BotClient`(`Sender` + `Ping` + `GetConfig`)를
  추가했습니다.
- call마다 target을 resolve하고 URL이 같으면 cached client를 재사용하며 교체된 client를
  `StaleCloseGrace` 뒤 닫는 `iris.RebindingClient`와 `iris.NewRebindingClient`를
  추가했습니다.

### 수정

- raw GET/POST 경로(config, rooms, diagnostics, cert-reload)의 transport-init failure를
  non-retryable `TransportError{Op: "init"}`로 분류했습니다. 이전에는 `Op: "get"` 또는
  `Op: "post"`로 노출되어 `ErrRetryable`과 일치했습니다.
- canonical query가 엄격하게 percent-decode하고 literal plus와 flag parameter를 보존하도록
  signing 경로를 강화했습니다. malformed input은 fail-closed하여 signed target과 실제 전송
  target이 달라지지 않습니다. path segment에는 길이 상한과 safe-token charset을 적용했고,
  multipart image admission은 runtime limit에 맞췄으며 `crypto/rand` 실패 때 boundary와 nonce를
  deterministic fallback으로 생성합니다.
- 외부 `TaskPool`이 work를 거부할 때 `webhook.Handler.Close()`가 멈추던 문제를
  수정했습니다. `SubmitWait`가 false이면 in-flight key를 해제하여 dispatcher가 drain됩니다.

### 제거

- 내부 dead code인 `wrapHTTPError` identity wrapper와 legacy `newHTTPClient` constructor를
  제거하고 `PingError`의 이중 `Err`/`err` field를 공개 `Err` 하나로 합쳤습니다. public API는
  바뀌지 않았습니다.

### CI

- transport TLS와 webhook worker recovery baseline을 검사하는 cross-cutting boundary checker를
  fast gate에 연결했습니다.

## v0.16.2 - 2026-06-08

### 변경

- Go dependency를 갱신했습니다.

## v0.16.1 - 2026-06-08

### 성능

- facade function alias를 제거하여 call path를 단순화했습니다.

## v0.16.0 - 2026-06-08

### 추가

- Iris nickname ledger payload의 nullable `chatLogId`에 맞춰
  `MemberNicknameUpdatedEvent.ChatLogID`를 추가했습니다.
- typed SSE body `SSERoomEventBody`(`room_event` frame), `SSEStreamState`
  (`iris.stream_state` frame)를 추가했습니다.
- `EventTypeMemberNicknameUpdated`, `SSEEventRoomEvent`, `SSEEventStreamState`,
  `StreamCursorStatusCurrent/Stale/Future`, `StreamRecoveryQueryRecentMessages` contract
  constant를 추가했습니다.
- Iris가 항상 설정하는 webhook delivery header용 `webhook.HeaderIrisRoute`
  (`X-Iris-Route`)를 추가했습니다.
- malformed webhook body가 dedup key를 소비하기 전에 거부해야 하는 consumer를 위해
  `WebhookDedupModeAfterDecode`를 지원하는 `iris.WithDedupMode`를 추가했습니다.

### 수정

- `ConfigDiscoveredState.BotID`가 Iris가 직렬화하는 `botId`를 decode하도록 고쳤습니다.
  이전 `bot_id` tag는 항상 0을 만들었습니다.
- `KaringDryRunResponse`가 live `202` camelCase response의 `receiverName`, `templateId`,
  `itemCount`, `streamCount`를 decode하도록 했습니다. 이전에는 live mode에서 이 field가
  조용히 버려졌습니다.
- `Retry-After`를 `HTTPError.RetryAfter`로 보존하고 bounded reply retry delay에 사용했습니다.
- `field:value` frame의 SSE parsing을 보강하고 scanner token을 1MiB로 제한하며 scanner
  error를 stream logger에 전달했습니다.
- diagnostic snippet 뒤 error response body drain을 bounded 처리하고 HMAC·scheduler hot path의
  불필요한 allocation을 제거했습니다.

### 제거

- retired room event struct alias를 `iris` facade에서 제거했습니다.
  `member_nickname_updated`만 semantic event 계약으로 유지합니다.
- Iris가 `createdAtMs`만 직렬화하므로 `RoomEventRecord.CreatedAt`을 제거했습니다.

### 문서

- `docs/webhook-type-attachment.md`를 현재 Iris 계약에 맞췄습니다. attachment는 opt-in이며
  allowlist로 sanitize한 metadata JSON이고 URL, path, raw blob은 포함하지 않습니다. retired
  event subtype은 제거했습니다.

## v0.15.4 - 2026-06-04

### 추가

- room event type filter API, admin config route, certificate reload API를 추가했습니다.

## v0.15.3 - 2026-06-03

### 변경

- toolchain 하한을 `go1.26.4`로 명시했습니다.

## v0.15.2 - 2026-06-03

### 수정

- `newRequest`의 HMAC body-hash signing을 server 계약과 맞추고 local lint gate를
  강화했습니다.

### 변경

- shared-go와 맞추기 위해 toolchain pin을 제거했다가 patch 하한을 별도 release에서
  고정했습니다.

## v0.15.1 - 2026-05-25

### 추가

- webhook receive diagnostics를 공개했습니다.

## v0.15.0 - 2026-05-25

### 추가

- `TaskPool` interface와 `WithTaskPool` option을 추가했습니다.

### 수정

- webhook completion channel buffer를 조정하여 shutdown deadlock을 해소했습니다.

## v0.14.0 - 2026-05-24

### 수정

- `errcheck` 위반을 해소하고 deduplication test coverage를 100%로 높였습니다.

## v0.13.1 - 2026-05-23

### 변경

- retry-safe body reconstruction을 유지하면서 `SendImage`, `SendMultipleImages` multipart
  upload가 image payload 전체를 memory에 buffering하지 않고 stream하도록 바꿨습니다.

## v0.13.0 - 2026-05-23

### 추가

- 공개 sentinel error `ErrRetryable`, `ErrPermanent`, `ErrAuthFailed`, `ErrRateLimited`,
  `ErrTransport`를 추가했습니다.
- typed error `HTTPError`, `TransportError`, `PingError`를 추가했습니다.

### 변경

- 명시적 HTTP/2 mode에서만 `ForceAttemptHTTP2`를 활성화하고 명시적 HTTP/1.1 mode에서는
  비활성 상태를 유지하도록 transport 선택을 변경했습니다.
- 내부 error type을 공개 type으로 교체하면서 한 version 동안 compatibility alias를
  유지했습니다.

### 참고

- 새 public API symbol을 보존하기 위해 이 release에서는 Phase G의 "public API symbol 유지"
  정책을 명시적으로 재정의했습니다.
- multipart streaming(P2.1)은 후속 Plan G로 분리되어 v0.13.1에 배포했습니다.
  `docs/2026-05-22-plan-g-multipart-streaming.md`를 참고하십시오.

## v0.12.5 - 2026-05-16

### 추가

- Karing content-list SDK를 추가했습니다.

## v0.12.4 - 2026-05-11

### 수정

- HTTP/3 initial packet이 QUIC minimum packet size 안에 유지되도록 했습니다.

## v0.12.3 - 2026-05-07

### 추가

- text reply에서 mention user ID를 전달할 수 있게 했습니다.

## v0.12.2 - 2026-05-06

### 추가

- Iris reply mention API를 추가했습니다.

## v0.12.1 - 2026-05-05

### 수정

- recent-message query에서 잘못된 thread ID를 request 전에 거부합니다.

## v0.12.0 - 2026-05-05

### 수정

- recent-message API의 thread ID 계약을 Iris server와 맞췄습니다.

## v0.11.4 - 2026-05-05

### 추가

- accepted text reply response와 recent-message sequence ID를 공개했습니다.

## v0.11.3 - 2026-05-02

### 추가

- open-link profile image field를 추가했습니다.

### 수정

- multipart reply signing이 전체 body hash를 사용하도록 고쳤습니다.

## v0.11.2 - 2026-04-26

### 변경

- Go module directive를 `1.26.2`로 갱신했습니다.

## v0.11.1 - 2026-04-07

### 수정

- webhook event payload metadata를 손실 없이 보존했습니다.

### 문서

- v0.11 migration guide를 추가했습니다.

## v0.11.0 - 2026-04-02

### 추가

- typed query·room API, webhook SDK helper, typed facade, SSE event envelope을 추가했습니다.
- route role별 인증을 분리하고 split-auth contract test를 추가했습니다.

### 제거 (호환성 변경)

- raw query·decrypt type과 preset compatibility layer를 제거하고 webhook payload의
  `senderRole`을 삭제했습니다.

## v0.10.1 - 2026-04-01

### 수정

- protected request에 body-hash header를 포함해 signing하도록 했습니다.

## v0.10.0 - 2026-03-31

### 추가

- multipart metadata에 image manifest를 추가했습니다.

## v0.9.0 - 2026-03-30

### 변경

- image send method가 `ReplyAcceptedResponse`를 반환하도록 했습니다.

## v0.8.0 - 2026-03-30

### 변경

- image reply를 Base64 형태 대신 binary `multipart/form-data`로 전송하도록 전환했습니다.

## v0.7.0 - 2026-03-28

### 문서

- README를 당시 API에 맞추고 stale 문서를 제거했습니다.

## v0.6.0 - 2026-03-24

### 추가

- `SendImage`에 `SendOption`을 지원했습니다.

## v0.5.0 - 2026-03-23

### 문서

- README 예제를 SDK constructor API에 맞췄습니다.

## v0.4.2 - 2026-03-22

### 추가

- bot consumer용 `iris` wrapper package를 추가했습니다.

## v0.4.1 - 2026-03-22

### 변경

- local agent artifact를 Git 추적에서 제외했습니다.

## v0.4.0 - 2026-03-21

### 수정

- webhook에서 실제로 관찰된 thread ID만 유지하도록 했습니다.

## v0.3.0 - 2026-03-20

### 변경

- 생성형 문서 artifact를 repository 추적에서 제거했습니다.

## v0.2.0 - 2026-03-20

### 문서

- 새 Go module path에 맞춰 project 문서를 갱신했습니다.

## v0.1.0 - 2026-03-20

### 추가

- 통합 Iris Go client library를 처음 공개했습니다.
