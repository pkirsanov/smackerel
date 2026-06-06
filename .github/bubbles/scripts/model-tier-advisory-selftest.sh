#!/usr/bin/env bash
#
# model-tier-advisory-selftest.sh — hermetic selftest for the v6.1 / S9 (R4)
# blocking model-tier floor (gate G126).
#
# Stages a minimal workflows.yaml fixture (via BUBBLES_WORKFLOWS_FILE) declaring
# modeDefaults.modelFloor + modelFloorEnforcedPhases, then asserts:
#
#   1. enforced phase + known model BELOW floor            -> exit 1 (BLOCKED)
#   2. enforced phase + known model AT/ABOVE floor         -> exit 0 (OK)
#   3. enforced phase + UNKNOWN model (env unset)          -> exit 0 (no false block)
#   4. non-enforced phase + known model below floor        -> exit 0 (advisory WARN)
#   5. phase with NO floor declared                        -> exit 0
#   6. --enforce forces blocking on a non-listed phase     -> exit 1
#   7. resolve op prints the resolved floor
#
# Exit 0 when all assertions pass; 1 otherwise.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/model-tier-advisory.sh"

if ! command -v python3 >/dev/null 2>&1; then
  echo "model-tier-advisory-selftest: SKIP (python3 not installed)"
  exit 0
fi
if ! python3 -c 'import yaml' >/dev/null 2>&1; then
  echo "model-tier-advisory-selftest: SKIP (PyYAML not installed)"
  exit 0
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

FIXTURE="$TMPDIR/workflows.yaml"
cat > "$FIXTURE" <<'YAML'
modes:
  full-delivery:
    description: fixture mode
modeDefaults:
  modelFloor:
    default: ""
    implement: sonnet-class
    validate: sonnet-class
    audit: sonnet-class
    security: opus-class
    test: ""
  modelFloorEnforcedPhases: [ audit, security, validate ]
YAML

export BUBBLES_WORKFLOWS_FILE="$FIXTURE"
# Keep selftest writes out of the real repo's tool-call log.
export BUBBLES_TOOL_LOG_FILE="$TMPDIR/tool-calls.jsonl"

pass_count=0
fail_count=0
pass() { echo "  PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "  FAIL: $1"; fail_count=$((fail_count + 1)); }

run() {
  # run <expected_exit> <description> -- <args...>   (model passed via $MODEL)
  local expected="$1"; shift
  local desc="$1"; shift
  shift # drop the literal --
  local rc=0
  if [[ -n "${MODEL:-}" ]]; then
    BUBBLES_ACTIVE_MODEL="$MODEL" bash "$TARGET" "$@" >/dev/null 2>&1 || rc=$?
  else
    env -u BUBBLES_ACTIVE_MODEL bash "$TARGET" "$@" >/dev/null 2>&1 || rc=$?
  fi
  if [[ "$rc" -eq "$expected" ]]; then
    pass "$desc (exit $rc)"
  else
    fail "$desc — expected exit $expected, got $rc"
  fi
}

# 1. enforced phase (audit) + haiku active (below sonnet floor) -> BLOCK (1)
MODEL="haiku-3.5" run 1 "enforced audit + below floor blocks" -- check --mode full-delivery --phase audit
# 2. enforced phase (audit) + opus active (above sonnet floor) -> OK (0)
MODEL="opus-4.7" run 0 "enforced audit + above floor passes" -- check --mode full-delivery --phase audit
# 2b. enforced phase (security) + sonnet active (below opus floor) -> BLOCK (1)
MODEL="sonnet-4.5" run 1 "enforced security + below opus floor blocks" -- check --mode full-delivery --phase security
# 3. enforced phase (audit) + UNKNOWN model -> no false block (0)
MODEL="" run 0 "enforced audit + unknown model does not block" -- check --mode full-delivery --phase audit
# 4. non-enforced phase (implement) + haiku active (below floor) -> advisory (0)
MODEL="haiku-3.5" run 0 "non-enforced implement + below floor is advisory" -- check --mode full-delivery --phase implement
# 5. phase with no floor (test) + haiku active -> 0
MODEL="haiku-3.5" run 0 "no-floor phase passes" -- check --mode full-delivery --phase test
# 6. --enforce forces blocking on a non-listed phase (implement) -> BLOCK (1)
MODEL="haiku-3.5" run 1 "--enforce forces blocking on non-listed phase" -- check --enforce --mode full-delivery --phase implement

# 7. resolve prints the floor
resolved="$(BUBBLES_WORKFLOWS_FILE="$FIXTURE" bash "$TARGET" resolve --mode full-delivery --phase security 2>/dev/null || true)"
if [[ "$resolved" == "opus-class" ]]; then
  pass "resolve prints declared floor (opus-class)"
else
  fail "resolve expected 'opus-class', got '$resolved'"
fi

echo ""
echo "[model-tier-advisory-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[model-tier-advisory-selftest] OK"
exit 0
