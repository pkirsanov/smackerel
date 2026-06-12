#!/usr/bin/env bash
set -uo pipefail

# prometheus-adapter-fetch-selftest.sh  (IMP-001 P2 — v7.10.1 review fix)
#
# Exercises the LIVE fetch path of bubbles/adapters/observability/prometheus.sh
# end-to-end WITHOUT a real Prometheus server, by shadowing `curl` on PATH with
# a fake that returns canned RAW provider envelopes. This proves the real
# pipeline the shape-only adapter-lint never executed:
#   * verb dispatch + required-env validation,
#   * URL / ?query= construction (urlencode of the configured query),
#   * the curl→normalize pipeline yields the CONTRACTED shapes
#     (fetch-alerts → bare array; slo-burn/error-rate → bare map;
#      deploy-impact → sha→{service,regressionDelta} map).
#
# The fake curl also records the URL it was called with so the test can assert
# the adapter hit the right /api/v1 path with the urlencoded query.
#
# jq is required (the adapter needs it for urlencode + normalize). If jq is
# absent the selftest SKIPs (exit 0) — the real fail-closed behavior is proven
# by the SLO guard's missing-parser case, not here.

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ADAPTER="$REPO_ROOT/adapters/observability/prometheus.sh"

if ! command -v jq >/dev/null 2>&1; then
  echo "SKIP: prometheus-adapter-fetch-selftest (jq not installed)"
  exit 0
fi
if [[ ! -f "$ADAPTER" ]]; then
  echo "prometheus-adapter-fetch-selftest: adapter not found: $ADAPTER" >&2
  exit 1
fi

WORKSPACE="$(mktemp -d)"
trap 'rm -rf "$WORKSPACE"' EXIT

PASS_COUNT=0
FAIL_COUNT=0
ok() { echo "[selftest] PASS: $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
ko() { echo "[selftest] FAIL: $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }

# --- Fake curl: a real curl binary is NOT used. -------------------------
# It writes its full argv to $CURL_ARGS_FILE and emits the canned raw envelope
# for the requested path. The adapter invokes:
#   curl --max-time N --silent --show-error --fail [auth] <BASE_URL><path>
# so the URL is always the LAST argument.
FAKEBIN="$WORKSPACE/bin"
mkdir -p "$FAKEBIN"
cat > "$FAKEBIN/curl" <<'FAKE'
#!/usr/bin/env bash
# Fake curl for prometheus adapter fetch selftest. Last arg is the URL.
url="${!#}"
printf '%s\n' "$url" >> "${CURL_ARGS_FILE:?CURL_ARGS_FILE unset}"
if [[ -n "${FAKE_PROMETHEUS_EMPTY:-}" && "$url" == */api/v1/query?query=* ]]; then
  printf '%s' '{"status":"success","data":{"resultType":"vector","result":[]}}'
  exit 0
fi
case "$url" in
  */api/v1/alerts)
    printf '%s' '{"status":"success","data":{"alerts":[{"labels":{"alertname":"HighLatency","service":"gateway","severity":"critical"},"state":"firing","activeAt":"2026-06-11T00:00:00Z","annotations":{"summary":"gateway.request p99 above SLO target"}}]}}'
    ;;
  */api/v1/query?query=*deploy*|*api/v1/query?query=*delta*)
    printf '%s' '{"status":"success","data":{"resultType":"vector","result":[{"metric":{"source_sha":"a1b2c3d4e5f6","service":"gateway"},"value":[1623456789,"0.03"]}]}}'
    ;;
  */api/v1/query?query=*)
    printf '%s' '{"status":"success","data":{"resultType":"vector","result":[{"metric":{"service":"gateway.request"},"value":[1623456789,"0.5"]}]}}'
    ;;
  *)
    echo "fake-curl: unexpected url: $url" >&2
    exit 22
    ;;
esac
exit 0
FAKE
chmod +x "$FAKEBIN/curl"

CURL_ARGS_FILE="$WORKSPACE/curl-args"
: > "$CURL_ARGS_FILE"
export CURL_ARGS_FILE

