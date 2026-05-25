# Design: BUG-027-002 — Annotation pipeline stability fixes

## Approach

Two surgical, independently-verifiable stability fixes in the spec-027
annotation pipeline:

1. **F1 — Deterministic phrase iteration in the parser.** Replace
   `for phrase, itype := range interactionMap` (randomized) with iteration
   over a cached `sortedInteractionPhrasesList` keyed by
   `(len desc, alphabetical asc)`. The map itself stays as the canonical
   `phrase → InteractionType` lookup; only the iteration order is
   stabilized.
2. **F2 — Atomic single-statement relevance update.** Collapse the
   `SELECT → compute → UPDATE` read-modify-write into a single
   `UPDATE artifacts SET relevance_score = LEAST(GREATEST(COALESCE(relevance_score, 0.5) + $1, 0), 1) RETURNING relevance_score`.
   The in-SQL arithmetic + clamp removes the race window entirely;
   PostgreSQL row-level write locks serialize concurrent updaters.

## Surfaces

| Pipeline Half | Pre-fix surface | Post-fix surface |
|---------------|-----------------|------------------|
| Parser (`internal/annotation/parser.go::Parse`) | `for phrase, itype := range interactionMap` | `for _, phrase := range sortedInteractionPhrasesList; itype := interactionMap[phrase]` |
| Intelligence (`internal/intelligence/annotations.go::updateRelevanceFromAnnotation`) | `SELECT current_score; compute newScore in Go; UPDATE relevance_score = $newScore` | `UPDATE … RETURNING relevance_score = LEAST(GREATEST(COALESCE(...) + $delta, 0), 1)` (single round-trip) |

The two fixes are independent of each other and of the public
annotation schema — both are local changes within their respective
packages.

## Changes

### `internal/annotation/parser.go` (deterministic phrase iteration)

Add `"sort"` to the import block (alphabetically placed after `"regexp"`).

Add a package-level `sortedInteractionPhrasesList` slice computed once
at init from the `interactionMap` key set:

```go
// sortedInteractionPhrases returns the full interactionMap key set in a
// deterministic order: longest phrase first, alphabetical as tiebreaker.
// Parse() iterates this list (NOT the raw map) so multi-phrase inputs
// resolve to a stable InteractionType regardless of Go's randomized map
// iteration. Longer-first ordering ensures the most-specific match wins
// when one phrase is a strict substring of another (the current key set
// does not contain such pairs, but the policy is defensive). The result
// is cached at package init time.
var sortedInteractionPhrasesList = func() []string {
    keys := make([]string, 0, len(interactionMap))
    for k := range interactionMap {
        keys = append(keys, k)
    }
    sort.Slice(keys, func(i, j int) bool {
        if len(keys[i]) != len(keys[j]) {
            return len(keys[i]) > len(keys[j])
        }
        return keys[i] < keys[j]
    })
    return keys
}()
```

Replace the interaction-detection loop in `Parse()`:

```go
// Pre-fix (lines 86-97 prior to BUG-027-002):
for phrase, itype := range interactionMap {
    if strings.Contains(lower, phrase) {
        result.InteractionType = itype
        remaining = strings.ReplaceAll(remaining, phrase, "")
        break
    }
}

// Post-fix:
for _, phrase := range sortedInteractionPhrasesList {
    if strings.Contains(lower, phrase) {
        result.InteractionType = interactionMap[phrase]
        remaining = strings.ReplaceAll(remaining, phrase, "")
        break
    }
}
```

### `internal/intelligence/annotations.go` (atomic relevance update)

Replace `updateRelevanceFromAnnotation` body:

```go
// Pre-fix (paraphrased — lines 47-79 prior to BUG-027-002):
var current sql.NullFloat64
err := e.Pool.QueryRow(ctx,
    `SELECT relevance_score FROM artifacts WHERE id = $1`,
    ann.ArtifactID,
).Scan(&current)
oldScore := 0.5
if current.Valid { oldScore = current.Float64 }
newScore := clampFloat64(oldScore + delta, 0, 1)
_, err = e.Pool.Exec(ctx,
    `UPDATE artifacts SET relevance_score = $1 WHERE id = $2`,
    newScore, ann.ArtifactID,
)
```

```go
// Post-fix (single atomic statement):
var newScore float64
err := e.Pool.QueryRow(ctx, `
    UPDATE artifacts
    SET relevance_score = LEAST(GREATEST(COALESCE(relevance_score, 0.5) + $1, 0), 1)
    WHERE id = $2
    RETURNING relevance_score
`, delta, ann.ArtifactID).Scan(&newScore)
```

The structured log line on success drops the prior `old_score` field
(no longer read before the UPDATE) and retains `artifact_id`, `delta`,
and `new_score` (the persisted post-update value returned by
`RETURNING`).

Add an exported test-only wrapper:

```go
// ApplyAnnotationRelevanceForTest is a test-only exported wrapper
// around the unexported updateRelevanceFromAnnotation. It exists so
// the BUG-027-002 race regression integration tests can drive the
// same SQL path without reaching through NATS (the production caller
// is the SubscribeAnnotations subscriber callback).
//
// This wrapper is NOT for production code.
func (e *Engine) ApplyAnnotationRelevanceForTest(ctx context.Context, ann *annotation.Annotation) error {
    return e.updateRelevanceFromAnnotation(ctx, ann)
}
```

### `internal/annotation/parser_test.go` (deterministic-iteration tests)

Add two new test functions:

- `TestParse_MultiPhrase_DeterministicOrder` — 3 multi-phrase inputs
  exercised through `Parse()` 5000 times each; assert exactly one
  distinct `InteractionType` per input.
