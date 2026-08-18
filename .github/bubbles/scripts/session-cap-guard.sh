#!/usr/bin/env bash
set -euo pipefail

# session-cap-guard.sh
#
# Gate G128 — session_cap_enforcement_gate.
#
# Mechanically enforces the IMP-003 `sessionBudget` aggregate safety caps
# for a whole goal/sprint SESSION. It is the AGGREGATE sibling of Gate
# G082 (`convergence-cap-guard.sh`):
#
#   * G082 caps convergence iterations PER (specDir, agent) — a per-spec
#     ceiling read from `bubbles/workflows.yaml` `maxConvergenceIterations`.
#   * G128 (this gate) caps the AGGREGATE usage across the WHOLE session —
#     every spec, every agent — read from a `sessionBudget` object recorded
#     in `.specify/memory/bubbles.session.json`.
#
# The active budget is whatever the running session recorded under
# `sessionBudget` in the session file. Its three dimensions are:
#
#   maxTotalConvergenceIterations  aggregate sum of `convergenceLoops[].iterationCount`
#   maxWallClockMinutes            earliest → latest `turnSnapshots[].timestamp`, in minutes
#   maxToolCalls                   aggregate `toolCallCount` counter
#
# DEFAULT-OFF (no-op) for every existing repo: if the session file has no
# `sessionBudget`, or ALL THREE caps are null/absent, the guard exits 0 and
# nothing is enforced. A dimension whose cap is null is not enforced; a
# dimension whose cap is set but whose usage data is absent (no
# `turnSnapshots[]`, no `toolCallCount`) is skipped (you cannot breach what
# you cannot measure). Only when a non-null cap is present AND its aggregate
# is measurable does the guard compare them.
#
# Exit codes:
#   0  no active budget (no-op) OR no present cap exceeded by its aggregate
#      (INCLUDING a crossed SOFT boundary — see below; a soft boundary is a
#      recommendation, not a refusal)
#   1  an active cap exceeded — orchestrator MUST emit a `blocked`
#      RESULT-ENVELOPE with finding G128 and STOP the session; stderr names
#      the breached dimension(s) and observed-vs-cap
#   2  malformed / missing inputs (unparseable session.json, non-integer
#      cap or counter), or bad usage — diagnostic on stderr
#
# SOFT BOUNDARY AT 70% (IMP-048 SCOPE-6 / COST-9)
# The hard stop was the guard's ONLY outcome, so a session learned it was over
# budget at the moment it was refused, with nothing preserved and no warning it
# could have acted on. A soft boundary fires when the most-consumed measurable
# dimension reaches 70% of its cap: the guard reports the crossing on STDOUT,
# asks `session-review.sh` for a Class C `handoff-to-fresh-session`
# recommendation, and CONTINUES with exit 0. The hard stop at 100% is
# unchanged.
#
# A rollover is NOT a blocked spec. The work is fine; the SESSION is full. The
# guard therefore never touches any `state.json`, and the soft-boundary block
# says so in the output, because "budget exhausted" being recorded as "work
# blocked" is how a healthy spec acquires a false terminal status. The owed
# response is a continuation envelope plus a `bubbles.handoff` packet.
#
# The recommendation reuses the EXISTING Class C surface rather than inventing
# a second one: `session-review.sh emit --trigger budget-threshold --budget-pct
# <pct> --class-c handoff-to-fresh-session=<pct>`. That surface already
# deduplicates (re-emitting only when the metric worsens by 25%) and already
# fires its 50/70/90 bands once each, so a long session is warned rather than
# nagged. With the default `sessionReview.adapter: none` the call is a clean
# no-op and the guard simply reports the recommendation as unrecorded.
#
# Usage:
#   bash bubbles/scripts/session-cap-guard.sh [--quiet]
#
# Optional:
#   --quiet     Suppress informational stdout on success (the PASS line is
#               always written to stdout; informational lines suppressed).
#
# There is NO `--skip` / `--force` / `--ignore` bypass (matches G082).
#
# Dependencies:
#   - jq      (hard dependency; also parses RFC3339 timestamps via
#             `fromdateiso8601`, so no GNU/BSD `date` divergence)
#
# Schema (additive in bubbles.session.json):
#   {
#     "sessionBudget": {
#       "maxTotalConvergenceIterations": <int|null>,
#       "maxWallClockMinutes":           <int|null>,
#       "maxToolCalls":                  <int|null>
#     },
#     "convergenceLoops": [ { "iterationCount": <int>, ... }, ... ],
#     "turnSnapshots":    [ { "timestamp": "<RFC3339>", ... }, ... ],
#     "toolCallCount":    <int>
#   }
#
# CONTEXT-VOLUME DIMENSIONS (IMP-039 SCOPE-3)
# The three original dimensions cannot see how much text a session carries. A
# run can hold every one of them and still replay 1.77 MB of terminal records
# into every later request, which is what the measured session behind IMP-039
# did. Four OPTIONAL caps close that blind spot, each defaulting to null so
# every existing repository keeps the current no-op posture:
#
#   maxSingleToolResultBytes      largest single tool result retained
#   maxCumulativeToolResultBytes  sum of all retained tool results
#   maxPromptTokensPerRequest     largest single request's prompt tokens
#   maxCumulativePromptTokens     sum of prompt tokens across the session
#
# MEASURABILITY DIFFERS BY DIMENSION, and the existing rule applies unchanged:
# a cap whose dimension cannot be measured is SKIPPED, never guessed.
#   - Byte dimensions read `.specify/runtime/tool-calls.jsonl`
#     (`stdoutBytes` + `stderrBytes` per record). No adapter needed.
#   - Token dimensions require a configured usage adapter
#     (`bubbles/adapters/usage/`, default `none`). With `none` there is no
#     honest token number, so those caps are skipped rather than compared
#     against a fabricated figure.
#
# Reference: improvements/IMP-003-autonomy-dial-and-safety-caps.md (SCOPE-2),
#            IMP-039 SCOPE-3 (context-volume dimensions)


