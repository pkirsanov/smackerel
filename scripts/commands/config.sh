#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG_FILE="$REPO_ROOT/config/smackerel.yaml"
TARGET_ENV="dev"
EMIT_BUNDLE=false
BUNDLE_OUTPUT_DIR=""
BUNDLE_SOURCE_SHA=""

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
    --bundle)
      EMIT_BUNDLE=true
      shift
      ;;
    --output-dir)
      BUNDLE_OUTPUT_DIR="$2"
      shift 2
      ;;
    --output-dir=*)
      BUNDLE_OUTPUT_DIR="${1#*=}"
      shift
      ;;
    --source-sha)
      BUNDLE_SOURCE_SHA="$2"
      shift 2
      ;;
    --source-sha=*)
      BUNDLE_SOURCE_SHA="${1#*=}"
      shift
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

if [[ "$EMIT_BUNDLE" == "true" ]]; then
  [[ -n "$BUNDLE_OUTPUT_DIR" ]] || BUNDLE_OUTPUT_DIR="$REPO_ROOT/dist/config-bundles"
  if [[ -z "$BUNDLE_SOURCE_SHA" ]]; then
    BUNDLE_SOURCE_SHA="$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || echo "")"
  fi
  [[ -n "$BUNDLE_SOURCE_SHA" ]] || {
    echo "ERROR: --bundle requires --source-sha=<sha> (or a git checkout to derive HEAD)" >&2
    exit 1
  }
fi

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

required_json_value() {
  local key="$1"
  local value

  value="$(yaml_get_json "$key")"
  if [[ -z "$value" ]]; then
    echo "Missing config key: $key" >&2
    exit 1
  fi

  printf '%s' "$value"
}

env_override_value() {
  local override_key="$1"
  local base_key="$2"
  local value

  if value="$(yaml_get "environments.$TARGET_ENV.$override_key" 2>/dev/null)"; then
    printf '%s' "$value"
    return
  fi

  required_value "$base_key"
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
                        print(json.dumps(scalar(val), separators=(',', ':')))
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
    print(json.dumps(result, separators=(',', ':')))

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
  dev|test|home-lab)
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
ML_PROCESSING_DEGRADED_FALLBACK_ENABLED="$(env_override_value ml_processing_degraded_fallback_enabled services.ml.processing_degraded_fallback_enabled)"

# Spec 045 FR-045-001 / FR-045-002 — deploy resource envelope + ML model
# memory profile extraction. Every key is required (fail-loud SST). The
# memory limit values are emitted both with their `compose-style` string
# (e.g. "1G", "512M") for direct compose substitution AND in MiB integer
# form for the Go core ML model envelope check. The envelope check is
# wired in internal/config/config.go::validateMLModelEnvelope.
POSTGRES_CPU_LIMIT="$(required_value deploy_resources.postgres.cpus)"
POSTGRES_MEMORY_LIMIT="$(required_value deploy_resources.postgres.memory)"
NATS_CPU_LIMIT="$(required_value deploy_resources.nats.cpus)"
NATS_MEMORY_LIMIT="$(required_value deploy_resources.nats.memory)"
CORE_CPU_LIMIT="$(required_value deploy_resources.smackerel_core.cpus)"
CORE_MEMORY_LIMIT="$(required_value deploy_resources.smackerel_core.memory)"
ML_CPU_LIMIT="$(required_value deploy_resources.smackerel_ml.cpus)"
ML_MEMORY_LIMIT="$(required_value deploy_resources.smackerel_ml.memory)"
OLLAMA_CPU_LIMIT="$(required_value deploy_resources.ollama.cpus)"
OLLAMA_MEMORY_LIMIT="$(required_value deploy_resources.ollama.memory)"
ML_MODEL_MEMORY_PROFILES_JSON="$(required_json_value services.ml.model_memory_profiles)"

POSTGRES_USER="$(required_value infrastructure.postgres.user)"
POSTGRES_PASSWORD="$(required_value infrastructure.postgres.password)"
# Spec 051 FR-051-005 / SCN-051-S02 — defense-in-depth dev-default rejection.
# When the SST loader runs for a non-dev/test target (currently: home-lab; any
# future production-class target should be added to the case below), the
# Postgres password MUST NOT match a known dev-default value. The list below
# is the parallel grep-friendly mirror of internal/config/secrets.go's
# DevDBPasswords slice. Keep the two lists in sync.
#
# The error message MUST name the offending KEY without echoing the VALUE
# (FR-051-007 redaction contract).
case "$TARGET_ENV" in
  home-lab)
    case "$(printf '%s' "$POSTGRES_PASSWORD" | tr '[:upper:]' '[:lower:]')" in
      smackerel|postgres|password|changeme|change-me|default)
        echo "ERROR: infrastructure.postgres.password is a known dev-default value — refusing to generate config for TARGET_ENV=$TARGET_ENV (spec 051 FR-051-005). Set a strong random password in config/smackerel.yaml or via the POSTGRES_PASSWORD env override before running config generate." >&2
        exit 1
        ;;
    esac
    ;;
esac
POSTGRES_DB="$(required_value infrastructure.postgres.database)"
POSTGRES_CONTAINER_PORT="$(required_value infrastructure.postgres.container_port)"
DB_MAX_CONNS="$(required_value infrastructure.postgres.max_conns)"
DB_MIN_CONNS="$(required_value infrastructure.postgres.min_conns)"
NATS_CLIENT_PORT="$(required_value infrastructure.nats.client_port)"
NATS_MONITOR_PORT="$(required_value infrastructure.nats.monitor_port)"
# Spec 046 FR-046-001 — ML sidecar reconnect contract (SST-owned).
NATS_MAX_RECONNECT_ATTEMPTS="$(required_value infrastructure.nats.client.max_reconnect_attempts)"
NATS_RECONNECT_TIME_WAIT_SECONDS="$(required_value infrastructure.nats.client.reconnect_time_wait_seconds)"
# Spec 046 FR-046-002 — NATS server payload + JetStream storage ceilings.
NATS_MAX_PAYLOAD_BYTES="$(required_value infrastructure.nats.max_payload_bytes)"
NATS_MAX_FILE_STORE_BYTES="$(required_value infrastructure.nats.max_file_store_bytes)"
NATS_MAX_MEM_STORE_BYTES="$(required_value infrastructure.nats.max_mem_store_bytes)"
# Spec 046 FR-046-003 — per-stream MaxBytes ceilings (JSON list).
NATS_STREAM_MAX_BYTES_JSON="$(required_json_value infrastructure.nats.stream_max_bytes)"
# Spec 043 — Ollama enabled flag uses per-env override so dev/test/home-lab can
# differ. infrastructure.ollama.enabled remains as the legacy fallback when no
# per-env value is set.
OLLAMA_ENABLED="$(env_override_value ollama_enabled infrastructure.ollama.enabled)"
OLLAMA_CONTAINER_PORT="$(required_value infrastructure.ollama.container_port)"
# Spec 043 — Ollama image identifier hoisted out of docker-compose.yml. The test
# image is pinned to a digest at scope-execution time per design.md OQ-D1; the
# initial pin matches the dev image. Test env wins when TARGET_ENV=test; the
# extra OLLAMA_TEST_* keys are emitted only for the test env (empty otherwise).
if [[ "$TARGET_ENV" == "test" ]]; then
  OLLAMA_IMAGE="$(required_value infrastructure.ollama.test.image)"
  OLLAMA_TEST_MODEL="$(required_value infrastructure.ollama.test.model)"
  OLLAMA_TEST_PULL_TIMEOUT_SECONDS="$(required_value infrastructure.ollama.test.pull_timeout_seconds)"
  OLLAMA_TEST_REQUEST_TEMPERATURE="$(required_value infrastructure.ollama.test.request_temperature)"
  OLLAMA_TEST_REQUEST_TOP_P="$(required_value infrastructure.ollama.test.request_top_p)"
  OLLAMA_TEST_REQUEST_TOP_K="$(required_value infrastructure.ollama.test.request_top_k)"
  OLLAMA_TEST_REQUEST_SEED="$(required_value infrastructure.ollama.test.request_seed)"
  OLLAMA_TEST_REQUEST_NUM_PREDICT="$(required_value infrastructure.ollama.test.request_num_predict)"
else
  OLLAMA_IMAGE="$(required_value infrastructure.ollama.image)"
  OLLAMA_TEST_MODEL=""
  OLLAMA_TEST_PULL_TIMEOUT_SECONDS=""
  OLLAMA_TEST_REQUEST_TEMPERATURE=""
  OLLAMA_TEST_REQUEST_TOP_P=""
  OLLAMA_TEST_REQUEST_TOP_K=""
  OLLAMA_TEST_REQUEST_SEED=""
  OLLAMA_TEST_REQUEST_NUM_PREDICT=""
