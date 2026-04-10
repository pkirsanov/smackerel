# Scopes: 022 Operational Resilience

## Execution Outline

### Phase Order

1. **Scope 1 — Backup Command + DB Pool SST Config:** Add `./smackerel.sh backup` CLI command and make DB pool MaxConns/MinConns configurable via SST. These are low-risk, foundational changes that extend the config pipeline and CLI surface without touching hot paths.
2. **Scope 2 — Capture Resilience + ML Health Cache:** Add DB health gate to capture (503 on DB down) and ML sidecar health cache for fast search fallback. Both are safety-net changes on critical read/write paths.
3. **Scope 3 — Cron Concurrency Guards:** Add per-group `sync.Mutex` to the scheduler's 9 cron jobs. Isolated scheduler change with no API surface impact.
4. **Scope 4 — Graceful Shutdown + Docker stop_grace_period + NATS Dead-Letter:** Rewrite shutdown sequence, align Docker timeout, and add NATS dead-letter routing. Highest-risk scope — touches main.go lifecycle, NATS streams, and Docker Compose.

### New Types & Signatures

- `scripts/commands/backup.sh` — new CLI command (pg_dump via docker exec)
- `config.Config.DBMaxConns`, `DBMinConns`, `ShutdownTimeoutS`, `MLHealthCacheTTLS` — new SST config fields
- `db.Connect(ctx, url, maxConns, minConns)` — updated signature (was hardcoded)
- `SearchEngine.mlHealthy atomic.Bool`, `mlHealthAt atomic.Int64` — ML health cache fields
- `SearchEngine.isMLHealthy(ctx) bool` — health cache method
- `Scheduler.muDigest`, `muHourly`, `muDaily`, `muWeekly`, `muMonthly`, `muFrequent` — per-group mutexes
- NATS `DEADLETTER` stream — new JetStream stream in `AllStreams()`
- `shutdownAll(ctx, ...)` — explicit sequential shutdown function replacing defer-based cleanup

### Validation Checkpoints

- After Scope 1: `./smackerel.sh backup` produces valid dump; `./smackerel.sh test unit` passes with new config validation tests; config generate emits DB_MAX_CONNS/DB_MIN_CONNS
- After Scope 2: Integration tests confirm 503 on DB down and text_fallback search within 2s
- After Scope 3: Unit tests confirm cron overlap skip with TryLock; race detector clean
- After Scope 4: E2E shutdown test completes within 30s; DEADLETTER stream receives exhausted messages

## Scope Summary

| # | Name | Surfaces | Key Tests | DoD Summary | Status |
|---|------|----------|-----------|-------------|--------|
| 1 | Backup Command + DB Pool SST | CLI, config, db | E2E backup, unit config validation | Backup works, pool SST-driven | [x] Done |
| 2 | Capture Resilience + ML Health Cache | API (capture, search), config | Integration 503, integration text_fallback | Capture fails visible, search degrades fast | [x] Done |
| 3 | Cron Concurrency Guards | Scheduler | Unit mutex TryLock, race detector | 9 jobs overlap-protected | [x] Done |
| 4 | Graceful Shutdown + Docker + Dead-Letter | main.go, NATS, docker-compose | E2E shutdown timing, integration dead-letter | Clean shutdown in order, no message loss | [x] Done |

---

## Scope 1: Backup Command + DB Pool SST Config

**Status:** [x] Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-022-01 Successful database backup
  Given the Smackerel stack is running with PostgreSQL healthy
  When the user runs ./smackerel.sh backup
  Then a compressed pg_dump file is created in backups/ with a timestamped name
  And the CLI prints the file path and size
  And the exit code is 0

Scenario: SCN-022-02 Backup fails when database is unreachable
  Given the Smackerel stack is running but PostgreSQL is unreachable
  When the user runs ./smackerel.sh backup
  Then the CLI exits with a non-zero exit code
  And the error message indicates the database is unreachable

