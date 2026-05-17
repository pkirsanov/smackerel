#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

CONFIG_FILE=''
if [[ -f "$REPO_ROOT/.github/bubbles-project.yaml" ]]; then
  CONFIG_FILE="$REPO_ROOT/.github/bubbles-project.yaml"
elif [[ -f "$REPO_ROOT/bubbles-project.yaml" ]]; then
  CONFIG_FILE="$REPO_ROOT/bubbles-project.yaml"
fi

FORMAT='text'
REQUIRE_CONFIG='false'
CHANGED_LIST_FILE=''
CHANGED_FROM=''
CHANGED_TO='HEAD'
CHANGED_FILES=()

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/test-impact-plan.sh [options] [changed-file ...]

Build an impact-aware validation plan from a project-owned testImpact map.

Options:
  --config PATH             Use an explicit bubbles-project.yaml file
  --repo-root PATH          Use an explicit repository root
  --changed-file-list PATH  Read changed files from a newline-delimited file
  --changed-from REF        Read changed files with git diff --name-only REF CHANGED_TO
  --changed-to REF          Target ref for --changed-from (default: HEAD)
  --format text|json        Output format (default: text)
  --require-config          Fail if no testImpact map is configured
  --help                    Show this help

Supported YAML shape:

testImpact:
  alwaysRun:
    - smoke
  fullSuiteTriggers:
    - "proto/**"
  components:
    api:
      paths:
        - "backend/api/**"
      testCategories:
        - unit
        - integration
      alwaysRun:
        - contract
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --config)
      CONFIG_FILE="$2"
      shift 2
      ;;
    --repo-root)
      REPO_ROOT="$2"
      shift 2
      ;;
    --changed-file-list)
      CHANGED_LIST_FILE="$2"
      shift 2
      ;;
    --changed-from)
      CHANGED_FROM="$2"
      shift 2
      ;;
    --changed-to)
      CHANGED_TO="$2"
      shift 2
      ;;
    --format)
      FORMAT="$2"
      shift 2
      ;;
    --require-config)
      REQUIRE_CONFIG='true'
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --*)
      echo "Unknown option: $1" >&2
      exit 2
      ;;
    *)
      CHANGED_FILES+=("$1")
      shift
      ;;
  esac
done

case "$FORMAT" in
  text|json) ;;
  *)
    echo "Unsupported --format value: $FORMAT" >&2
    exit 2
    ;;
esac

trim_yaml_value() {
  local value="$1"
  value="${value%%#*}"
  value="${value#${value%%[![:space:]]*}}"
  value="${value%${value##*[![:space:]]}}"
  value="${value%\"}"
  value="${value#\"}"
  value="${value%\'}"
  value="${value#\'}"
  printf '%s' "$value"
}

append_assoc() {
  local -n target_map="$1"
  local key="$2"
  local value="$3"

  [[ -n "$value" ]] || return 0
  target_map["$key"]+="$value"$'\n'
}

append_unique() {
  local value="$1"
  shift
  local -n target_array="$1"
  shift
  local -n seen_map="$1"

  [[ -n "$value" ]] || return 0
  if [[ -z "${seen_map[$value]:-}" ]]; then
    target_array+=("$value")
    seen_map["$value"]=1
  fi
}

path_matches_pattern() {
  local path_value="$1"
  local pattern_value="$2"

  [[ -n "$pattern_value" ]] || return 1
  [[ "$path_value" == $pattern_value ]]
}

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  printf '%s' "$value"
}

json_array() {
  local first='true'
  local item=''
  printf '['
  for item in "$@"; do
    [[ "$first" == 'false' ]] && printf ', '
    printf '"%s"' "$(json_escape "$item")"
    first='false'
  done
  printf ']'
}

if [[ -n "$CHANGED_LIST_FILE" ]]; then
  [[ -f "$CHANGED_LIST_FILE" ]] || {
    echo "Changed-file list does not exist: $CHANGED_LIST_FILE" >&2
    exit 2
  }
  while IFS= read -r changed_file; do
    [[ -n "$changed_file" ]] || continue
    CHANGED_FILES+=("$changed_file")
  done < "$CHANGED_LIST_FILE"
fi

if [[ -n "$CHANGED_FROM" ]]; then
  mapfile -t diff_files < <(git -C "$REPO_ROOT" diff --name-only "$CHANGED_FROM" "$CHANGED_TO")
  CHANGED_FILES+=("${diff_files[@]}")
fi

if [[ "${#CHANGED_FILES[@]}" -eq 0 ]]; then
  echo "No changed files supplied. Use --changed-file-list, --changed-from, or positional changed-file arguments." >&2
  exit 2
