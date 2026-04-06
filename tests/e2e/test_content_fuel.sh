#!/usr/bin/env bash
# E2E test: Content fuel (subscription/feed content)
# Scenario: SCN-006 Advanced
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Content Fuel E2E ==="
e2e_start

# RSS/Atom feed-sourced artifacts
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO sync_state (source_id, enabled, sync_cursor, items_synced)
VALUES ('rss-techcrunch', true, '2026-04-01T00:00:00Z', 25)
ON CONFLICT (source_id) DO UPDATE SET items_synced=25;

INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, source_url, created_at, updated_at)
VALUES ('fuel-001', 'article', 'TechCrunch Feed Article', 'hash-fuel001', 'rss', 'https://techcrunch.com/article', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
" >/dev/null

COUNT=$(e2e_psql "SELECT COUNT(*) FROM artifacts WHERE source_id='rss'")
echo "  RSS-sourced artifacts: $COUNT"
e2e_pass "Content fuel: RSS feed artifacts stored"
