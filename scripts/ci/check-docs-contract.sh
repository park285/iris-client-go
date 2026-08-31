#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readme="${1:-$repo_root/README.md}"
changelog="${2:-$repo_root/CHANGELOG.md}"
docs_dir="${3:-$repo_root/docs}"

unreleased_count="$(awk '$0 == "## 미출시" { count++ } END { print count + 0 }' "$changelog")"
if [[ "$unreleased_count" != "1" ]]; then
  echo "CHANGELOG must contain exactly one Unreleased section" >&2
  exit 1
fi
first_section="$(awk '/^## / { print; exit }' "$changelog")"
if [[ "$first_section" != "## 미출시" ]]; then
  echo "CHANGELOG Unreleased must be the first section" >&2
  exit 1
fi

versions="$(awk '
  /^## v[0-9]+\.[0-9]+\.[0-9]+ - / {
    version = $2
    sub(/^v/, "", version)
    print version
  }
' "$changelog")"
if [[ -z "$versions" ]]; then
  echo "CHANGELOG has no release sections" >&2
  exit 1
fi

duplicates="$(printf '%s\n' "$versions" | LC_ALL=C sort | uniq -d)"
if [[ -n "$duplicates" ]]; then
  echo "CHANGELOG contains duplicate release sections: $duplicates" >&2
  exit 1
fi

expected="$(printf '%s\n' "$versions" | LC_ALL=C sort -V -r)"
if [[ "$versions" != "$expected" ]]; then
  echo "CHANGELOG release sections are not in reverse SemVer order" >&2
  diff -u <(printf '%s\n' "$expected") <(printf '%s\n' "$versions") >&2 || true
  exit 1
fi

tag_versions="$(git -C "$repo_root" tag --list 'v*' --sort=-v:refname | awk '
  /^v[0-9]+\.[0-9]+\.[0-9]+$/ { sub(/^v/, ""); print }
')"
if [[ "$versions" != "$tag_versions" ]]; then
  staged_version="$(printf '%s\n' "$versions" | sed -n '1p')"
  released_versions="$(printf '%s\n' "$versions" | sed '1d')"
  if [[ -z "$staged_version" || "$released_versions" != "$tag_versions" ]]; then
    echo "CHANGELOG release sections must match SemVer tags plus at most one leading staged release" >&2
    diff -u <(printf '%s\n' "$tag_versions") <(printf '%s\n' "$versions") >&2 || true
    exit 1
  fi
fi

module_path="$(GOWORK=off go -C "$repo_root" list -m -f '{{.Path}}')"
active_major="${module_path##*/}"
if [[ ! "$active_major" =~ ^v[2-9][0-9]*$ ]]; then
  echo "module path must end in an active semantic import major" >&2
  exit 1
fi

readme_lead="$(sed -n '1,24p' "$readme")"
if [[ "$readme_lead" != *"${module_path}@latest"* ]] ||
   [[ "$readme_lead" != *"./docs/MIGRATION-${active_major}.0.0.md"* ]]; then
  echo "README lead must identify the active module and nearest major migration guide" >&2
  exit 1
fi
if [[ ! -f "$docs_dir/MIGRATION-${active_major}.0.0.md" ]]; then
  echo "README active-major migration guide does not exist" >&2
  exit 1
fi
if [[ "$readme_lead" != *'./docs/MIGRATION-v0.11.0.md'* ]] ||
   [[ "$readme_lead" != *'역사 문서'* ]]; then
  echo "README must label the v0.11 migration guide as historical" >&2
  exit 1
fi
