#!/usr/bin/env bash
# BUG-004-004 SCOPE-03 — T004-02-RESTART / T004-06-RECOVERY.
#
# These two properties cannot be proven from inside the process under test,
# because the thing being tested is what survives when that process goes away.
# A synthesis identity held in memory would look perfectly durable to every
# in-process test and would still be lost on the first restart.
#
# This runs on the host, where the container runtime is reachable, so it can
# restart the real core process and then ask the running system the same
# questions again. Identity must be unchanged, and the read path must still
# know what happened before the restart.
#
# It uses the shared e2e helpers, which adapt to whichever side owns the stack:
# when the suite runner has already started it, e2e_start only loads env and
# waits for health; otherwise it brings the stack up itself.
#
# SCN-004-004-C15 / T004-C15-RESTART:
# committed-unverified state survives restart and recovery appends after verified read-back
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=tests/e2e/lib/helpers.sh
source "$SCRIPT_DIR/lib/helpers.sh"

FAULT_PROFILE_ID="synthesis_readback_relation_failure"
FAULT_PROFILE_REGISTRY="$REPO_DIR/config/acceptance/fault-profiles.v1.yaml"
KNOWN_ARTIFACT_ID="t004-c15-restart-artifact-c"
KNOWN_TOPIC_ID="t004-c15-restart-topic"
CANONICAL_ARTIFACT_FENCE_TRIGGER="e2e_c15_canonical_artifact_fence"
CANONICAL_ARTIFACT_FENCE_FUNCTION="e2e_c15_canonical_artifact_fence_guard"

STACK_STARTED=0
TEST_PASS_READY=0
CANONICAL_ARTIFACT_FENCE_ACTIVE=0
HTTP_STATUS=""
HTTP_BODY=""

wait_for_stable_health() {
    local max_seconds="${1:?maximum wait seconds required}"
    local needed=3
    local streak=0
    local started_at=$SECONDS
    local health_response=""
    local health_error=""
    local postgres_response=""
    local api_transport_status="not_checked"
    local health_payload_status="not_checked"
    local postgres_status="not_checked"

    echo "WAIT: core infrastructure readiness status=starting required_streak=$needed max_seconds=$max_seconds"
    while ((SECONDS - started_at < max_seconds)); do
        api_transport_status="failed"
        health_payload_status="not_checked"
        postgres_status="failed"
        health_response=""
        health_error=""
        postgres_response=""

        if health_response="$(e2e_health_response 2>&1)"; then
            api_transport_status="ready"
            if health_error="$(e2e_health_payload_ready "$health_response" 2>&1)"; then
                health_payload_status="ready"
            else
                health_payload_status="failed"
            fi
        fi
        if postgres_response="$(e2e_postgres_select_one 2>&1)" \
            && [[ "$postgres_response" == "1" ]]; then
            postgres_status="ready"
        fi

        if [[ "$api_transport_status" == "ready" \
            && "$health_payload_status" == "ready" \
            && "$postgres_status" == "ready" ]]; then
            streak=$((streak + 1))
            if ((streak >= needed)); then
                echo "READY: core infrastructure status=stable streak=$streak"
                return 0
            fi
        else
            streak=0
        fi
        sleep 2
    done

    echo "FAIL: core did not reach $needed consecutive infrastructure-ready probes within ${max_seconds}s"
    echo "FAIL: final infrastructure readiness api_transport=$api_transport_status health_payload=$health_payload_status postgres=$postgres_status"
    return 1
}

synthesis_psql_command() {
    local sql="${1:?SQL command required}"

    smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
        env PGPASSWORD="$POSTGRES_PASSWORD" \
        psql -h 127.0.0.1 -p "$POSTGRES_CONTAINER_PORT" \
        -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -q -c "$sql"
}

synthesis_psql_rows() {
    local sql="${1:?SQL query required}"

    smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
        env PGPASSWORD="$POSTGRES_PASSWORD" \
        psql -h 127.0.0.1 -p "$POSTGRES_CONTAINER_PORT" \
        -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -A -t -F '|' -c "$sql"
}

install_canonical_artifact_fence() {
    local lifecycle="${1:?fence lifecycle required}"

    if ! synthesis_psql_command "
BEGIN;
LOCK TABLE artifacts IN SHARE ROW EXCLUSIVE MODE;
DROP TRIGGER IF EXISTS $CANONICAL_ARTIFACT_FENCE_TRIGGER ON artifacts;
DROP FUNCTION IF EXISTS $CANONICAL_ARTIFACT_FENCE_FUNCTION();
CREATE FUNCTION $CANONICAL_ARTIFACT_FENCE_FUNCTION()
RETURNS TRIGGER LANGUAGE plpgsql AS \$fence\$
BEGIN
    IF NEW.id NOT IN (
        't004-c15-restart-artifact-a',
        't004-c15-restart-artifact-b',
        '$KNOWN_ARTIFACT_ID'
    ) THEN
        RAISE EXCEPTION 'test canonical artifact insert fence rejected noncanonical row';
    END IF;
    RETURN NEW;
END;
\$fence\$;
CREATE TRIGGER $CANONICAL_ARTIFACT_FENCE_TRIGGER
BEFORE INSERT ON artifacts
FOR EACH ROW
EXECUTE FUNCTION $CANONICAL_ARTIFACT_FENCE_FUNCTION();
COMMIT;
"; then
        return 1
    fi
    CANONICAL_ARTIFACT_FENCE_ACTIVE=1
    echo "FENCE: canonical artifact insert fence status=active lifecycle=$lifecycle"
}

remove_canonical_artifact_fence() {
    local lifecycle="${1:?fence lifecycle required}"

    if ! synthesis_psql_command "
DROP TRIGGER IF EXISTS $CANONICAL_ARTIFACT_FENCE_TRIGGER ON artifacts;
DROP FUNCTION IF EXISTS $CANONICAL_ARTIFACT_FENCE_FUNCTION();
"; then
        return 1
    fi
    CANONICAL_ARTIFACT_FENCE_ACTIVE=0
    echo "FENCE: canonical artifact insert fence status=inactive lifecycle=$lifecycle"
}

assert_canonical_artifact_fence_active() {
    local retry_stage="${1:?retry stage required}"

    if [[ "$CANONICAL_ARTIFACT_FENCE_ACTIVE" != "1" ]]; then
        e2e_fail "historical restart canonical artifact fence status=inactive boundary=$retry_stage"
    fi
    echo "FENCE: canonical artifact insert fence status=active boundary=$retry_stage"
}

