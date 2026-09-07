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
#   Sc: exact-session conv aggregate UNDER cap    → exit 0
#   Sd: exact-session conv aggregate OVER cap
#       across TWO specs                          → exit 1, names G128 +
#                                                    "convergence"
#   Se: malformed session.json                    → exit 2
#   Sf: --skip bypass flag rejected               → exit 2
#   Sg: wall-clock minutes OVER cap               → exit 1, names
#                                                    "wallClockMinutes"
#   Sh: legacy tool-call scalar present            → exit 0, UNMEASURABLE
#   Si: caps set but usage data absent            → exit 0 (unmeasurable
#                                                    dimensions skipped)
#   Sj: unexpected positional argument rejected    → exit 2
#
# Reference:
#   improvements/IMP-003-autonomy-dial-and-safety-caps.md (SCOPE-2)

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd -P)"
SOURCE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd -P)"
GUARD_SCRIPT="$SCRIPT_DIR/session-cap-guard.sh"
POST_MUTATION_BASELINE_MODE="${BUBBLES_G128_POST_MUTATION_BASELINE_MODE:-false}"
BRIEF_OUTPUT="${BUBBLES_G128_SELFTEST_BRIEF:-false}"

if [[ ! -x "$GUARD_SCRIPT" ]]; then
  echo "session-cap-guard-selftest: guard script not executable: $GUARD_SCRIPT" >&2
  exit 2
fi

if [[ "$POST_MUTATION_BASELINE_MODE" != "false" && "$POST_MUTATION_BASELINE_MODE" != "true" ]]; then
  echo "session-cap-guard-selftest: BUBBLES_G128_POST_MUTATION_BASELINE_MODE must be true or false" >&2
  exit 2
fi
if [[ "$BRIEF_OUTPUT" != "false" && "$BRIEF_OUTPUT" != "true" ]]; then
  echo "session-cap-guard-selftest: BUBBLES_G128_SELFTEST_BRIEF must be true or false" >&2
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

note() {
  [[ "$BRIEF_OUTPUT" == "true" ]] || printf '[selftest] %s\n' "$*"
}
ok() {
  [[ "$BRIEF_OUTPUT" == "true" ]] || printf '[selftest] PASS: %s\n' "$*"
  PASS_COUNT=$((PASS_COUNT + 1))
}
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

write_raw_session_json() {
  local root="$1"
  local payload="$2"
  printf '%s\n' "$payload" > "$root/.specify/memory/bubbles.session.json"
}

write_session_json() {
  local root="$1"
  local payload="$2"
  local session_file="$root/.specify/memory/bubbles.session.json"

  if printf '%s\n' "$payload" | jq -e '
    type == "object"
    and (.sessionBudget | type) == "object"
    and (has("sessionBudgetHistory") | not)
  ' >/dev/null 2>&1; then
    printf '%s\n' "$payload" | jq '
      .sessionBudget as $legacy
      | . + {
          sessionBudgetHistory: [{
            recordSchemaVersion: 1,
            hostSessionId: "host-current",
            revision: 1,
            supersedesRevision: null,
            recordedAt: "2026-09-01T00:00:00Z",
            budget: ({
              schemaVersion: 1,
              maxTotalConvergenceIterations: null,
              maxWallClockMinutes: null,
              maxToolCalls: null,
              maxSingleToolResultBytes: null,
              maxCumulativeToolResultBytes: null,
              maxPromptTokensPerRequest: null,
              maxCumulativePromptTokens: null
            } + $legacy)
          }]
        }
    ' > "$session_file"
  else
    write_raw_session_json "$root" "$payload"
  fi
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
    return 0
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
    return 0
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
    return 0
  fi
  ok "$label: stderr contains '$needle'"
}

run_post_mutation_origin_main_baseline() {
  local baseline_dir="$WORKSPACE/origin-main-baseline"
  local fixture_root="$WORKSPACE/origin-main-mixed-session"
  local baseline_guard="$baseline_dir/session-cap-guard.sh"
  local origin_commit origin_blob materialized_blob baseline_rc
  local current_convergence current_wall_minutes current_max_bytes

  command -v git >/dev/null 2>&1 || {
    echo "POST_MUTATION_BASELINE_RECONSTRUCTION_ERROR=git-unavailable"
    exit 2
  }
  origin_commit="$(git -C "$SOURCE_ROOT" rev-parse --verify origin/main 2>/dev/null)" || {
    echo "POST_MUTATION_BASELINE_RECONSTRUCTION_ERROR=origin-main-unavailable"
    exit 2
  }
  origin_blob="$(git -C "$SOURCE_ROOT" rev-parse --verify 'origin/main:bubbles/scripts/session-cap-guard.sh' 2>/dev/null)" || {
    echo "POST_MUTATION_BASELINE_RECONSTRUCTION_ERROR=origin-main-guard-unavailable"
    exit 2
  }

  mkdir -p "$baseline_dir" "$fixture_root/.specify/memory" "$fixture_root/.specify/runtime"
  if ! git -C "$SOURCE_ROOT" show 'origin/main:bubbles/scripts/session-cap-guard.sh' > "$baseline_guard" 2>/dev/null; then
    echo "POST_MUTATION_BASELINE_RECONSTRUCTION_ERROR=origin-main-materialization-failed"
    exit 2
  fi
  chmod 700 "$baseline_guard"
  materialized_blob="$(git hash-object --no-filters "$baseline_guard")"
  [[ "$materialized_blob" == "$origin_blob" ]] || {
    echo "POST_MUTATION_BASELINE_RECONSTRUCTION_ERROR=origin-main-blob-mismatch"
    exit 2
  }

  printf '%s\n' '{
    "sessionBudget": {
      "maxTotalConvergenceIterations": 10,
      "maxWallClockMinutes": 60,
      "maxToolCalls": 1,
      "maxSingleToolResultBytes": 1000,
      "maxCumulativeToolResultBytes": 2000,
      "maxPromptTokensPerRequest": null,
      "maxCumulativePromptTokens": null
    },
    "toolCallCount": 9999,
    "convergenceLoops": [
      { "hostSessionId": "host-old", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": 99 },
      { "hostSessionId": "host-current", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": 4 },
      { "specDir": "specs/legacy", "agent": "bubbles.goal", "iterationCount": 777 }
    ],
    "turnSnapshots": [
      { "hostSessionId": "host-old", "timestamp": "2026-08-01T00:00:00Z" },
      { "hostSessionId": "host-old", "timestamp": "2026-08-01T04:00:00Z" },
      { "hostSessionId": "host-current", "timestamp": "2026-09-01T00:00:00Z" },
      { "hostSessionId": "host-current", "timestamp": "2026-09-01T00:10:00Z" },
      { "timestamp": "2020-01-01T00:00:00Z" }
    ]
  }' > "$fixture_root/.specify/memory/bubbles.session.json"
  printf '%s\n' \
    '{"sessionId":"host-old","stdoutBytes":9000,"stderrBytes":0}' \
    '{"sessionId":"host-current","stdoutBytes":100,"stderrBytes":20}' \
    '{"stdoutBytes":500000,"stderrBytes":0}' \
    > "$fixture_root/.specify/runtime/tool-calls.jsonl"
  cp "$fixture_root/.specify/memory/bubbles.session.json" "$fixture_root/session.before"
  cp "$fixture_root/.specify/runtime/tool-calls.jsonl" "$fixture_root/tools.before"

  current_convergence="$(jq '[.convergenceLoops[] | select(.hostSessionId == "host-current") | .iterationCount] | add' "$fixture_root/.specify/memory/bubbles.session.json")"
  current_wall_minutes="$(jq '[.turnSnapshots[] | select(.hostSessionId == "host-current") | .timestamp | fromdateiso8601] | ((max - min) / 60)' "$fixture_root/.specify/memory/bubbles.session.json")"
  current_max_bytes="$(jq -s '[.[] | select(.sessionId == "host-current") | ((.stdoutBytes // 0) + (.stderrBytes // 0))] | max' "$fixture_root/.specify/runtime/tool-calls.jsonl")"

  set +e
  BUBBLES_REPO_ROOT="$fixture_root" bash "$baseline_guard" --quiet \
    > "$WORKSPACE/origin-main.stdout" 2> "$WORKSPACE/origin-main.stderr"
  baseline_rc=$?
  set -e

  printf '%s\n' 'POST_MUTATION_BASELINE_RECONSTRUCTION_BEGIN'
  printf '%s\n' 'classification=post-mutation-baseline-reconstruction'
  printf '%s\n' 'temporal_claim=not-executed-before-existing-worktree-mutation'
  printf 'source_ref=origin/main source_commit=%s source_guard_blob=%s\n' "$origin_commit" "$origin_blob"
  printf '%s\n' 'fixture_target_session=host-current mixed_session=true legacy_records=true'
  printf 'fixture_current_convergence=%s cap=10 expected=within-cap\n' "$current_convergence"
  printf '%s\n' 'fixture_old_convergence=99 fixture_legacy_convergence=777'
  printf 'fixture_current_wall_minutes=%s cap=60 expected=within-cap\n' "$current_wall_minutes"
  printf '%s\n' 'fixture_old_wall_minutes=240 expected=over-cap'
  printf 'fixture_current_tool_result_max=%s cap=1000 expected=within-cap\n' "$current_max_bytes"
  printf '%s\n' 'fixture_old_tool_result_max=9000 fixture_unattributed_tool_result_max=500000'
  printf '%s\n' 'legacy_tool_call_scalar=9999 cap=1 fixed_contract=ineligible'
  printf '%s\n' 'expected_exact_session_exit=0'
  printf 'origin_main_guard_exit=%s\n' "$baseline_rc"

  if [[ "$baseline_rc" -eq 1 ]] &&
    grep -Fq 'aggregate iterationCount=880 > maxTotalConvergenceIterations=10' "$WORKSPACE/origin-main.stderr" &&
    grep -Fq 'aggregate toolCallCount=9999 > maxToolCalls=1' "$WORKSPACE/origin-main.stderr" &&
    grep -Fq 'largest retained tool result=500000 > maxSingleToolResultBytes=1000' "$WORKSPACE/origin-main.stderr" &&
    cmp -s "$fixture_root/session.before" "$fixture_root/.specify/memory/bubbles.session.json" &&
    cmp -s "$fixture_root/tools.before" "$fixture_root/.specify/runtime/tool-calls.jsonl"; then
    printf '%s\n' 'origin_main_observed_aggregate_convergence=880'
    printf '%s\n' 'retained_history_byte_identical=true'
    printf '%s\n' 'RED_REASON=origin/main charged mismatched and unattributed history to the target evaluation'
    printf '%s\n' 'POST_MUTATION_BASELINE_RECONSTRUCTION_RED=CONFIRMED'
    printf '%s\n' 'POST_MUTATION_BASELINE_RECONSTRUCTION_END'
    exit 1
  fi

  printf '%s\n' 'POST_MUTATION_BASELINE_RECONSTRUCTION_RED=NOT_CONFIRMED'
  printf '%s\n' 'POST_MUTATION_BASELINE_RECONSTRUCTION_END'
  exit 2
}

if [[ "$POST_MUTATION_BASELINE_MODE" == "true" ]]; then
  run_post_mutation_origin_main_baseline
fi

# =============================================================================
# Scenario S0: no session.json -> exit 0 (no-op)
# =============================================================================

note "Scenario S0: no session.json should pass with exit 0 (no-op)"

S0_ROOT="$WORKSPACE/s0"
stage_repo_root "$S0_ROOT"
# Intentionally NO session file written.

run_guard "$S0_ROOT"

assert_exit 0 "S0 exit code"
assert_stdout_contains "G128 status=NO-ACTIVE-BUDGET exit=0 reason=no-session-file" "S0 reports no session.json without claiming measurement"

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
assert_stdout_contains "G128 status=NO-ACTIVE-BUDGET exit=0 reason=no-session-budget-history" "Sa reports no exact-session budget history"

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
assert_stdout_contains "G128 status=NO-ACTIVE-BUDGET exit=0 reason=all-caps-null" "Sb reports the all-null budget"

# =============================================================================
# Scenario Sc: conv cap set, aggregate UNDER cap -> exit 0
# =============================================================================

note "Scenario Sc: aggregate convergence under cap should pass"

SC_ROOT="$WORKSPACE/sc"
stage_repo_root "$SC_ROOT"
write_session_json "$SC_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10, "maxWallClockMinutes": null, "maxToolCalls": null },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/900-a", "agent": "bubbles.workflow", "iterationCount": 5 },
    { "hostSessionId": "host-current", "specDir": "specs/901-b", "agent": "bubbles.workflow", "iterationCount": 3 }
  ]
}'

