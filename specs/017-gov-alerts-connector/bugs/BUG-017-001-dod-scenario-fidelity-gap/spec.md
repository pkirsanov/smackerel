# Bug: BUG-017-001 — DoD scenario fidelity gap (SCN-GA-* across all 6 scopes) + report.md evidence reference gap

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 017 — Government Alerts Connector
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed (artifact-only)

## Problem Statement

Bubbles `traceability-guard.sh` reported the following blocking failures for `specs/017-gov-alerts-connector` (parent feature already marked `done`):

1. **Test Plan absent on every scope.** The parent `scopes.md` had no `### Test Plan` table on any of the 6 scopes, so the guard's per-scope traceability pass produced no concrete test file references and the script aborted before reaching Gate G068 in earlier runs.
2. **G068 DoD-fidelity gap (latent).** Once Test Plan tables were present, Gate G068 (Gherkin → DoD Content Fidelity) needed each of the 13 `SCN-GA-*` Gherkin scenarios in scopes.md to be matched by a DoD bullet either by trace ID or by sufficient significant-word overlap. Existing DoD bullets described the implemented behavior but did not embed the `SCN-GA-NNN` ID that the guard's content-fidelity matcher uses as its first criterion.
3. **report.md evidence reference gap.** Scope 06's Test Plan resolves to live-stack tests at `tests/integration/weather_alerts_test.go` and `tests/e2e/weather_alerts_e2e_test.go`. The parent `report.md` referenced `internal/connector/alerts/alerts_test.go` extensively but never named the integration or e2e test files, so the guard's "report references concrete test evidence" check failed for `tests/e2e/weather_alerts_e2e_test.go`.

