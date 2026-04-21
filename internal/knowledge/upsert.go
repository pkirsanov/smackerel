package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oklog/ulid/v2"
)

// UpsertConcept finds a concept by normalized title. If it exists, merges new claims
// and adds the artifact as a source. If not, creates a new concept page.
// Designed to run within a transaction for atomicity.
func (ks *KnowledgeStore) UpsertConcept(ctx context.Context, tx pgx.Tx, name, description string, newClaims []Claim, artifactID, sourceType, contractVersion string) (*ConceptPage, error) {
	normalized := normalizeName(name)

	// Try to load existing concept
	var existing ConceptPage
	err := tx.QueryRow(ctx, `
		SELECT id, title, title_normalized, summary, claims, related_concept_ids,
		       source_artifact_ids, source_type_diversity, token_count,
		       prompt_contract_version, created_at, updated_at
		FROM knowledge_concepts WHERE title_normalized = $1
		FOR UPDATE`, normalized,
	).Scan(
		&existing.ID, &existing.Title, &existing.TitleNormalized, &existing.Summary,
		&existing.Claims, &existing.RelatedConceptIDs, &existing.SourceArtifactIDs,
		&existing.SourceTypeDiversity, &existing.TokenCount,
		&existing.PromptContractVersion, &existing.CreatedAt, &existing.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		// Create new concept page
		return ks.createConceptInTx(ctx, tx, name, description, newClaims, artifactID, sourceType, contractVersion)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup concept %q: %w", name, err)
	}

	// Merge into existing concept
	return ks.mergeConceptInTx(ctx, tx, &existing, description, newClaims, artifactID, sourceType, contractVersion)
}

func (ks *KnowledgeStore) createConceptInTx(ctx context.Context, tx pgx.Tx, name, description string, claims []Claim, artifactID, sourceType, contractVersion string) (*ConceptPage, error) {
	id := ulid.Make().String()
	now := time.Now().UTC()

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("marshal claims: %w", err)
	}

	concept := &ConceptPage{
		ID:                    id,
		Title:                 name,
		TitleNormalized:       normalizeName(name),
		Summary:               description,
		Claims:                claimsJSON,
		RelatedConceptIDs:     []string{},
		SourceArtifactIDs:     []string{artifactID},
		SourceTypeDiversity:   []string{sourceType},
		TokenCount:            estimateTokens(description, claimsJSON),
		PromptContractVersion: contractVersion,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO knowledge_concepts
			(id, title, title_normalized, summary, claims, related_concept_ids,
			 source_artifact_ids, source_type_diversity, token_count,
			 prompt_contract_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		concept.ID, concept.Title, concept.TitleNormalized, concept.Summary,
		concept.Claims, concept.RelatedConceptIDs,
		concept.SourceArtifactIDs, concept.SourceTypeDiversity,
		concept.TokenCount, concept.PromptContractVersion,
		concept.CreatedAt, concept.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert concept %q: %w", name, err)
	}

	return concept, nil
}

func (ks *KnowledgeStore) mergeConceptInTx(ctx context.Context, tx pgx.Tx, existing *ConceptPage, description string, newClaims []Claim, artifactID, sourceType, contractVersion string) (*ConceptPage, error) {
	// Merge claims: append new claims to existing
	var existingClaims []Claim
	if err := json.Unmarshal(existing.Claims, &existingClaims); err != nil {
		existingClaims = nil
	}
	mergedClaims := append(existingClaims, newClaims...)

	// Enforce concept page token cap (design: 4,000 tokens).
	// When over budget, drop oldest claims (FIFO) while preserving citations.
	if ks.MaxTokens > 0 {
		mergedClaims = enforceTokenCap(mergedClaims, existing.Summary, ks.MaxTokens)
	}

	claimsJSON, err := json.Marshal(mergedClaims)
	if err != nil {
		return nil, fmt.Errorf("marshal merged claims: %w", err)
	}

	// Add artifact_id if not already present
	sourceArtifacts := addUnique(existing.SourceArtifactIDs, artifactID)

	// Add source_type if not already present
	sourceTypes := addUnique(existing.SourceTypeDiversity, sourceType)

	// Update summary if new description is more detailed
	summary := existing.Summary
	if len(description) > len(summary) {
		summary = description
	}

	now := time.Now().UTC()

	_, err = tx.Exec(ctx, `
		UPDATE knowledge_concepts SET
			summary = $2, claims = $3,
			source_artifact_ids = $4, source_type_diversity = $5,
			token_count = $6, prompt_contract_version = $7, updated_at = $8
		WHERE id = $1`,
		existing.ID, summary, claimsJSON,
		sourceArtifacts, sourceTypes,
		estimateTokens(summary, claimsJSON), contractVersion, now,
	)
	if err != nil {
		return nil, fmt.Errorf("update concept %q: %w", existing.Title, err)
	}

	existing.Summary = summary
	existing.Claims = claimsJSON
	existing.SourceArtifactIDs = sourceArtifacts
	existing.SourceTypeDiversity = sourceTypes
	existing.PromptContractVersion = contractVersion
	existing.UpdatedAt = now

	return existing, nil
}

