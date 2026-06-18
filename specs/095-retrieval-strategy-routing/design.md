# Design — Spec 095 Retrieval-Strategy Routing + Freshness-Aware Retrieval

> **Status:** authored by bubbles.design 2026-06-17. Owns: technical design,
> capability-foundation split, SST key naming, module layout, OQ-1/2/3/4
> resolutions, the anti-parallel-store decision, the architecture-test surface,
> and the substrate contract boundary.
>
> **Reads-only (substrate, MUST NOT modify under this planning spec):**
> spec 061 (`internal/assistant/facade.go`, `internal/assistant/nl_routing.go`,
> `internal/assistant/provenance/`), spec 068 (`internal/assistant/intent/`),
> spec 064 (`internal/knowledge/agent_answer.go`), spec 021/034
> (`internal/intelligence/expenses.go`, `internal/intelligence/subscriptions.go`),
> spec 003/025 (`internal/pipeline/`, `internal/intelligence/synthesis.go`,
> `internal/topics/lifecycle.go`), the 17 connectors (`internal/connector/*`).
>
> **spec.md sections owned by other agents:** §1–§13 (analyst), §14 UX (UX).
> Design binds SST key names in §10; spec.md proposals were non-binding.

---

## 0. Design Brief (alignment checkpoint)

### Current state (grounded — every claim cites real code)

- **The retrieval pipeline is a single chunk-similarity-first path.** [docs/smackerel.md §9.2](../../docs/smackerel.md) (lines ~1575-1593): `parse intent → embed query → vector top-30 → graph-expand → LLM rerank → format`. There is exactly one path; nothing branches on query class.
- **The §9.3 Query Types table already enumerates the strategies — but only as documentation.** [docs/smackerel.md §9.3](../../docs/smackerel.md) lists "Location-scoped → aggregate trip dossier" and "Meta/self-knowledge → Type filter (bill/subscription) + aggregate". The strategies are *named* but the §9.2 pipeline never selects them.
- **An intent contract already exists.** [internal/assistant/intent/types.go](../../internal/assistant/intent/types.go): `CompiledIntent{ActionClass, Slots, Confidence, ScenarioHint, SourcePolicy, ...}` with the closed `ActionClass` vocabulary (`answer`, `retrieve`, `external_lookup`, `internal_action`, `state_mutation`, `clarify`, `capture_only`, `refuse`). Spec 068 produces this for every turn **before** routing — the router consumes it; it does NOT re-classify.
- **A deterministic facade routing seam already exists.** [internal/assistant/nl_routing.go](../../internal/assistant/nl_routing.go): `LookupNLRouting` maps "find me X" → the `retrieval_qa` scenario. This is the precedent extension point: a deterministic, observable routing rule at the facade layer.
- **A whole-document / synthesized-answer path already exists.** [internal/knowledge/agent_answer.go](../../internal/knowledge/agent_answer.go): `AgentAnswer` persists open-knowledge agent turns with cite-back `AgentAnswerSource{Kind: web|artifact|tool_computation}`. The full-artifact citation primitive exists.
- **Structured aggregates already exist to route to.** [internal/intelligence/expenses.go](../../internal/intelligence/expenses.go) (`ExpenseClassifier`, categories, vendor normalization over a `pgxpool.Pool`) and [internal/intelligence/subscriptions.go](../../internal/intelligence/subscriptions.go). These compute exact spend aggregates from structured tables today.
- **An ingestion tier-assignment front door already exists.** [internal/pipeline/tier.go](../../internal/pipeline/tier.go): `AssignTier(TierSignals{UserStarred, SourceID, HasContext, ContentLen}) → {full,standard,light,metadata}`. [internal/pipeline/ingest.go](../../internal/pipeline/ingest.go) `PublishRawArtifact` calls `resolveTierFromMetadata(...)` at the front door. This is the exact seam the evergreen signal extends.
- **The "LLM-driven judgment, SST operational bounds" pattern is established.** [internal/intelligence/cooling.go](../../internal/intelligence/cooling.go): "domain reasoning is LLM-driven, not encoded as fixed thresholds in Go … The Go core retrieves candidate signals; the scenario decides." `CoolingConfig` holds only operational bounds (`MaxCandidates`, `ConfidenceFloor`, `DedupWindowDays`), all SST-resolved fail-loud. This is docs §3.6, and it is the template for the evergreen judgment.
- **Lifecycle decay already exists.** [docs/smackerel.md §11.1](../../docs/smackerel.md) momentum scoring; [internal/topics/lifecycle.go](../../internal/topics/lifecycle.go); `internal/intelligence/cooling.go`. The evergreen signal is an **earlier input** to this, not a replacement.

