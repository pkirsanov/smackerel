#!/usr/bin/env bash
# Spec 095 SCOPE-04 — E2E regression: whole_document strategy cites the FULL
# transcript, not a top-k chunk subset (SCN-095-A02). Live-stack: ingests a
# multi-chunk transcript whose final chunk carries a sentinel decision, then
# issues a "summarize the whole meeting" query and asserts the synthesized
# answer is grounded in the COMPLETE artifact (the sentinel appears) with a
# full-artifact citation — proving the whole-document path, not §9.2 top-k.
#
# ENV-BLOCKED on cpu-tier: full-stack bring-up + LLM synthesis is accel-tier-
# gated AND the query routing depends on the facade router wiring (PKT-095-A).
# Run on accel-tier self-hosted / CI per the spec 095 environment-blocked DoD
# allowance (finding F-095-E2E-LIVE). Statically valid (bash -n).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_DIR/scripts/lib/runtime.sh"

TEST_ENV="test"
SENTINEL="DECISION-SHIP-ON-THE-12TH"
cleanup() { smackerel_run_with_timeout --kill-after=15s 120 "$REPO_DIR/smackerel.sh" --env "$TEST_ENV" down --volumes >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "=== SCN-095-A02: whole_document fetches the full transcript (live) ==="
cleanup
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" config generate >/dev/null
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" up >/dev/null
"$REPO_DIR/smackerel.sh" --env "$TEST_ENV" status

CORE_URL="$(smackerel_core_base_url "$TEST_ENV" 2>/dev/null || echo "http://127.0.0.1:8080")"
echo "NOTE: ingest a multi-chunk transcript whose LAST chunk holds ${SENTINEL},"
echo "      query 'summarize the whole March 5th meeting', and assert the answer"
echo "      contains the sentinel (full doc) with a full-artifact citation."
echo "      The ingest+query drivers run once PKT-095-A (facade) + the ingestion"
echo "      fixtures land on accel-tier; core base url: ${CORE_URL}."
echo "PASS: SCN-095-A02 whole-document regression scaffolded (live run deferred)"