QUIET="false"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# The soft boundary. Deliberately a single number: a per-dimension threshold
# would let the noisiest dimension pick its own warning point, and the whole
# value of one boundary is that the operator learns "this session is filling up"
# once, not seven times at seven different moments.
SOFT_BOUNDARY_PCT=70

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/session-cap-guard.sh [--quiet]

Enforces the IMP-003 aggregate sessionBudget caps recorded in
.specify/memory/bubbles.session.json (the AGGREGATE sibling of Gate G082).

Optional:
  --quiet     Suppress informational stdout; the final PASS or VIOLATION
              line is still emitted (stdout on pass, stderr on fail).
  -h, --help  Print this usage and exit.

Exit codes:
  0 = no active budget (no-op) or no cap exceeded
  1 = an active cap exceeded (Gate G128 violation)
  2 = malformed inputs or bad usage

No --skip / --force / --ignore bypass exists.
EOF
}

# --- Argument parsing ----------------------------------------------------

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --*)
      echo "session-cap-guard: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      echo "session-cap-guard: unexpected positional argument: $1" >&2
      echo "  (G128 is an aggregate gate; it takes no specDir — see G082 for the per-spec cap)" >&2
      usage >&2
      exit 2
      ;;
  esac
done

info() {
  if [[ "$QUIET" != "true" ]]; then
    echo "session-cap-guard: $*"
  fi
}

# --- jq dependency check -------------------------------------------------

if ! command -v jq >/dev/null 2>&1; then
  echo "session-cap-guard: jq is required but not found in PATH" >&2
  exit 2
fi

# --- Repo root resolution ------------------------------------------------

resolve_repo_root() {
  if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
    printf '%s' "$BUBBLES_REPO_ROOT"
    return 0
  fi
  local dir
  dir="$(pwd)"
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/.specify/memory" ]]; then
      printf '%s' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  return 1
}

