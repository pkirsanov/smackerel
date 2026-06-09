#!/usr/bin/env bash
# File: mcp-grant-selftest.sh
#
# Hermetic selftest for operator-managed MCP tool grants (v7.1):
#   mcp-grant-reconcile.sh  (lib: effective-grants / reconcile / inject)
#   mcp-grant-sync.sh       (CLI: deterministic injector)
#
# Proves the grant-aware integrity contract via the six adversarial cases the
# downstream-framework-write-guard depends on, plus injector idempotency and an
# end-to-end CLI run against a synthetic downstream tree.
#
# Exit 0 = all assertions pass. Exit 1 = a failure. Exit 0 (advisory skip) when
# yq is unavailable (grant resolution requires yq, like the mode resolver).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/mcp-grant-reconcile.sh"

pass=0
fail=0

ok()   { echo "  PASS: $1"; pass=$((pass + 1)); }
bad()  { echo "  FAIL: $1"; echo "    expected: [$2]"; echo "    actual:   [$3]"; fail=$((fail + 1)); }

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then ok "$label"; else bad "$label" "$expected" "$actual"; fi
}
assert_ne() {
  local label="$1" unexpected="$2" actual="$3"
  if [[ "$unexpected" != "$actual" ]]; then ok "$label"; else bad "$label" "!= $unexpected" "$actual"; fi
}

echo "mcp-grant-selftest: starting"

if ! command -v yq >/dev/null 2>&1; then
  echo "  SKIP: yq not available — grant resolution requires yq (advisory skip)"
  echo "mcp-grant-selftest: PASS (advisory skip, 0 assertions)"
  exit 0
fi

# HOME-based work dir: snap-confined yq cannot access caller-created /tmp paths
# (same constraint mode-resolver.sh documents). The lib reads config via stdin
# redirect, but HOME-basing the fixtures removes any path-access doubt.
WORK="$(mktemp -d "$HOME/.bubbles-mcp-grant-selftest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

CANON_TOOLS='tools: [read, search, edit, agent, todo, web, execute, bubbles, playwright]'

# Canonical restricted-agent fixture (mirrors real frontmatter shape).
write_canonical_agent() {
  local target="$1"
  {
    echo '---'
    echo 'description: test orchestrator'
    echo "$CANON_TOOLS"
    echo 'name: bubbles.goal'
    echo '---'
    echo ''
    echo '**Name:** bubbles.goal'
    echo ''
    echo 'Body content that must remain byte-identical.'
  } >"$target"
}

CANON_AGENT="$WORK/canonical.agent.md"
write_canonical_agent "$CANON_AGENT"
CANON_BYTES="$(cat "$CANON_AGENT")"

# Config: grant github + context7 to bubbles.goal (non-core grant tokens;
# bubbles + playwright are framework defaults in the canonical core, so they are
# never valid grants).
CONFIG="$WORK/bubbles-project.yaml"
{
  echo 'mcp:'
  echo '  grants:'
  echo '    bubbles.goal: [github, context7]'
} >"$CONFIG"

# Empty-grants config (grant removed).
CONFIG_EMPTY="$WORK/bubbles-project-empty.yaml"
{
  echo 'mcp:'
  echo '  grants: {}'
} >"$CONFIG_EMPTY"

# --- T0: effective grants resolve, sorted, core-excluded -------------------
EFF="$(bubbles_mcp_effective_grants "$CONFIG" bubbles.goal | paste -sd, -)"
assert_eq "T0 effective grants = context7,github (sorted, core-excluded)" "context7,github" "$EFF"

# Non-restricted/no-grant agent yields empty.
EFF_NONE="$(bubbles_mcp_effective_grants "$CONFIG" bubbles.sprint | paste -sd, -)"
assert_eq "T0b unrelated agent has no grants" "" "$EFF_NONE"

# --- T1 (case f): reconcile of canonical == canonical (no false positive) --
RECON_CANON="$(bubbles_mcp_reconcile_to_stdout "$CANON_AGENT" "$CONFIG" bubbles.goal)"
assert_eq "T1 reconcile(canonical) == canonical (pre-sync, no false positive)" "$CANON_BYTES" "$RECON_CANON"

# --- T2 (case a): inject then reconcile reproduces canonical ---------------
GRANTED="$(bubbles_mcp_inject_to_stdout "$CANON_AGENT" "$CONFIG" bubbles.goal)"
GRANTED_FILE="$WORK/granted.agent.md"
printf '%s\n' "$GRANTED" >"$GRANTED_FILE"
EXPECT_GRANTED_LINE='tools: [read, search, edit, agent, todo, web, execute, bubbles, playwright, context7, github]'
assert_eq "T2a inject appends sorted grants in canonical format" \
  "$EXPECT_GRANTED_LINE" "$(grep '^tools: ' "$GRANTED_FILE")"
