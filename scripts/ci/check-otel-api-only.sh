#!/usr/bin/env bash
set -euo pipefail

# 라이브러리는 OpenTelemetry API와 propagation만 쓴다. SDK·exporter·contrib 선택은 소비자 몫이다.
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
if forbidden="$(GOWORK=off go list -deps ./... | grep -E '^go\.opentelemetry\.io/(otel/(sdk|exporters)|contrib)(/|$)')"; then
  printf 'OpenTelemetry SDK/exporter/contrib dependency is forbidden:\n%s\n' "${forbidden}" >&2
  exit 1
fi
echo "ok: OpenTelemetry API-only dependency policy"
