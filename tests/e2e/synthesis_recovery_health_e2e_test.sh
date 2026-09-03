#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=tests/e2e/lib/helpers.sh
source "$SCRIPT_DIR/lib/helpers.sh"

export TEST_ENV=test
export E2E_STACK_MANAGED=0

STACK_STARTED=0
TEST_PASS_READY=0
HTTP_STATUS=""
HTTP_BODY=""
GLOBAL_ACTOR=""
OTHER_ACTOR="t004-c16-other-principal"
DAILY_FRESHNESS_SECONDS=""
WEEKLY_FRESHNESS_SECONDS=""
BETWEEN_FRESHNESS_SECONDS=""

verify_test_stack_absent() {
    local compose_project="smackerel-test"
    local container_ids=""
    local network_ids=""
    local volume_ids=""

    container_ids="$(docker ps -aq --filter "label=com.docker.compose.project=$compose_project")"
    network_ids="$(docker network ls -q --filter "label=com.docker.compose.project=$compose_project")"
    volume_ids="$(docker volume ls -q --filter "label=com.docker.compose.project=$compose_project")"
    if [[ -n "$container_ids" || -n "$network_ids" || -n "$volume_ids" ]]; then
        echo "FAIL: smackerel-test cleanup left compose-owned resources" >&2
        return 1
    fi
    echo "CLEANUP: compose_project=smackerel-test containers=0 networks=0 volumes=0"
}

cleanup() {
    local original_status="${1:?original status required}"
    local teardown_status=0
    local absence_status=0
    local final_status="$original_status"

    trap - EXIT INT TERM HUP
    set +e
    if [[ "$STACK_STARTED" == "1" ]]; then
        e2e_cleanup
        teardown_status=$?
        verify_test_stack_absent
        absence_status=$?
    fi
    if [[ "$final_status" == "0" && "$teardown_status" != "0" ]]; then
        final_status="$teardown_status"
    fi
    if [[ "$final_status" == "0" && "$absence_status" != "0" ]]; then
        final_status="$absence_status"
    fi
    if [[ "$final_status" == "0" && "$TEST_PASS_READY" == "1" ]]; then
        e2e_pass "T004-C16/C17/C19/C20 running-core recovery health contract"
    fi
    exit "$final_status"
}

trap 'cleanup "$?"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

assert_positive_integer() {
    local value="${1:?integer value required}"
    local label="${2:?integer label required}"

    if [[ ! "$value" =~ ^[0-9]+$ || "$value" == "0" ]]; then
        e2e_fail "$label must be a positive integer"
    fi
}

synthesis_psql() {
    local sql="${1:?SQL required}"
    shift

    printf '%s\n' "$sql" |
        smackerel_compose "$TEST_ENV" exec -T postgres \
            env PGPASSWORD="$POSTGRES_PASSWORD" \
            psql -h 127.0.0.1 -p "$POSTGRES_CONTAINER_PORT" \
            -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 \
            "$@" -q --file=-
}

synthesis_psql_rows() {
    local sql="${1:?SQL query required}"
    shift

    synthesis_psql "$sql" "$@" --tuples-only --no-align --field-separator='|'
}

reset_synthesis_state() {
    synthesis_psql "
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
    echo "RESET: causal synthesis state is empty"
}

seed_canonical_corpus() {
    synthesis_psql "
DELETE FROM edges;
DELETE FROM artifacts;
DELETE FROM topics;

INSERT INTO topics (id, name)
VALUES ('t004-c17-recovery-topic', 'recovery health cluster');

INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
VALUES
    ('t004-c17-artifact-a', 'article', 'recovery source a', 't004-c17-hash-a', 't004-c17-source-a'),
    ('t004-c17-artifact-b', 'article', 'recovery source b', 't004-c17-hash-b', 't004-c17-source-b'),
    ('t004-c17-artifact-c', 'article', 'recovery source c', 't004-c17-hash-c', 't004-c17-source-c');

INSERT INTO edges (id, src_id, src_type, dst_id, dst_type, edge_type)
VALUES
    ('t004-c17-edge-a', 't004-c17-artifact-a', 'artifact', 't004-c17-recovery-topic', 'topic', 'BELONGS_TO'),
    ('t004-c17-edge-b', 't004-c17-artifact-b', 'artifact', 't004-c17-recovery-topic', 'topic', 'BELONGS_TO'),
    ('t004-c17-edge-c', 't004-c17-artifact-c', 'artifact', 't004-c17-recovery-topic', 'topic', 'BELONGS_TO');
"

    local counts
    counts="$(synthesis_psql_rows "
SELECT (SELECT COUNT(*) FROM topics WHERE id = 't004-c17-recovery-topic'),
       (SELECT COUNT(*) FROM artifacts WHERE id LIKE 't004-c17-artifact-%'),
       (SELECT COUNT(*) FROM edges WHERE dst_id = 't004-c17-recovery-topic');
")"
    if [[ "$counts" != "1|3|3" ]]; then
        e2e_fail "canonical recovery corpus was not stored completely"
    fi
    echo "SEED: canonical recovery corpus topics=1 artifacts=3 edges=3"
}

