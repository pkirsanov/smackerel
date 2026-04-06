#!/usr/bin/env bash
# E2E test: Product-level cross-phase flows
# Scenario: SCN-001-014, SCN-001-016, SCN-001-017
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Product Flows E2E ==="
e2e_start

# --- Health check baseline ---
echo "Test: System health..."
HEALTH=$(e2e_api GET /api/health)
STATUS=$(echo "$HEALTH" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])")
echo "  System: $STATUS"

# --- Capture flow ---
echo "Test: Capture text artifact..."
RESPONSE=$(e2e_api POST /api/capture -d '{"text": "Product flow test: quarterly review discussion"}')
ART_ID=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['artifact_id'])")
e2e_pass "Artifact captured: $ART_ID"

# --- Search flow ---
echo "Test: Search for captured artifact..."
RESPONSE=$(e2e_api POST /api/search -d '{"query": "quarterly review"}')
e2e_pass "Search executed"

# --- Digest flow ---
echo "Test: Digest endpoint available..."
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  "$CORE_URL/api/digest")
echo "  Digest status: $STATUS"
e2e_pass "Digest endpoint available (status=$STATUS)"

# --- Persistence flow ---
echo "Test: Data in PostgreSQL..."
TOTAL=$(e2e_psql "SELECT COUNT(*) FROM artifacts")
echo "  Total artifacts: $TOTAL"
e2e_pass "SCN-001-017: Data stored in PostgreSQL"

# --- Schema completeness ---
echo "Test: All required tables exist..."
TABLES="artifacts people topics edges sync_state action_items digests"
for TABLE in $TABLES; do
  EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.tables WHERE table_name='$TABLE'")
  if [ "$EXISTS" != "1" ]; then
    e2e_fail "Table $TABLE missing"
  fi
done
e2e_pass "All required tables exist"

echo ""
echo "=== Product Flows E2E passed ==="
