#!/usr/bin/env bash
# Plan Dependency-Depth Guard (IMP-100 Phase 4 / IMP-022 SCOPE-3 + SCOPE-4)
# ---------------------------------------------------------------------------
# Complements the position-based vertical-delivery-plan-guard.sh with a
# DEPENDENCY-GRAPH horizontal-layer analysis. The position guard flags a plan
# whose first consumer-visible scope is deferred behind >=3 LEADING (by number)
# foundation scopes. This guard catches the case that position misses: a consumer
# scope that is early-NUMBERED but DEEP in the DAG — it transitively `dependsOn`
# a stack of foundation scopes, so it is not actually deliverable early (SCOPE-3).
#
# Consumer-timing rule (SCOPE-4): a plan is horizontal only when EVERY
# consumer-visible scope transitively requires >=3 foundation scopes. If ANY
# consumer needs fewer (an early usable increment exists), the plan passes —
# genuine last-mile canaries and early vertical slices are preserved.
#
# It reads the `dependsOn` DAG + per-scope `scopeDir` from state.json and
# classifies each scope by its scope.md body using the SAME consumer-surface
# signal as vertical-delivery-plan-guard (reuse-first, structural not keyword).
#
# Scope: requires the per-scope-directory layout (scopeProgress[].scopeDir +
# scopes/NN/scope.md) — the standard for multi-scope plans where DAG layering
# matters. A single-file scopes.md plan, a plan with no scopeProgress, or a plan
# with no dependency edges is a NO-OP (the position guard covers those).
#
# BACKWARD-COMPATIBLE: no-op unless the DAG signal is fully present. ADVISORY by
# default (warn + exit 0); blocks (exit 1) only when `.github/bubbles-project.yaml`
# sets `planDependencyDepthGuard` to block. No --skip/--force bypass.
#
# Usage:
#   bash bubbles/scripts/plan-dependency-depth-guard.sh <feature-dir>
#
# Exit codes:
#   0  clean / not-applicable / advisory finding
#   1  a dependency-graph horizontal-plan violation under block posture
#   2  usage / runtime error
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: plan-dependency-depth-guard.sh <feature-dir>

DAG-based horizontal-plan detection: flags a plan in which EVERY consumer-visible
scope transitively dependsOn >=3 foundation scopes (no early usable increment).
Requires the per-scope-directory layout with state.json dependsOn + scopeDir.
No-op otherwise. Advisory (exit 0) by default; blocks (exit 1) only when
.github/bubbles-project.yaml sets planDependencyDepthGuard to block.
EOF
}

feature_dir="${1:-}"
if [[ -z "$feature_dir" ]]; then
  usage >&2
  exit 2
fi
if [[ ! -d "$feature_dir" ]]; then
  echo "plan-dependency-depth-guard: feature dir not found: $feature_dir" >&2
  exit 2
fi

state_file="$feature_dir/state.json"
if [[ ! -f "$state_file" ]]; then
  echo "[plan-dependency-depth-guard] no state.json in $feature_dir — no-op"
  exit 0
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "[plan-dependency-depth-guard] WARN-and-skip — jq not installed (exit 0)." >&2
  exit 0
fi
if ! jq -e 'type == "object"' "$state_file" >/dev/null 2>&1; then
  echo "plan-dependency-depth-guard: malformed or non-object JSON: $state_file" >&2
  exit 2
fi

# scopeProgress (top-level canonical, or under certification for completed specs).
sp="$(jq -c '(.scopeProgress // .certification.scopeProgress // [])' "$state_file")"
sp_len="$(printf '%s' "$sp" | jq 'length')"
if [[ "$sp_len" -eq 0 ]]; then
  echo "[plan-dependency-depth-guard] no scopeProgress in $feature_dir — no-op (position guard covers this)"
  exit 0
fi

# Require at least one dependency edge; otherwise there is no DAG to analyze.
has_edges="$(printf '%s' "$sp" | jq '[.[] | (.dependsOn // []) | length] | add // 0')"
if [[ "$has_edges" -eq 0 ]]; then
  echo "[plan-dependency-depth-guard] no dependsOn edges in $feature_dir — no-op (position guard covers ordering)"
  exit 0
fi

# Require the per-scope-directory layout for every scope; else no-op (conservative).
missing_body=0
while IFS= read -r scope_dir; do
  if [[ -z "$scope_dir" || "$scope_dir" == "null" || ! -f "$feature_dir/$scope_dir/scope.md" ]]; then
    missing_body=1
    break
  fi
done < <(printf '%s' "$sp" | jq -r '.[].scopeDir // ""')
if [[ "$missing_body" -eq 1 ]]; then
  echo "[plan-dependency-depth-guard] not every scope has a readable scopeDir/scope.md in $feature_dir — no-op (conservative)"
  exit 0
fi

# ---------------------------------------------------------------------------
# Enforcement mode.
# ---------------------------------------------------------------------------
mode="advisory"
project_config=""
for candidate in \
  "$feature_dir/.github/bubbles-project.yaml" \
  ".github/bubbles-project.yaml" \
  "$(git -C "$feature_dir" rev-parse --show-toplevel 2>/dev/null)/.github/bubbles-project.yaml"; do
  if [[ -n "$candidate" && -f "$candidate" ]]; then
    project_config="$candidate"
    break
  fi
