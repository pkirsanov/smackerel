#!/usr/bin/env bash
# bubbles/scripts/acceptance-authority-lib.sh
#
# Capability: human-acceptance-authority
#
# The shared reader for `uservalidation.md` (IMP-047 PD-12).
#
# WHY THIS EXISTS
# Two surfaces read the acceptance file: `artifact-lint.sh` checks its SHAPE at
# any time, and `guards/tail-delegated-gates.sh` Check 43 (Gate G136) decides
# whether a TERMINAL transition may claim human acceptance. Before PD-12 each
# carried its own copy of the section parser, with a comment in one asking the
# next author to keep them in step. A comment is not a mechanism. This library
# is, and both now source it.
#
# It also draws the line the old shape could not draw. AUTOMATION READINESS and
# HUMAN ACCEPTANCE are different facts with different writers, so they live in
# different sections and only one of them can end a spec.
#
# Authority: bubbles/registry/acceptance-authority.yaml. Every heading, field,
# method and refusal code below is READ from it. Nothing is restated here,
# because a second copy is a second answer.
#
# Sourceable:
#   . bubbles/scripts/acceptance-authority-lib.sh
#
# Override the registry for hermetic tests with BUBBLES_ACCEPTANCE_REGISTRY.

# shellcheck shell=bash

BUBBLES_ACCEPTANCE_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

bubbles_acceptance_registry() {
  if [[ -n "${BUBBLES_ACCEPTANCE_REGISTRY:-}" ]]; then
    printf '%s\n' "$BUBBLES_ACCEPTANCE_REGISTRY"
    return 0
  fi
  printf '%s/../registry/acceptance-authority.yaml\n' "$BUBBLES_ACCEPTANCE_LIB_DIR"
}

# --- registry readers --------------------------------------------------------
# Shallow, fixed-shape YAML, so awk is enough and yq stays an optional
# convenience rather than a hard dependency that would make the check
# unavailable exactly where it matters.

bubbles_acceptance_heading() {
  local reg
  reg="$(bubbles_acceptance_registry)"
  [[ -f "$reg" ]] || return 1
  awk -v want="$1" '
    /^sections:/ {s=1; next}
    /^[a-zA-Z]/ {s=0}
    s && $0 ~ "^  - id: " want "$" {f=1; next}
    f && /^  - id: / {exit}
    f && /^    heading: / {
      sub(/^    heading: /, "")
      gsub(/^"|"$/, "")
      print
      exit
    }
  ' "$reg"
}

bubbles_acceptance_required_fields() {
  local reg
  reg="$(bubbles_acceptance_registry)"
  [[ -f "$reg" ]] || return 1
  awk '
    /^acceptanceRecord:/ {a=1; next}
    /^[a-zA-Z]/ {a=0}
    a && /^  requiredFields:/ {f=1; next}
    a && /^  [a-zA-Z]/ {f=0}
    f && /^    - / {sub(/^    - /, ""); print}
  ' "$reg"
}

bubbles_acceptance_methods() {
  local reg
  reg="$(bubbles_acceptance_registry)"
  [[ -f "$reg" ]] || return 1
  awk '
    /^acceptanceRecord:/ {a=1; next}
    /^[a-zA-Z]/ {a=0}
    a && /^  methods:/ {m=1; next}
    a && /^  [a-zA-Z]/ {m=0}
    m && /^    - id: / {sub(/^    - id: /, ""); print}
  ' "$reg"
}

bubbles_acceptance_method_requires_field() {
  local reg
  reg="$(bubbles_acceptance_registry)"
  [[ -f "$reg" ]] || return 1
  awk -v want="$1" '
    /^acceptanceRecord:/ {a=1; next}
    /^[a-zA-Z]/ {a=0}
    a && /^  methods:/ {m=1; next}
    a && /^  [a-zA-Z]/ {m=0}
    m && $0 ~ "^    - id: " want "$" {f=1; next}
    f && /^    - id: / {exit}
    f && /^      requiresField: / {sub(/^      requiresField: /, ""); print; exit}
  ' "$reg"
}

