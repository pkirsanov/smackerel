#!/usr/bin/env bash
set -euo pipefail

# Source fun mode support
source "$(dirname "${BASH_SOURCE[0]}")/fun-mode.sh"

feature_dir="${1:-}"
write_mode="${2:-}"

if [[ -z "$feature_dir" ]]; then
  echo "ERROR: missing feature directory argument"
  echo "Usage: bash bubbles/scripts/report-section-autofix.sh specs/<NNN-feature-name> [--write]"
  exit 2
fi

if [[ ! -d "$feature_dir" ]]; then
  echo "ERROR: feature directory not found: $feature_dir"
  exit 2
fi

if [[ -n "$write_mode" && "$write_mode" != "--write" ]]; then
  echo "ERROR: unsupported second argument '$write_mode'"
  echo "Use '--write' to apply changes, or omit for dry-run"
  exit 2
fi

state_file="$feature_dir/state.json"
scope_layout="single-file"
if [[ -f "$feature_dir/scopes/_index.md" ]]; then
  scope_layout="per-scope-directory"
fi

report_files=()
if [[ "$scope_layout" == "per-scope-directory" ]]; then
  while IFS= read -r scope_report_path; do
    report_files+=("$scope_report_path")
  done < <(find "$feature_dir/scopes" -mindepth 2 -maxdepth 2 -type f -name 'report.md' | sort)
else
  report_files+=("$feature_dir/report.md")
fi

if [[ ! -f "$state_file" ]]; then
  echo "ERROR: missing state file: $state_file"
  exit 2
fi

if [[ ${#report_files[@]} -eq 0 ]]; then
  echo "ERROR: no report files found for scope layout '$scope_layout'"
  exit 2
fi

state_status="$({
  grep -Eo '"status"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" \
    | head -n 1 \
    | sed -E 's/.*"status"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
} || true)"

state_workflow_mode="$({
  grep -Eo '"workflowMode"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" \
    | head -n 1 \
    | sed -E 's/.*"workflowMode"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
} || true)"

if [[ -z "$state_status" ]]; then
  echo "ERROR: unable to determine state.json status"
  exit 2
fi

required_headers=(
  "### Summary"
  "### Completion Statement"
  "### Test Evidence"
)

should_enforce_mode_gates="false"
case "$state_status" in
  done|validated|docs_updated|specs_hardened)
    should_enforce_mode_gates="true"
    ;;
esac

if [[ "$should_enforce_mode_gates" == "true" && -z "$state_workflow_mode" ]]; then
  echo "ERROR: state.json status '$state_status' requires workflowMode before autofix can determine mode-specific sections"
  exit 2
fi

if [[ "$should_enforce_mode_gates" == "true" && -n "$state_workflow_mode" ]]; then
  case "$state_workflow_mode" in
    full-delivery|value-first-e2e-batch|chaos-hardening|harden-to-doc|gaps-to-doc|harden-gaps-to-doc|reconcile-to-doc|chaos-to-doc|batch-implement|batch-harden|batch-gaps|batch-harden-gaps|batch-improve-existing|batch-reconcile-to-doc|product-to-delivery|improve-existing)
      required_headers+=(
        "### Validation Evidence"
        "### Audit Evidence"
        "### Chaos Evidence"
      )
      ;;
    feature-bootstrap|bugfix-fastlane)
      required_headers+=(
        "### Validation Evidence"
        "### Audit Evidence"
      )
      ;;
    validate-only)
      required_headers+=("### Validation Evidence")
      ;;
    audit-only)
      required_headers+=("### Audit Evidence")
      ;;
  esac
fi

total_missing=0
for report_file in "${report_files[@]}"; do
  if [[ ! -f "$report_file" ]]; then
    echo "ERROR: missing report file: $report_file"
    total_missing=$((total_missing + 1))
    continue
  fi

  missing_headers=()
  for header in "${required_headers[@]}"; do
    if ! grep -Eq "^${header}$" "$report_file"; then
      missing_headers+=("$header")
    fi
  done

  if [[ ${#missing_headers[@]} -eq 0 ]]; then
    echo "No missing required report headers in $report_file"
    continue
  fi

  total_missing=$((total_missing + ${#missing_headers[@]}))
  echo "Missing required headers in $report_file:"
  for header in "${missing_headers[@]}"; do
    echo "- $header"
  done

  if [[ "$write_mode" != "--write" ]]; then
    continue
  fi

  insert_block=""
  for header in "${missing_headers[@]}"; do
    insert_block+=$'\n'
    insert_block+="$header"
    insert_block+=$'\n\n'
    insert_block+="TODO: Add evidence content for this section."
    insert_block+=$'\n'
  done

  tmp_file="$(mktemp)"

  if grep -Eq '^Links:' "$report_file"; then
    awk -v block="$insert_block" '
      {
        print
        if (!inserted && $0 ~ /^Links:/) {
          printf "%s", block
          inserted = 1
        }
      }
    ' "$report_file" > "$tmp_file"
  else
    awk -v block="$insert_block" '
      {
        print
        if (!inserted && $0 ~ /^# /) {
          printf "\n%s", block
          inserted = 1
        }
      }
    ' "$report_file" > "$tmp_file"
  fi

  mv "$tmp_file" "$report_file"
  echo "Scaffolded ${#missing_headers[@]} missing section(s) in $report_file"
done

if [[ "$write_mode" != "--write" && "$total_missing" -gt 0 ]]; then
  echo "Dry-run only. Re-run with --write to scaffold missing sections."
fi