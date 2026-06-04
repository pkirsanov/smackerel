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
        level5 = ""
        path = level1
      } else if (indent == 2) {
        level2 = key
        level3 = ""
        level4 = ""
        level5 = ""
        path = level1 "." level2
      } else if (indent == 4) {
        level3 = key
        level4 = ""
        level5 = ""
        path = level1 "." level2 "." level3
      } else if (indent == 6) {
        level4 = key
        level5 = ""
        path = level1 "." level2 "." level3 "." level4
      } else if (indent == 8) {
        level5 = key
        path = level1 "." level2 "." level3 "." level4 "." level5
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

# ─────────────────────────────────────────────────────────────────────
# Spec 052 FR-052-001 / FR-052-002 — SST secret-key manifest shell mirror.
#
# This array enumerates the env-var keys whose values are NEVER inlined
# into bundles for production-class targets. For those targets the SST
# loader emits __SECRET_PLACEHOLDER__<KEY>__ markers in app.env; the knb
# deploy adapter substitutes real values at apply time from a sops-encrypted
# secrets file (knb/smackerel/secrets/<target>.enc.env).
#
# 3-mirror system (drift detected by Scope 3 contract test
# internal/deploy/bundle_secret_contract_test.go):
#   1. yaml: config/smackerel.yaml::infrastructure.secret_keys
#   2. Go:   internal/config/secret_keys.go::SecretKeys()
#   3. shell: SHELL_SECRET_KEYS below
#
# To add a new managed secret: update all three mirrors AND ship a real
# value via the knb deploy adapter at knb/smackerel/secrets/<target>.enc.env.
SHELL_SECRET_KEYS=(
  POSTGRES_PASSWORD
  AUTH_SIGNING_ACTIVE_PRIVATE_KEY
  AUTH_AT_REST_HASHING_KEY
  AUTH_BOOTSTRAP_TOKEN
  TELEGRAM_BOT_TOKEN
  KEEP_GOOGLE_APP_PASSWORD
)

# Spec 052 FR-052-002 — production-class target mirror.
# When TARGET_ENV is in this list, the SST loader emits placeholders for
# every key in SHELL_SECRET_KEYS above. dev/test environments are NEVER
# in this list (they keep inline values for local-dev convenience per
# FR-052-011). Mirrors config/smackerel.yaml::infrastructure.production_class_targets.
SHELL_PRODUCTION_CLASS_TARGETS=(
  home-lab
)

# Returns the SHELL_SECRET_KEYS list one-per-line.
secret_keys_list() {
  printf '%s\n' "${SHELL_SECRET_KEYS[@]}"
}

# Returns the SHELL_PRODUCTION_CLASS_TARGETS list one-per-line.
production_class_targets_list() {
  printf '%s\n' "${SHELL_PRODUCTION_CLASS_TARGETS[@]}"
}

# Returns 0 if the argument matches a production-class target, 1 otherwise.
is_production_class_target() {
  local candidate="$1"
  local t
  for t in "${SHELL_PRODUCTION_CLASS_TARGETS[@]}"; do
    [[ "$t" == "$candidate" ]] && return 0
  done
  return 1
}

# Returns 0 if the argument matches a managed secret key, 1 otherwise.
in_secret_keys() {
  local candidate="$1"
  local k
  for k in "${SHELL_SECRET_KEYS[@]}"; do
    [[ "$k" == "$candidate" ]] && return 0
  done
  return 1
}
# ─────────────────────────────────────────────────────────────────────

PROJECT_NAME="$(required_value project.name)"
CORE_CONTAINER_PORT="$(required_value services.core.container_port)"
SHUTDOWN_TIMEOUT_S="$(required_value services.core.shutdown_timeout_s)"
ML_CONTAINER_PORT="$(required_value services.ml.container_port)"
ML_HEALTH_CACHE_TTL_S="$(required_value services.ml.health_cache_ttl_s)"
ML_READINESS_TIMEOUT_S="$(required_value services.ml.readiness_timeout_s)"
ML_PROCESSING_DEGRADED_FALLBACK_ENABLED="$(env_override_value ml_processing_degraded_fallback_enabled services.ml.processing_degraded_fallback_enabled)"
# Spec 050 FR-050-001/002/003/005 — ML sidecar health/worker isolation contract.
# All three values are SST-owned and required. The Python ML sidecar consumes
# them in ml/app/main.py::_check_required_config and ml/app/embedder.py to
# bound the dedicated embedding ThreadPoolExecutor (FR-050-002), document the
# health latency SLA (FR-050-003), and emit worker/queue metrics (FR-050-005).
ML_EMBEDDING_WORKERS="$(required_value services.ml.embedding_workers)"
ML_EMBEDDING_QUEUE_MAX="$(required_value services.ml.embedding_queue_max)"
ML_HEALTH_LATENCY_SLA_MS="$(required_value services.ml.health_latency_sla_ms)"

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
# Spec 045 BUG-045-001 — ollama envelope is per-environment overridable so a
# big-RAM host (e.g. home-lab) can opt-up to gemma4:26b / gpt-oss:20b /
# deepseek-r1:32b without forcing the same envelope on dev/test/laptop.
# Override by setting environments.<env>.ollama_memory_limit in smackerel.yaml.
OLLAMA_MEMORY_LIMIT="$(env_override_value ollama_memory_limit deploy_resources.ollama.memory)"
# Spec 049 FR-049-005(c) — Prometheus resource envelope. Fail-loud SST;
# the deploy adapter MUST emit PROMETHEUS_CPU_LIMIT and
# PROMETHEUS_MEMORY_LIMIT in app.env or compose aborts at substitution
# time when the `monitoring` profile is enabled.
PROMETHEUS_CPU_LIMIT="$(required_value deploy_resources.prometheus.cpus)"
PROMETHEUS_MEMORY_LIMIT="$(required_value deploy_resources.prometheus.memory)"
ML_MODEL_MEMORY_PROFILES_JSON="$(required_json_value services.ml.model_memory_profiles)"

# Spec 049 — Monitoring stack SST (FR-049-001 / FR-049-003).
# Every key is required (fail-loud). PROMETHEUS_IMAGE is pinned in
# config/smackerel.yaml monitoring.prometheus.image. Container port,
# scrape/evaluation interval, and retention come from the same SST
# block. Host port + volume name are per-environment (read below).
PROMETHEUS_IMAGE="$(required_value monitoring.prometheus.image)"
PROMETHEUS_CONTAINER_PORT="$(required_value monitoring.prometheus.container_port)"
PROMETHEUS_SCRAPE_INTERVAL_S="$(required_value monitoring.prometheus.scrape_interval_seconds)"
PROMETHEUS_EVALUATION_INTERVAL_S="$(required_value monitoring.prometheus.evaluation_interval_seconds)"
PROMETHEUS_RETENTION_DAYS="$(required_value monitoring.prometheus.retention_days)"

# Spec 048 FR-048-001 / FR-048-002 / FR-048-003 — Backup and Restore
# Automation SST. Every key is required; missing values fail loud here.
# The deploy adapter overlay supplies a real off-host destination via
# ${BACKUP_DESTINATION_URL}; this repo MUST NOT name a real bucket.
BACKUP_LOCAL_DIR="$(required_value backup.local_dir)"
BACKUP_STATUS_FILE="$(required_value backup.status_file)"
BACKUP_RETENTION_DAILY="$(required_value backup.retention_daily)"
BACKUP_RETENTION_WEEKLY="$(required_value backup.retention_weekly)"
BACKUP_WATCHER_POLL_SECONDS="$(required_value backup.watcher_poll_seconds)"

POSTGRES_USER="$(required_value infrastructure.postgres.user)"
# Spec 052 FR-052-007 / FR-052-010 — POSTGRES_PASSWORD resolution with
# placeholder substitution + env-override fallback. Resolution order:
#   1. If POSTGRES_PASSWORD env var is set (non-empty), use it as override.
#      Env override beats yaml AND skips placeholder mode (the operator is
#      explicitly providing a literal). The FR-051-005 dev-default check
#      still fires on the env-override literal (BS-052-006 regression).
#   2. Else if TARGET_ENV is a production-class target AND POSTGRES_PASSWORD
#      is in SHELL_SECRET_KEYS, emit __SECRET_PLACEHOLDER__POSTGRES_PASSWORD__
#      and short-circuit the FR-051-005 dev-default check (the placeholder
#      is not a credential — defense in depth moves to the knb adapter
#      substitution gate AND the Go-runtime placeholder rejection).
#   3. Else (dev/test, or non-production-class targets), use the yaml value.
#      The FR-051-005 dev-default check fires only for production-class
#      targets that opt out of placeholder mode (currently none — kept for
#      future-proofing).
POSTGRES_PASSWORD_SOURCE=""
if [[ -n "${POSTGRES_PASSWORD:-}" ]]; then
  # POSTGRES_PASSWORD already set in env → env-override path. Do not reassign.
  POSTGRES_PASSWORD_SOURCE="env"
elif is_production_class_target "$TARGET_ENV" && in_secret_keys "POSTGRES_PASSWORD"; then
  POSTGRES_PASSWORD="__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__"
  POSTGRES_PASSWORD_SOURCE="placeholder"
else
  POSTGRES_PASSWORD="$(required_value infrastructure.postgres.password)"
  POSTGRES_PASSWORD_SOURCE="yaml"
fi
# Spec 051 FR-051-005 / SCN-051-S02 — defense-in-depth dev-default rejection.
# Fires only when emitting a literal value (env override or yaml). Placeholder
# mode short-circuits the check because the placeholder marker is not a
# credential and is rejected by the knb adapter + Go runtime if it leaks.
# When the SST loader runs for a non-dev/test target (currently: home-lab; any
# future production-class target should be added to the case below), the
# Postgres password MUST NOT match a known dev-default value. The list below
# is the parallel grep-friendly mirror of internal/config/secrets.go's
# DevDBPasswords slice. Keep the two lists in sync.
#
# The error message MUST name the offending KEY without echoing the VALUE
# (FR-051-007 redaction contract).
if [[ "$POSTGRES_PASSWORD_SOURCE" != "placeholder" ]]; then
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
fi
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
  # Spec 061 SCOPE-06a Round 65 (D4 hybrid fix).
  OLLAMA_KEEP_ALIVE="$(required_value infrastructure.ollama.test.keep_alive)"
  OLLAMA_TEST_PREWARM_WARMUP_NUM_PREDICT="$(required_value infrastructure.ollama.test.prewarm_warmup_num_predict)"
  # Spec 061 SCOPE-06c (Round 71d) — prewarm threshold derived from tier-resolved
  # RETRIEVAL_QA_TIMEOUT_MS minus 1000 ms safety margin (tracks the tier instead
  # of the stale 55000 ms hardcoded value). RETRIEVAL_QA_TIMEOUT_MS is resolved
  # below; this assignment is overridden after the tier resolution block.
  OLLAMA_TEST_PREWARM_SECOND_CALL_MAX_MS="$(required_value infrastructure.ollama.test.prewarm_warmup_second_call_max_ms)"
else
  OLLAMA_IMAGE="$(required_value infrastructure.ollama.image)"
  OLLAMA_TEST_MODEL=""
  OLLAMA_TEST_PULL_TIMEOUT_SECONDS=""
  OLLAMA_TEST_REQUEST_TEMPERATURE=""
  OLLAMA_TEST_REQUEST_TOP_P=""
  OLLAMA_TEST_REQUEST_TOP_K=""
  OLLAMA_TEST_REQUEST_SEED=""
  OLLAMA_TEST_REQUEST_NUM_PREDICT=""
  # Spec 061 SCOPE-06a Round 65 — non-test envs use the upstream default
  # keep_alive ("5m"). Warmup-budget knobs are emitted empty (the prewarm
  # hook never runs outside the test env per smackerel.sh).
  OLLAMA_KEEP_ALIVE="$(required_value infrastructure.ollama.keep_alive)"
  OLLAMA_TEST_PREWARM_WARMUP_NUM_PREDICT=""
  OLLAMA_TEST_PREWARM_SECOND_CALL_MAX_MS=""
fi
LLM_PROVIDER="$(required_value llm.provider)"
# Spec 061 SCOPE-06c (Round 71d) — Hardware-tier × model-role matrix. The
# SMACKEREL_HARDWARE_TIER switch (sourced from .smackerel.local.env by
# smackerel.sh before any subcommand runs) selects the interactive-role model
# for all 6 model env vars below and the interactive-role retrieval-qa
# timeouts. Fail-loud per smackerel-no-defaults: missing/empty/invalid tier
# aborts here with [F061-HARDWARE-TIER-*]. Per-env overrides (e.g. home-lab
# opt-up to gemma4:26b) remain a tier-orthogonal layer: if
# environments.<env>.<key> is set, it wins; otherwise the tier matrix value.
if [[ -z "${SMACKEREL_HARDWARE_TIER-}" ]]; then
  echo "[F061-HARDWARE-TIER-MISSING] SMACKEREL_HARDWARE_TIER is required (set in .smackerel.local.env at repo root to cpu or accel; copy .smackerel.local.env.example to start). See specs/061-conversational-assistant/scopes.md SCOPE-06c." >&2
  exit 1
fi
if [[ "$SMACKEREL_HARDWARE_TIER" != "cpu" && "$SMACKEREL_HARDWARE_TIER" != "accel" ]]; then
  echo "[F061-HARDWARE-TIER-INVALID] SMACKEREL_HARDWARE_TIER must be 'cpu' or 'accel', got: '$SMACKEREL_HARDWARE_TIER'" >&2
  exit 1
fi
TIER_INTERACTIVE_MODEL="$(required_value "models.tiers.${SMACKEREL_HARDWARE_TIER}.interactive.model")"
TIER_INTERACTIVE_RETRIEVAL_QA_TIMEOUT_MS="$(required_value "models.tiers.${SMACKEREL_HARDWARE_TIER}.interactive.retrieval_qa_timeout_ms")"
TIER_INTERACTIVE_RETRIEVAL_QA_PER_TOOL_TIMEOUT_MS="$(required_value "models.tiers.${SMACKEREL_HARDWARE_TIER}.interactive.retrieval_qa_per_tool_timeout_ms")"
# BUG-061-003 — recipe_search per-tier budgets (mirror retrieval_qa).
TIER_INTERACTIVE_RECIPE_SEARCH_TIMEOUT_MS="$(required_value "models.tiers.${SMACKEREL_HARDWARE_TIER}.interactive.recipe_search_timeout_ms")"
TIER_INTERACTIVE_RECIPE_SEARCH_PER_TOOL_TIMEOUT_MS="$(required_value "models.tiers.${SMACKEREL_HARDWARE_TIER}.interactive.recipe_search_per_tool_timeout_ms")"

# Per-env override OR tier matrix interactive model (tier-orthogonal layer).
tier_interactive_model_or_override() {
  local override_key="$1"
  local value
  if value="$(yaml_get "environments.$TARGET_ENV.$override_key" 2>/dev/null)"; then
    printf '%s' "$value"
  else
    printf '%s' "$TIER_INTERACTIVE_MODEL"
  fi
}

# Spec 045 BUG-045-001 — per-env overrides preserved as tier-orthogonal layer.
LLM_MODEL="$(tier_interactive_model_or_override llm_model)"
LLM_API_KEY="$(required_value llm.api_key)"
OLLAMA_URL="$(required_value llm.ollama_url)"
OLLAMA_MODEL="$(tier_interactive_model_or_override ollama_model)"
OLLAMA_VISION_MODEL="$(tier_interactive_model_or_override ollama_vision_model)"
# Spec 061 SCOPE-06c — retrieval-qa-v1 budget resolves from tier matrix.
RETRIEVAL_QA_TIMEOUT_MS="$TIER_INTERACTIVE_RETRIEVAL_QA_TIMEOUT_MS"
RETRIEVAL_QA_PER_TOOL_TIMEOUT_MS="$TIER_INTERACTIVE_RETRIEVAL_QA_PER_TOOL_TIMEOUT_MS"
# BUG-061-003 — recipe-search-v1 budget resolves from tier matrix.
RECIPE_SEARCH_TIMEOUT_MS="$TIER_INTERACTIVE_RECIPE_SEARCH_TIMEOUT_MS"
RECIPE_SEARCH_PER_TOOL_TIMEOUT_MS="$TIER_INTERACTIVE_RECIPE_SEARCH_PER_TOOL_TIMEOUT_MS"
# Spec 061 SCOPE-06c — prewarm second-call threshold tracks the tier's
# interactive retrieval-qa budget (minus 1000 ms safety margin) instead of
# the stale YAML literal (which is preserved for legacy non-test callers but
# unused on the test path).
if [[ "$TARGET_ENV" == "test" ]]; then
  OLLAMA_TEST_PREWARM_SECOND_CALL_MAX_MS="$((RETRIEVAL_QA_TIMEOUT_MS - 1000))"
fi
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
# Spec 064 SCOPE-17 — TARGET_ENV=dev also auto-generates a token when
# runtime.auth_token is empty, mirroring the test behavior. Dev token is
# persisted into config/generated/dev.env across regens (read-back path
# below) so existing client tokens stay valid. The YAML stays "" to preserve
# the spec 051 contract (TestBundleSecretContract_AdversarialA4_OptOutDetector).
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
if [[ "$TARGET_ENV" == "dev" && -z "$SMACKEREL_AUTH_TOKEN" ]]; then
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
#
# BUG-051-001 — TARGET_ENV=home-lab MUST also override SMACKEREL_ENV to
# "production" so the runtime defense-in-depth fires on the home-lab tailnet
# bundle. Without this override, a home-lab bundle generated against the
# default smackerel.yaml (runtime.environment: development) emits
# SMACKEREL_ENV=development, which silently disables:
#   - internal/auth/startup.go::ValidateRuntimeAuthStartup (returns nil unless
#     environment=="production"),
#   - internal/config/config.go production-mode auth + DB-password fail-fast
#     (gated on cfg.Environment=="production"),
#   - the spec 044 production-mode signing-material requirements,
#   - the spec 051 FR-051-005 dev-default Postgres password rejection at
#     runtime (the generator-side guard at lines ~415-433 still fires, but
#     the runtime-side guard becomes a no-op).
# The resulting bundle would auth-bypass on the home-lab tailnet endpoint and
# collapse spec 044 + spec 051 defense-in-depth to bundle-generator-only,
# violating the SEC-HL-001 finding from the home-lab readiness review
# 2026-05-13. The per-target case below is the single fix point: it preserves
# the existing TARGET_ENV=test override and adds the home-lab→production
# override required by BUG-051-001.
SMACKEREL_ENV="$(required_value runtime.environment)"
case "$SMACKEREL_ENV" in
  development|test|production) ;;
  *)
    echo "Error: runtime.environment must be one of development|test|production, got '$SMACKEREL_ENV'" >&2
    exit 1
    ;;
