#!/usr/bin/env bash
# bubbles/scripts/top-level-runtime-routing-selftest.sh
#
# Selftest for the requiresTopLevelRuntime routing rule.
#
# Asserts:
#   T1. Every mode in the canonical FAN_OUT_MODES list has
#       constraints.requiresTopLevelRuntime: true in bubbles/workflows.yaml.
#   T2. No other mode has the flag set (it is exclusive to fan-out modes
#       to prevent silent spread).
#   T3. The flag value is exactly the boolean `true` (no string typos,
#       no `True`, no `"true"`).
#   T4. workflow-execution-loops.md documents Failure Mode 4 AND the
#       Top-level-runtime modes section.
#   T5. Every mode in FAN_OUT_MODES is mentioned by name in the
#       Top-level-runtime modes section of workflow-execution-loops.md.
#
# Exit 0 = all assertions pass. Exit 1 = at least one assertion failed.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKFLOWS="$ROOT_DIR/bubbles/workflows.yaml"
EXEC_LOOPS="$ROOT_DIR/agents/bubbles_shared/workflow-execution-loops.md"

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; failures=$((failures + 1)); }

# Canonical list of fan-out modes that REQUIRE the top-level runtime.
# Update this list ONLY when adding a new fan-out mode. Single-spec modes
# (bugfix-fastlane, harden-to-doc, etc.) MUST NOT be added here.
FAN_OUT_MODES=(
  stochastic-quality-sweep
  retro-quality-sweep
  iterate
  autonomous-goal
  autonomous-sprint
  idea-to-release-completion
)

[[ -f "$WORKFLOWS" ]] || { fail "workflows.yaml not found at $WORKFLOWS"; exit 1; }
[[ -f "$EXEC_LOOPS" ]] || { fail "workflow-execution-loops.md not found at $EXEC_LOOPS"; exit 1; }

# --- T1 + T3: each fan-out mode has the flag set to exactly `true` ---

# Find every line that declares `requiresTopLevelRuntime:` along with the
# value, then for each FAN_OUT_MODES entry assert one such declaration
# exists inside that mode's block.
#
# We use awk to map mode-name -> requiresTopLevelRuntime value.
mode_flag_map="$(awk '
  /^  [a-z][a-z0-9-]*:$/ {
    current_mode = $1
    sub(":", "", current_mode)
    next
  }
  /^      requiresTopLevelRuntime:/ {
    val = $2
    print current_mode "=" val
  }
' "$WORKFLOWS")"

for mode in "${FAN_OUT_MODES[@]}"; do
  line="$(echo "$mode_flag_map" | grep -E "^${mode}=" || true)"
  if [[ -z "$line" ]]; then
    fail "T1: fan-out mode '${mode}' is missing requiresTopLevelRuntime: true"
    continue
  fi
  val="${line#*=}"
  if [[ "$val" != "true" ]]; then
    fail "T3: fan-out mode '${mode}' has requiresTopLevelRuntime: '${val}' (expected boolean true)"
  else
    pass "T1+T3: ${mode} has requiresTopLevelRuntime: true"
  fi
done

# --- T2: no OTHER mode has the flag set ---

while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  mode="${line%%=*}"
  found=0
  for fm in "${FAN_OUT_MODES[@]}"; do
    if [[ "$fm" == "$mode" ]]; then found=1; break; fi
  done
  if (( found == 0 )); then
    fail "T2: mode '${mode}' has requiresTopLevelRuntime set but is not in FAN_OUT_MODES (selftest list)"
  fi
done <<< "$mode_flag_map"

mode_with_flag_count="$(echo "$mode_flag_map" | grep -c . || true)"
if [[ "$mode_with_flag_count" == "${#FAN_OUT_MODES[@]}" ]]; then
  pass "T2: exactly ${#FAN_OUT_MODES[@]} modes declare requiresTopLevelRuntime (matches canonical list)"
fi

# --- T4: documentation present ---

if grep -q "Failure Mode 4" "$EXEC_LOOPS"; then
  pass "T4a: workflow-execution-loops.md mentions Failure Mode 4"
else
  fail "T4a: workflow-execution-loops.md missing Failure Mode 4 documentation"
fi

if grep -q "Top-level-runtime modes" "$EXEC_LOOPS"; then
  pass "T4b: workflow-execution-loops.md has Top-level-runtime modes section"
else
  fail "T4b: workflow-execution-loops.md missing Top-level-runtime modes section"
fi

if grep -q "routingReason: \"top-level-runtime-required\"" "$EXEC_LOOPS"; then
  pass "T4c: workflow-execution-loops.md specifies the route_required routingReason"
else
  fail "T4c: workflow-execution-loops.md missing route_required routingReason spec"
fi

# --- T5: each fan-out mode is named in the Top-level-runtime modes section ---

# Extract everything between the section header and the next ### header.
toplevel_section="$(awk '
  /^### Top-level-runtime modes/ { capture = 1; next }
  capture && /^### / { exit }
  capture { print }
' "$EXEC_LOOPS")"

if [[ -z "$toplevel_section" ]]; then
  fail "T5: could not extract Top-level-runtime modes section (regex change?)"
else
  for mode in "${FAN_OUT_MODES[@]}"; do
    if echo "$toplevel_section" | grep -q -F "\`${mode}\`"; then
      pass "T5: ${mode} listed in Top-level-runtime modes section"
    else
      fail "T5: ${mode} NOT listed in Top-level-runtime modes section"
    fi
  done
fi

# --- Summary ---

if (( failures == 0 )); then
  echo "OK: top-level-runtime-routing-selftest passed (${#FAN_OUT_MODES[@]} fan-out modes verified)"
  exit 0
else
  echo "FAILED: top-level-runtime-routing-selftest had ${failures} assertion failures" >&2
  exit 1
fi
