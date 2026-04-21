package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smackerel/smackerel/internal/metrics"
	"github.com/smackerel/smackerel/internal/pipeline"
)

// CaptureRequest is the JSON body for POST /api/capture.
type CaptureRequest struct {
	URL          string               `json:"url,omitempty"`
	Text         string               `json:"text,omitempty"`
	VoiceURL     string               `json:"voice_url,omitempty"`
	Context      string               `json:"context,omitempty"`
	Conversation *ConversationPayload `json:"conversation,omitempty"`
	MediaGroup   *MediaGroupPayload   `json:"media_group,omitempty"`
	ForwardMeta  *ForwardMetaPayload  `json:"forward_meta,omitempty"`
}

// ConversationPayload carries structured conversation data from assembled forwarded messages.
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

// ForwardMetaPayload carries forwarding metadata for a single forwarded message.
type ForwardMetaPayload struct {
	SenderName   string    `json:"sender_name"`
	SourceChat   string    `json:"source_chat,omitempty"`
	OriginalDate time.Time `json:"original_date"`
	IsChannel    bool      `json:"is_channel,omitempty"`
}

// CaptureResponse is the success response for POST /api/capture.
type CaptureResponse struct {
	ArtifactID   string   `json:"artifact_id"`
	Title        string   `json:"title"`
	ArtifactType string   `json:"artifact_type"`
	Summary      string   `json:"summary"`
	Connections  int      `json:"connections"`
	Topics       []string `json:"topics"`
	ProcessingMs int64    `json:"processing_time_ms"`
}

// ErrorResponse is the standard error response body.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error code and message.
type ErrorDetail struct {
	Code               string `json:"code"`
	Message            string `json:"message"`
	ExistingArtifactID string `json:"existing_artifact_id,omitempty"`
	Title              string `json:"title,omitempty"`
}

// CaptureHandler handles POST /api/capture.
func (d *Dependencies) CaptureHandler(w http.ResponseWriter, r *http.Request) {
	// Check DB health before processing — fail visible on DB outage.
	// A nil DB is treated as unavailable (misconfiguration or startup race).
	if d.DB == nil || !d.DB.Healthy(r.Context()) {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE",
			"Database is temporarily unavailable, please retry")
		return
	}

	var req CaptureRequest
	if !decodeJSONBody(w, r, &req, "INVALID_INPUT", "Invalid JSON body") {
		return
	}

	// Validate: at least one input field required
	if req.URL == "" && req.Text == "" && req.VoiceURL == "" && req.Conversation == nil && req.MediaGroup == nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "At least one of url, text, voice_url, conversation, or media_group is required")
		return
	}

	// Get the pipeline processor
	if d.Pipeline == nil {
		writeError(w, http.StatusServiceUnavailable, "ML_UNAVAILABLE", "Processing service unavailable")
		return
	}

	// Process the capture
	result, err := d.Pipeline.Process(r.Context(), &pipeline.ProcessRequest{
		URL:          req.URL,
		Text:         req.Text,
		VoiceURL:     req.VoiceURL,
		Context:      req.Context,
		SourceID:     pipeline.SourceCapture,
		Conversation: toPipelineConversation(req.Conversation),
		MediaGroup:   toPipelineMediaGroup(req.MediaGroup),
		ForwardMeta:  toPipelineForwardMeta(req.ForwardMeta),
	})

	if err != nil {
		// Check for specific error types
		var dupErr *pipeline.DuplicateError
		if errors.As(err, &dupErr) {
			writeJSON(w, http.StatusConflict, ErrorResponse{
				Error: ErrorDetail{
					Code:               "DUPLICATE_DETECTED",
					Message:            "Already saved",
					ExistingArtifactID: dupErr.ExistingID,
					Title:              dupErr.Title,
				},
			})
			return
		}

		if errors.Is(err, pipeline.ErrExtractionFailed) {
			writeError(w, http.StatusUnprocessableEntity, "EXTRACTION_FAILED", err.Error())
			return
		}

		if errors.Is(err, pipeline.ErrNATSPublish) {
			writeError(w, http.StatusServiceUnavailable, "ML_UNAVAILABLE", "Processing service unavailable")
			return
		}

		// Re-check DB health: if the database became unreachable during processing,
		// return 503 DB_UNAVAILABLE instead of a generic 500 so callers can distinguish
		// transient DB outage from other processing failures (chaos finding C-001).
		if d.DB != nil && !d.DB.Healthy(r.Context()) {
			writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE",
				"Database is temporarily unavailable, please retry")
			return
		}

		slog.Error("capture processing failed", "error", err)
		writeError(w, http.StatusInternalServerError, "PROCESSING_FAILED", "Internal processing error")
		return
	}

	resp := CaptureResponse{
		ArtifactID:   result.ArtifactID,
		Title:        result.Title,
		ArtifactType: result.ArtifactType,
		Summary:      result.Summary,
		Connections:  result.Connections,
		Topics:       result.Topics,
		ProcessingMs: result.ProcessingMs,
	}

	metrics.CaptureTotal.WithLabelValues(captureSource(r)).Inc()

	writeJSON(w, http.StatusOK, resp)
}

