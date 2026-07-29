# Scopes: BUG-019-003 — `artifacts.source_ref` never persisted

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

> **Planning-mode notice.** This scope plan was authored under workflow mode `product-to-planning` (statusCeiling `specs_hardened`, Gate G073 Source Code Edit Lockout ACTIVE). **Every DoD item below is unchecked and every scope is Not Started by design.** No source file was edited and no test was executed while authoring this plan. The fixer executes these scopes under `bugfix-fastlane`.
>
> **Scope ordering is strictly sequential.** Scope 1 must produce a *failing* test before Scope 2 changes any source (scenario-first TDD). Scope 5 must not run before Scope 4's gate is resolved for the `guesthost` population.

---

## Scope 1 — Reproduce the defect and lock it with a failing adversarial regression

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BUG-019-003-001 Connector-supplied source_ref is persisted for every connector
  Given an ephemeral test database with an empty artifacts table
  And a connector produces a RawArtifact whose SourceRef is a non-empty stable external identifier
  And that RawArtifact carries a URL distinct from its SourceRef
  When PublishRawArtifact ingests the artifact through the shared front door
  Then the persisted artifacts row has source_ref equal to the connector-supplied SourceRef
  And the persisted artifacts row has source_url equal to the connector-supplied URL
  And source_ref and source_url are not conflated in either direction
```

### Implementation Plan

1. Execute the runtime reproduction sequence in [bug.md → Reproduction Plan](bug.md#reproduction-plan-not-executed-in-planning-mode) steps 1-6 against an ephemeral test stack and capture verbatim output into `report.md`.
2. Author the **adversarial** regression fixture described in the Test Plan below, against the real ephemeral test database, and run it **before touching any source file**.
3. Confirm the new assertions FAIL. A regression test that passes pre-fix is not testing this bug and must be rewritten before Scope 2 begins.

### Adversarial Regression Requirement (BLOCKING)

The regression fixture **MUST NOT be tautological**. A fixture built from two byte-identical artifacts, or from a single artifact, proves nothing: the pre-existing `ON CONFLICT (content_hash)` clause already absorbs those cases and would keep the test green even with `source_ref` dropped from the `INSERT`.

**The required fixture shape:**

> Two artifacts sharing **the same `source_id` and the same non-empty `source_ref`**, but carrying **different content and therefore different `content_hash` values**, ingested sequentially through `PublishRawArtifact`.

This fixture is adversarial because it is the exact input for which the two independent dedup mechanisms disagree: the `content_hash` path **cannot** fire (hashes differ), so only a working `source_ref` path can produce the single-row outcome. If `source_ref` is ever again dropped from the `INSERT` column list, the probe finds nothing, both rows are inserted, and **this test fails**. That is the falsifiability property the regression must have.

A second adversarial case is required for the inverse failure: two artifacts with **the same `source_ref` but different `source_id`**, which MUST both be retained. This one fails if a fixer "fixes" the probe without `source_id` scoping.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| `T-BUG-019-003-1-01` | Pre-fix reproduction: `source_ref` is NULL for all connector rows | integration | ephemeral test DB via `./smackerel.sh test integration` | `SELECT source_ref IS NULL … GROUP BY source_id` reports `t` for every connector `source_id`; captured verbatim | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-1-02` | Pre-fix reproduction: source-ref dedup never fires | integration | ephemeral test DB via `./smackerel.sh test integration` | Same `source_ref`, differing content ⇒ **2** rows exist pre-fix (proves the probe is inert) | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-1-03` | Adversarial regression fixture (same `source_id` + same `source_ref`, differing `content_hash`) FAILS pre-fix | integration | `internal/pipeline` integration suite, real ephemeral DB | Asserts exactly 1 row; **exits non-zero pre-fix**; would fail again if `source_ref` were re-dropped from the `INSERT` | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-1-04` | Adversarial inverse fixture (same `source_ref` + different `source_id`) FAILS pre-fix on persistence | integration | `internal/pipeline` integration suite, real ephemeral DB | Asserts both rows retained AND both carry non-NULL `source_ref`; the persistence half fails pre-fix | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-1-05` | Unit: front-door binds `SourceRef` into the `source_ref` column | unit | `internal/pipeline` unit suite via `./smackerel.sh test unit` | Column list of the executed `INSERT` includes `source_ref` with `artifact.SourceRef` bound; fails pre-fix | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-1-06` | Scenario acceptance for `SCN-BUG-019-003-001` | scenario-acceptance | asserted by `T-BUG-019-003-1-03` and `T-BUG-019-003-1-05` against the ephemeral test DB | The persisted `artifacts` row holds the connector-supplied `SourceRef` in `source_ref` AND the connector-supplied URL in `source_url`, with the two values not conflated in either direction | `SCN-BUG-019-003-001` |

