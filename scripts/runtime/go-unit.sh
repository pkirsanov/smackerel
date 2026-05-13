#!/usr/bin/env bash
set -euo pipefail

# spec-047 R2R-CI: TestSSTLoader_RejectsDevPostgresPassword_HomeLab in
# internal/config invokes scripts/commands/config.sh which calls envsubst
# (provided by gettext-base on Debian/Ubuntu). The golang:bookworm test
# image used by run_go_tooling does not include gettext-base by default.
# Install it once per invocation if missing — keeps the test container
# image standard while satisfying the SST loader contract.
if ! command -v envsubst >/dev/null 2>&1; then
  apt-get update -qq
  apt-get install -y --no-install-recommends gettext-base
fi

cd /workspace
go test ./...