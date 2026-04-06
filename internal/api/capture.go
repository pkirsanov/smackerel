package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

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
	// Require authentication
	if !d.checkAuth(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authentication required")
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
	proc, ok := d.Pipeline.(*pipeline.Processor)
	if !ok || proc == nil {
		writeError(w, http.StatusServiceUnavailable, "ML_UNAVAILABLE", "Processing service unavailable")
		return
	}

	// Process the capture
	result, err := proc.Process(r.Context(), &pipeline.ProcessRequest{
		URL:      req.URL,
		Text:     req.Text,
		VoiceURL: req.VoiceURL,
		Context:  req.Context,
		SourceID: "capture",
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

// checkAuth validates the Bearer token from the Authorization header.
func (d *Dependencies) checkAuth(r *http.Request) bool {
	if d.AuthToken == "" {
		return true // No auth configured = allow all (development)
	}

	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(d.AuthToken)) == 1
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
