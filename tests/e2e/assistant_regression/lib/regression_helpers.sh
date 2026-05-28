#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #7 — shared helpers for the per-BS regression
# fixtures. The fixtures themselves are intentionally thin: each one
# either delegates to an authoritative SCOPE-05/07-owned fixture under
# tests/e2e/, runs a §18.5 slog-scrape assertion directly, OR exits 77
# (skip) with SKIP_REASON when a substrate pre-condition is absent.
#
# The skip-77 convention is required because the per-BS regression
# fixtures are persistent — they MUST be present in tree for every BS
# scenario in spec 061 — but several scenarios depend on substrate
# (provenance-violation stub, disabled-skill test env override,
# graph-seeding fixtures) that has not yet been authored as of round 19.
# Skipping with a documented blocker is honest: the fixture exists, the
# CI runner records the skip + the blocker id, and the round that
# unblocks the substrate flips the skip to an executed assertion
# without changing the fixture's external contract.

set -euo pipefail

REG_HELPERS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# REG_HELPERS_DIR = <repo>/tests/e2e/assistant_regression/lib
# REG_E2E_DIR     = <repo>/tests/e2e             (../..)
# REG_REPO_DIR    = <repo>                       (../../../..)
REG_E2E_DIR="$(cd "$REG_HELPERS_DIR/../.." && pwd)"
REG_REPO_DIR="$(cd "$REG_HELPERS_DIR/../../../.." && pwd)"

# Source the canonical e2e helpers so any fixture that opts into the
# live-stack path inherits e2e_start / e2e_wait_healthy / e2e_pass /
# e2e_fail without duplication.
source "$REG_E2E_DIR/lib/helpers.sh"

# reg_skip_with_blocker prints a structured skip record to stdout
# (consumable by the CI runner) and exits 77 — the Bubbles / shell
# convention for "skipped, not failed". A SKIP_REASON MUST be supplied
# and SHOULD reference a tracked finding id under spec 061.
reg_skip_with_blocker() {
  local bs="${1:?BS id required}"
  local reason="${2:?SKIP_REASON required}"
  echo "=== Spec 061 SCOPE-10 DoD #7 — $bs persistent regression fixture ==="
  echo "RESULT: SKIPPED"
  echo "SKIP_REASON: $reason"
  echo "FIXTURE_PATH: ${BASH_SOURCE[1]:-unknown}"
  echo "NOTE: this file is a persistent regression slot for $bs; the slot"
  echo "      is honored on every CI run so the skip itself is visible. The"
  echo "      round that closes \"$reason\" replaces this skip-77 with the"
  echo "      executed §18.5 assertion shape declared in the fixture body."
  exit 77
}

# reg_delegate runs an existing authoritative fixture under tests/e2e/
# in this same shell so any failure propagates as the fixture's exit
# code. Used by per-BS regression fixtures whose canonical assertion
# already lives under a SCOPE-05/07-owned file; the regression fixture
# is the single persistent slot that SCOPE-10 DoD #7 demands.
reg_delegate() {
  local bs="${1:?BS id required}"
  local target="${2:?target script path required}"
  if [ ! -f "$target" ]; then
    reg_skip_with_blocker "$bs" "missing-delegate-target:$target"
  fi
  echo "=== Spec 061 SCOPE-10 DoD #7 — $bs persistent regression fixture ==="
  echo "DELEGATE: $target"
  echo "NOTE: the per-BS regression slot delegates to the authoritative"
  echo "      SCOPE-05/07-owned fixture; both fixtures MUST stay in sync"
  echo "      with the §18.5 assertion shape from design.md."
  exec bash "$target"
}

# reg_required_env_value reads a key from the test env file and skips
# the fixture (exit 77) when the key is absent or empty. Used by
# fixtures whose §18.5 assertion needs a stub-providers URL or test
# secret that may not be wired in every environment.
reg_required_env_value() {
  local bs="${1:?BS id required}"
  local key="${2:?env key required}"
  local env_file
  env_file="$(smackerel_require_env_file "$TEST_ENV")"
  local val
  val="$(smackerel_env_value "$env_file" "$key" 2>/dev/null || true)"
  if [ -z "$val" ]; then
    reg_skip_with_blocker "$bs" "missing-env-key:$key:in:$env_file"
  fi
  printf '%s' "$val"
}
