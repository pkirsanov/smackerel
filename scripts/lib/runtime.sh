#!/usr/bin/env bash
set -euo pipefail

SMACKEREL_RUNTIME_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SMACKEREL_REPO_ROOT="$(cd "$SMACKEREL_RUNTIME_LIB_DIR/../.." && pwd)"

smackerel_repo_root() {
  printf '%s\n' "$SMACKEREL_REPO_ROOT"
}

smackerel_env_file() {
  printf '%s/config/generated/%s.env\n' "$SMACKEREL_REPO_ROOT" "$1"
}

smackerel_generate_config() {
  local target_env="$1"
  shift || true
  bash "$SMACKEREL_REPO_ROOT/scripts/commands/config.sh" --env "$target_env" "$@"
}

smackerel_require_env_file() {
  local target_env="$1"
  local env_file

  env_file="$(smackerel_env_file "$target_env")"
  if [[ ! -f "$env_file" || ! -r "$env_file" ]]; then
    smackerel_generate_config "$target_env" >/dev/null
  fi
  printf '%s\n' "$env_file"
}

smackerel_env_value() {
  local env_file="$1"
  local key="$2"

  awk -F= -v target="$key" '$1 == target { print substr($0, length($1) + 2); exit }' "$env_file"
}

smackerel_compose_project() {
  local target_env="$1"
  local env_file

  env_file="$(smackerel_require_env_file "$target_env")"
  smackerel_env_value "$env_file" "COMPOSE_PROJECT"
}

smackerel_is_truthy() {
  case "$1" in
    true|TRUE|yes|YES|1|on|ON)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

smackerel_compose() {
  local target_env="$1"
  shift

  local env_file
  local compose_project
  local enable_ollama
  local args=()

  env_file="$(smackerel_require_env_file "$target_env")"
  compose_project="$(smackerel_env_value "$env_file" "COMPOSE_PROJECT")"
  enable_ollama="$(smackerel_env_value "$env_file" "ENABLE_OLLAMA")"

  args=(docker compose --project-name "$compose_project" --env-file "$env_file" -f "$SMACKEREL_REPO_ROOT/docker-compose.yml")
  if smackerel_is_truthy "$enable_ollama"; then
    args+=(--profile ollama)
  fi
  # Spec 064 SCOPE-07 — `searxng` compose profile gates the self-hosted
  # SearxNG container that backs the open-knowledge web provider. Enabled
  # via ENABLE_SEARXNG=true in the generated env file (test env auto-on;
  # dev/self-hosted opt-in by flipping environments.<env>.searxng_enabled).
  enable_searxng="$(smackerel_env_value "$env_file" "ENABLE_SEARXNG")"
  if smackerel_is_truthy "$enable_searxng"; then
    args+=(--profile searxng)
  fi
  # Spec 061 design §18.4 — the `test` compose profile contains the
  # in-tree nginx stub-providers container that shell e2e fixtures
  # (BS-003/BS-006/...) target instead of real external HTTP providers.
  # The profile is enabled iff TARGET_ENV=test so production/dev never
  # bring up the stub.
  if [[ "$target_env" == "test" ]]; then
    args+=(--profile test)
  fi

  # Prevent docker compose exec from hanging on stdin in non-interactive contexts
  # (e.g. when run under timeout or piped shells)
  if [[ "${1:-}" == "exec" ]]; then
    "${args[@]}" "$@" </dev/null
  else
    "${args[@]}" "$@"
  fi
}

# BUG-099-002 — portable drop-in for GNU `timeout [--kill-after=<grace>] <seconds> <cmd...>`.
#
# macOS ships no bare `timeout` on PATH (GNU coreutils installs it as `gtimeout`),
# so test-lane call sites that invoke `timeout` directly die with
# `timeout: command not found` (exit 127) before the stack even starts. Per
# .github/instructions/wsl-macos-compatibility.instructions.md this resolves
# `timeout` -> `gtimeout` -> a watchdog fallback and preserves GNU timeout's
# exit-124-on-timeout semantics.
#
# Accepts the SAME argv as GNU `timeout`: an optional leading
# `--kill-after=<grace>` (both `timeout` and `gtimeout` support it; the watchdog
# approximates it as SIGTERM at <seconds> then SIGKILL <grace> later) followed by
# <seconds> and the command. Drop-in: swap `timeout` -> `smackerel_run_with_timeout`.
# For call sites that exec a binary (e.g. wrapped by env/setsid) or that do not
# source this lib, use the sibling executable scripts/lib/run-with-timeout.sh.
smackerel_run_with_timeout() {
  local rc=0

  if command -v timeout >/dev/null 2>&1; then
    timeout "$@" || rc=$?  # portable-ok: raw timeout guarded by 'command -v timeout' above; gtimeout + watchdog fallbacks follow
    return "$rc"
  fi
  if command -v gtimeout >/dev/null 2>&1; then
    gtimeout "$@" || rc=$?
    return "$rc"
  fi

  # Watchdog fallback: neither `timeout` nor `gtimeout` is on PATH.
  local kill_after=""
  if [[ "${1:-}" == --kill-after=* ]]; then
    kill_after="${1#--kill-after=}"
    shift
  fi
  if [[ $# -lt 2 ]]; then
    echo "smackerel_run_with_timeout: usage: [--kill-after=<grace>] <seconds> <cmd...>" >&2
    return 2
  fi
  local seconds="${1%[smh]}"
  shift
  local grace="${kill_after%[smh]}"

  "$@" &
  local cmd_pid=$!
  (
    sleep "$seconds"
    kill -TERM "$cmd_pid" 2>/dev/null || exit 0
    if [[ -n "$grace" ]]; then
      sleep "$grace"
      kill -KILL "$cmd_pid" 2>/dev/null || true
    fi
  ) &
  local watch_pid=$!

  wait "$cmd_pid" 2>/dev/null || rc=$?
  kill -TERM "$watch_pid" 2>/dev/null || true
  wait "$watch_pid" 2>/dev/null || true
  # Map a watchdog SIGTERM (143) / SIGKILL (137) to GNU timeout's 124.
  if [[ "$rc" -eq 143 || "$rc" -eq 137 ]]; then
    rc=124
  fi
  return "$rc"
}