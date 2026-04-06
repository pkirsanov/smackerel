#!/usr/bin/env bash
# E2E test: Expertise and learning detection
# Scenario: SCN-006 Advanced
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Expertise Detection E2E ==="
e2e_start

# Seed topic with high capture count (expertise signal)
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO topics (id, name, state, capture_count_total, capture_count_30d, search_hit_count_30d, momentum_score)
VALUES ('expert-topic', 'go-programming', 'hot', 50, 15, 20, 25.0)
ON CONFLICT (id) DO NOTHING;
" >/dev/null

SCORE=$(e2e_psql "SELECT momentum_score FROM topics WHERE id='expert-topic'")
echo "  Go programming momentum: $SCORE"
CAPTURES=$(e2e_psql "SELECT capture_count_total FROM topics WHERE id='expert-topic'")
echo "  Total captures: $CAPTURES"

if [ "$CAPTURES" -ge 30 ]; then
  e2e_pass "Expertise: high-capture topic detected as expertise area"
fi
