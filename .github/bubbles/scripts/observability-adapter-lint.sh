#!/usr/bin/env bash
# observability-adapter-lint.sh — validates every adapter under
# bubbles/adapters/observability/ implements the 4-verb contract.
#
# For each *.sh file in the adapter directory, verifies:
#   1. File is executable
#   2. Calling with each of the 4 verbs (fetch-alerts, fetch-slo-burn,
#      fetch-error-rate, fetch-deploy-impact) is declared in its case statement
#   3. The `none` adapter MUST exist and return the canonical NEUTRAL value per
#      verb (`[]` for fetch-alerts, `{}` for the other three)
#   4. PER-VERB normalized SHAPE (R2-D): fetch-alerts MUST be a JSON array, the
#      other three MUST be JSON maps/objects. Validated against:
#        - the `none` adapter's neutral output, and
#        - any adapter exposing a `selftest <verb>` fixture mode (e.g.
#          `prometheus`), whose normalized output is shape-checked WITHOUT a
#          live backend.
#
# Notes:
# - Shape validation requires `jq`. When jq is absent the lint WARN-and-skips
#   the SHAPE checks (the grep-based verb-presence checks still run) — a missing
#   developer tool must not silently pass nor hard-block the contract lint.
# - Adapters that need env vars for LIVE calls (prometheus needs
#   PROMETHEUS_BASE_URL) are never invoked live during lint; their shape is
#   validated only through the env-free `selftest` fixture mode when present.
#
# Exit 0 = clean. Exit 1 = contract violation.

set -euo pipefail

REPO_ROOT="${1:-.}"
ADAPTER_DIR="$REPO_ROOT/bubbles/adapters/observability"

FAILED=0
err() { echo "[observability-adapter-lint][ERROR] $*" >&2; FAILED=1; }
info() { echo "[observability-adapter-lint] $*"; }

REQUIRED_VERBS=( fetch-alerts fetch-slo-burn fetch-error-rate fetch-deploy-impact )

if [[ ! -d "$ADAPTER_DIR" ]]; then
  err "$ADAPTER_DIR not found"
  exit 1
fi

# `none` MUST exist
NONE_SH="$ADAPTER_DIR/none.sh"
if [[ ! -x "$NONE_SH" ]]; then
  err "$NONE_SH missing or not executable (required default adapter)"
fi

shopt -s nullglob
for adapter in "$ADAPTER_DIR"/*.sh; do
  name="$(basename "$adapter" .sh)"
  if [[ ! -x "$adapter" ]]; then
    err "$adapter not executable"
    continue
  fi
  # Verify every verb is declared in the script (grep is enough — we don't run
  # adapters that may need env vars during lint).
  for verb in "${REQUIRED_VERBS[@]}"; do
    # Verb can appear standalone (verb)) or inside a |-list (a|verb|b)). Match
    # either case by looking for the verb followed by `)` or `|`.
    if ! grep -qE "(^|[[:space:]\|])${verb}[)\|]" "$adapter"; then
      err "$name: verb '$verb' not handled (missing from case statement)"
    fi
  done
done

# Runtime PER-VERB SHAPE validation (R2-D): fetch-alerts is a JSON array; the
# other three verbs are JSON maps/objects. Requires jq; WARN-and-skip if absent.
HAVE_JQ=0
if command -v jq >/dev/null 2>&1; then HAVE_JQ=1; fi

# assert_verb_shape <label> <verb> <json>
assert_verb_shape() {
  local label="$1" verb="$2" json="$3"
  case "$verb" in
    fetch-alerts)
      if ! printf '%s' "$json" | jq -e 'type == "array"' >/dev/null 2>&1; then
        err "$label: '$verb' must emit a JSON array (R2-D), got: $json"
      fi
      ;;
    *)
      if ! printf '%s' "$json" | jq -e 'type == "object"' >/dev/null 2>&1; then
        err "$label: '$verb' must emit a JSON map/object (R2-D), got: $json"
      fi
      ;;
  esac
}

if [[ "$HAVE_JQ" -eq 1 ]]; then
  # (a) none.sh: its NORMAL verb output IS the canonical neutral shape.
  if [[ -x "$NONE_SH" ]]; then
    for verb in "${REQUIRED_VERBS[@]}"; do
      out="$("$NONE_SH" "$verb" 2>/dev/null || echo 'ERR')"
      assert_verb_shape "none.sh" "$verb" "$out"
    done
  fi
  # (b) Any adapter exposing a `selftest <verb>` fixture mode: validate the
  #     NORMALIZED shape it emits without a live backend (e.g. prometheus drives
  #     a canned raw envelope through its real normalizer). Adapters without a
  #     selftest mode (e.g. none) fail the probe and are skipped here.
  for adapter in "$ADAPTER_DIR"/*.sh; do
    [[ -x "$adapter" ]] || continue
    name="$(basename "$adapter" .sh)"
    if "$adapter" selftest fetch-alerts >/dev/null 2>&1; then
      for verb in "${REQUIRED_VERBS[@]}"; do
        sout="$("$adapter" selftest "$verb" 2>/dev/null || echo 'ERR')"
        assert_verb_shape "$name (selftest)" "$verb" "$sout"
      done
    fi
  done
else
  info "jq not found — skipping per-verb payload SHAPE validation (grep-based verb presence still enforced). Install jq to enable shape checks."
fi

if [[ "$FAILED" -ne 0 ]]; then
  exit 1
fi

info "OK ($(ls "$ADAPTER_DIR"/*.sh 2>/dev/null | wc -l) adapter(s) validated)"
exit 0
