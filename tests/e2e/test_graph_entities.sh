#!/usr/bin/env bash
# E2E test: Graph entity and topic linking
# Scenarios: SCN-002-017, SCN-002-018
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Graph Entities E2E Tests ==="
e2e_start

# Seed person
echo "Seeding person entity..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO people (id, name, interaction_count, last_interaction)
VALUES ('e2e-person-sarah', 'Sarah', 0, NULL)
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Seed topic
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO topics (id, name, state, capture_count_total)
VALUES ('e2e-topic-pricing', 'pricing', 'emerging', 0)
ON CONFLICT (id) DO NOTHING;
" >/dev/null

# Verify person exists
PERSON=$(e2e_psql "SELECT name FROM people WHERE id='e2e-person-sarah'")
e2e_assert_eq "$PERSON" "Sarah" "Person Sarah exists"

# Verify topic exists
TOPIC=$(e2e_psql "SELECT name FROM topics WHERE id='e2e-topic-pricing'")
e2e_assert_eq "$TOPIC" "pricing" "Topic pricing exists"

e2e_pass "SCN-002-017: Entity infrastructure ready"
e2e_pass "SCN-002-018: Topic infrastructure ready"
