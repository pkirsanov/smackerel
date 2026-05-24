# Spec: BUG-016-W4 — Weather connector cache must not be starved by permanent historical entries

> **Parent Feature:** [specs/016-weather-connector](../../)
> **Bug Packet:** [BUG-016-W4-historical-starves-cache](.)
> **Workflow Mode:** bugfix-fastlane
> **Created:** 2026-05-24

## Use Cases

- **UC-01: Maps enrichment under heavy historical-lookup load.** A Maps connector
  enriches several thousand past trips via `weather.enrich.request`. The historical
  archive cache must not destroy the cross-sync benefit of the current/forecast cache
  for ongoing weather syncs.
- **UC-02: Long-running deployment with steady historical-enrichment background.** A
  deployment with EnableAlerts and 50 configured locations runs for weeks. Background
  historical enrichment continues to accumulate, but the cache must keep serving
  current/forecast hits for the scheduled `Sync()` cycles instead of silently degrading
  to "all cache misses".
- **UC-03: Hard cap is preserved.** Under all eviction conditions the cache size never
  exceeds `maxCacheEntries`.

## Functional Requirements

- **FR-01:** `decodeCurrent` insertion of a fresh entry MUST succeed when the cache is
  full and all existing entries are unexpired historical entries.
- **FR-02:** `decodeForecast` insertion of a fresh entry MUST succeed when the cache is
  full and all existing entries are unexpired historical entries.
- **FR-03:** `decodeHistorical` insertion of a fresh entry MUST succeed when the cache
  is full.
- **FR-04:** The eviction helper MUST first remove expired entries; only if zero entries
  are expired MUST it fall back to removing the entry with the latest `expiresAt`
  (the most-permanent entry).
- **FR-05:** After any successful insertion the cache size MUST remain at most
  `maxCacheEntries`.
- **FR-06:** All call sites that previously invoked `evictExpiredLocked` MUST be updated
  to invoke the renamed helper.

## Acceptance Criteria

- **AC-01:** A unit test populates the cache with `maxCacheEntries` historical entries
  (100-year TTL) and asserts that a subsequent current/forecast insertion is present
  in the cache after the call returns.
- **AC-02:** A unit test populates the cache with mixed entries (some historical, some
  ephemeral) and asserts that when the cache is full of valid entries, eviction targets
  the entry with the latest `expiresAt`.
- **AC-03:** The existing `TestCacheOverflow_AllValid` test is updated so that its
  assertion reflects the new policy (an entry IS inserted when cache is full of valid
  entries, evicting the longest-expiry entry).
- **AC-04:** Adversarial fidelity: reverting the production fix MUST cause the new
  `TestEviction_HistoricalDoesNotStarveEphemeral` test to fail with a concrete
  diagnostic naming the missing current/forecast key.
- **AC-05:** `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`,
  and `./smackerel.sh test unit --go` for `internal/connector/weather/...` all pass.
- **AC-06:** `state-transition-guard.sh` against the BUG packet emits TRANSITION PERMITTED
  (zero BLOCK findings).
- **AC-07:** `artifact-lint.sh` against the BUG packet emits PASSED.
- **AC-08:** `traceability-guard.sh` against the BUG packet emits PASSED with G068
  DoD-Gherkin fidelity 100%.

## Out-of-Scope

- Partitioning the cache into separate historical and ephemeral maps (would also work
  but adds more code surface than the minimal eviction-policy change).
- Changing the 100-year TTL for historical entries (deliberate design choice; correctness
  is preserved as long as eviction handles the saturation case).
- LRU tracking of access time (the existing eviction algorithm uses TTL only; the fix
  preserves that contract).
- Shared live-stack readiness (routed to `specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`).
