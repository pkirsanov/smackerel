#!/usr/bin/env bash
# bubbles/scripts/acceptance-authority-selftest.sh
#
# Capability: human-acceptance-authority
#
# Hermetic selftest for the acceptance authority: IMP-047 PD-12 introduced the
# surface, BUG-037 inverted its contract to OPT-OUT acceptance.
#
# THE CONTRACT THIS PROVES
# The `## Checklist` ships CHECKED. A user who reviews the delivered behavior
# and objects to nothing performs NO act, and that silence is acceptance. A user
# who does object UNCHECKS the item, and an unchecked item at a terminal
# transition still refuses and is still NAMED — that is the BUG-029 closure and
# it survives the inversion untouched. `## Human Acceptance Record` is OPTIONAL
# and no longer demanded at terminal, but every shape rule still applies when it
# IS authored. `## Automation Readiness` still grants nothing.
#
# WHY THE ADVERSARIAL CASES ARE THE POINT. Every case marked ADVERSARIAL below
# fails if the fix is reverted OR over-applied. An inversion that widened into
# "terminal never checks acceptance" would pass a happy-path suite and would
# have reopened BUG-029 silently.
#
# Exit codes:
#   0 = all cases pass
#   1 = at least one case failed

set -uo pipefail

NAME="acceptance-authority-selftest"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LIB="$SCRIPT_DIR/acceptance-authority-lib.sh"
REGISTRY="$SCRIPT_DIR/../registry/acceptance-authority.yaml"
BUG_PACKET_REGISTRY="$SCRIPT_DIR/../registry/bug-packet.yaml"
GATES_REGISTRY="$SCRIPT_DIR/../registry/gates.yaml"
CONTROL_PLANE_GUARD="$SCRIPT_DIR/guards/control-plane-checks.sh"
TAIL_GUARD="$SCRIPT_DIR/guards/tail-delegated-gates.sh"
ARTIFACT_LINT="$SCRIPT_DIR/artifact-lint.sh"
TEMPLATES="$REPO_ROOT/agents/bubbles_shared/feature-templates.md"
CHANGELOG="$REPO_ROOT/CHANGELOG.md"

# The governance-conformance block at the end asserts facts about the bubbles
# SOURCE repository (its CHANGELOG entries, its bug packets). A downstream
# consumer ships the library and this selftest but not those artifacts, so the
# block is scoped by this marker rather than by silently passing when a file it
# needs is missing.
SOURCE_REPO_MARKER="$REPO_ROOT/bugs/BUG-037-uservalidation-opt-out-acceptance"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/$NAME.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

checks=0
failures=0
ok() {
  checks=$((checks + 1))
  printf '  ok   %s\n' "$1"
}
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -lt 2 ]] || printf '       %s\n' "$2"
}

[[ -f "$LIB" ]] || {
  printf '%s: library not found: %s\n' "$NAME" "$LIB" >&2
  exit 1
}
[[ -f "$REGISTRY" ]] || {
  printf '%s: registry not found: %s\n' "$NAME" "$REGISTRY" >&2
  exit 1
}

# shellcheck source=acceptance-authority-lib.sh
source "$LIB"

printf '%s: %s\n' "$NAME" "$REGISTRY"

write_file() {
  # Deliberately a heredoc into a file the TEST owns under mktemp. The
  # repository working tree is never written by this script.
  local path="$1"
  shift
  mkdir -p "$(dirname "$path")"
  printf '%s\n' "$@" > "$path"
}

# Replace a line via a temp copy. `sed -i` is deliberately avoided: GNU wants
# `-i` bare and BSD wants `-i ''`, and this selftest runs on both.
rewrite() {
  local file="$1" expr="$2"
  sed -e "$expr" "$file" > "$file.new" && mv "$file.new" "$file"
}

authority_failure_pair_findings() {
  local registry="$1" file="$2" verdict out rc line_count code_count
  for verdict in bubbles_acceptance_shape_verdict bubbles_acceptance_terminal_verdict; do
    out="$(BUBBLES_ACCEPTANCE_REGISTRY="$registry" "$verdict" "$file" 2>&1)"
    rc=$?
    if [[ -n "$out" ]]; then
      line_count="$(printf '%s\n' "$out" | awk 'END {print NR}')"
    else
      line_count=0
    fi
    code_count="$({ printf '%s\n' "$out" | grep -c '^PD12-AUTHORITY-UNAVAILABLE: '; } || true)"
    if [[ "$rc" -ne 1 || "$line_count" -ne 1 || "$code_count" -ne 1 || "$out" == *"$WORK"* ]]; then
      printf '%s: exit=%s lines=%s bootstrapCodes=%s output=%s\n' \
        "$verdict" "$rc" "$line_count" "$code_count" "${out:-<empty>}"
    fi
  done
}

failure_code_lifecycle_findings() {
  local registry="$1" actual expected
  actual="$(awk '
    /^failureCodes:/ {f=1; next}
    f && /^[a-zA-Z]/ {exit}
    f && /^  - id: / {
      code=$0
      sub(/^  - id: /, "", code)
      next
    }
    f && /^    lifecycle: / && code != "" {
      lifecycle=$0
      sub(/^    lifecycle: /, "", lifecycle)
      print lifecycle "|" code
      code=""
    }
  ' "$registry" | LC_ALL=C sort)"
  expected="$(printf '%s\n' \
    'bootstrap-active|PD12-AUTHORITY-UNAVAILABLE' \
    'conditional-active|PD12-NO-RECORD' \
    'default-active|PD12-AUTOMATION-ACCEPTOR' \
    'default-active|PD12-METHOD-FIELD-MISSING' \
    'default-active|PD12-METHOD-UNKNOWN' \
    'default-active|PD12-READINESS-NOT-CHECKBOX' \
    'default-active|PD12-RECORD-INCOMPLETE' \
    'default-active|PD12-UNCHECKED-ITEM' | LC_ALL=C sort)"
  if [[ "$actual" != "$expected" ]]; then
    printf 'expected: %s | actual: %s\n' \
      "$(printf '%s' "$expected" | tr '\n' ' ')" \
      "$(printf '%s' "$actual" | tr '\n' ' ')"
  fi
}

# --- fixture bodies ----------------------------------------------------------

# The shape as it ships after BUG-037: readiness separate, acceptance CHECKED,
# no record at all. This is what a satisfied user leaves behind by doing nothing.
shipped_shape() {
  write_file "$1" \
    '# User Validation Checklist' \
    '' \
    '## Automation Readiness' \
    '' \
    '- [x] Search returns results on the production route' \
    '' \
    '## Checklist' \
    '' \
    '- [x] Search returns results for a plain keyword query' \
    '- [x] An empty query renders the guidance state instead of an error'
}

# Five accepted behaviors and ONE the user rejected.
user_rejected_one() {
  write_file "$1" \
    '# User Validation Checklist' \
    '' \
    '## Checklist' \
    '' \
    '- [x] Search returns results for a plain keyword query' \
    '- [x] An empty query renders the guidance state' \
    '- [x] Results paginate at twenty per page' \
    '- [x] A no-match query renders the empty state' \
    '- [x] The result count matches the rendered rows' \
    '- [ ] Deleting a record removes it from the result list'
}

# The BUG-029 shape: one checked, five unchecked.
bug029_shape() {
  write_file "$1" \
    '# User Validation Checklist' \
    '' \
    '## Checklist' \
    '' \
    '- [x] Search returns results for a plain keyword query' \
    '- [ ] An empty query renders the guidance state' \
    '- [ ] Results paginate at twenty per page' \
    '- [ ] A no-match query renders the empty state' \
    '- [ ] The result count matches the rendered rows' \
    '- [ ] Deleting a record removes it from the result list'
}

# Checked checklist PLUS an authored record — the optional external-UAT shape.
recorded_file() {
  write_file "$1" \
    '# User Validation Checklist' \
    '' \
    '## Automation Readiness' \
    '' \
    '- [x] Search returns results on the production route' \
    '' \
    '## Checklist' \
    '' \
    '- [x] Search returns results' \
    '' \
    '## Human Acceptance Record' \
    '' \
    '- acceptedBy: p.kirsanov' \
    '- acceptedAt: 2026-08-16T10:00:00Z' \
    '- method: human-interactive'
}

# =============================================================================
# SCOPE 1 — the inverted contract and the shared reader
# =============================================================================

# --- S1-T1: fully checked, no record, reaches terminal -----------------------
# SCN-B037-001. The owner's normal case: the user unchecked nothing, so there is
# nothing to refuse. Under the pre-BUG-037 contract this file was refused with
# PD12-NO-RECORD and a satisfied user could not reach terminal at all.
f1="$WORK/1/uservalidation.md"
shipped_shape "$f1"
out="$(bubbles_acceptance_terminal_verdict "$f1" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 && -z "$out" ]]; then
  ok "S1-T1 SCN-B037-001: a fully checked checklist with no record reaches terminal"
else
  bad "S1-T1 fully checked, no record, reaches terminal" "exit $rc: $out"
fi

if printf '%s' "$out" | grep -q 'PD12-NO-RECORD'; then
  bad "S1-T1b no PD12-NO-RECORD finding is emitted" "$out"
else
  ok "S1-T1b SCN-B037-001: no PD12-NO-RECORD finding is emitted"
fi

out="$(bubbles_acceptance_shape_verdict "$f1" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 && -z "$out" ]]; then
  ok "S1-T1c the shipped (checked) shape also passes shape lint"
else
  bad "S1-T1c shipped shape passes shape lint" "exit $rc: $out"
fi

# --- S1-T2 ADVERSARIAL: one uncheck among five still refuses -----------------
# SCN-B037-002. This is the case that proves the inversion did not widen into
# "terminal never checks acceptance". A user's uncheck is a reported regression.
f2="$WORK/2/uservalidation.md"
user_rejected_one "$f2"
out="$(bubbles_acceptance_terminal_verdict "$f2" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] &&
  printf '%s' "$out" | grep -q 'PD12-UNCHECKED-ITEM: - \[ \] Deleting a record removes it from the result list'; then
  ok "S1-T2 SCN-B037-002 ADVERSARIAL: one unchecked item refuses terminal and is NAMED"
