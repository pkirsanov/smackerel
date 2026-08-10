# Scopes: 110 Retrieval Quality Foundation

**Status of this file:** `not_started` — **PROVISIONAL DECOMPOSITION.**
**Owner of this artifact:** `bubbles.plan`
**Created by:** `bubbles.analyst` as an honest initial artifact under explicit operator instruction (recorded in `state.json.executionHistory` and `report.md`).

---

## What This File Is, And What It Is Not

The scope boundaries, dependency order, implementation plans and test plans below are a
**provisional decomposition proposed by requirements analysis**. They are NOT a ratified
plan. `bubbles.plan` owns this artifact and may restructure any of it.

Every Definition-of-Done checkbox is **unchecked**, because nothing has been designed,
implemented, executed or evidenced. No scope has started. No evidence block appears
anywhere in this file, because there is no evidence.

**Three scopes below are blocked on unresolved findings, not merely unstarted.** Those
dependencies are recorded per scope rather than left for discovery during execution.

---

## Blocking Findings That Gate This Plan

| Finding | Blocks | Why the plan cannot be finalised without it |
|---|---|---|
| `F-110-PLAN-01` | Scope 03, and the floors in Scope 05 | Whether the query plan uses or bypasses the vector index determines whether this feature is primarily an accuracy fix or a latency fix. Floors declared before that measurement would be arbitrary. |
| `F-110-DIM-01` | Scope 01, Scope 02 | The declared model and the stored vector column width disagree, so re-embedding is also a column-type migration on the corpus's largest table. Sequencing, rollback and disk cost are unknown. |
| `F-110-LANE-01` | Scope 05 | The repository already has a gate in this directory that executes in no automated lane. Adding a second one there without fixing the lane would ship a gate that never runs. |
| `F-110-CORPUS-01` | Scope 05 | 100+ *genuinely vague* evaluation cases over synthetic content is real authoring work and trivially under-scoped into 100 keyword lookups, which would measure nothing. |
| `F-110-FLOOR-01` | Scope 05 | The first run establishes a baseline and cannot also gate against it. The baseline→gating transition must be an explicit step or the first green run will be misread as a passed gate. |

---

## Proposed Scope Table

| Scope | Title | Depends On | Primary requirements |
|---|---|---|---|
| 01 | Semantic Index Identity Contract | — | R-110-05, R-110-06, R-110-07 |
| 02 | Passage Model And Content Indexing | 01 | R-110-01, R-110-02, R-110-04, R-110-08 |
| 03 | Query Plan Measurement And Bounded Ingest Lookup | 01 | R-110-11, R-110-12, R-110-13 |
| 04 | Hybrid Fusion And Winning-Passage Evidence | 02 | R-110-03, R-110-09, R-110-10 |
| 05 | Evaluation Corpus, Executed Gate And Lane Wiring | 02, 03, 04 | R-110-14, R-110-15, R-110-16, R-110-17, R-110-18 |

---

## Scope 01: Semantic Index Identity Contract

**Status:** Not Started
**Depends On:** —
**Blocked by finding:** `F-110-DIM-01`

Makes model identity and vector dimension a single declared, verified fact, and makes
disagreement a loud refusal instead of a silent divergence.

### Scenarios Covered

`SCN-110-B01`, `SCN-110-B02`, `SCN-110-B03`

### Implementation Plan (provisional)

- Resolve model identity and dimension from the single declared source rather than a runtime literal.
- Verify declaration against the loaded model and against the stored vector column width at startup; refuse to serve on any disagreement, naming all three.
- Record the semantic index identity alongside every stored vector so mixed-identity corpora are detectable.
- Expose a per-identity item count so an operator can see migration progress and completion.

### Test Plan

| # | Test Type | Category | Description | Scenario |
|---|---|---|---|---|
| TP-01-01 | Unit | `unit` | Identity resolution reads exactly one declared source and rejects a second source of truth | SCN-110-B01 |
| TP-01-02 | Unit | `unit` | Startup refuses, naming all disagreeing sources, when declared identity differs from the loaded model | SCN-110-B01 |
| TP-01-03 | Unit | `unit` | Startup refuses, naming both widths, when declared dimension differs from stored vector width | SCN-110-B02 |
| TP-01-04 | Integration | `integration` | A corpus containing vectors under two identities is reported, and no cross-identity comparison is performed | SCN-110-B03 |
| TP-01-05 | Integration | `integration` | Per-identity item counts are reportable and correct against a seeded mixed corpus | SCN-110-B03 |

