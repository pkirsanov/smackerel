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
#   bash bubbles/scripts/convergence-cap-guard.sh <specDir> [--quiet]
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

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/convergence-cap-guard.sh <specDir> [--quiet]

Required:
  <specDir>   Spec directory whose convergence iterations are inspected
              (e.g. specs/900-convergence-fixture).

Optional:
  --quiet     Suppress informational stdout; the final PASS or VIOLATION
              line is still emitted (stdout on pass, stderr on fail).
  -h, --help  Print this usage and exit.

Exit codes:
  0 = cap not exceeded
  1 = cap exceeded (Gate G082 violation)
  2 = malformed inputs or missing arguments
EOF
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
    --*)
      echo "convergence-cap-guard: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -z "$SPEC_DIR" ]]; then
        SPEC_DIR="$1"
      else
        echo "convergence-cap-guard: unexpected positional argument: $1" >&2
        usage >&2
        exit 2
      fi
      shift
      ;;
  esac
done

if [[ -z "$SPEC_DIR" ]]; then
  echo "convergence-cap-guard: <specDir> is required" >&2
  usage >&2
  exit 2
fi

info() {
  if [[ "$QUIET" != "true" ]]; then
    echo "convergence-cap-guard: $*"
  fi
}

# --- jq dependency check -------------------------------------------------

if ! command -v jq >/dev/null 2>&1; then
  echo "convergence-cap-guard: jq is required but not found in PATH" >&2
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
  echo "convergence-cap-guard: unable to resolve repo root (no .specify/memory found)" >&2
  echo "  Set BUBBLES_REPO_ROOT explicitly or run from inside a Bubbles repo." >&2
  exit 2
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
  echo "convergence-cap-guard: workflows.yaml not found under $REPO_ROOT/bubbles/ or $REPO_ROOT/.github/bubbles/" >&2
  exit 2
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
  echo "convergence-cap-guard: maxConvergenceIterations must be a positive integer, got: $MAX_ITERATIONS" >&2
  exit 2
fi

# --- Locate session.json -------------------------------------------------

SESSION_FILE="$REPO_ROOT/.specify/memory/bubbles.session.json"
if [[ ! -f "$SESSION_FILE" ]]; then
  # No session file recorded yet — no convergence loops to enforce.
  info "no $SESSION_FILE present; nothing to enforce"
  echo "PASS Gate G082 (convergence_cap_enforcement_gate) — cap=$MAX_ITERATIONS, observed=0 (no session.json), specDir=$SPEC_DIR"
  exit 0
fi

# --- Validate session.json is parseable JSON -----------------------------

if ! jq empty "$SESSION_FILE" >/dev/null 2>&1; then
  echo "convergence-cap-guard: $SESSION_FILE is not valid JSON" >&2
  exit 2
fi

# --- Compute max iterationCount for matching specDir entries -------------
#
# We accept any convergenceLoops[] entry whose specDir matches either the
# raw spec dir argument OR a trailing-path equivalent (so callers can pass
# absolute or repo-relative forms interchangeably).

NORMALIZED_SPEC="$SPEC_DIR"
# Strip trailing slash if present.
NORMALIZED_SPEC="${NORMALIZED_SPEC%/}"

MAX_OBSERVED_JSON="$(jq -r --arg specDir "$NORMALIZED_SPEC" '
  (.convergenceLoops // [])
  | map(select(
      (.specDir // "") == $specDir
      or ((.specDir // "") | endswith("/" + ($specDir | sub("^.*/"; ""))))
    ))
  | if length == 0 then
      {observed: 0, agent: "none", lastIterationAt: null}
    else
      (max_by(.iterationCount // 0))
      | {observed: (.iterationCount // 0), agent: (.agent // "unknown"), lastIterationAt: (.lastIterationAt // null)}
    end
' "$SESSION_FILE" 2>/dev/null || true)"

if [[ -z "$MAX_OBSERVED_JSON" ]] || ! echo "$MAX_OBSERVED_JSON" | jq empty >/dev/null 2>&1; then
  echo "convergence-cap-guard: failed to parse convergenceLoops[] from $SESSION_FILE" >&2
  exit 2
fi

OBSERVED="$(echo "$MAX_OBSERVED_JSON" | jq -r '.observed')"
OFFENDING_AGENT="$(echo "$MAX_OBSERVED_JSON" | jq -r '.agent')"
LAST_AT="$(echo "$MAX_OBSERVED_JSON" | jq -r '.lastIterationAt // "unknown"')"

if ! [[ "$OBSERVED" =~ ^[0-9]+$ ]]; then
  echo "convergence-cap-guard: malformed iterationCount in session.json: $OBSERVED" >&2
  exit 2
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
  } >&2
  exit 1
fi

info "specDir=$NORMALIZED_SPEC observed=$OBSERVED maxConvergenceIterations=$MAX_ITERATIONS"
echo "PASS Gate G082 (convergence_cap_enforcement_gate) — cap=$MAX_ITERATIONS, observed=$OBSERVED, specDir=$NORMALIZED_SPEC"
exit 0
