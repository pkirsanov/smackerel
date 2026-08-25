#!/usr/bin/env bash
# Tier-skip contract driver for BUG-061-014 SCOPE-02.
#
# Scope 1 corrected the false-RED half: exit 77 was being counted as a failure.
# This driver covers the false-GREEN half, which is the more dangerous one.
# tests/e2e/lib/helpers.sh::skip_unless_accel_tier prints a structured SKIP line
# and then exits 0, so on a cpu-tier host ten fixtures report PASS while proving
# nothing — precisely the condition
# specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/
# was opened for, reproduced in shell.
#
# A fix that removes one untrustworthy colour and leaves the other has not
# restored the property the suite is supposed to have. Both helpers must resolve
# to the same reported outcome.
#
# The REAL helper is sourced, never re-implemented, so if it is unchanged these
# assertions fail. The runner-level checks symlink the tracked run_all.sh into a
# sandbox for the same reason.
#
# Usage:
#   bash tests/e2e/runner_contract/run_tier_skip_contract.sh
#   ./smackerel.sh test e2e --shell-run runner_contract/run_tier_skip_contract.sh

set -uo pipefail

DRIVER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(cd "$DRIVER_DIR/.." && pwd)"
REPO_ROOT="$(cd "$E2E_DIR/../.." && pwd)"
REAL_HELPERS="$E2E_DIR/lib/helpers.sh"
REAL_RUN_ALL="$E2E_DIR/run_all.sh"

SKIP_EXIT=77

CHECKS_RUN=0
CHECKS_FAILED=0
CURRENT_SCENARIO="(none)"

WORK_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/bug-061-014-tier-skip.XXXXXX")"
cleanup() { rm -rf "$WORK_ROOT"; }
trap cleanup EXIT

scenario() {
  CURRENT_SCENARIO="$1"
  echo ""
  echo "--- $1 ---"
}

record() { # <ok:0|1> <id> <desc> [detail]
  CHECKS_RUN=$((CHECKS_RUN + 1))
  if [ "$1" -eq 0 ]; then
    echo "  ok   $2 — $3"
  else
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
    echo "  FAIL $2 — $3"
    [ -n "${4:-}" ] && echo "       $4"
  fi
}

assert_eq() { # <id> <desc> <actual> <expected>
  if [ "$3" = "$4" ]; then
    record 0 "$1" "$2"
  else
    record 1 "$1" "$2" "expected [$4], got [$3]"
  fi
}

assert_ne() { # <id> <desc> <actual> <not-expected>
  if [ "$3" != "$4" ]; then
    record 0 "$1" "$2"
  else
    record 1 "$1" "$2" "expected anything but [$4], got [$3]"
  fi
}

assert_contains() { # <id> <desc> <haystack> <needle>
  case "$3" in
    *"$4"*) record 0 "$1" "$2" ;;
    *) record 1 "$1" "$2" "missing [$4]" ;;
  esac
}

assert_not_contains() { # <id> <desc> <haystack> <needle>
  case "$3" in
    *"$4"*) record 1 "$1" "$2" "unexpectedly contains [$4]" ;;
    *) record 0 "$1" "$2" ;;
  esac
}

# ── Run the REAL helper in a child shell at a given tier ─────────────────────
# The helper is sourced from its tracked path so this driver asserts against the
# shipped code. A child shell is used because the helper's cpu and unknown paths
# call exit, which would take this driver down with them.
run_helper_at_tier() { # <tier> -> sets HELPER_OUT / HELPER_STATUS
  local tier="$1"
  HELPER_OUT="$(
    SMACKEREL_HARDWARE_TIER="$tier" bash -c '
      set -uo pipefail
      source "$1"
      skip_unless_accel_tier "tier-contract-probe"
      # Reached only when the helper returns rather than exiting. Emitting a
      # marker here is what lets SCN-11 distinguish "returned" from "exited 0",
      # which a bare exit-code check cannot do.
      echo "FIXTURE_BODY_EXECUTED"
    ' _ "$REAL_HELPERS" 2>&1
  )"
  HELPER_STATUS=$?
}