### Definition of Done

- [ ] Model identity and vector dimension resolve from exactly one declared source (R-110-05)
- [ ] Startup refuses on declared-vs-running model disagreement, naming both (R-110-06)
- [ ] Startup refuses on declared-vs-stored dimension disagreement, naming both (R-110-06)
- [ ] Every stored vector carries exactly one recorded semantic index identity (R-110-07)
- [ ] Cross-identity vector comparison is prevented, not merely discouraged (R-110-07)
- [ ] Per-identity item counts are reportable to the operator (R-110-07)
- [ ] `F-110-DIM-01` is resolved in `design.md` with the column-width migration sequenced, costed and given a rollback path
- [ ] TP-01-01 executed and passing with recorded raw output
- [ ] TP-01-02 executed and passing with recorded raw output
- [ ] TP-01-03 executed and passing with recorded raw output
- [ ] TP-01-04 executed and passing with recorded raw output
- [ ] TP-01-05 executed and passing with recorded raw output
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## Scope 02: Passage Model And Content Indexing

**Status:** Not Started
**Depends On:** Scope 01
**Blocked by finding:** `F-110-DIM-01`

Gives content a retrieval granularity smaller than the whole artifact, and indexes the
content itself rather than a generated abstraction of it.

### Scenarios Covered

`SCN-110-A01`, `SCN-110-A03`, `SCN-110-A04`, `SCN-110-B04`

### Implementation Plan (provisional)

- Persist bounded, ordered passages per artifact with overlap sufficient to keep a boundary-spanning fact whole.
- Index a representation of actual stored content, not only title, summary and key ideas.
- Deliver re-embedding as a resumable migration that records position, is exactly-once per item across interruptions, and reports progress and completion.
- Keep the short-artifact path identical to the long-artifact path — one passage, no special case.

### Test Plan

| # | Test Type | Category | Description | Scenario |
|---|---|---|---|---|
| TP-02-01 | Unit | `unit` | Passage division produces bounded, ordered passages with the declared overlap | SCN-110-A04 |
| TP-02-02 | Unit | `unit` | An artifact shorter than one passage yields exactly one passage via the same path | SCN-110-A03 |
| TP-02-03 | Integration | `integration` | A fact present only mid-content, absent from summary and key ideas, is matchable | SCN-110-A01 |
| TP-02-04 | Integration | `integration` | A fact spanning a passage boundary is wholly present in at least one passage | SCN-110-A04 |
| TP-02-05 | Integration | `integration` | Re-embedding interrupted mid-run resumes and processes every item exactly once | SCN-110-B04 |
| TP-02-06 | Integration | `integration` | Re-embedding reports completion only when zero items remain under the superseded identity | SCN-110-B04 |
| TP-02-07 | Stress | `stress` | Passage storage growth stays within the declared bound and its multiplier is reportable | NFR-110-5 |

### Definition of Done

- [ ] Passages are persisted per artifact, bounded and ordered (R-110-01)
- [ ] Adjacent passage overlap keeps a boundary-spanning fact whole in at least one passage (R-110-02)
- [ ] Indexed text includes a representation of actual stored content, not only a generated abstraction (R-110-04)
- [ ] Short artifacts use the same path as long artifacts with no special case (R-110-01)
- [ ] Re-embedding is resumable and exactly-once per item across interruption (R-110-08)
- [ ] Re-embedding reports progress and reports completion only when no item remains under the superseded identity (R-110-08)
- [ ] Re-embedding is safe to interrupt mid-item without corrupting stored index state (NFR-110-2)
- [ ] Passage storage multiplier over item count is bounded and reportable (NFR-110-5)
- [ ] TP-02-01 executed and passing with recorded raw output
- [ ] TP-02-02 executed and passing with recorded raw output
- [ ] TP-02-03 executed and passing with recorded raw output
- [ ] TP-02-04 executed and passing with recorded raw output
- [ ] TP-02-05 executed and passing with recorded raw output
- [ ] TP-02-06 executed and passing with recorded raw output
- [ ] TP-02-07 executed and passing with recorded raw output
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## Scope 03: Query Plan Measurement And Bounded Ingest Lookup

