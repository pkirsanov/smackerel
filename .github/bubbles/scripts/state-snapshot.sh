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
         [--decision <text> [--decision-principle <name>] [--decision-chose <option>] \
          [--decision-considered <csv>]] \
         [--convergence-iteration <N> --spec-dir <path>] \
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
        occurrenceId(the --occurrence-id value or null; mirrors `phase` with an
                     identity that is distinct across repeated occurrences)
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
# stock macOS; there we fall back to a mkdir mutex that still uses the holder pid
# + lock-dir mtime to DECIDE staleness and recover a lock left behind by a
# SIGKILLed holder instead of spinning forever.
#
# The mkdir fallback breaks a stale lock in two separate steps — DECIDE
# (session_lock_is_stale) then ACT (rename-claim) — and the pair is NOT atomic.
# The rename is atomic only in the sense that exactly one concurrent breaker wins
# it; on its own it does not prove the directory being renamed is still the
# instance that was judged stale. Two guards close that window:
#   1. An ABSENT lock dir is never reported as stale. There is nothing to break,
#      so the waiter just races for mkdir again. Reporting absent as stale is what
#      let a waiter run the destructive break after the lock had merely been
#      released — by then a third process had legitimately won a FRESH lock, and
#      the rename destroyed that live lock, putting two runs in the critical
#      section and losing an update.
#   2. The judged instance's identity (lock-dir inode + recorded holder pid) is
#      re-verified immediately before the rename, so a lock released and re-taken
#      between decide and act is left alone.
SESSION_LOCK_DIR=""
SESSION_LOCK_PID_FILE=""
SESSION_LOCK_FILE=""
SESSION_LOCK_MODE=""
SESSION_LOCK_HELD=false
# Identity of the lock instance the most recent session_lock_is_stale call judged.
SESSION_LOCK_JUDGED_IDENTITY=""

# Detect flock once at acquire time; release routes on SESSION_LOCK_MODE (the
# strategy actually used), never on a re-probe.
session_lock_have_flock() {
  command -v flock >/dev/null 2>&1
}

_lock_trace() { [[ -z "${BUBBLES_LOCK_TRACE:-}" ]] || printf '%s %s %s %s\n' "$(date +%s.%N)" "$1" "$$" "$SESSION_LOCK_MODE" >> "$BUBBLES_LOCK_TRACE" 2>/dev/null || true; } # LOCKTRACE-DEBUG

session_lock_mtime_epoch() {
  stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null || printf '%s' '0'
}

# Identity token for the lock-dir instance that exists right now: inode plus the
# recorded holder pid. A release followed by a re-acquire yields a different
# token, which is what lets a breaker distinguish "the instance I judged" from
# "an instance created since I judged". `ls -di` is used because it is POSIX on
# both userlands (field 1 is the inode); `stat` needs different flags per
# userland, which is why session_lock_mtime_epoch has to try -c then -f. Prints
# nothing when the lock dir is absent.
session_lock_identity() {
  local ino='' pid=''
  [[ -d "$SESSION_LOCK_DIR" ]] || return 0
  # shellcheck disable=SC2012  # one known path in, only field 1 (the inode number) out — no filename is parsed
  ino="$(ls -di "$SESSION_LOCK_DIR" 2>/dev/null | awk 'NR == 1 { print $1 }' || true)"
  if [[ -f "$SESSION_LOCK_PID_FILE" ]]; then
    pid="$(cat "$SESSION_LOCK_PID_FILE" 2>/dev/null || true)"
  fi
  pid="${pid//[[:space:]]/}"
  printf '%s:%s' "${ino:-?}" "${pid:-?}"
}

# True only while the lock dir is still the instance session_lock_is_stale
# judged. Re-read immediately before the destructive rename, because decide and
# act are separate steps and the judged lock can be released and legitimately
# re-created between them.
session_lock_identity_unchanged() {
  [[ -n "$SESSION_LOCK_JUDGED_IDENTITY" ]] || return 1
  [[ "$(session_lock_identity)" == "$SESSION_LOCK_JUDGED_IDENTITY" ]]
}

