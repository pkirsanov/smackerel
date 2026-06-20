# Features — Smackerel v1

## Carried Forward From Prior Phases (Phase 1–5 + MVP)

All Phase 1–5 delivered capabilities (per [`docs/INVESTOR_OVERVIEW.md`](../../INVESTOR_OVERVIEW.md) Phase Overview) AND all RELEASE-MVP M-items (per [`../mvp/features.md`](../mvp/features.md)) carry forward into v1 unchanged. v1 does NOT deprecate any prior capability.

See [`../mvp/features.md`](../mvp/features.md) for the full carry-forward table and the MVP-locked connector roster. The connector-roster lock from MVP **ends at v1**: v1 explicitly expands the roster per the new-in-v1 table below.

| MVP capability | v1 status |
|----------------|-----------|
| All Phase 1–5 deliverables | carry forward unchanged |
| M1a (global surfacing controller) | carry forward; **extended in V5** to alert-promotion |
| M1b (calendar-triggered briefs) | carry forward unchanged |
| M1c (reminder/promise engine) | carry forward unchanged |
| M2 (wiki/graph-browse) | carry forward unchanged |
| M3 (ratified principles 1–10) | carry forward unchanged (now binding portfolio-wide) |
| M4 (026 drift fix) | carry forward; drift report stays clean per V6 |
| M5a–d (MINOR_DRIFT cleanup) | carry forward; drift report stays clean per V6 |
| Connector roster lock | **LIFTED** — v1 expands roster |

## New In This Phase (v1 — operator decisions 2026-06-03)

### V1 — Personal Productivity Sources (Gap A)

| ID | Capability | Owning spec(s) | New spec required? | Proposed slot |
|----|-----------|----------------|--------------------|--------------:|
| V1-A | Rich Gmail via Google APIs SDK (Pub/Sub push, watch API, labels-as-edges) | new | **Yes** | `specs/077-gmail-sdk-connector` |
| V1-B | Microsoft Graph mail (Outlook/M365) | new | **Yes** | `specs/078-microsoft-graph-mail` |
| V1-C | Google Calendar API (beyond CalDAV) | new | **Yes** | `specs/079-google-calendar-api` |
| V1-D | Microsoft Graph Calendar | new | **Yes** | `specs/080-microsoft-graph-calendar` |
| V1-E | Apple Calendar (EventKit bridge or iCloud-CalDAV reuse) | new | **Yes** (or extension of CalDAV path) | `specs/081-apple-calendar-eventkit` |
| V1-F | Reminders/Tasks family (Apple Reminders, Google Tasks, MS To Do, Todoist, TickTick) | new family | **Yes** | `specs/082-reminders-tasks-connector-family` |
| V1-G | Notes family (Notion, Obsidian, Apple Notes, OneNote) | new family | **Yes** | `specs/083-notes-connector-family` |
| V1-H | Messages family (SMS, iMessage, Signal, Slack) | new family | **Yes** | `specs/084-messages-connector-family` |
| V1-I | Voice capture + transcription | new | **Yes** | `specs/085-voice-capture` |
| V1-J | Extended cloud drives / photos providers (beyond 038/040 MVP set) | adjustments | **No** (extensions) | extend `specs/038-cloud-drives-integration`, `specs/040-cloud-photo-libraries` |

### V2 — Outbound Action capability (Gap B)

| ID | Capability | Owning spec(s) | New spec required? | Proposed slot |
|----|-----------|----------------|--------------------|--------------:|
| V2-A | **Outbound Action foundation** — first-class peer of inbound connector contract: capability registry, consent/permission model, audit log, dry-run, undo window, rate-limit, failure semantics | new (capability foundation) | **Yes** (foundation MUST land first per [`bubbles-capability-foundation-design`](../../../.github/skills/bubbles-capability-foundation-design/SKILL.md)) | `specs/086-outbound-action-foundation` |
| V2-B | Per-target outbound actions (send-gmail, create-calendar-event, create-reminder, post-slack-message, write-note, etc.) | per-source spec extension + V2-A foundation | **Mixed** — V2-A is new spec; per-target actions extend existing connector specs (V1-A..H + MVP roster) | `specs/087+` shared sequence |

