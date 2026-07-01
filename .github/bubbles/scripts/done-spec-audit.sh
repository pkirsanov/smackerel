#!/usr/bin/env bash
set -euo pipefail

# Source fun mode support
source "$(dirname "${BASH_SOURCE[0]}")/fun-mode.sh"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Portable in-place sed, kept self-contained (NO cross-file source) because the
# done-spec-audit selftest copies THIS script alone into an isolated fixture repo
# without its sibling libs. GNU `sed -i <prog>` and BSD `sed -i ''` are mutually
# incompatible, so rewrite via a temp file. FILE is the LAST argument; preceding
# args are sed flags/program.
bubbles_sed_inplace() {
  local argc=$# file tmp
  file="${!argc}"
  tmp="$(mktemp)" || return 1
  if sed "${@:1:argc-1}" "$file" >"$tmp" 2>/dev/null; then
    mv "$tmp" "$file"
  else
    rm -f "$tmp"
    return 1
  fi
}

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
  cat <<'EOF'
Usage: bash bubbles/scripts/done-spec-audit.sh [options] [spec-dir ...]

Profiles:
  --profile advisory       Read-only historical report. This is the default.
  --profile changed        Blocking prospective audit for changed spec dirs.
  --profile recertification
                           Deliberate current-policy recertification profile.

Selection:
  --changed                Alias for --profile changed.
  --all                    Select all specs with state.json for the active profile.
  --recertify-all          Alias for --profile recertification --all.
  --base-ref REF --head-ref REF
                           In changed profile, collect changed specs from REF..REF.

Mutation:
  --reopen-failing         Reopen failing done specs to in_progress.
                           Requires --recertify-all or --profile recertification --all.
  --fix                    Deprecated alias for --reopen-failing with the same guard.

Default advisory mode is report-only and does not invalidate historical done
specs. Historical done specs remain grandfathered under their closure epoch until
they are changed, reopened, used as current authority, or explicitly recertified.
EOF
}

profile="advisory"
selection="auto"
explicit_all="false"
reopen_failing="false"
deprecated_fix="false"
base_ref=""
head_ref=""
spec_args=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      profile="${2:?--profile requires advisory, changed, or recertification}"
      shift 2
      ;;
    --profile=*)
      profile="${1#--profile=}"
      shift
      ;;
    --changed|--changed-scope|--changed-specs)
      profile="changed"
      selection="changed"
      shift
      ;;
    --all)
      selection="all"
      explicit_all="true"
      shift
      ;;
    --recertify-all|--recertification-all)
      profile="recertification"
      selection="all"
      explicit_all="true"
      shift
      ;;
    --base-ref)
      base_ref="${2:?--base-ref requires a git ref}"
      shift 2
      ;;
    --base-ref=*)
      base_ref="${1#--base-ref=}"
      shift
      ;;
    --head-ref)
      head_ref="${2:?--head-ref requires a git ref}"
      shift 2
      ;;
    --head-ref=*)
      head_ref="${1#--head-ref=}"
      shift
      ;;
    --reopen-failing)
      reopen_failing="true"
      shift
      ;;
    --fix)
      reopen_failing="true"
      deprecated_fix="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      while [[ $# -gt 0 ]]; do
        spec_args+=("$1")
        shift
      done
      ;;
    --*)
      echo "ERROR: unsupported argument '$1'" >&2
      usage
      exit 2
      ;;
    *)
      spec_args+=("$1")
      shift
      ;;
  esac
done

case "$profile" in
  advisory|changed|recertification)
    ;;
  *)
    echo "ERROR: unsupported profile '$profile'" >&2
    usage
    exit 2
    ;;
esac

if [[ "$selection" == "auto" ]]; then
  if [[ "${#spec_args[@]}" -gt 0 ]]; then
    selection="explicit"
  elif [[ "$profile" == "changed" ]]; then
    selection="changed"
  else
    selection="all"
  fi
fi

if [[ "$deprecated_fix" == "true" ]]; then
  echo "WARNING: --fix is deprecated; use --reopen-failing with --recertify-all for explicit mutation."
fi

if [[ "$reopen_failing" == "true" ]]; then
  if [[ "$profile" != "recertification" || "$explicit_all" != "true" ]]; then
    echo "ERROR: --reopen-failing/--fix requires explicit historical recertification: --recertify-all --reopen-failing" >&2
    exit 2
  fi
fi

repo_root="$(resolve_repo_root)"
cd "$repo_root"

if [[ ! -d "$repo_root/specs" ]]; then
  echo "ERROR: specs directory not found under repo root: $repo_root" >&2
  echo "ERROR: done-spec-audit.sh must run from an installed project repo containing specs/." >&2
  exit 2
fi

lint_script="$SCRIPT_DIR/artifact-lint.sh"
if [[ ! -f "$lint_script" ]]; then
  echo "ERROR: missing lint script: $lint_script" >&2
  exit 2
fi

guard_script="$SCRIPT_DIR/state-transition-guard.sh"
if [[ ! -f "$guard_script" ]]; then
  echo "ERROR: missing guard script: $guard_script" >&2
  exit 2
fi

traceability_script="$SCRIPT_DIR/traceability-guard.sh"

spec_dirs=()
failed_specs=()
reopened_specs=()
total_specs_scanned=0
total_done_scanned=0
artifact_passed=0
artifact_failed=0
completion_passed=0
completion_failed=0

state_status() {
  local state_file="$1"
  local status_line

  status_line="$(grep -Eo '"status"[[:space:]]*:[[:space:]]*"[^"]+"' "$state_file" | head -n 1 || true)"
  printf '%s' "$status_line" | sed -E 's/.*"status"[[:space:]]*:[[:space:]]*"([^"]+)"/\1/'
}

append_unique_spec_dir() {
  local spec_dir="$1"
  local existing

  spec_dir="${spec_dir%/}"
  [[ -f "$spec_dir/state.json" ]] || return 0

  for existing in "${spec_dirs[@]}"; do
    [[ "$existing" == "$spec_dir" ]] && return 0
  done

  spec_dirs+=("$spec_dir")
}

append_spec_dir_for_path() {
  local changed_path="$1"
  local prefix
  local spec_name

  [[ "$changed_path" == specs/* ]] || return 0
  IFS='/' read -r prefix spec_name _ <<< "$changed_path"
  [[ "$prefix" == "specs" && -n "$spec_name" ]] || return 0
  append_unique_spec_dir "specs/$spec_name"
}

collect_paths_from_command() {
  local changed_path

  while IFS= read -r changed_path; do
    [[ -n "$changed_path" ]] || continue
    append_spec_dir_for_path "$changed_path"
  done < <("$@")
}

collect_changed_spec_dirs() {
  if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "Changed-scope discovery skipped: not inside a git worktree."
    return 0
  fi

  if [[ -n "$base_ref" || -n "$head_ref" ]]; then
    if [[ -z "$base_ref" || -z "$head_ref" ]]; then
      echo "ERROR: --base-ref and --head-ref must be provided together" >&2
      exit 2
    fi
    collect_paths_from_command git diff --name-only "$base_ref..$head_ref" -- 'specs/**'
    return 0
  fi

  collect_paths_from_command git diff --name-only --cached -- 'specs/**'
  collect_paths_from_command git diff --name-only -- 'specs/**'

  if git rev-parse --verify HEAD >/dev/null 2>&1; then
    collect_paths_from_command git diff --name-only HEAD -- 'specs/**'
  fi
}

validate_wi_parity() {
  local spec_dir="$1"
  local state_file="$spec_dir/state.json"

  local canonical_count
  local provisional_count
  local post_migration_target
  local migration_status
  local migration_source
  local trace_matrix
  local expected_total
  local migration_source_file

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

run_and_report_check() {
  local label="$1"
  shift
  local output
  local exit_code

  set +e
  output="$({ "$@"; } 2>&1)"
  exit_code=$?
  set -e

  if [[ "$exit_code" -eq 0 ]]; then
    echo "$label: PASS"
    return 0
  fi

  echo "$label: FAILED"
  if [[ -n "$output" ]]; then
    printf '%s\n' "$output"
  fi
  return 1
}

reopen_spec() {
  local spec_dir="$1"
  local state_file="$spec_dir/state.json"
  local now_utc

  bash "$guard_script" "$spec_dir" --revert-on-fail >/dev/null 2>&1 || true

  now_utc="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

  if grep -Eq '"status"[[:space:]]*:[[:space:]]*"done"' "$state_file"; then
    bubbles_sed_inplace -E 's/"status"[[:space:]]*:[[:space:]]*"done"/"status": "in_progress"/' "$state_file"
  fi

  if grep -Eq '"currentPhase"[[:space:]]*:' "$state_file"; then
    bubbles_sed_inplace -E 's/"currentPhase"[[:space:]]*:[[:space:]]*"[^"]+"/"currentPhase": "validate"/' "$state_file"
  fi

  if grep -Eq '"notes"[[:space:]]*:' "$state_file"; then
    bubbles_sed_inplace -E 's|"notes"[[:space:]]*:[[:space:]]*"[^"]*"|"notes": "Reopened by explicit done-spec recertification: current completion gates failed. Restore done only after current guard, lint, and traceability checks pass."|' "$state_file"
  fi

  if grep -Eq '"lastUpdatedAt"[[:space:]]*:' "$state_file"; then
    bubbles_sed_inplace -E 's/"lastUpdatedAt"[[:space:]]*:[[:space:]]*"[^"]+"/"lastUpdatedAt": "'"$now_utc"'"/' "$state_file"
  fi
}

audit_spec() {
  local spec_dir="$1"
  local state_file="$spec_dir/state.json"
  local status_value
  local spec_failed="false"
  local done_spec="false"

  status_value="$(state_status "$state_file")"
  total_specs_scanned=$((total_specs_scanned + 1))

  if [[ "$status_value" == "done" ]]; then
    done_spec="true"
    total_done_scanned=$((total_done_scanned + 1))
  fi

  if [[ "$selection" == "all" && "$done_spec" != "true" ]]; then
    return 0
  fi

  echo ""
  echo "=== Auditing spec: $spec_dir (status=${status_value:-unknown}, profile=$profile) ==="

  echo "--- Running artifact lint ---"
  if run_and_report_check "Lint" bash "$lint_script" "$spec_dir"; then
    artifact_passed=$((artifact_passed + 1))
  else
    artifact_failed=$((artifact_failed + 1))
    spec_failed="true"
  fi

  if [[ "$done_spec" != "true" ]]; then
    echo "Completion gates: SKIPPED (spec is not status=done)"
  else
    echo "--- Checking wiParity consistency ---"
    if validate_wi_parity "$spec_dir"; then
      :
    else
      spec_failed="true"
    fi

    echo "--- Running state transition guard ---"
    if run_and_report_check "Guard" bash "$guard_script" "$spec_dir"; then
      :
    else
      spec_failed="true"
    fi

    if [[ -f "$traceability_script" ]]; then
      echo "--- Running traceability guard ---"
      if run_and_report_check "Traceability" bash "$traceability_script" "$spec_dir"; then
        :
      else
        spec_failed="true"
      fi
    fi
  fi

  if [[ "$spec_failed" == "false" ]]; then
    if [[ "$done_spec" == "true" ]]; then
      completion_passed=$((completion_passed + 1))
    fi
    return 0
  fi

  failed_specs+=("$spec_dir")
  if [[ "$done_spec" == "true" ]]; then
    completion_failed=$((completion_failed + 1))
  fi

  if [[ "$reopen_failing" == "true" && "$done_spec" == "true" ]]; then
    reopen_spec "$spec_dir"
    reopened_specs+=("$spec_dir")
    echo "REOPENED: explicit recertification reopened failing done spec -> $spec_dir"
  fi
}

if [[ "${#spec_args[@]}" -gt 0 ]]; then
  for spec_arg in "${spec_args[@]}"; do
    append_unique_spec_dir "$spec_arg"
  done
elif [[ "$selection" == "changed" ]]; then
  collect_changed_spec_dirs
else
  for state_file in specs/*/state.json; do
    [[ -f "$state_file" ]] || continue
    append_unique_spec_dir "$(dirname "$state_file")"
  done
fi

echo "Done-spec audit"
fun_message audit_start
echo "- profile: $profile"
echo "- selection: $selection"
case "$profile" in
  advisory)
    echo "- posture: advisory/read-only; historical done specs are not automatically invalidated by newer framework policy"
    ;;
  changed)
    echo "- posture: prospective blocking audit for changed/reopened/newly promoted specs"
    ;;
  recertification)
    echo "- posture: deliberate current-policy recertification of historical done specs"
    ;;
esac

if [[ "${#spec_dirs[@]}" -eq 0 ]]; then
  echo "- matched spec directories: 0"
  if [[ "$selection" == "changed" ]]; then
    echo "No changed spec directories were detected; changed-scope audit has nothing to check."
  else
    echo "No spec directories with state.json were detected."
  fi
  fun_message audit_clean
  exit 0
fi

for spec_dir in "${spec_dirs[@]}"; do
  audit_spec "$spec_dir"
done

echo ""
echo "Done-spec audit summary"
echo "- specs scanned: $total_specs_scanned"
echo "- done specs scanned: $total_done_scanned"
echo "- artifact lint passed: $artifact_passed"
echo "- artifact lint failed: $artifact_failed"
echo "- done completion checks passed: $completion_passed"
echo "- done completion checks failed: $completion_failed"
echo "- reopened (--reopen-failing): ${#reopened_specs[@]}"

if [[ "${#failed_specs[@]}" -gt 0 ]]; then
  echo ""
  case "$profile" in
    advisory)
      echo "Historical advisory findings (recertification required before treating these specs as current-policy authority):"
      ;;
    changed)
      echo "Current-policy failures for changed/reopened/newly promoted specs:"
      ;;
    recertification)
      echo "Recertification failures:"
      ;;
  esac
  for spec in "${failed_specs[@]}"; do
    echo "- $spec"
  done
fi

if [[ "${#reopened_specs[@]}" -gt 0 ]]; then
  echo ""
  echo "Explicitly reopened specs:"
  for spec in "${reopened_specs[@]}"; do
    echo "- $spec"
  done
fi

if [[ "${#failed_specs[@]}" -gt 0 ]]; then
  fun_message audit_dirty
  case "$profile" in
    advisory)
      echo "Advisory mode exits 0: older done specs are grandfathered until touched, reopened, used as authority, or explicitly recertified."
      exit 0
      ;;
    changed|recertification)
      exit 1
      ;;
  esac
fi

fun_message audit_clean
exit 0
