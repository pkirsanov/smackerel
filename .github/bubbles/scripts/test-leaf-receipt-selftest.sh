#!/usr/bin/env bash
# test-leaf-receipt-selftest.sh — IMP-048 SCOPE-3 (PERF-9).
#
# Every assertion runs the SHIPPING script against a real fixture repository,
# with real files whose bytes really change, and reads the outcome back out of
# the JSONL store the script actually wrote. Crucially, "was NOT re-run" is
# proved by a SIDE EFFECT the leaf leaves behind -- a marker file it appends to
# every time it executes -- rather than by the script's own report. A no-replay
# claim checked only against the reporter that makes the claim proves nothing.
#
# Covered:
#   default off        an unconfigured repository is a clean no-op for RECEIPTS
#                      -- zero records, no .specify directory, exit 0 -- while
#                      still EXECUTING every leaf, because "unconfigured" must
#                      never silently mean "the tests did not run"
#   no replay          a passing leaf on an identical candidateDigest AND
#                      environmentFingerprint is ACCEPTED and its command does
#                      not execute a second time
#   candidate change   changing the leaf's own candidate material invalidates
#                      the receipt and forces a re-run
#   environment change changing ONLY the environment fingerprint, with every
#                      byte identical, also forces a re-run
#   precise invalidation  a changed production owner re-runs the leaves whose
#                      refs cover it and leaves a NON-covering sibling ACCEPTED.
#                      That sibling assertion is the precision claim; without it
#                      "invalidate everything" would pass too
#   unresolved         a timed-out leaf is UNRESOLVED, and asserting it as a
#                      pass OR as RED evidence is refused
#   resume             a second run starts at the first UNRESOLVED leaf, with
#                      the passing leaves before and after it left untouched
#   stated reruns      a second aggregate on the same digest is refused without
#                      --rerun-reason and permitted with one
#   earned receipts    a receipt forged into the store -- absent output hash, or
#                      a candidate digest that matches nothing on disk -- is
#                      never honoured as acceptance and cannot back a claim
#
# Hermetic: fixtures live under mktemp and are removed on exit.
#
# Exit codes:
#   0 = all assertions passed
#   1 = at least one assertion failed
#   2 = the script under test is unavailable

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SUT="$SCRIPT_DIR/test-leaf-receipt.sh"
NAME="test-leaf-receipt-selftest"
STORE_REL=".specify/runtime/test-leaf-receipts.jsonl"

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

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-test-leaf-receipt.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

# --- fixture helpers -------------------------------------------------------

repo_seq=0
REPO=""
# Sets the global REPO. NOT called through a command substitution: that would
# run in a subshell, repo_seq would never advance, and every "fresh" fixture
# would silently be the same directory carrying the previous test's records.
new_repo() {
  local adapter="${1:-jsonl}"
  repo_seq=$((repo_seq + 1))
  REPO="$TMP_DIR/repo$repo_seq"
  mkdir -p "$REPO/.github" "$REPO/src"
  printf 'owner alpha v1\n' > "$REPO/src/a.txt"
  printf 'owner beta v1\n' > "$REPO/src/b.txt"
  if [[ "$adapter" != "unset" ]]; then
    printf 'testLeafReceipts:\n  adapter: %s\n' "$adapter" > "$REPO/.github/bubbles-project.yaml"
  fi
}

LAST_OUT=""
LAST_RC=0
run_sut() {
  local sub="$1" repo="$2"
  shift 2
  LAST_OUT="$(bash "$SUT" "$sub" --repo-root "$repo" "$@" 2>&1)"
  LAST_RC=$?
}

# Read one key=value line out of the captured output. A herestring, never a
# discarding pipe: `... | grep -q` on unbounded input under pipefail is the
# BUG-009 SIGPIPE race.
kv() {
  awk -v k="$1" -F= '$1 == k { print substr($0, length(k) + 2); exit }' <<< "$LAST_OUT"
}

store_path() { printf '%s/%s' "$1" "$STORE_REL"; }

store_count() {
  local f
  f="$(store_path "$1")"
  if [[ -f "$f" ]]; then
    awk 'END { print NR + 0 }' "$f"
  else
    printf '0'
  fi
}

