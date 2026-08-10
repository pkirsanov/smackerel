# Feature: 110 Retrieval Quality Foundation

**Status:** `not_started` · **Workflow mode:** `full-delivery` · **Release train:** `mvp`
**Authored by:** `bubbles.analyst` (requirements only — no source file changed)
**Plan of record:** [`docs/Product_Delivery_Plan.md`](../../docs/Product_Delivery_Plan.md) §3 P8, §4 Stage 4
**Diagnostic evidence:** [`docs/Product_Direction_2026-07-31.md`](../../docs/Product_Direction_2026-07-31.md) D1, D2, D3, D5, D15, VAL-1
**Evidence re-verified:** 2026-08-04 against the working tree (commands in §3)

---

## 1. Problem Statement

A user asks for something they know they saved. Retrieval returns nothing useful, or
returns the right document without showing which part of it answered the question.

This is not one defect. It is **four compounding defects in the same substrate**, plus a
fifth that degrades ingest as the corpus grows. Each one alone would be recoverable. Stacked,
they mean the product cannot honour Product Principle 2 — *Vague In, Precise Out* — whose
stated contract is **>75% correct on first result for vague queries**, and there is no
executed measurement anywhere in the repository that says whether it does.

**The four retrieval defects.**

1. **Retrieval granularity is the whole artifact.** There are no chunk rows. A
   forty-page paper and a one-line note carry exactly one vector each, so a fact in
   the middle of a long document has no independent representation to match against.
2. **What is indexed is a summary, not the content.** The embedded text is
   title + short summary + up to five key ideas. Any fact the extraction step did not
   promote into that abstraction is **unreachable by vector search at all** — not
   ranked low, absent.
3. **The configured model is not the running model.** The single-source-of-truth
   declares one embedding model; the runtime hardcodes a different one, and hardcodes
   its dimension. Two sources disagree and neither fails.
4. **The index is unproven in both directions.** The vector index is IVFFlat with a
   fixed list count and the runtime never sets the probe count. Either the query uses
   the index and inspects too few lists to be accurate, or it does not use the index
   and scans every vector. No executed plan or corpus-scale latency measurement
   exists to settle which. **Both branches are unacceptable**, which is why "we do
   not know" is itself the finding.

**The fifth defect (ingest-side).** The temporal linker pairs artifacts with a
cross-product query shape and wraps the timestamp column in a function, which prevents
the existing created-time index from serving the predicate. Cost grows with corpus size
on every ingest.

**Why this cannot be closed by "add a better test."** The repository already contains a
gate whose thresholds the SST calls non-negotiable and which **executes in no automated
lane** — `tests/eval/assistant/acceptance_test.go` carries `//go:build integration`, and
the integration lane's package list does not include `./tests/eval/...`. Adding a
retrieval-quality gate without also proving it *ran* would reproduce that exact failure.
This spec therefore treats "the gate executed and asserted something" as a first-class
requirement, not a testing detail.

---

## 2. Outcome Contract

**Intent.** A user can ask a vague question about something in their own corpus, get the
right artifact back, and see **the specific passage that answered them** — and the
product can state, from an executed measurement rather than an assumption, how often that
is true.

**Success Signal.** A named, automatically-executed test lane reports, for a fixed
evaluation corpus of at least 100 vague queries with known answer artifact ids: accuracy@1,
accuracy@5, recall@20, p95 query latency, the query-plan node actually used, and a
**non-zero count of assertions executed**. The lane fails when any metric falls below its
declared floor **and** fails when the executed-assertion count is zero.

**Hard Constraints.**

- Exactly one declared source of truth resolves the embedding model identity **and** its
  vector dimension. Any disagreement between that source and the runtime, or between the
  declared dimension and the stored vector width, refuses startup with a named error.
- A retrieval result that cites a passage MUST cite a passage that exists in the stored
  artifact content. Retrieval never invents evidence.
- Re-embedding an existing corpus is resumable and does not lose or duplicate artifacts.
- Retrieval quality floors are set at the **measured** baseline and can only move
  upward by a recorded decision. They cannot regress silently.

**Failure Condition.** All tests pass, the code merges, and one of the following is still
true: the eval lane executed zero assertions; the floors were set to values nobody
measured; or a user can still not find a fact that exists in the middle of a stored
document. Any one of those means this feature failed even with a green build.

