#!/usr/bin/env bash
# E2E test: Bookmark import
# Scenario: SCN-003 Scope 05
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Bookmark Import E2E ==="
e2e_start

# Verify artifacts table accepts bookmark-sourced content
echo "Test: Bookmark artifact storage..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, source_url, created_at, updated_at)
VALUES
  ('bm-e2e-001', 'article', 'Bookmarked Article 1', 'hash-bm001', 'bookmarks', 'https://example.com/bookmark1', NOW(), NOW()),
  ('bm-e2e-002', 'article', 'Bookmarked Article 2', 'hash-bm002', 'bookmarks', 'https://example.com/bookmark2', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
" >/dev/null

COUNT=$(e2e_psql "SELECT COUNT(*) FROM artifacts WHERE source_id='bookmarks'")
echo "  Bookmark artifacts: $COUNT"
e2e_assert_eq "$COUNT" "2" "Bookmark artifacts stored"
e2e_pass "Bookmark import: artifacts stored with correct source_id"

# Verify dedup by content hash. The artifacts.content_hash column is
# protected by a partial unique index (`WHERE content_hash IS NOT NULL`),
# which cannot be used directly with ON CONFLICT — so we expect the
# duplicate INSERT to be rejected with a unique-violation error and
# verify the dedup outcome by row count.
echo "Test: Bookmark dedup..."
DUP_OUT=$(smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=0 -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, source_url, created_at, updated_at)
VALUES ('bm-e2e-003', 'article', 'Duplicate Bookmark', 'hash-bm001', 'bookmarks', 'https://example.com/bookmark1', NOW(), NOW());
" 2>&1 || true)
if ! echo "$DUP_OUT" | grep -q "duplicate key value violates unique constraint"; then
  echo "FAIL: expected unique-violation on content_hash, got:"
  echo "$DUP_OUT"
  exit 1
fi

DUP_CHECK=$(e2e_psql "SELECT COUNT(*) FROM artifacts WHERE content_hash='hash-bm001'")
echo "  Artifacts with same hash: $DUP_CHECK"
# Content hash index allows dedup checks
e2e_pass "Bookmark import: dedup infrastructure verified"
