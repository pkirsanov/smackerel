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

## Release Train

This bug targets the `mvp` train and introduces no feature flag.

## Test Isolation

The live turn and metrics scrape use the disposable test stack. The unique transport message identifier prevents collision, and stack teardown discards all state.

## Deployment Boundary

No runtime deployment, host, adapter, manifest, release-train, or secret surface changes.
