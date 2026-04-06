#!/usr/bin/env bash
# E2E test: Telegram output uses text markers, no emoji
# Scenario: SCN-001-004
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== SCN-001-004: Telegram Format E2E ==="
e2e_start

# The Telegram bot formats responses using text markers (. ? ! > - ~ # @).
# Since we can't interact with the actual Telegram API in E2E, we verify that
# the capture API responses don't contain emoji (the bot passes these through).

echo "Test: API responses contain no emoji..."
RESPONSE=$(e2e_api POST /api/capture -d '{"text": "Format test for emoji detection"}')
if echo "$RESPONSE" | python3 -c "
import sys
text = sys.stdin.read()
for c in text:
    if ord(c) >= 0x1F600 and ord(c) <= 0x1F9FF:
        sys.exit(1)
" 2>/dev/null; then
  e2e_pass "SCN-001-004: API response contains no emoji"
else
  e2e_fail "SCN-001-004: Emoji found in API response"
fi

# Verify health response also has no emoji
HEALTH=$(e2e_api GET /api/health)
if echo "$HEALTH" | python3 -c "
import sys
text = sys.stdin.read()
for c in text:
    if ord(c) >= 0x1F600 and ord(c) <= 0x1F9FF:
        sys.exit(1)
" 2>/dev/null; then
  e2e_pass "Health response contains no emoji"
else
  e2e_fail "Emoji found in health response"
fi
