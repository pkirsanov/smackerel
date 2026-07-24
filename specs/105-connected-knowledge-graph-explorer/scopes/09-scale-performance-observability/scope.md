# SCOPE-09: Scale, Performance, And Observability

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-08

## Outcome

Prove the explorer remains bounded, interactive, observable, and private on
representative high-degree disposable data beyond the measured 28,000-artifact
and 622,000-edge scale. Initial load, expansion, interaction, path, stress, and
sustained load must meet the stated NFR budgets or return typed bounded outcomes.

## Requirements And Scenarios

- NFR-105-001, NFR-105-002, NFR-105-003, NFR-105-004
- Primary scenario: SCN-105-007. Scale rows also revalidate SCN-105-001 and SCN-105-003.

```gherkin
Revalidation Case: SCN-105-001 Bounded overview remains interactive at graph scale
  Given a disposable store has representative high-degree topology at or beyond the measured scale
  When an authenticated reader opens the configured bounded overview
  Then the client receives no more than the configured limits and becomes meaningfully interactive within the NFR budget
  And the server query plan uses declared indexes rather than a whole-edge-store scan

Scenario: SCN-105-007 Distinguish no path from partial failure under scale
  Given path traversal exhausts an authorized high-degree scope without a match
  When every declared depth, visit, time, response, and dependency bound completes
  Then the result is no-path
  When any bound or required read prevents exhaustive completion
  Then the result is partial and no-path is absent
```

## Performance And Observability Plan

1. Build disposable deterministic scale fixtures with canonical nodes, stored edges, supported reasons/evidence, high-degree hubs, no personal operated data, and `env=test*` telemetry.
2. Capture `EXPLAIN`/query-plan evidence for overview, incoming/outgoing expansion, search, node detail, and path against migration 063 indexes.
3. Measure initial API plus settled renderer time, expansion completion/failure, search, detail, path, and post-data client interaction latency at P95/P99 where specified.
4. Run burst stress and sustained load separately. Refuse client/session limits visibly; never background-fetch or lay out the whole graph.
5. Assert response bytes, node/edge counts, path visited/depth/time, allocations, goroutine/request cancellation, browser responsiveness, and no unbounded growth across repeated expansion/collapse.
6. Validate graph metrics, logs, traces, and client observations against closed low-cardinality schemas and absence of content/tokens. The current project registry has no graph `observabilityWorkflow`, so no G080/G100 tag is invented.
7. Implementation must add a project-owned graph trace/SLO workflow before graph-specific G080/G100 evidence can be claimed; the planning rows below remain direct integration/stress evidence until then.

## NFR Budgets

| NFR | Planned Proof |
|---|---|
| NFR-105-001 | P95 meaningful interaction within 2s for 50-node/100-edge initial view |
| NFR-105-002 | P95 focus/pan/zoom/filter/panel response within 100ms after data |
| NFR-105-003 | P95 expansion terminal outcome within 2s |
| NFR-105-004 | Bounded behavior beyond measured store scale; no whole-store client render |

## Change Boundary

**Allowed:** scale fixture generators, graph query indexes/plans, performance
instrumentation, graph metrics/logs/traces/client observations, stress/load
harnesses and docs.  
**Excluded:** operated personal data writes, operate-plane telemetry capture by
feature tests, default/fallback limits, unrelated service optimization, deploy
adapter changes.

## Test Plan

