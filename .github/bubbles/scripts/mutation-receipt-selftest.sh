#!/usr/bin/env bash
# mutation-receipt-selftest.sh — IMP-048 SCOPE-4 (EV-11).
#
# Every assertion runs the SHIPPING script against a real fixture repository
# with a real source file, a real test that really fails when that source is
# wrong, and a real mutate command that really changes bytes. The kill is read
# back out of the JSONL store the script actually wrote.
#
# The load-bearing cases are the three that separate an EXECUTED control from a
# DECLARED one:
#   A2  a declared mutation with no receipt is a finding
#   A6  a receipt asserted into the store without an execution is refused
#   A7  a mutation aimed at the shared working tree is refused BEFORE it runs
# P3 is their necessary guard: `negativeControlFallbackReason` must keep working,
# because a repository with no mutation tooling that could no longer declare
# riskTier would simply stop declaring risk, which removes the signal instead of
# strengthening it (IMP-048 R4).
#
# ISOLATION IS ASSERTED INDEPENDENTLY. P2 re-reads the repository source after a
# successful run and compares its digest to the one taken before. That witness is
# outside the script under test: a restoration claim checked only against the
# reporter that makes the claim proves nothing.
#
# Hermetic: fixtures live under mktemp and are removed on exit.
#
# Exit codes:
#   0 = all assertions passed
#   1 = at least one assertion failed
#   2 = the script under test is unavailable

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SUT="$SCRIPT_DIR/mutation-receipt.sh"
NAME="mutation-receipt-selftest"
STORE_REL=".specify/runtime/mutation-receipts.jsonl"

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
assert_eq() {
  if [[ "$2" == "$3" ]]; then
    pass "$1"
  else
    fail "$1 (expected '$3', got '$2')"
  fi
}
assert_says() {
  if [[ "$LAST_OUT" == *"$2"* ]]; then
    pass "$1"
  else
    fail "$1 (output did not name '$2')"
  fi
}

[[ -f "$SUT" ]] || {
  printf '%s: script under test not found: %s\n' "$NAME" "$SUT" >&2
  exit 2
}

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-mutation-receipt.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

