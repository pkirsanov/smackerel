#!/usr/bin/env bash
# Hermetic selftest for the abandoned-run reaper in cli.sh.
#
# THE DEFECT THIS DEFENDS AGAINST
#
# recentRuns is capped at 25 by trim_recent_run_lines. activeRuns had no cap and
# no reaper, so any run whose process died before finish_run_state could fire —
# an interrupted framework-validate, a killed release-check, a crashed doctor —
# leaked an entry that stayed "active" forever. The live registry had
# accumulated 25 such entries, the oldest four months old, which makes the
# ledger useless for the one question it exists to answer: what is running now?
#
# A record carries no pid, so liveness cannot be tested directly and age is the
# only available signal. That makes the two cases below the whole contract:
# an old entry must be reclassified, and a RECENT entry must survive — a reaper
# that also killed live runs would be worse than the leak.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRAMEWORK_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$FRAMEWORK_DIR/.." && pwd)"
# bubbles_run_with_timeout resolves timeout -> gtimeout -> a watchdog fallback.
# A bare `timeout` here made the reaper-triggering subshell fail outright on any
# box without GNU coreutils, so nothing was reaped and A1/A3 failed while A2/A4
# passed vacuously on "nothing happened". (IMP-042 SCOPE-12.)
# shellcheck source=bubbles/scripts/guard-lib.sh
. "$SCRIPT_DIR/guard-lib.sh"
# Overridable so the reaper can be mutation-tested against a throwaway copy
# without relocating this script (which would break REPO_ROOT resolution).
CLI="${BUBBLES_CLI_UNDER_TEST:-$SCRIPT_DIR/cli.sh}"

checks=0
failures=0
ok() { checks=$((checks + 1)); echo "  ok   $1"; }
bad() { checks=$((checks + 1)); failures=$((failures + 1)); echo "  FAIL $1"; echo "       $2"; }

command -v jq >/dev/null 2>&1 || { echo "run-state-reaper-selftest: SKIP (jq not installed)"; exit 0; }
command -v git >/dev/null 2>&1 || { echo "run-state-reaper-selftest: SKIP (git not installed)"; exit 0; }
[[ -f "$CLI" ]] || { echo "FAIL: $CLI not found" >&2; exit 1; }