else
  bad "S1-T2 one unchecked item refuses and is named" "exit $rc: $out"
fi

f2_named="$(printf '%s\n' "$out" | grep -c 'PD12-UNCHECKED-ITEM' || true)"
if [[ "$f2_named" -eq 1 ]]; then
  ok "S1-T2b exactly the rejected item is reported, not the five accepted ones"
else
  bad "S1-T2b only the rejected item is reported" "reported $f2_named items: $out"
fi

# --- S1-T3 ADVERSARIAL: the BUG-029 pin --------------------------------------
# SCN-B037-003. One checked plus five unchecked. If BUG-037 reopened BUG-029,
# this case catches it — and it must name EVERY unchecked item, because a
# refusal that names only the first leaves four rejected behaviors invisible.
f3="$WORK/3/uservalidation.md"
bug029_shape "$f3"
out="$(bubbles_acceptance_terminal_verdict "$f3" 2>&1)"
rc=$?
f3_named="$(printf '%s\n' "$out" | grep -c 'PD12-UNCHECKED-ITEM' || true)"
if [[ "$rc" -ne 0 && "$f3_named" -eq 5 ]]; then
  ok "S1-T3 SCN-B037-003 ADVERSARIAL: the BUG-029 shape is refused and all five unchecked items are named"
else
  bad "S1-T3 BUG-029 pin intact" "exit $rc, named $f3_named of 5: $out"
fi

# --- S1-T4 ADVERSARIAL: an agent can never be the acceptor -------------------
# SCN-B037-004. The record became OPTIONAL; it did NOT become unvalidated. An
# agent authoring a record that named itself would be accepting on a human's
# behalf, which is the one act the record surface exists to prevent.
f4="$WORK/4/uservalidation.md"
recorded_file "$f4"
rewrite "$f4" 's/acceptedBy: p.kirsanov/acceptedBy: bubbles.validate/'
out="$(bubbles_acceptance_shape_verdict "$f4" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] && printf '%s' "$out" | grep -q 'PD12-AUTOMATION-ACCEPTOR'; then
  ok "S1-T4 SCN-B037-004 ADVERSARIAL: an agent id as acceptedBy still refuses the shape verdict"
else
  bad "S1-T4 automation acceptor still refused" "exit $rc: $out"
fi

# --- S1-T5: a present record missing a required field ------------------------
f5="$WORK/5/uservalidation.md"
recorded_file "$f5"
rewrite "$f5" '/acceptedAt: /d'
out="$(bubbles_acceptance_shape_verdict "$f5" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] && printf '%s' "$out" | grep -q 'PD12-RECORD-INCOMPLETE: human acceptance record has no acceptedAt'; then
  ok "S1-T5 a present record missing acceptedAt is PD12-RECORD-INCOMPLETE"
else
  bad "S1-T5 incomplete record refused" "exit $rc: $out"
fi

# --- S1-T6: external-record without its pointer ------------------------------
f6="$WORK/6/uservalidation.md"
recorded_file "$f6"
rewrite "$f6" 's/method: human-interactive/method: external-record/'
out="$(bubbles_acceptance_shape_verdict "$f6" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] && printf '%s' "$out" | grep -q 'PD12-METHOD-FIELD-MISSING'; then
  ok "S1-T6 external-record acceptance without its record pointer is refused"
else
  bad "S1-T6 external-record requires its pointer" "exit $rc: $out"
fi

f6b="$WORK/6b/uservalidation.md"
recorded_file "$f6b"
rewrite "$f6b" 's/method: human-interactive/method: external-record/'
printf -- '- record: UAT-2026-08-16\n' >> "$f6b"
out="$(bubbles_acceptance_terminal_verdict "$f6b" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]]; then
  ok "S1-T6b external-record acceptance WITH its record pointer is accepted"
else
  bad "S1-T6b external-record with pointer accepted" "exit $rc: $out"
fi

f6c="$WORK/6c/uservalidation.md"
recorded_file "$f6c"
rewrite "$f6c" 's/method: human-interactive/method: assumed-accepted/'
out="$(bubbles_acceptance_shape_verdict "$f6c" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] && printf '%s' "$out" | grep -q 'PD12-METHOD-UNKNOWN'; then
  ok "S1-T6c an acceptance method outside the closed vocabulary is refused"
else
  bad "S1-T6c unknown method refused" "exit $rc: $out"
fi

# --- S1-T7 ADVERSARIAL: readiness grants nothing -----------------------------
# SCN-B037-005. The whole separation is worthless if the section automation OWNS
# can discharge the obligation automation must never discharge.
f7="$WORK/7/uservalidation.md"
write_file "$f7" \
  '# User Validation Checklist' \
  '' \
  '## Automation Readiness' \
  '' \
  '- [x] Every behavior verified by automation' \
  '- [x] Every route exercised' \
  '' \
  '## Checklist' \
  '' \
  '- [x] Search returns results' \
  '- [ ] Deleting a record removes it from the result list'
out="$(bubbles_acceptance_terminal_verdict "$f7" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] &&
  printf '%s' "$out" | grep -q 'PD12-UNCHECKED-ITEM: - \[ \] Deleting a record'; then
  ok "S1-T7 SCN-B037-005 ADVERSARIAL: a fully checked readiness block discharges no acceptance obligation"
else
  bad "S1-T7 readiness grants no acceptance" "exit $rc: $out"
fi

f7b="$WORK/7b/uservalidation.md"
write_file "$f7b" \
  '# User Validation Checklist' \
  '' \
  '## Automation Readiness' \
  '' \
  '- everything looks fine' \
  '' \
  '## Checklist' \
  '' \
  '- [x] Search returns results'
out="$(bubbles_acceptance_shape_verdict "$f7b" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] && printf '%s' "$out" | grep -q 'PD12-READINESS-NOT-CHECKBOX'; then
  ok "S1-T7b a non-checkbox automation-readiness bullet is refused"
else
  bad "S1-T7b readiness bullets must be checkboxes" "exit $rc: $out"
fi

# --- S1-T8 ADVERSARIAL: the terminal record demand is REGISTRY DATA ----------
# The load-bearing case. Without it the implementation could satisfy every other
# test by DELETING the record check outright while claiming to read the registry.
# Flipping `requiredAtTerminal` back to `true` in a fixture registry must restore
# the refusal on the exact file S1-T1 accepts. That proves the shipped `false` is
# a contract decision this library READS, not a behavior it hardcoded.
alt_required="$WORK/alt-required-registry.yaml"
sed -e 's/^    requiredAtTerminal: false$/    requiredAtTerminal: true/' "$REGISTRY" > "$alt_required"
if grep -q '^    requiredAtTerminal: true$' "$alt_required"; then
  out="$(BUBBLES_ACCEPTANCE_REGISTRY="$alt_required" bash -c '
    . "$1"; bubbles_acceptance_terminal_verdict "$2"' _ "$LIB" "$f1" 2>&1)"
  rc=$?
  out_shipped="$(bubbles_acceptance_terminal_verdict "$f1" 2>&1 || true)"
  if [[ "$rc" -ne 0 ]] &&
    printf '%s' "$out" | grep -q 'PD12-NO-RECORD' &&
    [[ -z "$out_shipped" ]]; then
    ok "S1-T8 ADVERSARIAL: flipping requiredAtTerminal back in a fixture registry restores the refusal (the value is READ, not hardcoded)"
  else
    bad "S1-T8 requiredAtTerminal is read from the registry" "alt exit $rc: $out | shipped: $out_shipped"
  fi
else
  bad "S1-T8 fixture registry could not be built" "no 'requiredAtTerminal: false' line in $REGISTRY"
fi

# --- S1-T8b ADVERSARIAL: the headings are registry data too ------------------
# Rename the acceptance-record heading in a COPY. The library must follow it. If
# this fails, the shape has been duplicated in the script and the two copies will
# drift, which is the failure the shared library exists to prevent.
alt_heading="$WORK/alt-heading-registry.yaml"
sed -e 's|heading: "## Human Acceptance Record"|heading: "## Signed Off By A Human"|' \
  -e 's/^    requiredAtTerminal: false$/    requiredAtTerminal: true/' "$REGISTRY" > "$alt_heading"
f8b="$WORK/8b/uservalidation.md"
recorded_file "$f8b"
rewrite "$f8b" 's|## Human Acceptance Record|## Signed Off By A Human|'
out="$(BUBBLES_ACCEPTANCE_REGISTRY="$alt_heading" bash -c '
  . "$1"; bubbles_acceptance_terminal_verdict "$2"' _ "$LIB" "$f8b" 2>&1)"
rc=$?
out_original="$(BUBBLES_ACCEPTANCE_REGISTRY="$alt_required" bash -c '
  . "$1"; bubbles_acceptance_terminal_verdict "$2"' _ "$LIB" "$f8b" 2>&1 || true)"
if [[ "$rc" -eq 0 ]] && printf '%s' "$out_original" | grep -q 'PD12-NO-RECORD'; then
  ok "S1-T8b renaming the heading in the registry changes the reader (single source, not a copy)"
else
  bad "S1-T8b library reads the registry headings" "renamed exit $rc: $out | original heading: $out_original"
fi

# --- S1-T9: the section parser does not over-reach ---------------------------
f9="$WORK/9/uservalidation.md"
shipped_shape "$f9"
printf '\n## Notes\n\n- [ ] follow up on copy\n' >> "$f9"
out="$(bubbles_acceptance_terminal_verdict "$f9" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]]; then
  ok "S1-T9 an unchecked bullet outside the acceptance checklist is ignored"
else
  bad "S1-T9 section parser does not over-reach" "exit $rc: $out"
fi

# --- S1-T11 through S1-T15: authority failures always fail closed -----------
# SCN-B037-016. Both public verdicts must stop before artifact evaluation when
# the authority cannot be trusted. Every case requires one deterministic
# bootstrap line, exit 1, and no fixture path in the diagnostic.
missing_registry="$WORK/authority/missing.yaml"
authority_findings="$(authority_failure_pair_findings "$missing_registry" "$f1")"
if [[ -z "$authority_findings" ]]; then
  ok "S1-T11 SCN-B037-016: missing authority fails closed"
