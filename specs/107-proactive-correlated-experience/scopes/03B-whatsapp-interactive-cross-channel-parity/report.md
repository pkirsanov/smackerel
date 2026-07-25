# Report: SCOPE-03B WhatsApp Interactive + Cross-Channel Parity

## Summary

Planning-owner record for the gated WhatsApp + cross-channel-parity scope only. No
source, authored test, test pass, migration, browser run, or deployment is
claimed; every DoD item is unchecked. This scope adds the WhatsApp interactive
rendering over the spec-072 transport and proves the cross-channel
act-once-suppressed-everywhere parity assertion across web + Telegram + WhatsApp.
It stays honestly gated on spec-072 and spec-106/`SCOPE-02`; it is NOT buildable
now and its work is not folded into `SCOPE-03A`.

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-006/007; FR-107-007/008/009/011/012/028; NFR-107-004)
- Design source: `../../design.md` (`## Concrete Implementations` P2 WhatsApp / P5 parity, OQ2, OQ7, `## Single-Controller Routing`)
- Depends on: SCOPE-02 + SCOPE-03A; external gate spec-072 WhatsApp transport + spec-106 shell usable
- Planning owner: `bubbles.plan` (re-scope of the former SCOPE-03)

## Test Evidence

No implementation test evidence belongs to this planning invocation. All tests
are PLANNED / not-yet-authored:

- `./smackerel.sh test unit` — `internal/whatsapp/nudge_reply_test.go`, `internal/intelligence/surfacing/cross_channel_ack_test.go`
- `./smackerel.sh test integration` — `tests/integration/proactive/whatsapp_nudge_ack_test.go`, `tests/integration/proactive/cross_channel_suppression_test.go`
- `./smackerel.sh test e2e` — `tests/e2e/proactive_channel_parity_e2e_test.go`
- `./smackerel.sh test e2e-ui` — `web/pwa/tests/proactive-cross-channel.spec.ts`
- `./smackerel.sh test stress` — `tests/stress/proactive_suppression_propagation_test.go`

### Planned Evidence Anchors (Not Executed)

- `#t107-006-u` `#t107-006-i` `#t107-006-a` `#t107-006-w` — SCN-107-006 WhatsApp interactive nudge: Not executed — planned.
- `#t107-007-u` `#t107-007-i` `#t107-007-a` `#t107-007-w` — SCN-107-007 cross-channel suppression: Not executed — planned.
- `#t107-03-suppress` — NFR-107-004 suppression propagation window: Not executed — planned.
- `#t107-03b-tgweb-parity` — Telegram act reflects on the web card state: Not executed — planned.

## Completion Statement

WhatsApp + cross-channel-parity planning is complete; every scope test remains
PLANNED and every DoD item unchecked. No implementation, authored-test,
test-pass, migration, deployment, commit, or push claim is made. The scope is
`Blocked` on spec-072 + spec-106/`SCOPE-02`.
