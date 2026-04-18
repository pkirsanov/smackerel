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
                    # Strip inline YAML comments from the value
                    if val.startswith('"'):
                        close = val.find('"', 1)
                        if close >= 0:
                            val = val[:close + 1]
                    elif val.startswith("'"):
                        close = val.find("'", 1)
                        if close >= 0:
                            val = val[:close + 1]
                    else:
                        ci = val.find(' #')
                        if ci >= 0:
                            val = val[:ci].rstrip()
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
    is_list = False
    for i in range(target_line + 1, len(lines)):
        text = lines[i].rstrip('\n')
        stripped = text.lstrip()
        if not stripped or stripped.startswith('#'):
            continue
        indent = len(text) - len(stripped)
        if target_indent < 0:
            target_indent = indent
            is_list = stripped.startswith('- ')
        if indent < target_indent:
            break
        # Sibling keys at the same indent as list items are not part of the list
        if is_list and indent == target_indent and not stripped.startswith('- '):
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
    scalar_mode = False
    for ln in lines:
        s = ln.lstrip()
        if s.startswith('- '):
            if cur:
                arr.append(cur)
            elif scalar_mode:
                pass  # previous scalar already appended
            cur = {}
            kv = s[2:].strip()
            if ':' in kv:
                k, v = kv.split(':', 1)
                cur[k.strip()] = scalar(v.strip())
                scalar_mode = False
            else:
                # H-019-003: Block-format scalar array item (e.g., "- value")
                arr.append(scalar(kv))
                cur = {}
                scalar_mode = True
        elif ':' in s:
            k, v = s.split(':', 1)
            cur[k.strip()] = scalar(v.strip())
            scalar_mode = False
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
    # Strip inline YAML comments before processing
    if v:
        if v.startswith('"'):
            close = v.find('"', 1)
            if close >= 0:
                v = v[:close + 1]
        elif v.startswith("'"):
            close = v.find("'", 1)
            if close >= 0:
                v = v[:close + 1]
        else:
            ci = v.find(' #')
            if ci >= 0:
                v = v[:ci].rstrip()
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
ML_READINESS_TIMEOUT_S="$(required_value services.ml.readiness_timeout_s)"
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
TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS="$(yaml_get telegram.disambiguation_timeout_seconds 2>/dev/null)" || TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS="120"
TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES="$(required_value telegram.cook_session_timeout_minutes)"
TELEGRAM_COOK_SESSION_MAX_PER_CHAT="$(required_value telegram.cook_session_max_per_chat)"

# Expense tracking
EXPENSES_ENABLED="$(yaml_get expenses.enabled 2>/dev/null)" || EXPENSES_ENABLED="false"
EXPENSES_DEFAULT_CURRENCY="$(yaml_get expenses.default_currency 2>/dev/null)" || EXPENSES_DEFAULT_CURRENCY=""
EXPENSES_EXPORT_MAX_ROWS="$(yaml_get expenses.export.max_rows 2>/dev/null)" || EXPENSES_EXPORT_MAX_ROWS=""
EXPENSES_EXPORT_QB_DATE_FORMAT="$(yaml_get expenses.export.quickbooks_date_format 2>/dev/null)" || EXPENSES_EXPORT_QB_DATE_FORMAT=""
EXPENSES_EXPORT_STD_DATE_FORMAT="$(yaml_get expenses.export.standard_date_format 2>/dev/null)" || EXPENSES_EXPORT_STD_DATE_FORMAT=""
EXPENSES_SUGGESTIONS_MIN_CONFIDENCE="$(yaml_get expenses.suggestions.min_confidence 2>/dev/null)" || EXPENSES_SUGGESTIONS_MIN_CONFIDENCE=""
EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS="$(yaml_get expenses.suggestions.min_past_business_count 2>/dev/null)" || EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS=""
EXPENSES_SUGGESTIONS_MAX_PER_DIGEST="$(yaml_get expenses.suggestions.max_per_digest 2>/dev/null)" || EXPENSES_SUGGESTIONS_MAX_PER_DIGEST=""
EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT="$(yaml_get expenses.suggestions.reclassify_batch_limit 2>/dev/null)" || EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT=""
EXPENSES_VENDOR_CACHE_SIZE="$(yaml_get expenses.vendor_cache_size 2>/dev/null)" || EXPENSES_VENDOR_CACHE_SIZE=""
EXPENSES_DIGEST_MAX_WORDS="$(yaml_get expenses.digest.max_words 2>/dev/null)" || EXPENSES_DIGEST_MAX_WORDS=""
EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT="$(yaml_get expenses.digest.needs_review_limit 2>/dev/null)" || EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT=""
EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS="$(yaml_get expenses.digest.missing_receipt_lookback_days 2>/dev/null)" || EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS=""
IMAP_EXPENSE_LABELS="$(yaml_get_json connectors.imap.expense_labels 2>/dev/null)" || IMAP_EXPENSE_LABELS="{}"
EXPENSES_BUSINESS_VENDORS="$(yaml_get_json expenses.business_vendors 2>/dev/null)" || EXPENSES_BUSINESS_VENDORS="[]"
EXPENSES_CATEGORIES="$(yaml_get_json expenses.categories 2>/dev/null)" || EXPENSES_CATEGORIES="[]"

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

