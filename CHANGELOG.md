# 변경 이력

이 문서는 실제 Git tag를 기준으로 작성합니다. 기존 상세 기록은 모두 보존해 한국어로
옮겼고, 기록이 없던 릴리즈는 해당 tag 범위의 commit으로 보완했습니다. 태그 전 변경은
`## 미출시

## v2.1.2 - 2026-08-21

`에 임시 기재한 뒤 다음 태그 섹션으로 이관합니다.

## 미출시

## v2.1.3 - 2026-08-21

- **변경**: webhook 재생 방지 창 검사의 정수 오버플로 fail-open을 닫았습니다. `now.Sub`이
  표현 범위를 넘으면 `MinInt64`로 클램프되고 2의 보수에서 `-MinInt64 == MinInt64`라 부호가
  뒤집히지 않아, 현재 시각보다 약 292년 이상 미래인 타임스탬프가 창 안으로 판정되었습니다.
  뺄셈이 개입하지 않는 경계 시각 비교로 바꿔 정상 범위의 판정 결과와 서브밀리초 정밀도는
  그대로 유지합니다. 타임스탬프가 서명 대상이라 외부 공격자 단독으로는 만들 수 없지만,
  발신 호스트의 시계가 튀면 nonce TTL 만료 이후 무제한 재생이 가능했습니다.

## v2.1.2 - 2026-08-21

- **변경**: 내부 `randomhex.Generate`가 `crypto/rand` 실패 시 시간·카운터 기반 값으로
  대체하던 도달 불가 분기(Go 1.24 이후 `rand.Read`는 오류를 반환하지 않음)를 제거했습니다.
  HMAC nonce·dedup 토큰·multipart boundary의 정상 경로 출력 형식은 그대로입니다.

## v2.1.1 - 2026-08-20

- **변경**: Go 1.26 workspace가 선택한 Valkey와 `x/*` 의존성 버전을 스택 소비자와
  정합화하고 사용하지 않는 간접 YAML 의존성을 제거했습니다.

## v2.1.0 - 2026-08-20

- **진단**: `ReceiveDiagnostics.SchedulerEnabled`를 추가하고 durable admission에서는 생성되지
  않은 in-memory scheduler의 worker·queue 값을 더 이상 합성하지 않습니다.
- **호환성**: non-durable scheduler 진단과 기존 admission/signature counter는 유지합니다.

- **변경**: `internal/client/rebind`의 transport type/function alias를 제거하고 owning package를
  직접 참조하도록 정리했습니다. 공개 `iris` package의 v2 facade alias는 소비자 계약으로 유지합니다.

## v2.0.1 - 2026-08-19

- **수정**: legacy removal release gate의 비선언 `rg` 의존성을 제거해 표준 GitHub runner에서
  tag release 검증과 provenance artifact 게시가 중단되지 않도록 합니다.

## v2.0.0 - 2026-08-19

- **호환성이 깨지는 변경**: Go module path를 `github.com/park285/iris-client-go/v2`로
  전환하고 모든 import를 새 major path로 이관합니다.
- **호환성이 깨지는 변경**: webhook receiver와 `webhooksign.SignRequest`를 signature v3-only로
  축소합니다. v2는 unknown version으로 거절하며 v2 helper, constant, verifier와 current vector를
  제거했습니다. 역사 vector는 `docs/archive/`에 보존합니다.
- **호환성이 깨지는 변경**: `Deduplicator`, `DedupReleaser`, `StatefulDeduplicator`,
  `WithDeduplicator`, `WithNonceCache`, `valkeydedup.New`, `valkeydedup.Option`을 제거합니다.
  `MessageDeduplicator`/`WithMessageDeduplicator`와 `NonceStore`/`WithNonceStore`를 역할별로
  명시하십시오.
- **보안**: 모든 webhook handler constructor는 explicit `SetOnceNonceStore`가 없으면 실패합니다.
  process-local default와 message backend 암묵 재사용을 제거했습니다.
- **정합성**: message reservation 오류를 dispatch 성공으로 바꾸지 않습니다. 요청은 `503`으로
  fail closed하며 반환된 owner token이 있을 때만 bounded conditional release를 수행합니다.

이관 절차는 [`docs/MIGRATION-v2.0.0.md`](docs/MIGRATION-v2.0.0.md)를 참조하십시오.

## v1.10.1 - 2026-08-18

- **변경**: Go module toolchain을 `1.26.6`으로 올려 `govulncheck`가 보고한
  `net/url`·`crypto/tls`·`encoding/asn1`·`net/http` 표준라이브러리 취약점을 해소합니다.

## v1.10.0 - 2026-08-18

- **추가**: 실제 request authority를 서명 범위에 결속하는 webhook signature v3 verifier를
  추가합니다. 수신 handler는 v2와 v3를 함께 검증하며 발신 helper는 명시적 cutover 전까지
  v2를 유지합니다.
