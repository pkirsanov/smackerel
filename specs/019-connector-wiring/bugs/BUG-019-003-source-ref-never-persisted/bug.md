# Bug: BUG-019-003 — `artifacts.source_ref` is never persisted by the connector ingestion front door

## Summary

`RawArtifactPublisher.PublishRawArtifact` — the single shared ingestion front door for all 17 registered connectors — omits `source_ref` from its `INSERT INTO artifacts` column list, and its dedup probe compares the in-memory `SourceRef` against the *`source_url`* column. The declared, indexed `artifacts.source_ref` column is therefore NULL for every connector-produced artifact, connector-level deduplication silently degrades to content-hash-only, the `idx_artifacts_source (source_id, source_ref)` index is dead, and at least two shipped consumers (the GuestHost booking-context API and bookmark re-sync dedup) can never match a row.

## Severity

- [ ] Critical - System unusable, data loss
- [x] High - Major feature broken, no workaround
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

### Severity Argument

**HIGH, not Medium**, on four grounds:

1. **Blast radius is total, not per-connector.** The defect sits in the one function every connector's items flow through (`internal/connector/supervisor.go:362` calls `s.publisher.PublishRawArtifact(connCtx, item)` in a loop over each registered connector's returned items). All 17 connectors registered at `cmd/core/connectors.go:73-79` are affected: `imap`, `caldav`, `youtube`, `rss`, `keep`, `bookmarks`, `browser-history`, `google-maps-timeline`, `hospitable`, `guesthost`, `discord`, `twitter`, `weather`, `gov-alerts`, `financial-markets`, `qf-decisions`, `card-rewards`.
2. **It is a silent data-integrity defect.** No error is raised, no warning is logged. `PublishRawArtifact` even logs `"source_ref", artifact.SourceRef` (`internal/pipeline/ingest.go:57`, `:107`) while never writing that value, so the logs actively assert a persistence that does not occur. Nothing in the running system reports the loss.
3. **A shipped API is broken with no workaround.** `internal/api/context.go:284` filters `AND source_ref = $1` for GuestHost booking context. Because the column is always NULL, that query returns `pgx.ErrNoRows` for every input; there is no alternative predicate a caller can supply.
4. **It has already corrupted downstream reasoning.** A separate bug (`BUG-016-W3-source-ref-collision`, status `done`) was analysed, fixed, and certified on the explicit premise that "Pipeline deduplication uses `SourceRef` as part of artifact identity" (`specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/bug.md:78`). Given D1+D2 that premise is false. Certified work has been reasoned against a mechanism that does not exist.

Not **Critical**: ingested content itself is not lost (`content_raw`, `content_hash`, `source_id`, `source_url`, `metadata` all persist correctly), no crash or unavailability results, and no security or PII exposure is introduced. The loss is confined to the provenance/identity column and everything keyed on it.

## Status

- [ ] Reported
- [x] Confirmed (reproduced by static verification — see Verification Evidence)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

