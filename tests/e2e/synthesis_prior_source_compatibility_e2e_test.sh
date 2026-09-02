#!/usr/bin/env bash
# BUG-004-004 corrective SCOPE-03A — T004-C21-PRIOR-SOURCE.
#
# Proves local source compatibility across migration 067 without selecting a
# deployment pointer. The exact prior source and the dirty-or-clean candidate
# are built through their own repository CLIs. Both run only by immutable local
# image ID on one internal Docker network. The prior binary may either persist
# each cadence through the additive schema or fail closed, but it may never
# acquire invented causal history. The candidate must retain those rows and
# append a fully linked causal write.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"
PINNED_SOURCE_SHA="7c3838e3b2de9ecba2e6a7764493a0412c4ed268"
CURRENT_REVISION_LABEL=""

RUN_PREFIX="smk-c21-$(date -u +%Y%m%d%H%M%S)-$$-$RANDOM"
CHILD_RUN_ID=""
TEMP_ROOT="$(mktemp -d)"
PRIOR_WORKTREE="$TEMP_ROOT/prior-source"
PRIOR_WORKTREE_ADDED=0
HTTP_PROBE_HELPER="$SCRIPT_DIR/lib/synthesis_http_probe.py"
AUTH_TOKEN_FILE="$TEMP_ROOT/synthesis-auth-token"

NETWORK_NAME="${RUN_PREFIX}-network"
POSTGRES_VOLUME="${RUN_PREFIX}-postgres-data"
NATS_VOLUME="${RUN_PREFIX}-nats-data"
POSTGRES_CONTAINER="${RUN_PREFIX}-postgres"
NATS_CONTAINER="${RUN_PREFIX}-nats"
MIGRATION_CORE_CONTAINER="${RUN_PREFIX}-migration-core"
PRIOR_CORE_CONTAINER="${RUN_PREFIX}-prior-core"
CURRENT_CORE_CONTAINER="${RUN_PREFIX}-current-core"
ACTIVE_CORE_CONTAINER=""
ACTIVE_ENV_FILE=""
ACTIVE_ML_IMAGE_ID=""
CORE_URL=""
PROBE_SEQUENCE=0

PRIOR_CORE_IMAGE_REF="${RUN_PREFIX}-prior-core:local"
PRIOR_ML_IMAGE_REF="${RUN_PREFIX}-prior-ml:local"
CURRENT_CORE_IMAGE_REF="${RUN_PREFIX}-current-core:local"
CURRENT_ML_IMAGE_REF="${RUN_PREFIX}-current-ml:local"
SAVED_FIXED_CORE_REF="${RUN_PREFIX}-saved-core:local"
SAVED_FIXED_ML_REF="${RUN_PREFIX}-saved-ml:local"
FIXED_BUILD_TAG=""
FIXED_ML_BUILD_TAG=""
FIXED_BUILD_TAG_EXISTED=0
FIXED_ML_BUILD_TAG_EXISTED=0
PREEXISTING_FIXED_BUILD_IMAGE_ID=""
PREEXISTING_FIXED_ML_IMAGE_ID=""
PREEXISTING_IMAGE_IDS=""

PRIOR_CORE_IMAGE_ID=""
PRIOR_ML_IMAGE_ID=""
CURRENT_CORE_IMAGE_ID=""
CURRENT_ML_IMAGE_ID=""
PRIOR_ENV_FILE=""
CURRENT_ENV_FILE=""

POSTGRES_IMAGE=""
NATS_IMAGE=""
POSTGRES_USER=""
POSTGRES_PASSWORD=""
POSTGRES_DB=""
POSTGRES_CONTAINER_PORT=""
CORE_CONTAINER_PORT=""
AUTH_TOKEN=""

PRIOR_DAILY_OUTPUT_ID=""
PRIOR_WEEKLY_OUTPUT_ID=""
PRIOR_RUN_COUNT=0
PRIOR_OUTPUT_COUNT=0
PRIOR_ATTEMPT_COUNT=0
PRIOR_LEGACY_ATTEMPT_COUNT=0
PRIOR_EVENT_COUNT=0

TEST_PASS_READY=0
HTTP_STATUS=""
HTTP_BODY=""

fail() {
  echo "FAIL: T004-C21 $*" >&2
  exit 1
}

require_tool() {
  local tool="${1:?tool required}"
  command -v "$tool" >/dev/null 2>&1 || fail "required tool is unavailable: $tool"
}

run_revision_cli() {
  local repository_root="${1:?repository root required}"
  local revision="${2:?source revision required}"
  shift 2

  (
    cd "$repository_root"
    SMACKEREL_COMMIT="$revision" "$@"
  )
}

env_value() {
  local env_file="${1:?environment file required}"
  local key="${2:?environment key required}"
  local value

  value="$(awk -F= -v target="$key" '$1 == target { print substr($0, length($1) + 2); found = 1; exit } END { if (!found) exit 1 }' "$env_file")" \
    || fail "required generated environment key is absent: $key"
  [[ -n "$value" ]] || fail "required generated environment key is empty: $key"
  printf '%s\n' "$value"
}

compose_service_image() {
  local compose_file="${1:?compose file required}"
  local service="${2:?service required}"
  local image

  image="$(awk -v service="$service" '
    $0 == "  " service ":" { in_service = 1; next }
    in_service && /^  [[:alnum:]_-]+:$/ { exit }
    in_service && /^[[:space:]]+image:[[:space:]]+/ {
      sub(/^[[:space:]]+image:[[:space:]]+/, "")
      print
      found = 1
      exit
    }
    END { if (!found) exit 1 }
  ' "$compose_file")" || fail "compose service image is absent: $service"
  [[ -n "$image" && "$image" != *'$'* ]] \
    || fail "compose service image is not a resolved literal: $service"
  printf '%s\n' "$image"
}

resolved_from_lines() {
  local core_dockerfile="${1:?core Dockerfile required}"
  local ml_dockerfile="${2:?ML Dockerfile required}"
  local source_name line first remainder result=""

  for source_name in core ml; do
    if [[ "$source_name" == "core" ]]; then
      while IFS= read -r line; do
        first="${line%%[[:space:]]*}"
        [[ "$first" == "FROM" ]] || continue
        remainder="${line#FROM}"
        [[ "$line" != *'$'* ]] || fail "$source_name Dockerfile contains an unresolved FROM expression"
        if [[ -n "$result" ]]; then
          result="${result};"
        fi
        result="${result}${source_name}:FROM${remainder}"
      done <"$core_dockerfile"
    else
      while IFS= read -r line; do
        first="${line%%[[:space:]]*}"
        [[ "$first" == "FROM" ]] || continue
        remainder="${line#FROM}"
        [[ "$line" != *'$'* ]] || fail "$source_name Dockerfile contains an unresolved FROM expression"
        if [[ -n "$result" ]]; then
          result="${result};"
        fi
        result="${result}${source_name}:FROM${remainder}"
      done <"$ml_dockerfile"
    fi
  done

  [[ -n "$result" ]] || fail "no Dockerfile FROM inputs were resolved"
  printf '%s\n' "$result"
}

image_id_existed_before() {
  local candidate="${1:?image ID required}"
  local existing

  for existing in $PREEXISTING_IMAGE_IDS; do
    [[ "$existing" == "$candidate" ]] && return 0
  done
  return 1
}

remove_owned_container() {
  local container="${1:?container name required}"

  if docker container inspect "$container" >/dev/null 2>&1; then
    docker rm --force "$container"
  fi
}

remove_owned_probe_containers() {
  local container_id container_ids

  container_ids="$(docker ps -aq \
    --filter "label=com.smackerel.test.run-id=$RUN_PREFIX" \
    --filter "label=com.smackerel.e2e-child-run-id=$CHILD_RUN_ID" \
    --filter "label=com.smackerel.test.role=synthesis-http-probe")" || return $?
  while IFS= read -r container_id; do
    [[ -n "$container_id" ]] || continue
    docker rm --force "$container_id"
  done <<<"$container_ids"
}

remove_owned_volume() {
  local volume="${1:?volume name required}"

  if docker volume inspect "$volume" >/dev/null 2>&1; then
    docker volume rm "$volume"
  fi
}

remove_owned_network() {
  local network="${1:?network name required}"

  if docker network inspect "$network" >/dev/null 2>&1; then
    docker network rm "$network"
  fi
}

remove_owned_image_ref() {
  local image_ref="${1:?image reference required}"

  if docker image inspect "$image_ref" >/dev/null 2>&1; then
    docker image rm "$image_ref"
  fi
}

remove_new_final_image() {
  local image_id="${1:?image ID required}"

  if image_id_existed_before "$image_id"; then
    return 0
  fi
  if docker image inspect "$image_id" >/dev/null 2>&1; then
    docker image rm "$image_id"
  fi
}

restore_fixed_build_tag() {
  local status=0

  if [[ "$FIXED_BUILD_TAG_EXISTED" == "1" ]]; then
    docker image tag "$SAVED_FIXED_CORE_REF" "$FIXED_BUILD_TAG" || status=$?
  elif [[ -n "$FIXED_BUILD_TAG" ]]; then
    remove_owned_image_ref "$FIXED_BUILD_TAG" || status=$?
  fi

  if [[ "$FIXED_ML_BUILD_TAG_EXISTED" == "1" ]]; then
    docker image tag "$SAVED_FIXED_ML_REF" "$FIXED_ML_BUILD_TAG" || status=$?
  elif [[ -n "$FIXED_ML_BUILD_TAG" ]]; then
    remove_owned_image_ref "$FIXED_ML_BUILD_TAG" || status=$?
  fi

  return "$status"
}

