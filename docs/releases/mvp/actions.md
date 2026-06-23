# Actions — Smackerel MVP

Action items required to close the MVP gate, grouped by owner. All items are **planning outputs of this packet, not work performed by this packet**. The `route_required` dispatches in the [Next Dispatches](#next-dispatches) section below are the operator's playbook.

> **Reconciliation note (2026-06-06).** The action items below were planning
> outputs of the 2026-06-03 MVP packet and have since been **delivered and
> reconciled** (2026-06-06). [`features.md`](features.md) (Gate G101-bound) is now
> the **authoritative delivery record**: per its MVP delivery summary, M1a / M2a /
> M2b / M4 / M5d are delivered and validate-certified; M1c-basic is delivered;
> M5a / M5c are delivered-and-carried; M3 is delivered; and M1b, the M1c full
> conditional/arrival promise engine, and M5b are portfolio-approved deferrals to
> release-v1. This `actions.md` is retained as a **historical planning snapshot**,
> not the live action ledger.

## Engineering (route via `bubbles.workflow` dispatches)

| ID | Action | Owner spec | Priority |
|----|--------|-----------|----------|
| ENG-1 | Implement M1a (global interruption-budget controller) — **delivered** via a dedicated new spec `078-cross-surface-surfacing-prioritizer`, which adopted pre-existing in-tree controller groundwork rescoped OUT of 021 by commit `640b95d0` (not an in-place adjustment to 021) | 078 | P0 — MVP-blocking |
| ENG-2 | Implement M1b (calendar-triggered briefs) via adjustment to spec 025 | 025 | P0 — MVP-blocking |
| ENG-3 | Implement M1c (reminder/promise engine) via adjustment to spec 054 | 054 | P0 — MVP-blocking |
| ENG-4 | Implement M2 (wiki/graph-browse) via adjustments to specs 073 + 027 | 073, 027 | P0 — MVP-blocking |
| ENG-5 | Apply 026 MAJOR_DRIFT fix (scenario-manifest rewire) | 026 | P0 — MVP-blocking (clears portfolio drift) |
| ENG-6 | Apply MINOR_DRIFT fixes 039 / 058 / 067 | 039, 058, 067 | P1 — MVP cosmetic close-out |

## Docs (route via `bubbles.docs`)

| ID | Action | Target | Priority |
|----|--------|--------|----------|
| DOC-1 | Ratify [`docs/Product-Principles.md`](../../Product-Principles.md) 1–10 (banner: "Surfaced for owner approval" → "Ratified 2026-06-03") | `docs/Product-Principles.md` | P0 — MVP gating |
| DOC-2 | Flip [`.github/instructions/product-principles.instructions.md`](../../../.github/instructions/product-principles.instructions.md) advisory → BLOCKING for principles 1–10 (per the [Next Dispatches](#next-dispatches) edit shape) | `.github/instructions/product-principles.instructions.md` | P0 — MVP gating |
| DOC-3 | Update [`docs/INVESTOR_OVERVIEW.md`](../../INVESTOR_OVERVIEW.md) Phase Overview table to add an "MVP Gate (2026-06-03)" row pointing at this packet | `docs/INVESTOR_OVERVIEW.md` | P1 — packet visibility |

## Ops (route via `bubbles.workflow validate-only`)

| ID | Action | Target | Priority |
|----|--------|--------|----------|
| OPS-1 | Verify OPS-001 EB-7 idempotence: `grep -L "**Status:** Done"` across the 54 enumerated specs | `specs/_ops/OPS-001-spec-banner-sweep` | P2 — audit close-out |

## Owner decisions still pending (open questions)

These are surfaced for operator decision and parked here per the agent's non-interactive default. They are **NOT MVP blockers individually** but each gates one MVP item's full design.

| OQ-ID | Question | Affects MVP item | Suggested default if operator silent |
|-------|----------|------------------|--------------------------------------|
| OQ-1 | What is the numeric per-day nudge ceiling N for the global interruption-budget controller? | M1a | Start at 3/day (matches [`docs/Product-Principles.md`](../../Product-Principles.md) Principle 6 "< 3 per week" baseline as the conservative MVP target; spec 021 adjustment refines) |
| OQ-2 | What is the target acted-on-rate SLO? (e.g., ≥ 40% of nudges actioned within 24h) | M1a | Recommend 40% as initial published SLO; revisit after first week of telemetry |
| OQ-3 | What is the false-positive ceiling SLO? (% of nudges marked "not useful") | M1a | Recommend ≤ 15% as initial ceiling |
| OQ-4 | For M1b, what calendar-event filter qualifies for a brief? (All events? Only those with attendees? Only future events with attached docs?) | M1b | Recommend: events with ≥ 1 external attendee OR ≥ 1 attached doc OR ≥ 1 linked artifact in graph |
| OQ-5 | For M1c reminder engine, does "condition-based" support arbitrary graph-state predicates, or only a fixed vocabulary (e.g., artifact-arrived, calendar-event-occurred, contact-replied)? | M1c | Recommend fixed vocabulary in MVP; arbitrary predicates is RELEASE-V1 territory |
| OQ-6 | For M2 wiki surface, are write-side annotations stored as new artifacts (linked back to source) or as overlays on existing source artifacts? | M2 | Recommend: overlays linked back via annotation edges — preserves source immutability per Principle 4 |
| OQ-7 | M3 ratification: is operator ready to flip ALL 10 principles simultaneously, or stage them (e.g., 1, 2, 3, 6, 7, 8, 9, 10 BLOCKING and 4, 5 advisory until codebase audited)? | M3 | Operator decision — packet does not assume. `bubbles.docs` dispatch should confirm before editing. |

> **Clarifying note (2026-06-23):** OQ-1 concerns the **Tier-1 daily NUDGE ceiling N** — shipped as `5` in `surfacing.daily_nudge_budget` and enforced by the spec-078 surfacing controller — which is a DIFFERENT budget from the **Tier-3 `< 3 system-initiated prompts/week` (non-urgent) SLO** ([`docs/smackerel.md`](../../smackerel.md) §1.4 success-metrics table). The "3/day" suggested-default above is the daily nudge ceiling, not the weekly prompt SLO; see the three-tier interruption-budget taxonomy comment in `config/smackerel.yaml` (`surfacing:` block).

## Cross-product coordination actions

| ID | Action | Counterparty |
|----|--------|--------------|
| XP-1 | None in MVP — QF Companion boundary (spec 041 + [`docs/smackerel.md`](../../smackerel.md) §1.6) is preserved without change. | n/a |

## Items explicitly NOT taken on in this packet

Per non-goal discipline:
- Do not edit any spec artifact (all M-items dispatch to spec-owning workflows).
- Do not edit source code.
- Do not ratify [`docs/Product-Principles.md`](../../Product-Principles.md) here — `bubbles.releases` does not own that file.
- Do not mutate [`.github/instructions/product-principles.instructions.md`](../../../.github/instructions/product-principles.instructions.md) here.
- Do not update [`docs/INVESTOR_OVERVIEW.md`](../../INVESTOR_OVERVIEW.md) yet — operator confirmation that this packet is the canonical MVP gate is required first (open question implicit; if operator confirms, dispatch DOC-3).

## Next Dispatches

The operator should dispatch these AFTER this release packet closes. They are **not executed by this packet** (planning only).

```yaml
# HISTORICAL SNAPSHOT (2026-06-03 planning) — superseded by features.md delivery
# reconciliation (2026-06-06). Retained for provenance; not the live dispatch list.
- agent: bubbles.workflow
  mode: improve-existing
  spec: specs/026-domain-extraction
  reason: spec-review:MAJOR_DRIFT
  rationale: |
    scenario-manifest.json links 11+ scenarios to deleted internal/api/domain_intent*.go
    (spec 066 SCOPE-4, commit 1f74d5c0). Rewire to current canonical homes under
    internal/intelligence/ and spec 068 compiler tests, or explicitly mark as
    historically-removed in the manifest.
  evidence: specs/_spec-review-report.md (MAJOR_DRIFT section)

- agent: bubbles.docs
  scope: single-file
  target: .github/instructions/product-principles.instructions.md
  action: ratify principles 1–10 (flip advisory → BLOCKING)
  edit_shape: |
    - Update the front-matter "STATUS" block: replace "advisory" language with
      "Ratified 2026-06-03 by owner; BINDING."
    - For each principle 1–10 enforcement row, replace "Advisory until ratified"
      with "BLOCKING (enforced via grep in PR review + pre-push)."
    - Remove the Pre-Ratification Checklist section (all boxes presumed checked
      by operator at ratification time) OR mark each box [x] with the
      ratification date.
    - Update Product-Principles.md banner from "Surfaced for owner approval" to
      "Ratified 2026-06-03" per the principle entries.
  rationale: |
    MVP item 3 — ratification gate. Operator decision recorded in this packet.
    bubbles.releases MUST NOT make this edit itself per its ownership boundary.
  evidence: docs/releases/mvp/features.md (item M3)

- agent: bubbles.workflow
  mode: improve-existing
  spec: specs/078-cross-surface-surfacing-prioritizer  # superseded: M1a delivered via spec 078 (commit 640b95d0), rescoped out of 021
  reason: release-planning:MVP-gap-C-surfacing-controller
  rationale: |
    Add unified "Next Smackerel" prioritizer that owns the GLOBAL user-interruption
    budget across digest, push, Telegram, web, ntfy, email-out — measurable SLOs
    (≤ N nudges/day, target acted-on rate, false-positive ceiling). Add scope
    + design section + scenarios. See docs/releases/mvp/features.md item M1a.

- agent: bubbles.workflow
  mode: improve-existing
  spec: specs/025-knowledge-synthesis-layer
  reason: release-planning:MVP-gap-C-calendar-briefs
  rationale: |
    Add calendar-triggered brief producer (lead-time scheduled briefs tied to
    upcoming CalDAV events). Add scope + design tie to scheduler. See
    docs/releases/mvp/features.md item M1b.

- agent: bubbles.workflow
  mode: improve-existing
  spec: specs/054-notification-intelligence-handler
  reason: release-planning:MVP-gap-C-reminder-promise-engine
  rationale: |
    Add user-stated future-intent reminders ("ping me if X hasn't happened by Y").
    Scheduler-backed. Add scope + design section + scenarios. See
    docs/releases/mvp/features.md item M1c.

- agent: bubbles.workflow
  mode: improve-existing
  spec: specs/073-web-mobile-assistant-frontend
  reason: release-planning:MVP-gap-F-wiki-graph-browse
  rationale: |
    Add graph-browse views by topic / person / place / time with rendered
    cross-links. Annotations remain spec 027's domain — this is the browse
    surface. See docs/releases/mvp/features.md item M2.

- agent: bubbles.workflow
  mode: improve-existing
  spec: specs/027-user-annotations
  reason: release-planning:MVP-gap-F-editable-annotations
  rationale: |
    Extend editable annotation surface beyond current spec 027 to support the
    wiki surface delivered in 073 adjustment. See docs/releases/mvp/features.md item M2.

- agent: bubbles.workflow
  mode: improve-existing
  spec: specs/039-recommendations-engine
  reason: spec-review:MINOR_DRIFT
  rationale: Re-point evidence cell from deleted internal/api/domain_intent.go to spec 068 compiled-intent path.

- agent: bubbles.workflow
  mode: improve-existing
  spec: specs/058-chrome-extension-bridge
  reason: spec-review:MINOR_DRIFT
  rationale: Reconcile or clear done_with_concerns flag against current state.

- agent: bubbles.workflow
  mode: improve-existing
  spec: specs/067-intent-driven-policy-enforcement
  reason: spec-review:MINOR_DRIFT
  rationale: Re-frame design.md L137 inventory entry as past-tense ("retired by spec 066 SCOPE-4").

- agent: bubbles.workflow
  mode: validate-only
  spec: specs/_ops/OPS-001-spec-banner-sweep
  reason: spec-review:EB-7-idempotence-verification
  rationale: Run grep -L for canonical Done banner across the 54 enumerated specs to confirm idempotence holds.
```
