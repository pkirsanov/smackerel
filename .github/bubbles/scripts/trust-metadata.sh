#!/usr/bin/env bash

bubbles_sha256_raw() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$@"
    return 0
  fi

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$@"
    return 0
  fi

  echo "sha256sum or shasum is required for Bubbles trust metadata" >&2
  return 1
}

bubbles_sha256_file() {
  local target_file="$1"

  bubbles_sha256_raw "$target_file" | awk '{print $1}'
}

bubbles_sha256_stdin() {
  bubbles_sha256_raw | awk '{print $1}'
}

bubbles_current_timestamp() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

bubbles_provenance_ref_is_safe() {
  local ref_value="$1"

  [[ -n "$ref_value" ]] || return 1
  [[ "$ref_value" =~ ^[A-Za-z0-9._/@+-]+$ ]]
}

bubbles_join_list_items() {
  local separator="$1"
  shift
  local item
  local output=''

  for item in "$@"; do
    [[ -n "$item" ]] || continue
    if [[ -n "$output" ]]; then
      output+="$separator"
    fi
    output+="$item"
  done

  printf '%s' "$output"
}

bubbles_json_string_field_present() {
  local json_file="$1"
  local field_name="$2"

  grep -Eq '"'"$field_name"'"[[:space:]]*:[[:space:]]*"[^"]+"' "$json_file" 2>/dev/null
}

bubbles_json_number_field_present() {
  local json_file="$1"
  local field_name="$2"

  grep -Eq '"'"$field_name"'"[[:space:]]*:[[:space:]]*[0-9]+' "$json_file" 2>/dev/null
}

bubbles_json_array_field_present() {
  local json_file="$1"
  local field_name="$2"

  grep -Eq '"'"$field_name"'"[[:space:]]*:[[:space:]]*\[[^]]*\]' "$json_file" 2>/dev/null
}

bubbles_validate_release_manifest_schema() {
  local manifest_file="$1"
  local consumer_name="$2"
  local failures=0
  local field_name=''
  local field_type=''
  local requirement=''

  if [[ ! -f "$manifest_file" ]]; then
    echo "ERROR: ${consumer_name} requires release-manifest trust metadata at ${manifest_file}" >&2
    return 1
  fi

  while IFS=':' read -r field_name field_type requirement; do
    [[ -n "$field_name" ]] || continue

    case "$field_type" in
      string)
        if ! bubbles_json_string_field_present "$manifest_file" "$field_name"; then
          echo "ERROR: ${consumer_name} requires release-manifest.json field \"${field_name}\" as a non-empty JSON string (${requirement})" >&2
          failures=$((failures + 1))
        fi
        ;;
      number)
        if ! bubbles_json_number_field_present "$manifest_file" "$field_name"; then
          echo "ERROR: ${consumer_name} requires release-manifest.json field \"${field_name}\" as a JSON number (${requirement})" >&2
          failures=$((failures + 1))
        fi
        ;;
      array)
        if ! bubbles_json_array_field_present "$manifest_file" "$field_name"; then
          echo "ERROR: ${consumer_name} requires release-manifest.json field \"${field_name}\" as a JSON array (${requirement})" >&2
          failures=$((failures + 1))
        fi
        ;;
      *)
        echo "ERROR: Unsupported release-manifest schema validation type \"${field_type}\" for field \"${field_name}\"" >&2
        return 1
        ;;
    esac
  done <<'EOF'
schemaVersion:number:schema contract version
version:string:installed release version
gitSha:string:upstream source git SHA
generatedAt:string:release manifest generation timestamp
capabilityLedgerVersion:number:capability ledger source version
supportedProfiles:array:supported onboarding profiles
supportedInteropSources:array:supported interop sources
validatedSurfaces:array:validated trust surfaces
docsDigest:string:trust documentation digest
managedFileCount:number:framework-managed file count
EOF

  if [[ "$failures" -gt 0 ]]; then
    echo "ERROR: ${consumer_name} cannot continue with malformed release-manifest trust metadata. Regenerate the manifest from the Bubbles source repo and reinstall or upgrade." >&2
    return 1
  fi

  return 0
}

bubbles_json_string_field() {
  local json_file="$1"
  local field_name="$2"

  grep -oE '"'"$field_name"'"[[:space:]]*:[[:space:]]*"[^"]*"' "$json_file" 2>/dev/null \
    | head -1 \
    | sed -E 's/.*"([^"]*)"$/\1/'
}

bubbles_json_bool_field() {
  local json_file="$1"
  local field_name="$2"

  grep -oE '"'"$field_name"'"[[:space:]]*:[[:space:]]*(true|false)' "$json_file" 2>/dev/null \
    | head -1 \
    | sed -E 's/.*:[[:space:]]*(true|false)$/\1/'
}

bubbles_json_number_field() {
  local json_file="$1"
  local field_name="$2"

  grep -oE '"'"$field_name"'"[[:space:]]*:[[:space:]]*[0-9]+' "$json_file" 2>/dev/null \
    | head -1 \
    | sed -E 's/.*:[[:space:]]*([0-9]+)$/\1/'
}

