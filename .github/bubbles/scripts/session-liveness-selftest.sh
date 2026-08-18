#!/usr/bin/env bash
# session-liveness-selftest.sh — IMP-048 SCOPE-7 (WIP-5).
#
# Every assertion runs the SHIPPING script against a real fixture repository and
# reads the verdict it actually printed. The store under test is the same
# `.specify/memory/bubbles.session.json` that `state-snapshot.sh` writes; no
# second store is created here or anywhere else in this scope.
#
# THE ADVERSARIAL CASE, FIRST. A gate reading an EMPTY store must report
# UNMEASURED and specifically NOT pass. Reporting PASS over no data is the exact
# defect this scope exists to correct — it is how three controls (G083, G128,
# trajectory-inspector --health) sat over an empty session file for five days
# and all reported that nothing was wrong. The assertion below therefore checks
# BOTH halves: that UNMEASURED is present AND that PASS is absent. Checking only
# the first would still admit a script that printed both.
#
# Also covered:
#   default off     an unconfigured repository is a clean no-op: verdict SKIPPED,
#                   nothing read, nothing written
#   obligation 1    a run past 3 turns with no snapshot is a FINDING (exit 1);
#                   a run of exactly 3 with no snapshot is NOT (within-grace)
#   obligation 2    two concurrent host session ids in ONE repository each read
#                   back their own trajectory, and BOTH survive
#   obligation 3    a newest snapshot predating the newest commit is STALE
#   unattributed    snapshots with no hostSessionId are their own answer, never
#                   silently credited to, or debited from, another session
#   read-only       the session file is byte-identical after every check
#   no bypass       --skip / --force are refused
#
# Hermetic: fixtures live under mktemp and are removed on exit.
#
# Exit codes:
#   0 = all assertions passed
#   1 = at least one assertion failed
#   2 = the script under test is unavailable

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SUT="$SCRIPT_DIR/session-liveness.sh"
NAME="session-liveness-selftest"
STORE_REL=".specify/memory/bubbles.session.json"

passes=0
failures=0
pass() {
  passes=$((passes + 1))
  printf 'PASS: %s\n' "$1"
}
fail() {
  failures=$((failures + 1))
  printf 'FAIL: %s\n' "$1"
}

[[ -f "$SUT" ]] || {
  printf '%s: script under test not found: %s\n' "$NAME" "$SUT" >&2
  exit 2
}

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-session-liveness.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

# --- fixture helpers -------------------------------------------------------

repo_seq=0
REPO=""
# Sets the global REPO. NOT called through a command substitution: that would
# run in a subshell, repo_seq would never advance, and every "fresh" fixture
# would silently be the same directory carrying the previous case's records.
new_repo() {
  local adapter="${1:-session-json}"
  repo_seq=$((repo_seq + 1))
  REPO="$TMP_DIR/repo$repo_seq"
  mkdir -p "$REPO/.github" "$REPO/.specify/memory"
  if [[ "$adapter" != "unset" ]]; then
    printf 'sessionLiveness:\n  adapter: %s\n' "$adapter" > "$REPO/.github/bubbles-project.yaml"
  fi
}

write_store() {
  printf '%s\n' "$1" > "$REPO/$STORE_REL"
}

# A git checkout with ONE commit at a fixed instant, so freshness is decided by
# arithmetic rather than by how long the selftest took to run.
seed_commit() {
  local when="$1"
  git -C "$REPO" init -q > /dev/null 2>&1 || return 1
  GIT_AUTHOR_DATE="$when" GIT_COMMITTER_DATE="$when" \
    git -C "$REPO" \
    -c user.email=selftest@bubbles.invalid \
    -c user.name=selftest \
    -c commit.gpgsign=false \
    commit -q --allow-empty -m "fixture" > /dev/null 2>&1 || return 1
  return 0
}

LAST_OUT=""
LAST_RC=0
run_sut() {
  local sub="$1"
  shift
  LAST_OUT="$(bash "$SUT" "$sub" --repo-root "$REPO" "$@" 2>&1)"
  LAST_RC=$?
}

field() {
  awk -F= -v k="$1" '$1 == k { print $2; exit }' <<< "$LAST_OUT"
}

assert_field() {
  local key="$1" expected="$2" label="$3" actual
  actual="$(field "$key")"
  if [[ "$actual" == "$expected" ]]; then
    pass "$label ($key=$expected)"
  else
    fail "$label: expected $key=$expected, got '${actual:-<absent>}'"
    printf '  --- output ---\n%s\n' "$LAST_OUT"
  fi
}

