#!/usr/bin/env bash

bubbles_unset_git_local_env() {
  local git_local_env_vars
  local git_local_env_var

  git_local_env_vars="$(git rev-parse --local-env-vars)"
  while IFS= read -r git_local_env_var; do
    [[ -n "$git_local_env_var" ]] || continue
    unset "$git_local_env_var"
  done <<< "$git_local_env_vars"
}