fi
LLM_PROVIDER="$(required_value llm.provider)"
LLM_MODEL="$(required_value llm.model)"
LLM_API_KEY="$(required_value llm.api_key)"
OLLAMA_URL="$(required_value llm.ollama_url)"
OLLAMA_MODEL="$(required_value llm.ollama_model)"
OLLAMA_VISION_MODEL="$(required_value llm.ollama_vision_model)"
SMACKEREL_AUTH_TOKEN="$(required_value runtime.auth_token)"
# Auto-generate a disposable test token when the SST value is empty and TARGET_ENV=test.
# Dev/prod environments still require manual configuration (fail-loud at service startup).
#
# Reuse the existing token from the previously-generated env file when present so the
# token stays stable across multiple `./smackerel.sh config generate` calls within the
# same session. Test orchestration (e.g. the `test e2e` Go block, `test integration`,
# and `test stress`) reads SMACKEREL_AUTH_TOKEN from the env file once and then calls
# `./smackerel.sh up`, which regenerates the env file before bringing the stack up.
# Without reuse, the regen produces a fresh random token, the running stack is configured
# with the new token, and the previously-captured token used by the Go test container
# returns 401 UNAUTHORIZED on every API call.
if [[ "$TARGET_ENV" == "test" && -z "$SMACKEREL_AUTH_TOKEN" ]]; then
  EXISTING_ENV_FILE="$REPO_ROOT/config/generated/${TARGET_ENV}.env"
  EXISTING_TOKEN=""
  if [[ -f "$EXISTING_ENV_FILE" ]]; then
    EXISTING_TOKEN="$(awk -F= '/^SMACKEREL_AUTH_TOKEN=/ { sub(/^SMACKEREL_AUTH_TOKEN=/, ""); print; exit }' "$EXISTING_ENV_FILE" 2>/dev/null || true)"
  fi
  if [[ -n "$EXISTING_TOKEN" ]]; then
    SMACKEREL_AUTH_TOKEN="$EXISTING_TOKEN"
  else
    SMACKEREL_AUTH_TOKEN="$(openssl rand -hex 24 2>/dev/null || python3 -c 'import secrets; print(secrets.token_hex(24))')"
  fi
fi
HOST_BIND_ADDRESS="$(required_value runtime.host_bind_address)"
COMPOSE_WAIT_TIMEOUT_S="$(required_value runtime.compose_wait_timeout_s)"
DIGEST_CRON="$(required_value runtime.digest_cron)"
EMBEDDING_MODEL="$(required_value runtime.embedding_model)"
LOG_LEVEL="$(required_value runtime.log_level)"

# MIT-040-S-004 — SMACKEREL_ENV is a fail-loud SST signal consumed by the Go
# core (cmd/core/wiring.go + internal/api/router.go bearer middleware) and the
# Python ML sidecar (ml/app/main.py lifespan). Allowed values:
# development | test | production. When TARGET_ENV=test, the resolved value is
# overridden to "test" so integration/e2e/stress runs preserve the dev-mode
# warn-and-continue ergonomic for empty auth_token even when smackerel.yaml is
# configured for production.
SMACKEREL_ENV="$(required_value runtime.environment)"
case "$SMACKEREL_ENV" in
  development|test|production) ;;
  *)
    echo "Error: runtime.environment must be one of development|test|production, got '$SMACKEREL_ENV'" >&2
    exit 1
    ;;
esac
if [[ "$TARGET_ENV" == "test" ]]; then
  SMACKEREL_ENV="test"
fi
TELEGRAM_BOT_TOKEN="$(required_value telegram.bot_token)"
TELEGRAM_CHAT_IDS="$(required_value telegram.chat_ids)"
# Spec 044 Scope 03 — chat_id → user_id mapping for the Telegram bridge.
# Optional in dev/test; production deployments populate this so the
# claim-binding rejection in handleMessage admits real chats. The
# format mirrors chat_ids: "<chat_id>:<user_id>" pairs, comma-separated.
TELEGRAM_USER_MAPPING="$(yaml_get telegram.user_mapping 2>/dev/null)" || TELEGRAM_USER_MAPPING=""
TELEGRAM_ASSEMBLY_WINDOW_SECONDS="$(yaml_get telegram.assembly_window_seconds 2>/dev/null)" || TELEGRAM_ASSEMBLY_WINDOW_SECONDS="10"
TELEGRAM_ASSEMBLY_MAX_MESSAGES="$(yaml_get telegram.assembly_max_messages 2>/dev/null)" || TELEGRAM_ASSEMBLY_MAX_MESSAGES="100"
TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS="$(yaml_get telegram.media_group_window_seconds 2>/dev/null)" || TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS="3"
TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS="$(yaml_get telegram.disambiguation_timeout_seconds 2>/dev/null)" || TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS="120"
TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES="$(required_value telegram.cook_session_timeout_minutes)"
TELEGRAM_COOK_SESSION_MAX_PER_CHAT="$(required_value telegram.cook_session_max_per_chat)"

# Drive configuration (SST-owned; no generator defaults)
DRIVE_ENABLED="$(required_value drive.enabled)"
DRIVE_CLASSIFICATION_ENABLED="$(required_value drive.classification.enabled)"
DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD="$(required_value drive.classification.confidence_threshold)"
DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION="$(required_value drive.classification.low_confidence_action)"
DRIVE_CLASSIFICATION_CONFIRM_THRESHOLD="$(required_value drive.classification.confirm_threshold)"
DRIVE_CLASSIFICATION_CONFIRMATION_TTL_SECONDS="$(required_value drive.classification.confirmation_ttl_seconds)"
DRIVE_SCAN_PARALLELISM="$(required_value drive.scan.parallelism)"
DRIVE_SCAN_BATCH_SIZE="$(required_value drive.scan.batch_size)"
DRIVE_MONITOR_POLL_INTERVAL_SECONDS="$(required_value drive.monitor.poll_interval_seconds)"
DRIVE_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_FILES="$(required_value drive.monitor.cursor_invalidation_rescan_max_files)"
DRIVE_POLICY_SENSITIVITY_DEFAULT="$(required_value drive.policy.sensitivity_default)"
DRIVE_POLICY_SENSITIVITY_THRESHOLD_PUBLIC="$(required_value drive.policy.sensitivity_thresholds.public)"
DRIVE_POLICY_SENSITIVITY_THRESHOLD_INTERNAL="$(required_value drive.policy.sensitivity_thresholds.internal)"
DRIVE_POLICY_SENSITIVITY_THRESHOLD_SENSITIVE="$(required_value drive.policy.sensitivity_thresholds.sensitive)"
DRIVE_POLICY_SENSITIVITY_THRESHOLD_SECRET="$(required_value drive.policy.sensitivity_thresholds.secret)"
DRIVE_TELEGRAM_MAX_INLINE_SIZE_BYTES="$(required_value drive.telegram.max_inline_size_bytes)"
DRIVE_TELEGRAM_MAX_LINK_FILES_PER_REPLY="$(required_value drive.telegram.max_link_files_per_reply)"
DRIVE_LIMITS_MAX_FILE_SIZE_BYTES="$(required_value drive.limits.max_file_size_bytes)"
DRIVE_IO_LIMITS_PROVIDER_RESPONSE_MAX_BYTES="$(required_value drive.io_limits.provider_response_max_bytes)"
DRIVE_IO_LIMITS_PROVIDER_BINARY_MAX_BYTES="$(required_value drive.io_limits.provider_binary_max_bytes)"
DRIVE_IO_LIMITS_OAUTH_RESPONSE_MAX_BYTES="$(required_value drive.io_limits.oauth_response_max_bytes)"
DRIVE_RATE_LIMITS_REQUESTS_PER_MINUTE="$(required_value drive.rate_limits.requests_per_minute)"
DRIVE_SAVE_PROVIDER_URL_PREFIX="$(required_value drive.save.provider_url_prefix)"
DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_ID="$(required_value drive.providers.google.oauth_client_id)"
DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_SECRET="$(required_value drive.providers.google.oauth_client_secret)"
DRIVE_PROVIDER_GOOGLE_OAUTH_REDIRECT_URL="$(required_value drive.providers.google.oauth_redirect_url)"
DRIVE_PROVIDER_GOOGLE_OAUTH_BASE_URL="$(required_value drive.providers.google.oauth_base_url)"
DRIVE_PROVIDER_GOOGLE_API_BASE_URL="$(required_value drive.providers.google.api_base_url)"
DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS="$(required_json_value drive.providers.google.scope_defaults)"

# Photo library configuration (spec 040 SST-owned; no generator defaults)
PHOTOS_ENABLED="$(required_value photos.enabled)"
PHOTOS_SCAN_PARALLELISM="$(required_value photos.scan.parallelism)"
PHOTOS_SCAN_BATCH_SIZE="$(required_value photos.scan.batch_size)"
PHOTOS_SCAN_MAX_FILE_SIZE_BYTES="$(required_value photos.scan.max_file_size_bytes)"
PHOTOS_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_ITEMS="$(required_value photos.monitor.cursor_invalidation_rescan_max_items)"
PHOTOS_IO_LIMITS_PROVIDER_METADATA_MAX_BYTES="$(required_value photos.io_limits.provider_metadata_max_bytes)"
PHOTOS_IO_LIMITS_PHOTO_BINARY_MAX_BYTES="$(required_value photos.io_limits.photo_binary_max_bytes)"
PHOTOS_IO_LIMITS_TELEGRAM_RESPONSE_MAX_BYTES="$(required_value photos.io_limits.telegram_response_max_bytes)"
PHOTOS_POLICY_LIFECYCLE_CONFIRMATION_THRESHOLD="$(required_value photos.policy.lifecycle_confirmation_threshold)"
PHOTOS_POLICY_DUPLICATE_CONFIRMATION_THRESHOLD="$(required_value photos.policy.duplicate_confirmation_threshold)"
PHOTOS_POLICY_ROUTING_CONFIDENCE_THRESHOLD="$(required_value photos.policy.routing_confidence_threshold)"
PHOTOS_POLICY_SENSITIVITY_REVEAL_TTL_SECONDS="$(required_value photos.policy.sensitivity_reveal_ttl_seconds)"
PHOTOS_POLICY_ARCHIVE_ACTION_TOKEN_TTL_SECONDS="$(required_value photos.policy.archive_action_token_ttl_seconds)"
PHOTOS_POLICY_DELETE_ACTION_TOKEN_TTL_SECONDS="$(required_value photos.policy.delete_action_token_ttl_seconds)"
PHOTOS_POLICY_TELEGRAM_MAX_INLINE_SIZE_BYTES="$(required_value photos.policy.telegram_max_inline_size_bytes)"
PHOTOS_POLICY_ACTIONS_MAX_SCOPE_SIZE="$(required_value photos.policy.actions_max_scope_size)"
PHOTOS_INTELLIGENCE_CLASSIFY_MODEL="$(required_value photos.intelligence.classify_model)"
PHOTOS_INTELLIGENCE_EMBED_MODEL="$(required_value photos.intelligence.embed_model)"
PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL="$(required_value photos.intelligence.sensitivity_model)"
PHOTOS_INTELLIGENCE_AESTHETIC_MODEL="$(required_value photos.intelligence.aesthetic_model)"
PHOTOS_INTELLIGENCE_OCR_MODEL="$(required_value photos.intelligence.ocr_model)"
PHOTOS_INTELLIGENCE_MAX_INFLIGHT_PER_CONNECTOR="$(required_value photos.intelligence.max_inflight_per_connector)"
PHOTOS_PROVIDER_IMMICH_ENABLED="$(required_value photos.providers.immich.enabled)"
PHOTOS_PROVIDER_IMMICH_BASE_URL="$(required_value photos.providers.immich.base_url)"
PHOTOS_PROVIDER_IMMICH_API_KEY="$(required_value photos.providers.immich.api_key)"
PHOTOS_PROVIDER_IMMICH_POLL_INTERVAL_SECONDS="$(required_value photos.providers.immich.poll_interval_seconds)"
PHOTOS_PROVIDER_IMMICH_SUPPORTED_API_VERSIONS="$(required_json_value photos.providers.immich.supported_api_versions)"
PHOTOS_PROVIDER_PHOTOPRISM_ENABLED="$(required_value photos.providers.photoprism.enabled)"
PHOTOS_PROVIDER_PHOTOPRISM_BASE_URL="$(required_value photos.providers.photoprism.base_url)"
PHOTOS_PROVIDER_PHOTOPRISM_API_TOKEN="$(required_value photos.providers.photoprism.api_token)"
PHOTOS_PROVIDER_PHOTOPRISM_POLL_INTERVAL_SECONDS="$(required_value photos.providers.photoprism.poll_interval_seconds)"
PHOTOS_PROVIDER_PHOTOPRISM_SUPPORTED_API_VERSIONS="$(required_json_value photos.providers.photoprism.supported_api_versions)"