esac
case "$TARGET_ENV" in
  test)
    SMACKEREL_ENV="test"
    ;;
  home-lab)
    SMACKEREL_ENV="production"
    ;;
esac
# Spec 052 FR-052-007 — placeholder substitution: when TARGET_ENV is a
# production-class target AND TELEGRAM_BOT_TOKEN is in SHELL_SECRET_KEYS, emit
# the deterministic placeholder for the deploy adapter to substitute from sops.
# In dev/test, fall back to the yaml-literal value (empty string is fine for
# local dev — the bot stays disabled at startup until a real value is set).
if is_production_class_target "$TARGET_ENV" && in_secret_keys "TELEGRAM_BOT_TOKEN"; then
  TELEGRAM_BOT_TOKEN="__SECRET_PLACEHOLDER__TELEGRAM_BOT_TOKEN__"
else
  TELEGRAM_BOT_TOKEN="$(yaml_get telegram.bot_token 2>/dev/null)" || TELEGRAM_BOT_TOKEN=""
fi
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

# Chrome Extension Bridge ingest (spec 058 SST-owned; no generator defaults)
EXTENSION_INGEST_ENABLED="$(required_value extension.ingest.enabled)"
EXTENSION_INGEST_MAX_BATCH_ITEMS="$(required_value extension.ingest.max_batch_items)"
EXTENSION_INGEST_MAX_BODY_BYTES="$(required_value extension.ingest.max_body_bytes)"
EXTENSION_INGEST_DEFAULT_DEDUP_WINDOW_SECONDS="$(required_value extension.ingest.default_dedup_window_seconds)"
EXTENSION_INGEST_ACCEPTED_CONTENT_TYPES="$(required_json_value extension.ingest.accepted_content_types)"
EXTENSION_INGEST_REQUIRED_TOKEN_SCOPE="$(required_value extension.ingest.required_token_scope)"

# Knowledge Graph Public API (spec 080 SCOPE-080-01 SST-owned; no generator defaults)
KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT="$(required_value knowledge_graph_api.list_default_limit)"
KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT="$(required_value knowledge_graph_api.list_max_limit)"
KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS="$(required_value knowledge_graph_api.time_window_max_days)"
KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT="$(required_value knowledge_graph_api.edges_default_limit)"
KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT="$(required_value knowledge_graph_api.edges_max_limit)"
KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV="$(required_value knowledge_graph_api.cursor_secret_env)"

# Spec 080 SCOPE-080-02 — auto-generate the cursor HMAC secret for
# dev/test envs when it is not yet present in the environment. Prod
# operators inject the real secret via the spec 052 secret path; the
# generator never invents one for prod. Mirrors the SMACKEREL_AUTH_TOKEN
# dev/test ergonomic above (reuse existing value across regens; mint a
# fresh one only when no existing value is found).
KNOWLEDGE_GRAPH_API_CURSOR_SECRET="${KNOWLEDGE_GRAPH_API_CURSOR_SECRET:-}"
if [[ ( "$TARGET_ENV" == "test" || "$TARGET_ENV" == "dev" ) && -z "$KNOWLEDGE_GRAPH_API_CURSOR_SECRET" ]]; then
  EXISTING_ENV_FILE="$REPO_ROOT/config/generated/${TARGET_ENV}.env"
  EXISTING_SECRET=""
  if [[ -f "$EXISTING_ENV_FILE" ]]; then
    EXISTING_SECRET="$(awk -F= '/^KNOWLEDGE_GRAPH_API_CURSOR_SECRET=/ { sub(/^KNOWLEDGE_GRAPH_API_CURSOR_SECRET=/, ""); print; exit }' "$EXISTING_ENV_FILE" 2>/dev/null || true)"
  fi
  if [[ -n "$EXISTING_SECRET" ]]; then
    KNOWLEDGE_GRAPH_API_CURSOR_SECRET="$EXISTING_SECRET"
  else
    KNOWLEDGE_GRAPH_API_CURSOR_SECRET="$(openssl rand -hex 32 2>/dev/null || python3 -c 'import secrets; print(secrets.token_hex(32))')"
  fi
fi

# Unified surfacing controller (spec 021 Scope 4 SST-owned; no generator defaults)
SURFACING_DAILY_NUDGE_BUDGET="$(required_value surfacing.daily_nudge_budget)"
SURFACING_SUPPRESSION_WINDOW_HOURS="$(required_value surfacing.suppression_window_hours)"
SURFACING_DEDUPE_WINDOW_HOURS="$(required_value surfacing.dedupe_window_hours)"
SURFACING_URGENT_ESCALATION_ENABLED="$(required_value surfacing.urgent_escalation_enabled)"

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

# Notification intelligence configuration (spec 054 SST-owned; no generator defaults)
NOTIFICATION_INTELLIGENCE_ENABLED="$(required_value notification_intelligence.enabled)"
NOTIFICATION_PERSISTENCE_THRESHOLD="$(required_value notification_intelligence.persistence_threshold)"
NOTIFICATION_ESCALATION_SEVERITY="$(required_value notification_intelligence.escalation_severity)"
NOTIFICATION_LOW_CONFIDENCE_THRESHOLD="$(required_value notification_intelligence.low_confidence_threshold)"
NOTIFICATION_MAX_RETRIES="$(required_value notification_intelligence.max_retries)"
NOTIFICATION_OUTPUT_CHANNELS="$(required_json_value notification_outputs.channels)"
NTFY_SOURCES_JSON="$(required_json_value notification_sources.ntfy.instances)"
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

# Spec 067 policy guards (SST-sourced thresholds)
POLICY_SCENARIO_PROMPT_MAX_LINES="$(required_value policy.scenario_prompt_max_lines)"
POLICY_EXCEPTION_BASELINE_PATH="$(required_value policy.policy_exception_baseline_path)"
POLICY_EXCEPTION_MAX_AGE_DAYS="$(required_value policy.policy_exception_max_age_days)"
POLICY_INTENT_BYPASS_GUARD_ENABLED="$(required_value policy.intent_bypass_guard_enabled)"

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
# Spec 064 SCOPE-07 — SearxNG per-env shape. The `searxng` Compose
# profile is enabled iff SEARXNG_ENABLED=true in the generated env
# file. The integration leg in smackerel.sh injects
# OPEN_KNOWLEDGE_SEARXNG_URL=http://searxng:8080 into the Go runner.
SEARXNG_ENABLED="$(required_value environments.$TARGET_ENV.searxng_enabled)"
SEARXNG_HOST_PORT="$(required_value environments.$TARGET_ENV.searxng_host_port)"
SEARXNG_BIND_ADDRESS="$(required_value environments.$TARGET_ENV.searxng_bind_address)"
SEARXNG_IMAGE="$(required_value assistant.open_knowledge.searxng.image)"
SEARXNG_CONTAINER_PORT="$(required_value assistant.open_knowledge.searxng.container_port)"
SEARXNG_SECRET="$(required_value assistant.open_knowledge.searxng.secret_key)"
SEARXNG_BASE_URL="$(required_value assistant.open_knowledge.searxng.base_url)"
# Spec 049 — per-environment Prometheus host port. Used only when the
# `monitoring` Compose profile is enabled; the service is off by
# default. Fail-loud SST: every supported environment in
# config/smackerel.yaml MUST declare prometheus_host_port.
PROMETHEUS_HOST_PORT="$(required_value environments.$TARGET_ENV.prometheus_host_port)"
POSTGRES_VOLUME_NAME="$(required_value environments.$TARGET_ENV.postgres_volume_name)"
NATS_VOLUME_NAME="$(required_value environments.$TARGET_ENV.nats_volume_name)"
OLLAMA_VOLUME_NAME="$(required_value environments.$TARGET_ENV.ollama_volume_name)"
# Spec 049 — per-environment Prometheus TSDB volume name. Keeps dev,
# test, and home-lab data fully separated even when the same image is
# reused; also satisfies spec 045's docker-lifecycle isolation contract.
PROMETHEUS_VOLUME_NAME="$(required_value environments.$TARGET_ENV.prometheus_volume_name)"

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

