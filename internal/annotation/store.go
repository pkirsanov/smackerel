package annotation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// Store manages annotation CRUD operations in PostgreSQL.
type Store struct {
	Pool *pgxpool.Pool
	NATS *smacknats.Client
}

// Compile-time assertion: *Store implements AnnotationQuerier.
var _ AnnotationQuerier = (*Store)(nil)

// NewStore creates a new annotation store.
func NewStore(pool *pgxpool.Pool, nc *smacknats.Client) *Store {
	return &Store{Pool: pool, NATS: nc}
}

// Create inserts a single annotation event.
func (s *Store) Create(ctx context.Context, ann *Annotation) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO annotations (id, artifact_id, annotation_type, rating, note, tag, interaction_type, source_channel, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, ann.ID, ann.ArtifactID, ann.AnnotationType, ann.Rating, ann.Note,
		ann.Tag, ann.InteractionType, ann.SourceChannel, ann.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert annotation: %w", err)
	}
	return nil
}

// CreateFromParsed creates annotation events from a parsed freeform string.
// Returns the created annotations.
func (s *Store) CreateFromParsed(ctx context.Context, artifactID string, parsed ParsedAnnotation, channel SourceChannel) ([]Annotation, error) {
	var created []Annotation
	now := time.Now()

	if parsed.Rating != nil {
		ann := Annotation{
			ID:             fmt.Sprintf("ann-%s-%d-rating", artifactID[:min(8, len(artifactID))], now.UnixNano()),
			ArtifactID:     artifactID,
			AnnotationType: TypeRating,
			Rating:         parsed.Rating,
			SourceChannel:  channel,
			CreatedAt:      now,
		}
		if err := s.Create(ctx, &ann); err != nil {
			return nil, err
		}
		created = append(created, ann)
	}

	if parsed.InteractionType != "" {
		ann := Annotation{
			ID:              fmt.Sprintf("ann-%s-%d-interaction", artifactID[:min(8, len(artifactID))], now.UnixNano()),
			ArtifactID:      artifactID,
			AnnotationType:  TypeInteraction,
			InteractionType: parsed.InteractionType,
			SourceChannel:   channel,
			CreatedAt:       now,
		}
		if err := s.Create(ctx, &ann); err != nil {
			return nil, err
		}
		created = append(created, ann)
	}

	for _, tag := range parsed.Tags {
		ann := Annotation{
			ID:             fmt.Sprintf("ann-%s-%d-tag-%s", artifactID[:min(8, len(artifactID))], now.UnixNano(), tag),
			ArtifactID:     artifactID,
			AnnotationType: TypeTagAdd,
			Tag:            tag,
			SourceChannel:  channel,
			CreatedAt:      now,
		}
		if err := s.Create(ctx, &ann); err != nil {
			return nil, err
		}
		created = append(created, ann)
	}

	for _, tag := range parsed.RemovedTags {
		ann := Annotation{
			ID:             fmt.Sprintf("ann-%s-%d-untag-%s", artifactID[:min(8, len(artifactID))], now.UnixNano(), tag),
			ArtifactID:     artifactID,
			AnnotationType: TypeTagRemove,
			Tag:            tag,
			SourceChannel:  channel,
			CreatedAt:      now,
		}
		if err := s.Create(ctx, &ann); err != nil {
			return nil, err
		}
		created = append(created, ann)
	}

	if parsed.Note != "" {
		ann := Annotation{
			ID:             fmt.Sprintf("ann-%s-%d-note", artifactID[:min(8, len(artifactID))], now.UnixNano()),
			ArtifactID:     artifactID,
			AnnotationType: TypeNote,
			Note:           parsed.Note,
			SourceChannel:  channel,
			CreatedAt:      now,
		}
		if err := s.Create(ctx, &ann); err != nil {
			return nil, err
		}
		created = append(created, ann)
	}

	// Refresh materialized view (best-effort)
	if err := s.RefreshSummary(ctx); err != nil {
		slog.Warn("failed to refresh annotation summary view", "error", err)
	}

	// Publish NATS events for each created annotation (best-effort)
	if s.NATS != nil {
		for i := range created {
			data, err := json.Marshal(created[i])
			if err != nil {
				slog.Warn("failed to marshal annotation event", "error", err)
				continue
			}
			if err := s.NATS.Publish(ctx, smacknats.SubjectAnnotationsCreated, data); err != nil {
				slog.Warn("failed to publish annotation event", "error", err)
			}
		}
	}

	return created, nil
}