# Recommendations configuration (spec 039 SST-owned; no generator defaults)
RECOMMENDATIONS_ENABLED="$(required_value recommendations.enabled)"
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_ENABLED="$(required_value recommendations.providers.google_places.enabled)"
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_CATEGORIES="$(required_json_value recommendations.providers.google_places.categories)"
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_API_KEY="$(required_value recommendations.providers.google_places.api_key)"
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_QUOTA_WINDOW_SECONDS="$(required_value recommendations.providers.google_places.quota_window_seconds)"
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_MAX_REQUESTS_PER_WINDOW="$(required_value recommendations.providers.google_places.max_requests_per_window)"
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_ATTRIBUTION_LABEL="$(required_value recommendations.providers.google_places.attribution_label)"
RECOMMENDATIONS_PROVIDER_YELP_ENABLED="$(required_value recommendations.providers.yelp.enabled)"
RECOMMENDATIONS_PROVIDER_YELP_CATEGORIES="$(required_json_value recommendations.providers.yelp.categories)"
RECOMMENDATIONS_PROVIDER_YELP_API_KEY="$(required_value recommendations.providers.yelp.api_key)"
RECOMMENDATIONS_PROVIDER_YELP_QUOTA_WINDOW_SECONDS="$(required_value recommendations.providers.yelp.quota_window_seconds)"
RECOMMENDATIONS_PROVIDER_YELP_MAX_REQUESTS_PER_WINDOW="$(required_value recommendations.providers.yelp.max_requests_per_window)"
RECOMMENDATIONS_PROVIDER_YELP_ATTRIBUTION_LABEL="$(required_value recommendations.providers.yelp.attribution_label)"
RECOMMENDATIONS_LOCATION_PRECISION_USER_STANDARD="$(required_value recommendations.location_precision.user_standard)"
RECOMMENDATIONS_LOCATION_PRECISION_MOBILE_STANDARD="$(required_value recommendations.location_precision.mobile_standard)"
RECOMMENDATIONS_LOCATION_PRECISION_WATCH_STANDARD="$(required_value recommendations.location_precision.watch_standard)"
RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_SYSTEM="$(required_value recommendations.location_precision.neighborhood_cell_system)"
RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_LEVEL="$(required_value recommendations.location_precision.neighborhood_cell_level)"
RECOMMENDATIONS_WATCHES_MAX_ALERTS_PER_WINDOW="$(required_value recommendations.watches.max_alerts_per_window)"
RECOMMENDATIONS_WATCHES_ALERT_WINDOW_SECONDS="$(required_value recommendations.watches.alert_window_seconds)"
RECOMMENDATIONS_WATCHES_COOLDOWN_SECONDS_BY_KIND="$(required_json_value recommendations.watches.cooldown_seconds_by_kind)"
RECOMMENDATIONS_WATCHES_QUIET_HOURS_POLICY="$(required_json_value recommendations.watches.quiet_hours_policy)"
RECOMMENDATIONS_WATCHES_POLL_CRON="$(required_value recommendations.watches.poll_cron)"
RECOMMENDATIONS_RETENTION_RAW_PROVIDER_PAYLOAD_SECONDS="$(required_value recommendations.retention.raw_provider_payload_seconds)"
RECOMMENDATIONS_RETENTION_TRACE_RETENTION_SECONDS="$(required_value recommendations.retention.trace_retention_seconds)"
RECOMMENDATIONS_RANKING_MAX_CANDIDATES_PER_PROVIDER="$(required_value recommendations.ranking.max_candidates_per_provider)"
RECOMMENDATIONS_RANKING_MAX_FINAL_RESULTS="$(required_value recommendations.ranking.max_final_results)"
RECOMMENDATIONS_RANKING_STANDARD_RESULT_COUNT="$(required_value recommendations.ranking.standard_result_count)"
RECOMMENDATIONS_RANKING_STANDARD_STYLE="$(required_value recommendations.ranking.standard_style)"
RECOMMENDATIONS_RANKING_LOW_CONFIDENCE_THRESHOLD="$(required_value recommendations.ranking.low_confidence_threshold)"
RECOMMENDATIONS_POLICY_SPONSORED_PROMOTIONS_ENABLED="$(required_value recommendations.policy.sponsored_promotions_enabled)"
RECOMMENDATIONS_POLICY_RESTRICTED_CATEGORIES="$(required_json_value recommendations.policy.restricted_categories)"
RECOMMENDATIONS_POLICY_SAFETY_SOURCES="$(required_json_value recommendations.policy.safety_sources)"
RECOMMENDATIONS_DELIVERY_TELEGRAM_ENABLED="$(required_value recommendations.delivery.telegram_enabled)"
RECOMMENDATIONS_DELIVERY_DIGEST_ENABLED="$(required_value recommendations.delivery.digest_enabled)"
RECOMMENDATIONS_DELIVERY_TRIP_DOSSIER_ENABLED="$(required_value recommendations.delivery.trip_dossier_enabled)"

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

# CORS config
CORS_ALLOWED_ORIGINS_JSON="$(yaml_get_json cors.allowed_origins 2>/dev/null)" || CORS_ALLOWED_ORIGINS_JSON="[]"
# Convert JSON array to comma-separated string for env var
CORS_ALLOWED_ORIGINS=""
if [[ "$CORS_ALLOWED_ORIGINS_JSON" != "[]" && -n "$CORS_ALLOWED_ORIGINS_JSON" ]]; then
  CORS_ALLOWED_ORIGINS="$(python3 -c "import json,sys; print(','.join(json.loads(sys.argv[1])))" "$CORS_ALLOWED_ORIGINS_JSON" 2>/dev/null)" || CORS_ALLOWED_ORIGINS=""
fi

