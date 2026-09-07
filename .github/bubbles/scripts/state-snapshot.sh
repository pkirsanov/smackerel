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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPOSITORY_BINDING="$SCRIPT_DIR/repository-binding.sh"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/state-snapshot.sh \
         --phase <name> [--scope-id <id>] [--note <string>] [--mode <start|end>] \
         [--posture <autonomy>] \
         [--context-boundary <kind>[:<checkpointId>]] \
         [--mbe-posture <off|shadow|advisory|reference-enforce> \
          --mbe-store-root <path> --mbe-epoch-context <path>] \
         [--decision <text> [--decision-principle <name>] [--decision-chose <option>] \
          [--decision-considered <csv>]] \
         [--convergence-iteration <N> --spec-dir <path>] \
         [--session-budget-json <object> \
          --expected-session-budget-revision <N>] \
         [--scenario-file <compiled-scenario.json> --node-id <node-id>] \
         --session-id <id> --session-control-file <path> --binding-packet-file <path>

Required:
  --phase <name>       Phase the orchestrator is entering or closing
                       (e.g. phase_2_plan, phase_3_execute).

Required repository binding:
  --session-id <id>    Current interactive session id.
  --session-control-file <path>
                       Host-private authoritative session control record.
  --binding-packet-file <path>
                       Current local actionable repository binding packet.

Optional goal-node binding:
  --scenario-file <path>
                       Compiled scenario that declares the goal node.
  --node-id <id>       Goal-node ID declared by --scenario-file. Both
                       --scenario-file and --node-id MUST be supplied together.

Optional:
  --scope-id <id>      Scope being worked, when applicable.
  --occurrence-id <id> IMP-047 S-C. Occurrence identity for THIS phase run,
                       e.g. `validate#2`, as assigned by
                       bubbles/scripts/phase-coordinator.sh. A mode that runs
                       one phase twice produces two snapshots whose `phase` is
                       identical, so resume keyed on the name alone cannot tell
                       them apart. The legacy `phase` field is UNCHANGED and
                       still written; this mirrors it with the id that is
                       actually distinct. Omitted for a single-occurrence phase.
  --note <string>      Free-form note attached to this snapshot.
  --mode <start|end>   Records turn-start (default) or turn-end.
  --convergence-iteration <N>
                       Integer ≥ 0. When supplied alongside --spec-dir,
                       additively writes/updates the
                       (hostSessionId, specDir, agent)
                       entry in `convergenceLoops[]`. Enforced by Gate G082
                       via `bubbles/scripts/convergence-cap-guard.sh`. Both
                       --convergence-iteration and --spec-dir MUST be
                       supplied together; supplying only one is an error.
  --spec-dir <path>    Spec directory (repo-relative) that the
                       convergence iteration refers to. Paired with
                       --convergence-iteration.
  --session-budget-json <object>
                       Exact seven-cap session policy to append. Paired with
                       --expected-session-budget-revision. The first write
                       expects 0. A correction expects the unique current head.
  --expected-session-budget-revision <N>
                       Non-negative compare-and-append revision. Paired with
                       --session-budget-json.
  -h, --help           Print this usage and exit.

