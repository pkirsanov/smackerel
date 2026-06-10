#!/usr/bin/env bash
# scenario-compile-lint.sh — validates a compiled Goal Scenario DAG against the
# contract in agents/bubbles_shared/scenario-compile.md.
#
# Usage:
#   scenario-compile-lint.sh <scenario-json> [repo-root]   # validate a scenario
#   scenario-compile-lint.sh --list-forbidden [repo-root]  # print derived fan-out set
#
# A goal scenario is a runtime execution plan (a dependency-ordered DAG whose
# nodes each resolve to one EXISTING workflow mode or specialist). This lint
# enforces every Hard Rule:
#   1. No node resolves to a requiresTopLevelRuntime fan-out mode (Gate G064).
#      The forbidden set is DERIVED from bubbles/workflows/modes.yaml so it
#      never drifts.
#   2. Every node references a real mode (modes.yaml) or agent
#      (agent-capabilities.yaml); exactly one of mode/agent per node.
#   3. Every node declares a repo that exists in repos[].
#   4. action nodes: approvalRequired==true AND riskClass set AND opsPacket set.
#   5. ongoing-ops nodes: opsPacket set.
#   6. dependsOn forms a DAG (known ids, no self-ref, no cycles).
#   7. Node ids unique.
#   8. rootOutcome is a complete Outcome Contract (intent, successSignal,
#      hardConstraints[non-empty], failureCondition).
#
# Exit 0 = clean. Exit 1 = violation. Exit 2 = usage error.

set -euo pipefail

ALLOWED_TYPES="diagnostic planning delivery verification action ongoing-ops"

# Fallback fan-out set used only when modes.yaml / yq are unavailable. The
# authoritative source is modes.yaml constraints.requiresTopLevelRuntime.
FALLBACK_FORBIDDEN="iterate autonomous-goal autonomous-sprint stochastic-quality-sweep retro-quality-sweep idea-to-release-completion"

err() { echo "[scenario-compile-lint][ERROR] $*" >&2; FAILED=1; }
info() { echo "[scenario-compile-lint] $*"; }

resolve_repo_root() {
  local root="${1:-}"
  if [[ -n "$root" ]]; then echo "$root"; return; fi
  local sd
  sd="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  # script lives in <root>/bubbles/scripts
  ( cd "$sd/../.." && pwd )
}

modes_file() {
  local root="$1"
  if [[ -f "$root/bubbles/workflows/modes.yaml" ]]; then
    echo "$root/bubbles/workflows/modes.yaml"
  else
    echo "$root/bubbles/workflows.yaml"
  fi
}

derive_forbidden() {
  local root="$1"
  local mf
  mf="$(modes_file "$root")"
  if command -v yq >/dev/null 2>&1 && [[ -f "$mf" ]]; then
    local out
    out="$(yq -r '.modes | to_entries | .[] | select(.value.constraints.requiresTopLevelRuntime == true) | .key' "$mf" 2>/dev/null || true)"
    if [[ -n "$out" ]]; then echo "$out"; return; fi
  fi
  # Fallback
  printf '%s\n' $FALLBACK_FORBIDDEN
}

known_modes() {
  local root="$1"
  local mf
  mf="$(modes_file "$root")"
  command -v yq >/dev/null 2>&1 && [[ -f "$mf" ]] || return 0
  yq -r '.modes | keys | .[]' "$mf" 2>/dev/null || true
}

known_agents() {
  local root="$1"
  local cf="$root/bubbles/agent-capabilities.yaml"
  command -v yq >/dev/null 2>&1 && [[ -f "$cf" ]] || return 0
  yq -r '.agents | keys | .[]' "$cf" 2>/dev/null || true
}

in_list() {
  local needle="$1"; shift
  local x
  for x in "$@"; do [[ "$x" == "$needle" ]] && return 0; done
  return 1
}

# ---- --list-forbidden short-circuit ----
if [[ "${1:-}" == "--list-forbidden" ]]; then
  ROOT="$(resolve_repo_root "${2:-}")"
  derive_forbidden "$ROOT"
  exit 0
