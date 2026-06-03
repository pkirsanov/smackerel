# Smackerel — Product Principles

> **STATUS**: Ratified 2026-06-03; companion enforcement file in [`.github/instructions/product-principles.instructions.md`](../.github/instructions/product-principles.instructions.md) is BLOCKING.
>
> The constitution defines the **engineering principles** (10 numbered Core Principles) that are already NON-NEGOTIABLE on their own enforcement track. The design doc defines the **product design principles** (13 principles in §2). This document surfaces the **product-level principles** (the WHY and the user-facing contracts) at a higher abstraction level than the design doc's design principles.
>
> Each principle below cites its evidence source. The principles were ratified by the owner on 2026-06-03; the [companion enforcement file](../.github/instructions/product-principles.instructions.md) is now binding.

---

## How To Use This Document

| Audience | What To Do |
|----------|-----------|
| **Product owner** | Principles 1–10 are ratified (2026-06-03). Edits go through the normal product-principles change process. |
| **Engineering** | Principles 1–10 are binding via the companion enforcement file. |
| **Spec authors** | When writing a feature spec touching a principle area, reference the principle by number and cite `docs/Product-Principles.md`. |
| **AI agents** | Read this file alongside the constitution. Principles 1–10 are BLOCKING; constitution rules (C1–C10) remain NON-NEGOTIABLE on their own track. |

---

## Already-Ratified Engineering Principles (Constitution)

These are **already binding** in [`.specify/memory/constitution.md`](../.specify/memory/constitution.md). Listed here as reference; they are on a separate enforcement track from the ratified product principles below.

| # | Constitution Principle | Source |
|---|------------------------|--------|
| **C1** | Local-First Knowledge Ownership | Constitution Core Principle 1 |
| **C2** | Go-First Runtime, Python-Only ML Sidecar | Constitution Core Principle 2 |
| **C3** | Processed Knowledge Beats Raw Dumps | Constitution Core Principle 3 |
| **C4** | Explainable Synthesis | Constitution Core Principle 4 |
| **C5** | Passive by Default, Explicit on Action | Constitution Core Principle 5 |
| **C6** | Docker-First Self-Hosting | Constitution Core Principle 6 |
| **C7** | Single CLI Operations | Constitution Core Principle 7 |
| **C8** | Single Source Of Truth Configuration | Constitution Core Principle 8 |
| **C9** | Isolated Test Environments | Constitution Core Principle 9 |
| **C10** | Docker Lifecycle Safety And Freshness | Constitution Core Principle 10 |

The principles below are **product-level principles** that complement the constitution's engineering principles.

---

## Principle 1 — Observe First, Ask Second

**Status**: Ratified 2026-06-03
**Evidence**: [`docs/smackerel.md`](smackerel.md) §2 Design Principle 1, §1.2 Vision

The system exhausts its own inference before asking the user anything. Default mode is **passive**. Active capture is one of many input mechanisms, not the primary one.

This is the cornerstone product principle. Existing tools (Notion, Obsidian, Evernote) require organizing work at the highest cognitive load moment. Smackerel inverts this: the system observes, processes, and connects; the user lives their life.

**Implication for product decisions**: Any feature that requires the user to organize, tag, classify, or file at capture time MUST justify why it cannot be inferred from observation. The default UX path is "user does nothing extra; system figures it out."

---

## Principle 2 — Vague In, Precise Out

**Status**: Ratified 2026-06-03
**Evidence**: [`docs/smackerel.md`](smackerel.md) §2 Design Principle 3

Search and retrieval MUST work with imprecise, natural-language queries. Users remember impressions ("that pricing video", "what did Sarah recommend"), not metadata.

This is a customer-trust principle: a user who cannot find their saved information loses confidence in the system. Smackerel's success metric (per `docs/smackerel.md` §1.4) is **>75% correct on first result for vague queries**.

**Implication for product decisions**: A feature that requires the user to remember exact field names, exact dates, exact tags is a regression. Semantic search via pgvector + LLM re-ranking is the contract.

---

## Principle 3 — Knowledge Breathes (Lifecycle, Not Static)

