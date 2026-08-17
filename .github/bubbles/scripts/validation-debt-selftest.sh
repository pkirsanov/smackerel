#!/usr/bin/env bash
# bubbles/scripts/validation-debt-selftest.sh
#
# Hermetic selftest for the IMP-047 S-E append-only validation debt ledger.
#
# Capability: validation-debt-settlement
#
# THE DEFECT THIS PINS CLOSED
# Tier selection, changed-only selection and the result cache all REMOVE work
# from a run, and none of them recorded what they removed. A fast run therefore
# produced the same terminal shape as a complete one. "We skipped it and will do
# it later" is a promise, and without a ledger it is a promise with no creditor.
#
# Four properties turn that promise into an obligation, and each one is written
# here so that removing it fails a case:
#
#   1. A deferral creates EXACTLY ONE open entry.
#   2. The store is APPEND-ONLY: settlement adds a row, it never rewrites one.
#   3. A MISSING LEDGER WRITE FORCES IMMEDIATE EXECUTION — `record` refuses, so
#      its caller must run the check instead of treating it as deferred.
#   4. `full` assurance REFUSES any open debt; `fast` permits only BOUND debt.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAME="validation-debt-selftest"
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

if [[ ! -f "$DEBT" ]]; then
  printf '%s: ledger not found: %s\n' "$NAME" "$DEBT" >&2
  exit 2
fi

WORK="$(mktemp -d)" || exit 2
trap 'rm -rf "$WORK"' EXIT INT TERM

LEDGER_DIR="$WORK/debt"
LEDGER="$LEDGER_DIR/ledger.jsonl"
REV="deadbeefcafe"
mkdir -p "$LEDGER_DIR"

FLOOR="$WORK/floor.json"
printf '{"floor":"basic","sourceRevision":"%s","failures":"0"}\n' "$REV" >"$FLOOR"

vd() { bash "$DEBT" "$@" --ledger-dir "$LEDGER_DIR"; }

# --- P1 / A1. a deferral creates EXACTLY ONE open entry ---------------------
entry_id="$(vd record --check vc-alpha --class heavy-selftest --source-revision "$REV" \
  --spec IMP-047 --reason closure-unaffected --boundary release --floor-receipt "$FLOOR" 2>&1)"
record_rc=$?
open_after_one="$(vd count --open 2>/dev/null | tr -d ' ')"
if [[ "$record_rc" -eq 0 && -n "$entry_id" && "$open_after_one" == "1" ]]; then
  ok "P1 a deferral records one open obligation and returns its entry id"
else
  bad "P1 record one entry" "rc=$record_rc id=$entry_id open=$open_after_one"
fi

# ADVERSARIAL: the SAME deferral again must be refused, not silently duplicated.
# A ledger that accepts duplicates cannot answer "how much is owed".
dup_out="$(vd record --check vc-alpha --class heavy-selftest --source-revision "$REV" \
  --spec IMP-047 --reason closure-unaffected --boundary release --floor-receipt "$FLOOR" 2>&1)"
dup_rc=$?
open_after_dup="$(vd count --open 2>/dev/null | tr -d ' ')"
if [[ "$dup_rc" -eq 3 ]] && printf '%s' "$dup_out" | grep -q 'exactly one open entry' &&
  [[ "$open_after_dup" == "1" ]]; then
  ok "A1 a repeated deferral is refused and the open count stays at exactly one"
else
  bad "A1 exactly one open entry" "rc=$dup_rc open=$open_after_dup out=$(printf '%s' "$dup_out" | tr '\n' '|')"
fi

# --- A2. ADVERSARIAL: `full` assurance REFUSES any open debt ----------------
full_out="$(vd assurance --level full 2>&1)"
full_rc=$?
if [[ "$full_rc" -eq 1 ]] && printf '%s' "$full_out" | grep -q 'requires ZERO open validation debt'; then
  ok "A2 full assurance refuses while any obligation is open"
else
  bad "A2 full refuses open debt" "rc=$full_rc out=$(printf '%s' "$full_out" | tr '\n' '|')"
fi

# --- A3. `fast` permits BOUND debt ------------------------------------------
fast_out="$(vd assurance --level fast 2>&1)"
fast_rc=$?
if [[ "$fast_rc" -eq 0 ]] && printf '%s' "$fast_out" | grep -q 'bound open obligation'; then
  ok "A3 fast assurance permits debt that carries a valid settlement boundary"
