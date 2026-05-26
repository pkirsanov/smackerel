#!/usr/bin/env bash
set -euo pipefail

# state-snapshot.sh
# Per-turn state snapshot helper for Bubbles orchestrator agents.
#
# Each orchestrator agent calls this script at the start and end of every
# turn (a turn = one operator-visible cycle of work) to write a tiny
# structured record into `.specify/memory/bubbles.session.json` under a
# `turnSnapshots` array. The records make crash-resume deterministic and
# give the operator a per-turn audit trail of agent decisions.
#
# Hard dependency: jq. If jq is missing, this script fails loudly.
# (jq is already used elsewhere in the framework.)
#
# See: agents/bubbles_shared/operating-baseline.md
#      → "Per-Turn State Snapshot"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/state-snapshot.sh \
         --phase <name> [--scope-id <id>] [--note <string>] [--mode <start|end>] \
         [--convergence-iteration <N> --spec-dir <path>]

Required:
  --phase <name>       Phase the orchestrator is entering or closing
                       (e.g. phase_2_plan, phase_3_execute).

Optional:
  --scope-id <id>      Scope being worked, when applicable.
  --note <string>      Free-form note attached to this snapshot.
  --mode <start|end>   Records turn-start (default) or turn-end.
  --convergence-iteration <N>
                       Integer ≥ 0. When supplied alongside --spec-dir,
                       additively writes/updates the (specDir, agent)
                       entry in `convergenceLoops[]`. Enforced by Gate G082
                       via `bubbles/scripts/convergence-cap-guard.sh`. Both
                       --convergence-iteration and --spec-dir MUST be
                       supplied together; supplying only one is an error.
  --spec-dir <path>    Spec directory (repo-relative) that the
                       convergence iteration refers to. Paired with
                       --convergence-iteration.
  -h, --help           Print this usage and exit.

Behavior:
  - Appends a single record to `.specify/memory/bubbles.session.json` under
    the `turnSnapshots[]` array. Each record carries:
        turnNumber  (auto-incremented integer; 1 for first record)
        timestamp   (UTC ISO8601, wall clock)
        phase       (the --phase value)
        scopeId     (the --scope-id value or null)
        mode        ("start" | "end")
        note        (the --note value or null)
        agent       ($BUBBLES_AGENT_NAME if set, otherwise "unknown")
  - Prior records are NEVER touched. The array grows monotonically.
  - Two consecutive `--mode start` calls for the same phase + scope are
    intentionally allowed to support resume-after-crash flows.
  - Repo root is detected via $BUBBLES_REPO_ROOT (preferred) or by
    walking up from $PWD looking for `.specify/memory/`.

Hard dependency:
  - `jq` is required. If `jq` is missing the script exits non-zero
    with a clear error message — no silent fallback.

Reference:
  agents/bubbles_shared/operating-baseline.md
    -> "Per-Turn State Snapshot"
EOF
}

# --- Arg parsing -----------------------------------------------------------

PHASE=""
SCOPE_ID=""
NOTE=""
MODE="start"
CONV_ITER=""
SPEC_DIR=""

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
    --phase)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --phase requires a value" >&2; exit 2; }
      PHASE="$2"
      shift 2
      ;;
    --scope-id)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --scope-id requires a value" >&2; exit 2; }
      SCOPE_ID="$2"
      shift 2
      ;;
    --note)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --note requires a value" >&2; exit 2; }
      NOTE="$2"
      shift 2
      ;;
    --mode)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --mode requires a value" >&2; exit 2; }
      MODE="$2"
      shift 2
      ;;
    --convergence-iteration)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --convergence-iteration requires a value" >&2; exit 2; }
      CONV_ITER="$2"
      shift 2
      ;;
    --spec-dir)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --spec-dir requires a value" >&2; exit 2; }
      SPEC_DIR="$2"
      shift 2
      ;;
    *)
      echo "state-snapshot: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

# Pair check: --convergence-iteration and --spec-dir must be supplied together.
if [[ -n "$CONV_ITER" && -z "$SPEC_DIR" ]]; then
  echo "state-snapshot: --convergence-iteration requires --spec-dir" >&2
  exit 2
fi
if [[ -n "$SPEC_DIR" && -z "$CONV_ITER" ]]; then
  echo "state-snapshot: --spec-dir requires --convergence-iteration" >&2
  exit 2
fi

# Validate --convergence-iteration is a non-negative integer.
if [[ -n "$CONV_ITER" ]]; then
  if ! [[ "$CONV_ITER" =~ ^[0-9]+$ ]]; then
    echo "state-snapshot: --convergence-iteration must be a non-negative integer (got: $CONV_ITER)" >&2
    exit 2
  fi
fi

if [[ -z "$PHASE" ]]; then
  echo "state-snapshot: --phase is required" >&2
  usage >&2
  exit 2
