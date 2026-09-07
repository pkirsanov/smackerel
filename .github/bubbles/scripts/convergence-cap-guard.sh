#!/usr/bin/env bash
set -euo pipefail

# convergence-cap-guard.sh
#
# Gate G082 — convergence_cap_enforcement_gate.
#
# Mechanically enforces the orchestrator convergence iteration cap
# (`maxConvergenceIterations`, default 10) declared in
# `bubbles/workflows.yaml`. Reads `.specify/memory/bubbles.session.json`
# and inspects the `convergenceLoops[]` array (additively appended by
# `bubbles/scripts/state-snapshot.sh --convergence-iteration <N>`),
# filters entries whose `specDir` matches the spec directory passed on
# the command line, and computes the maximum observed `iterationCount`
# for that spec.
#
# Exit codes:
#   0  cap not exceeded (or no convergence loops recorded for this spec)
#   1  cap exceeded — orchestrator MUST treat this spec as `blocked`
#       with finding G082; stderr names the cap and the offending agent
#   2  malformed / missing inputs (workflows.yaml, session.json), or
#       missing required arguments — diagnostic on stderr
#
# Usage:
#   bash bubbles/scripts/convergence-cap-guard.sh <specDir> --session-id <id> [--quiet]
#
# Inputs:
#   <specDir>   Path to the spec directory (e.g.
#               specs/900-convergence-fixture). Used to filter
#               convergenceLoops[] entries.
#   --quiet     Suppress informational stdout on success (PASS line is
#               always written to stdout; informational lines suppressed).
#
# Dependencies:
#   - jq      (hard dependency)
#   - awk     (POSIX; used as a tiny YAML reader for one scalar)
#
# Schema (additive in bubbles.session.json):
#   {
#     "convergenceLoops": [
#       {
#         "specDir":        "<path>",
#         "agent":          "<bubbles.workflow|bubbles.goal|...>",
#         "iterationCount": <int>,
#         "lastIterationAt":"<RFC3339>",
#         "cappedAt":       "<RFC3339|null>"
#       },
#       ...
#     ]
#   }
#
# Reference: docs/Framework_Convergence_Health.md

QUIET="false"
SPEC_DIR=""
SESSION_ID=""
SESSION_ID_SEEN="false"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATE_IO_HELPER="$SCRIPT_DIR/session-state-io.py"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/convergence-cap-guard.sh <specDir> --session-id <id> [--quiet]

Required:
  <specDir>   Spec directory whose convergence iterations are inspected
              (e.g. specs/900-convergence-fixture).
  --session-id <id>
              Exact host-issued session identity to evaluate.

Optional:
  --quiet     Suppress informational stdout; the final PASS or VIOLATION
              line is still emitted (stdout on pass, stderr on fail).
  -h, --help  Print this usage and exit.

Exit codes:
  0 = PASS
  1 = BREACH
  2 = INPUT-ERROR
EOF
}

json_string() {
  local value="$1"
  local python_bin
  python_bin="$(command -v python3 2>/dev/null || true)"
  [[ -n "$python_bin" && -f "$STATE_IO_HELPER" && ! -L "$STATE_IO_HELPER" ]] || return 1
  "$python_bin" "$STATE_IO_HELPER" json-string -- "$value" 2>/dev/null
}

input_error() {
  local reason="$1"
  local value_name="${2:-}"
  local value="${3:-}"
  local encoded=""
  local session_display='""'
  local spec_display='""'

  session_display="$(json_string "$SESSION_ID" || printf '%s' '""')"
  spec_display="$(json_string "$SPEC_DIR" || printf '%s' '""')"
  if [[ -n "$value_name" ]]; then
    encoded="$(json_string "$value" || true)"
  fi
  if [[ -n "$value_name" && -n "$encoded" ]]; then
    printf 'convergence-cap-guard: input-error reason=%s %s=%s\n' \
      "$reason" "$value_name" "$encoded" >&2
  else
    printf 'convergence-cap-guard: input-error reason=%s\n' "$reason" >&2
  fi
  printf 'G082 status=INPUT-ERROR exit=2 session=%s spec=%s reason=%s\n' \
    "$session_display" "$spec_display" "$reason" >&2
  exit 2
}

# --- Argument parsing ----------------------------------------------------