# Spec 059 Scope 1 — Google Keep live-sync credentials.
# KEEP_GOOGLE_EMAIL is non-secret (standard env contract, yaml-driven).
# KEEP_GOOGLE_APP_PASSWORD is Bucket-2 secret: production-class targets get
# the deterministic spec-052 placeholder; dev/test fall through to the yaml
# literal (empty by default — fine for local dev; the connector fails loud
# at Connect() time when sync_mode ∈ {gkeepapi, hybrid} and gkeep_enabled).
KEEP_GOOGLE_EMAIL="$(yaml_get connectors.google-keep.email 2>/dev/null)" || KEEP_GOOGLE_EMAIL=""
if is_production_class_target "$TARGET_ENV" && in_secret_keys "KEEP_GOOGLE_APP_PASSWORD"; then
  KEEP_GOOGLE_APP_PASSWORD="__SECRET_PLACEHOLDER__KEEP_GOOGLE_APP_PASSWORD__"
else
  KEEP_GOOGLE_APP_PASSWORD="$(yaml_get connectors.google-keep.app_password 2>/dev/null)" || KEEP_GOOGLE_APP_PASSWORD=""
fi

# CORS config
CORS_ALLOWED_ORIGINS_JSON="$(yaml_get_json cors.allowed_origins 2>/dev/null)" || CORS_ALLOWED_ORIGINS_JSON="[]"
# Convert JSON array to comma-separated string for env var
CORS_ALLOWED_ORIGINS=""
if [[ "$CORS_ALLOWED_ORIGINS_JSON" != "[]" && -n "$CORS_ALLOWED_ORIGINS_JSON" ]]; then
  CORS_ALLOWED_ORIGINS="$(python3 -c "import json,sys; print(','.join(json.loads(sys.argv[1])))" "$CORS_ALLOWED_ORIGINS_JSON" 2>/dev/null)" || CORS_ALLOWED_ORIGINS=""
fi

# Runtime trusted reverse-proxy CIDR allowlist (BUG-020-005, F-SEC-R30-001).
# Mirrors the CORS_ALLOWED_ORIGINS pattern: YAML list → JSON → CSV env var.
# Empty list (the SST default) → empty CSV → Go side treats every TCP peer
# as untrusted → forwarded headers ignored → per-IP rate limits keyed on
# the raw TCP peer. See config/smackerel.yaml runtime.trusted_proxies.
RUNTIME_TRUSTED_PROXIES_JSON="$(yaml_get_json runtime.trusted_proxies 2>/dev/null)" || RUNTIME_TRUSTED_PROXIES_JSON="[]"
RUNTIME_TRUSTED_PROXIES=""
if [[ "$RUNTIME_TRUSTED_PROXIES_JSON" != "[]" && -n "$RUNTIME_TRUSTED_PROXIES_JSON" ]]; then
  RUNTIME_TRUSTED_PROXIES="$(python3 -c "import json,sys; print(','.join(json.loads(sys.argv[1])))" "$RUNTIME_TRUSTED_PROXIES_JSON" 2>/dev/null)" || RUNTIME_TRUSTED_PROXIES=""
fi

# Connector import paths — SST repo-default fallback (BUG-029-005 / HL-RESCAN-012 / Gate G028).
# Each of the 4 mount-path vars (BOOKMARKS_IMPORT_DIR, MAPS_IMPORT_DIR, BROWSER_HISTORY_PATH,
# TWITTER_ARCHIVE_DIR) resolves with precedence: (1) shell env value, (2) yaml value via yaml_get,
# (3) repo-default host fixture path. The defaults are SST emission-time placeholders — visible in
# config/generated/<env>.env and auditable — not Compose-substitution-time defaults (which Gate
# G028 forbids per the BUG-029-003 DD-2 precedent). The connector starts iff <Connector>_ENABLED is
# true; the path is consumed by the connector to locate fixture files (gitkept empty dirs by default).
BOOKMARKS_ENABLED="$(yaml_get connectors.bookmarks.enabled 2>/dev/null)" || BOOKMARKS_ENABLED="false"
BOOKMARKS_SYNC_SCHEDULE="$(yaml_get connectors.bookmarks.sync_schedule 2>/dev/null)" || BOOKMARKS_SYNC_SCHEDULE=""
BOOKMARKS_IMPORT_DIR="$(yaml_get connectors.bookmarks.import_dir 2>/dev/null)" || BOOKMARKS_IMPORT_DIR=""
if [[ -z "$BOOKMARKS_IMPORT_DIR" ]]; then BOOKMARKS_IMPORT_DIR="./data/bookmarks-import"; fi
BOOKMARKS_WATCH_INTERVAL="$(yaml_get connectors.bookmarks.watch_interval 2>/dev/null)" || BOOKMARKS_WATCH_INTERVAL=""
BOOKMARKS_ARCHIVE_PROCESSED="$(yaml_get connectors.bookmarks.archive_processed 2>/dev/null)" || BOOKMARKS_ARCHIVE_PROCESSED=""
BOOKMARKS_PROCESSING_TIER="$(yaml_get connectors.bookmarks.processing_tier 2>/dev/null)" || BOOKMARKS_PROCESSING_TIER=""
BOOKMARKS_MIN_URL_LENGTH="$(yaml_get connectors.bookmarks.min_url_length 2>/dev/null)" || BOOKMARKS_MIN_URL_LENGTH=""
BOOKMARKS_EXCLUDE_DOMAINS="$(yaml_get connectors.bookmarks.exclude_domains 2>/dev/null)" || BOOKMARKS_EXCLUDE_DOMAINS=""
MAPS_ENABLED="$(yaml_get connectors.google-maps-timeline.enabled 2>/dev/null)" || MAPS_ENABLED="false"
MAPS_SYNC_SCHEDULE="$(yaml_get connectors.google-maps-timeline.sync_schedule 2>/dev/null)" || MAPS_SYNC_SCHEDULE=""
MAPS_IMPORT_DIR="$(yaml_get connectors.google-maps-timeline.import_dir 2>/dev/null)" || MAPS_IMPORT_DIR=""
if [[ -z "$MAPS_IMPORT_DIR" ]]; then MAPS_IMPORT_DIR="./data/maps-import"; fi
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
if [[ -z "$BROWSER_HISTORY_PATH" ]]; then BROWSER_HISTORY_PATH="./data/browser-history/History"; fi
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
# BUG-020-010 — callback HMAC bridge signing keystore JSON. PERMISSIVE:
# empty allowed ("signing not configured in this environment"); non-empty
# parsed at boot by internal/config.Validate(). The deploy adapter MAY
# override with QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON in the environment.
QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON="$(env_override_value qf_decisions_callback_signing_keys_json connectors.qf-decisions.callback_signing_keys_json)"

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
if [[ -z "$TWITTER_ARCHIVE_DIR" ]]; then TWITTER_ARCHIVE_DIR="./data/twitter-archive"; fi
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
# BUG-020-009 — fail-loud SST: required, no default.
FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS="$(required_value connectors.financial-markets.http_timeout_seconds)"
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
# Spec 052 FR-052-007 — placeholder substitution for managed secret keys.
# When TARGET_ENV is a production-class target, the SST loader emits
# __SECRET_PLACEHOLDER__<KEY>__ instead of the literal yaml value; the knb
# deploy adapter substitutes the real value at apply time. dev/test keep
# the inline yaml-or-empty pattern (FR-052-011, SCN-AUTH-005).
if is_production_class_target "$TARGET_ENV" && in_secret_keys "AUTH_SIGNING_ACTIVE_PRIVATE_KEY"; then
  AUTH_SIGNING_ACTIVE_PRIVATE_KEY="__SECRET_PLACEHOLDER__AUTH_SIGNING_ACTIVE_PRIVATE_KEY__"
else
  AUTH_SIGNING_ACTIVE_PRIVATE_KEY="$(yaml_get auth.signing.active_private_key 2>/dev/null)" || AUTH_SIGNING_ACTIVE_PRIVATE_KEY=""
fi
AUTH_SIGNING_ACTIVE_KEY_ID="$(yaml_get auth.signing.active_key_id 2>/dev/null)" || AUTH_SIGNING_ACTIVE_KEY_ID=""
AUTH_SIGNING_PRIOR_PUBLIC_KEY="$(yaml_get auth.signing.prior_public_key 2>/dev/null)" || AUTH_SIGNING_PRIOR_PUBLIC_KEY=""
AUTH_SIGNING_PRIOR_KEY_ID="$(yaml_get auth.signing.prior_key_id 2>/dev/null)" || AUTH_SIGNING_PRIOR_KEY_ID=""
AUTH_TOKEN_TTL_HOURS="$(required_value auth.token_ttl_hours)"
AUTH_ROTATION_GRACE_WINDOW_HOURS="$(required_value auth.rotation_grace_window_hours)"
AUTH_CLOCK_SKEW_TOLERANCE_SECONDS="$(required_value auth.clock_skew_tolerance_seconds)"
AUTH_REVOCATION_CACHE_REFRESH_INTERVAL_SECONDS="$(required_value auth.revocation_cache_refresh_interval_seconds)"
AUTH_REVOCATION_NATS_SUBJECT="$(required_value auth.revocation_nats_subject)"
# BUG-020-009 — fail-loud SST: required, no default.
AUTH_OAUTH_HTTP_TIMEOUT_SECONDS="$(required_value auth.oauth.http_timeout_seconds)"
# Spec 052 FR-052-007 — placeholder substitution (see comment above).
if is_production_class_target "$TARGET_ENV" && in_secret_keys "AUTH_AT_REST_HASHING_KEY"; then
  AUTH_AT_REST_HASHING_KEY="__SECRET_PLACEHOLDER__AUTH_AT_REST_HASHING_KEY__"
else
  AUTH_AT_REST_HASHING_KEY="$(yaml_get auth.at_rest_hashing_key 2>/dev/null)" || AUTH_AT_REST_HASHING_KEY=""
fi
AUTH_PRODUCTION_SHARED_TOKEN_FALLBACK_ENABLED="$(required_value auth.production_shared_token_fallback_enabled)"
AUTH_TELEMETRY_ENABLED="$(required_value auth.telemetry_enabled)"
AUTH_TELEMETRY_METRIC_PREFIX="$(required_value auth.telemetry_metric_prefix)"
# Spec 052 FR-052-007 — placeholder substitution (see comment above).
if is_production_class_target "$TARGET_ENV" && in_secret_keys "AUTH_BOOTSTRAP_TOKEN"; then
  AUTH_BOOTSTRAP_TOKEN="__SECRET_PLACEHOLDER__AUTH_BOOTSTRAP_TOKEN__"
else
  AUTH_BOOTSTRAP_TOKEN="$(yaml_get auth.bootstrap_token 2>/dev/null)" || AUTH_BOOTSTRAP_TOKEN=""
fi

# Agent (spec 037 — LLM Scenario Agent & Tool Registry). SST zero-defaults:
# every value is REQUIRED. Missing keys → config generate exits non-zero.
AGENT_SCENARIO_DIR="$(required_value agent.scenario_dir)"
AGENT_SCENARIO_GLOB="$(required_value agent.scenario_glob)"
AGENT_HOT_RELOAD="$(required_value agent.hot_reload)"
AGENT_ROUTING_CONFIDENCE_FLOOR="$(required_value agent.routing.confidence_floor)"
AGENT_ROUTING_CONSIDER_TOP_N="$(required_value agent.routing.consider_top_n)"
AGENT_ROUTING_FALLBACK_SCENARIO_ID="$(required_value agent.routing.fallback_scenario_id)"
AGENT_ROUTING_EMBEDDING_MODEL="$(required_value agent.routing.embedding_model)"
# BUG-061-004 — assistant NL routing embedder substrate.
ASSISTANT_ROUTING_EMBEDDER_MODE="$(required_value agent.routing.embedder_mode)"
ASSISTANT_ROUTING_EMBED_TIMEOUT_MS="$(required_value agent.routing.embed_timeout_ms)"
AGENT_TRACE_RETENTION_DAYS="$(required_value agent.trace.retention_days)"
AGENT_TRACE_RECORD_LLM_MESSAGES="$(required_value agent.trace.record_llm_messages)"
AGENT_TRACE_REDACT_MARKER="$(required_value agent.trace.redact_marker)"
AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING="$(required_value agent.defaults.max_loop_iterations_ceiling)"
AGENT_DEFAULTS_TIMEOUT_MS_CEILING="$(required_value agent.defaults.timeout_ms_ceiling)"
AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING="$(required_value agent.defaults.schema_retry_budget_ceiling)"
AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING="$(required_value agent.defaults.per_tool_timeout_ms_ceiling)"
AGENT_PROVIDER_DEFAULT_PROVIDER="$(required_value agent.provider_routing.default.provider)"
# Spec 061 SCOPE-06c (Round 71d) — AGENT_PROVIDER_DEFAULT_MODEL resolves
# from the tier × interactive cell (per-env override wins if set). Replaces
# the SCOPE-06a fallback to agent.provider_routing.default.model so the
# retrieval-qa-v1 (model_preference: "default") scenario gets the
# tier-correct interactive model on both tiers.
AGENT_PROVIDER_DEFAULT_MODEL="$(tier_interactive_model_or_override agent_provider_default_model)"
AGENT_PROVIDER_REASONING_PROVIDER="$(required_value agent.provider_routing.reasoning.provider)"
AGENT_PROVIDER_REASONING_MODEL="$(required_value agent.provider_routing.reasoning.model)"
AGENT_PROVIDER_FAST_PROVIDER="$(required_value agent.provider_routing.fast.provider)"
# Spec 061 SCOPE-06c — fast tier model also resolves from tier × interactive.
AGENT_PROVIDER_FAST_MODEL="$(tier_interactive_model_or_override agent_provider_fast_model)"
AGENT_PROVIDER_VISION_PROVIDER="$(required_value agent.provider_routing.vision.provider)"
# Spec 061 SCOPE-06c — vision falls back to the interactive cell on cpu (no
# separate vision model on 0.5b); accel can opt-up via environments.<env>.
AGENT_PROVIDER_VISION_MODEL="$(tier_interactive_model_or_override agent_provider_vision_model)"
AGENT_PROVIDER_OCR_PROVIDER="$(required_value agent.provider_routing.ocr.provider)"
AGENT_PROVIDER_OCR_MODEL="$(required_value agent.provider_routing.ocr.model)"

