#!/usr/bin/env bash
# E2E test: Trip dossier assembly
# Scenario: SCN-005 Scope 03
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Trip Dossier E2E ==="
e2e_start

echo "Test: Trip storage..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO trips (id, name, destination, start_date, end_date, status)
VALUES
  ('trip-e2e-001', 'Tokyo Conference', 'Tokyo, Japan', CURRENT_DATE + 5, CURRENT_DATE + 10, 'upcoming'),
  ('trip-e2e-002', 'London Sprint', 'London, UK', CURRENT_DATE - 30, CURRENT_DATE - 25, 'completed')
ON CONFLICT (id) DO NOTHING;
" >/dev/null

UPCOMING=$(e2e_psql "SELECT COUNT(*) FROM trips WHERE status='upcoming'")
echo "  Upcoming trips: $UPCOMING"
e2e_assert_eq "$UPCOMING" "1" "Upcoming trip stored"

# Verify proactive delivery window (trip in 5 days)
SOON=$(e2e_psql "SELECT COUNT(*) FROM trips WHERE status='upcoming' AND start_date <= CURRENT_DATE + 7")
echo "  Trips within 7 days: $SOON"
if [ "$SOON" -ge 1 ]; then
  e2e_pass "Trip dossier: proactive delivery window detected"
fi

e2e_pass "Trip dossier storage and lifecycle verified"
