# Report: SCOPE-02 Web Proactive Card & Authenticated Action Transport

## Summary

Planning-owner record for the web card scope only. No source, authored test, test
pass, migration, browser run, or deployment is claimed; every DoD item is
unchecked. This scope renders one `ProactiveCardModel` as a spec-106
Pending-action-row and routes the web action as an authenticated same-origin
`{nudgeRef, action}` mutation through the SCOPE-01 `NudgeAck` path.

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-003; FR-107-003/004/005/023/029)
- Design source: `../../design.md` (`## Concrete Implementations` P2 web, `### Web Action Transport`)
- Depends on: SCOPE-01 (`ProactiveCardModel`, `NudgeRef`, `NudgeAck`); external gate spec-106 shell usable
- Planning owner: `bubbles.plan`

## Test Evidence

No implementation test evidence belongs to this planning invocation. All tests
are PLANNED / not-yet-authored:

- `./smackerel.sh test unit` — `web/pwa/tests/proactive_card_model_test.ts`
- `./smackerel.sh test integration` — `tests/integration/proactive/web_action_transport_test.go`
- `./smackerel.sh test e2e` — `tests/e2e/proactive_experience_e2e_test.go`
- `./smackerel.sh test e2e-ui` — `web/pwa/tests/proactive-card.spec.ts`, `web/pwa/tests/assistant_intents_dashboard.spec.ts`

### Planned Evidence Anchors (Not Executed)

- `#t107-003-u` `#t107-003-i` `#t107-003-a` `#t107-003-w` — SCN-107-003 provenance + one-tap actions: Not executed — planned.
- `#t107-02-transport` — no bearer in JS, no client-side content_key: Not executed — planned.
- `#t107-02-canary` — shell/assistant navigation preserved: Not executed — planned.

## Completion Statement

Web-card planning is complete; every scope test remains PLANNED and every DoD
item unchecked. No implementation, authored-test, test-pass, migration,
deployment, commit, or push claim is made. The scope is `Not Started`.
