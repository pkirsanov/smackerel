# Feature: 063 Knowledge AI Enrichment

**Status:** in_progress (analyst-owned authoring; ceiling = `specs_hardened`)
**Workflow Mode:** `product-to-planning`
**Owner Directive (2026-05-28, Goal 2 of original 3-ask):**
> "we need ai to be processing knowledge"

**Depends On (read-only):**
- spec 025 (knowledge synthesis layer — `done`) — provides `ConceptPage`/`EntityProfile` storage and heuristic SQL clustering
- spec 026 (domain extraction — `done`) — provides one-shot LLM extraction at ingest
- spec 021 (intelligence-delivery — `done`) — produces heuristic alerts/recommendations spec 063 may enrich with "why"
- spec 037 (LLM agent runtime — `done`) — router + executor + tool/scenario registry; substrate, MUST NOT modify
- spec 061 (conversational assistant — functionally shipped) — facade owns the conversational surface; spec 063 may register NEW scenarios + tools through documented extension points only

**Unblocks:**
- future spec 062 (forward-looking intelligence) consumption of enrichment "why" prose
- future user-facing knowledge browsing UX (out of scope here)

---

## 1. Problem Statement

Today Smackerel's "AI knowledge processing" surface is one-shot and heuristic:

- **Spec 026** runs LLM extraction at the moment an artifact is ingested (receipt parsing, product extraction, recipe parsing, Drive classification). Once extracted, the result is frozen — later context never re-illuminates it.
- **Spec 025** pre-synthesizes `ConceptPage`s and `EntityProfile`s from clusters, but the cluster discovery itself is a SQL `GROUP BY` over `BELONGS_TO` edges (`internal/intelligence/synthesis.go` lines 19-39: `HAVING COUNT(*) >= 3 AND COUNT(DISTINCT source_id) >= 2`). The LLM is invoked to author the concept page text, but the *relationships* it summarizes are heuristic.
- **Spec 021** intelligence producers (alerts, briefs, recommendations, resurfacing, expertise, learning) are zero-LLM. A user receives a recommendation score but no prose explaining *why* this matters to them.
- The conversational surface (spec 061) can answer questions, but it answers them by retrieving raw artifacts and summarizing them at query time. There is no curated, durable, LLM-synthesized graph of what the user *knows*.

The owner's Goal 2 asks for the AI layer to actually *process* the knowledge graph continuously — not just at ingest, not just at query time, and not just heuristically. Concretely: re-synthesize when the graph changes, infer relationships the SQL `GROUP BY` cannot see, explain heuristic outputs in user-felt prose, consolidate concepts when the user edits the graph, and answer "what do I know about X?" from synthesized knowledge rather than raw retrieval.

This spec defines the *enrichment* capability that sits between the heuristic substrate (specs 021/025/026) and the user-facing surfaces (specs 061/062/future UX). It does not replace any of them; it makes them richer.

---

## 2. Outcome Contract

**Intent:** Smackerel's knowledge graph is continuously enriched by LLM-driven synthesis that runs invisibly in the background and on demand: when artifacts arrive, the affected concept pages and entity profiles are re-synthesized; cross-artifact relationships the heuristic clustering misses are inferred and recorded with confidence + source attribution; intelligence-layer outputs are augmented with prose explanations citing graph evidence; user edits to topics trigger LLM consolidation suggestions; and the conversational facade can answer "what do I know about X?" from synthesized knowledge rather than raw retrieval.

**Success Signal:**
- For a curated evaluation set of ≥20 newly ingested artifacts that touch existing concept pages, ≥80% trigger a re-synthesis pass within the configured cadence window and the resulting `ConceptPage.updated_at` advances with a non-trivial diff (summary or claims changed by ≥1 token).
- For a curated evaluation set of ≥10 intelligence-layer alerts/recommendations, ≥80% receive an LLM-authored "why this matters" prose explanation whose claims are *all* traceable to artifact IDs already in the alert's source set (zero unattributed claims, zero hallucinated artifact IDs).
- For a curated evaluation set of ≥10 reactive "what do I know about X?" queries through the facade, the response cites concept-page IDs and/or artifact IDs and the cited evidence demonstrably supports each claim made (sampled by human eval ≥80%).
- For a curated evaluation set of ≥5 topic edits/merges by the user, the consolidation pass produces a suggestion (merge / keep separate / needs more data) with a confidence band and a list of evidence artifact IDs.

**Hard Constraints:**
1. **Single knowledge graph (Principle 5).** Enrichment MUST extend existing `ConceptPage` / `EntityProfile` / `edges` / `topics` storage. NO parallel store, NO sibling tables that duplicate concept-page semantics. New columns/tables are permitted only when they record enrichment provenance (LLM run metadata, inferred-relationship confidence, consolidation suggestions) that cannot be expressed on the existing schema without overloading.
2. **Source attribution for every LLM claim (Principle 8).** Every enrichment output — re-synthesized concept page, inferred relationship, "why" explanation, reactive answer, consolidation suggestion — MUST carry artifact-ID citations or concept-page-ID citations. Outputs without traceable evidence MUST be refused, not fabricated.
3. **Invisible by default (Principle 6).** Enrichment producers run async / on a scheduler cadence. Spec 063 itself fires zero user-facing notifications. (Spec 062 owns proactive surfacing.)
4. **Async re-synthesis catches up gracefully (Principle 9).** When the runtime starts after an offline period or a backlog of edits, the re-synthesis producer MUST drain the backlog with a bounded per-tick budget — no notification storm, no thrash on the graph, no LLM-cost cliff.
5. **NO-DEFAULTS / fail-loud SST.** All new config keys (cadence windows, confidence floors, per-producer budgets, LLM model selection, refusal thresholds) MUST be declared in `config/smackerel.yaml` and fail loud if unset. No silent fallbacks in source.
6. **Reuse spec 037 substrate; do NOT fork it.** Reactive enrichment queries through the facade MUST register as new scenarios + tools via spec 037's documented `init()` extension points. Spec 063 MUST NOT modify `internal/agent/`, `internal/assistant/facade.go`, or any spec 061/062 surface; if such a change is needed, it is routed as a packet to the owning spec.
7. **Refusal contract.** When graph evidence is insufficient to support a claim (concept has < N artifacts, alert has zero source artifacts in graph, reactive query has zero retrieved artifacts), the producer MUST emit a structured refusal with reason, NOT a low-confidence guess.
8. **QF boundary (Principle 10).** Enrichment MUST NOT produce financial advice. Financial-markets-connector artifacts (spec 018) may participate as evidence in concept pages, but enrichment outputs that touch financial topics MUST carry an explicit non-advice disclosure and MUST NOT recommend trades, mandate changes, or executions.

**Failure Condition:** Concept pages or "why" explanations contain claims that cannot be traced to an artifact ID in the user's graph (hallucination), OR the re-synthesis producer overwrites richer existing content with thinner re-synthesized content (regression), OR enrichment fires user-visible notifications (violates Principle 6 and spec 062 boundary), OR the producer cost cliff (LLM tokens / latency) makes the home-lab deploy unusable.

