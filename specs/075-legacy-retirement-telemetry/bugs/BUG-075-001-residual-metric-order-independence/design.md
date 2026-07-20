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

### Single-Implementation Justification

- **Existing owning abstraction:** `internal/assistant/legacyretirement.ResidualTelemetry` is the established recording contract. `policyImpl.recordResidual` derives the privacy-safe bucket and calls that contract, while `MultiResidualTelemetry` already provides bounded sink fan-out.
- **Concrete implementations:** `PrometheusResidualTelemetry` emits `smackerel_legacy_command_residual_total` and normalizes identity to a 64-character HMAC or `anonymous`; the existing residual store participates through the same fan-out for the rolling report. This bug changes neither implementation.
- **Current consumers:** The authenticated retired-command assistant flow, the residual privacy E2E, the live `/metrics` scrape, and the rolling seven-day retirement report consume the same observation path and labels.
- **Bounded variation axes:** Identity varies only between HMAC-shaped and canonical anonymous buckets, while retirement outcomes use the existing closed vocabulary. Sink composition is already bounded by the `ResidualTelemetry` interface and `MultiResidualTelemetry`.
- **Extension path:** Another telemetry sink implements `ResidualTelemetry` and is composed through the existing fan-out after preserving the same privacy contract. Another test scenario must create its own real retired-command observation before asserting a sample.
- **Foundation decision:** The reusable interface and fan-out already exist. The failure is one consumer test omitting its production precondition, so a new metric registry, sink abstraction, or test-only series initializer would add duplication and could conceal the ordering defect.

## Complexity Tracking

None - simplest viable fix used.
