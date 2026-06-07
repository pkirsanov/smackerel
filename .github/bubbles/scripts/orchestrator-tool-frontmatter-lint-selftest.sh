#!/usr/bin/env bash
# orchestrator-tool-frontmatter-lint-selftest.sh
#
# Hermetic selftest for orchestrator-tool-frontmatter-lint.sh.
#
# Stages a synthetic Bubbles-style repo under a temp directory with:
#   - a PASS orchestrator (frontmatter `tools: [..., agent, ...]`,
#     body uses runSubagent)                                  -> exit 0
#   - a FAIL orchestrator (body uses runSubagent BUT frontmatter
#     `tools: [..., todo, ...]` is missing 'agent')           -> exit 1
#   - a non-orchestrator (no runSubagent, no orchestrator name)
#     -> not flagged
#   - an opt-out orchestrator (delegationModel: none)         -> not flagged
#
# Asserts each fixture produces the expected exit code and that the
# FAIL fixture lists the orphan filename in ORCHESTRATOR_MISSING_AGENT_TOOL.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/orchestrator-tool-frontmatter-lint.sh"

if [[ ! -f "$LINT" ]]; then
  echo "[selftest orchestrator-tool-frontmatter-lint] FAIL: missing $LINT" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; failures=$((failures + 1)); }

# --- PASS fixture ---------------------------------------------------------

pass_root="$TMPDIR/repo-pass"
mkdir -p "$pass_root/agents"

cat > "$pass_root/agents/fake.good-orchestrator.agent.md" <<'EOF'
---
description: Fake orchestrator that correctly declares the agent tool
tools: [read, search, edit, agent, todo, web, execute]
---

## Agent Identity

**Name:** fake.good-orchestrator
**Role:** Test orchestrator that delegates correctly

This agent invokes runSubagent(target=fake.executor) to delegate work.
EOF

cat > "$pass_root/agents/fake.executor.agent.md" <<'EOF'
---
description: Fake non-orchestrator executor (no delegation)
tools: [read, edit]
---

## Agent Identity

**Name:** fake.executor
**Role:** Pure executor, never delegates
EOF

set +e
pass_log="$TMPDIR/pass.log"
bash "$LINT" --repo-root "$pass_root" >"$pass_log" 2>&1
pass_rc=$?
set -e

if [[ "$pass_rc" -eq 0 ]]; then
  pass "PASS fixture exits 0"
else
  fail "PASS fixture expected exit 0, got $pass_rc"
  sed -n '1,60p' "$pass_log"
fi

if grep -Fq "PASS — every orchestrator can delegate" "$pass_log"; then
  pass "PASS fixture reports zero missing"
else
  fail "PASS fixture missing PASS marker"
  sed -n '1,60p' "$pass_log"
fi

# --- Adversarial: orchestrator with NO tools: allowlist -------------------
#
# An orchestrator that declares no `tools:` allowlist inherits ALL tools
# (including `agent`), so delegation works. It MUST NOT be flagged. Before the
# v7.0.3 false-positive fix this exited 1 (absent allowlist was treated as
# "missing agent"); it must now exit 0.

inherit_root="$TMPDIR/repo-inherit"
mkdir -p "$inherit_root/agents"

cat > "$inherit_root/agents/fake.inherit-orchestrator.agent.md" <<'EOF'
---
description: Orchestrator with no tools allowlist (inherits all tools)
---

## Agent Identity

**Name:** fake.inherit-orchestrator
**Role:** Orchestrator that delegates and declares no tools: allowlist

This agent invokes runSubagent(target=fake.executor) to delegate work.
EOF

set +e
inherit_log="$TMPDIR/inherit.log"
bash "$LINT" --repo-root "$inherit_root" --verbose >"$inherit_log" 2>&1
inherit_rc=$?
set -e

if [[ "$inherit_rc" -eq 0 ]]; then
  pass "absent tools: orchestrator is NOT flagged (inherits agent, exit 0)"
else
  fail "absent tools: orchestrator expected exit 0, got $inherit_rc"
  sed -n '1,60p' "$inherit_log"
fi

