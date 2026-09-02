#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
. "${SCRIPT_DIR}/python-runtime.sh"
repo_python_init
cd "${ROOT_DIR}"

unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_PREFIX

export GOWORK=off

route_resolved=false
route_exact=false
changed_files=""

normalize_mode() {
  if [[ "${FULL_PRE_PUSH:-false}" == "true" ]]; then
    PRE_PUSH_MODE="${PRE_PUSH_MODE:-full}"
  else
    PRE_PUSH_MODE="${PRE_PUSH_MODE:-fast}"
  fi
  case "${PRE_PUSH_MODE}" in
    fast|full) export PRE_PUSH_MODE ;;
    *)
      echo "unsupported PRE_PUSH_MODE=${PRE_PUSH_MODE}; expected fast or full" >&2
      exit 1
      ;;
  esac
}

# 변경 집합은 정확한 push 범위(BASE_SHA..HEAD_SHA)에서만 계산한다. 범위가 없으면(새 branch·
# 삭제·수동 실행) 집합을 비워 docs-only skip과 자기 테스트 생략을 모두 닫는다.
# 계약: iris-stack docs/contracts/pre-push-gate-phases-v1.md
resolve_route() {
  if [[ "${route_resolved}" == "true" ]]; then
    return 0
  fi
  if [[ -n "${BASE_SHA:-}" && -n "${HEAD_SHA:-}" ]]; then
    if ! changed_files="$(git diff --name-only "${BASE_SHA}..${HEAD_SHA}")"; then
      echo "failed to resolve exact pushed range ${BASE_SHA}..${HEAD_SHA}" >&2
      exit 1
    fi
    route_exact=true
  else
    changed_files=""
    route_exact=false
  fi
  route_resolved=true
}

# docs/markdown 만 바뀐 push 는 Go 검증을 건너뛴다. full 모드는 이 최적화를 우회한다.
# grep -qv 회피: ugrep 는 quiet+invert 조합 exit 코드가 GNU grep 과 달라 필터 결과로 판정한다.
is_docs_only_route() {
  local non_doc_changes docs_code_changes
  resolve_route
  non_doc_changes="$(grep -vE '^(docs/|.*\.md$)' <<<"${changed_files}" || true)"
  docs_code_changes="$(grep -E '^docs/.*\.(go|sh|sql)$' <<<"${changed_files}" || true)"
  [[ "${PRE_PUSH_MODE}" != "full" ]] && [[ -n "${changed_files}" ]] \
    && [[ -z "${non_doc_changes}" ]] && [[ -z "${docs_code_changes}" ]]
}

