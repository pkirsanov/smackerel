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

source "$FRAMEWORK_ROOT/scripts/interop-intake.sh"

apply_marker_start() {
  local source_id="$1"
  printf '<!-- GENERATED:INTEROP_%s_START -->\n' "$(printf '%s' "$source_id" | tr '[:lower:]-' '[:upper:]_')"
}

apply_marker_end() {
  local source_id="$1"
  printf '<!-- GENERATED:INTEROP_%s_END -->\n' "$(printf '%s' "$source_id" | tr '[:lower:]-' '[:upper:]_')"
}

render_instruction_content() {
  local source_id="$1"
  local source_display="$2"
  shift 2
  local source_files=("$@")
  local source_file=''

  echo "# Imported ${source_display} Instructions"
  echo
  echo "- Source ID: ${source_id}"
  echo "- Import Mode: supported-apply"
  echo "- Ownership Boundary: project-owned output only"
  echo

  for source_file in "${source_files[@]}"; do
    echo "## Source: ${source_file}"
    echo
    cat "$CURRENT_PACKET_RAW_ROOT/$source_file"
    echo
  done
}

render_agents_block() {
  local source_id="$1"
  local source_display="$2"
  shift 2
  local source_files=("$@")
  local source_file=''

  echo "$(apply_marker_start "$source_id")"
  echo
  echo "## Imported ${source_display} Recommendations"
  echo
  echo "These notes came from a supported interop apply flow and remain project-owned guidance."
  echo
  for source_file in "${source_files[@]}"; do
    echo "### Imported Source: ${source_file}"
    echo
    cat "$CURRENT_PACKET_RAW_ROOT/$source_file"
    echo
  done
  echo "$(apply_marker_end "$source_id")"
}

render_tooling_content() {
  local source_id="$1"
  local source_display="$2"
  shift 2
  local source_files=("$@")
  local source_file=''

  echo "# Imported ${source_display} Tooling Notes"
  echo
  echo "This project-owned helper note was generated from imported tool-request assets."
  echo
  for source_file in "${source_files[@]}"; do
    echo "## Source: ${source_file}"
    echo
    cat "$CURRENT_PACKET_RAW_ROOT/$source_file"
    echo
  done
}

render_skill_content() {
  local source_id="$1"
  local source_display="$2"
  shift 2
  local source_files=("$@")
  local source_file=''

  cat <<EOF
---
name: imported-${source_id}-migration
description: Project-owned migration notes imported from ${source_display} assets.
---

# Imported ${source_display} Migration Notes

This project-owned skill captures imported guidance that still requires maintainer review before it becomes first-class Bubbles workflow behavior.

## Imported Sources
EOF

  for source_file in "${source_files[@]}"; do
    echo "- ${source_file}"
  done

  echo
  echo "## Source Content"
  echo
  for source_file in "${source_files[@]}"; do
    echo "### ${source_file}"
    echo
    cat "$CURRENT_PACKET_RAW_ROOT/$source_file"
    echo
  done
}

render_target_content() {
  local target_path="$1"
  local source_id="$2"
  local source_display="$3"
  shift 3
  local source_files=("$@")

  case "$target_path" in
    .github/instructions/*.instructions.md)
      render_instruction_content "$source_id" "$source_display" "${source_files[@]}"
      ;;
    .specify/memory/agents.md)
      render_agents_block "$source_id" "$source_display" "${source_files[@]}"
      ;;
    scripts/*)
      render_tooling_content "$source_id" "$source_display" "${source_files[@]}"
      ;;
    .github/skills/*/SKILL.md)
      render_skill_content "$source_id" "$source_display" "${source_files[@]}"
      ;;
    *)
      return 1
      ;;
  esac
}

write_replaceable_block() {
  local target_file="$1"
  local source_id="$2"
  local content_file="$3"
  local start_marker=''
  local end_marker=''
  local temp_file=''

  start_marker="$(apply_marker_start "$source_id")"
  end_marker="$(apply_marker_end "$source_id")"
  temp_file="$(mktemp)"

  if [[ -f "$target_file" ]] && grep -Fq "$start_marker" "$target_file"; then
    awk -v start_marker="$start_marker" -v end_marker="$end_marker" -v content_file="$content_file" '
      BEGIN {
        in_block = 0
      }
      index($0, start_marker) {
        while ((getline line < content_file) > 0) {
          print line
        }
        close(content_file)
        in_block = 1
        next
      }
      index($0, end_marker) {
        in_block = 0
        next
      }
      !in_block {
        print
      }
    ' "$target_file" > "$temp_file"
  else
    if [[ -f "$target_file" ]]; then
      cat "$target_file" > "$temp_file"
      printf '\n' >> "$temp_file"
    fi
    cat "$content_file" >> "$temp_file"
  fi

  mv "$temp_file" "$target_file"
}

