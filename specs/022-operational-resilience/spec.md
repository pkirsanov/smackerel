# Feature: 022 Operational Resilience

## Problem Statement

Smackerel stores a user's entire personal knowledge base in a single PostgreSQL volume with no backup mechanism — a volume loss event means total, irrecoverable data loss. Beyond storage, the runtime has multiple resilience gaps: capture requests silently vanish during DB outages, 9 scheduler cron jobs run without concurrency guards and compete for a 10-connection DB pool, NATS messages are permanently lost after 5 delivery attempts with no dead-letter routing, the shutdown sequence races (NATS drain vs DB pool close), the HTTP shutdown timeout (10s) is too short for Telegram's 30s long-poll cycle, and search blocks indefinitely on an unresponsive ML sidecar. These gaps were identified during a stability review (findings S-015, S-011, S-012, S-013, S-024, S-025, ENG-005, ENG-008) and represent immediate operational risks for a single-user, self-hosted system where data loss is permanent.

## Outcome Contract

**Intent:** Protect the user's knowledge base from data loss, prevent silent failures during component outages, and ensure the runtime degrades gracefully under partial-failure conditions rather than losing data or hanging indefinitely.

**Success Signal:** (1) `./smackerel.sh backup` produces a valid pg_dump file that can be restored; (2) POST /api/capture returns HTTP 503 when PostgreSQL is unreachable; (3) overlapping cron jobs are serialized per-type; (4) exhausted NATS messages land in a dead-letter stream; (5) shutdown completes cleanly in dependency order within 30s; (6) search returns text-fallback results within 2s when ML sidecar is down; (7) DB pool size is driven from SST config.

**Hard Constraints:**
- Zero silent data loss: every capture request must either succeed or return an explicit error to the caller
- Backup must use pg_dump against the running database — no volume-snapshot or container-stop required
- All new configuration values must flow from `config/smackerel.yaml` (SST) — zero hardcoded defaults
- Shutdown must complete within Docker's stop_grace_period to avoid SIGKILL
- Dead-letter stream must preserve message payload and failure metadata for manual inspection

**Failure Condition:** A capture request returns HTTP 200 but the artifact is not persisted, or a backup command silently produces an empty/corrupt dump, or shutdown leaves orphan goroutines that corrupt state.

## Goals

- G1: Provide a CLI backup command that produces a timestamped pg_dump file the user can store offsite
- G2: Make capture fail-visible when the database is unreachable (HTTP 503, not silent drop)
- G3: Prevent scheduler cron job overlap via per-type mutual exclusion
- G4: Route exhausted NATS messages to a dead-letter stream instead of discarding them
- G5: Implement explicit sequential shutdown ordering to prevent resource races
- G6: Align Docker stop_grace_period with actual shutdown budget (30s)
- G7: Add ML sidecar health caching so search degrades to text-fallback fast instead of blocking on NATS
- G8: Make DB pool MaxConns/MinConns configurable via SST config

## Non-Goals

- Automated offsite backup scheduling or cloud backup integration (user runs `./smackerel.sh backup` manually or via their own cron)
- Write-ahead queue for capture during DB outage (fail-visible is the requirement; buffering is future scope)
- HA/multi-node PostgreSQL replication
- Automatic dead-letter replay (manual inspection only in this phase)
- ML sidecar circuit breaker with half-open state (cached health check + fast fallback is sufficient)
- Hot-reload of DB pool configuration (requires restart)

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| User (Self-Hoster) | Single user running Smackerel on personal infrastructure | Protect knowledge base from loss, receive clear error signals | Full system access |
| Smackerel Core | Go runtime service | Process captures reliably, degrade gracefully, shut down cleanly | Internal system actor |
| ML Sidecar | Python FastAPI service | Process embeddings, digests, OCR | Internal system actor |
| Scheduler | Internal cron subsystem | Run intelligence jobs without overlap or pool exhaustion | Internal system actor |
| NATS JetStream | Message bus | Deliver messages reliably, route failures to dead-letter | Infrastructure actor |
| PostgreSQL | Database | Store artifacts, respond to health checks | Infrastructure actor |

## Use Cases

