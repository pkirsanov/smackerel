#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ALLOWLIST_FILE="$REPO_ROOT/bubbles/agnosticity-allowlist.txt"

source "$SCRIPT_DIR/fun-mode.sh"

mode="all"
verbose="false"
quiet="false"
failures=0
scanned=0
declare -a requested_files=()
declare -a candidate_files=()
declare -a target_files=()
declare -a allow_path_patterns=()
declare -a allow_rule_patterns=()
declare -a allow_line_patterns=()
declare -a allow_reasons=()

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/agnosticity-lint.sh [--staged] [--quiet] [--verbose] [files...]

Checks portable Bubbles surfaces for project-specific drift and concrete tooling assumptions.

Modes:
  --staged   Scan only staged files that belong to portable Bubbles surfaces
  --quiet    Suppress non-essential pass/info output
  --verbose  Show scanned file counts and allowlist matches

With no file arguments and no --staged flag, the script scans all portable surfaces.
EOF
}

pass() {
  [[ "$quiet" == "true" ]] && return 0
  echo "✅ $1"
}

info() {
  [[ "$quiet" == "true" ]] && return 0
  echo "ℹ️  $1"
}

warn() {
  echo "⚠️  $1"
  fun_warn
}

violation() {
  local file="$1"
  local line_num="$2"
  local rule_id="$3"
  local line="$4"

  echo "❌ [$rule_id] $file:$line_num"
  echo "   $line"
  fun_fail
  failures=$((failures + 1))
}

