# Monetization — Smackerel `next`

## Posture

**Smackerel `next` is pre-revenue and commits to no commercialization.** This is a decision, not a TBD.

That sentence is not new here — it is carried verbatim in substance from [`business-plan.md`](business-plan.md), which states: *"None. `next` commits to no commercialization."* This document records **why** that remains correct at this phase, and what would have to become true before the position could change.

The posture lineage across the three phases is consistent and deliberate:

| Phase | Position | Source |
|---|---|---|
| `mvp` | Pre-revenue, not monetized; *"working personal product, no revenue model, no obligation to develop one"* | [`../mvp/monetization.md`](../mvp/monetization.md) |
| `next` | **Unchanged — still pre-revenue.** `next` adds reasoning capability, not a commercial surface | this document |
| `v1` | *"v1 unlocks the monetization conversation. v1 does NOT commit to a monetization model."* Operator decision OQ-V9 | [`../v1/monetization.md`](../v1/monetization.md) |

`next` sits between a phase that had no revenue model and a phase that merely *unlocks the conversation*. It does not advance the commercial position, and nothing in its capability set is a commercial surface: Smackerel remains self-hosted, the operator supplies the hardware, and there is no Smackerel-side service to charge for.

## The `next`-specific argument against monetizing now

This is the one genuinely new monetization content in this phase, and it is derived from [`features.md`](features.md) plus a ratified principle rather than carried from `mvp` or `v1`.

`next` is the first phase in which Smackerel accumulates **materially more user value per corpus** — the reasoning layer makes a large corpus worth more than a small one. That is exactly the condition under which accumulated value becomes a **switching barrier**.

Product Principle 11 forbids that outcome: corpus export, relocation, and deletion must remain **unconditional**, and accumulated value must not become a switching barrier. The principles instruction file enforces it mechanically — a **BLOCKING** grep that fails when an export / delete / purge path is gated behind a licence, subscription, entitlement, paywall, or billing tier.

And the exit path is **not yet delivered**. Corpus portability (export / import / delete) is a **reserved slot** in [`features.md`](features.md) with `spec=none` — no spec directory exists yet. [`business-plan.md`](business-plan.md) records the same condition as risk **R5**: *"Accumulated value without an exit is the switching barrier Principle 11 forbids"*, and names it *"the strongest argument for keeping monetization deferred."*

The conclusion follows without needing a number attached to it:

> **A paid surface introduced before corpus portability lands would gate a corpus the user cannot yet leave. That is the switching barrier Principle 11 forbids, not a pricing choice.**

Corpus portability is therefore a **precondition for the monetization conversation**, and it is a precondition `next` introduces. It is additional to — not a replacement for — the three `mvp`-recorded gates below.

## Pricing tiers (`next`)

**None.** No tier, no plan, no metered surface, no paid feature exists or is proposed in this phase.

No tier structure is sketched here, even hypothetically. [`../v1/monetization.md`](../v1/monetization.md) already carries a table of *plausible, explicitly non-committal* models for the operator to consider at `v1`; reproducing or extending it here would manufacture the appearance of a pricing direction that no one has chosen.

## Revenue model (`next`)

**None.**

## Customer acquisition assumptions (`next`)

**n/a — there is no commercial offering and therefore no "customer" relationship.** There are operators who run the software on their own hardware. Reach is implicit via the channels in [`marketing.md`](marketing.md), which are themselves deliberately low-volume in this phase.

One audience shift is worth recording without commercial interpretation: [`business-plan.md`](business-plan.md) notes that `next` brings in **authorized external clients** as a *"secondary, non-paying but strategically important audience"* — the graph public API (`080`) converts Smackerel from an application into a substrate. Strategically important is not a revenue claim, and this packet does not convert it into one.

## Unit economics (`next`)

Unchanged from `mvp` and `v1`. `next` adds no Smackerel-side cost and no Smackerel-side revenue.

| Metric | `next` value |
|--------|--------------|
| Smackerel-side hosting cost per user | $0 (operator self-hosts) |
| Smackerel-side support cost per user | $0 (no commercial support obligation) |
| Smackerel-side inference cost per user | $0 (operator-provided; local inference is the shipped default) |
| Smackerel-side revenue per user | $0 |
| Gross margin | n/a — no revenue and no Smackerel-side variable cost |

**One honest nuance this phase introduces.** `088` makes model selection a runtime property, so an operator may point Smackerel at a hosted provider. That cost is **operator-side and operator-chosen**; it does not appear in the table above, because the table measures Smackerel-side economics and no such cost flows through Smackerel. It is recorded because a reader could otherwise mistake "runtime-switchable models" for a metering opportunity. It is not one: metering an operator's own inference on their own hardware has no mechanism and no justification.

## Path-to-revenue timeline

**This packet does not timeline a commercial conversation.** Speculating on a revenue date would violate anti-fabrication discipline, exactly as [`../v1/monetization.md`](../v1/monetization.md) states.

What can be stated truthfully is the **precondition set**, which `next` extends by one:

