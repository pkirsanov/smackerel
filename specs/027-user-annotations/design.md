# Design: 027 — User Annotations & Interaction Tracking

> **Spec:** [spec.md](spec.md) | **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Depends On:** Phase 2 Ingestion (003), Knowledge Synthesis Layer (025)
> **Author:** bubbles.design
> **Date:** April 17, 2026
> **Status:** Draft

---

## Design Brief

**Current State:** Smackerel captures and processes artifacts through connectors and the Telegram share flow. The only user preference signal is a binary `user_starred BOOLEAN` column on the `artifacts` table. The intelligence engine (`internal/intelligence/engine.go`) uses `relevance_score` for recommendations, the search engine (`internal/api/search.go`) supports type/date/person/topic filters, and the Telegram bot (`internal/telegram/bot.go`) routes commands and share captures but has no reply-to-artifact annotation flow. The existing `access_count` and `last_accessed` columns track API access, not real-world usage.

**Target State:** Users annotate any artifact with ratings (1–5), freeform notes, custom tags (hashtags), and interaction records ("made it", "bought it", "read it", "visited", "tried it", "used it"). Annotations are append-only events stored in a dedicated `annotations` table. An `artifact_annotation_summary` materialized view provides fast read access for search, intelligence, and digest systems. A new `telegram_message_artifacts` table maps Telegram message IDs to artifact IDs, enabling reply-to annotation. The annotation parser is a pure Go module that parses freeform strings like `"4/5 made it #weeknight needs more garlic"` into structured components. Search, intelligence, and digest consumers read annotation data to improve relevance scoring, recommendations, and resurfacing.

**Patterns to Follow:**
- NATS event-driven pattern for annotation → intelligence fan-out (consistent with `artifacts.processed` subscriber)
- Chi router group pattern from [internal/api/router.go](../../internal/api/router.go) for new `/api/artifacts/{id}/annotations` routes
- Dependencies injection from [internal/api/health.go](../../internal/api/health.go) — annotation store exposed via interface on `Dependencies`
- Telegram command handler pattern from [internal/telegram/bot.go](../../internal/telegram/bot.go) for `/rate` command
- Config-driven values from `config/smackerel.yaml` with zero hardcoded defaults

**Patterns to Avoid:**
- Direct SQL in API handlers — all annotation DB operations go through `internal/annotation/` package
- Modifying the `artifacts` table with annotation columns — annotations are a separate layer
- Synchronous intelligence scoring in the annotation write path — fan out via NATS
- Hardcoded interaction type strings in multiple places — define as Go constants in one package

**Resolved Decisions:**
- Annotations live in their own table, not as JSONB on artifacts (enables history, querying, and clean separation)
- Materialized view for annotation summary (balance between query speed and write simplicity)
- `telegram_message_artifacts` table for reply-to mapping (more reliable than regex-matching artifact titles in messages)
- Annotation parser is a pure function with no I/O dependencies (testable, reusable across Telegram and API)
- NATS event published on annotation write for downstream consumers (intelligence, digest)
- Existing `user_starred` treated as equivalent to rating 5 during migration/backwards-compat period
- Tag removal uses soft-delete on the annotation event (append-only log), with materialized view recomputation

---

## Architecture Overview

```
                    ┌──────────────────────────────────────────────────────┐
                    │                   User Input Channels                │
                    │                                                      │
                    │  ┌──────────────┐     ┌─────────────────┐           │
                    │  │  Telegram     │     │  REST API        │           │
                    │  │  Reply-to msg │     │  POST /api/      │           │
                    │  │  /rate cmd    │     │  artifacts/{id}/ │           │
                    │  │  #tags        │     │  annotations     │           │
                    │  └──────┬───────┘     └────────┬────────┘           │
                    │         │                      │                     │
                    └─────────┼──────────────────────┼─────────────────────┘
                              │                      │
                              ▼                      ▼
                    ┌──────────────────────────────────────────────────────┐
                    │               Annotation Parser (Go)                 │
                    │   "4/5 made it #weeknight needs more garlic"         │
                    │   → rating:4  interaction:made_it  tags:[weeknight]  │
                    │     note:"needs more garlic"                         │
                    └──────────────────────┬───────────────────────────────┘
                                           │
                                           ▼
                    ┌──────────────────────────────────────────────────────┐
                    │              Annotation Store (Go)                    │
                    │   internal/annotation/store.go                       │
                    │   - Insert annotation event → annotations table      │
                    │   - Refresh materialized view                        │
                    │   - Publish NATS event: annotations.created          │
                    └──────────┬──────────────────────┬────────────────────┘
                               │                      │
                    ┌──────────┴──────────┐  ┌───────┴─────────────────┐
                    │   PostgreSQL         │  │  NATS Stream            │
                    │   annotations table  │  │  annotations.created    │
                    │   materialized view  │  │                         │
                    │   message_artifacts  │  └──────────┬──────────────┘
                    └──────────┬──────────┘              │
                               │                         │
                    ┌──────────┴──────────────────────────┴────────────────┐
                    │                 Downstream Consumers                  │
                    │                                                      │
                    │  ┌────────────────┐ ┌──────────┐ ┌────────────────┐ │
                    │  │ Search Engine  │ │ Intel    │ │ Digest Gen     │ │
                    │  │ Filter/boost   │ │ Engine   │ │ Prioritize     │ │
                    │  │ by annotation  │ │ Scoring  │ │ annotated      │ │
                    │  └────────────────┘ └──────────┘ └────────────────┘ │
                    └──────────────────────────────────────────────────────┘
```

---

## Database Schema

### Migration: `015_user_annotations.sql`