# OAuth connector sync schedules (Phase 2 — IMAP, CalDAV, YouTube)
IMAP_SYNC_SCHEDULE="$(yaml_get connectors.imap.sync_schedule 2>/dev/null)" || IMAP_SYNC_SCHEDULE=""
CALDAV_SYNC_SCHEDULE="$(yaml_get connectors.caldav.sync_schedule 2>/dev/null)" || CALDAV_SYNC_SCHEDULE=""
YOUTUBE_SYNC_SCHEDULE="$(yaml_get connectors.youtube.sync_schedule 2>/dev/null)" || YOUTUBE_SYNC_SCHEDULE=""

# Connector import paths (optional — empty string is valid default)
BOOKMARKS_ENABLED="$(yaml_get connectors.bookmarks.enabled 2>/dev/null)" || BOOKMARKS_ENABLED="false"
BOOKMARKS_SYNC_SCHEDULE="$(yaml_get connectors.bookmarks.sync_schedule 2>/dev/null)" || BOOKMARKS_SYNC_SCHEDULE=""
BOOKMARKS_IMPORT_DIR="$(yaml_get connectors.bookmarks.import_dir 2>/dev/null)" || BOOKMARKS_IMPORT_DIR=""
MAPS_ENABLED="$(yaml_get connectors.google-maps-timeline.enabled 2>/dev/null)" || MAPS_ENABLED="false"
MAPS_SYNC_SCHEDULE="$(yaml_get connectors.google-maps-timeline.sync_schedule 2>/dev/null)" || MAPS_SYNC_SCHEDULE=""
MAPS_IMPORT_DIR="$(yaml_get connectors.google-maps-timeline.import_dir 2>/dev/null)" || MAPS_IMPORT_DIR=""
MAPS_WATCH_INTERVAL="$(yaml_get connectors.google-maps-timeline.watch_interval 2>/dev/null)" || MAPS_WATCH_INTERVAL=""
MAPS_ARCHIVE_PROCESSED="$(yaml_get connectors.google-maps-timeline.archive_processed 2>/dev/null)" || MAPS_ARCHIVE_PROCESSED=""
MAPS_MIN_DISTANCE_M="$(yaml_get connectors.google-maps-timeline.min_distance_m 2>/dev/null)" || MAPS_MIN_DISTANCE_M=""
MAPS_MIN_DURATION_MIN="$(yaml_get connectors.google-maps-timeline.min_duration_min 2>/dev/null)" || MAPS_MIN_DURATION_MIN=""
MAPS_LOCATION_RADIUS_M="$(yaml_get connectors.google-maps-timeline.clustering.location_radius_m 2>/dev/null)" || MAPS_LOCATION_RADIUS_M=""
MAPS_HOME_DETECTION="$(yaml_get connectors.google-maps-timeline.clustering.home_detection 2>/dev/null)" || MAPS_HOME_DETECTION=""
MAPS_COMMUTE_MIN_OCCURRENCES="$(yaml_get connectors.google-maps-timeline.commute.min_occurrences 2>/dev/null)" || MAPS_COMMUTE_MIN_OCCURRENCES=""
MAPS_COMMUTE_WINDOW_DAYS="$(yaml_get connectors.google-maps-timeline.commute.window_days 2>/dev/null)" || MAPS_COMMUTE_WINDOW_DAYS=""
MAPS_COMMUTE_WEEKDAYS_ONLY="$(yaml_get connectors.google-maps-timeline.commute.weekdays_only 2>/dev/null)" || MAPS_COMMUTE_WEEKDAYS_ONLY=""
MAPS_TRIP_MIN_DISTANCE_KM="$(yaml_get connectors.google-maps-timeline.trip.min_distance_km 2>/dev/null)" || MAPS_TRIP_MIN_DISTANCE_KM=""
MAPS_TRIP_MIN_OVERNIGHT_HOURS="$(yaml_get connectors.google-maps-timeline.trip.min_overnight_hours 2>/dev/null)" || MAPS_TRIP_MIN_OVERNIGHT_HOURS=""
MAPS_LINK_TIME_EXTEND_MIN="$(yaml_get connectors.google-maps-timeline.linking.time_extend_min 2>/dev/null)" || MAPS_LINK_TIME_EXTEND_MIN=""
MAPS_LINK_PROXIMITY_RADIUS_M="$(yaml_get connectors.google-maps-timeline.linking.proximity_radius_m 2>/dev/null)" || MAPS_LINK_PROXIMITY_RADIUS_M=""
BROWSER_HISTORY_ENABLED="$(yaml_get connectors.browser-history.enabled 2>/dev/null)" || BROWSER_HISTORY_ENABLED="false"
BROWSER_HISTORY_SYNC_SCHEDULE="$(yaml_get connectors.browser-history.sync_schedule 2>/dev/null)" || BROWSER_HISTORY_SYNC_SCHEDULE=""
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
WEATHER_ENABLE_ALERTS="$(yaml_get connectors.weather.enable_alerts 2>/dev/null)" || WEATHER_ENABLE_ALERTS="false"
WEATHER_FORECAST_DAYS="$(yaml_get connectors.weather.forecast_days 2>/dev/null)" || WEATHER_FORECAST_DAYS=""
WEATHER_PRECISION="$(yaml_get connectors.weather.precision 2>/dev/null)" || WEATHER_PRECISION=""
WEATHER_LOCATIONS="$(yaml_get_json connectors.weather.locations 2>/dev/null)" || WEATHER_LOCATIONS=""

