# Report: SCOPE-07 What-Changed Feed

## Summary

Planning-owner record for the what-changed feed scope only. No source, authored
test, test pass, migration, browser run, or deployment is claimed; every DoD item
is unchecked. This scope adds `WhatChangedRead`, a bounded, cursor-paged,
authorized, restart-safe projection over `agent_traces` + surfacing verdicts +
topic lifecycle + recency, with no second store and no unread watermark.

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-014/015; FR-107-020/021/022/027/028)
- Design source: `../../design.md` (`## Concrete Implementations` P6, `## Resolved Design Contracts` OQ4)
- Depends on: SCOPE-06; external gate spec-054 agent_traces + topic lifecycle usable
- Planning owner: `bubbles.plan`

## Test Evidence

No implementation test evidence belongs to this planning invocation. All tests
are PLANNED / not-yet-authored:

- `./smackerel.sh test unit` — `web/pwa/tests/what_changed_feed_model_test.ts`
- `./smackerel.sh test integration` — `tests/integration/proactive/what_changed_read_test.go`
- `./smackerel.sh test e2e` — `tests/e2e/proactive_experience_e2e_test.go`
- `./smackerel.sh test e2e-ui` — `web/pwa/tests/what-changed-feed.spec.ts`

### Planned Evidence Anchors (Not Executed)

- `#t107-014-u` `#t107-014-i` `#t107-014-a` `#t107-014-w` — SCN-107-014 real system decisions: Not executed — planned.
- `#t107-015-u` `#t107-015-i` `#t107-015-a` `#t107-015-w` — SCN-107-015 returning without guilt: Not executed — planned.
- `#t107-07-restart` — restart-safe no watermark/second store: Not executed — planned.

## Completion Statement

What-changed feed planning is complete; every scope test remains PLANNED and every
DoD item unchecked. No implementation, authored-test, test-pass, migration,
deployment, commit, or push claim is made. The scope is `Not Started`.
