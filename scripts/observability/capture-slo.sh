#!/usr/bin/env bash
# =============================================================================
# scripts/observability/capture-slo.sh
#
# Capture REAL core SLO telemetry for the IMP-001 observability dogfood and
# emit the G100 evidence file .specify/runtime/observability/<workflow>.slo.json.
#
# Subcommands:
#   run     — hit a LIVE core endpoint under modest concurrent load, measure
#             per-request latency + HTTP status CLIENT-SIDE (curl), then compute
#             + emit. THIS is the real dogfood path; `source` is the load label.
#   compute — compute + emit from a pre-recorded samples file ("<time_s> <code>"
#             one per line). This is the pure, offline-testable core; the unit
#             test drives it with a synthetic fixture and a non-prod --out path.
#             It is NEVER the path that writes real dogfood evidence.
#
# SLI definitions (client-side measurement):
#   errorRatePct    = 100 * (#5xx + #connection-failures) / total
#   availabilityPct = 100 - errorRatePct
#   latencyP99Ms    = p99 (nearest-rank) of per-request wall-clock latency in ms,
#                     computed over requests that received an HTTP response
#
# HONESTY CONTRACT: the `observed` block is produced ONLY from measured samples.
# There is no synthetic fallback. `run` FAILS LOUD if the endpoint is unreachable
# rather than emit a fake "100% healthy" file. SLO targets are read from
# .github/bubbles-project.yaml so the evidence `target` block is the SAME
# contract G100 (observability_slo_evidence_gate) asserts against. There is no
# --skip / --force / --fake flag, and there never will be.
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# --- Defaults ---------------------------------------------------------------
WORKFLOW=""
SLO=""
URL=""
REQUESTS=600
CONCURRENCY=20
SOURCE="stress"
CONFIG="$REPO_ROOT/.github/bubbles-project.yaml"
OUT=""
SAMPLES_FILE=""
SAMPLE_WINDOW=""

# --- Computed observed metrics (set by compute_metrics) ---------------------
OBS_P99=""
OBS_ERR=""
OBS_AVAIL=""
TOTAL=0
RESP_COUNT=0
FAIL_COUNT=0
TARGET_JSON="{}"

info() { echo "capture-slo: $*"; }
err()  { echo "capture-slo: $*" >&2; }

usage() {
  cat <<'EOF'
Usage:
  capture-slo.sh run     --workflow <key> --url <url> [--slo <key>]
                         [--requests N] [--concurrency C] [--source <label>]
                         [--config <path>] [--out <path>]
  capture-slo.sh compute --workflow <key> --samples <file> [--slo <key>]
                         [--source <label>] [--sample-window <ISO8601>]
                         [--config <path>] [--out <path>]

Exit codes: 0 = evidence written; 1 = error / unreachable / no samples; 2 = usage.
There is NO bypass flag.
EOF
}