- **추가**: authority-bound 발신을 명시적으로 선택하는 `webhooksign.SignRequestV3`를
  추가합니다. `req.Host`가 있으면 URL authority와 canonical parity를 검증하며 DNS case,
  명시 port, bracketed IPv6를 정규화합니다. 기존 `SignRequest`는 v2 동작을 유지한 채
  deprecated로 표시합니다.
- **추가**: `Handler.SignatureVersionDiagnostics()`가 v2/v3 HMAC compare 성공과
  unknown/malformed version 거절을 고정 네 counter로 노출합니다. 기존 `Metrics` interface와
  `ReceiveDiagnostics` shape는 변경하지 않습니다.
- **수정**: dedup reservation을 잃은 commit을 성공으로 오인하지 않고 명시적 오류로 보존합니다.
- **수정**: webhook signer가 hash한 bytes와 실제 transport가 보내는 bytes를 일치시켜 body
  mutation이 signature 검증 실패로 이어지는 경로를 차단합니다.
- **변경**: 기본 `PingStrategyAuto`가 legacy endpoint probing 없이 `GET /ready`만 사용합니다.

## v1.9.0 - 2026-08-13

- **추가**: 인증된 `/media/chunk` capability를 위한 `iris.MediaClient`와 strict
  `MediaChunkRequest`/`MediaChunkResponse` 계약을 추가합니다. KakaoTalk 26.7의 단일 이미지,
  묶음 이미지, `type=18` 미디어를 URL이나 로컬 경로 노출 없이 bounded chunk로 읽을 수 있습니다.
- **추가**: 채팅방 reaction을 추가·변경·삭제하는 `iris.ReactionClient`를 추가합니다.
  canonical ID, deterministic `requestId`, 응답 결속 검증을 클라이언트 경계에서 강제합니다.

## v1.7.0 - 2026-08-07

- **호환성이 깨지는 변경**: 스택 소비자가 0인 deprecated exported 심볼을 제거합니다.
  보장 대상 소비자(`hololive-bot`·`chat-bot-go-kakao`·`twentyq-bot`)는 셋 다 영향을 받지
  않으므로 major 승격 없이 제거합니다.
  - `iris.ReplyReissueSuffix`를 unexport합니다. 이 함수는 `ReissuedClientRequestID`의 내부
    suffix 포매터이며 세대 상한·ladder 중첩 차단·clientRequestId 검증을 소유하지 않습니다.
    재발급 id는 `iris.ReissuedClientRequestID`로만 만드십시오 — 세 소비자 모두 이미 그 경로만
    사용하며 `check-stack-reissue-contract.sh`가 이를 강제합니다.
  - `webhook.HeaderIrisToken`을 제거합니다. v0.33.0에서 예고한 token-only 인증 헤더 상수로,
    보류 사유였던 다운스트림 참조는 더 이상 존재하지 않습니다. 헤더 자체의 거부 동작은
    변하지 않았고 `hmac_verify_test.go`가 계속 검증합니다. 서명은
    `webhooksign.SignRequest`의 v2 헤더를 사용하십시오.

## v1.6.1 - 2026-08-06

- **수정(실결함)**: durable admission 경로(`NewDurableWebhookHandler`)가 큐 dispatch(`runTask`)를
  우회하면서 `webhook_handler_duration_seconds` 관측이 전혀 일어나지 않던 회귀를 고칩니다.
  admit 성공·실패 모두에서 admission(PG commit) 소요를 관측합니다. ChatBotGo는 2026-07-12
  durable 전환 이후 이 히스토그램과 연동 latency 알럿이 3주 이상 무신호였습니다.

## v1.6.0 - 2026-08-06

- **추가**: reply reissue ladder를 라이브러리로 승격 — 세대 상한, `:r` generation suffix,
  `CLIENT_REQUEST_ID_FAILED` 409 predicate를 단일 소유로 모읍니다.
- **구조**: dedup의 legacy `"1"` committed 판독 분기를 제거합니다.
- **구조**: iris-diag-exporter를 native Prometheus 노출로 전환해 외부 exporter를 제거하고,
  webhook DEAD 원인 metric을 추가합니다.
- **문서**: Iris per-attempt 재서명 전제와 legacy dedup 제거 조건의 API 결합을 명문화합니다.

## v1.5.0 - 2026-08-02

- **수정(실결함)**: dedup/nonce Valkey `SET NX`가 TTL을 초로 절사하는 `EX`를 사용해, 1초 미만
  TTL 설정이 `EX 0`이 되면 Valkey가 명령 자체를 거부해 해당 창의 모든 요청이 저장소 오류(503)로
  전락했습니다. `PX`(밀리초)로 전환하고 1ms floor를 둡니다.