normalize_file() {
  local raw="$1"

  if [[ "$raw" == "$REPO_ROOT"/* ]]; then
    printf '%s\n' "${raw#$REPO_ROOT/}"
    return 0
  fi

  if [[ -f "$REPO_ROOT/$raw" ]]; then
    printf '%s\n' "$raw"
    return 0
  fi

  printf '%s\n' "$raw"
}

is_portable_surface() {
  local file="$1"

  case "$file" in
    agents/bubbles*.agent.md|agents/bubbles_shared/*.md|prompts/bubbles*.prompt.md|instructions/bubbles*.instructions.md|README.md|docs/CHEATSHEET.md|docs/recipes/framework-ops.md|docs/recipes/setup-project.md|docs/examples/*.md|bubbles/scripts/*.sh|.github/workflows/*.yml)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

load_allowlist() {
  if [[ ! -f "$ALLOWLIST_FILE" ]]; then
    return 0
  fi

  while IFS=$'\t' read -r path_pattern rule_pattern line_pattern reason; do
    [[ -z "$path_pattern" ]] && continue
    [[ "$path_pattern" =~ ^# ]] && continue

    allow_path_patterns+=("$path_pattern")
    allow_rule_patterns+=("$rule_pattern")
    allow_line_patterns+=("$line_pattern")
    allow_reasons+=("$reason")
  done < "$ALLOWLIST_FILE"
}

is_allowlisted() {
  local file="$1"
  local rule_id="$2"
  local line="$3"
  local idx=0

  while [[ $idx -lt ${#allow_path_patterns[@]} ]]; do
    if [[ "$file" =~ ${allow_path_patterns[$idx]} ]] \
      && [[ "$rule_id" =~ ${allow_rule_patterns[$idx]} ]] \
      && [[ "$line" =~ ${allow_line_patterns[$idx]} ]]; then
      if [[ "$verbose" == "true" ]]; then
        info "Allowlisted [$rule_id] in $file (${allow_reasons[$idx]})"
      fi
      return 0
    fi
    idx=$((idx + 1))
  done

  return 1
}

context_permits_concrete_tool_reference() {
  local file="$1"
  local line_num="$2"
  local start=$((line_num - 2))
  local end=$((line_num + 2))

  (( start < 1 )) && start=1

  local context
  context="$(sed -n "${start},${end}p" "$REPO_ROOT/$file" | tr '[:upper:]' '[:lower:]')"

  if echo "$context" | grep -qE 'must never|never hardcode|do not hardcode|forbidden|prohibited|blocked|negative example|wrong example|guessing project commands|avoid assuming|bad example'; then
    return 0
  fi

  return 1
}

run_rule_on_file() {
  local file="$1"
  local line_num=0
  local line
  local line_lc
  local is_markdown="false"
  local project_name_pattern

  [[ "$file" == *.md ]] && is_markdown="true"

  project_name_pattern="$(printf '%s|' "wander""aide" "guest""host" "quantitative""finance")"
  project_name_pattern="${project_name_pattern%|}"

  while IFS= read -r line || [[ -n "$line" ]]; do
    line_num=$((line_num + 1))
    line_lc="$(printf '%s' "$line" | tr '[:upper:]' '[:lower:]')"

    if [[ "$line_lc" =~ (^|[^[:alnum:]_])(${project_name_pattern})([^[:alnum:]_]|$) ]]; then
      if ! is_allowlisted "$file" "PROJECT_NAME" "$line"; then
        violation "$file" "$line_num" "PROJECT_NAME" "$line"
      fi
    fi

    if [[ "$line" =~ /home/[[:alnum:]_.\/-]+ ]] || [[ "$line" =~ /Users/[[:alnum:]_.\/-]+ ]] || [[ "$line" =~ (^|[^[:alnum:]_])[A-Za-z]:\\[[:alnum:]_.\\/-]+ ]]; then
      if ! is_allowlisted "$file" "ABSOLUTE_PATH" "$line"; then
        violation "$file" "$line_num" "ABSOLUTE_PATH" "$line"
      fi
    fi

    if [[ "$is_markdown" == "true" ]]; then
      if [[ "$line" =~ (Playwright|Cypress|kubectl|docker[[:space:]]compose|cargo[[:space:]]test|go[[:space:]]test|npm[[:space:]]test|npx[[:space:]]playwright) ]] \
        || [[ "$line" =~ curl[[:space:]]--max-time[[:space:]][0-9]+ ]] \
        || [[ "$line" =~ localhost:[0-9]+ ]] \
        || [[ "$line" =~ 127\.0\.0\.1:[0-9]+ ]]; then
        if ! context_permits_concrete_tool_reference "$file" "$line_num" \
          && ! is_allowlisted "$file" "CONCRETE_TOOL" "$line"; then
          violation "$file" "$line_num" "CONCRETE_TOOL" "$line"
        fi
      fi
    fi
  done < "$REPO_ROOT/$file"

  scanned=$((scanned + 1))
}

for arg in "$@"; do
  case "$arg" in
    --staged)
      mode="staged"
      ;;
    --verbose)
      verbose="true"
      ;;
    --quiet)
      quiet="true"
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      requested_files+=("$arg")
      ;;
  esac
done

load_allowlist

if [[ "$mode" == "staged" ]]; then
  while IFS= read -r file; do
    [[ -n "$file" ]] && candidate_files+=("$file")
  done < <(git -C "$REPO_ROOT" diff --cached --name-only --diff-filter=ACMR)
elif [[ ${#requested_files[@]} -gt 0 ]]; then
  candidate_files=("${requested_files[@]}")
else
  while IFS= read -r file; do
    [[ -n "$file" ]] && candidate_files+=("$file")
  done < <(git -C "$REPO_ROOT" ls-files)
fi

for raw_file in "${candidate_files[@]}"; do
  file="$(normalize_file "$raw_file")"
  if is_portable_surface "$file" && [[ -f "$REPO_ROOT/$file" ]]; then
    target_files+=("$file")
  fi
done

if [[ ${#target_files[@]} -eq 0 ]]; then
  pass "No portable Bubbles surfaces to scan"
  exit 0
fi

info "Scanning ${#target_files[@]} portable file(s) for agnosticity drift"
fun_message lint_start

for file in "${target_files[@]}"; do
  run_rule_on_file "$file"
done

# ── Framework manifest integrity check ──────────────────────────────
# If a manifest exists (.github/bubbles/.manifest), check for non-framework
# files in framework-managed directories (scripts, agents, prompts, etc.)
MANIFEST_FILE="$REPO_ROOT/bubbles/.manifest"
if [[ -f "$MANIFEST_FILE" ]] && [[ "$mode" != "staged" ]]; then
  # Check scripts directory for non-manifested files
  for script_file in "$REPO_ROOT"/bubbles/scripts/*.sh; do
    [[ -f "$script_file" ]] || continue
    entry="bubbles/scripts/$(basename "$script_file")"
    if ! grep -qxF "$entry" "$MANIFEST_FILE"; then
      echo "❌ [FRAMEWORK_DRIFT] Non-framework file in managed directory: $entry"
      echo "   Move project-specific scripts to scripts/ or add upstream to Bubbles"
      fun_fail
      failures=$((failures + 1))
    fi
  done
  if [[ "$verbose" == "true" ]]; then
    info "Framework manifest integrity checked"
  fi
fi

if [[ "$verbose" == "true" ]]; then
  info "Scanned files: $scanned"
fi

if [[ "$failures" -eq 0 ]]; then
  pass "Portable Bubbles surfaces are project-agnostic and tool-agnostic"
  fun_message lint_clean
  exit 0
fi

warn "Detected $failures agnosticity violation(s) across portable Bubbles surfaces"
fun_message lint_dirty
exit 1