#!/usr/bin/env bash
# BUG-004-004 SCOPE-05 — every durable synthesis state renders exclusively.
#
# The browser suite proves the rendering path works for whatever state the live
# stack happens to be in. That leaves the states a running system rarely reaches
# on demand -- stale, partial, and both failure shapes -- asserted nowhere.
#
# This drives the database into each durable state in turn and asserts what the
# server actually renders. The states are produced through the same columns the
# read model reads, so a change to that mapping shows up here as a wrong state
# rather than as a test that quietly stops covering anything.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

echo "=== SCN-004-004-05: durable synthesis state matrix ==="

e2e_start

env_file="$(smackerel_require_env_file "$TEST_ENV")"
SYNTHESIS_ACTOR_USER_ID="$(smackerel_env_value "$env_file" "SYNTHESIS_ACTOR_USER_ID")"
: "${SYNTHESIS_ACTOR_USER_ID:?SYNTHESIS_ACTOR_USER_ID must be non-empty in the generated test environment}"

synthesis_psql() {
    local sql="${1:?SQL command required}"
    shift

    # Feed the template through psql's file input so :'name' variables are
    # expanded and quoted by psql instead of interpolated by the shell.
    printf '%s\n' "$sql" |
        smackerel_compose "$TEST_ENV" exec -T postgres \
            env PGPASSWORD="$POSTGRES_PASSWORD" \
            psql -h 127.0.0.1 -p "$POSTGRES_CONTAINER_PORT" \
            -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 \
            "$@" -q --file=-
}

synthesis_psql_command() {
    synthesis_psql "$@" >/dev/null
}

reset_synthesis() {
        # Events are immutable under UPDATE/DELETE and hold restrictive foreign
        # keys to attempts and outputs. Truncating the event ledger first in one
    # CASCADE operation clears the causal graph without attempting a
    # forbidden event mutation.
        synthesis_psql_command "
TRUNCATE synthesis_run_events, synthesis_citations, synthesis_output_insights, synthesis_output_source_classes,
         synthesis_outputs, synthesis_run_attempts, synthesis_runs RESTART IDENTITY CASCADE;
"
}

# seed_output <quiet|current|stale|partial|prior>
#
# Every output is a completed causal chain: succeeded run -> migration-067
# output identity -> linked attempt -> immutable attempt_started + matching
# terminal event. Fixture identities are selected from this closed case rather
# than accepted from user input. Selected identities and the configured actor
# reach SQL only through psql's literal-quoting variables.
seed_output() {
        local fixture="${1:?output fixture required}"
        local run_id logical_key output_id insight_id artifact_id source_set_digest
        local kind terminal_event lifecycle_state insight_count omitted_class
        local age_minutes window_age_days

        case "$fixture" in
                quiet)
                        run_id="run-quiet"
                        logical_key="key-quiet"
                        output_id="out-quiet"
                        insight_id="ins-quiet"
                        artifact_id="artifact-quiet"
                        source_set_digest="source-set-quiet"
                        kind="quiet"
                        terminal_event="quiet"
                        lifecycle_state="current"
                        insight_count=0
                        omitted_class=""
                        age_minutes=5
                        window_age_days=0
                        ;;
                current)
                        run_id="run-current"
                        logical_key="key-current"
                        output_id="out-current"
                        insight_id="ins-current"
                        artifact_id="artifact-current"
                        source_set_digest="source-set-current"
                        kind="full"
                        terminal_event="persisted"
                        lifecycle_state="current"
                        insight_count=1
                        omitted_class=""
                        age_minutes=5
                        window_age_days=0
                        ;;
                stale)
                        run_id="run-stale"
                        logical_key="key-stale"
                        output_id="out-stale"
                        insight_id="ins-stale"
                        artifact_id="artifact-stale"
                        source_set_digest="source-set-stale"
                        kind="full"
                        terminal_event="persisted"
                        lifecycle_state="stale"
                        insight_count=1
                        omitted_class=""
                        # Keep this deliberately generic until Scope 8 introduces a
                        # cadence-specific synthesis freshness SST contract.
                        age_minutes=$((400 * 24 * 60))
                        window_age_days=400
                        ;;
                partial)
                        run_id="run-partial"
                        logical_key="key-partial"
                        output_id="out-partial"
                        insight_id="ins-partial"
                        artifact_id="artifact-partial"
                        source_set_digest="source-set-partial"
                        kind="partial"
                        terminal_event="partial"
                        lifecycle_state="current"
                        insight_count=1
                        omitted_class="transcript"
                        age_minutes=5
                        window_age_days=0
                        ;;
                prior)
                        run_id="run-prior"
                        logical_key="key-prior"
                        output_id="out-prior"
                        insight_id="ins-prior"
                        artifact_id="artifact-prior"
                        source_set_digest="source-set-prior"
                        kind="full"
                        terminal_event="persisted"
                        lifecycle_state="current"
                        insight_count=1
                        omitted_class=""
                        age_minutes=30
                        window_age_days=0
                        ;;
                *)
                        echo "FAIL: unknown synthesis output fixture '$fixture'" >&2
                        exit 1
                        ;;
        esac

        synthesis_psql_command "
