# Feature: 095 Retrieval-Strategy Routing + Freshness-Aware Retrieval

**Status:** done — delivered + certified (ceiling: `done`)
**Workflow Mode:** `full-delivery` (delivered: the 3 retrieval ideas are implemented + wired + certified `done`; off-by-default via SST)
**Release Train:** `next`
**Owner Directive (2026-06-17):**
> Borrow 3 genuine gap-closers from a "5 levels of an AI second brain" competitor-concept review, grounded in Smackerel's real architecture. Smackerel already operates at the top of that ladder (semantic search + knowledge graph + automated lifecycle); these are gap-closers, NOT re-implementations.

**Depends On (read-only substrate, MUST NOT modify under this planning spec):**
- spec 061 (conversational assistant — facade, scenario routing, provenance gate) — `internal/assistant/facade.go`, `internal/assistant/nl_routing.go`
- spec 068 (structured intent compiler — `CompiledIntent`, `ActionClass`) — `internal/assistant/intent/`
- spec 064 (open-ended knowledge agent — `AgentAnswer`, cite-back sources) — `internal/knowledge/agent_answer.go`
- spec 034 (expense tracking) + spec 021 (intelligence delivery: subscriptions/expenses aggregates) — `internal/intelligence/expenses.go`, `internal/intelligence/subscriptions.go`
- spec 025 (knowledge synthesis), spec 003 (ingestion pipeline + tiering) — `internal/pipeline/tier.go`, `internal/pipeline/ingest.go`
- spec 011/012/.../018 (the 17 committed connectors) — `internal/connector/*` (docs §22.7)

**Integrates With (consumes stable contracts; does not modify):**
- `internal/intelligence/cooling.go` — the §3.6 "LLM-driven judgment, SST operational bounds" architectural precedent this spec follows for the evergreen judgment.

**Unblocks:**
- The full-delivery implementation of scopes SCOPE-01..SCOPE-10 — now DELIVERED in this spec (`planningOnly: false`; the spec was promoted past its original planning ceiling) and certified `done`.

---

## 1. Problem Statement

Smackerel's retrieval pipeline ([docs/smackerel.md §9.2](../../docs/smackerel.md), lines ~1575-1593) is a **single, chunk-similarity-first path** for every query:

```
parse intent → embed query → vector similarity top-30 → graph-expand → LLM rerank → format
```

This one path silently produces **partial or wrong answers** for two query classes that the existing [§9.3 Query Types table](../../docs/smackerel.md) already names but the §9.2 pipeline does not branch for:

1. **Whole-document / complete-context recall.** "Summarize the whole March 5th meeting." Vector search pulls only the top-30 chunks that are *similar to the words in the query* (e.g. "summary", "meeting"), NOT the full transcript. The synthesized summary is therefore built from a biased subset and is partial. Yet Smackerel **already preserves the raw artifact** (Design Principle 2 — "Processed, not raw … Raw is preserved"); the complete context exists but the pipeline never fetches it.

2. **Aggregate / superlative over structured data.** "Which month did I spend the most on subscriptions?" Similarity grabs the *one chunk most similar to the question* and misses the actually-highest rows. Yet Smackerel **already has structured aggregates** — `internal/intelligence/expenses.go` (`ExpenseClassifier`, categories, vendor normalization) and `internal/intelligence/subscriptions.go` — that can answer the question exactly. The §9.3 table even classifies "what am I spending on subscriptions" as *"Type filter (bill/subscription) + aggregate"*, but the §9.2 pipeline routes it through vector similarity anyway.

A third, deeper gap underlies both: **ingestion is not shaped by the query shapes each artifact type must satisfy.** Connectors already declare *what source metadata they preserve* (Principle 4 — Source-Qualified Processing), but no artifact type declares *what query shapes it must answer* (transcript → whole-document summary; bill/subscription/expense → aggregate spend; place/trip → dossier aggregation). Without that declaration, the retrieval layer has nothing principled to route on.

