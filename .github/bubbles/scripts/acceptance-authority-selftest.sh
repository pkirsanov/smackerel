#!/usr/bin/env bash
# bubbles/scripts/acceptance-authority-selftest.sh
#
# Capability: human-acceptance-authority
#
# Hermetic selftest for IMP-047 PD-12 — automation readiness is not human
# acceptance.
#
# THE DEFECT THIS PROVES CLOSED
# `uservalidation.md` shipped with its acceptance entries CHECKED, because
# artifact lint required at least one checked entry, and Gate G136 then read
# "every item checked" as terminal human acceptance. Composed, a planning
# template satisfied human sign-off with no human act — and automation could
# satisfy it on a human's behalf simply by writing the template.
#
# Every adversarial case below PASSES if the change is reverted. That is the
# point: a selftest that only exercises the new happy path proves nothing about
# the defect it claims to have closed.
#
# Exit codes:
#   0 = all cases pass
#   1 = at least one case failed

set -uo pipefail

NAME="acceptance-authority-selftest"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/acceptance-authority-lib.sh"
REGISTRY="$SCRIPT_DIR/../registry/acceptance-authority.yaml"
ARTIFACT_LINT="$SCRIPT_DIR/artifact-lint.sh"
TEMPLATES="$SCRIPT_DIR/../../agents/bubbles_shared/feature-templates.md"

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
  printf '%s\n' "$@" >"$path"
}

# --- fixture bodies ----------------------------------------------------------

# The shape as it ships after PD-12: readiness separate, acceptance unchecked,
# record scaffolded but unfilled.
shipped_template() {
  write_file "$1" \
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
    '- acceptedBy: [human name or handle — never an agent id]' \
    '- acceptedAt: [YYYY-MM-DDTHH:MM:SSZ]' \
    '- method: [human-interactive | external-record]'
}

# The pre-PD-12 shape: checked by default, no readiness section, no record.
legacy_checked_template() {
  write_file "$1" \
    '# User Validation Checklist' \
    '' \
    '## Checklist' \
    '' \
    '- [x] Baseline checklist initialized for this feature' \
    '- [x] [Scenario or flow validated]'
}

