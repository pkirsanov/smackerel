# Specification: BUG-075-001 Residual metric order independence

## Expected Behavior

The residual telemetry privacy E2E MUST generate one real retired-command observation through the live authenticated assistant route before scraping. It MUST then require the actual metric family, an actual sample, the exact privacy-safe labels, and an HMAC-shaped or canonical anonymous bucket. It MUST pass in isolation and in package order.

## Acceptance Criteria

1. The test sends a real retired-command assistant turn with a unique message identifier.
2. The response succeeds before metrics are evaluated.
3. The scrape uses the live canonical core endpoint.
4. HELP, TYPE, and at least one residual sample are required.
5. Raw identity labels and non-HMAC user buckets remain forbidden.
6. Removing the precondition turn or residual telemetry wiring makes the test fail.
7. No fake sample, registry mutation from test code, request interception, or sleep-based ordering is introduced.

### Single-Capability Justification

- **Classification:** This is an order-independence repair for an existing residual telemetry capability. It adds no metric provider, telemetry sink, or second observation path.
- **Existing foundation and reuse path:** A real retired-command assistant turn already reaches `internal/assistant/legacyretirement.Policy`, which calls the configured residual telemetry `Record` path; `PrometheusResidualTelemetry` then emits `smackerel_legacy_command_residual_total`. The privacy E2E reuses that live path before scraping the canonical core metrics endpoint.
- **Consumer set:** The authenticated assistant retirement flow, the residual privacy E2E, and the rolling seven-day retirement report consume the same metric family and privacy-safe command, bucket, and outcome labels.
- **Why no new abstraction or provider registry is needed:** The existing residual telemetry interface and Prometheus implementation already define the reusable boundary. The defect is a missing real precondition in one consumer test, so another sink registry or metric abstraction would not remove the ordering error.

## Release Train

This bug targets the `mvp` train and introduces no feature flag.

## Test Isolation

The live turn and metrics scrape use the disposable test stack. The unique transport message identifier prevents collision, and stack teardown discards all state.

## Deployment Boundary

No runtime deployment, host, adapter, manifest, release-train, or secret surface changes.
