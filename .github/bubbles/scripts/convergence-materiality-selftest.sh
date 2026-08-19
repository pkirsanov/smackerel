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

# ===========================================================================
# IMP-048 SCOPE-8 / GF-15 — ad-hoc session binding
#
# The goal-contract brake above only ever fires inside `autonomous-goal`. These
# cases prove the SAME brake binds when a session that opened as a question
# grows into a delivery scope — and, first and above everything else, that it
# never binds on a READ.
# ===========================================================================

SID="sess-scope8"

# A repo whose config OPTS IN. Default is off; that default is asserted in X6.
adhoc_repo() {
  local d="$TMP/$1"
  mkdir -p "$d/.github" "$d/.specify/memory" "$d/docs" "$d/bubbles/scripts"
  printf 'adHocMateriality:\n  adapter: session-json\n' > "$d/.github/bubbles-project.yaml"
  printf '{}\n' > "$d/.specify/memory/bubbles.session.json"
  printf 'notes\n' > "$d/docs/raid-activity.md"
  printf '#!/usr/bin/env bash\ntrue\n' > "$d/bubbles/scripts/containment.sh"
  printf '%s' "$d"
}

store_of() { printf '%s' "$1/.specify/memory/bubbles.session.json"; }

open_surface() {
  bash "$CM" open-surface --session-file "$(store_of "$1")" --repo-root "$1" \
    --session-id "$SID" --surface docs --note "read two days of RAID activity" >/dev/null 2>&1
}

guard() {
  local repo="$1" kind="$2" target="$3"
  bash "$CM" guard --session-file "$(store_of "$repo")" --repo-root "$repo" \
    --session-id "$SID" --action-kind "$kind" --target "$target"
}

# --- X1. THE LOAD-BEARING SAFETY PROPERTY (R7) ---------------------------
# ASSERTED FIRST AND EXPLICITLY. A read-only investigation OUTSIDE the opening
# surface is NEVER refused. Reading is how anyone discovers the work is bigger
# than they thought; a brake that refuses exploration is switched off on day one
# and then protects nothing. If this case ever goes red, the control is worse
# than useless and must not ship.
R1="$(adhoc_repo x1)"
open_surface "$R1"
expect 0 "$(rc_of guard "$R1" read bubbles/scripts/containment.sh)" \
  "X1 R7: a READ far outside the opening surface is ALLOWED, never refused"
if printf '%s' "$(guard "$R1" read bubbles/scripts/containment.sh 2>&1)" | grep -q 'reason=read-only'; then
  ok "X1b the read is allowed for being a read, before the surface is consulted at all"
else
  bad "X1b read-only reason" "$(guard "$R1" read bubbles/scripts/containment.sh 2>&1)"
fi
expect 0 "$(rc_of guard "$R1" read libs/nowhere/never-created.md)" \
  "X1c a read OUTSIDE the surface of a path that does not even exist is still ALLOWED"

# --- X2. a mutable action INSIDE the opening surface proceeds -------------
expect 0 "$(rc_of guard "$R1" mutable docs/raid-activity.md)" \
  "X2 a MUTABLE action inside the opening surface is allowed"

# --- X3. a mutable action OUTSIDE the opening surface is refused ----------
expect 1 "$(rc_of guard "$R1" mutable bubbles/scripts/containment.sh)" \
  "X3 the first MUTABLE action outside the opening surface is REFUSED"

set +e
x3=$(guard "$R1" mutable bubbles/scripts/containment.sh 2>&1)
set -e
if printf '%s' "$x3" | grep -q 'NEW GOAL, not a fixable obstacle' \
  && printf '%s' "$x3" | grep -q 'declare-boundary'; then
  ok "X3b the refusal reuses the brake's framing and names the two honest ways forward"
else
  bad "X3b refusal content" "$x3"
fi

# --- X4. after an explicit declaration the same action proceeds -----------
bash "$CM" declare-boundary --session-file "$(store_of "$R1")" --repo-root "$R1" \
  --session-id "$SID" --target bubbles/scripts/containment.sh \
  --note "operator widened: containment hardening is now in scope" >/dev/null 2>&1
expect 0 "$(rc_of guard "$R1" mutable bubbles/scripts/containment.sh)" \
  "X4 after an explicit boundary declaration the same mutable action proceeds"

# --- X5. no recorded opening surface => NO-OP, never a refusal ------------
# The surface is DECLARED, never inferred from the request text. With none
# recorded there is nothing to be outside of, and refusing on a guessed intent
# would be worse than not binding at all.
R5="$(adhoc_repo x5)"
expect 0 "$(rc_of guard "$R5" mutable bubbles/scripts/containment.sh)" \
  "X5 with NO recorded opening surface the brake is a no-op, not a refusal"
if printf '%s' "$(guard "$R5" mutable bubbles/scripts/containment.sh 2>&1)" | grep -q 'reason=no-opening-surface'; then
  ok "X5b the no-op says WHY it did not bind rather than reporting a pass"
else
  bad "X5b no-op reason" "$(guard "$R5" mutable bubbles/scripts/containment.sh 2>&1)"
fi

# --- X6. unconfigured repo => no-op, exit 0 ------------------------------
R6="$TMP/x6"
mkdir -p "$R6/.specify/memory" "$R6/bubbles/scripts"
printf '{}\n' > "$R6/.specify/memory/bubbles.session.json"
printf 'x\n' > "$R6/bubbles/scripts/containment.sh"
bash "$CM" open-surface --session-file "$(store_of "$R6")" --repo-root "$R6" \
  --session-id "$SID" --surface docs >/dev/null 2>&1
