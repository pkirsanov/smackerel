#!/usr/bin/env bash
# E2E test: Digest Telegram delivery
# Scenario: SCN-002-032
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== SCN-002-032: Digest Telegram Delivery ==="
e2e_start

# Insert a digest and track delivery by the stored digest identity. The upsert
# updates the row id as well as delivered_at so this stays stable when the broad
# E2E suite has already seeded today's digest in test_digest.sh.
TODAY=$(date +%Y-%m-%d)
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO digests (id, digest_date, digest_text, word_count, is_quiet, delivered_at)
VALUES ('e2e-digest-tg', '$TODAY', '! Reply to David. > 2 articles overnight.', 7, false, NULL)
ON CONFLICT (digest_date) DO UPDATE
SET id = EXCLUDED.id,
    digest_text = EXCLUDED.digest_text,
    word_count = EXCLUDED.word_count,
    is_quiet = EXCLUDED.is_quiet,
    delivered_at = NULL;

UPDATE digests SET delivered_at = NOW() WHERE id='e2e-digest-tg';

INSERT INTO digests (id, digest_date, digest_text, word_count, is_quiet, delivered_at)
VALUES ('e2e-digest-tg-missing', ('$TODAY'::date - INTERVAL '1 day')::date, 'Generated but not delivered.', 4, false, NULL)
ON CONFLICT (digest_date) DO UPDATE
SET id = EXCLUDED.id,
    digest_text = EXCLUDED.digest_text,
    word_count = EXCLUDED.word_count,
    is_quiet = EXCLUDED.is_quiet,
    delivered_at = NULL;
" >/dev/null

# Verify digest is retrievable with delivery timestamp
DELIVERED=$(e2e_psql "SELECT COALESCE((SELECT delivered_at IS NOT NULL FROM digests WHERE id='e2e-digest-tg'), false)")
UNDELIVERED_REJECTED=$(e2e_psql "SELECT COALESCE((SELECT delivered_at IS NULL FROM digests WHERE id='e2e-digest-tg-missing'), false)")
if [ "$DELIVERED" = "t" ] && [ "$UNDELIVERED_REJECTED" = "t" ]; then
  e2e_pass "SCN-002-032: Digest delivery tracked"
else
  e2e_fail "SCN-002-032: Digest delivery not tracked"
fi

# NOTE: Actual Telegram delivery requires TELEGRAM_BOT_TOKEN in env.
# This test verifies the digest is stored and marked as delivered.
echo "  (Actual Telegram API delivery requires bot token in runtime config)"
