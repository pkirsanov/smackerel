# Bug: BUG-076-002 Annotation shadow E2E omits source header

## Summary

The assistant annotation shadow-comparator E2E posts an annotation without the required SST-named source header, so the live API correctly rejects it with HTTP 400 before the comparator executes.

## Severity

- [ ] Critical
- [ ] High
- [x] Medium
- [ ] Low

## Status

- [x] Confirmed (reproduced)
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Run the assistant E2E package on a clean disposable stack.
2. Observe `TestAnnotationClassifierWithShadowComparator` POST an annotation.
3. Observe HTTP 400 `X-Smackerel-Source header required`.

## Expected Behavior

The test reads the canonical header name from generated test config, sends the explicit `api` source required by its metric assertion, and reaches the live shadow comparator.

## Actual Behavior

The request omits the required provenance header and never reaches classifier logic.

## Environment

- Version: `7ca186217c007a24075b2273275a22434d89fc44`
- Stack: repository-managed disposable E2E

## Error Output

```text
=== RUN   TestAnnotationClassifierWithShadowComparator
    annotation_classifier_e2e_test.go:82: POST annotation status = 400, body={"error":"X-Smackerel-Source header required"}
--- FAIL: TestAnnotationClassifierWithShadowComparator (39.58s)
```

## Root Cause

The API source contract was tightened under spec 027, but this later spec-076 E2E fixture did not consume the generated `ANNOTATIONS_SOURCE_HEADER_NAME` prerequisite.

## Related

- Feature: `specs/076-assistant-completion-rescope/`