else
  bad "S1-T11 missing authority fails closed" "$authority_findings"
fi

unreadable_registry="$WORK/authority/unreadable.yaml"
mkdir -p "$(dirname "$unreadable_registry")"
cp "$REGISTRY" "$unreadable_registry"
chmod 000 "$unreadable_registry"
authority_findings="$(authority_failure_pair_findings "$unreadable_registry" "$f1")"
chmod 600 "$unreadable_registry"
if [[ -z "$authority_findings" ]]; then
  ok "S1-T12 SCN-B037-016: unreadable authority fails closed"
else
  bad "S1-T12 unreadable authority fails closed" "$authority_findings"
fi

malformed_registry="$WORK/authority/malformed.yaml"
write_file "$malformed_registry" \
  'schemaVersion acceptance-authority/v1' \
  'sections: [' \
  'this is not the acceptance authority contract'
authority_findings="$(authority_failure_pair_findings "$malformed_registry" "$f1")"
if [[ -z "$authority_findings" ]]; then
  ok "S1-T13 SCN-B037-016: malformed authority fails closed"
else
  bad "S1-T13 malformed authority fails closed" "$authority_findings"
fi

syntax_invalid_registry="$WORK/authority/syntax-invalid-contract-complete.yaml"
cp "$REGISTRY" "$syntax_invalid_registry"
printf '%s\n' 'syntaxInvalid: [' >> "$syntax_invalid_registry"
authority_findings="$(authority_failure_pair_findings "$syntax_invalid_registry" "$f1")"
if [[ -z "$authority_findings" ]]; then
  ok "S1-T13b SCN-B037-016: contract-complete syntactically invalid YAML fails closed"
else
  bad "S1-T13b contract-complete syntactically invalid YAML fails closed" "$authority_findings"
fi

s1t14_findings=""
for fixture_spec in \
  'schema|/^schemaVersion:/d' \
  'section-heading|/heading: "## Automation Readiness"/d' \
  'shipped-state|/shippedState: checked/d' \
  'base-field|/    - acceptedAt/d' \
  'method|/    - id: external-record/d' \
  'method-field|/      requiresField: record/d' \
  'acceptor-pattern|/  pattern:/d' \
  'failure-code|/  - id: PD12-UNCHECKED-ITEM/d'; do
  fixture_name="${fixture_spec%%|*}"
  fixture_expr="${fixture_spec#*|}"
  fixture_registry="$WORK/authority/missing-$fixture_name.yaml"
  cp "$REGISTRY" "$fixture_registry"
  rewrite "$fixture_registry" "$fixture_expr"
  fixture_findings="$(authority_failure_pair_findings "$fixture_registry" "$f1")"
  if [[ -n "$fixture_findings" ]]; then
    s1t14_findings="${s1t14_findings}${fixture_name}: ${fixture_findings}"$'\n'
  fi
done
if [[ -z "$s1t14_findings" ]]; then
  ok "S1-T14 SCN-B037-016: missing required authority fields fail closed"
else
  bad "S1-T14 missing required authority fields fail closed" "$s1t14_findings"
fi

s1t15_findings=""
invalid_boolean_index=0
for invalid_boolean in '__missing__' '' '"false"' '0' 'yes' 'no'; do
  invalid_boolean_index=$((invalid_boolean_index + 1))
  fixture_registry="$WORK/authority/invalid-boolean-$invalid_boolean_index.yaml"
  cp "$REGISTRY" "$fixture_registry"
  if [[ "$invalid_boolean" == "__missing__" ]]; then
    rewrite "$fixture_registry" '/requiredAtTerminal:/d'
  else
    rewrite "$fixture_registry" "s/^    requiredAtTerminal: false$/    requiredAtTerminal: $invalid_boolean/"
  fi
  fixture_findings="$(authority_failure_pair_findings "$fixture_registry" "$f1")"
  if [[ -n "$fixture_findings" ]]; then
    s1t15_findings="${s1t15_findings}value=${invalid_boolean:-<empty>}: ${fixture_findings}"$'\n'
  fi
done
if [[ -z "$s1t15_findings" ]]; then
  ok "S1-T15 SCN-B037-016: invalid requiredAtTerminal values fail closed"
else
  bad "S1-T15 invalid requiredAtTerminal values fail closed" "$s1t15_findings"
fi

# --- S1-T16 through S1-T18: every recognized field can author the record ----
# SCN-B037-017. A method-conditional pointer is real authored input even when
# the three base fields still contain template tokens.
f16="$WORK/16/uservalidation.md"
write_file "$f16" \
  '# User Validation Checklist' \
  '' \
  '## Checklist' \
  '' \
  '- [x] Search returns results' \
  '' \
  '## Human Acceptance Record' \
  '' \
  '- acceptedBy: [human name or handle]' \
  '- acceptedAt: [ISO-8601 timestamp]' \
  '- method: [human-interactive | external-record]' \
  '- record: UAT-2026-09-01'
out="$(bubbles_acceptance_shape_verdict "$f16" 2>&1)"
rc=$?
s1t16_incomplete_count="$({ printf '%s\n' "$out" | grep -c '^PD12-RECORD-INCOMPLETE:'; } || true)"
if [[ "$rc" -eq 1 && "$s1t16_incomplete_count" -eq 3 ]] &&
  printf '%s\n' "$out" | grep -q 'no acceptedBy' &&
  printf '%s\n' "$out" | grep -q 'no acceptedAt' &&
  printf '%s\n' "$out" | grep -q 'no method'; then
  ok "S1-T16 SCN-B037-017: a real record pointer alone authors the record"
else
  bad "S1-T16 record-only authorship exposes every missing base field" "exit $rc, incomplete=$s1t16_incomplete_count: $out"
fi

f17="$WORK/17/uservalidation.md"
write_file "$f17" \
  '# User Validation Checklist' \
  '' \
  '## Checklist' \
  '' \
  '- [x] Search returns results' \
  '' \
  '## Human Acceptance Record' \
  '' \
  '- acceptedBy: [human name or handle]' \
  '- acceptedAt:' \
  '- method: [human-interactive | external-record]' \
  '- record: [external acceptance record]'
out="$(bubbles_acceptance_shape_verdict "$f17" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 && -z "$out" ]] && ! bubbles_acceptance_record_authored "$f17"; then
  ok "S1-T17 SCN-B037-017: untouched defaults remain unauthored"
else
  bad "S1-T17 empty and complete bracket defaults remain inert" "exit $rc: ${out:-<empty>}"
fi

f18_human="$WORK/18-human/uservalidation.md"
recorded_file "$f18_human"
human_out="$(bubbles_acceptance_shape_verdict "$f18_human" 2>&1)"
human_rc=$?
f18_external="$WORK/18-external/uservalidation.md"
recorded_file "$f18_external"
rewrite "$f18_external" 's/method: human-interactive/method: external-record/'
external_missing_out="$(bubbles_acceptance_shape_verdict "$f18_external" 2>&1)"
external_missing_rc=$?
printf -- '- record: UAT-2026-09-01\n' >> "$f18_external"
external_complete_out="$(bubbles_acceptance_shape_verdict "$f18_external" 2>&1)"
external_complete_rc=$?
if [[ "$human_rc" -eq 0 && -z "$human_out" &&
  "$external_missing_rc" -eq 1 && "$external_missing_out" == *"PD12-METHOD-FIELD-MISSING"* &&
  "$external_complete_rc" -eq 0 && -z "$external_complete_out" ]]; then
  ok "S1-T18 SCN-B037-017: method-specific fields remain mandatory"
else
  bad "S1-T18 supported method schemas stay distinct" \
    "human=$human_rc:${human_out:-<empty>} external-missing=$external_missing_rc:${external_missing_out:-<empty>} external-complete=$external_complete_rc:${external_complete_out:-<empty>}"
fi

# --- S1-T10 ADVERSARIAL: the failure-code set is closed in BOTH directions ---
# BUG-037 D-5 requires the default, conditional, and bootstrap lifecycle
# classes in one closed set. This catches both halves of the likely mistake: a
# code DECLARED but unemittable, and a code EMITTED but undeclared.
declared_codes="$(awk '
  /^failureCodes:/ {f=1; next}
  f && /^[a-zA-Z]/ {exit}
  f && /^  - id: / {sub(/^  - id: /, ""); print}
' "$REGISTRY" | sort -u)"

observed_codes="$(
  {
    bubbles_acceptance_terminal_verdict "$f1"
    bubbles_acceptance_terminal_verdict "$f2"
    bubbles_acceptance_terminal_verdict "$f3"
    bubbles_acceptance_shape_verdict "$f4"
    bubbles_acceptance_shape_verdict "$f5"
    bubbles_acceptance_shape_verdict "$f6"
    bubbles_acceptance_shape_verdict "$f6c"
    bubbles_acceptance_terminal_verdict "$f7"
    bubbles_acceptance_shape_verdict "$f7b"
    BUBBLES_ACCEPTANCE_REGISTRY="$alt_required" bubbles_acceptance_terminal_verdict "$f1"
    BUBBLES_ACCEPTANCE_REGISTRY="$missing_registry" bubbles_acceptance_shape_verdict "$f1"
  } 2>&1 | sed -n -E 's/^(PD12-[A-Z-]+):.*/\1/p' | sort -u
)"

if [[ "$declared_codes" == "$observed_codes" ]]; then
  ok "S1-T10 ADVERSARIAL: every declared failure code is emittable and every emitted code is declared"
else
  bad "S1-T10 declared code set equals observed code set" \
    "declared: $(printf '%s' "$declared_codes" | tr '\n' ' ') | observed: $(printf '%s' "$observed_codes" | tr '\n' ' ')"
fi

if printf '%s\n' "$declared_codes" | grep -qx 'PD12-NO-RECORD' &&
  printf '%s\n' "$observed_codes" | grep -qx 'PD12-NO-RECORD' &&
  printf '%s\n' "$declared_codes" | grep -qx 'PD12-AUTHORITY-UNAVAILABLE' &&
  printf '%s\n' "$observed_codes" | grep -qx 'PD12-AUTHORITY-UNAVAILABLE'; then
  ok "S1-T10b D-5: conditional and bootstrap codes are declared and reached"
