#!/usr/bin/env bash
# bubbles/scripts/acceptance-authority-lib.sh
#
# Capability: human-acceptance-authority
#
# The shared reader for `uservalidation.md` (IMP-047 PD-12; contract inverted to
# opt-out acceptance by BUG-037).
#
# WHY THIS EXISTS
# Two surfaces read the acceptance file: `artifact-lint.sh` checks its SHAPE at
# any time, and `guards/tail-delegated-gates.sh` Check 43 (Gate G136) decides
# whether a TERMINAL transition may proceed. Before PD-12 each carried its own
# copy of the section parser, with a comment in one asking the next author to
# keep them in step. A comment is not a mechanism. This library is, and both now
# source it.
#
# It also draws the line the old shape could not draw. AUTOMATION READINESS and
# HUMAN ACCEPTANCE are different facts with different writers, so they live in
# different sections and a fully checked readiness block grants nothing.
#
# WHAT THE TERMINAL VERDICT MEANS AFTER BUG-037. Acceptance is OPT-OUT: the
# checklist ships CHECKED, and a user who objects to nothing performs no act.
# So the terminal verdict proves exactly one thing — that no user recorded an
# objection. It is a REJECTION CHANNEL, not a proof that a human acted. An
# unchecked item still refuses and is still named, the library still NEVER edits
# the file, and an authored record is still validated in full.
#
# Authority: bubbles/registry/acceptance-authority.yaml. Every heading, field,
# method, shipped state, terminal requirement and refusal code below is READ
# from it. Nothing is restated here, because a second copy is a second answer.
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
# PyYAML establishes syntactic validity. The fixed-shape readers below use awk
# for contract fields after that parse gate succeeds.

bubbles_acceptance_heading() {
  bubbles_acceptance_section_field "$1" heading
}

# Any scalar field of one `sections:` entry, by section id. `heading`,
# `shippedState` and `requiredAtTerminal` are all read through here so a caller
# can never acquire a private copy of a value this registry owns.
bubbles_acceptance_section_field() {
  local reg
  reg="$(bubbles_acceptance_registry)"
  [[ -f "$reg" ]] || return 1
  awk -v want="$1" -v key="$2" '
    /^sections:/ {s=1; next}
    /^[a-zA-Z]/ {s=0}
    s && $0 ~ "^  - id: " want "$" {f=1; next}
    f && /^  - id: / {exit}
    f && $0 ~ "^    " key ": " {
      sub("^    " key ": ", "")
      gsub(/^"|"$/, "")
      print
      exit
    }
  ' "$reg"
}

# Preserve the raw scalar for contract fields whose YAML spelling matters.
# In particular, requiredAtTerminal accepts only unquoted lowercase booleans.
bubbles_acceptance_section_field_raw() {
  local reg
  reg="$(bubbles_acceptance_registry)"
  [[ -f "$reg" ]] || return 1
  awk -v want="$1" -v key="$2" '
    /^sections:/ {s=1; next}
    /^[a-zA-Z]/ {s=0}
    s && $0 ~ "^  - id: " want "$" {f=1; next}
    f && /^  - id: / {exit}
    f && $0 ~ "^    " key ":" {
      sub("^    " key ":[ ]*", "")
      print
      exit
    }
  ' "$reg"
}

# The state a freshly authored checklist ships in. BUG-037: `checked`. This is
# the CONTRACT the template must agree with, and the only mechanical detector of
# a template authored in the wrong state now that `artifact-lint.sh` deliberately
# carries no checked-entry rule (BUG-037 D-2).
bubbles_acceptance_checklist_shipped_state() {
  bubbles_acceptance_section_field acceptance-checklist shippedState
}