seed_verified_output() {
    local principal="${1:?principal required}"
    local cadence="${2:?cadence required}"
    local fixture="${3:?fixture required}"
    local kind="${4:?output kind required}"
    local age_seconds="${5:?age seconds required}"
    local terminal_event=""
    local window_start_days=""
    local window_end_days=""

    case "$cadence" in
        daily)
            window_start_days=2
            window_end_days=1
            ;;
        weekly)
            window_start_days=14
            window_end_days=7
            ;;
        *) e2e_fail "seed_verified_output received an unknown cadence" ;;
    esac
    case "$kind" in
        full) terminal_event="persisted" ;;
        quiet) terminal_event="quiet" ;;
        partial) terminal_event="partial" ;;
        *) e2e_fail "seed_verified_output received an unknown output kind" ;;
    esac
    assert_positive_integer "$age_seconds" "output age"

    synthesis_psql "
BEGIN;
INSERT INTO synthesis_runs
    (id, logical_key, cadence, principal, window_start, window_end,
     policy_version, source_set_digest, state, created_at, updated_at,
     lifecycle_state, attempt_count, lease_holder, lease_expires_at)
VALUES
    (:'run_id', :'logical_key', :'cadence', :'principal',
     CURRENT_TIMESTAMP - make_interval(days => (:'window_start_days')::integer),
     CURRENT_TIMESTAMP - make_interval(days => (:'window_end_days')::integer),
     'synthesis/v1',
     '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
     'succeeded',
     CURRENT_TIMESTAMP - make_interval(secs => (:'age_seconds')::integer) - INTERVAL '1 minute',
     CURRENT_TIMESTAMP - make_interval(secs => (:'age_seconds')::integer),
     'current', 1, NULL, NULL);

INSERT INTO synthesis_outputs
    (id, run_id, insight_count, citation_count, created_at, output_kind,
     evaluated_artifact_count, principal, cadence, window_start, window_end,
     lifecycle_state, superseded_at)
VALUES
    (:'output_id', :'run_id', 0, 0,
     CURRENT_TIMESTAMP - make_interval(secs => (:'age_seconds')::integer),
     :'kind', 3, :'principal', :'cadence',
     CURRENT_TIMESTAMP - make_interval(days => (:'window_start_days')::integer),
     CURRENT_TIMESTAMP - make_interval(days => (:'window_end_days')::integer),
     'current', NULL);

INSERT INTO synthesis_output_source_classes (output_id, source_class, disposition)
SELECT :'output_id', 'optional-source', 'omitted'
WHERE :'kind' = 'partial';

INSERT INTO synthesis_run_attempts
    (logical_key, outcome, failure_class, failure_message, recorded_at,
     failure_kind, run_id, attempt_no, trigger_kind, state, output_id,
     started_at, finished_at, failure_code, included_source_classes,
     omitted_source_classes, insight_count, citation_count)
VALUES
    (:'logical_key', 'succeeded', NULL, NULL,
     CURRENT_TIMESTAMP - make_interval(secs => (:'age_seconds')::integer),
     NULL, :'run_id', 1, 'scheduled', :'terminal_event', :'output_id',
     CURRENT_TIMESTAMP - make_interval(secs => (:'age_seconds')::integer) - INTERVAL '1 minute',
     CURRENT_TIMESTAMP - make_interval(secs => (:'age_seconds')::integer),
     NULL, '{}'::TEXT[],
     CASE WHEN :'kind' = 'partial' THEN ARRAY['optional-source']::TEXT[] ELSE '{}'::TEXT[] END,
     0, 0);

INSERT INTO synthesis_run_events
    (id, run_id, attempt_no, event_type, output_id, related_output_id,
     failure_code, insight_count, citation_count, created_at)
VALUES
    (:'started_event_id', :'run_id', 1, 'attempt_started', NULL, NULL,
     NULL, NULL, NULL,
     CURRENT_TIMESTAMP - make_interval(secs => (:'age_seconds')::integer) - INTERVAL '1 minute'),
    (:'terminal_event_id', :'run_id', 1, :'terminal_event', :'output_id', NULL,
     NULL, 0, 0,
     CURRENT_TIMESTAMP - make_interval(secs => (:'age_seconds')::integer));