fi

SCENARIO="${1:-}"
ROOT="$(resolve_repo_root "${2:-}")"
FAILED=0

[[ -n "$SCENARIO" ]] || { echo "usage: scenario-compile-lint.sh <scenario-json> [repo-root]" >&2; exit 2; }
[[ -f "$SCENARIO" ]] || { err "scenario file not found: $SCENARIO"; exit 1; }
command -v jq >/dev/null 2>&1 || { err "jq is required"; exit 1; }
jq -e . "$SCENARIO" >/dev/null 2>&1 || { err "scenario is not valid JSON: $SCENARIO"; exit 1; }

# ---- top-level fields ----
SCEN_ID="$(jq -r '.scenarioId // ""' "$SCENARIO")"
[[ -n "$SCEN_ID" ]] || err "scenarioId is missing or empty"

# Outcome Contract (Gate G070 shape)
[[ "$(jq -r '.rootOutcome.intent // ""' "$SCENARIO")" != "" ]] || err "rootOutcome.intent missing"
[[ "$(jq -r '.rootOutcome.successSignal // ""' "$SCENARIO")" != "" ]] || err "rootOutcome.successSignal missing"
[[ "$(jq -r '.rootOutcome.failureCondition // ""' "$SCENARIO")" != "" ]] || err "rootOutcome.failureCondition missing"
[[ "$(jq -r '(.rootOutcome.hardConstraints // []) | length' "$SCENARIO")" -gt 0 ]] || err "rootOutcome.hardConstraints must be a non-empty array"

# ---- repos ----
REPO_COUNT="$(jq -r '(.repos // []) | length' "$SCENARIO")"
[[ "$REPO_COUNT" -gt 0 ]] || err "repos[] must be non-empty"
mapfile -t REPO_IDS < <(jq -r '(.repos // [])[].id // ""' "$SCENARIO")
for rid in "${REPO_IDS[@]}"; do
  [[ -n "$rid" ]] || err "a repos[] entry is missing an id"
done

# ---- nodes ----
NODE_COUNT="$(jq -r '(.nodes // []) | length' "$SCENARIO")"
[[ "$NODE_COUNT" -gt 0 ]] || { err "nodes[] must be non-empty"; }

# Derive the forbidden fan-out set + known modes/agents.
mapfile -t FORBIDDEN < <(derive_forbidden "$ROOT")
mapfile -t MODES < <(known_modes "$ROOT")
mapfile -t AGENTS < <(known_agents "$ROOT")

declare -A SEEN_NODE_IDS
declare -A NODE_DEPS

