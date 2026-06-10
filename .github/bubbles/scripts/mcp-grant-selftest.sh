#!/usr/bin/env bash
# File: mcp-grant-selftest.sh
#
# Hermetic selftest for operator-managed MCP tool grants (v7.1) plus per-repo
# MCP server-token materialization (v7.7):
#   mcp-grant-reconcile.sh  (lib: effective-grants / reconcile / inject /
#                            server-token materialization)
#   mcp-grant-sync.sh       (CLI: deterministic injector)
#
# Proves the grant-aware integrity contract via the six adversarial cases the
# downstream-framework-write-guard depends on, injector idempotency, an
# end-to-end CLI run against a synthetic downstream tree, AND the per-repo
# server-token materialization round-trip (inject materializes `bubbles` →
# `bubbles-<slug>`; reconcile normalizes it back so the canonical .checksums
# still matches), plus the restricted-agent trailing-newline invariant.
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
# DROOT basename is "downstream" → the lib materializes the per-repo server
# token `bubbles-downstream` in the core position (replacing the `bubbles`
# placeholder), then appends the sorted operator grants.
DS_GRANTED_LINE='tools: [read, search, edit, agent, todo, web, execute, bubbles-downstream, playwright, context7, github]'
DS_MAT_TOOLS='tools: [read, search, edit, agent, todo, web, execute, bubbles-downstream, playwright]'
assert_eq "T8 CLI sync materializes per-repo server token + applies grants" \
  "$DS_GRANTED_LINE" "$(grep '^tools: ' "$DROOT/.github/agents/bubbles.goal.agent.md")"

if bash "$DROOT/.github/bubbles/scripts/mcp-grant-sync.sh" --check --quiet; then
  ok "T8b CLI --check reports in-sync after sync (exit 0)"
else
  bad "T8b CLI --check reports in-sync after sync (exit 0)" "exit 0" "exit $?"
fi

# T8d: the write-guard's reconcile (run via the DOWNSTREAM lib, which derives
# server token `bubbles-downstream` from its own path) normalizes the
# materialized token AND strips grants back to the canonical bytes — so the
# materialized+granted agent still matches the CANONICAL .checksums hash.
RECON_DS="$(
  # shellcheck source=/dev/null
  source "$DROOT/.github/bubbles/scripts/mcp-grant-reconcile.sh"
  bubbles_mcp_reconcile_to_stdout \
    "$DROOT/.github/agents/bubbles.goal.agent.md" \
    "$DROOT/.github/bubbles-project.yaml" bubbles.goal
)"
assert_eq "T8d downstream reconcile(materialized+granted) == canonical (write-guard stays green)" \
  "$CANON_BYTES" "$RECON_DS"

# Removing the grant + re-sync leaves the materialized per-repo token (no grants).
cp "$CONFIG_EMPTY" "$DROOT/.github/bubbles-project.yaml"
bash "$DROOT/.github/bubbles/scripts/mcp-grant-sync.sh" --quiet
assert_eq "T8c re-sync after grant removal keeps per-repo materialized token" \
  "$DS_MAT_TOOLS" "$(grep '^tools: ' "$DROOT/.github/agents/bubbles.goal.agent.md")"

# --- T9: materialization via FORCE override (deterministic, layout-agnostic) -
MAT_TOOLS_ACME='tools: [read, search, edit, agent, todo, web, execute, bubbles-acme, playwright]'
MAT_GRANTED_ACME='tools: [read, search, edit, agent, todo, web, execute, bubbles-acme, playwright, context7, github]'

MAT_INJECT="$(BUBBLES_MCP_FORCE_SERVER_TOKEN=bubbles-acme bubbles_mcp_inject_to_stdout "$CANON_AGENT" "$CONFIG_EMPTY" bubbles.goal)"
MAT_FILE="$WORK/materialized.agent.md"
printf '%s\n' "$MAT_INJECT" >"$MAT_FILE"
assert_eq "T9 inject materializes server token in core position (no grants)" \
  "$MAT_TOOLS_ACME" "$(grep '^tools: ' "$MAT_FILE")"