repair_readback_relation() {
    synthesis_psql_command "
DROP TRIGGER IF EXISTS e2e_c15_break_synthesis_readback_relation ON synthesis_citations;
DROP FUNCTION IF EXISTS e2e_c15_break_synthesis_readback_relation();
DO \$repair\$
BEGIN
    IF to_regclass('public.synthesis_output_source_classes_readback_fault') IS NOT NULL THEN
        IF to_regclass('public.synthesis_output_source_classes') IS NOT NULL THEN
            RAISE EXCEPTION 'both synthesis source-class relation names exist';
        END IF;
        EXECUTE 'ALTER TABLE synthesis_output_source_classes_readback_fault RENAME TO synthesis_output_source_classes';
    END IF;
END;
\$repair\$;
"
}

clear_corrective_fixture() {
    synthesis_psql_command "
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
DELETE FROM edges
WHERE id IN ('t004-c15-restart-edge-a', 't004-c15-restart-edge-b', 't004-c15-restart-edge-c')
     OR src_id IN ('t004-c15-restart-artifact-a', 't004-c15-restart-artifact-b', '$KNOWN_ARTIFACT_ID');
DELETE FROM artifacts
WHERE id IN ('t004-c15-restart-artifact-a', 't004-c15-restart-artifact-b', '$KNOWN_ARTIFACT_ID');
DELETE FROM topics WHERE id = '$KNOWN_TOPIC_ID';
"
}

reset_core_after_failure() {
    echo "CLEANUP: core status=recreate_starting"
    if ! smackerel_compose "$TEST_ENV" up -d --no-deps --force-recreate smackerel-core; then
        echo "FAIL: cleanup core status=recreate_failed"
        return 1
    fi
    if ! wait_for_stable_health 90; then
        echo "FAIL: cleanup core status=readiness_failed"
        return 1
    fi
    echo "CLEANUP: core status=recreated"
}

cleanup() {
    local original_status="${1:?original status required}"
    local fence_cleanup_status=0
    local repair_status=0
    local state_cleanup_status=0
    local recreate_status=0
    local stack_cleanup_status=0
    local final_status="$original_status"
    local cleanup_status

    trap - EXIT INT TERM HUP
    set +e
    if [[ "$STACK_STARTED" == "1" ]]; then
        remove_canonical_artifact_fence cleanup
        fence_cleanup_status=$?
        repair_readback_relation
        repair_status=$?
        if [[ "$repair_status" == "0" ]]; then
            clear_corrective_fixture
            state_cleanup_status=$?
        fi
        if [[ "$original_status" != "0" ]]; then
            reset_core_after_failure
            recreate_status=$?
        fi
        e2e_cleanup
        stack_cleanup_status=$?
    fi

    if [[ "$fence_cleanup_status" != "0" ]]; then
        echo "FAIL: cleanup canonical artifact fence status=remove_failed"
    fi
    if [[ "$repair_status" != "0" ]]; then
        echo "FAIL: cleanup relation status=repair_failed"
    fi
    if [[ "$state_cleanup_status" != "0" ]]; then
        echo "FAIL: cleanup synthesis state status=reset_failed"
    fi
    if [[ "$recreate_status" != "0" ]]; then
        echo "FAIL: cleanup core status=restore_failed"
    fi
    if [[ "$stack_cleanup_status" != "0" ]]; then
        echo "FAIL: cleanup stack status=teardown_failed"
    fi

    if [[ "$final_status" == "0" ]]; then
        for cleanup_status in "$fence_cleanup_status" "$repair_status" "$state_cleanup_status" "$recreate_status" "$stack_cleanup_status"; do
            if [[ "$cleanup_status" != "0" ]]; then
                final_status="$cleanup_status"
                break
            fi
        done
    fi
    if [[ "$final_status" == "0" && "$TEST_PASS_READY" == "1" ]]; then
        e2e_pass "T004-C15-RESTART committed-unverified durability and verified recovery"
    fi
    exit "$final_status"
}

trap 'cleanup "$?"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

assert_canonical_fault_profile() {
    local profile_count

    profile_count="$(grep -F -c "stableId: \"$FAULT_PROFILE_ID\"" "$FAULT_PROFILE_REGISTRY" || true)"
    if [[ "$profile_count" != "1" ]]; then
        e2e_fail "T004-C15 canonical fault profile count=$profile_count want=1"
    fi
    echo "PROFILE: stable_id=$FAULT_PROFILE_ID registry=canonical activation=test_owned"
}

reset_synthesis_state_and_seed_corpus() {
    local synthesis_counts
    local corpus_counts

    synthesis_psql_command "
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
DELETE FROM edges;
DELETE FROM artifacts;
DELETE FROM topics;

INSERT INTO topics (id, name)
VALUES ('$KNOWN_TOPIC_ID', 'restart durability cluster');

INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
VALUES
    ('t004-c15-restart-artifact-a', 'article', 'restart durability source a', 't004-c15-restart-hash-a', 't004-c15-source-a'),
    ('t004-c15-restart-artifact-b', 'article', 'restart durability source b', 't004-c15-restart-hash-b', 't004-c15-source-b'),
    ('$KNOWN_ARTIFACT_ID', 'article', 'restart durability source c', 't004-c15-restart-hash-c', 't004-c15-source-c');

INSERT INTO edges (id, src_id, src_type, dst_id, dst_type, edge_type)
VALUES
    ('t004-c15-restart-edge-a', 't004-c15-restart-artifact-a', 'artifact', '$KNOWN_TOPIC_ID', 'topic', 'BELONGS_TO'),
    ('t004-c15-restart-edge-b', 't004-c15-restart-artifact-b', 'artifact', '$KNOWN_TOPIC_ID', 'topic', 'BELONGS_TO'),
    ('t004-c15-restart-edge-c', '$KNOWN_ARTIFACT_ID', 'artifact', '$KNOWN_TOPIC_ID', 'topic', 'BELONGS_TO');
"

    synthesis_counts="$(synthesis_psql_rows "
SELECT (SELECT COUNT(*) FROM synthesis_runs),
             (SELECT COUNT(*) FROM synthesis_outputs),
             (SELECT COUNT(*) FROM synthesis_run_attempts),
             (SELECT COUNT(*) FROM synthesis_run_events);
")"
    if [[ "$synthesis_counts" != "0|0|0|0" ]]; then
        e2e_fail "T004-C15 synthesis reset status=nonempty counts=$synthesis_counts"
    fi

    corpus_counts="$(synthesis_psql_rows "
SELECT (SELECT COUNT(*) FROM topics WHERE id = '$KNOWN_TOPIC_ID'),
             (SELECT COUNT(*) FROM artifacts WHERE id IN (
                 't004-c15-restart-artifact-a', 't004-c15-restart-artifact-b', '$KNOWN_ARTIFACT_ID'
             )),
             (SELECT COUNT(*) FROM edges WHERE dst_id = '$KNOWN_TOPIC_ID' AND edge_type = 'BELONGS_TO'),
             (SELECT COUNT(DISTINCT source_id) FROM artifacts WHERE id IN (
                 't004-c15-restart-artifact-a', 't004-c15-restart-artifact-b', '$KNOWN_ARTIFACT_ID'
             ));
")"
  if [[ "$corpus_counts" != "1|3|3|3" ]]; then
    e2e_fail "T004-C15 canonical corpus status=invalid counts=$corpus_counts"
  fi
  echo "SEED: synthesis_rows=0 canonical_topics=1 canonical_artifacts=3 canonical_edges=3 distinct_sources=3"
}

