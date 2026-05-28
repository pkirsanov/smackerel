#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #5 — Telegram-adapter smoke covering 4
# scenario types (capture / retrieval / weather / notification) through
# the live Telegram webhook on the disposable test stack.
#
# Shape: for each scenario the smoke fires a synthetic Telegram update
# with a unique correlation_id, scrapes the assistant_turn slog line
# via the §18.5 pattern, and asserts the scenario_id matches the
# expected dispatch target. The smoke is intentionally orchestration-
# only: per-scenario terminal-state assertions live in the SCOPE-10
# DoD #7 per-BS regression fixtures under
# tests/e2e/assistant_regression/. The smoke proves the Telegram
# adapter dispatches; the regression fixtures prove the terminal-state
# semantics of each scenario.
#
# Pre-conditions (any missing => skip-77 cleanly):
#   - test env wires the Telegram webhook secret + path
#   - graph + provider stubs are seeded for retrieval / weather /
#     notification probes (tracked under SCOPE-04 / SCOPE-06 / SCOPE-07
#     substrate findings; same blockers as the per-BS regression
#     fixtures for BS-002 / BS-004)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/assistant_regression/lib/regression_helpers.sh"

# Currently blocked on the same substrate gaps as BS-002 and BS-004 —
# the 4-scenario smoke needs the same seeding that those fixtures need.
reg_skip_with_blocker "TELEGRAM-SMOKE" \
  "SCOPE-04-SCOPE-06-SUBSTRATE-NOT-YET-AUTHORED-FOR-4-SCENARIO-SMOKE"

:<<'EXECUTED_PATTERN'
trap e2e_cleanup EXIT
echo "=== Spec 061 SCOPE-10 DoD #5 — Telegram-adapter 4-scenario smoke ==="
e2e_start
WEBHOOK_SECRET="$(reg_required_env_value TELEGRAM-SMOKE ASSISTANT_TELEGRAM_WEBHOOK_SECRET)"
WEBHOOK_PATH="$(reg_required_env_value TELEGRAM-SMOKE ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH)"

run_scenario() {
  local label="$1" expected_scenario_id="$2" chat_id="$3" text="$4"
  local nonce="$(date +%s%N)$$$RANDOM"
  local update_id="$nonce"
  local since_ts="$(date --utc +%Y-%m-%dT%H:%M:%S)"
  local payload http_status turn_line scenario_id
  payload=$(cat <<JSON
{"update_id": $update_id, "message": {"message_id": $update_id,
 "date": $(date +%s), "chat": {"id": $chat_id, "type": "private"},
 "from": {"id": $chat_id, "is_bot": false, "first_name": "$label"},
 "text": "$text"}}
JSON
  )
  http_status=$(curl -s -o "/tmp/telegram_smoke_${label}.txt" -w "%{http_code}" \
    --max-time 10 -X POST -H "Content-Type: application/json" \
    -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
    -d "$payload" "$CORE_URL$WEBHOOK_PATH" || true)
  [ "$http_status" = "200" ] || e2e_fail "TELEGRAM-SMOKE/$label: webhook returned $http_status"
  turn_line=""
  for _ in $(seq 1 60); do
    turn_line="$(docker logs --since "$since_ts" smackerel-test-smackerel-core-1 2>&1 \
                | grep -F '"msg":"assistant_turn"' \
                | grep -F "\"correlation_id\":\"$update_id\"" | tail -1 || true)"
    [ -n "$turn_line" ] && break
    sleep 1
  done
  [ -n "$turn_line" ] || e2e_fail "TELEGRAM-SMOKE/$label: no assistant_turn slog line within 60s"
  scenario_id=$(echo "$turn_line" | jq -r '.scenario_id // ""')
  [ "$scenario_id" = "$expected_scenario_id" ] || \
    e2e_fail "TELEGRAM-SMOKE/$label: scenario_id='$scenario_id' (want '$expected_scenario_id')"
  echo "  $label: OK (scenario_id=$scenario_id)"
}

run_scenario capture       capture                        99101 "remember to buy milk tomorrow"
run_scenario retrieval     retrieval_qa                   99102 "what did I save about Tailscale last month?"
run_scenario weather       weather_query                  99103 "weather in Reykjavik tomorrow"
run_scenario notification  notification_decision_propose  99104 "show me pending notifications"

e2e_pass "TELEGRAM-SMOKE: 4-scenario Telegram-adapter smoke passed"
EXECUTED_PATTERN
