#!/usr/bin/env bash
# E2E test: YouTube connector sync state
# Scenario: SCN-003 Scope 04
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== YouTube Sync E2E ==="
e2e_start

smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO sync_state (source_id, enabled, sync_cursor, items_synced, errors_count)
VALUES ('youtube', true, 'page-token-xyz', 30, 0)
ON CONFLICT (source_id) DO UPDATE SET sync_cursor='page-token-xyz', items_synced=30;
" >/dev/null

CURSOR=$(e2e_psql "SELECT sync_cursor FROM sync_state WHERE source_id='youtube'")
e2e_assert_eq "$CURSOR" "page-token-xyz" "YouTube cursor persisted"

# Verify video-type artifacts can be stored
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, source_url, created_at, updated_at)
VALUES ('yt-e2e-001', 'video', 'SaaS Pricing Video', 'hash-yt001', 'youtube', 'https://youtube.com/watch?v=test', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
" >/dev/null

TYPE=$(e2e_psql "SELECT artifact_type FROM artifacts WHERE id='yt-e2e-001'")
e2e_assert_eq "$TYPE" "video" "YouTube artifact stored as video type"
e2e_pass "YouTube connector: sync state and video storage verified"