# Connector import paths (optional — empty string is valid default)
BOOKMARKS_ENABLED="$(yaml_get connectors.bookmarks.enabled 2>/dev/null)" || BOOKMARKS_ENABLED="false"
BOOKMARKS_SYNC_SCHEDULE="$(yaml_get connectors.bookmarks.sync_schedule 2>/dev/null)" || BOOKMARKS_SYNC_SCHEDULE=""
BOOKMARKS_IMPORT_DIR="$(yaml_get connectors.bookmarks.import_dir 2>/dev/null)" || BOOKMARKS_IMPORT_DIR=""
BOOKMARKS_WATCH_INTERVAL="$(yaml_get connectors.bookmarks.watch_interval 2>/dev/null)" || BOOKMARKS_WATCH_INTERVAL=""
BOOKMARKS_ARCHIVE_PROCESSED="$(yaml_get connectors.bookmarks.archive_processed 2>/dev/null)" || BOOKMARKS_ARCHIVE_PROCESSED=""
BOOKMARKS_PROCESSING_TIER="$(yaml_get connectors.bookmarks.processing_tier 2>/dev/null)" || BOOKMARKS_PROCESSING_TIER=""
BOOKMARKS_MIN_URL_LENGTH="$(yaml_get connectors.bookmarks.min_url_length 2>/dev/null)" || BOOKMARKS_MIN_URL_LENGTH=""
BOOKMARKS_EXCLUDE_DOMAINS="$(yaml_get connectors.bookmarks.exclude_domains 2>/dev/null)" || BOOKMARKS_EXCLUDE_DOMAINS=""
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
BROWSER_HISTORY_ACCESS_STRATEGY="$(yaml_get connectors.browser-history.chrome.access_strategy 2>/dev/null)" || BROWSER_HISTORY_ACCESS_STRATEGY=""
BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS="$(yaml_get connectors.browser-history.processing.initial_lookback_days 2>/dev/null)" || BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS=""
BROWSER_HISTORY_DWELL_FULL_MIN="$(yaml_get connectors.browser-history.processing.dwell_full_min 2>/dev/null)" || BROWSER_HISTORY_DWELL_FULL_MIN=""
BROWSER_HISTORY_DWELL_STANDARD_MIN="$(yaml_get connectors.browser-history.processing.dwell_standard_min 2>/dev/null)" || BROWSER_HISTORY_DWELL_STANDARD_MIN=""
BROWSER_HISTORY_DWELL_LIGHT_MIN="$(yaml_get connectors.browser-history.processing.dwell_light_min 2>/dev/null)" || BROWSER_HISTORY_DWELL_LIGHT_MIN=""
BROWSER_HISTORY_REPEAT_VISIT_WINDOW="$(yaml_get connectors.browser-history.processing.repeat_visit_window 2>/dev/null)" || BROWSER_HISTORY_REPEAT_VISIT_WINDOW=""
BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD="$(yaml_get connectors.browser-history.processing.repeat_visit_threshold 2>/dev/null)" || BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD=""
BROWSER_HISTORY_CONTENT_FETCH_TIMEOUT="$(yaml_get connectors.browser-history.processing.content_fetch_timeout 2>/dev/null)" || BROWSER_HISTORY_CONTENT_FETCH_TIMEOUT=""
BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY="$(yaml_get connectors.browser-history.processing.content_fetch_concurrency 2>/dev/null)" || BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY=""
BROWSER_HISTORY_CONTENT_FETCH_DOMAIN_DELAY="$(yaml_get connectors.browser-history.processing.content_fetch_domain_delay 2>/dev/null)" || BROWSER_HISTORY_CONTENT_FETCH_DOMAIN_DELAY=""
BROWSER_HISTORY_CUSTOM_SKIP_DOMAINS="$(yaml_get connectors.browser-history.skip.custom_domains 2>/dev/null)" || BROWSER_HISTORY_CUSTOM_SKIP_DOMAINS="[]"
BROWSER_HISTORY_SOCIAL_MEDIA_INDIVIDUAL_THRESHOLD="$(yaml_get connectors.browser-history.skip.social_media_individual_threshold 2>/dev/null)" || BROWSER_HISTORY_SOCIAL_MEDIA_INDIVIDUAL_THRESHOLD=""

# Discord connector
DISCORD_ENABLED="$(yaml_get connectors.discord.enabled 2>/dev/null)" || DISCORD_ENABLED="false"

# GuestHost connector
GUESTHOST_ENABLED="$(yaml_get connectors.guesthost.enabled 2>/dev/null)" || GUESTHOST_ENABLED="false"
GUESTHOST_BASE_URL="$(yaml_get connectors.guesthost.base_url 2>/dev/null)" || GUESTHOST_BASE_URL=""
GUESTHOST_API_KEY="$(yaml_get connectors.guesthost.api_key 2>/dev/null)" || GUESTHOST_API_KEY=""
GUESTHOST_SYNC_SCHEDULE="$(yaml_get connectors.guesthost.sync_schedule 2>/dev/null)" || GUESTHOST_SYNC_SCHEDULE=""
GUESTHOST_EVENT_TYPES="$(yaml_get connectors.guesthost.event_types 2>/dev/null)" || GUESTHOST_EVENT_TYPES=""

# QF decisions connector
QF_DECISIONS_ENABLED="$(env_override_value qf_decisions_enabled connectors.qf-decisions.enabled)"
QF_DECISIONS_BASE_URL="$(env_override_value qf_decisions_base_url connectors.qf-decisions.base_url)"
QF_DECISIONS_CREDENTIAL_REF="$(env_override_value qf_decisions_credential_ref connectors.qf-decisions.credential_ref)"
QF_DECISIONS_SYNC_SCHEDULE="$(env_override_value qf_decisions_sync_schedule connectors.qf-decisions.sync_schedule)"
QF_DECISIONS_PACKET_VERSION="$(env_override_value qf_decisions_packet_version connectors.qf-decisions.packet_version)"
QF_DECISIONS_PAGE_SIZE="$(env_override_value qf_decisions_page_size connectors.qf-decisions.page_size)"

# Hospitable connector
HOSPITABLE_ENABLED="$(yaml_get connectors.hospitable.enabled 2>/dev/null)" || HOSPITABLE_ENABLED="false"
HOSPITABLE_ACCESS_TOKEN="$(yaml_get connectors.hospitable.access_token 2>/dev/null)" || HOSPITABLE_ACCESS_TOKEN=""
HOSPITABLE_BASE_URL="$(yaml_get connectors.hospitable.base_url 2>/dev/null)" || HOSPITABLE_BASE_URL=""
HOSPITABLE_SYNC_SCHEDULE="$(yaml_get connectors.hospitable.sync_schedule 2>/dev/null)" || HOSPITABLE_SYNC_SCHEDULE=""
HOSPITABLE_INITIAL_LOOKBACK_DAYS="$(yaml_get connectors.hospitable.initial_lookback_days 2>/dev/null)" || HOSPITABLE_INITIAL_LOOKBACK_DAYS=""
HOSPITABLE_PAGE_SIZE="$(yaml_get connectors.hospitable.page_size 2>/dev/null)" || HOSPITABLE_PAGE_SIZE=""
HOSPITABLE_SYNC_PROPERTIES="$(yaml_get connectors.hospitable.sync_properties 2>/dev/null)" || HOSPITABLE_SYNC_PROPERTIES=""
HOSPITABLE_SYNC_RESERVATIONS="$(yaml_get connectors.hospitable.sync_reservations 2>/dev/null)" || HOSPITABLE_SYNC_RESERVATIONS=""
HOSPITABLE_SYNC_MESSAGES="$(yaml_get connectors.hospitable.sync_messages 2>/dev/null)" || HOSPITABLE_SYNC_MESSAGES=""
HOSPITABLE_SYNC_REVIEWS="$(yaml_get connectors.hospitable.sync_reviews 2>/dev/null)" || HOSPITABLE_SYNC_REVIEWS=""
HOSPITABLE_TIER_MESSAGES="$(yaml_get connectors.hospitable.processing_tier_messages 2>/dev/null)" || HOSPITABLE_TIER_MESSAGES=""
HOSPITABLE_TIER_REVIEWS="$(yaml_get connectors.hospitable.processing_tier_reviews 2>/dev/null)" || HOSPITABLE_TIER_REVIEWS=""
HOSPITABLE_TIER_RESERVATIONS="$(yaml_get connectors.hospitable.processing_tier_reservations 2>/dev/null)" || HOSPITABLE_TIER_RESERVATIONS=""
HOSPITABLE_TIER_PROPERTIES="$(yaml_get connectors.hospitable.processing_tier_properties 2>/dev/null)" || HOSPITABLE_TIER_PROPERTIES=""

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

# Spec 044 — Per-User Bearer Auth Foundation. SST zero-defaults: every key is
# REQUIRED at the generator boundary. Empty-string secret-bearing fields are
# the intended dev/test pattern (SCN-AUTH-005). The Go core fails loud at
# runtime startup when SMACKEREL_ENV=production AND AUTH_ENABLED=true AND any
# required signing/hashing/bootstrap value is empty (SCN-AUTH-006). The
# generator does NOT short-circuit on empty secret values because the
# operator MUST be able to run `./smackerel.sh config generate --env home-lab`
# and edit the resulting bundle before redeploying — the runtime is the
# fail-loud authority for production-mode validation.
AUTH_ENABLED="$(env_override_value auth_enabled auth.enabled)"
AUTH_TOKEN_FORMAT="$(required_value auth.token_format)"
AUTH_SIGNING_ACTIVE_PRIVATE_KEY="$(yaml_get auth.signing.active_private_key 2>/dev/null)" || AUTH_SIGNING_ACTIVE_PRIVATE_KEY=""
AUTH_SIGNING_ACTIVE_KEY_ID="$(yaml_get auth.signing.active_key_id 2>/dev/null)" || AUTH_SIGNING_ACTIVE_KEY_ID=""
AUTH_SIGNING_PRIOR_PUBLIC_KEY="$(yaml_get auth.signing.prior_public_key 2>/dev/null)" || AUTH_SIGNING_PRIOR_PUBLIC_KEY=""
AUTH_SIGNING_PRIOR_KEY_ID="$(yaml_get auth.signing.prior_key_id 2>/dev/null)" || AUTH_SIGNING_PRIOR_KEY_ID=""
AUTH_TOKEN_TTL_HOURS="$(required_value auth.token_ttl_hours)"
AUTH_ROTATION_GRACE_WINDOW_HOURS="$(required_value auth.rotation_grace_window_hours)"
AUTH_CLOCK_SKEW_TOLERANCE_SECONDS="$(required_value auth.clock_skew_tolerance_seconds)"
AUTH_REVOCATION_CACHE_REFRESH_INTERVAL_SECONDS="$(required_value auth.revocation_cache_refresh_interval_seconds)"
AUTH_REVOCATION_NATS_SUBJECT="$(required_value auth.revocation_nats_subject)"
AUTH_AT_REST_HASHING_KEY="$(yaml_get auth.at_rest_hashing_key 2>/dev/null)" || AUTH_AT_REST_HASHING_KEY=""
AUTH_PRODUCTION_SHARED_TOKEN_FALLBACK_ENABLED="$(required_value auth.production_shared_token_fallback_enabled)"
AUTH_TELEMETRY_ENABLED="$(required_value auth.telemetry_enabled)"
AUTH_TELEMETRY_METRIC_PREFIX="$(required_value auth.telemetry_metric_prefix)"
AUTH_BOOTSTRAP_TOKEN="$(yaml_get auth.bootstrap_token 2>/dev/null)" || AUTH_BOOTSTRAP_TOKEN=""