Separately, **high-churn volatile data dilutes the permanent knowledge surface.** Smackerel decays topics *after the fact* via momentum scoring ([docs/smackerel.md §11.1](../../docs/smackerel.md); `internal/intelligence/cooling.go`; `internal/topics/lifecycle.go`). But raw chat noise and transient notifications enter the synthesis (§10) and digest (§12) candidate pools at full weight *before* any decay signal accrues, polluting the highest-value surfaces with volatile content.

### What is NOT the problem (anti-scope)

Smackerel is **already at the top of the second-brain ladder**: it has semantic search (pgvector), a unified knowledge graph, and automated lifecycle. The competitor concept's heterogeneous *separate* backends (folders + wikis + a vector store + a graph) are an **anti-pattern here** — they violate Principle 5 ("One Graph, Many Views"). This spec borrows the *routing idea*, not the parallel-store architecture.

---

## 2. Outcome Contract

**Intent:** Smackerel selects the **right retrieval strategy for the query's intent** and keeps **volatile content out of the permanent-knowledge surfaces** — both operating over the **single existing pgvector + knowledge-graph store**, never a new index.

Concretely:
- A query whose intent is *summarize-document / complete-context* fetches the **full preserved artifact** (not top-k chunks) and synthesizes from complete context.
- A query whose intent is *aggregate / superlative over structured data* runs a **structured (SQL) aggregate** over the existing structured tables (expenses / subscriptions / bills), not vector similarity.
- A query whose intent is *vague content recall* keeps **today's vector → graph-expand → LLM-rerank path** unchanged.
- The strategy is chosen from each artifact type's declared **retrieval contract** (the query shapes it must satisfy), and **every selection is traced and attributable** (Principle 8).
- Each artifact is scored for **evergreen-ness near the ingestion front door**; low-evergreen, high-churn items are routed to **aggressive decay** and **excluded from the synthesis and digest candidate pools**.

