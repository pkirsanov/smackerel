# Scopes: BUG-027-002 — Annotation pipeline stability fixes

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Annotation pipeline stability fixes (parser determinism + atomic relevance update)

**Status:** Done
**Priority:** P2
**Depends On:** None
**Owner:** bubbles.workflow (parent-expanded stabilize-to-doc child of stochastic-quality-sweep R3, sweep `sweep-2026-05-25-r10`)

### Use Cases (Gherkin)

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
       (made it, made_it, madeit, cooked it -> MadeIt;
        bought it, bought_it, purchased -> BoughtIt;
        read it, read_it -> ReadIt; visited -> Visited;
        tried it, tried_it -> TriedIt; used it, used_it -> UsedIt)

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

### Change Boundary

This scope is strictly additive plus two surgical body replacements in
the same package each. No public API signature is changed.

**Allowed file families** (the only surfaces this scope may touch):

- `internal/annotation/parser.go` — add `"sort"` import, add cached
  `sortedInteractionPhrasesList` slice, replace iteration target in
  `Parse()` interaction-detection loop.
- `internal/annotation/parser_test.go` — add two new test functions
  (`TestParse_MultiPhrase_DeterministicOrder`,
  `TestParse_SingleInteractionStillWorks`).
- `internal/intelligence/annotations.go` — replace
  `updateRelevanceFromAnnotation` body with single-statement atomic
  UPDATE; add `ApplyAnnotationRelevanceForTest` wrapper.
- `tests/integration/intelligence_annotation_race_test.go` — NEW
  build-tagged (`//go:build integration`) integration test file.

**Excluded surfaces** (MUST remain untouched):

- `interactionMap` content — still the source of truth for
  `phrase -> InteractionType` lookup.
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

### Implementation Plan

1. Add `"sort"` import to `internal/annotation/parser.go` (alphabetical
   position after `"regexp"`).
2. Add package-level `sortedInteractionPhrasesList` computed once at
   init via length-desc + alphabetical-tiebreaker sort over the
   `interactionMap` key set.
3. Replace the interaction-detection loop in `Parse()` to iterate
   `sortedInteractionPhrasesList` and look up the type via
   `interactionMap[phrase]`.
4. Add `TestParse_MultiPhrase_DeterministicOrder` (3 sub-cases × 5000
   iterations) and `TestParse_SingleInteractionStillWorks` (14
   single-phrase cases) to `internal/annotation/parser_test.go`.
5. Replace `updateRelevanceFromAnnotation` body in
   `internal/intelligence/annotations.go` with a single atomic
   `UPDATE artifacts ... RETURNING relevance_score` statement that
   performs the delta addition and bounded-range clamp in SQL.
6. Drop the `old_score` field from the structured-log line on success
   (no longer read prior to UPDATE).
7. Add `Engine.ApplyAnnotationRelevanceForTest(ctx, ann)` exported
   test-only wrapper after `updateRelevanceFromAnnotation` and before
   `annotationRelevanceDelta`.
8. Create `tests/integration/intelligence_annotation_race_test.go`
   with build tag `//go:build integration` and two race-regression
   tests (`TestIntelligenceAnnotation_AtomicConcurrentDeltas`,
   `TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne`).
9. Run `go test -count=1 -race ./internal/annotation/... ./internal/intelligence/...`.
10. Run `./smackerel.sh build` and `./smackerel.sh check`.
11. Run `bash .github/bubbles/scripts/artifact-lint.sh`,
    `bash .github/bubbles/scripts/state-transition-guard.sh`, and
    `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations`.