**Status**: Ratified 2026-06-03
**Evidence**: [`docs/smackerel.md`](smackerel.md) §2 Design Principle 4, §11 Knowledge Lifecycle (Promotion & Decay)

Topics automatically promote (rising engagement) and decay (declining engagement). The knowledge surface is always **live**, not a static archive that grows forever and rots.

Topic lifecycle states (per design doc §11): emerging → active → hot → cooling → dormant → archived.

This is a customer-trust principle: existing tools accumulate stale information that buries fresh insight. Smackerel's freshness contract is the long-term retention lever.

**Implication for product decisions**: A feature that adds permanent state without lifecycle management (no decay path, no promotion signal) is incomplete. Every persisted artifact participates in the lifecycle.

---

## Principle 4 — Source-Qualified Processing

**Status**: Ratified 2026-06-03
**Evidence**: [`docs/smackerel.md`](smackerel.md) §2 Design Principle 5

Use metadata from source systems (Gmail labels, YouTube playlists, watch history, calendar attendees) to improve classification and priority. The source IS the signal.

This is a quality multiplier: connectors that strip metadata down to raw content lose the highest-leverage retrieval improvement signal.

**Implication for product decisions**: A connector spec MUST declare what source metadata it preserves. Stripping metadata for "simplicity" is rejected.

---

## Principle 5 — One Graph, Many Views

**Status**: Ratified 2026-06-03
**Evidence**: [`docs/smackerel.md`](smackerel.md) §2 Design Principle 6, §8 Knowledge Graph & Storage

ALL artifacts live in ONE knowledge graph. Views (by topic, person, time, place) are projections, not separate stores.

This is an architectural principle with product implications: cross-domain connections (the article-and-video-saying-the-same-thing problem) ONLY work when everything lives in one graph.

**Implication for product decisions**: A new artifact type that creates a parallel store (its own database, its own search index, its own graph) is rejected. Extend the existing graph.

---

## Principle 6 — Invisible By Default, Felt Not Heard

**Status**: Ratified 2026-06-03
**Evidence**: [`docs/smackerel.md`](smackerel.md) §2 Design Principle 7, §1.4 Success Metrics ("System-initiated prompts < 3 per week")

NO notifications unless actionable. NO "I processed 47 items today!" noise. The system is **felt, not heard**.

Success metric (per design doc §1.4): system-initiated prompts < 3 per week (non-urgent). Excessive prompting violates the contract.

This is a customer-trust principle: notification fatigue kills tools. Smackerel must earn every prompt.

**Implication for product decisions**: A feature that adds a notification, badge, or interruption MUST clear an explicit actionability bar. Status updates ("we processed your email") are forbidden by default.

---

## Principle 7 — Small, Frequent, Actionable Output

**Status**: Ratified 2026-06-03
**Evidence**: [`docs/smackerel.md`](smackerel.md) §2 Design Principle 8, §1.4 Success Metrics ("Daily digest read time < 2 minutes")

Digests fit a phone screen. Insights are one sentence. Actions are concrete. NEVER a 2,000-word analysis.

Success metric (per design doc §1.4): daily digest read time < 2 minutes.

This is a customer-trust principle: long output is a hidden tax. Users abandon tools that demand 10-minute reading sessions for daily digests.

**Implication for product decisions**: A feature that adds a long-form output (multi-page analysis, multi-section weekly synthesis) must justify why a phone-screen-fit version cannot deliver the value.

---

## Principle 8 — Trust Through Transparency

**Status**: Ratified 2026-06-03
**Evidence**: [`docs/smackerel.md`](smackerel.md) §2 Design Principle 11, Constitution Core Principle 4 (Explainable Synthesis), Model Compensations table

Every filing decision is logged. Every connection is explainable. Every synthesis cites sources. The user can ALWAYS trace WHY the system did what it did.

This is the explainability contract that prevents the system from becoming a black box. The constitution's Model Compensations table specifically requires "persist synthesized output only after schema validation and source-link attachment" — fabricated insights are blocked at the persistence boundary.

**Implication for product decisions**: A feature that produces output without source attribution (a digest item with no link back, a synthesis claim with no citation) is rejected. Trust degrades exponentially when explainability slips.

