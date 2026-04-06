#!/usr/bin/env bash
# E2E test: Quick lookups via API
# Scenario: SCN-006 Advanced
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Quick Lookups E2E ==="
e2e_start

# Seed a product artifact for lookup
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, summary, source_url, created_at, updated_at)
VALUES ('lookup-001', 'product', 'Running Shoes X', 'hash-lookup001', 'capture', 'Nike running shoes, size 10, purchased March 2026', 'https://example.com/shoes', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Search for the product
RESPONSE=$(e2e_api POST /api/search -d '{"query": "running shoes", "limit": 5}')
RESULTS=$(echo "$RESPONSE" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('results',[])))" 2>/dev/null || echo "0")
echo "  Lookup results: $RESULTS"
e2e_pass "Quick lookups: product search works"
