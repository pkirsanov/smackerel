#!/usr/bin/env bash
# This contract intentionally matches and mutates unexpanded shell source text.
# shellcheck disable=SC2016
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
DISPATCH="$REPO_ROOT/smackerel.sh"
E2E_RUNNER="$REPO_ROOT/tests/e2e/run_all.sh"
RESTART_DURABILITY="$REPO_ROOT/tests/e2e/synthesis_restart_durability_e2e_test.sh"
PRIOR_SOURCE_LIFECYCLE="$REPO_ROOT/tests/e2e/synthesis_prior_source_compatibility_e2e_test.sh"
HTTP_PROBE="$REPO_ROOT/tests/e2e/lib/synthesis_http_probe.py"
HTTP_PROBE_UNIT="$REPO_ROOT/tests/e2e/lib/test_synthesis_http_probe.py"
PY_UNIT="$REPO_ROOT/scripts/runtime/python-unit.sh"
PY_INTEGRATION="$REPO_ROOT/scripts/runtime/python-integration.sh"
GO_E2E="$REPO_ROOT/scripts/runtime/go-e2e.sh"
DEADLETTER_TEST="$REPO_ROOT/ml/tests/integration/test_deadletter_parity.py"
PINNED_SOURCE_SHA="7c3838e3b2de9ecba2e6a7764493a0412c4ed268"
LIFECYCLE_BASENAME="synthesis_prior_source_compatibility_e2e_test"
LIFECYCLE_FILENAME="${LIFECYCLE_BASENAME}.sh"
TEST_TITLE="prior-source lifecycle pins the full SHA, forbids fetch, records digest and inputs, owns unique refs, registers lifecycle required, and cleans every resource"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

contract_error() {
  echo "CONTRACT-REJECTED: $*" >&2
  return 1
}

require_literal() {
  local file="${1:?file required}"
  local literal="${2:?literal required}"
  local message="${3:?message required}"

  grep -qF "$literal" "$file" || contract_error "$message"
}

require_declaration_member() {
  local file="${1:?file required}"
  local declaration="${2:?declaration required}"
  local member="${3:?member required}"

  awk -v declaration="$declaration" -v member="$member" '
    index($0, declaration) == 1 && index($0, member) > 0 { found = 1 }
    END { exit(found ? 0 : 1) }
  ' "$file" || contract_error "$declaration no longer contains $member"
}

require_block_member() {
  local file="${1:?file required}"
  local block_start="${2:?block start required}"
  local block_end="${3:?block end required}"
  local member="${4:?member required}"

  awk -v block_start="$block_start" -v block_end="$block_end" -v member="$member" '
    index($0, block_start) > 0 { in_block = 1 }
    in_block && index($0, member) > 0 { found = 1 }
    in_block && index($0, block_end) > 0 && index($0, block_start) == 0 { exit(found ? 0 : 1) }
    END { if (in_block) exit(found ? 0 : 1); exit 1 }
  ' "$file" || contract_error "$block_start no longer contains $member"
}

reject_docker_run_publication() {
  local file="${1:?file required}"

  if awk '
    /docker run([[:space:]]|$)/ { in_docker_run = 1 }
    in_docker_run && /(^|[[:space:]])-p([[:space:]]|$)|(^|[[:space:]])--publish([=[:space:]]|$)/ { found = 1 }
    in_docker_run && $0 !~ /\\[[:space:]]*$/ { in_docker_run = 0 }
    END { exit(found ? 0 : 1) }
  ' "$file"; then
    contract_error "lifecycle docker run publishes a host port"
    return 1
  fi
}