Behavior:
  - Appends a single record to `.specify/memory/bubbles.session.json` under
    the `turnSnapshots[]` array. Each record carries:
        turnNumber  (auto-incremented integer; 1 for first record)
        timestamp   (UTC ISO8601, wall clock)
        phase       (the --phase value)
        occurrenceId(the --occurrence-id value or null; mirrors `phase` with an
                     identity that is distinct across repeated occurrences)
        scopeId     (the --scope-id value or null)
        mode        ("start" | "end")
        note        (the --note value or null)
        agent       ($BUBBLES_AGENT_NAME if set, otherwise "unknown")
        hostSessionId
                    (the --session-id value; attributes the record to ONE host
                     session so two concurrent sessions in one repository read
                     back their own trajectory instead of each other's —
                     bubbles/scripts/session-liveness.sh consumes it)
  - Prior records are NEVER touched. The array grows monotonically.
  - Two consecutive `--mode start` calls for the same phase + scope are
    intentionally allowed to support resume-after-crash flows.
  - The repository root comes only from the validated actionable packet.
    PWD and BUBBLES_REPO_ROOT are never repository authority.

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
OCCURRENCE_ID=""
NOTE=""
MODE="start"
POSTURE=""
CONTEXT_BOUNDARY_KIND=""
CONTEXT_BOUNDARY_ID=""
DECISION=""
DECISION_PRINCIPLE=""
DECISION_CHOSE=""
DECISION_CONSIDERED=""
CONV_ITER=""
SPEC_DIR=""
SESSION_ID=""
SESSION_CONTROL_FILE=""
BINDING_PACKET_FILE=""
SCENARIO_FILE=""
NODE_ID=""
MBE_POSTURE="off"
MBE_STORE_ROOT=""
MBE_EPOCH_CONTEXT=""
MBE_EPOCH_JSON=""
SESSION_BUDGET_JSON=""
EXPECTED_SESSION_BUDGET_REVISION=""
SESSION_BUDGET_FLAG_SEEN=0
EXPECTED_SESSION_BUDGET_REVISION_FLAG_SEEN=0

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
    --occurrence-id)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --occurrence-id requires a value" >&2; exit 2; }
      OCCURRENCE_ID="$2"
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
    --posture)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --posture requires a value" >&2; exit 2; }
      POSTURE="$2"
      shift 2
      ;;
    --context-boundary)
      # <kind>[:<checkpointId>]. Gate G083 validates the recorded value; this
      # only splits it. Declaring `unavailable` is always legal and is the
      # honest answer when the host exposes no compaction primitive.
      [[ $# -ge 2 ]] || { echo "state-snapshot: --context-boundary requires a value" >&2; exit 2; }
      CONTEXT_BOUNDARY_KIND="${2%%:*}"
      if [[ "$2" == *:* ]]; then
        CONTEXT_BOUNDARY_ID="${2#*:}"
      fi
      case "$CONTEXT_BOUNDARY_KIND" in
        host-checkpoint | fresh-context | unavailable) ;;
        *)
          echo "state-snapshot: --context-boundary kind must be host-checkpoint, fresh-context or unavailable (got: '$CONTEXT_BOUNDARY_KIND')" >&2
          exit 2
          ;;
      esac
      if [[ "$CONTEXT_BOUNDARY_KIND" == "host-checkpoint" && -z "$CONTEXT_BOUNDARY_ID" ]]; then
        echo "state-snapshot: --context-boundary host-checkpoint requires a checkpoint id (host-checkpoint:<id>)" >&2
        exit 2
      fi
      shift 2
      ;;
    --mbe-posture)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --mbe-posture requires a value" >&2; exit 2; }
      MBE_POSTURE="$2"
      shift 2
      ;;
    --mbe-store-root)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --mbe-store-root requires a value" >&2; exit 2; }
      MBE_STORE_ROOT="$2"
      shift 2
      ;;
    --mbe-epoch-context)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --mbe-epoch-context requires a value" >&2; exit 2; }
      MBE_EPOCH_CONTEXT="$2"
      shift 2
      ;;
    --decision)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --decision requires a value" >&2; exit 2; }
      DECISION="$2"
      shift 2
      ;;
    --decision-principle)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --decision-principle requires a value" >&2; exit 2; }
      DECISION_PRINCIPLE="$2"
      shift 2
      ;;
    --decision-chose)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --decision-chose requires a value" >&2; exit 2; }
      DECISION_CHOSE="$2"
      shift 2
      ;;
    --decision-considered)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --decision-considered requires a value" >&2; exit 2; }
      DECISION_CONSIDERED="$2"
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
    --session-budget-json)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --session-budget-json requires a value" >&2; exit 2; }
      [[ "$SESSION_BUDGET_FLAG_SEEN" -eq 0 ]] || {
        echo "state-snapshot: duplicate --session-budget-json is ambiguous" >&2
        exit 2
      }
      SESSION_BUDGET_FLAG_SEEN=1
      SESSION_BUDGET_JSON="$2"
      shift 2
      ;;
    --expected-session-budget-revision)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --expected-session-budget-revision requires a value" >&2; exit 2; }
      [[ "$EXPECTED_SESSION_BUDGET_REVISION_FLAG_SEEN" -eq 0 ]] || {
        echo "state-snapshot: duplicate --expected-session-budget-revision is ambiguous" >&2
        exit 2
      }
      EXPECTED_SESSION_BUDGET_REVISION_FLAG_SEEN=1
      EXPECTED_SESSION_BUDGET_REVISION="$2"
      shift 2
      ;;
    --session-id)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --session-id requires a value" >&2; exit 2; }
      SESSION_ID="$2"
      shift 2
      ;;
    --session-control-file)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --session-control-file requires a value" >&2; exit 2; }
      SESSION_CONTROL_FILE="$2"
      shift 2
      ;;
    --binding-packet-file)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --binding-packet-file requires a value" >&2; exit 2; }
      BINDING_PACKET_FILE="$2"
      shift 2
      ;;
    --scenario-file)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --scenario-file requires a value" >&2; exit 2; }
      SCENARIO_FILE="$2"
      shift 2
      ;;
    --node-id)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --node-id requires a value" >&2; exit 2; }
      NODE_ID="$2"
      shift 2
      ;;
    *)
      echo "state-snapshot: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -n "$SCENARIO_FILE" && -z "$NODE_ID" ]]; then
  echo "state-snapshot: --scenario-file requires --node-id" >&2
  exit 2
fi
if [[ -n "$NODE_ID" && -z "$SCENARIO_FILE" ]]; then
  echo "state-snapshot: --node-id requires --scenario-file" >&2
  exit 2
fi

