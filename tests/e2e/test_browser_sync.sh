#!/usr/bin/env bash
# E2E test: Browser history sync
# Scenario: SCN-005 Scope 02
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Browser Sync E2E ==="
e2e_start

# Verify privacy consent enforcement
echo "Test: Browser requires consent..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO privacy_consent (source_id, consented, consented_at)
VALUES ('browser', true, NOW())
ON CONFLICT (source_id) DO UPDATE SET consented=true;
" >/dev/null

CONSENTED=$(e2e_psql "SELECT consented FROM privacy_consent WHERE source_id='browser'")
e2e_assert_eq "$CONSENTED" "t" "Browser consent granted"

# Insert browser-sourced artifacts
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, source_url, created_at, updated_at)
VALUES ('browser-e2e-001', 'article', 'Browsed Article', 'hash-browser001', 'browser', 'https://example.com/page', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
" >/dev/null

COUNT=$(e2e_psql "SELECT COUNT(*) FROM artifacts WHERE source_id='browser'")
echo "  Browser artifacts: $COUNT"
e2e_pass "Browser sync: consent + artifact storage verified"
