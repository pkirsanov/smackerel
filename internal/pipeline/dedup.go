package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dedupQueryTimeout is the maximum time allowed for a dedup DB query.
// On timeout, the artifact is treated as non-duplicate (fail-open) to
// prefer double-ingest over data loss.
const dedupQueryTimeout = 5 * time.Second

// DedupChecker checks for duplicate content based on content hash.
type DedupChecker struct {
	Pool *pgxpool.Pool
}

// DedupResult contains information about a duplicate check.
type DedupResult struct {
	IsDuplicate bool   `json:"is_duplicate"`
	ExistingID  string `json:"existing_id,omitempty"`
	Title       string `json:"title,omitempty"`
}

// Check looks up a content hash in the artifacts table.
// Returns the existing artifact ID if found, empty string otherwise.
// Uses a timeout to prevent blocking forever on slow DB queries.
// On timeout, returns non-duplicate (fail-open: prefer double-ingest over data loss).
func (d *DedupChecker) Check(ctx context.Context, contentHash string) (*DedupResult, error) {
	queryCtx, cancel := context.WithTimeout(ctx, dedupQueryTimeout)
	defer cancel()

	var id, title string
	err := d.Pool.QueryRow(queryCtx,
		"SELECT id, title FROM artifacts WHERE content_hash = $1 LIMIT 1",
		contentHash,
	).Scan(&id, &title)

	if err != nil {
		// No rows = not a duplicate
		if errors.Is(err, pgx.ErrNoRows) {
			return &DedupResult{IsDuplicate: false}, nil
		}
		// Timeout or cancellation — fail-open
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			slog.Warn("dedup check timed out, treating as non-duplicate (fail-open)",
				"content_hash", contentHash,
				"timeout", dedupQueryTimeout,
			)
			return &DedupResult{IsDuplicate: false}, nil
		}
		return nil, fmt.Errorf("check duplicate: %w", err)
	}

	return &DedupResult{
		IsDuplicate: true,
		ExistingID:  id,
		Title:       title,
	}, nil
}

// CheckURL looks up a source URL in the artifacts table.
// Catches re-submission of the same URL even when content has changed (R-011).
// Uses a timeout to prevent blocking forever on slow DB queries.
// On timeout, returns non-duplicate (fail-open: prefer double-ingest over data loss).
func (d *DedupChecker) CheckURL(ctx context.Context, sourceURL string) (*DedupResult, error) {
	if sourceURL == "" {
		return &DedupResult{IsDuplicate: false}, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, dedupQueryTimeout)
	defer cancel()

	var id, title string
	err := d.Pool.QueryRow(queryCtx,
		"SELECT id, title FROM artifacts WHERE source_url = $1 LIMIT 1",
		sourceURL,
	).Scan(&id, &title)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &DedupResult{IsDuplicate: false}, nil
		}
		// Timeout or cancellation — fail-open
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			slog.Warn("URL dedup check timed out, treating as non-duplicate (fail-open)",
				"source_url", sourceURL,
				"timeout", dedupQueryTimeout,
			)
			return &DedupResult{IsDuplicate: false}, nil
		}
		return nil, fmt.Errorf("check URL duplicate: %w", err)
	}

	return &DedupResult{
		IsDuplicate: true,
		ExistingID:  id,
		Title:       title,
	}, nil
}