// toPipelineConversation converts an API ConversationPayload to a pipeline ConversationPayload.
func toPipelineConversation(c *ConversationPayload) *pipeline.ConversationPayload {
	if c == nil {
		return nil
	}
	msgs := make([]pipeline.ConversationMsgPayload, len(c.Messages))
	for i, m := range c.Messages {
		msgs[i] = pipeline.ConversationMsgPayload{
			Sender:    m.Sender,
			Timestamp: m.Timestamp,
			Text:      m.Text,
			HasMedia:  m.HasMedia,
		}
	}
	return &pipeline.ConversationPayload{
		Participants: c.Participants,
		MessageCount: c.MessageCount,
		SourceChat:   c.SourceChat,
		IsChannel:    c.IsChannel,
		Timeline: pipeline.TimelinePayload{
			FirstMessage: c.Timeline.FirstMessage,
			LastMessage:  c.Timeline.LastMessage,
		},
		Messages: msgs,
	}
}

// toPipelineMediaGroup converts an API MediaGroupPayload to a pipeline MediaGroupPayload.
func toPipelineMediaGroup(mg *MediaGroupPayload) *pipeline.MediaGroupPayload {
	if mg == nil {
		return nil
	}
	items := make([]pipeline.MediaItemPayload, len(mg.Items))
	for i, it := range mg.Items {
		items[i] = pipeline.MediaItemPayload{
			Type:     it.Type,
			FileID:   it.FileID,
			FileSize: it.FileSize,
			MimeType: it.MimeType,
		}
	}
	return &pipeline.MediaGroupPayload{
		Items:    items,
		Captions: mg.Captions,
	}
}

// toPipelineForwardMeta converts an API ForwardMetaPayload to a pipeline ForwardMetaPayload.
func toPipelineForwardMeta(fm *ForwardMetaPayload) *pipeline.ForwardMetaPayload {
	if fm == nil {
		return nil
	}
	return &pipeline.ForwardMetaPayload{
		SenderName:   fm.SenderName,
		SourceChat:   fm.SourceChat,
		OriginalDate: fm.OriginalDate,
		IsChannel:    fm.IsChannel,
	}
}

// validCaptureSources is the bounded set of allowed capture source label values.
var validCaptureSources = map[string]bool{
	"api":       true,
	"telegram":  true,
	"extension": true,
	"pwa":       true,
}

// captureSource reads the X-Capture-Source header from the request and validates
// it against the known set. Returns "api" if the header is missing or unknown.
func captureSource(r *http.Request) string {
	src := r.Header.Get("X-Capture-Source")
	if validCaptureSources[src] {
		return src
	}
	return "api"
}

// decodeJSONBody validates Content-Type, limits body size to 1MB, and decodes
// JSON into dst. Returns true on success. On failure it writes an error
// response to w and returns false so callers can simply return.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, errCode, errMsg string) bool {
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, errCode, errMsg)
		return false
	}
	return true
}

