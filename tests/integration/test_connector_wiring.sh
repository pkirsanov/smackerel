#!/usr/bin/env bash
# Integration test: Connector Wiring — Config Generation Produces Env Vars for All 5 Connectors
# Scenario: SCN-019-004
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== SCN-019-004: Config Generate Produces Connector Env Vars ==="

# Run config generate to ensure generated files are fresh.
"$REPO_DIR/smackerel.sh" config generate

DEV_ENV="$REPO_DIR/config/generated/dev.env"

if [[ ! -f "$DEV_ENV" ]]; then
  echo "FAIL: $DEV_ENV does not exist after config generate" >&2
  exit 1
fi

PASS=0
FAIL=0

assert_env_exists() {
  local var_name="$1"
  if grep -q "^${var_name}=" "$DEV_ENV"; then
    echo "  PASS: $var_name present"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $var_name missing from dev.env" >&2
    FAIL=$((FAIL + 1))
  fi
}

# --- Discord (5 newly wired connector) ---
echo ""
echo "--- Discord ---"
assert_env_exists "DISCORD_ENABLED"
assert_env_exists "DISCORD_BOT_TOKEN"
assert_env_exists "DISCORD_SYNC_SCHEDULE"
assert_env_exists "DISCORD_ENABLE_GATEWAY"
assert_env_exists "DISCORD_BACKFILL_LIMIT"
assert_env_exists "DISCORD_INCLUDE_THREADS"
assert_env_exists "DISCORD_INCLUDE_PINS"
assert_env_exists "DISCORD_CAPTURE_COMMANDS"
assert_env_exists "DISCORD_MONITORED_CHANNELS"

# --- Twitter/X ---
echo ""
echo "--- Twitter ---"
assert_env_exists "TWITTER_ENABLED"
assert_env_exists "TWITTER_BEARER_TOKEN"
assert_env_exists "TWITTER_SYNC_MODE"
assert_env_exists "TWITTER_ARCHIVE_DIR"
assert_env_exists "TWITTER_SYNC_SCHEDULE"

# --- Weather ---
echo ""
echo "--- Weather ---"
assert_env_exists "WEATHER_ENABLED"
assert_env_exists "WEATHER_LOCATIONS"
assert_env_exists "WEATHER_SYNC_SCHEDULE"

# --- Gov Alerts ---
echo ""
echo "--- Gov Alerts ---"
assert_env_exists "GOV_ALERTS_ENABLED"
assert_env_exists "GOV_ALERTS_LOCATIONS"
assert_env_exists "GOV_ALERTS_MIN_EARTHQUAKE_MAG"
assert_env_exists "GOV_ALERTS_SYNC_SCHEDULE"

# --- Financial Markets ---
echo ""
echo "--- Financial Markets ---"
assert_env_exists "FINANCIAL_MARKETS_ENABLED"
assert_env_exists "FINANCIAL_MARKETS_FINNHUB_API_KEY"
assert_env_exists "FINANCIAL_MARKETS_WATCHLIST"
assert_env_exists "FINANCIAL_MARKETS_ALERT_THRESHOLD"
assert_env_exists "FINANCIAL_MARKETS_SYNC_SCHEDULE"
assert_env_exists "FINANCIAL_MARKETS_COINGECKO_ENABLED"

# --- Connector enabled defaults ---
# Original spec 019 contract: all 5 default to enabled: false
# Superseded by commit 4a90ec5e: weather and gov-alerts intentionally enabled
# for active use (Bucket A connectors). Test now checks actual intended state.
echo ""
echo "--- Default enabled check ---"
declare -A EXPECTED_ENABLED=(
  ["DISCORD"]="false"
  ["TWITTER"]="false"
  ["WEATHER"]="true"           # Intentionally enabled: 4a90ec5e
  ["GOV_ALERTS"]="true"        # Intentionally enabled: 4a90ec5e
  ["FINANCIAL_MARKETS"]="false"
)

for connector in DISCORD TWITTER WEATHER GOV_ALERTS FINANCIAL_MARKETS; do
  val=$(grep "^${connector}_ENABLED=" "$DEV_ENV" | cut -d= -f2-)
  expected="${EXPECTED_ENABLED[$connector]}"
  if [[ "$val" == "$expected" ]]; then
    echo "  PASS: ${connector}_ENABLED = $val (expected)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: ${connector}_ENABLED = '$val', expected '$expected'" >&2
    FAIL=$((FAIL + 1))
  fi
done

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [[ "$FAIL" -gt 0 ]]; then
  exit 1
fi

echo "SCN-019-004: PASS"
