---
applyTo: "**"
---

# WSL + macOS Compatibility Policy (NON-NEGOTIABLE)

Smackerel commands and scripts must remain compatible across WSL2 (Ubuntu) and
macOS.

## Core Rule

**No Linux-only command assumptions without guarded fallback.**

## Timeout Compatibility (Required)

`timeout` might be unavailable on macOS. Use:

1. `timeout`
2. `gtimeout`
3. watchdog fallback

Timeout fallback must preserve exit `124` semantics.

## Portable Timeout Helper

```bash
run_with_timeout() {
  local seconds="$1"
  shift

  if command -v timeout >/dev/null 2>&1; then
    timeout "${seconds}s" "$@"
    return $?
  fi

  if command -v gtimeout >/dev/null 2>&1; then
    gtimeout "${seconds}s" "$@"
    return $?
  fi

  "$@" &
  local cmd_pid=$!
  (sleep "$seconds"; kill -TERM "$cmd_pid" 2>/dev/null) &
  local watch_pid=$!
  local rc=0
  wait "$cmd_pid" 2>/dev/null || rc=$?
  kill -TERM "$watch_pid" 2>/dev/null || true
  wait "$watch_pid" 2>/dev/null || true
  [[ "$rc" -eq 143 ]] && rc=124
  return "$rc"
}
```

## Additional Rules

- Guard GNU/BSD command differences.
- Do not introduce unsupported flags without capability checks.
- Keep command examples runnable on both WSL and macOS.
