# User Validation Checklist

> **Bug:** [BUG-016-W3] Weather sync SourceRef collision and sync test panic
> **Parent Feature:** [specs/016-weather-connector](../../)

## Checklist

- [x] Baseline checklist initialized for BUG-016-W3.
- [x] Two successful weather syncs for the same location inside one second produce distinct current artifact `SourceRef` values.
- [x] Forecast artifact `SourceRef` generation uses the same strengthened per-sync uniqueness strategy as current artifacts.
- [x] `TestSync_SourceRefUniquePerSync` or its replacement fails if `SourceRef` only includes second-level `time.RFC3339` granularity.
- [x] The SourceRef regression test does not rely on sleeping across a one-second boundary.
- [x] `TestSync_HealthSetToSyncingDuringSync` cannot panic with `close of closed channel` when its HTTP handler is invoked more than once.
- [x] `TestSync_ConfigGenGuard_ConnectDuringSync` cannot panic with `close of closed channel` when its HTTP handler is invoked more than once.
- [x] Health-sync and config-generation guard tests still assert their original health-state behavior after the signal fix.
- [x] Regression tests contain no silent-pass bailout patterns.
- [x] `./smackerel.sh test unit` exits 0 with `internal/connector/weather` green.
- [x] Repo-standard integration, e2e, stress, check, lint, and format gates are run or honestly blocked with raw evidence.
- [x] Parent `specs/016-weather-connector/uservalidation.md` receives a checked bug-fix entry after validate-owned certification.

All entries are checked after BUG-016-W3 validate/audit certification evidence was recorded in `report.md` and the parent weather user-validation checklist received the bug-fix entry.