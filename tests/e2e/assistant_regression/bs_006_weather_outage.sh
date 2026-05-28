#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #7 — BS-006 persistent regression slot.
#
# BS-006 (spec.md §3): weather skill 5xx outage degrades gracefully.
# The authoritative §18.5 fixture lives at tests/e2e/assistant_bs006_test.sh
# (SCOPE-07 owned; §18.4 stub-providers container in the failure mode).
# This file is the SCOPE-10 DoD #7 persistent slot that delegates.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/regression_helpers.sh"
reg_delegate "BS-006" "$REG_E2E_DIR/assistant_bs006_test.sh"
