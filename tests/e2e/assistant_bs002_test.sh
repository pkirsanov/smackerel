#!/usr/bin/env bash
# Spec 061 SCOPE-06 design §18.5 — BS-002 retrieval Q&A happy-path e2e.
#
# Drives a synthetic Telegram update through the live test-stack webhook
# and asserts via the canonical §18.5 slog-scrape pattern that the
# retrieval_qa scenario routed end-to-end through the real /api/search
# (nomic-embed-text) + ml/ sidecar synthesis (gemma3:4b) pipeline,
# returned a sourced answer, and the provenance gate passed (i.e.
# status="thinking" + error_cause="" — gate would have rewritten to
# saved_as_idea if Sources had been empty).
#
# Deterministic routing: the fixture uses the v1 slash shortcut "/ask"
# which bypasses the embedding-based router (design §3.4) and dispatches
# directly to the retrieval_qa scenario. This eliminates classifier
# flakiness as a failure mode and isolates the retrieval-skill path.
#
# Seeding strategy: pre-seed ONE artifact via POST /api/capture
# (pipeline.Process is synchronous and writes the embedding inline per
# internal/pipeline/processor.go ~L656) so /api/search has a vector
# to retrieve. Cleanup via DELETE in the trap.
#
# Correlation: a unique nonce is injected as Telegram update_id; §18.6
# propagates it into TransportMetadata["telegram_update_id"] which the
# facade stamps as correlation_id on the assistant_turn slog line.
#
# Adversarial guards:
#   - assert status != "saved_as_idea" to catch the inverse (BS-007)
#     condition where provenance gate refused for empty Sources.
#   - assert scenario_id == "retrieval_qa" exactly to catch a routing
#     regression that would silently send /ask to capture fallback.
#   - assert error_cause is empty/null to catch any provider/timeout
#     surface.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

# SCOPE-06c PACKET 4 — tier-gated SKIP for cpu-tier dev loop
# (design.md §5.1; plan-triage 010a45d4; docs/Testing.md → Tier-gated live-stack tests).
skip_unless_accel_tier "BS-002"

ARTIFACT_ID=""

bs002_cleanup() {
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
trap bs002_cleanup EXIT

echo "=== Spec 061 SCOPE-06 §18.5 — BS-002 retrieval Q&A happy-path e2e ==="
# NOTE: e2e_wait_healthy currently rejects /api/health when its
# "services" key is null (helper expects per-service status objects).
# This fixture verifies the stack via the webhook ack itself instead
# and only loads env vars from e2e_setup, then checks the webhook is
# reachable with a HEAD-equivalent ping before driving the real POST.
e2e_setup
if ! curl -sf --max-time 5 -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/api/health" >/dev/null 2>&1; then
  e2e_fail "BS-002: core health endpoint unreachable at $CORE_URL/api/health"
fi

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
WEBHOOK_SECRET="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TELEGRAM_WEBHOOK_SECRET")"
WEBHOOK_PATH="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH")"

# int64-safe unique nonce: seconds * 100000 + RANDOM (0..32767) is
# well under 2^63-1 and unique across a single fixture run.
NONCE=$(( $(date +%s) * 100000 + RANDOM ))
UPDATE_ID="$NONCE"
CHAT_ID=99002
SEED_MARKER="bs002seed${NONCE}"
SINCE_TS="$(date --utc +%Y-%m-%dT%H:%M:%S)"

# --- Seed ONE deterministic artifact via /api/capture. -----------------
# Pipeline.Process is synchronous (internal/api/capture.go:96) so the
# embedding is written inline before the response returns; /api/search
# can retrieve it immediately on the next request.
echo "--- seed: POST /api/capture with marker=$SEED_MARKER ---"
# Spec 061 §5 BS-002 Gherkin: "at least one artifact about 'Tailscale' exists".
# Use distinctive, unambiguous Tailscale content with the marker embedded as a
# unique tag so the small model has an unmistakable citation target.
SEED_PAYLOAD=$(cat <<JSON
{"text": "Tailscale mesh VPN setup notes (${SEED_MARKER}): I installed Tailscale on the self-hosted <deploy-host> host last month for SSH access from anywhere. Tag: tailscale-setup-${SEED_MARKER}. Key settings: MagicDNS enabled, ACL allows SSH from my devices only. This is the only Tailscale artifact in my graph."}
JSON
)
SEED_RESP=$(curl -s --max-time 120 \
  -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -d "$SEED_PAYLOAD" \
  "$CORE_URL/api/capture")
echo "  seed_resp=$SEED_RESP"
ARTIFACT_ID=$(echo "$SEED_RESP" | jq -r '.artifact_id // ""')
if [ -z "$ARTIFACT_ID" ] || [ "$ARTIFACT_ID" = "null" ]; then
  e2e_fail "BS-002: capture did not return artifact_id; response=$SEED_RESP"
fi
echo "  seeded artifact_id=$ARTIFACT_ID"

# Brief settle to let any async post-processing (graph edges, topics)
# finish; the embedding itself is already in the row.
sleep 3

