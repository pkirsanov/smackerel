package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/bookmarks"
)

// BookmarkPublisher publishes raw artifacts into the processing pipeline.
type BookmarkPublisher interface {
	PublishRawArtifact(ctx context.Context, artifact connector.RawArtifact) (string, error)
}

// BookmarkImportResponse is the JSON response for POST /api/bookmarks/import.
type BookmarkImportResponse struct {
	Imported int      `json:"imported"`
	Errors   []string `json:"errors,omitempty"`
}

// maxBookmarkUploadSize limits bookmark file uploads to 10 MB.
const maxBookmarkUploadSize = 10 << 20

// BookmarkImportHandler handles POST /api/bookmarks/import.
func (d *Dependencies) BookmarkImportHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBookmarkUploadSize)

	if err := r.ParseMultipartForm(maxBookmarkUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Failed to parse multipart form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Missing 'file' field in upload")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Failed to read uploaded file")
		return
	}

	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Uploaded file is empty")
		return
	}

	// Try Chrome JSON first, then Netscape HTML.
	parsed, err := bookmarks.ParseChromeJSON(data)
	if err != nil || len(parsed) == 0 {
		parsed, _ = bookmarks.ParseNetscapeHTML(data)
	}

	if len(parsed) == 0 {
		writeError(w, http.StatusBadRequest, "UNSUPPORTED_FORMAT", "File is neither Chrome JSON nor Netscape HTML bookmark format")
		return
	}

	artifacts := bookmarks.ToRawArtifacts(parsed)

	var importErrors []string
	published := 0

	if d.BookmarkPub != nil {
		for _, art := range artifacts {
			if _, pubErr := d.BookmarkPub.PublishRawArtifact(r.Context(), art); pubErr != nil {
				importErrors = append(importErrors, pubErr.Error())
				slog.Warn("bookmark publish failed", "url", art.URL, "error", pubErr)
			} else {
				published++
			}
		}
	} else {
		// No publisher configured — report parsed count as imported.
		published = len(artifacts)
	}

	slog.Info("bookmark import complete", "parsed", len(parsed), "published", published, "errors", len(importErrors))

	writeJSON(w, http.StatusOK, BookmarkImportResponse{
		Imported: published,
		Errors:   importErrors,
	})
}
