# Cross-Repo Refactor — INDEX (2026-05-22)

iris-stack(4개 레포) 전범위 리팩토링의 통합 문서. 신규 4개 plan(A~D)을 묶고, 기존 hololive-bot/Iris in-flight plan을 parallel track으로 명시한다.

---

## 범위 결정 (사용자 확정)

| 축 | 선택 |
|---|---|
| 레포 | chat-bot-go-kakao, iris-client-go, hololive-bot (Go), Iris (Kotlin+Rust) |
| 종류 | 구조 정리, API 경계, 중복 제거, 성능/안정성 핫스팟 |
| 호환성 budget | **외부 contract 마이그레이션 OK** |
| Worktree | wave size ≥ 2일 때만 worktree |
| 실행 모드 | subagent-driven-development + per-task TDD + risk-gates |

## 정책 override (사용자 명시 결정 2026-05-22)

- 사용자님이 직접 작성한 `Iris/docs/refactoring-2026-05-followup.md` **Phase G**의 *"Go SDK는 v0.x major 변경 없음, internal 정리만 허용"* 정책은 본 INDEX의 Plan A에 의해 **명시적으로 override**됨.
- Plan A의 sentinel/typed error export는 additive(기존 import 호환)이나, 신규 export는 SDK semver minor bump 요구.
- 다른 모든 기존 plan(hololive Phase 2.A-C, Iris Phase A-F)은 **유지·존중**되며 superseded 아님.

---

## 신규 Plan (4개)

