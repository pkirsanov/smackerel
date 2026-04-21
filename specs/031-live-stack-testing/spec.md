# Feature: 031 — Live-Stack Integration & E2E Testing

## Problem Statement

All 38 Go packages and 173 Python tests pass at the unit level using mocks, but zero tests have been executed against the real running Docker stack. The pipeline flow (artifact capture → NATS publish → ML sidecar processing → NATS response → DB write → graph linking → knowledge synthesis) has never been tested end-to-end against real PostgreSQL, real NATS JetStream, and the real ML sidecar. This scored 3/10 in the system review and is the most critical production readiness gap.

## Outcome Contract

**Intent:** Integration tests verify each service boundary (Go ↔ PostgreSQL, Go ↔ NATS, Go ↔ ML sidecar) against real running containers. E2E tests verify complete user journeys (capture a URL → process → search → find it) against the full stack.

**Success Signal:** `./smackerel.sh test integration` runs against `docker compose up` and verifies: DB migrations apply, NATS streams create, artifacts can be inserted and queried, embeddings round-trip through NATS. `./smackerel.sh test e2e` verifies: capture a URL via API → wait for processing → search by content → get result.

**Hard Constraints:**
- Tests must use the disposable test stack, never the persistent dev stack
- Test data must be uniquely identifiable and safe to clean up
- Tests must be idempotent — running twice produces the same result
- No LLM API calls in integration tests (mock LLM at the NATS boundary or use Ollama)
- E2E tests may use Ollama for real LLM processing but must have a timeout
- Tests must clean up after themselves (no leftover test artifacts in DB)

**Failure Condition:** If integration tests pass but E2E tests require manual intervention, the automation is incomplete. If tests leave state in the DB that affects subsequent runs, they're not isolated.

## Goals

1. Create integration test suite that runs against real PostgreSQL + NATS
2. Verify DB migration chain applies cleanly (consolidated schema: 001, 018, 019)
3. Verify NATS stream creation and message round-trip
4. Verify artifact CRUD against real PostgreSQL (insert, query, update, search)
5. Verify pgvector similarity search with real embeddings
6. Create E2E test suite for complete user journeys
7. Verify capture → process → search flow end-to-end
8. Add ML sidecar readiness gate to prevent search timeouts during cold start

## Non-Goals

- Performance/load testing (covered by stress tests)
- UI E2E testing (no committed UI beyond HTMX web, Telegram is tested separately)
- Multi-user concurrency testing (single-user system)
- Testing against cloud LLM providers (use Ollama or mock)

## User Scenarios (Gherkin)

```gherkin
Scenario: Database migrations apply cleanly
  Given a fresh PostgreSQL instance
  When all consolidated migrations (001, 018, 019) are applied in sequence
  Then all tables, indexes, and constraints exist
  And no migration fails

Scenario: NATS streams are created
  Given a fresh NATS instance
  When EnsureStreams is called
  Then all 11 streams exist (ARTIFACTS, SEARCH, DIGEST, KEEP, INTELLIGENCE, ALERTS, SYNTHESIS, DOMAIN, ANNOTATIONS, LISTS, DEADLETTER)

Scenario: Artifact insert and vector search
  Given migrations are applied and an embedding exists
  When an artifact with embedding is inserted
  And a vector similarity search is performed with a related query
  Then the inserted artifact appears in results

Scenario: Capture-to-search E2E
  Given the full stack is running (core, ML, PostgreSQL, NATS)
  When a text artifact is captured via POST /api/capture
  And processing completes (artifacts.processed received)
  Then searching for content from that artifact returns it in results

Scenario: Domain extraction E2E
  Given a recipe URL is captured
  When processing and domain extraction complete
  Then the artifact has domain_data with ingredients and steps
  And searching "recipes with [ingredient]" returns the artifact

Scenario: Test isolation
  Given integration tests have run
  When checking the database for test artifacts
  Then no test artifacts remain (cleanup completed)

Scenario: ML sidecar readiness gate prevents cold-start timeouts
  Given the ML sidecar container has just started
  When a search request arrives before the sidecar is ready
  Then the core waits for sidecar health before attempting NATS embed
  And the search falls back to text mode if readiness times out

Scenario: NATS consumer replay after crash
  Given the ML sidecar crashes while processing a message
  When the sidecar restarts and the consumer replays unacknowledged messages
  Then the replayed message is processed correctly (idempotent handling)
  And no duplicate artifacts or domain_data entries are created

Scenario: Schema DDL resilience (table drop and recreate)
  Given the consolidated schema has been applied
  When specific tables are dropped and recreated via DDL
  Then other tables are unaffected
  And the dropped tables can be recreated from migration SQL

Scenario: Tests run against populated database
  Given the test database already contains artifacts from a previous test run
  When integration tests run
  Then tests use unique identifiers that don't collide with existing data
  And test assertions don't depend on the database being empty

Scenario: Annotation CRUD against real PostgreSQL
  Given migrations for annotations have been applied
  When an annotation is created, queried, and the summary materialized view is refreshed
  Then all operations succeed against real PostgreSQL
  And the materialized view returns correct aggregated data

Scenario: List generation against real PostgreSQL
  Given migrations for lists have been applied and artifacts with domain_data exist
  When a shopping list is generated from those artifacts
  Then the list and items are persisted in real PostgreSQL
  And item status updates correctly modify checked_items counters
```

## Acceptance Criteria

- [ ] `./smackerel.sh test integration` runs against Docker stack and passes
- [ ] All consolidated migrations (001, 018, 019) apply cleanly on fresh PostgreSQL
- [ ] Schema DDL resilience tested (table drop and recreate for lists/list_items)
- [ ] NATS stream creation verified with 11 streams
- [ ] NATS consumer replay/idempotency tested after simulated crash
- [ ] Artifact CRUD operations verified against real PostgreSQL
- [ ] pgvector similarity search works with real embeddings
- [ ] Annotation CRUD verified against real PostgreSQL (spec 027)
- [ ] List generation verified against real PostgreSQL (spec 028)
- [ ] `./smackerel.sh test e2e` verifies capture → process → search flow
- [ ] All test data cleaned up after test run
- [ ] Tests are idempotent (re-runnable without manual cleanup)
- [ ] ML sidecar readiness gate prevents search embed timeouts during container cold start