require_deps() {
  local missing=()
  command -v jq  >/dev/null 2>&1 || missing+=("jq")
  command -v yq  >/dev/null 2>&1 || missing+=("yq (mikefarah v4)")
  command -v awk >/dev/null 2>&1 || missing+=("awk")
  command -v curl >/dev/null 2>&1 || missing+=("curl")
  if [[ ${#missing[@]} -gt 0 ]]; then
    err "required tool(s) missing: ${missing[*]}"
    exit 1
  fi
}

load_target() {
  TARGET_JSON="$(S="$SLO" yq -o=json -I=0 '.traceContracts.observability.slos[strenv(S)] // {}' "$CONFIG" 2>/dev/null || printf '{}')"
  if [[ -z "$TARGET_JSON" || "$TARGET_JSON" == "{}" || "$TARGET_JSON" == "null" ]]; then
    err "no SLO target for '$SLO' in $CONFIG (.traceContracts.observability.slos.$SLO). Declare it before capturing."
    exit 1
  fi
}

# compute_metrics <samples_file>
# samples line format: "<time_total_seconds> <http_code>"
compute_metrics() {
  local f="$1"
  TOTAL="$(awk 'END{print NR+0}' "$f")"
  if [[ "$TOTAL" -eq 0 ]]; then
    err "no samples collected — cannot compute SLO evidence"
    exit 1
  fi
  # Failures: connection failures (curl code 000) OR server errors (5xx).
  FAIL_COUNT="$(awk '$2=="000" || $2 ~ /^5[0-9][0-9]$/ {c++} END{print c+0}' "$f")"
  # Latency percentile is over requests that received an HTTP response only;
  # a connection failure (000) is not a latency sample, it is an availability hit.
  RESP_COUNT="$(awk '$2!="000"{c++} END{print c+0}' "$f")"
  if [[ "$RESP_COUNT" -eq 0 ]]; then
    err "every request failed to connect ($TOTAL/$TOTAL status=000); the core is not serving — refusing to emit evidence"
    exit 1
  fi
  OBS_P99="$(
    awk '$2!="000"{printf "%.4f\n", $1*1000}' "$f" \
      | sort -n \
      | awk -v n="$RESP_COUNT" 'BEGIN{k=int((99*n+99)/100); if(k<1)k=1; if(k>n)k=n} NR==k{print; exit}'
  )"
  OBS_ERR="$(awk -v fc="$FAIL_COUNT" -v t="$TOTAL" 'BEGIN{printf "%.4f", 100*fc/t}')"
  OBS_AVAIL="$(awk -v fc="$FAIL_COUNT" -v t="$TOTAL" 'BEGIN{printf "%.4f", 100-(100*fc/t)}')"
}

emit_json() {
  local out="$1"
  mkdir -p "$(dirname "$out")"
  jq -n \
    --arg wf "$WORKFLOW" \
    --arg slo "$SLO" \
    --arg win "$SAMPLE_WINDOW" \
    --arg src "$SOURCE" \
    --argjson tgt "$TARGET_JSON" \
    --argjson p99 "$OBS_P99" \
    --argjson err "$OBS_ERR" \
    --argjson avail "$OBS_AVAIL" \
    '{
      workflow: $wf,
      slo: $slo,
      sampleWindow: $win,
      source: $src,
      target: $tgt,
      observed: { latencyP99Ms: $p99, errorRatePct: $err, availabilityPct: $avail }
    }' >"$out"
  info "wrote evidence: ${out#"$REPO_ROOT"/}"
}

print_summary() {
  local tp99 terr tavail verdict
  tp99="$(printf '%s' "$TARGET_JSON" | jq -r '.latencyP99Ms // "n/a"')"
  terr="$(printf '%s' "$TARGET_JSON" | jq -r '.errorRatePct // "n/a"')"
  tavail="$(printf '%s' "$TARGET_JSON" | jq -r '.availabilityPct // "n/a"')"
  verdict="$(
    awk -v p="$OBS_P99" -v tp="$tp99" -v e="$OBS_ERR" -v te="$terr" \
        -v a="$OBS_AVAIL" -v ta="$tavail" 'BEGIN{
      ok=1
      if (tp!="n/a" && p+0 > tp+0) ok=0
      if (te!="n/a" && e+0 > te+0) ok=0
      if (ta!="n/a" && a+0 < ta+0) ok=0
      print (ok ? "WITHIN-TARGET" : "BREACH")
    }'
  )"
  echo "capture-slo: ---------------- SLO capture summary ----------------"
  echo "capture-slo:   workflow         : $WORKFLOW  (slo: $SLO)"
  echo "capture-slo:   source           : $SOURCE   window: $SAMPLE_WINDOW"
  echo "capture-slo:   requests total   : $TOTAL  (responded: $RESP_COUNT, failed: $FAIL_COUNT)"
  echo "capture-slo:   observed p99 ms  : $OBS_P99   target <= $tp99"
  echo "capture-slo:   observed err %   : $OBS_ERR   target <= $terr"
  echo "capture-slo:   observed avail % : $OBS_AVAIL   target >= $tavail"
  echo "capture-slo:   verdict          : $verdict"
  echo "capture-slo: ------------------------------------------------------"
}