// GetHistory returns the annotation history for an artifact, most recent first.
func (s *Store) GetHistory(ctx context.Context, artifactID string, limit int) ([]Annotation, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT id, artifact_id, annotation_type, rating, COALESCE(note, ''),
		       COALESCE(tag, ''), COALESCE(interaction_type, ''), source_channel, created_at
		FROM annotations WHERE artifact_id = $1
		ORDER BY created_at DESC LIMIT $2
	`, artifactID, limit)
	if err != nil {
		return nil, fmt.Errorf("query annotations: %w", err)
	}
	defer rows.Close()

	var annotations []Annotation
	for rows.Next() {
		var a Annotation
		var rating *int
		if err := rows.Scan(&a.ID, &a.ArtifactID, &a.AnnotationType, &rating,
			&a.Note, &a.Tag, &a.InteractionType, &a.SourceChannel, &a.CreatedAt); err != nil {
			continue
		}
		a.Rating = rating
		annotations = append(annotations, a)
	}
	return annotations, rows.Err()
}

// GetSummary returns the aggregated annotation summary for an artifact.
func (s *Store) GetSummary(ctx context.Context, artifactID string) (*Summary, error) {
	var sum Summary
	sum.ArtifactID = artifactID

	err := s.Pool.QueryRow(ctx, `
		SELECT current_rating, average_rating, rating_count,
		       times_used, last_used, COALESCE(tags, '{}'), notes_count
		FROM artifact_annotation_summary WHERE artifact_id = $1
	`, artifactID).Scan(&sum.CurrentRating, &sum.AverageRating, &sum.RatingCount,
		&sum.TimesUsed, &sum.LastUsed, &sum.Tags, &sum.NotesCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &sum, nil
		}
		return nil, fmt.Errorf("get annotation summary: %w", err)
	}
	return &sum, nil
}

// RefreshSummary refreshes the materialized view.
func (s *Store) RefreshSummary(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `REFRESH MATERIALIZED VIEW CONCURRENTLY artifact_annotation_summary`)
	if err != nil {
		return fmt.Errorf("refresh annotation summary: %w", err)
	}
	return nil
}

// DeleteTag removes a tag from an artifact by inserting a tag_remove event.
// This preserves the append-only log — the tag_remove event neutralizes the tag_add.
func (s *Store) DeleteTag(ctx context.Context, artifactID, tag string, channel SourceChannel) error {
	ann := Annotation{
		ID:             fmt.Sprintf("ann-%s-%d-untag-%s", artifactID[:min(8, len(artifactID))], time.Now().UnixNano(), tag),
		ArtifactID:     artifactID,
		AnnotationType: TypeTagRemove,
		Tag:            tag,
		SourceChannel:  channel,
		CreatedAt:      time.Now(),
	}
	if err := s.Create(ctx, &ann); err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}

	if err := s.RefreshSummary(ctx); err != nil {
		slog.Warn("failed to refresh annotation summary after tag delete", "error", err)
	}

	// Publish NATS event (best-effort)
	if s.NATS != nil {
		data, _ := json.Marshal(ann)
		if err := s.NATS.Publish(ctx, smacknats.SubjectAnnotationsCreated, data); err != nil {
			slog.Warn("failed to publish tag removal event", "error", err)
		}
	}

	return nil
}

// RecordMessageArtifact stores a Telegram message → artifact mapping.
func (s *Store) RecordMessageArtifact(ctx context.Context, messageID, chatID int64, artifactID string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO telegram_message_artifacts (message_id, chat_id, artifact_id, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (message_id, chat_id) DO UPDATE SET artifact_id = $3
	`, messageID, chatID, artifactID)
	if err != nil {
		return fmt.Errorf("record message artifact: %w", err)
	}
	return nil
}

// ResolveArtifactFromMessage looks up which artifact a Telegram message refers to.
func (s *Store) ResolveArtifactFromMessage(ctx context.Context, messageID, chatID int64) (string, error) {
	var artifactID string
	err := s.Pool.QueryRow(ctx, `
		SELECT artifact_id FROM telegram_message_artifacts
		WHERE message_id = $1 AND chat_id = $2
	`, messageID, chatID).Scan(&artifactID)
	if err != nil {
		return "", fmt.Errorf("resolve artifact from message: %w", err)
	}
	return artifactID, nil
}
