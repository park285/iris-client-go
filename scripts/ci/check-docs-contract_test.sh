#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
checker="$repo_root/scripts/ci/check-docs-contract.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

bash "$checker"

cp "$repo_root/README.md" "$tmp_dir/README.md"
cp "$repo_root/CHANGELOG.md" "$tmp_dir/CHANGELOG.md"
printf '\n## 미출시\n' >>"$tmp_dir/CHANGELOG.md"
if bash "$checker" "$tmp_dir/README.md" "$tmp_dir/CHANGELOG.md" >/dev/null 2>&1; then
  echo "docs checker accepted duplicate Unreleased sections" >&2
  exit 1
fi

awk '$0 != "## 미출시"' "$repo_root/CHANGELOG.md" >"$tmp_dir/CHANGELOG.md"
printf '\n## 미출시\n' >>"$tmp_dir/CHANGELOG.md"
if bash "$checker" "$tmp_dir/README.md" "$tmp_dir/CHANGELOG.md" >/dev/null 2>&1; then
  echo "docs checker accepted Unreleased after release sections" >&2
  exit 1
fi

awk '
  /^## v2\.3\.1 - / { first = $0; next }
  /^## v2\.3\.0 - / && first != "" { print; print first; first = ""; next }
  { print }
' "$repo_root/CHANGELOG.md" >"$tmp_dir/CHANGELOG.md"
if bash "$checker" "$tmp_dir/README.md" "$tmp_dir/CHANGELOG.md" >/dev/null 2>&1; then
  echo "docs checker accepted release sections out of SemVer order" >&2
  exit 1
fi

awk '
  /^## v2\.4\.0 - / { print "## v2.5.0 - 2026-08-31\n\n- staged twice" }
  { print }
' "$repo_root/CHANGELOG.md" >"$tmp_dir/CHANGELOG.md"
if bash "$checker" "$repo_root/README.md" "$tmp_dir/CHANGELOG.md" >/dev/null 2>&1; then
  echo "docs checker accepted more than one staged release" >&2
  exit 1
fi

awk '
  /^## v2\.2\.2 - / { dropping = 1; next }
  dropping && /^## / { dropping = 0 }
  !dropping { print }
' "$repo_root/CHANGELOG.md" >"$tmp_dir/CHANGELOG.md"
if bash "$checker" "$tmp_dir/README.md" "$tmp_dir/CHANGELOG.md" >/dev/null 2>&1; then
  echo "docs checker accepted a missing tagged release" >&2
  exit 1
fi

sed 's#\./docs/MIGRATION-v2\.0\.0\.md#./docs/MIGRATION-v1.0.0.md#' \
  "$repo_root/README.md" >"$tmp_dir/README.md"
if bash "$checker" "$tmp_dir/README.md" "$repo_root/CHANGELOG.md" >/dev/null 2>&1; then
  echo "docs checker accepted a README without the v2 migration guide" >&2
  exit 1
fi

mkdir "$tmp_dir/empty-docs"
if bash "$checker" "$repo_root/README.md" "$repo_root/CHANGELOG.md" \
  "$tmp_dir/empty-docs" >/dev/null 2>&1; then
  echo "docs checker accepted a missing active-major migration file" >&2
  exit 1
fi