run_verb() { # $1=verb ; optional $2=empty-vector ; sets RC + OUT
  local verb="$1"
  local empty="${2:-}"
  local of="$WORKSPACE/out.last"
  PATH="$FAKEBIN:$PATH" \
  FAKE_PROMETHEUS_EMPTY="$empty" \
  PROMETHEUS_BASE_URL="http://mock-prometheus:9090" \
  PROMETHEUS_CURL_MAX_TIME="5" \
  PROMETHEUS_QUERY_SLO_BURN="slo:burn_rate" \
  PROMETHEUS_QUERY_ERROR_RATE="rate(http_errors_total[5m])" \
  PROMETHEUS_QUERY_DEPLOY_IMPACT="delta(deploy_regression[1h])" \
    bash "$ADAPTER" "$verb" >"$of" 2>"$WORKSPACE/err.last"
  RC=$?
  OUT="$(cat "$of")"
}

assert_rc0()  { [[ "$RC" -eq 0 ]] && ok "$1 (exit 0)" || { ko "$1: expected exit 0, got $RC"; printf '  --- stderr ---\n%s\n' "$(cat "$WORKSPACE/err.last")" >&2; }; }
assert_jq()   { if jq -e "$1" >/dev/null 2>&1 <<<"$OUT"; then ok "$2"; else ko "$2: jq filter failed [$1]"; printf '  --- output ---\n%s\n' "$OUT" >&2; fi; }
assert_grep() { if grep -qF -- "$1" "$CURL_ARGS_FILE"; then ok "$2"; else ko "$2: curl never called with '$1'"; printf '  --- urls ---\n%s\n' "$(cat "$CURL_ARGS_FILE")" >&2; fi; }

# --- fetch-alerts: raw envelope → bare normalized ARRAY -------------------
run_verb fetch-alerts
assert_rc0 "fetch-alerts live path"
assert_jq 'type == "array"' "fetch-alerts yields a bare JSON array (not the raw envelope)"
assert_jq '.[0].id == "HighLatency" and .[0].service == "gateway" and .[0].severity == "critical"' "fetch-alerts normalizes label fields"
assert_grep "/api/v1/alerts" "fetch-alerts hits /api/v1/alerts"

# --- fetch-slo-burn: raw vector → bare MAP (service → float) -------------
run_verb fetch-slo-burn
assert_rc0 "fetch-slo-burn live path"
assert_jq 'type == "object" and (."gateway.request" == 0.5)' "fetch-slo-burn yields a bare map service→float (not the raw envelope)"
assert_grep "query=slo%3Aburn_rate" "fetch-slo-burn urlencodes the configured query"

# --- fetch-error-rate: raw vector → bare MAP -----------------------------
run_verb fetch-error-rate
assert_rc0 "fetch-error-rate live path"
assert_jq 'type == "object" and (."gateway.request" == 0.5)' "fetch-error-rate yields a bare map service→float"
assert_grep "query=rate%28http_errors_total%5B5m%5D%29" "fetch-error-rate urlencodes the exact rate() query"

# --- fetch-deploy-impact: raw vector → sha→{service,regressionDelta} map --
run_verb fetch-deploy-impact
assert_rc0 "fetch-deploy-impact live path"
assert_jq 'type == "object" and (."a1b2c3d4e5f6".service == "gateway") and (."a1b2c3d4e5f6".regressionDelta == 0.03)' "fetch-deploy-impact yields the contracted sha→{service,regressionDelta} map"
assert_grep "query=delta%28deploy_regression%5B1h%5D%29" "fetch-deploy-impact urlencodes the exact deploy-impact query"

# --- empty vector responses normalize to empty maps ----------------------
run_verb fetch-slo-burn empty
assert_rc0 "fetch-slo-burn empty-vector live path"
assert_jq 'type == "object" and length == 0' "fetch-slo-burn empty vector normalizes to {}"
run_verb fetch-deploy-impact empty
assert_rc0 "fetch-deploy-impact empty-vector live path"
assert_jq 'type == "object" and length == 0' "fetch-deploy-impact empty vector normalizes to {}"

# --- adversarial: a raw-envelope leak would FAIL the array/map assertions -
# Prove the test is not a rubber stamp: if the adapter regressed to emitting the
# raw Prometheus envelope, fetch-alerts (.status field, object not array) and
# the query verbs (object with .status, not service keys) would both fail the
# jq assertions above. The within-target asserts above are the guard.

echo ""
echo "prometheus-adapter-fetch-selftest: $PASS_COUNT passed, $FAIL_COUNT failed"
if (( FAIL_COUNT == 0 )); then
  echo "prometheus-adapter-fetch selftest passed."
  exit 0
fi
echo "prometheus-adapter-fetch selftest FAILED." >&2
exit 1