- **수정**: H3 dial guard가 allowset TTL 경계에서 IP가 바뀔 때마다 요청 한 건을 거부로
  희생하던 것을 고칩니다. 허용되는 dial은 기존대로 stale allowset으로 즉시 통과하고, 만료로
  거부된 dial만 진행 중인 refresh 완료를 기다렸다 한 번 더 판정합니다. refresh는 항상 detached
  goroutine에서 돌아 panic 복구·context 분리 규칙이 경로 간에 갈라지지 않습니다.
- **수정**: SSE scanner의 라인 상한 초과(`bufio.ErrTooLong`)를 `ErrLineTooLarge`로 감싸 이벤트
  누적 상한(`ErrEventTooLarge`)과 구분합니다. 기존에는 전송 실패와 구분할 수 없었습니다.
- **수정**: HTTP/3 QUIC `HandshakeIdleTimeout`을 10초로 명시해 shared-go `pkg/h3` client 설정과
  정렬합니다(quic-go 기본 5초). 두 경로가 같은 overlay를 지나므로 같은 값을 씁니다.
- **추가**: `iris.ValidateClientRequestID`를 export해 소비자가 재구현하던 client request ID 검증
  규칙을 라이브러리 한 곳으로 모읍니다.
- **구조**: `HMACSigner`의 축자 복제 구현을 제거하고 `irishmac` 단일 소유(타입 별칭)로
  위임합니다. boundary gate가 검사하지 못하는 두 번째 구현이 생기는 경로를 없앴고,
  `check-hmac-boundary`가 이 계약을 검증하도록 확장했습니다.
- **구조**: multipart body factory가 envelope 검증을 해시 계산 전에 수행합니다. 서버가
  결정론적으로 거부할 페이로드를 최대 30MiB까지 해시하고 나서 실패하던 순서를 파일 경로와
  정렬했습니다. webhook `DedupTimeout`은 0 이하가 무제한이 아니라 기본값(200ms)으로 정규화됨을
  계약으로 명시하고 조건부 무-timeout 경로를 제거했습니다(정규화는 기존에도 양수를 보장했으므로
  동작 동일).
- **성능**: SSE 파서가 알려진 이벤트명을 인터닝해 이벤트당 할당을 줄이고, 64KiB를 넘는 누적
  버퍼는 재사용하지 않고 해제합니다. 성공/디코드 경로의 응답 body drain에 상한을 두어 응답
  크기에 비례한 무제한 읽기를 막습니다.
- **jsonx**: sonic 설정을 frozen config로 고정합니다 — `CopyString`(decode된 string이 해제된
  request 버퍼를 alias하지 않도록 복사)과 `ValidateString`(stdlib처럼 unescaped 제어문자 거부).

## v1.4.0 - 2026-08-01

- **동작 변경**: HMAC nonce 저장소의 **조회 실패**(오류·타임아웃)를 실제 replay와 분리해
  `401` 대신 `503`으로 되돌립니다. Iris webhook worker는 `401`을 `Dead`로 분류해 재전송하지
  않으므로(`webhook/retry.rs`), PostgreSQL 기반 nonce store가 `DedupTimeout`(기본 `200ms`)을
  넘기거나 오류를 내면 그 창에 도착한 inbound webhook이 영구 유실됐습니다. nonce 검사는
  durable admission보다 앞이라 inbox row도 남지 않습니다. Iris는 attempt마다 서명과 nonce를
  새로 생성하므로(`webhook/signing.rs`) 재전송 수용에 nonce 상태가 필요 없고, 이 경로는 서명
  검증을 통과한 요청만 도달하므로 secret을 모르는 발신자는 `503`을 유도할 수 없습니다.
- 실제 nonce 재사용은 기존과 같이 `401`이고, `nonceCache`가 없는 fail-closed 경로도 `401`을
  유지합니다. 저장소 실패는 더 이상 `Metrics.ObserveUnauthorized`에 계상되지 않으며, 관측
  지점은 기존 warn 로그(`webhook hmac nonce check failed ...`, `error` 필드 포함)입니다.
  `HandlerOptions`/`ReceiveDiagnostics`를 포함한 공개 표면은 바뀌지 않습니다.
- 위 저장소 실패 `503`을 계상하는 additive accessor `Handler.NonceStoreUnavailableCount()`를
  추가했습니다(`DedupPendingRejectedCount`와 같은 형태, 공개 struct shape 불변).

## v1.3.0 - 2026-08-01

- `iris.HTTPErrorCode(err)`와 공개 `HTTPErrorCodeClientRequestID*` 상수를 추가했습니다.
  검증된 structured error code는 비공개 wrapper가 보존하고 기존 `iris.HTTPError`의 공개
  4-field struct layout은 바꾸지 않으므로 v1 외부 unkeyed literal과 `errors.As`/`errors.Is`
  계약을 유지합니다. code는 최대 64 KiB로 제한한 raw error payload에서 먼저 추출하고,
  `HTTPError.Body`에는 기존처럼 512-byte redaction snippet만 남깁니다.
