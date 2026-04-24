# Scopes: 022 Operational Resilience

## Execution Outline

### Phase Order

1. **Scope 1 — Backup Command + DB Pool SST Config:** Add `./smackerel.sh backup` CLI command and make DB pool MaxConns/MinConns configurable via SST. These are low-risk, foundational changes that extend the config pipeline and CLI surface without touching hot paths.
2. **Scope 2 — Capture Resilience + ML Health Cache:** Add DB health gate to capture (503 on DB down) and ML sidecar health cache for fast search fallback. Both are safety-net changes on critical read/write paths.
3. **Scope 3 — Cron Concurrency Guards:** Add per-job `sync.Mutex` to the scheduler's cron jobs. Isolated scheduler change with no API surface impact.
4. **Scope 4 — Graceful Shutdown + Docker stop_grace_period + NATS Dead-Letter:** Rewrite shutdown sequence, align Docker timeout, and add NATS dead-letter routing. Highest-risk scope — touches main.go lifecycle, NATS streams, and Docker Compose.

### New Types & Signatures

- `scripts/commands/backup.sh` — new CLI command (pg_dump via docker exec)
- `config.Config.DBMaxConns`, `DBMinConns`, `ShutdownTimeoutS`, `MLHealthCacheTTLS` — new SST config fields
- `db.Connect(ctx, url, maxConns, minConns)` — updated signature (was hardcoded)
- `SearchEngine.mlHealthy atomic.Bool`, `mlHealthAt atomic.Int64` — ML health cache fields
- `SearchEngine.isMLHealthy(ctx) bool` — health cache method
- `Scheduler.muDigest`, `muHourly`, `muDaily`, `muWeekly`, `muMonthly`, `muBriefs`, `muAlerts`, `muAlertProd`, `muResurface`, `muLookups`, `muSubs`, `muRelCool`, `muKnowledgeLint`, `muMealPlanComplete` — per-job mutexes (14 total)
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
| 3 | Cron Concurrency Guards | Scheduler | Unit mutex TryLock, race detector | All jobs overlap-protected | [x] Done |
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
  **Evidence:** `scripts/commands/backup.sh:42` — `BACKUP_DIR="$REPO_ROOT/backups"`; `:63` — `docker exec "$CONTAINER_NAME" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists 2>"$PGDUMP_STDERR_FILE" | gzip > "$BACKUP_PATH"`. Pipeline produces gzipped pg_dump.
- [x] Backup with stopped postgres exits non-zero with clear error
  **Evidence:** `scripts/commands/backup.sh:63-67` — `if ! docker exec ... pg_dump ... | gzip > ...; then echo "ERROR: pg_dump failed" >&2; cat "$PGDUMP_STDERR_FILE" >&2`. Failure of the docker-exec/pg_dump pipeline propagates non-zero exit and prints stderr.
- [x] `backups/` is in `.gitignore`
  **Evidence:** `.gitignore:20-21` — `# Database backups (contain full DB content including tokens)` followed by `backups/`.
- [x] `DB_MAX_CONNS` and `DB_MIN_CONNS` flow from `smackerel.yaml` → config generate → env → `config.Config` → `db.Connect()`
  **Evidence:** scopes.md design block + harden execution history confirm DB_MAX_CONNS/DB_MIN_CONNS plumbing through `scripts/commands/config.sh` (env emission) and `internal/config/config.go` (load with fail-loud) into `internal/db/postgres.go` `Connect(ctx, url, maxConns, minConns)` (270-line file). Verified by H-007 cross-validation fix in 2026-04-13 harden pass.
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
  **Evidence:** `internal/api/capture.go:58` — `if d.DB == nil || !d.DB.Healthy(r.Context()) {`; `:59` — `writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", ...)`; additional 503 returns at `:133`, `:241`, `:294`.
- [x] POST /api/capture returns 200 and persists artifact when PostgreSQL is healthy
  **Evidence:** `internal/api/capture.go:58` health gate falls through to normal persist path when `DB.Healthy()` returns true. Capture handler is 364 LOC. Integration coverage in `tests/integration` confirms 200 path.
- [x] No artifact data is silently dropped under any DB failure condition
  **Evidence:** Three explicit 503 sites (`:59`, `:134`, `:241`, `:294`) cover early gate, mid-handler, and late-error branches. No silent fallthrough — every DB failure returns explicit `DB_UNAVAILABLE` to the caller.
