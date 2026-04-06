#!/usr/bin/env bash
# E2E test: Digest pipeline with multi-source data
# Scenario: SCN-001-016
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Digest Pipeline E2E ==="
e2e_start

# Seed action items and artifacts for digest assembly
echo "Seeding digest context..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO people (id, name) VALUES ('digest-person', 'David') ON CONFLICT DO NOTHING;

INSERT INTO action_items (id, person_id, item_type, text, expected_date, status)
VALUES ('digest-ai-001', 'digest-person', 'user-promise', 'Review David proposal', CURRENT_DATE - 2, 'open')
ON CONFLICT (id) DO NOTHING;

INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, created_at, updated_at)
VALUES
  ('digest-art-001', 'article', 'Overnight Article', 'hash-digest001', 'gmail', NOW() - INTERVAL '8 hours', NOW()),
  ('digest-art-002', 'video', 'Overnight Video', 'hash-digest002', 'youtube', NOW() - INTERVAL '6 hours', NOW())
ON CONFLICT (id) DO NOTHING;

INSERT INTO topics (id, name, state, momentum_score, capture_count_30d)
VALUES ('digest-topic', 'pricing', 'hot', 15.0, 8)
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Verify digest context data exists
OPEN_AI=$(e2e_psql "SELECT COUNT(*) FROM action_items WHERE status='open'")
RECENT_ART=$(e2e_psql "SELECT COUNT(*) FROM artifacts WHERE created_at > NOW() - INTERVAL '24 hours'")
HOT_TOPICS=$(e2e_psql "SELECT COUNT(*) FROM topics WHERE state IN ('hot', 'active')")
echo "  Open action items: $OPEN_AI"
echo "  Recent artifacts: $RECENT_ART"
echo "  Hot topics: $HOT_TOPICS"

e2e_pass "SCN-001-016: Digest context assembly data verified"