if [[ $# -eq 0 ]]; then
  usage >&2
  exit 2
fi

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
    --session-id)
      [[ $# -ge 2 ]] || input_error "missing-session-id"
      if [[ "$SESSION_ID_SEEN" == "true" ]]; then
        if [[ "$SESSION_ID" == "$2" ]]; then
          input_error "duplicate-session-id"
        fi
        input_error "conflicting-session-id"
      fi
      SESSION_ID="$2"
      SESSION_ID_SEEN="true"
      shift 2
      ;;
    --*)
      input_error "unknown-flag" "argument" "$1"
      ;;
    *)
      if [[ -z "$SPEC_DIR" ]]; then
        SPEC_DIR="$1"
      else
        input_error "unexpected-positional-argument" "argument" "$1"
      fi
      shift
      ;;
  esac
done

if [[ -z "$SPEC_DIR" ]]; then
  input_error "missing-spec-dir"
fi

if [[ "$SESSION_ID_SEEN" != "true" || -z "$SESSION_ID" ]]; then
  input_error "missing-session-id"
fi

info() {
  if [[ "$QUIET" != "true" ]]; then
    echo "convergence-cap-guard: $*"
  fi
}

# --- jq dependency check -------------------------------------------------

if ! command -v jq >/dev/null 2>&1; then
  input_error "jq-unavailable"
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
  input_error "repository-root-unavailable"
fi

# --- Locate workflows.yaml (source-repo OR installed layout) -------------

WORKFLOWS_YAML=""
for candidate in \
  "$REPO_ROOT/bubbles/workflows.yaml" \
  "$REPO_ROOT/.github/bubbles/workflows.yaml"; do
  if [[ -f "$candidate" ]]; then
    WORKFLOWS_YAML="$candidate"
    break
  fi
done

if [[ -z "$WORKFLOWS_YAML" ]]; then
  input_error "workflows-unavailable"
fi

# --- Extract maxConvergenceIterations (first occurrence) -----------------
#
# workflows.yaml currently declares maxConvergenceIterations only in the
# autonomous-goal workflow's constraints block. Future workflows MAY add
# their own. For Gate G082 we treat the first declared value as the
# framework-level cap; if no declaration is present we default to 10
# (the documented Convergence Loop ceiling).

read_max_iterations() {
  local yaml_file="$1"
  awk '
    /^[[:space:]]*maxConvergenceIterations[[:space:]]*:[[:space:]]*[0-9]+/ {
      # Extract the integer after the colon.
      n = $0
      sub(/^.*maxConvergenceIterations[[:space:]]*:[[:space:]]*/, "", n)
      sub(/[^0-9].*$/, "", n)
      if (n ~ /^[0-9]+$/) {
        print n
        exit
      }
    }
  ' "$yaml_file"
}

MAX_ITERATIONS="$(read_max_iterations "$WORKFLOWS_YAML" || true)"
if [[ -z "$MAX_ITERATIONS" ]]; then
  MAX_ITERATIONS=10
  info "maxConvergenceIterations not declared in $WORKFLOWS_YAML; using framework default cap=10"
fi

if ! [[ "$MAX_ITERATIONS" =~ ^[0-9]+$ ]] || [[ "$MAX_ITERATIONS" -lt 1 ]]; then
  input_error "invalid-max-convergence-iterations" "observed" "$MAX_ITERATIONS"
fi

# --- Capture one immutable session-state revision ------------------------

PYTHON_BIN="$(command -v python3 2>/dev/null || true)"
[[ -n "$PYTHON_BIN" ]] || input_error "python-unavailable"
[[ -f "$STATE_IO_HELPER" && ! -L "$STATE_IO_HELPER" ]] || input_error "state-io-helper-unavailable"

CAPTURE_DIR="$(mktemp -d -t bubbles-g082-capture-XXXXXXXX 2>/dev/null || true)"
[[ -n "$CAPTURE_DIR" && -d "$CAPTURE_DIR" ]] || input_error "capture-directory-failed"
SESSION_FILE="$CAPTURE_DIR/bubbles.session.json"
cleanup_capture() {
  rm -rf "$CAPTURE_DIR"
}
trap cleanup_capture EXIT HUP INT TERM

NORMALIZED_SPEC="${SPEC_DIR%/}"
SESSION_DISPLAY="$(json_string "$SESSION_ID" || true)"
SPEC_DISPLAY="$(json_string "$NORMALIZED_SPEC" || true)"
[[ -n "$SESSION_DISPLAY" && -n "$SPEC_DISPLAY" ]] || input_error "json-encoder-failed"

set +e
CAPTURE_OUTPUT="$("$PYTHON_BIN" "$STATE_IO_HELPER" capture \
  --root "$REPO_ROOT" \
  --relative-path '.specify/memory/bubbles.session.json' \
  --destination "$SESSION_FILE" 2>/dev/null)"
CAPTURE_RC=$?
set -e
if [[ "$CAPTURE_RC" -eq 4 ]]; then
  info "no session state is present; nothing to enforce"
  printf 'PASS Gate G082 (convergence_cap_enforcement_gate) — cap=%s, observed=0 (no session.json), specDir=%s\n' \
    "$MAX_ITERATIONS" "$NORMALIZED_SPEC"
  printf 'G082 status=PASS exit=0 session=%s spec=%s\n' "$SESSION_DISPLAY" "$SPEC_DISPLAY"
  exit 0
fi
[[ "$CAPTURE_RC" -eq 0 ]] || input_error "unsafe-session-state"

STATE_REVISION="$(printf '%s' "$CAPTURE_OUTPUT" | jq -er '
  select(type == "object" and .status == "captured")
  | .revision
  | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))