else
  bad "A3 fast permits bound debt" "rc=$fast_rc out=$(printf '%s' "$fast_out" | tr '\n' '|')"
fi

# --- A4. ADVERSARIAL: `fast` REFUSES UNBOUND debt ---------------------------
# Debt with no settlement boundary is indistinguishable from a silent skip: it
# is owed to nobody, at no point. Written straight into the store because
# `record` will not accept an invalid boundary, which is itself the first gate.
printf '{"event":"defer","id":"vd-smuggled","check":"vc-smuggled","class":"heavy-selftest","sourceRevision":"%s","spec":"x","reason":"x","boundary":"someday","recordedAt":"now"}\n' \
  "$REV" >>"$LEDGER"
unbound_out="$(vd assurance --level fast 2>&1)"
unbound_rc=$?
if [[ "$unbound_rc" -eq 1 ]] && printf '%s' "$unbound_out" | grep -q 'only BOUND debt'; then
  ok "A4 fast assurance refuses an obligation with no valid settlement boundary"
else
  bad "A4 fast refuses unbound debt" "rc=$unbound_rc out=$(printf '%s' "$unbound_out" | tr '\n' '|')"
fi
# Remove the smuggled row so the remaining cases reason about real entries only.
LC_ALL=C grep -v '"id":"vd-smuggled"' "$LEDGER" >"$WORK/trimmed.jsonl"
cp "$WORK/trimmed.jsonl" "$LEDGER"

# --- A5. ADVERSARIAL: a MISSING LEDGER WRITE forces immediate execution -----
# The load-bearing rule. `record` must FAIL when it cannot append, because a
# deferral nobody could record is a silent skip. The parent path here is a
# regular FILE, so mkdir -p cannot succeed.
printf 'not a directory\n' >"$WORK/blocked"
blocked_out="$(bash "$DEBT" record --check vc-blocked --class heavy-selftest \
  --source-revision "$REV" --spec IMP-047 --reason closure-unaffected \
  --boundary release --floor-receipt "$FLOOR" --ledger-dir "$WORK/blocked/dir" 2>&1)"
blocked_rc=$?
if [[ "$blocked_rc" -eq 3 ]] && printf '%s' "$blocked_out" | grep -q 'EXECUTE the check'; then
  ok "A5 an unwritable ledger refuses with exit 3 and tells the caller to execute the check"
else
  bad "A5 missing write forces execution" "rc=$blocked_rc out=$(printf '%s' "$blocked_out" | tr '\n' '|')"
fi

# --- A6. ADVERSARIAL: debt cannot be taken out without the basic floor ------
noflor_out="$(vd record --check vc-nofloor --class heavy-selftest --source-revision "$REV" \
  --spec IMP-047 --reason closure-unaffected --boundary release \
  --floor-receipt "$WORK/absent-floor.json" 2>&1)"
noflor_rc=$?
wrongrev="$WORK/wrong-floor.json"
printf '{"floor":"basic","sourceRevision":"0000000000","failures":"0"}\n' >"$wrongrev"
wrongrev_out="$(vd record --check vc-wrongrev --class heavy-selftest --source-revision "$REV" \
  --spec IMP-047 --reason closure-unaffected --boundary release --floor-receipt "$wrongrev" 2>&1)"
wrongrev_rc=$?
if [[ "$noflor_rc" -eq 3 && "$wrongrev_rc" -eq 3 ]] &&
  printf '%s' "$noflor_out" | grep -q 'floor receipt not found' &&
  printf '%s' "$wrongrev_out" | grep -q 'does not name source revision'; then
  ok "A6 debt is refused without a floor receipt naming this source revision"
else
  bad "A6 basic floor required" \
    "nofloor_rc=$noflor_rc wrongrev_rc=$wrongrev_rc out=$(printf '%s%s' "$noflor_out" "$wrongrev_out" | tr '\n' '|')"
fi