# Assistant (spec 061 — Conversational Assistant, Transport-Agnostic). SST
# zero-defaults: every key is REQUIRED. Missing values → exit non-zero with
# the offending key named (the [F061-SST-MISSING] prefix is added by the Go
# loader at startup; required_value exits earlier here at config-generate).
ASSISTANT_ENABLED="$(required_value assistant.enabled)"
ASSISTANT_BORDERLINE_FLOOR="$(required_value assistant.borderline_floor)"
ASSISTANT_CONTEXT_WINDOW_TURNS="$(required_value assistant.context.window_turns)"
ASSISTANT_CONTEXT_IDLE_TIMEOUT="$(required_value assistant.context.idle_timeout)"
ASSISTANT_CONTEXT_IDLE_SWEEP_INTERVAL="$(required_value assistant.context.idle_sweep_interval)"
ASSISTANT_CONTEXT_STATE_KEY="$(required_value assistant.context.state_key)"
ASSISTANT_SOURCES_MAX="$(required_value assistant.sources_max)"
ASSISTANT_BODY_MAX_CHARS="$(required_value assistant.body_max_chars)"
ASSISTANT_STATUS_MAX_DURATION="$(required_value assistant.status_max_duration)"
ASSISTANT_DISAMBIGUATE_TIMEOUT="$(required_value assistant.disambiguate_timeout)"
ASSISTANT_ERROR_CAPTURE_TIMEOUT="$(required_value assistant.error.capture_timeout)"
ASSISTANT_RATE_LIMIT_RETRIEVAL_RPM="$(required_value assistant.rate_limit.retrieval.requests_per_minute)"
ASSISTANT_RATE_LIMIT_WEATHER_RPM="$(required_value assistant.rate_limit.weather.requests_per_minute)"
ASSISTANT_RATE_LIMIT_NOTIFICATIONS_RPM="$(required_value assistant.rate_limit.notifications.requests_per_minute)"
# BUG-061-003 — recipe_search rate limit + skill SST.
ASSISTANT_RATE_LIMIT_RECIPE_SEARCH_RPM="$(required_value assistant.rate_limit.recipe_search.requests_per_minute)"
ASSISTANT_SKILLS_RETRIEVAL_ENABLED="$(required_value assistant.skills.retrieval.enabled)"
ASSISTANT_SKILLS_RETRIEVAL_TOP_K="$(required_value assistant.skills.retrieval.top_k)"
ASSISTANT_SKILLS_RECIPE_SEARCH_ENABLED="$(required_value assistant.skills.recipe_search.enabled)"
ASSISTANT_SKILLS_RECIPE_SEARCH_TOP_K="$(required_value assistant.skills.recipe_search.top_k)"
ASSISTANT_SKILLS_WEATHER_ENABLED="$(required_value assistant.skills.weather.enabled)"
ASSISTANT_SKILLS_WEATHER_PROVIDER="$(required_value assistant.skills.weather.provider)"
# weather.api_key_ref is permissively-empty (provider may not require a key);
# yaml_get returns "" when present with empty value, or aborts when missing.
ASSISTANT_SKILLS_WEATHER_API_KEY_REF="$(yaml_get assistant.skills.weather.api_key_ref)"
ASSISTANT_SKILLS_WEATHER_CACHE_TTL="$(required_value assistant.skills.weather.cache_ttl)"
# Spec 061 design §18.3 — per-URL SST keys for external-provider URL
# injection seam. Production reads literal values from smackerel.yaml;
# the TARGET_ENV=test override below redirects them to the in-tree
# stub container so shell e2e fixtures are hermetic.
ASSISTANT_SKILLS_WEATHER_GEOCODE_URL="$(required_value assistant.skills.weather.geocode_url)"
ASSISTANT_SKILLS_WEATHER_FORECAST_URL="$(required_value assistant.skills.weather.forecast_url)"
ASSISTANT_SKILLS_NOTIFICATIONS_ENABLED="$(required_value assistant.skills.notifications.enabled)"
ASSISTANT_SKILLS_NOTIFICATIONS_CONFIRM_TIMEOUT="$(required_value assistant.skills.notifications.confirm_timeout)"
ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED="$(required_value assistant.transports.telegram.enabled)"
ASSISTANT_TRANSPORTS_TELEGRAM_MARKDOWN_MODE="$(required_value assistant.transports.telegram.markdown_mode)"
ASSISTANT_TRANSPORTS_TELEGRAM_MAX_MESSAGE_CHARS="$(required_value assistant.transports.telegram.max_message_chars)"
# Spec 061 SCOPE-05 design §17.5 — Telegram webhook mode SST.
ASSISTANT_TRANSPORTS_TELEGRAM_MODE="$(required_value assistant.transports.telegram.mode)"
# webhook_secret_ref is permissively-empty when mode=long_poll; the Go
# validator enforces non-empty resolution when mode=webhook.
ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF="$(yaml_get assistant.transports.telegram.webhook_secret_ref)"
ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH="$(required_value assistant.transports.telegram.webhook_path)"

# Spec 072 SCOPE-1 — WhatsApp Business Cloud API transport SST (NO defaults).
ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED="$(required_value assistant.transports.whatsapp.enabled)"
ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_PATH="$(required_value assistant.transports.whatsapp.webhook_path)"
ASSISTANT_TRANSPORTS_WHATSAPP_PHONE_NUMBER_ID="$(required_value assistant.transports.whatsapp.phone_number_id)"
ASSISTANT_TRANSPORTS_WHATSAPP_BUSINESS_ACCOUNT_ID="$(required_value assistant.transports.whatsapp.business_account_id)"
ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_VERIFY_TOKEN_REF="$(required_value assistant.transports.whatsapp.webhook_verify_token_ref)"
ASSISTANT_TRANSPORTS_WHATSAPP_APP_SECRET_REF="$(required_value assistant.transports.whatsapp.app_secret_ref)"
ASSISTANT_TRANSPORTS_WHATSAPP_ACCESS_TOKEN_REF="$(required_value assistant.transports.whatsapp.access_token_ref)"
ASSISTANT_TRANSPORTS_WHATSAPP_IDENTITY_HASH_KEY_REF="$(required_value assistant.transports.whatsapp.identity_hash_key_ref)"
ASSISTANT_TRANSPORTS_WHATSAPP_API_BASE_URL="$(required_value assistant.transports.whatsapp.api_base_url)"
ASSISTANT_TRANSPORTS_WHATSAPP_API_VERSION="$(required_value assistant.transports.whatsapp.api_version)"
ASSISTANT_TRANSPORTS_WHATSAPP_RATE_LIMIT_PER_USER_PER_MINUTE="$(required_value assistant.transports.whatsapp.rate_limit_per_user_per_minute)"
ASSISTANT_TRANSPORTS_WHATSAPP_MAX_TEXT_CHARS="$(required_value assistant.transports.whatsapp.max_text_chars)"

# Spec 069 SCOPE-1c-bis — HTTP transport SST (NO defaults). Every key
# REQUIRED at the generator boundary; loader fails loud naming any
# absent key. List-shaped keys are serialized as CSV (no JSON in env).
ASSISTANT_TRANSPORTS_HTTP_ENABLED="$(required_value assistant.transports.http.enabled)"
ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION="$(required_value assistant.transports.http.schema_version)"
ASSISTANT_TRANSPORTS_HTTP_BODY_SIZE_MAX_BYTES="$(required_value assistant.transports.http.body_size_max_bytes)"
ASSISTANT_TRANSPORTS_HTTP_RATE_LIMIT_PER_USER_PER_MINUTE="$(required_value assistant.transports.http.rate_limit_per_user_per_minute)"
ASSISTANT_TRANSPORTS_HTTP_CONVERSATION_TTL_SECONDS="$(required_value assistant.transports.http.conversation_ttl_seconds)"
ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE="$(required_value assistant.transports.http.required_scope)"
ASSISTANT_TRANSPORTS_HTTP_SHARED_USER_ID="$(required_value assistant.transports.http.shared_user_id)"
# CORS origins list is REQUIRED (key must be PRESENT) but may resolve
# to an empty CSV when the SST list is []. The Go loader accepts the
# empty value as "same-origin only".
ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS_JSON="$(yaml_get_json assistant.transports.http.cors_allowed_origins 2>/dev/null)" || ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS_JSON="[]"
ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS=""
if [[ "$ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS_JSON" != "[]" && -n "$ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS_JSON" ]]; then
  ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS="$(python3 -c "import json,sys; print(','.join(json.loads(sys.argv[1])))" "$ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS_JSON" 2>/dev/null)" || ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS=""
fi
# Transport-hint allowlist MUST be non-empty when enabled=true; the
# Go validator enforces it. The generator emits whatever the yaml
# resolved to.
ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST_JSON="$(yaml_get_json assistant.transports.http.transport_hint_allowlist 2>/dev/null)" || ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST_JSON="[]"
ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST=""
if [[ "$ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST_JSON" != "[]" && -n "$ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST_JSON" ]]; then
  ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST="$(python3 -c "import json,sys; print(','.join(json.loads(sys.argv[1])))" "$ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST_JSON" 2>/dev/null)" || ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST=""
fi

# Spec 061 SCOPE-10 — offline evaluation harness acceptance gates.
# Read by internal/config/assistant.go and consumed by
# tests/eval/assistant/harness.go to assert routing accuracy and
# capture-fallback coverage thresholds.
ASSISTANT_EVAL_ROUTING_ACCURACY_MIN="$(required_value assistant.eval.routing_accuracy_min)"
ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN="$(required_value assistant.eval.capture_fallback_min)"
# Spec 061 SCOPE-09a design §8.3.1 + §8.3.2 Step 1 — OTel SDK substrate
# SST keys. All three are REQUIRED at the generator boundary per
# smackerel-no-defaults. otel_endpoint is permissively-empty here
# because empty resolution is legal when otel_enabled=false; the Go
# validator enforces non-empty when otel_enabled=true.
ASSISTANT_OBSERVABILITY_OTEL_ENABLED="$(required_value assistant.observability.otel_enabled)"
ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT="$(yaml_get assistant.observability.otel_endpoint)"
ASSISTANT_OBSERVABILITY_OTEL_SERVICE_NAME="$(required_value assistant.observability.otel_service_name)"
# Spec 064 SCOPE-03 — open-ended knowledge agent SST. Every key is
# REQUIRED at the generator boundary (Gate G028, smackerel-no-defaults).
# When assistant.open_knowledge.enabled=false, several string values
# are legally empty (provider_endpoint, provider_api_key, llm_model_id)
# and tool_allowlist is legally []; the Go validator (Validate()) skips
# deep validation when Enabled=false. provider_api_key uses yaml_get
# because searxng allows empty.
ASSISTANT_OPEN_KNOWLEDGE_ENABLED="$(required_value assistant.open_knowledge.enabled)"
ASSISTANT_OPEN_KNOWLEDGE_PROVIDER="$(required_value assistant.open_knowledge.provider)"
ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT="$(yaml_get assistant.open_knowledge.provider_endpoint)"
ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY="$(yaml_get assistant.open_knowledge.provider_api_key)"
ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID="$(yaml_get "environments.$TARGET_ENV.assistant_open_knowledge_llm_model_id" 2>/dev/null || true)"
if [[ -z "$ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID" ]]; then
  ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID="$(yaml_get assistant.open_knowledge.llm_model_id)"
fi
ASSISTANT_OPEN_KNOWLEDGE_MAX_ITERATIONS="$(required_value assistant.open_knowledge.max_iterations)"
ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_TOKEN_BUDGET="$(required_value assistant.open_knowledge.per_query_token_budget)"
ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET="$(required_value assistant.open_knowledge.per_query_usd_budget)"
ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD="$(required_value assistant.open_knowledge.monthly_budget_usd)"
ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD="$(required_value assistant.open_knowledge.per_user_monthly_budget_usd)"
ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST="$(yaml_get_json assistant.open_knowledge.tool_allowlist)"
if [[ -z "$ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST" ]]; then
  ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST="[]"
