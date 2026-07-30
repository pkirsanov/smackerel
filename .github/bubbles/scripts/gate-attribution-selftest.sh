#!/usr/bin/env bash
# Hermetic selftest for gate-attribution.sh.
#
# The behaviour that must never occur is reporting a reduction SAFE when a gate
# has lost every always-loaded carrier and nothing points at where it went.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOOL="$SCRIPT_DIR/gate-attribution.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

passed=0
failed=0
pass() { printf 'PASS  %s\n' "$1"; passed=$((passed + 1)); }
fail() {
  printf 'FAIL  %s\n      %s\n' "$1" "$2"
  failed=$((failed + 1))
}

# Fixture closure: agent -> alpha (G001 sole), beta (G002 sole, pointed at by
# alpha), gamma (G003, never referenced by anything always-loaded).
mkdir -p "$TMP/agents/bubbles_shared"
A="$TMP/agents/fixture.agent.md"
S="$TMP/agents/bubbles_shared"
cat >"$A" <<'EOF'
# fixture agent
Gate G000 is handled here (fixture gate id, not a real gate).
See bubbles_shared/alpha.md and bubbles_shared/gamma.md
EOF
printf '# alpha\nG001 lives here.\nSee bubbles_shared/beta.md for more.\n' >"$S/alpha.md"
printf '# beta\nG002 lives here.\n' >"$S/beta.md"
printf '# gamma\nG003 lives here.\n' >"$S/gamma.md"

run() { bash "$TOOL" "$@" 2>&1 || true; }
rc() { bash "$TOOL" "$@" >/dev/null 2>&1; echo $?; }

out="$(run "$A")"
if grep -q 'closure modules      : 4' <<<"$out"; then
  pass "resolves the transitive closure"
else
  fail "resolves the transitive closure" "$(head -3 <<<"$out")"
fi

# gamma moved on-demand: alpha still points at gamma? No — the AGENT does.
out="$(run "$A" --ondemand gamma.md)"
if grep -q 'VERDICT: every gate stays reachable' <<<"$out"; then
  pass "a pointed-at on-demand module keeps its gate reachable"
else
  fail "a pointed-at on-demand module keeps its gate reachable" "$out"
fi

# beta moved AND alpha (its only pointer) moved too -> G002 unreachable.
out="$(run "$A" --ondemand beta.md,alpha.md)"
if grep -q 'UNREACHABLE' <<<"$out" && grep -q 'G002' <<<"$out"; then
  pass "gate with no carrier and no pointer is reported UNREACHABLE"
else
  fail "gate with no carrier and no pointer is reported UNREACHABLE" "$out"
fi

if [[ "$(rc "$A" --ondemand beta.md,alpha.md)" == "1" ]]; then
  pass "unreachable gate exits non-zero"
else
  fail "unreachable gate exits non-zero" "expected exit 1"
fi

if [[ "$(rc "$A" --ondemand gamma.md)" == "0" ]]; then
  pass "safe reduction exits zero"
else
  fail "safe reduction exits zero" "expected exit 0"
fi

if [[ "$(rc "$A" --ondemand nosuch.md)" == "1" ]]; then
  pass "module outside the closure is refused"
else
  fail "module outside the closure is refused" "expected exit 1"
fi

if [[ "$(rc "$A" --ondemand fixture.agent.md)" == "1" ]]; then
  pass "the agent file itself cannot be made on-demand"
else
  fail "the agent file itself cannot be made on-demand" "expected exit 1"
fi

out="$(run "$A" --ondemand gamma.md --json)"
if python3 -c "import json,sys; d=json.load(sys.stdin); sys.exit(0 if d['safe'] and d['bytesFreed']>0 else 1)" <<<"$out"; then
  pass "json mode emits a machine-readable verdict"
else
  fail "json mode emits a machine-readable verdict" "$out"
fi

printf '\ngate-attribution selftest: %d passed, %d failed\n' "$passed" "$failed"
[[ "$failed" -eq 0 ]] || exit 1
echo "gate-attribution selftest passed."
