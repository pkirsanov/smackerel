#!/usr/bin/env bash
# agent-roster-coverage-selftest.sh — hermetic coverage of agent-roster-coverage.sh
#
# Proves the checker PASSES when every agent appears on every roster surface and
# FAILS when any agent is missing from any surface. Uses a synthetic fixture
# tree; never touches the real repo.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/agent-roster-coverage.sh"

failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

[[ -f "$TARGET" ]] || {
  echo "selftest: target script not found: $TARGET" >&2
  exit 2
}

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

mkdir -p "$workdir/agents" "$workdir/docs/guides"
: >"$workdir/agents/bubbles.alpha.agent.md"
: >"$workdir/agents/bubbles.beta.agent.md"

# Case 1 — both agents present on all three surfaces => exit 0.
printf '`bubbles.alpha` and `bubbles.beta`\n' >"$workdir/README.md"
printf '`bubbles.alpha` and `bubbles.beta`\n' >"$workdir/docs/guides/AGENT_MANUAL.md"
printf 'bubbles.alpha / bubbles.beta\n' >"$workdir/docs/its-not-rocket-appliances.html"
if bash "$TARGET" --repo-root "$workdir" >/dev/null 2>&1; then
  pass "full coverage returns 0"
else
  fail "full coverage should return 0"
fi

# Case 2 — beta missing from the HTML surface => exit 1.
printf 'bubbles.alpha only\n' >"$workdir/docs/its-not-rocket-appliances.html"
if bash "$TARGET" --repo-root "$workdir" >/dev/null 2>&1; then
  fail "missing agent should return non-zero"
else
  pass "missing agent returns non-zero"
fi

# Case 3 — combined-card token match (beta inside a compound cell) => exit 0.
printf 'bubbles.alpha / bubbles.beta combined\n' >"$workdir/docs/its-not-rocket-appliances.html"
if bash "$TARGET" --repo-root "$workdir" >/dev/null 2>&1; then
  pass "combined-entry token match returns 0"
else
  fail "combined-entry token match should return 0"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "agent-roster-coverage-selftest: $failures case(s) failed" >&2
  exit 1
fi
echo "agent-roster-coverage-selftest: all cases passed"
exit 0