fi
ASSISTANT_OPEN_KNOWLEDGE_WEB_SNIPPET_CACHE_ENABLED="$(required_value assistant.open_knowledge.web_snippet_cache_enabled)"
ASSISTANT_OPEN_KNOWLEDGE_LLM_TIMEOUT_MS="$(required_value assistant.open_knowledge.llm_timeout_ms)"
# SCOPE-15 — egress allowlist (extra hosts beyond provider_endpoint).
# Empty list emitted as "[]" so the Go loader's JSON decoder accepts it.
ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS="$(yaml_get_json assistant.open_knowledge.allowed_egress_hosts)"
if [[ -z "$ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS" ]]; then
  ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS="[]"
fi
# SCOPE-16 — circuit breaker bounds for the web-search provider.
# All three keys REQUIRED at the generator boundary even when
# enabled=false (the Go validator skips deep checks when disabled).
ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD="$(required_value assistant.open_knowledge.circuit_breaker.failure_threshold)"
ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_OPEN_WINDOW_SECONDS="$(required_value assistant.open_knowledge.circuit_breaker.open_window_seconds)"
ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_HALF_OPEN_AFTER_SECONDS="$(required_value assistant.open_knowledge.circuit_breaker.half_open_after_seconds)"
# Spec 076 SCOPE-1 — cite-back verifier enforcement gate (SCN-076-F02 foundation key).
ASSISTANT_OPEN_KNOWLEDGE_CITEBACK_ENFORCEMENT_MODE="$(required_value assistant.open_knowledge.citeback.enforcement_mode)"
# Spec 076 SCOPE-1 — assistant.annotation.classifier.* foundation SST keys (SCN-076-F02).
ASSISTANT_ANNOTATION_CLASSIFIER_CONFIDENCE_FLOOR="$(required_value assistant.annotation.classifier.confidence_floor)"
ASSISTANT_ANNOTATION_CLASSIFIER_WARM_CACHE_ENABLED="$(required_value assistant.annotation.classifier.warm_cache_enabled)"
# Spec 027 scope 9 PLAN-9-04 / PLAN-9-05 — annotation editing API SST keys.
# Fail-loud at the generator boundary; the Go handler enforces the same
# allowlist in lockstep (internal/api/annotation_source.go).
ANNOTATIONS_SOURCE_HEADER_NAME="$(required_value annotations.source_header_name)"
ANNOTATIONS_SOURCE_ALLOWLIST="$(required_value annotations.source_allowlist | tr -d '[]" ' )"
ANNOTATIONS_LIST_MY_MAX_LIMIT="$(required_value annotations.list_my_max_limit)"
# Spec 068 SCOPE-1 — Structured Intent Compiler SST keys. All keys
# REQUIRED at the generator boundary (Gate G028 / smackerel-no-defaults).
# The Go loader (internal/config/assistant_intent_compiler.go) fails
# loud at startup if any value is missing or unparsable.
ASSISTANT_INTENT_COMPILER_ENABLED="$(required_value assistant.intent_compiler.enabled)"
ASSISTANT_INTENT_COMPILER_MODEL_ROLE="$(required_value assistant.intent_compiler.model_role)"
ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION="$(required_value assistant.intent_compiler.prompt_contract_version)"
ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION="$(required_value assistant.intent_compiler.schema_version)"
ASSISTANT_INTENT_COMPILER_TIMEOUT_MS="$(required_value assistant.intent_compiler.timeout_ms)"
ASSISTANT_INTENT_COMPILER_CONFIDENCE_FLOOR="$(required_value assistant.intent_compiler.confidence_floor)"
ASSISTANT_INTENT_COMPILER_MAX_CONTEXT_TURNS="$(required_value assistant.intent_compiler.max_context_turns)"
ASSISTANT_INTENT_COMPILER_MAX_OUTPUT_BYTES="$(required_value assistant.intent_compiler.max_output_bytes)"
ASSISTANT_INTENT_COMPILER_RETRY_BUDGET="$(required_value assistant.intent_compiler.retry_budget)"

# Spec 074 SCOPE-1 — Capture-as-Fallback SST (NO defaults; required at generator boundary).
CAPTURE_AS_FALLBACK_DEDUP_WINDOW="$(required_value assistant.capture_as_fallback.dedup_window)"
CAPTURE_AS_FALLBACK_CLARIFY_ABANDON_TIMEOUT="$(required_value assistant.capture_as_fallback.clarify_abandon_timeout)"
CAPTURE_AS_FALLBACK_NORMALIZATION_POLICY="$(required_value assistant.capture_as_fallback.normalization_policy)"
CAPTURE_AS_FALLBACK_DEDUP_HASH_KEY="$(required_value assistant.capture_as_fallback.dedup_hash_key)"
CAPTURE_AS_FALLBACK_RETENTION_AUDIT_DAYS="$(required_value assistant.capture_as_fallback.retention_audit_days)"

# Spec 065 SCOPE-1 — Generic micro-tools SST (NO defaults; required at generator boundary).
ASSISTANT_TOOLS_LOCATION_NORMALIZE_ENABLED="$(required_value assistant.tools.location_normalize.enabled)"
ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER="$(required_value assistant.tools.location_normalize.provider)"
ASSISTANT_TOOLS_LOCATION_NORMALIZE_TIMEOUT_MS="$(required_value assistant.tools.location_normalize.timeout_ms)"
ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_TTL_SECONDS="$(required_value assistant.tools.location_normalize.cache_ttl_seconds)"
ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_MAX_ENTRIES="$(required_value assistant.tools.location_normalize.cache_max_entries)"
ASSISTANT_TOOLS_UNIT_CONVERT_ENABLED="$(required_value assistant.tools.unit_convert.enabled)"
ASSISTANT_TOOLS_UNIT_CONVERT_CATALOG_VERSION="$(required_value assistant.tools.unit_convert.catalog_version)"
ASSISTANT_TOOLS_CALCULATOR_ENABLED="$(required_value assistant.tools.calculator.enabled)"
ASSISTANT_TOOLS_CALCULATOR_MAX_EXPRESSION_CHARS="$(required_value assistant.tools.calculator.max_expression_chars)"
ASSISTANT_TOOLS_ENTITY_RESOLVE_ENABLED="$(required_value assistant.tools.entity_resolve.enabled)"
ASSISTANT_TOOLS_ENTITY_RESOLVE_CONFIDENCE_FLOOR="$(required_value assistant.tools.entity_resolve.confidence_floor)"
ASSISTANT_TOOLS_ENTITY_RESOLVE_TIMEOUT_MS="$(required_value assistant.tools.entity_resolve.timeout_ms)"

# Spec 075 SCOPE-1 — Legacy-Surface Deprecation Telemetry SST (NO defaults; required at generator boundary).
LEGACY_RETIREMENT_WINDOW_ID="$(required_value legacy_retirement.window_id)"
LEGACY_RETIREMENT_WINDOW_STATE="$(required_value legacy_retirement.window_state)"
LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS="$(required_value legacy_retirement.rollback_threshold_percent_active_users)"
LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAYS_CONSECUTIVE="$(required_value legacy_retirement.rollback_threshold_days_consecutive)"
LEGACY_RETIREMENT_POST_WINDOW_OBSERVATION_DAYS="$(required_value legacy_retirement.post_window_observation_days)"
LEGACY_RETIREMENT_ACTIVE_USER_WINDOW_DAYS="$(required_value legacy_retirement.active_user_window_days)"
LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY="$(required_value legacy_retirement.user_bucket_hmac_key)"
LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND="$(required_json_value legacy_retirement.notice_copy_per_command)"
LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY="$(required_json_value legacy_retirement.post_window_unknown_response_copy)"
# Spec 076 SCOPE-6a — runtime wiring SST keys.
LEGACY_RETIREMENT_THRESHOLD_EVALUATOR_INTERVAL_SECONDS="$(required_value legacy_retirement.threshold_evaluator_interval_seconds)"
LEGACY_RETIREMENT_OBSERVATION_CRON_EXPR="$(required_value legacy_retirement.observation_cron_expr)"
LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAILY_INVOCATIONS="$(required_value legacy_retirement.rollback_threshold_daily_invocations)"

# Per-target override for the test e2e suite: force webhook mode and
# inject a stable, known test secret so the BS-001 webhook e2e shell
# test can POST authenticated requests against the live test stack.
# Production/home-lab/dev targets retain whatever assistant.transports.telegram.mode
# resolves from the SST yaml (default long_poll until a public HTTPS
# deployment story exists per design §17.6).
ASSISTANT_TELEGRAM_WEBHOOK_SECRET=""
if [[ "$TARGET_ENV" == "test" ]]; then
  ASSISTANT_TRANSPORTS_TELEGRAM_MODE="webhook"
  ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF="ASSISTANT_TELEGRAM_WEBHOOK_SECRET"
  ASSISTANT_TELEGRAM_WEBHOOK_SECRET="test-webhook-secret-061-scope-05-bs001-fixture"
  # Spec 061 SCOPE-05 §17.5 — BS-001 fixture chat IDs need a user
  # mapping so the assistant adapter's translateInbound path resolves
  # an actor user_id and either (a) routes via assistant.Handle +
  # CaptureRoute or (b) falls through to handleTextCapture. Without
  # this mapping the adapter returns (handled=true, translate error)
  # and swallows the message before the capture path runs, so the
  # BS-001 e2e fails its PG artifact poll. The chat IDs match the
  # fixtures hard-coded in tests/e2e/test_telegram_assistant_bs001.sh
  # (CHAT_ID=99001) and the BS-002/BS-007/weather companion tests.
  # OVERRIDE (not append) — the test stack MUST NOT inherit any
  # dev/production mappings that would point capture artifacts at a
  # real user_id.
  TELEGRAM_USER_MAPPING="99001:test-user-061-bs001,99002:test-user-061-bs002,99007:test-user-061-bs007,99003:test-user-061-weather-bs003,99006:test-user-061-weather-bs006"
  # Spec 061 design §18.3+§18.4 — test stack flips weather provider URLs
  # to the in-tree nginx stub container `smackerel-test-stub-providers`
  # under the `test` compose profile. Production URLs are NEVER reached
  # from the test stack (hermetic per bubbles-test-environment-isolation).
  # The §18.3 production-safety guard in internal/config/assistant.go
  # rejects startup if any non-test env contains the substring
  # `stub-providers`.
  ASSISTANT_SKILLS_WEATHER_GEOCODE_URL="http://stub-providers:8080/v1/search"
  ASSISTANT_SKILLS_WEATHER_FORECAST_URL="http://stub-providers:8080/v1/forecast"
  # SCOPE-07 BS-003/BS-006 need the weather skill ENABLED in the test
  # stack so the router dispatches "weather in <city>" to the weather
  # scenario instead of falling through to capture. Production keeps
  # the smackerel.yaml literal (false until packet 060 grants weather).
  ASSISTANT_SKILLS_WEATHER_ENABLED="true"
fi

# HL-RESCAN-012 / Gate G028 — build-metadata SST resolution.
# Source-of-truth precedence at config-generate time:
#   1. Shell environment (CI exports SMACKEREL_VERSION / SMACKEREL_COMMIT
#      via .github/workflows/ci.yml; release pipeline exports
#      SMACKEREL_BUILD_TIME / SMACKEREL_CORE_IMAGE / SMACKEREL_ML_IMAGE
#      with digest pins from the build manifest).
#   2. Hard-coded ad-hoc-dev defaults below (dev / unknown / empty).
# Emitting these into the env file lets docker-compose.yml use the
# fail-loud ${X:?...} substitution form (no `:-default` fallbacks),
# so the policy is enforced at compose substitution time.
if [[ -z "${SMACKEREL_VERSION+set}" ]]; then
  SMACKEREL_VERSION="dev"
fi
if [[ -z "${SMACKEREL_COMMIT+set}" ]]; then
  SMACKEREL_COMMIT="unknown"
fi
if [[ -z "${SMACKEREL_BUILD_TIME+set}" ]]; then
  SMACKEREL_BUILD_TIME="unknown"
fi
if [[ -z "${SMACKEREL_CORE_IMAGE+set}" ]]; then
  SMACKEREL_CORE_IMAGE=""
fi
if [[ -z "${SMACKEREL_ML_IMAGE+set}" ]]; then
  SMACKEREL_ML_IMAGE=""
fi

mkdir -p "$REPO_ROOT/config/generated"

OUTPUT_FILE="$REPO_ROOT/config/generated/${TARGET_ENV}.env"

# Spec 045 BUG-045-001 Scope 2 / DD-2 — write to a TEMP env file first,
# then run the config-validate binary against the TEMP file BEFORE the
# atomic promote (mv) to the final $OUTPUT_FILE path. This gates the
# generator on the runtime Config.Validate() chain (including per-service
# model-envelope validation), so any envelope mismatch is rejected at
# `./smackerel.sh config generate` time instead of at smackerel-core
# startup. On rejection the .tmp file is removed and the existing
# $OUTPUT_FILE (if any) is left untouched, so an operator with a
# previously-valid env file can keep running while they fix the
# regression. NO-DEFAULTS / fail-loud per Gate G028.
OUTPUT_FILE_TMP="${OUTPUT_FILE}.tmp.$$"

