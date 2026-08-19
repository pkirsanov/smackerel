#!/usr/bin/env bash
# File: framework-validate-tier-selftest.sh
#
# Hermetic-ish selftest for the IMP-012 framework-validate tiering. Uses the
# `--list-tier` DRY-LIST mode (no checks execute) so it is fast and non-circular.
# Proves: a known core check is WOULD-RUN under --list-tier=core; a known
# non-core check is WOULD-SKIP under core but WOULD-RUN under full; both exit 0;
# and an unknown flag exits 2.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FV="$SCRIPT_DIR/framework-validate.sh"

failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

# Force source mode so the self-only checks list rather than emit their
# install-mode SKIP, keeping the listing deterministic.
export BUBBLES_FRAMEWORK_VALIDATE_MODE=source

set +e
core_list="$(bash "$FV" --list-tier=core 2>&1)"
core_rc=$?
full_list="$(bash "$FV" --list-tier=full 2>&1)"
full_rc=$?
set -e

# --- core tier: a known fast check runs, a known heavy check is skipped --------
if [[ "$core_rc" -eq 0 ]]; then
  pass "--list-tier=core exits 0 without executing checks"
else
  fail "--list-tier=core should exit 0 (got $core_rc)"
  echo "$core_list" | tail -5
fi
if grep -qE '^WOULD-RUN:.*Registry consistency' <<<"$core_list"; then
  pass "core tier WOULD-RUN a fast structural check (Registry consistency)"
else
  fail "core tier should run the Registry consistency check"
fi
if grep -qE '^WOULD-RUN:.*Scan-lib' <<<"$core_list"; then
  pass "core tier WOULD-RUN the scan-lib selftest"
else
  fail "core tier should run the scan-lib selftest"
fi
if grep -qE '^WOULD-SKIP \(non-core\):.*Finding closure selftest' <<<"$core_list"; then
  pass "core tier WOULD-SKIP a non-core check (Finding closure selftest)"
else
  fail "core tier should skip the non-core Finding closure selftest"
fi

# --- full tier: everything runs (the previously-skipped check now runs) --------
if [[ "$full_rc" -eq 0 ]] \
  && grep -qE '^WOULD-RUN:.*Finding closure selftest' <<<"$full_list" \
  && ! grep -qE '^WOULD-SKIP' <<<"$full_list"; then
  pass "full tier WOULD-RUN every check (no WOULD-SKIP lines)"
else
  fail "full tier should run every check with no skips (got rc=$full_rc)"
  grep -E '^WOULD-SKIP' <<<"$full_list" | head -3
fi

# --- unknown flag → exit 2 ----------------------------------------------------
set +e
bash "$FV" --bogus-flag >/dev/null 2>&1
bad_rc=$?
set -e
[[ "$bad_rc" -eq 2 ]] \
  && pass "an unknown framework-validate flag exits 2" \
  || fail "unknown flag should exit 2 (got $bad_rc)"

# --- IMP-049 SCOPE-3: introspection answers while the lock is held ------------
# The lock exists to stop two runs corrupting each other's shared scratch
# fixtures. An invocation that executes no check builds none, so it must not
# wait. Before SCOPE-3 both assertions below returned exit 1 with a lock error.
# The third assertion is the safety property and matters more than the first
# two: an EXECUTING run must still contend, or the bypass has been widened into
# a hole. It is gated on flock because without it the lock degrades to a no-op
# and `--tier=core` would really execute (~260s) instead of refusing.
if command -v flock >/dev/null 2>&1; then
  lock_file="${TMPDIR:-/tmp}/bubbles-framework-validate.lock"
  : >"$lock_file"
  ( flock 9; sleep 20 ) 9>"$lock_file" &
  lock_holder=$!
  sleep 2

  set +e
  # A nested run inherits the holder's marker and bypasses the lock legitimately,
  # which would make these assertions vacuous. Clear it so the probes contend.
  held_help_out="$(env -u BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD bash "$FV" --help 2>&1)"
  held_help_rc=$?
  held_list_out="$(env -u BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD bash "$FV" --list-tier=core 2>&1)"
  held_list_rc=$?
  env -u BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD bash "$FV" --tier=core >/dev/null 2>&1
  held_exec_rc=$?
  set -e

  kill "$lock_holder" 2>/dev/null || true
  wait "$lock_holder" 2>/dev/null || true

  [[ "$held_help_rc" -eq 0 && "$held_help_out" == *"Usage: framework-validate.sh"* ]] \
    && pass "--help answers while another run holds the lock" \
    || fail "--help should not block on the lock (got $held_help_rc)"

  [[ "$held_list_rc" -eq 0 && "$held_list_out" == *"WOULD-"* ]] \
    && pass "--list-tier dry-lists while another run holds the lock" \
    || fail "--list-tier should not block on the lock (got $held_list_rc)"

  [[ "$held_exec_rc" -eq 1 ]] \
    && pass "an EXECUTING run still contends for the lock" \
    || fail "executing run must still be refused while the lock is held (got $held_exec_rc)"
else
  pass "flock absent — lock-bypass assertions skipped (protection is a no-op here)"
fi

# --- IMP-049 SCOPE-3: the lock pre-scan must not drift from the parser --------
# The pre-scan names the flags that execute checks; the parser below it names
# every flag it accepts. If someone adds an executing flag to the parser and
# forgets the pre-scan, that flag silently stops taking the lock and two real
# runs can corrupt each other's fixtures. Deriving both sets from the file keeps
# the duplication honest instead of trusting a comment.
prescan_exec="$(awk '/^_fv_executes_checks=true$/,/^fi$/' "$FV" \
  | grep -E '^[[:space:]]+--tier=core \|' \
  | sed 's/).*//' \
  | grep -oE '\-\-[a-z-]+(=[a-z]+)?' | sort -u)"
# Read only the case-arm LABELS (text before the closing paren). Taking whole
# lines also swallows `${_arg#--tier=}` from an arm's body, which looks like a
# bare `--tier` flag that no arm actually accepts.
parser_exec="$(awk '/^for _arg in "\$@"; do$/,/^done$/' "$FV" \
  | grep -E '^[[:space:]]+--' \
  | sed 's/).*//' \
  | grep -oE '\-\-[a-z-]+(=[a-z]+)?' \
  | grep -vE '^--(help|list-tier)' | sort -u)"

[[ -n "$prescan_exec" && "$prescan_exec" == "$parser_exec" ]] \
  && pass "lock pre-scan lists exactly the parser's executing flags" \
  || fail "lock pre-scan drifted from the parser (prescan=[${prescan_exec//$'\n'/,}] parser=[${parser_exec//$'\n'/,}])"

if [[ "$failures" -eq 0 ]]; then
  echo "[framework-validate-tier-selftest] OK"
else
  echo "[framework-validate-tier-selftest] $failures failed"
  exit 1
fi
