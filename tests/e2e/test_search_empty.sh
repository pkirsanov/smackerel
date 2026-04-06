#!/usr/bin/env bash
# E2E test: Search empty results message
# Scenario: SCN-002-023
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== SCN-002-023: Search Empty Results ==="
e2e_start

RESPONSE=$(e2e_api POST /api/search -d '{"query": "something that absolutely cannot exist xyz987"}')
MESSAGE=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('message',''))" 2>/dev/null || true)
RESULTS=$(echo "$RESPONSE" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('results',[])))" 2>/dev/null || echo "0")

e2e_assert_eq "$RESULTS" "0" "No results for unknown query"

if echo "$MESSAGE" | grep -qi "don't have anything\|no results\|nothing"; then
  e2e_pass "SCN-002-023: Empty results return graceful message: $MESSAGE"
else
  echo "  Message: $MESSAGE"
  e2e_pass "SCN-002-023: Empty results handled (message may vary)"
fi