```sql
-- 015_user_annotations.sql
-- User annotations, interaction tracking, and Telegram message-to-artifact mapping.
--
-- ROLLBACK:
--   DROP MATERIALIZED VIEW IF EXISTS artifact_annotation_summary CASCADE;
--   DROP TABLE IF EXISTS annotations CASCADE;
--   DROP TABLE IF EXISTS telegram_message_artifacts CASCADE;
--   DROP TYPE IF EXISTS annotation_type;
--   DROP TYPE IF EXISTS interaction_type;

-- Enum for annotation event types
CREATE TYPE annotation_type AS ENUM (
    'rating',
    'note',
    'tag_add',
    'tag_remove',
    'interaction',
    'status_change'
);

-- Enum for interaction types (domain-agnostic)
CREATE TYPE interaction_type AS ENUM (
    'made_it',
    'bought_it',
    'read_it',
    'visited',
    'tried_it',
    'used_it'
);

-- Annotations: append-only event log of user interactions with artifacts
CREATE TABLE IF NOT EXISTS annotations (
    id              TEXT PRIMARY KEY,
    artifact_id     TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    ann_type        annotation_type NOT NULL,
    rating          SMALLINT CHECK (rating IS NULL OR (rating >= 1 AND rating <= 5)),
    note            TEXT,
    tag             TEXT,          -- single tag per event (normalized lowercase, trimmed)
    interaction     interaction_type,
    source_channel  TEXT NOT NULL, -- 'telegram', 'api', 'web'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_annotations_artifact ON annotations(artifact_id);
CREATE INDEX IF NOT EXISTS idx_annotations_type ON annotations(ann_type);
CREATE INDEX IF NOT EXISTS idx_annotations_created ON annotations(created_at);
CREATE INDEX IF NOT EXISTS idx_annotations_tag ON annotations(tag) WHERE tag IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_annotations_rating ON annotations(rating) WHERE rating IS NOT NULL;

-- Telegram message-to-artifact mapping for reply-to resolution
-- When the bot sends a confirmation message after capture, record the mapping.
-- When the user replies to that message, look up the artifact ID.
CREATE TABLE IF NOT EXISTS telegram_message_artifacts (
    message_id      BIGINT NOT NULL,
    chat_id         BIGINT NOT NULL,
    artifact_id     TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, chat_id)
);

CREATE INDEX IF NOT EXISTS idx_tma_artifact ON telegram_message_artifacts(artifact_id);

-- Materialized view: pre-aggregated annotation summary per artifact.
-- Refreshed after annotation writes (REFRESH MATERIALIZED VIEW CONCURRENTLY).
CREATE MATERIALIZED VIEW artifact_annotation_summary AS
SELECT
    a.artifact_id,
    -- Latest rating (most recent rating event)
    (SELECT ann.rating
     FROM annotations ann
     WHERE ann.artifact_id = a.artifact_id AND ann.ann_type = 'rating'
     ORDER BY ann.created_at DESC LIMIT 1
    ) AS current_rating,
    -- Average rating
    AVG(a.rating) FILTER (WHERE a.ann_type = 'rating') AS average_rating,
    -- Rating count
    COUNT(*) FILTER (WHERE a.ann_type = 'rating') AS rating_count,
    -- Interaction count (times used/made/read/etc.)
    COUNT(*) FILTER (WHERE a.ann_type = 'interaction') AS times_used,
    -- Last interaction timestamp
    MAX(a.created_at) FILTER (WHERE a.ann_type = 'interaction') AS last_used,
    -- Active tags (added minus removed)
    ARRAY(
        SELECT DISTINCT sub.tag FROM annotations sub
        WHERE sub.artifact_id = a.artifact_id
          AND sub.ann_type = 'tag_add'
          AND sub.tag NOT IN (
              SELECT sub2.tag FROM annotations sub2
              WHERE sub2.artifact_id = a.artifact_id
                AND sub2.ann_type = 'tag_remove'
                AND sub2.created_at > sub.created_at
          )
    ) AS tags,
    -- Notes count
    COUNT(*) FILTER (WHERE a.ann_type = 'note') AS notes_count,
    -- Total annotation events
    COUNT(*) AS total_events,
    -- Most recent annotation of any type
    MAX(a.created_at) AS last_annotated
FROM annotations a
GROUP BY a.artifact_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_aas_artifact ON artifact_annotation_summary(artifact_id);
CREATE INDEX IF NOT EXISTS idx_aas_rating ON artifact_annotation_summary(current_rating) WHERE current_rating IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_aas_times_used ON artifact_annotation_summary(times_used DESC) WHERE times_used > 0;
CREATE INDEX IF NOT EXISTS idx_aas_last_used ON artifact_annotation_summary(last_used DESC) WHERE last_used IS NOT NULL;
```

### Key Schema Decisions

1. **Append-only event log** — Every annotation action (rate, tag, note, interact) is an immutable event. No UPDATEs on the `annotations` table. The materialized view computes current state from the event stream.

2. **Single tag per event** — Tag add/remove events carry one tag each. This keeps the event log clean and makes tag removal unambiguous (remove event references the exact tag string).

3. **Materialized view over cached columns** — The `artifact_annotation_summary` view is refreshed concurrently (no lock) after writes. This avoids complex trigger logic and keeps the annotations table append-only. For a single-user system, refresh cost is negligible.

4. **Telegram message mapping** — `telegram_message_artifacts` stores `(message_id, chat_id) → artifact_id`. The bot records this after every capture confirmation. Reply-to lookups are a simple primary key lookup.

5. **`source_channel` column** — Tracks where the annotation came from. Enables future analytics and channel-specific behavior without schema changes.

---

## Go Types

### Package: `internal/annotation`

```go
package annotation

import "time"

// AnnotationType is the kind of annotation event.
type AnnotationType string

const (
    TypeRating       AnnotationType = "rating"
    TypeNote         AnnotationType = "note"
    TypeTagAdd       AnnotationType = "tag_add"
    TypeTagRemove    AnnotationType = "tag_remove"
    TypeInteraction  AnnotationType = "interaction"
    TypeStatusChange AnnotationType = "status_change"
)

// InteractionType is the kind of real-world interaction.
type InteractionType string

const (
    InteractionMadeIt   InteractionType = "made_it"
    InteractionBoughtIt InteractionType = "bought_it"
    InteractionReadIt   InteractionType = "read_it"
    InteractionVisited  InteractionType = "visited"
    InteractionTriedIt  InteractionType = "tried_it"
    InteractionUsedIt   InteractionType = "used_it"
)

// SourceChannel identifies where the annotation originated.
type SourceChannel string

const (
    ChannelTelegram SourceChannel = "telegram"
    ChannelAPI      SourceChannel = "api"
    ChannelWeb      SourceChannel = "web"
)

// Annotation is a single event in the annotation log.
type Annotation struct {
    ID            string          `json:"id"`
    ArtifactID    string          `json:"artifact_id"`
    Type          AnnotationType  `json:"type"`
    Rating        *int            `json:"rating,omitempty"`        // 1-5, nil if not a rating event
    Note          string          `json:"note,omitempty"`
    Tag           string          `json:"tag,omitempty"`           // single tag per event
    Interaction   InteractionType `json:"interaction,omitempty"`
    SourceChannel SourceChannel   `json:"source_channel"`
    CreatedAt     time.Time       `json:"created_at"`
}

// Summary is the pre-aggregated annotation state for an artifact.
// Populated from the artifact_annotation_summary materialized view.
type Summary struct {
    ArtifactID    string    `json:"artifact_id"`
    CurrentRating *int      `json:"current_rating,omitempty"`
    AverageRating *float64  `json:"average_rating,omitempty"`
    RatingCount   int       `json:"rating_count"`
    TimesUsed     int       `json:"times_used"`
    LastUsed      *time.Time `json:"last_used,omitempty"`
    Tags          []string  `json:"tags"`
    NotesCount    int       `json:"notes_count"`
    TotalEvents   int       `json:"total_events"`
    LastAnnotated *time.Time `json:"last_annotated,omitempty"`
}

// ParsedAnnotation is the output of the annotation parser.
// A single freeform input can produce multiple annotation events.
type ParsedAnnotation struct {
    Rating      *int            `json:"rating,omitempty"`
    Interaction InteractionType `json:"interaction,omitempty"`
    Tags        []string        `json:"tags,omitempty"`
    Note        string          `json:"note,omitempty"`
}
```

### Package: `internal/annotation/parser.go`

```go
package annotation

import (
    "regexp"
    "strconv"
    "strings"
)

var (
    // ratingPattern matches "N/5", "N / 5", "N out of 5", or bare 1-5 followed by stars
    ratingPattern = regexp.MustCompile(`(?i)\b([1-5])\s*/\s*5\b|([1-5])\s+out\s+of\s+5\b|^([1-5])(?:\s*★+)?\s*$`)

    // tagPattern matches #hashtag (alphanumeric, hyphens, underscores)
    tagPattern = regexp.MustCompile(`#([a-zA-Z][a-zA-Z0-9_-]{0,49})`)

    // interactionPatterns maps normalized phrases to interaction types
    interactionKeywords = map[string]InteractionType{
        "made it":   InteractionMadeIt,
        "made this": InteractionMadeIt,
        "cooked it": InteractionMadeIt,
        "bought it": InteractionBoughtIt,
        "purchased": InteractionBoughtIt,
        "read it":   InteractionReadIt,
        "read this": InteractionReadIt,
        "finished":  InteractionReadIt,
        "visited":   InteractionVisited,
        "been here": InteractionVisited,
        "went here": InteractionVisited,
        "tried it":  InteractionTriedIt,
        "tried this":InteractionTriedIt,
        "used it":   InteractionUsedIt,
        "used this": InteractionUsedIt,
    }

    // tagRemovePattern matches #remove-tagname
    tagRemovePattern = regexp.MustCompile(`(?i)#remove-([a-zA-Z][a-zA-Z0-9_-]{0,49})`)
)

