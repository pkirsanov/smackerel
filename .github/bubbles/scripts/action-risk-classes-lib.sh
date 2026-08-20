#!/usr/bin/env bash
# action-risk-classes-lib.sh — ONE authority for the action risk-class vocabulary.
#
# Sourceable and Bash-3.2-safe (no associative arrays, no mapfile).
#
# Before this lib the vocabulary was written twice with no binding between the
# copies: the complete valid set lived in action-risk-registry-lint.sh, and the
# blocking/warning subsets lived independently in pre-tool-risk-gate.sh. A class
# added to one and not the other silently produced an ALLOW, because the gate's
# decision path treats "not in BLOCK_CLASSES" as permission. IMP-052 SCOPE-1
# requires a single authority; consumers source this file instead of re-listing.
#
# Exports:
#   ACTION_RISK_CLASSES        — array of every valid class, ascending severity
#   ACTION_RISK_BLOCK_CLASSES  — classes that MUST block before execution
#   ACTION_RISK_WARN_CLASSES   — classes that warn but proceed
#   action_risk_classes_list   — prints the valid classes space-separated
#   action_risk_is_valid_class — exit 0 iff "$1" is exactly a valid class

if [ -n "${_ACTION_RISK_CLASSES_LIB_SOURCED:-}" ]; then
  return 0
fi
_ACTION_RISK_CLASSES_LIB_SOURCED=1

# Ascending severity. Order is the reporting order for a validation failure, so
# an operator reads offending classes against a stable severity ladder.
ACTION_RISK_CLASSES=(
  "read_only"
  "owned_mutation"
  "runtime_teardown"
  "external_side_effect"
  "destructive_mutation"
)

# Consumed by pre-tool-risk-gate.sh; shellcheck cannot see across the source seam.
# shellcheck disable=SC2034
ACTION_RISK_BLOCK_CLASSES=(
  "destructive_mutation"
  "external_side_effect"
)

# shellcheck disable=SC2034
ACTION_RISK_WARN_CLASSES=(
  "runtime_teardown"
)

action_risk_classes_list() {
  local out="" name
  for name in "${ACTION_RISK_CLASSES[@]}"; do
    if [ -z "$out" ]; then
      out="$name"
    else
      out="$out $name"
    fi
  done
  printf '%s' "$out"
}

# Exact match only. A near-miss (typo, hyphen, different case) is NOT valid --
# that is precisely the input class that used to reach the gate's silent allow.
action_risk_is_valid_class() {
  local candidate="${1-}" name
  [ -n "$candidate" ] || return 1
  for name in "${ACTION_RISK_CLASSES[@]}"; do
    if [ "$candidate" = "$name" ]; then
      return 0
    fi
  done
  return 1
}
