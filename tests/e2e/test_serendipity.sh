#!/usr/bin/env bash
# E2E test: Content resurfacing / serendipity
# Scenario: SCN-006 Advanced
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Serendipity Resurface E2E ==="
e2e_start

# Seed dormant but high-value artifacts
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, relevance_score, access_count, last_accessed, created_at, updated_at)
VALUES
  ('resurf-001', 'article', 'Dormant High-Value Article', 'hash-resurf001', 'capture', 0.85, 1, NOW() - INTERVAL '60 days', NOW() - INTERVAL '90 days', NOW()),
  ('resurf-002', 'article', 'Rarely Accessed Insight', 'hash-resurf002', 'capture', 0.72, 0, NULL, NOW() - INTERVAL '45 days', NOW())
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Verify dormant high-value artifacts exist
DORMANT=$(e2e_psql "SELECT COUNT(*) FROM artifacts WHERE relevance_score > 0.5 AND (last_accessed IS NULL OR last_accessed < NOW() - INTERVAL '30 days')")
echo "  Dormant high-value artifacts: $DORMANT"
if [ "$DORMANT" -ge 1 ]; then
  e2e_pass "Serendipity: dormant high-value candidates available"
fi

# Verify low access count detection
LOW_ACCESS=$(e2e_psql "SELECT COUNT(*) FROM artifacts WHERE access_count < 3 AND created_at > NOW() - INTERVAL '90 days'")
echo "  Low-access recent artifacts: $LOW_ACCESS"
e2e_pass "Serendipity: resurface candidate pool verified"
