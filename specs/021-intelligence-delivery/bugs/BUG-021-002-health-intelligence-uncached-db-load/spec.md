# Bug: BUG-021-002 — HealthHandler runs uncached intelligence engine DB queries per request (STAB-021-R13-001)

## Classification

- **Type:** Stabilization defect — uncached synchronous DB queries on /api/health hot path
- **Severity:** MEDIUM (no functional bug; resource/latency stability regression risk under polling load)
- **Parent Spec:** 021 — Intelligence Delivery
- **Workflow Mode:** bugfix-fastlane (parent-expanded child of stochastic-quality-sweep round 13, trigger `stabilize`, mapped mode `stabilize-to-doc`)
- **Status:** Open — discovered by stabilize R13 (sweep `sweep-2026-05-23-r30`)

## Problem Statement

`HealthHandler` (`internal/api/health.go`) calls two intelligence-engine database queries synchronously inside the request goroutine on **every** `/api/health` request:

1. `d.IntelligenceEngine.GetLastSynthesisTime(ctx)` — `SELECT COALESCE(MAX(created_at), '1970-01-01'::timestamptz) FROM synthesis_insights` (`internal/intelligence/synthesis.go:376`)
2. `d.IntelligenceEngine.HasStalePendingAlerts(ctx, 30*time.Minute)` — `SELECT COUNT(*) FROM alerts WHERE status='pending' AND created_at < NOW() - MAKE_INTERVAL(secs => $1)` (`internal/intelligence/alerts.go::HasStalePendingAlerts`)

Neither call is cached. Both contend for the same `pgxpool` connections as live traffic and the cron scheduler.

The underlying data sources update infrequently:

- `synthesis_insights` writes are produced by the daily synthesis cron job (~once per 24h).
- `alerts` writes are produced by the alert delivery sweep (every 15 minutes by default).

A request-time poll is therefore **two orders of magnitude more frequent** than the data it observes.

### Comparison with sibling pattern

The same file already implements `getCachedKnowledgeHealth` (lines 446-481) which uses `sync.RWMutex` + `KnowledgeHealthCacheTTL` (sourced from the existing `ML_HEALTH_CACHE_TTL_S` SST contract). That caching layer was introduced for `C-023-C001` to avoid exactly the same anti-pattern on the knowledge-layer health subsection. The intelligence/alert-delivery subsection of `HealthHandler` was missed.

## Impact

| Axis | Impact |
|------|--------|
| **Performance** | Each `/api/health` request bounds its floor latency to 2× DB round-trip time, sequentially (the two queries are not parallelised). With Docker HEALTHCHECK polling every 30s and any external monitor (Prometheus, Tailscale uptime, etc.) adding additional pollers, p99 latency under DB contention can exceed Docker's default 3 s HEALTHCHECK timeout. |
| **Resource (DB)** | Conservatively, HEALTHCHECK 30s + 2 external monitors at 60s ≈ 4 320 health requests/day ⇒ **8 640 redundant intelligence DB round-trips per day** for data that updates ≤ 96 times/day (alerts every 15 min). Ratio: ~90× over-fetch. |
| **Reliability** | Under DB pressure, the two queries amplify backpressure: every health probe consumes pool connections needed by real traffic and the cron scheduler. This is the canonical fault pattern that `C-023-C001` already fixed for the knowledge subsection. |
| **Observability noise** | Slow query logs and connection-acquisition latency metrics get polluted by health-check fan-out. |

## Why this is "stabilize" not "performance"

Per `stabilize-to-doc` charter: **performance/reliability/resource probe with no functional defect**. The handler is correct; it is just structurally susceptible to the polling-load anti-pattern. The fix is structural (cache the two probes for the same TTL window as the existing knowledge-layer cache).

## Why prior stabilize rounds missed it

- **R6 stabilize (2026-05-12)** scoped to `internal/intelligence`, `internal/scheduler`, `internal/digest` packages — audited cron topology, mutex graph, shutdown discipline, row-close coverage, goroutine leak risk. Did **not** scan API handler call-sites.
- **R16 stabilize (2026-05-13)** explicitly logged in `report.md` lines 895-989 that scope was `internal/intelligence`, `internal/scheduler`, `internal/digest`. Audited 9 stability invariants. Did **not** include `internal/api/health.go` call-site density on `IntelligenceEngine`.

