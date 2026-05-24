# User Validation: [BUG-016-W4] Historical entries with 100-year TTL starve weather connector cache

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [scenario-manifest.json](scenario-manifest.json)

## Checklist

- [x] **AC-01:** A unit test populates the cache with `maxCacheEntries` historical entries (100-year TTL) and asserts the subsequent current entry is present after the call returns. Implemented as `TestEviction_HistoricalDoesNotStarveEphemeral`; PASS evidence in `report.md::Test-Owned Evidence Closure`.
- [x] **AC-02:** A unit test asserts that when the cache is full of valid mixed-TTL entries, eviction targets the entry with the latest `expiresAt`. Implemented as `TestEviction_LongestExpiryEvictedWhenFull`; PASS evidence in `report.md::Test-Owned Evidence Closure`.
- [x] **AC-03:** The existing `TestCacheOverflow_AllValid` test is updated to assert the new policy (overflow entry inserted, longest-expiry entry evicted, hard cap preserved). PASS evidence in `report.md::Test-Owned Evidence Closure`.
- [x] **AC-04:** Adversarial fidelity: reverting the production fix causes `TestEviction_HistoricalDoesNotStarveEphemeral` to fail with the diagnostic `current key "current-50.0000-10.0000" was not inserted after eviction; permanent historical entries starved the ephemeral cache slot (BUG-016-W4 regression)`. Verified by scratch revert-and-restore cycle recorded in `report.md::Adversarial Fidelity Proof`.
- [x] **AC-05:** `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, and `./smackerel.sh test unit --go` all pass with exit code 0. Evidence recorded in `report.md::Validation Evidence` and `report.md::Test-Owned Evidence Closure`.
- [x] **AC-06:** `state-transition-guard.sh` against the BUG packet emits `TRANSITION PERMITTED` with zero BLOCK findings. Evidence in `report.md::Audit Evidence`.
- [x] **AC-07:** `artifact-lint.sh` against the BUG packet emits `PASSED`. Evidence in `report.md::Audit Evidence`.
- [x] **AC-08:** `traceability-guard.sh` against the BUG packet emits 100% DoD-Gherkin fidelity. Evidence in `report.md::Audit Evidence`.

## User-Visible Behavior Confirmation

A weather connector deployed on a long-running host with the Maps connector
enriching past trips will retain its cross-`Sync()` cache benefit for current and
forecast data. The previous silent-degradation failure mode (every `Sync()`
re-fetching every location from Open-Meteo after ~1024 historical lookups
accumulate) is eliminated. The hard `maxCacheEntries=1024` cap is preserved.

No user-visible artifact contract changed. The fix is entirely internal to the
weather connector's cache layer.
