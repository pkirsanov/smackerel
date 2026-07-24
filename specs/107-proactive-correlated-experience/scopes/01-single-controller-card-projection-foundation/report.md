# Report: SCOPE-01 Single-Controller Card Projection & Nudge-Ack Foundation

## Summary

Planning-owner record for the foundation scope only. No source, authored test,
test pass, migration, browser run, or deployment is claimed. Every Definition of
Done item is unchecked. This scope defines the `ProactiveCardModel`, the
`NudgeRef` registry, the single `NudgeAck` path (`Acknowledge(content_key)`), the
`HonestStatePresenter`, the `BudgetMeterRead`, and the additive `a:n:` callback
encode/decode consumed by every later scope.

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-004, 008, 009; FR-107-003/005/006/007/008/010/023/024/028; NFR-107-001/006)
- Design source: `../../design.md` (`## Capability Foundation`, `## Resolved Design Contracts` OQ2/OQ6, `## Single-Controller Routing`)
- External entry gate: `specs/078-cross-surface-surfacing-prioritizer` + `internal/intelligence/surfacing/` controller usable
- Planning owner: `bubbles.plan`
- Implementation owner: unresolved until orchestration dispatches `bubbles.implement`

## Test Evidence

No implementation test evidence belongs to this planning invocation. All tests
are PLANNED / not-yet-authored. The planned commands and files are recorded so
the executor can bind them at pickup:

- `./smackerel.sh test unit` — `internal/web/proactive/nudge_ack_test.go`, `internal/intelligence/surfacing/budget_meter_test.go`, `internal/intelligence/surfacing/escalation_projection_test.go`, `internal/web/proactive/nudge_ref_leak_test.go`
- `./smackerel.sh test integration` — `tests/integration/proactive/nudge_ack_controller_test.go`, `tests/integration/proactive/budget_defer_parity_test.go`, `tests/integration/proactive/escalation_parity_test.go`
- `./smackerel.sh test e2e` — `tests/e2e/proactive_experience_e2e_test.go`
- `./smackerel.sh test e2e-ui` — `web/pwa/tests/proactive-card.spec.ts`
- `./smackerel.sh test stress` — `tests/stress/proactive_hotpath_test.go`

### Planned Evidence Anchors (Not Executed)

- `#t107-004-u` `#t107-004-i` `#t107-004-a` `#t107-004-w` — SCN-107-004 acknowledge through controller (unit/integration/e2e-api/e2e-ui): Not executed — planned.
- `#t107-008-u` `#t107-008-i` `#t107-008-a` `#t107-008-w` — SCN-107-008 budget exhausted defer: Not executed — planned.
- `#t107-009-u` `#t107-009-i` `#t107-009-a` `#t107-009-w` — SCN-107-009 urgent escalation: Not executed — planned.
- `#t107-01-hotpath` — NFR-107-001 controller hot-path preserved: Not executed — planned.
- `#t107-01-leak` — FR-107-028 no content_key/node label/query on any wire: Not executed — planned.

## Completion Statement

Foundation planning is complete; every scope test remains PLANNED and every DoD
item unchecked. No implementation, authored-test, test-pass, migration,
deployment, commit, or push claim is made. The scope is `Not Started`.