### UC-001: Backup Knowledge Base
- **Actor:** User (Self-Hoster)
- **Preconditions:** Smackerel stack is running with PostgreSQL healthy
- **Main Flow:**
  1. User runs `./smackerel.sh backup`
  2. CLI resolves PostgreSQL connection details from SST-generated config
  3. CLI executes `pg_dump` against the running database
  4. CLI writes compressed dump to `backups/smackerel-YYYY-MM-DD-HHMMSS.sql.gz`
  5. CLI reports file path and size on success
- **Alternative Flows:**
  - AF-1: PostgreSQL unreachable → CLI exits with non-zero code and error message
  - AF-2: `backups/` directory does not exist → CLI creates it
  - AF-3: pg_dump not available in PATH → CLI exits with clear error
- **Postconditions:** A valid, restorable pg_dump file exists in `backups/`

### UC-002: Capture With DB Outage Detection
- **Actor:** User via API or Telegram
- **Preconditions:** User submits POST /api/capture
- **Main Flow:**
  1. Capture handler receives valid request
  2. Handler checks DB health (pool connectivity)
  3. DB is healthy → process capture normally
- **Alternative Flows:**
  - AF-1: DB is unreachable → return HTTP 503 with error code `DB_UNAVAILABLE` and message "Database is temporarily unavailable, please retry"
- **Postconditions:** Capture either succeeds with artifact persisted, or caller receives explicit 503

### UC-003: Cron Job Overlap Prevention
- **Actor:** Scheduler
- **Preconditions:** A cron job fires for a job type (e.g., synthesis, momentum)
- **Main Flow:**
  1. Scheduler attempts to acquire per-type mutex
  2. Mutex acquired → execute job
  3. Job completes → release mutex
- **Alternative Flows:**
  - AF-1: Mutex already held (previous run still executing) → skip this invocation with a warning log
- **Postconditions:** At most one instance of each job type runs concurrently

### UC-004: NATS Dead-Letter Routing
- **Actor:** NATS JetStream
- **Preconditions:** A message has exceeded MaxDeliver attempts (currently 5)
- **Main Flow:**
  1. Consumer exhausts MaxDeliver for a message
  2. Message is routed to `DEADLETTER` stream
  3. Dead-letter message preserves original subject, payload, and delivery metadata
- **Alternative Flows:**
  - AF-1: Dead-letter stream does not exist → EnsureStreams creates it at startup
- **Postconditions:** No message is permanently lost; exhausted messages are inspectable in the dead-letter stream

### UC-005: Sequential Graceful Shutdown
- **Actor:** Smackerel Core (on SIGINT/SIGTERM)
- **Preconditions:** Core is running with all subsystems active
- **Main Flow:**
  1. Signal received → cancel context
  2. Stop scheduler (no new cron jobs fire)
  3. Shut down HTTP server (drain in-flight requests)
  4. Stop Telegram bot (cancel long-poll)
  5. Stop result subscribers (NATS consumer drain)
  6. Stop connector supervisor (all connectors)
  7. Drain NATS connection
  8. Close DB pool
  9. Log "smackerel-core stopped" and exit 0
- **Alternative Flows:**
  - AF-1: Any step times out → log warning, proceed to next step
  - AF-2: Docker sends SIGKILL after stop_grace_period → process terminates (data already flushed by earlier steps)
- **Postconditions:** All resources released in dependency order, no orphan goroutines

### UC-006: Search With ML Sidecar Down
- **Actor:** User via API
- **Preconditions:** User submits POST /api/search, ML sidecar is unreachable
- **Main Flow:**
  1. Search handler checks cached ML sidecar health status
  2. Sidecar marked unhealthy → skip NATS embed request entirely
  3. Fall back to text-based search immediately
  4. Return results with `search_mode: "text_fallback"`
- **Alternative Flows:**
  - AF-1: Cache is stale (older than TTL) → perform one health check, update cache, then decide
  - AF-2: Sidecar recovers → next health check marks it healthy, subsequent searches use semantic path
- **Postconditions:** Search responds within 2s regardless of sidecar state

### UC-007: Configurable DB Pool Size
- **Actor:** User (Self-Hoster) via config
- **Preconditions:** User sets `infrastructure.postgres.max_conns` and `min_conns` in `config/smackerel.yaml`
- **Main Flow:**
  1. `./smackerel.sh config generate` emits `DB_MAX_CONNS` and `DB_MIN_CONNS` into generated env files
  2. Core reads env vars at startup
  3. `db.Connect()` uses configured values for pool sizing
