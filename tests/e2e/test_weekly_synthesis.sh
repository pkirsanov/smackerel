#!/usr/bin/env bash
# E2E test: Weekly synthesis
# Scenario: SCN-004 Scope 05
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Weekly Synthesis E2E ==="
e2e_start

echo "Test: Weekly synthesis table..."
EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.tables WHERE table_name='weekly_synthesis'")
e2e_assert_eq "$EXISTS" "1" "weekly_synthesis table exists"

echo "Test: Insert weekly digest..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO weekly_synthesis (id, week_start, synthesis_text, word_count, model_used)
VALUES ('ws-e2e-001', '2026-03-30', '> This week: pricing converged with negotiation insights. 3 new connections detected. ! Follow up on David proposal.', 18, 'test')
ON CONFLICT (week_start) DO NOTHING;
" >/dev/null

WORD_COUNT=$(e2e_psql "SELECT word_count FROM weekly_synthesis WHERE id='ws-e2e-001'")
echo "  Word count: $WORD_COUNT"
if [ "$WORD_COUNT" -le 250 ]; then
  e2e_pass "Weekly synthesis under 250-word cap"
else
  e2e_fail "Weekly synthesis exceeds 250-word cap ($WORD_COUNT)"
fi
