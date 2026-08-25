#!/usr/bin/env bash
# Runner-contract regression driver for BUG-061-014.
#
# The defect: both shell E2E classifiers encode a two-valued result vocabulary
# (PASS / FAIL) while the outcome domain of a test suite has three values
# (proved / disproved / not run). Exit 77 — the skip convention that
# tests/e2e/assistant_regression/lib/regression_helpers.sh already produces —
# therefore falls down the failure branch in both.
#
# This driver asserts the three-outcome contract against the REAL classifiers:
#
#   * tests/e2e/run_all.sh is SYMLINKED into a sandbox, so the tracked file is
#     the code that executes. Only its environment is stubbed (a no-op
#     smackerel.sh and a no-op lib/helpers.sh) so no Docker stack is booted and
#     this driver is safe to nest inside a lane that already owns a stack.
#   * smackerel.sh's classifier block is extracted VERBATIM by function name and
#     evaluated, so the tracked text is the code that executes.
#
# Per ADV-061-014-04 the branch logic is never re-implemented here. If the
# runners are unchanged, the extracted/symlinked code is the unchanged code and
# these assertions fail.
#
# Usage:
#   bash tests/e2e/runner_contract/run_runner_contract.sh
#   ./smackerel.sh test e2e --shell-run runner_contract/run_runner_contract.sh

set -uo pipefail

DRIVER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(cd "$DRIVER_DIR/.." && pwd)"
REPO_ROOT="$(cd "$E2E_DIR/../.." && pwd)"
REAL_RUN_ALL="$E2E_DIR/run_all.sh"
REAL_CLI="$REPO_ROOT/smackerel.sh"

CHECKS_RUN=0
CHECKS_FAILED=0
CURRENT_SCENARIO="(none)"

WORK_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/bug-061-014-runner-contract.XXXXXX")"
cleanup() { rm -rf "$WORK_ROOT"; }
trap cleanup EXIT

scenario() {
  CURRENT_SCENARIO="$1"
  echo ""
  echo "── $CURRENT_SCENARIO ──"
}

# record <ok:0|1> <assertion-id> <description> [detail...]
record() {
  local ok="$1" id="$2" desc="$3"
  shift 3
  CHECKS_RUN=$((CHECKS_RUN + 1))
  if [ "$ok" -eq 0 ]; then
    echo "  ok   $id — $desc"
    return 0
  fi
  CHECKS_FAILED=$((CHECKS_FAILED + 1))
  echo "  FAIL $id — $desc"
  local line
  for line in "$@"; do
    echo "       $line"
  done
  return 0
}

assert_contains() { # <id> <desc> <haystack> <needle>
  local id="$1" desc="$2" haystack="$3" needle="$4"
  if printf '%s' "$haystack" | grep -qF -- "$needle"; then
    record 0 "$id" "$desc"
  else
    record 1 "$id" "$desc" "expected output to contain: $needle" "actual output follows:" "$haystack"
  fi
}

assert_not_contains() { # <id> <desc> <haystack> <needle>
  local id="$1" desc="$2" haystack="$3" needle="$4"
  if printf '%s' "$haystack" | grep -qF -- "$needle"; then
    record 1 "$id" "$desc" "expected output NOT to contain: $needle" "actual output follows:" "$haystack"
  else
    record 0 "$id" "$desc"
  fi
}

assert_eq() { # <id> <desc> <actual> <expected>
  local id="$1" desc="$2" actual="$3" expected="$4"
  if [ "$actual" = "$expected" ]; then
    record 0 "$id" "$desc"
  else
    record 1 "$id" "$desc" "expected: [$expected]" "actual:   [$actual]"
  fi
}

assert_ne() { # <id> <desc> <actual> <not-expected>
  local id="$1" desc="$2" actual="$3" forbidden="$4"
  if [ "$actual" = "$forbidden" ]; then
    record 1 "$id" "$desc" "value must not be: [$forbidden]" "actual: [$actual]"
  else
    record 0 "$id" "$desc"
  fi
}

# ── Sandbox construction ─────────────────────────────────────────────────────
# A sandbox mirrors just enough of the repository shape for run_all.sh to run:
#   <sandbox>/smackerel.sh          no-op stand-in for stack boot / teardown
#   <sandbox>/tests/e2e/run_all.sh  SYMLINK to the tracked runner under test
#   <sandbox>/tests/e2e/lib/helpers.sh  no-op stand-in for the live-stack helpers
# helpers.sh computes REPO_DIR as ../../.. from its own location, exactly as the
# real one does, so the stub smackerel.sh is what run_all.sh invokes.