done
if [[ -n "$project_config" ]] && grep -qE '^[[:space:]]*planDependencyDepthGuard:[[:space:]]*block[[:space:]]*$' "$project_config"; then
  mode="block"
fi

# ---------------------------------------------------------------------------
# Classify each scope consumer|foundation by its scope.md body (same structural
# consumer-surface signal as vertical-delivery-plan-guard). Build classmap JSON.
# ---------------------------------------------------------------------------
consumer_re='(/api/|GET /|POST /|PUT /|DELETE /|PATCH /|\.route\(|dashboard|frontend|web page|webpage|navigation|breadcrumb|deep link|WebSocket|CLI command|operator surface|user interface|admin portal)'

classmap="{}"
depmap="{}"
while IFS=$'\t' read -r scope_num scope_dir deps_json; do
  [[ -n "$scope_num" ]] || continue
  body="$feature_dir/$scope_dir/scope.md"
  if grep -qiE "$consumer_re" "$body" 2>/dev/null; then
    cls="consumer"
  else
    cls="foundation"
  fi
  classmap="$(printf '%s' "$classmap" | jq --arg k "$scope_num" --arg v "$cls" '. + {($k): $v}')"
  depmap="$(printf '%s' "$depmap" | jq --arg k "$scope_num" --argjson d "$deps_json" '. + {($k): $d}')"
done < <(printf '%s' "$sp" | jq -r '.[] | [(.scope|tostring), (.scopeDir // ""), ((.dependsOn // []) | tostring)] | @tsv')

# ---------------------------------------------------------------------------
# For each consumer scope, count DISTINCT foundation scopes in its transitive
# dependency closure. The plan is horizontal only when the LEAST-blocked consumer
# still requires >= THRESHOLD foundations (no early usable increment).
# ---------------------------------------------------------------------------
THRESHOLD=3
analysis="$(jq -n \
  --argjson dep "$depmap" \
  --argjson cls "$classmap" \
  --argjson threshold "$THRESHOLD" '
  def tdeps($start):
    def grow($acc):
      (($acc + ($acc | map($dep[(.|tostring)] // []) | add)) | unique) as $next
      | if ($next | length) == ($acc | length) then $acc else grow($next) end;
    grow($dep[($start|tostring)] // []);
  ([ $cls | to_entries[] | select(.value == "consumer") | (.key | tonumber) ]) as $consumers
  | if ($consumers | length) == 0 then {noConsumer: true}
    else
      ([ $consumers[]
         | { scope: .,
             fdeps: ([ tdeps(.)[] | select(($cls[(.|tostring)] // "") == "foundation") ] | unique | length) } ]) as $rows
      | ($rows | min_by(.fdeps)) as $earliest
      | { noConsumer: false,
          minFdeps: $earliest.fdeps,
          earliestConsumer: $earliest.scope,
          horizontal: ($earliest.fdeps >= $threshold),
          rows: $rows }
    end
')"

no_consumer="$(printf '%s' "$analysis" | jq -r '.noConsumer')"
if [[ "$no_consumer" == "true" ]]; then
  echo "[plan-dependency-depth-guard] $feature_dir has no consumer-visible scope — no-op (position guard owns the no-consumer case)"
  exit 0
fi

horizontal="$(printf '%s' "$analysis" | jq -r '.horizontal')"
min_fdeps="$(printf '%s' "$analysis" | jq -r '.minFdeps')"
earliest="$(printf '%s' "$analysis" | jq -r '.earliestConsumer')"

if [[ "$horizontal" != "true" ]]; then
  echo "[plan-dependency-depth-guard] OK — an early usable increment exists (consumer scope $earliest transitively depends on only $min_fdeps foundation scope(s), below the $THRESHOLD threshold)."
  exit 0
fi

{
  echo "[plan-dependency-depth-guard] DEPENDENCY-GRAPH HORIZONTAL PLAN in $feature_dir:"
  echo "  Every consumer-visible scope is deferred behind a foundation stack in the dependsOn DAG."
  echo "  The least-blocked consumer (scope $earliest) still transitively depends on $min_fdeps foundation scope(s) (threshold $THRESHOLD)."
  printf '%s' "$analysis" | jq -r '.rows[] | "  - consumer scope \(.scope): transitively depends on \(.fdeps) foundation scope(s)"'
  echo "  Remediation: restructure the DAG so an EARLY consumer scope depends on only its"
  echo "  minimum backing foundation (a runnable vertical slice), instead of stacking the"
  echo "  whole foundation layer ahead of every consumer. A genuine last-mile canary is fine"
  echo "  as long as another consumer delivers an early usable increment."
} >&2

if [[ "$mode" == "block" ]]; then
  exit 1
fi
echo "[plan-dependency-depth-guard] advisory only (exit 0). Set planDependencyDepthGuard to block in .github/bubbles-project.yaml to enforce." >&2
exit 0
