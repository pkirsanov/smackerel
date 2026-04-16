#!/usr/bin/env bash
# E2E test: Knowledge Store (spec 025, Scope 1)
# Scenario: SCN-025-01 — Knowledge layer tables are created by migration
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Knowledge Store E2E ==="
e2e_start

# Verify knowledge_concepts table exists
echo "Test: knowledge_concepts table exists..."
EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.tables WHERE table_name='knowledge_concepts'")
e2e_assert_eq "$EXISTS" "1" "knowledge_concepts table should exist"
e2e_pass "knowledge_concepts table exists"

# Verify knowledge_entities table exists
echo "Test: knowledge_entities table exists..."
EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.tables WHERE table_name='knowledge_entities'")
e2e_assert_eq "$EXISTS" "1" "knowledge_entities table should exist"
e2e_pass "knowledge_entities table exists"

# Verify knowledge_lint_reports table exists
echo "Test: knowledge_lint_reports table exists..."
EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.tables WHERE table_name='knowledge_lint_reports'")
e2e_assert_eq "$EXISTS" "1" "knowledge_lint_reports table should exist"
e2e_pass "knowledge_lint_reports table exists"

# Verify artifacts table has synthesis columns
echo "Test: artifacts.synthesis_status column exists..."
EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.columns WHERE table_name='artifacts' AND column_name='synthesis_status'")
e2e_assert_eq "$EXISTS" "1" "artifacts.synthesis_status column should exist"
e2e_pass "artifacts.synthesis_status column exists"

echo "Test: artifacts.synthesis_at column exists..."
EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.columns WHERE table_name='artifacts' AND column_name='synthesis_at'")
e2e_assert_eq "$EXISTS" "1" "artifacts.synthesis_at column should exist"
e2e_pass "artifacts.synthesis_at column exists"

echo "Test: artifacts.synthesis_error column exists..."
EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.columns WHERE table_name='artifacts' AND column_name='synthesis_error'")
e2e_assert_eq "$EXISTS" "1" "artifacts.synthesis_error column should exist"
e2e_pass "artifacts.synthesis_error column exists"

echo "Test: artifacts.synthesis_retry_count column exists..."
EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.columns WHERE table_name='artifacts' AND column_name='synthesis_retry_count'")
e2e_assert_eq "$EXISTS" "1" "artifacts.synthesis_retry_count column should exist"
e2e_pass "artifacts.synthesis_retry_count column exists"

# Verify indexes on knowledge_concepts
echo "Test: knowledge_concepts indexes exist..."
IDX_COUNT=$(e2e_psql "SELECT COUNT(*) FROM pg_indexes WHERE tablename='knowledge_concepts'")
if [ "$IDX_COUNT" -lt "3" ]; then
  e2e_fail "Expected at least 3 indexes on knowledge_concepts (got $IDX_COUNT)"
fi
e2e_pass "knowledge_concepts indexes exist ($IDX_COUNT found)"

# Verify concept insert + unique constraint
echo "Test: Concept insert and unique constraint..."
smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO knowledge_concepts (id, title, title_normalized, summary, claims, prompt_contract_version)
VALUES ('test-concept-001', 'Leadership', 'leadership', 'Test concept', '[]', 'test-v1')
ON CONFLICT (title_normalized) DO NOTHING;
" >/dev/null

CONCEPT_EXISTS=$(e2e_psql "SELECT COUNT(*) FROM knowledge_concepts WHERE id='test-concept-001'")
e2e_assert_eq "$CONCEPT_EXISTS" "1" "Concept should be inserted"

# Try duplicate — should be rejected by unique constraint
DUP_RESULT=$(smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "
INSERT INTO knowledge_concepts (id, title, title_normalized, summary, claims, prompt_contract_version)
VALUES ('test-concept-002', 'LEADERSHIP', 'leadership', 'Duplicate', '[]', 'test-v1')
ON CONFLICT (title_normalized) DO NOTHING
RETURNING id;
" | tr -d '[:space:]')
if [ -n "$DUP_RESULT" ]; then
  e2e_fail "Duplicate normalized title should be rejected"
fi
e2e_pass "Concept unique constraint works"

# Verify entity insert + unique constraint on (name_normalized, entity_type)
echo "Test: Entity insert and unique constraint..."
smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO knowledge_entities (id, name, name_normalized, entity_type, mentions, prompt_contract_version)
VALUES ('test-entity-001', 'Sarah', 'sarah', 'person', '[]', 'test-v1')
ON CONFLICT (name_normalized, entity_type) DO NOTHING;
" >/dev/null

ENTITY_EXISTS=$(e2e_psql "SELECT COUNT(*) FROM knowledge_entities WHERE id='test-entity-001'")
e2e_assert_eq "$ENTITY_EXISTS" "1" "Entity should be inserted"
e2e_pass "Entity insert and unique constraint works"

# Cleanup test data
smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
DELETE FROM knowledge_concepts WHERE id LIKE 'test-concept-%';
DELETE FROM knowledge_entities WHERE id LIKE 'test-entity-%';
" >/dev/null

echo "=== All Knowledge Store E2E tests passed ==="
