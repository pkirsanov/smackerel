#!/usr/bin/env bash
# bubbles/scripts/validation-batch-selftest.sh
#
# Hermetic selftest for the IMP-047 S-E batched obligation executor.
#
# Capability: validation-debt-settlement
#
# WHY BATCHING NEEDS ITS OWN GUARD
# Batching is the saving: deferred heavy work runs once instead of once per
# obligation. The danger is entirely in the accounting. One command runs, and it
# is tempting to call every grouped obligation settled because the batch "went
# green" — or, worse, to hand partial credit to the entries that were not
# implicated in a failure.
#
# The two rules that make batching safe, each pinned by an adversarial case:
#
#   1. ONE SETTLING RECEIPT PER OBLIGATION. The batch writes a separate receipt
#      naming that entry's check, source revision and closure digest, so a
#      settlement still points at the specific obligation it discharges.
#   2. A FAILED BATCH SETTLES NOTHING. Not the entries that "would have passed",
#      not the ones unrelated to the failure. Partial credit from a failed run
#      is how a red result becomes a green ledger.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAME="validation-batch-selftest"
BATCH="$SCRIPT_DIR/validation-batch.sh"
DEBT="$SCRIPT_DIR/validation-debt.sh"

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
  return 0
}

if [[ ! -f "$BATCH" || ! -f "$DEBT" ]]; then
  printf '%s: batch executor or ledger not found\n' "$NAME" >&2
  exit 2
fi

WORK="$(mktemp -d)" || exit 2
trap 'rm -rf "$WORK"' EXIT INT TERM

LEDGER_DIR="$WORK/debt"
LEDGER="$LEDGER_DIR/ledger.jsonl"
RECEIPTS="$LEDGER_DIR/receipts"
REV="feedfacedeadbeef"
mkdir -p "$LEDGER_DIR"

OTHER_REV="other-revision-0001"
FLOOR="$WORK/floor.json"
printf '{"floor":"basic","sourceRevision":"%s","failures":"0"}\n' "$REV" >"$FLOOR"
# The basic floor is per-revision by construction, so the second tree needs its
# own receipt; reusing the first would be debt taken out against a floor that
# never ran for that revision.
FLOOR_OTHER="$WORK/floor-other.json"
printf '{"floor":"basic","sourceRevision":"%s","failures":"0"}\n' "$OTHER_REV" >"$FLOOR_OTHER"

vd() { bash "$DEBT" "$@" --ledger-dir "$LEDGER_DIR"; }
vb() { bash "$BATCH" "$@"; }

defer() {
  local revision="${3:-$REV}" floor="$FLOOR"
  [[ "$revision" == "$REV" ]] || floor="$FLOOR_OTHER"
  vd record --check "$1" --class "${2:-heavy-selftest}" --source-revision "$revision" \
    --spec IMP-047 --reason closure-unaffected --boundary release --floor-receipt "$floor"
}

entry_a="$(defer vc-alpha)"
entry_b="$(defer vc-beta)"
entry_c="$(defer vc-gamma e2e)"
entry_d="$(defer vc-delta heavy-selftest "$OTHER_REV")"

if [[ -n "$entry_a" && -n "$entry_b" && -n "$entry_c" && -n "$entry_d" ]]; then
  ok "P1 four obligations are open before any batch runs"
else
  bad "P1 fixture obligations" "a=$entry_a b=$entry_b c=$entry_c d=$entry_d"
fi

# --- A1. ADVERSARIAL: A FAILED BATCH SETTLES NOTHING ------------------------
# The whole reason this executor exists. Take a byte-exact fingerprint of the
# ledger, run a failing batch over both compatible entries, and require the
# ledger to be UNCHANGED and both obligations still open.
ledger_before="$(cat "$LEDGER")"
open_before="$(vd count --open 2>/dev/null | tr -d ' ')"
fail_out="$(vb --class heavy-selftest --source-revision "$REV" \
  --entry "$entry_a" --entry "$entry_b" --ledger-dir "$LEDGER_DIR" -- false 2>&1)"