fi

case "$MODE" in
  start|end) ;;
  *)
    echo "state-snapshot: --mode must be 'start' or 'end' (got: $MODE)" >&2
    exit 2
    ;;
esac

# --- jq dependency check ---------------------------------------------------

if ! command -v jq >/dev/null 2>&1; then
  echo "state-snapshot: jq is required but not found in PATH." >&2
  echo "  Install jq before invoking state-snapshot.sh." >&2
  exit 3
fi

# --- Repo root resolution --------------------------------------------------

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
  echo "state-snapshot: unable to resolve repo root (no .specify/memory found)." >&2
  echo "  Set BUBBLES_REPO_ROOT explicitly or run from inside a Bubbles repo." >&2
  exit 4
fi

SESSION_DIR="$REPO_ROOT/.specify/memory"
SESSION_FILE="$SESSION_DIR/bubbles.session.json"

mkdir -p "$SESSION_DIR"

if [[ ! -f "$SESSION_FILE" ]]; then
  printf '{}\n' > "$SESSION_FILE"
fi

# --- Build snapshot record -------------------------------------------------

AGENT_NAME="${BUBBLES_AGENT_NAME:-unknown}"
TIMESTAMP="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

# Compute next turnNumber from existing turnSnapshots array length.
NEXT_TURN="$(jq '
  (.turnSnapshots // []) | length + 1
' "$SESSION_FILE")"

# Append a new record. We use --argjson for ints, --arg for strings, and
# pass scope_id / note as strings that may be empty (mapped to null below).
TMP_FILE="$(mktemp)"
trap 'rm -f "$TMP_FILE"' EXIT INT TERM

jq \
  --argjson turn "$NEXT_TURN" \
  --arg timestamp "$TIMESTAMP" \
  --arg phase "$PHASE" \
  --arg scope_id "$SCOPE_ID" \
  --arg note "$NOTE" \
  --arg mode "$MODE" \
  --arg agent "$AGENT_NAME" \
  '
  . as $root
  | ($root + {
      turnSnapshots: ((($root.turnSnapshots // []) + [
        {
          turnNumber: $turn,
          timestamp: $timestamp,
          phase: $phase,
          scopeId: (if $scope_id == "" then null else $scope_id end),
          mode: $mode,
          note: (if $note == "" then null else $note end),
          agent: $agent
        }
      ]))
    })
  ' "$SESSION_FILE" > "$TMP_FILE"

mv "$TMP_FILE" "$SESSION_FILE"

# --- Convergence loop update (Gate G082) -----------------------------------
#
# When both --convergence-iteration and --spec-dir are supplied, additively
# update the `convergenceLoops[]` array entry keyed by (specDir, agent).
# If an entry for that key already exists, replace its `iterationCount` and
# `lastUpdated`. Otherwise append a new entry. Other entries (for other
# specs or other agents) are NEVER touched.
#
# This array is consumed by `bubbles/scripts/convergence-cap-guard.sh`
# which enforces `maxConvergenceIterations` (default 10) per Gate G082.
if [[ -n "$CONV_ITER" && -n "$SPEC_DIR" ]]; then
  CONV_TMP="$(mktemp)"
  trap 'rm -f "$CONV_TMP"' EXIT INT TERM
  jq \
    --arg specDir "$SPEC_DIR" \
    --arg agent "$AGENT_NAME" \
    --argjson iterationCount "$CONV_ITER" \
    --arg lastUpdated "$TIMESTAMP" \
    '
    . as $root
    | ($root.convergenceLoops // []) as $loops
    | ([ $loops[]
         | select(.specDir != $specDir or .agent != $agent)
       ] + [{
         specDir: $specDir,
         agent: $agent,
         iterationCount: $iterationCount,
         lastUpdated: $lastUpdated
       }]) as $updated
    | $root + { convergenceLoops: $updated }
    ' "$SESSION_FILE" > "$CONV_TMP"
  mv "$CONV_TMP" "$SESSION_FILE"
fi
trap - EXIT INT TERM

# Echo a one-line summary to stdout for orchestrator log capture.
if [[ -n "$CONV_ITER" && -n "$SPEC_DIR" ]]; then
  printf 'state-snapshot: turnNumber=%s mode=%s phase=%s scopeId=%s agent=%s convergenceIteration=%s specDir=%s\n' \
    "$NEXT_TURN" "$MODE" "$PHASE" "${SCOPE_ID:-null}" "$AGENT_NAME" "$CONV_ITER" "$SPEC_DIR"
else
  printf 'state-snapshot: turnNumber=%s mode=%s phase=%s scopeId=%s agent=%s\n' \
    "$NEXT_TURN" "$MODE" "$PHASE" "${SCOPE_ID:-null}" "$AGENT_NAME"
fi