eligible_source_set_snapshot() {
    synthesis_psql_rows "
SELECT COUNT(*),
                         COALESCE(STRING_AGG(id, ',' ORDER BY id COLLATE \"C\"), '')
FROM artifacts;
"
}

sha256_for_source_set_input() {
    local digest_input="${1:?source-set digest input required}"
    local digest_line

    if command -v sha256sum >/dev/null 2>&1; then
        digest_line="$(printf '%s' "$digest_input" | sha256sum)"
    elif command -v shasum >/dev/null 2>&1; then
        digest_line="$(printf '%s' "$digest_input" | shasum -a 256)"
    else
        return 1
    fi
    printf '%s\n' "${digest_line%% *}"
}

durable_run_key_fields() {
    local output_id="${1:?output id required}"

    synthesis_psql_rows "
SELECT r.source_set_digest, r.policy_version
FROM synthesis_runs r
JOIN synthesis_outputs o ON o.run_id = r.id
WHERE o.id = '$output_id';
"
}

synthesis_history_snapshot() {
    local relation_state
    local source_class_relation

    relation_state="$(synthesis_psql_rows "
SELECT (to_regclass('public.synthesis_output_source_classes') IS NOT NULL)::int,
             (to_regclass('public.synthesis_output_source_classes_readback_fault') IS NOT NULL)::int;
")"
    case "$relation_state" in
        "1|0")
            source_class_relation="synthesis_output_source_classes"
            ;;
        "0|1")
            source_class_relation="synthesis_output_source_classes_readback_fault"
            ;;
        "0|0")
            e2e_fail "historical restart synthesis history source-class relation status=missing"
            ;;
        "1|1")
            e2e_fail "historical restart synthesis history source-class relation status=ambiguous"
            ;;
        *)
            e2e_fail "historical restart synthesis history source-class relation status=invalid"
            ;;
    esac

    case "$source_class_relation" in
        synthesis_output_source_classes | synthesis_output_source_classes_readback_fault) ;;
        *)
            e2e_fail "historical restart synthesis history source-class relation status=outside_closed_set"
            ;;
    esac

    synthesis_psql_rows "
SELECT (SELECT COUNT(*) FROM synthesis_runs),
                         (SELECT COUNT(*) FROM synthesis_outputs),
                         (SELECT COUNT(*) FROM synthesis_run_attempts),
                         (SELECT COUNT(*) FROM synthesis_run_events),
                         (SELECT COUNT(*) FROM synthesis_output_insights),
                         (SELECT COUNT(*) FROM $source_class_relation),
                         (SELECT COUNT(*) FROM synthesis_citations),
                         (SELECT COUNT(*) FROM synthesis_insights),
                         (SELECT COUNT(*) FROM weekly_synthesis);
"
}

restore_and_verify_canonical_corpus_after_restart() {
    local noncanonical_artifacts
    local noncanonical_citations
    local history_before
    local history_after
    local corpus_counts

    noncanonical_artifacts="$(synthesis_psql_rows "
SELECT COUNT(*)
FROM artifacts
WHERE id NOT IN (
        't004-c15-restart-artifact-a',
        't004-c15-restart-artifact-b',
        '$KNOWN_ARTIFACT_ID'
);
")"
    assert_nonnegative_integer "$noncanonical_artifacts" "noncanonical_artifact_count"

    noncanonical_citations="$(synthesis_psql_rows "
SELECT COUNT(*)
FROM synthesis_citations
WHERE artifact_id NOT IN (
        't004-c15-restart-artifact-a',
        't004-c15-restart-artifact-b',
        '$KNOWN_ARTIFACT_ID'
);
")"
    assert_nonnegative_integer "$noncanonical_citations" "noncanonical_citation_count"
    if [[ "$noncanonical_citations" != "0" ]]; then
        e2e_fail "historical restart corpus restore refused noncanonical_citations=$noncanonical_citations"
    fi

    history_before="$(synthesis_history_snapshot)"
    synthesis_psql_command "
BEGIN;
LOCK TABLE artifacts, edges, synthesis_citations IN SHARE ROW EXCLUSIVE MODE;
DO \$guard\$
BEGIN
        IF EXISTS (
                SELECT 1
                FROM synthesis_citations
                WHERE artifact_id NOT IN (
                        't004-c15-restart-artifact-a',
                        't004-c15-restart-artifact-b',
                        '$KNOWN_ARTIFACT_ID'
                )
        ) THEN
                RAISE EXCEPTION 'noncanonical synthesis citations prevent corpus restoration';
        END IF;
END;
\$guard\$;
DELETE FROM edges
WHERE (src_type = 'artifact' AND src_id NOT IN (
                     't004-c15-restart-artifact-a',
                     't004-c15-restart-artifact-b',
                     '$KNOWN_ARTIFACT_ID'
             ))
     OR (dst_type = 'artifact' AND dst_id NOT IN (
                     't004-c15-restart-artifact-a',
                     't004-c15-restart-artifact-b',
                     '$KNOWN_ARTIFACT_ID'
             ));
DELETE FROM artifacts
WHERE id NOT IN (
        't004-c15-restart-artifact-a',
        't004-c15-restart-artifact-b',
        '$KNOWN_ARTIFACT_ID'
);
COMMIT;
"
    history_after="$(synthesis_history_snapshot)"
    if [[ "$history_after" != "$history_before" ]]; then
        e2e_fail "historical restart corpus restore changed synthesis history"
    fi

    corpus_counts="$(synthesis_psql_rows "
SELECT (SELECT COUNT(*) FROM artifacts),
                         (SELECT COUNT(*) FROM artifacts WHERE id IN (
                                 't004-c15-restart-artifact-a',
                                 't004-c15-restart-artifact-b',
                                 '$KNOWN_ARTIFACT_ID'
                         )),
                         (SELECT COUNT(DISTINCT source_id) FROM artifacts),
                         (SELECT COUNT(*) FROM edges
                                WHERE src_type = 'artifact'
                                        AND edge_type = 'BELONGS_TO'
                                        AND src_id IN (
                                                't004-c15-restart-artifact-a',
                                                't004-c15-restart-artifact-b',
                                                '$KNOWN_ARTIFACT_ID'
                                        ));
")"
    if [[ "$corpus_counts" != "3|3|3|3" ]]; then
        e2e_fail "historical restart canonical corpus status=invalid counts=$corpus_counts"
    fi
    echo "RESTORE: canonical_artifacts=3 distinct_sources=3 canonical_edges=3 removed_noncanonical_artifacts=$noncanonical_artifacts synthesis_history_preserved=true"
}