write_apply_collision_proposal() {
  local source_id="$1"
  local source_display="$2"
  local timestamp="$3"
  shift 3
  local collision_targets=("$@")
  local target=''
  local proposal_path=''

  proposal_path="$PROPOSALS_ROOT/${timestamp:0:8}-$(slugify "$source_id-apply-collision-review").md"
  mkdir -p "$PROPOSALS_ROOT"

  {
    echo "# Bubbles Apply Collision Proposal"
    echo
    echo "- Source ID: ${source_id}"
    echo "- Display Name: ${source_display}"
    echo "- Created: ${timestamp}"
    echo
    echo "## Collision Targets"
    for target in "${collision_targets[@]}"; do
      echo "- ${target}"
    done
    echo
    echo "## Resolution Guidance"
    echo
    echo "These targets already exist with conflicting project-owned content."
    echo "The supported apply flow left them untouched and preserved the review packet under .github/bubbles-project/imports/**/proposed-overrides/."
  } > "$proposal_path"

  printf '%s\n' "${proposal_path#"$REPO_ROOT"/}"
}

latest_manifest_entries() {
  local requested_source="${1:-}"
  local entry_file=''
  local source_id=''
  local -A latest_entries=()

  [[ -d "$IMPORTS_ROOT" ]] || return 0

  while IFS= read -r entry_file; do
    [[ -n "$entry_file" ]] || continue
    source_id="$(bubbles_json_string_field "$entry_file" sourceId)"
    [[ -n "$source_id" ]] || continue
    if [[ -n "$requested_source" && "$source_id" != "$requested_source" ]]; then
      continue
    fi
    latest_entries["$source_id"]="$entry_file"
  done < <(find "$IMPORTS_ROOT" -mindepth 3 -maxdepth 3 -name manifest-entry.json | sort)

  for source_id in "${!latest_entries[@]}"; do
    printf '%s\n' "${latest_entries[$source_id]}"
  done | sort
}