if grep -Fq "OK (no tools: allowlist; inherits agent)" "$inherit_log"; then
  pass "absent tools: orchestrator reported as inheriting agent"
else
  fail "absent tools: orchestrator missing inherit-OK verbose line"
  sed -n '1,60p' "$inherit_log"
fi

# --- FAIL fixture ---------------------------------------------------------

fail_root="$TMPDIR/repo-fail"
mkdir -p "$fail_root/agents"

cat > "$fail_root/agents/fake.bad-orchestrator.agent.md" <<'EOF'
---
description: Fake orchestrator missing the agent tool in frontmatter
tools: [read, search, edit, todo, web, execute]
---

## Agent Identity

**Name:** fake.bad-orchestrator
**Role:** Orchestrator whose runSubagent calls will silently fail

Body uses runSubagent(target=fake.executor) but frontmatter omits 'agent'.
EOF

cat > "$fail_root/agents/fake.executor.agent.md" <<'EOF'
---
description: Pure executor (no delegation)
tools: [read, edit]
---

## Agent Identity

**Name:** fake.executor
EOF

cat > "$fail_root/agents/fake.terminal.agent.md" <<'EOF'
---
description: Terminal agent that explicitly opts out
tools: [read, edit]
delegationModel: none
---

## Agent Identity

**Name:** fake.terminal
**Role:** Mentions runSubagent in docs but opted out via delegationModel: none

Note: this body mentions runSubagent for documentation, but the opt-out
should remove this agent from the lint scope entirely.
EOF

set +e
fail_log="$TMPDIR/fail.log"
bash "$LINT" --repo-root "$fail_root" >"$fail_log" 2>&1
fail_rc=$?
set -e

if [[ "$fail_rc" -eq 1 ]]; then
  pass "FAIL fixture exits 1"
else
  fail "FAIL fixture expected exit 1, got $fail_rc"
  sed -n '1,80p' "$fail_log"
fi

if grep -Fq "ORCHESTRATOR_MISSING_AGENT_TOOL:" "$fail_log"; then
  pass "FAIL fixture prints ORCHESTRATOR_MISSING_AGENT_TOOL marker"
else
  fail "FAIL fixture missing marker"
  sed -n '1,80p' "$fail_log"
fi

if grep -Fq "fake.bad-orchestrator.agent.md" "$fail_log"; then
  pass "FAIL fixture lists the bad orchestrator"
else
  fail "FAIL fixture did not list fake.bad-orchestrator.agent.md"
  sed -n '1,80p' "$fail_log"
fi

if grep -Fq "fake.terminal" "$fail_log"; then
  fail "FAIL fixture incorrectly flagged opt-out agent fake.terminal"
  sed -n '1,80p' "$fail_log"
else
  pass "Opt-out (delegationModel: none) excluded from findings"
fi

if grep -Fq "fake.executor" "$fail_log"; then
  # Acceptable IFF executor was reported as non-orchestrator (no marker line)
  if grep -Fq "fake.executor.agent.md  (name=" "$fail_log"; then
    fail "Non-orchestrator fake.executor was incorrectly flagged"
  else
    pass "Non-orchestrator fake.executor not in findings list"
  fi
else
  pass "Non-orchestrator fake.executor not in findings list"
fi

# --- --allow rescue test --------------------------------------------------

set +e
allow_log="$TMPDIR/allow.log"
bash "$LINT" --repo-root "$fail_root" --allow fake.bad-orchestrator >"$allow_log" 2>&1
allow_rc=$?
set -e

if [[ "$allow_rc" -eq 0 ]]; then
  pass "--allow rescues a flagged orchestrator (exit 0)"
else
  fail "--allow expected exit 0, got $allow_rc"
  sed -n '1,80p' "$allow_log"
fi

# --- Summary --------------------------------------------------------------

echo
if [[ "$failures" -eq 0 ]]; then
  echo "[selftest orchestrator-tool-frontmatter-lint] OK — all assertions passed"
  exit 0
else
  echo "[selftest orchestrator-tool-frontmatter-lint] FAIL — $failures assertion(s) failed"
  exit 1
fi
