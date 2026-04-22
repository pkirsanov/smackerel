package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/digest"
	"github.com/smackerel/smackerel/internal/domain"
	"github.com/smackerel/smackerel/internal/knowledge"
	"github.com/smackerel/smackerel/internal/metrics"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/stringutil"
)

// DefaultMaxDeliver is the maximum delivery attempts before dead-letter routing.
// Must match the MaxDeliver value in consumer configs created by Start().
const DefaultMaxDeliver = 5

type ResultSubscriber struct {
	DB                    *pgxpool.Pool
	NATS                  *smacknats.Client
	Processor             *Processor
	DigestGen             *digest.Generator
	KnowledgeEnabled      bool
	KnowledgeStore        *knowledge.KnowledgeStore
	PromptContractVersion string
	DomainRegistry        *domain.Registry
	done                  chan struct{}
	wg                    sync.WaitGroup
	mu                    sync.Mutex
	started               bool
	stopped               bool
}

// NewResultSubscriber creates a subscriber for artifacts.processed messages.
func NewResultSubscriber(db *pgxpool.Pool, nc *smacknats.Client, registry *connector.Registry) *ResultSubscriber {
	return &ResultSubscriber{
		DB:        db,
		NATS:      nc,
		Processor: NewProcessor(db, nc),
		DigestGen: digest.NewGenerator(db, nc, registry),
	}
}