# --- A7. ADVERSARIAL: a settlement needs a MATCHING, PASSING receipt --------
mismatch="$WORK/receipt-mismatch.json"
printf '{"check":"vc-somethingelse","sourceRevision":"%s","exitCode":"0","closureDigest":"abc"}\n' "$REV" >"$mismatch"
mismatch_out="$(vd settle --entry "$entry_id" --receipt "$mismatch" 2>&1)"
mismatch_rc=$?
failed_receipt="$WORK/receipt-failed.json"
printf '{"check":"vc-alpha","sourceRevision":"%s","exitCode":"1","closureDigest":"abc"}\n' "$REV" >"$failed_receipt"
failed_out="$(vd settle --entry "$entry_id" --receipt "$failed_receipt" 2>&1)"
failed_rc=$?
nodigest="$WORK/receipt-nodigest.json"
printf '{"check":"vc-alpha","sourceRevision":"%s","exitCode":"0"}\n' "$REV" >"$nodigest"
nodigest_out="$(vd settle --entry "$entry_id" --receipt "$nodigest" 2>&1)"
nodigest_rc=$?
if [[ "$mismatch_rc" -eq 1 && "$failed_rc" -eq 1 && "$nodigest_rc" -eq 1 ]] &&
  printf '%s' "$mismatch_out" | grep -q 'does not match the entry' &&
  printf '%s' "$failed_out" | grep -q 'a failed run settles nothing' &&
  printf '%s' "$nodigest_out" | grep -q 'no closureDigest'; then
  ok "A7 a settlement is refused for a foreign, failing or unbound receipt"
else
  bad "A7 settlement needs a matching passing receipt" \
    "rc=$mismatch_rc/$failed_rc/$nodigest_rc out=$(printf '%s%s%s' "$mismatch_out" "$failed_out" "$nodigest_out" | tr '\n' '|')"
fi

# --- A8. ADVERSARIAL: the store is APPEND-ONLY ------------------------------
# Settlement must ADD a row that references the entry, never edit the defer row.
# Rewriting history is how a ledger stops being evidence.
before_lines="$(wc -l <"$LEDGER" | tr -d ' ')"
before_defer="$(LC_ALL=C grep '"event":"defer"' "$LEDGER" | head -1)"
good_receipt="$WORK/receipt-good.json"
printf '{"check":"vc-alpha","sourceRevision":"%s","exitCode":"0","closureDigest":"sha256:fixture"}\n' "$REV" >"$good_receipt"
settle_out="$(vd settle --entry "$entry_id" --receipt "$good_receipt" 2>&1)"
settle_rc=$?
after_lines="$(wc -l <"$LEDGER" | tr -d ' ')"
after_defer="$(LC_ALL=C grep '"event":"defer"' "$LEDGER" | head -1)"
open_after_settle="$(vd count --open 2>/dev/null | tr -d ' ')"
if [[ "$settle_rc" -eq 0 && "$after_lines" -eq $((before_lines + 1)) &&
  "$before_defer" == "$after_defer" && "$open_after_settle" == "0" ]] &&
  printf '%s\n' "$(vd list --settled 2>/dev/null)" | grep -q "\"entry\":\"$entry_id\""; then
  ok "A8 settlement appends a referencing row, leaves the defer row byte-identical, and closes the obligation"
else
  bad "A8 append-only settlement" \
    "rc=$settle_rc lines=$before_lines->$after_lines open=$open_after_settle defer_intact=$([[ "$before_defer" == "$after_defer" ]] && echo yes || echo no) out=$(printf '%s' "$settle_out" | tr '\n' '|')"
fi

# --- A9. full assurance passes once the ledger is clear ---------------------
# Non-vacuity twin of A2: without this, A2 would also pass against a `full`
# that always refuses.
clear_out="$(vd assurance --level full 2>&1)"
clear_rc=$?
if [[ "$clear_rc" -eq 0 ]] && printf '%s' "$clear_out" | grep -q 'zero open validation debt'; then
  ok "A9 full assurance passes only once every obligation is settled"
else
  bad "A9 full passes when clear" "rc=$clear_rc out=$(printf '%s' "$clear_out" | tr '\n' '|')"
fi

# --- A10. ADVERSARIAL: prototype is never deployable, and no bypass exists ---
proto_out="$(vd assurance --level prototype 2>&1)"
proto_rc=$?
bypass_out="$(vd assurance --level full --force 2>&1)"
bypass_rc=$?
if [[ "$proto_rc" -eq 1 && "$bypass_rc" -eq 2 ]] &&
  printf '%s' "$proto_out" | grep -q 'never deployable' &&
  printf '%s' "$bypass_out" | grep -q 'no bypass'; then
  ok "A10 prototype is refused as never deployable and a bypass-shaped flag exits 2"
else
  bad "A10 prototype and no bypass" \
    "proto_rc=$proto_rc bypass_rc=$bypass_rc out=$(printf '%s%s' "$proto_out" "$bypass_out" | tr '\n' '|')"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
