#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #5 — Capture acceptance smoke (4 of 4).
#
# Minimal happy-path smoke for the capture-fallback path: drives
# plain text (no slash shortcut, low-confidence routing) through the
# live Telegram webhook on the disposable test stack and asserts a
# single §18.5 invariant — the assistant_turn slog line carries
# scenario_id="" (the canonical no-scenario marker per BS-004
# §18.5 comment block — "capture-fallback would have
# scenario_id=='' (no scenario routed)"). Exhaustive coverage of the
# capture-fallback path lives in tests/e2e/test_telegram_assistant_bs001.sh
# and the SCOPE-04-owned regression coverage; this fixture is the
# SCOPE-10 v1-acceptance smoke marker for the capture branch.
#
# Tier gate: cpu → SKIP; accel → runs against the live stack. (Capture
# fallback itself does not require accelerator inference, but it
# routes through the same router → embedding classifier path that the
# other three smoke fixtures exercise, so the tier-gate is consistent
# across all four acceptance smokes.)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

skip_unless_accel_tier "ACCEPT-CAPTURE"

trap e2e_cleanup EXIT
echo "=== Spec 061 SCOPE-10 DoD #5 — capture acceptance smoke ==="
e2e_start
ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
WEBHOOK_SECRET="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TELEGRAM_WEBHOOK_SECRET")"
WEBHOOK_PATH="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH")"

NONCE="$(date +%s%N)$$"
UPDATE_ID="$NONCE"
CHAT_ID=99013
SINCE_TS="$(date --utc +%Y-%m-%dT%H:%M:%S)"
PAYLOAD=$(cat <<JSON
{"update_id": $UPDATE_ID, "message": {"message_id": $UPDATE_ID,
 "date": $(date +%s), "chat": {"id": $CHAT_ID, "type": "private"},
 "from": {"id": $CHAT_ID, "is_bot": false, "first_name": "AcceptC"},
 "text": "interesting random thought to remember later"}}
JSON
)

HTTP_STATUS=$(curl -s -o /tmp/accept_capture_resp.txt -w "%{http_code}" --max-time 10 \
  -X POST -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
  -d "$PAYLOAD" "$CORE_URL$WEBHOOK_PATH" || true)
[ "$HTTP_STATUS" = "200" ] || e2e_fail "ACCEPT-CAPTURE: webhook returned $HTTP_STATUS"

TURN_LINE=""
for _ in $(seq 1 60); do
  TURN_LINE="$(docker logs --since "$SINCE_TS" smackerel-test-smackerel-core-1 2>&1 \
              | grep -F '"msg":"assistant_turn"' \
              | grep -F "\"correlation_id\":\"$UPDATE_ID\"" | tail -1 || true)"
  [ -n "$TURN_LINE" ] && break
  sleep 1
done
[ -n "$TURN_LINE" ] || e2e_fail "ACCEPT-CAPTURE: no assistant_turn slog line within 60s"
SCENARIO_ID=$(echo "$TURN_LINE" | jq -r '.scenario_id // ""')
[ "$SCENARIO_ID" = "" ] || \
  e2e_fail "ACCEPT-CAPTURE: scenario_id='$SCENARIO_ID' (want '' — capture-fallback marker per BS-004 §18.5 note)"

e2e_pass "ACCEPT-CAPTURE: plain text routed to capture-fallback (scenario_id='')"
