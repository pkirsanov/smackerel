# User Validation: BUG-021-002 — HealthHandler intelligence-probe TTL cache

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Scope

Stabilization defect — no externally observable functional change. The fix preserves all pre-existing `/api/health` response shapes (intelligence={up,down,stale}, alert_delivery={up,stale,omitted}) and only changes the upstream DB load profile.

## Acceptance Surface

- Operator perspective: `/api/health` latency under polling load drops because the two intelligence DB probes are now TTL-cached at the existing `ML_HEALTH_CACHE_TTL_S` interval (same TTL as the knowledge subsection).
- Operator perspective: stale-state surfacing latency is bounded by the TTL (≤ 30 s by default) — this is the same trade-off already accepted for the knowledge-layer cache.
- End-user perspective: no change. Health endpoint is not user-visible.

## Validation Steps

1. Confirm pre-existing intelligence-section health-check semantics (SCN-021-012, SCN-021-013, IntelligenceDown, IntelligenceNilEngine) still pass without modification.
2. Confirm the six new SCN-021-FIX-002* scenarios pass.
3. Confirm artifact-lint and traceability-guard PASS for the parent spec.

## Checklist

- [x] Acceptance surface validated against the as-built code (sibling cache pattern reused from C-023-C001 / `getCachedKnowledgeHealth`).
- [x] No user-facing response-shape change confirmed via existing tests (`TestHealthHandler_IntelligenceDown`, `TestHealthHandler_IntelligenceNilEngine`, `TestHealthHandler_IntelligenceStalenessThreshold`, `TestHealthHandler_IntelligenceDownDegrades` all still pass without modification).
- [x] TTL parity with the knowledge subsection confirmed (both wired from `cfg.MLHealthCacheTTLS`).
- [x] Stale-state surfacing latency trade-off (≤ 30 s default) explicitly accepted and matches the pre-existing knowledge-cache contract.

## Sign-off

Accepted as a stabilization fix with no externally observable behavior change. Closed under bugfix-fastlane.
