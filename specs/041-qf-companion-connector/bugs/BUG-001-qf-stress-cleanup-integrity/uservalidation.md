# User Validation Checklist: [BUG-001] QF Stress Cleanup Integrity

## Automation Readiness

- [ ] The parent/child live stress regression observes zero owned artifacts, annotations, edges, and sync-state rows after child cleanup.
- [ ] The adversarial closed-pool regression proves cleanup errors fail loud.
- [ ] Existing QF E2E and broader stress regressions pass against disposable state.

## Checklist

- [ ] QF stress runs leave no rows owned by their unique source IDs after teardown.
- [ ] Cleanup failure is visible as a failed test and cannot be reduced to an informational log.
- [ ] Freshness results reflect current-run event/packet time rather than `2026-05-06T00:00:00Z`.
- [ ] An unrelated control artifact survives the source-scoped cleanup.

## Human Acceptance Record

No human acceptance has been recorded. The bug remains `in_progress`, and automation readiness is not established.

## Goal

- Goal: Keep QF companion stress evidence isolated, current, and failure-visible.
- Success signal: A human can review the executed parent/child evidence and see zero owned residue, an intact control row, a forced cleanup error, and current-run freshness measurements.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Review the stress cleanup result | Not run | No runtime evidence yet | missing |
| 2 | Confirm an injected cleanup failure is visible | Not run | No runtime evidence yet | missing |
| 3 | Confirm freshness uses current-run time | Not run | No runtime evidence yet | missing |

## Open Refinements

- None recorded during artifact-only packet creation.
