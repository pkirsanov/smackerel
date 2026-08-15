#!/usr/bin/env bash
# imp041-evaluation-corpus.sh — held-out incident corpus for the semantic
# boundary (IMP-041 SCOPE-8 / COV-13).
#
# WHY A CORPUS RATHER THAN MORE UNIT CASES
#
# Each SCOPE-1..7 selftest proves its own mechanism in isolation. None of them
# answers the question that actually matters: given the SHAPE of the incident
# this IMP was written from, does the assembled system refuse it — and does it
# still accept the legitimate work that looks superficially similar?
#
# A guard that refuses everything is as useless as one that refuses nothing, so
# every case below is a PAIR: an accepted plan and an adversarial rejected plan
# that differs from it in exactly the way that matters. False acceptance and
# false rejection are reported separately, because they are different failures.
#
# The corpus is repository-neutral. It builds its own sessions and contracts and
# touches no spec in this or any other repository.
#
# Exit codes
#   0  no false acceptance and no false rejection
#   1  at least one case behaved wrongly
#   2  usage or runtime error
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GC="$SCRIPT_DIR/goal-contract.sh"
EA="$SCRIPT_DIR/expansion-approval.sh"
CM="$SCRIPT_DIR/convergence-materiality.sh"
RECEIPT="$SCRIPT_DIR/goal-boundary-receipt.sh"
LINT="$SCRIPT_DIR/scenario-compile-lint.sh"

command -v jq >/dev/null 2>&1 || { echo "imp041-evaluation-corpus: jq is required" >&2; exit 2; }
for required in "$GC" "$EA" "$CM" "$RECEIPT"; do
  [[ -f "$required" ]] || { echo "imp041-evaluation-corpus: missing $required" >&2; exit 2; }
done

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

CASES=0
FALSE_ACCEPT=0
FALSE_REJECT=0
START="$(date +%s)"

# record <case> <expectation:accept|reject> <actual-rc> <what>
record() {
  local name="$1" want="$2" rc="$3" what="$4"
  CASES=$((CASES + 1))
  if [[ "$want" == "accept" ]]; then
    if [[ "$rc" -eq 0 ]]; then
      printf '  accept  %-58s ok\n' "$name"
    else
      printf '  accept  %-58s FALSE REJECTION (rc=%s) %s\n' "$name" "$rc" "$what"
      FALSE_REJECT=$((FALSE_REJECT + 1))
    fi
  else
    if [[ "$rc" -ne 0 ]]; then
      printf '  reject  %-58s ok\n' "$name"
    else
      printf '  reject  %-58s FALSE ACCEPTANCE %s\n' "$name" "$what"
      FALSE_ACCEPT=$((FALSE_ACCEPT + 1))
    fi
  fi
}

rc_of() { set +e; "$@" >/dev/null 2>&1; local r=$?; set -e; printf '%s' "$r"; }

# freeze <name> <shape> [extra goal-contract args...] -> session path
freeze() {
  local name="$1" shape="$2"; shift 2
  local d="$TMP/$name"
  mkdir -p "$d"
  printf 'operator request for %s\n' "$name" > "$d/request.txt"
  bash "$GC" freeze --session-file "$d/session.json" --source-request-file "$d/request.txt" \
    --intent "$name" --success-signal "the declared signal is demonstrated" \
    --runner bubbles.goal --session-id "$name" --repository-alias bubbles \
    --target repository=bubbles --repository-root bubbles \
    --execution-shape "$shape" ${1+"$@"} >/dev/null 2>&1
  printf '%s' "$d/session.json"
}

echo "============================================================"
echo "  IMP-041 EVALUATION CORPUS (SCOPE-8 / COV-13)"
echo "============================================================"
echo

# --- CASE 1 -----------------------------------------------------------------
# Evaluate an already-installed model through existing settings and tests.
# This is the ORIGINAL goal, and it must sail through untouched.
echo "case 1 — bounded evaluation through existing settings"
S1="$(freeze c1-eval one-off --allow-change-class existing-config --allow-change-class existing-test --delta-budget maxNewFiles=3)"
record "1a bounded evaluation needs no expansion approval" accept \
  "$(rc_of bash "$EA" verify --session-file "$S1" --planned-delta '{"changeClasses":["existing-config","existing-test"],"maxNewFiles":2}')" \
  "the original goal must not be obstructed"
