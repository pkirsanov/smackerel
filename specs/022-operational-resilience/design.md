# Design: 022 Operational Resilience

## Design Brief

**Current State:** Smackerel stores the user's entire knowledge base in a single PostgreSQL volume with no backup mechanism. The runtime has multiple resilience gaps: capture proceeds without DB health verification, 9 scheduler cron jobs run without concurrency guards on a hardcoded 10-connection pool, NATS consumers discard messages after 5 delivery attempts with no dead-letter routing, shutdown uses defer-based ordering that races NATS drain against DB pool close with a 10s timeout insufficient for Telegram's 30s long-poll, and search blocks indefinitely on NATS when the ML sidecar is unresponsive.

**Target State:** Protect the knowledge base with `./smackerel.sh backup`, make failures visible (503 on DB outage, dead-letter for NATS), prevent resource contention (cron mutex, configurable pool), and ensure clean shutdown in explicit dependency order within Docker's stop_grace_period.

**Patterns to Follow:**
- SST config pipeline: values in `config/smackerel.yaml` → `scripts/commands/config.sh` generates env → Go reads env at startup with fail-loud validation ([config/smackerel.yaml](config/smackerel.yaml), [scripts/commands/config.sh](scripts/commands/config.sh), [internal/config/config.go](internal/config/config.go))
- CLI command surface via `./smackerel.sh` dispatching to `scripts/commands/*.sh` ([smackerel.sh](smackerel.sh))
- `db.Postgres.Healthy()` pattern already exists for health checks ([internal/db/postgres.go](internal/db/postgres.go#L67))
- NATS stream management via `EnsureStreams()` ([internal/nats/client.go](internal/nats/client.go#L89))
- Chi router dependency injection via `api.Dependencies` struct ([internal/api/router.go](internal/api/router.go))

**Patterns to Avoid:**
- Defer-based shutdown ordering in `main.go` (lines 318-328) — defers execute in LIFO which creates race between `nc.Close()` (NATS drain) and `pg.Close()` registered earlier. Must replace with explicit sequential function.
- Hardcoded pool config in `db.Connect()` (`MaxConns = 10`, `MinConns = 2`) — violates SST. Must read from env.
- Inline cron job functions in scheduler without concurrency guards — all 9 jobs can overlap freely.

**Resolved Decisions:**
- Backup uses `pg_dump` via `docker exec` against the running postgres container — no volume snapshot needed
- Dead-letter uses a dedicated `DEADLETTER` JetStream stream with `MaxDeliver` advisory routing
- Cron concurrency uses `sync.Mutex` per job group, not per individual job — allows independent groups to run concurrently
- ML health cache is a simple atomic bool + timestamp in the `SearchEngine` struct, not a circuit breaker
- Shutdown timeout set to 25s in Go code (5s margin before Docker's 30s SIGKILL)

**Open Questions:** None — all design decisions are resolved from spec requirements and codebase analysis.

---

## 1. Architecture Overview

This feature adds eight discrete resilience improvements to the existing runtime. No new services, containers, or external dependencies are introduced. All changes are internal to the Go core, the CLI surface, and Docker Compose configuration.

```
┌──────────────────────────────────────────────────────────────────────┐
│                        User / Caller                                 │
│  ./smackerel.sh backup    POST /api/capture    POST /api/search     │
└──────┬────────────────────────┬────────────────────┬─────────────────┘
       │                        │                    │
       ▼                        ▼                    ▼
┌──────────────┐  ┌─────────────────────┐  ┌──────────────────────┐
│ backup.sh    │  │ CaptureHandler      │  │ SearchHandler         │
│ (pg_dump via │  │ + DB health gate    │  │ + ML health cache     │
│ docker exec) │  │                     │  │ + text fallback       │
└──────────────┘  └─────────────────────┘  └──────────────────────┘
                          │                         │
                          ▼                         ▼
                  ┌───────────────┐         ┌──────────────┐
                  │ PostgreSQL    │         │ NATS          │
                  │ (SST pool)   │         │ + DEADLETTER  │
                  └───────────────┘         │ stream        │
                                            └──────────────┘
                          │
┌─────────────────────────┼──────────────────────┐
│ Scheduler               │  Shutdown Manager     │
│ + per-group mutex       │  + explicit ordering  │
│ (daily/hourly/weekly/   │  (sched→http→tg→nats  │
│  monthly/frequent)      │   →sub→conn→nats→db)  │
└─────────────────────────┴──────────────────────┘
```

### Component Impact Summary

| Component | File(s) | Change Type |
|-----------|---------|-------------|
| Backup CLI | `scripts/commands/backup.sh` (new) | New CLI command |
| Capture handler | `internal/api/capture.go` | Add DB health gate |
| Scheduler | `internal/scheduler/scheduler.go` | Add per-group mutex |
| NATS client | `internal/nats/client.go` | Add DEADLETTER stream |
| Main startup/shutdown | `cmd/core/main.go` | Replace defer shutdown with explicit function |
| Docker Compose | `docker-compose.yml` | Add stop_grace_period |
| Search engine | `internal/api/search.go` | Add ML health cache + fast fallback |
| DB connect | `internal/db/postgres.go` | Read pool config from env |
| Config | `internal/config/config.go` | Add DB pool + health cache TTL + shutdown timeout fields |
| SST config | `config/smackerel.yaml` | Add new config keys |
| Config generator | `scripts/commands/config.sh` | Emit new env vars |

---

## 2. Data Model

No schema migrations required. All changes are runtime configuration and operational behavior. The DEADLETTER stream uses NATS JetStream storage (file-based, already provisioned via `nats-data` volume).

### DEADLETTER Stream Message Format

Dead-letter messages preserve the original payload as the message body. Delivery metadata is carried in NATS headers:

| Header | Description |
|--------|-------------|
| `Smackerel-Original-Subject` | Original NATS subject (e.g., `artifacts.processed`) |
| `Smackerel-Original-Stream` | Source stream name (e.g., `ARTIFACTS`) |
| `Smackerel-Original-Consumer` | Consumer durable name |
| `Smackerel-Failed-At` | RFC3339 timestamp of final failure |
| `Smackerel-Delivery-Count` | Number of delivery attempts before exhaustion |
| `Smackerel-Last-Error` | Last error message (truncated to 256 bytes) |

---

## 3. API / Contract Changes

### 3.1 POST /api/capture — DB Health Gate

**Change:** Add a `db.Healthy(ctx)` check at the top of `CaptureHandler`, before any processing.

**New error response** (added to existing error model):

```json
{
  "error": {
    "code": "DB_UNAVAILABLE",
    "message": "Database is temporarily unavailable, please retry"
  }
}
```

- HTTP status: **503 Service Unavailable**
- Condition: `pg.Pool.Ping(ctx)` fails within a 2s timeout (uses existing `Healthy()` method on `db.Postgres`)
- Latency impact: <5ms when DB is healthy (pool connection check, not a full round-trip query)

### 3.2 POST /api/search — ML Health Cache

**Change:** Before attempting the NATS embed request, check a cached ML sidecar health status. If unhealthy, skip directly to `textSearch()`.

**Response shape unchanged.** The existing `search_mode` field already returns `"text_fallback"` when embedding fails. This change makes the fallback instantaneous rather than waiting for a 2s NATS timeout.

**New `SearchEngine` fields:**

```go
type SearchEngine struct {
    Pool           *pgxpool.Pool
    NATS           *smacknats.Client
    MLSidecarURL   string           // from config, for health check HTTP GET
    HealthCacheTTL time.Duration    // from SST config

    mlHealthy      atomic.Bool      // cached health status
    mlHealthAt     atomic.Int64     // unix nanos of last health check
}
```

**Health check mechanism:**
1. On each search request, check if `time.Now().UnixNano() - mlHealthAt > HealthCacheTTL`
2. If stale: perform `GET {MLSidecarURL}/health` with 1s timeout, update cache
3. If `mlHealthy == false`: skip NATS embed, return `textSearch()` with `search_mode: "text_fallback"`
4. Background: no separate goroutine; cache is refreshed lazily on search requests

### 3.3 CLI: ./smackerel.sh backup

**New command** routed through the existing CLI dispatcher.

```
Usage: ./smackerel.sh backup [--env dev|test]

Output: backups/smackerel-YYYY-MM-DD-HHMMSS.sql.gz
Exit 0: success (prints file path and size)
Exit 1: failure (prints error to stderr)
```

**Implementation:** `scripts/commands/backup.sh` performs:
1. Source the generated env file for DB credentials (`config/generated/{env}.env`)
2. Resolve the Compose project name from env
3. `docker exec` the postgres container to run `pg_dump`
4. Pipe through `gzip` and write to `backups/` directory
5. Validate output file is non-empty

---

## 4. Backup Command Design

### 4.1 scripts/commands/backup.sh

```
Flow:
1. Parse --env flag (default: dev)
2. Source config/generated/{env}.env
3. Validate required vars: COMPOSE_PROJECT, POSTGRES_USER, POSTGRES_DB
4. Create backups/ directory if missing
5. Generate filename: smackerel-$(date -u +%Y-%m-%d-%H%M%S).sql.gz
6. Run: docker exec ${COMPOSE_PROJECT}-postgres-1 pg_dump -U ${POSTGRES_USER} -d ${POSTGRES_DB} --clean --if-exists | gzip > backups/${filename}
7. Check exit code and file size > 0
8. Print: "Backup created: backups/${filename} (${size})"
```

**Error cases:**
- Postgres container not running → `docker exec` fails → exit 1 with "database container is not running"
- pg_dump fails → non-zero exit → exit 1 with "pg_dump failed"
- Empty output file → exit 1 with "backup file is empty — dump may have failed"

**Security:** The backup file contains all database content including any stored tokens. The `backups/` directory is already gitignored (verify/add to `.gitignore`).

### 4.2 CLI Dispatch

Add `backup` case to `smackerel.sh` command dispatcher, routing to `scripts/commands/backup.sh`.

---

## 5. Capture Resilience Design

### 5.1 DB Health Gate in CaptureHandler

**Location:** [internal/api/capture.go](internal/api/capture.go) — `CaptureHandler` method

**Change:** Insert a DB health check after request parsing, before pipeline processing:

```go
// Check DB health before processing
if !d.DB.Healthy(r.Context()) {
    writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE",
        "Database is temporarily unavailable, please retry")
    return
}
```

**Why not middleware:** The health gate is specific to write operations. Read-only endpoints (search, health, recent) should continue to serve cached or degraded responses. Making it handler-level keeps the policy explicit.

**Dependency:** Uses `db.Postgres.Healthy(ctx)` which already exists at [internal/db/postgres.go](internal/db/postgres.go#L67) — performs `pool.Ping(ctx)` with a 2s timeout.

---

## 6. Cron Concurrency Design

### 6.1 Job Group Classification

The 12 cron jobs are classified into 7 mutex groups based on execution frequency and resource contention patterns:

| Group | Mutex Name | Jobs | Schedule |
|-------|-----------|------|----------|
| `digest` | `muDigest` | Digest generation + retry | User-configured cron |
| `hourly` | `muHourly` | Topic momentum | `0 * * * *` |
| `daily` | `muDaily` | Synthesis + overdue, Resurfacing, Frequent lookups, Alert producers | `0 2 * * *`, `0 8 * * *`, `0 4 * * *`, `0 6 * * *` |
| `weekly` | `muWeekly` | Weekly synthesis, Subscription detection, Relationship cooling alerts | `0 16 * * 0`, `0 3 * * 1`, `0 7 * * 1` |
| `monthly` | `muMonthly` | Monthly report | `0 3 1 * *` |
| `briefs` | `muBriefs` | Pre-meeting briefs | `*/5 * * * *` |
| `alerts` | `muAlerts` | Alert delivery sweep | `*/15 * * * *` |

**Rationale for grouping:** Jobs within the same group share resource affinity (e.g., daily jobs all hit the intelligence engine heavily). Grouping prevents intra-group contention while allowing inter-group concurrency. The digest job gets its own mutex because it has a user-configured schedule and a unique retry mechanism.

### 6.2 Scheduler Struct Changes

Add mutex fields to the `Scheduler` struct:

```go
type Scheduler struct {
    cron      *cron.Cron
    // ... existing fields ...

    // Per-group concurrency guards
    muDigest   sync.Mutex
    muHourly   sync.Mutex
    muDaily    sync.Mutex
    muWeekly   sync.Mutex
    muMonthly  sync.Mutex
    muBriefs   sync.Mutex // pre-meeting briefs (every 5 min)
    muAlerts   sync.Mutex // alert delivery sweep (every 15 min)
}
```

### 6.3 Guard Pattern

Each cron callback wraps its body in a `TryLock` check:

```go
if !s.muDaily.TryLock() {
    slog.Warn("skipping overlapping job", "group", "daily", "job", "synthesis")
    return
}
defer s.muDaily.Unlock()
```

`sync.Mutex.TryLock()` is available since Go 1.18. It returns immediately without blocking — exactly the skip-on-overlap semantic required by the spec.

---

## 7. NATS Dead-Letter Design

### 7.1 DEADLETTER Stream

**Location:** [internal/nats/client.go](internal/nats/client.go) — `AllStreams()` and `EnsureStreams()`

Add a new stream to `AllStreams()`:

```go
{Name: "DEADLETTER", Subjects: []string{"deadletter.>"}},
```

Stream configuration:
- Retention: `LimitsPolicy` (not WorkQueue — dead-letter messages are inspected, not consumed-and-deleted)
- MaxAge: 30 days (retain for forensic inspection)
- Storage: `FileStorage`
- MaxMsgs: 10000 (prevent unbounded growth)

### 7.2 Dead-Letter Routing

NATS JetStream does not natively route exhausted messages to another stream. The routing must be implemented in the consumer's message handler.

**Pattern:** In `ResultSubscriber.handleMessage()` and `handleDigestMessage()`, when `msg.Headers().Get("Nats-Num-Delivered")` equals `MaxDeliver`, the handler:

1. Publishes the message payload to `deadletter.{original_subject}` (e.g., `deadletter.artifacts.processed`)
2. Includes metadata headers (original subject, stream, consumer, error, timestamp)
3. Acks the original message (so JetStream stops redelivering)

**Updated consumer flow:**

```
Message received → attempt processing
  ├─ Success → msg.Ack()
  ├─ Failure (delivery < MaxDeliver) → msg.Nak() → JetStream redelivers
  └─ Failure (delivery == MaxDeliver) → publish to deadletter.{subject} → msg.Ack()
```

**Note:** The delivery count check uses `msg.Headers().Get("Nats-Num-Delivered")`. When the count reaches `MaxDeliver`, the handler knows this is the final attempt.

### 7.3 Dead-Letter Inspection

Dead-letter messages are inspectable via:
- NATS CLI: `nats stream view DEADLETTER`
- Monitoring: `nats stream info DEADLETTER` shows message count
- Future: a `./smackerel.sh dlq list` command (out of scope for this feature)

---

## 8. Graceful Shutdown Design

### 8.1 Current State (Problems)

In [cmd/core/main.go](cmd/core/main.go#L300-L330):

1. `defer pg.Close()` registered at line ~92 — closes DB pool first in LIFO defer order
2. `defer nc.Close()` registered at line ~100 — NATS drain happens after DB close in defers
3. `srv.Shutdown()` uses hardcoded `10*time.Second` timeout — insufficient for Telegram's 30s long-poll
4. Post-shutdown cleanup (`tgBot.Stop()`, `resultSub.Stop()`, `supervisor.StopAll()`) happens after server shutdown but before defers run — correct ordering is accidental, not guaranteed
5. Scheduler `defer sched.Stop()` registered late but executes before HTTP server shutdown in the defer stack — potential for new cron jobs to fire during HTTP drain

### 8.2 New Shutdown Design

Replace defer-based cleanup with an explicit `shutdownAll()` function called after signal receipt.

**Shutdown sequence (explicit, sequential):**

```
Signal received (SIGINT/SIGTERM)
  │
  ├─ 1. sched.Stop()           — prevent new cron jobs from firing
  │     Wait for cron.Stop() context (blocks until running jobs complete)
  │
  ├─ 2. srv.Shutdown(ctx)      — drain in-flight HTTP requests
  │     Timeout: from SST config (default 25s)
  │
  ├─ 3. tgBot.Stop()           — cancel Telegram long-poll
  │     (returns immediately — the long-poll goroutine exits on context cancel)
  │
  ├─ 4. resultSub.Stop()       — stop NATS consumer goroutines
  │     Wait for consumer wg.Wait()
  │
  ├─ 5. supervisor.StopAll()   — stop all connectors
  │
  ├─ 6. nc.Close()             — NATS drain (flushes pending publishes)
  │     (DB pool still open — any in-flight NATS handlers that touch DB can complete)
  │
  ├─ 7. pg.Close()             — close DB connection pool
  │
  └─ 8. Log "smackerel-core stopped", exit 0
```

**Key invariant:** NATS drain (step 6) happens before DB close (step 7). This ensures any in-flight NATS message handlers that need DB access can complete.

### 8.3 Timeout Budget

Total shutdown budget: 30s (Docker stop_grace_period).

| Step | Budget | Rationale |
|------|--------|-----------|
| Scheduler stop | 2s | Running cron jobs have their own context timeouts |
| HTTP server drain | 15s | Longest expected request (digest generation) |
| Telegram stop | instant | Context cancellation |
| Result subscriber stop | 2s | Fetch timeout is 5s but we interrupt via done channel |
| Connector stop | 2s | Connectors have individual stop timeouts |
| NATS drain | 2s | Flush pending publishes |
| DB close | instant | Pool close is synchronous |
| **Total** | **~23s** | **7s margin before SIGKILL** |

The overall shutdown context uses a timeout read from SST config (`SHUTDOWN_TIMEOUT_S`, default `25`). If any individual step hangs, the context cancellation propagates, and the next step proceeds after logging a warning.

### 8.4 Implementation

Remove `defer pg.Close()`, `defer nc.Close()`, and `defer sched.Stop()` from `run()`. Instead:

```go
func shutdownAll(ctx context.Context, sched *scheduler.Scheduler, srv *http.Server,
    tgBot *telegram.Bot, resultSub *pipeline.ResultSubscriber,
    supervisor *connector.Supervisor, nc *smacknats.Client, pg *db.Postgres) {

    // Each step gets a sub-context with proportional timeout
    // If a step fails/times out, log warning and proceed
}
```

Call `shutdownAll()` in the signal handler path, replacing the current inline shutdown code.

---

## 9. Docker stop_grace_period Design

### 9.1 Change

In [docker-compose.yml](docker-compose.yml), add `stop_grace_period: 30s` to the `smackerel-core` service. This aligns the Docker SIGKILL timeout with the Go shutdown budget.

```yaml
smackerel-core:
  # ... existing config ...
  stop_grace_period: 30s
```

**Why 30s:** The spec requires the shutdown sequence to complete before Docker sends SIGKILL. The Go-side budget is 25s, leaving 5s margin. Docker's default is 10s, which is insufficient.

**Other services:** `smackerel-ml` (Python/uvicorn) and infrastructure containers (postgres, nats) use Docker defaults (10s), which is sufficient for their simpler shutdown paths.

---

## 10. ML Sidecar Health Cache Design

### 10.1 Cache Mechanism

**Location:** [internal/api/search.go](internal/api/search.go) — `SearchEngine` struct

Add two atomic fields to `SearchEngine`:

```go
mlHealthy  atomic.Bool   // last known health status
mlHealthAt atomic.Int64  // UnixNano timestamp of last check
```

Plus config fields set at construction:

```go
MLSidecarURL   string        // e.g., "http://smackerel-ml:8081"
HealthCacheTTL time.Duration // from SST config, e.g., 10s
```

### 10.2 Search Fast-Path

In `SearchEngine.Search()`, before the NATS embed request:

```go
if !s.isMLHealthy(ctx) {
    results, total, err := s.textSearch(ctx, req)
    return results, total, "text_fallback", err
}
```

`isMLHealthy()` logic:
1. Read `mlHealthAt` — if `time.Since(lastCheck) < HealthCacheTTL`, return cached `mlHealthy`
2. If stale: `GET {MLSidecarURL}/health` with 1s timeout
3. Update `mlHealthy` and `mlHealthAt`
4. Return result

**Thread safety:** `atomic.Bool` and `atomic.Int64` are lock-free. Multiple concurrent search requests may race to refresh the cache, but the worst case is a few redundant health checks — no correctness issue.

### 10.3 Startup Behavior

On startup, `mlHealthy` defaults to `false` (zero value). The first search request triggers a health check. If the sidecar is still starting (`start_period: 120s` in Compose), the first few searches use text fallback, which is the correct degraded behavior.

---

## 11. SST Pool Configuration Design

### 11.1 New Config Keys in smackerel.yaml

```yaml
infrastructure:
  postgres:
    # ... existing keys ...
    max_conns: 10
    min_conns: 2
```

And for ML health cache TTL and shutdown timeout:

```yaml
services:
  core:
    container_port: 8080
    shutdown_timeout_s: 25
  ml:
    container_port: 8081
    health_cache_ttl_s: 10
```

### 11.2 Config Generation

`scripts/commands/config.sh` adds new env var emissions:

```bash
DB_MAX_CONNS="$(required_value infrastructure.postgres.max_conns)"
DB_MIN_CONNS="$(required_value infrastructure.postgres.min_conns)"
SHUTDOWN_TIMEOUT_S="$(required_value services.core.shutdown_timeout_s)"
ML_HEALTH_CACHE_TTL_S="$(required_value services.ml.health_cache_ttl_s)"
```

These are emitted into `config/generated/{env}.env` and consumed by Go at startup.

### 11.3 Config Loading in Go

`internal/config/config.go` — add fields:

```go
type Config struct {
    // ... existing fields ...
    DBMaxConns        int32
    DBMinConns        int32
    ShutdownTimeoutS  int
    MLHealthCacheTTLS int
}
```

In `Load()`:

```go
cfg.DBMaxConns = parseRequiredInt32("DB_MAX_CONNS")
cfg.DBMinConns = parseRequiredInt32("DB_MIN_CONNS")
cfg.ShutdownTimeoutS = parseRequiredInt("SHUTDOWN_TIMEOUT_S")
cfg.MLHealthCacheTTLS = parseRequiredInt("ML_HEALTH_CACHE_TTL_S")
```

Missing values → startup failure (fail-loud, no defaults).

### 11.4 DB Connect Change

`internal/db/postgres.go` — `Connect()` signature changes:

```go
func Connect(ctx context.Context, databaseURL string, maxConns int32, minConns int32) (*Postgres, error) {
```

Replace hardcoded values:

```go
config.MaxConns = maxConns
config.MinConns = minConns
```

**Caller update:** `cmd/core/main.go` passes `cfg.DBMaxConns` and `cfg.DBMinConns` to `db.Connect()`.

### 11.5 Docker Compose Env Pass-Through

Add to `smackerel-core` environment in `docker-compose.yml`:

```yaml
DB_MAX_CONNS: ${DB_MAX_CONNS}
DB_MIN_CONNS: ${DB_MIN_CONNS}
SHUTDOWN_TIMEOUT_S: ${SHUTDOWN_TIMEOUT_S}
ML_HEALTH_CACHE_TTL_S: ${ML_HEALTH_CACHE_TTL_S}
```

---

## 12. Security & Compliance

| Concern | Mitigation |
|---------|------------|
| Backup files contain DB secrets (OAuth tokens) | `backups/` added to `.gitignore`; user responsible for offsite storage security |
| pg_dump credentials in `docker exec` args | Credentials passed via env vars inside the container, not command-line args |
| Dead-letter messages may contain user content | DEADLETTER stream uses same NATS auth token as all other streams; no new exposure surface |
| ML health check endpoint | Uses existing `/health` endpoint; no auth required (monitoring endpoint) |

---

## 13. Configuration & Migrations

### 13.1 New SST Config Keys

| YAML Path | Env Var | Default | Purpose |
|-----------|---------|---------|---------|
| `infrastructure.postgres.max_conns` | `DB_MAX_CONNS` | 10 | pgxpool MaxConns |
| `infrastructure.postgres.min_conns` | `DB_MIN_CONNS` | 2 | pgxpool MinConns |
| `services.core.shutdown_timeout_s` | `SHUTDOWN_TIMEOUT_S` | 25 | Go shutdown context timeout |
| `services.ml.health_cache_ttl_s` | `ML_HEALTH_CACHE_TTL_S` | 10 | ML sidecar health cache duration |

### 13.2 Migration Path

No database schema migrations. Config changes require:
1. Add keys to `config/smackerel.yaml`
2. Update `scripts/commands/config.sh` to emit new env vars
3. Run `./smackerel.sh config generate`
4. Restart stack: `./smackerel.sh down && ./smackerel.sh up`

---

## 14. Observability & Failure Handling

All resilience events emit structured slog output at INFO or WARN level:

| Event | Level | Structured Fields |
|-------|-------|-------------------|
| Backup success | INFO | `file`, `size_bytes` |
| Backup failure | ERROR | `error`, `step` |
| Capture DB health failure | WARN | `error`, `latency_ms` |
| Cron job skipped (overlap) | WARN | `group`, `job` |
| Dead-letter message routed | WARN | `original_subject`, `consumer`, `delivery_count`, `error` |
| Shutdown step started | INFO | `step`, `component` |
| Shutdown step timeout | WARN | `step`, `component`, `elapsed_ms` |
| ML health check result | DEBUG | `healthy`, `url`, `latency_ms` |
| ML health cache stale refresh | INFO | `healthy`, `previous`, `ttl_s` |
| Search text fallback (ML down) | INFO | `reason`, `query_length` |

---

## 15. Testing & Validation Strategy

### 15.1 Unit Tests

| Test | File | Validates |
|------|------|-----------|
| DB health check returns false when pool is closed | `internal/db/postgres_test.go` | Healthy() method |
| Cron mutex TryLock skip | `internal/scheduler/scheduler_test.go` | BS-005, BS-006: overlap prevention |
| Dead-letter routing on max delivery | `internal/pipeline/subscriber_test.go` | BS-007: exhausted message → DEADLETTER |
| Config validation fails on missing DB_MAX_CONNS | `internal/config/validate_test.go` | BS-013: fail-loud |
| ML health cache returns stale value within TTL | `internal/api/search_test.go` | Cache behavior |
| ML health cache refreshes when TTL expired | `internal/api/search_test.go` | Cache refresh |
| Shutdown sequence function calls in correct order | `cmd/core/main_test.go` | BS-008: dependency ordering |

### 15.2 Integration Tests

| Test | Validates |
|------|-----------|
| POST /api/capture with stopped postgres returns 503 | AC-003: DB_UNAVAILABLE |
| POST /api/capture with healthy postgres returns 200 | AC-004: Normal operation |
| POST /api/search with stopped ML sidecar returns text_fallback within 2s | AC-008: Search degradation |
| NATS message exhaustion lands in DEADLETTER stream | AC-006: Dead-letter routing |
| Pool size matches SST config value | AC-010: SST pool configuration |

### 15.3 E2E Tests

| Test | Validates |
|------|-----------|
| `./smackerel.sh backup` produces valid .sql.gz file | AC-001: Backup creation |
| `./smackerel.sh backup` with stopped postgres exits non-zero | AC-002: Backup failure |
| SIGTERM → clean shutdown within 30s | AC-007: Graceful shutdown |

### 15.4 Scenario-to-Test Mapping

| Scenario | Test Type | AC |
|----------|-----------|-----|
| BS-001: Backup success | E2E | AC-001 |
| BS-002: Backup failure | E2E | AC-002 |
| BS-003: Capture 503 on DB outage | Integration | AC-003 |
| BS-004: Capture 200 on healthy DB | Integration | AC-004 |
| BS-005: Overlapping cron skipped | Unit | AC-005 |
| BS-006: All 9 jobs protected | Unit | AC-005 |
| BS-007: Dead-letter routing | Integration | AC-006 |
| BS-008: Shutdown within 30s | E2E | AC-007 |
| BS-009: NATS drain before DB close | Unit | AC-007 |
| BS-010: Search text fallback | Integration | AC-008 |
| BS-011: Search semantic recovery | Integration | AC-009 |
| BS-012: Pool size from SST | Integration | AC-010 |
| BS-013: Missing pool config fails | Unit | AC-010 |

---

## 16. Alternatives Considered

### 16.1 Write-Ahead Queue for Capture During DB Outage

**Rejected.** The spec explicitly lists this as a non-goal. Fail-visible (503) is simpler, more honest, and sufficient for a single-user system where the user can retry.

### 16.2 Circuit Breaker for ML Sidecar

**Rejected.** A full circuit breaker (with half-open state, failure counting, and configurable thresholds) is over-engineered for this use case. The spec states "cached health check + fast fallback is sufficient." The atomic-based health cache achieves the same user-visible behavior with ~20 lines of code instead of a new dependency.

### 16.3 NATS Advisory-Based Dead-Letter

**Considered and rejected for now.** NATS JetStream has advisory messages for delivery exhaustion, but subscribing to advisories requires a separate consumer and complex subject matching. The simpler approach — checking delivery count in the message handler and explicitly publishing to the dead-letter stream — is more reliable and easier to test.

### 16.4 Backup via Volume Snapshot

**Rejected.** Requires stopping the container or using filesystem-level snapshots, which is complex and error-prone. `pg_dump` against a running database is the PostgreSQL-recommended approach and produces a logically consistent dump.

### 16.5 Per-Job Mutex (Instead of Per-Group)

**Considered.** Would give finer-grained control but adds 9 mutex fields instead of 6. Since jobs within the same frequency band share resource patterns (e.g., all daily jobs hit the intelligence engine), per-group is sufficient and simpler.

---

## 17. Rollout Strategy

All changes can be deployed in a single `./smackerel.sh build && ./smackerel.sh up` cycle:

1. Add new config keys to `config/smackerel.yaml`
2. Run `./smackerel.sh config generate` to emit new env vars
3. `./smackerel.sh build` to rebuild the core image with code changes
4. `./smackerel.sh down && ./smackerel.sh up` to apply Docker Compose and config changes

**Rollback:** Revert the Git commit and repeat steps 2-4. No database migrations to undo.

**User action required:** None beyond normal `./smackerel.sh` workflow. Default config values in `smackerel.yaml` preserve current behavior (pool size 10/2, 25s shutdown, 10s health cache TTL).