Scenario: SCN-022-03 DB pool size flows from SST config
  Given the user sets infrastructure.postgres.max_conns: 20 in config/smackerel.yaml
  When ./smackerel.sh config generate runs and the core service starts
  Then the database connection pool is configured with MaxConns=20
  And no hardcoded pool size value is used

Scenario: SCN-022-04 Missing DB pool config fails loudly
  Given DB_MAX_CONNS or DB_MIN_CONNS is not set in the environment
  When the core service starts
  Then startup fails with an explicit error naming the missing variable
```

### Implementation Plan

**Files touched:**
- `scripts/commands/backup.sh` (new) — pg_dump via `docker exec`, gzip to `backups/`
- `smackerel.sh` — add `backup` case to command dispatcher
- `config/smackerel.yaml` — add `infrastructure.postgres.max_conns`, `min_conns`, `services.core.shutdown_timeout_s`, `services.ml.health_cache_ttl_s`
- `scripts/commands/config.sh` — emit `DB_MAX_CONNS`, `DB_MIN_CONNS`, `SHUTDOWN_TIMEOUT_S`, `ML_HEALTH_CACHE_TTL_S`
- `internal/config/config.go` — add `DBMaxConns`, `DBMinConns`, `ShutdownTimeoutS`, `MLHealthCacheTTLS` fields with fail-loud validation
- `internal/db/postgres.go` — `Connect()` accepts `maxConns`, `minConns` params instead of hardcoded values
- `cmd/core/main.go` — pass `cfg.DBMaxConns`, `cfg.DBMinConns` to `db.Connect()`
- `.gitignore` — ensure `backups/` is gitignored
- `docker-compose.yml` — add `DB_MAX_CONNS`, `DB_MIN_CONNS`, `SHUTDOWN_TIMEOUT_S`, `ML_HEALTH_CACHE_TTL_S` env pass-through

**SST flow:** `smackerel.yaml` → `config.sh` generates env → `config.go` reads env with fail-loud → `db.Connect()` uses configured values.

**Security:** Backup files contain full DB content (including tokens). `backups/` must be gitignored. Credentials passed via container env vars, not CLI args.

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| E2E | `./smackerel.sh backup` produces valid .sql.gz file | Backup creation + validation | SCN-022-01 |
| E2E | `./smackerel.sh backup` with stopped postgres exits non-zero | Backup failure detection | SCN-022-02 |
| Unit | `config.Load()` parses `DB_MAX_CONNS`/`DB_MIN_CONNS` from env | SST config loading | SCN-022-03 |
| Unit | `config.Load()` fails with missing `DB_MAX_CONNS` | Fail-loud validation | SCN-022-04 |
| Unit | `db.Connect()` uses provided maxConns/minConns | Pool sizing from params | SCN-022-03 |
| Integration | Pool size matches SST config value after startup | End-to-end SST flow | SCN-022-03 |
| E2E (regression) | `./smackerel.sh backup` produces restorable dump | Regression: backup integrity | SCN-022-01 |

### Definition of Done

- [x] `./smackerel.sh backup` produces a valid, non-empty `.sql.gz` file in `backups/`
- [x] Backup with stopped postgres exits non-zero with clear error
- [x] `backups/` is in `.gitignore`
- [x] `DB_MAX_CONNS` and `DB_MIN_CONNS` flow from `smackerel.yaml` → config generate → env → `config.Config` → `db.Connect()`
- [x] Missing `DB_MAX_CONNS` or `DB_MIN_CONNS` causes startup failure (no hardcoded fallback)
- [x] `SHUTDOWN_TIMEOUT_S` and `ML_HEALTH_CACHE_TTL_S` added to SST pipeline (used in later scopes)
- [x] All unit tests pass: `./smackerel.sh test unit`
- [x] E2E backup tests pass: `./smackerel.sh test e2e`
- [x] Zero hardcoded pool sizes remain in `internal/db/postgres.go`
- [x] Config generation produces all new env vars: `./smackerel.sh config generate`

---

## Scope 2: Capture Resilience + ML Health Cache for Search

**Status:** [x] Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-022-05 Capture returns 503 when database is unreachable
  Given PostgreSQL is temporarily unreachable
  When a user submits a capture request via POST /api/capture
  Then the API returns HTTP 503 with error code DB_UNAVAILABLE
  And no artifact data is silently dropped

Scenario: SCN-022-06 Capture succeeds during normal operation
  Given PostgreSQL is healthy
  When a user submits a valid capture request
  Then the artifact is persisted and an HTTP 200 response is returned

Scenario: SCN-022-07 Search degrades to text fallback when ML sidecar is down
  Given the ML sidecar is unreachable
  When a user performs a search via POST /api/search
  Then the search handler skips the NATS embed request
  And returns text-fallback results within 2 seconds
  And the response includes search_mode: "text_fallback"

Scenario: SCN-022-08 Search resumes semantic path when ML sidecar recovers
  Given the ML sidecar was down but has recovered
  When the cached health check detects the sidecar is healthy again
  Then subsequent searches use the full semantic (embedding + vector) path
```

