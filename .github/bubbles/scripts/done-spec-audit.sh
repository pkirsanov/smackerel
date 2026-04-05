#!/usr/bin/env bash
set -euo pipefail

# Source fun mode support
source "$(dirname "${BASH_SOURCE[0]}")/fun-mode.sh"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

resolve_repo_root() {
  local candidate
  candidate="$(cd "$SCRIPT_DIR/../.." && pwd)"

  if [[ "$(basename "$candidate")" == ".github" ]]; then
    dirname "$candidate"
    return 0
  fi

  echo "$candidate"
}

usage() {
  echo "Usage: bash bubbles/scripts/done-spec-audit.sh [--fix]"
  echo ""
  echo "Scans specs/*/state.json for status=done, runs state-transition-guard + artifact lint, and reports failures."
  echo "With --fix, failing done specs are downgraded to in_progress using the transition guard's auto-revert path."
}

apply_fix="false"

for arg in "$@"; do
  case "$arg" in
    --fix)
      apply_fix="true"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unsupported argument '$arg'"
      usage
      exit 2
      ;;
  esac
done

repo_root="$(resolve_repo_root)"
cd "$repo_root"

if [[ ! -d "$repo_root/specs" ]]; then
  echo "ERROR: specs directory not found under repo root: $repo_root"
  echo "ERROR: done-spec-audit.sh must run from an installed project repo containing specs/."
  exit 2
fi

lint_script="$SCRIPT_DIR/artifact-lint.sh"
if [[ ! -f "$lint_script" ]]; then
  echo "ERROR: missing lint script: $lint_script"
  exit 2
fi

guard_script="$SCRIPT_DIR/state-transition-guard.sh"
if [[ ! -f "$guard_script" ]]; then
  echo "ERROR: missing guard script: $guard_script"
  exit 2
fi

traceability_script="$SCRIPT_DIR/traceability-guard.sh"

total_done=0
lint_passed=0
lint_failed=0
fixed_count=0
failed_specs=()
total_state_files=0

validate_wi_parity() {
  local spec_dir="$1"
  local state_file="$spec_dir/state.json"

  local canonical_count
  local provisional_count
  local post_migration_target
  local migration_status
  local migration_source
  local trace_matrix

  canonical_count="$({
    grep -Eo '"canonicalCount"[[:space:]]*:[[:space:]]*[0-9]+' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/'
  } || true)"

  provisional_count="$({
    grep -Eo '"provisionalIntakeCount"[[:space:]]*:[[:space:]]*[0-9]+' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/'
  } || true)"

  post_migration_target="$({
    grep -Eo '"postMigrationTargetCount"[[:space:]]*:[[:space:]]*[0-9]+' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/'
  } || true)"

  migration_status="$({
    grep -Eo '"migrationStatus"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*"migrationStatus"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
  } || true)"

  migration_source="$({
    grep -Eo '"migrationSource"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*"migrationSource"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
  } || true)"

  trace_matrix="$({
    grep -Eo '"traceMatrix"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" \
      | head -n 1 \
      | sed -E 's/.*"traceMatrix"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
  } || true)"

  if [[ -z "$canonical_count$provisional_count$post_migration_target$migration_status" ]]; then
    echo "wiParity: SKIPPED (metadata not present)"
    return 0
  fi

  if [[ -z "$canonical_count" ]] || [[ -z "$provisional_count" ]] || [[ -z "$post_migration_target" ]] || [[ -z "$migration_status" ]]; then
    echo "wiParity: FAILED (incomplete metadata)"
    return 1
  fi

  expected_total=$((canonical_count + provisional_count))
  if [[ "$expected_total" -ne "$post_migration_target" ]]; then
    echo "wiParity: FAILED (count mismatch: $canonical_count + $provisional_count != $post_migration_target)"
    return 1
  fi

  case "$migration_status" in
    proposed_not_activated|activated|not_applicable)
      ;;
    *)
      echo "wiParity: FAILED (invalid migrationStatus '$migration_status')"
      return 1
      ;;
  esac

  if [[ "$migration_status" == "activated" ]] && [[ "$provisional_count" -gt 0 ]]; then
    echo "wiParity: FAILED (activated migration requires provisionalIntakeCount=0, found $provisional_count)"
    return 1
  fi

  if [[ -n "$migration_source" ]]; then
    migration_source_file="${migration_source%%#*}"
    if [[ ! -f "$spec_dir/$migration_source_file" ]]; then
      echo "wiParity: FAILED (missing migrationSource file: $migration_source_file)"
      return 1
    fi
  fi

  if [[ -n "$trace_matrix" ]]; then
    if [[ ! -f "$spec_dir/$trace_matrix" ]]; then
      echo "wiParity: FAILED (missing traceMatrix file: $trace_matrix)"
      return 1
    fi
  fi

  echo "wiParity: PASS (canonical=$canonical_count provisional=$provisional_count target=$post_migration_target status=$migration_status)"
  return 0
}

