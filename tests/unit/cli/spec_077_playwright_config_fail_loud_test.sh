#!/usr/bin/env bash
# tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh
#
# Spec 077 SCOPE-1b — TP-077-01-03 / SCN-077-A10 fail-loud unit.
#
# This shell driver is auto-discovered by `./smackerel.sh test unit`
# (tests/unit/cli/*.sh discovery shipped in SCOPE-1a) and combines
# three assertion blocks that together prove the SCN-077-A10 contract:
#
#   A. Static composition — `web/pwa/playwright.config.ts` sources
#      `baseURL` exclusively from `requireSmackerelBaseUrl()` and carries
#      no `??` / `||` / hardcoded localhost / port default, satisfying
#      the NO-DEFAULTS SST policy.
#
#   B. Node behavioral test — `web/pwa/tests/_support/csp.test.ts`,
#      executed under `node --experimental-strip-types --test`, asserts
#      that `requireSmackerelBaseUrl()` throws an Error naming
#      `SMACKEREL_BASE_URL` when the env var is unset or empty, returns
#      the value when set, and that the `attachCSPGuard(page)` skeleton
#      compiles + exposes a one-parameter function.
#
#   C. Exit-code propagation — sourcing `scripts/runtime/web-e2e-ui.sh`
#      and calling `run_node_tooling` with `SMACKEREL_E2E_UI_NPX` pointed
#      at a stub that exits with a configurable code (0, 7, 127) proves
#      the helper propagates the exit code unchanged.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
CONFIG="$REPO_ROOT/web/pwa/playwright.config.ts"
ENV_TS="$REPO_ROOT/web/pwa/tests/_support/env.ts"
CSP_TS="$REPO_ROOT/web/pwa/tests/_support/csp.ts"
NODE_TEST="$REPO_ROOT/web/pwa/tests/_support/csp.test.ts"
WRAPPER="$REPO_ROOT/scripts/runtime/web-e2e-ui.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

#############################################
# A. Static composition checks on playwright.config.ts and env.ts.
#############################################
for f in "$CONFIG" "$ENV_TS" "$CSP_TS" "$NODE_TEST" "$WRAPPER"; do
  [[ -f "$f" ]] || fail "required spec-077 SCOPE-1b artifact missing: $f"
done

# Config must reference SMACKEREL_BASE_URL via the fail-loud helper, not
# directly via process.env (which would invite a `??` / `||` default).
grep -q 'requireSmackerelBaseUrl' "$CONFIG" \
  || fail "playwright.config.ts does not invoke requireSmackerelBaseUrl()
$(cat "$CONFIG")"

# Forbidden default patterns: nullish-coalescing (??), logical-or (||),
# any hardcoded http(s) URL literal, any explicit 'localhost'/'127.0.0.1'
# substitution, or a bare process.env.SMACKEREL_BASE_URL read (which
# would bypass the fail-loud helper).
if grep -nE 'SMACKEREL_BASE_URL[[:space:]]*(\?\?|\|\|)' "$CONFIG" "$ENV_TS"; then
  fail "found forbidden ?? or || default near SMACKEREL_BASE_URL"
fi
if grep -nE 'process\.env\.SMACKEREL_BASE_URL[[:space:]]*(\?\?|\|\|)' "$CONFIG" "$ENV_TS"; then
  fail "found forbidden default substitution on process.env.SMACKEREL_BASE_URL"
fi
if grep -nE 'baseURL[[:space:]]*:[[:space:]]*"https?://' "$CONFIG"; then
  fail "playwright.config.ts has a hardcoded baseURL literal"
fi
if grep -nE 'baseURL[[:space:]]*:[[:space:]]*(.+\|\||.+\?\?)' "$CONFIG"; then
  fail "playwright.config.ts uses a fallback expression for baseURL"
fi
if grep -nE 'process\.env\.SMACKEREL_BASE_URL' "$CONFIG"; then
  fail "playwright.config.ts reads process.env.SMACKEREL_BASE_URL directly (must go through requireSmackerelBaseUrl)"
fi

