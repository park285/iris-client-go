#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
. "${SCRIPT_DIR}/python-runtime.sh"
repo_python_init
cd "${ROOT_DIR}"

unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_PREFIX
export GOWORK=off

# 정확한 push 범위에서 문서만 바뀌었으면 Go 검증을 건너뛴다. 범위를 모르거나 full이면 전부 실행한다.
if [[ "${FULL_PRE_PUSH:-false}" != "true" && -n "${BASE_SHA:-}" && -n "${HEAD_SHA:-}" ]]; then
  changed_files="$(git diff --name-only "${BASE_SHA}..${HEAD_SHA}")"
  if [[ -n "${changed_files}" ]] \
    && ! grep -vE '^(docs/|.*\.md$)' <<<"${changed_files}" >/dev/null; then
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