# Agent (spec 037 — LLM Scenario Agent & Tool Registry). SST zero-defaults:
# every value is REQUIRED. Missing keys → config generate exits non-zero.
AGENT_SCENARIO_DIR="$(required_value agent.scenario_dir)"
AGENT_SCENARIO_GLOB="$(required_value agent.scenario_glob)"
AGENT_HOT_RELOAD="$(required_value agent.hot_reload)"
AGENT_ROUTING_CONFIDENCE_FLOOR="$(required_value agent.routing.confidence_floor)"
AGENT_ROUTING_CONSIDER_TOP_N="$(required_value agent.routing.consider_top_n)"
AGENT_ROUTING_FALLBACK_SCENARIO_ID="$(required_value agent.routing.fallback_scenario_id)"
AGENT_ROUTING_EMBEDDING_MODEL="$(required_value agent.routing.embedding_model)"
AGENT_TRACE_RETENTION_DAYS="$(required_value agent.trace.retention_days)"
AGENT_TRACE_RECORD_LLM_MESSAGES="$(required_value agent.trace.record_llm_messages)"
AGENT_TRACE_REDACT_MARKER="$(required_value agent.trace.redact_marker)"
AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING="$(required_value agent.defaults.max_loop_iterations_ceiling)"
AGENT_DEFAULTS_TIMEOUT_MS_CEILING="$(required_value agent.defaults.timeout_ms_ceiling)"
AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING="$(required_value agent.defaults.schema_retry_budget_ceiling)"
AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING="$(required_value agent.defaults.per_tool_timeout_ms_ceiling)"
AGENT_PROVIDER_DEFAULT_PROVIDER="$(required_value agent.provider_routing.default.provider)"
AGENT_PROVIDER_DEFAULT_MODEL="$(required_value agent.provider_routing.default.model)"
AGENT_PROVIDER_REASONING_PROVIDER="$(required_value agent.provider_routing.reasoning.provider)"
AGENT_PROVIDER_REASONING_MODEL="$(required_value agent.provider_routing.reasoning.model)"
AGENT_PROVIDER_FAST_PROVIDER="$(required_value agent.provider_routing.fast.provider)"
# Spec 043 — agent_provider_fast_model uses per-env override so the test
# environment can pin to a small tool-calling model (the smallest one that
# runs on the CI / no-GPU lane) without forcing dev / home-lab off their
# preferred routes. The model literal lives in
# environments.<env>.agent_provider_fast_model in config/smackerel.yaml.
AGENT_PROVIDER_FAST_MODEL="$(env_override_value agent_provider_fast_model agent.provider_routing.fast.model)"
AGENT_PROVIDER_VISION_PROVIDER="$(required_value agent.provider_routing.vision.provider)"
AGENT_PROVIDER_VISION_MODEL="$(required_value agent.provider_routing.vision.model)"
AGENT_PROVIDER_OCR_PROVIDER="$(required_value agent.provider_routing.ocr.provider)"
AGENT_PROVIDER_OCR_MODEL="$(required_value agent.provider_routing.ocr.model)"

mkdir -p "$REPO_ROOT/config/generated"

OUTPUT_FILE="$REPO_ROOT/config/generated/${TARGET_ENV}.env"

