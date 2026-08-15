#!/usr/bin/env bash
# scenario-compile-lint-selftest.sh — hermetic selftest for scenario-compile-lint.sh.
#
# Runs the lint against the REAL repo root (so modes.yaml / agent-capabilities.yaml
# resolve and the requiresTopLevelRuntime forbidden set is derived correctly), with
# temp scenario JSON fixtures for each case:
#   1.  Clean cross-repo DAG (diagnostic→planning→delivery→verify→action→ongoing-ops) → exit 0
#   2.  Node mode = autonomous-goal (fan-out)            → exit 1
#   3.  Node mode = iterate (fan-out)                    → exit 1
#   4.  action node missing approvalRequired             → exit 1
#   5.  action node missing opsPacket                    → exit 1
#   6.  action node missing riskClass                    → exit 1
#   7.  ongoing-ops node missing opsPacket               → exit 1
#   8.  dependsOn references unknown node                → exit 1
#   9.  cyclic dependsOn                                 → exit 1
#   10. duplicate node id                                → exit 1
#   11. node with neither mode nor agent                 → exit 1
#   12. node with both mode and agent                    → exit 1
#   13. node repo not in repos[]                         → exit 1
#   14. unknown mode                                     → exit 1
#   15. unknown agent                                    → exit 1
#   16. missing rootOutcome.successSignal                → exit 1
#   17. empty rootOutcome.hardConstraints                → exit 1
#   18. --list-forbidden contains all 6 fan-out modes (derived from modes.yaml)
#   19. targetReleasePacket coverage — all required features covered     → exit 0
#   20. targetReleasePacket coverage — a required feature uncovered      → exit 1 (G101)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# The lint validates that every fixture's repositoryRoot IS the canonical Git
# root, so this must resolve to one. A fixed "../.." hop only satisfies that in
# the framework source layout (bubbles/scripts/../.. -> repo root); in a
# downstream install (.github/bubbles/scripts/../.. -> <repo>/.github) it lands
# on a directory that is not a Git root, and every fixture is rejected. Resolve
# it the same way the lint does, keeping the hop as a fallback for a non-Git
# (tarball) install.
if REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null)" && [[ -n "$REPO_ROOT" ]]; then
  REPO_ROOT="$(cd -P -- "$REPO_ROOT" && pwd -P)"
else
  REPO_ROOT="$(cd -P -- "$SCRIPT_DIR/../.." && pwd -P)"
fi
LINT="$SCRIPT_DIR/scenario-compile-lint.sh"

[[ -x "$LINT" ]] || { echo "FAIL: $LINT not executable" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "FAIL: jq required" >&2; exit 1; }

TMP="$(mktemp -d "${HOME}/.bubbles-selftest-scenario.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
F="$TMP/scenario.json"

write_clean() {
  cat > "$F" <<'JSON'
{
  "version": 1,
  "scenarioId": "example-mvp-target-readiness",
  "rootOutcome": {
    "intent": "Product is live and operable on the target environment",
    "successSignal": "Service health endpoint green on the target after deploy",
    "hardConstraints": ["local-target build, not cloud"],
    "failureCondition": "Any node blocked or health check red after deploy"
  },
  "repos": [
    {"id": "product", "role": "product"},
    {"id": "adapter", "role": "deployment-adapter"}
  ],
  "nodes": [
    {"id": "readiness", "type": "diagnostic", "repo": "product", "agent": "bubbles.system-review"},
    {"id": "product-plan", "type": "planning", "repo": "product", "mode": "product-to-planning", "dependsOn": ["readiness"]},
    {"id": "adapter-plan", "type": "planning", "repo": "adapter", "mode": "product-to-planning", "dependsOn": ["readiness"]},
    {"id": "product-deliver", "type": "delivery", "repo": "product", "mode": "full-delivery", "dependsOn": ["product-plan"]},
    {"id": "adapter-deliver", "type": "delivery", "repo": "adapter", "mode": "devops-to-doc", "dependsOn": ["adapter-plan"]},
    {"id": "deploy-verify", "type": "verification", "repo": "product", "mode": "validate-only", "dependsOn": ["product-deliver", "adapter-deliver"]},
    {"id": "deploy", "type": "action", "repo": "adapter", "mode": "devops-to-doc", "opsPacket": "specs/_ops/OPS-deploy-target", "approvalRequired": true, "riskClass": "external_side_effect", "dependsOn": ["deploy-verify"]},
    {"id": "live-ops", "type": "ongoing-ops", "repo": "product", "mode": "stabilize-to-doc", "opsPacket": "specs/_ops/OPS-target-operation", "dependsOn": ["deploy"]}
  ]
}
JSON
  jq --arg root "$REPO_ROOT" --arg session "scenario-selftest" \
    --arg controlPathDigest "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" '
    .repos |= map(. + {repositoryRoot: $root, repositoryAlias: .id})
    | .nodes |= map(. + {
        repositoryResolution: {
          sessionId: $session,
          decisionId: ("rb:" + $session + ":1:node:" + .id),
          controlRevision: 1,
          controlPathDigest: $controlPathDigest,
          authority: "scoped-scenario-node",
          transition: "scoped-override",
          scopeKind: "goal-node",
          scopeId: .id,
          targetKind: "goal-node",
          pathVisibility: "local",
          actionable: true
        }
      })
  ' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
}