---

## 3. Evidence Base (verified 2026-08-04)

Every row was produced by a command run against this repository root on 2026-08-04. No
product build, no test run, no live stack, no database query.

| # | Claim | Verification | Result |
|---|---|---|---|
| E1 | No chunk table exists | `grep -r 'artifact_chunks' internal/db/migrations/` | **0 matches** across 62 active + 17 archived migrations |
| E2 | Embedded text is an abstraction | `ml/app/embedder.py` `generate_artifact_embedding(title, summary, key_ideas)` joins `title` + `summary` + `key_ideas[:5]` | confirmed |
| E3 | Model is hardcoded | `ml/app/embedder.py` module global `_model_name = "all-MiniLM-L6-v2"` | confirmed |
| E4 | Dimension is hardcoded | `ml/app/embedder.py` `embedding_dimension()` returns literal `384` with comment `all-MiniLM-L6-v2 fixed at 384` | confirmed |
| E5 | SST declares a different model | `config/smackerel.yaml` `embedding_model: nomic-embed-text` | confirmed — **E3/E4 and E5 disagree** |
| E6 | The stored column matches the hardcoded model, not the declared one | `internal/db/migrations/001_initial_schema.sql` `embedding vector(384)` | confirmed — a model change is also a **column-width migration** |
| E7 | Index is IVFFlat, fixed lists | `001_initial_schema.sql` `USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)` | confirmed |
| E8 | Probe count is never set | `grep -rn 'ivfflat.probes' --include='*.go' --include='*.sql' --include='*.yaml'` | **0 matches** |
| E9 | No approximate-nearest-neighbour graph index exists | `grep -rin 'hnsw' --include='*.go' --include='*.sql'` | 1 match, and it is a **test fixture sentence** in `tests/eval/assistant/harness_test.go`, not an index |
| E10 | The vector query joins an annotation table | `internal/api/search.go` `vectorSearch` — `FROM artifacts a LEFT JOIN artifact_annotation_summary aas …` with `1 - (a.embedding <=> $1::vector)` | confirmed — this is the join whose plan is unknown |
| E11 | Temporal linker is cross-product + function-wrapped | `internal/graph/linker.go` `linkByTemporal` — `FROM artifacts a1, artifacts a2 … DATE(a2.created_at) = DATE(a1.created_at)` | confirmed |
| E12 | The precedent gate runs in no lane | `tests/eval/assistant/acceptance_test.go` line 1 is `//go:build integration`; `grep -c 'tests/eval' scripts/runtime/go-integration.sh` | **0** — the eval package is absent from the integration lane's package list |

**Evidence limit, stated rather than glossed.** E1–E12 are established by reading source
and configuration. **No EXPLAIN was executed and no latency was measured.** D1 is therefore
recorded here as an *unresolved ambiguity with two unacceptable branches*, not as a measured
result. Establishing which branch is true is delivery work under this spec, not a claim it
already makes.

---

## 4. Domain Capability Model

Capability-first proportionality applies: this feature introduces a **fusion strategy over
multiple retrieval signals** and a **measurement contract** that other retrieval work will
reuse. The domain model is therefore defined before any concrete index, model, or query.

### 4.1 Primitives

| Primitive | Definition | Lifecycle |
|---|---|---|
| **Corpus Item** | A stored unit of user knowledge with durable identity and retrievable content. | created → indexed → re-indexed → deleted |
| **Passage** | A bounded, ordered span of a Corpus Item's content, independently addressable and independently matchable. Passages of one item overlap at their boundaries so a fact spanning a boundary is not lost. | derived → indexed → superseded on re-index |
| **Semantic Index Identity** | The triple (model identity, vector dimension, index generation) under which every stored vector was produced. Vectors produced under different identities are **not comparable**. | declared → active → superseded |
| **Retrieval Signal** | One scored opinion about how well a Corpus Item answers a query. Distinct signals may disagree; none is authoritative alone. | computed per query |
| **Fused Result** | One Corpus Item, one comparable score derived from its Retrieval Signals, and the **winning Passage** retained as displayed evidence. | computed per query |
| **Evaluation Case** | A vague natural-language query paired with the identity of the Corpus Item that correctly answers it. | authored → frozen → measured against |
| **Quality Floor** | A declared minimum for one measured retrieval metric, whose value originates from an executed measurement. | measured → declared → raised |