install_readback_relation_failure() {
  synthesis_psql_command "
CREATE FUNCTION e2e_c15_break_synthesis_readback_relation()
RETURNS TRIGGER LANGUAGE plpgsql AS \$fault\$
BEGIN
    ALTER TABLE synthesis_output_source_classes
        RENAME TO synthesis_output_source_classes_readback_fault;
    RETURN NEW;
END;
\$fault\$;
CREATE TRIGGER e2e_c15_break_synthesis_readback_relation
AFTER INSERT ON synthesis_citations
FOR EACH ROW WHEN (NEW.artifact_id = '$KNOWN_ARTIFACT_ID')
EXECUTE FUNCTION e2e_c15_break_synthesis_readback_relation();
"
    echo "FAULT: profile=$FAULT_PROFILE_ID status=active boundary=production_readback_relation"
}

request_retry() {
    local response

    if ! response="$(curl -sS --max-time 60 -X POST "$CORE_URL/api/synthesis/retry" \
        -H "Authorization: Bearer $AUTH_TOKEN" \
        -H 'Content-Type: application/json' \
        -d '{"cadence":"daily"}' \
        -w $'\n%{http_code}')"; then
        e2e_fail "T004-C15 retry transport status=failed"
    fi
    HTTP_STATUS="${response##*$'\n'}"
    HTTP_BODY="${response%$'\n'*}"
    if [[ ! "$HTTP_STATUS" =~ ^[0-9]{3}$ ]]; then
        e2e_fail "T004-C15 retry transport status=invalid_http_code"
    fi
}

request_authenticated_get() {
    local path="${1:?request path required}"
    local response

    if ! response="$(curl -sS --max-time 30 \
        -H "Authorization: Bearer $AUTH_TOKEN" \
        "$CORE_URL$path" \
        -w $'\n%{http_code}')"; then
        e2e_fail "T004-C15 authenticated read transport status=failed"
    fi
    HTTP_STATUS="${response##*$'\n'}"
    HTTP_BODY="${response%$'\n'*}"
    if [[ ! "$HTTP_STATUS" =~ ^[0-9]{3}$ ]]; then
        e2e_fail "T004-C15 authenticated read transport status=invalid_http_code"
    fi
}

assert_safe_retry_failure() {
    local failure_code
    local safe_message
    local safe_shape
    local forbidden

    request_retry
    if [[ "$HTTP_STATUS" == 2* ]]; then
        e2e_fail "T004-C15 read-back fault returned false success status=$HTTP_STATUS"
    fi
    if [[ "$HTTP_STATUS" != "500" ]]; then
        e2e_fail "T004-C15 read-back fault status=$HTTP_STATUS want=500"
    fi
    if ! failure_code="$(jq -er '.error.code | select(type == "string" and length > 0)' <<<"$HTTP_BODY")"; then
        e2e_fail "T004-C15 read-back fault response error_code=missing"
    fi
    if [[ "$failure_code" != "synthesis_retry_failed" ]]; then
        e2e_fail "T004-C15 read-back fault error_code=$failure_code want=synthesis_retry_failed"
    fi
    if ! safe_message="$(jq -er '.error.message | select(. == "Synthesis run failed")' <<<"$HTTP_BODY")" \
        || [[ "$safe_message" != "Synthesis run failed" ]]; then
        e2e_fail "T004-C15 read-back fault response message=unsafe"
    fi
    if ! safe_shape="$(jq -er '((has("outcome") or has("output") or has("delivered") or has("delivery") or has("healthy") or has("persisted") or has("recovered")) | not)' <<<"$HTTP_BODY")" \
        || [[ "$safe_shape" != "true" ]]; then
        e2e_fail "T004-C15 read-back fault response shape=false_success"
    fi
    for forbidden in "$KNOWN_ARTIFACT_ID" "synthesis_output_source_classes" "forced read-back" "PGPASSWORD" "$AUTH_TOKEN"; do
        if [[ "$HTTP_BODY" == *"$forbidden"* ]]; then
            e2e_fail "T004-C15 read-back fault response leaked a disallowed marker"
        fi
    done
    echo "OBSERVE: first_retry http_status=$HTTP_STATUS error_code=$failure_code success_fields=0"
}

assert_nonnegative_integer() {
    local value="${1:?integer value required}"
    local label="${2:?integer label required}"

    if [[ ! "$value" =~ ^[0-9]+$ ]]; then
        e2e_fail "T004-C15 $label status=non_numeric"
    fi
}

load_failure_snapshot() {
    local row="${1:?failure snapshot required}"

    IFS='|' read -r FAIL_OUTPUT_ID FAIL_RUN_ID FAIL_ATTEMPT_NO \
        FAIL_OUTPUT_LIFECYCLE FAIL_RUN_LIFECYCLE FAIL_RUN_STATE \
        FAIL_ATTEMPT_STATE FAIL_ATTEMPT_OUTCOME FAIL_TOTAL_OUTPUTS \
        FAIL_CURRENT_OUTPUTS FAIL_READBACK_ATTEMPTS FAIL_STARTED_EVENTS \
        FAIL_READBACK_EVENTS FAIL_SUCCESS_EVENTS FAIL_RECOVERED_EVENTS \
        FAIL_KNOWN_CITATIONS FAIL_INSIGHT_ROWS FAIL_CITATION_ROWS <<<"$row"
}