make_sandbox() { # <name> -> echoes the sandbox tests/e2e dir
  local name="$1"
  local sandbox="$WORK_ROOT/$name"
  mkdir -p "$sandbox/tests/e2e/lib"

  cat >"$sandbox/smackerel.sh" <<'STUB'
#!/usr/bin/env bash
# Sandbox stand-in: the runner-contract driver asserts classification, not
# stack lifecycle, so every lifecycle verb succeeds without touching Docker.
exit 0
STUB
  chmod +x "$sandbox/smackerel.sh"

  cat >"$sandbox/tests/e2e/lib/helpers.sh" <<'STUB'
#!/usr/bin/env bash
# Sandbox stand-in for tests/e2e/lib/helpers.sh. Provides only the three names
# run_all.sh consumes, with the same REPO_DIR derivation as the real helpers.
set -euo pipefail
HELPERS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$HELPERS_DIR/../../.." && pwd)"
TEST_ENV="${TEST_ENV:-test}"
E2E_STACK_MANAGED="${E2E_STACK_MANAGED:-0}"
e2e_setup() { return 0; }
e2e_wait_healthy() { return 0; }
STUB

  ln -s "$REAL_RUN_ALL" "$sandbox/tests/e2e/run_all.sh"
  printf '%s\n' "$sandbox/tests/e2e"
}

write_fixture() { # <sandbox-e2e-dir> <basename-without-.sh> <exit-code> [skip-reason-token]
  local dir="$1" name="$2" code="$3" token="${4:-}"
  {
    echo '#!/usr/bin/env bash'
    echo 'set -euo pipefail'
    echo "echo \"synthetic fixture $name executing (will exit $code)\""
    if [ -n "$token" ]; then
      echo 'echo "RESULT: SKIPPED"'
      echo "echo \"SKIP_REASON: $token\""
    fi
    echo "exit $code"
  } >"$dir/$name.sh"
  chmod +x "$dir/$name.sh"
}

# run_sandbox_runner <sandbox-e2e-dir> -> sets RUNNER_OUT / RUNNER_STATUS
RUNNER_OUT=""
RUNNER_STATUS=0
run_sandbox_runner() {
  local dir="$1"
  RUNNER_OUT="$(cd "$dir" && bash "$dir/run_all.sh" 'test_*.sh' 2>&1)"
  RUNNER_STATUS=$?
}

# ── Read the runner's own required-set declaration ───────────────────────────
# Requiredness is a runner-side declaration (design.md D3): a fixture must not
# be able to downgrade itself out of the required set. The driver therefore
# reads the declared array rather than inventing a name, so the assertion is
# made against whatever the tracked runner actually declares.
declared_required_name() {
  sed -n 's/^REQUIRED_TESTS="\(.*\)"$/\1/p' "$REAL_RUN_ALL" | head -1 | awk '{print $1}'
}