bubbles_acceptance_forbidden_acceptor_pattern() {
  local reg
  reg="$(bubbles_acceptance_registry)"
  [[ -f "$reg" ]] || return 1
  awk '
    /^forbiddenAcceptedBy:/ {a=1; next}
    /^[a-zA-Z]/ {a=0}
    a && /^  pattern:/ {
      sub(/^  pattern:[ ]*/, "")
      gsub(/^'"'"'|'"'"'$/, "")
      print
      exit
    }
  ' "$reg"
}

# --- file readers ------------------------------------------------------------

# Body of one `## ` section, stopping at the next `## `. This is the parser both
# callers used to carry privately; sharing it is what makes a desync impossible
# rather than merely discouraged.
bubbles_acceptance_section_body() {
  local file="$1" heading="$2"
  [[ -f "$file" ]] || return 0
  awk -v h="$heading" '
    index($0, h) == 1 && !seen {seen=1; inside=1; next}
    /^## / {if (inside) exit}
    inside {print}
  ' "$file"
}

# Unchecked acceptance items. Only the acceptance checklist is parsed, so a
# `[ ]` under `## Notes` or under `## Automation Readiness` is ignored on
# purpose — an unrelated bullet is not a withheld acceptance.
bubbles_acceptance_unchecked_items() {
  local file="$1" heading
  heading="$(bubbles_acceptance_heading acceptance-checklist)"
  { bubbles_acceptance_section_body "$file" "$heading" | grep -E '^- \[ \] '; } || true
}

bubbles_acceptance_checklist_items() {
  local file="$1" heading
  heading="$(bubbles_acceptance_heading acceptance-checklist)"
  { bubbles_acceptance_section_body "$file" "$heading" | grep -E '^- \[(x| )\] '; } || true
}

# Automation readiness bullets that are not checkboxes. Automation may CHECK
# these freely; it may not smuggle prose into a block a reader scans as a list
# of verified facts.
bubbles_acceptance_readiness_findings() {
  local file="$1" heading body
  heading="$(bubbles_acceptance_heading automation-readiness)"
  body="$(bubbles_acceptance_section_body "$file" "$heading")"
  [[ -n "$body" ]] || return 0
  { printf '%s\n' "$body" | grep -E '^- ' | grep -Ev '^- \[(x| )\] '; } || true
}

bubbles_acceptance_has_readiness_section() {
  local file="$1" heading
  heading="$(bubbles_acceptance_heading automation-readiness)"
  [[ -n "$(bubbles_acceptance_section_body "$file" "$heading")" ]]
}

bubbles_acceptance_record_field() {
  local file="$1" field="$2" heading body
  heading="$(bubbles_acceptance_heading acceptance-record)"
  body="$(bubbles_acceptance_section_body "$file" "$heading")"
  [[ -n "$body" ]] || return 0
  printf '%s\n' "$body" |
    sed -n -E "s/^[-[:space:]]*${field}:[[:space:]]*(.*[^[:space:]])[[:space:]]*$/\\1/p" |
    sed -E 's/^`//; s/`$//' |
    head -1
}

bubbles_acceptance_has_record_section() {
  local file="$1" heading
  heading="$(bubbles_acceptance_heading acceptance-record)"
  [[ -n "$(bubbles_acceptance_section_body "$file" "$heading")" ]]
}

# A `[placeholder]` is the template's own unfilled slot, not a value. Treating
# it as one is what let a shipped template read as a completed record — the same
# class of defect as a checked-by-default box, one level up.
bubbles_acceptance_value_is_real() {
  local value="$1"
  [[ -n "$value" && "$value" != "["* ]]
}

# TRUE only when a human has begun filling the record in. An untouched template
# stub is deliberately NOT "authored": planning must be able to scaffold the
# section and still pass lint, exactly as it can scaffold an unchecked box.
bubbles_acceptance_record_authored() {
  local file="$1" field value
  bubbles_acceptance_has_record_section "$file" || return 1
  while IFS= read -r field; do
    [[ -n "$field" ]] || continue
    value="$(bubbles_acceptance_record_field "$file" "$field")"
    bubbles_acceptance_value_is_real "$value" && return 0
  done <<<"$(bubbles_acceptance_required_fields)"
  return 1
}

