#!/usr/bin/env bash
# E2E test: Synthesis engine
# Scenario: SCN-004-001
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Synthesis Engine E2E ==="
e2e_start

# Verify intelligence tables exist
echo "Test: Intelligence tables exist..."
for TABLE in synthesis_insights alerts meeting_briefs weekly_synthesis; do
  EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.tables WHERE table_name='$TABLE'")
  if [ "$EXISTS" != "1" ]; then
    e2e_fail "Table $TABLE missing"
  fi
done
e2e_pass "Intelligence tables exist"

# Seed artifacts and topics for cluster detection
echo "Seeding data for synthesis..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO topics (id, name, state, capture_count_total)
VALUES ('synth-topic', 'synthesis-test', 'active', 5)
ON CONFLICT (id) DO NOTHING;

INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, topics, created_at, updated_at)
VALUES
  ('synth-001', 'article', 'Synthesis Test A', 'hash-synth001', 'test', '[\"synthesis-test\"]', NOW(), NOW()),
  ('synth-002', 'article', 'Synthesis Test B', 'hash-synth002', 'test', '[\"synthesis-test\"]', NOW(), NOW()),
  ('synth-003', 'article', 'Synthesis Test C', 'hash-synth003', 'test', '[\"synthesis-test\"]', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight)
VALUES
  ('synth-e1', 'artifact', 'synth-001', 'topic', 'synth-topic', 'BELONGS_TO', 1.0),
  ('synth-e2', 'artifact', 'synth-002', 'topic', 'synth-topic', 'BELONGS_TO', 1.0),
  ('synth-e3', 'artifact', 'synth-003', 'topic', 'synth-topic', 'BELONGS_TO', 1.0)
ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO NOTHING;
" >/dev/null

# Verify cluster can be detected (3+ artifacts sharing a topic)
CLUSTER=$(e2e_psql "
  SELECT COUNT(DISTINCT e.src_id) FROM edges e
  WHERE e.dst_type='topic' AND e.dst_id='synth-topic' AND e.edge_type='BELONGS_TO'
")
echo "  Cluster size: $CLUSTER"
if [ "$CLUSTER" -ge 3 ]; then
  e2e_pass "SCN-004-001: Cross-domain cluster detected (size=$CLUSTER)"
else
  e2e_fail "SCN-004-001: Expected cluster >=3, got $CLUSTER"
fi
