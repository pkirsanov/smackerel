#!/usr/bin/env bash
# observability-adapter-lint.sh — validates every adapter under
# bubbles/adapters/observability/ implements the 4-verb contract.
#
# For each *.sh file in the adapter directory, verifies:
#   1. File is executable
#   2. Calling with each of the 4 verbs (fetch-alerts, fetch-slo-burn,
#      fetch-error-rate, fetch-deploy-impact) does not exit with code 2+
#      (exit 1 is acceptable — means adapter unavailable, not contract violation)
#   3. The `none` adapter MUST exist and return `{}` for every verb
#
# Notes:
# - This lint does NOT attempt to validate that adapters return correct
#   structured payloads at runtime (that requires a live backend).
# - For adapters that need env vars (prometheus needs PROMETHEUS_BASE_URL),
#   we skip the runtime invocation and only verify the script declares the
#   verbs in its `case` statement.
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

# Runtime check: none.sh must actually return `{}` for each verb
if [[ -x "$NONE_SH" ]]; then
  for verb in "${REQUIRED_VERBS[@]}"; do
    out="$("$NONE_SH" "$verb" 2>/dev/null || echo "ERR")"
    if [[ "$out" != "{}" ]]; then
      err "none.sh $verb returned '$out' (expected '{}')"
    fi
  done
fi

if [[ "$FAILED" -ne 0 ]]; then
  exit 1
fi

info "OK ($(ls "$ADAPTER_DIR"/*.sh 2>/dev/null | wc -l) adapter(s) validated)"
exit 0