BEGIN;
INSERT INTO synthesis_runs
  (id, logical_key, cadence, principal, window_start, window_end, policy_version,
     source_set_digest, state, created_at, updated_at, lifecycle_state, attempt_count,
     lease_holder, lease_expires_at)
VALUES
    (:'run_id', :'logical_key', 'daily', :'actor',
     date_trunc('day', CURRENT_TIMESTAMP - make_interval(days => (:'window_age_days')::integer)) - INTERVAL '2 days',
     date_trunc('day', CURRENT_TIMESTAMP - make_interval(days => (:'window_age_days')::integer)) - INTERVAL '1 day',
     'matrix-policy-v1', :'source_set_digest', 'succeeded',
     CURRENT_TIMESTAMP - make_interval(mins => (:'age_minutes')::integer) - INTERVAL '1 minute',
     CURRENT_TIMESTAMP - make_interval(mins => (:'age_minutes')::integer),
     :'lifecycle_state', 1, NULL, NULL);
INSERT INTO synthesis_outputs
    (id, run_id, insight_count, citation_count, created_at, output_kind,
     evaluated_artifact_count, principal, cadence, window_start, window_end,
     lifecycle_state, superseded_at)
VALUES
    (:'output_id', :'run_id', (:'insight_count')::integer,
     (:'insight_count')::integer,
     CURRENT_TIMESTAMP - make_interval(mins => (:'age_minutes')::integer),
     :'kind', 7, :'actor', 'daily',
     date_trunc('day', CURRENT_TIMESTAMP - make_interval(days => (:'window_age_days')::integer)) - INTERVAL '2 days',
     date_trunc('day', CURRENT_TIMESTAMP - make_interval(days => (:'window_age_days')::integer)) - INTERVAL '1 day',
     :'lifecycle_state', NULL);

INSERT INTO synthesis_output_source_classes (output_id, source_class, disposition)
SELECT :'output_id', 'article', 'included'
WHERE (:'insight_count')::integer > 0;
INSERT INTO synthesis_output_source_classes (output_id, source_class, disposition)
SELECT :'output_id', :'omitted_class', 'omitted'
WHERE :'omitted_class' <> '';

INSERT INTO synthesis_output_insights
  (id, output_id, ordinal, insight_type, through_line, confidence, created_at)
SELECT :'insight_id', :'output_id', 0, 'pattern',
             'Matrix through line for ' || :'fixture', 0.9,
             CURRENT_TIMESTAMP - make_interval(mins => (:'age_minutes')::integer)
WHERE (:'insight_count')::integer > 0;
INSERT INTO synthesis_citations (insight_id, artifact_id, ordinal)
SELECT :'insight_id', :'artifact_id', 0
WHERE (:'insight_count')::integer > 0;

INSERT INTO synthesis_run_attempts
    (logical_key, outcome, failure_class, failure_message, recorded_at,
     failure_kind, run_id, attempt_no, trigger_kind, state, output_id,
     started_at, finished_at, failure_code, included_source_classes,
     omitted_source_classes, insight_count, citation_count)
VALUES
    (:'logical_key', 'succeeded', NULL, NULL,
     CURRENT_TIMESTAMP - make_interval(mins => (:'age_minutes')::integer) - INTERVAL '1 minute',
     NULL, :'run_id', 1, 'scheduled', :'terminal_event', :'output_id',
     CURRENT_TIMESTAMP - make_interval(mins => (:'age_minutes')::integer) - INTERVAL '1 minute',
     CURRENT_TIMESTAMP - make_interval(mins => (:'age_minutes')::integer),
     NULL,
     CASE WHEN (:'insight_count')::integer > 0 THEN ARRAY['article']::TEXT[] ELSE '{}'::TEXT[] END,
     CASE WHEN :'omitted_class' <> '' THEN ARRAY[:'omitted_class']::TEXT[] ELSE '{}'::TEXT[] END,
     (:'insight_count')::integer, (:'insight_count')::integer);