**Status:** Not Started
**Depends On:** Scope 01
**Blocked by finding:** `F-110-PLAN-01`

Replaces the D1 ambiguity with an executed fact, and stops ingest-time linking from growing
with the corpus.

### Scenarios Covered

`SCN-110-C01`, `SCN-110-C02`, `SCN-110-C03`

### Implementation Plan (provisional)

- Execute the real retrieval query under plan inspection against a realistic corpus and record the plan node actually chosen.
- Fail the check when the observed node is a full scan of stored vectors.
- Measure p95 query latency from those executions at evaluation-corpus scale.
- Replace the cross-product temporal candidate lookup with a bounded lookup whose predicate can be served by the existing created-time index.

### Test Plan

| # | Test Type | Category | Description | Scenario |
|---|---|---|---|---|
| TP-03-01 | Integration | `integration` | The retrieval query's plan node is recorded from an executed plan, not asserted from source | SCN-110-C01 |
| TP-03-02 | Integration | `integration` | The check fails when the observed plan node is a full scan of stored vectors | SCN-110-C01 |
| TP-03-03 | Integration | `integration` | The temporal candidate lookup's plan uses the created-time index | SCN-110-C03 |
| TP-03-04 | Stress | `stress` | p95 query latency is measured at evaluation-corpus scale | SCN-110-C02 |
| TP-03-05 | Stress | `stress` | Ingest-time linking cost stays bounded as corpus size grows | SCN-110-C03 |

### Definition of Done

- [ ] The plan node actually used by the retrieval query is recorded from an executed plan (R-110-11)
- [ ] A full scan of stored vectors fails the plan check rather than warning (R-110-11)
- [ ] `F-110-PLAN-01` is resolved: the D1 ambiguity is replaced by a recorded measured result
- [ ] p95 query latency is measured at evaluation-corpus scale (R-110-12)
- [ ] Ingest-time relationship candidate lookup is bounded as corpus size grows (R-110-13)
- [ ] The candidate lookup predicate does not exclude the existing created-time index (R-110-13)
- [ ] TP-03-01 executed and passing with recorded raw output
- [ ] TP-03-02 executed and passing with recorded raw output
- [ ] TP-03-03 executed and passing with recorded raw output
- [ ] TP-03-04 executed and passing with recorded raw output
- [ ] TP-03-05 executed and passing with recorded raw output
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## Scope 04: Hybrid Fusion And Winning-Passage Evidence

**Status:** Not Started
**Depends On:** Scope 02

Turns several incomparable opinions into one comparable result per artifact, and keeps the
passage that won as the evidence the user sees.

### Scenarios Covered

`SCN-110-A01`, `SCN-110-A02`

### Implementation Plan (provisional)

- Fuse the distinct retrieval signals into one comparable score per artifact by a declared rule.
- Prevent raw scores of different kinds from being ranked directly against each other.
- Retain the winning passage on the fused result and present it as the displayed evidence.
- Verify the cited passage against stored content so displayed evidence can never be generated.

### Test Plan

| # | Test Type | Category | Description | Scenario |
|---|---|---|---|---|
| TP-04-01 | Unit | `unit` | Fusion produces one comparable score per artifact from multiple signals by the declared rule | SCN-110-A01 |
| TP-04-02 | Unit | `unit` | Adversarial: raw scores of different kinds are not ranked directly against each other | SCN-110-A01 |
| TP-04-03 | Integration | `integration` | The fused result carries the winning passage and returns it as displayed evidence | SCN-110-A01 |
| TP-04-04 | Integration | `integration` | Adversarial: a cited passage that is not a verbatim span of the named artifact's stored content fails | SCN-110-A02 |

### Definition of Done

- [ ] Distinct retrieval signals are fused into one comparable score per artifact by a declared rule (R-110-09)
- [ ] Raw scores of different kinds are never ranked directly against each other (R-110-10)
- [ ] The fused result carries and displays the winning passage as evidence (R-110-03)
- [ ] A cited passage is verified to be a verbatim span of the named artifact's stored content (R-110-03)
- [ ] `F-110-SUMMARY-01` is resolved: the summary-vector signal is either justified as distinct or removed
- [ ] TP-04-01 executed and passing with recorded raw output
- [ ] TP-04-02 executed and passing with recorded raw output
- [ ] TP-04-03 executed and passing with recorded raw output
- [ ] TP-04-04 executed and passing with recorded raw output
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## Scope 05: Evaluation Corpus, Executed Gate And Lane Wiring

