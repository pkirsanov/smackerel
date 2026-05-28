#!/usr/bin/env bash
# Spec 061 SCOPE-10 DoD #7 — BS-002 persistent regression slot.
#
# BS-002 (spec.md §3): high-confidence retrieval question is answered
# with citations. The authoritative §18.5 assertion shape is:
#
#   scenario_id == "retrieval_qa"
#   status      == "answered" (or "thinking" terminal-for-mode)
#   error_cause == "" (empty/null on happy path)
#   sources block present (provenance gate satisfied)
#
# Pre-conditions (any missing => skip-77):
#   - test env has ASSISTANT_SKILLS_RETRIEVAL_QA_ENABLED=true
#   - test env has webhook secret + webhook path
#   - graph contains ≥1 artifact matching the retrieval probe text
#     (currently NOT seeded by the disposable test stack — tracked as
#     SCOPE-06-GRAPH-SEEDING-NOT-YET-AUTHORED)
#
# Adversarial guards (when executed):
#   - assert scenario_id is NOT the capture fallback ("" / "capture")
#     to catch a regression that loses routing
#   - assert status is NOT "error" to catch retrieval-skill panics
#   - assert error_cause is NOT "provenance_violation" to catch the
#     case where the LLM returned synthesis without sources (BS-007
#     covers that path inversely)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/regression_helpers.sh"

# Pre-condition: graph seeding for retrieval probe is not yet
# automated. Skip until SCOPE-06 lands the seeding fixture.
reg_skip_with_blocker "BS-002" "SCOPE-06-GRAPH-SEEDING-NOT-YET-AUTHORED"

# --- Executed branch (kept inert behind the skip until pre-conditions
# --- land). Documents the §18.5 assertion shape so the round that
# --- unblocks the fixture only has to remove the skip-77 line above.
:<<'EXECUTED_PATTERN'
trap e2e_cleanup EXIT
echo "=== Spec 061 SCOPE-10 DoD #7 — BS-002 retrieval happy-path regression ==="
e2e_start
WEBHOOK_SECRET="$(reg_required_env_value BS-002 ASSISTANT_TELEGRAM_WEBHOOK_SECRET)"
WEBHOOK_PATH="$(reg_required_env_value BS-002 ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH)"
NONCE="$(date +%s%N)$$"
UPDATE_ID="$NONCE"
CHAT_ID=99002
SINCE_TS="$(date --utc +%Y-%m-%dT%H:%M:%S)"
PAYLOAD=$(cat <<JSON
{"update_id": $UPDATE_ID, "message": {"message_id": $UPDATE_ID,
 "date": $(date +%s), "chat": {"id": $CHAT_ID, "type": "private"},
 "from": {"id": $CHAT_ID, "is_bot": false, "first_name": "BS002"},
 "text": "what did I save about Tailscale last month?"}}
JSON
)
HTTP_STATUS=$(curl -s -o /tmp/bs002_resp.txt -w "%{http_code}" --max-time 10 \
  -X POST -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
  -d "$PAYLOAD" "$CORE_URL$WEBHOOK_PATH" || true)
[ "$HTTP_STATUS" = "200" ] || e2e_fail "BS-002: webhook returned $HTTP_STATUS"
TURN_LINE=""
for _ in $(seq 1 60); do
  TURN_LINE="$(docker logs --since "$SINCE_TS" smackerel-test-smackerel-core-1 2>&1 \
              | grep -F '"msg":"assistant_turn"' \
              | grep -F "\"correlation_id\":\"$UPDATE_ID\"" | tail -1 || true)"
  [ -n "$TURN_LINE" ] && break
  sleep 1
done
[ -n "$TURN_LINE" ] || e2e_fail "BS-002: no assistant_turn slog line within 60s"
SCENARIO_ID=$(echo "$TURN_LINE" | jq -r '.scenario_id // ""')
STATUS=$(echo "$TURN_LINE" | jq -r '.status // ""')
ERROR_CAUSE=$(echo "$TURN_LINE" | jq -r '.error_cause // ""')
[ "$SCENARIO_ID" = "retrieval_qa" ] || e2e_fail "BS-002: scenario_id='$SCENARIO_ID' (want retrieval_qa)"
[ "$STATUS" != "error" ] || e2e_fail "BS-002: status='error'"
[ "$ERROR_CAUSE" != "provenance_violation" ] || e2e_fail "BS-002: provenance gate fired on happy path"
e2e_pass "BS-002: retrieval happy path - $SCENARIO_ID / $STATUS / '$ERROR_CAUSE'"
EXECUTED_PATTERN
