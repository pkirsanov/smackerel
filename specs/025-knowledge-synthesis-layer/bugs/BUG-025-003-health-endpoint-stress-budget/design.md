# Bug Fix Design: BUG-025-003 Health endpoint stress budget

## Root Cause Analysis

### Investigation Summary
The residual finding was reported during the BUG-039 test phase after the recommendation stress workload was already green. Full stress still exited 1 because `tests/stress/knowledge_stress_test.go::TestKnowledge_HealthEndpointIncludesKnowledgeSection` reported rapid `/api/health` calls slightly above the strict 2 second budget.

Source inspection confirms this is the parent feature 025 Scope 8 health integration surface. Parent artifacts define `/api/health` knowledge-section behavior in Scope 8 and `SCN-025-23`; current implementation uses `internal/api/health.go` with `getCachedKnowledgeHealth`, `internal/knowledge/store.go::GetKnowledgeHealthStats`, and tests in `internal/api/health_test.go`, `tests/e2e/knowledge_health_test.go`, and `tests/stress/knowledge_stress_test.go`.

### Root Ownership
Primary owner: `specs/025-knowledge-synthesis-layer`, Scope 8, `SCN-025-23` Health endpoint includes knowledge stats.

This residual is outside BUG-039 because BUG-039 recommendation stress passed in the same upstream full-stress phase. It is also outside the closed BUG-025-001 and BUG-025-002 packets because the current failure is latency on `/api/health`, not empty-store `/api/knowledge/stats` HTTP 500 and not external URL extraction in the synthesis E2E.

### Open Technical Cause
The precise technical cause is not confirmed in this classification pass. The implementation owner must reproduce the red state and determine whether the first broken contract is one or more of these knowledge-owned paths:

- `GET /api/health` performs too much synchronous work when authenticated callers receive the knowledge section.
- `GetKnowledgeHealthStats` is slow on the disposable stress data shape, especially with many pending syntheses.
- The knowledge health cache is not warmed or not effective in the rapid stress sequence.
- The stress assertion is stricter than the parent feature's P95-style latency requirement and should be reconciled without hiding real slowness.
- Cold live-stack behavior or concurrent probe work around the first rapid health calls pushes the endpoint over the budget.

## Fix Design

### Solution Approach
Start by reproducing the full-stress red condition with the current repository state, after shared readiness and the BUG-039 recommendation workload are confirmed green. Then isolate where the 2 second budget is spent: HTTP handler probe fan-out, authenticated health topology, knowledge stats lookup, cache behavior, or serialization under slow knowledge stats.

The fix must preserve the knowledge section contract while keeping health fast:

1. Capture red-stage full-stress output that shows `TestKnowledge_HealthEndpointIncludesKnowledgeSection` failing after readiness and after the recommendation workload is green.
2. Add focused timing/diagnostic evidence for `/api/health`, including whether the first call is cold-cache-only or repeated calls are all affected.
3. Inspect and, if needed, repair `internal/api/health.go::getCachedKnowledgeHealth`, `internal/knowledge/store.go::GetKnowledgeHealthStats`, and the stress test's budget semantics.
4. Add or strengthen an adversarial regression around slow knowledge health stats so health checks cannot serialize into multi-second total latency.
5. Re-run full stress, integration, E2E, unit, regression-quality, artifact lint, traceability guard, and validate-owned transition checks.

### Alternative Approaches Considered
1. Reopen BUG-039: rejected because the recommendation workload passed and the failing test is knowledge-owned.
2. Reopen BUG-025-001: rejected because the current failure is latency on `/api/health`, not `/api/knowledge/stats` HTTP 500 on an empty store.
3. Reopen BUG-025-002: rejected because no external URL extraction failure appears in the current residual.
4. Remove the knowledge section from health: rejected because parent Scope 8 requires it for authenticated health callers.
5. Add a silent skip or broad tolerance bump without root cause: rejected because it would hide a stress-budget regression rather than proving health behavior.

## Change Boundary

Allowed implementation surfaces:

- `internal/api/health.go`
- `internal/api/health_test.go`
- `internal/knowledge/store.go` and focused knowledge health stats helpers if reproduction proves query ownership
- `tests/stress/knowledge_stress_test.go`
- `tests/e2e/knowledge_health_test.go`
- This bug packet under `specs/025-knowledge-synthesis-layer/bugs/BUG-025-003-health-endpoint-stress-budget/`

Excluded surfaces:

- BUG-039 recommendation runtime, store, stress tests, and bug artifacts
- BUG-031 shared stress readiness lifecycle and Docker/runtime scripts
- `config/generated/**`
- Docker Compose lifecycle files unless a new shared readiness bug is opened with evidence
- Parent 025 certification fields unless `bubbles.validate` owns the transition

## Regression Test Design

- Red-stage stress reproduction: full repo stress reaches `TestStressReadinessCanary_Live` pass and recommendation stress green evidence, then fails in `TestKnowledge_HealthEndpointIncludesKnowledgeSection` with health calls just above 2 seconds.
- Scenario-specific stress green proof: the same test performs 25 rapid `/api/health` calls, returns HTTP 200, preserves the knowledge behavior, and satisfies the final accepted latency contract.
- Contract preservation proof: unit/E2E tests still show the knowledge section for authenticated callers and preserve core health fields.
- Adversarial slow-path proof: a slow, cold, or cache-expired knowledge stats path must not serialize concurrent or rapid health checks into multi-second response latency.
- No-bailout proof: regression-quality guard and scans must show no test path silently returns when the health latency failure condition appears.

## Validation Plan

Owning sequence:

1. `bubbles.implement`: reproduce, diagnose, implement the minimal knowledge-owned fix and regression tests.
2. `bubbles.test`: run targeted health, unit, integration, E2E, full stress, regression-quality, and no-bailout scans using repo-standard commands.
3. `bubbles.validate`: certify only after artifact lint, traceability guard, state-transition guard, and residual ownership checks pass.