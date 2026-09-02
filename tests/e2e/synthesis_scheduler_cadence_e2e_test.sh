#!/usr/bin/env bash
# BUG-004-004 corrective SCOPE-03A — T004-C11-WEEKLY.
#
# Proves both SST-configured scheduler cadences execute through the real core,
# persist one causal attempt/event chain, and read back through the mounted
# authenticated synthesis API before this test reports cadence completion.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=tests/e2e/lib/helpers.sh
source "$SCRIPT_DIR/lib/helpers.sh"

OVERLAY_FILE="$REPO_DIR/docker-compose.synthesis-cadence-e2e.override.yml"
overlay_active=0
test_pass_ready=0

wait_for_stable_core() {
  local max_seconds="${1:?maximum wait seconds required}"
  local required_streak=3
  local streak=0
  local started_at=$SECONDS
  local health_response=""
  local postgres_response=""

  echo "WAIT: core readiness status=starting required_streak=$required_streak max_seconds=$max_seconds"
  while ((SECONDS - started_at < max_seconds)); do
    if health_response="$(e2e_health_response 2>&1)" \
      && e2e_health_payload_ready "$health_response" \
      && postgres_response="$(e2e_postgres_select_one 2>&1)" \
      && [[ "$postgres_response" == "1" ]]; then
      streak=$((streak + 1))
      if ((streak >= required_streak)); then
        echo "READY: core status=stable streak=$streak"
        return 0
      fi
    else
      streak=0
    fi
    sleep 2
  done

  echo "FAIL: core readiness status=unstable streak=$streak max_seconds=$max_seconds"
  return 1
}

restore_base_core() {
  unset SMACKEREL_COMPOSE_OVERRIDE_FILE
  echo "RESTORE: core compose_model=base status=starting"
  if ! smackerel_compose "$TEST_ENV" up -d --no-deps --force-recreate smackerel-core; then
    echo "FAIL: core compose_model=base status=recreate_failed"
    return 1
  fi
  if ! wait_for_stable_core 60; then
    return 1
  fi
  echo "RESTORE: core compose_model=base status=ready"
}

cleanup() {
  local original_status="${1:?original status required}"
  local restore_status=0
  local teardown_status=0
  local final_status="$original_status"

  trap - EXIT INT TERM HUP
  set +e
  if [[ "$overlay_active" == "1" ]]; then
    restore_base_core
    restore_status=$?
  else
    unset SMACKEREL_COMPOSE_OVERRIDE_FILE
  fi
  e2e_cleanup
  teardown_status=$?

  if [[ "$final_status" == "0" && "$restore_status" != "0" ]]; then
    final_status="$restore_status"
  fi
  if [[ "$final_status" == "0" && "$teardown_status" != "0" ]]; then
    final_status="$teardown_status"
  fi
  if [[ "$final_status" == "0" && "$test_pass_ready" == "1" ]]; then
    e2e_pass "synthesis scheduler cadence live contract complete"
  fi
  exit "$final_status"
}

trap 'cleanup "$?"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

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

reset_synthesis_tables() {
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
"
  echo "RESET: synthesis persistence status=empty"
}

query_invariant_counts() {
  synthesis_psql_rows "
WITH event_counts AS (
  SELECT a.run_id,
         a.attempt_no,
         a.state,
         COUNT(e.id) FILTER (WHERE e.event_type = 'attempt_started') AS started_count,
         COUNT(e.id) FILTER (WHERE e.event_type IN (
           'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
           'retryable_failure', 'failed', 'readback_failed', 'recovered'
         )) AS terminal_count
  FROM synthesis_run_attempts a
  LEFT JOIN synthesis_run_events e
    ON e.run_id = a.run_id AND e.attempt_no = a.attempt_no
  WHERE a.run_id IS NOT NULL AND a.attempt_no IS NOT NULL
  GROUP BY a.run_id, a.attempt_no, a.state
), pair_mismatches AS (
  SELECT COUNT(*) AS mismatch_count
  FROM synthesis_run_events e
  JOIN synthesis_run_attempts a
    ON a.run_id = e.run_id AND a.attempt_no = e.attempt_no
  LEFT JOIN synthesis_outputs attempt_output ON attempt_output.id = a.output_id
  LEFT JOIN synthesis_outputs event_output ON event_output.id = e.output_id
  WHERE e.event_type IN (
    'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
    'retryable_failure', 'failed', 'readback_failed', 'recovered'
  )
    AND (
      e.output_id IS DISTINCT FROM a.output_id
      OR (a.output_id IS NOT NULL AND
          (attempt_output.id IS NULL OR attempt_output.run_id <> a.run_id))
      OR (e.output_id IS NOT NULL AND
          (event_output.id IS NULL OR event_output.run_id <> e.run_id))
    )
)
SELECT (SELECT COUNT(*) FROM synthesis_run_attempts WHERE trigger_kind = 'operator_retry'),
       (SELECT COUNT(*) FROM event_counts
        WHERE started_count > 1
           OR terminal_count > 1
           OR (state <> 'running' AND (started_count <> 1 OR terminal_count <> 1))),
       (SELECT mismatch_count FROM pair_mismatches);
"
}

