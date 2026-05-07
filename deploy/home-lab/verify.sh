#!/usr/bin/env bash
# verify.sh — post-apply health checks. Read-only. Idempotent.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PARAMS="$SCRIPT_DIR/params.yaml"

[[ -f "$PARAMS" ]] || { echo "ERROR: $PARAMS missing" >&2; exit 1; }

# Health endpoint — placeholder; real adapter reads CORE_EXTERNAL_URL from the extracted bundle.
HEALTH_URL="${SMACKEREL_HEALTH_URL:-http://127.0.0.1:8080/api/health}"

echo "▶ verify: GET $HEALTH_URL"
if curl --max-time 5 -fsS "$HEALTH_URL" >/dev/null; then
  echo "verify OK"
  exit 0
fi

echo "ERROR: health check failed at $HEALTH_URL" >&2
exit 1