cmd_run() {
  [[ -n "$WORKFLOW" ]] || { err "run requires --workflow"; usage >&2; exit 2; }
  [[ -n "$URL" ]] || { err "run requires --url"; usage >&2; exit 2; }
  [[ -n "$SLO" ]] || SLO="$WORKFLOW"
  [[ -n "$OUT" ]] || OUT="$REPO_ROOT/.specify/runtime/observability/${WORKFLOW}.slo.json"
  require_deps
  load_target

  # Preflight: the endpoint MUST return an HTTP response. A non-HTTP code (curl
  # emits "000" on connection failure) means the stack is down; we refuse to
  # emit a fabricated "healthy" evidence file. curl prints the code via -w even
  # when it exits non-zero, so we must NOT append a second sentinel on failure
  # (that would corrupt the value, e.g. "000000"); default only when empty.
  local pf
  pf="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$URL" 2>/dev/null || true)"
  pf="${pf:-000}"
  if [[ ! "$pf" =~ ^[1-5][0-9][0-9]$ ]]; then
    err "endpoint $URL is unreachable (HTTP $pf). Bring the stack up (./smackerel.sh up) before capturing. Refusing to emit fabricated evidence."
    exit 1
  fi
  info "preflight OK ($URL -> HTTP $pf); generating load: $REQUESTS requests @ concurrency $CONCURRENCY"

  local samples t0 t1
  samples="$(mktemp)"
  # shellcheck disable=SC2064
  trap "rm -f '$samples'" RETURN
  t0="$(date +%s)"
  seq "$REQUESTS" \
    | xargs -P "$CONCURRENCY" -I{} curl -s -o /dev/null -w '%{time_total} %{http_code}\n' --max-time 10 "$URL" \
    >>"$samples" 2>/dev/null || true
  t1="$(date +%s)"
  SAMPLE_WINDOW="PT$(( t1 - t0 ))S"

  compute_metrics "$samples"
  emit_json "$OUT"
  print_summary
}

cmd_compute() {
  [[ -n "$WORKFLOW" ]] || { err "compute requires --workflow"; usage >&2; exit 2; }
  [[ -n "$SAMPLES_FILE" ]] || { err "compute requires --samples <file>"; usage >&2; exit 2; }
  [[ -f "$SAMPLES_FILE" ]] || { err "samples file not found: $SAMPLES_FILE"; exit 1; }
  [[ -n "$SLO" ]] || SLO="$WORKFLOW"
  [[ -n "$OUT" ]] || OUT="$REPO_ROOT/.specify/runtime/observability/${WORKFLOW}.slo.json"
  [[ -n "$SAMPLE_WINDOW" ]] || SAMPLE_WINDOW="PT0S"
  require_deps
  load_target
  compute_metrics "$SAMPLES_FILE"
  emit_json "$OUT"
  print_summary
}

main() {
  local sub="${1:-}"
  if [[ $# -gt 0 ]]; then shift; fi
  case "$sub" in
    -h|--help|"") usage; [[ "$sub" == "" ]] && exit 2 || exit 0 ;;
    run|compute) : ;;
    *) err "unknown subcommand: $sub"; usage >&2; exit 2 ;;
  esac

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --workflow) WORKFLOW="${2:?--workflow requires a value}"; shift 2 ;;
      --slo) SLO="${2:?--slo requires a value}"; shift 2 ;;
      --url) URL="${2:?--url requires a value}"; shift 2 ;;
      --requests) REQUESTS="${2:?--requests requires a value}"; shift 2 ;;
      --concurrency) CONCURRENCY="${2:?--concurrency requires a value}"; shift 2 ;;
      --source) SOURCE="${2:?--source requires a value}"; shift 2 ;;
      --config) CONFIG="${2:?--config requires a value}"; shift 2 ;;
      --out) OUT="${2:?--out requires a value}"; shift 2 ;;
      --samples) SAMPLES_FILE="${2:?--samples requires a value}"; shift 2 ;;
      --sample-window) SAMPLE_WINDOW="${2:?--sample-window requires a value}"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      *) err "unknown flag: $1"; usage >&2; exit 2 ;;
    esac
  done

  case "$sub" in
    run) cmd_run ;;
    compute) cmd_compute ;;
  esac
}

main "$@"