assert_pass() {
  local desc="$1"
  if "$LINT" "$F" "$REPO_ROOT" >/dev/null 2>&1; then
    echo "PASS: $desc"
  else
    echo "FAIL: $desc (expected exit 0)"; "$LINT" "$F" "$REPO_ROOT"; exit 1
  fi
}
assert_fail() {
  local desc="$1"
  local rc=0
  "$LINT" "$F" "$REPO_ROOT" >/dev/null 2>&1 || rc=$?
  if [[ $rc -eq 1 ]]; then
    echo "PASS: $desc"
  else
    echo "FAIL: $desc (expected exit 1, got $rc)"; "$LINT" "$F" "$REPO_ROOT"; exit 1
  fi
}

# 1. Clean
write_clean
assert_pass "clean cross-repo DAG passes"

# 2. fan-out mode autonomous-goal
write_clean
jq '(.nodes[] | select(.id=="product-deliver") | .mode) = "autonomous-goal"' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node resolving to autonomous-goal rejected (Gate G064)"

# 3. fan-out mode iterate
write_clean
jq '(.nodes[] | select(.id=="product-deliver") | .mode) = "iterate"' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node resolving to iterate rejected (Gate G064)"

# 4. action node missing approvalRequired
write_clean
jq '(.nodes[] | select(.id=="deploy")) |= del(.approvalRequired)' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "action node missing approvalRequired rejected"

# 5. action node missing opsPacket
write_clean
jq '(.nodes[] | select(.id=="deploy")) |= del(.opsPacket)' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "action node missing opsPacket rejected"

# 6. action node missing riskClass
write_clean
jq '(.nodes[] | select(.id=="deploy")) |= del(.riskClass)' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "action node missing riskClass rejected"

# 7. ongoing-ops node missing opsPacket
write_clean
jq '(.nodes[] | select(.id=="live-ops")) |= del(.opsPacket)' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "ongoing-ops node missing opsPacket rejected"

# 8. dependsOn references unknown node
write_clean
jq '(.nodes[] | select(.id=="product-plan") | .dependsOn) = ["ghost"]' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "dependsOn referencing unknown node rejected"

# 9. cyclic dependsOn (product-plan <-> product-deliver)
write_clean
jq '(.nodes[] | select(.id=="product-plan") | .dependsOn) = ["product-deliver"]' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "cyclic dependsOn rejected"

# 10. duplicate node id
write_clean
jq '(.nodes[] | select(.id=="adapter-plan") | .id) = "product-plan"' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "duplicate node id rejected"

# 11. node with neither mode nor agent
write_clean
jq '(.nodes[] | select(.id=="product-deliver")) |= del(.mode)' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node with neither mode nor agent rejected"

# 12. node with both mode and agent
write_clean
jq '(.nodes[] | select(.id=="product-deliver") | .agent) = "bubbles.implement"' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node with both mode and agent rejected"

# 13. node repo not in repos[]
write_clean
jq '(.nodes[] | select(.id=="readiness") | .repo) = "nonexistent"' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node with undeclared repo rejected"

# 14. unknown mode
write_clean
jq '(.nodes[] | select(.id=="product-deliver") | .mode) = "no-such-mode"' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node with unknown mode rejected"

# 15. unknown agent
write_clean
jq '(.nodes[] | select(.id=="readiness") | .agent) = "bubbles.notanagent"' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node with unknown agent rejected"

# 16. missing rootOutcome.successSignal
write_clean
jq 'del(.rootOutcome.successSignal)' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "missing rootOutcome.successSignal rejected"

# 17. empty rootOutcome.hardConstraints
write_clean
jq '.rootOutcome.hardConstraints = []' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "empty rootOutcome.hardConstraints rejected"

# 18. repository alias missing
write_clean
jq 'del(.repos[0].repositoryAlias)' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "repository declaration missing repositoryAlias rejected"

# 19. repository alias contains a path separator
write_clean
jq '.repos[0].repositoryAlias = "forged/alias"' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "repositoryAlias containing a separator rejected"