# Pair check: --convergence-iteration and --spec-dir must be supplied together.
if [[ -n "$CONV_ITER" && -z "$SPEC_DIR" ]]; then
  echo "state-snapshot: --convergence-iteration requires --spec-dir" >&2
  exit 2
fi
if [[ -n "$SPEC_DIR" && -z "$CONV_ITER" ]]; then
  echo "state-snapshot: --spec-dir requires --convergence-iteration" >&2
  exit 2
fi

if [[ -n "$SESSION_BUDGET_JSON" && -z "$EXPECTED_SESSION_BUDGET_REVISION" ]]; then
  echo "state-snapshot: --session-budget-json requires --expected-session-budget-revision" >&2
  exit 2
fi
if [[ -n "$EXPECTED_SESSION_BUDGET_REVISION" && -z "$SESSION_BUDGET_JSON" ]]; then
  echo "state-snapshot: --expected-session-budget-revision requires --session-budget-json" >&2
  exit 2
fi
if [[ -n "$EXPECTED_SESSION_BUDGET_REVISION" ]] &&
  ! [[ "$EXPECTED_SESSION_BUDGET_REVISION" =~ ^[0-9]+$ ]]; then
  echo "state-snapshot: --expected-session-budget-revision must be a non-negative integer" >&2
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

case "$MBE_POSTURE" in
  off | shadow | advisory | reference-enforce) ;;
  *) echo "state-snapshot: --mbe-posture must be off, shadow, advisory, or reference-enforce (got: $MBE_POSTURE)" >&2; exit 2 ;;
esac
if [[ "$MBE_POSTURE" != "off" && -n "$MBE_STORE_ROOT" && -n "$MBE_EPOCH_CONTEXT" ]]; then
  MBE_EPOCH_JSON="$(python3 "$SCRIPT_DIR/mbe-reference-verify.py" \
    --store-root "$MBE_STORE_ROOT" --context "$MBE_EPOCH_CONTEXT" --purpose epoch 2>/dev/null || true)"
fi
if [[ "$MBE_POSTURE" == "reference-enforce" ]]; then
  [[ -n "$CONTEXT_BOUNDARY_KIND" ]] || { echo "state-snapshot: reference-enforce requires --context-boundary" >&2; exit 3; }
  [[ -n "$MBE_STORE_ROOT" && -d "$MBE_STORE_ROOT" ]] || { echo "state-snapshot: reference-enforce requires an existing --mbe-store-root" >&2; exit 3; }
  [[ -n "$MBE_EPOCH_CONTEXT" && -f "$MBE_EPOCH_CONTEXT" ]] || { echo "state-snapshot: reference-enforce requires an existing --mbe-epoch-context" >&2; exit 3; }
  [[ -n "$MBE_EPOCH_JSON" ]] || { echo "state-snapshot: reference-enforce requires a verified MBE epoch" >&2; exit 3; }
fi

# Record the posture that produced this turn, so an audit never has to
# reconstruct the operator's shell environment. A resolver failure (e.g. an
# unbounded `unattended`) must not fail the snapshot: leave it unset instead.
if [[ -z "$POSTURE" && -x "$SCRIPT_DIR/autonomy-resolve.sh" ]]; then
  POSTURE="$(bash "$SCRIPT_DIR/autonomy-resolve.sh" --format json 2>/dev/null |
    sed -n 's/.*"autonomy":"\([^"]*\)".*/\1/p')"
fi

# Decision metadata without a decision would record a principle that fired on
# nothing, which is worse than no ledger entry at all.
if [[ -z "$DECISION" ]] &&
  [[ -n "$DECISION_PRINCIPLE$DECISION_CHOSE$DECISION_CONSIDERED" ]]; then
  echo "state-snapshot: --decision-principle/--decision-chose/--decision-considered require --decision" >&2
  exit 2
fi

[[ -n "$SESSION_ID" ]] || { echo "state-snapshot: --session-id is required for repository-local snapshots" >&2; exit 2; }
[[ -n "$SESSION_CONTROL_FILE" ]] || { echo "state-snapshot: --session-control-file is required for repository-local snapshots" >&2; exit 2; }
[[ -n "$BINDING_PACKET_FILE" ]] || { echo "state-snapshot: --binding-packet-file is required for repository-local snapshots" >&2; exit 2; }

# --- jq dependency check ---------------------------------------------------

if ! command -v jq >/dev/null 2>&1; then
  echo "state-snapshot: jq is required but not found in PATH." >&2
  echo "  Install jq before invoking state-snapshot.sh." >&2
  exit 3
fi

# --- Validated repository root ---------------------------------------------

[[ -f "$REPOSITORY_BINDING" ]] || { echo "state-snapshot: repository binding validator missing at $REPOSITORY_BINDING" >&2; exit 3; }
NORMALIZED_PACKET_FILE=""
TMP_FILE=""
CONV_TMP=""
POLICY_TMP=""
PRE_TRANSACTION_STATE=""

