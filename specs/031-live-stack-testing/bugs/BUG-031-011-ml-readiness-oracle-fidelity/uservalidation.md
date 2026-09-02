# User Validation: [BUG-031-011] ML Readiness Oracle Fidelity

## Automation Readiness

- [ ] The focused test proves HTTP 503 cannot produce `ready=true`.
- [ ] The healthy control proves HTTP 200 can produce `ready=true` through the same path.
- [ ] Current endpoint text distinguishes ML `/health` from core `/readyz`.

## Checklist

- [ ] Review the focused test output and confirm the non-ready dependency is never reported ready.
- [ ] Review the route wording and confirm core `/readyz` is not presented as the ML dependency endpoint.

## Human Acceptance Record

No human acceptance is recorded for this in-progress packet.

## Goal

- Goal: trust that the ML readiness regression fails when a dependency is not ready.
- Success signal: one test discriminates HTTP 200 from HTTP 503 through the production readiness loop.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Review current regression claim | Test supplies HTTP 200 and expects true | `report.md#source-grounded-findings` | broken |
| 2 | Confirm actual endpoint identity | ML probe uses `/health`, core uses `/readyz` | `report.md#source-grounded-findings` | unclear |
| 3 | Review repaired adversarial proof | Not executed | Planned evidence anchors in `report.md` | missing |

## Open Refinements

- `bubbles.design` must approve the route and mutation model.
- `bubbles.plan` must approve the scope and test-plan mapping.
- `bubbles.test` must produce red-green mutation and regression evidence.