bubbles_json_array_items() {
  local json_file="$1"
  local field_name="$2"
  local raw_items

  raw_items="$({
    grep -E '"'"$field_name"'"[[:space:]]*:' "$json_file" 2>/dev/null | head -1
  } || true)"

  if [[ -z "$raw_items" ]]; then
    return 0
  fi

  raw_items="${raw_items#*\[}"
  raw_items="${raw_items%%]*}"

  printf '%s\n' "$raw_items" \
    | tr ',' '\n' \
    | sed -E 's/^[[:space:]]*"//; s/"[[:space:]]*$//; s/^[[:space:]]+//; s/[[:space:]]+$//' \
    | sed '/^$/d'
}

bubbles_json_array_joined() {
  local json_file="$1"
  local field_name="$2"
  local separator="$3"
  local default_value="${4:-}"
  local joined_value=''
  local items=()

  mapfile -t items < <(bubbles_json_array_items "$json_file" "$field_name")
  joined_value="$(bubbles_join_list_items "$separator" "${items[@]}")"

  if [[ -n "$joined_value" ]]; then
    printf '%s' "$joined_value"
  else
    printf '%s' "$default_value"
  fi
}

bubbles_read_release_manifest_summary() {
  local manifest_file="$1"
  local version_var="$2"
  local git_sha_var="$3"
  local profiles_var="$4"
  local interop_var="$5"
  local managed_count_var="$6"

  [[ -f "$manifest_file" ]] || return 0

  printf -v "$version_var" '%s' "$(bubbles_json_string_field "$manifest_file" version)"
  printf -v "$git_sha_var" '%s' "$(bubbles_json_string_field "$manifest_file" gitSha)"
  printf -v "$profiles_var" '%s' "$(bubbles_json_array_joined "$manifest_file" supportedProfiles ', ')"
  printf -v "$interop_var" '%s' "$(bubbles_json_array_joined "$manifest_file" supportedInteropSources ', ')"
  printf -v "$managed_count_var" '%s' "$(bubbles_json_number_field "$manifest_file" managedFileCount)"
}

bubbles_read_install_provenance_summary() {
  local provenance_file="$1"
  local version_var="$2"
  local mode_var="$3"
  local ref_var="$4"
  local git_sha_var="$5"
  local dirty_var="$6"

  [[ -f "$provenance_file" ]] || return 0

  printf -v "$version_var" '%s' "$(bubbles_json_string_field "$provenance_file" installedVersion)"
  printf -v "$mode_var" '%s' "$(bubbles_json_string_field "$provenance_file" installMode)"
  printf -v "$ref_var" '%s' "$(bubbles_json_string_field "$provenance_file" sourceRef)"
  printf -v "$git_sha_var" '%s' "$(bubbles_json_string_field "$provenance_file" sourceGitSha)"
  printf -v "$dirty_var" '%s' "$(bubbles_json_bool_field "$provenance_file" sourceDirty)"
}

bubbles_fill_unknown_if_empty() {
  local variable_name=''

  for variable_name in "$@"; do
    if [[ -z "${!variable_name:-}" ]]; then
      printf -v "$variable_name" '%s' 'unknown'
    fi
  done
}

bubbles_framework_manifest_entries() {
  local source_root="$1"
  local include_release_manifest="${2:-false}"
  local file_path
  local relative_path
  local skill_dir
  local skill_name

  for file_path in "$source_root"/bubbles/scripts/*.sh; do
    [[ -f "$file_path" ]] || continue
    printf 'bubbles/scripts/%s\n' "$(basename "$file_path")"
  done

  while IFS= read -r file_path; do
    [[ -f "$file_path" ]] || continue
    relative_path="${file_path#$source_root/}"
    printf '%s\n' "$relative_path"
  done < <(find "$source_root/templates" -type f 2>/dev/null | LC_ALL=C sort)

  for relative_path in \
    '.specify/memory/bubbles.config.json' \
    '.specify/memory/.gitignore' \
    '.specify/metrics/.gitignore' \
    '.specify/runtime/.gitignore'; do
    [[ -f "$source_root/$relative_path" ]] && printf '%s\n' "$relative_path"
  done

  while IFS= read -r file_path; do
    [[ -f "$file_path" ]] || continue
    relative_path="${file_path#$source_root/}"
    printf '%s\n' "$relative_path"
  done < <(find "$source_root/docs" -type f | LC_ALL=C sort)

  [[ -f "$source_root/bubbles/workflows.yaml" ]] && printf '%s\n' 'bubbles/workflows.yaml'
  [[ -f "$source_root/bubbles/agnosticity-allowlist.txt" ]] && printf '%s\n' 'bubbles/agnosticity-allowlist.txt'

  for file_path in "$source_root"/bubbles/*.yaml; do
    [[ -f "$file_path" ]] || continue
    relative_path="bubbles/$(basename "$file_path")"
    [[ "$relative_path" == 'bubbles/workflows.yaml' ]] && continue
    printf '%s\n' "$relative_path"
  done

  if [[ "$include_release_manifest" == 'true' && -f "$source_root/bubbles/release-manifest.json" ]]; then
    printf '%s\n' 'bubbles/release-manifest.json'
  fi

  for file_path in "$source_root"/agents/bubbles.*.agent.md; do
    [[ -f "$file_path" ]] || continue
    printf 'agents/%s\n' "$(basename "$file_path")"
  done

  for file_path in "$source_root"/agents/bubbles_shared/*.md; do
    [[ -f "$file_path" ]] || continue
    printf 'agents/bubbles_shared/%s\n' "$(basename "$file_path")"
  done

  for file_path in "$source_root"/prompts/bubbles.*.prompt.md; do
    [[ -f "$file_path" ]] || continue
    printf 'prompts/%s\n' "$(basename "$file_path")"
  done

  for file_path in "$source_root"/instructions/bubbles-*.instructions.md; do
    [[ -f "$file_path" ]] || continue
    printf 'instructions/%s\n' "$(basename "$file_path")"
  done

  for skill_dir in "$source_root"/skills/bubbles-*/; do
    [[ -d "$skill_dir" ]] || continue
    skill_name="$(basename "$skill_dir")"
    while IFS= read -r file_path; do
      [[ -f "$file_path" ]] || continue
      relative_path="${file_path#$skill_dir}"
      printf 'skills/%s/%s\n' "$skill_name" "$relative_path"
    done < <(find "$skill_dir" -type f | LC_ALL=C sort)
  done
}