// Parse extracts structured annotation components from freeform text.
// Pure function with no I/O. Input examples:
//   - "4/5" → rating 4
//   - "made it, 4/5, needs more garlic" → interaction + rating + note
//   - "#weeknight #quick" → two tags
//   - "4/5 made it #weeknight needs more garlic" → all components
//   - "#remove-quick" → tag removal
func Parse(input string) ParsedAnnotation {
    input = strings.TrimSpace(input)
    if input == "" {
        return ParsedAnnotation{}
    }

    var result ParsedAnnotation
    remaining := input

    // Extract rating
    if m := ratingPattern.FindStringSubmatch(remaining); m != nil {
        for _, g := range m[1:] {
            if g != "" {
                if v, err := strconv.Atoi(g); err == nil {
                    result.Rating = &v
                }
                break
            }
        }
        remaining = ratingPattern.ReplaceAllString(remaining, "")
    }

    // Extract tag removals first (before tag adds)
    removeMatches := tagRemovePattern.FindAllStringSubmatch(remaining, -1)
    // Tag removals are returned as tags with a "remove-" prefix in ParsedAnnotation.
    // The caller (store) interprets "remove-X" as TypeTagRemove for tag "X".

    // Extract tag additions
    tagMatches := tagPattern.FindAllStringSubmatch(remaining, -1)
    for _, m := range tagMatches {
        tag := strings.ToLower(strings.TrimSpace(m[1]))
        if !strings.HasPrefix(tag, "remove-") {
            result.Tags = append(result.Tags, tag)
        }
    }
    // Add removal tags with prefix
    for _, m := range removeMatches {
        result.Tags = append(result.Tags, "remove-"+strings.ToLower(strings.TrimSpace(m[1])))
    }
    remaining = tagPattern.ReplaceAllString(remaining, "")
    remaining = tagRemovePattern.ReplaceAllString(remaining, "")

    // Extract interaction type
    lower := strings.ToLower(remaining)
    for phrase, itype := range interactionKeywords {
        if strings.Contains(lower, phrase) {
            result.Interaction = itype
            // Remove the matched phrase from remaining text
            idx := strings.Index(lower, phrase)
            remaining = remaining[:idx] + remaining[idx+len(phrase):]
            lower = strings.ToLower(remaining)
            break
        }
    }

    // Whatever is left (after stripping ratings, tags, interactions, commas,
    // leading/trailing whitespace) is the note.
    remaining = strings.TrimSpace(remaining)
    remaining = strings.Trim(remaining, ",;. ")
    remaining = strings.TrimSpace(remaining)
    if remaining != "" {
        result.Note = remaining
    }

    return result
}
```

**Parser contract:** The parser is intentionally lenient. Unrecognized text becomes a note. Missing components are zero-valued. The caller decides which annotation events to create from the parsed output.

---

## Annotation Store

### Package: `internal/annotation/store.go`

```go
package annotation

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/oklog/ulid/v2"

    smacknats "github.com/smackerel/smackerel/internal/nats"
)

// Store handles annotation persistence and event publication.
type Store struct {
    DB   *pgxpool.Pool
    NATS *smacknats.Client
}

// NewStore creates an annotation store.
func NewStore(db *pgxpool.Pool, nats *smacknats.Client) *Store {
    return &Store{DB: db, NATS: nats}
}

// CreateAnnotation inserts an annotation event, refreshes the materialized view,
// and publishes a NATS event for downstream consumers.
func (s *Store) CreateAnnotation(ctx context.Context, ann *Annotation) error {
    ann.ID = ulid.Make().String()

    _, err := s.DB.Exec(ctx, `
        INSERT INTO annotations (id, artifact_id, ann_type, rating, note, tag, interaction, source_channel, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
    `, ann.ID, ann.ArtifactID, ann.Type, ann.Rating, ann.Note, ann.Tag, ann.Interaction, ann.SourceChannel)
    if err != nil {
        return fmt.Errorf("insert annotation: %w", err)
    }

    // Refresh materialized view concurrently (non-blocking for other readers)
    _, err = s.DB.Exec(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY artifact_annotation_summary")
    if err != nil {
        // Log but don't fail — view will be stale until next refresh
        return fmt.Errorf("refresh annotation summary: %w", err)
    }

    // Publish NATS event for intelligence/digest consumers (best-effort)
    if s.NATS != nil {
        _ = s.NATS.Publish(ctx, SubjectAnnotationsCreated, mustMarshal(ann))
    }

    return nil
}

// CreateFromParsed converts a ParsedAnnotation into individual annotation events
// and writes them all within the same context. Returns the list of created annotations.
func (s *Store) CreateFromParsed(ctx context.Context, artifactID string, parsed ParsedAnnotation, channel SourceChannel) ([]Annotation, error) {
    // Verify artifact exists
    var exists bool
    err := s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM artifacts WHERE id = $1)", artifactID).Scan(&exists)
    if err != nil {
        return nil, fmt.Errorf("check artifact existence: %w", err)
    }
    if !exists {
        return nil, fmt.Errorf("artifact not found: %s", artifactID)
    }

    var created []Annotation

    // Rating event
    if parsed.Rating != nil {
        ann := Annotation{
            ArtifactID:    artifactID,
            Type:          TypeRating,
            Rating:        parsed.Rating,
            SourceChannel: channel,
        }
        if err := s.CreateAnnotation(ctx, &ann); err != nil {
            return created, err
        }
        created = append(created, ann)
    }

    // Interaction event
    if parsed.Interaction != "" {
        ann := Annotation{
            ArtifactID:    artifactID,
            Type:          TypeInteraction,
            Interaction:   parsed.Interaction,
            SourceChannel: channel,
        }
        if err := s.CreateAnnotation(ctx, &ann); err != nil {
            return created, err
        }
        created = append(created, ann)
    }

    // Tag events
    for _, tag := range parsed.Tags {
        if strings.HasPrefix(tag, "remove-") {
            ann := Annotation{
                ArtifactID:    artifactID,
                Type:          TypeTagRemove,
                Tag:           strings.TrimPrefix(tag, "remove-"),
                SourceChannel: channel,
            }
            if err := s.CreateAnnotation(ctx, &ann); err != nil {
                return created, err
            }
            created = append(created, ann)
        } else {
            ann := Annotation{
                ArtifactID:    artifactID,
                Type:          TypeTagAdd,
                Tag:           tag,
                SourceChannel: channel,
            }
            if err := s.CreateAnnotation(ctx, &ann); err != nil {
                return created, err
            }
            created = append(created, ann)
        }
    }

    // Note event
    if parsed.Note != "" {
        ann := Annotation{
            ArtifactID:    artifactID,
            Type:          TypeNote,
            Note:          parsed.Note,
            SourceChannel: channel,
        }
        if err := s.CreateAnnotation(ctx, &ann); err != nil {
            return created, err
        }
        created = append(created, ann)
    }

    return created, nil
}

