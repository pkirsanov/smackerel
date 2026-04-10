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
ML_CONTAINER_PORT="$(required_value services.ml.container_port)"
POSTGRES_USER="$(required_value infrastructure.postgres.user)"
POSTGRES_PASSWORD="$(required_value infrastructure.postgres.password)"
POSTGRES_DB="$(required_value infrastructure.postgres.database)"
POSTGRES_CONTAINER_PORT="$(required_value infrastructure.postgres.container_port)"
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
DIGEST_CRON="$(required_value runtime.digest_cron)"
EMBEDDING_MODEL="$(required_value runtime.embedding_model)"
LOG_LEVEL="$(required_value runtime.log_level)"
TELEGRAM_BOT_TOKEN="$(required_value telegram.bot_token)"
TELEGRAM_CHAT_IDS="$(required_value telegram.chat_ids)"

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
CORE_EXTERNAL_URL=${CORE_EXTERNAL_URL}
ML_EXTERNAL_URL=${ML_EXTERNAL_URL}
BOOKMARKS_IMPORT_DIR=${BOOKMARKS_IMPORT_DIR}
MAPS_IMPORT_DIR=${MAPS_IMPORT_DIR}
BROWSER_HISTORY_PATH=${BROWSER_HISTORY_PATH}
EOF

echo "Generated $OUTPUT_FILE"