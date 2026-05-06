# Bug Fix Design: [BUG-016-W3] Weather SourceRef collision and unsafe sync test signal

> **Owner Surface:** `specs/016-weather-connector`
> **Status:** Root cause and selected design reconciled with implementation/test evidence

---

## Design Brief

### Current State

Pre-fix weather sync built current and forecast `SourceRef` values from `time.RFC3339`, which has only second-level precision. Rapid same-location syncs could therefore emit identical refs such as `current-City-2026-05-03T21:16:37Z`, and two weather tests could panic because repeated HTTP handler calls directly closed the same `syncStarted` channel.

The implementation and test phases have now repaired the bug in `internal/connector/weather/weather.go` and `internal/connector/weather/weather_test.go`. This design records the accepted root cause and the selected, implemented design so downstream validation sees one current truth.

### Target State

Weather `SourceRef` construction remains deterministic, source-qualified, and unique for rapid same-location syncs below one second. Weather sync tests tolerate repeated handler invocations while still asserting the original health-state behavior.

### Patterns To Follow

- Keep the fix local to the weather connector implementation and weather connector unit tests.
- Preserve existing weather artifact source ID, content types, metadata shape, titles, raw content, and cursor behavior.
- Use small helper functions for shared current/forecast `SourceRef` construction so the two artifact paths cannot drift.
- Use idempotent test synchronization around handler signals and explicit repeated-handler assertions.

### Patterns To Avoid

- Do not rely on `time.RFC3339` seconds alone for per-sync identity.
- Do not add `time.Sleep(time.Second)` as a uniqueness workaround.
- Do not append random data that weakens deterministic regression assertions.
- Do not directly close a shared sync signal from every HTTP handler invocation.

### Resolved Decisions

- Current and forecast `SourceRef` values use a stable helper that combines UTC `time.RFC3339Nano` with a connector-local monotonic sequence.
- The helper preserves `current-` / `forecast-` prefixes and the location name.
- Cursor return remains `now.Format(time.RFC3339)`; only artifact identity was strengthened.
- The affected sync tests use `sync.Once` via `notifySyncStarted` and assert repeated handler calls plus zero signal panics.

### Open Questions

No design-owned open questions remain for this bug packet. Remaining blockers belong to test, stress, validation, certification, or workflow phase-record ownership.

## Root Cause Analysis

### Investigation Summary

The red evidence came from a broad 039 regression phase, but the affected files and earlier related finding are all weather connector owned:

- Pre-fix `internal/connector/weather/weather.go` constructed current weather `SourceRef` values as `fmt.Sprintf("current-%s-%s", loc.Name, now.Format(time.RFC3339))`.
- Pre-fix `internal/connector/weather/weather.go` constructed forecast `SourceRef` values as `fmt.Sprintf("forecast-%s-%s", loc.Name, now.Format(time.RFC3339))`.
- `specs/016-weather-connector/report.md` already records `IMP-016-R4-001`, which fixed date-only granularity by moving to `time.RFC3339`.
- The new evidence shows `time.RFC3339` is still too coarse because it truncates sub-second time.
- Pre-fix `internal/connector/weather/weather_test.go` closed `syncStarted` directly inside the `httptest.Server` handlers in `TestSync_HealthSetToSyncingDuringSync` and `TestSync_ConfigGenGuard_ConnectDuringSync`; a second handler invocation closed the same channel again and panicked.

### Root Cause: BUG-016-W3-F1

The `SourceRef` uniqueness contract is tied to a string timestamp with only second-level precision. The connector takes `now := time.Now()` at the start of `Sync()`, then formats that value with `time.RFC3339`. For two successful syncs of the same location in the same second, the generated current artifact key is identical:

```text
current-City-2026-05-03T21:16:37Z
```

This is the same failure class as `IMP-016-R4-001`, but at a smaller time scale. The previous test expectation that `SourceRef` merely contains `T` is no longer strong enough; it proves sub-daily granularity, not per-sync uniqueness.

### Root Cause: BUG-016-W3-F2

Two weather tests coordinate a blocked sync with this pattern:

```go
syncStarted := make(chan struct{})
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    close(syncStarted)
    <-proceed
    ...
}))
```

That signal is not idempotent. Weather sync may request current conditions and forecast data, and retry behavior can add more handler invocations. If the handler runs twice, the second `close(syncStarted)` panics with `close of closed channel`. This is a test harness defect, but it is blocking because it pollutes the same unit run that revealed the production `SourceRef` issue.

## Impact Analysis

| Area | Impact |
|------|--------|
| Artifact pipeline | Duplicate same-second `SourceRef` values can collapse distinct weather observations during deduplication. |
| Knowledge freshness | Later weather observations may be discarded, leaving stale current conditions and forecasts. |
| Unit reliability | Weather unit tests can panic independently of the behavior they are trying to assert. |
| Feature ownership | The failure belongs to `specs/016-weather-connector`; 039 only surfaced it through broad regression. |

## Fix Design

### SourceRef strategy

The selected design is a stable helper pair in the weather connector:

