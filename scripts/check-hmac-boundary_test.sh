#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CHECKER="${SCRIPT_DIR}/check-hmac-boundary.sh"

cd "${REPO_ROOT}"

if [[ ! -x "${CHECKER}" ]]; then
  echo "not ok - checker missing or not executable: ${CHECKER}" >&2
  exit 1
fi

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "${TMP_ROOT}"' EXIT
LAST_STATUS=0
LAST_OUTPUT=""
PASSED=0

write_clean_fixture() {
  local root="$1"

  mkdir -p "${root}/internal/client/transport" "${root}/internal/client/signing"
  cat >"${root}/internal/client/transport/client.go" <<'EOF'
package client

import (
  "strings"
  signing "example/signing"
)

type authSecrets struct {
	inboundSecret string
}

type H2CClient struct {
	signers map[string]*hmacSigner
}

func buildHMACSigners(auth authSecrets) map[string]*hmacSigner {
	signers := make(map[string]*hmacSigner, 1)
	for _, secret := range []string{strings.TrimSpace(auth.inboundSecret)} {
		if secret == "" {
			continue
		}
		if _, ok := signers[secret]; !ok {
			signers[secret] = signing.NewHMACSigner(secret)
		}
	}
	return signers
}

func (c *H2CClient) signerFor(secret string) *hmacSigner {
	if signer, ok := c.signers[secret]; ok {
		return signer
	}
	return signing.NewHMACSigner(secret)
}
EOF

  cat >"${root}/internal/client/signing/signer.go" <<'EOF'
package client

type hmacSigner struct{}

func NewHMACSigner(secret string) *hmacSigner {
	return &hmacSigner{}
}
EOF
}

run_checker() {
  local root="$1"

  set +e
  LAST_OUTPUT="$("${CHECKER}" --root "${root}" 2>&1)"
  LAST_STATUS=$?
  set -e
}

run_checker_without_go() {
  local root="$1"
  local bin_dir="${TMP_ROOT}/bin-without-go"

  mkdir -p "${bin_dir}"
  ln -sf "$(command -v bash)" "${bin_dir}/bash"

  set +e
  LAST_OUTPUT="$(env -i PATH="${bin_dir}" "${CHECKER}" --root "${root}" 2>&1)"
  LAST_STATUS=$?
  set -e
}

assert_success() {
  local name="$1"

  if [[ "${LAST_STATUS}" -ne 0 ]]; then
    printf 'not ok - %s\nstatus=%s\n%s\n' "${name}" "${LAST_STATUS}" "${LAST_OUTPUT}" >&2
    exit 1
  fi
  PASSED=$((PASSED + 1))
  printf 'ok - %s\n' "${name}"
}

assert_failure() {
  local name="$1"

  if [[ "${LAST_STATUS}" -eq 0 ]]; then
    printf 'not ok - %s unexpectedly succeeded\n%s\n' "${name}" "${LAST_OUTPUT}" >&2
    exit 1
  fi
  PASSED=$((PASSED + 1))
  printf 'ok - %s\n' "${name}"
}

assert_contains() {
  local name="$1"
  local needle="$2"

  if [[ "${LAST_OUTPUT}" != *"${needle}"* ]]; then
    printf 'not ok - %s missing %q\n%s\n' "${name}" "${needle}" "${LAST_OUTPUT}" >&2
    exit 1
  fi
}

run_checker "${REPO_ROOT}"
assert_success "current codebase has no HMAC boundary violations"

fixture="${TMP_ROOT}/sign-helper-production"
write_clean_fixture "${fixture}"
cat >"${fixture}/internal/client/transport/hmac_helper.go" <<'EOF'
package client

func signIrisRequest(secret, method, path, timestamp, nonce, body string) (string, error) {
	return "", nil
}
EOF
run_checker "${fixture}"
assert_failure "production signIrisRequest is rejected"
assert_contains "production signIrisRequest location" "internal/client/transport/hmac_helper.go:"

fixture="${TMP_ROOT}/sign-helper-method-production"
write_clean_fixture "${fixture}"
cat >"${fixture}/internal/client/transport/hmac_method.go" <<'EOF'
package client

type requestSigner struct{}

func (r *requestSigner) signIrisRequest(secret, method, path, timestamp, nonce, body string) (string, error) {
	return "", nil
}
EOF
run_checker "${fixture}"
assert_failure "production method-form signIrisRequest is rejected"
assert_contains "production method-form signIrisRequest location" "internal/client/transport/hmac_method.go:"

fixture="${TMP_ROOT}/missing-go"
write_clean_fixture "${fixture}"
cat >"${fixture}/internal/client/transport/hmac_helper.go" <<'EOF'
package client

func signIrisRequest(secret, method, path, timestamp, nonce, body string) (string, error) {
	return "", nil
}
EOF
run_checker_without_go "${fixture}"
assert_failure "missing go fails closed"
assert_contains "missing go error" "required command not found: go"

fixture="${TMP_ROOT}/third-client-call"
write_clean_fixture "${fixture}"
cat >>"${fixture}/internal/client/transport/client.go" <<'EOF'

