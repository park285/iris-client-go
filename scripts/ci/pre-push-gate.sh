#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_PREFIX
export GOWORK=off

# 정확한 push 범위에서 문서만 바뀌었으면 Go 검증을 건너뛴다. 범위를 모르거나 full이면 전부 실행한다.
# docs/ 아래라도 .go·.sh·.sql은 실행 가능한 코드라 문서로 보지 않는다.
if [[ "${FULL_PRE_PUSH:-false}" != "true" && -n "${BASE_SHA:-}" && -n "${HEAD_SHA:-}" ]]; then
  changed_files="$(git diff --name-only "${BASE_SHA}..${HEAD_SHA}")"
  if [[ -n "${changed_files}" ]] \
    && ! grep -vE '^(docs/|.*\.md$)' <<<"${changed_files}" >/dev/null \
    && ! grep -qE '^docs/.*\.(go|sh|sql)$' <<<"${changed_files}"; then
    echo "[pre-push] docs-only change detected; skipping Go gate"
    exit 0
  fi
fi

for stage in lint test-race vulncheck tidy; do
  echo "[pre-push] make ${stage}"
  make "${stage}"
done
bash scripts/ci/check-otel-api-only.sh
echo "[pre-push] iris-client-go gate passed"
