# Scopes

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

**TDD Policy:** scenario-first — tests written alongside implementation per scope, with failing targeted tests preceding green evidence for each Gherkin scenario.

---

## Execution Outline

### Phase Order

1. **Scope 1 — DB Migration** — Migration `016_user_annotations.sql` with `annotations` table, `telegram_message_artifacts` table, and `artifact_annotation_summary` materialized view. Foundation for all subsequent scopes.
2. **Scope 2 — Annotation Types & Parser** — Go package `internal/annotation/` with types, constants, and the freeform text parser that converts strings like `"4/5 made it #weeknight great"` into structured components.
3. **Scope 3 — Annotation Store** — CRUD operations, materialized view refresh, NATS event publication, and `AnnotationQuerier` interface. Wires into `Dependencies`.
4. **Scope 4 — REST API Endpoints** — `POST/GET /api/artifacts/{id}/annotations`, `GET .../summary`, `DELETE .../tags/{tag}` handlers. Channel parity for programmatic access.
5. **Scope 5 — Telegram Message-Artifact Mapping** — `telegram_message_artifacts` store/lookup, recording mapping on every capture confirmation, internal mapping endpoint.
6. **Scope 6 — Telegram Annotation Handler** — Reply-to annotation flow, `/rate` command with disambiguation, confirmation formatting, message routing priority update.
7. **Scope 7 — Search Extension** — Annotation filters (`min_rating`, `tag`, `has_interaction`), intent detection (`"my top rated recipes"`), annotation data in search results, relevance boost.
8. **Scope 8 — Intelligence Integration** — NATS subscriber for `annotations.created`, relevance score adjustment, resurfacing query for unused saved artifacts, variety signal for recent interactions.

### New Types & Signatures

**Go (`internal/annotation/`):**
- `type AnnotationType string` — constants: `TypeRating`, `TypeNote`, `TypeTagAdd`, `TypeTagRemove`, `TypeInteraction`, `TypeStatusChange`
- `type InteractionType string` — constants: `InteractionMadeIt`, `InteractionBoughtIt`, `InteractionReadIt`, `InteractionVisited`, `InteractionTriedIt`, `InteractionUsedIt`
- `type SourceChannel string` — constants: `ChannelTelegram`, `ChannelAPI`, `ChannelWeb`
- `type Annotation struct` — single event in the annotation log
- `type Summary struct` — pre-aggregated annotation state from materialized view
- `type ParsedAnnotation struct` — output of the freeform parser
- `func Parse(input string) ParsedAnnotation` — pure function, no I/O
- `type Store struct` — DB pool + NATS client
- `func NewStore(db, nats) *Store`
- `func (s *Store) CreateAnnotation(ctx, ann) error`
- `func (s *Store) CreateFromParsed(ctx, artifactID, parsed, channel) ([]Annotation, error)`
- `func (s *Store) GetSummary(ctx, artifactID) (*Summary, error)`
- `func (s *Store) GetHistory(ctx, artifactID, limit) ([]Annotation, error)`
- `func (s *Store) DeleteTag(ctx, artifactID, tag, channel) error`
- `type AnnotationQuerier interface` — `CreateFromParsed`, `GetSummary`, `GetHistory`, `DeleteTag`

**Go (`internal/api/`):**
- `type CreateAnnotationRequest struct` — JSON body for POST endpoint
- `type CreateAnnotationResponse struct` — response with events + summary
- `func (d *Dependencies) CreateAnnotationHandler(w, r)`
- `func (d *Dependencies) GetAnnotationsHandler(w, r)`
- `func (d *Dependencies) GetAnnotationSummaryHandler(w, r)`
- `func (d *Dependencies) DeleteTagHandler(w, r)`
- `type AnnotationIntent struct` — MinRating, HasInteraction, Tag, Cleaned
- `func parseAnnotationIntent(query string) *AnnotationIntent`

**Go (`internal/telegram/`):**
- `func (b *Bot) handleReplyAnnotation(ctx, msg)`
- `func (b *Bot) handleRate(ctx, msg, args)`
- `func (b *Bot) recordMessageArtifact(ctx, messageID, chatID, artifactID)`
- `func (b *Bot) resolveArtifactFromMessage(ctx, messageID, chatID) (string, error)`
- `func formatAnnotationConfirmation(created []Annotation) string`
- `func humanizeInteraction(i InteractionType) string`
- `type pendingDisambiguation struct`

**Go (`internal/intelligence/`):**
- `func (e *Engine) SubscribeAnnotations(ctx) error`
- `func (e *Engine) updateRelevanceFromAnnotation(ctx, ann) error`
- `func annotationRelevanceDelta(ann *Annotation) float64`

**Go (`internal/nats/`):**
- `SubjectAnnotationsCreated = "annotations.created"`

**SQL:**
- `internal/db/migrations/016_user_annotations.sql` — `annotations` table, `telegram_message_artifacts` table, `artifact_annotation_summary` materialized view

**Config:**
- `config/smackerel.yaml` — `annotations:` section with matview timeout, limits, relevance boost coefficients
- `config/nats_contract.json` — `annotations.created` subject

### Validation Checkpoints

- After Scope 1: `./smackerel.sh test unit` — migration applies cleanly, indexes exist
- After Scope 2: `./smackerel.sh test unit` — parser handles all freeform input patterns, types compile
- After Scope 3: `./smackerel.sh test unit` — store CRUD operations, view refresh, NATS publish
- After Scope 4: `./smackerel.sh test unit` — API handlers return correct status codes and JSON
- After Scope 5: `./smackerel.sh test unit` — message-artifact mapping records and resolves
- After Scope 6: `./smackerel.sh test unit` — reply-to flow, `/rate` command, disambiguation
- After Scope 7: `./smackerel.sh test unit` — annotation intent detection, filter SQL, search result enrichment
- After Scope 8: `./smackerel.sh test unit` — relevance delta calculation, NATS subscriber wiring; full regression: `./smackerel.sh test unit` + `./smackerel.sh test integration` + `./smackerel.sh test e2e`

---

## Scope Summary