# --- verdicts ----------------------------------------------------------------
#
# Each verdict prints zero or more `CODE: message` lines and returns non-zero
# when it printed any. Callers decide whether a finding is a lint failure or a
# transition refusal; the library never decides that for them, and it NEVER
# edits the file. Checking a box on the author's behalf would fabricate the one
# fact this whole surface exists to require.

# Shape only. Safe during planning, where acceptance has legitimately not
# happened yet: an absent record is NOT a finding here.
bubbles_acceptance_shape_verdict() {
  local file="$1"
  local findings=0 line field value method requires pattern

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    printf 'PD12-READINESS-NOT-CHECKBOX: automation readiness bullet is not a checkbox: %s\n' "$line"
    findings=$((findings + 1))
  done <<<"$(bubbles_acceptance_readiness_findings "$file")"

  if bubbles_acceptance_record_authored "$file"; then
    while IFS= read -r field; do
      [[ -n "$field" ]] || continue
      value="$(bubbles_acceptance_record_field "$file" "$field")"
      if ! bubbles_acceptance_value_is_real "$value"; then
        printf 'PD12-RECORD-INCOMPLETE: human acceptance record has no %s\n' "$field"
        findings=$((findings + 1))
      fi
    done <<<"$(bubbles_acceptance_required_fields)"

    method="$(bubbles_acceptance_record_field "$file" method)"
    if bubbles_acceptance_value_is_real "$method"; then
      if ! bubbles_acceptance_methods | grep -qx -- "$method"; then
        printf 'PD12-METHOD-UNKNOWN: acceptance method "%s" is not in the closed vocabulary (%s)\n' \
          "$method" "$(bubbles_acceptance_methods | tr '\n' ' ')"
        findings=$((findings + 1))
      else
        requires="$(bubbles_acceptance_method_requires_field "$method")"
        if [[ -n "$requires" ]]; then
          value="$(bubbles_acceptance_record_field "$file" "$requires")"
          if ! bubbles_acceptance_value_is_real "$value"; then
            printf 'PD12-METHOD-FIELD-MISSING: method "%s" requires %s, which the record does not carry\n' \
              "$method" "$requires"
            findings=$((findings + 1))
          fi
        fi
      fi
    fi

    pattern="$(bubbles_acceptance_forbidden_acceptor_pattern)"
    value="$(bubbles_acceptance_record_field "$file" acceptedBy)"
    if [[ -n "$pattern" && -n "$value" ]] && printf '%s' "$value" | grep -Eq "$pattern"; then
      printf 'PD12-AUTOMATION-ACCEPTOR: acceptedBy "%s" is an automation identity; an agent cannot accept for a human\n' "$value"
      findings=$((findings + 1))
    fi
  fi

  [[ "$findings" -eq 0 ]]
}

# Terminal only. Adds the two facts a `done` transition asserts and planning
# does not: every acceptance item is checked, AND a human-owned record exists.
# The second half is what PD-12 adds. Without it a shipped template — which used
# to arrive fully checked — satisfied terminal acceptance on its own.
bubbles_acceptance_terminal_verdict() {
  local file="$1"
  local findings=0 line

  if ! bubbles_acceptance_shape_verdict "$file"; then
    findings=$((findings + 1))
  fi

  if [[ -n "$(bubbles_acceptance_checklist_items "$file")" ]]; then
    while IFS= read -r line; do
      [[ -n "$line" ]] || continue
      printf 'PD12-UNCHECKED-ITEM: %s\n' "$line"
      findings=$((findings + 1))
    done <<<"$(bubbles_acceptance_unchecked_items "$file")"
  fi

  if ! bubbles_acceptance_record_authored "$file"; then
    printf 'PD12-NO-RECORD: no authored "%s"; checked boxes alone are not human acceptance, because a template used to ship them checked\n' \
      "$(bubbles_acceptance_heading acceptance-record)"
    findings=$((findings + 1))
  fi

  [[ "$findings" -eq 0 ]]
}
