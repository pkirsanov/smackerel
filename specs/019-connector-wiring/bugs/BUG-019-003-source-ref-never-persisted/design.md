# Bug Fix Design: BUG-019-003 — `artifacts.source_ref` never persisted

> **Bug:** [bug.md](bug.md) | **Spec:** [spec.md](spec.md) | **Scopes:** [scopes.md](scopes.md) | **Report:** [report.md](report.md) | **Validation:** [uservalidation.md](uservalidation.md)
> **Authoring Workflow Mode:** `product-to-planning` (statusCeiling `specs_hardened`, Gate G073 ACTIVE — no source edited)
> **Planned Fix Workflow Mode:** `bugfix-fastlane`

---

## Root Cause Analysis

### Investigation Summary

The connector subsystem has a clean layered shape: each connector builds `connector.RawArtifact` values (including a `SourceRef` identity field, `internal/connector/connector.go:43`), the supervisor iterates a registry of all 17 connectors and hands each produced item to one shared publisher (`internal/connector/supervisor.go:362`), and that publisher writes to PostgreSQL. The investigation traced a `SourceRef` value from construction to persistence and found the value is dropped at the last hop.

### Root Cause

**A single omitted column in one `INSERT` statement, plus a single wrong column name in one `SELECT`, in the one function every connector's output passes through.**

`internal/pipeline/ingest.go:94`:

```
INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash,
                       source_id, source_url, processing_tier, capture_method,
                       processing_status, metadata, evergreen_score, evergreen_source)
```

`source_ref` is not in the column list. `artifact.SourceRef` is never bound. It is referenced only as a `slog` field at `:57` and `:107` — the logs assert a persistence that never happens, which is precisely why the defect stayed invisible.

`internal/pipeline/ingest.go:51`:

```sql
SELECT id FROM artifacts WHERE source_url = $1 AND content_hash = $2 LIMIT 1
```

bound to `artifact.SourceRef, contentHash`. The comment directly above (`:47`) states the intent — *"Dedup by source_ref"* — so this is a genuine implementation/intent divergence, not a deliberate design choice.

**Why the two defects are one root cause and must be fixed together.** They are mutually masking. Fixing D2 alone (probe `source_ref` instead of `source_url`) yields a probe against a universally-NULL column — still zero matches, still no behaviour change, and now with a *dead* rather than merely *wrong* query. Fixing D1 alone (persist `source_ref`) populates the column while the probe still reads `source_url` — provenance is restored but dedup remains broken. Only the paired fix produces observable correct behaviour. This is also why no test caught either one: neither defect, in isolation, changes any externally observable output.

### Why This Survived to Certification

The `INSERT` carries `ON CONFLICT (content_hash) WHERE content_hash IS NOT NULL DO NOTHING` (`internal/pipeline/ingest.go:96`). That clause absorbs the common duplicate case — the *byte-identical* re-ingest — which is the case any hand-run smoke test naturally produces. The failure mode this bug exposes is the *content-changed re-ingest*, which requires deliberately mutating upstream content between two syncs. No existing test does that. The system therefore looked correct under every test that was written.

### Impact Analysis

- **Affected components:** `internal/pipeline/ingest.go` (front door), `internal/pipeline/processor.go` (processed-artifact path), all 17 connectors registered at `cmd/core/connectors.go:73-79`, `internal/api/context.go` (booking context), `internal/connector/bookmarks/dedup.go` (both dedup paths).
- **Affected data:** `artifacts.source_ref` is NULL for every connector-ingested row in every existing deployment. Duplicate rows exist wherever an upstream item's content changed between syncs. `idx_artifacts_source` is dead.
- **Affected users:** any consumer of GuestHost booking context (hard failure); any user re-syncing bookmarks (silent duplicate accumulation); any downstream feature that assumes artifact provenance is complete.
- **Not affected:** `location_clusters.source_ref` and the `photos` table's `source_ref` — separate tables with their own correct writers (`internal/connector/maps/connector.go:475`, `internal/connector/photos/store.go:124`). `internal/knowledge/sensitivity_query.go:116` degrades safely via `COALESCE`. See [bug.md → `F-BUG-019-003-005`](bug.md#f-bug-019-003-005--scoping-correction-recorded-to-prevent-wasted-fix-effort).

