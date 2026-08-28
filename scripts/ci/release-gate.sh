#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
. "${ROOT_DIR}/scripts/ci/python-runtime.sh"
repo_python_init
cd "$ROOT_DIR"
export GOWORK=off

bash scripts/ci/check-release-provenance.sh
make lint
make test
make test-race
make vulncheck
make tidy
