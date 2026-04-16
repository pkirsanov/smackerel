package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/smackerel/smackerel/internal/knowledge"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// SynthesisResultSubscriber consumes synthesis.extracted messages and integrates
// extracted knowledge into concept pages and entity profiles.
// It also handles cross-source connection detection by consuming
// synthesis.crosssource.result messages.
type SynthesisResultSubscriber struct {
	DB                               *pgxpool.Pool
	NATS                             *smacknats.Client
	KnowledgeStore                   *knowledge.KnowledgeStore
	CrossSourceConfidenceThreshold   float64
	CrossSourcePromptContractVersion string
	done                             chan struct{}
	wg                               sync.WaitGroup
	mu                               sync.Mutex
	started                          bool
	stopped                          bool
}

// NewSynthesisResultSubscriber creates a subscriber for synthesis.extracted messages.
func NewSynthesisResultSubscriber(db *pgxpool.Pool, nc *smacknats.Client, ks *knowledge.KnowledgeStore) *SynthesisResultSubscriber {
	return &SynthesisResultSubscriber{
		DB:             db,
		NATS:           nc,
		KnowledgeStore: ks,
	}
}

// Start begins consuming synthesis.extracted messages in a background goroutine.
func (s *SynthesisResultSubscriber) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return fmt.Errorf("synthesis subscriber already stopped")
	}
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("synthesis subscriber already started")
	}
	s.done = make(chan struct{})
	s.started = true
	s.mu.Unlock()

	consumer, err := s.NATS.JetStream.CreateOrUpdateConsumer(ctx, "SYNTHESIS", jetstream.ConsumerConfig{
		Durable:       "smackerel-core-synthesized",
		FilterSubject: smacknats.SubjectSynthesisExtracted,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create consumer for synthesis.extracted: %w", err)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			msgs, err := consumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
			if err != nil {
				select {
				case <-s.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				slog.Debug("fetch synthesis.extracted batch", "error", err)
				select {
				case <-s.done:
					return
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
				}
				continue
			}

			for msg := range msgs.Messages() {
				select {
				case <-s.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				s.handleSynthesized(ctx, msg)
			}
		}
	}()

	slog.Info("synthesis result subscriber started", "subject", smacknats.SubjectSynthesisExtracted)

	// Cross-source result consumer
	crossSourceConsumer, err := s.NATS.JetStream.CreateOrUpdateConsumer(ctx, "SYNTHESIS", jetstream.ConsumerConfig{
		Durable:       "smackerel-core-crosssource-result",
		FilterSubject: smacknats.SubjectSynthesisCrossSourceResult,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create consumer for synthesis.crosssource.result: %w", err)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			msgs, err := crossSourceConsumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
			if err != nil {
				select {
				case <-s.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				slog.Debug("fetch synthesis.crosssource.result batch", "error", err)
				select {
				case <-s.done:
					return
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
				}
				continue
			}

			for msg := range msgs.Messages() {
				select {
				case <-s.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				s.handleCrossSourceResult(ctx, msg)
			}
		}
	}()

	slog.Info("cross-source result subscriber started", "subject", smacknats.SubjectSynthesisCrossSourceResult)
	return nil
}

// Stop signals the background goroutine to exit and waits with a bounded timeout.
func (s *SynthesisResultSubscriber) Stop() {
	s.mu.Lock()
	if !s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.done)
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("synthesis result subscriber stopped cleanly")
	case <-time.After(5 * time.Second):
		slog.Warn("synthesis result subscriber stop timed out")
	}
}

// handleSynthesized processes a single synthesis.extracted message.
func (s *SynthesisResultSubscriber) handleSynthesized(ctx context.Context, msg jetstream.Msg) {
	var resp SynthesisExtractResponse
	if err := json.Unmarshal(msg.Data(), &resp); err != nil {
		slog.Error("invalid synthesis.extracted payload", "error", err)
		_ = msg.Ack()
		return
	}

	if err := ValidateSynthesisExtractResponse(&resp); err != nil {
		slog.Error("synthesis.extracted payload validation failed", "error", err)
		_ = msg.Ack()
		return
	}

	if !resp.Success {
		// Synthesis failed — mark artifact and ack
		if err := s.KnowledgeStore.UpdateArtifactSynthesisStatus(ctx, resp.ArtifactID, "failed", resp.Error); err != nil {
			slog.Error("failed to update synthesis status on failure",
				"artifact_id", resp.ArtifactID,
				"error", err,
			)
		}
		_ = msg.Ack()
		slog.Warn("synthesis extraction failed",
			"artifact_id", resp.ArtifactID,
			"error", resp.Error,
		)
		return
	}

	if resp.Result == nil {
		_ = msg.Ack()
		return
	}

	// Load artifact title and source type for citation creation
	artifact, err := s.KnowledgeStore.GetArtifactForSynthesis(ctx, resp.ArtifactID)
	if err != nil {
		slog.Error("failed to load artifact for knowledge update",
			"artifact_id", resp.ArtifactID,
			"error", err,
		)
		_ = msg.Nak()
		return
	}

	// Transactional knowledge update
	conceptIDs, err := s.applyKnowledgeUpdate(ctx, &resp, artifact)
	if err != nil {
		slog.Error("knowledge update failed",
			"artifact_id", resp.ArtifactID,
			"error", err,
		)
		_ = msg.Nak()
		return
	}

	// Mark artifact synthesis as completed
	if err := s.KnowledgeStore.UpdateArtifactSynthesisStatus(ctx, resp.ArtifactID, "completed", ""); err != nil {
		slog.Error("failed to update synthesis status on success",
			"artifact_id", resp.ArtifactID,
			"error", err,
		)
	}

	// Best-effort cross-source connection detection after successful synthesis
	s.checkCrossSourceConnections(ctx, conceptIDs)

	_ = msg.Ack()
	slog.Info("synthesis result processed",
		"artifact_id", resp.ArtifactID,
		"concepts", len(resp.Result.Concepts),
		"entities", len(resp.Result.Entities),
		"relationships", len(resp.Result.Relationships),
		"processing_ms", resp.ProcessingTimeMs,
	)
}

// applyKnowledgeUpdate performs all concept/entity upserts and edge creation
// within a single database transaction. Returns the IDs of concepts that were
// created or updated.
func (s *SynthesisResultSubscriber) applyKnowledgeUpdate(ctx context.Context, resp *SynthesisExtractResponse, artifact *knowledge.ArtifactSynthesisData) ([]string, error) {
	tx, err := s.KnowledgeStore.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	contractVersion := resp.PromptContractVersion
	artifactID := resp.ArtifactID
	sourceType := artifact.ArtifactType

	// Upsert concepts and create CONCEPT_REFERENCES edges
	conceptIDMap := make(map[string]string) // concept name → concept ID
	for _, extracted := range resp.Result.Concepts {
		claims := make([]knowledge.Claim, 0, len(extracted.Claims))
		for _, c := range extracted.Claims {
			claims = append(claims, knowledge.Claim{
				Text:          c.Text,
				ArtifactID:    artifactID,
				ArtifactTitle: artifact.Title,
				SourceType:    sourceType,
				ExtractedAt:   time.Now().UTC().Format(time.RFC3339),
				Confidence:    c.Confidence,
			})
		}

		concept, err := s.KnowledgeStore.UpsertConcept(ctx, tx, extracted.Name, extracted.Description, claims, artifactID, sourceType, contractVersion)
		if err != nil {
			return nil, fmt.Errorf("upsert concept %q: %w", extracted.Name, err)
		}
		conceptIDMap[extracted.Name] = concept.ID

		// Create CONCEPT_REFERENCES edge: artifact → concept
		if err := s.KnowledgeStore.CreateEdgeInTx(ctx, tx, "artifact", artifactID, "concept", concept.ID, "CONCEPT_REFERENCES", 1.0, map[string]interface{}{
			"prompt_contract_version": contractVersion,
		}); err != nil {
			slog.Debug("edge creation failed", "type", "CONCEPT_REFERENCES", "error", err)
		}
	}

	// Upsert entities and create ENTITY_MENTIONED_IN edges
	for _, extracted := range resp.Result.Entities {
		entity, err := s.KnowledgeStore.UpsertEntity(ctx, tx, extracted.Name, extracted.Type, extracted.Context, artifactID, artifact.Title, sourceType, contractVersion)
		if err != nil {
			return nil, fmt.Errorf("upsert entity %q: %w", extracted.Name, err)
		}

		// Create ENTITY_MENTIONED_IN edge: artifact → entity
		if err := s.KnowledgeStore.CreateEdgeInTx(ctx, tx, "artifact", artifactID, "knowledge_entity", entity.ID, "ENTITY_MENTIONED_IN", 1.0, map[string]interface{}{
			"context":                 extracted.Context,
			"prompt_contract_version": contractVersion,
		}); err != nil {
			slog.Debug("edge creation failed", "type", "ENTITY_MENTIONED_IN", "error", err)
		}
	}

	// Create relationship edges
	for _, rel := range resp.Result.Relationships {
		srcID := conceptIDMap[rel.Source]
		dstID := conceptIDMap[rel.Target]
		if srcID == "" || dstID == "" {
			continue // relationship between unknown concepts, skip
		}

		srcType := "concept"
		dstType := "concept"
		if rel.Type == "ENTITY_RELATES_TO_CONCEPT" {
			srcType = "knowledge_entity"
		}

		if err := s.KnowledgeStore.CreateEdgeInTx(ctx, tx, srcType, srcID, dstType, dstID, rel.Type, 1.0, map[string]interface{}{
			"description":             rel.Description,
			"artifact_ids":            []string{artifactID},
			"prompt_contract_version": contractVersion,
		}); err != nil {
			slog.Debug("relationship edge creation failed", "type", rel.Type, "error", err)
		}
	}

	// Create contradiction edges
	for _, contra := range resp.Result.Contradictions {
		conceptID := conceptIDMap[contra.Concept]
		if conceptID == "" || contra.ExistingArtifactID == "" {
			continue
		}

		if err := s.KnowledgeStore.CreateEdgeInTx(ctx, tx, "artifact", artifactID, "artifact", contra.ExistingArtifactID, "CONTRADICTS", 1.0, map[string]interface{}{
			"concept_id":              conceptID,
			"claim_a":                 contra.ExistingClaim,
			"claim_b":                 contra.NewClaim,
			"prompt_contract_version": contractVersion,
		}); err != nil {
			slog.Debug("contradiction edge creation failed", "error", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit knowledge update: %w", err)
	}

	// Collect concept IDs for cross-source checking
	var conceptIDs []string
	for _, id := range conceptIDMap {
		conceptIDs = append(conceptIDs, id)
	}

	return conceptIDs, nil
}

// checkCrossSourceConnections examines each concept and publishes a cross-source
// assessment request if the concept has artifacts from 2+ different source types.
func (s *SynthesisResultSubscriber) checkCrossSourceConnections(ctx context.Context, conceptIDs []string) {
	if s.CrossSourcePromptContractVersion == "" {
		return
	}

	for _, conceptID := range conceptIDs {
		concept, err := s.KnowledgeStore.GetConceptByID(ctx, conceptID)
		if err != nil || concept == nil {
			slog.Debug("cross-source: failed to load concept", "concept_id", conceptID, "error", err)
			continue
		}

		// Only trigger cross-source check if concept has 2+ distinct source types
		if len(concept.SourceTypeDiversity) < 2 {
			continue
		}

		// Get representative artifacts from different source types (max 10)
		artifacts, err := s.KnowledgeStore.GetCrossSourceArtifacts(ctx, conceptID, 10)
		if err != nil {
			slog.Warn("cross-source: failed to get artifacts", "concept_id", conceptID, "error", err)
			continue
		}
		if len(artifacts) < 2 {
			continue
		}

		// Build cross-source request
		artSummaries := make([]CrossSourceArtifactSummary, 0, len(artifacts))
		for _, a := range artifacts {
			artSummaries = append(artSummaries, CrossSourceArtifactSummary{
				ID:         a.ID,
				Title:      a.Title,
				SourceType: a.SourceType,
				Summary:    a.Summary,
			})
		}

		req := CrossSourceRequest{
			ConceptID:             concept.ID,
			ConceptTitle:          concept.Title,
			Artifacts:             artSummaries,
			PromptContractVersion: s.CrossSourcePromptContractVersion,
		}

		data, err := json.Marshal(req)
		if err != nil {
			slog.Warn("cross-source: failed to marshal request", "concept_id", conceptID, "error", err)
			continue
		}

		if _, err := s.NATS.JetStream.Publish(ctx, smacknats.SubjectSynthesisCrossSource, data); err != nil {
			slog.Warn("cross-source: publish failed (fail-open)", "concept_id", conceptID, "error", err)
		} else {
			slog.Info("cross-source request published", "concept_id", conceptID, "concept_title", concept.Title, "artifact_count", len(artifacts))
		}
	}
}

// handleCrossSourceResult processes a single synthesis.crosssource.result message.
// If the ML sidecar found a genuine connection with confidence above threshold,
// creates a CROSS_SOURCE_CONNECTION edge. Otherwise discards silently.
func (s *SynthesisResultSubscriber) handleCrossSourceResult(ctx context.Context, msg jetstream.Msg) {
	var resp CrossSourceResponse
	if err := json.Unmarshal(msg.Data(), &resp); err != nil {
		slog.Error("invalid synthesis.crosssource.result payload", "error", err)
		_ = msg.Ack()
		return
	}

	if resp.ConceptID == "" {
		slog.Error("synthesis.crosssource.result: missing concept_id")
		_ = msg.Ack()
		return
	}

	// Discard if not a genuine connection or below confidence threshold
	if !resp.HasGenuineConnection || resp.Confidence <= s.CrossSourceConfidenceThreshold {
		_ = msg.Ack()
		slog.Debug("cross-source result below threshold, discarded",
			"concept_id", resp.ConceptID,
			"has_genuine_connection", resp.HasGenuineConnection,
			"confidence", resp.Confidence,
			"threshold", s.CrossSourceConfidenceThreshold,
		)
		return
	}

	// Create CROSS_SOURCE_CONNECTION edge
	if err := s.KnowledgeStore.CreateCrossSourceEdge(
		ctx,
		resp.ConceptID,
		resp.InsightText,
		resp.Confidence,
		resp.ArtifactIDs,
		resp.PromptContractVersion,
	); err != nil {
		slog.Error("failed to create cross-source edge",
			"concept_id", resp.ConceptID,
			"error", err,
		)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
	slog.Info("cross-source connection created",
		"concept_id", resp.ConceptID,
		"confidence", resp.Confidence,
		"artifact_count", len(resp.ArtifactIDs),
		"model_used", resp.ModelUsed,
	)
}
