#!/usr/bin/env bash
# bubbles/scripts/bug-packet-resolve-selftest.sh
#
# Hermetic selftest for BUG-041's bug-packet-resolve.sh.
#
# The measured defect: bubbles/registry/bug-packet.yaml was the declared single
# bug-artifact authority and had NO production reader, so the `compact` form —
# the DEFAULT route since IMP-047 S-D — could not pass artifact-lint.sh.
#
# The adversarial cases are A1 through A4. They exist because the failure mode
# of a resolver is not "it crashes", it is "it quietly answers nothing", and a
# caller that receives an empty requirement set reports a false PASS. A1 proves
# a form declaring zero artifacts is refused. A2 proves an absent registry is
# refused rather than degraded. A3 proves the absent-default is a real,
# fail-closed value rather than a comment. A4 proves the bypass flags this
# resolver must never grow are rejected by name.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVER="$SCRIPT_DIR/bug-packet-resolve.sh"
REGISTRY="$SCRIPT_DIR/../registry/bug-packet.yaml"
NAME="bug-packet-resolve-selftest"

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

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# --- P0. the resolver runs against the shipped registry --------------------
facts="$(bash "$RESOLVER" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]] && [[ -n "$facts" ]]; then
  ok "P0 the resolver reads the shipped registry (exit 0)"
else
  bad "P0 resolver reads shipped registry" "rc=$rc out=$(printf '%s' "$facts" | tr '\n' '|')"
fi

# --- P1. all three forms are emitted ---------------------------------------
missing_forms=""
for form in full compact single-file; do
  printf '%s\n' "$facts" | grep -qx "form=$form" || missing_forms="$missing_forms $form"
done
if [[ -z "$missing_forms" ]]; then
  ok "P1 the resolver emits the full, compact and single-file forms"
else
  bad "P1 three forms emitted" "missing:$missing_forms"
fi

# --- P2. the compact form resolves to a NON-EMPTY, SMALLER artifact set ----
# Both halves matter. Non-empty is the false-PASS guard; smaller is the whole
# point of the form, and a compact set equal to the full set would mean the
# registry had quietly lost its proportionality.
full_n="$(printf '%s\n' "$facts" | grep -c '^artifact=full|')"
compact_n="$(printf '%s\n' "$facts" | grep -c '^artifact=compact|')"
if [[ "$compact_n" -gt 0 ]] && [[ "$compact_n" -lt "$full_n" ]]; then
  ok "P2 compact resolves to $compact_n artifact(s), fewer than full's $full_n"
else
  bad "P2 compact is non-empty and smaller than full" "compact=$compact_n full=$full_n"
fi

# --- P3. the declaration block is complete ---------------------------------
decl_field="$(printf '%s\n' "$facts" | sed -n 's/^field=//p' | head -1)"
decl_default="$(printf '%s\n' "$facts" | sed -n 's/^default=//p' | head -1)"
decl_location="$(printf '%s\n' "$facts" | sed -n 's/^location=//p' | head -1)"
if [[ -n "$decl_field" ]] && [[ -n "$decl_location" ]] && [[ -n "$decl_default" ]]; then
  ok "P3 the declaration names its field ($decl_field in $decl_location) and absent-default ($decl_default)"
else
  bad "P3 declaration complete" "field=$decl_field location=$decl_location default=$decl_default"
fi

# --- P4. the shipped word micro-fix-admission.sh reads is in the vocabulary -
# The field was real and consumed at micro-fix-admission.sh:144 while no
# registry declared it. If the alias silently disappears, every packet storing
# it becomes unreadable, so its presence is asserted rather than assumed.
if printf '%s\n' "$facts" | grep -qx 'vocab=micro|compact' \
  && printf '%s\n' "$facts" | grep -qx 'alias=micro|compact'; then
  ok "P4 the deprecated alias micro is declared, and marked as an alias"
else
  bad "P4 micro declared as a deprecated alias" "$(printf '%s\n' "$facts" | grep -E '^(vocab|alias)=' | tr '\n' '|')"
fi

