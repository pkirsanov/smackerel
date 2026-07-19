# Specification: BUG-076-002 Annotation source header E2E

## Expected Behavior

The live annotation shadow-comparator test MUST obtain the required header name from generated SST config, send source `api`, receive HTTP 201, and prove the `channel="api"` shadow counter advances. Missing header config MUST fail loudly.

## Acceptance Criteria

1. No header name fallback or hardcoded alternate is added.
2. The request uses generated `ANNOTATIONS_SOURCE_HEADER_NAME`.
3. The live API and real comparator execute without interception.
4. Removing the header assignment makes the E2E fail with HTTP 400.

## Release Train

Target: `mvp`. No feature flag introduced.

## Test Isolation

The test uses the disposable E2E stack and a unique artifact marker.

## Deployment Boundary

No deployment, release-train, host, or secret changes.
