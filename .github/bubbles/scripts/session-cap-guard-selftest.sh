#!/usr/bin/env bash
set -euo pipefail

# session-cap-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/session-cap-guard.sh`
# (Gate G128 — session_cap_enforcement_gate).
#
# Builds a private mktemp Bubbles-repo surface (no edits to the host repo),
# stages fixture session files in its `.specify/memory/` directory, invokes
# the guard with explicit `BUBBLES_REPO_ROOT`, and asserts exit codes plus
# stdout/stderr fingerprints. Covers BOTH directions (clean → 0, breach → 1)
# for every enforced dimension plus the no-op defaults and the exit-2
# malformed/bypass cases.
#
# Scenarios:
#   S0: no session.json                          → exit 0 (no-op)
#   Sa: session.json without sessionBudget        → exit 0 (no-op)
#   Sb: sessionBudget with all-null caps          → exit 0 (no-op)
#   Sc: conv cap set, aggregate UNDER cap         → exit 0
#   Sd: conv cap set, aggregate OVER cap across
#       TWO specs (proves aggregate, not per-spec) → exit 1, names G128 +
#                                                    "convergence"
#   Se: malformed session.json                    → exit 2
#   Sf: --skip bypass flag rejected               → exit 2
#   Sg: wall-clock minutes OVER cap               → exit 1, names
#                                                    "wallClockMinutes"
#   Sh: tool calls OVER cap                        → exit 1, names "toolCalls"
#   Si: caps set but usage data absent            → exit 0 (unmeasurable
#                                                    dimensions skipped)
#   Sj: unexpected positional argument rejected    → exit 2
#
# Reference:
#   improvements/IMP-003-autonomy-dial-and-safety-caps.md (SCOPE-2)

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd -P)"
GUARD_SCRIPT="$SCRIPT_DIR/session-cap-guard.sh"

if [[ ! -x "$GUARD_SCRIPT" ]]; then
  echo "session-cap-guard-selftest: guard script not executable: $GUARD_SCRIPT" >&2
  exit 2
fi

# --- Hermetic workspace --------------------------------------------------

WORKSPACE="$(mktemp -d -t bubbles-session-cap-selftest-XXXXXXXX)"
cleanup() {
  rm -rf "$WORKSPACE"
}
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
declare -a FAILED_SCENARIOS=()

note() { printf '[selftest] %s\n' "$*"; }
ok()   { printf '[selftest] PASS: %s\n' "$*"; PASS_COUNT=$((PASS_COUNT + 1)); }
ko()   {
  printf '[selftest] FAIL: %s\n' "$*" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
  FAILED_SCENARIOS+=("$1")
}

# --- Stage a minimal fake "Bubbles" repo surface inside WORKSPACE --------
#
# G128 reads ONLY the session file (caps come from `sessionBudget`, NOT from
# workflows.yaml), so a bare `.specify/memory/` directory is all we need.
# The selftest stages files INSIDE its own mktemp workspace via heredocs —
# allowed by terminal-discipline policy (the workspace is throwaway and
# never becomes part of the working tree).

stage_repo_root() {
  local root="$1"
  mkdir -p "$root/.specify/memory"
}

write_session_json() {
  local root="$1"
  local payload="$2"
  printf '%s\n' "$payload" > "$root/.specify/memory/bubbles.session.json"
}

# --- Helper: run guard, capture exit + stdout + stderr -------------------

run_guard() {
  local root="$1"; shift
  local stdout_file="$WORKSPACE/stdout.last"
  local stderr_file="$WORKSPACE/stderr.last"

  set +e
  BUBBLES_REPO_ROOT="$root" bash "$GUARD_SCRIPT" "$@" \
    > "$stdout_file" \
    2> "$stderr_file"
  local rc=$?
  set -e

  printf '%s\n' "$rc" > "$WORKSPACE/exit.last"
}

last_exit()   { cat "$WORKSPACE/exit.last"; }
last_stdout() { cat "$WORKSPACE/stdout.last"; }
last_stderr() { cat "$WORKSPACE/stderr.last"; }

