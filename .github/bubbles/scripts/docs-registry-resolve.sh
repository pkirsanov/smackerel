#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ "$SCRIPT_DIR" == *"/.github/bubbles/scripts" ]]; then
  PROJECT_ROOT="${SCRIPT_DIR%/.github/bubbles/scripts}"
  FRAMEWORK_ROOT="$PROJECT_ROOT/.github/bubbles"
else
  PROJECT_ROOT="${SCRIPT_DIR%/bubbles/scripts}"
  FRAMEWORK_ROOT="$PROJECT_ROOT/bubbles"
fi

FRAMEWORK_REGISTRY="$FRAMEWORK_ROOT/docs-registry.yaml"
PROJECT_CONFIG="$PROJECT_ROOT/.github/bubbles-project.yaml"
MODE="effective"
PATHS_ONLY="false"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/docs-registry-resolve.sh [--effective|--framework-default] [--paths-only]

Resolves the effective Bubbles managed-doc registry.

Modes:
  --effective         Merge framework defaults with project-owned docsRegistryOverrides from .github/bubbles-project.yaml (default)
  --framework-default Output the framework default registry only

Options:
  --paths-only        Output only managed doc keys and resolved paths
  --help              Show this help message
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --effective)
      MODE="effective"
      shift
      ;;
    --framework-default|--default)
      MODE="framework-default"
      shift
      ;;
    --paths-only)
      PATHS_ONLY="true"
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[[ -f "$FRAMEWORK_REGISTRY" ]] || {
  echo "ERROR: missing framework docs registry: $FRAMEWORK_REGISTRY" >&2
  exit 2
}

awk -v framework_registry="$FRAMEWORK_REGISTRY" -v project_config="$PROJECT_CONFIG" -v mode="$MODE" -v paths_only="$PATHS_ONLY" '
function trim(value) {
  sub(/^[[:space:]]+/, "", value)
  sub(/[[:space:]]+$/, "", value)
  return value
}

function append_list_value(target, key, field, value, composite) {
  composite = key SUBSEP field
  if (value == "") {
    return
  }
  if (target[composite] == "") {
    target[composite] = value
  } else {
    target[composite] = target[composite] RS_SEP value
  }
}

function note_doc_key(target_seen, target_order, key,    idx) {
  if (!(key in target_seen)) {
    target_seen[key] = 1
    target_order[++target_order[0]] = key
  }
}

function set_doc_field(target, key, field, value) {
  target[key SUBSEP field] = trim(value)
}

function parse_framework_registry(file,    line, section, doc_key, list_field, match_arr, field, value) {
  section = ""
  doc_key = ""
  list_field = ""

  while ((getline line < file) > 0) {
    if (line ~ /^[[:space:]]*#/ || trim(line) == "") {
      continue
    }

    if (match(line, /^version:[[:space:]]*(.*)$/, match_arr)) {
      version = trim(match_arr[1])
      continue
    }

    if (line ~ /^managedDocs:[[:space:]]*$/) {
      section = "managedDocs"
      doc_key = ""
      list_field = ""
      continue
    }
    if (line ~ /^policy:[[:space:]]*$/) {
      section = "policy"
      doc_key = ""
      list_field = ""
      continue
    }
    if (line ~ /^classification:[[:space:]]*$/) {
      section = "classification"
      doc_key = ""
      list_field = ""
      continue
    }

    if (section == "managedDocs") {
      if (match(line, /^  ([A-Za-z0-9_-]+):[[:space:]]*$/, match_arr)) {
        doc_key = match_arr[1]
        note_doc_key(default_doc_seen, default_doc_order, doc_key)
        list_field = ""
        continue
      }

      if (doc_key != "" && match(line, /^    ([A-Za-zA-Z]+):[[:space:]]*(.*)$/, match_arr)) {
        field = match_arr[1]
        value = trim(match_arr[2])
        if (field == "requiredSections") {
          list_field = field
          delete default_doc_list[doc_key SUBSEP field]
        } else {
          set_doc_field(default_doc_fields, doc_key, field, value)
          list_field = ""
        }
        continue
      }

      if (doc_key != "" && list_field != "" && match(line, /^      -[[:space:]]*(.*)$/, match_arr)) {
        append_list_value(default_doc_list, doc_key, list_field, trim(match_arr[1]))
        continue
      }
    }

    if (section == "policy" && match(line, /^  ([A-Za-zA-Z0-9_-]+):[[:space:]]*(.*)$/, match_arr)) {
      default_policy[match_arr[1]] = trim(match_arr[2])
      continue
    }

    if (section == "classification" && match(line, /^  ([A-Za-zA-Z0-9_-]+):[[:space:]]*(.*)$/, match_arr)) {
      default_classification[match_arr[1]] = trim(match_arr[2])
      continue
    }
  }

  close(file)
}

