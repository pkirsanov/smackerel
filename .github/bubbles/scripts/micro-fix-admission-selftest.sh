#!/usr/bin/env bash
# bubbles/scripts/micro-fix-admission-selftest.sh
#
# Hermetic selftest for micro-fix-admission.sh (IMP-042 SCOPE-9).
#
# The load-bearing property is NOT "it accepts a tidy bug". It is that the
# compact packet CANNOT be used for the defects that look small and are not:
# case 3 (a payment-surface fix) and case 6 (a regression test that only ever
# passed) are the two that keep proportionality from becoming a loophole.
#
# Usage: bash bubbles/scripts/micro-fix-admission-selftest.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/micro-fix-admission.sh"
REGISTRY="$SCRIPT_DIR/../registry/micro-fix-packet.yaml"
NAME="micro-fix-admission-selftest"

failures=0
checks=0
ok() {
  checks=$((checks + 1))
  printf '  ok   %s\n' "$1"
}
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

[[ -f "$TARGET" ]] || {
  printf '%s: SKIP (target missing)\n' "$NAME"
  exit 0
}
[[ -f "$REGISTRY" ]] || {
  printf '%s: SKIP (registry missing)\n' "$NAME"
  exit 0
}

WORK="$(mktemp -d 2>/dev/null)" || {
  printf '%s: cannot create temp dir\n' "$NAME" >&2
  exit 1
}
# shellcheck disable=SC2317  # invoked indirectly by the EXIT/INT/TERM trap
cleanup() { [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

# Build a bug dir that conforms, then mutate copies of it per case. Starting
# from a PASSING fixture is deliberate: it proves each refusal is caused by the
# one thing the case changed, not by fixture sloppiness.
all_yes_no() {
  # Every condition answered the admitted way.
  cat <<'EOF'
# Bug: [BUG-001] Off-by-one in retry ceiling

## Root Cause
The retry loop compared with <= instead of <, so it ran one extra attempt.

micro-fix-admission: no-new-behavior = no
micro-fix-admission: no-schema-change = no
micro-fix-admission: no-auth-surface = no
micro-fix-admission: no-payment-surface = no
micro-fix-admission: no-secret-surface = no
micro-fix-admission: no-deployment-surface = no
micro-fix-admission: no-cross-product-effect = no
micro-fix-admission: contract-preserving = yes
EOF
}

good_report() {
  cat <<'EOF'
## Reproduction
Reproduced the defect before any change.

    $ bash retry-test.sh
    attempts=4 expected=3
    exit code: 1

## Regression
The regression test fails without the fix and passes with the fix.

    before fix: exit code: 1
    after fix:  exit code: 0
EOF
}

make_bug() {
  local dir="$1"
  mkdir -p "$dir"
  printf '{"version":3,"packet":"micro","status":"in_progress"}\n' >"$dir/state.json"
  all_yes_no >"$dir/bug.md"
  good_report >"$dir/report.md"
}

# --- 1. a bug that answers no admission condition uses the full packet -------
# The default route resolves from the ANSWERS, so a bug nobody assessed is not
# silently pulled onto the compact route.
d1="$WORK/plain"
mkdir -p "$d1"
printf '{"version":3,"status":"in_progress"}\n' >"$d1/state.json"
out="$(bash "$TARGET" "$d1" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | grep -q 'answers no admission condition'; then
  ok "an unassessed bug uses the full packet and is passed untouched"
else
  bad "unassessed bug uses the full packet" "exit $rc: $out"
fi

# --- 1b. ADVERSARIAL: the compact packet is the DEFAULT, not an opt-in --------
# This is the activation case (IMP-047 S-D). The bug answers all 8 conditions
# admissibly and declares NO packet. Before activation it passed untouched with
# "does not declare the micro-fix packet"; now it must be admitted to the
# compact route by default. If activation is reverted, this case fails.
d1b="$WORK/default-compact"
mkdir -p "$d1b"
printf '{"version":3,"status":"in_progress"}\n' >"$d1b/state.json"
all_yes_no >"$d1b/bug.md"
good_report >"$d1b/report.md"
out="$(bash "$TARGET" "$d1b" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]] &&
  printf '%s' "$out" | grep -q 'COMPACT packet is the default route' &&
  printf '%s' "$out" | grep -q 'admitted'; then
  ok "a bug passing all 8 admission questions uses the compact packet BY DEFAULT"
else
  bad "compact packet is the default route" "exit $rc: $out"
fi

# --- 1c. ADVERSARIAL: a failed condition escalates with no override ----------
# Same undeclared bug, one payment answer flipped. Escalation must be automatic
# and must not require anyone to decide.
d1c="$WORK/default-escalate"
mkdir -p "$d1c"
printf '{"version":3,"status":"in_progress"}\n' >"$d1c/state.json"
all_yes_no >"$d1c/bug.md"
good_report >"$d1c/report.md"
sed -e 's/no-payment-surface = no/no-payment-surface = yes/' "$d1c/bug.md" >"$d1c/bug.new" &&
  mv "$d1c/bug.new" "$d1c/bug.md"
out="$(bash "$TARGET" "$d1c" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]] &&
  printf '%s' "$out" | grep -q 'escalates automatically to the full bug packet' &&
  printf '%s' "$out" | grep -q 'no reviewer discretion and no override flag'; then
  ok "a failed admission condition escalates automatically, with no override"
else
  bad "automatic escalation with no override" "exit $rc: $out"
fi

# --- 1d. an explicit packet: full is honoured --------------------------------
d1d="$WORK/explicit-full"
mkdir -p "$d1d"
printf '{"version":3,"packet":"full","status":"in_progress"}\n' >"$d1d/state.json"
all_yes_no >"$d1d/bug.md"
good_report >"$d1d/report.md"
out="$(bash "$TARGET" "$d1d" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | grep -q 'declares packet: full'; then
  ok "an explicit packet: full declaration keeps the bug on the full packet"
else
  bad "explicit full packet honoured" "exit $rc: $out"
fi

# --- 2. a conforming compact packet is admitted ------------------------------
d2="$WORK/good"
make_bug "$d2"
out="$(bash "$TARGET" "$d2" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | grep -q 'admitted'; then
  ok "a conforming compact packet is admitted"
else
  bad "conforming compact packet admitted" "exit $rc: $out"
fi

# --- 3. ADVERSARIAL: a payment-surface fix must escalate ---------------------
# This is the case the whole guard exists for. A payment defect can be one
# character of diff and still move money.
d3="$WORK/payment"
make_bug "$d3"
sed -e 's/no-payment-surface = no/no-payment-surface = yes/' "$d3/bug.md" >"$d3/bug.new" &&
  mv "$d3/bug.new" "$d3/bug.md"
out="$(bash "$TARGET" "$d3" 2>&1)"
rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s' "$out" | grep -q 'no-payment-surface'; then
  ok "a payment-surface fix is refused the compact packet and escalated"
else
  bad "payment-surface fix escalates" "exit $rc: $out"
fi

# --- 4. an UNANSWERED condition is a refusal, not a default ------------------
# Silence is how the inconvenient question gets skipped.
d4="$WORK/silent"
make_bug "$d4"
grep -v 'no-auth-surface' "$d4/bug.md" >"$d4/bug.new" && mv "$d4/bug.new" "$d4/bug.md"
out="$(bash "$TARGET" "$d4" 2>&1)"
rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s' "$out" | grep -q 'unanswered'; then
  ok "an unanswered admission condition is refused, never defaulted to admit"
else
  bad "unanswered condition is refused" "exit $rc: $out"
fi

# --- 5. the compact packet still requires its artifacts ----------------------
d5="$WORK/noreport"
make_bug "$d5"
rm -f "$d5/report.md"
out="$(bash "$TARGET" "$d5" 2>&1)"
rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s' "$out" | grep -q 'missing-artifact'; then
  ok "a compact packet without report.md is refused (fewer artifacts, never zero)"
else
  bad "missing artifact refused" "exit $rc: $out"
fi

# --- 6. ADVERSARIAL: a regression that only ever PASSED is refused -----------
# A test green after the fix proves the suite is green, not that it would have
# caught the defect.
d6="$WORK/greenonly"
make_bug "$d6"
printf '## Reproduction\nReproduced it.\n\n## Regression\nafter fix: exit code: 0\n' >"$d6/report.md"
out="$(bash "$TARGET" "$d6" 2>&1)"
rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s' "$out" | grep -q 'failing WITHOUT the fix'; then
  ok "a regression test that never failed without the fix is refused"
else
  bad "one-sided regression refused" "exit $rc: $out"
fi

# --- 7. no bypass flag exists ------------------------------------------------
out="$(bash "$TARGET" --force "$d3" 2>&1)"
rc=$?
if [[ "$rc" -eq 2 ]] && printf '%s' "$out" | grep -q 'no bypass'; then
  ok "a bypass-shaped flag is rejected by name"
else
  bad "bypass flag rejected" "exit $rc: $out"
fi

# --- 8. the guard READS the registry, it does not restate it -----------------
# Add a condition to a copied registry; the guard must start demanding it. If
# this fails, the admission list has been duplicated in the script and the two
# copies will drift.
alt_root="$WORK/altfw"
mkdir -p "$alt_root/scripts" "$alt_root/registry"
cp "$TARGET" "$alt_root/scripts/"
# Insert a new condition into the admission block of the COPY.
awk '
  /^admission:/{print;print "  - id: no-telemetry-surface";print "    question: Does it touch telemetry?";print "    admitWhen: \"no\"";print "    reason: fixture";next}
  {print}
' "$REGISTRY" >"$alt_root/registry/micro-fix-packet.yaml"
out="$(bash "$alt_root/scripts/micro-fix-admission.sh" "$d2" 2>&1)"
rc=$?
if [[ "$rc" -eq 1 ]] && printf '%s' "$out" | grep -q 'no-telemetry-surface'; then
  ok "adding a registry condition changes the guard (single source, not a copy)"
else
  bad "guard reads the registry" "exit $rc: $out"
fi

# --- 9. the registry records that activation did not change the contract ----
# IMP-047 S-D activated the compact route as the DEFAULT. The one thing
# activation must NOT have done is widen the window, so the assurance floor and
# the absence of an override are pinned here by name. If a future edit adds an
# override flag or drops an obligation, this case fails.
if grep -q 'overrideFlag: none' "$REGISTRY"; then
  ok "escalation still carries overrideFlag: none"
else
  bad "escalation has no override flag"
fi

registry_condition_count="$(bash "$TARGET" --registry | awk '/^admission:/{a=1;next} /^[a-z]/{a=0} a && /^  - id:/{c++} END{print c+0}')"
if [[ "$registry_condition_count" -eq 8 ]]; then
  ok "the 8 admission conditions are intact after activation"
else
  bad "8 admission conditions intact" "found $registry_condition_count"
fi

registry_obligation_count="$(bash "$TARGET" --registry | awk '/^preservedObligations:/{a=1;next} /^[a-z]/{a=0} a && /^  - id:/{c++} END{print c+0}')"
if [[ "$registry_obligation_count" -eq 4 ]]; then
  ok "the 4 preserved obligations are intact after activation"
else
  bad "4 preserved obligations intact" "found $registry_obligation_count"
fi

registry_artifact_count="$(bash "$TARGET" --registry | awk '/^requiredArtifacts:/{a=1;next} /^[a-z]/{a=0} a && /^  - /{c++} END{print c+0}')"
if [[ "$registry_artifact_count" -eq 3 ]]; then
  ok "the 3 required artifacts are intact after activation"
else
  bad "3 required artifacts intact" "found $registry_artifact_count"
fi

if grep -qi 'THE DEFAULT ROUTE SINCE IMP-047' "$REGISTRY" && ! grep -q 'NOT THE DEFAULT' "$REGISTRY"; then
  ok "the registry records the compact packet as the default route"
else
  bad "registry records default-route status"
fi

# --- 10. defect escape is recorded FORWARD, not required beforehand ---------
# The measurement that used to be a precondition is now an outcome. It must
# exist as a producer, and it must be honest about an empty denominator rather
# than reporting a rate from a sample of zero.
logger="$(dirname "$TARGET")/micro-fix-outcome-log.sh"
if [[ -f "$logger" ]]; then
  fresh_root="$WORK/outcome-repo"
  mkdir -p "$fresh_root"
  bash "$logger" route --bug "$d1b" --route compact --resolution default --repo-root "$fresh_root" >/dev/null 2>&1
  bash "$logger" escape --bug "$d1b" --reopened-as BUG-999 --repo-root "$fresh_root" >/dev/null 2>&1
  rep="$(bash "$logger" report --repo-root "$fresh_root" --json 2>&1)"
  if printf '%s' "$rep" | grep -q '"compactRoutes":1' && printf '%s' "$rep" | grep -q '"escapes":1'; then
    ok "defect escape is recorded going forward, with its denominator"
  else
    bad "forward defect-escape measurement" "$rep"
  fi

  empty_root="$WORK/outcome-empty"
  mkdir -p "$empty_root"
  rep_empty="$(bash "$logger" report --repo-root "$empty_root" 2>&1)"
  if printf '%s' "$rep_empty" | grep -q 'no denominator'; then
    ok "an escape rate with no denominator refuses to be quoted"
  else
    bad "empty denominator is reported honestly" "$rep_empty"
  fi
else
  bad "micro-fix outcome logger exists" "not found: $logger"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