**Status:** Not Started
**Depends On:** Scope 02, Scope 03, Scope 04
**Blocked by findings:** `F-110-LANE-01`, `F-110-CORPUS-01`, `F-110-FLOOR-01`

Makes retrieval quality a measured, automatically-enforced fact — and makes a gate that did
not run a failure rather than a pass.

### Scenarios Covered

`SCN-110-D01`, `SCN-110-D02`, `SCN-110-D03`, `SCN-110-D04`

### Implementation Plan (provisional)

- Author an evaluation set of at least 100 genuinely vague queries with known answer artifact ids over repository-owned synthetic content.
- Version the evaluation set together with its quality floors.
- Compute accuracy@1, accuracy@5, recall@20, p95 and the plan node from executed runs.
- Report an executed-assertion count and fail when it is zero.
- Wire the gate into a named lane reachable from the documented command surface with no undocumented manual step.
- Make the baseline-establishing run explicitly distinct from a gating run.

### Test Plan

| # | Test Type | Category | Description | Scenario |
|---|---|---|---|---|
| TP-05-01 | Integration | `integration` | The evaluation set contains at least 100 cases, each with a known answer artifact id | SCN-110-D01 |
| TP-05-02 | Integration | `integration` | accuracy@1, accuracy@5 and recall@20 are computed from executed retrievals and compared to floors | SCN-110-D01 |
| TP-05-03 | Integration | `integration` | The lane fails when any metric is below its declared floor | SCN-110-D01 |
| TP-05-04 | Integration | `integration` | Adversarial: the lane fails, naming the lane and reporting zero, when the gate's assertions do not execute | SCN-110-D02 |
| TP-05-05 | Integration | `integration` | Adversarial: a change that would lower a floor does not pass by adopting the newly measured value | SCN-110-D03 |
| TP-05-06 | Integration | `integration` | The gate's assertions execute from the documented command surface with no extra manual invocation | SCN-110-D04 |
| TP-05-07 | Integration | `integration` | Evaluation runs use ephemeral disposable storage and write to no persistent store | NFR-110-4 |

### Definition of Done

- [ ] An evaluation set of at least 100 vague queries with known answer ids exists (R-110-14)
- [ ] The evaluation set and its floors are versioned together (R-110-14)
- [ ] `F-110-CORPUS-01` is resolved: the cases are demonstrably vague, not keyword lookups
- [ ] accuracy@1, accuracy@5, recall@20, p95 and the plan node are all reported by the lane (R-110-15)
- [ ] The lane fails when any reported metric is below its floor (R-110-15)
- [ ] The lane reports an executed-assertion count (R-110-16)
- [ ] The lane fails when the executed-assertion count is zero (R-110-16)
- [ ] `F-110-LANE-01` is resolved: the gate executes in a named automated lane, verified by observing it run
- [ ] The gate executes from the documented command surface with no undocumented manual step (R-110-17)
- [ ] Every floor value originates from an executed measurement (R-110-18)
- [ ] `F-110-FLOOR-01` is resolved: the baseline-establishing run is explicitly distinguished from a gating run
- [ ] Lowering a floor is an explicit declared decision, not an automatic adoption (R-110-18)
- [ ] Evaluation data is repository-owned synthetic fixture data, never a real user corpus (NFR-110-3)
- [ ] Evaluation runs use ephemeral disposable storage only (NFR-110-4)
- [ ] The gate runs fast enough to remain in an automatically-executed lane (NFR-110-6)
- [ ] TP-05-01 executed and passing with recorded raw output
- [ ] TP-05-02 executed and passing with recorded raw output
- [ ] TP-05-03 executed and passing with recorded raw output
- [ ] TP-05-04 executed and passing with recorded raw output
- [ ] TP-05-05 executed and passing with recorded raw output
- [ ] TP-05-06 executed and passing with recorded raw output
- [ ] TP-05-07 executed and passing with recorded raw output
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned
