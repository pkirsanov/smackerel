# Bug Fix Design: BUG-071-001

## Root Cause Analysis

### Investigation Summary

The canonical E2E runner injects `CORE_EXTERNAL_URL=http://smackerel-core:<container-port>` and `SMACKEREL_TEST_ENV_FILE`. The failing test ignores that core endpoint and instead requires `SMACKEREL_CORE_METRICS_URL`, a variable with no SST producer or runner wiring anywhere else in the repository.

### Root Cause

The test-side live environment contract drifted from the runner-owned endpoint contract. This is a test-harness defect, not a missing production metrics route.

### Impact Analysis

- Affected component: `tests/e2e/assistant/intent_refusal_join_e2e_test.go`
- Affected behavior: one required spec-071 live observability scenario
- Affected data: none; the failing path performs no request

## Fix Design

### Solution Approach

Resolve the metrics URL from required `CORE_EXTERNAL_URL`, append `/metrics` exactly once, retain the bounded real HTTP request, and add a closed assistant-package selector to the repository E2E command so this package can run without the all-package suite. Preserve fail-loud behavior when the endpoint or either metric family is absent.

### Alternative Approaches Considered

1. Add `SMACKEREL_CORE_METRICS_URL` to runtime SST - rejected because it duplicates the canonical core endpoint for test-only use.
2. Synthesize a metrics body in the test - rejected because it would no longer verify the live registry.
3. Keep the both-unset success path - rejected because repository-managed E2E must execute the scenario.

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|---|---|---|
| Add a closed E2E package selector | Continue using a long test-name regex | A regex cannot prove all tests in exactly one package executed and risks matching other packages. |
