# Marketing — Smackerel `next`

> Every claim in this file traces to a **delivered** row in [`features.md`](features.md). No claim is fabricated. No competitor is invented. Nothing in the *planned*, *blocked*, or *reserved* groups of [`features.md`](features.md) is marketed as though it exists.

## Posture

`next` marketing stays **technically-honest, low-volume, self-hosted-audience-first** — the posture [`../mvp/marketing.md`](../mvp/marketing.md) established and [`../v1/marketing.md`](../v1/marketing.md) carried forward. It is **not** a launch push.

What changes is the *claim class*. `mvp` could honestly say Smackerel **captures and delivers**. `next` is the first phase that can honestly say Smackerel **reasons and answers** — per [`vision.md`](vision.md), this is the promotion-candidate gate where intent-aware retrieval, composing synthesis, runtime model choice, and a public graph read surface become simultaneously true.

Two constraints make this phase's marketing unusually easy to get wrong, and both are recorded here because they are the difference between an honest claim and a fabricated one:

1. **Answer quality is not yet measured.** Per [`business-plan.md`](business-plan.md) R2, `next` ships a router and a synthesiser but no retrieval-evaluation gate. **All quality claims in this phase must stay qualitative.** No accuracy percentage, no benchmark score, no "N% better" comparison may be published until the reserved `next-retrieval-quality-pipeline` slot lands.
2. **The model is not pinned by a deploy.** Per [`deployment.md`](deployment.md), `088` makes model selection a runtime property. Any demonstration or quality statement must **record which model was selected**, or it describes nothing reproducible.

## Audience segments + messaging

| Segment | Who | Core message (claims trace to [`features.md`](features.md)) | Anti-message |
|---------|-----|-------------------------------------------------------------|--------------|
| **Carry-forward: self-hosters** | Same segment as `mvp`; already Compose-comfortable and running an instance | "Same self-hosted, local-first, budget-respecting Smackerel — now able to answer a vague question precisely, with its sources attached." (`084`, `087`, `095`) | Don't re-pitch capture as if it were new; that was the `mvp` gate. Don't suggest a hosted/SaaS pivot. |
| **The `mvp` operator who wants a thinking tool** (primary for this phase) | Passed the `mvp` gate, has a corpus accumulating, wants it useful for thinking rather than only recall — the audience named in [`business-plan.md`](business-plan.md) | "Ask a vague question, get a precise answer with its sources — from a corpus that never leaves hardware you control, using a model you chose at runtime." (`095` routing, `087` synthesis, `088` runtime models) | Don't imply the system is now an assistant that acts. `next` still only reads — outbound action is a `v1` non-goal per [`vision.md`](vision.md). |
| **Authorized external clients** (new in this phase) | Other software the operator already trusts — an MCP-speaking assistant, a companion product — that wants to read the graph through a supported surface | "The knowledge graph has a stable public read API, so other software you authorize can read it instead of reaching into the store." (`080`) | **Do not market the MCP server.** `109` is `specs_hardened`, not delivered. Do not imply per-client grant enforcement exists — `108` is also undelivered. |
| **Reliability-sensitive operators** | Care whether the thing stays up, not what it can say | "The bus path between the Go core and the Python sidecar is hardened to the same bar as the core, and target-readiness checks run before an apply rather than after." (`081`, `082`) | Don't translate hardening into an uptime or SLA number. None is established for this phase. |
| **QF Companion users** | Carry-forward from `mvp` | Unchanged from `mvp`. **`next` expands nothing on the QF axis.** | Per [`actions.md`](actions.md) XP-N1: do not suggest the graph API or the planned MCP server creates any write path into QF. Principle 10 stands. |

## Channel strategy

| Channel | `next` use | Cadence |
|---------|------------|---------|
| Project [`README.md`](../../../README.md) | Reflects the delivered seven once the phase promotes | At phase close (see OQ-N1 in [`actions.md`](actions.md) — the close criterion is itself undecided) |
| [`docs/INVESTOR_OVERVIEW.md`](../../INVESTOR_OVERVIEW.md) | **No `next` row exists today.** Its Phase Overview table carries Phases 1–5, the MVP Gate and the v1 Gate only | Pending — routed in the envelope of this authoring pass, not silently added here |
| [`docs/Product-Principles.md`](../../Product-Principles.md) | Unchanged by this phase; `next` capabilities must satisfy the already-ratified principles | n/a — no `next` mutation |
| [`docs/releases/next/`](.) (this packet) | Reference for the promotion-gate decisions | Locked when the phase closes |
| External posts / talks | Optional, and only after the delivered seven promote **and** a selected-model note accompanies any quality claim | Operator decision; not authorized by this packet |

## Asset list