- **Alternative Flows:**
  - AF-1: Env vars not set → startup fails loudly (no fallback to hardcoded 10/2)
- **Postconditions:** DB pool runs with user-specified connection limits

## Business Scenarios

### BS-001: User Backs Up Before System Maintenance
Given the Smackerel stack is running
When the user runs `./smackerel.sh backup`
Then a compressed pg_dump file is created in `backups/` with a timestamped name
And the CLI prints the file path and size
And the dump can be restored with `pg_restore` or `psql`

### BS-002: User Backs Up With Database Down
Given the Smackerel stack is running but PostgreSQL is unreachable
When the user runs `./smackerel.sh backup`
Then the CLI exits with a non-zero exit code
And the error message indicates the database is unreachable

### BS-003: Capture During Database Outage Returns Explicit Error
Given PostgreSQL is temporarily unreachable
When a user submits a capture request via API
Then the API returns HTTP 503 with error code `DB_UNAVAILABLE`
And no artifact data is silently dropped

### BS-004: Capture During Normal Operation Succeeds
Given PostgreSQL is healthy
When a user submits a valid capture request
Then the artifact is persisted and an HTTP 200 response is returned

### BS-005: Long-Running Synthesis Does Not Block Hourly Momentum
Given the daily synthesis job (5-minute timeout) is currently running
When the hourly topic momentum job fires
Then the momentum job acquires its own mutex and runs concurrently with synthesis
And if a second synthesis job fires, it is skipped with a warning log

### BS-006: All Nine Cron Jobs Protected From Self-Overlap
Given 9 cron jobs are registered (digest, momentum, synthesis, resurfacing, pre-meeting briefs, weekly synthesis, monthly report, subscription detection, frequent lookups)
When any job fires while a previous instance of the same type is still running
Then the new invocation is skipped and a warning is logged
And different job types may run concurrently

### BS-007: NATS Message Exhaustion Routes to Dead-Letter
Given a NATS consumer has a message that has failed MaxDeliver (5) times
When the consumer gives up on the message
Then the message payload and metadata are routed to the DEADLETTER stream
And the message is inspectable via NATS CLI or monitoring

### BS-008: Graceful Shutdown Completes Within Docker Timeout
Given the core service is running with active connections
When Docker sends SIGTERM (e.g., `./smackerel.sh down`)
Then the shutdown sequence completes within 30 seconds
And all subsystems are stopped in reverse-dependency order
And the process exits cleanly before Docker sends SIGKILL

### BS-009: Shutdown Order Prevents NATS Drain Racing DB Close
Given the core service is shutting down
When the shutdown sequence reaches NATS drain
Then the DB pool is still open (NATS drain happens before DB close)
And any in-flight NATS handlers that touch the DB can complete

### BS-010: Search Degrades Gracefully When ML Sidecar Down
Given the ML sidecar is unreachable
When a user performs a search
Then the search handler skips the NATS embed request
And returns text-fallback results within 2 seconds
And the response includes `search_mode: "text_fallback"`

### BS-011: Search Uses Semantic Path When ML Sidecar Recovers
Given the ML sidecar was down but has recovered
When the cached health check detects the sidecar is healthy again
Then subsequent searches use the full semantic (embedding → vector) path

### BS-012: DB Pool Size Flows From SST Config
Given the user sets `infrastructure.postgres.max_conns: 20` in `config/smackerel.yaml`
When `./smackerel.sh config generate` runs and the core service starts
Then the database connection pool is configured with MaxConns=20
And no hardcoded pool size value is used

### BS-013: DB Pool Config Missing Fails Loudly
Given `DB_MAX_CONNS` or `DB_MIN_CONNS` is not set in the environment
When the core service starts
Then startup fails with an explicit error naming the missing variable

## Competitive Analysis

Not applicable — this is an internal operational resilience feature with no user-facing competitive dimension. All improvements are driven by stability findings, not competitor comparison.

## Improvement Proposals

### IP-001: Backup CLI Command ⭐ Data Protection
- **Impact:** High
- **Effort:** S
- **Rationale:** Single point of failure for user's entire knowledge base; pg_dump is a well-understood, reliable mechanism
- **Actors Affected:** User (Self-Hoster)
- **Business Scenarios:** BS-001, BS-002