run_guard "$SC_ROOT" --session-id host-current

assert_exit 0 "Sc exit code (aggregate 8 <= cap 10)"
assert_stdout_contains "G128 dimension name=maxTotalConvergenceIterations cap=10 state=MEASURED observed=8" "Sc reports exact-session convergence 8/10"
assert_stdout_contains 'G128 status=SOFT-BOUNDARY exit=0 session="host-current"' "Sc preserves the 70 percent soft-boundary status"

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
    { "hostSessionId": "host-current", "specDir": "specs/900-a", "agent": "bubbles.workflow", "iterationCount": 5 },
    { "hostSessionId": "host-current", "specDir": "specs/901-b", "agent": "bubbles.workflow", "iterationCount": 8 }
  ]
}'

run_guard "$SD_ROOT" --session-id host-current

assert_exit 1 "Sd exit code (aggregate 13 > cap 10)"
assert_stderr_contains "G128" "Sd stderr names Gate G128"
assert_stderr_contains 'G128 status=BREACH exit=1 session="host-current"' "Sd stderr names the exact-session breach"
assert_stderr_contains "G128 breach name=maxTotalConvergenceIterations observed=13 cap=10" "Sd stderr names observation and cap"

# =============================================================================
# Scenario Se: malformed session.json -> exit 2 + diagnostic
# =============================================================================

note "Scenario Se: malformed session.json should exit 2"

SE_ROOT="$WORKSPACE/se"
stage_repo_root "$SE_ROOT"
write_raw_session_json "$SE_ROOT" '{"sessionBudget": {'

run_guard "$SE_ROOT"

assert_exit 2 "Se exit code (malformed JSON)"
assert_stderr_contains "session-cap-guard" "Se stderr has diagnostic prefix"
assert_stderr_contains "G128 status=INPUT-ERROR exit=2 reason=invalid-session-json" "Se stderr names malformed-JSON condition"

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
assert_stderr_contains 'argument="--skip"' "Sf stderr rejects --skip"

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
    { "hostSessionId": "host-current", "turnNumber": 1, "timestamp": "2026-06-01T10:00:00Z", "mode": "start" },
    { "hostSessionId": "host-current", "turnNumber": 2, "timestamp": "2026-06-01T11:30:00Z", "mode": "end" }
  ]
}'

run_guard "$SG_ROOT" --session-id host-current

assert_exit 1 "Sg exit code (90 min > cap 60)"
assert_stderr_contains "G128 breach name=maxWallClockMinutes observed=90 cap=60" "Sg stderr names the wall-clock observation and cap"

# =============================================================================
# Scenario Sh: tool calls OVER cap -> exit 1
# =============================================================================

note "Scenario Sh: legacy tool-call scalar is retained but remains unmeasurable"

SH_ROOT="$WORKSPACE/sh"
stage_repo_root "$SH_ROOT"
write_session_json "$SH_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": null, "maxWallClockMinutes": null, "maxToolCalls": 100 },
  "toolCallCount": 250
}'

run_guard "$SH_ROOT" --session-id host-current

assert_exit 0 "Sh exit code (no exact tool-call producer)"
assert_stdout_contains "G128 dimension name=maxToolCalls cap=100 state=UNMEASURABLE observed=- reason=no-exact-producer" "Sh reports maxToolCalls honestly"

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

run_guard "$SI_ROOT" --session-id host-current

assert_exit 0 "Si exit code (unmeasurable dimensions skipped)"
assert_stdout_contains 'G128 status=PASS exit=0 session="host-current"' "Si PASS marker on stdout"

# =============================================================================
# Scenario Sj: unexpected positional argument rejected -> exit 2
# =============================================================================

note "Scenario Sj: unexpected positional argument must be rejected with exit 2"

SJ_ROOT="$WORKSPACE/sj"
stage_repo_root "$SJ_ROOT"
write_session_json "$SJ_ROOT" '{ "sessionBudget": { "maxTotalConvergenceIterations": 10 } }'

run_guard "$SJ_ROOT" "specs/900-a"

assert_exit 2 "Sj exit code (positional rejected)"
assert_stderr_contains 'argument="specs/900-a"' "Sj stderr rejects positional"

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
  '{"sessionId":"host-current","cmd":"a","stdoutBytes":1200,"stderrBytes":0}' \
  '{"sessionId":"host-current","cmd":"b","stdoutBytes":80000,"stderrBytes":112}'

run_guard "$SK_ROOT" --session-id host-current

assert_exit 1 "Sk exit code (80112 > cap 50000)"
assert_stderr_contains "G128 breach name=maxSingleToolResultBytes observed=80112 cap=50000" "Sk stderr names the single-result observation and cap"

note "Scenario Sl: cumulative tool-result bytes over cap should exit 1"

SL_ROOT="$WORKSPACE/sl"
stage_repo_root "$SL_ROOT"
write_session_json "$SL_ROOT" '{
  "sessionBudget": { "maxCumulativeToolResultBytes": 250000 }
}'
write_tool_log "$SL_ROOT" \
  '{"sessionId":"host-current","cmd":"a","stdoutBytes":150000,"stderrBytes":0}' \
  '{"sessionId":"host-current","cmd":"b","stdoutBytes":150000,"stderrBytes":0}'

run_guard "$SL_ROOT" --session-id host-current

assert_exit 1 "Sl exit code (300000 > cap 250000)"
assert_stderr_contains "G128 breach name=maxCumulativeToolResultBytes observed=300000 cap=250000" "Sl stderr names the cumulative observation and cap"

note "Scenario Sm: byte usage under cap should pass"

SM_ROOT="$WORKSPACE/sm"
stage_repo_root "$SM_ROOT"
write_session_json "$SM_ROOT" '{
  "sessionBudget": { "maxSingleToolResultBytes": 50000, "maxCumulativeToolResultBytes": 250000 }
}'
write_tool_log "$SM_ROOT" \
  '{"sessionId":"host-current","cmd":"a","stdoutBytes":10,"stderrBytes":5}'

run_guard "$SM_ROOT" --session-id host-current

assert_exit 0 "Sm exit code (under both byte caps)"
assert_stdout_contains 'G128 status=PASS exit=0 session="host-current"' "Sm PASS marker on stdout"

note "Scenario Sn: byte caps set but no tool-call log should pass (unmeasurable skipped)"

SN_ROOT="$WORKSPACE/sn"
stage_repo_root "$SN_ROOT"
write_session_json "$SN_ROOT" '{
  "sessionBudget": { "maxSingleToolResultBytes": 1, "maxCumulativeToolResultBytes": 1 }
}'

run_guard "$SN_ROOT" --session-id host-current

assert_exit 0 "Sn exit code (no tool-call log -> unmeasurable, skipped)"
assert_stdout_contains "state=UNMEASURABLE" "Sn reports the dimension as unmeasurable, not zero"

note "Scenario So: token caps with the default 'none' usage adapter are skipped"

SO_ROOT="$WORKSPACE/so"
stage_repo_root "$SO_ROOT"
write_session_json "$SO_ROOT" '{
  "sessionBudget": { "maxPromptTokensPerRequest": 1, "maxCumulativePromptTokens": 1 }
}'

run_guard "$SO_ROOT" --session-id host-current

assert_exit 0 "So exit code (no usage adapter -> token dimensions skipped)"
assert_stdout_contains "G128 dimension name=maxPromptTokensPerRequest cap=1 state=UNMEASURABLE observed=- reason=no-exact-usage-result" "So reports exact usage as unmeasurable"

note "Scenario Sp: a non-integer context cap must be rejected with exit 2"

SP_ROOT="$WORKSPACE/sp"
stage_repo_root "$SP_ROOT"
write_session_json "$SP_ROOT" '{
  "sessionBudget": { "maxSingleToolResultBytes": "50kb" }
}'

run_guard "$SP_ROOT"

assert_exit 2 "Sp exit code (non-integer cap rejected)"
assert_stderr_contains "maxSingleToolResultBytes" "Sp stderr names the malformed cap"

note "Scenario Sp2: non-numeric cap types must not collapse to null"

SP2_ROOT="$WORKSPACE/sp2"
stage_repo_root "$SP2_ROOT"
write_session_json "$SP2_ROOT" '{
  "sessionBudget": { "maxToolCalls": false }
}'

run_guard "$SP2_ROOT"

assert_exit 2 "Sp2 exit code (boolean cap rejected instead of default-off)"
assert_stderr_contains "maxToolCalls" "Sp2 stderr names the malformed boolean cap"

SP2B_ROOT="$WORKSPACE/sp2b"
stage_repo_root "$SP2B_ROOT"
write_session_json "$SP2B_ROOT" '{
  "sessionBudget": { "maxWallClockMinutes": "5" }
}'

run_guard "$SP2B_ROOT"

assert_exit 2 "Sp2b exit code (digit-only string cap rejected)"
assert_stderr_contains "maxWallClockMinutes" "Sp2b stderr names the malformed string cap"

