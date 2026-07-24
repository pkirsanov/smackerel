# Report: SCOPE-05 Correlation Rail (bounded spec-105 neighborhood + deep-link)

## Summary

Planning-owner record for the correlation-rail scope only. No source, authored
test, test pass, migration, browser run, or deployment is claimed; every DoD item
is unchecked. This scope adds `CorrelationRailRead` as a `RAIL_MAX`-bounded call of
the spec-105 neighborhood contract, deep-linking into the explorer, with honest
no-related vs unavailable states.

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-010/011; FR-107-013/014/015/016/027/030; NFR-107-003)
- Design source: `../../design.md` (`## Concrete Implementations` P3, `## Resolved Design Contracts` OQ5)
- Depends on: SCOPE-04; external gate spec-105 explorer deep-link usable
- Planning owner: `bubbles.plan`

## Test Evidence

No implementation test evidence belongs to this planning invocation. All tests
are PLANNED / not-yet-authored:

- `./smackerel.sh test unit` — `web/pwa/tests/correlation_rail_model_test.ts`, `tests/integration/proactive/correlation_rail_read_test.go`
- `./smackerel.sh test integration` — `tests/integration/proactive/correlation_rail_read_test.go`
- `./smackerel.sh test e2e` — `tests/e2e/proactive_experience_e2e_test.go`
- `./smackerel.sh test e2e-ui` — `web/pwa/tests/correlation-rail.spec.ts`, `web/pwa/tests/photos_lifecycle_review.spec.ts`

### Planned Evidence Anchors (Not Executed)

- `#t107-010-u` `#t107-010-i` `#t107-010-a` `#t107-010-w` — SCN-107-010 real edges + deep-link: Not executed — planned.
- `#t107-011-u` `#t107-011-i` `#t107-011-a` `#t107-011-w` — SCN-107-011 no-related honesty: Not executed — planned.
- `#t107-05-bound` — NFR-107-003 RAIL_MAX bound: Not executed — planned.
- `#t107-05-canary` — existing item views + explorer deep-link preserved: Not executed — planned.

## Completion Statement

Correlation-rail planning is complete; every scope test remains PLANNED and every
DoD item unchecked. No implementation, authored-test, test-pass, migration,
deployment, commit, or push claim is made. The scope is `Not Started`.
