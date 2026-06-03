// Package annotation provides the annotation model, parser, and store
// for user annotations on artifacts (ratings, notes, tags, interactions).
package annotation

import (
	"context"
	"time"
)

// AnnotationQuerier defines the interface for annotation CRUD operations.
// Implemented by Store; consumed by API handlers and other packages
// that need annotation operations without depending on the concrete store.
type AnnotationQuerier interface {
	CreateFromParsed(ctx context.Context, artifactID string, parsed ParsedAnnotation, channel SourceChannel) ([]Annotation, error)
	// CreateFromParsedAs is the spec 027 scope 9 variant that persists
	// the bearer subject as the annotation's actor_id. Telegram and
	// extension paths continue to call CreateFromParsed (legacy);
	// browser/PWA writes call this so the audit trail records the
	// authenticated principal (PLAN-9-02).
	CreateFromParsedAs(ctx context.Context, artifactID string, parsed ParsedAnnotation, channel SourceChannel, actorID string) ([]Annotation, error)
	GetSummary(ctx context.Context, artifactID string) (*Summary, error)
	// GetSummaryVersion returns the monotonic per-artifact version
	// maintained by the annotation_summary_version trigger. Absent
	// row → 0 (clean cold-start semantic, not a fallback default).
	GetSummaryVersion(ctx context.Context, artifactID string) (int64, error)
	GetHistory(ctx context.Context, artifactID string, limit int) ([]Annotation, error)
	// ListByActor returns annotations authored by the given actor_id
	// across all artifacts, most recent first. Backed by
	// idx_annotations_actor_created.
	ListByActor(ctx context.Context, actorID string, limit int, since *time.Time) ([]Annotation, error)
	DeleteTag(ctx context.Context, artifactID, tag string, channel SourceChannel) error
	RecordMessageArtifact(ctx context.Context, messageID, chatID int64, artifactID string) error
	ResolveArtifactFromMessage(ctx context.Context, messageID, chatID int64) (string, error)
}

// AnnotationType identifies the kind of annotation event.
type AnnotationType string

const (
	TypeRating       AnnotationType = "rating"
	TypeNote         AnnotationType = "note"
	TypeTagAdd       AnnotationType = "tag_add"
	TypeTagRemove    AnnotationType = "tag_remove"
	TypeInteraction  AnnotationType = "interaction"
	TypeStatusChange AnnotationType = "status_change"
)

// InteractionType identifies the kind of real-world usage.
type InteractionType string

const (
	InteractionMadeIt   InteractionType = "made_it"
	InteractionBoughtIt InteractionType = "bought_it"
	InteractionReadIt   InteractionType = "read_it"
	InteractionVisited  InteractionType = "visited"
	InteractionTriedIt  InteractionType = "tried_it"
	InteractionUsedIt   InteractionType = "used_it"
)

// SourceChannel identifies where the annotation came from.
type SourceChannel string

const (
	ChannelTelegram  SourceChannel = "telegram"
	ChannelAPI       SourceChannel = "api"
	ChannelWeb       SourceChannel = "web"
	ChannelExtension SourceChannel = "extension"
)

// Annotation is a single event in the annotation log.
type Annotation struct {
	ID              string          `json:"id"`
	ArtifactID      string          `json:"artifact_id"`
	AnnotationType  AnnotationType  `json:"annotation_type"`
	Rating          *int            `json:"rating,omitempty"`
	Note            string          `json:"note,omitempty"`
	Tag             string          `json:"tag,omitempty"`
	InteractionType InteractionType `json:"interaction_type,omitempty"`
	SourceChannel   SourceChannel   `json:"source_channel"`
	// ActorID is the authenticated bearer subject that produced this
	// annotation. Empty for legacy/pre-spec-044 rows and for
	// telegram/extension paths that have not been migrated yet.
	// Spec 027 scope 9 PLAN-9-02.
	ActorID   string    `json:"actor_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Summary is the pre-aggregated annotation state for an artifact.
type Summary struct {
	ArtifactID    string     `json:"artifact_id"`
	CurrentRating *int       `json:"current_rating,omitempty"`
	AverageRating *float32   `json:"average_rating,omitempty"`
	RatingCount   int        `json:"rating_count"`
	TimesUsed     int        `json:"times_used"`
	LastUsed      *time.Time `json:"last_used,omitempty"`
	Tags          []string   `json:"tags"`
	NotesCount    int        `json:"notes_count"`
	TotalEvents   int        `json:"total_events"`
	LastAnnotated *time.Time `json:"last_annotated,omitempty"`
	// Version is the monotonic per-artifact counter maintained by the
	// annotation_summary_version trigger (spec 027 scope 9 PLAN-9-05).
	// Used by the UI as an If-Match precondition for stale-edit
	// conflict detection. Absent row → 0.
	Version int64 `json:"version"`
}
