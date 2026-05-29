# Design — Spec 063 Knowledge AI Enrichment

> **Status:** authored by bubbles.design 2026-05-29. Owns: technical design,
> capability foundation split, SST key naming, module layout, OQ-1/2/3/4/8/9
> resolutions, storage shape decisions, architecture-test surface, and the
> contract boundary with spec 062.
>
> **Reads-only (substrate, MUST NOT modify):** spec 021 (`internal/intelligence/`),
> spec 025 (`internal/intelligence/synthesis.go`, `internal/knowledge/`),
> spec 026 (`internal/extract/`, `ml/app/{domain,synthesis}.py`), spec 037
> (`internal/agent/`), spec 061 (`internal/assistant/facade.go`,
> `internal/assistant/provenance/`), spec 054 (`internal/notification/`).
>
> **spec.md sections owned by other agents:** §1–§13 (analyst), §14 UX (UX).
> Design recommendations bind SST key names in §10 below; spec.md §12 proposals
> were non-binding.

---

## 0. Design Brief (alignment checkpoint)

**Current state.** Smackerel already has every substrate spec 063 needs:
- **Conversational facade** ([internal/assistant/facade.go](../../internal/assistant/facade.go), spec 061) routes inbound turns through scenario YAML manifests ([config/prompt_contracts/](../../config/prompt_contracts/)) and tool handlers ([internal/agent/tools/](../../internal/agent/tools/)). A provenance gate (`internal/assistant/provenance/`) refuses scenarios with empty `sources[]`.
- **Agent runtime** (`internal/agent/{router,executor,registry,nats_driver,tracer}.go`, spec 037, terminal `done`) executes scenarios. New tools register via `init()` under `internal/agent/tools/<name>/`.
- **Heuristic knowledge layer** ([internal/intelligence/synthesis.go](../../internal/intelligence/synthesis.go), spec 025) produces `ConceptPage`s via a SQL `GROUP BY` on `BELONGS_TO` edges. Production-stable; UX §14.C declares it the durable canonical signal.
- **Cron producers** ([internal/intelligence/alert_producers.go](../../internal/intelligence/alert_producers.go), [briefs.go](../../internal/intelligence/briefs.go), spec 021) emit alerts/recommendations/briefs heuristically with `source_artifact_ids` set, but no "why" prose.
- **Graph edges** ([001_initial_schema.sql](../../internal/db/migrations/001_initial_schema.sql) lines 122-137) — generic `(src_type, src_id, dst_type, dst_id, edge_type, weight, metadata JSONB)` with `UNIQUE(src_type, src_id, dst_type, dst_id, edge_type)`. Existing types: `BELONGS_TO`, `ENTITY_RELATES_TO_CONCEPT`, `CONTRADICTS`. The schema is already polymorphic enough to host `INFERRED_*` types without migration of existing rows.

**Target state.** A **knowledge-enrichment producer** capability foundation that hosts five concrete surfaces (`resynthesis`, `relationship_inference`, `why_augmenter`, `consolidation_analyzer`, reactive `knowledge_lookup`) as either scenario+producer pairs (reactive surface) or producer-only cron jobs (background surfaces). Reactive entry reuses the spec 061 facade verbatim. Background producers reuse the spec 021 cron-producer pattern and invoke the LLM through the spec 037 agent runtime (never direct Ollama HTTP). LLM outputs are stored alongside heuristic outputs with `INFERRED_*` provenance — never overwriting (UX §14.C).

**Patterns to follow.**
- **Scenario+tool pair** from [internal/agent/tools/retrieval/](../../internal/agent/tools/retrieval/) + [config/prompt_contracts/retrieval-qa-v1.yaml](../../config/prompt_contracts/retrieval-qa-v1.yaml). `knowledge_lookup` mirrors this exactly: one scenario YAML, one tool handler package, one `SourceAssembler` adapter.
- **Source-assembler pattern** from [facade_assembler.go](../../internal/agent/tools/retrieval/facade_assembler.go) — the reactive scenario MUST implement `contracts.SourceAssembler` so the facade's provenance gate refuses empty `sources[]`.
- **Cron-producer pattern** from [alert_producers.go](../../internal/intelligence/alert_producers.go) — each background producer is an independent `Engine` method, fire-or-skip per tick, bounded by `LIMIT N`. No chained dependencies; failure of one producer never silently disables another.
- **Edge insert pattern** from [internal/graph/linker.go](../../internal/graph/linker.go) line 394 — `INSERT INTO edges ... ON CONFLICT ... DO UPDATE SET weight = $7, metadata = $8`. Spec 063 reuses verbatim; `metadata JSONB` carries `{llm_run_id, prompt_contract_version, confidence, evidence_artifact_ids}`.
- **Migration conventions** — sequential numbered SQL files in [internal/db/migrations/](../../internal/db/migrations/), `IF NOT EXISTS` guards, `CHECK` constraints on enum-like text columns. Next number: **045** (062 reserves 044–046 if/when implemented; design.md establishes the contract that spec 063 takes the lowest free number at implementation time).
- **LLM invocation through agent runtime** — producers compose a scenario request and call the executor; tracer/driver/registry handle everything else.

**Patterns to avoid.**
- A parallel orchestrator under `internal/knowledge/enrichment/` re-implementing scenario routing. The facade owns reactive routing; producers own only the "should we enqueue?" decision plus the `executor.Run` call.
- Direct `ml/` HTTP calls or direct `ollama` HTTP. The agent runtime already abstracts driver selection.
- Mutating spec 025's `synthesis.go` heuristic path. Heuristic remains canonical per UX §14.C.
- Mutating heuristic `edges` rows (`BELONGS_TO`, `ENTITY_RELATES_TO_CONCEPT`, `CONTRADICTS`) — only **appending** new rows with `INFERRED_*` `edge_type`.
- A separate `inferred_edges` table — would force every consumer to UNION two sources and break P5 (one graph, many views).
- A single shared `enrichment-v1.yaml` parameterized by producer — per-producer prompts (§9) are the dominant pattern in `config/prompt_contracts/` (18 files, all per-task).