| Asset | Status | Owner | Source-of-truth claim trace |
|-------|--------|-------|-----------------------------|
| `README.md` refresh covering the delivered seven | Pending phase close | `bubbles.docs` dispatch | [`features.md`](features.md) delivered table |
| `INVESTOR_OVERVIEW.md` `next` Gate row | **Missing — not authored here** | `bubbles.releases` (routed) | [`features.md`](features.md), [`vision.md`](vision.md) |
| "Intent-aware retrieval" explainer (one-pager) | Net-new; author after promotion | `bubbles.docs` dispatch | `095` — the three routed intents (`whole_document`, `structured_aggregate`, `vague_recall`) |
| "Honest refusal" explainer — why the system declines instead of guessing | Net-new; **highest-value asset in this phase** | `bubbles.docs` dispatch | `087`, `084`; [`business-plan.md`](business-plan.md) value-proposition table |
| Knowledge-graph public API reference | Follows the spec's own docs | `bubbles.docs` dispatch | `080` |
| Runtime model-switching note (incl. the "record the selected model" rule) | Net-new | `bubbles.docs` dispatch | `088`; [`deployment.md`](deployment.md) rollout table |
| Quality benchmark / accuracy figures | **NOT AUTHORIZED** — no evaluation gate exists | n/a | [`business-plan.md`](business-plan.md) R2 |
| MCP integration guide | **NOT AUTHORIZED** — `109` undelivered, and gated behind `108` | n/a | [`features.md`](features.md) planned table |
| Multi-provider model marketing | **NOT AUTHORIZED** — `096` is `blocked` | n/a | [`features.md`](features.md) blocked table |
| Corpus export/portability messaging | **NOT AUTHORIZED** — reserved slot, no spec directory yet | n/a | [`features.md`](features.md) reserved table |
| Public testimonials | None — same as `mvp` and `v1` | n/a | n/a |
| Demo videos | None authorized by this packet | n/a | n/a |

## Launch sequence

1. **Internal acknowledgement** when Gate G101 for phase `next` is green — every `required` binding resolving to a terminal, non-blocked, validate-certified spec.
2. **Promotion onto `staging`** per [`deployment.md`](deployment.md). Marketing says nothing before this holds.
3. **`README.md` refresh** via `bubbles.docs` dispatch, covering the delivered seven only.
4. **`INVESTOR_OVERVIEW.md` `next` row** authored (currently absent — see the channel table).
5. **Optional external "promotion-candidate gate reached" post** — operator decision, no obligation in this packet, and only with a selected-model note attached to any quality statement.

Step 2 is a hard precondition. A promotion-gate phase that markets before it promotes is describing an intention, not a capability.

## Forbidden marketing patterns

Carried forward unchanged from [`../mvp/marketing.md`](../mvp/marketing.md) and [`../v1/marketing.md`](../v1/marketing.md):

- ❌ "Smackerel manages your finances" / "gives financial advice" — Principle 10 (QF boundary)
- ❌ "End-to-end encrypted" — still false; self-hosted ≠ e2ee
- ❌ "Smackerel never bothers you" — it bothers within a budget (Principle 6)
- ❌ Streak / unread-count / backlog / guilt framing (Principle 9)
- ❌ Multi-tenant SaaS framing — `next` remains self-hosted
- ❌ "Magic AI" framing — the value is the contract, not the model
- ❌ Any claim that a connector exists when it does not

New in this phase, and each one is a trap this specific capability set creates:

- ❌ **Any copy asserting that Smackerel verifies, enforces, or guarantees where an authorized client's model executes.** Smackerel cannot observe that. Per Principle 11 this is a **BLOCKING** grep gate that scans `docs/`, so the violation is mechanical, not stylistic. The **only permitted claim shape** is: *"Smackerel never sends your knowledge anywhere; a client you explicitly authorize may."*
- ❌ **Marketing cloud inference as the default.** `088` makes cloud an *option*; local inference stays the shipped default (Principle 11, constitution C1). A second BLOCKING grep watches the config default.
- ❌ **Any quantitative answer-quality claim** — accuracy, precision, benchmark, "N% better". No evaluation gate exists ([`business-plan.md`](business-plan.md) R2).
- ❌ **Any quality claim without naming the selected model** ([`deployment.md`](deployment.md)).
- ❌ **Marketing the MCP server (`109`) or per-client corpus grants (`108`)** — both `specs_hardened`, neither delivered.
- ❌ **Marketing multi-provider model connections (`096`)** — `blocked` on an owner-directed handoff.
- ❌ **Marketing corpus export/import/delete** — a reserved slot with no spec directory. This one matters beyond accuracy: claiming an exit path that does not exist is precisely the switching-barrier risk Principle 11 forbids (see [`monetization.md`](monetization.md)).
- ❌ **Implying the graph API is internet-exposed by default.** Its exposure is a deploy-adapter decision; the default posture is *not published beyond the trusted network boundary* ([`deployment.md`](deployment.md)).
- ❌ **Any new-connector claim.** The `mvp` roster lock is still in force; the roster lifts at `v1`, not here ([`vision.md`](vision.md) non-goals).

## Honest `next` framing

> "Smackerel `next` is the moment the corpus becomes answerable. A vague question routes to the right retrieval strategy, the answer is composed from multiple sources with attribution — and when it cannot be grounded, the system refuses instead of inventing. The corpus stays on your hardware, and the model is one you chose at runtime."

That is the honest message. The refusal half is not a caveat to be trimmed in a shorter cut — per [`business-plan.md`](business-plan.md) it is the differentiator, because refusing honestly costs demo quality and systems optimised for demos fabricate instead. Anything beyond this framing requires a delivered row in [`features.md`](features.md).
