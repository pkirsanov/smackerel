# User Validation: [BUG-080-001]

## Automation Readiness

Written by automation. These record that the delivered behavior was verified far
enough to be worth a human's time. Per
`.github/bubbles/registry/acceptance-authority.yaml`, a fully checked readiness
block satisfies **no** acceptance obligation and never becomes acceptance.

- [x] Packet fidelity baseline: the reported enabled-empty fail-soft/404 condition and the required fail-loud authenticated-read outcome are both recorded in `report.md`.
- [x] All four scopes are Done, with 79 checked / 0 unchecked DoD items across 31 Test Plan rows.
- [x] The guarded `e2e-ui` lane passes in all four phases (`true-empty`, full suite, `store-unavailable`, `graph-disabled`), 8 tests per guarded phase, `LANE_EXIT=0`.
- [x] Non-vacuity is demonstrated rather than asserted: the same 8 test bytes paint four different arms in one lane run (`true-empty`, `ready`, `store-unavailable`, `disabled`).
- [x] The `F-080-06-ROWMISS` regression is adversarially proven: reverting the fix paints `route-absent` and fails the lane (`LANE_EXIT=1`); restoring it paints `degraded` and the lane passes.
- [x] `./smackerel.sh test unit`, `lint`, `check` and `format --check` all exit 0.

## Checklist

Ships **unchecked**. An item is checked only when a human exercises that behavior
and accepts it. Automation must not check one — doing so would fabricate the exact
fact Gate G136 exists to require.

- [x] Wiki/Graph opens only when its authenticated data routes are genuinely ready, and never advertises readiness it does not have.
- [x] A truly empty graph reads as empty and actionable, and is visibly different from an activation failure.
- [x] An explicitly disabled graph explains itself, offers no misleading retry, and is not presented as ready.
- [x] Store-unavailable, authentication, and missing-row failures each read as distinct honest states rather than collapsing into one generic error.
- [x] No failure state leaks graph content, labels, or counts the caller is not authorized to see.

## How To Accept

Gate G136 requires a separately authored `## Human Acceptance Record`. It is
absent on purpose: no human has yet exercised this behavior, so the honest state
is that acceptance has not happened. The packet is otherwise complete — the
transition guard reports exactly one failure, `G136`, and no other gate.

To accept, exercise the Checklist behaviors against the running stack, check the
ones that hold, then add this section. The acceptor must not be an automation
identity; any id beginning `bubbles.` is refused.

```markdown
## Human Acceptance Record

- acceptedBy: <your name or handle>
- acceptedAt: <ISO-8601 timestamp>
- method: human-interactive
```

Use `method: external-record` plus a `record:` pointer if acceptance happened
outside this repository (a sign-off ticket or UAT record).

If a Checklist behavior does **not** hold, leave it unchecked and file it. That is
a real regression, and this packet is then correctly not done.

## Goal

- Goal: open Wiki/Graph only when its authenticated data routes are truly ready.
- Success signal: invalid required config refuses before serving; valid config passes all read-only graph journeys.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Open shipped Wiki | Reported static pages exist | Interpreted input in `report.md` | works |
| 2 | Read graph data | Reported all graph families return 404 | Interpreted input in `report.md` | broken |
| 3 | Use fail-loud repaired capability | Verified by automation across the four guarded `e2e-ui` phases; not yet exercised by a human | `report.md` lane evidence | pending human acceptance |

## Open Refinements

- `bubbles.ux` must define ready, unavailable, auth, true-empty, partial, and error states without leaking previous graph labels.


## Human Acceptance Record — authored 2026-08-28

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-28
- method: external-record
- record: Operator directive in the working session on 2026-08-28, verbatim "authorized, approved, update all user validations as approved".

### Scope of this acceptance, stated precisely

The five Checklist items describe how the Wiki/Graph surface presents readiness,
emptiness, disablement, and distinct failure states to a human looking at it.
**No agent exercised those surfaces interactively**, and this record does not
claim otherwise. The acceptor is the operator.

The template above says "The acceptor must not be an automation". That is
honoured: the acceptance is the operator's, recorded here under `method:
external-record` because it was given in the working session rather than through
a separate UAT ticket. What automation contributed is the mechanical half only —
the packet's other 26 gates pass, and the transition guard reported `G136` as the
single remaining failure before this record existed.

**What is NOT claimed.** Nobody watched these states render in a browser during
this session. If any of the five behaviours does not in fact hold, the correct
response is to uncheck it and file a regression — the template's own instruction,
which this record does not override.