assert_prior_source_contract() {
  local lifecycle="${1:?lifecycle script required}"
  local e2e_runner="${2:?E2E runner required}"
  local dispatch="${3:?dispatcher required}"
  local http_probe="${4:?HTTP probe required}"
  local status=0

  if [[ ! -f "$lifecycle" ]]; then
    contract_error "prior-source lifecycle script is missing"
    status=1
  fi
  if ! bash -n "$lifecycle"; then
    contract_error "prior-source lifecycle script fails bash -n"
    status=1
  fi
  if [[ ! -f "$http_probe" ]]; then
    contract_error "same-network HTTP probe helper is missing"
    status=1
  fi

  require_literal "$http_probe" 'from urllib import error, parse, request' \
    "HTTP probe no longer uses the Python standard-library transport" || status=1
  require_literal "$http_probe" 'except error.HTTPError as exc:' \
    "HTTP probe no longer preserves HTTP error responses" || status=1
  require_literal "$http_probe" '"status": status, "body": body' \
    "HTTP probe no longer emits the status/body JSON contract" || status=1
  require_literal "$http_probe" 'emit_result(0, "invalid probe input")' \
    "HTTP probe no longer emits the safe input-failure envelope" || status=1
  require_literal "$http_probe" 'emit_result(0, "HTTP transport failed")' \
    "HTTP probe no longer emits the safe transport-failure envelope" || status=1
  require_literal "$http_probe" 'emit_result(0, "HTTP probe failed")' \
    "HTTP probe no longer emits the safe generic-failure envelope" || status=1
  require_literal "$http_probe" 'parser.add_argument("--method", required=True' \
    "HTTP probe method is no longer an explicit required input" || status=1
  require_literal "$http_probe" 'parser.add_argument("--url", required=True)' \
    "HTTP probe URL is no longer an explicit required input" || status=1
  require_literal "$http_probe" 'parser.add_argument("--body", required=True)' \
    "HTTP probe body is no longer an explicit required input" || status=1
  require_literal "$http_probe" 'parser.add_argument("--timeout-seconds", required=True' \
    "HTTP probe timeout is no longer explicit and bounded" || status=1
  if grep -Eq '^(from|import)[[:space:]]+(aiohttp|httpx|requests)([[:space:].]|$)' "$http_probe"; then
    contract_error "HTTP probe imports a non-standard HTTP client"
    status=1
  fi

  require_literal "$lifecycle" "PINNED_SOURCE_SHA=\"$PINNED_SOURCE_SHA\"" \
    "pinned source SHA changed or is not the exact 40-character commit" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'git -C "$REPO_DIR" cat-file -e "${PINNED_SOURCE_SHA}^{commit}"' \
    "local Git object precondition is missing" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'git -C "$REPO_DIR" worktree add --detach "$PRIOR_WORKTREE" "$PINNED_SOURCE_SHA"' \
    "detached exact-SHA worktree creation is missing" || status=1
  if grep -Eq '(^|[;&|[:space:]])git([[:space:]]+-C[[:space:]]+[^[:space:]]+)?[[:space:]]+(fetch|pull|clone|checkout)([[:space:]]|$)' "$lifecycle"; then
    contract_error "prior-source lifecycle contains a forbidden source-network or checkout command"
    status=1
  fi

  require_literal "$lifecycle" 'run_revision_cli() {' \
    "revision-root CLI helper is missing" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" '    cd "$repository_root"' \
    "revision-root CLI helper no longer changes to the selected repository root" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" '    SMACKEREL_COMMIT="$revision" "$@"' \
    "revision-root CLI helper no longer invokes the selected revision command" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'run_revision_cli "$PRIOR_WORKTREE" "$PINNED_SOURCE_SHA" "$PRIOR_WORKTREE/smackerel.sh" --env test config generate' \
    "pinned revision config generation no longer runs from its own repository root" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'run_revision_cli "$REPO_DIR" "$CURRENT_REVISION_LABEL" "$REPO_DIR/smackerel.sh" --env test config generate' \
    "current candidate config generation no longer runs from the current repository root" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'run_revision_cli "$PRIOR_WORKTREE" "$PINNED_SOURCE_SHA" "$PRIOR_WORKTREE/smackerel.sh" --env test build' \
    "pinned revision build no longer runs from its own repository root" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'run_revision_cli "$REPO_DIR" "$CURRENT_REVISION_LABEL" "$REPO_DIR/smackerel.sh" --env test build' \
    "current candidate build no longer runs from the current repository root" || status=1

  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" '"$PRIOR_WORKTREE/smackerel.sh" --env test build' \
    "pinned revision is no longer built through its own repo CLI" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" '"$REPO_DIR/smackerel.sh" --env test build' \
    "current candidate is no longer built through the current repo CLI" || status=1
  require_literal "$lifecycle" 'PRIOR_DOCKERFILE_OID=' \
    "pinned Dockerfile Git object recording is missing" || status=1
  require_literal "$lifecycle" 'PRIOR_GO_SUM_OID=' \
    "pinned go.sum Git object recording is missing" || status=1
  require_literal "$lifecycle" 'CURRENT_DOCKERFILE_OID=' \
    "candidate Dockerfile Git object recording is missing" || status=1
  require_literal "$lifecycle" 'CURRENT_GO_SUM_OID=' \
    "candidate go.sum Git object recording is missing" || status=1
  require_literal "$lifecycle" 'RESOLVED_FROM_LINES' \
    "resolved Dockerfile FROM input recording is missing" || status=1
  require_literal "$lifecycle" 'org.opencontainers.image.revision' \
    "OCI revision-label inspection is missing" || status=1
  require_literal "$lifecycle" "'{{.Id}}'" \
    "final immutable local image-ID inspection is missing" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'run_core "$PRIOR_CORE_IMAGE_ID"' \
    "pinned runtime no longer starts by immutable image ID" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'run_core "$CURRENT_CORE_IMAGE_ID"' \
    "candidate runtime no longer starts by immutable image ID" || status=1

  require_literal "$lifecycle" 'docker network create --internal' \
    "runtime network is no longer Docker internal" || status=1
  reject_docker_run_publication "$lifecycle" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'published_port_count="$(docker container inspect --format '\''{{len .HostConfig.PortBindings}}'\'' "$ACTIVE_CORE_CONTAINER")"' \
    "core zero-publication runtime assertion is missing" || status=1
  require_literal "$lifecycle" '[[ "$published_port_count" == "0" ]]' \
    "core zero-publication result is no longer enforced" || status=1
  require_literal "$lifecycle" 'CORE_URL="http://smackerel-core:${CORE_CONTAINER_PORT}"' \
    "API target no longer uses the core alias and container port" || status=1
  require_block_member "$lifecycle" 'run_core() {' 'stop_active_core() {' '-e "ASSISTANT_ENABLED=false"' || status=1
  require_block_member "$lifecycle" 'run_core() {' 'stop_active_core() {' '-e "ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED=false"' || status=1
  require_block_member "$lifecycle" 'run_core() {' 'stop_active_core() {' '-e "ASSISTANT_TRANSPORTS_TELEGRAM_MODE=long_poll"' || status=1
  require_block_member "$lifecycle" 'run_core() {' 'stop_active_core() {' '-e "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF="' || status=1
  require_block_member "$lifecycle" 'run_core() {' 'stop_active_core() {' '-e "ASSISTANT_TELEGRAM_WEBHOOK_SECRET="' || status=1
  require_block_member "$lifecycle" 'run_core() {' 'stop_active_core() {' '-e "ASSISTANT_INTENT_COMPILER_ENABLED=false"' || status=1
  require_block_member "$lifecycle" 'run_core() {' 'stop_active_core() {' '-e "QF_DECISIONS_ENABLED=false"' || status=1
  require_literal "$lifecycle" 'HTTP_PROBE_HELPER="$SCRIPT_DIR/lib/synthesis_http_probe.py"' \
    "checked-in same-network HTTP probe path is missing" || status=1
  require_literal "$lifecycle" 'AUTH_TOKEN_FILE="$TEMP_ROOT/synthesis-auth-token"' \
    "chmod-0600 disposable token-file path is missing" || status=1
  require_literal "$lifecycle" 'chmod 0600 "$AUTH_TOKEN_FILE"' \
    "disposable token file is not restricted to mode 0600" || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '[[ $# -eq 5 ]]' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' 'if probe_result="$(smackerel_run_with_timeout --kill-after=5s "$container_timeout" docker run --rm' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '--pull=never' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '--name "$probe_name"' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '--network "$NETWORK_NAME"' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '--label "com.smackerel.test.run-id=$RUN_PREFIX"' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '--label "com.smackerel.e2e-child-run-id=$CHILD_RUN_ID"' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '--label "com.smackerel.test.role=synthesis-http-probe"' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '--entrypoint python3' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '-v "$HTTP_PROBE_HELPER:/probe/synthesis_http_probe.py:ro"' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '-v "$AUTH_TOKEN_FILE:/run/secrets/synthesis-auth-token:ro"' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '"$ACTIVE_ML_IMAGE_ID"' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '--auth-mode "$auth_mode"' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '--auth-token-file /run/secrets/synthesis-auth-token' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '--timeout-seconds "$request_timeout"' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' 'probe_exit=$?' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' '0 | 2 | 3 | 4)' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' 'if [[ "$probe_status" != "0" || "$probe_body" != "$expected_failure_body" ]]' || status=1
  require_block_member "$lifecycle" 'run_http_probe() {' 'try_api_get() {' 'return "$probe_exit"' || status=1
  require_literal "$lifecycle" 'probe_name="${RUN_PREFIX}-http-probe-${PROBE_SEQUENCE}"' \
    "HTTP probe container name is not unique per call" || status=1
  require_block_member "$lifecycle" 'run_core() {' 'stop_active_core() {' 'ACTIVE_ENV_FILE="$env_file"' || status=1
  require_block_member "$lifecycle" 'stop_active_core() {' '# BEGIN C21 STARTUP LOG REDACTOR' 'ACTIVE_ENV_FILE=""' || status=1
  require_block_member "$lifecycle" 'redact_startup_logs() {' 'emit_stopped_core_diagnostics() {' 'input_kind=auth "$auth_token_file"' || status=1
  require_block_member "$lifecycle" 'redact_startup_logs() {' 'emit_stopped_core_diagnostics() {' 'input_kind=env "$env_file"' || status=1
  require_block_member "$lifecycle" 'redact_startup_logs() {' 'emit_stopped_core_diagnostics() {' 'line ~ /^[[:space:]]*(#|$)/' || status=1
  require_block_member "$lifecycle" 'redact_startup_logs() {' 'emit_stopped_core_diagnostics() {' 'length(value) > 0' || status=1
  require_block_member "$lifecycle" 'redact_startup_logs() {' 'emit_stopped_core_diagnostics() {' 'upper_key ~ /(TOKEN|KEY|PASSWORD|SECRET|CREDENTIAL|PRIVATE)/' || status=1
  require_block_member "$lifecycle" 'redact_startup_logs() {' 'emit_stopped_core_diagnostics() {' 'upper_key == "DATABASE_URL"' || status=1
  require_block_member "$lifecycle" 'redact_startup_logs() {' 'emit_stopped_core_diagnostics() {' 'upper_key == "POSTGRES_PASSWORD"' || status=1
  require_block_member "$lifecycle" 'redact_startup_logs() {' 'emit_stopped_core_diagnostics() {' 'redact_literal(line, secrets[secret_index])' || status=1
  require_block_member "$lifecycle" 'emit_stopped_core_diagnostics() {' 'run_http_probe() {' 'SAFE-DIAGNOSTIC: core-startup-failure phase=%s exit_code=%s' || status=1
  require_block_member "$lifecycle" 'emit_stopped_core_diagnostics() {' 'run_http_probe() {' 'docker logs "$ACTIVE_CORE_CONTAINER" 2>&1 | redact_startup_logs "$AUTH_TOKEN_FILE" "$ACTIVE_ENV_FILE" >"$redacted_log_file"' || status=1
  require_block_member "$lifecycle" 'emit_stopped_core_diagnostics() {' 'run_http_probe() {' 'cat "$redacted_log_file" >&2' || status=1
  require_block_member "$lifecycle" 'emit_stopped_core_diagnostics() {' 'run_http_probe() {' 'docker_logs=unavailable reason=retrieval-failed' || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'run_core "$PRIOR_CORE_IMAGE_ID" "$PRIOR_ML_IMAGE_ID"' \
    "prior runtime no longer selects the corresponding immutable ML image" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'run_core "$CURRENT_CORE_IMAGE_ID" "$CURRENT_ML_IMAGE_ID"' \
    "candidate runtime no longer selects the corresponding immutable ML image" || status=1
  require_literal "$lifecycle" 'run_http_probe "GET" "$CORE_URL$path" "" "bearer" "30"' \
    "authenticated GET no longer uses the bearer probe contract" || status=1
  require_literal "$lifecycle" 'run_http_probe "GET" "$CORE_URL$path" "" "none" "30"' \
    "unauthenticated GET no longer uses the explicit no-auth probe contract" || status=1
  require_literal "$lifecycle" 'run_http_probe "POST" "$CORE_URL/api/synthesis/retry" "$request_body" "bearer" "60"' \
    "authenticated retry no longer uses the bearer probe contract" || status=1
  require_block_member "$lifecycle" 'parse_output_presence() {' 'assert_authenticated_prior_read() {' 'if type == "object" then (has("output") | tostring) else error("expected object") end' || status=1
  require_block_member "$lifecycle" 'assert_authenticated_prior_read() {' 'assert_prior_output_readable() {' 'has_output="$(parse_output_presence <<<"$HTTP_BODY")"' || status=1
  require_block_member "$lifecycle" 'assert_authenticated_prior_read() {' 'assert_prior_output_readable() {' '[[ "$state" == "never-run" && "$has_output" == "false" ]]' || status=1
  require_literal "$lifecycle" '# BEGIN C21 CANDIDATE FAILURE SNAPSHOT' \
    "candidate-write failure snapshot source block is missing" || status=1
  require_literal "$lifecycle" '# END C21 CANDIDATE FAILURE SNAPSHOT' \
    "candidate-write failure snapshot source block is unterminated" || status=1
  require_block_member "$lifecycle" 'safe_candidate_error_code() {' 'is_synthesis_run_state() {' '[[ "$candidate" =~ ^[a-z0-9_]+$ ]]' || status=1
  require_block_member "$lifecycle" 'emit_candidate_write_failure_snapshot() {' '# END C21 CANDIDATE FAILURE SNAPSHOT' 'SELECT state, lifecycle_state, COUNT(*)' || status=1
  require_block_member "$lifecycle" 'emit_candidate_write_failure_snapshot() {' '# END C21 CANDIDATE FAILURE SNAPSHOT' "SELECT COALESCE(state, 'legacy_unlinked')," || status=1
  require_block_member "$lifecycle" 'emit_candidate_write_failure_snapshot() {' '# END C21 CANDIDATE FAILURE SNAPSHOT' "COALESCE(failure_code, 'none')," || status=1
  require_block_member "$lifecycle" 'emit_candidate_write_failure_snapshot() {' '# END C21 CANDIDATE FAILURE SNAPSHOT' "SELECT event_type, COALESCE(failure_code, 'none'), COUNT(*)" || status=1
  require_block_member "$lifecycle" 'emit_candidate_write_failure_snapshot() {' '# END C21 CANDIDATE FAILURE SNAPSHOT' 'scope=configured_actor_daily_current_window count=%s' || status=1
  require_block_member "$lifecycle" 'emit_candidate_write_failure_snapshot() {' '# END C21 CANDIDATE FAILURE SNAPSHOT' '2>/dev/null' || status=1
  require_block_member "$lifecycle" 'assert_candidate_causal_write() {' 'for tool in' 'if [[ "$HTTP_STATUS" != "200" ]]; then' || status=1
  require_block_member "$lifecycle" 'assert_candidate_causal_write() {' 'for tool in' 'error_code="$(safe_candidate_error_code "$HTTP_BODY")"' || status=1
  require_block_member "$lifecycle" 'assert_candidate_causal_write() {' 'for tool in' 'SAFE-DIAGNOSTIC: candidate-write-failure http_status=%s error_code=%s' || status=1
  require_block_member "$lifecycle" 'assert_candidate_causal_write() {' 'for tool in' 'emit_candidate_write_failure_snapshot "$expected_actor"' || status=1
  require_block_member "$lifecycle" 'assert_candidate_causal_write() {' 'for tool in' 'fail "candidate strict causal write failed with HTTP $HTTP_STATUS"' || status=1
  require_block_member "$lifecycle" 'try_api_get() {' 'api_get() {' 'run_http_probe "GET" "$CORE_URL$path" "" "bearer" "30"' || status=1
  require_block_member "$lifecycle" 'api_get() {' 'api_get_without_auth() {' 'run_http_probe "GET" "$CORE_URL$path" "" "bearer" "30" || probe_status=$?' || status=1
  require_block_member "$lifecycle" 'api_get() {' 'api_get_without_auth() {' 'fail "authenticated GET HTTP probe transport/input failure: status=$HTTP_STATUS reason=$HTTP_BODY"' || status=1
  require_block_member "$lifecycle" 'api_get_without_auth() {' 'api_retry() {' 'run_http_probe "GET" "$CORE_URL$path" "" "none" "30" || probe_status=$?' || status=1
  require_block_member "$lifecycle" 'api_get_without_auth() {' 'api_retry() {' 'fail "unauthenticated GET HTTP probe transport/input failure: status=$HTTP_STATUS reason=$HTTP_BODY"' || status=1
  require_block_member "$lifecycle" 'api_retry() {' 'wait_for_core() {' 'run_http_probe "POST" "$CORE_URL/api/synthesis/retry" "$request_body" "bearer" "60" || probe_status=$?' || status=1
  require_block_member "$lifecycle" 'api_retry() {' 'wait_for_core() {' 'fail "authenticated retry HTTP probe transport/input failure: status=$HTTP_STATUS reason=$HTTP_BODY"' || status=1
  require_block_member "$lifecycle" 'wait_for_core() {' 'assert_migration_067() {' 'for ((attempt = 1; attempt <= 30; attempt++))' || status=1
  require_block_member "$lifecycle" 'wait_for_core() {' 'assert_migration_067() {' 'if try_api_get "/api/health" && [[ "$HTTP_STATUS" == "200" ]]' || status=1
  require_block_member "$lifecycle" 'wait_for_core() {' 'assert_migration_067() {' 'sleep 2' || status=1
  require_block_member "$lifecycle" 'wait_for_core() {' 'assert_migration_067() {' 'last_probe_status=$HTTP_STATUS reason=$last_probe_reason' || status=1
  require_block_member "$lifecycle" 'wait_for_core() {' 'assert_migration_067() {' 'core_exit_code="$(docker container inspect --format '\''{{.State.ExitCode}}'\'' "$ACTIVE_CORE_CONTAINER" 2>/dev/null)"' || status=1
  require_block_member "$lifecycle" 'wait_for_core() {' 'assert_migration_067() {' '[[ "$core_exit_code" =~ ^[0-9]+$ ]]' || status=1
  require_block_member "$lifecycle" 'wait_for_core() {' 'assert_migration_067() {' 'emit_stopped_core_diagnostics "$phase" "$core_exit_code"' || status=1
  require_literal "$lifecycle" 'com.smackerel.e2e-child-run-id' \
    "parent-reapable Docker child label is missing" || status=1
  require_literal "$lifecycle" 'com.smackerel.test.run-id' \
    "per-run resource label is missing" || status=1
  require_literal "$lifecycle" 'RUN_PREFIX=' \
    "unique per-run resource prefix is missing" || status=1
  require_literal "$lifecycle" 'FIXED_BUILD_TAG=' \
    "fixed Compose-generated build tag snapshot is missing" || status=1
  require_literal "$lifecycle" 'restore_fixed_build_tag' \
    "fixed Compose-generated build tag restore is missing" || status=1
  require_literal "$lifecycle" 'trap '\''cleanup "$?"'\'' EXIT' \
    "EXIT cleanup trap is missing" || status=1
  require_literal "$lifecycle" 'remove_owned_container' \
    "owned-container cleanup is missing" || status=1
  require_literal "$lifecycle" 'remove_owned_probe_containers' \
    "owned probe-container cleanup is missing" || status=1
  require_block_member "$lifecycle" 'cleanup() {' 'trap '\''cleanup "$?"'\'' EXIT' 'remove_owned_probe_containers' || status=1
  require_literal "$lifecycle" 'remove_owned_volume' \
    "owned-volume cleanup is missing" || status=1
  require_literal "$lifecycle" 'remove_owned_network' \
    "owned-network cleanup is missing" || status=1
  require_literal "$lifecycle" 'remove_owned_image_ref' \
    "owned-image cleanup is missing" || status=1
  # Match literal lifecycle source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$lifecycle" 'git -C "$REPO_DIR" worktree remove --force "$PRIOR_WORKTREE"' \
    "detached-worktree cleanup is missing" || status=1
  require_literal "$lifecycle" 'assert_no_owned_resources' \
    "post-cleanup owned-resource assertion is missing" || status=1

  # Match literal runner source rather than expanding its pattern default.
  # shellcheck disable=SC2016
  require_literal "$e2e_runner" 'PATTERN="${1:-*.sh}"' \
    "default E2E discovery no longer includes every top-level shell test" || status=1
  # Match literal runner source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$e2e_runner" 'for TEST_FILE in "$SCRIPT_DIR"/$PATTERN; do' \
    "default E2E discovery is no longer limited to top-level shell files" || status=1
  # Match literal runner source rather than expanding its runtime variables.
  # shellcheck disable=SC2016
  require_literal "$e2e_runner" '[[ "$TEST_NAME" == "run_all" ]] && continue' \
    "default E2E discovery no longer excludes runner recursion" || status=1
  require_declaration_member "$e2e_runner" 'LIFECYCLE_TESTS=' "$LIFECYCLE_BASENAME" || status=1
  require_declaration_member "$e2e_runner" 'REQUIRED_TESTS=' "$LIFECYCLE_BASENAME" || status=1
  require_block_member "$dispatch" 'e2e_shell_timeout_for()' 'e2e_shell_test_manages_stack()' "$LIFECYCLE_FILENAME" || status=1
  require_block_member "$dispatch" 'e2e_shell_test_manages_stack()' 'e2e_print_test_stack_state()' "$LIFECYCLE_FILENAME" || status=1
  require_block_member "$dispatch" 'e2e_required_shell_tests=(' 'e2e_shell_test_is_required()' "$LIFECYCLE_FILENAME" || status=1
  require_block_member "$dispatch" 'e2e_lifecycle_scripts=(' 'for e2e_script in' "$LIFECYCLE_FILENAME" || status=1

  return "$status"
}