function parse_project_overrides(file,    line, section, subsection, doc_key, list_field, match_arr, field, value) {
  if (mode != "effective") {
    return
  }
  if ((getline line < file) < 0) {
    close(file)
    return
  }
  close(file)

  section = ""
  subsection = ""
  doc_key = ""
  list_field = ""

  while ((getline line < file) > 0) {
    if (line ~ /^[[:space:]]*#/ || trim(line) == "") {
      continue
    }

    if (line ~ /^docsRegistryOverrides:[[:space:]]*$/) {
      section = "docsRegistryOverrides"
      subsection = ""
      doc_key = ""
      list_field = ""
      continue
    }

    if (section == "docsRegistryOverrides" && line ~ /^[^[:space:]][^:]*:[[:space:]]*$/) {
      section = ""
      subsection = ""
      doc_key = ""
      list_field = ""
    }

    if (section != "docsRegistryOverrides") {
      continue
    }

    if (match(line, /^  (managedDocs|policy|classification):[[:space:]]*$/, match_arr)) {
      subsection = match_arr[1]
      doc_key = ""
      list_field = ""
      continue
    }

    if (subsection == "managedDocs") {
      if (match(line, /^    ([A-Za-z0-9_-]+):[[:space:]]*$/, match_arr)) {
        doc_key = match_arr[1]
        note_doc_key(override_doc_seen, override_doc_order, doc_key)
        list_field = ""
        continue
      }

      if (doc_key != "" && match(line, /^      ([A-Za-zA-Z]+):[[:space:]]*(.*)$/, match_arr)) {
        field = match_arr[1]
        value = trim(match_arr[2])
        if (field == "requiredSections") {
          list_field = field
          delete override_doc_list[doc_key SUBSEP field]
        } else {
          set_doc_field(override_doc_fields, doc_key, field, value)
          list_field = ""
        }
        continue
      }

      if (doc_key != "" && list_field != "" && match(line, /^        -[[:space:]]*(.*)$/, match_arr)) {
        append_list_value(override_doc_list, doc_key, list_field, trim(match_arr[1]))
        continue
      }
    }

    if (subsection == "policy" && match(line, /^    ([A-Za-zA-Z0-9_-]+):[[:space:]]*(.*)$/, match_arr)) {
      override_policy[match_arr[1]] = trim(match_arr[2])
      continue
    }

    if (subsection == "classification" && match(line, /^    ([A-Za-zA-Z0-9_-]+):[[:space:]]*(.*)$/, match_arr)) {
      override_classification[match_arr[1]] = trim(match_arr[2])
      continue
    }
  }

  close(file)
}

function resolved_doc_value(key, field, composite) {
  composite = key SUBSEP field
  if (composite in override_doc_fields) {
    return override_doc_fields[composite]
  }
  if (composite in default_doc_fields) {
    return default_doc_fields[composite]
  }
  return ""
}

function resolved_doc_list_value(key, field, composite) {
  composite = key SUBSEP field
  if (composite in override_doc_list) {
    return override_doc_list[composite]
  }
  if (composite in default_doc_list) {
    return default_doc_list[composite]
  }
  return ""
}

function resolved_policy_value(key) {
  if (key in override_policy) {
    return override_policy[key]
  }
  return default_policy[key]
}

function resolved_classification_value(key) {
  if (key in override_classification) {
    return override_classification[key]
  }
  return default_classification[key]
}

function emit_required_sections(list_blob, sections, idx, count) {
  count = split(list_blob, sections, RS_SEP)
  for (idx = 1; idx <= count; idx++) {
    if (sections[idx] != "") {
      print "      - " sections[idx]
    }
  }
}

