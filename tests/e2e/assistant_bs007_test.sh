#!/usr/bin/env bash
# Spec 061 SCOPE-06 design §18.5 — BS-007 retrieval Q&A refusal e2e
# (synthesis without provenance is rejected).
#
# Drives a synthetic Telegram update through the live test-stack webhook
# with a query that cannot match any seeded artifact, so /api/search
# returns zero hits → the LLM has nothing to cite → cited_artifact_ids
# resolves to empty Sources[] → provenance.Enforce rewrites the response
# to the canonical refusal (Status=saved_as_idea, CaptureRoute=true).
#
# Deterministic routing: uses the v1 slash shortcut "/ask" which
# bypasses embedding-based routing (design §3.4) and dispatches
# directly to retrieval_qa. The fixture is non-tautological because
# any regression that allows the gate to pass with empty Sources
# would flip status away from "saved_as_idea" and fail the assertion.
#
# Adversarial guards:
#   - assert scenario_id == "retrieval_qa" (not capture-fallback, which
#     would have scenario_id="").
#   - assert status == "saved_as_idea" — the canonical refusal token.
#   - assert error_cause is empty/null — the gate intentionally does
#     not set ErrorCause (internal/assistant/provenance/gate.go ~L89)
#     because a soft refusal is not an unavailability error. A
#     regression that incorrectly stamped error_cause would fail this.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Spec 061 SCOPE-06 §18.5 — BS-007 retrieval refusal e2e ==="
# NOTE: e2e_wait_healthy currently rejects /api/health when its
# "services" key is null (helper expects per-service status objects).
# This fixture verifies the stack via the webhook ack itself instead
# and only loads env vars from e2e_setup, then checks the health
# endpoint is reachable before driving the real POST.
e2e_setup
if ! curl -sf --max-time 5 -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/api/health" >/dev/null 2>&1; then
  e2e_fail "BS-007: core health endpoint unreachable at $CORE_URL/api/health"
fi

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
WEBHOOK_SECRET="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TELEGRAM_WEBHOOK_SECRET")"
WEBHOOK_PATH="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH")"

# int64-safe unique nonce: seconds * 100000 + RANDOM (0..32767) is
# well under 2^63-1 and unique across a single fixture run.
NONCE=$(( $(date +%s) * 100000 + RANDOM ))
UPDATE_ID="$NONCE"
CHAT_ID=99007
# Random-feeling query against an empty graph for this user — guarantees
# /api/search returns zero hits even if other fixtures left artifacts
# behind, because nothing in the seed corpus mentions this nonce string.
QUERY_NONCE="qzqz${NONCE}xyzunmatchable"
SINCE_TS="$(date --utc +%Y-%m-%dT%H:%M:%S)"

# --- LLM warmup so the first /ask invocation does not blow the budget --
# Spec 061 SCOPE-06a (BS-002-OPTION2-INCOMPLETE-MULTI-PATH-MODEL-LEAK) —
# warmup MUST read the SST-resolved test-tier default model from the
# generated env file; missing/empty value aborts the test with a named error.
AGENT_PROVIDER_DEFAULT_MODEL="$(smackerel_env_value "$ENV_FILE" "AGENT_PROVIDER_DEFAULT_MODEL")"
if [ -z "$AGENT_PROVIDER_DEFAULT_MODEL" ]; then
  e2e_fail "BS-007: AGENT_PROVIDER_DEFAULT_MODEL missing/empty in $ENV_FILE (spec 061 SCOPE-06a; regenerate via './smackerel.sh --env test config generate')"
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
    "from": {"id": $CHAT_ID, "is_bot": false, "first_name": "BS007"},
    "text": "/ask tell me about ${QUERY_NONCE} which I never captured"
  }
}
JSON
)

echo "--- ROW-1: webhook POST with /ask (unmatchable query) ---"
HTTP_STATUS=$(curl -s -o /tmp/bs007_resp.txt -w "%{http_code}" \
  --max-time 180 \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
  -d "$PAYLOAD" \
  "$CORE_URL$WEBHOOK_PATH" || true)
echo "  http_status=$HTTP_STATUS body=$(cat /tmp/bs007_resp.txt)"
if [ "$HTTP_STATUS" != "200" ]; then
  e2e_fail "BS-007: webhook returned $HTTP_STATUS (want 200)"
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
  e2e_fail "BS-007: no assistant_turn slog line with correlation_id=$UPDATE_ID within 120s"
fi
echo "  matched: $TURN_LINE"

SCENARIO_ID=$(echo "$TURN_LINE" | jq -r '.scenario_id // ""')
STATUS=$(echo "$TURN_LINE" | jq -r '.status // ""')
ERROR_CAUSE=$(echo "$TURN_LINE" | jq -r '.error_cause // ""')
BODY_REDACTED=$(echo "$TURN_LINE" | jq -r '.body_redacted // "false"')

if [ "$SCENARIO_ID" != "retrieval_qa" ]; then
  e2e_fail "BS-007: scenario_id='$SCENARIO_ID' (want 'retrieval_qa'); /ask shortcut did not route to retrieval"
fi
if [ "$STATUS" != "saved_as_idea" ]; then
  e2e_fail "BS-007: status='$STATUS' (want 'saved_as_idea'); provenance gate did not refuse — either Sources was non-empty (unexpected on unmatchable query) OR the gate code regressed"
fi
if [ -n "$ERROR_CAUSE" ] && [ "$ERROR_CAUSE" != "null" ]; then
  e2e_fail "BS-007: error_cause='$ERROR_CAUSE' (want empty; gate intentionally leaves ErrorCause unset — soft refusal, not unavailability)"
fi
if [ "$BODY_REDACTED" != "true" ]; then
  e2e_fail "BS-007: body_redacted='$BODY_REDACTED' (want true; §18.5 Principle 8 affirmation)"
fi

# Adversarial substitution guard (§18.5): reasoning — status="saved_as_idea"
# could in principle come from (a) the provenance gate rewriting an empty-
# Sources retrieval response (the BS-007 path we want to assert), or
# (b) the capture-fallback band=low classifier outcome. Branch (b) would
# leave scenario_id="" (no scenario chosen) because routing bailed out
# before scenario dispatch (facade.go ~L355). The assertion above that
# scenario_id == "retrieval_qa" rules out (b), so status="saved_as_idea"
# with scenario_id="retrieval_qa" can ONLY come from the gate firing.
echo "  §18.5 adversarial guard: status==saved_as_idea AND scenario_id==retrieval_qa is uniquely the gate-rewrite path; capture-fallback would have scenario_id=='' (no scenario routed), so this fixture proves the gate fired on empty Sources."

e2e_pass "BS-007: retrieval refusal - /ask routed to retrieval_qa, correlation_id matched, provenance gate fired (status=saved_as_idea), error_cause empty (soft refusal), body_redacted=true"
echo "  scenario_id=$SCENARIO_ID status=$STATUS error_cause='$ERROR_CAUSE' body_redacted=$BODY_REDACTED"
