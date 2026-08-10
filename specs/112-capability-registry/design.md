# Design: 112 Capability Registry

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
| Requirements R-112-01 … R-112-33 | [`spec.md`](spec.md) §8 | The behavioural contract; design chooses how, never whether |
| Domain capability model | [`spec.md`](spec.md) §4 | Primitives, relationships, and the nine policies every implementation must obey |
| Verified evidence base E1 … E19 | [`spec.md`](spec.md) §3 | Current-state facts, each re-verified 2026-08-04 against the working tree |
| The `D17` sharpening | [`spec.md`](spec.md) §3 | The unsurfaced set is 11 dispatchable capabilities, not 22 contracts; the unit distinction drives `F-112-UNIT-01` |
| Open findings `F-112-*` | [`spec.md`](spec.md) §11 | Four are BLOCKING and gate the design pass itself |
| The existing experience catalog | [`internal/experience/`](../../internal/experience/) | The artifact being EXTENDED. `catalog.go`, `validator.go`, and the navigation, renderer, consumer-inventory, state and mutation projections are the starting point, not prior art to replace |
| The catalog's generation path | `config/smackerel.yaml` `product_experience` block → `catalog.gen.json` | The registry's generation must join this path, not create a second one |
| Principle 5 | [`docs/Product-Principles.md`](../../docs/Product-Principles.md) — Principle 5, One Graph, Many Views | The governing principle; it is what forbids a second registry |
| Principle 11 | [`docs/Product-Principles.md`](../../docs/Product-Principles.md) — Principle 11, Local-First Data Ownership (ratified 2026-07-29, BLOCKING) | Governs the external tool projection: exposure to an external client is a grant decision |
| Spec 108 grant model | [`specs/108-corpus-grant-enforcement/spec.md`](../108-corpus-grant-enforcement/spec.md) | Supplies `corpus:read`; `specs_hardened`, not delivered; same `next` train |
| Spec 109 tool surface | [`specs/109-mcp-knowledge-server/spec.md`](../109-mcp-knowledge-server/spec.md) | Consumes the external projection; explicitly forbids passthrough derivation |

---

## Decisions Required Before Any Scope Can Execute

Each row is undecided. None has a preferred answer recorded here, because recording one
would be `bubbles.analyst` making a `bubbles.design` decision.

| # | Decision | Blocking finding | Consequence of getting it wrong |
|---|---|---|---|
| D1 | What constitutes **one capability**, and how prompt contracts bind to capabilities given the mapping is not 1:1 | `F-112-UNIT-01` | The registry records the wrong unit and reproduces the original duplication at a new granularity |
| D2 | How the **independent universe** of built capabilities is derived for the coverage check, avoiding the circularity already present at `route_inventory_test.go:92-100` | `F-112-UNIVERSE-01` | A check whose expected set comes from the artifact under test cannot fail and is decorative |
| D3 | How the descriptor **extends** the existing catalog: whether capability becomes a first-class record the catalog's `capability_id` resolves into, and how the `KnownCapabilities` seam is fed in production rather than only in tests | — | A capability record that does not resolve the existing reference leaves the half-built state in place with a new layer on top |
| D4 | The **exposure-class vocabulary**, and the shape of the recorded reason required for every non-user-facing class | — | A vocabulary that permits an implicit or empty state re-admits "undeclared", which R-112-13 forbids |
| D5 | Which of the two contradictory behaviours is **correct for `/ask`**, and whether `/recipe` and `/cook` are kept or retired | `F-112-ALIAS-01` | Generating from one descriptor silently changes shipped user-visible behaviour for at least one token |
| D6 | Whether this feature performs the **navigation cutover** or hands it to the shell specification, and how R-112-09 is satisfied by whichever does not | `F-112-CUTOVER-01` | The hand-maintained authorities survive beside their projections, which P2 defines as a defect |
| D7 | How the **external tool projection** is declared per capability without becoming the passthrough spec 109 forbids | `F-112-109-01` | Either the tool list is hand-maintained again, or spec 109's constraint is violated |
| D8 | What the **guaranteed core guarantees on a surface that renders no navigation**, given 29 of 31 PWA pages render none | `F-112-ISLANDS-01` | R-112-20 is satisfied vacuously and the guarantee means nothing on most pages |
| D9 | **Ordering against spec 108 and spec 109**, both `specs_hardened` on the same train and both depended on | `F-112-108-01` | Building against a grant model or a tool surface that does not exist yet |
| D10 | How a capability is **withheld** under R-112-25 when its grant is unenforceable, and how that withholding is reported without leaking the capability's existence (NFR-112-06) | — | Either capabilities surface unguarded, or the withholding itself becomes an information leak |

---

## Constraints The Design Pass Cannot Trade Away

These are restated from `spec.md` because they bound the design space rather than sit
inside it.

- **Extend, never duplicate** (P5, Non-Goal 1). A second registry beside
  `internal/experience/` is a failure of this feature, not an implementation variant of it.
- **Every surface is generated** (P2, R-112-08). A hand-maintained list surviving beside its
  projection is a defect, not a migration phase.
- **Exposure is declared, never defaulted** (P3, R-112-13). "Not listed" must stop being a
  way to be invisible.
- **Reach never outruns the guard** (P4, R-112-25). A capability whose authorization cannot
  be enforced is not surfaced.
- **Coverage is checked against an independent universe** (P6, R-112-31). This is the one
  requirement most easily satisfied in appearance only.
- **The registry is content-free** (P9, R-112-07). Identity and policy only.
- **The grant vocabulary is consumed, not redefined** (R-112-26, Non-Goal 2).

---

## Explicitly Not Decided Here

Record shape, field types, file format, generation mechanism, the exposure-class values
themselves, the intent-phrasing representation, projection file layout, the coverage
check's implementation, migration sequencing, and scope decomposition. Scope decomposition
belongs to `bubbles.plan`; everything else on that list belongs to `bubbles.design` and is
untouched.