# Spec 064 SCOPE-17 — Aggregate enabled Docker Compose profiles from
# the SST ENABLE_<PROFILE> flags into a single COMPOSE_PROFILES value.
# Docker Compose natively reads COMPOSE_PROFILES from --env-file, so
# any deploy adapter that invokes `docker compose --env-file app.env up`
# automatically includes these profiles without needing per-adapter
# --profile wiring. This is the durable, target-agnostic contract:
# SST owns which profiles are active; Compose honors it natively.
# To add a new profile-gated service, set ENABLE_<PROFILE>=true via
# SST and append it to the aggregation below.
_compose_profiles=()
if [[ "${OLLAMA_ENABLED:-false}" == "true" ]]; then _compose_profiles+=(ollama); fi
if [[ "${SEARXNG_ENABLED:-false}" == "true" ]]; then _compose_profiles+=(searxng); fi
COMPOSE_PROFILES="$(IFS=,; echo "${_compose_profiles[*]:-}")"

cat > "$OUTPUT_FILE_TMP" <<EOF
# Auto-generated from config/smackerel.yaml — DO NOT EDIT DIRECTLY
# Regenerate: ./smackerel.sh config generate
# Environment: ${TARGET_ENV}
# Generated: $(date -u +%Y-%m-%dT%H:%M:%S+00:00)
SMACKEREL_ENV_FILE=config/generated/${TARGET_ENV}.env
# HL-RESCAN-012 / Gate G028 — build metadata flowed through SST so
# docker-compose.yml can use the fail-loud \${X:?...} form (no \`:-default\`
# fallbacks). CI / release pipelines override these via shell env at
# config-generate time; ad-hoc dev runs see the safe placeholders below.
SMACKEREL_VERSION=${SMACKEREL_VERSION}
SMACKEREL_COMMIT=${SMACKEREL_COMMIT}
SMACKEREL_BUILD_TIME=${SMACKEREL_BUILD_TIME}
SMACKEREL_CORE_IMAGE=${SMACKEREL_CORE_IMAGE}
SMACKEREL_ML_IMAGE=${SMACKEREL_ML_IMAGE}
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
OLLAMA_KEEP_ALIVE=${OLLAMA_KEEP_ALIVE}
OLLAMA_TEST_PREWARM_WARMUP_NUM_PREDICT=${OLLAMA_TEST_PREWARM_WARMUP_NUM_PREDICT}
OLLAMA_TEST_PREWARM_SECOND_CALL_MAX_MS=${OLLAMA_TEST_PREWARM_SECOND_CALL_MAX_MS}
ENABLE_OLLAMA=${OLLAMA_ENABLED}
# Spec 064 SCOPE-07 — SearxNG SST emission. runtime.sh adds
# --profile searxng to docker compose iff ENABLE_SEARXNG is truthy.
ENABLE_SEARXNG=${SEARXNG_ENABLED}
# Spec 064 SCOPE-17 — Docker Compose native profile activation.
# Computed above from ENABLE_<PROFILE> flags. Compose reads this
# automatically from --env-file, so adapters need NO --profile flag.
COMPOSE_PROFILES=${COMPOSE_PROFILES}
SEARXNG_HOST_PORT=${SEARXNG_HOST_PORT}
SEARXNG_BIND_ADDRESS=${SEARXNG_BIND_ADDRESS}
SEARXNG_IMAGE=${SEARXNG_IMAGE}
SEARXNG_CONTAINER_PORT=${SEARXNG_CONTAINER_PORT}
SEARXNG_SECRET=${SEARXNG_SECRET}
SEARXNG_BASE_URL=${SEARXNG_BASE_URL}
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
EXTENSION_INGEST_ENABLED=${EXTENSION_INGEST_ENABLED}
EXTENSION_INGEST_MAX_BATCH_ITEMS=${EXTENSION_INGEST_MAX_BATCH_ITEMS}
EXTENSION_INGEST_MAX_BODY_BYTES=${EXTENSION_INGEST_MAX_BODY_BYTES}
EXTENSION_INGEST_DEFAULT_DEDUP_WINDOW_SECONDS=${EXTENSION_INGEST_DEFAULT_DEDUP_WINDOW_SECONDS}
EXTENSION_INGEST_ACCEPTED_CONTENT_TYPES=${EXTENSION_INGEST_ACCEPTED_CONTENT_TYPES}
EXTENSION_INGEST_REQUIRED_TOKEN_SCOPE=${EXTENSION_INGEST_REQUIRED_TOKEN_SCOPE}
KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT=${KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT}
KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT=${KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT}
KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS=${KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS}
KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT=${KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT}
KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT=${KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT}
KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV=${KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV}
KNOWLEDGE_GRAPH_API_CURSOR_SECRET=${KNOWLEDGE_GRAPH_API_CURSOR_SECRET}
SURFACING_DAILY_NUDGE_BUDGET=${SURFACING_DAILY_NUDGE_BUDGET}
SURFACING_SUPPRESSION_WINDOW_HOURS=${SURFACING_SUPPRESSION_WINDOW_HOURS}
SURFACING_DEDUPE_WINDOW_HOURS=${SURFACING_DEDUPE_WINDOW_HOURS}
SURFACING_URGENT_ESCALATION_ENABLED=${SURFACING_URGENT_ESCALATION_ENABLED}
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
POLICY_SCENARIO_PROMPT_MAX_LINES=${POLICY_SCENARIO_PROMPT_MAX_LINES}
POLICY_EXCEPTION_BASELINE_PATH=${POLICY_EXCEPTION_BASELINE_PATH}
POLICY_EXCEPTION_MAX_AGE_DAYS=${POLICY_EXCEPTION_MAX_AGE_DAYS}
POLICY_INTENT_BYPASS_GUARD_ENABLED=${POLICY_INTENT_BYPASS_GUARD_ENABLED}
RECOMMENDATIONS_DELIVERY_TELEGRAM_ENABLED=${RECOMMENDATIONS_DELIVERY_TELEGRAM_ENABLED}
RECOMMENDATIONS_DELIVERY_DIGEST_ENABLED=${RECOMMENDATIONS_DELIVERY_DIGEST_ENABLED}
RECOMMENDATIONS_DELIVERY_TRIP_DOSSIER_ENABLED=${RECOMMENDATIONS_DELIVERY_TRIP_DOSSIER_ENABLED}
NOTIFICATION_INTELLIGENCE_ENABLED=${NOTIFICATION_INTELLIGENCE_ENABLED}
NOTIFICATION_PERSISTENCE_THRESHOLD=${NOTIFICATION_PERSISTENCE_THRESHOLD}
NOTIFICATION_ESCALATION_SEVERITY=${NOTIFICATION_ESCALATION_SEVERITY}
NOTIFICATION_LOW_CONFIDENCE_THRESHOLD=${NOTIFICATION_LOW_CONFIDENCE_THRESHOLD}
NOTIFICATION_MAX_RETRIES=${NOTIFICATION_MAX_RETRIES}
NOTIFICATION_OUTPUT_CHANNELS=${NOTIFICATION_OUTPUT_CHANNELS}
NTFY_SOURCES_JSON=${NTFY_SOURCES_JSON}
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
KEEP_GOOGLE_EMAIL=${KEEP_GOOGLE_EMAIL}
KEEP_GOOGLE_APP_PASSWORD=${KEEP_GOOGLE_APP_PASSWORD}
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
FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS=${FINANCIAL_MARKETS_HTTP_TIMEOUT_SECONDS}
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
QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON=${QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON}
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
ML_EMBEDDING_WORKERS=${ML_EMBEDDING_WORKERS}
ML_EMBEDDING_QUEUE_MAX=${ML_EMBEDDING_QUEUE_MAX}
ML_HEALTH_LATENCY_SLA_MS=${ML_HEALTH_LATENCY_SLA_MS}
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
AUTH_OAUTH_HTTP_TIMEOUT_SECONDS=${AUTH_OAUTH_HTTP_TIMEOUT_SECONDS}
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
RUNTIME_TRUSTED_PROXIES=${RUNTIME_TRUSTED_PROXIES}
AGENT_SCENARIO_DIR=${AGENT_SCENARIO_DIR}
AGENT_SCENARIO_GLOB=${AGENT_SCENARIO_GLOB}
AGENT_HOT_RELOAD=${AGENT_HOT_RELOAD}
AGENT_ROUTING_CONFIDENCE_FLOOR=${AGENT_ROUTING_CONFIDENCE_FLOOR}
AGENT_ROUTING_CONSIDER_TOP_N=${AGENT_ROUTING_CONSIDER_TOP_N}
AGENT_ROUTING_FALLBACK_SCENARIO_ID=${AGENT_ROUTING_FALLBACK_SCENARIO_ID}
AGENT_ROUTING_EMBEDDING_MODEL=${AGENT_ROUTING_EMBEDDING_MODEL}
ASSISTANT_ROUTING_EMBEDDER_MODE=${ASSISTANT_ROUTING_EMBEDDER_MODE}
ASSISTANT_ROUTING_EMBED_TIMEOUT_MS=${ASSISTANT_ROUTING_EMBED_TIMEOUT_MS}
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
SMACKEREL_HARDWARE_TIER=${SMACKEREL_HARDWARE_TIER}
RETRIEVAL_QA_TIMEOUT_MS=${RETRIEVAL_QA_TIMEOUT_MS}
RETRIEVAL_QA_PER_TOOL_TIMEOUT_MS=${RETRIEVAL_QA_PER_TOOL_TIMEOUT_MS}
RECIPE_SEARCH_TIMEOUT_MS=${RECIPE_SEARCH_TIMEOUT_MS}
RECIPE_SEARCH_PER_TOOL_TIMEOUT_MS=${RECIPE_SEARCH_PER_TOOL_TIMEOUT_MS}
ASSISTANT_ENABLED=${ASSISTANT_ENABLED}
ASSISTANT_BORDERLINE_FLOOR=${ASSISTANT_BORDERLINE_FLOOR}
ASSISTANT_CONTEXT_WINDOW_TURNS=${ASSISTANT_CONTEXT_WINDOW_TURNS}
ASSISTANT_CONTEXT_IDLE_TIMEOUT=${ASSISTANT_CONTEXT_IDLE_TIMEOUT}
ASSISTANT_CONTEXT_IDLE_SWEEP_INTERVAL=${ASSISTANT_CONTEXT_IDLE_SWEEP_INTERVAL}
ASSISTANT_CONTEXT_STATE_KEY=${ASSISTANT_CONTEXT_STATE_KEY}
ASSISTANT_SOURCES_MAX=${ASSISTANT_SOURCES_MAX}
ASSISTANT_BODY_MAX_CHARS=${ASSISTANT_BODY_MAX_CHARS}
ASSISTANT_STATUS_MAX_DURATION=${ASSISTANT_STATUS_MAX_DURATION}
ASSISTANT_DISAMBIGUATE_TIMEOUT=${ASSISTANT_DISAMBIGUATE_TIMEOUT}
ASSISTANT_ERROR_CAPTURE_TIMEOUT=${ASSISTANT_ERROR_CAPTURE_TIMEOUT}
ASSISTANT_RATE_LIMIT_RETRIEVAL_RPM=${ASSISTANT_RATE_LIMIT_RETRIEVAL_RPM}
ASSISTANT_RATE_LIMIT_WEATHER_RPM=${ASSISTANT_RATE_LIMIT_WEATHER_RPM}
ASSISTANT_RATE_LIMIT_NOTIFICATIONS_RPM=${ASSISTANT_RATE_LIMIT_NOTIFICATIONS_RPM}
ASSISTANT_SKILLS_RETRIEVAL_ENABLED=${ASSISTANT_SKILLS_RETRIEVAL_ENABLED}
ASSISTANT_SKILLS_RETRIEVAL_TOP_K=${ASSISTANT_SKILLS_RETRIEVAL_TOP_K}
ASSISTANT_RATE_LIMIT_RECIPE_SEARCH_RPM=${ASSISTANT_RATE_LIMIT_RECIPE_SEARCH_RPM}
ASSISTANT_SKILLS_RECIPE_SEARCH_ENABLED=${ASSISTANT_SKILLS_RECIPE_SEARCH_ENABLED}
ASSISTANT_SKILLS_RECIPE_SEARCH_TOP_K=${ASSISTANT_SKILLS_RECIPE_SEARCH_TOP_K}
ASSISTANT_SKILLS_WEATHER_ENABLED=${ASSISTANT_SKILLS_WEATHER_ENABLED}
ASSISTANT_SKILLS_WEATHER_PROVIDER=${ASSISTANT_SKILLS_WEATHER_PROVIDER}
ASSISTANT_SKILLS_WEATHER_API_KEY_REF=${ASSISTANT_SKILLS_WEATHER_API_KEY_REF}
ASSISTANT_SKILLS_WEATHER_CACHE_TTL=${ASSISTANT_SKILLS_WEATHER_CACHE_TTL}
ASSISTANT_SKILLS_WEATHER_GEOCODE_URL=${ASSISTANT_SKILLS_WEATHER_GEOCODE_URL}
ASSISTANT_SKILLS_WEATHER_FORECAST_URL=${ASSISTANT_SKILLS_WEATHER_FORECAST_URL}
ASSISTANT_SKILLS_NOTIFICATIONS_ENABLED=${ASSISTANT_SKILLS_NOTIFICATIONS_ENABLED}
ASSISTANT_SKILLS_NOTIFICATIONS_CONFIRM_TIMEOUT=${ASSISTANT_SKILLS_NOTIFICATIONS_CONFIRM_TIMEOUT}
ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED=${ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED}
ASSISTANT_TRANSPORTS_TELEGRAM_MARKDOWN_MODE=${ASSISTANT_TRANSPORTS_TELEGRAM_MARKDOWN_MODE}
ASSISTANT_TRANSPORTS_TELEGRAM_MAX_MESSAGE_CHARS=${ASSISTANT_TRANSPORTS_TELEGRAM_MAX_MESSAGE_CHARS}
ASSISTANT_TRANSPORTS_TELEGRAM_MODE=${ASSISTANT_TRANSPORTS_TELEGRAM_MODE}
ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF=${ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF}
ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH=${ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH}
ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED=${ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED}
ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_PATH=${ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_PATH}
ASSISTANT_TRANSPORTS_WHATSAPP_PHONE_NUMBER_ID=${ASSISTANT_TRANSPORTS_WHATSAPP_PHONE_NUMBER_ID}
ASSISTANT_TRANSPORTS_WHATSAPP_BUSINESS_ACCOUNT_ID=${ASSISTANT_TRANSPORTS_WHATSAPP_BUSINESS_ACCOUNT_ID}
ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_VERIFY_TOKEN_REF=${ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_VERIFY_TOKEN_REF}
ASSISTANT_TRANSPORTS_WHATSAPP_APP_SECRET_REF=${ASSISTANT_TRANSPORTS_WHATSAPP_APP_SECRET_REF}
ASSISTANT_TRANSPORTS_WHATSAPP_ACCESS_TOKEN_REF=${ASSISTANT_TRANSPORTS_WHATSAPP_ACCESS_TOKEN_REF}
ASSISTANT_TRANSPORTS_WHATSAPP_IDENTITY_HASH_KEY_REF=${ASSISTANT_TRANSPORTS_WHATSAPP_IDENTITY_HASH_KEY_REF}
ASSISTANT_TRANSPORTS_WHATSAPP_API_BASE_URL=${ASSISTANT_TRANSPORTS_WHATSAPP_API_BASE_URL}
ASSISTANT_TRANSPORTS_WHATSAPP_API_VERSION=${ASSISTANT_TRANSPORTS_WHATSAPP_API_VERSION}
ASSISTANT_TRANSPORTS_WHATSAPP_RATE_LIMIT_PER_USER_PER_MINUTE=${ASSISTANT_TRANSPORTS_WHATSAPP_RATE_LIMIT_PER_USER_PER_MINUTE}
ASSISTANT_TRANSPORTS_WHATSAPP_MAX_TEXT_CHARS=${ASSISTANT_TRANSPORTS_WHATSAPP_MAX_TEXT_CHARS}
ASSISTANT_TRANSPORTS_HTTP_ENABLED=${ASSISTANT_TRANSPORTS_HTTP_ENABLED}
ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION=${ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION}
ASSISTANT_TRANSPORTS_HTTP_BODY_SIZE_MAX_BYTES=${ASSISTANT_TRANSPORTS_HTTP_BODY_SIZE_MAX_BYTES}
ASSISTANT_TRANSPORTS_HTTP_RATE_LIMIT_PER_USER_PER_MINUTE=${ASSISTANT_TRANSPORTS_HTTP_RATE_LIMIT_PER_USER_PER_MINUTE}
ASSISTANT_TRANSPORTS_HTTP_CONVERSATION_TTL_SECONDS=${ASSISTANT_TRANSPORTS_HTTP_CONVERSATION_TTL_SECONDS}
ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE=${ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE}
ASSISTANT_TRANSPORTS_HTTP_SHARED_USER_ID=${ASSISTANT_TRANSPORTS_HTTP_SHARED_USER_ID}
ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS=${ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS}
ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST=${ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST}
ASSISTANT_TELEGRAM_WEBHOOK_SECRET=${ASSISTANT_TELEGRAM_WEBHOOK_SECRET}
ASSISTANT_EVAL_ROUTING_ACCURACY_MIN=${ASSISTANT_EVAL_ROUTING_ACCURACY_MIN}
ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN=${ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN}
ASSISTANT_OBSERVABILITY_OTEL_ENABLED=${ASSISTANT_OBSERVABILITY_OTEL_ENABLED}
ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT=${ASSISTANT_OBSERVABILITY_OTEL_ENDPOINT}
ASSISTANT_OBSERVABILITY_OTEL_SERVICE_NAME=${ASSISTANT_OBSERVABILITY_OTEL_SERVICE_NAME}
ASSISTANT_OPEN_KNOWLEDGE_ENABLED=${ASSISTANT_OPEN_KNOWLEDGE_ENABLED}
ASSISTANT_OPEN_KNOWLEDGE_PROVIDER=${ASSISTANT_OPEN_KNOWLEDGE_PROVIDER}
ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT=${ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT}
ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY=${ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY}
ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID=${ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID}
ASSISTANT_OPEN_KNOWLEDGE_MAX_ITERATIONS=${ASSISTANT_OPEN_KNOWLEDGE_MAX_ITERATIONS}
ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_TOKEN_BUDGET=${ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_TOKEN_BUDGET}
ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET=${ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET}
ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD=${ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD}
ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD=${ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD}
ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST=${ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST}
ASSISTANT_OPEN_KNOWLEDGE_WEB_SNIPPET_CACHE_ENABLED=${ASSISTANT_OPEN_KNOWLEDGE_WEB_SNIPPET_CACHE_ENABLED}
ASSISTANT_OPEN_KNOWLEDGE_LLM_TIMEOUT_MS=${ASSISTANT_OPEN_KNOWLEDGE_LLM_TIMEOUT_MS}
ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS=${ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS}
ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD=${ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD}
ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_OPEN_WINDOW_SECONDS=${ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_OPEN_WINDOW_SECONDS}
ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_HALF_OPEN_AFTER_SECONDS=${ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_HALF_OPEN_AFTER_SECONDS}
ASSISTANT_OPEN_KNOWLEDGE_CITEBACK_ENFORCEMENT_MODE=${ASSISTANT_OPEN_KNOWLEDGE_CITEBACK_ENFORCEMENT_MODE}
ASSISTANT_ANNOTATION_CLASSIFIER_CONFIDENCE_FLOOR=${ASSISTANT_ANNOTATION_CLASSIFIER_CONFIDENCE_FLOOR}
ASSISTANT_ANNOTATION_CLASSIFIER_WARM_CACHE_ENABLED=${ASSISTANT_ANNOTATION_CLASSIFIER_WARM_CACHE_ENABLED}
ANNOTATIONS_SOURCE_HEADER_NAME=${ANNOTATIONS_SOURCE_HEADER_NAME}
ANNOTATIONS_SOURCE_ALLOWLIST=${ANNOTATIONS_SOURCE_ALLOWLIST}
ANNOTATIONS_LIST_MY_MAX_LIMIT=${ANNOTATIONS_LIST_MY_MAX_LIMIT}
ASSISTANT_INTENT_COMPILER_ENABLED=${ASSISTANT_INTENT_COMPILER_ENABLED}
ASSISTANT_INTENT_COMPILER_MODEL_ROLE=${ASSISTANT_INTENT_COMPILER_MODEL_ROLE}
ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION=${ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION}
ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION=${ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION}
ASSISTANT_INTENT_COMPILER_TIMEOUT_MS=${ASSISTANT_INTENT_COMPILER_TIMEOUT_MS}
ASSISTANT_INTENT_COMPILER_CONFIDENCE_FLOOR=${ASSISTANT_INTENT_COMPILER_CONFIDENCE_FLOOR}
ASSISTANT_INTENT_COMPILER_MAX_CONTEXT_TURNS=${ASSISTANT_INTENT_COMPILER_MAX_CONTEXT_TURNS}
ASSISTANT_INTENT_COMPILER_MAX_OUTPUT_BYTES=${ASSISTANT_INTENT_COMPILER_MAX_OUTPUT_BYTES}
ASSISTANT_INTENT_COMPILER_RETRY_BUDGET=${ASSISTANT_INTENT_COMPILER_RETRY_BUDGET}
CAPTURE_AS_FALLBACK_DEDUP_WINDOW=${CAPTURE_AS_FALLBACK_DEDUP_WINDOW}
CAPTURE_AS_FALLBACK_CLARIFY_ABANDON_TIMEOUT=${CAPTURE_AS_FALLBACK_CLARIFY_ABANDON_TIMEOUT}
CAPTURE_AS_FALLBACK_NORMALIZATION_POLICY=${CAPTURE_AS_FALLBACK_NORMALIZATION_POLICY}
CAPTURE_AS_FALLBACK_DEDUP_HASH_KEY=${CAPTURE_AS_FALLBACK_DEDUP_HASH_KEY}
CAPTURE_AS_FALLBACK_RETENTION_AUDIT_DAYS=${CAPTURE_AS_FALLBACK_RETENTION_AUDIT_DAYS}
ASSISTANT_TOOLS_LOCATION_NORMALIZE_ENABLED=${ASSISTANT_TOOLS_LOCATION_NORMALIZE_ENABLED}
ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER=${ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER}
ASSISTANT_TOOLS_LOCATION_NORMALIZE_TIMEOUT_MS=${ASSISTANT_TOOLS_LOCATION_NORMALIZE_TIMEOUT_MS}
ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_TTL_SECONDS=${ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_TTL_SECONDS}
ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_MAX_ENTRIES=${ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_MAX_ENTRIES}
ASSISTANT_TOOLS_UNIT_CONVERT_ENABLED=${ASSISTANT_TOOLS_UNIT_CONVERT_ENABLED}
ASSISTANT_TOOLS_UNIT_CONVERT_CATALOG_VERSION=${ASSISTANT_TOOLS_UNIT_CONVERT_CATALOG_VERSION}
ASSISTANT_TOOLS_CALCULATOR_ENABLED=${ASSISTANT_TOOLS_CALCULATOR_ENABLED}
ASSISTANT_TOOLS_CALCULATOR_MAX_EXPRESSION_CHARS=${ASSISTANT_TOOLS_CALCULATOR_MAX_EXPRESSION_CHARS}
ASSISTANT_TOOLS_ENTITY_RESOLVE_ENABLED=${ASSISTANT_TOOLS_ENTITY_RESOLVE_ENABLED}
ASSISTANT_TOOLS_ENTITY_RESOLVE_CONFIDENCE_FLOOR=${ASSISTANT_TOOLS_ENTITY_RESOLVE_CONFIDENCE_FLOOR}
ASSISTANT_TOOLS_ENTITY_RESOLVE_TIMEOUT_MS=${ASSISTANT_TOOLS_ENTITY_RESOLVE_TIMEOUT_MS}
LEGACY_RETIREMENT_WINDOW_ID=${LEGACY_RETIREMENT_WINDOW_ID}
LEGACY_RETIREMENT_WINDOW_STATE=${LEGACY_RETIREMENT_WINDOW_STATE}
LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS=${LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_PERCENT_ACTIVE_USERS}
LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAYS_CONSECUTIVE=${LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAYS_CONSECUTIVE}
LEGACY_RETIREMENT_POST_WINDOW_OBSERVATION_DAYS=${LEGACY_RETIREMENT_POST_WINDOW_OBSERVATION_DAYS}
LEGACY_RETIREMENT_ACTIVE_USER_WINDOW_DAYS=${LEGACY_RETIREMENT_ACTIVE_USER_WINDOW_DAYS}
LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY=${LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY}
LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND=${LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND}
LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY=${LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY}
LEGACY_RETIREMENT_THRESHOLD_EVALUATOR_INTERVAL_SECONDS=${LEGACY_RETIREMENT_THRESHOLD_EVALUATOR_INTERVAL_SECONDS}
LEGACY_RETIREMENT_OBSERVATION_CRON_EXPR=${LEGACY_RETIREMENT_OBSERVATION_CRON_EXPR}
LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAILY_INVOCATIONS=${LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAILY_INVOCATIONS}
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
PROMETHEUS_IMAGE=${PROMETHEUS_IMAGE}
PROMETHEUS_CONTAINER_PORT=${PROMETHEUS_CONTAINER_PORT}
PROMETHEUS_HOST_PORT=${PROMETHEUS_HOST_PORT}
PROMETHEUS_VOLUME_NAME=${PROMETHEUS_VOLUME_NAME}
PROMETHEUS_SCRAPE_INTERVAL_S=${PROMETHEUS_SCRAPE_INTERVAL_S}
PROMETHEUS_EVALUATION_INTERVAL_S=${PROMETHEUS_EVALUATION_INTERVAL_S}
PROMETHEUS_RETENTION_DAYS=${PROMETHEUS_RETENTION_DAYS}
PROMETHEUS_CPU_LIMIT=${PROMETHEUS_CPU_LIMIT}
PROMETHEUS_MEMORY_LIMIT=${PROMETHEUS_MEMORY_LIMIT}
BACKUP_LOCAL_DIR=${BACKUP_LOCAL_DIR}
BACKUP_STATUS_FILE=${BACKUP_STATUS_FILE}
BACKUP_RETENTION_DAILY=${BACKUP_RETENTION_DAILY}
BACKUP_RETENTION_WEEKLY=${BACKUP_RETENTION_WEEKLY}
BACKUP_WATCHER_POLL_SECONDS=${BACKUP_WATCHER_POLL_SECONDS}
EOF

