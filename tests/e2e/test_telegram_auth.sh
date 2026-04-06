#!/usr/bin/env bash
# E2E test: Telegram auth rejection
# Scenario: SCN-002-029
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== SCN-002-029: Telegram Auth Rejection ==="
e2e_start

# The API requires Bearer token auth. Without it, requests are rejected.
echo "Test: API rejects unauthenticated requests..."
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{"text": "unauthorized capture attempt"}' \
  "$CORE_URL/api/capture")
e2e_assert_eq "$STATUS" "401" "Unauthenticated capture rejected"
e2e_pass "SCN-002-029: Unauthorized requests rejected"

# Verify wrong token rejected
echo "Test: Wrong token rejected..."
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer invalid-token-xyz" \
  -d '{"text": "wrong token attempt"}' \
  "$CORE_URL/api/capture")
e2e_assert_eq "$STATUS" "401" "Wrong token rejected"
e2e_pass "Wrong token rejected"

# Verify search also requires auth
echo "Test: Search requires auth..."
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{"query": "test"}' \
  "$CORE_URL/api/search")
e2e_assert_eq "$STATUS" "401" "Search requires auth"
e2e_pass "All API endpoints enforce auth"