---

## 3. Goals

- G1: Async re-synthesis producer that re-runs LLM concept-page authoring when source artifacts arrive, mutate, or are removed, within a bounded per-tick budget and with backlog catch-up discipline.
- G2: LLM relationship inference producer that detects artifact-to-artifact and concept-to-concept relationships the SQL `GROUP BY` clustering cannot see (semantic similarity below the heuristic threshold, transitive relationships, named-entity coreference across sources), records them as typed edges with confidence + LLM run metadata, and refuses below a configured confidence floor.
- G3: LLM "why this matters" augmenter that takes an intelligence-layer alert/recommendation/brief and produces a prose explanation citing the alert's existing source artifact IDs (zero new evidence introduced), persisted on the alert/recommendation row or a sibling enrichment column.
- G4: Topic consolidation analyzer that, when the user edits or merges topics, invokes an LLM pass over the affected topics' artifacts and produces a structured suggestion (merge / keep separate / needs more data) with confidence + evidence artifact IDs.
- G5: Reactive enrichment scenario(s) registered against the spec 061 facade — at minimum a `knowledge_lookup` scenario that answers "what do I know about X?" using synthesized concept pages + raw retrieval as fallback, with full source attribution.
- G6: All enrichment outputs carry a `prompt_contract_version` + `llm_run_id` + `source_artifact_ids` triple sufficient to reproduce the output and audit the evidence chain.

---

## 4. Non-Goals

- **Forward-looking / proactive surfacing.** Predicting what the user *will need*, firing notifications, scheduling nudges, or watching for upcoming events — owned by spec 062. Spec 063 enriches the graph; spec 062 surfaces forward-looking insights from it.
- **Conversational surface itself.** Spec 061 owns the facade, router, tool registry, transport adapters, conversational state, capture-as-fallback, and provenance gate. Spec 063 may register NEW scenarios/tools through the documented extension points but MUST NOT modify the facade or any spec 061 substrate.
- **One-shot extraction at ingest.** Spec 026 owns receipt / product / recipe / Drive extraction at the moment of ingest. Spec 063 may consume those outputs as evidence but MUST NOT duplicate or modify the ingest-time path.
- **Heuristic SQL clustering.** Spec 025's `synthesis.go` `GROUP BY` path remains the substrate. Spec 063's relationship inference is *additive* and is recorded as separate edges with distinct provenance; it does NOT delete or rewrite heuristic edges.
- **New UI screens.** The web/UI surface for browsing enriched knowledge is out of scope here (future spec). Reactive enrichment via the facade is the only user-facing v1 channel.
- **Financial advice / QF actions.** Per Principle 10.
- **New connector data sources.** Spec 063 enriches what the existing connectors (007–018) already ingest.

---

## 5. Domain Capability Model

Spec 063 introduces a new capability foundation — *knowledge enrichment* — with multiple concrete producers/consumers, so the AN5 capability-first triggers apply. The model below is provider-/screen-/class-neutral and describes the domain primitives every concrete producer MUST honor.

### 5.1 Domain primitives

| Primitive | Definition | Lifecycle states |
|-----------|------------|------------------|
| **EnrichmentJob** | A unit of LLM-driven work over a defined slice of the graph (one concept page, one alert, one topic edit, one reactive query). | `pending → running → completed → applied` (happy) ; `pending → refused` (evidence-insufficient) ; `running → failed → retrying → abandoned` (LLM / infra failure) |
| **EnrichmentTrigger** | The signal that produced an `EnrichmentJob`. | `artifact_arrived`, `artifact_mutated`, `artifact_removed`, `topic_edited`, `topic_merged`, `alert_emitted`, `recommendation_emitted`, `brief_emitted`, `reactive_query`, `scheduler_tick_catchup` |
| **EnrichmentOutput** | The LLM result attached to the target primitive (concept page text, inferred edge, "why" prose, consolidation suggestion, reactive answer). | `draft → applied → superseded` (newer enrichment replaces it) ; `draft → refused` (evidence below floor) |
| **EvidenceSet** | The artifact IDs + concept-page IDs the LLM was permitted to cite. | Closed set per job; outputs citing IDs outside the set MUST be refused. |
| **ProvenanceRecord** | `{prompt_contract_version, llm_run_id, model_id, evidence_artifact_ids, evidence_concept_page_ids, confidence, refusal_reason?}` attached to every `EnrichmentOutput`. | Immutable once written. |
| **EnrichmentProducer** | A registered handler that turns one `EnrichmentTrigger` into ≥0 `EnrichmentJob`s. | Producers register at startup; disabling a producer MUST drain its queue gracefully. |

### 5.2 Relationships

- `EnrichmentTrigger` 1→N `EnrichmentJob`.
- `EnrichmentJob` 1→1 `EvidenceSet` (the closed set the LLM may cite).
- `EnrichmentJob` 1→0..1 `EnrichmentOutput` (refused jobs have no output, only a refusal record).
- `EnrichmentOutput` 1→1 `ProvenanceRecord`.
- `EnrichmentOutput` 1→1 *target primitive* in the existing graph: an updated `ConceptPage`, a new `edges` row with an enrichment-typed relationship, a column on an alert/recommendation/brief row, a topic-consolidation suggestion record, or a reactive-query response envelope.

### 5.3 Business policies (every concrete producer MUST obey)

- **Closed evidence set.** A producer MUST pre-compute the closed set of evidence IDs the LLM is permitted to cite, pass them through the prompt contract, and reject any output that cites an ID outside the set.
- **Confidence floor.** Every producer MUST honor a configured confidence floor; outputs below the floor MUST be refused, not stored as low-confidence.
- **Idempotent on identical evidence.** Re-running a producer over the same evidence set + prompt contract version SHOULD produce a semantically equivalent output (modulo LLM nondeterminism — the producer SHOULD detect "no meaningful diff" and skip persistence).
- **Backlog-bounded.** Every producer MUST declare a per-tick budget (max jobs per scheduler tick) and a backlog catch-up cap (max jobs per offline-period drain).
- **Refusal is first-class.** Refusal is a normal terminal state, not an error.
- **Never overwrites richer content.** A re-synthesis producer MUST detect if the new output is strictly thinner than the existing one (fewer claims, shorter summary, fewer cited artifacts) and either refuse, merge, or escalate per a documented policy.

### 5.4 Provider-neutral behavior vocabulary

Concrete producers (re-synthesis, relationship-inference, why-augmenter, consolidation-analyzer, reactive-knowledge-lookup) all speak: `enqueue(trigger) → jobs[]`, `runJob(job) → output | refusal`, `applyOutput(output) → targetPrimitiveUpdated`, `drainBacklog(budget) → jobsProcessed`. No producer holds private LLM access; all go through the spec 037 ML bridge.

