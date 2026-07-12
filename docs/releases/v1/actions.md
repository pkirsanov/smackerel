# Actions — Smackerel v1

Action items required to close the v1 gate, grouped by owner. All items are **planning outputs of this packet, not work performed by this packet**. The `route_required` dispatches in the [Next Dispatches](#next-dispatches) section below are the playbook AFTER MVP closes.

## Engineering (route via `bubbles.workflow` dispatches)

| ID | Action | Owner spec slot | Priority | Dependency |
|----|--------|-----------------|----------|------------|
| ENG-V1 | Implement V1-A Gmail SDK connector | new `specs/077` | P0 — v1 keystone | none |
| ENG-V2 | Implement V1-B Microsoft Graph mail | new `specs/078` | P0 | none |
| ENG-V3 | Implement V1-C Google Calendar API | new `specs/079` | P1 | none (CalDAV baseline already serves) |
| ENG-V4 | Implement V1-D Microsoft Graph Calendar | new `specs/080` | P1 | ENG-V2 (shared Graph auth) |
| ENG-V5 | Implement V1-E Apple Calendar bridge | new `specs/081` | P1 | OQ-V1 decision (bridge surface) |
| ENG-V6 | Implement V1-F Reminders/Tasks family | new `specs/082` | P0 | OQ-V2 decision (one spec vs family) |
| ENG-V7 | Implement V1-G Notes family | new `specs/083` | P1 | OQ-V2 decision |
| ENG-V8 | Implement V1-H Messages family | new `specs/084` | P0 | OQ-V3 (consent/permission model — coordinates with V2-A foundation) |
| ENG-V9 | Implement V1-I Voice capture | new `specs/085` | P1 | OQ-V4 (transcription backend) |
| ENG-V10 | Extend V1-J cloud drives / photos | extend `specs/038`, `specs/040` | P2 | none |
| ENG-V11 | **Implement V2-A Outbound Action foundation** | new `specs/086` | **P0 — MUST land before any V2-B** | none |
| ENG-V12 | Implement V2-B per-target outbound actions | per-spec extensions | P0–P2 (per target) | ENG-V11 |
| ENG-V13 | Implement V4-A capability map generator | new `specs/090` | P1 | at least one V1 connector landed |
| ENG-V14 | Implement V5-A SLO alerting for surfacing controller | extend `specs/049` | P0 (operational SLO gate) | none |
| ENG-V15 | Implement V5-B SLO alerting for outbound actions | extend `specs/049` | P0 | ENG-V11 |
| ENG-V16 | Conditional: implement V3-B iOS native (if V3-A chooses native) | new `specs/088` | P0 if chosen | DOC-V1 |
| ENG-V17 | Conditional: implement V3-C Android native (if V3-A chooses native) | new `specs/089` | P0 if chosen | DOC-V1 |

## Docs (route via `bubbles.docs`)

| ID | Action | Target | Priority |
|----|--------|--------|----------|
| DOC-V1 | Author operator decision doc V3-A | new `docs/Mobile_Decision.md` | P0 — gates ENG-V16/V17 |
| DOC-V2 | Update [`docs/INVESTOR_OVERVIEW.md`](../../INVESTOR_OVERVIEW.md) Phase Overview table to add a "v1 Gate (target date TBD)" row | `docs/INVESTOR_OVERVIEW.md` | P1 — packet visibility |
| DOC-V3 | After V4-A lands, route auto-generated capability map into managed docs | `docs/Capability_Map.md` | P1 (depends on ENG-V13) |
| DOC-V4 | Update [`docs/smackerel.md`](../../smackerel.md) §1.5 Non-Goals if v1 changes any prior non-goal (e.g., Outbound Action arrival flips a former non-goal) | `docs/smackerel.md` | P1 |

## Ops (route via `bubbles.upkeep` / `bubbles.devops`)

| ID | Action | Target | Priority |
|----|--------|--------|----------|
| OPS-V1 | Re-run `bubbles.spec-review` at v1 close (V6-A) | portfolio-wide | P1 — gate to v1 declaration |
| OPS-V2 | `bubbles.devops` validates v1 deployment-narrative technical claims (per [`deployment.md`](deployment.md)) | this packet | P0 before external claim |
| OPS-V3 | Update `bubbles.train` flag bundles for each new V-spec flag (default-ON in `next` train, default-OFF in `mvp`) | `config/feature-flags.*.yaml` | per-spec |
| OPS-V4 | **Route release-TRAIN re-targeting for spec 095 (V7)** — recommend `releaseTrain` `mvp` → `next`. `bubbles.plan` owns the change to `specs/095-retrieval-strategy-routing/state.json` `releaseTrain`; `bubbles.train` owns confirming the `next` train is the correct home (no flag-bundle change needed — `flagsIntroduced: []`). Rationale in OQ-V10. `bubbles.releases` does NOT edit either file. | `bubbles.plan` + `bubbles.train` | P1 — before 095 full-delivery dispatch |

## Owner decisions still pending (open questions)

| OQ-ID | Question | Affects v1 item | Suggested default |
|-------|----------|-----------------|-------------------|
| OQ-V1 | Apple Calendar bridge surface — companion macOS helper vs iCloud-CalDAV reuse? | V1-E | iCloud-CalDAV reuse if it works (no extra runtime); else macOS helper |
| OQ-V2 | Reminders/Tasks (V1-F) and Notes (V1-G) — one shared connector with provider modules, or one spec per provider? | V1-F, V1-G | Shared connector per [`bubbles-capability-foundation-design`](../../../.github/skills/bubbles-capability-foundation-design/SKILL.md); spec-creation phase makes the call |
| OQ-V3 | Messages (V1-H) consent/permission model — should it ride entirely on V2-A foundation, or have its own consent model since inbound is read-only? | V1-H, V2-A | Read-side messages use existing read-only model; write-side messages ride V2-A — write-side is a V2-B target |
| OQ-V4 | Voice (V1-I) transcription backend — local Whisper via Ollama vs cloud provider configurable per `LLM_PROVIDER`? | V1-I | Local-first default (Whisper via existing ML sidecar); cloud configurable for users who already use cloud LLMs |
| OQ-V5 | Outbound Action foundation (V2-A) — initial per-target rate limit defaults? Each user-target combo gets a configurable limit, or one global cap? | V2-A | Per-target configurable with conservative defaults; operator may tune per-target via config |
| OQ-V6 | Undo window default duration for V2-A? | V2-A | 60 s default for reversible actions; 0 s for irreversible (deletes); operator-tunable |
| OQ-V7 | Native mobile decision (V3-A) — is operator ready to decide at v1 kickoff, or defer to v1.5? | V3-A | Defer-friendly: V3-A may sit unresolved without blocking V1-A..J / V2 progress. Decision can land late in v1 cycle. |
| OQ-V8 | V4-A capability map output — is it a single managed doc, or a structured artifact (JSON/YAML) consumed by the trio? | V4-A | Both: managed doc for human reading, structured artifact for automation |
| OQ-V9 | Monetization conversation — does v1 close trigger a pricing/tier decision? | [`monetization.md`](monetization.md) | Defer; v1 unlocks the conversation but does NOT commit to commercialization |
| OQ-V10 | **Release-TRAIN targeting for spec 095 (V7)** — `specs/095-retrieval-strategy-routing/state.json` sets `releaseTrain: "mvp"`, but 095 is a NEW post-MVP (RELEASE-V1-phase) spec whose theme ("retrieval-strategy routing + freshness-aware retrieval", synthesis/digest pool exclusion) matches the `next` deployment train's charter verbatim ("Next promotion candidate (synthesis + multi-source coordination)" in [`config/release-trains.yaml`](../../../config/release-trains.yaml)), and OPS-V3 above already states v1-phase specs default-ON in the `next` train / default-OFF in `mvp`. Should 095's `releaseTrain` change `mvp` → `next`? | V7 (spec 095) | **Yes — recommend `next`** (routed to `bubbles.plan` + `bubbles.train` via OPS-V4). `mvp` is defensible only as the single active self-hosted destination train, but `next` is the correct promotion train for a new post-MVP synthesis-adjacent capability. `bubbles.releases` does NOT edit the spec `state.json` or `release-trains.yaml`. |

## Cross-product coordination actions

| ID | Action | Counterparty |
|----|--------|--------------|
| XP-V1 | If V2-A Outbound Action lands, QF Companion may eventually consume a Smackerel outbound surface (e.g., "Smackerel evidences a personal-context fact that QF then renders in a decision packet"). v1 does NOT commit to this; the QF boundary remains read-only-from-Smackerel-side per [`docs/smackerel.md`](../../smackerel.md) §1.6. | QF Companion repo |
| XP-V2 | Per `.github/instructions/product-principles.instructions.md` Principle 10, any cross-product schema change between Smackerel and QF MUST update QF spec 063 FIRST, then Smackerel. This applies if v1 work touches `PersonalEvidenceBundle` shape. | QF Companion repo |

## Items explicitly NOT taken on in this packet

- Do not author any new spec (all V-items emitted as `route_required`).
- Do not edit `docs/smackerel.md` (DOC-V4 dispatched, not executed).
- Do not decide V3-A native-mobile question (DOC-V1 dispatched).
- Do not commit to any monetization model (operator decision per OQ-V9).
- Do not extend QF boundary (XP-V1 explicit non-commitment).

## Next Dispatches

These are **not** to be dispatched until RELEASE-MVP fully closes (all M-items terminal). They are recorded here so v1 planning can begin immediately on MVP close.

```yaml
# V1 — Personal Productivity Sources (each is its own future spec or family)
- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec
  proposed_slot: specs/077-gmail-sdk-connector
  reason: release-planning-v1:gap-A-personal-productivity
  rationale: |
    Rich Gmail integration via Google APIs Go SDK (gmail.googleapis.com), beyond
    existing IMAP baseline. Adds Pub/Sub push, watch API, label-as-graph-edge,
    thread-as-conversation-node.
  evidence: specs/_spec-review-report.md, docs/smackerel.md §19 Phase 2

- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec
  proposed_slot: specs/078-microsoft-graph-mail
  reason: release-planning-v1:gap-A-personal-productivity
  rationale: Outlook/Microsoft Graph mail equivalent of 077.

- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec
  proposed_slot: specs/079-google-calendar-api
  reason: release-planning-v1:gap-A-personal-productivity
  rationale: Native Google Calendar (Google Calendar API) beyond existing CalDAV baseline.

- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec
  proposed_slot: specs/080-microsoft-graph-calendar
  reason: release-planning-v1:gap-A-personal-productivity

- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec
  proposed_slot: specs/081-apple-calendar-eventkit
  reason: release-planning-v1:gap-A-personal-productivity
  rationale: |
    Apple Calendar via EventKit bridge. Operator decision required on bridge
    surface (companion macOS helper vs cloud-side iCloud-CalDAV reuse).

- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec family
  proposed_slot: specs/082-reminders-tasks-connector-family
  reason: release-planning-v1:gap-A-personal-productivity
  rationale: |
    Unified Reminders/Tasks ingestion: Apple Reminders, Google Tasks, MS To Do,
    Todoist, TickTick. Consider one shared connector with provider modules vs
    one spec per provider — design decision at spec-creation time.

- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec family
  proposed_slot: specs/083-notes-connector-family
  reason: release-planning-v1:gap-A-personal-productivity
  rationale: Notion, Obsidian, Apple Notes, OneNote. Same shared-connector decision as 082.

- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec family
  proposed_slot: specs/084-messages-connector-family
  reason: release-planning-v1:gap-A-personal-productivity
  rationale: |
    SMS/iMessage/Signal/Slack. iMessage and Signal require client-side bridge;
    explicit consent/permission model required. May benefit from capability
    foundation per bubbles-capability-foundation-design skill.

- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec
  proposed_slot: specs/085-voice-capture
  reason: release-planning-v1:gap-A-personal-productivity
  rationale: |
    Voice-note ingestion + transcription. Hooks into existing ML sidecar; design
    decision on transcription backend (local Whisper via Ollama vs cloud).

- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: existing spec extension
  proposed_slot: specs/038-cloud-drives-integration (extend) + specs/040-cloud-photo-libraries (extend)
  reason: release-planning-v1:gap-A-personal-productivity
  rationale: Extended providers beyond MVP set.

# V2 — Outbound Action capability
- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec (foundation)
  proposed_slot: specs/086-outbound-action-foundation
  reason: release-planning-v1:gap-B-outbound-action
  rationale: |
    First-class peer of inbound connector contract. Must include: capability
    registry; consent/permission model; audit log; dry-run mode; undo window;
    rate limits per target; failure semantics. This is a capability-foundation
    item per bubbles-capability-foundation-design skill — MUST be designed BEFORE
    individual outbound actions are added.

- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec family (after 086 lands)
  proposed_slot: specs/087+ outbound-action-connectors
  reason: release-planning-v1:gap-B-outbound-action
  rationale: |
    Per-target outbound actions (e.g., send-gmail, create-calendar-event,
    create-reminder, post-slack-message, etc.). Each connector that gains a
    write-side must extend its existing spec with an outbound-action scope.

# V3 — Native mobile decision
- agent: bubbles.workflow
  mode: docs-only
  target: new doc
  proposed_slot: docs/Mobile_Decision.md
  reason: release-planning-v1:rec-8-mobile-decision
  rationale: |
    Operator decision doc evaluating iOS+Android native vs continued PWA-only
    path. Decision factors: capture latency, background ingestion (clipboard,
    share-sheet), notification SLOs, voice capture (V1 V1-V/V5), Apple
    EventKit/Reminders/Notes bridge feasibility (V1 V1-V/V8). If native chosen,
    spawn specs/088-mobile-ios + specs/089-mobile-android (or shared cross-platform
    framework spec).

# V4 — Auto-generated capability map
- agent: bubbles.workflow
  mode: idea-to-release-completion
  target: new spec
  proposed_slot: specs/090-capability-map-generator
  reason: release-planning-v1:rec-9-capability-map
  rationale: |
    Auto-generate a capability map doc from the connector registry
    (internal/connector/registry.go), skills manifest, and capability ledger.
    Replaces manual upkeep of capability-claim docs.

# V5 — Promote SLOs to alerted
- agent: bubbles.workflow
  mode: improve-existing
  target: specs/049-monitoring-stack
  reason: release-planning-v1:rec-3-slo-alerting
  rationale: |
    Promote MVP M1a surfacing-controller SLOs (nudges/day, acted-on-rate,
    false-positive ratio) and any V2 outbound-action SLOs from measured-only to
    paged-alerts. Coordinate with spec 030 observability and bubbles-observability-adapter.

# V6 — Drift cleanup (continuous)
- agent: bubbles.spec-review
  mode: classify
  reason: release-planning-v1:rec-6-drift-cleanup
  rationale: |
    Re-run portfolio drift classification at v1 close. Any remaining
    MAJOR_DRIFT items dispatch via improve-existing as before.

# V7 — Retrieval-strategy routing + freshness-aware retrieval (spec 095, planning hardened 2026-06-17)
# NOTE: 095 is ALREADY authored to specs_hardened (product-to-planning). This is NOT a
# new-spec dispatch — it is the full-delivery run that consumes the existing planning packet.
- agent: bubbles.workflow
  mode: full-delivery
  target: existing spec (consume planning packet)
  spec: specs/095-retrieval-strategy-routing
  reason: release-planning-v1:V7-retrieval-strategy-routing
  rationale: |
    Implement->test->validate->audit->finalize the 9 SCOPE-01..09 scopes / 16 SCN-095-*
    scenarios authored at the specs_hardened ceiling. Ideas: (1) intent-aware
    retrieval-strategy routing, (2) evergreen-vs-ephemeral ingestion signal, (3)
    per-artifact-type retrieval contract. All over the single existing store
    (Principle 5; TestNoParallelStore). flagsIntroduced: []; deliverableFiles: [].
  precondition: |
    Release-TRAIN finding (OQ-V10 / OPS-V4): recommend bubbles.plan change
    specs/095 state.json releaseTrain "mvp" -> "next" (post-MVP synthesis-adjacent
    capability matches the `next` train charter), with bubbles.train confirming the
    `next` train home. bubbles.releases does not edit either file.
```

## Carry-forward integrity

This packet carries forward 100% of MVP capabilities (see [`features.md`](features.md)). No MVP capability is deprecated in v1. The connector roster lock from MVP ends at v1 — V1 expands it explicitly.