note "Scenario Sp3: malformed record collections must not collapse to empty arrays"

SP3_ROOT="$WORKSPACE/sp3"
stage_repo_root "$SP3_ROOT"
write_session_json "$SP3_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 1 },
  "turnSnapshots": false,
  "convergenceLoops": false
}'

run_guard "$SP3_ROOT" --session-id host-current

assert_exit 2 "Sp3 exit code (non-array record collections rejected)"
assert_stderr_contains "G128 status=INPUT-ERROR exit=2 reason=invalid-session-records" "Sp3 names malformed session record collections"

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
assert_stdout_contains "G128 status=NO-ACTIVE-BUDGET exit=0 reason=all-caps-null" "Sq reports the default-off posture"

# =============================================================================
# IMP-048 SCOPE-6: the 70% SOFT boundary and the mode-default budgets.
#
# The hard stop used to be the guard's only outcome, so a session learned it was
# over budget at the moment it was refused. These scenarios prove the soft
# boundary warns and CONTINUES, that it never converts a full session into a
# blocked spec, and that the hard stop is untouched.
# =============================================================================

assert_stdout_lacks() {
  local needle="$1"
  local label="$2"
  if grep -Fq -- "$needle" "$WORKSPACE/stdout.last"; then
    ko "$label: stdout unexpectedly contained '$needle'"
    echo "  --- stdout ---" >&2
    last_stdout >&2
    return 0
  fi
  ok "$label: stdout does not contain '$needle'"
}

assert_file_unchanged() {
  local file="$1" before="$2" label="$3" after
  after="$(cat "$file")"
  if [[ "$after" != "$before" ]]; then
    ko "$label: $file was modified"
    return 1
  fi
  ok "$label: $file byte-identical"
}

note "Scenario Sr: crossing 70% warns, continues, and does NOT block the spec"

SR_ROOT="$WORKSPACE/sr"
stage_repo_root "$SR_ROOT"
mkdir -p "$SR_ROOT/specs/900-a"
# A real spec, in a real status, sitting next to a session that is 74% full. The
# distinction this scenario exists to prove is that a full SESSION is not a
# blocked SPEC — the work is fine, the container is not.
printf '%s\n' '{ "status": "in_progress" }' > "$SR_ROOT/specs/900-a/state.json"
SR_STATE_BEFORE="$(cat "$SR_ROOT/specs/900-a/state.json")"
write_session_json "$SR_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 350 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/900-a", "agent": "bubbles.goal", "iterationCount": 260 }
  ]
}'

run_guard "$SR_ROOT" --session-id host-current

assert_exit 0 "Sr exit code (260/350 = 74% -> soft boundary, NOT a refusal)"
assert_stdout_contains 'G128 status=SOFT-BOUNDARY exit=0 session="host-current"' "Sr announces the soft boundary"
assert_stdout_contains "consumedPct=74" "Sr names the observed consumption"
assert_stdout_contains "action=handoff" "Sr retains the handoff action"
assert_stdout_lacks "G128 BREACH" "Sr does not report a hard breach"
assert_file_unchanged "$SR_ROOT/specs/900-a/state.json" "$SR_STATE_BEFORE" "Sr spec state.json"

note "Scenario Ss: crossing 100% still hard-stops (unchanged)"

SS_ROOT="$WORKSPACE/ss"
stage_repo_root "$SS_ROOT"
write_session_json "$SS_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 350 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/900-a", "agent": "bubbles.goal", "iterationCount": 351 }
  ]
}'

run_guard "$SS_ROOT" --session-id host-current

assert_exit 1 "Ss exit code (351 > cap 350 -> hard stop preserved)"
assert_stderr_contains 'G128 status=BREACH exit=1 session="host-current"' "Ss stderr still reports the breach"
assert_stderr_contains "G128 breach name=maxTotalConvergenceIterations observed=351 cap=350" "Ss stderr names the breached cap"

note "Scenario St: below the soft boundary emits no rollover recommendation"

ST_ROOT="$WORKSPACE/st"
stage_repo_root "$ST_ROOT"
write_session_json "$ST_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 350 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/900-a", "agent": "bubbles.goal", "iterationCount": 240 }
  ]
}'

run_guard "$ST_ROOT" --session-id host-current

assert_exit 0 "St exit code (240/350 = 68% -> under the 70% boundary)"
assert_stdout_lacks "SOFT-BOUNDARY" "St stays quiet below the boundary"
assert_stdout_contains 'G128 status=PASS exit=0 session="host-current"' "St PASS marker on stdout"

note "Scenario Su: the soft boundary reuses the EXISTING Class C surface"

SU_ROOT="$WORKSPACE/su"
stage_repo_root "$SU_ROOT"
mkdir -p "$SU_ROOT/.github"
printf 'sessionReview:\n  adapter: jsonl\n' > "$SU_ROOT/.github/bubbles-project.yaml"
write_session_json "$SU_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 350 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/900-a", "agent": "bubbles.goal", "iterationCount": 300 }
  ]
}'

run_guard "$SU_ROOT" --session-id host-current

assert_exit 0 "Su exit code (86% -> soft boundary, still continuing)"
assert_stdout_contains "handoffRecommendation=recorded-" "Su records the recommendation"
if [[ -f "$SU_ROOT/.specify/runtime/session-review.jsonl" ]] &&
  grep -Fq 'handoff-to-fresh-session' "$SU_ROOT/.specify/runtime/session-review.jsonl"; then
  ok "Su: the recommendation landed in the EXISTING session-review store (no second handoff mechanism)"
else
  ko "Su: no Class C handoff-to-fresh-session record in .specify/runtime/session-review.jsonl"
fi

note "Scenario Sv: with sessionReview off, the soft boundary still warns and still passes"

SV_ROOT="$WORKSPACE/sv"
stage_repo_root "$SV_ROOT"
write_session_json "$SV_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 350 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/900-a", "agent": "bubbles.goal", "iterationCount": 300 }
  ]
}'

run_guard "$SV_ROOT" --session-id host-current

assert_exit 0 "Sv exit code (review adapter none -> recommendation unrecorded, guard unaffected)"
assert_stdout_contains "handoffRecommendation=unrecorded" "Sv reports the recommendation as unrecorded"
assert_stdout_contains 'G128 status=SOFT-BOUNDARY exit=0 session="host-current"' "Sv still announces the boundary"

# =============================================================================
# IMP-048 SCOPE-6 registry assertions.
#
# The mode-default budgets are FRAMEWORK defaults in bubbles/workflows/modes.yaml
# and are NOT per-repo config. Nothing mechanically copies them into a session
# file, which is why declaring them cannot newly block an existing repository —
# Sw proves that from the guard's side, and Sx/Sy prove the registry side.
# =============================================================================

MODES_YAML="$(cd "$SCRIPT_DIR/../.." && pwd)/bubbles/workflows/modes.yaml"

note "Scenario Sw: a session recording no sessionBudget is UNBOUNDED, mode defaults notwithstanding"

SW_ROOT="$WORKSPACE/sw"
stage_repo_root "$SW_ROOT"
write_session_json "$SW_ROOT" '{
  "workflowMode": "full-delivery",
  "toolCallCount": 99999,
  "convergenceLoops": [ { "specDir": "specs/900-a", "agent": "bubbles.workflow", "iterationCount": 9999 } ],
  "turnSnapshots": [
    { "turnNumber": 1, "timestamp": "2026-06-01T00:00:00Z", "mode": "start" },
    { "turnNumber": 2, "timestamp": "2026-06-08T00:00:00Z", "mode": "end" }
  ]
}'

run_guard "$SW_ROOT"

assert_exit 0 "Sw exit code (mode names full-delivery; the SESSION records no budget -> unbounded)"
assert_stdout_contains "G128 status=NO-ACTIVE-BUDGET exit=0 reason=no-session-budget-history" "Sw confirms the guard reads only exact-session policy history"
assert_stdout_lacks "SOFT-BOUNDARY" "Sw emits no soft boundary for an unbounded session"

note "Scenario Sx: rapid-tool-delivery keeps its own TIGHTER caps"

if [[ ! -f "$MODES_YAML" ]]; then
  ko "Sx: modes registry not found at $MODES_YAML"
else
  RTD_BLOCK="$(awk '
    /^  rapid-tool-delivery:[[:space:]]*$/ { inmode = 1; next }
    inmode && /^  [a-z]/ { inmode = 0 }
    inmode && /^    sessionBudget:[[:space:]]*$/ { inbudget = 1; next }
    inbudget && /^    [a-zA-Z]/ { inbudget = 0 }
    inbudget { print }
  ' "$MODES_YAML")"
  RTD_OK=1
  for expected in "maxTotalConvergenceIterations: 2" "maxWallClockMinutes: 90" "maxToolCalls: 250"; do
    if ! grep -Fq -- "$expected" <<< "$RTD_BLOCK"; then
      RTD_OK=0
      ko "Sx: rapid-tool-delivery lost its own cap '$expected'"
    fi
  done
  if grep -Fq -- "maxWallClockMinutes: 180" <<< "$RTD_BLOCK"; then
    RTD_OK=0
    ko "Sx: rapid-tool-delivery was overwritten with the looser 180-minute default"
  fi
  [[ "$RTD_OK" -eq 1 ]] && ok "Sx: rapid-tool-delivery keeps 2 iterations / 90 min / 250 tool calls"
fi

note "Scenario Sy: every delivery mode declares a budget; no other mode does"

if [[ ! -f "$MODES_YAML" ]]; then
  ko "Sy: modes registry not found at $MODES_YAML"
