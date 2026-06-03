# Marketing — Smackerel v1

> Every claim in this file traces to a row in [`features.md`](features.md). No claim is fabricated. No competitor is invented. No outbound-action capability is marketed as autonomous-agent behavior — V2-A is consent-gated, dry-run-first, undo-windowed.

## Posture

v1 marketing remains **technically-honest** but expands beyond MVP's self-hoster-only target. v1 unlocks the "serious knowledge worker" segment because Personal Productivity Sources (V1) close the obvious gaps, AND v1 unlocks the "I want it to act for me" use case because Outbound Action (V2) lands.

v1 marketing is still **not** a launch push. v1 declaration is a capability-completeness milestone, not a commercial-go-live.

## Audience segments + messaging

| Segment | Who | Core message (claims trace to [`features.md`](features.md)) | Anti-message |
|---------|-----|-------------------------------------------------------------|--------------|
| **Carry-forward MVP audience: self-hosters** | Same as MVP | "Same self-hosted, local-first, daily-budget-respecting Smackerel — now ingesting from Gmail/Outlook/Notion/Obsidian/Reminders/Messages/voice, with consented outbound action." | Don't suggest commercial hosting / SaaS pivot |
| **Serious knowledge workers in Apple+Google+Microsoft ecosystems** | Use Gmail+Calendar OR Outlook+Calendar OR Apple Calendar+Reminders+Notes — for whom MVP's IMAP+CalDAV+Keep baseline was insufficient | "Native integrations: Gmail SDK, Microsoft Graph (mail + calendar), Google Calendar API, Apple Calendar via EventKit, Apple Reminders, Google Tasks, MS To Do, Todoist, TickTick, Notion, Obsidian, Apple Notes, OneNote." | Don't claim feature parity with Notion AI / Microsoft Copilot — Smackerel's wedge is cross-source ingestion, not in-app authoring |
| **Outbound-action users** | Want Smackerel to send the email, create the calendar event, post the Slack message — under explicit consent | "Smackerel can act on your behalf — under explicit per-action consent, with audit log, dry-run by default, and an undo window. You stay in control." (V2-A) | Don't market as "autonomous AI agent". Don't omit consent / audit / undo language. Don't claim Smackerel makes decisions for you. |
| **Mobile-first users** | Phone-first; PWA constraints were a blocker at MVP | If V3-A chooses native: "Native iOS and Android apps with proper background ingestion, share-sheet capture, and notification SLOs." If V3-A keeps PWA: "Upgraded PWA with the same SLOs." | Don't claim native if V3-A defers; don't claim PWA parity with native if V3-A chooses native |
| **QF Companion users** | Carry-forward from MVP | Same as MVP. **No v1 expansion of QF integration.** | Don't market Smackerel v1 outbound action against QF systems |

## Channel strategy

| Channel | v1 use | Cadence |
|---------|--------|---------|
| Project README | Reflects v1 capability set after V-items close | At v1 close |
| `docs/INVESTOR_OVERVIEW.md` | v1 row added per DOC-V2 | At v1 close |
| `docs/Product-Principles.md` | Unchanged in scope (ratified at MVP); v1 capabilities must satisfy ratified principles | n/a — no v1 mutation |
| `docs/Capability_Map.md` | Auto-generated per V4-A | Continuous after V4-A lands |
| `docs/Mobile_Decision.md` | Operator's decision record | At V3-A close |
| Optional external posts / talks | After all V-items terminal | Operator decision |

## Asset list

| Asset | Status | Owner | Source-of-truth claim trace |
|-------|--------|-------|-----------------------------|
| Updated `README.md` reflecting v1 capability set | Pending (after V-items close) | `bubbles.docs` | [`features.md`](features.md) |
| `docs/INVESTOR_OVERVIEW.md` v1 row | Pending (DOC-V2) | `bubbles.docs` | [`features.md`](features.md), [`vision.md`](vision.md) |
| `docs/Capability_Map.md` (auto-generated) | Pending (V4-A lands first) | V4-A generator | `internal/connector/registry.go`, skills manifest, capability ledger |
| `docs/Mobile_Decision.md` (DOC-V1) | Pending operator decision | Operator + `bubbles.docs` | [`actions.md`](actions.md) OQ-V7 |
| "Consent-gated outbound action" explainer (one-pager) | Net-new; author after V2-A lands | `bubbles.docs` | V2-A spec |
| Per-source connector landing pages (Gmail SDK, Notion, etc.) | Net-new; author per-V1-spec close | `bubbles.docs` | per-V1 spec |
| Native mobile launch assets (if V3-B/C land) | Conditional | TBD | V3-B/V3-C specs |
| SLO alert-runbook entries (V5-A/B) | Net-new ops doc | `bubbles.upkeep` + `bubbles.docs` | spec 049 |
| Demo videos | None at v1 (same as MVP); operator decision | n/a | n/a |
| Public testimonials | None — same as MVP | n/a | n/a |

## Launch sequence

1. **Internal acknowledgement** when all V-items terminal + V6-A drift sweep clean.
2. **`README.md` + `INVESTOR_OVERVIEW.md` refresh** via `bubbles.docs`.
3. **`docs/Capability_Map.md` refreshed** by V4-A generator.
4. **Optional external "v1 capability gate reached" post** — operator decision.
5. **RELEASE-V2 planning kickoff** — handover to `docs/releases/v2/` (if operator decides v2 exists).

## Forbidden marketing patterns

Per [`docs/Product-Principles.md`](../../Product-Principles.md):
- ❌ "Smackerel autonomously runs your life / makes decisions for you" — violates Principle 6 (consent + budget) and the consent gate of V2-A
- ❌ "Smackerel manages your finances / advises you financially" — Principle 10 (QF boundary)
- ❌ "End-to-end encrypted" — still false at v1
- ❌ Any "magic AI" framing — Smackerel's value is the contract (observe + ingest + surface within budget + act with consent), not the model
- ❌ Any claim of native mobile if V3-A keeps PWA
- ❌ Any outbound-action capability claim that does NOT include consent / audit / dry-run / undo language
- ❌ Streak / unread-count / backlog framing (Principle 9)
- ❌ Multi-tenant SaaS framing — v1 remains self-hosted

## Honest v1 framing

> "Smackerel v1 is the moment Smackerel becomes a daily-driver knowledge tool for users beyond the Compose-comfortable hobbyist — ingesting from the productivity sources serious knowledge workers actually use, and able to ACT on the user's behalf under explicit consent."

That's the honest message. Anything beyond it requires evidence in [`features.md`](features.md).