- webhook non-durable 모드에 token 기반 dedup 상태 계약 `webhook.StatefulDeduplicator`를
  추가했습니다. 예약(reserve)은 enqueue 성공 시에만 `Commit`으로 확정되고, 실패하면 owner
  token이 쥔 예약만 해제한 뒤 `503`을 반환하므로 정상 재전송이 중복으로 흡수되지 않습니다.
  선행 요청이 확정되기 전에 도착한 동시 중복은 `200` 대신 `503`을 받습니다.
  `IsDuplicate`만 구현한 기존 backend의 동작은 그대로 유지되지만, 이 stateless 경로는
  제거 예정 잔여 경로입니다. 해당 backend로 기동하면 Handler가 P1이 살아 있음을 warn하고,
  `webhook.Deduplicator`는 message dedup 용도에 한해 `Deprecated:`로 표기됩니다.
- **동작 변경**: `webhook.DefaultDedupTTL`이 `60s`에서 `16m`으로 올라갑니다. Iris는 모든
  attempt의 `delivery.request_timeout_ms`(기본 `125s`)와 attempt 사이의 최대 wait를 포함한
  절대 delivery horizon을 적용합니다. 기본 profile의 상한은
  `6 × 125s + 5 × max(30s, 30s) = 900s`이며 breaker `Deferred`도 attempt budget을
  소비합니다. 확정 TTL은 이 상한보다 엄격히 길어야 하므로 기본값은 `16m`입니다.
- 값을 명시 설정하는 stack consumer의 기본값도 `shared-go/pkg/workerconfig`에서 `16m`으로
  올립니다. 기존 profile에 더 짧은 `receive.dedup_ttl_ms`를 명시한 배포는 별도로 `900s`보다
  크게 올려야 합니다. `twentyq-bot`은 PostgreSQL dedup에 명시적인 `24h` committed TTL을
  사용합니다.
- `WithDedupTTL`로 지정한 값이 delivery horizon 이하이면 기동 시 warn합니다. durable
  admitter 배포와 Noop backend에는 나오지 않고(둘 다 `DedupTTL`로
  키를 남기지 않습니다), 예약 관련 warn 두 건과 달리 legacy stateless backend에도 적용됩니다
  — `IsDuplicate`가 같은 `DedupTTL`로 키를 심기 때문입니다.
- 예약(pending)과 확정(committed)의 TTL을 분리했습니다. 예약은 새 옵션
  `webhook.WithDedupPendingTTL`(기본 `5s`)을, 확정은 기존 `WithDedupTTL`을 따릅니다. 예약 후
  확정 전에 프로세스가 죽으면 그 키는 pending TTL 동안만 묶이므로,
  `EnqueueTimeout + 2 × DedupTimeout < DedupPendingTTL < 발신자에게 남은 재시도 예산`이
  성립하는 한 재전송이 유실되지 않습니다. `DedupTTL`을 넘는 값은 clamp하고 warn을 남깁니다.
  값은 `Handler` 내부에 저장해 기존 공개 `HandlerOptions`의 9-field layout과 unkeyed literal
  source compatibility를 유지합니다.
- `DedupPendingTTL`이 **발신자에게 남은 재시도 예산의 하한(12.8초)**을 넘으면 기동 시
  warn합니다. 이전에는 clamp 경고(`> DedupTTL`)와 in-flight 창 경고뿐이어서
  `WithDedupPendingTTL(45*time.Second)` 같은 조합이 조용히 통과했습니다. 비교 대상은 첫
  시도부터의 전체 지평(24.8~37.2초)이 아닙니다 — 예약이 남는 시점은 프로세스가 죽은 그
  attempt이므로, 최악인 마지막 재시도 가능 attempt에서는 base `16s`에 `-20%` jitter가 걸린
  대기 한 번(`12.8s`)만 남습니다. 발신자가 `delivery.max_attempts`를 낮춘 배포는 warn
  없이도 이 예산을 넘을 수 있습니다.
- in-flight 창 경고가 `DedupTimeout`을 두 번 셉니다. reserve와 commit이 각각 자기
  `DedupTimeout` context를 받으므로 실제 예산은 `EnqueueTimeout + 2 × DedupTimeout`인데
  이전 식은 한 번만 셌습니다. `DedupTimeout=2s, EnqueueTimeout=1s, DedupPendingTTL=4s`
  같은 조합이 경고 없이 통과한 뒤 모든 `Commit`이 `ErrDedupReservationLost`로 실패했습니다.
  기본값 조합(`50ms + 2 × 200ms` vs `5s`)에서는 결과가 바뀌지 않습니다.