# Gov Alerts connector
GOV_ALERTS_ENABLED="$(yaml_get connectors.gov-alerts.enabled 2>/dev/null)" || GOV_ALERTS_ENABLED="false"
GOV_ALERTS_SYNC_SCHEDULE="$(yaml_get connectors.gov-alerts.sync_schedule 2>/dev/null)" || GOV_ALERTS_SYNC_SCHEDULE=""
GOV_ALERTS_MIN_EARTHQUAKE_MAG="$(yaml_get connectors.gov-alerts.min_earthquake_magnitude 2>/dev/null)" || GOV_ALERTS_MIN_EARTHQUAKE_MAG=""
GOV_ALERTS_SOURCE_EARTHQUAKE="$(yaml_get connectors.gov-alerts.source_earthquake 2>/dev/null)" || GOV_ALERTS_SOURCE_EARTHQUAKE=""
GOV_ALERTS_SOURCE_WEATHER="$(yaml_get connectors.gov-alerts.source_weather 2>/dev/null)" || GOV_ALERTS_SOURCE_WEATHER=""
GOV_ALERTS_SOURCE_TSUNAMI="$(yaml_get connectors.gov-alerts.source_tsunami 2>/dev/null)" || GOV_ALERTS_SOURCE_TSUNAMI=""
GOV_ALERTS_SOURCE_VOLCANO="$(yaml_get connectors.gov-alerts.source_volcano 2>/dev/null)" || GOV_ALERTS_SOURCE_VOLCANO=""
GOV_ALERTS_SOURCE_WILDFIRE="$(yaml_get connectors.gov-alerts.source_wildfire 2>/dev/null)" || GOV_ALERTS_SOURCE_WILDFIRE=""
GOV_ALERTS_SOURCE_AIRNOW="$(yaml_get connectors.gov-alerts.source_airnow 2>/dev/null)" || GOV_ALERTS_SOURCE_AIRNOW=""
GOV_ALERTS_SOURCE_GDACS="$(yaml_get connectors.gov-alerts.source_gdacs 2>/dev/null)" || GOV_ALERTS_SOURCE_GDACS=""
GOV_ALERTS_AIRNOW_API_KEY="$(yaml_get connectors.gov-alerts.airnow_api_key 2>/dev/null)" || GOV_ALERTS_AIRNOW_API_KEY=""
GOV_ALERTS_LOCATIONS="$(yaml_get_json connectors.gov-alerts.locations 2>/dev/null)" || GOV_ALERTS_LOCATIONS=""
GOV_ALERTS_TRAVEL_LOCATIONS="$(yaml_get_json connectors.gov-alerts.travel_locations 2>/dev/null)" || GOV_ALERTS_TRAVEL_LOCATIONS=""

