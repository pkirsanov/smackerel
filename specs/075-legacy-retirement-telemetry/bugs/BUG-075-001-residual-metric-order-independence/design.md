# Bug Fix Design: BUG-075-001

## Root Cause Analysis

### Investigation Summary

The failing privacy test scrapes before any call to `PrometheusResidualTelemetry.Record`. The same seven-test run later executes a retired-command notice request, after which `TestLegacyRetirementReport_E2E_RollingSevenDay` sees the metric and passes. Production registration and increment wiring are present; test setup is incomplete.

### Root Cause

The live privacy assertion depends on a metric child created by another test. Its stated zero-sample allowance is incompatible with Prometheus vector exposition behavior.

### Impact Analysis

- Affected component: `tests/e2e/assistant/legacy_privacy_e2e_test.go`
- Affected behavior: one privacy telemetry regression
- Production defect: none observed in registration or increment wiring

## Fix Design

### Solution Approach

Use the existing live assistant turn helper to invoke a retired command with unique identity, assert the request succeeded, then scrape and require a concrete residual sample. Keep the exact label and bucket-shape assertions. Do not register or increment metrics directly from the test.

### Alternative Approaches Considered

1. Pre-initialize the vector with empty labels - rejected because it creates a semantically fake series.
2. Accept a missing family when no samples exist - rejected because the test would not prove production wiring.
3. Add a delay before scraping - rejected because time cannot create an observation.

## Complexity Tracking

None - simplest viable fix used.
