#!/usr/bin/env bash
# Forecast-Eval Check (IMP-100 Phase 6 / IMP-020 S6 — FIN-001)
# ---------------------------------------------------------------------------
# A GENERIC, product-agnostic forecast evaluator: the reusable temporal /
# leakage / scoring core that any product's forecast eval composes (as an
# executable-oracle in an eval-harness task, or standalone). It carries NO
# product data or model — only the generic mechanics:
#
#   - TEMPORAL INTEGRITY / LEAKAGE — every prediction MUST be made strictly
#     BEFORE its outcome is known (`predictedAt` < `resolvedAt`). A prediction
#     that "knows the future" (predictedAt >= resolvedAt) is leakage and
#     invalidates the forecast.
#   - PROPER SCORING — a Brier score (mean squared error of a probabilistic
#     prediction vs the binary outcome); lower is better, range [0, 1].
#   - SHAPE — `predicted` is a probability in [0, 1]; `actual` is 0 or 1.
#
# Input (JSON):
#   { "predictions": [ { "id": "p1", "predictedAt": "<iso>", "resolvedAt": "<iso>",
#                        "predicted": 0.7, "actual": 1 }, ... ] }
#
# Output (JSON on stdout):
#   { count, leakageIds, leakage, brierScore, valid }
#
# Usage:
#   bash bubbles/scripts/forecast-eval-check.sh --forecast <json> [--strict]
#
# Exit codes:
#   0  report produced (default) — or, under --strict, no leakage and shape valid
#   1  under --strict: leakage present or invalid shape
#   2  usage / runtime error (missing file, empty/invalid predictions)
set -euo pipefail

FORECAST=""
STRICT="false"

usage() {
  cat <<'EOF'
Usage: forecast-eval-check.sh --forecast <json> [--strict]

Generic forecast evaluation: temporal-integrity/leakage detection (predictedAt <
resolvedAt) + Brier scoring over probabilistic predictions vs binary outcomes.
--strict exits 1 on leakage or invalid shape.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --forecast)
      [[ $# -ge 2 ]] || { echo "forecast-eval-check: --forecast requires a value" >&2; exit 2; }
      FORECAST="$2"
      shift 2
      ;;
    --strict)
      STRICT="true"
      shift
      ;;
    *)
      echo "forecast-eval-check: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$FORECAST" ]]; then
  echo "forecast-eval-check: missing required --forecast" >&2
  usage >&2
  exit 2
fi
if [[ ! -f "$FORECAST" ]]; then
  echo "forecast-eval-check: forecast file not found: $FORECAST" >&2
  exit 2
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "forecast-eval-check: jq is required but not found in PATH" >&2
  exit 2
fi
if ! jq -e 'type == "object"' "$FORECAST" >/dev/null 2>&1; then
  echo "forecast-eval-check: malformed or non-object JSON: $FORECAST" >&2
  exit 2
fi

# Predictions must be a non-empty array of well-shaped objects.
if ! jq -e '(.predictions | type) == "array" and (.predictions | length) > 0' "$FORECAST" >/dev/null 2>&1; then
  echo "forecast-eval-check: .predictions must be a non-empty array" >&2
  exit 2
fi

# Shape validation: predicted in [0,1], actual in {0,1}, timestamps present.
shape_ok="$(jq -r '
  [ .predictions[]
    | (has("predictedAt") and has("resolvedAt")
       and (.predicted | type) == "number" and .predicted >= 0 and .predicted <= 1
       and (.actual | type) == "number" and (.actual == 0 or .actual == 1)) ]
  | all
' "$FORECAST" 2>/dev/null || echo "false")"
if [[ "$shape_ok" != "true" ]]; then
  echo "forecast-eval-check: invalid prediction shape (predicted must be a probability in [0,1], actual must be 0 or 1, timestamps required)" >&2
  if [[ "$STRICT" == "true" ]]; then exit 1; fi
  # Non-strict: still report what we can (invalid shape → valid:false, no score).
  jq -n --argjson count "$(jq '.predictions | length' "$FORECAST")" \
    '{count: $count, leakageIds: [], leakage: 0, brierScore: null, valid: false}'
  exit 0
fi

# Temporal integrity / leakage: predictedAt must be strictly before resolvedAt.
report="$(jq '
  (.predictions) as $p
  | ([ $p[] | select(.predictedAt >= .resolvedAt) | .id ]) as $leak
  | {
      count: ($p | length),
      leakageIds: $leak,
      leakage: ($leak | length),
      brierScore: ( ( [ $p[] | (.predicted - .actual) | . * . ] | add ) / ($p | length) ),
      valid: (($leak | length) == 0)
    }
' "$FORECAST")"

printf '%s\n' "$report"

if [[ "$STRICT" == "true" ]]; then
  leak_count="$(printf '%s' "$report" | jq -r '.leakage')"
  [[ "$leak_count" -eq 0 ]] || exit 1
fi
exit 0
