#!/usr/bin/env bash
set -euo pipefail
unset CDPATH

root=$(pwd -P)
cd -P -- "$root"
target=$1
shift
if [[ "$target" == fmt || "$target" == lint ]]; then
  inventory=$(scripts/ci/python-runner.sh -- -I scripts/ci/go-lint-matrix.py --list-modules)
elif [[ -e .git ]]; then
  inventory=$(git ls-files -z -- go.mod ':(glob)**/go.mod' ':(exclude,glob)**/vendor/**' |
    while IFS= read -r -d '' manifest; do
      [[ "$manifest" != *$'\n'* ]] || { echo "unsupported module path with a newline" >&2; exit 1; }
      dir=$(dirname -- "$manifest")
      printf '%s\n' "$dir"
    done)
else
  inventory=$(find . -type d -name vendor -prune -o -type f -name go.mod -exec dirname {} \; | sed 's|^\./||')
fi
[[ -n "$inventory" ]] || { echo "no Go modules in $root" >&2; exit 1; }

relative_module() {
  local dir
  dir=$(cd -L -- "$1" 2>/dev/null && pwd -L) || return 1
  case "$dir" in
    "$root") printf '.\n' ;;
    "$root"/*) printf '%s\n' "${dir#"$root/"}" ;;
    *) return 1 ;;
  esac
}

single_module() {
  local relative
  relative=$(relative_module "$1") && [[ "$relative" != *$'\n'* ]] && grep -Fxq -e "$relative" <<<"$inventory"
}

if (( $# == 1 )); then
  requested=$1
  trimmed=${requested#"${requested%%[![:space:]]*}"}
  trimmed=${trimmed%"${trimmed##*[![:space:]]}"}
  if [[ -z "$trimmed" ]]; then
    set --
  elif single_module "$requested"; then
    :
  elif single_module "$trimmed"; then
    set -- "$trimmed"
  else
    read -r -d '' -a requested_modules <<<"${requested//[[:space:]]/ }" || true
    set -- "${requested_modules[@]}"
  fi
fi
if (( $# == 0 )); then
  if [[ "$target" == test-race && -e .git ]] && ! grep -Fxq -e adapters/valkeydedup <<<"$inventory"; then
    echo "adapter contract: missing tracked public module adapters/valkeydedup" >&2
    exit 1
  fi
  while IFS= read -r module; do set -- "$@" "$module"; done <<<"$inventory"
fi
selected=()
for module in "$@"; do
  dir=$(cd -L -- "$module" && pwd -L)
  case "$dir" in
    "$root") relative=. ;;
    "$root"/*) relative=${dir#"$root/"} ;;
    *) echo "unknown repository module: $module" >&2; exit 1 ;;
  esac
  if [[ "$relative" == *$'\n'* ]] || ! grep -Fxq -e "$relative" <<<"$inventory"; then
    echo "unknown repository module: $module" >&2
    exit 1
  fi
  if [[ ! -f "$dir/go.mod" || -L "$dir/go.mod" || "$(cd -P -- "$dir" && pwd -P)" != "$dir" ]]; then
    echo "missing or symlinked module manifest: $dir/go.mod" >&2
    exit 1
  fi
  selected+=("$dir")
done
printf '%s\n' "${selected[@]}"
