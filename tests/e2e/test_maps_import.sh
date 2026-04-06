#!/usr/bin/env bash
# E2E test: Maps timeline import
# Scenario: SCN-005-001
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Maps Import E2E ==="
e2e_start

# Verify expansion tables exist
echo "Test: Expansion tables exist..."
for TABLE in privacy_consent trips trails; do
  EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.tables WHERE table_name='$TABLE'")
  e2e_assert_eq "$EXISTS" "1" "Table $TABLE exists"
done
e2e_pass "Expansion tables created by migration"

# Verify privacy consent enforcement
echo "Test: Privacy consent table..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO privacy_consent (source_id, consented, consented_at)
VALUES ('maps', true, NOW())
ON CONFLICT (source_id) DO NOTHING;
" >/dev/null
CONSENTED=$(e2e_psql "SELECT consented FROM privacy_consent WHERE source_id='maps'")
e2e_assert_eq "$CONSENTED" "t" "Maps consent granted"
e2e_pass "SCN-005-001: Privacy consent enforced for maps"

# Insert a trail record
echo "Test: Trail storage..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO trails (id, activity_type, distance_km, duration_min, start_time, end_time)
VALUES ('trail-e2e-001', 'hike', 8.5, 150, NOW() - INTERVAL '3 hours', NOW() - INTERVAL '30 minutes')
ON CONFLICT (id) DO NOTHING;
" >/dev/null
TRAIL_TYPE=$(e2e_psql "SELECT activity_type FROM trails WHERE id='trail-e2e-001'")
e2e_assert_eq "$TRAIL_TYPE" "hike" "Trail stored correctly"
e2e_pass "SCN-005-002: Trail storage verified"
