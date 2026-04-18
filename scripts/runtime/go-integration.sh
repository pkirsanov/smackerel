#!/usr/bin/env bash
set -euo pipefail

cd /workspace
go test -tags integration -v -count=1 -timeout 300s ./tests/integration/...
