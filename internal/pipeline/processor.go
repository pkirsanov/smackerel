package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/extract"
	"github.com/smackerel/smackerel/internal/graph"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/stringutil"
)

// Processing status constants and source ID constants are in constants.go.

// ProcessRequest is the input for the processing pipeline.
type ProcessRequest struct {
	URL          string               `json:"url,omitempty"`
	Text         string               `json:"text,omitempty"`
	VoiceURL     string               `json:"voice_url,omitempty"`
	Context      string               `json:"context,omitempty"`
	SourceID     string               `json:"source_id"`
	Starred      bool                 `json:"starred,omitempty"`
	Conversation *ConversationPayload `json:"conversation,omitempty"`
	MediaGroup   *MediaGroupPayload   `json:"media_group,omitempty"`
	ForwardMeta  *ForwardMetaPayload  `json:"forward_meta,omitempty"`
}

// ConversationPayload carries structured conversation data.
type ConversationPayload struct {
	Participants []string                 `json:"participants"`
	MessageCount int                      `json:"message_count"`
	SourceChat   string                   `json:"source_chat"`
	IsChannel    bool                     `json:"is_channel"`
	Timeline     TimelinePayload          `json:"timeline"`
	Messages     []ConversationMsgPayload `json:"messages"`
}

// TimelinePayload holds conversation time boundaries.
type TimelinePayload struct {
	FirstMessage time.Time `json:"first_message"`
	LastMessage  time.Time `json:"last_message"`
}

// ConversationMsgPayload is a single message within a conversation.
type ConversationMsgPayload struct {
	Sender    string    `json:"sender"`
	Timestamp time.Time `json:"timestamp"`
	Text      string    `json:"text"`
	HasMedia  bool      `json:"has_media,omitempty"`
}

// MediaGroupPayload carries assembled media group data.
type MediaGroupPayload struct {
	Items    []MediaItemPayload `json:"items"`
	Captions string             `json:"captions,omitempty"`
}