assert_fixed_build_tags_restored() {
  local current_id=""

  if [[ -n "$FIXED_BUILD_TAG" ]]; then
    if [[ "$FIXED_BUILD_TAG_EXISTED" == "1" ]]; then
      current_id="$(docker image inspect --format '{{.Id}}' "$FIXED_BUILD_TAG")" || return 1
      [[ "$current_id" == "$PREEXISTING_FIXED_BUILD_IMAGE_ID" ]] || return 1
    elif docker image inspect "$FIXED_BUILD_TAG" >/dev/null 2>&1; then
      return 1
    fi
  fi

  if [[ -n "$FIXED_ML_BUILD_TAG" ]]; then
    if [[ "$FIXED_ML_BUILD_TAG_EXISTED" == "1" ]]; then
      current_id="$(docker image inspect --format '{{.Id}}' "$FIXED_ML_BUILD_TAG")" || return 1
      [[ "$current_id" == "$PREEXISTING_FIXED_ML_IMAGE_ID" ]] || return 1
    elif docker image inspect "$FIXED_ML_BUILD_TAG" >/dev/null 2>&1; then
      return 1
    fi
  fi
}

assert_no_owned_resources() {
  local remaining=""
  local ref worktrees image_id

  remaining="$(docker ps -aq --filter "label=com.smackerel.test.run-id=$RUN_PREFIX")"
  [[ -z "$remaining" ]] || {
    echo "FAIL: T004-C21 cleanup left owned containers" >&2
    return 1
  }
  remaining="$(docker volume ls -q --filter "label=com.smackerel.test.run-id=$RUN_PREFIX")"
  [[ -z "$remaining" ]] || {
    echo "FAIL: T004-C21 cleanup left owned volumes" >&2
    return 1
  }
  remaining="$(docker network ls -q --filter "label=com.smackerel.test.run-id=$RUN_PREFIX")"
  [[ -z "$remaining" ]] || {
    echo "FAIL: T004-C21 cleanup left owned networks" >&2
    return 1
  }

  for ref in "$PRIOR_CORE_IMAGE_REF" "$PRIOR_ML_IMAGE_REF" \
    "$CURRENT_CORE_IMAGE_REF" "$CURRENT_ML_IMAGE_REF" \
    "$SAVED_FIXED_CORE_REF" "$SAVED_FIXED_ML_REF"; do
    if docker image inspect "$ref" >/dev/null 2>&1; then
      echo "FAIL: T004-C21 cleanup left owned image reference: $ref" >&2
      return 1
    fi
  done

  for image_id in "$PRIOR_CORE_IMAGE_ID" "$PRIOR_ML_IMAGE_ID" \
    "$CURRENT_CORE_IMAGE_ID" "$CURRENT_ML_IMAGE_ID"; do
    [[ -n "$image_id" ]] || continue
    if image_id_existed_before "$image_id"; then
      continue
    fi
    if docker image inspect "$image_id" >/dev/null 2>&1; then
      echo "FAIL: T004-C21 cleanup left a newly built final image" >&2
      return 1
    fi
  done

  worktrees="$(git -C "$REPO_DIR" worktree list --porcelain)" || return 1
  if [[ "$worktrees" == *"$PRIOR_WORKTREE"* || -e "$PRIOR_WORKTREE" ]]; then
    echo "FAIL: T004-C21 cleanup left the detached worktree" >&2
    return 1
  fi
  assert_fixed_build_tags_restored || {
    echo "FAIL: T004-C21 cleanup did not restore the fixed build tags" >&2
    return 1
  }

  echo "CLEANUP: owned containers=0 volumes=0 networks=0 image_refs=0 worktrees=0 fixed_tags=restored"
}

record_cleanup_failure() {
  local step_status="${1:?cleanup status required}"
  if [[ "$step_status" != "0" && "$CLEANUP_STATUS" == "0" ]]; then
    CLEANUP_STATUS="$step_status"
  fi
}

cleanup() {
  local original_status="${1:?original status required}"
  local final_status="$original_status"
  local step_status=0
  CLEANUP_STATUS=0

  trap - EXIT INT TERM HUP
  set +e

  remove_owned_probe_containers
  record_cleanup_failure "$?"
  remove_owned_container "$CURRENT_CORE_CONTAINER"
  record_cleanup_failure "$?"
  remove_owned_container "$PRIOR_CORE_CONTAINER"
  record_cleanup_failure "$?"
  remove_owned_container "$MIGRATION_CORE_CONTAINER"
  record_cleanup_failure "$?"
  remove_owned_container "$NATS_CONTAINER"
  record_cleanup_failure "$?"
  remove_owned_container "$POSTGRES_CONTAINER"
  record_cleanup_failure "$?"

  restore_fixed_build_tag
  record_cleanup_failure "$?"
  remove_owned_image_ref "$PRIOR_CORE_IMAGE_REF"
  record_cleanup_failure "$?"
  remove_owned_image_ref "$PRIOR_ML_IMAGE_REF"
  record_cleanup_failure "$?"
  remove_owned_image_ref "$CURRENT_CORE_IMAGE_REF"
  record_cleanup_failure "$?"
  remove_owned_image_ref "$CURRENT_ML_IMAGE_REF"
  record_cleanup_failure "$?"
  remove_owned_image_ref "$SAVED_FIXED_CORE_REF"
  record_cleanup_failure "$?"
  remove_owned_image_ref "$SAVED_FIXED_ML_REF"
  record_cleanup_failure "$?"

  for image_id in "$PRIOR_CORE_IMAGE_ID" "$PRIOR_ML_IMAGE_ID" \
    "$CURRENT_CORE_IMAGE_ID" "$CURRENT_ML_IMAGE_ID"; do
    if [[ -n "$image_id" ]]; then
      remove_new_final_image "$image_id"
      record_cleanup_failure "$?"
    fi
  done

  remove_owned_volume "$NATS_VOLUME"
  record_cleanup_failure "$?"
  remove_owned_volume "$POSTGRES_VOLUME"
  record_cleanup_failure "$?"
  remove_owned_network "$NETWORK_NAME"
  record_cleanup_failure "$?"

  if [[ "$PRIOR_WORKTREE_ADDED" == "1" ]]; then
    git -C "$REPO_DIR" worktree remove --force "$PRIOR_WORKTREE"
    step_status=$?
    record_cleanup_failure "$step_status"
  fi
  rm -rf "$TEMP_ROOT"
  record_cleanup_failure "$?"

  assert_no_owned_resources
  record_cleanup_failure "$?"

  if [[ "$final_status" == "0" && "$CLEANUP_STATUS" != "0" ]]; then
    final_status="$CLEANUP_STATUS"
  fi
  if [[ "$final_status" == "0" && "$TEST_PASS_READY" == "1" ]]; then
    echo "PASS: T004-C21 pinned local prior source and current candidate preserve migration 067 without fabricated causal history"
  fi
  exit "$final_status"
}

trap 'cleanup "$?"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

snapshot_fixed_build_tags() {
  PREEXISTING_IMAGE_IDS="$(docker image ls --quiet --no-trunc)"
  if PREEXISTING_FIXED_BUILD_IMAGE_ID="$(docker image inspect --format '{{.Id}}' "$FIXED_BUILD_TAG" 2>&1)"; then
    docker image tag "$PREEXISTING_FIXED_BUILD_IMAGE_ID" "$SAVED_FIXED_CORE_REF"
    FIXED_BUILD_TAG_EXISTED=1
  else
    PREEXISTING_FIXED_BUILD_IMAGE_ID=""
  fi

  if PREEXISTING_FIXED_ML_IMAGE_ID="$(docker image inspect --format '{{.Id}}' "$FIXED_ML_BUILD_TAG" 2>&1)"; then
    docker image tag "$PREEXISTING_FIXED_ML_IMAGE_ID" "$SAVED_FIXED_ML_REF"
    FIXED_ML_BUILD_TAG_EXISTED=1
  else
    PREEXISTING_FIXED_ML_IMAGE_ID=""
  fi

  if ! docker image inspect "$SAVED_FIXED_CORE_REF" >/dev/null 2>&1; then
    [[ "$FIXED_BUILD_TAG_EXISTED" == "0" ]] || fail "fixed core build tag snapshot is unreadable"
  fi
  if ! docker image inspect "$SAVED_FIXED_ML_REF" >/dev/null 2>&1; then
    [[ "$FIXED_ML_BUILD_TAG_EXISTED" == "0" ]] || fail "fixed ML build tag snapshot is unreadable"
  fi
  echo "BUILD_TAGS: fixed core and ML tags snapshotted without changing their final restore targets"
}

