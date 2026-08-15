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

# --- 1. a bug that does not declare the compact packet is untouched ----------
d1="$WORK/plain"
mkdir -p "$d1"
printf '{"version":3,"status":"in_progress"}\n' >"$d1/state.json"
out="$(bash "$TARGET" "$d1" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | grep -q 'does not declare'; then
  ok "a bug without packet: micro passes untouched (the route stays opt-in)"
else
  bad "non-micro bug passes untouched" "exit $rc: $out"
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

# --- 9. the registry states the packet is NOT the default --------------------
# IMP-042 SCOPE-9 requires measurement before defaulting. If that caveat is ever
# dropped, the compact route silently becomes the norm unmeasured.
if grep -qi 'NOT THE DEFAULT' "$REGISTRY"; then
  ok "the registry records that the compact packet is not the default route"
else
  bad "registry records non-default status"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
