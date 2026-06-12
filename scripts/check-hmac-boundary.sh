#!/usr/bin/env bash
set -euo pipefail

ROOT="."

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

if ! command -v rg >/dev/null 2>&1; then
  echo "required command not found: rg" >&2
  exit 127
fi

ROOT="$(cd "${ROOT}" && pwd)"
cd "${ROOT}"

status=0
sign_matches="$(rg -n 'func[[:space:]]+(\([^)]*\)[[:space:]]*)?signIrisRequest[[:space:]]*\(' --glob '*.go' --glob '!**/*_test.go' || true)"

if [[ -n "${sign_matches}" ]]; then
  echo "signIrisRequest must remain test-only; offending production definitions:" >&2
  printf '%s\n' "${sign_matches}" >&2
  status=1
fi

signer_matches="$(rg -n 'newHMACSigner[[:space:]]*\(' --glob '*.go' --glob '!**/*_test.go' || true)"
signer_calls=""
if [[ -n "${signer_matches}" ]]; then
  signer_calls="$(printf '%s\n' "${signer_matches}" | rg -v 'func[[:space:]]+newHMACSigner[[:space:]]*\(' || true)"
fi

unexpected_calls=""
call_count=0
if [[ -n "${signer_calls}" ]]; then
  unexpected_calls="$(printf '%s\n' "${signer_calls}" | rg -v '^internal/client/client\.go:' || true)"
  call_count="$(printf '%s\n' "${signer_calls}" | rg -o 'newHMACSigner[[:space:]]*\(' | wc -l | tr -d ' ')"
fi

if [[ -n "${unexpected_calls}" || "${call_count}" -gt 2 ]]; then
  echo "newHMACSigner production call sites are restricted to internal/client/client.go (max 2); offending call lines:" >&2
  if [[ -n "${unexpected_calls}" ]]; then
    printf '%s\n' "${unexpected_calls}" >&2
  fi
  if [[ "${call_count}" -gt 2 ]]; then
    printf '%s\n' "${signer_calls}" >&2
  fi
  status=1
fi

if [[ "${status}" -eq 0 ]]; then
  echo "ok - hmac boundary clean"
fi

exit "${status}"
