# User Validation: [BUG-002-007]

## Checklist

- [x] Packet fidelity baseline: the reported current-row false-empty failure and required typed/read-state outcome are recorded; this is not runtime acceptance.

## Goal

- Goal: open Digest and see the latest authorized stored content or an honest recoverable state.
- Success signal: current content/date render; only a successful zero-row read shows never-generated copy.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Open current Digest | Reported false no-digest state despite stored row | Interpreted input in `report.md` | broken |
| 2 | Distinguish quiet/stale/error/empty | Not observed | None | missing |
| 3 | Use repaired Digest | Not observed | None | missing |

## Open Refinements

- `bubbles.ux` must specify current, quiet, stale, true-empty, auth, error, retry, source-link, mobile, and assistive states.