fail_rc=$?
ledger_after="$(cat "$LEDGER")"
open_after="$(vd count --open 2>/dev/null | tr -d ' ')"
receipts_after=0
[[ -d "$RECEIPTS" ]] && receipts_after="$(find "$RECEIPTS" -type f | wc -l | tr -d ' ')"
if [[ "$fail_rc" -eq 1 && "$ledger_before" == "$ledger_after" &&
  "$open_before" == "$open_after" && "$receipts_after" -eq 0 ]] &&
  printf '%s' "$fail_out" | grep -q 'settling NOTHING'; then
  ok "A1 a failed batch settles nothing, writes no receipt, and leaves the ledger byte-identical"
else
  bad "A1 failed batch settles nothing" \
    "rc=$fail_rc open=$open_before->$open_after receipts=$receipts_after ledger_intact=$([[ "$ledger_before" == "$ledger_after" ]] && echo yes || echo no)"
fi

# --- A2. ADVERSARIAL: incompatible entries are refused BEFORE the run -------
# Refusing after an expensive batch would still burn the run. The marker file
# proves the command never executed.
marker="$WORK/marker-class"
class_out="$(vb --class heavy-selftest --source-revision "$REV" \
  --entry "$entry_a" --entry "$entry_c" --ledger-dir "$LEDGER_DIR" -- touch "$marker" 2>&1)"
class_rc=$?
marker_rev="$WORK/marker-rev"
rev_out="$(vb --class heavy-selftest --source-revision "$REV" \
  --entry "$entry_a" --entry "$entry_d" --ledger-dir "$LEDGER_DIR" -- touch "$marker_rev" 2>&1)"
rev_rc=$?
if [[ "$class_rc" -eq 1 && "$rev_rc" -eq 1 && ! -e "$marker" && ! -e "$marker_rev" ]] &&
  printf '%s' "$class_out" | grep -q 'incompatible entries cannot share a batch' &&
  printf '%s' "$rev_out" | grep -q 'one batch cannot validate two trees'; then
  ok "A2 a mixed obligation class or source revision is refused before the command runs"
else
  bad "A2 incompatible entries refused up front" \
    "class_rc=$class_rc rev_rc=$rev_rc marker=$([[ -e "$marker" ]] && echo created || echo absent) marker_rev=$([[ -e "$marker_rev" ]] && echo created || echo absent)"
fi

# --- A3. ADVERSARIAL: an unknown or already-settled entry is refused --------
marker_unknown="$WORK/marker-unknown"
unknown_out="$(vb --class heavy-selftest --source-revision "$REV" \
  --entry vd-does-not-exist --ledger-dir "$LEDGER_DIR" -- touch "$marker_unknown" 2>&1)"
unknown_rc=$?
if [[ "$unknown_rc" -eq 1 && ! -e "$marker_unknown" ]] &&
  printf '%s' "$unknown_out" | grep -q 'no OPEN debt entry'; then
  ok "A3 an unknown obligation id is refused before the command runs"
else
  bad "A3 unknown entry refused" "rc=$unknown_rc out=$(printf '%s' "$unknown_out" | tr '\n' '|')"
fi

# --- A4. a passing batch settles ONLY its grouped entries -------------------
pass_out="$(vb --class heavy-selftest --source-revision "$REV" \
  --entry "$entry_a" --entry "$entry_b" --ledger-dir "$LEDGER_DIR" -- true 2>&1)"
pass_rc=$?
settled_rows="$(vd list --settled 2>/dev/null)"
open_rows="$(vd list --open 2>/dev/null)"
if [[ "$pass_rc" -eq 0 ]] &&
  printf '%s' "$pass_out" | grep -q 'settled 2 obligation(s) with one receipt each' &&
  printf '%s\n' "$settled_rows" | grep -q "\"entry\":\"$entry_a\"" &&
  printf '%s\n' "$settled_rows" | grep -q "\"entry\":\"$entry_b\"" &&
  printf '%s\n' "$open_rows" | grep -q "\"id\":\"$entry_c\"" &&
  printf '%s\n' "$open_rows" | grep -q "\"id\":\"$entry_d\""; then
  ok "A4 a passing batch settles exactly the grouped obligations and leaves the rest open"