query_failure_snapshot() {
    synthesis_psql_rows "
SELECT o.id,
             o.run_id,
             a.attempt_no,
             o.lifecycle_state,
             r.lifecycle_state,
             r.state,
             a.state,
             a.outcome,
             (SELECT COUNT(*) FROM synthesis_outputs),
             (SELECT COUNT(*) FROM synthesis_outputs WHERE lifecycle_state = 'current'),
             (SELECT COUNT(*) FROM synthesis_run_attempts WHERE state = 'readback_failed'),
             (SELECT COUNT(*) FROM synthesis_run_events e
                WHERE e.run_id = o.run_id AND e.attempt_no = a.attempt_no
                    AND e.event_type = 'attempt_started'),
             (SELECT COUNT(*) FROM synthesis_run_events e
                WHERE e.run_id = o.run_id AND e.attempt_no = a.attempt_no
                    AND e.output_id = o.id AND e.event_type = 'readback_failed'),
             (SELECT COUNT(*) FROM synthesis_run_events e
                WHERE e.run_id = o.run_id AND e.attempt_no = a.attempt_no
                    AND e.event_type IN ('idempotent', 'persisted', 'quiet', 'partial', 'recovered')),
             (SELECT COUNT(*) FROM synthesis_run_events WHERE event_type = 'recovered'),
             (SELECT COUNT(*) FROM synthesis_citations c
                JOIN synthesis_output_insights i ON i.id = c.insight_id
                WHERE i.output_id = o.id AND c.artifact_id = '$KNOWN_ARTIFACT_ID'),
             (SELECT COUNT(*) FROM synthesis_output_insights i WHERE i.output_id = o.id),
             (SELECT COUNT(*) FROM synthesis_citations c
                JOIN synthesis_output_insights i ON i.id = c.insight_id
                WHERE i.output_id = o.id)
FROM synthesis_outputs o
JOIN synthesis_runs r ON r.id = o.run_id
JOIN synthesis_run_attempts a
    ON a.run_id = o.run_id AND a.output_id = o.id
WHERE a.trigger_kind = 'operator_retry' AND a.state = 'readback_failed'
ORDER BY a.attempt_no DESC
LIMIT 1;
"
}

assert_failure_snapshot() {
    local relation_state
    local numeric_value
    local numeric_label

    if [[ -z "$FAIL_OUTPUT_ID" || -z "$FAIL_RUN_ID" ]]; then
        e2e_fail "T004-C15 forensic identity status=missing"
    fi
    for numeric_label in FAIL_ATTEMPT_NO FAIL_TOTAL_OUTPUTS FAIL_CURRENT_OUTPUTS \
        FAIL_READBACK_ATTEMPTS FAIL_STARTED_EVENTS FAIL_READBACK_EVENTS \
        FAIL_SUCCESS_EVENTS FAIL_RECOVERED_EVENTS FAIL_KNOWN_CITATIONS \
        FAIL_INSIGHT_ROWS FAIL_CITATION_ROWS; do
        numeric_value="${!numeric_label}"
        assert_nonnegative_integer "$numeric_value" "$numeric_label"
    done
    if ((FAIL_ATTEMPT_NO < 1)); then
        e2e_fail "T004-C15 attempt identity status=invalid"
    fi
    if [[ "$FAIL_OUTPUT_LIFECYCLE" != "superseded" || "$FAIL_RUN_LIFECYCLE" != "superseded" ||
        "$FAIL_RUN_STATE" != "failed" || "$FAIL_ATTEMPT_STATE" != "readback_failed" ||
        "$FAIL_ATTEMPT_OUTCOME" != "readback_failed" ]]; then
        e2e_fail "T004-C15 committed-unverified lifecycle status=invalid"
    fi
    if [[ "$FAIL_TOTAL_OUTPUTS" != "1" || "$FAIL_CURRENT_OUTPUTS" != "0" ||
        "$FAIL_READBACK_ATTEMPTS" != "1" || "$FAIL_STARTED_EVENTS" != "1" ||
        "$FAIL_READBACK_EVENTS" != "1" || "$FAIL_SUCCESS_EVENTS" != "0" ||
        "$FAIL_RECOVERED_EVENTS" != "0" ]]; then
        e2e_fail "T004-C15 committed-unverified event/count status=invalid"
    fi
    if [[ "$FAIL_KNOWN_CITATIONS" != "1" || "$FAIL_INSIGHT_ROWS" != "1" ||
        "$FAIL_CITATION_ROWS" != "3" ]]; then
        e2e_fail "T004-C15 committed content status=invalid"
    fi

    relation_state="$(synthesis_psql_rows "
SELECT (to_regclass('public.synthesis_output_source_classes') IS NULL)::int,
             (to_regclass('public.synthesis_output_source_classes_readback_fault') IS NOT NULL)::int;
")"
    if [[ "$relation_state" != "1|1" ]]; then
        e2e_fail "T004-C15 read-back relation fault status=inactive"
    fi
    echo "OBSERVE: forensic output_id=$FAIL_OUTPUT_ID run_id=$FAIL_RUN_ID attempt_no=$FAIL_ATTEMPT_NO lifecycle=superseded event=readback_failed known_citations=1"
}

assert_failure_not_surfaced_in_read_apis() {
    local stage="${1:?assertion stage required}"
    local latest_state
    local latest_has_output
    local history_count
    local detail_code

    request_authenticated_get "/api/synthesis/latest"
    if [[ "$HTTP_STATUS" != "200" ]]; then
        e2e_fail "T004-C15 stage=$stage latest status=$HTTP_STATUS want=200"
    fi
    if ! latest_state="$(jq -er '.state | select(type == "string" and length > 0)' <<<"$HTTP_BODY")"; then
        e2e_fail "T004-C15 stage=$stage latest state=missing"
    fi
    if ! latest_has_output="$(jq -r 'has("output")' <<<"$HTTP_BODY")"; then
        e2e_fail "T004-C15 stage=$stage latest shape=invalid"
    fi
    if [[ "$latest_state" != "never-run" || "$latest_has_output" != "false" ]]; then
        e2e_fail "T004-C15 stage=$stage latest surfaced committed-unverified output"
    fi

    request_authenticated_get "/api/synthesis/runs?limit=25"
    if [[ "$HTTP_STATUS" != "200" ]]; then
        e2e_fail "T004-C15 stage=$stage history status=$HTTP_STATUS want=200"
    fi
    if ! history_count="$(jq -er '.runs | arrays | length' <<<"$HTTP_BODY")"; then
        e2e_fail "T004-C15 stage=$stage history shape=invalid"
    fi
    if [[ "$history_count" != "0" ]]; then
        e2e_fail "T004-C15 stage=$stage history surfaced committed-unverified output"
    fi

    request_authenticated_get "/api/synthesis/runs/$FAIL_OUTPUT_ID"
    if [[ "$HTTP_STATUS" != "500" ]]; then
        e2e_fail "T004-C15 stage=$stage detail status=$HTTP_STATUS want=500"
    fi
    if ! detail_code="$(jq -er '.error.code | select(. == "synthesis_read_failed")' <<<"$HTTP_BODY")" \
        || [[ "$detail_code" != "synthesis_read_failed" ]]; then
        e2e_fail "T004-C15 stage=$stage detail failure=unsafe"
    fi

    echo "OBSERVE: stage=$stage latest_state=$latest_state latest_output=absent history_count=0 detail_status=500 detail_error_code=$detail_code"
}