fi

if [[ -z "$CONFIG_FILE" || ! -f "$CONFIG_FILE" ]]; then
  if [[ "$REQUIRE_CONFIG" == 'true' ]]; then
    echo "No bubbles-project.yaml testImpact map found." >&2
    exit 1
  fi
  if [[ "$FORMAT" == 'json' ]]; then
    printf '{"configured": false, "changedFiles": '
    json_array "${CHANGED_FILES[@]}"
    printf ', "matchedComponents": [], "testCategories": [], "alwaysRun": [], "fullSuiteRequired": false}\n'
  else
    echo "Test Impact Plan"
    echo "Configured: false"
    echo "Changed files:"
    for changed_file in "${CHANGED_FILES[@]}"; do
      echo "- $changed_file"
    done
    echo "No testImpact map configured; run the repo's normal required validation set."
  fi
  exit 0
fi

if ! grep -q '^testImpact:' "$CONFIG_FILE"; then
  if [[ "$REQUIRE_CONFIG" == 'true' ]]; then
    echo "Configured file has no top-level testImpact map: $CONFIG_FILE" >&2
    exit 1
  fi
  if [[ "$FORMAT" == 'json' ]]; then
    printf '{"configured": false, "config": "%s", "changedFiles": ' "$(json_escape "$CONFIG_FILE")"
    json_array "${CHANGED_FILES[@]}"
    printf ', "matchedComponents": [], "testCategories": [], "alwaysRun": [], "fullSuiteRequired": false}\n'
  else
    echo "Test Impact Plan"
    echo "Configured: false"
    echo "Config: $CONFIG_FILE"
    echo "Changed files:"
    for changed_file in "${CHANGED_FILES[@]}"; do
      echo "- $changed_file"
    done
    echo "No testImpact map configured; run the repo's normal required validation set."
  fi
  exit 0
fi

declare -a GLOBAL_ALWAYS=()
declare -a GLOBAL_FULL_TRIGGERS=()
declare -a COMPONENTS=()
declare -A COMPONENT_SEEN=()
declare -A COMPONENT_PATHS=()
declare -A COMPONENT_CATEGORIES=()
declare -A COMPONENT_ALWAYS=()

in_test_impact='false'
in_components='false'
current_component=''
current_list=''

while IFS= read -r raw_line; do
  line="${raw_line%%$'\r'}"
  [[ -z "$(trim_yaml_value "$line")" ]] && continue
  [[ "$(trim_yaml_value "$line")" == \#* ]] && continue

  if [[ "$line" =~ ^[^[:space:]][A-Za-z0-9_-]+: ]]; then
    if [[ "$line" == testImpact:* ]]; then
      in_test_impact='true'
      in_components='false'
      current_component=''
      current_list=''
      continue
    fi

    if [[ "$in_test_impact" == 'true' ]]; then
      break
    fi
  fi

  [[ "$in_test_impact" == 'true' ]] || continue

  if [[ "$line" =~ ^[[:space:]]{2}components: ]]; then
    in_components='true'
    current_component=''
    current_list=''
    continue
  fi

  if [[ "$in_components" == 'false' ]]; then
    if [[ "$line" =~ ^[[:space:]]{2}alwaysRun: ]]; then
      current_list='global_always'
      continue
    fi
    if [[ "$line" =~ ^[[:space:]]{2}fullSuiteTriggers: ]]; then
      current_list='global_full'
      continue
    fi
  fi

  if [[ "$in_components" == 'true' && "$line" =~ ^[[:space:]]{4}([A-Za-z0-9_.-]+):[[:space:]]*$ ]]; then
    current_component="${BASH_REMATCH[1]}"
    if [[ -z "${COMPONENT_SEEN[$current_component]:-}" ]]; then
      COMPONENTS+=("$current_component")
      COMPONENT_SEEN["$current_component"]=1
    fi
    current_list=''
    continue
  fi

  if [[ -n "$current_component" ]]; then
    if [[ "$line" =~ ^[[:space:]]{6}paths: ]]; then
      current_list='component_paths'
      continue
    fi
    if [[ "$line" =~ ^[[:space:]]{6}testCategories: ]]; then
      current_list='component_categories'
      continue
    fi
    if [[ "$line" =~ ^[[:space:]]{6}alwaysRun: ]]; then
      current_list='component_always'
      continue
    fi
  fi

  if [[ "$line" =~ ^[[:space:]]*-[[:space:]]*(.*)$ ]]; then
    item="$(trim_yaml_value "${BASH_REMATCH[1]}")"
    case "$current_list" in
      global_always)
        GLOBAL_ALWAYS+=("$item")
        ;;
      global_full)
        GLOBAL_FULL_TRIGGERS+=("$item")
        ;;
      component_paths)
        append_assoc COMPONENT_PATHS "$current_component" "$item"
        ;;
      component_categories)
        append_assoc COMPONENT_CATEGORIES "$current_component" "$item"
        ;;
      component_always)
        append_assoc COMPONENT_ALWAYS "$current_component" "$item"
        ;;
    esac
  fi