# --- Exclusive descriptor-safe transaction --------------------------------
#
# The outer process validates packet authority before deriving any repository
# path. It then delegates one complete state transaction to session-state-io.py.
# The helper creates missing parent directories without following symlinks.
# Its flock strategy opens without truncation and rechecks descriptor identity.
# Its mkdir strategy rechecks both directory and holder identity before cleanup.
# The child holds that lock across mirror, policy, turn, and convergence writes.
SESSION_LOCK_STRATEGY=""
LOCK_TRANSACTION_MODE="${BUBBLES_STATE_SNAPSHOT_LOCK_TRANSACTION:-}"

# Detect flock once before dispatching the helper-owned transaction.
session_lock_have_flock() {
  command -v flock >/dev/null 2>&1
}

_lock_trace() { [[ -z "${BUBBLES_LOCK_TRACE:-}" ]] || printf '%s %s %s %s\n' "$(date +%s.%N)" "$1" "$$" "$SESSION_LOCK_STRATEGY" >> "$BUBBLES_LOCK_TRACE" 2>/dev/null || true; } # LOCKTRACE-DEBUG

cleanup_temp_files() {
  [[ -z "$NORMALIZED_PACKET_FILE" ]] || rm -f "$NORMALIZED_PACKET_FILE"
  [[ -z "$TMP_FILE" ]] || rm -f "$TMP_FILE"
  [[ -z "$CONV_TMP" ]] || rm -f "$CONV_TMP"
  [[ -z "$POLICY_TMP" ]] || rm -f "$POLICY_TMP"
  [[ -z "$PRE_TRANSACTION_STATE" ]] || rm -f "$PRE_TRANSACTION_STATE"
}

trap cleanup_temp_files EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

NORMALIZED_PACKET_FILE="$(mktemp)"
cp -- "$BINDING_PACKET_FILE" "$NORMALIZED_PACKET_FILE" || {
  echo "state-snapshot: unable to read binding packet" >&2
  exit 2
}
chmod 600 "$NORMALIZED_PACKET_FILE"

MIRROR_GOAL_NODE_ARGS=()
if [[ -n "$SCENARIO_FILE" ]]; then
  MIRROR_GOAL_NODE_ARGS=(--scenario-file "$SCENARIO_FILE" --node-id "$NODE_ID")
fi

set +e
BINDING_OUTPUT="$(bash "$REPOSITORY_BINDING" validate-packet \
  --session-id "$SESSION_ID" \
  --session-control-file "$SESSION_CONTROL_FILE" \
  --packet-file "$NORMALIZED_PACKET_FILE" \
  ${MIRROR_GOAL_NODE_ARGS[@]+"${MIRROR_GOAL_NODE_ARGS[@]}"} 2>&1)"
BINDING_RC=$?
set -e
if [[ "$BINDING_RC" -ne 0 ]]; then
  printf '%s\n' "$BINDING_OUTPUT" >&2
  exit "$BINDING_RC"
fi

REPO_ROOT="$(jq -r '.repositoryRoot' "$NORMALIZED_PACKET_FILE")"
PACKET_SESSION_ID="$(jq -r '.repositoryResolution.sessionId' "$NORMALIZED_PACKET_FILE")"
if [[ "$PACKET_SESSION_ID" != "$SESSION_ID" ]]; then
  echo "state-snapshot: validated packet session does not match --session-id" >&2
  exit 2
fi
SESSION_STATE_IO="$SCRIPT_DIR/session-state-io.py"
[[ -f "$SESSION_STATE_IO" ]] || {
  echo "state-snapshot: session state I/O helper is unavailable" >&2
  exit 3
}
if ! command -v python3 >/dev/null 2>&1; then
  echo "state-snapshot: python3 is required for descriptor-safe session state I/O." >&2
  exit 3
fi
SESSION_DIR="$REPO_ROOT/.specify/memory"
SESSION_FILE="$SESSION_DIR/bubbles.session.json"

# Resolve posture only after packet authority establishes the repository and
# exact host session. A first policy write supplies its requested boundedness.
# Every later unattended lookup reads only the validated session policy head.
if [[ -z "$POSTURE" && -x "$SCRIPT_DIR/autonomy-resolve.sh" ]]; then
  AUTONOMY_ARGS=(--format json --repo-root "$REPO_ROOT")
  if [[ -n "$SESSION_BUDGET_JSON" ]]; then
    REQUESTED_BUDGET_STATE="$(jq -r '
      [
        .maxTotalConvergenceIterations,
        .maxWallClockMinutes,
        .maxToolCalls,
        .maxSingleToolResultBytes,
        .maxCumulativeToolResultBytes,
        .maxPromptTokensPerRequest,
        .maxCumulativePromptTokens
      ]
      | if any(.[]; . != null) then "bounded" else "unbounded" end
    ' <<< "$SESSION_BUDGET_JSON")"
    AUTONOMY_ARGS+=(--session-budget "$REQUESTED_BUDGET_STATE")
  else
    AUTONOMY_ARGS+=(
      --session-id "$SESSION_ID"
      --session-control-file "$SESSION_CONTROL_FILE"
      --binding-packet-file "$NORMALIZED_PACKET_FILE"
    )
    if [[ -n "$SCENARIO_FILE" ]]; then
      AUTONOMY_ARGS+=(--scenario-file "$SCENARIO_FILE" --node-id "$NODE_ID")
    fi
  fi
  POSTURE="$(bash "$SCRIPT_DIR/autonomy-resolve.sh" \
    ${AUTONOMY_ARGS[@]+"${AUTONOMY_ARGS[@]}"} 2>/dev/null |
    sed -n 's/.*"autonomy":"\([^"]*\)".*/\1/p')"