assert_rc() {
  local expected="$1" label="$2"
  if [[ "$LAST_RC" == "$expected" ]]; then
    pass "$label (exit $expected)"
  else
    fail "$label: expected exit $expected, got $LAST_RC"
    printf '  --- output ---\n%s\n' "$LAST_OUT"
  fi
}

assert_lacks() {
  local needle="$1" label="$2"
  if grep -Fq -- "$needle" <<< "$LAST_OUT"; then
    fail "$label: output unexpectedly contained '$needle'"
    printf '  --- output ---\n%s\n' "$LAST_OUT"
  else
    pass "$label"
  fi
}

# ===========================================================================
# ADVERSARIAL: an EMPTY store reports UNMEASURED, and specifically NOT pass
# ===========================================================================

new_repo session-json
run_sut check
assert_rc 0 "empty store is not a failure"
assert_field verdict UNMEASURED "empty store reports UNMEASURED"
assert_lacks "verdict=PASS" "empty store NEVER reports PASS (the exact defect)"
assert_field snapshotCount 0 "empty store counts zero snapshots"
assert_field liveness unmeasured "empty store with unknown turn count is unmeasured"

# A store that EXISTS but holds an empty turnSnapshots array is the same claim
# wearing a file. It must land in the same place.
new_repo session-json
write_store '{ "sessionId": "s-1", "turnSnapshots": [] }'
run_sut check
assert_rc 0 "present-but-empty store is not a failure"
assert_field verdict UNMEASURED "present-but-empty store reports UNMEASURED"
assert_lacks "verdict=PASS" "present-but-empty store NEVER reports PASS"

# ===========================================================================
# Default OFF: an unconfigured repository is a clean no-op
# ===========================================================================

new_repo unset
run_sut check --turns 40
assert_rc 0 "unconfigured repo exits 0"
assert_field adapter none "unconfigured repo resolves adapter none"
assert_field verdict SKIPPED "unconfigured repo reports SKIPPED"
assert_lacks "verdict=PASS" "SKIPPED is not PASS — 'we did not look' is a different claim"
assert_lacks "verdict=FINDING" "an unconfigured repo is never newly failed"

new_repo none
run_sut check --turns 40
assert_field verdict SKIPPED "explicit adapter none reports SKIPPED"

new_repo mysterious
run_sut check
assert_rc 2 "a configured-but-unknown adapter fails loud rather than degrading to none"

# ===========================================================================
# Obligation 1: silence past the grace window is a FINDING
# ===========================================================================

new_repo session-json
write_store '{ "sessionId": "s-1", "turnSnapshots": [] }'
run_sut check --session-id host-a --turns 4
assert_rc 1 "a 4-turn run with no snapshot exits 1"
assert_field verdict FINDING "a run past the grace window with no snapshot is a FINDING"
assert_field liveness finding "liveness names the finding"
assert_field observedTurns 4 "the finding names the observed turn count"

new_repo session-json
write_store '{ "sessionId": "s-1", "turnSnapshots": [] }'
run_sut check --session-id host-a --turns 3
assert_rc 0 "a 3-turn run with no snapshot does NOT fail"
assert_field liveness within-grace "a run at the grace boundary is within grace, not a finding"
assert_lacks "verdict=FINDING" "a short run is never a finding"
assert_lacks "verdict=PASS" "a short run over an empty store is still not a pass"

# Snapshots exist but carry no hostSessionId. Records WERE appended; they simply
# cannot be proved to be this run's. Calling that silence would be as wrong as
# calling an empty store a pass.
new_repo session-json
write_store '{
  "turnSnapshots": [
    { "turnNumber": 1, "timestamp": "2026-06-01T10:00:00Z", "phase": "implement" },
    { "turnNumber": 2, "timestamp": "2026-06-01T11:00:00Z", "phase": "test" }
  ]
}'
run_sut check --session-id host-a --turns 9
assert_rc 0 "unattributed snapshots are not a finding"
assert_field liveness unattributed "unattributed snapshots are their own answer"
assert_field unattributedSnapshots 2 "unattributed snapshots are counted, not discarded"
assert_lacks "verdict=PASS" "unattributed snapshots do not earn a pass either"

# ===========================================================================
# Obligation 2: two concurrent host session ids do not overwrite each other
# ===========================================================================

new_repo session-json
write_store '{
  "turnSnapshots": [
    { "turnNumber": 1, "timestamp": "2026-06-01T10:00:00Z", "phase": "implement", "hostSessionId": "host-a" },
    { "turnNumber": 2, "timestamp": "2026-06-01T10:05:00Z", "phase": "implement", "hostSessionId": "host-b" },
    { "turnNumber": 3, "timestamp": "2026-06-01T10:10:00Z", "phase": "test",      "hostSessionId": "host-a" },
    { "turnNumber": 4, "timestamp": "2026-06-01T10:15:00Z", "phase": "test",      "hostSessionId": "host-b" },
    { "turnNumber": 5, "timestamp": "2026-06-01T10:20:00Z", "phase": "validate",  "hostSessionId": "host-b" }
  ]
}'
CONCURRENT_REPO="$REPO"
STORE_BEFORE="$(cat "$REPO/$STORE_REL")"