### Implementation Plan

**Files touched:**
- `internal/api/capture.go` — add `db.Healthy(ctx)` check at top of `CaptureHandler`, return 503 with `DB_UNAVAILABLE` if unhealthy
- `internal/api/search.go` — add `mlHealthy atomic.Bool`, `mlHealthAt atomic.Int64`, `MLSidecarURL`, `HealthCacheTTL` to `SearchEngine`; add `isMLHealthy(ctx)` method; check health before NATS embed request and fall back to `textSearch()` if unhealthy
- `cmd/core/main.go` — wire `MLSidecarURL` and `HealthCacheTTL` from config into `SearchEngine`

**API contract:** POST /api/capture adds 503 response for `DB_UNAVAILABLE`. POST /api/search response shape unchanged — `search_mode: "text_fallback"` already exists; this makes it instantaneous instead of waiting for NATS timeout.

**Health cache design:** `atomic.Bool` + `atomic.Int64` (lock-free). Lazy refresh on search requests — no background goroutine. First search after startup triggers a health check; if sidecar is still starting, text fallback is used (correct degraded behavior).

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Integration | POST /api/capture with stopped postgres returns 503 | DB health gate | SCN-022-05 |
| Integration | POST /api/capture with healthy postgres returns 200 | Normal capture path | SCN-022-06 |
| Unit | CaptureHandler returns 503 when DB.Healthy() returns false | Handler-level gate test | SCN-022-05 |
| Integration | POST /api/search with stopped ML sidecar returns text_fallback within 2s | Search degradation | SCN-022-07 |
| Unit | `isMLHealthy()` returns cached value within TTL | Cache TTL behavior | SCN-022-07 |
| Unit | `isMLHealthy()` refreshes when TTL expired | Cache refresh behavior | SCN-022-08 |
| Unit | `isMLHealthy()` returns true after sidecar recovery | Recovery detection | SCN-022-08 |
| E2E (regression) | Capture with healthy DB returns 200 | Regression: normal capture unbroken | SCN-022-06 |
| E2E (regression) | Search returns results when ML sidecar is healthy | Regression: semantic search unbroken | SCN-022-08 |

### Definition of Done

- [x] POST /api/capture returns 503 with `DB_UNAVAILABLE` when PostgreSQL is unreachable
- [x] POST /api/capture returns 200 and persists artifact when PostgreSQL is healthy
- [x] No artifact data is silently dropped under any DB failure condition
- [x] Search returns text-fallback results within 2s when ML sidecar is down
- [x] Search resumes semantic path when ML sidecar recovers (health cache TTL refresh)
- [x] Health cache TTL is configurable via SST (`ML_HEALTH_CACHE_TTL_S`)
- [x] All unit tests pass: `./smackerel.sh test unit`
- [x] Integration tests pass: `./smackerel.sh test integration`
- [x] Race detector clean: `go test -race` on `internal/api/...`
- [x] Zero silent data loss: every capture either succeeds or returns explicit error