# Tri-state contract, read by the caller from the explicit exit code:
#   0 = a lock instance EXISTS and is stale (safe to break)
#   1 = a lock instance EXISTS and is held by a live, non-stale holder
#   2 = NO lock instance exists — nothing to judge, and nothing to break
# 2 is deliberately NOT folded into 0: an absent directory is not a stale lock.
# On 0 and 1 the judged instance's identity is published in
# SESSION_LOCK_JUDGED_IDENTITY for the caller to re-verify before acting on it.
session_lock_is_stale() {
  local pid='' mtime now age max
  SESSION_LOCK_JUDGED_IDENTITY=''
  [[ -d "$SESSION_LOCK_DIR" ]] || return 2
  SESSION_LOCK_JUDGED_IDENTITY="$(session_lock_identity)"
  # The dir can be released between the test above and this line. An instance
  # that no longer exists is absent, not stale. Without this check the mtime
  # lookup below falls back to epoch 0, the lock reads as ~infinitely old, and it
  # is judged "stale by age" — absent-is-not-stale leaking in a second disguise.
  [[ -n "$SESSION_LOCK_JUDGED_IDENTITY" ]] || return 2

  if [[ -f "$SESSION_LOCK_PID_FILE" ]]; then
    pid="$(cat "$SESSION_LOCK_PID_FILE" 2>/dev/null || true)"
  fi
  pid="${pid//[[:space:]]/}"

  max=600
  mtime="$(session_lock_mtime_epoch "$SESSION_LOCK_DIR")"
  # '0' is that helper's failure sentinel: both stat forms failed because the dir
  # was released while we were judging it. Unknown mtime must read as ABSENT, not
  # as "aged past the cap" — a 1970 mtime makes every vanished lock look stale,
  # which is absent-is-not-stale leaking in through the age test.
  [[ "$mtime" != "0" ]] || return 2
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

# Destroy the lock instance session_lock_is_stale just judged, via an ATOMIC
# rename-claim: renaming a directory is atomic, so exactly ONE concurrent breaker
# wins the `mv` and only that winner removes the claimed (renamed) dir. The
# rename alone does not establish that the dir is still the judged instance, so
# the identity is re-verified first. Returns 1 (break refused) when the instance
# changed under us, so the caller re-races mkdir instead of destroying a lock
# that now belongs to somebody else.
session_lock_break_judged_instance() {
  local reason="$1" claim
  if ! session_lock_identity_unchanged; then
    _lock_trace RETRY-IDENTITY # LOCKTRACE-DEBUG
    return 1
  fi
  _lock_trace "$reason" # LOCKTRACE-DEBUG
  claim="$SESSION_LOCK_DIR.stale.$$.${RANDOM}"
  if mv "$SESSION_LOCK_DIR" "$claim" 2>/dev/null; then
    rm -rf "$claim" 2>/dev/null || true
  fi
  return 0
}

# FALLBACK path (stock macOS, no flock): mkdir mutex. Staleness is DECIDED by
# session_lock_is_stale (holder pid liveness + lock-dir mtime); breaking is done
# by session_lock_break_judged_instance, which acts only on the instance that was
# judged. The absent case (exit 2) never reaches a break at all: a lock dir that
# is gone is not a stale lock, it is an uncontended one, and the only correct
# response is to race for mkdir again.
acquire_session_lock_mkdir() {
  local waited=0
  local max_wait=600
  local absent_spins=0
  local absent_max=2000
  local stale_rc
  while true; do
    if mkdir "$SESSION_LOCK_DIR" 2>/dev/null; then
      printf '%s\n' "$$" > "$SESSION_LOCK_PID_FILE" 2>/dev/null || true
      SESSION_LOCK_MODE="mkdir"
      SESSION_LOCK_HELD=true
      _lock_trace ACQUIRE # LOCKTRACE-DEBUG
      return 0
    fi

    stale_rc=0
    session_lock_is_stale || stale_rc=$?

    if (( stale_rc == 2 )); then
      # Our mkdir lost to a holder that has since released. Nothing to break.
      _lock_trace RETRY-ABSENT # LOCKTRACE-DEBUG
      absent_spins=$(( absent_spins + 1 ))
      if (( absent_spins > absent_max )); then
        # mkdir kept failing while the dir kept reading as absent, so the failure
        # is structural (permissions, full filesystem) rather than contention.
        # Bail loudly instead of spinning, and never proceed unlocked.
        echo "state-snapshot: unable to create the session lock directory after ${absent_max} consecutive attempts." >&2
        echo "  Lock dir: $SESSION_LOCK_DIR" >&2
        echo "  mkdir kept failing while the directory read as ABSENT, which is a filesystem or permission" >&2
        echo "  failure rather than lock contention; refusing to proceed unlocked." >&2
        exit 3
      fi
      continue
    fi
    absent_spins=0

    if (( stale_rc == 0 )); then
      session_lock_break_judged_instance BREAK-STALE || sleep 0.1
      continue
    fi

    waited=$(( waited + 1 ))
    if (( waited > max_wait )); then
      # A live holder has exceeded the wait budget; break it defensively.
      session_lock_break_judged_instance BREAK-DEFENSIVE || sleep 0.1
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

MIRROR_GOAL_NODE_ARGS=()
if [[ -n "$SCENARIO_FILE" ]]; then
  MIRROR_GOAL_NODE_ARGS=(--scenario-file "$SCENARIO_FILE" --node-id "$NODE_ID")
fi

set +e
BINDING_OUTPUT="$(bash "$REPOSITORY_BINDING" mirror-session \
  --session-id "$SESSION_ID" \
  --session-control-file "$SESSION_CONTROL_FILE" \
  --packet-file "$NORMALIZED_PACKET_FILE" \
  "${MIRROR_GOAL_NODE_ARGS[@]}" 2>&1)"
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
  --arg occurrence_id "$OCCURRENCE_ID" \
  --arg note "$NOTE" \
  --arg mode "$MODE" \
  --arg posture "$POSTURE" \
  --arg cbKind "$CONTEXT_BOUNDARY_KIND" \
  --arg cbId "$CONTEXT_BOUNDARY_ID" \
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
          occurrenceId: (if $occurrence_id == "" then null else $occurrence_id end),
          scopeId: (if $scope_id == "" then null else $scope_id end),
          mode: $mode,
          posture: (if $posture == "" then null else $posture end),
          note: (if $note == "" then null else $note end),
          agent: $agent,
          goalRef: $goalRef
        }
      ])),
      autonomyPosture: (if $posture == "" then ($root.autonomyPosture // null) else $posture end),
      contextBoundary: (
        if $cbKind == "" then ($root.contextBoundary // null)
        else { kind: $cbKind,
               checkpointId: (if $cbId == "" then null else $cbId end),
               at: $timestamp }
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
