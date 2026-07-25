# Report: SCOPE-03B2 Cross-Channel Web Parity

## Summary

Planning-owner record for the gated cross-channel-web-parity slice only. No
source, authored test, test pass, migration, browser run, or deployment is
claimed; every DoD item is unchecked. This scope proves the cross-channel
act-once-suppressed-everywhere parity assertion across web + Telegram + WhatsApp
and the Telegram/WhatsApp→web reflection, all resolving through the one
`Acknowledge(content_key)` path. It stays honestly gated on spec-106 / `SCOPE-02`
(the web card) — the messaging renders are already delivered (`SCOPE-03A`) or
buildable now (`SCOPE-03B1`), but the "everywhere" + web-reflection assertions
require the spec-106 web card.

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-007; FR-107-007/008/028; NFR-107-004)
- Design source: `../../design.md` (`## Concrete Implementations` P5 parity, OQ7, `## Single-Controller Routing`)
- Depends on: SCOPE-03B1 + SCOPE-02; external gate spec-106 shell + `SCOPE-02` web card usable
- Planning owner: `bubbles.plan` (split of the former SCOPE-03B)

## Test Evidence

No implementation test evidence belongs to this planning invocation. All tests
are PLANNED / not-yet-authored:

- `./smackerel.sh test unit` — `internal/intelligence/surfacing/cross_channel_ack_test.go`
- `./smackerel.sh test integration` — `tests/integration/proactive/cross_channel_suppression_test.go`
- `./smackerel.sh test e2e` — `tests/e2e/proactive_channel_parity_e2e_test.go`
- `./smackerel.sh test e2e-ui` — `web/pwa/tests/proactive-cross-channel.spec.ts`
- `./smackerel.sh test stress` — `tests/stress/proactive_suppression_propagation_test.go`

### Planned Evidence Anchors (Not Executed)

- `#t107-007-u` `#t107-007-i` `#t107-007-a` `#t107-007-w` — SCN-107-007 cross-channel suppression: Not executed — planned.
- `#t107-03-suppress` — NFR-107-004 suppression propagation window: Not executed — planned.
- `#t107-03b2-tgweb-parity` — Telegram act reflects on the web card state: Not executed — planned.
- `#t107-03b2-waweb-parity` — WhatsApp act reflects on the web card state: Not executed — planned.

## Completion Statement

Cross-channel-web-parity planning is complete; every scope test remains PLANNED
and every DoD item unchecked. No implementation, authored-test, test-pass,
migration, deployment, commit, or push claim is made. The scope is `Blocked` on
spec-106 / `SCOPE-02` (the web card).
