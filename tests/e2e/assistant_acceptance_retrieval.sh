#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #5 — Retrieval acceptance smoke (1 of 4).
#
# Minimal happy-path smoke for the retrieval skill: drives /ask
# through the live Telegram webhook on the disposable test stack and
# asserts a single §18.5 invariant — the assistant_turn slog line
# carries scenario_id="retrieval_qa". Exhaustive coverage of the
# retrieval path lives in the SCOPE-06-owned BS-002 fixture
# (tests/e2e/assistant_bs002_test.sh); this fixture is the SCOPE-10
# v1-acceptance smoke marker that proves the dispatch wiring holds
# end-to-end through the real Telegram adapter.
#
# Tier gate: cpu → SKIP (live-stack inference overshoots ceiling per
# SCOPE-06c Round 71e); accel → runs the assertion against the
# live stack. The cpu-tier SKIP is the canonical CI mode for this
# fixture per docs/Testing.md → "Tier-gated live-stack tests".

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

skip_unless_accel_tier "ACCEPT-RETRIEVAL"

trap e2e_cleanup EXIT
echo "=== Spec 061 SCOPE-10 DoD #5 — retrieval acceptance smoke ==="
e2e_start
ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
WEBHOOK_SECRET="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TELEGRAM_WEBHOOK_SECRET")"
WEBHOOK_PATH="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH")"

NONCE="$(date +%s%N)$$"
UPDATE_ID="$NONCE"
CHAT_ID=99010
SINCE_TS="$(date --utc +%Y-%m-%dT%H:%M:%S)"
PAYLOAD=$(cat <<JSON
{"update_id": $UPDATE_ID, "message": {"message_id": $UPDATE_ID,
 "date": $(date +%s), "chat": {"id": $CHAT_ID, "type": "private"},
 "from": {"id": $CHAT_ID, "is_bot": false, "first_name": "AcceptR"},
 "text": "/ask what did I save about Tailscale"}}
JSON
)

HTTP_STATUS=$(curl -s -o /tmp/accept_retrieval_resp.txt -w "%{http_code}" --max-time 10 \
  -X POST -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
  -d "$PAYLOAD" "$CORE_URL$WEBHOOK_PATH" || true)
[ "$HTTP_STATUS" = "200" ] || e2e_fail "ACCEPT-RETRIEVAL: webhook returned $HTTP_STATUS"

TURN_LINE=""
for _ in $(seq 1 60); do
  TURN_LINE="$(docker logs --since "$SINCE_TS" smackerel-test-smackerel-core-1 2>&1 \
              | grep -F '"msg":"assistant_turn"' \
              | grep -F "\"correlation_id\":\"$UPDATE_ID\"" | tail -1 || true)"
  [ -n "$TURN_LINE" ] && break
  sleep 1
done
[ -n "$TURN_LINE" ] || e2e_fail "ACCEPT-RETRIEVAL: no assistant_turn slog line within 60s"
SCENARIO_ID=$(echo "$TURN_LINE" | jq -r '.scenario_id // ""')
[ "$SCENARIO_ID" = "retrieval_qa" ] || \
  e2e_fail "ACCEPT-RETRIEVAL: scenario_id='$SCENARIO_ID' (want 'retrieval_qa')"

e2e_pass "ACCEPT-RETRIEVAL: /ask dispatched to retrieval_qa (scenario_id=$SCENARIO_ID)"
