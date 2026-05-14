#!/usr/bin/env bash
# spec-047 R7: trace mode (`x`) added to surface the exact failing
# command in CI logs even when only step-level metadata is available
# externally. No secrets pass through this script — only apt + go test
# invocations — so command tracing is safe to leave on in CI.
set -euxo pipefail

# spec-047 R2R-CI: TestSSTLoader_RejectsDevPostgresPassword_HomeLab in
# internal/config invokes scripts/commands/config.sh which calls envsubst
# (provided by gettext-base on Debian/Ubuntu). The golang:bookworm test
# image used by run_go_tooling does not include gettext-base by default.
# Install it once per invocation if missing — keeps the test container
# image standard while satisfying the SST loader contract.
if ! command -v envsubst >/dev/null 2>&1; then
  echo "[go-unit] envsubst missing — installing gettext-base"
  apt-get update -qq
  apt-get install -y --no-install-recommends gettext-base
  echo "[go-unit] gettext-base install OK"
else
  echo "[go-unit] envsubst already present"
fi

cd /workspace
echo "[go-unit] starting go test ./..."
go test "$@" ./...
echo "[go-unit] go test ./... finished OK"