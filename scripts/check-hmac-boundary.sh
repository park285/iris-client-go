#!/usr/bin/env bash
set -euo pipefail

ROOT="."
SCRIPT_PATH="${BASH_SOURCE[0]}"
case "${SCRIPT_PATH}" in
  */*) SCRIPT_DIR="${SCRIPT_PATH%/*}" ;;
  *) SCRIPT_DIR="." ;;
esac
SCRIPT_DIR="$(cd "${SCRIPT_DIR}" && pwd)"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root)
      if [[ $# -lt 2 ]]; then
        echo "usage: $0 [--root DIR]" >&2
        exit 2
      fi
      ROOT="$2"
      shift 2
      ;;
    -h|--help)
      echo "usage: $0 [--root DIR]"
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      echo "usage: $0 [--root DIR]" >&2
      exit 2
      ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  echo "required command not found: go" >&2
  exit 127
fi

ROOT="$(cd "${ROOT}" && pwd)"
exec env GOWORK=off GO111MODULE=off go run "${SCRIPT_DIR}/check-hmac-boundary.go" --root "${ROOT}"
