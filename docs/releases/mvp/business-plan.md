# Business Plan — Smackerel MVP

## Target audience for MVP

A single technically-comfortable user — capable of running Docker Compose on their own hardware, comfortable with a `.sh` runtime entrypoint, willing to host their own data — who currently loses personal knowledge to fragmentation across email, calendar, browser, maps, messaging, RSS, and personal connectors.

Secondary audience: a small household (2–5 users) sharing one Smackerel instance under per-user bearer auth (spec 044, 060).

This is **not** a commercial offering at MVP. It is a self-hosted personal product with an explicit, surfaced [`docs/Product-Principles.md`](../../Product-Principles.md) contract.

## Value proposition

> "Observe everything I already produce. When I ask vaguely, answer precisely. Surface what matters without asking me to organize anything — and respect a strict daily interruption budget."

Consistent with [`vision.md`](vision.md). Sourced from [`docs/smackerel.md`](../../smackerel.md) §1.2, §1.3, §1.4 + [`docs/Product-Principles.md`](../../Product-Principles.md) principles 1, 2, 6.

## Pricing model

**MVP is pre-revenue.** No pricing tier, no paid plan, no subscription. See [`monetization.md`](monetization.md) for the deferred monetization stance.

Self-hosted under whatever container/storage costs the user already pays for their own infrastructure. No Smackerel-charged costs.

## Competition

Competitors are evaluated against the founding promise (passive ingestion + vague-in/precise-out + interruption-budgeted surfacing + self-hosted).

| Competitor | Founding-promise gap | Why Smackerel MVP wins for the target audience |
|------------|----------------------|------------------------------------------------|
| Notion / Obsidian / Logseq | Require user to organize at capture; no passive ingestion across email/calendar/browser/maps | Smackerel observes first, asks second (Principle 1) |
| Apple Spotlight / macOS Recall / Windows Recall / Rewind.ai | Closed-source; cloud-dependent; OS-locked; no graph layer | Smackerel is self-hosted, cross-platform, exposes a knowledge graph |
| Mem.ai / Reflect / Tana | SaaS-only; user-organization-required; no source-system passive ingestion | Smackerel is local-first; ingests passively from 18+ connectors |
| Self-hosted research assistants (Khoj, Continue, Open WebUI + RAG) | Document-pile RAG; no calendar/maps/messaging/connector breadth; no surfacing controller | Smackerel ingests breadth + has the surfacing-controller MVP item |
| QF Companion (sibling product) | Different domain entirely — financial decision support | Smackerel is the personal-knowledge counterpart; preserves QF interop via spec 041 |

No competitor in this list offers all four founding-promise pillars together. That is the MVP wedge.

## Risk assessment

| ID | Risk | Likelihood | Impact | Mitigation |
|----|------|-----------|--------|-----------|
| R1 | Surfacing controller (M1a) misses operator-decided SLOs (OQ-1/2/3) | Medium | High — undermines Principle 6 | Open questions in [`actions.md`](actions.md); spec 021 adjustment forces explicit SLO commitment before implement |
| R2 | Ratification of Product-Principles.md (M3) flushes out existing violations in committed code → PRs blocked | High | Medium | Operator decision OQ-7 supports staged ratification if mass-violation surfaces; `release-train-guard` does not yet enforce these gates so initial blocking is in PR review only |
| R3 | Spec 026 MAJOR_DRIFT fix uncovers second-order drift in scenario manifests of dependent specs | Low | Low | `bubbles.workflow improve-existing` handles cascading drift naturally |
| R4 | Wiki surface (M2) confuses users by exposing graph internals (topics/edges) instead of artifacts | Medium | Medium | M2 design adjustment must include UX testing scenarios; default view should remain artifact-centric with pivot UI as opt-in |
| R5 | "MVP-complete" claim made externally before all 10 M-items dispatch terminal | Low | High — reputational | This packet is `docs_updated`; no external claim authorized. Operator must wait for ENG-1..6 + DOC-1..3 + OPS-1 close-out before declaring MVP-complete |
| R6 | Connector roster lock breaks during MVP window because operator adds a connector mid-flight | Low | Medium | Roster lock is policy, not technical; any in-flight connector addition becomes RELEASE-V1 scope automatically |
| R7 | Adding M-items as adjustments to existing specs creates artifact-size bloat that triggers framework gates (large scope.md, etc.) | Medium | Low | `bubbles.workflow improve-existing` mode supports new-scope-directory layout for large adjustments per [`bubbles-scope-workflow-runtime`](../../../.github/skills/bubbles-scope-workflow-runtime/SKILL.md) |
| R8 | Adversarial input to the reminder engine (M1c) creates DoS via promise spam | Low | Medium | Spec 054 adjustment must include input-rate ceiling + per-promise resource budget |
| R9 | Graph-browse (M2) exposes annotated artifacts containing sensitive data to households-as-users without proper auth filtering | Medium | High | Per-user bearer auth (spec 044, 060) MUST scope graph traversal; M2 adjustment must include scenarios for cross-user isolation |
| R10 | MVP gate declared before EB-7 idempotence (OPS-1) verified → portfolio drift report still misses 54-spec banner sweep audit | Low | Low | OPS-1 is a 1-minute grep; trivially close-able before any external claim |

## Capital requirements

None at MVP. The product is self-hosted; the owner-operator's existing infrastructure carries any runtime cost. No team hire, no infrastructure spend, no marketing spend are introduced by MVP.

Capital posture is a RELEASE-V1+ question (when Outbound Action capability + Personal Productivity Sources + native mobile decision converge into something with commercial gravity).