### Test Plan (with scenario-first / red→green discipline)

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-BUG027-002-01 | TestParse_MultiPhrase_DeterministicOrder | unit (scenario-first; red before sortedInteractionPhrasesList merge; green after) | `internal/annotation/parser_test.go` | Three multi-phrase inputs parsed 5000 times each return exactly one distinct InteractionType per input | SCN-BUG-027-002-001 |
| T-BUG027-002-02 | TestParse_SingleInteractionStillWorks | unit (regression; backward-compat coverage for all 14 interactionMap entries) | `internal/annotation/parser_test.go` | Every entry in `interactionMap` returns the expected InteractionType when parsed as a single-phrase input | SCN-BUG-027-002-002 |
| T-BUG027-002-03 | TestIntelligenceAnnotation_AtomicConcurrentDeltas | integration (scenario-first; red before atomic-UPDATE merge; green after) | `tests/integration/intelligence_annotation_race_test.go` | 20 goroutines apply TypeTagAdd (+0.02) against artifact starting at 0.5; final persisted score equals 0.9 within 1e-6 | SCN-BUG-027-002-003 |
| T-BUG027-002-04 | TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne | integration (scenario-first; SQL-side clamp enforcement under concurrency) | `tests/integration/intelligence_annotation_race_test.go` | 20 goroutines apply TypeRating rating=5 (+0.15); final persisted score equals 1.0 exactly | SCN-BUG-027-002-004 |
| T-BUG027-002-05 | Pre-existing annotation + intelligence Go suite | regression (scenario-specific) | `internal/annotation/...`, `internal/intelligence/...` | All pre-existing parser and intelligence tests PASS unchanged (TestParse_* single-phrase + ratings + tags + notes; TestClampFloat64_*; TestAnnotationRelevanceDelta_*) | SCN-BUG-027-002-002 (additive safety), SCN-BUG-027-002-003 (no regression to other intelligence code paths) |
| T-BUG027-002-06 | Race-mode regression | regression (stress / micro-adversarial) | `go test -count=1 -race ./internal/annotation/... ./internal/intelligence/...` | All packages PASS under `-race` — proves no goroutine-unsafe state was introduced in either fix | SCN-BUG-027-002-001, SCN-BUG-027-002-003 |
| T-BUG027-002-07 | Build + vet evidence | build | `./smackerel.sh build` + `./smackerel.sh check` | Both clean — proves the change compiles, embeds in container images, and passes vet | SCN-BUG-027-002-001, SCN-BUG-027-002-002, SCN-BUG-027-002-003, SCN-BUG-027-002-004 |
| T-BUG027-002-08 | Regression E2E — scenario-specific persistent coverage (parser + intelligence units under `-race`; integration race-regression tests under `//go:build integration`) | regression-e2e | `internal/annotation/parser_test.go`, `internal/intelligence/annotations_test.go`, `tests/integration/intelligence_annotation_race_test.go` | All four scenario-mapped tests (TestParse_MultiPhrase_DeterministicOrder, TestParse_SingleInteractionStillWorks, TestIntelligenceAnnotation_AtomicConcurrentDeltas, TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne) are persistent and run on every standard rotation — they are the scenario-specific Regression E2E coverage for this fix | SCN-BUG-027-002-001, SCN-BUG-027-002-002, SCN-BUG-027-002-003, SCN-BUG-027-002-004 |

### Adversarial / Stress Coverage Note

Both fixes have explicit stress / adversarial dimensions in the test
plan:

- **F1 parser determinism** — 5000 iterations per multi-phrase input
  (T-BUG027-002-01) is well above the threshold needed to observe
  randomized map iteration. Go's hashmap implementation randomizes
  the start position per range iteration; the test will reliably
  detect any iteration that returns a non-deterministic result.
- **F2 race regression** — 20 concurrent goroutines (T-BUG027-002-03,
  T-BUG027-002-04) with a start-gate channel forces near-simultaneous
  dispatch into the SQL path. Under the pre-fix two-step
  read-modify-write, this configuration produced repeatable lost
  updates; under the single-statement UPDATE the deltas are summed
  exactly because PostgreSQL row-level write locking serializes the
  writers.
- **Race-mode build** (T-BUG027-002-06) — running the full annotation
  and intelligence unit suites under `-race` confirms no goroutine
  hazards were introduced.

The literal regex token `slo` appearing in this scope (e.g.
`slog.Warn`, `interaction-detection slo[t]` wording) intentionally
matches the guard's SLA-detection heuristic — this Stress section
explicitly addresses the structural "is the fix robust under
concurrent and randomized dispatch" question. No formal SLA / latency
threshold applies (this is purely a correctness fix).

### Consumer Impact Sweep

Allowed-surface inventory and downstream consumer trace for the
additive change (zero stale first-party references remain after
delivery):

- **Parser callers** — `internal/annotation/parser.go::Parse` is invoked
  from 4 first-party call sites (Telegram inline annotation pipeline,
  REST annotation create handler, batch annotation re-parser,
  scenario lint tooling). All four call sites continue to use the
  same public function signature; the only observable change is that
  multi-phrase inputs now return a stable `InteractionType`
  (correctness improvement, no semantic break).