else
  # SELECTION RULE, read straight off the registry rather than from a
  # hand-maintained list: a delivery mode is one whose phaseOrder contains BOTH
  # `implement` and `test`. A hand-maintained list is exactly how a new mode
  # ships unbounded without anyone noticing.
  MODE_AUDIT="$(awk '
    /^  [a-z][a-zA-Z0-9-]*:[[:space:]]*$/ {
      if (mode != "") emit()
      mode = $1; sub(/:$/, "", mode); po = ""; budget = 0
      next
    }
    mode != "" && /^    phaseOrder:/ { po = $0 }
    mode != "" && /^    sessionBudget:/ { budget = 1 }
    END { if (mode != "") emit() }
    function emit(   hasimpl, hastest) {
      hasimpl = (po ~ /[[:space:],]implement[[:space:],]/)
      hastest = (po ~ /[[:space:],]test[[:space:],]/)
      if (hasimpl && hastest) {
        delivery++
        if (budget) budgeted++; else printf "UNBOUNDED-DELIVERY %s\n", mode
      } else if (budget) {
        printf "STRAY-BUDGET %s\n", mode
      }
    }
    END { printf "TOTALS delivery=%d budgeted=%d\n", delivery, budgeted }
  ' "$MODES_YAML")"

  MODE_TOTALS="$(awk '$1 == "TOTALS" { print $2, $3 }' <<< "$MODE_AUDIT")"
  MODE_PROBLEMS="$(awk '$1 == "UNBOUNDED-DELIVERY" || $1 == "STRAY-BUDGET" { print }' <<< "$MODE_AUDIT")"

  if [[ -n "$MODE_PROBLEMS" ]]; then
    while IFS= read -r problem; do
      [[ -n "$problem" ]] || continue
      ko "Sy: $problem"
    done <<< "$MODE_PROBLEMS"
  else
    ok "Sy: every delivery mode is bounded and no other mode is ($MODE_TOTALS)"
  fi

  # docs-only is the canonical non-delivery mode. It must stay unbounded, which
  # is the mechanical form of "no existing repo is newly blocked".
  DOCS_BLOCK="$(awk '
    /^  docs-only:[[:space:]]*$/ { inmode = 1; next }
    inmode && /^  [a-z]/ { inmode = 0 }
    inmode { print }
  ' "$MODES_YAML")"
  if grep -Fq 'sessionBudget' <<< "$DOCS_BLOCK"; then
    ko "Sy: docs-only (a non-delivery mode) acquired a session budget"
  else
    ok "Sy: docs-only stays unbounded"
  fi
fi

# =============================================================================
# BUG-037: exact host-session projection
#
# These are deliberately adversarial mixed-session fixtures. The historical
# session is over every populated cap while the requested session is under it.
# A repository-wide aggregate, prefix selector, legacy scalar, or receipt-count
# proxy therefore makes at least one assertion fail.
# =============================================================================

assert_stderr_lacks() {
  local needle="$1"
  local label="$2"
  if grep -Fq -- "$needle" "$WORKSPACE/stderr.last"; then
    ko "$label: stderr unexpectedly contained '$needle'"
    echo "  --- stderr ---" >&2
    last_stderr >&2
    return 0
  fi
  ok "$label: stderr does not contain '$needle'"
}

assert_dimension_count() {
  local expected="$1"
  local label="$2"
  local actual
  actual="$(grep -c '^G128 dimension name=' "$WORKSPACE/stdout.last" 2>/dev/null || true)"
  if [[ "$actual" == "$expected" ]]; then
    ok "$label: dimension rows=$expected"
  else
    ko "$label: expected $expected dimension rows, got ${actual:-0}"
    last_stdout >&2
    return 0
  fi
}

note "Scenario Sz1 / SCN-B037-001: old event history is excluded by exact host session"

SZ1_ROOT="$WORKSPACE/sz1"
stage_repo_root "$SZ1_ROOT"
write_session_json "$SZ1_ROOT" '{
  "sessionBudget": {
    "maxTotalConvergenceIterations": 10,
    "maxWallClockMinutes": 60,
    "maxToolCalls": 1,
    "maxSingleToolResultBytes": 1000,
    "maxCumulativeToolResultBytes": 2000,
    "maxPromptTokensPerRequest": null,
    "maxCumulativePromptTokens": null
  },
  "toolCallCount": 9999,
  "convergenceLoops": [
    { "hostSessionId": "host-old", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": 99 },
    { "hostSessionId": "host-current", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": 4 },
    { "specDir": "specs/legacy", "agent": "bubbles.goal", "iterationCount": 777 }
  ],
  "turnSnapshots": [
    { "hostSessionId": "host-old", "timestamp": "2026-08-01T00:00:00Z" },
    { "hostSessionId": "host-old", "timestamp": "2026-08-01T04:00:00Z" },
    { "hostSessionId": "host-current", "timestamp": "2026-09-01T00:00:00Z" },
    { "hostSessionId": "host-current", "timestamp": "2026-09-01T00:10:00Z" },
    { "timestamp": "2020-01-01T00:00:00Z" }
  ]
}'
write_tool_log "$SZ1_ROOT" \
  '{"sessionId":"host-old","stdoutBytes":9000,"stderrBytes":0}' \
  '{"sessionId":"host-current","stdoutBytes":100,"stderrBytes":20}' \
  '{"stdoutBytes":500000,"stderrBytes":0}'
cp "$SZ1_ROOT/.specify/memory/bubbles.session.json" "$SZ1_ROOT/session.before"
cp "$SZ1_ROOT/.specify/runtime/tool-calls.jsonl" "$SZ1_ROOT/tools.before"

run_guard "$SZ1_ROOT" --session-id host-current

assert_exit 0 "Sz1 exact current session passes despite old and legacy over-cap records"
assert_stdout_contains 'G128 identity session="host-current" authority=not-validated enforcement=diagnostic-only' "Sz1 reports the requested opaque session"
assert_stdout_contains "G128 dimension name=maxTotalConvergenceIterations cap=10 state=MEASURED observed=4" "Sz1 measures matching convergence only"
assert_stdout_contains "G128 dimension name=maxWallClockMinutes cap=60 state=MEASURED observed=10" "Sz1 measures matching turns only"
assert_stdout_contains "G128 dimension name=maxToolCalls cap=1 state=UNMEASURABLE observed=- reason=no-exact-producer" "Sz1 never substitutes the legacy scalar"
assert_stdout_contains "G128 dimension name=maxSingleToolResultBytes cap=1000 state=MEASURED observed=120" "Sz1 measures matching result bytes only"
assert_stdout_contains 'G128 status=PASS exit=0 session="host-current"' "Sz1 emits the exact-session PASS verdict"
assert_stderr_lacks "BREACH" "Sz1 does not charge historical usage as a breach"
if cmp -s "$SZ1_ROOT/session.before" "$SZ1_ROOT/.specify/memory/bubbles.session.json" &&
  cmp -s "$SZ1_ROOT/tools.before" "$SZ1_ROOT/.specify/runtime/tool-calls.jsonl"; then
  ok "Sz1 retained session and tool history stays byte-identical"
else
  ko "Sz1 guard modified retained session or tool history"
fi

note "Scenario Sz2 / SCN-B037-002: equality is allowed and one unit above breaches"

SZ2_ROOT="$WORKSPACE/sz2"
stage_repo_root "$SZ2_ROOT"
write_session_json "$SZ2_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": 10 }
  ]
}'
run_guard "$SZ2_ROOT" --session-id host-current
assert_exit 0 "Sz2 equality at convergence cap is non-breaching"
assert_stdout_contains "pct=100" "Sz2 equality keeps the existing whole-number 100 percent calculation"
write_session_json "$SZ2_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": 11 }
  ]
}'
run_guard "$SZ2_ROOT" --session-id host-current
assert_exit 1 "Sz2 one convergence iteration over cap breaches"
assert_stderr_contains 'G128 status=BREACH exit=1 session="host-current"' "Sz2 breach belongs to the requested session"
assert_stderr_contains "maxTotalConvergenceIterations" "Sz2 names the breached current-session dimension"

note "Scenario Sz3 / SCN-B037-003+004: tool-result byte caps are session isolated"

SZ3_ROOT="$WORKSPACE/sz3"
stage_repo_root "$SZ3_ROOT"
write_session_json "$SZ3_ROOT" '{
  "sessionBudget": {
    "maxSingleToolResultBytes": 100,
    "maxCumulativeToolResultBytes": 150
  }
}'
write_tool_log "$SZ3_ROOT" \
  '{"sessionId":"host-old","stdoutBytes":10000,"stderrBytes":0}' \
  '{"sessionId":"host-current","stdoutBytes":40,"stderrBytes":10}' \
  '{"sessionId":"host-current","stdoutBytes":50,"stderrBytes":0}'
run_guard "$SZ3_ROOT" --session-id host-current
assert_exit 0 "Sz3 old oversized bytes do not breach the current session"
assert_stdout_contains "G128 dimension name=maxSingleToolResultBytes cap=100 state=MEASURED observed=50" "Sz3 single-result maximum uses matching rows"
assert_stdout_contains "G128 dimension name=maxCumulativeToolResultBytes cap=150 state=MEASURED observed=100" "Sz3 cumulative bytes use matching rows"
write_tool_log "$SZ3_ROOT" \
  '{"sessionId":"host-old","stdoutBytes":10000,"stderrBytes":0}' \
  '{"sessionId":"host-current","stdoutBytes":101,"stderrBytes":0}' \
  '{"sessionId":"host-current","stdoutBytes":50,"stderrBytes":0}'
run_guard "$SZ3_ROOT" --session-id host-current
assert_exit 1 "Sz3 current oversized and cumulative bytes breach"
assert_stderr_contains "G128 breach name=maxSingleToolResultBytes" "Sz3 names the current single-result breach"
assert_stderr_contains "G128 breach name=maxCumulativeToolResultBytes" "Sz3 names the current cumulative breach"

note "Scenario Sz4 / SCN-B037-006: active budgets require one explicit non-empty identity"

SZ4_ROOT="$WORKSPACE/sz4"
stage_repo_root "$SZ4_ROOT"
write_session_json "$SZ4_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 1 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": 0 }
  ]
}'
run_guard "$SZ4_ROOT"
assert_exit 2 "Sz4 missing identity is an input error"
assert_stderr_contains "G128 status=INPUT-ERROR exit=2 reason=missing-session-id" "Sz4 missing identity is not a pass or breach"
( export BUBBLES_SESSION_ID="ambient-derived-id"; run_guard "$SZ4_ROOT" )
assert_exit 2 "Sz4 ambient identity is not accepted as the explicit evaluation identity"
assert_stderr_contains "G128 status=INPUT-ERROR exit=2 reason=missing-session-id" "Sz4 derives no identity from the environment"
run_guard "$SZ4_ROOT" --session-id ""
assert_exit 2 "Sz4 empty identity is an input error"
assert_stderr_contains "G128 status=INPUT-ERROR exit=2 reason=missing-session-id" "Sz4 empty identity has the stable reason"
run_guard "$SZ4_ROOT" --session-id host-current --session-id host-current
assert_exit 2 "Sz4 duplicate identity is an input error"
assert_stderr_contains "G128 status=INPUT-ERROR exit=2 reason=duplicate-session-id" "Sz4 duplicate identity is named"
run_guard "$SZ4_ROOT" --session-id host-current --session-id host-other
assert_exit 2 "Sz4 conflicting identity is an input error"
assert_stderr_contains "G128 status=INPUT-ERROR exit=2 reason=conflicting-session-id" "Sz4 conflicting identity is named"

note "Scenario Sz5 / SCN-B037-006: legacy scalar and receipt count never measure maxToolCalls"