cat > "$OUTPUT_FILE" <<EOF
# Auto-generated from config/smackerel.yaml — DO NOT EDIT DIRECTLY
# Regenerate: ./smackerel.sh config generate
# Environment: ${TARGET_ENV}
# Generated: $(date -u +%Y-%m-%dT%H:%M:%S+00:00)
SMACKEREL_ENV_FILE=config/generated/${TARGET_ENV}.env
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
NATS_MAX_RECONNECT_ATTEMPTS=${NATS_MAX_RECONNECT_ATTEMPTS}
NATS_RECONNECT_TIME_WAIT_SECONDS=${NATS_RECONNECT_TIME_WAIT_SECONDS}
NATS_MAX_PAYLOAD_BYTES=${NATS_MAX_PAYLOAD_BYTES}
NATS_MAX_FILE_STORE_BYTES=${NATS_MAX_FILE_STORE_BYTES}
NATS_MAX_MEM_STORE_BYTES=${NATS_MAX_MEM_STORE_BYTES}
NATS_STREAM_MAX_BYTES_JSON=${NATS_STREAM_MAX_BYTES_JSON}
CORE_CONTAINER_PORT=${CORE_CONTAINER_PORT}
CORE_HOST_PORT=${CORE_HOST_PORT}
ML_CONTAINER_PORT=${ML_CONTAINER_PORT}
ML_HOST_PORT=${ML_HOST_PORT}
OLLAMA_CONTAINER_PORT=${OLLAMA_CONTAINER_PORT}
OLLAMA_HOST_PORT=${OLLAMA_HOST_PORT}
OLLAMA_VOLUME_NAME=${OLLAMA_VOLUME_NAME}
OLLAMA_IMAGE=${OLLAMA_IMAGE}
OLLAMA_TEST_MODEL=${OLLAMA_TEST_MODEL}
OLLAMA_TEST_PULL_TIMEOUT_SECONDS=${OLLAMA_TEST_PULL_TIMEOUT_SECONDS}
OLLAMA_TEST_REQUEST_TEMPERATURE=${OLLAMA_TEST_REQUEST_TEMPERATURE}
OLLAMA_TEST_REQUEST_TOP_P=${OLLAMA_TEST_REQUEST_TOP_P}
OLLAMA_TEST_REQUEST_TOP_K=${OLLAMA_TEST_REQUEST_TOP_K}
OLLAMA_TEST_REQUEST_SEED=${OLLAMA_TEST_REQUEST_SEED}
OLLAMA_TEST_REQUEST_NUM_PREDICT=${OLLAMA_TEST_REQUEST_NUM_PREDICT}
ENABLE_OLLAMA=${OLLAMA_ENABLED}
DATABASE_URL=${DATABASE_URL}
NATS_URL=${NATS_URL}
LLM_PROVIDER=${LLM_PROVIDER}
LLM_MODEL=${LLM_MODEL}
LLM_API_KEY=${LLM_API_KEY}
SMACKEREL_AUTH_TOKEN=${SMACKEREL_AUTH_TOKEN}
SMACKEREL_ENV=${SMACKEREL_ENV}
HOST_BIND_ADDRESS=${HOST_BIND_ADDRESS}
COMPOSE_WAIT_TIMEOUT_S=${COMPOSE_WAIT_TIMEOUT_S}
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
TELEGRAM_USER_MAPPING=${TELEGRAM_USER_MAPPING}
TELEGRAM_ASSEMBLY_WINDOW_SECONDS=${TELEGRAM_ASSEMBLY_WINDOW_SECONDS}
TELEGRAM_ASSEMBLY_MAX_MESSAGES=${TELEGRAM_ASSEMBLY_MAX_MESSAGES}
TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS=${TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS}
TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS=${TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS}
TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES=${TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES}
TELEGRAM_COOK_SESSION_MAX_PER_CHAT=${TELEGRAM_COOK_SESSION_MAX_PER_CHAT}
DRIVE_ENABLED=${DRIVE_ENABLED}
DRIVE_CLASSIFICATION_ENABLED=${DRIVE_CLASSIFICATION_ENABLED}
DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD=${DRIVE_CLASSIFICATION_CONFIDENCE_THRESHOLD}
DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION=${DRIVE_CLASSIFICATION_LOW_CONFIDENCE_ACTION}
DRIVE_CLASSIFICATION_CONFIRM_THRESHOLD=${DRIVE_CLASSIFICATION_CONFIRM_THRESHOLD}
DRIVE_CLASSIFICATION_CONFIRMATION_TTL_SECONDS=${DRIVE_CLASSIFICATION_CONFIRMATION_TTL_SECONDS}
DRIVE_SCAN_PARALLELISM=${DRIVE_SCAN_PARALLELISM}
DRIVE_SCAN_BATCH_SIZE=${DRIVE_SCAN_BATCH_SIZE}
DRIVE_MONITOR_POLL_INTERVAL_SECONDS=${DRIVE_MONITOR_POLL_INTERVAL_SECONDS}
DRIVE_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_FILES=${DRIVE_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_FILES}
DRIVE_POLICY_SENSITIVITY_DEFAULT=${DRIVE_POLICY_SENSITIVITY_DEFAULT}
DRIVE_POLICY_SENSITIVITY_THRESHOLD_PUBLIC=${DRIVE_POLICY_SENSITIVITY_THRESHOLD_PUBLIC}
DRIVE_POLICY_SENSITIVITY_THRESHOLD_INTERNAL=${DRIVE_POLICY_SENSITIVITY_THRESHOLD_INTERNAL}
DRIVE_POLICY_SENSITIVITY_THRESHOLD_SENSITIVE=${DRIVE_POLICY_SENSITIVITY_THRESHOLD_SENSITIVE}
DRIVE_POLICY_SENSITIVITY_THRESHOLD_SECRET=${DRIVE_POLICY_SENSITIVITY_THRESHOLD_SECRET}
DRIVE_TELEGRAM_MAX_INLINE_SIZE_BYTES=${DRIVE_TELEGRAM_MAX_INLINE_SIZE_BYTES}
DRIVE_TELEGRAM_MAX_LINK_FILES_PER_REPLY=${DRIVE_TELEGRAM_MAX_LINK_FILES_PER_REPLY}
DRIVE_LIMITS_MAX_FILE_SIZE_BYTES=${DRIVE_LIMITS_MAX_FILE_SIZE_BYTES}
DRIVE_IO_LIMITS_PROVIDER_RESPONSE_MAX_BYTES=${DRIVE_IO_LIMITS_PROVIDER_RESPONSE_MAX_BYTES}
DRIVE_IO_LIMITS_PROVIDER_BINARY_MAX_BYTES=${DRIVE_IO_LIMITS_PROVIDER_BINARY_MAX_BYTES}
DRIVE_IO_LIMITS_OAUTH_RESPONSE_MAX_BYTES=${DRIVE_IO_LIMITS_OAUTH_RESPONSE_MAX_BYTES}
DRIVE_RATE_LIMITS_REQUESTS_PER_MINUTE=${DRIVE_RATE_LIMITS_REQUESTS_PER_MINUTE}
DRIVE_SAVE_PROVIDER_URL_PREFIX=${DRIVE_SAVE_PROVIDER_URL_PREFIX}
DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_ID=${DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_ID}
DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_SECRET=${DRIVE_PROVIDER_GOOGLE_OAUTH_CLIENT_SECRET}
DRIVE_PROVIDER_GOOGLE_OAUTH_REDIRECT_URL=${DRIVE_PROVIDER_GOOGLE_OAUTH_REDIRECT_URL}
DRIVE_PROVIDER_GOOGLE_OAUTH_BASE_URL=${DRIVE_PROVIDER_GOOGLE_OAUTH_BASE_URL}
DRIVE_PROVIDER_GOOGLE_API_BASE_URL=${DRIVE_PROVIDER_GOOGLE_API_BASE_URL}
DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS=${DRIVE_PROVIDER_GOOGLE_SCOPE_DEFAULTS}
PHOTOS_ENABLED=${PHOTOS_ENABLED}
PHOTOS_SCAN_PARALLELISM=${PHOTOS_SCAN_PARALLELISM}
PHOTOS_SCAN_BATCH_SIZE=${PHOTOS_SCAN_BATCH_SIZE}
PHOTOS_SCAN_MAX_FILE_SIZE_BYTES=${PHOTOS_SCAN_MAX_FILE_SIZE_BYTES}
PHOTOS_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_ITEMS=${PHOTOS_MONITOR_CURSOR_INVALIDATION_RESCAN_MAX_ITEMS}
PHOTOS_IO_LIMITS_PROVIDER_METADATA_MAX_BYTES=${PHOTOS_IO_LIMITS_PROVIDER_METADATA_MAX_BYTES}
PHOTOS_IO_LIMITS_PHOTO_BINARY_MAX_BYTES=${PHOTOS_IO_LIMITS_PHOTO_BINARY_MAX_BYTES}
PHOTOS_IO_LIMITS_TELEGRAM_RESPONSE_MAX_BYTES=${PHOTOS_IO_LIMITS_TELEGRAM_RESPONSE_MAX_BYTES}
PHOTOS_POLICY_LIFECYCLE_CONFIRMATION_THRESHOLD=${PHOTOS_POLICY_LIFECYCLE_CONFIRMATION_THRESHOLD}
PHOTOS_POLICY_DUPLICATE_CONFIRMATION_THRESHOLD=${PHOTOS_POLICY_DUPLICATE_CONFIRMATION_THRESHOLD}
PHOTOS_POLICY_ROUTING_CONFIDENCE_THRESHOLD=${PHOTOS_POLICY_ROUTING_CONFIDENCE_THRESHOLD}
PHOTOS_POLICY_SENSITIVITY_REVEAL_TTL_SECONDS=${PHOTOS_POLICY_SENSITIVITY_REVEAL_TTL_SECONDS}
PHOTOS_POLICY_ARCHIVE_ACTION_TOKEN_TTL_SECONDS=${PHOTOS_POLICY_ARCHIVE_ACTION_TOKEN_TTL_SECONDS}
PHOTOS_POLICY_DELETE_ACTION_TOKEN_TTL_SECONDS=${PHOTOS_POLICY_DELETE_ACTION_TOKEN_TTL_SECONDS}
PHOTOS_POLICY_TELEGRAM_MAX_INLINE_SIZE_BYTES=${PHOTOS_POLICY_TELEGRAM_MAX_INLINE_SIZE_BYTES}
PHOTOS_POLICY_ACTIONS_MAX_SCOPE_SIZE=${PHOTOS_POLICY_ACTIONS_MAX_SCOPE_SIZE}
PHOTOS_INTELLIGENCE_CLASSIFY_MODEL=${PHOTOS_INTELLIGENCE_CLASSIFY_MODEL}
PHOTOS_INTELLIGENCE_EMBED_MODEL=${PHOTOS_INTELLIGENCE_EMBED_MODEL}
PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL=${PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL}
PHOTOS_INTELLIGENCE_AESTHETIC_MODEL=${PHOTOS_INTELLIGENCE_AESTHETIC_MODEL}
PHOTOS_INTELLIGENCE_OCR_MODEL=${PHOTOS_INTELLIGENCE_OCR_MODEL}
PHOTOS_INTELLIGENCE_MAX_INFLIGHT_PER_CONNECTOR=${PHOTOS_INTELLIGENCE_MAX_INFLIGHT_PER_CONNECTOR}
PHOTOS_PROVIDER_IMMICH_ENABLED=${PHOTOS_PROVIDER_IMMICH_ENABLED}
PHOTOS_PROVIDER_IMMICH_BASE_URL=${PHOTOS_PROVIDER_IMMICH_BASE_URL}
PHOTOS_PROVIDER_IMMICH_API_KEY=${PHOTOS_PROVIDER_IMMICH_API_KEY}
PHOTOS_PROVIDER_IMMICH_POLL_INTERVAL_SECONDS=${PHOTOS_PROVIDER_IMMICH_POLL_INTERVAL_SECONDS}
PHOTOS_PROVIDER_IMMICH_SUPPORTED_API_VERSIONS=${PHOTOS_PROVIDER_IMMICH_SUPPORTED_API_VERSIONS}
PHOTOS_PROVIDER_PHOTOPRISM_ENABLED=${PHOTOS_PROVIDER_PHOTOPRISM_ENABLED}
PHOTOS_PROVIDER_PHOTOPRISM_BASE_URL=${PHOTOS_PROVIDER_PHOTOPRISM_BASE_URL}
PHOTOS_PROVIDER_PHOTOPRISM_API_TOKEN=${PHOTOS_PROVIDER_PHOTOPRISM_API_TOKEN}
PHOTOS_PROVIDER_PHOTOPRISM_POLL_INTERVAL_SECONDS=${PHOTOS_PROVIDER_PHOTOPRISM_POLL_INTERVAL_SECONDS}
PHOTOS_PROVIDER_PHOTOPRISM_SUPPORTED_API_VERSIONS=${PHOTOS_PROVIDER_PHOTOPRISM_SUPPORTED_API_VERSIONS}
RECOMMENDATIONS_ENABLED=${RECOMMENDATIONS_ENABLED}
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_ENABLED=${RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_ENABLED}
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_CATEGORIES=${RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_CATEGORIES}
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_API_KEY=${RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_API_KEY}
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_QUOTA_WINDOW_SECONDS=${RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_QUOTA_WINDOW_SECONDS}
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_MAX_REQUESTS_PER_WINDOW=${RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_MAX_REQUESTS_PER_WINDOW}
RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_ATTRIBUTION_LABEL=${RECOMMENDATIONS_PROVIDER_GOOGLE_PLACES_ATTRIBUTION_LABEL}
RECOMMENDATIONS_PROVIDER_YELP_ENABLED=${RECOMMENDATIONS_PROVIDER_YELP_ENABLED}
RECOMMENDATIONS_PROVIDER_YELP_CATEGORIES=${RECOMMENDATIONS_PROVIDER_YELP_CATEGORIES}
RECOMMENDATIONS_PROVIDER_YELP_API_KEY=${RECOMMENDATIONS_PROVIDER_YELP_API_KEY}
RECOMMENDATIONS_PROVIDER_YELP_QUOTA_WINDOW_SECONDS=${RECOMMENDATIONS_PROVIDER_YELP_QUOTA_WINDOW_SECONDS}
RECOMMENDATIONS_PROVIDER_YELP_MAX_REQUESTS_PER_WINDOW=${RECOMMENDATIONS_PROVIDER_YELP_MAX_REQUESTS_PER_WINDOW}
RECOMMENDATIONS_PROVIDER_YELP_ATTRIBUTION_LABEL=${RECOMMENDATIONS_PROVIDER_YELP_ATTRIBUTION_LABEL}
RECOMMENDATIONS_LOCATION_PRECISION_USER_STANDARD=${RECOMMENDATIONS_LOCATION_PRECISION_USER_STANDARD}
RECOMMENDATIONS_LOCATION_PRECISION_MOBILE_STANDARD=${RECOMMENDATIONS_LOCATION_PRECISION_MOBILE_STANDARD}
RECOMMENDATIONS_LOCATION_PRECISION_WATCH_STANDARD=${RECOMMENDATIONS_LOCATION_PRECISION_WATCH_STANDARD}
RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_SYSTEM=${RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_SYSTEM}
RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_LEVEL=${RECOMMENDATIONS_LOCATION_PRECISION_NEIGHBORHOOD_CELL_LEVEL}
RECOMMENDATIONS_WATCHES_MAX_ALERTS_PER_WINDOW=${RECOMMENDATIONS_WATCHES_MAX_ALERTS_PER_WINDOW}
RECOMMENDATIONS_WATCHES_ALERT_WINDOW_SECONDS=${RECOMMENDATIONS_WATCHES_ALERT_WINDOW_SECONDS}
RECOMMENDATIONS_WATCHES_COOLDOWN_SECONDS_BY_KIND=${RECOMMENDATIONS_WATCHES_COOLDOWN_SECONDS_BY_KIND}
RECOMMENDATIONS_WATCHES_QUIET_HOURS_POLICY=${RECOMMENDATIONS_WATCHES_QUIET_HOURS_POLICY}
RECOMMENDATIONS_WATCHES_POLL_CRON=${RECOMMENDATIONS_WATCHES_POLL_CRON}
RECOMMENDATIONS_RETENTION_RAW_PROVIDER_PAYLOAD_SECONDS=${RECOMMENDATIONS_RETENTION_RAW_PROVIDER_PAYLOAD_SECONDS}
RECOMMENDATIONS_RETENTION_TRACE_RETENTION_SECONDS=${RECOMMENDATIONS_RETENTION_TRACE_RETENTION_SECONDS}
RECOMMENDATIONS_RANKING_MAX_CANDIDATES_PER_PROVIDER=${RECOMMENDATIONS_RANKING_MAX_CANDIDATES_PER_PROVIDER}
RECOMMENDATIONS_RANKING_MAX_FINAL_RESULTS=${RECOMMENDATIONS_RANKING_MAX_FINAL_RESULTS}
RECOMMENDATIONS_RANKING_STANDARD_RESULT_COUNT=${RECOMMENDATIONS_RANKING_STANDARD_RESULT_COUNT}
RECOMMENDATIONS_RANKING_STANDARD_STYLE=${RECOMMENDATIONS_RANKING_STANDARD_STYLE}
RECOMMENDATIONS_RANKING_LOW_CONFIDENCE_THRESHOLD=${RECOMMENDATIONS_RANKING_LOW_CONFIDENCE_THRESHOLD}
RECOMMENDATIONS_POLICY_SPONSORED_PROMOTIONS_ENABLED=${RECOMMENDATIONS_POLICY_SPONSORED_PROMOTIONS_ENABLED}
RECOMMENDATIONS_POLICY_RESTRICTED_CATEGORIES=${RECOMMENDATIONS_POLICY_RESTRICTED_CATEGORIES}
RECOMMENDATIONS_POLICY_SAFETY_SOURCES=${RECOMMENDATIONS_POLICY_SAFETY_SOURCES}
RECOMMENDATIONS_DELIVERY_TELEGRAM_ENABLED=${RECOMMENDATIONS_DELIVERY_TELEGRAM_ENABLED}
RECOMMENDATIONS_DELIVERY_DIGEST_ENABLED=${RECOMMENDATIONS_DELIVERY_DIGEST_ENABLED}
RECOMMENDATIONS_DELIVERY_TRIP_DOSSIER_ENABLED=${RECOMMENDATIONS_DELIVERY_TRIP_DOSSIER_ENABLED}
CORE_EXTERNAL_URL=${CORE_EXTERNAL_URL}
ML_EXTERNAL_URL=${ML_EXTERNAL_URL}
BOOKMARKS_ENABLED=${BOOKMARKS_ENABLED}
BOOKMARKS_SYNC_SCHEDULE=${BOOKMARKS_SYNC_SCHEDULE}
BOOKMARKS_IMPORT_DIR=${BOOKMARKS_IMPORT_DIR}
BOOKMARKS_WATCH_INTERVAL=${BOOKMARKS_WATCH_INTERVAL}
BOOKMARKS_ARCHIVE_PROCESSED=${BOOKMARKS_ARCHIVE_PROCESSED}
BOOKMARKS_PROCESSING_TIER=${BOOKMARKS_PROCESSING_TIER}
BOOKMARKS_MIN_URL_LENGTH=${BOOKMARKS_MIN_URL_LENGTH}
BOOKMARKS_EXCLUDE_DOMAINS=${BOOKMARKS_EXCLUDE_DOMAINS}
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
BROWSER_HISTORY_ACCESS_STRATEGY=${BROWSER_HISTORY_ACCESS_STRATEGY}
BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS=${BROWSER_HISTORY_INITIAL_LOOKBACK_DAYS}
BROWSER_HISTORY_DWELL_FULL_MIN=${BROWSER_HISTORY_DWELL_FULL_MIN}
BROWSER_HISTORY_DWELL_STANDARD_MIN=${BROWSER_HISTORY_DWELL_STANDARD_MIN}
BROWSER_HISTORY_DWELL_LIGHT_MIN=${BROWSER_HISTORY_DWELL_LIGHT_MIN}
BROWSER_HISTORY_REPEAT_VISIT_WINDOW=${BROWSER_HISTORY_REPEAT_VISIT_WINDOW}
BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD=${BROWSER_HISTORY_REPEAT_VISIT_THRESHOLD}
BROWSER_HISTORY_CONTENT_FETCH_TIMEOUT=${BROWSER_HISTORY_CONTENT_FETCH_TIMEOUT}
BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY=${BROWSER_HISTORY_CONTENT_FETCH_CONCURRENCY}
BROWSER_HISTORY_CONTENT_FETCH_DOMAIN_DELAY=${BROWSER_HISTORY_CONTENT_FETCH_DOMAIN_DELAY}
BROWSER_HISTORY_CUSTOM_SKIP_DOMAINS=${BROWSER_HISTORY_CUSTOM_SKIP_DOMAINS}
BROWSER_HISTORY_SOCIAL_MEDIA_INDIVIDUAL_THRESHOLD=${BROWSER_HISTORY_SOCIAL_MEDIA_INDIVIDUAL_THRESHOLD}
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
QF_DECISIONS_ENABLED=${QF_DECISIONS_ENABLED}
QF_DECISIONS_BASE_URL=${QF_DECISIONS_BASE_URL}
QF_DECISIONS_CREDENTIAL_REF=${QF_DECISIONS_CREDENTIAL_REF}
QF_DECISIONS_SYNC_SCHEDULE=${QF_DECISIONS_SYNC_SCHEDULE}
QF_DECISIONS_PACKET_VERSION=${QF_DECISIONS_PACKET_VERSION}
QF_DECISIONS_PAGE_SIZE=${QF_DECISIONS_PAGE_SIZE}
HOSPITABLE_ENABLED=${HOSPITABLE_ENABLED}
HOSPITABLE_ACCESS_TOKEN=${HOSPITABLE_ACCESS_TOKEN}
HOSPITABLE_BASE_URL=${HOSPITABLE_BASE_URL}
HOSPITABLE_SYNC_SCHEDULE=${HOSPITABLE_SYNC_SCHEDULE}
HOSPITABLE_INITIAL_LOOKBACK_DAYS=${HOSPITABLE_INITIAL_LOOKBACK_DAYS}
HOSPITABLE_PAGE_SIZE=${HOSPITABLE_PAGE_SIZE}
HOSPITABLE_SYNC_PROPERTIES=${HOSPITABLE_SYNC_PROPERTIES}
HOSPITABLE_SYNC_RESERVATIONS=${HOSPITABLE_SYNC_RESERVATIONS}
HOSPITABLE_SYNC_MESSAGES=${HOSPITABLE_SYNC_MESSAGES}
HOSPITABLE_SYNC_REVIEWS=${HOSPITABLE_SYNC_REVIEWS}
HOSPITABLE_TIER_MESSAGES=${HOSPITABLE_TIER_MESSAGES}
HOSPITABLE_TIER_REVIEWS=${HOSPITABLE_TIER_REVIEWS}
HOSPITABLE_TIER_RESERVATIONS=${HOSPITABLE_TIER_RESERVATIONS}
HOSPITABLE_TIER_PROPERTIES=${HOSPITABLE_TIER_PROPERTIES}
DB_MAX_CONNS=${DB_MAX_CONNS}
DB_MIN_CONNS=${DB_MIN_CONNS}
SHUTDOWN_TIMEOUT_S=${SHUTDOWN_TIMEOUT_S}
ML_HEALTH_CACHE_TTL_S=${ML_HEALTH_CACHE_TTL_S}
ML_READINESS_TIMEOUT_S=${ML_READINESS_TIMEOUT_S}
ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=${ML_PROCESSING_DEGRADED_FALLBACK_ENABLED}
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
AUTH_ENABLED=${AUTH_ENABLED}
AUTH_TOKEN_FORMAT=${AUTH_TOKEN_FORMAT}
AUTH_SIGNING_ACTIVE_PRIVATE_KEY=${AUTH_SIGNING_ACTIVE_PRIVATE_KEY}
AUTH_SIGNING_ACTIVE_KEY_ID=${AUTH_SIGNING_ACTIVE_KEY_ID}
AUTH_SIGNING_PRIOR_PUBLIC_KEY=${AUTH_SIGNING_PRIOR_PUBLIC_KEY}
AUTH_SIGNING_PRIOR_KEY_ID=${AUTH_SIGNING_PRIOR_KEY_ID}
AUTH_TOKEN_TTL_HOURS=${AUTH_TOKEN_TTL_HOURS}
AUTH_ROTATION_GRACE_WINDOW_HOURS=${AUTH_ROTATION_GRACE_WINDOW_HOURS}
AUTH_CLOCK_SKEW_TOLERANCE_SECONDS=${AUTH_CLOCK_SKEW_TOLERANCE_SECONDS}
AUTH_REVOCATION_CACHE_REFRESH_INTERVAL_SECONDS=${AUTH_REVOCATION_CACHE_REFRESH_INTERVAL_SECONDS}
AUTH_REVOCATION_NATS_SUBJECT=${AUTH_REVOCATION_NATS_SUBJECT}
AUTH_AT_REST_HASHING_KEY=${AUTH_AT_REST_HASHING_KEY}
AUTH_PRODUCTION_SHARED_TOKEN_FALLBACK_ENABLED=${AUTH_PRODUCTION_SHARED_TOKEN_FALLBACK_ENABLED}
AUTH_TELEMETRY_ENABLED=${AUTH_TELEMETRY_ENABLED}
AUTH_TELEMETRY_METRIC_PREFIX=${AUTH_TELEMETRY_METRIC_PREFIX}
AUTH_BOOTSTRAP_TOKEN=${AUTH_BOOTSTRAP_TOKEN}
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
CORS_ALLOWED_ORIGINS=${CORS_ALLOWED_ORIGINS}
AGENT_SCENARIO_DIR=${AGENT_SCENARIO_DIR}
AGENT_SCENARIO_GLOB=${AGENT_SCENARIO_GLOB}
AGENT_HOT_RELOAD=${AGENT_HOT_RELOAD}
AGENT_ROUTING_CONFIDENCE_FLOOR=${AGENT_ROUTING_CONFIDENCE_FLOOR}
AGENT_ROUTING_CONSIDER_TOP_N=${AGENT_ROUTING_CONSIDER_TOP_N}
AGENT_ROUTING_FALLBACK_SCENARIO_ID=${AGENT_ROUTING_FALLBACK_SCENARIO_ID}
AGENT_ROUTING_EMBEDDING_MODEL=${AGENT_ROUTING_EMBEDDING_MODEL}
AGENT_TRACE_RETENTION_DAYS=${AGENT_TRACE_RETENTION_DAYS}
AGENT_TRACE_RECORD_LLM_MESSAGES=${AGENT_TRACE_RECORD_LLM_MESSAGES}
AGENT_TRACE_REDACT_MARKER=${AGENT_TRACE_REDACT_MARKER}
AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING=${AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING}
AGENT_DEFAULTS_TIMEOUT_MS_CEILING=${AGENT_DEFAULTS_TIMEOUT_MS_CEILING}
AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING=${AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING}
AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING=${AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING}
AGENT_PROVIDER_DEFAULT_PROVIDER=${AGENT_PROVIDER_DEFAULT_PROVIDER}
AGENT_PROVIDER_DEFAULT_MODEL=${AGENT_PROVIDER_DEFAULT_MODEL}
AGENT_PROVIDER_REASONING_PROVIDER=${AGENT_PROVIDER_REASONING_PROVIDER}
AGENT_PROVIDER_REASONING_MODEL=${AGENT_PROVIDER_REASONING_MODEL}
AGENT_PROVIDER_FAST_PROVIDER=${AGENT_PROVIDER_FAST_PROVIDER}
AGENT_PROVIDER_FAST_MODEL=${AGENT_PROVIDER_FAST_MODEL}
AGENT_PROVIDER_VISION_PROVIDER=${AGENT_PROVIDER_VISION_PROVIDER}
AGENT_PROVIDER_VISION_MODEL=${AGENT_PROVIDER_VISION_MODEL}
AGENT_PROVIDER_OCR_PROVIDER=${AGENT_PROVIDER_OCR_PROVIDER}
AGENT_PROVIDER_OCR_MODEL=${AGENT_PROVIDER_OCR_MODEL}
POSTGRES_CPU_LIMIT=${POSTGRES_CPU_LIMIT}
POSTGRES_MEMORY_LIMIT=${POSTGRES_MEMORY_LIMIT}
NATS_CPU_LIMIT=${NATS_CPU_LIMIT}
NATS_MEMORY_LIMIT=${NATS_MEMORY_LIMIT}
CORE_CPU_LIMIT=${CORE_CPU_LIMIT}
CORE_MEMORY_LIMIT=${CORE_MEMORY_LIMIT}
ML_CPU_LIMIT=${ML_CPU_LIMIT}
ML_MEMORY_LIMIT=${ML_MEMORY_LIMIT}
OLLAMA_CPU_LIMIT=${OLLAMA_CPU_LIMIT}
OLLAMA_MEMORY_LIMIT=${OLLAMA_MEMORY_LIMIT}
ML_MODEL_MEMORY_PROFILES_JSON=${ML_MODEL_MEMORY_PROFILES_JSON}
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