---

## 6. Actors & Personas

| Actor | Description | Goals | Permissions |
|-------|-------------|-------|-------------|
| **Human user (knowledge owner)** | Single human operator whose artifacts are enriched. | Get a knowledge graph that "knows itself" — concept pages stay fresh, relationships surface, intelligence outputs are explained, reactive queries return synthesized answers with citations. | Read everything; edits topics, merges concepts, asks reactive questions. |
| **Async Enrichment Producer** (new) | Background producer registered with the scheduler; consumes `EnrichmentTrigger`s and emits `EnrichmentJob`s. | Drain backlog, honor budgets, never block ingestion. | Reads graph + intelligence outputs; writes enrichment outputs + provenance; never writes to user-facing notification channels. |
| **Reactive Enrichment Scenario(s)** (new, registered with spec 061 facade via init() extension) | Spec-037-style scenario(s) that answer reactive enrichment queries through the conversational facade. | Synthesize answers from concept pages + retrieval, cite evidence, refuse when evidence is insufficient. | Read-only on the graph; writes only the conversational response envelope through the facade. |
| **Scheduler** (existing, `internal/scheduler/`) | Cron-style ticker that drives producers. | Fire producer ticks per configured cadence. | Existing scheduler surface; new producers register via existing extension points. |
| **LLM Bridge** (existing, `ml/` sidecar via spec 037 substrate) | Routes prompt contracts to Ollama models. | Execute prompts with declared model + contract version. | Existing; spec 063 adds new prompt contracts under `config/prompt_contracts/` only. |
| **Operator** | Owns the deploy and rotates models/budgets. | Enable/disable each producer; set cadence, confidence floor, per-tick budget, model selection per producer. | Edits `config/smackerel.yaml` `enrichment.*` block. |

---

## 7. Requirements

### 7.1 Functional

- **R-1 (re-synthesis producer):** When an artifact arrives, mutates, or is removed, the re-synthesis producer SHALL identify all concept pages whose `source_artifact_ids` overlap with the changed artifact and enqueue re-synthesis jobs for them, bounded by a per-tick budget.
- **R-2 (re-synthesis catch-up):** When the runtime starts after an offline period, the re-synthesis producer SHALL drain the backlog of changed artifacts since last successful run, bounded by a backlog catch-up cap, oldest-first by artifact `created_at`.
- **R-3 (re-synthesis no-thinning guard):** A re-synthesis output that produces strictly fewer claims OR strictly fewer cited artifact IDs OR a shorter summary than the existing concept page SHALL be refused with reason `would_thin_existing_content`, NOT applied.
- **R-4 (relationship inference producer):** A scheduled producer SHALL select candidate artifact pairs / concept-page pairs the SQL clustering did not connect (e.g., artifacts sharing zero topics but sharing named entities; concept pages with semantically related summaries below the heuristic threshold) and invoke the LLM to classify the candidate relationship.
- **R-5 (relationship inference confidence floor):** Inferred relationships SHALL be persisted as typed `edges` rows with a new `edge_type` distinguishing them from heuristic edges (e.g., `INFERRED_RELATED`, `INFERRED_COREFERENCE`) and SHALL carry the confidence value; rows below the configured confidence floor SHALL NOT be persisted.
- **R-6 (why-augmenter):** When an intelligence-layer alert/recommendation/brief is emitted with ≥1 source artifact ID in its evidence set, the why-augmenter SHALL produce an LLM prose "why this matters" string whose every factual claim cites an artifact ID from the existing evidence set, and SHALL persist it alongside the alert/recommendation/brief row.
- **R-7 (why-augmenter refusal):** When the source evidence set is empty or the LLM output cites an ID outside the set, the augmenter SHALL refuse with reason `insufficient_evidence` or `evidence_set_violation` respectively.
- **R-8 (consolidation analyzer):** When the user edits a topic name, merges two topics, or splits a topic, the consolidation analyzer SHALL invoke an LLM pass over the affected topics' artifacts and emit a structured suggestion `{decision: merge|keep_separate|needs_more_data, confidence: <float>, evidence_artifact_ids: [...]}`.
- **R-9 (reactive enrichment scenario — knowledge_lookup):** A new scenario `knowledge_lookup` SHALL register with the spec 061 facade through the documented `init()` extension. Given a user query containing a noun phrase, the scenario SHALL retrieve relevant concept pages + artifacts, invoke the LLM to synthesize an answer, cite concept-page IDs and artifact IDs for every claim, and refuse via the existing facade provenance gate when evidence is insufficient.
- **R-10 (provenance):** Every enrichment output SHALL persist `{prompt_contract_version, llm_run_id, model_id, evidence_artifact_ids, evidence_concept_page_ids, confidence}` and (for refusals) `refusal_reason`.
- **R-11 (idempotency / no-op detection):** A re-run of any producer over an unchanged evidence set + unchanged prompt contract version SHOULD detect a no-op (semantic-equivalent output) and skip persistence.
- **R-12 (per-producer config — SST fail-loud):** Every producer SHALL declare: enabled flag, cadence, per-tick budget, backlog catch-up cap, confidence floor, prompt contract version, model selection. All declared in `config/smackerel.yaml` under `enrichment.<producer>.*`; missing values fail loud.
- **R-13 (no notification side effects):** No enrichment producer SHALL invoke any notification surface (Telegram, web, mobile, email). Surfacing is spec 062's concern.

### 7.2 Non-Functional

