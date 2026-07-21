# Bug Fix Design: BUG-076-002

## Root Cause Analysis

The API requires the configured annotation source header and validates its value against the source allowlist. The test asserts the `api` metric channel but never sends the corresponding request metadata.

## Fix Design

Read `ANNOTATIONS_SOURCE_HEADER_NAME` from the generated E2E environment, fail if empty, and set that header to `api` on the annotation request. Preserve the existing missing-header adversary in the owning API E2E suite.

### Single-Implementation Justification

- **Existing owning abstraction:** `internal/api/annotation_source.go` (`AnnotationSourceHeader = "X-Smackerel-Source"`) is the established annotation-provenance contract; the SCOPE-4b dual-write shadow comparator is the established shadow path. This bug adds neither.
- **Concrete implementations:** the API source-header validator (HTTP 400 on a missing or unknown source) and the shadow comparator emitting `smackerel_annotation_classifier_shadow_calls_total{channel="api"}`. This bug changes neither implementation — it supplies the header from generated SST so an existing consumer test reaches them.
- **Current consumers:** the authenticated annotation flow, the shadow-comparator E2E, and the live `/metrics` scrape consume the same header contract and metric family.
- **Bounded variation axes:** the annotation source varies only across the existing closed source allowlist (e.g. `api`, `web`), and the header name is a single generated SST value (`ANNOTATIONS_SOURCE_HEADER_NAME`); the fix introduces no new axis.
- **Extension path:** another source value is added to the existing allowlist behind the same header contract; another test scenario reads the same generated header name and sends its explicit source.

## Complexity Tracking

None - simplest viable fix used.
