# Specification: BUG-076-002 Annotation source header E2E

## Expected Behavior

The live annotation shadow-comparator test MUST obtain the required header name from generated SST config, send source `api`, receive HTTP 201, and prove the `channel="api"` shadow counter advances. Missing header config MUST fail loudly.

### Single-Capability Justification

- **Classification:** This is a test-prerequisite repair for an existing annotation-provenance capability. It introduces no new annotation source, telemetry channel, shadow comparator, connector, or second write path — the `channel`, `connector`, and comparator terms in the evidence are pre-existing product surfaces referenced as proof, not new foundations.
- **Existing foundation and reuse path:** The annotation write already flows through `internal/api/annotation_source.go` (`AnnotationSourceHeader = "X-Smackerel-Source"`, which requires the header and validates its value against the source allowlist) and the SCOPE-4b dual-write shadow comparator (`smackerel_annotation_classifier_shadow_calls_total`). The E2E reuses that live path; the fix only supplies the required provenance header, read from the generated SST env `ANNOTATIONS_SOURCE_HEADER_NAME`.
- **Consumer set:** The authenticated annotation API, the shadow-comparator E2E, and the `channel="api"` metric family consume the same pre-existing header contract; no consumer is added.
- **Why no new abstraction or provider registry is needed:** The header contract and the comparator already define the reusable boundary. The defect is a missing request header in one consumer test, so another source channel, sink registry, or metric abstraction would not remove it.

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