### 4.2 Relationships

- A Corpus Item has one or more Passages. An item short enough to fit one Passage still
  has exactly one — there is no special-case path for short items.
- Every Passage and every item-level vector carries exactly one Semantic Index Identity.
- A Fused Result cites exactly one winning Passage, and that Passage belongs to the item
  the result names.
- A Quality Floor is meaningless without the Evaluation Case set it was measured over;
  the two are versioned together.

### 4.3 Policies every implementation must obey

| ID | Policy |
|---|---|
| **P110-1** | Semantic Index Identity is resolved from exactly one declared source. A runtime that cannot confirm agreement between declaration, running model, and stored vector width **refuses to serve**, and says which of the three disagreed. |
| **P110-2** | Comparing vectors across different Semantic Index Identities is forbidden. A mixed corpus is a detectable, reported state — never a silently degraded one. |
| **P110-3** | A signal is only a Retrieval Signal if its score is comparable within its own kind. Signals of different kinds are fused by a declared rule, never by ranking raw incomparable numbers against each other. |
| **P110-4** | Displayed evidence is derived from stored content, never generated. |
| **P110-5** | A Quality Floor whose value was not produced by an executed measurement is invalid, regardless of whether the gate is green. |
| **P110-6** | A retrieval quality gate that executes zero assertions is a **failure**, not a pass. Absence of failure is not evidence of execution. |

---

## 5. Design Constraints Inherited From The Plan Of Record

These name mechanisms rather than outcomes. They are recorded separately from the
requirements because they are **inherited constraints, not derived ones**. `bubbles.design`
may depart from any of them only with a recorded justification in `design.md`.

| ID | Constraint | Source |
|---|---|---|
| **C110-1** | Passages are persisted as `artifact_chunks(artifact_id, ordinal, text, embedding)` with bounded overlap. | Plan §3 P8 "Exact change" |
| **C110-2** | The vector index migrates from IVFFlat to HNSW, tuned from the SST. | Plan §3 P8; D1 recommendation |
| **C110-3** | Re-embedding is delivered as a **resumable migration**, not a one-shot backfill. | Plan §3 P8 |
| **C110-4** | Fusion combines chunk score, summary-vector score and lexical score into one artifact-level result. | Plan §3 P8 |
| **C110-5** | The evaluation corpus is ≥100 vague queries with known answer ids; recorded metrics are accuracy@1, accuracy@5, recall@20, p95, and the plan node. | Plan §3 P8; §4 Stage 4 |
| **C110-6** | The gate is wired into a **named lane** and asserts a non-zero executed-assertion count. | Plan §3 P8 ("otherwise it repeats P3") |
| **C110-7** | Files in scope: `ml/app/embedder.py`, `ml/app/nats_client.py`, `internal/pipeline/processor.go`, `internal/api/search.go`, `internal/graph/linker.go`, plus one migration, one eval corpus, one lane change. | Plan §3 P8 "Files" |

---

## 6. Actors & Personas

| Actor | Description | Goal in this feature | Authority |
|---|---|---|---|
| **Daily user** | Owner of the corpus, asking vague questions in natural language. | Find the fact; see which passage answered. | Reads own corpus through the existing authorization boundary. This spec does not change who may read what. |
| **Operator** | Runs the self-hosted deployment; owns model selection and the re-index decision. | Change the embedding model deliberately, know when re-embedding is complete, and roll back without data loss. | Declares Semantic Index Identity; starts, resumes and observes re-embedding. |
| **Ingest pipeline** | Non-human producer of Passages and vectors. | Produce comparable vectors under exactly one identity. | Writes Passages; never chooses the identity. |
| **Quality gate** | Non-human consumer of the Evaluation Case set. | Measure and refuse regression. | Blocks; never repairs. |

---

## 7. Use Cases