| # | Precondition | Origin | State at this phase |
|---|---|---|---|
| 1 | Outbound Action capability | `mvp` path-to-revenue gate → `v1` scope | Not started. Explicitly a `next` non-goal ([`vision.md`](vision.md)). |
| 2 | Personal Productivity Sources | `mvp` path-to-revenue gate → `v1` scope | Not started. The connector-roster lock is still in force through `next`. |
| 3 | Native-mobile decision | `mvp` path-to-revenue gate → `v1` scope | Undecided. A `v1` item; the PWA remains the client surface. |
| 4 | **Corpus portability (unconditional export / import / delete)** | **New — introduced by this phase** | **Reserved slot, `spec=none`.** No spec directory exists yet. |

None of the four is satisfied at `next`. The earliest plausible commercial conversation therefore remains where [`../v1/monetization.md`](../v1/monetization.md) put it — post-`v1` — and `next` moves it no closer.

## Investor / capital signaling

[`docs/INVESTOR_OVERVIEW.md`](../../INVESTOR_OVERVIEW.md) is a reference document; it does **not** imply an active fundraise, at this phase or any prior one. No external capital is sought.

Two accuracy notes:

- [`business-plan.md`](business-plan.md) records that capital requirements are *not applicable in the conventional sense* for this phase: no hosted infrastructure, no paid third-party dependency, no headcount assumption. The one genuine cost is **operator time** on the blocked handoff (OPS-N1 in [`actions.md`](actions.md)) — human attention, not capital.
- **`INVESTOR_OVERVIEW.md` currently carries no `next` Gate row.** Its Phase Overview table lists Phases 1–5, the MVP Gate and the v1 Gate. Any capital-adjacent reading of that document today therefore omits this phase entirely. That gap is routed, not silently patched here.

## Anti-monetization risks

Carried forward from [`../v1/monetization.md`](../v1/monetization.md), with per-row applicability assessed against **this** phase's delivered capability set. These are the ways a future commercial decision could break a ratified principle.

| Risk | Principle at stake | Live at `next`? |
|------|--------------------|-----------------|
| Tiering outbound action behind a paywall | Principle 1 | **No** — outbound action is not in this phase. |
| Cloud-routing corpus data through Smackerel-controlled infrastructure | Principle 11 (local-first) | **Latent.** `088` makes a provider switch one config change away. [`business-plan.md`](business-plan.md) R4 tracks the same erosion risk on the technical axis; a commercial motive would be a second pressure on it. |
| Engagement metrics that nudge more usage | Principles 6, 9 | **No** — nothing in this phase adds an engagement surface. |
| The surfacing controller suggesting paid upgrades | Principle 6 | **No** — no upgrade surface exists. |
| Bundling QF-flavoured decision support behind a paid tier | Principle 10 | **No**, and doubly guarded: [`actions.md`](actions.md) XP-N1 forbids any write path into the QF boundary regardless of commercial framing. |

New at `next`, from this phase's own capability set:

| Risk | Principle at stake | Why it is specific to this phase |
|------|--------------------|----------------------------------|
| **Gating corpus export behind any entitlement** | Principle 11 — **BLOCKING** grep on export/delete/purge paths | The reasoning layer is what makes the corpus expensive to leave. This is the risk this phase creates and the reason for the deferral argument above. |
| **Tiering the knowledge-graph public API (`080`)** | Principle 11 | The graph is derived from the operator's own corpus on the operator's own hardware. Charging for read access to it is a toll on the user's own data. |
| **Gating per-client corpus grants (`108`) as a paid feature** | Principle 11 | Grants are the *audit and consent* mechanism for egress. Making safety a paid tier makes the free configuration the less safe one. |
| **Metering model switching (`088`)** | Principle 11, constitution C1 | Any meter creates an incentive to make the metered (cloud) path the convenient default, which is exactly the local-default erosion C1 forbids. |

## Open owner decisions — recorded, not resolved

These are genuinely undecided. They are written down rather than answered with a plausible figure, and **none of them is currently carried in [`actions.md`](actions.md)**, whose pending list holds OQ-N1 (phase-close criterion), OQ-N2 (reserved-slot homing) and OQ-N3 (`096` disposition). They are proposed for addition there by `bubbles.releases`; this document does not edit `actions.md`.

| Proposed ID | Question | Why it is not answered here |
|---|---|---|
| MON-N1 | Does the `next` position — pre-revenue with no commitment — hold through phase close, or does promotion of the delivered seven trigger a commercial review? | Requires operator direction. This packet's recommendation is that it holds: none of the four preconditions is met. |
| MON-N2 | Is corpus portability accepted as a **hard** precondition for any paid surface, or as a strong recommendation the operator may override? | Principle 11 supplies the constraint but not the governance strength. A hard precondition is enforceable; a recommendation is not. |
| MON-N3 | If the graph public API (`080`) or a future MCP surface (`109`) is consumed by a **third party's** software rather than the operator's own, does that change the licensing question? | Touches the repo [`LICENSE`](../../../LICENSE) and is a licence-review question, not a pricing question. `next` is the first phase where the scenario is technically possible. |

No revenue figure, pricing point, conversion assumption, market size, or customer-count target appears anywhere in this document, because none is derivable from anything in this repository.

## Honest `next` framing

> "Smackerel `next` makes the corpus far more valuable to its owner. It deliberately does **not** make it more expensive to leave — and until the exit path is built, that restraint is the whole monetization position."

That is the position. Anything stronger requires operator direction at the time of the decision, and an exit path that exists.