bubbles_source_bundle_clean() {
  local source_root="$1"
  local checksums_file="$source_root/bubbles/.checksums"
  local expected_sum=''
  local relative_path=''
  local target_path=''
  local actual_sum=''

  [[ -f "$checksums_file" ]] || return 1

  while IFS=$'\t' read -r expected_sum relative_path; do
    [[ -n "$expected_sum" ]] || continue
    [[ "$expected_sum" == \#* ]] && continue
    [[ -n "$relative_path" ]] || return 1

    target_path="$source_root/$relative_path"
    [[ -f "$target_path" ]] || return 1

    actual_sum="$(bubbles_sha256_file "$target_path")" || return 1
    [[ "$actual_sum" == "$expected_sum" ]] || return 1
  done < "$checksums_file"

  return 0
}

bubbles_owns_git_checkout() {
  local source_root="$1"
  local repo_root=''
  local canonical_source=''
  local canonical_repo=''

  repo_root="$({ git -C "$source_root" rev-parse --show-toplevel; } 2>/dev/null || true)"
  [[ -n "$repo_root" ]] || return 1

  canonical_source="$(cd "$source_root" && pwd -P)"
  canonical_repo="$(cd "$repo_root" && pwd -P)"
  [[ "$canonical_source" == "$canonical_repo" ]]
}

bubbles_local_source_ref() {
  local source_root="$1"
  local ref_name=''

  if bubbles_owns_git_checkout "$source_root"; then
    ref_name="$({ git -C "$source_root" symbolic-ref --quiet --short HEAD; } 2>/dev/null || true)"
    if [[ -n "$ref_name" ]]; then
      if bubbles_provenance_ref_is_safe "$ref_name"; then
        printf '%s\n' "$ref_name"
      else
        printf '%s\n' 'local-source'
      fi
      return 0
    fi

    ref_name="$({ git -C "$source_root" describe --tags --exact-match; } 2>/dev/null || true)"
    if [[ -n "$ref_name" ]]; then
      if bubbles_provenance_ref_is_safe "$ref_name"; then
        printf '%s\n' "$ref_name"
      else
        printf '%s\n' 'local-source'
      fi
      return 0
    fi
  fi

  if [[ -f "$source_root/bubbles/.install-source.json" ]]; then
    ref_name="$(bubbles_json_string_field "$source_root/bubbles/.install-source.json" sourceRef)"
    if bubbles_provenance_ref_is_safe "$ref_name"; then
      printf '%s\n' "$ref_name"
      return 0
    fi
  fi

  printf '%s\n' 'local-source'
}

bubbles_local_source_sha() {
  local source_root="$1"

  if bubbles_owns_git_checkout "$source_root" && git -C "$source_root" rev-parse HEAD >/dev/null 2>&1; then
    git -C "$source_root" rev-parse HEAD 2>/dev/null
    return 0
  fi

  if [[ -f "$source_root/bubbles/release-manifest.json" ]]; then
    bubbles_json_string_field "$source_root/bubbles/release-manifest.json" gitSha
    return 0
  fi

  return 1
}

bubbles_local_source_dirty() {
  local source_root="$1"

  if ! bubbles_owns_git_checkout "$source_root"; then
    if bubbles_source_bundle_clean "$source_root"; then
      echo 'false'
    else
      echo 'true'
    fi
    return 0
  fi

  if git -C "$source_root" diff --no-ext-diff --quiet --exit-code && \
     git -C "$source_root" diff --no-ext-diff --cached --quiet --exit-code; then
    echo 'false'
  else
    echo 'true'
  fi
}