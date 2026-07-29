# Bug Spec: BUG-019-003 — `artifacts.source_ref` MUST be persisted by the connector ingestion front door

> **Bug:** [bug.md](bug.md) | **Design:** [design.md](design.md) | **Scopes:** [scopes.md](scopes.md) | **Report:** [report.md](report.md) | **Validation:** [uservalidation.md](uservalidation.md)
> **Parent:** [019 spec](../../spec.md) | [019 scopes](../../scopes.md)

---

## Classification

- **Type:** Data-integrity defect in shared ingestion infrastructure (persistence + dedup correctness)
- **Severity:** HIGH — major feature broken, no workaround (argued in [bug.md → Severity Argument](bug.md#severity-argument))
- **Parent Spec:** 019 — Connector Wiring
- **Ownership:** `specs/019-connector-wiring` (shared front door, not any single connector)
- **Authoring Workflow Mode:** `product-to-planning` (statusCeiling `specs_hardened`, Gate G073 ACTIVE)
- **Planned Fix Workflow Mode:** `bugfix-fastlane`
- **Release Train:** `mvp`
- **Status:** Specs hardened — planning artifacts complete, fix not started

## Problem Statement

The shared connector ingestion front door `internal/pipeline/ingest.go::PublishRawArtifact` omits `source_ref` from its `INSERT INTO artifacts` column list (`:94`) and probes for duplicates against the wrong column (`:51` — `WHERE source_url = $1` bound to `artifact.SourceRef`). As a result:

- `artifacts.source_ref` is `NULL` for all 17 registered connectors.
- Source-ref deduplication is structurally unreachable; ingestion silently degrades to `ON CONFLICT (content_hash)`.
- `idx_artifacts_source (source_id, source_ref)` is a dead index carrying write cost and no read value.
- The GuestHost booking-context API (`internal/api/context.go:284`) and both bookmark dedup paths (`internal/connector/bookmarks/dedup.go:131,185`) match zero rows unconditionally.

## Expected Behavior

### EB-1 — Provenance persistence is universal

Every artifact written through `PublishRawArtifact` MUST persist the connector-supplied `RawArtifact.SourceRef` into `artifacts.source_ref`. When a connector supplies an empty `SourceRef`, the column MUST be written as SQL `NULL` (not the empty string), so that "no external identifier available" and "external identifier is the empty string" remain distinguishable and so that unique/partial-index strategies remain available.

`source_url` MUST continue to receive `RawArtifact.URL`. `source_ref` and `source_url` are distinct fields with distinct meanings and MUST NOT be conflated in either direction.

### EB-2 — Deduplication compares like with like, scoped by source

The ingestion dedup probe MUST compare `artifacts.source_ref` against the incoming `RawArtifact.SourceRef`, **scoped by `source_id`**. Two different connectors MAY legitimately emit the same opaque ref string (for example a bare numeric ID); those MUST NOT be treated as the same artifact.

The probe MUST NOT additionally require `content_hash` equality. Requiring both defeats the purpose: a source-ref probe exists precisely to recognise *the same external entity whose content has changed*. Content-hash equality is already handled independently by the `ON CONFLICT (content_hash)` clause.

### EB-3 — The composite index becomes live

`idx_artifacts_source ON artifacts(source_id, source_ref)` MUST become the access path for the `(source_id, source_ref)` dedup probe and for consumer lookups, rather than remaining an index on a universally-NULL second column.

### EB-4 — Consumers resolve

- `internal/api/context.go:284` (GuestHost booking context) MUST be able to resolve a booking artifact by the identifier its API contract advertises. **That identifier is the booking entity ID**, per the resolved D3 decision (see [Resolved Decision](#resolved-decision--d3-guesthost-booking-identity-semantics)); EB-4 is satisfied once D3 Option 4 is implemented.
- `internal/connector/bookmarks/dedup.go` batch and single-URL paths MUST correctly recognise a previously-ingested normalized URL, so that a re-synced bookmark whose title or content changed is **not** inserted as a duplicate row.

### EB-5 — Re-ingestion is not over-deduplicated

The fix restores a dedup path that has never been active in production. It MUST NOT suppress legitimate re-ingestion. Specifically, an artifact matching an existing `(source_id, source_ref)` pair MUST be recognised as *the same external entity* — the system MUST NOT silently drop a genuinely updated version in a way that leaves stale content permanently unreachable. The chosen behaviour (skip vs. update-in-place) is specified in [design.md → Re-ingestion Semantics](design.md#re-ingestion-semantics-and-the-over-suppression-risk).

### EB-6 — Historical rows are addressed honestly

Rows written before the fix have `source_ref IS NULL`. The system MUST NOT silently behave as though those rows carry provenance. Backfill MUST be attempted where the ref is genuinely reconstructable, MUST be skipped where it is not, and the residual non-reconstructable population MUST be measurable and documented. See [design.md → Backfill / Migration Strategy](design.md#backfill--migration-strategy).

## Acceptance Criteria

- [ ] `AC-01` — `artifacts.source_ref` is non-NULL for every newly-ingested artifact whose connector supplied a non-empty `SourceRef`, across all 17 registered connectors.
- [ ] `AC-02` — An empty connector-supplied `SourceRef` persists as SQL `NULL`, not `''`.
- [ ] `AC-03` — The ingestion dedup probe filters on `source_ref` scoped by `source_id`, and does not require `content_hash` equality.
- [ ] `AC-04` — Two artifacts with the same `source_ref` but different `source_id` are both retained (no cross-connector false-positive dedup).
- [ ] `AC-05` — A re-ingested item with an unchanged `source_ref` and changed content does not create a second row for the same `(source_id, source_ref)` pair.
- [ ] `AC-06` — `internal/connector/bookmarks/dedup.go` batch dedup recognises previously-ingested normalized URLs against real persisted data.
- [ ] `AC-07` — The GuestHost booking-context lookup resolves for the identifier its contract advertises, per the resolved D3 decision.
- [ ] `AC-08` — `internal/pipeline/processor.go:523` either persists `source_ref` or is documented as intentionally not doing so, with the reason recorded.
- [ ] `AC-09` — The backfill strategy is executed; the reconstructable population is backfilled and the non-reconstructable residual is counted and documented.
- [ ] `AC-10` — An adversarial regression test exists that FAILS if `source_ref` is again dropped from the ingestion `INSERT`.
- [ ] `AC-11` — Documentation, release, and capability surfaces are reconciled with the corrected provenance behaviour (exact paths enumerated in [scopes.md](scopes.md)).

## Gherkin Scenarios

```gherkin
Feature: BUG-019-003 — connector artifact source_ref persistence and identity-based deduplication

  Scenario: SCN-BUG-019-003-001 Connector-supplied source_ref is persisted for every connector
    Given an ephemeral test database with an empty artifacts table
    And a connector produces a RawArtifact whose SourceRef is a non-empty stable external identifier
    And that RawArtifact carries a URL distinct from its SourceRef
    When PublishRawArtifact ingests the artifact through the shared front door
    Then the persisted artifacts row has source_ref equal to the connector-supplied SourceRef
    And the persisted artifacts row has source_url equal to the connector-supplied URL
    And source_ref and source_url are not conflated in either direction

  Scenario: SCN-BUG-019-003-002 An absent source_ref persists as SQL NULL rather than empty string
    Given an ephemeral test database with an empty artifacts table
    And a connector produces a RawArtifact whose SourceRef is the empty string
    When PublishRawArtifact ingests the artifact
    Then the persisted artifacts row has source_ref IS NULL
    And the persisted row does not have source_ref equal to the empty string

  Scenario: SCN-BUG-019-003-003 Re-ingesting the same external entity with changed content deduplicates by source_ref
    Given an artifact has already been ingested with source_id "bookmarks" and source_ref "https://example.test/a"
    And the upstream item's title and body have since changed so its content_hash differs from the stored row
    When PublishRawArtifact ingests the changed item with the same source_id and the same source_ref
    Then the dedup probe matches the existing row by source_ref scoped to source_id
    And no second artifacts row exists for that source_id and source_ref pair
    And the dedup decision does not depend on content_hash equality

  Scenario: SCN-BUG-019-003-004 Identical source_ref values from different connectors are not falsely deduplicated
    Given an artifact has already been ingested with source_id "discord" and source_ref "12345"
    And a different connector produces a RawArtifact with source_id "youtube" and the identical source_ref "12345"
    And the two artifacts have different content and therefore different content_hash values
    When PublishRawArtifact ingests the second artifact
    Then both artifacts rows are retained
    And the row for source_id "discord" is not overwritten or suppressed by the row for source_id "youtube"

  Scenario: SCN-BUG-019-003-005 GuestHost booking context resolves by the identifier its API contract advertises
    Given the GuestHost connector has ingested a booking artifact through the shared front door
    And the D3 identity-semantics decision has been resolved and implemented
    When a caller requests booking context using the identifier the API contract advertises
    Then the booking artifact is found rather than returning a no-rows error
    And the returned context carries the booking's guest, property, and stay-date fields

  Scenario: SCN-BUG-019-003-006 Bookmark re-sync is idempotent when the bookmark title changes
    Given the bookmarks connector has already ingested a bookmark whose normalized URL is stored as its source_ref
    And the same bookmark is re-exported upstream with a changed title so its content_hash differs
    When the bookmarks connector runs a second sync containing that bookmark
    Then the batch dedup query recognises the normalized URL as already known
    And the bookmark is filtered out of the new-only result set
    And exactly one artifacts row exists for that bookmark's normalized URL

  Scenario: SCN-BUG-019-003-007 Pre-existing NULL source_ref rows are backfilled where reconstructable and counted where not
    Given the artifacts table contains rows ingested before the fix whose source_ref is NULL
    And some of those rows belong to connectors whose source_ref is deterministically reconstructable from persisted columns
    And other rows belong to connectors whose source_ref is not reconstructable from any persisted column
    When the backfill migration runs
    Then the reconstructable rows have source_ref populated with the reconstructed identifier
    And the non-reconstructable rows retain source_ref IS NULL rather than receiving a fabricated value
    And the count of remaining non-reconstructable rows is reported so the residual is measurable

  Scenario: SCN-BUG-019-003-008 The composite source index becomes a live access path
    Given source_ref is persisted for connector artifacts
    When a dedup or consumer lookup filters artifacts by source_id together with source_ref
    Then the query is served by the idx_artifacts_source composite index
    And the index's second key column is no longer universally NULL for connector traffic
```

## Non-Goals

- Changing `source_url` semantics, or removing the `ON CONFLICT (content_hash)` safety net. The content-hash path stays as a second, independent layer.
- Re-opening, re-certifying, or modifying `specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/`. That packet is foreign and its own fix remains independently valid; only a routing note is raised.
- Changing `location_clusters.source_ref` or the `photos` table's `source_ref` handling. Both are separate tables with correct, independent writers (see [bug.md → `F-BUG-019-003-005`](bug.md#f-bug-019-003-005--scoping-correction-recorded-to-prevent-wasted-fix-effort)).
- Introducing a `UNIQUE (source_id, source_ref)` constraint as part of this fix. That is a follow-on hardening step which cannot be applied while a non-reconstructable NULL residual exists; it is recorded as a deferred consideration in [design.md](design.md#deferred-follow-on-unique-source_id-source_ref).

## Resolved Decision — D3 GuestHost Booking Identity Semantics

`internal/connector/guesthost/normalizer.go:27` sets `sourceRef := event.ID` (the **activity-event** ID), while the external business entity is `ActivityEvent.EntityID` (`internal/connector/guesthost/types.go:10`). `internal/api/context.go:284` looks up by a **booking** ID.

Even with EB-1 and EB-2 delivered, a caller passing a booking ID will not match an artifact keyed by event ID. Three events (`booking.created`, `booking.updated`, `booking.cancelled`) map to one booking, so the relationship is many-to-one and cannot be resolved by renaming a field.

**This decision is RESOLVED as Option 4 and is RATIFIED.** `artifacts.source_ref` carries `event.EntityID` for booking event types, `event.ID` is retained in `metadata`, and the decision is coupled to Re-ingestion **Option B** (update-in-place) — Option 4 without Option B is prohibited, because the three lifecycle events collide on one ref by design and skip-on-match would freeze every booking in its created state. The full options table and justification are recorded in [design.md → D3 Identity-Semantics Decision (DECIDED — Option 4)](design.md#d3-identity-semantics-decision-decided--option-4).

**Why Option 4 is the long-term-correct choice.** The consumer contract at `internal/api/context.go:284` is expressed in **business-entity** terms — it is handed a booking ID by its caller. Event IDs are transport artifacts; entity IDs are domain identity. Because three events (`booking.created` / `booking.updated` / `booking.cancelled`) collapse to one booking, the many-to-one relation only resolves when the indexed provenance column `idx_artifacts_source (source_id, source_ref)` is keyed on the **entity**. Keying on `event.ID` would instead require every future consumer to carry an event→entity join that the index cannot serve.

**Ratifier:** Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts. Recorded by `bubbles.bug` on behalf of the operator. Tracked as `TR-BUG-019-003-001` in `state.json` (now `resolved`).

`AC-07` and `SCN-BUG-019-003-005` are consequently **no longer decision-blocked**; like every other criterion and scenario in this planning packet they remain unexecuted until a delivery-mode run implements Scope 4.

## Traceability

| Scenario ID | Acceptance Criteria | Finding |
|---|---|---|
| `SCN-BUG-019-003-001` | AC-01 | `F-BUG-019-003-001` |
| `SCN-BUG-019-003-002` | AC-02 | `F-BUG-019-003-001` |
| `SCN-BUG-019-003-003` | AC-03, AC-05 | `F-BUG-019-003-002` |
| `SCN-BUG-019-003-004` | AC-04 | `F-BUG-019-003-002` |
| `SCN-BUG-019-003-005` | AC-07 | `F-BUG-019-003-003` (D3 decided — Option 4, ratified) |
| `SCN-BUG-019-003-006` | AC-06 | `F-BUG-019-003-001`, `F-BUG-019-003-002` |
| `SCN-BUG-019-003-007` | AC-09 | `F-BUG-019-003-001` |
| `SCN-BUG-019-003-008` | AC-03 | `F-BUG-019-003-001` |
| (adversarial regression) | AC-10 | `F-BUG-019-003-001` |
| (docs propagation) | AC-11 | `F-BUG-019-003-004` |