COMMIT;
" \
        --set="principal=$principal" \
        --set="cadence=$cadence" \
        --set="run_id=t004-$fixture-run" \
        --set="logical_key=t004-$fixture-key" \
        --set="output_id=t004-$fixture-output" \
        --set="kind=$kind" \
        --set="terminal_event=$terminal_event" \
        --set="age_seconds=$age_seconds" \
        --set="window_start_days=$window_start_days" \
        --set="window_end_days=$window_end_days" \
        --set="started_event_id=t004-$fixture-started" \
        --set="terminal_event_id=t004-$fixture-$terminal_event"
}

seed_running_attempt() {
    local fixture="${1:?fixture required}"

    synthesis_psql "
BEGIN;
INSERT INTO synthesis_runs
    (id, logical_key, cadence, principal, window_start, window_end,
     policy_version, source_set_digest, state, created_at, updated_at,
     lifecycle_state, attempt_count, lease_holder, lease_expires_at)
VALUES
    (:'run_id', :'logical_key', 'weekly', :'principal',
     CURRENT_TIMESTAMP - INTERVAL '14 days', CURRENT_TIMESTAMP - INTERVAL '7 days',
     'synthesis/v1',
     '1123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
     'running', CURRENT_TIMESTAMP - INTERVAL '2 minutes', CURRENT_TIMESTAMP - INTERVAL '1 minute',
     'current', 1, 't004-c17-running-owner', CURRENT_TIMESTAMP + INTERVAL '1 hour');

INSERT INTO synthesis_run_attempts
    (logical_key, outcome, failure_class, failure_message, recorded_at,
     failure_kind, run_id, attempt_no, trigger_kind, state, output_id,
     started_at, finished_at, failure_code, included_source_classes,
     omitted_source_classes, insight_count, citation_count)
VALUES
    (:'logical_key', 'running', NULL, NULL, CURRENT_TIMESTAMP - INTERVAL '1 minute',
     NULL, :'run_id', 1, 'scheduled', 'running', NULL,
     CURRENT_TIMESTAMP - INTERVAL '1 minute', NULL, NULL,
     '{}'::TEXT[], '{}'::TEXT[], 0, 0);

INSERT INTO synthesis_run_events
    (id, run_id, attempt_no, event_type, output_id, related_output_id,
     failure_code, insight_count, citation_count, created_at)
VALUES
    (:'claimed_event_id', :'run_id', 1, 'claimed', NULL, NULL,
     NULL, NULL, NULL, CURRENT_TIMESTAMP - INTERVAL '70 seconds'),
    (:'started_event_id', :'run_id', 1, 'attempt_started', NULL, NULL,
     NULL, NULL, NULL, CURRENT_TIMESTAMP - INTERVAL '1 minute');
COMMIT;
" \
        --set="principal=$GLOBAL_ACTOR" \
        --set="run_id=t004-$fixture-run" \
        --set="logical_key=t004-$fixture-key" \
        --set="claimed_event_id=t004-$fixture-claimed" \
        --set="started_event_id=t004-$fixture-started"
}

seed_failed_attempt() {
    local fixture="${1:?fixture required}"

    synthesis_psql "
BEGIN;
INSERT INTO synthesis_runs
    (id, logical_key, cadence, principal, window_start, window_end,
     policy_version, source_set_digest, state, created_at, updated_at,
     lifecycle_state, attempt_count, lease_holder, lease_expires_at)
VALUES
    (:'run_id', :'logical_key', 'weekly', :'principal',
     CURRENT_TIMESTAMP - INTERVAL '21 days', CURRENT_TIMESTAMP - INTERVAL '14 days',
     'synthesis/v1',
     '2123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
     'failed', CURRENT_TIMESTAMP - INTERVAL '3 minutes', CURRENT_TIMESTAMP - INTERVAL '1 minute',
     'current', 1, NULL, NULL);

INSERT INTO synthesis_run_attempts
    (logical_key, outcome, failure_class, failure_message, recorded_at,
     failure_kind, run_id, attempt_no, trigger_kind, state, output_id,
     started_at, finished_at, failure_code, included_source_classes,
     omitted_source_classes, insight_count, citation_count)
VALUES
    (:'logical_key', 'failed', 'transaction_failed', 'terminal transaction failure',
     CURRENT_TIMESTAMP - INTERVAL '1 minute', 'terminal', :'run_id', 1,
     'scheduled', 'failed', NULL, CURRENT_TIMESTAMP - INTERVAL '2 minutes',
     CURRENT_TIMESTAMP - INTERVAL '1 minute', 'transaction_failed',
     '{}'::TEXT[], '{}'::TEXT[], 0, 0);

INSERT INTO synthesis_run_events
    (id, run_id, attempt_no, event_type, output_id, related_output_id,
     failure_code, insight_count, citation_count, created_at)
VALUES
    (:'started_event_id', :'run_id', 1, 'attempt_started', NULL, NULL,
     NULL, NULL, NULL, CURRENT_TIMESTAMP - INTERVAL '2 minutes'),
    (:'failed_event_id', :'run_id', 1, 'failed', NULL, NULL,
     'transaction_failed', NULL, NULL, CURRENT_TIMESTAMP - INTERVAL '1 minute');
COMMIT;
" \
        --set="principal=$GLOBAL_ACTOR" \
        --set="run_id=t004-$fixture-run" \
        --set="logical_key=t004-$fixture-key" \
        --set="started_event_id=t004-$fixture-started" \
        --set="failed_event_id=t004-$fixture-failed"
}

