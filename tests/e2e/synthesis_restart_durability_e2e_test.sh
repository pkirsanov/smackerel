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
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

echo "=== T004-02-RESTART / T004-06-RECOVERY: durability across a real restart ==="

e2e_start

synthesis_retry_output_id() {
    local body
    body="$(curl -fsS --max-time 60 -X POST "$CORE_URL/api/synthesis/retry" \
        -H "Authorization: Bearer $AUTH_TOKEN" \
        -H 'Content-Type: application/json' \
        -d '{"cadence":"daily"}')"
    jq -r '.output.outputId // ""' <<<"$body"
}

BEFORE_ID="$(synthesis_retry_output_id)"
if [[ -z "$BEFORE_ID" ]]; then
    echo "FAIL: first trigger produced no output id; there is no identity to carry across a restart"
    exit 1
fi
echo "committed output before restart: $BEFORE_ID"

# Kill the process. Anything it held only in memory goes with it.
echo "--- restarting the core process ---"
smackerel_compose "$TEST_ENV" restart smackerel-core

# A single successful probe right after a restart proves nothing: the old
# process may still be holding the socket for a moment before it goes down.
# Requiring several consecutive successes, spaced out, means the probe cannot be
# satisfied by a listener that is about to disappear.
wait_for_stable_health() {
    local needed=3 streak=0 deadline=$((SECONDS + 180))
    while ((SECONDS < deadline)); do
        if curl -fsS --max-time 5 -H "Authorization: Bearer $AUTH_TOKEN" \
            "$CORE_URL/api/health" >/dev/null 2>&1; then
            streak=$((streak + 1))
            if ((streak >= needed)); then
                return 0
            fi
        else
            streak=0
        fi
        sleep 2
    done
    echo "FAIL: core did not reach $needed consecutive healthy probes within 180s after the restart"
    exit 1
}
wait_for_stable_health
echo "core process restarted and serving again"

# T004-02-RESTART: the same window must resolve to the same durable identity.
AFTER_ID="$(synthesis_retry_output_id)"
if [[ -z "$AFTER_ID" ]]; then
    echo "FAIL: trigger after restart produced no output id; the window lost its identity"
    exit 1
fi
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
if ! jq -e --arg id "$BEFORE_ID" '.runs | map(.outputId) | index($id) != null' <<<"$RUNS" >/dev/null; then
    echo "FAIL: output $BEFORE_ID survived into latest but is absent from run history after the restart;"
    echo "      the two read surfaces disagree"
    exit 1
fi
echo "PASS: T004-06-RECOVERY — health and history both recovered $BEFORE_ID from storage (state '$LATEST_STATE')"

echo "=== durability across a real restart: complete ==="
