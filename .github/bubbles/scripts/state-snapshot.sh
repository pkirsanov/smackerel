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
         [--decision <text> [--decision-principle <name>] [--decision-chose <option>] \
          [--decision-considered <csv>]] \
         [--convergence-iteration <N> --spec-dir <path>] \
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
NOTE=""
MODE="start"
POSTURE=""
DECISION=""
DECISION_PRINCIPLE=""
DECISION_CHOSE=""
DECISION_CONSIDERED=""
CONV_ITER=""
SPEC_DIR=""
SESSION_ID=""
SESSION_CONTROL_FILE=""
BINDING_PACKET_FILE=""

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
    --posture)
      [[ $# -ge 2 ]] || { echo "state-snapshot: --posture requires a value" >&2; exit 2; }
      POSTURE="$2"
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

# --- Exclusive session-file lock (concurrency safety) ----------------------
#
# One state-snapshot run performs a read-modify-`mv` on bubbles.session.json in
# up to three places: the mirror-session subprocess (which sets
# `.repositoryBindingMirror`), the turnSnapshots append, and the convergenceLoops
# update. Without a lock, two concurrent state-snapshot runs both read the same
# session file and both `mv` their result, silently discarding one update. A lost
# convergenceLoops update under-counts iterations and weakens Gate G082/G128
# convergence-cap enforcement. A single exclusive lock, held from before
# mirror-session through the final update, serializes the whole interaction so no
# update is lost.
#
# Lock strategy is flock-first. `flock` (util-linux) is a kernel-managed,
# race-free advisory lock: the kernel serializes concurrent acquirers, so there
# is NO stale-detect/break window in which two runs could both enter the critical
# section. It is the PRIMARY path (Linux/CI/selftest). `flock` is absent only on
# stock macOS; there we fall back to a mkdir mutex whose stale/defensive break is
# made ATOMIC via a rename-claim (renaming a directory is atomic, so exactly one
# breaker wins and no live/fresh lock is ever destroyed), still using the holder
# pid + lock-dir mtime to DECIDE staleness and recover a lock left behind by a
# SIGKILLed holder instead of spinning forever.
SESSION_LOCK_DIR=""
SESSION_LOCK_PID_FILE=""
SESSION_LOCK_FILE=""
SESSION_LOCK_MODE=""
SESSION_LOCK_HELD=false

# Detect flock once at acquire time; release routes on SESSION_LOCK_MODE (the
# strategy actually used), never on a re-probe.
session_lock_have_flock() {
  command -v flock >/dev/null 2>&1
}

_lock_trace() { [[ -z "${BUBBLES_LOCK_TRACE:-}" ]] || printf '%s %s %s %s\n' "$(date +%s.%N)" "$1" "$$" "$SESSION_LOCK_MODE" >> "$BUBBLES_LOCK_TRACE" 2>/dev/null || true; } # LOCKTRACE-DEBUG

session_lock_mtime_epoch() {
  stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null || printf '%s' '0'
}

# Returns 0 = stale (safe to break), 1 = held by a live, non-stale holder.
session_lock_is_stale() {
  local pid='' mtime now age max
  [[ -d "$SESSION_LOCK_DIR" ]] || return 0

  if [[ -f "$SESSION_LOCK_PID_FILE" ]]; then
    pid="$(cat "$SESSION_LOCK_PID_FILE" 2>/dev/null || true)"
  fi
  pid="${pid//[[:space:]]/}"

  max=600
  mtime="$(session_lock_mtime_epoch "$SESSION_LOCK_DIR")"
  now="$(date -u +%s 2>/dev/null || printf '%s' '0')"
  age=-1
  if [[ "$mtime" =~ ^[0-9]+$ && "$now" =~ ^[0-9]+$ ]]; then
    age=$(( now - mtime ))
  fi

  if [[ -n "$pid" && "$pid" =~ ^[0-9]+$ ]]; then
    # A holder pid is recorded: a dead holder is stale immediately; a live holder
    # is stale only if the lock has outlived the age cap (defensive).
    if ! kill -0 "$pid" 2>/dev/null; then
      return 0
    fi
    if (( age > max )); then
      return 0
    fi
    return 1
  fi

  # No holder pid recorded yet. mkdir wins the lock, THEN the holder records its
  # pid, so there is a brief window where the dir exists with no pid file. Do NOT
  # break a freshly created lock (that window is an in-flight acquirer, not a
  # crash) — only a lock dir aged past the stale threshold with no live holder is
  # genuinely stale. This closes the acquire/pid-write TOCTOU that would
  # otherwise let two waiters both break each other's fresh lock and lose an
  # update. A truly wedged pid-less lock is still recovered by acquire's bounded
  # max-wait defensive break.
  if (( age > max )); then
    return 0
  fi
  return 1
}

# Take the exclusive session lock. flock-first (race-free); mkdir mutex only
# where flock is unavailable.
acquire_session_lock() {
  if session_lock_have_flock; then
    acquire_session_lock_flock
  else
    acquire_session_lock_mkdir
  fi
}

# PRIMARY path: kernel-managed flock on a dedicated lock file next to the session
# file. flock is race-free — the kernel blocks concurrent acquirers until the
# holder releases, so there is no stale-detect/break step and therefore no window
# in which two runs could both hold the lock (the exact residual race the mkdir
# mutex had). A BOUNDED `-w` timeout stops a genuinely wedged holder from
# deadlocking THIS run forever; on timeout we fail loudly and non-zero rather
# than silently proceeding unlocked. A fixed FD (9) is used so the `exec 9>`
# redirection works on every bash (the dynamic `exec {fd}>` form needs bash
# >=4.1; the flock path only runs where flock exists, but a fixed FD keeps it
# version-independent). The lock FILE is created once and never unlinked (see
# release_session_lock).
acquire_session_lock_flock() {
  local flock_wait=120
  exec 9>"$SESSION_LOCK_FILE" || {
    echo "state-snapshot: unable to open session lock file: $SESSION_LOCK_FILE" >&2
    exit 3
  }
  if ! flock -x -w "$flock_wait" 9; then
    echo "state-snapshot: timed out after ${flock_wait}s acquiring the exclusive session lock." >&2
    echo "  Lock file: $SESSION_LOCK_FILE" >&2
    echo "  Another state-snapshot run appears wedged holding it; refusing to proceed unlocked." >&2
    exec 9>&- || true
    exit 3
  fi
  SESSION_LOCK_MODE="flock"
  SESSION_LOCK_HELD=true
  _lock_trace ACQUIRE # LOCKTRACE-DEBUG
}

# FALLBACK path (stock macOS, no flock): mkdir mutex. Staleness is DECIDED by
# session_lock_is_stale (holder pid liveness + lock-dir mtime) exactly as before;
# only the BREAK mechanism is hardened. A bare `rm -rf "$SESSION_LOCK_DIR"` can
# delete a lock another process is concurrently (re)acquiring — a transient
# DOUBLE-ACQUIRE that lets two runs enter the critical section and lose one `mv`.
# Instead we ATOMICALLY CLAIM the stale lock by renaming it to a unique path:
# renaming a directory is atomic, so exactly ONE concurrent breaker's `mv`
# succeeds; every loser's `mv` fails and it simply loops to re-check. Only the
# winner removes the CLAIMED (renamed) dir, so a live/fresh lock is never
# destroyed out from under its holder.
acquire_session_lock_mkdir() {
  local waited=0
  local max_wait=600
  local claim
  while true; do
    if mkdir "$SESSION_LOCK_DIR" 2>/dev/null; then
      printf '%s\n' "$$" > "$SESSION_LOCK_PID_FILE" 2>/dev/null || true
      SESSION_LOCK_MODE="mkdir"
      SESSION_LOCK_HELD=true
      _lock_trace ACQUIRE # LOCKTRACE-DEBUG
      return 0
    fi
    if session_lock_is_stale; then
      _lock_trace BREAK-STALE # LOCKTRACE-DEBUG
      claim="$SESSION_LOCK_DIR.stale.$$.${RANDOM}"
      if mv "$SESSION_LOCK_DIR" "$claim" 2>/dev/null; then
        rm -rf "$claim" 2>/dev/null || true
      fi
      continue
    fi
    waited=$(( waited + 1 ))
    if (( waited > max_wait )); then
      # A live holder has exceeded the wait budget; break it defensively — but
      # STILL atomically (rename-claim), so a concurrent fresh acquirer's lock is
      # never destroyed out from under it.
      _lock_trace BREAK-DEFENSIVE # LOCKTRACE-DEBUG
      claim="$SESSION_LOCK_DIR.stale.$$.${RANDOM}"
      if mv "$SESSION_LOCK_DIR" "$claim" 2>/dev/null; then
        rm -rf "$claim" 2>/dev/null || true
      fi
      continue
    fi
    sleep 0.1
  done
}

release_session_lock() {
  [[ "$SESSION_LOCK_HELD" == true ]] || return 0
  _lock_trace RELEASE # LOCKTRACE-DEBUG
  if [[ "$SESSION_LOCK_MODE" == "flock" ]]; then
    # Release by closing the FD (drops the kernel lock). The lock FILE is
    # deliberately LEFT in place: unlinking it would let a new acquirer create
    # and lock a fresh inode while an old holder still holds the previous one —
    # reintroducing a race. flock keys on the open file description, not the path.
    exec 9>&- || true
  else
    rm -f "$SESSION_LOCK_PID_FILE" 2>/dev/null || true
    rmdir "$SESSION_LOCK_DIR" 2>/dev/null || rm -rf "$SESSION_LOCK_DIR" 2>/dev/null || true
  fi
  SESSION_LOCK_HELD=false
}

cleanup_temp_files() {
  release_session_lock
  [[ -z "$NORMALIZED_PACKET_FILE" ]] || rm -f "$NORMALIZED_PACKET_FILE"
  [[ -z "$TMP_FILE" ]] || rm -f "$TMP_FILE"
  [[ -z "$CONV_TMP" ]] || rm -f "$CONV_TMP"
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

# Resolve the repository-local session file from the caller-normalized packet and
# take the exclusive session lock BEFORE mirror-session runs. mirror-session
# (repository-binding.sh) performs its own read-modify-`mv` on this same session
# file, so the lock must span from here through the turnSnapshots +
# convergenceLoops updates below for concurrent runs to be lose-update-free.
# The authoritative repository root is still the packet's `.repositoryRoot`
# (the same value mirror-session validates and uses); locking only proceeds for a
# well-formed absolute root, so a malformed packet falls through to the existing
# mirror-session refusal below without creating a spurious lock.
REPO_ROOT="$(jq -r '.repositoryRoot' "$NORMALIZED_PACKET_FILE")"
SESSION_DIR="$REPO_ROOT/.specify/memory"
SESSION_FILE="$SESSION_DIR/bubbles.session.json"
SESSION_LOCK_DIR="$SESSION_FILE.lock"
SESSION_LOCK_PID_FILE="$SESSION_LOCK_DIR/holder.pid"
SESSION_LOCK_FILE="$SESSION_FILE.flock"
if [[ -n "$REPO_ROOT" && "$REPO_ROOT" == /* ]]; then
  mkdir -p "$SESSION_DIR"
  acquire_session_lock
fi

set +e
BINDING_OUTPUT="$(bash "$REPOSITORY_BINDING" mirror-session \
  --session-id "$SESSION_ID" \
  --session-control-file "$SESSION_CONTROL_FILE" \
  --packet-file "$NORMALIZED_PACKET_FILE" 2>&1)"
BINDING_RC=$?
set -e
if [[ "$BINDING_RC" -ne 0 ]]; then
  printf '%s\n' "$BINDING_OUTPUT" >&2
  exit "$BINDING_RC"
fi

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
  --arg note "$NOTE" \
  --arg mode "$MODE" \
  --arg posture "$POSTURE" \
  --arg decision "$DECISION" \
  --arg dprinciple "$DECISION_PRINCIPLE" \
  --arg dchose "$DECISION_CHOSE" \
  --arg dconsidered "$DECISION_CONSIDERED" \
  --arg agent "$AGENT_NAME" \
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
  | ($root + {
      turnSnapshots: ((($root.turnSnapshots // []) + [
        {
          turnNumber: $turn,
          timestamp: $timestamp,
          phase: $phase,
          scopeId: (if $scope_id == "" then null else $scope_id end),
          mode: $mode,
          posture: (if $posture == "" then null else $posture end),
          note: (if $note == "" then null else $note end),
          agent: $agent,
          goalRef: $goalRef
        }
      ])),
      autonomyPosture: (if $posture == "" then ($root.autonomyPosture // null) else $posture end),
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
  ' "$SESSION_FILE" > "$TMP_FILE"

mv "$TMP_FILE" "$SESSION_FILE"
TMP_FILE=""

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
  CONV_TMP="$(mktemp "$SESSION_DIR/.bubbles.session.json.convergence.XXXXXX")"
  jq \
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
         | select(.specDir != $specDir or .agent != $agent)
       ] + [{
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

# Release the exclusive session lock now that every read-modify-write critical
# section (mirror-session mirror, turnSnapshots, convergenceLoops) has completed.
# (The EXIT trap also releases it; this frees it promptly on the happy path.)
release_session_lock

# Echo a one-line summary to stdout for orchestrator log capture.
if [[ -n "$CONV_ITER" && -n "$SPEC_DIR" ]]; then
  printf 'state-snapshot: turnNumber=%s mode=%s phase=%s scopeId=%s agent=%s convergenceIteration=%s specDir=%s\n' \
    "$NEXT_TURN" "$MODE" "$PHASE" "${SCOPE_ID:-null}" "$AGENT_NAME" "$CONV_ITER" "$SPEC_DIR"
else
  printf 'state-snapshot: turnNumber=%s mode=%s phase=%s scopeId=%s agent=%s\n' \
    "$NEXT_TURN" "$MODE" "$PHASE" "${SCOPE_ID:-null}" "$AGENT_NAME"
fi
