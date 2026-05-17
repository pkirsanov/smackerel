#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

CONTRACT_FILE=''
if [[ -f "$REPO_ROOT/.github/bubbles-project.yaml" ]]; then
  CONTRACT_FILE="$REPO_ROOT/.github/bubbles-project.yaml"
elif [[ -f "$REPO_ROOT/bubbles-project.yaml" ]]; then
  CONTRACT_FILE="$REPO_ROOT/bubbles-project.yaml"
fi

TRACE_FILE=''
WORKFLOW_ID=''
REQUIRE_CONFIG='false'

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/trace-contract-guard.sh --trace-output PATH [options]

Validate trace evidence against optional project-owned traceContracts.
This guard checks mechanical minimums: required span names, attributes,
invariant evidence strings, and configured red-flag patterns.

Options:
  --contract PATH       Use an explicit contract YAML file
  --trace-output PATH   Trace evidence/export to inspect
  --workflow ID         Validate one traceContracts.workflows entry
  --repo-root PATH      Use an explicit repository root
  --require-config      Fail if no traceContracts map is configured
  --help                Show this help

Supported YAML shape:

traceContracts:
  workflows:
    booking.create:
      requiredSpans:
        - name: api.request
          attributes:
            - trace_id
      requiredAttributes:
        - booking.id
      requiredInvariants:
        - booking emitted exactly one confirmation event
      redFlags:
        error:
          - Missing trace_id
        warning:
          - slow span
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --contract)
      CONTRACT_FILE="$2"
      shift 2
      ;;
    --trace-output|--trace-evidence)
      TRACE_FILE="$2"
      shift 2
      ;;
    --workflow)
      WORKFLOW_ID="$2"
      shift 2
      ;;
    --repo-root)
      REPO_ROOT="$2"
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
      echo "Unexpected argument: $1" >&2
      exit 2
      ;;
  esac
done

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

append_item() {
  local value="$1"
  shift
  local -n target_array="$1"

  [[ -n "$value" ]] || return 0
  target_array+=("$value")
}

if [[ -z "$TRACE_FILE" ]]; then
  echo "Missing required --trace-output PATH" >&2
  exit 2
fi

[[ -f "$TRACE_FILE" ]] || {
  echo "Trace evidence file does not exist: $TRACE_FILE" >&2
  exit 2
}

if [[ -z "$CONTRACT_FILE" || ! -f "$CONTRACT_FILE" ]]; then
  if [[ "$REQUIRE_CONFIG" == 'true' ]]; then
    echo "No trace contract file found." >&2
    exit 1
  fi
  echo "Trace Contract Guard"
  echo "Configured: false"
  echo "Trace evidence: $TRACE_FILE"
  echo "No traceContracts map configured; skipping optional guard."
  exit 0
fi

if ! grep -q '^traceContracts:' "$CONTRACT_FILE"; then
  if [[ "$REQUIRE_CONFIG" == 'true' ]]; then
    echo "Configured file has no top-level traceContracts map: $CONTRACT_FILE" >&2
    exit 1
  fi
  echo "Trace Contract Guard"
  echo "Configured: false"
  echo "Contract: $CONTRACT_FILE"
  echo "Trace evidence: $TRACE_FILE"
  echo "No traceContracts map configured; skipping optional guard."
  exit 0
fi

declare -a REQUIRED_SPANS=()
declare -a REQUIRED_ATTRIBUTES=()
declare -a REQUIRED_INVARIANTS=()
declare -a ERROR_REDFLAGS=()
declare -a WARNING_REDFLAGS=()

in_contracts='false'
in_workflows='false'
in_selected_workflow='false'
current_workflow=''
current_list=''
current_redflag_severity=''
selected_seen='false'