---

## Fix Design

### Solution Approach

**Two paired minimal edits at the front door, then consumer verification, then backfill.**

**Part 1 — persist the column (`internal/pipeline/ingest.go:94`).** Add `source_ref` to the `INSERT` column list and bind `artifact.SourceRef`. Bind SQL `NULL` rather than `''` when `SourceRef` is empty, so "no identifier" stays distinguishable from "empty identifier" and a future partial-unique index remains possible (`spec.md` EB-1 / AC-02). In Go this means binding a nil-able type (e.g. `*string` or `sql.NullString`) rather than a bare `string`.

**Part 2 — correct the probe (`internal/pipeline/ingest.go:51`).** Change the predicate to compare `source_ref` against `source_ref`, scoped by `source_id`:

```sql
SELECT id FROM artifacts WHERE source_id = $1 AND source_ref = $2 LIMIT 1
```

Two deliberate changes beyond the column-name correction:

- **Add `source_id` scoping.** Refs are connector-local opaque strings. `discord` and `youtube` can both legitimately emit `"12345"`. Without `source_id` scoping the fix would introduce a *new* cross-connector false-positive dedup defect — worse than the bug being fixed. (`spec.md` AC-04 / `SCN-BUG-019-003-004`.)
- **Drop the `content_hash` conjunct.** Requiring `source_ref` AND `content_hash` to match makes the probe redundant with the `ON CONFLICT (content_hash)` clause and defeats its purpose: a source-ref probe exists exactly to catch *the same entity whose content changed*. Keeping the conjunct would leave the bug functionally unfixed while appearing fixed. (`spec.md` AC-03 / `SCN-BUG-019-003-003`.)

**Part 3 — `internal/pipeline/processor.go:523`.** The processed-artifact `INSERT` also omits `source_ref`. The fixer must determine whether that path ever creates rows for connector-sourced artifacts or only updates rows the front door already created. If it creates, it must persist `source_ref` too; if it only ever updates, the omission must be **documented as intentional with the reason recorded**, not left silent. (`spec.md` AC-08.)

**Part 4 — consumers.** With Parts 1-2 landed, `internal/connector/bookmarks/dedup.go:131,185` should begin matching with no code change (it already normalizes URLs into `SourceRef` at `:160`). This must be *verified*, not assumed. `internal/api/context.go:284` remains blocked on the D3 decision below.