assert_synthesis_invariants() {
  local counts=""
  local operator_retry_count=""
  local malformed_chain_count=""
  local cross_pair_count=""

  counts="$(query_invariant_counts)"
  IFS='|' read -r operator_retry_count malformed_chain_count cross_pair_count <<<"$counts"

  if [[ ! "$operator_retry_count" =~ ^[0-9]+$ ||
    ! "$malformed_chain_count" =~ ^[0-9]+$ ||
    ! "$cross_pair_count" =~ ^[0-9]+$ ]]; then
    e2e_fail "synthesis invariant query status=invalid"
  fi
  if ((operator_retry_count != 0)); then
    e2e_fail "synthesis trigger status=unexpected operator_retry_count=$operator_retry_count"
  fi
  if ((malformed_chain_count != 0)); then
    e2e_fail "synthesis event chain status=invalid count=$malformed_chain_count"
  fi
  if ((cross_pair_count != 0)); then
    e2e_fail "synthesis run_attempt_output pairing status=invalid count=$cross_pair_count"
  fi
}

query_verified_scheduled_attempts() {
  synthesis_psql_rows "
WITH event_counts AS (
  SELECT r.cadence,
         a.run_id,
         a.attempt_no,
         a.trigger_kind,
         a.output_id AS attempt_output_id,
         COUNT(e.id) FILTER (WHERE e.event_type = 'attempt_started') AS started_count,
         COUNT(e.id) FILTER (WHERE e.event_type IN (
           'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
           'retryable_failure', 'failed', 'readback_failed', 'recovered'
         )) AS terminal_count,
         COALESCE(MAX(e.event_type) FILTER (WHERE e.event_type IN (
           'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
           'retryable_failure', 'failed', 'readback_failed', 'recovered'
         )), '') AS terminal_type,
         COALESCE(MAX(e.output_id) FILTER (WHERE e.event_type IN (
           'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
           'retryable_failure', 'failed', 'readback_failed', 'recovered'
         )), '') AS terminal_output_id,
         MAX(e.created_at) FILTER (WHERE e.event_type IN (
           'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
           'retryable_failure', 'failed', 'readback_failed', 'recovered'
         )) AS terminal_at
  FROM synthesis_run_attempts a
  JOIN synthesis_runs r ON r.id = a.run_id
  LEFT JOIN synthesis_run_events e
    ON e.run_id = a.run_id AND e.attempt_no = a.attempt_no
  WHERE a.run_id IS NOT NULL AND a.attempt_no IS NOT NULL
  GROUP BY r.cadence, a.run_id, a.attempt_no, a.trigger_kind, a.output_id
)
SELECT cadence,
       run_id,
       attempt_no,
       attempt_output_id,
       terminal_type,
       terminal_output_id,
       started_count,
       terminal_count
FROM event_counts
WHERE trigger_kind = 'scheduled'
  AND attempt_output_id IS NOT NULL
  AND started_count = 1
  AND terminal_count = 1
  AND terminal_output_id = attempt_output_id
  AND terminal_type IN ('idempotent', 'persisted', 'quiet', 'partial', 'recovered')
ORDER BY terminal_at DESC, cadence, run_id DESC;
"
}

DAILY_RUN_ID=""
DAILY_ATTEMPT_NO=""
DAILY_OUTPUT_ID=""
DAILY_EVENT_TYPE=""
WEEKLY_RUN_ID=""
WEEKLY_ATTEMPT_NO=""
WEEKLY_OUTPUT_ID=""
WEEKLY_EVENT_TYPE=""

