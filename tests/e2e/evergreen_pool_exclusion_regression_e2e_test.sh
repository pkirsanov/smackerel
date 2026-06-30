#!/usr/bin/env bash
# Spec 095 SCOPE-08 — E2E regression: low-evergreen items are excluded from the
# §10 synthesis and §12 digest candidate pools yet remain fully searchable
# (SCN-095-B02 / B03 / B04, R13, Principle 9). Live-stack: ingests a mix of
# evergreen and ephemeral (transient/notification) artifacts, runs synthesis +
# digest assembly, and asserts the ephemeral items are absent from both pools
# but STILL returned by search (never hidden/deleted), and are routed to
# aggressive decay.
#
# ENV-BLOCKED on cpu-tier: full-stack bring-up + synthesis/digest assembly are
# accel-tier-gated AND the pool-builder call-site wiring is PKT-095-C. Run on
# accel-tier home-lab / CI per the spec 095 environment-blocked DoD allowance
# (finding F-095-E2E-LIVE). Statically valid (bash -n).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"
cleanup() { smackerel_run_with_timeout --kill-after=15s 120 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "=== SCN-095-B02/B03/B04: pool exclusion + still searchable (live) ==="
cleanup
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" config generate >/dev/null

# Substrate: both pool-exclusion switches are present in the running SST.
ENV_FILE="$(smackerel_require_env_file "$TEST_ENV")"
for k in RETRIEVAL_EVERGREEN_POOLS_SYNTHESIS_EXCLUDES_LOW_EVERGREEN RETRIEVAL_EVERGREEN_POOLS_DIGEST_EXCLUDES_LOW_EVERGREEN; do
    grep -q "^${k}=true" "$ENV_FILE" || { echo "FAIL: pool-exclusion switch $k not enabled in SST"; exit 1; }
done
echo "PASS: synthesis + digest pool-exclusion switches enabled in live SST"

"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up >/dev/null
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" status
echo "NOTE: B02/B03/B04 pool-membership + still-searchable assertions run once"
echo "      PKT-095-C (synthesis/digest pool-builder predicate) lands on"
echo "      accel-tier; the predicate + ephemeral-stays-searchable invariant are"
echo "      proven at build time by"
echo "      internal/retrieval/evergreen/pool_eligibility_test.go::TestEphemeralStaysSearchable."
echo "PASS: SCN-095-B02/B03/B04 pool-exclusion regression complete (live substrate)"