// GetSummary returns the pre-aggregated annotation summary for an artifact.
func (s *Store) GetSummary(ctx context.Context, artifactID string) (*Summary, error) {
    var sum Summary
    err := s.DB.QueryRow(ctx, `
        SELECT artifact_id, current_rating, average_rating, rating_count,
               times_used, last_used, tags, notes_count, total_events, last_annotated
        FROM artifact_annotation_summary
        WHERE artifact_id = $1
    `, artifactID).Scan(
        &sum.ArtifactID, &sum.CurrentRating, &sum.AverageRating, &sum.RatingCount,
        &sum.TimesUsed, &sum.LastUsed, &sum.Tags, &sum.NotesCount,
        &sum.TotalEvents, &sum.LastAnnotated,
    )
    if err != nil {
        return nil, err // pgx.ErrNoRows if no annotations exist
    }
    return &sum, nil
}

// GetHistory returns the annotation event history for an artifact, newest first.
func (s *Store) GetHistory(ctx context.Context, artifactID string, limit int) ([]Annotation, error) {
    if limit <= 0 || limit > 100 {
        limit = 50
    }
    rows, err := s.DB.Query(ctx, `
        SELECT id, artifact_id, ann_type, rating, note, tag, interaction, source_channel, created_at
        FROM annotations
        WHERE artifact_id = $1
        ORDER BY created_at DESC
        LIMIT $2
    `, artifactID, limit)
    if err != nil {
        return nil, fmt.Errorf("query annotation history: %w", err)
    }
    defer rows.Close()

    var history []Annotation
    for rows.Next() {
        var a Annotation
        if err := rows.Scan(&a.ID, &a.ArtifactID, &a.Type, &a.Rating, &a.Note,
            &a.Tag, &a.Interaction, &a.SourceChannel, &a.CreatedAt); err != nil {
            continue
        }
        history = append(history, a)
    }
    return history, rows.Err()
}

// DeleteTag removes a specific tag from an artifact by inserting a tag_remove event.
func (s *Store) DeleteTag(ctx context.Context, artifactID, tag string, channel SourceChannel) error {
    ann := Annotation{
        ArtifactID:    artifactID,
        Type:          TypeTagRemove,
        Tag:           strings.ToLower(strings.TrimSpace(tag)),
        SourceChannel: channel,
    }
    return s.CreateAnnotation(ctx, &ann)
}
```

### Store Interface (for Dependencies injection)

```go
// AnnotationQuerier is the interface exposed on api.Dependencies.
type AnnotationQuerier interface {
    CreateFromParsed(ctx context.Context, artifactID string, parsed ParsedAnnotation, channel SourceChannel) ([]Annotation, error)
    GetSummary(ctx context.Context, artifactID string) (*Summary, error)
    GetHistory(ctx context.Context, artifactID string, limit int) ([]Annotation, error)
    DeleteTag(ctx context.Context, artifactID, tag string, channel SourceChannel) error
}
```

---

## NATS Events

### New Subjects

Add to `internal/nats/client.go`:

```go
const (
    // Annotation events
    SubjectAnnotationsCreated = "annotations.created"
)
```

Add `annotations.*` to the existing `ARTIFACTS` stream (reuse existing stream infrastructure):

```go
// In AllStreams(), add to ARTIFACTS stream subjects:
{
    Name:     "ARTIFACTS",
    Subjects: []string{
        "artifacts.>",
        "annotations.>",  // NEW
    },
},
```

### Event Payload

The NATS event payload is the JSON-serialized `Annotation` struct. Downstream consumers (intelligence engine, digest generator) subscribe to `annotations.created` and react accordingly.

### NATS Contract Update

Add to `config/nats_contract.json`:

```json
{
    "annotations.created": {
        "direction": "go_publishes",
        "description": "Published when a user annotation event is recorded",
        "payload_type": "annotation_event"
    }
}
```

---

## API Endpoints

### New Routes

Add to [internal/api/router.go](../../internal/api/router.go) inside the authenticated group:

```go
// Annotation endpoints (spec 027)
if deps.AnnotationStore != nil {
    r.Post("/artifacts/{id}/annotations", deps.CreateAnnotationHandler)
    r.Get("/artifacts/{id}/annotations", deps.GetAnnotationsHandler)
    r.Get("/artifacts/{id}/annotations/summary", deps.GetAnnotationSummaryHandler)
    r.Delete("/artifacts/{id}/tags/{tag}", deps.DeleteTagHandler)
}
```

### `POST /api/artifacts/{id}/annotations`

Creates one or more annotation events from a freeform or structured input.

**Request:**

```json
{
    "text": "4/5 made it #weeknight needs more garlic",
    "rating": null,
    "note": null,
    "tags": null,
    "interaction": null
}
```

Either `text` (freeform, parsed by annotation parser) OR explicit fields. If `text` is provided, it takes precedence and is parsed. If explicit fields are provided, they are used directly.

**Response (201 Created):**

```json
{
    "annotations": [
        {"id": "01J...", "type": "rating", "rating": 4, "created_at": "..."},
        {"id": "01J...", "type": "interaction", "interaction": "made_it", "created_at": "..."},
        {"id": "01J...", "type": "tag_add", "tag": "weeknight", "created_at": "..."},
        {"id": "01J...", "type": "note", "note": "needs more garlic", "created_at": "..."}
    ],
    "summary": {
        "artifact_id": "...",
        "current_rating": 4,
        "times_used": 1,
        "tags": ["weeknight"],
        "notes_count": 1
    }
}
```

**Error Responses:**
- `400` — Empty body or invalid rating value
- `404` — Artifact not found
- `500` — Database error

### `GET /api/artifacts/{id}/annotations`

Returns annotation event history for an artifact.

**Query Parameters:**
- `limit` (optional, default 50, max 100)

**Response (200):**

```json
{
    "annotations": [...],
    "total": 12
}
```

### `GET /api/artifacts/{id}/annotations/summary`

Returns the materialized annotation summary.

**Response (200):**

```json
{
    "artifact_id": "...",
    "current_rating": 4,
    "average_rating": 4.2,
    "rating_count": 5,
    "times_used": 3,
    "last_used": "2026-04-15T18:30:00Z",
    "tags": ["weeknight", "quick", "kids-approved"],
    "notes_count": 2,
    "total_events": 12,
    "last_annotated": "2026-04-16T20:00:00Z"
}
```

Returns `{}` with `200` if no annotations exist (not a 404).

### `DELETE /api/artifacts/{id}/tags/{tag}`

Removes a tag by inserting a `tag_remove` event.

**Response (200):**

```json
{
    "removed": "weeknight",
    "summary": { ... }
}
```

### Handler Implementation: `internal/api/annotations.go`

```go
package api

import (
    "encoding/json"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/smackerel/smackerel/internal/annotation"
)

// CreateAnnotationRequest is the JSON body for POST /api/artifacts/{id}/annotations.
type CreateAnnotationRequest struct {
    Text        string  `json:"text,omitempty"`
    Rating      *int    `json:"rating,omitempty"`
    Note        string  `json:"note,omitempty"`
    Tags        []string `json:"tags,omitempty"`
    Interaction string  `json:"interaction,omitempty"`
}

