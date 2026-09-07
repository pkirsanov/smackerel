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
# BACKWARD-COMPATIBLE: genuinely absent DAG signals remain no-ops. Incomplete or
# malformed DAG signals are findings: report posture warns with exit 0, while
# block posture refuses with exit 1. No --skip/--force bypass.
#
# Usage:
#   bash bubbles/scripts/plan-dependency-depth-guard.sh <feature-dir>
#
# Exit codes:
#   0  clean / not-applicable / advisory finding
#   1  a dependency-graph horizontal-plan violation under block posture
#   2  usage / runtime error
set -euo pipefail
export LC_ALL=C

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

# ---------------------------------------------------------------------------
# Enforcement posture. The existing contract is report-by-default and becomes
# blocking only through the explicit project setting.
# ---------------------------------------------------------------------------
posture="report"
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
  posture="block"
fi

signal_finding() {
  printf '[plan-dependency-depth-guard] INCOMPLETE DAG SIGNAL — %s\n' "$1" >&2
  if [[ "$posture" == "block" ]]; then
    return 1
  fi
  echo "[plan-dependency-depth-guard] report only (exit 0); block posture would refuse this incomplete signal." >&2
  return 0
}

# certification.scopeProgress is canonical whenever it is non-null. The
# deprecated top-level field is a compatibility fallback only when canonical
# data is absent or null. execution.scopeProgress is never an authority source.
sp="$(jq -c '
  if (((.certification | type) == "object")
      and (.certification | has("scopeProgress"))
      and (.certification.scopeProgress != null)) then
    .certification.scopeProgress
  elif (has("scopeProgress") and (.scopeProgress != null)) then
    .scopeProgress
  else
    []
  end
' "$state_file")"

# scopeProgress has two legitimate shapes in the wild: the per-scope ARRAY
# ([{scope,scopeDir,dependsOn}, ...]) this DAG analysis is written for, and the
# counts-summary OBJECT ({total,done,inProgress,notStarted}) that single-file
# scopes.md plans carry. Only the array form carries a DAG signal.
#
# The object form MUST be treated as "DAG signal absent" BEFORE any length or
# iteration, and the check has to be an explicit type test rather than a length
# test: `jq length` on an object returns its KEY COUNT (4 for the counts form),
# so a length-only guard reads a signal that is not there, falls through, and
# then `.[] | .dependsOn` iterates the counts VALUES and dies with
# "Cannot index number with string \"dependsOn\"" (jq exit 5). Under a block
# posture that crash surfaced as a substantive verdict — a single-scope plan was
# told every consumer-visible scope sat behind >=3 foundation scopes, which is
# arithmetically impossible — so the failure mode was not a loud crash but a
# false, confidently-worded BLOCK. Type-check first; the documented contract
# above already says a single-file scopes.md plan is a NO-OP here.
sp_type="$(printf '%s' "$sp" | jq -r 'type')"
if [[ "$sp_type" != "array" ]]; then
  echo "[plan-dependency-depth-guard] scopeProgress is '$sp_type', not the per-scope array — no-op (position guard covers this)"
  exit 0
fi

sp_len="$(printf '%s' "$sp" | jq 'length')"
if [[ "$sp_len" -eq 0 ]]; then
  echo "[plan-dependency-depth-guard] no scopeProgress in $feature_dir — no-op (position guard covers this)"
  exit 0
fi

# Every element must be an object; a malformed array cannot be indexed either.
if ! printf '%s' "$sp" | jq -e 'all(type == "object")' >/dev/null 2>&1; then
  signal_finding "scopeProgress entries are not all objects"
  exit $?
fi

# A present per-scope registry has a closed dependency-field shape. Establish
# this before counting edges so malformed dependsOn values cannot masquerade as
# an absent DAG.
if ! printf '%s' "$sp" | jq -e '
  all(.[ ];
    (.dependsOn == null)
    or ((.dependsOn | type) == "array"
        and all(.dependsOn[];
          ((type == "string") and ((gsub("^\\s+|\\s+$"; "") | length) > 0))
          or (type == "number"))))
' >/dev/null 2>&1; then
  signal_finding "dependsOn must be an array of nonblank string or finite numeric aliases"
  exit $?
fi

