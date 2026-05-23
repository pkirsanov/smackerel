#!/usr/bin/env bash
set -euo pipefail

# spec-052 chaos finding: integration tests that shell out to
# scripts/commands/config.sh (e.g.,
# tests/integration/config_validate_test.go,
# tests/integration/ollama_config_contract_test.go,
# tests/integration/drive/drive_config_contract_test.go) require
# envsubst (gettext-base) which is not present in the
# golang:bookworm base image. Use the shared helper so all four
# go-*.sh wrappers share one envsubst-install implementation.
# shellcheck source=scripts/runtime/_ensure_envsubst.sh
source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
ensure_envsubst "go-integration"

cd /workspace
go test -p 1 -tags integration -v -count=1 -timeout 300s ./tests/integration/... ./internal/notification/...
