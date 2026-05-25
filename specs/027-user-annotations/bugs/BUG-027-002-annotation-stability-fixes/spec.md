# Bug: BUG-027-002 — Annotation pipeline stability fixes (parser determinism + atomic relevance update)

## Classification

- **Type:** Stabilize / reliability gaps — two latent stability defects in the annotation pipeline that survive normal unit tests but fail under concurrency or randomized map iteration
- **Severity:** MEDIUM — F2 (lost-update race) silently corrupts persisted `relevance_score` under concurrent annotation traffic; F1 (parser non-determinism) makes one observable parser output (`InteractionType`) flap across runs for multi-phrase inputs
- **Parent Spec:** 027 — User Annotations
- **Workflow Mode:** bugfix-fastlane (parent-expanded child of stochastic-quality-sweep round 3, trigger `stabilize`, mapped mode `stabilize-to-doc`)
- **Status:** Open — discovered by stabilize R3 (sweep `sweep-2026-05-25-r10`)

## Problem Statement

Two latent stability defects in the spec-027 annotation pipeline survive
normal unit tests but fail under concurrency or randomized iteration:

### F1 — Parser non-determinism for multi-phrase inputs

`internal/annotation/parser.go::Parse` iterated `interactionMap` directly
to detect interaction phrases (e.g., "made it", "tried it", "read it").
Go's map iteration order is randomized per run, so when an annotation
string contained multiple phrases (e.g., "made it then tried it later",
"used it and read it twice"), the resulting `InteractionType` was
non-deterministic — different processes parsing the same input could
return different categorizations, and reruns inside the same process
under different iterations could too. The first-match-wins loop body
was the same; only the iteration order differed.

Pre-fix code (lines 86-97 prior to BUG-027-002):

```go
lower := strings.ToLower(remaining)
for phrase, itype := range interactionMap {
    if strings.Contains(lower, phrase) {
        result.InteractionType = itype
        remaining = strings.ReplaceAll(remaining, phrase, "")
        break
    }
}
```

This is a stability defect because the parser is invoked from 4 call
sites (Telegram inline annotation pipeline, REST annotation create
handler, batch annotation re-parser, scenario lint tooling). Every
call site assumed deterministic output. Downstream consumers including
the materialized annotation summary view, intelligence relevance
updates, and digest output rely on stable `InteractionType` values for
the same input.

### F2 — Lost-update race on `artifacts.relevance_score`

`internal/intelligence/annotations.go::updateRelevanceFromAnnotation`
performed a two-step `SELECT current_score` → `compute new_score` →
`UPDATE relevance_score = new_score` read-modify-write sequence:

```go
var current sql.NullFloat64
err := e.Pool.QueryRow(ctx,
    `SELECT relevance_score FROM artifacts WHERE id = $1`,
    ann.ArtifactID,
).Scan(&current)
// ... compute delta and newScore in Go ...
_, err = e.Pool.Exec(ctx,
    `UPDATE artifacts SET relevance_score = $1 WHERE id = $2`,
    newScore, ann.ArtifactID,
)
```

The two statements are not in a transaction. The NATS subscriber
callback in `SubscribeAnnotations` is invoked per message, and the
NATS client dispatches each callback in its own goroutine. When two
annotation events arrive for the same artifact in burst (e.g., a
user sends two Telegram annotations back-to-back, or a batch
re-parser fires concurrently with a live capture), the read-modify-write
window races: both goroutines SELECT the same `current`, both compute
their own `newScore`, and both UPDATE — the second UPDATE silently
overwrites the first delta. The lost update is invisible: no error,
no warning, no metric. The persisted `relevance_score` reflects only
the last-arriving delta, not the sum.

The bounded-range clamp (`[0, 1]`) was also enforced in Go via
`math.Max/math.Min` after the SELECT and before the UPDATE — meaning
the clamp invariant only held assuming a serialized read-modify-write,
which the race violates. A concurrent clamp-skipping write was
theoretically possible.

## Impact

| Axis | Impact |
|------|--------|
| **F1 — Functional correctness** | Annotation re-parses against the same input could return different `InteractionType` values across runs. Materialized view rebuilds, batch re-processing, scenario lint output, and digest synthesis could all reach different conclusions for identical inputs. |
| **F1 — Test reliability** | Any future regression test that asserts on `InteractionType` for a multi-phrase input is flaky by construction without the fix. |
| **F2 — Data integrity** | Concurrent annotation events for the same artifact silently lose deltas. `relevance_score` drifts low against the true sum of applied deltas. Affects ranking, digest selection, and downstream intelligence decisions. |
| **F2 — Invariant enforcement** | Bounded-range clamp `[0, 1]` was enforced in application code; concurrent writers could theoretically bypass it (no DB-side guard). |
| **F2 — Observability** | The lost-update is silent — no error, no warning, no counter, no log line. The only signal is the artifact's score being lower than the sum of its applied deltas, which is invisible to monitoring without a ground-truth comparison. |
| **Severity** | MEDIUM. No data loss in the strict sense (other annotation event fields are preserved in the append-only `annotations` table). The materialized view and intelligence relevance score are the affected derived state. |

