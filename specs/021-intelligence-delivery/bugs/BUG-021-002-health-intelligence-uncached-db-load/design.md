# Design: BUG-021-002 — HealthHandler intelligence-probe TTL cache

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Current Truth (objective baseline before fix)

`HealthHandler` runs two DB queries per request against `IntelligenceEngine`:

```go
// internal/api/health.go (lines 343-368)
if d.IntelligenceEngine != nil {
    if d.IntelligenceEngine.Pool == nil {
        services["intelligence"] = ServiceStatus{Status: "down"}
    } else {
        lastSynthesis, err := d.IntelligenceEngine.GetLastSynthesisTime(ctx)
        if err != nil {
            slog.Warn("intelligence freshness check failed", "error", err)
            services["intelligence"] = ServiceStatus{Status: "up"}
        } else if lastSynthesis.IsZero() || lastSynthesis.Year() < 2000 {
            services["intelligence"] = ServiceStatus{Status: "up"}
        } else if time.Since(lastSynthesis) > 48*time.Hour {
            services["intelligence"] = ServiceStatus{Status: "stale"}
        } else {
            services["intelligence"] = ServiceStatus{Status: "up"}
        }

        staleAlerts, err := d.IntelligenceEngine.HasStalePendingAlerts(ctx, 30*time.Minute)
        if err != nil {
            slog.Warn("alert delivery freshness check failed", "error", err)
        } else if staleAlerts {
            services["alert_delivery"] = ServiceStatus{Status: "stale"}
        } else {
            services["alert_delivery"] = ServiceStatus{Status: "up"}
        }
    }
}
```

The sibling `getCachedKnowledgeHealth` (lines 446-481) already shows the canonical RWMutex+TTL pattern in the same file. It reuses the existing `MLHealthCacheTTLS` SST config value via the `KnowledgeHealthCacheTTL` Dependencies field.

## Solution

Mirror `getCachedKnowledgeHealth` for the intelligence/alert-delivery probe pair.

### Data structures (added to `Dependencies` in `internal/api/health.go`)

```go
IntelligenceHealthCacheTTL time.Duration

intelligenceHealthMu    sync.RWMutex
intelligenceHealthCache *intelligenceHealthSnapshot
intelligenceHealthAt    time.Time
```

```go
// intelligenceHealthSnapshot caches the result of HealthHandler's two
// intelligence DB queries. intelligenceStatus is always populated when the
// snapshot is valid ("up" | "down" | "stale"). alertDeliveryStatus is empty
// when the alert-delivery probe errored, preserving the pre-cache behaviour of
// omitting "alert_delivery" from the response map in that case.
type intelligenceHealthSnapshot struct {
    intelligenceStatus  string
    alertDeliveryStatus string
}
```

### Helper (added near `getCachedKnowledgeHealth`)

```go
// getCachedIntelligenceHealth returns a cached intelligence/alert-delivery
// snapshot, refreshing when stale. Mirrors getCachedKnowledgeHealth's
// RWMutex + TTL pattern (BUG-021-002 — stabilize R13). Without caching, every
// /api/health request triggered two synchronous DB round-trips (GetLastSynthesisTime
// + HasStalePendingAlerts) for data that updates at most once per 24h
// (synthesis) or every 15 min (alert sweep).
func (d *Dependencies) getCachedIntelligenceHealth(ctx context.Context) *intelligenceHealthSnapshot {
    // Fast path: serve from cache under read lock.
    d.intelligenceHealthMu.RLock()
    if d.IntelligenceHealthCacheTTL > 0 && d.intelligenceHealthCache != nil &&
        time.Since(d.intelligenceHealthAt) < d.IntelligenceHealthCacheTTL {
        cached := d.intelligenceHealthCache
        d.intelligenceHealthMu.RUnlock()
        return cached
    }
    d.intelligenceHealthMu.RUnlock()

    // Slow path: compute fresh snapshot without holding any lock.
    snapshot := &intelligenceHealthSnapshot{}
    if d.IntelligenceEngine.Pool == nil {
        snapshot.intelligenceStatus = "down"
    } else {
        lastSynthesis, err := d.IntelligenceEngine.GetLastSynthesisTime(ctx)
        if err != nil {
            slog.Warn("intelligence freshness check failed", "error", err)
            snapshot.intelligenceStatus = "up"
        } else if lastSynthesis.IsZero() || lastSynthesis.Year() < 2000 {
            snapshot.intelligenceStatus = "up"
        } else if time.Since(lastSynthesis) > 48*time.Hour {
            snapshot.intelligenceStatus = "stale"
        } else {
            snapshot.intelligenceStatus = "up"
        }

        staleAlerts, err := d.IntelligenceEngine.HasStalePendingAlerts(ctx, 30*time.Minute)
        if err != nil {
            slog.Warn("alert delivery freshness check failed", "error", err)
            // leave alertDeliveryStatus empty (pre-cache parity)
        } else if staleAlerts {
            snapshot.alertDeliveryStatus = "stale"
        } else {
            snapshot.alertDeliveryStatus = "up"
        }
    }

    // Update cache under write lock.
    d.intelligenceHealthMu.Lock()
    d.intelligenceHealthCache = snapshot
    d.intelligenceHealthAt = time.Now()
    d.intelligenceHealthMu.Unlock()

    return snapshot
}
```

### Call-site refactor in `HealthHandler`