```go
func (c *Connector) nextSourceRefSuffix(syncTime time.Time) string {
    sequence := c.syncSeq.Add(1)
    return fmt.Sprintf("%s-%d", syncTime.UTC().Format(time.RFC3339Nano), sequence)
}

func weatherSourceRef(artifactType, locationName, syncSuffix string) string {
    return fmt.Sprintf("%s-%s-%s", artifactType, locationName, syncSuffix)
}
```

`Sync()` computes one suffix for the sync and uses it for both current and forecast artifacts. The `time.RFC3339Nano` component fixes the lost sub-second precision, while the connector-local monotonic sequence protects uniqueness even if the system clock has coarse resolution or two sync starts share the same nanosecond-formatted timestamp. The artifact type and location remain visible in the prefix, so existing operators and tests can still identify `current-City-...` and `forecast-City-...` refs.

The implementation must preserve existing content types (`weather/current`, `weather/forecast`), source ID (`weather`), metadata shape, titles, raw content, and cursor behavior. The fix should be localized to weather connector SourceRef construction and its regression tests.

### Test synchronization strategy

The selected test design wraps the `syncStarted` channel close in `sync.Once` through `notifySyncStarted`. The affected tests count handler invocations with `handlerCalls`, recover and count any sync-start signal panic with `signalPanics`, then assert `handlerCalls >= 2` and `signalPanics == 0` after the blocked sync completes.

The tests must still assert their original behavioral contracts: health is `HealthSyncing` during the blocked sync, and the config-generation guard prevents a stale sync from clobbering the health set by `Connect()`.

## Regression Test Design

### Same-second SourceRef adversarial test

- **Scenario IDs:** SCN-BUG016W3-001, SCN-BUG016W3-002
- **Test target:** `internal/connector/weather/weather_test.go::TestSync_SourceRefUniquePerSync` or a replacement with the same intent.
- **Setup:** configure one weather location named `City`; the test server returns valid current and forecast payloads so both artifact types are covered.
- **Action:** execute two successful `Sync()` calls without sleeping past a second boundary. Clear connector cache between calls if needed to force the second request path.
- **Adversarial assertion:** fail if same-second current refs or forecast refs are equal; also assert the test would fail if the implementation used `now.Format(time.RFC3339)` only.
- **Anti-tautology rule:** do not assert only that the string contains `T`; that repeats the incomplete `IMP-016-R4-001` check.

### Repeated-handler adversarial test

- **Scenario IDs:** SCN-BUG016W3-003, SCN-BUG016W3-004
- **Test target:** `TestSync_HealthSetToSyncingDuringSync` and `TestSync_ConfigGenGuard_ConnectDuringSync`.
- **Setup:** force or permit at least two handler invocations against the same `syncStarted` signal.
- **Action:** run the same blocked-sync flow used by the existing tests.
- **Adversarial assertion:** the test must fail against an unsafe direct `close(syncStarted)` on every request by recording a recovered signal panic, and pass only when the signal is idempotent. The original health-state assertions must remain.

### Silent-pass scan

- **Scenario ID:** SCN-BUG016W3-005
- **Required scan:** run the repo regression quality guard or an equivalent grep through the Bubbles toolchain, then record raw output in `report.md`.
- **Forbidden:** `if duplicate { return }`, `if panicObserved { return }`, or any branch that exits before asserting the failure condition.

## Affected Files

| File | Implemented Behavior |
|------|-----------------|
| `internal/connector/weather/weather.go` | Current and forecast `SourceRef` construction uses `weatherSourceRef` plus `nextSourceRefSuffix`, combining UTC `time.RFC3339Nano` and a connector-local monotonic sequence. |
| `internal/connector/weather/weather_test.go` | Same-second SourceRef regression covers current and forecast artifacts; blocked-sync test handler signals are idempotent via `sync.Once`; repeated-handler assertions detect double-close regressions. |
| `specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/*` | Track bug evidence, scenarios, DoD, and validation handoff. |

## Alternatives Considered

| Option | Decision | Rationale |
|--------|----------|-----------|
| Add another `time.Sleep(time.Second)` to the test | Rejected | It avoids the same-second case instead of fixing the production collision. |
| Keep `time.RFC3339` and append random data | Possible but not preferred | Random IDs reduce reproducibility unless carefully controlled in tests. A high-resolution timestamp or monotonic sequence is easier to assert. |
| Remove `TestSync_SourceRefUniquePerSync` | Rejected | The test is protecting a real deduplication contract. It should be strengthened, not removed. |
| Ignore channel panics as test-only noise | Rejected | The panic destabilizes the same red unit run and can mask the production regression. |

## Resolved Implementation Decisions

The implementation and test owners resolved the design options as follows:

1. Use `time.RFC3339Nano` plus a connector-local monotonic sequence for SourceRef suffixes.
2. Use helper functions so current and forecast SourceRef construction share one suffix strategy.
3. Use `sync.Once` in the blocked-sync tests, with repeated-handler and zero-panic assertions.

No design-owned blocker remains for `DOD-BUG016W3-001` after this reconciliation.