declared_cli_required_name() {
  awk '
    /^[[:space:]]*e2e_required_shell_tests=\(/ { inside = 1; next }
    inside && /^[[:space:]]*\)/ { exit }
    inside {
      gsub(/#.*/, "")
      gsub(/^[[:space:]]+|[[:space:]]+$/, "")
      if (length($0) > 0) { print $1; exit }
    }
  ' "$REAL_CLI"
}

echo "========================================="
echo "  BUG-061-014 runner-contract regression"
echo "========================================="
echo "runner under test: $REAL_RUN_ALL"
echo "cli under test:    $REAL_CLI"

# ── SCN-061-014-01 / -03 / -04 / -07: mixed run through the real run_all.sh ──
scenario "SCN-061-014-01/03/04/07 — mixed pass/skip/fail through tests/e2e/run_all.sh"

MIXED_DIR="$(make_sandbox mixed)"
write_fixture "$MIXED_DIR" test_rc_pass 0
write_fixture "$MIXED_DIR" test_rc_optional_skip 77 RC-SYNTHETIC-BLOCKER-TOKEN
write_fixture "$MIXED_DIR" test_rc_fail 1
run_sandbox_runner "$MIXED_DIR"
echo "$RUNNER_OUT"
echo "  (runner exit status: $RUNNER_STATUS)"

assert_contains AC-01-1 "skip fixture is reported as SKIP" \
  "$RUNNER_OUT" "SKIP: test_rc_optional_skip"
# ADV-061-014-01: mapping 77 onto the PASS branch removes the red line while
# recreating BUG-069-005. These two assertions are what refuse that fix.
assert_not_contains ADV-01-a "ADV-061-014-01: skip fixture is NOT reported as PASS" \
  "$RUNNER_OUT" "PASS: test_rc_optional_skip"
assert_not_contains AC-01-2 "skip fixture is NOT reported as FAIL" \
  "$RUNNER_OUT" "FAIL: test_rc_optional_skip"
assert_contains AC-04-1 "SCN-061-014-04: results block carries the SKIP_REASON token" \
  "$RUNNER_OUT" "RC-SYNTHETIC-BLOCKER-TOKEN"
assert_contains AC-05-1 "fixture stdout is still streamed live" \
  "$RUNNER_OUT" "synthetic fixture test_rc_optional_skip executing"

assert_contains AC-03-1 "SCN-061-014-03: passed count is 1" "$RUNNER_OUT" "Passed: 1"
assert_contains AC-03-2 "SCN-061-014-03: failed count is 1" "$RUNNER_OUT" "Failed: 1"
assert_contains AC-03-3 "SCN-061-014-03: skipped count is 1" "$RUNNER_OUT" "Skipped: 1"
assert_contains AC-03-4 "SCN-061-014-03: total reconciles to the three-way sum" "$RUNNER_OUT" "Total:  3"

# ADV-061-014-03: the exit-1 control proves the skip branch was not broadened to
# "any non-zero the runner does not recognise".
assert_contains ADV-03-a "ADV-061-014-03: SCN-061-014-07 exit-1 control stays FAIL" \
  "$RUNNER_OUT" "FAIL: test_rc_fail"
assert_not_contains ADV-03-b "ADV-061-014-03: exit-1 control is NOT reclassified as SKIP" \
  "$RUNNER_OUT" "SKIP: test_rc_fail"
assert_ne ADV-03-c "ADV-061-014-03: a real failure keeps the suite exit non-zero" \
  "$RUNNER_STATUS" "0"

# ── SCN-061-014-06: an optional skip does not redden an otherwise clean run ──
scenario "SCN-061-014-06 — optional skip on an otherwise clean run"

OPT_DIR="$(make_sandbox optional)"
write_fixture "$OPT_DIR" test_rc_pass 0
write_fixture "$OPT_DIR" test_rc_optional_skip 77 RC-SYNTHETIC-BLOCKER-TOKEN
run_sandbox_runner "$OPT_DIR"
echo "$RUNNER_OUT"
echo "  (runner exit status: $RUNNER_STATUS)"

assert_contains AC-06-1 "optional skip is reported as SKIP" \
  "$RUNNER_OUT" "SKIP: test_rc_optional_skip"
assert_eq AC-06-2 "optional skip leaves the suite exit status at 0" "$RUNNER_STATUS" "0"
assert_contains AC-06-3 "optional skip is not counted as a failure" "$RUNNER_OUT" "Failed: 0"

# ── SCN-061-014-05: a required skip keeps the suite non-green ────────────────
scenario "SCN-061-014-05 — required skip keeps the suite non-green with zero failures"

REQUIRED_NAME="$(declared_required_name)"
if [ -z "$REQUIRED_NAME" ]; then
  record 1 AC-05-0 "ADV-061-014-02: run_all.sh declares a REQUIRED_TESTS set" \
    "tests/e2e/run_all.sh has no REQUIRED_TESTS declaration, so requiredness cannot be" \
    "resolved and every skip is silently benign — the BUG-069-005 condition." \
    "Scenario SCN-061-014-05 cannot be exercised against this runner."
else
  record 0 AC-05-0 "ADV-061-014-02: run_all.sh declares a REQUIRED_TESTS set (first entry: $REQUIRED_NAME)"
  REQ_DIR="$(make_sandbox required)"
  write_fixture "$REQ_DIR" test_rc_pass 0
  write_fixture "$REQ_DIR" "$REQUIRED_NAME" 77 RC-REQUIRED-BLOCKER-TOKEN
  run_sandbox_runner "$REQ_DIR"
  echo "$RUNNER_OUT"
  echo "  (runner exit status: $RUNNER_STATUS)"

  assert_contains AC-05-1b "required fixture is reported as SKIP" \
    "$RUNNER_OUT" "SKIP: $REQUIRED_NAME"
  assert_contains AC-05-2 "required skip records zero failures" "$RUNNER_OUT" "Failed: 0"
  # ADV-061-014-02: making every skip benign would let required behaviour go
  # unproven under a green suite.
  assert_ne ADV-02-a "ADV-061-014-02: required skip yields a non-zero suite exit" \
    "$RUNNER_STATUS" "0"
fi

# ── SCN-061-014-02: the smackerel.sh shell-lane classifier ──────────────────
scenario "SCN-061-014-02 — smackerel.sh shell-lane classifier (extracted verbatim)"

# ADV-061-014-04: extract the tracked classifier text by function name so the
# code executed here is the code that ships. A re-implementation in this file
# would pass against an unchanged smackerel.sh; this cannot.
CLI_BLOCK="$(awk '
  /^[[:space:]]*e2e_shell_results=\(\)[[:space:]]*$/ { capture = 1 }
  capture { print }
  capture && /^[[:space:]]*e2e_print_shell_summary\(\)[[:space:]]*\{[[:space:]]*$/ { closing = 1 }
  closing && /^[[:space:]]*\}[[:space:]]*$/ { exit }
' "$REAL_CLI")"

if ! printf '%s' "$CLI_BLOCK" | grep -q 'e2e_record_shell_result()'; then
  record 1 AC-02-0 "smackerel.sh classifier block extracted" \
    "could not locate e2e_record_shell_result() between e2e_shell_results=() and" \
    "the close of e2e_print_shell_summary() in $REAL_CLI"
else
  record 0 AC-02-0 "smackerel.sh classifier block extracted ($(printf '%s' "$CLI_BLOCK" | wc -l) lines)"

  CLI_OUT="$(
    set +u
    eval "$CLI_BLOCK"
    e2e_record_shell_result "runner_contract/rc_pass_fixture.sh" 0
    e2e_record_shell_result "runner_contract/rc_optional_skip_fixture.sh" 77
    e2e_record_shell_result "runner_contract/rc_fail_fixture.sh" 1
    e2e_print_shell_summary
    echo "OVERALL_STATUS=$e2e_overall_status"
    echo "SHELL_FAILURES=$e2e_shell_failures"
  )"
  echo "$CLI_OUT"

  CLI_OVERALL="$(printf '%s\n' "$CLI_OUT" | sed -n 's/^OVERALL_STATUS=//p' | tail -1)"
  CLI_FAILURES="$(printf '%s\n' "$CLI_OUT" | sed -n 's/^SHELL_FAILURES=//p' | tail -1)"

  assert_contains AC-02-1 "skip fixture is reported as SKIP in the shell results block" \
    "$CLI_OUT" "SKIP: runner_contract/rc_optional_skip_fixture.sh"
  assert_not_contains ADV-01-b "ADV-061-014-01: skip fixture is NOT reported as PASS" \
    "$CLI_OUT" "PASS: runner_contract/rc_optional_skip_fixture.sh"
  assert_not_contains AC-02-2 "skip fixture is NOT reported as FAIL" \
    "$CLI_OUT" "FAIL: runner_contract/rc_optional_skip_fixture.sh"
  assert_eq AC-02-3 "the shell failure tally counts only the exit-1 fixture" "$CLI_FAILURES" "1"
  assert_ne AC-02-4 "the lane exit status is not the raw child status 77" "$CLI_OVERALL" "77"
  assert_contains AC-02-5 "the shell summary reports a skipped tally" "$CLI_OUT" "Skipped: 1"
  assert_contains AC-02-6 "the shell summary total reconciles to the three-way sum" "$CLI_OUT" "Total:  3"
  assert_contains AC-02-7 "the shell summary passed tally excludes the skip" "$CLI_OUT" "Passed: 1"
  assert_contains ADV-03-d "ADV-061-014-03: the exit-1 control stays FAIL in the shell lane" \
    "$CLI_OUT" "FAIL: runner_contract/rc_fail_fixture.sh"

  CLI_REQUIRED_NAME="$(declared_cli_required_name)"
  if [ -z "$CLI_REQUIRED_NAME" ]; then
    record 1 AC-02-8 "ADV-061-014-02: smackerel.sh declares a required shell-test set" \
      "smackerel.sh has no e2e_required_shell_tests declaration, so a required skip" \
      "in the CLI lane is silently benign — the BUG-069-005 condition."
  else
    record 0 AC-02-8 "ADV-061-014-02: smackerel.sh declares a required shell-test set (first entry: $CLI_REQUIRED_NAME)"
    CLI_REQ_OUT="$(
      set +u
      eval "$CLI_BLOCK"
      e2e_record_shell_result "$CLI_REQUIRED_NAME" 77
      e2e_print_shell_summary
      echo "OVERALL_STATUS=$e2e_overall_status"
      echo "SHELL_FAILURES=$e2e_shell_failures"
    )"
    echo "$CLI_REQ_OUT"
    CLI_REQ_OVERALL="$(printf '%s\n' "$CLI_REQ_OUT" | sed -n 's/^OVERALL_STATUS=//p' | tail -1)"
    CLI_REQ_FAILURES="$(printf '%s\n' "$CLI_REQ_OUT" | sed -n 's/^SHELL_FAILURES=//p' | tail -1)"
    assert_contains AC-02-9 "required CLI skip is reported as SKIP" "$CLI_REQ_OUT" "SKIP: $CLI_REQUIRED_NAME"
    assert_eq AC-02-10 "required CLI skip records zero failures" "$CLI_REQ_FAILURES" "0"
    assert_ne ADV-02-b "ADV-061-014-02: required CLI skip yields a non-zero lane exit" "$CLI_REQ_OVERALL" "0"
    assert_ne AC-02-11 "required CLI skip does not propagate the raw child status 77" "$CLI_REQ_OVERALL" "77"
  fi
