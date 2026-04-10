# Feature: 023 Engineering Quality

## Problem Statement

An engineering review of the Smackerel codebase identified correctness bugs, SST config violations, type-safety gaps, dead code, inconsistent handler patterns, placeholder health checks, excessive logging, and hardcoded scheduling. These issues degrade reliability under concurrency, violate the project's Configuration Single Source of Truth (SST) policy, weaken compile-time safety, and reduce operational observability. Addressing them now prevents compounding technical debt as the feature surface grows.

## Outcome Contract

**Intent:** Eliminate identified code quality and correctness issues across the Go core runtime so that the system is safe under concurrency, config-compliant, type-safe, and operationally clean.

**Success Signal:** All nine findings are resolved with verified test coverage: mlClient race is gone (race detector clean), connector env vars flow through config.Config, Dependencies uses typed interfaces, dead code is removed, intelligence handlers use writeJSON, Ollama and Telegram health probes are live, health endpoint requests are excluded from request logging, and connector sync intervals honour per-connector sync_schedule from smackerel.yaml.

**Hard Constraints:**
- Zero hardcoded config values — all values originate from `config/smackerel.yaml` (SST policy)
- No runtime type assertions for owned internal boundaries — compile-time interface satisfaction required
- Health endpoint must remain unauthenticated and backward-compatible (same JSON shape)
- No new dependencies introduced beyond stdlib and already-vendored packages

**Failure Condition:** Any finding regresses, a race detector failure surfaces in the changed code paths, or a new SST violation is introduced during the fix.

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| Operator | Self-hosted user running the Smackerel stack | Reliable health checks, clean logs, correct scheduling | Full system access |
| Monitoring Probe | Docker/external health check caller (e.g., Docker HEALTHCHECK, uptime robot) | Fast, accurate health status without auth or log pollution | Unauthenticated GET /api/health |
| Connector Runtime | Internal goroutine-per-connector sync loop | Honour configured schedule, report accurate health | Internal process |

## Use Cases

### UC-001: Concurrent Health Checks

- **Actor:** Monitoring Probe
- **Preconditions:** Core runtime is running; ML sidecar URL is configured
- **Main Flow:**
  1. Multiple concurrent GET /api/health requests arrive
  2. Each request reads the shared ML HTTP client
  3. Health response is returned with accurate per-service status
- **Alternative Flows:**
  - ML sidecar is down → status "down" without panic or race
- **Postconditions:** No data race on MLClient; response is valid JSON

### UC-002: Connector Sync Scheduling

- **Actor:** Connector Runtime
- **Preconditions:** Connector is registered and connected; smackerel.yaml has per-connector sync_schedule
- **Main Flow:**
  1. Supervisor starts connector sync loop
  2. After each successful sync, supervisor reads the connector's configured sync_schedule
  3. Supervisor waits for the schedule-derived interval before next sync
- **Alternative Flows:**
  - No sync_schedule configured → use a sensible fallback from config (not hardcoded)
- **Postconditions:** Connector syncs at the frequency defined in smackerel.yaml

### UC-003: Operational Log Cleanliness

- **Actor:** Operator
- **Preconditions:** Docker HEALTHCHECK or monitoring probe is configured at typical 10-second intervals
- **Main Flow:**
  1. Health check requests arrive every 10 seconds
  2. Request logger skips /api/health path
  3. Operator views logs without health check noise
- **Postconditions:** Health check requests do not appear in application logs

### UC-004: Live Service Health Probing

- **Actor:** Monitoring Probe
- **Preconditions:** Ollama and/or Telegram bot are configured
- **Main Flow:**
  1. GET /api/health is called
  2. Ollama status is determined by probing GET /api/tags on the configured Ollama URL
  3. Telegram bot status is determined by querying the bot's connection state from Dependencies
- **Postconditions:** Health response reflects actual reachability of Ollama and Telegram

## Business Scenarios

### BS-001: Race-Free Health Under Concurrency

Given the core runtime is serving traffic
When 50 concurrent health check requests arrive simultaneously
Then all requests return valid JSON with no race condition and no panic

### BS-002: Config-Driven Connector Scheduling

Given smackerel.yaml defines sync_schedule "*/30 * * * *" for the RSS connector
When the RSS connector completes a sync cycle
Then the supervisor waits ~30 minutes (not 5 minutes) before the next sync

### BS-003: SST-Compliant Connector Config

Given smackerel.yaml defines bookmarks.import_dir, browser.history_path, and maps.import_dir
When the core runtime starts connectors
Then connector paths are read from config.Config, not from raw os.Getenv()

### BS-004: Compile-Time Dependency Safety

Given the Dependencies struct uses typed interfaces for Pipeline, SearchEngine, DigestGen, WebHandler, OAuthHandler
When a developer changes an interface method signature
Then compilation fails immediately rather than silently passing until a runtime type assertion panics

### BS-005: Clean Operational Logs

Given Docker HEALTHCHECK probes /api/health every 10 seconds
When the operator reviews application logs after 24 hours
Then zero health check request log lines are present (saving ~8,640 lines/day)

### BS-006: Accurate Ollama Health

Given Ollama is running and accessible at the configured URL
When GET /api/health is called
Then services.ollama.status is "up" (not hardcoded "unavailable")

### BS-007: Accurate Telegram Bot Health

Given Telegram bot is initialized and connected
When GET /api/health is called
Then services.telegram_bot.status reflects the bot's actual connection state

### BS-008: No Dead Code

Given the codebase has been cleaned
When a developer searches for checkAuth in capture.go
Then no results are found (the dead method has been removed)

### BS-009: Consistent Handler Error Patterns