# How many times a leaf ACTUALLY executed, counted from the marker file the leaf
# command itself appends to. This is the independent witness: it is written by
# the leaf, not by the script that reports on the leaf.
executions() {
  local f="$REPO/ran-$1"
  if [[ -f "$f" ]]; then
    awk 'END { print NR + 0 }' "$f"
  else
    printf '0'
  fi
}

# A leaf command whose only effect is to append one line to its own marker file.
# It writes nothing to stdout, so it cannot collide with the key=value report.
marker_cmd() { printf 'printf "ran\\n" >> %s/ran-%s' "$REPO" "$1"; }

assert_eq() {
  if [[ "$2" == "$3" ]]; then
    pass "$1"
  else
    fail "$1 (expected '$3', got '$2')"
  fi
}

# ---------------------------------------------------------------------------
# DEFAULT OFF. An unconfigured repository records nothing and enforces nothing.
# It still RUNS every leaf: a receipt capability that quietly stopped executing
# tests when unconfigured would be far worse than the waste it replaces.
# ---------------------------------------------------------------------------
new_repo unset
run_sut run "$REPO" --leaf "unit-a=$(marker_cmd a)"
assert_eq "unconfigured repo: run exits 0" "$LAST_RC" "0"
assert_eq "unconfigured repo: adapter is none" "$(kv adapter)" "none"
assert_eq "unconfigured repo: receipts skipped" "$(kv receipts)" "skipped"
assert_eq "unconfigured repo: zero records written" "$(store_count "$REPO")" "0"
assert_eq "unconfigured repo: the leaf STILL executed" "$(executions a)" "1"
if [[ -e "$REPO/.specify" ]]; then
  fail "unconfigured repo: no runtime directory is created"
else
  pass "unconfigured repo: no runtime directory is created"
fi

# An explicit opt-out is the same no-op as never configuring the capability.
new_repo none
run_sut run "$REPO" --leaf "unit-a=$(marker_cmd a)"
assert_eq "explicit adapter=none: run exits 0" "$LAST_RC" "0"
assert_eq "explicit adapter=none: zero records written" "$(store_count "$REPO")" "0"

# A configured-but-unknown adapter fails LOUD rather than degrading to none: a
# typo must not silently disable the control.
new_repo bogus
run_sut resolve "$REPO"
assert_eq "unknown adapter fails loud" "$LAST_RC" "1"

# ---------------------------------------------------------------------------
# NO REPLAY. Identical candidateDigest AND environmentFingerprint means the leaf
# is ACCEPTED and does not run again.
# ---------------------------------------------------------------------------
new_repo jsonl
run_sut run "$REPO" --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt'
assert_eq "no replay: first run executes and passes" "$(kv leaf.1.outcome)" "RAN_PASS"
assert_eq "no replay: first run really ran the leaf" "$(executions a)" "1"
assert_eq "no replay: first run exits 0" "$LAST_RC" "0"
FIRST_CAND="$(kv leaf.1.candidateDigest)"
FIRST_ENV="$(kv environmentFingerprint)"

run_sut run "$REPO" --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt'
assert_eq "no replay: second run reports ACCEPTED" "$(kv leaf.1.outcome)" "ACCEPTED"
assert_eq "no replay: second run reports it was not replayed" "$(kv leaf.1.replayed)" "false"
assert_eq "no replay: the command did NOT execute again" "$(executions a)" "1"
assert_eq "no replay: the digest is unchanged" "$(kv leaf.1.candidateDigest)" "$FIRST_CAND"
assert_eq "no replay: the environment fingerprint is unchanged too" "$(kv environmentFingerprint)" "$FIRST_ENV"
assert_eq "no replay: ACCEPTED is reported, never recorded a second time" "$(store_count "$REPO")" "1"
assert_eq "no replay: a fully accepted run exits 0" "$LAST_RC" "0"

# ---------------------------------------------------------------------------
# CANDIDATE CHANGE. A different candidate digest is a different thing, so the
# prior receipt covers nothing and the leaf runs again.
# ---------------------------------------------------------------------------
run_sut run "$REPO" --leaf "unit-a=$(marker_cmd a) # v2" --leaf-ref 'unit-a=src/a.txt'
assert_eq "candidate change: the digest changed" \
  "$([[ "$(kv leaf.1.candidateDigest)" == "$FIRST_CAND" ]] && printf 'same' || printf 'different')" "different"
