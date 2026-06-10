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

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
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

# 18. --list-forbidden derivation contains all 6 fan-out modes
forbidden="$("$LINT" --list-forbidden "$REPO_ROOT" 2>/dev/null || true)"
for m in iterate autonomous-goal autonomous-sprint stochastic-quality-sweep retro-quality-sweep idea-to-release-completion; do
  if ! grep -qx "$m" <<< "$forbidden"; then
    echo "FAIL: --list-forbidden missing '$m'"; echo "$forbidden"; exit 1
  fi
done
echo "PASS: --list-forbidden derives all 6 requiresTopLevelRuntime fan-out modes"

echo "All scenario-compile-lint selftests passed."