record "1b its pre-planning boundary yields a receipt" accept \
  "$(rc_of bash "$RECEIPT" emit --boundary pre-planning --session-file "$S1")" ""

# --- CASE 2 -----------------------------------------------------------------
# THE INCIDENT. The same evaluation expands into virtualization, runners,
# caches and certification authorities. It must fail BEFORE any planning
# dispatch — that is the whole point of a pre-planning gate.
echo
echo "case 2 — the same evaluation overbuilt into a platform (THE INCIDENT)"
OVERBUILT='{"changeClasses":["new-virtual-machine","new-runner","new-cache","new-approval-authority"],"maxNewFiles":40,"maxNewScopes":9}'
record "2a overbuilt evaluation is refused before planning" reject \
  "$(rc_of bash "$EA" verify --session-file "$S1" --planned-delta "$OVERBUILT")" \
  "an unapproved platform build was admitted"

# The brake compares against a baseline, so the bounded plan must establish one
# first. This ordering is deliberate: the FIRST iteration is necessarily
# accepted (it defines the baseline), which is precisely why the pre-planning
# expansion gate above — not the brake — is what must catch an overbuilt start.
bash "$CM" check --session-file "$S1" --iteration 1 \
  --planned-delta '{"changeClasses":["existing-config","existing-test"],"maxNewFiles":2}' >/dev/null 2>&1
record "2b after a bounded baseline, the overbuild cannot proceed" reject \
  "$(rc_of bash "$CM" check --session-file "$S1" --iteration 2 --planned-delta "$OVERBUILT")" \
  "the convergence brake did not hold"
record "2c a narrower follow-up iteration still proceeds" accept \
  "$(rc_of bash "$CM" check --session-file "$S1" --iteration 3 --planned-delta '{"changeClasses":["existing-test"],"maxNewFiles":1}')" \
  "the brake blocked legitimate narrowing"

# --- CASE 3 -----------------------------------------------------------------
# Repair one bug inside an existing adapter. Legitimate, bounded, and it must
# not be dragged into foundation-shaped requirements.
echo
echo "case 3 — repair one bug inside an existing adapter"
S3="$(freeze c3-repair existing-capability-change --allow-change-class new-product-code --allow-change-class existing-test --delta-budget maxNewFiles=4)"
record "3a a bounded repair needs no expansion approval" accept \
  "$(rc_of bash "$EA" verify --session-file "$S3" --planned-delta '{"changeClasses":["new-product-code","existing-test"],"maxNewFiles":3}')" \
  "ordinary repair work must not be blocked"
record "3b the same repair may not stand up a second foundation" reject \
  "$(rc_of bash "$EA" verify --session-file "$S3" --planned-delta '{"changeClasses":["new-shared-library","new-workflow"],"maxNewFiles":3}')" \
  "a parallel foundation was admitted under a repair"

# --- CASE 4 -----------------------------------------------------------------
# A genuine second provider that really does need a reusable foundation. The
# system must ALLOW this once it is declared — a guard that refuses legitimate
# platform work would simply be routed around.
echo
echo "case 4 — a genuine second provider that needs a reusable foundation"
S4="$(freeze c4-provider reusable-capability --allow-change-class new-shared-library --allow-change-class new-product-code --delta-budget maxNewFiles=25 --delta-budget maxNewScopes=4)"
record "4a declared reusable-capability work proceeds" accept \
  "$(rc_of bash "$EA" verify --session-file "$S4" --planned-delta '{"changeClasses":["new-shared-library","new-product-code"],"maxNewFiles":20,"maxNewScopes":3}')" \
  "declared foundation work must not be blocked"
record "4b it still cannot silently add a virtual machine" reject \
  "$(rc_of bash "$EA" verify --session-file "$S4" --planned-delta '{"changeClasses":["new-shared-library","new-virtual-machine"],"maxNewFiles":20}')" \
  "an undeclared class rode in on an approved shape"