RECON_GRANTED="$(bubbles_mcp_reconcile_to_stdout "$GRANTED_FILE" "$CONFIG" bubbles.goal)"
assert_eq "T2b reconcile(granted) == canonical (declared grant accepted)" "$CANON_BYTES" "$RECON_GRANTED"

# --- T3: injector idempotency ---------------------------------------------
GRANTED2="$(bubbles_mcp_inject_to_stdout "$GRANTED_FILE" "$CONFIG" bubbles.goal)"
assert_eq "T3 inject is idempotent" "$GRANTED" "$GRANTED2"

# --- T4 (case b): body tamper still drifts ---------------------------------
TAMPER_FILE="$WORK/tamper.agent.md"
{ cat "$GRANTED_FILE"; echo 'INJECTED MALICIOUS LINE'; } >"$TAMPER_FILE"
RECON_TAMPER="$(bubbles_mcp_reconcile_to_stdout "$TAMPER_FILE" "$CONFIG" bubbles.goal)"
assert_ne "T4 reconcile(granted + body tamper) != canonical (drift)" "$CANON_BYTES" "$RECON_TAMPER"

# --- T5 (case c): undeclared tool still drifts -----------------------------
UNDECL_FILE="$WORK/undeclared.agent.md"
sed 's/^tools: \[.*\]$/tools: [read, search, edit, agent, todo, web, execute, bubbles, playwright, github, eviltool]/' \
  "$CANON_AGENT" >"$UNDECL_FILE"
RECON_UNDECL="$(bubbles_mcp_reconcile_to_stdout "$UNDECL_FILE" "$CONFIG" bubbles.goal)"
assert_ne "T5 reconcile(declared github + UNDECLARED eviltool) != canonical (drift)" \
  "$CANON_BYTES" "$RECON_UNDECL"
# And the declared token WAS stripped (only eviltool remains beyond core).
assert_eq "T5b undeclared tool survives strip; declared one removed" \
  "tools: [read, search, edit, agent, todo, web, execute, bubbles, playwright, eviltool]" \
  "$(printf '%s\n' "$RECON_UNDECL" | grep '^tools: ')"

# --- T6 (case d): grant removed from config resets to canonical ------------
RESET="$(bubbles_mcp_inject_to_stdout "$GRANTED_FILE" "$CONFIG_EMPTY" bubbles.goal)"
assert_eq "T6 inject(granted, empty-grants config) == canonical (grant removed)" "$CANON_BYTES" "$RESET"

# --- T7 (case e): core token removed still drifts --------------------------
NOAGENT_FILE="$WORK/noagent.agent.md"
sed 's/^tools: \[.*\]$/tools: [read, search, edit, todo, web, execute, bubbles, playwright, github]/' \
  "$CANON_AGENT" >"$NOAGENT_FILE"
RECON_NOAGENT="$(bubbles_mcp_reconcile_to_stdout "$NOAGENT_FILE" "$CONFIG" bubbles.goal)"
assert_ne "T7 reconcile(granted minus core 'agent') != canonical (drift)" "$CANON_BYTES" "$RECON_NOAGENT"

# --- T8: end-to-end CLI sync against a synthetic downstream tree ------------
DROOT="$WORK/downstream"
mkdir -p "$DROOT/.github/bubbles/scripts" "$DROOT/.github/agents"
cp "$SCRIPT_DIR/mcp-grant-reconcile.sh" "$DROOT/.github/bubbles/scripts/"
cp "$SCRIPT_DIR/mcp-grant-sync.sh" "$DROOT/.github/bubbles/scripts/"
write_canonical_agent "$DROOT/.github/agents/bubbles.goal.agent.md"
cp "$CONFIG" "$DROOT/.github/bubbles-project.yaml"

bash "$DROOT/.github/bubbles/scripts/mcp-grant-sync.sh" --quiet
assert_eq "T8 CLI sync applied grants to downstream agent" \
  "$EXPECT_GRANTED_LINE" "$(grep '^tools: ' "$DROOT/.github/agents/bubbles.goal.agent.md")"

if bash "$DROOT/.github/bubbles/scripts/mcp-grant-sync.sh" --check --quiet; then
  ok "T8b CLI --check reports in-sync after sync (exit 0)"
else
  bad "T8b CLI --check reports in-sync after sync (exit 0)" "exit 0" "exit $?"
fi

# Removing the grant + re-sync resets the agent to canonical.
cp "$CONFIG_EMPTY" "$DROOT/.github/bubbles-project.yaml"
bash "$DROOT/.github/bubbles/scripts/mcp-grant-sync.sh" --quiet
assert_eq "T8c re-sync after grant removal resets agent to canonical" \
  "$CANON_TOOLS" "$(grep '^tools: ' "$DROOT/.github/agents/bubbles.goal.agent.md")"

echo "mcp-grant-selftest: ${pass} passed, ${fail} failed"
[[ "$fail" -eq 0 ]] || exit 1
echo "mcp-grant-selftest: PASS"
exit 0