// CreateAnnotationResponse is the response for POST /api/artifacts/{id}/annotations.
type CreateAnnotationResponse struct {
    Annotations []annotation.Annotation `json:"annotations"`
    Summary     *annotation.Summary     `json:"summary,omitempty"`
}

// CreateAnnotationHandler handles POST /api/artifacts/{id}/annotations.
func (d *Dependencies) CreateAnnotationHandler(w http.ResponseWriter, r *http.Request) {
    artifactID := chi.URLParam(r, "id")
    if artifactID == "" {
        writeError(w, http.StatusBadRequest, "MISSING_ID", "Artifact ID is required")
        return
    }

    var req CreateAnnotationRequest
    if !decodeJSONBody(w, r, &req, "INVALID_INPUT", "Invalid JSON body") {
        return
    }

    var parsed annotation.ParsedAnnotation

    if req.Text != "" {
        // Freeform parse
        parsed = annotation.Parse(req.Text)
    } else {
        // Structured fields
        parsed.Rating = req.Rating
        parsed.Note = req.Note
        parsed.Tags = req.Tags
        if req.Interaction != "" {
            parsed.Interaction = annotation.InteractionType(req.Interaction)
        }
    }

    // Validate rating range
    if parsed.Rating != nil && (*parsed.Rating < 1 || *parsed.Rating > 5) {
        writeError(w, http.StatusBadRequest, "INVALID_RATING", "Rating must be 1-5")
        return
    }

    // Validate at least one annotation component
    if parsed.Rating == nil && parsed.Interaction == "" && len(parsed.Tags) == 0 && parsed.Note == "" {
        writeError(w, http.StatusBadRequest, "EMPTY_ANNOTATION", "At least one annotation field is required")
        return
    }

    created, err := d.AnnotationStore.CreateFromParsed(r.Context(), artifactID, parsed, annotation.ChannelAPI)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "ANNOTATION_FAILED", "Failed to create annotation")
        return
    }

    // Fetch updated summary
    summary, _ := d.AnnotationStore.GetSummary(r.Context(), artifactID)

    writeJSON(w, http.StatusCreated, CreateAnnotationResponse{
        Annotations: created,
        Summary:     summary,
    })
}
```

---

## Telegram Integration

### Message-to-Artifact Mapping

The Telegram bot must record which artifact each confirmation message refers to. This happens in every handler that sends a capture confirmation.

**Recording the mapping** — After `callCapture` returns and the bot sends its confirmation reply, record the sent message ID:

```go
// recordMessageArtifact stores the telegram message → artifact mapping
// for reply-to annotation resolution.
func (b *Bot) recordMessageArtifact(ctx context.Context, messageID int, chatID int64, artifactID string) {
    if b.annotationURL == "" {
        return
    }
    // POST to internal API endpoint that records the mapping.
    // Alternatively, the bot can write directly to DB if it has pool access.
    // Design decision: bot calls internal API to avoid direct DB coupling.
    body, _ := json.Marshal(map[string]interface{}{
        "message_id":  messageID,
        "chat_id":     chatID,
        "artifact_id": artifactID,
    })
    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        b.coreAPIURL+"/internal/telegram-message-artifact", bytes.NewReader(body))
    if err != nil {
        slog.Warn("failed to record message-artifact mapping", "error", err)
        return
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+b.authToken)
    resp, err := b.httpClient.Do(req)
    if err != nil {
        slog.Warn("failed to record message-artifact mapping", "error", err)
        return
    }
    defer resp.Body.Close()
}
```

**Alternative (simpler):** If the bot has access to `*pgxpool.Pool`, it can insert directly. This avoids an HTTP round-trip but couples the bot to the DB. Given the bot already makes HTTP calls to the core API for captures, maintaining the HTTP pattern is more consistent.

### Internal Endpoint for Message Mapping

Add to router (outside authenticated group, internal-only — same auth token):

```go
// Internal: Telegram message-artifact mapping (used by bot)
r.Post("/internal/telegram-message-artifact", deps.RecordTelegramMessageArtifactHandler)
```

### Reply-to Annotation Handler

Add to `handleMessage()` in [internal/telegram/bot.go](../../internal/telegram/bot.go), before the command routing:

```go
// handleMessage routes incoming messages to the appropriate handler.
func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
    // ... existing allowlist check ...

    // NEW: Handle reply-to-artifact annotation
    if msg.ReplyToMessage != nil && !msg.IsCommand() {
        b.handleReplyAnnotation(ctx, msg)
        return
    }

    // ... existing command routing, URL handling, text capture ...
}
```

**Reply annotation handler:**

```go
// handleReplyAnnotation handles a user reply to a bot message as an annotation.
// Looks up the artifact from the replied-to message, parses the annotation,
// and records it.
func (b *Bot) handleReplyAnnotation(ctx context.Context, msg *tgbotapi.Message) {
    repliedMsgID := msg.ReplyToMessage.MessageID
    chatID := msg.Chat.ID

    // Look up artifact from telegram_message_artifacts
    artifactID, err := b.resolveArtifactFromMessage(ctx, repliedMsgID, chatID)
    if err != nil || artifactID == "" {
        // Not a reply to a known artifact message — fall through to normal handling
        // Re-dispatch to the normal text/URL handler
        b.handleFallbackMessage(ctx, msg)
        return
    }

    // Parse the reply text as an annotation
    parsed := annotation.Parse(msg.Text)

    // If parsing produced nothing useful, treat as a note
    if parsed.Rating == nil && parsed.Interaction == "" && len(parsed.Tags) == 0 && parsed.Note == "" {
        if msg.Text != "" {
            parsed.Note = msg.Text
        } else {
            b.reply(chatID, "? Reply with a rating (e.g. 4/5), tags (#weeknight), or a note")
            return
        }
    }

    // Submit annotation via API
    created, err := b.submitAnnotation(ctx, artifactID, parsed)
    if err != nil {
        b.reply(chatID, "? Failed to save annotation. Try again.")
        return
    }

    // Format confirmation
    b.reply(chatID, formatAnnotationConfirmation(created))
}