assert_eq "candidate change: the leaf re-ran" "$(kv leaf.1.outcome)" "RAN_PASS"
assert_eq "candidate change: the command executed a second time" "$(executions a)" "2"

# ---------------------------------------------------------------------------
# ENVIRONMENT CHANGE. Every byte on disk is identical and the command is
# identical; only the declared environment differs. Acceptance is keyed on the
# fingerprint too, so this still forces a re-run.
# ---------------------------------------------------------------------------
new_repo jsonl
run_sut run "$REPO" --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt' --env 'TOOLCHAIN=1.70'
assert_eq "environment change: baseline run passes" "$(kv leaf.1.outcome)" "RAN_PASS"
BASE_CAND="$(kv leaf.1.candidateDigest)"
BASE_ENV="$(kv environmentFingerprint)"

run_sut run "$REPO" --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt' --env 'TOOLCHAIN=1.70'
assert_eq "environment change: same env is still ACCEPTED" "$(kv leaf.1.outcome)" "ACCEPTED"
assert_eq "environment change: same env did not re-run" "$(executions a)" "1"

run_sut run "$REPO" --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt' --env 'TOOLCHAIN=1.83'
assert_eq "environment change: the candidate digest is UNCHANGED" "$(kv leaf.1.candidateDigest)" "$BASE_CAND"
assert_eq "environment change: the fingerprint changed" \
  "$([[ "$(kv environmentFingerprint)" == "$BASE_ENV" ]] && printf 'same' || printf 'different')" "different"
assert_eq "environment change: a new environment forces a re-run" "$(kv leaf.1.outcome)" "RAN_PASS"
assert_eq "environment change: the command executed again" "$(executions a)" "2"

# ---------------------------------------------------------------------------
# PRECISE INVALIDATION. Touch ONE production owner. The covering leaf re-runs;
# the sibling covering an untouched owner stays ACCEPTED. The sibling assertion
# is the whole claim: invalidating everything would satisfy the first half.
# ---------------------------------------------------------------------------
new_repo jsonl
run_sut run "$REPO" \
  --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt' \
  --leaf "unit-b=$(marker_cmd b)" --leaf-ref 'unit-b=src/b.txt'
assert_eq "precision: both leaves pass on the first run" \
  "$(kv leaf.1.outcome)/$(kv leaf.2.outcome)" "RAN_PASS/RAN_PASS"
assert_eq "precision: both leaves really ran" "$(executions a)/$(executions b)" "1/1"

printf 'owner alpha v2\n' > "$REPO/src/a.txt"
run_sut run "$REPO" \
  --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt' \
  --leaf "unit-b=$(marker_cmd b)" --leaf-ref 'unit-b=src/b.txt'
assert_eq "precision: the leaf COVERING the changed owner re-ran" "$(kv leaf.1.outcome)" "RAN_PASS"
assert_eq "precision: the covering leaf executed a second time" "$(executions a)" "2"
assert_eq "precision: the NON-covering sibling stayed ACCEPTED" "$(kv leaf.2.outcome)" "ACCEPTED"
assert_eq "precision: the non-covering sibling did NOT re-execute" "$(executions b)" "1"
assert_eq "precision: resume points at the invalidated leaf" "$(kv resumedAt)" "unit-a#1"

# A declared owner that does not exist cannot be digested, so the receipt cannot
# be earned. Digesting an absent file as "" would make a deleted owner look
# exactly like an unchanged one.
run_sut run "$REPO" --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/gone.txt'
assert_eq "precision: an absent declared owner is refused" "$LAST_RC" "2"