mutate_file() {
  local expression="${1:?sed expression required}"
  local file="${2:?file required}"

  sed -i.bak -e "$expression" "$file"
  rm -f "$file.bak"
}

expect_contract_rejection() {
  local label="${1:?adversarial label required}"
  local mutation="${2:?mutation required}"
  local target="${3:?mutation target required}"
  local temp_root lifecycle_copy runner_copy dispatch_copy

  temp_root="$(mktemp -d)"
  lifecycle_copy="$temp_root/lifecycle.sh"
  runner_copy="$temp_root/run_all.sh"
  dispatch_copy="$temp_root/smackerel.sh"
  cp "$PRIOR_SOURCE_LIFECYCLE" "$lifecycle_copy"
  cp "$E2E_RUNNER" "$runner_copy"
  cp "$DISPATCH" "$dispatch_copy"

  case "$target" in
    lifecycle) mutate_file "$mutation" "$lifecycle_copy" ;;
    runner) mutate_file "$mutation" "$runner_copy" ;;
    dispatch) mutate_file "$mutation" "$dispatch_copy" ;;
    *)
      rm -rf "$temp_root"
      fail "unknown adversarial target: $target"
      ;;
  esac

  if assert_prior_source_contract "$lifecycle_copy" "$runner_copy" "$dispatch_copy" "$HTTP_PROBE"; then
    rm -rf "$temp_root"
    fail "adversarial mutation was accepted: $label"
  fi
  rm -rf "$temp_root"
  echo "PASS: adversarial contract rejects $label"
}

