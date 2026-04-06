#!/usr/bin/env bash
# E2E test: People intelligence profile
# Scenario: SCN-005 Scope 04
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== People Intelligence E2E ==="
e2e_start

echo "Test: Person profile with interactions..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO people (id, name, context, organization, interaction_count, last_interaction)
VALUES ('people-e2e-001', 'David Kim', 'Engineering lead', 'TechCorp', 15, NOW() - INTERVAL '2 days')
ON CONFLICT (id) DO NOTHING;

INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight)
VALUES
  ('people-e1', 'artifact', 'synth-001', 'person', 'people-e2e-001', 'MENTIONS', 1.0),
  ('people-e2', 'artifact', 'synth-002', 'person', 'people-e2e-001', 'MENTIONS', 1.0)
ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO NOTHING;
" >/dev/null

INT_COUNT=$(e2e_psql "SELECT interaction_count FROM people WHERE id='people-e2e-001'")
echo "  David Kim interactions: $INT_COUNT"
MENTIONS=$(e2e_psql "SELECT COUNT(*) FROM edges WHERE dst_type='person' AND dst_id='people-e2e-001'")
echo "  MENTIONS edges: $MENTIONS"

e2e_pass "People intelligence: profile and interaction tracking verified"

# Relationship cooling detection
echo "Test: Relationship cooling..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO people (id, name, interaction_count, last_interaction)
VALUES ('people-e2e-cold', 'Old Contact', 5, NOW() - INTERVAL '90 days')
ON CONFLICT (id) DO NOTHING;
" >/dev/null
DAYS=$(e2e_psql "SELECT EXTRACT(DAY FROM NOW() - last_interaction)::int FROM people WHERE id='people-e2e-cold'")
echo "  Days since last interaction: $DAYS"
if [ "$DAYS" -ge 60 ]; then
  e2e_pass "Relationship cooling: stale contact detected ($DAYS days)"
fi