- **동작 변경**: `WithAdmitTimeout`을 지정하지 않은 durable admission에 `30s` 기본 deadline이
  적용됩니다. 이전에는 기본값이 없어 유일한 상한이 요청 context였고, 발신자가 그것을 `125s`
  까지 유지하므로 저장소가 정체되면 admission goroutine이 그동안 살아남아 `Close`까지
  지연시켰습니다. `0` 이하를 넘겨도 "무제한"이 아니라 기본값으로 정규화되므로, 타임아웃을
  끄는 경로는 더 이상 없습니다.
- 예약 수명에 관한 두 warn(in-flight 창, 재시도 지평)이 durable admission 모드에서는 나오지
  않습니다. durable admitter는 message dedup 예약을 만들지 않으므로 존재하지 않는 창에 대한
  경고였습니다. `WithDurableAdmission`과 `valkeydedup.Option`을 함께 쓰는 구성이 영향을
  받습니다.
- 확정(commit) 실패 시 일시 오류는 bounded 재시도합니다. 이미 처리가 확정된 메시지의
  재전송이 거부(503)되는 방향으로 degrade하지 않도록 흡수(200) 쪽을 우선하며, 다른 owner가
  쥔 키(`ErrDedupReservationLost`)는 재시도하지 않고 그대로 둡니다.
- `valkeydedup` backend가 상태 계약을 구현합니다. 예약 값에 owner token을 담고
  commit/release는 token을 검증하는 원자적 스크립트로만 수행하므로, 다른 owner의 키를
  삭제하던 무조건 `DEL` 경로를 더 이상 사용하지 않습니다. 소유권을 검증하지 않는
  `Release(ctx, key)`는 호환을 위해 남아 있으나 deprecated이며 Handler는 호출하지 않습니다.
- reply 재시도 backoff 대기 중 context가 만료될 때, 직전 시도가 transport 오류였다면
  오류를 `ErrTransport` 계열로 감싸 반환합니다. `errors.Is(err, context.DeadlineExceeded)`와
  `errors.Is(err, context.Canceled)`는 그대로 성립하며, 소비자의 admission-lost 판정이
  context 오류로 대체되지 않습니다. 이 오류는 `ErrTransport`와 함께 `ErrRetryable`에도
  매칭됩니다 — ctx 만료가 겹치지 않은 동일한 transport 실패와 분류를 일치시키기 위한
  의도된 동작이며, 재시도 여부를 이 술어로 판단하는 소비자의 기존 경로와 호환됩니다.
- 롤링 배포 비대칭: 구버전이 남긴 `"1"` 값은 신버전이 확정으로 읽어 기존과 같이 `200`으로
  흡수하지만, 신버전이 만든 pending 예약 키를 **구버전은 `SET NX` 실패로만 인식해 `200`으로
  흡수**합니다. 롤백 또는 혼재 구간에서는 해당 키에 한해 기존 재전송 유실이 다시 나타날 수
  있으며, 이 창은 `DedupPendingTTL` 만료까지 유지됩니다.
- 확정 전 pending 중복이 받는 `503`을 `ObserveDuplicate`에서 분리해
  additive `Handler.DedupPendingRejectedCount()`로 계상합니다. 기존 `ReceiveDiagnostics`의
  7-field struct와 JSON shape는 유지되며, 소비자는 public JSON에
  `dedupPendingRejectedCount`를 명시적으로 추가할 수 있습니다. `Commit`이 지속 실패해 모든
  재전송이 `503`이 되는 상태는 이 카운터로만 관측됩니다.
- pending `503`을 metric으로 바로 올릴 수 있도록 선택적 마커 `webhook.DedupPendingObserver`
  (`ObserveDedupPendingRejected()`)를 추가했습니다. `WithMetrics`로 주입한 값이 이 메서드를
  가지면 Handler가 호출합니다. `Metrics` 인터페이스는 그대로이므로 기존 구현은 수정 없이
  컴파일되고, 구현하지 않으면 호출되지 않습니다. `Diagnostics()`를 배선하지 않은 소비자는
  이 마커가 유일한 노출 경로입니다.
- `valkeydedup` backend의 `LegacyCommittedReads()`를 읽으려면 인스턴스를 붙들어야 합니다.
  `valkeydedup.Option(client)`은 인스턴스를 반환하지 않으므로, 대신
  `valkeydedup.New(client)`로 만들어 `webhook.WithDeduplicator`에 넘기십시오. 이 카운터는
  인스턴스 로컬이고 재시작 시 0으로 리셋되므로 인스턴스별로 관측해야 합니다.
