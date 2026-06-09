#!/usr/bin/env bash
# File: mcp-grant-sync.sh
#
# Apply operator-declared MCP tool grants (from .github/bubbles-project.yaml
# `mcp.grants`) to the five framework-managed restricted orchestrator agents.
#
# For each restricted agent, the canonical single-line `tools:` array is
# rewritten to: core tools (canonical order) + that agent's effective declared
# grants (sorted suffix). The rewrite is deterministic and idempotent: running
# it twice with the same config produces identical bytes, and removing a grant
# from config + re-running resets the agent to the new desired state.
#
# This is wired into install/refresh (so grants survive a framework refresh) and
# is also runnable on demand:
#
#   bash .github/bubbles/scripts/cli.sh mcp sync
#
# Exit codes:
#   0  success (agents in sync; some may have been rewritten)
#   1  --check mode found an agent whose on-disk tools line != desired
#   2  usage / environment error
#
# The integrity guard (downstream-framework-write-guard.sh) is grant-aware, so a
# synced agent is recognized as clean (declared grant) while any undeclared edit
# still fails as drift.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/mcp-grant-reconcile.sh"

if [[ "$SCRIPT_DIR" == *"/.github/bubbles/scripts" ]]; then
  PROJECT_ROOT="${SCRIPT_DIR%/.github/bubbles/scripts}"
  AGENTS_DIR="$PROJECT_ROOT/.github/agents"
else
  PROJECT_ROOT="${SCRIPT_DIR%/bubbles/scripts}"
  AGENTS_DIR="$PROJECT_ROOT/agents"
fi

quiet="false"
check_only="false"

usage() {
  cat <<'EOF'
Usage: bash .github/bubbles/scripts/cli.sh mcp sync [--check] [--quiet]

Applies operator-declared MCP tool grants (.github/bubbles-project.yaml
`mcp.grants`) to the restricted orchestrator agents. Idempotent.

  --check   Do not write; exit 1 if any agent's tools line != desired.
  --quiet   Suppress per-agent progress output.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    sync) shift ;;                 # tolerate `mcp sync` dispatch passing the subcommand through
    --check) check_only="true"; shift ;;
    --quiet) quiet="true"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "mcp-grant-sync: unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

say() { [[ "$quiet" == "true" ]] || echo "$1"; }

CONFIG_FILE="$(bubbles_mcp_config_path "$PROJECT_ROOT")"
MCP_JSON="$PROJECT_ROOT/.vscode/mcp.json"

if [[ ! -d "$AGENTS_DIR" ]]; then
  echo "mcp-grant-sync: agents directory not found: $AGENTS_DIR" >&2
  exit 2
fi

# Optional soft check: warn (never fail) when a granted tool's MCP server is not
# configured in .vscode/mcp.json. A grant is inert until its server exists there.
warn_missing_server() {
  local token="$1"
  [[ -f "$MCP_JSON" ]] || return 0
  command -v yq >/dev/null 2>&1 || return 0
  local known=''
  known="$(yq -r '.servers // {} | keys | .[]?' - <"$MCP_JSON" 2>/dev/null || true)"
  if ! printf '%s\n' "$known" | grep -qxF "$token"; then
    say "  ⚠️  grant '${token}' has no matching server in .vscode/mcp.json (grant is inert until configured)"
  fi
}

changed=0
drift=0
considered=0

for agent in "${BUBBLES_MCP_RESTRICTED_AGENTS[@]}"; do
  agent_file="$AGENTS_DIR/${agent}.agent.md"
  [[ -f "$agent_file" ]] || continue
  considered=$((considered + 1))

  if ! grep -qE '^tools: \[.*\]$' "$agent_file"; then
    echo "mcp-grant-sync: ${agent} has no canonical single-line 'tools:' array — skipping (not safe to rewrite)" >&2
    continue
  fi

  desired="$(bubbles_mcp_inject_to_stdout "$agent_file" "$CONFIG_FILE" "$agent")"

  if [[ "$desired" == "$(cat "$agent_file")" ]]; then
    say "  ✓ ${agent} (in sync)"
    continue
  fi

  if [[ "$check_only" == "true" ]]; then
    say "  ✗ ${agent} (out of sync — run 'mcp sync')"
    drift=$((drift + 1))
    continue
  fi

  tmp="$(mktemp)"
  printf '%s\n' "$desired" >"$tmp"
  mv "$tmp" "$agent_file"
  changed=$((changed + 1))
  say "  ↻ ${agent} (grants applied)"

  while IFS= read -r token; do
    [[ -n "$token" ]] || continue
    warn_missing_server "$token"
  done < <(bubbles_mcp_effective_grants "$CONFIG_FILE" "$agent")
done

if [[ "$check_only" == "true" ]]; then
  if [[ "$drift" -gt 0 ]]; then
    echo "mcp-grant-sync: ${drift} agent(s) out of sync"
    exit 1
  fi
  say "mcp-grant-sync: all ${considered} restricted agent(s) in sync"
  exit 0
fi

if [[ -z "$CONFIG_FILE" ]]; then
  say "mcp-grant-sync: no .github/bubbles-project.yaml — restricted agents kept canonical"
else
  say "mcp-grant-sync: ${changed} agent(s) updated from ${CONFIG_FILE#"$PROJECT_ROOT"/}"
fi
exit 0
