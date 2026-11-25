# 변경 이력

출시 항목은 실제 Git tag를 기준으로 작성합니다. tag는 소비 저장소가 쓸 변경을 확정해 모듈을 게시할 때만 만들고,
그 사이 병합한 변경은 `미출시`에 누적합니다. 소비 저장소는 게시된 버전을 검증한 뒤 `go.mod`·`go.sum`을 갱신합니다. 단계 규칙은 `docs/release-provenance.md`가 게시를 허용한 모듈에만 적용하며, 스택 소비자를 같은 배포에서 함께 이전하는
변경은 minor로, 스택 밖 사용자가 직접 조치해야 하는 비호환 변경만 major(모듈 경로 전환)로 게시하며, 판단이 갈리면 낮은 단계를 고릅니다. 발행 PR에서는 다음 release 섹션 하나만 앞에 준비할 수 있으며,
release 섹션은 최신 순으로 배치합니다. 이 문서는 `미출시`와 최근 세 버전 구간만 유지하고 발행 PR에서는 준비하는 release 섹션을 그 첫 번째로 세며, 이전 기록은 이 파일의 Git 이력과 tag에서 확인합니다.

## 미출시

## v6.0.0 - 2026-11-25

- **major 전환:** 루트 모듈 경로를 `github.com/park285/iris-client-go/v6`으로 옮겼습니다. 아래의 `/v5/valkeydedup` 제거 때문에 같은 major로 게시할 수 없어서입니다. 소비자는 import와 require만 `/v6`으로 바꾸며 그 밖의 공개 API는 v5.2.1과 같습니다(`docs/MIGRATION-v6.0.0.md`). 릴리스 워크플로는 `v6.*` 태그를 게시합니다.
- **답장 실패 사유:** `ReplyStatusSnapshot.Reason`을 추가했습니다. Iris가 `failed` 상태에 `reason: "room_not_joined"`를 붙이면 활성 KakaoTalk 계정이 그 방에 없는 것이며, `iris.ReplyStatusRoomNotJoined(snapshot)`으로 판별해 같은 방으로 재발급하지 않습니다. 다른 실패에는 `Reason`이 없습니다.
- 의존성을 최신 안정판으로 올렸습니다. `valkey-io/valkey-go` v1.0.80(`adapters/valkeydedup`), `golang.org/x/net` v0.63.0, CI의 `actions/setup-go`·`actions/setup-python` v7.1.0, uv 0.13.9, govulncheck 핀 v1.10.0입니다. 공개 Go API는 바뀌지 않습니다.
- Go 린트에서 테스트를 포함해 `unsafe` import를 기본 금지합니다. Valkey nonce 저장소 테스트는 `valkey-go`의 공개 API로 명령 빌더와 오류 응답을 생성합니다.
- **Go 린트:** `make lint`는 golangci-lint 실행 파일 경로와 고정 버전을 스택 공통 `go-lint-matrix.py`에 전달합니다. 각 빌드 구성에서 루트 `.golangci.yml`로 모듈 전체를 검사하고, 테스트를 포함한 Git 추적 수기 Go 파일이 린터에서 누락되면 경로를 출력하고 실패합니다. `MODULES`로 검사 범위를 선택할 수 있으며, `make fmt`는 선택한 모듈의 현재 빌드 구성에 자동 수정을 적용합니다.
- **빌드와 검증:** 체크아웃의 빌드·race 테스트는 추적 모듈을 자동으로 찾고, 전체 CI와 pre-push는 공개 Valkey 어댑터의 소비자 계약을 검사합니다. 루트 `Makefile`이 포함된 소스 아카이브는 `make build`로, 배포된 어댑터 아카이브는 `adapters/valkeydedup`에서 `GOWORK=off go build ./...`로 빌드합니다. 린트와 자동 수정은 같은 고정 버전의 golangci-lint와 루트 설정을 사용합니다.
- 로컬·CI·릴리스 게이트는 pre-push에 고정한 `golangci-lint`와 `govulncheck`를 스택 공통 `resolve-go-tool.py`로 선택합니다. 사용자 캐시의 소유권·권한, SHA-256과 Go 빌드 정보를 검증하며, Go 모듈 캐시에서 오프라인으로도 빌드할 수 있습니다. Make는 도구 실행 파일 재정의를 거부하고 `GOWORK=off`를 강제합니다. 보안 워크플로는 Git이 추적하는 모든 Go 모듈을 검사합니다.
- `make fmt`는 선택한 경로가 모두 이 체크아웃의 추적 모듈인지 확인한 후 수정합니다. 루트 Makefile이 있는 소스 아카이브의 테스트와 tidy는 Bash 3.2에서 아카이브에 포함된 모듈을 탐색할 수 있습니다.
- **루트 SDK:** `/v5/valkeydedup` 경로와 Valkey 의존성을 소스에서 제거했습니다. 이 변경을 새 루트 v5 태그로 게시하지 않습니다. 기존 `v5.2.1` 태그와 그 소비자는 바뀌지 않습니다.
- 릴리스 게이트는 태그의 Git 체크아웃에서 선택한 모듈을 검사하고, pre-push는 전체 체크아웃을 검사합니다. 구형 manifest는 실제 발행된 태그와 커밋의 조합만 허용하며, 게시자와 소비자는 서명된 태그 객체 및 자산 네 개의 이름·SHA-256을 같은 코드로 대조합니다.
- **주석 금지:** 기계 지시문을 뺀 Go 주석과 doc 주석을 지웠습니다. godoc 설명은 더 이상 제공하지 않으며 계약은 이 README와 테스트가 소유합니다. pre-push gate가 `bash scripts/ci/verify-tools.sh scan`으로 강제합니다.
- **테스트:** 실제 시간을 기다리던 `time.Sleep` 2곳을 동기화로 바꾸고, webhook message ID·서명 버전 해석 fuzz 대상을 추가했습니다.
- 결정 기록 목록을 폐기했습니다. 과거 결정은 Git 이력에서 확인할 수 있습니다.
- 스택 PostgreSQL 코드 규칙을 golangci-lint forbidigo로 받습니다. `pgxpool`·`pgx`·`pgxdb.Querier`의 `Exec`·`Query`·`QueryRow`와 `pgx.Batch.Queue` 직접 호출, `pgx.BeginFunc`·`BeginTxFunc`, 트랜잭션 직접 `Rollback`, `pgxpool.New`·`NewWithConfig`를 거부해 SQL은 `pgxdb.Statement`로만 실행됩니다. 이 규칙이 스택 ast-grep 규칙 `go-postgres-conventions.yml`을 대신하므로 그 사본을 지웠습니다. 현재 위반은 없습니다.

## adapters/valkeydedup/v1.0.0 - 2026-11-19

- 기존 `v5.2.1`의 `webhook.WithNonceStore`에 독립 모듈 `github.com/park285/iris-client-go/adapters/valkeydedup`의 저장소를 전달합니다. nonce 키, TTL, 단일 `SET NX PX`와 저장 오류 처리는 그대로입니다.

## v5.2.1 - 2026-11-18

- 파일·이미지 multipart 요청은 전송 함수가 반환한 뒤 원본을 더 읽지 않습니다. 파일 source는 진행 중인 읽기 종료까지 기다리며, transport의 body 종료는 비차단으로 유지합니다.
- reply 상태 응답의 request ID가 요청과 다르면 영구 실패로 거절하며 자동 재시도하지 않습니다.
- SDK가 소유한 HTTP1 진단 연결은 `Close`에서 정리하고, 호출자가 제공한 HTTP client의 연결 소유권은 유지합니다.

- 공개 `iris` 패키지와 내부 전송 코드가 응답·쿼리·미디어 타입과 서명 헤더를 정의 패키지에서 직접 참조합니다. 공개 타입 이름과 메서드 시그니처, 멘션 ID의 숫자·문자열 JSON 표현을 유지합니다.
