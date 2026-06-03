# Marketing — Smackerel MVP

> Every claim in this file traces to a row in [`features.md`](features.md). No claim is fabricated. No competitor is invented.

## Posture

MVP marketing is **low-volume, technically-honest, self-hosted-audience-first**. Smackerel is not a launch product at MVP; it is a founding-promise gate that proves the architecture works. Marketing assets exist to (a) recruit aligned early users, (b) document the surfaced [`docs/Product-Principles.md`](../../Product-Principles.md) contract publicly, and (c) provide reference material for any downstream commercial decision in RELEASE-V1+.

## Audience segments + messaging

| Segment | Who | Core message (claims trace to features.md) | Anti-message |
|---------|-----|--------------------------------------------|--------------|
| **Self-hosters** (Homelab, Tailscale users, Docker Compose comfortable) | Owns hardware, distrusts cloud-SaaS for personal data | "Smackerel observes your already-existing digital footprint across 18+ source types ([`features.md`](features.md) connector table), answers vaguely-phrased questions precisely, and respects a strict daily interruption budget — running entirely on your hardware." | Do NOT pitch as "easy install" — Compose comfort required |
| **Knowledge workers exhausted by manual organization** (Notion/Obsidian abandoners) | Tried PKM tools, hit the organize-everything wall | "Observation first, organization second. Smackerel ingests passively from email, calendar, browser, maps, messaging, RSS — no tagging at capture, no manual filing." ([`docs/Product-Principles.md`](../../Product-Principles.md) Principle 1, 2) | Do NOT pitch as "second brain" — that frame implies user-curation work |
| **Privacy-aware power users** (e2ee chat users, Tailscale-only access) | Will not put personal data in a cloud SaaS | "Local-first. Self-hosted. Your data never leaves your network unless you point an LLM provider at a cloud model — and the source-attribution metadata travels with every synthesis." ([`docs/Product-Principles.md`](../../Product-Principles.md) Principle 8) | Do NOT claim e2ee — Smackerel is not e2ee; it is self-hosted |
| **QF Companion users** (sibling product) | Already running QF; want to ingest QF decision packets into personal knowledge | "Smackerel preserves QF decision-packet metadata (`CalibrationBadge`, `DataProvenanceBadge`, packet IDs) without modification, and exports consent-scoped `PersonalEvidenceBundle`s back to QF." ([`docs/smackerel.md`](../../smackerel.md) §1.6, spec 041) | Do NOT claim Smackerel gives financial advice or executes trades — both forbidden per Principle 10 |

## Channel strategy

| Channel | Use | Cadence |
|---------|-----|---------|
| Project README ([`README.md`](../../../README.md)) | Primary entry; honest scope summary | Updated when M-items close |
| `docs/INVESTOR_OVERVIEW.md` | Long-form "what is this" doc | Updated by DOC-3 action when operator confirms |
| `docs/Product-Principles.md` (ratified) | The contract Smackerel commits to | Ratified by M3; thereafter binding |
| Release packet folder (this packet) | Reference for the MVP gate decisions | Locked at `docs_updated` |
| External posts / talks | Optional — only after all M-items terminal | Not authorized by this packet |

## Asset list

| Asset | Status | Owner | Source-of-truth claim trace |
|-------|--------|-------|----------------------------|
| Updated [`README.md`](../../../README.md) reflecting MVP capability set | Pending (after M-items close) | `bubbles.docs` dispatch | [`features.md`](features.md) |
| Updated [`docs/INVESTOR_OVERVIEW.md`](../../INVESTOR_OVERVIEW.md) Phase Overview table with MVP-gate row | Pending (DOC-3 in [`actions.md`](actions.md)) | `bubbles.docs` dispatch | [`features.md`](features.md), [`vision.md`](vision.md) |
| Architecture diagram showing 18+ connectors → bus → graph → surfacing controller | Already in [`docs/Architecture.md`](../../Architecture.md); refresh after M1a lands | `bubbles.docs` dispatch | [`docs/smackerel.md`](../../smackerel.md) §3.1 |
| "Daily interruption budget" explainer (one-pager) | Net-new; author after M1a SLOs operator-confirmed | `bubbles.docs` dispatch | [`features.md`](features.md) M1a, [`actions.md`](actions.md) OQ-1/2/3 |
| Wiki/graph-browse UI screencast (≤ 60 s) | Net-new; produce after M2 lands | TBD — operator | [`features.md`](features.md) M2 |
| Principle-ratification announcement (changelog entry) | Pending; author at M3 close | `bubbles.docs` dispatch | [`actions.md`](actions.md) DOC-1/2 |
| Public testimonials | None — not authorized at MVP (no external users) | n/a | n/a |
| Demo videos beyond the M2 screencast | None at MVP | n/a | n/a |

## Launch sequence

1. **Internal-only acknowledgement** when all M-items reach terminal-for-mode + OPS-1 passes.
2. **`README.md` + `INVESTOR_OVERVIEW.md` refresh** via `bubbles.docs` dispatch.
3. **Optional external "MVP gate reached" post** — operator decision; no obligation in this packet.
4. **RELEASE-V1 planning kickoff** — handover to [`docs/releases/v1/`](../v1/).

## Forbidden marketing patterns

Per [`docs/Product-Principles.md`](../../Product-Principles.md) and [`docs/smackerel.md`](../../smackerel.md) §1.6:
- ❌ "Smackerel manages your finances" / "Smackerel gives advice" — violates Principle 10
- ❌ "Smackerel never bothers you" — incorrect; it bothers within a budget (Principle 6). Use the honest framing.
- ❌ "Smackerel encrypts end-to-end" — false; self-hosted ≠ e2ee
- ❌ Any claim that a connector exists when it does not (Personal Productivity Sources are RELEASE-V1)
- ❌ Any claim that Smackerel writes back to source systems (Outbound Action is RELEASE-V1)
- ❌ Any backlog/streak/guilt-inducing UX framing (violates Principle 9)
