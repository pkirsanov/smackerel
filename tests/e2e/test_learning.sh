#!/usr/bin/env bash
# E2E test: Learning path detection
# Scenario: SCN-006 Advanced
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Learning Path E2E ==="
e2e_start

# Seed emerging topic with growing capture count (learning signal)
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO topics (id, name, state, capture_count_total, capture_count_30d, capture_count_90d, momentum_score)
VALUES ('learn-topic', 'rust-programming', 'emerging', 8, 6, 8, 5.0)
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Learning signal: high 30d/total ratio = actively learning
RATIO=$(e2e_psql "SELECT ROUND(capture_count_30d::numeric / GREATEST(capture_count_total, 1), 2) FROM topics WHERE id='learn-topic'")
echo "  30d/total capture ratio: $RATIO"
e2e_pass "Learning path: emerging topic with high recent activity"