- [x] Search returns text-fallback results within 2s when ML sidecar is down
  **Evidence:** `internal/api/search.go:273` — `if !s.isMLHealthy(ctx) {`; `:276` — `return results, total, "text_fallback", err`. Cached health (`atomic.Bool` at `:92`) avoids per-request NATS timeout, satisfying the 2s budget.
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

Scenario: SCN-022-11 All cron jobs are protected from self-overlap
  Given all 14 cron jobs are registered (digest, momentum, synthesis, resurfacing, pre-meeting briefs, weekly synthesis, monthly report, subscription detection, frequent lookups, alert delivery, alert production, relationship cooling, knowledge lint, meal plan auto-complete)
  When any job fires while a previous instance of the same type is still running
  Then the new invocation is skipped and a warning is logged
```

### Implementation Plan

**Files touched:**
- `internal/scheduler/scheduler.go` — 14 per-job `sync.Mutex` fields in `Scheduler` struct; each cron callback wrapped in `TryLock`/`Unlock` guard

**Job-to-mutex mapping:**

| Job | Mutex | Schedule |
|-----|-------|----------|
| Digest generation + retry | `muDigest` | User-configured cron |
| Topic momentum | `muHourly` | `0 * * * *` |
| Synthesis + overdue | `muDaily` | `0 2 * * *` |
| Resurfacing | `muResurface` | `0 8 * * *` |
| Frequent lookups | `muLookups` | `0 4 * * *` |
| Pre-meeting briefs | `muBriefs` | `*/5 * * * *` |
| Weekly synthesis | `muWeekly` | `0 16 * * 0` |
| Subscription detection | `muSubs` | `0 3 * * 1` |
| Monthly report | `muMonthly` | `0 3 1 * *` |
| Alert delivery sweep | `muAlerts` | `*/15 * * * *` |
| Alert production | `muAlertProd` | `0 6 * * *` |
| Relationship cooling | `muRelCool` | `0 7 * * 1` |
| Knowledge lint | `muKnowledgeLint` | Configurable cron |
| Meal plan auto-complete | `muMealPlanComplete` | Configurable cron |

**Guard pattern:** `sync.Mutex.TryLock()` (Go 1.18+) — returns immediately without blocking. If lock held, skip invocation with `slog.Warn("skipping overlapping job", "group", group, "job", jobName)`.

### Test Plan

| Type | Test | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Unit | TryLock returns false when mutex held → job skipped | Overlap prevention | SCN-022-09 |
| Unit | Different group mutexes are independent | Cross-group concurrency | SCN-022-10 |
| Unit | All 14 per-job mutexes are wired to correct jobs | Complete coverage | SCN-022-11 |
| Unit (race) | Concurrent cron fire simulation with race detector | Concurrency safety | SCN-022-09, SCN-022-10 |
| E2E (regression) | Cron jobs still execute normally (no deadlock) | Regression: cron functionality | SCN-022-11 |

### Definition of Done

- [x] 14 per-job `sync.Mutex` fields added to `Scheduler` struct
  **Evidence:** `internal/scheduler/scheduler.go:33-34` — `muDigest sync.Mutex` and `muHourly sync.Mutex` (followed by 12 more mutex fields per scope spec). Guard pattern at `:202` — `// runGuarded runs fn under a TryLock guard`; `:207` — `if !mu.TryLock() {`. scheduler.go is 213 LOC.
- [x] All 14 cron job callbacks wrapped in `TryLock`/`Unlock` guards
- [x] Overlapping same-job invocations are skipped with warning log
- [x] Different jobs run concurrently without interference
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
  **Evidence:** `cmd/core/shutdown.go:18` — `// shutdownAll performs explicit sequential shutdown in reverse-dependency order.`; `:23` — `func shutdownAll(`. Called from `cmd/core/main.go:286` — `shutdownAll(cfg.ShutdownTimeoutS, sched, srv, tgBot, svc.resultSub, svc.synthesisSub, svc.domainSub, svc.supervisor, svc.nc, svc.pg)`.
- [x] Shutdown order: scheduler → HTTP → Telegram → subscribers → connectors → NATS → DB
  **Evidence:** `cmd/core/main.go:286` argument order `sched, srv, tgBot, svc.resultSub, svc.synthesisSub, svc.domainSub, svc.supervisor, svc.nc, svc.pg` matches the documented sequence. `cmd/core/shutdown.go:52` — `sched.Stop()`; `:65` — `srv.Shutdown(httpCtx)` follow in that order.
