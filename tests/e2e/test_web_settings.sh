#!/usr/bin/env bash
# E2E test: Web UI settings and status pages
# Scenarios: SCN-002-035, SCN-002-036
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Settings & Status Pages E2E ==="
e2e_start

# --- SCN-002-035: Settings page ---
echo "Test: Settings page..."
BODY=$(curl -sf --max-time 15 "$CORE_URL/settings" 2>/dev/null || true)
e2e_assert_contains "$BODY" "Settings" "Settings page has title"
e2e_pass "SCN-002-035: Settings page renders"

# --- SCN-002-036: Status page ---
echo "Test: Status page..."
STATUS_URL="${CORE_URL}/ui/status"
BODY=$(curl -sf --max-time 15 "$STATUS_URL" 2>/dev/null || true)
if [ -n "$BODY" ]; then
  e2e_assert_contains "$BODY" "Status" "Status page has title"
  e2e_pass "SCN-002-036: Status page renders"
else
  # Try /status as fallback
  BODY=$(curl -sf --max-time 15 "$CORE_URL/status" 2>/dev/null || true)
  if [ -n "$BODY" ]; then
    e2e_pass "SCN-002-036: Status page renders at /status"
  else
    e2e_fail "SCN-002-036: Status page not accessible"
  fi
fi
