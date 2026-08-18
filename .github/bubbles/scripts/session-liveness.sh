#!/usr/bin/env bash
# session-liveness.sh — session-state liveness (IMP-048 SCOPE-7, WIP-5).
#
# Owner: bubbles.workflow
#
# WHY THIS EXISTS
# G083 (compaction discipline), G128 (session caps) and
# `trajectory-inspector.sh --health` all read
# `.specify/memory/bubbles.session.json`. Measured: knb's newest
# `turnSnapshots[]` entry was `2026-07-29T20:23:14Z` while a five-day August
# session ran to completion and appended NOTHING. `operating-baseline.md`
# documented snapshotting as what an orchestrator "SHOULD" do, with no
# consequence for silence, so three controls sat over an empty store and every
# one of them reported that nothing was wrong.
#
# THE LOAD-BEARING RULE: AN EMPTY STORE IS **UNMEASURED**, NEVER **PASS**.
# This is the same correction `retro-framework-health.sh` already applied to its
# own gate and capability sections: it prints
# "UNMEASURED: N manifest(s) record no scenario states" and
# "Capability validation age: UNMEASURED" instead of the confident
# "no stale capabilities detected" it used to print over a field the schema does
# not define. A filter over an absent field is a false negative wearing the word
# "detected", and a verdict of PASS over an empty store is the same defect with
# a different word. So this script separates the two facts and prints both:
#
#   liveness=<live|within-grace|finding|unattributed|unmeasured>   what was observed
#   verdict=<PASS|FINDING|STALE|UNMEASURED|SKIPPED>                what it means
#
# `verdict=PASS` is reachable ONLY from measured snapshots plus measured
# freshness. Nothing about an empty store can produce it.
#
# THREE OBLIGATIONS (IMP-048 SCOPE-7)
#   1  A run exceeding 3 turns MUST have appended a `turnSnapshots[]` entry.
#      Silence is a FINDING, not a silent pass. Three turns of grace exist so a
#      short read-only answer is not required to write session state.
#   2  Session state is keyed by HOST SESSION ID. `turnSnapshots[].hostSessionId`
#      (written by `state-snapshot.sh` from its already-required `--session-id`)
#      makes each record attributable, so two concurrent sessions in ONE
#      repository read back their own trajectory instead of each other's. This
#      repository genuinely runs concurrent sessions, so an unattributed
#      trajectory is not a hypothetical.
#   3  A session file whose newest snapshot predates the newest commit is STALE:
#      the repository moved and the session did not record it. `doctor` surfaces
#      this; it is ADVISORY and never changes doctor's exit code.
#
# ONE STORE, NOT TWO. Everything here reads the EXISTING
# `.specify/memory/bubbles.session.json` written by `state-snapshot.sh`. No
# second session store, no second trajectory file, no shadow index.
#
# NO NEW HARD DEPENDENCY. `jq` is used when present. When it is absent the
# answer is `verdict=UNMEASURED reason=jq-unavailable` — which is the honest
# result and, not by coincidence, the same rule as an empty store: a tool that
# cannot measure must not report a pass.
#
# DEFAULT OFF, per repo. With no `sessionLiveness:` block, no config file, or an
# explicit `adapter: none`, every subcommand is a clean no-op that writes
# nothing and reports `verdict=SKIPPED`. Same config shape as
# `sessionReview:`, `dispatchReceipts:` and `testLeafReceipts:`.
#
# READ-ONLY. This script never writes to the session file, never creates
# `.specify`, and never touches any `state.json`.
#
# Usage:
#   session-liveness.sh check [--session-id ID] [--turns N] [--repo-root PATH]
#   session-liveness.sh sessions [--repo-root PATH]
#
# Exit codes:
#   0  reported (PASS, STALE, UNMEASURED or SKIPPED) — all non-failures
#   1  FINDING: a run past the grace window appended no snapshot for its session
#   2  usage error, or a configured-but-unknown adapter
#
# There is no --skip, --force, --ignore or --assume flag.

set -euo pipefail

NAME="session-liveness"
STORE_REL=".specify/memory/bubbles.session.json"