REPO_ROOT="$(resolve_repo_root || true)"
if [[ -z "$REPO_ROOT" ]]; then
  echo "session-cap-guard: unable to resolve repo root (no .specify/memory found)" >&2
  echo "  Set BUBBLES_REPO_ROOT explicitly or run from inside a Bubbles repo." >&2
  exit 2
fi

# --- Locate session.json -------------------------------------------------

SESSION_FILE="$REPO_ROOT/.specify/memory/bubbles.session.json"
if [[ ! -f "$SESSION_FILE" ]]; then
  # No session file recorded yet — no aggregate budget to enforce.
  info "no $SESSION_FILE present; nothing to enforce"
  echo "PASS Gate G128 (session_cap_enforcement_gate) — no session budget recorded (no session.json)"
  exit 0
fi

# --- Validate session.json is parseable JSON -----------------------------

if ! jq empty "$SESSION_FILE" >/dev/null 2>&1; then
  echo "session-cap-guard: $SESSION_FILE is not valid JSON" >&2
  exit 2
fi

# --- No-op unless a sessionBudget object is present ----------------------

if ! jq -e '.sessionBudget != null' "$SESSION_FILE" >/dev/null 2>&1; then
  info "no sessionBudget recorded in $SESSION_FILE; nothing to enforce"
  echo "PASS Gate G128 (session_cap_enforcement_gate) — no sessionBudget recorded"
  exit 0
fi

# --- Extract the raw caps ("null" when absent) ---------------------------

CAP_CONV="$(jq -r '.sessionBudget.maxTotalConvergenceIterations // "null"' "$SESSION_FILE")"
CAP_MINS="$(jq -r '.sessionBudget.maxWallClockMinutes // "null"' "$SESSION_FILE")"
CAP_TOOLS="$(jq -r '.sessionBudget.maxToolCalls // "null"' "$SESSION_FILE")"
CAP_SINGLE_BYTES="$(jq -r '.sessionBudget.maxSingleToolResultBytes // "null"' "$SESSION_FILE")"
CAP_CUM_BYTES="$(jq -r '.sessionBudget.maxCumulativeToolResultBytes // "null"' "$SESSION_FILE")"
CAP_REQ_TOKENS="$(jq -r '.sessionBudget.maxPromptTokensPerRequest // "null"' "$SESSION_FILE")"
CAP_CUM_TOKENS="$(jq -r '.sessionBudget.maxCumulativePromptTokens // "null"' "$SESSION_FILE")"

# All caps null/absent → no-op (the default for every existing repo).
if [[ "$CAP_CONV" == "null" && "$CAP_MINS" == "null" && "$CAP_TOOLS" == "null" &&
  "$CAP_SINGLE_BYTES" == "null" && "$CAP_CUM_BYTES" == "null" &&
  "$CAP_REQ_TOKENS" == "null" && "$CAP_CUM_TOKENS" == "null" ]]; then
  info "sessionBudget present but every cap is null; nothing to enforce"
  echo "PASS Gate G128 (session_cap_enforcement_gate) — sessionBudget has no non-null cap"
  exit 0
fi

# --- Validate every PRESENT cap is a non-negative integer ----------------

validate_cap() {
  local label="$1" value="$2"
  if [[ "$value" == "null" ]]; then
    return 0
  fi
  if ! [[ "$value" =~ ^[0-9]+$ ]]; then
    echo "session-cap-guard: sessionBudget.$label must be a non-negative integer, got: $value" >&2
    exit 2
  fi
}

validate_cap "maxTotalConvergenceIterations" "$CAP_CONV"
validate_cap "maxWallClockMinutes" "$CAP_MINS"
validate_cap "maxToolCalls" "$CAP_TOOLS"
validate_cap "maxSingleToolResultBytes" "$CAP_SINGLE_BYTES"
validate_cap "maxCumulativeToolResultBytes" "$CAP_CUM_BYTES"
validate_cap "maxPromptTokensPerRequest" "$CAP_REQ_TOKENS"
validate_cap "maxCumulativePromptTokens" "$CAP_CUM_TOKENS"