else
  bad "S1-T10b conditional and bootstrap lifecycle classes are closed" \
    "declared: $(printf '%s' "$declared_codes" | tr '\n' ' ') | observed: $(printf '%s' "$observed_codes" | tr '\n' ' ')"
fi

lifecycle_findings="$(failure_code_lifecycle_findings "$REGISTRY")"
if [[ -z "$lifecycle_findings" ]]; then
  ok "S1-T10e SCN-B037-018: failure codes have exact default, conditional, and bootstrap lifecycle labels"
else
  bad "S1-T10e lifecycle labels match the closed authority classification" "$lifecycle_findings"
fi

misclassified_lifecycle="$WORK/authority/misclassified-conditional-lifecycle.yaml"
cp "$alt_required" "$misclassified_lifecycle"
rewrite "$misclassified_lifecycle" \
  '/^  - id: PD12-NO-RECORD$/,/^    lifecycle: conditional-active$/ s/^    lifecycle: conditional-active$/    lifecycle: default-active/'
misclassified_lifecycle_findings="$(failure_code_lifecycle_findings "$misclassified_lifecycle")"
misclassified_preflight_out="$(BUBBLES_ACCEPTANCE_REGISTRY="$misclassified_lifecycle" \
  bubbles_acceptance_authority_preflight 2>&1)"
misclassified_preflight_rc=$?
misclassified_out="$(BUBBLES_ACCEPTANCE_REGISTRY="$misclassified_lifecycle" \
  bubbles_acceptance_terminal_verdict "$f1" 2>&1)"
misclassified_rc=$?
misclassified_line_count="$(printf '%s\n' "$misclassified_out" | awk 'END {print NR}')"
misclassified_code_count="$({ printf '%s\n' "$misclassified_out" | grep -c '^PD12-NO-RECORD: '; } || true)"
if [[ -n "$misclassified_lifecycle_findings" &&
  "$misclassified_preflight_rc" -eq 0 && -z "$misclassified_preflight_out" &&
  "$misclassified_rc" -eq 1 && "$misclassified_line_count" -eq 1 &&
  "$misclassified_code_count" -eq 1 ]]; then
  ok "S1-T10f ADVERSARIAL SCN-B037-018: a conditional lifecycle misclassification fails the label assertion while its code remains reachable"
else
  bad "S1-T10f lifecycle mutation is detected without changing code reachability" \
    "findings=${misclassified_lifecycle_findings:-<empty>} preflight=$misclassified_preflight_rc:${misclassified_preflight_out:-<empty>} verdict=$misclassified_rc lines=$misclassified_line_count codes=$misclassified_code_count output=${misclassified_out:-<empty>}"
fi

conditional_out="$(BUBBLES_ACCEPTANCE_REGISTRY="$alt_required" \
  bubbles_acceptance_terminal_verdict "$f1" 2>&1)"
conditional_rc=$?
conditional_line_count="$(printf '%s\n' "$conditional_out" | awk 'END {print NR}')"
conditional_code_count="$({ printf '%s\n' "$conditional_out" | grep -c '^PD12-NO-RECORD: '; } || true)"
if [[ "$conditional_rc" -eq 1 && "$conditional_line_count" -eq 1 &&
  "$conditional_code_count" -eq 1 ]]; then
  ok "S1-T10d D-5: exact true override emits one conditional PD12-NO-RECORD line"
else
  bad "S1-T10d conditional PD12-NO-RECORD cardinality is exact" \
    "exit=$conditional_rc lines=$conditional_line_count codes=$conditional_code_count output=${conditional_out:-<empty>}"
fi

# No source literal receives a retirement or conditional exception. Supported
# configuration must never produce a refusal absent from the canonical set.
source_codes="$(grep -oE "'PD12-[A-Z-]+" "$LIB" | sed "s/^'//" | sort -u)"
undeclared_source_codes="$(comm -23 <(printf '%s\n' "$source_codes") <(printf '%s\n' "$declared_codes"))"
if [[ -z "$undeclared_source_codes" ]]; then
  ok "S1-T10c every source refusal literal is declared without exceptions"
else
  bad "S1-T10c every source refusal literal is declared" \
    "undeclared: $(printf '%s' "$undeclared_source_codes" | tr '\n' ' ')"
fi

# =============================================================================
# SCOPE 2 — template, lint, and the authoring contract
# =============================================================================

# The uservalidation template's checklist bullets, from a given
# feature-templates.md. Shared by S2-T2 and its adversarial partner S2-T9 so the
# adversarial case cannot pass against a different extractor.
template_checklist_block() {
  awk '
    /^## uservalidation.md Template/ {t=1; next}
    t && /^## Checklist/ {c=1; next}
    c && /^## / {exit}
    c {print}
  ' "$1"
}

template_headings() {
  awk '
    /^## uservalidation.md Template/ {t=1; next}
    t && /^## scenario-manifest/ {exit}
    t && /^## / {print}
  ' "$1"
}

# S2-T2's engine. Prints one finding per disagreement between the template block
# in $1 and the acceptance registry. Silent output means agreement.
template_registry_findings() {
  local templates="$1" heading shipped block wrong_state section
  for section in automation-readiness acceptance-checklist acceptance-record; do
    heading="$(bubbles_acceptance_heading "$section")"
    if ! template_headings "$templates" | grep -qxF "$heading"; then
      printf 'TEMPLATE-HEADING-MISSING: %s (%s)\n' "$heading" "$section"
    fi
  done

  shipped="$(bubbles_acceptance_checklist_shipped_state)"
  block="$(template_checklist_block "$templates")"
  if [[ -z "$({ printf '%s\n' "$block" | grep -E '^- \[(x| )\] '; } || true)" ]]; then
    printf 'TEMPLATE-CHECKLIST-EMPTY: the template checklist carries no checkbox entries\n'
  fi
  case "$shipped" in
    checked) wrong_state="$({ printf '%s\n' "$block" | grep -E '^- \[ \] '; } || true)" ;;
    unchecked) wrong_state="$({ printf '%s\n' "$block" | grep -E '^- \[x\] '; } || true)" ;;
    *)
      printf 'TEMPLATE-SHIPPED-STATE-UNKNOWN: registry declares "%s"\n' "$shipped"
      return 0
      ;;
  esac
  if [[ -n "$wrong_state" ]]; then
    printf 'TEMPLATE-SHIPPED-STATE-MISMATCH: registry declares "%s"; template ships:\n%s\n' \
      "$shipped" "$wrong_state"
  fi
}

# --- S2-T1: a template materialization reaches terminal unaided --------------
# SCN-B037-006. The template is the artifact the defect travelled in. A library
# that accepts a hand-written fixture while the shipped template still ships
# unchecked would close nothing.
if [[ -f "$TEMPLATES" ]]; then
  s2_materialized="$WORK/s2-materialized/uservalidation.md"
  mkdir -p "$(dirname "$s2_materialized")"
  # Fill the placeholder slots the way a real packet does, so no finding can be
  # attributed to an unfilled template stub.
  awk '
    /^## uservalidation.md Template/ {t=1; next}
    t && /^```markdown$/ {inb=1; next}
    inb && /^```$/ {exit}
    inb {print}
  ' "$TEMPLATES" |
    sed -e 's/\[Scenario or flow the human accepts\]/Search returns results for a plain keyword query/' \
      -e 's/\[Another flow the human accepts\]/An empty query renders the guidance state/' \
      -e 's/\[Behavior verified by automation and ready for human acceptance\]/Search route exercised end to end/' \
      -e 's/\[Another verified behavior\]/Empty-query branch exercised/' \
      > "$s2_materialized"
  out="$(bubbles_acceptance_terminal_verdict "$s2_materialized" 2>&1)"
  rc=$?
  if [[ "$rc" -eq 0 && -z "$out" ]]; then
    ok "S2-T1 SCN-B037-006: a uservalidation.md materialized from the shipped template reaches terminal with no human act"
  else
    bad "S2-T1 template materialization reaches terminal" "exit $rc: $out"
  fi
else
  bad "S2-T1 feature-templates.md is readable" "not found: $TEMPLATES"
fi

# --- S2-T2: template <-> registry agreement ----------------------------------
# LOAD-BEARING. BUG-037 D-2 declines to restore the "must carry a checked [x]"
# lint rule, because that rule is the coupling leg of the fabrication
# composition PD-12 diagnosed. This check buys the ONE thing that rule actually
# bought — detection of a template authored in the wrong shipped state — without
# ever refusing a user who legitimately unchecks everything.
if [[ -f "$TEMPLATES" ]]; then
  s2t2_findings="$(template_registry_findings "$TEMPLATES")"
  if [[ -z "$s2t2_findings" ]]; then
    ok "S2-T2 the uservalidation template agrees with acceptance-authority.yaml on section headings AND shippedState"
  else
    bad "S2-T2 template agrees with the registry" "$s2t2_findings"
  fi
else
  bad "S2-T2 feature-templates.md is readable" "not found: $TEMPLATES"
fi

# --- S2-T9 ADVERSARIAL: S2-T2 can actually refuse ----------------------------
# A check that cannot fail proves nothing. Drive the SAME engine over a fixture
# whose template block ships `- [ ]` while the registry declares `checked`.
s2t9_templates="$WORK/s2t9-feature-templates.md"
write_file "$s2t9_templates" \
  '## uservalidation.md Template' \
  '' \
  '```markdown' \
  '# User Validation Checklist' \
  '' \
  '## Automation Readiness' \
  '' \
  '- [ ] [Behavior verified by automation and ready for human acceptance]' \
  '' \
  '## Checklist' \
  '' \
  '- [ ] [Scenario or flow the human accepts]' \
  '' \
  '## Human Acceptance Record' \
  '' \
  '- acceptedBy: [human name or handle]' \
  '```' \
  '' \
  '## scenario-manifest.json Template'
s2t9_findings="$(template_registry_findings "$s2t9_templates")"
if printf '%s' "$s2t9_findings" | grep -q 'TEMPLATE-SHIPPED-STATE-MISMATCH'; then
  ok "S2-T9 ADVERSARIAL: a fixture template block shipping '- [ ]' FAILS the agreement check"