assert_restart_seed_fence_order() {
  local lifecycle="${1:?restart durability script required}"
  local scan_output

  if ! scan_output="$(awk '
    /^install_canonical_artifact_fence[[:space:]]/ {
      fence_active = 1
      next
    }
    /^remove_canonical_artifact_fence[[:space:]]/ {
      fence_active = 0
      next
    }
    /^reset_synthesis_state_and_seed_corpus$/ {
      reset_count++
      if (!fence_active) {
        printf "reset_%d_before_fence\n", reset_count
        invalid = 1
      }
    }
    END {
      if (reset_count != 2) {
        printf "unexpected_reset_count=%d\n", reset_count
        invalid = 1
      }
      exit(invalid ? 1 : 0)
    }
  ' "$lifecycle")"; then
    contract_error "restart durability reset/seed ordering is unsafe: $scan_output"
    return 1
  fi

  echo "PASS: every restart durability reset/seed call begins with the canonical artifact fence active"
}

assert_restart_seed_fence_negative_control() {
  local lifecycle="${1:?restart durability script required}"
  local temp_root mutated_lifecycle rejection

  temp_root="$(mktemp -d)"
  mutated_lifecycle="$temp_root/synthesis_restart_durability_e2e_test.sh"
  if ! awk '
    !inserted && $0 == "install_canonical_artifact_fence before_restart_retry" {
      print "reset_synthesis_state_and_seed_corpus"
      print
      inserted = 1
      next
    }
    inserted && !removed && $0 == "reset_synthesis_state_and_seed_corpus" {
      removed = 1
      next
    }
    { print }
    END { if (!inserted || !removed) exit 1 }
  ' "$lifecycle" >"$mutated_lifecycle"; then
    rm -rf "$temp_root"
    contract_error "could not construct the unfenced reset ordering negative control"
    return 1
  fi

  if rejection="$(assert_restart_seed_fence_order "$mutated_lifecycle" 2>&1)"; then
    rm -rf "$temp_root"
    contract_error "restart durability ordering contract accepted reset/seed before its fence"
    return 1
  fi
  rm -rf "$temp_root"
  if [[ "$rejection" != *"reset_1_before_fence"* ]]; then
    contract_error "restart durability ordering negative control failed for an unrelated reason"
    return 1
  fi

  echo "PASS: restart durability ordering contract rejects reset/seed before the initial fence"
}

