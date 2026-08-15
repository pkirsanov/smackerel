#!/usr/bin/env bash
# Hermetic selftest for goal-boundary-receipt.sh (IMP-041 SCOPE-3 / GF-7).
#
# The guarantee this script defends is narrow and load-bearing: a receipt may
# exist ONLY if the boundary actually passed. Every adversarial case below
# attacks a different way a caller could otherwise hold a receipt that does not
# describe a check that ran — a failed guard, another boundary, an edited body,
# a superseded revision, or a boundary that changed after the fact.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RECEIPT="$SCRIPT_DIR/goal-boundary-receipt.sh"
GC="$SCRIPT_DIR/goal-contract.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM
checks=0
failures=0
ok() { checks=$((checks + 1)); echo "  ok   $1"; }
bad() { checks=$((checks + 1)); failures=$((failures + 1)); echo "  FAIL $1"; echo "       $2"; }

if ! command -v jq >/dev/null 2>&1; then
  echo "goal-boundary-receipt-selftest: SKIP (jq not installed)"
  exit 0
fi
[[ -f "$RECEIPT" ]] || { echo "FAIL: $RECEIPT not found" >&2; exit 1; }
[[ -f "$GC" ]] || { echo "FAIL: $GC not found" >&2; exit 1; }

# freeze_case <name> [extra goal-contract args...] — echoes the session file.
freeze_case() {
  local name="$1"; shift
  local d="$TMP/$name"
  mkdir -p "$d"
  printf 'evaluate the installed model through existing settings\n' > "$d/request.txt"
  bash "$GC" freeze \
    --session-file "$d/session.json" \
    --source-request-file "$d/request.txt" \
    --intent "evaluate the installed model" \
    --success-signal "the existing suite reports a score" \
    --hard-constraint "no new infrastructure" \
    --non-goal "a certification program" \
    --runner bubbles.goal \
    --session-id "$name" \
    --repository-alias bubbles \
    --target repository=bubbles \
    --repository-root bubbles \
    ${1+"$@"} >/dev/null 2>&1
  printf '%s' "$d/session.json"
}

# --- P1. a held boundary mints a receipt carrying every binding -------------
S1="$(freeze_case p1 --execution-shape one-off --allow-change-class existing-test --delta-budget maxNewFiles=2)"
set +e
R1="$(bash "$RECEIPT" emit --boundary pre-planning --session-file "$S1" 2>/dev/null)"
rc1=$?
set -e
if [[ "$rc1" -eq 0 ]] && [[ -n "$R1" ]] &&
  [[ "$(jq -r '[.receiptVersion,.boundary,.goalId,.sourceRequestDigest,.semanticBoundaryDigest,.receiptDigest] | map(select(. != null and . != "")) | length' <<< "$R1")" == "6" ]]; then
  ok "P1 a held boundary mints a receipt carrying every binding"
else
  bad "P1 emit on a held boundary" "rc=$rc1 receipt=$R1"
fi

printf '%s\n' "$R1" > "$TMP/r1.json"

# --- P2. a fresh receipt verifies ------------------------------------------
set +e
bash "$RECEIPT" verify --receipt-file "$TMP/r1.json" --session-file "$S1" >/dev/null 2>&1
rc2=$?
set -e
if [[ "$rc2" -eq 0 ]]; then
  ok "P2 a fresh receipt verifies against its contract"
else
  bad "P2 fresh receipt verify" "rc=$rc2"
fi

# --- P3. every boundary name is accepted by the enum -----------------------
p3_ok="true"
for b in pre-planning post-planning pre-dispatch post-finding post-compaction pre-certification; do
  set +e
  out="$(bash "$RECEIPT" emit --boundary "$b" --session-file "$S1" 2>&1 >/dev/null)"
  set -e
  # A boundary may legitimately refuse for missing inputs (--spec-dir etc.);
  # what must NEVER happen is rejection of the NAME itself.
  if printf '%s' "$out" | grep -q -- "--boundary must be one of"; then
    p3_ok="false"; bad "P3 boundary '$b' rejected by the receipt enum" "$out"
  fi
done
[[ "$p3_ok" == "true" ]] && ok "P3 all six G134 boundary names are accepted"

# --- A1. ADVERSARIAL: a FAILED boundary mints NOTHING ----------------------
# This is the whole point. If a receipt could survive a failing guard, every
# consumer that demands one would be verifying a decoration.
printf '{}\n' > "$TMP/no-contract.json"
set +e
A1="$(bash "$RECEIPT" emit --boundary pre-planning --session-file "$TMP/no-contract.json" 2>/dev/null)"
rca1=$?
set -e
if [[ "$rca1" -ne 0 ]] && [[ -z "$A1" ]]; then
  ok "A1 a failed boundary mints NO receipt and propagates a non-zero exit"
else
  bad "A1 failed boundary must not mint a receipt" "rc=$rca1 stdout='$A1'"