# Financial Markets connector
FINANCIAL_MARKETS_ENABLED="$(yaml_get connectors.financial-markets.enabled 2>/dev/null)" || FINANCIAL_MARKETS_ENABLED="false"
FINANCIAL_MARKETS_SYNC_SCHEDULE="$(yaml_get connectors.financial-markets.sync_schedule 2>/dev/null)" || FINANCIAL_MARKETS_SYNC_SCHEDULE=""
FINANCIAL_MARKETS_FINNHUB_API_KEY="$(yaml_get connectors.financial-markets.finnhub_api_key 2>/dev/null)" || FINANCIAL_MARKETS_FINNHUB_API_KEY=""
FINANCIAL_MARKETS_FRED_API_KEY="$(yaml_get connectors.financial-markets.fred_api_key 2>/dev/null)" || FINANCIAL_MARKETS_FRED_API_KEY=""
FINANCIAL_MARKETS_FRED_ENABLED="$(yaml_get connectors.financial-markets.fred_enabled 2>/dev/null)" || FINANCIAL_MARKETS_FRED_ENABLED=""
FINANCIAL_MARKETS_FRED_SERIES="$(yaml_get_json connectors.financial-markets.fred_series 2>/dev/null)" || FINANCIAL_MARKETS_FRED_SERIES=""
FINANCIAL_MARKETS_COINGECKO_ENABLED="$(yaml_get connectors.financial-markets.coingecko_enabled 2>/dev/null)" || FINANCIAL_MARKETS_COINGECKO_ENABLED=""
FINANCIAL_MARKETS_ALERT_THRESHOLD="$(yaml_get connectors.financial-markets.alert_threshold 2>/dev/null)" || FINANCIAL_MARKETS_ALERT_THRESHOLD=""
FINANCIAL_MARKETS_WATCHLIST="$(yaml_get_json connectors.financial-markets.watchlist 2>/dev/null)" || FINANCIAL_MARKETS_WATCHLIST=""

# Knowledge layer
KNOWLEDGE_ENABLED="$(required_value knowledge.enabled)"

