#!/usr/bin/env bash
# Spec 061 SCOPE-05 design §17.5 — BS-001 webhook-driven e2e regression.
#
# This script resolves `SCOPE-05-E2E-INJECTION-MECHANISM` by driving a
# real Telegram-shaped update through the live test stack via the
# webhook endpoint registered in webhook mode (design §17.4). The test
# stack is configured at `./smackerel.sh config generate` time (via the
# TARGET_ENV=test override in scripts/commands/config.sh) to set:
#
#   ASSISTANT_TRANSPORTS_TELEGRAM_MODE=webhook
#   ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF=ASSISTANT_TELEGRAM_WEBHOOK_SECRET
#   ASSISTANT_TELEGRAM_WEBHOOK_SECRET=<test secret>
#   ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH=/v1/telegram/webhook
#
# Adversarial coverage:
#
#   ROW-1 (happy path) POST /v1/telegram/webhook with a valid
#                      X-Telegram-Bot-Api-Secret-Token header carrying
#                      a Telegram-shaped message update → 200 OK; poll
#                      PostgreSQL for the resulting `idea` artifact
#                      whose content_raw equals the verbatim probe.
#
#   ROW-2 (wrong sec)  POST with the WRONG (length-equal, byte-similar)
#                      secret header → 401; subsequent PG poll proves
#                      NO artifact row was created. Catches a future
#                      regression that replaces subtle.ConstantTimeCompare
#                      with a plain `==` or a permissive fallback.
#
#   ROW-3 (no header)  POST with NO header → 401; same no-artifact
#                      assertion. Distinct from ROW-2 to disambiguate
#                      auth_failures{reason="missing"} from reason="mismatch".

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Spec 061 SCOPE-05 §17.5 — BS-001 Telegram webhook e2e ==="
e2e_start

ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
WEBHOOK_SECRET="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TELEGRAM_WEBHOOK_SECRET")"
WEBHOOK_PATH="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH")"
TG_MODE="$(smackerel_env_value "$ENV_FILE" "ASSISTANT_TRANSPORTS_TELEGRAM_MODE")"

if [ "$TG_MODE" != "webhook" ]; then
  e2e_fail "test stack must run with ASSISTANT_TRANSPORTS_TELEGRAM_MODE=webhook (got '$TG_MODE'); see scripts/commands/config.sh TARGET_ENV=test override"
fi
if [ -z "$WEBHOOK_SECRET" ]; then
  e2e_fail "ASSISTANT_TELEGRAM_WEBHOOK_SECRET missing from test env; SST override broken"
fi
if [ -z "$WEBHOOK_PATH" ]; then
  e2e_fail "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH missing from test env"
fi

PROBE_HAPPY="bs001-webhook-probe-$(date +%s)-$RANDOM happy-path-marker"
PROBE_WRONG="bs001-webhook-probe-$(date +%s)-$RANDOM-wrong-secret-marker"
PROBE_NONE="bs001-webhook-probe-$(date +%s)-$RANDOM-no-secret-marker"
CHAT_ID=99001

make_update_json() {
  local probe="$1"
  local update_id="$2"
  cat <<JSON
{
  "update_id": $update_id,
  "message": {
    "message_id": $update_id,
    "date": $(date +%s),
    "chat": {"id": $CHAT_ID, "type": "private"},
    "from": {"id": $CHAT_ID, "is_bot": false, "first_name": "BS001"},
    "text": "$probe"
  }
}
JSON
}

