#!/usr/bin/env bash
# Hermetic regression selftest for bubbles_run_with_timeout's fallback watchdog.
#
# Guards the OW-009 defect: on a host with no coreutils `timeout`/`gtimeout`
# (a stock macOS PATH), the fallback watchdog used to be
#
#     ( sleep "$secs"; kill -TERM "$cmd_pid" 2>/dev/null ) &
#
# which is wrong in two independent ways:
#
#   1. The background subshell INHERITS the caller's stdout pipe. When the
#      caller is a command substitution -- state-transition-guard.sh line ~3208
#      does `reality_output="$(bubbles_run_with_timeout 120 ...)"` -- the
#      substitution reads until EOF, and EOF cannot arrive while the watchdog
#      holds the write end. An instantly-returning command therefore blocked for
#      the FULL timeout. That is the reported 129s state-transition-guard run on
#      a stock macOS PATH versus 9s with MacPorts GNU tools on PATH.
#
#   2. `kill -TERM "$watch_pid"` kills the SUBSHELL, not its `sleep` grandchild,
#      so a single long sleep survives as an orphan for its whole duration.
#
# These three cases fail against the old implementation and pass against the
# fixed one. Case A is the adversarial case: it is the one that would regress if
# the /dev/null redirection were ever removed as "noise".
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/guard-lib.sh"

# Force the fallback path regardless of what the host actually has installed, so
# this test exercises the macOS-shaped branch on every platform including CI.
command() {
  if [ "${1:-}" = "-v" ] && { [ "${2:-}" = "timeout" ] || [ "${2:-}" = "gtimeout" ]; }; then
    return 1
  fi
  builtin command "$@"
}

failures=0
pass() { printf '  PASS  %s\n' "$1"; }
fail() { printf '  FAIL  %s\n' "$1"; failures=$((failures + 1)); }

# --- Case A: instant command inside a command substitution returns instantly ---
# Against the old watchdog this took the full 20s.
start=$(date +%s)
captured="$(bubbles_run_with_timeout 20 bash -c 'echo instant-result')"
elapsed=$(($(date +%s) - start))
if [ "$captured" = "instant-result" ] && [ "$elapsed" -le 3 ]; then
  pass "instant command in \$( ) returns promptly (${elapsed}s, output intact)"
else
  fail "instant command in \$( ) took ${elapsed}s (expected <=3s), captured=[$captured]"
fi

# --- Case B: the timeout still actually fires, with GNU timeout's exit code ---
start=$(date +%s)
bubbles_run_with_timeout 3 bash -c 'sleep 30' >/dev/null 2>&1
rc=$?
elapsed=$(($(date +%s) - start))
if [ "$rc" -eq 124 ] && [ "$elapsed" -le 8 ]; then
  pass "timeout fires and normalizes to 124 (${elapsed}s)"
else
  fail "timeout returned rc=$rc after ${elapsed}s (expected rc=124 within 8s)"
fi

# --- Case C: a non-zero exit code from the command is preserved, not masked ---
start=$(date +%s)
bubbles_run_with_timeout 20 bash -c 'exit 7' >/dev/null 2>&1
rc=$?
elapsed=$(($(date +%s) - start))
if [ "$rc" -eq 7 ] && [ "$elapsed" -le 3 ]; then
  pass "command exit code preserved (rc=$rc, ${elapsed}s)"
else
  fail "expected rc=7 within 3s, got rc=$rc after ${elapsed}s"
fi

if [ "$failures" -ne 0 ]; then
  printf 'guard-lib timeout selftest: %d failure(s)\n' "$failures"
  exit 1
fi
printf 'guard-lib timeout selftest: OK (3 cases)\n'
exit 0