# A run may answer a short question without recording session state. Past this
# many turns it may not: a run long enough to need resuming is long enough to
# owe a trajectory.
GRACE_TURNS=3

usage() {
  cat <<'EOF'
Usage:
  session-liveness.sh check    [--session-id ID] [--turns N] [--repo-root PATH]
  session-liveness.sh sessions [--repo-root PATH]

check reports, from .specify/memory/bubbles.session.json:
  snapshotCount / hostSessionCount / sessionSnapshotCount
  newestSnapshotAt vs newestCommitAt       (freshness: fresh | stale | unmeasured)
  liveness   live | within-grace | finding | unattributed | unmeasured
  verdict    PASS | FINDING | STALE | UNMEASURED | SKIPPED

An EMPTY store reports UNMEASURED. It never reports PASS.

Options:
  --session-id ID   Host session id to attribute snapshots to (turnSnapshots[].hostSessionId).
  --turns N         Turns this run has taken. Past 3, a session with no snapshot is a FINDING.
  --repo-root PATH  Repository to inspect (default: current directory).

Project config (default OFF):

  sessionLiveness:
    adapter: none | session-json
EOF
}

fail() {
  printf '%s: %s\n' "$NAME" "$1" >&2
  exit "${2:-1}"
}

die_usage() {
  printf '%s: %s\n' "$NAME" "$1" >&2
  usage >&2
  exit 2
}

valid_token() {
  case "$1" in
    '' | *[!A-Za-z0-9._:-]*) return 1 ;;
    *) return 0 ;;
  esac
}

