#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${ROOT_DIR}"

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
run_stage make vulncheck
run_stage make tidy

echo "════════════════════════════════════════"
echo "  iris-client-go pre-push full gate passed"
echo "════════════════════════════════════════"