```go
// Intelligence engine status — runs while external probes are in flight.
// Both DB queries are TTL-cached via getCachedIntelligenceHealth to avoid
// hammering Postgres on every /api/health request (BUG-021-002).
if d.IntelligenceEngine != nil {
    snap := d.getCachedIntelligenceHealth(ctx)
    services["intelligence"] = ServiceStatus{Status: snap.intelligenceStatus}
    if snap.alertDeliveryStatus != "" {
        services["alert_delivery"] = ServiceStatus{Status: snap.alertDeliveryStatus}
    }
}
```

### Wiring (`cmd/core/wiring.go`)

Add one line to the `&api.Dependencies{...}` literal, next to the existing `KnowledgeHealthCacheTTL`:

```go
KnowledgeHealthCacheTTL:         time.Duration(cfg.MLHealthCacheTTLS) * time.Second,
IntelligenceHealthCacheTTL:      time.Duration(cfg.MLHealthCacheTTLS) * time.Second,
```

### Why reuse `MLHealthCacheTTLS`?

- It is already an SST-managed config knob.
- It already governs the knowledge-layer cache in the same handler.
- Operators do not need a second TTL dial to manage the same handler's two cached subsections.
- Avoids adding another config field (and the associated `validate_test.go` + smackerel.yaml + generated env file churn).

If a future spec needs an independent TTL for intelligence health, it can split the field — the `IntelligenceHealthCacheTTL` Dependencies field is already independent, only the wiring assignment shares the SST source.

## Invariants Preserved

| Invariant | Mechanism |
|-----------|-----------|
| Nil engine omits "intelligence" key | Call-site `if d.IntelligenceEngine != nil` guard kept |
| Nil pool returns `{intelligence: "down"}` (no "alert_delivery") | `snapshot.intelligenceStatus = "down"` branch + cache write preserves omission |
| Synthesis query error returns "up" | Slow-path error branch unchanged |
| `IsZero` / `Year()<2000` returns "up" | Slow-path epoch branch unchanged |
| `> 48h` returns "stale" (SCN-021-012) | Slow-path threshold branch unchanged |
| `≤ 48h` returns "up" (SCN-021-013) | Slow-path threshold branch unchanged |
| `HasStalePendingAlerts == true` returns alert_delivery="stale" | Slow-path branch unchanged |
| `HasStalePendingAlerts == false` returns alert_delivery="up" | Slow-path branch unchanged |
| Alert query error omits "alert_delivery" key | `snapshot.alertDeliveryStatus = ""` + call-site `if != ""` guard |
| Concurrent health requests do not serialize on cache | RWMutex; slow-path computed without holding lock |
| Cache disabled (TTL=0) → always-fresh | Fast-path `IntelligenceHealthCacheTTL > 0` guard fails ⇒ slow path each call |

## Concurrency Notes

- Read lock is held only for the cache-hit branch; if the TTL is exceeded the read lock is released before the slow path begins.
- Slow path computes the snapshot with **no locks held** (the DB calls are the slow part — locking around them would re-introduce the bug `C-023-C001` solved).
- A brief race window exists where two concurrent slow paths can both compute and both write — the second write wins. This is identical to the knowledge cache and is acceptable because both writes carry an equally fresh snapshot.
- `sync.RWMutex` zero value is usable, matching the `knowledgeHealthMu` precedent.

## Test Strategy

Tests use direct same-package field access on `Dependencies`:

1. **Cache hit:** Pre-seed `d.intelligenceHealthCache` + `d.intelligenceHealthAt = time.Now()` + `d.IntelligenceHealthCacheTTL = 1 * time.Minute`. Set `d.IntelligenceEngine.Pool = nil` (which would normally return "down"). Call handler. Assert response uses the pre-seeded snapshot ("up"), not "down".
2. **Cache expiry:** Pre-seed `d.intelligenceHealthCache` + `d.intelligenceHealthAt = time.Now().Add(-2 * time.Minute)` + `d.IntelligenceHealthCacheTTL = 1 * time.Minute`. Set `Pool = nil`. Assert refresh: response is "down".
3. **Cache disabled (TTL=0):** Pre-seed `d.intelligenceHealthCache` with "up". Set `IntelligenceHealthCacheTTL = 0` and `Pool = nil`. Assert always-fresh: response is "down".
4. **Nil-pool preserved:** No pre-seed. `Pool = nil`. `TTL = 0`. Assert "down". Also assert no "alert_delivery" key.
5. **Nil-engine preserved:** `IntelligenceEngine = nil`. Assert no "intelligence" key in response.
6. **Snapshot stores both subsections:** Pre-seed snapshot with `intelligenceStatus: "stale"`, `alertDeliveryStatus: "stale"`. Assert response includes both.

## Risk

- **Cache staleness up to TTL:** A stale-alert situation may take up to `MLHealthCacheTTLS` seconds (default 30s) to surface in `/api/health`. This is the same trade-off accepted for the knowledge cache and is well within the SLOs (which target minute-scale freshness, not sub-second).
- **Memory:** One `intelligenceHealthSnapshot` per `Dependencies` instance (≈ 48 bytes). Negligible.
- **Lock contention:** RWMutex allows unbounded concurrent readers; only writers serialize. The write window is tiny (pointer + time assignment).

## Test Plan Mapping

See `scopes.md` Test Plan table for SCN-021-FIX-002A through SCN-021-FIX-002F.