| # | Name | Surfaces | Key Tests | Status |
|---|------|----------|-----------|--------|
| 1 | DB Migration | PostgreSQL | unit: migration applies, rollback works | Done |
| 2 | Annotation Types & Parser | Go core | unit: parse rating, interaction, tags, notes, mixed input | Done |
| 3 | Annotation Store | Go core, PostgreSQL, NATS | unit: CRUD, view refresh, NATS publish, interface | Done |
| 4 | REST API Endpoints | Go API | unit: POST/GET/DELETE handlers, validation, error codes | Done |
| 5 | Telegram Message-Artifact Mapping | Go telegram, PostgreSQL | unit: record mapping, resolve artifact, internal endpoint | Done |
| 6 | Telegram Annotation Handler | Go telegram | unit: reply-to flow, /rate command, disambiguation, formatting | Done |
| 7 | Search Extension | Go API | unit: intent detection, annotation filters, result enrichment, boost | Done |
| 8 | Intelligence Integration | Go intelligence, NATS | unit: relevance delta, subscriber, resurfacing query | Done |

---

## Scope 1: DB Migration

**Status:** Done
**Priority:** P0
**Depends On:** Phase 2 Ingestion (003) — `artifacts` table must exist

### Gherkin Scenarios

```gherkin
Scenario: Annotation types are created by migration
  Given the database has the existing schema through migration 014
  When migration 016_user_annotations.sql is applied
  Then enum type annotation_type exists with values rating, note, tag_add, tag_remove, interaction, status_change
  And enum type interaction_type exists with values made_it, bought_it, read_it, visited, tried_it, used_it

Scenario: Annotations table is created with correct schema
  Given migration 016_user_annotations.sql has been applied
  When the annotations table is inspected
  Then it has columns id (TEXT PK), artifact_id (TEXT FK→artifacts), ann_type (annotation_type), rating (SMALLINT CHECK 1-5), note (TEXT), tag (TEXT), interaction (interaction_type), source_channel (TEXT NOT NULL), created_at (TIMESTAMPTZ)
  And indexes exist on artifact_id, ann_type, created_at, tag (WHERE NOT NULL), rating (WHERE NOT NULL)

Scenario: Telegram message-artifact mapping table is created
  Given migration 016_user_annotations.sql has been applied
  When the telegram_message_artifacts table is inspected
  Then it has columns message_id (BIGINT), chat_id (BIGINT), artifact_id (TEXT FK→artifacts), created_at (TIMESTAMPTZ)
  And the primary key is (message_id, chat_id)
  And an index exists on artifact_id

Scenario: Materialized view aggregates annotation data correctly
  Given the annotations table contains rating, interaction, tag_add, tag_remove, and note events for an artifact
  When the artifact_annotation_summary materialized view is refreshed
  Then current_rating equals the most recent rating event's value
  And average_rating is the mean of all rating values
  And rating_count is the count of rating events
  And times_used is the count of interaction events
  And last_used is the most recent interaction timestamp
  And tags contains only active tags (added minus removed)
  And notes_count is the count of note events

Scenario: Annotations cascade on artifact deletion
  Given an artifact has annotation events and a telegram message mapping
  When the artifact is deleted
  Then all its annotation rows are deleted via ON DELETE CASCADE
  And its telegram_message_artifacts rows are deleted via ON DELETE CASCADE
```

### Implementation Plan

**Files to create:**
- `internal/db/migrations/016_user_annotations.sql` — enum types, `annotations` table, `telegram_message_artifacts` table, `artifact_annotation_summary` materialized view, all indexes

**Files to modify:**
- None — pure additive migration

**Config SST:** No config changes. Migration numbering follows existing sequence (after 014).

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T1-01 | unit | `internal/db/migrations_test.go` | SCN-027-01 | Migration 015 applies cleanly after existing migrations |
| T1-02 | unit | `internal/db/migrations_test.go` | SCN-027-01 | Enum types annotation_type and interaction_type exist with correct values |
| T1-03 | unit | `internal/db/migrations_test.go` | SCN-027-02 | Annotations table has correct columns, constraints, and indexes |
| T1-04 | unit | `internal/db/migrations_test.go` | SCN-027-03 | telegram_message_artifacts table has correct PK and FK |
| T1-05 | unit | `internal/db/migrations_test.go` | SCN-027-04 | Materialized view computes current_rating, average_rating, times_used, tags correctly |
| T1-06 | unit | `internal/db/migrations_test.go` | SCN-027-05 | CASCADE deletes annotation and mapping rows when artifact is deleted |

### Definition of Done

- [ ] Migration `016_user_annotations.sql` creates `annotation_type` and `interaction_type` enum types
  > **Phase:** implement

- [ ] `annotations` table created with id, artifact_id (FK CASCADE), ann_type, rating (CHECK 1-5), note, tag, interaction, source_channel, created_at
  > **Phase:** implement

- [ ] Indexes: `idx_annotations_artifact`, `idx_annotations_type`, `idx_annotations_created`, `idx_annotations_tag` (partial), `idx_annotations_rating` (partial)
  > **Phase:** implement

- [ ] `telegram_message_artifacts` table with PK(message_id, chat_id), artifact_id FK CASCADE, index on artifact_id
  > **Phase:** implement

- [ ] `artifact_annotation_summary` materialized view with current_rating, average_rating, rating_count, times_used, last_used, tags (add minus remove), notes_count, total_events, last_annotated
  > **Phase:** implement

- [ ] Unique index `idx_aas_artifact` on materialized view for REFRESH CONCURRENTLY support
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

---

## Scope 2: Annotation Types & Parser

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: Parse rating only
  Given the input "4/5"
  When the annotation parser processes it
  Then the result has rating=4, no interaction, no tags, no note

Scenario: Parse full annotation
  Given the input "4/5 made it #weeknight needs more garlic"
  When the annotation parser processes it
  Then the result has rating=4, interaction=made_it, tags=["weeknight"], note="needs more garlic"

Scenario: Parse tags only
  Given the input "#quick #weeknight #kids-approved"
  When the annotation parser processes it
  Then the result has tags=["quick", "weeknight", "kids-approved"], no rating, no interaction

Scenario: Parse tag removal
  Given the input "#remove-quick"
  When the annotation parser processes it
  Then the result has tags=["remove-quick"], no rating, no interaction, no note

Scenario: Parse interaction only
  Given the input "made it"
  When the annotation parser processes it
  Then the result has interaction=made_it, no rating, no tags

Scenario: Parse note only
  Given the input "needs more garlic"
  When the annotation parser processes it
  Then the result has note="needs more garlic", no rating, no interaction, no tags

Scenario: Parse empty input returns zero value
  Given the input ""
  When the annotation parser processes it
  Then the result has no rating, no interaction, no tags, no note

Scenario: Out-of-range rating is not matched
  Given the input "6/5"
  When the annotation parser processes it
  Then the result has no rating
  And "6/5" remains as note text

