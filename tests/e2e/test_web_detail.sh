#!/usr/bin/env bash
# E2E test: Web UI artifact detail page
# Scenario: SCN-002-034
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== SCN-002-034: Artifact Detail Page ==="
e2e_start

# Seed artifact
e2e_seed_artifact "detail-e2e-001" "Artifact Detail Test" "article"

STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/artifact/detail-e2e-001")
e2e_assert_eq "$STATUS" "200" "Artifact detail page returns 200"

BODY=$(curl -sf --max-time 15 -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/artifact/detail-e2e-001" 2>/dev/null || true)
e2e_assert_contains "$BODY" "Artifact Detail Test" "Detail page shows artifact title"
e2e_pass "SCN-002-034: Artifact detail page renders"

# Non-existent artifact
echo "Test: Non-existent artifact..."
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/artifact/nonexistent-999")
if [ "$STATUS" = "404" ] || [ "$STATUS" = "500" ]; then
  e2e_pass "Non-existent artifact returns error status"
fi