# 20. repository aliases are unique
write_clean
jq '.repos[1].repositoryAlias = .repos[0].repositoryAlias' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "duplicate repositoryAlias rejected"

# 21. --list-forbidden derivation contains all 6 fan-out modes
forbidden="$("$LINT" --list-forbidden "$REPO_ROOT" 2>/dev/null || true)"
for m in iterate autonomous-goal autonomous-sprint stochastic-quality-sweep retro-quality-sweep idea-to-release-completion; do
  if ! grep -qx "$m" <<< "$forbidden"; then
    echo "FAIL: --list-forbidden missing '$m'"; echo "$forbidden"; exit 1
  fi
done
echo "PASS: --list-forbidden derives all 6 requiresTopLevelRuntime fan-out modes"

# 22-23. release-packet coverage (IMP-006 / Gate G101)
COVROOT="$TMP/covroot"
mkdir -p "$COVROOT/docs/releases/mvp"
cat > "$COVROOT/docs/releases/mvp/features.md" <<'MD'
# mvp — features
<!-- bubbles:reconciled-packet schemaVersion=1 phase=mvp -->
<!-- bubbles:feature id=auth-real spec=specs/074-auth delivery=required -->
<!-- bubbles:feature id=billing spec=specs/076-billing delivery=required -->
<!-- bubbles:feature id=sso spec=none delivery=deferred-to:v2.0 -->
MD

write_covered() {
  cat > "$F" <<'JSON'
{
  "version": 1,
  "scenarioId": "covered-mvp",
  "rootOutcome": {
    "intent": "MVP delivered",
    "successSignal": "All required MVP features validate-certified",
    "hardConstraints": ["no fabrication"],
    "failureCondition": "any required feature undelivered",
    "targetReleasePacket": "mvp"
  },
  "repos": [{"id": "product", "role": "product"}],
  "nodes": [
    {"id": "deliver-auth", "type": "delivery", "repo": "product", "mode": "full-delivery", "coversFeatures": ["auth-real"]},
    {"id": "deliver-billing", "type": "delivery", "repo": "product", "mode": "full-delivery", "coversFeatures": ["billing"]}
  ]
}
JSON
  jq --arg root "$REPO_ROOT" --arg session "coverage-selftest" \
    --arg controlPathDigest "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" '
    .repos |= map(. + {repositoryRoot: $root, repositoryAlias: .id})
    | .nodes |= map(. + {
        repositoryResolution: {
          sessionId: $session,
          decisionId: ("rb:" + $session + ":1:node:" + .id),
          controlRevision: 1,
          controlPathDigest: $controlPathDigest,
          authority: "scoped-scenario-node",
          transition: "scoped-override",
          scopeKind: "goal-node",
          scopeId: .id,
          targetKind: "goal-node",
          pathVisibility: "local",
          actionable: true
        }
      })
  ' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
}

# 22. covered scenario → exit 0
write_covered
if "$LINT" "$F" "$COVROOT" >/dev/null 2>&1; then
  echo "PASS: targetReleasePacket coverage — all required features covered (exit 0)"
else
  echo "FAIL: covered release-phase scenario should pass"; "$LINT" "$F" "$COVROOT"; exit 1
fi

# 23. under-scoped scenario (required feature 'billing' uncovered) → exit 1
write_covered
jq '(.nodes[] | select(.id=="deliver-billing") | .coversFeatures) = []' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
rc=0
"$LINT" "$F" "$COVROOT" >/dev/null 2>&1 || rc=$?
if [[ $rc -eq 1 ]]; then
  echo "PASS: under-scoped release-phase scenario — uncovered required feature 'billing' rejected (Gate G101, exit 1)"
else
  echo "FAIL: under-scoped scenario should exit 1, got $rc"; "$LINT" "$F" "$COVROOT"; exit 1
fi

# --- IMP-041 SCOPE-2: contribution + planned-delta contract (GF-9) ----------
# A compiled scenario proved SHAPE but not CONTRIBUTION: every node could be
# individually well-formed while the graph drifted off the frozen outcome.
# These checks activate ONLY when a canonical goalContractRef with a v2
# semanticBoundary is present, so case 24 is the additivity guard.

# write_semantic — a clean scenario plus a frozen v2 reference and, on every
# node, the three new declarations.
write_semantic() {
  write_clean
  jq '
    . + {
      goalContractRef: {
        goalId: "gc:sess-041:1",
        revision: 1,
        sourceRequestDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
        workBoundary: { repositoryRoots: ["product"], crossRepoPolicy: "authorized" },
        semanticBoundary: {
          executionShape: "one-off",
          allowedChangeClasses: ["existing-config", "existing-test"],
          approvalRequiredChangeClasses: ["new-runner", "new-virtual-machine"],
          deltaBudget: { maxNewScopes: 2, maxNewFiles: 5 }
        }
      }
    }
    | .nodes |= map(. + {
        contributesTo: ["successSignal"],
        ownershipFit: "the existing validate-only mode already owns this command surface",
        plannedDelta: { changeClasses: ["existing-test"], maxNewFiles: 1 }
      })
  ' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
}