### Target state

A **RetrievalStrategyRouter** capability foundation that, for each query, consumes the already-computed `CompiledIntent` + the queried type's **RetrievalContract**, and selects one **RetrievalStrategy**:
- `whole_document` — fetch the full preserved artifact (reuses the `agent_answer` full-artifact citation primitive), synthesize from complete context.
- `structured_aggregate` — delegate to the existing intelligence aggregates (`expenses`/`subscriptions`) over the existing structured tables.
- `vague_recall` — today's §9.2 vector → graph-expand → rerank path, unchanged (default + low-confidence fallback).

Plus an **EvergreenSignal** computed at the `AssignTier` front door (scenario-driven judgment per §3.6; deterministic `TierSignals` extension as the graceful fallback), feeding lifecycle decay and pool eligibility for §10 synthesis + §12 digests.

### Patterns to follow
- **Deterministic facade routing rule** from `nl_routing.go` — the router is observable and stable, mirroring `LookupNLRouting`.
- **Consume `CompiledIntent`, do not re-classify** — spec 068 already produced the intent (NFR-1: no second LLM round-trip to route).
- **Reuse the intelligence aggregates** for `structured_aggregate` — `expenses.go`/`subscriptions.go` already compute exact figures; the strategy calls them, it does not re-implement SQL.
- **Extend the `AssignTier` seam** for the evergreen signal — add a signal at the existing front door, not a new pipeline stage.
- **Scenario-driven judgment + SST operational bounds** from `cooling.go` (§3.6) — the canonical evergreen judgment path.

### Patterns to avoid (anti-patterns — BLOCKING)
- ❌ **A new/parallel vector index, database, search backend, or graph.** This is the competitor concept's heterogeneous-store architecture and it **violates Principle 5**. The structured-aggregate strategy queries the **existing** expenses/subscriptions tables (already part of the one store) — NOT a new analytics DB or OLAP cube. An architecture test (SCOPE-03) fails any strategy package that opens a second store/index. (See §2.)
- ❌ **A hardcoded evergreen cutoff in Go** (`if churn > 0.7 { ephemeral }`). Violates §3.6 + NO-DEFAULTS. The judgment is scenario-driven; only bounds are SST.
- ❌ **Re-classifying intent inside the router** — duplicates spec 068, costs a second LLM call, violates NFR-1.
- ❌ **Forcing precise user phrasing** to trigger a strategy — violates Principle 2; intent is inferred.
- ❌ **Blocking/hiding ephemeral artifacts** from search — violates Principle 9; exclusion is pool-eligibility only.

### Resolved decisions (this design owns)
- **OQ-1:** Router is a **pre-retrieval stage** consumed by the `retrieval_qa` scenario path, selected via a deterministic facade rule mirroring `nl_routing.go`. §3.
- **OQ-2:** `structured_aggregate` **reuses** the existing `expenses`/`subscriptions` intelligence aggregates through a thin strategy adapter; no new SQL engine. §5.
- **OQ-3:** RetrievalContract registry is an **in-code registry seeded from SST-declared type→shape mappings** (config is the SST source; the registry is the typed read model). §4.
- **OQ-4:** Evergreen judgment is **scenario-driven (canonical)** with a deterministic `TierSignals` extension as the graceful fallback (Principle 9, NFR-2). §6.
- **SST keys:** `retrieval.routing.*` + `retrieval.evergreen.*`, every key REQUIRED at startup, fail-loud. §10.

### Open questions remaining (bubbles.plan-owned)
- OQ-PLAN-1 (final routing-confidence + evergreen-bound values), OQ-PLAN-2 (migration need for provenance columns), OQ-PLAN-3 (scenario→test file mapping), OQ-PLAN-4 (which artifact types ship a specialized contract in v1 vs default to vague_recall). All resolved in [scopes.md](scopes.md).

