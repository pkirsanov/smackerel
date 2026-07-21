#!/usr/bin/env bash
# Hermetic selftest for effective-bundle-measure.sh (IMP-100 Phase 6 / IMP-020 S5).
# macOS+WSL portable; jq-gated.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOOL="$SCRIPT_DIR/effective-bundle-measure.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

if ! command -v jq >/dev/null 2>&1; then
  echo "effective-bundle-measure-selftest: SKIP (jq not installed)"
  exit 0
fi

field() { printf '%s' "$1" | jq -r ".$2"; }

# ── Layout with a transitive reference chain agent → a → b, plus a skill pointer.
d="$TMP_ROOT/agents"
mkdir -p "$d/bubbles_shared"
printf '%s\n' '# Agent' 'See [agent-common.md](bubbles_shared/agent-common.md).' 'Skills: skills/bubbles-foo/SKILL.md' > "$d/bubbles.demo.agent.md"
printf '%s\n' 'Common contract. See [scope-workflow.md](bubbles_shared/scope-workflow.md).' > "$d/bubbles_shared/agent-common.md"
printf '%s\n' 'Scope workflow contract content.' > "$d/bubbles_shared/scope-workflow.md"

echo "Running effective-bundle-measure selftest..."

# T1: transitive closure = agent + agent-common + scope-workflow = 3 files; skillPointers >= 1.
out="$(bash "$TOOL" "$d/bubbles.demo.agent.md")" && rc=0 || rc=$?
if [[ "$rc" -eq 0 && "$(field "$out" totalFiles)" -eq 3 && "$(field "$out" skillPointers)" -ge 1 && "$(field "$out" totalBytes)" -gt 0 ]]; then
  pass "T1 transitive closure = 3 files, skillPointers>=1 (exit 0)"
else
  fail "T1 expected totalFiles=3 skillPointers>=1 (rc=$rc, out=$out)"
fi

# T2: an agent with no references → just itself.
solo="$TMP_ROOT/solo/bubbles.solo.agent.md"
mkdir -p "$TMP_ROOT/solo/bubbles_shared"
printf '%s\n' '# Solo agent, no shared references.' > "$solo"
out="$(bash "$TOOL" "$solo")"
if [[ "$(field "$out" totalFiles)" -eq 1 ]]; then
  pass "T2 no references → totalFiles=1"
else
  fail "T2 expected totalFiles=1 (out=$out)"
fi

# T3: cycle a ↔ b terminates and counts each once (agent + a + b = 3).
c="$TMP_ROOT/cyc"
mkdir -p "$c/bubbles_shared"
printf '%s\n' '# Cyclic agent' 'See [a.md](bubbles_shared/a.md).' > "$c/bubbles.cyc.agent.md"
printf '%s\n' 'A refers to [b.md](bubbles_shared/b.md).' > "$c/bubbles_shared/a.md"
printf '%s\n' 'B refers back to [a.md](bubbles_shared/a.md).' > "$c/bubbles_shared/b.md"
out="$(bash "$TOOL" "$c/bubbles.cyc.agent.md")" && rc=0 || rc=$?
if [[ "$rc" -eq 0 && "$(field "$out" totalFiles)" -eq 3 ]]; then
  pass "T3 cycle terminates, each file counted once (totalFiles=3)"
else
  fail "T3 expected termination with totalFiles=3 (rc=$rc, out=$out)"
fi

# T4: missing agent file → usage error.
bash "$TOOL" "$TMP_ROOT/nope.md" >/dev/null 2>&1 && rc=0 || rc=$?
if [[ "$rc" -eq 2 ]]; then pass "T4 missing agent file → exit 2"; else fail "T4 expected exit 2 (rc=$rc)"; fi

# T5: a reference to a non-existent shared doc is skipped (not counted, no error).
m="$TMP_ROOT/miss/bubbles.miss.agent.md"
mkdir -p "$TMP_ROOT/miss/bubbles_shared"
printf '%s\n' '# Agent' 'See [gone.md](bubbles_shared/gone.md).' > "$m"
out="$(bash "$TOOL" "$m")" && rc=0 || rc=$?
if [[ "$rc" -eq 0 && "$(field "$out" totalFiles)" -eq 1 ]]; then
  pass "T5 dangling reference skipped → totalFiles=1 (exit 0)"
else
  fail "T5 expected totalFiles=1 (rc=$rc, out=$out)"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "effective-bundle-measure-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "effective-bundle-measure-selftest: all cases passed."