run_apply() {
  local requested_source="${1:-}"
  local safe_mode="$2"
  local timestamp=''
  local entry_file=''
  local source_id=''
  local source_display=''
  local packet_dir=''
  local target_path=''
  local primary_class=''
  local source_file=''
  local target_file=''
  local render_file=''
  local proposal_path=''
  local apply_status=''
  local -a entry_files=()
  local -a source_files=()
  local -a generated_targets=()
  local -a supported_apply_targets=()
  local -a unsupported_items=()
  local -a proposal_refs=()
  local -a applied_targets=()
  local -a collision_targets=()
  local -a target_paths=()
  local -a grouped_source_files=()
  declare -A target_sources=()

  [[ "$safe_mode" == 'true' ]] || die "Apply-mode requires --safe"
  timestamp="$(resolve_interop_timestamp)"
  mapfile -t entry_files < <(latest_manifest_entries "$requested_source")
  [[ "${#entry_files[@]}" -gt 0 ]] || die "No review-only interop packets are available to apply"

  echo "Apply mode: safe"

  for entry_file in "${entry_files[@]}"; do
    source_id="$(bubbles_json_string_field "$entry_file" sourceId)"
    source_display="$(bubbles_interop_scalar "$REGISTRY_FILE" "$source_id" displayName)"
    packet_dir="$(dirname "$entry_file")"
    CURRENT_PACKET_RAW_ROOT="$packet_dir/raw"

    mapfile -t source_files < <(bubbles_json_array_items "$entry_file" sourceFiles)
    mapfile -t generated_targets < <(bubbles_json_array_items "$entry_file" generatedTargets)
    mapfile -t supported_apply_targets < <(bubbles_json_array_items "$entry_file" supportedApplyTargets)
    mapfile -t unsupported_items < <(bubbles_json_array_items "$entry_file" unsupportedItems)
    mapfile -t proposal_refs < <(bubbles_json_array_items "$entry_file" proposalRefs)
    applied_targets=()
    collision_targets=()
    unset target_sources
    declare -A target_sources=()

    echo "Source: ${source_id} (${source_display})"
    echo "  Packet: ${packet_dir#"$REPO_ROOT"/}"

    for source_file in "${source_files[@]}"; do
      primary_class="$(primary_class_for_file "$source_file")"

      if should_propose_for_class "$primary_class"; then
        unsupported_items+=("${source_file} -> ${primary_class} remains proposal-only")
        continue
      fi

      target_path="$(apply_target_for_class "$source_id" "$primary_class" 2>/dev/null || true)"
      if [[ -z "$target_path" ]]; then
        unsupported_items+=("${source_file} -> no supported apply target")
        continue
      fi

      target_path="$(expand_target_template "$target_path" "$source_id" "$timestamp")"
      if ! array_contains "$target_path" "${supported_apply_targets[@]}"; then
        unsupported_items+=("${source_file} -> ${target_path} is outside the manifest-declared apply boundary")
        continue
      fi

      target_sources["$target_path"]+="${target_sources[$target_path]:+$'\n'}${source_file}"
    done

    if [[ "${#target_sources[@]}" -gt 0 ]]; then
      mapfile -t target_paths < <(printf '%s\n' "${!target_sources[@]}" | sort)
    else
      target_paths=()
    fi

    for target_path in "${target_paths[@]}"; do
      mapfile -t grouped_source_files < <(printf '%s\n' "${target_sources[$target_path]}")
      render_file="$(mktemp)"
      render_target_content "$target_path" "$source_id" "$source_display" "${grouped_source_files[@]}" > "$render_file"
      target_file="$REPO_ROOT/$target_path"

      if [[ "$target_path" == '.specify/memory/agents.md' ]]; then
        mkdir -p "$(dirname "$target_file")"
        write_replaceable_block "$target_file" "$source_id" "$render_file"
        applied_targets+=("$target_path")
      elif [[ -f "$target_file" ]]; then
        if cmp -s "$render_file" "$target_file"; then
          applied_targets+=("$target_path")
        else
          collision_targets+=("$target_path")
        fi
      else
        mkdir -p "$(dirname "$target_file")"
        mv "$render_file" "$target_file"
        render_file=''
        applied_targets+=("$target_path")
      fi

      if [[ -n "$render_file" && -f "$render_file" ]]; then
        rm -f "$render_file"
      fi
    done

    apply_status='not-applied'
    if [[ "${#collision_targets[@]}" -gt 0 ]]; then
      proposal_path="$(write_apply_collision_proposal "$source_id" "$source_display" "$timestamp" "${collision_targets[@]}")"
      if ! array_contains "$proposal_path" "${proposal_refs[@]}"; then
        proposal_refs+=("$proposal_path")
      fi
      apply_status='applied-with-collisions'
    elif [[ "${#applied_targets[@]}" -gt 0 ]]; then
      apply_status='applied'
    fi

    write_manifest_entry \
      "$entry_file" \
      "$source_id" \
      "$(bubbles_json_string_field "$entry_file" normalizedOutput)" \
      "$(bubbles_json_string_field "$entry_file" reviewStatus)" \
      "$(json_array_from_lines "${source_files[@]}")" \
      "$(json_array_from_lines "${generated_targets[@]}")" \
      "$(json_array_from_lines "${supported_apply_targets[@]}")" \
      "$(json_array_from_lines "${unsupported_items[@]}")" \
      "$(json_array_from_lines "${proposal_refs[@]}")" \
      "$apply_status" \
      "$(json_array_from_lines "${applied_targets[@]}")" \
      "$(json_array_from_lines "${collision_targets[@]}")"

    echo "  Applied targets:"
    if [[ "${#applied_targets[@]}" -gt 0 ]]; then
      printf '    - %s\n' "${applied_targets[@]}"
    else
      echo "    - none"
    fi
    if [[ "${#collision_targets[@]}" -gt 0 ]]; then
      echo "  Collision fallback:"
      printf '    - %s\n' "${collision_targets[@]}"
    fi
  done

  rebuild_interop_manifest
  echo "Interop manifest: ${IMPORTS_ROOT#"$REPO_ROOT"/}/interop-manifest.json"
}

main() {
  local safe_mode='false'
  local positionals=()

  ensure_registry

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --safe)
        safe_mode='true'
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

  run_apply "${positionals[0]:-}" "$safe_mode"
}

main "$@"