#!/usr/bin/env bash
# bubbles/adapters/observability/prometheus.sh — Prometheus telemetry adapter.
#
# Queries the Prometheus HTTP API at ${PROMETHEUS_BASE_URL} (e.g.
# http://localhost:9090). The operator MUST set PROMETHEUS_BASE_URL before
# invoking any live verb. NO default URL — fail-fast.
#
# Verbs (all 4 are mandatory per the observability adapter contract):
#   fetch-alerts          → /api/v1/alerts (active alerts), NORMALIZED to a
#                           bare JSON array (R2-D) — NOT the raw provider
#                           envelope.
#   fetch-slo-burn        → /api/v1/query?query=slo:burn_rate, NORMALIZED to a
#                           bare JSON map (service → float) — NOT the raw vector
#                           envelope.
#   fetch-error-rate      → /api/v1/query?query=rate(...), NORMALIZED to a bare
#                           JSON map (service → float).
#   fetch-deploy-impact   → /api/v1/query?query=delta(...), NORMALIZED to a bare
#                           JSON map (sourceSha → {service, regressionDelta}).
#
# Output: structured JSON to stdout. Adapter failure exits 1; framework
# treats that as "telemetry unavailable", NOT as a framework failure.
#
# Shape selftest (NO live backend, NO env required) — IMP-001 SCOPE-3 T3.2:
#   prometheus.sh selftest <verb>
# emits the canonical normalized SHAPE for <verb> so observability-adapter-lint
# can validate output shape without a Prometheus server. ALL 4 selftest verbs
# now drive a canned RAW provider envelope through the SAME normalizer the live
# path uses (fetch-alerts → normalize_alerts; slo-burn/error-rate →
# normalize_query_map; deploy-impact → normalize_deploy_impact), so the shape
# selftest proves the REAL normalization rather than a hand-written shape.
#
# Live fetch selftest (fake curl, NO live backend) —
# bubbles/scripts/prometheus-adapter-fetch-selftest.sh exercises every live verb
# end-to-end against a shadowed `curl` that returns canned raw envelopes,
# proving the real curl→normalize pipeline (verb dispatch, URL/query
# construction, and normalization) yields the contracted shapes.
#
# Operator override hooks (env vars):
#   PROMETHEUS_BASE_URL              required (live verbs); no default
#   PROMETHEUS_BEARER_TOKEN          optional bearer token for /api/v1/*
#   PROMETHEUS_CURL_MAX_TIME         required (live verbs); no default
#   PROMETHEUS_QUERY_SLO_BURN        required for fetch-slo-burn; no default
#   PROMETHEUS_QUERY_ERROR_RATE      required for fetch-error-rate; no default
#   PROMETHEUS_QUERY_DEPLOY_IMPACT   required for fetch-deploy-impact; no default

set -euo pipefail

VERB="${1:-}"

usage() {
  cat >&2 <<'EOF'
prometheus.sh — Prometheus telemetry adapter
Usage: PROMETHEUS_BASE_URL=http://host:9090 PROMETHEUS_CURL_MAX_TIME=10 prometheus.sh <verb>
Verbs: fetch-alerts | fetch-slo-burn | fetch-error-rate | fetch-deploy-impact
Shape selftest (no live backend): prometheus.sh selftest <verb>
EOF
}

