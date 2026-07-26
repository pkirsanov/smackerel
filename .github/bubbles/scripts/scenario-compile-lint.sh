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
#   9. release coverage (IMP-006 / Gate G101): when rootOutcome.targetReleasePacket
#      names a phase whose docs/releases/<phase>/features.md is reachable under the
#      repo-root, every delivery=required feature MUST be covered by some
#      delivery-type node's coversFeatures[]. Unreachable features.md → info-only
#      (the convergence-time reconciliation guard is the backstop).
#
# Exit 0 = clean. Exit 1 = violation. Exit 2 = usage error.

set -euo pipefail

ALLOWED_TYPES=(diagnostic planning delivery verification action ongoing-ops)

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
  printf '%s\n' \
    iterate \
    autonomous-goal \
    autonomous-sprint \
    stochastic-quality-sweep \
    retro-quality-sweep \
    idea-to-release-completion
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

canonical_git_root() {
  local candidate="$1"
  local physical_candidate
  local git_root

  physical_candidate="$(cd -P -- "$candidate" 2>/dev/null && pwd -P)" || return 1
  git_root="$(git -C "$physical_candidate" rev-parse --show-toplevel 2>/dev/null)" || return 1
  (cd -P -- "$git_root" 2>/dev/null && pwd -P)
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
command -v git >/dev/null 2>&1 || { err "git is required"; exit 1; }
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
REPO_IDS=()
REPO_ALIASES=()
while IFS= read -r repo_id; do
  REPO_IDS[${#REPO_IDS[@]}]="$repo_id"
done < <(jq -r '(.repos // [])[].id // ""' "$SCENARIO")
for ((i = 0; i < REPO_COUNT; i++)); do
  rid="$(jq -r ".repos[$i].id // \"\"" "$SCENARIO")"
  rroot="$(jq -r ".repos[$i].repositoryRoot // \"\"" "$SCENARIO")"
  ralias="$(jq -r ".repos[$i].repositoryAlias // \"\"" "$SCENARIO")"
  [[ -n "$rid" ]] || err "a repos[] entry is missing an id"
  case "$ralias" in
    ""|[!A-Za-z0-9]*|*[!A-Za-z0-9._-]*)
      err "repos[$i] '$rid': repositoryAlias must be one safe path segment"
      ;;
    *)
      if in_list "$ralias" "${REPO_ALIASES[@]}"; then
        err "repos[$i] '$rid': repositoryAlias '$ralias' is duplicated"
      else
        REPO_ALIASES[${#REPO_ALIASES[@]}]="$ralias"
      fi
      ;;
  esac
  if [[ -z "$rroot" ]]; then
    err "repos[$i] '$rid': repositoryRoot missing; a canonical absolute Git root is required"
  elif [[ "$rroot" != /* ]]; then
    err "repos[$i] '$rid': repositoryRoot must be an absolute canonical Git root"
  else
    canonical_root="$(canonical_git_root "$rroot" 2>/dev/null || true)"
    if [[ -z "$canonical_root" || "$canonical_root" != "$rroot" ]]; then
      err "repos[$i] '$rid': repositoryRoot is missing, ineligible, or not the canonical Git root"
    fi
  fi
done

# ---- nodes ----
NODE_COUNT="$(jq -r '(.nodes // []) | length' "$SCENARIO")"
[[ "$NODE_COUNT" -gt 0 ]] || { err "nodes[] must be non-empty"; }

# Derive the forbidden fan-out set + known modes/agents.
FORBIDDEN=()
while IFS= read -r forbidden_mode; do
  FORBIDDEN[${#FORBIDDEN[@]}]="$forbidden_mode"
done < <(derive_forbidden "$ROOT")
MODES=()
while IFS= read -r known_mode; do
  MODES[${#MODES[@]}]="$known_mode"
done < <(known_modes "$ROOT")
AGENTS=()
while IFS= read -r known_agent; do
  AGENTS[${#AGENTS[@]}]="$known_agent"
done < <(known_agents "$ROOT")

NODE_IDS=()

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
    elif in_list "$nid" "${NODE_IDS[@]}"; then
      err "duplicate node id '$nid'"
    else
      NODE_IDS[${#NODE_IDS[@]}]="$nid"
    fi

    # type
    if ! in_list "$ntype" "${ALLOWED_TYPES[@]}"; then
      err "$label: type '$ntype' invalid (allowed: ${ALLOWED_TYPES[*]})"
    fi

    # repo
    if [[ -z "$nrepo" ]]; then
      err "$label: repo missing"
    elif ! in_list "$nrepo" "${REPO_IDS[@]}"; then
      err "$label: repo '$nrepo' not declared in repos[]"
    fi

    if ! jq -e --argjson index "$i" --arg nodeId "$nid" '
      .nodes[$index].repositoryResolution as $resolution
      | ($resolution | type == "object")
      and (($resolution | keys | sort) ==
        (["sessionId", "decisionId", "controlRevision", "controlPathDigest", "authority", "transition",
          "scopeKind", "scopeId", "targetKind", "pathVisibility", "actionable"] | sort))
      and ($resolution.sessionId | type == "string" and length > 0)
      and ($resolution.controlRevision | type == "number" and . >= 1 and floor == .)
      and ($resolution.controlPathDigest | type == "string" and test("^sha256:[0-9a-f]{64}$"))
      and $resolution.decisionId ==
        ("rb:" + $resolution.sessionId + ":" + ($resolution.controlRevision | tostring) + ":node:" + $nodeId)
      and $resolution.authority == "scoped-scenario-node"
      and $resolution.transition == "scoped-override"
      and $resolution.scopeKind == "goal-node"
      and $resolution.scopeId == $nodeId
      and $resolution.targetKind == "goal-node"
      and $resolution.pathVisibility == "local"
      and $resolution.actionable == true
    ' "$SCENARIO" >/dev/null 2>&1; then
      err "$label: repositoryResolution must be the exact local actionable goal-node decision for '$nid'"
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

  done

  # dependsOn references + self-ref
  for ((i = 0; i < NODE_COUNT; i++)); do
    nid="$(jq -r ".nodes[$i].id // \"\"" "$SCENARIO")"
    while IFS= read -r dep; do
      [[ -n "$dep" ]] || continue
      if [[ "$dep" == "$nid" ]]; then
        err "node '$nid': dependsOn references itself"
      elif ! in_list "$dep" "${NODE_IDS[@]}"; then
        err "node '$nid': dependsOn references unknown node '$dep'"
      fi
    done < <(jq -r ".nodes[$i].dependsOn // [] | .[]" "$SCENARIO" 2>/dev/null)
  done

  # Cycle detection via Kahn's algorithm.
  RESOLVED=()
  resolved_count=0
  total_nodes="${#NODE_IDS[@]}"
  progress=1
  while [[ "$resolved_count" -lt "$total_nodes" && "$progress" -eq 1 ]]; do
    progress=0
    for ((i = 0; i < NODE_COUNT; i++)); do
      nid="$(jq -r ".nodes[$i].id // \"\"" "$SCENARIO")"
      in_list "$nid" "${RESOLVED[@]}" && continue
      all_deps_resolved=1
      while IFS= read -r dep; do
        [[ -n "$dep" ]] || continue
        # ignore unknown deps (already reported); only gate on known unresolved deps
        in_list "$dep" "${NODE_IDS[@]}" || continue
        if ! in_list "$dep" "${RESOLVED[@]}"; then
          all_deps_resolved=0
          break
        fi
      done < <(jq -r ".nodes[$i].dependsOn // [] | .[]" "$SCENARIO" 2>/dev/null)
      if [[ "$all_deps_resolved" -eq 1 ]]; then
        RESOLVED[${#RESOLVED[@]}]="$nid"
        resolved_count=$((resolved_count + 1))
        progress=1
      fi
    done
  done
  if [[ "$resolved_count" -lt "$total_nodes" ]]; then
    err "dependsOn graph contains a cycle (could not topologically order all nodes)"
  fi
fi

# ---- release-packet coverage (IMP-006 / Gate G101) -------------------------
# When rootOutcome.targetReleasePacket names a phase AND that phase's
# features.md is reachable under the repo-root, every delivery=required feature
# MUST be covered by some delivery-type node's coversFeatures[]. This is the
# compile-time twin of release-delivery-reconciliation-guard.sh: it catches an
# under-scoped DAG (a promised required feature with no delivery node) BEFORE
# execution. When features.md is not reachable (e.g. it lives in a supporting
# repo the lint cannot see, or the source-repo framework-validate run), the
# convergence-time guard remains the backstop, so this stays informational.
TARGET_PHASE="$(jq -r '.rootOutcome.targetReleasePacket // ""' "$SCENARIO")"
if [[ -n "$TARGET_PHASE" ]]; then
  FEATURES_MD="$ROOT/docs/releases/$TARGET_PHASE/features.md"
  if [[ -f "$FEATURES_MD" ]]; then
    REQUIRED_FEATURES=()
    while IFS= read -r required_feature; do
      REQUIRED_FEATURES[${#REQUIRED_FEATURES[@]}]="$required_feature"
    done < <(grep -oE 'bubbles:feature[^>]*' "$FEATURES_MD" 2>/dev/null |
      grep -E 'delivery=required' |
      grep -oE 'id=[^[:space:]>]+' | sed -E 's/^id=//')
    COVERED_FEATURES=()
    while IFS= read -r covered_feature; do
      COVERED_FEATURES[${#COVERED_FEATURES[@]}]="$covered_feature"
    done < <(jq -r '[ .nodes[]? | select(.type=="delivery") | (.coversFeatures // [])[] ] | .[]' "$SCENARIO" 2>/dev/null || true)
    for feat in "${REQUIRED_FEATURES[@]}"; do
      [[ -n "$feat" ]] || continue
      if ! in_list "$feat" "${COVERED_FEATURES[@]}"; then
        err "rootOutcome.targetReleasePacket '$TARGET_PHASE': required feature '$feat' is not covered by any delivery node's coversFeatures[] (Gate G101 — a release-phase scenario MUST cover every required feature with a delivery node)"
      fi
    done
  else
    info "rootOutcome.targetReleasePacket '$TARGET_PHASE': features.md not reachable at $FEATURES_MD; release coverage will be enforced at convergence by release-delivery-reconciliation-guard.sh (Gate G101)"
  fi
fi

if [[ "$FAILED" -ne 0 ]]; then
  exit 1
fi

info "OK (scenario '$SCEN_ID': $NODE_COUNT node(s), $REPO_COUNT repo(s) validated)"
exit 0
