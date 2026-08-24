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
# Cases A-D pin the portable fixed-timeout fallback, including trappable SIGINT.
# Cases E-J pin the progress-aware runner used by nested downstream validation:
# active output extends the idle window, silence and endless output stay bounded,
# command status is preserved, and timeout/error descendants are reaped.
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
tmp_root="$(mktemp -d)"
trap 'rm -rf "$tmp_root"' EXIT INT TERM

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
  # portable-ok: assertion prose only; no raw timeout command is invoked here.
  pass "timeout fires and normalizes to 124 (${elapsed}s)"
else
  # portable-ok: assertion prose only; no raw timeout command is invoked here.
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

# --- Case D: fallback children inherit a trappable SIGINT disposition -------
# Without monitor mode around the asynchronous launch, Bash starts the command
# with SIGINT ignored. A nested non-interactive Bash cannot undo that inherited
# disposition, so its INT trap never runs and the watchdog eventually returns
# 124 instead of the command's intended 130.
signal_fifo="$tmp_root/sigint-ready"
mkfifo "$signal_fifo"
(
  IFS= read -r signal_pid < "$signal_fifo"
  kill -INT "$signal_pid"
) &
signaler_pid=$!
start=$(date +%s)
bubbles_run_with_timeout 10 bash -c '
  trap "exit 130" INT
  printf "%s\n" "$$" > "$1"
  while :; do :; done
' bash "$signal_fifo" >/dev/null 2>&1
rc=$?
elapsed=$(($(date +%s) - start))
wait "$signaler_pid" 2>/dev/null || true
if [ "$rc" -eq 130 ] && [ "$elapsed" -le 3 ]; then
  pass "fallback child can trap SIGINT (rc=$rc, ${elapsed}s)"
else
  fail "fallback child SIGINT returned rc=$rc after ${elapsed}s (expected rc=130 within 3s)"
fi

progress_root="$tmp_root/progress"
mkdir -p "$progress_root"

# --- Case E: regular output resets the idle deadline -------------------------
progress_log="$progress_root/progress.log"
start=$(date +%s)
bubbles_run_with_progress_timeout 2 8 "$progress_log" \
  bash -c 'for value in 1 2 3 4; do echo "$value"; sleep 1; done'
rc=$?
elapsed=$(($(date +%s) - start))
if [ "$rc" -eq 0 ] && [ "$elapsed" -ge 3 ] && [ "$elapsed" -le 7 ] &&
  [ "$(wc -l < "$progress_log")" -eq 4 ]; then
  pass "progress extends the idle window without exceeding the absolute ceiling (${elapsed}s)"
else
  fail "progress-aware command returned rc=$rc after ${elapsed}s"
fi

# --- Case F: a silent command is stopped by the idle deadline ----------------
idle_log="$progress_root/idle.log"
start=$(date +%s)
bubbles_run_with_progress_timeout 2 8 "$idle_log" bash -c 'sleep 30'
rc=$?
elapsed=$(($(date +%s) - start))
if [ "$rc" -eq 124 ] && [ "$BUBBLES_PROGRESS_TIMEOUT_REASON" = "idle" ] &&
  [ "$elapsed" -le 5 ]; then
  pass "silent command stops at the idle deadline with rc=124 (${elapsed}s)"
else
  # portable-ok: assertion prose only; no raw timeout command is invoked here.
  fail "idle timeout returned rc=$rc reason=${BUBBLES_PROGRESS_TIMEOUT_REASON:-none} after ${elapsed}s"
fi

# --- Case G: endless progress is stopped by the absolute deadline ------------
absolute_log="$progress_root/absolute.log"
start=$(date +%s)
bubbles_run_with_progress_timeout 2 4 "$absolute_log" \
  bash -c 'while true; do echo progress; sleep 1; done'
rc=$?
elapsed=$(($(date +%s) - start))
if [ "$rc" -eq 125 ] && [ "$BUBBLES_PROGRESS_TIMEOUT_REASON" = "absolute" ] &&
  [ "$elapsed" -ge 3 ] && [ "$elapsed" -le 7 ]; then
  pass "chatty command stops at the absolute deadline with rc=125 (${elapsed}s)"
else
  # portable-ok: assertion prose only; no raw timeout command is invoked here.
  fail "absolute timeout returned rc=$rc reason=${BUBBLES_PROGRESS_TIMEOUT_REASON:-none} after ${elapsed}s"
fi

# --- Case H: progress runner preserves a non-zero command exit ----------------
exit_log="$progress_root/exit.log"
bubbles_run_with_progress_timeout 2 8 "$exit_log" bash -c 'echo failing; exit 9'
rc=$?
if [ "$rc" -eq 9 ] && [ -z "$BUBBLES_PROGRESS_TIMEOUT_REASON" ]; then
  pass "progress runner preserves command exit code (rc=$rc)"
else
  # portable-ok: assertion prose only; no raw timeout command is invoked here.
  fail "progress runner expected rc=9 with no timeout reason, got rc=$rc reason=${BUBBLES_PROGRESS_TIMEOUT_REASON:-none}"
fi

# --- Case I: timeout terminates the complete validator process group ---------
group_log="$progress_root/group.log"
descendant_pid_file="$progress_root/descendant.pid"
start=$(date +%s)
bubbles_run_with_progress_timeout 2 4 "$group_log" \
  bash -c 'trap "" TERM; (trap "" TERM; sleep 30) & echo "$!" > "$1"; while true; do echo progress; sleep 1; done' \
  _ "$descendant_pid_file"
rc=$?
elapsed=$(($(date +%s) - start))
descendant_pid="$(cat "$descendant_pid_file" 2>/dev/null || true)"
if [ "$rc" -eq 125 ] && [ "$BUBBLES_PROGRESS_TIMEOUT_REASON" = "absolute" ] &&
  [ -n "$descendant_pid" ] && ! kill -0 "$descendant_pid" 2>/dev/null &&
  [ "$elapsed" -le 12 ]; then
  # portable-ok: assertion prose only; no raw timeout command is invoked here.
  pass "absolute timeout force-terminates a TERM-resistant validator process group (${elapsed}s)"
else
  fail "timed-out validator leaked or blocked: pid=${descendant_pid:-missing} rc=$rc reason=${BUBBLES_PROGRESS_TIMEOUT_REASON:-none} elapsed=${elapsed}s"
  [ -z "$descendant_pid" ] || kill -KILL "$descendant_pid" 2>/dev/null || true
fi

# --- Case J: a lost progress log fails loud through the same cleanup path ----
lost_log="$progress_root/lost.log"
start=$(date +%s)
bubbles_run_with_progress_timeout 2 8 "$lost_log" \
  bash -c 'rm -f "$1"; while true; do sleep 1; done' _ "$lost_log"
rc=$?
elapsed=$(($(date +%s) - start))
if [ "$rc" -eq 2 ] && [ "$elapsed" -le 5 ]; then
  pass "lost progress log fails loud through bounded cleanup (${elapsed}s)"
else
  fail "lost progress log returned rc=$rc after ${elapsed}s (expected rc=2 within 5s)"
fi

if [ "$failures" -ne 0 ]; then
  # portable-ok: selftest verdict label only; no raw timeout command is invoked here.
  printf 'guard-lib timeout selftest: %d failure(s)\n' "$failures"
  exit 1
fi
# portable-ok: selftest verdict label only; no raw timeout command is invoked here.
printf 'guard-lib timeout selftest: OK (10 cases)\n'
exit 0
