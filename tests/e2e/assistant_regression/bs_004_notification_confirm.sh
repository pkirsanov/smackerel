#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #7 — BS-004 persistent regression slot.
#
# BS-004 (spec.md §3): notification confirm-card surfaces and the
# user-confirmation path commits the assistant-proposed action.
# Expected §18.5 assertion shape:
#
#   first turn:  scenario_id == "notification_decision_propose"
#                kind        == "confirm"
#                status      == "awaiting_user_confirmation"
#   second turn (after the user types "yes"):
#                scenario_id == "notification_decision_commit"
#                status      == "committed"
#                error_cause == ""
#
# Pre-conditions (any missing => skip-77):
#   - test env wires the notification surface and the proposal fixture
#     (SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED)
#
# ---------------------------------------------------------------------------
# CORRECTION 2026-08-28 — this fixture is STALE, not blocked on missing work.
#
# The expected shape above names scenario/status tokens that the implementation
# never adopted. BS-004 behaviour IS shipped (spec 061 SCOPE-08, "confirm-card
# state machine activated end-to-end"): contracts.KindConfirm and ConfirmCard
# both exist, as does smackerel_assistant_confirm_card_outcomes_total.
#
# Design-era token (asserted above)     ->  Shipped token (verify before use)
#   notification_decision_propose       ->  scenario "notification_schedule"
#                                           (internal/assistant/shortcuts.go:49, "/remind")
#   status "awaiting_user_confirmation" ->  status "reminder_proposed"
#   status "committed"                  ->  status "reminder_confirmed"
#   (alt-flow, spec 061 spec.md:1642)   ->  status "reminder_cancelled"
# Shipped tokens: internal/assistant/contracts/response.go:164-166.
#
# So the unblock is a REWRITE against the shipped vocabulary, not a wait for a
# feature. Until that happens this slot stays skip-77 and the lane exits non-zero.
# ---------------------------------------------------------------------------
#   - test stack seeds at least one outstanding notification proposal
#     against the BS-004 chat id
#
# Adversarial guards (when executed):
#   - assert the first turn's kind is "confirm" (not "text") to catch
#     a regression that loses the confirm-card surface
#   - assert the commit turn does NOT carry error_cause="external_provider"
#     to catch a regression that 5xxs against the notification backend

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/regression_helpers.sh"

reg_skip_with_blocker "BS-004" "SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED"

:<<'EXECUTED_PATTERN'
# When the proposal fixture lands:
#   1. e2e_start; seed a notification proposal for $CHAT_ID
#   2. POST a synthetic Telegram update that triggers the proposal
#   3. §18.5 scrape: assert kind=confirm, status=awaiting_user_confirmation
#   4. POST a second Telegram update with text "yes"
#   5. §18.5 scrape on the second correlation_id: assert
#      scenario_id=notification_decision_commit, status=committed
EXECUTED_PATTERN
