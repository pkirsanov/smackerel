#!/usr/bin/env bash
# convergence-materiality.sh — the convergence materiality brake
# (IMP-041 SCOPE-7 / GF-13).
#
# WHY PERSISTENCE NEEDS A BRAKE
#
# The `autonomous-goal` mode enables solution search and
# `neverStopForFixableObstacles`. Both are correct for their purpose: an agent
# that stops at the first compile error is useless. But neither rule
# distinguishes "this is hard" from "this is BIGGER". Without that distinction,
# persistence amplifies expansion — every iteration that discovers more work
# treats the extra work as an obstacle to push through rather than as evidence
# the goal changed.
#
# This brake makes the distinction mechanical. It compares the current
# iteration's planned delta against the baseline recorded at the first
# iteration:
#
#   narrower or equal  -> proceed (solution search is doing its job)
#   larger             -> REFUSE, naming exactly what grew
#
# An undeclared expansion is a NEW GOAL, not a fixable obstacle. The refusal
# offers the only two honest ways forward: narrow the plan, or widen the
# contract through an approved revision.
#
# A generic continuation resumes the approved graph and nothing more. A session
# budget limits runtime cost and never grants scope.
#
# Exit codes
#   0  baseline recorded, or the iteration is within it
#   1  REFUSED — the plan grew relative to the baseline
#   2  usage or runtime error
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: convergence-materiality.sh check --session-file <path> --planned-delta <json>
                                        [--iteration <n>] [--scenario-file <path>]
       convergence-materiality.sh baseline --session-file <path> --planned-delta <json>
                                           [--scenario-file <path>]
       convergence-materiality.sh show --session-file <path>

  check     compare this iteration against the recorded baseline; records the
            baseline itself when none exists yet
  baseline  (re)record a baseline — only permitted when the contract revision
            changed, i.e. after an approved widening
  show      print the recorded baseline

There is no --force / --skip / --accept-growth.
EOF
}

fail_usage() { echo "convergence-materiality: $*" >&2; exit 2; }
fail_refuse() { echo "convergence-materiality: REFUSED — $*" >&2; exit 1; }

command -v jq >/dev/null 2>&1 || fail_usage "jq is required"

SESSION_FILE=""
PLANNED_DELTA=""
SCENARIO_FILE=""
ITERATION=""

parse() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --session-file) SESSION_FILE="${2:-}"; shift 2 ;;
      --planned-delta) PLANNED_DELTA="${2:-}"; shift 2 ;;
      --scenario-file) SCENARIO_FILE="${2:-}"; shift 2 ;;
      --iteration) ITERATION="${2:-}"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      --force|--skip|--accept-growth|--no-verify)
        fail_usage "bypass-shaped flag '$1' does not exist — growth is a new goal, not an obstacle to push through" ;;
      *) fail_usage "unknown option: $1" ;;
    esac
  done
  [[ -n "$SESSION_FILE" ]] || fail_usage "--session-file is required"
  [[ -f "$SESSION_FILE" ]] || fail_usage "session file not found: $SESSION_FILE"
}

require_delta() {
  [[ -n "$PLANNED_DELTA" ]] || fail_usage "--planned-delta is required"
  jq empty <<< "$PLANNED_DELTA" 2>/dev/null || fail_usage "--planned-delta is not valid JSON"
}