- `TestParse_SingleInteractionStillWorks` — exercise every entry in
  `interactionMap` as a single-phrase input; assert the expected
  `InteractionType` is returned (backward-compatibility coverage).

### `tests/integration/intelligence_annotation_race_test.go` (new)

Build-tagged `//go:build integration` integration test exercising the
single-statement atomic UPDATE under concurrent goroutine dispatch
against a real PostgreSQL backend. Two scenarios:

- `TestIntelligenceAnnotation_AtomicConcurrentDeltas` — 20 goroutines
  × `TypeTagAdd (+0.02)`, start gate via `close(ch)`, assert final
  persisted score `0.9 ± 1e-6` (= `0.5 + 20 * 0.02`).
- `TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne` — 20
  goroutines × `TypeRating rating=5 (+0.15)`, assert final persisted
  score clamps at exactly `1.0` (SQL-side `LEAST` enforcement).

Helpers: `intelligenceRacePool(t)` (opens pgxpool, applies migrations),
`insertRaceArtifact(t, pool, initialScore)` (inserts row, registers
cleanup), `readRelevanceScore(t, pool, id)` (post-test verification).

## Allowed File Families

This bug is strictly additive within the four files below.

- `internal/annotation/parser.go` — add `"sort"` import, add cached
  `sortedInteractionPhrasesList`, replace iteration target in `Parse()`.
- `internal/annotation/parser_test.go` — add two new test functions.
- `internal/intelligence/annotations.go` — replace
  `updateRelevanceFromAnnotation` body with atomic single-statement
  UPDATE; add `ApplyAnnotationRelevanceForTest` wrapper.
- `tests/integration/intelligence_annotation_race_test.go` — NEW
  build-tagged integration test file.

## Excluded Surfaces (must remain untouched)

- `interactionMap` content — still the source of truth for
  `phrase → InteractionType` lookup.
- `InteractionPhrases()` canonical-phrase API — public signature and
  ordering preserved verbatim.
- Existing parser test cases for ratings, tags, removed tags, notes,
  and single-phrase interactions — preserved unchanged.
- `clampFloat64` helper — retained at end of `annotations.go` (still
  referenced by 3 existing `TestClampFloat64_*` tests).
- `annotationRelevanceDelta` and the per-type delta values — preserved
  unchanged.
- `SubscribeAnnotations` callback wiring and the NATS subject name.
- The append-only `annotations` table schema, the materialized view
  `artifact_annotation_summary` definition, and the
  `artifacts.relevance_score` column declaration.

## Non-Changes

- No new SST config knob, no new metric family (this is a stability
  fix, not an observability improvement — observability gaps are out
  of scope and a separate concern).
- No transaction wrap around the relevance update — PostgreSQL
  row-level write locks already serialize concurrent updaters.
- No new advisory lock — single-statement UPDATE is sufficient.
- No change to `Engine` struct fields, method signatures, or public
  symbols other than the new `ApplyAnnotationRelevanceForTest`.

## Backwards Compatibility

- F1 — observable parser output is **strictly more deterministic**.
  Multi-phrase inputs that previously flapped now return a stable
  `InteractionType`. Single-phrase inputs return the same
  `InteractionType` they did before (verified by
  `TestParse_SingleInteractionStillWorks`).
- F2 — the persisted `relevance_score` is now exactly the
  `clamp(prior + Σ(deltas), 0, 1)` instead of a race-corrupted value.
  Going forward, monitoring of the score is more stable. Historical
  drift is not backfilled (out of scope — separate operational task if
  needed).
- No public API signature change.
- No migration required (schema unchanged).

## Risk

| Risk | Mitigation |
|------|------------|
| Hot-path cost of building `sortedInteractionPhrasesList` per parse | Cached at package init; built once for the process lifetime. |
| Two atomic UPDATEs vs prior SELECT+UPDATE — net DB round-trips | Reduced from 2 to 1; the single statement is cheaper, not more expensive. |
| Concurrent updates serializing through PostgreSQL row-level write lock could increase tail latency under hot-artifact bursts | Acceptable. Hot-artifact bursts are bounded by user behaviour (a user can annotate an artifact at most a few times per second); the row-lock holding window is microseconds. |
| Integration test requires real PostgreSQL | Already covered by `tests/integration` infrastructure under `//go:build integration` — runs against the disposable test stack via `./smackerel.sh test integration`. |

## Observability

The fix does not add metrics. The integration test asserts the
correctness invariant directly (sum of deltas equals the persisted
score within tolerance). If, post-deploy, monitoring of the relevance
update path becomes operationally useful, that is a separate
observability concern (out of scope here).

## Test Plan

| Scenario | Test Function | File | Type |
|----------|---------------|------|------|
| SCN-BUG-027-002-001 Parser determinism | `TestParse_MultiPhrase_DeterministicOrder` | `internal/annotation/parser_test.go` | unit |
| SCN-BUG-027-002-002 Parser single-phrase backward-compat | `TestParse_SingleInteractionStillWorks` | `internal/annotation/parser_test.go` | unit |
| SCN-BUG-027-002-003 Atomic concurrent deltas | `TestIntelligenceAnnotation_AtomicConcurrentDeltas` | `tests/integration/intelligence_annotation_race_test.go` | integration |
| SCN-BUG-027-002-004 SQL-side clamp under concurrency | `TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne` | `tests/integration/intelligence_annotation_race_test.go` | integration |
| Regression: pre-existing parser tests | `TestParse_*` (existing) | `internal/annotation/parser_test.go` | regression |
| Regression: pre-existing intelligence tests | `TestClampFloat64_*`, `TestAnnotationRelevanceDelta_*` | `internal/intelligence/*_test.go` | regression |
| Build/vet | `./smackerel.sh build`, `./smackerel.sh check` | n/a | build |