# ---------------------------------------------------------------------------
# UNRESOLVED + RESUME. Leaf b times out. It is neither a pass nor a failure, it
# cannot back either claim, and the next run starts AT b -- not at the front.
# The command for b is byte-identical across both runs, so the resume is driven
# by the recorded outcome rather than by a changed digest.
# ---------------------------------------------------------------------------
new_repo jsonl
SLOW_CMD="[ -f $REPO/unblock ] || sleep 20"
run_sut run "$REPO" --timeout 1 \
  --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt' \
  --leaf "unit-b=$SLOW_CMD" --leaf-ref 'unit-b=src/b.txt' \
  --leaf "unit-c=$(marker_cmd c)" --leaf-ref 'unit-c=src/b.txt' \
  --aggregate 'full=true'
assert_eq "unresolved: the timed-out leaf is UNRESOLVED" "$(kv leaf.2.outcome)" "UNRESOLVED"
assert_eq "unresolved: it is not reported as a pass" \
  "$([[ "$(kv leaf.2.outcome)" == "RAN_PASS" ]] && printf 'pass' || printf 'not-pass')" "not-pass"
assert_eq "unresolved: it is not reported as a failure" \
  "$([[ "$(kv leaf.2.outcome)" == "RAN_FAIL" ]] && printf 'fail' || printf 'not-fail')" "not-fail"
assert_eq "unresolved: independent siblings still executed" "$(executions a)/$(executions c)" "1/1"
assert_eq "unresolved: the aggregate is held back, not run" "$(kv aggregate.outcome)" "BLOCKED_NOT_RUN"
assert_eq "unresolved: the run reports work outstanding" "$LAST_RC" "1"

run_sut assert "$REPO" --test-id unit-b --as pass
assert_eq "unresolved: claiming it PASSED is refused" "$LAST_RC" "2"
run_sut assert "$REPO" --test-id unit-b --as red-evidence
assert_eq "unresolved: claiming it is RED evidence is refused" "$LAST_RC" "2"
run_sut assert "$REPO" --test-id unit-a --as pass
assert_eq "unresolved: a genuinely passing sibling still supports a pass" "$LAST_RC" "0"

printf 'go\n' > "$REPO/unblock"
run_sut run "$REPO" --timeout 30 \
  --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt' \
  --leaf "unit-b=$SLOW_CMD" --leaf-ref 'unit-b=src/b.txt' \
  --leaf "unit-c=$(marker_cmd c)" --leaf-ref 'unit-c=src/b.txt' \
  --aggregate 'full=true'
assert_eq "resume: the run resumes AT the unresolved leaf, not at the beginning" \
  "$(kv resumedAt)" "unit-b#1"
assert_eq "resume: the leaf BEFORE the unresolved one is ACCEPTED" "$(kv leaf.1.outcome)" "ACCEPTED"
assert_eq "resume: the leaf before it did NOT re-execute" "$(executions a)" "1"
assert_eq "resume: the previously unresolved leaf now passes" "$(kv leaf.2.outcome)" "RAN_PASS"
assert_eq "resume: the leaf AFTER the unresolved one is ACCEPTED" "$(kv leaf.3.outcome)" "ACCEPTED"
assert_eq "resume: the leaf after it did NOT re-execute" "$(executions c)" "1"
assert_eq "resume: the aggregate runs once every leaf is resolved" "$(kv aggregate.outcome)" "RAN_PASS"
assert_eq "resume: the completed run exits 0" "$LAST_RC" "0"

# ---------------------------------------------------------------------------
# STATED RERUNS. The aggregate is not banned from running twice; the reason has
# to be stated. A refusal writes nothing, so the store is unchanged.
# ---------------------------------------------------------------------------
BEFORE="$(store_count "$REPO")"
run_sut run "$REPO" --timeout 30 \
  --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt' \
  --leaf "unit-b=$SLOW_CMD" --leaf-ref 'unit-b=src/b.txt' \
  --leaf "unit-c=$(marker_cmd c)" --leaf-ref 'unit-c=src/b.txt' \
  --aggregate 'full=true'
assert_eq "stated reruns: a second aggregate on the same digest is refused" "$LAST_RC" "2"
assert_eq "stated reruns: the refusal wrote nothing" "$(store_count "$REPO")" "$BEFORE"

run_sut run "$REPO" --timeout 30 \
  --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt' \
  --leaf "unit-b=$SLOW_CMD" --leaf-ref 'unit-b=src/b.txt' \
  --leaf "unit-c=$(marker_cmd c)" --leaf-ref 'unit-c=src/b.txt' \
  --aggregate 'full=true' --rerun-reason integration-order