# --- A1. ADVERSARIAL: a form declaring zero artifacts is REFUSED -----------
# An empty requirement set is a false PASS, which is the same defect class as
# IMP-047 PD-04. This is the single most important assertion in the file.
empty_reg="$WORK/empty.yaml"
# BUG-042 made this fixture BLOCK-AWARE. It used to match the compact artifact
# list as literal text, so adding a `purpose:` to each entry silently stopped the
# edit from applying. The validity probe below caught that rather than letting
# the assertion go vacuous, which is the whole reason the probe exists — but a
# fixture that breaks on every wording change is a maintenance tax with no
# assurance. Delete the compact `artifacts:` entries structurally instead: the
# block ends at the next key at the same indent.
awk '
  /^[A-Za-z]/            { in_compact = 0; drop = 0 }
  /^  - form: /          { drop = 0; in_compact = ($0 == "  - form: compact") ? 1 : 0 }
  (in_compact && drop && /^    [A-Za-z]/) { drop = 0 }
  (in_compact && /^    artifacts:[ \t]*$/) { print; drop = 1; next }
  (in_compact && drop)   { next }
                         { print }
' "$REGISTRY" >"$empty_reg"
# Scope the fixture-validity check to the compact form's ARTIFACTS sub-block.
# The full form also carries a `- id: bug.md` line, so a whole-file grep would
# pass on an edit that never applied; and since BUG-042 the compact block also
# holds obligation `- id:` lines, so a whole-BLOCK grep counts those and reports
# artifacts that are no longer there. Either way the assertion below would be
# vacuous.
compact_ids="$(awk '
  /^[A-Za-z]/            { in_compact = 0; in_artifacts = 0 }
  /^  - form: /          { in_artifacts = 0; in_compact = ($0 == "  - form: compact") ? 1 : 0; next }
  (in_compact && in_artifacts && /^    [A-Za-z]/) { in_artifacts = 0 }
  (in_compact && /^    artifacts:[ \t]*$/) { in_artifacts = 1; next }
  (in_compact && in_artifacts) { print }
' "$empty_reg" | grep -c '^      - id: ' || true)"
if [[ "$compact_ids" -ne 0 ]]; then
  bad "A1 fixture actually empties the compact artifact list" "the awk edit did not apply ($compact_ids artifact id line(s) remain); the assertion below would be vacuous"
else
  empty_out="$(bash "$RESOLVER" --registry "$empty_reg" 2>&1)"
  rc=$?
  if [[ "$rc" -ne 0 ]] && printf '%s' "$empty_out" | grep -q 'zero artifacts'; then
    ok "A1 a form declaring zero artifacts is refused (exit $rc)"
  else
    bad "A1 empty artifact set refused" "rc=$rc out=$(printf '%s' "$empty_out" | tr '\n' '|')"
  fi
fi

# --- A2. ADVERSARIAL: an absent registry REFUSES rather than degrades ------
absent_out="$(bash "$RESOLVER" --registry "$WORK/does-not-exist.yaml" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] && [[ -z "$(printf '%s' "$absent_out" | grep '^form=' || true)" ]]; then
  ok "A2 an absent registry exits non-zero and emits no facts (exit $rc)"
else
  bad "A2 absent registry refused" "rc=$rc out=$(printf '%s' "$absent_out" | tr '\n' '|')"
fi

# --- A3. ADVERSARIAL: the absent-default FAILS CLOSED ----------------------
# If the absent-default were `compact`, a packet that declares nothing would
# silently lose four artifacts. The direction of the default is the entire
# safety argument, so it is asserted by value, not merely by presence.
if [[ "$decl_default" == "full" ]]; then
  ok "A3 the absent-default is 'full', so silence cannot reduce a requirement"
else
  bad "A3 absent-default fails closed" "absent-default is '$decl_default', not 'full'"
fi

# --- A4. ADVERSARIAL: no bypass flag is accepted --------------------------
bypassed=""
for flag in --skip --force --ignore --no-verify; do
  bash "$RESOLVER" "$flag" >/dev/null 2>&1
  [[ $? -eq 2 ]] || bypassed="$bypassed $flag"