done < "$CONFIG_FILE"

declare -a MATCHED_COMPONENTS=()
declare -A MATCHED_COMPONENT_SEEN=()
declare -a IMPACTED_CATEGORIES=()
declare -A IMPACTED_CATEGORY_SEEN=()
declare -a ALWAYS_RUN=()
declare -A ALWAYS_RUN_SEEN=()
declare -a FULL_SUITE_FILES=()

for always_item in "${GLOBAL_ALWAYS[@]}"; do
  append_unique "$always_item" ALWAYS_RUN ALWAYS_RUN_SEEN
done

FULL_SUITE_REQUIRED='false'
for changed_file in "${CHANGED_FILES[@]}"; do
  for trigger_pattern in "${GLOBAL_FULL_TRIGGERS[@]}"; do
    if path_matches_pattern "$changed_file" "$trigger_pattern"; then
      FULL_SUITE_REQUIRED='true'
      FULL_SUITE_FILES+=("$changed_file matches $trigger_pattern")
    fi
  done

done

for component in "${COMPONENTS[@]}"; do
  component_matched='false'
  while IFS= read -r component_pattern; do
    [[ -n "$component_pattern" ]] || continue
    for changed_file in "${CHANGED_FILES[@]}"; do
      if path_matches_pattern "$changed_file" "$component_pattern"; then
        component_matched='true'
      fi
    done
  done <<< "${COMPONENT_PATHS[$component]:-}"

  [[ "$component_matched" == 'true' ]] || continue
  append_unique "$component" MATCHED_COMPONENTS MATCHED_COMPONENT_SEEN

  while IFS= read -r category_item; do
    append_unique "$category_item" IMPACTED_CATEGORIES IMPACTED_CATEGORY_SEEN
  done <<< "${COMPONENT_CATEGORIES[$component]:-}"

  while IFS= read -r always_item; do
    append_unique "$always_item" ALWAYS_RUN ALWAYS_RUN_SEEN
  done <<< "${COMPONENT_ALWAYS[$component]:-}"
done

if [[ "$FORMAT" == 'json' ]]; then
  printf '{'
  printf '"configured": true, '
  printf '"config": "%s", ' "$(json_escape "$CONFIG_FILE")"
  printf '"changedFiles": '
  json_array "${CHANGED_FILES[@]}"
  printf ', "matchedComponents": '
  json_array "${MATCHED_COMPONENTS[@]}"
  printf ', "testCategories": '
  json_array "${IMPACTED_CATEGORIES[@]}"
  printf ', "alwaysRun": '
  json_array "${ALWAYS_RUN[@]}"
  printf ', "fullSuiteRequired": %s, ' "$FULL_SUITE_REQUIRED"
  printf '"fullSuiteReasons": '
  json_array "${FULL_SUITE_FILES[@]}"
  printf '}\n'
  exit 0
fi

echo "Test Impact Plan"
echo "Configured: true"
echo "Config: $CONFIG_FILE"
echo "Changed files:"
for changed_file in "${CHANGED_FILES[@]}"; do
  echo "- $changed_file"
done

echo "Matched components:"
if [[ "${#MATCHED_COMPONENTS[@]}" -eq 0 ]]; then
  echo "- none"
else
  for component in "${MATCHED_COMPONENTS[@]}"; do
    echo "- $component"
  done
fi

echo "Impacted test categories:"
if [[ "${#IMPACTED_CATEGORIES[@]}" -eq 0 ]]; then
  echo "- none mapped"
else
  for category in "${IMPACTED_CATEGORIES[@]}"; do
    echo "- $category"
  done
fi

echo "Always-run checks:"
if [[ "${#ALWAYS_RUN[@]}" -eq 0 ]]; then
  echo "- none mapped"
else
  for check_name in "${ALWAYS_RUN[@]}"; do
    echo "- $check_name"
  done
fi

echo "Full suite required: $FULL_SUITE_REQUIRED"
if [[ "${#FULL_SUITE_FILES[@]}" -gt 0 ]]; then
  echo "Full suite triggers:"
  for trigger_reason in "${FULL_SUITE_FILES[@]}"; do
    echo "- $trigger_reason"
  done
fi