## Reproduction (Pre-fix)

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector 2>&1 | grep -E "RESULT|❌|FAIL|fidelity|G068"
❌ Scope 06: Proactive Delivery & Travel Alerts report is missing evidence reference for concrete test file: tests/e2e/weather_alerts_e2e_test.go
--- Gherkin → DoD Content Fidelity (Gate G068) ---
ℹ️  DoD fidelity: 13 scenarios checked, 13 mapped to DoD, 0 unmapped
ℹ️  DoD fidelity scenarios: 13 (mapped: 13, unmapped: 0)
RESULT: FAILED (1 failures, 0 warnings)
```

(That snapshot is the post-Test-Plan state. Before adding the Test Plan tables, the guard exited silently after the manifest cross-check because `extract_test_rows` returned an empty pipeline under `set -euo pipefail`.)

## Gap Analysis (per scenario)

For each Gherkin scenario in the parent `scopes.md`, the bug investigator searched the production code (`internal/connector/alerts/alerts.go`) and the test files (`internal/connector/alerts/alerts_test.go`, `tests/integration/weather_alerts_test.go`, `tests/e2e/weather_alerts_e2e_test.go`). All 13 behaviors are genuinely **delivered-but-undocumented at the trace-ID level** — there is no missing implementation and no missing test fixture; the only gaps are (a) DoD bullets did not embed the `SCN-GA-NNN` ID the guard uses for fidelity matching, (b) `scopes.md` had no per-scope Test Plan tables, and (c) `report.md` did not reference the live-stack test files.

| Scenario | Behavior delivered? | Tests pass? | Concrete test file(s) | Concrete source |
|---|---|---|---|---|
| SCN-GA-PROX-001 | Yes — Haversine great-circle distance + per-location nearest-match within radius | Yes — `TestHaversineKm`, `TestFindNearestLocation` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::haversineKm`, `findNearestLocation` |
| SCN-GA-LIFE-001 | Yes — `known` map records first-seen, dedupes unchanged alerts, evicts after `knownEvictionAge` | Yes — `TestKnownMapEviction`, `TestSync_Deduplication` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::Sync` (lifecycle path) |
| SCN-GA-USGS-001 | Yes — USGS GeoJSON parsed with `minmagnitude` query parameter | Yes — `TestSync_PassesMinMagnitudeToURL` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::fetchUSGSEarthquakes` |
| SCN-GA-USGS-002 | Yes — magnitude+distance severity classification + tier assignment | Yes — `TestClassifyEarthquakeSeverity`, `TestNormalizeEarthquake_TierAssignment` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::classifyEarthquakeSeverity` |
| SCN-GA-NWS-001 | Yes — NWS Alert API queried with point coordinates + User-Agent | Yes — `TestFetchNWSAlerts_ValidResponse`, `TestFetchNWSAlerts_UserAgentHeader` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::fetchNWSAlerts` |
| SCN-GA-NWS-002 | Yes — NWS severity → CAP standard; event types classified | Yes — `TestMapNWSSeverity`, `TestClassifyNWSEventType` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::mapNWSSeverity`, `classifyNWSEventType` |
| SCN-GA-CONN-001 | Yes — multi-source Sync iterates enabled sources, tolerates partial source failure | Yes — `TestSync_BothEarthquakeAndWeather`, `TestSync_PartialFailure_USGSDown_NWSUp` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::Sync` |
| SCN-GA-TSUN-001 | Yes — NOAA Atom feed parsed, georss:point proximity-filtered | Yes — `TestSync_TsunamiSource`, `TestNormalizeTsunamiAlert` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::fetchTsunamiAlerts` |
| SCN-GA-AQI-001 | Yes — AirNow JSON parsed; AQI 151–200 → "moderate" | Yes — `TestSync_AirNow_ProducesArtifacts`, `TestClassifyAQISeverity` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::fetchAirNowAQI`, `classifyAQISeverity` |
| SCN-GA-GDACS-001 | Yes — GDACS RSS parsed; Red-level → "extreme"; distant filtered out | Yes — `TestSync_GDACS_ProximityFiltered`, `TestClassifyGDACSAlertLevel` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::fetchGDACSAlerts` |
| SCN-GA-NOTIF-001 | Yes — extreme/severe alerts published to `alerts.notify` with full payload | Yes — `TestSync_ExtremeEarthquake_NotifiesAlert` PASS (live-stack proxies in `tests/integration/weather_alerts_test.go`, `tests/e2e/weather_alerts_e2e_test.go`) | `internal/connector/alerts/alerts_test.go`, `tests/integration/weather_alerts_test.go`, `tests/e2e/weather_alerts_e2e_test.go` | `internal/connector/alerts/alerts.go::maybeNotify` |
| SCN-GA-NOTIF-002 | Yes — moderate severity does NOT trigger notification | Yes — `TestMaybeNotify_Moderate_NoNotification` PASS | `internal/connector/alerts/alerts_test.go`, `tests/integration/weather_alerts_test.go` | `internal/connector/alerts/alerts.go::maybeNotify` |
| SCN-GA-TRAVEL-001 | Yes — travel locations get 2x effective radius | Yes — `TestTravelLocations_DoubleRadius`, `TestSync_TravelLocation_ExpandedRadius` PASS | `internal/connector/alerts/alerts_test.go` | `internal/connector/alerts/alerts.go::mergedLocations` |

**Disposition:** All 13 scenarios are **delivered-but-undocumented** — artifact-only fix.

## Acceptance Criteria

- [x] Parent `specs/017-gov-alerts-connector/scopes.md` has a `### Test Plan` table on every one of the 6 scopes with concrete test file paths
- [x] Parent `scopes.md` has a DoD bullet that explicitly contains `SCN-GA-PROX-001` with raw `go test` evidence and a concrete test file pointer; same for the other 12 SCN-GA-* IDs
- [x] Parent `specs/017-gov-alerts-connector/report.md` references `tests/integration/weather_alerts_test.go` and `tests/e2e/weather_alerts_e2e_test.go` by full relative path
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/017-gov-alerts-connector` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/017-gov-alerts-connector/bugs/BUG-017-001-dod-scenario-fidelity-gap` PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector` PASS
- [x] No production code changed (boundary)