assert_exit() {
  local expected="$1"
  local label="$2"
  local actual
  actual="$(last_exit)"
  if [[ "$actual" != "$expected" ]]; then
    ko "$label: expected exit $expected, got $actual"
    echo "  --- stdout ---" >&2
    last_stdout >&2
    echo "  --- stderr ---" >&2
    last_stderr >&2
    return 1
  fi
  ok "$label: exit $expected"
}

assert_stdout_contains() {
  local needle="$1"
  local label="$2"
  if ! grep -Fq -- "$needle" "$WORKSPACE/stdout.last"; then
    ko "$label: stdout did not contain '$needle'"
    echo "  --- stdout ---" >&2
    last_stdout >&2
    return 1
  fi
  ok "$label: stdout contains '$needle'"
}

assert_stderr_contains() {
  local needle="$1"
  local label="$2"
  if ! grep -Fq -- "$needle" "$WORKSPACE/stderr.last"; then
    ko "$label: stderr did not contain '$needle'"
    echo "  --- stderr ---" >&2
    last_stderr >&2
    return 1
  fi
  ok "$label: stderr contains '$needle'"
}

# =============================================================================
# Scenario S0: no session.json -> exit 0 (no-op)
# =============================================================================

note "Scenario S0: no session.json should pass with exit 0 (no-op)"

S0_ROOT="$WORKSPACE/s0"
stage_repo_root "$S0_ROOT"
# Intentionally NO session file written.

run_guard "$S0_ROOT"

assert_exit 0 "S0 exit code"
assert_stdout_contains "PASS Gate G128" "S0 PASS marker on stdout"
assert_stdout_contains "no session budget recorded" "S0 reports no session.json"

# =============================================================================
# Scenario Sa: session.json without sessionBudget -> exit 0 (no-op)
# =============================================================================

note "Scenario Sa: session.json without sessionBudget should pass (no-op)"

SA_ROOT="$WORKSPACE/sa"
stage_repo_root "$SA_ROOT"
write_session_json "$SA_ROOT" '{
  "convergenceLoops": [
    { "specDir": "specs/900-a", "agent": "bubbles.workflow", "iterationCount": 99 }
  ]
}'

run_guard "$SA_ROOT"

assert_exit 0 "Sa exit code"
assert_stdout_contains "no sessionBudget recorded" "Sa reports no sessionBudget"

# =============================================================================
# Scenario Sb: sessionBudget with all-null caps -> exit 0 (no-op)
# =============================================================================

note "Scenario Sb: sessionBudget with all-null caps should pass (no-op)"

SB_ROOT="$WORKSPACE/sb"
stage_repo_root "$SB_ROOT"
write_session_json "$SB_ROOT" '{
  "sessionBudget": {
    "maxTotalConvergenceIterations": null,
    "maxWallClockMinutes": null,
    "maxToolCalls": null
  },
  "convergenceLoops": [
    { "specDir": "specs/900-a", "agent": "bubbles.workflow", "iterationCount": 99 }
  ]
}'

run_guard "$SB_ROOT"

assert_exit 0 "Sb exit code"
assert_stdout_contains "no non-null cap" "Sb reports no non-null cap"

# =============================================================================
# Scenario Sc: conv cap set, aggregate UNDER cap -> exit 0
# =============================================================================

note "Scenario Sc: aggregate convergence under cap should pass"

SC_ROOT="$WORKSPACE/sc"
stage_repo_root "$SC_ROOT"
write_session_json "$SC_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10, "maxWallClockMinutes": null, "maxToolCalls": null },
  "convergenceLoops": [
    { "specDir": "specs/900-a", "agent": "bubbles.workflow", "iterationCount": 5 },
    { "specDir": "specs/901-b", "agent": "bubbles.workflow", "iterationCount": 3 }
  ]
}'

run_guard "$SC_ROOT"

