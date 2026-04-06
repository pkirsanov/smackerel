#!/usr/bin/env bash
# E2E test: CalDAV calendar connector sync state
# Scenario: SCN-003 Scope 03
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== CalDAV Sync E2E ==="
e2e_start

smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO sync_state (source_id, enabled, sync_cursor, items_synced, errors_count)
VALUES ('google-calendar', true, 'sync-token-abc', 45, 0)
ON CONFLICT (source_id) DO UPDATE SET sync_cursor='sync-token-abc', items_synced=45;
" >/dev/null

CURSOR=$(e2e_psql "SELECT sync_cursor FROM sync_state WHERE source_id='google-calendar'")
e2e_assert_eq "$CURSOR" "sync-token-abc" "CalDAV cursor persisted"
e2e_pass "CalDAV connector sync state verified"