seed_readback_failed_attempt() {
    local fixture="${1:?fixture required}"

    synthesis_psql "
BEGIN;
INSERT INTO synthesis_runs
    (id, logical_key, cadence, principal, window_start, window_end,
     policy_version, source_set_digest, state, created_at, updated_at,
     lifecycle_state, attempt_count, lease_holder, lease_expires_at)
VALUES
    (:'run_id', :'logical_key', 'weekly', :'principal',
     CURRENT_TIMESTAMP - INTERVAL '21 days', CURRENT_TIMESTAMP - INTERVAL '14 days',
     'synthesis/v1',
     '3123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
     'failed', CURRENT_TIMESTAMP - INTERVAL '3 minutes', CURRENT_TIMESTAMP - INTERVAL '1 minute',
     'superseded', 1, NULL, NULL);

INSERT INTO synthesis_outputs
    (id, run_id, insight_count, citation_count, created_at, output_kind,
     evaluated_artifact_count, principal, cadence, window_start, window_end,
     lifecycle_state, superseded_at)
VALUES
    (:'output_id', :'run_id', 0, 0, CURRENT_TIMESTAMP - INTERVAL '1 minute',
     'full', 3, :'principal', 'weekly',
     CURRENT_TIMESTAMP - INTERVAL '21 days', CURRENT_TIMESTAMP - INTERVAL '14 days',
     'superseded', CURRENT_TIMESTAMP - INTERVAL '1 minute');

INSERT INTO synthesis_run_attempts
    (logical_key, outcome, failure_class, failure_message, recorded_at,
     failure_kind, run_id, attempt_no, trigger_kind, state, output_id,
     started_at, finished_at, failure_code, included_source_classes,
     omitted_source_classes, insight_count, citation_count)
VALUES
    (:'logical_key', 'readback_failed', 'readback_failed', 'aggregate read-back unavailable',
     CURRENT_TIMESTAMP - INTERVAL '1 minute', 'transient', :'run_id', 1,
     'scheduled', 'readback_failed', :'output_id', CURRENT_TIMESTAMP - INTERVAL '2 minutes',
     CURRENT_TIMESTAMP - INTERVAL '1 minute', 'readback_failed',
     '{}'::TEXT[], '{}'::TEXT[], 0, 0);

INSERT INTO synthesis_run_events
    (id, run_id, attempt_no, event_type, output_id, related_output_id,
     failure_code, insight_count, citation_count, created_at)
VALUES
    (:'started_event_id', :'run_id', 1, 'attempt_started', NULL, NULL,
     NULL, NULL, NULL, CURRENT_TIMESTAMP - INTERVAL '2 minutes'),
    (:'readback_event_id', :'run_id', 1, 'readback_failed', :'output_id', NULL,
     'readback_failed', 0, 0, CURRENT_TIMESTAMP - INTERVAL '1 minute');
COMMIT;
" \
        --set="principal=$GLOBAL_ACTOR" \
        --set="run_id=t004-$fixture-run" \
        --set="logical_key=t004-$fixture-key" \
        --set="output_id=t004-$fixture-output" \
        --set="started_event_id=t004-$fixture-started" \
        --set="readback_event_id=t004-$fixture-readback-failed"
}

