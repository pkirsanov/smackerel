#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$SCRIPT_DIR/fun-mode.sh"

feature_dir="${1:-}"

if [[ -z "$feature_dir" ]]; then
  echo "ERROR: missing feature directory argument"
  echo "Usage: bash bubbles/scripts/artifact-freshness-guard.sh specs/<NNN-feature-name>"
  exit 2
fi

if [[ ! -d "$feature_dir" ]]; then
  echo "ERROR: feature directory not found: $feature_dir"
  exit 2
fi

failures=0
warnings=0

fail() {
  local message="$1"
  echo "❌ $message"
  fun_fail
  failures=$((failures + 1))
}

warn() {
  local message="$1"
  echo "⚠️  $message"
  fun_warn
  warnings=$((warnings + 1))
}

pass() {
  local message="$1"
  echo "✅ $message"
}

info() {
  local message="$1"
  echo "ℹ️  $message"
}

json_first_string() {
  local key="$1"
  local file="$2"
  if [[ ! -f "$file" ]]; then
    return 0
  fi

  grep -Eo '"'"$key"'"[[:space:]]*:[[:space:]]*"[^"]+"' "$file" \
    | head -n 1 \
    | sed -E 's/.*"'"$key"'"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
}

detect_scope_layout() {
  local state_layout=""
  state_layout="$(json_first_string "scopeLayout" "$feature_dir/state.json" || true)"
  if [[ "$state_layout" == "per-scope-directory" ]] || [[ -f "$feature_dir/scopes/_index.md" ]]; then
    echo "per-scope-directory"
  else
    echo "single-file"
  fi
}

heading_title() {
  local line="$1"
  echo "$line" | sed -E 's/^#{1,6}[[:space:]]+//' | sed -E 's/[[:space:]]+$//'
}

heading_level() {
  local line="$1"
  echo "$line" | sed -E 's/^(#{1,6}).*/\1/' | awk '{ print length($0) }'
}

scope_layout="$(detect_scope_layout)"
scope_files=()
scope_index_file="$feature_dir/scopes/_index.md"

if [[ "$scope_layout" == "per-scope-directory" ]]; then
  while IFS= read -r scope_path; do
    scope_files+=("$scope_path")
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name 'scope.md' | sort)
else
  scope_files+=("$feature_dir/scopes.md")
fi

echo "============================================================"
echo "  BUBBLES ARTIFACT FRESHNESS GUARD"
echo "  Feature: $feature_dir"
echo "  Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "============================================================"
echo ""

echo "--- Check 1: Freshness Boundary Isolation (spec.md / design.md) ---"
freshness_boundary_detected=0
for artifact_path in "$feature_dir/spec.md" "$feature_dir/design.md"; do
  [[ -f "$artifact_path" ]] || continue

  boundary_seen="false"
  boundary_heading=""
  line_num=0

  while IFS= read -r line; do
    line_num=$((line_num + 1))
    if ! echo "$line" | grep -qE '^#{1,6} '; then
      continue
    fi

    title="$(heading_title "$line")"
    if echo "$title" | grep -qiE 'Superseded|Suppressed'; then
      if [[ "$boundary_seen" == "false" ]]; then
        boundary_seen="true"
        boundary_heading="$title"
        freshness_boundary_detected=$((freshness_boundary_detected + 1))
      fi
      continue
    fi

    if [[ "$boundary_seen" == "true" ]] && ! echo "$title" | grep -qiE 'Appendix|Archive|History|Notes|References|Changelog|Change Log'; then
      fail "${artifact_path#$feature_dir/} line $line_num has active-looking heading '$title' after freshness boundary '$boundary_heading'"
    fi
  done < "$artifact_path"

  if [[ "$boundary_seen" == "true" ]]; then
    pass "${artifact_path#$feature_dir/} isolates superseded/suppressed sections at the end"
  else
    info "${artifact_path#$feature_dir/} has no superseded/suppressed sections"
  fi
done

if [[ "$freshness_boundary_detected" -eq 0 ]]; then
  info "No spec/design freshness boundaries detected"
fi
echo ""

