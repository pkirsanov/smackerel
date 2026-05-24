# Scopes: BUG-021-002 — HealthHandler intelligence-probe TTL cache

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: TTL cache the two intelligence DB probes invoked by HealthHandler

**Status:** Done
**Priority:** P1
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-021-FIX-002A TTL cache hit reuses snapshot without DB round-trip
  Given a Dependencies value with IntelligenceHealthCacheTTL = 1 minute
  And intelligenceHealthCache pre-seeded with {intelligenceStatus: "up", alertDeliveryStatus: "up"} at time.Now()
  And the IntelligenceEngine.Pool is nil (a path that would otherwise return "down")
  When the HealthHandler responds to /api/health
  Then the response services["intelligence"].status equals "up"
  And the response services["alert_delivery"].status equals "up"
  And GetLastSynthesisTime was not invoked

Scenario: SCN-021-FIX-002B TTL cache expiry triggers refresh
  Given a Dependencies value with IntelligenceHealthCacheTTL = 1 minute
  And intelligenceHealthCache pre-seeded at time.Now().Add(-2 * time.Minute)
  And the IntelligenceEngine.Pool is nil
  When the HealthHandler responds to /api/health
  Then the response services["intelligence"].status equals "down"

Scenario: SCN-021-FIX-002C Cache disabled (TTL=0) preserves always-fresh path
  Given a Dependencies value with IntelligenceHealthCacheTTL = 0
  And intelligenceHealthCache pre-seeded with {intelligenceStatus: "up"}
  And the IntelligenceEngine.Pool is nil
  When the HealthHandler responds to /api/health
  Then the response services["intelligence"].status equals "down"

Scenario: SCN-021-FIX-002D Nil pool preserved (no alert_delivery key)
  Given a Dependencies value with IntelligenceHealthCacheTTL = 0
  And the IntelligenceEngine.Pool is nil
  When the HealthHandler responds to /api/health
  Then the response services["intelligence"].status equals "down"
  And the response services map has no "alert_delivery" key

Scenario: SCN-021-FIX-002E Nil engine preserved (no intelligence key)
  Given a Dependencies value with IntelligenceEngine = nil
  When the HealthHandler responds to /api/health
  Then the response services map has no "intelligence" key

Scenario: SCN-021-FIX-002F Snapshot stores stale-state for both subsections
  Given a Dependencies value with IntelligenceHealthCacheTTL = 1 minute
  And intelligenceHealthCache pre-seeded with {intelligenceStatus: "stale", alertDeliveryStatus: "stale"} at time.Now()
  When the HealthHandler responds to /api/health
  Then the response services["intelligence"].status equals "stale"
  And the response services["alert_delivery"].status equals "stale"
```

### Implementation Plan

1. Add `IntelligenceHealthCacheTTL time.Duration` plus private `intelligenceHealthMu sync.RWMutex`, `intelligenceHealthCache *intelligenceHealthSnapshot`, `intelligenceHealthAt time.Time` fields to `Dependencies` in `internal/api/health.go`, immediately after the existing knowledge cache fields.
2. Add `type intelligenceHealthSnapshot struct { intelligenceStatus string; alertDeliveryStatus string }` declaration near `KnowledgeHealthSection`.
3. Add `func (d *Dependencies) getCachedIntelligenceHealth(ctx context.Context) *intelligenceHealthSnapshot` helper near `getCachedKnowledgeHealth`, mirroring its RWMutex+TTL pattern: fast-path RLock TTL check, slow-path compute under no lock, write-lock cache update.
4. Refactor the existing intelligence/alert-delivery block in `HealthHandler` (lines ~340-368) to call the cached helper and populate `services["intelligence"]` + optional `services["alert_delivery"]` from the returned snapshot.
5. Wire `IntelligenceHealthCacheTTL: time.Duration(cfg.MLHealthCacheTTLS) * time.Second` into the `&api.Dependencies{...}` literal in `cmd/core/wiring.go`, immediately after the existing `KnowledgeHealthCacheTTL` field assignment.
6. Add Go unit tests in `internal/api/health_test.go` covering SCN-021-FIX-002A through SCN-021-FIX-002F.
7. Run `go test -count=1 -race ./internal/api/... ./internal/intelligence/... ./internal/scheduler/...`, `go vet ./...`, `go build ./...`.
8. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent spec and this bug folder.
9. Run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/021-intelligence-delivery`.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX2-1-01 | TestHealthHandler_IntelligenceCacheHit | unit | `internal/api/health_test.go` | With TTL=1m and a pre-seeded "up" snapshot at time.Now(), nil-pool engine returns response "intelligence"="up" (cache served) | SCN-021-FIX-002A |
| T-FIX2-1-02 | TestHealthHandler_IntelligenceCacheExpired | unit | `internal/api/health_test.go` | With TTL=1m and a snapshot dated 2m ago, nil-pool engine returns "down" (refresh occurred) | SCN-021-FIX-002B |
| T-FIX2-1-03 | TestHealthHandler_IntelligenceCacheDisabled | unit | `internal/api/health_test.go` | With TTL=0 and a pre-seeded "up" snapshot, nil-pool engine returns "down" (cache bypassed) | SCN-021-FIX-002C |
| T-FIX2-1-04 | TestHealthHandler_IntelligenceNilPool_OmitsAlertDelivery | unit | `internal/api/health_test.go` | Nil-pool with TTL=0 returns "intelligence"="down" and no "alert_delivery" key | SCN-021-FIX-002D |
| T-FIX2-1-05 | TestHealthHandler_IntelligenceNilEngine_OmitsKey (existing) | unit | `internal/api/health_test.go` | Nil engine returns response with no "intelligence" key — preserved exactly | SCN-021-FIX-002E |
| T-FIX2-1-06 | TestHealthHandler_IntelligenceCacheStaleSubsections | unit | `internal/api/health_test.go` | Pre-seeded {stale, stale} snapshot surfaces in both keys | SCN-021-FIX-002F |
| T-FIX2-1-07 | Underlying SCN-021-012/013 invariants still pass | unit | `internal/api/health_test.go::TestHealthHandler_IntelligenceStalenessThreshold`, `TestHealthHandler_IntelligenceFreshInstallNotStale`, `TestHealthHandler_IntelligenceDown` | All pre-existing intelligence-section tests continue to PASS unchanged | SCN-021-012, SCN-021-013 |