// Start begins consuming artifacts.processed and digest.generated messages in background goroutines.
func (rs *ResultSubscriber) Start(ctx context.Context) error {
	rs.mu.Lock()
	if rs.stopped {
		rs.mu.Unlock()
		return fmt.Errorf("subscriber already stopped")
	}
	if rs.started {
		rs.mu.Unlock()
		return fmt.Errorf("subscriber already started")
	}
	rs.done = make(chan struct{})
	rs.started = true
	rs.mu.Unlock()

	// artifacts.processed consumer
	processedConsumer, err := rs.NATS.JetStream.CreateOrUpdateConsumer(ctx, "ARTIFACTS", jetstream.ConsumerConfig{
		Durable:       "smackerel-core-processed",
		FilterSubject: smacknats.SubjectArtifactsProcessed,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    DefaultMaxDeliver,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create consumer for artifacts.processed: %w", err)
	}

	rs.wg.Add(1)
	go func() {
		defer rs.wg.Done()
		for {
			msgs, err := processedConsumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
			if err != nil {
				// Check for shutdown between fetch attempts
				select {
				case <-rs.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				slog.Debug("fetch artifacts.processed batch", "error", err)
				// Backoff before retry to prevent tight spin on transient NATS errors
				select {
				case <-rs.done:
					return
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
				}
				continue
			}

			for msg := range msgs.Messages() {
				select {
				case <-rs.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				rs.handleMessage(ctx, msg)
			}
		}
	}()

	slog.Info("result subscriber started", "subject", smacknats.SubjectArtifactsProcessed)

	// digest.generated consumer
	digestConsumer, err := rs.NATS.JetStream.CreateOrUpdateConsumer(ctx, "DIGEST", jetstream.ConsumerConfig{
		Durable:       "smackerel-core-digest",
		FilterSubject: smacknats.SubjectDigestGenerated,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    DefaultMaxDeliver,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create consumer for digest.generated: %w", err)
	}

	rs.wg.Add(1)
	go func() {
		defer rs.wg.Done()
		for {
			msgs, err := digestConsumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
			if err != nil {
				// Check for shutdown between fetch attempts
				select {
				case <-rs.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				slog.Debug("fetch digest.generated batch", "error", err)
				// Backoff before retry to prevent tight spin on transient NATS errors
				select {
				case <-rs.done:
					return
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
				}
				continue
			}

			for msg := range msgs.Messages() {
				select {
				case <-rs.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				rs.handleDigestMessage(ctx, msg)
			}
		}
	}()

	slog.Info("result subscriber started", "subject", smacknats.SubjectDigestGenerated)
	return nil
}

// Stop signals the background goroutines to exit and waits for them to finish.
// Uses a bounded timeout to prevent blocking shutdown indefinitely if a consumer
// goroutine hangs despite the done channel being closed.
// Timeout is 5s to fit within the 6s shutdown step budget in shutdownAll (IMP-022-002).
func (rs *ResultSubscriber) Stop() {
	rs.mu.Lock()
	if !rs.started || rs.stopped {
		rs.mu.Unlock()
		return
	}
	rs.stopped = true
	close(rs.done)
	rs.mu.Unlock()

	done := make(chan struct{})
	go func() {
		rs.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("result subscriber stopped cleanly")
	case <-time.After(5 * time.Second):
		slog.Warn("result subscriber stop timed out waiting for consumer goroutines")
	}
}

// handleMessage processes a single artifacts.processed message.
func (rs *ResultSubscriber) handleMessage(ctx context.Context, msg jetstream.Msg) {
	var payload NATSProcessedPayload
	if err := json.Unmarshal(msg.Data(), &payload); err != nil {
		slog.Error("invalid artifacts.processed payload",
			"error", err,
			"raw_payload_truncated", truncateBytes(msg.Data(), 200),
		)
		// Ack to prevent infinite redelivery of malformed messages
		_ = msg.Ack()
		return
	}

	if err := ValidateProcessedPayload(&payload); err != nil {
		slog.Error("artifacts.processed payload validation failed",
			"error", err,
			"artifact_id", payload.ArtifactID,
		)
		_ = msg.Ack()
		return
	}

	if err := rs.Processor.HandleProcessedResult(ctx, &payload); err != nil {
		slog.Error("failed to handle processed result",
			"artifact_id", payload.ArtifactID,
			"error", err,
		)
		rs.handleDeliveryFailure(ctx, msg, "artifacts.processed", "ARTIFACTS", err)
		return
	}

	// Record ingestion metric
	metrics.ArtifactsIngested.WithLabelValues("pipeline", payload.Result.ArtifactType).Inc()

	// Best-effort knowledge synthesis — fail-open, never blocks ingestion (SCN-025-06)
	if rs.KnowledgeEnabled && payload.Success {
		if err := rs.publishSynthesisRequest(ctx, &payload); err != nil {
			slog.Warn("synthesis publish failed (fail-open)",
				"artifact_id", payload.ArtifactID,
				"error", err,
			)
		}
	}

	// Best-effort domain extraction — fail-open, never blocks ingestion (SCN-026-01)
	if rs.DomainRegistry != nil && payload.Success {
		if err := rs.publishDomainExtractionRequest(ctx, &payload); err != nil {
			slog.Warn("domain extraction publish failed (fail-open)",
				"artifact_id", payload.ArtifactID,
				"error", err,
			)
		}
	}

	_ = msg.Ack()
	slog.Debug("processed result stored", "artifact_id", payload.ArtifactID)
}

// handleDigestMessage processes a single digest.generated message.
func (rs *ResultSubscriber) handleDigestMessage(ctx context.Context, msg jetstream.Msg) {
	var payload NATSDigestGeneratedPayload
	if err := json.Unmarshal(msg.Data(), &payload); err != nil {
		slog.Error("invalid digest.generated payload",
			"error", err,
			"raw_payload_truncated", truncateBytes(msg.Data(), 200),
		)
		_ = msg.Ack()
		return
	}

	if err := ValidateDigestGeneratedPayload(&payload); err != nil {
		slog.Error("digest.generated payload validation failed", "error", err)
		_ = msg.Ack()
		return
	}

	if err := rs.DigestGen.HandleDigestResult(ctx, payload.DigestDate, payload.Text, payload.WordCount, payload.ModelUsed); err != nil {
		slog.Error("failed to handle digest result", "error", err)
		rs.handleDeliveryFailure(ctx, msg, "digest.generated", "DIGEST", err)
		return
	}

	_ = msg.Ack()
	slog.Debug("digest result stored")
}

// handleDeliveryFailure routes a failed message to dead-letter if delivery is exhausted,
// otherwise Naks for retry. On dead-letter publish failure, Naks to preserve the message.
func (rs *ResultSubscriber) handleDeliveryFailure(ctx context.Context, msg jetstream.Msg, subject, stream string, lastErr error) {
	if rs.isDeliveryExhausted(msg, DefaultMaxDeliver) {
		if dlErr := rs.publishToDeadLetter(ctx, msg, subject, stream, lastErr.Error()); dlErr != nil {
			slog.Error("dead-letter publish failed, Nak to preserve message", "error", dlErr)
			if nakErr := msg.Nak(); nakErr != nil {
				slog.Error("Nak also failed after dead-letter failure — message may be lost",
					"nak_error", nakErr,
					"dead_letter_error", dlErr,
					"subject", subject,
				)
			}
			return
		}
		_ = msg.Ack()
		return
	}
	_ = msg.Nak()
}

// isDeliveryExhausted checks if a message's delivery count has reached maxDeliver.
func (rs *ResultSubscriber) isDeliveryExhausted(msg jetstream.Msg, maxDeliver int) bool {
	md, err := msg.Metadata()
	if err != nil {
		return false
	}
	return int(md.NumDelivered) >= maxDeliver
}

// publishToDeadLetter routes an exhausted message to the DEADLETTER stream.
// Returns an error if the publish fails so callers can Nak instead of Ack.
func (rs *ResultSubscriber) publishToDeadLetter(ctx context.Context, msg jetstream.Msg, originalSubject, originalStream, lastError string) error {
	headers := nats.Header{}
	headers.Set("Smackerel-Original-Subject", originalSubject)
	headers.Set("Smackerel-Original-Stream", originalStream)
	headers.Set("Smackerel-Failed-At", time.Now().UTC().Format(time.RFC3339))
	if lastError != "" {
		// Truncate to 256 bytes per design contract to prevent oversized headers.
		// Use UTF-8-safe truncation to avoid splitting multi-byte characters
		// which would produce invalid UTF-8 in NATS headers (IMP-022-R29-003).
		if len(lastError) > 256 {
			lastError = stringutil.TruncateUTF8(lastError, 256)
		}
		headers.Set("Smackerel-Last-Error", lastError)
	}

	md, err := msg.Metadata()
	if err == nil {
		headers.Set("Smackerel-Delivery-Count", strconv.FormatUint(md.NumDelivered, 10))
		if md.Consumer != "" {
			headers.Set("Smackerel-Original-Consumer", md.Consumer)
		}
	}

	dlSubject := "deadletter." + originalSubject
	dlMsg := &nats.Msg{
		Subject: dlSubject,
		Data:    msg.Data(),
		Header:  headers,
	}

	if _, err := rs.NATS.JetStream.PublishMsg(ctx, dlMsg); err != nil {
		slog.Error("failed to publish to dead-letter",
			"subject", dlSubject,
			"error", err,
		)
		return fmt.Errorf("publish to dead-letter %s: %w", dlSubject, err)
	}

	slog.Warn("message routed to dead-letter",
		"subject", dlSubject,
		"original_subject", originalSubject,
	)
	metrics.NATSDeadLetter.WithLabelValues(originalStream).Inc()
	return nil
}

// truncateBytes returns a string representation of data, truncated to maxLen bytes.
// Delegates to stringutil.TruncateUTF8 for rune-safe truncation (IMP-022-R29-003).
func truncateBytes(data []byte, maxLen int) string {
	if len(data) <= maxLen {
		return string(data)
	}
	return stringutil.TruncateUTF8(string(data), maxLen) + "...(truncated)"
}

// maxSynthesisContentChars is the maximum character count for content_raw sent to the LLM.
const maxSynthesisContentChars = 8000

// maxSynthesisContextItems caps existing concepts/entities included in extraction requests.
const maxSynthesisContextItems = 50

// publishSynthesisRequest builds and publishes a SynthesisExtractRequest for an artifact.
// Fail-open: errors are returned for logging but must never block ingestion.
func (rs *ResultSubscriber) publishSynthesisRequest(ctx context.Context, payload *NATSProcessedPayload) error {
	if rs.KnowledgeStore == nil || rs.PromptContractVersion == "" {
		return nil
	}

	artifact, err := rs.KnowledgeStore.GetArtifactForSynthesis(ctx, payload.ArtifactID)
	if err != nil {
		return fmt.Errorf("load artifact: %w", err)
	}

	// Truncate content to 8000 chars for LLM context window budget
	contentRaw := artifact.ContentRaw
	if len(contentRaw) > maxSynthesisContentChars {
		contentRaw = stringutil.TruncateUTF8(contentRaw, maxSynthesisContentChars)
	}

	// Parse ML-extracted fields
	var keyIdeas []string
	_ = json.Unmarshal(artifact.KeyIdeasJSON, &keyIdeas)

	var entities map[string][]string
	_ = json.Unmarshal(artifact.EntitiesJSON, &entities)

	var topics []string
	_ = json.Unmarshal(artifact.TopicsJSON, &topics)

	// Load existing concepts for context (up to 50)
	existingConcepts, _, err := rs.KnowledgeStore.ListConcepts(ctx, maxSynthesisContextItems, 0)
	if err != nil {
		slog.Debug("failed to load existing concepts for synthesis context", "error", err)
	}
	var conceptSummaries []SynthesisConceptSummary
	for _, c := range existingConcepts {
		conceptSummaries = append(conceptSummaries, SynthesisConceptSummary{
			ID:      c.ID,
			Title:   c.Title,
			Summary: c.Summary,
		})
	}

	// Load existing entities for context (up to 50)
	existingEntities, _, err := rs.KnowledgeStore.ListEntities(ctx, maxSynthesisContextItems, 0)
	if err != nil {
		slog.Debug("failed to load existing entities for synthesis context", "error", err)
	}
	var entitySummaries []SynthesisEntitySummary
	for _, e := range existingEntities {
		entitySummaries = append(entitySummaries, SynthesisEntitySummary{
			ID:   e.ID,
			Name: e.Name,
			Type: e.EntityType,
		})
	}

	req := &SynthesisExtractRequest{
		ArtifactID:            payload.ArtifactID,
		ContentType:           artifact.ArtifactType,
		Title:                 artifact.Title,
		Summary:               artifact.Summary,
		ContentRaw:            contentRaw,
		KeyIdeas:              keyIdeas,
		Entities:              entities,
		Topics:                topics,
		SourceID:              artifact.SourceID,
		SourceType:            artifact.ArtifactType,
		ExistingConcepts:      conceptSummaries,
		ExistingEntities:      entitySummaries,
		PromptContractVersion: rs.PromptContractVersion,
		RetryCount:            artifact.RetryCount,
	}

	if err := ValidateSynthesisExtractRequest(req); err != nil {
		return fmt.Errorf("validate synthesis request: %w", err)
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal synthesis request: %w", err)
	}

	if len(data) > MaxNATSMessageSize {
		return fmt.Errorf("synthesis request too large: %d bytes", len(data))
	}

	if err := rs.NATS.Publish(ctx, smacknats.SubjectSynthesisExtract, data); err != nil {
		return fmt.Errorf("publish to synthesis.extract: %w", err)
	}

	slog.Info("synthesis request published",
		"artifact_id", payload.ArtifactID,
		"contract_version", rs.PromptContractVersion,
	)
	return nil
}

// maxDomainContentChars is the maximum character count for content_raw sent to domain extraction.
const maxDomainContentChars = 15000

// publishDomainExtractionRequest matches the artifact against the domain registry
// and publishes a domain extraction request if a contract is found.
// Fail-open: errors are returned for logging but must never block ingestion.
func (rs *ResultSubscriber) publishDomainExtractionRequest(ctx context.Context, payload *NATSProcessedPayload) error {
	if rs.DomainRegistry == nil {
		return nil
	}

	// Load artifact fields needed for domain registry matching and extraction request
	var sourceURL, contentRaw, title, summary string
	err := rs.DB.QueryRow(ctx,
		`SELECT COALESCE(source_url, ''), COALESCE(content_raw, ''), COALESCE(title, ''), COALESCE(summary, '')
		 FROM artifacts WHERE id = $1`,
		payload.ArtifactID,
	).Scan(&sourceURL, &contentRaw, &title, &summary)
	if err != nil {
		return fmt.Errorf("load artifact for domain extraction: %w", err)
	}

	contract := rs.DomainRegistry.Match(payload.Result.ArtifactType, sourceURL)
	if contract == nil {
		return nil // no matching domain schema — skip silently
	}

	// Content length gating
	if contract.MinContentLen > 0 && len(contentRaw) < contract.MinContentLen && len(summary) < contract.MinContentLen {
		// Mark as skipped
		_, _ = rs.DB.Exec(ctx,
			`UPDATE artifacts SET domain_extraction_status = 'skipped' WHERE id = $1`,
			payload.ArtifactID,
		)
		slog.Debug("domain extraction skipped — content too short",
			"artifact_id", payload.ArtifactID,
			"content_len", len(contentRaw),
			"min_required", contract.MinContentLen,
		)
		return nil
	}

	// Truncate content
	if len(contentRaw) > maxDomainContentChars {
		contentRaw = stringutil.TruncateUTF8(contentRaw, maxDomainContentChars)
	}

	req := &DomainExtractRequest{
		ArtifactID:      payload.ArtifactID,
		ContentType:     payload.Result.ArtifactType,
		Title:           title,
		Summary:         summary,
		ContentRaw:      contentRaw,
		SourceURL:       sourceURL,
		ContractVersion: contract.Version,
	}

	if err := ValidateDomainExtractRequest(req); err != nil {
		return fmt.Errorf("validate domain request: %w", err)
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal domain request: %w", err)
	}

	if len(data) > MaxNATSMessageSize {
		return fmt.Errorf("domain request too large: %d bytes", len(data))
	}

	// Mark as pending before publish
	_, _ = rs.DB.Exec(ctx,
		`UPDATE artifacts SET domain_extraction_status = 'pending', domain_schema_version = $2 WHERE id = $1`,
		payload.ArtifactID, contract.Version,
	)

	if err := rs.NATS.Publish(ctx, smacknats.SubjectDomainExtract, data); err != nil {
		// S-002: Revert pending status so the artifact isn't stuck in 'pending' forever.
		_, _ = rs.DB.Exec(ctx,
			`UPDATE artifacts SET domain_extraction_status = NULL WHERE id = $1 AND domain_extraction_status = 'pending'`,
			payload.ArtifactID,
		)
		metrics.DomainExtraction.WithLabelValues(contract.Version, "error").Inc()
		return fmt.Errorf("publish to domain.extract: %w", err)
	}

	metrics.DomainExtraction.WithLabelValues(contract.Version, "published").Inc()
	slog.Info("domain extraction request published",
		"artifact_id", payload.ArtifactID,
		"contract_version", contract.Version,
	)
	return nil
}