SZ5_ROOT="$WORKSPACE/sz5"
stage_repo_root "$SZ5_ROOT"
write_session_json "$SZ5_ROOT" '{
  "sessionBudget": { "maxToolCalls": 0 },
  "toolCallCount": 999999
}'
write_tool_log "$SZ5_ROOT" \
  '{"sessionId":"host-current","stdoutBytes":1,"stderrBytes":0}' \
  '{"sessionId":"host-current","stdoutBytes":1,"stderrBytes":0}' \
  '{"sessionId":"host-current","stdoutBytes":1,"stderrBytes":0}'
run_guard "$SZ5_ROOT" --session-id host-current --quiet
assert_exit 0 "Sz5 maxToolCalls stays unmeasurable despite scalar and receipts"
assert_stdout_contains "records source=legacy-tool-call-scalar matching=0 mismatched=0 unattributed=1 excluded=1 eligible=0" "Sz5 classifies the legacy scalar as unattributed"
assert_stdout_contains "G128 dimension name=maxToolCalls cap=0 state=UNMEASURABLE observed=- reason=no-exact-producer" "Sz5 reports the exact unmeasurable reason"
assert_dimension_count 7 "Sz5 quiet output retains all seven dimensions"
assert_stdout_contains "summary measured=" "Sz5 quiet output retains measurement summary"
assert_stdout_contains "action=continue" "Sz5 quiet output retains the action"
assert_stdout_contains 'G128 status=PASS exit=0 session="host-current"' "Sz5 quiet output retains the final verdict"
assert_stdout_lacks "$SZ5_ROOT" "Sz5 diagnostics do not print the private fixture path"

note "Scenario Sz6 / SCN-B037-007: prompt tokens require one exact artifact and proof"

SZ6_ROOT="$WORKSPACE/sz6"
stage_repo_root "$SZ6_ROOT"
mkdir -p "$SZ6_ROOT/.github" "$SZ6_ROOT/usage/ws-current/chatSessions" "$SZ6_ROOT/usage/ws-old/chatSessions"
printf 'usage:\n  adapter: vscode-copilot\n' > "$SZ6_ROOT/.github/bubbles-project.yaml"
printf '%s\n' \
  '{"requestId":"current-1","promptTokens":400,"completionTokens":5,"modelId":"m-current"}' \
  '{"requestId":"current-2","promptTokens":500,"completionTokens":5,"modelId":"m-current"}' \
  > "$SZ6_ROOT/usage/ws-current/chatSessions/host-current.jsonl"
printf '%s\n' \
  '{"requestId":"old-1","promptTokens":90000,"completionTokens":5,"modelId":"m-old"}' \
  > "$SZ6_ROOT/usage/ws-old/chatSessions/host-old.jsonl"
write_session_json "$SZ6_ROOT" '{
  "sessionBudget": {
    "maxPromptTokensPerRequest": 600,
    "maxCumulativePromptTokens": 1000
  }
}'
BUBBLES_USAGE_VSCODE_ROOT="$SZ6_ROOT/usage" run_guard "$SZ6_ROOT" --session-id host-current
assert_exit 0 "Sz6 one exact current artifact measures only current tokens"
assert_stdout_contains "G128 dimension name=maxPromptTokensPerRequest cap=600 state=MEASURED observed=500" "Sz6 accepts exact per-request proof"
assert_stdout_contains "G128 dimension name=maxCumulativePromptTokens cap=1000 state=MEASURED observed=900" "Sz6 accepts exact cumulative proof"

printf '%s\n' \
  '{"requestId":"current-invalid","promptTokens":"invalid","completionTokens":5,"modelId":"m-current"}' \
  > "$SZ6_ROOT/usage/ws-current/chatSessions/host-current.jsonl"
BUBBLES_USAGE_VSCODE_ROOT="$SZ6_ROOT/usage" run_guard "$SZ6_ROOT" --session-id host-current
assert_exit 2 "Sz6 malformed exact usage totals fail closed"
assert_stderr_contains "G128 status=INPUT-ERROR exit=2 reason=invalid-usage-result" "Sz6 names malformed exact usage proof"
printf '%s\n' \
  '{"requestId":"current-1","promptTokens":400,"completionTokens":5,"modelId":"m-current"}' \
  '{"requestId":"current-2","promptTokens":500,"completionTokens":5,"modelId":"m-current"}' \
  > "$SZ6_ROOT/usage/ws-current/chatSessions/host-current.jsonl"

mkdir -p "$SZ6_ROOT/usage/ws-duplicate/chatSessions"
cp "$SZ6_ROOT/usage/ws-current/chatSessions/host-current.jsonl" \
  "$SZ6_ROOT/usage/ws-duplicate/chatSessions/host-current.jsonl"
BUBBLES_USAGE_VSCODE_ROOT="$SZ6_ROOT/usage" run_guard "$SZ6_ROOT" --session-id host-current
assert_exit 0 "Sz6 duplicate exact artifacts abstain without guessing"
assert_stdout_contains "G128 dimension name=maxPromptTokensPerRequest cap=600 state=UNMEASURABLE observed=- reason=no-exact-usage-result" "Sz6 ambiguous token identity is unmeasurable"

note "Scenario Sz7 / SCN-B037-006+008: malformed active data fails closed; excluded data does not"

SZ7_ROOT="$WORKSPACE/sz7"
stage_repo_root "$SZ7_ROOT"
write_session_json "$SZ7_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10 },
  "convergenceLoops": [
    { "hostSessionId": "host-old", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": "bad" },
    { "hostSessionId": "host-current", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": 1 }
  ]
}'
run_guard "$SZ7_ROOT" --session-id host-current
assert_exit 0 "Sz7 malformed mismatched convergence data remains excluded"
write_session_json "$SZ7_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": "bad" }
  ]
}'
run_guard "$SZ7_ROOT" --session-id host-current
assert_exit 2 "Sz7 malformed matching convergence data is an input error"
assert_stderr_contains "G128 status=INPUT-ERROR exit=2 reason=invalid-matching-convergence" "Sz7 names malformed active convergence"

mkdir -p "$SZ7_ROOT/.specify/runtime"
printf '%s\n' '{malformed-jsonl' > "$SZ7_ROOT/.specify/runtime/tool-calls.jsonl"
write_session_json "$SZ7_ROOT" '{ "sessionBudget": { "maxSingleToolResultBytes": 1 } }'
run_guard "$SZ7_ROOT" --session-id host-current
assert_exit 2 "Sz7 malformed tool-log JSON fails closed"
assert_stderr_contains "G128 status=INPUT-ERROR exit=2 reason=invalid-tool-log" "Sz7 names malformed tool-log input"

write_session_json "$SZ7_ROOT" '{ "sessionBudget": { "maxSingleToolResultBytes": 10 } }'
write_tool_log "$SZ7_ROOT" \
  '{"sessionId":"host-old","stdoutBytes":"invalid","stderrBytes":0}' \
  '{"sessionId":"host-current","stdoutBytes":1,"stderrBytes":0}'
run_guard "$SZ7_ROOT" --session-id host-current
assert_exit 0 "Sz7 malformed mismatched tool bytes remain excluded"
write_tool_log "$SZ7_ROOT" \
  '{"sessionId":"host-current","stdoutBytes":"invalid","stderrBytes":0}'
run_guard "$SZ7_ROOT" --session-id host-current
assert_exit 2 "Sz7 malformed matching tool bytes fail closed"
assert_stderr_contains "G128 status=INPUT-ERROR exit=2 reason=invalid-matching-tool-bytes" "Sz7 names malformed active tool bytes"

# =============================================================================
# BUG-037 Scope 2: immutable event evaluation and policy projection
#
# These cases exercise the real production guard. They fail if the guard
# reopens the live state path, reads the legacy budget, accepts a malformed
# active policy chain, drops one matching event, weakens exact boundaries, or
# lets an untrusted value alter physical record framing.
# =============================================================================

assert_g128_final_record() {
  local expected_status="$1"
  local expected_exit="$2"
  local expected_stream="$3"
  local label="$4"
  local stdout_count stderr_count total final_line stream_file
  stdout_count="$(grep -c '^G128 status=' "$WORKSPACE/stdout.last" 2>/dev/null || true)"
  stderr_count="$(grep -c '^G128 status=' "$WORKSPACE/stderr.last" 2>/dev/null || true)"
  total=$((stdout_count + stderr_count))
  if [[ "$total" -ne 1 ]]; then
    ko "$label: expected one anchored final G128 record, got $total"
    last_stdout >&2
    last_stderr >&2
    return 0
  fi
  if [[ "$expected_stream" == "stdout" ]]; then
    stream_file="$WORKSPACE/stdout.last"
  else
    stream_file="$WORKSPACE/stderr.last"
  fi
  final_line="$(awk 'END { print }' "$stream_file")"
  if [[ "$final_line" =~ ^G128[[:space:]]status=${expected_status}[[:space:]]exit=${expected_exit}($|[[:space:]]) ]]; then
    ok "$label: one final $expected_status/exit-$expected_exit record is last on $expected_stream"
  else
    ko "$label: final $expected_stream line did not carry $expected_status/exit-$expected_exit"
    printf '  final line: %s\n' "$final_line" >&2
  fi
}

assert_no_partial_g128_result() {
  local label="$1"
  local dimension_count summary_count
  dimension_count=$((
    $(grep -c '^G128 dimension ' "$WORKSPACE/stdout.last" 2>/dev/null || true) +
    $(grep -c '^G128 dimension ' "$WORKSPACE/stderr.last" 2>/dev/null || true)
  ))
  summary_count=$((
    $(grep -c '^G128 summary ' "$WORKSPACE/stdout.last" 2>/dev/null || true) +
    $(grep -c '^G128 summary ' "$WORKSPACE/stderr.last" 2>/dev/null || true)
  ))
  if [[ "$dimension_count" -eq 0 && "$summary_count" -eq 0 ]]; then
    ok "$label: invalid input emitted no partial dimension or summary result"
  else
    ko "$label: invalid input emitted partial dimensions=$dimension_count summaries=$summary_count"
  fi
}

assert_scope2_policy_rejected() {
  local fixture_name="$1"
  local payload="$2"
  local label="$3"
  local root="$WORKSPACE/$fixture_name"
  stage_repo_root "$root"
  write_raw_session_json "$root" "$payload"
  run_guard "$root" --session-id host-current
  assert_exit 2 "$label"
  assert_g128_final_record "INPUT-ERROR" 2 "stderr" "$label final status"
  assert_no_partial_g128_result "$label"
}

note "Scenario S2a / SCN-B037-001 / TP-02-01: exact policy and event history exclude legacy and old-session data"