> **Planning-mode note.** This packet was authored under workflow mode `product-to-planning` (statusCeiling `specs_hardened`, Gate G073 Source Code Edit Lockout ACTIVE). Confirmation below is by direct source inspection at the cited `file:line` coordinates. Runtime reproduction is **planned, not executed** — see [Reproduction Plan](#reproduction-plan-not-executed-in-planning-mode).

## Ownership Classification

**Owned by `specs/019-connector-wiring` (this parent feature). Not owned by any individual connector spec.**

Rationale:

- The defective statement is the shared ingestion front door `internal/pipeline/ingest.go::PublishRawArtifact`, which spec 019 is the feature-of-record for as the connector-wiring surface. It is not connector-specific code.
- Every affected connector produces a *correct* `RawArtifact.SourceRef` in its own normalizer. The value is lost after the connector boundary, inside shared pipeline code. Filing this against `016-weather-connector`, `020-bookmarks`, or `013-guesthost` would misattribute a shared-infrastructure defect to a correct component.
- The single exception requiring a connector-owned decision is D3 (GuestHost booking identity semantics). This packet **decides** it (Option 4) so the fix can proceed coherently, and routes it to the `specs/013-guesthost-connector` owner for **ratification** rather than leaving it unresolved.

## Findings

| Finding ID | Title | Kind | Evidence anchor |
|---|---|---|---|
| `F-BUG-019-003-001` | `source_ref` omitted from the ingestion `INSERT` column list; `RawArtifact.SourceRef` silently dropped | Defect (D1) | `internal/pipeline/ingest.go:94` |
| `F-BUG-019-003-002` | Dedup probe compares a source-ref against the `source_url` column | Defect (D2) | `internal/pipeline/ingest.go:51` |
| `F-BUG-019-003-003` | GuestHost booking `sourceRef` is the activity-event ID, not the booking entity ID — identity-semantics decision required | Design question (D3, **DECIDED — Option 4**) | `internal/connector/guesthost/normalizer.go:26` |
| `F-BUG-019-003-004` | Certified `BUG-016-W3` was reasoned against a dedup mechanism that does not exist — blast-radius corroboration | Corroborating evidence | `specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/bug.md:78` |
| `F-BUG-019-003-005` | Scoping correction — the `maps` `source_ref` sites read `location_clusters`, **not** `artifacts`, and are unaffected | Scope correction | `internal/db/migrations/001_initial_schema.sql:322-323` |

---

### `F-BUG-019-003-001` (D1) — `source_ref` is never persisted

`internal/pipeline/ingest.go:94` — the `INSERT INTO artifacts` column list is:

```
(id, artifact_type, title, content_raw, content_hash, source_id, source_url,
 processing_tier, capture_method, processing_status, metadata,
 evergreen_score, evergreen_source)
```

`source_ref` is absent. The `$7` bind is `artifact.URL` into `source_url`. `connector.RawArtifact.SourceRef` (`internal/connector/connector.go:43`) is read only for log fields (`:57`, `:107`) and never written.

`internal/pipeline/processor.go:523` — the second `INSERT INTO artifacts` (processed-artifact path) likewise omits `source_ref`.

**The only writers of `artifacts.source_ref` in the repository** are:

- `internal/connector/photos/store.go:124` (insert), `:197` (`ON CONFLICT … source_ref = EXCLUDED.source_ref`), `:363` (second insert path)
- `internal/drive/scan/service.go:316`

Both are bespoke stores that bypass `PublishRawArtifact`. Every artifact that *does* go through the shared front door — i.e. all 17 registered connectors — lands with `source_ref IS NULL`.

**Schema consequence.** `internal/db/migrations/001_initial_schema.sql:29` declares `source_ref TEXT` on `artifacts`, and `:68` builds `CREATE INDEX IF NOT EXISTS idx_artifacts_source ON artifacts(source_id, source_ref);`. Because the second index column is universally NULL for connector rows, this composite index carries no discriminating information for connector traffic — it is a dead index that still costs write amplification on every ingest.

### `F-BUG-019-003-002` (D2) — dedup probes the wrong column

`internal/pipeline/ingest.go:47-52`:

```go
// Dedup by source_ref (connector-specific unique ID like message UID, video ID, event UID)
if artifact.SourceRef != "" {
    var existingID string
    err := p.DB.QueryRow(ctx,
        "SELECT id FROM artifacts WHERE source_url = $1 AND content_hash = $2 LIMIT 1",
        artifact.SourceRef, contentHash,
    ).Scan(&existingID)
```

The comment states the intent (dedup by `source_ref`); the SQL filters `source_url`; the bound value is `artifact.SourceRef`. The probe can only ever match when a connector happens to set `SourceRef == URL`. For every other connector it is a guaranteed `pgx.ErrNoRows`, and ingestion falls through to `ON CONFLICT (content_hash) … DO NOTHING` (`internal/pipeline/ingest.go:96`).

**The two defects compound.** D2 alone would be a wrong-column bug; D1 alone would be a missing-provenance bug. Together they make the source-ref dedup path *structurally unreachable*: even if D2 were fixed to probe `source_ref`, D1 guarantees the column it probes is always NULL.

**Behavioural consequence.** Connector dedup silently degrades to *content-hash equality only*. Any upstream item whose content changes — a bookmark re-titled, a calendar event edited, a Discord message edited — produces a new `content_hash`, misses both dedup paths, and is inserted as a **duplicate row for the same external entity**.

### `F-BUG-019-003-003` (D3) — GuestHost booking identity semantics (DECIDED — Option 4)

`internal/connector/guesthost/normalizer.go:26-36`:

```go
sourceRef := event.ID
if sourceRef == "" {
    contentHash := sha256.Sum256([]byte(event.Type + event.EntityID + event.Timestamp))
    sourceRef = fmt.Sprintf("%x", contentHash[:])
}
```

`event.ID` is the **activity-event** identifier. The external business entity the event describes is `ActivityEvent.EntityID` (`internal/connector/guesthost/types.go:10`). For booking events the normalizer discards `EntityID` from the identity slot entirely.

`internal/api/context.go:273-289` looks up booking context by `bookingID`:

```sql
WHERE artifact_type = 'booking' AND source_id = 'guesthost' AND source_ref = $1
```

with the preceding comment (`:276-277`) asserting *"Booking artifacts from guesthost have source_ref set to the event ID"*.

So even after D1 and D2 are fixed, a caller passing a **booking ID** will still not match, because the persisted ref would be the **event ID**. This is a distinct identity-semantics defect layered on top of the persistence defect.

**Decision of record: Option 4** — persist `source_ref = event.EntityID` for booking event types and retain `event.ID` in `metadata`, coupled to Re-ingestion **Option B** (update-in-place). Full options table and justification: [design.md → D3 Identity-Semantics Decision (Option 4)](design.md#d3-identity-semantics-decision-decided--option-4). The decision has API-contract consequences (`booking.created` / `booking.updated` / `booking.cancelled` are three events for one booking), so the `specs/013-guesthost-connector` and API-contract owners MUST be notified for **ratification** — tracked as routing item `TR-BUG-019-003-001`, not as a blocker. Scope 4 implements it; Scopes 2-3 are unaffected if it is overturned.

### `F-BUG-019-003-004` — corroboration: a certified bug reasoned against a mechanism that does not exist

`specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/` (status `done`) was raised because two weather syncs within the same second produced identical `SourceRef` values. Its stated impact (`bug.md:78`):

> "Pipeline deduplication uses `SourceRef` as part of artifact identity. Duplicate same-second `SourceRef` values can cause a real later weather observation to be treated as a duplicate of an earlier one, leaving stale weather data in the knowledge graph."

and (`bug.md:42`, quoting the failing test):

> `consecutive syncs produced identical SourceRef "current-City-2026-05-03T21:16:37Z" — would cause pipeline dedup collision`

Given D1+D2, **no such collision was possible**: `source_ref` is never persisted, and the dedup probe reads `source_url`. Two same-second weather observations with identical `SourceRef` but different content produce different `content_hash` values and are both inserted regardless.

This matters for three reasons, and is why this finding is recorded rather than filed away as trivia:

- **It proves the defect is latent and invisible.** A full bug lifecycle — analysis, fix, tests, validation, certification — ran to completion without anyone noticing the mechanism was absent.
- **It bounds the blast radius upward.** The defect does not merely break queries; it corrupts the *reasoning premises* of work built on top of provenance.
- **It establishes urgency ordering.** Any future feature that depends on connector-artifact provenance or identity-based dedup will inherit the same false premise. This defect should be fixed **before** such a feature ships, not alongside it.

**Boundary:** `BUG-016-W3` is a foreign artifact owned by `specs/016-weather-connector`. This packet does **not** reopen, modify, or re-certify it. Its own fix (increasing `SourceRef` timestamp precision) is independently harmless and remains correct-in-itself. A routing note is recorded under [Routing](#routing) so the owner can decide whether a premise-correction annotation is warranted **after** this bug is fixed.

### `F-BUG-019-003-005` — scoping correction (recorded to prevent wasted fix effort)

Two `source_ref` read sites that superficially resemble broken consumers are **not** affected, and a fixer should not chase them:

| Site | Table actually queried | Status |
|---|---|---|
| `internal/connector/maps/patterns.go:225` (`SELECT id, source_ref … FROM location_clusters`) | `location_clusters` | **Unaffected.** Different table. |
| `internal/connector/maps/connector.go:475` (`INSERT INTO location_clusters (id, source_ref, …)`) | `location_clusters` | **Unaffected.** This is a correct writer for its own table. |
| `internal/connector/photos/routing.go:288` (`WHERE p.source_channel=$1 AND p.source_ref=$2`) | `photos` | **Unaffected.** Fed by the bespoke `photos/store.go` writer. |

`location_clusters.source_ref` is declared `TEXT NOT NULL` at `internal/db/migrations/001_initial_schema.sql:323` and is correctly written by `maps/connector.go:475` (binding `sourceRef, sourceRef` into `id, source_ref`). It is a separate table from `artifacts` and is unaffected by D1.

Separately, `internal/knowledge/sensitivity_query.go:116` uses `COALESCE(source_url, COALESCE(source_ref, ''))`. Because `source_url` is populated, this **degrades safely** — it silently returns the URL where a source-ref was intended, but does not error or return empty. It is recorded here as *affected-but-non-failing*; after the fix its precedence order should be reviewed, but it is not a blocker.

## Confirmed Broken Consumers

| Consumer | Location | Failure mode |
|---|---|---|
| GuestHost booking context API | `internal/api/context.go:284` | `WHERE … AND source_ref = $1` matches zero rows for every input → `pgx.ErrNoRows` returned to the caller on every request. Compounded by D3. |
| Bookmark batch dedup | `internal/connector/bookmarks/dedup.go:131-133` | `SELECT source_ref FROM artifacts WHERE source_id = 'bookmarks' AND source_ref = ANY($1)` returns zero rows → `known` map is always empty → **no bookmark is ever recognised as a duplicate** by URL. Note `dedup.go:160` sets `a.SourceRef = normalized[i]` — a value the front door then discards, closing the round-trip break. |
| Bookmark single-URL dedup | `internal/connector/bookmarks/dedup.go:184-189` | `SELECT EXISTS(… WHERE source_id = 'bookmarks' AND source_ref = $1)` always returns `false`. |
| Personal-context sensitivity query | `internal/knowledge/sensitivity_query.go:116` | Degrades safely via `COALESCE` to `source_url`. **Not a blocker** — recorded for post-fix review only. |

## Expected Behavior

`artifacts.source_ref` is persisted for every connector-produced artifact, carrying the connector-supplied stable external identifier; the ingestion dedup probe compares `source_ref` against `source_ref` scoped by `source_id`; and `idx_artifacts_source (source_id, source_ref)` becomes a live, discriminating index. Full expected-behaviour contract and Gherkin scenarios: [spec.md](spec.md).

## Actual Behavior

`artifacts.source_ref` is `NULL` for all 17 registered connectors. Source-ref dedup is structurally unreachable; ingestion dedup is content-hash-only. The GuestHost booking-context endpoint and both bookmark dedup paths match zero rows unconditionally. `idx_artifacts_source` is dead for connector traffic.

## Environment

- Service: `smackerel-core` (`internal/pipeline`, `internal/connector`, `internal/api`)
- Version: repository `HEAD` at packet authoring time (`specs/019-connector-wiring` parent status `done`)
- Platform: Docker Compose stack (`./smackerel.sh up`), PostgreSQL + pgvector
- Release train: `mvp` (`config/release-trains.yaml`, `phase: active`, `target_slot: self-hosted`)

## Verification Evidence (static, planning-mode)

Each claim below was confirmed by direct inspection at the cited coordinate. **Claim Source: `executed`** (read-only source inspection; no runtime execution).

| Claim | Coordinate | Confirmed |
|---|---|---|
| Ingestion `INSERT` omits `source_ref` | `internal/pipeline/ingest.go:94` | Yes — column list enumerated above contains no `source_ref` |
| Processor `INSERT` omits `source_ref` | `internal/pipeline/processor.go:523` | Yes |
| Dedup probe filters `source_url`, binds `SourceRef` | `internal/pipeline/ingest.go:51` | Yes |
| Only writers of `artifacts.source_ref` | `internal/connector/photos/store.go:124,197,363`; `internal/drive/scan/service.go:316` | Yes — repo-wide `source_ref` scan over `internal/` + `cmd/` returns no other `artifacts` writer |
| Column declared | `internal/db/migrations/001_initial_schema.sql:29` | Yes — `source_ref TEXT,` |
| Index declared | `internal/db/migrations/001_initial_schema.sql:68` | Yes — `idx_artifacts_source ON artifacts(source_id, source_ref)` |
| Booking-context consumer | `internal/api/context.go:284` | Yes — `AND source_ref = $1` |
| Bookmark dedup consumers | `internal/connector/bookmarks/dedup.go:131,132,185` | Yes |
| Sensitivity query degrades safely | `internal/knowledge/sensitivity_query.go:116` | Yes — `COALESCE(source_url, COALESCE(source_ref, ''))` |
| GuestHost `sourceRef = event.ID`; `EntityID` discarded | `internal/connector/guesthost/normalizer.go:27`; `internal/connector/guesthost/types.go:10` | Yes |
| 17 connectors registered through one loop | `cmd/core/connectors.go:73-79` | Yes — 17 constructors in the `[]connector.Connector` literal |
| Supervisor routes every item through the front door | `internal/connector/supervisor.go:362` | Yes |
| Documented inventory says 17 | `docs/smackerel.md:2945` | Yes — "Committed Connector Inventory (17 connectors)" |
| `BUG-016-W3` false premise | `specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/bug.md:42,78` | Yes |
| `location_clusters` is a separate table | `internal/db/migrations/001_initial_schema.sql:322-323` | Yes — `CREATE TABLE … location_clusters (id TEXT PRIMARY KEY, source_ref TEXT NOT NULL, …)` |

## Reproduction Plan (NOT executed in planning mode)

Gate G073 forbids running the fix or its tests in this mode. The commands below are the exact sequence a fixer must run to reproduce the defect at runtime **before** changing any source. Expected pre-fix outcomes are stated so a fixer can distinguish "reproduced" from "environment problem".

**Step 1 — bring up an ephemeral test stack**

```bash
./smackerel.sh config generate
./smackerel.sh up
./smackerel.sh status
```

**Step 2 — ingest one artifact through the shared front door via a connector-backed path**

Use the Chrome-extension bridge ingest endpoint (`docs/API.md` §Chrome Extension Bridge Ingestion, line 274) or the manual-ingest endpoint (`docs/API.md` §Manual Ingest, line 170), whichever the fixer's harness already exercises. Both route through `PublishRawArtifact`.

**Step 3 — assert the persistence defect (D1)**

```bash
./smackerel.sh logs
docker exec -i "$(docker ps --filter name=postgres --format '{{.Names}}')" \
  psql -U smackerel -d smackerel -c \
  "SELECT source_id, source_ref IS NULL AS ref_is_null, count(*) FROM artifacts GROUP BY 1,2 ORDER BY 1;"
```

> **Expected pre-fix:** every row reports `ref_is_null = t`. Any row with `ref_is_null = f` is from `photos` or `drive/scan` (the two bespoke writers) and does not falsify D1.

**Step 4 — assert the dead index (D1 corollary)**

```bash
docker exec -i "$(docker ps --filter name=postgres --format '{{.Names}}')" \
  psql -U smackerel -d smackerel -c \
  "EXPLAIN ANALYZE SELECT id FROM artifacts WHERE source_id = 'bookmarks' AND source_ref = 'https://example.test/a';"
```

> **Expected pre-fix:** zero rows; `idx_artifacts_source` yields no matches because the second key column is universally NULL.

**Step 5 — assert the dedup mis-probe (D2)**

Ingest the *same* external item twice with an unchanged `SourceRef` but *modified content* (so `content_hash` differs), then:

```bash
docker exec -i "$(docker ps --filter name=postgres --format '{{.Names}}')" \
  psql -U smackerel -d smackerel -c \
  "SELECT source_id, count(*) FROM artifacts WHERE source_id = '<connector-id>' GROUP BY 1;"
```

> **Expected pre-fix:** count is `2` — the second ingest was NOT deduped, proving source-ref dedup never fired.

**Step 6 — assert the broken booking-context consumer**

```bash
curl --max-time 5 -i "http://127.0.0.1:<core-host-port>/api/v1/context?bookingId=<known-booking-id>"
```

> **Expected pre-fix:** the handler surfaces the `pgx.ErrNoRows` path from `internal/api/context.go:289` for every input, including a booking that demonstrably exists in `artifacts`.

**Step 7 — capture the failing regression test (scenario-first)**

Write the adversarial regression fixture described in [scopes.md → Scope 2 Test Plan](scopes.md#scope-2--restore-source_ref-persistence-and-correct-the-dedup-probe) and run it **before** any source change:

```bash
./smackerel.sh test integration
```

> **Expected pre-fix:** the new `source_ref` persistence/dedup assertions FAIL. A regression test that passes before the fix is not testing this bug and must be rewritten.

## Root Cause

Recorded in [design.md → Root Cause Analysis](design.md#root-cause-analysis).

## Routing

| Item | Owner | Action |
|---|---|---|
| `F-BUG-019-003-003` (D3 identity semantics) | `specs/013-guesthost-connector` owner + API contract owner | **DECIDED as Option 4 and RATIFIED 2026-07-29.** `source_ref = event.EntityID` for booking event types, `event.ID` retained in `metadata`, coupled to Re-ingestion Option B. Options and justification in [design.md](design.md#d3-identity-semantics-decision-decided--option-4). Ratifier: owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts; recorded by `bubbles.bug` on behalf of the operator. Tracked as `TR-BUG-019-003-001` (`resolved`); recorded in [uservalidation.md](uservalidation.md) as `OR-01`. |
| `F-BUG-019-003-004` (`BUG-016-W3` false premise) | `specs/016-weather-connector` owner | **ACCEPTED AS ADVISORY 2026-07-29 — scheduled follow-up only.** This packet does not reopen or modify `BUG-016-W3`, and nothing under `specs/016-weather-connector/` was touched. **After** BUG-019-003 is fixed, a premise-correction annotation is to be appended to that packet recording that the original dedup-collision rationale was not operative at the time. Its shipped fix remains independently valid; **no re-certification is implied or authorised.** Ratifier: owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts; recorded by `bubbles.bug` on behalf of the operator. Tracked as `TR-BUG-019-003-002` (`resolved`); recorded in [uservalidation.md](uservalidation.md) as `OR-02`. |

## Related

- Feature: `specs/019-connector-wiring/`
- Sibling bugs: `specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap/`, `specs/019-connector-wiring/bugs/BUG-019-002-scn-019-004-005-fidelity-gap/`
- Corroborating (foreign, do not modify): `specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/`
- Affected connector specs (consumers, not owners): `specs/013-guesthost-connector`, bookmarks connector surface

## Planning-Mode Boundary Statement

This packet was produced under `product-to-planning` with Gate G073 (Source Code Edit Lockout) ACTIVE.

- **Files created:** the 8 artifacts in this bug folder only.
- **Files modified:** none.
- **Source, migration, config, test, and documentation files edited:** **zero**.
- **Tests executed:** none. All runtime verification is recorded as a plan, never as a result.