assert_immutable_image_id() {
  local image_id="${1:?image ID required}"
  local label="${2:?image label required}"
  [[ "$image_id" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$label is not an immutable local image ID"
}

assert_revision_label() {
  local image_id="${1:?image ID required}"
  local expected_revision="${2:?expected revision required}"
  local label="${3:?image label required}"
  local actual_revision

  actual_revision="$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$image_id")"
  [[ "$actual_revision" == "$expected_revision" ]] \
    || fail "$label OCI revision label does not match its source revision"
}

capture_prior_images() {
  PRIOR_CORE_IMAGE_ID="$(docker image inspect --format '{{.Id}}' "$FIXED_BUILD_TAG")"
  PRIOR_ML_IMAGE_ID="$(docker image inspect --format '{{.Id}}' "$FIXED_ML_BUILD_TAG")"
  assert_immutable_image_id "$PRIOR_CORE_IMAGE_ID" "prior core image"
  assert_immutable_image_id "$PRIOR_ML_IMAGE_ID" "prior ML image"
  assert_revision_label "$PRIOR_CORE_IMAGE_ID" "$PINNED_SOURCE_SHA" "prior core image"
  assert_revision_label "$PRIOR_ML_IMAGE_ID" "$PINNED_SOURCE_SHA" "prior ML image"
  docker image tag "$PRIOR_CORE_IMAGE_ID" "$PRIOR_CORE_IMAGE_REF"
  docker image tag "$PRIOR_ML_IMAGE_ID" "$PRIOR_ML_IMAGE_REF"
  echo "IMAGE: revision=prior core_id=$PRIOR_CORE_IMAGE_ID ml_id=$PRIOR_ML_IMAGE_ID oci_revision=verified"
}

capture_current_images() {
  CURRENT_CORE_IMAGE_ID="$(docker image inspect --format '{{.Id}}' "$FIXED_BUILD_TAG")"
  CURRENT_ML_IMAGE_ID="$(docker image inspect --format '{{.Id}}' "$FIXED_ML_BUILD_TAG")"
  assert_immutable_image_id "$CURRENT_CORE_IMAGE_ID" "candidate core image"
  assert_immutable_image_id "$CURRENT_ML_IMAGE_ID" "candidate ML image"
  assert_revision_label "$CURRENT_CORE_IMAGE_ID" "$CURRENT_REVISION_LABEL" "candidate core image"
  assert_revision_label "$CURRENT_ML_IMAGE_ID" "$CURRENT_REVISION_LABEL" "candidate ML image"
  docker image tag "$CURRENT_CORE_IMAGE_ID" "$CURRENT_CORE_IMAGE_REF"
  docker image tag "$CURRENT_ML_IMAGE_ID" "$CURRENT_ML_IMAGE_REF"
  echo "IMAGE: revision=candidate core_id=$CURRENT_CORE_IMAGE_ID ml_id=$CURRENT_ML_IMAGE_ID oci_revision=verified"
}

require_local_image() {
  local image="${1:?image reference required}"
  docker image inspect "$image" >/dev/null 2>&1 \
    || fail "required infrastructure image is not local; lifecycle source/runtime acquisition is disabled: $image"
}

create_infrastructure() {
  local created network_internal postgres_id nats_id

  require_local_image "$POSTGRES_IMAGE"
  require_local_image "$NATS_IMAGE"

  created="$(docker network create --internal \
    --label "com.smackerel.test.run-id=$RUN_PREFIX" \
    "$NETWORK_NAME")"
  [[ -n "$created" ]] || fail "internal runtime network was not created"
  network_internal="$(docker network inspect --format '{{.Internal}}' "$NETWORK_NAME")"
  [[ "$network_internal" == "true" ]] || fail "runtime network is not Docker internal"

  created="$(docker volume create \
    --label "com.smackerel.test.run-id=$RUN_PREFIX" \
    "$POSTGRES_VOLUME")"
  [[ "$created" == "$POSTGRES_VOLUME" ]] || fail "postgres volume identity mismatch"
  created="$(docker volume create \
    --label "com.smackerel.test.run-id=$RUN_PREFIX" \
    "$NATS_VOLUME")"
  [[ "$created" == "$NATS_VOLUME" ]] || fail "NATS volume identity mismatch"

  postgres_id="$(docker run -d \
    --name "$POSTGRES_CONTAINER" \
    --network "$NETWORK_NAME" \
    --network-alias postgres \
    --label "com.smackerel.test.run-id=$RUN_PREFIX" \
    --label "com.smackerel.e2e-child-run-id=$CHILD_RUN_ID" \
    -e "POSTGRES_USER=$POSTGRES_USER" \
    -e "POSTGRES_PASSWORD=$POSTGRES_PASSWORD" \
    -e "POSTGRES_DB=$POSTGRES_DB" \
    -v "$POSTGRES_VOLUME:/var/lib/postgresql/data" \
    "$POSTGRES_IMAGE")"
  [[ -n "$postgres_id" ]] || fail "postgres container was not created"

  nats_id="$(docker run -d \
    --name "$NATS_CONTAINER" \
    --network "$NETWORK_NAME" \
    --network-alias nats \
    --label "com.smackerel.test.run-id=$RUN_PREFIX" \
    --label "com.smackerel.e2e-child-run-id=$CHILD_RUN_ID" \
    -v "$NATS_VOLUME:/data" \
    "$NATS_IMAGE" -js --store_dir /data)"
  [[ -n "$nats_id" ]] || fail "NATS container was not created"
  echo "INFRA: isolated internal network and uniquely labelled postgres/NATS resources started"
}

wait_for_postgres() {
  local attempt output

  for ((attempt = 1; attempt <= 30; attempt++)); do
    if output="$(docker exec "$POSTGRES_CONTAINER" \
      env "PGPASSWORD=$POSTGRES_PASSWORD" \
      pg_isready -h 127.0.0.1 -p "$POSTGRES_CONTAINER_PORT" \
      -U "$POSTGRES_USER" -d "$POSTGRES_DB" 2>&1)"; then
      echo "READY: postgres accepted connections after probe=$attempt"
      return 0
    fi
    sleep 2
  done
  fail "postgres did not become ready within 30 bounded probes"
}

psql_command() {
  local sql="${1:?SQL command required}"
  local output

  output="$(docker exec "$POSTGRES_CONTAINER" \
    env "PGPASSWORD=$POSTGRES_PASSWORD" \
    psql -h 127.0.0.1 -p "$POSTGRES_CONTAINER_PORT" \
    -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
    -v ON_ERROR_STOP=1 -q -c "$sql" 2>&1)" || {
    printf '%s\n' "$output" >&2
    fail "database command failed"
  }
}

psql_rows() {
  local sql="${1:?SQL query required}"

  docker exec "$POSTGRES_CONTAINER" \
    env "PGPASSWORD=$POSTGRES_PASSWORD" \
    psql -h 127.0.0.1 -p "$POSTGRES_CONTAINER_PORT" \
    -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
    -v ON_ERROR_STOP=1 -A -t -F '|' -c "$sql"
}

prepare_runtime_mounts() {
  mkdir -p "$TEMP_ROOT/bookmarks" "$TEMP_ROOT/maps" \
    "$TEMP_ROOT/browser-history" "$TEMP_ROOT/twitter-archive"
  touch "$TEMP_ROOT/browser-history/History"
}

run_core() {
  local core_image_id="${1:?core image ID required}"
  local ml_image_id="${2:?ML image ID required}"
  local env_file="${3:?generated environment file required}"
  local source_root="${4:?source root required}"
  local phase="${5:?runtime phase required}"
  local container_id network_count running_image child_label run_label published_port_count

  case "$phase" in
    migration) ACTIVE_CORE_CONTAINER="$MIGRATION_CORE_CONTAINER" ;;
    prior) ACTIVE_CORE_CONTAINER="$PRIOR_CORE_CONTAINER" ;;
    current) ACTIVE_CORE_CONTAINER="$CURRENT_CORE_CONTAINER" ;;
    *) fail "unknown core runtime phase: $phase" ;;
  esac
  ACTIVE_ENV_FILE="$env_file"

  [[ -d "$source_root/config/prompt_contracts" ]] || fail "$phase prompt-contract mount is missing"
  [[ -d "$source_root/config/assistant" ]] || fail "$phase assistant-config mount is missing"
  assert_immutable_image_id "$ml_image_id" "$phase ML probe image"
  ACTIVE_ML_IMAGE_ID="$ml_image_id"

  container_id="$(docker run -d \
    --name "$ACTIVE_CORE_CONTAINER" \
    --network "$NETWORK_NAME" \
    --network-alias smackerel-core \
    --label "com.smackerel.test.run-id=$RUN_PREFIX" \
    --label "com.smackerel.e2e-child-run-id=$CHILD_RUN_ID" \
    --read-only \
    --tmpfs /tmp:rw,nosuid,nodev,noexec,size=64m \
    --env-file "$env_file" \
    -e "PORT=$CORE_CONTAINER_PORT" \
    -e "SMACKEREL_AUTH_TOKEN=$AUTH_TOKEN" \
    -e "SMACKEREL_ENV=test" \
    -e "ASSISTANT_ENABLED=false" \
    -e "ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED=false" \
    -e "ASSISTANT_TRANSPORTS_TELEGRAM_MODE=long_poll" \
    -e "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF=" \
    -e "ASSISTANT_TELEGRAM_WEBHOOK_SECRET=" \
    -e "ASSISTANT_INTENT_COMPILER_ENABLED=false" \
    -e "QF_DECISIONS_ENABLED=false" \
    -e "SYNTHESIS_DAILY_CRON=0 0 1 1 *" \
    -e "SYNTHESIS_WEEKLY_CRON=0 0 1 1 *" \
    -e "OLLAMA_URL=http://c21-ollama-unavailable.invalid:11434" \
    -e "ML_SIDECAR_URL=http://c21-ml-unavailable.invalid:8081" \
    -e "AGENT_SCENARIO_DIR=/app/prompt_contracts" \
    -e "PROMPT_CONTRACTS_DIR=/app/prompt_contracts" \
    -e "BOOKMARKS_IMPORT_DIR=/data/bookmarks-import" \
    -e "MAPS_IMPORT_DIR=/data/maps-import" \
    -e "BROWSER_HISTORY_PATH=/data/browser-history/History" \
    -e "TWITTER_ARCHIVE_DIR=/data/twitter-archive" \
    -v "$source_root/config/prompt_contracts:/app/prompt_contracts:ro" \
    -v "$source_root/config/assistant:/app/assistant:ro" \
    -v "$TEMP_ROOT/bookmarks:/data/bookmarks-import:ro" \
    -v "$TEMP_ROOT/maps:/data/maps-import:ro" \
    -v "$TEMP_ROOT/browser-history:/data/browser-history:ro" \
    -v "$TEMP_ROOT/twitter-archive:/data/twitter-archive:ro" \
    "$core_image_id")"
  [[ -n "$container_id" ]] || fail "$phase core container was not created"

  running_image="$(docker container inspect --format '{{.Image}}' "$ACTIVE_CORE_CONTAINER")"
  [[ "$running_image" == "$core_image_id" ]] || fail "$phase core did not start by immutable image ID"
  network_count="$(docker container inspect --format '{{len .NetworkSettings.Networks}}' "$ACTIVE_CORE_CONTAINER")"
  [[ "$network_count" == "1" ]] || fail "$phase core joined an unexpected runtime network"
  child_label="$(docker container inspect --format '{{ index .Config.Labels "com.smackerel.e2e-child-run-id" }}' "$ACTIVE_CORE_CONTAINER")"
  run_label="$(docker container inspect --format '{{ index .Config.Labels "com.smackerel.test.run-id" }}' "$ACTIVE_CORE_CONTAINER")"
  [[ "$child_label" == "$CHILD_RUN_ID" && "$run_label" == "$RUN_PREFIX" ]] \
    || fail "$phase core ownership labels are incomplete"

  published_port_count="$(docker container inspect --format '{{len .HostConfig.PortBindings}}' "$ACTIVE_CORE_CONTAINER")"
  [[ "$published_port_count" == "0" ]] || fail "$phase core unexpectedly published a host port"
  CORE_URL="http://smackerel-core:${CORE_CONTAINER_PORT}"
  echo "RUNTIME: phase=$phase immutable_image_id=$core_image_id network=internal host_ports=0 labels=verified"
}