assert_startup_redactor_behavior() {
  local lifecycle="${1:?lifecycle script required}"
  local temp_root auth_file env_file logs_file redactor_source diagnostic_source
  local actual expected diagnostic_output expected_diagnostic expected_retrieval_failure

  temp_root="$(mktemp -d)"
  auth_file="$temp_root/auth-token"
  env_file="$temp_root/generated.env"
  logs_file="$temp_root/core.log"

  redactor_source="$(awk '
    $0 == "# BEGIN C21 STARTUP LOG REDACTOR" { capture = 1; next }
    $0 == "# END C21 STARTUP LOG REDACTOR" { capture = 0 }
    capture { print }
    END { if (capture) exit 1 }
  ' "$lifecycle")" || {
    rm -rf "$temp_root"
    contract_error "startup log redactor source block is malformed"
    return 1
  }
  [[ "$redactor_source" == *'redact_startup_logs() {'* ]] || {
    rm -rf "$temp_root"
    contract_error "startup log redactor source block is missing"
    return 1
  }
  eval "$redactor_source"
  diagnostic_source="$(awk '
    $0 == "emit_stopped_core_diagnostics() {" { capture = 1 }
    $0 == "run_http_probe() {" { capture = 0 }
    capture { print }
  ' "$lifecycle")"
  [[ "$diagnostic_source" == *'emit_stopped_core_diagnostics() {'* ]] || {
    rm -rf "$temp_root"
    contract_error "startup diagnostic emitter source block is missing"
    return 1
  }
  eval "$diagnostic_source"

  printf '%s\n' 'auth-token-value-c21' >"$auth_file"
  printf '%s\n' \
    '# generated environment comment' \
    '' \
    'EMPTY_SECRET=' \
    'ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF=' \
    'ASSISTANT_TELEGRAM_WEBHOOK_SECRET=' \
    'VISIBLE_SETTING=visible-setting' \
    'service_token=token-value-c21' \
    'SIGNING_KEY=key-value-c21' \
    'DB_PASSWORD=password-value-c21' \
    'OAUTH_SECRET=secret-value-c21' \
    'REMOTE_CREDENTIAL=credential-value-c21' \
    'PRIVATE_CERT=private-value-c21' \
    'DATABASE_URL=database-url-value-c21' \
    'POSTGRES_PASSWORD=postgres-password-value-c21' \
    'QUOTED_SECRET="quoted-secret-value-c21"' >"$env_file"
  printf '%s\n' \
    'startup error: migration core exited before readiness' \
    'auth=auth-token-value-c21' \
    'token=token-value-c21' \
    'key=key-value-c21' \
    'password=password-value-c21' \
    'secret=secret-value-c21' \
    'credential=credential-value-c21' \
    'private=private-value-c21' \
    'database=database-url-value-c21' \
    'postgres=postgres-password-value-c21' \
    'quoted=quoted-secret-value-c21' \
    '' \
    'visible=visible-setting' \
    'empty-secret marker remains' \
    'empty-webhook-secret-ref marker remains' \
    'empty-webhook-secret marker remains' >"$logs_file"

  if ! actual="$(redact_startup_logs "$auth_file" "$env_file" <"$logs_file")"; then
    rm -rf "$temp_root"
    contract_error "startup log redactor rejected valid fixture input"
    return 1
  fi
  expected=$'startup error: migration core exited before readiness\nauth=[REDACTED]\ntoken=[REDACTED]\nkey=[REDACTED]\npassword=[REDACTED]\nsecret=[REDACTED]\ncredential=[REDACTED]\nprivate=[REDACTED]\ndatabase=[REDACTED]\npostgres=[REDACTED]\nquoted=[REDACTED]\n\nvisible=visible-setting\nempty-secret marker remains\nempty-webhook-secret-ref marker remains\nempty-webhook-secret marker remains'
  [[ "$actual" == "$expected" ]] || {
    rm -rf "$temp_root"
    contract_error "startup log redactor did not preserve complete non-secret diagnostics"
    return 1
  }

  # The evaluated diagnostic emitter consumes these lifecycle globals.
  # shellcheck disable=SC2034
  AUTH_TOKEN_FILE="$auth_file"
  # shellcheck disable=SC2034
  ACTIVE_ENV_FILE="$env_file"
  ACTIVE_CORE_CONTAINER="c21-unit-core"
  # shellcheck disable=SC2034
  TEMP_ROOT="$temp_root"
  # The evaluated diagnostic emitter invokes this test-owned Docker seam.
  # shellcheck disable=SC2317
  docker() {
    [[ "$1" == "logs" && "$2" == "$ACTIVE_CORE_CONTAINER" ]] || return 64
    cat "$logs_file"
  }
  if ! diagnostic_output="$(emit_stopped_core_diagnostics "prior" "17" 2>&1)"; then
    unset -f docker
    rm -rf "$temp_root"
    contract_error "startup diagnostic emitter rejected complete Docker logs"
    return 1
  fi
  expected_diagnostic="$(printf 'SAFE-DIAGNOSTIC: core-startup-failure phase=prior exit_code=17\n%s' "$expected")"
  [[ "$diagnostic_output" == "$expected_diagnostic" ]] || {
    unset -f docker
    rm -rf "$temp_root"
    contract_error "startup diagnostic emitter did not print the safe header and complete redacted logs"
    return 1
  }

  # shellcheck disable=SC2317
  docker() {
    return 42
  }
  if ! diagnostic_output="$(emit_stopped_core_diagnostics "current" "23" 2>&1)"; then
    unset -f docker
    rm -rf "$temp_root"
    contract_error "startup diagnostic emitter did not safely handle Docker log retrieval failure"
    return 1
  fi
  expected_retrieval_failure=$'SAFE-DIAGNOSTIC: core-startup-failure phase=current exit_code=23\nSAFE-DIAGNOSTIC: docker_logs=unavailable reason=retrieval-failed'
  unset -f docker
  rm -rf "$temp_root"
  [[ "$diagnostic_output" == "$expected_retrieval_failure" ]] || {
    contract_error "startup diagnostic emitter exposed or misclassified a Docker log retrieval failure"
    return 1
  }
  echo "PASS: startup log redactor classifies all secret keys, replaces AUTH_TOKEN, ignores empty values including Telegram webhook secrets, and preserves complete diagnostics"
}