// UpsertEntity finds an entity by normalized name + type. If it exists, appends
// the new mention and updates source types. If not, creates a new entity profile.
func (ks *KnowledgeStore) UpsertEntity(ctx context.Context, tx pgx.Tx, name, entityType, entityContext, artifactID, artifactTitle, sourceType, contractVersion string) (*EntityProfile, error) {
	normalized := normalizeName(name)

	var existing EntityProfile
	err := tx.QueryRow(ctx, `
		SELECT id, name, name_normalized, entity_type, summary, mentions,
		       source_types, related_concept_ids, interaction_count,
		       people_id, prompt_contract_version, created_at, updated_at
		FROM knowledge_entities WHERE name_normalized = $1 AND entity_type = $2
		FOR UPDATE`, normalized, entityType,
	).Scan(
		&existing.ID, &existing.Name, &existing.NameNormalized, &existing.EntityType,
		&existing.Summary, &existing.Mentions, &existing.SourceTypes,
		&existing.RelatedConceptIDs, &existing.InteractionCount,
		&existing.PeopleID, &existing.PromptContractVersion,
		&existing.CreatedAt, &existing.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return ks.createEntityInTx(ctx, tx, name, entityType, entityContext, artifactID, artifactTitle, sourceType, contractVersion)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup entity %q: %w", name, err)
	}

	return ks.mergeEntityInTx(ctx, tx, &existing, entityContext, artifactID, artifactTitle, sourceType, contractVersion)
}

