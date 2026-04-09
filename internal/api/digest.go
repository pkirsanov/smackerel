package api

import (
	"net/http"

	"github.com/smackerel/smackerel/internal/digest"
)

// DigestHandler handles GET /api/digest.
func (d *Dependencies) DigestHandler(w http.ResponseWriter, r *http.Request) {
	gen, ok := d.DigestGen.(*digest.Generator)
	if !ok || gen == nil {
		writeError(w, http.StatusServiceUnavailable, "DIGEST_UNAVAILABLE", "Digest service unavailable")
		return
	}

	// Check for date parameter
	date := r.URL.Query().Get("date")

	result, err := gen.GetLatest(r.Context(), date)
	if err != nil {
		writeError(w, http.StatusNotFound, "NO_DIGEST", "No digest generated for this date")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"date":         result.DigestDate.Format("2006-01-02"),
		"text":         result.DigestText,
		"word_count":   result.WordCount,
		"is_quiet":     result.IsQuiet,
		"generated_at": result.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}
