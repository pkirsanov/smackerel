#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
  FRAMEWORK_DIR="$REPO_ROOT/.github/bubbles"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
  FRAMEWORK_DIR="$REPO_ROOT/bubbles"
fi

REGISTRY_FILE="$FRAMEWORK_DIR/action-risk-registry.yaml"

valid_classes='read_only owned_mutation destructive_mutation external_side_effect runtime_teardown'
required_commands='doctor runtime framework-validate release-check repo-readiness framework-events run-state policy recall'

if [[ ! -f "$REGISTRY_FILE" ]]; then
  echo "Missing action risk registry: $REGISTRY_FILE" >&2
  exit 1
fi

for command_name in $required_commands; do
  if ! grep -q "^  ${command_name}:$" "$REGISTRY_FILE"; then
    echo "Missing action risk command entry: $command_name" >&2
    exit 1
  fi
  default_class="$({
    awk -v cmd="$command_name" '
      $0 == "  " cmd ":" { in_cmd = 1; next }
      in_cmd && $0 ~ /^  [^[:space:]].*:/ { exit }
      in_cmd && /defaultRiskClass:/ {
        sub(/.*defaultRiskClass:[[:space:]]*/, "", $0)
        print $0
        exit
      }
    ' "$REGISTRY_FILE"
  } || true)"
  if [[ -z "$default_class" ]]; then
    echo "Missing defaultRiskClass for command: $command_name" >&2
    exit 1
  fi
  if [[ " $valid_classes " != *" $default_class "* ]]; then
    echo "Invalid defaultRiskClass '$default_class' for command: $command_name" >&2
    exit 1
  fi
done

recall_default_class="$({
  awk '
    $0 == "  recall:" { in_cmd = 1; next }
    in_cmd && $0 ~ /^  [^[:space:]].*:/ { exit }
    in_cmd && /defaultRiskClass:/ {
      sub(/.*defaultRiskClass:[[:space:]]*/, "", $0)
      print $0
      exit
    }
  ' "$REGISTRY_FILE"
} || true)"
if [[ "$recall_default_class" != "owned_mutation" ]]; then
  echo "Recall unknown-operation default must be owned_mutation" >&2
  exit 1
fi

for recall_operation in search read status freshness sync; do
  recall_class="$({
  awk '
    $0 == "  recall:" { in_cmd = 1; next }
    in_cmd && $0 ~ /^  [^[:space:]].*:/ { exit }
    in_cmd && $0 == "    overrides:" { in_overrides = 1; next }
    in_overrides && $0 ~ "^      " operation ":" {
      sub(/^[^:]*:[[:space:]]*/, "", $0)
      print $0
      exit
    }
  ' operation="$recall_operation" "$REGISTRY_FILE"
  } || true)"
  recall_expected="read_only"
  [[ "$recall_operation" == "sync" ]] && recall_expected="owned_mutation"
  if [[ "$recall_class" != "$recall_expected" ]]; then
    echo "Recall $recall_operation must be classified as $recall_expected" >&2
    exit 1
  fi
done

grep -E 'defaultRiskClass:|: (read_only|owned_mutation|destructive_mutation|external_side_effect|runtime_teardown)$' "$REGISTRY_FILE" >/dev/null

echo "Action risk registry OK: $REGISTRY_FILE"
