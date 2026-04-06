#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ "$SCRIPT_DIR" == *"/.github/bubbles/scripts" ]]; then
  REPO_ROOT="${SCRIPT_DIR%/.github/bubbles/scripts}"
  FRAMEWORK_ROOT="$REPO_ROOT/.github/bubbles"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
  FRAMEWORK_ROOT="$REPO_ROOT/bubbles"
fi

source "$FRAMEWORK_ROOT/scripts/trust-metadata.sh"
source "$FRAMEWORK_ROOT/scripts/interop-registry.sh"

REGISTRY_FILE="$(bubbles_interop_registry_path "$REPO_ROOT")"
PROJECT_ROOT="$REPO_ROOT/.github/bubbles-project"
IMPORTS_ROOT="$PROJECT_ROOT/imports"
PROPOSALS_ROOT="$PROJECT_ROOT/proposals"

die() {
  echo "ERROR: $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  bash bubbles/scripts/interop-intake.sh detect [path]
  bash bubbles/scripts/interop-intake.sh import [source] [path] [--review-only] [--dry-run]
  bash bubbles/scripts/interop-intake.sh apply [source] [--safe]
  bash bubbles/scripts/interop-intake.sh status

Notes:
  - Import stays review-only and writes only under .github/bubbles-project/imports/** and .github/bubbles-project/proposals/**.
  - Apply-mode supports only explicit project-owned targets recorded in each import manifest entry.
  - Non-dry-run output is written only under .github/bubbles-project/imports/** and .github/bubbles-project/proposals/**.
  - If [source] is omitted for import, all detected sources under [path] are imported.
  - If [path] is omitted, BUBBLES_INTEROP_FIXTURE_DIR is used when set; otherwise the repo root is scanned.
EOF
}

array_contains() {
  local needle="$1"
  shift
  local item=''

  for item in "$@"; do
    if [[ "$item" == "$needle" ]]; then
      return 0
    fi
  done

  return 1
}

ensure_registry() {
  [[ -f "$REGISTRY_FILE" ]] || die "Missing interop registry: $REGISTRY_FILE"
}

json_escape() {
  local raw="$1"
  raw=${raw//\\/\\\\}
  raw=${raw//"/\\"}
  raw=${raw//$'\n'/\\n}
  raw=${raw//$'\r'/\\r}
  raw=${raw//$'\t'/\\t}
  printf '%s' "$raw"
}

slugify() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//'
}

default_interop_timestamp() {
  date -u +%Y%m%dT%H%M%SZ
}

sanitize_interop_timestamp() {
  local raw_timestamp="$1"

  if [[ "$raw_timestamp" =~ ^[0-9]{8}T[0-9]{6}Z$ ]]; then
    printf '%s\n' "$raw_timestamp"
    return 0
  fi

  die "BUBBLES_INTEROP_TIMESTAMP must match YYYYMMDDTHHMMSSZ"
}

resolve_interop_timestamp() {
  local raw_timestamp="${BUBBLES_INTEROP_TIMESTAMP:-$(default_interop_timestamp)}"

  sanitize_interop_timestamp "$raw_timestamp"
}

resolve_scan_root() {
  local requested_path="${1:-}"

  if [[ -n "$requested_path" ]]; then
    if [[ "$requested_path" = /* ]]; then
      printf '%s\n' "$requested_path"
    else
      printf '%s\n' "$REPO_ROOT/$requested_path"
    fi
    return 0
  fi

  if [[ -n "${BUBBLES_INTEROP_FIXTURE_DIR:-}" ]]; then
    printf '%s\n' "$BUBBLES_INTEROP_FIXTURE_DIR"
  else
    printf '%s\n' "$REPO_ROOT"
  fi
}

expand_target_template() {
  local template="$1"
  local source_id="$2"
  local timestamp="$3"
  printf '%s\n' "${template//<source>/$source_id}" | sed "s#<timestamp>#$timestamp#g"
}

primary_class_for_file() {
  local relative_path="$1"

  case "$relative_path" in
    CLAUDE.md|.cursorrules|.clinerules)
      printf '%s\n' 'instruction'
      ;;
    .roomodes|*/modes/*)
      printf '%s\n' 'workflow-mode'
      ;;
    */commands/*)
      printf '%s\n' 'command-surface'
      ;;
    */agents/*)
      printf '%s\n' 'tool-request'
      ;;
    */rules/*)
      printf '%s\n' 'guardrail'
      ;;
    *)
      printf '%s\n' 'docs-reference'
      ;;
  esac
}

apply_target_key_for_class() {
  local primary_class="$1"

  case "$primary_class" in
    instruction)
      printf '%s\n' 'applyTargetInstruction'
      ;;
    guardrail)
      printf '%s\n' 'applyTargetGuardrail'
      ;;
    command-surface)
      printf '%s\n' 'applyTargetCommandSurface'
      ;;
    tool-request)
      printf '%s\n' 'applyTargetToolRequest'
      ;;
    docs-reference)
      printf '%s\n' 'applyTargetDocsReference'
      ;;
    *)
      return 1
      ;;
  esac
}

apply_target_for_class() {
  local source_id="$1"
  local primary_class="$2"
  local target_key=''
  local target_template=''

  target_key="$(apply_target_key_for_class "$primary_class" 2>/dev/null)" || return 1
  target_template="$(bubbles_interop_scalar "$REGISTRY_FILE" "$source_id" "$target_key")"
  [[ -n "$target_template" ]] || return 1
  printf '%s\n' "$target_template"
}

candidate_target_for_class() {
  local source_id="$1"
  local primary_class="$2"

  case "$primary_class" in
    instruction|guardrail)
      printf '%s\n' ".github/instructions/imported-${source_id}.instructions.md"
      ;;
    command-surface)
      printf '%s\n' ".specify/memory/imported-${source_id}-agents.md"
      ;;
    tool-request)
      printf '%s\n' "scripts/imported-${source_id}-tooling.md"
      ;;
    docs-reference)
      printf '%s\n' "docs/imported-${source_id}-migration-notes.md"
      ;;
    *)
      return 1
      ;;
  esac
}

should_propose_for_class() {
  local primary_class="$1"
  [[ "$primary_class" == 'workflow-mode' ]]
}

detect_sources() {
  local scan_root="$1"
  local source_id=''
  local detected='false'
  local matched_files=()

  ensure_registry

  while IFS= read -r source_id; do
    [[ -n "$source_id" ]] || continue
    mapfile -t matched_files < <(bubbles_interop_detect_files "$scan_root" "$REGISTRY_FILE" "$source_id")
    if [[ "${#matched_files[@]}" -gt 0 ]]; then
      detected='true'
      printf '%s\n' "$source_id"
    fi
  done < <(bubbles_interop_source_ids "$REGISTRY_FILE")

  [[ "$detected" == 'true' ]]
}

print_detect() {
  local scan_root="$1"
  local source_id=''
  local display_name=''
  local parser_kind=''
  local detected_any='false'
  local matched_file=''

  ensure_registry
  echo "Scan root: $scan_root"

  while IFS= read -r source_id; do
    [[ -n "$source_id" ]] || continue
    mapfile -t matched_files < <(bubbles_interop_detect_files "$scan_root" "$REGISTRY_FILE" "$source_id")
    [[ "${#matched_files[@]}" -gt 0 ]] || continue
    detected_any='true'
    display_name="$(bubbles_interop_scalar "$REGISTRY_FILE" "$source_id" displayName)"
    parser_kind="$(bubbles_interop_scalar "$REGISTRY_FILE" "$source_id" parserKind)"
    echo "Source: $source_id ($display_name)"
    echo "  Parser: $parser_kind"
    echo "  Files:"
    for matched_file in "${matched_files[@]}"; do
      echo "    - $matched_file"
    done
    echo "  Normalized classes: $(paste -sd ', ' < <(bubbles_interop_list "$REGISTRY_FILE" "$source_id" normalizedClasses))"
  done < <(bubbles_interop_source_ids "$REGISTRY_FILE")

  [[ "$detected_any" == 'true' ]] || die "No supported interop sources detected under $scan_root"
}

write_candidate_file() {
  local packet_dir="$1"
  local target_path="$2"
  local source_id="$3"
  local source_display="$4"
  shift 4
  local source_files=("$@")
  local candidate_path="$packet_dir/proposed-overrides/$target_path"
  local source_file=''

  mkdir -p "$(dirname "$candidate_path")"
  {
    echo "# Imported ${source_display} Candidate"
    echo
    echo "- Source ID: ${source_id}"
    echo "- Review Mode: review-only"
    echo
    echo "## Imported Files"
    for source_file in "${source_files[@]}"; do
      echo "- ${source_file}"
    done
    echo
    echo "## Notes"
    echo "This candidate stays under .github/bubbles-project/imports/**/proposed-overrides/ until a maintainer reviews it."
  } > "$candidate_path"
}