wait_for_both_cadences() {
  local max_seconds="${1:?maximum wait seconds required}"
  local started_at=$SECONDS
  local rows=""
  local cadence=""
  local run_id=""
  local attempt_no=""
  local attempt_output_id=""
  local terminal_type=""
  local terminal_output_id=""
  local started_count=""
  local terminal_count=""

  echo "WAIT: synthesis cadences status=observing max_seconds=$max_seconds"
  while ((SECONDS - started_at < max_seconds)); do
    assert_synthesis_invariants
    rows="$(query_verified_scheduled_attempts)"
    while IFS='|' read -r cadence run_id attempt_no attempt_output_id terminal_type terminal_output_id started_count terminal_count; do
      [[ -n "$cadence" ]] || continue
      if [[ "$started_count" != "1" || "$terminal_count" != "1" ||
        -z "$attempt_output_id" || "$terminal_output_id" != "$attempt_output_id" ]]; then
        e2e_fail "synthesis cadence=$cadence causal_chain status=invalid"
      fi
      case "$terminal_type" in
        idempotent | persisted | quiet | partial | recovered) ;;
        *) e2e_fail "synthesis cadence=$cadence terminal_event status=invalid" ;;
      esac
      case "$cadence" in
        daily)
          if [[ -z "$DAILY_OUTPUT_ID" ]]; then
            DAILY_RUN_ID="$run_id"
            DAILY_ATTEMPT_NO="$attempt_no"
            DAILY_OUTPUT_ID="$attempt_output_id"
            DAILY_EVENT_TYPE="$terminal_type"
            echo "OBSERVE: cadence=daily run_id=$DAILY_RUN_ID attempt_no=$DAILY_ATTEMPT_NO output_id=$DAILY_OUTPUT_ID started=1 terminal=1 event=$DAILY_EVENT_TYPE"
          fi
          ;;
        weekly)
          if [[ -z "$WEEKLY_OUTPUT_ID" ]]; then
            WEEKLY_RUN_ID="$run_id"
            WEEKLY_ATTEMPT_NO="$attempt_no"
            WEEKLY_OUTPUT_ID="$attempt_output_id"
            WEEKLY_EVENT_TYPE="$terminal_type"
            echo "OBSERVE: cadence=weekly run_id=$WEEKLY_RUN_ID attempt_no=$WEEKLY_ATTEMPT_NO output_id=$WEEKLY_OUTPUT_ID started=1 terminal=1 event=$WEEKLY_EVENT_TYPE"
          fi
          ;;
        *) e2e_fail "synthesis cadence status=unexpected" ;;
      esac
    done <<<"$rows"

    if [[ -n "$DAILY_OUTPUT_ID" && -n "$WEEKLY_OUTPUT_ID" ]]; then
      assert_synthesis_invariants
      if [[ "$DAILY_RUN_ID" == "$WEEKLY_RUN_ID" || "$DAILY_OUTPUT_ID" == "$WEEKLY_OUTPUT_ID" ]]; then
        e2e_fail "synthesis cadence identity status=cross_paired"
      fi
      return 0
    fi
    sleep 2
  done

  assert_synthesis_invariants
  if [[ -z "$DAILY_OUTPUT_ID" ]]; then
    e2e_fail "synthesis cadence=daily status=missing"
  fi
  e2e_fail "synthesis cadence=weekly status=missing"
}

assert_authenticated_readback() {
  local cadence="${1:?cadence required}"
  local persisted_output_id="${2:?persisted output id required}"
  local response=""
  local http_status=""
  local http_body=""
  local response_output_id=""

  response="$(curl -sS --max-time 15 \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    -w $'\n%{http_code}' \
    "$CORE_URL/api/synthesis/runs/$persisted_output_id")"
  http_status="${response##*$'\n'}"
  http_body="${response%$'\n'*}"

  if [[ "$http_status" != "200" ]]; then
    e2e_fail "synthesis cadence=$cadence output_id=$persisted_output_id http_status=$http_status"
  fi
  if ! response_output_id="$(jq -er '.output.outputId | select(type == "string" and length > 0)' <<<"$http_body")"; then
    e2e_fail "synthesis cadence=$cadence output_id=$persisted_output_id response_status=invalid"
  fi
  if [[ "$response_output_id" != "$persisted_output_id" ]]; then
    e2e_fail "synthesis cadence=$cadence output_pair status=mismatch"
  fi

  e2e_pass "cadence=$cadence output_id=$persisted_output_id http_status=200 readback=matched"
}

echo "START: synthesis scheduler cadence live contract"
e2e_start
reset_synthesis_tables

if [[ ! -f "$OVERLAY_FILE" ]]; then
  e2e_fail "synthesis cadence overlay status=missing"
fi
export SMACKEREL_COMPOSE_OVERRIDE_FILE="$OVERLAY_FILE"
overlay_active=1
smackerel_compose "$TEST_ENV" up -d --no-deps --force-recreate smackerel-core
wait_for_stable_core 60
wait_for_both_cadences 120

assert_authenticated_readback daily "$DAILY_OUTPUT_ID"
assert_authenticated_readback weekly "$WEEKLY_OUTPUT_ID"
assert_synthesis_invariants

test_pass_ready=1
