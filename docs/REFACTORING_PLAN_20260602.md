# iris-client-go 리팩토링 계획 (2026-06-02)

> cross-cutting 마스터: `iris-stack/docs/REFACTORING_PLAN_20260602.md`
> 범위: `internal/client`, `webhook`, `internal/dedup`, `internal/jsonx`, `iris` facade (~5K LOC)

## 0. 요약

소비자(cbgk 46파일, hololive 47파일)를 가진 잘 테스트된 라이브러리(테스트 ~9.7K LOC)입니다. `iris` facade → `internal/client`+`webhook` → `internal/dedup`/`jsonx` 레이어링이 깔끔하고 `internal/` 경계가 컴파일러로 강제됩니다. HMAC 계약은 vector 테스트(`hmac_contract_test.go`)로, wire 상수는 drift 테스트(`wire_constants_test.go`)로 고정돼 있습니다. 결함은 소수의 일관성/엣지 항목입니다.

## 1. 검증으로 폐기·하향된 1차 주장

| 주장 | 결과 | 근거 |
|---|---|---|
| postMultipart가 TE: chunked + Content-Length 동시설정 → wire 버그 | **폐기** | Go `net/http` `transfer.go`가 chunked 시 CL 제거. H3는 `req.ContentLength`로 known-length DATA frame 최적화. 정상. |
| `internalPool.StopAndWait` double-call 위험 | **폐기** | `stopOnce.Do`로 close 1회, 2회차 `workerWG.Wait()` 즉시 반환. RWMutex로 send-to-closed 방지. |
| `newSignedRequest` double sha256 | **하향 P3** | 동일 바이트 재해시(redundant)일 뿐 정확성 무관. |
| `WebhookMention.UnmarshalJSON`의 stdlib json 사용 | **하향 P3** | sonic outer + stdlib inner. 동작 정확, perf 일관성만. |

## 2. Findings

### [P1] ping 경로 `newRequest`가 `X-Iris-Body-Sha256` 미설정
- **증거**: `internal/client/client.go:625-641` (`newRequest`는 헤더 미설정) vs `client.go:576-603` (`newSignedRequest`는 설정). `ping.go:80`이 `newRequest` 사용.
- **문제**: probe(`OPTIONS /reply`, `GET /ready|/health`)가 HMAC 서명은 포함하나 body sha256 헤더 누락. 서버가 모든 서명 요청에 헤더를 강제하면 probe가 401.
- **수정**: 4개 HMAC 헤더 설정을 `setHMACHeaders(req, secret, method, path, bodyBytes)` 헬퍼로 추출, `newSignedRequest`/`newSignedStreamRequest`/`newRequest` 공용. (빈 body는 `sha256("")` = `e3b0c44298...`.)
- **Risk/Effort**: 낮음(additive)/Small. 테스트: probe 요청의 `X-Iris-Body-Sha256` 단언.
- **선행 확인(서버측)**: Iris가 GET/OPTIONS에서 이 헤더를 필수화하는지 — 필수면 현재 P1 버그, 아니면 P3 일관성.

### [P2] dedup가 body decode 이전 실행 → malformed 첫 전송이 dedup slot 소비
- **증거**: `webhook/handler.go:251`(auth) → `:261`(dedup `SET NX`) → `:270`(decode). `internal/dedup/valkey.go:26` `SET NX EX` atomic.
- **문제**: 새 message-id의 첫 전송 body가 깨지면 dedup 키가 등록된 뒤 400 → TTL(기본 60s) 내 정상 재시도가 duplicate로 drop. (Iris 서버가 malformed body를 보낼 때만 발생 — 가능성 낮으나 코드 보호 없음.)
- **수정안 B(비파괴)**: `Deduplicator`에 `Commit(ctx,key,ttl)` 추가, `IsDuplicate`는 `EXISTS`만; decode+validate 성공 후 `Commit`. 또는 수정안 C(문서화): 의도임을 주석화 + 테스트 추가.
- **Risk/Effort**: B=Medium(interface 변경), C=Small.

### [P2] `webhook.Handler`의 SDK 전용 shadow 필드
- **증거**: `webhook/handler.go:78-80` (`sdkToken`/`sdkLogger`/`sdkCtx`), `webhook/sdk.go:8-23`. `NewHandler` 직접 경로에서 이 필드는 읽히지 않고 `iris/sdk.go`의 `ResolveSDKConfig`만 사용.
- **문제**: `WithWebhookToken` 등이 `NewHandler`에 전달되면 무시되는 암묵 동작.
- **수정**: SDK 옵션을 별도 옵션 타입으로 분리하거나 `NewWebhookHandler`가 env를 직접 해석. (구조 명확화)
- **Risk/Effort**: 낮음/Medium.

### [P2] `ReplyMentionUserID = any` — 컴파일타임 타입 안정성 없음
- **증거**: `internal/client/types.go:29`. 현 소비자는 cbgk `irisreply/sender.go:57`(int64)만, hololive 미사용.
- **문제**: 미래 호출자가 잘못된 타입 전달 시 `MarshalJSON` 런타임 오류로만 발견.
- **수정**: `interface{ ~string | ~int64 }` sealed type. minor version bump.
- **Risk/Effort**: 낮음(현 노출 없음)/Large(하위호환).

### [P3] 기타
- `PingError.err` 미설정 private 필드(dead) — `errors.go:114`. 제거.
- `requestHasClientRequestID` 비망라 type switch — `client.go:553`. default=false는 안전(idempotency 보수적 비활성)하나 신규 타입 누락 위험. 메서드화 권장.
- SSE backoff cap이 의도(2s)와 달리 3.2s — `sse.go:45` (`if backoff < 2*time.Second`). `min(backoff*2, 2*time.Second)`로.
- `ErrRetryable`가 5xx를 retryable로 광고하나 `isRetryableError`는 429만 재시도 — `errors.go:44` vs `client.go:119`. 문서화 또는 일치.
- `WebhookMention.UnmarshalJSON` 내부 `jsonx` 통일(perf 일관성) — `types.go:75`.

## 3. Top refactors (ranked)
1. `setHMACHeaders` 헬퍼 추출 + ping 헤더 갭 해소(P1).
2. dedup-before-decode 문서화 또는 2-phase 인터페이스(P2).
3. SDK shadow 필드 정리(P2).
4. `PingError.err` 제거 + SSE backoff cap 수정(P3, 사소).
5. `ReplyMentionUserID` 타입 안정화(차기 minor).

## 4. 미해결(서버측 — iris-stack 범위 밖)
- GET/OPTIONS probe에서 `X-Iris-Body-Sha256` 필수 여부.
- Iris 서버측 `ClientRequestID` idempotency 처리 방식·시간창(alarm-worker 전달 정확성 의존).

## 5. Deep-read (opus 2차)
`client.go`(689), `hmac.go`+`hmac_test.go`+`hmac_contract_test.go`, `ping.go`, `sse.go`, `errors.go`, `transport.go`, `http3_transport.go`, `multipart_writer.go`, `handler.go`(804)+`handler_test.go`(1602), `scheduler.go`, `internal_pool.go`, `dedup/valkey.go`, `jsonx.go`, `iris/{iris,sdk}.go` 전량 정독. key-ordering scheduler/shard pool은 RWMutex+done-buffer 수정 이후 정상. `handler.go:519`에 핸들러 panic recover 존재(소비자 webhook 서버 보호).
