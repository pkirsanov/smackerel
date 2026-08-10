# Design: 111 Corpus Portability & Artifact Sensitivity

**Status:** `not_started` · **Workflow mode:** `product-to-planning` · **Release train:** `next`
**Owner of this artifact:** `bubbles.design`
**Created by:** `bubbles.analyst` as an honest initial artifact so the packet is structurally complete

---

## What This File Is, And Is Not

**No design decision has been made for this feature.** This file was created alongside
[`spec.md`](spec.md) during requirements authoring so the artifact set is complete and
lintable. It contains no architecture, no schema, no interface, and no chosen approach.

`bubbles.analyst` owns `spec.md` and authored it. `bubbles.design` owns this file. Nothing
below is a decision; every entry is a question this feature cannot be built without
answering, recorded so the design pass starts from a real inventory rather than a blank
page. Anything that reads like a proposal is a restatement of a **requirement already in
`spec.md`**, not a design choice added here.

---

## Inputs The Design Pass Must Consume

| Input | Location | Why it is load-bearing |
|---|---|---|
| Requirements R-111-01 … R-111-29 | [`spec.md`](spec.md) §8 | The behavioural contract; design chooses how, never whether |
| Domain capability model | [`spec.md`](spec.md) §4 | Primitives, relationships and the eight policies every implementation must obey |
| Verified evidence base E1 … E12 | [`spec.md`](spec.md) §3 | Current-state facts, each re-verified 2026-08-04 against the working tree |
| Open findings F-111-* | [`spec.md`](spec.md) §11 | Three are BLOCKING and gate the design pass itself |
| Principle 11 | [`docs/Product-Principles.md`](../../docs/Product-Principles.md) §Principle 11 | Ratified 2026-07-29; BLOCKING; governs exit and egress |
| Spec 110 passage record class | [`specs/110-retrieval-quality-foundation/spec.md`](../110-retrieval-quality-foundation/spec.md) `F-110-EGRESS-01` | Routed here explicitly; affects the manifest's first version |
| Spec 108 grant model | [`specs/108-corpus-grant-enforcement/spec.md`](../108-corpus-grant-enforcement/spec.md) | The egress decision consumes its grants; also on `next` |

---

## Decisions Required Before Any Scope Can Execute

Each row is undecided. None has a preferred answer recorded here, because recording one
would be `bubbles.analyst` making a `bubbles.design` decision.

| # | Decision | Blocking finding | Consequence of getting it wrong |
|---|---|---|---|
| D1 | How the manifest's record-class list is derived, so it cannot be incomplete on the day it is written | `F-111-MANIFEST-01` | A hand-written list reproduces the original defect in a new location |
| D2 | What "canonical content hash per record class" means: serialisation order, included fields, excluded fields | `F-111-CENSUS-01` | Parity comparison is not reproducible across two instances |
| D3 | The resume-position encoding that satisfies R-111-10, R-111-11 and R-111-12 together | — | Tied ordering keys skip records; coarse precision duplicates them |
| D4 | Whether deleting an artifact deletes, orphans, or rewrites records derived from it | `F-111-DELETE-01` | Affects both delete correctness and round-trip parity |
| D5 | The sensitivity vocabulary, and its relationship to the vocabularies already established for other record classes (E9) | — | A third independent vocabulary would be a new inconsistency |
| D6 | Migration posture for existing artifacts left unset once sensitivity is enforced | `F-111-BACKFILL-01` | Enabling a fail-closed gate against an unclassified corpus refuses legitimate traffic |
| D7 | Where the single egress chokepoint sits so that R-111-27 is structurally true, not merely intended | — | A second uncontrolled path makes the whole gate decorative |
| D8 | Ordering against spec 110 and spec 108, both of which this feature depends on | `F-111-110-01`, `F-111-108-01` | Building against a grant model or record class that does not exist yet |

---

## Constraints The Design Pass Cannot Trade Away

These are restated from `spec.md` because they bound the design space rather than sit
inside it.

- **One manifest governs three operations** (P1). A design that gives export, import or
  delete its own scope list has not satisfied the spec, however convenient.
- **No second export path** (Non-Goal 1). The existing surface is corrected.
- **The egress decision fails closed** (P7, R-111-25). Missing input is refusal.
- **Unset sensitivity is not permissive** (R-111-20).
- **Membership never depends on processing state** (P2, R-111-08).
- **Copy obeys Principle 11's honesty constraint** (NFR-111-06). No surface may claim the
  product enforces, verifies, guarantees or attests client-side inference locality.

---

## Explicitly Not Decided Here

Storage shape, interface signatures, serialisation format, transport, pagination encoding,
vocabulary values, migration mechanics, and scope decomposition. Scope decomposition is
`bubbles.plan`'s; everything else on that list is `bubbles.design`'s and is untouched.
