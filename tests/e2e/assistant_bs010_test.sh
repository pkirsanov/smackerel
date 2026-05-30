#!/usr/bin/env bash
# Spec 061 SCOPE-05 DoD #9 — BS-010 Telegram reference adapter e2e (v1 acceptance).
#
# BS-010 is the v1 acceptance scenario for the Telegram reference adapter:
# capability → adapter → tgbotapi flow end-to-end with per-transport telemetry.
# It overlaps substantially with BS-002 (retrieval happy-path) — both drive
# /ask through the Telegram webhook and assert the assistant_turn slog
# line. BS-010's distinguishing element vs BS-002 is the per-transport
# telemetry surface: assert transport="telegram" on assistant_turn AND
# confirm the per-transport webhook metric family is exposed via /metrics.
#
# Tier policy: cpu tier → structured SKIP + exit 0 (same gate as BS-002/003/006/007
# per SCOPE-06c PACKET 4; live-stack retrieval-qa loop overshoots the 15s
# ceiling on non-accelerator hardware — see docs/Testing.md → "Tier-gated
# live-stack tests" and design §5.1).
#
# Deep retrieval coverage lives in assistant_bs002_test.sh; BS-010 adds the
# per-transport assertion layer on top of that proven shape.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

# SCOPE-06c PACKET 4 + SCOPE-05 DoD #9 — tier-gated SKIP for cpu dev loop.
skip_unless_accel_tier "BS-010"

ARTIFACT_ID=""

bs010_cleanup() {
  if [ -n "$ARTIFACT_ID" ]; then
    echo "--- cleanup: DELETE seeded artifact $ARTIFACT_ID ---"
    curl -s -o /dev/null -w "  delete_status=%{http_code}\n" \
      --max-time 10 \
      -X DELETE \
      -H "Authorization: Bearer $AUTH_TOKEN" \
      "$CORE_URL/api/artifacts/$ARTIFACT_ID" || true
  fi
  e2e_cleanup
}
trap bs010_cleanup EXIT

echo "=== Spec 061 SCOPE-05 DoD #9 — BS-010 Telegram adapter v1 acceptance e2e ==="
e2e_setup
if ! curl -sf --max-time 5 -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/api/health" >/dev/null 2>&1; then
  e2e_fail "BS-010: core health endpoint unreachable at $CORE_URL/api/health"
fi

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
WEBHOOK_SECRET="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TELEGRAM_WEBHOOK_SECRET")"
WEBHOOK_PATH="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH")"

NONCE=$(( $(date +%s) * 100000 + RANDOM ))
UPDATE_ID="$NONCE"
CHAT_ID=99010
SEED_MARKER="bs010seed${NONCE}"
SINCE_TS="$(date --utc +%Y-%m-%dT%H:%M:%S)"

echo "--- seed: POST /api/capture with marker=$SEED_MARKER ---"
SEED_PAYLOAD=$(cat <<JSON
{"text": "BS-010 acceptance seed note (${SEED_MARKER}): the Telegram reference adapter is the v1 transport; per-transport telemetry tags assistant_turn with transport=telegram. Tag: bs010-${SEED_MARKER}. This is the only BS-010 acceptance artifact."}
JSON
)
SEED_RESP=$(curl -s --max-time 120 \
  -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -d "$SEED_PAYLOAD" \
  "$CORE_URL/api/capture")
ARTIFACT_ID=$(echo "$SEED_RESP" | jq -r '.artifact_id // ""')
if [ -z "$ARTIFACT_ID" ] || [ "$ARTIFACT_ID" = "null" ]; then
  e2e_fail "BS-010: capture did not return artifact_id; response=$SEED_RESP"
fi
echo "  seeded artifact_id=$ARTIFACT_ID"
sleep 3

# LLM warmup per BS-002 shape (model name from SST, no fallback).
AGENT_PROVIDER_DEFAULT_MODEL="$(smackerel_env_value "$ENV_FILE" "AGENT_PROVIDER_DEFAULT_MODEL")"
if [ -z "$AGENT_PROVIDER_DEFAULT_MODEL" ]; then
  e2e_fail "BS-010: AGENT_PROVIDER_DEFAULT_MODEL missing in $ENV_FILE"
fi
echo "--- LLM warmup: priming $AGENT_PROVIDER_DEFAULT_MODEL ---"
curl -s -o /dev/null --max-time 180 \
  -X POST \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"$AGENT_PROVIDER_DEFAULT_MODEL\",\"prompt\":\"hi\",\"stream\":false,\"options\":{\"num_predict\":64}}" \
  "http://127.0.0.1:47004/api/generate" || true

PAYLOAD=$(cat <<JSON
{
  "update_id": $UPDATE_ID,
  "message": {
    "message_id": $UPDATE_ID,
    "date": $(date +%s),
    "chat": {"id": $CHAT_ID, "type": "private"},
    "from": {"id": $CHAT_ID, "is_bot": false, "first_name": "BS010"},
    "text": "/ask what did I save about the BS-010 acceptance note?"
  }
}
JSON
)

echo "--- webhook POST /ask (routes to retrieval_qa via shortcut) ---"
HTTP_STATUS=$(curl -s -o /tmp/bs010_resp.txt -w "%{http_code}" \
  --max-time 180 \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
  -d "$PAYLOAD" \
  "$CORE_URL$WEBHOOK_PATH" || true)
echo "  http_status=$HTTP_STATUS body=$(cat /tmp/bs010_resp.txt)"
if [ "$HTTP_STATUS" != "200" ]; then
  e2e_fail "BS-010: webhook returned $HTTP_STATUS (want 200)"
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
  e2e_fail "BS-010: no assistant_turn slog line with correlation_id=$UPDATE_ID within 120s"
fi
echo "  matched: $TURN_LINE"

SCENARIO_ID=$(echo "$TURN_LINE" | jq -r '.scenario_id // ""')
STATUS=$(echo "$TURN_LINE" | jq -r '.status // ""')
TRANSPORT=$(echo "$TURN_LINE" | jq -r '.transport // ""')

if [ "$SCENARIO_ID" != "retrieval_qa" ]; then
  e2e_fail "BS-010: scenario_id='$SCENARIO_ID' (want 'retrieval_qa')"
fi
if [ "$STATUS" = "saved_as_idea" ]; then
  e2e_fail "BS-010: status='saved_as_idea' on happy path (provenance gate refused; empty Sources?)"
fi
# BS-010 distinguishing assertion vs BS-002: per-transport telemetry tag.
if [ "$TRANSPORT" != "telegram" ]; then
  e2e_fail "BS-010: transport='$TRANSPORT' (want 'telegram'); per-transport telemetry tagging missing on assistant_turn"
fi

echo "--- /metrics scrape: assert per-transport webhook metric family exposed ---"
METRICS_BODY="$(curl -s --max-time 10 -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/metrics" || true)"
if ! echo "$METRICS_BODY" | grep -qE '^# (HELP|TYPE) assistant_telegram_webhook_auth_failures_total'; then
  e2e_fail "BS-010: assistant_telegram_webhook_auth_failures_total metric family not exposed on /metrics (per-transport telemetry surface broken)"
fi

e2e_pass "BS-010: Telegram adapter v1 acceptance — /ask → retrieval_qa, transport=telegram on assistant_turn, per-transport webhook metric family exposed on /metrics"
echo "  scenario_id=$SCENARIO_ID status=$STATUS transport=$TRANSPORT"