INSERT INTO synthesis_run_events
    (id, run_id, attempt_no, event_type, output_id, related_output_id,
     failure_code, insight_count, citation_count, created_at)
VALUES
    (:'started_event_id', :'run_id', 1, 'attempt_started', NULL, NULL,
     NULL, NULL, NULL,
     CURRENT_TIMESTAMP - make_interval(mins => (:'age_minutes')::integer) - INTERVAL '1 minute'),
    (:'terminal_event_id', :'run_id', 1, :'terminal_event', :'output_id', NULL,
     NULL, (:'insight_count')::integer, (:'insight_count')::integer,
     CURRENT_TIMESTAMP - make_interval(mins => (:'age_minutes')::integer));
COMMIT;
" \
                --set="actor=$SYNTHESIS_ACTOR_USER_ID" \
                --set="fixture=$fixture" \
                --set="run_id=$run_id" \
                --set="logical_key=$logical_key" \
                --set="output_id=$output_id" \
                --set="insight_id=$insight_id" \
                --set="artifact_id=$artifact_id" \
                --set="source_set_digest=$source_set_digest" \
                --set="kind=$kind" \
                --set="terminal_event=$terminal_event" \
                --set="lifecycle_state=$lifecycle_state" \
                --set="insight_count=$insight_count" \
                --set="omitted_class=$omitted_class" \
                --set="age_minutes=$age_minutes" \
                --set="window_age_days=$window_age_days" \
                --set="started_event_id=event-$fixture-started" \
                --set="terminal_event_id=event-$fixture-$terminal_event"
}

# Prove the quiet fixture exists as one causally linked run/output/attempt/event
# graph before asking the UI to classify it. This precondition keeps a missing
# or disconnected fixture from masquerading as a rendering regression.
assert_quiet_fixture_linked() {
    local actual expected

    actual="$(synthesis_psql "
SELECT concat(
        'runs=', (
                SELECT COUNT(*) FROM synthesis_runs r
                WHERE r.id = :'run_id' AND r.logical_key = :'logical_key'
                    AND r.principal = :'actor' AND r.cadence = 'daily'
                    AND r.state = 'succeeded'
        ),
        ',outputs=', (
                SELECT COUNT(*) FROM synthesis_outputs o
                WHERE o.id = :'output_id' AND o.run_id = :'run_id'
                    AND o.principal = :'actor' AND o.cadence = 'daily'
                    AND o.output_kind = 'quiet' AND o.insight_count = 0
                    AND o.citation_count = 0 AND o.lifecycle_state = 'current'
        ),
        ',attempts=', (
                SELECT COUNT(*)
                FROM synthesis_run_attempts a
                JOIN synthesis_runs r ON r.id = a.run_id
                JOIN synthesis_outputs o ON o.id = a.output_id AND o.run_id = a.run_id
                WHERE r.id = :'run_id' AND o.id = :'output_id'
                    AND a.logical_key = r.logical_key AND a.attempt_no = 1
                    AND a.state = 'quiet' AND a.outcome = 'succeeded'
        ),
        ',started_events=', (
                SELECT COUNT(*)
                FROM synthesis_run_events e
                JOIN synthesis_run_attempts a
                    ON a.run_id = e.run_id AND a.attempt_no = e.attempt_no
                WHERE e.run_id = :'run_id' AND e.attempt_no = 1
                    AND e.event_type = 'attempt_started' AND e.output_id IS NULL
        ),
        ',terminal_events=', (
                SELECT COUNT(*)
                FROM synthesis_run_events e
                JOIN synthesis_run_attempts a
                    ON a.run_id = e.run_id AND a.attempt_no = e.attempt_no
                JOIN synthesis_outputs o ON o.id = e.output_id AND o.run_id = e.run_id
                WHERE e.run_id = :'run_id' AND e.attempt_no = 1
                    AND e.event_type = 'quiet' AND e.output_id = :'output_id'
        )
);
" \
        --set="actor=$SYNTHESIS_ACTOR_USER_ID" \
        --set="run_id=run-quiet" \
        --set="logical_key=key-quiet" \
        --set="output_id=out-quiet" \
        --tuples-only --no-align)"
    expected="runs=1,outputs=1,attempts=1,started_events=1,terminal_events=1"
    if [[ "$actual" != "$expected" ]]; then
        echo "FAIL: quiet fixture causal precondition was '$actual', want '$expected'"
        exit 1
    fi
    echo "PASS: quiet fixture has one causally linked run, output, attempt, start event, and terminal event"
}

