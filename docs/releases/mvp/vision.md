# Vision — Smackerel MVP

## What "MVP" means here

The 2026-06-03 MVP gate declares Smackerel **feature-complete for its founding promise**: a self-hosted, local-first personal knowledge engine that ingests passively across the user's existing digital footprint, retrieves precisely from vague input, and surfaces what matters proactively — within an explicit, measurable user-interruption budget.

This vision is restated in full here. It does NOT reference the vision of any prior phase packet; readers do not need to load Phase 1–5 packets to evaluate MVP scope.

## Audience this gate serves

A single technically-comfortable user running Smackerel on their own hardware (or a small household). They already produce knowledge across email, calendars, messaging, browsing, location, media, RSS, and personal connectors — and they currently lose it to fragmentation.

## What shipping MVP proves

1. **Observation works.** Eighteen-plus connectors run on schedule, classify-at-capture is unnecessary, and the knowledge graph forms without user effort. (Source: [`docs/smackerel.md`](../../smackerel.md) §3.3, §7.)
2. **Vague-in / precise-out works.** Semantic search via pgvector + LLM rerank returns useful answers from imprecise queries across all source types. (Source: [`docs/smackerel.md`](../../smackerel.md) §3.5, §1.4.)
3. **Synthesis works.** Daily digests, weekly digests, pre-meeting briefs, and contextual alerts run end-to-end with source attribution. (Source: [`docs/smackerel.md`](../../smackerel.md) §3.7; specs 021, 025.)
4. **Proactive surfacing respects the user.** A single global interruption budget governs all outbound nudges (digest + push + Telegram + web + ntfy + email-out), with measurable SLOs and a per-day ceiling. **This is GAP C — the MVP-blocking capability this packet adds via adjustments to specs 021, 025, 054.**
5. **The graph is browsable.** A wiki-style view by topic / person / place / time renders cross-links and supports inline annotation. **This is GAP F — the MVP-blocking surface this packet adds via adjustments to specs 073, 027.**
6. **Product principles are enforced.** [`docs/Product-Principles.md`](../../Product-Principles.md) 1–10 transition from advisory to BLOCKING; the grep gates in [`.github/instructions/product-principles.instructions.md`](../../../.github/instructions/product-principles.instructions.md) ratify on this date.

## Success signal (observable proof MVP is delivered)

- 18+ committed connectors continuously ingest without user prompts (already true; see [`internal/connector/`](../../../internal/connector/)).
- A user query of the form "what was that thing about X" returns a relevant artifact in ≤ 5 s end-to-end.
- The system emits ≤ N nudges/day across all channels combined, with an acted-on-rate metric exposed and a false-positive ceiling enforced. (N is set in the surfacing controller spec adjustment; bubbles.releases does not fix it here.)
- The web UI exposes a graph-browse view; users can traverse topic ↔ person ↔ place ↔ time pivots without re-querying.
- Pre-push enforcement BLOCKS PRs violating principles 1–10 grep gates.
- Spec 026 MAJOR_DRIFT is cleared; portfolio drift report shows zero MAJOR_DRIFT items.

## Non-goals (explicit)

- **Active outbound action.** Smackerel observes and surfaces; it does NOT yet write back to source systems (no Gmail send, no calendar event mutation, no Slack post, no shopping checkout). That capability is RELEASE-V1 Gap B.
- **Personal Productivity Sources beyond current connector set.** No Gmail SDK / Graph / Apple Reminders / Notion / Obsidian / messages / voice in MVP. RELEASE-V1.
- **Native mobile app.** PWA remains the mobile surface in MVP. Native decision deferred to RELEASE-V1.
- **SLO-as-alert promotion.** Surfacing controller SLOs are measured and exposed in MVP, but full alert wiring against monitoring stack (spec 049) is RELEASE-V1.
- **Capability map auto-generation.** Manual capability ledger remains source of truth in MVP. RELEASE-V1.

## Cross-product context

The QF Companion boundary ([`docs/smackerel.md`](../../smackerel.md) §1.6) is preserved: Smackerel may ingest QF decision packets, preserve trust metadata, and export consent-scoped personal evidence bundles. Smackerel MUST NOT execute trades, approve mandates, or give financial advice. The qfdecisions connector (spec 041) is in the MVP-locked connector roster; no additional QF integration is in MVP scope.

## What MVP does not promise

This MVP is a **founding-promise gate**, not a commercial-product gate. It does not promise:
- A monetization model (see [`monetization.md`](monetization.md) for pre-revenue posture)
- A hosted multi-tenant offering (self-hosted only)
- A support contract (community-only)
- Any guarantee about external API stability (internal API; user-controlled deployment)
