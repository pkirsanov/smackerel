#!/usr/bin/env bash
# Hermetic selftest for expansion-approval.sh (IMP-041 SCOPE-4 / GF-10).
#
# The guarantee: an architecture expansion is authorised ONLY by an operator
# note naming the exact canonical expansionDigest. Every adversarial case below
# attacks a different way an expansion could otherwise proceed unapproved — a
# generic continuation, an edited preview, growth after approval, or an
# approval granted under a boundary that has since changed.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EA="$SCRIPT_DIR/expansion-approval.sh"
GC="$SCRIPT_DIR/goal-contract.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM
checks=0
failures=0
ok() { checks=$((checks + 1)); echo "  ok   $1"; }
bad() { checks=$((checks + 1)); failures=$((failures + 1)); echo "  FAIL $1"; echo "       $2"; }

command -v jq >/dev/null 2>&1 || { echo "expansion-approval-selftest: SKIP (jq not installed)"; exit 0; }
[[ -f "$EA" ]] || { echo "FAIL: $EA not found" >&2; exit 1; }
[[ -f "$GC" ]] || { echo "FAIL: $GC not found" >&2; exit 1; }

GATED_DELTA='{"changeClasses":["new-virtual-machine"],"maxNewFiles":2}'

# new_session <name> — a frozen v2 contract gating VMs and runners.
new_session() {
  local d="$TMP/$1"
  mkdir -p "$d"
  printf 'evaluate the installed model through existing settings\n' > "$d/request.txt"
  bash "$GC" freeze \
    --session-file "$d/session.json" --source-request-file "$d/request.txt" \
    --intent "evaluate the installed model" --success-signal "the existing suite reports a score" \
    --runner bubbles.goal --session-id "$1" --repository-alias bubbles \
    --target repository=bubbles --repository-root bubbles \
    --execution-shape one-off \
    --allow-change-class existing-test \
    --approval-change-class new-virtual-machine \
    --approval-change-class new-runner \
    --delta-budget maxNewFiles=5 >/dev/null 2>&1
  printf '%s' "$d/session.json"
}

make_preview() {
  bash "$EA" preview --session-file "$1" --planned-delta "${2:-$GATED_DELTA}" \
    --reason new-virtual-machine="isolation for the eval harness" \
    --rejected-alternative "run inside the existing container" \
    --rollback "delete the VM definition"
}

# grant <session> <preview-file> — the operator flow: note the digest, then record.
grant() {
  local s="$1" p="$2" digest
  digest="$(jq -r '.expansionDigest' "$p")"
  bash "$GC" revise --session-file "$s" --approval-note "operator approves expansion:$digest" >/dev/null 2>&1
  bash "$EA" approve --session-file "$s" --preview-file "$p" >/dev/null 2>&1
}

rc_of() { set +e; "$@" >/dev/null 2>&1; local r=$?; set -e; printf '%s' "$r"; }

# expect <want-rc> <got-rc> <description>
expect() {
  if [[ "$2" -eq "$1" ]]; then ok "$3"; else bad "$3" "rc=$2 (wanted $1)"; fi
}

# --- P1. a plan with no gated class needs no approval ----------------------
S1="$(new_session p1)"
r="$(rc_of bash "$EA" verify --session-file "$S1" --planned-delta '{"changeClasses":["existing-test"],"maxNewFiles":1}')"
expect 0 "$r" "P1 a plan with no approval-required class needs no approval"

# --- A1. ADVERSARIAL: planning a VM is refused before approval -------------
r="$(rc_of bash "$EA" verify --session-file "$S1" --planned-delta "$GATED_DELTA")"
expect 1 "$r" "A1 planning a virtual machine is refused with no approval"

# --- A2. ADVERSARIAL: every gated class must state its contribution --------
set +e
out="$(bash "$EA" preview --session-file "$S1" --planned-delta "$GATED_DELTA" 2>&1)"
r2=$?
set -e
if [[ "$r2" -eq 2 ]] && printf '%s' "$out" | grep -q 'needs --reason'; then
  ok "A2 a gated class with no stated contribution cannot even be previewed"
else
  bad "A2 reason required" "rc=$r2 out=$out"
fi

# --- A3. ADVERSARIAL: a generic continuation is not an approval ------------
# This is the case the whole design exists for. "continue" cannot contain a
# digest that did not exist when it was written, so it can never approve.
S3="$(new_session a3)"
make_preview "$S3" > "$TMP/p3.json"
bash "$GC" revise --session-file "$S3" --approval-note "continue" >/dev/null 2>&1
set +e
out3="$(bash "$EA" approve --session-file "$S3" --preview-file "$TMP/p3.json" 2>&1)"
r3=$?
set -e
if [[ "$r3" -eq 1 ]] && printf '%s' "$out3" | grep -q 'does not name expansion:'; then
  ok "A3 a generic continuation cannot approve an architecture expansion"
else
  bad "A3 generic continuation" "rc=$r3 out=$out3"
fi