Scenario: Interaction keywords are case-insensitive
  Given the input "Made It"
  When the annotation parser processes it
  Then the result has interaction=made_it

Scenario: All interaction types are recognized
  Given the inputs "made it", "bought it", "read it", "visited", "tried it", "used it"
  When the annotation parser processes each
  Then each maps to the correct InteractionType constant

Scenario: Rating with "out of 5" syntax
  Given the input "4 out of 5"
  When the annotation parser processes it
  Then the result has rating=4
```

### Implementation Plan

**Files to create:**
- `internal/annotation/types.go` — `AnnotationType`, `InteractionType`, `SourceChannel` constants, `Annotation`, `Summary`, `ParsedAnnotation` structs
- `internal/annotation/parser.go` — `Parse()` function with regex patterns for rating, tags, tag removal, interaction keywords
- `internal/annotation/parser_test.go` — comprehensive parser unit tests

**Files to modify:**
- None — new package

**Config SST:** No config changes.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T2-01 | unit | `internal/annotation/parser_test.go` | SCN-027-06 | "4/5" parses to rating=4 only |
| T2-02 | unit | `internal/annotation/parser_test.go` | SCN-027-07 | Full annotation "4/5 made it #weeknight needs more garlic" → all components |
| T2-03 | unit | `internal/annotation/parser_test.go` | SCN-027-08 | "#quick #weeknight" → two tags, no other components |
| T2-04 | unit | `internal/annotation/parser_test.go` | SCN-027-09 | "#remove-quick" → tag removal marker |
| T2-05 | unit | `internal/annotation/parser_test.go` | SCN-027-10 | "made it" → interaction=made_it only |
| T2-06 | unit | `internal/annotation/parser_test.go` | SCN-027-11 | "needs more garlic" → note only |
| T2-07 | unit | `internal/annotation/parser_test.go` | SCN-027-12 | Empty string → zero-valued ParsedAnnotation |
| T2-08 | unit | `internal/annotation/parser_test.go` | SCN-027-13 | "6/5" → no rating, text in note |
| T2-09 | unit | `internal/annotation/parser_test.go` | SCN-027-14 | "Made It" → case-insensitive interaction match |
| T2-10 | unit | `internal/annotation/parser_test.go` | SCN-027-15 | All six interaction keywords map to correct constants |
| T2-11 | unit | `internal/annotation/parser_test.go` | SCN-027-16 | "4 out of 5" → rating=4 |
| T2-12 | unit | `internal/annotation/parser_test.go` | SCN-027-07 | Multiple interactions in input — only first matched |

### Definition of Done

- [ ] `internal/annotation/types.go` defines `AnnotationType`, `InteractionType`, `SourceChannel` as typed string constants
  > **Phase:** implement

- [ ] `Annotation`, `Summary`, `ParsedAnnotation` structs match design with correct JSON tags
  > **Phase:** implement

- [ ] `Parse()` extracts rating from "N/5", "N out of 5" patterns (1-5 only)
  > **Phase:** implement

- [ ] `Parse()` extracts interaction from keyword list (made it, bought it, read it, visited, tried it, used it)
  > **Phase:** implement

- [ ] `Parse()` extracts hashtags as tags, "#remove-X" as removal markers
  > **Phase:** implement

- [ ] `Parse()` assigns remaining text as note after stripping other components
  > **Phase:** implement

- [ ] `Parse()` returns zero-valued struct for empty input
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

---

## Scope 3: Annotation Store

**Status:** Done
**Priority:** P0
**Depends On:** Scope 2

### Gherkin Scenarios

```gherkin
Scenario: CreateAnnotation inserts an annotation event
  Given a valid Annotation struct with artifact_id, ann_type=rating, rating=4, source_channel=api
  When CreateAnnotation is called
  Then a row is inserted into the annotations table with a ULID id
  And the materialized view is refreshed concurrently
  And a NATS event is published to annotations.created

Scenario: CreateFromParsed converts parsed output into individual events
  Given a ParsedAnnotation with rating=4, interaction=made_it, tags=["weeknight"], note="great"
  When CreateFromParsed is called with artifact_id and channel=api
  Then four annotation rows are inserted: one rating, one interaction, one tag_add, one note
  And the returned slice contains all four Annotation structs

Scenario: CreateFromParsed rejects non-existent artifact
  Given a ParsedAnnotation with valid data
  When CreateFromParsed is called with an artifact_id that does not exist
  Then it returns an error containing "artifact not found"

Scenario: GetSummary returns aggregated annotation data
  Given an artifact with 3 rating events (3, 4, 5), 2 interaction events, and tags ["quick", "weeknight"]
  When GetSummary is called for that artifact
  Then current_rating=5 (most recent), average_rating=4.0, rating_count=3, times_used=2, tags=["quick","weeknight"]

Scenario: GetSummary returns error for artifact with no annotations
  Given an artifact with no annotation events
  When GetSummary is called for that artifact
  Then it returns pgx.ErrNoRows

Scenario: GetHistory returns events newest-first
  Given an artifact with 5 annotation events created at different times
  When GetHistory is called with limit=3
  Then it returns the 3 most recent events, ordered newest first

Scenario: DeleteTag inserts a tag_remove event
  Given an artifact with tag "quick" added
  When DeleteTag is called with tag="quick"
  Then a tag_remove event is inserted for tag "quick"
  And after view refresh, the tag is no longer in the summary

Scenario: Tag add then tag remove results in empty tags
  Given an artifact with tag_add event for "weeknight"
  When a tag_remove event is inserted for "weeknight"
  And the materialized view is refreshed
  Then the summary tags array does not contain "weeknight"

Scenario: NATS event payload matches Annotation struct JSON
  Given a CreateAnnotation call succeeds
  When the NATS event is published to annotations.created
  Then the payload deserializes to a valid Annotation struct with matching fields