echo
echo "--- ROW-1: webhook POST with valid secret -> 200 + artifact ---"
HAPPY_BODY="$(make_update_json "$PROBE_HAPPY" 900001)"
HAPPY_STATUS=$(curl -s -o /tmp/bs001_happy_resp.txt -w "%{http_code}" \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WEBHOOK_SECRET" \
  -d "$HAPPY_BODY" \
  "$CORE_URL$WEBHOOK_PATH" || true)
echo "  http_status=$HAPPY_STATUS body=$(cat /tmp/bs001_happy_resp.txt)"
if [ "$HAPPY_STATUS" != "200" ]; then
  e2e_fail "ROW-1: webhook with valid secret must return 200, got $HAPPY_STATUS"
fi

SELECT_HAPPY="SELECT content_raw FROM artifacts WHERE content_raw = '$PROBE_HAPPY' LIMIT 1"
CONTENT_RAW=""
# BUG-061-001 — 60s budget (was 15s). The webhook handler dispatches synchronously
# into assistant.Handle, whose first invocation against a cold Ollama can take up
# to ~45s for model load before CaptureRoute/handleTextCapture writes the artifact.
# A genuine dispatch break still produces zero rows for the full window, so widening
# the budget weakens neither the assertion nor the adversarial coverage in ROW-2/ROW-3.
for i in $(seq 1 60); do
  CONTENT_RAW="$(e2e_psql "$SELECT_HAPPY" || true)"
  if [ -n "$CONTENT_RAW" ]; then
    break
  fi
  sleep 1
done
if [ -z "$CONTENT_RAW" ]; then
  e2e_fail "ROW-1: artifact with content_raw='$PROBE_HAPPY' not present in PG after 60s"
fi
NORMALIZED_HAPPY="$(printf '%s' "$PROBE_HAPPY" | tr -d '[:space:]')"
e2e_assert_eq "$CONTENT_RAW" "$NORMALIZED_HAPPY" \
  "ROW-1: content_raw MUST equal the verbatim probe (no LLM rewrite of raw user message)"
e2e_pass "ROW-1: happy path - webhook POST -> idea artifact with verbatim text"

echo
echo "--- ROW-2: webhook POST with WRONG secret -> 401 + zero artifact rows ---"
WRONG_SECRET="${WEBHOOK_SECRET%?}X"
if [ "$WRONG_SECRET" = "$WEBHOOK_SECRET" ]; then
  e2e_fail "test bug: wrong-secret derivation failed (last char already X)"
fi
WRONG_BODY="$(make_update_json "$PROBE_WRONG" 900002)"
WRONG_STATUS=$(curl -s -o /tmp/bs001_wrong_resp.txt -w "%{http_code}" \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Telegram-Bot-Api-Secret-Token: $WRONG_SECRET" \
  -d "$WRONG_BODY" \
  "$CORE_URL$WEBHOOK_PATH" || true)
echo "  http_status=$WRONG_STATUS body=$(cat /tmp/bs001_wrong_resp.txt)"
if [ "$WRONG_STATUS" != "401" ]; then
  e2e_fail "ROW-2: webhook with WRONG secret must return 401, got $WRONG_STATUS"
fi
sleep 2
SELECT_WRONG="SELECT count(*) FROM artifacts WHERE content_raw = '$PROBE_WRONG'"
COUNT_WRONG="$(e2e_psql "$SELECT_WRONG" | tr -d '[:space:]')"
if [ "$COUNT_WRONG" != "0" ]; then
  e2e_fail "ROW-2: 401 path leaked an artifact (count=$COUNT_WRONG); secret-mismatch MUST short-circuit before dispatch"
fi
e2e_pass "ROW-2: adversarial wrong-secret refused with 401 and zero artifact rows"

echo
echo "--- ROW-3: webhook POST with NO secret header -> 401 + zero artifact rows ---"
NONE_BODY="$(make_update_json "$PROBE_NONE" 900003)"
NONE_STATUS=$(curl -s -o /tmp/bs001_nosec_resp.txt -w "%{http_code}" \
  -X POST \
  -H "Content-Type: application/json" \
  -d "$NONE_BODY" \
  "$CORE_URL$WEBHOOK_PATH" || true)
echo "  http_status=$NONE_STATUS body=$(cat /tmp/bs001_nosec_resp.txt)"
if [ "$NONE_STATUS" != "401" ]; then
  e2e_fail "ROW-3: webhook with NO secret header must return 401, got $NONE_STATUS"
fi
sleep 2
SELECT_NONE="SELECT count(*) FROM artifacts WHERE content_raw = '$PROBE_NONE'"
COUNT_NONE="$(e2e_psql "$SELECT_NONE" | tr -d '[:space:]')"
if [ "$COUNT_NONE" != "0" ]; then
  e2e_fail "ROW-3: missing-header path leaked an artifact (count=$COUNT_NONE)"
fi
e2e_pass "ROW-3: missing-header POST refused with 401 and zero artifact rows"

echo
e2e_pass "Spec 061 SCOPE-05 §17.5 BS-001: webhook injection mechanism live-stack-green; SCOPE-05-E2E-INJECTION-MECHANISM resolved"