### Definition of Done

- [x] `Dependencies` in `internal/api/health.go` has `IntelligenceHealthCacheTTL` plus three private cache fields, mirroring the knowledge-cache fields by name and ordering — **Phase:** implement
  > Evidence: `grep -nE "IntelligenceHealthCacheTTL|intelligenceHealthMu|intelligenceHealthCache|intelligenceHealthAt" internal/api/health.go` returns all four fields adjacent to the existing knowledge cache fields in the `Dependencies` struct.
- [x] `intelligenceHealthSnapshot` type is declared in `internal/api/health.go` — **Phase:** implement
  > Evidence: `grep -n "type intelligenceHealthSnapshot" internal/api/health.go` returns the declaration immediately before `getCachedIntelligenceHealth`.
- [x] `getCachedIntelligenceHealth` helper exists in `internal/api/health.go`, uses RWMutex correctly (read lock for fast-path, no lock for slow-path DB calls, write lock for cache update), and preserves the nil-pool / synthesis-error / epoch / 48h / alert-error branches by value — **Phase:** implement
  > Evidence: `grep -n "func (d \*Dependencies) getCachedIntelligenceHealth" internal/api/health.go` returns the helper definition; structural review confirms RLock → check TTL → release → slow-path with no lock → Lock → update cache flow, mirroring `getCachedKnowledgeHealth`. All five branch values (nil pool=down/omit, synth-err=up/up, epoch=up/up, >48h=stale/probe, ≤48h=up/probe) are preserved by direct port.
- [x] `HealthHandler` no longer calls `GetLastSynthesisTime` or `HasStalePendingAlerts` directly; instead it calls `d.getCachedIntelligenceHealth(ctx)` — **Phase:** implement
  > Evidence: `grep -nE "GetLastSynthesisTime|HasStalePendingAlerts" internal/api/health.go` returns matches only inside `getCachedIntelligenceHealth`; the `HealthHandler` block uses `d.getCachedIntelligenceHealth(ctx)` exactly once.
- [x] `cmd/core/wiring.go` sets `IntelligenceHealthCacheTTL: time.Duration(cfg.MLHealthCacheTTLS) * time.Second` in the `&api.Dependencies{...}` literal — **Phase:** implement
  > Evidence: `grep -n "IntelligenceHealthCacheTTL" cmd/core/wiring.go` returns the line immediately after `KnowledgeHealthCacheTTL`.
- [x] All six SCN-021-FIX-002* unit tests are present in `internal/api/health_test.go` and PASS — **Phase:** test
  > Evidence: `go test -count=1 -race -timeout 60s -v ./internal/api/... -run 'IntelligenceCache|IntelligenceNilPool'` shows 6 PASS — see [report.md → Test Evidence](report.md#test-evidence). 002E is covered by the pre-existing `TestHealthHandler_IntelligenceNilEngine` (unchanged) — the call-site `if d.IntelligenceEngine != nil` guard was deliberately preserved.
- [x] All pre-existing SCN-021-012/013 invariant tests still PASS — **Phase:** test
  > Evidence: `go test -count=1 -race -timeout 240s ./internal/api/... ./internal/intelligence/... ./internal/scheduler/...` PASS — see [report.md → Test Evidence](report.md#test-evidence). `TestHealthHandler_IntelligenceDown`, `TestHealthHandler_IntelligenceNilEngine`, `TestHealthHandler_IntelligenceStalenessThreshold`, `TestHealthHandler_IntelligenceFreshInstallNotStale`, `TestHealthHandler_IntelligenceDownDegrades` all pass without source modification.
- [x] `go test -count=1 -race ./internal/api/... ./internal/intelligence/... ./internal/scheduler/...` PASS — **Phase:** validate
  > Evidence: `ok internal/api 16.615s | ok internal/intelligence 1.114s | ok internal/scheduler 6.121s` — see [report.md → Validation Evidence](report.md#validation-evidence).
- [x] `go vet ./...` and `go build ./...` clean — **Phase:** validate
  > Evidence: both commands returned with no output — see [report.md → Validation Evidence](report.md#validation-evidence).
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/021-intelligence-delivery` PASS — **Phase:** audit
  > Evidence: parent spec artifact-lint output captured in [report.md → Audit Evidence](report.md#audit-evidence).
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/021-intelligence-delivery/bugs/BUG-021-002-health-intelligence-uncached-db-load` PASS — **Phase:** audit
  > Evidence: bug-folder artifact-lint output captured in [report.md → Audit Evidence](report.md#audit-evidence) after Summary, Completion Statement, and Checklist sections were added.
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/021-intelligence-delivery` PASS — **Phase:** audit
  > Evidence: traceability-guard output captured in [report.md → Audit Evidence](report.md#audit-evidence). New SCN-021-FIX-002* IDs are bug-local; parent spec's scope/scenario IDs are unchanged.
- [x] Parent `specs/021-intelligence-delivery/report.md` has a "Round 13 Stabilize Sweep" entry referencing BUG-021-002 closure — **Phase:** docs
  > Evidence: section appended to parent report.md (this round) following the R16 structural template.
