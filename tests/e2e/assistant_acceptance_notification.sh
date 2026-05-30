#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #5 — Notification acceptance smoke (3 of 4).
#
# Minimal happy-path smoke for the notification skill confirm-card
# flow: drives /remind through the live Telegram webhook on the
# disposable test stack and asserts two §18.5 invariants — the
# assistant_turn slog line carries scenario_id="notification_schedule"
# AND status="awaiting_confirm" (which is the uniquely-notification
# terminal-for-mode state per BS-004 §18.5 assertion shape; the
# combined assertion catches a regression that silently routes /remind
# to capture-fallback). Exhaustive coverage of the notification path
# (including the confirm/cancel branches and the SCOPE-08-owned state
# machine) lives in tests/e2e/assistant_bs004_test.sh; this fixture is
# the SCOPE-10 v1-acceptance smoke marker.
#
# Tier gate: cpu → SKIP; accel → runs against the live stack.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

skip_unless_accel_tier "ACCEPT-NOTIFICATION"

trap e2e_cleanup EXIT
echo "=== Spec 061 SCOPE-10 DoD #5 — notification acceptance smoke ==="
e2e_start
ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
WEBHOOK_SECRET="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TELEGRAM_WEBHOOK_SECRET")"
WEBHOOK_PATH="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH")"

NONCE="$(date +%s%N)$$"
UPDATE_ID="$NONCE"
CHAT_ID=99012
SINCE_TS="$(date --utc +%Y-%m-%dT%H:%M:%S)"
PAYLOAD=$(cat <<JSON
{"update_id": $UPDATE_ID, "message": {"message_id": $UPDATE_ID,
 "date": $(date +%s), "chat": {"id": $CHAT_ID, "type": "private"},
 "from": {"id": $CHAT_ID, "is_bot": false, "first_name": "AcceptN"},
 "text": "/remind me to water plants tomorrow 9am"}}
JSON
)

HTTP_STATUS=$(curl -s -o /tmp/accept_notif_resp.txt -w "%{http_code}" --max-time 10 \
  -X POST -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
  -d "$PAYLOAD" "$CORE_URL$WEBHOOK_PATH" || true)
[ "$HTTP_STATUS" = "200" ] || e2e_fail "ACCEPT-NOTIFICATION: webhook returned $HTTP_STATUS"

TURN_LINE=""
for _ in $(seq 1 60); do
  TURN_LINE="$(docker logs --since "$SINCE_TS" smackerel-test-smackerel-core-1 2>&1 \
              | grep -F '"msg":"assistant_turn"' \
              | grep -F "\"correlation_id\":\"$UPDATE_ID\"" | tail -1 || true)"
  [ -n "$TURN_LINE" ] && break
  sleep 1
done
[ -n "$TURN_LINE" ] || e2e_fail "ACCEPT-NOTIFICATION: no assistant_turn slog line within 60s"
SCENARIO_ID=$(echo "$TURN_LINE" | jq -r '.scenario_id // ""')
STATUS=$(echo "$TURN_LINE" | jq -r '.status // ""')
[ "$SCENARIO_ID" = "notification_schedule" ] || \
  e2e_fail "ACCEPT-NOTIFICATION: scenario_id='$SCENARIO_ID' (want 'notification_schedule')"
[ "$STATUS" = "awaiting_confirm" ] || \
  e2e_fail "ACCEPT-NOTIFICATION: status='$STATUS' (want 'awaiting_confirm'; confirm-card not raised)"

e2e_pass "ACCEPT-NOTIFICATION: /remind dispatched, confirm-card raised (scenario_id=$SCENARIO_ID status=$STATUS)"