R13 (this round) extends the stabilize lens to **API-handler call-sites that hit intelligence-engine DB methods** and discovered the uncached probe pair.

## Reproduction (pre-fix)

```text
$ grep -nE "IntelligenceEngine\.(GetLastSynthesisTime|HasStalePendingAlerts)" internal/api/health.go
344:			lastSynthesis, err := d.IntelligenceEngine.GetLastSynthesisTime(ctx)
360:			staleAlerts, err := d.IntelligenceEngine.HasStalePendingAlerts(ctx, 30*time.Minute)

$ grep -nE "knowledgeHealthCache|intelligenceHealthCache" internal/api/health.go
197:	knowledgeHealthMu    sync.RWMutex
198:	knowledgeHealthCache *KnowledgeHealthSection
199:	knowledgeHealthAt    time.Time
446:func (d *Dependencies) getCachedKnowledgeHealth(ctx context.Context) *KnowledgeHealthSection {
# No matches for intelligenceHealthCache — caching infra exists for knowledge layer only.
```

## Acceptance Criteria

- [ ] `internal/api/health.go::Dependencies` gains `IntelligenceHealthCacheTTL time.Duration` plus private `intelligenceHealthMu sync.RWMutex`, `intelligenceHealthCache *intelligenceHealthSnapshot`, `intelligenceHealthAt time.Time` fields, mirroring the existing knowledge cache fields.
- [ ] A new package-private helper `getCachedIntelligenceHealth(ctx)` mirrors the structure of `getCachedKnowledgeHealth` (fast-path RLock TTL check, slow-path compute under no lock, write-lock cache update).
- [ ] `HealthHandler` calls the cached helper instead of invoking `GetLastSynthesisTime` / `HasStalePendingAlerts` directly.
- [ ] All pre-existing externally-observable behaviour is preserved exactly: nil-engine returns no `intelligence` key; nil-pool returns `{intelligence: "down"}`; epoch / `Year()<2000` returns `up`; > 48h returns `stale`; alert-probe error omits `alert_delivery` from the response.
- [ ] `cmd/core/wiring.go` wires `IntelligenceHealthCacheTTL` to `time.Duration(cfg.MLHealthCacheTTLS) * time.Second` (same SST contract as `KnowledgeHealthCacheTTL`).
- [ ] New unit tests demonstrate (a) TTL cache hit reuses snapshot without DB roundtrip, (b) TTL cache expiry triggers refresh, (c) TTL=0 disables caching (always-fresh path), (d) nil pool still returns "down", (e) 48h staleness threshold (SCN-021-012) preserved, (f) fresh-install epoch handling (SCN-021-013) preserved.
- [ ] `go test -count=1 -race ./internal/api/... ./internal/intelligence/... ./internal/scheduler/...` PASS.
- [ ] `go vet ./...` and `go build ./...` clean.
- [ ] `artifact-lint.sh` and `traceability-guard.sh` PASS for the parent spec and this bug folder.

## Non-Goals

- Adding a new SST config knob (the existing `ML_HEALTH_CACHE_TTL_S` is reused — same TTL contract as the knowledge cache).
- Changing the 48h synthesis-staleness threshold or 30-minute alert-staleness threshold.
- Refactoring `IntelligenceEngine` into an interface (concrete pointer is kept; tests exploit same-package field access).
- Changing the parallel-probe topology of `mlStatus` / `ollamaStatus` (already addressed in IMP-023-R19-001).

## Constraint: Boundary

All code edits confined to:

- `internal/api/health.go`
- `internal/api/health_test.go`
- `cmd/core/wiring.go`

No changes to `internal/intelligence/**`, `internal/scheduler/**`, `config/smackerel.yaml`, `internal/config/**`, migrations, or scripts.

## References

- Parent spec: `specs/021-intelligence-delivery/spec.md`
- Sibling pattern: `getCachedKnowledgeHealth` (`internal/api/health.go:446-481`)
- Sibling concern fixed: `C-023-C001` (knowledge-layer health cache)
- SST contract for TTL source: `ML_HEALTH_CACHE_TTL_S` env (`internal/config/config.go:858`, `config/smackerel.yaml:744`)
- Stabilize sweep parent: `sweep-2026-05-23-r30` round 13
