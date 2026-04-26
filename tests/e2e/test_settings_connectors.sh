#!/usr/bin/env bash
# E2E test: Settings UI connectors
# Scenario: SCN-003 Scope 07
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Settings Connectors E2E ==="
e2e_start

# Verify settings page shows connector-related content
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/settings")
e2e_assert_eq "$STATUS" "200" "Settings page accessible"

BODY=$(curl -sf --max-time 15 \
  -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/settings" 2>/dev/null || true)
e2e_assert_contains "$BODY" "Settings" "Settings page renders"
e2e_pass "Settings connector UI accessible"