### UC-110-001 — Daily user finds a fact buried in a long document
- **Actor:** Daily user
- **Preconditions:** A long artifact is stored and indexed; the fact appears once, mid-document, and is absent from the artifact's summary and key ideas.
- **Main flow:** User asks a vague natural-language question → system matches passages, not only whole items → system fuses signals into one result per item → system returns the item with the winning passage shown as evidence.
- **Alternative flow (no confident match):** System reports that it found nothing confident, and does not present a low-confidence item as an answer.
- **Postconditions:** The displayed passage is a verbatim span of that item's stored content.

### UC-110-002 — Operator changes the embedding model deliberately
- **Actor:** Operator
- **Preconditions:** A corpus exists, indexed under the current Semantic Index Identity.
- **Main flow:** Operator declares a new identity → runtime detects that stored vectors were produced under the previous identity → runtime reports the mismatch and refuses to serve mixed-identity comparisons → operator starts re-embedding → progress is observable → on completion the new identity becomes active.
- **Alternative flow (interrupted):** Re-embedding is interrupted; on restart it resumes from its recorded position without reprocessing completed items and without skipping any.
- **Postconditions:** Every stored vector carries exactly one identity; the count of items under each identity is reportable.

### UC-110-003 — Operator learns whether the index is actually used
- **Actor:** Operator
- **Preconditions:** A corpus of realistic size exists.
- **Main flow:** The query-plan check executes the real retrieval query → records the plan node actually chosen → asserts it is the expected index node → records p95 latency at corpus scale.
- **Alternative flow (plan is a full scan):** The check **fails** and names the observed node. It does not pass with a warning.
- **Postconditions:** The ambiguity recorded as D1 is replaced by a measured fact.

### UC-110-004 — Quality gate refuses a silent regression
- **Actor:** Quality gate
- **Preconditions:** Evaluation Case set and Quality Floors are declared and versioned together.
- **Main flow:** Lane runs → each case is executed → metrics computed → each compared to its floor → assertion count reported.
- **Alternative flow (gate did not run):** The executed-assertion count is zero and the lane **fails**, naming the lane and the empty selector.
- **Postconditions:** No change can lower a floor without that being visible as a declared decision.

---

## 8. Business Scenarios (Gherkin)

### Retrieval granularity and evidence

#### SCN-110-A01 — A mid-document fact is retrievable
```gherkin
Given an artifact whose stored content is long enough to span several passages
  And the fact the user is looking for appears once, in the middle of that content
  And that fact appears in neither the artifact's summary nor its key ideas
When the user asks a vague natural-language question whose answer is that fact
Then the artifact is returned among the results
  And the passage containing the fact is shown as the evidence for that result
```

#### SCN-110-A02 — Displayed evidence is real content, never generated
```gherkin
Given a retrieval result that cites a passage as its evidence
When the cited passage is compared against the stored content of the artifact it names
Then the cited passage is a verbatim span of that artifact's stored content
```

#### SCN-110-A03 — A short artifact needs no special case
```gherkin
Given an artifact whose entire content fits within a single passage
When it is indexed
Then it has exactly one passage
  And it is retrievable and citable by the same path a long artifact uses
```

#### SCN-110-A04 — A fact spanning a passage boundary is not lost
```gherkin
Given a fact whose wording spans the boundary between two adjacent passages
When the user asks a question whose answer is that fact
Then the artifact is returned
  And the cited passage contains the whole fact
```

### Semantic index identity

#### SCN-110-B01 — Declared and running identity must agree
```gherkin
Given the declared embedding model identity differs from the model the runtime would load
When the system starts
Then it refuses to serve retrieval
  And the refusal names the declared identity, the running identity, and which source declared each
```

#### SCN-110-B02 — Declared dimension and stored vector width must agree
```gherkin
Given the declared embedding dimension differs from the width of the stored vector column
When the system starts
Then it refuses to serve retrieval
  And the refusal names both widths
```

#### SCN-110-B03 — A mixed-identity corpus is reported, never silently compared
```gherkin
Given some stored vectors were produced under a previous semantic index identity
When retrieval is requested
Then the system does not compare vectors across identities
  And the number of items under each identity is reportable to the operator
```

#### SCN-110-B04 — Re-embedding resumes without loss or duplication
```gherkin
Given a re-embedding run over an existing corpus is interrupted partway
When the run is started again
Then it resumes from its recorded position
  And every item is re-embedded exactly once across the two runs
  And no item is left under the superseded identity when the run reports completion
```