chmod 0600 "$OUTPUT_FILE_TMP"

# Spec 045 BUG-045-001 Scope 2 / DD-2 — pre-emit validation gate.
# Invoke the cmd/config-validate binary against the TEMP env file. If
# the runtime Validate() chain rejects the env file, remove the .tmp
# and exit non-zero with a fail-loud message. The existing $OUTPUT_FILE
# (if any) is left untouched. Stderr of the binary is propagated to the
# operator so the violating envelope/model/profile is named explicitly.
#
# Skip rationale for production-class (placeholder-mode) targets: when
# TARGET_ENV is a production-class target (e.g. home-lab), shell-managed
# secrets like POSTGRES_PASSWORD and runtime.auth_token are intentionally
# empty or emitted as __SECRET_PLACEHOLDER__ markers (spec 052 FR-052-002
# bundle contract; downstream secret injection fills real values at apply
# time). Running internal/config.Validate() against placeholder-mode
# output would fail-loud on "SMACKEREL_AUTH_TOKEN must be set when
# SMACKEREL_ENV=production" — a false positive because the operator's
# deploy adapter is the legitimate filler. The runtime envelope check
# inside smackerel-core's startup still enforces the model envelope at
# container start, so home-lab operators STILL get fail-loud on broken
# model choices; the difference is that the failure surfaces at apply
# time instead of generate time. Dev/test targets DO get pre-emit
# enforcement (they have literal values, not placeholders).
if is_production_class_target "$TARGET_ENV"; then
  echo "config-validate: skipped for production-class target env=$TARGET_ENV (placeholder mode; runtime check enforces at container start)" >&2
