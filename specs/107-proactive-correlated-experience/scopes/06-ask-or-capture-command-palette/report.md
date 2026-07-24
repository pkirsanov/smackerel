# Report: SCOPE-06 Ask-or-Capture Command Palette

## Summary

Planning-owner record for the command-palette scope only. No source, authored
test, test pass, migration, browser run, or deployment is claimed; every DoD item
is unchecked. This scope adds the Cmd/Ctrl-K global overlay routed through the
existing assistant `Facade.Handle` to answer / spec-074 capture / honest refusal /
error, never rendering a failed ask as "saved as an idea".

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-012/013; FR-107-017/018/019)
- Design source: `../../design.md` (`## Concrete Implementations` P4, `### Command-Palette Routing Contract`)
- Depends on: SCOPE-05; external gate spec-074 capture + spec-061/073 `Facade` usable
- Planning owner: `bubbles.plan`

## Test Evidence

No implementation test evidence belongs to this planning invocation. All tests
are PLANNED / not-yet-authored:

- `./smackerel.sh test unit` — `web/pwa/tests/command_palette_router_test.ts`
- `./smackerel.sh test integration` — `tests/integration/proactive/palette_turn_router_test.go`
- `./smackerel.sh test e2e` — `tests/e2e/proactive_experience_e2e_test.go`
- `./smackerel.sh test e2e-ui` — `web/pwa/tests/command-palette.spec.ts`, `web/pwa/tests/assistant_accessibility.spec.ts`

### Planned Evidence Anchors (Not Executed)

- `#t107-012-u` `#t107-012-i` `#t107-012-a` `#t107-012-w` — SCN-107-012 grounded answer: Not executed — planned.
- `#t107-013-u` `#t107-013-i` `#t107-013-a` `#t107-013-w` — SCN-107-013 capture as fallback: Not executed — planned.
- `#t107-06-honesty` — failed ask never saved-as-idea: Not executed — planned.
- `#t107-06-canary` — existing assistant capture/correction path preserved: Not executed — planned.

## Completion Statement

Command-palette planning is complete; every scope test remains PLANNED and every
DoD item unchecked. No implementation, authored-test, test-pass, migration,
deployment, commit, or push claim is made. The scope is `Not Started`.