# Spec 046 FR-046-002 — payload + JetStream storage ceilings. Bytes-typed;
# values come from infrastructure.nats.{max_payload_bytes,max_file_store_bytes,
# max_mem_store_bytes}. Missing values fail loud at generate time.
max_payload: ${NATS_MAX_PAYLOAD_BYTES}

jetstream {
  store_dir: /data
  max_file_store: ${NATS_MAX_FILE_STORE_BYTES}
  max_memory_store: ${NATS_MAX_MEM_STORE_BYTES}
}

http_port: ${NATS_MONITOR_PORT}${NATS_AUTH_SECTION}"

printf '%s\n' "$NATS_CONF_CONTENT" > "$NATS_CONF_FILE"
chmod 0600 "$NATS_CONF_FILE"

echo "Generated $NATS_CONF_FILE"

# ─────────────────────────────────────────────────────────────────────
# Build-Once Deploy-Many: emit deterministic config bundle (per bubbles G074)
#
# When --bundle is set, package the generated env file + nats.conf into a
# deterministic tar.gz at <output-dir>/config-bundle-<env>-<sourceSha>.tar.gz.
# Determinism: same (sourceSha, env, smackerel.yaml content) MUST produce the
# same bundle bytes (and therefore the same sha256). Volatile content (the
# `Generated:` timestamp comment in the env file) is stripped from the bundle
# copy.
# ─────────────────────────────────────────────────────────────────────
if [[ "$EMIT_BUNDLE" == "true" ]]; then
  mkdir -p "$BUNDLE_OUTPUT_DIR"

  STAGE_DIR="$(mktemp -d)"
  trap 'rm -rf "$STAGE_DIR"' EXIT

  # Bundle layout (extracted by adapter `apply.sh` into <composeDir>/):
  #   ./app.env                    — generated env file (renamed from <env>.env)
  #   ./nats.conf                  — NATS server config
  #   ./docker-compose.yml         — deployment compose (no `build:` blocks)
  #   ./nats_contract.json         — schema registry consumed by ML sidecar
  #   ./prompt_contracts/*.yaml    — prompt YAMLs mounted into core + ml
  #   ./bundle-manifest.yaml       — manifest of files in this bundle
  #
  # Determinism: same (sourceSha, env, smackerel.yaml content,
  # deploy/compose.deploy.yml, config/prompt_contracts/, config/nats_contract.json)
  # MUST produce the same bundle bytes (and therefore the same sha256). Volatile
  # content (the `Generated:` timestamp comment in the env file) is stripped
  # from the bundle copy.
  COMPOSE_TEMPLATE="$REPO_ROOT/deploy/compose.deploy.yml"
  NATS_CONTRACT_FILE="$REPO_ROOT/config/nats_contract.json"
  PROMPT_CONTRACTS_DIR="$REPO_ROOT/config/prompt_contracts"

  [[ -f "$COMPOSE_TEMPLATE" ]] || { echo "ERROR: deploy compose template not found: $COMPOSE_TEMPLATE" >&2; exit 1; }
  [[ -f "$NATS_CONTRACT_FILE" ]] || { echo "ERROR: nats contract not found: $NATS_CONTRACT_FILE" >&2; exit 1; }
  [[ -d "$PROMPT_CONTRACTS_DIR" ]] || { echo "ERROR: prompt contracts dir not found: $PROMPT_CONTRACTS_DIR" >&2; exit 1; }

  # Strip the volatile `Generated:` line so the bundle is reproducible.
  # Renamed to app.env so the deploy compose can reference it generically.
  grep -v '^# Generated: ' "$OUTPUT_FILE" > "$STAGE_DIR/app.env"
  cp "$NATS_CONF_FILE" "$STAGE_DIR/nats.conf"
  cp "$COMPOSE_TEMPLATE" "$STAGE_DIR/docker-compose.yml"
  cp "$NATS_CONTRACT_FILE" "$STAGE_DIR/nats_contract.json"
  mkdir -p "$STAGE_DIR/prompt_contracts"
  cp "$PROMPT_CONTRACTS_DIR"/*.yaml "$STAGE_DIR/prompt_contracts/"
  chmod 0644 "$STAGE_DIR/app.env" "$STAGE_DIR/nats.conf" \
    "$STAGE_DIR/docker-compose.yml" "$STAGE_DIR/nats_contract.json" \
    "$STAGE_DIR/prompt_contracts"/*.yaml

  # Deterministic file list for bundle-manifest.yaml (sorted).
  PROMPT_FILES_LIST="$(cd "$STAGE_DIR" && find prompt_contracts -maxdepth 1 -type f -name '*.yaml' | LC_ALL=C sort)"

  {
    echo "bundleVersion: 1"
    echo "sourceSha: ${BUNDLE_SOURCE_SHA}"
    echo "environment: ${TARGET_ENV}"
    echo "files:"
    echo "  - app.env"
    echo "  - nats.conf"
    echo "  - docker-compose.yml"
    echo "  - nats_contract.json"
    while IFS= read -r f; do
      echo "  - $f"
    done <<< "$PROMPT_FILES_LIST"
  } > "$STAGE_DIR/bundle-manifest.yaml"
  chmod 0644 "$STAGE_DIR/bundle-manifest.yaml"

  BUNDLE_FILE="$BUNDLE_OUTPUT_DIR/config-bundle-${TARGET_ENV}-${BUNDLE_SOURCE_SHA}.tar.gz"

  # Build deterministic file list. tar's --sort=name will recurse into the
  # prompt_contracts directory automatically, so we only list it once. Top-level
  # files are listed in LC_ALL=C name order to make the argv deterministic too.
  TAR_FILES=(
    "app.env"
    "bundle-manifest.yaml"
    "docker-compose.yml"
    "nats.conf"
    "nats_contract.json"
    "prompt_contracts"
  )

  # Deterministic tar: sorted entries, fixed owner/group, fixed mtime, no extended attrs.
  tar \
    --sort=name \
    --owner=0 \
    --group=0 \
    --numeric-owner \
    --mtime='1970-01-01 00:00:00 UTC' \
    --format=ustar \
    -C "$STAGE_DIR" \
    -cf - \
    "${TAR_FILES[@]}" \
    | gzip -n -9 > "$BUNDLE_FILE"

  chmod 0644 "$BUNDLE_FILE"
  BUNDLE_SHA="$(sha256sum "$BUNDLE_FILE" | awk '{print $1}')"

  echo "Generated $BUNDLE_FILE"
  echo "  sha256: $BUNDLE_SHA"
  echo "  sourceSha: $BUNDLE_SOURCE_SHA"
  echo "  environment: $TARGET_ENV"
fi