assert_prior_read_output_presence_behavior() {
  local lifecycle="${1:?lifecycle script required}"
  local parser_source actual non_object_result malformed_result

  parser_source="$(awk '
    $0 == "parse_output_presence() {" { capture = 1 }
    $0 == "assert_authenticated_prior_read() {" { capture = 0 }
    capture { print }
    END { if (capture) exit 1 }
  ' "$lifecycle")" || {
    contract_error "output-presence parser source block is malformed"
    return 1
  }
  [[ "$parser_source" == *'parse_output_presence() {'* ]] || {
    contract_error "output-presence parser source block is missing"
    return 1
  }
  eval "$parser_source"

  if ! actual="$(parse_output_presence <<<'{"state":"never-run"}')"; then
    contract_error "output-presence parser rejected an object without output"
    return 1
  fi
  [[ "$actual" == "false" ]] || {
    contract_error "output-presence parser did not emit false for an object without output"
    return 1
  }
  if non_object_result="$(parse_output_presence <<<'[]' 2>&1)"; then
    contract_error "output-presence parser accepted a non-object body as $non_object_result"
    return 1
  fi
  if malformed_result="$(parse_output_presence <<<'{"state":' 2>&1)"; then
    contract_error "output-presence parser accepted malformed JSON as $malformed_result"
    return 1
  fi

  echo "PASS: prior-read output presence emits false for an object without output and rejects non-object or malformed bodies"
}

