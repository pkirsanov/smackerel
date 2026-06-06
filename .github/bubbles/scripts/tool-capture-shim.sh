#!/usr/bin/env bash
#
# tool-capture-shim.sh — sourceable auto-capture shim (review R2, v6.1).
#
# Moves tool-call evidence from a HEURISTIC (grep the markdown) toward a
# GROUND-TRUTH lookup: when this shim is sourced, gate-relevant commands are
# transparently routed through tool-log.sh, which appends a
# {ts, cmd, exitCode, stdoutHash, ...} record to
# .specify/runtime/tool-calls.jsonl. The evidence bridge (M2 / F1) and the MCP
# `query_tool_log` tool then answer "did this DoD's command actually run?"
# deterministically instead of inferring it from prose.
#
# Two modes:
#
#   1. EXPLICIT one-shot capture (no shadowing):
#        source bubbles/scripts/tool-capture-shim.sh
#        bubbles_capture -- ./myproject.sh test --rust
#
#   2. AUTO-CAPTURE (shadow an allowlist of commands as shell functions):
#        BUBBLES_AUTOCAPTURE=1 \
#        BUBBLES_CAPTURE_COMMANDS="cargo npm pytest go curl psql" \
#        source bubbles/scripts/tool-capture-shim.sh
#      After sourcing, invoking any listed command (e.g. `cargo test`) auto-logs.
#
# HONEST LIMITATION (documented on purpose): in a stock VS Code Copilot session
# the agent runs commands directly in the terminal, NOT through a wrapper. The
# framework cannot transparently capture those unless (a) the project CLI routes
# gate-relevant subcommands through tool-log.sh, or (b) this shim is sourced into
# the shell the agent uses. This shim is the supported mechanism for (b); the
# MCP server covers commands it executes itself. Markdown evidence remains a
# valid fallback when no tool-log entry exists.
#
# This file is meant to be SOURCED. Running it directly prints usage.

# Resolve tool-log.sh next to this shim.
if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
  _BUBBLES_SHIM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
  _BUBBLES_SHIM_DIR="$(pwd)"
fi
_BUBBLES_TOOL_LOG="${BUBBLES_TOOL_LOG_SCRIPT:-$_BUBBLES_SHIM_DIR/tool-log.sh}"

# Explicit one-shot capture. `bubbles_capture -- <cmd...>` or `bubbles_capture <cmd...>`.
bubbles_capture() {
  if [[ "${1:-}" == "--" ]]; then
    shift
  fi
  if [[ $# -lt 1 ]]; then
    echo "bubbles_capture: usage: bubbles_capture -- <command> [args...]" >&2
    return 2
  fi
  if [[ ! -f "$_BUBBLES_TOOL_LOG" ]]; then
    echo "bubbles_capture: tool-log.sh not found at $_BUBBLES_TOOL_LOG" >&2
    # Fail open: still run the command so we never break a workflow.
    "$@"
    return $?
  fi
  bash "$_BUBBLES_TOOL_LOG" "$@"
}

# Install shadow wrappers for an allowlist so listed commands auto-capture.
_bubbles_install_wrappers() {
  local commands="${BUBBLES_CAPTURE_COMMANDS:-cargo npm pnpm yarn go pytest python python3 curl psql}"
  local name real
  for name in $commands; do
    # Resolve the REAL executable path so the wrapper never recurses into itself.
    real="$(builtin type -P "$name" 2>/dev/null || true)"
    [[ -n "$real" ]] || continue
    eval "${name}() { bubbles_capture -- \"$real\" \"\$@\"; }"
  done
}

# Remove the shadow wrappers (restore plain command resolution).
bubbles_capture_uninstall() {
  local commands="${BUBBLES_CAPTURE_COMMANDS:-cargo npm pnpm yarn go pytest python python3 curl psql}"
  local name
  for name in $commands; do
    unset -f "$name" 2>/dev/null || true
  done
}

if [[ "${BUBBLES_AUTOCAPTURE:-0}" == "1" ]]; then
  _bubbles_install_wrappers
fi

# If executed (not sourced), print usage and exit non-zero so misuse is obvious.
if [[ "${BASH_SOURCE[0]:-}" == "${0:-}" ]]; then
  cat >&2 <<'USAGE'
tool-capture-shim.sh is meant to be SOURCED, not executed:

  source bubbles/scripts/tool-capture-shim.sh
  bubbles_capture -- ./myproject.sh test

  # auto-capture an allowlist:
  BUBBLES_AUTOCAPTURE=1 BUBBLES_CAPTURE_COMMANDS="cargo npm curl" \
    source bubbles/scripts/tool-capture-shim.sh
USAGE
  exit 2
fi