done
if [[ -z "$bypassed" ]]; then
  ok "A4 every bypass-shaped flag is rejected as a usage error"
else
  bad "A4 no bypass flag accepted" "accepted:$bypassed"
fi

# --- P7. BUG-042: the compact form emits its obligations, WITH carriers ----
# `obligationsRetained:` had a tidy structure and zero consumers before BUG-042,
# so the carrier fields are what make the block real. Asserting only that four
# lines appear would stay green if both carriers resolved empty, which is the
# shape that silently disables the guard's obligation basis.
compact_obl="$(printf '%s\n' "$facts" | grep '^obligation=compact|' || true)"
compact_obl_n="$(printf '%s\n' "$compact_obl" | grep -c '^obligation=compact|' || true)"
missing_obl=""
for pair in \
  'reproduce-before-fix|report.md|report.md' \
  'adversarial-regression|report.md|report.md' \
  'root-cause-stated|bug.md|report.md' \
  'evidence-is-execution|report.md|report.md'; do
  printf '%s\n' "$compact_obl" | grep -qx "obligation=compact|$pair" || missing_obl="$missing_obl ${pair%%|*}"
done
if [[ "$compact_obl_n" -eq 4 ]] && [[ -z "$missing_obl" ]]; then
  ok "P7 compact emits all 4 obligations with their declared discharge and attestation carriers"
else
  bad "P7 compact obligations with carriers" "n=$compact_obl_n missing:$missing_obl out=$(printf '%s' "$compact_obl" | tr '\n' '|')"
fi

# --- P8. BUG-042: the carriers are NOT uniform ----------------------------
# micro-fix-packet.yaml's requirement text names bug.md for the root cause and
# report.md for the other three. If a later edit flattened every dischargedIn to
# report.md the block would still parse, still emit four lines, and still pass
# P7's count — while quietly asserting that the root cause is stated somewhere
# it is not. The asymmetry is the fact worth pinning.
root_discharge="$(printf '%s\n' "$facts" | sed -n 's/^obligation=compact|root-cause-stated|\([^|]*\)|.*/\1/p' | head -1)"
repro_discharge="$(printf '%s\n' "$facts" | sed -n 's/^obligation=compact|reproduce-before-fix|\([^|]*\)|.*/\1/p' | head -1)"
if [[ "$root_discharge" == "bug.md" ]] && [[ "$repro_discharge" == "report.md" ]]; then
  ok "P8 dischargedIn follows micro-fix-packet.yaml's own text (root-cause in bug.md, reproduction in report.md)"
else
  bad "P8 dischargedIn is not flattened" "root-cause-stated=$root_discharge reproduce-before-fix=$repro_discharge"
fi

# --- P9. BUG-042: single-file still resolves, with EMPTY carriers ----------
# The new block selector must not break the form that already used the field.
# single-file has one artifact, so its carrier is unambiguous and the fields are
# honestly empty rather than invented.
sf_obl_n="$(printf '%s\n' "$facts" | grep -c '^obligation=single-file|' || true)"
sf_bad_carriers="$(printf '%s\n' "$facts" | grep '^obligation=single-file|' | grep -vc '||$' || true)"
if [[ "$sf_obl_n" -eq 4 ]] && [[ "$sf_bad_carriers" -eq 0 ]]; then
  ok "P9 single-file still resolves its 4 obligations, with empty carrier fields"
else
  bad "P9 single-file obligations" "n=$sf_obl_n non-empty-carriers=$sf_bad_carriers"
fi

# --- A5. ADVERSARIAL: a REDUCED form declaring ZERO obligations is REFUSED -
# Fewer artifacts is the contract; fewer obligations is the loophole. Without
# this refusal a form could be stripped to three artifacts and no obligations,
# and the transition guard's obligation basis would then certify it against an
# EMPTY required set — a false PASS wearing the shape of a stronger check. This
# is the A1 of BUG-042 and is the single most important assertion added here.
noobl_reg="$WORK/no-obligations.yaml"
# Block-aware for the same reason A1 is: the entries carry no fixed text this
# fixture should depend on.
awk '
  /^[A-Za-z]/            { in_compact = 0; drop = 0 }
  /^  - form: /          { drop = 0; in_compact = ($0 == "  - form: compact") ? 1 : 0 }
  (in_compact && drop && /^    [A-Za-z]/) { drop = 0 }
  (in_compact && /^    obligationsRetained:[ \t]*$/) { drop = 1; next }
  (in_compact && drop)   { next }
                         { print }
