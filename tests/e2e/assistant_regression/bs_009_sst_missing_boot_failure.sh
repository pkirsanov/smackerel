#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #7 — BS-009 persistent regression slot.
#
# BS-009 (spec.md §3): assistant subsystem fails LOUD at boot when a
# required SST config key is missing — no silent default, no fallback,
# no degraded operation. Expected boot-failure assertion shape:
#
#   exit code != 0
#   stderr contains the missing key's documented error text
#   no assistant_turn slog line is ever emitted
#
# Pre-conditions (any missing => skip-77):
#   - the test harness can boot smackerel-core against a deliberately
#     incomplete env file without contaminating the running test stack
#     (today the disposable stack always boots against a fully-resolved
#     env file generated from config/smackerel.yaml; tracked as
#     SCOPE-10-BOOT-FAILURE-HARNESS-NOT-YET-AUTHORED)
#
# Note: the in-process unit / contract tests under
# internal/config/ and internal/assistant/ already cover the SST
# zero-defaults invariant. This persistent slot is the E2E-level
# proof that the same invariant survives all the way through the
# real boot path.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/regression_helpers.sh"

reg_skip_with_blocker "BS-009" "SCOPE-10-BOOT-FAILURE-HARNESS-NOT-YET-AUTHORED"

:<<'EXECUTED_PATTERN'
# When the boot-failure harness lands:
#   1. Copy the resolved test env file to /tmp/bs009_broken.env
#   2. Strip ASSISTANT_TELEGRAM_WEBHOOK_SECRET (a known-required key)
#   3. Boot smackerel-core via docker run --env-file /tmp/bs009_broken.env
#      with a 10s timeout
#   4. Assert exit code != 0 and stderr contains the documented
#      "ASSISTANT_TELEGRAM_WEBHOOK_SECRET ... must be set" error string
#   5. Adversarial: assert no assistant_turn slog line was emitted
#      during the failed boot (proves the failure is at config-load
#      time, not after a turn was already accepted).
EXECUTED_PATTERN