fi

# ── SCN-061-014-08: the seven existing reg_skip_with_blocker slots ───────────
scenario "SCN-061-014-08 — the seven reg_skip_with_blocker fixtures classify as SKIP"

EXISTING_FIXTURES=(
  assistant_regression/bs_002_retrieval_qa.sh
  assistant_regression/bs_004_notification_confirm.sh
  assistant_regression/bs_005_ambiguous_disambig.sh
  assistant_regression/bs_007_provenance_violation.sh
  assistant_regression/bs_008_disabled_skill.sh
  assistant_regression/bs_009_sst_missing_boot_failure.sh
  assistant_acceptance_telegram_smoke.sh
)

EXISTING_DIR="$(make_sandbox existing)"
for rel in "${EXISTING_FIXTURES[@]}"; do
  base="test_rc_existing_$(basename "$rel" .sh)"
  {
    echo '#!/usr/bin/env bash'
    echo "exec bash \"$E2E_DIR/$rel\""
  } >"$EXISTING_DIR/$base.sh"
  chmod +x "$EXISTING_DIR/$base.sh"
done
run_sandbox_runner "$EXISTING_DIR"
echo "$RUNNER_OUT"
echo "  (runner exit status: $RUNNER_STATUS)"

assert_contains AC-08-0 "all seven slots are accounted for" "$RUNNER_OUT" "Total:  7"
for rel in "${EXISTING_FIXTURES[@]}"; do
  base="test_rc_existing_$(basename "$rel" .sh)"
  assert_contains "AC-08-${base#test_rc_existing_}" "$rel classifies as SKIP" \
    "$RUNNER_OUT" "SKIP: $base"
  assert_not_contains "AC-08F-${base#test_rc_existing_}" "$rel does NOT classify as FAIL" \
    "$RUNNER_OUT" "FAIL: $base"
done
assert_contains AC-08-9 "each slot carries its own SKIP_REASON in the results block" \
  "$RUNNER_OUT" "SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED"
assert_contains AC-08-10 "a second slot carries a different SKIP_REASON" \
  "$RUNNER_OUT" "SCOPE-06-GRAPH-SEEDING-NOT-YET-AUTHORED"

# ── Summary ─────────────────────────────────────────────────────────────────
echo ""
echo "========================================="
echo "  Runner-contract results"
echo "========================================="
echo "  Assertions run:    $CHECKS_RUN"
echo "  Assertions failed: $CHECKS_FAILED"
echo "========================================="

if [ "$CHECKS_RUN" -eq 0 ]; then
  echo "ERROR: no assertions executed — the driver proved nothing." >&2
  exit 1
fi
if [ "$CHECKS_FAILED" -gt 0 ]; then
  exit 1
fi
exit 0
