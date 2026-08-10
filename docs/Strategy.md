# Smackerel — Strategy on One Page

**Updated:** 2026-08-02 · re-measured 2026-08-10 · Detail: [`Product_Delivery_Plan.md`](Product_Delivery_Plan.md) · Evidence: [`Product_Direction_2026-07-31.md`](Product_Direction_2026-07-31.md) · Principles: [`Product-Principles.md`](Product-Principles.md)

---

## What it is

Smackerel is a **personal context server**. It quietly ingests everything you touch — mail,
calendar, browsing, location, purchases, media, chat, property, markets — into one graph you
own, on hardware you control, and exposes that context to the tools you already work in.

The thin web app is an operator console, not the product. The product is the governed corpus.

## Why it can win

Passive capture and daily digests are table stakes now; every competitor has them. "Local AI"
alone is contested too. One thing is still unclaimed:

> **A whole-life corpus, on your hardware, with no required vendor, exposed through explicit
> grants to the clients you choose.**

Competitors each hold a slice — notes, meetings, reading, bookmarks — and each holds it on
their servers. Nobody holds all of it under the user's control. That is the wedge, and it is
only defensible if ownership, exit, and authorisation are genuinely airtight.

## The philosophy

Eleven ratified principles govern the product. Four commitments decide almost every design call:

| Commitment | What it forces |
|---|---|
| **Observe first, ask second** | The system infers; it never makes you file, tag, or classify at capture. Vague questions in, precise cited answers out. |
| **Invisible by default** | Fewer than three non-urgent interruptions a week. Small, frequent, actionable output — never a report on how busy the system has been. |
| **Trust through transparency** | Every claim carries its source. A refusal is honest. A failure is never dressed up as a success. |
| **Local-first ownership** | Your corpus, your hardware, no required vendor. Export, relocate and delete stay unconditional — accumulated value must never become a switching cost. |

The rest follow from these: knowledge has a lifecycle, sources stay qualified, one graph feeds
many views, and returning after a gap is welcomed rather than punished with a backlog.

## What it does — three pillars

| Pillar | In one line |
|---|---|
| **LLM wiki** | Everything you captured, readable as connected pages — topics, people, places, time — that you *browse*, not just search. |
| **Second brain** | It tells you what matters before you ask, and is right often enough that you trust it. |
| **Extended scenarios** | Every capability is reachable by plain language — and from the other tools you already use. |

## Where we stand

The foundations are essentially built. The surfaces are not.

| | |
|---|---:|
| Specs done | **99 of 112** |
| Connectors shipped | **19** |
| Capabilities built · exposed to users | **27 · 5** |
| Remaining product work (3 specs) | **80 of 537** items |

Translation: we can ingest almost anything and reason over it, but a user cannot yet navigate
it, trust what it surfaces, or ask for most of what it can already do.

**Re-measured 2026-08-10.** Every figure above held across 34 commits — including
`80 of 537`, unchanged. No pillar item was completed in that window. Three new
specs (110, 111, 112) now carry the enabling work. The release-train gate passes
locally but **still fails at `HEAD`** — its fix is uncommitted. See
[`Delivery_Position_2026-08-10.md`](Delivery_Position_2026-08-10.md).

## How we get there — six stages

Each stage ends with something a person can actually do.

| # | Stage | The user can then… |
|---|---|---|
| **1** | **Stop leaking and losing** — close five holes: scope bypass, model-supplied identity, default-open bot, credential leak, cursor data loss | *(nothing new — but nothing is lost or leaked)* |
| **2** | **One front door** — collapse 31 loose pages onto the 20 declared surfaces | Move around the product; finish onboarding; trust an offline save |
| **3** | **A wiki you can walk** — real edge semantics, then the graph explorer | Open any topic, person, place or date and follow genuine connections → **Pillar A** |
| **4** | **Retrieval that finds** — chunked content, one configured model, measured quality | Find a fact buried mid-document, and see which passage answered |
| **5** | **A brain that speaks first** — rank by relevance, surface the insight already being computed, tell the truth about delivery | Open one daily surface, see a few things that matter with reasons and citations → **Pillar B** |
| **6** | **Ask anywhere** — one capability registry, then the MCP server | Ask for anything in plain language, from Smackerel or from your editor → **Pillar C** |

**The bottleneck is Stage 2.** The shell gates the proactive experience directly and gates the
wiki's home indirectly. Nothing user-visible finishes until it lands.

## What we will not build yet

More connectors. Outbound actions. New standalone pages. A native mobile app. External MCP
exposure before ownership and grants are solid.

Every one of these makes the product *wider*. The product does not need to be wider — it needs
to become usable, trustworthy, and askable. Breadth resumes after Stage 5.

## How we will know it worked

Not by shipped features. By four questions we can answer with current numbers at any time:

1. Does it find what you were looking for? *(measured retrieval accuracy and latency)*
2. Are the connections it shows real? *(typed edges with evidence — no "same mailbox")*
3. Did it actually deliver what it claims? *(delivery recorded only on acknowledgement)*
4. Can you leave with everything? *(export, relocate, delete — proven round-trip)*

A promise with no probe behind it gets labelled planned, not delivered.