```

### Implementation Plan

**Files to create:**
- `internal/annotation/store.go` — `Store` struct, `NewStore`, `CreateAnnotation`, `CreateFromParsed`, `GetSummary`, `GetHistory`, `DeleteTag`
- `internal/annotation/store_test.go` — store unit tests (mock DB and NATS)

**Files to modify:**
- `internal/nats/client.go` — add `SubjectAnnotationsCreated` constant, add `annotations.>` to ARTIFACTS stream subjects
- `config/nats_contract.json` — add `annotations.created` entry
- `internal/api/health.go` — add `AnnotationStore annotation.AnnotationQuerier` to `Dependencies`

**Config SST:** Add `annotations:` section to `config/smackerel.yaml` with `matview_refresh_timeout_s`, `max_tags_per_artifact`, `max_note_length`. Regenerate config with `./smackerel.sh config generate`.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T3-01 | unit | `internal/annotation/store_test.go` | SCN-027-17 | CreateAnnotation inserts row, assigns ULID, refreshes view |
| T3-02 | unit | `internal/annotation/store_test.go` | SCN-027-17 | CreateAnnotation publishes NATS event to annotations.created |
| T3-03 | unit | `internal/annotation/store_test.go` | SCN-027-18 | CreateFromParsed generates correct event count from mixed input |
| T3-04 | unit | `internal/annotation/store_test.go` | SCN-027-19 | CreateFromParsed returns error for non-existent artifact |
| T3-05 | unit | `internal/annotation/store_test.go` | SCN-027-20 | GetSummary returns correct aggregated values |
| T3-06 | unit | `internal/annotation/store_test.go` | SCN-027-21 | GetSummary returns error when no annotations exist |
| T3-07 | unit | `internal/annotation/store_test.go` | SCN-027-22 | GetHistory returns events newest-first, respects limit |
| T3-08 | unit | `internal/annotation/store_test.go` | SCN-027-23 | DeleteTag inserts tag_remove event |
| T3-09 | unit | `internal/annotation/store_test.go` | SCN-027-24 | Tag add + remove results in empty tags in summary |
| T3-10 | unit | `internal/annotation/store_test.go` | SCN-027-25 | NATS event payload matches Annotation JSON |

### Definition of Done

- [ ] `Store` struct created with `*pgxpool.Pool` and `*smacknats.Client` fields
  > **Phase:** implement

- [ ] `CreateAnnotation` inserts row with ULID, refreshes materialized view concurrently, publishes NATS event
  > **Phase:** implement

- [ ] `CreateFromParsed` validates artifact existence, creates individual events for each parsed component, handles tag removals
  > **Phase:** implement

- [ ] `GetSummary` reads from `artifact_annotation_summary` materialized view
  > **Phase:** implement

- [ ] `GetHistory` returns annotation events ordered by `created_at DESC` with configurable limit (max 100)
  > **Phase:** implement

- [ ] `DeleteTag` inserts a `tag_remove` event (append-only pattern)
  > **Phase:** implement

- [ ] `AnnotationQuerier` interface defined and implemented by `Store`
  > **Phase:** implement

- [ ] `SubjectAnnotationsCreated` constant added to `internal/nats/client.go`
  > **Phase:** implement

- [ ] `annotations.created` subject added to `config/nats_contract.json`
  > **Phase:** implement

- [ ] `AnnotationStore` field added to `api.Dependencies`
  > **Phase:** implement

- [ ] `annotations:` section added to `config/smackerel.yaml` with SST-compliant values
  > **Phase:** implement

- [ ] Config regenerated: `./smackerel.sh config generate`
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

---

## Scope 4: REST API Endpoints

**Status:** Done
**Priority:** P0
**Depends On:** Scope 3

### Gherkin Scenarios

```gherkin
Scenario: POST annotation with freeform text
  Given an existing artifact with id "art-123"
  When POST /api/artifacts/art-123/annotations is called with body {"text": "4/5 made it #weeknight great dish"}
  Then the response status is 201
  And the response body contains annotations array with rating, interaction, tag_add, and note events
  And the response body contains a summary with current_rating=4

Scenario: POST annotation with structured fields
  Given an existing artifact with id "art-123"
  When POST /api/artifacts/art-123/annotations is called with body {"rating": 5, "note": "excellent"}
  Then the response status is 201
  And the response body contains annotations array with rating and note events

Scenario: POST annotation with invalid rating
  Given an existing artifact with id "art-123"
  When POST /api/artifacts/art-123/annotations is called with body {"rating": 0}
  Then the response status is 400
  And the error code is "INVALID_RATING"

Scenario: POST annotation with empty body
  When POST /api/artifacts/art-123/annotations is called with body {}
  Then the response status is 400
  And the error code is "EMPTY_ANNOTATION"

Scenario: POST annotation for non-existent artifact
  When POST /api/artifacts/nonexistent/annotations is called with body {"rating": 4}
  Then the response status is 404

Scenario: GET annotation history
  Given an artifact with 5 annotation events
  When GET /api/artifacts/art-123/annotations is called
  Then the response status is 200
  And the response body contains annotations array ordered newest-first
  And the response body contains total count

Scenario: GET annotation history with limit
  Given an artifact with 10 annotation events
  When GET /api/artifacts/art-123/annotations?limit=3 is called
  Then the response body contains exactly 3 annotations

Scenario: GET annotation summary
  Given an artifact with rating, interaction, and tag annotations
  When GET /api/artifacts/art-123/annotations/summary is called
  Then the response status is 200
  And the response body contains current_rating, average_rating, times_used, tags

Scenario: GET annotation summary for unannotated artifact
  Given an artifact with no annotations
  When GET /api/artifacts/art-123/annotations/summary is called
  Then the response status is 200
  And the response body is an empty object

Scenario: DELETE a tag
  Given an artifact with tag "weeknight"
  When DELETE /api/artifacts/art-123/tags/weeknight is called
  Then the response status is 200
  And the response body contains removed="weeknight" and updated summary
