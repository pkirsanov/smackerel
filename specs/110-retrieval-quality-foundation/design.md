# Design: 110 Retrieval Quality Foundation

**Status:** `not_started` — **NO DESIGN DECISION HAS BEEN MADE YET.**
**Owner of this artifact:** `bubbles.design`
**Created by:** `bubbles.analyst` as an honest initial artifact under explicit operator instruction (recorded in `state.json.executionHistory` and `report.md`).

---

## What This File Is, And What It Is Not

This file exists so the feature packet is structurally complete and lintable. It records
**the inputs a designer needs, the constraints they inherit, and the questions they must
answer**. It does **not** contain a technical design, and nothing in it may be cited as
one.

Specifically, this file records **no** chosen schema, **no** chosen chunk size or overlap,
**no** chosen index parameters, **no** chosen fusion weighting, **no** chosen migration
mechanism, and **no** chosen lane wiring. Every one of those is an open decision listed in
§4.

A future run that fills this file in owns every claim it adds.

---

## 1. Inherited Constraints

The binding constraints are recorded in [`spec.md`](spec.md) §5 (`C110-1` … `C110-7`) and
must not be duplicated here in a form that could drift from them. They come from
[`docs/Product_Delivery_Plan.md`](../../docs/Product_Delivery_Plan.md) §3 P8 and §4 Stage 4.

A designer departing from any `C110-*` constraint must record the departure and its
justification **in this file**, referencing the constraint id.

## 2. Requirements This Design Must Satisfy

`R-110-01` … `R-110-18` in [`spec.md`](spec.md) §9, and `NFR-110-1` … `NFR-110-6` in §10.
Every requirement must be traceable to a design element once this file is authored.

## 3. Evidence The Design Starts From

[`spec.md`](spec.md) §3 records twelve verified current-state facts (`E1` … `E12`), each
with the command that produced it, and states its own evidence limit: no plan was
executed and no latency was measured. A design that assumes a query plan rather than
measuring one would inherit exactly the ambiguity this feature exists to remove.

## 4. Open Design Decisions (none resolved)

Each row is a decision this file must record once it is authored. None is decided.

| # | Decision | Depends on | Recorded finding |
|---|---|---|---|
| D-110-1 | Whether the current retrieval query uses the vector index or scans it, established by executing a plan against a realistic corpus. Everything about index choice and tuning follows from this. | executed measurement | `F-110-PLAN-01` |
| D-110-2 | Passage boundary rule: how content is divided, how much adjacent passages overlap, and how that satisfies `R-110-02` without unbounded storage growth (`NFR-110-5`). | D-110-1 | — |
| D-110-3 | Whether the item-level summary vector survives as a distinct fusion signal once passages exist, or becomes redundant. | D-110-2 | `F-110-SUMMARY-01` |
| D-110-4 | The fusion rule that turns per-kind signals into one comparable per-item score without ranking incomparable raw scores against each other (`R-110-09`, `R-110-10`, policy `P110-3`). | D-110-3 | — |
| D-110-5 | How semantic index identity is represented, stored per vector, and verified at startup across declaration, running model and stored column width (`R-110-05`, `R-110-06`, `R-110-07`). | — | — |
| D-110-6 | The re-embed migration mechanism: how progress is recorded, how exactly-once-per-item survives interruption, and how the vector column width change is sequenced against it. | D-110-5 | `F-110-DIM-01` |
| D-110-7 | Serving behaviour while a corpus is mixed-identity mid-migration, satisfying `NFR-110-1` without silently comparing across identities. | D-110-6 | — |
| D-110-8 | Lane wiring: extend the existing integration lane's package list, or declare a new named lane — and how the executed-assertion count is produced and asserted (`R-110-16`, `R-110-17`). | — | `F-110-LANE-01` |
| D-110-9 | The bounded, index-usable replacement for the cross-product temporal candidate lookup (`R-110-13`). | D-110-1 | — |
| D-110-10 | How the first, baseline-establishing run is distinguished from a gating run so the first green result is not mistaken for a passed gate (`R-110-18`). | D-110-8 | `F-110-FLOOR-01` |
| D-110-11 | How passages are covered by export and deletion so a new content-bearing record class does not silently narrow unconditional exit. Coordinates with spec 111. | D-110-2 | `F-110-EGRESS-01` |
| D-110-12 | What the `chunkedHybridRetrieval` flag actually switches at read time, and what it must **not** switch (it must not gate the identity-agreement refusal, which is a correctness guard rather than a feature). | D-110-7 | — |

## 5. Blast Radius (inputs only, not an assessment)

The plan of record names five source files plus one migration, one evaluation corpus and
one lane change (`C110-7`). A real blast-radius assessment — which callers of the
retrieval path change shape, what the migration costs on a realistic corpus, and what
rollback looks like after a column-width change — is part of authoring this file and has
not been performed.

## 6. Handoffs This File Must Produce

| Consumer | What it needs from this file |
|---|---|
| `bubbles.plan` | Scope decomposition boundaries and dependency order; resolution of `F-110-FLOOR-01` and `F-110-CORPUS-01` into plannable work. |
| `bubbles.test` | The measurement contract: what is executed, against what corpus, producing which metrics, asserted how. |
| `bubbles.train` | What the flag switches, and the condition under which it can be flipped and later retired. |
| spec 111 | The passage record class, so `CorpusBundle` covers it (`F-110-EGRESS-01`). |
