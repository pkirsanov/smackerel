# User Validation: [BUG-004-004]

## Checklist

- [x] Packet fidelity baseline: the reported zero-persistence and false never-run health findings plus the durable/readable outcome are recorded; this is not runtime acceptance.

## Goal

- Goal: receive durable source-linked synthesis and truthful capability health.
- Success signal: persisted output reads back; never-run/stale/failed states and alerts reflect actual runs.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Run synthesis | Reported in-memory structs/count log | Interpreted input in `report.md` | unclear |
| 2 | Read persisted output | Reported zero rows | Interpreted input in `report.md` | broken |
| 3 | Trust health | Reported never-run as up | Interpreted input in `report.md` | broken |

## Open Refinements

- `bubbles.ux` must define current, quiet, never-run, stale, partial, failed, source, and recovery presentation.
