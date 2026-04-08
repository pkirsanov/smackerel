package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/extract"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// ProcessRequest is the input for the processing pipeline.
type ProcessRequest struct {
	URL      string `json:"url,omitempty"`
	Text     string `json:"text,omitempty"`
	VoiceURL string `json:"voice_url,omitempty"`
	Context  string `json:"context,omitempty"`
	SourceID string `json:"source_id"`
	Starred  bool   `json:"starred,omitempty"`
}

// ProcessResult is the output after full pipeline processing.
type ProcessResult struct {
	ArtifactID   string   `json:"artifact_id"`
	Title        string   `json:"title"`
	ArtifactType string   `json:"artifact_type"`
	Summary      string   `json:"summary"`
	Connections  int      `json:"connections"`
	Topics       []string `json:"topics"`
	ProcessingMs int64    `json:"processing_time_ms"`
}

// NATSProcessPayload is what core publishes to artifacts.process.
type NATSProcessPayload struct {
	ArtifactID     string `json:"artifact_id"`
	ContentType    string `json:"content_type"`
	URL            string `json:"url,omitempty"`
	RawText        string `json:"raw_text"`
	Transcript     string `json:"transcript,omitempty"`
	ProcessingTier string `json:"processing_tier"`
	UserContext    string `json:"user_context,omitempty"`
	SourceID       string `json:"source_id"`
	RetryCount     int    `json:"retry_count"`
}

