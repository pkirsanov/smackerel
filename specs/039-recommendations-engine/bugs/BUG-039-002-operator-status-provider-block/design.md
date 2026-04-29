# Bug Fix Design: BUG-039-002

## Root Cause Analysis

### Investigation Summary
The broad E2E suite reports `tests/e2e/operator_status_test.go::TestOperatorStatus_RecommendationProvidersEmptyByDefault` failing because the `/status` page is missing the `Recommendation Providers` block. Existing feature 039 artifacts define `SCN-039-002` as the empty-provider registry scenario and map it to this E2E test. Existing `BUG-039-001` covers certification-state drift only and does not cover this product/UI failure.

### Root Cause
Unproven at packetization time. The fix owner must capture the targeted red-stage `/status` response and determine whether the provider block is absent due to template omission, config disablement, registry/view-model wiring, broad-suite environment drift, or route/template mismatch.

### Impact Analysis
- Affected components: operator status route/template, recommendation provider registry view data, feature 039 config in test stack, E2E operator status coverage.
- Affected data: no persistent user data impact is known from packetization alone.
- Affected users: operators cannot see recommendation provider health or the empty-provider state, reducing observability for feature 039.

## Fix Design

### Solution Approach
Start with a targeted E2E run that captures the status page body and effective recommendation config/provider registry state. Repair the first confirmed broken contract so `/status` renders the provider health block and zero-provider state through the real page, not a synthetic test path.

### Alternative Approaches Considered
1. Accepting a missing provider block when no providers are configured - rejected because `SCN-039-002` explicitly requires the empty-provider UI state.
2. Moving the proof to a unit-only template test - rejected because the failure is a live E2E UI contract.
3. Treating `BUG-039-001` as covering this issue - rejected because that bug is limited to certification-state reconciliation.

## Regression Test Design
- Targeted E2E: `TestOperatorStatus_RecommendationProvidersEmptyByDefault` passes with the `Recommendation Providers` block and zero-provider messaging.
- Adversarial E2E: a status response without the provider block fails loudly.
- Integration/API guard: empty provider request continues returning `no_providers` with no fabricated candidates.
- Broad E2E: `./smackerel.sh test e2e` no longer reports the operator status provider block failure.
