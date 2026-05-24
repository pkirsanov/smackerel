# Design: BUG-016-W4 — Weather connector cache eviction must not let permanent historical entries starve ephemeral entries

> **Parent Feature:** [specs/016-weather-connector](../../)
> **Bug Packet:** [BUG-016-W4-historical-starves-cache](.)
> **Workflow Mode:** bugfix-fastlane
> **Created:** 2026-05-24
> **Author:** bubbles.design (parent-expanded under bubbles.workflow stochastic sweep round 26)

## Current Truth

HEAD before this BUG packet: `3b4fe6a4` (round 25 BUG-015-002 reconcile-artifact-drift)

Live code state of `internal/connector/weather/weather.go`:

- `const maxCacheEntries = 1024` (line 26) — single bounded cache shared across current,
  forecast, and historical entries.
- `decodeCurrent` (line 547) inserts entries with `expiresAt: time.Now().Add(30 * time.Minute)`.
- `decodeForecast` (line 654) inserts entries with `expiresAt: time.Now().Add(2 * time.Hour)`.
- `decodeHistorical` (line 757) inserts entries with `expiresAt: time.Now().Add(100 * 365 * 24 * time.Hour)`.
- All three insertions call `evictExpiredLocked()` at line 600 / 703 / 809 when the
  cache is at capacity, then test `len(c.cache) < maxCacheEntries` before insertion.
- `evictExpiredLocked` (line 822) only deletes entries where `now.After(entry.expiresAt)`.
- The existing test `TestCacheOverflow_AllValid` (weather_test.go:222) asserts the
  current (broken) policy: when the cache is full of valid entries, the new entry is
  NOT inserted.

The defect path is exercised whenever `decodeHistorical` is called enough times to
saturate the cache (1024+ unique `(lat, lon, date)` tuples). After saturation,
subsequent `decodeCurrent` and `decodeForecast` insertions silently fail and the
connector loses its cross-sync cache benefit.

## Root Cause

Single-policy eviction (TTL-only) is correct when all entries share a TTL distribution,
but the weather connector mixes ephemeral entries (30 min / 2 h TTL) with effectively
permanent entries (100 year TTL) in the same bounded map. Permanent entries are never
eviction candidates, so once enough of them accumulate, ephemeral entries lose the right
to be cached at all.

## Design Decision

**DEC-1:** Keep the single bounded cache. Do NOT partition into two maps.

Rationale: partitioning would solve the problem but adds more code surface (a second
map, second order-tracking slice or counter, second cap constant, second cleanup path
in `Close()`, second test surface). The minimum-surface fix is a smarter eviction
policy.

**DEC-2:** Replace `evictExpiredLocked` with `evictOneLocked` that:

1. First tries to evict one expired entry (preserves existing TTL-eviction preference).
2. If no entries are expired, evicts the single entry with the latest `expiresAt`
   (structurally biased toward historical entries because their `expiresAt` is decades
   ahead of current/forecast `expiresAt`).
3. Returns whether an entry was evicted (callers use this implicitly by re-checking
   `len(c.cache) < maxCacheEntries`).

Rationale: "evict entry with latest `expiresAt`" naturally targets historical entries
without naming them explicitly. The policy is correct even if historical TTL changes:
as long as historical entries have larger TTL than ephemeral entries, ephemeral entries
will be preferred. If the policy is ever reversed (e.g. all entries given the same TTL),
the eviction becomes uniform random and still correct.

**DEC-3:** The eviction helper evicts exactly ONE entry per call.