# 24. ADDITIVITY: a legacy scenario carrying no goalContractRef is untouched.
# Without this, every case below could pass while the new checks silently
# started rejecting every scenario already in the field.
write_clean
assert_pass "legacy scenario with no goalContractRef is unaffected by SCOPE-2"

# 25. a fully-declared semantic scenario passes.
write_semantic
assert_pass "semantic scenario with contribution, ownership fit and planned delta"

# 26. a node that names no anchor is unmoored from the goal.
write_semantic
jq '(.nodes[0].contributesTo) = []' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node with empty contributesTo is rejected"

# 27. a contribution must name a REAL anchor, not an invented one.
write_semantic
jq '(.nodes[0].contributesTo) = ["deliver the platform"]' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "contributesTo naming a non-existent anchor is rejected"

# 28. ownership fit is what stops a node grabbing a wider owner than it needs.
write_semantic
jq 'del(.nodes[0].ownershipFit)' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node with no ownershipFit is rejected"

# 29. a node with no declared delta cannot be checked against the budget.
write_semantic
jq 'del(.nodes[0].plannedDelta)' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node with no plannedDelta is rejected"

# 30. THE INCIDENT SHAPE: the node sits on an allowed repo and an allowed path,
# but plans a KIND of change the frozen boundary never declared. The v1
# path boundary cannot see this; the semantic boundary is the only layer that
# refuses it.
write_semantic
jq '(.nodes[0].plannedDelta.changeClasses) = ["new-virtual-machine", "new-runner", "new-cache"]' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "in-boundary node planning an undeclared change class is rejected"

# 31. growth beyond the frozen budget.
write_semantic
jq '(.nodes[0].plannedDelta.maxNewFiles) = 99' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "plannedDelta exceeding the frozen deltaBudget is rejected"

# 32/33. a node reference must DERIVE from the canonical one — a substituted or
# stale reference proves nothing about the goal it claims to serve.
write_semantic
jq '(.nodes[0].goalRef) = {goalId: "gc:other-session:1", revision: 1}' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node goalRef with a substituted goalId is rejected"

write_semantic
jq '(.nodes[0].goalRef) = {goalId: "gc:sess-041:1", revision: 7}' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
assert_fail "node goalRef with a stale revision is rejected"

# --- IMP-041 SCOPE-6: finding admission control (GF-12) ---------------------
# Findings are the second way a goal grows. Each amendment is individually
# defensible; the sum is a different goal. These cases prove admission is
# explicit rather than automatic.

add_finding_node() { # add_finding_node <goalImpact> <extra-jq-assignments>
  write_semantic
  jq --arg impact "$1" "
    .nodes[0] += {
      originFinding: {id: \"F-1\", reportedBy: \"bubbles.audit\"},
      goalImpact: \$impact,
      impactDecidedBy: \"bubbles.goal\"
    } | $2" "$F" > "$F.tmp" && mv "$F.tmp" "$F"
}

# 34. a required finding that names an anchor and was admitted by the parent.
add_finding_node required '.'
assert_pass "a required finding admitted by the parent is accepted"

# 35. an independent finding belongs to its own packet, not this DAG.
add_finding_node independent '.'
assert_fail "an independent finding inserted into the current DAG is rejected"

# 36. blocking-external may stop the goal; it may not authorise delivery work.
add_finding_node blocking-external '(.nodes[0].type) = "delivery"'
assert_fail "a blocking-external finding cannot authorise delivery work"

# 37. an unknown impact value is not a quiet pass.
add_finding_node urgent-ish '.'
assert_fail "an unrecognised goalImpact value is rejected"

# 38. ADVERSARIAL: the reporter cannot grade its own finding. This is the whole
# failure mode — a specialist that both raises and promotes a finding has
# rewritten the goal it was dispatched under.
add_finding_node required '(.nodes[0].impactDecidedBy) = "bubbles.audit"'
assert_fail "a specialist promoting its own finding to required is rejected"

# 39. a required finding still has to earn its place against the outcome.
add_finding_node required '(.nodes[0].contributesTo) = []'
assert_fail "a required finding naming no outcome anchor is rejected"

# 40. ADDITIVITY: a node with no originFinding is untouched.
write_semantic
assert_pass "a node with no originFinding is unaffected by SCOPE-6"

echo "All scenario-compile-lint selftests passed."
