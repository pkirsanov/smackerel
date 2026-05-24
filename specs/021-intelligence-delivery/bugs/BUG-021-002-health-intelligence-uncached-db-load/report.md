# Report: BUG-021-002 — HealthHandler intelligence-probe TTL cache

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

HealthHandler's intelligence subsection invoked two un-cached PostgreSQL probes (`IntelligenceEngine.GetLastSynthesisTime` + `HasStalePendingAlerts`) on every `/api/health` request. The knowledge subsection had a TTL cache (added by C-023-C001) but the intelligence subsection did not, so under default polling (30 s) every hit cost two DB round-trips. The fix layers the same `sync.RWMutex` + snapshot-TTL pattern used by `getCachedKnowledgeHealth` onto the two intelligence probes, reusing the existing `ML_HEALTH_CACHE_TTL_S` SST config (no new env var). All pre-existing behaviour — nil engine omits the section; nil pool reports `down` and omits `alert_delivery`; >48h or epoch synthesis reports `stale` or `up`; alert errors omit `alert_delivery` — is preserved.

## Discovery

Discovered by stochastic-quality-sweep round 13 (parent run `sweep-2026-05-23-r30`, trigger `stabilize`, mapped child mode `stabilize-to-doc`, executed parent-expanded by `bubbles.workflow` because the nested runtime lacked `runSubagent`).

The R13 stabilize probe widened the lens beyond R6/R16 (which scoped to `internal/intelligence`, `internal/scheduler`, `internal/digest`) and audited the **API-handler call-site density** on `IntelligenceEngine` methods. The fan-in pattern (every `/api/health` request → 2 DB round-trips) matched the same anti-pattern that `C-023-C001` already fixed for the knowledge-layer health subsection.

## Evidence Index

- Implementation: `internal/api/health.go`, `cmd/core/wiring.go`
- Tests: `internal/api/health_test.go`
- Sibling pattern reference: `getCachedKnowledgeHealth` (`internal/api/health.go:446-481`)
- SST config reuse: `MLHealthCacheTTLS` (`internal/config/config.go:60`, `internal/config/config.go:858`)

## Phase Evidence

### Implementation Evidence

Three files changed:

1. `internal/api/health.go`:
   - Added 4 fields to `Dependencies` immediately after the existing knowledge cache fields:
     - `IntelligenceHealthCacheTTL time.Duration` — public, wired from `MLHealthCacheTTLS`.
     - `intelligenceHealthMu sync.RWMutex` — fast-path RLock, slow-path Lock (mirrors `knowledgeHealthMu`).
     - `intelligenceHealthCache *intelligenceHealthSnapshot` — cached snapshot pointer.
     - `intelligenceHealthAt time.Time` — last refresh timestamp.
   - Replaced the inline `if d.IntelligenceEngine != nil { ... GetLastSynthesisTime / HasStalePendingAlerts ... }` block in `HealthHandler` with a single `d.getCachedIntelligenceHealth(ctx)` call that returns `(intelligenceStatus, alertDeliveryStatus string, alertDeliveryPresent bool)` and preserves the exact same `services["intelligence"]` / `services["alert_delivery"]` write semantics (omitting `alert_delivery` when the snapshot reports it absent).
   - Added `type intelligenceHealthSnapshot struct { intelligenceStatus string; alertDeliveryStatus string }` (`alertDeliveryStatus == ""` encodes the omitted-section case).
   - Added `getCachedIntelligenceHealth` helper directly after `getCachedKnowledgeHealth`, structurally identical: fast-path RLock checks `IntelligenceHealthCacheTTL > 0 && cache != nil && time.Since(at) < TTL`; slow-path runs both probes with no lock held, then write-lock updates `intelligenceHealthCache` + `intelligenceHealthAt`. Slow-path preserves all pre-existing branches: nil pool → `("down", "", false)`; synthesis-error → `("up", "up", true)`; epoch / Year<2000 → `("up", "up", true)`; >48h → `("stale", probeStaleAlertsAndReturn(), present)`; ≤48h → `("up", probeStaleAlertsAndReturn(), present)`; alert-error during stale-check → `(status, "", false)`.

2. `cmd/core/wiring.go`:
   - Added `IntelligenceHealthCacheTTL: time.Duration(cfg.MLHealthCacheTTLS) * time.Second,` immediately after the existing `KnowledgeHealthCacheTTL:` line in `buildAPIDeps`. SST: reuses the existing `MLHealthCacheTTLS` config knob (default 30 s); no new env var, no `smackerel.yaml` change, no `validate_test.go` change.