assert_recovery_database_truth() {
    local row
    local output_lifecycle
    local run_lifecycle
    local run_state
    local readback_failed_count
    local recovered_count
    local readback_attempt_no
    local recovered_attempt_no
    local event_history
    local current_count
    local recovered_attempt_state
    local recovered_attempt_outcome
    local recovered_attempt_output
    local total_outputs
    local later_recovery_count
    local nonrecovery_success_count
    local numeric_value

    row="$(synthesis_psql_rows "
SELECT o.lifecycle_state,
             r.lifecycle_state,
             r.state,
             (SELECT COUNT(*) FROM synthesis_run_events e
                WHERE e.run_id = o.run_id AND e.output_id = o.id AND e.event_type = 'readback_failed'),
             (SELECT COUNT(*) FROM synthesis_run_events e
                WHERE e.run_id = o.run_id AND e.output_id = o.id AND e.event_type = 'recovered'),
             (SELECT MIN(attempt_no) FROM synthesis_run_events e
                WHERE e.run_id = o.run_id AND e.output_id = o.id AND e.event_type = 'readback_failed'),
             (SELECT MAX(attempt_no) FROM synthesis_run_events e
                WHERE e.run_id = o.run_id AND e.output_id = o.id AND e.event_type = 'recovered'),
             (SELECT STRING_AGG(event_type, ',' ORDER BY attempt_no, created_at, id)
                FROM synthesis_run_events e
                WHERE e.run_id = o.run_id AND e.output_id = o.id
                    AND e.event_type IN ('readback_failed', 'recovered')),
             (SELECT COUNT(*) FROM synthesis_outputs current_output
                WHERE current_output.principal = o.principal
                    AND current_output.cadence = o.cadence
                    AND current_output.window_start = o.window_start
                    AND current_output.window_end = o.window_end
                    AND current_output.lifecycle_state = 'current'),
             (SELECT state FROM synthesis_run_attempts a
                WHERE a.run_id = o.run_id AND a.output_id = o.id AND a.state = 'recovered'
                ORDER BY a.attempt_no DESC LIMIT 1),
             (SELECT outcome FROM synthesis_run_attempts a
                WHERE a.run_id = o.run_id AND a.output_id = o.id AND a.state = 'recovered'
                ORDER BY a.attempt_no DESC LIMIT 1),
             (SELECT output_id FROM synthesis_run_attempts a
                WHERE a.run_id = o.run_id AND a.output_id = o.id AND a.state = 'recovered'
                ORDER BY a.attempt_no DESC LIMIT 1),
             (SELECT COUNT(*) FROM synthesis_outputs),
             (SELECT COUNT(*) FROM synthesis_run_events failed_event
                JOIN synthesis_run_events recovered_event
                    ON recovered_event.run_id = failed_event.run_id
                 AND recovered_event.output_id = failed_event.output_id
                WHERE failed_event.event_type = 'readback_failed'
                    AND recovered_event.event_type = 'recovered'
                    AND recovered_event.attempt_no > failed_event.attempt_no
                    AND recovered_event.created_at >= failed_event.created_at
                    AND failed_event.output_id = o.id),
             (SELECT COUNT(*) FROM synthesis_run_events e
                WHERE e.run_id = o.run_id AND e.output_id = o.id
                    AND e.event_type IN ('idempotent', 'persisted', 'quiet', 'partial'))
FROM synthesis_outputs o
JOIN synthesis_runs r ON r.id = o.run_id
WHERE o.id = '$FAIL_OUTPUT_ID';
")"
    if [[ -z "$row" ]]; then
        e2e_fail "T004-C15 recovery database row=missing"
    fi
    IFS='|' read -r output_lifecycle run_lifecycle run_state \
        readback_failed_count recovered_count readback_attempt_no recovered_attempt_no \
        event_history current_count recovered_attempt_state recovered_attempt_outcome \
        recovered_attempt_output total_outputs later_recovery_count \
        nonrecovery_success_count <<<"$row"

    for numeric_value in "$readback_failed_count" "$recovered_count" "$readback_attempt_no" \
        "$recovered_attempt_no" "$current_count" "$total_outputs" \
        "$later_recovery_count" "$nonrecovery_success_count"; do
        assert_nonnegative_integer "$numeric_value" "recovery_count"
    done
    if [[ "$output_lifecycle" != "current" || "$run_lifecycle" != "current" ||
        "$run_state" != "succeeded" || "$readback_failed_count" != "1" ||
        "$recovered_count" != "1" || "$event_history" != "readback_failed,recovered" ||
        "$current_count" != "1" || "$total_outputs" != "1" ||
        "$later_recovery_count" != "1" || "$nonrecovery_success_count" != "0" ]]; then
        e2e_fail "T004-C15 recovered lifecycle/event status=invalid"
    fi
    if [[ "$readback_attempt_no" != "$FAIL_ATTEMPT_NO" ||
        "$recovered_attempt_no" != "$((FAIL_ATTEMPT_NO + 1))" ||
        "$recovered_attempt_state" != "recovered" ||
        "$recovered_attempt_outcome" != "recovered" ||
        "$recovered_attempt_output" != "$FAIL_OUTPUT_ID" ]]; then
        e2e_fail "T004-C15 recovered attempt linkage status=invalid"
    fi
    echo "OBSERVE: recovery output_id=$FAIL_OUTPUT_ID run_id=$FAIL_RUN_ID event_history=$event_history recovered_attempt_no=$recovered_attempt_no current_count=1"
}

