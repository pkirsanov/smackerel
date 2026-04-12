#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG_FILE="$REPO_ROOT/config/smackerel.yaml"
TARGET_ENV="dev"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env)
      TARGET_ENV="$2"
      shift 2
      ;;
    --env=*)
      TARGET_ENV="${1#*=}"
      shift
      ;;
    --config)
      CONFIG_FILE="$2"
      shift 2
      ;;
    --config=*)
      CONFIG_FILE="${1#*=}"
      shift
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

[[ -f "$CONFIG_FILE" ]] || {
  echo "Missing config file: $CONFIG_FILE" >&2
  exit 1
}

flatten_yaml() {
  local yaml_file="$1"

  awk '
    function trim(value) {
      sub(/^[[:space:]]+/, "", value)
      sub(/[[:space:]]+$/, "", value)
      return value
    }

    {
      line = $0
      sub(/[[:space:]]+#.*$/, "", line)
      if (line ~ /^[[:space:]]*$/) {
        next
      }

      indent = match(line, /[^ ]/) - 1
      key = line
      sub(/:.*/, "", key)
      key = trim(key)

      is_map = (line ~ /:[[:space:]]*$/)

      value = line
      sub(/^[^:]+:[[:space:]]*/, "", value)
      value = trim(value)
      gsub(/^"/, "", value)
      gsub(/"$/, "", value)

      if (indent == 0) {
        level1 = key
        level2 = ""
        level3 = ""
        level4 = ""
        path = level1
      } else if (indent == 2) {
        level2 = key
        level3 = ""
        level4 = ""
        path = level1 "." level2
      } else if (indent == 4) {
        level3 = key
        level4 = ""
        path = level1 "." level2 "." level3
      } else if (indent == 6) {
        level4 = key
        path = level1 "." level2 "." level3 "." level4
      } else {
        next
      }

      if (is_map) {
        next
      }

      print path "=" value
    }
  ' "$yaml_file"
}

FLATTENED_CONFIG="$(flatten_yaml "$CONFIG_FILE")"

yaml_get() {
  local key="$1"

  awk -F= -v target="$key" '
    $1 == target {
      print substr($0, length($1) + 2)
      found = 1
      exit
    }
    END {
      if (!found) {
        exit 1
      }
    }
  ' <<<"$FLATTENED_CONFIG"
}

required_value() {
  local key="$1"
  local value

  value="$(yaml_get "$key")" || {
    echo "Missing config key: $key" >&2
    exit 1
  }

  printf '%s' "$value"
}

# Extract a complex YAML value (array or object) as JSON using Python3.
# Returns empty string if the path is not found or python3 is unavailable.
yaml_get_json() {
  local key="$1"
  python3 - "$CONFIG_FILE" "$key" <<'PYEOF' 2>/dev/null || echo ""
import sys, json

def extract(filepath, dotpath):
    with open(filepath) as f:
        lines = f.readlines()
    parts = dotpath.split('.')
    depth = 0
    target_line = -1
    target_indent = 0
    for i, raw in enumerate(lines):
        text = raw.rstrip('\n')
        stripped = text.lstrip()
        if not stripped or stripped.startswith('#'):
            continue
        indent = len(text) - len(stripped)
        expected = depth * 2
        if indent == expected and stripped.startswith(parts[0] + ':'):
            if len(parts) == 1:
                val = stripped.split(':', 1)[1].strip()
                if val:
                    print(val)
                    return
                target_line = i
                target_indent = -1
                break
            parts.pop(0)
            depth += 1
    if target_line < 0:
        return
    block = []
    for i in range(target_line + 1, len(lines)):
        text = lines[i].rstrip('\n')
        stripped = text.lstrip()
        if not stripped or stripped.startswith('#'):
            continue
        indent = len(text) - len(stripped)
        if target_indent < 0:
            target_indent = indent
        if indent < target_indent:
            break
        block.append(text[target_indent:])
    if not block:
        return
    if block[0].lstrip().startswith('- '):
        result = parse_array(block)
    else:
        result = parse_object(block)
    print(json.dumps(result))

def parse_array(lines):
    arr, cur = [], {}
    for ln in lines:
        s = ln.lstrip()
        if s.startswith('- '):
            if cur:
                arr.append(cur)
            cur = {}
            kv = s[2:]
            if ':' in kv:
                k, v = kv.split(':', 1)
                cur[k.strip()] = scalar(v.strip())
        elif ':' in s:
            k, v = s.split(':', 1)
            cur[k.strip()] = scalar(v.strip())
    if cur:
        arr.append(cur)
    return arr

def parse_object(lines):
    obj = {}
    for ln in lines:
        s = ln.lstrip()
        if ':' in s:
            k, v = s.split(':', 1)
            obj[k.strip()] = scalar(v.strip())
    return obj

def scalar(v):
    if v in ('', '""', "''"):
        return ''
    if v == '[]':
        return []
    if v == '{}':
        return {}
    if v == 'true':
        return True
    if v == 'false':
        return False
    if v.startswith('[') and v.endswith(']'):
        inner = v[1:-1].strip()
        return [s.strip().strip('"').strip("'") for s in inner.split(',')] if inner else []
    try:
        return float(v) if '.' in v else int(v)
    except ValueError:
        pass
    if len(v) >= 2 and v[0] in ('"', "'") and v[-1] == v[0]:
        return v[1:-1]
    return v

extract(sys.argv[1], sys.argv[2])
PYEOF
}

case "$TARGET_ENV" in
  dev|test)
    ;;
  *)
    echo "Unsupported environment: $TARGET_ENV" >&2
    exit 1
    ;;
esac

PROJECT_NAME="$(required_value project.name)"
CORE_CONTAINER_PORT="$(required_value services.core.container_port)"
SHUTDOWN_TIMEOUT_S="$(required_value services.core.shutdown_timeout_s)"
ML_CONTAINER_PORT="$(required_value services.ml.container_port)"
ML_HEALTH_CACHE_TTL_S="$(required_value services.ml.health_cache_ttl_s)"
POSTGRES_USER="$(required_value infrastructure.postgres.user)"
POSTGRES_PASSWORD="$(required_value infrastructure.postgres.password)"
POSTGRES_DB="$(required_value infrastructure.postgres.database)"
POSTGRES_CONTAINER_PORT="$(required_value infrastructure.postgres.container_port)"
DB_MAX_CONNS="$(required_value infrastructure.postgres.max_conns)"
DB_MIN_CONNS="$(required_value infrastructure.postgres.min_conns)"
NATS_CLIENT_PORT="$(required_value infrastructure.nats.client_port)"
NATS_MONITOR_PORT="$(required_value infrastructure.nats.monitor_port)"
OLLAMA_ENABLED="$(required_value infrastructure.ollama.enabled)"
OLLAMA_CONTAINER_PORT="$(required_value infrastructure.ollama.container_port)"
LLM_PROVIDER="$(required_value llm.provider)"
LLM_MODEL="$(required_value llm.model)"
LLM_API_KEY="$(required_value llm.api_key)"
OLLAMA_URL="$(required_value llm.ollama_url)"
OLLAMA_MODEL="$(required_value llm.ollama_model)"
OLLAMA_VISION_MODEL="$(required_value llm.ollama_vision_model)"
SMACKEREL_AUTH_TOKEN="$(required_value runtime.auth_token)"
HOST_BIND_ADDRESS="$(required_value runtime.host_bind_address)"
DIGEST_CRON="$(required_value runtime.digest_cron)"
EMBEDDING_MODEL="$(required_value runtime.embedding_model)"
LOG_LEVEL="$(required_value runtime.log_level)"
TELEGRAM_BOT_TOKEN="$(required_value telegram.bot_token)"
TELEGRAM_CHAT_IDS="$(required_value telegram.chat_ids)"
TELEGRAM_ASSEMBLY_WINDOW_SECONDS="$(yaml_get telegram.assembly_window_seconds 2>/dev/null)" || TELEGRAM_ASSEMBLY_WINDOW_SECONDS="10"
TELEGRAM_ASSEMBLY_MAX_MESSAGES="$(yaml_get telegram.assembly_max_messages 2>/dev/null)" || TELEGRAM_ASSEMBLY_MAX_MESSAGES="100"
TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS="$(yaml_get telegram.media_group_window_seconds 2>/dev/null)" || TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS="3"

COMPOSE_PROJECT="$(required_value environments.$TARGET_ENV.compose_project)"
POSTGRES_HOST_PORT="$(required_value environments.$TARGET_ENV.postgres_host_port)"
NATS_CLIENT_HOST_PORT="$(required_value environments.$TARGET_ENV.nats_client_host_port)"
NATS_MONITOR_HOST_PORT="$(required_value environments.$TARGET_ENV.nats_monitor_host_port)"
CORE_HOST_PORT="$(required_value environments.$TARGET_ENV.core_host_port)"
ML_HOST_PORT="$(required_value environments.$TARGET_ENV.ml_host_port)"
OLLAMA_HOST_PORT="$(required_value environments.$TARGET_ENV.ollama_host_port)"
POSTGRES_VOLUME_NAME="$(required_value environments.$TARGET_ENV.postgres_volume_name)"
NATS_VOLUME_NAME="$(required_value environments.$TARGET_ENV.nats_volume_name)"
OLLAMA_VOLUME_NAME="$(required_value environments.$TARGET_ENV.ollama_volume_name)"

DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:${POSTGRES_CONTAINER_PORT}/${POSTGRES_DB}?sslmode=disable"
NATS_URL="nats://nats:${NATS_CLIENT_PORT}"
ML_SIDECAR_URL="http://smackerel-ml:${ML_CONTAINER_PORT}"
CORE_API_URL="http://smackerel-core:${CORE_CONTAINER_PORT}"
CORE_EXTERNAL_URL="http://127.0.0.1:${CORE_HOST_PORT}"
ML_EXTERNAL_URL="http://127.0.0.1:${ML_HOST_PORT}"

# Connector import paths (optional — empty string is valid default)
BOOKMARKS_IMPORT_DIR="$(yaml_get connectors.bookmarks.import_dir 2>/dev/null)" || BOOKMARKS_IMPORT_DIR=""
MAPS_IMPORT_DIR="$(yaml_get connectors.google-maps-timeline.import_dir 2>/dev/null)" || MAPS_IMPORT_DIR=""
BROWSER_HISTORY_PATH="$(yaml_get connectors.browser-history.chrome.history_path 2>/dev/null)" || BROWSER_HISTORY_PATH=""

# Discord connector
DISCORD_ENABLED="$(yaml_get connectors.discord.enabled 2>/dev/null)" || DISCORD_ENABLED="false"

# GuestHost connector
GUESTHOST_ENABLED="$(yaml_get connectors.guesthost.enabled 2>/dev/null)" || GUESTHOST_ENABLED="false"
GUESTHOST_BASE_URL="$(yaml_get connectors.guesthost.base_url 2>/dev/null)" || GUESTHOST_BASE_URL=""
GUESTHOST_API_KEY="$(yaml_get connectors.guesthost.api_key 2>/dev/null)" || GUESTHOST_API_KEY=""
GUESTHOST_SYNC_SCHEDULE="$(yaml_get connectors.guesthost.sync_schedule 2>/dev/null)" || GUESTHOST_SYNC_SCHEDULE=""
GUESTHOST_EVENT_TYPES="$(yaml_get connectors.guesthost.event_types 2>/dev/null)" || GUESTHOST_EVENT_TYPES=""
DISCORD_BOT_TOKEN="$(yaml_get connectors.discord.bot_token 2>/dev/null)" || DISCORD_BOT_TOKEN=""
DISCORD_SYNC_SCHEDULE="$(yaml_get connectors.discord.sync_schedule 2>/dev/null)" || DISCORD_SYNC_SCHEDULE=""
DISCORD_ENABLE_GATEWAY="$(yaml_get connectors.discord.enable_gateway 2>/dev/null)" || DISCORD_ENABLE_GATEWAY=""
DISCORD_BACKFILL_LIMIT="$(yaml_get connectors.discord.backfill_limit 2>/dev/null)" || DISCORD_BACKFILL_LIMIT=""
DISCORD_INCLUDE_THREADS="$(yaml_get connectors.discord.include_threads 2>/dev/null)" || DISCORD_INCLUDE_THREADS=""
DISCORD_INCLUDE_PINS="$(yaml_get connectors.discord.include_pins 2>/dev/null)" || DISCORD_INCLUDE_PINS=""
DISCORD_CAPTURE_COMMANDS="$(yaml_get_json connectors.discord.capture_commands 2>/dev/null)" || DISCORD_CAPTURE_COMMANDS=""
DISCORD_MONITORED_CHANNELS="$(yaml_get_json connectors.discord.monitored_channels 2>/dev/null)" || DISCORD_MONITORED_CHANNELS=""

# Twitter connector
TWITTER_ENABLED="$(yaml_get connectors.twitter.enabled 2>/dev/null)" || TWITTER_ENABLED="false"
TWITTER_SYNC_MODE="$(yaml_get connectors.twitter.sync_mode 2>/dev/null)" || TWITTER_SYNC_MODE=""
TWITTER_ARCHIVE_DIR="$(yaml_get connectors.twitter.archive_dir 2>/dev/null)" || TWITTER_ARCHIVE_DIR=""
TWITTER_BEARER_TOKEN="$(yaml_get connectors.twitter.bearer_token 2>/dev/null)" || TWITTER_BEARER_TOKEN=""
TWITTER_SYNC_SCHEDULE="$(yaml_get connectors.twitter.sync_schedule 2>/dev/null)" || TWITTER_SYNC_SCHEDULE=""

# Weather connector
WEATHER_ENABLED="$(yaml_get connectors.weather.enabled 2>/dev/null)" || WEATHER_ENABLED="false"
WEATHER_SYNC_SCHEDULE="$(yaml_get connectors.weather.sync_schedule 2>/dev/null)" || WEATHER_SYNC_SCHEDULE=""
WEATHER_LOCATIONS="$(yaml_get_json connectors.weather.locations 2>/dev/null)" || WEATHER_LOCATIONS=""

# Gov Alerts connector
GOV_ALERTS_ENABLED="$(yaml_get connectors.gov-alerts.enabled 2>/dev/null)" || GOV_ALERTS_ENABLED="false"
GOV_ALERTS_SYNC_SCHEDULE="$(yaml_get connectors.gov-alerts.sync_schedule 2>/dev/null)" || GOV_ALERTS_SYNC_SCHEDULE=""
GOV_ALERTS_MIN_EARTHQUAKE_MAG="$(yaml_get connectors.gov-alerts.min_earthquake_magnitude 2>/dev/null)" || GOV_ALERTS_MIN_EARTHQUAKE_MAG=""
GOV_ALERTS_SOURCE_WEATHER="$(yaml_get connectors.gov-alerts.source_weather 2>/dev/null)" || GOV_ALERTS_SOURCE_WEATHER=""
GOV_ALERTS_SOURCE_TSUNAMI="$(yaml_get connectors.gov-alerts.source_tsunami 2>/dev/null)" || GOV_ALERTS_SOURCE_TSUNAMI=""
GOV_ALERTS_SOURCE_VOLCANO="$(yaml_get connectors.gov-alerts.source_volcano 2>/dev/null)" || GOV_ALERTS_SOURCE_VOLCANO=""
GOV_ALERTS_SOURCE_WILDFIRE="$(yaml_get connectors.gov-alerts.source_wildfire 2>/dev/null)" || GOV_ALERTS_SOURCE_WILDFIRE=""
GOV_ALERTS_SOURCE_AIRNOW="$(yaml_get connectors.gov-alerts.source_airnow 2>/dev/null)" || GOV_ALERTS_SOURCE_AIRNOW=""
GOV_ALERTS_SOURCE_GDACS="$(yaml_get connectors.gov-alerts.source_gdacs 2>/dev/null)" || GOV_ALERTS_SOURCE_GDACS=""
GOV_ALERTS_AIRNOW_API_KEY="$(yaml_get connectors.gov-alerts.airnow_api_key 2>/dev/null)" || GOV_ALERTS_AIRNOW_API_KEY=""
GOV_ALERTS_LOCATIONS="$(yaml_get_json connectors.gov-alerts.locations 2>/dev/null)" || GOV_ALERTS_LOCATIONS=""

# Financial Markets connector
FINANCIAL_MARKETS_ENABLED="$(yaml_get connectors.financial-markets.enabled 2>/dev/null)" || FINANCIAL_MARKETS_ENABLED="false"
FINANCIAL_MARKETS_SYNC_SCHEDULE="$(yaml_get connectors.financial-markets.sync_schedule 2>/dev/null)" || FINANCIAL_MARKETS_SYNC_SCHEDULE=""
FINANCIAL_MARKETS_FINNHUB_API_KEY="$(yaml_get connectors.financial-markets.finnhub_api_key 2>/dev/null)" || FINANCIAL_MARKETS_FINNHUB_API_KEY=""
FINANCIAL_MARKETS_FRED_API_KEY="$(yaml_get connectors.financial-markets.fred_api_key 2>/dev/null)" || FINANCIAL_MARKETS_FRED_API_KEY=""
FINANCIAL_MARKETS_COINGECKO_ENABLED="$(yaml_get connectors.financial-markets.coingecko_enabled 2>/dev/null)" || FINANCIAL_MARKETS_COINGECKO_ENABLED=""
FINANCIAL_MARKETS_ALERT_THRESHOLD="$(yaml_get connectors.financial-markets.alert_threshold 2>/dev/null)" || FINANCIAL_MARKETS_ALERT_THRESHOLD=""
FINANCIAL_MARKETS_WATCHLIST="$(yaml_get_json connectors.financial-markets.watchlist 2>/dev/null)" || FINANCIAL_MARKETS_WATCHLIST=""

mkdir -p "$REPO_ROOT/config/generated"

OUTPUT_FILE="$REPO_ROOT/config/generated/${TARGET_ENV}.env"

cat > "$OUTPUT_FILE" <<EOF
# Auto-generated from config/smackerel.yaml — DO NOT EDIT DIRECTLY
# Regenerate: ./smackerel.sh config generate
# Environment: ${TARGET_ENV}
# Generated: $(date -u +%Y-%m-%dT%H:%M:%S+00:00)
PROJECT_NAME=${PROJECT_NAME}
COMPOSE_PROJECT=${COMPOSE_PROJECT}
POSTGRES_USER=${POSTGRES_USER}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_DB=${POSTGRES_DB}
POSTGRES_CONTAINER_PORT=${POSTGRES_CONTAINER_PORT}
POSTGRES_HOST_PORT=${POSTGRES_HOST_PORT}
POSTGRES_VOLUME_NAME=${POSTGRES_VOLUME_NAME}
NATS_CLIENT_PORT=${NATS_CLIENT_PORT}
NATS_MONITOR_PORT=${NATS_MONITOR_PORT}
NATS_CLIENT_HOST_PORT=${NATS_CLIENT_HOST_PORT}
NATS_MONITOR_HOST_PORT=${NATS_MONITOR_HOST_PORT}
NATS_VOLUME_NAME=${NATS_VOLUME_NAME}
CORE_CONTAINER_PORT=${CORE_CONTAINER_PORT}
CORE_HOST_PORT=${CORE_HOST_PORT}
ML_CONTAINER_PORT=${ML_CONTAINER_PORT}
ML_HOST_PORT=${ML_HOST_PORT}
OLLAMA_CONTAINER_PORT=${OLLAMA_CONTAINER_PORT}
OLLAMA_HOST_PORT=${OLLAMA_HOST_PORT}
OLLAMA_VOLUME_NAME=${OLLAMA_VOLUME_NAME}
ENABLE_OLLAMA=${OLLAMA_ENABLED}
DATABASE_URL=${DATABASE_URL}
NATS_URL=${NATS_URL}
LLM_PROVIDER=${LLM_PROVIDER}
LLM_MODEL=${LLM_MODEL}
LLM_API_KEY=${LLM_API_KEY}
SMACKEREL_AUTH_TOKEN=${SMACKEREL_AUTH_TOKEN}
HOST_BIND_ADDRESS=${HOST_BIND_ADDRESS}
OLLAMA_URL=${OLLAMA_URL}
OLLAMA_MODEL=${OLLAMA_MODEL}
OLLAMA_VISION_MODEL=${OLLAMA_VISION_MODEL}
EMBEDDING_MODEL=${EMBEDDING_MODEL}
DIGEST_CRON=${DIGEST_CRON}
LOG_LEVEL=${LOG_LEVEL}
PORT=${CORE_CONTAINER_PORT}
ML_SIDECAR_URL=${ML_SIDECAR_URL}
CORE_API_URL=${CORE_API_URL}
TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
TELEGRAM_CHAT_IDS=${TELEGRAM_CHAT_IDS}
TELEGRAM_ASSEMBLY_WINDOW_SECONDS=${TELEGRAM_ASSEMBLY_WINDOW_SECONDS}
TELEGRAM_ASSEMBLY_MAX_MESSAGES=${TELEGRAM_ASSEMBLY_MAX_MESSAGES}
TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS=${TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS}
CORE_EXTERNAL_URL=${CORE_EXTERNAL_URL}
ML_EXTERNAL_URL=${ML_EXTERNAL_URL}
BOOKMARKS_IMPORT_DIR=${BOOKMARKS_IMPORT_DIR}
MAPS_IMPORT_DIR=${MAPS_IMPORT_DIR}
BROWSER_HISTORY_PATH=${BROWSER_HISTORY_PATH}
DISCORD_ENABLED=${DISCORD_ENABLED}
DISCORD_BOT_TOKEN=${DISCORD_BOT_TOKEN}
DISCORD_SYNC_SCHEDULE=${DISCORD_SYNC_SCHEDULE}
DISCORD_ENABLE_GATEWAY=${DISCORD_ENABLE_GATEWAY}
DISCORD_BACKFILL_LIMIT=${DISCORD_BACKFILL_LIMIT}
DISCORD_INCLUDE_THREADS=${DISCORD_INCLUDE_THREADS}
DISCORD_INCLUDE_PINS=${DISCORD_INCLUDE_PINS}
DISCORD_CAPTURE_COMMANDS=${DISCORD_CAPTURE_COMMANDS}
DISCORD_MONITORED_CHANNELS=${DISCORD_MONITORED_CHANNELS}
TWITTER_ENABLED=${TWITTER_ENABLED}
TWITTER_SYNC_MODE=${TWITTER_SYNC_MODE}
TWITTER_ARCHIVE_DIR=${TWITTER_ARCHIVE_DIR}
TWITTER_BEARER_TOKEN=${TWITTER_BEARER_TOKEN}
TWITTER_SYNC_SCHEDULE=${TWITTER_SYNC_SCHEDULE}
WEATHER_ENABLED=${WEATHER_ENABLED}
WEATHER_SYNC_SCHEDULE=${WEATHER_SYNC_SCHEDULE}
WEATHER_LOCATIONS=${WEATHER_LOCATIONS}
GOV_ALERTS_ENABLED=${GOV_ALERTS_ENABLED}
GOV_ALERTS_SYNC_SCHEDULE=${GOV_ALERTS_SYNC_SCHEDULE}
GOV_ALERTS_MIN_EARTHQUAKE_MAG=${GOV_ALERTS_MIN_EARTHQUAKE_MAG}
GOV_ALERTS_SOURCE_WEATHER=${GOV_ALERTS_SOURCE_WEATHER}
GOV_ALERTS_SOURCE_TSUNAMI=${GOV_ALERTS_SOURCE_TSUNAMI}
GOV_ALERTS_SOURCE_VOLCANO=${GOV_ALERTS_SOURCE_VOLCANO}
GOV_ALERTS_SOURCE_WILDFIRE=${GOV_ALERTS_SOURCE_WILDFIRE}
GOV_ALERTS_SOURCE_AIRNOW=${GOV_ALERTS_SOURCE_AIRNOW}
GOV_ALERTS_SOURCE_GDACS=${GOV_ALERTS_SOURCE_GDACS}
GOV_ALERTS_AIRNOW_API_KEY=${GOV_ALERTS_AIRNOW_API_KEY}
GOV_ALERTS_LOCATIONS=${GOV_ALERTS_LOCATIONS}
FINANCIAL_MARKETS_ENABLED=${FINANCIAL_MARKETS_ENABLED}
FINANCIAL_MARKETS_SYNC_SCHEDULE=${FINANCIAL_MARKETS_SYNC_SCHEDULE}
FINANCIAL_MARKETS_FINNHUB_API_KEY=${FINANCIAL_MARKETS_FINNHUB_API_KEY}
FINANCIAL_MARKETS_FRED_API_KEY=${FINANCIAL_MARKETS_FRED_API_KEY}
FINANCIAL_MARKETS_COINGECKO_ENABLED=${FINANCIAL_MARKETS_COINGECKO_ENABLED}
FINANCIAL_MARKETS_ALERT_THRESHOLD=${FINANCIAL_MARKETS_ALERT_THRESHOLD}
FINANCIAL_MARKETS_WATCHLIST=${FINANCIAL_MARKETS_WATCHLIST}
GUESTHOST_ENABLED=${GUESTHOST_ENABLED}
GUESTHOST_BASE_URL=${GUESTHOST_BASE_URL}
GUESTHOST_API_KEY=${GUESTHOST_API_KEY}
GUESTHOST_SYNC_SCHEDULE=${GUESTHOST_SYNC_SCHEDULE}
GUESTHOST_EVENT_TYPES=${GUESTHOST_EVENT_TYPES}
DB_MAX_CONNS=${DB_MAX_CONNS}
DB_MIN_CONNS=${DB_MIN_CONNS}
SHUTDOWN_TIMEOUT_S=${SHUTDOWN_TIMEOUT_S}
ML_HEALTH_CACHE_TTL_S=${ML_HEALTH_CACHE_TTL_S}
EOF

chmod 0600 "$OUTPUT_FILE"
echo "Generated $OUTPUT_FILE"

# Generate NATS config file with resolved auth token and monitor port.
# NATS does not support env var substitution in config files, so values
# are resolved from the SST at generation time.
NATS_CONF_FILE="$REPO_ROOT/config/generated/nats.conf"

# Build NATS auth section only when a token is set
NATS_AUTH_SECTION=""
if [[ -n "$SMACKEREL_AUTH_TOKEN" ]]; then
  NATS_AUTH_SECTION="
authorization {
  token: ${SMACKEREL_AUTH_TOKEN}
}"
fi

NATS_CONF_CONTENT="# Auto-generated from config/smackerel.yaml — DO NOT EDIT DIRECTLY
# Regenerate: ./smackerel.sh config generate

jetstream {
  store_dir: /data
}

http_port: ${NATS_MONITOR_PORT}${NATS_AUTH_SECTION}"

printf '%s\n' "$NATS_CONF_CONTENT" > "$NATS_CONF_FILE"
chmod 0600 "$NATS_CONF_FILE"

echo "Generated $NATS_CONF_FILE"