request_http() {
    local method="${1:?HTTP method required}"
    local path="${2:?HTTP path required}"
    local auth_mode="${3:?auth mode required}"
    local body="${4-}"
    local max_seconds="${5:?request timeout required}"
    local response=""

    case "$auth_mode" in
        bearer)
            if [[ -n "$body" ]]; then
                response="$(curl -sS --max-time "$max_seconds" -X "$method" \
                    -H "Authorization: Bearer $AUTH_TOKEN" \
                    -H 'Content-Type: application/json' -d "$body" \
                    -w $'\n%{http_code}' "$CORE_URL$path")" \
                    || e2e_fail "authenticated HTTP transport failed"
            else
                response="$(curl -sS --max-time "$max_seconds" -X "$method" \
                    -H "Authorization: Bearer $AUTH_TOKEN" \
                    -w $'\n%{http_code}' "$CORE_URL$path")" \
                    || e2e_fail "authenticated HTTP transport failed"
            fi
            ;;
        public)
            response="$(curl -sS --max-time "$max_seconds" -X "$method" \
                -w $'\n%{http_code}' "$CORE_URL$path")" \
                || e2e_fail "public HTTP transport failed"
            ;;
        *) e2e_fail "request_http received an unknown auth mode" ;;
    esac

    HTTP_STATUS="${response##*$'\n'}"
    HTTP_BODY="${response%$'\n'*}"
    if [[ ! "$HTTP_STATUS" =~ ^[0-9]{3}$ ]]; then
        e2e_fail "HTTP response did not include a status code"
    fi
}

assert_no_public_identity() {
    local body="${1:?response body required}"
    local surface="${2:?surface required}"
    local marker

    for marker in "$GLOBAL_ACTOR" "$OTHER_ACTOR" "t004-c16" "t004-c17" "$AUTH_TOKEN"; do
        if [[ "$body" == *"$marker"* ]]; then
            e2e_fail "$surface exposed a principal, run, content, or credential marker"
        fi
    done
}

assert_public_health_is_aggregate_only() {
    local expected_status="${1:?expected aggregate status required}"
    local aggregate_check=""
    local diagnostics='{"actual_status":"<invalid-json>","services_type":"<invalid-json>","keys":[]}'

    if ! aggregate_check="$(jq -er --arg status "$expected_status" '
        type == "object" and
        .status == $status and
        .services == null and
        ((keys | sort) == ["services", "status"])
    ' <<<"$HTTP_BODY")" || [[ "$aggregate_check" != "true" ]]; then
        if ! diagnostics="$(jq -cer '
            {
                actual_status: (if type == "object" and has("status") and (.status | type) == "string" then .status else "<missing-or-non-string>" end),
                services_type: (if type == "object" and has("services") then (.services | type) else "<missing>" end),
                keys: (if type == "object" then (keys | sort) else [] end)
            }
        ' <<<"$HTTP_BODY")"; then
            diagnostics='{"actual_status":"<invalid-json>","services_type":"<invalid-json>","keys":[]}'
        fi
        e2e_fail "public strict health was not aggregate-only: diagnostics=$diagnostics"
    fi
    assert_no_public_identity "$HTTP_BODY" "public strict health"
}

assert_metric_state() {
    local expected="${1:?expected metric state required}"
    local metric_name=""
    local metric_value=""
    local remainder=""
    local observed=""
    local matches=0

    request_http GET "/metrics" bearer "" 15
    if [[ "$HTTP_STATUS" != "200" ]]; then
        e2e_fail "metrics endpoint did not return HTTP 200"
    fi
    assert_no_public_identity "$HTTP_BODY" "metrics"
    while IFS=' ' read -r metric_name metric_value remainder; do
        if [[ "$metric_name" == "smackerel_synthesis_state" ]]; then
            observed="$metric_value"
            matches=$((matches + 1))
        fi
    done <<<"$HTTP_BODY"
    if [[ "$matches" != "1" || "$observed" != "$expected" ]]; then
        e2e_fail "canonical synthesis metric did not expose the expected exclusive state"
    fi
}

assert_latest_state() {
    local cadence="${1:?cadence required}"
    local expected_state="${2:?expected latest state required}"
    local output_presence="${3:?expected output presence required}"
    local latest_summary=""

    request_http GET "/api/synthesis/latest?cadence=$cadence" bearer "" 30
    if [[ "$HTTP_STATUS" != "200" ]]; then
        e2e_fail "latest synthesis endpoint did not return HTTP 200"
    fi
    if ! latest_summary="$(jq -er '
        [.state, (has("output") | tostring),
         ((has("principal") or (.output? | type == "object" and
           (has("principal") or has("text") or has("insights") or has("citations")))) | not | tostring)]
        | @tsv
    ' <<<"$HTTP_BODY")"; then
        e2e_fail "latest synthesis response shape was invalid"
    fi

    local actual_state=""
    local actual_output_presence=""
    local content_free=""
    IFS=$'\t' read -r actual_state actual_output_presence content_free <<<"$latest_summary"
    if [[ "$actual_state" != "$expected_state" ||
        "$actual_output_presence" != "$output_presence" ||
        "$content_free" != "true" ]]; then
        e2e_fail "latest synthesis state or content-free shape was incorrect for cadence=$cadence"
    fi
}

