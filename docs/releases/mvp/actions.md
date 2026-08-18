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

## Live routed defects (post-reconciliation — NOT part of the historical snapshot above)

Everything above this heading is the 2026-06-03 planning snapshot. The rows below are
**live** routes raised while reconciling this packet's machine bindings, and they are
the target of the cross-references from [`features.md`](features.md).

| ID | Finding | Owner | Why it is not fixed in this packet |
|----|---------|-------|------------------------------------|
| RTE-M1 — **cause fixed upstream; binding not yet flipped** | [`specs/039-recommendations-engine`](../../../specs/039-recommendations-engine/) is `done` and genuinely validate-certified, but its `state.json` records `certification.certifiedCompletedPhases` as a **mixed array** of strings and objects. Gate G101 reads that array with `.[]? \| ascii_downcase`, which aborts at the first object; in 039's case every string element before that point excludes `validate`, so the guard cannot see the certification. The binding is therefore `carried`, not `required` — the capability is delivered, the record shape is the defect. Verified at HEAD `8d971420`: first object element is at index 12; `validate` appears only after it. **Re-measured 2026-08-18 at HEAD `03474d3a` and unchanged:** 18 entries, mixed `str`/`dict`, first object at index 12, `validate` present. **The guard defect is now fixed at the framework source** — `bubbles` commit `18c8733` makes `is_validate_certified` map object entries through `.phase`, so no element can truncate the stream (see [`../next/actions.md`](../next/actions.md) RTE-N2 for the mechanism and its adversarial proof). **This row stays open** because `.github/bubbles/` is a framework-managed install refreshed only by the canonical installer; this repo still runs the pre-fix copy. **Flip condition** — after a framework refresh, re-run `./smackerel.sh release reconcile`; if 039 then reads as certified, rebind `m5a-recommendations-drift` to `delivery=required`. | `bubbles.plan` (owns every spec's `state.json`); `bubbles.releases` (owns the binding, after refresh) | `bubbles.releases` does not edit any spec's `state.json`. Flipping the binding to `required` without fixing the record would turn this gate red for a data-shape reason and teach readers to distrust it. The record shape was also left alone on purpose: normalising it would have masked a framework defect that mis-reports delivery in **every** downstream repo, so the repair was made where the defect lives. |
| RTE-M2 — **risk removed upstream; still latent here** | [`specs/038-cloud-drives-integration`](../../../specs/038-cloud-drives-integration/) and [`specs/040-cloud-photo-libraries`](../../../specs/040-cloud-photo-libraries/) carry the **same** mixed-array shape as 039. They currently pass only because `validate` happens to appear at index 3, before the first object at index 12. **Measured directly, old expression vs new, against these two records:** the shipped expression sees **12 of 40** phase entries on each of 038 and 040 — it discards **70%** of the certification record — and they pass only because `validate` lands inside the surviving 12. The fixed expression sees **40 of 40**. On 039 the same probe gives 12 (no `validate` → read as uncertified) versus 18 (`validate` present → certified), which is the whole of the difference between its `carried` and `required` binding. Their `required` bindings are correct today but rest on element ordering, not on a guaranteed shape. Fixing all three together is the durable repair. **The ordering dependence is now removed at the framework source** by `bubbles` commit `18c8733`; selftest case **S34** exists specifically to hold that line, asserting a pass when `validate` appears *after* an object. Until a framework refresh lands, these two bindings still rest on luck **in this repo** — the difference is that the luck is no longer load-bearing upstream, so the refresh converts them from accidentally-correct to structurally-correct without any edit here. | `bubbles.plan`; framework refresh via `bubbles.setup` | Same ownership boundary as RTE-M1. |
| RTE-M3 | The guard's `is_validate_certified` treats a jq parse abort as "not validate-certified" rather than as malformed input. The direction is fail-safe, but the diagnostic is wrong in kind: a `state.json` shape defect is reported to the operator as a delivery defect. Worth a distinct diagnostic upstream. | Bubbles framework — route via `bubbles.setup` | `release-delivery-reconciliation-guard.sh` is a framework-managed install artifact and is not patched downstream. |
| RTE-M4 | A release packet can bind a `spec-scope-hardening` packet as `delivery=required`, which is **unsatisfiable by construction**: that mode's `statusCeiling` is `specs_hardened` ([`workflows/modes.yaml`](../../../.github/bubbles/workflows/modes.yaml)), which is never delivery-capable, so no amount of work can turn the binding green. Gate G101 reports this as a DELIVERY defect when it is a BINDING category error. Two MVP rows hit it: `m5d-spec-banner-sweep` → [`_ops/OPS-001`](../../../specs/_ops/OPS-001-spec-banner-sweep/) and `ops-g088-certifiedat-backfill` → [`_ops/OPS-002`](../../../specs/_ops/OPS-002-g088-certifiedat-backfill/). Both are validate-certified (`validate` is present in each `certification.certifiedCompletedPhases`); only the ceiling blocks them. Both bindings are now `carried`. **Suggested upstream fix:** reject `delivery=required` at annotation-validation time when the bound spec's `workflowMode` has a non-delivery-capable ceiling, and emit a distinct diagnostic instead of a delivery failure. | Bubbles framework — route via `bubbles.setup` | `release-delivery-reconciliation-guard.sh` is a framework-managed install artifact and is not patched downstream. |
| RTE-M5 — **corrected 2026-08-18; disposition is DO NOT EXECUTE** | The [`_ops/OPS-001`](../../../specs/_ops/OPS-001-spec-banner-sweep/) banner sweep was a point-in-time remediation and drift has **re-accumulated**. **Re-measured at `HEAD` `57bcb187` (2026-08-18)** over every `specs/*/state.json` with `status == "done"` that has a sibling `spec.md` (blockquote-aware banner regex `^\s*(?:>\s*)?\*\*Status:\*\*\s*(.+)$`): **99** such specs, **62** carrying a genuinely canonical Done banner, **2** carrying no banner at all (`070-web-username-password-login`, `090-observability-slo-dogfood`), **35** carrying a stale banner (`057`, `059`, `060`, `062`, `064`, `065`, `066`, `067`, `068`, `069`, `071`, `072`, `073`, `074`, `075`, `076`, `077`, `078`, `080`, `081`, `083`, `084`, `085`, `086`, `087`, `088`, `089`, `091`, `093`, `094`, `097`, `099`, `100`, `102`, `103`) — **37 drifted**. This row previously claimed 84 canonical / 13 stale / 15 drifted; **those figures were wrong and are superseded.** <br><br>**Re-measurement warning — this metric has now been mismeasured twice, in OPPOSITE directions. Both traps must be avoided:** **(1)** a naive regex that does not allow the `> ` blockquote prefix inflates the drift count to 41 false positives. **(2)** classifying a banner as canonical because it *contains* the word "done" silently accepts **22** banners of the form ``not_started (ceiling = `done`)`` and ``in_progress (planning bootstrap; ceiling = `done`)`` — the literal word "done" appears in the *ceiling* clause while the status word itself is `not_started` / `in_progress`. Proof: strict `startswith("done")` yields **62**; `contains("done")` yields 62 + 22 = **84**, exactly reproducing the superseded claim. Examples: `062-per-transport-configuration-audit` = ``not_started (ceiling = `done`)``; `064-open-ended-knowledge-agent` = ``in_progress (planning bootstrap; ceiling = `done`)``. **The correct test is that the banner STARTS WITH the canonical status word, not that it mentions it.** | `bubbles.plan` (owns every spec's `state.json`), with execution by an implementation agent | **DO NOT EXECUTE the banner sweep as designed.** The sweep edits `spec.md` files, which `bubbles.releases` does not own, and executing it would convert a cosmetic inconsistency in 37 `spec.md` files into **37 NEW Gate G088** (`post_certification_spec_edit_gate`) violations **on top of the 40 that already exist** — measured by running `post-cert-spec-edit-guard.sh` over all 99 done specs at `HEAD` `57bcb187`: **PASS 59 / FAIL 40**. That collision is no longer a prediction: one of the causing commits is literally `docs(092): SR-09 reconcile spec header Status to done (matches state.json)`, i.e. a prior session performed exactly this RTE-M5 remediation on spec `092-card-rewards-ui-elevation`, and 092 is now one of the 40 failures. This is the hazard [`_ops/OPS-002`](../../../specs/_ops/OPS-002-g088-certifiedat-backfill/) exists to manage and which OPS-002 deliberately refused to resolve by bulk recertification; **do not bulk-recertify here either.** The banner is derived, redundant metadata — `state.json` is the authority. If banner/state consistency is genuinely wanted long-term, the **principled** direction is **(iii) fix G088 upstream** so it distinguishes mandated redaction/reconciliation from planning-truth change (see RTE-M6); the cheaper alternatives are **(i)** stop duplicating status in `spec.md` at all, or **(ii)** make any consistency check *read* `state.json` rather than *rewrite* certified planning files. |
| RTE-M6 — **new 2026-08-18; framework-level structural conflict** | This repo's mandatory **"No Env-Specific Content In This Repo"** policy ([`.github/copilot-instructions.md`](../../../.github/copilot-instructions.md)) and Gate **G088**'s post-certification planning-truth immutability are in **direct structural conflict**. Measured at `HEAD` `57bcb187` by running `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh <specDir>` over every `done` spec with a sibling `spec.md`: **PASS 59 / FAIL 40 of 99** — a **pre-existing portfolio condition**, not something any future sweep would create from nothing. Grouping the guard's reported `subject=` lines across all failures gives **90** post-certification file-touches, and **68 of those 90 (76%) are mandated redaction, not planning drift**: 60 × `refactor(deploy): enforce generic self-hosted boundary`, 7 × `chore(genericize): remove machine-local and deployment-specific values`, 1 × `docs(specs): land remaining spec evidence + BUG-042-007 (env-host refs redacted to placeholders)`. The remainder are evidence/state reconciliation sweeps: 12 × `docs(specs): portfolio evidence + state.json sweep`, 2 × `docs(specs): reconcile stale spec annotations to committed reality`, 2 × `docs(review): address MVP/deploy/ops readiness-review findings`, 2 × `fix(ml): BUG-067-001 ...`, 2 × `feat(056): ...`, 1 × `wip(smackerel): spec-092 ...`. **Structural conclusion:** a repo that obeys its own PII-genericization policy will *necessarily and continuously* accumulate G088 violations, because G088 cannot distinguish a hostname redaction from a requirements change. Every such scrub is policy-compliant and mandatory, yet each one is reported as a certification-integrity defect. | Bubbles framework — route via `bubbles.setup` | This is a **framework-level concern, not a smackerel content defect**, and `post-cert-spec-edit-guard.sh` is a framework-managed install artifact that is not patched downstream. It is recorded and routed here, **not fixed here**. Explicitly **not** resolved by bulk recertification — that would erase the certification signal for all 40 specs to silence a guard that is asking the wrong question. The upstream repair is to let G088 classify a post-certification touch as *mandated redaction / evidence reconciliation* versus *planning-truth change* (for example via a declared redaction-commit convention or a content-diff that ignores placeholder substitution), so policy-compliant scrubs stop being reported as integrity failures. |

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