### Definition of Done

- [ ] Runtime reproduction executed against an ephemeral test stack; verbatim output for `bug.md` Reproduction Plan steps 3-6 recorded in `report.md` (≥10 lines each) — **Phase:** analyze
- [ ] `T-BUG-019-003-1-01` executed and its NULL-`source_ref` output recorded verbatim, confirming the defect at runtime rather than by inspection alone — **Phase:** test
- [ ] `T-BUG-019-003-1-02` executed and its 2-row pre-fix result recorded verbatim, proving the dedup probe is inert — **Phase:** test
- [ ] `T-BUG-019-003-1-03` adversarial regression fixture authored and **observed FAILING** before any source change, with the failure output recorded verbatim — **Phase:** test
- [ ] `T-BUG-019-003-1-04` adversarial inverse fixture authored and **observed FAILING** on its persistence assertion before any source change — **Phase:** test
- [ ] `T-BUG-019-003-1-05` unit assertion authored and **observed FAILING** before any source change — **Phase:** test
- [ ] Fixture non-tautology argued explicitly in `report.md`: the recorded pre-fix failure demonstrates the `content_hash` path cannot mask the missing `source_ref` path — **Phase:** test
- [ ] `SCN-BUG-019-003-001` satisfied: for a connector-supplied artifact whose `SourceRef` is a non-empty stable external identifier and whose URL is distinct from that identifier, the persisted `artifacts` row holds that `SourceRef` in `source_ref` and that URL in `source_url`, and the two values are not conflated in either direction — asserted by `T-BUG-019-003-1-06` with verbatim output — **Phase:** test
- [ ] Zero source files modified during Scope 1 (`git diff --name-only` shows only test files and this bug packet) — **Phase:** audit

---

## Scope 2 — Restore `source_ref` persistence and correct the dedup probe

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1

### Use Cases (Gherkin)

```gherkin
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

Scenario: SCN-BUG-019-003-008 The composite source index becomes a live access path
  Given source_ref is persisted for connector artifacts
  When a dedup or consumer lookup filters artifacts by source_id together with source_ref
  Then the query is served by the idx_artifacts_source composite index
  And the index's second key column is no longer universally NULL for connector traffic
```

### Implementation Plan