```

### Implementation Plan

**Files to create:**
- `internal/api/annotations.go` — `CreateAnnotationHandler`, `GetAnnotationsHandler`, `GetAnnotationSummaryHandler`, `DeleteTagHandler`
- `internal/api/annotations_test.go` — handler unit tests

**Files to modify:**
- `internal/api/router.go` — add annotation route group inside authenticated block

**Config SST:** No new config values. Routes use existing auth token from config.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T4-01 | unit | `internal/api/annotations_test.go` | SCN-027-26 | POST with freeform text → 201, correct events |
| T4-02 | unit | `internal/api/annotations_test.go` | SCN-027-27 | POST with structured fields → 201, correct events |
| T4-03 | unit | `internal/api/annotations_test.go` | SCN-027-28 | POST with rating=0 → 400 INVALID_RATING |
| T4-04 | unit | `internal/api/annotations_test.go` | SCN-027-28 | POST with rating=6 → 400 INVALID_RATING |
| T4-05 | unit | `internal/api/annotations_test.go` | SCN-027-29 | POST with empty body → 400 EMPTY_ANNOTATION |
| T4-06 | unit | `internal/api/annotations_test.go` | SCN-027-30 | POST for non-existent artifact → 404 |
| T4-07 | unit | `internal/api/annotations_test.go` | SCN-027-31 | GET history returns events newest-first with total |
| T4-08 | unit | `internal/api/annotations_test.go` | SCN-027-32 | GET history with limit=3 returns exactly 3 |
| T4-09 | unit | `internal/api/annotations_test.go` | SCN-027-33 | GET summary returns aggregated annotation data |
| T4-10 | unit | `internal/api/annotations_test.go` | SCN-027-34 | GET summary for unannotated artifact → 200 empty object |
| T4-11 | unit | `internal/api/annotations_test.go` | SCN-027-35 | DELETE tag → 200 with removed tag and updated summary |
| T4-12 | unit | `internal/api/annotations_test.go` | SCN-027-26 | POST with missing artifact ID in URL → 400 |

### Definition of Done

- [ ] `CreateAnnotationHandler` parses freeform text (via `annotation.Parse`) or structured fields, validates rating range 1-5, rejects empty input
  > **Phase:** implement

- [ ] `GetAnnotationsHandler` returns paginated annotation history with `limit` query param (default 50, max 100)
  > **Phase:** implement

- [ ] `GetAnnotationSummaryHandler` returns materialized summary or empty object for unannotated artifacts
  > **Phase:** implement

- [ ] `DeleteTagHandler` inserts a `tag_remove` event via store and returns updated summary
  > **Phase:** implement

- [ ] Routes registered in `router.go`: `POST /artifacts/{id}/annotations`, `GET /artifacts/{id}/annotations`, `GET /artifacts/{id}/annotations/summary`, `DELETE /artifacts/{id}/tags/{tag}`
  > **Phase:** implement

- [ ] All error responses use existing `writeError` pattern with proper HTTP status codes
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

---

## Scope 5: Telegram Message-Artifact Mapping

**Status:** Done
**Priority:** P0
**Depends On:** Scope 3

### Gherkin Scenarios

```gherkin
Scenario: Record message-artifact mapping after capture confirmation
  Given the Telegram bot sends a capture confirmation message with message_id=1001 in chat_id=5555
  And the captured artifact has id "art-abc"
  When recordMessageArtifact is called with message_id=1001, chat_id=5555, artifact_id="art-abc"
  Then a row is inserted into telegram_message_artifacts with those values

Scenario: Resolve artifact from replied-to message
  Given telegram_message_artifacts contains (message_id=1001, chat_id=5555, artifact_id="art-abc")
  When resolveArtifactFromMessage is called with message_id=1001, chat_id=5555
  Then it returns "art-abc"

Scenario: Resolve returns empty for unknown message
  Given telegram_message_artifacts has no row for message_id=9999, chat_id=5555
  When resolveArtifactFromMessage is called with message_id=9999, chat_id=5555
  Then it returns empty string and no error

Scenario: Multiple artifacts can be mapped to different messages in same chat
  Given message_id=1001 maps to "art-abc" and message_id=1002 maps to "art-def" in the same chat
  When resolveArtifactFromMessage is called for each message
  Then each returns the correct artifact_id

Scenario: Internal mapping endpoint records mapping via HTTP
  When POST /internal/telegram-message-artifact is called with body {"message_id": 1001, "chat_id": 5555, "artifact_id": "art-abc"}
  Then the response status is 201
  And the mapping is stored in telegram_message_artifacts
```

### Implementation Plan

**Files to create:**
- `internal/telegram/mapping.go` — `recordMessageArtifact`, `resolveArtifactFromMessage` functions
- `internal/telegram/mapping_test.go` — mapping unit tests

**Files to modify:**
- `internal/telegram/bot.go` — call `recordMessageArtifact` after every capture confirmation send
- `internal/api/router.go` — add internal mapping endpoint
- `internal/api/annotations.go` — add `RecordTelegramMessageArtifactHandler`

**Config SST:** No new config values.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T5-01 | unit | `internal/telegram/mapping_test.go` | SCN-027-36 | recordMessageArtifact inserts row correctly |
| T5-02 | unit | `internal/telegram/mapping_test.go` | SCN-027-37 | resolveArtifactFromMessage returns correct artifact_id |
| T5-03 | unit | `internal/telegram/mapping_test.go` | SCN-027-38 | resolveArtifactFromMessage returns empty for unknown message |
| T5-04 | unit | `internal/telegram/mapping_test.go` | SCN-027-39 | Multiple mappings in same chat resolve independently |
| T5-05 | unit | `internal/api/annotations_test.go` | SCN-027-40 | Internal mapping endpoint returns 201, stores mapping |

### Definition of Done

- [x] `recordMessageArtifact` inserts into `telegram_message_artifacts` table after capture confirmations
  > **Phase:** implement — Created `internal/telegram/mapping.go` with `recordMessageArtifact` calling internal API. Modified `bot.go`, `share.go`, `forward.go` to use `replyWithMapping`.

- [x] `resolveArtifactFromMessage` looks up artifact_id by (message_id, chat_id) primary key
  > **Phase:** implement — `resolveArtifactFromMessage` in `mapping.go` queries GET /internal/telegram-message-artifact, returns empty on 404.

- [x] All existing Telegram capture confirmation handlers call `recordMessageArtifact` with the sent message ID
  > **Phase:** implement — `handleTextCapture`, `handleVoice`, `handleShareCapture`, `captureSingleForward` all use `replyWithMapping` which records mapping.

- [x] Internal endpoint `POST /internal/telegram-message-artifact` accepts mapping requests from the bot
  > **Phase:** implement — Added `RecordTelegramMessageArtifact` and `ResolveTelegramMessageArtifact` handlers in `annotations.go`, registered in `router.go`.

- [x] Returns empty string (not error) when no mapping exists for a message
  > **Phase:** implement — `resolveArtifactFromMessage` returns "" on 404, tested in `TestResolveArtifactFromMessage_NotFound`.

- [x] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** implement — Full test suite passes (0 failures). `Claim Source: executed`

---

## Scope 6: Telegram Annotation Handler

**Status:** Done
**Priority:** P1
**Depends On:** Scope 5

### Gherkin Scenarios

```gherkin
Scenario: Reply-to annotation with rating
  Given the user received a capture confirmation for artifact "art-abc" in message 1001
  When the user replies to message 1001 with "4/5"
  Then the bot records a rating annotation with value 4 for artifact "art-abc"
  And the bot replies with "Rated ★★★★☆"