func (ks *KnowledgeStore) createEntityInTx(ctx context.Context, tx pgx.Tx, name, entityType, entityContext, artifactID, artifactTitle, sourceType, contractVersion string) (*EntityProfile, error) {
	id := ulid.Make().String()
	now := time.Now().UTC()

	mention := Mention{
		ArtifactID:    artifactID,
		ArtifactTitle: artifactTitle,
		SourceType:    sourceType,
		Context:       entityContext,
		MentionedAt:   now.Format(time.RFC3339),
	}
	mentionsJSON, err := json.Marshal([]Mention{mention})
	if err != nil {
		return nil, fmt.Errorf("marshal mentions: %w", err)
	}

	entity := &EntityProfile{
		ID:                    id,
		Name:                  name,
		NameNormalized:        normalizeName(name),
		EntityType:            entityType,
		Summary:               "",
		Mentions:              mentionsJSON,
		SourceTypes:           []string{sourceType},
		RelatedConceptIDs:     []string{},
		InteractionCount:      1,
		PromptContractVersion: contractVersion,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO knowledge_entities
			(id, name, name_normalized, entity_type, summary, mentions,
			 source_types, related_concept_ids, interaction_count,
			 people_id, prompt_contract_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		entity.ID, entity.Name, entity.NameNormalized, entity.EntityType,
		entity.Summary, entity.Mentions,
		entity.SourceTypes, entity.RelatedConceptIDs,
		entity.InteractionCount, entity.PeopleID,
		entity.PromptContractVersion, entity.CreatedAt, entity.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert entity %q: %w", name, err)
	}

	return entity, nil
}

func (ks *KnowledgeStore) mergeEntityInTx(ctx context.Context, tx pgx.Tx, existing *EntityProfile, entityContext, artifactID, artifactTitle, sourceType, contractVersion string) (*EntityProfile, error) {
	// Append new mention
	var existingMentions []Mention
	if err := json.Unmarshal(existing.Mentions, &existingMentions); err != nil {
		existingMentions = nil
	}

	newMention := Mention{
		ArtifactID:    artifactID,
		ArtifactTitle: artifactTitle,
		SourceType:    sourceType,
		Context:       entityContext,
		MentionedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	mergedMentions := append(existingMentions, newMention)
	mentionsJSON, err := json.Marshal(mergedMentions)
	if err != nil {
		return nil, fmt.Errorf("marshal merged mentions: %w", err)
	}

	sourceTypes := addUnique(existing.SourceTypes, sourceType)
	now := time.Now().UTC()

	_, err = tx.Exec(ctx, `
		UPDATE knowledge_entities SET
			mentions = $2, source_types = $3,
			interaction_count = $4, prompt_contract_version = $5, updated_at = $6
		WHERE id = $1`,
		existing.ID, mentionsJSON, sourceTypes,
		existing.InteractionCount+1, contractVersion, now,
	)
	if err != nil {
		return nil, fmt.Errorf("update entity %q: %w", existing.Name, err)
	}

	existing.Mentions = mentionsJSON
	existing.SourceTypes = sourceTypes
	existing.InteractionCount++
	existing.PromptContractVersion = contractVersion
	existing.UpdatedAt = now

	return existing, nil
}

// UpdateArtifactSynthesisStatus sets the synthesis_status and related columns on an artifact.
func (ks *KnowledgeStore) UpdateArtifactSynthesisStatus(ctx context.Context, artifactID, status, synthError string) error {
	_, err := ks.pool.Exec(ctx, `
		UPDATE artifacts SET
			synthesis_status = $2,
			synthesis_at = NOW(),
			synthesis_error = $3,
			synthesis_retry_count = CASE WHEN $2 = 'failed' THEN synthesis_retry_count + 1 ELSE synthesis_retry_count END
		WHERE id = $1`,
		artifactID, status, synthError,
	)
	if err != nil {
		return fmt.Errorf("update artifact synthesis status: %w", err)
	}
	return nil
}

// UpdateArtifactSynthesisStatusInTx sets the synthesis_status within an existing transaction.
// This ensures the status update is atomic with knowledge layer writes (C-025-C001).
func (ks *KnowledgeStore) UpdateArtifactSynthesisStatusInTx(ctx context.Context, tx pgx.Tx, artifactID, status, synthError string) error {
	_, err := tx.Exec(ctx, `
		UPDATE artifacts SET
			synthesis_status = $2,
			synthesis_at = NOW(),
			synthesis_error = $3,
			synthesis_retry_count = CASE WHEN $2 = 'failed' THEN synthesis_retry_count + 1 ELSE synthesis_retry_count END
		WHERE id = $1`,
		artifactID, status, synthError,
	)
	if err != nil {
		return fmt.Errorf("update artifact synthesis status in tx: %w", err)
	}
	return nil
}

// ErrArtifactNotFound is returned when an artifact ID does not exist in the database.
var ErrArtifactNotFound = fmt.Errorf("artifact not found")

// GetArtifactForSynthesis loads the fields needed to build a SynthesisExtractRequest.
// Returns ErrArtifactNotFound when the artifact ID does not exist (C-025-C003).
func (ks *KnowledgeStore) GetArtifactForSynthesis(ctx context.Context, artifactID string) (*ArtifactSynthesisData, error) {
	var a ArtifactSynthesisData
	err := ks.pool.QueryRow(ctx, `
		SELECT id, COALESCE(artifact_type, ''), COALESCE(title, ''), COALESCE(summary, ''),
		       COALESCE(content_raw, ''), COALESCE(source_id, ''),
		       COALESCE(key_ideas, '[]'::jsonb), COALESCE(entities, '{}'::jsonb),
		       COALESCE(topics, '[]'::jsonb), COALESCE(synthesis_retry_count, 0)
		FROM artifacts WHERE id = $1`, artifactID,
	).Scan(&a.ID, &a.ArtifactType, &a.Title, &a.Summary,
		&a.ContentRaw, &a.SourceID,
		&a.KeyIdeasJSON, &a.EntitiesJSON, &a.TopicsJSON, &a.RetryCount)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrArtifactNotFound
		}
		return nil, fmt.Errorf("get artifact for synthesis: %w", err)
	}
	return &a, nil
}