else
  bad "A4 batch settles only its group" \
    "rc=$pass_rc out=$(printf '%s' "$pass_out" | tr '\n' '|') settled=$(printf '%s' "$settled_rows" | tr '\n' '|') open=$(printf '%s' "$open_rows" | tr '\n' '|')"
fi

# --- A5. ADVERSARIAL: ONE SETTLING RECEIPT PER OBLIGATION -------------------
# A single shared receipt would let one green run discharge obligations it never
# named. Each receipt must exist separately and carry its own entry and check.
receipt_count="$(find "$RECEIPTS" -type f -name '*.json' | wc -l | tr -d ' ')"
receipt_a="$(LC_ALL=C grep -l "\"entry\":\"$entry_a\"" "$RECEIPTS"/*.json 2>/dev/null | head -1)"
receipt_b="$(LC_ALL=C grep -l "\"entry\":\"$entry_b\"" "$RECEIPTS"/*.json 2>/dev/null | head -1)"
if [[ "$receipt_count" -eq 2 && -n "$receipt_a" && -n "$receipt_b" && "$receipt_a" != "$receipt_b" ]] &&
  LC_ALL=C grep -q '"check":"vc-alpha"' "$receipt_a" &&
  LC_ALL=C grep -q '"check":"vc-beta"' "$receipt_b" &&
  LC_ALL=C grep -q '"exitCode":"0"' "$receipt_a"; then
  ok "A5 the batch wrote one distinct settling receipt per obligation, each naming its own check"
else
  bad "A5 one receipt per obligation" \
    "count=$receipt_count a=$receipt_a b=$receipt_b"
fi

# --- A6. ADVERSARIAL: settled obligations cannot be batched again -----------
# Without this, the same batch could be replayed to inflate the settled count.
marker_replay="$WORK/marker-replay"
replay_out="$(vb --class heavy-selftest --source-revision "$REV" \
  --entry "$entry_a" --ledger-dir "$LEDGER_DIR" -- touch "$marker_replay" 2>&1)"
replay_rc=$?
if [[ "$replay_rc" -eq 1 && ! -e "$marker_replay" ]] &&
  printf '%s' "$replay_out" | grep -q 'no OPEN debt entry'; then
  ok "A6 an already-settled obligation cannot be batched a second time"
else
  bad "A6 no replay" "rc=$replay_rc out=$(printf '%s' "$replay_out" | tr '\n' '|')"
fi

# --- A7. ADVERSARIAL: no bypass flag exists ---------------------------------
bypass_out="$(vb --class heavy-selftest --source-revision "$REV" --entry "$entry_c" \
  --ledger-dir "$LEDGER_DIR" --settle-anyway -- true 2>&1)"
bypass_rc=$?
missing_cmd_out="$(vb --class heavy-selftest --source-revision "$REV" \
  --entry "$entry_c" --ledger-dir "$LEDGER_DIR" 2>&1)"
missing_cmd_rc=$?
if [[ "$bypass_rc" -eq 2 && "$missing_cmd_rc" -eq 2 ]] &&
  printf '%s' "$bypass_out" | grep -q 'no bypass' &&
  printf '%s' "$missing_cmd_out" | grep -q 'batch command is required'; then
  ok "A7 a bypass-shaped flag and a missing batch command both exit 2"
else
  bad "A7 no bypass" \
    "bypass_rc=$bypass_rc missing_cmd_rc=$missing_cmd_rc out=$(printf '%s%s' "$bypass_out" "$missing_cmd_out" | tr '\n' '|')"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