// writeError writes a standardized error response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{
		Error: ErrorDetail{Code: code, Message: message},
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// RecentItem represents a single recent artifact in the response.
type RecentItem struct {
	ID           string `json:"artifact_id"`
	Title        string `json:"title"`
	ArtifactType string `json:"artifact_type"`
	Summary      string `json:"summary"`
	CreatedAt    string `json:"created_at"`
}

// RecentResponse is the JSON response for GET /api/recent.
type RecentResponse struct {
	Results []RecentItem `json:"results"`
}

// ArtifactDetailResponse is the JSON response for GET /api/artifact/{id}.
type ArtifactDetailResponse struct {
	ArtifactID     string `json:"artifact_id"`
	Title          string `json:"title"`
	ArtifactType   string `json:"artifact_type"`
	Summary        string `json:"summary"`
	SourceURL      string `json:"source_url"`
	Sentiment      string `json:"sentiment"`
	SourceQuality  string `json:"source_quality"`
	ProcessingTier string `json:"processing_tier"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// RecentHandler handles GET /api/recent.
func (d *Dependencies) RecentHandler(w http.ResponseWriter, r *http.Request) {
	if d.ArtifactStore == nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Service unavailable")
		return
	}

	limit := 5
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 50 {
		limit = 50
	}

	items, err := d.ArtifactStore.RecentArtifacts(r.Context(), limit)
	if err != nil {
		slog.Error("recent query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "Failed to fetch recent artifacts")
		return
	}

	results := make([]RecentItem, 0, len(items))
	for _, a := range items {
		results = append(results, RecentItem{
			ID:           a.ID,
			Title:        a.Title,
			ArtifactType: a.ArtifactType,
			Summary:      a.Summary,
			CreatedAt:    a.CreatedAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, RecentResponse{
		Results: results,
	})
}

// ArtifactDetailHandler handles GET /api/artifact/{id}.
// maxArtifactIDLen limits artifact ID length to prevent abuse (ULIDs are 26 chars).
const maxArtifactIDLen = 128

func (d *Dependencies) ArtifactDetailHandler(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "id")
	if artifactID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Artifact ID is required")
		return
	}
	if len(artifactID) > maxArtifactIDLen {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Artifact ID exceeds maximum length")
		return
	}

	if d.ArtifactStore == nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Service unavailable")
		return
	}

	a, err := d.ArtifactStore.GetArtifact(r.Context(), artifactID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Artifact not found")
		return
	}

	writeJSON(w, http.StatusOK, ArtifactDetailResponse{
		ArtifactID:     a.ID,
		Title:          a.Title,
		ArtifactType:   a.ArtifactType,
		Summary:        a.Summary,
		SourceURL:      a.SourceURL,
		Sentiment:      a.Sentiment,
		SourceQuality:  a.SourceQuality,
		ProcessingTier: a.ProcessingTier,
		CreatedAt:      a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      a.UpdatedAt.Format(time.RFC3339),
	})
}

// ExportHandler streams artifacts as JSONL for backup/export with cursor-based pagination.
func (d *Dependencies) ExportHandler(w http.ResponseWriter, r *http.Request) {
	// Parse cursor (RFC3339 timestamp)
	var cursor time.Time
	if c := r.URL.Query().Get("cursor"); c != "" {
		var err error
		cursor, err = time.Parse(time.RFC3339, c)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_INPUT", "cursor must be RFC3339 timestamp")
			return
		}
	}

	// Parse limit (default 10000, max 10000)
	limit := 10000
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 10000 {
		limit = 10000
	}

	if d.ArtifactStore == nil {
		writeError(w, http.StatusServiceUnavailable, "EXPORT_UNAVAILABLE", "Export not supported")
		return
	}

	result, err := d.ArtifactStore.ExportArtifacts(r.Context(), cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EXPORT_FAILED", "Failed to export artifacts")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", "attachment; filename=smackerel-export.jsonl")
	if !result.NextCursor.IsZero() {
		w.Header().Set("X-Next-Cursor", result.NextCursor.Format(time.RFC3339))
	}

	enc := json.NewEncoder(w)
	for _, a := range result.Artifacts {
		enc.Encode(a)
	}
	slog.Info("export complete", "artifacts", len(result.Artifacts))
}
