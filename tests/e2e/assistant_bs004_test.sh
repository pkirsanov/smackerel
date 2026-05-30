#!/usr/bin/env bash
# Spec 061 SCOPE-08 design §18.5 — BS-004 notification confirm-flow e2e.
#
# Tier-gated SKIP on cpu (operator-defer to accel per docs/Testing.md
# → "Tier-gated live-stack tests"). On accel-tier the fixture drives
# the REQUEST turn of the notification confirm flow through the live
# webhook and asserts the §18.5 slog scrape pattern:
#   - scenario_id == "notification_schedule"  (slot-extraction routed)
#   - status      == "awaiting_confirm"        (confirm-card proposed)
#   - error_cause is empty                     (happy path)
#   - body_redacted == true                    (§18.5 Principle 8)
#
# Foreign-owned blockers carried forward (per SCOPE-08 closure annotation):
#   - CALLBACK TURN (`status="reminder_confirmed"`) requires the spec 054
#     scheduler-binding follow-up scope to land — the current
#     `notificationSchedulerStub` in `cmd/core/wiring_assistant_skills.go`
#     accepts the call but does not persist a row, so a row-poll
#     assertion would be a false negative.
#   - REAL SCHEDULER ROW assertion (PG `scheduler_jobs` poll for
#     `source="assistant.skill.notifications"`) likewise depends on the
#     spec 054 follow-up. When that lands, an operator extends THIS
#     fixture with the callback POST + row poll (the §18.6 correlation
#     propagation pattern is already shown by BS-007).
#
# Adversarial guard: scenario_id="notification_schedule" rules out the
# capture-fallback band=low path (which leaves scenario_id="" because
# routing bails out before scenario dispatch — facade.go ~L355). The
# assertion that scenario_id=="notification_schedule" AND
# status=="awaiting_confirm" can therefore ONLY come from the
# notification skill firing through the confirm machine.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

# Tier-gated SKIP — design.md §5.1; docs/Testing.md "Tier-gated live-stack tests".
skip_unless_accel_tier "BS-004"

trap e2e_cleanup EXIT

echo "=== Spec 061 SCOPE-08 §18.5 — BS-004 notification confirm-flow e2e (REQUEST turn) ==="
e2e_setup
if ! curl -sf --max-time 5 -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/api/health" >/dev/null 2>&1; then
  e2e_fail "BS-004: core health endpoint unreachable at $CORE_URL/api/health"
fi

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
WEBHOOK_SECRET="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TELEGRAM_WEBHOOK_SECRET")"
WEBHOOK_PATH="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH")"

# int64-safe unique nonce.
NONCE=$(( $(date +%s) * 100000 + RANDOM ))
UPDATE_ID="$NONCE"
CHAT_ID=99004
SINCE_TS="$(date --utc +%Y-%m-%dT%H:%M:%S)"

# --- LLM warmup so the first /remind invocation does not blow the budget --
AGENT_PROVIDER_DEFAULT_MODEL="$(smackerel_env_value "$ENV_FILE" "AGENT_PROVIDER_DEFAULT_MODEL")"
if [ -z "$AGENT_PROVIDER_DEFAULT_MODEL" ]; then
  e2e_fail "BS-004: AGENT_PROVIDER_DEFAULT_MODEL missing/empty in $ENV_FILE"
fi
echo "--- LLM warmup: priming $AGENT_PROVIDER_DEFAULT_MODEL inference ---"
curl -s -o /dev/null --max-time 60 \
  -X POST \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"$AGENT_PROVIDER_DEFAULT_MODEL\",\"prompt\":\"hi\",\"stream\":false,\"options\":{\"num_predict\":4}}" \
  "http://127.0.0.1:47004/api/generate" || true

PAYLOAD=$(cat <<JSON
{
  "update_id": $UPDATE_ID,
  "message": {
    "message_id": $UPDATE_ID,
    "date": $(date +%s),
    "chat": {"id": $CHAT_ID, "type": "private"},
    "from": {"id": $CHAT_ID, "is_bot": false, "first_name": "BS004"},
    "text": "/remind me to call Mom at 6pm tomorrow"
  }
}
JSON
)

echo "--- ROW-1: webhook POST with /remind (well-formed slot) ---"
HTTP_STATUS=$(curl -s -o /tmp/bs004_resp.txt -w "%{http_code}" \
  --max-time 180 \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
  -d "$PAYLOAD" \
  "$CORE_URL$WEBHOOK_PATH" || true)
echo "  http_status=$HTTP_STATUS body=$(cat /tmp/bs004_resp.txt)"
if [ "$HTTP_STATUS" != "200" ]; then
  e2e_fail "BS-004: webhook returned $HTTP_STATUS (want 200)"
fi

echo "--- §18.5 slog scrape for assistant_turn with correlation_id=$UPDATE_ID ---"
TURN_LINE=""
for _ in $(seq 1 120); do
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
  e2e_fail "BS-004: no assistant_turn slog line with correlation_id=$UPDATE_ID within 120s"
fi
echo "  matched: $TURN_LINE"

SCENARIO_ID=$(echo "$TURN_LINE" | jq -r '.scenario_id // ""')
STATUS=$(echo "$TURN_LINE" | jq -r '.status // ""')
ERROR_CAUSE=$(echo "$TURN_LINE" | jq -r '.error_cause // ""')
BODY_REDACTED=$(echo "$TURN_LINE" | jq -r '.body_redacted // "false"')

if [ "$SCENARIO_ID" != "notification_schedule" ]; then
  e2e_fail "BS-004: scenario_id='$SCENARIO_ID' (want 'notification_schedule'); /remind did not route to notification skill"
fi
if [ "$STATUS" != "awaiting_confirm" ]; then
  e2e_fail "BS-004: status='$STATUS' (want 'awaiting_confirm'); confirm-card was not proposed"
fi
if [ -n "$ERROR_CAUSE" ] && [ "$ERROR_CAUSE" != "null" ]; then
  e2e_fail "BS-004: error_cause='$ERROR_CAUSE' (want empty on happy path)"
fi
if [ "$BODY_REDACTED" != "true" ]; then
  e2e_fail "BS-004: body_redacted='$BODY_REDACTED' (want true; §18.5 Principle 8)"
fi

echo "  §18.5 adversarial guard: scenario_id==notification_schedule AND status==awaiting_confirm is uniquely the notification-skill confirm-card path; capture-fallback would have scenario_id=='' (no scenario routed)."
echo "  CARRIED FORWARD: callback turn (status=reminder_confirmed) + scheduler-row PG poll require spec 054 scheduler-binding follow-up. Extend this fixture when that lands."

e2e_pass "BS-004: /remind routed to notification_schedule, confirm-card proposed (status=awaiting_confirm), correlation_id matched, body_redacted=true"
echo "  scenario_id=$SCENARIO_ID status=$STATUS error_cause='$ERROR_CAUSE' body_redacted=$BODY_REDACTED"
