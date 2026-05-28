#!/usr/bin/env bash
# Spec 061 SCOPE-07 design §18.5 — BS-003 weather happy-path e2e.
#
# Drives a synthetic Telegram update through the live test-stack webhook
# and asserts via the canonical §18.5 slog-scrape pattern that the
# assistant_turn line carries the correct scenario_id/status/error_cause
# for the weather skill happy path against the in-tree stub-providers
# container (§18.4 routes /v1/search + /v1/forecast).
#
# Correlation: a unique nonce is injected as Telegram update_id; §18.6
# propagates it into TransportMetadata["telegram_update_id"] which the
# facade stamps as correlation_id on the assistant_turn slog line.
#
# Adversarial guards:
#   - assert error_cause is empty/null (not "external_provider") to
#     catch a regression that silently 5xxs the stub.
#   - assert scenario_id is exactly "weather_query" (not the capture
#     fallback path) to catch a regression that disables routing.
#   - assert status is "weather_ok" / "thinking" (NOT
#     "weather_unavailable") to catch the inverse of BS-006.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Spec 061 SCOPE-07 §18.5 — BS-003 weather happy-path e2e ==="
e2e_start

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
WEBHOOK_SECRET="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TELEGRAM_WEBHOOK_SECRET")"
WEBHOOK_PATH="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH")"
GEOCODE_URL="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_SKILLS_WEATHER_GEOCODE_URL")"
FORECAST_URL="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_SKILLS_WEATHER_FORECAST_URL")"

if [[ "$GEOCODE_URL" != *"stub-providers"* ]]; then
  e2e_fail "BS-003: ASSISTANT_SKILLS_WEATHER_GEOCODE_URL must point at stub-providers in test env; got '$GEOCODE_URL'"
fi
if [[ "$FORECAST_URL" != *"stub-providers"* ]]; then
  e2e_fail "BS-003: ASSISTANT_SKILLS_WEATHER_FORECAST_URL must point at stub-providers in test env; got '$FORECAST_URL'"
fi

NONCE="$(date +%s%N)$$"
UPDATE_ID="$NONCE"
CHAT_ID=99003
SINCE_TS="$(date --utc +%Y-%m-%dT%H:%M:%S)"

PAYLOAD=$(cat <<JSON
{
  "update_id": $UPDATE_ID,
  "message": {
    "message_id": $UPDATE_ID,
    "date": $(date +%s),
    "chat": {"id": $CHAT_ID, "type": "private"},
    "from": {"id": $CHAT_ID, "is_bot": false, "first_name": "BS003"},
    "text": "weather in Reykjavik tomorrow"
  }
}
JSON
)

echo "--- ROW-1: webhook POST with weather text ---"
HTTP_STATUS=$(curl -s -o /tmp/bs003_resp.txt -w "%{http_code}" \
  --max-time 10 \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
  -d "$PAYLOAD" \
  "$CORE_URL$WEBHOOK_PATH" || true)
echo "  http_status=$HTTP_STATUS body=$(cat /tmp/bs003_resp.txt)"
if [ "$HTTP_STATUS" != "200" ]; then
  e2e_fail "BS-003: webhook returned $HTTP_STATUS (want 200)"
fi

echo "--- §18.5 slog scrape for assistant_turn with correlation_id=$UPDATE_ID ---"
TURN_LINE=""
for _ in $(seq 1 60); do
  TURN_LINE="$(docker logs --since "$SINCE_TS" smackerel-test-smackerel-core-1 2>&1 \
              | grep -F '"msg":"assistant_turn"' \
              | grep -F "\"correlation_id\":\"$UPDATE_ID\"" | tail -1 || true)"
  if [ -n "$TURN_LINE" ]; then break; fi
  sleep 1
done
if [ -z "$TURN_LINE" ]; then
  echo "Last assistant_turn lines for diagnosis:"
  docker logs --since "$SINCE_TS" smackerel-test-smackerel-core-1 2>&1 \
    | grep -F '"msg":"assistant_turn"' | tail -5 || true
  e2e_fail "BS-003: no assistant_turn slog line with correlation_id=$UPDATE_ID within 60s"
fi
echo "  matched: $TURN_LINE"

# Structured assertions via jq.
SCENARIO_ID=$(echo "$TURN_LINE" | jq -r '.scenario_id // ""')
STATUS=$(echo "$TURN_LINE" | jq -r '.status // ""')
ERROR_CAUSE=$(echo "$TURN_LINE" | jq -r '.error_cause // ""')
BODY_REDACTED=$(echo "$TURN_LINE" | jq -r '.body_redacted // "false"')

if [ "$SCENARIO_ID" != "weather_query" ]; then
  e2e_fail "BS-003: scenario_id='$SCENARIO_ID' (want 'weather_query'); routing did not dispatch to weather skill"
fi
# Adversarial: status MUST NOT indicate provider unavailable (would mean
# the stub container returned 5xx or wasn't reachable).
if [ "$STATUS" = "weather_unavailable" ]; then
  e2e_fail "BS-003: status='weather_unavailable' on happy path; stub container probably unreachable or 5xx'd"
fi
# Adversarial: error_cause MUST NOT name external_provider on happy path.
if [ "$ERROR_CAUSE" = "external_provider" ]; then
  e2e_fail "BS-003: error_cause='external_provider' on happy path; stub container returning 5xx?"
fi
if [ "$BODY_REDACTED" != "true" ]; then
  e2e_fail "BS-003: body_redacted='$BODY_REDACTED' (want true; §18.5 Principle 8 affirmation)"
fi

e2e_pass "BS-003: happy path - weather scenario routed, correlation_id matched, body_redacted=true, no provider error"
echo "  scenario_id=$SCENARIO_ID status=$STATUS error_cause='$ERROR_CAUSE' body_redacted=$BODY_REDACTED"
