#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #7 — BS-001 persistent regression slot.
#
# BS-001 (spec.md §3): plain note is captured (regression guard, any
# frontend). The authoritative §18.5 fixture for the Telegram-adapter
# path lives at tests/e2e/test_telegram_assistant_bs001.sh and is
# owned by SCOPE-05. This file is the SCOPE-10 DoD #7 persistent slot
# that the CI runner walks for the BS-001 regression slot; it delegates
# in-process so any regression of the §18.5 capture-fallback assertion
# fails this fixture identically.
#
# The slot is intentionally a delegate (not a copy): duplicating the
# §18.5 payload + slog assertion would create two sources of truth for
# the same scenario. The reg_delegate helper exec's the target so its
# exit code is the only signal the runner sees.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/regression_helpers.sh"
reg_delegate "BS-001" "$REG_E2E_DIR/test_telegram_assistant_bs001.sh"
