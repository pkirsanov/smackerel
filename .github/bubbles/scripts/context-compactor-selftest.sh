#!/usr/bin/env bash
set -euo pipefail

# context-compactor-selftest.sh — verify context-compactor.sh behavior.
#
# Cases:
#   1. Long-output envelope is truncated and rawPointer is preserved.
#   2. Short-output envelope is preserved verbatim with no truncation.
#   3. Envelope missing optional fields does not crash; missing fields
#      are recorded as JSON null.
#   4. Running twice on the same input produces byte-identical output
#      (idempotency).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPACTOR="$SCRIPT_DIR/context-compactor.sh"

if [[ ! -x "$COMPACTOR" ]]; then
  echo "FAIL: context-compactor.sh is not executable at $COMPACTOR" >&2
  exit 1
fi

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

failures=0

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

write_fixture() {
  # Write a fixture file under $TMP_ROOT. Uses heredoc redirection into a
  # transient test fixture (not a source/spec artifact), matching the
  # existing selftest convention (see runtime-lease-selftest.sh).
  local target="$1"
  local body="$2"
  printf '%s\n' "$body" > "$target"
}

echo "Running context-compactor selftest..."
echo "Scenario: orchestrator agents must compact subagent RESULT-ENVELOPEs without losing key fields or rawPointer provenance."

# ---- Case 1: long evidence is truncated, rawPointer preserved -------------

long_fixture="$TMP_ROOT/long-envelope.md"
write_fixture "$long_fixture" "## RESULT-ENVELOPE

agent: bubbles.implement
outcome: completed_owned
featureDir: specs/042-long-feature
scopeIds: scope-1, scope-2
dodItems: 12/12
evidenceRefs:
- raw evidence line 1
- raw evidence line 2
- raw evidence line 3
- raw evidence line 4
- raw evidence line 5
- raw evidence line 6
- raw evidence line 7
- raw evidence line 8
nextRequiredOwner: bubbles.test"

long_out="$("$COMPACTOR" "$long_fixture")"

if printf '%s' "$long_out" | grep -q '\.\.\.3 more lines'; then
  pass "Long-evidence envelope is truncated with '...3 more lines' sentinel"
else
  fail "Long-evidence envelope should include the truncation sentinel"
  echo "  output: $long_out"
fi

if printf '%s' "$long_out" | grep -Fq "\"rawPointer\":\"$long_fixture\""; then
  pass "rawPointer preserves absolute path back to the original envelope"
else
  fail "rawPointer should preserve the original envelope path"
  echo "  output: $long_out"
fi

if printf '%s' "$long_out" | grep -q '"agent":"bubbles.implement"' \
   && printf '%s' "$long_out" | grep -q '"outcome":"completed_owned"' \
   && printf '%s' "$long_out" | grep -q '"nextRequiredOwner":"bubbles.test"'; then
  pass "Long-envelope core routing fields (agent/outcome/nextRequiredOwner) survive truncation"
else
  fail "Truncation must preserve core routing fields"
  echo "  output: $long_out"
fi

# ---- Case 2: short envelope preserved verbatim ----------------------------

short_fixture="$TMP_ROOT/short-envelope.md"
write_fixture "$short_fixture" "agent: bubbles.test
outcome: completed_owned
featureDir: specs/099-tiny
scopeIds: only-scope
evidenceRefs:
- single evidence ref
nextRequiredOwner: bubbles.audit"

short_out="$("$COMPACTOR" "$short_fixture")"

if printf '%s' "$short_out" | grep -q '"evidenceRefs":"single evidence ref"'; then
  pass "Short-evidence envelope preserves the full evidence value verbatim"
else
  fail "Short-evidence envelope must preserve evidence verbatim (no truncation)"
  echo "  output: $short_out"
fi

if printf '%s' "$short_out" | grep -q 'more lines'; then
  fail "Short-evidence envelope must NOT add a truncation sentinel"
  echo "  output: $short_out"
else
  pass "Short-evidence envelope correctly skips the truncation sentinel"
fi

# ---- Case 3: missing optional fields do not crash, recorded as null -------

minimal_fixture="$TMP_ROOT/minimal-envelope.md"
write_fixture "$minimal_fixture" "outcome: blocked
blockedReason: external API unavailable"

minimal_out="$("$COMPACTOR" "$minimal_fixture")"

if printf '%s' "$minimal_out" | grep -q '"agent":null' \
   && printf '%s' "$minimal_out" | grep -q '"featureDir":null' \
   && printf '%s' "$minimal_out" | grep -q '"scopeIds":null' \
   && printf '%s' "$minimal_out" | grep -q '"evidenceRefs":null' \
   && printf '%s' "$minimal_out" | grep -q '"nextRequiredOwner":null'; then
  pass "Missing optional fields are recorded as JSON null"