**Resolved decisions** (this design owns).
- OQ-1: hybrid event-driven enqueue + cron drain. §4.
- OQ-2: per-surface confidence floors with high-cost defaults; one SST key per producer. §5.
- OQ-3: dedicated sibling `enrichment_why` table keyed by `(parent_kind, parent_id)`. §6.
- OQ-4: reuse `edges` table with `INFERRED_*` `edge_type` taxonomy in `metadata`. §7.
- OQ-8: `retrieval_search` and `knowledge_lookup` coexist; router picks on intent (recall vs synthesis). §8.
- OQ-9: per-producer prompt contracts, one YAML per surface. §9.
- SST keys: `enrichment.*` namespace, every key REQUIRED at startup, fail-loud per [smackerel-no-defaults](../../.github/instructions/smackerel-no-defaults.instructions.md). §10.
- Token-cost mechanism: global daily ledger in new `enrichment_token_ledger` table; 80%-soft / 100%-hard / cheap-also-exceeds → refuse. §11.
- Refusal contract: reuse spec 061 provenance gate on reactive; structured `refusal_reason` + Prometheus counter on background. §12.

**Open questions remaining.** OQ-PLAN-1 (per-tick budget calibration), OQ-PLAN-2 (consolidation analyzer retention policy), OQ-PLAN-3 (architecture-test placement), OQ-PLAN-4 (candidate-pair selector SQL), OQ-PLAN-5 (NATS publication points in foreign substrate). All bubbles.plan-owned. §16.

---

## 1. Capability Surface

Spec 063 has two surfaces. The reactive surface is conversational; the background surface is silent.

### 1.1 Reactive path (user-initiated)

```
user turn → facade (spec 061) → router → knowledge_lookup scenario
        → executor → tool: knowledge_lookup_search
        → SourceAssembler → provenance gate → reply
```

- **Entry point:** spec 061 `facade.go`. UNTOUCHED.
- **New scenario:** `config/prompt_contracts/enrichment-knowledge-lookup-v1.yaml`. Registers through the existing manifest pattern (`config/assistant/scenarios.yaml` sibling row addition only — sibling manifest, not facade code).
- **New tool handler:** `internal/agent/tools/enrichment/knowledgelookup/` — package with `init()` registering one tool. Mirrors `internal/agent/tools/retrieval/`.
- **Source assembler:** `internal/agent/tools/enrichment/knowledgelookup/facade_assembler.go` translates `{answer, cited_concept_page_ids, cited_artifact_ids}` into `[]contracts.Source` for the provenance gate. Empty sources ⇒ gate refuses ⇒ canonical refusal body (spec 061 §14.A.7 + spec 063 §14.F).
- **Latency:** binds UX §14.G p95 < 5s. Model selection MUST keep the round trip inside the ceiling; see §11.

### 1.2 Background path (system-initiated, silent)

```
trigger source (NATS event / topic-edit / cron tick)
        → enrichment producer (one per surface)
        → builds closed evidence set
        → executor.Run(<surface-scenario-id>, evidence)
        → output parser
        → persist (concept_pages | edges | enrichment_why | consolidation_candidates)
        → metrics + trace span
```

- **Entry point:** new package `internal/knowledge/enrichment/`. Sibling to `internal/intelligence/`, NOT inside `internal/agent/`.
- **Per-surface producers:**
  - `resynthesis.go` (re-synthesizes affected concept pages)
  - `relationship_inference.go` (proposes `INFERRED_*` edges)
  - `why_augmenter.go` (attaches prose to alerts/recommendations/briefs)
  - `consolidation_analyzer.go` (suggests merge/keep-separate on topic edits)
- **Scheduling:** existing `internal/scheduler/` ticker; each producer registers an independent job. Producers MAY also be enqueued event-driven via NATS (§4).
- **Notifications:** ZERO. Per Hard Constraint #3, R-13, UX §14.A. Spec 063 background producers never invoke `internal/notification/`.

---

## 2. Capability Foundation (DE4)

Spec 063 introduces 5 enrichment surfaces. Per [bubbles-capability-foundation-design](../../.github/skills/bubbles-capability-foundation-design/SKILL.md) the "≥2 concrete implementations" trigger applies; the foundation MUST be made explicit.

### 2.1 Foundation contract

The Go interface:

```go
// internal/knowledge/enrichment/producer.go
package enrichment

// EnrichmentProducer is the foundation contract. Every concrete background
// surface (resynthesis, relationship_inference, why_augmenter,
// consolidation_analyzer) implements this interface. The reactive
// knowledge_lookup surface does NOT implement this interface — it lives
// behind the facade as a scenario+tool pair (§1.1) and shares only the
// EvidenceSet + ProvenanceRecord types.
type EnrichmentProducer interface {
    // Name is the producer identifier, used for metrics, logs, and SST keys
    // (enrichment.producers.<Name>.*). Closed vocabulary:
    // "resynthesis" | "relationship_inference" | "why_augmenter" | "consolidation_analyzer".
    Name() string

    // Enqueue translates a trigger into 0..N jobs. Producer-specific logic
    // (which concept pages overlap with this artifact? which (a, b) pairs
    // are candidates? which alert just emitted?) lives here.
    Enqueue(ctx context.Context, trigger Trigger) ([]Job, error)

    // RunJob executes one job: builds the closed EvidenceSet, invokes the
    // executor with the producer's scenario id, parses output, and returns
    // either an Output or a Refusal. MUST NOT persist; persistence is the
    // caller's responsibility (separation enables dry-run + testing).
    RunJob(ctx context.Context, job Job) (Result, error)

    // ApplyOutput persists the output (or records the refusal) atomically.
    // MUST be a single transaction; partial writes are forbidden (NFR-4).
    ApplyOutput(ctx context.Context, result Result) error

    // DrainBacklog catches up after an offline period, bounded by the
    // producer's backlog_cap. Oldest-first by trigger timestamp.
    DrainBacklog(ctx context.Context, budget int) (processed int, err error)
}
```

### 2.2 Shared types (foundation-owned)

