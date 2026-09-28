#!/usr/bin/env bash
set -euo pipefail

# 라이브러리는 OpenTelemetry API와 propagation만 쓴다. SDK·exporter·contrib 선택은 소비자 몫이다.
# 테스트 전용 import와 go.mod require도 소비자 모듈 그래프(MVS)에 들어가므로 함께 검사한다.
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
export GOWORK=off

forbidden_pattern='^go\.opentelemetry\.io/(otel/(sdk|exporters)|contrib)(/|$)'

# 목록 수집 실패는 판정 불가이므로 grep 전에 따로 받아 즉시 실패시킨다.
packages="$(go list -deps -test ./...)"
modules="$(go mod edit -json | grep -oE '"Path": *"[^"]+"' | cut -d'"' -f4)"

if forbidden="$(sort -u <<<"${packages}"$'\n'"${modules}" | grep -E "${forbidden_pattern}")"; then
  printf 'OpenTelemetry SDK/exporter/contrib dependency is forbidden:\n%s\n' "${forbidden}" >&2
  exit 1
fi
echo "ok: OpenTelemetry API-only dependency policy"