expect 0 "$(rc_of guard "$R6" mutable bubbles/scripts/containment.sh)" \
  "X6 an UNCONFIGURED repo is a clean no-op — default OFF, exit 0"
if printf '%s' "$(guard "$R6" mutable bubbles/scripts/containment.sh 2>&1)" | grep -q 'verdict=SKIPPED'; then
  ok "X6b the unconfigured verdict is SKIPPED, not PASS — 'we did not look' is not 'it was fine'"
else
  bad "X6b unconfigured verdict" "$(guard "$R6" mutable bubbles/scripts/containment.sh 2>&1)"
fi

# --- X7. ADVERSARIAL: no retroactive self-grant --------------------------
# The abuse this has to survive: mutate first, then declare the boundary to
# authorise what already happened. A declaration widens what MAY happen, never
# what DID, so a target whose bytes moved since the refusal cannot be laundered.
R7="$(adhoc_repo x7)"
open_surface "$R7"
guard "$R7" mutable bubbles/scripts/containment.sh >/dev/null 2>&1 || true
printf '#!/usr/bin/env bash\n# the expansion already landed\nfalse\n' > "$R7/bubbles/scripts/containment.sh"
expect 1 "$(rc_of bash "$CM" declare-boundary --session-file "$(store_of "$R7")" --repo-root "$R7" \
  --session-id "$SID" --target bubbles/scripts/containment.sh --note "post-hoc")" \
  "X7 ADVERSARIAL a boundary cannot be self-granted AFTER the mutation already landed"

set +e
x7=$(bash "$CM" declare-boundary --session-file "$(store_of "$R7")" --repo-root "$R7" \
  --session-id "$SID" --target bubbles/scripts/containment.sh --note "post-hoc" 2>&1)
set -e
if printf '%s' "$x7" | grep -q 'already landed'; then
  ok "X7b the refusal names the laundering rather than reporting a generic denial"
else
  bad "X7b retroactive refusal content" "$x7"
fi

# --- X8. sessions do not inherit each other's boundary -------------------
# The surface is keyed by HOST SESSION ID for the same reason SCOPE-7 keys
# snapshots that way: this repository genuinely runs concurrent sessions. The
# first session declared `bubbles/scripts/containment.sh` into its surface in X4;
# a second session in the SAME store must not be widened by that.
bash "$CM" open-surface --session-file "$(store_of "$R1")" --repo-root "$R1" \
  --session-id "sess-other" --surface docs --note "a different question" >/dev/null 2>&1
expect 1 "$(rc_of bash "$CM" guard --session-file "$(store_of "$R1")" --repo-root "$R1" \
  --session-id "sess-other" --action-kind mutable --target bubbles/scripts/containment.sh)" \
  "X8 a concurrent session is NOT widened by the boundary the first session declared"
expect 0 "$(rc_of bash "$CM" guard --session-file "$(store_of "$R1")" --repo-root "$R1" \
  --session-id "sess-other" --action-kind mutable --target docs/raid-activity.md)" \
  "X8b and its own surface still admits its own work"

# --- X9. no bypass on the ad-hoc path ------------------------------------
set +e
b1="$(bash "$CM" guard --session-file "$(store_of "$R1")" --session-id "$SID" --retroactive 2>&1)"; r1=$?
bash "$CM" guard --session-file "$(store_of "$R1")" --repo-root "$R1" --session-id "$SID" \
  --action-kind maybe --target docs >/dev/null 2>&1; r2=$?
bash "$CM" declare-boundary --session-file "$(store_of "$R1")" --repo-root "$R1" \
  --session-id "$SID" --target docs/x >/dev/null 2>&1; r3=$?
set -e
if [[ "$r1" -eq 2 && "$r2" -eq 2 && "$r3" -eq 2 ]] && printf '%s' "$b1" | grep -q 'bypass-shaped'; then
  ok "X9 --retroactive, an unknown --action-kind and a note-less declaration all exit 2"
else
  bad "X9 ad-hoc bypass surface" "retroactive=$r1 unknownKind=$r2 noNote=$r3"
fi

# --- X10. NO REGRESSION: the autonomous-goal binding is unchanged --------
# The ad-hoc subcommands must not have altered the contract path. Same sequence
# as P1/P2/A1 above, run against a fresh session in a repo that opts INTO the
# ad-hoc brake: the config must have no effect on `check` whatsoever.
SR="$(new_session x10)"
expect 0 "$(rc_of bash "$CM" check --session-file "$SR" --iteration 1 --planned-delta "$BASE")" \
  "X10 no regression: the first goal-contract iteration still records a baseline"
expect 0 "$(rc_of bash "$CM" check --session-file "$SR" --iteration 2 --planned-delta "$BASE")" \
  "X10b no regression: an identical iteration still proceeds"
expect 1 "$(rc_of bash "$CM" check --session-file "$SR" --iteration 3 --planned-delta '{"changeClasses":["existing-test","new-workflow"],"maxNewFiles":3}')" \
  "X10c no regression: growth on the contract path is still refused"
expect 0 "$(rc_of bash "$CM" show --session-file "$SR")" \
  "X10d no regression: show still reports the recorded baseline"

printf 'convergence-materiality-selftest: %s check(s), %s failure(s)\n' "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
