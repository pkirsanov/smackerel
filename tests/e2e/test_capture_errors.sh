#!/usr/bin/env bash
# E2E test: Capture API error responses
# Scenarios: SCN-002-014, SCN-002-015, SCN-002-039
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Capture Error Responses E2E Tests ==="
e2e_start

# --- Invalid JSON ---
echo "Test: Invalid JSON body..."
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -d 'not-json' \
  "$CORE_URL/api/capture")
e2e_assert_eq "$STATUS" "400" "Invalid JSON returns 400"
e2e_pass "Invalid JSON returns 400"

# --- No auth header ---
echo "Test: Missing auth header..."
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{"text": "test"}' \
  "$CORE_URL/api/capture")
e2e_assert_eq "$STATUS" "401" "Missing auth returns 401"
e2e_pass "Missing auth returns 401"

# --- Wrong auth token ---
echo "Test: Wrong auth token..."
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer wrong-token-123" \
  -d '{"text": "test"}' \
  "$CORE_URL/api/capture")
e2e_assert_eq "$STATUS" "401" "Wrong auth returns 401"
e2e_pass "Wrong auth returns 401"

# --- Empty body (no fields) ---
echo "Test: Empty body..."
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -d '{}' \
  "$CORE_URL/api/capture")
e2e_assert_eq "$STATUS" "400" "Empty body returns 400"

RESPONSE=$(curl -sf --max-time 15 \
  -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -d '{}' \
  "$CORE_URL/api/capture" 2>/dev/null || true)
e2e_assert_contains "${RESPONSE:-}" "INVALID_INPUT" "Error code is INVALID_INPUT"
e2e_pass "Empty body returns 400 with INVALID_INPUT"

# --- Duplicate detection ---
echo "Test: Duplicate detection..."
e2e_api POST /api/capture -d '{"text": "Unique text for dedup test e2e"}' >/dev/null 2>&1 || true
DUP_STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -d '{"text": "Unique text for dedup test e2e"}' \
  "$CORE_URL/api/capture")
e2e_assert_eq "$DUP_STATUS" "409" "Duplicate returns 409"

DUP_RESPONSE=$(curl -sf --max-time 15 \
  -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -d '{"text": "Unique text for dedup test e2e"}' \
  "$CORE_URL/api/capture" 2>/dev/null || true)
e2e_assert_contains "${DUP_RESPONSE:-}" "DUPLICATE_DETECTED" "Error code is DUPLICATE_DETECTED"
e2e_pass "Duplicate detection returns 409 with DUPLICATE_DETECTED"

echo ""
echo "=== All Capture Error E2E tests passed ==="
