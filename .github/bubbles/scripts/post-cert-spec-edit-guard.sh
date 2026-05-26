#!/usr/bin/env bash
set -euo pipefail

# post-cert-spec-edit-guard.sh
#
# Gate G088 - post_certification_spec_edit_gate.
#
# Certified specs must not silently change planning truth after certification.
# For specs whose top-level state.status is done or legacy read-only
# done_with_concerns compatibility, this guard checks git history for commits after
# top-level certifiedAt touching spec.md, design.md, scopes.md,
# scopes/_index.md, or per-scope scope.md files. G092 makes
# done_with_concerns read-only compatibility only; touched or recertified
# specs migrate to done plus observations or blocked.
#
# Usage:
#   bash bubbles/scripts/post-cert-spec-edit-guard.sh <specDir> [--quiet]
#
# Exit codes:
#   0  clean
#   1  one or more G088 post-certification edit violations
#   2  runtime error, invalid arguments, missing state, malformed JSON/date, or
#      git inspection error

QUIET="false"
SPEC_DIR_INPUT=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/post-cert-spec-edit-guard.sh <specDir> [--quiet]

Arguments:
  <specDir>  Spec directory containing state.json.

Optional:
  --quiet    Suppress success output.
  -h, --help Print this usage and exit.

Exit codes:
  0 = clean
  1 = G088 post-certification planning edit violation
  2 = runtime error, invalid arguments, missing state, malformed JSON/date, or git inspection error
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
      echo "post-cert-spec-edit-guard: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$SPEC_DIR_INPUT" ]]; then
        echo "post-cert-spec-edit-guard: only one specDir may be supplied" >&2
        usage >&2
        exit 2
      fi
      SPEC_DIR_INPUT="$1"
      shift
      ;;
  esac
done

if [[ -z "$SPEC_DIR_INPUT" ]]; then
  echo "post-cert-spec-edit-guard: missing required specDir" >&2
  usage >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "post-cert-spec-edit-guard: jq is required but not found in PATH" >&2
  exit 2
fi

if [[ ! -d "$SPEC_DIR_INPUT" ]]; then
  echo "post-cert-spec-edit-guard: specDir does not exist: $SPEC_DIR_INPUT" >&2
  exit 2
fi

SPEC_DIR_ABS="$(cd "$SPEC_DIR_INPUT" && pwd -P)"
STATE_FILE="$SPEC_DIR_ABS/state.json"

if [[ ! -f "$STATE_FILE" ]]; then
  echo "post-cert-spec-edit-guard: state.json not found: $STATE_FILE" >&2
  exit 2
fi

if ! jq -e 'type == "object"' "$STATE_FILE" >/dev/null 2>&1; then
  echo "post-cert-spec-edit-guard: malformed or non-object JSON: $STATE_FILE" >&2
  exit 2
fi

rfc3339_epoch() {
  local value="$1"
  jq -nr --arg d "$value" 'try ($d | fromdateiso8601 | floor) catch empty'
}

state_type_for() {
  local expr="$1"
  jq -r "$expr" "$STATE_FILE"
}

status_type="$(state_type_for 'if has("status") then (.status | type) else "missing" end')"
if [[ "$status_type" != "string" ]]; then
  echo "post-cert-spec-edit-guard: state.status must be a string in $STATE_FILE" >&2
  exit 2
fi

status="$(state_type_for '.status')"
spec_rel="$SPEC_DIR_INPUT"

legacy_status_compatible() {
  jq -e '(.legacyStatusCompatibility == true) or (.certification.legacyStatusCompatibility == true)' "$STATE_FILE" >/dev/null 2>&1
}

recertification_active() {
  jq -e '
    (.requiresRevalidation == true)
    or (.touchedForRecertification == true)
    or (.recertifying == true)
    or (.certification.touched == true)
    or (.certification.recertifying == true)
  ' "$STATE_FILE" >/dev/null 2>&1
}

if [[ "$status" != "done" && "$status" != "done_with_concerns" ]]; then
  if [[ "$QUIET" != "true" ]]; then
    echo "post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=$spec_rel status=$status is not certified done"
  fi
  exit 0