# Normalised comparison shape. Targets and change classes are sets; every max*
# key is a scalar ceiling. Anything absent from the baseline counts as zero, so
# a NEW dimension is growth rather than an unconstrained free pass.
normalise() {
  local delta="$1" scenario_targets="$2"
  jq -n --argjson d "$delta" --argjson t "$scenario_targets" '{
    changeClasses: (($d.changeClasses // []) | unique),
    targets: ($t | unique),
    counts: ($d | with_entries(select(.key | startswith("max"))))
  }'
}

scenario_targets() {
  if [[ -n "$SCENARIO_FILE" ]]; then
    [[ -f "$SCENARIO_FILE" ]] || fail_usage "scenario file not found: $SCENARIO_FILE"
    jq -c '[ (.repos // [])[].id ] + [ (.nodes // [])[].repo ] | map(select(. != null)) | unique' "$SCENARIO_FILE"
  else
    printf '[]'
  fi
}

write_baseline() {
  local payload="$1" revision="$2" tmp
  tmp="$(mktemp "$(dirname "$SESSION_FILE")/.convergence-materiality.XXXXXX")"
  jq --argjson b "$payload" --argjson rev "$revision" \
    '. + { convergenceBaseline: ($b + { atRevision: $rev }) }' "$SESSION_FILE" > "$tmp"
  mv "$tmp" "$SESSION_FILE"
}

contract_revision() {
  jq -r '.goalContract.revision // 0' "$SESSION_FILE"
}

cmd_show() {
  parse "$@"
  jq -c '.convergenceBaseline // null' "$SESSION_FILE"
}

cmd_baseline() {
  parse "$@"
  require_delta
  local current rev existing
  current="$(normalise "$PLANNED_DELTA" "$(scenario_targets)")"
  rev="$(contract_revision)"
  existing="$(jq -c '.convergenceBaseline // null' "$SESSION_FILE")"
  if [[ "$existing" != "null" ]]; then
    local at
    at="$(jq -r '.atRevision // -1' <<< "$existing")"
    # Re-baselining without an approved revision is how a brake gets released
    # from inside the loop. Only a contract revision may reset it.
    [[ "$at" != "$rev" ]] ||
      fail_refuse "a baseline already exists at contract revision $rev. Re-baselining without an approved revision would release the brake from inside the loop — widen the contract with 'goal-contract.sh revise --approval-note' first."
  fi
  write_baseline "$current" "$rev"
  echo "convergence-materiality: baseline recorded at contract revision $rev"
}

cmd_check() {
  parse "$@"
  require_delta
  local current rev baseline
  current="$(normalise "$PLANNED_DELTA" "$(scenario_targets)")"
  rev="$(contract_revision)"
  baseline="$(jq -c '.convergenceBaseline // null' "$SESSION_FILE")"

  if [[ "$baseline" == "null" ]]; then
    write_baseline "$current" "$rev"
    echo "convergence-materiality: OK (first iteration — baseline recorded at contract revision $rev)"
    return 0
  fi

  # An approved revision legitimately resets the brake: the operator has just
  # re-declared what the goal is allowed to become.
  local at
  at="$(jq -r '.atRevision // -1' <<< "$baseline")"
  if [[ "$at" != "$rev" ]]; then
    write_baseline "$current" "$rev"
    echo "convergence-materiality: OK (contract revision moved $at -> $rev; baseline re-recorded against the approved contract)"
    return 0
  fi

  local growth
  growth="$(jq -r -n --argjson b "$baseline" --argjson c "$current" '
    [ (($c.changeClasses - $b.changeClasses)[] | "change class \(. | tojson)"),
      (($c.targets - $b.targets)[] | "target \(. | tojson)"),
      ($c.counts | to_entries[] | . as $e
        | select($e.value > (($b.counts[$e.key]) // 0))
        | "\($e.key) \((($b.counts[$e.key]) // 0)) -> \($e.value)") ]
    | join("; ")')"

  if [[ -n "$growth" ]]; then
    fail_refuse "iteration ${ITERATION:-<n>} grows the goal: $growth. Undeclared expansion is a NEW GOAL, not a fixable obstacle — neverStopForFixableObstacles does not apply. Either narrow the plan back inside the baseline, or record an approved widening with 'goal-contract.sh revise --approval-note' and re-baseline."
  fi

  echo "convergence-materiality: OK (iteration ${ITERATION:-<n>} is within the baseline at contract revision $rev)"
}

case "${1:-}" in
  check) shift; cmd_check "$@" ;;
  baseline) shift; cmd_baseline "$@" ;;
  show) shift; cmd_show "$@" ;;
  -h|--help) usage; exit 0 ;;
  "") usage >&2; exit 2 ;;
  *) fail_usage "unknown subcommand: $1" ;;
esac