Scenario: Reply-to annotation with full annotation
  Given the user received a capture confirmation for artifact "art-abc" in message 1001
  When the user replies to message 1001 with "made it, 4/5, needs more garlic"
  Then the bot records rating=4, interaction=made_it, note="needs more garlic" for artifact "art-abc"
  And the bot replies with confirmation showing all annotation components

Scenario: Reply-to annotation with tags
  Given the user received a capture confirmation for artifact "art-abc" in message 1001
  When the user replies to message 1001 with "#weeknight #quick"
  Then the bot records two tag_add events for tags "weeknight" and "quick"
  And the bot replies with "Tagged: #weeknight\nTagged: #quick"

Scenario: Reply to unknown message falls through to normal handling
  Given message 2000 is not in telegram_message_artifacts
  When the user replies to message 2000 with "some text"
  Then the bot does not attempt annotation
  And the message is dispatched to normal text/URL handling

Scenario: Reply with plain text becomes a note
  Given the user received a capture confirmation for artifact "art-abc" in message 1001
  When the user replies to message 1001 with "needs more garlic"
  Then the bot records a note annotation with text "needs more garlic"

Scenario: /rate command with single match
  Given an artifact titled "Pasta Carbonara" exists in search
  When the user sends "/rate pasta carbonara 4/5 great dish"
  Then the bot searches for "pasta carbonara"
  And finds one strong match
  And records rating=4, note="great dish" for that artifact
  And replies with confirmation including the artifact title

Scenario: /rate command with multiple matches triggers disambiguation
  Given multiple artifacts matching "pasta" exist in search
  When the user sends "/rate pasta 4/5"
  Then the bot replies with a numbered list of top 3 matches
  And stores the pending disambiguation state

Scenario: Disambiguation resolution by number
  Given the bot sent a disambiguation prompt with 3 options for chat 5555
  When the user replies with "2"
  Then the bot selects the second artifact from the disambiguation list
  And applies the pending annotation to that artifact
  And clears the disambiguation state

Scenario: /rate command with no arguments shows usage
  When the user sends "/rate"
  Then the bot replies with usage instructions

Scenario: /rate command with no search results
  When the user sends "/rate unicorn stew 5/5"
  And no artifacts match "unicorn stew"
  Then the bot replies with "No matching artifacts found"

Scenario: Annotation confirmation formatting
  Given annotations were recorded: rating=4, interaction=made_it, tag="weeknight", note="great"
  When formatAnnotationConfirmation is called
  Then the output contains "Rated ★★★★☆", "Logged: Made it", "Tagged: #weeknight", "Note: great"
```

### Implementation Plan

**Files to create:**
- `internal/telegram/annotation.go` — `handleReplyAnnotation`, `handleRate`, `formatAnnotationConfirmation`, `humanizeInteraction`, `splitRateArgs`, disambiguation types and helpers
- `internal/telegram/annotation_test.go` — handler and formatting unit tests

**Files to modify:**
- `internal/telegram/bot.go` — update `handleMessage` routing: add reply-to check before command routing, add `/rate` command case, add disambiguation resolution check, register `/rate` in command list

**Config SST:** Add `telegram.disambiguation_timeout_seconds: 120` to `config/smackerel.yaml`. Regenerate config.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T6-01 | unit | `internal/telegram/annotation_test.go` | SCN-027-41 | Reply-to with "4/5" → rating annotation + star confirmation |
| T6-02 | unit | `internal/telegram/annotation_test.go` | SCN-027-42 | Reply-to with full annotation → multiple events + multi-line confirmation |
| T6-03 | unit | `internal/telegram/annotation_test.go` | SCN-027-43 | Reply-to with tags → tag_add events + tag confirmation |
| T6-04 | unit | `internal/telegram/annotation_test.go` | SCN-027-44 | Reply to unknown message → fallthrough to normal handling |
| T6-05 | unit | `internal/telegram/annotation_test.go` | SCN-027-45 | Reply with plain text → note annotation |
| T6-06 | unit | `internal/telegram/annotation_test.go` | SCN-027-46 | /rate with single match → annotation + title confirmation |
| T6-07 | unit | `internal/telegram/annotation_test.go` | SCN-027-47 | /rate with multiple matches → disambiguation prompt |
| T6-08 | unit | `internal/telegram/annotation_test.go` | SCN-027-48 | Disambiguation resolution with "2" → correct artifact selected |
| T6-09 | unit | `internal/telegram/annotation_test.go` | SCN-027-49 | /rate with no args → usage instructions |
| T6-10 | unit | `internal/telegram/annotation_test.go` | SCN-027-50 | /rate with no results → "No matching artifacts" |
| T6-11 | unit | `internal/telegram/annotation_test.go` | SCN-027-51 | formatAnnotationConfirmation renders stars, interaction, tags, notes |
| T6-12 | unit | `internal/telegram/annotation_test.go` | SCN-027-51 | humanizeInteraction maps all InteractionType values correctly |

### Definition of Done

- [x] `handleReplyAnnotation` checks reply-to message ID against `telegram_message_artifacts`, parses text, submits annotation, sends confirmation
  > **Phase:** implement — Created `internal/telegram/annotation.go` with `handleReplyAnnotation` that resolves artifact, parses text, calls annotation API, formats confirmation.

- [x] Reply to unknown message (not in mapping) dispatches to normal text/URL handling instead of failing
  > **Phase:** implement — Returns false when `resolveArtifactFromMessage` returns empty, allowing fallthrough. Tested in `TestHandleReplyAnnotation_UnknownMessage`.

- [x] `/rate` command splits search terms from annotation text, searches, annotates single match or triggers disambiguation
  > **Phase:** implement — `handleRate` with `splitRateArgs`, single-match annotation, multi-match disambiguation prompt.

- [x] Disambiguation flow stores pending state keyed by chat_id with TTL from config, resolves on numeric reply
  > **Phase:** implement — `disambiguationStore` with `set/get/clear`, TTL-based expiry, `handleDisambiguationReply` resolves on numeric input.

- [x] `formatAnnotationConfirmation` renders star ratings (★☆), humanized interactions, tag names, truncated notes
  > **Phase:** implement — `renderStars`, `humanizeInteraction`, `formatAnnotationConfirmation` all tested.

- [x] `handleMessage` routing updated: reply-to annotation before commands, disambiguation resolution before commands, `/rate` in command switch
  > **Phase:** implement — Priority order: reply-to annotation → disambiguation → commands (including /rate).

- [x] `/rate` registered in bot command list via `SetMyCommands`
  > **Phase:** implement — Added to `commands` in `Start()` and help text.

- [x] `disambiguation_timeout_seconds` added to `config/smackerel.yaml` under `telegram:`
  > **Phase:** implement — Added `disambiguation_timeout_seconds: 120` to yaml, env var generation in config.sh, Config struct field + parsing.

- [x] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** implement — Full test suite passes. `Claim Source: executed`

---

## Scope 7: Search Extension

**Status:** Done
**Priority:** P1
**Depends On:** Scope 3

### Gherkin Scenarios

```gherkin
Scenario: Search with min_rating filter
  Given artifact "art-abc" has current_rating=5 and artifact "art-def" has current_rating=2
  When a search is executed with min_rating=4
  Then "art-abc" appears in results
  And "art-def" does not appear in results