```go
type Trigger struct {
    Kind       TriggerKind // artifact_arrived | artifact_mutated | artifact_removed |
                           // topic_edited | topic_merged | alert_emitted |
                           // recommendation_emitted | brief_emitted | scheduler_tick_catchup
    TargetID   string      // artifact_id | topic_id | alert_id | (none for tick)
    TargetKind string
    OccurredAt time.Time
}

type Job struct {
    ProducerName string
    TargetKind   string  // "concept_page" | "edge_candidate" | "alert" | "topic_pair" | "reactive_query"
    TargetID     string
    EvidenceSet  EvidenceSet
    EnqueuedAt   time.Time
}

type EvidenceSet struct {
    ArtifactIDs    []string  // closed set; LLM citations outside ⇒ refusal
    ConceptPageIDs []string
}

type Result struct {
    Job        Job
    Output     *Output  // nil iff refused
    Refusal    *Refusal // nil iff applied
    Provenance ProvenanceRecord
}

type ProvenanceRecord struct {
    PromptContractVersion  string
    LLMRunID               string  // ulid; same id used in agent_traces
    ModelID                string  // e.g. "gemma3:4b"
    EvidenceArtifactIDs    []string
    EvidenceConceptPageIDs []string
    Confidence             float64 // [0.0, 1.0]
}

type Refusal struct {
    Reason string // closed: "would_thin_existing_content" | "below_confidence_floor" |
                  // "insufficient_evidence" | "evidence_set_violation" |
                  // "daily_token_budget_exhausted"
}
```

### 2.3 Concrete implementations

| Concrete | Trigger sources | Input shape | Output target | Scenario YAML |
|---|---|---|---|---|
| `ResynthesisProducer` | `artifact_arrived`, `artifact_mutated`, `artifact_removed`, `scheduler_tick_catchup` | affected `concept_page_id` + union of its `source_artifact_ids` ∪ new artifact | UPDATE `concept_pages` on no-thinning-pass; refusal otherwise | `enrichment-resynthesis-v1.yaml` |
| `RelationshipInferenceProducer` | `scheduler_tick_catchup` (with candidate-pair selector query) | one candidate `(src_artifact_id, dst_artifact_id)` pair with both summaries | INSERT `edges` row, `edge_type ∈ {INFERRED_RELATED, INFERRED_COREFERENCE, INFERRED_TEMPORAL_SEQUENCE}` | `enrichment-relationship-inference-v1.yaml` |
| `WhyAugmenterProducer` | `alert_emitted`, `recommendation_emitted`, `brief_emitted` (NATS) | parent row id + its `source_artifact_ids` + parent body | INSERT `enrichment_why` row | `enrichment-why-augmenter-v1.yaml` |
| `ConsolidationAnalyzerProducer` | `topic_edited`, `topic_merged` | affected topic-pair `(t1_id, t2_id)` + union of their artifacts | INSERT `consolidation_candidates` row | `enrichment-consolidation-analyzer-v1.yaml` |
| `knowledge_lookup` (NOT EnrichmentProducer) | facade routing | conversational query + retrieved concept pages/artifacts | reply envelope (no persistence beyond `agent_traces`) | `enrichment-knowledge-lookup-v1.yaml` |

### 2.4 Variation axes (≥ 2 required by DE4)