### IP-002: Fail-Visible Capture on DB Outage ⭐ Data Integrity
- **Impact:** High
- **Effort:** S
- **Rationale:** Silent data loss is the worst failure mode for a knowledge engine; explicit 503 lets callers retry
- **Actors Affected:** User, Smackerel Core
- **Business Scenarios:** BS-003, BS-004

### IP-003: Cron Job Concurrency Guards ⭐ Stability
- **Impact:** High
- **Effort:** S
- **Rationale:** 9 jobs competing for 10 DB connections without overlap protection creates pool exhaustion under load
- **Actors Affected:** Scheduler
- **Business Scenarios:** BS-005, BS-006

### IP-004: NATS Dead-Letter Stream ⭐ Message Reliability
- **Impact:** Medium
- **Effort:** S
- **Rationale:** Messages permanently lost after 5 retries with no forensic trail; dead-letter preserves them for inspection
- **Actors Affected:** NATS JetStream, ML Sidecar
- **Business Scenarios:** BS-007

### IP-005: Sequential Shutdown Ordering ⭐ Stability
- **Impact:** Medium
- **Effort:** M
- **Rationale:** Current defer-based shutdown races NATS drain with DB close; explicit ordering prevents corruption
- **Actors Affected:** Smackerel Core
- **Business Scenarios:** BS-008, BS-009

### IP-006: Docker stop_grace_period Alignment ⭐ Stability
- **Impact:** Medium
- **Effort:** S
- **Rationale:** 10s default is insufficient for Telegram 30s long-poll + NATS drain + connector shutdown
- **Actors Affected:** Smackerel Core
- **Business Scenarios:** BS-008

### IP-007: ML Sidecar Health Caching for Search ⭐ Responsiveness
- **Impact:** High
- **Effort:** S
- **Rationale:** Search currently blocks on NATS embed request when ML sidecar is down; goroutine pile-up risk under load
- **Actors Affected:** User, Smackerel Core
- **Business Scenarios:** BS-010, BS-011

### IP-008: Configurable DB Pool Size via SST ⭐ Operational Control
- **Impact:** Low
- **Effort:** S
- **Rationale:** Hardcoded MaxConns=10, MinConns=2 cannot be tuned without code changes; SST config is the required pattern
- **Actors Affected:** User (Self-Hoster)
- **Business Scenarios:** BS-012, BS-013

## UI Scenario Matrix

Not applicable — all changes are backend/CLI/infrastructure with no UI surface.

## Non-Functional Requirements

- **Backup performance:** pg_dump must complete within 5 minutes for databases up to 10GB
- **Capture latency:** DB health check must add <5ms to capture request path
- **Search latency:** Text-fallback path must return results within 2s when ML sidecar is down
- **Shutdown budget:** Full sequential shutdown must complete within 25s (5s margin before 30s SIGKILL)
- **Observability:** All resilience events (backup, DB health failure, cron skip, dead-letter, shutdown steps) must be logged at INFO or WARN level with structured fields
- **Configuration:** All new tunables (pool size, health cache TTL, shutdown timeout) must flow from `config/smackerel.yaml` via the SST pipeline

## Requirements

- R-001: `./smackerel.sh backup` executes pg_dump against the running PostgreSQL instance and writes a compressed dump to `backups/smackerel-YYYY-MM-DD-HHMMSS.sql.gz`
- R-002: `./smackerel.sh backup` fails loudly with non-zero exit code if PostgreSQL is unreachable or pg_dump is not available
- R-003: POST /api/capture returns HTTP 503 with `DB_UNAVAILABLE` error code when the DB pool reports unhealthy
- R-004: Each cron job type has an independent `sync.Mutex`; if the mutex is held when the job fires, the invocation is skipped with a WARN log
- R-005: NATS stream configuration includes a `DEADLETTER` stream; consumers use `MaxDeliver` with an advisory that routes exhausted messages to the dead-letter stream
- R-006: Shutdown follows explicit sequential order: scheduler → HTTP server → Telegram → subscribers → connectors → NATS drain → DB close
- R-007: Docker Compose sets `stop_grace_period: 30s` on the `smackerel-core` service
- R-008: Search handler checks a cached ML sidecar health status before attempting NATS embed; if unhealthy, falls back to text search without blocking
- R-009: ML sidecar health cache TTL is configurable via SST config (default suggestion: 10s)
- R-010: DB pool MaxConns and MinConns are read from `DB_MAX_CONNS` and `DB_MIN_CONNS` environment variables; missing values cause startup failure
- R-011: `config/smackerel.yaml` gains `infrastructure.postgres.max_conns` (default: 10) and `infrastructure.postgres.min_conns` (default: 2) fields
- R-012: `config/smackerel.yaml` gains `services.ml.health_cache_ttl` field (default: 10s)
- R-013: The HTTP shutdown timeout in `main.go` is replaced with a value derived from SST config, not hardcoded 10s