WT="$(mktemp -d)/wt"
cleanup() {
  git -C "$REPO_ROOT" worktree remove "$WT" --force >/dev/null 2>&1 || true
  rm -rf "$(dirname "$WT")"
  git -C "$REPO_ROOT" worktree prune >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

# The reaper only runs for TRACKED (non read_only) commands, and it rewrites a
# real registry, so it is exercised inside a throwaway worktree rather than
# against the repository's own bookkeeping.
if ! git -C "$REPO_ROOT" worktree add --detach "$WT" HEAD >/dev/null 2>&1; then
  echo "run-state-reaper-selftest: SKIP (could not create a worktree)"
  exit 0
fi

# The framework does not sit at the same repo-relative path everywhere: it is
# bubbles/ in the source tree and .github/bubbles/ in an installed downstream,
# and a downstream repository may not have committed its install at all, which
# leaves the worktree without any framework to run. Materialise the LIVE
# framework at its own repo-relative path instead of assuming the checkout
# already carries it -- this is also what makes the selftest exercise the
# working-tree code rather than HEAD.
#
# Both paths are resolved with `pwd -P`. `pwd` reports the LOGICAL path while
# `git rev-parse --show-toplevel` reports the physical one, so under a symlinked
# root -- /tmp and /var on macOS -- the prefix strip below silently fails and
# leaves an ABSOLUTE path. That path still exists, so every command keeps
# working while pointing at the wrong tree, and the reaper then edits a registry
# this selftest never inspects.
FRAMEWORK_TOPLEVEL="$(cd "$(git -C "$REPO_ROOT" rev-parse --show-toplevel 2>/dev/null || printf '%s' "$REPO_ROOT")" && pwd -P)"
FRAMEWORK_PHYSICAL_DIR="$(cd "$FRAMEWORK_DIR" && pwd -P)"
FRAMEWORK_REL="${FRAMEWORK_PHYSICAL_DIR#"$FRAMEWORK_TOPLEVEL"/}"
if [[ "$FRAMEWORK_REL" == /* || "$FRAMEWORK_REL" == "$FRAMEWORK_PHYSICAL_DIR" ]]; then
  echo "FAIL: could not resolve the framework directory relative to the repository root" >&2
  echo "       framework=$FRAMEWORK_PHYSICAL_DIR root=$FRAMEWORK_TOPLEVEL" >&2
  exit 1
fi
mkdir -p "$WT/$FRAMEWORK_REL"
cp -R "$FRAMEWORK_DIR/." "$WT/$FRAMEWORK_REL/"
if [[ -d "$FRAMEWORK_TOPLEVEL/.specify/memory" ]]; then
  mkdir -p "$WT/.specify/memory"
  cp -R "$FRAMEWORK_TOPLEVEL/.specify/memory/." "$WT/.specify/memory/"
fi
cp "$CLI" "$WT/$FRAMEWORK_REL/scripts/cli.sh"
mkdir -p "$WT/.specify/runtime"

iso_ago() { # iso_ago <seconds-ago>
  local secs="$1"
  if date -u -d "@$(( $(date -u +%s) - secs ))" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null; then return 0; fi
  date -u -r "$(( $(date -u +%s) - secs ))" '+%Y-%m-%dT%H:%M:%SZ'
}

record() { # record <runId> <startedAt>
  printf '{"runId":"%s","command":"framework-validate","args":"","sessionId":"s","agent":"cli","repo":"b","branch":"main","worktree":"%s","status":"active","startedAt":"%s","updatedAt":"%s","completedAt":"","result":"pending","durationMs":0,"target":"","runtimeAttachment":"","posture":"fresh","riskClass":"owned_mutation"}' \
    "$1" "$WT" "$2" "$2"
}

seed_registry() { # seed_registry <line1> [line2]
  {
    printf '{\n  "version": 1,\n  "activeRuns": [\n'
    if [[ -n "${2:-}" ]]; then printf '    %s,\n    %s\n' "$1" "$2"; else printf '    %s\n' "$1"; fi
    printf '  ],\n  "recentRuns": [\n  ]\n}\n'
  } > "$WT/.specify/runtime/workflow-runs.json"
}

run_tracked() {
  set +e
  (cd "$WT" && bubbles_run_with_timeout 120 bash "$FRAMEWORK_REL/scripts/cli.sh" upgrade --help >/dev/null 2>&1)
  set -e
}

OLD="$(iso_ago $((120 * 86400)))"
FRESH="$(iso_ago 300)"

# --- P1/A1. an old entry is reaped, a fresh one is not ---------------------
seed_registry "$(record wrn_OLD_leaked "$OLD")" "$(record wrn_FRESH_running "$FRESH")"
run_tracked
REG="$WT/.specify/runtime/workflow-runs.json"

if jq empty "$REG" >/dev/null 2>&1; then
  ok "P1 the registry is still valid JSON after reaping"
else
  bad "P1 registry validity" "the reaper produced malformed JSON"
fi

if [[ "$(jq -r '[.recentRuns[] | select(.runId=="wrn_OLD_leaked" and .result=="abandoned")] | length' "$REG")" == "1" ]]; then
  ok "A1 a four-month-old active run is reclassified as abandoned"
else
  bad "A1 stale run reaped" "$(jq -c '{active:[.activeRuns[].runId],recent:[.recentRuns[]|{id:.runId,r:.result}]}' "$REG")"
fi

# The load-bearing half. Reaping by age is only safe if a LIVE run survives it;
# without this case a reaper that emptied activeRuns outright would pass A1.
if [[ "$(jq -r '[.activeRuns[] | select(.runId=="wrn_FRESH_running")] | length' "$REG")" == "1" ]]; then
  ok "A2 a five-minute-old run is left ACTIVE, not reaped"
else
  bad "A2 live run preserved" "$(jq -c '[.activeRuns[].runId]' "$REG")"
fi

# --- A3. the audit trail is preserved, not deleted -------------------------
if [[ "$(jq -r '[.recentRuns[] | select(.runId=="wrn_OLD_leaked")] | .[0].startedAt' "$REG")" == "$OLD" ]]; then
  ok "A3 the reaped record keeps its original startedAt (moved, not discarded)"
else
  bad "A3 audit trail" "startedAt was rewritten or the record was dropped"
fi

# --- A4. the threshold is honoured ----------------------------------------
# With a huge threshold nothing is old enough, so an unconditional reaper is
# distinguishable from one that actually compares against the cutoff.
seed_registry "$(record wrn_OLD_leaked "$OLD")"
set +e
(cd "$WT" && BUBBLES_RUN_STATE_ABANDON_HOURS=100000 bubbles_run_with_timeout 120 bash "$FRAMEWORK_REL/scripts/cli.sh" upgrade --help >/dev/null 2>&1)
set -e
if [[ "$(jq -r '[.activeRuns[] | select(.runId=="wrn_OLD_leaked")] | length' "$REG")" == "1" ]]; then
  ok "A4 a threshold wider than the entry's age leaves it active"
else
  bad "A4 threshold honoured" "the reaper ignored BUBBLES_RUN_STATE_ABANDON_HOURS"
fi

printf 'run-state-reaper-selftest: %s check(s), %s failure(s)\n' "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
