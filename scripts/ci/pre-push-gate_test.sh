#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
GATE="${ROOT_DIR}/scripts/ci/pre-push-gate.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

MOCK_BIN="${TMP_DIR}/bin"
MAKE_LOG="${TMP_DIR}/make.log"
mkdir -p "${MOCK_BIN}"
cat >"${MOCK_BIN}/make" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"${MAKE_LOG}"
if [[ "${1:-}" == "${MAKE_FAIL_TARGET:-}" ]]; then
  exit 23
fi
EOF
chmod +x "${MOCK_BIN}/make"

run_gate() {
  PATH="${MOCK_BIN}:${PATH}" MAKE_LOG="${MAKE_LOG}" "${GATE}" "$@"
}

: >"${MAKE_LOG}"
default_output="$(run_gate)"
expected_default=$'lint\ntest\ntest-race\nvulncheck\ntidy'
[[ "$(cat "${MAKE_LOG}")" == "${expected_default}" ]] || fail "no-argument gate order changed"
[[ "${default_output}" == *"iris-client-go pre-push full gate"* ]] || fail "no-argument start banner changed"
[[ "${default_output}" == *"iris-client-go pre-push full gate passed"* ]] || fail "no-argument success banner changed"
failure_output="${TMP_DIR}/failure.out"
: >"${MAKE_LOG}"
if MAKE_FAIL_TARGET="test" run_gate >"${failure_output}" 2>&1; then
  fail "no-argument gate ignored a stage failure"
fi
[[ "$(cat "${MAKE_LOG}")" == $'lint\ntest' ]] || fail "no-argument gate did not stop at the failing stage"
grep -Fxq '[pre-push] make test' "${failure_output}" || fail "failing stage banner changed"
if grep -Fq 'full gate passed' "${failure_output}"; then
  fail "no-argument gate printed success after failure"
fi
awk '
  /^lint: check-boundaries$/ { lint_boundary = 1 }
  /^test:$/ { test_target = 1 }
  /^test-race:$/ { race_target = 1 }
  /^vulncheck:$/ { vuln_target = 1 }
  /^tidy:$/ { tidy_target = 1 }
  END { exit !(lint_boundary && test_target && race_target && vuln_target && tidy_target) }
' "${ROOT_DIR}/Makefile" || fail "expected Makefile targets or lint boundary dependency are missing"

expected_manifest='{"schema_version":1,"protocol":"iris-stack-pre-push-gate-v1","profile_id":"iris-client-go-v1"}'
[[ "$(cat "${ROOT_DIR}/scripts/ci/pre-push-gate-profile-v1.json")" == "${expected_manifest}" ]] || \
  fail "profile manifest is not the strict v1 contract"

: >"${MAKE_LOG}"
run_gate --phase=reusable >/dev/null
[[ "$(cat "${MAKE_LOG}")" == $'lint\ntest\ntest-race' ]] || fail "reusable phase coverage changed"

: >"${MAKE_LOG}"
run_gate --phase=freshness >/dev/null
[[ "$(cat "${MAKE_LOG}")" == $'vulncheck\ntidy' ]] || fail "freshness phase coverage changed"

: >"${MAKE_LOG}"
run_gate --phase=ambient >/dev/null
[[ ! -s "${MAKE_LOG}" ]] || fail "ambient phase must be a no-op"

for args in "--phase=unknown" "fingerprint" "--phase fingerprint"; do
  read -r -a argv <<<"${args}"
  if run_gate "${argv[@]}" >/dev/null 2>&1; then
    fail "invalid arguments were accepted: ${args}"
  fi
done

FIXTURE="${TMP_DIR}/fixture"
mkdir -p "${FIXTURE}/scripts/ci"
cp "${ROOT_DIR}/.golangci.yml" "${ROOT_DIR}/Makefile" "${ROOT_DIR}/go.mod" \
  "${ROOT_DIR}/go.sum" "${FIXTURE}/"
cp "${ROOT_DIR}/scripts/ci/go-tooling.sh" \
  "${ROOT_DIR}/scripts/ci/pre-push-gate-profile-v1.json" \
  "${ROOT_DIR}/scripts/ci/pre-push-gate.sh" "${FIXTURE}/scripts/ci/"
git -C "${FIXTURE}" init -q
git -C "${FIXTURE}" add .
git -C "${FIXTURE}" -c user.name=test -c user.email=test@example.invalid commit -qm fixture
head_sha="$(git -C "${FIXTURE}" rev-parse HEAD)"
zero=0000000000000000000000000000000000000000

fingerprint="$({
  cd "${FIXTURE}"
  HEAD_SHA="${head_sha}" \
  PRE_PUSH_LOCAL_REF=refs/heads/main \
  PRE_PUSH_LOCAL_SHA="${head_sha}" \
  PRE_PUSH_REMOTE_REF=refs/heads/main \
  PRE_PUSH_REMOTE_SHA="${zero}" \
  PRE_PUSH_UPDATE_KIND=branch \
  PRE_PUSH_MODE=fast \
  scripts/ci/pre-push-gate.sh --phase=fingerprint
})"

fingerprint_pattern='^\{"schema_version":1,"profile_id":"iris-client-go-v1","effective_base_sha":"","route_fingerprint":"[0-9a-f]{64}","toolchain_fingerprint":"[0-9a-f]{64}","profile_inputs_fingerprint":"[0-9a-f]{64}"\}$'
[[ "${fingerprint}" =~ ${fingerprint_pattern} ]] || \
  fail "fingerprint is not strict canonical JSON: ${fingerprint}"

touch "${FIXTURE}/dirty-file"
if (
  cd "${FIXTURE}"
  HEAD_SHA="${head_sha}" \
  PRE_PUSH_LOCAL_REF=refs/heads/main \
  PRE_PUSH_LOCAL_SHA="${head_sha}" \
  PRE_PUSH_REMOTE_REF=refs/heads/main \
  PRE_PUSH_REMOTE_SHA="${zero}" \
  PRE_PUSH_UPDATE_KIND=branch \
  PRE_PUSH_MODE=fast \
  scripts/ci/pre-push-gate.sh --phase=fingerprint
) >/dev/null 2>&1; then
  fail "fingerprint accepted a dirty worktree"
fi
rm "${FIXTURE}/dirty-file"

STATUS_BIN="${TMP_DIR}/status-bin"
mkdir -p "${STATUS_BIN}"
REAL_GIT="$(command -v git)"
cat >"${STATUS_BIN}/git" <<GIT
#!/usr/bin/env bash
if [[ "\${1:-}" == status ]]; then
  exit 42
fi
exec "${REAL_GIT}" "\$@"
GIT
chmod +x "${STATUS_BIN}/git"
if (
  cd "${FIXTURE}"
  PATH="${STATUS_BIN}:${PATH}" \
  HEAD_SHA="${head_sha}" \
  PRE_PUSH_LOCAL_REF=refs/heads/main \
  PRE_PUSH_LOCAL_SHA="${head_sha}" \
  PRE_PUSH_REMOTE_REF=refs/heads/main \
  PRE_PUSH_REMOTE_SHA="${zero}" \
  PRE_PUSH_UPDATE_KIND=branch \
  PRE_PUSH_MODE=fast \
  scripts/ci/pre-push-gate.sh --phase=fingerprint
) >/dev/null 2>&1; then
  fail "fingerprint accepted git status failure"
fi

echo "PASS: pre-push gate phase and fingerprint contract"