Given the intelligence handlers (ExpertiseHandler, LearningPathsHandler, SubscriptionsHandler, SerendipityHandler) use writeJSON
When any intelligence endpoint returns a success response
Then the response uses the same writeJSON helper as all other API handlers

## Requirements

### R-ENG-001: Fix mlClient() Race Condition (High)

The `mlClient()` method on Dependencies (health.go:136) lazily initializes `MLClient` without synchronization. Under concurrent health checks this is a data race. Fix by initializing MLClient in the constructor or using sync.Once.

**Evidence:** `internal/api/health.go` lines 136-139 — unsynchronized nil check and assignment.

### R-ENG-002: Route Connector Env Vars Through Config (Medium)

`BOOKMARKS_IMPORT_DIR`, `BROWSER_HISTORY_PATH`, and `MAPS_IMPORT_DIR` are read via raw `os.Getenv()` in `cmd/core/main.go` (lines ~155, 170, 185). This bypasses the SST config struct. Add these fields to `config.Config` and read from `config/smackerel.yaml`.

**Evidence:** `cmd/core/main.go` lines 149, 167, 185 — raw os.Getenv() calls.

### R-ENG-009: Replace interface{} Dependencies With Typed Interfaces (Medium)

The `Dependencies` struct uses `interface{}` for Pipeline, SearchEngine, DigestGen, WebHandler, and OAuthHandler. This forces runtime type assertions in `router.go` and `capture.go`. Define named interfaces and use them as field types.

**Evidence:** `internal/api/health.go` lines 23-27 — `interface{}` fields. `internal/api/router.go` lines 56-95 — runtime type assertions.

### R-ENG-010: Remove Dead checkAuth Method (Low)

`checkAuth` in `capture.go` (line 126) duplicates `bearerAuthMiddleware` in `router.go` (line 170) and is never called. Remove it.

**Evidence:** `internal/api/capture.go` lines 126-143 — unused method.

### R-ENG-013: Use writeJSON Helper in Intelligence Handlers (Medium)

Intelligence handlers (`ExpertiseHandler`, `LearningPathsHandler`, `SubscriptionsHandler`, `SerendipityHandler`) manually call `json.NewEncoder(w).Encode()` instead of using the `writeJSON` helper. This skips the consistent status-code and content-type handling that other handlers use.

**Evidence:** `internal/api/intelligence.go` — four handlers with manual JSON encoding.

### R-S-001: Add Real Ollama Health Probing (Medium)

Ollama health is hardcoded as `"unavailable"` (health.go:112). Probe `GET /api/tags` on the configured Ollama URL and report actual status.

**Evidence:** `internal/api/health.go` line 112, `config/smackerel.yaml` ollama section.

### R-S-002: Wire Telegram Bot Health Into Dependencies (Medium)

Telegram bot status is hardcoded as `"disconnected"` (health.go:109). Wire the bot's connection state (or a health interface) into Dependencies and report live status.

**Evidence:** `internal/api/health.go` line 109.

### R-S-007: Exclude /api/health From Request Logging (Low)

The `structuredLogger` middleware logs every request including health checks. At typical 10-second probe intervals, this generates ~8,640 unnecessary log lines per day. Skip logging for `/api/health` and `/ping`.

**Evidence:** `internal/api/router.go` lines 121-133 — structuredLogger logs all paths.

### R-S-014: Use Per-Connector sync_schedule From Config (Medium)

The connector supervisor hardcodes a 5-minute wait between sync cycles (`supervisor.go:240`). `smackerel.yaml` already defines per-connector `sync_schedule` cron expressions. The supervisor should parse and use these schedules.

**Evidence:** `internal/connector/supervisor.go` line 240 — `time.After(5 * time.Minute)`. `config/smackerel.yaml` — per-connector `sync_schedule` fields.

## Non-Functional Requirements

- **Concurrency Safety:** All shared mutable state must be safe under concurrent access. The Go race detector (`go test -race`) must pass cleanly on all changed packages.
- **Performance:** Health check latency must not increase by more than 50ms from added Ollama/Telegram probes. Use timeouts (2s) on probe requests.
- **Observability:** Log volume reduction from health check exclusion should be measurable (~8,640 fewer lines/day at 10s probe interval).
- **Backward Compatibility:** Health endpoint JSON response shape must remain unchanged (existing fields preserved, new statuses use same ServiceStatus shape).
- **SST Compliance:** Zero raw `os.Getenv()` calls for values that exist in `config/smackerel.yaml`.

## Competitive Analysis

Not applicable — this is an internal engineering quality feature with no user-facing competitive dimension.

## Improvement Proposals

### IP-001: Typed Dependency Injection Pattern

- **Impact:** High
- **Effort:** M
- **Rationale:** Replacing interface{} with typed interfaces (R-ENG-009) enables compile-time verification of the entire handler wiring. This also makes the codebase more navigable for IDE tooling (go-to-definition, find-all-references).
- **Actors Affected:** All developers working on the codebase

### IP-002: Centralized Health Probe Registry

- **Impact:** Medium
- **Effort:** S
- **Rationale:** Rather than adding Ollama and Telegram probes ad-hoc, introduce a simple probe registry pattern where each service registers a health check function. This scales as more services are added.
- **Actors Affected:** Operator, developers adding new services

### IP-003: Structured Connector Schedule Management

- **Impact:** Medium
- **Effort:** M
- **Rationale:** Moving from hardcoded to config-driven scheduling (R-S-014) opens the door to operator-tunable sync frequencies without code changes. Combined with the existing cron library, this is a natural extension.
- **Actors Affected:** Operator, Connector Runtime

## UI Scenario Matrix

Not applicable — this feature has no UI changes.
