#!/usr/bin/env bash
#
# pre-tool-risk-gate-selftest.sh — hermetic selftest for the v6.1 (R10)
# real-time PreToolUse risk gate.
#
# Stages a fixture action-risk-registry.yaml (via BUBBLES_ACTION_RISK_REGISTRY)
# and asserts the gate's ALLOW / WARN / BLOCK decisions and confirmation path.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/pre-tool-risk-gate.sh"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

REG="$TMPDIR/action-risk-registry.yaml"
cat > "$REG" <<'YAML'
version: 1
validRiskClasses:
- read_only
- owned_mutation
- destructive_mutation
- external_side_effect
- runtime_teardown
commands:
  status:
    defaultRiskClass: read_only
  policy:
    defaultRiskClass: read_only
    overrides:
      set: owned_mutation
  runtime:
    defaultRiskClass: read_only
    overrides:
      release: runtime_teardown
  nuke:
    defaultRiskClass: destructive_mutation
  publish:
    defaultRiskClass: external_side_effect
YAML
export BUBBLES_ACTION_RISK_REGISTRY="$REG"

pass_count=0
fail_count=0
pass() { echo "  PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "  FAIL: $1"; fail_count=$((fail_count + 1)); }

expect_exit() {
  # expect_exit <expected> <description> -- <gate args...>
  local expected="$1"; shift
  local desc="$1"; shift
  shift # drop --
  local rc=0
  env -u BUBBLES_RISK_CONFIRM bash "$TARGET" "$@" >/dev/null 2>&1 || rc=$?
  if [[ "$rc" -eq "$expected" ]]; then
    pass "$desc (exit $rc)"
  else
    fail "$desc — expected $expected, got $rc"
  fi
}

# read_only -> allow
expect_exit 0 "read_only command allowed" -- status
# owned_mutation (policy set) -> allow
expect_exit 0 "owned_mutation command allowed" -- policy set foo
# runtime_teardown (runtime release) -> warn/allow
expect_exit 0 "runtime_teardown warns but allows" -- runtime release lease-1
# destructive_mutation -> block (exit 3)
expect_exit 3 "destructive_mutation blocked" -- nuke --all
# external_side_effect -> block (exit 3)
expect_exit 3 "external_side_effect blocked" -- publish
# destructive + --confirm flag -> allow
expect_exit 0 "destructive allowed with --confirm" -- nuke --all --confirm

# destructive + BUBBLES_RISK_CONFIRM=1 -> allow
rc=0; BUBBLES_RISK_CONFIRM=1 bash "$TARGET" nuke --all >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 0 ]]; then pass "destructive allowed with BUBBLES_RISK_CONFIRM=1 (exit 0)"; else fail "confirm env expected 0, got $rc"; fi

# --risk-class direct lookup blocks
expect_exit 3 "--risk-class destructive_mutation blocks" -- --risk-class destructive_mutation

# --resolve prints the class without gating
resolved="$(bash "$TARGET" --resolve runtime release lease-1 2>/dev/null || true)"
if [[ "$resolved" == "runtime_teardown" ]]; then pass "--resolve prints effective class"; else fail "--resolve got '$resolved' (expected runtime_teardown)"; fi

# env override: escalate runtime_teardown into the block set
rc=0; BUBBLES_RISK_BLOCK="destructive_mutation external_side_effect runtime_teardown" bash "$TARGET" runtime release lease-1 >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 3 ]]; then pass "BUBBLES_RISK_BLOCK escalation blocks runtime_teardown"; else fail "escalation expected 3, got $rc"; fi

echo ""
echo "[pre-tool-risk-gate-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[pre-tool-risk-gate-selftest] OK"
exit 0