- **NFR-1 (reactive latency):** Reactive `knowledge_lookup` scenarios SHALL meet the existing facade p95 budget (< 5s — matches spec 061's facade SLA).
- **NFR-2 (async budget):** Async producers SHALL respect their per-tick budget; total LLM tokens consumed per day SHALL be observable as a Prometheus counter so the operator can detect cost cliffs.
- **NFR-3 (refusal disclosure):** Refusals SHALL be observable in logs + metrics so the operator can detect a producer that refuses 100% of jobs.
- **NFR-4 (no graph corruption on failure):** Producer failures (LLM unreachable, prompt contract parse error, model timeout) SHALL leave the graph in its pre-job state; partial writes are forbidden.
- **NFR-5 (backlog draining bounded):** The backlog catch-up cap SHALL ensure that a 30-day offline period produces no more than 1 day's worth of LLM cost when the runtime restarts.
- **NFR-6 (observability):** Every producer SHALL emit a trace span tagged with producer name, trigger type, evidence size, refusal reason (if any), and `llm_run_id`.
- **NFR-7 (no-defaults compliance):** All config keys SHALL be enforced via the existing `internal/config/validate.go` style; no `getEnv(..., "fallback")` patterns.

---

## 8. Product Principle Alignment

| Principle | Application | Evidence Pointer |
|-----------|-------------|------------------|
| **P1 — Observe First, Ask Second** | All enrichment runs automatically over already-observed artifacts. Consolidation analyzer is the only producer reacting to a user input (topic edit), and even there the LLM makes the suggestion — the user is not asked to manually classify. | `docs/Product-Principles.md` (P1 ratified TBD); `internal/intelligence/synthesis.go` shows the existing observe-first pattern this spec extends. |
| **P3 — Knowledge Breathes** | Re-synthesis producer (R-1, R-2) IS the explicit lifecycle pump. `EnrichmentOutput` lifecycle (`draft → applied → superseded`) and `ConceptPage.updated_at` advancement encode the breathing. | `internal/topics/lifecycle.go` already encodes lifecycle states for topics; spec 063 brings the same discipline to concept pages and inferred relationships. |
| **P5 — One Graph, Many Views** | Hard Constraint #1 forbids parallel stores. Re-synthesized concept pages update existing `ConceptPage` rows; inferred relationships extend the existing `edges` table with new typed `edge_type` values. | `internal/knowledge/types.go` (`ConceptPage`), `internal/knowledge/store.go` (`UpdateConcept`), existing `edges` table. |
| **P6 — Invisible By Default** | Hard Constraint #3, R-13. Spec 063 fires zero notifications. Surfacing belongs to spec 062. | `docs/Product-Principles.md` P6 quotation; spec 062 boundary in Non-Goals. |
| **P8 — Trust Through Transparency** | Hard Constraint #2, R-6 / R-7 / R-10. Every output carries `evidence_artifact_ids` + `evidence_concept_page_ids` + `prompt_contract_version` + `llm_run_id`. Outputs that cite outside the closed set are refused. | Existing pattern in `internal/knowledge/types.go` (`prompt_contract_version`, `source_artifact_ids` already present on `ConceptPage`) — spec 063 extends this discipline to every enrichment output. |
| **P9 — Design For Restart** | R-2 catch-up + NFR-5 bounded drain. After an offline period the producer drains oldest-first under a configured cap; no notification storm (P6); operator sees a clean Prometheus catch-up curve. | `internal/scheduler/` existing tick discipline. |

**P10 (QF Companion Boundary):** No financial-action surface. Hard Constraint #8 makes the boundary explicit for any enrichment touching financial-markets-connector artifacts.

---

## 9. User Scenarios (Gherkin — representative; full set tracked in `scenario-manifest.json` during plan phase)

```gherkin
Scenario: SCN-063-001 Re-synthesis triggers on new artifact
  Given a ConceptPage "C1" exists with source_artifact_ids [A1, A2, A3]
  And a new artifact A4 is ingested with topic overlap to C1
  When the re-synthesis producer's next scheduler tick fires
  Then an EnrichmentJob for C1 is enqueued with evidence set {A1, A2, A3, A4}
  And on completion C1.updated_at advances
  And C1.source_artifact_ids contains A4
  And C1's ProvenanceRecord carries the new llm_run_id and prompt_contract_version

Scenario: SCN-063-002 Re-synthesis refuses when output would thin existing content
  Given a ConceptPage "C2" exists with 8 claims and a 200-token summary
  And the LLM re-synthesis run returns 3 claims and a 60-token summary
  When the re-synthesis producer applies the no-thinning guard
  Then the new output is refused with reason "would_thin_existing_content"
  And C2 remains unchanged
  And the refusal is recorded with provenance for audit

Scenario: SCN-063-003 Backlog catch-up after offline period
  Given the runtime was offline for 30 days
  And 500 new artifacts arrived during the offline period
  When the re-synthesis producer starts and drains backlog
  Then jobs are processed oldest-first by artifact created_at
  And total jobs processed per tick respects enrichment.resynthesis.per_tick_budget
  And total jobs queued for catch-up respects enrichment.resynthesis.backlog_cap
  And zero user-visible notifications are emitted by spec 063 producers

Scenario: SCN-063-004 LLM relationship inference between artifacts
  Given two artifacts A5 and A6 with zero shared topics
  And A5 and A6 share named entities the heuristic clustering did not detect
  When the relationship-inference producer evaluates the (A5, A6) candidate
  And the LLM returns relationship_type=INFERRED_COREFERENCE with confidence 0.82
  And the configured floor enrichment.relationship_inference.confidence_floor=0.7
  Then a new edges row is persisted with edge_type=INFERRED_COREFERENCE
  And confidence and llm_run_id are recorded on the row's provenance
  And the heuristic edges between A5/A6 (if any) are untouched

Scenario: SCN-063-005 Relationship inference refuses below confidence floor
  Given a candidate (A7, A8) and confidence_floor=0.7
  When the LLM returns confidence 0.55
  Then no edge is persisted
  And the refusal is logged with reason "below_confidence_floor"

Scenario: SCN-063-006 Why-augmenter on intelligence alert
  Given an alert ALERT-1 emitted by internal/intelligence with source_artifact_ids [A9, A10]
  When the why-augmenter consumes ALERT-1
  Then an LLM "why this matters" prose is persisted alongside ALERT-1
  And every factual claim in the prose cites an artifact ID from {A9, A10}
  And the persisted prose carries prompt_contract_version + llm_run_id

Scenario: SCN-063-007 Why-augmenter refuses on empty evidence
  Given an alert ALERT-2 emitted with source_artifact_ids []
  When the why-augmenter evaluates ALERT-2
  Then no prose is persisted
  And the refusal is recorded with reason "insufficient_evidence"

Scenario: SCN-063-008 Topic merge triggers consolidation analyzer
  Given the user merges topic T1 into topic T2
  When the consolidation analyzer runs over the union of their artifacts
  Then a structured suggestion {decision, confidence, evidence_artifact_ids} is persisted
  And the suggestion is visible to the user (via reactive scenario or future UX) but no notification is fired

Scenario: SCN-063-009 Reactive knowledge_lookup answer cites concept pages
  Given the user asks the conversational facade "what do I know about <topic X>?"
  And concept page CP-X exists with 5 cited artifacts
  When the knowledge_lookup scenario runs
  Then the facade response synthesizes an answer
  And every factual claim cites either CP-X or an artifact ID in CP-X's source set
  And the response respects the existing facade provenance gate

Scenario: SCN-063-010 Reactive knowledge_lookup refuses on empty graph evidence
  Given the user asks "what do I know about <topic Y>?"
  And zero concept pages or artifacts match topic Y above the retrieval floor
  When the knowledge_lookup scenario runs
  Then the facade returns the canonical refusal body (no fabrication)
  And the user can still capture the query as an idea via spec 061's capture-as-fallback path
```

---

## 10. Acceptance Criteria

Each Gherkin scenario above SHALL be linked, during the planning phase, to:

- One stable `SCN-063-NNN` entry in `scenario-manifest.json` (currently empty skeleton; plan-phase work).
- At least one test of the appropriate type (unit for producer-internal logic; integration for graph mutations; e2e-api or e2e-ui for reactive facade scenarios).
- A regression entry per the project's `Regression E2E` rule for any scenario that survives into the long-term test suite.

Negative-path coverage (refusals: SCN-063-002, -005, -007, -010) is mandatory and explicitly listed as adversarial.

---

## 11. Out-of-Scope (re-stated explicitly to bound the planning phase)

- Spec 062's forward-looking surfacing (proactive notifications, predictive insights, "you should know" pushes).
- Spec 061's conversational substrate (facade, router, executor, tool registry, transport adapters, capture-as-fallback, provenance gate). Spec 063 only *registers new scenarios* against documented extension points.
- Spec 037's agent runtime (router/executor internals).
- Spec 025's heuristic SQL clustering substrate (left intact; spec 063 enrichment is additive).
- Spec 026's ingest-time extraction (left intact).
- New connectors.
- New web/UI screens for knowledge browsing.
- Any financial-advice surface (Principle 10).

---

## 12. Open Questions (routed to bubbles.ux / bubbles.design / bubbles.plan)

These are genuinely ambiguous decisions that should be resolved before design/scope phases — NOT analyst guesses.

| ID | Question | Recommended Owner |
|----|----------|-------------------|
| OQ-1 | **RESOLVED 2026-05-29 (bubbles.design).** Hybrid event-driven enqueue + cron drain. See [design.md §4](design.md#4-resolves-oq-1-re-synthesis-trigger-granularity). | bubbles.design ✓ |
| OQ-2 | **RESOLVED 2026-05-29 (bubbles.design).** Per-surface floors; relationship_inference 0.70, consolidation_analyzer 0.75, why_augmenter 0.50; resynthesis gated by no-thinning guard (no floor); knowledge_lookup gated by spec 061 provenance gate. See [design.md §5](design.md#5-resolves-oq-2-confidence-floor-calibration). | bubbles.design ✓ |
| OQ-3 | **RESOLVED 2026-05-29 (bubbles.design).** Dedicated sibling table `enrichment_why` keyed by `(parent_kind, parent_id)`; avoids foreign-spec mutation of spec 021 tables. See [design.md §6](design.md#6-resolves-oq-3-storage-shape-for-why-prose). | bubbles.design ✓ |
| OQ-4 | **RESOLVED 2026-05-29 (bubbles.design).** Reuse `edges` table with closed `INFERRED_RELATED | INFERRED_COREFERENCE | INFERRED_TEMPORAL_SEQUENCE` taxonomy; provenance in `metadata JSONB`; partial index on `edge_type LIKE 'INFERRED_%'`. See [design.md §7](design.md#7-resolves-oq-4-storage-shape-for-inferred-relationships). | bubbles.design ✓ |
| OQ-5 | **RESOLVED 2026-05-29 (bubbles.ux).** Backlog drain UX. See §14.B — staleness-threshold-gated disclosure (footer earns its place only when last_enrichment > 60min OR backlog > 100 jobs). Design owns wiring SST keys + producer→facade signal. | bubbles.ux ✓ → bubbles.design |
| OQ-6 | **RESOLVED 2026-05-29 (bubbles.ux).** LLM vs heuristic resolution. See §14.C — heuristic remains durable canonical signal; LLM stored alongside as soft `INFERRED_*` provenance, never overwrites; reactive surface discloses both when they diverge. Design owns storage shape (intersects OQ-3/OQ-4). | bubbles.ux ✓ → bubbles.design |
| OQ-7 | **RESOLVED 2026-05-29 (bubbles.ux).** Consolidation analyzer surface. See §14.D — persisted-but-inert (async detection, no auto-merge, no proactive nudge); surfaces only on explicit reactive ask or in-flow topic-edit context. Design+plan own persistence shape. | bubbles.ux ✓ → bubbles.design + bubbles.plan |
| OQ-8 | **RESOLVED 2026-05-29 (bubbles.design).** Coexist; router picks on intent — `retrieval_qa` for recall verbs, `knowledge_lookup` for synthesis verbs; `knowledge_lookup` composes `retrieval_search` as a subroutine. See [design.md §8](design.md#8-resolves-oq-8-reactive-scenario-boundary-with-spec-061-retrieval_search). | bubbles.design ✓ |
| OQ-9 | **RESOLVED 2026-05-29 (bubbles.design).** Per-producer prompt contracts (5 new YAMLs); mirrors existing `config/prompt_contracts/` per-task pattern (18/18 files). See [design.md §9](design.md#9-resolves-oq-9-per-producer-prompt-contracts-vs-shared). | bubbles.design ✓ |
| OQ-10 | **RESOLVED 2026-05-29 (bubbles.ux).** Token-cost cap. See §14.E — disclosed downgrade to cheaper model first (gemma3:4b fallback with explicit footer), refuse only when even cheap model exceeds remaining budget. Design owns SST key naming + model-selection wiring. | bubbles.ux ✓ → bubbles.design |
| OQ-PLAN-1 | **RESOLVED 2026-05-29 (bubbles.plan).** Recommended starting values stamped into SCOPE-01 SST; empirical calibration deferred to SCOPE-11 load test. See [scopes.md OQ-PLAN-1](scopes.md#open-question-resolutions-plan-owned). | bubbles.plan ✓ |
| OQ-PLAN-2 | **RESOLVED 2026-05-29 (bubbles.plan).** Persist `consolidation_candidates` with 90-day TTL + manual cleanup; soft-delete only when `last_surfaced_at IS NULL`. See [scopes.md OQ-PLAN-2](scopes.md#open-question-resolutions-plan-owned). | bubbles.plan ✓ |
| OQ-PLAN-3 | **RESOLVED 2026-05-29 (bubbles.plan).** Reuse spec 062 architecture-test pattern (co-located `architecture_test.go` + adversarial sub-tests, picked up by existing `./smackerel.sh test unit --go`). No new CI workflow file. See [scopes.md OQ-PLAN-3](scopes.md#open-question-resolutions-plan-owned). | bubbles.plan ✓ |
| OQ-PLAN-4 | **RESOLVED 2026-05-29 (bubbles.plan).** Existing `knowledge_entities` schema sufficient; candidate-pair selector SQL drafted in SCOPE-07 implementation plan. No route-required packet to spec 025. See [scopes.md SCOPE-07](scopes.md#scope-07-relationshipinferenceproducer-r-45). | bubbles.plan ✓ |
| OQ-PLAN-5 | **PARTIALLY ROUTED 2026-05-29 (bubbles.plan).** `SubjectArtifactsProcessed` reusable for resynthesis (no packet). 3 route-required packets PKT-063-A/B/C queued for spec 025 (topic edit/merge) and spec 021 (alert/recommendation/brief emitted). Until packets land, SCOPE-08/09 ship with cron-only fallback. See [scopes.md Routing section](scopes.md#routing-packets-to-other-spec-owners). | bubbles.plan ✓ (3 packets routed) |

---

## 13. Routing Note (artifact ownership boundary)

Per `.github/skills/bubbles-artifact-ownership-routing/SKILL.md`:

- **Spec 063 OWNS:** new enrichment producers (new files under `internal/enrichment/` or equivalent — design phase decides), new prompt contracts under `config/prompt_contracts/enrichment-*.yaml`, new SST keys under `enrichment.*`, new typed `edges.edge_type` values for inferred relationships, new columns/tables for enrichment provenance (subject to OQ-3/OQ-4), new spec-037-registered scenarios under their own scenario IDs (`knowledge_lookup`, etc.).
- **Spec 063 READS-ONLY:** spec 025 (`internal/knowledge/`, `internal/intelligence/synthesis.go`), spec 026 (`internal/extract/`, `ml/app/domain.py`, `ml/app/synthesis.py`), spec 021 (`internal/intelligence/`), spec 037 (`internal/agent/`), spec 061 (`internal/assistant/facade.go`).
- **Spec 063 INTEGRATES VIA DOCUMENTED EXTENSION POINTS ONLY:** spec 037 scenario/tool `init()` registration; spec 061 facade scenario routing. Any substrate change is a packet routed to the owning spec — NOT a direct edit.
- **Spec 062 (stashed):** spec 063 explicitly leaves room for spec 062 to consume enrichment "why" prose; no direct dependency in either direction is required for v1.

---

## 14. UX (Workflow Behavior, Status Language, Disclosure & Refusal Shape)

> Owner: bubbles.ux. Authored 2026-05-29. Spec 063 has **no new app screens** — it is a background enrichment capability. The user-facing surface is (a) the spec 061 conversational facade for reactive queries (`knowledge_lookup` and consolidation asks), and (b) the spec 062 forward-looking surface that consumes enrichment "why" prose as evidence. "UX" here means: status language for enrichment outputs, async-state disclosure rules, LLM-vs-heuristic resolution policy, consolidation surfacing policy, token-budget UX, refusal copy, and latency budget.
>
> All capability-layer rendering (status tokens, `sources:` block, error line, disambiguation prompt, trace-ref footer) is **inherited unchanged** from spec 061 §14.A / §14.B.1 and spec 062 §14.A / §14.F. Spec 063 adds enrichment-specific vocabulary and disclosure rules; it does NOT redefine spec 061/062 primitives.
>
> Resolves Open Questions OQ-5, OQ-6, OQ-7, OQ-10. Numeric thresholds below are bubbles.ux recommendations to bubbles.design — the SST keys in design.md are authoritative; spec 063 honors `smackerel-no-defaults` (every value REQUIRED at startup, no implicit fallback).

### 14.A Status Language (closed vocabulary additions)

Spec 063 introduces four new closed-vocabulary status tokens, additive to spec 061 §14.A.2 and spec 062 §14.A. Each token maps to a Principle 6 actionability shape; each carries a precise distinction so downstream surfaces (spec 062 nudges, reactive facade replies, future UX) can render consistently.

| Token | Kind | Meaning | Actionability bar (P6) | Source set required |
|-------|------|---------|------------------------|---------------------|
| `inferred_connection` | enrichment output, persisted | A typed `edges` row produced by the relationship-inference producer (R-4/R-5). Stored with `INFERRED_*` edge type + confidence + `llm_run_id`. NEVER user-facing on its own — surfaces only as a citation underneath another response. | Cleared by being citation-only (never volunteered). | `evidence_artifact_ids` from candidate pair; closed set. |
| `synthesized_topic` | enrichment output, persisted | A re-synthesized `ConceptPage` (R-1..R-3). Replaces existing concept-page text only when no-thinning guard passes (SCN-063-002). Surfaces when a reactive `knowledge_lookup` reads it. | Cleared by being read-on-demand (no proactive surface). | `evidence_artifact_ids` ⊇ pre-synthesis source set. |
| `consolidation_candidate` | enrichment output, persisted-but-inert | A topic-merge / keep-separate suggestion (R-8) with confidence + evidence. Stored async; rendered ONLY when user asks reactively or is mid-topic-edit (see §14.D). | Cleared by being user-pulled, never pushed. | `evidence_artifact_ids` from affected topics' union. |
| `why_context` | enrichment output, persisted-on-parent | LLM "why this matters" prose (R-6/R-7) attached to a spec 021 alert/recommendation/brief row. Rendered by the consuming surface (spec 062 nudge, future UX) — spec 063 itself does NOT render. | Inherited from parent surface's actionability (spec 063 adds no new prompts). | `evidence_artifact_ids` ⊆ parent alert's existing source set. |

**Naming rationale (grounded):**
- `inferred_connection` vs `synthesized_topic` distinguishes **edge** (artifact↔artifact relationship) from **node** (concept-page content). Two different graph mutations; two different provenance vocabularies.
- `consolidation_candidate` deliberately uses "candidate" not "suggestion" — signals the system holds an opinion but is **not** asking the user to act now (honors P1 observe-first and P6 invisible-by-default).
- `why_context` deliberately avoids "why_explanation" or "rationale" — those imply spec 063 owns the surface. It does not; spec 062 renders.
- `enrichment` is **deliberately not used** as a user-facing token. Users don't think in "enrichment"; they think in "what does the system know about X?" Reserving the term for internal/operator vocabulary preserves Principle 8 transparency without leaking jargon.

Capability-layer rules:
- `inferred_connection` and `synthesized_topic` MUST carry non-empty `evidence_artifact_ids` (R-10).
- `why_context` MUST cite IDs strictly ⊆ the parent alert's source set (R-7 evidence-set-violation refusal).
- `consolidation_candidate` MUST carry `decision ∈ {merge, keep_separate, needs_more_data}` plus confidence and evidence (R-8).
- Spec 063 emits **zero** in-flight status tokens at the conversational surface; reactive queries use the inherited `status=thinking → nudge_brief|nudge_refused` from spec 061/062.

### 14.B Async-State Disclosure (resolves OQ-5)

When a reactive query (`knowledge_lookup` or any spec-061-routed scenario that reads concept pages) runs while the async re-synthesis producer is mid-catch-up after an offline period (SCN-063-003), the user MAY be reading a stale graph view. Disclosure policy:

**Default: invisible (P6).** The reactive scenario answers from the current graph state silently. No footer. No banner. No "warming up" indicator. The user perceives a normal answer.

**Disclosure earns its place when staleness exceeds threshold.** A small footer appears appended to the response body **only if BOTH** of the following hold at query time:

| Condition | Recommended threshold (UX → design) | SST key (design-owned) |
|-----------|-------------------------------------|------------------------|
| Time since last successful re-synthesis tick | > 60 minutes | `enrichment.disclosure.staleness_minutes` |
| Re-synthesis backlog depth (pending jobs) | > 100 jobs | `enrichment.disclosure.backlog_threshold` |

Both thresholds MUST trip together to fire disclosure. Single-condition disclosure (e.g., "60min since last tick" alone) would chatter on quiet days; both-together captures "the graph is genuinely behind."

**Disclosure footer shape (rendered as the line above the `sources:` block):**

```
note: graph is catching up (N items pending, last enriched ~Xm ago)
```

- `N` = exact pending-job count (truthful; no rounding).
- `Xm` = minutes-since-last-tick rounded to 5m increments (avoids second-by-second jitter without misleading).
- Footer is one line; never wraps; phone-screen-fit (P7).
- Disclosure does NOT change the answer — same retrieval, same synthesis, same `sources[]`. It only labels the staleness window.

**Why disclosed-only-above-threshold (grounded in P6 + P8):**
- P6 (invisible by default) means absence of noise is the default state. A perpetual "graph status" indicator violates this.
- P8 (transparency) means the user MUST be told when the answer might genuinely mislead. A graph 5 minutes behind on 3 pending jobs is not misleading; a graph 6 hours behind on 800 pending jobs is.
- The both-conditions AND-gate is the asymmetric trigger: it fires when there is actual reason to disclose, not on the routine drain.

**User-initiated force-enrich:** Out of scope for v1. Spec 063 does NOT expose an "enrich now" affordance — that would be a new user-facing surface (violates §11 out-of-scope: "no new UI screens"). If demand surfaces post-v1, route to a separate spec; the substrate (producer queue + scheduler) supports it without architectural change.

**Adversarial check:** Footer MUST NOT appear when both thresholds are below trip (verified by test). Footer MUST appear when both are above trip (verified by test). One-condition cases (only stale OR only backlogged) MUST NOT fire (verified by test).

### 14.C LLM vs Heuristic Resolution (resolves OQ-6)

When LLM relationship inference (R-4) contradicts an existing heuristic SQL clustering signal — e.g., LLM infers `INFERRED_RELATED` between A5 and A6, but the heuristic `BELONGS_TO`-cluster groups A5 with A7 instead, NOT A6 — the resolution policy is:

**Heuristic synthesis wins by default as the durable canonical signal.** The heuristic SQL clustering (spec 025 `synthesis.go`) is the production-stable, deterministic, replayable substrate. LLM inference is additive, soft, and provenance-tagged. The LLM never silently overwrites a heuristic edge or a heuristic-authored concept-page claim.

**Storage shape (UX commits user-facing behavior; design owns OQ-3/OQ-4 storage choice):**

| Signal source | Storage | Mutability | Visible in reactive answers |
|---------------|---------|------------|------------------------------|
| Heuristic SQL clustering (existing) | `edges` table, heuristic `edge_type` (`BELONGS_TO`, etc.) | Authoritative; only mutated by heuristic re-runs. | Primary citation. |
| Heuristic concept-page text (spec 025) | `concept_pages` table | Mutated by re-synthesis only when no-thinning guard passes (R-3). | Primary body. |
| LLM-inferred relationship | `edges` table, `INFERRED_*` `edge_type` (new) with confidence + `llm_run_id` | Append-only; never overwrites a heuristic edge. | Secondary "also suggests…" hint when divergence is material. |
| LLM-augmented "why" prose | `why_context` attached to parent alert (design owns column-vs-sibling per OQ-3) | Re-runnable; overwrite of own prior `why_context` allowed under provenance bump. | Inherited from parent surface (spec 062 nudge). |

**Reactive surface behavior when both signals fire on the same query:**

- **Agree** (LLM and heuristic both link the same artifacts): single combined citation, no special rendering. The user perceives a normal sourced answer.
- **LLM adds where heuristic is silent** (LLM finds a connection the SQL clustering didn't): rendered as a secondary line under the primary body: `also related (inferred, confidence X.XX): <short label>` with citation. Confidence MUST be rendered as a two-decimal float (e.g., `0.82`); never as "high"/"medium"/"low" word-class (that's lossy and invites overtrust).
- **Disagree materially** (LLM contradicts a heuristic signal that the answer cites): rendered as a transparency disclosure: `note: 3 signals agree (heuristic clustering, …); 1 dissents (LLM inference, confidence X.XX) — both shown.` Both citations are surfaced; neither is hidden. The user decides.

**Why heuristic-canonical (grounded in P8 + existing production behavior):**
- Heuristic SQL clustering has been production-stable since spec 025 shipped (`done` per spec-dashboard). Replacing or letting LLM silently override it would break the existing observability story (operators can no longer reason about why two artifacts are linked).
- P8 (transparency) demands the user see disagreement, not have it hidden by an authority choice. Surfacing both honors transparency; picking one and hiding the other does not.
- Adversarial: the policy MUST NEVER silently delete a heuristic edge in favor of an LLM inference, AND MUST NEVER hide a divergent LLM signal that the producer accepted above the confidence floor.

### 14.D Consolidation Surfacing (resolves OQ-7)

The topic-consolidation analyzer (R-8) detects merge / keep-separate / needs-more-data candidates when user edits topics (SCN-063-008). UX policy for **when** and **how** these candidates surface to the user:

**Recommended: persisted-but-inert with two pull-only surfaces.**

| Surface | Trigger | UX shape | Principle alignment |
|---------|---------|----------|---------------------|
| Reactive ask via facade | User asks the conversational surface "what duplicates exist?" / "should I merge T1 and T2?" / "are there overlapping topics?" | Facade returns `consolidation_candidate` list sorted by confidence; each entry shows `decision`, two-decimal confidence, top-3 evidence artifact IDs. Refuses with structured `insufficient_evidence` body when no candidates above floor exist. | P1 (observe first, ask second) — the system detects but does not push. |
| In-flow contextual (future UX) | User opens the topic-edit screen for T1 (future spec; not v1 here). | Topic-edit screen reads `consolidation_candidate` rows for T1 and renders inline (design + future-UX spec own the rendering). | P6 (invisible by default until user enters the relevant context). |

**Forbidden v1 surfaces (these would violate spec 063 boundaries):**
- ❌ Proactive Telegram nudge ("you have 3 duplicate topics") — that is spec 062's territory and would require a new nudge thread; spec 063 fires zero notifications (R-13).
- ❌ Auto-merge based on confidence — even at confidence 1.0, the analyzer MUST present as suggestion, never as fait accompli. User edits to the topic graph are user-owned actions (P1).
- ❌ Banner / badge / counter in any global navigation — that surfaces enrichment as a permanent attention sink (violates P6).

**Why persisted (not reactive-only computation):**
- Reactive-only would mean the system re-runs the LLM analysis on every user ask, costing tokens and adding latency. The producer runs async per topic edit; persistence is the natural shape and lets the reactive surface return in <5s (NFR-1).
- Persistence is **inert** — no surface renders it without an explicit user trigger (ask or in-flow context). The data sits in the graph as cold storage until pulled.

**Storage shape:** design-owned (intersects OQ-7 → bubbles.design + bubbles.plan). UX commits only that the row MUST carry `{topic_pair_ids, decision, confidence, evidence_artifact_ids, llm_run_id, prompt_contract_version, created_at}` per R-8 + R-10. Design picks: sibling table vs JSON column on `topics`, retention policy, supersession semantics on re-analyzed pairs.

**Refusal copy (when user asks reactively and no candidates exist):**
```
no consolidation candidates above the confidence floor right now.
the analyzer last ran ~Xm ago over N topic edits.
```
- Truthful (cites last-run + edit count); no fabrication.
- Phone-screen-fit (P7); two lines.
- Inherits spec 061 §14.A.7 error-line discipline.

### 14.E Token-Cost Cap UX (resolves OQ-10)

When the daily token budget `enrichment.daily_token_budget` (SST, fail-loud, design-owned key naming) approaches exhaustion, producers and reactive scenarios degrade in this order:

| Budget state | Producer behavior | Reactive-scenario behavior | User-perceived shape |
|--------------|-------------------|----------------------------|----------------------|
| Healthy (< 80% of daily budget consumed) | Run normally on configured model (`deepseek-r1:7b` or per-producer choice). | Run normally on configured model. | No footer. No disclosure. |
| Approaching cap (80–100% of daily budget consumed) | Continue on configured model until cap is hit. | **Downgrade to fast/cheap model** (`gemma3:4b`); answer body unchanged in shape; append disclosure footer. | Footer line below body: `note: using fast model (daily budget threshold reached); answer may be terser.` |
| Cap exhausted (cheap model also exceeds remaining budget) | **Refuse** the job; record `refusal_reason=daily_token_budget_exhausted` (NFR-2/NFR-3 observability counter). | **Refuse** the reactive query with structured `insufficient_capacity` body. | Refusal body: `daily LLM budget exhausted; reactive answers resume tomorrow. you can still capture this as an idea.` Inherits spec 061 capture-as-fallback. |

**Why disclosed-downgrade-first (grounded in P8 + P9):**
- Silent downgrade (cheaper model, no footer) violates P8 (transparency) — the user reads a terser answer and assumes the system "knows less today" without knowing the actual cause.
- Refuse-on-first-cap-hit violates P9 (design for restart, not perfection) — the user is punished for the system's cost ceiling on the very query they came for.
- Disclosed downgrade + reactive-capture fallback honors both: the user gets an answer (possibly terser), knows why, and can still capture intent. Refusal is the last resort, after the cheap model also exceeds.

**Adversarial check:**
- Footer text MUST appear when and only when the downgrade fires (verified by test).
- Refusal body MUST appear when cheap model exceeds (verified by test); the user MUST NOT see a silent timeout or a fabricated "best-effort" answer.
- The downgrade path MUST NOT silently switch models without the footer; that would be the worst-of-both-worlds (terser AND opaque).

**Cap reset:** Cap resets at calendar-day boundary in operator-configured timezone (design-owned: SST key for timezone). UX commits that the reset is deterministic and user-comprehensible — never "rolling 24h window" (harder to reason about) without explicit operator opt-in.

### 14.F Refusal Copy (per producer / surface)

Spec 063 producers MUST refuse rather than fabricate when graph evidence is insufficient (Hard Constraint #2, R-7, R-10, SCN-063-005, SCN-063-007, SCN-063-010). User-facing refusal language per surface:

| Producer / surface | Refusal trigger | User-facing copy | Where rendered |
|--------------------|-----------------|-------------------|----------------|
| Reactive `knowledge_lookup` (R-9, SCN-063-010) | Zero concept pages OR artifacts above retrieval floor for the query topic. | `i don't have enough evidence about <topic> to answer confidently. the graph has N artifacts on this topic; i'd want at least M.` Then inherits spec 061 §14.A.7 capture-as-fallback offer card. | Facade reply body. |
| Reactive consolidation ask (§14.D refusal) | No `consolidation_candidate` above confidence floor. | `no consolidation candidates above the confidence floor right now. the analyzer last ran ~Xm ago over N topic edits.` | Facade reply body. |
| Re-synthesis no-thinning refusal (R-3, SCN-063-002) | New output strictly thinner than existing concept page. | **NOT user-facing.** Recorded as `refusal_reason=would_thin_existing_content` in provenance + Prometheus counter (NFR-3). Existing concept page is preserved silently. | Logs + metrics only. |
| Relationship-inference confidence-floor refusal (R-5, SCN-063-005) | LLM confidence below `enrichment.relationship_inference.confidence_floor`. | **NOT user-facing.** Recorded as `refusal_reason=below_confidence_floor` in logs + metrics. No edge persisted. | Logs + metrics only. |
| Why-augmenter empty-evidence refusal (R-7, SCN-063-007) | Parent alert has empty `source_artifact_ids` OR LLM cites IDs outside the set. | **NOT user-facing directly.** Parent surface (spec 062 nudge / future UX) renders WITHOUT a `why_context` block — i.e., the alert/nudge ships with body + sources but no "why" line, rather than with fabricated rationale. | Inherited from parent surface; spec 063 emits nothing user-facing. |
| Budget-exhausted refusal (§14.E) | Cheap-model fallback also exceeds remaining daily budget. | `daily LLM budget exhausted; reactive answers resume tomorrow. you can still capture this as an idea.` | Facade reply body. |

**Refusal copy invariants (grounded in P8):**
- Every user-facing refusal MUST state the **cause** in user terms (not "errorCause=insufficient_evidence" jargon).
- Every user-facing refusal MUST offer a **next action** (capture as idea, ask again later, etc.) or explicitly state none exists.
- Never end on a wall — even budget-exhausted offers capture-as-fallback.
- Never fabricate a degraded answer ("here's what i think though…"); refusal is the answer.

### 14.G Reactive Query Latency Budget

| Path | Target | Rationale |
|------|--------|-----------|
| Reactive `knowledge_lookup` (R-9, SCN-063-009/010) | p95 < 5s | Matches spec 061 §3 facade SLA and spec 062 §14.G — consistent UX across reactive surfaces. User waits with inherited `status=thinking`. |
| Reactive consolidation ask (§14.D) | p95 < 5s | Same budget — consolidation candidates are persisted (no fresh LLM call on read path), so this is a graph-read + render budget. |
| Reactive refusal (§14.F) | p95 < 5s | Refusal IS the answer; same budget. |
| Async producer ticks (R-1/R-4/R-6/R-8) | No user-perceived target | Async; per-tick wall-clock is design-owned operational concern (NFR-2/NFR-5). |
| Budget-downgrade detection (§14.E) | MUST NOT push reactive p95 past 5s | If downgrade decision itself adds > 200ms, design MUST cache the budget state with a short TTL. Reactive p95 budget is binding; the downgrade path is an internal optimization that MUST stay inside it. |

Model selection (`deepseek-r1:7b` vs `gemma3:4b` vs other) is design-owned. UX commits only to user-perceived p95 and the §14.E disclosed-downgrade behavior. If a model swap would push p95 past budget, design MUST either pre-select the faster model or shorten the prompt — UX does NOT relax the 5s ceiling.

---
