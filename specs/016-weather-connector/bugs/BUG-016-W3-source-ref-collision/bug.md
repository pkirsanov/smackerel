# Bug: [BUG-016-W3] Same-second weather SourceRef collision and unsafe sync test signal

> **Parent Feature:** [specs/016-weather-connector](../../)
> **Discovered From:** [specs/039-recommendations-engine](../../../039-recommendations-engine/) feature-level regression phase, 2026-05-03T22:05:34Z
> **Date Opened:** 2026-05-04
> **Status:** Fixed and verified - bugfix-fastlane certification recorded
> **Severity:** High
> **Workflow Mode:** bugfix-fastlane

---

## Summary

The 039 feature-level regression phase surfaced a red unit regression in the weather connector ownership surface. `internal/connector/weather/weather_test.go::TestSync_SourceRefUniquePerSync` produced identical `SourceRef` values for two syncs of the same location within the same second, for example `current-City-2026-05-03T21:16:37Z`. The current weather connector builds current and forecast `SourceRef` values with `now.Format(time.RFC3339)`, which preserves only second-level precision. That is still insufficient for rapid consecutive syncs and can cause downstream pipeline deduplication to collapse distinct weather observations.

The same unit run also emitted `panic: close of closed channel` from weather test HTTP handlers in `TestSync_HealthSetToSyncingDuringSync` and `TestSync_ConfigGenGuard_ConnectDuringSync`. Those handlers close a coordination channel on every request, but `Sync()` can issue multiple HTTP requests or retry requests against the same handler. The tests therefore contain a non-idempotent signal path that can panic and obscure the production regression.

## Finding IDs

| ID | Classification | Owner Surface | Evidence Source |
|----|----------------|---------------|-----------------|
| BUG-016-W3-F1 | Failing unit regression / production dedup risk | `internal/connector/weather/weather.go` and `internal/connector/weather/weather_test.go` | [specs/039-recommendations-engine/report.md](../../../039-recommendations-engine/report.md) feature-level regression evidence |
| BUG-016-W3-F2 | Test harness instability / panic under repeated handler invocation | `internal/connector/weather/weather_test.go` | [specs/039-recommendations-engine/report.md](../../../039-recommendations-engine/report.md) feature-level regression evidence |
| IMP-016-R4-001-REGRESSION | Regression of earlier date-only SourceRef hardening: RFC3339 seconds remain too coarse | `specs/016-weather-connector/report.md` R4 finding | [specs/016-weather-connector/report.md](../../report.md) Improve Pass R4 |

## Ownership Classification

This is owned by `specs/016-weather-connector`, not `specs/039-recommendations-engine`. The red evidence appeared during the broad 039 regression phase, but both failing symbols and both panic frames are in `internal/connector/weather`. The earlier related finding `IMP-016-R4-001` is also recorded in the weather connector report, and the current production code still formats `SourceRef` values with second-level `time.RFC3339` granularity.

## Reproduction Evidence

The authoritative executed evidence is already recorded in [specs/039-recommendations-engine/report.md](../../../039-recommendations-engine/report.md) under `Feature-Level Regression Phase - 2026-05-03T22:05:34Z`:

```text
$ ./smackerel.sh test unit
2026/05/03 21:16:37 INFO weather connector connected id=weather locations=1
2026/05/03 21:16:37 WARN weather forecast fetch failed location=City error="open-meteo forecast returned no daily data"
2026/05/03 21:16:37 INFO weather sync complete id=weather locations=1 artifacts=1 failures=0 duration=3.751132ms
2026/05/03 21:16:37 WARN weather forecast fetch failed location=City error="open-meteo forecast returned no daily data"
2026/05/03 21:16:37 INFO weather sync complete id=weather locations=1 artifacts=1 failures=0 duration=1.889045ms
--- FAIL: TestSync_SourceRefUniquePerSync (1.05s)
  weather_test.go:818: consecutive syncs produced identical SourceRef "current-City-2026-05-03T21:16:37Z" — would cause pipeline dedup collision
Exit Code: 1
```

**Claim Source:** interpreted from executed evidence recorded by `bubbles.regression`; this planning invocation did not re-run runtime commands.

```text
$ ./smackerel.sh test unit
2026/05/03 21:16:37 http: panic serving 127.0.0.1:55360: close of closed channel
goroutine 293 [running]:
net/http.(*conn).serve.func1()
    /usr/local/go/src/net/http/server.go:1947 +0xbe
panic({0x8eb360?, 0xa50820?})
    /usr/local/go/src/runtime/panic.go:792 +0x132
github.com/smackerel/smackerel/internal/connector/weather.TestSync_HealthSetToSyncingDuringSync.func1({0xa571c8, 0xc000001180}, 0x0?)
    /workspace/internal/connector/weather/weather_test.go:1046 +0x2e
Exit Code: 1
```

**Claim Source:** interpreted from executed evidence recorded by `bubbles.regression`; this planning invocation did not re-run runtime commands.

## Expected Behavior

- Distinct weather `Sync()` calls for the same location must produce distinct `SourceRef` values, even when they start in the same wall-clock second.
- The `SourceRef` uniqueness contract must hold for current and forecast artifacts without relying on `time.Sleep(time.Second)` in tests.
- Weather unit tests that coordinate with `httptest.Server` handlers must tolerate multiple handler invocations without `close of closed channel` panics.
- Regression tests must include adversarial same-second sync and repeated-handler-invocation cases.

## Actual Behavior

- Two syncs can produce the same `SourceRef` when the string only includes `time.RFC3339` second-level precision.
- The existing adversarial test can still observe duplicate `SourceRef` values and fail under the broad unit suite.
- Two weather test handlers close the same channel from the handler body without guarding repeated calls, so a second request to the same handler panics.

## Impact

Pipeline deduplication uses `SourceRef` as part of artifact identity. Duplicate same-second `SourceRef` values can cause a real later weather observation to be treated as a duplicate of an earlier one, leaving stale weather data in the knowledge graph. The channel double-close panic is a test stability bug, but it matters because it can mask or interrupt the regression signal for the production `SourceRef` defect.

## Certification Status

BUG-016-W3 is fixed and verified in the bugfix-fastlane lane. The implementation and test changes are limited to the weather connector SourceRef construction, weather connector sync-signal tests, and this bug packet's evidence/provenance artifacts. Shared stress readiness remains routed to `specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness` and is not absorbed into this bug.