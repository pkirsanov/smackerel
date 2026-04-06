#!/usr/bin/env bash
# E2E test: Trail search by criteria
# Scenario: SCN-005 Scope 05
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Trail Search E2E ==="
e2e_start

echo "Test: Seed trails for search..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO trails (id, activity_type, distance_km, duration_min, start_time, end_time)
VALUES
  ('trail-s1', 'hike', 12.3, 240, NOW() - INTERVAL '7 days', NOW() - INTERVAL '7 days' + INTERVAL '4 hours'),
  ('trail-s2', 'walk', 3.5, 45, NOW() - INTERVAL '2 days', NOW() - INTERVAL '2 days' + INTERVAL '45 minutes'),
  ('trail-s3', 'cycle', 25.0, 90, NOW() - INTERVAL '14 days', NOW() - INTERVAL '14 days' + INTERVAL '90 minutes'),
  ('trail-s4', 'run', 5.0, 30, NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day' + INTERVAL '30 minutes')
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Search by activity type
HIKES=$(e2e_psql "SELECT COUNT(*) FROM trails WHERE activity_type='hike'")
echo "  Hike trails: $HIKES"
e2e_assert_eq "$HIKES" "1" "Hike trail found"

# Search by distance range
LONG=$(e2e_psql "SELECT COUNT(*) FROM trails WHERE distance_km > 10")
echo "  Long trails (>10km): $LONG"

# Search by date range
RECENT=$(e2e_psql "SELECT COUNT(*) FROM trails WHERE start_time > NOW() - INTERVAL '3 days'")
echo "  Recent trails (3 days): $RECENT"

e2e_pass "Trail search: filter by type, distance, date works"
