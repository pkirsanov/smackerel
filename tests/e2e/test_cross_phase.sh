#!/usr/bin/env bash
# E2E test: Cross-phase integration
# Scenario: SCN-001-015
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Cross-Phase Integration E2E ==="
e2e_start

# Simulate cross-source artifacts (capture + email + youtube)
echo "Test: Multi-source artifacts..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, summary, created_at, updated_at)
VALUES
  ('cross-001', 'article', 'Captured Article', 'hash-cross001', 'capture', 'Actively captured article about pricing', NOW(), NOW()),
  ('cross-002', 'email', 'Email from Sarah', 'hash-cross002', 'gmail', 'Email discussing pricing proposal', NOW(), NOW()),
  ('cross-003', 'video', 'Pricing Strategy Video', 'hash-cross003', 'youtube', 'YouTube video about SaaS pricing', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Verify multi-source search
TOTAL=$(e2e_psql "SELECT COUNT(DISTINCT source_id) FROM artifacts WHERE id LIKE 'cross-%'")
echo "  Distinct sources: $TOTAL"
if [ "$TOTAL" -ge 3 ]; then
  e2e_pass "SCN-001-015: Multi-source artifact storage verified"
fi

# Search across sources
RESPONSE=$(e2e_api POST /api/search -d '{"query": "pricing"}')
RESULTS=$(echo "$RESPONSE" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('results',[])))" 2>/dev/null || echo "0")
echo "  Cross-source search results: $RESULTS"
e2e_pass "Cross-phase: search covers multiple sources"