# Meal planning
MEAL_PLANNING_ENABLED="$(yaml_get meal_planning.enabled 2>/dev/null)" || MEAL_PLANNING_ENABLED="false"
MEAL_PLANNING_DEFAULT_SERVINGS="$(yaml_get meal_planning.default_servings 2>/dev/null)" || MEAL_PLANNING_DEFAULT_SERVINGS=""
MEAL_PLANNING_MEAL_TYPES="$(yaml_get meal_planning.meal_types 2>/dev/null)" || MEAL_PLANNING_MEAL_TYPES=""
MEAL_PLANNING_MEAL_TIME_BREAKFAST="$(yaml_get meal_planning.meal_times.breakfast 2>/dev/null)" || MEAL_PLANNING_MEAL_TIME_BREAKFAST=""
MEAL_PLANNING_MEAL_TIME_LUNCH="$(yaml_get meal_planning.meal_times.lunch 2>/dev/null)" || MEAL_PLANNING_MEAL_TIME_LUNCH=""
MEAL_PLANNING_MEAL_TIME_DINNER="$(yaml_get meal_planning.meal_times.dinner 2>/dev/null)" || MEAL_PLANNING_MEAL_TIME_DINNER=""
MEAL_PLANNING_MEAL_TIME_SNACK="$(yaml_get meal_planning.meal_times.snack 2>/dev/null)" || MEAL_PLANNING_MEAL_TIME_SNACK=""
MEAL_PLANNING_CALENDAR_SYNC="$(yaml_get meal_planning.calendar_sync 2>/dev/null)" || MEAL_PLANNING_CALENDAR_SYNC="false"
MEAL_PLANNING_AUTO_COMPLETE="$(yaml_get meal_planning.auto_complete_past_plans 2>/dev/null)" || MEAL_PLANNING_AUTO_COMPLETE="false"
MEAL_PLANNING_AUTO_COMPLETE_CRON="$(yaml_get meal_planning.auto_complete_cron 2>/dev/null)" || MEAL_PLANNING_AUTO_COMPLETE_CRON=""
KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS="$(required_value knowledge.synthesis_timeout_seconds)"
KNOWLEDGE_LINT_CRON="$(required_value knowledge.lint_cron)"
KNOWLEDGE_LINT_STALE_DAYS="$(required_value knowledge.lint_stale_days)"
KNOWLEDGE_CONCEPT_MAX_TOKENS="$(required_value knowledge.concept_max_tokens)"
KNOWLEDGE_CONCEPT_SEARCH_THRESHOLD="$(required_value knowledge.concept_search_threshold)"
KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD="$(required_value knowledge.cross_source_confidence_threshold)"
KNOWLEDGE_MAX_SYNTHESIS_RETRIES="$(required_value knowledge.max_synthesis_retries)"
KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS="$(required_value knowledge.prompt_contracts.ingest_synthesis)"
KNOWLEDGE_PROMPT_CONTRACT_CROSS_SOURCE="$(required_value knowledge.prompt_contracts.cross_source_connection)"
KNOWLEDGE_PROMPT_CONTRACT_LINT_AUDIT="$(required_value knowledge.prompt_contracts.lint_audit)"
KNOWLEDGE_PROMPT_CONTRACT_QUERY_AUGMENT="$(required_value knowledge.prompt_contracts.query_augment)"
KNOWLEDGE_PROMPT_CONTRACT_DIGEST_ASSEMBLY="$(required_value knowledge.prompt_contracts.digest_assembly)"
PROMPT_CONTRACTS_DIR="$(required_value knowledge.prompt_contracts_dir)"

