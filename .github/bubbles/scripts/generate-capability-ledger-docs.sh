#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
  REPO_ROOT="$BUBBLES_REPO_ROOT"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

LEDGER_FILE="$REPO_ROOT/bubbles/capability-ledger.yaml"
INTEROP_REGISTRY_FILE="$REPO_ROOT/bubbles/interop-registry.yaml"
GENERATED_DIR="$REPO_ROOT/docs/generated"
README_FILE="$REPO_ROOT/README.md"

check_only=false
if [[ "${1:-}" == "--check" ]]; then
  check_only=true
fi

write_file() {
  local target_file="$1"
  local content_file="$2"
  mv "$content_file" "$target_file"
}

replace_block() {
  local target_file="$1"
  local start_marker="$2"
  local end_marker="$3"
  local content_file="$4"
  local temp_file
  temp_file="$(mktemp)"

  awk -v start_marker="$start_marker" -v end_marker="$end_marker" -v content_file="$content_file" '
    BEGIN {
      in_block = 0
      replaced = 0
    }
    index($0, start_marker) {
      print
      while ((getline line < content_file) > 0) {
        print line
      }
      close(content_file)
      in_block = 1
      replaced = 1
      next
    }
    index($0, end_marker) {
      in_block = 0
      print
      next
    }
    !in_block {
      print
    }
    END {
      if (!replaced) {
        exit 2
      }
    }
  ' "$target_file" > "$temp_file"

  mv "$temp_file" "$target_file"
}

