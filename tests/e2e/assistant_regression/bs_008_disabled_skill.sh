#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #7 — BS-008 persistent regression slot.
#
# BS-008 (spec.md §3): a skill marked disabled in the scenario manifest
# is never invoked even when a probe maps to it; instead the assistant
# returns a graceful "skill_disabled" response and routes through the
# capture fallback. Expected §18.5 assertion shape:
#
#   scenario_id == "<disabled-skill-id>"
#   status      == "skill_disabled"
#   error_cause == "" (deliberate gate, not an error)
#   capture_fallback_total{cause="skill_disabled"} delta == 1
#
# Pre-conditions (any missing => skip-77):
#   - test env can flip a known skill to manifest.enabled=false at
#     runtime (today the manifest is loaded at boot and the disposable
#     test stack does not support per-test manifest mutation; tracked
#     as SCOPE-04-MANIFEST-HOT-FLIP-NOT-YET-AUTHORED)
#   - OR the test env ships a built-in always-disabled probe skill

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/regression_helpers.sh"

reg_skip_with_blocker "BS-008" "SCOPE-04-MANIFEST-HOT-FLIP-NOT-YET-AUTHORED"

:<<'EXECUTED_PATTERN'
# When manifest hot-flip lands:
#   1. e2e_start; mark a known skill as disabled in the active manifest
#   2. POST a Telegram update whose probe maps to that skill
#   3. §18.5 scrape:
#        scenario_id matches the disabled skill id
#        status      == skill_disabled
#        error_cause == ""
#   4. Adversarial: assert the skill's invocation counter delta == 0
#      (proves the gate prevented the call rather than the skill
#      handling the request and short-circuiting).
EXECUTED_PATTERN