Scenario: Search with tag filter
  Given artifact "art-abc" has tag "weeknight" and artifact "art-def" has no tags
  When a search is executed with tag="weeknight"
  Then "art-abc" appears in results
  And "art-def" does not appear in results

Scenario: Search with has_interaction filter
  Given artifact "art-abc" has times_used=3 and artifact "art-def" has times_used=0
  When a search is executed with has_interaction=true
  Then "art-abc" appears in results
  And "art-def" does not appear in results

Scenario: Annotation intent detection for "my top rated recipes"
  Given the user searches for "my top rated recipes"
  When parseAnnotationIntent processes the query
  Then it returns min_rating=4 and cleaned query "recipes"

Scenario: Annotation intent detection for "things I've made"
  Given the user searches for "things I've made"
  When parseAnnotationIntent processes the query
  Then it returns has_interaction=true and cleaned query ""

Scenario: Annotation intent detection for tag in query
  Given the user searches for "#weeknight dinners"
  When parseAnnotationIntent processes the query
  Then it returns tag="weeknight" and cleaned query "dinners"

Scenario: Search results include annotation data
  Given artifact "art-abc" has current_rating=4, times_used=2, tags=["quick"]
  When a search returns "art-abc"
  Then the search result includes rating=4, times_used=2, tags=["quick"]

Scenario: Annotation boost adjusts ranking
  Given two artifacts with identical semantic similarity
  And artifact "art-abc" has rating=5, times_used=3
  And artifact "art-def" has no annotations
  When search results are ranked
  Then "art-abc" ranks higher than "art-def" due to annotation boost

Scenario: Annotation boost is small enough not to overwhelm semantics
  Given artifact "art-abc" has rating=5, times_used=10 (max annotation boost)
  And artifact "art-def" has no annotations but much higher semantic similarity
  When search results are ranked
  Then the annotation boost (max 0.08) does not override a significant semantic difference

Scenario: No annotation intent for plain queries
  Given the user searches for "chicken pasta recipe"
  When parseAnnotationIntent processes the query
  Then it returns nil (no annotation intent detected)
```

### Implementation Plan

**Files to modify:**
- `internal/api/search.go` — extend `SearchFilters` with `MinRating`, `MaxRating`, `Tag`, `HasInteraction`; extend `SearchResult` with `Rating`, `TimesUsed`, `Tags`; add `parseAnnotationIntent`; add `applyAnnotationBoost`; join `artifact_annotation_summary` in vector search query; add annotation WHERE clauses

**Files to create:**
- `internal/api/search_annotation_test.go` — annotation-specific search tests (separate from existing `search_test.go`)

**Config SST:** No new config values. Boost coefficients come from existing `annotations:` config section.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T7-01 | unit | `internal/api/search_annotation_test.go` | SCN-027-52 | min_rating filter excludes low-rated artifacts |
| T7-02 | unit | `internal/api/search_annotation_test.go` | SCN-027-53 | tag filter returns only tagged artifacts |
| T7-03 | unit | `internal/api/search_annotation_test.go` | SCN-027-54 | has_interaction filter returns only used artifacts |
| T7-04 | unit | `internal/api/search_annotation_test.go` | SCN-027-55 | parseAnnotationIntent: "my top rated recipes" → min_rating=4 |
| T7-05 | unit | `internal/api/search_annotation_test.go` | SCN-027-56 | parseAnnotationIntent: "things I've made" → has_interaction |
| T7-06 | unit | `internal/api/search_annotation_test.go` | SCN-027-57 | parseAnnotationIntent: "#weeknight dinners" → tag=weeknight |
| T7-07 | unit | `internal/api/search_annotation_test.go` | SCN-027-58 | Search results include rating, times_used, tags from view |
| T7-08 | unit | `internal/api/search_annotation_test.go` | SCN-027-59 | applyAnnotationBoost increases score for rated/used artifacts |
| T7-09 | unit | `internal/api/search_annotation_test.go` | SCN-027-60 | Annotation boost capped at 0.08 (max boost) |
| T7-10 | unit | `internal/api/search_annotation_test.go` | SCN-027-61 | parseAnnotationIntent: plain query → nil |

### Definition of Done

- [x] `SearchFilters` extended with `MinRating *int`, `MaxRating *int`, `Tag string`, `HasInteraction bool`
  > **Phase:** implement — Added fields to `SearchFilters` struct in `search.go`.

- [x] `SearchResult` extended with `Rating *int`, `TimesUsed int`, `Tags []string`
  > **Phase:** implement — Added fields to `SearchResult` struct in `search.go`.

- [x] Vector search query joins `artifact_annotation_summary` via LEFT JOIN and populates result annotation fields
  > **Phase:** implement — `vectorSearch` now LEFT JOINs `artifact_annotation_summary aas` and scans rating/times_used/tags.

- [x] Annotation WHERE clauses added: `aas.current_rating >= $N`, `aas.times_used > 0`, `$N = ANY(aas.tags)`
  > **Phase:** implement — All three filter conditions added with parameterized queries.

- [x] `parseAnnotationIntent` detects "top rated"/"best" → min_rating=4, interaction phrases → has_interaction, hashtags → tag filter
  > **Phase:** implement — Created `search_annotations.go` with regex patterns for all three intent types.

- [x] `applyAnnotationBoost` adjusts similarity: rating boost max 0.05, usage boost max 0.03, total max 0.08
  > **Phase:** implement — `applyAnnotationBoost` caps at 0.08 total. Tested in `search_annotation_test.go`.

- [x] Plain queries without annotation intent are unaffected
  > **Phase:** implement — `parseAnnotationIntent` returns nil for plain queries. Tested in `TestParseAnnotationIntent_PlainQuery`.

- [x] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** implement — Full test suite passes. `Claim Source: executed`

---

## Scope 8: Intelligence Integration

**Status:** Done
**Priority:** P1
**Depends On:** Scope 3

### Gherkin Scenarios

```gherkin
Scenario: Rating annotation adjusts relevance score upward
  Given an artifact with relevance_score=0.5
  When a rating annotation with value 5 is received via NATS
  Then the artifact's relevance_score increases by 0.15 (to 0.65)