fi

if [[ "$status" == "done_with_concerns" ]]; then
  if ! legacy_status_compatible || recertification_active; then
    echo "G092 strict_terminal_status_gate violation: $spec_rel uses new or recertified done_with_concerns; migrate to status done with observations[] or status blocked" >&2
    exit 1
  fi
fi

if ! command -v git >/dev/null 2>&1; then
  echo "post-cert-spec-edit-guard: git is required but not found in PATH" >&2
  exit 2
fi

if ! REPO_ROOT="$(git -C "$SPEC_DIR_ABS" rev-parse --show-toplevel 2>/dev/null)"; then
  echo "post-cert-spec-edit-guard: specDir is not inside a git worktree: $SPEC_DIR_INPUT" >&2
  exit 2
fi

REPO_ROOT="$(cd "$REPO_ROOT" && pwd -P)"

if [[ "$SPEC_DIR_ABS" != "$REPO_ROOT"/* ]]; then
  echo "post-cert-spec-edit-guard: specDir is outside the git repository root: $SPEC_DIR_ABS" >&2
  exit 2
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

spec_rel="$(relative_path "$SPEC_DIR_ABS")"

certified_type="$(state_type_for 'if has("certifiedAt") then (.certifiedAt | type) else "missing" end')"
if [[ "$certified_type" != "string" ]]; then
  echo "post-cert-spec-edit-guard: G088 requires top-level certifiedAt for certified spec $spec_rel (status=$status)" >&2
  exit 2
fi

certified_at="$(state_type_for '.certifiedAt')"
if [[ "$certified_at" =~ ^[[:space:]]*$ ]]; then
  echo "post-cert-spec-edit-guard: certifiedAt is empty for certified spec $spec_rel" >&2
  exit 2
fi

certified_epoch="$(rfc3339_epoch "$certified_at")"
if [[ -z "$certified_epoch" ]]; then
  echo "post-cert-spec-edit-guard: malformed certifiedAt timestamp for $spec_rel: $certified_at" >&2
  exit 2
fi

requires_type="$(state_type_for 'if has("requiresRevalidation") then (.requiresRevalidation | type) else "missing" end')"
requires_revalidation="false"
if [[ "$requires_type" == "boolean" ]]; then
  requires_revalidation="$(state_type_for 'if .requiresRevalidation == true then "true" else "false" end')"
elif [[ "$requires_type" != "missing" ]]; then
  echo "post-cert-spec-edit-guard: requiresRevalidation must be boolean when present in $STATE_FILE" >&2
  exit 2
fi

latest_current_review="$(jq -r '
  [
    .executionHistory[]?
    | select((.agent? // "") == "bubbles.spec-review")
    | select(((.reviewStatus? // .reviewVerdict? // .verdict? // "") | ascii_upcase) == "CURRENT")
    | (.runCompletedAt? // .completedAt? // .reviewedAt? // empty)
    | select(type == "string" and length > 0)
  ]
  | sort
  | last // ""
' "$STATE_FILE")"

latest_current_review_epoch=""
if [[ -n "$latest_current_review" ]]; then
  latest_current_review_epoch="$(rfc3339_epoch "$latest_current_review")"
  if [[ -z "$latest_current_review_epoch" ]]; then
    echo "post-cert-spec-edit-guard: malformed bubbles.spec-review CURRENT timestamp for $spec_rel: $latest_current_review" >&2
    exit 2
  fi
fi

tracked_paths=()

add_tracked_file() {
  local rel="$1"
  if [[ -f "$REPO_ROOT/$rel" ]]; then
    tracked_paths+=("$rel")
  fi
}

add_tracked_file "$spec_rel/spec.md"
add_tracked_file "$spec_rel/design.md"
add_tracked_file "$spec_rel/scopes.md"
add_tracked_file "$spec_rel/scopes/_index.md"

if [[ -d "$SPEC_DIR_ABS/scopes" ]]; then
  while IFS= read -r scope_file; do
    tracked_paths+=("$(relative_path "$scope_file")")
  done < <(find "$SPEC_DIR_ABS/scopes" -mindepth 2 -maxdepth 2 -type f -name 'scope.md' | sort)
fi

if [[ "${#tracked_paths[@]}" -eq 0 ]]; then
  echo "post-cert-spec-edit-guard: no planning truth files found under $spec_rel" >&2
  exit 2
fi

LOG_FILE="$(mktemp -t g088-git-log-XXXXXXXX)"
DIFF_FILE="$(mktemp -t g088-git-diff-XXXXXXXX)"
cleanup() {
  rm -f "$LOG_FILE" "$DIFF_FILE" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

if ! git -C "$REPO_ROOT" log --format='@@G088@@%x09%H%x09%cI%x09%s' --name-only --since="$certified_at" -- "${tracked_paths[@]}" > "$LOG_FILE"; then
  echo "post-cert-spec-edit-guard: git log inspection failed for $spec_rel" >&2
  exit 2
fi

if ! git -C "$REPO_ROOT" diff --name-only -- "${tracked_paths[@]}" > "$DIFF_FILE"; then
  echo "post-cert-spec-edit-guard: git diff inspection failed for $spec_rel" >&2
  exit 2
fi

if ! git -C "$REPO_ROOT" diff --cached --name-only -- "${tracked_paths[@]}" >> "$DIFF_FILE"; then
  echo "post-cert-spec-edit-guard: staged git diff inspection failed for $spec_rel" >&2
  exit 2
fi

post_cert_entries=()
current_hash=""
current_date=""
current_subject=""

while IFS= read -r line; do
  if [[ "$line" == @@G088@@$'\t'* ]]; then
    IFS=$'\t' read -r _ current_hash current_date current_subject <<< "$line"
    continue
  fi

  if [[ -n "$line" && -n "$current_hash" ]]; then
    post_cert_entries+=("commit=$current_hash date=$current_date file=$line subject=$current_subject")
  fi
done < "$LOG_FILE"

while IFS= read -r dirty_path; do
  [[ -n "$dirty_path" ]] || continue
  post_cert_entries+=("commit=WORKTREE date=uncommitted file=$dirty_path subject=uncommitted planning truth edit")
done < "$DIFF_FILE"

if [[ "${#post_cert_entries[@]}" -gt 0 && "$requires_revalidation" == "true" ]]; then
  if [[ "$QUIET" != "true" ]]; then
    echo "post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=$spec_rel status=$status requiresRevalidation=true postCertEdits=${#post_cert_entries[@]}"
  fi
  exit 0
fi

if [[ "${#post_cert_entries[@]}" -gt 0 ]]; then
  echo "G088 post_certification_spec_edit_gate violation: certified planning truth changed after certifiedAt" >&2
  echo "  spec: $spec_rel" >&2
  echo "  status: $status" >&2
  echo "  certifiedAt: $certified_at" >&2
  echo "  trackedFiles: ${#tracked_paths[@]}" >&2
  echo "  postCertEdits: ${#post_cert_entries[@]}" >&2
  echo "  remediation: demote status out of done, set requiresRevalidation:true, or complete a current bubbles.spec-review recertification and update certifiedAt after the edit" >&2
  echo "  G092: legacy done_with_concerns is read-only compatibility only; touched or recertified specs must migrate to done plus observations or blocked" >&2
  echo "  commits/files:" >&2
  for entry in "${post_cert_entries[@]}"; do
    echo "    - $entry" >&2
  done
  exit 1
fi

if [[ "$QUIET" != "true" ]]; then
  if [[ -n "$latest_current_review" && "$latest_current_review_epoch" -le "$certified_epoch" ]]; then
    echo "post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=$spec_rel status=$status certifiedAt=$certified_at currentSpecReview=$latest_current_review trackedFiles=${#tracked_paths[@]}"
  else
    echo "post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=$spec_rel status=$status certifiedAt=$certified_at trackedFiles=${#tracked_paths[@]}"
  fi
fi

exit 0