# Every DAG node needs a usable local identity. Match the closed identity rule
# used by scope-universe-resolver.py: prefer a nonblank string scopeId, then
# fall back to a positive integral number or nonblank string legacy scope.
# Fractional and non-positive numbers are not scope identities.
if ! printf '%s' "$sp" | jq -e '
  def trimmed: gsub("^\\s+|\\s+$"; "");
  all(.[];
    ((.scopeId | type) == "string" and ((.scopeId | trimmed | length) > 0))
    or ((.scope | type) == "number" and .scope > 0 and (.scope | floor) == .scope)
    or ((.scope | type) == "string" and ((.scope | trimmed | length) > 0)))
' >/dev/null 2>&1; then
  signal_finding "not every scope has a usable scopeId/scope identity"
  exit $?
fi

# Establish full DAG applicability before validating graph-only identity
# invariants. Duplicate canonical IDs and ambiguous aliases matter only when
# dependency tokens will be resolved into a complete per-scope graph.
has_edges="$(printf '%s' "$sp" | jq '[.[] | (.dependsOn // []) | length] | add // 0')"
if [[ "$has_edges" -eq 0 ]]; then
  echo "[plan-dependency-depth-guard] no dependsOn edges in $feature_dir — no-op (position guard covers ordering)"
  exit 0
fi