# ── Sandbox for the runner-level checks ──────────────────────────────────────
make_sandbox() { # <name> -> echoes the sandbox tests/e2e dir
  local name="$1"
  local sandbox="$WORK_ROOT/$name"
  mkdir -p "$sandbox/tests/e2e/lib"

  cat >"$sandbox/smackerel.sh" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB
  chmod +x "$sandbox/smackerel.sh"

  # Source order is load-bearing: the REAL helpers come first so the tracked
  # skip_unless_accel_tier is the one under test, then the lifecycle names are
  # overridden to no-ops. Reversing this lets the real e2e_setup win and boot an
  # actual stack, which is not what this driver is measuring.
  cat >"$sandbox/tests/e2e/lib/helpers.sh" <<STUB
#!/usr/bin/env bash
set -uo pipefail
source "$REAL_HELPERS"
HELPERS_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="\$(cd "\$HELPERS_DIR/../../.." && pwd)"
TEST_ENV="\${TEST_ENV:-test}"
E2E_STACK_MANAGED="\${E2E_STACK_MANAGED:-0}"
e2e_setup() { return 0; }
e2e_wait_healthy() { return 0; }
e2e_teardown() { return 0; }
STUB

  ln -s "$REAL_RUN_ALL" "$sandbox/tests/e2e/run_all.sh"
  printf '%s\n' "$sandbox/tests/e2e"
}

write_tier_fixture() { # <sandbox-e2e-dir> <basename>
  local dir="$1" name="$2"
  cat >"$dir/$name.sh" <<'FIX'
#!/usr/bin/env bash
set -uo pipefail
FIX_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$FIX_DIR/lib/helpers.sh"
skip_unless_accel_tier "synthetic-tier-fixture"
echo "FIXTURE_BODY_EXECUTED"
exit 0
FIX
  chmod +x "$dir/$name.sh"
}

write_plain_pass_fixture() { # <sandbox-e2e-dir> <basename>
  local dir="$1" name="$2"
  {
    echo '#!/usr/bin/env bash'
    echo 'set -uo pipefail'
    echo 'echo "plain fixture executing"'
    echo 'exit 0'
  } >"$dir/$name.sh"
  chmod +x "$dir/$name.sh"
}

RUNNER_OUT=""
RUNNER_STATUS=0
run_sandbox_runner() { # <sandbox-e2e-dir> <tier>
  local dir="$1" tier="$2"
  RUNNER_OUT="$(cd "$dir" && SMACKEREL_HARDWARE_TIER="$tier" bash "$dir/run_all.sh" 'test_*.sh' 2>&1)"
  RUNNER_STATUS=$?
}

echo "========================================="
echo "  BUG-061-014 SCOPE-02 tier-skip contract"
echo "========================================="
echo "helper under test: $REAL_HELPERS"

# ── SCN-061-014-09 ───────────────────────────────────────────────────────────
scenario "SCN-061-014-09 — the hardware-tier skip no longer reports success"

run_helper_at_tier cpu
assert_eq SCN-09-1 "SCN-061-014-09: cpu tier exits ${SKIP_EXIT}" "$HELPER_STATUS" "$SKIP_EXIT"
assert_ne SCN-09-2 "SCN-061-014-09: cpu tier does NOT exit 0" "$HELPER_STATUS" "0"
assert_contains SCN-09-3 "SCN-061-014-09: the structured SKIP line is preserved" \
  "$HELPER_OUT" "SKIP: tier-contract-probe"
# Uniformity with the other helper: both must emit a SKIP_REASON the classifiers
# can lift into the results block, or the two skips report differently.
assert_contains SCN-09-4 "SCN-061-014-09: a SKIP_REASON is emitted for the classifier" \
  "$HELPER_OUT" "SKIP_REASON:"
assert_not_contains SCN-09-5 "SCN-061-014-09: the fixture body did NOT run" \
  "$HELPER_OUT" "FIXTURE_BODY_EXECUTED"

# ── SCN-061-014-11 ───────────────────────────────────────────────────────────
scenario "SCN-061-014-11 — the accel path is unaffected"

run_helper_at_tier accel
assert_eq SCN-11-1 "SCN-061-014-11: accel tier exits 0" "$HELPER_STATUS" "0"
assert_contains SCN-11-2 "SCN-061-014-11: the helper returned and the body executed" \
  "$HELPER_OUT" "FIXTURE_BODY_EXECUTED"