- HMAC nonce 저장소의 계약 타입을 `webhook.NonceStore`로 분리했습니다. message dedup의
  `webhook.Deduplicator`만 사용 중단 대상이며, nonce 역할은 유지됩니다. `WithNonceCache`는
  기존 exact 함수 타입 `func(Deduplicator) HandlerOption`을 유지하고, 같은 메서드 집합을 가진
  `NonceStore` interface 변수와 concrete backend도 그대로 받습니다. backend가 set-once임을
  선언하는 선택적 마커 `webhook.SetOnceNonceStore`를 추가했고, 이를 구현한 backend
  (`valkeydedup`)에는 암묵적 fallback warn을 내지 않습니다. v1.2.5 함수 값 대입과
  `HandlerOptions`/`ReceiveDiagnostics` unkeyed literal은 compile-time 회귀 테스트로 고정합니다.
- 암묵적 nonce fallback warn이 legacy stateless backend에도 나옵니다. 이전에는
  `StatefulDeduplicator`를 구현한 backend에만 검사해서, 계약이 가장 덜 알려진 조합
  (`IsDuplicate`만 구현 + `WithNonceCache` 미지정)이 경고 없이 nonce cache로 재사용됐습니다.
- `Reserve`가 오류를 반환할 때 예약이 저장소에 남았을 수 있으면 시도에 쓴 owner token을
  함께 반환하도록 계약을 명시했습니다. Handler는 fail-open으로 처리한 뒤 그 token으로
  `Commit`/`ReleaseReservation`을 시도해, 응답 유실로 생긴 고아 예약이 pending TTL 동안
  재전송을 `503`으로 막는 경로를 없앱니다. 빈 token은 **예약이 존재할 수 없음을 증명할 수
  있을 때만** 허용됩니다 — 저장소가 낸 오류는 예약 쓰기가 단일 왕복의 마지막 변경 단계인
  backend에서만 그 증명이 되고, 다단계 `Reserve`의 두 번째 왕복이 낸 오류는 이미 저장된
  예약을 부정하지 못합니다. `valkeydedup`은 `GET` 뒤 `SET` 하나로 끝나 이 조건을 만족하며,
  스크립트의 호출 형태 자체를 단위 테스트가 고정합니다. 이전에는 모든 오류에 token을
  돌려주어, 존재하지도 않는 예약에 `Commit` 왕복이 붙어 degraded 구간 지연이 `DedupTimeout`
  2회로 늘었습니다.
- 확정 실패 warn이 고아 예약(degraded reserve) 여부로 분기합니다. 예약이 애초에 없을 수
  있는 경우에 "예약이 pending으로 남아 재전송이 `503`으로 거부된다"고 단정하던 문구를
  분리했습니다 — 그 경우 재전송은 오히려 재처리됩니다.
- `valkeydedup`의 reserve 스크립트가 알려진 값만 상태로 매핑합니다. 구버전 `"1"`은 별도
  코드로 계상되어 `LegacyCommittedReads()`로 잔량을 관측할 수 있고, 알 수 없는 값은 확정이
  아니라 오류가 되어 호출자가 fail-open으로 처리합니다(기존에는 catch-all로 확정 처리되어
  메시지를 `200`으로 버릴 수 있었습니다). 같은 token의 재전송은 self-idempotent합니다.
- Valkey Lua 계약 통합 테스트를 `make test-valkey`와 CI `dedup-contract` job으로 실제
  실행합니다. 기존에는 `make test`/`make test-race`가 이 경로를 조용히 skip해 스크립트
  본문이 어떤 게이트에서도 평가되지 않았습니다. `dedup-contract` job의 valkey service
  이미지를 태그 대신 digest로 고정했습니다.

## v1.2.5 - 2026-07-30

- HMAC v2 검증을 통과한 webhook에서 `text`가 비어 있어도 non-empty `type`이 있으면
  type-only Kakao message로 수용합니다. 완전히 빈 payload 거부는 유지하며,
  `chatbotgo-observer` fan-out이 HTTP 400으로 DEAD 처리되던 회귀를 해소합니다.
- `iris-diag-exporter`의 certificate expiry metric을 Prometheus base-unit 규칙에 맞춰
  `iris_h3_cert_remaining_seconds`로 직접 노출하고 legacy `*_days` 이름은 방출하지 않습니다.

## v1.2.4 - 2026-07-29

- `v1.2.3`과 동일한 source commit에 release tag를 다시 발행해 실패한 최초 release publication을
  복구했습니다. library source 변경은 없습니다.

## v1.2.3 - 2026-07-29

- tag 기반 release에서 local full gate, SBOM, checksum manifest, keyless attestation과 immutable
  GitHub Release를 생성·검증하는 provenance 파이프라인을 추가했습니다.

## v1.2.2 - 2026-07-28

- HTTP/3 transport dependency `quic-go`를 `v0.61.0`으로 갱신합니다.
- local/repository security gate의 `govulncheck`를 `v1.6.0`으로 갱신합니다.

## v1.2.1 - 2026-07-28