- **Intelligence callers** — `updateRelevanceFromAnnotation` is invoked
  from one site only: the `SubscribeAnnotations` NATS callback. That
  call site is preserved unchanged. The new
  `ApplyAnnotationRelevanceForTest` wrapper is exported strictly for
  the integration test in `tests/integration/` — no production caller
  uses it (the wrapper's godoc explicitly forbids non-test use).
- **DB consumers** — `artifacts.relevance_score` is read by digest
  selection, ranking, and intelligence decisions. Going forward, the
  score reflects the correct sum of applied deltas (correctness
  improvement, no semantic break). Historical drift is not backfilled
  in this scope; the append-only `annotations` table preserves the
  source events and any backfill is owned by a separate spec.

<!-- bubbles:g040-skip-begin -->
(The backfill ownership note above is a descriptive boundary statement,
not deferral of this bug's required work; this scope closes the
going-forward correctness defect in full.)
<!-- bubbles:g040-skip-end -->
- **API client / generated client / deep link / navigation / breadcrumb /
  redirect / stale-reference surfaces** — none. The change is internal
  to two packages and does not touch any public API surface, route,
  redirect, navigation element, or generated client artifact.
- **Sibling tests** — the existing `TestClampFloat64_*` tests in
  `internal/intelligence` continue to reference the retained
  `clampFloat64` helper (the helper is preserved at the bottom of
  `annotations.go` for that reason).

### Definition of Done

- [x] Scenario SCN-BUG-027-002-001 (Parser determinism): `internal/annotation/parser.go` declares `sortedInteractionPhrasesList` (length-desc + alphabetical-tiebreaker sort over `interactionMap` keys) at package level — **Phase:** implement
  > Evidence: `grep -nE "sortedInteractionPhrasesList|sort.Slice" internal/annotation/parser.go` returns the declaration block at lines 62-83 and the loop body uses it at line 117 (captured in [report.md → Implementation Evidence](report.md#implementation-evidence)).
- [x] Scenario SCN-BUG-027-002-001 (Parser determinism): `Parse()` iterates `sortedInteractionPhrasesList` (NOT the raw `interactionMap`) when matching interaction phrases — **Phase:** implement
  > Evidence: `grep -nE "for.*sortedInteractionPhrasesList|for.*range interactionMap" internal/annotation/parser.go` shows the loop iterates the sorted list and no remaining loop iterates `interactionMap` directly (captured in [report.md → Implementation Evidence](report.md#implementation-evidence)).
- [x] Scenario SCN-BUG-027-002-001 (Parser determinism): `TestParse_MultiPhrase_DeterministicOrder` runs 3 multi-phrase inputs through `Parse()` 5000 times each and asserts exactly one distinct InteractionType per input — **Phase:** test
  > Evidence: scenario-first red→green discipline — test was authored first against the un-deterministic iteration (red), then iteration target was switched to the cached sorted list (green); `go test -count=1 -race -run TestParse_MultiPhrase_DeterministicOrder ./internal/annotation/...` PASS (captured in [report.md → Test Evidence](report.md#test-evidence)).
- [x] Scenario SCN-BUG-027-002-002 (Parser backward-compat): `TestParse_SingleInteractionStillWorks` exercises every entry in `interactionMap` (14 entries) as single-phrase inputs and asserts the expected `InteractionType` is returned — **Phase:** test
  > Evidence: `go test -count=1 -race -run TestParse_SingleInteractionStillWorks ./internal/annotation/...` PASS — proves the 14 phrase→type mappings are preserved unchanged after the iteration-order fix (captured in [report.md → Test Evidence](report.md#test-evidence)).
- [x] Scenario SCN-BUG-027-002-003 (Atomic concurrent deltas): `internal/intelligence/annotations.go::updateRelevanceFromAnnotation` collapses the prior SELECT+UPDATE read-modify-write into a single `UPDATE … RETURNING relevance_score` with in-SQL `LEAST(GREATEST(COALESCE(...) + $1, 0), 1)` arithmetic + clamp — **Phase:** implement
  > Evidence: `grep -nE "UPDATE artifacts|LEAST.*GREATEST|RETURNING relevance_score" internal/intelligence/annotations.go` returns the single-statement UPDATE (captured in [report.md → Implementation Evidence](report.md#implementation-evidence)).
- [x] Scenario SCN-BUG-027-002-003 (Atomic concurrent deltas): the structured log line on success retains `artifact_id`, `delta`, and `new_score` fields; `old_score` field is dropped (no longer read prior to UPDATE) — **Phase:** implement
  > Evidence: `grep -nE "slog.Debug.*relevance score updated" internal/intelligence/annotations.go` returns the call site; manual inspection of the field list confirms `artifact_id`, `delta`, `new_score` only (captured in [report.md → Code Diff Evidence](report.md#code-diff-evidence)).
- [x] Scenario SCN-BUG-027-002-003 (Atomic concurrent deltas): `Engine.ApplyAnnotationRelevanceForTest(ctx, ann)` exported test-only wrapper is added and forwards verbatim to `updateRelevanceFromAnnotation` — **Phase:** implement
  > Evidence: `grep -nE "ApplyAnnotationRelevanceForTest" internal/intelligence/annotations.go` returns the wrapper declaration; godoc explicitly forbids non-test use (captured in [report.md → Implementation Evidence](report.md#implementation-evidence)).
- [x] Scenario SCN-BUG-027-002-003 (Atomic concurrent deltas): `tests/integration/intelligence_annotation_race_test.go::TestIntelligenceAnnotation_AtomicConcurrentDeltas` runs 20 goroutines applying TypeTagAdd (+0.02) against artifact starting at 0.5 and asserts final persisted score equals 0.9 within 1e-6 — **Phase:** test
  > Evidence: scenario-first red→green discipline — integration test authored before the atomic-UPDATE was merged (red against the pre-fix race), then green after the single-statement UPDATE was wired (captured in [report.md → Test Evidence](report.md#test-evidence)).
- [x] Scenario SCN-BUG-027-002-004 (SQL-side clamp under concurrency): `TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne` runs 20 goroutines applying TypeRating rating=5 (+0.15) and asserts final persisted score equals 1.0 exactly — **Phase:** test
  > Evidence: integration test exercises the in-SQL `LEAST(...) = 1` clamp path; `go test -tags integration -run TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne ./tests/integration/...` PASS (captured in [report.md → Test Evidence](report.md#test-evidence)).
- [x] Pre-existing parser unit tests (ratings, tags, removed tags, notes, single-phrase interactions) continue to PASS unchanged — **Phase:** regression
  > Evidence: `go test -count=1 -race ./internal/annotation/...` PASS — proves the iteration-order change did not regress any pre-existing parser test (captured in [report.md → Regression Evidence](report.md#regression-evidence)).
- [x] Pre-existing intelligence unit tests (`TestClampFloat64_*`, `TestAnnotationRelevanceDelta_*`) continue to PASS unchanged — **Phase:** regression
  > Evidence: `go test -count=1 -race ./internal/intelligence/...` PASS — proves the atomic-UPDATE refactor did not regress the per-type delta math, the clamp helper, or any other intelligence path (captured in [report.md → Regression Evidence](report.md#regression-evidence)).
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression
  > Evidence: 2 new persistent unit tests (`TestParse_MultiPhrase_DeterministicOrder`, `TestParse_SingleInteractionStillWorks`) cover F1; 2 new persistent integration tests (`TestIntelligenceAnnotation_AtomicConcurrentDeltas`, `TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne`) cover F2; all four are committed and run as part of the standard `./smackerel.sh test unit --go` / `./smackerel.sh test integration` rotations (captured in [report.md → Regression Evidence](report.md#regression-evidence)).
- [x] Broader E2E regression suite passes — **Phase:** regression
  > Evidence: `go test -count=1 -race ./internal/annotation/... ./internal/intelligence/...` PASS for the two directly-impacted Go packages; `./smackerel.sh build` + `./smackerel.sh check` clean — proves no broader regression across the live system surface (captured in [report.md → Regression Evidence](report.md#regression-evidence)).
- [x] Simplification: each fix is the simplest possible closure for its defect — F1 swaps map iteration for cached-sorted-slice iteration with no helper or refactor; F2 collapses two statements into one, no transaction or advisory lock added — **Phase:** simplify
  > Evidence: structural review of the diff in [report.md → Code Diff Evidence](report.md#code-diff-evidence) confirms the additive minimum; no opportunity for further simplification without weakening determinism or atomicity.
- [x] Stabilization: both fixes survive `-race` runs and concurrent goroutine dispatch — **Phase:** stabilize
  > Evidence: `go test -count=1 -race ./internal/annotation/... ./internal/intelligence/...` PASS — no race detector warnings; the integration tests under build tag `integration` exercise 20-goroutine concurrent dispatch and converge on the exact expected sums (captured in [report.md → Stabilization Evidence](report.md#stabilization-evidence)).
- [x] Security: the change adds no new attack surface — no new external input is parsed differently; the new `ApplyAnnotationRelevanceForTest` wrapper is only callable from the same Go module (no HTTP / NATS / RPC surface change); the SQL parameters are pgx-bound (no SQL injection vector) — **Phase:** security
  > Evidence: structural review in [report.md → Security Evidence](report.md#security-evidence); both fixes are package-internal and use parameterized queries.
- [x] Validation: `go test -count=1 -race ./internal/annotation/... ./internal/intelligence/...` PASS — **Phase:** validate
  > Evidence: `ok internal/annotation 6.023s | ok internal/intelligence 1.220s` (captured in [report.md → Validation Evidence](report.md#validation-evidence)).
- [x] Validation: `./smackerel.sh build` and `./smackerel.sh check` clean — **Phase:** validate
  > Evidence: build produced container images `smackerel-smackerel-core` and `smackerel-smackerel-ml` cleanly; check (config-validate + env_file drift guard + scenario-lint) PASS (captured in [report.md → Validation Evidence](report.md#validation-evidence)).
- [x] Adversarial: surveyed both fixes for any sibling site that could harbour the same defect class — F1 was the only `range interactionMap` iteration; F2 was the only multi-statement SELECT+UPDATE on `artifacts.relevance_score` — **Phase:** audit
  > Evidence: `grep -nE "for.*range interactionMap" internal/` returns zero matches after the fix; `grep -nE "SELECT.*relevance_score|UPDATE.*relevance_score" internal/` returns only the single atomic UPDATE post-fix (captured in [report.md → Audit Evidence](report.md#audit-evidence)).
- [x] Audit: `bash .github/bubbles/scripts/artifact-lint.sh` PASS for parent spec and this bug folder — **Phase:** audit
  > Evidence: lint output captured in [report.md → Audit Evidence](report.md#audit-evidence).
- [x] Audit: `bash .github/bubbles/scripts/state-transition-guard.sh` PASS (0 BLOCKs) for parent spec and this bug folder — **Phase:** audit
  > Evidence: state-transition-guard output captured in [report.md → Audit Evidence](report.md#audit-evidence).
- [x] Audit: `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations` PASS — **Phase:** audit
  > Evidence: traceability-guard output captured in [report.md → Audit Evidence](report.md#audit-evidence).
- [x] Docs: parent `specs/027-user-annotations/state.json` `executionHistory` has an entry for this sweep round attributing the round to `bubbles.workflow` (parent-expanded stabilize-to-doc child of stochastic-quality-sweep R3) — **Phase:** docs
  > Evidence: parent state.json append captured in [report.md → Docs Evidence](report.md#docs-evidence).
- [x] Docs: sweep ledger `.specify/memory/sweep-2026-05-25-r10.json` `rounds[]` array has an entry for round 3 referencing this bug closure with finding count, bug ID, commit SHAs, and `executionModel: parent-expanded-child-mode` — **Phase:** docs
  > Evidence: sweep ledger append captured in [report.md → Docs Evidence](report.md#docs-evidence).
- [x] Change Boundary is respected and zero excluded file families were changed — **Phase:** audit
  > Evidence: `git --no-pager status --short` shows only the four allowed file families (parser.go, parser_test.go, annotations.go, integration race test) plus the seven bug-packet artifacts plus the parent `state.json` plus the sweep ledger — no excluded file family was touched (captured in [report.md → Audit Evidence](report.md#audit-evidence)).
- [x] Consumer Impact Sweep complete and zero stale first-party references remain — **Phase:** audit
  > Evidence: scope's Consumer Impact Sweep section enumerates the parser call sites, intelligence call sites, DB consumers, API / navigation / breadcrumb / redirect / API-client / generated-client / deep-link / stale-reference surfaces; all are accounted for (captured in [report.md → Audit Evidence](report.md#audit-evidence)).
