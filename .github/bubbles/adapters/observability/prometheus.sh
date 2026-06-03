#!/usr/bin/env bash
# bubbles/adapters/observability/prometheus.sh — Prometheus telemetry adapter.
#
# Queries the Prometheus HTTP API at ${PROMETHEUS_BASE_URL} (e.g.
# http://localhost:9090). The operator MUST set PROMETHEUS_BASE_URL before
# invoking any verb. NO default URL — fail-fast.
#
# Verbs (all 4 are mandatory per the observability adapter contract):
#   fetch-alerts          → /api/v1/alerts (active alerts)
#   fetch-slo-burn        → /api/v1/query?query=slo:burn_rate
#   fetch-error-rate      → /api/v1/query?query=rate(http_requests_errors_total[5m])
#   fetch-deploy-impact   → /api/v1/query?query=delta(deploy_regression_score[1h])
#
# Output: structured JSON to stdout. Adapter failure exits 1; framework
# treats that as "telemetry unavailable", NOT as a framework failure.
#
# Operator override hooks (env vars, all optional):
#   PROMETHEUS_BASE_URL              required; no default
#   PROMETHEUS_BEARER_TOKEN          optional bearer token for /api/v1/*
#   PROMETHEUS_CURL_MAX_TIME         required; no default
#   PROMETHEUS_QUERY_SLO_BURN        required; no default
#   PROMETHEUS_QUERY_ERROR_RATE      required; no default
#   PROMETHEUS_QUERY_DEPLOY_IMPACT   required; no default

set -euo pipefail

VERB="${1:-}"

[[ -n "${PROMETHEUS_BASE_URL:-}" ]] || { echo "[prometheus][ERROR] PROMETHEUS_BASE_URL not set" >&2; exit 1; }
[[ -n "${PROMETHEUS_CURL_MAX_TIME:-}" ]] || { echo "[prometheus][ERROR] PROMETHEUS_CURL_MAX_TIME not set" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "[prometheus][ERROR] curl required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "[prometheus][ERROR] jq required for URL encoding" >&2; exit 1; }

AUTH_HEADER=()
[[ -n "${PROMETHEUS_BEARER_TOKEN:-}" ]] && AUTH_HEADER=(-H "Authorization: Bearer ${PROMETHEUS_BEARER_TOKEN}")

urlencode() {
  jq -nr --arg v "$1" '$v|@uri'
}

call() {
  local path="$1"
  curl --max-time "$PROMETHEUS_CURL_MAX_TIME" --silent --show-error --fail "${AUTH_HEADER[@]}" "${PROMETHEUS_BASE_URL}${path}"
}

case "$VERB" in
  fetch-alerts)
    call '/api/v1/alerts'
    ;;
  fetch-slo-burn)
    [[ -n "${PROMETHEUS_QUERY_SLO_BURN:-}" ]] || { echo "[prometheus][ERROR] PROMETHEUS_QUERY_SLO_BURN not set" >&2; exit 1; }
    call "/api/v1/query?query=$(urlencode "$PROMETHEUS_QUERY_SLO_BURN")"
    ;;
  fetch-error-rate)
    [[ -n "${PROMETHEUS_QUERY_ERROR_RATE:-}" ]] || { echo "[prometheus][ERROR] PROMETHEUS_QUERY_ERROR_RATE not set" >&2; exit 1; }
    call "/api/v1/query?query=$(urlencode "$PROMETHEUS_QUERY_ERROR_RATE")"
    ;;
  fetch-deploy-impact)
    [[ -n "${PROMETHEUS_QUERY_DEPLOY_IMPACT:-}" ]] || { echo "[prometheus][ERROR] PROMETHEUS_QUERY_DEPLOY_IMPACT not set" >&2; exit 1; }
    call "/api/v1/query?query=$(urlencode "$PROMETHEUS_QUERY_DEPLOY_IMPACT")"
    ;;
  -h|--help|"")
    cat >&2 <<'EOF'
prometheus.sh — Prometheus telemetry adapter
Usage: PROMETHEUS_BASE_URL=http://host:9090 PROMETHEUS_CURL_MAX_TIME=10 prometheus.sh <verb>
Verbs: fetch-alerts | fetch-slo-burn | fetch-error-rate | fetch-deploy-impact
EOF
    exit 0
    ;;
  *)
    echo "[prometheus][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