assert_latest_pair() {
    local cadence="${1:?cadence required}"
    local expected_run="${2:?expected run required}"
    local expected_output="${3:?expected output required}"
    local pair_summary=""
    local state_match=""
    local run_match=""
    local output_match=""
    local cadence_match=""
    local content_free=""
    local actual_state=""
    local actual_run=""
    local actual_output=""
    local actual_cadence=""

    request_http GET "/api/synthesis/latest?cadence=$cadence" bearer "" 30
    if [[ "$HTTP_STATUS" != "200" ]]; then
        e2e_fail "causal latest endpoint did not return HTTP 200"
    fi
    if ! pair_summary="$(jq -er --arg cadence "$cadence" --arg run "$expected_run" --arg output "$expected_output" '
        [(.state == "quiet"),
         (.latestAttempt.runId == $run),
         (.output.outputId == $output),
         (.output.cadence == $cadence),
         ((has("principal") | not) and
          (.output | (has("principal") or has("text") or has("insights") or has("citations") | not))),
         .state,
         .latestAttempt.runId,
         .output.outputId,
         .output.cadence]
        | map(tostring) | @tsv
    ' <<<"$HTTP_BODY")"; then
        e2e_fail "running-core latest response could not be evaluated"
    fi
    IFS=$'\t' read -r state_match run_match output_match cadence_match content_free \
        actual_state actual_run actual_output actual_cadence <<<"$pair_summary"
    if [[ "$state_match" != "true" || "$run_match" != "true" ||
        "$output_match" != "true" || "$cadence_match" != "true" ||
        "$content_free" != "true" ]]; then
        e2e_fail "running-core latest pair mismatch requested_cadence=$cadence expected_run=$expected_run expected_output=$expected_output actual_state=$actual_state actual_run=$actual_run actual_output=$actual_output actual_cadence=$actual_cadence state_match=$state_match run_match=$run_match output_match=$output_match cadence_match=$cadence_match content_free=$content_free"
    fi
}

restart_core_with_current_state() {
    local probe

    echo "RESTART: smackerel-test core state refresh starting"
    smackerel_compose "$TEST_ENV" restart smackerel-core
    for probe in 1 2 3; do
        e2e_wait_healthy 180
        if [[ "$probe" != "3" ]]; then
            sleep 2
        fi
    done
    echo "RESTART: smackerel-test core state refresh complete"
}

assert_health_contract() {
    local label="${1:?case label required}"
    local expected_intelligence="${2:?expected intelligence status required}"
    local expected_overall="${3:?expected overall status required}"
    local expected_strict_http="${4:?expected strict status required}"
    local expected_metric="${5:?expected metric required}"
    local diagnostic_summary=""
    local diagnostic_overall="<invalid-health-json>"
    local diagnostic_services="<unavailable>"
    local health_summary=""
    local actual_overall=""
    local actual_intelligence=""

    request_http GET "/api/health?strict=true" bearer "" 15
    if [[ "$HTTP_STATUS" != "$expected_strict_http" ]]; then
        if diagnostic_summary="$(jq -er '
            [
              (if (.status | type) == "string" then .status else "<invalid>" end),
              (if (.services | type) == "object" then
                 [.services | to_entries[] |
                   "\(.key)=\(if (.value | type) == \"object\" and ((.value.status | type) == \"string\") then .value.status else \"<invalid>\" end)"]
                 | sort | join(",")
               else "<invalid>" end)
            ] | @tsv
        ' <<<"$HTTP_BODY" 2>/dev/null)"; then
            IFS=$'\t' read -r diagnostic_overall diagnostic_services <<<"$diagnostic_summary"
        fi
        e2e_fail "$label authenticated strict health returned the wrong status expected_http=$expected_strict_http actual_http=$HTTP_STATUS aggregate_status=$diagnostic_overall service_statuses=$diagnostic_services"
    fi
    if ! health_summary="$(jq -er '[.status, .services.intelligence.status] | @tsv' <<<"$HTTP_BODY")"; then
        e2e_fail "$label authenticated strict health shape was invalid"
    fi
    IFS=$'\t' read -r actual_overall actual_intelligence <<<"$health_summary"
    if [[ "$actual_overall" != "$expected_overall" || "$actual_intelligence" != "$expected_intelligence" ]]; then
        e2e_fail "$label authenticated strict health did not reflect synthesis truth"
    fi
    assert_no_public_identity "$HTTP_BODY" "authenticated health"

    request_http GET "/api/health?strict=true" public "" 15
    if [[ "$HTTP_STATUS" != "$expected_strict_http" ]]; then
        e2e_fail "$label public strict health returned the wrong status"
    fi
    assert_public_health_is_aggregate_only "$expected_overall"

    request_http GET "/api/health" public "" 15
    if [[ "$HTTP_STATUS" != "200" ]]; then
        e2e_fail "$label default liveness was not HTTP 200"
    fi
    assert_public_health_is_aggregate_only "$expected_overall"
    assert_metric_state "$expected_metric"
    echo "PASS: $label strict=$expected_strict_http liveness=200 metric=$expected_metric"
}