1. Add `source_ref` to the `INSERT INTO artifacts` column list at `internal/pipeline/ingest.go:94` and bind `artifact.SourceRef` via a nil-able type so an empty ref persists as SQL `NULL`, not `''` (design Part 1).
2. Rewrite the dedup probe at `internal/pipeline/ingest.go:51` to `WHERE source_id = $1 AND source_ref = $2`, dropping the `content_hash` conjunct (design Part 2).
3. Make the matched-row behaviour an explicit decision per [design.md → Re-ingestion Semantics](design.md#re-ingestion-semantics-and-the-over-suppression-risk); record the chosen option (A/B/C) and its reasoning in `report.md`.
4. Re-run the Scope 1 fixtures and confirm they flip from FAIL to PASS.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| `T-BUG-019-003-2-01` | Scope 1 adversarial fixture now PASSES | integration | `./smackerel.sh test integration` | `T-BUG-019-003-1-03` flips FAIL→PASS; exactly 1 row for the `(source_id, source_ref)` pair | `SCN-BUG-019-003-003` |
| `T-BUG-019-003-2-02` | Scope 1 adversarial inverse fixture now PASSES | integration | `./smackerel.sh test integration` | `T-BUG-019-003-1-04` flips FAIL→PASS; both rows retained, both with non-NULL `source_ref` | `SCN-BUG-019-003-004` |
| `T-BUG-019-003-2-03` | Empty `SourceRef` persists as SQL NULL | unit + integration | `internal/pipeline` suites | `source_ref IS NULL` is true; `source_ref = ''` matches zero rows | `SCN-BUG-019-003-002` |
| `T-BUG-019-003-2-04` | `source_ref` and `source_url` are not conflated | integration | `internal/pipeline` integration suite | For an artifact with distinct ref and URL, both columns hold their own value | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-2-05` | Dedup does not require `content_hash` equality | integration | `internal/pipeline` integration suite | Match occurs with differing `content_hash`; the probe SQL contains no `content_hash` conjunct | `SCN-BUG-019-003-003` |
| `T-BUG-019-003-2-06` | Chosen re-ingestion semantics are asserted explicitly | integration | `internal/pipeline` integration suite | Asserts the recorded A/B/C decision's observable outcome (for Option B: matched row's `content_raw`/`content_hash` are updated, not frozen) | `SCN-BUG-019-003-003` |
| `T-BUG-019-003-2-07` | `idx_artifacts_source` serves the dedup lookup | integration | ephemeral test DB `EXPLAIN` | Plan for `WHERE source_id = … AND source_ref = …` references `idx_artifacts_source`; second key column no longer universally NULL | `SCN-BUG-019-003-008` |
| `T-BUG-019-003-2-08` | E2E: connector sync round-trip preserves provenance | e2e | `./smackerel.sh test e2e` | An end-to-end connector sync produces artifacts whose `source_ref` is queryable via the live stack | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-2-09` | **Regression E2E** — full ingestion suite green, no green→red drift | e2e | `./smackerel.sh test e2e` + `./smackerel.sh test integration` | No previously-passing test regresses; suite exit 0 | all Scope 2 scenarios |
| `T-BUG-019-003-2-10` | Scenario acceptance for `SCN-BUG-019-003-002` | scenario-acceptance | asserted by `T-BUG-019-003-2-03` on the ephemeral test DB | An artifact whose `SourceRef` is the empty string persists with `source_ref IS NULL`, and no persisted row carries `source_ref = ''` | `SCN-BUG-019-003-002` |
| `T-BUG-019-003-2-11` | Scenario acceptance for `SCN-BUG-019-003-003` | scenario-acceptance | asserted by `T-BUG-019-003-2-01`, `T-BUG-019-003-2-05`, `T-BUG-019-003-2-06` | Re-ingesting `source_id` `bookmarks` + `source_ref` `https://example.test/a` with changed content matches the existing row by `source_ref` scoped to `source_id`, leaves exactly one row for that pair, and reaches that decision without `content_hash` equality | `SCN-BUG-019-003-003` |
| `T-BUG-019-003-2-12` | Scenario acceptance for `SCN-BUG-019-003-004` | scenario-acceptance | asserted by `T-BUG-019-003-2-02` on the ephemeral test DB | `source_ref` `12345` ingested under `source_id` `discord` and again under `source_id` `youtube` with differing content retains BOTH rows; the `discord` row is neither overwritten nor suppressed by the `youtube` row | `SCN-BUG-019-003-004` |
| `T-BUG-019-003-2-13` | Scenario acceptance for `SCN-BUG-019-003-008` | scenario-acceptance | asserted by `T-BUG-019-003-2-07` via ephemeral test DB `EXPLAIN` | A lookup filtering `artifacts` by `source_id` together with `source_ref` is served by `idx_artifacts_source`, and that index's second key column is no longer universally NULL for connector traffic | `SCN-BUG-019-003-008` |

### Definition of Done

- [ ] `internal/pipeline/ingest.go:94` `INSERT` column list includes `source_ref` with `artifact.SourceRef` bound — **Phase:** implement
- [ ] Empty `SourceRef` binds SQL `NULL`, not `''` (nil-able bind type used) — **Phase:** implement
- [ ] `internal/pipeline/ingest.go:51` probe filters `source_id` **and** `source_ref`, with the `content_hash` conjunct removed — **Phase:** implement
- [ ] Re-ingestion semantics decision (A/B/C) recorded in `report.md` with reasoning, per [design.md](design.md#re-ingestion-semantics-and-the-over-suppression-risk) — **Phase:** design
- [ ] `T-BUG-019-003-2-01` PASSES with verbatim output (≥10 lines) showing the FAIL→PASS flip from Scope 1 — **Phase:** test
- [ ] `T-BUG-019-003-2-02` PASSES with verbatim output (≥10 lines) — **Phase:** test
- [ ] `T-BUG-019-003-2-03` PASSES with verbatim output — **Phase:** test
- [ ] `T-BUG-019-003-2-04` PASSES with verbatim output — **Phase:** test
- [ ] `T-BUG-019-003-2-05` PASSES with verbatim output — **Phase:** test
- [ ] `T-BUG-019-003-2-06` PASSES and asserts the chosen semantics, not merely row count — **Phase:** test
- [ ] `T-BUG-019-003-2-07` PASSES; `EXPLAIN` output recorded verbatim — **Phase:** test
- [ ] `T-BUG-019-003-2-08` PASSES with verbatim output — **Phase:** test
- [ ] `SCN-BUG-019-003-002` satisfied: an artifact whose `SourceRef` is the empty string persists with `source_ref IS NULL`, and no persisted row carries `source_ref` equal to the empty string — asserted by `T-BUG-019-003-2-10` with verbatim output — **Phase:** test
- [ ] `SCN-BUG-019-003-003` satisfied: re-ingesting an already-stored external entity (`source_id` `bookmarks`, `source_ref` `https://example.test/a`) whose title and body have changed so its `content_hash` differs matches the existing row by `source_ref` scoped to `source_id`, leaves no second `artifacts` row for that pair, and the dedup decision does not depend on `content_hash` equality — asserted by `T-BUG-019-003-2-11` with verbatim output — **Phase:** test
- [ ] `SCN-BUG-019-003-004` satisfied: an identical `source_ref` (`12345`) arriving from a different connector (`source_id` `youtube` after `source_id` `discord`) with different content and different `content_hash` retains BOTH `artifacts` rows, and the `discord` row is neither overwritten nor suppressed by the `youtube` row — asserted by `T-BUG-019-003-2-12` with verbatim output — **Phase:** test
- [ ] `SCN-BUG-019-003-008` satisfied: a dedup or consumer lookup filtering `artifacts` by `source_id` together with `source_ref` is served by the `idx_artifacts_source` composite index, and that index's second key column is no longer universally NULL for connector traffic — asserted by `T-BUG-019-003-2-13` with the `EXPLAIN` plan recorded verbatim — **Phase:** test
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression
- [ ] Broader E2E regression suite passes — **Phase:** regression

---

## Scope 3 — Verify downstream consumers and resolve the `processor.go` omission

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 2

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BUG-019-003-006 Bookmark re-sync is idempotent when the bookmark title changes
  Given the bookmarks connector has already ingested a bookmark whose normalized URL is stored as its source_ref
  And the same bookmark is re-exported upstream with a changed title so its content_hash differs
  When the bookmarks connector runs a second sync containing that bookmark
  Then the batch dedup query recognises the normalized URL as already known
  And the bookmark is filtered out of the new-only result set
  And exactly one artifacts row exists for that bookmark's normalized URL
```

### Implementation Plan

1. Verify — do not assume — that `internal/connector/bookmarks/dedup.go:131-133` (batch) and `:184-189` (single) now match against real persisted data with no code change.
2. Determine whether `internal/pipeline/processor.go:523` ever **creates** rows for connector-sourced artifacts or only **updates** rows the front door created. If it creates, persist `source_ref` there too; if it only updates, record the omission as intentional with the reason in `report.md` (`spec.md` AC-08).
3. Review `internal/knowledge/sensitivity_query.go:116` `COALESCE(source_url, COALESCE(source_ref, ''))` precedence now that `source_ref` is populated. This is a **review-and-record** item, not a required change — the query degrades safely either way.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| `T-BUG-019-003-3-01` | Bookmark batch dedup recognises a known normalized URL | integration | `internal/connector/bookmarks` integration suite, real ephemeral DB | `FilterNew`-equivalent path returns the bookmark as a duplicate, not as new | `SCN-BUG-019-003-006` |
| `T-BUG-019-003-3-02` | Bookmark re-sync with changed title yields exactly one row | integration | `./smackerel.sh test integration` | One `artifacts` row for the normalized URL after two syncs with differing content | `SCN-BUG-019-003-006` |
| `T-BUG-019-003-3-03` | Bookmark single-URL `IsKnown` returns true for an ingested URL | integration | `internal/connector/bookmarks` integration suite | `IsKnown` returns `true`; fails pre-fix | `SCN-BUG-019-003-006` |
| `T-BUG-019-003-3-04` | `processor.go` disposition verified and recorded | integration | `internal/pipeline` integration suite | Either the processed path persists `source_ref`, or a test proves it only ever updates pre-existing rows | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-3-05` | Sensitivity query unaffected by the change | unit | `internal/knowledge` unit suite | `sensitivity_query.go` results are unchanged or improved; no regression | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-3-06` | **Regression E2E** — consumer suites show no green→red drift | e2e | `./smackerel.sh test e2e` | Bookmarks, knowledge, and pipeline suites all exit 0 | all Scope 3 scenarios |
| `T-BUG-019-003-3-07` | Scenario acceptance for `SCN-BUG-019-003-006` | scenario-acceptance | asserted by `T-BUG-019-003-3-01`, `T-BUG-019-003-3-02`, `T-BUG-019-003-3-03` | A second bookmarks sync carrying the same bookmark with a changed title (differing `content_hash`) has its normalized URL recognised as already known by the batch dedup query, is filtered out of the new-only result set, and leaves exactly one `artifacts` row for that normalized URL | `SCN-BUG-019-003-006` |

### Definition of Done

- [ ] `T-BUG-019-003-3-01` PASSES with verbatim output (≥10 lines) — **Phase:** test
- [ ] `T-BUG-019-003-3-02` PASSES with verbatim output (≥10 lines) — **Phase:** test
- [ ] `T-BUG-019-003-3-03` PASSES with verbatim output — **Phase:** test
- [ ] `T-BUG-019-003-3-04` PASSES; the `processor.go:523` disposition is recorded in `report.md` as either "persists `source_ref`" or "update-only, intentionally omitted because …" — **Phase:** implement
- [ ] `T-BUG-019-003-3-05` PASSES; the `sensitivity_query.go:116` precedence review outcome is recorded in `report.md` — **Phase:** test
- [ ] `SCN-BUG-019-003-006` satisfied: when the bookmarks connector re-syncs a bookmark whose title changed upstream so its `content_hash` differs, the batch dedup query recognises the stored normalized URL as already known, the bookmark is filtered out of the new-only result set, and exactly one `artifacts` row exists for that normalized URL — asserted by `T-BUG-019-003-3-07` with verbatim output — **Phase:** test
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression
- [ ] Broader E2E regression suite passes — **Phase:** regression

---

## Scope 4 — GuestHost booking identity semantics (D3 decided — Option 4)

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 2

> **Decided.** `internal/connector/guesthost/normalizer.go:26` keys on `event.ID` while `internal/api/context.go:284` looks up a booking ID, and three lifecycle events map to one booking. This packet **decides** the semantics: [design.md → D3 Identity-Semantics Decision (Option 4)](design.md#d3-identity-semantics-decision-decided--option-4) — `source_ref = event.EntityID`, `event.ID` retained in `metadata`, coupled to Re-ingestion Option B. Ratification by the `specs/013-guesthost-connector` and API-contract owners is tracked as a routing item (`TR-BUG-019-003-001`), **not** as a start-blocker. If ratification overturns the decision, this scope is re-planned; Scopes 2-3 are unaffected.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BUG-019-003-005 GuestHost booking context resolves by the identifier its API contract advertises
  Given the GuestHost connector has ingested a booking artifact through the shared front door
  And the D3 identity-semantics decision has been resolved and implemented
  When a caller requests booking context using the identifier the API contract advertises
  Then the booking artifact is found rather than returning a no-rows error
  And the returned context carries the booking's guest, property, and stay-date fields
```

### Implementation Plan

1. Confirm the D3 decision (Option 4) is still ratified by the `specs/013-guesthost-connector` and API-contract owners; record the ratification (or overturn) with decider and date in `report.md`.
2. Implement Option 4 in `internal/connector/guesthost/normalizer.go:26` — set `sourceRef = event.EntityID` for booking event types and retain `event.ID` under `metadata` — and, if required, `internal/api/context.go`.
3. Correct the now-stale inline comment at `internal/api/context.go:276-277`, which asserts a persistence and a semantics that were both wrong.
4. Verify the booking-context endpoint resolves end-to-end.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| `T-BUG-019-003-4-01` | Booking artifact carries `EntityID` in `source_ref` and `event.ID` in `metadata` | unit | `internal/connector/guesthost` unit suite | Normalizer output matches D3 Option 4; asserts `source_ref == event.EntityID` AND `metadata` retains `event.ID` | `SCN-BUG-019-003-005` |
| `T-BUG-019-003-4-02` | Booking-context lookup resolves a real ingested booking | integration | `internal/api` integration suite, real ephemeral DB | Query returns a row instead of `pgx.ErrNoRows`; fails pre-fix | `SCN-BUG-019-003-005` |
| `T-BUG-019-003-4-03` | Booking-context response carries guest, property, and stay dates | e2e | `./smackerel.sh test e2e` | Response body contains the booking's guest, property, and check-in/check-out fields | `SCN-BUG-019-003-005` |
| `T-BUG-019-003-4-04` | Three lifecycle events collapse to one booking row, latest state wins | integration | `internal/api` integration suite | `booking.created` → `updated` → `cancelled` yield exactly ONE row whose state is `cancelled` (Option 4 + Option B); asserts the update-in-place path, not merely row count | `SCN-BUG-019-003-005` |
| `T-BUG-019-003-4-05` | **Regression E2E** — GuestHost + API suites show no green→red drift | e2e | `./smackerel.sh test e2e` | Suites exit 0; no previously-passing GuestHost or API test regresses | `SCN-BUG-019-003-005` |
| `T-BUG-019-003-4-06` | Scenario acceptance for `SCN-BUG-019-003-005` | scenario-acceptance | asserted by `T-BUG-019-003-4-02` and `T-BUG-019-003-4-03` | A caller requesting booking context with the identifier the API contract advertises finds the ingested booking artifact instead of a no-rows error, and the returned context carries the booking's guest, property, and stay-date fields | `SCN-BUG-019-003-005` |

### Definition of Done

- [ ] D3 decision (Option 4 + Re-ingestion Option B) ratified or overturned by the `specs/013-guesthost-connector` and API-contract owners; the ratification, decider, and date recorded in `report.md` — **Phase:** design
- [ ] `T-BUG-019-003-4-01` PASSES with verbatim output — **Phase:** test
- [ ] `T-BUG-019-003-4-02` PASSES with verbatim output (≥10 lines) showing the pre-fix `ErrNoRows` → post-fix row flip — **Phase:** test
- [ ] `T-BUG-019-003-4-03` PASSES with verbatim output — **Phase:** test
- [ ] `T-BUG-019-003-4-04` PASSES and asserts the Option 4 + Option B lifecycle semantics explicitly (one row, latest state wins) — **Phase:** test
- [ ] Stale comment at `internal/api/context.go:276-277` corrected to state the actual persisted semantics — **Phase:** implement
- [ ] `SCN-BUG-019-003-005` satisfied: once the D3 identity-semantics decision is implemented, a caller requesting booking context using the identifier the API contract advertises finds the ingested booking artifact rather than receiving a no-rows error, and the returned context carries the booking's guest, property, and stay-date fields — asserted by `T-BUG-019-003-4-06` with verbatim output — **Phase:** test
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression
- [ ] Broader E2E regression suite passes — **Phase:** regression

---

## Scope 5 — Backfill existing NULL `source_ref` rows

**Status:** Not Started
**Priority:** P2
**Depends On:** Scope 2; **and Scope 4 for the `guesthost` population only**

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BUG-019-003-007 Pre-existing NULL source_ref rows are backfilled where reconstructable and counted where not
  Given the artifacts table contains rows ingested before the fix whose source_ref is NULL
  And some of those rows belong to connectors whose source_ref is deterministically reconstructable from persisted columns
  And other rows belong to connectors whose source_ref is not reconstructable from any persisted column
  When the backfill migration runs
  Then the reconstructable rows have source_ref populated with the reconstructed identifier
  And the non-reconstructable rows retain source_ref IS NULL rather than receiving a fabricated value
  And the count of remaining non-reconstructable rows is reported so the residual is measurable
```

### Implementation Plan

1. Run the read-only survey (design step 1): per `source_id`, total rows, NULL-`source_ref` rows, and — for the conditional class — whether the identifier is present in `metadata`. Record verbatim in `report.md`.
2. Backfill the **deterministic** class (`bookmarks` via `NormalizeURL(source_url)`; `browser-history` via `source_url`) in a forward-only, idempotent, batched migration scoped `WHERE source_ref IS NULL`.
3. Backfill the **survey-cleared conditional** class, one `source_id` at a time.
4. Leave `weather` and `google-maps-timeline` NULL — provably non-reconstructable (`design.md` per-connector table). **Do not fabricate.**
5. Report the residual NULL count broken down by `source_id`.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| `T-BUG-019-003-5-01` | Survey output recorded per `source_id` | integration | ephemeral test DB | Total / NULL / metadata-availability reported for every `source_id`; recorded verbatim | `SCN-BUG-019-003-007` |
| `T-BUG-019-003-5-02` | Deterministic backfill populates `bookmarks` rows correctly | integration | migration test against seeded ephemeral DB | Backfilled `source_ref` equals `NormalizeURL(source_url)` for every seeded `bookmarks` row | `SCN-BUG-019-003-007` |
| `T-BUG-019-003-5-03` | Non-reconstructable rows are left NULL, not fabricated | integration | migration test against seeded ephemeral DB | Seeded `weather` and `google-maps-timeline` rows still have `source_ref IS NULL` after the migration | `SCN-BUG-019-003-007` |
| `T-BUG-019-003-5-04` | Migration is idempotent | integration | migration test | Running the migration twice yields identical row state; already-populated `source_ref` values are never overwritten | `SCN-BUG-019-003-007` |
| `T-BUG-019-003-5-05` | Residual NULL count reported and recorded | integration | ephemeral test DB | Final `source_ref IS NULL` count per `source_id` emitted and recorded verbatim in `report.md` | `SCN-BUG-019-003-007` |
| `T-BUG-019-003-5-06` | **Regression E2E** — post-backfill ingestion and consumers still green | e2e | `./smackerel.sh test e2e` | Full suite exits 0 after the migration; no consumer regresses | `SCN-BUG-019-003-007` |
| `T-BUG-019-003-5-07` | Scenario acceptance for `SCN-BUG-019-003-007` | scenario-acceptance | asserted by `T-BUG-019-003-5-02`, `T-BUG-019-003-5-03`, `T-BUG-019-003-5-05` | Pre-fix rows that are deterministically reconstructable end up carrying the reconstructed `source_ref`; rows that are not reconstructable retain `source_ref IS NULL` rather than a fabricated value; and the remaining non-reconstructable count is reported so the residual is measurable | `SCN-BUG-019-003-007` |

### Definition of Done

- [ ] Survey executed and recorded verbatim per `source_id` in `report.md` — **Phase:** analyze
- [ ] `T-BUG-019-003-5-02` PASSES with verbatim output (≥10 lines) — **Phase:** test
- [ ] `T-BUG-019-003-5-03` PASSES; no fabricated `source_ref` written for `weather` or `google-maps-timeline` — **Phase:** test
- [ ] `T-BUG-019-003-5-04` PASSES; idempotency demonstrated by a double run — **Phase:** test
- [ ] `T-BUG-019-003-5-05` PASSES; residual NULL count per `source_id` recorded verbatim in `report.md` — **Phase:** test
- [ ] `SCN-BUG-019-003-007` satisfied: of the pre-fix `artifacts` rows whose `source_ref` is NULL, the deterministically reconstructable ones end up carrying the reconstructed `source_ref` value, the non-reconstructable ones retain `source_ref IS NULL` rather than receiving a fabricated value, and the count of remaining non-reconstructable rows is reported so the residual is measurable — asserted by `T-BUG-019-003-5-07` with verbatim output — **Phase:** test
- [ ] Historical-duplicate consequence documented honestly in `report.md`: pre-fix duplicates for non-reconstructable connectors remain, and one final boundary duplicate per external entity is expected — **Phase:** docs
- [ ] `guesthost` rows excluded from backfill until the Scope 4 D3 ruling is implemented — **Phase:** implement
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression
- [ ] Broader E2E regression suite passes — **Phase:** regression

---

## Scope 6 — Documentation, release, and capability propagation

**Status:** Not Started
**Priority:** P1
**Depends On:** Scopes 2, 3, 5 (and Scope 4 if the D3 ruling changes the documented contract)

> **Not editable in planning mode.** Gate G073 forbids editing these files while authoring this packet, so every item below is an **unchecked DoD item for the future fixer** with an exact path. The current state of each target was verified read-only and is recorded so the fixer knows precisely what is stale.

### Propagation Targets (verified to exist at packet-authoring time)

| Target | Exact location | Current state (verified read-only) | Required change |
|---|---|---|---|
| Trust architecture claim | `docs/smackerel.md` §17.1 Trust Architecture, table row at **line 2490** | Reads: *"Every artifact has `created_at`, `source_id`, `capture_method`, `processing_tier`. Full provenance."* The row claims **full provenance** while the declared `source_ref` identity column is universally NULL. | After the fix, either add `source_ref` to the enumerated provenance fields, or — until Scope 5 completes — qualify the "full provenance" claim to reflect the non-reconstructable historical residual. |
| Connector inventory | `docs/smackerel.md` §22.7, **line 2945** (*"Committed Connector Inventory (17 connectors)"*) | Accurate count; no provenance statement | Verify no provenance claim needs qualification alongside the corrected behaviour. |
| API reference | `docs/API.md` — §Manual Ingest (**line 170**), §Chrome Extension Bridge Ingestion (**line 274**), §Source Health (**line 35**) | Verified: `docs/API.md` currently contains **no** `source_ref` field documentation and **no** booking-context endpoint section | Audit these three ingest/health surfaces; document the `source_ref` field wherever an artifact or ingest response shape implies a source reference. If the D3 ruling changes the booking-context contract, that endpoint must be documented here. |
| Architecture | `docs/Architecture.md` §Data And Control Flows (**line 44**), §Major Components (**line 34**) | Describes the ingestion pipeline without stating the provenance/identity contract | Describe the corrected ingestion provenance contract: `source_ref` persistence, `(source_id, source_ref)` dedup identity, and the relationship to the `content_hash` fallback. |
| Release features | `docs/releases/mvp/features.md` §Connector roster (**line 104**), §Capability evidence trace (**line 196**) | Connector roster is LOCKED at MVP; no provenance behaviour statement | Reflect the corrected provenance behaviour in the owning `mvp` release packet's features list. |
| Capability ledger | `.github/bubbles/capability-ledger.yaml` | **Verified: this ledger is framework-owned** (Bubbles capabilities such as `workflow-orchestration`, `artifact-ownership`). It contains **no** product-level connector/ingest/provenance capability entry. | **Verification item, not an edit item.** Re-confirm at fix time that provenance is still not a claimed capability there. If it has since become one, update it; otherwise record "no product provenance capability claimed — no change required" and make no edit. |

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| `T-BUG-019-003-6-01` | Trust-architecture provenance row reconciled | docs-verification | `docs/smackerel.md:2490` | The §17.1 row no longer asserts provenance the implementation does not deliver | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-6-02` | API reference audited for source-reference surfaces | docs-verification | `docs/API.md:35,170,274` | Every documented ingest/artifact response shape implying a source reference is accurate post-fix | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-6-03` | Architecture ingestion description updated | docs-verification | `docs/Architecture.md:34,44` | Ingestion provenance + dedup identity contract is described | `SCN-BUG-019-003-003` |
| `T-BUG-019-003-6-04` | Release features list reflects corrected provenance | docs-verification | `docs/releases/mvp/features.md:104,196` | The `mvp` packet states the corrected behaviour | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-6-05` | Capability ledger re-verified | docs-verification | `.github/bubbles/capability-ledger.yaml` | Either updated (if provenance became a claimed capability) or explicitly recorded as requiring no change | `SCN-BUG-019-003-001` |
| `T-BUG-019-003-6-06` | Artifact lint PASSES on this bug packet | artifact | `.github/bubbles/scripts/artifact-lint.sh` | Exit 0 against `specs/019-connector-wiring/bugs/BUG-019-003-source-ref-never-persisted` | all |
| `T-BUG-019-003-6-07` | Traceability guard PASSES on this bug packet | artifact | `.github/bubbles/scripts/traceability-guard.sh` | Exit 0; every Gherkin scenario maps to a DoD item | all |
| `T-BUG-019-003-6-08` | **Regression E2E** — documentation freshness guard green | artifact | `.github/bubbles/scripts/artifact-freshness-guard.sh` | Exit 0; no stale documentation claims remain | all |

### Definition of Done

- [ ] `docs/smackerel.md` §17.1 Trust Architecture row at line 2490 reconciled with the delivered provenance behaviour — **Phase:** docs
- [ ] `docs/smackerel.md` §22.7 connector-inventory section at line 2945 verified for provenance-claim accuracy — **Phase:** docs
- [ ] `docs/API.md` §Manual Ingest (line 170), §Chrome Extension Bridge Ingestion (line 274), and §Source Health (line 35) audited; `source_ref` documented wherever a response shape implies a source reference — **Phase:** docs
- [ ] `docs/Architecture.md` §Major Components (line 34) and §Data And Control Flows (line 44) describe the corrected ingestion provenance and dedup-identity contract — **Phase:** docs
- [ ] `docs/releases/mvp/features.md` §Connector roster (line 104) and §Capability evidence trace (line 196) reflect the corrected provenance behaviour — **Phase:** docs
- [ ] `.github/bubbles/capability-ledger.yaml` re-verified; either updated or explicitly recorded in `report.md` as "no product provenance capability claimed — no change required" — **Phase:** docs
- [ ] `T-BUG-019-003-6-06` artifact lint exits 0 with verbatim output — **Phase:** validate
- [ ] `T-BUG-019-003-6-07` traceability guard exits 0 with verbatim output — **Phase:** validate
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** regression
- [ ] Broader E2E regression suite passes — **Phase:** regression

---

## Build Quality Gate (applies to Scopes 2-6)

- [ ] `./smackerel.sh check` exits 0 with zero warnings — **Phase:** stabilize
- [ ] `./smackerel.sh lint` exits 0 with zero warnings — **Phase:** stabilize
- [ ] `./smackerel.sh format --check` exits 0 — **Phase:** stabilize
- [ ] `./smackerel.sh test unit` exits 0 — **Phase:** test
- [ ] `./smackerel.sh test integration` exits 0 against the ephemeral test stack (never the persistent dev stack) — **Phase:** test
- [ ] `./smackerel.sh test e2e` exits 0 — **Phase:** test
- [ ] No issue encountered during the fix is deferred, skipped, or worked around — **Phase:** audit
- [ ] `bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring/bugs/BUG-019-003-source-ref-never-persisted` exits 0 — **Phase:** validate
- [ ] `bash .github/bubbles/scripts/state-transition-guard.sh specs/019-connector-wiring/bugs/BUG-019-003-source-ref-never-persisted` exits 0 before any terminal status is set — **Phase:** validate