Rationale: the existing pattern at all three call sites is "evict if at capacity, then
attempt insertion". Bulk eviction would change the semantics of `len(c.cache) <
maxCacheEntries` check and require more invasive call-site updates.

**DEC-4:** Update `TestCacheOverflow_AllValid` to assert the new policy.

Rationale: the existing test codifies the broken behavior. Keeping it as-is would either
block the fix or require an exemption that hides intent. The renamed/updated test still
verifies the hard `maxCacheEntries` cap.

## Selected Design

```go
// evictOneLocked removes one cache entry to make room for a new insertion.
// Must be called with c.mu held.
//
// Strategy:
//  1. If any entry is expired, delete one expired entry and return. This
//     preserves the historical TTL-eviction preference and never sacrifices
//     a still-valid ephemeral entry while an expired entry exists.
//  2. Otherwise, delete the entry with the latest expiresAt. Historical
//     entries (100-year TTL) are structurally always the largest expiresAt,
//     so they get evicted first when ephemeral entries (30-min/2-hour TTL)
//     compete for the bounded cache. This prevents the failure mode where
//     accumulated historical lookups starve out the cross-sync benefit of
//     current/forecast caching.
//
// The helper evicts exactly one entry; callers re-check
// len(c.cache) < maxCacheEntries before inserting.
func (c *Connector) evictOneLocked() {
	now := time.Now()
	for key, entry := range c.cache {
		if now.After(entry.expiresAt) {
			delete(c.cache, key)
			return
		}
	}
	// No expired entries — evict the entry with the latest expiresAt.
	// This biases eviction toward permanent historical entries when
	// ephemeral entries are competing for cache slots.
	var victimKey string
	var victimExpiry time.Time
	for key, entry := range c.cache {
		if victimKey == "" || entry.expiresAt.After(victimExpiry) {
			victimKey = key
			victimExpiry = entry.expiresAt
		}
	}
	if victimKey != "" {
		delete(c.cache, victimKey)
	}
}
```

All three call sites (`decodeCurrent`, `decodeForecast`, `decodeHistorical`) update
the helper name from `evictExpiredLocked` to `evictOneLocked`. No other change at the
call sites is needed.

## Test Plan

### Updated tests

- `TestCacheOverflow_AllValid` (weather_test.go:222) — new assertion: when cache is
  full of valid entries, eviction removes one entry and the new entry IS inserted;
  cache size after insertion is exactly `maxCacheEntries`.
- `TestEvictExpiredLocked` (weather_test.go:126) — renamed assertion: still verifies
  expired-first preference but calls `evictOneLocked` (only one entry evicted per call).
  Test logic adapts so that with 5 expired and 5 unexpired entries, exactly one expired
  entry is removed per call.

### New tests

- `TestEviction_HistoricalDoesNotStarveEphemeral` — populates cache with
  `maxCacheEntries` historical entries (100-year TTL), inserts one current entry, asserts
  the current key is present in the cache and that exactly one historical entry was
  evicted. **Adversarial-fidelity test:** if the production fix is reverted, this test
  fails with a concrete diagnostic.
- `TestEviction_LongestExpiryEvictedWhenFull` — populates cache with mixed entries
  spanning a range of TTLs, calls `evictOneLocked` once, asserts the entry with the
  largest `expiresAt` is the one removed.

## Change Boundary

Files MAY change:
- `internal/connector/weather/weather.go` — `evictExpiredLocked` → `evictOneLocked`,
  fallback eviction logic added; call-site rename only.
- `internal/connector/weather/weather_test.go` — updated/added cache eviction tests.
- `specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache/**` — bug packet.
- `specs/016-weather-connector/state.json` — append BUG-016-W4 to `resolvedBugs[]`.
- `specs/016-weather-connector/uservalidation.md` — append BUG-016-W4 entry.
- `.specify/memory/sweep-2026-05-23-r30.json` — round 26 status update (NOT committed).

Files that MUST NOT change:
- Any file under `cmd/core/`, `internal/api/`, `internal/config/`, `internal/web/`,
  `internal/notification/`, `internal/pipeline/`, `config/`, `scripts/`, `smackerel.sh`,
  `specs/055-*/`, `specs/044-per-user-bearer-auth/state.json`.
- Any framework-managed file under `.github/bubbles/`, `.github/agents/bubbles_shared/`,
  `.github/agents/bubbles.*.agent.md`, `.github/prompts/bubbles.*.prompt.md`,
  `.github/instructions/bubbles-*.instructions.md`.
- Any other connector source.

## Adversarial Fidelity Proof Plan

1. Apply the production fix and updated tests.
2. Run `go test -race -count=1 ./internal/connector/weather/...` and confirm GREEN.
3. Mentally revert the fallback eviction branch (or comment it out in a scratch
   experiment).
4. Confirm that `TestEviction_HistoricalDoesNotStarveEphemeral` would fail with the
   message naming the missing current key.
5. Restore the fix.

The adversarial-fidelity reasoning is recorded in `report.md` rather than performing an
actual destructive flip-and-restore cycle, to keep git history clean.

## Scenario-First TDD Evidence

This packet uses scenario-first TDD per repo policy:

- SCN-BUG016W4-001 (historical-starves-current): test added first, observed RED against
  pre-fix code, then production fix applied, test observed GREEN.
- SCN-BUG016W4-002 (longest-expiry-evicted-when-full): test added first, observed RED
  against pre-fix code, then production fix applied, test observed GREEN.
- SCN-BUG016W4-003 (max-cap-preserved): assertion folded into both tests; cache size
  never exceeds `maxCacheEntries` after the inserts.
- SCN-BUG016W4-004 (existing-test-updated): the existing `TestCacheOverflow_AllValid`
  was updated to reflect the new policy; without this update the build would fail to
  compile-link the new behavior into the existing test surface.

red→green markers are recorded in `report.md` under `### Scenario-First TDD Evidence`.