| Plan | 경로 | 범위 | Blocks | Blocked by | 상태 |
|---|---|---|---|---|---|
| **A** | [`iris-client-go/docs/2026-05-22-plan-a-error-export-and-march-residuals.md`](./2026-05-22-plan-a-error-export-and-march-residuals.md) | iris-client-go error sentinel + typed export, March P2.1 multipart, P2.6 transport 명확화 | Plan B | — | **Shipped** (v0.13.0, iris-client-go PR #4) |
| **B** | [`iris-client-go/docs/2026-05-22-plan-b-consumer-error-migration.md`](./2026-05-22-plan-b-consumer-error-migration.md) | cbgk + hololive에서 새 sentinel 채택, retry 분류 정밀화 | — | **Plan A** | **Shipped** (cbgk PR #5, hololive PR #130) |
| **C** | [`chat-bot-go-kakao/docs/agent-workflows/plans/2026-05-22-plan-c-retry-backoff-consolidation.md`](../../chat-bot-go-kakao/docs/agent-workflows/plans/2026-05-22-plan-c-retry-backoff-consolidation.md) | cbgk gemini + memoryqueue retry → shared-go/pkg/backoff 통합 | — | **Plan E** | **Shipped** (hololive PR #135 + tag `shared-go/v0.0.2`, cbgk PR #8 + PR #9 CI fix) |
| **E** | [`chat-bot-go-kakao/docs/agent-workflows/plans/2026-05-22-plan-e-shared-go-publish-path.md`](../../chat-bot-go-kakao/docs/agent-workflows/plans/2026-05-22-plan-e-shared-go-publish-path.md) | shared-go publish path 정의 (Plan C STOP의 follow-up) | Plan C 재개 | — | **Shipped** (hololive PR #132 + tag `shared-go/v0.0.1`, cbgk PR #7 verify-only CASE B) |
| **F** | [`chat-bot-go-kakao/docs/agent-workflows/plans/2026-05-22-plan-f-internal-app-logger-race.md`](../../chat-bot-go-kakao/docs/agent-workflows/plans/2026-05-22-plan-f-internal-app-logger-race.md) | internal/app logger pre-existing race fix (Plan D Phase 1 final validation에서 발견) | `go test ./... -race` 전체 통과 | — | **Shipped** (cbgk PR #4) |
| **G** | [`iris-client-go/docs/2026-05-22-plan-g-multipart-streaming.md`](./2026-05-22-plan-g-multipart-streaming.md) | multipart streaming alloc-safe 재시도 (Plan A Task 4 split) | — | **Plan A core merge** | **Shipped** (iris-client-go PR #7 + tag `v0.13.1`. allocs/op 102→70, B/op -99.87%) |
| **H** | [`hololive-bot/docs/agent-workflows/plans/2026-05-23-plan-h-dispatcher-retry-sentinel-queue.md`](../../hololive-bot/docs/agent-workflows/plans/2026-05-23-plan-h-dispatcher-retry-sentinel-queue.md) | hololive dispatcher queue가 iris sentinel을 인식해 permanent 실패 즉시 FAILED (Plan B Task 4 escalate) | — | Plan B merge | **Shipped** (hololive PR #133, **schema migration 불필요**) |
| **D** | [`chat-bot-go-kakao/docs/agent-workflows/plans/2026-05-22-plan-d-bot-package-decomposition.md`](../../chat-bot-go-kakao/docs/agent-workflows/plans/2026-05-22-plan-d-bot-package-decomposition.md) | internal/bot 188 파일 단일 패키지 → 7-phase 서브패키지 분해 | — | — | **Phase 1 Shipped** (cbgk PR #3). Phase 2~7 outline, 별도 wave 대기 |

## 기존 in-flight Plan (Parallel Track, NOT superseded)

| 위치 | 내용 | 진도 | 본 INDEX와의 관계 |
|---|---|---|---|
| [`hololive-bot/docs/agent-workflows/plans/2026-05-21-monorepo-refactor-master.md`](../../hololive-bot/docs/agent-workflows/plans/2026-05-21-monorepo-refactor-master.md) + 8 sub-plan + inventory | hololive monorepo 8-tree Phase 2.A→2.B→2.C | Phase 0 완료(`7c1d762`), Phase 1(plan) 완료, **Phase 2 실행 미개시** | 별도 트랙. Plan B에서 hololive-shared의 `dispatcher_send_flow.go`만 cross-touch. |
| [`Iris/docs/refactoring-2026-05.md`](../../Iris/docs/refactoring-2026-05.md) (Phase A-E) | Stability/structure/test/perf/tooling 5-phase | **거의 완료** (15개 merge commit + `53a7eb5a` 후속 fix) | 본 INDEX와 무관. 별도 진행. |
| [`Iris/docs/refactoring-2026-05-followup.md`](../../Iris/docs/refactoring-2026-05-followup.md) Phase F | iris-common H3 timeout F1, tools coverage F2, Bridge DI F3, cache_stats façade F4 | F3 진행 중(`7a3bfb73`), F1/F2/F4 미시작 | 별도 트랙. Plan A와 cross-touch 없음. |
| `Iris/docs/refactoring-2026-05-followup.md` **Phase G** | iris-client-go SDK 내부 정리 (public API 보존) | 미시작 | **Plan A에 의해 override·흡수**. Phase G의 G1(deps update), G2(diagnostics field), G3(naming/error wrapping)는 Plan A에서 더 큰 contract change와 함께 처리. |

---

## 실행 순서 (DAG)

```
Plan A (iris-client-go) ──────────────┐
                                       ├─► Plan B (consumer migration: cbgk + hololive)
                                       │
Plan D (cbgk bot package split, Phase 1) ┘  (병렬, 독립)


Plan E (shared-go publish path) ─► Plan C (cbgk retry consolidation, 재개)


[parallel tracks, 본 wave 영향 없음]
hololive Phase 2.A → 2.B → 2.C
Iris Phase F1/F2/F3/F4
```

- **Wave 1 (즉시 시작 가능, 병렬):** Plan A + Plan D Phase 1. Plan C는 STOP 발동으로 wave에서 제외.
- **Wave 2 (Plan A 완료 후):** Plan B.
- **별도 트랙:** Plan E → Plan C 재개 (옵션 결정 필요).

## Subagent 배치

| Plan | Worktree | Subagent 수 | 비고 |
|---|---|---|---|
| A | 1 (iris-client-go) | 1 worker + 1 reviewer | 단일 레포, 단계별 task. |
| B | 2 (cbgk + hololive) | 2 worker (parallel) + 1 reviewer per side | cross-repo, 두 work scope 분리. |
| C | 2 (cbgk + shared-go) | 2 worker (shared-go change → cbgk change 순차) | shared-go PR이 cbgk PR보다 선행. |
| D | 1 (cbgk, Phase 1만) | 1 worker + 1 reviewer | Phase 2~5 병렬은 별도 wave로 dispatch. |

`subagent-driven-development` 스킬 규약: 각 worker는 wave 내에서 disjoint write scope, dependency-class 명시(`blocking`/`background`). Phase 7(app.go)은 본 INDEX 범위 밖, 별도 plan 권장.

---

## Risk Gates (전체 매트릭스)

| Gate | Plans 영향 | Trigger | Mitigation |
|---|---|---|---|
| **API contract / schema** | A, B | sentinel + typed error export, consumer 채택 | additive surface, semver minor. 기존 internal type alias 1버전 유지 후 제거. |
| **Cross-repo dependency** | C | cbgk가 shared-go에 처음 의존 | go.mod replace 임시, 별도 publish path follow-up. CI 검증 필수. |
| **Behavior parity** | C | jitter 분포 변경 가능성 | empirical parity test ±5%, 실패 시 helper 재설계. |
| **Retry behavior change** | B | 401/400에서 retry 안 함으로 변경 | production metric 비교(retry count, 4xx surface), 1주 soak. |
| **Cyclic import** | D | 추출 서브패키지가 원본을 역import | interface seam(D Phase 1 Task 1.2). 발견 시 즉시 중단. |
| **Test 회귀** | A, B, C, D | 기존 test PASS 깨짐 | 각 plan의 Validation step에서 보장. revert ready. |
| **Concurrent in-flight work** | D | webhook-worker-decoupling 잔여(Task 6-3), 외 bug fix | worktree 격리 + 작업 전 main sync. |
| **hololive Phase 2 충돌** | B | Plan B가 `dispatcher_send_flow.go`를 수정. Phase 2.C.1(hololive-shared)도 같은 영역 | hololive Phase 2.C 시작 전에 Plan B 완료 또는 conflict 사전 조율. INDEX에서 추적. |
| **Performance** | A | io.Pipe streaming 전환 | throughput/latency 회귀 측정. 실패 시 P2.1 잔여만 별도 PR로 split. |
| **Transport behavior** | A | ForceAttemptHTTP2 분기 변경 | http1 모드 실사용 사전 확인. 사용처 없으면 안전. |
| **Public alias 누적** | D | facade re-export 영구화 | phase 종료 후 호출처 마이그레이션 + facade 제거 task. |

각 plan 문서 안에 plan-단독 risk gate가 별도 명시되어 있다. 본 매트릭스는 cross-plan 합산.

---

## Validation 전체

각 plan 자체 validation은 plan 문서 안. 본 INDEX의 통합 validation:

```bash
# Wave 1 (Plan A, C, D 완료 후)
cd /home/kapu/work/iris-stack/iris-client-go && go test ./... -race
cd /home/kapu/work/iris-stack/chat-bot-go-kakao && go test ./... -race && ./scripts/pre-commit-go-checks.sh
cd /home/kapu/work/iris-stack/hololive-bot/shared-go && go test ./... -race

# Wave 2 (Plan B 완료 후)
cd /home/kapu/work/iris-stack/chat-bot-go-kakao && go test ./... -race
cd /home/kapu/work/iris-stack/hololive-bot && ./build-all.sh --no-bump
```

## Stop Rules (전체)

- **Plan A 회귀:** Plan B 차단. Plan A 안정화 우선.
- **Plan C Task 1 결정이 (C) 별도 repo 분리:** Plan C 일시 중단, follow-up plan 작성 후 재개.
- **Plan D Phase 1에서 순환 의존 발견:** Plan D 전체 reconsider. interface seam 재설계.
- **hololive Phase 2.B와 Plan C 충돌(둘 다 backoff helper 추가):** 어느 한 쪽 hold + 조율.

## 다음 단계 (이 INDEX 작성 후)

1. ~~**risk-gates 스킬로 4개 plan에 정식 evidence 평가 통과.**~~ ✅ 완료 (2026-05-22). 10개 action item이 각 plan 문서에 병합됨.
2. **사용자 최종 승인.** plan 검토 후 commit/PR 시작.
3. **subagent-driven-development로 Wave 1 dispatch.** A, C, D 병렬.
4. **Wave 1 통합 후 Wave 2(Plan B) dispatch.**
5. **각 plan별 PR 분리.** A는 iris-client-go PR, B는 cbgk + hololive 2개 PR, C는 shared-go(hololive-bot) PR + cbgk PR, D는 phase별 PR.

## Deploy Ordering (risk-gate #4)

1. **iris-client-go v0.13.0 release tag** push (Plan A 산출, 사용자 결정 2026-05-22)
2. **chatbotgo (cbgk) PR merge & deploy** (Plan B + Plan C cbgk part)
3. **24시간 soak window**
4. **hololive PR merge & deploy** (Plan B + Plan C shared-go part)

단계 2와 3 사이 24시간은 retry 분류 변경이 cbgk에서 안정화되는지 확인 시간. 동시 배포는 디버깅 곤란.

## Operations Monitor Owner (risk-gate #5)

배포 후 1주 soak. monitor owner: **사용자님 본인** (결정 2026-05-22).

관측 metric:
- cbgk: `iris_reply_retry_attempts_total`, `iris_reply_4xx_surface_total`
- hololive: `delivery_failure_reason{reason=...}` cardinality 분포
- 비정상 시 24시간 내 rollback 결정.

---

## 변경 로그

| 일자 | 변경 |
|---|---|
| 2026-05-22 | 신규 작성. baseline 서베이 + 4-repo audit 기반. |
| 2026-05-22 | Wave 1 사전 검증 결과 반영: Plan C STOP(CI 단독 빌드 확정), Plan E follow-up 생성, iris-client-go v0.NEXT를 v0.13.0으로 확정, monitor owner를 사용자님 본인으로 지정, Plan D Phase 1만 우선 실행. |
| 2026-05-23 | Wave 1 DONE_WITH_CONCERNS 처리: Plan A Task 4(multipart streaming)를 Plan G로 split (B/op 99%↓ but allocs/op 28%↑가 risk-gate 위반), Plan D Phase 1에서 발견된 internal/app logger pre-existing race를 Plan F로 등록. |
| 2026-05-23 | Wave 1/2 후처리: 3 repo worktree + branch + filter-branch backup(`refs/original/`) 정리 완료, squash-merged remote branch 8개 삭제, Plan A/B/D Phase 1/F status를 Shipped로 갱신. Plan E 옵션 B(module path 정합화) 확정, Plan G/H wave 진행 시작, Plan D Phase 2~7과 hololive scraper budget fix는 후속 wave로 분리. |
| 2026-05-23 | Wave 2a/2b 완결: Plan G(iris-client-go PR #7 + tag `v0.13.1`, hybrid 전략으로 `allocs/op 102→70`, `B/op -99.87%`), Plan E(hololive PR #132 + tag `shared-go/v0.0.1`, cbgk PR #7 verify-only CASE B — PRIVATE repo 자격증명 이슈로 cbgk 실제 require lock은 Plan C 진행 시점), Plan H(hololive PR #133, **schema migration 불필요** — 기존 column만으로 sentinel→FAILED 즉시 전이). Plan C는 Ready to resume. |
| 2026-05-23 | Session 2 완결: Plan C Shipped(hololive PR #135 `shared-go/v0.0.2` half-jitter helper, cbgk PR #8 retry consolidation, cbgk PR #9 CI token auth + slices fix). 모든 신규 Plan(A~H) Shipped. 잔여: Plan D Phase 2~7, hololive monorepo Phase 2, Iris Phase F. |
