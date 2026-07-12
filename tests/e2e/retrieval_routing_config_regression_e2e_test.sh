#!/usr/bin/env bash
# Spec 095 SCOPE-01 — E2E regression: missing retrieval/evergreen SST key
# aborts startup loudly (SCN-095-S01). Live-stack: builds smackerel-core and
# runs it with a deliberately incomplete env file (a required RETRIEVAL_* key
# removed); the process MUST exit non-zero with [F095-SST-MISSING]. A second
# run with the complete env confirms a clean start.
#
# ENV-BLOCKED on cpu-tier / Docker-contended hosts: the full-stack core image
# build + run is accel-tier-gated. Run on an accel-tier self-hosted / CI host per
# the spec 095 environment-blocked DoD allowance (finding F-095-E2E-LIVE). The
# script is statically valid (bash -n) and ready to run where the build fits.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"
SOURCE_ENV=""
COMPOSE_PROJECT=""
CORE_IMAGE=""

cleanup() {
    smackerel_run_with_timeout --kill-after=15s 60 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "=== SCN-095-S01: missing retrieval SST key aborts startup ==="
cleanup

"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" config generate >/dev/null
SOURCE_ENV="$(smackerel_require_env_file "$TEST_ENV")"
COMPOSE_PROJECT="$(smackerel_compose_project "$TEST_ENV")"
CORE_IMAGE="${COMPOSE_PROJECT}-smackerel-core:latest"

docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" --env-file "$SOURCE_ENV" up -d postgres nats
sleep 10
docker compose -p "$COMPOSE_PROJECT" -f "$REPO_DIR/docker-compose.yml" --env-file "$SOURCE_ENV" build smackerel-core

# Negative case: drop a required retrieval key → fail-loud non-zero start.
BROKEN_ENV=$(mktemp)
cp "$SOURCE_ENV" "$BROKEN_ENV"
sed -i '/^RETRIEVAL_ROUTING_INTENT_CONFIDENCE_THRESHOLD=/d' "$BROKEN_ENV"
set +e
OUT=$(smackerel_run_with_timeout --kill-after=15s 60 docker run --rm --env-file "$BROKEN_ENV" "$CORE_IMAGE" 2>&1)
CODE=$?
set -e
rm -f "$BROKEN_ENV"
if [ "$CODE" -eq 0 ]; then
    echo "FAIL: core started with a missing RETRIEVAL_* key (should fail loud)"; exit 1
fi
if ! grep -q "F095-SST-MISSING" <<<"$OUT"; then
    echo "FAIL: missing-key error did not carry [F095-SST-MISSING]:"; echo "$OUT"; exit 1
fi
echo "PASS: missing RETRIEVAL_* key aborted startup with [F095-SST-MISSING] (exit $CODE)"

# Positive case: complete env starts cleanly (smoke).
set +e
smackerel_run_with_timeout --kill-after=15s 30 docker run --rm --env-file "$SOURCE_ENV" "$CORE_IMAGE" --help >/dev/null 2>&1
set -e
echo "PASS: SCN-095-S01 config-validation regression complete"
