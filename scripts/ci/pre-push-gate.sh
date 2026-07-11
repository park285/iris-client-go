#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${ROOT_DIR}"

# pre-push가 주입한 Git 환경이 perf fixture의 저장소 탐색에 섞이지 않도록 격리한다.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_PREFIX

export GOWORK=off

echo "════════════════════════════════════════"
echo "  iris-client-go pre-push full gate"
echo "════════════════════════════════════════"

run_stage() {
  echo "[pre-push] $*"
  "$@"
}

run_stage make lint
run_stage make test
run_stage make test-race
run_stage make perf-gate
run_stage make vulncheck
run_stage make tidy
run_stage make check-boundaries

echo "════════════════════════════════════════"
echo "  iris-client-go pre-push full gate passed"
echo "════════════════════════════════════════"