// resolveArtifactFromMessage looks up the artifact ID for a Telegram message.
func (b *Bot) resolveArtifactFromMessage(ctx context.Context, messageID int, chatID int64) (string, error) {
    // Call internal API or query DB directly
    // Returns artifact_id or empty string if not found
    ...
}
```

### `/rate` Command Handler

For annotating artifacts without a reply-to context (BS-003 from spec):

```go
// handleRate handles /rate <search terms> <annotation>
// Example: /rate pasta carbonara 4/5 too salty
func (b *Bot) handleRate(ctx context.Context, msg *tgbotapi.Message, args string) {
    if args == "" {
        b.reply(msg.Chat.ID, "? Usage: /rate <what to rate> <rating/note/tags>\nExample: /rate pasta carbonara 4/5 great dish")
        return
    }

    // Heuristic: split on first rating/tag/interaction keyword to separate
    // search terms from annotation content.
    searchTerms, annotationText := splitRateArgs(args)

    if searchTerms == "" {
        b.reply(msg.Chat.ID, "? I need something to search for. Example: /rate pasta carbonara 4/5")
        return
    }

    // Search for matching artifact
    results, err := b.callSearch(ctx, searchTerms)
    if err != nil {
        b.reply(msg.Chat.ID, "? Search failed. Try again.")
        return
    }

    resultList, ok := results["results"].([]interface{})
    if !ok || len(resultList) == 0 {
        b.reply(msg.Chat.ID, "? No matching artifacts found for: "+searchTerms)
        return
    }

    // If only one strong match, annotate directly
    if len(resultList) == 1 {
        result := resultList[0].(map[string]interface{})
        artifactID, _ := result["artifact_id"].(string)
        parsed := annotation.Parse(annotationText)
        created, err := b.submitAnnotation(ctx, artifactID, parsed)
        if err != nil {
            b.reply(msg.Chat.ID, "? Failed to save annotation.")
            return
        }
        title, _ := result["title"].(string)
        b.reply(msg.Chat.ID, fmt.Sprintf("Annotated \"%s\":\n%s", title, formatAnnotationConfirmation(created)))
        return
    }

    // Multiple matches — disambiguation
    // Show top 3 and ask user to reply with number
    var lines []string
    lines = append(lines, "? Which one did you mean?")
    for i, r := range resultList {
        if i >= 3 {
            break
        }
        result, _ := r.(map[string]interface{})
        title, _ := result["title"].(string)
        artType, _ := result["artifact_type"].(string)
        lines = append(lines, fmt.Sprintf("%d. %s (%s)", i+1, title, artType))
    }
    lines = append(lines, "\nReply with the number to annotate.")
    b.reply(msg.Chat.ID, strings.Join(lines, "\n"))

    // Store pending disambiguation state (in-memory, short TTL)
    // The next numeric reply from this chat resolves the disambiguation.
    b.storePendingDisambiguation(msg.Chat.ID, resultList, annotationText)
}
```

**Register the command** — Add to `Start()` command registration and `handleMessage()` command switch:

```go
tgbotapi.BotCommand{Command: "rate", Description: "Rate or annotate an artifact"},
```

```go
case "rate":
    b.handleRate(ctx, msg, msg.CommandArguments())
```

### Disambiguation State

```go
// pendingDisambiguation holds state for a /rate disambiguation flow.
type pendingDisambiguation struct {
    results        []interface{}
    annotationText string
    expiresAt      time.Time
}

// disambiguationStore is a simple in-memory map with TTL.
// Chat-scoped, single-user system, so no concurrency concerns.
type disambiguationStore struct {
    mu      sync.Mutex
    pending map[int64]*pendingDisambiguation // keyed by chat_id
}
```

When the user replies with a number (1, 2, or 3), the bot looks up the pending disambiguation, selects the artifact, and applies the annotation.

### Annotation Confirmation Formatting

```go
// formatAnnotationConfirmation formats a human-readable confirmation.
func formatAnnotationConfirmation(created []annotation.Annotation) string {
    var parts []string
    for _, a := range created {
        switch a.Type {
        case annotation.TypeRating:
            stars := strings.Repeat("★", *a.Rating) + strings.Repeat("☆", 5-*a.Rating)
            parts = append(parts, "Rated "+stars)
        case annotation.TypeInteraction:
            parts = append(parts, "Logged: "+humanizeInteraction(a.Interaction))
        case annotation.TypeTagAdd:
            parts = append(parts, "Tagged: #"+a.Tag)
        case annotation.TypeTagRemove:
            parts = append(parts, "Removed tag: #"+a.Tag)
        case annotation.TypeNote:
            note := a.Note
            if len(note) > 50 {
                note = note[:50] + "..."
            }
            parts = append(parts, "Note: "+note)
        }
    }
    return strings.Join(parts, "\n")
}

func humanizeInteraction(i annotation.InteractionType) string {
    switch i {
    case annotation.InteractionMadeIt:   return "Made it"
    case annotation.InteractionBoughtIt: return "Bought it"
    case annotation.InteractionReadIt:   return "Read it"
    case annotation.InteractionVisited:  return "Visited"
    case annotation.InteractionTriedIt:  return "Tried it"
    case annotation.InteractionUsedIt:   return "Used it"
    default: return string(i)
    }
}
```

---

## Search Extension

### Annotation-Aware Search Filters

Extend `SearchFilters` in [internal/api/search.go](../../internal/api/search.go):

```go
type SearchFilters struct {
    Type         string `json:"type,omitempty"`
    DateFrom     string `json:"date_from,omitempty"`
    DateTo        string `json:"date_to,omitempty"`
    Person       string `json:"person,omitempty"`
    Topic        string `json:"topic,omitempty"`
    // NEW: Annotation filters (spec 027)
    MinRating    *int   `json:"min_rating,omitempty"`    // 1-5
    MaxRating    *int   `json:"max_rating,omitempty"`    // 1-5
    Tag          string `json:"tag,omitempty"`            // filter by custom tag
    HasInteraction bool `json:"has_interaction,omitempty"` // only artifacts user has used/made/etc.
    Starred      *bool  `json:"starred,omitempty"`        // backwards compat with user_starred
}
```

### Annotation-Aware Search Results

Extend `SearchResult`:

```go
type SearchResult struct {
    ArtifactID    string   `json:"artifact_id"`
    Title         string   `json:"title"`
    ArtifactType  string   `json:"artifact_type"`
    Summary       string   `json:"summary"`
    SourceURL     string   `json:"source_url,omitempty"`
    Relevance     string   `json:"relevance"`
    Explanation   string   `json:"explanation"`
    CreatedAt     string   `json:"created_at"`
    Topics        []string `json:"topics"`
    Connections   int      `json:"connections"`
    // NEW: Annotation data in results (spec 027)
    Rating        *int     `json:"rating,omitempty"`
    TimesUsed     int      `json:"times_used,omitempty"`
    Tags          []string `json:"tags,omitempty"`
}
```

### Annotation Intent Detection

Add annotation-aware intent detection in `Search()`, similar to the existing `parseTemporalIntent()`:

```go
// parseAnnotationIntent detects annotation-related search queries.
// Returns annotation filters and a cleaned query string.
//
// Examples:
//   "my top rated recipes"      → min_rating=4, type=recipe, query="recipes"
//   "things I've made"          → has_interaction=true, query=""
//   "weeknight dinners"         → tag=weeknight (if user has that tag), query="dinners"
//   "stuff rated 3 or higher"   → min_rating=3, query=""
//   "unrated articles"          → max_rating=0 (special: no rating), query="articles"
func parseAnnotationIntent(query string) *AnnotationIntent {
    lower := strings.ToLower(query)

    var intent AnnotationIntent

    // "top rated", "highest rated", "best rated"
    if strings.Contains(lower, "top rated") || strings.Contains(lower, "highest rated") || strings.Contains(lower, "best") {
        minR := 4
        intent.MinRating = &minR
        intent.Cleaned = removeAnnotationPhrases(query)
    }

    // "things I've made", "stuff I've tried", "what I've cooked"
    if matchesInteractionPhrase(lower) {
        intent.HasInteraction = true
        intent.Cleaned = removeAnnotationPhrases(query)
    }

    // "#tag" in query → direct tag filter
    if tags := tagPattern.FindAllStringSubmatch(query, -1); len(tags) > 0 {
        intent.Tag = strings.ToLower(tags[0][1])
        intent.Cleaned = tagPattern.ReplaceAllString(query, "")
    }

    if intent.MinRating != nil || intent.HasInteraction || intent.Tag != "" {
        return &intent
    }
    return nil
}
```

### Vector Search with Annotation Join

When annotation filters are present, the vector search query joins against `artifact_annotation_summary`:

```go
// vectorSearch with annotation filters joins the materialized view.
query := `
    SELECT a.id, a.title, a.artifact_type, COALESCE(a.summary, ''),
           COALESCE(a.source_url, ''), COALESCE(a.topics::text, '[]'), a.created_at,
           1 - (a.embedding <=> $1::vector) AS similarity,
           aas.current_rating, COALESCE(aas.times_used, 0), COALESCE(aas.tags, '{}')
    FROM artifacts a
    LEFT JOIN artifact_annotation_summary aas ON aas.artifact_id = a.id
    WHERE a.embedding IS NOT NULL`