write_proposal_file() {
  local source_id="$1"
  local source_display="$2"
  local timestamp="$3"
  shift 3
  local workflow_files=("$@")
  local proposal_slug
  local proposal_path
  local workflow_file=''

  proposal_slug="$(slugify "$source_id-framework-workflow-review")"
  proposal_path="$PROPOSALS_ROOT/${timestamp:0:8}-${proposal_slug}.md"
  mkdir -p "$PROPOSALS_ROOT"

  {
    echo "# Bubbles Framework Change Proposal"
    echo
    echo "- Title: ${source_display} workflow-mode review"
    echo "- Slug: ${proposal_slug}"
    echo "- Created: ${timestamp}"
    echo "- Created From: $(basename "$REPO_ROOT")"
    echo
    echo "## Summary"
    echo
    echo "Imported ${source_display} assets request workflow-mode behavior that would collide with framework-managed Bubbles surfaces."
    echo
    echo "## Imported Workflow Files"
    for workflow_file in "${workflow_files[@]}"; do
      echo "- ${workflow_file}"
    done
    echo
    echo "## Why Proposal Routing Is Required"
    echo
    echo "These changes cannot land directly in project-owned files alone because the requested behavior maps to framework-managed mode or workflow surfaces."
    echo "Review the packet under .github/bubbles-project/imports/** first, then implement any accepted framework change in the Bubbles source repo."
  } > "$proposal_path"

  printf '%s\n' "${proposal_path#"$REPO_ROOT"/}"
}

