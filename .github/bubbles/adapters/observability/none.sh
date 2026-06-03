#!/usr/bin/env bash
# bubbles/adapters/observability/none.sh — no-op telemetry adapter.
#
# Default adapter when no live telemetry source is wired. All verbs return
# empty JSON objects (`{}`) and exit 0. Consumers that depend on telemetry
# enrichment MUST gracefully skip when the adapter returns empty.
#
# Verbs (all 4 are mandatory per the observability adapter contract):
#   fetch-alerts          → {}
#   fetch-slo-burn        → {}
#   fetch-error-rate      → {}
#   fetch-deploy-impact   → {}

set -euo pipefail

VERB="${1:-}"

case "$VERB" in
  fetch-alerts|fetch-slo-burn|fetch-error-rate|fetch-deploy-impact)
    echo '{}'
    exit 0
    ;;
  -h|--help|"")
    cat >&2 <<'EOF'
none.sh — no-op telemetry adapter
Usage: none.sh <verb>
Verbs: fetch-alerts | fetch-slo-burn | fetch-error-rate | fetch-deploy-impact
EOF
    exit 0
    ;;
  *)
    echo "[none][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