else
  fail "Missing optional fields must be recorded as JSON null (no crash)"
  echo "  output: $minimal_out"
fi

if printf '%s' "$minimal_out" | grep -q '"blockedReason":"external API unavailable"' \
   && printf '%s' "$minimal_out" | grep -q '"outcome":"blocked"'; then
  pass "Present fields in a sparse envelope are still extracted"
else
  fail "Sparse envelope must still extract present fields"
  echo "  output: $minimal_out"
fi

# ---- Case 4: idempotency — two runs produce byte-identical output ---------

idem_fixture="$TMP_ROOT/idem-envelope.md"
write_fixture "$idem_fixture" "agent: bubbles.docs
outcome: completed_owned
featureDir: specs/123-idem
scopeIds: scope-A
evidenceRefs:
- run-twice evidence
nextRequiredOwner: bubbles.validate"

run_one="$("$COMPACTOR" "$idem_fixture")"
run_two="$("$COMPACTOR" "$idem_fixture")"

if [[ "$run_one" == "$run_two" ]]; then
  pass "Compactor is idempotent — two runs on the same input produce byte-identical output"
else
  fail "Compactor must be idempotent across repeated runs"
  echo "  run 1: $run_one"
  echo "  run 2: $run_two"
fi

# ---- Case 5: goal identity survives compaction verbatim (IMP-038 SCOPE-3) --
#
# The point of preserving these three fields is that a SUBSTITUTED ref must
# still be detectable after compaction. Deriving them from the session file
# would silently replace a wrong ref with the right one and destroy the
# evidence — so the fixture below carries a deliberately wrong digest and the
# test asserts that exact wrong value comes back out.

goal_fixture="$TMP_ROOT/goal-envelope.md"
write_fixture "$goal_fixture" "agent: bubbles.implement
outcome: completed
goalId: gc:vscode-abc123:2
revision: 2
sourceRequestDigest: sha256:1111111111111111111111111111111111111111111111111111111111111111
nextRequiredOwner: bubbles.test"

goal_out="$("$COMPACTOR" "$goal_fixture")"

if printf '%s' "$goal_out" | grep -q '"goalId":"gc:vscode-abc123:2"' \
   && printf '%s' "$goal_out" | grep -q '"revision":2,' \
   && printf '%s' "$goal_out" | grep -q '"sourceRequestDigest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"'; then
  pass "Goal identity is preserved verbatim through compaction"
else
  fail "Goal identity must survive compaction unchanged"
  echo "  output: $goal_out"
fi

# `revision` is an integer in the schema; emitting it quoted would make a
# downstream verify-ref comparison fail on type rather than on substance.
if printf '%s' "$goal_out" | grep -q '"revision":"2"'; then
  fail "revision must be emitted as a JSON integer, not a quoted string"
  echo "  output: $goal_out"
else
  pass "revision is emitted as a JSON integer"
fi

# A wrong digest must arrive at the verifier unchanged. This is the assertion
# that fails if compaction ever starts "helpfully" repairing the ref.
substituted_fixture="$TMP_ROOT/substituted-goal-envelope.md"
write_fixture "$substituted_fixture" "agent: bubbles.implement
outcome: completed
goalId: gc:someone-elses-session:1
revision: 1
sourceRequestDigest: sha256:0000000000000000000000000000000000000000000000000000000000000000"

substituted_out="$("$COMPACTOR" "$substituted_fixture")"

if printf '%s' "$substituted_out" | grep -q '"goalId":"gc:someone-elses-session:1"' \
   && printf '%s' "$substituted_out" | grep -q '"sourceRequestDigest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"'; then
  pass "A substituted goal ref survives compaction so resume can still catch it"
else
  fail "Compaction must NOT repair or drop a substituted goal ref"
  echo "  output: $substituted_out"
fi

# A pre-IMP-038 or read-only envelope froze no contract; null is the honest
# record, and it must not be confused with a present-but-empty ref.
if printf '%s' "$minimal_out" | grep -q '"goalRef":null'; then
  pass "An envelope with no goal identity records goalRef as null"
else
  fail "An envelope with no goal identity must record goalRef as null"
  echo "  output: $minimal_out"
fi

# ---- Sanity: --help exits 0 and prints usage ------------------------------

if "$COMPACTOR" --help >/dev/null 2>&1; then
  pass "--help exits 0"
else
  fail "--help should exit 0"
fi

if "$COMPACTOR" --help 2>/dev/null | grep -q '^Usage:'; then
  pass "--help prints a Usage banner"
else
  fail "--help should print a Usage banner"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "context-compactor selftest failed with $failures issue(s)."
  exit 1
fi
echo "context-compactor selftest passed."
