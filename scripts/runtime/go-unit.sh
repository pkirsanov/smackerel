#!/usr/bin/env bash
# spec-047 R7: trace mode (`x`) added to surface the exact failing
# command in CI logs even when only step-level metadata is available
# externally. No secrets pass through this script — only apt + go test
# invocations — so command tracing is safe to leave on in CI.
set -euxo pipefail

# spec-047 R2R-CI / spec-052 chaos finding: TestSSTLoader_RejectsDevPostgresPassword_HomeLab
# in internal/config (and any other test that shells out to
# scripts/commands/config.sh) calls envsubst (provided by gettext-base
# on Debian/Ubuntu). The golang:bookworm test image does not include
# gettext-base by default. The shared helper installs it idempotently
# so go-unit, go-integration, go-e2e, and go-stress all share one
# implementation.
# shellcheck source=scripts/runtime/_ensure_envsubst.sh
source "$(dirname "${BASH_SOURCE[0]}")/_ensure_envsubst.sh"
ensure_envsubst "go-unit"

cd /workspace

# Spec 045 / BUG-045-001 — optional focused-run flags consistent with
# scripts/runtime/go-e2e.sh and scripts/runtime/go-stress.sh.
# Allows operators and report evidence to use the repo-standard CLI
# (./smackerel.sh test unit --go --go-run '<regex>' --verbose) without
# bypassing into raw `go test`.
go_run_selector=""
go_verbose=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --run)
      if [[ $# -lt 2 ]] || [[ -z "$2" ]]; then
        echo "ERROR: --run requires a non-empty regex" >&2
        exit 1
      fi
      go_run_selector="$2"
      shift 2
      ;;
    --run=*)
      go_run_selector="${1#*=}"
      if [[ -z "$go_run_selector" ]]; then
        echo "ERROR: --run requires a non-empty regex" >&2
        exit 1
      fi
      shift
      ;;
    --verbose|-v)
      go_verbose="-v"
      shift
      ;;
    *)
      echo "Unknown go-unit option: $1" >&2
      exit 1
      ;;
  esac
done

go_test_args=()
if [[ -n "$go_verbose" ]]; then
  go_test_args+=("$go_verbose")
fi
if [[ -n "$go_run_selector" ]]; then
  echo "[go-unit] applying -run selector: $go_run_selector"
  go_test_args+=(-run "$go_run_selector" -count=1)
fi

echo "[go-unit] starting go test ./..."
go test "${go_test_args[@]}" ./...
echo "[go-unit] go test ./... finished OK"