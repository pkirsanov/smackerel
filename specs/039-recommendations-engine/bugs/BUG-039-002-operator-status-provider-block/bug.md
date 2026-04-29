# Bug: BUG-039-002 Operator status provider block

## Summary
`TestOperatorStatus_RecommendationProvidersEmptyByDefault` reports the `/status` page missing the `Recommendation Providers` block, blocking feature 039 full-delivery certification for `SCN-039-002`.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Active feature 039 scope certification blocked for a protected E2E UI scenario
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed (targeted red-stage output captured: missing `Recommendation Providers` block)
- [x] In Progress
- [x] Fixed (behavior green; validate-owned certification metadata still pending)
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Run the broad E2E command through `./smackerel.sh test e2e`.
2. Allow `tests/e2e/operator_status_test.go::TestOperatorStatus_RecommendationProvidersEmptyByDefault` to execute.
3. Request the operator `/status` page with recommendations enabled and no providers configured.
4. Observe whether the page contains the `Recommendation Providers` block.

## Expected Behavior
The `/status` page should render the recommendation provider health block for feature 039, showing zero configured providers without fabricated provider rows.

## Actual Behavior
The Go E2E failure reports `status page missing Recommendation Providers block`.

## Environment
- Service: Go core web/status page, recommendation provider registry, E2E stack
- Version: Workspace state on 2026-04-28 during 039 broad E2E failure classification
- Platform: Linux, Docker-backed disposable E2E stack

## Error Output
```text
Broad E2E classification input:
Go E2E TestOperatorStatus_RecommendationProvidersEmptyByDefault:
operator_status_test.go: status page missing Recommendation Providers block.
```

## Root Cause
The `/status` page omitted the recommendation provider operator contract. `internal/web/handler.go` did not pass recommendation provider status data into the status view model, and `internal/web/templates.go` did not render a `Recommendation Providers` section. The recommendation SST block and provider registry already existed; the registry is intentionally empty for this scope, so the fix was status view-model/template wiring rather than config changes or fabricated providers.

## Resolution Status
The targeted operator status E2E regression is green, the broad E2E suite exits 0, and the empty-provider state remains honest with zero configured providers and no fabricated rows. The bug is marked Fixed on the bug-owned artifact only.

Certification is not claimed here. Validate-owned `state.json` fields still need reconciliation for `certification.completedScopes` and required phase records. The integration-suite caveat remains active: `./smackerel.sh test integration` is red due unrelated NATS failures mapped to existing `BUG-022-001`; this bug only relies on the passed recommendation-provider integration case plus targeted and broad E2E evidence.

## Related
- Feature: `specs/039-recommendations-engine/`
- Scenario: `SCN-039-002 Provider registry is empty by default`
- Scenario contract: `specs/039-recommendations-engine/scenario-manifest.json`
- E2E test: `tests/e2e/operator_status_test.go::TestOperatorStatus_RecommendationProvidersEmptyByDefault`
- Existing non-covering bug: `BUG-039-001 Certification state drift`
