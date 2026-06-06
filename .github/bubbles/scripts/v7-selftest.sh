#!/usr/bin/env bash
# bubbles/scripts/v7-selftest.sh
#
# v7.0 selftest — bare v5 mode-name input removal + grandfather.
#
# v7.0 removes bare v5 mode NAMES as operator input. They remain the canonical
# registry keys (state.json.workflowMode stores them; guards resolve ceilings by
# direct registry lookup), so existing artifacts are unaffected. This selftest
# asserts the rejection-for-new-input / grandfather-for-persisted-modes contract.
#
# Assertions:
#   T1. A bare v5 mode name is rejected (exit 3) with a v6-form hint.
#   T2. The v6 primitive+tag form resolves cleanly.
#   T3. BUBBLES_MODE_GRANDFATHER=1 resolves a stored v5 key with a deprecation notice.
#   T4. The --grandfather flag resolves a stored v5 key.
#   T5. A persisted v5-key mode still resolves its status ceiling through the guards.
#   T6. The alias table is structurally intact — a v6 form reverse-resolves to the key.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVER="$SCRIPT_DIR/mode-resolver.sh"
IS_TERMINAL="$SCRIPT_DIR/is-terminal-for-mode.sh"

failures=0
pass() { echo "PASS: $*"; }
fail() { echo "FAIL: $*"; failures=$((failures + 1)); }

# A representative v5 mode name that has a v6 alias form.
V5_MODE="bugfix-fastlane"

[[ -x "$RESOLVER" ]] || { echo "v7-selftest: missing resolver: $RESOLVER" >&2; exit 2; }
command -v yq >/dev/null 2>&1 || { echo "v7-selftest: yq required" >&2; exit 2; }

# T1: bare v5 name is rejected (exit 3) with a v6-form hint.
t1_err="$(bash "$RESOLVER" "$V5_MODE" 2>&1 >/dev/null)"
t1_rc=$?
if [[ $t1_rc -eq 3 ]] && grep -qi "removed in v7" <<<"$t1_err" && grep -q "fix" <<<"$t1_err"; then
  pass "T1: bare v5 name '$V5_MODE' rejected (exit 3) with v6-form hint"
else
  fail "T1: expected exit 3 + v6 hint for bare v5 name; got rc=$t1_rc err=${t1_err:0:160}"
fi

# T2: the v6 primitive+tag form resolves cleanly.
t2_out="$(bash "$RESOLVER" fix target:bug action:fastlane 2>/dev/null)"
t2_rc=$?
if [[ $t2_rc -eq 0 ]] && grep -q 'statusCeiling' <<<"$t2_out"; then
  pass "T2: v6 form 'fix target:bug action:fastlane' resolves"
else
  fail "T2: v6 form did not resolve; rc=$t2_rc"
fi

# T3: grandfather env resolves the stored v5 key with a deprecation notice.
t3_out="$(BUBBLES_MODE_GRANDFATHER=1 bash "$RESOLVER" "$V5_MODE" 2>/dev/null)"
t3_rc=$?
t3_err="$(BUBBLES_MODE_GRANDFATHER=1 bash "$RESOLVER" "$V5_MODE" 2>&1 >/dev/null)"
if [[ $t3_rc -eq 0 ]] && grep -q 'statusCeiling' <<<"$t3_out" && grep -qi 'grandfather' <<<"$t3_err"; then
  pass "T3: BUBBLES_MODE_GRANDFATHER=1 resolves '$V5_MODE' with deprecation notice"
else
  fail "T3: grandfather env did not resolve with notice; rc=$t3_rc"
fi

# T4: --grandfather flag resolves the stored v5 key.
t4_out="$(bash "$RESOLVER" --grandfather "$V5_MODE" 2>/dev/null)"
t4_rc=$?
if [[ $t4_rc -eq 0 ]] && grep -q 'statusCeiling' <<<"$t4_out"; then
  pass "T4: --grandfather flag resolves '$V5_MODE'"
else
  fail "T4: --grandfather flag did not resolve; rc=$t4_rc"
fi

# T5: a persisted v5-key mode still resolves its ceiling through the guards.
if [[ -x "$IS_TERMINAL" ]]; then
  bash "$IS_TERMINAL" done "$V5_MODE" >/dev/null 2>&1
  t5_done=$?
  bash "$IS_TERMINAL" in_progress "$V5_MODE" >/dev/null 2>&1
  t5_prog=$?
  if [[ $t5_done -eq 0 && $t5_prog -ne 0 ]]; then
    pass "T5: persisted v5-key mode '$V5_MODE' resolves ceiling (done terminal, in_progress not)"
  else
    fail "T5: persisted-mode ceiling resolution broke; done=$t5_done in_progress=$t5_prog"
  fi
else
  pass "T5: is-terminal-for-mode.sh not present (skipped)"
fi

# T6: alias table intact — v6 form reverse-resolves to the registry key.
t6_out="$(bash "$RESOLVER" --resolve-v6 fix target:bug action:fastlane 2>/dev/null)"
t6_rc=$?
if [[ $t6_rc -eq 0 && "$t6_out" == "$V5_MODE" ]]; then
  pass "T6: alias table intact — v6 form reverse-resolves to registry key '$V5_MODE'"
else
  fail "T6: --resolve-v6 did not map back to '$V5_MODE'; rc=$t6_rc got='$t6_out'"
fi

if [[ $failures -eq 0 ]]; then
  echo "v7-selftest passed."
  exit 0
fi
echo "v7-selftest FAILED with $failures issue(s)."
exit 1
