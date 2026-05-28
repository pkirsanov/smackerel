#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #7 — BS-003 persistent regression slot.
#
# BS-003 (spec.md §3): weather query returns provider-attributed
# answer. The authoritative §18.5 fixture lives at
# tests/e2e/assistant_bs003_test.sh (SCOPE-07 owned, §18.3 SST keys +
# §18.4 stub-providers container). This file is the SCOPE-10 DoD #7
# persistent slot that delegates.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/regression_helpers.sh"
reg_delegate "BS-003" "$REG_E2E_DIR/assistant_bs003_test.sh"
