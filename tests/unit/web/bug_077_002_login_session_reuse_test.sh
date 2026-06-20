#!/usr/bin/env bash
# Spec 077 e2e-ui lane — BUG-002 regression driver.
#
# Runs the Node-level regression test
# `web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts` under
# `node --experimental-strip-types --test`. Auto-discovered by
# `./smackerel.sh test unit` (tests/unit/web/*.sh discovery, spec 077 SCOPE-2),
# so no dispatcher edit is required.
#
# Provides SMACKEREL_AUTH_TOKEN in the environment because the shared
# `web/pwa/tests/_support/cardrewards.ts` helper resolves
# `requireAuthToken()` from a module-load const — setting it inside the test
# body would be too late.
#
# Anchors SCN-077-BUG-002-01 (worker session reuse) and SCN-077-BUG-002-02
# (no per-test /v1/web/login POST reintroduced in any cardrewards_*.spec.ts).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
NODE_TEST="$REPO_ROOT/web/pwa/tests/_support/cardrewards_login_session_reuse.test.ts"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

[[ -f "$NODE_TEST" ]] || fail "BUG-002 regression test missing: $NODE_TEST"

if ! command -v node >/dev/null 2>&1; then
  fail "node is required to run the BUG-002 login-session-reuse regression test"
fi
echo "[bug_077_002_login_session_reuse] node $(node --version)"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

set +e
SMACKEREL_AUTH_TOKEN="stub-dev-token-bug002" node \
  --experimental-strip-types \
  --no-warnings=ExperimentalWarning \
  --test "$NODE_TEST" >"$TMP/node.out" 2>"$TMP/node.err"
RC=$?
set -e

cat "$TMP/node.out"
if [[ "$RC" -ne 0 ]]; then
  echo "----- node stderr -----" >&2
  cat "$TMP/node.err" >&2
  fail "BUG-002 regression node:test run failed (exit=$RC) on $NODE_TEST"
fi

# Sanity: ensure the runner actually executed our tests (not a zero-test silent
# pass). Node's TAP summary prints '# tests N' / '# pass N'.
grep -Eq '^# tests [1-9]' "$TMP/node.out" \
  || fail "node:test reported zero tests for $NODE_TEST"
grep -Eq '^# pass 2' "$TMP/node.out" \
  || fail "expected exactly 2 passing BUG-002 regression tests in $NODE_TEST"

echo "PASS: bug_077_002_login_session_reuse_test (SCN-077-BUG-002-01 / SCN-077-BUG-002-02)"