### Query plan and latency

#### SCN-110-C01 — The plan node is asserted, not assumed
```gherkin
Given an evaluation corpus of realistic size
When the real retrieval query is executed under plan inspection
Then the plan node actually used is recorded
  And the check fails if that node is a full scan of the stored vectors
```

#### SCN-110-C02 — Latency is measured at corpus scale
```gherkin
Given an evaluation corpus of realistic size
When the evaluation query set is executed
Then a p95 query latency is recorded from those executions
  And it is compared against a floor whose value came from a previous executed measurement
```

#### SCN-110-C03 — Ingest linking stays bounded as the corpus grows
```gherkin
Given the corpus has grown substantially
When a new artifact is ingested and linked to temporally related artifacts
Then the candidate lookup is bounded rather than proportional to the whole corpus
  And its plan uses the existing created-time index
```

### The gate itself

#### SCN-110-D01 — Retrieval accuracy is measured against known answers
```gherkin
Given an evaluation set of at least 100 vague queries, each with a known answer artifact id
When the retrieval quality lane runs
Then accuracy@1, accuracy@5 and recall@20 are computed from those executions
  And each is compared against its declared floor
  And the lane fails if any metric is below its floor
```

#### SCN-110-D02 — A gate that did not run is a failure
```gherkin
Given the retrieval quality gate is wired into a named lane
When the lane runs and the gate's assertions do not execute
Then the lane fails
  And the failure names the lane and reports an executed-assertion count of zero
```

#### SCN-110-D03 — Floors cannot be lowered silently
```gherkin
Given declared quality floors that originated from an executed measurement
When a change would lower any floor
Then the change is visible as an explicit declared decision
  And the lane does not pass by adopting the newly measured lower value automatically
```

#### SCN-110-D04 — The gate is discoverable from the documented command surface
```gherkin
Given an engineer runs the repository's documented test command for this lane
When the command completes
Then the retrieval quality gate's assertions are among those executed
  And no additional manual, undocumented invocation was required
```

---

## 9. Requirements

Tech-agnostic. Each maps to at least one scenario.

| ID | Requirement | Scenarios |
|---|---|---|
| **R-110-01** | Retrieval MUST be able to match a bounded passage of an item's content independently of the item as a whole. | A01, A03 |
| **R-110-02** | Adjacent passages MUST overlap sufficiently that a fact spanning a boundary remains wholly present in at least one passage. | A04 |
| **R-110-03** | A retrieval result MUST cite the winning passage, and the citation MUST be a verbatim span of that item's stored content. | A01, A02 |
| **R-110-04** | Indexed text MUST include a representation of the item's actual content, not only a generated abstraction of it. | A01 |
| **R-110-05** | The system MUST resolve model identity and vector dimension from exactly one declared source. | B01, B02 |
| **R-110-06** | Disagreement between declared identity, running model, or stored vector width MUST refuse startup with an error naming all disagreeing sources. | B01, B02 |
| **R-110-07** | Vectors produced under different semantic index identities MUST NOT be compared; a mixed corpus MUST be reportable. | B03 |
| **R-110-08** | Re-embedding MUST be resumable, exactly-once per item across interruptions, and MUST report progress and completion. | B04 |
| **R-110-09** | Retrieval MUST fuse its distinct signals by a declared rule into one comparable score per item. | A01 |
| **R-110-10** | Signals of different kinds MUST NOT be ranked directly against each other as if their raw scores were comparable. | A01 |
| **R-110-11** | The query plan actually used by the retrieval query MUST be recorded and asserted; a full scan of stored vectors MUST fail the check. | C01 |
| **R-110-12** | p95 query latency MUST be measured at evaluation-corpus scale and compared against a declared floor. | C02 |
| **R-110-13** | Ingest-time relationship candidate lookup MUST remain bounded as corpus size grows, and MUST NOT exclude the existing created-time index from its predicate. | C03 |
| **R-110-14** | An evaluation set of at least 100 vague queries with known answer ids MUST exist and be versioned together with its floors. | D01, D03 |
| **R-110-15** | The lane MUST report accuracy@1, accuracy@5, recall@20, p95 and the plan node, and MUST fail when any is below its floor. | D01, C01, C02 |
| **R-110-16** | The lane MUST report an executed-assertion count and MUST fail when that count is zero. | D02 |
| **R-110-17** | The gate MUST execute from the repository's documented command surface with no undocumented manual step. | D04 |
| **R-110-18** | Quality floors MUST originate from an executed measurement, and lowering one MUST be an explicit declared decision. | D03 |

