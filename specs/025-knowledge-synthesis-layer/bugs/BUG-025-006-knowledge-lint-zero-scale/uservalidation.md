# User Validation Checklist: BUG-025-006

## Automation Readiness

- [ ] The disposable stress run proves exactly 1000 owned artifacts before production lint starts.
- [ ] The production lint report and both five-minute timing assertions pass.
- [ ] Cleanup proves zero owned artifact and lint-report residue.
- [ ] The zero and off-by-one adversarial regression passes.

## Checklist

- [ ] The 1000-artifact lint stress test fails if its seed is removed or its count drifts.
- [ ] The stress test uses only disposable PostgreSQL and leaves no test-owned rows.
- [ ] Missing dependencies and endpoint failures fail instead of skipping.

Unchecked items indicate that human acceptance has not occurred or that a user reported a regression.

## Human Acceptance Record

No human acceptance record exists. This packet contains no delivered fix.

## Goal

- Goal: make the knowledge lint scale claim measurable and resistant to zero-work false passes.
- Success signal: the stress output proves 1000 unique owned rows, a fresh production lint result within budget, and zero owned residue.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|------|-------------|----------|----------|----------|
| 1 | Trust the 1000-artifact lint budget | Source inspection found no seed or cardinality assertion | `report.md#source-inspection-evidence` | broken |
| 2 | Preserve honest stress failures | Dirty changes replace skip paths with direct failures | `design.md#fail-loud-preservation` | works by inspection only |

## Open Refinements

None recorded. The packet defines the implementation and evidence boundary.