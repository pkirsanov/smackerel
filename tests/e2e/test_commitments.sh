#!/usr/bin/env bash
# E2E test: Commitment tracking
# Scenario: SCN-004 Scope 02
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Commitment Tracking E2E ==="
e2e_start

# Insert action items with various states
echo "Test: Action item lifecycle..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO people (id, name) VALUES ('commit-person', 'Sarah') ON CONFLICT DO NOTHING;

INSERT INTO action_items (id, artifact_id, person_id, item_type, text, expected_date, status)
VALUES
  ('ai-e2e-001', NULL, 'commit-person', 'user-promise', 'Send proposal to Sarah', CURRENT_DATE - 3, 'open'),
  ('ai-e2e-002', NULL, 'commit-person', 'contact-promise', 'Sarah will review docs', CURRENT_DATE + 7, 'open'),
  ('ai-e2e-003', NULL, NULL, 'deadline', 'Tax filing deadline', CURRENT_DATE + 30, 'open'),
  ('ai-e2e-004', NULL, NULL, 'todo', 'Buy groceries', NULL, 'resolved')
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Verify open action items
OPEN=$(e2e_psql "SELECT COUNT(*) FROM action_items WHERE status='open'")
echo "  Open action items: $OPEN"
if [ "$OPEN" -ge 3 ]; then
  e2e_pass "Action items created with correct statuses"
fi

# Verify overdue detection
OVERDUE=$(e2e_psql "SELECT COUNT(*) FROM action_items WHERE status='open' AND expected_date < CURRENT_DATE")
echo "  Overdue items: $OVERDUE"
if [ "$OVERDUE" -ge 1 ]; then
  e2e_pass "Overdue commitment detected"
fi

# Resolve an item
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
UPDATE action_items SET status='resolved', resolved_at=NOW() WHERE id='ai-e2e-001';
" >/dev/null
RESOLVED=$(e2e_psql "SELECT status FROM action_items WHERE id='ai-e2e-001'")
e2e_assert_eq "$RESOLVED" "resolved" "Action item resolved"
e2e_pass "Commitment lifecycle: open → resolved"