fi

PRE_TRANSACTION_STATE=""
if [[ -n "${BUBBLES_STATE_SNAPSHOT_TRANSACTION_ACTIVE:-}" && -n "$SESSION_BUDGET_JSON" ]]; then
  PRE_TRANSACTION_STATE="$(mktemp)"
  chmod 600 "$PRE_TRANSACTION_STATE"
  if [[ -f "$SESSION_FILE" ]]; then
    cp -- "$SESSION_FILE" "$PRE_TRANSACTION_STATE"
  else
    printf '{}\n' > "$PRE_TRANSACTION_STATE"
  fi
fi

case "$LOCK_TRANSACTION_MODE" in
  "")
    if session_lock_have_flock; then
      SESSION_LOCK_STRATEGY="flock-run"
    else
      SESSION_LOCK_STRATEGY="mkdir-run"
    fi
    ;;
  flock|flock-run) SESSION_LOCK_STRATEGY="flock-run" ;;
  mkdir|mkdir-run) SESSION_LOCK_STRATEGY="mkdir-run" ;;
  *)
    echo "state-snapshot: invalid BUBBLES_STATE_SNAPSHOT_LOCK_TRANSACTION" >&2
    exit 2
    ;;
esac

TRANSACTION_ARGS=(
  --phase "$PHASE"
  --mode "$MODE"
  --session-id "$SESSION_ID"
  --session-control-file "$SESSION_CONTROL_FILE"
  --binding-packet-file "$NORMALIZED_PACKET_FILE"
)
[[ -z "$SCOPE_ID" ]] || TRANSACTION_ARGS+=(--scope-id "$SCOPE_ID")
[[ -z "$OCCURRENCE_ID" ]] || TRANSACTION_ARGS+=(--occurrence-id "$OCCURRENCE_ID")
[[ -z "$NOTE" ]] || TRANSACTION_ARGS+=(--note "$NOTE")
[[ -z "$POSTURE" ]] || TRANSACTION_ARGS+=(--posture "$POSTURE")
if [[ -n "$CONTEXT_BOUNDARY_KIND" ]]; then
  if [[ -n "$CONTEXT_BOUNDARY_ID" ]]; then
    TRANSACTION_ARGS+=(--context-boundary "$CONTEXT_BOUNDARY_KIND:$CONTEXT_BOUNDARY_ID")
  else
    TRANSACTION_ARGS+=(--context-boundary "$CONTEXT_BOUNDARY_KIND")
  fi
fi
[[ -z "$DECISION" ]] || TRANSACTION_ARGS+=(--decision "$DECISION")
[[ -z "$DECISION_PRINCIPLE" ]] || TRANSACTION_ARGS+=(--decision-principle "$DECISION_PRINCIPLE")
[[ -z "$DECISION_CHOSE" ]] || TRANSACTION_ARGS+=(--decision-chose "$DECISION_CHOSE")
[[ -z "$DECISION_CONSIDERED" ]] || TRANSACTION_ARGS+=(--decision-considered "$DECISION_CONSIDERED")
if [[ -n "$CONV_ITER" ]]; then
  TRANSACTION_ARGS+=(--convergence-iteration "$CONV_ITER" --spec-dir "$SPEC_DIR")
fi
if [[ -n "$SESSION_BUDGET_JSON" ]]; then
  TRANSACTION_ARGS+=(
    --session-budget-json "$SESSION_BUDGET_JSON"
    --expected-session-budget-revision "$EXPECTED_SESSION_BUDGET_REVISION"
  )
fi
if [[ -n "$SCENARIO_FILE" ]]; then
  TRANSACTION_ARGS+=(--scenario-file "$SCENARIO_FILE" --node-id "$NODE_ID")
fi

