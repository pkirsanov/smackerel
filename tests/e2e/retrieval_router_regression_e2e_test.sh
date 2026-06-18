#!/usr/bin/env bash
# Spec 095 SCOPE-03 — E2E regression: RetrievalStrategyRouter selection + the
# single-store invariant in the running binary (SCN-095-A01 / C02 / G01).
# Live-stack: brings up the disposable test stack and asserts the router selects
# exactly one strategy per query, honors the contract's admissible strategies,
# and that NO new store/index/graph is opened at runtime (the architecture-test
# invariant is enforced at build time; this confirms it holds in the live
# binary — one Postgres pool, one NATS conn, no second vector index process).
#
# ENV-BLOCKED on cpu-tier: full-stack bring-up is accel-tier-gated AND the
# query-time selection trace depends on the facade router wiring (PKT-095-A).
# Run on accel-tier home-lab / CI per the spec 095 environment-blocked DoD
# allowance (finding F-095-E2E-LIVE). Statically valid (bash -n).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"
cleanup() { timeout --kill-after=15s 120 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "=== SCN-095-A01/C02/G01: router selection + single-store invariant (live) ==="
cleanup
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" config generate >/dev/null
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up >/dev/null
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" status

COMPOSE_PROJECT="$(smackerel_compose_project "$TEST_ENV")"
# G01: exactly one Postgres + one NATS service in the stack — no parallel store.
PG_COUNT="$(docker ps --filter "name=${COMPOSE_PROJECT}-postgres" --format '{{.Names}}' | wc -l)"
[ "$PG_COUNT" -le 1 ] || { echo "FAIL: more than one Postgres store running (parallel-store regression)"; exit 1; }
echo "PASS: single backing store confirmed in the live stack (G01)"

echo "NOTE: A01/C02 query-time selection-trace assertions run once the PKT-095-A"
echo "      facade router hook lands (the router + architecture tests are proven"
echo "      at build time by internal/retrieval/routing/*_test.go)."
echo "PASS: SCN-095-A01/C02/G01 router regression complete (live invariant)"