stop_active_core() {
  if [[ -n "$ACTIVE_CORE_CONTAINER" ]]; then
    remove_owned_container "$ACTIVE_CORE_CONTAINER"
  fi
  ACTIVE_CORE_CONTAINER=""
  ACTIVE_ENV_FILE=""
  ACTIVE_ML_IMAGE_ID=""
  CORE_URL=""
}

# BEGIN C21 STARTUP LOG REDACTOR
redact_startup_logs() {
  local auth_token_file="${1:?authentication token file required}"
  local env_file="${2:?active generated environment file required}"

  [[ -r "$auth_token_file" && -r "$env_file" ]] || return 1
  awk '
    function add_secret(value, insertion_index, secret_index) {
      if (length(value) > 0) {
        for (secret_index = 1; secret_index <= secret_count; secret_index++) {
          if (secrets[secret_index] == value) {
            return
          }
        }
        secret_count++
        insertion_index = secret_count
        while (insertion_index > 1 &&
               length(secrets[insertion_index - 1]) < length(value)) {
          secrets[insertion_index] = secrets[insertion_index - 1]
          insertion_index--
        }
        secrets[insertion_index] = value
      }
    }

    function trim(value) {
      sub(/^[[:space:]]+/, "", value)
      sub(/[[:space:]]+$/, "", value)
      return value
    }

    function redact_literal(line, needle, output, remaining, position) {
      output = ""
      remaining = line
      while ((position = index(remaining, needle)) > 0) {
        output = output substr(remaining, 1, position - 1) "[REDACTED]"
        remaining = substr(remaining, position + length(needle))
      }
      return output remaining
    }

    input_kind == "auth" {
      value = $0
      sub(/\r$/, "", value)
      add_secret(value)
      next
    }

    input_kind == "env" {
      line = $0
      sub(/\r$/, "", line)
      if (line ~ /^[[:space:]]*(#|$)/) {
        next
      }
      sub(/^[[:space:]]*export[[:space:]]+/, "", line)
      separator = index(line, "=")
      if (separator == 0) {
        next
      }
      key = trim(substr(line, 1, separator - 1))
      value = substr(line, separator + 1)
      if (length(value) >= 2) {
        first_character = substr(value, 1, 1)
        last_character = substr(value, length(value), 1)
        single_quote = sprintf("%c", 39)
        if ((first_character == "\"" && last_character == "\"") ||
            (first_character == single_quote && last_character == single_quote)) {
          value = substr(value, 2, length(value) - 2)
        }
      }
      upper_key = toupper(key)
      if (upper_key == "DATABASE_URL" ||
          upper_key == "POSTGRES_PASSWORD" ||
          upper_key ~ /(TOKEN|KEY|PASSWORD|SECRET|CREDENTIAL|PRIVATE)/) {
        add_secret(value)
      }
      next
    }

    input_kind == "logs" {
      line = $0
      for (secret_index = 1; secret_index <= secret_count; secret_index++) {
        line = redact_literal(line, secrets[secret_index])
      }
      print line
    }
  ' input_kind=auth "$auth_token_file" input_kind=env "$env_file" input_kind=logs -
}
# END C21 STARTUP LOG REDACTOR

emit_stopped_core_diagnostics() {
  local phase="${1:?runtime phase required}"
  local exit_code="${2:?numeric exit code required}"
  local redacted_log_file="$TEMP_ROOT/${phase}-core-startup.redacted.log"
  local docker_logs_status redactor_status
  local -a pipeline_status

  printf 'SAFE-DIAGNOSTIC: core-startup-failure phase=%s exit_code=%s\n' "$phase" "$exit_code" >&2
  if [[ ! -r "$AUTH_TOKEN_FILE" || ! -r "$ACTIVE_ENV_FILE" ]]; then
    echo "SAFE-DIAGNOSTIC: docker_logs=unavailable reason=redaction-input-unreadable" >&2
    return 0
  fi
  if ! (umask 077 && printf '' >"$redacted_log_file"); then
    echo "SAFE-DIAGNOSTIC: docker_logs=unavailable reason=secure-buffer-create-failed" >&2
    return 0
  fi

  if docker logs "$ACTIVE_CORE_CONTAINER" 2>&1 | redact_startup_logs "$AUTH_TOKEN_FILE" "$ACTIVE_ENV_FILE" >"$redacted_log_file"; then
    if ! cat "$redacted_log_file" >&2; then
      echo "SAFE-DIAGNOSTIC: docker_logs=unavailable reason=redacted-output-failed" >&2
    fi
    return 0
  fi

  pipeline_status=("${PIPESTATUS[@]}")
  docker_logs_status="${pipeline_status[0]:?docker logs status missing}"
  redactor_status="${pipeline_status[1]:?startup redactor status missing}"
  if [[ "$docker_logs_status" != "0" ]]; then
    echo "SAFE-DIAGNOSTIC: docker_logs=unavailable reason=retrieval-failed" >&2
  elif [[ "$redactor_status" != "0" ]]; then
    echo "SAFE-DIAGNOSTIC: docker_logs=unavailable reason=redaction-failed" >&2
  else
    echo "SAFE-DIAGNOSTIC: docker_logs=unavailable reason=pipeline-status-invalid" >&2
  fi
}

run_http_probe() {
  [[ $# -eq 5 ]] || fail "HTTP probe requires method, URL, body, auth mode, and timeout"
  local method="$1"
  local url="$2"
  local body="$3"
  local auth_mode="$4"
  local request_timeout="$5"
  local container_timeout expected_failure_body probe_body probe_exit probe_name probe_result probe_status

  [[ -r "$HTTP_PROBE_HELPER" ]] || fail "same-network HTTP probe helper is unreadable"
  [[ -r "$AUTH_TOKEN_FILE" ]] || fail "disposable authentication token file is unreadable"
  assert_immutable_image_id "$ACTIVE_ML_IMAGE_ID" "active ML probe image"
  [[ "$request_timeout" =~ ^[1-9][0-9]*$ ]] \
    || fail "HTTP probe timeout is not a positive integer"
  ((request_timeout <= 60)) || fail "HTTP probe timeout exceeds the bounded maximum"

  ((PROBE_SEQUENCE += 1))
  probe_name="${RUN_PREFIX}-http-probe-${PROBE_SEQUENCE}"
  container_timeout="$((request_timeout + 5))"
  if probe_result="$(smackerel_run_with_timeout --kill-after=5s "$container_timeout" docker run --rm \
    --pull=never \
    --name "$probe_name" \
    --network "$NETWORK_NAME" \
    --label "com.smackerel.test.run-id=$RUN_PREFIX" \
    --label "com.smackerel.e2e-child-run-id=$CHILD_RUN_ID" \
    --label "com.smackerel.test.role=synthesis-http-probe" \
    --read-only \
    --user "$(id -u):$(id -g)" \
    --entrypoint python3 \
    -v "$HTTP_PROBE_HELPER:/probe/synthesis_http_probe.py:ro" \
    -v "$AUTH_TOKEN_FILE:/run/secrets/synthesis-auth-token:ro" \
    "$ACTIVE_ML_IMAGE_ID" \
    -B /probe/synthesis_http_probe.py \
    --method "$method" \
    --url "$url" \
    --body "$body" \
    --auth-mode "$auth_mode" \
    --auth-token-file /run/secrets/synthesis-auth-token \
    --timeout-seconds "$request_timeout")"; then
    probe_exit=0
  else
    probe_exit=$?
  fi

  case "$probe_exit" in
    0 | 2 | 3 | 4) ;;
    *)
      HTTP_STATUS="0"
      HTTP_BODY="HTTP probe runner failed"
      return "$probe_exit"
      ;;
  esac

  if [[ -z "$probe_result" || "$probe_result" == *$'\n'* || "$probe_result" == *$'\r'* ]] \
    || ! jq -e 'type == "object" and (keys == ["body", "status"])' <<<"$probe_result" >/dev/null 2>&1; then
    HTTP_STATUS="0"
    HTTP_BODY="HTTP probe result envelope invalid"
    return 1
  fi
  if ! probe_status="$(jq -er '.status | select(type == "number") | tostring' <<<"$probe_result" 2>/dev/null)" \
    || [[ ! "$probe_status" =~ ^(0|[1-5][0-9][0-9])$ ]]; then
    HTTP_STATUS="0"
    HTTP_BODY="HTTP probe result status invalid"
    return 1
  fi
  if ! probe_body="$(jq -er '.body | select(type == "string")' <<<"$probe_result" 2>/dev/null)"; then
    HTTP_STATUS="0"
    HTTP_BODY="HTTP probe result body invalid"
    return 1
  fi

  HTTP_STATUS="$probe_status"
  HTTP_BODY="$probe_body"
  if [[ "$probe_exit" == "0" ]]; then
    if [[ ! "$probe_status" =~ ^[1-5][0-9][0-9]$ ]]; then
      HTTP_STATUS="0"
      HTTP_BODY="HTTP probe success envelope invalid"
      return 1
    fi
    return 0
  fi

  case "$probe_exit" in
    2) expected_failure_body="invalid probe input" ;;
    3) expected_failure_body="HTTP transport failed" ;;
    4) expected_failure_body="HTTP probe failed" ;;
  esac
  if [[ "$probe_status" != "0" || "$probe_body" != "$expected_failure_body" ]]; then
    HTTP_STATUS="0"
    HTTP_BODY="HTTP probe failure envelope invalid"
    return 1
  fi
  return "$probe_exit"
}

