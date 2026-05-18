#!/usr/bin/env bash
# Shared library: idempotently ensures the `envsubst` binary (from
# the `gettext-base` Debian/Ubuntu package) is available inside the
# go-tooling container before any wrapper that may shell out to
# `scripts/commands/config.sh` runs Go tests.
#
# Rationale (spec 045 + spec 047 R2R-CI + spec 052 chaos-phase finding):
#   * `scripts/commands/config.sh` invokes `envsubst` to substitute a
#     known whitelist of Prometheus template variables.
#   * Multiple test layers shell out to that script for SST-loader
#     contract assertions:
#       - internal/config (unit, via go-unit.sh)
#       - tests/integration/config_validate_test.go, ollama_config_contract_test.go,
#         drive/drive_config_contract_test.go (integration, via go-integration.sh)
#       - tests/e2e/drive/drive_foundation_e2e_test.go (e2e, via go-e2e.sh)
#       - tests/stress/... (potential, via go-stress.sh)
#   * The base test image `golang:1.25.10-bookworm` does NOT include
#     `gettext-base` by default, so without this helper any wrapper
#     other than go-unit.sh would fail with exit 127
#     `envsubst: command not found` (spec 052 chaos observation).
#
# Design: idempotent, no-op when envsubst is already present. Safe to
# source repeatedly from any wrapper without retriggering apt-get.
#
# Usage (in any `scripts/runtime/go-*.sh` wrapper):
#   # shellcheck source=scripts/runtime/_ensure_envsubst.sh
#   source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
#   ensure_envsubst "go-integration"   # tag for log prefix
#
# This file is a library — it does NOT call `set -e` of its own
# because callers already do (per their explicit error-handling
# contract). It does NOT run anything at source time; callers must
# invoke `ensure_envsubst <tag>` to trigger the check.

ensure_envsubst() {
  local tag="${1:-go-tooling}"
  if command -v envsubst >/dev/null 2>&1; then
    echo "[${tag}] envsubst already present"
    return 0
  fi
  echo "[${tag}] envsubst missing — installing gettext-base"
  apt-get update -qq
  apt-get install -y --no-install-recommends gettext-base
  echo "[${tag}] gettext-base install OK"
}