## Why this is "stabilize" not "harden" or "improve"

Per `stabilize-to-doc` charter: reliability gap on an already-functional
surface that manifests under concurrency, randomization, or burst load
— exactly the two defects above. Neither defect violates a security
invariant (rules out `harden`); neither is a missing piece of
ergonomics, observability symmetry, or API consistency (rules out
`improve`); both are latent stability defects that survive single-shot
unit tests because the failure modes only emerge under randomized map
iteration (F1) or concurrent goroutine dispatch (F2). This is the
canonical stabilize signature.

## Why prior rounds missed it

- **R1 improve (2026-05-25)** — focused on the alert-producer observability
  symmetry gap (closed as BUG-021-003). Did not exercise multi-phrase
  parser inputs or concurrent annotation event dispatch.
- **R2 reconcile (2026-05-25)** — focused on connector artifact drift
  (closed as BUG-027-001). Did not exercise the parser or the NATS
  subscriber callback.
- The existing parser unit tests (`internal/annotation/parser_test.go`)
  cover single-phrase inputs only — multi-phrase inputs were never
  attempted.
- The existing intelligence unit tests cover `annotationRelevanceDelta`
  in isolation; the SQL round-trip and concurrent dispatch were never
  exercised at the unit-test layer (the integration suite did not yet
  have an annotation-race test).

R3 stabilize (this round) extends the lens to "iteration determinism
under randomized map order" and "atomic write under concurrent
subscriber dispatch" and discovered both gaps.

## Reproduction (pre-fix)

### F1 — Parser non-determinism

```text
$ cat > /tmp/parser_demo.go << 'EOF'
// ... ran a 200-iteration loop calling annotation.Parse() against
// the multi-phrase inputs "made it then tried it later", "used it and
// read it twice", "bought it visited tried it"; observed that the
// `InteractionType` outcomes were distributed across the matching
// types instead of converging on a single deterministic value.
EOF
```

(Demo script was authored, evidenced, and removed during the discovery
round — see `report.md` for the captured outcomes.)

### F2 — Lost-update race

The integration test `TestIntelligenceAnnotation_AtomicConcurrentDeltas`
in `tests/integration/intelligence_annotation_race_test.go` reproduces
the race deterministically — without the fix, running 20 goroutines
that each apply a `TypeTagAdd` event (`+0.02`) against the same
artifact starting from `0.5` would converge to a value below the
expected `0.9` (= `0.5 + 20 * 0.02`) because of overlapping
read-modify-write windows. With the fix, the final value matches
`0.9` within `1e-6` floating-point tolerance.

## Acceptance Criteria

- [ ] `internal/annotation/parser.go` declares a package-level
      `sortedInteractionPhrasesList` slice computed once at init from the
      `interactionMap` key set, sorted by `(len desc, alphabetical asc)`.
- [ ] `Parse()` iterates `sortedInteractionPhrasesList` (NOT the raw
      `interactionMap`) when matching interaction phrases.
- [ ] The existing canonical-phrase API `InteractionPhrases()` is
      preserved verbatim — no caller has its signature changed.
- [ ] `interactionMap` is unchanged (still the source of truth for
      `phrase → InteractionType` lookup).
- [ ] A new unit test `TestParse_MultiPhrase_DeterministicOrder` in
      `internal/annotation/parser_test.go` runs at least three
      multi-phrase inputs through `Parse()` 5000 times each and asserts
      that exactly one distinct `InteractionType` value is observed
      per input.
- [ ] A new unit test `TestParse_SingleInteractionStillWorks` exercises
      every entry in `interactionMap` as a single-phrase input and
      asserts the expected `InteractionType` is returned (backward
      compatibility).
- [ ] `internal/intelligence/annotations.go::updateRelevanceFromAnnotation`
      collapses the prior `SELECT → compute → UPDATE` read-modify-write
      into a single atomic `UPDATE … RETURNING relevance_score` statement
      with the bounded-range clamp enforced in SQL via
      `LEAST(GREATEST(COALESCE(relevance_score, 0.5) + $1, 0), 1)`.
- [ ] The structured log line on success still emits `artifact_id`,
      `delta`, and `new_score` (the persisted post-update value returned
      by `RETURNING`); only the prior `old_score` field is dropped (no
      longer read prior to UPDATE).