try_api_get() {
  local path="${1:?API path required}"

  run_http_probe "GET" "$CORE_URL$path" "" "bearer" "30"
}

api_get() {
  local path="${1:?API path required}"
  local probe_status=0

  run_http_probe "GET" "$CORE_URL$path" "" "bearer" "30" || probe_status=$?
  [[ "$probe_status" == "0" ]] \
    || fail "authenticated GET HTTP probe transport/input failure: status=$HTTP_STATUS reason=$HTTP_BODY"
}

api_get_without_auth() {
  local path="${1:?API path required}"
  local probe_status=0

  run_http_probe "GET" "$CORE_URL$path" "" "none" "30" || probe_status=$?
  [[ "$probe_status" == "0" ]] \
    || fail "unauthenticated GET HTTP probe transport/input failure: status=$HTTP_STATUS reason=$HTTP_BODY"
}

api_retry() {
  local cadence="${1:?synthesis cadence required}"
  local probe_status=0 request_body

  request_body="{\"cadence\":\"$cadence\"}"
  run_http_probe "POST" "$CORE_URL/api/synthesis/retry" "$request_body" "bearer" "60" || probe_status=$?
  [[ "$probe_status" == "0" ]] \
    || fail "authenticated retry HTTP probe transport/input failure: status=$HTTP_STATUS reason=$HTTP_BODY"
}

wait_for_core() {
  local phase="${1:?runtime phase required}"
  local attempt core_exit_code health_status running
  local last_probe_reason="probe not attempted"

  for ((attempt = 1; attempt <= 30; attempt++)); do
    running="$(docker container inspect --format '{{.State.Running}}' "$ACTIVE_CORE_CONTAINER" 2>/dev/null)" || running="false"
    if [[ "$running" != "true" ]]; then
      if ! core_exit_code="$(docker container inspect --format '{{.State.ExitCode}}' "$ACTIVE_CORE_CONTAINER" 2>/dev/null)"; then
        echo "SAFE-DIAGNOSTIC: core-startup-failure phase=$phase exit_code=unavailable" >&2
        echo "SAFE-DIAGNOSTIC: docker_logs=unavailable reason=exit-code-retrieval-failed" >&2
        fail "$phase core exited before readiness"
      fi
      [[ "$core_exit_code" =~ ^[0-9]+$ ]] || fail "$phase core returned a non-numeric exit code"
      emit_stopped_core_diagnostics "$phase" "$core_exit_code"
      fail "$phase core exited before readiness"
    fi
    if try_api_get "/api/health" && [[ "$HTTP_STATUS" == "200" ]]; then
      health_status="$(jq -er '.status | select(. == "healthy" or . == "degraded")' <<<"$HTTP_BODY" 2>/dev/null)" || health_status=""
      if [[ -n "$health_status" ]]; then
        echo "READY: phase=$phase authenticated_liveness=$health_status probe=$attempt"
        return 0
      fi
      last_probe_reason="HTTP 200 returned a non-ready health body"
    elif [[ "$HTTP_STATUS" == "0" ]]; then
      last_probe_reason="$HTTP_BODY"
    else
      last_probe_reason="HTTP status $HTTP_STATUS"
    fi
    sleep 2
  done
  fail "$phase core did not become ready within 30 bounded probes: last_probe_status=$HTTP_STATUS reason=$last_probe_reason"
}

assert_migration_067() {
  local migration_count
  migration_count="$(psql_rows "SELECT COUNT(*) FROM schema_migrations WHERE version = '067_synthesis_causal_event_truth.sql';")"
  [[ "$migration_count" == "1" ]] || fail "migration 067 is not recorded exactly once"
}

reset_synthesis_state() {
  psql_command "
TRUNCATE synthesis_run_events,
         synthesis_citations,
         synthesis_output_insights,
         synthesis_output_source_classes,
         synthesis_outputs,
         synthesis_run_attempts,
         synthesis_runs,
         synthesis_insights,
         weekly_synthesis
RESTART IDENTITY CASCADE;
"
  echo "RESET: synthesis persistence is empty after migration 067"
}

persistence_counts() {
  psql_rows "
SELECT (SELECT COUNT(*) FROM synthesis_runs),
       (SELECT COUNT(*) FROM synthesis_outputs),
       (SELECT COUNT(*) FROM synthesis_run_attempts),
       (SELECT COUNT(*) FROM synthesis_run_events),
       (SELECT COUNT(*) FROM synthesis_run_attempts
          WHERE run_id IS NOT NULL OR attempt_no IS NOT NULL
             OR trigger_kind IS NOT NULL OR state IS NOT NULL
             OR output_id IS NOT NULL OR started_at IS NOT NULL
             OR finished_at IS NOT NULL OR failure_code IS NOT NULL
             OR included_source_classes IS NOT NULL
             OR omitted_source_classes IS NOT NULL
             OR insight_count IS NOT NULL OR citation_count IS NOT NULL),
       (SELECT COUNT(*) FROM synthesis_runs
          WHERE state = 'running' OR lease_holder IS NOT NULL OR lease_expires_at IS NOT NULL);
"
}

assert_prior_zero_causal_history() {
  local row runs outputs attempts events causal_attempts running_runs

  row="$(persistence_counts)"
  IFS='|' read -r runs outputs attempts events causal_attempts running_runs <<<"$row"
  for value in "$runs" "$outputs" "$attempts" "$events" "$causal_attempts" "$running_runs"; do
    [[ "$value" =~ ^[0-9]+$ ]] || fail "prior persistence count is not numeric"
  done
  [[ "$events" == "0" ]] || fail "migration 067 inferred causal events for prior-source writes"
  [[ "$causal_attempts" == "0" ]] || fail "migration 067 inferred causal attempt fields for prior-source writes"
  [[ "$running_runs" == "0" ]] || fail "prior-source write left an unclosed run or lease"
  echo "LEGACY: runs=$runs outputs=$outputs attempts=$attempts causal_attempts=0 inferred_events=0"
}

parse_output_presence() {
  jq -er 'if type == "object" then (has("output") | tostring) else error("expected object") end'
}

assert_authenticated_prior_read() {
  local state has_output error_code

  api_get_without_auth "/api/synthesis/latest"
  [[ "$HTTP_STATUS" == "401" ]] || fail "prior synthesis read did not enforce authentication"
  error_code="$(jq -er '.error.code | select(type == "string" and length > 0)' <<<"$HTTP_BODY")" \
    || fail "prior unauthenticated read did not return a structured refusal"

  api_get "/api/synthesis/latest"
  [[ "$HTTP_STATUS" == "200" ]] || fail "authenticated prior synthesis read failed"
  state="$(jq -er '.state | select(type == "string" and length > 0)' <<<"$HTTP_BODY")" \
    || fail "authenticated prior synthesis read omitted state"
  has_output="$(parse_output_presence <<<"$HTTP_BODY")" \
    || fail "authenticated prior synthesis read has an invalid shape"
  [[ "$state" == "never-run" && "$has_output" == "false" ]] \
    || fail "prior synthesis baseline is not the explicit empty state"
  echo "READ: prior authentication=required latest_state=never-run refusal_code=$error_code"
}

assert_prior_output_readable() {
  local cadence="${1:?cadence required}"
  local output_id="${2:?output ID required}"
  local response_id

  api_get "/api/synthesis/runs/$output_id"
  [[ "$HTTP_STATUS" == "200" ]] || fail "prior $cadence output is not readable through the authenticated API"
  response_id="$(jq -er '.output.outputId | select(type == "string" and length > 0)' <<<"$HTTP_BODY")" \
    || fail "prior $cadence detail response omitted output identity"
  [[ "$response_id" == "$output_id" ]] || fail "prior $cadence detail response changed output identity"
}