sha() {
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# --- fixture ---------------------------------------------------------------

repo_seq=0
REPO=""
# Sets the global REPO. NOT called through a command substitution: that would
# run in a subshell, repo_seq would never advance, and every "fresh" fixture
# would silently be the same directory carrying the previous test's records.
new_repo() {
  local adapter="${1:-command}"
  repo_seq=$((repo_seq + 1))
  REPO="$TMP_DIR/repo$repo_seq"
  mkdir -p "$REPO/.github" "$REPO/src" "$REPO/tests" "$REPO/scripts" "$REPO/specs/feature"

  # The production owner. `add` is the predicate the high-risk scenario claims
  # its test is sensitive to.
  printf 'add() { echo $(( $1 + $2 )); }\n' > "$REPO/src/calc.sh"

  # A test that REALLY fails when that predicate is wrong.
  {
    printf '#!/usr/bin/env bash\n'
    printf '. ./src/calc.sh\n'
    printf 'got="$(add 2 3)"\n'
    printf 'if [ "$got" != "5" ]; then\n'
    printf '  echo "FAIL: add 2 3 expected 5 got $got"\n'
    printf '  exit 1\n'
    printf 'fi\n'
    printf 'echo ok\n'
  } > "$REPO/tests/calc-test.sh"

  # The project-owned mutation runner. Its existence and executability are what
  # mutation-resolve.sh validates; this selftest drives the isolated run through
  # mutation-receipt.sh directly.
  printf '#!/usr/bin/env bash\nexit 0\n' > "$REPO/scripts/mut"
  chmod +x "$REPO/scripts/mut"

  if [[ "$adapter" == "command" ]]; then
    printf 'mutationExecution:\n  adapter: command\n  command: scripts/mut\n' \
      > "$REPO/.github/bubbles-project.yaml"
  elif [[ "$adapter" == "none" ]]; then
    printf 'mutationExecution:\n  adapter: none\n' > "$REPO/.github/bubbles-project.yaml"
  fi
}

# A manifest whose single scenario is riskTier high with a mutation control.
# `extra` injects the fallback reason for the escape case.
write_manifest() {
  local extra="${1:-}"
  printf '%s\n' "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"addition is correct\",\"requiredTestType\":\"unit\",\"riskTier\":\"high\",\"testMechanism\":{\"entrypoint\":\"public-function\",\"inputOrigin\":\"synthetic-fixture\",\"assertionSurface\":\"returned-value\",\"dependencyPath\":\"not-applicable\",\"productionOwners\":[\"src/calc.sh\"],\"negativeControl\":\"inverting the operator changes the total\",\"negativeControlMechanism\":\"mutation\"${extra}}}]}" \
    > "$REPO/specs/feature/scenario-manifest.json"
}

MUTATE='awk "{gsub(/\\+/,\"-\"); print}" src/calc.sh > src/calc.new && mv src/calc.new src/calc.sh'
TESTCMD='bash tests/calc-test.sh'

LAST_OUT=""
LAST_RC=0
run_sut() {
  local sub="$1"
  shift
  LAST_OUT="$(bash "$SUT" "$sub" "$@" 2>&1)"
  LAST_RC=$?
}

# Read one key=value line out of the captured output. A herestring, never a
# discarding pipe: `... | grep -q` on unbounded input under pipefail is the
# BUG-009 SIGPIPE race.
kv() {
  awk -v k="$1" -F= '$1 == k { print substr($0, length(k) + 2); exit }' <<< "$LAST_OUT"
}

store_count() {
  if [[ -f "$REPO/$STORE_REL" ]]; then
    awk 'END { print NR + 0 }' "$REPO/$STORE_REL"
  else
    printf '0'
  fi
}

# Overwrite the store with one forged record. Every field is well-formed except
# the ones each case is probing.
forge() {
  mkdir -p "$REPO/.specify/runtime"
  printf '%s\n' "$1" > "$REPO/$STORE_REL"
}

# ---------------------------------------------------------------------------
# DEFAULT OFF. An unconfigured repository is a clean no-op: nothing runs,
# nothing is recorded, no runtime directory appears, and a high-risk mutation
# declaration owes no receipt. This is the measured state of every repository in
# the estate today, so it is the state that must not change.
# ---------------------------------------------------------------------------
new_repo unset
write_manifest
run_sut run --repo-root "$REPO" --scenario-id SCN-001-001 --test-id calc \
  --source src/calc.sh --mutate "$MUTATE" --test "$TESTCMD" --expect 'FAIL: add'
assert_eq "unconfigured repo: run exits 0" "$LAST_RC" "0"
assert_eq "unconfigured repo: adapter is none" "$(kv adapter)" "none"
assert_eq "unconfigured repo: receipts skipped" "$(kv receipts)" "skipped"
assert_eq "unconfigured repo: zero records written" "$(store_count)" "0"
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "unconfigured repo: a high-risk mutation declaration owes no receipt" "$LAST_RC" "0"
if [[ -e "$REPO/.specify" ]]; then
  fail "unconfigured repo: no runtime directory is created"
else
  pass "unconfigured repo: no runtime directory is created"
fi

# An explicit opt-out is the same no-op as never configuring the capability.
new_repo none
write_manifest
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "explicit adapter=none: check is inert" "$LAST_RC" "0"

# A configured-but-unknown adapter fails LOUD rather than degrading to none: a
# typo must not silently disable the control.
new_repo command
printf 'mutationExecution:\n  adapter: command\n  command: scripts/absent\n' \
  > "$REPO/.github/bubbles-project.yaml"
run_sut resolve --repo-root "$REPO"
assert_eq "a configured-but-broken adapter fails loud" "$LAST_RC" "1"

# ---------------------------------------------------------------------------
# P1/P2. A REAL executed mutation. The mutant is derived from bytes that really
# changed, the test really fails, and the failure really carries the expectation.
# ---------------------------------------------------------------------------
new_repo command
write_manifest
BEFORE_DIGEST="$(sha "$REPO/src/calc.sh")"
run_sut run --repo-root "$REPO" --scenario-id SCN-001-001 --test-id calc \
  --source src/calc.sh --mutate "$MUTATE" --test "$TESTCMD" --expect 'FAIL: add'
assert_eq "P1 a killed mutant exits 0" "$LAST_RC" "0"
assert_eq "P1 the outcome is KILLED" "$(kv outcome)" "KILLED"
assert_eq "P1 one receipt was recorded" "$(store_count)" "1"
assert_eq "P1 the mutant digest differs from the source" \
  "$([[ "$(kv mutantId)" == "$(kv sourceDigest)" ]] && printf 'same' || printf 'different')" "different"
assert_eq "P1 the observed failure carries the expectation" \
  "$([[ "$(kv observedFailure)" == *"FAIL: add"* ]] && printf 'carried' || printf 'absent')" "carried"

# The independent witness. The repository file is re-read here, not reported by
# the script: isolation is a claim about the tree, so the tree is what is asked.
assert_eq "P2 ISOLATION: the repository source is byte-identical after the run" \
  "$(sha "$REPO/src/calc.sh")" "$BEFORE_DIGEST"
assert_eq "P2 the receipt's restoredDigest equals its sourceDigest" \
  "$(kv restoredDigest)" "$(kv sourceDigest)"
assert_eq "P2 the run is recorded as an isolated copy" "$(kv isolation)" "copied-fixture"

run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "P1 a high-risk scenario WITH a valid receipt passes the check" "$LAST_RC" "0"

# ---------------------------------------------------------------------------
# A2. The same scenario with NO receipt is a finding. This is the whole slice:
# the declaration alone was already accepted, and it proved nothing.
# ---------------------------------------------------------------------------
new_repo command
write_manifest
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "A2 a declared mutation with NO receipt is a finding" "$LAST_RC" "1"
assert_says "A2 the finding is named" "MUTATION-UNEXECUTED"

# ---------------------------------------------------------------------------
# P3. THE HONEST ESCAPE (IMP-048 R4). A declared negativeControlFallbackReason
# is not a finding. A repository with no mutation tooling stays shippable by
# SAYING it has none.
# ---------------------------------------------------------------------------
new_repo command
write_manifest ',"negativeControlFallbackReason":"no mutation runner exists for bash in this repository"'
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "P3 a declared fallback reason is NOT a finding" "$LAST_RC" "0"

# A3. ...and the escape is NOT implicit. The identical scenario without the
# stated reason is a finding, so silence can never buy what a declaration buys.
new_repo command
write_manifest ',"negativeControlFallbackReason":"   "'
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "A3 an UNDECLARED absence (blank reason) is still a finding" "$LAST_RC" "1"
assert_says "A3 the undeclared absence is named" "MUTATION-UNEXECUTED"

# P4. GUARD: an ordinary scenario carries no new burden. Without riskTier high
# and a mutation control, the check stays inert even with tooling configured.
new_repo command
printf '%s\n' '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-002","title":"t","requiredTestType":"unit","riskTier":"medium","testMechanism":{"entrypoint":"public-function","inputOrigin":"synthetic-fixture","assertionSurface":"returned-value","dependencyPath":"not-applicable","productionOwners":["src/calc.sh"],"negativeControl":"x","negativeControlMechanism":"perturbed-input"}}]}' \
  > "$REPO/specs/feature/scenario-manifest.json"
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "P4 a scenario that is not high-risk-with-mutation owes nothing" "$LAST_RC" "0"

# ---------------------------------------------------------------------------
# A4. A LEFT-BEHIND MUTANT. A receipt whose restoredDigest does not equal its
# sourceDigest is refused: the run ended with the tree still carrying the mutant.
# ---------------------------------------------------------------------------
new_repo command
write_manifest
SRC_D="$(sha "$REPO/src/calc.sh")"
forge "{\"schemaVersion\":\"mutation-receipt/v1\",\"scenarioId\":\"SCN-001-001\",\"testId\":\"calc\",\"sourcePath\":\"src/calc.sh\",\"sourceDigest\":\"$SRC_D\",\"mutantId\":\"deadbeef\",\"expectedFailureSignature\":\"FAIL: add\",\"observedFailure\":\"FAIL: add 2 3 expected 5 got -1\",\"restoredDigest\":\"deadbeef\",\"isolation\":\"copied-fixture\",\"workspace\":\"/tmp/x\",\"startedAt\":\"x\",\"finishedAt\":\"x\",\"exitCode\":1,\"outcome\":\"KILLED\"}"
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "A4 a receipt whose restoredDigest != sourceDigest is refused" "$LAST_RC" "1"
assert_says "A4 the left-behind mutant is named" "MUTATION-NOT-RESTORED"

# ---------------------------------------------------------------------------
# A5/A6. ADVERSARIAL: a receipt ASSERTED rather than executed. The store is the
# one artifact a caller can reach without running anything, so a record forged
# into it must not be able to buy a pass.
# ---------------------------------------------------------------------------
forge "{\"schemaVersion\":\"mutation-receipt/v1\",\"scenarioId\":\"SCN-001-001\",\"testId\":\"calc\",\"sourcePath\":\"src/calc.sh\",\"sourceDigest\":\"$SRC_D\",\"mutantId\":\"\",\"expectedFailureSignature\":\"FAIL: add\",\"observedFailure\":\"FAIL: add 2 3 expected 5 got -1\",\"restoredDigest\":\"$SRC_D\",\"isolation\":\"copied-fixture\",\"workspace\":\"/tmp/x\",\"startedAt\":\"x\",\"finishedAt\":\"x\",\"exitCode\":1,\"outcome\":\"KILLED\"}"
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "A5 a receipt with no mutantId cannot back a kill" "$LAST_RC" "1"
assert_says "A5 the forged receipt is named" "MUTATION-FORGED"

forge "{\"schemaVersion\":\"mutation-receipt/v1\",\"scenarioId\":\"SCN-001-001\",\"testId\":\"calc\",\"sourcePath\":\"src/calc.sh\",\"sourceDigest\":\"$SRC_D\",\"mutantId\":\"$SRC_D\",\"expectedFailureSignature\":\"FAIL: add\",\"observedFailure\":\"FAIL: add\",\"restoredDigest\":\"$SRC_D\",\"isolation\":\"copied-fixture\",\"workspace\":\"/tmp/x\",\"startedAt\":\"x\",\"finishedAt\":\"x\",\"exitCode\":1,\"outcome\":\"KILLED\"}"
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "A5b a mutant byte-identical to the source is not a mutant" "$LAST_RC" "1"
assert_says "A5b the identical mutant is named" "MUTATION-FORGED"

forge "{\"schemaVersion\":\"mutation-receipt/v1\",\"scenarioId\":\"SCN-001-001\",\"testId\":\"calc\",\"sourcePath\":\"src/calc.sh\",\"sourceDigest\":\"$SRC_D\",\"mutantId\":\"deadbeef\",\"expectedFailureSignature\":\"FAIL: add\",\"observedFailure\":\"\",\"restoredDigest\":\"$SRC_D\",\"isolation\":\"copied-fixture\",\"workspace\":\"/tmp/x\",\"startedAt\":\"x\",\"finishedAt\":\"x\",\"exitCode\":1,\"outcome\":\"KILLED\"}"
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "A6 a kill with no observedFailure was asserted, not executed" "$LAST_RC" "1"
assert_says "A6 the unobserved kill is named" "MUTATION-FORGED"

forge "{\"schemaVersion\":\"mutation-receipt/v1\",\"scenarioId\":\"SCN-001-001\",\"testId\":\"calc\",\"sourcePath\":\"src/calc.sh\",\"sourceDigest\":\"$SRC_D\",\"mutantId\":\"deadbeef\",\"expectedFailureSignature\":\"\",\"observedFailure\":\"FAIL: add\",\"restoredDigest\":\"$SRC_D\",\"isolation\":\"copied-fixture\",\"workspace\":\"/tmp/x\",\"startedAt\":\"x\",\"finishedAt\":\"x\",\"exitCode\":1,\"outcome\":\"KILLED\"}"
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "A6b a receipt with no expected signature is unfalsifiable" "$LAST_RC" "1"
assert_says "A6b the unfalsifiable receipt is named" "MUTATION-FORGED"

# A receipt claiming the shared tree as its workspace is refused on the READ
# side too, so an old record cannot outlive the run-side refusal.
forge "{\"schemaVersion\":\"mutation-receipt/v1\",\"scenarioId\":\"SCN-001-001\",\"testId\":\"calc\",\"sourcePath\":\"src/calc.sh\",\"sourceDigest\":\"$SRC_D\",\"mutantId\":\"deadbeef\",\"expectedFailureSignature\":\"FAIL: add\",\"observedFailure\":\"FAIL: add\",\"restoredDigest\":\"$SRC_D\",\"isolation\":\"shared-tree\",\"workspace\":\"/tmp/x\",\"startedAt\":\"x\",\"finishedAt\":\"x\",\"exitCode\":1,\"outcome\":\"KILLED\"}"
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "A6c a receipt recording a non-isolated run is refused" "$LAST_RC" "1"
assert_says "A6c the non-isolated run is named" "MUTATION-NOT-ISOLATED"

# ---------------------------------------------------------------------------
# A7. ADVERSARIAL: the mutation is aimed at the SHARED working tree. Refused
# BEFORE anything is mutated, because the refusal is worthless afterwards.
# ---------------------------------------------------------------------------
new_repo command
write_manifest
BEFORE_DIGEST="$(sha "$REPO/src/calc.sh")"
run_sut run --repo-root "$REPO" --scenario-id SCN-001-001 --test-id calc \
  --source src/calc.sh --mutate "$MUTATE" --test "$TESTCMD" --expect 'FAIL: add' \
  --workspace "$REPO"
assert_eq "A7 a mutation aimed at the repository itself is refused" "$LAST_RC" "2"
assert_says "A7 the commit window is named in the refusal" "commit window"
assert_eq "A7 the refusal wrote nothing" "$(store_count)" "0"
assert_eq "A7 the source was never touched" "$(sha "$REPO/src/calc.sh")" "$BEFORE_DIGEST"

# A subdirectory of the repository is the same shared tree.
mkdir -p "$REPO/build/scratch"
run_sut run --repo-root "$REPO" --scenario-id SCN-001-001 --test-id calc \
  --source src/calc.sh --mutate "$MUTATE" --test "$TESTCMD" --expect 'FAIL: add' \
  --workspace "$REPO/build/scratch"
assert_eq "A7b a workspace INSIDE the repository is refused too" "$LAST_RC" "2"

# A workspace genuinely outside the repository is accepted.
OUTSIDE="$TMP_DIR/outside$repo_seq"
mkdir -p "$OUTSIDE"
run_sut run --repo-root "$REPO" --scenario-id SCN-001-001 --test-id calc \
  --source src/calc.sh --mutate "$MUTATE" --test "$TESTCMD" --expect 'FAIL: add' \
  --workspace "$OUTSIDE"
assert_eq "P5 a workspace outside the repository is accepted" "$LAST_RC" "0"
assert_eq "P5 it is recorded as a declared workspace" "$(kv isolation)" "declared-workspace"
assert_eq "P5 the repository source is still untouched" "$(sha "$REPO/src/calc.sh")" "$BEFORE_DIGEST"

# ---------------------------------------------------------------------------
# A8. A SURVIVING mutant. The test still passed with the owning predicate wrong,
# which is precisely the vacuous control the tier exists to catch.
# ---------------------------------------------------------------------------
new_repo command
write_manifest
run_sut run --repo-root "$REPO" --scenario-id SCN-001-001 --test-id calc \
  --source src/calc.sh --mutate "$MUTATE" --test 'true' --expect 'FAIL: add'
assert_eq "A8 a surviving mutant is a finding" "$LAST_RC" "1"
assert_eq "A8 the outcome is SURVIVED" "$(kv outcome)" "SURVIVED"
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature"
assert_eq "A8 a SURVIVED receipt does not satisfy the check" "$LAST_RC" "1"
assert_says "A8 the survival is named" "MUTATION-NOT-KILLED"

# A9. The test failed, but not for the stated reason. A failure that is not the
# expected one is a broken harness masquerading as a kill.
new_repo command
write_manifest
run_sut run --repo-root "$REPO" --scenario-id SCN-001-001 --test-id calc \
  --source src/calc.sh --mutate "$MUTATE" --test "$TESTCMD" --expect 'FAIL: subtract'
assert_eq "A9 a failure that does not carry the expectation is a finding" "$LAST_RC" "1"
assert_eq "A9 the outcome is SIGNATURE_MISMATCH" "$(kv outcome)" "SIGNATURE_MISMATCH"

# A10. A mutate command that changes no bytes produced no mutant, so there is
# nothing to kill and no receipt to earn.
new_repo command
write_manifest
run_sut run --repo-root "$REPO" --scenario-id SCN-001-001 --test-id calc \
  --source src/calc.sh --mutate 'true' --test "$TESTCMD" --expect 'FAIL: add'
assert_eq "A10 a mutate command changing no bytes is refused" "$LAST_RC" "2"
assert_says "A10 the absent mutant is named" "is not a mutant"
assert_eq "A10 the refusal wrote nothing" "$(store_count)" "0"

# A11. A source that does not exist cannot be digested, so no receipt can be
# earned from it.
run_sut run --repo-root "$REPO" --scenario-id SCN-001-001 --test-id calc \
  --source src/absent.sh --mutate "$MUTATE" --test "$TESTCMD" --expect 'FAIL: add'
assert_eq "A11 an absent source is refused" "$LAST_RC" "2"

run_sut run --repo-root "$REPO" --scenario-id SCN-001-001 --test-id calc \
  --source ../outside.sh --mutate "$MUTATE" --test "$TESTCMD" --expect 'FAIL: add'
assert_eq "A11b a source escaping the repository is refused" "$LAST_RC" "2"

# A12. An expectation is mandatory: a receipt recording only "it failed" cannot
# distinguish a killed mutant from a broken harness.
run_sut run --repo-root "$REPO" --scenario-id SCN-001-001 --test-id calc \
  --source src/calc.sh --mutate "$MUTATE" --test "$TESTCMD"
assert_eq "A12 a run with no --expect is refused" "$LAST_RC" "2"

# ---------------------------------------------------------------------------
# U1. No bypass exists.
# ---------------------------------------------------------------------------
run_sut run --repo-root "$REPO" --scenario-id x --test-id y --source src/calc.sh \
  --mutate true --test true --expect z --force
assert_eq "U1 a bypass flag is rejected by name on run" "$LAST_RC" "2"
run_sut check --repo-root "$REPO" --spec-dir "$REPO/specs/feature" --skip-receipts
assert_eq "U1 a bypass flag is rejected by name on check" "$LAST_RC" "2"
run_sut check --repo-root "$REPO"
assert_eq "U1 check without --spec-dir exits 2" "$LAST_RC" "2"
LAST_OUT="$(bash "$SUT" 2>&1)"
LAST_RC=$?
assert_eq "U1 no subcommand exits 2" "$LAST_RC" "2"

# ---------------------------------------------------------------------------
printf '\n%s: %d passed, %d failed\n' "$NAME" "$passes" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