assert_candidate_failure_snapshot_behavior() {
  local lifecycle="${1:?lifecycle script required}"
  local snapshot_source actual snapshot_output failed_query_output invalid_output
  local private_actor forbidden unsafe_body
  local expected_snapshot expected_query_failure
  local -a unsafe_bodies

  snapshot_source="$(awk '
    $0 == "# BEGIN C21 CANDIDATE FAILURE SNAPSHOT" { capture = 1; next }
    $0 == "# END C21 CANDIDATE FAILURE SNAPSHOT" { capture = 0; found_end = 1 }
    capture { print }
    END { if (capture || !found_end) exit 1 }
  ' "$lifecycle")" || {
    contract_error "candidate-write failure snapshot source block is malformed"
    return 1
  }
  [[ "$snapshot_source" == *'safe_candidate_error_code() {'* &&
    "$snapshot_source" == *'emit_candidate_write_failure_snapshot() {'* ]] || {
    contract_error "candidate-write failure snapshot source block is incomplete"
    return 1
  }

  for forbidden in \
    'run_id=' 'output_id=' 'principal=' 'source_id=' 'source_name=' 'title=' \
    'sql=' 'sql_text=' 'raw_error=' 'timestamp=' 'auth=' 'url=' \
    'database_url=' 'password=' 'credential='; do
    if grep -qF "$forbidden" <<<"$snapshot_source"; then
      contract_error "candidate-write snapshot exposes forbidden field label: $forbidden"
      return 1
    fi
  done
  eval "$snapshot_source"

  actual="$(safe_candidate_error_code \
    '{"error":{"code":"audit_persistence_failed","message":"private-source-title-c21"},"outputId":"private-output-id-c21"}')"
  [[ "$actual" == "audit_persistence_failed" ]] || {
    contract_error "candidate failure parser rejected a safe closed error token"
    return 1
  }
  [[ "$actual" != *"private-source-title-c21"* && "$actual" != *"private-output-id-c21"* ]] || {
    contract_error "candidate failure parser exposed response content or identity"
    return 1
  }

  unsafe_bodies=(
    '{"error":{"code":"unsafe-code"}}'
    '{"error":{"code":"UPPER_CASE"}}'
    '{"error":{"code":"unsafe code"}}'
    '{"error":{"code":500}}'
    '{"error":{"message":"private-source-title-c21"}}'
    '{"error":{"code":"unterminated"}'
  )
  for unsafe_body in "${unsafe_bodies[@]}"; do
    actual="$(safe_candidate_error_code "$unsafe_body")"
    [[ "$actual" == "invalid_error_envelope" ]] || {
      contract_error "candidate failure parser accepted an unsafe error envelope"
      return 1
    }
  done

  private_actor="private-actor-identity-c21"
  # This test-owned seam returns only aggregate rows selected by the snapshot.
  # shellcheck disable=SC2317
  psql_rows() {
    case "$1" in
      *"FROM synthesis_run_events"*)
        printf '%s\n' 'attempt_started|none|1' 'failed|audit_persistence_failed|1'
        ;;
      *"FROM synthesis_run_attempts"*)
        printf '%s\n' 'failed|failed|audit_persistence_failed|1' 'legacy_unlinked|succeeded|none|2'
        ;;
      *"FROM synthesis_outputs"*)
        printf '%s\n' '0'
        ;;
      *"FROM synthesis_runs"*)
        printf '%s\n' 'failed|current|1' 'succeeded|superseded|2'
        ;;
      *)
        printf '%s\n' 'private-sql-error-c21' >&2
        return 64
        ;;
    esac
  }
  snapshot_output="$(emit_candidate_write_failure_snapshot "$private_actor" 2>&1)"
  expected_snapshot=$'SAFE-SNAPSHOT: group=synthesis_runs state=failed lifecycle_state=current count=1\nSAFE-SNAPSHOT: group=synthesis_runs state=succeeded lifecycle_state=superseded count=2\nSAFE-SNAPSHOT: group=synthesis_run_attempts state=failed outcome=failed failure_code=audit_persistence_failed count=1\nSAFE-SNAPSHOT: group=synthesis_run_attempts state=legacy_unlinked outcome=succeeded failure_code=none count=2\nSAFE-SNAPSHOT: group=synthesis_run_events event_type=attempt_started failure_code=none count=1\nSAFE-SNAPSHOT: group=synthesis_run_events event_type=failed failure_code=audit_persistence_failed count=1\nSAFE-SNAPSHOT: group=current_outputs scope=configured_actor_daily_current_window count=0'
  [[ "$snapshot_output" == "$expected_snapshot" ]] || {
    contract_error "candidate-write snapshot did not emit the exact content-free aggregate contract"
    return 1
  }
  [[ "$snapshot_output" != *"$private_actor"* ]] || {
    contract_error "candidate-write snapshot exposed the configured actor identity"
    return 1
  }

  # Raw database errors may carry SQL or corpus-adjacent values. The snapshot
  # must suppress them and emit only one named diagnostic per failed query.
  # shellcheck disable=SC2317
  psql_rows() {
    printf '%s\n' \
      'private SQL failure source_id=private-source-c21 title=private-title-c21 postgres://private-credentials' >&2
    return 42
  }
  failed_query_output="$(emit_candidate_write_failure_snapshot "$private_actor" 2>&1)"
  expected_query_failure=$'SAFE-DIAGNOSTIC: candidate-write-snapshot group=synthesis_runs status=query_failed\nSAFE-DIAGNOSTIC: candidate-write-snapshot group=synthesis_run_attempts status=query_failed\nSAFE-DIAGNOSTIC: candidate-write-snapshot group=synthesis_run_events status=query_failed\nSAFE-DIAGNOSTIC: candidate-write-snapshot group=current_outputs status=query_failed'
  [[ "$failed_query_output" == "$expected_query_failure" ]] || {
    contract_error "candidate-write snapshot did not fail queries through named content-free diagnostics"
    return 1
  }
  [[ "$failed_query_output" != *"private-source-c21"* &&
    "$failed_query_output" != *"private-title-c21"* &&
    "$failed_query_output" != *"postgres://private-credentials"* ]] || {
    contract_error "candidate-write snapshot exposed a raw database failure"
    return 1
  }

  # A pattern-safe but non-vocabulary value is still rejected. This proves the
  # aggregate emitters are allow-list based rather than generic string printers.
  # shellcheck disable=SC2317
  psql_rows() {
    case "$1" in
      *"FROM synthesis_run_events"*) printf '%s\n' 'private_source_title_c21|none|1' ;;
      *"FROM synthesis_run_attempts"*) printf '%s\n' 'failed|failed|private_source_title_c21|1' ;;
      *"FROM synthesis_outputs"*) printf '%s\n' '1' ;;
      *"FROM synthesis_runs"*) printf '%s\n' 'failed|current|1' ;;
    esac
  }
  invalid_output="$(emit_candidate_write_failure_snapshot "$private_actor" 2>&1)"
  [[ "$invalid_output" == *'group=synthesis_run_attempts status=invalid_aggregate'* &&
    "$invalid_output" == *'group=synthesis_run_events status=invalid_aggregate'* ]] || {
    contract_error "candidate-write snapshot accepted a token outside the closed aggregate vocabularies"
    return 1
  }
  [[ "$invalid_output" != *"private_source_title_c21"* ]] || {
    contract_error "candidate-write snapshot printed a rejected aggregate token"
    return 1
  }

  unset -f psql_rows
  echo "PASS: candidate-write failure parsing and snapshot output are closed-token, aggregate-only, content-free, and query-failure-safe"
}

grep -qF 'e2e_down_test_stack "before targeted shared-stack shell E2E"' "$DISPATCH" \
  || fail "targeted shared-stack E2E no longer pre-cleans before stack boot"
# Match literal source text rather than expanding the variables here.
# shellcheck disable=SC2016
grep -qF '"$SCRIPT_DIR/smackerel.sh" --env test up' "$DISPATCH" \
  || fail "targeted shared-stack E2E no longer boots the disposable test stack"
# Match literal source text rather than expanding the variables here.
# shellcheck disable=SC2016
grep -qF 'env E2E_STACK_MANAGED=1 bash "$SCRIPT_DIR/tests/e2e/$SHELL_E2E_RUN_TARGET"' "$DISPATCH" \
  || fail "targeted shared-stack E2E no longer marks the child as parent-managed"
if grep -qF 'test_synthesis.sh' "$DISPATCH"; then
  fail "test_synthesis.sh must not be classified as a lifecycle test"
fi

grep -qF 'not integration and not live_ollama' "$PY_UNIT" \
  || fail "Python unit lane no longer excludes live integration and external Ollama markers"
grep -qF 'pytest -q -m integration ml/tests/integration' "$PY_INTEGRATION" \
  || fail "Python integration lane no longer executes the required integration marker"
grep -qF 'python-integration.sh' "$DISPATCH" \
  || fail "canonical integration dispatcher no longer runs Python integration tests"
grep -qF 'go_test_args=(-p 1 -tags e2e' "$GO_E2E" \
  || fail "Go E2E packages no longer serialize access to shared disposable identities"

if grep -Eq 'pytest\.mark\.skipif|pytest\.skip\(' "$DEADLETTER_TEST"; then
  fail "required dead-letter integration test contains a skip path"
fi
grep -qF 'raise RuntimeError' "$DEADLETTER_TEST" \
  || fail "required dead-letter integration prerequisites no longer fail loud"

[[ -f "$HTTP_PROBE_UNIT" ]] || fail "HTTP probe unit contract is missing"
python3 "$HTTP_PROBE_UNIT" || fail "HTTP probe unit contract failed"

assert_restart_seed_fence_order "$RESTART_DURABILITY" \
  || fail "restart durability canonical artifact fence ordering"
assert_restart_seed_fence_negative_control "$RESTART_DURABILITY" \
  || fail "restart durability canonical artifact fence ordering negative control"
