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
	GetSummary(ctx context.Context, artifactID string) (*Summary, error)
	GetHistory(ctx context.Context, artifactID string, limit int) ([]Annotation, error)
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
	ChannelTelegram SourceChannel = "telegram"
	ChannelAPI      SourceChannel = "api"
	ChannelWeb      SourceChannel = "web"
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
	CreatedAt       time.Time       `json:"created_at"`
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
}