assert_not_contains SCN-11-3 "SCN-061-014-11: no SKIP line on the accel path" \
  "$HELPER_OUT" "SKIP: tier-contract-probe"

# ── SCN-061-014-12 / ADV-061-014-06 ──────────────────────────────────────────
scenario "SCN-061-014-12 — an unknown tier is still a hard error"

run_helper_at_tier bogus-tier
# ADV-061-014-06: collapsing every non-accel tier into the skip branch would hide
# a misconfigured host as a benign skip. The unknown path must stay exit 2.
assert_eq ADV-06-a "ADV-061-014-06: unknown tier exits 2" "$HELPER_STATUS" "2"
assert_ne ADV-06-b "ADV-061-014-06: unknown tier is NOT reclassified as a skip" \
  "$HELPER_STATUS" "$SKIP_EXIT"
assert_ne ADV-06-c "ADV-061-014-06: unknown tier is NOT a success" "$HELPER_STATUS" "0"
assert_contains ADV-06-d "ADV-061-014-06: the misconfiguration is named" \
  "$HELPER_OUT" "unknown SMACKEREL_HARDWARE_TIER"

# ── SCN-061-014-10 / ADV-061-014-05 ──────────────────────────────────────────
scenario "SCN-061-014-10 — an accel-tier fixture is reported as skipped, not passed"

SANDBOX="$(make_sandbox tier_cpu)"
write_tier_fixture "$SANDBOX" test_tier_gated
write_plain_pass_fixture "$SANDBOX" test_plain_pass
run_sandbox_runner "$SANDBOX" cpu

assert_contains SCN-10-1 "SCN-061-014-10: the tier-gated fixture is reported as SKIP" \
  "$RUNNER_OUT" "SKIP: test_tier_gated"
assert_not_contains SCN-10-2 "SCN-061-014-10: it is NOT reported as PASS" \
  "$RUNNER_OUT" "PASS: test_tier_gated"
assert_not_contains SCN-10-3 "SCN-061-014-10: it is NOT reported as FAIL" \
  "$RUNNER_OUT" "FAIL: test_tier_gated"

# ADV-061-014-05: emitting a SKIP line while still counting the fixture as passed
# would leave the tally lying even though the label looks right. Assert the
# COUNT, not just the line — the plain fixture is the only pass in this sandbox.
assert_contains ADV-05-a "ADV-061-014-05: the passed count excludes the skipped fixture" \
  "$RUNNER_OUT" "Passed: 1"
assert_contains ADV-05-b "ADV-061-014-05: the skipped count includes it" \
  "$RUNNER_OUT" "Skipped: 1"
assert_contains ADV-05-c "ADV-061-014-05: failed count is 0" "$RUNNER_OUT" "Failed: 0"
assert_contains ADV-05-d "ADV-061-014-05: total reconciles to the three-way sum" \
  "$RUNNER_OUT" "Total:  2"
assert_not_contains ADV-05-e "ADV-061-014-05: the tier fixture's body did NOT execute" \
  "$RUNNER_OUT" "FIXTURE_BODY_EXECUTED"

# An optional tier skip must not redden the lane; that is the whole point of the
# tier gate. Requiredness is a separate, runner-side decision.
assert_eq SCN-10-4 "SCN-061-014-10: an optional tier skip keeps the suite exit 0" \
  "$RUNNER_STATUS" "0"

# ── Uniformity: both helpers resolve to the same reported outcome ────────────
scenario "SCOPE-02 goal — both skip helpers resolve to the same reported outcome"

REG_HELPERS="$E2E_DIR/assistant_regression/lib/regression_helpers.sh"
REG_EXIT="$(sed -n 's/^[[:space:]]*exit \([0-9]\+\)$/\1/p' "$REG_HELPERS" | tail -1)"
TIER_EXIT="$(awk '/^[[:space:]]*cpu\)/{f=1} f && /exit [0-9]+/{print $2; exit}' "$REAL_HELPERS")"
assert_eq UNIF-1 "both skip helpers exit with the same code" "$TIER_EXIT" "$REG_EXIT"
assert_eq UNIF-2 "and that code is the skip convention" "$TIER_EXIT" "$SKIP_EXIT"

echo ""
echo "========================================="
echo "  Tier-skip contract results"
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