prior_write_or_fail_closed() {
  local cadence="${1:?cadence required}"
  local before after
  local before_runs before_outputs before_attempts before_events before_causal before_running
  local after_runs after_outputs after_attempts after_events after_causal after_running
  local output_id db_row error_code safe_shape result

  before="$(persistence_counts)"
  IFS='|' read -r before_runs before_outputs before_attempts before_events before_causal before_running <<<"$before"
  [[ "$before_events" == "0" && "$before_causal" == "0" && "$before_running" == "0" ]] \
    || fail "prior $cadence write began with fabricated causal state or an open lease"
  api_retry "$cadence"

  if [[ "$HTTP_STATUS" == "200" ]]; then
    jq -er '.outcome | select(. == "persisted")' <<<"$HTTP_BODY" >/dev/null \
      || fail "prior $cadence success did not carry the persisted outcome"
    output_id="$(jq -er '.output.outputId | select(type == "string" and length > 0)' <<<"$HTTP_BODY")" \
      || fail "prior $cadence success omitted output identity"
    db_row="$(psql_rows "
SELECT r.cadence, r.state, o.lifecycle_state
FROM synthesis_outputs o
JOIN synthesis_runs r ON r.id = o.run_id
WHERE o.id = '$output_id';
")"
    [[ "$db_row" == "$cadence|succeeded|current" ]] \
      || fail "prior $cadence success did not round-trip through migration 067"
    assert_prior_output_readable "$cadence" "$output_id"
    result="compatible"
  else
    case "$HTTP_STATUS" in
      422 | 500) ;;
      *) fail "prior $cadence write returned a non-fail-closed HTTP status: $HTTP_STATUS" ;;
    esac
    error_code="$(jq -er '.error.code | select(type == "string" and length > 0)' <<<"$HTTP_BODY")" \
      || fail "prior $cadence failure omitted its safe error code"
    safe_shape="$(jq -er '((has("outcome") or has("output") or has("delivery") or has("delivered") or has("healthy") or has("persisted") or has("recovered")) | not)' <<<"$HTTP_BODY")" \
      || fail "prior $cadence failure response has an invalid shape"
    [[ "$safe_shape" == "true" && "$HTTP_BODY" != *"$AUTH_TOKEN"* ]] \
      || fail "prior $cadence failure exposed success state or authentication material"
    output_id=""
    result="fail-closed:$error_code"
  fi

  after="$(persistence_counts)"
  IFS='|' read -r after_runs after_outputs after_attempts after_events after_causal after_running <<<"$after"
  if [[ "$HTTP_STATUS" == "200" ]]; then
    ((after_runs == before_runs + 1)) || fail "prior $cadence compatible write did not add exactly one run"
    ((after_outputs == before_outputs + 1)) || fail "prior $cadence compatible write did not add exactly one output"
    ((after_attempts == before_attempts + 1)) || fail "prior $cadence compatible write did not add exactly one legacy attempt"
  else
    [[ "$after_outputs" == "$before_outputs" ]] || fail "prior $cadence failed response left success-shaped output"
  fi
  [[ "$after_events" == "$before_events" && "$after_events" == "0" ]] \
    || fail "prior $cadence write acquired fabricated causal events"
  [[ "$after_causal" == "0" && "$after_running" == "0" ]] \
    || fail "prior $cadence write acquired fabricated causal linkage or an open lease"

  case "$cadence" in
    daily)
      PRIOR_DAILY_OUTPUT_ID="$output_id"
      ;;
    weekly)
      PRIOR_WEEKLY_OUTPUT_ID="$output_id"
      ;;
    *) fail "unsupported prior cadence: $cadence" ;;
  esac
  echo "WRITE: revision=prior cadence=$cadence result=$result legacy_causal_events=0"
}

capture_prior_persistence_snapshot() {
  local row causal_attempts running_runs

  row="$(persistence_counts)"
  IFS='|' read -r PRIOR_RUN_COUNT PRIOR_OUTPUT_COUNT PRIOR_ATTEMPT_COUNT \
    PRIOR_EVENT_COUNT causal_attempts running_runs <<<"$row"
  PRIOR_LEGACY_ATTEMPT_COUNT="$PRIOR_ATTEMPT_COUNT"
  [[ "$PRIOR_EVENT_COUNT" == "0" && "$causal_attempts" == "0" && "$running_runs" == "0" ]] \
    || fail "prior persistence snapshot contains invented causal state"
  assert_migration_067
  echo "SNAPSHOT: prior runs=$PRIOR_RUN_COUNT outputs=$PRIOR_OUTPUT_COUNT attempts=$PRIOR_ATTEMPT_COUNT migration_067=retained"
}

seed_candidate_source_set() {
  local topic_id="${RUN_PREFIX}-topic"
  local artifact_a="${RUN_PREFIX}-artifact-a"
  local artifact_b="${RUN_PREFIX}-artifact-b"
  local artifact_c="${RUN_PREFIX}-artifact-c"
  local counts

  psql_command "
INSERT INTO topics (id, name)
VALUES ('$topic_id', 'C21 retained-data compatibility');

INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
VALUES
  ('$artifact_a', 'article', 'C21 source A', '${RUN_PREFIX}-hash-a', '${RUN_PREFIX}-source-a'),
  ('$artifact_b', 'article', 'C21 source B', '${RUN_PREFIX}-hash-b', '${RUN_PREFIX}-source-b'),
  ('$artifact_c', 'article', 'C21 source C', '${RUN_PREFIX}-hash-c', '${RUN_PREFIX}-source-c');

INSERT INTO edges (id, src_id, src_type, dst_id, dst_type, edge_type)
VALUES
  ('${RUN_PREFIX}-edge-a', '$artifact_a', 'artifact', '$topic_id', 'topic', 'BELONGS_TO'),
  ('${RUN_PREFIX}-edge-b', '$artifact_b', 'artifact', '$topic_id', 'topic', 'BELONGS_TO'),
  ('${RUN_PREFIX}-edge-c', '$artifact_c', 'artifact', '$topic_id', 'topic', 'BELONGS_TO');
"
  counts="$(psql_rows "
SELECT (SELECT COUNT(*) FROM artifacts WHERE id LIKE '${RUN_PREFIX}-artifact-%'),
       (SELECT COUNT(DISTINCT source_id) FROM artifacts WHERE id LIKE '${RUN_PREFIX}-artifact-%'),
       (SELECT COUNT(*) FROM edges WHERE dst_id = '$topic_id' AND edge_type = 'BELONGS_TO');
")"
  [[ "$counts" == "3|3|3" ]] || fail "candidate source-set seed is incomplete"
  echo "SEED: candidate artifacts=3 distinct_sources=3 edges=3"
}

assert_prior_rows_retained() {
  local retained

  if [[ -n "$PRIOR_DAILY_OUTPUT_ID" ]]; then
    retained="$(psql_rows "SELECT COUNT(*) FROM synthesis_outputs WHERE id = '$PRIOR_DAILY_OUTPUT_ID';")"
    [[ "$retained" == "1" ]] || fail "candidate lost the prior daily output"
  fi
  if [[ -n "$PRIOR_WEEKLY_OUTPUT_ID" ]]; then
    retained="$(psql_rows "SELECT COUNT(*) FROM synthesis_outputs WHERE id = '$PRIOR_WEEKLY_OUTPUT_ID';")"
    [[ "$retained" == "1" ]] || fail "candidate lost the prior weekly output"
  fi
}

# BEGIN C21 CANDIDATE FAILURE SNAPSHOT
safe_candidate_error_code() {
  local response_body="${1-}"
  local candidate=""

  if candidate="$(jq -er '.error.code | select(type == "string")' <<<"$response_body" 2>/dev/null)" \
    && [[ "$candidate" =~ ^[a-z0-9_]+$ ]]; then
    printf '%s\n' "$candidate"
    return 0
  fi
  printf '%s\n' "invalid_error_envelope"
}

is_synthesis_run_state() {
  case "${1-}" in
    running | succeeded | failed) return 0 ;;
    *) return 1 ;;
  esac
}

is_synthesis_lifecycle_state() {
  case "${1-}" in
    current | stale | superseded | archived) return 0 ;;
    *) return 1 ;;
  esac
}

is_synthesis_attempt_state() {
  case "${1-}" in
    legacy_unlinked | running | persisted | quiet | partial | idempotent | rolled_back | retryable_failure | failed | readback_failed | recovered) return 0 ;;
    *) return 1 ;;
  esac
}

is_synthesis_attempt_outcome() {
  case "${1-}" in
    running | succeeded | failed | idempotent_no_change | retryable_failure | rolled_back | readback_failed | recovered) return 0 ;;
    *) return 1 ;;
  esac
}

is_synthesis_event_type() {
  case "${1-}" in
    claimed | attempt_started | idempotent | persisted | quiet | partial | rolled_back | retryable_failure | failed | readback_failed | recovered | superseded) return 0 ;;
    *) return 1 ;;
  esac
}

is_synthesis_failure_code() {
  case "${1-}" in
    none | missing_citation | unauthorized_source | invalid_payload | required_source_omitted | confidence_out_of_band | transaction_failed | readback_failed | audit_persistence_failed) return 0 ;;
    *) return 1 ;;
  esac
}

is_snapshot_count() {
  [[ "${1-}" =~ ^(0|[1-9][0-9]*)$ ]]
}

emit_synthesis_run_snapshot() {
  local rows="${1-}"
  local row state lifecycle_state count

  if [[ -z "$rows" ]]; then
    echo "SAFE-SNAPSHOT: group=synthesis_runs rows=0" >&2
    return 0
  fi
  while IFS= read -r row; do
    IFS='|' read -r state lifecycle_state count <<<"$row"
    [[ "$row" == "$state|$lifecycle_state|$count" ]] || return 1
    is_synthesis_run_state "$state" || return 1
    is_synthesis_lifecycle_state "$lifecycle_state" || return 1
    is_snapshot_count "$count" || return 1
    printf 'SAFE-SNAPSHOT: group=synthesis_runs state=%s lifecycle_state=%s count=%s\n' \
      "$state" "$lifecycle_state" "$count" >&2
  done <<<"$rows"
}