' 2>/dev/null || true)"
[[ -n "$STATE_REVISION" ]] || input_error "invalid-capture-result"

# --- Validate session.json is parseable JSON -----------------------------

if ! jq -e 'type == "object"' "$SESSION_FILE" >/dev/null 2>&1; then
  printf 'convergence-cap-guard: session state is not valid JSON\n' >&2
  input_error "invalid-session-json"
fi

# --- Compute max iterationCount for matching specDir entries -------------

MAX_OBSERVED_JSON="$(jq -c --arg specDir "$NORMALIZED_SPEC" --arg sessionId "$SESSION_ID" '
  if ((.convergenceLoops // []) | type) != "array" then
    {error: "invalid-convergence-history"}
  else
    [(.convergenceLoops // [])[]
      | select(type == "object" and .hostSessionId == $sessionId and .specDir == $specDir)] as $matching
    | if any($matching[];
        ((.iterationCount | type) != "number")
        or (.iterationCount < 0)
        or ((.iterationCount | floor) != .iterationCount)
        or ((.agent | type) != "string")
        or (.agent == "")) then
        {error: "malformed-matching-convergence-row"}
      elif ($matching | length) == 0 then
      {observed: 0, agent: "none", lastIterationAt: null}
    else
        ($matching | max_by(.iterationCount))
        | {observed: .iterationCount, agent: .agent, lastIterationAt: (.lastIterationAt // null)}
      end
    end
' "$SESSION_FILE" 2>/dev/null || true)"

if [[ -z "$MAX_OBSERVED_JSON" ]] || ! echo "$MAX_OBSERVED_JSON" | jq empty >/dev/null 2>&1; then
  input_error "invalid-convergence-history"
fi

EVALUATION_ERROR="$(printf '%s' "$MAX_OBSERVED_JSON" | jq -r '.error // empty')"
[[ -z "$EVALUATION_ERROR" ]] || input_error "$EVALUATION_ERROR"

OBSERVED="$(echo "$MAX_OBSERVED_JSON" | jq -r '.observed')"
OFFENDING_AGENT="$(echo "$MAX_OBSERVED_JSON" | jq -r '.agent')"
LAST_AT="$(echo "$MAX_OBSERVED_JSON" | jq -r '.lastIterationAt // "unknown"')"

if ! [[ "$OBSERVED" =~ ^[0-9]+$ ]]; then
  input_error "malformed-matching-convergence-row" "observed" "$OBSERVED"
fi

# --- Decision -----------------------------------------------------------

if [[ "$OBSERVED" -gt "$MAX_ITERATIONS" ]]; then
  {
    echo "G082 convergence_cap_enforcement_gate violation"
    echo "  specDir:                  $NORMALIZED_SPEC"
    echo "  agent:                    $OFFENDING_AGENT"
    echo "  observed iterationCount:  $OBSERVED"
    echo "  maxConvergenceIterations: $MAX_ITERATIONS"
    echo "  lastIterationAt:          $LAST_AT"
    echo "  workflows.yaml:           $WORKFLOWS_YAML"
    echo "  session.json:             $SESSION_FILE"
    echo "  remediation:              orchestrator MUST emit a 'blocked' RESULT-ENVELOPE referencing Gate G082 and STOP further convergence iterations for this spec"
    printf 'G082 status=BREACH exit=1 session=%s spec=%s\n' "$SESSION_DISPLAY" "$SPEC_DISPLAY"
  } >&2
  exit 1
fi

info "revision=$STATE_REVISION immutable=true"
info "specDir=$NORMALIZED_SPEC observed=$OBSERVED maxConvergenceIterations=$MAX_ITERATIONS"
echo "PASS Gate G082 (convergence_cap_enforcement_gate) — cap=$MAX_ITERATIONS, observed=$OBSERVED, specDir=$NORMALIZED_SPEC"
printf 'G082 status=PASS exit=0 session=%s spec=%s\n' "$SESSION_DISPLAY" "$SPEC_DISPLAY"
exit 0