assert_exit 0 "Sc exit code (aggregate 8 <= cap 10)"
assert_stdout_contains "PASS Gate G128" "Sc PASS marker on stdout"
assert_stdout_contains "conv=8/10" "Sc reports aggregate conv=8/10"

# =============================================================================
# Scenario Sd: conv cap set, aggregate OVER cap across TWO specs -> exit 1
# Proves G128 caps the AGGREGATE (5 + 8 = 13 > 10) even though NO single spec
# exceeds the per-spec G082 cap of 10 — the core distinction from G082.
# =============================================================================

note "Scenario Sd: aggregate convergence over cap across two specs should exit 1"

SD_ROOT="$WORKSPACE/sd"
stage_repo_root "$SD_ROOT"
write_session_json "$SD_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10, "maxWallClockMinutes": null, "maxToolCalls": null },
  "convergenceLoops": [
    { "specDir": "specs/900-a", "agent": "bubbles.workflow", "iterationCount": 5 },
    { "specDir": "specs/901-b", "agent": "bubbles.workflow", "iterationCount": 8 }
  ]
}'

run_guard "$SD_ROOT"

assert_exit 1 "Sd exit code (aggregate 13 > cap 10)"
assert_stderr_contains "G128" "Sd stderr names Gate G128"
assert_stderr_contains "session_cap_enforcement_gate" "Sd stderr names gate full name"
assert_stderr_contains "convergence" "Sd stderr names the convergence dimension"
assert_stderr_contains "maxTotalConvergenceIterations=10" "Sd stderr names the cap"
assert_stderr_contains "AGGREGATE" "Sd stderr documents the G082 distinction"
assert_stderr_contains "blocked" "Sd stderr documents 'blocked' remediation"

# =============================================================================
# Scenario Se: malformed session.json -> exit 2 + diagnostic
# =============================================================================

note "Scenario Se: malformed session.json should exit 2"

SE_ROOT="$WORKSPACE/se"
stage_repo_root "$SE_ROOT"
write_session_json "$SE_ROOT" '{"sessionBudget": {'

run_guard "$SE_ROOT"

assert_exit 2 "Se exit code (malformed JSON)"
assert_stderr_contains "session-cap-guard" "Se stderr has diagnostic prefix"
assert_stderr_contains "not valid JSON" "Se stderr names malformed-JSON condition"

# =============================================================================
# Scenario Sf: --skip bypass flag rejected -> exit 2
# =============================================================================

note "Scenario Sf: --skip bypass flag must be rejected with exit 2"

SF_ROOT="$WORKSPACE/sf"
stage_repo_root "$SF_ROOT"
write_session_json "$SF_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10, "maxWallClockMinutes": null, "maxToolCalls": null },
  "convergenceLoops": [ { "specDir": "specs/900-a", "agent": "bubbles.workflow", "iterationCount": 99 } ]
}'

run_guard "$SF_ROOT" --skip

assert_exit 2 "Sf exit code (bypass flag rejected)"
assert_stderr_contains "unknown flag: --skip" "Sf stderr rejects --skip"

# =============================================================================
# Scenario Sg: wall-clock minutes OVER cap -> exit 1
# 10:00:00Z -> 11:30:00Z = 90 minutes; cap 60 -> breach.
# =============================================================================

note "Scenario Sg: aggregate wall-clock over cap should exit 1"

SG_ROOT="$WORKSPACE/sg"
stage_repo_root "$SG_ROOT"
write_session_json "$SG_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": null, "maxWallClockMinutes": 60, "maxToolCalls": null },
  "turnSnapshots": [
    { "turnNumber": 1, "timestamp": "2026-06-01T10:00:00Z", "mode": "start" },
    { "turnNumber": 2, "timestamp": "2026-06-01T11:30:00Z", "mode": "end" }
  ]
}'

run_guard "$SG_ROOT"

assert_exit 1 "Sg exit code (90 min > cap 60)"
assert_stderr_contains "wallClockMinutes" "Sg stderr names the wall-clock dimension"
assert_stderr_contains "maxWallClockMinutes=60" "Sg stderr names the wall-clock cap"