valid_count() {
  case "$1" in
    '' | *[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

resolve_adapter() {
  local repo_root="$1" config_file='' adapter=''
  if [ -f "$repo_root/.github/bubbles-project.yaml" ]; then
    config_file="$repo_root/.github/bubbles-project.yaml"
  elif [ -f "$repo_root/bubbles-project.yaml" ]; then
    config_file="$repo_root/bubbles-project.yaml"
  fi

  if [ -n "$config_file" ]; then
    adapter="$(awk '
      /^[[:space:]]*#/ { next }
      /^sessionLiveness:[[:space:]]*$/ { inblock = 1; next }
      inblock && /^[^[:space:]]/ { inblock = 0 }
      inblock && $1 == "adapter:" {
        value = $2
        gsub(/["\047]/, "", value)
        print value
        exit
      }
    ' "$config_file" 2> /dev/null || true)"
  fi

  [ -n "$adapter" ] || adapter='none'

  # A configured-but-unknown adapter fails LOUD rather than degrading to `none`.
  # A typo that silently produced "not checking" would be indistinguishable from
  # a deliberate opt-out, and the liveness of the store would go unchecked on
  # the strength of a misspelling.
  case "$adapter" in
    none | session-json) ;;
    *) fail "unknown sessionLiveness.adapter '$adapter' (expected none or session-json)" 2 ;;
  esac

  printf '%s' "$adapter"
}

# Newest commit time as a unix epoch, or empty when this is not a git checkout
# (or carries no commit yet). Epoch seconds on BOTH sides of the freshness
# comparison, so no GNU/BSD `date` parsing divergence exists to get wrong.
newest_commit_epoch() {
  local repo_root="$1" epoch
  command -v git > /dev/null 2>&1 || return 0
  epoch="$(git -C "$repo_root" log -1 --format=%ct 2> /dev/null || true)"
  valid_count "$epoch" || return 0
  printf '%s' "$epoch"
}

epoch_to_iso() {
  local epoch="$1"
  [ -n "$epoch" ] || {
    printf 'none'
    return 0
  }
  # jq's `todateiso8601` rather than `date -d` / `date -r`, which spell this
  # differently on GNU and BSD.
  jq -rn --argjson e "$epoch" '$e | todateiso8601' 2> /dev/null || printf 'none'
}

cmd_check() {
  local repo_root="$PWD" session_id='' turns=''

  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --session-id)
        [ "$#" -ge 2 ] || die_usage "--session-id requires a value"
        session_id="$2"
        valid_token "$session_id" || die_usage "invalid --session-id '$session_id' (allowed: A-Z a-z 0-9 . _ : -)"
        shift 2
        ;;
      --turns)
        [ "$#" -ge 2 ] || die_usage "--turns requires a value"
        turns="$2"
        valid_count "$turns" || die_usage "--turns must be a non-negative integer, got '$turns'"
        shift 2
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      --skip* | --force* | --ignore* | --assume*)
        printf '%s: "%s" does not exist. Liveness is established by snapshotting, never by asserting it.\n' "$NAME" "$1" >&2
        exit 2
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done

  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"

  local adapter
  adapter="$(resolve_adapter "$repo_root")"
  printf 'adapter=%s\n' "$adapter"
  if [ "$adapter" = "none" ]; then
    # DEFAULT OFF. Nothing inspected, nothing reported. Note the verdict is
    # SKIPPED and not PASS: "we did not look" is a different claim from
    # "we looked and it was fine".
    printf 'liveness=unmeasured\n'
    printf 'verdict=SKIPPED\n'
    return 0
  fi

  local store="$repo_root/$STORE_REL"
  printf 'store=%s\n' "$store"
  printf 'graceTurns=%s\n' "$GRACE_TURNS"
  if [ -n "$turns" ]; then
    printf 'observedTurns=%s\n' "$turns"
  else
    printf 'observedTurns=unmeasured\n'
  fi
  printf 'sessionId=%s\n' "${session_id:-none}"

  if ! command -v jq > /dev/null 2>&1; then
    printf 'snapshotCount=unmeasured\n'
    printf 'liveness=unmeasured\n'
    printf 'freshness=unmeasured\n'
    printf 'reason=jq-unavailable\n'
    printf 'verdict=UNMEASURED\n'
    return 0
  fi

  local total=0 attributed=0 unattributed=0 sessions=0 mine=0 newest_snap=''
  if [ -f "$store" ] && jq empty "$store" > /dev/null 2>&1; then
    local agg
    agg="$(jq -r --arg sid "$session_id" '
      (.turnSnapshots // []) as $t
      | [ $t[] | select((.hostSessionId // null) != null) | .hostSessionId ] as $ids
      | [ $t[] | (.timestamp // empty) | (try fromdateiso8601) // empty ] as $ts
      | [ ($t | length),
          ($ids | length),
          (($t | length) - ($ids | length)),
          ($ids | unique | length),
          (if $sid == "" then 0 else ($ids | map(select(. == $sid)) | length) end),
          (if ($ts | length) > 0 then ($ts | max) else "" end) ]
      | @tsv
    ' "$store" 2> /dev/null || true)"
    if [ -n "$agg" ]; then
      IFS="$(printf '\t')" read -r total attributed unattributed sessions mine newest_snap <<< "$agg"
    fi
  fi

  printf 'snapshotCount=%s\n' "$total"
  printf 'attributedSnapshots=%s\n' "$attributed"
  printf 'unattributedSnapshots=%s\n' "$unattributed"
  printf 'hostSessionCount=%s\n' "$sessions"
  if [ -n "$session_id" ]; then
    printf 'sessionSnapshotCount=%s\n' "$mine"
  else
    printf 'sessionSnapshotCount=unmeasured\n'
  fi

  # --- freshness: newest snapshot vs newest commit ------------------------
  local commit_epoch freshness
  commit_epoch="$(newest_commit_epoch "$repo_root")"
  printf 'newestSnapshotAt=%s\n' "$(epoch_to_iso "$newest_snap")"
  printf 'newestCommitAt=%s\n' "$(epoch_to_iso "$commit_epoch")"
  if [ -z "$newest_snap" ] || [ -z "$commit_epoch" ]; then
    freshness='unmeasured'
  elif [ "$newest_snap" -lt "$commit_epoch" ]; then
    freshness='stale'
  else
    freshness='fresh'
  fi
  printf 'freshness=%s\n' "$freshness"

  # --- liveness: obligation 1, scoped by obligation 2 ---------------------
  #
  # The observed population is this session's own snapshots when a session id
  # was supplied, and the whole store otherwise. `unattributed` is its own
  # answer rather than a finding: records WERE appended, they simply cannot be
  # proved to be this run's, and calling that silence would be as wrong as
  # calling an empty store a pass.
  local observed liveness
  if [ -n "$session_id" ]; then
    observed="$mine"
  else
    observed="$total"
  fi

  if [ "$observed" -gt 0 ]; then
    liveness='live'
  elif [ -z "$turns" ]; then
    liveness='unmeasured'
  elif [ "$turns" -le "$GRACE_TURNS" ]; then
    liveness='within-grace'
  elif [ "$unattributed" -gt 0 ]; then
    liveness='unattributed'
  else
    liveness='finding'
  fi
  printf 'liveness=%s\n' "$liveness"

  # --- verdict: FINDING > STALE > UNMEASURED > PASS -----------------------
  if [ "$liveness" = 'finding' ]; then
    printf 'verdict=FINDING\n'
    {
      printf '%s: a run of %s turn(s) appended no turnSnapshots[] entry' "$NAME" "$turns"
      if [ -n "$session_id" ]; then
        printf ' for host session %s' "$session_id"
      fi
      printf '\n'
      printf '  store:        %s\n' "$store"
      printf '  grace:        %s turn(s); past that, session state is owed, not optional\n' "$GRACE_TURNS"
      printf '  consequence:  G083, G128 and trajectory-inspector --health read this store; over silence they measure nothing\n'
      printf '  remediation:  call bubbles/scripts/state-snapshot.sh at each phase boundary with --session-id\n'
    } >&2
    return 1
  fi

  if [ "$freshness" = 'stale' ]; then
    printf 'verdict=STALE\n'
    return 0
  fi

  # PASS is reachable ONLY from measured liveness AND measured freshness. This
  # is the whole point: an empty or unmeasurable store falls through to
  # UNMEASURED below and can never claim to be healthy.
  if [ "$liveness" = 'live' ] && [ "$freshness" = 'fresh' ]; then
    printf 'verdict=PASS\n'
    return 0
  fi

  printf 'verdict=UNMEASURED\n'
  return 0
}

cmd_sessions() {
  local repo_root="$PWD"

  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done

  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"

  local adapter
  adapter="$(resolve_adapter "$repo_root")"
  printf 'adapter=%s\n' "$adapter"
  if [ "$adapter" = "none" ]; then
    printf 'verdict=SKIPPED\n'
    return 0
  fi

  local store="$repo_root/$STORE_REL"
  printf 'store=%s\n' "$store"

  if ! command -v jq > /dev/null 2>&1; then
    printf 'verdict=UNMEASURED\n'
    printf 'reason=jq-unavailable\n'
    return 0
  fi

  if [ ! -f "$store" ] || ! jq empty "$store" > /dev/null 2>&1; then
    printf 'hostSessionCount=0\n'
    printf 'verdict=UNMEASURED\n'
    return 0
  fi

  local listing
  listing="$(jq -r '
    (.turnSnapshots // [])
    | map(select((.hostSessionId // null) != null))
    | group_by(.hostSessionId)
    | map("session." + .[0].hostSessionId + ".snapshots=" + (length | tostring))
    | .[]
  ' "$store" 2> /dev/null || true)"

  local count=0 line
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    printf '%s\n' "$line"
    count=$((count + 1))
  done <<< "$listing"

  printf 'hostSessionCount=%s\n' "$count"
  if [ "$count" -eq 0 ]; then
    printf 'verdict=UNMEASURED\n'
  else
    printf 'verdict=PASS\n'
  fi
  return 0
}

[ "$#" -ge 1 ] || die_usage "a subcommand is required (check | sessions)"

SUBCOMMAND="$1"
shift

case "$SUBCOMMAND" in
  check) cmd_check "$@" ;;
  sessions) cmd_sessions "$@" ;;
  -h | --help)
    usage
    exit 0
    ;;
  *) die_usage "unknown subcommand: $SUBCOMMAND (expected check | sessions)" ;;
esac
