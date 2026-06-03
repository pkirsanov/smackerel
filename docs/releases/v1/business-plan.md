# Business Plan — Smackerel v1

## Target audience for v1

Two segments (expanded from MVP's single self-hoster audience):

1. **Serious knowledge workers** living across Gmail / Outlook / Apple Calendar / Notion / Obsidian / Reminders families / messages / voice. Still technically capable (self-hosted), but with a substantially broader ingestion surface need than MVP's IMAP-plus-CalDAV-plus-Keep baseline served.
2. **Daily-driver mobile users** whose use case demands either a native mobile app (V3-B/C) or substantially upgraded PWA (V3-A may keep PWA path).

The MVP target audience (technically-comfortable single-host self-hosters) is retained without regression.

## Value proposition (v1)

> "Observe everywhere your knowledge actually lives. Act on your behalf when you ask — under explicit consent, with audit, dry-run, and undo. Respect the same daily interruption budget as before."

Consistent with [`vision.md`](vision.md) and with [`docs/Product-Principles.md`](../../Product-Principles.md) principles 1, 2, 6, 8, 9.

## Pricing model (v1)

**Operator decision pending (OQ-V9 in [`actions.md`](actions.md)).** v1 capability arrival unlocks the monetization conversation but does NOT commit to a pricing model. See [`monetization.md`](monetization.md).

## Competition (re-assessed for v1)

| Competitor | v1 gap (vs Smackerel post-v1) | Smackerel v1 advantage |
|------------|--------------------------------|------------------------|
| Notion AI / Notion Q&A | Cloud-only; no cross-source ingestion outside Notion | Smackerel ingests Notion AND everything else; self-hosted |
| Obsidian (+ Smart Connections plugin) | Local notes only; no calendar/email/messages | Smackerel pulls Obsidian as one source among many |
| Apple Intelligence / Apple Reminders+Notes auto-suggest | Apple-ecosystem locked; closed source; cloud-tied | Smackerel runs cross-platform; self-hosted |
| Microsoft 365 Copilot | M365-locked; cloud SaaS; no cross-vendor | Smackerel ingests M365 + Google + Apple + open-source |
| Rewind.ai / Recall / Microsoft Recall | Local-machine snapshot; no cross-device source ingestion; no outbound action | Smackerel cross-device, cross-source, with consented outbound action |
| Mem.ai / Reflect / Tana / Anytype | SaaS-first; no native outbound to user's own systems | Smackerel self-hosted with outbound to user's own systems |
| Personal AI / Pi / Inflection | Conversational only; no graph; no source ingestion | Smackerel ingests + graphs + converses + acts |
| Khoj / self-hosted RAG stacks | Document-pile RAG; no surfacing controller; no outbound action | Smackerel has surfacing-budget contract (MVP M1a) + outbound action (V2) |

The v1 wedge is **the combination**: passive ingestion across the serious-user source set, surfacing within a user-respecting budget, AND consented outbound action — none of the above competitors covers all three.

## Risk assessment

| ID | Risk | Likelihood | Impact | Mitigation |
|----|------|-----------|--------|-----------|
| RV1 | Outbound Action capability (V2-A) misused → external account action user did not consent to | Low (consent-gated) | **Catastrophic** (trust loss) | V2-A foundation MUST include: dry-run as default for first invocation per action type, undo window (OQ-V6), audit log non-deletable, per-target rate limit (OQ-V5), explicit opt-in per action type. Per [`bubbles-capability-foundation-design`](../../../.github/skills/bubbles-capability-foundation-design/SKILL.md) the foundation MUST land BEFORE per-target actions ride on it. |
| RV2 | Apple Calendar / Reminders / Notes / Messages bridges require macOS/iOS device for ingestion | High (Apple ecosystem reality) | Medium (audience-narrowing) | Document explicitly in V1-E/F/G/H specs; offer iCloud-CalDAV fallback where available; non-Apple users simply don't enable those connectors |
| RV3 | Personal Productivity Sources expansion (V1-A..I) overwhelms ML sidecar / postgres | Medium | Medium | Per-spec capacity testing; spec 050 ML sidecar scaling already in place; pgvector index tuning per OPS guidance |
| RV4 | Connector OAuth flows multiply support burden (each provider has its own quirks) | High | Medium | Centralize OAuth-token storage + refresh per spec 044/060; consent UX in V2-A foundation reused for inbound auth where possible |
| RV5 | Native mobile decision (V3-A) chooses native → effectively doubles maintenance surface | Medium | High | DOC-V1 decision must include explicit honest cost/benefit; PWA-only is a valid choice |
| RV6 | Auto-generated capability map (V4-A) reveals undocumented capability claims in MVP that don't match reality → marketing-claim retraction needed | Medium | Low | Run V4-A early; treat any surfaced mismatch as `bubbles.spec-review` MAJOR_DRIFT — fix the spec or fix the marketing |
| RV7 | SLO alerting (V5-A/B) generates alert fatigue → operator silences alerts → reverts to MVP measured-only posture | Medium | Medium | Tune alert thresholds against first month of v1 telemetry before paging; document tuning playbook per spec 049 |
| RV8 | Messages family (V1-H) ingestion of e2ee chats (Signal, iMessage) requires device bridges → bridge security compromise leaks chats | Low (operator-controlled bridges) | **Catastrophic** | V1-H spec design MUST address bridge auth model + isolation; potentially defer Signal/iMessage to v1.5 if bridge model unclear at spec-creation time |
| RV9 | v1 spec count balloons (16+ new spec slots in [`features.md`](features.md)) → portfolio coordination cost dominates implementation cost | High | Medium | Sequence per [`features.md`](features.md) dependencies; V2-A foundation first; V1-A..I in parallel as capacity allows; V3-A can defer per OQ-V7 |
| RV10 | Monetization conversation triggered prematurely → operator pressured into a model that conflicts with principles | Low | High | OQ-V9 explicit: v1 unlocks but does not commit. [`monetization.md`](monetization.md) takes the same posture. |

## Capital requirements

**v1 implementation does NOT require external capital.** Operator-self-funded under existing infrastructure costs. Each new connector / outbound-action target is incremental engineering work, not infrastructure investment.

Capital becomes a question only if:
- Operator decides to pivot to commercial hosting (post-v1)
- Native mobile decision (V3-A) chooses native AND operator wants accelerated delivery
- A commercial outbound-action partnership is pursued (e.g., a vendor sponsorship)

None of these are commitments at v1.

## Pre-v1 commercial-conversation gates

Before any commercial framing of v1, the following MUST be true:
1. All V-items in [`features.md`](features.md) reach terminal-for-mode
2. `bubbles.devops` validates [`deployment.md`](deployment.md) technical claims (OPS-V2)
3. `bubbles.spec-review` shows zero MAJOR_DRIFT (V6-A / OPS-V1)
4. [`docs/Product-Principles.md`](../../Product-Principles.md) Principle 10 (QF Companion Boundary) remains intact
5. Operator personally signs off on any "AI agent that acts on your behalf" claim — V2-A surface MUST be opt-in per action type, not blanket
