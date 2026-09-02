# Expected Behavior: [BUG-031-011] ML Readiness Oracle Fidelity

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Problem Statement

The current stress test presents an always-healthy ML dependency and expects a ready result. That arrangement cannot reject an implementation that treats every completed probe as healthy. The same test also names an ML route that the production router does not expose.

## Outcome Contract

**Intent:** Make the ML readiness regression oracle discriminate between ready and non-ready dependency responses.

**Success Signal:** The focused test passes for HTTP 200, rejects HTTP 503 as ready, and names ML `/health` separately from core `/readyz`.

**Hard Constraints:** Keep production readiness behavior unchanged. Preserve core `/readyz` as the existing database readiness route. Do not rewrite historical execution evidence.

**Failure Condition:** The repaired test still passes when the ML health probe ignores a non-200 status or when endpoint comments claim a route the test never calls.

## Goals

- Add a two-sided readiness discriminator to the existing stress test surface.
- Preserve the existing timeout-boundary regression as a distinct guard.
- Correct scenario and test text that conflates ML `/health` with core `/readyz`.
- Prove the repaired oracle kills an always-ready mutation.

## Non-Goals

- Changing `SearchEngine.WaitForMLReady` or `SearchEngine.probeMLHealth`.
- Adding ML health to core `GET /readyz`.
- Changing the database or strict graph readiness contracts.
- Modifying Docker, deployment, configuration, or runtime source.

## Requirements

- **R-BUG-031-011-001:** The regression must exercise the production `WaitForMLReady` path through an HTTP dependency.
- **R-BUG-031-011-002:** A healthy control must return HTTP 200 and require `ready=true` with at least one observed probe.
- **R-BUG-031-011-003:** The adversarial case must return HTTP 503 and require `ready=false` with at least one observed probe.
- **R-BUG-031-011-004:** The adversarial case must use a bounded timeout consistent with the existing compressed stress boundary.
- **R-BUG-031-011-005:** A surviving always-ready mutation must be captured before the repair. The repaired test must kill the same mutation.
- **R-BUG-031-011-006:** Test comments and scenario contracts must identify `${MLSidecarURL}/health` as the ML dependency endpoint.
- **R-BUG-031-011-007:** Test comments and scenario contracts must identify core `GET /readyz` as a separate database readiness endpoint.
- **R-BUG-031-011-008:** Historical raw evidence remains unchanged. Supersession metadata must replace inaccurate current claims.
- **R-BUG-031-011-009:** No production source, configuration, or route behavior may change in this bug scope.

## User Scenarios

```gherkin
Scenario: Non-ready ML dependency cannot be masked as ready
  Given the production ML readiness loop probes an HTTP dependency
  And the dependency returns HTTP 503 for every probe
  When the bounded readiness wait completes
  Then the result is not ready
  And at least one dependency probe occurred
```

```gherkin
Scenario: Healthy control proves the oracle can observe readiness
  Given the same production ML readiness loop and test harness
  And the dependency returns HTTP 200
  When the readiness wait runs
  Then the result is ready
  And at least one dependency probe occurred
```

```gherkin
Scenario: Endpoint identity remains truthful
  Given `WaitForMLReady` probes the ML sidecar
  When the regression contract names the dependency endpoint
  Then it names `${MLSidecarURL}/health`
  And it does not describe core `GET /readyz` as an ML route
```

## Acceptance Criteria

- **AC-01:** The focused stress test contains both healthy and non-ready dependency cases.
- **AC-02:** The non-ready case fails if `probeMLHealth` treats every HTTP response as healthy.
- **AC-03:** The healthy case fails if probing is bypassed or HTTP 200 cannot produce readiness.
- **AC-04:** The old `SCN-BUG-031-006-007` claim is superseded without altering its historical evidence.
- **AC-05:** Focused, integration companion, broader E2E, and broader stress checks pass through `./smackerel.sh`.
- **AC-06:** The dirty readiness test retains its pre-packet hash until the owning test phase starts.

## Exposure Contract

| Capability | Surface class | Surface id | Status | Plan |
|---|---|---|---|---|
| ML startup readiness | internal | `internal/api.SearchEngine.WaitForMLReady` | delivered | Test-only correction in this bug |
| ML health dependency | internal HTTP dependency | `${MLSidecarURL}/health` | delivered | Name accurately in the repaired test |
| Core readiness | httpRoute | `GET /readyz` | delivered | Preserve the DB-only default contract |

## Product Principle Alignment

- **Principle 8, Trust Through Transparency:** The test and scenario text must identify the real dependency and reject false readiness.
- This packet changes no delivered user capability. It corrects executable assurance for existing behavior.
- No roadmap-only example is presented as current behavior.

## Release Train

Target train: `mvp`.

This bug introduces no feature flag. Other trains receive no behavior change because the scope changes test assurance only.