' "$REGISTRY" >"$noobl_reg"
# Fixture validity, scoped to the compact BLOCK and to the KEY line. single-file
# also carries an obligationsRetained key, so a whole-file grep would pass on an
# edit that never applied; and the compact block's own comments NAME the key, so
# a substring grep counts prose and reports a removal that did not happen.
noobl_compact="$(awk '/^  - form: compact$/{c=1;next} c&&/^  - form: /{c=0} c' "$noobl_reg")"
noobl_compact_obl="$(printf '%s\n' "$noobl_compact" | grep -cx '    obligationsRetained:' || true)"
noobl_compact_ids="$(printf '%s\n' "$noobl_compact" | grep -c '^      - id: ' || true)"
if [[ "$noobl_compact_obl" -ne 0 ]] || [[ "$noobl_compact_ids" -ne 3 ]]; then
  bad "A5 fixture actually removes the compact obligations" "the awk edit did not apply (obligationsRetained key line=$noobl_compact_obl, id lines=$noobl_compact_ids, expected 0 and 3); the assertion below would be vacuous"
else
  noobl_out="$(bash "$RESOLVER" --registry "$noobl_reg" 2>&1)"
  rc=$?
  if [[ "$rc" -ne 0 ]] && printf '%s' "$noobl_out" | grep -q 'ZERO obligationsRetained'; then
    ok "A5 a reduced form declaring zero obligations is refused (exit $rc)"
  else
    bad "A5 zero-obligation reduced form refused" "rc=$rc out=$(printf '%s' "$noobl_out" | tr '\n' '|')"
  fi
fi

# --- P5. the resolver is CONSUMED by a production surface ------------------
# The defect this selftest guards was not a wrong list, it was an unread
# contract. A resolver nobody calls reproduces it exactly.
# Exclusions match the BASENAME, never the path: v5.3-selftest.sh synthesizes its
# downstream tree under `mktemp -d -t bubbles-v5.3-selftest.XXXXXX`, so a
# path-wide `grep -v selftest` discards every installed script and reports zero
# readers on a tree that in fact has two.
readers=0
while IFS= read -r reader_path; do
  [[ -n "$reader_path" ]] || continue
  reader_name="${reader_path##*/}"
  case "$reader_name" in
    *selftest* | bug-packet-resolve.sh) continue ;;
  esac
  readers=$((readers + 1))
done < <(grep -l 'bug-packet-resolve\.sh' "$SCRIPT_DIR"/*.sh 2>/dev/null)
if [[ "$readers" -ge 1 ]]; then
  ok "P5 $readers non-selftest surface(s) call bug-packet-resolve.sh"
else
  bad "P5 the resolver has a production consumer" "only selftests call it, which is the original defect"
fi

# --- P6. the NAMED consumers are both of them ------------------------------
# BUG-041 F-041-02. P5 counts readers, so it stays green while any ONE reader
# survives. Both duplicate hard-coded artifact lists were replaced: artifact-lint.sh
# answers the lint question and state-transition-guard.sh answers the transition
# question. If either reverts to a private list the two surfaces disagree again,
# which is the drift this resolver exists to end, and P5 alone would not see it.
for consumer in artifact-lint.sh state-transition-guard.sh; do
  if grep -q 'bug-packet-resolve\.sh' "$SCRIPT_DIR/$consumer" 2>/dev/null; then
    ok "P6 $consumer reads the artifact set through bug-packet-resolve.sh"
  else
    bad "P6 $consumer reads bug-packet-resolve.sh" "it carries a private artifact list again"
  fi
done

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