**Part 5 — backfill.** See [Backfill / Migration Strategy](#backfill--migration-strategy).

### Alternative Approaches Considered

1. **Reuse `source_url` as the identity column; drop `source_ref` from the schema.**
   *Rejected.* The two fields carry genuinely different meanings and diverge in practice: `hospitable` emits `"reservation:<id>"`, `markets` emits `"fred-<series>-<date>"`, `maps` emits a hash — none of which are URLs. Collapsing them would corrupt `source_url` for every non-URL-identified connector and break every consumer that reads `source_url` as a link. It would also silently redefine a shipped column.

2. **Write the ref into `metadata` JSONB instead of the dedicated column.**
   *Rejected.* The column already exists (`001_initial_schema.sql:29`) and is already indexed (`:68`). Four consumers already query the column by name. Routing around it would leave the schema permanently lying about itself and the index permanently dead, and would make every dedup probe a JSONB extraction.

3. **Add `UNIQUE (source_id, source_ref)` and rely on `ON CONFLICT` instead of an explicit probe.**
   *Rejected for this fix; recorded as deferred.* It cannot be applied while a large non-reconstructable NULL residual exists (see below), and it changes failure semantics from "skip" to "constraint violation" across all 17 connectors at once. See [Deferred Follow-On](#deferred-follow-on-unique-source_id-source_ref).

4. **Fix D2 only (correct the column name), leave D1 for later.**
   *Rejected.* As established in Root Cause, this produces zero behaviour change and creates a false impression of resolution. The two defects mask each other and must land together.

---

## Re-ingestion Semantics And The Over-Suppression Risk

**This fix activates a dedup path that has never once executed in production.** That is a genuine behaviour-change risk and deserves explicit treatment rather than an assumption that "more dedup is better".

**The risk.** Before the fix, a connector re-emitting the same external entity with changed content produced a **new row**. After the fix, that same input **matches an existing row**. If the matched-row branch simply returns the existing ID and discards the incoming artifact — which is exactly what `internal/pipeline/ingest.go:52-61` does today for its (never-firing) probe — then **updated content is silently dropped**. A bookmark whose page was rewritten, a calendar event whose time moved, a booking that changed status would all be ingested once and then frozen forever.

This is not hypothetical: it is precisely the failure mode `BUG-016-W3` *believed* it was preventing (`specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/bug.md:78` — *"leaving stale weather data in the knowledge graph"*). That concern was inapplicable then because the mechanism was absent. **After this fix it becomes applicable for the first time.** The fixer must not inherit the pre-fix skip-on-match branch unexamined.

**Required treatment.** The fixer MUST make the matched-row behaviour an explicit, tested decision, choosing between:

| Option | Behaviour on `(source_id, source_ref)` match with different `content_hash` | Trade-off |
|---|---|---|
| **A — skip** (today's branch) | Return existing ID, discard incoming | Simplest; **but permanently freezes stale content.** Reintroduces the exact staleness risk quoted above. |
| **B — update-in-place** | Update `content_raw`, `content_hash`, `title`, `metadata`, `updated_at` on the matched row | Keeps one row per external entity AND keeps it current. Must not clobber downstream-enriched columns (`summary`, `key_ideas`, `entities`, `embedding`) without re-triggering processing. |
| **C — insert new revision** | Retain history, mark prior row superseded | Highest fidelity; largest schema and consumer change; out of proportion to a bug fix. |

**Decision of record: Option B**, scoped narrowly — update only the raw-capture columns (`content_raw`, `content_hash`, `title`, `metadata`, `updated_at`) and re-publish to NATS so downstream enrichment reruns, leaving derived columns (`summary`, `key_ideas`, `entities`, `embedding`) to be regenerated rather than blanked.

Justification: Option A is the status-quo branch and would ship a *new* staleness defect under the banner of a bug fix — it is the only option that makes the system worse than before the fix for changed content. Option C (revision history) is a feature with schema and consumer consequences out of all proportion to a bug fix. Option B is the only choice that satisfies the fix's actual goal — one row per external entity — without introducing a staleness regression. It is also a hard prerequisite of the [D3 Option 4 decision](#d3-identity-semantics-decision-decided--option-4).

The implementer MUST still record the observed behaviour in `report.md` and assert it in `SCN-BUG-019-003-003` explicitly (not merely "only one row exists"); `spec.md` EB-5 encodes this requirement.

---

## Backfill / Migration Strategy

### The honest position

**Most existing NULL `source_ref` rows cannot be reconstructed, and no backfill should pretend otherwise.** A fabricated ref is worse than a NULL: NULL is honestly "unknown", whereas a fabricated ref will be matched against by a future dedup probe and will suppress or mis-merge real artifacts. The strategy below is therefore deliberately conservative: **reconstruct only where the derivation is provably deterministic from persisted columns; leave everything else NULL and count it.**

### Per-connector reconstructability

Derived by reading each connector's `SourceRef` construction against the columns the front door actually persists (`source_id`, `source_url`, `title`, `content_raw`, `content_hash`, `metadata`, `created_at`).

| Class | Connectors | Basis | Verdict |
|---|---|---|---|
| **Deterministically reconstructable** | `bookmarks` (`SourceRef = NormalizeURL(b.URL)`, `bookmarks.go:129`); `browser-history` primary path (`SourceRef = e.URL`, `browser/connector.go:514`) | `source_url` is persisted, and the transform is a pure function of it | **Backfill.** `bookmarks` via `NormalizeURL(source_url)`; `browser-history` via `source_url` directly. |
| **Likely reconstructable, requires verification** | `youtube` (`vid.VideoID`, `youtube.go:148`); `twitter` primary path (`tweet.ID`, `twitter.go:911,1234`) | The ID is typically embedded in the canonical `source_url` | **Backfill only after a spot-check proves the extraction is exact for the stored URL shapes.** Do not regex-guess at scale without that proof. |
| **Conditionally reconstructable — gated on a metadata survey** | `imap` (`msg.UID`); `caldav` (`evt.UID`); `keep` (`noteID`); `rss` (`item.GUID`); `discord`; `alerts` (`alert.ID`/`eq.ID`/`obs.ID`); `hospitable` (`"property:"`/`"reservation:"`/`"message:"`/`"review:"` + ID); `qf-decisions` (`envelope.PacketID`); `markets` (`quote-<symbol>-<date>` etc.); `card-rewards` (`<url>#<date>`) | Reconstructable **iff** the connector independently wrote the identifier into `metadata` JSONB | **Survey first.** For each connector, confirm the identifier is present in `metadata` for real stored rows. Backfill only the connectors where it is. This survey is a required, non-skippable pre-backfill step; its results must be recorded in `report.md`. |
| **Provably NOT reconstructable** | `weather`; `google-maps-timeline` | `weather`: `SourceRef = "<type>-<location>-<syncSuffix>"` (`weather.go:273-275`) where `syncSuffix` comes from `nextSourceRefSuffix(now)` (`:150`) — a per-sync high-precision suffix persisted nowhere. `maps`: `SourceRef = sourceRefHash("<date>:<type>:<hour>:<grid-lat,lng>:<grid-lat,lng>")` (`maps/normalizer.go:132-140`) — a hash over grid-rounded activity fields not persisted on the artifact row. | **Leave NULL. Do not fabricate.** Any reconstruction would be a guess that a live dedup probe would then trust. |
| **Not applicable** | `photos`, `drive/scan` | Bypass `PublishRawArtifact`; already write `source_ref` correctly | No action. |
| **Blocked** | `guesthost` | Backfill target depends on the D3 decision below | **Do not backfill until D3 is resolved.** Backfilling to `event.ID` and then deciding on `EntityID` would require a second migration. |

### Migration shape

1. **Survey (read-only, no writes).** Per `source_id`, report: total rows, rows with `source_ref IS NULL`, and — for the conditional class — whether the identifier is present in `metadata`. Record verbatim in `report.md`.
2. **Backfill the deterministic class** in a forward-only migration, `WHERE source_ref IS NULL` and scoped by `source_id`, in bounded batches. The migration MUST be idempotent (safe to re-run) and MUST NOT touch rows that already have a non-NULL `source_ref`.
3. **Backfill the survey-cleared conditional class** the same way, one `source_id` at a time.
4. **Report the residual.** Emit the final count of rows still `source_ref IS NULL`, broken down by `source_id`. This number is the honest measure of permanently-unrecoverable provenance and MUST appear in `report.md` (`spec.md` AC-09 / `SCN-BUG-019-003-007`).
5. **No down-migration data restoration is offered.** Rolling back the schema-side change is trivial (the column already exists and stays); rolling back the *data* is not meaningful, since the pre-state is NULL and re-NULLing backfilled rows would discard correct information. The rollback entry in `state.json` reflects this.

### What the residual means for historical dedup

For the non-reconstructable population, historical duplicates **cannot be retroactively merged** — there is no key to merge them on. Two consequences must be stated plainly rather than glossed:

- Existing duplicate rows from `weather`, `maps`, and any survey-failing connector **remain duplicated**. The fix stops the bleeding; it does not heal the wound.
- After the fix, a `weather` or `maps` artifact re-ingested with the *same* newly-persisted ref will dedup correctly going forward, but will **not** match its own pre-fix NULL-ref ancestors. Expect one final duplicate per external entity at the fix boundary. This is unavoidable and must be documented, not hidden.

If the operator judges the historical duplicate population unacceptable, the remedy is a separate, explicitly-scoped cleanup effort with its own bug packet — **not** a fabricated backfill inside this one.

---

## D3 Identity-Semantics Decision (DECIDED — Option 4)

**Decision of record: Option 4 (`source_ref = event.EntityID`, with `event.ID` retained in `metadata`), coupled to Re-ingestion Option B.** This packet decides D3 rather than deferring it, because the D1/D2 fix cannot be implemented coherently without knowing what `source_ref` *means* for `guesthost`, and because Scope 5's backfill target for `guesthost` is undefined until this is settled.

The decision is recorded here as the plan of record. It carries an API-contract consequence, so the `specs/013-guesthost-connector` owner and the API-contract owner MUST be **notified for ratification** (tracked as a routing item in `state.json.transitionRequests`, not as a blocker on this packet). If ratification overturns it, Scope 4 is re-planned; the D1/D2 fix in Scopes 2-3 is unaffected either way.

### The problem

`internal/connector/guesthost/normalizer.go:27` sets `sourceRef := event.ID` — the **activity-event** ID. The external business entity is `ActivityEvent.EntityID` (`internal/connector/guesthost/types.go:10`). `internal/api/context.go:284` looks up by a **booking** ID, with an inline comment at `:276-277` asserting *"Booking artifacts from guesthost have source_ref set to the event ID"* — an assertion that is doubly wrong today (the column is NULL, and the semantics the consumer needs are the entity's, not the event's).

The relationship is **many-to-one**: `booking.created`, `booking.updated`, and `booking.cancelled` are three distinct activity events describing one booking. No renaming resolves that.

### Options

| Option | Mechanism | Pros | Cons |
|---|---|---|---|
| **1 — key on `EntityID`** | `sourceRef = event.EntityID` for booking event types | Booking-context lookup works directly; one artifact per booking | Collapses three lifecycle events into one identity — `booking.updated` would dedup against `booking.created`. Under Re-ingestion Option B that is arguably *correct* (latest state wins), but it **discards event history**. |
| **2 — keep `event.ID`, add `EntityID` to `metadata`, query metadata** | `source_ref` unchanged; consumer switches to `metadata->>'entity_id'` | Preserves full event history; no identity change | The `(source_id, source_ref)` index does not serve the lookup — booking-context queries fall back to a JSONB scan. Leaves `source_ref` semantically inconsistent with other connectors. |
| **3 — composite ref** | `sourceRef = "<EntityID>:<eventType>"` | Preserves event history AND makes the ref entity-derived; index-usable via prefix | Consumer must know the event-type suffix or use a prefix match, which the composite index supports only partially. Adds parsing at every consumer. |
| **4 — dual persistence** | `source_ref = event.EntityID`, plus event ID retained in `metadata` | Booking lookup is index-served; event ID remains available for audit | Requires the Re-ingestion Option B semantics to be settled first, since events would now collide by design. |

### Decision and justification

**Option 4 is adopted, contingent on Re-ingestion Option B (also adopted — see [Re-ingestion Semantics](#re-ingestion-semantics-and-the-over-suppression-risk)).**

Why Option 4 is the long-term-correct choice:

1. **The consumer contract is expressed in business-entity terms.** `internal/api/context.go:284` is handed a *booking* ID by its caller. An identity column that stores an *activity-event* ID can never satisfy that lookup without a join or a scan. Storing the entity ID makes the existing query correct as written.
2. **`source_ref` is the indexed identity column.** `idx_artifacts_source (source_id, source_ref)` exists precisely to serve `(connector, external-entity)` lookups. Putting the entity ID there makes the index discriminating for `guesthost`; Option 2 would leave the lookup on a JSONB scan forever.
3. **Nothing is lost.** Retaining `event.ID` in `metadata` preserves full audit lineage at zero storage or complexity cost, so Option 4 strictly dominates Option 1 (which discards the event ID entirely).
4. **Consistency across connectors.** Every other connector's `source_ref` denotes "the stable external thing this artifact is about" (`vid.VideoID`, `msg.UID`, `evt.UID`, `item.GUID`). Keying `guesthost` on a per-event ID would make it the sole semantic outlier. Option 3's composite ref reintroduces that outlier status *and* pushes string-parsing into every consumer.

Why the Option B coupling is mandatory, not optional: under Option 4 the three lifecycle events (`booking.created` / `booking.updated` / `booking.cancelled`) collide on one `source_ref` **by design**. With Option A (skip-on-match) that collision would cause `booking.updated` to be silently discarded, permanently freezing every booking in its created state — strictly worse than today's bug. Option B (update-in-place) turns the same collision into the desired behaviour: one artifact per booking, always reflecting the latest known state. **Option 4 without Option B is prohibited.**

Residual risk accepted: per-event history is no longer recoverable from `artifacts` rows alone (only the latest state plus the latest event ID in `metadata`). If an auditable per-event trail is later required, that is a separate feature (Re-ingestion Option C), not a bug fix.

---

## Deferred Follow-On: `UNIQUE (source_id, source_ref)`

A partial unique index — `CREATE UNIQUE INDEX … ON artifacts(source_id, source_ref) WHERE source_ref IS NOT NULL` — would make the invariant structural rather than advisory, and would be the strongest possible regression guard against D1 recurring.

It is **deliberately excluded from this fix**:

- It cannot be created while the historical duplicate population (from the never-active dedup path) still exists — index creation would fail on existing violations.
- It changes failure semantics for all 17 connectors simultaneously, from silent skip to constraint violation, which is too large a blast radius to couple to a bug fix.
- It requires the D3 decision to be settled first, since `guesthost` identity is in flux.

Recorded here so it is not silently forgotten. It should follow as its own scoped change once the fix has soaked and the duplicate population is quantified.

---

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|---|---|---|
| Add `source_id` scoping to the dedup probe, not just correct the column name | Change `source_url` → `source_ref` in the existing `WHERE` clause and stop | Refs are connector-local opaque strings; an unscoped probe would introduce a new cross-connector false-positive dedup defect (`SCN-BUG-019-003-004`), which is strictly worse than the bug being fixed |
| Drop the `content_hash` conjunct from the probe | Keep `AND content_hash = $2` as-is | Requiring both makes the probe redundant with `ON CONFLICT (content_hash)` and cannot catch the actual failure mode (same entity, changed content), leaving the bug functionally unfixed while appearing fixed |
| Bind SQL `NULL` rather than `''` for an empty `SourceRef` | Bind the bare `string`, letting empty become `''` | `''` is a *value* a dedup probe would match on, collapsing every ref-less artifact from a connector into one identity; it also forecloses the deferred partial-unique index |
| Make matched-row behaviour an explicit decision (Re-ingestion A/B/C) | Inherit today's skip-on-match branch unchanged | The branch has never executed; inheriting it unexamined would ship a new content-staleness defect under the banner of a bug fix |
| Survey `metadata` before backfilling the conditional connector class | Backfill all connectors with a best-effort derivation | A fabricated `source_ref` is worse than NULL — a live dedup probe would trust it and mis-merge real artifacts |
| Defer `UNIQUE (source_id, source_ref)` | Add it in the same change as the structural guarantee | Cannot be created over the existing duplicate population; changes failure semantics for 17 connectors at once; blocked on D3 |
