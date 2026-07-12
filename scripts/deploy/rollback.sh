#!/usr/bin/env bash
# scripts/deploy/rollback.sh — pointer-swap rollback wrapper.
#
# Usage:
#   bash scripts/deploy/rollback.sh --target self-hosted
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

TARGET=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)   TARGET="$2"; shift 2 ;;
    --target=*) TARGET="${1#*=}"; shift ;;
    *) echo "ERROR: unknown arg: $1" >&2; exit 1 ;;
  esac
done

[[ -n "$TARGET" ]] || { echo "ERROR: --target required" >&2; exit 1; }

exec "$REPO_ROOT/smackerel.sh" deploy-target "$TARGET" rollback