### V3 — Native mobile decision

| ID | Capability | Owning spec(s) | New spec required? | Proposed slot |
|----|-----------|----------------|--------------------|--------------:|
| V3-A | **Mobile decision doc** — operator-signed decision between native iOS/Android and continued PWA-only | new decision doc (NOT a spec) | **No** — new doc | `docs/Mobile_Decision.md` |
| V3-B | iOS native app (if V3-A chooses native) | new | **Yes** (conditional) | `specs/088-mobile-ios` |
| V3-C | Android native app (if V3-A chooses native) | new | **Yes** (conditional) | `specs/089-mobile-android` |

### V4 — Auto-generated capability map

| ID | Capability | Owning spec(s) | New spec required? | Proposed slot |
|----|-----------|----------------|--------------------|--------------:|
| V4-A | Capability map generator (reads `internal/connector/registry.go`, skills manifest, capability ledger; emits `docs/Capability_Map.md`) | new | **Yes** | `specs/090-capability-map-generator` |

### V5 — SLO promotion (MVP M1a measured → v1 alerted)

| ID | Capability | Owning spec(s) | New spec required? | Proposed slot |
|----|-----------|----------------|--------------------|--------------:|
| V5-A | Wire MVP M1a surfacing-controller SLOs (nudges/day, acted-on-rate, false-positive ratio) as paged alerts | adjustment | **No** | extend `specs/049-monitoring-stack` (+ tie-in to 021, 030) |
| V5-B | Wire V2-A Outbound Action SLOs (action-success rate, undo-rate, dry-run-vs-real ratio) as paged alerts | adjustment | **No** (rides on V5-A path) | extend `specs/049-monitoring-stack` |

### V6 — Continuous drift cleanup

| ID | Capability | Owning spec(s) | New spec required? | Proposed slot |
|----|-----------|----------------|--------------------|--------------:|
| V6-A | Re-run `bubbles.spec-review` at v1 close; dispatch any remaining MAJOR_DRIFT via `improve-existing` | per-spec | **No** — case-by-case | n/a |

### V7 — Retrieval-strategy routing + freshness-aware retrieval (post-MVP intelligence gap-closers, planning hardened 2026-06-17)