// MediaItemPayload represents one item in a media group.
type MediaItemPayload struct {
	Type     string `json:"type"`
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// ForwardMetaPayload carries forwarding metadata.
type ForwardMetaPayload struct {
	SenderName   string    `json:"sender_name"`
	SourceChat   string    `json:"source_chat,omitempty"`
	OriginalDate time.Time `json:"original_date"`
	IsChannel    bool      `json:"is_channel,omitempty"`
}

// ProcessResult is the output after full pipeline processing.
type ProcessResult struct {
	ArtifactID       string   `json:"artifact_id"`
	Title            string   `json:"title"`
	ArtifactType     string   `json:"artifact_type"`
	Summary          string   `json:"summary"`
	Connections      int      `json:"connections"`
	Topics           []string `json:"topics"`
	ProcessingMs     int64    `json:"processing_time_ms"`
	ProcessingStatus string   `json:"processing_status"`
}

// NATSProcessPayload is what core publishes to artifacts.process.
type NATSProcessPayload struct {
	ArtifactID     string                 `json:"artifact_id"`
	ContentType    string                 `json:"content_type"`
	URL            string                 `json:"url,omitempty"`
	RawText        string                 `json:"raw_text"`
	Transcript     string                 `json:"transcript,omitempty"`
	ProcessingTier string                 `json:"processing_tier"`
	UserContext    string                 `json:"user_context,omitempty"`
	SourceID       string                 `json:"source_id"`
	RetryCount     int                    `json:"retry_count"`
	TraceID        string                 `json:"trace_id,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
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

// ValidateProcessPayload checks required fields on an outgoing NATS process payload.
// Catches schema drift at the publish boundary rather than at ML-sidecar runtime.
func ValidateProcessPayload(p *NATSProcessPayload) error {
	if p.ArtifactID == "" {
		return fmt.Errorf("NATSProcessPayload: artifact_id is required")
	}
	if p.ContentType == "" {
		return fmt.Errorf("NATSProcessPayload: content_type is required")
	}
	if p.RawText == "" && p.URL == "" {
		return fmt.Errorf("NATSProcessPayload: at least one of raw_text or url is required")
	}
	return nil
}

// ValidateProcessedPayload checks required fields on an incoming ML result payload.
// Catches schema drift at the receive boundary.
func ValidateProcessedPayload(p *NATSProcessedPayload) error {
	if p.ArtifactID == "" {
		return fmt.Errorf("NATSProcessedPayload: artifact_id is required")
	}
	return nil
}

// NATSDigestGeneratedPayload is what ML sidecar publishes to digest.generated.
type NATSDigestGeneratedPayload struct {
	DigestDate string `json:"digest_date"`
	Text       string `json:"text"`
	WordCount  int    `json:"word_count"`
	ModelUsed  string `json:"model_used,omitempty"`
}

// ValidateDigestGeneratedPayload checks required fields on an incoming digest result.
func ValidateDigestGeneratedPayload(p *NATSDigestGeneratedPayload) error {
	if p.DigestDate == "" {
		return fmt.Errorf("NATSDigestGeneratedPayload: digest_date is required")
	}
	if p.Text == "" {
		return fmt.Errorf("NATSDigestGeneratedPayload: text is required")
	}
	return nil
}

// Processor orchestrates the content processing pipeline.
type Processor struct {
	DB                *pgxpool.Pool
	NATS              *smacknats.Client
	Linker            *graph.Linker
	HospitalityLinker *graph.HospitalityLinker
}

// NewProcessor creates a new pipeline processor.
func NewProcessor(db *pgxpool.Pool, nats *smacknats.Client) *Processor {
	return &Processor{DB: db, NATS: nats, Linker: graph.NewLinker(db)}
}

// Process runs the full pipeline: extract -> dedup -> publish to NATS for ML processing.
// The ML sidecar will asynchronously process and publish results back.
func (p *Processor) Process(ctx context.Context, req *ProcessRequest) (*ProcessResult, error) {
	start := time.Now()

	extracted, err := ExtractContent(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := p.DedupCheck(ctx, req, extracted); err != nil {
		return nil, err
	}

	tier := AssignTier(TierSignals{
		UserStarred: req.Starred,
		SourceID:    req.SourceID,
		HasContext:  req.Context != "",
		ContentLen:  len(extracted.Text),
	})

	result, err := p.submitForProcessing(ctx, req, extracted, tier)
	if err != nil {
		return nil, err
	}

	result.ProcessingMs = time.Since(start).Milliseconds()
	return result, nil
}

// ExtractContent dispatches content extraction based on the request type.
// Handles URL (article, YouTube, image, PDF), plain text, voice note,
// conversation, and media group inputs.
// This function has no DB or NATS dependencies and is independently testable.
func ExtractContent(ctx context.Context, req *ProcessRequest) (*extract.Result, error) {
	// Conversation takes priority — structured data available
	if req.Conversation != nil {
		text := req.Text
		if text == "" {
			text = formatConversationText(req.Conversation)
		}
		return &extract.Result{
			ContentType: extract.ContentTypeConversation,
			Title:       conversationTitle(req.Conversation),
			Text:        text,
			ContentHash: extract.HashContent(text),
		}, nil
	}

	// Media group
	if req.MediaGroup != nil {
		text := req.Text
		if text == "" {
			text = req.MediaGroup.Captions
		}
		if text == "" {
			text = fmt.Sprintf("Media group: %d items", len(req.MediaGroup.Items))
		}
		return &extract.Result{
			ContentType: extract.ContentTypeMediaGroup,
			Title:       fmt.Sprintf("Media group (%d items)", len(req.MediaGroup.Items)),
			Text:        text,
			ContentHash: extract.HashContent(text),
		}, nil
	}

	switch {
	case req.URL != "":
		contentType := extract.DetectContentType(req.URL)
		switch contentType {
		case extract.ContentTypeYouTube:
			return &extract.Result{
				ContentType: extract.ContentTypeYouTube,
				Title:       "YouTube Video",
				Text:        req.URL,
				ContentHash: extract.HashContent(req.URL),
				SourceURL:   req.URL,
				VideoID:     extract.ExtractYouTubeID(req.URL),
			}, nil
		case extract.ContentTypeImage:
			return &extract.Result{
				ContentType: extract.ContentTypeImage,
				Title:       "Image",
				Text:        req.URL,
				ContentHash: extract.HashContent(req.URL),
				SourceURL:   req.URL,
			}, nil
		case extract.ContentTypePDF:
			return &extract.Result{
				ContentType: extract.ContentTypePDF,
				Title:       "PDF Document",
				Text:        req.URL,
				ContentHash: extract.HashContent(req.URL),
				SourceURL:   req.URL,
			}, nil
		default:
			result, err := extract.ExtractArticle(ctx, req.URL)
			if err != nil {
				return nil, fmt.Errorf("content extraction failed: %w", err)
			}
			return result, nil
		}
	case req.Text != "":
		return extract.ExtractText(req.Text), nil
	case req.VoiceURL != "":
		return &extract.Result{
			ContentType: extract.ContentTypeVoice,
			Title:       "Voice Note",
			Text:        req.VoiceURL,
			ContentHash: extract.HashContent(req.VoiceURL),
			SourceURL:   req.VoiceURL,
		}, nil
	default:
		return nil, fmt.Errorf("at least one of url, text, or voice_url is required")
	}
}

// formatConversationText builds a human-readable text representation from conversation payload.
func formatConversationText(c *ConversationPayload) string {
	var parts []string
	header := "Conversation"
	if c.SourceChat != "" {
		header += " from " + c.SourceChat
	}
	parts = append(parts, header)
	if len(c.Participants) > 0 {
		parts = append(parts, fmt.Sprintf("Participants: %s", strings.Join(c.Participants, ", ")))
	}
	parts = append(parts, fmt.Sprintf("Messages: %d", c.MessageCount))
	parts = append(parts, "---")
	for _, m := range c.Messages {
		ts := m.Timestamp.Format("15:04")
		line := fmt.Sprintf("[%s] %s: %s", ts, m.Sender, m.Text)
		parts = append(parts, line)
	}
	return strings.Join(parts, "\n")
}

// conversationTitle generates a title for a conversation artifact.
func conversationTitle(c *ConversationPayload) string {
	if c.SourceChat != "" {
		return fmt.Sprintf("Conversation from %s (%d messages)", c.SourceChat, c.MessageCount)
	}
	if len(c.Participants) > 0 && len(c.Participants) <= 3 {
		return fmt.Sprintf("Conversation with %s", strings.Join(c.Participants, ", "))
	}
	return fmt.Sprintf("Conversation (%d messages, %d participants)", c.MessageCount, len(c.Participants))
}

// DedupCheck performs deduplication: URL-first (R-011 delta re-processing), then content hash.
// Returns nil if the content is new or has changed (delta), DuplicateError if it's a true duplicate.
func (p *Processor) DedupCheck(ctx context.Context, req *ProcessRequest, extracted *extract.Result) error {
	dedup := &DedupChecker{Pool: p.DB}

	if req.URL != "" {
		urlResult, err := dedup.CheckURL(ctx, req.URL)
		if err != nil {
			slog.Warn("URL dedup check failed, continuing", "error", err)
			return nil
		}
		if urlResult != nil && urlResult.IsDuplicate {
			// URL exists — check if content actually changed (delta re-processing, R-011)
			hashResult, hashErr := dedup.Check(ctx, extracted.ContentHash)
			if hashErr != nil {
				slog.Warn("content hash check for delta failed, treating as duplicate", "error", hashErr)
				return &DuplicateError{
					ExistingID: urlResult.ExistingID,
					Title:      urlResult.Title,
				}
			}
			if hashResult != nil && hashResult.IsDuplicate {
				// Same URL, same content — true duplicate
				return &DuplicateError{
					ExistingID: urlResult.ExistingID,
					Title:      urlResult.Title,
				}
			}
			// Same URL, different content — delta re-processing (R-011)
			slog.Info("delta re-processing: URL exists but content changed",
				"url", req.URL,
				"existing_id", urlResult.ExistingID,
			)
		}
		return nil
	}

	// No URL — standard content-hash dedup only
	dupResult, err := dedup.Check(ctx, extracted.ContentHash)
	if err != nil {
		slog.Warn("dedup check failed, continuing", "error", err)
		return nil
	}
	if dupResult != nil && dupResult.IsDuplicate {
		return &DuplicateError{
			ExistingID: dupResult.ExistingID,
			Title:      dupResult.Title,
		}
	}
	return nil
}

// submitForProcessing stores the initial artifact and publishes to NATS for ML processing.
// Cleans up the DB record if NATS publish fails.
func (p *Processor) submitForProcessing(ctx context.Context, req *ProcessRequest, extracted *extract.Result, tier Tier) (*ProcessResult, error) {
	artifactID := ulid.Make().String()

	if err := p.storeInitialArtifact(ctx, artifactID, extracted, req, string(tier)); err != nil {
		return nil, fmt.Errorf("store initial artifact: %w", err)
	}

	payload := NATSProcessPayload{
		ArtifactID:     artifactID,
		ContentType:    string(extracted.ContentType),
		URL:            req.URL,
		RawText:        extracted.Text,
		ProcessingTier: string(tier),
		UserContext:    req.Context,
		SourceID:       req.SourceID,
		RetryCount:     0,
		TraceID:        middleware.GetReqID(ctx),
	}

	if req.VoiceURL != "" {
		payload.ContentType = string(extract.ContentTypeVoice)
		payload.URL = req.VoiceURL
	}

	if err := ValidateProcessPayload(&payload); err != nil {
		return nil, fmt.Errorf("validate NATS payload: %w", err)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal NATS payload: %w", err)
	}

	if len(data) > MaxNATSMessageSize {
		slog.Warn("NATS payload exceeds max message size",
			"artifact_id", artifactID,
			"payload_size", len(data),
			"max_size", MaxNATSMessageSize,
			"source_id", req.SourceID,
		)
		return nil, fmt.Errorf("NATS payload too large: %d bytes exceeds max %d", len(data), MaxNATSMessageSize)
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
		ArtifactID:       artifactID,
		Title:            extracted.Title,
		ArtifactType:     string(extracted.ContentType),
		Summary:          "",
		Connections:      0,
		Topics:           nil,
		ProcessingStatus: string(StatusPending),
	}, nil
}

// storeInitialArtifact saves the artifact to PostgreSQL before ML processing.
func (p *Processor) storeInitialArtifact(ctx context.Context, id string, result *extract.Result, req *ProcessRequest, tier string) error {
	sourceID := req.SourceID
	if sourceID == "" {
		sourceID = SourceCapture
	}

	captureMethod := "active"
	sourceURL := result.SourceURL
	if req.VoiceURL != "" {
		sourceURL = req.VoiceURL
	}

	// Truncate content_raw to 500KB to prevent database bloat.
	// Use rune-safe truncation to avoid splitting multi-byte UTF-8 characters.
	contentRaw := result.Text
	const maxContentRaw = 500 * 1024
	if len(contentRaw) > maxContentRaw {
		contentRaw = stringutil.TruncateUTF8(contentRaw, maxContentRaw)
	}

	// Use ON CONFLICT to handle the TOCTOU race: if a concurrent request already
	// inserted the same content_hash, this INSERT becomes a no-op and we return
	// a DuplicateError consistent with the explicit dedup check path.
	ct, err := p.DB.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, source_url, processing_tier, capture_method, user_starred, processing_status, participants, message_count, source_chat, timeline)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (content_hash) WHERE content_hash IS NOT NULL DO NOTHING
	`, id, string(result.ContentType), result.Title, contentRaw, result.ContentHash,
		sourceID, sourceURL, tier, captureMethod, req.Starred, string(StatusPending),
		conversationParticipantsJSON(req), conversationMessageCount(req),
		conversationSourceChat(req), conversationTimelineJSON(req))
	if err != nil {
		return fmt.Errorf("insert artifact: %w", err)
	}
	if ct.RowsAffected() == 0 {
		// Concurrent insert won — look up the winner to return a proper DuplicateError.
		var existingID, existingTitle string
		lookupErr := p.DB.QueryRow(ctx,
			"SELECT id, title FROM artifacts WHERE content_hash = $1 LIMIT 1",
			result.ContentHash,
		).Scan(&existingID, &existingTitle)
		if lookupErr != nil {
			return fmt.Errorf("lookup concurrent duplicate: %w", lookupErr)
		}
		return &DuplicateError{ExistingID: existingID, Title: existingTitle}
	}
	return nil
}

