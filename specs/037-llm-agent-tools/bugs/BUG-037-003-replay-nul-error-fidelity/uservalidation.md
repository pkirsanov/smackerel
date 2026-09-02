# User Validation: [BUG-037-003] Replay NUL Error Fidelity

## Automation Readiness

- [ ] The NUL case proves `errors.Is(runErr, syscall.EINVAL)`.
- [ ] The NUL case proves the built binary exists and the command context remains active.
- [ ] Ordinary malformed trace IDs still execute the binary and return replay ERROR 2.

## Checklist

- [ ] Review the focused test output and confirm an unrelated launch failure cannot satisfy the NUL case.
- [ ] Review the companion output and confirm ordinary replay PASS, FAIL, and ERROR meanings remain distinct.

## Human Acceptance Record

No human acceptance is recorded for this in-progress packet.

## Goal

- Goal: trust that the NUL chaos result proves the exact invalid-argument boundary.
- Success signal: the subtest requires a present binary, active context, and EINVAL identity.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Review current NUL assertion | Any non-ExitError qualifies | `report.md#source-grounded-findings` | broken |
| 2 | Exclude missing binary and cancellation | No exact checks exist | `report.md#source-grounded-findings` | missing |
| 3 | Review repaired exact-cause proof | Not executed | Planned evidence anchors in `report.md` | missing |

## Open Refinements

- `bubbles.design` must approve the exact wrapped-error model.
- `bubbles.plan` must approve the scope and test-plan mapping.
- `bubbles.test` must produce red-green mutation and regression evidence.