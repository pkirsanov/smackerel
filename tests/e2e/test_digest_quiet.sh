#!/usr/bin/env bash
# E2E test: Quiet day digest
# Scenario: SCN-002-031
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== SCN-002-031: Quiet Day Digest ==="
e2e_start

# Insert quiet day digest
smackerel_compose "$TEST_ENV" exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
INSERT INTO digests (id, digest_date, digest_text, word_count, is_quiet)
VALUES ('e2e-quiet-001', '2025-12-25', 'All quiet. Nothing needs your attention today.', 9, true)
ON CONFLICT (digest_date) DO NOTHING;
" >/dev/null

RESPONSE=$(e2e_api GET "/api/digest?date=2025-12-25")
TEXT=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('text',''))" 2>/dev/null || true)

if echo "$TEXT" | grep -qi "quiet"; then
  e2e_pass "SCN-002-031: Quiet day digest says 'All quiet'"
else
  e2e_fail "SCN-002-031: Expected quiet message, got: $TEXT"
fi