func extraSigner(secret string) *hmacSigner {
	return NewHMACSigner(secret)
}
EOF
run_checker "${fixture}"
assert_failure "third client.go NewHMACSigner call is rejected"
assert_contains "third client.go call location" "internal/client/transport/client.go:"

fixture="${TMP_ROOT}/packed-client-calls"
mkdir -p "${fixture}/internal/client/transport"
cat >"${fixture}/internal/client/transport/client.go" <<'EOF'
package client

type hmacSigner struct{}

func buildSigners(a, b string) []*hmacSigner {
	return []*hmacSigner{NewHMACSigner(a), NewHMACSigner(b)}
}

func signerFor(secret string) *hmacSigner { return NewHMACSigner(secret) }

func NewHMACSigner(secret string) *hmacSigner {
	return &hmacSigner{}
}
EOF
run_checker "${fixture}"
assert_failure "packed client.go NewHMACSigner calls are counted by occurrence"
assert_contains "packed client.go call location" "internal/client/transport/client.go:"

fixture="${TMP_ROOT}/different-production-file"
write_clean_fixture "${fixture}"
mkdir -p "${fixture}/iris"
cat >"${fixture}/iris/extra.go" <<'EOF'
package iris

func extraSigner() {
	_ = NewHMACSigner("bad")
}
EOF
run_checker "${fixture}"
assert_failure "different production file NewHMACSigner call is rejected"
assert_contains "different production file location" "iris/extra.go:"

fixture="${TMP_ROOT}/function-value-escape"
write_clean_fixture "${fixture}"
cat >"${fixture}/internal/client/transport/escape.go" <<'EOF'
package client

var makeSigner = NewHMACSigner

func extraSigner(secret string) *hmacSigner {
	return makeSigner(secret)
}
EOF
run_checker "${fixture}"
assert_failure "NewHMACSigner function value escape is rejected"
assert_contains "NewHMACSigner escape location" "internal/client/transport/escape.go:"

fixture="${TMP_ROOT}/different-irishmac-signer-file"
write_clean_fixture "${fixture}"
mkdir -p "${fixture}/other"
cat >"${fixture}/other/signer.go" <<'EOF'
package other

import "github.com/park285/iris-client-go/internal/irishmac"

func signer() {
	_ = irishmac.NewSigner("bad")
}
EOF
run_checker "${fixture}"
assert_failure "different irishmac.NewSigner file is rejected"
assert_contains "different irishmac.NewSigner location" "other/signer.go:"

fixture="${TMP_ROOT}/aliased-irishmac-import"
write_clean_fixture "${fixture}"
mkdir -p "${fixture}/other"
cat >"${fixture}/other/alias.go" <<'EOF'
package other

import h "github.com/park285/iris-client-go/internal/irishmac"

func signer() {
	_ = h.NewSigner("bad")
}
EOF
run_checker "${fixture}"
assert_failure "aliased irishmac import is rejected"
assert_contains "aliased irishmac import location" "other/alias.go:"
assert_contains "aliased irishmac import boundary" "internal/irishmac imports are restricted"

fixture="${TMP_ROOT}/irishmac-function-value"
write_clean_fixture "${fixture}"
mkdir -p "${fixture}/other"
cat >"${fixture}/other/escape.go" <<'EOF'
package other

import "github.com/park285/iris-client-go/internal/irishmac"

var makeSigner = irishmac.NewSigner
EOF
run_checker "${fixture}"
assert_failure "irishmac.NewSigner function value escape is rejected"
assert_contains "irishmac.NewSigner function value escape location" "other/escape.go:"
assert_contains "irishmac.NewSigner function value import boundary" "internal/irishmac imports are restricted"

fixture="${TMP_ROOT}/multiline-method-helper"
write_clean_fixture "${fixture}"
cat >"${fixture}/internal/client/transport/hmac_method.go" <<'EOF'
package client

type requestSigner struct{}

func (
	r *requestSigner
) signIrisRequest(secret, method, path, timestamp, nonce, body string) (string, error) {
	return "", nil
}
EOF
run_checker "${fixture}"
assert_failure "multiline method-form signIrisRequest is rejected"
assert_contains "multiline method-form signIrisRequest location" "internal/client/transport/hmac_method.go:"

fixture="${TMP_ROOT}/comments-and-strings"
write_clean_fixture "${fixture}"
cat >"${fixture}/internal/client/transport/commentary.go" <<'EOF'
package client

const helperName = "NewHMACSigner("

// signIrisRequest( and NewHMACSigner( in comments must not count as production code.
func describeBoundary() string {
	return helperName
}
EOF
run_checker "${fixture}"
assert_success "comments and strings do not trigger HMAC boundary violations"

fixture="${TMP_ROOT}/test-files-ignored"
write_clean_fixture "${fixture}"
cat >"${fixture}/internal/client/transport/hmac_helpers_test.go" <<'EOF'
package client

func signIrisRequest(secret, method, path, timestamp, nonce, body string) (string, error) {
	_ = NewHMACSigner(secret)
	return "", nil
}
EOF
run_checker "${fixture}"
assert_success "test file helper and signer occurrences are ignored"

printf 'ok - %s cases passed\n' "${PASSED}"