---

## 10. Non-Functional Requirements

| ID | Requirement |
|---|---|
| **NFR-110-1** | Re-embedding MUST NOT require taking retrieval offline for the whole run; degraded behaviour during the run MUST be honestly reported rather than silently mixed. |
| **NFR-110-2** | Re-embedding MUST be safe to interrupt at any point, including mid-item, without corrupting the stored index state. |
| **NFR-110-3** | The evaluation corpus MUST be synthetic or user-authored fixture data owned by the repository — never a snapshot of a real user corpus. |
| **NFR-110-4** | Evaluation runs MUST use ephemeral, disposable storage and MUST NOT write to any persistent or production data store. |
| **NFR-110-5** | Passage storage growth MUST be bounded and its multiplier over item count MUST be reportable, so an operator can size storage before re-embedding. |
| **NFR-110-6** | The gate's runtime MUST be short enough to stay in an automatically-executed lane; a gate too slow to run automatically has failed R-110-16 by another route. |

---

## 11. Non-Goals

1. **Changing who may read what.** Authorization of corpus reads is owned elsewhere. This spec changes *what retrieval finds*, never *whose corpus it searches*.
2. **Redefining graph edge semantics.** Edge type semantics and observational-versus-inferential ranking are a separate concern (plan §3 P7). This spec touches the temporal linker **only** for the bounded-lookup defect in R-110-13.
3. **Digest or synthesis ranking.** Relevance ranking for daily output is separate work (plan §3 P9/P10).
4. **A second retrieval store.** Product Principle 5 forbids a parallel index; passages extend the one graph rather than forking it.
5. **Choosing a specific embedding model for the operator.** This spec makes model identity *declared, verified and migratable*; which model is correct is an operator decision.

---

## 12. Open Findings (routed, not resolved here)

| ID | Severity | Finding | Owner |
|---|---|---|---|
| **F-110-DIM-01** | **BLOCKING** | Changing the declared model changes the vector width (E5/E6: declared model is not the 384-dimension one the column was sized for). The re-embed is therefore also a **column-type migration** on the corpus's largest table, with its own downtime, disk and rollback profile. The plan of record names re-embedding but does not name this. | `bubbles.design` |
| **F-110-PLAN-01** | **BLOCKING** | D1's two branches are still both open (E8: probes never set; E9: no graph index exists). Which branch is true determines whether the fix is primarily an accuracy fix or primarily a latency fix — and therefore what the first measured floors even mean. Nothing may declare a floor before this is executed. | `bubbles.design` → `bubbles.test` |
| **F-110-LANE-01** | **BLOCKING** | E12 shows the precedent failure is still live: `./tests/eval/...` is absent from the integration lane's package list. Adding this spec's gate to the same directory without also fixing the lane would ship a second gate that never runs. Whether the fix is extending the existing lane or declaring a new named one is a design call. | `bubbles.design` |
| **F-110-FLOOR-01** | HIGH | R-110-18 requires floors to come from measurement, but the first run has no prior measurement to compare against. The first execution is therefore **baseline-establishing, not gating**, and that transition must be an explicit recorded step or the first green run will be mistaken for a passed gate. | `bubbles.plan` |
| **F-110-CORPUS-01** | HIGH | NFR-110-3 forbids using a real user corpus, but "vague query with a known answer" is only meaningful against content rich enough to be ambiguous. Authoring 100+ genuinely vague cases over synthetic content is real work and is easy to under-scope into 100 keyword lookups, which would measure nothing. | `bubbles.plan` |
| **F-110-EGRESS-01** | MEDIUM | Passages persist a **second copy of raw content** in a new location. Every existing rule about content handling, retention and export must extend to it, or portability and deletion silently become incomplete. Interacts directly with spec 111's bundle record classes. | `bubbles.design` → spec 111 |
| **F-110-SUMMARY-01** | MEDIUM | R-110-04 requires indexing actual content; C110-4 retains a summary-vector signal in the fusion. Whether the summary vector remains a *distinct* signal or becomes redundant once passages exist is unresolved, and keeping a redundant signal would add cost without evidence of benefit. | `bubbles.design` |
| **F-110-METRIC-01** | LOW | Principle 2 states the product metric as ">75% correct on first result for vague queries". Whether that number becomes the declared accuracy@1 floor, or whether the measured baseline is lower and the gap is recorded honestly, is a product decision — not one the gate should make by adopting whatever it first measures. | operator via `bubbles.plan` |