echo "--- Check 2: Superseded Scope Sections Are Non-Executable ---"
superseded_scope_sections=0
for scope_path in "${scope_files[@]}"; do
  [[ -f "$scope_path" ]] || continue

  in_superseded_scope="false"
  superseded_level=0
  line_num=0
  scope_had_superseded_section="false"

  while IFS= read -r line; do
    line_num=$((line_num + 1))

    if echo "$line" | grep -qE '^#{1,6} '; then
      title="$(heading_title "$line")"
      level="$(heading_level "$line")"

      if echo "$title" | grep -qiE '(Superseded|Suppressed).*(Scopes|Scope)|(Scopes|Scope).*(Superseded|Suppressed)'; then
        in_superseded_scope="true"
        superseded_level="$level"
        scope_had_superseded_section="true"
        superseded_scope_sections=$((superseded_scope_sections + 1))
        continue
      fi

      if [[ "$in_superseded_scope" == "true" ]] && [[ "$level" -le "$superseded_level" ]]; then
        in_superseded_scope="false"
      fi
    fi

    if [[ "$in_superseded_scope" != "true" ]]; then
      continue
    fi

    if echo "$line" | grep -qE '\*\*Status:\*\*'; then
      fail "${scope_path#$feature_dir/} line $line_num keeps a status marker inside a superseded scope section"
    fi

    if echo "$line" | grep -qE '^#{1,6}[[:space:]]+(Definition of Done|DoD|Test Plan)'; then
      fail "${scope_path#$feature_dir/} line $line_num keeps an executable planning heading inside a superseded scope section"
    fi

    if echo "$line" | grep -qE '^\|[[:space:]]*Test Type[[:space:]]*\|'; then
      fail "${scope_path#$feature_dir/} line $line_num keeps a Test Plan table inside a superseded scope section"
    fi

    if echo "$line" | grep -qE '^\- \[( |x)\] '; then
      fail "${scope_path#$feature_dir/} line $line_num keeps DoD checkbox items inside a superseded scope section"
    fi
  done < "$scope_path"

  if [[ "$scope_had_superseded_section" == "true" ]]; then
    pass "${scope_path#$feature_dir/} keeps superseded scope history non-executable"
  else
    info "${scope_path#$feature_dir/} has no superseded scope section"
  fi
done

if [[ "$superseded_scope_sections" -eq 0 ]]; then
  info "No superseded scope sections detected"
fi
echo ""

echo "--- Check 3: Per-Scope Directory Index References ---"
if [[ "$scope_layout" == "per-scope-directory" ]]; then
  if [[ ! -f "$scope_index_file" ]]; then
    fail "Per-scope layout requires scopes/_index.md for orphan detection"
  else
    indexed_scope_numbers=()
    indexed_scope_dirs=()
    state_scope_dirs=()

    while IFS= read -r index_scope_num; do
      [[ -n "$index_scope_num" ]] || continue
      indexed_scope_numbers+=("$index_scope_num")
    done < <(
      awk '
        /^##[[:space:]]+Dependency Graph/ {in_graph=1; next}
        in_graph && /^##[[:space:]]+/ {exit}
        in_graph && $0 ~ /^\|[[:space:]]*[0-9][0-9][[:space:]]*\|/ {
          line=$0
          sub(/^\|[[:space:]]*/, "", line)
          split(line, parts, "|")
          num=parts[1]
          gsub(/[[:space:]]/, "", num)
          print num
        }
      ' "$scope_index_file"
    )

    while IFS= read -r state_scope_dir; do
      [[ -n "$state_scope_dir" ]] || continue
      state_scope_dirs+=("$state_scope_dir")
    done < <(
      grep -Eo '"scopeDir"[[:space:]]*:[[:space:]]*"scopes/[^"]+"' "$feature_dir/state.json" 2>/dev/null \
        | sed -E 's/.*"(scopes\/[^"]+)"/\1/'
    )

    while IFS= read -r actual_scope_dir; do
      [[ -n "$actual_scope_dir" ]] || continue
      indexed_scope_dirs+=("${actual_scope_dir#$feature_dir/}")
    done < <(find "$feature_dir/scopes" -mindepth 1 -maxdepth 1 -type d | sort)

    orphaned_scope_dirs=0
    for actual_scope_dir in "${indexed_scope_dirs[@]}"; do
      scope_basename="$(basename "$actual_scope_dir")"
      scope_prefix="$(echo "$scope_basename" | sed -E 's/^([0-9][0-9]).*/\1/')"

      if [[ ! "$scope_basename" =~ ^[0-9][0-9]- ]]; then
        warn "$actual_scope_dir does not follow the scopes/NN-name directory convention"
        continue
      fi

      scope_number_referenced="false"
      for indexed_scope_num in "${indexed_scope_numbers[@]}"; do
        if [[ "$indexed_scope_num" == "$scope_prefix" ]]; then
          scope_number_referenced="true"
          break
        fi
      done

      if [[ "$scope_number_referenced" != "true" ]]; then
        fail "$actual_scope_dir exists on disk but its scope number '$scope_prefix' is not referenced from scopes/_index.md Dependency Graph"
        orphaned_scope_dirs=$((orphaned_scope_dirs + 1))
        continue
      fi

      state_scope_registered="false"
      for state_scope_dir in "${state_scope_dirs[@]}"; do
        if [[ "$state_scope_dir" == "$actual_scope_dir" ]]; then
          state_scope_registered="true"
          break
        fi
      done

      if [[ "$state_scope_registered" != "true" ]]; then
        fail "$actual_scope_dir is referenced by scopes/_index.md but missing from state.json scopeProgress.scopeDir"
      fi
    done

    if [[ "$orphaned_scope_dirs" -eq 0 ]]; then
      pass "All per-scope directories are referenced by scopes/_index.md"
    fi
  fi
else
  info "Single-file scope layout detected — orphaned per-scope directory check not applicable"
fi
echo ""

echo "--- Check 4: Result ---"
if [[ "$failures" -gt 0 ]]; then
  echo "RESULT: BLOCKED ($failures failures, $warnings warnings)"
  exit 1
fi

echo "RESULT: PASS (0 failures, $warnings warnings)"
exit 0