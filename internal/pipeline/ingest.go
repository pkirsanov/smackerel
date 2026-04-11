package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/extract"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/stringutil"
)

// RawArtifactPublisher bridges connector-produced RawArtifacts into the
// processing pipeline by storing the initial artifact in PostgreSQL and
// publishing to NATS for ML sidecar processing.
type RawArtifactPublisher struct {
	DB   *pgxpool.Pool
	NATS *smacknats.Client
}

// NewRawArtifactPublisher creates a publisher for connector artifacts.
func NewRawArtifactPublisher(db *pgxpool.Pool, nats *smacknats.Client) *RawArtifactPublisher {
	return &RawArtifactPublisher{DB: db, NATS: nats}
}

// PublishRawArtifact converts a connector RawArtifact into the processing pipeline.
// It stores the initial artifact in PostgreSQL and publishes to NATS for ML processing.
func (p *RawArtifactPublisher) PublishRawArtifact(ctx context.Context, artifact connector.RawArtifact) (string, error) {
	artifactID := ulid.Make().String()

	contentHash := extract.HashContent(artifact.RawContent)

	// Dedup by source_ref (connector-specific unique ID like message UID, video ID, event UID)
	if artifact.SourceRef != "" {
		var existingID string
		err := p.DB.QueryRow(ctx,
			"SELECT id FROM artifacts WHERE source_url = $1 AND content_hash = $2 LIMIT 1",
			artifact.SourceRef, contentHash,
		).Scan(&existingID)
		if err == nil {
			slog.Debug("connector artifact already exists, skipping",
				"source_id", artifact.SourceID,
				"source_ref", artifact.SourceRef,
				"existing_id", existingID,
			)
			return existingID, nil
		}
		// pgx.ErrNoRows is expected — not a duplicate
	}

	// Resolve processing tier from connector metadata or default
	tier := resolveTierFromMetadata(artifact.Metadata)

	// Truncate content to 500KB (same as processor.go)
	contentRaw := artifact.RawContent
	const maxContentRaw = 500 * 1024
	if len(contentRaw) > maxContentRaw {
		contentRaw = stringutil.TruncateUTF8(contentRaw, maxContentRaw)
	}

	// Store initial artifact
	ct, err := p.DB.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, source_url, processing_tier, capture_method, processing_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (content_hash) WHERE content_hash IS NOT NULL DO NOTHING
	`, artifactID, artifact.ContentType, artifact.Title, contentRaw, contentHash,
		artifact.SourceID, artifact.URL, tier, "passive", string(StatusPending))
	if err != nil {
		return "", fmt.Errorf("insert connector artifact: %w", err)
	}

	if ct.RowsAffected() == 0 {
		slog.Debug("connector artifact deduped by content hash",
			"source_id", artifact.SourceID,
			"source_ref", artifact.SourceRef,
		)
		return "", nil
	}

	// Build NATS payload
	payload := NATSProcessPayload{
		ArtifactID:     artifactID,
		ContentType:    artifact.ContentType,
		URL:            artifact.URL,
		RawText:        contentRaw,
		ProcessingTier: tier,
		SourceID:       artifact.SourceID,
		Metadata:       artifact.Metadata,
	}

	if err := ValidateProcessPayload(&payload); err != nil {
		return "", fmt.Errorf("validate NATS payload: %w", err)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal NATS payload: %w", err)
	}

	if err := p.NATS.Publish(ctx, smacknats.SubjectArtifactsProcess, data); err != nil {
		// Clean up orphaned artifact on NATS publish failure
		if _, cleanupErr := p.DB.Exec(ctx, "DELETE FROM artifacts WHERE id = $1", artifactID); cleanupErr != nil {
			slog.Error("failed to clean up orphaned connector artifact", "artifact_id", artifactID, "error", cleanupErr)
		}
		return "", fmt.Errorf("publish connector artifact to NATS: %w", err)
	}

	slog.Info("connector artifact submitted for processing",
		"artifact_id", artifactID,
		"source_id", artifact.SourceID,
		"content_type", artifact.ContentType,
		"tier", tier,
	)

	return artifactID, nil
}

// resolveTierFromMetadata extracts the processing tier from connector metadata,
// falling back to "standard" if not specified.
func resolveTierFromMetadata(metadata map[string]interface{}) string {
	if metadata == nil {
		return string(TierStandard)
	}
	if tier, ok := metadata["processing_tier"].(string); ok && tier != "" {
		return tier
	}
	return string(TierStandard)
}