# seed_failed_attempt <failed-no-output|failed-after-prior>
#
# A failure is a newer, linked run and attempt with a content-free safe code,
# one immutable start event, and one matching failed terminal event. It never
# inserts a recovered event. When a prior verified output exists, the failed
# run is superseded while that independently verified output remains current.
seed_failed_attempt() {
        local fixture="${1:?failure fixture required}"
        local run_id logical_key source_set_digest lifecycle_state

        case "$fixture" in
                failed-no-output)
                        run_id="run-failed-no-output"
                        logical_key="key-failed-no-output"
                        source_set_digest="source-set-failed-no-output"
                        lifecycle_state="current"
                        ;;
                failed-after-prior)
                        run_id="run-failed-after-prior"
                        logical_key="key-failed-after-prior"
                        source_set_digest="source-set-failed-after-prior"
                        lifecycle_state="superseded"
                        ;;
                *)
                        echo "FAIL: unknown synthesis failure fixture '$fixture'" >&2
                        exit 1
                        ;;
        esac

        synthesis_psql_command "
BEGIN;
INSERT INTO synthesis_runs
    (id, logical_key, cadence, principal, window_start, window_end, policy_version,
     source_set_digest, state, created_at, updated_at, lifecycle_state, attempt_count,
     lease_holder, lease_expires_at)
VALUES
    (:'run_id', :'logical_key', 'daily', :'actor',
     date_trunc('day', CURRENT_TIMESTAMP) - INTERVAL '2 days',
     date_trunc('day', CURRENT_TIMESTAMP) - INTERVAL '1 day',
     'matrix-policy-v1', :'source_set_digest', 'failed',
     CURRENT_TIMESTAMP - INTERVAL '6 minutes',
     CURRENT_TIMESTAMP - INTERVAL '5 minutes', :'lifecycle_state', 1, NULL, NULL);

INSERT INTO synthesis_run_attempts
    (logical_key, outcome, failure_class, failure_message, recorded_at,
     failure_kind, run_id, attempt_no, trigger_kind, state, output_id,
     started_at, finished_at, failure_code, included_source_classes,
     omitted_source_classes, insight_count, citation_count)
VALUES
    (:'logical_key', 'failed', 'transaction_failed', 'attempt failed',
     CURRENT_TIMESTAMP - INTERVAL '6 minutes', 'terminal', :'run_id', 1,
     'scheduled', 'failed', NULL, CURRENT_TIMESTAMP - INTERVAL '6 minutes',
     CURRENT_TIMESTAMP - INTERVAL '5 minutes', 'transaction_failed',
     '{}'::TEXT[], '{}'::TEXT[], 0, 0);

INSERT INTO synthesis_run_events
    (id, run_id, attempt_no, event_type, output_id, related_output_id,
     failure_code, insight_count, citation_count, created_at)
VALUES
    (:'started_event_id', :'run_id', 1, 'attempt_started', NULL, NULL,
     NULL, NULL, NULL, CURRENT_TIMESTAMP - INTERVAL '6 minutes'),
    (:'failed_event_id', :'run_id', 1, 'failed', NULL, NULL,
     'transaction_failed', NULL, NULL, CURRENT_TIMESTAMP - INTERVAL '5 minutes');
COMMIT;
" \
                --set="actor=$SYNTHESIS_ACTOR_USER_ID" \
                --set="run_id=$run_id" \
                --set="logical_key=$logical_key" \
                --set="source_set_digest=$source_set_digest" \
                --set="lifecycle_state=$lifecycle_state" \
                --set="started_event_id=event-$fixture-started" \
                --set="failed_event_id=event-$fixture-failed"
}

# assert_state <path> <expected>
#
# Asserts the rendered page carries EXACTLY ONE state marker and that it is the
# expected one. The count matters as much as the value: two markers would mean
# two states are showing at once, which is the failure the closed vocabulary
# exists to prevent.
assert_state() {
    local path="$1" expected="$2" html count actual
    html="$(curl -fsS --max-time 20 -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL$path")"

    count="$(grep -o 'data-synthesis-state="[a-z_]*"' <<<"$html" | wc -l | tr -d ' ')"
    if [[ "$count" != "1" ]]; then
        echo "FAIL: $path rendered $count synthesis state markers, want exactly 1"
        exit 1
    fi

    actual="$(grep -o 'data-synthesis-state="[a-z_]*"' <<<"$html" | head -1 | sed 's/.*="//;s/"//')"
    if [[ "$actual" != "$expected" ]]; then
        echo "FAIL: $path rendered state '$actual', want '$expected'"
        exit 1
    fi

    # A state that may not show prose must not show any.
    if [[ "$expected" != "current" && "$expected" != "stale" && "$expected" != "partial" ]]; then
        if grep -q 'class="card synthesis-insight"' <<<"$html"; then
            echo "FAIL: $path rendered synthesis prose in state '$expected', which carries no content"
            exit 1
        fi
    fi
}

