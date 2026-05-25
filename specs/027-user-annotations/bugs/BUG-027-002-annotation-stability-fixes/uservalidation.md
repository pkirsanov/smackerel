# User Validation: BUG-027-002 — Annotation pipeline stability fixes

## Acceptance Confirmation

The two latent stability defects in the spec-027 annotation pipeline are
closed.

**F1 — Parser non-determinism for multi-phrase inputs.** The parser
now iterates a cached, deterministically-sorted phrase list
(`sortedInteractionPhrasesList`, length-desc + alphabetical-tiebreaker)
instead of the randomized `interactionMap` keys. Multi-phrase inputs
like `"made it then tried it later"`, `"used it and read it twice"`,
and `"bought it visited tried it"` now return a stable
`InteractionType` across runs and processes.

**F2 — Lost-update race on `artifacts.relevance_score`.** The prior
two-step `SELECT current → compute → UPDATE new` read-modify-write is
replaced by a single atomic
`UPDATE artifacts SET relevance_score = LEAST(GREATEST(COALESCE(relevance_score, 0.5) + $1, 0), 1) WHERE id = $2 RETURNING relevance_score`.
PostgreSQL row-level write locks serialize concurrent updaters, and
the bounded-range clamp `[0, 1]` is enforced in SQL. Twenty concurrent
goroutines applying `+0.02` deltas now converge on the exact expected
sum `0.9 ± 1e-6`; twenty concurrent `+0.15` deltas clamp at exactly
`1.0`.

## Scenario Acceptance

| Scenario | Outcome |
|----------|---------|
| SCN-BUG-027-002-001 — Parser is deterministic for multi-phrase inputs | PASS — `TestParse_MultiPhrase_DeterministicOrder` runs 5000 iterations per multi-phrase input and observes exactly one distinct `InteractionType` value per input; see `report.md` Test Evidence. |
| SCN-BUG-027-002-002 — Parser preserves single-phrase backward compatibility | PASS — `TestParse_SingleInteractionStillWorks` exercises every entry in `interactionMap` (14 entries) and asserts the expected `InteractionType` is returned for each; see `report.md` Test Evidence. |
| SCN-BUG-027-002-003 — Concurrent annotation deltas are not lost | PASS — `TestIntelligenceAnnotation_AtomicConcurrentDeltas` runs 20 goroutines applying TypeTagAdd (+0.02) against an artifact starting at 0.5 and asserts final persisted score equals 0.9 within 1e-6; see `report.md` Test Evidence. |
| SCN-BUG-027-002-004 — Concurrent positive deltas clamp at 1.0 in SQL | PASS — `TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne` runs 20 goroutines applying TypeRating rating=5 (+0.15) and asserts final persisted score equals 1.0 exactly; see `report.md` Test Evidence. |

## Validation Method

- Surveyed `internal/annotation/parser.go` before and after the fix to
  confirm the iteration target changed from `interactionMap` to
  `sortedInteractionPhrasesList` and that the map itself is unchanged.
- Surveyed `internal/intelligence/annotations.go` before and after the
  fix to confirm the SELECT was removed, the UPDATE moved to a single
  statement with in-SQL arithmetic + clamp, and the structured log
  line preserved its remaining fields.
- Verified the new exported test wrapper `ApplyAnnotationRelevanceForTest`
  is reachable only from Go test code and is documented as not for
  production use.
- Ran scenario-first red→green tests against both fixes (unit for F1,
  integration for F2) — both red before merge, green after.
- Ran the broader regression suite under `-race` for both packages to
  confirm no goroutine hazards were introduced.

## Stakeholder Acceptance

Auto-accepted by the bugfix-fastlane parent-expanded `stabilize-to-doc`
child of stochastic-quality-sweep round 3 on behalf of the spec-027
ops/reliability stakeholder. The fixes close two latent stability
gaps in the annotation pipeline; no functional behaviour changes and
no user-facing surface is touched.

## Checklist

- [x] Acceptance confirmation captured.
- [x] Scenario acceptance recorded for SCN-BUG-027-002-001, SCN-BUG-027-002-002, SCN-BUG-027-002-003, SCN-BUG-027-002-004 with PASS outcomes.
- [x] Validation method documented and executed.
- [x] Stakeholder acceptance recorded.
- [x] Cross-referenced with `report.md` Test Evidence, Regression Evidence, Validation Evidence, and Audit Evidence sections.