else
  # Honor SMACKEREL_CONFIG_VALIDATE_BIN when set (CI/test harnesses pre-build
  # the binary once and pass its path here to avoid sandbox builds without
  # the cmd/ source tree). Falls back to `go run` for the normal repo flow
  # where the cmd/ source is present alongside go.mod.
  if [[ -n "${SMACKEREL_CONFIG_VALIDATE_BIN:-}" ]]; then
    CONFIG_VALIDATE_CMD=("$SMACKEREL_CONFIG_VALIDATE_BIN")
  else
    CONFIG_VALIDATE_CMD=("go" "run" "$REPO_ROOT/cmd/config-validate")
  fi
  if ! "${CONFIG_VALIDATE_CMD[@]}" --env-file="$OUTPUT_FILE_TMP" 1>&2; then
    rm -f "$OUTPUT_FILE_TMP"
    echo "ERROR: config-generate-time validation failed for env=$TARGET_ENV (see above)" >&2
    exit 1
  fi
fi

mv -f "$OUTPUT_FILE_TMP" "$OUTPUT_FILE"
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
# Spec 049 — Monitoring stack (FR-049-001).
#
# Render config/prometheus/prometheus.yml.tmpl into
# config/generated/prometheus.yml via envsubst. Substituted variables
# (PROMETHEUS_SCRAPE_INTERVAL_S, PROMETHEUS_EVALUATION_INTERVAL_S,
# CORE_CONTAINER_PORT, ML_CONTAINER_PORT) come from the SST resolution
# above. The contract test
# internal/deploy/monitoring_scrape_contract_test.go parses the
# template directly so a regression that drops a job is caught at
# build time even before config-generate runs.
# ─────────────────────────────────────────────────────────────────────
PROM_TMPL_FILE="$REPO_ROOT/config/prometheus/prometheus.yml.tmpl"
PROM_OUT_FILE="$REPO_ROOT/config/generated/prometheus.yml"

[[ -f "$PROM_TMPL_FILE" ]] || {
  echo "ERROR: Prometheus template not found: $PROM_TMPL_FILE" >&2
  exit 1
}

# Use envsubst to substitute ONLY the named variables. This avoids
# accidental expansion of '$' characters in the template that happen
# to look like env vars but aren't.
PROM_SUBST_VARS='${PROMETHEUS_SCRAPE_INTERVAL_S} ${PROMETHEUS_EVALUATION_INTERVAL_S} ${CORE_CONTAINER_PORT} ${ML_CONTAINER_PORT}'
PROMETHEUS_SCRAPE_INTERVAL_S="$PROMETHEUS_SCRAPE_INTERVAL_S" \
  PROMETHEUS_EVALUATION_INTERVAL_S="$PROMETHEUS_EVALUATION_INTERVAL_S" \
  CORE_CONTAINER_PORT="$CORE_CONTAINER_PORT" \
  ML_CONTAINER_PORT="$ML_CONTAINER_PORT" \
  envsubst "$PROM_SUBST_VARS" < "$PROM_TMPL_FILE" > "$PROM_OUT_FILE"

chmod 0644 "$PROM_OUT_FILE"

echo "Generated $PROM_OUT_FILE"

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
  #   ./prometheus.yml             — rendered Prometheus scrape config (spec 049)
  #   ./alerts.yml                 — Prometheus alert rules (spec 049)
  #   ./prompt_contracts/*.yaml    — prompt YAMLs mounted into core + ml
  #   ./assistant/scenarios.yaml   — spec 061 sibling skills manifest mounted into core
  #   ./bundle-manifest.yaml       — manifest of files in this bundle
  #
  # Determinism: same (sourceSha, env, smackerel.yaml content,
  # deploy/compose.deploy.yml, config/prompt_contracts/, config/assistant/,
  # config/nats_contract.json, config/prometheus/) MUST produce the same
  # bundle bytes (and therefore the same sha256). Volatile content (the
  # `Generated:` timestamp comment in the env file) is stripped from the
  # bundle copy.
  COMPOSE_TEMPLATE="$REPO_ROOT/deploy/compose.deploy.yml"
  NATS_CONTRACT_FILE="$REPO_ROOT/config/nats_contract.json"
  PROMPT_CONTRACTS_DIR="$REPO_ROOT/config/prompt_contracts"
  ASSISTANT_MANIFEST_DIR="$REPO_ROOT/config/assistant"
  PROMETHEUS_ALERTS_FILE="$REPO_ROOT/config/prometheus/alerts.yml"
  SEARXNG_SETTINGS_FILE="$REPO_ROOT/config/searxng/settings.yml"

  [[ -f "$COMPOSE_TEMPLATE" ]] || { echo "ERROR: deploy compose template not found: $COMPOSE_TEMPLATE" >&2; exit 1; }
  [[ -f "$NATS_CONTRACT_FILE" ]] || { echo "ERROR: nats contract not found: $NATS_CONTRACT_FILE" >&2; exit 1; }
  [[ -d "$PROMPT_CONTRACTS_DIR" ]] || { echo "ERROR: prompt contracts dir not found: $PROMPT_CONTRACTS_DIR" >&2; exit 1; }
  [[ -d "$ASSISTANT_MANIFEST_DIR" ]] || { echo "ERROR: assistant manifest dir not found: $ASSISTANT_MANIFEST_DIR" >&2; exit 1; }
  [[ -f "$ASSISTANT_MANIFEST_DIR/scenarios.yaml" ]] || { echo "ERROR: assistant scenarios manifest not found: $ASSISTANT_MANIFEST_DIR/scenarios.yaml" >&2; exit 1; }
  [[ -f "$PROM_OUT_FILE" ]] || { echo "ERROR: rendered prometheus.yml not found: $PROM_OUT_FILE" >&2; exit 1; }
  [[ -f "$PROMETHEUS_ALERTS_FILE" ]] || { echo "ERROR: prometheus alerts file not found: $PROMETHEUS_ALERTS_FILE" >&2; exit 1; }
  [[ -f "$SEARXNG_SETTINGS_FILE" ]] || { echo "ERROR: searxng settings file not found: $SEARXNG_SETTINGS_FILE" >&2; exit 1; }

  # Strip the volatile `Generated:` line so the bundle is reproducible.
  # Renamed to app.env so the deploy compose can reference it generically.
  grep -v '^# Generated: ' "$OUTPUT_FILE" > "$STAGE_DIR/app.env"
  cp "$NATS_CONF_FILE" "$STAGE_DIR/nats.conf"
  cp "$COMPOSE_TEMPLATE" "$STAGE_DIR/docker-compose.yml"
  cp "$NATS_CONTRACT_FILE" "$STAGE_DIR/nats_contract.json"
  cp "$PROM_OUT_FILE" "$STAGE_DIR/prometheus.yml"
  cp "$PROMETHEUS_ALERTS_FILE" "$STAGE_DIR/alerts.yml"
  mkdir -p "$STAGE_DIR/prompt_contracts"
  cp "$PROMPT_CONTRACTS_DIR"/*.yaml "$STAGE_DIR/prompt_contracts/"
  mkdir -p "$STAGE_DIR/assistant"
  cp "$ASSISTANT_MANIFEST_DIR"/*.yaml "$STAGE_DIR/assistant/"
  # Spec 064 SCOPE-17 — searxng settings.yml mounted by deploy/compose.deploy.yml
  # at ./config/searxng/settings.yml. Without this file in the bundle, the
  # docker bind mount source is missing and Docker creates a DIRECTORY at the
  # target path, breaking the searxng container's settings.yml load.
  mkdir -p "$STAGE_DIR/config/searxng"
  cp "$SEARXNG_SETTINGS_FILE" "$STAGE_DIR/config/searxng/settings.yml"
  chmod 0644 "$STAGE_DIR/app.env" "$STAGE_DIR/nats.conf" \
    "$STAGE_DIR/docker-compose.yml" "$STAGE_DIR/nats_contract.json" \
    "$STAGE_DIR/prometheus.yml" "$STAGE_DIR/alerts.yml" \
    "$STAGE_DIR/config/searxng/settings.yml" \
    "$STAGE_DIR/prompt_contracts"/*.yaml \
    "$STAGE_DIR/assistant"/*.yaml

  # Spec 052 FR-052-003 / FR-052-006 — sibling secret-keys manifest.
  # Enumerates the canonical list of env-var keys whose values were emitted
  # as __SECRET_PLACEHOLDER__<KEY>__ markers (for production-class targets)
  # OR will be substituted by the deploy adapter at apply time. The knb
  # adapter parses this file post-extraction and validates that every key
  # has been substituted before container start; the Go runtime rejects any
  # placeholder marker that leaks through. Determinism: file content is
  # purely key-derived (no timestamp, source-sha, or environment data).
  {
    echo "# Spec 052 FR-052-003 — keys substituted at apply time."
    echo "# Mirrors:"
    echo "#   shell:  scripts/commands/config.sh::SHELL_SECRET_KEYS"
    echo "#   yaml:   config/smackerel.yaml::infrastructure.secret_keys"
    echo "#   Go:     internal/config/secret_keys.go::SecretKeys()"
    echo "secretKeys:"
    for key in "${SHELL_SECRET_KEYS[@]}"; do
      echo "  - $key"
    done
  } > "$STAGE_DIR/secret-keys.yaml"
  chmod 0644 "$STAGE_DIR/secret-keys.yaml"

  # Deterministic file list for bundle-manifest.yaml (sorted).
  PROMPT_FILES_LIST="$(cd "$STAGE_DIR" && find prompt_contracts -maxdepth 1 -type f -name '*.yaml' | LC_ALL=C sort)"
  ASSISTANT_FILES_LIST="$(cd "$STAGE_DIR" && find assistant -maxdepth 1 -type f -name '*.yaml' | LC_ALL=C sort)"

  {
    echo "bundleVersion: 1"
    echo "sourceSha: ${BUNDLE_SOURCE_SHA}"
    echo "environment: ${TARGET_ENV}"
    echo "files:"
    echo "  - alerts.yml"
    echo "  - app.env"
    echo "  - config/searxng/settings.yml"
    echo "  - nats.conf"
    echo "  - docker-compose.yml"
    echo "  - nats_contract.json"
    echo "  - prometheus.yml"
    echo "  - secret-keys.yaml"
    while IFS= read -r f; do
      echo "  - $f"
    done <<< "$ASSISTANT_FILES_LIST"
    while IFS= read -r f; do
      echo "  - $f"
    done <<< "$PROMPT_FILES_LIST"
  } > "$STAGE_DIR/bundle-manifest.yaml"
  chmod 0644 "$STAGE_DIR/bundle-manifest.yaml"

  BUNDLE_FILE="$BUNDLE_OUTPUT_DIR/config-bundle-${TARGET_ENV}-${BUNDLE_SOURCE_SHA}.tar.gz"

  # Build deterministic file list. tar's --sort=name will recurse into the
  # prompt_contracts and assistant directories automatically, so we only list
  # each once. Top-level files are listed in LC_ALL=C name order to make the
  # argv deterministic too.
  TAR_FILES=(
    "alerts.yml"
    "app.env"
    "assistant"
    "bundle-manifest.yaml"
    "config"
    "docker-compose.yml"
    "nats.conf"
    "nats_contract.json"
    "prometheus.yml"
    "prompt_contracts"
    "secret-keys.yaml"
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