write_normalized_json() {
  local normalized_path="$1"
  local source_id="$2"
  local source_display="$3"
  local parser_kind="$4"
  shift 4
  local source_files=("$@")
  local source_file=''
  local primary_class=''
  local checksum=''
  local index=0

  {
    echo '{'
    echo '  "version": 1,'
    printf '  "sourceId": "%s",\n' "$(json_escape "$source_id")"
    printf '  "displayName": "%s",\n' "$(json_escape "$source_display")"
    printf '  "parserKind": "%s",\n' "$(json_escape "$parser_kind")"
    echo '  "files": ['
    for source_file in "${source_files[@]}"; do
      primary_class="$(primary_class_for_file "$source_file")"
      checksum="$(bubbles_sha256_file "$SCAN_ROOT/$source_file")"
      printf '    {"path": "%s", "primaryClass": "%s", "sha256": "%s"}' \
        "$(json_escape "$source_file")" \
        "$(json_escape "$primary_class")" \
        "$(json_escape "$checksum")"
      index=$((index + 1))
      if [[ "$index" -lt "${#source_files[@]}" ]]; then
        echo ','
      else
        echo
      fi
    done
    echo '  ]'
    echo '}'
  } > "$normalized_path"
}

write_manifest_entry() {
  local entry_path="$1"
  local source_id="$2"
  local normalized_output="$3"
  local review_status="$4"
  local source_files_json="$5"
  local generated_targets_json="$6"
  local supported_apply_targets_json="$7"
  local unsupported_json="$8"
  local proposals_json="$9"
  local apply_status="${10}"
  local applied_targets_json="${11}"
  local collision_targets_json="${12}"

  {
    echo '{'
    printf '  "sourceId": "%s",\n' "$(json_escape "$source_id")"
    printf '  "sourceFiles": %s,\n' "$source_files_json"
    printf '  "normalizedOutput": "%s",\n' "$(json_escape "$normalized_output")"
    printf '  "generatedTargets": %s,\n' "$generated_targets_json"
    printf '  "supportedApplyTargets": %s,\n' "$supported_apply_targets_json"
    printf '  "unsupportedItems": %s,\n' "$unsupported_json"
    printf '  "proposalRefs": %s,\n' "$proposals_json"
    printf '  "reviewStatus": "%s",\n' "$(json_escape "$review_status")"
    printf '  "applyStatus": "%s",\n' "$(json_escape "$apply_status")"
    printf '  "appliedTargets": %s,\n' "$applied_targets_json"
    printf '  "collisionTargets": %s\n' "$collision_targets_json"
    echo '}'
  } > "$entry_path"
}

