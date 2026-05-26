#!/usr/bin/env bash
set -euo pipefail

# repo-drift-report.sh
#
# Non-blocking repository drift visibility for SCOPE-14. The report composes
# Pass 2 metadata and guard signals into markdown rows. Drift findings exit 0;
# runtime failures exit 2 so framework validation can fail only when the report
# itself cannot run.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT_INPUT=""
NOW_INPUT=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/repo-drift-report.sh [--repo-root <root>] [--now <rfc3339>]

Optional:
  --repo-root <root>  Repository root to inspect. Defaults to BUBBLES_REPO_ROOT
                      or the repository containing this script.
  --now <rfc3339>     Current time override for hermetic age checks.
  -h, --help          Print this usage and exit.

Exit codes:
  0 = report emitted; drift findings are informational
  2 = runtime error, invalid arguments, malformed state, or git inspection error
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --repo-root)
      [[ $# -ge 2 ]] || { echo "repo-drift-report: --repo-root requires a value" >&2; exit 2; }
      REPO_ROOT_INPUT="$2"
      shift 2
      ;;
    --now)
      [[ $# -ge 2 ]] || { echo "repo-drift-report: --now requires an RFC3339 value" >&2; exit 2; }
      NOW_INPUT="$2"
      shift 2
      ;;
    --*)
      echo "repo-drift-report: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      echo "repo-drift-report: unexpected positional argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v jq >/dev/null 2>&1; then
  echo "repo-drift-report: jq is required but not found in PATH" >&2
  exit 2
fi

if ! command -v git >/dev/null 2>&1; then
  echo "repo-drift-report: git is required but not found in PATH" >&2
  exit 2
fi

resolve_repo_root() {
  if [[ -n "$REPO_ROOT_INPUT" ]]; then
    (cd "$REPO_ROOT_INPUT" && pwd -P)
    return 0
  fi

  if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
    (cd "$BUBBLES_REPO_ROOT" && pwd -P)
    return 0
  fi

  if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
    (cd "$SCRIPT_DIR/../../.." && pwd -P)
  else
    (cd "$SCRIPT_DIR/../.." && pwd -P)
  fi
}

REPO_ROOT="$(resolve_repo_root || true)"
if [[ -z "$REPO_ROOT" || ! -d "$REPO_ROOT" ]]; then
  echo "repo-drift-report: unable to resolve repository root" >&2
  exit 2
fi

if ! git -C "$REPO_ROOT" rev-parse --show-toplevel >/dev/null 2>&1; then
  echo "repo-drift-report: repository root is not inside a git worktree: $REPO_ROOT" >&2
  exit 2
fi

rfc3339_epoch() {
  local value="$1"
  jq -nr --arg d "$value" 'try ($d | fromdateiso8601 | floor) catch empty'
}

if [[ -n "$NOW_INPUT" ]]; then
  CURRENT_EPOCH="$(rfc3339_epoch "$NOW_INPUT")"
  if [[ -z "$CURRENT_EPOCH" ]]; then
    echo "repo-drift-report: malformed --now timestamp: $NOW_INPUT" >&2
    exit 2
  fi
  GENERATED_AT="$NOW_INPUT"
else
  CURRENT_EPOCH="$(date -u +%s)"
  GENERATED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
fi

relative_path() {
  local path="$1"
  if [[ "$path" == "$REPO_ROOT" ]]; then
    printf '.'
  elif [[ "$path" == "$REPO_ROOT"/* ]]; then
    printf '%s' "${path#$REPO_ROOT/}"
  else
    printf '%s' "$path"
  fi
}

display_path() {
  local path="$1"
  if [[ -n "${HOME:-}" && "$path" == "$HOME"/* ]]; then
    printf '~/%s' "${path#$HOME/}"
  else
    printf '%s' "$path"
  fi
}

if [[ ! -d "$REPO_ROOT/specs" ]]; then
  echo "# Repository Drift Report"
  echo
  echo "Generated: $GENERATED_AT"
  echo "Repo root: $(display_path "$REPO_ROOT")"
  echo
  echo "No specs directory found under repository root; no repo-local execution packets to inspect."
  echo "This is expected for the Bubbles source repository, which publishes durable framework behavior in docs, scripts, selftests, and the release manifest instead of keeping persistent source-repo specs."
  exit 0
fi

markdown_escape() {
  local value="$1"
  value="${value//$'\n'/ }"
  value="${value//$'\r'/ }"
  value="${value//|/\\|}"
  printf '%s' "$value"
}

ROWS_CATEGORY=()
ROWS_SPEC=()
ROWS_FINDING=()
ROWS_ACTION=()

add_row() {
  ROWS_CATEGORY+=("$1")
  ROWS_SPEC+=("$2")
  ROWS_FINDING+=("$3")
  ROWS_ACTION+=("$4")
}

state_string() {
  local expr="$1"
  local file="$2"
  jq -r "$expr" "$file"
}

validate_state() {
  local state_file="$1"
  local spec_rel="$2"
  if ! jq -e 'type == "object"' "$state_file" >/dev/null 2>&1; then
    echo "repo-drift-report: malformed or non-object JSON: $spec_rel/state.json" >&2
    exit 2
  fi
}

spec_age_epoch() {
  local state_file="$1"
  jq -r '
    .hardenedAt
    // .certifiedAt
    // .updatedAt
    // ([.executionHistory[]?
      | (.runCompletedAt? // .completedAt? // .completedAt // empty)
      | select(type == "string" and length > 0)] | sort | last)
    // ""
  ' "$state_file"
}

inspect_orphan_planning_packet() {
  local state_file="$1"
  local spec_rel="$2"
  local status planning_only link_type linked_impl age_value age_epoch age_days

  status="$(state_string '.status // ""' "$state_file")"
  [[ "$status" == "specs_hardened" ]] || return 0

  planning_only="$(state_string 'if .planningOnly == true then "true" else "false" end' "$state_file")"
  [[ "$planning_only" != "true" ]] || return 0

  link_type="$(state_string '(.linkedImplementationSpec | type) // "null"' "$state_file")"
  linked_impl="$(state_string '.linkedImplementationSpec // ""' "$state_file")"
  if [[ "$link_type" == "string" && ! "$linked_impl" =~ ^[[:space:]]*$ ]]; then
    return 0
  fi

  age_value="$(spec_age_epoch "$state_file")"
  [[ -n "$age_value" ]] || return 0
  age_epoch="$(rfc3339_epoch "$age_value")"
  [[ -n "$age_epoch" ]] || return 0
  age_days=$(( (CURRENT_EPOCH - age_epoch) / 86400 ))
  if [[ "$age_days" -gt 30 ]]; then
    add_row \
      "orphan-planning-packet" \
      "$spec_rel" \
      "status=specs_hardened ageDays=$age_days planningOnly=false linkedImplementationSpec missing" \
      "bubbles.plan/bubbles.validate: link implementation spec or mark planningOnly:true with justification"
  fi
}

tracked_planning_paths_for_spec() {
  local spec_abs="$1"
  local spec_rel="$2"

  [[ -f "$spec_abs/spec.md" ]] && printf '%s\n' "$spec_rel/spec.md"
  [[ -f "$spec_abs/design.md" ]] && printf '%s\n' "$spec_rel/design.md"
  [[ -f "$spec_abs/scopes.md" ]] && printf '%s\n' "$spec_rel/scopes.md"
  [[ -f "$spec_abs/scopes/_index.md" ]] && printf '%s\n' "$spec_rel/scopes/_index.md"
  if [[ -d "$spec_abs/scopes" ]]; then
    while IFS= read -r scope_file; do
      printf '%s\n' "$(relative_path "$scope_file")"
    done < <(find "$spec_abs/scopes" -mindepth 2 -maxdepth 2 -type f -name 'scope.md' | sort)
  fi
}

planning_family() {
  local file="$1"
  case "$file" in
    */spec.md) printf 'spec.md' ;;
    */design.md) printf 'design.md' ;;
    */scopes.md) printf 'scopes.md' ;;
    */scopes/_index.md) printf 'scopes/_index.md' ;;
    */scopes/*/scope.md) printf 'scopes/*/scope.md' ;;
    *) printf 'planning-file' ;;
  esac
}

collect_changed_paths_since() {
  local since_value="$1"
  shift
  local current_hash=""
  local current_date=""
  local current_subject=""
  local line

  while IFS= read -r line; do
    if [[ "$line" == @@DRIFT@@$'\t'* ]]; then
      IFS=$'\t' read -r _ current_hash current_date current_subject <<< "$line"
      continue
    fi

    if [[ -n "$line" && -n "$current_hash" ]]; then
      printf 'commit=%s date=%s file=%s subject=%s\n' "$current_hash" "$current_date" "$line" "$current_subject"
    fi
  done < <(git -C "$REPO_ROOT" log --format='@@DRIFT@@%x09%H%x09%cI%x09%s' --name-only --since="$since_value" -- "$@")

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    printf 'commit=WORKTREE date=uncommitted file=%s subject=uncommitted edit\n' "$line"
  done < <(git -C "$REPO_ROOT" diff --name-only -- "$@")

  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    printf 'commit=INDEX date=staged file=%s subject=staged edit\n' "$line"
  done < <(git -C "$REPO_ROOT" diff --cached --name-only -- "$@")
}

inspect_post_cert_planning_edits() {
  local state_file="$1"
  local spec_abs="$2"
  local spec_rel="$3"
  local status certified_at certified_epoch requires_revalidation
  local tracked_paths=()
  local entry file family seen="|"

  status="$(state_string '.status // ""' "$state_file")"
  [[ "$status" == "done" || "$status" == "done_with_concerns" ]] || return 0

  certified_at="$(state_string '.certifiedAt // ""' "$state_file")"
  [[ -n "$certified_at" ]] || return 0
  certified_epoch="$(rfc3339_epoch "$certified_at")"
  [[ -n "$certified_epoch" ]] || return 0
  requires_revalidation="$(state_string 'if .requiresRevalidation == true then "true" else "false" end' "$state_file")"

  while IFS= read -r path; do
    [[ -n "$path" ]] && tracked_paths+=("$path")
  done < <(tracked_planning_paths_for_spec "$spec_abs" "$spec_rel")
  [[ "${#tracked_paths[@]}" -gt 0 ]] || return 0

  while IFS= read -r entry; do
    [[ -n "$entry" ]] || continue
    file="${entry#* file=}"
    file="${file%% subject=*}"
    family="$(planning_family "$file")"
    if [[ "$seen" == *"|$family|"* ]]; then
      continue
    fi
    seen="${seen}${family}|"
    if [[ "$requires_revalidation" == "true" ]]; then
      add_row \
        "post-cert-planning-edit" \
        "$spec_rel" \
        "changedFamily=$family after certifiedAt=$certified_at; requiresRevalidation=true" \
        "bubbles.validate: recertify after review or demote until revalidation completes"
    else
      add_row \
        "post-cert-planning-edit" \
        "$spec_rel" \
        "changedFamily=$family after certifiedAt=$certified_at" \
        "bubbles.validate/spec-review: recertify, set requiresRevalidation:true, or demote"
    fi
  done < <(collect_changed_paths_since "$certified_at" "${tracked_paths[@]}")
}

extract_report_references() {
  local report_file="$1"
  awk '
    /^#{1,6}[[:space:]]+/ {
      section = $0
      sub(/^#+[[:space:]]+/, "", section)
      next
    }
    {
      line = $0
      while (match(line, /[A-Za-z0-9._\/:-]+\.[A-Za-z0-9][A-Za-z0-9._-]*/)) {
        token = substr(line, RSTART, RLENGTH)
        print section "\t" token
        line = substr(line, RSTART + RLENGTH)
      }
    }
  ' "$report_file"
}

is_source_reference() {
  local token="$1"
  local spec_rel="$2"

  token="${token#./}"
  token="${token%%#*}"
  [[ -n "$token" ]] || return 1
  [[ "$token" == /* ]] && return 1
  [[ -f "$REPO_ROOT/$token" ]] || return 1

  case "$token" in
    "$spec_rel"/state.json|"$spec_rel"/spec.md|"$spec_rel"/design.md|"$spec_rel"/scopes.md|"$spec_rel"/uservalidation.md|"$spec_rel"/report.md|"$spec_rel"/scopes/_index.md|"$spec_rel"/scopes/*/scope.md|"$spec_rel"/scopes/*/report.md)
      return 1
      ;;
  esac

  return 0
}

inspect_post_cert_source_edits() {
  local state_file="$1"
  local spec_abs="$2"
  local spec_rel="$3"
  local status certified_at certified_epoch report_file section token normalized seen_refs="|"
  local entry change_seen file

  status="$(state_string '.status // ""' "$state_file")"
  [[ "$status" == "done" || "$status" == "done_with_concerns" ]] || return 0

  certified_at="$(state_string '.certifiedAt // ""' "$state_file")"
  [[ -n "$certified_at" ]] || return 0
  certified_epoch="$(rfc3339_epoch "$certified_at")"
  [[ -n "$certified_epoch" ]] || return 0

  report_file="$spec_abs/report.md"
  [[ -f "$report_file" ]] || return 0

  while IFS=$'\t' read -r section token; do
    [[ -n "$token" ]] || continue
    normalized="${token#./}"
    normalized="${normalized%%#*}"
    if ! is_source_reference "$normalized" "$spec_rel"; then
      continue
    fi
    if [[ "$seen_refs" == *"|$normalized|$section|"* ]]; then
      continue
    fi
    seen_refs="${seen_refs}${normalized}|${section}|"

    change_seen="false"
    while IFS= read -r entry; do
      [[ -n "$entry" ]] || continue
      file="${entry#* file=}"
      file="${file%% subject=*}"
      [[ "$file" == "$normalized" ]] || continue
      if [[ "$change_seen" == "false" ]]; then
        add_row \
          "post-cert-source-edit" \
          "$spec_rel" \
          "sourceFile=$normalized changed after certifiedAt=$certified_at; referencedIn=${section:-<unknown report section>}" \
          "bubbles.validate/spec-review: revalidate report evidence against changed source file"
        change_seen="true"
      fi
    done < <(collect_changed_paths_since "$certified_at" "$normalized")
  done < <(extract_report_references "$report_file")
}

inspect_requires_revalidation() {
  local state_file="$1"
  local spec_rel="$2"
  local requires reason dependencies

  requires="$(state_string 'if .requiresRevalidation == true then "true" else "false" end' "$state_file")"
  [[ "$requires" == "true" ]] || return 0

  reason="$(jq -r '.revalidationReason // .requiresRevalidationReason // .revalidation.reason // ""' "$state_file")"
  dependencies="$(jq -r 'if ((.specDependsOn // []) | length) > 0 then (.specDependsOn | join(", ")) else "" end' "$state_file")"
  [[ -n "$reason" ]] || reason="<none>"
  [[ -n "$dependencies" ]] || dependencies="<none>"

  add_row \
    "requires-revalidation" \
    "$spec_rel" \
    "requiresRevalidation=true; reason=$reason; dependencies=$dependencies" \
    "bubbles.validate: replay impacted scenarios and clear flag only after certified review"
}

first_non_empty_line() {
  local value="$1"
  local line
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    printf '%s' "$line"
    return 0
  done <<< "$value"
  printf '<no diagnostic output>'
}

inspect_dependency_guard() {
  local spec_abs="$1"
  local spec_rel="$2"
  local guard="$SCRIPT_DIR/inter-spec-dependency-guard.sh"
  local output rc summary

  [[ -f "$guard" ]] || return 0

  set +e
  output="$(bash "$guard" "$spec_abs" --quiet --repo-root "$REPO_ROOT" 2>&1)"
  rc=$?
  set -e

  if [[ "$rc" -eq 0 ]]; then
    return 0
  fi
  if [[ "$rc" -eq 1 ]]; then
    summary="$(first_non_empty_line "$output")"
    add_row \
      "dependency-revalidation" \
      "$spec_rel" \
      "G089 dependency finding: $summary" \
      "bubbles.plan/bubbles.validate: repair specDependsOn or recertify dependencies"
    return 0
  fi

  echo "repo-drift-report: G089 dependency inspection runtime error for $spec_rel" >&2
  echo "$output" >&2
  exit 2
}

while IFS= read -r state_file; do
  spec_abs="$(cd "$(dirname "$state_file")" && pwd -P)"
  spec_rel="$(relative_path "$spec_abs")"
  validate_state "$state_file" "$spec_rel"

  inspect_orphan_planning_packet "$state_file" "$spec_rel"
  inspect_post_cert_planning_edits "$state_file" "$spec_abs" "$spec_rel"
  inspect_post_cert_source_edits "$state_file" "$spec_abs" "$spec_rel"
  inspect_requires_revalidation "$state_file" "$spec_rel"
  inspect_dependency_guard "$spec_abs" "$spec_rel"
done < <(find "$REPO_ROOT/specs" -mindepth 2 -maxdepth 2 -type f -name 'state.json' | sort)

echo "# Repository Drift Report"
echo
echo "Generated: $GENERATED_AT"
echo
echo "## Repository: $(display_path "$REPO_ROOT")"
echo
echo "| Category | Spec | Finding | Recommended Owner / Action |"
echo "|---|---|---|---|"

if [[ "${#ROWS_CATEGORY[@]}" -eq 0 ]]; then
  echo "| none | . | No drift findings detected | No action |"
else
  for index in "${!ROWS_CATEGORY[@]}"; do
    printf '| %s | %s | %s | %s |\n' \
      "$(markdown_escape "${ROWS_CATEGORY[$index]}")" \
      "$(markdown_escape "${ROWS_SPEC[$index]}")" \
      "$(markdown_escape "${ROWS_FINDING[$index]}")" \
      "$(markdown_escape "${ROWS_ACTION[$index]}")"
  done
fi

echo
echo "_Informational only: drift findings do not change this script's exit status._"

exit 0