else
  bad "S2-T9 the agreement check can refuse a wrongly-shipped template" "findings: ${s2t9_findings:-<none>}"
fi

# --- S2-T3: lint passes a FULLY UNCHECKED checklist --------------------------
# SCN-B037-007. D-2's retention proof. A user mid-review is entitled to reject
# every behavior; a lint that demanded one `[x]` would refuse the user's own act.
lint_pkg_unchecked="$WORK/specs/900-all-unchecked"
mkdir -p "$lint_pkg_unchecked"
write_file "$lint_pkg_unchecked/uservalidation.md" \
  '# User Validation Checklist' \
  '' \
  '## Checklist' \
  '' \
  '- [ ] Search returns results for a plain keyword query' \
  '- [ ] An empty query renders the guidance state'
lint_out="$(bash "$ARTIFACT_LINT" "$lint_pkg_unchecked" 2>&1 || true)"
if printf '%s' "$lint_out" | grep -qE 'uservalidation checklist has no checkbox entries|uservalidation checklist contains non-checkbox bullet items|uservalidation acceptance authority is malformed|checked-by-default'; then
  bad "S2-T3 lint accepts a fully unchecked checklist" \
    "$({ printf '%s' "$lint_out" | grep -E 'uservalidation|checked-by-default'; } || true)"
else
  ok "S2-T3 SCN-B037-007: lint accepts a fully unchecked checklist and demands no checked entry"
fi
if printf '%s' "$lint_out" | grep -q 'separates automation readiness from human acceptance'; then
  ok "S2-T3b lint still runs the acceptance-authority shape check"
else
  bad "S2-T3b lint runs the acceptance shape check" "$lint_out"
fi

# --- S2-T4 ADVERSARIAL: lint still rejects a non-checkbox bullet -------------
# SCN-B037-008. Bound on D-2: the ruling was "no checked-entry rule", not "lint
# no longer reads the checklist".
lint_pkg_bullet="$WORK/specs/901-non-checkbox"
mkdir -p "$lint_pkg_bullet"
write_file "$lint_pkg_bullet/uservalidation.md" \
  '# User Validation Checklist' \
  '' \
  '## Checklist' \
  '' \
  '- [x] Search returns results for a plain keyword query' \
  '- everything else also works'
lint_out="$(bash "$ARTIFACT_LINT" "$lint_pkg_bullet" 2>&1 || true)"
if printf '%s' "$lint_out" | grep -q 'uservalidation checklist contains non-checkbox bullet items'; then
  ok "S2-T4 ADVERSARIAL: lint still fails on a non-checkbox checklist bullet"
else
  bad "S2-T4 lint still rejects non-checkbox bullets" "$lint_out"
fi

# --- S2-T5 ADVERSARIAL: lint still rejects a checkbox-less checklist ---------
lint_pkg_empty="$WORK/specs/902-no-checkboxes"
mkdir -p "$lint_pkg_empty"
write_file "$lint_pkg_empty/uservalidation.md" \
  '# User Validation Checklist' \
  '' \
  '## Checklist' \
  '' \
  'The user accepted everything verbally.'
lint_out="$(bash "$ARTIFACT_LINT" "$lint_pkg_empty" 2>&1 || true)"
if printf '%s' "$lint_out" | grep -q 'uservalidation checklist has no checkbox entries'; then
  ok "S2-T5 ADVERSARIAL: lint still fails on a checklist with zero checkbox entries"
else
  bad "S2-T5 lint still rejects a checkbox-less checklist" "$lint_out"
fi

# --- S2-T6: no agents/** file asserts acceptance items ship unchecked --------
agents_dir="$REPO_ROOT/agents"
if [[ -d "$agents_dir" ]]; then
  s2t6_hits="$({ grep -rniE 'ship(s|ped)?[[:space:]]+unchecked' "$agents_dir"; } || true)"
  if [[ -z "$s2t6_hits" ]]; then
    ok "S2-T6 no agents/** surface asserts that acceptance items ship unchecked"
  else
    bad "S2-T6 agents/** agrees with the opt-out contract" "$s2t6_hits"
  fi
else
  bad "S2-T6 agents/ is readable" "not found: $agents_dir"
fi

# --- S2-T7: bug-packet.yaml agrees with the registry -------------------------
if [[ -f "$BUG_PACKET_REGISTRY" ]]; then
  s2t7_purpose="$(awk '
    /^      - id: uservalidation.md$/ {f=1; next}
    f && /^      - id: / {exit}
    f {print}
  ' "$BUG_PACKET_REGISTRY")"
  s2t7_shipped="$(bubbles_acceptance_checklist_shipped_state)"
  if [[ -z "$s2t7_purpose" ]]; then
    bad "S2-T7 bug-packet.yaml declares a uservalidation.md artifact" "no entry found"
  elif printf '%s' "$s2t7_purpose" | grep -qiE 'shipped[[:space:]]+unchecked'; then
    bad "S2-T7 bug-packet.yaml purpose agrees with the registry" \
      "registry shippedState=$s2t7_shipped but bug-packet.yaml still says shipped UNCHECKED"
  elif printf '%s' "$s2t7_purpose" | grep -qiE "shipped[[:space:]]+$s2t7_shipped"; then
    ok "S2-T7 bug-packet.yaml's uservalidation purpose agrees with acceptance-authority.yaml (shipped $s2t7_shipped)"
  else
    bad "S2-T7 bug-packet.yaml purpose names the shipped state" "$s2t7_purpose"
  fi
else
  bad "S2-T7 bug-packet.yaml is readable" "not found: $BUG_PACKET_REGISTRY"
fi

# --- S2-T8 ADVERSARIAL: explicit delivery range changes no foreign acceptance file
# BUG-037 D-1 forbids every foreign `*uservalidation.md` modification, not only
# an unchecked-to-checked transition. The detector therefore compares an
# explicit base and candidate tree and classifies paths only. A prose-only edit
# is enough to fail; content inspection cannot narrow this boundary.
foreign_uservalidation_path_findings() {
  local allowed_root="$1" path
  while IFS= read -r path; do
    [[ -n "$path" ]] || continue
    case "$path" in
      *uservalidation.md)
        case "$path" in
          "$allowed_root"/*) ;;
          *) printf 'FOREIGN-USERVALIDATION-CHANGE: %s\n' "$path" ;;
        esac
        ;;
    esac
  done
}

delivery_range_foreign_uservalidation_findings() {
  local git_dir="$1" base_ref="$2" candidate_ref="$3" allowed_root="$4"
  local changed_paths
  if [[ -z "$base_ref" || -z "$candidate_ref" ]]; then
    printf 'DELIVERY-RANGE-INVALID: base and candidate revisions are required\n'
    return 0
  fi
  if ! git --git-dir "$git_dir" cat-file -e "$base_ref^{tree}" 2> /dev/null; then
    printf 'DELIVERY-RANGE-INVALID: base revision does not resolve\n'
    return 0
  fi
  if ! git --git-dir "$git_dir" cat-file -e "$candidate_ref^{tree}" 2> /dev/null; then
    printf 'DELIVERY-RANGE-INVALID: candidate revision does not resolve\n'
    return 0
  fi
  if ! changed_paths="$(git --git-dir "$git_dir" diff --name-only \
    "$base_ref" "$candidate_ref" -- ':(glob)**/*uservalidation.md')"; then
    printf 'DELIVERY-RANGE-DIFF-FAILED\n'
    return 0
  fi
  foreign_uservalidation_path_findings "$allowed_root" <<< "$changed_paths"
}

snapshot_tree() {
  local git_dir="$1" snapshot="$2" index_file="$3"
  rm -f "$index_file"
  GIT_DIR="$git_dir" GIT_WORK_TREE="$snapshot" GIT_INDEX_FILE="$index_file" \
    git read-tree --empty || return 1
  (
    cd "$snapshot" || exit 1
    GIT_DIR="$git_dir" GIT_WORK_TREE="$snapshot" GIT_INDEX_FILE="$index_file" \
      git add -A .
  ) || return 1
  GIT_DIR="$git_dir" GIT_WORK_TREE="$snapshot" GIT_INDEX_FILE="$index_file" \
    git write-tree
}

s2t8_git="$WORK/s2t8/range.git"
s2t8_base_root="$WORK/s2t8/base"
s2t8_allowed_root="$WORK/s2t8/candidate-allowed"
s2t8_foreign_root="$WORK/s2t8/candidate-foreign"
s2t8_packet_path='bugs/BUG-037-uservalidation-opt-out-acceptance'
s2t8_foreign_path='bugs/BUG-999-foreign/legacy-uservalidation.md'
mkdir -p "$s2t8_base_root/$s2t8_packet_path" \
  "$s2t8_base_root/bugs/BUG-999-foreign" "$s2t8_allowed_root" "$s2t8_foreign_root"
write_file "$s2t8_base_root/$s2t8_packet_path/uservalidation.md" \
  '# BUG-037 acceptance' '- [x] Current contract'
write_file "$s2t8_base_root/$s2t8_foreign_path" \
  '# Foreign acceptance' '- [x] Existing behavior'
cp -R "$s2t8_base_root/." "$s2t8_allowed_root"
write_file "$s2t8_allowed_root/$s2t8_packet_path/uservalidation.md" \
  '# BUG-037 acceptance' '- [x] Current contract' '- [x] New BUG-037 assertion'
cp -R "$s2t8_allowed_root/." "$s2t8_foreign_root"
# Prose-only foreign mutation: no checkbox transition exists for a
# content-sensitive detector to notice.
write_file "$s2t8_foreign_root/$s2t8_foreign_path" \
  '# Foreign acceptance with changed prose' '- [x] Existing behavior'