**Success Signal:**
- For a curated set of ≥15 whole-document queries ("summarize the whole X"), ≥80% route to the whole-document strategy and the synthesized answer cites the **full** artifact (not a chunk subset), verified against a labeled gold set.
- For a curated set of ≥15 aggregate/superlative queries over structured data, ≥80% route to the structured-aggregate strategy and return the **correct extremum/aggregate** (exact-match against a SQL ground-truth), where the legacy vector path returned a wrong or partial answer.
- For a curated set of ≥20 vague-recall queries, **100%** keep the existing vector+graph+rerank path (zero regression of today's >75%-first-result contract).
- For a labeled set of ≥20 artifacts split evergreen/ephemeral, the ingestion-front-door classifier agrees with the label ≥80%, and ≥95% of items labeled ephemeral are excluded from the synthesis and digest candidate pools.

**Hard Constraints:**
1. **Single knowledge graph (Principle 5).** All strategies read the **existing** pgvector + knowledge-graph + structured tables. NO new vector index, NO new database, NO parallel graph, NO duplicate search backend. New columns/tables are permitted ONLY to record routing/evergreen provenance that cannot be expressed on the existing schema.
2. **Vague In, Precise Out preserved (Principle 2).** Routing MUST NOT require the user to phrase queries precisely or name a strategy. Intent is inferred from the existing `CompiledIntent` ([internal/assistant/intent/types.go](../../internal/assistant/intent/types.go)); the user keeps typing vaguely.
3. **Source-qualified + transparent (Principles 4 & 8).** Every retrieval contract is grounded in declared source metadata; every strategy selection and every evergreen judgment is **traced with a reason and attributable** to the contract + intent that produced it. No silent routing.
4. **Knowledge breathes earlier (Principle 3).** The evergreen signal is an **earlier** lifecycle input, not a replacement for momentum decay. It sharpens §11 lifecycle; it does not fork it.
5. **NO-DEFAULTS / fail-loud SST.** Every new config value (intent-confidence routing threshold, strategy enablement flags, evergreen operational bounds) MUST originate from `config/smackerel.yaml` and fail loud if unset ([smackerel-no-defaults](../../.github/instructions/smackerel-no-defaults.instructions.md)). No hidden fallback defaults in source.
6. **Domain judgment is scenario/LLM-driven, not a hardcoded Go threshold (docs §3.6).** Following the `internal/intelligence/cooling.go` precedent, the evergreen-vs-ephemeral *judgment* is a scenario decision over retrieved signals; Go holds only **operational bounds** (confidence floor, per-tick budget, dedup window) as SST fail-loud knobs.
7. **Reuse the existing intent + facade substrate; do NOT fork it.** Routing extends the existing `CompiledIntent.ActionClass` + `nl_routing` seam through documented extension points. This planning spec authors ZERO edits to `internal/assistant/`, `internal/agent/`, `internal/pipeline/`, or `internal/intelligence/`; the implementation run owns those edits.

**Failure Condition:** A new parallel vector index or store is introduced (violates P5); OR routing demands precise user phrasing (violates P2); OR a strategy is selected without a traceable reason (violates P8); OR an evergreen cutoff is hardcoded in Go with a silent default (violates §3.6 + NO-DEFAULTS); OR the vague-recall path regresses below today's first-result contract.

---

## 3. Goals

- **G1 (Idea 1 — strategy routing).** A retrieval-strategy router that consumes the inferred query intent and the artifact-type retrieval contract, then selects one of: whole-document, structured-aggregate, or vague-recall (today's vector+graph+rerank). Default + low-confidence fallback is the vague-recall path (zero regression).
- **G2 (Idea 1a — whole-document strategy).** For complete-context intents, fetch the full preserved artifact and synthesize from it, instead of top-k chunk similarity.
- **G3 (Idea 1b — structured-aggregate strategy).** For aggregate/superlative intents over structured data, run a structured (SQL) aggregate over the existing expenses/subscriptions/bills tables and return the exact extremum/aggregate.
- **G4 (Idea 3 — retrieval contract).** A lightweight per-artifact-type retrieval contract declaring the query shapes each type must satisfy (transcript → whole-document summary; bill/subscription/expense → aggregate spend; place/trip → dossier). This contract DRIVES G1's routing.
- **G5 (Idea 2 — evergreen signal).** An evergreen-vs-ephemeral signal scored near the ingestion tier-assignment front door ([internal/pipeline/tier.go](../../internal/pipeline/tier.go)), scenario-driven per §3.6, that routes low-evergreen high-churn items to aggressive decay.
- **G6 (Idea 2 — pool exclusion).** Low-evergreen items are excluded from the §10 synthesis and §12 digest candidate pools so volatile content does not dilute the highest-value surfaces.
- **G7 (transparency).** Every strategy selection and every evergreen judgment carries a traceable reason (intent class + confidence + contract id, or evergreen score + signals) attributable end-to-end (Principle 8).

---

## 4. Non-Goals

- **A new vector index / store / graph.** Explicitly rejected (Principle 5). All strategies operate over the existing store.
- **Replacing momentum decay (§11).** The evergreen signal is additive and earlier; it does not delete or rewrite the cooling/lifecycle path.
- **Modifying the intent compiler (spec 068) or facade (spec 061) substrate.** Routing registers through documented extension points; substrate is read-only here.
- **New connectors or new structured-data domains.** Routing reuses what the 17 committed connectors and the existing expenses/subscriptions tables already ingest.
- **Per-user strategy preferences / a "choose your strategy" UI.** Strategy is inferred, never user-selected (Principle 1, Principle 2).
- **Financial advice / QF actions (Principle 10).** Aggregates over financial-markets or QF artifacts are descriptive recall only; no trade/mandate/execution output.
- **Implementation** *(was a planning-ceiling non-goal; SUPERSEDED — DELIVERED).* This started as a planning spec (`product-to-planning`, ceiling `specs_hardened`); it was promoted to `full-delivery` and the 3 ideas were implemented + wired + certified `done` (off-by-default via SST). Source/test/migration code is committed — see scopes.md DoD + report.md.

---

## 5. Domain Capability Model

This spec introduces a **capability foundation** — *intent-aware, freshness-aware retrieval over the single graph* — with multiple concrete strategies and a contract registry, so the capability-first triggers apply. The model is provider-/strategy-/type-neutral.

### 5.1 Domain primitives

| Primitive | Definition | Lifecycle / states |
|-----------|------------|--------------------|
| **RetrievalContract** | Per-artifact-type declaration of the query shapes that type must satisfy (e.g. `whole_document_summary`, `aggregate_spend`, `dossier`, `vague_recall`). Grounded in the type's declared source metadata (Principle 4). | declared at type-registration; versioned; read-only at query time |
| **QueryIntent** | The inferred interpretation of one inbound query — reuses the existing `CompiledIntent` (`ActionClass`, `Slots`, `Confidence`, `SourcePolicy`). | produced per turn by the existing intent compiler |
| **RetrievalStrategy** | A concrete retrieval path: `whole_document`, `structured_aggregate`, or `vague_recall`. Selected by the router from `(QueryIntent, RetrievalContract)`. | stateless; one selected per query |
| **StrategySelection** | The traced decision: which strategy, why (intent class + confidence + matched contract), and the fallback taken if confidence < threshold. | emitted per query; attributable (P8) |
| **EvergreenSignal** | A near-ingestion judgment of an artifact's durability (evergreen ↔ ephemeral), with the signals it was judged on and a reason. | scored at the ingestion front door; feeds lifecycle + pool eligibility |

### 5.2 Capability foundation (variation axes)

The foundation is the **RetrievalStrategyRouter** + the **RetrievalContract registry**. Concrete variation axes that MUST be expressible without forking the foundation:
1. **Strategy kind** — whole-document / structured-aggregate / vague-recall (extensible to future kinds, e.g. dossier).
2. **Artifact type → contract** — each of the existing artifact types maps to one or more required query shapes.
3. **Structured-aggregate backend** — expenses, subscriptions, bills (each an existing structured table; new backends added without router changes).
4. **Evergreen judgment source** — scenario/LLM judgment (canonical) vs deterministic signal extension to `TierSignals` (fallback), selected by SST.
5. **Pool eligibility** — synthesis pool, digest pool (each consults the evergreen signal independently).

### 5.3 Invariants

- A `StrategySelection` is **never** emitted without a reason and a confidence; below the SST confidence threshold the router MUST fall back to `vague_recall` (the safe default), and the fallback MUST be recorded.
- An `EvergreenSignal` is **never** a silent hardcoded cutoff: the judgment is scenario-driven; only operational bounds are SST values; the signals used are recorded.
- Every strategy reads the **same** store. A strategy that introduces a parallel index is rejected by an architecture test (SCOPE-03).

---

## 6. Requirements (technology-agnostic)

### Functional — Idea 1 (Retrieval-strategy routing)

- **R1.** The system MUST infer query intent from the existing `CompiledIntent` (`ActionClass`, `Slots`, `Confidence`) and select exactly one `RetrievalStrategy` per query.
- **R2.** When intent is *summarize-document / complete-context*, the system MUST select the **whole-document** strategy and retrieve the **full preserved artifact**, not top-k chunks.
- **R3.** When intent is *aggregate / superlative over structured data* (e.g. "which month did I spend the most on subscriptions"), the system MUST select the **structured-aggregate** strategy and compute the result via a structured query over the existing structured tables.
- **R4.** When intent is *vague content recall*, the system MUST keep today's **vector → graph-expand → LLM-rerank** path unchanged.
- **R5.** When the intent confidence is below the SST-configured routing threshold, the system MUST fall back to the `vague_recall` path and record the fallback reason (no guessing a riskier strategy on low confidence).
- **R6.** The router MUST consult the matched artifact-type **RetrievalContract** to decide which strategies are admissible for the queried type; a type with no admissible specialized contract resolves to `vague_recall`.

### Functional — Idea 3 (Per-artifact-type retrieval contract)

- **R7.** Each artifact type MUST be able to declare a **RetrievalContract** enumerating the query shapes it must satisfy (e.g. transcript → `whole_document_summary`; bill/subscription/expense → `aggregate_spend`; place/trip → `dossier`).
- **R8.** A RetrievalContract MUST be grounded in the type's declared source metadata (Principle 4); it MUST NOT invent query shapes the type cannot support.
- **R9.** A missing or unknown contract for a queried type MUST resolve **safely** to `vague_recall` (fail-safe routing), never error the user's query, and MUST be observable (Principle 8).

### Functional — Idea 2 (Evergreen-vs-ephemeral at the ingestion front door)

- **R10.** Each artifact MUST receive an **EvergreenSignal** near the ingestion tier-assignment front door, judged from retrieved signals (source kind, churn/volatility, content shape) — extending the existing tiering seam, not replacing it.
- **R11.** The evergreen *judgment* MUST be scenario/LLM-driven (docs §3.6 precedent); Go MUST hold only operational bounds (confidence floor, per-tick budget, dedup window) as SST fail-loud values.
- **R12.** Low-evergreen, high-churn artifacts MUST be routed to **aggressive decay** (sharpening §11 lifecycle) and MUST be **excluded from the §10 synthesis candidate pool and the §12 digest candidate pool**.
- **R13.** The evergreen signal MUST NOT block ingestion, retrieval, or search of the artifact — an ephemeral item remains fully searchable (Principle 9, no punishment); it is only de-prioritized from the permanent-synthesis/digest surfaces.

### Cross-cutting — single graph, transparency, SST

- **R14.** All strategies and the evergreen pool-eligibility checks MUST operate over the **existing** pgvector + knowledge-graph + structured tables. Introducing a parallel index/store/graph is a failure (Principle 5).
- **R15.** Every `StrategySelection` and `EvergreenSignal` MUST be **traceable** — carrying the intent class + confidence + matched contract (for routing) or the score + signals (for evergreen) — sufficient to explain the decision after the fact (Principle 8).
- **R16.** Every new configuration value MUST be declared in `config/smackerel.yaml` and resolved fail-loud; absence aborts startup with a named error (NO-DEFAULTS SST).

### Non-Functional

- **NFR-1.** The router decision adds bounded overhead to the existing reactive p95 latency budget; routing classification MUST reuse the already-computed `CompiledIntent` (no second LLM round-trip solely to route).
- **NFR-2.** The evergreen judgment runs within a bounded per-tick budget at ingestion and degrades gracefully (fallback to the deterministic `TierSignals` extension) when the scenario judge is unavailable, per Principle 9.
- **NFR-3.** Zero regression: the existing vague-recall path, the `nl_routing` deterministic rules, and the provenance gate behave exactly as today for all non-routed queries.

---

## 7. Representative Gherkin Scenarios

> These are representative user journeys, not the exhaustive test list. The plan (`scopes.md`) maps each `SCN-095-*` to concrete intended test files in `scenario-manifest.json`.

```gherkin
# --- Idea 1: Retrieval-strategy routing ---

Scenario: SCN-095-A01 — Query intent selects a retrieval strategy
  Given a user query and its inferred CompiledIntent with a confident action class
  When the retrieval-strategy router evaluates the intent against the artifact-type retrieval contract
  Then exactly one RetrievalStrategy is selected
  And a StrategySelection is emitted carrying the intent class, confidence, and matched contract id

Scenario: SCN-095-A02 — "Summarize the whole March 5th meeting" fetches the full transcript
  Given a transcript artifact whose retrieval contract declares whole_document_summary
  And a query whose intent is summarize-document / complete-context
  When the router selects a strategy
  Then the whole_document strategy is selected
  And the full preserved artifact is retrieved (not a top-k chunk subset)
  And the synthesized summary is grounded in the complete artifact

Scenario: SCN-095-A03 — "Which month did I spend the most on subscriptions?" runs a structured aggregate
  Given subscription/expense artifacts whose contract declares aggregate_spend
  And a query whose intent is aggregate / superlative over structured data
  When the router selects a strategy
  Then the structured_aggregate strategy is selected
  And the answer is computed by a structured query over the existing subscriptions/expenses tables
  And the returned extremum matches the SQL ground truth (not the single most-similar chunk)

Scenario: SCN-095-A04 — Vague recall keeps the existing vector+graph+rerank path
  Given a vague content-recall query ("that pricing video")
  When the router selects a strategy
  Then the vague_recall strategy is selected
  And the existing vector → graph-expand → LLM-rerank pipeline runs unchanged

Scenario: SCN-095-A05 — Low-confidence intent falls back to vague recall (no risky guess)
  Given a query whose CompiledIntent confidence is below the SST routing threshold
  When the router selects a strategy
  Then the vague_recall strategy is selected as the safe fallback
  And the StrategySelection records the low-confidence fallback reason

# --- Idea 3: Per-artifact-type retrieval contract ---

Scenario: SCN-095-C01 — Each artifact type declares the query shapes it must satisfy
  Given the artifact-type registry
  When a retrieval contract is read for a type
  Then it enumerates the query shapes that type must satisfy
  And each declared shape is grounded in the type's declared source metadata

Scenario: SCN-095-C02 — The router consults the contract to pick an admissible strategy
  Given an artifact type whose contract admits only vague_recall
  When an aggregate-intent query targets that type
  Then the router resolves to vague_recall (the contract does not admit structured_aggregate)
  And the resolution reason is recorded

Scenario: SCN-095-C03 — Unknown contract resolves safely to vague recall
  Given a queried type with no registered retrieval contract
  When the router selects a strategy
  Then it resolves to vague_recall without erroring the query
  And the missing-contract condition is observable

# --- Idea 2: Evergreen-vs-ephemeral at ingestion front door ---

Scenario: SCN-095-B01 — Artifact is scored evergreen vs ephemeral near ingestion
  Given an artifact arriving at the ingestion tier-assignment front door
  When the evergreen signal is computed from retrieved signals
  Then an EvergreenSignal is attached carrying the score, the signals used, and a reason

Scenario: SCN-095-B02 — High-churn ephemeral item is routed to aggressive decay
  Given an artifact judged low-evergreen / high-churn (e.g. transient chat noise)
  When the evergreen signal is applied to lifecycle
  Then the artifact is routed to aggressive decay
  And it remains fully searchable (not deleted or hidden)

Scenario: SCN-095-B03 — Low-evergreen item is excluded from the synthesis candidate pool
  Given an artifact judged low-evergreen
  When the synthesis engine assembles its candidate pool
  Then the low-evergreen artifact is excluded from synthesis candidacy

Scenario: SCN-095-B04 — Low-evergreen item is excluded from the digest candidate pool
  Given an artifact judged low-evergreen
  When the daily digest assembles its candidate pool
  Then the low-evergreen artifact is excluded from digest candidacy

Scenario: SCN-095-B05 — Evergreen judgment is scenario-driven; only bounds are SST
  Given the evergreen judgment configuration
  When the system resolves how to judge evergreen-ness
  Then the judgment is delegated to a scenario decision over retrieved signals
  And only operational bounds (confidence floor, per-tick budget, dedup window) are read from SST
  And no evergreen cutoff is hardcoded as a Go literal

# --- Cross-cutting: single graph, transparency, SST ---

Scenario: SCN-095-G01 — All strategies read the single existing store (no parallel index)
  Given the retrieval-strategy foundation
  When any strategy retrieves
  Then it reads the existing pgvector + knowledge-graph + structured tables
  And no new vector index, database, or graph is created (architecture test enforced)

Scenario: SCN-095-S01 — Missing routing/evergreen config aborts startup loudly
  Given config/smackerel.yaml omits a required retrieval-routing or evergreen key
  When the runtime starts and config validation runs
  Then startup aborts non-zero with a named missing-config error
  And no silent fallback default is substituted
```

---

## 8. Product Principle Alignment

Per [product-principles.instructions.md](../../.github/instructions/product-principles.instructions.md) this section is **binding and blocking**. Evidence: [docs/Product-Principles.md](../../docs/Product-Principles.md).

- **Principle 2 — Vague In, Precise Out.** This is the spec's core. Today the single chunk-similarity path returns *imprecise* output for whole-document and aggregate queries. Routing makes vague input ("summarize the whole meeting", "which month did I spend most") yield **precise** output, **without** asking the user to phrase the query precisely or name a strategy. The user keeps typing vaguely; the system infers and routes. (Evidence: §9.3 Query Types table already names the strategies; §1.4 success metric ">75% correct on first result for vague queries".)
- **Principle 3 — Knowledge Breathes.** Idea 2 adds an **earlier** lifecycle signal (evergreen-ness at ingestion) that sharpens the existing momentum-decay lifecycle (§11.1, `internal/intelligence/cooling.go`, `internal/topics/lifecycle.go`). Volatile content decays faster and never pollutes the permanent surfaces. The signal is additive to lifecycle, not a parallel mechanism.
- **Principle 4 — Source-Qualified Processing.** Idea 3's retrieval contracts are grounded in the *declared source metadata* the 17 connectors already preserve (§22.7). A type's admissible query shapes derive from the source signal, not from invented capability.
- **Principle 5 — One Graph, Many Views.** Routing is a **view-selection** layer over the **single** existing graph + pgvector + structured tables. The competitor concept's parallel stores (folders/wikis/vector/graph) are **explicitly rejected**; an architecture test (SCOPE-03) fails any strategy that introduces a parallel index. The structured-aggregate strategy queries the *existing* expenses/subscriptions tables — already part of the one store — not a new analytics DB.
- **Principle 8 — Trust Through Transparency.** Every `StrategySelection` and every `EvergreenSignal` is traced with a reason (intent class + confidence + contract, or score + signals), so the user (and the audit trail) can always see **why** a strategy or an exclusion was chosen. No silent routing, no silent pool exclusion.

This feature initiates **no** financial action and carries **no** QF companion packet (Principle 10 — descriptive recall only over financial artifacts; not applicable to routing/lifecycle).

**No principle deviations.** No requirement requires user organization at capture (P1 respected), adds a notification (P6 respected — routing is pull-only), or produces long-form output (P7 respected — strategies feed the existing phone-screen-fit formats).

---

## 9. Release Train

**Target train:** `next` (the staging promotion-candidate train — charter "synthesis + multi-source coordination"; see [config/release-trains.yaml](../../config/release-trains.yaml)). The active `mvp` home-lab train is frozen for new specs (see [docs/releases/mvp/features.md](../../docs/releases/mvp/features.md)), so this NEW spec targets `next` — consistent with `state.json.releaseTrain` and the V7 row in [docs/releases/v1/features.md](../../docs/releases/v1/features.md).

This spec enhances always-on retrieval + lifecycle surfaces. **No new feature flag is required** — the routing and evergreen behaviors are gated by their SST keys (`retrieval.routing.*`, `retrieval.evergreen.*`), which live in `config/smackerel.yaml`. `state.json.flagsIntroduced` is therefore `[]`. Because no flag is introduced, there is **no default-off-on-other-trains toggle**: the new SST keys are REQUIRED in `config/smackerel.yaml` (the single source of truth all trains derive from), so behavior is identical on every train (`next` as the owning train and `mvp` as a non-owning train alike) — each reads the same SST contract.

---

## 10. Open Questions

> Resolved during the planning chain (analyst → ux → design → plan). Pointers are to the resolving artifact section.

| OQ | Question | Owner | Status |
|----|----------|-------|--------|
| OQ-1 | Where does the strategy router sit — inside the facade turn or as a pre-retrieval stage consumed by `retrieval_qa`? | bubbles.design | RESOLVED — see [design.md §3](design.md) |
| OQ-2 | Is the structured-aggregate strategy a new scenario+tool or a reuse of the existing intelligence aggregates? | bubbles.design | RESOLVED — see [design.md §5](design.md) |
| OQ-3 | Storage shape for the RetrievalContract registry (config vs migration vs in-code registry)? | bubbles.design | RESOLVED — see [design.md §4](design.md) |
| OQ-4 | Evergreen judgment: scenario-driven vs deterministic `TierSignals` extension as the canonical path? | bubbles.design | RESOLVED — see [design.md §6](design.md) |
| OQ-5 | Status language + transparency surface for routing/evergreen decisions (non-UI UX)? | bubbles.ux | RESOLVED — see §14 |
| OQ-6 | Disclosure shape when the router falls back / a contract is missing? | bubbles.ux | RESOLVED — see §14 |
| OQ-7 | Final SST key set + values, scope decomposition, scenario→test mapping? | bubbles.plan | RESOLVED — see [scopes.md](scopes.md) |

---

## 11. Routing Note (substrate boundary)

This planning spec authors ZERO source/test/migration edits. The implementation run that consumes this packet MUST register the router + strategies + evergreen signal through **documented extension points** and MUST NOT modify the spec 061 facade, spec 068 intent compiler, spec 003 pipeline core, or spec 021/025 intelligence substrate beyond the additive seams design.md names. Any required substrate change is routed as a packet to the owning spec at implementation time (design.md §"Routing").

---

## 14. UX — Workflow Behavior, Status Language, Transparency (non-UI)

> Authored by bubbles.ux. Spec 095 adds **no new app screen**. Per the canonical UX-as-workflow-behavior precedent (spec 061 §14, spec 063 §14), this section defines the behavioral contract: status language, transparency/disclosure shape, fallback copy, and latency budget. Every recommendation is grounded in a product principle; numeric thresholds are **bubbles.ux recommendations to bubbles.design**, which owns final SST key names and values.

### 14.A Status language (closed vocabulary)

The routing/freshness layer introduces four trace tokens (observability + audit, not user-facing chrome), each with an actionability bar per Principle 6:

| Token | Meaning | Surfaced where |
|-------|---------|----------------|
| `strategy_selected` | The router chose a strategy for this query | trace / audit only (invisible by default) |
| `strategy_fallback` | Confidence below threshold OR contract missing → vague_recall fallback | trace / audit; surfaced to the user ONLY as the normal answer (no apology, no chrome) |
| `evergreen_scored` | An artifact received an evergreen signal at ingestion | trace / audit only |
| `pool_excluded` | A low-evergreen artifact was excluded from a synthesis/digest pool | trace / audit only |

These are **felt, not heard** (Principle 6): the user sees a better answer, not a routing notification.

### 14.B Transparency / disclosure (Principle 8)

- A `StrategySelection` is **always** recorded with its reason; the *user-facing* disclosure is minimal — when the whole-document or structured-aggregate strategy is used, the answer keeps its existing source attribution (the full artifact citation, or the structured-table provenance), which already tells the user the answer came from the complete source / the exact figures. No new "I routed this to strategy X" banner.
- When the router falls back (`strategy_fallback`), there is **no** user-facing apology or downgrade notice; the vague-recall answer is returned normally (Principle 9 — no punishment, no anxiety). The fallback reason lives in the trace for the audit/operator surface only.
- The evergreen exclusion is **never** surfaced to the user as "we hid this" — the item stays searchable (Principle 9). Exclusion is visible only in the operator/audit trace (`pool_excluded` with the score + signals).

### 14.C Refusal / fail-safe copy

- Missing retrieval contract → **silent safe fallback** to vague_recall (R9). No error to the user. Internal-only observable.
- Structured-aggregate over financial artifacts → the answer is **descriptive recall only** and, where the existing financial surfaces require it, carries the existing non-advice framing (Principle 10). The router never adds advice copy.

### 14.D Latency budget

- Routing MUST reuse the already-computed `CompiledIntent` and add no second LLM round-trip solely to route (NFR-1). The reactive p95 budget (< 5s, matching spec 061/062/063 §14.G) is a **binding ceiling**; strategy selection MUST stay inside it.

### 14.E Honesty declarations

- Every numeric threshold named here (routing confidence threshold, evergreen confidence floor, per-tick budget) is a **bubbles.ux recommendation**; bubbles.design owns the final SST key names and authoritative values per smackerel-no-defaults. UX commits no SST nomenclature.
- The "scenario-driven evergreen judgment" recommendation is grounded in the observable `internal/intelligence/cooling.go` precedent (docs §3.6), not a UX preference.