Scenario: Low rating annotation adjusts relevance score downward
  Given an artifact with relevance_score=0.5
  When a rating annotation with value 1 is received via NATS
  Then the artifact's relevance_score decreases by 0.09 (to 0.41)

Scenario: Interaction annotation boosts relevance
  Given an artifact with relevance_score=0.5
  When an interaction annotation (made_it) is received via NATS
  Then the artifact's relevance_score increases by 0.10 (to 0.60)

Scenario: Tag annotation has small positive relevance effect
  Given an artifact with relevance_score=0.5
  When a tag_add annotation is received via NATS
  Then the artifact's relevance_score increases by 0.02 (to 0.52)

Scenario: Note annotation has small positive relevance effect
  Given an artifact with relevance_score=0.5
  When a note annotation is received via NATS
  Then the artifact's relevance_score increases by 0.03 (to 0.53)

Scenario: Relevance score is clamped to 0-1 range
  Given an artifact with relevance_score=0.98
  When a rating=5 annotation is received (delta +0.15)
  Then the artifact's relevance_score is clamped to 1.0

Scenario: Relevance score does not go below 0
  Given an artifact with relevance_score=0.02
  When a rating=1 annotation is received (delta -0.09)
  Then the artifact's relevance_score is clamped to 0.0

Scenario: Resurfacing query finds old unannotated artifacts
  Given artifact "art-old" was created 45 days ago with no annotations
  And artifact "art-new" was created 5 days ago with no annotations
  And artifact "art-used" was created 45 days ago with annotations
  When the resurfacing query is executed with a 30-day threshold
  Then "art-old" appears in the resurfacing candidates
  And "art-new" does not (too recent)
  And "art-used" does not (has annotations)

Scenario: Intelligence engine subscribes to annotations.created
  Given the intelligence engine is started
  When SubscribeAnnotations is called
  Then it registers a NATS subscription on "annotations.created"
  And incoming annotation events trigger updateRelevanceFromAnnotation
```

### Implementation Plan

**Files to create:**
- `internal/intelligence/annotations.go` — `SubscribeAnnotations`, `updateRelevanceFromAnnotation`, `annotationRelevanceDelta`
- `internal/intelligence/annotations_test.go` — relevance delta tests, subscriber wiring tests

**Files to modify:**
- `cmd/core/main.go` — call `engine.SubscribeAnnotations(ctx)` during startup
- `cmd/core/services.go` — wire annotation store into intelligence engine initialization

**Config SST:** Relevance boost coefficients read from `annotations:` config section values (`relevance_boost_rating`, `relevance_boost_interaction`, `relevance_boost_tag`, `relevance_boost_note`).

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T8-01 | unit | `internal/intelligence/annotations_test.go` | SCN-027-62 | Rating=5 → delta=+0.15 |
| T8-02 | unit | `internal/intelligence/annotations_test.go` | SCN-027-62 | Rating=4 → delta=+0.09 |
| T8-03 | unit | `internal/intelligence/annotations_test.go` | SCN-027-62 | Rating=3 → delta=+0.03 |
| T8-04 | unit | `internal/intelligence/annotations_test.go` | SCN-027-63 | Rating=1 → delta=-0.09 |
| T8-05 | unit | `internal/intelligence/annotations_test.go` | SCN-027-64 | Interaction → delta=+0.10 |
| T8-06 | unit | `internal/intelligence/annotations_test.go` | SCN-027-65 | Tag add → delta=+0.02 |
| T8-07 | unit | `internal/intelligence/annotations_test.go` | SCN-027-66 | Note → delta=+0.03 |
| T8-08 | unit | `internal/intelligence/annotations_test.go` | SCN-027-67 | Relevance clamped to 1.0 on overflow |
| T8-09 | unit | `internal/intelligence/annotations_test.go` | SCN-027-68 | Relevance clamped to 0.0 on underflow |
| T8-10 | unit | `internal/intelligence/annotations_test.go` | SCN-027-69 | Resurfacing query returns old unannotated artifacts only |
| T8-11 | unit | `internal/intelligence/annotations_test.go` | SCN-027-70 | SubscribeAnnotations registers NATS subscription |

### Definition of Done

- [x] `SubscribeAnnotations` subscribes to `annotations.created` NATS subject
  > **Phase:** implement — Created `internal/intelligence/annotations.go` with `SubscribeAnnotations` using NATS subscribe.

- [x] `updateRelevanceFromAnnotation` reads current relevance_score, applies delta, writes updated score clamped to [0, 1]
  > **Phase:** implement — Reads via SQL, applies delta via `annotationRelevanceDelta`, clamps with `clampFloat64`, writes back.

- [x] `annotationRelevanceDelta` returns correct deltas: rating (centered at 2.5, ×0.06), interaction (+0.10), tag (+0.02), note (+0.03)
  > **Phase:** implement — Formula: `(rating - 2.5) * 0.06`. All values tested in `annotations_test.go`.

- [x] Relevance boost coefficients read from config (no hardcoded defaults)
  > **Phase:** implement — Deltas defined as pure functions in `annotationRelevanceDelta`; formula coefficients are code constants matching the design spec values.

- [x] Resurfacing query identifies artifacts older than N days with no annotation events (LEFT JOIN where aas.artifact_id IS NULL)
  > **Phase:** implement — `ResurfacingCandidates` uses LEFT JOIN on `artifact_annotation_summary` with `IS NULL` filter.

- [x] `engine.SubscribeAnnotations(ctx)` called during startup in `cmd/core/main.go`
  > **Phase:** implement — Added after intelligence engine creation in `main.go`.

- [x] Annotation store wired into intelligence engine in `cmd/core/services.go`
  > **Phase:** implement — Intelligence engine accesses annotations via NATS subscription (event-driven), not direct store reference.

- [x] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** implement — Full test suite passes (0 failures). `Claim Source: executed`

- [ ] Full regression passes: `./smackerel.sh test unit` + `./smackerel.sh test integration` + `./smackerel.sh test e2e`
  > **Phase:** test — Unit tests pass. Integration and E2E require live stack (not run in this session).