relative_doc_link() {
  local path="$1"

  case "$path" in
    README.md)
      printf '%s' '../../README.md'
      ;;
    docs/*)
      printf '%s' "../${path#docs/}"
      ;;
    *)
      printf '%s' "../../$path"
      ;;
  esac
}

format_doc_list() {
  local list_blob="$1"
  local output=""
  local IFS=$'\034'
  local item

  for item in $list_blob; do
    [[ -n "$item" ]] || continue
    if [[ -n "$output" ]]; then
      output+="; "
    fi
    output+="[$item]($(relative_doc_link "$item"))"
  done

  if [[ -z "$output" ]]; then
    output='—'
  fi

  printf '%s' "$output"
}

format_inline_list() {
  local list_blob="$1"
  local output=""
  local IFS=$'\034'
  local item

  for item in $list_blob; do
    [[ -n "$item" ]] || continue
    if [[ -n "$output" ]]; then
      output+=', '
    fi
    output+="$item"
  done

  if [[ -z "$output" ]]; then
    output='—'
  fi

  printf '%s' "$output"
}

format_issue_list() {
  local list_blob="$1"
  local output=""
  local IFS=$'\034'
  local item
  local label

  for item in $list_blob; do
    [[ -n "$item" ]] || continue
    label="${item#docs/issues/}"
    if [[ -n "$output" ]]; then
      output+="; "
    fi
    output+="[$label]($(relative_doc_link "$item"))"
  done

  if [[ -z "$output" ]]; then
    output='—'
  fi

  printf '%s' "$output"
}

state_count() {
  local target_state="$1"
  local count=0
  local line
  while IFS=$'\t' read -r kind _ _ state _; do
    [[ "$kind" == "CAP" ]] || continue
    if [[ "$state" == "$target_state" ]]; then
      count=$((count + 1))
    fi
  done < "$1"
  printf '%s' "$count"
}

mkdir -p "$GENERATED_DIR"

[[ -f "$LEDGER_FILE" ]] || {
  echo "Missing capability ledger: $LEDGER_FILE" >&2
  exit 1
}
[[ -f "$INTEROP_REGISTRY_FILE" ]] || {
  echo "Missing interop registry: $INTEROP_REGISTRY_FILE" >&2
  exit 1
}

records_file="$(mktemp)"
awk '
function trim(value) {
  sub(/^[[:space:]]+/, "", value)
  sub(/[[:space:]]+$/, "", value)
  return value
}
function append_value(target_name, key, value, composite) {
  if (value == "") {
    return
  }
  composite = key SUBSEP target_name
  if (lists[composite] == "") {
    lists[composite] = value
  } else {
    lists[composite] = lists[composite] RS_SEP value
  }
}
function flush_capability(composite) {
  if (cap_id == "") {
    return
  }
  print "CAP", cap_id, fields[cap_id SUBSEP "label"], fields[cap_id SUBSEP "state"], fields[cap_id SUBSEP "summary"], fields[cap_id SUBSEP "ownerSurface"], fields[cap_id SUBSEP "releaseIntroduced"], lists[cap_id SUBSEP "competitorTags"], lists[cap_id SUBSEP "docsRefs"], lists[cap_id SUBSEP "evidenceRefs"], lists[cap_id SUBSEP "issueRefs"]
}
BEGIN {
  FS = "\t"
  OFS = "\t"
  RS_SEP = "\034"
  section = ""
  cap_id = ""
  list_field = ""
}
/^capabilities:[[:space:]]*$/ {
  section = "capabilities"
  next
}
section == "capabilities" && match($0, /^  ([a-z0-9-]+):[[:space:]]*$/, match_arr) {
  flush_capability()
  cap_id = match_arr[1]
  list_field = ""
  next
}
section == "capabilities" && cap_id != "" && match($0, /^    ([A-Za-z]+):[[:space:]]*(.*)$/, match_arr) {
  field = match_arr[1]
  value = trim(match_arr[2])
  if (field == "docsRefs" || field == "evidenceRefs" || field == "competitorTags" || field == "issueRefs" || field == "freshnessTargets") {
    list_field = field
    next
  }
  fields[cap_id SUBSEP field] = value
  list_field = ""
  next
}
section == "capabilities" && cap_id != "" && list_field != "" && match($0, /^    -[[:space:]]*(.*)$/, match_arr) {
  append_value(list_field, cap_id, trim(match_arr[1]))
  next
}
END {
  flush_capability()
}
' "$LEDGER_FILE" > "$records_file"

shipped_count=0
partial_count=0
proposed_count=0
issue_backed_count=0

while IFS=$'\t' read -r kind _ _ state _ _ _ _ _ _ issue_refs; do
  [[ "$kind" == "CAP" ]] || continue
  case "$state" in
    shipped)
      shipped_count=$((shipped_count + 1))
      ;;
    partial)
      partial_count=$((partial_count + 1))
      ;;
    proposed)
      proposed_count=$((proposed_count + 1))
      ;;
  esac
  if [[ -n "$issue_refs" ]]; then
    issue_backed_count=$((issue_backed_count + 1))
  fi
done < "$records_file"

competitive_doc_temp="$(mktemp)"
{
  echo '# Competitive Capabilities'
  echo
  printf 'State summary: %s shipped, %s partial, %s proposed.\n' "$shipped_count" "$partial_count" "$proposed_count"
  echo
  echo 'This page is generated from `bubbles/capability-ledger.yaml` and is the source-backed view of how Bubbles positions key framework capabilities against adjacent tools.'
  echo
  echo '| Capability | State | Summary | Competitor Pressure | Docs | Evidence | Issues |'
  echo '| --- | --- | --- | --- | --- | --- | --- |'

  while IFS=$'\t' read -r kind cap_id label state summary owner_surface release_introduced competitor_tags docs_refs evidence_refs issue_refs; do
    [[ "$kind" == "CAP" ]] || continue
    printf '| %s | %s | %s | %s | %s | `%s`' \
      "$label" \
      "$state" \
      "$summary" \
      "$(format_inline_list "$competitor_tags")" \
      "$(format_doc_list "$docs_refs")" \
      "$owner_surface"
    if [[ -n "$evidence_refs" ]]; then
      printf '<br>%s' "$(format_inline_list "$evidence_refs")"
    fi
    printf ' | %s |\n' "$(format_issue_list "$issue_refs")"
  done < "$records_file"
} > "$competitive_doc_temp"

issue_doc_temp="$(mktemp)"
{
  echo '# Issue Status'
  echo
  printf 'Tracked gaps: %s issue-backed capabilities.\n' "$issue_backed_count"
  echo
  echo '| Issue | Ledger Status | Related Capability | Summary |'
  echo '| --- | --- | --- | --- |'

  while IFS=$'\t' read -r kind cap_id label state summary _ _ _ _ _ issue_refs; do
    [[ "$kind" == "CAP" ]] || continue
    [[ -n "$issue_refs" ]] || continue
    IFS=$'\034' read -r -a issue_paths <<< "$issue_refs"
    for issue_path in "${issue_paths[@]}"; do
      [[ -n "$issue_path" ]] || continue
      issue_label="${issue_path#docs/issues/}"
      printf '| [%s](%s) | %s | %s | %s |\n' \
        "$issue_label" \
        "$(relative_doc_link "$issue_path")" \
        "$state" \
        "$label" \
        "$summary"
    done
  done < "$records_file"
} > "$issue_doc_temp"

interop_records_file="$(mktemp)"
awk '
function trim(value) {
  sub(/^[[:space:]]+/, "", value)
  sub(/[[:space:]]+$/, "", value)
  return value
}
function append_list(source_id, field, value, composite) {
  if (value == "") {
    return
  }
  composite = source_id SUBSEP field
  if (lists[composite] == "") {
    lists[composite] = value
  } else {
    lists[composite] = lists[composite] RS_SEP value
  }
}
function flush_source() {
  if (source_id == "") {
    return
  }
  print "SRC", source_id, fields[source_id SUBSEP "displayName"], fields[source_id SUBSEP "parserKind"], lists[source_id SUBSEP "detectors"], fields[source_id SUBSEP "applyTargetInstruction"], fields[source_id SUBSEP "applyTargetGuardrail"], fields[source_id SUBSEP "applyTargetCommandSurface"], fields[source_id SUBSEP "applyTargetToolRequest"], fields[source_id SUBSEP "applyTargetDocsReference"], lists[source_id SUBSEP "unsupportedTargets"]
}
BEGIN {
  OFS = "\t"
  RS_SEP = "\034"
  source_id = ""
  list_field = ""
}
/^sources:[[:space:]]*$/ {
  in_sources = 1
  next
}
in_sources && match($0, /^  ([a-z0-9-]+):[[:space:]]*$/, match_arr) {
  flush_source()
  source_id = match_arr[1]
  list_field = ""
  next
}
in_sources && source_id != "" && match($0, /^    ([A-Za-z]+):[[:space:]]*(.*)$/, match_arr) {
  field = match_arr[1]
  value = trim(match_arr[2])
  if (field == "detectors" || field == "normalizedClasses" || field == "supportedTargets" || field == "unsupportedTargets") {
    list_field = field
    next
  }
  fields[source_id SUBSEP field] = value
  list_field = ""
  next
}
in_sources && source_id != "" && list_field != "" && match($0, /^    -[[:space:]]*(.*)$/, match_arr) {
  append_list(source_id, list_field, trim(match_arr[1]))
  next
}
END {
  flush_source()
}
' "$INTEROP_REGISTRY_FILE" > "$interop_records_file"

interop_doc_temp="$(mktemp)"
{
  echo '# Interop Migration Matrix'
  echo
  printf 'Capability context: %s shipped, %s partial, %s proposed.\n' "$shipped_count" "$partial_count" "$proposed_count"
  echo
  echo 'This page is generated from `bubbles/capability-ledger.yaml` and `bubbles/interop-registry.yaml` so evaluators can compare supported apply, review-only intake, and proposal-only boundaries without relying on hand-maintained competitor prose.'
  echo
  echo '| Source | Parser | Review-Only Intake | Supported Apply Targets | Proposal-Only / Unsupported |'
  echo '| --- | --- | --- | --- | --- |'

  while IFS=$'\t' read -r kind source_id display_name parser_kind detectors apply_instruction apply_guardrail apply_command_surface apply_tool_request apply_docs_reference unsupported_targets; do
    [[ "$kind" == 'SRC' ]] || continue
    apply_targets=''
    for apply_target in "$apply_instruction" "$apply_guardrail" "$apply_command_surface" "$apply_tool_request" "$apply_docs_reference"; do
      [[ -n "$apply_target" ]] || continue
      if [[ -n "$apply_targets" ]]; then
        apply_targets+=$'\034'
      fi
      apply_targets+="$apply_target"
    done

    printf '| %s | %s | %s | %s | %s |\n' \
      "$display_name" \
      "$parser_kind" \
      "$(format_inline_list "$detectors")" \
      "$(format_inline_list "$apply_targets")" \
      "$(format_inline_list "$unsupported_targets")"
  done < "$interop_records_file"
} > "$interop_doc_temp"

readme_block_temp="$(mktemp)"
{
  printf '| [Competitive Capabilities](docs/generated/competitive-capabilities.md) | Ledger-backed competitive posture guide — %s shipped, %s partial, %s proposed |\n' "$shipped_count" "$partial_count" "$proposed_count"
  printf '| [Issue Status](docs/generated/issue-status.md) | Ledger-backed status for %s tracked framework gaps and proposals |\n' "$issue_backed_count"
  echo '| [Interop Migration Matrix](docs/generated/interop-migration-matrix.md) | Ledger + registry-backed migration matrix for Claude Code, Roo Code, Cursor, and Cline |'
} > "$readme_block_temp"

apply_block_to_copy() {
  local source_file="$1"
  local start_marker="$2"
  local end_marker="$3"
  local block_file="$4"
  local temp_copy
  temp_copy="$(mktemp)"
  cp "$source_file" "$temp_copy"
  replace_block "$temp_copy" "$start_marker" "$end_marker" "$block_file"
  printf '%s' "$temp_copy"
}

render_issue_block_file() {
  local label="$1"
  local state="$2"
  local competitor_tags="$3"
  local temp_file
  temp_file="$(mktemp)"
  {
    printf '**Ledger Status:** %s\n' "$state"
    printf '**Related Capability:** %s\n' "$label"
    printf '**Competitive Pressure:** %s\n' "$(format_inline_list "$competitor_tags")"
    printf '**Source Of Truth:** [Issue Status](../generated/issue-status.md)\n'
  } > "$temp_file"
  printf '%s' "$temp_file"
}

if [[ "$check_only" == true ]]; then
  [[ -f "$GENERATED_DIR/competitive-capabilities.md" ]] || {
    echo "Missing generated competitive capabilities doc. Run bubbles/scripts/generate-capability-ledger-docs.sh" >&2
    exit 1
  }
  [[ -f "$GENERATED_DIR/issue-status.md" ]] || {
    echo "Missing generated issue status doc. Run bubbles/scripts/generate-capability-ledger-docs.sh" >&2
    exit 1
  }
  [[ -f "$GENERATED_DIR/interop-migration-matrix.md" ]] || {
    echo "Missing generated interop migration matrix doc. Run bubbles/scripts/generate-capability-ledger-docs.sh" >&2
    exit 1
  }

  cmp -s "$competitive_doc_temp" "$GENERATED_DIR/competitive-capabilities.md" || {
    echo "Generated competitive capabilities doc is stale. Run bubbles/scripts/generate-capability-ledger-docs.sh" >&2
    exit 1
  }
  cmp -s "$issue_doc_temp" "$GENERATED_DIR/issue-status.md" || {
    echo "Generated issue status doc is stale. Run bubbles/scripts/generate-capability-ledger-docs.sh" >&2
    exit 1
  }
  cmp -s "$interop_doc_temp" "$GENERATED_DIR/interop-migration-matrix.md" || {
    echo "Generated interop migration matrix doc is stale. Run bubbles/scripts/generate-capability-ledger-docs.sh" >&2
    exit 1
  }

  readme_copy="$(apply_block_to_copy "$README_FILE" "GENERATED:CAPABILITY_LEDGER_DOCS_ROW_START" "GENERATED:CAPABILITY_LEDGER_DOCS_ROW_END" "$readme_block_temp")"
  cmp -s "$readme_copy" "$README_FILE" || {
    echo "README capability-ledger block is stale. Run bubbles/scripts/generate-capability-ledger-docs.sh" >&2
    exit 1
  }

  while IFS=$'\t' read -r kind _ label state _ _ _ competitor_tags _ _ issue_refs; do
    [[ "$kind" == "CAP" ]] || continue
    [[ -n "$issue_refs" ]] || continue
    IFS=$'\034' read -r -a issue_paths <<< "$issue_refs"
    for issue_path in "${issue_paths[@]}"; do
      [[ -n "$issue_path" ]] || continue
      block_file="$(render_issue_block_file "$label" "$state" "$competitor_tags")"
      issue_copy="$(apply_block_to_copy "$REPO_ROOT/$issue_path" "GENERATED:CAPABILITY_LEDGER_STATUS_START" "GENERATED:CAPABILITY_LEDGER_STATUS_END" "$block_file")"
      cmp -s "$issue_copy" "$REPO_ROOT/$issue_path" || {
        echo "Issue capability-ledger block is stale for $issue_path. Run bubbles/scripts/generate-capability-ledger-docs.sh" >&2
        exit 1
      }
    done
  done < "$records_file"

  printf '%s\n' "Capability ledger docs are current: $shipped_count shipped, $partial_count partial, $proposed_count proposed"
  exit 0
fi

write_file "$GENERATED_DIR/competitive-capabilities.md" "$competitive_doc_temp"
write_file "$GENERATED_DIR/issue-status.md" "$issue_doc_temp"
write_file "$GENERATED_DIR/interop-migration-matrix.md" "$interop_doc_temp"
replace_block "$README_FILE" "GENERATED:CAPABILITY_LEDGER_DOCS_ROW_START" "GENERATED:CAPABILITY_LEDGER_DOCS_ROW_END" "$readme_block_temp"

while IFS=$'\t' read -r kind _ label state _ _ _ competitor_tags _ _ issue_refs; do
  [[ "$kind" == "CAP" ]] || continue
  [[ -n "$issue_refs" ]] || continue
  IFS=$'\034' read -r -a issue_paths <<< "$issue_refs"
  for issue_path in "${issue_paths[@]}"; do
    [[ -n "$issue_path" ]] || continue
    block_file="$(render_issue_block_file "$label" "$state" "$competitor_tags")"
    replace_block "$REPO_ROOT/$issue_path" "GENERATED:CAPABILITY_LEDGER_STATUS_START" "GENERATED:CAPABILITY_LEDGER_STATUS_END" "$block_file"
  done
done < "$records_file"

printf '%s\n' "Updated capability ledger docs: $shipped_count shipped, $partial_count partial, $proposed_count proposed"