load_generated_freshness() {
    local env_file=""
    local compose_project=""

    env_file="$(smackerel_require_env_file "$TEST_ENV")"
    compose_project="$(smackerel_compose_project "$TEST_ENV")"
    if [[ "$compose_project" != "smackerel-test" ]]; then
        e2e_fail "recovery E2E resolved a non-test Compose project"
    fi

    GLOBAL_ACTOR="$(smackerel_env_value "$env_file" "SYNTHESIS_ACTOR_USER_ID")"
    DAILY_FRESHNESS_SECONDS="$(smackerel_env_value "$env_file" "SYNTHESIS_DAILY_FRESHNESS_SECONDS")"
    WEEKLY_FRESHNESS_SECONDS="$(smackerel_env_value "$env_file" "SYNTHESIS_WEEKLY_FRESHNESS_SECONDS")"
    : "${GLOBAL_ACTOR:?generated test environment must set SYNTHESIS_ACTOR_USER_ID}"
    assert_positive_integer "$DAILY_FRESHNESS_SECONDS" "generated daily freshness"
    assert_positive_integer "$WEEKLY_FRESHNESS_SECONDS" "generated weekly freshness"
    if ((DAILY_FRESHNESS_SECONDS >= WEEKLY_FRESHNESS_SECONDS)); then
        e2e_fail "generated daily and weekly freshness values are not distinct and ordered"
    fi
    BETWEEN_FRESHNESS_SECONDS=$((DAILY_FRESHNESS_SECONDS + (WEEKLY_FRESHNESS_SECONDS - DAILY_FRESHNESS_SECONDS) / 2))
    if ((BETWEEN_FRESHNESS_SECONDS <= DAILY_FRESHNESS_SECONDS || BETWEEN_FRESHNESS_SECONDS >= WEEKLY_FRESHNESS_SECONDS)); then
        e2e_fail "could not derive an age strictly between generated cadence budgets"
    fi
    echo "CONFIG: generated daily_seconds=$DAILY_FRESHNESS_SECONDS weekly_seconds=$WEEKLY_FRESHNESS_SECONDS distinct=true"
}

run_cross_pair_contract() {
    reset_synthesis_state
    seed_verified_output "$GLOBAL_ACTOR" daily "c16-global-daily" quiet 600
    seed_verified_output "$GLOBAL_ACTOR" weekly "c16-global-weekly" quiet 500
    seed_verified_output "$OTHER_ACTOR" daily "c16-other-daily" quiet 30
    seed_verified_output "$OTHER_ACTOR" weekly "c16-other-weekly" quiet 20
    restart_core_with_current_state

    assert_latest_pair daily "t004-c16-global-daily-run" "t004-c16-global-daily-output"
    assert_latest_pair weekly "t004-c16-global-weekly-run" "t004-c16-global-weekly-output"
    echo "PASS: T004-C16 running-core causal reads stayed inside global actor and requested cadence"
}

run_freshness_contract() {
    reset_synthesis_state
    seed_verified_output "$GLOBAL_ACTOR" daily "c19-between-daily" quiet "$BETWEEN_FRESHNESS_SECONDS"
    seed_verified_output "$GLOBAL_ACTOR" weekly "c19-between-weekly" quiet "$BETWEEN_FRESHNESS_SECONDS"
    restart_core_with_current_state
    assert_latest_state daily quiet true
    assert_latest_state weekly quiet true
    assert_health_contract "T004-C19 daily stale at between-budget age" stale degraded 503 2

    reset_synthesis_state
    seed_verified_output "$GLOBAL_ACTOR" daily "c19-fresh-daily" quiet 60
    seed_verified_output "$GLOBAL_ACTOR" weekly "c19-valid-weekly" quiet "$BETWEEN_FRESHNESS_SECONDS"
    restart_core_with_current_state
    assert_health_contract "T004-C19 weekly current at between-budget age" up healthy 200 1
    echo "PASS: T004-C19 running core applied distinct generated cadence freshness budgets"
}

