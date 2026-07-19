# Bug Fix Design: BUG-076-002

## Root Cause Analysis

The API requires the configured annotation source header and validates its value against the source allowlist. The test asserts the `api` metric channel but never sends the corresponding request metadata.

## Fix Design

Read `ANNOTATIONS_SOURCE_HEADER_NAME` from the generated E2E environment, fail if empty, and set that header to `api` on the annotation request. Preserve the existing missing-header adversary in the owning API E2E suite.

## Complexity Tracking

None - simplest viable fix used.
