#!/usr/bin/env bash
set -euo pipefail

# capability-foundation-guard.sh
#
# Gate G094 - capability_foundation_gate.
#
# Enforces capability-first planning for specs created on or after the
# gate introduction date. The gate is proportional: it only fires when
# capability-foundation trigger words appear or when design.md lists two
# or more concrete implementation entries.
#
# Usage:
#   bash bubbles/scripts/capability-foundation-guard.sh <specDir> [--quiet]
#
# Exit codes:
#   0 = clean, not applicable, or grandfathered
#   1 = G094 finding(s)
#   2 = missing/malformed input

INTRODUCTION_DATE="2026-05-25"
QUIET="false"
SPEC_DIR=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/capability-foundation-guard.sh <specDir> [--quiet]

Required:
  <specDir>   Feature or bug spec directory containing state.json.

Optional:
  --quiet     Suppress informational success output.
  -h, --help  Print this usage.

Exit codes:
  0 = clean, not applicable, or grandfathered
  1 = G094 capability_foundation_gate violation
  2 = malformed input
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --*)
      echo "capability-foundation-guard: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -z "$SPEC_DIR" ]]; then
        SPEC_DIR="$1"
      else
        echo "capability-foundation-guard: unexpected positional argument: $1" >&2
        usage >&2
        exit 2
      fi
      shift
      ;;
  esac
done

if [[ -z "$SPEC_DIR" ]]; then
  echo "capability-foundation-guard: <specDir> is required" >&2
  usage >&2
  exit 2
fi

if [[ ! -d "$SPEC_DIR" ]]; then
  echo "capability-foundation-guard: specDir not found or not a directory: $SPEC_DIR" >&2
  exit 2
fi

STATE_FILE="$SPEC_DIR/state.json"
SPEC_FILE="$SPEC_DIR/spec.md"
DESIGN_FILE="$SPEC_DIR/design.md"

if [[ ! -f "$STATE_FILE" ]]; then
  echo "capability-foundation-guard: missing state.json: $STATE_FILE" >&2
  exit 2
fi

info() {
  if [[ "$QUIET" != "true" ]]; then
    echo "capability-foundation-guard: $*"
  fi
}

finding_count=0
finding() {
  echo "G094 capability_foundation_gate violation: $*" >&2
  finding_count=$((finding_count + 1))
}

json_string_field() {
  local field_name="$1"
  local json_file="$2"
  grep -Eo '"'"$field_name"'"[[:space:]]*:[[:space:]]*"[^"]+"' "$json_file" 2>/dev/null \
    | sed -E 's/.*"'"$field_name"'"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/' \
    | sed -n '1p'
}

created_at="$(json_string_field createdAt "$STATE_FILE" || true)"
created_date="${created_at:0:10}"

if [[ -z "$created_date" ]]; then
  info "PASS Gate G094 - state.json.createdAt is missing; treating spec as grandfathered"
  exit 0
fi

