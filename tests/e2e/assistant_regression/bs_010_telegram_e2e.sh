#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #7 — BS-010 persistent regression slot.
#
# BS-010 (spec.md §3): end-to-end Telegram-adapter coverage of the
# assistant turn — the same canonical flow that the broader Telegram
# smoke (tests/e2e/assistant_acceptance_telegram_smoke.sh) exercises
# across 4 scenario types. The BS-010 regression slot is the named
# per-scenario entry that the BS table maps to; it delegates to the
# capture-fallback fixture (BS-001) as the always-available Telegram
# adapter probe so the slot is honored on every CI run.
#
# Adversarial guards (inherited from the delegate):
#   - assert the Telegram webhook returns 200
#   - assert the §18.5 assistant_turn slog line carries the
#     correlation_id propagated from telegram_update_id (§18.6)
#   - assert body_redacted == true (§18.5 Principle 8 affirmation)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/regression_helpers.sh"
reg_delegate "BS-010" "$REG_E2E_DIR/test_telegram_assistant_bs001.sh"