# --- LLM warmup so the first /ask invocation does not blow the budget --
# Spec 061 SCOPE-06a (BS-002-OPTION2-INCOMPLETE-MULTI-PATH-MODEL-LEAK) —
# warmup MUST read the SST-resolved test-tier default model from the
# generated env file (no literal `gemma3:4b`; no silent fallback).
# Missing/empty value aborts the test with a named error.
AGENT_PROVIDER_DEFAULT_MODEL="$(smackerel_env_value "$ENV_FILE" "AGENT_PROVIDER_DEFAULT_MODEL")"
if [ -z "$AGENT_PROVIDER_DEFAULT_MODEL" ]; then
  e2e_fail "BS-002: AGENT_PROVIDER_DEFAULT_MODEL missing/empty in $ENV_FILE (spec 061 SCOPE-06a; regenerate via './smackerel.sh --env test config generate')"
fi
echo "--- LLM warmup: priming $AGENT_PROVIDER_DEFAULT_MODEL inference ---"
# Round 66: gemma3:4b is the test default per operator quality directive.
# Use num_predict=64 + max-time 180 so the model is genuinely warm (not just
# loaded in memory) before the live /ask call, which has a 120s budget.
curl -s -o /dev/null --max-time 180 \
  -X POST \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"$AGENT_PROVIDER_DEFAULT_MODEL\",\"prompt\":\"hi\",\"stream\":false,\"options\":{\"num_predict\":64}}" \
  "http://127.0.0.1:47004/api/generate" || true

# --- Drive /ask via the synthetic Telegram webhook. --------------------
PAYLOAD=$(cat <<JSON
{
  "update_id": $UPDATE_ID,
  "message": {
    "message_id": $UPDATE_ID,
    "date": $(date +%s),
    "chat": {"id": $CHAT_ID, "type": "private"},
    "from": {"id": $CHAT_ID, "is_bot": false, "first_name": "BS002"},
    "text": "/ask what did I save about Tailscale last month?"
  }
}
JSON
)

echo "--- ROW-1: webhook POST with /ask text (routes to retrieval_qa via shortcut) ---"
HTTP_STATUS=$(curl -s -o /tmp/bs002_resp.txt -w "%{http_code}" \
  --max-time 180 \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
  -d "$PAYLOAD" \
  "$CORE_URL$WEBHOOK_PATH" || true)
echo "  http_status=$HTTP_STATUS body=$(cat /tmp/bs002_resp.txt)"
if [ "$HTTP_STATUS" != "200" ]; then
  e2e_fail "BS-002: webhook returned $HTTP_STATUS (want 200)"
fi

echo "--- §18.5 slog scrape for assistant_turn with correlation_id=$UPDATE_ID ---"
TURN_LINE=""
# Retrieval path = /api/search (vector + ollama embed) + ml/ synthesize
# (gemma3:4b). Manifest budget is 5s but warm-stack reality on the
# bench can spike past that on first warm call; allow 120s of polling
# to keep the fixture deterministic.
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
  e2e_fail "BS-002: no assistant_turn slog line with correlation_id=$UPDATE_ID within 120s"
fi
echo "  matched: $TURN_LINE"

# Structured assertions via jq.
SCENARIO_ID=$(echo "$TURN_LINE" | jq -r '.scenario_id // ""')
STATUS=$(echo "$TURN_LINE" | jq -r '.status // ""')
ERROR_CAUSE=$(echo "$TURN_LINE" | jq -r '.error_cause // ""')
BODY_REDACTED=$(echo "$TURN_LINE" | jq -r '.body_redacted // "false"')
BAND=$(echo "$TURN_LINE" | jq -r '.band // ""')

if [ "$SCENARIO_ID" != "retrieval_qa" ]; then
  e2e_fail "BS-002: scenario_id='$SCENARIO_ID' (want 'retrieval_qa'); /ask shortcut did not route to retrieval"
fi
# Adversarial: status MUST NOT be saved_as_idea — that would mean the
# provenance gate rewrote the response because Sources was empty
# (i.e. BS-007 path, not BS-002).
if [ "$STATUS" = "saved_as_idea" ]; then
  e2e_fail "BS-002: status='saved_as_idea' on happy path; provenance gate refused (empty Sources?); /ask was routed but assembler produced no citations — LLM probably did not cite the seeded artifact_id=$ARTIFACT_ID"
fi
if [ "$STATUS" != "thinking" ]; then
  e2e_fail "BS-002: status='$STATUS' (want 'thinking' for OutcomeOK retrieval success)"
fi
if [ -n "$ERROR_CAUSE" ] && [ "$ERROR_CAUSE" != "null" ]; then
  e2e_fail "BS-002: error_cause='$ERROR_CAUSE' (want empty on happy path)"
fi
if [ "$BODY_REDACTED" != "true" ]; then
  e2e_fail "BS-002: body_redacted='$BODY_REDACTED' (want true; §18.5 Principle 8 affirmation)"
fi

e2e_pass "BS-002: retrieval happy path - /ask routed to retrieval_qa, correlation_id matched, status=thinking (provenance gate passed → Sources were populated), body_redacted=true, no error"
echo "  scenario_id=$SCENARIO_ID status=$STATUS band='$BAND' error_cause='$ERROR_CAUSE' body_redacted=$BODY_REDACTED"
