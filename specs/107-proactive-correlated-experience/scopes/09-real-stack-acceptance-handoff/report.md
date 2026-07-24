# Report: SCOPE-09 Real-Stack Acceptance & Implementation Handoff

## Summary

Planning-owner record for the acceptance + handoff scope only. No source, authored
test, test pass, migration, browser run, or deployment is claimed; every DoD item
is unchecked. This scope defines the controller-origin invariant assertion
(SCN-107-016), the complete no-interception acceptance matrix, the Telegram/WhatsApp
adapter-level parity coverage, the honest-state matrix, and the value-safe
implementation handoff.

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-016; acceptance rerun SCN-107-001..020; FR-107-003/006/007/008/009/022; NFR-107-004)
- Design source: `../../design.md` (`## Single-Controller Routing`, `## Routed Questions`)
- Depends on: SCOPE-08
- Planning owner: `bubbles.plan`
- Implementation / test / validation owners: `bubbles.implement` / `bubbles.test` / `bubbles.validate` at pickup

## Test Evidence

No implementation test evidence belongs to this planning invocation. All tests
are PLANNED / not-yet-authored:

- `./smackerel.sh test unit` — `internal/web/proactive/card_origin_invariant_test.go`
- `./smackerel.sh test integration` — `tests/integration/proactive/controller_origin_test.go`
- `./smackerel.sh test e2e` — `tests/e2e/proactive_experience_e2e_test.go`, `tests/e2e/proactive_channel_parity_e2e_test.go`
- `./smackerel.sh test e2e-ui` — `web/pwa/tests/proactive-acceptance.spec.ts`

### Planned Evidence Anchors (Not Executed)

- `#t107-016-u` `#t107-016-i` `#t107-016-a` `#t107-016-w` — SCN-107-016 controller-origin invariant: Not executed — planned.
- `#t107-09-matrix` — SCN-107-001..020 acceptance rerun (no interception): Not executed — planned.
- `#t107-09-channels` — Telegram + WhatsApp adapter-level parity: Not executed — planned.
- `#t107-09-honest` — honest-state matrix: Not executed — planned.

## Implementation Handoff (Value-Safe)

- **Scenario contracts:** `../../scenario-manifest.json` — 20 SCN-107 scenarios, all tests PLANNED / not-yet-authored (0 linked), matching the spec-105 planning-only convention.
- **Test inventory:** `../../test-plan.json` — the consolidated planned test rows per scope.
- **SST no-default keys (reserved; NOT edited this phase):** `nudge_ref_ttl_hours=6`, `RAIL_MAX=6`, `what_changed_page_cap=25`; snooze reuses `suppression_window_hours` (no distinct `snooze_window_hours` for MVP). All fail-loud under `config/smackerel.yaml` at implementation.
- **Migration reservation:** none expected — the `NudgeRef` registry is in-memory and no new PostgreSQL business table is added; a migration number is allocated at pickup only if implementation proves one is needed.
- **Coordination notes (routed to owners, not edited):** the `whatsapp` `Channel` enum value → spec-078 owner; the `<kind>:<id>` seed + `Explore connections` deep-link stability → spec-105 owner; the `Today` landing-route registration → spec-106 owner.

## Completion Statement

Acceptance + handoff planning is complete; every scope test remains PLANNED and
every DoD item unchecked. No implementation, authored-test, test-pass, migration,
browser-executed, deployment, commit, or push claim is made. The scope is
`Not Started`.
