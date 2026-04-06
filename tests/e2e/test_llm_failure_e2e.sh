#!/usr/bin/env bash
# E2E test: LLM failure resilience
# Scenario: SCN-002-038
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== SCN-002-038: LLM Failure Resilience ==="
e2e_start

# Capture an artifact — it will go to NATS for ML processing
echo "Test: Capture with potentially unavailable LLM..."
RESPONSE=$(e2e_api POST /api/capture -d '{"text": "LLM resilience test content for e2e verification"}')
ART_ID=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['artifact_id'])")
echo "  Artifact: $ART_ID"

# Verify artifact was stored even if LLM processing fails
COUNT=$(e2e_psql "SELECT COUNT(*) FROM artifacts WHERE id='$ART_ID'")
e2e_assert_eq "$COUNT" "1" "Artifact stored despite potential LLM failure"

# Verify the system remains healthy after processing attempt
sleep 5
HEALTH=$(e2e_api GET /api/health)
STATUS=$(echo "$HEALTH" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])")
echo "  System health: $STATUS"

if [ "$STATUS" = "healthy" ] || [ "$STATUS" = "degraded" ]; then
  e2e_pass "SCN-002-038: System remains healthy after LLM processing attempt"
else
  e2e_fail "SCN-002-038: System unhealthy after LLM processing ($STATUS)"
fi

# Verify no partial data — either fully processed or metadata-only
TIER=$(e2e_psql "SELECT processing_tier FROM artifacts WHERE id='$ART_ID'")
echo "  Processing tier: $TIER"
if [ "$TIER" = "full" ] || [ "$TIER" = "standard" ] || [ "$TIER" = "metadata" ]; then
  e2e_pass "SCN-002-038: Artifact has valid processing tier ($TIER)"
else
  e2e_fail "SCN-002-038: Unexpected processing tier: $TIER"
fi
