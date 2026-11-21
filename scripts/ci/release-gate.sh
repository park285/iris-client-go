#!/usr/bin/env bash
set -euo pipefail
unset CDPATH

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"
export GOWORK=off

make MODULES="${MODULES:-}" tidy lint test-allocations test-race vulncheck
