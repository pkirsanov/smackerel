package pipeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
func (d *DedupChecker) Check(ctx context.Context, contentHash string) (*DedupResult, error) {
	var id, title string
	err := d.Pool.QueryRow(ctx,
		"SELECT id, title FROM artifacts WHERE content_hash = $1 LIMIT 1",
		contentHash,
	).Scan(&id, &title)

	if err != nil {
		// No rows = not a duplicate
		if errors.Is(err, pgx.ErrNoRows) {
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
func (d *DedupChecker) CheckURL(ctx context.Context, sourceURL string) (*DedupResult, error) {
	if sourceURL == "" {
		return &DedupResult{IsDuplicate: false}, nil
	}

	var id, title string
	err := d.Pool.QueryRow(ctx,
		"SELECT id, title FROM artifacts WHERE source_url = $1 LIMIT 1",
		sourceURL,
	).Scan(&id, &title)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