# --- A4. ADVERSARIAL: an edited preview cannot be approved -----------------
S4="$(new_session a4)"
make_preview "$S4" > "$TMP/p4.json"
jq '.plannedCounts.maxNewFiles = 99' "$TMP/p4.json" > "$TMP/p4-edited.json"
d4="$(jq -r '.expansionDigest' "$TMP/p4.json")"
bash "$GC" revise --session-file "$S4" --approval-note "approves expansion:$d4" >/dev/null 2>&1
r="$(rc_of bash "$EA" approve --session-file "$S4" --preview-file "$TMP/p4-edited.json")"
expect 1 "$r" "A4 a preview edited after generation fails its own digest"

# --- P2/P3. the approved plan verifies; a NARROWER plan stays covered ------
S5="$(new_session p5)"
make_preview "$S5" > "$TMP/p5.json"
grant "$S5" "$TMP/p5.json"
r="$(rc_of bash "$EA" verify --session-file "$S5" --planned-delta "$GATED_DELTA")"
expect 0 "$r" "P2 the exact approved plan verifies"

r="$(rc_of bash "$EA" verify --session-file "$S5" --planned-delta '{"changeClasses":["new-virtual-machine"],"maxNewFiles":1}')"
expect 0 "$r" "P3 a narrower post-approval plan stays covered"

# --- A5/A6. ADVERSARIAL: growth escapes coverage ---------------------------
r="$(rc_of bash "$EA" verify --session-file "$S5" --planned-delta '{"changeClasses":["new-virtual-machine"],"maxNewFiles":9}')"
expect 1 "$r" "A5 a later count increase invalidates the approval"

r="$(rc_of bash "$EA" verify --session-file "$S5" --planned-delta '{"changeClasses":["new-virtual-machine","new-runner"],"maxNewFiles":2}')"
expect 1 "$r" "A6 adding a runner absent from the approved preview is refused"

# --- A7. ADVERSARIAL: an approval does not survive a boundary change -------
# Binding the record to the semantic boundary is what makes this hold. Binding
# it to a revision instead would be self-defeating, because granting an approval
# REQUIRES a revise and would invalidate the approval at the instant it landed —
# a bug this selftest caught during development (P2 above is its red control).
bash "$GC" revise --session-file "$S5" --approval-note "operator widens the boundary" \
  --execution-shape reusable-capability --allow-change-class existing-test \
  --approval-change-class new-virtual-machine --approval-change-class new-runner \
  --approval-change-class new-datastore >/dev/null 2>&1
r="$(rc_of bash "$EA" verify --session-file "$S5" --planned-delta "$GATED_DELTA")"
expect 1 "$r" "A7 an approval does not survive a change to the semantic boundary"

# --- U1. usage + no bypass -------------------------------------------------
set +e
bash "$EA" >/dev/null 2>&1; u1=$?
bypass="$(bash "$EA" verify --force 2>&1)"; u2=$?
bash "$EA" frobnicate >/dev/null 2>&1; u3=$?
set -e
if [[ "$u1" -eq 2 && "$u2" -eq 2 && "$u3" -eq 2 ]] && printf '%s' "$bypass" | grep -q 'bypass-shaped'; then
  ok "U1 no args, a bypass flag, and an unknown subcommand all exit 2"
else
  bad "U1 usage" "noargs=$u1 bypass=$u2 unknown=$u3"
fi

# --- A8. ADVERSARIAL: an UNDECLARED class is gated too --------------------
# Found by the SCOPE-8 corpus, not by the cases above. The first implementation
# gated only classes named in approvalRequiredChangeClasses, so a contract that
# simply never MENTIONED virtual machines waved them straight through — the
# exact overbuilt-evaluation shape this IMP exists to refuse. A class in neither
# list is undeclared, and undeclared is the more dangerous population of the two.
S8="$(new_session a8)"
r="$(rc_of bash "$EA" verify --session-file "$S8" --planned-delta '{"changeClasses":["new-datastore"],"maxNewFiles":1}')"
expect 1 "$r" "A8 a class in NEITHER the allowed nor the approval-required list is gated"

r="$(rc_of bash "$EA" verify --session-file "$S8" --planned-delta '{"changeClasses":["existing-test"],"maxNewFiles":1}')"
expect 0 "$r" "A8b a class explicitly in allowedChangeClasses still needs no approval"

# --- A9. ADVERSARIAL: a v1 contract is out of scope for this gate ---------
# The undeclared-class rule must not leak into v1, where there is no semantic
# boundary at all — otherwise every legacy goal would be refused on its first
# plan. Also caught by the corpus.
D9="$TMP/a9"
mkdir -p "$D9"
printf 'legacy request\n' > "$D9/request.txt"
bash "$GC" freeze --session-file "$D9/session.json" --source-request-file "$D9/request.txt" \
  --intent "legacy goal" --success-signal "signal" --runner bubbles.goal \
  --session-id a9legacy --repository-alias bubbles \
  --target repository=bubbles --repository-root bubbles >/dev/null 2>&1
r="$(rc_of bash "$EA" verify --session-file "$D9/session.json" --planned-delta '{"changeClasses":["new-virtual-machine"]}')"
expect 0 "$r" "A9 a v1 contract with no semantic boundary is out of scope for this gate"

printf 'expansion-approval-selftest: %s check(s), %s failure(s)\n' "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