// conversationParticipantsJSON returns the participants as a JSON byte slice for JSONB storage,
// or nil when the request is not a conversation.
func conversationParticipantsJSON(req *ProcessRequest) []byte {
	if req.Conversation == nil || len(req.Conversation.Participants) == 0 {
		return nil
	}
	data, err := json.Marshal(req.Conversation.Participants)
	if err != nil {
		return nil
	}
	return data
}

// conversationMessageCount returns the message count, or nil when not a conversation.
func conversationMessageCount(req *ProcessRequest) *int {
	if req.Conversation == nil {
		return nil
	}
	return &req.Conversation.MessageCount
}

// conversationSourceChat returns the source chat name, or nil when not a conversation.
func conversationSourceChat(req *ProcessRequest) *string {
	if req.Conversation == nil || req.Conversation.SourceChat == "" {
		return nil
	}
	return &req.Conversation.SourceChat
}

// conversationTimelineJSON returns timeline data as JSON for JSONB storage,
// or nil when the request is not a conversation.
func conversationTimelineJSON(req *ProcessRequest) []byte {
	if req.Conversation == nil {
		return nil
	}
	data, err := json.Marshal(req.Conversation.Timeline)
	if err != nil {
		return nil
	}
	return data
}

// HandleProcessedResult processes the result from the ML sidecar (artifacts.processed).
func (p *Processor) HandleProcessedResult(ctx context.Context, payload *NATSProcessedPayload) error {
	if p.DB == nil {
		return fmt.Errorf("database pool is nil")
	}

	if err := ValidateProcessedPayload(payload); err != nil {
		return fmt.Errorf("validate processed payload: %w", err)
	}

	if !payload.Success {
		// Mark artifact as metadata-only on LLM failure and set processing_status
		// to 'failed' so it can be distinguished from still-pending artifacts.
		_, err := p.DB.Exec(ctx, `
			UPDATE artifacts SET processing_tier = 'metadata', processing_status = $2, updated_at = NOW()
			WHERE id = $1
		`, payload.ArtifactID, string(StatusFailed))
		if err != nil {
			return fmt.Errorf("update artifact on failure: %w", err)
		}
		slog.Warn("ML processing failed for artifact",
			"artifact_id", payload.ArtifactID,
			"error", payload.Error,
		)
		return nil
	}

	// Update artifact with ML results — propagate marshal errors instead of
	// silently storing nil/empty JSON which was a recurring bug-fix trigger.
	entitiesJSON, err := json.Marshal(payload.Result.Entities)
	if err != nil {
		return fmt.Errorf("marshal entities for artifact %s: %w", payload.ArtifactID, err)
	}
	keyIdeasJSON, err := json.Marshal(payload.Result.KeyIdeas)
	if err != nil {
		return fmt.Errorf("marshal key_ideas for artifact %s: %w", payload.ArtifactID, err)
	}
	actionItemsJSON, err := json.Marshal(payload.Result.ActionItems)
	if err != nil {
		return fmt.Errorf("marshal action_items for artifact %s: %w", payload.ArtifactID, err)
	}
	topicsJSON, err := json.Marshal(payload.Result.Topics)
	if err != nil {
		return fmt.Errorf("marshal topics for artifact %s: %w", payload.ArtifactID, err)
	}

	// Convert embedding to pgvector format
	embeddingStr := db.FormatEmbedding(payload.Embedding)

	ct, err := p.DB.Exec(ctx, `
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
			processing_status = $13,
			updated_at = NOW()
		WHERE id = $1
	`, payload.ArtifactID, payload.Result.ArtifactType, payload.Result.Title,
		payload.Result.Summary, keyIdeasJSON, entitiesJSON,
		actionItemsJSON, topicsJSON, payload.Result.Sentiment,
		payload.Result.SourceQuality, embeddingStr,
		"", // keep existing tier
		string(StatusProcessed))

	if err != nil {
		return fmt.Errorf("update artifact with ML results: %w", err)
	}

	if ct.RowsAffected() == 0 {
		slog.Warn("artifact not found for update", "artifact_id", payload.ArtifactID)
	}

	slog.Info("artifact ML processing complete",
		"artifact_id", payload.ArtifactID,
		"type", payload.Result.ArtifactType,
		"model", payload.ModelUsed,
		"processing_ms", payload.ProcessingMs,
	)

	// Link artifact in knowledge graph — creates edges via similarity,
	// entity, topic, and temporal strategies
	if p.Linker != nil {
		edgeCount, linkErr := p.Linker.LinkArtifact(ctx, payload.ArtifactID)
		if linkErr != nil {
			slog.Warn("knowledge graph linking failed",
				"artifact_id", payload.ArtifactID,
				"error", linkErr,
			)
		} else if edgeCount > 0 {
			slog.Info("knowledge graph linked",
				"artifact_id", payload.ArtifactID,
				"edges_created", edgeCount,
			)
		}
	}

	// Hospitality-specific graph linking for GuestHost connector artifacts
	if p.HospitalityLinker != nil {
		if err := p.HospitalityLinker.LinkArtifact(ctx, payload.ArtifactID); err != nil {
			slog.Warn("hospitality graph linking failed",
				"artifact_id", payload.ArtifactID,
				"error", err,
			)
		}
	}

	return nil
}

// DuplicateError indicates that the submitted content already exists.
type DuplicateError struct {
	ExistingID string
	Title      string
}

func (e *DuplicateError) Error() string {
	return fmt.Sprintf("duplicate content: existing artifact %s (%s)", e.ExistingID, e.Title)
}
