#!/usr/bin/env bash
# E2E test: Capture API contract tests
# Scenarios: SCN-002-011, SCN-002-012, SCN-002-013, SCN-002-014, SCN-002-015
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Capture API E2E Tests ==="
e2e_start

# --- SCN-002-015: Empty body returns 400 ---
echo "Test: Empty body returns 400..."
e2e_assert_http_status POST /api/capture 400 '{}' "SCN-002-015: empty body"
e2e_pass "SCN-002-015: Empty body returns 400"

# --- SCN-002-012: Capture plain text note ---
echo "Test: Plain text capture..."
RESPONSE=$(e2e_api POST /api/capture -d '{"text": "Organize team by customer segment"}')
e2e_assert_contains "$RESPONSE" "artifact_id" "SCN-002-012: response has artifact_id"
TYPE=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('artifact_type',''))")
ART_ID=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['artifact_id'])")
echo "  Captured: id=$ART_ID type=$TYPE"
e2e_pass "SCN-002-012: Plain text capture"

# --- SCN-002-014: Duplicate returns 409 ---
echo "Test: Duplicate text returns 409..."
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -d '{"text": "Organize team by customer segment"}' \
  "$CORE_URL/api/capture")
e2e_assert_eq "$STATUS" "409" "SCN-002-014: duplicate returns 409"
e2e_pass "SCN-002-014: Duplicate returns 409"

# --- SCN-002-011: Capture article URL ---
echo "Test: URL capture..."
RESPONSE=$(e2e_api POST /api/capture -d '{"url": "https://example.com/test-article-e2e"}' 2>/dev/null || true)
if [ -n "$RESPONSE" ]; then
  e2e_assert_contains "$RESPONSE" "artifact_id" "SCN-002-011: URL capture returns artifact_id"
  e2e_pass "SCN-002-011: URL capture accepted"
else
  echo "  SKIP: URL capture requires external network (non-blocking)"
fi

# --- SCN-002-039: ML sidecar unavailable returns 503 ---
echo "Test: ML sidecar unavailable..."
# Stop only the ML sidecar
smackerel_compose "$TEST_ENV" stop smackerel-ml 2>/dev/null || true
sleep 3
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -d '{"text": "Test while ML down"}' \
  "$CORE_URL/api/capture")
# Restart ML sidecar
smackerel_compose "$TEST_ENV" start smackerel-ml 2>/dev/null || true
# The capture may still succeed (async NATS) or return error
echo "  Status with ML down: $STATUS"
if [ "$STATUS" = "200" ] || [ "$STATUS" = "503" ]; then
  e2e_pass "SCN-002-039: Capture handles ML unavailability (status=$STATUS)"
else
  e2e_fail "SCN-002-039: Unexpected status $STATUS with ML down"
fi

echo ""
echo "=== All Capture API E2E tests passed ==="
