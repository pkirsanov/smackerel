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
	"github.com/smackerel/smackerel/internal/retrieval/evergreen"
	"github.com/smackerel/smackerel/internal/stringutil"
)

// RawArtifactPublisher bridges connector-produced RawArtifacts into the
// processing pipeline by storing the initial artifact in PostgreSQL and
// publishing to NATS for ML sidecar processing.
type RawArtifactPublisher struct {
	DB   *pgxpool.Pool
	NATS *smacknats.Client

	// Scorer is the OPTIONAL evergreen-signal scorer (spec 095 SCOPE-07 /
	// PKT-095-B). nil-safe: when nil, PublishRawArtifact persists a NULL
	// evergreen_score/evergreen_source (the artifact is "not yet scored" ⇒
	// treated as evergreen downstream, Principle 9) and changes NOTHING else
	// about ingestion (NFR-3 — byte-for-byte the prior behaviour). Injected in
	// cmd/core from the fail-loud SST evergreen config + the agent-bridge judge.
	Scorer EvergreenScorer
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

	// Spec 095 SCOPE-07 / PKT-095-B — ADDITIVELY score the artifact's
	// evergreen-vs-ephemeral signal at the LIVE ingestion front door and
	// persist it. nil-safe: a nil Scorer returns (nil, nil) ⇒ both columns are
	// left NULL and NOTHING else changes (the tier outcome and every existing
	// field are byte-for-byte unchanged — NFR-3). Scoring NEVER blocks
	// ingestion (R13); a scenario-judge error degrades to the deterministic
	// fallback inside Scorer.Score.
	evergreenScore, evergreenSource := p.scoreEvergreen(ctx, artifactID, artifact)

	// Truncate content to 500KB (same as processor.go)
	contentRaw := artifact.RawContent
	const maxContentRaw = 500 * 1024
	if len(contentRaw) > maxContentRaw {
		contentRaw = stringutil.TruncateUTF8(contentRaw, maxContentRaw)
	}

	// Store initial artifact
	var metadataJSON []byte
	var err error
	if artifact.Metadata != nil {
		metadataJSON, err = json.Marshal(artifact.Metadata)
		if err != nil {
			return "", fmt.Errorf("marshal connector metadata: %w", err)
		}
	}
	ct, err := p.DB.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, source_url, processing_tier, capture_method, processing_status, metadata, evergreen_score, evergreen_source)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (content_hash) WHERE content_hash IS NOT NULL DO NOTHING
	`, artifactID, artifact.ContentType, artifact.Title, contentRaw, contentHash,
		artifact.SourceID, artifact.URL, tier, "passive", string(StatusPending), metadataJSON,
		evergreenScore, evergreenSource)
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

	if len(data) > MaxNATSMessageSize {
		slog.Warn("NATS payload exceeds max message size",
			"artifact_id", artifactID,
			"payload_size", len(data),
			"max_size", MaxNATSMessageSize,
			"source_id", artifact.SourceID,
		)
		return "", fmt.Errorf("NATS payload too large: %d bytes exceeds max %d", len(data), MaxNATSMessageSize)
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

// scoreEvergreen computes the (evergreen_score, evergreen_source) values to
// persist for an artifact at the live ingestion front door (spec 095 SCOPE-07 /
// PKT-095-B). nil-safe: a nil Scorer returns (nil, nil) ⇒ both columns are left
// NULL ⇒ the artifact is "not yet scored" (treated as evergreen / not-excluded
// downstream, Principle 9). The score NEVER blocks ingestion (R13) — Scorer.Score
// always returns a signal (scenario judgment or deterministic fallback).
func (p *RawArtifactPublisher) scoreEvergreen(ctx context.Context, artifactID string, artifact connector.RawArtifact) (*float64, *string) {
	if p.Scorer == nil {
		return nil, nil
	}
	cand := buildEvergreenCandidate(artifact)
	cand.ArtifactID = artifactID
	sig := p.Scorer.Score(ctx, cand)
	score := sig.PersistedScore()
	source := sig.Source
	return &score, &source
}

// buildEvergreenCandidate derives the deterministic front-door signals for the
// evergreen judgment from a connector RawArtifact: SourceKind from the source
// id, ContentLen from the raw content, UserStarred/HasContext from connector
// metadata. Pure — NO business threshold is applied here; the Scorer (scenario
// judge or deterministic fallback) decides evergreen vs ephemeral.
func buildEvergreenCandidate(artifact connector.RawArtifact) evergreen.EvergreenCandidate {
	return evergreen.EvergreenCandidate{
		SourceKind:  artifact.SourceID,
		ContentLen:  len(artifact.RawContent),
		UserStarred: metadataBool(artifact.Metadata, "user_starred"),
		HasContext:  metadataBool(artifact.Metadata, "has_context"),
	}
}

// metadataBool reads a boolean connector-metadata flag, defaulting to false
// when absent or not a bool. Additive — no existing metadata key is required,
// so connectors that do not set these flags keep their exact prior behaviour.
func metadataBool(metadata map[string]interface{}, key string) bool {
	if metadata == nil {
		return false
	}
	v, ok := metadata[key].(bool)
	return ok && v
}
