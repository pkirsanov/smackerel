#!/usr/bin/env bash
# preconditions.sh — verify host has the tools and paths required by apply/rollback.
#
# Idempotent. Read-only. Exits non-zero with a clear message on any missing dependency.
# Adapter contract: this script MUST NOT mutate host state, MUST NOT pull artifacts,
# MUST NOT write any file.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PARAMS="$SCRIPT_DIR/params.yaml"

[[ -f "$PARAMS" ]] || { echo "ERROR: $PARAMS missing" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "ERROR: required command '$1' not found on PATH" >&2; exit 1; }
}

require_cmd docker
require_cmd cosign
require_cmd syft
require_cmd sha256sum
require_cmd tar
require_cmd curl

if ! docker info >/dev/null 2>&1; then
  echo "ERROR: docker daemon not reachable (is the daemon running and is the user in the docker group?)" >&2
  exit 1
fi

echo "preconditions OK"
