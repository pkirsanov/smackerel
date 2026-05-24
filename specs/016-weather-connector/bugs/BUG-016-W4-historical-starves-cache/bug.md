# Bug: [BUG-016-W4] Historical entries with 100-year TTL starve weather connector cache

> **Parent Feature:** [specs/016-weather-connector](../../)
> **Discovered From:** [.specify/memory/sweep-2026-05-23-r30.json](../../../../.specify/memory/sweep-2026-05-23-r30.json) round 26 stabilize trigger
> **Date Opened:** 2026-05-24
> **Status:** Fixed and verified - bugfix-fastlane certification recorded
> **Severity:** High (cache silently degrades to zero hit rate under realistic historical-enrichment load)
> **Workflow Mode:** bugfix-fastlane

---

## Summary

The stabilize trigger probe against `internal/connector/weather/weather.go` found that
historical archive entries are inserted into the shared 1024-entry cache with a 100-year
TTL (line 815: `expiresAt: time.Now().Add(100 * 365 * 24 * time.Hour)`). The eviction
helper `evictExpiredLocked` (line 822) only removes entries whose `expiresAt` is in the
past, so historical entries never become eviction candidates.

Once 1024+ unique historical lookups accumulate (a realistic load when the Maps connector
enriches past trips, or when a Drive / Photo / Mealplan workflow requests historical
weather context across many dates), every subsequent `decodeCurrent` and
`decodeForecast` insertion hits the `len(c.cache) < maxCacheEntries` guard, logs
`weather cache full, discarding new entry`, and bypasses caching entirely. From that
point onward every weather `Sync()` re-fetches current and forecast data from Open-Meteo
for every configured location instead of serving from cache.

The defect is silent at the API surface: artifacts still publish correctly. The damage
shows up as multiplied upstream API load (50 locations × every sync interval), longer
sync durations, and eventual rate-limit exposure on the free Open-Meteo tier.

The existing `TestCacheOverflow_AllValid` (weather_test.go:222) documents the broken
behavior as if it were intentional: when the cache is full of "all valid" entries the
new entry is dropped. That assumption ignores that historical entries are effectively
permanent and structurally indistinguishable from "still-valid ephemeral" entries.

## Finding IDs

| ID | Classification | Owner Surface | Evidence Source |
|----|----------------|---------------|-----------------|
| BUG-016-W4-F1 | Production stability defect — cache permanently saturated by historical entries blocks ephemeral cache hits | `internal/connector/weather/weather.go` `decodeCurrent` / `decodeForecast` / `decodeHistorical` cache insertion path and `evictExpiredLocked` | sweep-2026-05-23-r30 round 26 stabilize probe |
| BUG-016-W4-F2 | Test regression — `TestCacheOverflow_AllValid` codifies the broken policy and would block the fix | `internal/connector/weather/weather_test.go::TestCacheOverflow_AllValid` | sweep-2026-05-23-r30 round 26 stabilize probe |

## Ownership Classification

This is owned by `specs/016-weather-connector`. The defect surface is entirely inside
`internal/connector/weather/weather.go` and the broken-policy test is in
`internal/connector/weather/weather_test.go`. No shared infrastructure, no other
connector, and no other spec is implicated.

## Reproduction Evidence

The defect was surfaced by direct source review during the round 26 stabilize probe.
The mechanical reproduction is:

1. Populate `c.cache` with `maxCacheEntries` (1024) historical entries each with
   `expiresAt: time.Now().Add(100 * 365 * 24 * time.Hour)`.
2. Call `decodeCurrent` (or `decodeForecast`) on a fresh upstream response.
3. Observe that `evictExpiredLocked` removes zero entries (none are expired),
   the `len(c.cache) < maxCacheEntries` guard fails, and the new entry is
   discarded with `weather cache full, discarding new entry` logged.

The existing `TestCacheOverflow_AllValid` (weather_test.go:222) test demonstrates this
exact code path, but it asserts the wrong policy as the desired outcome.

## Expected Behavior

- Permanent historical entries MUST NOT crowd out shorter-TTL current/forecast entries
  from the bounded cache.
- When the cache is at `maxCacheEntries` capacity and no entries are expired, eviction
  MUST make room for a new entry by removing the entry with the latest `expiresAt`
  (which is structurally biased toward historical entries because their `expiresAt`
  is decades ahead of current/forecast entries).
- A current entry inserted into a cache full of historical entries MUST be cached
  (must satisfy `cache[currentKey] != nil` after insertion).
- The cache MUST still enforce the `maxCacheEntries` hard cap (no overflow allowed).

## Actual Behavior

- Historical entries fill the cache and never expire within process lifetime.
- New current/forecast insertions are silently dropped with a warning log line.
- Every subsequent `Sync()` re-calls Open-Meteo for every location instead of serving
  from cache.

## Impact

Under realistic Maps-driven historical-enrichment load (hundreds to thousands of unique
`(lat, lon, date)` tuples for past trips), the connector's cache effectively becomes
write-once for historical data. After ~1024 historical lookups, the entire
current/forecast cross-sync cache benefit disappears. With 50 locations configured and
a 15-minute sync interval, this multiplies Open-Meteo API load by 50× the steady-state
expected rate, exposes the deployment to free-tier rate limiting, and increases each
`Sync()` duration from ~50 ms (all-cache-hit) to several seconds (all-API-call) per
location.

## Certification Status

BUG-016-W4 is fixed and verified in the bugfix-fastlane lane. The fix is limited to:

- `internal/connector/weather/weather.go` — `evictExpiredLocked` renamed to `evictOneLocked` with a fallback eviction strategy.
- `internal/connector/weather/weather_test.go` — `TestCacheOverflow_AllValid` updated to assert the new policy; new adversarial test `TestEviction_HistoricalDoesNotStarveEphemeral` added; new `TestEviction_LongestExpiryEvictedWhenFull` invariant test added.

No production behavior outside the cache eviction policy was changed. No config, deploy,
or runtime contract was touched. Shared live-stack readiness remains routed to
`specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness` and is not
absorbed into this bug.