# env.ts must throw with SMACKEREL_BASE_URL named in the message.
grep -q 'throw new Error' "$ENV_TS" \
  || fail "env.ts does not throw an Error on missing SMACKEREL_BASE_URL"
grep -q 'SMACKEREL_BASE_URL' "$ENV_TS" \
  || fail "env.ts throw does not name SMACKEREL_BASE_URL"

#############################################
# B. Node behavioral test (node:test runner, strip-types).
#############################################
if ! command -v node >/dev/null 2>&1; then
  fail "node is required to run the spec 077 SCOPE-1b fail-loud unit"
fi

NODE_VERSION="$(node --version)"
echo "[spec_077_playwright_config_fail_loud] node $NODE_VERSION"

# Run with a clean env so SMACKEREL_BASE_URL is reliably unset for the
# "throws when unset" case; the test re-sets it locally where needed.
set +e
env -u SMACKEREL_BASE_URL node \
  --experimental-strip-types \
  --no-warnings=ExperimentalWarning \
  --test "$NODE_TEST" >"$TMP/node.out" 2>"$TMP/node.err"
RC=$?
set -e
if [[ "$RC" -ne 0 ]]; then
  echo "----- node stdout -----" >&2
  cat "$TMP/node.out" >&2
  echo "----- node stderr -----" >&2
  cat "$TMP/node.err" >&2
  fail "node:test run failed (exit=$RC) on $NODE_TEST"
fi
# Sanity: ensure the runner actually executed our tests (not zero-test
# silent pass). Node test reporter prints '# tests N' summary.
if ! grep -Eq '^# tests [1-9]' "$TMP/node.out"; then
  echo "----- node stdout -----" >&2
  cat "$TMP/node.out" >&2
  fail "node:test reported zero tests for $NODE_TEST"
fi

#############################################
# C. run_node_tooling exit-code propagation.
#############################################
# shellcheck source=/dev/null
source "$WRAPPER"

if ! declare -F run_node_tooling >/dev/null; then
  fail "sourcing $WRAPPER did not expose run_node_tooling()"
fi

# Stub npx that records args and exits with the code in $STUB_EXIT.
STUB_NPX="$TMP/stub-npx.sh"
cat >"$STUB_NPX" <<'STUB'
#!/usr/bin/env bash
echo "stub-npx invoked: $*"
exit "${STUB_EXIT:-0}"
STUB
chmod +x "$STUB_NPX"

for code in 0 7 127; do
  set +e
  STUB_EXIT="$code" SMACKEREL_E2E_UI_NPX="$STUB_NPX" \
    run_node_tooling --some-flag >"$TMP/rt.$code.out" 2>"$TMP/rt.$code.err"
  rc=$?
  set -e
  [[ "$rc" -eq "$code" ]] \
    || fail "run_node_tooling did not propagate stub exit code (got $rc, want $code)
stdout: $(cat "$TMP/rt.$code.out")
stderr: $(cat "$TMP/rt.$code.err")"
  grep -q 'stub-npx invoked: playwright test --some-flag' "$TMP/rt.$code.out" \
    || fail "run_node_tooling did not forward 'playwright test' + args to npx stub
$(cat "$TMP/rt.$code.out")"
done

# Adversarial: when SMACKEREL_E2E_UI_NPX points at a non-existent binary,
# the helper must fail loud (exit 127), not silently no-op.
set +e
SMACKEREL_E2E_UI_NPX="/definitely/not/a/real/binary/$$" \
  run_node_tooling >"$TMP/rt.missing.out" 2>"$TMP/rt.missing.err"
rc=$?
set -e
[[ "$rc" -eq 127 ]] \
  || fail "run_node_tooling did not return 127 for missing npx binary (got $rc)
stderr: $(cat "$TMP/rt.missing.err")"
grep -q 'is required to run the spec 077 PWA e2e-ui harness' "$TMP/rt.missing.err" \
  || fail "missing-npx error message lacks the fail-loud reference
$(cat "$TMP/rt.missing.err")"

echo "PASS: spec_077_playwright_config_fail_loud_test (TP-077-01-03 / SCN-077-A10)"