---

## Product Principle Alignment

Bound by [`.github/instructions/product-principles.instructions.md`](../../.github/instructions/product-principles.instructions.md) (Principles 1–10 ratified 2026-06-03, Principle 11 ratified 2026-07-29; BLOCKING).

| Principle | Alignment | Evidence |
|---|---|---|
| **2 — Vague In, Precise Out** | The load-bearing principle. Its stated contract (>75% correct on first result for vague queries) currently has no executed measurement anywhere in the repository. This spec makes the contract measurable and enforced, and R-110-01/02/03 make a mid-document fact reachable rather than only a matching title. | R-110-01, R-110-03, R-110-14, R-110-15; SCN-110-A01, SCN-110-D01 |
| **5 — One Graph, Many Views** | Passages **extend** the existing artifact store as a related record class; they are not a parallel search store. Explicitly reinforced as Non-Goal 4. | Non-Goal 4; §4.2 |
| **8 — Trust Through Transparency** | A result now shows *which passage* answered, and R-110-03 requires that passage to be verbatim stored content. A cited passage that is generated rather than stored is a defect, not a presentation choice. | R-110-03; SCN-110-A02 |
| **4 — Source-Qualified Processing** | No source metadata is stripped. Passaging changes retrieval granularity only; source qualifiers remain attached to the item. | Non-Goal 2 |
| **3 — Knowledge Breathes** | Passages are lifecycle participants, not permanent orphan state: they are derived on index and superseded on re-index. A passage set that outlived its item or its index identity would violate this principle. | §4.1 Passage lifecycle; R-110-07 |
| **11 — Local-First Data Ownership** | Passages persist a second copy of user content. F-110-EGRESS-01 records that export and deletion must extend to that copy or unconditional exit silently becomes partial. This spec does **not** claim to close that; it routes it to spec 111. | F-110-EGRESS-01 |

**Deviations:** none. **Tension recorded:** Principle 11's unconditional-exit guarantee is *weakened* by adding a new content-bearing record class until spec 111 covers it. That is stated as F-110-EGRESS-01 rather than assumed away.

---

## Release Train

Targets the **`mvp`** train (`config/release-trains.yaml` — `id: mvp`, `phase: active`,
`target_slot: home-lab`, `flags_bundle: config/feature-flags.mvp.yaml`).

**Why `mvp`.** This is a defect fix in the capability the currently-deployed train already
claims to provide. Product Principle 2 is an MVP promise, the plan of record places P8 in
Stage 4 on the critical path (*"Pillars A and C both depend on it"*), and the failure mode —
a user cannot find a fact they saved — is experienced on the train that is live today. A
correctness fix to a shipped promise belongs on the train that shipped it.

**Behaviour on `next`.** Unchanged. The flag ships `false` in
`config/feature-flags.next.yaml`, so the `next` train continues to serve the existing
single-vector retrieval path until `bubbles.train` promotes.

**Flag.** `flagsIntroduced: ["chunkedHybridRetrieval"]` — one flag, gating the cutover from
the single-vector path to the passage-plus-fusion path. The flag is genuinely required
rather than ceremonial: re-embedding an existing corpus is not instantaneous, so a
deployment will spend real time in a state where both representations exist, and the flag
is what decides which one serves reads during that window. Final flag name and the
default-ON/default-OFF bundle edits are owned by `bubbles.train`; this spec does not edit
`config/feature-flags.*` or `config/release-trains.yaml`.

**Flag lifecycle.** The flag retires one cycle after `chunkedHybridRetrieval` is default-ON
in `mvp` and every deployed corpus reports zero items remaining under the superseded
semantic index identity (SCN-110-B03).