json_array_from_lines() {
  local values=("$@")
  local idx=0

  printf '['
  for idx in "${!values[@]}"; do
    [[ "$idx" -gt 0 ]] && printf ', '
    printf '"%s"' "$(json_escape "${values[$idx]}")"
  done
  printf ']'
}

rebuild_interop_manifest() {
  local manifest_path="$IMPORTS_ROOT/interop-manifest.json"
  local entry_path=''
  local entry_paths=()
  local idx=0

  mkdir -p "$IMPORTS_ROOT"
  while IFS= read -r entry_path; do
    [[ -n "$entry_path" ]] || continue
    entry_paths+=("$entry_path")
  done < <(find "$IMPORTS_ROOT" -mindepth 3 -maxdepth 3 -name manifest-entry.json | sort)

  {
    echo '{'
    echo '  "version": 1,'
    echo '  "imports": ['
    for idx in "${!entry_paths[@]}"; do
      sed 's/^/    /' "${entry_paths[$idx]}"
      if [[ "$idx" -lt $((${#entry_paths[@]} - 1)) ]]; then
        echo '    ,'
      fi
    done
    echo '  ]'
    echo '}'
  } > "$manifest_path"
}

write_translation_report() {
  local report_path="$1"
  local source_id="$2"
  local source_display="$3"
  local parser_kind="$4"
  local review_status="$5"
  shift 5
  local source_files=("$@")
  local source_file=''
  local supported_target=''
  local proposal_ref=''
  local primary_class=''

  {
    echo "# Review-Only Interop Translation Report"
    echo
    echo "- Source ID: ${source_id}"
    echo "- Display Name: ${source_display}"
    echo "- Parser Kind: ${parser_kind}"
    echo "- Review Status: ${review_status}"
    echo
    echo "## Imported Files"
    for source_file in "${source_files[@]}"; do
      primary_class="$(primary_class_for_file "$source_file")"
      echo "- ${source_file} (${primary_class})"
    done
    echo
    echo "## Supported Targets"
    while IFS= read -r supported_target; do
      [[ -n "$supported_target" ]] || continue
      echo "- ${supported_target}"
    done < <(bubbles_interop_list "$REGISTRY_FILE" "$source_id" supportedTargets)
    echo
    echo "## Unsupported Framework Targets"
    while IFS= read -r supported_target; do
      [[ -n "$supported_target" ]] || continue
      echo "- ${supported_target}"
    done < <(bubbles_interop_list "$REGISTRY_FILE" "$source_id" unsupportedTargets)
  } > "$report_path"
}

run_import() {
  local requested_source="${1:-}"
  local requested_path="${2:-}"
  local dry_run="$3"
  local timestamp=''
  local source_id=''
  local source_display=''
  local parser_kind=''
  local packet_dir=''
  local raw_root=''
  local normalized_path=''
  local report_path=''
  local manifest_entry_path=''
  local review_status='review-required'

  SCAN_ROOT="$(resolve_scan_root "$requested_path")"
  [[ -d "$SCAN_ROOT" ]] || die "Interop scan root does not exist: $SCAN_ROOT"
  ensure_registry
  timestamp="$(resolve_interop_timestamp)"

  local source_ids=()
  if [[ -n "$requested_source" ]]; then
    source_ids+=("$requested_source")
  else
    mapfile -t source_ids < <(detect_sources "$SCAN_ROOT")
  fi

  [[ "${#source_ids[@]}" -gt 0 ]] || die "No supported interop sources detected under $SCAN_ROOT"

  echo "Interop scan root: $SCAN_ROOT"
  echo "Review-only mode: enabled"
  echo "Dry run: $dry_run"

  for source_id in "${source_ids[@]}"; do
    source_display="$(bubbles_interop_scalar "$REGISTRY_FILE" "$source_id" displayName)"
    [[ -n "$source_display" ]] || die "Unknown interop source: $source_id"
    parser_kind="$(bubbles_interop_scalar "$REGISTRY_FILE" "$source_id" parserKind)"
    mapfile -t matched_files < <(bubbles_interop_detect_files "$SCAN_ROOT" "$REGISTRY_FILE" "$source_id")
    [[ "${#matched_files[@]}" -gt 0 ]] || die "No files matched $source_id under $SCAN_ROOT"

    packet_dir="$IMPORTS_ROOT/$source_id/$timestamp"
    raw_root="$packet_dir/raw"
    normalized_path="$packet_dir/normalized.json"
    report_path="$packet_dir/translation-report.md"
    manifest_entry_path="$packet_dir/manifest-entry.json"

    echo "Source: $source_id ($source_display)"
    echo "  Packet: ${packet_dir#"$REPO_ROOT"/}"
    echo "  Matched files: ${#matched_files[@]}"

    local generated_targets=()
    local supported_apply_targets=()
    local unsupported_items=()
    local proposal_refs=()
    local workflow_files=()
    local source_file=''
    local primary_class=''
    local candidate_target=''

    generated_targets+=("${raw_root#"$REPO_ROOT"/}")
    generated_targets+=("${normalized_path#"$REPO_ROOT"/}")
    generated_targets+=("${report_path#"$REPO_ROOT"/}")

    for source_file in "${matched_files[@]}"; do
      primary_class="$(primary_class_for_file "$source_file")"
      if should_propose_for_class "$primary_class"; then
        workflow_files+=("$source_file")
        unsupported_items+=("${source_file} -> workflow-mode requires framework proposal")
      else
        if candidate_target="$(candidate_target_for_class "$source_id" "$primary_class" 2>/dev/null)"; then
          generated_targets+=("${packet_dir#"$REPO_ROOT"/}/proposed-overrides/$candidate_target")
        fi
        if candidate_target="$(apply_target_for_class "$source_id" "$primary_class" 2>/dev/null)"; then
          candidate_target="$(expand_target_template "$candidate_target" "$source_id" "$timestamp")"
          if ! array_contains "$candidate_target" "${supported_apply_targets[@]}"; then
            supported_apply_targets+=("$candidate_target")
          fi
        fi
      fi
    done

    if [[ "$dry_run" == 'false' ]]; then
      mkdir -p "$raw_root" "$packet_dir/proposed-overrides"

      for source_file in "${matched_files[@]}"; do
        mkdir -p "$(dirname "$raw_root/$source_file")"
        cp "$SCAN_ROOT/$source_file" "$raw_root/$source_file"
      done

      write_normalized_json "$normalized_path" "$source_id" "$source_display" "$parser_kind" "${matched_files[@]}"
      write_translation_report "$report_path" "$source_id" "$source_display" "$parser_kind" "$review_status" "${matched_files[@]}"

      local created_candidates=()
      for source_file in "${matched_files[@]}"; do
        primary_class="$(primary_class_for_file "$source_file")"
        if candidate_target="$(candidate_target_for_class "$source_id" "$primary_class" 2>/dev/null)"; then
          if ! array_contains "$candidate_target" "${created_candidates[@]}"; then
            write_candidate_file "$packet_dir" "$candidate_target" "$source_id" "$source_display" "${matched_files[@]}"
            created_candidates+=("$candidate_target")
          fi
        fi
      done

      if [[ "${#workflow_files[@]}" -gt 0 ]]; then
        proposal_refs+=("$(write_proposal_file "$source_id" "$source_display" "$timestamp" "${workflow_files[@]}")")
      fi

      write_manifest_entry \
        "$manifest_entry_path" \
        "$source_id" \
        "${normalized_path#"$REPO_ROOT"/}" \
        "$review_status" \
        "$(json_array_from_lines "${matched_files[@]}")" \
        "$(json_array_from_lines "${generated_targets[@]}")" \
        "$(json_array_from_lines "${supported_apply_targets[@]}")" \
        "$(json_array_from_lines "${unsupported_items[@]}")" \
        "$(json_array_from_lines "${proposal_refs[@]}")" \
        "not-applied" \
        '[]' \
        '[]'
    fi

    echo "  Generated targets:"
    printf '    - %s\n' "${generated_targets[@]}"

    if [[ "${#unsupported_items[@]}" -gt 0 ]]; then
      echo "  Unsupported items:"
      printf '    - %s\n' "${unsupported_items[@]}"
    fi

    if [[ "${#proposal_refs[@]}" -gt 0 ]]; then
      echo "  Proposal refs:"
      printf '    - %s\n' "${proposal_refs[@]}"
    elif [[ "$dry_run" == 'true' && "${#workflow_files[@]}" -gt 0 ]]; then
      echo "  Proposal refs:"
      echo "    - .github/bubbles-project/proposals/${timestamp:0:8}-$(slugify "$source_id-framework-workflow-review").md"
    fi
  done

  if [[ "$dry_run" == 'false' ]]; then
    rebuild_interop_manifest
    echo "Interop manifest: ${IMPORTS_ROOT#"$REPO_ROOT"/}/interop-manifest.json"
  fi
}

print_status() {
  local entry_file=''
  local source_id=''
  local review_status=''
  local apply_status=''
  local normalized_output=''
  local proposal_refs=''
  local applied_targets=''
  local collision_targets=''

  if [[ ! -f "$IMPORTS_ROOT/interop-manifest.json" ]]; then
    echo "No interop imports recorded yet."
    return 0
  fi

  echo "Interop manifest: ${IMPORTS_ROOT#"$REPO_ROOT"/}/interop-manifest.json"

  while IFS= read -r entry_file; do
    [[ -n "$entry_file" ]] || continue
    source_id="$(bubbles_json_string_field "$entry_file" sourceId)"
    review_status="$(bubbles_json_string_field "$entry_file" reviewStatus)"
    apply_status="$(bubbles_json_string_field "$entry_file" applyStatus)"
    normalized_output="$(bubbles_json_string_field "$entry_file" normalizedOutput)"
    proposal_refs="$(bubbles_json_array_joined "$entry_file" proposalRefs ', ' 'none')"
    applied_targets="$(bubbles_json_array_joined "$entry_file" appliedTargets ', ' 'none')"
    collision_targets="$(bubbles_json_array_joined "$entry_file" collisionTargets ', ' 'none')"
    echo "- ${source_id}: reviewStatus=${review_status} applyStatus=${apply_status} normalizedOutput=${normalized_output} applied=${applied_targets} collisions=${collision_targets} proposals=${proposal_refs}"
  done < <(find "$IMPORTS_ROOT" -mindepth 3 -maxdepth 3 -name manifest-entry.json | sort)
}

main() {
  ensure_registry

  [[ $# -gt 0 ]] || {
    usage
    exit 1
  }

  local subcommand="$1"
  shift

  case "$subcommand" in
    detect)
      [[ $# -le 1 ]] || die "Usage: detect [path]"
      print_detect "$(resolve_scan_root "${1:-}")"
      ;;
    import)
      local review_only='false'
      local dry_run='false'
      local positionals=()

      while [[ $# -gt 0 ]]; do
        case "$1" in
          --review-only)
            review_only='true'
            shift
            ;;
          --dry-run)
            dry_run='true'
            shift
            ;;
          --help|-h)
            usage
            exit 0
            ;;
          *)
            positionals+=("$1")
            shift
            ;;
        esac
      done

      if [[ "$review_only" == 'false' ]]; then
        review_only='true'
      fi

      run_import "${positionals[0]:-}" "${positionals[1]:-}" "$dry_run"
      ;;
    apply)
      exec bash "$FRAMEWORK_ROOT/scripts/interop-apply.sh" "$@"
      ;;
    status)
      [[ $# -eq 0 ]] || die "Usage: status"
      print_status
      ;;
    --help|-h|help)
      usage
      ;;
    *)
      die "Unknown interop subcommand: $subcommand"
      ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi