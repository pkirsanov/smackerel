#!/usr/bin/env bash
# Spec 061 SCOPE-01 validation rule #6 — concrete predicate.
#
# Asserts that the three v1 user-facing assistant scenarios shipped by
# spec 061 SCOPE-03 are PRESENT in the live config/prompt_contracts/
# directory AND that they load cleanly against the Spec 037 loader
# (allowed_tools resolved, schemas compile, side-effect class ordered).
#
# Wires SCOPE-01 design §7.2 rule #6: "three v1 scenario YAMLs present
# in AGENT_SCENARIO_DIR and pass Spec 037 loader validation". SCOPE-01
# ships the validator hook signature; this functional test injects the
# concrete predicate.
#
# Test environment isolation: pure validation against committed YAML +
# in-process Go binary; no DB, no NATS, no external network.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

LOG="$(mktemp)"
trap 'rm -f "$LOG"' EXIT

if ! "$REPO_ROOT/scripts/lib/run-with-timeout.sh" 120 go run ./cmd/scenario-lint/ config/prompt_contracts >"$LOG" 2>&1; then
    sed 's|'"$HOME"'|~|g' "$LOG" >&2
    echo "FAIL scenario-lint exited non-zero" >&2
    exit 1
fi

# Loader must report 0 rejections.
if ! grep -qE 'scenarios registered: [0-9]+, rejected: 0' "$LOG"; then
    sed 's|'"$HOME"'|~|g' "$LOG" >&2
    echo "FAIL scenario-lint did not report 'rejected: 0'" >&2
    exit 1
fi

# The three v1 spec-061 scenarios must be present as files. The loader
# above already proves they pass validation; this checks file presence
# so a regression that deletes a YAML surfaces a precise per-file error.
missing=()
for f in retrieval-qa-v1.yaml weather-query-v1.yaml notification-schedule-v1.yaml; do
    if [[ ! -f "config/prompt_contracts/$f" ]]; then
        missing+=("$f")
    fi
done
if (( ${#missing[@]} > 0 )); then
    echo "FAIL missing spec-061 v1 scenario file(s): ${missing[*]}" >&2
    exit 1
fi

# Rule #6 happy path — scenario-lint with -assistant-manifest must
# exit zero and print the "rule #6 OK" confirmation line. This proves
# the SCOPE-03 ValidateScenariosPresent hook is wired into the
# scenario-lint binary.
LOG_OK="$(mktemp)"
if ! "$REPO_ROOT/scripts/lib/run-with-timeout.sh" 120 go run ./cmd/scenario-lint/ \
        -assistant-manifest config/assistant/scenarios.yaml \
        config/prompt_contracts >"$LOG_OK" 2>&1; then
    sed 's|'"$HOME"'|~|g' "$LOG_OK" >&2
    rm -f "$LOG_OK"
    echo "FAIL scenario-lint -assistant-manifest (happy path) exited non-zero" >&2
    exit 1
fi
if ! grep -qE 'rule #6 OK' "$LOG_OK"; then
    sed 's|'"$HOME"'|~|g' "$LOG_OK" >&2
    rm -f "$LOG_OK"
    echo "FAIL scenario-lint -assistant-manifest did not emit 'rule #6 OK'" >&2
    exit 1
fi
rm -f "$LOG_OK"

# Rule #6 adversarial path — copy config/prompt_contracts into a temp
# dir, delete the weather YAML, and assert scenario-lint exits non-zero
# with the [F061-SCENARIO-MISSING] prefix naming the weather scenario.
TMPDIR_T="$(mktemp -d)"
trap 'rm -f "$LOG"; rm -rf "$TMPDIR_T"' EXIT
cp config/prompt_contracts/*.yaml "$TMPDIR_T/"
rm "$TMPDIR_T/weather-query-v1.yaml"

LOG_FAIL="$(mktemp)"
set +e
"$REPO_ROOT/scripts/lib/run-with-timeout.sh" 120 go run ./cmd/scenario-lint/ \
    -assistant-manifest config/assistant/scenarios.yaml \
    "$TMPDIR_T" >"$LOG_FAIL" 2>&1
EXIT_CODE=$?
set -e
if [[ $EXIT_CODE -eq 0 ]]; then
    sed 's|'"$HOME"'|~|g' "$LOG_FAIL" >&2
    rm -f "$LOG_FAIL"
    echo "FAIL scenario-lint must fail when weather YAML is missing (got exit 0)" >&2
    exit 1
fi
if ! grep -qE '\[F061-SCENARIO-MISSING\]' "$LOG_FAIL"; then
    sed 's|'"$HOME"'|~|g' "$LOG_FAIL" >&2
    rm -f "$LOG_FAIL"
    echo "FAIL scenario-lint did not emit [F061-SCENARIO-MISSING] for missing weather YAML" >&2
    exit 1
fi
if ! grep -qE 'weather_query' "$LOG_FAIL"; then
    sed 's|'"$HOME"'|~|g' "$LOG_FAIL" >&2
    rm -f "$LOG_FAIL"
    echo "FAIL scenario-lint error must name the missing 'weather_query' scenario id" >&2
    exit 1
fi
rm -f "$LOG_FAIL"

echo "PASS spec-061 v1 scenarios present and load cleanly + rule #6 happy and adversarial paths OK"