# --- CASE 5 -----------------------------------------------------------------
# An independent infrastructure finding. Real, worth fixing, NOT this goal.
echo
echo "case 5 — route an independent infrastructure finding"
if [[ -f "$LINT" ]]; then
  mk_scenario() { # mk_scenario <impact> <file>
    jq -n --arg impact "$1" '{
      version: 1, scenarioId: "corpus-c5",
      rootOutcome: { intent: "evaluate", successSignal: "the suite reports a score",
                     hardConstraints: ["no new infrastructure"], failureCondition: "blocked" },
      repos: [{id: "bubbles", role: "product"}],
      nodes: [{ id: "n1", type: "delivery", repo: "bubbles", mode: "full-delivery",
                originFinding: {id: "F-9", reportedBy: "bubbles.audit"},
                goalImpact: $impact, impactDecidedBy: "bubbles.goal",
                contributesTo: ["successSignal"] }]
    }' > "$2"
  }
  mk_scenario independent "$TMP/c5-independent.json"
  record "5a an independent finding cannot join the current DAG" reject \
    "$(rc_of bash "$LINT" "$TMP/c5-independent.json" "$SCRIPT_DIR/../..")" \
    "an unrelated finding was admitted into the goal"
  mk_scenario required "$TMP/c5-required.json"
  # A required finding is admissible in principle; the lint may still object to
  # unrelated scenario shape, so this arm only asserts it is not rejected FOR
  # BEING A FINDING.
  set +e
  out5="$(bash "$LINT" "$TMP/c5-required.json" "$SCRIPT_DIR/../.." 2>&1)"
  set -e
  if printf '%s' "$out5" | grep -q 'independent'; then
    record "5b a required finding is not refused as independent" accept 1 "misclassified"
  else
    record "5b a required finding is not refused as independent" accept 0 ""
  fi
else
  echo "  (scenario-compile-lint.sh unavailable — case 5 skipped)"
fi

# --- CASE 6 -----------------------------------------------------------------
# Resume a compacted goal without changing its approved delta. Resumption must
# not become a laundering step for extra scope.
echo
echo "case 6 — resume a compacted goal"
S6="$(freeze c6-resume one-off --allow-change-class existing-test --delta-budget maxNewFiles=5)"
RESUME='{"changeClasses":["existing-test"],"maxNewFiles":4}'
bash "$CM" check --session-file "$S6" --iteration 1 --planned-delta "$RESUME" >/dev/null 2>&1
record "6a resuming with the same delta proceeds" accept \
  "$(rc_of bash "$CM" check --session-file "$S6" --iteration 2 --planned-delta "$RESUME")" \
  "a faithful resume was blocked"
record "6b resuming with MORE scope is refused" reject \
  "$(rc_of bash "$CM" check --session-file "$S6" --iteration 3 --planned-delta '{"changeClasses":["existing-test","new-runner"],"maxNewFiles":4}')" \
  "resumption laundered extra scope"
record "6c a post-compaction receipt is still obtainable" accept \
  "$(rc_of bash "$RECEIPT" emit --boundary pre-planning --session-file "$S6")" ""

# --- CASE 7 -----------------------------------------------------------------
# Legacy compatibility: a v1 contract must remain readable throughout the
# migration, or every already-frozen goal in the field breaks at once.
echo
echo "case 7 — legacy v1 contract remains readable"
D7="$TMP/c7"; mkdir -p "$D7"; printf 'legacy\n' > "$D7/request.txt"
bash "$GC" freeze --session-file "$D7/session.json" --source-request-file "$D7/request.txt" \
  --intent "legacy goal" --success-signal "signal" --runner bubbles.goal \
  --session-id c7-legacy --repository-alias bubbles \
  --target repository=bubbles --repository-root bubbles >/dev/null 2>&1
record "7a a v1 contract still verifies" accept \
  "$(rc_of bash "$GC" verify --session-file "$D7/session.json")" \
  "the migration broke every contract already in the field"
record "7b a v1 goal with no semantic boundary needs no approval" accept \
  "$(rc_of bash "$EA" verify --session-file "$D7/session.json" --planned-delta '{"changeClasses":["existing-test"]}')" ""

ELAPSED=$(( $(date +%s) - START ))
echo
echo "------------------------------------------------------------"
printf '  cases evaluated        : %d\n' "$CASES"
printf '  FALSE ACCEPTANCE       : %d\n' "$FALSE_ACCEPT"
printf '  FALSE REJECTION        : %d\n' "$FALSE_REJECT"
printf '  runtime cost           : %ds wall clock\n' "$ELAPSED"
echo "------------------------------------------------------------"

if [[ "$FALSE_ACCEPT" -gt 0 || "$FALSE_REJECT" -gt 0 ]]; then
  echo "imp041-evaluation-corpus: FAILED — the assembled system did not behave as specified." >&2
  exit 1
fi
echo "imp041-evaluation-corpus: PASS — no false acceptance, no false rejection across 7 incident shapes."
exit 0
