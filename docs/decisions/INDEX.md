# iris-client-go 결정 카탈로그 색인

iris-stack의 `bash tools/checks/check-decision-catalog.sh render`가 생성하는 파일입니다. 직접 편집하지 말고 레코드를 고친 뒤 다시 생성하십시오. 규칙은 iris-stack의 `docs/agent-workflows/decisions/README.md`에 있고, 둘 이상의 저장소에 걸치는 결정은 그쪽 색인에 있습니다.

레코드 2건: proposed 0, accepted 1, rejected 0, withdrawn 0, superseded 1

| ID | 제목 | 결정 상태 | 이행 상태 | scope | 결정일 | 재검토 | 대체 관계 | 원본 |
|---|---|---|---|---|---|---|---|---|
| [DEC-20260825-iris-client-go-public-surface-major-only](records/DEC-20260825-iris-client-go-public-surface-major-only.json) | iris-client-go 공개 surface는 stack 내부 무소비만으로 제거하지 않고 coordinated major에서만 축소한다 | accepted | not_applicable | iris-client-go | 2026-08-25 | trigger | supersedes DEC-20260719-iris-client-go-unused-surface | - |
| [DEC-20260719-iris-client-go-unused-surface](records/DEC-20260719-iris-client-go-unused-surface.json) | iris-client-go의 무소비 공개 심볼을 deprecate 후 제거할지 유지할지 | superseded | unknown | iris-client-go | 2026-07-19 | - | superseded by DEC-20260825-iris-client-go-public-surface-major-only | [iris-stack: 2026-07-19-stack-code-audit-refactoring-proposal.md](../../../docs/agent-workflows/plans/2026-07-19-stack-code-audit-refactoring-proposal.md) |
