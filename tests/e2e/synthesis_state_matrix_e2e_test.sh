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

FRESH="NOW()"
# The freshness budget comes from DIGEST_STALE_AFTER_HOURS. Backdating well past
# any plausible budget is what makes the stale row stale rather than guessing the
# exact threshold.
OLD="NOW() - INTERVAL '400 days'"

reset_synthesis() {
    smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
        env PGPASSWORD="$POSTGRES_PASSWORD" \
        psql -h 127.0.0.1 -p "$POSTGRES_CONTAINER_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -q -c "
TRUNCATE synthesis_citations, synthesis_output_insights, synthesis_output_source_classes,
         synthesis_outputs, synthesis_run_attempts, synthesis_runs RESTART IDENTITY CASCADE;
" >/dev/null
}

# seed_output <suffix> <kind> <insight_count> <created_at_expr>
seed_output() {
    local suffix="$1" kind="$2" insights="$3" created="$4"
    smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
        env PGPASSWORD="$POSTGRES_PASSWORD" \
        psql -h 127.0.0.1 -p "$POSTGRES_CONTAINER_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -q -c "
INSERT INTO synthesis_runs
  (id, logical_key, cadence, principal, window_start, window_end, policy_version,
   source_set_digest, state, created_at, updated_at, lifecycle_state, attempt_count)
VALUES
  ('run-$suffix', 'key-$suffix', 'daily', 'matrix', $created - INTERVAL '1 day', $created,
   'v1', 'digest-$suffix', 'succeeded', $created, $created, 'current', 1);
INSERT INTO synthesis_outputs
  (id, run_id, insight_count, citation_count, created_at, output_kind, evaluated_artifact_count)
VALUES
  ('out-$suffix', 'run-$suffix', $insights, $insights, $created, '$kind', 7);
" >/dev/null

    if [[ "$insights" -gt 0 ]]; then
        smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
            env PGPASSWORD="$POSTGRES_PASSWORD" \
            psql -h 127.0.0.1 -p "$POSTGRES_CONTAINER_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -q -c "
INSERT INTO synthesis_output_insights
  (id, output_id, ordinal, insight_type, through_line, confidence, created_at)
VALUES
  ('ins-$suffix', 'out-$suffix', 0, 'pattern', 'Matrix through line for $suffix', 0.9, $created);
INSERT INTO synthesis_citations (insight_id, artifact_id, ordinal)
VALUES ('ins-$suffix', 'artifact-$suffix', 0);
" >/dev/null
    fi
}

# seed_attempt <outcome>
#
# The schema requires failure_class to travel with a failed outcome, so a failed
# attempt is seeded with one rather than worked around.
seed_attempt() {
    local outcome="$1" class_sql="NULL" msg_sql="NULL"
    if [[ "$outcome" == "failed" ]]; then
        class_sql="'transient'"
        msg_sql="'seeded failure for the state matrix'"
    fi
    smackerel_compose "$TEST_ENV" exec --interactive=false -T postgres \
        env PGPASSWORD="$POSTGRES_PASSWORD" \
        psql -h 127.0.0.1 -p "$POSTGRES_CONTAINER_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -q -c "
INSERT INTO synthesis_run_attempts (logical_key, outcome, failure_class, failure_message, recorded_at)
VALUES ('key-attempt', '$outcome', $class_sql, $msg_sql, NOW());
" >/dev/null
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
seed_output quiet quiet 0 "$FRESH"
seed_attempt succeeded
check "quiet window" quiet
echo "PASS: quiet carries no prose (asserted inside assert_state)"

# 3. current — a fresh full output with a cited insight.
reset_synthesis
seed_output current full 1 "$FRESH"
seed_attempt succeeded
check "current" current
assert_prose_present /digest
echo "PASS: current renders persisted prose with citation disclosure"

# 4. stale — the same shape, past any plausible freshness budget.
reset_synthesis
seed_output stale full 1 "$OLD"
seed_attempt succeeded
check "stale" stale

# 5. partial — a policy-approved incomplete answer.
reset_synthesis
seed_output partial partial 1 "$FRESH"
seed_attempt succeeded
check "partial" partial

# 6. failed_without_output — an attempt failed and nothing verified exists.
reset_synthesis
seed_attempt failed
check "failure with no prior output" failed_without_output

# 7. failed_with_prior_output — an attempt failed but an older output survives.
#    The older output must NOT be presented as the current answer.
reset_synthesis
seed_output prior full 1 "$FRESH"
seed_attempt failed
check "failure with a prior output" failed_with_prior_output

echo "=== durable synthesis state matrix: all seven states render exclusively ==="