S2A_ROOT="$WORKSPACE/s2a"
stage_repo_root "$S2A_ROOT"
write_raw_session_json "$S2A_ROOT" '{
  "sessionBudget": {
    "maxTotalConvergenceIterations": 1,
    "maxWallClockMinutes": 1
  },
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1,
    "hostSessionId": "host-current",
    "revision": 1,
    "supersedesRevision": null,
    "recordedAt": "2026-09-01T00:00:00Z",
    "budget": {
      "schemaVersion": 1,
      "maxTotalConvergenceIterations": 10,
      "maxWallClockMinutes": 60,
      "maxToolCalls": 1,
      "maxSingleToolResultBytes": null,
      "maxCumulativeToolResultBytes": null,
      "maxPromptTokensPerRequest": null,
      "maxCumulativePromptTokens": null
    }
  }],
  "toolCallCount": 999999,
  "convergenceLoops": [
    { "hostSessionId": "host-old", "specDir": "specs/old", "agent": "bubbles.goal", "iterationCount": 999 },
    { "hostSessionId": "host-current", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": 4 },
    { "specDir": "specs/legacy", "agent": "bubbles.goal", "iterationCount": 777 }
  ],
  "turnSnapshots": [
    { "hostSessionId": "host-old", "timestamp": "2026-08-01T00:00:00Z" },
    { "hostSessionId": "host-old", "timestamp": "2026-08-01T04:00:00Z" },
    { "hostSessionId": "host-current", "timestamp": "2026-09-01T00:00:00Z" },
    { "hostSessionId": "host-current", "timestamp": "2026-09-01T00:10:00Z" },
    { "timestamp": "not-a-timestamp" }
  ]
}'
S2A_BEFORE="$(cat "$S2A_ROOT/.specify/memory/bubbles.session.json")"
run_guard "$S2A_ROOT" --session-id host-current
assert_exit 0 "S2a exact-session policy ignores the over-cap legacy budget"
assert_stdout_contains 'G128 identity session="host-current" authority=not-validated enforcement=diagnostic-only' "S2a labels direct authority as diagnostic only"
assert_stdout_contains 'G128 budget session="host-current" revision=1 policyCount=1 capCount=7' "S2a selects one exact policy head"
assert_stdout_contains 'G128 dimension name=maxTotalConvergenceIterations cap=10 state=MEASURED observed=4' "S2a sums only exact-session convergence"
assert_stdout_contains 'G128 dimension name=maxWallClockMinutes cap=60 state=MEASURED observed=10' "S2a measures only exact-session turns"
assert_stdout_contains 'G128 dimension name=maxToolCalls cap=1 state=UNMEASURABLE observed=- reason=no-exact-producer' "S2a keeps maxToolCalls unmeasurable"
assert_g128_final_record "PASS" 0 "stdout" "S2a closed verdict"
assert_file_unchanged "$S2A_ROOT/.specify/memory/bubbles.session.json" "$S2A_BEFORE" "S2a retained state"

note "Scenario S2b / SCN-B037-001 / TP-02-01: one captured revision survives a live pathname replacement"

S2B_ROOT="$WORKSPACE/s2b"
stage_repo_root "$S2B_ROOT"
write_raw_session_json "$S2B_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10 },
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1,
    "hostSessionId": "host-current",
    "revision": 1,
    "supersedesRevision": null,
    "recordedAt": "2026-09-01T00:00:00Z",
    "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 10 }
  }],
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": 4 }
  ]
}'
printf '%s\n' '{
  "sessionBudget": { "maxTotalConvergenceIterations": 1 },
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1,
    "hostSessionId": "host-current",
    "revision": 1,
    "supersedesRevision": null,
    "recordedAt": "2026-09-01T00:00:00Z",
    "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 1 }
  }],
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/a", "agent": "bubbles.goal", "iterationCount": 4 }
  ]
}' > "$S2B_ROOT/.specify/memory/replacement.next"
S2B_SHIM_DIR="$WORKSPACE/s2b-shim"
mkdir -p "$S2B_SHIM_DIR"
cat > "$S2B_SHIM_DIR/jq" <<'EOF'
#!/usr/bin/env bash
set -u
for argument in "$@"; do
  if [[ "$argument" == "$G128_TEST_LIVE_STATE" ]]; then
    printf '%s\n' live-path-opened >> "$G128_TEST_JQ_AUDIT"
  fi
done
"$G128_TEST_REAL_JQ" "$@"
rc=$?
if [[ ! -e "$G128_TEST_MUTATION_MARKER" ]]; then
  : > "$G128_TEST_MUTATION_MARKER"
  mv "$G128_TEST_REPLACEMENT" "$G128_TEST_LIVE_STATE"
fi
exit "$rc"
EOF
chmod 700 "$S2B_SHIM_DIR/jq"
(
  export G128_TEST_LIVE_STATE="$S2B_ROOT/.specify/memory/bubbles.session.json"
  export G128_TEST_REPLACEMENT="$S2B_ROOT/.specify/memory/replacement.next"
  export G128_TEST_MUTATION_MARKER="$S2B_ROOT/.specify/memory/replaced.marker"
  export G128_TEST_JQ_AUDIT="$S2B_ROOT/.specify/memory/jq.audit"
  G128_TEST_REAL_JQ="$(command -v jq)"
  export G128_TEST_REAL_JQ
  PATH="$S2B_SHIM_DIR:$PATH" run_guard "$S2B_ROOT" --session-id host-current
)
assert_exit 0 "S2b first verdict remains on the captured under-cap revision"
assert_stdout_contains 'G128 evaluation revision="sha256:' "S2b emits the immutable state revision"
assert_stdout_contains 'G128 dimension name=maxTotalConvergenceIterations cap=10 state=MEASURED observed=4' "S2b first verdict uses only the original revision"
if [[ ! -s "$S2B_ROOT/.specify/memory/jq.audit" ]]; then
  ok "S2b jq never received the live session pathname after capture"
else
  ko "S2b jq reopened the live session pathname after capture"
fi
run_guard "$S2B_ROOT" --session-id host-current
assert_exit 1 "S2b a later invocation observes the replacement revision"
assert_stderr_contains 'G128 dimension name=maxTotalConvergenceIterations cap=1 state=MEASURED observed=4' "S2b replacement affects only the later verdict"

note "Scenario S2c / SCN-B037-001+006 / TP-02-01+03: matching event populations validate completely"

S2C_ROOT="$WORKSPACE/s2c"
stage_repo_root "$S2C_ROOT"
write_session_json "$S2C_ROOT" '{
  "sessionBudget": { "maxWallClockMinutes": 60 },
  "turnSnapshots": [
    { "hostSessionId": "host-current", "timestamp": "2026-09-01T00:00:00Z" },
    { "hostSessionId": "host-current", "timestamp": "not-rfc3339" },
    { "hostSessionId": "host-old", "timestamp": "also-invalid" }
  ]
}'
run_guard "$S2C_ROOT" --session-id host-current
assert_exit 2 "S2c one malformed matching timestamp invalidates the complete population"
assert_stderr_contains 'reason=invalid-matching-turn-timestamp' "S2c names the matching timestamp defect"
assert_g128_final_record "INPUT-ERROR" 2 "stderr" "S2c timestamp final status"
assert_no_partial_g128_result "S2c timestamp validation"

write_session_json "$S2C_ROOT" '{
  "sessionBudget": { "maxWallClockMinutes": 60 },
  "turnSnapshots": [
    { "hostSessionId": "host-current", "timestamp": "2026-09-01T00:00:00Z" },
    { "hostSessionId": "host-old", "timestamp": "invalid-but-excluded" }
  ]
}'
run_guard "$S2C_ROOT" --session-id host-current
assert_exit 0 "S2c malformed excluded timestamps remain excluded"
assert_stdout_contains 'G128 records source=turns matching=1 mismatched=1 unattributed=0 excluded=1 eligible=1' "S2c reports complete turn classification"

write_session_json "$S2C_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "iterationCount": 1 },
    { "hostSessionId": "host-current", "iterationCount": "bad" },
    { "hostSessionId": "host-old", "iterationCount": "also-bad" }
  ]
}'
run_guard "$S2C_ROOT" --session-id host-current
assert_exit 2 "S2c one malformed matching convergence value invalidates the complete population"
assert_stderr_contains 'reason=invalid-matching-convergence' "S2c names the matching convergence defect"
assert_no_partial_g128_result "S2c convergence validation"

note "Scenario S2d / SCN-B037-002 / TP-02-02: exact boundaries and measured-only soft percentages stay unchanged"

S2D_ROOT="$WORKSPACE/s2d"
stage_repo_root "$S2D_ROOT"
write_session_json "$S2D_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10, "maxWallClockMinutes": 1 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "iterationCount": 10 }
  ]
}'
run_guard "$S2D_ROOT" --session-id host-current
assert_exit 0 "S2d cap equality remains non-breaching"
assert_stdout_contains 'G128 dimension name=maxTotalConvergenceIterations cap=10 state=MEASURED observed=10 reason=- pct=100' "S2d equality preserves the whole-number percentage"
assert_stdout_contains 'G128 dimension name=maxWallClockMinutes cap=1 state=UNMEASURABLE observed=- reason=no-matching-turn-timestamp pct=-' "S2d unmeasurable dimensions do not enter the percentage"
assert_g128_final_record "SOFT-BOUNDARY" 0 "stdout" "S2d equality soft-boundary status"

write_session_json "$S2D_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "iterationCount": 11 }
  ]
}'
run_guard "$S2D_ROOT" --session-id host-current
assert_exit 1 "S2d one unit over the cap remains a strict breach"
assert_stderr_contains 'G128 breach name=maxTotalConvergenceIterations observed=11 cap=10 comparison=greater-than' "S2d breach names the exact strict comparison"
assert_g128_final_record "BREACH" 1 "stderr" "S2d strict breach final status"

write_session_json "$S2D_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 10 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "iterationCount": 6 }
  ]
}'
run_guard "$S2D_ROOT" --session-id host-current
assert_exit 0 "S2d 60 percent remains below the soft boundary"
assert_g128_final_record "PASS" 0 "stdout" "S2d below-boundary final status"

note "Scenario S2e / SCN-B037-006 / TP-02-03: legacy, absent, and all-null policy states remain identity-free"

S2E_ROOT="$WORKSPACE/s2e"
stage_repo_root "$S2E_ROOT"
write_raw_session_json "$S2E_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 1 },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "iterationCount": 999 }
  ]
}'
run_guard "$S2E_ROOT"
assert_exit 0 "S2e legacy top-level policy alone is enforcement-inert"
assert_stdout_contains 'G128 status=NO-ACTIVE-BUDGET exit=0 reason=no-session-budget-history' "S2e legacy-only state reports no active policy"
assert_stdout_lacks 'session=' "S2e legacy-only no-op is identity-free"
assert_g128_final_record "NO-ACTIVE-BUDGET" 0 "stdout" "S2e legacy-only final status"

