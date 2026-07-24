# Report: SCOPE-03 Telegram & WhatsApp Nudge Renderings + Cross-Channel Parity

## Summary

Planning-owner record for the messaging + parity scope only. No source, authored
test, test pass, migration, browser run, or deployment is claimed; every DoD item
is unchecked. This scope adds the additive Telegram `a:n:` family and the WhatsApp
interactive rendering, both resolving through the SCOPE-01 registry to the one
ack path, and proves act-once-suppressed-everywhere.

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-005/006/007; FR-107-007/008/009/010/011/012; NFR-107-004)
- Design source: `../../design.md` (`## Concrete Implementations` P2/P5, OQ2, OQ7, `## Single-Controller Routing`)
- Depends on: SCOPE-02; external gate spec-072 WhatsApp transport usable
- Planning owner: `bubbles.plan`

## Test Evidence

No implementation test evidence belongs to this planning invocation. All tests
are PLANNED / not-yet-authored:

- `./smackerel.sh test unit` — `internal/telegram/assistant_adapter/nudge_callbacks_test.go`, `internal/whatsapp/nudge_reply_test.go`, `internal/intelligence/surfacing/cross_channel_ack_test.go`
- `./smackerel.sh test integration` — `tests/integration/proactive/telegram_nudge_ack_test.go`, `tests/integration/proactive/whatsapp_nudge_ack_test.go`, `tests/integration/proactive/cross_channel_suppression_test.go`
- `./smackerel.sh test e2e` — `tests/e2e/proactive_channel_parity_e2e_test.go`
- `./smackerel.sh test e2e-ui` — `web/pwa/tests/proactive-cross-channel.spec.ts`
- `./smackerel.sh test stress` — `tests/stress/proactive_suppression_propagation_test.go`

### Planned Evidence Anchors (Not Executed)

- `#t107-005-u` `#t107-005-i` `#t107-005-a` `#t107-005-w` — SCN-107-005 Telegram inline nudge: Not executed — planned.
- `#t107-006-u` `#t107-006-i` `#t107-006-a` `#t107-006-w` — SCN-107-006 WhatsApp interactive nudge: Not executed — planned.
- `#t107-007-u` `#t107-007-i` `#t107-007-a` `#t107-007-w` — SCN-107-007 cross-channel suppression: Not executed — planned.
- `#t107-03-suppress` — NFR-107-004 suppression propagation window: Not executed — planned.
- `#t107-03-collision` — `a:n:` non-collision: Not executed — planned.

## Completion Statement

Messaging + parity planning is complete; every scope test remains PLANNED and
every DoD item unchecked. No implementation, authored-test, test-pass, migration,
deployment, commit, or push claim is made. The scope is `Not Started`.