while IFS= read -r raw_line; do
  line="${raw_line%%$'\r'}"
  [[ -z "$(trim_yaml_value "$line")" ]] && continue
  [[ "$(trim_yaml_value "$line")" == \#* ]] && continue

  if [[ "$line" =~ ^[^[:space:]][A-Za-z0-9_-]+: ]]; then
    if [[ "$line" == traceContracts:* ]]; then
      in_contracts='true'
      in_workflows='false'
      in_selected_workflow='false'
      current_workflow=''
      current_list=''
      current_redflag_severity=''
      continue
    fi

    if [[ "$in_contracts" == 'true' ]]; then
      break
    fi
  fi

  [[ "$in_contracts" == 'true' ]] || continue

  if [[ "$line" =~ ^[[:space:]]{2}workflows: ]]; then
    in_workflows='true'
    continue
  fi

  if [[ "$in_workflows" == 'true' && "$line" =~ ^[[:space:]]{4}([A-Za-z0-9_.:-]+):[[:space:]]*$ ]]; then
    current_workflow="${BASH_REMATCH[1]}"
    current_list=''
    current_redflag_severity=''
    if [[ -z "$WORKFLOW_ID" || "$WORKFLOW_ID" == "$current_workflow" ]]; then
      in_selected_workflow='true'
      selected_seen='true'
    else
      in_selected_workflow='false'
    fi
    continue
  fi

  [[ "$in_selected_workflow" == 'true' ]] || continue

  if [[ "$line" =~ ^[[:space:]]{6}requiredSpans: ]]; then
    current_list='required_spans'
    current_redflag_severity=''
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]{6}requiredAttributes: ]]; then
    current_list='required_attributes'
    current_redflag_severity=''
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]{6}requiredInvariants: || "$line" =~ ^[[:space:]]{6}invariants: ]]; then
    current_list='required_invariants'
    current_redflag_severity=''
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]{6}redFlags: ]]; then
    current_list='red_flags'
    current_redflag_severity=''
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]{8}attributes: ]]; then
    current_list='required_attributes'
    current_redflag_severity=''
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]{8}error: ]]; then
    current_list='red_flags'
    current_redflag_severity='error'
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]{8}warning: ]]; then
    current_list='red_flags'
    current_redflag_severity='warning'
    continue
  fi

  if [[ "$line" =~ ^[[:space:]]*-[[:space:]]*name:[[:space:]]*(.*)$ ]]; then
    append_item "$(trim_yaml_value "${BASH_REMATCH[1]}")" REQUIRED_SPANS
    continue
  fi

  if [[ "$line" =~ ^[[:space:]]*name:[[:space:]]*(.*)$ && "$current_list" == 'required_spans' ]]; then
    append_item "$(trim_yaml_value "${BASH_REMATCH[1]}")" REQUIRED_SPANS
    continue
  fi

  if [[ "$line" =~ ^[[:space:]]*-[[:space:]]*(.*)$ ]]; then
    item="$(trim_yaml_value "${BASH_REMATCH[1]}")"
    case "$current_list" in
      required_spans)
        append_item "$item" REQUIRED_SPANS
        ;;
      required_attributes)
        append_item "$item" REQUIRED_ATTRIBUTES
        ;;
      required_invariants)
        append_item "$item" REQUIRED_INVARIANTS
        ;;
      red_flags)
        case "$current_redflag_severity" in
          error) append_item "$item" ERROR_REDFLAGS ;;
          warning) append_item "$item" WARNING_REDFLAGS ;;
        esac
        ;;
    esac
  fi
done < "$CONTRACT_FILE"

if [[ -n "$WORKFLOW_ID" && "$selected_seen" != 'true' ]]; then
  echo "No trace contract workflow found for: $WORKFLOW_ID" >&2
  exit 1
fi

if [[ "${#REQUIRED_SPANS[@]}" -eq 0 && "${#REQUIRED_ATTRIBUTES[@]}" -eq 0 && "${#REQUIRED_INVARIANTS[@]}" -eq 0 && "${#ERROR_REDFLAGS[@]}" -eq 0 && "${#WARNING_REDFLAGS[@]}" -eq 0 ]]; then
  echo "traceContracts exists but no mechanical requirements were found." >&2
  exit 1
fi

failures=0
warnings=0

check_required_string() {
  local label="$1"
  local value="$2"

  if grep -Fq -- "$value" "$TRACE_FILE"; then
    echo "PASS: $label present: $value"
  else
    echo "FAIL: $label missing: $value"
    failures=$((failures + 1))
  fi
}

for span_name in "${REQUIRED_SPANS[@]}"; do
  check_required_string 'required span' "$span_name"
done

for attribute_name in "${REQUIRED_ATTRIBUTES[@]}"; do
  check_required_string 'required attribute' "$attribute_name"
done

for invariant_text in "${REQUIRED_INVARIANTS[@]}"; do
  check_required_string 'required invariant evidence' "$invariant_text"
done

for redflag in "${ERROR_REDFLAGS[@]}"; do
  if grep -Fq -- "$redflag" "$TRACE_FILE"; then
    echo "FAIL: error red flag observed: $redflag"
    failures=$((failures + 1))
  else
    echo "PASS: error red flag absent: $redflag"
  fi
done

for redflag in "${WARNING_REDFLAGS[@]}"; do
  if grep -Fq -- "$redflag" "$TRACE_FILE"; then
    echo "WARN: warning red flag observed: $redflag"
    warnings=$((warnings + 1))
  else
    echo "PASS: warning red flag absent: $redflag"
  fi
done

echo "Trace Contract Guard"
echo "Configured: true"
echo "Contract: $CONTRACT_FILE"
echo "Trace evidence: $TRACE_FILE"
if [[ -n "$WORKFLOW_ID" ]]; then
  echo "Workflow: $WORKFLOW_ID"
else
  echo "Workflow: all"
fi
echo "Failures: $failures"
echo "Warnings: $warnings"

if [[ "$failures" -gt 0 ]]; then
  exit 1
fi