if [[ -z "${BUBBLES_STATE_SNAPSHOT_TRANSACTION_ACTIVE:-}" ]]; then
  if [[ "$SESSION_LOCK_STRATEGY" == "flock-run" ]]; then
    LOCK_RELATIVE_PATH=".specify/runtime/session-state/bubbles.session.json.flock"
  else
    LOCK_RELATIVE_PATH=".specify/runtime/session-state/bubbles.session.json.lock"
  fi
  _lock_trace REQUEST
  BUBBLES_STATE_SNAPSHOT_TRANSACTION_ACTIVE=1 \
    BUBBLES_STATE_SNAPSHOT_LOCK_TRANSACTION="$SESSION_LOCK_STRATEGY" \
    BUBBLES_AGENT_NAME="${BUBBLES_AGENT_NAME:-}" \
    python3 "$SESSION_STATE_IO" "$SESSION_LOCK_STRATEGY" \
      --root "$REPO_ROOT" \
      --relative-lock "$LOCK_RELATIVE_PATH" \
      --timeout-seconds 120 -- \
      "$BASH" "${BASH_SOURCE[0]}" ${TRANSACTION_ARGS[@]+"${TRANSACTION_ARGS[@]}"}
  exit $?
fi

AGENT_NAME="${BUBBLES_AGENT_NAME:-unknown}"
TIMESTAMP="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

# --- Exact-session policy compare-and-append -------------------------------

