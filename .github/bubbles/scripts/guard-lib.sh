#!/usr/bin/env bash
#
# guard-lib.sh — shared helpers for state-transition-guard.sh and its sub-guards
# (extracted in v6.1 / R1 as the first step of the guard split + the BUG-001
# reliability fix). Sourced, not executed.
#
# Provides:
#   bubbles_run_with_timeout <secs> <cmd...>   portable timeout (124 on timeout)
#   bubbles_pruned_find       <root> <pred...> find that prunes generated dirs
#
# These convert two BUG-001 failure modes into bounded, observable behavior:
#   - sub-guard invocations that hang with no timeout
#   - whole-repo find walks that traverse .git / node_modules / target / build
#
# Idempotent: guarded against double-source.

[[ -n "${_BUBBLES_GUARD_LIB_SOURCED:-}" ]] && return 0
_BUBBLES_GUARD_LIB_SOURCED=1

# Portable command timeout. Prefers GNU `timeout`, then `gtimeout`, else a bash
# watchdog fallback. Returns the command's exit code; 124 on timeout (matching
# GNU timeout). Caller-supplied stdout/stderr redirections are inherited.
bubbles_run_with_timeout() {
  local secs="$1"; shift
  if command -v timeout >/dev/null 2>&1; then
    timeout "${secs}s" "$@"
    return $?
  fi
  if command -v gtimeout >/dev/null 2>&1; then
    gtimeout "${secs}s" "$@"
    return $?
  fi
  # Fallback watchdog (rare: only hosts without coreutils timeout).
  "$@" &
  local cmd_pid=$!
  ( sleep "$secs"; kill -TERM "$cmd_pid" 2>/dev/null ) &
  local watch_pid=$!
  local rc=0
  wait "$cmd_pid" 2>/dev/null || rc=$?
  kill -TERM "$watch_pid" 2>/dev/null || true
  wait "$watch_pid" 2>/dev/null || true
  # Normalize a watchdog SIGTERM (143) to GNU timeout's 124.
  [[ "$rc" -eq 143 ]] && rc=124
  return $rc
}

# find that prunes high-fan-out generated directories so whole-repo walks do not
# traverse .git / node_modules / target / build caches. Usage:
#   bubbles_pruned_find <root> <find-predicate...>   # predicate SHOULD end -print
bubbles_pruned_find() {
  local root="$1"; shift
  find "$root" \
    \( -type d \( -name .git -o -name node_modules -o -name target -o -name vendor \
       -o -name dist -o -name build -o -name .venv -o -name venv -o -name __pycache__ \
       -o -name coverage -o -name .bubbles-cache -o -name .next -o -name .gradle \) -prune \) \
    -o \( "$@" \)
}
