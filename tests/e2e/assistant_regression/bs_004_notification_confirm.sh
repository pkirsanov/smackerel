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