// CreateEdgeInTx creates an edge in the edges table within a transaction.
func (ks *KnowledgeStore) CreateEdgeInTx(ctx context.Context, tx pgx.Tx, srcType, srcID, dstType, dstID, edgeType string, weight float32, metadata map[string]interface{}) error {
	id := ulid.Make().String()

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal edge metadata: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT DO NOTHING`,
		id, srcType, srcID, dstType, dstID, edgeType, weight, metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("create edge %s: %w", edgeType, err)
	}
	return nil
}

// BeginTx starts a new database transaction.
func (ks *KnowledgeStore) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return ks.pool.Begin(ctx)
}

// ArtifactSynthesisData holds the artifact fields needed for a synthesis request.
type ArtifactSynthesisData struct {
	ID           string          `json:"id"`
	ArtifactType string          `json:"artifact_type"`
	Title        string          `json:"title"`
	Summary      string          `json:"summary"`
	ContentRaw   string          `json:"content_raw"`
	SourceID     string          `json:"source_id"`
	KeyIdeasJSON json.RawMessage `json:"key_ideas"`
	EntitiesJSON json.RawMessage `json:"entities"`
	TopicsJSON   json.RawMessage `json:"topics"`
	RetryCount   int             `json:"retry_count"`
}

// addUnique appends a value to a slice if not already present.
func addUnique(slice []string, val string) []string {
	for _, s := range slice {
		if strings.EqualFold(s, val) {
			return slice
		}
	}
	return append(slice, val)
}

// estimateTokens provides a rough token count for content (1 token ≈ 4 chars).
func estimateTokens(summary string, claimsJSON json.RawMessage) int {
	totalChars := len(summary) + len(claimsJSON)
	return totalChars / 4
}

// enforceTokenCap trims the oldest claims (FIFO) until the estimated token count
// for summary + claims is within maxTokens. Citations (source_artifact_ids) are
// preserved independently of claims, so no provenance is lost.
func enforceTokenCap(claims []Claim, summary string, maxTokens int) []Claim {
	for len(claims) > 0 {
		claimsJSON, err := json.Marshal(claims)
		if err != nil {
			break
		}
		if estimateTokens(summary, claimsJSON) <= maxTokens {
			return claims
		}
		// Drop the oldest claim (index 0 = earliest appended)
		claims = claims[1:]
	}
	return claims
}

// CreateCrossSourceEdge creates a CROSS_SOURCE_CONNECTION edge linking artifacts
// from different source types that share a genuine connection through a concept.
func (ks *KnowledgeStore) CreateCrossSourceEdge(ctx context.Context, conceptID, insightText string, confidence float64, artifactIDs []string, promptContractVersion string) error {
	if len(artifactIDs) < 2 {
		return fmt.Errorf("cross-source edge requires at least 2 artifacts")
	}

	tx, err := ks.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx for cross-source edge: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Create pairwise edges between all artifacts involved in the cross-source connection.
	// Each edge carries the concept context, insight, and confidence.
	for i := 0; i < len(artifactIDs); i++ {
		for j := i + 1; j < len(artifactIDs); j++ {
			metadata := map[string]interface{}{
				"concept_id":              conceptID,
				"insight_text":            insightText,
				"confidence":              confidence,
				"artifact_ids":            artifactIDs,
				"prompt_contract_version": promptContractVersion,
			}
			if err := ks.CreateEdgeInTx(ctx, tx, "artifact", artifactIDs[i], "artifact", artifactIDs[j], "CROSS_SOURCE_CONNECTION", float32(confidence), metadata); err != nil {
				return fmt.Errorf("create cross-source edge %s→%s: %w", artifactIDs[i], artifactIDs[j], err)
			}
		}
	}

	return tx.Commit(ctx)
}
