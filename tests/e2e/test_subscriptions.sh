#!/usr/bin/env bash
# E2E test: Subscription management
# Scenario: SCN-006 Advanced
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Subscriptions E2E ==="
e2e_start

# Verify sync_state tracks subscriptions
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO sync_state (source_id, enabled, sync_cursor, items_synced)
VALUES
  ('rss-feed-1', true, '2026-04-01', 10),
  ('rss-feed-2', true, '2026-04-02', 15),
  ('rss-feed-3', false, '', 0)
ON CONFLICT (source_id) DO NOTHING;
" >/dev/null

ENABLED=$(e2e_psql "SELECT COUNT(*) FROM sync_state WHERE source_id LIKE 'rss-feed%' AND enabled=true")
echo "  Enabled feed subscriptions: $ENABLED"
e2e_assert_eq "$ENABLED" "2" "Active feed subscriptions"

DISABLED=$(e2e_psql "SELECT COUNT(*) FROM sync_state WHERE source_id LIKE 'rss-feed%' AND enabled=false")
e2e_assert_eq "$DISABLED" "1" "Disabled feed subscription"
e2e_pass "Subscriptions: enable/disable via sync_state"
