#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #7 — BS-007 persistent regression slot.
#
# BS-007 (spec.md §3): retrieval synthesis without sources is blocked
# by the provenance gate and surfaces a provenance-violation outcome
# (not a successful retrieval answer). The §18.5 assertion shape is:
#
#   scenario_id        == "retrieval_qa"
#   status             == "provenance_blocked"
#   error_cause        == "provenance_violation"
#   provenance_violations_total counter incremented by 1
#     (closed in round 19 SCOPE-09 work — emission site is live)
#
# Pre-conditions (any missing => skip-77):
#   - test env wires an LLM stub that returns synthesis without
#     attaching sources (currently NOT in the stub-providers container;
#     tracked as SCOPE-04-LLM-NOSOURCE-STUB-NOT-YET-AUTHORED)
#   - retrieval skill is enabled in the test env

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/regression_helpers.sh"

reg_skip_with_blocker "BS-007" "SCOPE-04-LLM-NOSOURCE-STUB-NOT-YET-AUTHORED"

:<<'EXECUTED_PATTERN'
# When the no-source LLM stub lands:
#   1. e2e_start; ensure the no-source response mode is selected for
#      the BS-007 probe (e.g., header or query param the stub honors)
#   2. POST a synthetic Telegram update with a retrieval question
#   3. §18.5 scrape:
#        scenario_id == retrieval_qa
#        status      == provenance_blocked
#        error_cause == provenance_violation
#   4. Adversarial: assert provenance_violations_total counter delta
#      is exactly 1 (use the /metrics endpoint before/after the turn).
EXECUTED_PATTERN
