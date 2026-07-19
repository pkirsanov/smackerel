# Specification: BUG-071-001 Canonical metrics endpoint

## Expected Behavior

The spec-071 refusal/trace join E2E MUST use the repository's canonical live core endpoint and append `/metrics`. It MUST scrape the real disposable core service and require both `openknowledge_refusal_total` and `smackerel_assistant_intent_traces_total`. A repository-managed E2E run with missing endpoint wiring MUST fail loudly rather than silently omit the scenario.

## Acceptance Criteria

1. `TestIntentRefusalJoinE2E_LiveCoreExposesJoinKeyOnBothMetrics` consumes `CORE_EXTERNAL_URL`, the same endpoint contract as other assistant live tests.
2. The test performs a real HTTP scrape against the disposable stack with a bounded request timeout.
3. Removing the canonical endpoint from the E2E runner causes a direct test failure.
4. Removing either required metric family from the live registry causes a direct assertion failure.
5. Both joined CounterVec families expose valid closed-vocabulary zero series before the first real event; zero initialization MUST NOT count as an event.
6. The full assistant package runs through a repository CLI package selector without invoking every E2E package.
7. No request interception, canned metrics body, hidden endpoint value, or conditional success path is introduced.

## Release Train

This bug targets the `mvp` train and introduces no feature flag. Behavior on every train remains explicit and identical.

## Test Isolation

Live verification uses the `smackerel-test` disposable stack owned by `./smackerel.sh test e2e`. The stack is torn down by the repository CLI on success and failure.

## Deployment Boundary

No deployment, host, adapter, manifest, release-train, or secret surface is changed.
