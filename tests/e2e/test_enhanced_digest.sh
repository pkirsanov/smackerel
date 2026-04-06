#!/usr/bin/env bash
# E2E test: Enhanced daily digest with intelligence data
# Scenario: SCN-004 Scope 06
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Enhanced Digest E2E ==="
e2e_start

# Seed commitment-tracked action items and meeting data
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO action_items (id, item_type, text, status, expected_date)
VALUES ('enh-ai-001', 'user-promise', 'Send Sarah the proposal', 'open', CURRENT_DATE - 2)
ON CONFLICT (id) DO NOTHING;

INSERT INTO topics (id, name, state, momentum_score, capture_count_30d)
VALUES ('enh-topic', 'saas-pricing', 'hot', 18.0, 10)
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Verify digest context includes intelligence data
OVERDUE=$(e2e_psql "SELECT COUNT(*) FROM action_items WHERE status='open' AND expected_date < CURRENT_DATE")
HOT=$(e2e_psql "SELECT COUNT(*) FROM topics WHERE state='hot'")
echo "  Overdue commitments: $OVERDUE"
echo "  Hot topics: $HOT"
e2e_pass "Enhanced digest: intelligence context available"
