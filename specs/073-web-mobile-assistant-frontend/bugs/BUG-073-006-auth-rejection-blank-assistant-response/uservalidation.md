# User Validation: [BUG-073-006]

## Checklist

- [x] Packet fidelity baseline: the reported pre-facade blank-response failure and required visible retryable outcome are recorded; this is not runtime acceptance.

## Goal

- Goal: every submitted Assistant message receives an honest accessible outcome.
- Success signal: no blank response; failures preserve transcript/input and offer retry or re-authentication.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Submit message | Reported user message remains | Interpreted input in `report.md` | works |
| 2 | Understand rejection | Reported blank Assistant response | Interpreted input in `report.md` | broken |
| 3 | Retry safely | Not observed | None | missing |

## Open Refinements

- `bubbles.ux` must define pending, answer, refusal, auth, timeout/network, server/schema error, retry, focus, and mobile composition.
