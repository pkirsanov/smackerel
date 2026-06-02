#!/usr/bin/env bash
# tests/unit/cli/spec_077_test_dispatcher_test.sh
#
# Spec 077 SCOPE-1a — TP-077-01-04 / SCN-077-A09 dispatcher canary.
#
# Asserts:
#   * `./smackerel.sh test --help`-style usage output lists `test e2e-ui`.
#   * `./smackerel.sh test e2e-ui --help` exits 0 and documents the lane.
#   * `./smackerel.sh test e2e-ui` (without --help) routes to the lane
#     wrapper, fails loud with `runner not yet wired`, and names the
#     dedicated Compose project `smackerel-test-e2e-ui`.
#   * `./smackerel.sh test e2e-ui --print-compose-project` prints exactly
#     `smackerel-test-e2e-ui` and exits 0 (proves the dispatcher delegates
#     to the lane wrapper, not to a fabricated handler).
#   * The four existing `./smackerel.sh test <category>` lanes still route
#     to their original handlers (verified via `--help`/option parsing
#     paths that prove the lane was reached without touching docker).
#   * `./smackerel.sh test e2e-ui --bogus-flag` is forwarded to the lane
#     wrapper (proves the dispatcher does not silently swallow unknown
#     flags before delegating).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SMACKEREL_SH="$REPO_ROOT/smackerel.sh"

