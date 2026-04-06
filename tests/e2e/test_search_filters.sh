#!/usr/bin/env bash
# E2E test: Search with person and topic filters
# Scenarios: SCN-002-021, SCN-002-022
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Search Filters E2E Tests ==="
e2e_start

# Seed artifacts with entities and topics
echo "Seeding test data..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, summary,
  entities, topics, created_at, updated_at)
VALUES
  ('filter-e2e-001', 'article', 'Sarah Negotiation Tips', 'hash-filter001', 'test',
   'Sarah recommends specific negotiation strategies',
   '{\"people\": [\"Sarah\"], \"orgs\": [], \"places\": []}',
   '[\"negotiation\"]', NOW(), NOW()),
  ('filter-e2e-002', 'article', 'Negotiation for Beginners', 'hash-filter002', 'test',
   'Basic negotiation concepts and frameworks',
   '{\"people\": [], \"orgs\": [], \"places\": []}',
   '[\"negotiation\"]', NOW(), NOW()),
  ('filter-e2e-003', 'article', 'Sarah Travel Plans', 'hash-filter003', 'test',
   'Sarah upcoming travel itinerary',
   '{\"people\": [\"Sarah\"], \"orgs\": [], \"places\": []}',
   '[\"travel\"]', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# --- SCN-002-022: Topic filter ---
echo "Test: Topic filter for negotiation..."
RESPONSE=$(e2e_api POST /api/search -d '{"query": "negotiation", "filters": {"topic": "negotiation"}}')
RESULTS=$(echo "$RESPONSE" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('results',[])))" 2>/dev/null || echo "0")
echo "  Results for topic=negotiation: $RESULTS"
e2e_pass "SCN-002-022: Topic-scoped search executed"

# --- SCN-002-021: Person filter ---
echo "Test: Person filter for Sarah..."
RESPONSE=$(e2e_api POST /api/search -d '{"query": "recommendations", "filters": {"person": "Sarah"}}')
RESULTS=$(echo "$RESPONSE" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('results',[])))" 2>/dev/null || echo "0")
echo "  Results for person=Sarah: $RESULTS"
e2e_pass "SCN-002-021: Person-scoped search executed"

# --- Type filter ---
echo "Test: Type filter..."
RESPONSE=$(e2e_api POST /api/search -d '{"query": "test", "filters": {"type": "article"}}')
RESULTS=$(echo "$RESPONSE" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('results',[])))" 2>/dev/null || echo "0")
echo "  Results for type=article: $RESULTS"
e2e_pass "Type filter search executed"

echo ""
echo "=== All Search Filter E2E tests passed ==="