seed_daily_control() {
    seed_verified_output "$GLOBAL_ACTOR" daily "${1:?fixture prefix required}-daily-control" quiet 60
}

run_non_green_matrix() {
    reset_synthesis_state
    seed_daily_control "c20-never-run"
    restart_core_with_current_state
    assert_latest_state weekly never-run false
    assert_health_contract "T004-C20 weekly never-run" down degraded 503 0

    reset_synthesis_state
    seed_daily_control "c20-running"
    seed_running_attempt "c20-running-weekly"
    restart_core_with_current_state
    assert_latest_state weekly running false
    assert_health_contract "T004-C20 weekly running" down degraded 503 4

    reset_synthesis_state
    seed_daily_control "c20-stale"
    seed_verified_output "$GLOBAL_ACTOR" weekly "c20-stale-weekly" quiet "$((WEEKLY_FRESHNESS_SECONDS + 3600))"
    restart_core_with_current_state
    assert_latest_state weekly quiet true
    assert_health_contract "T004-C20 weekly stale" stale degraded 503 2

    reset_synthesis_state
    seed_daily_control "c20-partial"
    seed_verified_output "$GLOBAL_ACTOR" weekly "c20-partial-weekly" partial 60
    restart_core_with_current_state
    assert_latest_state weekly partial true
    assert_health_contract "T004-C20 weekly partial" down degraded 503 3

    reset_synthesis_state
    seed_daily_control "c20-readback"
    seed_readback_failed_attempt "c20-readback-weekly"
    restart_core_with_current_state
    assert_latest_state weekly read-degraded false
    assert_health_contract "T004-C20 weekly read-degraded" down degraded 503 4
}

assert_failure_history_count() {
    local expected="${1:?expected failure history count required}"
    local counts=""

    counts="$(synthesis_psql_rows "
SELECT (SELECT COUNT(*) FROM synthesis_run_attempts
        WHERE logical_key = 't004-c17-recovery-failure-key'
          AND state = 'failed' AND outcome = 'failed'),
       (SELECT COUNT(*) FROM synthesis_run_events
        WHERE id = 't004-c17-recovery-failure-failed'
          AND event_type = 'failed' AND failure_code = 'transaction_failed');
")"
    if [[ "$counts" != "$expected|$expected" ]]; then
        e2e_fail "failed attempt/event history was not retained"
    fi
}

run_recovery_contract() {
    local recovery_summary=""
    local recovery_outcome=""
    local recovery_output_id=""
    local running_leases=""

    reset_synthesis_state
    seed_canonical_corpus
    seed_daily_control "c17-recovery"
    seed_failed_attempt "c17-recovery-failure"
    restart_core_with_current_state
    assert_latest_state weekly failed-without-output false
    assert_failure_history_count 1
    assert_health_contract "T004-C17 weekly failed before retry" down degraded 503 4

    request_http POST "/api/synthesis/retry" bearer '{"cadence":"weekly"}' 60
    if [[ "$HTTP_STATUS" != "200" ]]; then
        e2e_fail "production synthesis retry did not return HTTP 200"
    fi
    if ! recovery_summary="$(jq -er '[.outcome, .output.outputId] | @tsv' <<<"$HTTP_BODY")"; then
        e2e_fail "production synthesis retry response shape was invalid"
    fi
    IFS=$'\t' read -r recovery_outcome recovery_output_id <<<"$recovery_summary"
    if [[ "$recovery_outcome" != "persisted" || -z "$recovery_output_id" ]]; then
        e2e_fail "production synthesis retry did not return a persisted identity"
    fi

    restart_core_with_current_state
    assert_latest_pair weekly "$(synthesis_psql_rows "
SELECT run_id FROM synthesis_outputs WHERE id = :'output_id';
" --set="output_id=$recovery_output_id")" "$recovery_output_id"
    assert_failure_history_count 1
    assert_health_contract "T004-C17 verified weekly recovery" up healthy 200 1

    running_leases="$(synthesis_psql_rows "
SELECT COUNT(*) FROM synthesis_runs
WHERE state = 'running' OR lease_holder IS NOT NULL OR lease_expires_at IS NOT NULL;
")"
    if [[ "$running_leases" != "0" ]]; then
        e2e_fail "recovery contract left an active synthesis run or lease"
    fi
    echo "PASS: T004-C17 failure history remained append-only after production retry recovery"
}

echo "=== SCOPE-04A recovery, health, freshness, and alert-source E2E ==="
STACK_STARTED=1
e2e_start
load_generated_freshness

run_cross_pair_contract
run_freshness_contract
run_non_green_matrix
run_recovery_contract

TEST_PASS_READY=1