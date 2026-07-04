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
#   1  an active cap exceeded — orchestrator MUST emit a `blocked`
#      RESULT-ENVELOPE with finding G128 and STOP the session; stderr names
#      the breached dimension(s) and observed-vs-cap
#   2  malformed / missing inputs (unparseable session.json, non-integer
#      cap or counter), or bad usage — diagnostic on stderr
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
# Reference: improvements/IMP-003-autonomy-dial-and-safety-caps.md (SCOPE-2)

QUIET="false"

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

# --- Extract the three raw caps ("null" when absent) ---------------------

CAP_CONV="$(jq -r '.sessionBudget.maxTotalConvergenceIterations // "null"' "$SESSION_FILE")"
CAP_MINS="$(jq -r '.sessionBudget.maxWallClockMinutes // "null"' "$SESSION_FILE")"
CAP_TOOLS="$(jq -r '.sessionBudget.maxToolCalls // "null"' "$SESSION_FILE")"

# All three null/absent → no-op (the default for every existing repo).
if [[ "$CAP_CONV" == "null" && "$CAP_MINS" == "null" && "$CAP_TOOLS" == "null" ]]; then
  info "sessionBudget present but all three caps are null; nothing to enforce"
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

# --- Decision: check every PRESENT + MEASURABLE dimension ----------------

declare -a BREACHES=()

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

# --- Verdict -------------------------------------------------------------

fmt_cap() { [[ "$1" == "null" ]] && printf 'unset' || printf '%s' "$1"; }
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
    echo "  distinction from G082:        G082 caps iterations PER (specDir, agent); G128 caps the AGGREGATE across the whole session"
    echo "  remediation:                  orchestrator MUST emit a 'blocked' RESULT-ENVELOPE referencing Gate G128 and STOP the session (no further specs/scopes)"
  } >&2
  exit 1
fi

info "aggregate convergence=$CONV_OBSERVED (cap $(fmt_cap "$CAP_CONV")), wall-clock=$(fmt_min)min (cap $(fmt_cap "$CAP_MINS")), toolCalls=$(fmt_tool) (cap $(fmt_cap "$CAP_TOOLS"))"
echo "PASS Gate G128 (session_cap_enforcement_gate) — no aggregate cap exceeded (conv=$CONV_OBSERVED/$(fmt_cap "$CAP_CONV"), mins=$(fmt_min)/$(fmt_cap "$CAP_MINS"), tools=$(fmt_tool)/$(fmt_cap "$CAP_TOOLS"))"
exit 0