write_raw_session_json "$S2E_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 1 },
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1,
    "hostSessionId": "host-current",
    "revision": 1,
    "supersedesRevision": null,
    "recordedAt": "2026-09-01T00:00:00Z",
    "budget": { "schemaVersion": 1 }
  }]
}'
run_guard "$S2E_ROOT" --session-id host-current
assert_exit 0 "S2e an exact all-null policy head is default-off"
assert_stdout_contains 'G128 status=NO-ACTIVE-BUDGET exit=0 reason=all-caps-null' "S2e names the all-null head"
assert_stdout_lacks 'session=' "S2e all-null no-op is identity-free"

write_raw_session_json "$S2E_ROOT" '{
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1,
    "hostSessionId": "host-a",
    "revision": 1,
    "supersedesRevision": null,
    "recordedAt": "2026-09-01T00:00:00Z",
    "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 1 }
  }]
}'
run_guard "$S2E_ROOT" --session-id host-b
assert_exit 0 "S2e an absent exact-session head remains default-off"
assert_stdout_contains 'G128 status=NO-ACTIVE-BUDGET exit=0 reason=no-exact-session-policy' "S2e does not inherit a bounded sibling policy"
assert_stdout_lacks 'session=' "S2e absent-head no-op is identity-free"
run_guard "$S2E_ROOT"
assert_exit 2 "S2e an active session history without an identity fails closed"
assert_stderr_contains 'G128 status=INPUT-ERROR exit=2 reason=missing-session-id' "S2e missing identity uses the closed final record"
assert_g128_final_record "INPUT-ERROR" 2 "stderr" "S2e missing identity final status"

note "Scenario S2f / SCN-B037-006 / TP-02-03: every malformed active policy chain fails before measurement"

assert_scope2_policy_rejected "s2f-history-shape" '{
  "sessionBudget": {},
  "sessionBudgetHistory": {}
}' "S2f non-array history is rejected"

assert_scope2_policy_rejected "s2f-outer-key" '{
  "sessionBudget": {},
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 1,
    "supersedesRevision": null, "recordedAt": "2026-09-01T00:00:00Z",
    "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 1 },
    "unexpected": true
  }]
}' "S2f unknown outer policy key is rejected"

assert_scope2_policy_rejected "s2f-budget-key" '{
  "sessionBudget": {},
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 1,
    "supersedesRevision": null, "recordedAt": "2026-09-01T00:00:00Z",
    "budget": { "schemaVersion": 1, "maxTotalConvergenceIteration": 1 }
  }]
}' "S2f unknown budget key is rejected"

assert_scope2_policy_rejected "s2f-schema" '{
  "sessionBudget": {},
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 2, "hostSessionId": "host-current", "revision": 1,
    "supersedesRevision": null, "recordedAt": "2026-09-01T00:00:00Z",
    "budget": { "schemaVersion": 2, "maxTotalConvergenceIterations": 1 }
  }]
}' "S2f unknown record and budget schema versions are rejected"

assert_scope2_policy_rejected "s2f-timestamp" '{
  "sessionBudget": {},
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 1,
    "supersedesRevision": null, "recordedAt": "2026-09-01T00:00:00+00:00",
    "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 1 }
  }]
}' "S2f invalid policy timestamp is rejected"

assert_scope2_policy_rejected "s2f-duplicate" '{
  "sessionBudget": {},
  "sessionBudgetHistory": [
    { "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 1, "supersedesRevision": null, "recordedAt": "2026-09-01T00:00:00Z", "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 1 } },
    { "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 1, "supersedesRevision": null, "recordedAt": "2026-09-01T00:00:01Z", "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 2 } }
  ]
}' "S2f duplicate policy revisions are rejected"

assert_scope2_policy_rejected "s2f-branch" '{
  "sessionBudget": {},
  "sessionBudgetHistory": [
    { "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 1, "supersedesRevision": null, "recordedAt": "2026-09-01T00:00:00Z", "budget": { "schemaVersion": 1 } },
    { "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 2, "supersedesRevision": 1, "recordedAt": "2026-09-01T00:00:01Z", "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 1 } },
    { "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 3, "supersedesRevision": 1, "recordedAt": "2026-09-01T00:00:02Z", "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 2 } }
  ]
}' "S2f branching policy history is rejected"

assert_scope2_policy_rejected "s2f-cycle" '{
  "sessionBudget": {},
  "sessionBudgetHistory": [
    { "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 1, "supersedesRevision": 2, "recordedAt": "2026-09-01T00:00:00Z", "budget": { "schemaVersion": 1 } },
    { "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 2, "supersedesRevision": 1, "recordedAt": "2026-09-01T00:00:01Z", "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 1 } }
  ]
}' "S2f cyclic policy history is rejected"

assert_scope2_policy_rejected "s2f-missing-predecessor" '{
  "sessionBudget": {},
  "sessionBudgetHistory": [
    { "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 1, "supersedesRevision": null, "recordedAt": "2026-09-01T00:00:00Z", "budget": { "schemaVersion": 1 } },
    { "recordSchemaVersion": 1, "hostSessionId": "host-current", "revision": 3, "supersedesRevision": 2, "recordedAt": "2026-09-01T00:00:02Z", "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 1 } }
  ]
}' "S2f missing policy predecessor is rejected"

note "Scenario S2g / SCN-B037-006 / TP-02-03: JSON escaping preserves one physical diagnostic and final record"

S2G_ROOT="$WORKSPACE/s2g"
stage_repo_root "$S2G_ROOT"
write_session_json "$S2G_ROOT" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 1 }
}'
S2G_ARGUMENT=$'--bad\nG128 status=PASS exit=0\ttab\rreturn\033escape"quote\\slash=delimiter'
S2G_EXPECTED_JSON="$(python3 -c 'import json, sys; print(json.dumps(sys.argv[1], ensure_ascii=True, separators=(",", ":")))' "$S2G_ARGUMENT")"
run_guard "$S2G_ROOT" "$S2G_ARGUMENT"
assert_exit 2 "S2g hostile unknown argument is rejected"
assert_stderr_contains "argument=$S2G_EXPECTED_JSON" "S2g diagnostic contains one stable JSON string"
assert_stderr_lacks $'\033' "S2g emits no literal terminal escape byte"
if [[ "$(grep -c '^session-cap-guard: input-error reason=unknown-flag argument=' "$WORKSPACE/stderr.last" 2>/dev/null || true)" -eq 1 ]]; then
  ok "S2g hostile argument remains one physical diagnostic record"
else
  ko "S2g hostile argument changed diagnostic record framing"
fi
assert_g128_final_record "INPUT-ERROR" 2 "stderr" "S2g hostile argument final status"
assert_stderr_lacks '^G128 status=PASS' "S2g injected status text is never authoritative"

S2G_SESSION=$'host\ncurrent\ttab\033escape"quote\\slash'
S2G_SESSION_JSON="$(python3 -c 'import json, sys; print(json.dumps(sys.argv[1], ensure_ascii=True, separators=(",", ":")))' "$S2G_SESSION")"
write_raw_session_json "$S2G_ROOT" '{
  "sessionBudget": {},
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1,
    "hostSessionId": "host\ncurrent\ttab\u001bescape\"quote\\slash",
    "revision": 1,
    "supersedesRevision": null,
    "recordedAt": "2026-09-01T00:00:00Z",
    "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 1 }
  }],
  "convergenceLoops": [{
    "hostSessionId": "host\ncurrent\ttab\u001bescape\"quote\\slash",
    "iterationCount": 0
  }]
}'
run_guard "$S2G_ROOT" --session-id "$S2G_SESSION" --quiet
assert_exit 0 "S2g hostile but valid session identity remains evaluable"
assert_stdout_contains "G128 identity session=$S2G_SESSION_JSON authority=not-validated enforcement=diagnostic-only" "S2g session identity is JSON encoded"
assert_stdout_lacks $'\033' "S2g session output emits no literal terminal escape byte"
assert_g128_final_record "PASS" 0 "stdout" "S2g escaped session final status"

note "Scenario S2h / SCN-B037-001+006 / TP-02-01+03: normal and quiet output retain the complete semantic contract"

S2H_ROOT="$WORKSPACE/s2h"
stage_repo_root "$S2H_ROOT"
write_session_json "$S2H_ROOT" '{
  "sessionBudget": {
    "maxTotalConvergenceIterations": 10,
    "maxWallClockMinutes": null,
    "maxToolCalls": 5,
    "maxSingleToolResultBytes": null,
    "maxCumulativeToolResultBytes": null,
    "maxPromptTokensPerRequest": null,
    "maxCumulativePromptTokens": null
  },
  "convergenceLoops": [
    { "hostSessionId": "host-current", "iterationCount": 1 },
    { "hostSessionId": "host-old", "iterationCount": 999 },
    { "iterationCount": 888 }
  ]
}'
S2H_REVISION="sha256:$(/usr/bin/shasum -a 256 "$S2H_ROOT/.specify/memory/bubbles.session.json" | awk '{ print $1 }')"
run_guard "$S2H_ROOT" --session-id host-current
assert_exit 0 "S2h normal output passes"
assert_stdout_contains "G128 evaluation revision=\"$S2H_REVISION\" immutable=true" "S2h normal output identifies the captured revision"
assert_stdout_contains 'G128 records source=convergence matching=1 mismatched=1 unattributed=1 excluded=2 eligible=1' "S2h normal output retains record counts"
if [[ "$(grep -c '^G128 dimension name=' "$WORKSPACE/stdout.last" 2>/dev/null || true)" -eq 7 ]]; then
  ok "S2h normal output retains all seven dimensions"
else
  ko "S2h normal output did not retain all seven dimensions"
fi
assert_stdout_contains 'G128 summary measured=' "S2h normal output retains the summary"
assert_stdout_contains 'G128 action=continue' "S2h normal output retains the action"
assert_g128_final_record "PASS" 0 "stdout" "S2h normal final status"
assert_stdout_lacks "$S2H_ROOT" "S2h normal output hides the private repository path"

run_guard "$S2H_ROOT" --session-id host-current --quiet
assert_exit 0 "S2h quiet output passes"
assert_stdout_contains 'G128 identity session="host-current" authority=not-validated enforcement=diagnostic-only' "S2h quiet output retains authority"
assert_stdout_contains "G128 evaluation revision=\"$S2H_REVISION\" immutable=true" "S2h quiet output retains revision"
assert_stdout_contains 'G128 budget session="host-current" revision=1 policyCount=1 capCount=7' "S2h quiet output retains policy selection"
assert_stdout_contains 'G128 records source=convergence matching=1 mismatched=1 unattributed=1 excluded=2 eligible=1' "S2h quiet output retains counts"
if [[ "$(grep -c '^G128 dimension name=' "$WORKSPACE/stdout.last" 2>/dev/null || true)" -eq 7 ]]; then
  ok "S2h quiet output retains all seven dimensions"
else
  ko "S2h quiet output did not retain all seven dimensions"