# =============================================================================
# Scenario Sh: tool calls OVER cap -> exit 1
# =============================================================================

note "Scenario Sh: aggregate tool calls over cap should exit 1"

SH_ROOT="$WORKSPACE/sh"
stage_repo_root "$SH_ROOT"
write_session_json "$SH_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": null, "maxWallClockMinutes": null, "maxToolCalls": 100 },
  "toolCallCount": 250
}'

run_guard "$SH_ROOT"

assert_exit 1 "Sh exit code (250 > cap 100)"
assert_stderr_contains "toolCalls" "Sh stderr names the tool-calls dimension"
assert_stderr_contains "maxToolCalls=100" "Sh stderr names the tool-calls cap"

# =============================================================================
# Scenario Si: caps set but usage data absent -> exit 0 (skip unmeasurable)
# maxWallClockMinutes + maxToolCalls set, but NO turnSnapshots and NO
# toolCallCount -> those dimensions are skipped, convergence cap is null.
# =============================================================================

note "Scenario Si: caps set but usage absent should pass (unmeasurable skipped)"

SI_ROOT="$WORKSPACE/si"
stage_repo_root "$SI_ROOT"
write_session_json "$SI_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": null, "maxWallClockMinutes": 1, "maxToolCalls": 1 }
}'

run_guard "$SI_ROOT"

assert_exit 0 "Si exit code (unmeasurable dimensions skipped)"
assert_stdout_contains "PASS Gate G128" "Si PASS marker on stdout"

# =============================================================================
# Scenario Sj: unexpected positional argument rejected -> exit 2
# =============================================================================

note "Scenario Sj: unexpected positional argument must be rejected with exit 2"

SJ_ROOT="$WORKSPACE/sj"
stage_repo_root "$SJ_ROOT"
write_session_json "$SJ_ROOT" '{ "sessionBudget": { "maxTotalConvergenceIterations": 10 } }'

run_guard "$SJ_ROOT" "specs/900-a"

assert_exit 2 "Sj exit code (positional rejected)"
assert_stderr_contains "unexpected positional argument" "Sj stderr rejects positional"

# =============================================================================
# IMP-039 SCOPE-3: context-volume dimensions
#
# The three original dimensions cannot see how much text a session carries, so
# these scenarios are the ones that prove the new caps actually bite. Sk is the
# adversarial case: the session holds every legacy dimension and is still
# refused on retained bytes.
# =============================================================================

write_tool_log() {
  local root="$1"
  shift
  mkdir -p "$root/.specify/runtime"
  printf '%s\n' "$@" > "$root/.specify/runtime/tool-calls.jsonl"
}

note "Scenario Sk: a single oversized tool result should exit 1"

SK_ROOT="$WORKSPACE/sk"
stage_repo_root "$SK_ROOT"
write_session_json "$SK_ROOT" '{
  "sessionBudget": { "maxSingleToolResultBytes": 50000 }
}'
write_tool_log "$SK_ROOT" \
  '{"cmd":"a","stdoutBytes":1200,"stderrBytes":0}' \
  '{"cmd":"b","stdoutBytes":80000,"stderrBytes":112}'

run_guard "$SK_ROOT"

assert_exit 1 "Sk exit code (80112 > cap 50000)"
assert_stderr_contains "singleToolResultBytes" "Sk stderr names the single-result dimension"
assert_stderr_contains "maxSingleToolResultBytes=50000" "Sk stderr names the cap"

note "Scenario Sl: cumulative tool-result bytes over cap should exit 1"

SL_ROOT="$WORKSPACE/sl"
stage_repo_root "$SL_ROOT"
write_session_json "$SL_ROOT" '{
  "sessionBudget": { "maxCumulativeToolResultBytes": 250000 }
}'
write_tool_log "$SL_ROOT" \
  '{"cmd":"a","stdoutBytes":150000,"stderrBytes":0}' \
  '{"cmd":"b","stdoutBytes":150000,"stderrBytes":0}'

