# Report: SCOPE-04 Today Cockpit Composition (spec-106 `Today` body)

## Summary

Planning-owner record for the cockpit scope only. No source, authored test, test
pass, migration, browser run, or deployment is claimed; every DoD item is
unchecked. This scope composes the digest lede + `FOR YOU NOW` cards +
what-changed strip + secondary ask-or-capture bar + budget meter inside the
spec-106 `Today` body, observe-first, with honest quiet/partial/degraded states.

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-001/002/017; FR-107-001/002/022/024/029; NFR-107-002)
- Design source: `../../design.md` (`## Concrete Implementations` P1, `### Landing Route Coordination`)
- Depends on: SCOPE-03; external gate spec-106 `Today` destination usable
- Planning owner: `bubbles.plan`

## Test Evidence

No implementation test evidence belongs to this planning invocation. All tests
are PLANNED / not-yet-authored:

- `./smackerel.sh test unit` — `web/pwa/tests/today_cockpit_compose_test.ts`
- `./smackerel.sh test integration` — `tests/integration/proactive/today_cockpit_test.go`
- `./smackerel.sh test e2e` — `tests/e2e/proactive_experience_e2e_test.go`
- `./smackerel.sh test e2e-ui` — `web/pwa/tests/today-cockpit.spec.ts`, `web/pwa/tests/unified_journey.spec.ts`
- `./smackerel.sh test stress` — `tests/stress/proactive_cockpit_landing_test.go`

### Planned Evidence Anchors (Not Executed)

- `#t107-001-u` `#t107-001-i` `#t107-001-a` `#t107-001-w` — SCN-107-001 cockpit leads with produced intelligence: Not executed — planned.
- `#t107-002-u` `#t107-002-i` `#t107-002-a` `#t107-002-w` — SCN-107-002 quiet day is honest: Not executed — planned.
- `#t107-017-u` `#t107-017-i` `#t107-017-a` `#t107-017-w` — SCN-107-017 producer failure region: Not executed — planned.
- `#t107-04-landing` — NFR-107-002 landing budget: Not executed — planned.
- `#t107-04-canary` — existing Today/Digest + shell preserved: Not executed — planned.

## Completion Statement

Cockpit planning is complete; every scope test remains PLANNED and every DoD item
unchecked. No implementation, authored-test, test-pass, migration, deployment,
commit, or push claim is made. The scope is `Not Started`.