---

## 1. The anti-parallel-store decision (Principle 5 — explicit)

The borrowed competitor concept shows **separate heterogeneous backends** (folders + wikis + a vector store + a graph), each answering a different query class. Smackerel **rejects** that architecture because Principle 5 ("One Graph, Many Views") requires all artifacts to live in **one** graph; cross-domain connections only work when everything is co-located.

Therefore:
- **Idea 1 is implemented as retrieval STRATEGY ROUTING over the EXISTING store** — pgvector embeddings, the knowledge graph, and the existing structured tables (expenses/subscriptions/bills). The three strategies are three *read paths* over the *same* data, not three stores.
- **Idea 2 is implemented as a FRESHNESS SIGNAL on the EXISTING lifecycle** — an earlier input to the §11 momentum/cooling machinery, plus a pool-eligibility predicate. It creates **no** new store and forks **no** lifecycle.
- **Idea 3 is a per-type CONTRACT (a read model)** describing query shapes; it persists at most a small provenance/registry surface, never a duplicate of artifact data.

This decision is enforced mechanically by the SCOPE-03 architecture test `TestNoParallelStore` (and its `would_catch_regression` adversarial sub-test).

---

## 2. Capability foundation

Per the capability-first design triggers, the foundation is two collaborating pieces:

1. **`RetrievalStrategyRouter`** — pure decision function `select(intent, contract) → StrategySelection`. Stateless; no I/O; trivially testable; emits a traced selection.
2. **`RetrievalContract` registry** — `contractFor(artifactType) → RetrievalContract` (the admissible query shapes for a type).

Concrete strategies (`whole_document`, `structured_aggregate`, `vague_recall`) are **overlays** behind a `RetrievalStrategy` interface. The router selects; an executor dispatches to the selected overlay. Architecture tests (SCOPE-03) assert: no overlay opens a second store; every overlay reads the existing store; the router never re-classifies intent.

```
proposed module layout (implementation run authors these; NOT this planning spec):

internal/retrieval/routing/
  contract.go          # RetrievalContract type + in-code registry (seeded from SST)
  strategy.go          # RetrievalStrategy interface + StrategyKind closed vocabulary
  router.go            # RetrievalStrategyRouter.select(intent, contract) → StrategySelection
  selection.go         # StrategySelection trace type (P8)
  architecture_test.go # TestNoParallelStore, TestRouterDoesNotReclassify, TestReadsExistingStoreOnly (+ adversarial sub-tests)
internal/retrieval/routing/strategies/
  wholedocument/       # full-artifact fetch (reuses knowledge.AgentAnswerSource artifact citation)
  structuredaggregate/ # thin adapter over internal/intelligence/{expenses,subscriptions}
  vaguerecall/         # thin adapter over the existing §9.2 vector+graph+rerank path
internal/retrieval/evergreen/
  signal.go            # EvergreenSignal type + scenario-judged + TierSignals-fallback
  pool_eligibility.go  # synthesis/digest pool predicate
  architecture_test.go # TestEvergreenJudgmentNotHardcoded, TestEphemeralStaysSearchable
internal/config/retrieval.go  # SST struct + fail-loud validation for retrieval.routing.* / retrieval.evergreen.*
```

> The path layout is a **design proposal** for the implementation run; the planning ceiling authors no source. SCOPE numbering below is independent of final package names.

---

## 3. OQ-1 RESOLVED — Router placement (pre-retrieval stage)

The router sits as a **pre-retrieval stage** in front of the `retrieval_qa` path, selected by a deterministic facade rule that mirrors `LookupNLRouting` ([nl_routing.go](../../internal/assistant/nl_routing.go)). Rationale:
- The facade already owns the `CompiledIntent` for the turn; the router consumes it with zero extra LLM cost (NFR-1).
- A deterministic, observable rule (not a similarity guess) keeps routing **stable and auditable** (Principle 8) — exactly the property `nl_routing.go`'s comment cites ("a deterministic facade rule makes the routing observable and stable").
- The facade and intent compiler are **read-only substrate**; the router registers through the documented `nl_routing`-style extension seam at implementation time (packet to spec 061 if a facade hook is required).

