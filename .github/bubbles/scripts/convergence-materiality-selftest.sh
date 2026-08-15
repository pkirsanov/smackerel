#!/usr/bin/env bash
# Hermetic selftest for convergence-materiality.sh (IMP-041 SCOPE-7 / GF-13).
#
# The guarantee: persistence may push through DIFFICULTY but never through
# GROWTH. `neverStopForFixableObstacles` and solution search are correct for
# their purpose, and neither distinguishes "this is hard" from "this is bigger".
# These cases prove the brake makes that distinction mechanically, and that it
# cannot be released from inside the loop.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CM="$SCRIPT_DIR/convergence-materiality.sh"
GC="$SCRIPT_DIR/goal-contract.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM
checks=0
failures=0
ok() { checks=$((checks + 1)); echo "  ok   $1"; }
bad() { checks=$((checks + 1)); failures=$((failures + 1)); echo "  FAIL $1"; echo "       $2"; }
expect() { if [[ "$2" -eq "$1" ]]; then ok "$3"; else bad "$3" "rc=$2 (wanted $1)"; fi; }
rc_of() { set +e; "$@" >/dev/null 2>&1; local r=$?; set -e; printf '%s' "$r"; }

command -v jq >/dev/null 2>&1 || { echo "convergence-materiality-selftest: SKIP (jq not installed)"; exit 0; }
[[ -f "$CM" ]] || { echo "FAIL: $CM not found" >&2; exit 1; }

BASE='{"changeClasses":["existing-test"],"maxNewFiles":3}'

new_session() {
  local d="$TMP/$1"
  mkdir -p "$d"
  printf 'evaluate the installed model\n' > "$d/request.txt"
  bash "$GC" freeze --session-file "$d/session.json" --source-request-file "$d/request.txt" \
    --intent "evaluate the installed model" --success-signal "the suite reports a score" \
    --runner bubbles.goal --session-id "$1" --repository-alias bubbles \
    --target repository=bubbles --repository-root bubbles \
    --execution-shape one-off --allow-change-class existing-test >/dev/null 2>&1
  printf '%s' "$d/session.json"
}

# --- P1/P2/P3. difficulty is allowed through -------------------------------
S="$(new_session p1)"
expect 0 "$(rc_of bash "$CM" check --session-file "$S" --iteration 1 --planned-delta "$BASE")" \
  "P1 the first iteration records a baseline and proceeds"
expect 0 "$(rc_of bash "$CM" check --session-file "$S" --iteration 2 --planned-delta "$BASE")" \
  "P2 an identical second iteration proceeds"
expect 0 "$(rc_of bash "$CM" check --session-file "$S" --iteration 3 --planned-delta '{"changeClasses":["existing-test"],"maxNewFiles":1}')" \
  "P3 a NARROWER iteration proceeds — solution search doing its job"

# --- A1/A2/A3. growth is stopped, and named ------------------------------
expect 1 "$(rc_of bash "$CM" check --session-file "$S" --iteration 4 --planned-delta '{"changeClasses":["existing-test","new-workflow"],"maxNewFiles":3}')" \
  "A1 an iteration that adds a workflow is refused"
expect 1 "$(rc_of bash "$CM" check --session-file "$S" --iteration 5 --planned-delta '{"changeClasses":["existing-test"],"maxNewFiles":9}')" \
  "A2 an iteration that raises a count is refused"

set +e
out="$(bash "$CM" check --session-file "$S" --iteration 6 --planned-delta '{"changeClasses":["existing-test","new-runner"],"maxNewFiles":3}' 2>&1)"
set -e
if printf '%s' "$out" | grep -q 'new-runner' && printf '%s' "$out" | grep -q 'not a fixable obstacle'; then
  ok "A3 the refusal NAMES what grew and says growth is a new goal, not an obstacle"
else
  bad "A3 refusal content" "$out"
fi

# --- A4. ADVERSARIAL: a NEW dimension is growth, not a free pass ----------
# An absent baseline key must count as zero. Treating "not previously declared"
# as unconstrained would let every new budget dimension in unchecked.
S4="$(new_session a4)"
bash "$CM" check --session-file "$S4" --iteration 1 --planned-delta '{"changeClasses":["existing-test"]}' >/dev/null 2>&1
expect 1 "$(rc_of bash "$CM" check --session-file "$S4" --iteration 2 --planned-delta '{"changeClasses":["existing-test"],"maxNewVirtualMachines":1}')" \
  "A4 a dimension absent from the baseline counts as zero, so introducing it is growth"

# --- A5. ADVERSARIAL: the brake cannot be released from inside the loop ---
# Re-baselining at the same contract revision is exactly how an autonomous
# runner would free itself; only an approved revision may reset it.
expect 1 "$(rc_of bash "$CM" baseline --session-file "$S" --planned-delta '{"changeClasses":["existing-test","new-runner"],"maxNewFiles":99}')" \
  "A5 re-baselining without an approved revision is refused"

# --- P4. an approved revision legitimately resets the brake --------------
bash "$GC" revise --session-file "$S" --approval-note "operator widened the goal" \
  --execution-shape reusable-capability --allow-change-class existing-test \
  --allow-change-class new-workflow >/dev/null 2>&1
expect 0 "$(rc_of bash "$CM" check --session-file "$S" --iteration 7 --planned-delta '{"changeClasses":["existing-test","new-workflow"],"maxNewFiles":3}')" \
  "P4 an approved contract revision re-baselines and the wider plan proceeds"

# --- U1. usage + no bypass ------------------------------------------------
set +e
bash "$CM" >/dev/null 2>&1; u1=$?
bypass="$(bash "$CM" check --accept-growth 2>&1)"; u2=$?
bash "$CM" frobnicate >/dev/null 2>&1; u3=$?
set -e
if [[ "$u1" -eq 2 && "$u2" -eq 2 && "$u3" -eq 2 ]] && printf '%s' "$bypass" | grep -q 'bypass-shaped'; then
  ok "U1 no args, --accept-growth, and an unknown subcommand all exit 2"
else
  bad "U1 usage" "noargs=$u1 bypass=$u2 unknown=$u3"
fi

printf 'convergence-materiality-selftest: %s check(s), %s failure(s)\n' "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
