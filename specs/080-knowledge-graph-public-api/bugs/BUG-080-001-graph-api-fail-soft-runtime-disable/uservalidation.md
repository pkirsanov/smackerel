# User Validation: [BUG-080-001]

## Checklist

- [x] Packet fidelity baseline: the reported enabled-empty fail-soft/404 condition and required fail-loud authenticated-read outcome are recorded; this is not runtime acceptance.

## Goal

- Goal: open Wiki/Graph only when its authenticated data routes are truly ready.
- Success signal: invalid required config refuses before serving; valid config passes all read-only graph journeys.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Open shipped Wiki | Reported static pages exist | Interpreted input in `report.md` | works |
| 2 | Read graph data | Reported all graph families return 404 | Interpreted input in `report.md` | broken |
| 3 | Use fail-loud repaired capability | Not observed | None | missing |

## Open Refinements

- `bubbles.ux` must define ready, unavailable, auth, true-empty, partial, and error states without leaking previous graph labels.