## 4. OQ-3 RESOLVED — RetrievalContract registry shape

The registry is an **in-code typed read model seeded from SST-declared type→shape mappings**:
- `config/smackerel.yaml` `retrieval.routing.contracts` declares, per artifact type, the admissible query shapes (e.g. `transcript: [whole_document_summary, vague_recall]`, `subscription: [aggregate_spend, vague_recall]`, `place: [dossier, vague_recall]`).
- At startup the config is validated fail-loud and loaded into the typed `RetrievalContract` registry. Unknown types resolve to `[vague_recall]` (R9 fail-safe).
- No migration is required for the registry itself (it is config-derived); a small provenance surface for `StrategySelection`/`EvergreenSignal` traces is the only possible storage need (OQ-PLAN-2, resolved in scopes.md).

This honors NO-DEFAULTS (the mapping is SST, fail-loud) AND keeps the contract grounded in source metadata (Principle 4 — the admissible shapes derive from what the connector preserves).

## 5. OQ-2 RESOLVED — structured_aggregate reuses existing aggregates

The `structured_aggregate` strategy is a **thin adapter** over the existing `internal/intelligence/expenses.go` + `subscriptions.go` aggregates. It does NOT introduce a new SQL/OLAP engine. The adapter:
- Maps the aggregate/superlative intent + slots (period, category, extremum) onto the existing aggregate calls.
- Returns the exact computed figure with structured-table provenance (Principle 8).
- For financial-markets/QF artifacts, returns descriptive recall only with the existing non-advice framing (Principle 10).

This keeps the structured path inside the **one store** (the existing tables) — no analytics DB (Principle 5).

## 6. OQ-4 RESOLVED — Evergreen judgment is scenario-driven (canonical) + deterministic fallback

Following the `cooling.go` precedent (docs §3.6):
- **Canonical path:** the Go front door retrieves evergreen *signals* (source kind, churn/volatility, content shape, structured-vs-prose) and a scenario decides `evergreen ↔ ephemeral` with a confidence. Go applies only the **operational bounds** from SST (`confidence_floor`, `per_tick_budget`, `dedup_window_days`).
- **Graceful fallback (NFR-2, Principle 9):** when the scenario judge is unavailable, a deterministic extension to `TierSignals` (e.g. transient source kinds → ephemeral) produces a conservative signal so ingestion never blocks. The fallback is recorded in the trace.
- **No hardcoded cutoff** (SCOPE-07 architecture test `TestEvergreenJudgmentNotHardcoded` + adversarial sub-test).
- The signal **only** affects lifecycle decay weighting and pool eligibility; it never blocks ingestion/search (R13, Principle 9).

---

## 10. SST keys — final fail-loud required set

Per [smackerel-no-defaults](../../.github/instructions/smackerel-no-defaults.instructions.md), every key below is REQUIRED at startup; a missing/empty/out-of-range value aborts startup with `[F095-SST-MISSING] missing or invalid required retrieval configuration: <key>`. NO key has an in-source fallback default. Values shown are bubbles.plan's stamped starting values (OQ-PLAN-1), operator-overridable, NOT in-code literals.

```yaml
retrieval:
  routing:
    enabled: true
    intent_confidence_threshold: 0.65   # below → vague_recall fallback (R5)
    strategies:
      whole_document: { enabled: true }
      structured_aggregate: { enabled: true }
      vague_recall: { enabled: true }     # default; cannot be disabled (safe fallback)
    contracts:                            # type → admissible query shapes (Idea 3, R7/R8)
      transcript:   [whole_document_summary, vague_recall]
      meeting:      [whole_document_summary, vague_recall]
      subscription: [aggregate_spend, vague_recall]
      expense:      [aggregate_spend, vague_recall]
      bill:         [aggregate_spend, vague_recall]
      place:        [dossier, vague_recall]
      trip:         [dossier, vague_recall]
      # any type absent here resolves to [vague_recall] (R9 fail-safe)
  evergreen:
    enabled: true
    judgment_source: scenario            # scenario (canonical) | tier_signals (deterministic fallback)
    confidence_floor: 0.60               # operational bound (NOT a business cutoff)
    per_tick_budget: 50                  # ingestion-tick judgment cap (NFR-2)
    dedup_window_days: 7                 # re-judge dedup window
    pools:
      synthesis_excludes_low_evergreen: true   # R12
      digest_excludes_low_evergreen: true      # R12
```