# normalize_alerts: read a raw Prometheus /api/v1/alerts envelope on stdin
# (`{"status":"success","data":{"alerts":[...]}}`) and emit the bare normalized
# JSON ARRAY required by the observability contract (R2-D):
#   [ { id, service, severity, startedAt, summary }, ... ]
normalize_alerts() {
  jq '[ (.data.alerts // [])[] | {
        id:        (.labels.alertname // "unknown"),
        service:   (.labels.service // "unknown"),
        severity:  (.labels.severity // "warning"),
        startedAt: (.activeAt // ""),
        summary:   (.annotations.summary // .labels.alertname // "")
      } ]'
}

# normalize_query_map: read a raw Prometheus /api/v1/query vector response on
# stdin (`{"status":"success","data":{"resultType":"vector","result":[
# {"metric":{"service":"..."},"value":[ts,"0.5"]}, ...]}}`) and emit the bare
# normalized JSON MAP required by the contract for fetch-slo-burn /
# fetch-error-rate (R2-D):  { "<service>": <float>, ... }
# The service key falls back to __name__ then "unknown"; the value is the
# instant-vector sample coerced to a number. Empty result → {}.
normalize_query_map() {
  jq '[ (.data.result // [])[]
        | { ( .metric.service // .metric.__name__ // "unknown" ):
            ( (.value[1] // "0") | tonumber? // 0 ) } ]
      | add // {}'
}

# normalize_deploy_impact: read a raw Prometheus /api/v1/query vector response
# on stdin and emit the contracted deploy-impact MAP (R2-D):
#   { "<sourceSha>": { "service": "<svc>", "regressionDelta": <float> }, ... }
# The sha key falls back through source_sha/sha/commit then "unknown".
normalize_deploy_impact() {
  jq '[ (.data.result // [])[]
        | { ( .metric.source_sha // .metric.sha // .metric.commit // "unknown" ):
            { service:         ( .metric.service // "unknown" ),
              regressionDelta: ( (.value[1] // "0") | tonumber? // 0 ) } } ]
      | add // {}'
}

# --- selftest / help short-circuits (NO live backend, NO env required) ----
case "$VERB" in
  -h|--help|"")
    usage
    exit 0
    ;;
  selftest)
    command -v jq >/dev/null 2>&1 || { echo "[prometheus][ERROR] jq required for selftest" >&2; exit 1; }
    SUB="${2:-}"
    case "$SUB" in
      fetch-alerts)
        # Drive a canned RAW Prometheus envelope through the SAME normalizer the
        # live fetch-alerts path uses, proving it yields a bare JSON array.
        printf '%s' '{"status":"success","data":{"alerts":[{"labels":{"alertname":"HighLatency","service":"gateway","severity":"critical"},"state":"firing","activeAt":"2026-06-11T00:00:00Z","annotations":{"summary":"gateway.request p99 above SLO target"}}]}}' | normalize_alerts
        ;;
      fetch-slo-burn|fetch-error-rate)
        # Drive a canned RAW Prometheus instant-vector envelope through the SAME
        # normalizer the live path uses, proving it yields the contracted map.
        printf '%s' '{"status":"success","data":{"resultType":"vector","result":[{"metric":{"service":"gateway.request"},"value":[1623456789,"0.5"]}]}}' | normalize_query_map
        ;;
      fetch-deploy-impact)
        # Drive a canned RAW envelope through the SAME deploy-impact normalizer.
        printf '%s' '{"status":"success","data":{"resultType":"vector","result":[{"metric":{"source_sha":"a1b2c3d4e5f6","service":"gateway"},"value":[1623456789,"0.03"]}]}}' | normalize_deploy_impact
        ;;
      *)
        echo "[prometheus][ERROR] selftest: unknown verb '$SUB' (expected fetch-alerts|fetch-slo-burn|fetch-error-rate|fetch-deploy-impact)" >&2
        exit 1
        ;;
    esac
    exit 0
    ;;
esac

# --- live verbs require env + curl + jq -----------------------------------
[[ -n "${PROMETHEUS_BASE_URL:-}" ]] || { echo "[prometheus][ERROR] PROMETHEUS_BASE_URL not set" >&2; exit 1; }
[[ -n "${PROMETHEUS_CURL_MAX_TIME:-}" ]] || { echo "[prometheus][ERROR] PROMETHEUS_CURL_MAX_TIME not set" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "[prometheus][ERROR] curl required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "[prometheus][ERROR] jq required for URL encoding + normalization" >&2; exit 1; }

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
    # Normalize the raw Prometheus alerts envelope to a bare JSON array (R2-D).
    call '/api/v1/alerts' | normalize_alerts
    ;;
  fetch-slo-burn)
    [[ -n "${PROMETHEUS_QUERY_SLO_BURN:-}" ]] || { echo "[prometheus][ERROR] PROMETHEUS_QUERY_SLO_BURN not set" >&2; exit 1; }
    call "/api/v1/query?query=$(urlencode "$PROMETHEUS_QUERY_SLO_BURN")" | normalize_query_map
    ;;
  fetch-error-rate)
    [[ -n "${PROMETHEUS_QUERY_ERROR_RATE:-}" ]] || { echo "[prometheus][ERROR] PROMETHEUS_QUERY_ERROR_RATE not set" >&2; exit 1; }
    call "/api/v1/query?query=$(urlencode "$PROMETHEUS_QUERY_ERROR_RATE")" | normalize_query_map
    ;;
  fetch-deploy-impact)
    [[ -n "${PROMETHEUS_QUERY_DEPLOY_IMPACT:-}" ]] || { echo "[prometheus][ERROR] PROMETHEUS_QUERY_DEPLOY_IMPACT not set" >&2; exit 1; }
    call "/api/v1/query?query=$(urlencode "$PROMETHEUS_QUERY_DEPLOY_IMPACT")" | normalize_deploy_impact
    ;;
  *)
    echo "[prometheus][ERROR] unknown verb '$VERB'" >&2
    exit 1
    ;;
esac