# assert_prose_present <path>
assert_prose_present() {
    local path="$1" html
    html="$(curl -fsS --max-time 20 -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL$path")"
    if ! grep -q 'class="card synthesis-insight"' <<<"$html"; then
        echo "FAIL: $path is a content state but rendered no insight; a headline with nothing under it is not an answer"
        exit 1
    fi
    if ! grep -q 'data-citation-count=' <<<"$html"; then
        echo "FAIL: $path rendered an insight with no citation disclosure"
        exit 1
    fi
}

# assert_retry_offered <path>
#
# A failure state must offer recovery, and it must do so as a real button rather
# than as text that merely reads like one. The browser suite cannot reach a
# failure state on a healthy stack, so the structural claim is asserted here
# where the state can be seeded.
assert_retry_offered() {
    local path="$1" html
    html="$(curl -fsS --max-time 20 -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL$path")"
    if ! grep -qE '<button[^>]*data-synthesis-retry' <<<"$html"; then
        echo "FAIL: $path is a failure state but offers no retry button"
        exit 1
    fi
    if ! grep -qE '<button[^>]*class="action"[^>]*data-synthesis-retry' <<<"$html"; then
        echo "FAIL: $path retry control does not carry the action class that gives it its target size"
        exit 1
    fi
}

# assert_provenance <path> <output-id>
#
# A degraded state must still say WHICH run it is describing. Rendering a
# limitation without naming the output behind it leaves a reader unable to tell
# one stale answer from another.
assert_provenance() {
    local path="$1" output_id="$2" html
    html="$(curl -fsS --max-time 20 -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL$path")"
    if ! grep -q "$output_id" <<<"$html"; then
        echo "FAIL: $path does not name output $output_id; the state carries no durable provenance"
        exit 1
    fi
}

# assert_no_citation_disclosure <path>
#
# A rejected or failed candidate must expose neither prose nor the citation
# counts that would reveal a synthesis exists.
assert_no_citation_disclosure() {
    local path="$1" html
    html="$(curl -fsS --max-time 20 -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL$path")"
    if grep -q 'data-citation-count=' <<<"$html"; then
        echo "FAIL: $path exposes a citation count in a state that carries no verified content"
        exit 1
    fi
}

check() {
    local label="$1" expected="$2"
    assert_state /digest "$expected"
    assert_state /status "$expected"
    echo "PASS: $label renders '$expected' on both Today and Status"
}

# 1. never_run — nothing has ever been attempted or committed.
reset_synthesis
check "never-run" never_run

# 2. quiet — a committed output that deliberately says nothing.
reset_synthesis
seed_output quiet
assert_quiet_fixture_linked
check "quiet window" quiet
echo "PASS: quiet carries no prose (asserted inside assert_state)"

# 3. current — a fresh full output with a cited insight.
reset_synthesis
seed_output current
check "current" current
assert_prose_present /digest
echo "PASS: current renders persisted prose with citation disclosure"

# 4. stale — the same shape, past any plausible freshness budget.
reset_synthesis
seed_output stale
check "stale" stale
assert_provenance /digest out-stale
echo "PASS: stale names the durable output behind it"

# 5. partial — a policy-approved incomplete answer.
reset_synthesis
seed_output partial
check "partial" partial
assert_provenance /digest out-partial
echo "PASS: partial names the durable output behind it"

# 6. failed_without_output — an attempt failed and nothing verified exists.
reset_synthesis
seed_failed_attempt failed-no-output
check "failure with no prior output" failed_without_output
assert_retry_offered /digest
assert_no_citation_disclosure /digest
echo "PASS: failed_without_output offers a real retry button and leaks no citation disclosure"

# 7. failed_with_prior_output — an attempt failed but an older output survives.
#    The older output must NOT be presented as the current answer.
reset_synthesis
seed_output prior
seed_failed_attempt failed-after-prior
check "failure with a prior output" failed_with_prior_output
assert_retry_offered /digest
assert_no_citation_disclosure /digest
assert_provenance /digest out-prior
echo "PASS: failed_with_prior_output names the prior run without presenting it as the answer"

echo "=== durable synthesis state matrix: all seven states render exclusively ==="