# --- Compute the aggregate usage across ALL specs in one jq pass ---------
#
# Timestamps are parsed with jq's `fromdateiso8601`, NOT the system `date`,
# so wall-clock math is identical on GNU (WSL/Linux) and BSD (macOS)
# userland. Non-numeric convergence entries are coerced to 0 rather than
# crashing jq; the final integer validations below still catch a malformed
# aggregate.

AGG_JSON="$(jq -c '
  {
    convObserved: (
      (.convergenceLoops // [])
      | map(.iterationCount // 0)
      | map(if type == "number" then . else 0 end)
      | add // 0
    ),
    toolPresent:  ((has("toolCallCount")) and (.toolCallCount != null)),
    toolObserved: (.toolCallCount // null),
    toolType:     (.toolCallCount | type),
    minObserved: (
      (
        (.turnSnapshots // [])
        | map(.timestamp // empty)
        | map((try fromdateiso8601) // empty)
      ) as $ts
      | if ($ts | length) >= 1 then (($ts | max) - ($ts | min)) / 60 else null end
    )
  }
' "$SESSION_FILE" 2>/dev/null || true)"

if [[ -z "$AGG_JSON" ]] || ! echo "$AGG_JSON" | jq empty >/dev/null 2>&1; then
  echo "session-cap-guard: failed to compute aggregate usage from $SESSION_FILE" >&2
  exit 2
fi

CONV_OBSERVED="$(echo "$AGG_JSON" | jq -r '.convObserved')"
TOOL_PRESENT="$(echo "$AGG_JSON" | jq -r '.toolPresent')"
TOOL_OBSERVED="$(echo "$AGG_JSON" | jq -r '.toolObserved')"
TOOL_TYPE="$(echo "$AGG_JSON" | jq -r '.toolType')"
MIN_OBSERVED="$(echo "$AGG_JSON" | jq -r '.minObserved')"

# --- Validate the measured aggregates ------------------------------------

if ! [[ "$CONV_OBSERVED" =~ ^[0-9]+$ ]]; then
  echo "session-cap-guard: malformed aggregate convergence count in session.json: $CONV_OBSERVED" >&2
  exit 2
fi

if [[ "$TOOL_PRESENT" == "true" ]]; then
  if [[ "$TOOL_TYPE" != "number" ]] || ! [[ "$TOOL_OBSERVED" =~ ^[0-9]+$ ]]; then
    echo "session-cap-guard: toolCallCount must be a non-negative integer, got: $TOOL_OBSERVED (type=$TOOL_TYPE)" >&2
    exit 2
  fi
fi

if [[ "$MIN_OBSERVED" != "null" ]]; then
  if ! [[ "$MIN_OBSERVED" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    echo "session-cap-guard: malformed wall-clock minutes computed from turnSnapshots: $MIN_OBSERVED" >&2
    exit 2
  fi
fi

# --- Context-volume measurement (IMP-039 SCOPE-3) ------------------------
#
# Byte dimensions come from the tool-call log, which already records
# stdoutBytes/stderrBytes per call. "null" means UNMEASURABLE (no log, or no
# parsable record) and the corresponding cap is skipped, matching how the
# wall-clock dimension already behaves when turnSnapshots is empty.

TOOL_LOG="$REPO_ROOT/.specify/runtime/tool-calls.jsonl"
SINGLE_BYTES_OBSERVED="null"
CUM_BYTES_OBSERVED="null"

if [[ -f "$TOOL_LOG" ]]; then
  BYTES_JSON="$(jq -s -c '
    [ .[] | objects | select(has("stdoutBytes") or has("stderrBytes"))
          | ((.stdoutBytes // 0) + (.stderrBytes // 0))
          | if type == "number" then . else 0 end ] as $b
    | if ($b | length) == 0 then {max: null, sum: null}
      else {max: ($b | max), sum: ($b | add)}
      end
  ' "$TOOL_LOG" 2>/dev/null || true)"
  if [[ -n "$BYTES_JSON" ]] && echo "$BYTES_JSON" | jq empty >/dev/null 2>&1; then
    SINGLE_BYTES_OBSERVED="$(echo "$BYTES_JSON" | jq -r '.max // "null"')"
    CUM_BYTES_OBSERVED="$(echo "$BYTES_JSON" | jq -r '.sum // "null"')"
  fi
fi

# Token dimensions require a configured usage adapter. With the default `none`
# there is no honest number, so these stay "null" and their caps are skipped.
# Guessing here would reintroduce exactly the fabrication IMP-039 SCOPE-2 exists
# to forbid.
REQ_TOKENS_OBSERVED="null"
CUM_TOKENS_OBSERVED="null"

if [[ "$CAP_REQ_TOKENS" != "null" || "$CAP_CUM_TOKENS" != "null" ]]; then
  USAGE_RESOLVE="$SCRIPT_DIR/usage-resolve.sh"
  if [[ -x "$USAGE_RESOLVE" || -f "$USAGE_RESOLVE" ]]; then
    USAGE_ADAPTER_PATH="$(bash "$USAGE_RESOLVE" --repo-root "$REPO_ROOT" 2>/dev/null |
      awk -F= '$1 == "adapterPath" { print $2 }' || true)"
    USAGE_ADAPTER_NAME="$(bash "$USAGE_RESOLVE" --repo-root "$REPO_ROOT" --names-only 2>/dev/null |
      awk -F= '$1 == "adapter" { print $2 }' || true)"
    if [[ -n "$USAGE_ADAPTER_PATH" && "$USAGE_ADAPTER_NAME" != "none" ]]; then
      USAGE_SESSION="$(bash "$USAGE_ADAPTER_PATH" session 2>/dev/null || true)"
      if [[ -n "$USAGE_SESSION" ]] && echo "$USAGE_SESSION" | jq empty >/dev/null 2>&1; then
        REQ_TOKENS_OBSERVED="$(echo "$USAGE_SESSION" | jq -r '.maxPromptTokens // "null"')"
        CUM_TOKENS_OBSERVED="$(echo "$USAGE_SESSION" | jq -r '.promptTokens // "null"')"
      fi
    fi
  fi
fi

validate_observed() {
  local label="$1" value="$2"
  [[ "$value" == "null" ]] && return 0
  if ! [[ "$value" =~ ^[0-9]+$ ]]; then
    echo "session-cap-guard: malformed $label measurement: $value" >&2
    exit 2
  fi
}

validate_observed "single tool-result bytes" "$SINGLE_BYTES_OBSERVED"
validate_observed "cumulative tool-result bytes" "$CUM_BYTES_OBSERVED"
validate_observed "per-request prompt tokens" "$REQ_TOKENS_OBSERVED"
validate_observed "cumulative prompt tokens" "$CUM_TOKENS_OBSERVED"

# --- Decision: check every PRESENT + MEASURABLE dimension ----------------

declare -a BREACHES=()

# Consumption is recorded for exactly the dimensions the breach checks below
# consider: present cap AND measurable aggregate. A dimension that cannot be
# measured contributes no percentage, for the same reason it contributes no
# breach — a soft boundary derived from an unmeasured dimension would be the
# fabricated number this guard already refuses to produce for tokens.
declare -a CONSUMPTION=()

record_consumption() {
  local label="$1" observed="$2" cap="$3" pct
  [[ "$cap" == "null" || "$observed" == "null" ]] && return 0
  # awk, not shell arithmetic: wall-clock minutes carry a fractional part.
  pct="$(awk -v o="$observed" -v c="$cap" 'BEGIN {
    if (c + 0 <= 0) { print (o + 0 > 0) ? 100 : 0; exit }
    p = int(o * 100 / c)
    if (p < 0) p = 0
    print p
  }')"
  CONSUMPTION+=("$pct|$label|$observed|$cap")
}

# Convergence: aggregate is always measurable (defaults to 0).
if [[ "$CAP_CONV" != "null" ]] && [[ "$CONV_OBSERVED" -gt "$CAP_CONV" ]]; then
  BREACHES+=("convergence: aggregate iterationCount=$CONV_OBSERVED > maxTotalConvergenceIterations=$CAP_CONV")
fi

# Tool calls: only when the aggregate counter is present.
if [[ "$CAP_TOOLS" != "null" ]] && [[ "$TOOL_PRESENT" == "true" ]] && [[ "$TOOL_OBSERVED" -gt "$CAP_TOOLS" ]]; then
  BREACHES+=("toolCalls: aggregate toolCallCount=$TOOL_OBSERVED > maxToolCalls=$CAP_TOOLS")
fi

# Wall-clock minutes: only when turnSnapshots yielded a measurable span.
# Float-safe comparison via awk (minutes may carry a fractional part).
if [[ "$CAP_MINS" != "null" ]] && [[ "$MIN_OBSERVED" != "null" ]]; then
  if awk -v a="$MIN_OBSERVED" -v b="$CAP_MINS" 'BEGIN { exit !(a > b) }'; then
    BREACHES+=("wallClockMinutes: elapsed=$MIN_OBSERVED min > maxWallClockMinutes=$CAP_MINS")
  fi
fi

# Context volume (IMP-039 SCOPE-3). Each pair is checked only when the cap is
# set AND the dimension was actually measured.
if [[ "$CAP_SINGLE_BYTES" != "null" && "$SINGLE_BYTES_OBSERVED" != "null" ]] &&
  [[ "$SINGLE_BYTES_OBSERVED" -gt "$CAP_SINGLE_BYTES" ]]; then
  BREACHES+=("singleToolResultBytes: largest retained tool result=$SINGLE_BYTES_OBSERVED > maxSingleToolResultBytes=$CAP_SINGLE_BYTES")
fi

if [[ "$CAP_CUM_BYTES" != "null" && "$CUM_BYTES_OBSERVED" != "null" ]] &&
  [[ "$CUM_BYTES_OBSERVED" -gt "$CAP_CUM_BYTES" ]]; then
  BREACHES+=("cumulativeToolResultBytes: retained tool results=$CUM_BYTES_OBSERVED > maxCumulativeToolResultBytes=$CAP_CUM_BYTES")
fi

if [[ "$CAP_REQ_TOKENS" != "null" && "$REQ_TOKENS_OBSERVED" != "null" ]] &&
  [[ "$REQ_TOKENS_OBSERVED" -gt "$CAP_REQ_TOKENS" ]]; then
  BREACHES+=("promptTokensPerRequest: largest request=$REQ_TOKENS_OBSERVED > maxPromptTokensPerRequest=$CAP_REQ_TOKENS")
fi

if [[ "$CAP_CUM_TOKENS" != "null" && "$CUM_TOKENS_OBSERVED" != "null" ]] &&
  [[ "$CUM_TOKENS_OBSERVED" -gt "$CAP_CUM_TOKENS" ]]; then
  BREACHES+=("cumulativePromptTokens: session total=$CUM_TOKENS_OBSERVED > maxCumulativePromptTokens=$CAP_CUM_TOKENS")
fi

# --- Consumption, over the same present + measurable set -----------------

record_consumption "convergence" "$CONV_OBSERVED" "$CAP_CONV"
if [[ "$TOOL_PRESENT" == "true" ]]; then
  record_consumption "toolCalls" "$TOOL_OBSERVED" "$CAP_TOOLS"
fi
record_consumption "wallClockMinutes" "$MIN_OBSERVED" "$CAP_MINS"
record_consumption "singleToolResultBytes" "$SINGLE_BYTES_OBSERVED" "$CAP_SINGLE_BYTES"
record_consumption "cumulativeToolResultBytes" "$CUM_BYTES_OBSERVED" "$CAP_CUM_BYTES"
record_consumption "promptTokensPerRequest" "$REQ_TOKENS_OBSERVED" "$CAP_REQ_TOKENS"
record_consumption "cumulativePromptTokens" "$CUM_TOKENS_OBSERVED" "$CAP_CUM_TOKENS"

# --- Verdict -------------------------------------------------------------

fmt_cap() { [[ "$1" == "null" ]] && printf 'unset' || printf '%s' "$1"; }
fmt_obs() { [[ "$1" == "null" ]] && printf 'unmeasured' || printf '%s' "$1"; }
fmt_min() { [[ "$MIN_OBSERVED" == "null" ]] && printf 'n/a' || printf '%s' "$MIN_OBSERVED"; }
fmt_tool() { [[ "$TOOL_PRESENT" == "true" ]] && printf '%s' "$TOOL_OBSERVED" || printf 'n/a'; }

if [[ "${#BREACHES[@]}" -gt 0 ]]; then
  {
    echo "G128 session_cap_enforcement_gate violation"
    echo "  session.json:                 $SESSION_FILE"
    echo "  breached dimension(s):"
    for b in "${BREACHES[@]}"; do
      echo "    - $b"
    done
    echo "  aggregate usage:"
    echo "    convergence iterations:     $CONV_OBSERVED (cap $(fmt_cap "$CAP_CONV"))"
    echo "    wall-clock minutes:         $(fmt_min) (cap $(fmt_cap "$CAP_MINS"))"
    echo "    tool calls:                 $(fmt_tool) (cap $(fmt_cap "$CAP_TOOLS"))"
    echo "    largest tool result bytes:  $(fmt_obs "$SINGLE_BYTES_OBSERVED") (cap $(fmt_cap "$CAP_SINGLE_BYTES"))"
    echo "    tool result bytes total:    $(fmt_obs "$CUM_BYTES_OBSERVED") (cap $(fmt_cap "$CAP_CUM_BYTES"))"
    echo "    max prompt tokens/request:  $(fmt_obs "$REQ_TOKENS_OBSERVED") (cap $(fmt_cap "$CAP_REQ_TOKENS"))"
    echo "    prompt tokens total:        $(fmt_obs "$CUM_TOKENS_OBSERVED") (cap $(fmt_cap "$CAP_CUM_TOKENS"))"
    echo "  distinction from G082:        G082 caps iterations PER (specDir, agent); G128 caps the AGGREGATE across the whole session"
    echo "  remediation:                  orchestrator MUST emit a 'blocked' RESULT-ENVELOPE referencing Gate G128 and STOP the session (no further specs/scopes)"
  } >&2
  exit 1
fi

# --- Soft boundary (IMP-048 SCOPE-6) -------------------------------------
#
# Reached only when NOTHING breached. Everything below is written to STDOUT and
# leaves the exit code at 0, because a soft boundary that could fail the guard
# would just be the hard stop moved to 70%.

request_handoff_recommendation() {
  local pct="$1" detail="$2" review="$SCRIPT_DIR/session-review.sh" out rc adapter emitted

  if [[ ! -f "$review" ]]; then
    printf 'unrecorded (session-review.sh not installed)'
    return 0
  fi

  set +e
  out="$(bash "$review" emit \
    --repo-root "$REPO_ROOT" \
    --trigger budget-threshold \
    --budget-pct "$pct" \
    --class-c "handoff-to-fresh-session=$pct" \
    --class-c-reason "handoff-to-fresh-session=session budget at ${pct}% of cap on ${detail}; roll over to a fresh session before the hard stop" 2>&1)"
  rc=$?
  set -e

  # The review refusing, being off, or being absent must never turn a
  # recommendation into a failure. The guard reports what happened and moves on.
  if [[ "$rc" -ne 0 ]]; then
    printf 'unrecorded (session-review exit %s)' "$rc"
    return 0
  fi

  adapter="$(awk -F= '$1 == "adapter" { print $2; exit }' <<< "$out")"
  if [[ "$adapter" == "none" ]]; then
    printf 'unrecorded (sessionReview.adapter: none — default off)'
    return 0
  fi

  emitted="$(awk -F= '$1 == "classCEmitted" { print $2; exit }' <<< "$out")"
  if [[ "${emitted:-0}" -gt 0 ]]; then
    printf 'recorded (Class C handoff-to-fresh-session, emitted)'
  else
    printf 'recorded (Class C handoff-to-fresh-session, deduplicated)'
  fi
}

if [[ "${#CONSUMPTION[@]}" -gt 0 ]]; then
  # awk rather than `sort | head`: the list is bounded and one pass cannot
  # truncate a producer mid-write.
  TOP_CONSUMPTION="$(printf '%s\n' "${CONSUMPTION[@]}" |
    awk -F'|' '$1 + 0 > max { max = $1 + 0; best = $0 } END { print best }')"
  TOP_PCT="${TOP_CONSUMPTION%%|*}"
  TOP_REST="${TOP_CONSUMPTION#*|}"
  TOP_LABEL="${TOP_REST%%|*}"
  TOP_REST="${TOP_REST#*|}"
  TOP_OBSERVED="${TOP_REST%%|*}"
  TOP_CAP="${TOP_REST##*|}"

  if [[ "${TOP_PCT:-0}" -ge "$SOFT_BOUNDARY_PCT" ]]; then
    HANDOFF_STATUS="$(request_handoff_recommendation "$TOP_PCT" "$TOP_LABEL")"
    echo "SOFT-BOUNDARY Gate G128 (session_cap_enforcement_gate) — session budget at ${TOP_PCT}% (threshold ${SOFT_BOUNDARY_PCT}%)"
    echo "  most-consumed dimension:      $TOP_LABEL $TOP_OBSERVED/$TOP_CAP = ${TOP_PCT}%"
    echo "  outcome:                      CONTINUING — the hard stop remains at 100%"
    echo "  owed response:                persist state, emit a continuation envelope, open a bubbles.handoff packet"
    echo "  specStatus:                   UNCHANGED — the work is not blocked, the session is full"
    echo "  handoffRecommendation:        $HANDOFF_STATUS"
    echo "  softBoundary=crossed dimension=$TOP_LABEL consumedPct=$TOP_PCT rollover=recommended specBlocked=false"
  fi
fi

info "aggregate convergence=$CONV_OBSERVED (cap $(fmt_cap "$CAP_CONV")), wall-clock=$(fmt_min)min (cap $(fmt_cap "$CAP_MINS")), toolCalls=$(fmt_tool) (cap $(fmt_cap "$CAP_TOOLS"))"
info "context volume: largestToolResult=$(fmt_obs "$SINGLE_BYTES_OBSERVED")B (cap $(fmt_cap "$CAP_SINGLE_BYTES")), toolResultTotal=$(fmt_obs "$CUM_BYTES_OBSERVED")B (cap $(fmt_cap "$CAP_CUM_BYTES")), maxPromptTokens=$(fmt_obs "$REQ_TOKENS_OBSERVED") (cap $(fmt_cap "$CAP_REQ_TOKENS")), promptTokensTotal=$(fmt_obs "$CUM_TOKENS_OBSERVED") (cap $(fmt_cap "$CAP_CUM_TOKENS"))"
echo "PASS Gate G128 (session_cap_enforcement_gate) — no aggregate cap exceeded (conv=$CONV_OBSERVED/$(fmt_cap "$CAP_CONV"), mins=$(fmt_min)/$(fmt_cap "$CAP_MINS"), tools=$(fmt_tool)/$(fmt_cap "$CAP_TOOLS"), toolBytesMax=$(fmt_obs "$SINGLE_BYTES_OBSERVED")/$(fmt_cap "$CAP_SINGLE_BYTES"), toolBytesSum=$(fmt_obs "$CUM_BYTES_OBSERVED")/$(fmt_cap "$CAP_CUM_BYTES"), promptTokensMax=$(fmt_obs "$REQ_TOKENS_OBSERVED")/$(fmt_cap "$CAP_REQ_TOKENS"), promptTokensSum=$(fmt_obs "$CUM_TOKENS_OBSERVED")/$(fmt_cap "$CAP_CUM_TOKENS"))"
exit 0