---

## Scope 3: Cron Concurrency Guards

**Status:** [x] Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-022-09 Overlapping cron job of same type is skipped
  Given the daily synthesis job is currently running
  When a second daily synthesis job fires
  Then the second invocation is skipped with a warning log
  And the first job continues uninterrupted

Scenario: SCN-022-10 Different job groups run concurrently
  Given the daily synthesis job (5-minute timeout) is currently running
  When the hourly topic momentum job fires
  Then the momentum job acquires its own mutex and runs concurrently with synthesis

Scenario: SCN-022-11 All nine cron jobs are protected from self-overlap
  Given 9 cron jobs are registered (digest, momentum, synthesis, resurfacing, pre-meeting briefs, weekly synthesis, monthly report, subscription detection, frequent lookups)
  When any job fires while a previous instance of the same type is still running
  Then the new invocation is skipped and a warning is logged
```

### Implementation Plan

**Files touched:**
- `internal/scheduler/scheduler.go` — add 6 `sync.Mutex` fields to `Scheduler` struct: `muDigest`, `muHourly`, `muDaily`, `muWeekly`, `muMonthly`, `muFrequent`; wrap each cron callback in `TryLock`/`Unlock` guard

**Job group classification:**

| Group | Mutex | Jobs |
|-------|-------|------|
| digest | `muDigest` | Digest generation + retry |
| hourly | `muHourly` | Topic momentum |
| daily | `muDaily` | Synthesis, resurfacing, frequent lookups |
| weekly | `muWeekly` | Weekly synthesis, subscription detection |
| monthly | `muMonthly` | Monthly report |
| frequent | `muFrequent` | Pre-meeting briefs |

**Guard pattern:** `sync.Mutex.TryLock()` (Go 1.18+) — returns immediately without blocking. If lock held, skip invocation with `slog.Warn("skipping overlapping job", "group", group, "job", jobName)`.

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Unit | TryLock returns false when mutex held → job skipped | Overlap prevention | SCN-022-09 |
| Unit | Different group mutexes are independent | Cross-group concurrency | SCN-022-10 |
| Unit | All 6 mutex groups are wired to correct jobs | Complete coverage | SCN-022-11 |
| Unit (race) | Concurrent cron fire simulation with race detector | Concurrency safety | SCN-022-09, SCN-022-10 |
| E2E (regression) | Cron jobs still execute normally (no deadlock) | Regression: cron functionality | SCN-022-11 |

### Definition of Done

- [x] 6 per-group `sync.Mutex` fields added to `Scheduler` struct
- [x] All 9 cron job callbacks wrapped in `TryLock`/`Unlock` guards
- [x] Overlapping same-group job invocations are skipped with warning log
- [x] Different-group jobs run concurrently without interference
- [x] Race detector clean: `go test -race ./internal/scheduler/...`
- [x] All unit tests pass: `./smackerel.sh test unit`
- [x] No hardcoded sleep or wait in concurrency tests — use channels/sync for determinism

---

## Scope 4: Graceful Shutdown + Docker stop_grace_period + NATS Dead-Letter

**Status:** [x] Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-022-12 Graceful shutdown completes within Docker timeout
  Given the core service is running with active connections
  When Docker sends SIGTERM (e.g., ./smackerel.sh down)
  Then the shutdown sequence completes within 30 seconds
  And all subsystems are stopped in reverse-dependency order
  And the process exits cleanly before Docker sends SIGKILL

Scenario: SCN-022-13 Shutdown order prevents NATS drain racing DB close
  Given the core service is shutting down
  When the shutdown sequence reaches NATS drain
  Then the DB pool is still open (NATS drain happens before DB close)
  And any in-flight NATS handlers that touch the DB can complete

Scenario: SCN-022-14 NATS message exhaustion routes to dead-letter
  Given a NATS consumer has a message that has failed MaxDeliver (5) times
  When the consumer gives up on the message
  Then the message payload and metadata are routed to the DEADLETTER stream
  And the message is inspectable via NATS CLI or monitoring
```

