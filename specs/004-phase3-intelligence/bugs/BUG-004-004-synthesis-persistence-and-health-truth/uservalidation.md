# User Validation: [BUG-004-004]

## Corrective Plan Authorization - 2026-08-30

- [x] The operator authorizes execution of the corrected SCOPE-03A and SCOPE-04A plan recorded in [scopes.md](scopes.md), including SCOPE-05 regression revalidation after the causal read model is corrected.

This checked row records planning authorization only. It does not accept implementation, runtime behavior, test results, candidate recovery, deployment admission, or rollback.

### Corrective Runtime Acceptance - Not Yet Granted

- [ ] The immutable causal event ledger, audit-write failure propagation, changed-source replacement, and read-back recovery contracts are accepted after new SCOPE-03A evidence is reviewed in [report.md](report.md#corrective-scope-03a-evidence).
- [ ] Cadence-scoped health, startup reconciliation, fail-closed strict readiness, and verified recovery contracts are accepted after new SCOPE-04A evidence is reviewed in [report.md](report.md#corrective-scope-04a-evidence).
- [ ] The historical Today and Status behavior is accepted against the corrected model after SCOPE-05 regression revalidation evidence is reviewed in [report.md](report.md#corrective-scope-04a-evidence).

**Authorization source:** The operator's 2026-08-30 instruction to finish this planning reconciliation and set `nextRequiredOwner` to `bubbles.implement`.

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

Checked on the operator sign-off recorded below. The rows state what the
operator accepted; the Human Acceptance Record states the basis, so a reader can
weigh the acceptance rather than infer a hands-on review that did not happen.

- [x] Packet fidelity baseline: the reported zero-persistence and false never-run health findings plus the durable read requirement match the delivered behavior.
- [x] Durable synthesis survives a restart and reads back unchanged.
- [x] Today and Status agree on the synthesis state in a live browser.
- [x] Health reports never-run, quiet, stale, partial and failed truthfully rather than reporting a never-run capability as up.
- [x] A failed state offers a usable retry control.
- [x] No state discloses citation counts to an unauthenticated reader.

## Human Acceptance Record

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-27
- method: external-record
- record: Operator directive in the working session on 2026-08-27, verbatim "human gates approved, check all uservalidations, continue", issued after the agent reported that the packet was complete on every automated surface and awaited only the human acceptance gate.

The operator accepted on the executed evidence cited in `report.md` rather than
by driving each state in a browser. That distinction is recorded because it is
what a later reader needs: the acceptance is real and given by the human who
owns this repository, and its basis is the automation readiness above.

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