# Whether a terminal transition additionally demands an authored acceptance
# record. BUG-037 ships `false`: unchecking nothing IS the acceptance, so a
# satisfied user performs no act. READ, never assumed — a downstream repository
# that genuinely wants a named acceptor at terminal flips this key in its own
# registry rather than editing this library.
bubbles_acceptance_record_required_at_terminal() {
  local value
  value="$(bubbles_acceptance_section_field acceptance-record requiredAtTerminal)"
  [[ "$value" == "true" ]]
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

bubbles_acceptance_method_required_fields() {
  local reg
  reg="$(bubbles_acceptance_registry)"
  [[ -f "$reg" ]] || return 1
  awk '
    /^acceptanceRecord:/ {a=1; next}
    /^[a-zA-Z]/ {a=0}
    a && /^  methods:/ {m=1; next}
    a && /^  [a-zA-Z]/ {m=0}
    m && /^      requiresField: / {sub(/^      requiresField: /, ""); print}
  ' "$reg"
}

bubbles_acceptance_failure_codes() {
  local reg
  reg="$(bubbles_acceptance_registry)"
  [[ -f "$reg" ]] || return 1
  awk '
    /^failureCodes:/ {f=1; next}
    f && /^[a-zA-Z]/ {exit}
    f && /^  - id: / {sub(/^  - id: /, ""); print}
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

bubbles_acceptance_yaml_syntax_valid() {
  local reg="$1" python_env="$BUBBLES_ACCEPTANCE_LIB_DIR/python-env.sh" python
  [[ -f "$python_env" ]] || return 1
  # shellcheck source=python-env.sh
  source "$python_env"
  python="$(bubbles_python_resolve)" || return 1
  "$python" -c '
import pathlib
import sys

import yaml

yaml.safe_load(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
' "$reg" >/dev/null 2>&1
}

# Validate the authority before either public verdict reads an artifact. The
# failure code is bootstrap-active, so this path never depends on parsing the
# unavailable registry and never includes its path or contents in diagnostics.
bubbles_acceptance_authority_preflight() {
  local reg section heading shipped required fields methods requires pattern
  local declared_codes expected_codes failure_id_count failure_meaning_count

  reg="$(bubbles_acceptance_registry)"
  if [[ -z "$reg" || ! -f "$reg" || ! -r "$reg" ]]; then
    printf 'PD12-AUTHORITY-UNAVAILABLE: acceptance authority is missing or unreadable\n'
    return 1
  fi

  if ! bubbles_acceptance_yaml_syntax_valid "$reg"; then
    printf 'PD12-AUTHORITY-UNAVAILABLE: acceptance authority schema is malformed\n'
    return 1
  fi

  if [[ "$({ grep -c '^schemaVersion: acceptance-authority/v1$' "$reg" 2>/dev/null; } || true)" -ne 1 ]]; then
    printf 'PD12-AUTHORITY-UNAVAILABLE: acceptance authority schema is malformed\n'
    return 1
  fi

  for section in automation-readiness acceptance-checklist acceptance-record; do
    if [[ "$({ grep -c "^  - id: $section$" "$reg" 2>/dev/null; } || true)" -ne 1 ]]; then
      printf 'PD12-AUTHORITY-UNAVAILABLE: acceptance section contract is incomplete\n'
      return 1
    fi
    heading="$(bubbles_acceptance_section_field "$section" heading 2>/dev/null)"
    if [[ -z "$heading" ]]; then
      printf 'PD12-AUTHORITY-UNAVAILABLE: acceptance section contract is incomplete\n'
      return 1
    fi
  done

  shipped="$(bubbles_acceptance_section_field_raw acceptance-checklist shippedState 2>/dev/null)"
  case "$shipped" in
    checked | unchecked) ;;
    *)
      printf 'PD12-AUTHORITY-UNAVAILABLE: checklist shipped state is invalid\n'
      return 1
      ;;
  esac

  required="$(bubbles_acceptance_section_field_raw acceptance-record requiredAtTerminal 2>/dev/null)"
  case "$required" in
    true | false) ;;
    *)
      printf 'PD12-AUTHORITY-UNAVAILABLE: terminal record requirement is invalid\n'
      return 1
      ;;
  esac

  fields="$(bubbles_acceptance_required_fields 2>/dev/null)"
  for section in acceptedBy acceptedAt method; do
    if [[ "$({ printf '%s\n' "$fields" | grep -cxF "$section"; } || true)" -ne 1 ]]; then
      printf 'PD12-AUTHORITY-UNAVAILABLE: acceptance record field contract is incomplete\n'
      return 1
    fi
  done

  methods="$(bubbles_acceptance_methods 2>/dev/null)"
  for section in human-interactive external-record; do
    if [[ "$({ printf '%s\n' "$methods" | grep -cxF "$section"; } || true)" -ne 1 ]]; then
      printf 'PD12-AUTHORITY-UNAVAILABLE: acceptance method contract is incomplete\n'
      return 1
    fi
  done
  requires="$(bubbles_acceptance_method_requires_field external-record 2>/dev/null)"
  if [[ "$requires" != "record" || -n "$(bubbles_acceptance_method_requires_field human-interactive 2>/dev/null)" ]]; then
    printf 'PD12-AUTHORITY-UNAVAILABLE: acceptance method contract is incomplete\n'
    return 1
  fi

  pattern="$(bubbles_acceptance_forbidden_acceptor_pattern 2>/dev/null)"
  if [[ -z "$pattern" ]]; then
    printf 'PD12-AUTHORITY-UNAVAILABLE: acceptedBy policy is incomplete\n'
    return 1
  fi

  declared_codes="$(bubbles_acceptance_failure_codes 2>/dev/null | LC_ALL=C sort -u)"
  expected_codes="$(printf '%s\n' \
    'PD12-AUTHORITY-UNAVAILABLE' \
    'PD12-AUTOMATION-ACCEPTOR' \
    'PD12-METHOD-FIELD-MISSING' \
    'PD12-METHOD-UNKNOWN' \
    'PD12-NO-RECORD' \
    'PD12-READINESS-NOT-CHECKBOX' \
    'PD12-RECORD-INCOMPLETE' \
    'PD12-UNCHECKED-ITEM' | LC_ALL=C sort -u)"
  failure_id_count="$({ grep -c '^  - id: PD12-[A-Z-][A-Z-]*$' "$reg" 2>/dev/null; } || true)"
  failure_meaning_count="$({ grep -c '^    meaning: [^[:space:]].*$' "$reg" 2>/dev/null; } || true)"
  if [[ "$declared_codes" != "$expected_codes" ||
    "$failure_id_count" -ne 8 || "$failure_meaning_count" -ne 8 ]]; then
    printf 'PD12-AUTHORITY-UNAVAILABLE: failure-code contract is incomplete\n'
    return 1
  fi

  return 0
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
# purpose — an unrelated bullet is not a withheld acceptance. Under opt-out an
# entry here is a user's deliberate UNCHECK, which is why nothing in this
# library ever writes one back.
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
  local value="$1" inner
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  [[ -n "$value" ]] || return 1
  case "$value" in
    \[*\])
      inner="${value#\[}"
      inner="${inner%\]}"
      if [[ "$inner" != *"["* && "$inner" != *"]"* ]]; then
        return 1
      fi
      ;;
  esac
  return 0
}