```

When `min_rating` is set:

```sql
AND aas.current_rating >= $N
```

When `has_interaction` is set:

```sql
AND aas.times_used > 0
```

When `tag` is set:

```sql
AND $N = ANY(aas.tags)
```

### Annotation-Based Relevance Boost

After vector similarity scoring, apply an annotation boost to the final ranking:

```go
// applyAnnotationBoost adjusts similarity scores based on user annotations.
// High-rated artifacts get a boost; frequently-used artifacts get a smaller boost.
// This runs after vector search, before LLM re-ranking.
func applyAnnotationBoost(results []SearchResult) {
    for i := range results {
        boost := 0.0
        if results[i].Rating != nil {
            // Rating boost: 0.0 to 0.05 based on rating (1-5)
            boost += float64(*results[i].Rating-1) * 0.0125
        }
        if results[i].TimesUsed > 0 {
            // Usage boost: up to 0.03 for frequently used items
            usageBoost := float64(results[i].TimesUsed) * 0.01
            if usageBoost > 0.03 {
                usageBoost = 0.03
            }
            boost += usageBoost
        }
        // Apply boost to similarity (capped at 1.0)
        // The similarity field is stored as explanation text, so we track boost separately.
        results[i].annotationBoost = boost
    }
}
```

The boost is intentionally small (max 0.08) so annotations influence but don't overwhelm semantic relevance.

---

## Intelligence Integration

### Annotation Signal Consumer

The intelligence engine subscribes to `annotations.created` and uses annotation data in its scoring and recommendation workflows.

**New file: `internal/intelligence/annotations.go`**

```go
package intelligence

import (
    "context"
    "encoding/json"
    "log/slog"

    "github.com/smackerel/smackerel/internal/annotation"
    smacknats "github.com/smackerel/smackerel/internal/nats"
)

// SubscribeAnnotations starts a NATS subscription for annotation events.
// Updates the artifact's relevance_score based on annotation signals.
func (e *Engine) SubscribeAnnotations(ctx context.Context) error {
    return e.NATS.Subscribe(ctx, smacknats.SubjectAnnotationsCreated, func(data []byte) {
        var ann annotation.Annotation
        if err := json.Unmarshal(data, &ann); err != nil {
            slog.Warn("invalid annotation event payload", "error", err)
            return
        }

        if err := e.updateRelevanceFromAnnotation(ctx, &ann); err != nil {
            slog.Warn("relevance update from annotation failed",
                "artifact_id", ann.ArtifactID, "error", err)
        }
    })
}

// updateRelevanceFromAnnotation adjusts an artifact's relevance_score
// based on new annotation data.
func (e *Engine) updateRelevanceFromAnnotation(ctx context.Context, ann *annotation.Annotation) error {
    if e.Pool == nil {
        return nil
    }

    // Fetch current relevance and annotation summary
    var currentRelevance float64
    err := e.Pool.QueryRow(ctx,
        "SELECT relevance_score FROM artifacts WHERE id = $1", ann.ArtifactID,
    ).Scan(&currentRelevance)
    if err != nil {
        return err
    }

    // Calculate annotation-based relevance adjustment
    delta := annotationRelevanceDelta(ann)
    newRelevance := currentRelevance + delta
    if newRelevance < 0 {
        newRelevance = 0
    }
    if newRelevance > 1 {
        newRelevance = 1
    }

    _, err = e.Pool.Exec(ctx,
        "UPDATE artifacts SET relevance_score = $1, updated_at = NOW() WHERE id = $2",
        newRelevance, ann.ArtifactID)
    return err
}

// annotationRelevanceDelta returns the relevance score adjustment for an annotation event.
func annotationRelevanceDelta(ann *annotation.Annotation) float64 {
    switch ann.Type {
    case annotation.TypeRating:
        if ann.Rating == nil {
            return 0
        }
        // Rating 5 → +0.15, Rating 4 → +0.10, Rating 3 → +0.05, Rating 2 → 0, Rating 1 → -0.05
        return (float64(*ann.Rating) - 2.5) * 0.06
    case annotation.TypeInteraction:
        // Any interaction is a strong positive signal
        return 0.10
    case annotation.TypeTagAdd:
        // Tagging implies engagement
        return 0.02
    case annotation.TypeNote:
        // Adding a note implies engagement
        return 0.03
    default:
        return 0
    }
}
```

### Recommendation Signals

The intelligence engine's recommendation methods (expertise, serendipity, content fuel, seasonal patterns) gain access to annotation data through the materialized view:

1. **Preference learning** — High-rated artifacts in a topic boost that topic's weight in the user's preference profile. The existing `topics` JSONB on artifacts + the annotation rating create a `topic → average_user_rating` signal.

2. **Resurfacing** — Artifacts saved more than N days ago with no annotation events are candidates for "Still interested in...?" resurfacing in digests. Query:

```sql
SELECT a.id, a.title, a.artifact_type, a.created_at
FROM artifacts a
LEFT JOIN artifact_annotation_summary aas ON aas.artifact_id = a.id
WHERE aas.artifact_id IS NULL           -- no annotations at all
  AND a.created_at < NOW() - INTERVAL '30 days'
  AND a.artifact_type NOT IN ('note', 'idea')  -- skip ephemeral types
ORDER BY a.relevance_score DESC
LIMIT 5
```

3. **Variety signal** — Recently-used artifacts (last 7 days) get a negative weight for immediate re-recommendation. This prevents the digest from repeatedly suggesting the same recipe the user just made.

4. **Taste profile** — For future recommendation queries, build a per-topic preference vector:

```sql
SELECT t.topic, AVG(aas.current_rating) AS avg_rating, SUM(aas.times_used) AS total_uses
FROM artifacts a
JOIN artifact_annotation_summary aas ON aas.artifact_id = a.id
CROSS JOIN LATERAL jsonb_array_elements_text(a.topics) AS t(topic)
WHERE aas.current_rating IS NOT NULL
GROUP BY t.topic
ORDER BY avg_rating DESC, total_uses DESC
```

---

## Telegram Bot Changes Summary

### New Bot Fields

```go
type Bot struct {
    // ... existing fields ...
    annotationURL     string                    // internal API URL for annotations
    disambiguationMu  sync.Mutex
    pendingDisambig   map[int64]*pendingDisambiguation
}
```

Add `annotationURL` initialization in `NewBot()`:

```go
bot.annotationURL = baseURL + "/api/artifacts"
```

### Config Addition

Add to `config/smackerel.yaml` under `telegram:`:

```yaml
telegram:
  # ... existing fields ...
  disambiguation_timeout_seconds: 120   # TTL for /rate disambiguation prompts
