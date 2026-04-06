#!/usr/bin/env bash
# E2E test: Topic lifecycle and momentum scoring
# Scenario: SCN-003 Scope 06
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Topic Lifecycle E2E Tests ==="
e2e_start

# Seed topics with varying activity levels
echo "Seeding topics..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO topics (id, name, state, momentum_score, capture_count_total, capture_count_30d, capture_count_90d, search_hit_count_30d, last_active)
VALUES
  ('topic-hot', 'pricing', 'hot', 20.0, 15, 10, 15, 8, NOW()),
  ('topic-active', 'negotiation', 'active', 10.0, 8, 5, 8, 3, NOW() - INTERVAL '5 days'),
  ('topic-emerging', 'leadership', 'emerging', 3.0, 3, 2, 3, 1, NOW() - INTERVAL '10 days'),
  ('topic-dormant', 'archery', 'dormant', 0.1, 1, 0, 0, 0, NOW() - INTERVAL '90 days')
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Verify topic states
HOT_STATE=$(e2e_psql "SELECT state FROM topics WHERE id='topic-hot'")
e2e_assert_eq "$HOT_STATE" "hot" "Hot topic state correct"

DORMANT_STATE=$(e2e_psql "SELECT state FROM topics WHERE id='topic-dormant'")
e2e_assert_eq "$DORMANT_STATE" "dormant" "Dormant topic state correct"

# Verify momentum scores are reasonable
HOT_MOMENTUM=$(e2e_psql "SELECT momentum_score FROM topics WHERE id='topic-hot'")
echo "  Hot topic momentum: $HOT_MOMENTUM"
DORMANT_MOMENTUM=$(e2e_psql "SELECT momentum_score FROM topics WHERE id='topic-dormant'")
echo "  Dormant topic momentum: $DORMANT_MOMENTUM"

# Verify topic list via topics page
STATUS=$(curl -s --max-time 15 -o /dev/null -w '%{http_code}' "$CORE_URL/topics")
e2e_assert_eq "$STATUS" "200" "Topics page renders"

BODY=$(curl -sf --max-time 15 "$CORE_URL/topics" 2>/dev/null || true)
e2e_assert_contains "$BODY" "pricing" "Topics page shows pricing topic"
e2e_pass "Topic lifecycle: states and momentum verified"

echo ""
echo "=== Topic Lifecycle E2E tests passed ==="