accepted_file() {
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

# --- 1. the shipped template passes SHAPE lint -------------------------------
# Planning must be able to scaffold the file. If the new shape failed lint,
# authors would restore the checked-by-default form to make lint quiet, which is
# how the defect came back.
f1="$WORK/1/uservalidation.md"
shipped_template "$f1"
out="$(bubbles_acceptance_shape_verdict "$f1" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 && -z "$out" ]]; then
  ok "the shipped template (acceptance unchecked, record unfilled) passes shape lint"
else
  bad "shipped template passes shape lint" "exit $rc: $out"
fi

# --- 2. ADVERSARIAL: the shipped template does NOT grant terminal acceptance --
out="$(bubbles_acceptance_terminal_verdict "$f1" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] &&
  printf '%s' "$out" | grep -q 'PD12-UNCHECKED-ITEM' &&
  printf '%s' "$out" | grep -q 'PD12-NO-RECORD'; then
  ok "the shipped template does not satisfy terminal acceptance"
else
  bad "shipped template refused at terminal" "exit $rc: $out"
fi

# --- 3. ADVERSARIAL: the pre-PD-12 checked-by-default template is refused -----
# This is THE regression case. Before PD-12 this file satisfied Gate G136 with
# no human act anywhere. If the change is reverted, this case passes the gate
# and this assertion fails.
f3="$WORK/3/uservalidation.md"
legacy_checked_template "$f3"
out="$(bubbles_acceptance_terminal_verdict "$f3" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] && printf '%s' "$out" | grep -q 'PD12-NO-RECORD'; then
  ok "a checked-by-default template no longer satisfies terminal human acceptance"
else
  bad "checked-by-default template refused" "exit $rc: $out"
fi

# --- 4. a human-owned record with every box checked is accepted --------------
f4="$WORK/4/uservalidation.md"
accepted_file "$f4"
out="$(bubbles_acceptance_terminal_verdict "$f4" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 && -z "$out" ]]; then
  ok "checked items plus an authored human record satisfy terminal acceptance"
else
  bad "valid human acceptance is accepted" "exit $rc: $out"
fi

# --- 5. ADVERSARIAL: automation cannot be the acceptor -----------------------
f5="$WORK/5/uservalidation.md"
accepted_file "$f5"
sed -e 's/acceptedBy: p.kirsanov/acceptedBy: bubbles.validate/' "$f5" >"$f5.new" && mv "$f5.new" "$f5"
out="$(bubbles_acceptance_terminal_verdict "$f5" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] && printf '%s' "$out" | grep -q 'PD12-AUTOMATION-ACCEPTOR'; then
  ok "an agent id as acceptedBy is refused — automation cannot accept for a human"
else
  bad "automation acceptor refused" "exit $rc: $out"
fi

# --- 6. ADVERSARIAL: a fully checked readiness block grants nothing ----------
# The whole separation is worthless if the section automation OWNS can discharge
# the obligation automation must never discharge.
f6="$WORK/6/uservalidation.md"
write_file "$f6" \
  '# User Validation Checklist' \
  '' \
  '## Automation Readiness' \
  '' \
  '- [x] Every behavior verified by automation' \
  '- [x] Every route exercised' \
  '' \
  '## Checklist' \
  '' \
  '- [ ] Search returns results'
out="$(bubbles_acceptance_terminal_verdict "$f6" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] &&
  printf '%s' "$out" | grep -q 'PD12-UNCHECKED-ITEM' &&
  printf '%s' "$out" | grep -q 'PD12-NO-RECORD'; then
  ok "a fully checked automation-readiness block discharges no acceptance obligation"
else
  bad "readiness grants no acceptance" "exit $rc: $out"
fi

# --- 7. external-record without its pointer is incomplete --------------------
f7="$WORK/7/uservalidation.md"
accepted_file "$f7"
sed -e 's/method: human-interactive/method: external-record/' "$f7" >"$f7.new" && mv "$f7.new" "$f7"
out="$(bubbles_acceptance_terminal_verdict "$f7" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] && printf '%s' "$out" | grep -q 'PD12-METHOD-FIELD-MISSING'; then
  ok "external-record acceptance without its record pointer is refused"
else
  bad "external-record requires its pointer" "exit $rc: $out"
fi

f7b="$WORK/7b/uservalidation.md"
accepted_file "$f7b"
sed -e 's/method: human-interactive/method: external-record/' "$f7b" >"$f7b.new" && mv "$f7b.new" "$f7b"
printf -- '- record: UAT-2026-08-16\n' >>"$f7b"
out="$(bubbles_acceptance_terminal_verdict "$f7b" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]]; then
  ok "external-record acceptance WITH its record pointer is accepted"
else
  bad "external-record with pointer accepted" "exit $rc: $out"
fi

# --- 8. an invented method is refused ----------------------------------------
f8="$WORK/8/uservalidation.md"
accepted_file "$f8"
sed -e 's/method: human-interactive/method: assumed-accepted/' "$f8" >"$f8.new" && mv "$f8.new" "$f8"
out="$(bubbles_acceptance_terminal_verdict "$f8" 2>&1)"
rc=$?
if [[ "$rc" -ne 0 ]] && printf '%s' "$out" | grep -q 'PD12-METHOD-UNKNOWN'; then
  ok "an acceptance method outside the closed vocabulary is refused"
else
  bad "unknown method refused" "exit $rc: $out"
fi

# --- 9. the section parser does not over-reach -------------------------------
# Preserved from the pre-PD-12 guarantee: a `[ ]` outside the acceptance
# checklist is an unrelated bullet, not a withheld acceptance.
f9="$WORK/9/uservalidation.md"
accepted_file "$f9"
printf '\n## Notes\n\n- [ ] follow up on copy\n' >>"$f9"
out="$(bubbles_acceptance_terminal_verdict "$f9" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]]; then
  ok "an unchecked bullet outside the acceptance checklist is ignored"
else
  bad "section parser does not over-reach" "exit $rc: $out"
fi

# --- 10. the library READS the registry, it does not restate it --------------
# Rename the acceptance-record heading in a COPY. The library must follow it. If
# this fails, the shape has been duplicated in the script and the two copies
# will drift, which is the failure the shared library exists to prevent.
alt_reg="$WORK/alt-registry.yaml"
sed -e 's|heading: "## Human Acceptance Record"|heading: "## Signed Off By A Human"|' "$REGISTRY" >"$alt_reg"
f10="$WORK/10/uservalidation.md"
accepted_file "$f10"
sed -e 's|## Human Acceptance Record|## Signed Off By A Human|' "$f10" >"$f10.new" && mv "$f10.new" "$f10"
out="$(BUBBLES_ACCEPTANCE_REGISTRY="$alt_reg" bash -c '
  . "$1"; bubbles_acceptance_terminal_verdict "$2"' _ "$LIB" "$f10" 2>&1)"
rc=$?
out_default="$(bubbles_acceptance_terminal_verdict "$f10" 2>&1 || true)"
if [[ "$rc" -eq 0 ]] && printf '%s' "$out_default" | grep -q 'PD12-NO-RECORD'; then
  ok "renaming the heading in the registry changes the reader (single source, not a copy)"
else
  bad "library reads the registry" "alt exit $rc: $out | default: $out_default"
fi

# --- 11. artifact-lint no longer demands a checked-by-default entry ----------
# End-to-end against the REAL lint, not a reimplementation of it. The removed
# rule is the one that forced the template to ship checked.
spec="$WORK/specs/900-acceptance"
mkdir -p "$spec"
shipped_template "$spec/uservalidation.md"
lint_out="$(bash "$ARTIFACT_LINT" "$spec" 2>&1 || true)"
if ! printf '%s' "$lint_out" | grep -q 'checked-by-default'; then
  ok "artifact-lint no longer requires a checked-by-default acceptance entry"
else
  bad "checked-by-default requirement removed" "$(printf '%s' "$lint_out" | grep 'checked-by-default')"
fi
if printf '%s' "$lint_out" | grep -q 'separates automation readiness from human acceptance'; then
  ok "artifact-lint reports the acceptance-authority shape check"
else
  bad "artifact-lint runs the acceptance shape check" "$lint_out"
fi

# --- 12. the shipped TEMPLATE in feature-templates.md is unchecked -----------
# The template is the artifact the defect actually travelled in. Checking the
# library while the template still ships `[x]` would close nothing.
if [[ -f "$TEMPLATES" ]]; then
  tmpl_checklist="$(awk '
    /^## uservalidation.md Template/ {t=1; next}
    t && /^## Checklist/ {c=1; next}
    c && /^## / {exit}
    c {print}
  ' "$TEMPLATES")"
  if [[ -n "$tmpl_checklist" ]] && ! printf '%s' "$tmpl_checklist" | grep -q '^- \[x\] '; then
    ok "the uservalidation template ships its acceptance entries UNCHECKED"
  else
    bad "template ships unchecked acceptance entries" "$tmpl_checklist"
  fi
else
  bad "feature-templates.md is readable" "not found: $TEMPLATES"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