### Implementation Plan

**Files touched:**
- `cmd/core/main.go` — remove `defer pg.Close()`, `defer nc.Close()`, `defer sched.Stop()`; add `shutdownAll()` function with explicit sequential ordering: sched → HTTP → Telegram → resultSub → connectors → NATS → DB; call from signal handler
- `docker-compose.yml` — add `stop_grace_period: 30s` to `smackerel-core` service
- `internal/nats/client.go` — add `DEADLETTER` stream to `AllStreams()`; stream config: `LimitsPolicy`, `MaxAge: 30d`, `FileStorage`, `MaxMsgs: 10000`
- `internal/pipeline/subscriber.go` (or equivalent result subscriber) — in message handler, when delivery count == `MaxDeliver`, publish to `deadletter.{original_subject}` with metadata headers, then `msg.Ack()`

**Shutdown timeout budget (25s Go-side, 30s Docker-side):**

| Step | Budget | Component |
|------|--------|-----------|
| Scheduler stop | 2s | `sched.Stop()` |
| HTTP drain | 15s | `srv.Shutdown(ctx)` |
| Telegram stop | instant | Context cancel |
| Result subscriber | 2s | Consumer drain |
| Connectors | 2s | `supervisor.StopAll()` |
| NATS drain | 2s | `nc.Close()` |
| DB close | instant | `pg.Close()` |
| **Total** | **~23s** | **7s margin** |

**Dead-letter message headers:** `Smackerel-Original-Subject`, `Smackerel-Original-Stream`, `Smackerel-Failed-At`, `Smackerel-Delivery-Count`, `Smackerel-Last-Error`

**Consumer Impact Sweep:** The shutdown rewrite in `main.go` touches the lifecycle of all subsystems. Verify that no other call site depends on `defer`-based cleanup ordering.

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| E2E | SIGTERM → clean shutdown within 30s | Shutdown timing | SCN-022-12 |
| Unit | `shutdownAll()` calls components in correct order | Dependency ordering | SCN-022-13 |
| Unit | Shutdown step timeout → log warning, proceed | Timeout resilience | SCN-022-12 |
| Integration | NATS message with delivery count == MaxDeliver → DEADLETTER stream | Dead-letter routing | SCN-022-14 |
| Unit | Dead-letter message has correct headers | Metadata preservation | SCN-022-14 |
| Integration | DEADLETTER stream created by `EnsureStreams()` at startup | Stream provisioning | SCN-022-14 |
| E2E (regression) | Normal NATS message processing unaffected by dead-letter logic | Regression: no message path disruption | SCN-022-14 |
| E2E (regression) | `./smackerel.sh down` completes cleanly | Regression: stack lifecycle | SCN-022-12 |

### Definition of Done

- [x] `shutdownAll()` replaces defer-based cleanup in `main.go`
- [x] Shutdown order: scheduler → HTTP → Telegram → subscribers → connectors → NATS → DB
- [x] Each shutdown step has a sub-context timeout; timeout logs warning and proceeds
- [x] `docker-compose.yml` has `stop_grace_period: 30s` on `smackerel-core`
- [x] `DEADLETTER` stream created by `EnsureStreams()` with `LimitsPolicy`, 30d MaxAge, 10000 MaxMsgs
- [x] Exhausted NATS messages route to `deadletter.{subject}` with metadata headers
- [x] Dead-letter messages preserve original payload + failure metadata
- [x] E2E: SIGTERM → clean exit within 30s
- [x] All unit tests pass: `./smackerel.sh test unit`
- [x] Integration tests pass: `./smackerel.sh test integration`
- [x] E2E tests pass: `./smackerel.sh test e2e`
- [x] No orphan goroutines after shutdown (verified via test or log)