## User Scenarios (Gherkin)

```gherkin
Scenario: Successful backup of knowledge base
  Given the Smackerel stack is running with PostgreSQL healthy
  When the user runs "./smackerel.sh backup"
  Then a file matching "backups/smackerel-*.sql.gz" is created
  And the file size is greater than 0
  And the CLI exits with code 0

Scenario: Backup fails when database is unreachable
  Given PostgreSQL is not running
  When the user runs "./smackerel.sh backup"
  Then the CLI exits with a non-zero exit code
  And stderr contains "database" and "unreachable" or "connection refused"

Scenario: Capture returns 503 when database is down
  Given PostgreSQL is temporarily unreachable
  When a POST request is sent to /api/capture with valid input
  Then the response status is 503
  And the response body contains error code "DB_UNAVAILABLE"

Scenario: Capture succeeds when database is healthy
  Given PostgreSQL is healthy
  When a POST request is sent to /api/capture with valid input
  Then the response status is 200
  And the artifact is persisted in the database

Scenario: Overlapping cron job is skipped
  Given a synthesis cron job is currently running
  When a second synthesis cron job fires
  Then the second invocation is skipped
  And a WARN log "skipping overlapping job" is emitted

Scenario: Different cron job types run concurrently
  Given a synthesis job is running
  When the hourly momentum job fires
  Then both jobs execute concurrently

Scenario: Exhausted NATS message routed to dead-letter
  Given a NATS consumer message has exceeded MaxDeliver attempts
  When the consumer abandons the message
  Then the message appears in the DEADLETTER stream
  And the dead-letter message contains the original payload

Scenario: Shutdown completes in sequential order within 30s
  Given the core service is running with all subsystems
  When SIGTERM is received
  Then scheduler stops before HTTP server
  And HTTP server stops before Telegram bot
  And NATS drains before DB pool closes
  And the process exits within 30 seconds

Scenario: Search falls back to text when ML sidecar is down
  Given the ML sidecar health cache reports unhealthy
  When a POST request is sent to /api/search with a query
  Then the response contains results with search_mode "text_fallback"
  And the response time is under 2 seconds

Scenario: DB pool size from SST config
  Given config/smackerel.yaml has infrastructure.postgres.max_conns set to 20
  When config is generated and the core service starts
  Then the database connection pool MaxConns is 20

Scenario: Missing DB pool config fails startup
  Given DB_MAX_CONNS is not set in the environment
  When the core service starts
  Then startup fails with an error message naming "DB_MAX_CONNS"
```

## Acceptance Criteria

- AC-001: `./smackerel.sh backup` produces a valid, non-empty `.sql.gz` file (maps to BS-001, R-001)
- AC-002: `./smackerel.sh backup` exits non-zero when DB is unreachable (maps to BS-002, R-002)
- AC-003: POST /api/capture returns 503 with `DB_UNAVAILABLE` when DB pool is unhealthy (maps to BS-003, R-003)
- AC-004: POST /api/capture returns 200 and persists artifact when DB is healthy (maps to BS-004)
- AC-005: Overlapping cron jobs of the same type are skipped with WARN log (maps to BS-005, BS-006, R-004)
- AC-006: Exhausted NATS messages appear in DEADLETTER stream with original payload (maps to BS-007, R-005)
- AC-007: Shutdown completes in explicit sequential order within 30s (maps to BS-008, BS-009, R-006, R-007)
- AC-008: Search returns text-fallback results within 2s when ML sidecar is down (maps to BS-010, R-008)
- AC-009: Search uses semantic path when ML sidecar health cache reports healthy (maps to BS-011)
- AC-010: DB pool size is configurable via SST and missing config fails startup (maps to BS-012, BS-013, R-010, R-011)
