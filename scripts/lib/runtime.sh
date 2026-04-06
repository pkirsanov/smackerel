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
  bash "$SMACKEREL_REPO_ROOT/scripts/commands/config.sh" --env "$1"
}

smackerel_require_env_file() {
  local target_env="$1"
  local env_file

  env_file="$(smackerel_env_file "$target_env")"
  if [[ ! -f "$env_file" ]]; then
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

  "${args[@]}" "$@"
}