assert_eq "stated reruns: WITH a stated reason it is permitted" "$LAST_RC" "0"
assert_eq "stated reruns: the aggregate really ran again" "$(kv aggregate.outcome)" "RAN_PASS"
assert_eq "stated reruns: the reason is reported" "$(kv aggregate.rerunReason)" "integration-order"
assert_eq "stated reruns: the reason is recorded in the store" \
  "$(awk '/"rerunReason":"integration-order"/ { n++ } END { print n + 0 }' "$(store_path "$REPO")")" "1"

# ---------------------------------------------------------------------------
# ADVERSARIAL. A receipt is EARNED by running the leaf. A record forged into the
# append-only store must not be able to buy acceptance, because the store is the
# one artifact a caller can reach without executing anything.
# ---------------------------------------------------------------------------
new_repo jsonl
run_sut run "$REPO" --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt'
GOOD_CAND="$(kv leaf.1.candidateDigest)"
GOOD_ENV="$(kv environmentFingerprint)"
assert_eq "adversarial: baseline leaf ran once" "$(executions a)" "1"

# Forge a receipt for a leaf that never executed: everything looks right except
# the output hash, which only a real execution can produce.
FORGED_STORE="$(store_path "$REPO")"
printf '{"schemaVersion":"test-leaf-receipt/v1","testOccurrenceId":"forged#1","testId":"forged","kind":"leaf","candidateDigest":"%s","inputPathDigests":[],"environmentFingerprint":"%s","timeout":0,"startedAt":"x","finishedAt":"x","exitCode":0,"outputHash":"","outcome":"RAN_PASS","rerunReason":null}\n' \
  "$GOOD_CAND" "$GOOD_ENV" >> "$FORGED_STORE"
run_sut assert "$REPO" --test-id forged --as pass
assert_eq "adversarial: a receipt with no output hash cannot back a pass" "$LAST_RC" "2"
run_sut assert "$REPO" --test-id forged --as red-evidence
assert_eq "adversarial: it cannot back RED evidence either" "$LAST_RC" "2"

run_sut run "$REPO" --leaf "forged=$(marker_cmd forged)" --leaf-ref 'forged=src/a.txt'
assert_eq "adversarial: the forged receipt does not buy acceptance" "$(kv leaf.1.outcome)" "RAN_PASS"
assert_eq "adversarial: the leaf was executed for real" "$(executions forged)" "1"

# A receipt whose candidate digest matches nothing on disk is equally inert: it
# is a claim about bytes that were never under test.
printf '{"schemaVersion":"test-leaf-receipt/v1","testOccurrenceId":"unit-a#1","testId":"unit-a","kind":"leaf","candidateDigest":"deadbeef","inputPathDigests":[],"environmentFingerprint":"%s","timeout":0,"startedAt":"x","finishedAt":"x","exitCode":0,"outputHash":"deadbeef","outcome":"RAN_PASS","rerunReason":null}\n' \
  "$GOOD_ENV" >> "$FORGED_STORE"
run_sut run "$REPO" --leaf "unit-a=$(marker_cmd a)" --leaf-ref 'unit-a=src/a.txt'
assert_eq "adversarial: a mismatched candidate digest forces a real run" "$(kv leaf.1.outcome)" "RAN_PASS"
assert_eq "adversarial: the leaf executed again rather than being accepted" "$(executions a)" "2"

# A claim about a leaf with no receipt at all is an assertion, not evidence.
run_sut assert "$REPO" --test-id never-ran --as pass
assert_eq "adversarial: a claim with no receipt at all is refused" "$LAST_RC" "2"

# No bypass exists.
run_sut run "$REPO" --leaf "unit-a=true" --force
assert_eq "adversarial: a bypass flag is rejected by name" "$LAST_RC" "2"
run_sut run "$REPO" --leaf "unit-a=true" --replay-accepted
assert_eq "adversarial: --replay-accepted is rejected by name" "$LAST_RC" "2"

# ---------------------------------------------------------------------------
printf '\n%s: %d passed, %d failed\n' "$NAME" "$passes" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
