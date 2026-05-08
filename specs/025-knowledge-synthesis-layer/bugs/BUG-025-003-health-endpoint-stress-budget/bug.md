# Bug: BUG-025-003 Health endpoint stress budget

## Summary
`tests/stress/knowledge_stress_test.go::TestKnowledge_HealthEndpointIncludesKnowledgeSection` can fail the full stress gate because rapid authenticated `/api/health` calls with the knowledge section enabled exceed the strict per-call 2 second budget by a few milliseconds.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Full stress validation is blocked for the knowledge health endpoint after recommendation stress passes
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed by upstream BUG-039 test-phase stress evidence
- [x] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

## Reproduction Steps
1. Start from the current repaired shared stress readiness path and the current BUG-039 recommendation stress fix.
2. Run the full repo-standard stress gate: `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`.
3. Observe the disposable stress stack and Go stress readiness canary pass.
4. Observe BUG-039 recommendation stress pass its sample, p95, and error-budget assertions.
5. Observe `TestKnowledge_HealthEndpointIncludesKnowledgeSection` fail because one or more rapid `/api/health` calls take just over 2 seconds.

## Expected Behavior
The knowledge health endpoint stress check should keep the protected `/api/health` response inside the current stress budget while preserving the knowledge section contract. The endpoint should return HTTP 200, preserve existing health fields, and include the knowledge section when the authenticated caller and enabled knowledge layer allow it.

## Actual Behavior
The full stress command exits 1 even though the recommendation stress workload passes. The failing knowledge test reports rapid health checks taking approximately 2.004s to 2.027s, where the current assertion expects each check to complete in less than 2 seconds.

## Environment
- Service: Go core `/api/health` endpoint with knowledge health stats enabled
- Parent feature: `specs/025-knowledge-synthesis-layer`
- Parent scope: Scope 8, Digest Integration & Health
- Parent scenario: `SCN-025-23` Health endpoint includes knowledge stats
- Test: `tests/stress/knowledge_stress_test.go::TestKnowledge_HealthEndpointIncludesKnowledgeSection`
- Platform: Linux, Docker-backed disposable test stack managed by `./smackerel.sh test stress`
- Source context date: 2026-05-05

## Error Output
```text
Residual finding from BUG-039 test phase:

COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress
Exit Code: 1

BUG-039 recommendation stress passed, but the full command failed outside BUG-039:

=== RUN   TestKnowledge_HealthEndpointIncludesKnowledgeSection
    knowledge_stress_test.go:274: health check 0 took 2.021036143s, expected < 2s
    knowledge_stress_test.go:290: Knowledge stats: concepts=0, entities=0, pending=1100
    knowledge_stress_test.go:274: health check 1 took 2.027047451s, expected < 2s
    knowledge_stress_test.go:274: health check 2 took 2.007984206s, expected < 2s
    knowledge_stress_test.go:274: health check 3 took 2.004238477s, expected < 2s
--- FAIL: TestKnowledge_HealthEndpointIncludesKnowledgeSection
FAIL
```

## Root Ownership
This residual does not map to the existing closed BUG-025 packets:

- `BUG-025-001-knowledge-stats-empty-store` covers `/api/knowledge/stats` returning HTTP 500 on an empty store.
- `BUG-025-002-knowledge-e2e-external-url` covers non-deterministic external URL extraction in the knowledge synthesis E2E.

The current residual is a post-readiness, post-BUG-039 stress latency issue in the knowledge health section of `/api/health`. Ownership belongs to `specs/025-knowledge-synthesis-layer`, Scope 8, and the knowledge health endpoint/runtime path.

Initial ownership surfaces for the implementation owner:

- `tests/stress/knowledge_stress_test.go`
- `internal/api/health.go`
- `internal/api/health_test.go`
- `internal/knowledge/store.go`
- `tests/e2e/knowledge_health_test.go`

Precise technical root cause remains open for the implementation owner. The next owner must reproduce the red state and determine whether the first defect is endpoint latency, cache/cold-start behavior, knowledge health stats query behavior, stress assertion contract mismatch, or another knowledge-owned runtime path.

## Related
- Parent feature: `specs/025-knowledge-synthesis-layer/`
- Parent scope: Scope 8, Digest Integration & Health
- Parent scenario: `SCN-025-23` Health endpoint includes knowledge stats
- Routed from: `specs/039-recommendations-engine/bugs/BUG-039-003-recommendation-stress-zero-samples/` test phase residual
- Sibling bugs: `BUG-025-001-knowledge-stats-empty-store`, `BUG-025-002-knowledge-e2e-external-url`

## Resolution

**Root Cause:** The authenticated `/api/health` knowledge-section path allowed optional ML/Ollama probes to consume the full strict 2 second stress budget, and knowledge health stats refresh could add cold or expired-cache work on top of the auxiliary probe fan-out before the response completed. Under the stress test's 25 rapid authenticated calls, this pushed individual `/api/health` responses to 2.004s–2.027s, just over the strict `< 2s` per-call assertion in `tests/stress/knowledge_stress_test.go::TestKnowledge_HealthEndpointIncludesKnowledgeSection`. The protected stress budget was correct; the handler's auxiliary work budget was not.

**Fix:** Added a 1.5 second `healthAuxiliaryProbeTimeout` ceiling to bound `checkMLSidecar`, `checkOllama`, and `mlClient` health probes; started authenticated knowledge health stats refresh concurrently with the existing probe fan-out (rather than serially after); and made `getCachedKnowledgeHealth` return the stale knowledge cache when refresh fails or exceeds the bounded context. Added two adversarial unit regressions in `internal/api/health_test.go`: `TestHealthKnowledgeStressBudgetWithSlowProbesAndColdStats` (slow optional probes plus cold stats must not push the response past the budget) and `TestHealthKnowledgeReturnsStaleCacheWhenRefreshTimesOut` (expired stale cache plus slow refresh must not block the response).

**Fix Commit:** `9276735` `fix(025): BUG-025-003 — health endpoint stress budget (bug blocked, evidence captured)`

**Re-Verification at HEAD `ca2c843` (2026-05-08):** Full stress gate (`COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`) exited 0. `TestKnowledge_HealthEndpointIncludesKnowledgeSection` passed in 4.11s with all 25 rapid authenticated `/api/health` calls satisfying the strict `< 2s` per-call assertion (the prior failing run measured 2.004s–2.027s; the fix continues to hold). The first response's knowledge section was parsed and logged: `Knowledge stats: concepts=0, entities=0, pending=1100`. `TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests` remained green with `total=27146 ok=27146 unexpected_errors=0 timeout_errors=0 p95=1.006768166s max=1.884646563s` against the 10s budget. Go unit suite exit 0, including `internal/api` health regressions. The fix continues to hold across the Go 1.25.10 upgrade, photos chaos hardening, BUG-031-005 stress readiness fix, BUG-039-003 pgxpool deadlock fix, and BUG-040 hash-reveal hardening commits landed since the last validation pass on 2026-05-05. Full validate-phase evidence: [report.md → Validate Phase — Re-verification at HEAD ca2c843 — 2026-05-08](report.md).