| ID | Test Type | Category | Scenario / NFR | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T105-007-U | Unit | `unit` | SCN-105-007 | `internal/api/graphapi/path_service_test.go` - `SCN-105-007 path outcome unit` | `./smackerel.sh test unit` | No |
| T105-007-I | Integration | `integration` | SCN-105-007 | `tests/integration/graphapi/path_service_test.go` - `SCN-105-007 path bounds integration` | `./smackerel.sh test integration` | Yes |
| T105-007-A | E2E API regression | `e2e-api` | SCN-105-007 | `tests/e2e/graph_explorer_e2e_test.go` - `SCN-105-007 no path versus partial API` | `./smackerel.sh test e2e` | Yes |
| T105-007-W | E2E UI regression | `e2e-ui` | SCN-105-007 | `web/pwa/tests/graph-explorer.spec.ts` - `SCN-105-007 no path versus partial UI` | `./smackerel.sh test e2e-ui` | Yes |
| T105-09-QUERYPLAN | Integration | `integration` | SCN-105-007; NFR-105-004 | `tests/integration/graphapi/query_plan_test.go` - `Graph overview expansion search detail and path use bounded indexed plans` | `./smackerel.sh test integration` | Yes |
| T105-09-STRESS-INITIAL | Stress | `stress` | SCN-105-001; NFR-105-001, 004 | `tests/stress/graph_explorer_stress_test.go` - `Bounded overview meets interaction budget at measured scale` | `./smackerel.sh test stress` | Yes |
| T105-09-STRESS-EXPAND | Stress | `stress` | SCN-105-003; NFR-105-003, 004 | `tests/stress/graph_explorer_stress_test.go` - `Concurrent high-degree expansion remains bounded and truthful` | `./smackerel.sh test stress` | Yes |
| T105-09-LOAD-CLIENT | Load | `load` | SCN-105-003; NFR-105-002, 004 | `tests/stress/graph_explorer_load_test.go` - `Sustained graph interactions remain responsive without unbounded growth` | `./smackerel.sh test stress` | Yes |
| T105-09-OBS | Observability integration | `integration` | SCN-105-001, SCN-105-003 | `tests/integration/graphapi/observability_test.go` - `Graph query path and client telemetry uses closed content-free outcomes` | `./smackerel.sh test integration` | Yes |
| T105-09-SLO | SLO evidence | `stress` | NFR-105-001, 002, 003 | `tests/stress/graph_explorer_stress_test.go` - `Graph explorer captured percentiles meet declared NFR targets` | `./smackerel.sh test stress` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-105-007 Distinguish no path from partial failure: exhaustive bounded traversal alone may return no-path, while any limit, timeout, unsupported edge, or dependency gap returns partial under scale.
- [ ] Initial, expansion, interaction, and path outcomes meet NFR-105-001 through NFR-105-004 under burst stress and sustained load.
- [ ] Validate-plane graph telemetry is complete, low-cardinality, content-free, and isolated; no operated personal graph or operate-plane destination is mutated.

#### Test Evidence - One Item Per Test Plan Row

- [ ] T105-007-U passes with evidence in `report.md#t105-007-u`.
- [ ] T105-007-I passes with evidence in `report.md#t105-007-i`.
- [ ] T105-007-A passes with evidence in `report.md#t105-007-a`.
- [ ] T105-007-W passes without interception with evidence in `report.md#t105-007-w`.
- [ ] T105-09-QUERYPLAN passes with query-plan/index evidence in `report.md#t105-09-queryplan`.
- [ ] T105-09-STRESS-INITIAL passes with P95, bounds, and responsiveness evidence in `report.md#t105-09-stress-initial`.
- [ ] T105-09-STRESS-EXPAND passes with concurrency, terminal outcome, and resource-bound evidence in `report.md#t105-09-stress-expand`.
- [ ] T105-09-LOAD-CLIENT passes sustained-load and no-growth evidence in `report.md#t105-09-load-client`.
- [ ] T105-09-OBS passes value-safe metric/log/trace/client-observation evidence in `report.md#t105-09-obs`.
- [ ] T105-09-SLO passes captured percentile evidence against every declared target in `report.md#t105-09-slo`.

#### Build Quality Gate

- [ ] Scope tests, stress/load isolation, environment-pollution and source-privacy scans, check, lint, format, docs, artifact lint, traceability, query-plan review, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because no scale fixture, query plan, stress, load,
SLO, telemetry, or runtime validation was executed by the planning owner. A
graph-specific registered trace/SLO workflow remains implementation-owned.