---

## Principle 9 — Design For Restart, Not Perfection

**Status**: Ratified 2026-06-03
**Evidence**: [`docs/smackerel.md`](smackerel.md) §2 Design Principle 12

Miss a week? The system kept ingesting passively. Just ask "what did I miss?" NO backlog guilt. NO punishment for absence.

This is a customer-trust principle: existing tools (todo apps especially) punish absence with guilt-inducing backlog screens. Smackerel must NEVER make a returning user feel bad about being away.

**Implication for product decisions**: A feature that surfaces "you have 47 unread items" or "you missed 12 captures" is rejected. The returning UX is "ask the system what mattered while you were gone, get a normal-length digest."

---

## Principle 10 — QF Companion Boundary (NON-NEGOTIABLE Cross-Product Surface)

**Status**: Ratified 2026-06-03
**Evidence**: [`docs/smackerel.md`](smackerel.md) §1.6 (QF Companion Boundary)

When acting as a companion surface for QuantitativeFinance:

- Smackerel ingests QF decision packets as **external authoritative artifacts**, not local recommendations
- Smackerel preserves QF trust metadata (`CalibrationBadge`, `DataProvenanceBadge`, packet IDs, intent/scenario IDs, trace IDs, deep links) WITHOUT modification
- Smackerel exports `PersonalEvidenceBundle`s with source, sensitivity, consent, provenance metadata back to QF
- Smackerel **DOES NOT** approve trades, change mandates, execute orders, or give financial advice
- QF owns financial decisions; Smackerel owns personal memory, reminders, digesting, retrieval, context assembly

This is a customer-trust principle for both products. Smackerel users know personal context never triggers financial action without QF's explicit decision pipeline. QF users know their decision integrity is unmediated by personal-memory inference.

**Implication for product decisions**: ANY feature that crosses the QF companion boundary (Smackerel-initiated trade, mandate suggestion, execution recommendation) is rejected without explicit cross-product principal review.

---

## Surfacing Process (How This File Got Built)

This file was generated by reading existing repo evidence:

| Source Read | What It Surfaced |
|-------------|------------------|
| [`.specify/memory/constitution.md`](../.specify/memory/constitution.md) | C1-C10 (referenced as already-ratified) + Model Compensations table for Principle 8 |
| [`docs/smackerel.md`](smackerel.md) §1.2 Vision | Principle 1 |
| [`docs/smackerel.md`](smackerel.md) §1.4 Success Metrics | Principles 2, 6, 7 (quantified retrieval, prompts, digest length targets) |
| [`docs/smackerel.md`](smackerel.md) §1.6 QF Companion Boundary | Principle 10 |
| [`docs/smackerel.md`](smackerel.md) §2 Design Principles | All 10 principles trace to design doc §2 entries |
| [`docs/smackerel.md`](smackerel.md) §11 Knowledge Lifecycle | Principle 3 lifecycle states |

NO principles were fabricated. Every principle traces to existing source. The owner's job is to confirm each surfaced principle accurately reflects current product direction.

---

## Ratification Process

1. Owner reviews each principle in this file (1-10; C1-C10 already live).
2. Owner approves, edits, or rejects each principle.
3. Owner stamps each principle as "Ratified YYYY-MM-DD" once confirmed.
4. Once ALL principles are ratified, the [companion enforcement file](../.github/instructions/product-principles.instructions.md) becomes binding policy.

Principles 1–10 were ratified 2026-06-03. The companion enforcement file is now binding. The constitution remains the sole NON-NEGOTIABLE engineering authority on its own track.

---

## Cross-References

- [`.specify/memory/constitution.md`](../.specify/memory/constitution.md) — Engineering principles (NON-NEGOTIABLE)
- [`docs/smackerel.md`](smackerel.md) — Authoritative product and architecture design (source for all surfaced principles)
- [`.github/instructions/product-principles.instructions.md`](../.github/instructions/product-principles.instructions.md) — Product principles enforcement (BLOCKING; ratified 2026-06-03)
- [`docs/INVESTOR_OVERVIEW.md`](INVESTOR_OVERVIEW.md) — Investor-facing platform overview
