#!/usr/bin/env bash
# E2E test: Pre-meeting briefs
# Scenario: SCN-004 Scope 03
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Pre-Meeting Briefs E2E ==="
e2e_start

echo "Test: Meeting briefs table..."
EXISTS=$(e2e_psql "SELECT COUNT(*) FROM information_schema.tables WHERE table_name='meeting_briefs'")
e2e_assert_eq "$EXISTS" "1" "meeting_briefs table exists"

echo "Test: Insert meeting brief..."
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO meeting_briefs (id, event_id, event_title, event_time, attendees, brief_text, generated_at)
VALUES ('mb-e2e-001', 'cal-event-001', 'Sprint Planning', NOW() + INTERVAL '1 day',
  '[{\"name\": \"David Kim\", \"email\": \"david@example.com\"}]',
  '> David Kim — last interaction 2 days ago. Recent topics: distributed systems, architecture.',
  NOW())
ON CONFLICT (event_id) DO NOTHING;
" >/dev/null

BRIEF=$(e2e_psql "SELECT event_title FROM meeting_briefs WHERE id='mb-e2e-001'")
e2e_assert_eq "$BRIEF" "SprintPlanning" "Meeting brief stored"

# Verify dedup by event_id
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO meeting_briefs (id, event_id, event_title, event_time)
VALUES ('mb-e2e-002', 'cal-event-001', 'Sprint Planning', NOW() + INTERVAL '1 day')
ON CONFLICT (event_id) DO NOTHING;
" >/dev/null

COUNT=$(e2e_psql "SELECT COUNT(*) FROM meeting_briefs WHERE event_id='cal-event-001'")
e2e_assert_eq "$COUNT" "1" "Meeting brief deduped by event_id"
e2e_pass "Pre-meeting brief: storage and dedup verified"
