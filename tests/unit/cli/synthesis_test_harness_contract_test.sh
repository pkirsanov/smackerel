#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
DISPATCH="$REPO_ROOT/smackerel.sh"
PY_UNIT="$REPO_ROOT/scripts/runtime/python-unit.sh"
PY_INTEGRATION="$REPO_ROOT/scripts/runtime/python-integration.sh"
GO_E2E="$REPO_ROOT/scripts/runtime/go-e2e.sh"
DEADLETTER_TEST="$REPO_ROOT/ml/tests/integration/test_deadletter_parity.py"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

grep -qF 'e2e_down_test_stack "before targeted shared-stack shell E2E"' "$DISPATCH" \
  || fail "targeted shared-stack E2E no longer pre-cleans before stack boot"
# Match literal source text rather than expanding the variables here.
# shellcheck disable=SC2016
grep -qF '"$SCRIPT_DIR/smackerel.sh" --env test up' "$DISPATCH" \
  || fail "targeted shared-stack E2E no longer boots the disposable test stack"
# Match literal source text rather than expanding the variables here.
# shellcheck disable=SC2016
grep -qF 'env E2E_STACK_MANAGED=1 bash "$SCRIPT_DIR/tests/e2e/$SHELL_E2E_RUN_TARGET"' "$DISPATCH" \
  || fail "targeted shared-stack E2E no longer marks the child as parent-managed"
if grep -qF 'test_synthesis.sh' "$DISPATCH"; then
  fail "test_synthesis.sh must not be classified as a lifecycle test"
fi

grep -qF 'not integration and not live_ollama' "$PY_UNIT" \
  || fail "Python unit lane no longer excludes live integration and external Ollama markers"
grep -qF 'pytest -q -m integration ml/tests/integration' "$PY_INTEGRATION" \
  || fail "Python integration lane no longer executes the required integration marker"
grep -qF 'python-integration.sh' "$DISPATCH" \
  || fail "canonical integration dispatcher no longer runs Python integration tests"
grep -qF 'go_test_args=(-p 1 -tags e2e' "$GO_E2E" \
  || fail "Go E2E packages no longer serialize access to shared disposable identities"

if grep -Eq 'pytest\.mark\.skipif|pytest\.skip\(' "$DEADLETTER_TEST"; then
  fail "required dead-letter integration test contains a skip path"
fi
grep -qF 'raise RuntimeError' "$DEADLETTER_TEST" \
  || fail "required dead-letter integration prerequisites no longer fail loud"

echo "PASS: synthesis test harness preserves stack lifecycle and zero-skip category boundaries"