# --- T10: reconcile normalizes the materialized token back to canonical ------
RECON_MAT="$(BUBBLES_MCP_FORCE_SERVER_TOKEN=bubbles-acme bubbles_mcp_reconcile_to_stdout "$MAT_FILE" "$CONFIG_EMPTY" bubbles.goal)"
assert_eq "T10 reconcile(materialized) == canonical (no grant, token normalized)" "$CANON_BYTES" "$RECON_MAT"

# --- T11: materialize + grants round-trips to canonical ----------------------
MAT_GRANTED="$(BUBBLES_MCP_FORCE_SERVER_TOKEN=bubbles-acme bubbles_mcp_inject_to_stdout "$CANON_AGENT" "$CONFIG" bubbles.goal)"
MAT_GRANTED_FILE="$WORK/materialized-granted.agent.md"
printf '%s\n' "$MAT_GRANTED" >"$MAT_GRANTED_FILE"
assert_eq "T11 inject materializes token AND appends sorted grants" \
  "$MAT_GRANTED_ACME" "$(grep '^tools: ' "$MAT_GRANTED_FILE")"
RECON_MG="$(BUBBLES_MCP_FORCE_SERVER_TOKEN=bubbles-acme bubbles_mcp_reconcile_to_stdout "$MAT_GRANTED_FILE" "$CONFIG" bubbles.goal)"
assert_eq "T11b reconcile(materialized + granted) == canonical" "$CANON_BYTES" "$RECON_MG"

# --- T12: a grant equal to the server token is excluded as core (no dup) ------
CONFIG_DUP="$WORK/bubbles-project-dup.yaml"
{
  echo 'mcp:'
  echo '  grants:'
  echo '    bubbles.goal: [bubbles-acme, github]'
} >"$CONFIG_DUP"
DUP_EFF="$(BUBBLES_MCP_FORCE_SERVER_TOKEN=bubbles-acme bubbles_mcp_effective_grants "$CONFIG_DUP" bubbles.goal | paste -sd, -)"
assert_eq "T12 grant equal to materialized server token is excluded as core" "github" "$DUP_EFF"
DUP_INJECT="$(BUBBLES_MCP_FORCE_SERVER_TOKEN=bubbles-acme bubbles_mcp_inject_to_stdout "$CANON_AGENT" "$CONFIG_DUP" bubbles.goal)"
assert_eq "T12b server token not double-appended when granted" \
  "tools: [read, search, edit, agent, todo, web, execute, bubbles-acme, playwright, github]" \
  "$(printf '%s\n' "$DUP_INJECT" | grep '^tools: ')"

# --- T13: the 5 restricted SOURCE agents end with a trailing newline ---------
# The inject/reconcile awk rewrite re-emits the whole file and ALWAYS terminates
# with a newline; a restricted agent that lacks a trailing newline would gain a
# byte on sync and drift from .checksums (the latent bug this release fixes).
AGENTS_SRC="$SCRIPT_DIR/../../agents"
if [[ -d "$AGENTS_SRC" ]]; then
  nl_fail=0
  for a in goal sprint iterate bug workflow; do
    f="$AGENTS_SRC/bubbles.${a}.agent.md"
    [[ -f "$f" ]] || continue
    if [[ "$(tail -c1 "$f" | od -An -tx1 | tr -d ' \n')" != "0a" ]]; then
      echo "    restricted agent missing trailing newline: bubbles.${a}.agent.md"
      nl_fail=$((nl_fail + 1))
    fi
  done
  assert_eq "T13 all 5 restricted source agents end with a trailing newline" "0" "$nl_fail"
fi

echo "mcp-grant-selftest: ${pass} passed, ${fail} failed"
[[ "$fail" -eq 0 ]] || exit 1
echo "mcp-grant-selftest: PASS"
exit 0
