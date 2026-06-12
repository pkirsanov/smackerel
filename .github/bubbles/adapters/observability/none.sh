#!/usr/bin/env bash
# bubbles/adapters/observability/none.sh — no-op telemetry adapter.
#
# Default adapter when no live telemetry source is wired. Each verb returns the
# NEUTRAL EMPTY VALUE for its canonical normalized shape (R2-D) and exits 0.
# Consumers that depend on telemetry enrichment MUST gracefully skip when the
# adapter returns an empty value.
#
# Canonical per-verb shapes (validated by observability-adapter-lint.sh):
#   fetch-alerts          → JSON array  → neutral empty value: []
#   fetch-slo-burn        → JSON map    → neutral empty value: {}
#   fetch-error-rate      → JSON map    → neutral empty value: {}
#   fetch-deploy-impact   → JSON map    → neutral empty value: {}
#
# The `fetch-alerts` array vs `{}` map split was introduced in IMP-001 SCOPE-3a
# (R2-D) to resolve the prior doc-vs-lint-vs-impl contradiction in which every
# verb returned `{}` while the contract documented `fetch-alerts` as an array.

set -euo pipefail

VERB="${1:-}"

case "$VERB" in
  fetch-alerts)
    # Alerts normalize to a JSON ARRAY of alert objects; neutral empty is [].
    echo '[]'
    exit 0
    ;;
  fetch-slo-burn|fetch-error-rate|fetch-deploy-impact)
    # These three normalize to JSON MAPs (key -> value); neutral empty is {}.
    echo '{}'
    exit 0
    ;;
  -h|--help|"")
    cat >&2 <<'EOF'
none.sh — no-op telemetry adapter
Usage: none.sh <verb>
Verbs: fetch-alerts (-> []) | fetch-slo-burn (-> {}) | fetch-error-rate (-> {}) | fetch-deploy-impact (-> {})
EOF
    exit 0
    ;;
  *)
    echo "[none][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