# Build one deterministic registry before constructing the graph maps. A
# canonical scopeId remains the node key when present. The valid legacy scope
# is only an alias. The closed alias projection mirrors scope-universe-resolver:
# canonical ID, numeric/string legacy scope, zero-padded numeric scope,
# normalized scopeDir, its basename, and normalized scopeDir/scope.md.
registry="$(printf '%s' "$sp" | jq -c '
  def trimmed: gsub("^\\s+|\\s+$"; "");
  def scope_identity:
    if ((.scopeId | type) == "string" and ((.scopeId | trimmed | length) > 0)) then (.scopeId | trimmed)
    elif ((.scope | type) == "number" and .scope > 0 and (.scope | floor) == .scope) then (.scope | tostring)
    elif ((.scope | type) == "string" and ((.scope | trimmed | length) > 0)) then (.scope | trimmed)
    else empty
    end;
  def scope_aliases($canonical):
    ([ $canonical ]
     + (if ((.scope | type) == "number" and .scope > 0 and (.scope | floor) == .scope)
        then [(.scope | tostring), (if .scope < 10 then "0" + (.scope | tostring) else (.scope | tostring) end)]
        elif ((.scope | type) == "string" and ((.scope | trimmed | length) > 0))
        then [(.scope | trimmed)]
        else [] end)
     + (if ((.scopeId | type) == "string" and ((.scopeId | trimmed | length) > 0))
        then [(.scopeId | trimmed)]
        else [] end)
      + (if ((.scopeDir | type) == "string" and ((.scopeDir | trimmed | length) > 0))
        then ((.scopeDir | trimmed | sub("/+$"; "")) as $dir
            | [$dir, ($dir | split("/") | last), ($dir + "/scope.md")])
        else [] end))
    | unique;
  [ .[]
    | scope_identity as $canonical
    | {id: $canonical,
      scopeDir: (if ((.scopeDir | type) == "string")
            then (.scopeDir | trimmed | sub("/+$"; "")) else "" end),
       deps: (.dependsOn // []),
       aliases: scope_aliases($canonical)} ] as $nodes
    | ($nodes
     | sort_by(.id)
     | group_by(.id)
     | map(select(length > 1)
          | {canonical: .[0].id, recordCount: length})) as $duplicates
    | ([ $nodes[] as $node
       | $node.aliases[]
       | {alias: ., canonical: $node.id} ]
      | sort_by(.alias, .canonical)) as $alias_rows
    | ($alias_rows
     | group_by(.alias)
     | map({alias: .[0].alias, canonicals: ([.[].canonical] | unique)})
     | map(select(.canonicals | length > 1))) as $collisions
  | {nodes: $nodes,
      aliases: (reduce $alias_rows[] as $row
             ({}; . + {($row.alias): $row.canonical})),
      duplicate: (if ($duplicates | length) == 0 then null else $duplicates[0] end),
     collision: (if ($collisions | length) == 0 then null else $collisions[0] end)}
')"

duplicate_canonical="$(printf '%s' "$registry" | jq -r '.duplicate // empty | "canonical scope \"\(.canonical)\" is declared by \(.recordCount) records"')"
if [[ -n "$duplicate_canonical" ]]; then
  signal_finding "duplicate canonical scope identity — $duplicate_canonical"
  exit $?
fi

alias_collision="$(printf '%s' "$registry" | jq -r '.collision // empty | "alias \"\(.alias)\" identifies canonical scopes: \(.canonicals | join(", "))"')"

# Canonicalize every claimed body without realpath/readlink -f. Reject absolute
# and dot-segment scopeDir values, every symlink component, non-regular bodies,
# physical escapes, and two records claiming the same physical body.
feature_physical="$(cd -P -- "$feature_dir" 2>/dev/null && pwd -P)" || {
  echo "plan-dependency-depth-guard: cannot canonicalize feature dir: $feature_dir" >&2
  exit 2
}
physical_claims=$'\n'
body_rows=""
while IFS=$'\t' read -r scope_id scope_dir; do
  if [[ -z "$scope_dir" || "$scope_dir" == /* || "/$scope_dir/" == *"/../"* || "/$scope_dir/" == *"/./"* ]]; then
    signal_finding "scope $scope_id has an invalid or escaping scopeDir: $scope_dir"
    exit $?
  fi
  cursor="$feature_physical"
  remainder="$scope_dir"
  path_invalid=0
  while [[ -n "$remainder" ]]; do
    if [[ "$remainder" == */* ]]; then
      component="${remainder%%/*}"
      remainder="${remainder#*/}"
    else
      component="$remainder"
      remainder=""
    fi
    if [[ -z "$component" || "$component" == "." || "$component" == ".." ]]; then
      path_invalid=1
      break
    fi
    cursor="$cursor/$component"
    if [[ -L "$cursor" ]]; then
      path_invalid=1
      break
    fi
  done
  body_candidate="$cursor/scope.md"
  if [[ "$path_invalid" -eq 1 || -L "$body_candidate" || ! -f "$body_candidate" || ! -r "$body_candidate" ]]; then
    if [[ -n "$alias_collision" ]]; then
      signal_finding "ambiguous scope alias — $alias_collision"
      exit $?
    fi
    signal_finding "scope $scope_id does not claim a readable regular non-symlink scopeDir/scope.md"
    exit $?
  fi
  body_parent="$(cd -P -- "$(dirname "$body_candidate")" 2>/dev/null && pwd -P)" || {
    signal_finding "scope $scope_id body parent cannot be canonicalized"
    exit $?
  }
  physical_body="$body_parent/$(basename "$body_candidate")"
  case "$physical_body" in
    "$feature_physical"/*) ;;
    *)
      signal_finding "scope $scope_id body escapes the feature directory"
      exit $?
      ;;
  esac
  case "$physical_claims" in
    *$'\n'"$physical_body"$'\n'*)
      signal_finding "multiple scope records claim physical body $physical_body"
      exit $?
      ;;
  esac
  physical_claims="${physical_claims}${physical_body}"$'\n'
  body_rows="${body_rows}${scope_id}"$'\t'"${physical_body}"$'\n'
done < <(printf '%s' "$registry" | jq -r '.nodes[] | [.id, .scopeDir] | @tsv')

if [[ -n "$alias_collision" ]]; then
  signal_finding "ambiguous scope alias — $alias_collision"
  exit $?
fi

# Resolve every dependency token through the complete closed alias registry
# exactly once. All subsequent validation and closure analysis consumes only
# these canonical targets.
resolved_graph="$(printf '%s' "$registry" | jq -c '
  .aliases as $aliases
  | {nodes: [ .nodes[] as $node
      | {id: $node.id,
         deps: [ $node.deps[]
           | (tostring | gsub("^\\s+|\\s+$"; "")) as $token
           | {token: $token, target: ($aliases[$token] // null)} ]} ]}
')"

unknown_dependency="$(printf '%s' "$resolved_graph" | jq -r '
  [ .nodes[] as $node
    | $node.deps[]
    | select(.target == null)
    | {scope: $node.id, token: .token} ]
  | sort_by([.scope, .token])
  | .[0] // empty
  | if type == "object" then "scope \(.scope) depends on unknown alias \(.token)" else empty end
')"
if [[ -n "$unknown_dependency" ]]; then
  signal_finding "$unknown_dependency"
  exit $?
fi

self_dependency="$(printf '%s' "$resolved_graph" | jq -r '
  [ .nodes[] as $node
    | $node.deps[]
    | select(.target == $node.id)
    | $node.id ]
  | sort
  | .[0] // empty
')"
if [[ -n "$self_dependency" ]]; then
  signal_finding "self dependency on canonical scope $self_dependency"
  exit $?
fi

duplicate_effective_dependency="$(printf '%s' "$resolved_graph" | jq -r '
  [ .nodes[]
    | .id as $scope
    | (.deps | sort_by(.target) | group_by(.target)[]
       | select(length > 1)
       | {scope: $scope, target: .[0].target}) ]
  | sort_by([.scope, .target])
  | .[0] // empty
  | if type == "object" then "scope \(.scope) has duplicate effective dependency target \(.target)" else empty end
')"
if [[ -n "$duplicate_effective_dependency" ]]; then
  signal_finding "$duplicate_effective_dependency"
  exit $?
fi

depmap="$(printf '%s' "$resolved_graph" | jq -c '
  reduce .nodes[] as $node
    ({}; . + {($node.id): [$node.deps[].target]})
')"

# Fixed-point expansion is iterative in graph depth and remains safe for deep
# graphs. A canonical node appearing in its own transitive closure proves a
# cycle of length two or greater because direct self edges were rejected above.
dependency_cycle="$(jq -n -r --argjson dep "$depmap" '
  ($dep | with_entries(.value |= unique)) as $initial
  | def grow($state):
      (reduce ($dep | keys_unsorted[]) as $node
        ({closure: $state.closure, frontier: {}};
         ([ $state.frontier[$node][] as $direct
            | $dep[$direct][]? ]
          | unique
          | . - $state.closure[$node]) as $next
         | .frontier[$node] = $next
         | .closure[$node] = ((.closure[$node] + $next) | unique))) as $next_state
      | if ([ $next_state.frontier[] | length ] | add // 0) == 0
        then $next_state.closure
        else grow($next_state)
        end;
    grow({closure: $initial, frontier: $initial}) as $closures
  | [ $dep | keys[] as $node
    | select(($closures[$node] | index($node)) != null)
    | $node ]
  | sort
  | if length == 0 then empty else join(", ") end
')"
if [[ -n "$dependency_cycle" ]]; then
  signal_finding "dependency cycle includes canonical scopes: $dependency_cycle"
  exit $?
fi

# ---------------------------------------------------------------------------
# Classify each scope consumer|foundation by its scope.md body (same structural
# consumer-surface signal as vertical-delivery-plan-guard). Build classmap JSON.
# ---------------------------------------------------------------------------
consumer_re='(/api/|GET /|POST /|PUT /|DELETE /|PATCH /|\.route\(|dashboard|frontend|web page|webpage|navigation|breadcrumb|deep link|WebSocket|CLI command|operator surface|user interface|admin portal)'

classification_rows=""
while IFS=$'\t' read -r scope_id body; do
  if grep -qiE "$consumer_re" "$body" 2>/dev/null; then
    cls="consumer"
  else
    cls="foundation"
  fi
  classification_rows="${classification_rows}${scope_id}"$'\t'"${cls}"$'\n'
done < <(printf '%s' "$body_rows")

# Materialize each complete map once. The former loop launched one jq process
# per scope and rebuilt two growing JSON objects on every iteration.
classmap="$(printf '%s' "$classification_rows" | jq -Rn '
  reduce (inputs | select(length > 0) | split("\t")) as $row
    ({}; . + {($row[0]): $row[1]})
')"

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
  def closure_map:
    ($dep | with_entries(.value |= unique)) as $initial
    | def grow($state):
        (reduce ($dep | keys_unsorted[]) as $node
          ({closure: $state.closure, frontier: {}};
           ([ $state.frontier[$node][] as $direct
              | $dep[$direct][]? ]
            | unique
            | . - $state.closure[$node]) as $next
           | .frontier[$node] = $next
           | .closure[$node] = ((.closure[$node] + $next) | unique))) as $next_state
        | if ([ $next_state.frontier[] | length ] | add // 0) == 0
          then $next_state.closure
          else grow($next_state)
          end;
      grow({closure: $initial, frontier: $initial});
  closure_map as $closures
  |
  ([ $cls | to_entries[] | select(.value == "consumer") | .key ]) as $consumers
  | if ($consumers | length) == 0 then {noConsumer: true}
    else
      ([ $consumers[]
         | { scope: .,
           fdeps: ([ $closures[.][] | select(($cls[.] // "") == "foundation") ] | unique | length) } ]
         | sort_by([.fdeps, .scope])) as $rows
        | $rows[0] as $earliest
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

if [[ "$posture" == "block" ]]; then
  exit 1
fi
echo "[plan-dependency-depth-guard] report only (exit 0). Set planDependencyDepthGuard to block in .github/bubbles-project.yaml to enforce." >&2
exit 0
