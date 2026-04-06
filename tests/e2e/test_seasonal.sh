#!/usr/bin/env bash
# E2E test: Seasonal/temporal relevance
# Scenario: SCN-006 Advanced
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Seasonal Relevance E2E ==="
e2e_start

# Seed artifact with temporal relevance metadata
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, temporal_relevance, created_at, updated_at)
VALUES ('season-001', 'article', 'Summer BBQ Recipes', 'hash-season001', 'capture',
  '{\"relevant_from\": \"2026-06-01\", \"relevant_until\": \"2026-09-30\"}',
  NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Verify temporal_relevance JSONB stored correctly
HAS_TEMPORAL=$(e2e_psql "SELECT temporal_relevance IS NOT NULL FROM artifacts WHERE id='season-001'")
e2e_assert_eq "$HAS_TEMPORAL" "t" "Temporal relevance JSONB stored"

RELEVANT_FROM=$(e2e_psql "SELECT temporal_relevance->>'relevant_from' FROM artifacts WHERE id='season-001'")
echo "  Relevant from: $RELEVANT_FROM"
e2e_pass "Seasonal relevance: temporal JSONB metadata stored"