# Observability
OTEL_ENABLED="$(yaml_get observability.otel_enabled 2>/dev/null)" || OTEL_ENABLED="false"
OTEL_EXPORTER_ENDPOINT="$(yaml_get observability.otel_exporter_endpoint 2>/dev/null)" || OTEL_EXPORTER_ENDPOINT=""

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
TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS=${TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS}
TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES=${TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES}
TELEGRAM_COOK_SESSION_MAX_PER_CHAT=${TELEGRAM_COOK_SESSION_MAX_PER_CHAT}
CORE_EXTERNAL_URL=${CORE_EXTERNAL_URL}
ML_EXTERNAL_URL=${ML_EXTERNAL_URL}
BOOKMARKS_ENABLED=${BOOKMARKS_ENABLED}
BOOKMARKS_SYNC_SCHEDULE=${BOOKMARKS_SYNC_SCHEDULE}
BOOKMARKS_IMPORT_DIR=${BOOKMARKS_IMPORT_DIR}
MAPS_ENABLED=${MAPS_ENABLED}
MAPS_SYNC_SCHEDULE=${MAPS_SYNC_SCHEDULE}
MAPS_IMPORT_DIR=${MAPS_IMPORT_DIR}
MAPS_WATCH_INTERVAL=${MAPS_WATCH_INTERVAL}
MAPS_ARCHIVE_PROCESSED=${MAPS_ARCHIVE_PROCESSED}
MAPS_MIN_DISTANCE_M=${MAPS_MIN_DISTANCE_M}
MAPS_MIN_DURATION_MIN=${MAPS_MIN_DURATION_MIN}
MAPS_LOCATION_RADIUS_M=${MAPS_LOCATION_RADIUS_M}
MAPS_HOME_DETECTION=${MAPS_HOME_DETECTION}
MAPS_COMMUTE_MIN_OCCURRENCES=${MAPS_COMMUTE_MIN_OCCURRENCES}
MAPS_COMMUTE_WINDOW_DAYS=${MAPS_COMMUTE_WINDOW_DAYS}
MAPS_COMMUTE_WEEKDAYS_ONLY=${MAPS_COMMUTE_WEEKDAYS_ONLY}
MAPS_TRIP_MIN_DISTANCE_KM=${MAPS_TRIP_MIN_DISTANCE_KM}
MAPS_TRIP_MIN_OVERNIGHT_HOURS=${MAPS_TRIP_MIN_OVERNIGHT_HOURS}
MAPS_LINK_TIME_EXTEND_MIN=${MAPS_LINK_TIME_EXTEND_MIN}
MAPS_LINK_PROXIMITY_RADIUS_M=${MAPS_LINK_PROXIMITY_RADIUS_M}
BROWSER_HISTORY_ENABLED=${BROWSER_HISTORY_ENABLED}
BROWSER_HISTORY_SYNC_SCHEDULE=${BROWSER_HISTORY_SYNC_SCHEDULE}
BROWSER_HISTORY_PATH=${BROWSER_HISTORY_PATH}
IMAP_SYNC_SCHEDULE=${IMAP_SYNC_SCHEDULE}
CALDAV_SYNC_SCHEDULE=${CALDAV_SYNC_SCHEDULE}
YOUTUBE_SYNC_SCHEDULE=${YOUTUBE_SYNC_SCHEDULE}
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
WEATHER_ENABLE_ALERTS=${WEATHER_ENABLE_ALERTS}
WEATHER_FORECAST_DAYS=${WEATHER_FORECAST_DAYS}
WEATHER_PRECISION=${WEATHER_PRECISION}
WEATHER_LOCATIONS=${WEATHER_LOCATIONS}
GOV_ALERTS_ENABLED=${GOV_ALERTS_ENABLED}
GOV_ALERTS_SYNC_SCHEDULE=${GOV_ALERTS_SYNC_SCHEDULE}
GOV_ALERTS_MIN_EARTHQUAKE_MAG=${GOV_ALERTS_MIN_EARTHQUAKE_MAG}
GOV_ALERTS_SOURCE_EARTHQUAKE=${GOV_ALERTS_SOURCE_EARTHQUAKE}
GOV_ALERTS_SOURCE_WEATHER=${GOV_ALERTS_SOURCE_WEATHER}
GOV_ALERTS_SOURCE_TSUNAMI=${GOV_ALERTS_SOURCE_TSUNAMI}
GOV_ALERTS_SOURCE_VOLCANO=${GOV_ALERTS_SOURCE_VOLCANO}
GOV_ALERTS_SOURCE_WILDFIRE=${GOV_ALERTS_SOURCE_WILDFIRE}
GOV_ALERTS_SOURCE_AIRNOW=${GOV_ALERTS_SOURCE_AIRNOW}
GOV_ALERTS_SOURCE_GDACS=${GOV_ALERTS_SOURCE_GDACS}
GOV_ALERTS_AIRNOW_API_KEY=${GOV_ALERTS_AIRNOW_API_KEY}
GOV_ALERTS_LOCATIONS=${GOV_ALERTS_LOCATIONS}
GOV_ALERTS_TRAVEL_LOCATIONS=${GOV_ALERTS_TRAVEL_LOCATIONS}
FINANCIAL_MARKETS_ENABLED=${FINANCIAL_MARKETS_ENABLED}
FINANCIAL_MARKETS_SYNC_SCHEDULE=${FINANCIAL_MARKETS_SYNC_SCHEDULE}
FINANCIAL_MARKETS_FINNHUB_API_KEY=${FINANCIAL_MARKETS_FINNHUB_API_KEY}
FINANCIAL_MARKETS_FRED_API_KEY=${FINANCIAL_MARKETS_FRED_API_KEY}
FINANCIAL_MARKETS_FRED_ENABLED=${FINANCIAL_MARKETS_FRED_ENABLED}
FINANCIAL_MARKETS_FRED_SERIES=${FINANCIAL_MARKETS_FRED_SERIES}
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
ML_READINESS_TIMEOUT_S=${ML_READINESS_TIMEOUT_S}
KNOWLEDGE_ENABLED=${KNOWLEDGE_ENABLED}
KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS=${KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS}
KNOWLEDGE_LINT_CRON=${KNOWLEDGE_LINT_CRON}
KNOWLEDGE_LINT_STALE_DAYS=${KNOWLEDGE_LINT_STALE_DAYS}
KNOWLEDGE_CONCEPT_MAX_TOKENS=${KNOWLEDGE_CONCEPT_MAX_TOKENS}
KNOWLEDGE_CONCEPT_SEARCH_THRESHOLD=${KNOWLEDGE_CONCEPT_SEARCH_THRESHOLD}
KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD=${KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD}
KNOWLEDGE_MAX_SYNTHESIS_RETRIES=${KNOWLEDGE_MAX_SYNTHESIS_RETRIES}
KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS=${KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS}
KNOWLEDGE_PROMPT_CONTRACT_CROSS_SOURCE=${KNOWLEDGE_PROMPT_CONTRACT_CROSS_SOURCE}
KNOWLEDGE_PROMPT_CONTRACT_LINT_AUDIT=${KNOWLEDGE_PROMPT_CONTRACT_LINT_AUDIT}
KNOWLEDGE_PROMPT_CONTRACT_QUERY_AUGMENT=${KNOWLEDGE_PROMPT_CONTRACT_QUERY_AUGMENT}
KNOWLEDGE_PROMPT_CONTRACT_DIGEST_ASSEMBLY=${KNOWLEDGE_PROMPT_CONTRACT_DIGEST_ASSEMBLY}
PROMPT_CONTRACTS_DIR=${PROMPT_CONTRACTS_DIR}
OTEL_ENABLED=${OTEL_ENABLED}
OTEL_EXPORTER_ENDPOINT=${OTEL_EXPORTER_ENDPOINT}
EXPENSES_ENABLED=${EXPENSES_ENABLED}
EXPENSES_DEFAULT_CURRENCY=${EXPENSES_DEFAULT_CURRENCY}
EXPENSES_EXPORT_MAX_ROWS=${EXPENSES_EXPORT_MAX_ROWS}
EXPENSES_EXPORT_QB_DATE_FORMAT=${EXPENSES_EXPORT_QB_DATE_FORMAT}
EXPENSES_EXPORT_STD_DATE_FORMAT=${EXPENSES_EXPORT_STD_DATE_FORMAT}
EXPENSES_SUGGESTIONS_MIN_CONFIDENCE=${EXPENSES_SUGGESTIONS_MIN_CONFIDENCE}
EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS=${EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS}
EXPENSES_SUGGESTIONS_MAX_PER_DIGEST=${EXPENSES_SUGGESTIONS_MAX_PER_DIGEST}
EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT=${EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT}
EXPENSES_VENDOR_CACHE_SIZE=${EXPENSES_VENDOR_CACHE_SIZE}
EXPENSES_DIGEST_MAX_WORDS=${EXPENSES_DIGEST_MAX_WORDS}
EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT=${EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT}
EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS=${EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS}
IMAP_EXPENSE_LABELS=${IMAP_EXPENSE_LABELS}
EXPENSES_BUSINESS_VENDORS=${EXPENSES_BUSINESS_VENDORS}
EXPENSES_CATEGORIES=${EXPENSES_CATEGORIES}
MEAL_PLANNING_ENABLED=${MEAL_PLANNING_ENABLED}
MEAL_PLANNING_DEFAULT_SERVINGS=${MEAL_PLANNING_DEFAULT_SERVINGS}
MEAL_PLANNING_MEAL_TYPES=${MEAL_PLANNING_MEAL_TYPES}
MEAL_PLANNING_MEAL_TIME_BREAKFAST=${MEAL_PLANNING_MEAL_TIME_BREAKFAST}
MEAL_PLANNING_MEAL_TIME_LUNCH=${MEAL_PLANNING_MEAL_TIME_LUNCH}
MEAL_PLANNING_MEAL_TIME_DINNER=${MEAL_PLANNING_MEAL_TIME_DINNER}
MEAL_PLANNING_MEAL_TIME_SNACK=${MEAL_PLANNING_MEAL_TIME_SNACK}
MEAL_PLANNING_CALENDAR_SYNC=${MEAL_PLANNING_CALENDAR_SYNC}
MEAL_PLANNING_AUTO_COMPLETE=${MEAL_PLANNING_AUTO_COMPLETE}
MEAL_PLANNING_AUTO_COMPLETE_CRON=${MEAL_PLANNING_AUTO_COMPLETE_CRON}
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
  # Escape backslash and double-quote inside the token value for NATS
  # double-quoted string syntax. Without this, a token containing " or \
  # corrupts the config file or silently disables authentication (CWE-74).
  ESCAPED_NATS_TOKEN="${SMACKEREL_AUTH_TOKEN//\\/\\\\}"
  ESCAPED_NATS_TOKEN="${ESCAPED_NATS_TOKEN//\"/\\\"}"
  NATS_AUTH_SECTION="
authorization {
  token: \"${ESCAPED_NATS_TOKEN}\"
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