assert_recovered_detail() {
    local detail_summary
    local detail_output_id
    local detail_kind
    local declared_insights
    local declared_citations
    local known_citation_occurrences
    local actual_insights
    local actual_citations
    local numeric_value

    request_authenticated_get "/api/synthesis/runs/$FAIL_OUTPUT_ID"
    if [[ "$HTTP_STATUS" != "200" ]]; then
        e2e_fail "T004-C15 recovered detail status=$HTTP_STATUS want=200"
    fi
    if ! detail_summary="$(jq -er --arg known "$KNOWN_ARTIFACT_ID" '
        [
            .output.outputId,
            .output.kind,
            .output.insightCount,
            .output.citationCount,
            ([.insights[].citations[]? | select(. == $known)] | length),
            (.insights | length),
            ([.insights[].citations[]?] | length)
        ] | @tsv
    ' <<<"$HTTP_BODY")"; then
        e2e_fail "T004-C15 recovered detail shape=invalid"
    fi
    IFS=$'\t' read -r detail_output_id detail_kind declared_insights \
        declared_citations known_citation_occurrences actual_insights \
        actual_citations <<<"$detail_summary"
    for numeric_value in "$declared_insights" "$declared_citations" \
        "$known_citation_occurrences" "$actual_insights" "$actual_citations"; do
        assert_nonnegative_integer "$numeric_value" "detail_count"
    done
    if [[ "$detail_output_id" != "$FAIL_OUTPUT_ID" || "$detail_kind" != "full" ||
        "$declared_insights" != "1" || "$declared_citations" != "3" ||
        "$known_citation_occurrences" != "1" || "$actual_insights" != "$declared_insights" ||
        "$actual_citations" != "$declared_citations" ]]; then
        e2e_fail "T004-C15 recovered detail coherence status=invalid"
    fi
    echo "OBSERVE: recovered_detail output_id=$detail_output_id kind=$detail_kind insights=$actual_insights citations=$actual_citations known_citations=1"
}

echo "=== T004-02-RESTART / T004-06-RECOVERY: durability across a real restart ==="

e2e_start
STACK_STARTED=1

synthesis_retry_output_id() {
    local body
    body="$(curl -fsS --max-time 60 -X POST "$CORE_URL/api/synthesis/retry" \
        -H "Authorization: Bearer $AUTH_TOKEN" \
        -H 'Content-Type: application/json' \
        -d '{"cadence":"daily"}')"
    jq -r '.output.outputId // ""' <<<"$body"
}

install_canonical_artifact_fence before_restart_retry
assert_canonical_artifact_fence_active before_restart_retry
reset_synthesis_state_and_seed_corpus
BEFORE_ID="$(synthesis_retry_output_id)"
if [[ -z "$BEFORE_ID" ]]; then
    echo "FAIL: first trigger produced no output id; there is no identity to carry across a restart"
    exit 1
fi
echo "committed output before restart: $BEFORE_ID"

BEFORE_RUN_KEY_FIELDS="$(durable_run_key_fields "$BEFORE_ID")"
IFS='|' read -r BEFORE_SOURCE_SET_DIGEST BEFORE_POLICY_VERSION <<<"$BEFORE_RUN_KEY_FIELDS"
if [[ ! "$BEFORE_SOURCE_SET_DIGEST" =~ ^[0-9a-f]{64}$ || -z "$BEFORE_POLICY_VERSION" ]]; then
  e2e_fail "historical restart durable run key fields status=invalid"
fi
BEFORE_SOURCE_SET_SNAPSHOT="$(eligible_source_set_snapshot)"
IFS='|' read -r BEFORE_SOURCE_COUNT BEFORE_SOURCE_SET_INPUT <<<"$BEFORE_SOURCE_SET_SNAPSHOT"
if [[ "$BEFORE_SOURCE_COUNT" != "3" || -z "$BEFORE_SOURCE_SET_INPUT" ]]; then
  e2e_fail "historical restart source-set precondition status=invalid count=$BEFORE_SOURCE_COUNT"
fi
BEFORE_COMPUTED_SOURCE_SET_DIGEST="$(sha256_for_source_set_input "$BEFORE_SOURCE_SET_INPUT")" \
  || e2e_fail "historical restart source-set digest tool status=unavailable"
if [[ "$BEFORE_COMPUTED_SOURCE_SET_DIGEST" != "$BEFORE_SOURCE_SET_DIGEST" ]]; then
  e2e_fail "historical restart stored source-set digest does not match eligible input"
fi
echo "OBSERVE: before_restart eligible_source_count=3 source_set_digest_match=true policy_version_recorded=true"

# Kill the process. Anything it held only in memory goes with it.
echo "--- restarting the core process ---"
remove_canonical_artifact_fence core_restart
smackerel_compose "$TEST_ENV" restart smackerel-core

# A single successful probe right after a restart proves nothing: the old
# process may still be holding the socket for a moment before it goes down.
# Requiring several consecutive successes, spaced out, means the probe cannot be
# satisfied by a listener that is about to disappear.
wait_for_stable_health 180
echo "core process restarted and serving again"

install_canonical_artifact_fence after_restart_retry
restore_and_verify_canonical_corpus_after_restart
AFTER_RESTART_SOURCE_SET_SNAPSHOT="$(eligible_source_set_snapshot)"
IFS='|' read -r AFTER_RESTART_SOURCE_COUNT AFTER_RESTART_SOURCE_SET_INPUT <<<"$AFTER_RESTART_SOURCE_SET_SNAPSHOT"
AFTER_RESTART_COMPUTED_DIGEST="$(sha256_for_source_set_input "$AFTER_RESTART_SOURCE_SET_INPUT")" \
    || e2e_fail "historical restart source-set digest tool status=unavailable"
if [[ "$AFTER_RESTART_SOURCE_COUNT" != "$BEFORE_SOURCE_COUNT" ||
    "$AFTER_RESTART_SOURCE_SET_INPUT" != "$BEFORE_SOURCE_SET_INPUT" ||
    "$AFTER_RESTART_COMPUTED_DIGEST" != "$BEFORE_SOURCE_SET_DIGEST" ]]; then
    e2e_fail "historical restart eligible source-set digest input changed across restart"
fi
echo "OBSERVE: after_restart eligible_source_count=3 digest_input_equal=true source_set_digest_match=true"

# T004-02-RESTART: the same window must resolve to the same durable identity.
assert_canonical_artifact_fence_active after_restart_retry
AFTER_ID="$(synthesis_retry_output_id)"
if [[ -z "$AFTER_ID" ]]; then
  echo "FAIL: trigger after restart produced no output id; the window lost its identity"
  exit 1
