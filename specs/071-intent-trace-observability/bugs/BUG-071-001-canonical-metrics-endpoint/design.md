# Bug Fix Design: BUG-071-001

## Root Cause Analysis

### Investigation Summary

The canonical E2E runner injects `CORE_EXTERNAL_URL=http://smackerel-core:<container-port>` and `SMACKEREL_TEST_ENV_FILE`. The failing test ignores that core endpoint and instead requires `SMACKEREL_CORE_METRICS_URL`. After that mismatch is removed, the real scrape shows `openknowledge_refusal_total` and `smackerel_assistant_intent_traces_total` are registered label vectors but absent from exposition until a child series exists.

### Root Cause

The test-side endpoint contract drifted from the runner-owned contract, and both production label vectors lacked pre-event closed-vocabulary child series. The route itself is present; visibility before first traffic is the production observability defect.

### Impact Analysis

- Affected component: `tests/e2e/assistant/intent_refusal_join_e2e_test.go`
- Affected behavior: one required spec-071 live observability scenario
- Affected data: none; the failing path performs no request

## Fix Design

### Solution Approach

Resolve the metrics URL from required `CORE_EXTERNAL_URL`, append `/metrics` exactly once, and retain the bounded real HTTP request. Initialize one valid zero series for intent traces and the complete bounded refusal-cause vocabulary for open knowledge; `Add(0)` exposes registration without fabricating usage. Add fresh-registry units and a closed assistant-package selector.

### Alternative Approaches Considered

1. Add `SMACKEREL_CORE_METRICS_URL` to runtime SST - rejected because it duplicates the canonical core endpoint for test-only use.
2. Synthesize a metrics body in the test - rejected because it would no longer verify the live registry.
3. Keep the both-unset success path - rejected because repository-managed E2E must execute the scenario.

### Single-Implementation Justification

- **Existing owning abstraction:** `smackerel.sh` injects the canonical `CORE_EXTERNAL_URL` into `scripts/runtime/go-e2e.sh`, and `internal/metrics.Handler` exposes the process Prometheus registry at `/metrics`. The E2E derives its scrape URL by appending `/metrics` to that one endpoint.
- **Concrete implementations:** `internal/assistant/openknowledge/metrics` owns `openknowledge_refusal_total`, and `internal/assistant/intenttrace.IntentTracesTotal` owns `smackerel_assistant_intent_traces_total`. Each is a concrete metric family in the same core registry, not an interchangeable metrics provider.
- **Current consumers:** `tests/e2e/assistant/intent_refusal_join_e2e_test.go`, the assistant-package selector in `go-e2e.sh`, and the Assistant Intents Grafana join query consume the canonical endpoint, family names, and closed label vocabularies.
- **Bounded variation axes:** Metric-family semantics vary between refusal causes and persisted intent-trace status, while E2E execution varies between the complete Go E2E set and the closed `assistant` package selector. Neither axis changes endpoint ownership or registry implementation.
- **Extension path:** A new assistant metric registers with the existing process registry and is verified through `CORE_EXTERNAL_URL + /metrics`. A new package selector requires an explicit closed CLI contract; it is not discovered through a plugin registry.
- **Foundation decision:** The defect is endpoint-contract drift plus missing zero-series initialization in two existing counters. A URL registry, metric-provider interface, or dynamic package registry would duplicate established ownership and introduce unsupported runtime variation.

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|---|---|---|
| Add a closed E2E package selector | Continue using a long test-name regex | A regex cannot prove all tests in exactly one package executed and risks matching other packages. |
