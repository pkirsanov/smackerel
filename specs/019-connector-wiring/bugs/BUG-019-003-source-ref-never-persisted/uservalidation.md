# User Validation: BUG-019-003 — `artifacts.source_ref` never persisted

> Bug: [bug.md](bug.md) | Spec: [spec.md](spec.md) | Design: [design.md](design.md) | Scopes: [scopes.md](scopes.md) | Report: [report.md](report.md)

---

## Scope Of This Checklist

This packet was produced under workflow mode `product-to-planning` (statusCeiling `specs_hardened`, Gate G073 ACTIVE). The checklist below validates **the planning deliverable** — the artifacts, the verified defect claims, and the boundary discipline — because that is what this mode actually produced.

It does **not** validate the fix. The fix's acceptance criteria are `AC-01` through `AC-11` in [spec.md](spec.md#acceptance-criteria) and remain unchecked; the Definition-of-Done items across the six scopes in [scopes.md](scopes.md) likewise remain unchecked. Checking any of those here would assert work that has not happened.

## Checklist

- [x] **PV-01:** The defect is real and confirmed by direct source inspection, not inferred. `internal/pipeline/ingest.go:94` omits `source_ref` from the `INSERT INTO artifacts` column list, and `internal/pipeline/ingest.go:51` filters `source_url` while binding `artifact.SourceRef`.
  - **Verified by:** read-only inspection at both coordinates; per-claim table in [bug.md → Verification Evidence](bug.md#verification-evidence-static-planning-mode).
- [x] **PV-02:** The blast radius is established as all 17 registered connectors, not a subset, because every connector's items pass through the one shared publisher.
  - **Verified by:** the 17-constructor `[]connector.Connector` literal at `cmd/core/connectors.go:73-79`, and the single publish loop at `internal/connector/supervisor.go:362`. Corroborated by `docs/smackerel.md:2945` ("Committed Connector Inventory (17 connectors)").
- [x] **PV-03:** The only writers of `artifacts.source_ref` were enumerated exhaustively, confirming the column is NULL for all front-door traffic.
  - **Verified by:** repo-wide `source_ref` scan across `internal/` and `cmd/`, yielding only `internal/connector/photos/store.go:124,197,363` and `internal/drive/scan/service.go:316` — both bespoke writers that bypass `PublishRawArtifact`.
- [x] **PV-04:** The dead-index consequence is grounded in the schema, not asserted. The column is declared and the composite index is built over it.
  - **Verified by:** `internal/db/migrations/001_initial_schema.sql:29` (`source_ref TEXT,`) and `:68` (`idx_artifacts_source ON artifacts(source_id, source_ref)`).
- [x] **PV-05:** Each claimed broken consumer was checked individually rather than assumed from the shared root cause.
  - **Verified by:** `internal/api/context.go:284`, `internal/connector/bookmarks/dedup.go:131-133` and `:184-189` confirmed broken; `internal/knowledge/sensitivity_query.go:116` confirmed to degrade safely via `COALESCE`.
- [x] **PV-06:** A scoping correction was recorded rather than repeating an unverified premise. The maps and photos `source_ref` sites read different tables and are unaffected.
  - **Verified by:** `internal/db/migrations/001_initial_schema.sql:322-323` shows `location_clusters` is a separate table with `source_ref TEXT NOT NULL`, correctly written by `internal/connector/maps/connector.go:475`. Recorded as `F-BUG-019-003-005` and as a Non-Goal in [spec.md](spec.md#non-goals).
- [x] **PV-07:** The D3 GuestHost identity-semantics question is decided explicitly — with its options, trade-offs, and justification recorded — and routed for ratification rather than being silently assumed.
  - **Verified by:** [design.md → D3 Identity-Semantics Decision (DECIDED — Option 4)](design.md#d3-identity-semantics-decision-decided--option-4) lists four options with trade-offs and adopts Option 4 coupled to Re-ingestion Option B; Scope 4 in [scopes.md](scopes.md) implements it; ratification was routed as `TR-BUG-019-003-001` and is now recorded under `OR-01` below.
- [x] **PV-08:** The `BUG-016-W3` false-premise corroboration is recorded with its source quotation, and that foreign packet was **not** reopened or modified.
  - **Verified by:** `specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/bug.md:78` quoted in `F-BUG-019-003-004`; a note-only routing candidate is raised in [bug.md → Routing](bug.md#routing) with no edit to that packet.
- [x] **PV-09:** The backfill strategy states honestly what is and is not recoverable, and forbids fabricating a `source_ref` where none can be derived.
  - **Verified by:** [design.md → Backfill / Migration Strategy](design.md#backfill--migration-strategy) classifies every connector, proves `weather` (`weather.go:150,273-275`) and `google-maps-timeline` (`maps/normalizer.go:132-140`) are non-reconstructable, and requires the residual NULL count to be reported.
- [x] **PV-10:** The over-suppression risk introduced by activating a never-active dedup path is addressed rather than assumed away.
  - **Verified by:** [design.md → Re-ingestion Semantics](design.md#re-ingestion-semantics-and-the-over-suppression-risk) requires an explicit A/B/C decision and notes that inheriting today's skip-on-match branch would ship a new staleness defect.
- [x] **PV-11:** The regression contract is adversarial, not tautological. The required fixture fails if `source_ref` is again dropped from the `INSERT`.
  - **Verified by:** [scopes.md → Scope 1 Adversarial Regression Requirement](scopes.md#adversarial-regression-requirement-blocking) mandates two artifacts with identical `source_id` + `source_ref` but differing `content_hash`, so the `ON CONFLICT (content_hash)` clause cannot mask the missing path; plus an inverse fixture that fails if the probe is fixed without `source_id` scoping.
- [x] **PV-12:** Documentation, release, and capability propagation are recorded as unchecked DoD items with exact file paths and verified current state, not performed in planning mode.
  - **Verified by:** [scopes.md → Scope 6 Propagation Targets](scopes.md#propagation-targets-verified-to-exist-at-packet-authoring-time) cites `docs/smackerel.md:2490` and `:2945`, `docs/API.md:35,170,274`, `docs/Architecture.md:34,44`, `docs/releases/mvp/features.md:104,196`, and `.github/bubbles/capability-ledger.yaml` (recorded as a re-verification item because it is framework-owned and carries no product provenance capability).
- [x] **PV-13:** Every Test Plan row across the six scopes has a corresponding Definition-of-Done checkbox, and all DoD items are unchecked.
  - **Verified by:** [scopes.md](scopes.md) — Scopes 1-6 each pair their Test Plan table with a DoD list; the two mechanically-required regression items (`Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior` and `Broader E2E regression suite passes`) appear in Scopes 2-6.
- [x] **PV-14:** The planning-mode boundary held. Zero source, migration, config, test, or documentation files were edited and zero tests were executed.
  - **Verified by:** [bug.md → Planning-Mode Boundary Statement](bug.md#planning-mode-boundary-statement) and [report.md → Test Evidence](report.md#test-evidence), which records the absence of test execution honestly rather than presenting a placeholder result.

### Owner Ratification — 2026-07-29

The three routing items this packet raised (`TR-BUG-019-003-001`, `-002`, `-003`) were put to the repository owner, who delegated the calls with a stated decision criterion rather than selecting each option individually. The delegation and its criterion are recorded verbatim below; no claim is made that the owner hand-picked each option, and no product authority is claimed by any agent.

> **Attribution for all three items:** Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts. Recorded by `bubbles.bug` on behalf of the operator.

- [x] **OR-01 (`TR-BUG-019-003-001`) — D3 GuestHost booking identity semantics: RATIFIED AS OPTION 4.** `artifacts.source_ref` carries `event.EntityID` for booking event types, `event.ID` is retained in `metadata`, coupled to Re-ingestion **Option B** (update-in-place).
  - **Long-term rationale:** the consumer contract at `internal/api/context.go:284` is expressed in **business-entity** terms — it looks up by booking ID. Event IDs are transport artifacts; entity IDs are domain identity. Three events (`booking.created` / `booking.updated` / `booking.cancelled`) collapse to one booking, so the many-to-one relation only resolves when the indexed provenance column is keyed on the entity. Keying on `event.ID` would require every future consumer to carry an event→entity join that `idx_artifacts_source (source_id, source_ref)` cannot serve.
  - **Ratifier:** Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts. Recorded by `bubbles.bug` on behalf of the operator.
  - **Recorded in:** [spec.md → Resolved Decision](spec.md#resolved-decision--d3-guesthost-booking-identity-semantics), [design.md](design.md#d3-identity-semantics-decision-decided--option-4), `state.json` finding `F-BUG-019-003-003` and `TR-BUG-019-003-001` (`status: resolved`).
- [x] **OR-02 (`TR-BUG-019-003-002`) — weather-connector premise correction: ACCEPTED AS ADVISORY.** After this bug's fix lands, a premise-correction annotation is to be appended to `specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/`. **No re-certification** of that packet is implied or authorised.
  - **Constraint honoured in this run:** nothing under `specs/016-weather-connector/` was read for modification, opened, or edited. This is recorded as a **scheduled follow-up only**, contingent on the BUG-019-003 fix landing first.
  - **Ratifier:** Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts. Recorded by `bubbles.bug` on behalf of the operator.
  - **Recorded in:** `state.json` `TR-BUG-019-003-002` (`status: resolved`, `resolution: accepted`).
- [x] **OR-03 (`TR-BUG-019-003-003`) — mode escalation to `bugfix-fastlane`: APPROVED.** Delivery may proceed under `bugfix-fastlane`, **starting at Scope 1** so the adversarial regression fixture is observed FAILING before any source change.
  - **What this approval does NOT do:** it does not promote this packet. `status` and `certification.status` remain `specs_hardened`; this packet stays planning-only until a delivery-mode run actually executes. Approval schedules the delivery run; it does not substitute for it.
  - **Ratifier:** Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts. Recorded by `bubbles.bug` on behalf of the operator.
  - **Recorded in:** `state.json` `TR-BUG-019-003-003` (`status: resolved`, `resolution: approved`).

## Acceptance

This is a planning-mode bug packet. Acceptance here means the **defect is documented truthfully, its blast radius is grounded in verified evidence, its open decisions are surfaced rather than assumed, and the fix is fully specified without being started**.

The packet is not accepted as a fix and makes no claim to be one. Both design decisions it raised are now settled: the D3 GuestHost booking identity semantics (Option 4, owner-ratified 2026-07-29 — see `OR-01`) and the re-ingestion matched-row semantics (Option B, decision of record in [design.md](design.md#re-ingestion-semantics-and-the-over-suppression-risk), and a hard prerequisite of Option 4). What remains is **execution**, not decision: the fixer must observe and assert the matched-row behaviour in `report.md` and in `SCN-BUG-019-003-003` during a `bugfix-fastlane` delivery run.

**Recorded by:** `bubbles.bug` under workflow mode `product-to-planning`, status ceiling `specs_hardened`.