if git init --bare "$s2t8_git" > /dev/null 2>&1; then
  s2t8_base_tree="$(snapshot_tree "$s2t8_git" "$s2t8_base_root" "$WORK/s2t8/base.index")"
  s2t8_allowed_tree="$(snapshot_tree "$s2t8_git" "$s2t8_allowed_root" "$WORK/s2t8/allowed.index")"
  s2t8_foreign_tree="$(snapshot_tree "$s2t8_git" "$s2t8_foreign_root" "$WORK/s2t8/foreign.index")"

  s2t8_findings="$(delivery_range_foreign_uservalidation_findings \
    "$s2t8_git" "$s2t8_base_tree" "$s2t8_allowed_tree" "$s2t8_packet_path")"
  if [[ -z "$s2t8_findings" ]]; then
    ok "S2-T8 D-1 bound: an explicit base/candidate range permits only BUG-037 uservalidation changes"
  else
    bad "S2-T8 explicit delivery range permits the packet-owned acceptance file" "$s2t8_findings"
  fi

  s2t8_negative="$(delivery_range_foreign_uservalidation_findings \
    "$s2t8_git" "$s2t8_base_tree" "$s2t8_foreign_tree" "$s2t8_packet_path")"
  if [[ "$s2t8_negative" == "FOREIGN-USERVALIDATION-CHANGE: $s2t8_foreign_path" ]]; then
    ok "S2-T8a ADVERSARIAL: a prose-only foreign *uservalidation.md change fails the explicit delivery range"
  else
    bad "S2-T8a every foreign uservalidation modification is detected" \
      "findings: ${s2t8_negative:-<none>}"
  fi
else
  bad "S2-T8 isolated delivery-range repository initializes" "git init --bare failed"
fi

# --- S2-T8b: D-1 shipped no migration script ---------------------------------
# D-1 rejected an operator-run migration script outright: it cannot tell a
# template-authored `[ ]` from a user's deliberate uncheck, so running it would
# perform the act AC-6 forbids. Scoped to the acceptance surface — this repo
# carries unrelated migration scripts (workflow modes, expand/migrate contract)
# that D-1 says nothing about.
s2t8b_hits="$({ find "$SCRIPT_DIR" -type f -name '*migrat*' | grep -iE 'accept|uservalidation|checklist'; } || true)"
if [[ -z "$s2t8b_hits" ]]; then
  ok "S2-T8b D-1: no acceptance/uservalidation migration script was authored"
else
  bad "S2-T8b no migration script exists" "$s2t8b_hits"
fi

# =============================================================================
# SCOPE 3 / SCOPE 4 — governance surfaces agree with what executes
# =============================================================================
# These assert facts about the bubbles SOURCE repository (its CHANGELOG entries,
# its bug packets). A downstream consumer ships the library and this selftest but
# not those artifacts, so the block is scoped by an explicit marker rather than
# by quietly passing when an artifact it needs is absent.