3. `internal/api/health_test.go`:
   - Added 6 new tests (SCN-021-FIX-002A through 002F) immediately after `TestHealthHandler_IntelligenceDownDegrades` and before the C-023-C001 chaos test block. Tests exploit same-package field access on `Dependencies` to pre-seed `intelligenceHealthCache` + `intelligenceHealthAt` and assert cache behaviour without depending on a live pgxpool.

```text
$ git diff --stat HEAD -- internal/api/health.go internal/api/health_test.go cmd/core/wiring.go
 cmd/core/wiring.go             |   1 +
 internal/api/health.go         |  93 +++++++++++++++++++++++++++--
 internal/api/health_test.go    | 196 +++++++++++++++++++++++++++++++++++++++++
 3 files changed, 287 insertions(+), 3 deletions(-)
```

### Test Evidence

All 6 new SCN-021-FIX-002* tests pass with race detector enabled.

```text
$ go test -count=1 -race -timeout 60s -v ./internal/api/... -run 'IntelligenceCache|IntelligenceNilPool'
=== RUN   TestHealthHandler_IntelligenceCacheHit
--- PASS: TestHealthHandler_IntelligenceCacheHit (0.00s)
=== RUN   TestHealthHandler_IntelligenceCacheExpired
--- PASS: TestHealthHandler_IntelligenceCacheExpired (0.00s)
=== RUN   TestHealthHandler_IntelligenceCacheDisabled
--- PASS: TestHealthHandler_IntelligenceCacheDisabled (0.00s)
=== RUN   TestHealthHandler_IntelligenceNilPool_OmitsAlertDelivery
--- PASS: TestHealthHandler_IntelligenceNilPool_OmitsAlertDelivery (0.00s)
=== RUN   TestHealthHandler_IntelligenceCacheStaleSubsections
--- PASS: TestHealthHandler_IntelligenceCacheStaleSubsections (0.00s)
=== RUN   TestHealthHandler_IntelligenceCacheConcurrentReaders
--- PASS: TestHealthHandler_IntelligenceCacheConcurrentReaders (0.01s)
PASS
ok      github.com/smackerel/smackerel/internal/api     1.151s
```

Full affected-package regression (api + intelligence + scheduler) also clean:

```text
$ go test -count=1 -race -timeout 240s ./internal/api/... ./internal/intelligence/... ./internal/scheduler/...
ok      github.com/smackerel/smackerel/internal/api     16.615s
ok      github.com/smackerel/smackerel/internal/intelligence    1.114s
ok      github.com/smackerel/smackerel/internal/scheduler       6.121s
```

### Validation Evidence

Build and vet clean across the full module:

```text
$ go build ./...
(no output)

$ go vet ./...
(no output)
```

Pre-existing intelligence tests still pass without modification, proving the fast-path RLock + slow-path snapshot preserves the original branch tree:

- `TestHealthHandler_IntelligenceDown` (nil pool → `down`, no `alert_delivery`)
- `TestHealthHandler_IntelligenceNilEngine` (nil engine → section omitted entirely — preserved by the unchanged `if d.IntelligenceEngine != nil` guard at the call site)
- `TestHealthHandler_IntelligenceStalenessThreshold` (>48 h → `stale`)
- `TestHealthHandler_IntelligenceDownDegrades` (intelligence-down → overall `degraded`)

### Audit Evidence

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/021-intelligence-delivery/bugs/BUG-021-002-health-intelligence-uncached-db-load
(see commit terminal output — final result PASS after Summary/Completion Statement/Checklist sections added)

$ bash .github/bubbles/scripts/artifact-lint.sh specs/021-intelligence-delivery
(passes — parent spec untouched except for the Round 13 Stabilize Sweep section appended to report.md)

$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/021-intelligence-delivery
(passes — no scope/scenario IDs changed on the parent spec; new SCN-021-FIX-002* IDs are bug-local)
```

### Docs Evidence

No customer- or operator-facing docs change. The fix is internal: same SST knob, same response shape, same staleness semantics. The Round 13 entry on the parent spec's `report.md` is the only docs surface touched.

## Completion Statement

BUG-021-002 is closed. STAB-021-R13-001 has a one-to-one closure with this bug. All 6 SCN-021-FIX-002* scenarios pass under `-race`. Pre-existing intelligence-health tests pass without modification. Build, vet, and the full affected-package regression (api + intelligence + scheduler) are clean. Spec 021's parent state remains `done` (this bug did not regress the parent).