```

### Updated Command Registration

```go
commands := tgbotapi.NewSetMyCommands(
    // ... existing commands ...
    tgbotapi.BotCommand{Command: "rate", Description: "Rate or annotate an artifact"},
)
```

### Message Flow Priority

Updated `handleMessage()` routing order:

1. Check allowlist
2. **NEW: Reply-to annotation** (if `msg.ReplyToMessage != nil && !msg.IsCommand()`)
3. **NEW: Disambiguation resolution** (if pending disambiguation for this chat and message is numeric)
4. Commands (`/find`, `/rate`, `/concept`, etc.)
5. Media groups
6. Forwarded messages
7. Voice notes
8. Photos
9. Documents
10. URLs
11. Plain text

---

## Dependencies Struct Update

Add to `Dependencies` in [internal/api/health.go](../../internal/api/health.go):

```go
type Dependencies struct {
    // ... existing fields ...

    // Annotation layer (spec 027)
    AnnotationStore annotation.AnnotationQuerier
}
```

---

## Configuration

### `config/smackerel.yaml` Additions

```yaml
annotations:
  matview_refresh_timeout_s: 5   # max seconds for REFRESH MATERIALIZED VIEW CONCURRENTLY
  max_tags_per_artifact: 50      # prevent tag spam
  max_note_length: 2000          # truncate notes beyond this length
  relevance_boost_rating: 0.06   # per-rating-point relevance delta (centered at 2.5)
  relevance_boost_interaction: 0.10
  relevance_boost_tag: 0.02
  relevance_boost_note: 0.03

telegram:
  # ... existing fields ...
  disambiguation_timeout_seconds: 120
```

All values originate from `config/smackerel.yaml`. Zero hardcoded defaults in Go code — read from generated env vars via the standard config pipeline.

---

## Web UI Integration Points

The existing web UI ([internal/web/](../../internal/web/)) artifact detail page gains annotation display and input. This is additive:

1. **Artifact detail page** — Show annotation summary (rating stars, tags, usage count, last used) below the existing artifact metadata section. Add a simple form for adding annotations.

2. **Search results** — Show rating stars and tag badges inline with each result.

3. **No new routes** — Web UI reads annotation data from the existing API endpoints (`/api/artifacts/{id}/annotations/summary`).

---

## Backwards Compatibility

### `user_starred` Migration

The existing `user_starred BOOLEAN` column on `artifacts` is preserved. During the transition period:

1. Search and intelligence treat `user_starred = TRUE` as equivalent to `current_rating = 5` when the artifact has no explicit rating annotation.
2. The annotation API does not modify `user_starred` — they are separate signals.
3. A future migration can optionally convert `user_starred = TRUE` rows into `TypeRating` annotation events with rating 5, but this is not required for launch.

### Search API Backwards Compatibility

The new `SearchResult` fields (`rating`, `times_used`, `tags`) are optional in the JSON response. Existing clients that don't parse these fields continue to work unchanged.

---

## File Map

| File | Purpose | Status |
|------|---------|--------|
| `internal/annotation/types.go` | Go types, constants, enums | New |
| `internal/annotation/parser.go` | Freeform annotation parser | New |
| `internal/annotation/parser_test.go` | Parser unit tests | New |
| `internal/annotation/store.go` | DB operations, NATS publish | New |
| `internal/annotation/store_test.go` | Store unit tests | New |
| `internal/api/annotations.go` | REST API handlers | New |
| `internal/api/annotations_test.go` | API handler tests | New |
| `internal/api/router.go` | Add annotation routes | Modify |
| `internal/api/health.go` | Add `AnnotationStore` to `Dependencies` | Modify |
| `internal/api/search.go` | Annotation filters, result fields, intent detection | Modify |
| `internal/intelligence/annotations.go` | Annotation → relevance scoring | New |
| `internal/intelligence/annotations_test.go` | Intelligence annotation tests | New |
| `internal/telegram/bot.go` | Reply-to handler, `/rate` command, disambiguation | Modify |
| `internal/telegram/annotation.go` | Telegram annotation helpers | New |
| `internal/telegram/annotation_test.go` | Telegram annotation tests | New |
| `internal/nats/client.go` | Add `SubjectAnnotationsCreated` constant | Modify |
| `internal/db/migrations/015_user_annotations.sql` | Schema migration | New |
| `config/smackerel.yaml` | Add `annotations:` section | Modify |
| `config/nats_contract.json` | Add `annotations.created` subject | Modify |
| `cmd/core/main.go` | Wire annotation store, start subscriber | Modify |
| `cmd/core/services.go` | Initialize annotation store | Modify |

---

## Testing Strategy

### Unit Tests

| Test | Package | What it validates |
|------|---------|-------------------|
| `TestParse_RatingOnly` | `annotation` | "4/5" → rating 4 |
| `TestParse_FullAnnotation` | `annotation` | "4/5 made it #weeknight needs more garlic" → all components |
| `TestParse_TagsOnly` | `annotation` | "#quick #weeknight" → two tags |
| `TestParse_TagRemoval` | `annotation` | "#remove-quick" → tag removal |
| `TestParse_InteractionOnly` | `annotation` | "made it" → interaction made_it |
| `TestParse_NoteOnly` | `annotation` | "needs more garlic" → note |
| `TestParse_EmptyInput` | `annotation` | "" → zero-valued ParsedAnnotation |
| `TestParse_RatingOutOfRange` | `annotation` | "6/5" → no rating (regex doesn't match) |
| `TestParse_MultipleInteractions` | `annotation` | Only first interaction matched |
| `TestCreateAnnotationHandler_ValidRating` | `api` | POST with rating → 201 + events |
| `TestCreateAnnotationHandler_InvalidRating` | `api` | Rating 0 or 6 → 400 |
| `TestCreateAnnotationHandler_EmptyBody` | `api` | No fields → 400 |
| `TestCreateAnnotationHandler_ArtifactNotFound` | `api` | Missing artifact → 404 |
| `TestDeleteTagHandler` | `api` | DELETE tag → tag_remove event + refreshed view |
| `TestAnnotationRelevanceDelta` | `intelligence` | Rating/interaction/tag deltas correct |
| `TestParseAnnotationIntent_TopRated` | `api` | "my top rated recipes" → min_rating=4 |
| `TestParseAnnotationIntent_ThingsIveMade` | `api` | "things I've made" → has_interaction=true |

### Integration Tests

| Test | What it validates |
|------|-------------------|
| Annotation → materialized view refresh → search returns updated data |
| Annotation → NATS event → intelligence subscriber → relevance_score updated |
| Telegram reply-to → artifact resolution → annotation recorded |
| `/rate` command → search → disambiguation → annotation recorded |
| Multiple annotations on same artifact → summary correctly aggregated |
| Tag add + tag remove → view shows tag removed |

### E2E Tests

| Test | What it validates |
|------|-------------------|
| Full flow: capture artifact → annotate via API → search with annotation filter → correct results |
| Full flow: capture → get confirmation message ID → reply with "5/5 amazing" → annotation recorded |

---

## Scope Boundaries

### In Scope (this spec)
- `annotations` table, materialized view, `telegram_message_artifacts` table
- Annotation parser (Go)
- REST API endpoints for CRUD
- Telegram reply-to handler, `/rate` command, disambiguation flow
- Search filter extension (min_rating, tag, has_interaction)
- Annotation intent detection in search queries
- Intelligence relevance scoring from annotation events
- NATS event publication
- Backwards compatibility with `user_starred`

### Out of Scope
- Collaborative/multi-user annotations
- Photo annotations (IP-002 from spec)
- Modification tracking (IP-003 from spec)
- Mood/context tagging (IP-001 from spec)
- Annotation analytics or visualization dashboard
- Automated annotation inference
- Web UI annotation forms (additive, can follow as enhancement)
