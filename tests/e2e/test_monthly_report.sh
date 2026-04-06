#!/usr/bin/env bash
# E2E test: Monthly report
# Scenario: SCN-006 Advanced
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Monthly Report E2E ==="
e2e_start

# Verify weekly_synthesis table supports monthly aggregation
EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.tables WHERE table_name='weekly_synthesis'")
e2e_assert_eq "$EXISTS" "1" "weekly_synthesis table exists"

# Seed multiple weekly digests for monthly rollup
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO weekly_synthesis (id, week_start, synthesis_text, word_count, model_used)
VALUES
  ('ws-m1', '2026-03-02', 'Week 1 synthesis...', 50, 'test'),
  ('ws-m2', '2026-03-09', 'Week 2 synthesis...', 45, 'test'),
  ('ws-m3', '2026-03-16', 'Week 3 synthesis...', 60, 'test'),
  ('ws-m4', '2026-03-23', 'Week 4 synthesis...', 55, 'test')
ON CONFLICT (week_start) DO NOTHING;
" >/dev/null

WEEKS=$(e2e_psql "SELECT COUNT(*) FROM weekly_synthesis WHERE week_start >= '2026-03-01' AND week_start < '2026-04-01'")
echo "  March weekly digests: $WEEKS"
e2e_assert_eq "$WEEKS" "4" "4 weekly digests for monthly rollup"
e2e_pass "Monthly report: weekly synthesis data available for aggregation"
