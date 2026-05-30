#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #5 — Weather acceptance smoke (2 of 4).
#
# Minimal happy-path smoke for the weather skill: drives /weather
# through the live Telegram webhook on the disposable test stack and
# asserts a single §18.5 invariant — the assistant_turn slog line
# carries scenario_id="weather_query". Exhaustive coverage of the
# weather path (including the §18.3/§18.4 stub-container provenance
# guard and the outage variant) lives in the SCOPE-07-owned BS-003
# (tests/e2e/assistant_bs003_test.sh) and BS-006
# (tests/e2e/assistant_bs006_test.sh) fixtures. This fixture is the
# SCOPE-10 v1-acceptance smoke marker.
#
# Tier gate: cpu → SKIP; accel → runs against the live stack.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

skip_unless_accel_tier "ACCEPT-WEATHER"

trap e2e_cleanup EXIT
echo "=== Spec 061 SCOPE-10 DoD #5 — weather acceptance smoke ==="
e2e_start
ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
WEBHOOK_SECRET="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TELEGRAM_WEBHOOK_SECRET")"
WEBHOOK_PATH="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH")"

NONCE="$(date +%s%N)$$"
UPDATE_ID="$NONCE"
CHAT_ID=99011
SINCE_TS="$(date --utc +%Y-%m-%dT%H:%M:%S)"
PAYLOAD=$(cat <<JSON
{"update_id": $UPDATE_ID, "message": {"message_id": $UPDATE_ID,
 "date": $(date +%s), "chat": {"id": $CHAT_ID, "type": "private"},
 "from": {"id": $CHAT_ID, "is_bot": false, "first_name": "AcceptW"},
 "text": "/weather Reykjavik"}}
JSON
)

HTTP_STATUS=$(curl -s -o /tmp/accept_weather_resp.txt -w "%{http_code}" --max-time 10 \
  -X POST -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
  -d "$PAYLOAD" "$CORE_URL$WEBHOOK_PATH" || true)
[ "$HTTP_STATUS" = "200" ] || e2e_fail "ACCEPT-WEATHER: webhook returned $HTTP_STATUS"

TURN_LINE=""
for _ in $(seq 1 60); do
  TURN_LINE="$(docker logs --since "$SINCE_TS" smackerel-test-smackerel-core-1 2>&1 \
              | grep -F '"msg":"assistant_turn"' \
              | grep -F "\"correlation_id\":\"$UPDATE_ID\"" | tail -1 || true)"
  [ -n "$TURN_LINE" ] && break
  sleep 1
done
[ -n "$TURN_LINE" ] || e2e_fail "ACCEPT-WEATHER: no assistant_turn slog line within 60s"
SCENARIO_ID=$(echo "$TURN_LINE" | jq -r '.scenario_id // ""')
[ "$SCENARIO_ID" = "weather_query" ] || \
  e2e_fail "ACCEPT-WEATHER: scenario_id='$SCENARIO_ID' (want 'weather_query')"

e2e_pass "ACCEPT-WEATHER: /weather dispatched to weather_query (scenario_id=$SCENARIO_ID)"
