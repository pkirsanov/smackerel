#!/usr/bin/env bash
# E2E test: IMAP email connector sync state
# Scenario: SCN-003 Scope 02
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== IMAP Sync E2E ==="
e2e_start

echo "Test: IMAP connector sync state..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO sync_state (source_id, enabled, sync_cursor, items_synced, errors_count)
VALUES ('gmail-imap', true, 'uid-500', 120, 0)
ON CONFLICT (source_id) DO UPDATE SET sync_cursor='uid-500', items_synced=120;
" >/dev/null

CURSOR=$(e2e_psql "SELECT sync_cursor FROM sync_state WHERE source_id='gmail-imap'")
e2e_assert_eq "$CURSOR" "uid-500" "IMAP cursor persisted"
ITEMS=$(e2e_psql "SELECT items_synced FROM sync_state WHERE source_id='gmail-imap'")
e2e_assert_eq "$ITEMS" "120" "IMAP items synced count correct"
e2e_pass "IMAP connector sync state verified"
