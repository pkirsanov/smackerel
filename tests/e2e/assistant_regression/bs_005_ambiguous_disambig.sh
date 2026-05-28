#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #7 — BS-005 persistent regression slot.
#
# BS-005 (spec.md §3): borderline-confidence ambiguous input surfaces a
# disambiguation prompt with the always-last "save as note" choice, and
# the user-resolved or timeout paths are honored.
#
# The §18.5 assertion shape (full happy path is exercised by the
# in-process facade tests under
# internal/assistant/facade_disambig_resolver_test.go — they cover the
# 7 emission branches including resolved_user / resolved_timeout_capture
# / resolved_non_matching_reply_capture). The end-to-end slot here
# verifies that the same surface flows out through the Telegram adapter
# (not just the facade): one Telegram update lands the disambig prompt,
# a follow-up Telegram update with "1" / "save as note" / non-numeric
# text triggers the matching outcome on the second turn.
#
# Pre-conditions (any missing => skip-77):
#   - test env exposes at least 2 manifest-enabled candidate scenarios
#     with band-borderline classification configured
#   - router is wired to emit TopScore in the AgentConfidenceFloor
#     (0.50) <= TopScore < BorderlineFloor (0.75) band for the probe
#   - currently NOT seeded in the disposable test stack — tracked as
#     SCOPE-07-BORDERLINE-SEEDING-NOT-YET-AUTHORED

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/regression_helpers.sh"

reg_skip_with_blocker "BS-005" "SCOPE-07-BORDERLINE-SEEDING-NOT-YET-AUTHORED"

:<<'EXECUTED_PATTERN'
trap e2e_cleanup EXIT
e2e_start
WEBHOOK_SECRET="$(reg_required_env_value BS-005 ASSISTANT_TELEGRAM_WEBHOOK_SECRET)"
WEBHOOK_PATH="$(reg_required_env_value BS-005 ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH)"
# Turn 1: probe that lands borderline band.
UPDATE_ID_1="$(date +%s%N)$$1"
SINCE_TS_1="$(date --utc +%Y-%m-%dT%H:%M:%S)"
# Turn 2: numeric "1" reply (matches first disambig choice).
UPDATE_ID_2="$(date +%s%N)$$2"
# §18.5 scrapes assert turn 1 has kind=disambiguation, turn 2 has the
# matching disambig_outcomes_total emission via the assistant_turn
# slog line (or the dedicated disambig_resolved slog event).
EXECUTED_PATTERN
