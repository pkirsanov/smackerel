#!/usr/bin/env bash
# File: bubbles-graph-neighbors.sh
#
# MCP `graph_neighbors` bash twin (IMP-015 Scope A) — a THIN wrapper over the
# read-only governance hub composer bubbles-hub-report.sh. It verifies the
# requested node is a REAL governance node, then delegates the reverse-
# dependency lookup UNCHANGED to:
#     bubbles-hub-report.sh --node <node> --format <format> --root <root>
# and returns that composer's stdout verbatim. It adds NO graph/edge logic —
# all in-degree / dependent computation stays in the composer (IMP-014).
#
# Why the existence guard exists: `bubbles-hub-report.sh --node` ALWAYS exits 0
# and returns {kind:null, inDegree:0, dependents:[]} for BOTH a real-but-
# isolated node AND a totally-unknown node — it cannot distinguish them from
# its output. The MCP `graph_neighbors` contract requires an UNKNOWN node to
# surface as a structured error, while a REAL node with no dependents is a
# legitimate success (inDegree 0). This wrapper draws exactly that line with a
# node-EXISTENCE check (a real file under bubbles/scripts[/guards]/*.sh, a real
# agents/bubbles_shared/*.md module, or a real Gxxx in bubbles/registry/
# gates.yaml) — NEVER by treating inDegree 0 as an error.
#
# Usage: bubbles-graph-neighbors.sh --node <id> [--format json|text] [--root <path>]
# Exit:  0 = report emitted (node is real; may legitimately have 0 dependents).
#        2 = usage error (missing/invalid arguments).
#        3 = unknown node (not a real script / shared-module / gate).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

NODE=""
FORMAT="json"
ROOT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --node)
      NODE="${2:-}"
      shift 2 || {
        echo "bubbles-graph-neighbors: --node needs a value" >&2
        exit 2
      }
      ;;
    --node=*)
      NODE="${1#*=}"
      shift
      ;;
    --format)
      FORMAT="${2:-}"
      shift 2 || {
        echo "bubbles-graph-neighbors: --format needs a value" >&2
        exit 2
      }
      ;;
    --format=*)
      FORMAT="${1#*=}"
      shift
      ;;
    --root)
      ROOT="${2:-}"
      shift 2 || {
        echo "bubbles-graph-neighbors: --root needs a value" >&2
        exit 2
      }
      ;;
    --root=*)
      ROOT="${1#*=}"
      shift
      ;;
    -h | --help)
      echo "Usage: bubbles-graph-neighbors.sh --node <id> [--format json|text] [--root <path>]"
      exit 0
      ;;
    *)
      echo "bubbles-graph-neighbors: unknown argument '$1'." >&2
      exit 2
      ;;
  esac
done

if [[ -z "$NODE" ]]; then
  echo "bubbles-graph-neighbors: --node is required" >&2
  exit 2
fi

case "$FORMAT" in
  text | json) ;;
  *)
    echo "bubbles-graph-neighbors: --format must be 'text' or 'json' (got '$FORMAT')." >&2
    exit 2
    ;;
esac

if [[ -z "$ROOT" ]]; then
  if [[ "$SCRIPT_DIR" == *"/.github/bubbles/scripts" ]]; then
    ROOT="${SCRIPT_DIR%/bubbles/scripts}"
  else
    ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
  fi
fi

HUB="$SCRIPT_DIR/bubbles-hub-report.sh"
if [[ ! -f "$HUB" ]]; then
  echo "bubbles-graph-neighbors: composer not found at $HUB" >&2
  exit 2
fi

# Node-existence guard (node identity ONLY — never edge/in-degree derivation).
# Mirrors the composer's own known-node discovery surfaces so the verb can tell
# a genuinely-unknown node apart from a real node that simply has 0 dependents.
node_is_known() {
  local n="$1"
  case "$n" in
    G[0-9][0-9][0-9])
      local gates="$ROOT/bubbles/registry/gates.yaml"
      [[ -f "$gates" ]] && grep -qE "^  ${n}:" "$gates"
      ;;
    *.sh)
      [[ -f "$ROOT/bubbles/scripts/$n" || -f "$ROOT/bubbles/scripts/guards/$n" ]]
      ;;
    *.md)
      [[ -f "$ROOT/agents/bubbles_shared/$n" ]]
      ;;
    *)
      return 1
      ;;
  esac
}

if ! node_is_known "$NODE"; then
  printf '{"error":"unknown node","node":"%s","detail":"not a known bubbles/scripts[/guards]/*.sh basename, agents/bubbles_shared/*.md module, or Gxxx gate id"}\n' \
    "$NODE" >&2
  exit 3
fi

exec bash "$HUB" --node "$NODE" --format "$FORMAT" --root "$ROOT"