fi
AFTER_RUN_KEY_FIELDS="$(durable_run_key_fields "$AFTER_ID")"
IFS='|' read -r AFTER_SOURCE_SET_DIGEST AFTER_POLICY_VERSION <<<"$AFTER_RUN_KEY_FIELDS"
if [[ "$AFTER_SOURCE_SET_DIGEST" != "$BEFORE_SOURCE_SET_DIGEST" ||
    "$AFTER_POLICY_VERSION" != "$BEFORE_POLICY_VERSION" ]]; then
    e2e_fail "historical restart durable run source-set digest or policy version changed"
fi
echo "OBSERVE: after_retry source_set_digest_equal=true policy_version_equal=true"
if [[ "$AFTER_ID" != "$BEFORE_ID" ]]; then
  echo "FAIL: the same window resolved to $BEFORE_ID before the restart and $AFTER_ID after it;"
  echo "      identity did not survive the process, so it was never durable"
  exit 1
fi
echo "PASS: T004-02-RESTART — window identity $BEFORE_ID survived a real process restart"

# T004-06-RECOVERY: the read path must recover what happened from storage.
LATEST="$(curl -fsS --max-time 30 "$CORE_URL/api/synthesis/latest" \
  -H "Authorization: Bearer $AUTH_TOKEN")"
LATEST_STATE="$(jq -r '.state // ""' <<<"$LATEST")"
LATEST_ID="$(jq -r '.output.outputId // ""' <<<"$LATEST")"

if [[ "$LATEST_STATE" == "never-run" ]]; then
  echo "FAIL: output $BEFORE_ID was committed before the restart yet latest now reports never-run;"
  echo "      the read path is answering from lost memory instead of storage"
  exit 1
fi
if [[ -z "$LATEST_ID" ]]; then
  echo "FAIL: state '$LATEST_STATE' carried no output after the restart despite $BEFORE_ID being committed"
  exit 1
fi
if [[ "$LATEST_ID" != "$BEFORE_ID" ]]; then
  echo "FAIL: latest names $LATEST_ID after the restart but $BEFORE_ID was committed before it;"
  echo "      recovery resolved to a different run"
  exit 1
fi

# The history surface must agree with the single-output view, otherwise recovery
# is only half restored and the two surfaces tell a reader different stories.
RUNS="$(curl -fsS --max-time 30 "$CORE_URL/api/synthesis/runs?limit=25" \
  -H "Authorization: Bearer $AUTH_TOKEN")"
HISTORY_HAS_ID="$(jq -er --arg id "$BEFORE_ID" '.runs | map(.outputId) | index($id) != null' <<<"$RUNS")" \
  || e2e_fail "historical run history response shape=invalid"
if [[ "$HISTORY_HAS_ID" != "true" ]]; then
  echo "FAIL: output $BEFORE_ID survived into latest but is absent from run history after the restart;"
  echo "      the two read surfaces disagree"
  exit 1
fi
echo "PASS: T004-06-RECOVERY — health and history both recovered $BEFORE_ID from storage (state '$LATEST_STATE')"

remove_canonical_artifact_fence historical_assertions_complete
echo "=== historical durability identity/recovery checks: complete ==="
echo "=== T004-C15-RESTART: committed-unverified durability and verified recovery ==="
assert_canonical_fault_profile
install_canonical_artifact_fence corrective_faulted_retry
reset_synthesis_state_and_seed_corpus
install_readback_relation_failure
assert_canonical_artifact_fence_active corrective_faulted_retry
assert_safe_retry_failure

FAILURE_SNAPSHOT_BEFORE_RESTART="$(query_failure_snapshot)"
if [[ -z "$FAILURE_SNAPSHOT_BEFORE_RESTART" ]]; then
    e2e_fail "T004-C15 committed-unverified snapshot=missing"
fi
load_failure_snapshot "$FAILURE_SNAPSHOT_BEFORE_RESTART"
assert_failure_snapshot
assert_failure_not_surfaced_in_read_apis before_restart

echo "--- restarting the actual core with the read-back relation still broken ---"
remove_canonical_artifact_fence corrective_core_restart
smackerel_compose "$TEST_ENV" restart smackerel-core
wait_for_stable_health 180

install_canonical_artifact_fence corrective_repaired_retry
restore_and_verify_canonical_corpus_after_restart
FAILURE_SNAPSHOT_AFTER_RESTART="$(query_failure_snapshot)"
if [[ "$FAILURE_SNAPSHOT_AFTER_RESTART" != "$FAILURE_SNAPSHOT_BEFORE_RESTART" ]]; then
    e2e_fail "T004-C15 committed-unverified identity/counts changed across restart"
fi
load_failure_snapshot "$FAILURE_SNAPSHOT_AFTER_RESTART"
assert_failure_snapshot
assert_failure_not_surfaced_in_read_apis after_restart
echo "PASS: T004-C15 committed-unverified output and readback_failed event survived a real core restart"

repair_readback_relation
echo "FAULT: profile=$FAULT_PROFILE_ID status=repaired"
assert_canonical_artifact_fence_active corrective_repaired_retry
request_retry
if [[ "$HTTP_STATUS" != "200" ]]; then
  e2e_fail "T004-C15 repaired retry status=$HTTP_STATUS want=200"
fi
RECOVERY_RESPONSE="$(jq -er '[.outcome, .output.outputId] | @tsv' <<<"$HTTP_BODY")" \
  || e2e_fail "T004-C15 repaired retry response shape=invalid"
IFS=$'\t' read -r RECOVERY_API_OUTCOME RECOVERY_API_OUTPUT_ID <<<"$RECOVERY_RESPONSE"
if [[ "$RECOVERY_API_OUTCOME" != "persisted" || "$RECOVERY_API_OUTPUT_ID" != "$FAIL_OUTPUT_ID" ]]; then
  e2e_fail "T004-C15 repaired retry API identity/outcome status=invalid"
fi
echo "OBSERVE: repaired_retry http_status=200 api_outcome=$RECOVERY_API_OUTCOME output_id=$RECOVERY_API_OUTPUT_ID"

assert_recovery_database_truth
assert_recovered_detail
remove_canonical_artifact_fence corrective_assertions_complete
echo "PASS: T004-C15 recovered was appended only after production read-back verified the same forensic output"

TEST_PASS_READY=1

echo "=== durability across a real restart: complete ==="
