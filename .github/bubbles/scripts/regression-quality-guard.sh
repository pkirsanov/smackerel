#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/fun-mode.sh"

detect_repo_root() {
  if [[ "$SCRIPT_DIR" == */.github/bubbles/scripts ]]; then
    (cd "$SCRIPT_DIR/../../.." && pwd)
  else
    (cd "$SCRIPT_DIR/../.." && pwd)
  fi
}

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/regression-quality-guard.sh [--bugfix] [--verbose] <test-file-or-dir> [...]

Scans required regression/E2E tests for:
  - silent-pass bailout patterns
  - optional assertions that do not prove required behavior
  - missing adversarial regression signals in bug-fix mode

Options:
  --bugfix   Require at least one adversarial regression signal
  --verbose  Print matching lines for detected issues
  --help     Show this help message
EOF
}

repo_root="$(detect_repo_root)"
project_config="$repo_root/.github/bubbles-project.yaml"
bugfix_mode="false"
verbose="false"
inputs=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bugfix)
      bugfix_mode="true"
      shift
      ;;
    --verbose|-v)
      verbose="true"
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      inputs+=("$1")
      shift
      ;;
  esac
done

if [[ ${#inputs[@]} -eq 0 ]]; then
  usage
  exit 2
fi

violations=0
warnings=0
scanned_files=0
adversarial_signal_files=0
resolved_files=()

info() {
  echo "ℹ️  $1"
}

pass() {
  echo "✅ $1"
}

warn() {
  echo "⚠️  $1"
  fun_warn
  warnings=$((warnings + 1))
}

violation() {
  local code="$1"
  local file="$2"
  local line_num="$3"
  local detail="$4"
  echo "🔴 VIOLATION [$code] $file:$line_num"
  if [[ "$verbose" == "true" && -n "$detail" ]]; then
    echo "   Match: $detail"
  fi
  fun_fail
  violations=$((violations + 1))
}

append_unique() {
  local value="$1"
  local existing
  for existing in "${resolved_files[@]:-}"; do
    if [[ "$existing" == "$value" ]]; then
      return
    fi
  done
  resolved_files+=("$value")
}

strip_yaml_list_item() {
  sed -E "s/^[[:space:]]*-[[:space:]]*//; s/^[\"'](.*)[\"']$/\1/"
}

load_yaml_list_override() {
  local section="$1"
  local key="$2"
  local target_name="$3"
  local values=()
  local line=""

  if [[ ! -f "$project_config" ]]; then
    return 0
  fi

  while IFS= read -r line; do
    line="$(printf '%s' "$line" | strip_yaml_list_item)"
    [[ -n "$line" ]] || continue
    values+=("$line")
  done < <(sed -n "/scans:/,/^[^ ]/{ /${section}:/,/^    [^ ]/{/${key}:/,/^      [^ ]/{/^        -/p}}}" "$project_config" 2>/dev/null || true)

  if [[ ${#values[@]} -gt 0 ]]; then
    eval "$target_name=()"
    local value
    for value in "${values[@]}"; do
      eval "$target_name+=(\"\$value\")"
    done
    info "Loaded ${#values[@]} override pattern(s) from .github/bubbles-project.yaml scans.${section}.${key}"
  fi
}

is_code_file() {
  case "$1" in
    *.ts|*.tsx|*.js|*.jsx|*.py|*.go|*.rs|*.java|*.dart|*.scala|*.brs)
      return 0
      ;;
  esac
  return 1
}

looks_like_test_path() {
  local path="$1"
  local base
  base="$(basename "$path")"

  case "$path" in
    */test/*|*/tests/*|*/e2e/*|*/spec/*)
      return 0
      ;;
  esac

  case "$base" in
    *test*.*|*spec*.*|test_*|spec_*|*_test.*|*_spec.*)
      return 0
      ;;
  esac

  return 1
}

collect_files_from_input() {
  local input="$1"
  local candidate=""
  if [[ -f "$input" ]]; then
    append_unique "$input"
    return 0
  fi

  if [[ -d "$input" ]]; then
    while IFS= read -r candidate; do
      [[ -n "$candidate" ]] || continue
      if is_code_file "$candidate" && looks_like_test_path "$candidate"; then
        append_unique "$candidate"
      fi
    done < <(find "$input" -type f 2>/dev/null | sort)
    return 0
  fi

  warn "Input does not exist or is not readable: $input"
}

BAILOUT_PATTERNS=(
  "if[[:space:]]*\\([^)]*includes\\([^)]*login[^)]*\\)[^)]*\\).*return[;[:space:]]*$"
  "if[[:space:]]*\\([^)]*!has[A-Za-z0-9_]*[^)]*\\).*return[;[:space:]]*$"
  "if[[:space:]]*\\([^)]*(login|logout|redirect|unauth|unauthor|forbidden|missing)[^)]*\\).*return[;[:space:]]*$"
)

OPTIONAL_ASSERTION_PATTERNS=(
  "if[[:space:]]*\\([^)]*layout[^)]*\\)"
  "toBeDefined\\(\\)"
)

ADVERSARIAL_SIGNAL_PATTERNS=(
  "\\.not\\."
  "\\bfalse\\b"
  "\\bmissing\\b"
  "\\babsent\\b"
  "\\bwithout\\b"
  "\\bempty\\b"
  "\\bedge\\b"
  "\\binvalid\\b"
)

load_yaml_list_override "regressionQuality" "bailoutPatterns" "BAILOUT_PATTERNS"
load_yaml_list_override "regressionQuality" "optionalAssertionPatterns" "OPTIONAL_ASSERTION_PATTERNS"
load_yaml_list_override "regressionQuality" "adversarialSignals" "ADVERSARIAL_SIGNAL_PATTERNS"

for input in "${inputs[@]}"; do
  collect_files_from_input "$input"
done

if [[ ${#resolved_files[@]} -eq 0 ]]; then
  echo "ERROR: no test files resolved from inputs"
  exit 2
fi

echo "============================================================"
echo "  BUBBLES REGRESSION QUALITY GUARD"
echo "  Repo: $repo_root"
echo "  Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "  Bugfix mode: $bugfix_mode"
echo "============================================================"
fun_banner
echo ""

for file in "${resolved_files[@]}"; do
  scanned_files=$((scanned_files + 1))
  info "Scanning $file"

  for pattern in "${BAILOUT_PATTERNS[@]}"; do
    while IFS=: read -r line_num match; do
      [[ -n "$line_num" ]] || continue
      violation "FALSE_NEGATIVE_BAILOUT" "$file" "$line_num" "$match"
    done < <(grep -En "$pattern" "$file" 2>/dev/null || true)
  done

  for pattern in "${OPTIONAL_ASSERTION_PATTERNS[@]}"; do
    while IFS=: read -r line_num match; do
      [[ -n "$line_num" ]] || continue
      violation "OPTIONAL_REQUIRED_ASSERTION" "$file" "$line_num" "$match"
    done < <(grep -En "$pattern" "$file" 2>/dev/null || true)
  done

  if [[ "$bugfix_mode" == "true" ]]; then
    for pattern in "${ADVERSARIAL_SIGNAL_PATTERNS[@]}"; do
      if grep -Eq "$pattern" "$file" 2>/dev/null; then
        adversarial_signal_files=$((adversarial_signal_files + 1))
        pass "Adversarial signal detected in $file"
        break
      fi
    done
  fi
done

if [[ "$bugfix_mode" == "true" && "$adversarial_signal_files" -eq 0 ]]; then
  violation "ADVERSARIAL_REGRESSION_MISSING" "(all scanned files)" "0" "No adversarial regression signal detected. Add a case that would fail if the bug returned or extend scans.regressionQuality.adversarialSignals."
fi

echo ""
echo "============================================================"
echo "  REGRESSION QUALITY RESULT: $violations violation(s), $warnings warning(s)"
echo "  Files scanned: $scanned_files"
if [[ "$bugfix_mode" == "true" ]]; then
  echo "  Files with adversarial signals: $adversarial_signal_files"
fi
echo "============================================================"

if [[ "$violations" -gt 0 ]]; then
  exit 1
fi

exit 0