- [ ] A new exported test-only wrapper `Engine.ApplyAnnotationRelevanceForTest`
      forwards to the unexported `updateRelevanceFromAnnotation` so the
      integration race tests can drive the same SQL path without
      reaching through NATS.
- [ ] A new build-tagged integration test
      `tests/integration/intelligence_annotation_race_test.go::TestIntelligenceAnnotation_AtomicConcurrentDeltas`
      runs 20 goroutines that each apply a `TypeTagAdd` annotation
      (`+0.02`) against the same artifact starting from `relevance_score = 0.5`
      and asserts the final persisted score is `0.9 ± 1e-6` (no lost updates).
- [ ] A second build-tagged integration test
      `TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne` runs 20
      goroutines that each apply a `TypeRating rating=5` annotation
      (`+0.15`) and asserts the persisted score is clamped at exactly
      `1.0` (SQL-side `LEAST` enforcement).
- [ ] Pre-existing parser unit tests (single-phrase inputs, ratings,
      tags, removed tags, notes) continue to PASS unchanged.
- [ ] Pre-existing intelligence unit tests (including `TestClampFloat64_*`
      and `TestAnnotationRelevanceDelta_*`) continue to PASS unchanged.
- [ ] `go test -count=1 -race ./internal/annotation/... ./internal/intelligence/...` PASS.
- [ ] `./smackerel.sh build` and `./smackerel.sh check` clean.
- [ ] `artifact-lint.sh`, `state-transition-guard.sh`, and
      `traceability-guard.sh` PASS for parent spec 027 and this bug folder.

## Non-Goals

- Wider parser refactor (e.g., extracting a phrase-matching helper or
  switching to a regexp-based matcher). The single-statement loop is
  the simplest closure for the determinism gap.
- Introducing a transaction or advisory lock around the relevance
  update. PostgreSQL row-level write locking on `UPDATE` already
  serializes the concurrent updaters; an explicit transaction would
  add overhead without changing correctness.
- A `reason`-labelled metric for relevance update outcomes (deferred
  until ops feedback indicates the differentiation is operationally
  useful — out of scope for stability fix).
- Backfilling or reconciling historical `relevance_score` values that
  may have drifted prior to this fix (the append-only `annotations`
  table preserves the source events; backfill is a separate operational
  task if it becomes necessary).
- Changing the public `Annotation` schema, the NATS subject name, the
  per-type delta values, or the materialized view definition.

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-BUG-027-002-001 Parser is deterministic for multi-phrase inputs
  Given the annotation parser caches a sorted phrase list at init
  When the same multi-phrase input ("made it then tried it later",
       "used it and read it twice", "bought it visited tried it")
       is parsed 5000 times in a single process
  Then exactly one distinct InteractionType value is observed per input

Scenario: SCN-BUG-027-002-002 Parser preserves single-phrase backward compatibility
  Given the annotation parser sorted phrase list is active
  When each entry in the interactionMap is parsed as a single-phrase input
  Then the parser returns the expected InteractionType for every entry
       (made it, made_it, madeit, cooked it → MadeIt;
        bought it, bought_it, purchased → BoughtIt;
        read it, read_it → ReadIt; visited → Visited;
        tried it, tried_it → TriedIt; used it, used_it → UsedIt)

Scenario: SCN-BUG-027-002-003 Concurrent annotation deltas are not lost
  Given an artifact with relevance_score = 0.5
  When 20 goroutines concurrently apply a TypeTagAdd annotation (+0.02)
       against the same artifact via ApplyAnnotationRelevanceForTest
  Then the final persisted relevance_score equals 0.9 within 1e-6
       (no read-modify-write race lost any delta)

Scenario: SCN-BUG-027-002-004 Concurrent positive deltas clamp at 1.0 in SQL
  Given an artifact with relevance_score = 0.5
  When 20 goroutines concurrently apply a TypeRating rating=5 annotation (+0.15)
       against the same artifact via ApplyAnnotationRelevanceForTest
  Then the final persisted relevance_score equals 1.0 exactly
       (the in-SQL LEAST/GREATEST clamp held under concurrency)
```

## Acceptance Test References

| Scenario | Test Function | File |
|----------|---------------|------|
| SCN-BUG-027-002-001 | `TestParse_MultiPhrase_DeterministicOrder` | `internal/annotation/parser_test.go` |
| SCN-BUG-027-002-002 | `TestParse_SingleInteractionStillWorks` | `internal/annotation/parser_test.go` |
| SCN-BUG-027-002-003 | `TestIntelligenceAnnotation_AtomicConcurrentDeltas` | `tests/integration/intelligence_annotation_race_test.go` |
| SCN-BUG-027-002-004 | `TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne` | `tests/integration/intelligence_annotation_race_test.go` |
