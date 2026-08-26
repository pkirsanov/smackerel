# User Validation: [BUG-004-004]

## Automation Readiness

Automation verified the delivered behavior far enough to be worth a human's
time. These rows grant no acceptance whatsoever; they exist so the reviewer
knows the surface is worth exercising.

- [x] Synthesis output persists across a real container restart and reads back with the same logical key, insight count and citation count. Evidence: the restart durability shell test.
- [x] Today and Status render exactly one state from one shared projection, so the two surfaces cannot disagree. Evidence: `report.md#phase-simplify`.
- [x] The seven durable states are each seeded and asserted on both surfaces. Evidence: the state matrix shell test.
- [x] An unauthenticated caller is refused by the read and retry routes, and an unauthenticated render cannot satisfy a content assertion vacuously. Evidence: `report.md#phase-security`.
- [x] Unwiring the durable reader makes four of five browser specs fail for the right reason, so the suite is not vacuous. Evidence: the mutation records in `report.md`.

## Checklist

Ships unchecked. Each row is checked by the human who accepted that behavior.
Automation left every row below untouched on purpose: an agent was the only
party that exercised this surface, and the registry is explicit that when that
is true, acceptance has not happened yet.

- [ ] Packet fidelity baseline: the reported zero-persistence and false never-run health findings plus the durable read requirement match the delivered behavior.
- [ ] Durable synthesis survives a restart and reads back unchanged.
- [ ] Today and Status agree on the synthesis state in a live browser.
- [ ] Health reports never-run, quiet, stale, partial and failed truthfully rather than reporting a never-run capability as up.
- [ ] A failed state offers a usable retry control.
- [ ] No state discloses citation counts to an unauthenticated reader.

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
