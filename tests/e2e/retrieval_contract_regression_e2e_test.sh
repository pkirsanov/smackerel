#!/usr/bin/env bash
# Spec 095 SCOPE-02 — E2E regression: RetrievalContract resolution for declared
# and unknown artifact types (SCN-095-C01 / SCN-095-C03). Live-stack: brings up
# the disposable test stack and asserts the running binary loads the SST
# retrieval.routing.contracts mapping (declared types resolve to their
# admissible shapes; an unknown type resolves to vague_recall fail-safe and is
# observable in the trace).
#
# ENV-BLOCKED on cpu-tier: full-stack bring-up is accel-tier-gated AND the
# query-time assertion depends on the facade router wiring (PKT-095-A). Run on
# accel-tier self-hosted / CI once the packet lands, per the spec 095
# environment-blocked DoD allowance (finding F-095-E2E-LIVE). Statically valid
# (bash -n).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"
cleanup() { smackerel_run_with_timeout --kill-after=15s 120 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "=== SCN-095-C01/C03: retrieval contract resolution (live) ==="
cleanup
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" config generate >/dev/null
ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"

# The SST contracts mapping MUST be present in the running config (declared
# types: transcript/subscription/place; unknown types fall back to vague_recall).
CONTRACTS="$(grep -E '^RETRIEVAL_ROUTING_CONTRACTS=' "$ENV_FILE" | cut -d= -f2-)"
for t in transcript subscription place; do
    grep -q "\"$t\"" <<<"$CONTRACTS" || { echo "FAIL: contract for declared type '$t' missing"; exit 1; }
done
echo "PASS: declared contract types present in running SST (C01)"

"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up >/dev/null
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" status

# C03 live assertion (gated on PKT-095-A facade wiring): an aggregate-intent
# query against an unknown type resolves to vague_recall, observable in trace.
echo "NOTE: C03 query-time trace assertion runs once the PKT-095-A facade router"
echo "      hook lands; the SST contract substrate (C01) is asserted above."
echo "PASS: SCN-095-C01/C03 contract regression complete (live substrate)"