if [[ -d "$SOURCE_REPO_MARKER" ]]; then

  # Extract only one gate's `description` scalar. Sibling metadata such as
  # `name`, `enforcedBy`, or a decoy note must never satisfy a description
  # contract. This handles the folded and literal YAML block forms used by the
  # registry without requiring an optional YAML module.
  gate_description() {
    local gate_id="$1" file="$2"
    awk -v target="  $gate_id:" '
      $0 == target {in_gate=1; next}
      in_gate && /^  G[0-9][0-9][0-9]:/ {exit}
      in_gate && /^    description:[[:space:]]*/ {
        in_description=1
        line=$0
        sub(/^    description:[[:space:]]*/, "", line)
        if (line !~ /^([>|][-+]?)?[[:space:]]*$/) print line
        next
      }
      in_description {
        if ($0 ~ /^    [A-Za-z0-9_-]+:/) exit
        if ($0 ~ /^      /) {
          line=$0
          sub(/^      /, "", line)
          print line
          next
        }
        if ($0 ~ /^[[:space:]]*$/) {print; next}
        exit
      }
    ' "$file"
  }

  write_gate_description_fixture() {
    local path="$1" gate_id="$2" description="$3" decoy="$4"
    write_file "$path" \
      'gates:' \
      "  $gate_id:" \
      '    name: fixture_gate' \
      '    description: >-' \
      "      $description" \
      '    assertionDecoy: >-' \
      "      $decoy" \
      '  G999:' \
      '    name: fixture_boundary' \
      '    description: boundary'
  }

  remove_first_literal() {
    local text="$1" needle="$2"
    [[ "$text" == *"$needle"* ]] || return 1
    printf '%s%s\n' "${text%%"$needle"*}" "${text#*"$needle"}"
  }

  # --- S4-T1: no surface asserts the deleted lint requirement ----------------
  # GC-1 / SCN-B037-013. `artifact-lint.sh` carries no "at least one checked
  # [x]" rule. Any surface still describing one is describing a rule that does
  # not exist — the exact defect class the second half of BUG-037 is about.
  if grep -qiE 'at least (one|ONE) checked' "$ARTIFACT_LINT"; then
    bad "S4-T1 artifact-lint.sh carries no checked-entry requirement" \
      "$(grep -niE 'at least (one|ONE) checked' "$ARTIFACT_LINT")"
  else
    s4t1_hits="$({
      grep -rniE 'requires the checklist to carry at least|at least (one|ONE) checked' \
        "$REPO_ROOT/agents" "$REPO_ROOT/skills" "$REPO_ROOT/bubbles/registry" "$REPO_ROOT/docs" 2> /dev/null
    } || true)"
    if [[ -z "$s4t1_hits" ]]; then
      ok "S4-T1 SCN-B037-013: no governance surface asserts the checked-entry lint rule artifact-lint.sh does not carry"
    else
      bad "S4-T1 no surface describes a deleted lint rule" "$s4t1_hits"
    fi
  fi

  # A second false assertion travelled with the first on every stale surface.
  s4t1b_hits="$({
    grep -rniE 'checked-by-default template is legitimate' \
      "$REPO_ROOT/agents" "$REPO_ROOT/skills" "$REPO_ROOT/bubbles/registry" "$REPO_ROOT/docs" 2> /dev/null
  } || true)"
  if [[ -z "$s4t1b_hits" ]]; then
    ok "S4-T1b no governance surface carries the stale 'checked-by-default template is legitimate' framing"
  else
    bad "S4-T1b stale G136 framing removed" "$s4t1b_hits"
  fi

  # --- S4-T2: G136.description carries the complete executable contract ------
  # GC-2. Parse only the description scalar. All three sections, the done-only
  # scope, opt-out semantics, and the exact closed code set are required.
  g136_description_findings() {
    local file="$1" desc actual_codes missing_codes extra_codes section
    desc="$(gate_description G136 "$file" | tr '\n' ' ')"
    if [[ -z "$desc" ]]; then
      printf 'G136-DESCRIPTION-MISSING\n'
      return 0
    fi

    for section in '## Automation Readiness' '## Checklist' '## Human Acceptance Record'; do
      if ! printf '%s\n' "$desc" | grep -qF "\`$section\`"; then
        printf 'G136-SECTION-MISSING: %s\n' "$section"
      fi
    done

    if ! printf '%s\n' "$desc" |
      grep -qF 'Runs only when the resolved target status is `done`.'; then
      printf 'G136-DONE-SCOPE-MISSING\n'
    fi
    if ! printf '%s\n' "$desc" |
      grep -qF 'Acceptance is opt-out: the checklist ships checked, and unchecking records a user rejection.'; then
      printf 'G136-OPT-OUT-MISSING\n'
    fi

    actual_codes="$({ printf '%s\n' "$desc" | grep -oE 'PD12-[A-Z-]+' | LC_ALL=C sort -u; } || true)"
    missing_codes="$(comm -23 \
      <(printf '%s\n' "$declared_codes" | grep -v '^$') \
      <(printf '%s\n' "$actual_codes" | grep -v '^$'))"
    extra_codes="$(comm -13 \
      <(printf '%s\n' "$declared_codes" | grep -v '^$') \
      <(printf '%s\n' "$actual_codes" | grep -v '^$'))"
    if [[ -n "$missing_codes" || -n "$extra_codes" ]]; then
      printf 'G136-CODE-SET-MISMATCH: missing=%s extra=%s\n' \
        "$(printf '%s' "$missing_codes" | tr '\n' ',')" \
        "$(printf '%s' "$extra_codes" | tr '\n' ',')"
    fi
  }

  if [[ -f "$GATES_REGISTRY" ]]; then
    g136_desc="$(gate_description G136 "$GATES_REGISTRY" | tr '\n' ' ')"
    s4t2_findings="$(g136_description_findings "$GATES_REGISTRY")"
    if [[ -z "$s4t2_findings" ]]; then
      ok "S4-T2 SCN-B037-013: G136.description carries all sections, done-only scope, opt-out semantics, and the exact declared code set"
    else
      bad "S4-T2 G136.description matches the complete contract" "$s4t2_findings"
    fi

    s4t2_control_index=0
    for fixture_spec in \
      'automation-readiness|`## Automation Readiness`|G136-SECTION-MISSING: ## Automation Readiness' \
      'checklist|`## Checklist`|G136-SECTION-MISSING: ## Checklist' \
      'acceptance-record|`## Human Acceptance Record`|G136-SECTION-MISSING: ## Human Acceptance Record' \
      'done-scope|Runs only when the resolved target status is `done`.|G136-DONE-SCOPE-MISSING' \
      'opt-out|Acceptance is opt-out: the checklist ships checked, and unchecking records a user rejection.|G136-OPT-OUT-MISSING' \
      'code-set|`PD12-NO-RECORD`|G136-CODE-SET-MISMATCH:'; do
      fixture_name="${fixture_spec%%|*}"
      fixture_rest="${fixture_spec#*|}"
      fixture_needle="${fixture_rest%%|*}"
      fixture_expected="${fixture_rest#*|}"
      s4t2_control_index=$((s4t2_control_index + 1))
      fixture_file="$WORK/s4t2/$s4t2_control_index-$fixture_name.yaml"
      if fixture_desc="$(remove_first_literal "$g136_desc" "$fixture_needle")"; then
        write_gate_description_fixture "$fixture_file" G136 "$fixture_desc" "$fixture_needle"
        fixture_findings="$(g136_description_findings "$fixture_file")"
        if printf '%s\n' "$fixture_findings" | grep -qF "$fixture_expected"; then
          ok "S4-T2-$fixture_name ADVERSARIAL: removing the description-local clause is detected despite a sibling-key decoy"
        else
          bad "S4-T2-$fixture_name negative control detects the missing clause" \
            "findings: ${fixture_findings:-<none>}"
        fi
      else
        bad "S4-T2-$fixture_name negative-control precondition" \
          "clause not found in G136.description: $fixture_needle"
      fi
    done

    s4t2_extra_code="$WORK/s4t2/extra-code.yaml"
    write_gate_description_fixture "$s4t2_extra_code" G136 \
      "$g136_desc \`PD12-UNDECLARED-FIXTURE\`" 'unrelated sibling metadata'
    s4t2_extra_findings="$(g136_description_findings "$s4t2_extra_code")"
    if printf '%s\n' "$s4t2_extra_findings" | grep -qF 'G136-CODE-SET-MISMATCH:'; then
      ok "S4-T2-code-set-extra ADVERSARIAL: an undeclared description code fails exact-set parity"
    else
      bad "S4-T2 exact code-set check rejects extra codes" \
        "findings: ${s4t2_extra_findings:-<none>}"
    fi

    # --- S4-T8: only G057.description carries the D-3 classification --------
    g057_description_findings() {
      local file="$1" enforcer="$2" desc
      desc="$(gate_description G057 "$file" | tr '\n' ' ')"
      if [[ -z "$desc" ]]; then
        printf 'G057-DESCRIPTION-MISSING\n'
        return 0
      fi
      printf '%s\n' "$desc" |
        grep -qF 'Automation MAY author the INITIAL checked state when the artifact is created' ||
        printf 'G057-RULE-1-MISSING\n'
      printf '%s\n' "$desc" |
        grep -qF 'Automation MUST NOT re-check an item a user unchecked.' ||
        printf 'G057-RULE-2-MISSING\n'
      printf '%s\n' "$desc" |
        grep -qF 'Automation MUST NOT toggle an item either way to mirror a test outcome.' ||
        printf 'G057-RULE-3-MISSING\n'
      printf '%s\n' "$desc" |
        grep -qF 'it IS mechanically checkable through the template-versus-registry agreement case' ||
        printf 'G057-MECHANICAL-CLASSIFICATION-MISSING\n'
      printf '%s\n' "$desc" |
        grep -qF 'Rules 2 and 3 are ADVISORY here' ||
        printf 'G057-ADVISORY-CLASSIFICATION-MISSING\n'
      if [[ ! -f "$enforcer" ]] || grep -q 'uservalidation' "$enforcer"; then
        printf 'G057-ENFORCER-READS-USERVALIDATION\n'
      fi
    }

    g057_desc="$(gate_description G057 "$GATES_REGISTRY" | tr '\n' ' ')"
    s4t8_findings="$(g057_description_findings "$GATES_REGISTRY" "$CONTROL_PLANE_GUARD")"
    if [[ -z "$s4t8_findings" ]]; then
      ok "S4-T8 D-3: G057.description carries all three rules, the mechanical/advisory split, and an enforcer with no uservalidation read"
    else
      bad "S4-T8 G057.description matches the complete D-3 contract" "$s4t8_findings"
    fi

    s4t8_control_index=0
    for fixture_spec in \
      'rule-1|Automation MAY author the INITIAL checked state when the artifact is created|G057-RULE-1-MISSING' \
      'rule-2|Automation MUST NOT re-check an item a user unchecked.|G057-RULE-2-MISSING' \
      'rule-3|Automation MUST NOT toggle an item either way to mirror a test outcome.|G057-RULE-3-MISSING' \
      'mechanical|it IS mechanically checkable through the template-versus-registry agreement case|G057-MECHANICAL-CLASSIFICATION-MISSING' \
      'advisory|Rules 2 and 3 are ADVISORY here|G057-ADVISORY-CLASSIFICATION-MISSING'; do
      fixture_name="${fixture_spec%%|*}"
      fixture_rest="${fixture_spec#*|}"
      fixture_needle="${fixture_rest%%|*}"
      fixture_expected="${fixture_rest#*|}"
      s4t8_control_index=$((s4t8_control_index + 1))
      fixture_file="$WORK/s4t8/$s4t8_control_index-$fixture_name.yaml"
      if fixture_desc="$(remove_first_literal "$g057_desc" "$fixture_needle")"; then
        write_gate_description_fixture "$fixture_file" G057 "$fixture_desc" "$fixture_needle"
        fixture_findings="$(g057_description_findings "$fixture_file" "$CONTROL_PLANE_GUARD")"
        if printf '%s\n' "$fixture_findings" | grep -qF "$fixture_expected"; then
          ok "S4-T8-$fixture_name ADVERSARIAL: removing the description-local clause is detected despite a sibling-key decoy"
        else
          bad "S4-T8-$fixture_name negative control detects the missing clause" \
            "findings: ${fixture_findings:-<none>}"
        fi
      else
        bad "S4-T8-$fixture_name negative-control precondition" \
          "clause not found in G057.description: $fixture_needle"
      fi
    done

    s4t8_enforcer_fixture="$WORK/s4t8/control-plane-checks.sh"
    write_file "$s4t8_enforcer_fixture" \
      '#!/usr/bin/env bash' \
      'cat "$packet/uservalidation.md"'
    s4t8_enforcer_findings="$(g057_description_findings "$GATES_REGISTRY" "$s4t8_enforcer_fixture")"
    if printf '%s\n' "$s4t8_enforcer_findings" |
      grep -qF 'G057-ENFORCER-READS-USERVALIDATION'; then
      ok "S4-T8-enforcer ADVERSARIAL: a uservalidation.md read by the declared enforcer is detected"
    else
      bad "S4-T8 enforcer-read negative control detects the forbidden read" \
        "findings: ${s4t8_enforcer_findings:-<none>}"
    fi
  else
    bad "S4-T2 gates.yaml is readable" "not found: $GATES_REGISTRY"
  fi

  # --- S3-T9: Check 43's refusal text carries no stale record demand --------
  if [[ -f "$TAIL_GUARD" ]]; then
    if grep -q 'a human acceptance record is present' "$TAIL_GUARD"; then
      bad "S3-T9 Check 43 text states the opt-out contract" \
        "$(grep -n 'a human acceptance record is present' "$TAIL_GUARD")"
    else
      ok "S3-T9 Check 43's pass/fail text no longer asserts a required human acceptance record"
    fi
  else
    bad "S3-T9 tail-delegated-gates.sh is readable" "not found: $TAIL_GUARD"
  fi

  # Changelog helpers parse exact named entries. An unrelated mention elsewhere
  # in the file cannot satisfy S4-T4 or S4-T10.
  changelog_section() {
    local file="$1" heading="$2"
    awk -v heading="$heading" '
      $0 == heading {inside=1}
      inside && emitted && /^### / {exit}
      inside {print; emitted=1}
    ' "$file"
  }

  changelog_bug037_entry() {
    changelog_section "$1" '### User Acceptance Is Opt-Out Again (BUG-037)'
  }

  changelog_pd12_entry() {
    changelog_section "$1" '### The Acceptance-Authority Change Had No Changelog Entry (IMP-047 PD-12)'
  }

  changelog_bug029_entry() {
    awk '
      index($0, "(EV-8, BUG-029).**") > 0 {inside=1}
      inside && emitted && /^\*\*/ {exit}
      inside && emitted && /^## / {exit}
      inside {print; emitted=1}
    ' "$1"
  }

  changelog_paragraph() {
    local file="$1" prefix="$2"
    awk -v prefix="$prefix" '
      index($0, prefix) > 0 {inside=1}
      inside && emitted && /^[[:space:]]*$/ {exit}
      inside {print; emitted=1}
    ' "$file"
  }

  migration_class_findings() {
    local file="$1" block

    block="$(changelog_paragraph "$file" 'A current opt-out file' | tr '\n' ' ')"
    [[ "$block" == *"scaffolded from the BUG-037 template"* &&
      "$block" == *"Apply the current contract without"* &&
      "$block" == *"unchecked item as a user rejection"* ]] ||
      printf 'MIGRATION-CLASS-INCOMPLETE: current opt-out\n'

    block="$(changelog_paragraph "$file" 'A legacy pre-PD-12 checked file' | tr '\n' ' ')"
    [[ "$block" == *"inherited checks may be template-authored"* &&
      "$block" == *"do not prove fresh human"* &&
      "$block" == *"Re-author the current checklist before review"* &&
      "$block" == *"Do not cite inherited checks as a human act"* ]] ||
      printf 'MIGRATION-CLASS-INCOMPLETE: legacy pre-PD-12 checked\n'

    block="$(changelog_paragraph "$file" 'A legacy PD-12 unchecked file' | tr '\n' ' ')"
    [[ "$block" == *"inherited unchecks may be template-authored"* &&
      "$block" == *"not automatically user rejections"* &&
      "$block" == *"Re-author the current checklist"* &&
      "$block" == *"only when history proves that no user interaction occurred"* ]] ||
      printf 'MIGRATION-CLASS-INCOMPLETE: legacy PD-12 unchecked\n'
  }

  migration_unknown_findings() {
    local file="$1" block
    block="$(changelog_paragraph "$file" 'A file with unknown provenance' | tr '\n' ' ')"
    [[ "$block" == *"ambiguous checkbox bytes"* &&
      "$block" == *'`in_progress`'* &&
      "$block" == *"Preserve every checkbox byte"* &&
      "$block" == *"artifact owner resolves"* &&
      "$block" == *"with the user"* &&
      "$block" == *"Current bytes and file dates alone never establish"* ]] ||
      printf 'MIGRATION-UNKNOWN-INCOMPLETE\n'
  }

  migration_script_findings() {
    local file="$1" block
    block="$(changelog_paragraph "$file" '**No migration script exists' | tr '\n' ' ')"
    [[ "$block" == *"no bulk migration script will be shipped"* &&
      "$block" == *"template-authored"* &&
      "$block" == *"user's deliberate uncheck"* &&
      "$block" == *"erase the only rejection signal"* ]] ||
      printf 'MIGRATION-SCRIPT-CONTRACT-INCOMPLETE\n'
  }

  changelog_contract_findings() {
    local file="$1" bug037_entry bug037_file pd12_entry pd12_flat
    local bug029_entry bug029_flat
    bug037_entry="$(changelog_bug037_entry "$file")"
    pd12_entry="$(changelog_pd12_entry "$file")"
    bug029_entry="$(changelog_bug029_entry "$file")"

    if [[ -z "$bug037_entry" ]]; then
      printf 'CHANGELOG-BUG037-ENTRY-MISSING\n'
    else
      mkdir -p "$WORK/s4t4"
      bug037_file="$(mktemp "$WORK/s4t4/bug037-entry.XXXXXX")"
      write_file "$bug037_file" "$bug037_entry"
      migration_class_findings "$bug037_file"
      migration_unknown_findings "$bug037_file"
      rm -f "$bug037_file"
    fi

    pd12_flat="$(printf '%s\n' "$pd12_entry" | tr '\n' ' ')"
    if [[ -z "$pd12_entry" ]] ||
      [[ "$pd12_flat" != *"introduced the acceptance authority"* ]] ||
      [[ "$pd12_flat" != *"The deletion stands; the inversion is superseded by BUG-037 above."* ]]; then
      printf 'CHANGELOG-PD12-ENTRY-MISSING\n'
    fi

    bug029_flat="$(printf '%s\n' "$bug029_entry" | tr '\n' ' ')"
    if [[ -z "$bug029_entry" ]] ||
      [[ "$bug029_flat" != *"Superseded in part by BUG-037"* ]] ||
      [[ "$bug029_flat" != *"lint rule was deleted by IMP-047 PD-12 and was not restored"* ]] ||
      [[ "$bug029_flat" != *"see the BUG-037 entry above"* ]]; then
      printf 'CHANGELOG-BUG029-CORRECTION-MISSING\n'
    fi
  }

  # --- S4-T4: named changelog entries carry the complete contract -----------
  # GC-4 / SCN-B037-014. This uses the same section-local migration predicates
  # as S4-T10 and separately requires the corrected BUG-029 entry.
  if [[ -f "$CHANGELOG" ]]; then
    s4t4_findings="$(changelog_contract_findings "$CHANGELOG")"
    if [[ -z "$s4t4_findings" ]]; then
      ok "S4-T4 SCN-B037-014: CHANGELOG.md carries the BUG-037 entry, the PD-12 entry and the D-1 upgrade note"
    else
      bad "S4-T4 section-local changelog contract is complete" "$s4t4_findings"
    fi

    s4t4_bug037_entry="$(changelog_bug037_entry "$CHANGELOG")"
    s4t4_pd12_entry="$(changelog_pd12_entry "$CHANGELOG")"
    s4t4_bug029_entry="$(changelog_bug029_entry "$CHANGELOG")"
    s4t4_complete_fixture="$WORK/s4t4/complete.md"
    mkdir -p "$(dirname "$s4t4_complete_fixture")"
    write_file "$s4t4_complete_fixture" \
      "$s4t4_bug037_entry" '' "$s4t4_pd12_entry" '' "$s4t4_bug029_entry"

    write_file "$WORK/s4t4/missing-bug037.md" \
      "$s4t4_pd12_entry" '' "$s4t4_bug029_entry"
    s4t4_negative="$(changelog_contract_findings "$WORK/s4t4/missing-bug037.md")"
    if printf '%s\n' "$s4t4_negative" | grep -qF 'CHANGELOG-BUG037-ENTRY-MISSING'; then
      ok "S4-T4-bug037 ADVERSARIAL: deleting the named BUG-037 entry is detected"
    else
      bad "S4-T4 BUG-037 entry deletion control fails" "findings: ${s4t4_negative:-<none>}"
    fi

    write_file "$WORK/s4t4/missing-pd12.md" \
      "$s4t4_bug037_entry" '' "$s4t4_bug029_entry"
    s4t4_negative="$(changelog_contract_findings "$WORK/s4t4/missing-pd12.md")"
    if printf '%s\n' "$s4t4_negative" | grep -qF 'CHANGELOG-PD12-ENTRY-MISSING'; then
      ok "S4-T4-pd12 ADVERSARIAL: deleting the named PD-12 entry is detected"
    else
      bad "S4-T4 PD-12 entry deletion control fails" "findings: ${s4t4_negative:-<none>}"
    fi

    write_file "$WORK/s4t4/missing-bug029.md" \
      "$s4t4_bug037_entry" '' "$s4t4_pd12_entry"
    s4t4_negative="$(changelog_contract_findings "$WORK/s4t4/missing-bug029.md")"
    if printf '%s\n' "$s4t4_negative" |
      grep -qF 'CHANGELOG-BUG029-CORRECTION-MISSING'; then
      ok "S4-T4-bug029 ADVERSARIAL: deleting the corrected BUG-029 entry is detected"
    else
      bad "S4-T4 corrected BUG-029 deletion control fails" \
        "findings: ${s4t4_negative:-<none>}"
    fi

    for fixture_spec in \
      'current|/^\*\*UPGRADE NOTE.*A current opt-out file/,/^$/d|MIGRATION-CLASS-INCOMPLETE: current opt-out' \
      'legacy-checked|/^A legacy pre-PD-12 checked file/,/^$/d|MIGRATION-CLASS-INCOMPLETE: legacy pre-PD-12 checked' \
      'legacy-unchecked|/^A legacy PD-12 unchecked file/,/^$/d|MIGRATION-CLASS-INCOMPLETE: legacy PD-12 unchecked' \
      'unknown|/^A file with unknown provenance/,/^$/d|MIGRATION-UNKNOWN-INCOMPLETE'; do
      fixture_name="${fixture_spec%%|*}"
      fixture_rest="${fixture_spec#*|}"
      fixture_expr="${fixture_rest%%|*}"
      fixture_expected="${fixture_rest#*|}"
      fixture_file="$WORK/s4t4/missing-class-$fixture_name.md"
      cp "$s4t4_complete_fixture" "$fixture_file"
      rewrite "$fixture_file" "$fixture_expr"
      fixture_findings="$(changelog_contract_findings "$fixture_file")"
      if printf '%s\n' "$fixture_findings" | grep -qF "$fixture_expected"; then
        ok "S4-T4-$fixture_name ADVERSARIAL: deleting the class-specific handling is detected"
      else
        bad "S4-T4-$fixture_name migration-class deletion control fails" \
          "findings: ${fixture_findings:-<none>}"
      fi
    done
  else
    bad "S4-T4 CHANGELOG.md is readable" "not found: $CHANGELOG"
  fi

  # --- S4-T10: four provenance classes receive distinct safe handling ------
  # SCN-B037-019. Parse only the BUG-037 entry so an unrelated changelog row
  # cannot satisfy the migration contract. Each class is checked in its own
  # paragraph, which keeps a list of keywords from masquerading as guidance.

  s4t10_entry="$WORK/s4t10/bug037-changelog.md"
  mkdir -p "$(dirname "$s4t10_entry")"
  changelog_bug037_entry "$CHANGELOG" > "$s4t10_entry"

  s4t10_class_findings="$(migration_class_findings "$s4t10_entry")"
  if [[ -z "$s4t10_class_findings" ]]; then
    ok "S4-T10 SCN-B037-019: the changelog distinguishes all four migration classes"
  else
    bad "S4-T10 migration classes carry distinct handling" "$s4t10_class_findings"
  fi

  s4t10_unknown_findings="$(migration_unknown_findings "$s4t10_entry")"
  if [[ -z "$s4t10_unknown_findings" ]]; then
    ok "S4-T10a SCN-B037-019: unknown provenance preserves bytes and remains in progress"
  else
    bad "S4-T10a unknown provenance fails closed" "$s4t10_unknown_findings"
  fi

  s4t10_script_findings="$(migration_script_findings "$s4t10_entry")"
  if [[ -z "$s4t10_script_findings" ]]; then
    ok "S4-T10b SCN-B037-019: no bulk acceptance migration script exists"
  else
    bad "S4-T10b no bulk migration mechanism ships" "$s4t10_script_findings"
  fi

  s4t10_control_failures=""
  for fixture_spec in \
    'current|/^\*\*UPGRADE NOTE.*A current opt-out file/,/^$/d' \
    'legacy-checked|/^A legacy pre-PD-12 checked file/,/^$/d' \
    'legacy-unchecked|/^A legacy PD-12 unchecked file/,/^$/d' \
    'unknown|/^A file with unknown provenance/,/^$/d'; do
    fixture_name="${fixture_spec%%|*}"
    fixture_expr="${fixture_spec#*|}"
    fixture_file="$WORK/s4t10/missing-$fixture_name.md"
    cp "$s4t10_entry" "$fixture_file"
    rewrite "$fixture_file" "$fixture_expr"
    fixture_findings="$({
      migration_class_findings "$fixture_file"
      migration_unknown_findings "$fixture_file"
    })"
    if [[ -z "$fixture_findings" ]]; then
      s4t10_control_failures="${s4t10_control_failures}${fixture_name}: deletion was not detected"$'\n'
    fi
  done
  if [[ -z "$s4t10_control_failures" ]]; then
    ok "S4-T10c ADVERSARIAL: deleting any migration-class paragraph is detected"
  else
    bad "S4-T10c migration-class checks can fail" "$s4t10_control_failures"
  fi

  # --- S4-T9 ADVERSARIAL: BUG-032's packet was not touched ------------------
  # D-4 bound. BUG-037 corrects ONE sentence in improvements/INDEX.md and
  # touches nothing under bugs/BUG-032-. Closing BUG-032 as a side effect of
  # removing its only G136 finding is exactly what this refuses.
  if command -v git > /dev/null 2>&1 && git -C "$REPO_ROOT" rev-parse --git-dir > /dev/null 2>&1; then
    s4t9_base_ref="${BUBBLES_ACCEPTANCE_BASE_REF:-HEAD}"
    s4t9_touched="$({
      git -C "$REPO_ROOT" status --porcelain -- 'bugs/BUG-032-*'
      git -C "$REPO_ROOT" diff --name-only "$s4t9_base_ref" -- 'bugs/BUG-032-*'
    } || true)"
    if [[ -z "$s4t9_touched" ]]; then
      ok "S4-T9 D-4 bound: no path under bugs/BUG-032- was modified"
    else
      bad "S4-T9 BUG-032's packet is untouched" "$s4t9_touched"
    fi
  else
    bad "S4-T9 git is available to resolve the base ref" "not a git worktree: $REPO_ROOT"
  fi

fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
