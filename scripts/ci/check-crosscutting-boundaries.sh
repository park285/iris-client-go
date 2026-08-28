#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
. "${SCRIPT_DIR}/python-runtime.sh"
repo_python_init
"${CI_PYTHON_BIN}" "${SCRIPT_DIR}/check-crosscutting-boundaries.py" --root "${ROOT_DIR}" --profile "iris-client-go" "$@"