BEGIN {
  RS_SEP = "\034"
  version = "1"

  parse_framework_registry(framework_registry)
  if (mode == "effective" && project_config != "" && system("test -f \"" project_config "\"") == 0) {
    parse_project_overrides(project_config)
  }

  default_key_count = default_doc_order[0]
  override_key_count = override_doc_order[0]

  if (paths_only == "true") {
    for (idx = 1; idx <= default_key_count; idx++) {
      key = default_doc_order[idx]
      seen_paths[key] = 1
      print key ": " resolved_doc_value(key, "path")
    }
    for (idx = 1; idx <= override_key_count; idx++) {
      key = override_doc_order[idx]
      if (!(key in seen_paths)) {
        print key ": " resolved_doc_value(key, "path")
      }
    }
    exit 0
  }

  print "version: " version
  print ""
  print "managedDocs:"

  for (idx = 1; idx <= default_key_count; idx++) {
    key = default_doc_order[idx]
    emitted_docs[key] = 1
    print "  " key ":"

    path_value = resolved_doc_value(key, "path")
    owner_value = resolved_doc_value(key, "owner")
    required_value = resolved_doc_value(key, "required")
    audience_value = resolved_doc_value(key, "audience")
    publish_sources_value = resolved_doc_value(key, "publishSources")
    required_sections_value = resolved_doc_list_value(key, "requiredSections")

    if (path_value != "") print "    path: " path_value
    if (owner_value == "") owner_value = "bubbles.docs"
    print "    owner: " owner_value
    if (required_value != "") print "    required: " required_value
    if (audience_value != "") print "    audience: " audience_value
    if (publish_sources_value != "") print "    publishSources: " publish_sources_value
    if (required_sections_value != "") {
      print "    requiredSections:"
      emit_required_sections(required_sections_value)
    }
  }

  for (idx = 1; idx <= override_key_count; idx++) {
    key = override_doc_order[idx]
    if (key in emitted_docs) {
      continue
    }

    print "  " key ":"
    path_value = resolved_doc_value(key, "path")
    owner_value = resolved_doc_value(key, "owner")
    required_value = resolved_doc_value(key, "required")
    audience_value = resolved_doc_value(key, "audience")
    publish_sources_value = resolved_doc_value(key, "publishSources")
    required_sections_value = resolved_doc_list_value(key, "requiredSections")

    if (path_value != "") print "    path: " path_value
    if (owner_value == "") owner_value = "bubbles.docs"
    print "    owner: " owner_value
    if (required_value != "") print "    required: " required_value
    if (audience_value != "") print "    audience: " audience_value
    if (publish_sources_value != "") print "    publishSources: " publish_sources_value
    if (required_sections_value != "") {
      print "    requiredSections:"
      emit_required_sections(required_sections_value)
    }
  }

  print ""
  print "policy:"
  for (key in default_policy) {
    policy_seen[key] = 1
  }
  for (key in override_policy) {
    policy_seen[key] = 1
  }
  policy_order[1] = "publishManagedDocsOnCloseout"
  policy_order[2] = "removeObsoleteContent"
  policy_order[3] = "mergeDuplicateContentIntoManagedDocs"
  policy_order[4] = "unmanagedDocsRequireExplicitTarget"
  for (idx = 1; idx <= 4; idx++) {
    key = policy_order[idx]
    if (key in policy_seen) {
      print "  " key ": " resolved_policy_value(key)
      emitted_policy[key] = 1
    }
  }
  for (key in policy_seen) {
    if (!(key in emitted_policy)) {
      print "  " key ": " resolved_policy_value(key)
    }
  }

  print ""
  print "classification:"
  classification_order[1] = "featureRoot"
  classification_order[2] = "featurePattern"
  classification_order[3] = "opsRoot"
  classification_order[4] = "opsPattern"
  classification_order[5] = "crossCuttingBugRoot"
  classification_order[6] = "crossCuttingBugPattern"
  classification_order[7] = "featureBugPattern"
  for (key in default_classification) {
    classification_seen[key] = 1
  }
  for (key in override_classification) {
    classification_seen[key] = 1
  }
  for (idx = 1; idx <= 7; idx++) {
    key = classification_order[idx]
    if (key in classification_seen) {
      print "  " key ": " resolved_classification_value(key)
      emitted_classification[key] = 1
    }
  }
  for (key in classification_seen) {
    if (!(key in emitted_classification)) {
      print "  " key ": " resolved_classification_value(key)
    }
  }
}
' /dev/null