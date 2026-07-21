#!/usr/bin/env bash
# Expand→Migrate→Contract Guard (IMP-100 Phase 4 / IMP-026 SCOPE-2)
# ---------------------------------------------------------------------------
# Mechanizes the safe wide-refactor pattern. A wide refactor (rename/move/replace
# a widely-consumed route, symbol, contract, or schema) is safe ONLY when it is
# staged as EXPAND (add the new alongside the old) → MIGRATE (move each consumer
# batch onto the new) → CONTRACT (remove the old), where every migrate batch
# depends on the expand and the contract depends on ALL migrates. Deleting the
# old before every consumer has migrated is the classic breakage.
#
# OPT-IN: a plan declares `"refactorPattern": "expand-migrate-contract"` in
# state.json and tags each scope with `"refactorPhase": "expand"|"migrate"|
# "contract"` in its `.scopeProgress[]` entry. The guard then checks the DAG
# (reusing the existing `dependsOn` edges) for the four structural invariants:
#   1. every scope carries a valid refactorPhase, and at least one expand, one
#      migrate, and one contract scope exist;
#   2. every MIGRATE scope dependsOn at least one EXPAND scope;
#   3. every CONTRACT scope dependsOn ALL migrate scopes;
#   4. no EXPAND scope dependsOn a migrate/contract scope.
#
# Reuse-first: it composes with (does not replace) G043 (consumer_trace_gate) /
# G044 (comprehensive_regression_gate) — those prove consumers are updated; this
# proves the removal is SEQUENCED after every migration. There is NO
# integration-branch escape hatch.
#
# BACKWARD-COMPATIBLE: a plan without `refactorPattern: expand-migrate-contract`
# is a no-op (exit 0). ADVISORY by default (warn + exit 0); blocks (exit 1) only
# when `.github/bubbles-project.yaml` sets `expandMigrateContractGuard` to block.
# There is no --skip/--force bypass.
#
# Usage:
#   bash bubbles/scripts/expand-migrate-contract-guard.sh <feature-dir>
#
# Exit codes:
#   0  clean / not-applicable / advisory finding
#   1  a structural violation under block posture
#   2  usage / runtime error
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: expand-migrate-contract-guard.sh <feature-dir>

For a plan that opts in with state.json "refactorPattern": "expand-migrate-contract"
(each scope tagged "refactorPhase": expand|migrate|contract), verifies the
expand→migrate→contract dependency structure. No-op for other plans. Advisory
(exit 0 + warning) by default; blocks (exit 1) only when .github/bubbles-project.yaml
sets expandMigrateContractGuard to block.
EOF
}

feature_dir="${1:-}"
if [[ -z "$feature_dir" ]]; then
  usage >&2
  exit 2
fi
if [[ ! -d "$feature_dir" ]]; then
  echo "expand-migrate-contract-guard: feature dir not found: $feature_dir" >&2
  exit 2
fi

state_file="$feature_dir/state.json"
if [[ ! -f "$state_file" ]]; then
  echo "[expand-migrate-contract-guard] no state.json in $feature_dir — nothing to check"
  exit 0
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "[expand-migrate-contract-guard] WARN-and-skip — jq not installed; cannot parse $state_file (exit 0)." >&2
  exit 0
fi

if ! jq -e 'type == "object"' "$state_file" >/dev/null 2>&1; then
  echo "expand-migrate-contract-guard: malformed or non-object JSON: $state_file" >&2
  exit 2
fi

# Opt-in gate: only plans that DECLARE the pattern are checked.
pattern="$(jq -r '.refactorPattern // ""' "$state_file")"
if [[ "$pattern" != "expand-migrate-contract" ]]; then
  echo "[expand-migrate-contract-guard] $feature_dir does not declare refactorPattern: expand-migrate-contract — no-op"
  exit 0
fi

# ---------------------------------------------------------------------------
# Resolve enforcement mode (advisory | block). Advisory unless the repo opts in.
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
if [[ -n "$project_config" ]] && grep -qE '^[[:space:]]*expandMigrateContractGuard:[[:space:]]*block[[:space:]]*$' "$project_config"; then
  mode="block"
fi

# ---------------------------------------------------------------------------
# Compute structural violations over the scopeProgress DAG. scopeProgress lives
# top-level (canonical) or under .certification (completed specs); support both.
# ---------------------------------------------------------------------------
violations="$(jq -r '
  (.scopeProgress // .certification.scopeProgress // []) as $sp
  | ($sp | map(select(.refactorPhase == "expand")   | .scope)) as $expand
  | ($sp | map(select(.refactorPhase == "migrate")  | .scope)) as $migrate
  | ($sp | map(select(.refactorPhase == "contract") | .scope)) as $contract
  | [
      # 1a. every scope must carry a valid refactorPhase
      ( $sp[]
        | (.refactorPhase // "") as $p
        | select($p != "expand" and $p != "migrate" and $p != "contract")
        | "scope \(.scope) (\(.name // "?")) has invalid/missing refactorPhase \"\($p)\" (expected expand|migrate|contract)" ),
      # 1b. at least one of each phase
      ( if ($expand   | length) == 0 then "no scope tagged refactorPhase: expand"   else empty end ),
      ( if ($migrate  | length) == 0 then "no scope tagged refactorPhase: migrate"  else empty end ),
      ( if ($contract | length) == 0 then "no scope tagged refactorPhase: contract" else empty end ),
      # 2. every migrate depends on >= 1 expand
      ( $sp[]
        | select(.refactorPhase == "migrate")
        | select([ (.dependsOn // [])[] | select(. as $d | $expand | index($d)) ] | length == 0)
        | "migrate scope \(.scope) (\(.name // "?")) does not depend on any expand scope" ),
      # 3. every contract depends on ALL migrate scopes
      ( $sp[]
        | select(.refactorPhase == "contract")
        | . as $c
        | ($migrate - ($c.dependsOn // [])) as $missing
        | select($missing | length > 0)
        | "contract scope \($c.scope) (\($c.name // "?")) must depend on ALL migrate scopes; missing dependsOn: \($missing | map(tostring) | join(","))" ),
      # 4. no expand depends on a migrate/contract scope
      ( $sp[]
        | select(.refactorPhase == "expand")
        | . as $e
        | select([ (.dependsOn // [])[] | select(. as $d | ($migrate + $contract) | index($d)) ] | length > 0)
        | "expand scope \($e.scope) (\($e.name // "?")) must not depend on a migrate/contract scope" )
    ]
  | .[]
' "$state_file")"

if [[ -z "$violations" ]]; then
  echo "[expand-migrate-contract-guard] OK — $feature_dir follows expand→migrate→contract (every migrate depends on expand; contract depends on all migrates)."
  exit 0
fi

{
  echo "[expand-migrate-contract-guard] EXPAND→MIGRATE→CONTRACT violations in $feature_dir:"
  while IFS= read -r v; do
    [[ -n "$v" ]] && echo "  - $v"
  done <<< "$violations"
  echo "  Remediation: stage the refactor as expand (add new alongside old) → migrate"
  echo "  (move each consumer batch, dependsOn the expand) → contract (remove old, dependsOn"
  echo "  ALL migrates). Never delete the old contract before every consumer has migrated."
  echo "  This composes with G043 (consumer trace) + G044 (regression). No integration-branch bypass."
} >&2

if [[ "$mode" == "block" ]]; then
  exit 1
fi
echo "[expand-migrate-contract-guard] advisory only (exit 0). Set expandMigrateContractGuard to block in .github/bubbles-project.yaml to enforce." >&2
exit 0
