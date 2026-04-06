#!/usr/bin/env bash
# E2E test: Alerts system
# Scenario: SCN-004 Scope 04
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Alerts E2E Tests ==="
e2e_start

# Verify alerts table exists
EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.tables WHERE table_name='alerts'")
e2e_assert_eq "$EXISTS" "1" "Alerts table exists"

# Insert test alerts
echo "Test: Insert and query alerts..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO alerts (id, alert_type, title, body, priority, status, created_at)
VALUES
  ('alert-e2e-001', 'commitment_overdue', 'Reply to Sarah', 'Email proposal waiting 3 days', 1, 'pending', NOW()),
  ('alert-e2e-002', 'bill', 'AWS Invoice', 'Monthly AWS bill due', 2, 'pending', NOW()),
  ('alert-e2e-003', 'trip_prep', 'Pack for Tokyo', 'Flight in 5 days', 2, 'delivered', NOW())
ON CONFLICT (id) DO NOTHING;
" >/dev/null

PENDING=$(e2e_psql "SELECT COUNT(*) FROM alerts WHERE status='pending'")
echo "  Pending alerts: $PENDING"
if [ "$PENDING" -ge 2 ]; then
  e2e_pass "Alert creation and querying works"
else
  e2e_fail "Expected >=2 pending alerts, got $PENDING"
fi

# Dismiss an alert
echo "Test: Dismiss alert..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
UPDATE alerts SET status='dismissed' WHERE id='alert-e2e-001';
" >/dev/null
DISMISSED=$(e2e_psql "SELECT status FROM alerts WHERE id='alert-e2e-001'")
e2e_assert_eq "$DISMISSED" "dismissed" "Alert dismissed"
e2e_pass "Alert lifecycle: pending → dismissed"

# Snooze an alert
echo "Test: Snooze alert..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
UPDATE alerts SET status='snoozed', snooze_until=NOW() + INTERVAL '1 day' WHERE id='alert-e2e-002';
" >/dev/null
SNOOZED=$(e2e_psql "SELECT status FROM alerts WHERE id='alert-e2e-002'")
e2e_assert_eq "$SNOOZED" "snoozed" "Alert snoozed"
e2e_pass "Alert lifecycle: pending → snoozed"

echo ""
echo "=== Alerts E2E passed ==="