run_sut check --session-id host-a --turns 12
assert_rc 0 "host-a sees its own trajectory"
assert_field sessionSnapshotCount 2 "host-a reads back exactly its own 2 snapshots"
assert_field snapshotCount 5 "the shared store still holds all 5 records"
assert_field hostSessionCount 2 "both concurrent sessions are visible"
assert_lacks "verdict=FINDING" "host-a has a trajectory, so it is not a finding"

run_sut check --session-id host-b --turns 12
assert_rc 0 "host-b sees its own trajectory"
assert_field sessionSnapshotCount 3 "host-b reads back exactly its own 3 snapshots"
assert_field snapshotCount 5 "host-b's read did not truncate host-a's records"

# A third, unrelated session in the SAME repository is a finding for ITSELF
# without disturbing either incumbent — which is the whole point of keying.
run_sut check --session-id host-c --turns 12
assert_rc 1 "a third session with no snapshots is a finding for itself"
assert_field snapshotCount 5 "the other two sessions' records survive the third's finding"

run_sut sessions
assert_rc 0 "sessions subcommand exits 0"
if grep -Fq 'session.host-a.snapshots=2' <<< "$LAST_OUT" &&
  grep -Fq 'session.host-b.snapshots=3' <<< "$LAST_OUT"; then
  pass "both concurrent trajectories survive and are separately readable"
else
  fail "sessions did not report both trajectories: $LAST_OUT"
fi

STORE_AFTER="$(cat "$CONCURRENT_REPO/$STORE_REL")"
if [[ "$STORE_BEFORE" == "$STORE_AFTER" ]]; then
  pass "the session store is byte-identical after every read (liveness is read-only)"
else
  fail "the session store was modified by a read"
fi

# ===========================================================================
# Obligation 3: a snapshot older than the newest commit is STALE
# ===========================================================================

new_repo session-json
if seed_commit "2026-06-10T00:00:00Z"; then
  write_store '{
    "turnSnapshots": [
      { "turnNumber": 1, "timestamp": "2026-06-01T00:00:00Z", "phase": "implement", "hostSessionId": "host-a" }
    ]
  }'
  run_sut check --session-id host-a --turns 8
  assert_rc 0 "a stale session file is advisory, not a failure"
  assert_field freshness stale "a snapshot predating the newest commit is stale"
  assert_field verdict STALE "the verdict surfaces staleness for doctor"
  assert_lacks "verdict=PASS" "a stale session file never reports PASS"
else
  fail "could not seed a git fixture for the staleness case"
fi

new_repo session-json
if seed_commit "2026-06-01T00:00:00Z"; then
  write_store '{
    "turnSnapshots": [
      { "turnNumber": 1, "timestamp": "2026-06-10T00:00:00Z", "phase": "implement", "hostSessionId": "host-a" }
    ]
  }'
  run_sut check --session-id host-a --turns 8
  assert_rc 0 "a live and fresh session file exits 0"
  assert_field freshness fresh "a snapshot after the newest commit is fresh"
  assert_field liveness live "a session with its own snapshots is live"
  assert_field verdict PASS "PASS is reachable ONLY from measured liveness AND measured freshness"
else
  fail "could not seed a git fixture for the fresh case"
fi

# Measured snapshots but NO commit to compare against: freshness is unmeasured,
# so the verdict falls back to UNMEASURED rather than claiming a pass on half
# the evidence.
new_repo session-json
write_store '{
  "turnSnapshots": [
    { "turnNumber": 1, "timestamp": "2026-06-10T00:00:00Z", "phase": "implement", "hostSessionId": "host-a" }
  ]
}'
run_sut check --session-id host-a --turns 8
assert_field liveness live "a non-git fixture can still measure liveness"
assert_field freshness unmeasured "freshness with no commit to compare is unmeasured"
assert_field verdict UNMEASURED "half-measured evidence does not earn a PASS"

# ===========================================================================
# No bypass
# ===========================================================================

new_repo session-json
run_sut check --skip
assert_rc 2 "--skip is refused"
new_repo session-json
run_sut check --force
assert_rc 2 "--force is refused"
new_repo session-json
run_sut check --nonsense
assert_rc 2 "an unknown flag is refused"

# ===========================================================================
# Verdict
# ===========================================================================

printf '\n%s: %d passed, %d failed\n' "$NAME" "$passes" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