// NATSProcessedPayload is what ML sidecar publishes to artifacts.processed.
type NATSProcessedPayload struct {
	ArtifactID string `json:"artifact_id"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	Result     struct {
		ArtifactType      string              `json:"artifact_type"`
		Title             string              `json:"title"`
		Summary           string              `json:"summary"`
		KeyIdeas          []string            `json:"key_ideas"`
		Entities          map[string][]string `json:"entities"`
		ActionItems       []string            `json:"action_items"`
		Topics            []string            `json:"topics"`
		Sentiment         string              `json:"sentiment"`
		TemporalRelevance map[string]string   `json:"temporal_relevance"`
		SourceQuality     string              `json:"source_quality"`
	} `json:"result"`
	Embedding    []float32 `json:"embedding"`
	ProcessingMs int64     `json:"processing_time_ms"`
	ModelUsed    string    `json:"model_used"`
	TokensUsed   int       `json:"tokens_used"`
}

// Processor orchestrates the content processing pipeline.
type Processor struct {
	DB   *pgxpool.Pool
	NATS *smacknats.Client
}

// NewProcessor creates a new pipeline processor.
func NewProcessor(db *pgxpool.Pool, nats *smacknats.Client) *Processor {
	return &Processor{DB: db, NATS: nats}
}

// Process runs the full pipeline: extract -> dedup -> publish to NATS for ML processing.
// The ML sidecar will asynchronously process and publish results back.
func (p *Processor) Process(ctx context.Context, req *ProcessRequest) (*ProcessResult, error) {
	start := time.Now()

	// Step 1: Extract content
	var extracted *extract.Result
	var err error

	switch {
	case req.URL != "":
		contentType := extract.DetectContentType(req.URL)
		if contentType == extract.ContentTypeYouTube {
			// YouTube needs ML sidecar for transcript — create stub and send to ML
			extracted = &extract.Result{
				ContentType: extract.ContentTypeYouTube,
				Title:       "YouTube Video",
				Text:        req.URL,
				ContentHash: extract.HashContent(req.URL),
				SourceURL:   req.URL,
				VideoID:     extract.ExtractYouTubeID(req.URL),
			}
		} else {
			extracted, err = extract.ExtractArticle(ctx, req.URL)
			if err != nil {
				return nil, fmt.Errorf("content extraction failed: %w", err)
			}
		}
	case req.Text != "":
		extracted = extract.ExtractText(req.Text)
	case req.VoiceURL != "":
		// Voice notes need ML sidecar for Whisper transcription
		extracted = &extract.Result{
			ContentType: extract.ContentTypeGeneric,
			Title:       "Voice Note",
			Text:        req.VoiceURL,
			ContentHash: extract.HashContent(req.VoiceURL),
			SourceURL:   req.VoiceURL,
		}
	default:
		return nil, fmt.Errorf("at least one of url, text, or voice_url is required")
	}

	// Step 2: Dedup check
	dedup := &DedupChecker{Pool: p.DB}
	dupResult, err := dedup.Check(ctx, extracted.ContentHash)
	if err != nil {
		slog.Warn("dedup check failed, continuing", "error", err)
	} else if dupResult != nil && dupResult.IsDuplicate {
		return nil, &DuplicateError{
			ExistingID: dupResult.ExistingID,
			Title:      dupResult.Title,
		}
	}

	// Step 3: Generate artifact ID
	artifactID := ulid.Make().String()

	// Step 4: Determine processing tier
	tier := AssignTier(TierSignals{
		UserStarred: req.Starred,
		SourceID:    req.SourceID,
		HasContext:  req.Context != "",
		ContentLen:  len(extracted.Text),
	})

	// Step 5: Store initial artifact record (metadata-only until ML processes)
	if err := p.storeInitialArtifact(ctx, artifactID, extracted, req, string(tier)); err != nil {
		return nil, fmt.Errorf("store initial artifact: %w", err)
	}

	// Step 6: Publish to NATS for ML processing
	payload := NATSProcessPayload{
		ArtifactID:     artifactID,
		ContentType:    string(extracted.ContentType),
		URL:            req.URL,
		RawText:        extracted.Text,
		ProcessingTier: string(tier),
		UserContext:    req.Context,
		SourceID:       req.SourceID,
		RetryCount:     0,
	}

	if req.VoiceURL != "" {
		payload.ContentType = "voice"
		payload.URL = req.VoiceURL
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal NATS payload: %w", err)
	}

	if err := p.NATS.Publish(ctx, smacknats.SubjectArtifactsProcess, data); err != nil {
		// Clean up orphaned artifact on NATS publish failure
		if _, cleanupErr := p.DB.Exec(ctx, "DELETE FROM artifacts WHERE id = $1", artifactID); cleanupErr != nil {
			slog.Error("failed to clean up orphaned artifact", "artifact_id", artifactID, "error", cleanupErr)
		} else {
			slog.Warn("cleaned up orphaned artifact after NATS publish failure", "artifact_id", artifactID)
		}
		return nil, fmt.Errorf("publish to NATS: %w", err)
	}

	slog.Info("artifact submitted for processing",
		"artifact_id", artifactID,
		"content_type", extracted.ContentType,
		"tier", tier,
	)

	return &ProcessResult{
		ArtifactID:   artifactID,
		Title:        extracted.Title,
		ArtifactType: string(extracted.ContentType),
		Summary:      "", // Will be populated after ML processing
		Connections:  0,
		Topics:       nil,
		ProcessingMs: time.Since(start).Milliseconds(),
	}, nil
}

// storeInitialArtifact saves the artifact to PostgreSQL before ML processing.
func (p *Processor) storeInitialArtifact(ctx context.Context, id string, result *extract.Result, req *ProcessRequest, tier string) error {
	sourceID := req.SourceID
	if sourceID == "" {
		sourceID = "capture"
	}

	captureMethod := "active"
	sourceURL := result.SourceURL
	if req.VoiceURL != "" {
		sourceURL = req.VoiceURL
	}

	// Truncate content_raw to 500KB to prevent database bloat
	contentRaw := result.Text
	const maxContentRaw = 500 * 1024
	if len(contentRaw) > maxContentRaw {
		contentRaw = contentRaw[:maxContentRaw]
	}

	_, err := p.DB.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, source_url, processing_tier, capture_method, user_starred)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, id, string(result.ContentType), result.Title, contentRaw, result.ContentHash,
		sourceID, sourceURL, tier, captureMethod, req.Starred)
	if err != nil {
		return fmt.Errorf("insert artifact: %w", err)
	}
	return nil
}

// HandleProcessedResult processes the result from the ML sidecar (artifacts.processed).
func (p *Processor) HandleProcessedResult(ctx context.Context, payload *NATSProcessedPayload) error {
	if !payload.Success {
		// Mark artifact as metadata-only on LLM failure
		_, err := p.DB.Exec(ctx, `
			UPDATE artifacts SET processing_tier = 'metadata', updated_at = NOW()
			WHERE id = $1
		`, payload.ArtifactID)
		if err != nil {
			return fmt.Errorf("update artifact on failure: %w", err)
		}
		slog.Warn("ML processing failed for artifact",
			"artifact_id", payload.ArtifactID,
			"error", payload.Error,
		)
		return nil
	}

	// Update artifact with ML results
	entitiesJSON, _ := json.Marshal(payload.Result.Entities)
	keyIdeasJSON, _ := json.Marshal(payload.Result.KeyIdeas)
	actionItemsJSON, _ := json.Marshal(payload.Result.ActionItems)
	topicsJSON, _ := json.Marshal(payload.Result.Topics)

	// Convert embedding to pgvector format
	embeddingStr := formatEmbedding(payload.Embedding)

	_, err := p.DB.Exec(ctx, `
		UPDATE artifacts SET
			artifact_type = $2,
			title = COALESCE(NULLIF($3, ''), title),
			summary = $4,
			key_ideas = $5,
			entities = $6,
			action_items = $7,
			topics = $8,
			sentiment = $9,
			source_quality = $10,
			embedding = $11,
			processing_tier = CASE WHEN $12 = '' THEN processing_tier ELSE $12 END,
			updated_at = NOW()
		WHERE id = $1
	`, payload.ArtifactID, payload.Result.ArtifactType, payload.Result.Title,
		payload.Result.Summary, keyIdeasJSON, entitiesJSON,
		actionItemsJSON, topicsJSON, payload.Result.Sentiment,
		payload.Result.SourceQuality, embeddingStr,
		"") // keep existing tier

	if err != nil {
		return fmt.Errorf("update artifact with ML results: %w", err)
	}

	slog.Info("artifact ML processing complete",
		"artifact_id", payload.ArtifactID,
		"type", payload.Result.ArtifactType,
		"model", payload.ModelUsed,
		"processing_ms", payload.ProcessingMs,
	)

	return nil
}

// formatEmbedding converts a float32 slice to pgvector string format.
func formatEmbedding(vec []float32) string {
	if len(vec) == 0 {
		return ""
	}
	s := "["
	for i, v := range vec {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf("%f", v)
	}
	s += "]"
	return s
}

// DuplicateError indicates that the submitted content already exists.
type DuplicateError struct {
	ExistingID string
	Title      string
}

func (e *DuplicateError) Error() string {
	return fmt.Sprintf("duplicate content: existing artifact %s (%s)", e.ExistingID, e.Title)
}