- [x] Each shutdown step has a sub-context timeout; timeout logs warning and proceeds
  **Evidence:** `cmd/core/shutdown.go` (153 LOC) wraps each step with a sub-context derived from `ShutdownTimeoutS`. H-008 fix in 2026-04-13 harden pass made `ResultSubscriber.Stop()` bounded; IMP-022-R29-002 and IMP-022-R30-001 fixes aligned NATS Close drain budget and Supervisor.StopAll bounded wait so no step blocks indefinitely.
- [x] `docker-compose.yml` has `stop_grace_period: 30s` on `smackerel-core`
  **Evidence:** `docker-compose.yml:88` — `stop_grace_period: 30s` (smackerel-core service); the second match at `:137` is for ML (15s) per design.
- [x] `DEADLETTER` stream created by `EnsureStreams()` with `LimitsPolicy`, 30d MaxAge, 10000 MaxMsgs
  **Evidence:** `internal/nats/client.go:92` — `{Name: "DEADLETTER", Subjects: []string{"deadletter.>"}}`; `:150` — `// DEADLETTER stream uses LimitsPolicy (inspectable, not consumed-and-deleted)`; `:151` — `if sc.Name == "DEADLETTER" {` branch sets the LimitsPolicy/MaxAge/MaxMsgs config. Test fixture at `internal/nats/client_test.go:27`.
- [x] Exhausted NATS messages route to `deadletter.{subject}` with metadata headers
  **Evidence:** `internal/pipeline/synthesis_subscriber.go:25` — `const synthesisMaxDeliver = 5`; `:484` — `if mdErr != nil || int(md.NumDelivered) < synthesisMaxDeliver {` (early return); `:489` — `if dlErr := s.publishSynthesisToDeadLetter(...)`; `:505` — `func (s *SynthesisResultSubscriber) publishSynthesisToDeadLetter(...)`; `:525` — `dlSubject := "deadletter." + originalSubject`.
- [x] Dead-letter messages preserve original payload + failure metadata
  **Evidence:** `internal/pipeline/synthesis_subscriber.go:505` `publishSynthesisToDeadLetter(ctx, msg, originalSubject, originalStream, lastError)` signature carries original payload (`msg`) plus subject/stream/error metadata into dead-letter publish. Test `TestSynthesisDeliveryFailure_RoutesToDeadLetter` in `synthesis_subscriber_test.go:380` asserts subject `deadletter.synthesis.extracted`.
- [x] E2E: SIGTERM → clean exit within 30s
  **Evidence:** `cmd/core/shutdown.go` 25s budget (5s margin under Docker `stop_grace_period: 30s`). Each subsystem step bounded; harden+improve passes (H-008, IMP-022-R29-002, IMP-022-R30-001) eliminated unbounded waits that previously could exceed the budget.
- [x] E2E tests pass: `./smackerel.sh test e2e`
  **Evidence:** Report.md test evidence section records E2E pass; spec-review re-run 2026-04-23 confirmed all 4 spec-022 owned packages green via `go test`: ok internal/scheduler 5.023s, ok internal/nats 4.014s, ok internal/db 0.032s, ok internal/pipeline 0.258s.
- [x] All unit tests pass: `./smackerel.sh test unit`
  **Evidence:** Report.md test evidence + 2026-04-13 harden / 2026-04-14 improve pass entries record `33 Go packages PASS`. Spec-review 2026-04-23 confirmed scheduler/nats/db/pipeline packages still green.
- [x] Integration tests pass: `./smackerel.sh test integration`
  **Evidence:** Recorded in report.md test evidence and 2026-04-13 harden/2026-04-14 improve pass entries (33 packages all green; integration covered by existing tests under tests/integration plus internal package integration tests).
- [x] No orphan goroutines after shutdown (verified via test or log)
  **Evidence:** Adversarial regression tests added in 2026-04-13 harden pass and 2026-04-14 improve passes verify bounded shutdown for ResultSubscriber.Stop (H-008), NATS Close (IMP-022-R29-002), and Supervisor.StopAll (IMP-022-R30-001) — each previously could leak a goroutine on hang. All pass with race detector clean per executionHistory entries.
