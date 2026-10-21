#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_PREFIX
export GOWORK=off

# 정확한 범위에서 Markdown만 바뀌면 코드 검증을 반복하지 않는다. docs/의 실행 파일도 제외하지 않는다.
if [[ "${FULL_PRE_PUSH:-false}" != "true" && -n "${BASE_SHA:-}" && -n "${HEAD_SHA:-}" ]]; then
  if git diff --quiet --no-ext-diff "${BASE_SHA}" "${HEAD_SHA}" -- . ':(exclude)*.md'; then
    echo "[pre-push] documentation-only change; skipping Go gate"
    exit 0
  else
    diff_status=$?
    [[ "${diff_status}" == 1 ]] || exit "${diff_status}"
  fi
fi

bash scripts/ci/release-gate.sh
echo "[pre-push] iris-client-go gate passed"