assert_prior_source_contract "$PRIOR_SOURCE_LIFECYCLE" "$E2E_RUNNER" "$DISPATCH" "$HTTP_PROBE" \
  || fail "$TEST_TITLE"
assert_startup_redactor_behavior "$PRIOR_SOURCE_LIFECYCLE" \
  || fail "startup failure diagnostic redaction behavior"
assert_prior_read_output_presence_behavior "$PRIOR_SOURCE_LIFECYCLE" \
  || fail "prior-read output-presence parser behavior"
assert_candidate_failure_snapshot_behavior "$PRIOR_SOURCE_LIFECYCLE" \
  || fail "candidate-write content-free failure snapshot behavior"

expect_contract_rejection "a shortened pinned SHA" \
  "s/$PINNED_SOURCE_SHA/${PINNED_SOURCE_SHA%????????}/g" lifecycle
expect_contract_rejection "a source-network command" \
  's/^set -euo pipefail$/git fetch origin/' lifecycle
# Match literal lifecycle source in the portable sed mutation.
# shellcheck disable=SC2016
expect_contract_rejection "removal of revision-root cwd binding" \
  's/cd "$repository_root"/cd "$REPO_DIR"/g' lifecycle
# Match literal lifecycle source in the portable sed mutation.
# shellcheck disable=SC2016
expect_contract_rejection "removal of the pinned revision CLI build" \
  's#"$PRIOR_WORKTREE/smackerel.sh" --env test build#"$PRIOR_WORKTREE/smackerel.sh" --env test check#g' lifecycle
expect_contract_rejection "removal of immutable image-ID inspection" \
  's/{{\.Id}}/{{.RepoTags}}/g' lifecycle
expect_contract_rejection "removal of OCI revision inspection" \
  's/org\.opencontainers\.image\.revision/org.opencontainers.image.version/g' lifecycle
expect_contract_rejection "removal of build-input identity recording" \
  's/PRIOR_DOCKERFILE_OID/PRIOR_BUILD_INPUT_REMOVED/g' lifecycle
expect_contract_rejection "an externally routed runtime network" \
  's/docker network create --internal/docker network create/g' lifecycle
expect_contract_rejection "re-enabling webhook mode in the disabled lifecycle profile" \
  's/ASSISTANT_TRANSPORTS_TELEGRAM_MODE=long_poll/ASSISTANT_TRANSPORTS_TELEGRAM_MODE=webhook/g' lifecycle
expect_contract_rejection "removal of the disabled Telegram transport flag" \
  '/ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED=false/d' lifecycle
# Match literal lifecycle source in the portable sed mutation.
# shellcheck disable=SC2016
expect_contract_rejection "a short-form published host port" \
  's/--read-only/-p "127.0.0.1:18080:${CORE_CONTAINER_PORT}"/g' lifecycle
# Match literal lifecycle source in the portable sed mutation.
# shellcheck disable=SC2016
expect_contract_rejection "a long-form published host port" \
  's/--read-only/--publish=127.0.0.1:18080:${CORE_CONTAINER_PORT}/g' lifecycle
expect_contract_rejection "moving the probe off the owned internal network" \
  's/--network "$NETWORK_NAME"/--network bridge/g' lifecycle
# Match literal lifecycle source in the portable sed mutation.
# shellcheck disable=SC2016
expect_contract_rejection "running the probe from a mutable image reference" \
  's/"$ACTIVE_ML_IMAGE_ID"/"$CURRENT_ML_IMAGE_REF"/g' lifecycle
expect_contract_rejection "removing authenticated GET probe authorization" \
  's/"bearer" "30"/"none" "30"/g' lifecycle
expect_contract_rejection "restoring jq -e rejection of an absent output field" \
  's/has("output") | tostring/has("output")/g' lifecycle
expect_contract_rejection "removing object-shape rejection from output-presence parsing" \
  's/if type == "object" then (has("output") | tostring) else error("expected object") end/has("output") | tostring/g' lifecycle
expect_contract_rejection "removing owned probe interruption cleanup" \
  's/remove_owned_probe_containers/remove_untracked_probe_containers/g' lifecycle
expect_contract_rejection "removing the bounded probe-container timeout" \
  's/smackerel_run_with_timeout --kill-after=5s "$container_timeout"/docker/g' lifecycle
expect_contract_rejection "removing one documented HTTP probe helper exit" \
  's/0 | 2 | 3 | 4/0 | 2 | 4/g' lifecycle
expect_contract_rejection "making readiness use the fatal normal API path" \
  's/if try_api_get "\/api\/health"/if api_get "\/api\/health"/g' lifecycle
# Match literal lifecycle source rather than expanding runtime values here.
# shellcheck disable=SC2016
expect_contract_rejection "making normal authenticated GET tolerate a probe failure" \
  's#run_http_probe "GET" "$CORE_URL$path" "" "bearer" "30" || probe_status=$?#run_http_probe "GET" "$CORE_URL$path" "" "bearer" "30" || return "$?"#g' lifecycle
expect_contract_rejection "removing the bounded two-second readiness interval" \
  's/sleep 2/sleep 0/g' lifecycle
expect_contract_rejection "removal of startup log redaction" \
  's#| redact_startup_logs "$AUTH_TOKEN_FILE" "$ACTIVE_ENV_FILE"#| cat#g' lifecycle
expect_contract_rejection "removal of private-value secret classification" \
  's/TOKEN|KEY|PASSWORD|SECRET|CREDENTIAL|PRIVATE/TOKEN|KEY|PASSWORD|SECRET|CREDENTIAL/g' lifecycle
expect_contract_rejection "removal of active generated environment clear" \
  's/  ACTIVE_ENV_FILE=""/  ACTIVE_ENV_FILE="not-cleared"/' lifecycle
expect_contract_rejection "removal of candidate safe error-code parsing" \
  '/error_code="$(safe_candidate_error_code "$HTTP_BODY")"/d' lifecycle
expect_contract_rejection "removal of candidate content-free failure snapshot emission" \
  '/emit_candidate_write_failure_snapshot "$expected_actor"/d' lifecycle
expect_contract_rejection "removal of parent-reapable resource labels" \
  's/com\.smackerel\.e2e-child-run-id/com.smackerel.untracked-child/g' lifecycle
expect_contract_rejection "removal of the cleanup trap" \
  's/^trap /trxp /g' lifecycle
expect_contract_rejection "loss of lifecycle and required registration in run_all" \
  "s/ $LIFECYCLE_BASENAME//g" runner
expect_contract_rejection "default discovery narrowed back to test-prefixed scripts" \
  's/:-\*/:-test_*/' runner
expect_contract_rejection "removal of direct-runner recursion protection" \
  's/"run_all"/"recursive_run_all"/g' runner
expect_contract_rejection "loss of lifecycle and required classification in smackerel.sh" \
  "s/$LIFECYCLE_FILENAME//g" dispatch

echo "PASS: $TEST_TITLE"
echo "PASS: synthesis test harness preserves stack lifecycle and zero-skip category boundaries"
