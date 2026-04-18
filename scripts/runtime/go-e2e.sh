#!/usr/bin/env bash
set -euo pipefail

cd /workspace
go test -tags e2e -v -count=1 -timeout 300s ./tests/e2e/...