for state_file in specs/*/state.json; do
  [[ -f "$state_file" ]] || continue
  total_state_files=$((total_state_files + 1))

  status_line="$(grep -Eo '"status"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" | head -n 1 || true)"
  status_value="$(echo "$status_line" | sed -E 's/.*"status"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/')"

  if [[ "$status_value" != "done" ]]; then
    continue
  fi

  total_done=$((total_done + 1))
  spec_dir="$(dirname "$state_file")"

  echo ""
  echo "=== Auditing done spec: $spec_dir ==="

  spec_failed="false"

  # Validate wiParity consistency explicitly so done-spec audit reports parity
  # failures even when guard/lint outputs are suppressed.
  echo "--- Checking wiParity consistency ---"
  if validate_wi_parity "$spec_dir"; then
    :
  else
    spec_failed="true"
  fi

  # Run state transition guard first (if available) — catches fabrication patterns
  if [[ -n "$guard_script" ]]; then
    echo "--- Running state transition guard ---"
    if bash "$guard_script" "$spec_dir" > /dev/null 2>&1; then
      echo "Guard: PASS"
    else
      echo "Guard: FAILED (fabrication indicators detected)"
      spec_failed="true"
    fi
  fi

  # Run artifact lint
  echo "--- Running artifact lint ---"
  if bash "$lint_script" "$spec_dir" > /dev/null 2>&1; then
    echo "Lint: PASS"
  else
    echo "Lint: FAILED"
    spec_failed="true"
  fi

  # Run traceability guard (scenario-to-test mapping)
  if [[ -f "$traceability_script" ]]; then
    echo "--- Running traceability guard ---"
    if bash "$traceability_script" "$spec_dir" > /dev/null 2>&1; then
      echo "Traceability: PASS"
    else
      echo "Traceability: FAILED (scenario-to-test mapping gaps detected)"
      spec_failed="true"
    fi
  fi

  if [[ "$spec_failed" == "false" ]]; then
    lint_passed=$((lint_passed + 1))
    continue
  fi

  lint_failed=$((lint_failed + 1))
  failed_specs+=("$spec_dir")

  if [[ "$apply_fix" == "true" ]]; then
    bash "$guard_script" "$spec_dir" --revert-on-fail > /dev/null 2>&1 || true

    now_utc="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

    if grep -Eq '"status"[[:space:]]*:[[:space:]]*"done"' "$state_file"; then
      sed -i -E 's/"status"[[:space:]]*:[[:space:]]*"done"/"status": "in_progress"/' "$state_file"
    fi

    if grep -Eq '"currentPhase"[[:space:]]*:' "$state_file"; then
      sed -i -E 's/"currentPhase"[[:space:]]*:[[:space:]]*"[^"]+"/"currentPhase": "validate"/' "$state_file"
    fi

    if grep -Eq '"notes"[[:space:]]*:' "$state_file"; then
      sed -i -E 's|"notes"[[:space:]]*:[[:space:]]*"[^"]*"|"notes": "Auto-downgraded by done-spec-audit: completion gates failed for a done spec. Restore done only after guard and lint pass."|' "$state_file"
    fi

    if grep -Eq '"lastUpdatedAt"[[:space:]]*:' "$state_file"; then
      sed -i -E 's/"lastUpdatedAt"[[:space:]]*:[[:space:]]*"[^"]+"/"lastUpdatedAt": "'"$now_utc"'"/' "$state_file"
    fi

    fixed_count=$((fixed_count + 1))
    echo "FIXED: downgraded to in_progress -> $spec_dir"
  fi
done

if [[ "$total_state_files" -gt 0 ]] && [[ "$total_done" -eq 0 ]]; then
  echo ""
  echo "ERROR: done-spec audit scanned $total_state_files state files but found zero done specs."
  echo "ERROR: This is treated as a blocking audit failure to prevent silent no-op scans."
  exit 1
fi

echo ""
echo "Done-spec audit summary"
fun_message audit_start
echo "- done specs scanned: $total_done"
echo "- lint passed: $lint_passed"
echo "- lint failed: $lint_failed"
echo "- auto-downgraded (--fix): $fixed_count"

if [[ ${#failed_specs[@]} -gt 0 ]]; then
  echo ""
  echo "Failing specs:"
  for spec in "${failed_specs[@]}"; do
    echo "- $spec"
  done
fi

if [[ "$apply_fix" == "true" ]]; then
  if [[ $lint_failed -gt 0 ]]; then
    fun_message audit_dirty
    exit 1
  fi
  fun_message audit_clean
  exit 0
fi

if [[ $lint_failed -gt 0 ]]; then
  fun_message audit_dirty
  exit 1
fi

fun_message audit_clean

exit 0