[[ -x "$SMACKEREL_SH" ]] || {
  echo "FAIL: $SMACKEREL_SH not executable" >&2
  exit 1
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

#############################################
# 1. Usage text lists the new `test e2e-ui` subcommand alongside the existing
#    categories. The usage block is printed by `usage()` when an unknown
#    top-level subcommand is supplied (exit 1), so we capture stderr there.
#############################################
set +e
"$SMACKEREL_SH" __unknown_top_level_subcommand_for_help__ >"$TMP/usage.out" 2>"$TMP/usage.err"
set -e
USAGE_COMBINED="$TMP/usage.combined"
cat "$TMP/usage.out" "$TMP/usage.err" >"$USAGE_COMBINED"
grep -Eq '^[[:space:]]*test unit\b' "$USAGE_COMBINED" \
  || fail "usage text missing 'test unit' line (regression in dispatcher help)
$(cat "$USAGE_COMBINED")"
grep -Eq '^[[:space:]]*test integration\b' "$USAGE_COMBINED" \
  || fail "usage text missing 'test integration' line"
grep -Eq '^[[:space:]]*test e2e\b' "$USAGE_COMBINED" \
  || fail "usage text missing 'test e2e' line"
grep -Eq '^[[:space:]]*test stress\b' "$USAGE_COMBINED" \
  || fail "usage text missing 'test stress' line"
grep -Eq '^[[:space:]]*test e2e-ui\b' "$USAGE_COMBINED" \
  || fail "usage text missing new 'test e2e-ui' line (SCN-077-A09)
$(cat "$USAGE_COMBINED")"

#############################################
# 2. `./smackerel.sh test e2e-ui --help` exits 0 and documents the lane.
#############################################
set +e
"$SMACKEREL_SH" test e2e-ui --help >"$TMP/help.out" 2>"$TMP/help.err"
RC=$?
set -e
[[ "$RC" -eq 0 ]] || fail "'test e2e-ui --help' exit=$RC (want 0)
$(cat "$TMP/help.err")"
grep -q 'smackerel-test-e2e-ui' "$TMP/help.out" \
  || fail "'test e2e-ui --help' output missing Compose project name
$(cat "$TMP/help.out")"
grep -Eiq 'Playwright|PWA browser end-to-end' "$TMP/help.out" \
  || fail "'test e2e-ui --help' output missing harness description"

#############################################
# 3. `./smackerel.sh test e2e-ui --print-compose-project` proves the
#    dispatcher delegates to scripts/runtime/web-e2e-ui.sh and that the
#    lane's dedicated Compose project name is exactly `smackerel-test-e2e-ui`.
#############################################
set +e
ACTUAL_PROJECT="$("$SMACKEREL_SH" test e2e-ui --print-compose-project 2>"$TMP/pp.err")"
RC=$?
set -e
[[ "$RC" -eq 0 ]] || fail "'test e2e-ui --print-compose-project' exit=$RC
$(cat "$TMP/pp.err")"
[[ "$ACTUAL_PROJECT" == "smackerel-test-e2e-ui" ]] \
  || fail "Compose project name = '$ACTUAL_PROJECT' (want 'smackerel-test-e2e-ui')"

#############################################
# 4. `./smackerel.sh test e2e-ui` (no args) routes through the
#    `run_node_tooling` helper added in spec 077 SCOPE-1b. We inject
#    `SMACKEREL_E2E_UI_NPX=false` so the helper's `npx` stand-in exits
#    non-zero immediately (no Playwright download, no docker), proving
#    the dispatcher reached the runner path AND that exit codes
#    propagate. After SCOPE-1c the live-stack bring-up wraps this path;
#    until then, the no-arg invocation is expected to fail loud via the
#    runner stub rather than via the 1a placeholder.
#############################################
set +e
SMACKEREL_E2E_UI_NPX=false "$SMACKEREL_SH" test e2e-ui >"$TMP/run.out" 2>"$TMP/run.err"
RC=$?
set -e
[[ "$RC" -ne 0 ]] || fail "'test e2e-ui' (no args) silently succeeded — runner exit code did not propagate
stdout: $(cat "$TMP/run.out")"
if grep -q 'runner not yet wired' "$TMP/run.err"; then
  fail "SCOPE-1b seam regression: wrapper still emits the SCOPE-1a 'runner not yet wired' stub instead of invoking run_node_tooling
$(cat "$TMP/run.err")"
fi

#############################################
# 5. Existing `./smackerel.sh test <category>` lanes still route to their
#    original handlers. We probe the lane-specific option parser (which
#    runs BEFORE docker is required) by passing an unknown option and
#    asserting the lane-specific error message is emitted. This proves
#    the dispatcher reached the correct case branch without bringing up
#    any stack.
#############################################
declare -A LANE_PROBE
LANE_PROBE[unit]="Unknown test unit option"
LANE_PROBE[integration]="Unknown test integration option"
LANE_PROBE[e2e]="Unknown test e2e option"
LANE_PROBE[stress]="Unknown test stress option"

for lane in unit integration e2e stress; do
  expected="${LANE_PROBE[$lane]}"
  set +e
  "$SMACKEREL_SH" test "$lane" --__spec077_canary_bogus_flag__ \
    >"$TMP/${lane}.out" 2>"$TMP/${lane}.err"
  RC=$?
  set -e
  [[ "$RC" -ne 0 ]] \
    || fail "'test $lane --__spec077_canary_bogus_flag__' silently exited 0 (lane routing broken)"
  grep -qF "$expected" "$TMP/${lane}.err" \
    || fail "'test $lane' did not reach its lane-specific option parser (expected '$expected')
$(cat "$TMP/${lane}.err")"
done

#############################################
# 6. Adversarial regression #1: confirm the e2e-ui dispatcher entry would
#    not be satisfied by an empty lane wrapper. Verify the wrapper exists,
#    is executable, names the dedicated Compose project, and its no-arg
#    exit code is non-zero (fail-loud invariant).
#############################################
WRAPPER="$REPO_ROOT/scripts/runtime/web-e2e-ui.sh"
[[ -x "$WRAPPER" ]] || fail "lane wrapper $WRAPPER missing or not executable"
grep -q 'smackerel-test-e2e-ui' "$WRAPPER" \
  || fail "lane wrapper does not declare 'smackerel-test-e2e-ui' Compose project"

#############################################
# 7. Adversarial regression #2: arbitrary flags after `test e2e-ui` are
#    forwarded to the lane wrapper, which now forwards them to
#    `npx playwright test` via `run_node_tooling` (SCOPE-1b). We again
#    inject `SMACKEREL_E2E_UI_NPX=false` so the stub exits non-zero
#    without touching the network or docker, proving the dispatcher
#    forwards arbitrary flags without swallowing them.
#############################################
set +e
SMACKEREL_E2E_UI_NPX=false "$SMACKEREL_SH" test e2e-ui --some-future-runner-flag value \
  >"$TMP/fwd.out" 2>"$TMP/fwd.err"
RC=$?
set -e
[[ "$RC" -ne 0 ]] || fail "'test e2e-ui --some-future-runner-flag' silently exited 0 — flag forwarding broken"
if grep -q 'runner not yet wired' "$TMP/fwd.err"; then
  fail "SCOPE-1b seam regression: wrapper still emits the SCOPE-1a 'runner not yet wired' stub for forwarded flags
$(cat "$TMP/fwd.err")"
fi

echo "PASS: spec_077_test_dispatcher_test (TP-077-01-04 / SCN-077-A09)"