if [[ -n "$SESSION_BUDGET_JSON" ]]; then
  if [[ -n "$PRE_TRANSACTION_STATE" ]]; then
    POLICY_SOURCE_FILE="$PRE_TRANSACTION_STATE"
  else
    POLICY_SOURCE_FILE="$SESSION_FILE"
  fi
  POLICY_TMP="$(mktemp)"
  chmod 600 "$POLICY_TMP"
  set +e
  jq \
    --arg hostSessionId "$SESSION_ID" \
    --arg recordedAt "$TIMESTAMP" \
    --argjson expectedRevision "$EXPECTED_SESSION_BUDGET_REVISION" \
    --argjson requestedBudget "$SESSION_BUDGET_JSON" '
    def cap_keys:
      ["schemaVersion", "maxTotalConvergenceIterations", "maxWallClockMinutes",
       "maxToolCalls", "maxSingleToolResultBytes", "maxCumulativeToolResultBytes",
       "maxPromptTokensPerRequest", "maxCumulativePromptTokens"];
    def outer_keys:
      ["recordSchemaVersion", "hostSessionId", "revision", "supersedesRevision",
       "recordedAt", "budget"];
    def valid_timestamp:
      type == "string"
      and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")
      and (try (fromdateiso8601 | type == "number") catch false);
    def valid_budget:
      type == "object"
      and ((keys - cap_keys) | length) == 0
      and .schemaVersion == 1
      and ([cap_keys[1:][] as $key
        | ((has($key) | not) or .[$key] == null
           or ((.[$key] | type) == "number" and (.[$key] | floor) == .[$key] and .[$key] >= 0))]
        | all);
    def valid_record:
      type == "object"
      and ((keys | sort) == (outer_keys | sort))
      and .recordSchemaVersion == 1
      and (.hostSessionId | type) == "string" and (.hostSessionId | length) > 0
      and (.revision | type) == "number" and (.revision | floor) == .revision and .revision > 0
      and (.supersedesRevision == null
           or ((.supersedesRevision | type) == "number"
               and (.supersedesRevision | floor) == .supersedesRevision
               and .supersedesRevision > 0
               and .supersedesRevision < .revision))
      and (.recordedAt | valid_timestamp)
      and (.budget | valid_budget);
    def normalized_budget:
      {
        schemaVersion: 1,
        maxTotalConvergenceIterations: (.maxTotalConvergenceIterations // null),
        maxWallClockMinutes: (.maxWallClockMinutes // null),
        maxToolCalls: (.maxToolCalls // null),
        maxSingleToolResultBytes: (.maxSingleToolResultBytes // null),
        maxCumulativeToolResultBytes: (.maxCumulativeToolResultBytes // null),
        maxPromptTokensPerRequest: (.maxPromptTokensPerRequest // null),
        maxCumulativePromptTokens: (.maxCumulativePromptTokens // null)
      };
    def select_head($records):
      if ($records | length) == 0 then {revision: 0, budget: null}
      elif (all($records[]; valid_record) | not) then error("invalid-policy-record")
      elif (($records | map(.revision) | unique | length) != ($records | length)) then error("duplicate-policy-revision")
      elif (($records | map(select(.revision == 1 and .supersedesRevision == null)) | length) != 1) then error("missing-policy-root")
      elif ([$records[] | select(.revision > 1) as $record
             | (([$records[] | select(.revision == ($record.revision - 1))] | length) != 1
                or $record.supersedesRevision != ($record.revision - 1))]
            | any) then error("branching-policy-chain")
      else ($records | max_by(.revision))
      end;
    if (.sessionBudgetHistory? != null and (.sessionBudgetHistory | type) != "array") then
      error("invalid-policy-history")
    elif ($requestedBudget | valid_budget | not) then
      error("invalid-requested-budget")
    else
      . as $root
      | ($root.sessionBudgetHistory // []) as $history
      | ([$history[] | select(.hostSessionId == $hostSessionId)]) as $records
      | select_head($records) as $head
      | ($requestedBudget | normalized_budget) as $normalized
      | if $head.revision == 0 and $expectedRevision == 0 then
          $root + {sessionBudgetHistory: ($history + [{
            recordSchemaVersion: 1,
            hostSessionId: $hostSessionId,
            revision: 1,
            supersedesRevision: null,
            recordedAt: $recordedAt,
            budget: $normalized
          }])}
        elif $head.revision == 1 and $expectedRevision == 0 and $head.budget == $normalized then
          $root
        elif $head.revision != $expectedRevision then
          error("stale-policy-revision")
        elif $head.budget == $normalized then
          $root
        else
          $root + {sessionBudgetHistory: ($history + [{
            recordSchemaVersion: 1,
            hostSessionId: $hostSessionId,
            revision: ($head.revision + 1),
            supersedesRevision: $head.revision,
            recordedAt: $recordedAt,
            budget: $normalized
          }])}
        end
    end
    ' "$POLICY_SOURCE_FILE" > "$POLICY_TMP"
  POLICY_RC=$?
  set -e
  if [[ "$POLICY_RC" -ne 0 ]]; then
    echo "state-snapshot: session policy compare-and-append refused" >&2
    exit 4
  fi
fi

set +e
BINDING_OUTPUT="$(bash "$REPOSITORY_BINDING" mirror-session \
  --session-id "$SESSION_ID" \
  --session-control-file "$SESSION_CONTROL_FILE" \
  --packet-file "$NORMALIZED_PACKET_FILE" \
  ${MIRROR_GOAL_NODE_ARGS[@]+"${MIRROR_GOAL_NODE_ARGS[@]}"} 2>&1)"
BINDING_RC=$?
set -e
if [[ "$BINDING_RC" -ne 0 ]]; then
  printf '%s\n' "$BINDING_OUTPUT" >&2
  exit "$BINDING_RC"
fi

if [[ ! -f "$SESSION_FILE" ]]; then
  printf '{}\n' > "$SESSION_FILE"
fi

# --- Build snapshot record -------------------------------------------------

# Compute next turnNumber from existing turnSnapshots array length.
NEXT_TURN="$(jq '
  (.turnSnapshots // []) | length + 1
' "$SESSION_FILE")"

# Append a new record. We use --argjson for ints, --arg for strings, and
# pass scope_id / note as strings that may be empty (mapped to null below).
#
# `goalRef` is DERIVED from `.goalContract` in this same read, never accepted as
# a flag (IMP-038 SCOPE-3 / GF-1, GF-5). A caller-supplied ref could disagree
# with the contract the turn actually ran under, which is precisely the
# substitution this field exists to make detectable. It is `null` for a
# read-only or pre-IMP-038 run that froze no contract. The projection matches
# `goal-contract.sh ref` exactly: identity plus boundary, no contract prose.
TMP_FILE="$(mktemp "$SESSION_DIR/.bubbles.session.json.update.XXXXXX")"

jq \
  --argjson turn "$NEXT_TURN" \
  --arg timestamp "$TIMESTAMP" \
  --arg phase "$PHASE" \
  --arg scope_id "$SCOPE_ID" \
  --arg occurrence_id "$OCCURRENCE_ID" \
  --arg note "$NOTE" \
  --arg mode "$MODE" \
  --arg posture "$POSTURE" \
  --arg cbKind "$CONTEXT_BOUNDARY_KIND" \
  --arg cbId "$CONTEXT_BOUNDARY_ID" \
  --arg mbePosture "$MBE_POSTURE" \
  --argjson mbeEpoch "${MBE_EPOCH_JSON:-null}" \
  --arg decision "$DECISION" \
  --arg dprinciple "$DECISION_PRINCIPLE" \
  --arg dchose "$DECISION_CHOSE" \
  --arg dconsidered "$DECISION_CONSIDERED" \
  --arg agent "$AGENT_NAME" \
  --arg host_session "$SESSION_ID" \
  --arg has_policy "$([[ -n "$SESSION_BUDGET_JSON" ]] && printf true || printf false)" \
  --slurpfile policy_state "${POLICY_TMP:-$SESSION_FILE}" \
  '
  def goal_ref:
    if (.goalContract | type) == "object" then
      { goalId: .goalContract.goalId,
        revision: .goalContract.revision,
        sourceRequestDigest: .goalContract.sourceRequestDigest,
        workBoundary: .goalContract.workBoundary }
    else null end;
    . as $live
    | (if $has_policy == "true"
      then $live + {sessionBudgetHistory: $policy_state[0].sessionBudgetHistory}
      else $live
      end) as $root
  | ($root | goal_ref) as $goalRef
  | ($root + {
      turnSnapshots: ((($root.turnSnapshots // []) + [
        {
          turnNumber: $turn,
          timestamp: $timestamp,
          phase: $phase,
          occurrenceId: (if $occurrence_id == "" then null else $occurrence_id end),
          scopeId: (if $scope_id == "" then null else $scope_id end),
          mode: $mode,
          posture: (if $posture == "" then null else $posture end),
          note: (if $note == "" then null else $note end),
          agent: $agent,
          hostSessionId: (if $host_session == "" then null else $host_session end),
          goalRef: $goalRef,
          mbeEpoch: $mbeEpoch
        }
      ])),
      autonomyPosture: (if $posture == "" then ($root.autonomyPosture // null) else $posture end),
      contextBoundary: (
        if $cbKind == "" then ($root.contextBoundary // null)
        else ({ kind: $cbKind,
                checkpointId: (if $cbId == "" then null else $cbId end),
                at: $timestamp }
              + (if $mbeEpoch == null then {} else { mbeEpoch: $mbeEpoch } end))
        end
      ),
      autonomyDecisions: (
        if $decision == "" then ($root.autonomyDecisions // [])
        else (($root.autonomyDecisions // []) + [{
          turnNumber: $turn,
          timestamp: $timestamp,
          description: $decision,
          principle: (if $dprinciple == "" then null else $dprinciple end),
          chose: (if $dchose == "" then null else $dchose end),
          considered: (if $dconsidered == "" then []
                       else ($dconsidered | split(",") | map(gsub("^ +| +$"; "")) | map(select(length > 0))) end),
          posture: (if $posture == "" then null else $posture end),
          agent: $agent
        }])
        end
      )
    })
    + (if $mbePosture == "off" then {} else { mbeRolloutPosture: $mbePosture } end)
  ' "$SESSION_FILE" > "$TMP_FILE"

mv "$TMP_FILE" "$SESSION_FILE"
TMP_FILE=""
[[ -z "$POLICY_TMP" ]] || rm -f "$POLICY_TMP"
POLICY_TMP=""
[[ -z "$PRE_TRANSACTION_STATE" ]] || rm -f "$PRE_TRANSACTION_STATE"
PRE_TRANSACTION_STATE=""

# --- Convergence loop update (Gate G082) -----------------------------------
#
# When both --convergence-iteration and --spec-dir are supplied, additively
# update the `convergenceLoops[]` array entry keyed by
# (hostSessionId, specDir, agent).
# If an entry for that key already exists, replace its `iterationCount` and
# `lastUpdated`. Otherwise append a new entry. Other entries (for other
# sessions, specs, or agents) are NEVER touched. Legacy entries without
# `hostSessionId` do not match the expanded key and remain stored unchanged.
#
# This array is consumed by `bubbles/scripts/convergence-cap-guard.sh`
# which enforces `maxConvergenceIterations` (default 10) per Gate G082.
if [[ -n "$CONV_ITER" && -n "$SPEC_DIR" ]]; then
  CONV_TMP="$(mktemp "$SESSION_DIR/.bubbles.session.json.convergence.XXXXXX")"
  jq \
    --arg hostSessionId "$SESSION_ID" \
    --arg specDir "$SPEC_DIR" \
    --arg agent "$AGENT_NAME" \
    --argjson iterationCount "$CONV_ITER" \
    --arg lastUpdated "$TIMESTAMP" \
    '
    def goal_ref:
      if (.goalContract | type) == "object" then
        { goalId: .goalContract.goalId,
          revision: .goalContract.revision,
          sourceRequestDigest: .goalContract.sourceRequestDigest,
          workBoundary: .goalContract.workBoundary }
      else null end;
    . as $root
    | ($root | goal_ref) as $goalRef
    | ($root.convergenceLoops // []) as $loops
    | ([ $loops[]
         | select(.hostSessionId != $hostSessionId or .specDir != $specDir or .agent != $agent)
       ] + [{
         hostSessionId: $hostSessionId,
         specDir: $specDir,
         agent: $agent,
         iterationCount: $iterationCount,
         lastUpdated: $lastUpdated,
         goalRef: $goalRef
       }]) as $updated
    | $root + { convergenceLoops: $updated }
    ' "$SESSION_FILE" > "$CONV_TMP"
  mv "$CONV_TMP" "$SESSION_FILE"
  CONV_TMP=""
fi

# Echo a one-line summary to stdout for orchestrator log capture.
if [[ -n "$CONV_ITER" && -n "$SPEC_DIR" ]]; then
  printf 'state-snapshot: turnNumber=%s mode=%s phase=%s scopeId=%s agent=%s convergenceIteration=%s specDir=%s\n' \
    "$NEXT_TURN" "$MODE" "$PHASE" "${SCOPE_ID:-null}" "$AGENT_NAME" "$CONV_ITER" "$SPEC_DIR"
else
  printf 'state-snapshot: turnNumber=%s mode=%s phase=%s scopeId=%s agent=%s\n' \
    "$NEXT_TURN" "$MODE" "$PHASE" "${SCOPE_ID:-null}" "$AGENT_NAME"
fi