emit_synthesis_attempt_snapshot() {
  local rows="${1-}"
  local row state outcome failure_code count

  if [[ -z "$rows" ]]; then
    echo "SAFE-SNAPSHOT: group=synthesis_run_attempts rows=0" >&2
    return 0
  fi
  while IFS= read -r row; do
    IFS='|' read -r state outcome failure_code count <<<"$row"
    [[ "$row" == "$state|$outcome|$failure_code|$count" ]] || return 1
    is_synthesis_attempt_state "$state" || return 1
    is_synthesis_attempt_outcome "$outcome" || return 1
    is_synthesis_failure_code "$failure_code" || return 1
    is_snapshot_count "$count" || return 1
    printf 'SAFE-SNAPSHOT: group=synthesis_run_attempts state=%s outcome=%s failure_code=%s count=%s\n' \
      "$state" "$outcome" "$failure_code" "$count" >&2
  done <<<"$rows"
}

emit_synthesis_event_snapshot() {
  local rows="${1-}"
  local row event_type failure_code count

  if [[ -z "$rows" ]]; then
    echo "SAFE-SNAPSHOT: group=synthesis_run_events rows=0" >&2
    return 0
  fi
  while IFS= read -r row; do
    IFS='|' read -r event_type failure_code count <<<"$row"
    [[ "$row" == "$event_type|$failure_code|$count" ]] || return 1
    is_synthesis_event_type "$event_type" || return 1
    is_synthesis_failure_code "$failure_code" || return 1
    is_snapshot_count "$count" || return 1
    printf 'SAFE-SNAPSHOT: group=synthesis_run_events event_type=%s failure_code=%s count=%s\n' \
      "$event_type" "$failure_code" "$count" >&2
  done <<<"$rows"
}

emit_candidate_write_failure_snapshot() {
  local expected_actor="${1:?configured synthesis actor required}"
  local rows current_output_count

  if rows="$(psql_rows "
SELECT state, lifecycle_state, COUNT(*)
FROM synthesis_runs
GROUP BY state, lifecycle_state
ORDER BY state, lifecycle_state;
" 2>/dev/null)"; then
    if ! emit_synthesis_run_snapshot "$rows"; then
      echo "SAFE-DIAGNOSTIC: candidate-write-snapshot group=synthesis_runs status=invalid_aggregate" >&2
    fi
  else
    echo "SAFE-DIAGNOSTIC: candidate-write-snapshot group=synthesis_runs status=query_failed" >&2
  fi

  if rows="$(psql_rows "
SELECT COALESCE(state, 'legacy_unlinked'),
       outcome,
       COALESCE(failure_code, 'none'),
       COUNT(*)
FROM synthesis_run_attempts
GROUP BY COALESCE(state, 'legacy_unlinked'), outcome, COALESCE(failure_code, 'none')
ORDER BY 1, 2, 3;
" 2>/dev/null)"; then
    if ! emit_synthesis_attempt_snapshot "$rows"; then
      echo "SAFE-DIAGNOSTIC: candidate-write-snapshot group=synthesis_run_attempts status=invalid_aggregate" >&2
    fi
  else
    echo "SAFE-DIAGNOSTIC: candidate-write-snapshot group=synthesis_run_attempts status=query_failed" >&2
  fi

  if rows="$(psql_rows "
SELECT event_type, COALESCE(failure_code, 'none'), COUNT(*)
FROM synthesis_run_events
GROUP BY event_type, COALESCE(failure_code, 'none')
ORDER BY 1, 2;
" 2>/dev/null)"; then
    if ! emit_synthesis_event_snapshot "$rows"; then
      echo "SAFE-DIAGNOSTIC: candidate-write-snapshot group=synthesis_run_events status=invalid_aggregate" >&2
    fi
  else
    echo "SAFE-DIAGNOSTIC: candidate-write-snapshot group=synthesis_run_events status=query_failed" >&2
  fi

  if [[ ! "$expected_actor" =~ ^[a-z0-9_-]+$ ]]; then
    echo "SAFE-DIAGNOSTIC: candidate-write-snapshot group=current_outputs status=invalid_actor_token" >&2
  elif current_output_count="$(psql_rows "
SELECT COUNT(*)
FROM synthesis_outputs
WHERE principal = '$expected_actor'
  AND cadence = 'daily'
  AND lifecycle_state = 'current'
  AND window_start = (
    (date_trunc('day', CURRENT_TIMESTAMP AT TIME ZONE 'UTC') - INTERVAL '1 day')
    AT TIME ZONE 'UTC'
  )
  AND window_end = (
    date_trunc('day', CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
    AT TIME ZONE 'UTC'
  );
" 2>/dev/null)"; then
    if is_snapshot_count "$current_output_count"; then
      printf 'SAFE-SNAPSHOT: group=current_outputs scope=configured_actor_daily_current_window count=%s\n' \
        "$current_output_count" >&2
    else
      echo "SAFE-DIAGNOSTIC: candidate-write-snapshot group=current_outputs status=invalid_aggregate" >&2
    fi
  else
    echo "SAFE-DIAGNOSTIC: candidate-write-snapshot group=current_outputs status=query_failed" >&2
  fi
  return 0
}
# END C21 CANDIDATE FAILURE SNAPSHOT

assert_candidate_causal_write() {
  local expected_actor error_code output_id detail_id row
  local principal cadence trigger_kind attempt_state attempt_no lifecycle
  local started_count terminal_count matching_terminal_count linked_attempt_count
  local totals total_runs total_outputs total_attempts total_events legacy_attempts linked_attempts migration_count

  expected_actor="$(env_value "$CURRENT_ENV_FILE" "SYNTHESIS_ACTOR_USER_ID")"
  api_retry "daily"
  if [[ "$HTTP_STATUS" != "200" ]]; then
    error_code="$(safe_candidate_error_code "$HTTP_BODY")"
    printf 'SAFE-DIAGNOSTIC: candidate-write-failure http_status=%s error_code=%s\n' \
      "$HTTP_STATUS" "$error_code" >&2
    emit_candidate_write_failure_snapshot "$expected_actor"
    fail "candidate strict causal write failed with HTTP $HTTP_STATUS"
  fi
  jq -er '.outcome | select(. == "persisted")' <<<"$HTTP_BODY" >/dev/null \
    || fail "candidate write did not carry the persisted outcome"
  output_id="$(jq -er '.output.outputId | select(type == "string" and length > 0)' <<<"$HTTP_BODY")" \
    || fail "candidate write omitted output identity"

  row="$(psql_rows "
SELECT r.principal,
       r.cadence,
       a.trigger_kind,
       a.state,
       a.attempt_no,
       o.lifecycle_state,
       COUNT(e.id) FILTER (WHERE e.event_type = 'attempt_started'),
       COUNT(e.id) FILTER (WHERE e.event_type IN (
         'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
         'retryable_failure', 'failed', 'readback_failed', 'recovered'
       )),
       COUNT(e.id) FILTER (WHERE e.event_type IN (
         'idempotent', 'persisted', 'quiet', 'partial', 'recovered'
       ) AND e.output_id = o.id),
       (SELECT COUNT(*) FROM synthesis_run_attempts linked
          WHERE linked.run_id = r.id AND linked.output_id = o.id)
FROM synthesis_outputs o
JOIN synthesis_runs r ON r.id = o.run_id
JOIN synthesis_run_attempts a ON a.run_id = r.id AND a.output_id = o.id
LEFT JOIN synthesis_run_events e
  ON e.run_id = a.run_id AND e.attempt_no = a.attempt_no
WHERE o.id = '$output_id'
GROUP BY r.principal, r.cadence, a.trigger_kind, a.state, a.attempt_no,
         o.lifecycle_state, r.id, o.id;
")"
  IFS='|' read -r principal cadence trigger_kind attempt_state attempt_no lifecycle \
    started_count terminal_count matching_terminal_count linked_attempt_count <<<"$row"
  [[ "$principal" == "$expected_actor" ]] || fail "candidate causal write used the wrong configured actor"
  [[ "$cadence" == "daily" && "$trigger_kind" == "operator_retry" ]] \
    || fail "candidate causal write crossed cadence or trigger identity"
  case "$attempt_state" in
    persisted | quiet | partial | recovered | idempotent) ;;
    *) fail "candidate causal write has a non-success terminal state" ;;
  esac
  [[ "$attempt_no" =~ ^[1-9][0-9]*$ ]] || fail "candidate causal attempt number is invalid"
  [[ "$lifecycle" == "current" ]] || fail "candidate output is not current"
  [[ "$started_count" == "1" && "$terminal_count" == "1" &&
    "$matching_terminal_count" == "1" && "$linked_attempt_count" == "1" ]] \
    || fail "candidate causal attempt/event/output chain is incomplete or cross-paired"

  api_get "/api/synthesis/runs/$output_id"
  [[ "$HTTP_STATUS" == "200" ]] || fail "candidate causal output is not readable"
  detail_id="$(jq -er '.output.outputId | select(type == "string" and length > 0)' <<<"$HTTP_BODY")" \
    || fail "candidate causal output detail omitted identity"
  [[ "$detail_id" == "$output_id" ]] || fail "candidate causal output detail changed identity"

  totals="$(psql_rows "
SELECT (SELECT COUNT(*) FROM synthesis_runs),
       (SELECT COUNT(*) FROM synthesis_outputs),
       (SELECT COUNT(*) FROM synthesis_run_attempts),
       (SELECT COUNT(*) FROM synthesis_run_events),
       (SELECT COUNT(*) FROM synthesis_run_attempts WHERE run_id IS NULL),
       (SELECT COUNT(*) FROM synthesis_run_attempts WHERE run_id IS NOT NULL),
       (SELECT COUNT(*) FROM schema_migrations WHERE version = '067_synthesis_causal_event_truth.sql');