- webhook attachment 문서를 Iris의 type 1/2/27/71 canonical reference context, bounded HTTPS URL 예외,
  durable/query/SSE redaction 계약과 일치시켰습니다. Go wire mapping 동작은 변경하지 않습니다.

## v1.2.0 - 2026-07-28

- signed request의 redirect를 origin과 무관하게 fail closed하고, 주입된 `http.Client`는
  shallow copy해 caller의 redirect policy나 timeout 값을 변경하지 않습니다. API 성공 범위는
  unary/signed/SSE 모두 `2xx`로 통일했습니다.
- SSE는 unary `http.Client.Timeout`을 response header와 오류 body에만 적용하고 정상 stream body는
  caller context가 소유합니다. `204 No Content`는 terminal stream으로 종료하며, 즉시 끝나는 빈
  `200` stream 재연결에는 exponential backoff를 유지합니다.
- numeric `Retry-After`가 `int64` 또는 `time.Duration` 범위를 넘을 때 짧은 fallback retry로
  overflow하지 않고 포화시킵니다.
- `iris.WithCertReloadToken`과 `iris.ErrCertReloadTokenRequired` facade를 복원해
  `ReloadH3Certificate` 전용 credential을 public entry point에서 설정할 수 있습니다.
- `webhook.MessageContext.StableMessageIdentity`의 반환 format은 아직 안정 계약이 아니며
  v1.x 내에서 변경될 수 있음을 godoc에 명시했습니다.
- 인증 통과 수단으로만 `webhook.HeaderIrisToken`을 참조하던 webhook 테스트를 현행 v2 서명
  경로로 이관했습니다. `HeaderIrisToken`은 다음 major에서 제거 예정이며, 토큰 헤더의 거부
  계약을 검증하는 하위호환 계약 테스트만 상수를 계속 참조합니다.
- webhook에 bounded routing context, semantic event schema version, durable-only handler constructor를
  추가하고 fallback message identity의 scope를 좁혔습니다.
- room event의 역방향 cursor 조회를 지원하고, deployment-specific Iris endpoint 기본값과 도달할 수
  없던 H3 insecure-skip 경로를 제거했습니다.

## v1.1.1 - 2026-07-22

### 수정

- `RebindingClient`가 typed-nil stale client를 cleanup 대상으로 예약하지 않도록 수정했습니다.

## v1.1.0 - 2026-07-21

### 추가

- 기존 `iris.Sender`를 변경하지 않는 additive `iris.FileSender` capability와
  `iris.ReplyFile`, `iris.NewReplyFile`, `iris.NewReplyFileBytes`를 추가했습니다.
- `H2CClient.SendFile`, regular file 경로용 `SendFilePath`와 `RebindingClient` forwarding을
  추가했습니다. 한 요청에는 1 byte 이상 30 MiB 이하의 파일 한 건만 허용합니다.

### 성능·자원 수명

- `io.ReaderAt`을 기반으로 파일 전체나 multipart body를 메모리에 복제하지 않고
  `multipart/form-data`를 스트리밍합니다. digest 계산은 내용을 지우고 반환하는 32 KiB
  재사용 buffer를 사용합니다.
- caller-owned `ReaderAt`은 SDK가 닫지 않으며 `SendFilePath`가 연 descriptor는 모든 반환
  경로에서 닫습니다. context 취소와 short·unstable source는 network 전송 전에 감지합니다.

### 정합성·재시도

- metadata manifest, multipart boundary, content length와 body SHA-256을 결정론적으로
  구성합니다. HTTP 429 또는 `clientRequestId`가 있는 transport 오류만 같은 source에서 body를
  다시 생성해 재시도하여 중복 전송 위험을 제한합니다.
- 파일명·MIME type·파일 크기, 31 MiB multipart 상한과 64 KiB metadata 상한을 client에서
  fail closed 검증합니다.

## v1.0.0 - 2026-07-21

첫 stable major 릴리스입니다. 스택 소비자(`chat-bot-go-kakao`, `twentyq-bot`,
`hololive-bot`)와 자체 테스트가 사용하지 않는 facade re-export 및 no-op webhook 옵션을
제거해 공개 표면을 축소했습니다. 모듈 경로는 v0→v1이라 `/v2` suffix가 필요 없습니다.

### 제거 (BREAKING)

- 무소비 facade 타입 re-export 25개 제거: `iris.SecretRole`, `iris.PingStrategy`,
  `iris.RoomStatsOptions`, `iris.ReplyMentionUserID`, `iris.ConfigState`,
  `iris.ConfigDiscoveredState`, `iris.ConfigPendingRestart`, `iris.BridgeHealthCheck`,
  `iris.BridgeDiscoveryHook`, `iris.BridgeDiagnosticsCapability`,
  `iris.BridgeDiagnosticsCapabilities`, `iris.KeyCacheStats`, `iris.RoomInfoResponse`,
  `iris.NoticeInfo`, `iris.BotCommandInfo`, `iris.OpenLinkInfo`, `iris.PeriodRange`,
  `iris.MemberActivityResponse`, `iris.QueryRoomSummaryRequest`,
  `iris.QueryRecentThreadsRequest`, `iris.ThreadListResponse`, `iris.ThreadSummary`,
  `iris.RawSSEEvent`, `iris.SSERoomEventBody`, `iris.SSEStreamState`.
