package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// DigestResponse is the JSON response for GET /api/digest.
type DigestResponse struct {
	Date        string `json:"date"`
	Text        string `json:"text"`
	WordCount   int    `json:"word_count"`
	IsQuiet     bool   `json:"is_quiet"`
	GeneratedAt string `json:"generated_at"`
}

// DigestHandler handles GET /api/digest.
func (d *Dependencies) DigestHandler(w http.ResponseWriter, r *http.Request) {
	// Check for date parameter and validate format first (before service check)
	date := r.URL.Query().Get("date")
	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_DATE", "Date must be in YYYY-MM-DD format")
			return
		}
	}

	if d.DigestGen == nil {
		writeError(w, http.StatusServiceUnavailable, "DIGEST_UNAVAILABLE", "Digest service unavailable")
		return
	}

	result, err := d.DigestGen.GetLatest(r.Context(), date)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NO_DIGEST", "No digest generated for this date")
			return
		}
		slog.Error("digest lookup failed", "error", err, "date", date)
		writeError(w, http.StatusInternalServerError, "DIGEST_ERROR", "Failed to retrieve digest")
		return
	}

	writeJSON(w, http.StatusOK, DigestResponse{
		Date:        result.DigestDate.Format("2006-01-02"),
		Text:        result.DigestText,
		WordCount:   result.WordCount,
		IsQuiet:     result.IsQuiet,
		GeneratedAt: result.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}