")"
  IFS='|' read -r total_runs total_outputs total_attempts total_events \
    legacy_attempts linked_attempts migration_count <<<"$totals"
  ((total_runs == PRIOR_RUN_COUNT + 1)) || fail "candidate did not retain prior runs while adding one causal run"
  ((total_outputs == PRIOR_OUTPUT_COUNT + 1)) || fail "candidate did not retain prior outputs while adding one causal output"
  ((total_attempts == PRIOR_ATTEMPT_COUNT + 1)) || fail "candidate did not retain prior attempts while adding one causal attempt"
  [[ "$legacy_attempts" == "$PRIOR_LEGACY_ATTEMPT_COUNT" ]] \
    || fail "candidate rewrote prior legacy attempt causality"
  [[ "$linked_attempts" == "1" ]] || fail "candidate did not append exactly one linked attempt"
  ((total_events >= 2)) || fail "candidate did not append start and terminal causal events"
  [[ "$migration_count" == "1" ]] || fail "candidate lost migration 067 ledger truth"
  assert_prior_rows_retained

  echo "WRITE: revision=candidate output_id=$output_id attempt_no=$attempt_no started_events=1 terminal_events=1"
  echo "RETAIN: prior_runs=$PRIOR_RUN_COUNT prior_outputs=$PRIOR_OUTPUT_COUNT prior_attempts=$PRIOR_ATTEMPT_COUNT migration_067=1"
}

for tool in git docker jq awk mktemp openssl; do
  require_tool "$tool"
done
: "${SMACKEREL_HARDWARE_TIER:?SMACKEREL_HARDWARE_TIER is required for each revision config generator}"
: "${SMACKEREL_OLLAMA_URL:?SMACKEREL_OLLAMA_URL is required for each revision config generator}"

if [[ "${SMACKEREL_E2E_CHILD_RUN_ID+set}" == "set" && -n "$SMACKEREL_E2E_CHILD_RUN_ID" ]]; then
  CHILD_RUN_ID="$SMACKEREL_E2E_CHILD_RUN_ID"
else
  CHILD_RUN_ID="$RUN_PREFIX"
fi

if ! git -C "$REPO_DIR" cat-file -e "${PINNED_SOURCE_SHA}^{commit}"; then
  fail "pinned source commit is absent from the local Git object database"
fi
CURRENT_REVISION_LABEL="$(git -C "$REPO_DIR" rev-parse HEAD)"
[[ "$CURRENT_REVISION_LABEL" =~ ^[0-9a-f]{40}$ ]] || fail "current source SHA is not a full commit identity"
if [[ -n "$(git -C "$REPO_DIR" status --porcelain)" ]]; then
  CURRENT_REVISION_LABEL="${CURRENT_REVISION_LABEL}-dirty"
fi

git -C "$REPO_DIR" worktree add --detach "$PRIOR_WORKTREE" "$PINNED_SOURCE_SHA"
PRIOR_WORKTREE_ADDED=1

run_revision_cli "$PRIOR_WORKTREE" "$PINNED_SOURCE_SHA" "$PRIOR_WORKTREE/smackerel.sh" --env test config generate
run_revision_cli "$REPO_DIR" "$CURRENT_REVISION_LABEL" "$REPO_DIR/smackerel.sh" --env test config generate
PRIOR_ENV_FILE="$PRIOR_WORKTREE/config/generated/test.env"
CURRENT_ENV_FILE="$REPO_DIR/config/generated/test.env"
[[ -r "$PRIOR_ENV_FILE" && -r "$CURRENT_ENV_FILE" ]] || fail "revision-generated test environment file is unreadable"

PRIOR_COMPOSE_PROJECT="$(env_value "$PRIOR_ENV_FILE" "COMPOSE_PROJECT")"
CURRENT_COMPOSE_PROJECT="$(env_value "$CURRENT_ENV_FILE" "COMPOSE_PROJECT")"
[[ "$PRIOR_COMPOSE_PROJECT" == "$CURRENT_COMPOSE_PROJECT" ]] \
  || fail "prior and candidate build tags do not share the test compose project"
FIXED_BUILD_TAG="${CURRENT_COMPOSE_PROJECT}-smackerel-core:latest"
FIXED_ML_BUILD_TAG="${CURRENT_COMPOSE_PROJECT}-smackerel-ml:latest"

for key in POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB POSTGRES_CONTAINER_PORT CORE_CONTAINER_PORT; do
  prior_value="$(env_value "$PRIOR_ENV_FILE" "$key")"
  current_value="$(env_value "$CURRENT_ENV_FILE" "$key")"
  [[ "$prior_value" == "$current_value" ]] || fail "prior and candidate disagree on required test key: $key"
done
POSTGRES_USER="$(env_value "$CURRENT_ENV_FILE" "POSTGRES_USER")"
POSTGRES_PASSWORD="$(env_value "$CURRENT_ENV_FILE" "POSTGRES_PASSWORD")"
POSTGRES_DB="$(env_value "$CURRENT_ENV_FILE" "POSTGRES_DB")"
POSTGRES_CONTAINER_PORT="$(env_value "$CURRENT_ENV_FILE" "POSTGRES_CONTAINER_PORT")"
CORE_CONTAINER_PORT="$(env_value "$CURRENT_ENV_FILE" "CORE_CONTAINER_PORT")"
AUTH_TOKEN="$(openssl rand -hex 24)"
[[ ${#AUTH_TOKEN} -eq 48 ]] || fail "disposable authentication token generation failed"
printf '%s\n' "$AUTH_TOKEN" >"$AUTH_TOKEN_FILE"
chmod 0600 "$AUTH_TOKEN_FILE"

PRIOR_DOCKERFILE_OID="$(git -C "$REPO_DIR" rev-parse "${PINNED_SOURCE_SHA}:Dockerfile")"
PRIOR_GO_SUM_OID="$(git -C "$REPO_DIR" rev-parse "${PINNED_SOURCE_SHA}:go.sum")"
PRIOR_ML_DOCKERFILE_OID="$(git -C "$REPO_DIR" rev-parse "${PINNED_SOURCE_SHA}:ml/Dockerfile")"
PRIOR_ML_REQUIREMENTS_OID="$(git -C "$REPO_DIR" rev-parse "${PINNED_SOURCE_SHA}:ml/requirements.txt")"
CURRENT_DOCKERFILE_OID="$(git -C "$REPO_DIR" hash-object "$REPO_DIR/Dockerfile")"
CURRENT_GO_SUM_OID="$(git -C "$REPO_DIR" hash-object "$REPO_DIR/go.sum")"
CURRENT_ML_DOCKERFILE_OID="$(git -C "$REPO_DIR" hash-object "$REPO_DIR/ml/Dockerfile")"
CURRENT_ML_REQUIREMENTS_OID="$(git -C "$REPO_DIR" hash-object "$REPO_DIR/ml/requirements.txt")"
PRIOR_RESOLVED_FROM_LINES="$(resolved_from_lines "$PRIOR_WORKTREE/Dockerfile" "$PRIOR_WORKTREE/ml/Dockerfile")"
CURRENT_RESOLVED_FROM_LINES="$(resolved_from_lines "$REPO_DIR/Dockerfile" "$REPO_DIR/ml/Dockerfile")"
echo "BUILD_INPUT: revision=prior dockerfile_oid=$PRIOR_DOCKERFILE_OID go_sum_oid=$PRIOR_GO_SUM_OID ml_dockerfile_oid=$PRIOR_ML_DOCKERFILE_OID ml_requirements_oid=$PRIOR_ML_REQUIREMENTS_OID from=$PRIOR_RESOLVED_FROM_LINES"
echo "BUILD_INPUT: revision=candidate dockerfile_oid=$CURRENT_DOCKERFILE_OID go_sum_oid=$CURRENT_GO_SUM_OID ml_dockerfile_oid=$CURRENT_ML_DOCKERFILE_OID ml_requirements_oid=$CURRENT_ML_REQUIREMENTS_OID from=$CURRENT_RESOLVED_FROM_LINES"

snapshot_fixed_build_tags
run_revision_cli "$PRIOR_WORKTREE" "$PINNED_SOURCE_SHA" "$PRIOR_WORKTREE/smackerel.sh" --env test build
capture_prior_images
run_revision_cli "$REPO_DIR" "$CURRENT_REVISION_LABEL" "$REPO_DIR/smackerel.sh" --env test build
capture_current_images

POSTGRES_IMAGE="$(compose_service_image "$REPO_DIR/docker-compose.yml" "postgres")"
NATS_IMAGE="$(compose_service_image "$REPO_DIR/docker-compose.yml" "nats")"
prepare_runtime_mounts
create_infrastructure
wait_for_postgres

run_core "$CURRENT_CORE_IMAGE_ID" "$CURRENT_ML_IMAGE_ID" "$CURRENT_ENV_FILE" "$REPO_DIR" "migration"
wait_for_core "migration"
assert_migration_067
stop_active_core
reset_synthesis_state

run_core "$PRIOR_CORE_IMAGE_ID" "$PRIOR_ML_IMAGE_ID" "$PRIOR_ENV_FILE" "$PRIOR_WORKTREE" "prior"
wait_for_core "prior"
assert_authenticated_prior_read
prior_write_or_fail_closed "daily"
assert_prior_zero_causal_history
prior_write_or_fail_closed "weekly"
assert_prior_zero_causal_history
capture_prior_persistence_snapshot
stop_active_core

seed_candidate_source_set
run_core "$CURRENT_CORE_IMAGE_ID" "$CURRENT_ML_IMAGE_ID" "$CURRENT_ENV_FILE" "$REPO_DIR" "current"
wait_for_core "current"
assert_candidate_causal_write
stop_active_core

TEST_PASS_READY=1