if [[ ! "$created_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
  echo "capability-foundation-guard: malformed state.json.createdAt: $created_at" >&2
  exit 2
fi

if [[ "$created_date" < "$INTRODUCTION_DATE" ]]; then
  info "PASS Gate G094 - grandfathered spec createdAt=$created_date introductionDate=$INTRODUCTION_DATE"
  exit 0
fi

TARGET_FILES=()
[[ -f "$SPEC_FILE" ]] && TARGET_FILES+=("$SPEC_FILE")
[[ -f "$DESIGN_FILE" ]] && TARGET_FILES+=("$DESIGN_FILE")
[[ -f "$SPEC_DIR/scopes.md" ]] && TARGET_FILES+=("$SPEC_DIR/scopes.md")
if [[ -d "$SPEC_DIR/scopes" ]]; then
  while IFS= read -r -d '' scope_file; do
    TARGET_FILES+=("$scope_file")
  done < <(find "$SPEC_DIR/scopes" -type f -name 'scope.md' -print0 | sort -z)
fi

if [[ ${#TARGET_FILES[@]} -eq 0 ]]; then
  info "PASS Gate G094 - no planning artifacts found to scan"
  exit 0
fi

trigger_pattern='\b(adapter|provider|strategy|plugin|channel|driver|connector|variant)s?\b'
trigger_hits=0
for target_file in "${TARGET_FILES[@]}"; do
  count="$(grep -Eic "$trigger_pattern" "$target_file" 2>/dev/null || true)"
  trigger_hits=$((trigger_hits + count))
done

section_exists() {
  local file="$1"
  local heading_regex="$2"
  [[ -f "$file" ]] && grep -Eq "$heading_regex" "$file"
}

section_content_lines() {
  local file="$1"
  local heading_regex="$2"
  [[ -f "$file" ]] || return 0
  awk -v heading_regex="$heading_regex" '
    $0 ~ heading_regex { in_section = 1; next }
    in_section && /^#{1,3}[[:space:]]+/ { exit }
    in_section {
      line = $0
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
      if (line != "" && line !~ /^(TBD|TODO|N\/A|n\/a|placeholder)$/) {
        count++
      }
    }
    END { print count + 0 }
  ' "$file"
}

non_empty_section() {
  local file="$1"
  local heading_regex="$2"
  local line_count
  line_count="$(section_content_lines "$file" "$heading_regex")"
  [[ "$line_count" -gt 0 ]]
}

concrete_entries_count() {
  local file="$1"
  [[ -f "$file" ]] || { echo 0; return 0; }
  awk '
    /^##[[:space:]]+Concrete Implementations[[:space:]]*$/ { in_section = 1; next }
    in_section && /^##[[:space:]]+/ { in_section = 0 }
    in_section && /^###[[:space:]]+/ { count++ }
    in_section && /^-[[:space:]]+[^[:space:]]/ { count++ }
    in_section && /^\|/ && $0 !~ /^\|[-:[:space:]|]+\|$/ && $0 !~ /Implementation[[:space:]]*\|/ { count++ }
    END { print count + 0 }
  ' "$file"
}

variation_axes_count() {
  local file="$1"
  [[ -f "$file" ]] || { echo 0; return 0; }
  awk '
    /^###[[:space:]]+Variation Axes[[:space:]]*$/ { in_section = 1; next }
    in_section && /^#{1,3}[[:space:]]+/ { in_section = 0 }
    in_section && /^-[[:space:]]+[^[:space:]]/ { count++ }
    in_section && /^\|/ && $0 !~ /^\|[-:[:space:]|]+\|$/ && $0 !~ /Axis[[:space:]]*\|/ { count++ }
    END { print count + 0 }
  ' "$file"
}

concrete_entries="$(concrete_entries_count "$DESIGN_FILE")"
applicable="false"
if [[ "$trigger_hits" -gt 0 ]] || [[ "$concrete_entries" -ge 2 ]]; then
  applicable="true"
fi

if [[ "$applicable" != "true" ]]; then
  info "PASS Gate G094 - proportionality triggers not present"
  exit 0
fi

info "Gate G094 applies: triggerHits=$trigger_hits concreteImplementationEntries=$concrete_entries"

# spec.md - Analyst ownership
if [[ ! -f "$SPEC_FILE" ]]; then
  finding "spec.md is missing; cannot verify Domain Capability Model or Single-Capability Justification"
elif section_exists "$SPEC_FILE" '^##[[:space:]]+Domain Capability Model[[:space:]]*$'; then
  info "spec.md contains Domain Capability Model"
elif section_exists "$SPEC_FILE" '^###[[:space:]]+Single-Capability Justification[[:space:]]*$'; then
  if non_empty_section "$SPEC_FILE" '^###[[:space:]]+Single-Capability Justification[[:space:]]*$'; then
    info "spec.md contains non-empty Single-Capability Justification"
  else
    finding "spec.md Single-Capability Justification is empty"
  fi
else
  finding "spec.md must contain ## Domain Capability Model or ### Single-Capability Justification when proportionality applies"
fi

# design.md - Design ownership
has_design_split="false"
if [[ ! -f "$DESIGN_FILE" ]]; then
  finding "design.md is missing; cannot verify Capability Foundation"
elif section_exists "$DESIGN_FILE" '^###[[:space:]]+Single-Implementation Justification[[:space:]]*$'; then
  if non_empty_section "$DESIGN_FILE" '^###[[:space:]]+Single-Implementation Justification[[:space:]]*$'; then
    info "design.md contains non-empty Single-Implementation Justification"
  else
    finding "design.md Single-Implementation Justification is empty"
  fi
else
  if ! section_exists "$DESIGN_FILE" '^##[[:space:]]+Capability Foundation[[:space:]]*$'; then
    finding "design.md missing ## Capability Foundation"
  fi
  if ! section_exists "$DESIGN_FILE" '^##[[:space:]]+Concrete Implementations[[:space:]]*$'; then
    finding "design.md missing ## Concrete Implementations"
  fi
  if ! section_exists "$DESIGN_FILE" '^###[[:space:]]+Variation Axes[[:space:]]*$'; then
    finding "design.md missing ### Variation Axes"
  else
    axes="$(variation_axes_count "$DESIGN_FILE")"
    if [[ "$axes" -lt 2 ]]; then
      finding "design.md ### Variation Axes must list at least 2 axes (found $axes)"
    fi
  fi

  if section_exists "$DESIGN_FILE" '^##[[:space:]]+Capability Foundation[[:space:]]*$' \
    && section_exists "$DESIGN_FILE" '^##[[:space:]]+Concrete Implementations[[:space:]]*$' \
    && section_exists "$DESIGN_FILE" '^###[[:space:]]+Variation Axes[[:space:]]*$' \
    && [[ "$(variation_axes_count "$DESIGN_FILE")" -ge 2 ]]; then
    has_design_split="true"
    info "design.md contains capability foundation split with sufficient variation axes"
  fi
fi

# UX ownership: only applies for multi-screen or explicit reusable UI wording.
ui_screen_count=0
if [[ -f "$SPEC_FILE" ]]; then
  ui_screen_count="$(grep -cE '^###[[:space:]]+Screen:' "$SPEC_FILE" 2>/dev/null || true)"
fi
ui_reuse_hits=0
if [[ -f "$SPEC_FILE" ]]; then
  ui_reuse_hits="$(grep -Eic '\b(UI Primitives|shared UI|reusable UI|cross-feature reuse|composition rule|component primitive)\b' "$SPEC_FILE" 2>/dev/null || true)"
fi

if [[ "$ui_screen_count" -ge 2 ]] || [[ "$ui_reuse_hits" -gt 0 ]]; then
  if section_exists "$SPEC_FILE" '^###[[:space:]]+UI Primitives[[:space:]]*$'; then
    info "spec.md contains UI Primitives for multi-screen or reusable UI work"
  elif section_exists "$SPEC_FILE" '^###[[:space:]]+Single-Screen Justification[[:space:]]*$'; then
    if non_empty_section "$SPEC_FILE" '^###[[:space:]]+Single-Screen Justification[[:space:]]*$'; then
      info "spec.md contains non-empty Single-Screen Justification"
    else
      finding "spec.md Single-Screen Justification is empty"
    fi
  else
    finding "spec.md has multi-screen/reusable UI signals but lacks ### UI Primitives or ### Single-Screen Justification"
  fi
else
  info "UX primitive check not applicable: screenCount=$ui_screen_count uiReuseHits=$ui_reuse_hits"
fi

# Planning ownership: only applies when a foundation/overlay split exists.
if [[ "$has_design_split" == "true" ]]; then
  scope_text_files=()
  [[ -f "$SPEC_DIR/scopes.md" ]] && scope_text_files+=("$SPEC_DIR/scopes.md")
  if [[ -d "$SPEC_DIR/scopes" ]]; then
    while IFS= read -r -d '' scope_file; do
      scope_text_files+=("$scope_file")
    done < <(find "$SPEC_DIR/scopes" -type f -name 'scope.md' -print0 | sort -z)
  fi

  if [[ ${#scope_text_files[@]} -eq 0 ]]; then
    finding "capability foundation split exists but no scopes.md or scopes/*/scope.md planning artifact was found"
  else
    foundation_tag_hits=0
    depends_on_foundation_hits=0
    for scope_text_file in "${scope_text_files[@]}"; do
      foundation_tag_hits=$((foundation_tag_hits + $(grep -Eic 'foundation[[:space:]]*:[[:space:]]*true' "$scope_text_file" 2>/dev/null || true)))
      depends_on_foundation_hits=$((depends_on_foundation_hits + $(grep -Eic 'Depends On.*foundation|foundation.*Depends On' "$scope_text_file" 2>/dev/null || true)))
    done

    if [[ "$foundation_tag_hits" -eq 0 ]]; then
      finding "scopes must include a foundation scope tagged foundation:true when design splits foundation from concrete implementations"
    fi
    if [[ "$depends_on_foundation_hits" -eq 0 ]]; then
      finding "overlay/concrete implementation scopes must declare Depends On referencing the foundation scope"
    fi
    if [[ "$foundation_tag_hits" -gt 0 && "$depends_on_foundation_hits" -gt 0 ]]; then
      info "scopes include foundation:true and overlay Depends On foundation ordering"
    fi
  fi
fi

if [[ "$finding_count" -gt 0 ]]; then
  echo "G094 capability_foundation_gate: FAILED with $finding_count finding(s)" >&2
  exit 1
fi

info "PASS Gate G094 - capability foundation requirements satisfied"
exit 0