`vague_recall.enabled` is structurally pinned `true` (the router's safe fallback must always exist); config validation rejects `false` with a named error. `judgment_source` is a closed vocabulary; an unrecognized value is rejected at startup.

---

## 11. Storage / provenance shape (OQ-PLAN-2 input)

The router + contract registry are config-derived (no migration). The only candidate storage is a **trace/provenance surface** for `StrategySelection` and `EvergreenSignal` (Principle 8). Design recommends recording these as **structured trace events on the existing observability/trace path** (mirroring spec 071 intent-trace), NOT a new artifact table — keeping Principle 5 intact. If a durable column is needed (e.g. `artifacts.evergreen_score`), it is an **additive column on the existing `artifacts` table**, never a sibling store. bubbles.plan finalizes the migration decision (OQ-PLAN-2) in scopes.md.

---

## 12. Architecture-test surface

| Test | Asserts | Scope |
|------|---------|-------|
| `TestNoParallelStore` | No routing/strategy/evergreen package opens a second DB pool, vector index, or graph store | SCOPE-03 |
| `TestRouterDoesNotReclassify` | The router consumes `CompiledIntent`; it does not call the intent compiler / ML sidecar to re-classify (NFR-1) | SCOPE-03 |
| `TestReadsExistingStoreOnly` | Every strategy adapter reads the existing pgvector/graph/structured tables | SCOPE-03 |
| `TestEvergreenJudgmentNotHardcoded` | No Go literal evergreen cutoff; judgment routes to the scenario or the SST-selected fallback | SCOPE-07 |
| `TestEphemeralStaysSearchable` | An ephemeral artifact is excluded from pools but remains retrievable/searchable (R13, P9) | SCOPE-08 |

Each carries a `would_catch_regression` adversarial sub-test (per [bubbles-test-integrity](../../.github/skills/bubbles-test-integrity/SKILL.md)).

---

## 13. Contract boundary (read-only substrate)

Spec 095 (implementation run) consumes these **stable contracts read-only** and registers through documented extension points; any required substrate change is a routed packet at implementation time:
- `CompiledIntent` ([intent/types.go](../../internal/assistant/intent/types.go)) — consumed; never modified.
- `LookupNLRouting` seam ([nl_routing.go](../../internal/assistant/nl_routing.go)) — extension precedent for the router rule.
- `AssignTier`/`TierSignals` ([pipeline/tier.go](../../internal/pipeline/tier.go)) — extension seam for the evergreen signal.
- `ExpenseClassifier` / subscriptions aggregates — called by the structured strategy adapter; never modified.
- `internal/topics/lifecycle.go` + `cooling.go` — the lifecycle the evergreen signal feeds; consumed, not forked.

---

## 15. Product-principle design evidence

- **P2:** routing infers from `CompiledIntent`; user phrasing stays vague. Whole-doc + structured-aggregate make vague input precise.
- **P3:** evergreen is an earlier lifecycle input feeding the existing momentum/cooling machinery.
- **P4:** contracts derive admissible shapes from declared source metadata.
- **P5:** ENFORCED by `TestNoParallelStore`; structured strategy uses existing tables; evergreen forks no lifecycle.
- **P8:** `StrategySelection` + `EvergreenSignal` are traced with reasons; routing/exclusions are auditable.
- **P9:** ephemeral items stay searchable; fallback returns a normal answer with no apology.
- **P10:** financial aggregates are descriptive recall only with existing non-advice framing.

## 16. Open questions remaining (bubbles.plan)

- OQ-PLAN-1 — final `intent_confidence_threshold` + evergreen bound values (stamped in §10; calibration deferred to the implementation run).
- OQ-PLAN-2 — migration need for an additive `artifacts.evergreen_score` column vs trace-only provenance.
- OQ-PLAN-3 — scenario→test file mapping (`scenario-manifest.json`).
- OQ-PLAN-4 — which artifact types ship a specialized contract in v1 vs default to `vague_recall`.