bubbles_acceptance_record_authorship_fields() {
  {
    bubbles_acceptance_required_fields
    bubbles_acceptance_method_required_fields
  } | awk 'NF && !seen[$0]++'
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
  done <<<"$(bubbles_acceptance_record_authorship_fields)"
  return 1
}

# --- verdicts ----------------------------------------------------------------
#
# Each verdict prints zero or more `CODE: message` lines and returns non-zero
# when it printed any. Callers decide whether a finding is a lint failure or a
# transition refusal; the library never decides that for them, and it NEVER
# edits the file. Checking a box on the author's behalf would erase the only
# signal a user has for rejecting delivered behavior.

# Shape only. Safe during planning, and safe at any point in a review: an absent
# record is NOT a finding here, and neither is an all-unchecked checklist — a
# user is entitled to reject every behavior.
bubbles_acceptance_shape_verdict_after_preflight() {
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

bubbles_acceptance_shape_verdict() {
  local file="$1"

  bubbles_acceptance_authority_preflight || return 1
  bubbles_acceptance_shape_verdict_after_preflight "$file"
}

# Terminal only. Adds the one fact a `done` transition asserts and planning does
# not: no user has recorded an objection. BUG-037 made acceptance OPT-OUT, so an
# authored record is no longer demanded here — unchecking nothing IS the
# acceptance. What remains is the half that costs a satisfied user nothing: an
# unchecked item is a user-reported regression, it refuses the transition, and
# it is NAMED. That is the BUG-029 closure and it is unchanged.
#
# The record demand survives only as registry DATA (`requiredAtTerminal`), which
# this framework ships as `false`. It is read rather than assumed so a
# downstream repository with a compliance obligation can turn it on without
# forking this library.
bubbles_acceptance_terminal_verdict() {
  local file="$1"
  local findings=0 line

  bubbles_acceptance_authority_preflight || return 1

  if ! bubbles_acceptance_shape_verdict_after_preflight "$file"; then
    findings=$((findings + 1))
  fi

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    printf 'PD12-UNCHECKED-ITEM: %s\n' "$line"
    findings=$((findings + 1))
  done <<<"$(bubbles_acceptance_unchecked_items "$file")"

  if bubbles_acceptance_record_required_at_terminal &&
    ! bubbles_acceptance_record_authored "$file"; then
    printf 'PD12-NO-RECORD: no authored "%s"; this registry sets acceptance-record.requiredAtTerminal to true\n' \
      "$(bubbles_acceptance_heading acceptance-record)"
    findings=$((findings + 1))
  fi

  [[ "$findings" -eq 0 ]]
}
