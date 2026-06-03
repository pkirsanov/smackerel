# Monetization — Smackerel v1

## Posture

**v1 unlocks the monetization conversation. v1 does NOT commit to a monetization model.**

This packet records that capability arrival (V1 sources + V2 outbound action + V3 mobile decision + V4 capability map + V5 SLO alerts) crosses a threshold where a commercial offering becomes architecturally feasible. It does not assert that one will be built. The operator decides post-v1 whether Smackerel stays a self-hosted personal tool indefinitely, pivots to a hosted offering, or stays self-hosted but adds paid support / paid features.

## Pricing tiers (v1)

None committed. **Operator decision OQ-V9 in [`actions.md`](actions.md).**

If the operator chooses to pursue a monetization conversation post-v1, plausible models (NOT commitments) include:

| Model | Architectural fit | Operator-decision considerations |
|-------|-------------------|----------------------------------|
| OSS-style + paid managed hosting | Requires multi-tenant work (not in v1) and an entirely new operational surface | Tradeoff: scope-creep risk vs revenue path |
| OSS-style + commercial license for non-personal use | Requires license review; existing repo `LICENSE` may already constrain | Low engineering cost; uncertain revenue |
| Paid first-party connectors (e.g., voice transcription cloud backend bundled) | Rides existing self-hosted model; user can opt out | Modest revenue; preserves principles |
| Sponsored / patronage | No engineering work; minimal commitment | Lowest revenue, lowest principle risk |
| Hybrid free OSS + paid premium V-item connectors | Splits the source set into free vs paid; potentially violates "observe everything" promise | High principle-risk; recommend against |

None of these is endorsed by this packet. They are surfaced for operator consideration.

## Revenue model (v1)

None committed.

## Customer acquisition assumptions (v1)

n/a — no commercial offering yet.

## Unit economics (v1)

Same as MVP (all-zero, since no Smackerel-side cost or revenue). v1 capability arrival does NOT change Smackerel-side unit economics by default; only an operator decision to pursue a commercial offering would.

| Metric | v1 value (absent operator monetization decision) |
|--------|--------------------------------------------------|
| Smackerel-side hosting cost per user | $0 |
| Smackerel-side support cost per user | $0 |
| Smackerel-side LLM cost per user | $0 (user-provided) |
| Smackerel-side revenue per user | $0 |

## Path-to-revenue timeline

**Earliest plausible commercial conversation:** post-v1 capability close + `bubbles.devops` deployment validation + V6-A drift sweep clean (per [`business-plan.md`](business-plan.md) gates).

**This packet does NOT timeline that conversation.** Speculating on revenue date violates anti-fabrication discipline.

## Investor / capital signaling

[`docs/INVESTOR_OVERVIEW.md`](../../INVESTOR_OVERVIEW.md) gets a v1 row per DOC-V2 in [`actions.md`](actions.md). This does NOT signal an active fundraise; it documents the surfaced capability state for operator and any future advisory conversations.

## Anti-monetization risks

Even pursuing the conversation carries principled risks. Per [`docs/Product-Principles.md`](../../Product-Principles.md):

| Risk | Principle violated if monetization triggers it |
|------|-----------------------------------------------|
| Tiering V2-A Outbound Action behind a paywall | Principle 1 (observe first, ask second) — if free tier loses the "act for me" capability, the contract weakens |
| Cloud-routing data through Smackerel-controlled infrastructure | Principle 8 (local-first) — local-first promise breaks |
| Adding "engagement metrics" that nudge users to use the product more | Principle 6 (invisible by default) + Principle 9 (design for restart) |
| Making the surfacing controller suggest paid upgrades | Principle 6 (felt not heard); also a UX-trust break |
| Bundling QF-flavored decision support behind a paid tier | Principle 10 (QF companion boundary) — even mentioning QF in marketing violates |

Any commercial pivot MUST be evaluated against these. This packet records the warning; operator decision owns the call.

## Honest v1 framing

> "Smackerel v1 makes the commercial conversation possible. It does not make the commercial decision."

That's the position. Anything stronger requires operator direction at the time of the decision.