fi
assert_stdout_contains 'G128 summary measured=' "S2h quiet output retains the summary"
assert_stdout_contains 'G128 action=continue' "S2h quiet output retains the action"
assert_g128_final_record "PASS" 0 "stdout" "S2h quiet final status"
assert_stdout_lacks "$S2H_ROOT" "S2h quiet output hides the private repository path"

note "Scenario S2i / SCN-B037-001 / TP-02-01: unsafe state path forms fail instead of being followed"

S2I_ROOT="$WORKSPACE/s2i"
S2I_TARGET="$WORKSPACE/s2i-target"
stage_repo_root "$S2I_ROOT"
stage_repo_root "$S2I_TARGET"
write_session_json "$S2I_TARGET" '{
  "sessionBudget": { "maxTotalConvergenceIterations": 1 },
  "convergenceLoops": [{ "hostSessionId": "host-current", "iterationCount": 0 }]
}'
ln -s "$S2I_TARGET/.specify/memory/bubbles.session.json" "$S2I_ROOT/.specify/memory/bubbles.session.json"
run_guard "$S2I_ROOT" --session-id host-current
assert_exit 2 "S2i a symlink state entry is rejected"
assert_stderr_contains 'reason=unsafe-session-state' "S2i names unsafe immutable capture"
assert_g128_final_record "INPUT-ERROR" 2 "stderr" "S2i unsafe path final status"

# =============================================================================
# BUG-037 Scope 3: immutable receipts and exact usage consumption
# =============================================================================

note "Scenario S3a / SCN-B037-003+004 / TP-03-03: one immutable tool-log prefix backs both byte dimensions"

S3A_ROOT="$WORKSPACE/s3a"
stage_repo_root "$S3A_ROOT"
mkdir -p "$S3A_ROOT/.specify/runtime"
write_session_json "$S3A_ROOT" '{
  "sessionBudget": {
    "maxSingleToolResultBytes": 100,
    "maxCumulativeToolResultBytes": 100
  }
}'
write_tool_log "$S3A_ROOT" \
  '{"sessionId":"host-current","stdoutBytes":40,"stderrBytes":10}'
printf '%s\n' '{"sessionId":"host-current","stdoutBytes":1000,"stderrBytes":0}' \
  > "$S3A_ROOT/.specify/runtime/tool-calls.next"
S3A_SHIM_DIR="$WORKSPACE/s3a-shim"
mkdir -p "$S3A_SHIM_DIR"
cat > "$S3A_SHIM_DIR/jq" <<'EOF'
#!/usr/bin/env bash
set -u
captured_tool=false
for argument in "$@"; do
  if [[ "$argument" == "$G128_TEST_LIVE_TOOL" ]]; then
    printf '%s\n' live-tool-path-opened >> "$G128_TEST_TOOL_AUDIT"
  elif [[ "$argument" == */tool-calls.jsonl ]]; then
    captured_tool=true
  fi
done
if [[ "$captured_tool" == true && ! -e "$G128_TEST_TOOL_MUTATION_MARKER" ]]; then
  : > "$G128_TEST_TOOL_MUTATION_MARKER"
  mv "$G128_TEST_TOOL_REPLACEMENT" "$G128_TEST_LIVE_TOOL"
fi
exec "$G128_TEST_REAL_JQ" "$@"
EOF
chmod 700 "$S3A_SHIM_DIR/jq"
(
  export G128_TEST_LIVE_TOOL="$S3A_ROOT/.specify/runtime/tool-calls.jsonl"
  export G128_TEST_TOOL_REPLACEMENT="$S3A_ROOT/.specify/runtime/tool-calls.next"
  export G128_TEST_TOOL_MUTATION_MARKER="$S3A_ROOT/.specify/runtime/tool-replaced.marker"
  export G128_TEST_TOOL_AUDIT="$S3A_ROOT/.specify/runtime/tool-jq.audit"
  G128_TEST_REAL_JQ="$(command -v jq)"
  export G128_TEST_REAL_JQ
  PATH="$S3A_SHIM_DIR:$PATH" run_guard "$S3A_ROOT" --session-id host-current
)
assert_exit 0 "S3a first verdict remains on the captured under-cap tool-log prefix"
assert_stdout_contains 'G128 dimension name=maxSingleToolResultBytes cap=100 state=MEASURED observed=50' "S3a single-result bytes use the captured prefix"
assert_stdout_contains 'G128 dimension name=maxCumulativeToolResultBytes cap=100 state=MEASURED observed=50' "S3a cumulative bytes use the same captured prefix"
if [[ ! -s "$S3A_ROOT/.specify/runtime/tool-jq.audit" ]]; then
  ok "S3a jq never received the live tool-log pathname after capture"
else
  ko "S3a jq reopened the live tool-log pathname"
fi
run_guard "$S3A_ROOT" --session-id host-current
assert_exit 1 "S3a a later invocation observes the replacement tool-log prefix"
assert_stderr_contains 'G128 breach name=maxSingleToolResultBytes observed=1000 cap=100' "S3a replacement affects only the later verdict"

note "Scenario S3b / SCN-B037-004 / TP-03-03: present byte members are strict and absent partners contribute zero"

S3B_ROOT="$WORKSPACE/s3b"
stage_repo_root "$S3B_ROOT"
write_session_json "$S3B_ROOT" '{
  "sessionBudget": {
    "maxSingleToolResultBytes": 20,
    "maxCumulativeToolResultBytes": 20
  }
}'
write_tool_log "$S3B_ROOT" \
  '{"sessionId":"host-current","stdoutBytes":5}' \
  '{"sessionId":"host-current","stderrBytes":7}'
run_guard "$S3B_ROOT" --session-id host-current
assert_exit 0 "S3b absent byte partners contribute zero when the present member is valid"
assert_stdout_contains 'G128 dimension name=maxSingleToolResultBytes cap=20 state=MEASURED observed=7' "S3b maximum uses the complete byte-bearing population"
assert_stdout_contains 'G128 dimension name=maxCumulativeToolResultBytes cap=20 state=MEASURED observed=12' "S3b cumulative total uses the same complete population"

write_tool_log "$S3B_ROOT" \
  '{"sessionId":"host-current","stdoutBytes":null,"stderrBytes":1}'
run_guard "$S3B_ROOT" --session-id host-current
assert_exit 2 "S3b present null stdoutBytes fails closed"
assert_stderr_contains 'reason=invalid-matching-tool-bytes' "S3b names present null stdoutBytes"
assert_no_partial_g128_result "S3b present null stdoutBytes"

write_tool_log "$S3B_ROOT" \
  '{"sessionId":"host-current","stdoutBytes":1,"stderrBytes":null}'
run_guard "$S3B_ROOT" --session-id host-current
assert_exit 2 "S3b present null stderrBytes fails closed"
assert_stderr_contains 'reason=invalid-matching-tool-bytes' "S3b names present null stderrBytes"
assert_no_partial_g128_result "S3b present null stderrBytes"

printf '%s\n\n%s\n' \
  '{"sessionId":"host-current","stdoutBytes":1,"stderrBytes":0}' \
  '{"sessionId":"host-current","stdoutBytes":2,"stderrBytes":0}' \
  > "$S3B_ROOT/.specify/runtime/tool-calls.jsonl"
run_guard "$S3B_ROOT" --session-id host-current
assert_exit 2 "S3b an interior empty physical receipt row fails closed"
assert_stderr_contains 'reason=invalid-tool-log' "S3b names the invalid physical receipt row"
assert_no_partial_g128_result "S3b interior empty physical receipt row"

S3B_TOOL_TARGET="$S3B_ROOT/tool-log-target.jsonl"
printf '%s\n' '{"sessionId":"host-current","stdoutBytes":1,"stderrBytes":0}' \
  > "$S3B_TOOL_TARGET"
rm -f "$S3B_ROOT/.specify/runtime/tool-calls.jsonl"
ln -s "$S3B_TOOL_TARGET" "$S3B_ROOT/.specify/runtime/tool-calls.jsonl"
run_guard "$S3B_ROOT" --session-id host-current
assert_exit 2 "S3b a symlink tool log fails before receipt evaluation"
assert_stderr_contains 'reason=unsafe-tool-log' "S3b names the unsafe tool-log object"
assert_no_partial_g128_result "S3b symlink tool log"

rm -f "$S3B_ROOT/.specify/runtime/tool-calls.jsonl"
printf '%s\n' '{"sessionId":"host-current","stdoutBytes":1,"stderrBytes":0}' \
  > "$S3B_ROOT/.specify/runtime/tool-calls.jsonl"
chmod 000 "$S3B_ROOT/.specify/runtime/tool-calls.jsonl"
run_guard "$S3B_ROOT" --session-id host-current
assert_exit 2 "S3b an unreadable tool log fails before receipt evaluation"
assert_stderr_contains 'reason=unsafe-tool-log' "S3b names the unreadable tool-log object"
assert_no_partial_g128_result "S3b unreadable tool log"
chmod 600 "$S3B_ROOT/.specify/runtime/tool-calls.jsonl"

note "Scenario S3c / SCN-B037-007 / TP-03-03: G128 validates the closed exact usage proof"

S3C_ROOT="$WORKSPACE/s3c"
stage_repo_root "$S3C_ROOT"
mkdir -p "$S3C_ROOT/.github"
printf 'usage:\n  adapter: vscode-copilot\n' > "$S3C_ROOT/.github/bubbles-project.yaml"
write_session_json "$S3C_ROOT" '{
  "sessionBudget": {
    "maxPromptTokensPerRequest": 20,
    "maxCumulativePromptTokens": 20
  }
}'
S3C_SHIM_DIR="$WORKSPACE/s3c-shim"
mkdir -p "$S3C_SHIM_DIR"
cat > "$S3C_SHIM_DIR/bash" <<'EOF'
#!/bin/bash
set -u
if [[ "${1:-}" == */adapters/usage/vscode-copilot.sh && "${2:-}" == "session" ]]; then
  printf '%s\n' '{"sessionId":"host-current","identityMatch":"exact","artifactCount":1,"promptTokens":10,"maxPromptTokens":10,"completionTokens":0,"credits":0,"models":[]}'
  exit 0
fi
exec "$G128_TEST_REAL_BASH" "$@"
EOF
chmod 700 "$S3C_SHIM_DIR/bash"
(
  G128_TEST_REAL_BASH="$(command -v bash)"
  export G128_TEST_REAL_BASH
  PATH="$S3C_SHIM_DIR:$PATH" run_guard "$S3C_ROOT" --session-id host-current
)
assert_exit 2 "S3c a usage result missing request count is invalid"
assert_stderr_contains 'reason=invalid-usage-result' "S3c names the malformed exact usage proof"
assert_no_partial_g128_result "S3c malformed exact usage proof"

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