| Axis | Variation across surfaces |
|---|---|
| **Trigger shape** | Event-driven (resynthesis, why_augmenter, consolidation_analyzer), cron-only (relationship_inference), reactive facade (knowledge_lookup). |
| **Output storage** | UPDATE of existing row (resynthesis → `concept_pages`), INSERT into existing polymorphic table (relationship_inference → `edges`), INSERT into new sibling table (why_augmenter → `enrichment_why`, consolidation_analyzer → `consolidation_candidates`), no persistence (knowledge_lookup → reply only). |
| **Refusal user-visibility** | User-facing structured refusal (knowledge_lookup, consolidation reactive ask), silent log+metric (resynthesis thinning, relationship floor, why empty-evidence). Per UX §14.F table. |
| **Confidence-floor scale** | High floor (relationship_inference 0.70 — pollutes graph permanently), high floor (consolidation_analyzer 0.75 — affects user's topic graph), low-medium floor (why_augmenter 0.50 — prose adds context, easy to ignore), no floor (resynthesis — gated by no-thinning guard instead, §5). |
| **Authorship model** | Reactive facade scenario (knowledge_lookup) vs scheduled background producer (other four). |

### 2.5 Single-implementation justification

N/A — five concrete implementations. Foundation is required, not optional. The `knowledge_lookup` reactive surface deliberately does NOT implement `EnrichmentProducer`: forcing the reactive scenario through a background-producer abstraction would create a fake mode (no `Enqueue`, no `DrainBacklog`, no `ApplyOutput`). Sharing only the value-object types (`EvidenceSet`, `ProvenanceRecord`) is the honest split.

---

## 3. Module Layout

```
internal/knowledge/enrichment/                  # NEW package (sibling to internal/intelligence/)
    producer.go             # EnrichmentProducer interface + Trigger/Job/Result/Refusal types
    evidence.go             # EvidenceSet construction + citation-set-violation check
    provenance.go           # ProvenanceRecord helpers; ulid LLMRunID minting
    scheduler.go            # Per-tick driver: pulls triggers from NATS + cron; fan-out to producers
    nats_subjects.go        # NATS subject constants: "enrichment.trigger.artifact_arrived" etc.
    refusal.go              # Refusal taxonomy + structured logger + metrics + UX §14.F copy constants
    token_budget.go         # §11 budget gate; reads enrichment_token_ledger
    resynthesis.go          # ResynthesisProducer impl (R-1..R-3)
    resynthesis_test.go
    relationship_inference.go   # RelationshipInferenceProducer (R-4, R-5)
    relationship_inference_test.go
    why_augmenter.go        # WhyAugmenterProducer (R-6, R-7)
    why_augmenter_test.go
    consolidation_analyzer.go   # ConsolidationAnalyzerProducer (R-8)
    consolidation_analyzer_test.go
    architecture_test.go    # §13 architecture tests

internal/agent/tools/enrichment/                # NEW; spec 037 init() extension point
    knowledgelookup/
        tool.go             # registers "knowledge_lookup_search" tool via init()
        tool_test.go
        facade_assembler.go # SourceAssembler implementation
        facade_assembler_test.go
        source_assembly.go  # cited_*_ids → []contracts.Source translator
        source_assembly_test.go

config/prompt_contracts/                         # NEW YAMLs (one per surface; §9)
    enrichment-resynthesis-v1.yaml
    enrichment-relationship-inference-v1.yaml
    enrichment-why-augmenter-v1.yaml
    enrichment-consolidation-analyzer-v1.yaml
    enrichment-knowledge-lookup-v1.yaml

config/assistant/scenarios.yaml                  # APPEND ONE ROW (sibling manifest;
                                                 # registers knowledge_lookup with facade router)

internal/db/migrations/                          # NEW migration (lowest free number at impl time)
    NNN_knowledge_enrichment.sql                 # enrichment_why, consolidation_candidates,
                                                 # enrichment_token_ledger; INFERRED_* edge_type
                                                 # values are NOT a schema change (free-form TEXT).

config/smackerel.yaml                            # NEW enrichment.* block (§10)

internal/config/enrichment.go                    # NEW; mirrors internal/config/assistant.go; validates §10 keys; fail-loud

internal/metrics/counters.go                     # APPEND enrichment_* counters (§12 observability)
```

**Files NOT touched by spec 063 (substrate freeze):**
- `internal/agent/{router,executor,registry,nats_driver,tracer}.go` — spec 037 terminal `done`.
- `internal/assistant/facade.go` — spec 061 substrate.
- `internal/intelligence/synthesis.go` — spec 025 heuristic; remains durable canonical.
- `internal/intelligence/alert_producers.go`, `briefs.go`, etc. — spec 021. why_augmenter reads alert rows but never modifies them.
- `internal/extract/*`, `ml/app/{domain,synthesis}.py` — spec 026 ingest path.
- Any spec 062 artifact (planning complete, stashed).

---

## 4. RESOLVES OQ-1: Re-synthesis trigger granularity

**Decision: Hybrid (event-driven enqueue + cron drain).**

Rationale grounded in existing patterns:

- **Event-driven enqueue** mirrors the way spec 021 producers emit to NATS post-ingest. When an artifact arrives, mutates, or is removed, the ingest pipeline publishes to a NATS subject `enrichment.trigger.artifact_arrived` (etc.) carrying `{artifact_id, occurred_at}`. The `ResynthesisProducer` subscribes and immediately runs `Enqueue` to compute affected `concept_page_id`s (SQL: `SELECT concept_page_id FROM ... WHERE artifact_id IN source_artifact_ids`). Jobs are placed on an in-memory bounded queue.
- **Cron drain** mirrors the spec 021 cron ticker pattern. The scheduler fires `ResynthesisProducer.RunJob` at `enrichment.producers.resynthesis.cadence_seconds` cadence, pulling up to `enrichment.producers.resynthesis.per_tick_budget` jobs. This is also the entry point for `DrainBacklog` after restart.
- **Why hybrid:** pure event-driven risks thundering herd on bulk-ingest (e.g., Twitter archive import = 12k artifacts); pure cron is laggy (user sees stale concept pages for hours). Hybrid: enqueue is cheap (queue push), drain is rate-limited.

**Trigger sources per producer:**

| Producer | Event source (NATS subject) | Cron tick role |
|---|---|---|
| `resynthesis` | `enrichment.trigger.artifact.{arrived,mutated,removed}` | drain queue; backlog catch-up |
| `relationship_inference` | none (no event signal indicates a candidate pair is novel) | full driver: candidate-pair selector query + batch processing |
| `why_augmenter` | `enrichment.trigger.intelligence.{alert,recommendation,brief}_emitted` | drain queue; catch-up after restart for parent rows whose `enrichment_why` row is missing |
| `consolidation_analyzer` | `enrichment.trigger.topic.{edited,merged}` | drain queue; catch-up |

**Publisher boundary:** spec 026 ingest path, spec 021 producers, and spec 025 topic mutation path are READ-ONLY substrate for spec 063. If the existing pipeline does not already publish at these points, the plan-phase MUST surface a route-required packet to the owning spec (NOT silently add publishers). See OQ-PLAN-5.

**Bounded queue:** in-memory bounded channel, capacity = SST `enrichment.queue.capacity` per producer. Overflow drops oldest with structured log + Prometheus counter `enrichment_queue_overflow_total{producer}`. Backlog persistence beyond process restart is provided by the catch-up scan (R-2), NOT by an on-disk queue.

---

## 5. RESOLVES OQ-2: Confidence floor calibration

**Decision: per-surface confidence floors with high defaults for graph-mutating surfaces.**

False-positive cost rationale: knowledge-graph pollution is permanent and degrades every downstream consumer (spec 025 queries, spec 062 forward-looking, future UX). Pollution cost dominates miss cost. Recommended initial values (SST-tunable):

| Producer | Initial floor | Rationale |
|---|---|---|
| `relationship_inference` | **0.70** | Permanent `INFERRED_*` edges. Pollution is high cost. Conservative. |
| `consolidation_analyzer` | **0.75** | Highest — affects user's topic graph (user edits propagate through every retrieval). |
| `why_augmenter` | **0.50** | Prose attached to existing alerts. Low pollution risk (parent row already exists; missing/poor prose is graceful). |
| `resynthesis` | N/A — gated by **no-thinning guard** (R-3) and citation-set-violation check, not a numeric floor. Confidence is recorded but does not gate. | Re-synthesis output is gated structurally: must not thin, must cite only in-set IDs. Floor would be redundant. |
| `knowledge_lookup` | N/A — gated by **spec 061 provenance gate** (empty `sources[]` ⇒ refuse). | Reactive surface; refusal is structural per spec 061 §3. |

Every floor is a per-producer SST key (§10) — operator MUST tune calibration data over time. NO hardcoded floors; missing key ⇒ startup fail-loud.

---

## 6. RESOLVES OQ-3: Storage shape for "why" prose

**Decision: dedicated sibling table `enrichment_why` keyed by `(parent_kind, parent_id)`.**

Rejected alternatives:

- **Add column on each intelligence row** (alerts, recommendations, briefs) — requires migrating 3+ tables, foreign-spec mutation (`internal/intelligence/` schema touch routes a packet to spec 021), and forces every consumer of those rows to handle a nullable LLM column they don't care about.
- **JSON blob on parent row** — same migration cost; loses query-ability of provenance fields.

Sibling table is clean:
- Single migration owned by spec 063.
- Parent rows untouched.
- Provenance fields are first-class columns (queryable by operator).
- Supersession (re-augment after model swap) is a soft delete + insert pattern, not a JSON-field mutation.

```sql
-- internal/db/migrations/NNN_knowledge_enrichment.sql (extract)
CREATE TABLE IF NOT EXISTS enrichment_why (
    id                       TEXT PRIMARY KEY,                          -- ulid
    parent_kind              TEXT NOT NULL CHECK (parent_kind IN ('alert','recommendation','brief')),
    parent_id                TEXT NOT NULL,                             -- references alerts.id | recommendations.id | briefs.id
    prose                    TEXT NOT NULL,
    confidence               REAL NOT NULL CHECK (confidence >= 0.0 AND confidence <= 1.0),
    evidence_artifact_ids    TEXT[] NOT NULL,                           -- closed set; ⊆ parent.source_artifact_ids
    prompt_contract_version  TEXT NOT NULL,
    llm_run_id               TEXT NOT NULL,
    model_id                 TEXT NOT NULL,
    superseded_at            TIMESTAMPTZ,                                -- non-null iff a newer row replaced this
    superseded_by            TEXT REFERENCES enrichment_why(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (parent_kind, parent_id, llm_run_id)
);
CREATE INDEX IF NOT EXISTS idx_enrichment_why_parent ON enrichment_why(parent_kind, parent_id) WHERE superseded_at IS NULL;
```

Consumer pattern: any UI/notification surface reading an alert joins `LEFT JOIN enrichment_why ON ... WHERE superseded_at IS NULL` to surface the latest non-superseded prose. Missing row ⇒ render parent without "why" line (per UX §14.F: spec 063 emits nothing user-facing on why_augmenter refusal).

---

## 7. RESOLVES OQ-4: Storage shape for inferred relationships

**Decision: reuse existing `edges` table with `INFERRED_*` `edge_type` taxonomy; provenance lives in `metadata JSONB`.**

Per UX §14.C: heuristic edges are durable canonical; LLM edges are additive provenance. Per P5: one knowledge graph, many views. A sibling `inferred_edges` table would force every consumer (spec 025 retrieval, spec 062 forward-looking, future UX) to UNION two sources — violates P5.

**Taxonomy (closed vocabulary for spec 063):**

| `edge_type` | Semantic | Direction | Example |
|---|---|---|---|
| `INFERRED_RELATED` | Generic "these are related but heuristic clustering didn't catch it" | undirected (insert one row, conventional `src < dst` ordering by ULID lex) | two recipes sharing an unusual ingredient combination not in heuristic topics |
| `INFERRED_COREFERENCE` | "These two entities/artifacts are the same real-world referent" | undirected | "Acme Corp" mention in email and "ACME, Inc." in receipt resolved to same vendor |
| `INFERRED_TEMPORAL_SEQUENCE` | "Artifact A precedes / causes B" | directed (src → dst) | a research-note artifact followed by a recipe artifact, LLM infers the recipe was the result |

Migration adds NO new column to `edges`; the schema already accommodates arbitrary `edge_type` values (TEXT). Migration adds an index for fast filtering of inferred edges:

```sql
CREATE INDEX IF NOT EXISTS idx_edges_type_inferred
    ON edges(edge_type)
    WHERE edge_type LIKE 'INFERRED_%';
```

`metadata JSONB` shape for inferred rows (foundation-enforced; rejected on insert if shape invalid):

```json
{
  "llm_run_id": "01H...",
  "prompt_contract_version": "enrichment-relationship-inference-v1",
  "model_id": "deepseek-r1:7b",
  "confidence": 0.82,
  "evidence_artifact_ids": ["art_01...", "art_02..."],
  "evidence_concept_page_ids": []
}
```

`weight REAL` column carries confidence as a second representation (denormalized intentionally — enables `ORDER BY weight DESC` without JSONB extraction). Insert path SET `weight = confidence`.

**Heuristic untouched guarantee:** Architecture test §13 forbids any `UPDATE edges` or `DELETE FROM edges` from `internal/knowledge/enrichment/` against rows where `edge_type NOT LIKE 'INFERRED_%'`. Spec 063 only INSERTs `INFERRED_*` rows (and may UPDATE its own `INFERRED_*` rows on re-run with a bumped `llm_run_id`).

---

## 8. RESOLVES OQ-8: Reactive scenario boundary with spec 061 `retrieval_search`

**Decision: `retrieval_search` and `knowledge_lookup` coexist; router picks on user intent.**

| Tool / scenario | Returns | When the router picks it |
|---|---|---|
| `retrieval_qa` (spec 061, via `retrieval_search` tool) | raw artifact rows, summarized by the scenario LLM at query time, citations = artifact IDs | **exact recall** — "what did I save about X?", "find my notes on Y", "did I capture anything on Z?" |
| `knowledge_lookup` (spec 063, new) | LLM-synthesized answer over `concept_pages` (read), with `retrieval_search` as a subroutine for raw-artifact backfill, citations = concept-page IDs + artifact IDs | **synthesized knowledge** — "what do I know about X?", "summarize what I've learned about Y", "tell me about my Z" |

**Overlap-avoidance rule:** the new scenario's `intent_examples` block deliberately uses noun-phrase synthesis verbs ("what do I know about", "summarize what I've learned", "tell me about"). The existing `retrieval_qa` `intent_examples` use recall verbs ("what did I save", "find", "remind me"). Router uses spec 037 intent classification; no router code changes (extension only via new scenario manifest row).

**knowledge_lookup composition:** the scenario's `allowed_tools` includes the existing `retrieval_search` tool (read-only reuse — no fork). The LLM's prompt directs it to (1) look up `concept_pages` matching the noun phrase, (2) optionally call `retrieval_search` for artifacts not already cited in those concept pages, (3) synthesize citing both. `SourceAssembler` translates both cited-id sets into `[]contracts.Source`. Provenance gate refuses empty.

**Disambiguation:** if the router cannot decide (intent score gap below threshold), the facade's existing disambiguation prompt (spec 061 §14.B.1) surfaces — spec 063 does NOT add new disambiguation logic.

---

## 9. RESOLVES OQ-9: Per-producer prompt contracts vs shared

**Decision: per-producer prompt contracts, one YAML per surface (5 new files).**

Grounded in existing pattern: [config/prompt_contracts/](../../config/prompt_contracts/) holds 18 YAMLs, every one task-specific (`receipt-extraction-v1`, `recipe-extraction-v1`, `recommendation-why-v1`, `notification-schedule-v1`, etc.). Zero shared-parameterized contracts. The spec 062 design (read earlier) also chose per-thread contracts.

Rationale:
- **Clarity:** each contract documents one task's `input_schema`, `output_schema`, `system_prompt`, `allowed_tools`, `model_preference`, `limits` in one file.
- **Independent evolution:** a `why_augmenter` prompt iteration does NOT churn the `resynthesis` contract.
- **Per-surface tuning:** `model_preference`, `temperature`, `token_budget`, `timeout_ms` differ materially between surfaces (relationship inference wants reasoning depth → `deepseek-r1:7b`; why_augmenter wants speed → `gemma3:4b`). Shared parameterization would force a matrix of overrides.
- **DRY cost is marginal:** the shared boilerplate (~10 lines of YAML preamble) does not justify the cost of indirection.

The five new contracts (one each per producer + `knowledge_lookup`) are listed in §3.

---

## 10. SST Keys — final fail-loud REQUIRED set

Per [.github/instructions/smackerel-no-defaults.instructions.md](../../.github/instructions/smackerel-no-defaults.instructions.md), every key below MUST be REQUIRED at startup with no fallback. Validation lives in `internal/config/enrichment.go` (new file, mirrors `internal/config/assistant.go` pattern), wired into the master validator chain. Missing/empty value ⇒ `[F063-SST-MISSING] missing or invalid required enrichment configuration: <field>` ⇒ process exits non-zero.

```yaml
# config/smackerel.yaml (new block; appended)
enrichment:
  global_enabled: <bool>                          # master kill-switch; false ⇒ no producers run, no scenario registered

  queue:
    capacity: <int>                               # per-producer in-memory queue capacity (§4)

  disclosure:                                     # UX §14.B
    staleness_minutes: <int>                      # disclosure footer threshold A (recommended 60)
    backlog_threshold: <int>                      # disclosure footer threshold B (recommended 100)

  daily_token_budget: <int>                       # UX §14.E; global daily LLM-token cap
  cap_reset_timezone: <string>                    # IANA tz id (e.g. "Europe/Amsterdam"); calendar-day boundary

  refusal:
    min_sources_required: <int>                   # knowledge_lookup minimum retrieval-floor sources before LLM call (UX §14.F first row M value)

  producers:
    resynthesis:
      enabled: <bool>
      cadence_seconds: <int>                      # cron drain cadence
      per_tick_budget: <int>                      # max jobs per tick
      backlog_cap: <int>                          # max jobs across full catch-up after restart
      prompt_contract_version: <string>           # e.g. "enrichment-resynthesis-v1"
      model_provider: <string>                    # e.g. "gemma3:4b"
      no_thinning_guard_enabled: <bool>           # MUST be true at launch; gate is non-negotiable per R-3 but key forces operator awareness

    relationship_inference:
      enabled: <bool>
      cadence_seconds: <int>
      per_tick_budget: <int>
      backlog_cap: <int>
      confidence_floor: <float>                   # [0.0, 1.0]; recommended 0.70
      candidate_selector_limit: <int>             # SQL LIMIT for candidate-pair selector per tick
      prompt_contract_version: <string>
      model_provider: <string>                    # recommended "deepseek-r1:7b"

    why_augmenter:
      enabled: <bool>
      cadence_seconds: <int>
      per_tick_budget: <int>
      backlog_cap: <int>
      confidence_floor: <float>                   # recommended 0.50
      prompt_contract_version: <string>
      model_provider: <string>                    # recommended "gemma3:4b"

    consolidation_analyzer:
      enabled: <bool>
      cadence_seconds: <int>
      per_tick_budget: <int>
      backlog_cap: <int>
      confidence_floor: <float>                   # recommended 0.75
      prompt_contract_version: <string>
      model_provider: <string>

  reactive:
    knowledge_lookup:
      enabled: <bool>                             # facade registration gated on this
      prompt_contract_version: <string>
      model_provider_primary: <string>            # recommended "gemma3:4b" (p95 < 5s per UX §14.G)
      model_provider_fallback: <string>           # UX §14.E disclosed downgrade; same as primary if no separate fallback
      latency_budget_ms: <int>                    # binding ceiling; producer logs warn at 80%
```

**Forbidden shapes (re-stated):**
- `os.Getenv("ENRICHMENT_X", "fallback")` — no Go fallback strings.
- `${ENRICHMENT_X:-default}` — no shell fallback.
- `if cfg.X == 0 { cfg.X = N }` — no silent post-load defaulting.
- Optional/pointer fields where the operator could omit the key — every key non-optional.

---

## 11. Token-Cost Budget Mechanism (UX §14.E)

**Decision: persistent daily ledger in new `enrichment_token_ledger` table; soft cap at 80%, hard cap at 100%, cheap-also-exceeds ⇒ refuse.**

Rejected: in-memory counter (loses state on restart; budget would silently reset → operator surprise).

```sql
CREATE TABLE IF NOT EXISTS enrichment_token_ledger (
    ledger_day      DATE NOT NULL,                  -- calendar day in enrichment.cap_reset_timezone
    surface         TEXT NOT NULL,                  -- producer Name() | "knowledge_lookup"
    model_id        TEXT NOT NULL,
    tokens_consumed BIGINT NOT NULL DEFAULT 0,
    job_count       BIGINT NOT NULL DEFAULT 0,
    refusal_count   BIGINT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (ledger_day, surface, model_id)
);
```

**Budget gate algorithm (in `internal/knowledge/enrichment/token_budget.go`):**

```
func (g *Gate) Admit(ctx, surface, estimated_tokens) (Decision, error):
    day := today_in(cfg.enrichment.cap_reset_timezone)
    used := SUM(tokens_consumed) over all (day, *, *) rows
    budget := cfg.enrichment.daily_token_budget
    remaining := budget - used

    if remaining < estimated_tokens_for_cheap_model:
        return REFUSE{reason: "daily_token_budget_exhausted"}
    if used >= 0.80 * budget:
        return DOWNGRADE{model: cfg.enrichment.reactive.knowledge_lookup.model_provider_fallback}
    return PROCEED{model: configured_primary_for_surface}
```

`estimated_tokens` is a per-prompt-contract upper bound declared in the YAML (`token_budget:` field — already an existing convention; see `retrieval-qa-v1.yaml` `token_budget: 1200`). After job completion, `tokens_consumed` is incremented by the actual token count from the agent runtime's tracer span (existing field). Budget consumption is post-hoc; the gate uses estimates for the admission decision and reconciles on completion.

**Reactive surface UX integration:** when `DOWNGRADE` fires, the `knowledge_lookup` scenario sets a flag on the reply envelope; the facade renders the §14.E footer (`note: using fast model (daily budget threshold reached); answer may be terser.`). When `REFUSE` fires, the scenario returns the canonical §14.E refusal body and the facade routes through spec 061 capture-as-fallback.

**Background surface integration:** producers consult the gate before invoking the executor; `REFUSE` ⇒ job recorded as refused with `refusal_reason=daily_token_budget_exhausted` + Prometheus counter increment. No retry; next tick re-evaluates after midnight reset.

**Cap reset:** the ledger query filters `WHERE ledger_day = today_in(tz)`; tomorrow's first call sees an empty sum ⇒ remaining = full budget. No cron job needed; the day rollover is implicit in the date filter.

**Reactive p95 guard (UX §14.G):** the budget gate query MUST hit a covering index and complete in single-digit ms. The cached view of "today's used tokens" MAY be held in-process with a 30s TTL — disclosed here so the architecture-test layer can assert the cache is invalidated on every job completion (otherwise budget could be exceeded silently).

---

## 12. Refusal Contract (UX §14.F)

**Decision: spec 061 provenance gate handles reactive refusal; background refusals are structured `Refusal` records + Prometheus counters.**

| Surface | Refusal mechanism | User-visible? |
|---|---|---|
| `knowledge_lookup` reactive | (1) Min-sources gate (`enrichment.refusal.min_sources_required`) — checked BEFORE LLM call, cheap precondition. Empty retrieval ⇒ scenario returns `{answer:"", cited_*:[]}`. (2) `SourceAssembler` returns zero-value `SourceAssembly`. (3) Spec 061 provenance gate refuses on empty `sources[]`. (4) Facade renders canonical refusal body (spec 063 §14.F first row copy). | YES — structured body. |
| Reactive consolidation ask (read-only over `consolidation_candidates`) | If `SELECT ... WHERE confidence >= floor ORDER BY confidence DESC LIMIT N` returns zero rows: facade renders §14.F second-row refusal body with last-run-Xm + edit-count from `enrichment_token_ledger` + topic-edit event log. | YES — structured body. |
| `resynthesis` no-thinning | `Refusal{Reason:"would_thin_existing_content"}`; Prometheus `enrichment_refusal_total{producer="resynthesis", reason="would_thin_existing_content"}`. | NO — logs+metrics only. |
| `relationship_inference` below floor | `Refusal{Reason:"below_confidence_floor"}`; counter. | NO — logs+metrics only. |
| `why_augmenter` empty evidence / set violation | `Refusal{Reason:"insufficient_evidence" \| "evidence_set_violation"}`; counter. Parent row remains without `enrichment_why` row. | NO — parent surface renders without "why" line. |
| Budget-exhausted (reactive or background) | `Refusal{Reason:"daily_token_budget_exhausted"}`; counter; reactive surfaces render §14.E refusal body. | reactive: YES; background: NO. |

**Reuse of spec 061 provenance gate:** the `SourceAssembler` adapter (§3 `internal/agent/tools/enrichment/knowledgelookup/facade_assembler.go`) is the ONLY new code touching the provenance flow. The gate itself is unmodified. Empty `[]contracts.Source` ⇒ existing gate logic produces the canonical refusal.

**Min-sources gate is cheap.** Implemented as a SQL `COUNT(*)` query in the `knowledge_lookup` tool handler BEFORE the LLM call. Avoids spending tokens on queries the gate will refuse anyway.

**Refusal copy is non-negotiable per UX §14.F.** Copy lives in `internal/knowledge/enrichment/refusal.go` constants. Architecture test §13 forbids string-literal divergence.

---

## 13. Substrate-Reuse Invariants + Architecture Tests

Architecture tests live in `internal/knowledge/enrichment/architecture_test.go` and run under `go test ./internal/knowledge/enrichment/...`. They use AST scanning (mirroring spec 061's architecture-test pattern). Each test maps to a specific forbidden mutation:

| Test | Forbidden mutation | Detection |
|---|---|---|
| `TestArchitecture_NoFacadeMutation` | Spec 063 modifying `internal/assistant/facade.go` | git-blame scan in CI: any commit on a spec 063 PR touching that file fails. |
| `TestArchitecture_NoAgentRuntimeMutation` | Spec 063 modifying `internal/agent/{router,executor,registry,nats_driver,tracer}.go` | same scan. |
| `TestArchitecture_NoDirectOllamaHTTP` | Producer code calling Ollama directly | AST scan of `internal/knowledge/enrichment/*.go`: no `import "net/http"` calls to `/api/generate` or `ml/`-equivalent endpoints. Must invoke `executor.Run`. |
| `TestArchitecture_NoHeuristicEdgeMutation` | `UPDATE/DELETE FROM edges WHERE edge_type NOT LIKE 'INFERRED_%'` from enrichment package | AST scan of SQL string literals in `internal/knowledge/enrichment/*.go`. |
| `TestArchitecture_NoHeuristicSynthesisCall` | Enrichment package calling `intelligence.Engine.RunSynthesis` or mutating `ConceptPage` rows outside the no-thinning-guard path | AST scan: only `internal/knowledge/enrichment/resynthesis.go` may `UPDATE concept_pages`; all updates must go through `applyOutputIfNotThinning(...)`. |
| `TestArchitecture_NoNotificationCall` | Enrichment package importing `internal/notification/` | import-graph assertion. |
| `TestArchitecture_RefusalCopyConstants` | Refusal copy string literals diverging from UX §14.F canonical text | golden-file comparison against `refusal.go` constants. |

These 7 tests are foundation-owned; new producers automatically inherit them. The plan phase MUST place these tests under SCN-063-* scenario coverage.

---

## 14. Contract Boundary with Spec 062

Spec 062 (forward-looking intelligence, planning complete, stashed) reads enriched graph state for its `topic_decay` and `seasonal_pattern` threads. Spec 063 silently enriches background; the contract between them is the **graph itself plus the `enrichment_why` table**, NOT a Go API surface.

**One-way data flow:**
- Spec 063 writes: `concept_pages` (UPDATE via resynthesis), `edges` (INSERT `INFERRED_*`), `enrichment_why` (INSERT), `consolidation_candidates` (INSERT), `enrichment_token_ledger` (UPSERT).
- Spec 062 reads: all of the above, plus existing heuristic tables. Spec 062 does NOT write to any spec 063 table.

**Spec 063 emits NO events for spec 062 consumption.** Spec 062 producers run on their own cron schedule and read whatever graph state is current. No subscription, no callback, no `forward_looking.trigger_on_enrichment_complete` event. This prevents a circular dependency and keeps each spec independently runnable.

**Spec 063 may consume spec 062 outputs in v2 (not v1).** Out of scope here.

**Shared SST namespace partitioning:**
- `enrichment.*` — owned by spec 063.
- `assistant.forward_looking.*` — owned by spec 062 (per spec 062 §7).
- Zero overlap.

---

## 15. Product Principle Alignment (design-level)

Extends spec.md §8. Design-level evidence pointers:

| Principle | Design-level evidence |
|---|---|
| **P1 — Observe First, Ask Second** | §1.2 background producers fire on observed events (artifact ingest, topic edit) without prompting user. Consolidation analyzer (§5) detects async; surfaces only on pull (UX §14.D). |
| **P3 — Knowledge Breathes** | §4 hybrid trigger + cron drain IS the lifecycle pump. §6 `superseded_at` column + §7 `INFERRED_*` rows with `metadata.llm_run_id` history give explicit lifecycle states. |
| **P5 — One Graph, Many Views** | §7 reuses `edges`; §6 sibling table is a provenance store, not a parallel graph. Architecture test (§13) forbids parallel store. |
| **P6 — Invisible By Default** | §12 background refusals are logs+metrics only. §1.2 zero notifications. Disclosure footer (§4 wiring to UX §14.B) earns its place via AND-gated threshold. |
| **P7 — Small, Frequent, Actionable Output** | §10 per-tick budget keeps producer ticks small. Reactive p95 < 5s (UX §14.G) keeps replies phone-screen-fit. |
| **P8 — Trust Through Transparency** | §2.2 `ProvenanceRecord` on every output. §6 schema makes provenance queryable. §13 architecture test forbids citation-set violations. §11 disclosed downgrade (never silent). |
| **P9 — Design For Restart** | §4 `DrainBacklog` bounded by `backlog_cap`; §11 ledger persists across restart (no cost cliff on resume). |
| **P10 — QF Companion Boundary** | Financial-markets-connector artifacts (spec 018) may participate as evidence; spec 063 emits zero financial-action surface. Architecture test (§13 `TestArchitecture_NoNotificationCall`) covers; future financial-aware refusal copy is a plan-phase concern. |

---

## 16. Open Questions Remaining (plan-owned)

| ID | Question | Owner |
|---|---|---|
| OQ-PLAN-1 | Per-tick budget calibration. Concrete values for `enrichment.producers.<name>.per_tick_budget` and `backlog_cap`. Requires empirical load test on representative graph size; not a design decision. | bubbles.plan |
| OQ-PLAN-2 | `consolidation_candidates` retention policy. Should rows be auto-deleted after N days of non-render, or persisted indefinitely until user acts? Storage-vs-staleness trade-off; not user-perceivable in v1 since persisted-but-inert (UX §14.D). | bubbles.plan |
| OQ-PLAN-3 | Architecture-test placement and CI wiring. Where should the git-blame foreign-file mutation check live (CI workflow vs `go test` vs pre-commit)? Spec 062 may have established the precedent; plan to reuse if so. | bubbles.plan |
| OQ-PLAN-4 | Candidate-pair selector SQL for `relationship_inference`. The "artifacts sharing zero topics but sharing named entities" query needs concrete SQL — depends on existing `entities` table schema discovery. May surface a route-required packet if existing schema is insufficient. | bubbles.plan |
| OQ-PLAN-5 | Event publication points for `enrichment.trigger.artifact_*`, `enrichment.trigger.intelligence.*_emitted`, and `enrichment.trigger.topic_*`. Spec 026 ingest path, spec 021 producers, and spec 025 topic mutation are READ-ONLY substrate; if existing NATS publish points are missing, plan-phase MUST surface a routed packet to the owning spec (NOT silently add publishers). | bubbles.plan |

---

## 17. Routing Note

Per [bubbles-artifact-ownership-routing](../../.github/skills/bubbles-artifact-ownership-routing/SKILL.md):

- **Spec 063 design.md OWNS:** §1 capability surface, §2 foundation contract, §3 module layout, §4–§9 OQ resolutions, §10 SST key names, §11 budget mechanism design, §12 refusal contract design, §13 architecture-test surface, §14 spec 062 boundary, §15 principle alignment.
- **Spec 063 design.md DOES NOT OWN:** §10 numeric defaults (operator-set; design only names keys), per-scope test plans (scopes.md), implementation, spec.md §14 UX (UX-owned).
- **If implementation discovers required substrate mutation:** STOP and route packet to owning spec (037 / 061 / 025 / 026 / 021 / 054). Do NOT silently edit.
