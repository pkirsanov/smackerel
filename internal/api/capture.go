package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/pipeline"
)

// CaptureRequest is the JSON body for POST /api/capture.
type CaptureRequest struct {
	URL      string `json:"url,omitempty"`
	Text     string `json:"text,omitempty"`
	VoiceURL string `json:"voice_url,omitempty"`
	Context  string `json:"context,omitempty"`
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
	// Check DB health before processing — fail visible on DB outage
	if d.DB != nil && !d.DB.Healthy(r.Context()) {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE",
			"Database is temporarily unavailable, please retry")
		return
	}

	var req CaptureRequest
	// Limit request body to 1MB to prevent memory exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Invalid JSON body")
		return
	}

	// Validate: at least one input field required
	if req.URL == "" && req.Text == "" && req.VoiceURL == "" {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "At least one of url, text, or voice_url is required")
		return
	}

	// Get the pipeline processor
	if d.Pipeline == nil {
		writeError(w, http.StatusServiceUnavailable, "ML_UNAVAILABLE", "Processing service unavailable")
		return
	}

	// Process the capture
	result, err := d.Pipeline.Process(r.Context(), &pipeline.ProcessRequest{
		URL:      req.URL,
		Text:     req.Text,
		VoiceURL: req.VoiceURL,
		Context:  req.Context,
		SourceID: pipeline.SourceCapture,
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

		if strings.Contains(err.Error(), "content extraction failed") {
			writeError(w, http.StatusUnprocessableEntity, "EXTRACTION_FAILED", err.Error())
			return
		}

		if strings.Contains(err.Error(), "publish to NATS") {
			writeError(w, http.StatusServiceUnavailable, "ML_UNAVAILABLE", "Processing service unavailable")
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

	writeJSON(w, http.StatusOK, resp)
}

// writeError writes a standardized error response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{
		Error: ErrorDetail{Code: code, Message: message},
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// RecentHandler handles GET /api/recent.
func (d *Dependencies) RecentHandler(w http.ResponseWriter, r *http.Request) {
	engine, ok := d.SearchEngine.(*SearchEngine)
	if !ok || engine == nil {
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

	rows, err := engine.Pool.Query(r.Context(), `
		SELECT id, title, artifact_type, COALESCE(summary, ''), created_at
		FROM artifacts ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		slog.Error("recent query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "Failed to fetch recent artifacts")
		return
	}
	defer rows.Close()

	type RecentItem struct {
		ID           string `json:"artifact_id"`
		Title        string `json:"title"`
		ArtifactType string `json:"artifact_type"`
		Summary      string `json:"summary"`
		CreatedAt    string `json:"created_at"`
	}

	var items []RecentItem
	for rows.Next() {
		var item RecentItem
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.Title, &item.ArtifactType, &item.Summary, &createdAt); err != nil {
			continue
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		slog.Error("recent items row iteration error", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": items,
	})
}

// ArtifactDetailHandler handles GET /api/artifact/{id}.
func (d *Dependencies) ArtifactDetailHandler(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "id")
	if artifactID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Artifact ID is required")
		return
	}

	engine, ok := d.SearchEngine.(*SearchEngine)
	if !ok || engine == nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Service unavailable")
		return
	}

	var (
		id, title, artifactType, summary, sourceURL, sentiment, sourceQuality, processingTier string
		createdAt, updatedAt                                                                  time.Time
	)
	err := engine.Pool.QueryRow(r.Context(), `
		SELECT id, title, artifact_type, COALESCE(summary, ''), COALESCE(source_url, ''),
		       COALESCE(sentiment, ''), COALESCE(source_quality, ''), COALESCE(processing_tier, ''),
		       created_at, updated_at
		FROM artifacts WHERE id = $1
	`, artifactID).Scan(&id, &title, &artifactType, &summary, &sourceURL,
		&sentiment, &sourceQuality, &processingTier,
		&createdAt, &updatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Artifact not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"artifact_id":     id,
		"title":           title,
		"artifact_type":   artifactType,
		"summary":         summary,
		"source_url":      sourceURL,
		"sentiment":       sentiment,
		"source_quality":  sourceQuality,
		"processing_tier": processingTier,
		"created_at":      createdAt.Format(time.RFC3339),
		"updated_at":      updatedAt.Format(time.RFC3339),
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

	// Get query capability from DB
	type querier interface {
		ExportArtifacts(ctx context.Context, cursor time.Time, limit int) (*db.ExportResult, error)
	}
	exporter, ok := d.DB.(querier)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "EXPORT_UNAVAILABLE", "Export not supported")
		return
	}

	result, err := exporter.ExportArtifacts(r.Context(), cursor, limit)
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