if [[ "${NODE_COUNT:-0}" -gt 0 ]]; then
  for ((i = 0; i < NODE_COUNT; i++)); do
    nid="$(jq -r ".nodes[$i].id // \"\"" "$SCENARIO")"
    ntype="$(jq -r ".nodes[$i].type // \"\"" "$SCENARIO")"
    nrepo="$(jq -r ".nodes[$i].repo // \"\"" "$SCENARIO")"
    nmode="$(jq -r ".nodes[$i].mode // \"\"" "$SCENARIO")"
    nagent="$(jq -r ".nodes[$i].agent // \"\"" "$SCENARIO")"
    napproval="$(jq -r ".nodes[$i].approvalRequired // false" "$SCENARIO")"
    nrisk="$(jq -r ".nodes[$i].riskClass // \"\"" "$SCENARIO")"
    nops="$(jq -r ".nodes[$i].opsPacket // \"\"" "$SCENARIO")"

    label="nodes[$i]"
    [[ -n "$nid" ]] && label="node '$nid'"

    # id present + unique
    if [[ -z "$nid" ]]; then
      err "nodes[$i]: id missing"
    elif [[ -n "${SEEN_NODE_IDS[$nid]:-}" ]]; then
      err "duplicate node id '$nid'"
    else
      SEEN_NODE_IDS[$nid]=1
    fi

    # type
    if ! in_list "$ntype" $ALLOWED_TYPES; then
      err "$label: type '$ntype' invalid (allowed: $ALLOWED_TYPES)"
    fi

    # repo
    if [[ -z "$nrepo" ]]; then
      err "$label: repo missing"
    elif ! in_list "$nrepo" "${REPO_IDS[@]}"; then
      err "$label: repo '$nrepo' not declared in repos[]"
    fi

    # exactly one of mode/agent
    if [[ -n "$nmode" && -n "$nagent" ]]; then
      err "$label: declares both mode and agent (exactly one required)"
    elif [[ -z "$nmode" && -z "$nagent" ]]; then
      err "$label: declares neither mode nor agent (exactly one required)"
    fi

    # mode checks
    if [[ -n "$nmode" ]]; then
      if in_list "$nmode" "${FORBIDDEN[@]}"; then
        err "$label: mode '$nmode' is a requiresTopLevelRuntime fan-out mode and MUST NOT be a scenario node (Gate G064)"
      fi
      if [[ "${#MODES[@]}" -gt 0 ]] && ! in_list "$nmode" "${MODES[@]}"; then
        err "$label: mode '$nmode' is not defined in modes.yaml"
      fi
    fi

    # agent checks
    if [[ -n "$nagent" && "${#AGENTS[@]}" -gt 0 ]] && ! in_list "$nagent" "${AGENTS[@]}"; then
      err "$label: agent '$nagent' is not defined in agent-capabilities.yaml"
    fi

    # action node gating
    if [[ "$ntype" == "action" ]]; then
      [[ "$napproval" == "true" ]] || err "$label: action node requires approvalRequired: true"
      [[ -n "$nrisk" ]] || err "$label: action node requires riskClass"
      [[ -n "$nops" ]] || err "$label: action node requires opsPacket"
    fi

    # ongoing-ops node
    if [[ "$ntype" == "ongoing-ops" ]]; then
      [[ -n "$nops" ]] || err "$label: ongoing-ops node requires opsPacket"
    fi

    # collect deps (validated after all ids known)
    deps="$(jq -r ".nodes[$i].dependsOn // [] | .[]" "$SCENARIO" 2>/dev/null | tr '\n' ' ')"
    NODE_DEPS[$nid]="$deps"
  done

  # dependsOn references + self-ref
  for nid in "${!NODE_DEPS[@]}"; do
    for dep in ${NODE_DEPS[$nid]}; do
      if [[ "$dep" == "$nid" ]]; then
        err "node '$nid': dependsOn references itself"
      elif [[ -z "${SEEN_NODE_IDS[$dep]:-}" ]]; then
        err "node '$nid': dependsOn references unknown node '$dep'"
      fi
    done
  done

  # Cycle detection via Kahn's algorithm.
  declare -A RESOLVED
  resolved_count=0
  total_nodes="${#SEEN_NODE_IDS[@]}"
  progress=1
  while [[ "$resolved_count" -lt "$total_nodes" && "$progress" -eq 1 ]]; do
    progress=0
    for nid in "${!SEEN_NODE_IDS[@]}"; do
      [[ -n "${RESOLVED[$nid]:-}" ]] && continue
      all_deps_resolved=1
      for dep in ${NODE_DEPS[$nid]:-}; do
        # ignore unknown deps (already reported); only gate on known unresolved deps
        [[ -z "${SEEN_NODE_IDS[$dep]:-}" ]] && continue
        if [[ -z "${RESOLVED[$dep]:-}" ]]; then all_deps_resolved=0; break; fi
      done
      if [[ "$all_deps_resolved" -eq 1 ]]; then
        RESOLVED[$nid]=1
        resolved_count=$((resolved_count + 1))
        progress=1
      fi
    done
  done
  if [[ "$resolved_count" -lt "$total_nodes" ]]; then
    err "dependsOn graph contains a cycle (could not topologically order all nodes)"
  fi
fi

if [[ "$FAILED" -ne 0 ]]; then
  exit 1
fi

info "OK (scenario '$SCEN_ID': $NODE_COUNT node(s), $REPO_COUNT repo(s) validated)"
exit 0
