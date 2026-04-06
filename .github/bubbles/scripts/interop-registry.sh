#!/usr/bin/env bash

bubbles_interop_registry_path() {
  local repo_root="$1"

  if [[ -f "$repo_root/.github/bubbles/interop-registry.yaml" ]]; then
    printf '%s/.github/bubbles/interop-registry.yaml\n' "$repo_root"
  else
    printf '%s/bubbles/interop-registry.yaml\n' "$repo_root"
  fi
}

bubbles_interop_source_ids() {
  local registry_file="$1"

  awk '
    /^sources:/ { in_sources=1; next }
    in_sources && /^  [a-z0-9][a-z0-9-]*:$/ {
      source=$1
      sub(":$", "", source)
      print source
    }
  ' "$registry_file"
}

bubbles_interop_scalar() {
  local registry_file="$1"
  local source_id="$2"
  local field_name="$3"

  awk -v source_id="$source_id" -v field_name="$field_name" '
    /^sources:/ { in_sources=1; next }
    in_sources && $0 == "  " source_id ":" { in_source=1; next }
    in_source && /^  [a-z0-9][a-z0-9-]*:$/ { exit }
    in_source && $0 ~ ("^    " field_name ":") {
      sub("^    " field_name ":[[:space:]]*", "", $0)
      gsub(/^"|"$/, "", $0)
      print $0
      exit
    }
  ' "$registry_file"
}

bubbles_interop_list() {
  local registry_file="$1"
  local source_id="$2"
  local field_name="$3"

  awk -v source_id="$source_id" -v field_name="$field_name" '
    /^sources:/ { in_sources=1; next }
    in_sources && $0 == "  " source_id ":" { in_source=1; next }
    in_source && /^  [a-z0-9][a-z0-9-]*:$/ { exit }
    in_source && $0 == "    " field_name ":" { in_list=1; next }
    in_list && /^    [A-Za-z0-9_-]+:/ { exit }
    in_list && /^    - / {
      sub(/^    - /, "", $0)
      gsub(/^"|"$/, "", $0)
      print $0
    }
  ' "$registry_file"
}

bubbles_interop_detect_files() {
  local scan_root="$1"
  local registry_file="$2"
  local source_id="$3"
  local detector=''
  local detector_path=''

  while IFS= read -r detector; do
    [[ -n "$detector" ]] || continue
    detector_path="$scan_root/$detector"

    if [[ "$detector" == */ ]]; then
      [[ -d "$detector_path" ]] || continue
      while IFS= read -r detected_file; do
        [[ -n "$detected_file" ]] || continue
        printf '%s\n' "${detected_file#"$scan_root"/}"
      done < <(find "$detector_path" -type f | sort)
    else
      [[ -f "$detector_path" ]] || continue
      printf '%s\n' "$detector"
    fi
  done < <(bubbles_interop_list "$registry_file" "$source_id" detectors)
}