fi

# --- A2. ADVERSARIAL: a receipt from another boundary ----------------------
set +e
bash "$RECEIPT" verify --receipt-file "$TMP/r1.json" --session-file "$S1" --expect-boundary pre-certification >/dev/null 2>&1
rca2=$?
set -e
if [[ "$rca2" -eq 1 ]]; then
  ok "A2 a receipt for another boundary is refused"
else
  bad "A2 wrong-boundary receipt" "rc=$rca2"
fi

# --- A3. ADVERSARIAL: an edited receipt body ------------------------------
jq '.revision = 99' "$TMP/r1.json" > "$TMP/tampered.json"
set +e
bash "$RECEIPT" verify --receipt-file "$TMP/tampered.json" --session-file "$S1" >/dev/null 2>&1
rca3=$?
set -e
if [[ "$rca3" -eq 1 ]]; then
  ok "A3 an edited receipt body fails its own receiptDigest"
else
  bad "A3 tampered receipt" "rc=$rca3"
fi

# --- A4/A5. ADVERSARIAL: superseded by an approved revision ---------------
bash "$GC" revise --session-file "$S1" --approval-note "operator widened the boundary" \
  --execution-shape reusable-capability --allow-change-class existing-test \
  --allow-change-class new-shared-library >/dev/null 2>&1
set +e
out45="$(bash "$RECEIPT" verify --receipt-file "$TMP/r1.json" --session-file "$S1" 2>&1)"
rca4=$?
set -e
if [[ "$rca4" -eq 1 ]] && printf '%s' "$out45" | grep -qE 'goalId|revision'; then
  ok "A4 a receipt minted before an approved revision no longer verifies"
else
  bad "A4 stale receipt after revision" "rc=$rca4 out=$out45"
fi

# A5 isolates the SEMANTIC binding specifically: same goalId and revision, but
# the declared boundary swapped underneath. Without semanticBoundaryDigest in
# the receipt this case would pass and the semantic layer would be unbound.
S5="$(freeze_case a5 --execution-shape one-off --allow-change-class existing-test)"
bash "$RECEIPT" emit --boundary pre-planning --session-file "$S5" 2>/dev/null > "$TMP/r5.json"
jq '.goalContract.semanticBoundary.allowedChangeClasses = ["existing-config","new-runner"]' "$S5" > "$S5.tmp" && mv "$S5.tmp" "$S5"
set +e
out5="$(bash "$RECEIPT" verify --receipt-file "$TMP/r5.json" --session-file "$S5" 2>&1)"
rca5=$?
set -e
if [[ "$rca5" -eq 1 ]] && printf '%s' "$out5" | grep -q 'semanticBoundaryDigest'; then
  ok "A5 a receipt is refused when the declared semantic boundary changed underneath it"
else
  bad "A5 semantic binding" "rc=$rca5 out=$out5"
fi

# --- A6. canonicalisation: key order must not change a digest -------------
S6="$(freeze_case a6 --execution-shape one-off --allow-change-class existing-test --allow-change-class existing-config)"
bash "$RECEIPT" emit --boundary pre-planning --session-file "$S6" 2>/dev/null > "$TMP/r6a.json"
jq '.goalContract.semanticBoundary = {deltaBudget: .goalContract.semanticBoundary.deltaBudget, approvalRequiredChangeClasses: .goalContract.semanticBoundary.approvalRequiredChangeClasses, allowedChangeClasses: .goalContract.semanticBoundary.allowedChangeClasses, executionShape: .goalContract.semanticBoundary.executionShape}' "$S6" > "$S6.tmp" && mv "$S6.tmp" "$S6"
set +e
bash "$RECEIPT" verify --receipt-file "$TMP/r6a.json" --session-file "$S6" >/dev/null 2>&1
rca6=$?
set -e
if [[ "$rca6" -eq 0 ]]; then
  ok "A6 reordering the boundary's keys does not invalidate a receipt"
else
  bad "A6 canonicalisation" "rc=$rca6 — digests must be order-independent, else every consumer sees false staleness"
fi

# --- U1. usage + no bypass ------------------------------------------------
set +e
bash "$RECEIPT" >/dev/null 2>&1; u1=$?
bash "$RECEIPT" emit --boundary nope --session-file "$S1" >/dev/null 2>&1; u2=$?
bypass="$(bash "$RECEIPT" emit --force 2>&1)"; u3=$?
set -e
if [[ "$u1" -eq 2 && "$u2" -eq 2 && "$u3" -eq 2 ]] && printf '%s' "$bypass" | grep -q 'bypass-shaped'; then
  ok "U1 no args, an unknown boundary, and a bypass flag all exit 2"
else
  bad "U1 usage" "noargs=$u1 badboundary=$u2 bypass=$u3"
fi

printf 'goal-boundary-receipt-selftest: %s check(s), %s failure(s)\n' "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