run_guard "$SL_ROOT"

assert_exit 1 "Sl exit code (300000 > cap 250000)"
assert_stderr_contains "cumulativeToolResultBytes" "Sl stderr names the cumulative dimension"

note "Scenario Sm: byte usage under cap should pass"

SM_ROOT="$WORKSPACE/sm"
stage_repo_root "$SM_ROOT"
write_session_json "$SM_ROOT" '{
  "sessionBudget": { "maxSingleToolResultBytes": 50000, "maxCumulativeToolResultBytes": 250000 }
}'
write_tool_log "$SM_ROOT" \
  '{"cmd":"a","stdoutBytes":10,"stderrBytes":5}'

run_guard "$SM_ROOT"

assert_exit 0 "Sm exit code (under both byte caps)"
assert_stdout_contains "PASS Gate G128" "Sm PASS marker on stdout"

note "Scenario Sn: byte caps set but no tool-call log should pass (unmeasurable skipped)"

SN_ROOT="$WORKSPACE/sn"
stage_repo_root "$SN_ROOT"
write_session_json "$SN_ROOT" '{
  "sessionBudget": { "maxSingleToolResultBytes": 1, "maxCumulativeToolResultBytes": 1 }
}'

run_guard "$SN_ROOT"

assert_exit 0 "Sn exit code (no tool-call log -> unmeasurable, skipped)"
assert_stdout_contains "unmeasured" "Sn reports the dimension as unmeasured, not zero"

note "Scenario So: token caps with the default 'none' usage adapter are skipped"

SO_ROOT="$WORKSPACE/so"
stage_repo_root "$SO_ROOT"
write_session_json "$SO_ROOT" '{
  "sessionBudget": { "maxPromptTokensPerRequest": 1, "maxCumulativePromptTokens": 1 }
}'

run_guard "$SO_ROOT"

assert_exit 0 "So exit code (no usage adapter -> token dimensions skipped)"
assert_stdout_contains "PASS Gate G128" "So PASS marker on stdout"

note "Scenario Sp: a non-integer context cap must be rejected with exit 2"

SP_ROOT="$WORKSPACE/sp"
stage_repo_root "$SP_ROOT"
write_session_json "$SP_ROOT" '{
  "sessionBudget": { "maxSingleToolResultBytes": "50kb" }
}'

run_guard "$SP_ROOT"

assert_exit 2 "Sp exit code (non-integer cap rejected)"
assert_stderr_contains "maxSingleToolResultBytes" "Sp stderr names the malformed cap"

note "Scenario Sq: an all-null budget including the new caps stays a no-op"

SQ_ROOT="$WORKSPACE/sq"
stage_repo_root "$SQ_ROOT"
write_session_json "$SQ_ROOT" '{
  "sessionBudget": {
    "maxTotalConvergenceIterations": null, "maxWallClockMinutes": null, "maxToolCalls": null,
    "maxSingleToolResultBytes": null, "maxCumulativeToolResultBytes": null,
    "maxPromptTokensPerRequest": null, "maxCumulativePromptTokens": null
  }
}'
write_tool_log "$SQ_ROOT" '{"cmd":"a","stdoutBytes":999999,"stderrBytes":999999}'

run_guard "$SQ_ROOT"

assert_exit 0 "Sq exit code (every cap null -> no-op even with huge retained bytes)"
assert_stdout_contains "no non-null cap" "Sq reports the default-off posture"

# =============================================================================
# Final verdict
# =============================================================================

echo ""
echo "============================================================"
echo "  SESSION-CAP-GUARD SELFTEST VERDICT"
echo "============================================================"
printf 'Passed assertions: %d\n' "$PASS_COUNT"
printf 'Failed assertions: %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo ""
  echo "FAILED scenarios:"
  for s in "${FAILED_SCENARIOS[@]}"; do
    echo "  - $s"
  done
  exit 1
fi

echo ""
echo "🟢 session-cap-guard-selftest: ALL SCENARIOS PASS"
exit 0
