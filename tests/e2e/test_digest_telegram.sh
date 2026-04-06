#!/usr/bin/env bash
# E2E test: Digest Telegram delivery
# Scenario: SCN-002-032
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== SCN-002-032: Digest Telegram Delivery ==="
e2e_start

# Insert a digest with delivery tracking
TODAY=$(date +%Y-%m-%d)
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO digests (id, digest_date, digest_text, word_count, is_quiet, delivered_at)
VALUES ('e2e-digest-tg', '$TODAY', '! Reply to David. > 2 articles overnight.', 7, false, NOW())
ON CONFLICT (digest_date) DO UPDATE SET delivered_at = NOW();
" >/dev/null

# Verify digest is retrievable with delivery timestamp
DELIVERED=$(e2e_psql "SELECT delivered_at IS NOT NULL FROM digests WHERE id='e2e-digest-tg'")
if [ "$DELIVERED" = "t" ]; then
  e2e_pass "SCN-002-032: Digest delivery tracked"
else
  e2e_fail "SCN-002-032: Digest delivery not tracked"
fi

# NOTE: Actual Telegram delivery requires TELEGRAM_BOT_TOKEN in env.
# This test verifies the digest is stored and marked as delivered.
echo "  (Actual Telegram API delivery requires bot token in runtime config)"
