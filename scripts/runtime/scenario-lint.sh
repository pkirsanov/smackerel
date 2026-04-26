#!/usr/bin/env bash
# Spec 037 Scope 10 — wire cmd/scenario-lint into ./smackerel.sh check.
#
# Runs the scenario-lint binary against AGENT_SCENARIO_DIR (sourced from
# the generated env file) and exits non-zero on any rejection. Called
# from inside the Go tooling docker container (golang:1.24.3-bookworm)
# by smackerel.sh check, so `go` is available and the workspace is
# mounted at /workspace.
#
# Exit codes:
#   0 = clean (every YAML in the directory passed every load-time rule
#       from spec 037 design §2.2, OR none of them were scenario YAMLs)
#   1 = at least one scenario was rejected by the loader
#   2 = usage error (env unset, dir missing)
#
# We do NOT require AGENT_* env vars to be set in the linter's own
# environment — the linter only needs the directory + glob, which we
# read from config/generated/<env>.env via grep. The runtime config
# block (provider routes, trace settings, ...) is irrelevant to a
# pure-static scenario lint.

set -euo pipefail

cd /workspace

env_file_arg="${1:-config/generated/dev.env}"
if [[ ! -f "$env_file_arg" ]]; then
  echo "scenario-lint: env file not found: $env_file_arg" >&2
  exit 2
fi

scenario_dir="$(grep -E '^AGENT_SCENARIO_DIR=' "$env_file_arg" | tail -n 1 | cut -d= -f2-)"
scenario_glob="$(grep -E '^AGENT_SCENARIO_GLOB=' "$env_file_arg" | tail -n 1 | cut -d= -f2-)"

if [[ -z "$scenario_dir" ]]; then
  echo "scenario-lint: AGENT_SCENARIO_DIR is empty in $env_file_arg" >&2
  exit 2
fi
if [[ -z "$scenario_glob" ]]; then
  scenario_glob='*.yaml'
fi
if [[ ! -d "$scenario_dir" ]]; then
  echo "scenario-lint: scenario dir does not exist: $scenario_dir" >&2
  exit 2
fi

echo "scenario-lint: scanning $scenario_dir (glob: $scenario_glob)"
go run ./cmd/scenario-lint -glob "$scenario_glob" "$scenario_dir"
