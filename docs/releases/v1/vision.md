# Vision — Smackerel v1

## What "v1" means here

The 2026-06-03 v1 gate (forward-looking) declares Smackerel **commercially-serious for daily use** by users who do not (or cannot) accept the MVP constraints — by expanding the ingestion surface to all the personal-productivity sources serious knowledge workers actually use, by adding a first-class outbound-action capability so Smackerel can ACT (not just OBSERVE), and by deciding the native-mobile question.

This vision is restated in full here. It does NOT reference the [`../mvp/vision.md`](../mvp/vision.md); v1 readers do not need to load the MVP packet to understand v1 scope.

## Audience this gate serves

Two segments, expanded from MVP's single self-hoster:

1. **Serious knowledge workers** who live across Gmail / Outlook / Apple Calendar / Notion / Obsidian / Apple Notes / Reminders / Tasks / Slack / SMS — for whom MVP's IMAP-plus-CalDAV-plus-Keep baseline is insufficient.
2. **Daily-driver mobile users** who currently can't make Smackerel work as a phone-first product because the PWA constraints (background ingestion, share-sheet, notification SLOs) limit them.

The MVP audience (technically-comfortable self-hosters) remains served.

## What shipping v1 proves

1. **Smackerel ingests where serious knowledge actually lives** — Personal Productivity Sources arrive: Gmail SDK, Microsoft Graph mail + calendar, native Google Calendar, Apple Calendar via EventKit bridge, Reminders/Tasks family, Notes family, messages family, voice. (Deep review Gap A; per-spec dispatch in [`features.md`](features.md).)

2. **Smackerel can act, not just observe** — A first-class Outbound Action capability foundation lands. Capability registry, consent/permission model, audit log, dry-run, undo window. Per-target actions (send-gmail, create-calendar-event, create-reminder, post-slack-message) are layered on top of the foundation. (Deep review Gap B.)

3. **The mobile question is decided** — Native iOS/Android vs PWA-only is resolved via an operator decision doc; if native is chosen, the spec slots are reserved. (Deep review rec 8.)

4. **Capabilities document themselves** — Auto-generated capability map replaces hand-maintained capability claims. (Deep review rec 9.)

5. **Surfacing controller SLOs are alerted, not just measured** — MVP exposed M1a SLOs; v1 wires them as paged alerts in the monitoring stack. (Deep review rec 3, spec 049 adjustment.)

6. **Portfolio drift stays clean** — Continuous spec-review cycle maintains zero MAJOR_DRIFT going into and out of v1.

## Success signal (observable proof v1 is delivered)

- Every connector family in [`features.md`](features.md) v1 list is `done` per its spec's `state.json`.
- Outbound Action foundation spec (`086-outbound-action-foundation`) is `done`; at least three downstream per-target outbound-action implementations are `done`.
- `docs/Mobile_Decision.md` exists and is signed by operator; if native chosen, native specs are `done` or `in-progress` with explicit timeline.
- Auto-generated capability map (`090-capability-map-generator`) replaces manual capability claim files.
- Prometheus alerting rules exist for M1a SLO breach and Outbound-Action SLO breach; spec 049 reflects.
- `bubbles.spec-review` classifies portfolio as zero MAJOR_DRIFT at v1 close.

## Non-goals (explicit)

v1 does NOT promise:
- **Commercial hosting / multi-tenant SaaS.** Smackerel remains self-hosted under the operator's runtime. Hosting is a v2+ question (and not a commitment).
- **End-to-end encryption.** Smackerel is local-first / self-hosted, not e2ee.
- **Public marketing of "AI agent that does your work."** Outbound Action is consent-scoped, audit-logged, dry-runnable, undo-windowed. Anything resembling unattended autonomous action against external systems is out of scope at v1.
- **A pricing model.** See [`monetization.md`](monetization.md).
- **QF Companion integration expansion.** The QF boundary remains exactly as in MVP per [`docs/smackerel.md`](../../smackerel.md) §1.6.

## Cross-product context

QF Companion boundary preserved unchanged. The QF integration may grow in scope at v2+ as the Outbound Action capability matures, but at v1 the qfdecisions connector (spec 041) remains read-only per Principle 10.

## What v1 does not promise (continued)

v1 is a **capability-completeness gate**, not a commercial-product gate. Smackerel after v1 can be left to operate as the operator's tool — there is no obligation to monetize, no obligation to scale to multi-tenant, no obligation to develop a v2.
