#!/usr/bin/env bash
# Thin shell facade over the canonical Python execution-control store.

if [[ -n "${BUBBLES_EXECUTION_CONTROL_LIB_LOADED:-}" ]]; then
  return 0 2>/dev/null || exit 0
fi
BUBBLES_EXECUTION_CONTROL_LIB_LOADED=1

BUBBLES_EXECUTION_CONTROL_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUBBLES_EXECUTION_CONTROL_STORE="$BUBBLES_EXECUTION_CONTROL_SCRIPT_DIR/execution-control-store.py"

bubbles_execution_control() {
  python3 "$BUBBLES_EXECUTION_CONTROL_STORE" "$@"
}

bubbles_execution_control_object_put() {
  local store_root="$1" input="$2"
  bubbles_execution_control object-put --store-root "$store_root" --input "$input"
}

bubbles_execution_control_append() {
  local store_root="$1" expected_sequence="$2" expected_head_digest="$3" event_file="$4"
  bubbles_execution_control append \
    --store-root "$store_root" \
    --expected-sequence "$expected_sequence" \
    --expected-head-digest "$expected_head_digest" \
    --event-file "$event_file"
}

bubbles_execution_control_read() {
  local store_root="$1" from_sequence="$2" limit="$3"
  bubbles_execution_control read --store-root "$store_root" --from-sequence "$from_sequence" --limit "$limit"
}

bubbles_execution_control_verify() {
  local store_root="$1" checkpoint="${2:-}"
  if [[ -n "$checkpoint" ]]; then
    bubbles_execution_control verify --store-root "$store_root" --checkpoint "$checkpoint"
  else
    bubbles_execution_control verify --store-root "$store_root"
  fi
}

bubbles_execution_control_project() {
  local store_root="$1"
  bubbles_execution_control project --store-root "$store_root"
}

bubbles_execution_control_checkpoint() {
  local store_root="$1"
  bubbles_execution_control checkpoint --store-root "$store_root"
}