- 무소비 facade 상수 re-export 26개 제거: `iris.SSEEventRoomEvent`,
  `iris.SSEEventStreamState`, `iris.StreamCursorStatusCurrent`,
  `iris.StreamCursorStatusStale`, `iris.StreamCursorStatusFuture`,
  `iris.StreamRecoveryQueryRecentMessages`, `iris.PathConfig`, `iris.PathDiagnosticsBridge`,
  `iris.PathRooms`, `iris.PathEventsStream`, `iris.PathQueryRoomSummary`,
  `iris.PathQueryMemberStats`, `iris.PathQueryRecentThreads`, `iris.PathQueryRecentMessages`,
  `iris.SecretRoleInbound`, `iris.SecretRoleBotControl`, `iris.SecretRoleCertReload`,
  `iris.PathDiagnosticsChatroom`, `iris.PathDiagnosticsNativeCore`,
  `iris.PathDiagnosticsRuntime`, `iris.PathDiagnosticsTextPing`,
  `iris.PathDiagnosticsChatroomOpen`, `iris.PathAdminCertReload`, `iris.PingStrategyAuto`,
  `iris.PingStrategyReady`, `iris.PingStrategyHealth`.
- 무소비 facade 옵션 re-export 12개 제거: `iris.WithTLSHandshakeTimeout`,
  `iris.WithMaxConnsPerHost`, `iris.WithReadIdleTimeout`, `iris.WithPingTimeout`,
  `iris.WithPingProbeTimeout`, `iris.WithPingStrategy`, `iris.WithWriteByteTimeout`,
  `iris.WithRoundTripper`, `iris.WithH3CACertReloadInterval`,
  `iris.WithH3InsecureSkipVerifyForTests`, `iris.WithAttachmentJSON`,
  `iris.WithCertReloadToken`.
- 무소비 facade 에러 re-export 2개 제거: `iris.PingError` 타입, `iris.ErrCertReloadTokenRequired`
  sentinel.
- v0.33.0에서 deprecated로 표시했던 no-op webhook dedup 모드 표면 제거:
  `webhook.WithDedupMode`, `webhook.DedupMode` 타입, `webhook.DedupModeBeforeDecode`·
  `webhook.DedupModeAfterDecode` 상수, `webhook.HandlerOptions.DedupMode` 필드. 중복 제거는
  항상 디코딩·검증·식별자 정합 이후에 수행되므로 모드 선택은 동작에 영향을 주지 않았습니다.

### 참고

- 핵심 송신·webhook·H3 dial guard API와 자체 테스트가 참조하는 관측/재익스포트 심볼은 유지합니다.
- Karing 계열(`iris.KaringClient` 등)은 `hololive-bot`이 소비 중이므로 이번 릴리스에서 제거하지
  않았습니다.
- `webhook.HeaderIrisToken`은 제거 예정으로 표시됐으나 다운스트림 테스트가 아직 참조 중이라 제거를
  보류합니다.

## v0.33.0 - 2026-07-20

### 추가

- reply 재시도와 `Retry-After` 적용, SSE 재연결 시도·실패·성공을 관측하는
  `iris.TransportMetrics`와 `iris.WithTransportMetrics`를 추가했습니다.
- H3 egress 대상을 Base URL host의 DNS allowset으로 제한하고 TTL 만료 시 stale-while-refresh로
  재해석하는 `iris.NewH3DialGuardForBaseURL`과 `iris.WithH3DialGuardForBaseURL`을 추가했습니다.
  기본 생성은 초기 DNS 실패를 반환하며 `iris.WithH3DialGuardLenientInit`으로 deny-all 상태에서
  기동한 뒤 자가회복하도록 선택할 수 있습니다.

### 변경

- 동작하지 않는 `webhook.DedupModeBeforeDecode`, `webhook.WithDedupMode`, token-only 인증용
  `webhook.HeaderIrisToken`을 다음 major release 제거 예정으로 deprecated 표시했습니다.

### 수정

- SSE 재연결 실패가 반복될 때 오류와 시도 횟수를 남기되 동일 오류 로그를 억제하도록 했습니다.

### 내부

- in-memory webhook nonce cache의 만료 entry 전체 sweep을 TTL 기반 간격으로 상각하고,
  조회 대상 entry의 만료는 sweep 시점과 무관하게 검사하도록 했습니다.
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
