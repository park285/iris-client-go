#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root_dir"

fail_on_match() {
  local description="$1"
  local output status
  shift
  set +e
  output="$("$@" 2>&1)"
  status=$?
  set -e
  if (( status == 0 )); then
    printf '%s\n' "$output" >&2
    echo "legacy removal gate: ${description}" >&2
    return 1
  fi
  if (( status != 1 )); then
    printf '%s\n' "$output" >&2
    echo "legacy removal gate: inspection command failed for ${description}" >&2
    return "$status"
  fi
}

production_paths=(webhook internal/dedup valkeydedup webhooksign iris)

find_pre_v2_imports() {
  python3 - <<'PY'
from pathlib import Path
import subprocess

old_path = "github.com/park285/iris-client-go"
current_path = f"{old_path}/v2"
matches = []
tracked = subprocess.check_output(
    ["git", "ls-files", "-z", "--", "*.go"],
).decode("utf-8").split("\0")
for name in filter(None, tracked):
    path = Path(name)
    for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if old_path in line and current_path not in line:
            matches.append(f"{path}:{line_number}:{line}")

if matches:
    print("\n".join(matches))
    raise SystemExit(0)
raise SystemExit(1)
PY
}

fail_on_match "retired public or production symbol is reachable" \
  grep -RInE --exclude='*_test.go' \
  'type (Deduplicator|DedupReleaser|StatefulDeduplicator)\b|func (WithDeduplicator|WithNonceCache|SignRequestV3)\b|SignatureVersionV2|CanonicalWebhookRequestV2|V2Validated|nonceCacheFellBack|resolveNonceCacheBackend|orphanedReservation' \
  "${production_paths[@]}"

fail_on_match "production memory nonce default is reachable" \
  grep -RInE --exclude='*_test.go' 'newMemoryNonceCache|memoryNonceCache' webhook

fail_on_match "current documentation advertises a retired API" \
  grep -nE 'WithDeduplicator|WithNonceCache|SignRequestV3|SignatureVersionV2|V2Validated|valkeydedup\.(New|Option)\b' \
  README.md docs/webhook-routing.md

if [[ "$(sed -n '1p' go.mod)" != 'module github.com/park285/iris-client-go/v2' ]]; then
  echo "legacy removal gate: go.mod module path is not the v2 semantic import path" >&2
  exit 1
fi

fail_on_match "Go source imports the pre-v2 module path" \
  find_pre_v2_imports

echo "legacy removal gate passed"