self_test_inputs_changed() {
  local input file
  for input in "$@"; do
    while IFS= read -r file; do
      [[ -n "${file}" ]] || continue
      case "${file}" in
        "${input}"|"${input}"/*) return 0 ;;
      esac
    done <<<"${changed_files}"
  done
  return 1
}

# 자기 테스트는 테스트 자신·검사기·fixture·공용 lib 이 push 범위에서 바뀔 때만 실행한다.
# 범위를 못 얻었거나 PRE_PUSH_MODE=full 이면 전량 실행한다(fail-closed).
run_self_test() {
  local test="$1"
  shift
  resolve_route
  if [[ "${PRE_PUSH_MODE}" != "full" && "${route_exact}" == "true" ]] \
    && ! self_test_inputs_changed "${test}" "$@"; then
    echo "[pre-push] self-test skipped (inputs unchanged): ${test}"
    return 0
  fi
  echo "[pre-push] self-test: ${test}"
  case "${test}" in
    *.py) "${CI_PYTHON_BIN}" "${test}" ;;
    *) bash "${test}" ;;
  esac
}

run_stage() {
  echo "[pre-push] $*"
  "$@"
}

run_reusable_phase() {
  if is_docs_only_route; then
    echo "[pre-push] docs-only change detected; skipping reusable Go gate"
    return 0
  fi
  run_self_test scripts/check-hmac-boundary_test.sh scripts/check-hmac-boundary.sh
  run_stage make lint
  run_stage make test-race
}

run_freshness_phase() {
  if is_docs_only_route; then
    echo "[pre-push] docs-only change detected; skipping freshness Go gate"
    return 0
  fi
  run_stage make vulncheck
  run_stage make tidy
}

run_ambient_phase() {
  if is_docs_only_route; then
    echo "[pre-push] docs-only change detected; skipping ambient Go gate"
    return 0
  fi
  return 0
}

sha256_text() {
  command -v sha256sum >/dev/null 2>&1 || {
    echo "pre-push fingerprint: required tool is missing: sha256sum" >&2
    return 1
  }
  sha256sum | awk '{print $1}'
}

require_sha() {
  local name="$1" value="$2"
  if [[ ! "${value}" =~ ^[0-9a-f]{40}$ && ! "${value}" =~ ^[0-9a-f]{64}$ ]]; then
    echo "pre-push fingerprint: ${name} must be a lowercase Git object ID" >&2
    return 1
  fi
}

require_ref() {
  local name="$1" value="$2"
  if [[ ! "${value}" =~ ^refs/(heads|tags)/[A-Za-z0-9._/-]+$ ]]; then
    echo "pre-push fingerprint: ${name} is not a supported ref" >&2
    return 1
  fi
}

hash_profile_inputs() {
  local path digest
  local -a paths=(
    .golangci.yml
    Makefile
    go.mod
    go.sum
    scripts/ci/go-tooling.sh
    .python-version
    scripts/ci/python-runner.sh
    scripts/ci/python-runtime.sh
    scripts/ci/pre-push-gate-profile-v1.json
    scripts/ci/pre-push-gate.sh
  )

  for path in "${paths[@]}"; do
    [[ -f "${path}" ]] || {
      echo "pre-push fingerprint: required input is missing: ${path}" >&2
      return 1
    }
    digest="$(sha256sum "${path}" | awk '{print $1}')"
    printf '%s=%s\n' "${path}" "${digest}"
  done | sha256_text
}

print_fingerprint() {
  local repository_head tree_sha effective_base_sha route_fingerprint
  local go_version go_env toolchain_fingerprint profile_inputs_fingerprint status_output

  command -v git >/dev/null 2>&1 || {
    echo "pre-push fingerprint: required tool is missing: git" >&2
    return 1
  }
  command -v go >/dev/null 2>&1 || {
    echo "pre-push fingerprint: required tool is missing: go" >&2
    return 1
  }
  command -v awk >/dev/null 2>&1 || {
    echo "pre-push fingerprint: required tool is missing: awk" >&2
    return 1
  }

  repository_head="$(git rev-parse --verify HEAD)"
  tree_sha="$(git rev-parse --verify 'HEAD^{tree}')"
  require_sha repository_head "${repository_head}"
  require_sha tree_sha "${tree_sha}"
  if ! status_output="$(git status --porcelain=v1 --untracked-files=all)"; then
    echo "pre-push fingerprint: repository status is unavailable" >&2
    return 1
  fi
  if [[ -n "$status_output" ]]; then
    echo "pre-push fingerprint: repository worktree must be clean" >&2
    return 1
  fi

  require_sha HEAD_SHA "${HEAD_SHA:-}"
  require_sha PRE_PUSH_LOCAL_SHA "${PRE_PUSH_LOCAL_SHA:-}"
  require_sha PRE_PUSH_REMOTE_SHA "${PRE_PUSH_REMOTE_SHA:-}"
  require_ref PRE_PUSH_LOCAL_REF "${PRE_PUSH_LOCAL_REF:-}"
  require_ref PRE_PUSH_REMOTE_REF "${PRE_PUSH_REMOTE_REF:-}"
  case "${PRE_PUSH_UPDATE_KIND:-}" in
    branch|tag|deletion) ;;
    *) echo "pre-push fingerprint: PRE_PUSH_UPDATE_KIND is invalid" >&2; return 1 ;;
  esac
  case "${PRE_PUSH_MODE:-}" in
    fast|full) ;;
    *) echo "pre-push fingerprint: PRE_PUSH_MODE is invalid" >&2; return 1 ;;
  esac
  if [[ -n "${PRE_PUSH_PEELED_TAG_TARGET:-}" ]]; then
    require_sha PRE_PUSH_PEELED_TAG_TARGET "${PRE_PUSH_PEELED_TAG_TARGET}"
  fi

  effective_base_sha="${BASE_SHA:-}"
  if [[ -n "${effective_base_sha}" ]]; then
    require_sha BASE_SHA "${effective_base_sha}"
  fi
  if [[ "${PRE_PUSH_REMOTE_SHA}" =~ ^0+$ ]]; then
    [[ -z "${effective_base_sha}" ]] || {
      echo "pre-push fingerprint: BASE_SHA must be empty for a new remote ref" >&2
      return 1
    }
  elif [[ "${effective_base_sha}" != "${PRE_PUSH_REMOTE_SHA}" ]]; then
    echo "pre-push fingerprint: BASE_SHA does not match PRE_PUSH_REMOTE_SHA" >&2
    return 1
  fi

  route_fingerprint="$({
    printf 'repository_head=%s\n' "${repository_head}"
    printf 'tree_sha=%s\n' "${tree_sha}"
    printf 'head_sha=%s\n' "${HEAD_SHA}"
    printf 'base_sha=%s\n' "${effective_base_sha}"
    printf 'local_ref=%s\n' "${PRE_PUSH_LOCAL_REF}"
    printf 'local_sha=%s\n' "${PRE_PUSH_LOCAL_SHA}"
    printf 'remote_ref=%s\n' "${PRE_PUSH_REMOTE_REF}"
    printf 'remote_sha=%s\n' "${PRE_PUSH_REMOTE_SHA}"
    printf 'update_kind=%s\n' "${PRE_PUSH_UPDATE_KIND}"
    printf 'mode=%s\n' "${PRE_PUSH_MODE}"
    printf 'peeled_tag_target=%s\n' "${PRE_PUSH_PEELED_TAG_TARGET:-}"
  } | sha256_text)"

  go_version="$(go version)"
  go_env="$(go env GOOS GOARCH GOVERSION GOTOOLCHAIN)"
  [[ -n "${go_version}" && -n "${go_env}" ]] || {
    echo "pre-push fingerprint: Go toolchain identity is unavailable" >&2
    return 1
  }
  toolchain_fingerprint="$({
    printf 'go_version=%s\n' "${go_version}"
    printf 'go_env=%s\n' "${go_env}"
    sha256sum scripts/ci/go-tooling.sh | awk '{print "go_tooling_sha256=" $1}'
    sha256sum Makefile | awk '{print "makefile_sha256=" $1}'
    printf 'uv_version=%s\n' "$(uv --version)"
    printf 'python_version=%s\n' "$("${CI_PYTHON_BIN}" -I -S -c 'import platform; print(platform.python_version())')"
    sha256sum .python-version scripts/ci/python-runner.sh scripts/ci/python-runtime.sh
  } | sha256_text)"
  profile_inputs_fingerprint="$(hash_profile_inputs)"

  printf '{"schema_version":1,"profile_id":"iris-client-go-v1","effective_base_sha":"%s","route_fingerprint":"%s","toolchain_fingerprint":"%s","profile_inputs_fingerprint":"%s"}\n' \
    "${effective_base_sha}" "${route_fingerprint}" "${toolchain_fingerprint}" "${profile_inputs_fingerprint}"
}

if (( $# == 0 )); then
  echo "════════════════════════════════════════"
  echo "  iris-client-go pre-push full gate"
  echo "════════════════════════════════════════"

  normalize_mode
  run_reusable_phase
  run_freshness_phase
  run_ambient_phase

  echo "════════════════════════════════════════"
  echo "  iris-client-go pre-push full gate passed"
  echo "════════════════════════════════════════"
  exit 0
fi

if (( $# != 1 )); then
  echo "usage: $0 [--phase=fingerprint|reusable|freshness|ambient]" >&2
  exit 2
fi

case "$1" in
  --phase=fingerprint) print_fingerprint ;;
  --phase=reusable) normalize_mode; run_reusable_phase ;;
  --phase=freshness) normalize_mode; run_freshness_phase ;;
  --phase=ambient) normalize_mode; run_ambient_phase ;;
  *)
    echo "usage: $0 [--phase=fingerprint|reusable|freshness|ambient]" >&2
    exit 2
    ;;
esac