> **Status: PLANNED / specced — NOT delivered.** Owning spec [`specs/095-retrieval-strategy-routing`](../../../specs/095-retrieval-strategy-routing/) is hardened to the `specs_hardened` ceiling (`product-to-planning` mode): `spec.md` + `design.md` + `scopes.md` (9 scopes) + `scenario-manifest.json` (16 `SCN-095-*` scenarios) authored, `planningOnly: true`, `flagsIntroduced: []`, `deliverableFiles: []`. Zero source delivered — `internal/retrieval/` does not exist yet. This row traces to a **real owning spec at `specs_hardened`**, NOT to certified code. Delivery is a separate later-stage full-delivery run (see [Plan-to-Release Traceability](#plan-to-release-traceability)).
>
> This V7 group is a **2026-06-17 post-MVP addition**, distinct from the original 2026-06-03 operator-decision V1–V6 set above. Per [`../mvp/features.md`](../mvp/features.md) ("Any new connector after this MVP gate is RELEASE-V1 scope"; "No new spec is required for MVP"), the MVP phase is frozen for new specs, so spec 095 — a NEW spec — is RELEASE-V1-phase scope.

<!-- bubbles:feature id=retrieval-strategy-routing spec=specs/095-retrieval-strategy-routing delivery=optional -->
<!-- machine-binding note (Gate G101 / release-delivery-reconciliation-guard.sh): delivery=optional is deliberate, NOT required. Spec 095 is planning-only at the specs_hardened ceiling and is not yet validate-certified/delivered. The guard requires every delivery=required feature to bind a TERMINAL + validate-certified spec; 095 stays optional (NOT-ENFORCED) until its full-delivery run reaches done, at which point the v1 finalize refresh flips it to required. The packet-level reconciliation header is intentionally NOT added yet because the V1-V6 rows above still bind to not-yet-created proposed spec slots; full-packet machine reconciliation is a future bubbles.releases backfill. -->

| ID | Capability | Owning spec | New spec required? | Status |
|----|-----------|-------------|--------------------|--------|
| V7-A | **Idea 1 — Retrieval-strategy routing by query intent**: `whole_document` + `structured_aggregate` + `vague_recall` default + low-confidence fallback, selected per query intent over the single existing store | [`specs/095-retrieval-strategy-routing`](../../../specs/095-retrieval-strategy-routing/) | No — authored (`specs_hardened`) | PLANNED |
| V7-B | **Idea 2 — Evergreen-vs-ephemeral classification at the ingestion front door**: freshness signal emitted at `AssignTier`, synthesis/digest pool exclusion + aggressive decay for ephemeral artifacts (which stay searchable) | (same spec 095) | No — authored (`specs_hardened`) | PLANNED |
| V7-C | **Idea 3 — Per-artifact-type retrieval contract**: in-code registry of per-artifact-type query shapes that drives Idea 1's router | (same spec 095) | No — authored (`specs_hardened`) | PLANNED |

**Idea → scope mapping** (owning spec `specs/095-retrieval-strategy-routing`; 9 scopes / 16 `SCN-095-*` scenarios):

| Idea | Scopes | Surface (planned) |
|------|--------|-------------------|
| Shared foundation | SCOPE-01 (fail-loud SST `retrieval.*` config) | `config/smackerel.yaml`, `internal/config/retrieval.go` |
| Idea 3 — per-artifact-type retrieval contract | SCOPE-02 (RetrievalContract registry) | `internal/retrieval/routing/contract.go` |
| Idea 1 — retrieval-strategy routing | SCOPE-03 (router + `StrategySelection` trace), SCOPE-04 (`whole_document`), SCOPE-05 (`structured_aggregate`), SCOPE-06 (`vague_recall` default + low-confidence fallback + facade integration) | `internal/retrieval/routing/` |
| Idea 2 — evergreen-vs-ephemeral | SCOPE-07 (evergreen signal at `AssignTier`), SCOPE-08 (synthesis/digest pool exclusion + aggressive decay) | `internal/retrieval/evergreen/` |
| Docs | SCOPE-09 (docs-only) | `docs/smackerel.md` §9, `docs/Operations.md` |

**Single-graph invariant (Principle 5 — "One Graph, Many Views"):** all V7 routing + evergreen behavior operates over the ONE existing pgvector + knowledge-graph + structured store — no parallel index / store / graph (enforced by the planned `TestNoParallelStore` architecture test). Recorded so the v1 packet's One-Graph principle is not silently violated by a new retrieval surface.

## Plan-to-Release Traceability

| v1 item | Target dispatch | Dispatch mode |
|---------|-----------------|---------------|
| V1-A | new `specs/077-gmail-sdk-connector` | `idea-to-release-completion` |
| V1-B | new `specs/078-microsoft-graph-mail` | `idea-to-release-completion` |
| V1-C | new `specs/079-google-calendar-api` | `idea-to-release-completion` |
| V1-D | new `specs/080-microsoft-graph-calendar` | `idea-to-release-completion` |
| V1-E | new `specs/081-apple-calendar-eventkit` | `idea-to-release-completion` |
| V1-F | new `specs/082-reminders-tasks-connector-family` | `idea-to-release-completion` |
| V1-G | new `specs/083-notes-connector-family` | `idea-to-release-completion` |
| V1-H | new `specs/084-messages-connector-family` | `idea-to-release-completion` |
| V1-I | new `specs/085-voice-capture` | `idea-to-release-completion` |
| V1-J | extend `specs/038`, `specs/040` | `improve-existing` |
| V2-A | new `specs/086-outbound-action-foundation` | `idea-to-release-completion` (FOUNDATION — must land first) |
| V2-B | per-connector spec extensions | `improve-existing` (depends on V2-A) |
| V3-A | new `docs/Mobile_Decision.md` | `bubbles.docs` docs-only |
| V3-B/C | conditional new specs | `idea-to-release-completion` (gated on V3-A) |
| V4-A | new `specs/090-capability-map-generator` | `idea-to-release-completion` |
| V5-A | extend `specs/049-monitoring-stack` | `improve-existing` |
| V5-B | extend `specs/049-monitoring-stack` | `improve-existing` |
| V6-A | continuous | `bubbles.spec-review classify` |
| V7-A/B/C (one cohesive spec) | owning `specs/095-retrieval-strategy-routing` (already at `specs_hardened`) | **full-delivery** run (implement→test→validate→audit→finalize) consuming the `product-to-planning` packet |

## Capability evidence trace

Every "new in v1" capability traces to a row in the deep review (Gap A, Gap B, recs 3/8/9) and to a proposed spec slot. No capability is claimed as "delivered in v1" — they are all `planned`. The v1 packet refresh (per `idea-to-release-completion` mode `finalReleasesPhasePosition: -2`) will flip capabilities to `delivered` only when their underlying specs reach terminal-for-mode AND audit certifies the spec as `done`.

**V7 (spec 095)** is the first v1 feature that traces to a **real, already-authored owning spec** (`specs/095-retrieval-strategy-routing` at `specs_hardened`) rather than to a not-yet-created proposed slot — its capability claim is evidence-backed by the hardened planning artifacts (spec/design/scopes/16 `SCN-095-*` scenarios), but it remains `planned` (NOT `delivered`): no source exists yet (`internal/retrieval/` absent) and delivery is a separate full-delivery run. It is machine-bound for Gate G101 with `delivery=optional` (not-yet-enforced) per the feature-binding annotation in the V7 section above.

> **Non-blocking finding (future spec-review, NOT fixed here):** the V1–V6 "Proposed slot" numbers above (`specs/077`–`specs/090`) now collide with already-created specs at those numbers (e.g. `specs/077-pwa-browser-test-harness`, `specs/078-cross-surface-surfacing-prioritizer`, `specs/086-local-client-build`, `specs/090-observability-slo-dogfood` all exist with unrelated content). The proposed slots are stale and must be re-numbered to the next free spec numbers when each V-item is actually dispatched. This is out of scope for this 2026-06-17 V7 addition and is surfaced for a future `bubbles.spec-review` / `bubbles.releases` refresh.

## Sequencing / dependencies

- **V2-A foundation MUST land BEFORE V2-B per-target actions.** This is a capability-foundation pattern per [`bubbles-capability-foundation-design`](../../../.github/skills/bubbles-capability-foundation-design/SKILL.md).
- **V3-A decision MUST land BEFORE V3-B/V3-C.** No native spec begins until operator signs the decision doc.
- **V5-B SLO-alert wiring DEPENDS on V2-A landing** (Outbound Action SLOs do not exist until the foundation exposes the metrics).
- **V1-A..I are independent of each other** and may dispatch in parallel as operator capacity allows.
- **V4-A capability map generator should land AFTER at least one V1 connector lands**, so the generator has fresh data to consume.

## Deprecations in v1

None planned. The MVP connector-roster lock LIFTS at v1 (the roster is expanded, not deprecated). If any MVP-roster connector is superseded by a v1 connector (e.g., V1-A Gmail SDK supersedes the IMAP path for Gmail users), the superseded path remains supported as a fallback — explicit deprecation requires its own spec dispatch.
