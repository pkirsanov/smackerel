package bookmarks

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/connector"
)

// trackingParams lists URL query parameters to strip during normalization.
var trackingParams = map[string]bool{
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	"fbclid":       true,
	"gclid":        true,
	"ref":          true,
}

// NormalizeURL lowercases scheme+host, strips trailing slash, and removes tracking params.
// Path casing is preserved. Invalid URLs are returned as-is.
func NormalizeURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return rawURL
	}

	// Lowercase scheme and host only
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)

	// F-CHAOS-R24-002: Strip userinfo to prevent credential leaks into SourceRef.
	u.User = nil

	// IMP-009-R-002: Strip www. prefix for consistent dedup across www/non-www variants.
	if strings.HasPrefix(u.Host, "www.") {
		u.Host = u.Host[4:]
	}

	// Strip trailing slash from path (unless path is just "/")
	if len(u.Path) > 1 && strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimRight(u.Path, "/")
	}

	// Remove tracking parameters
	q := u.Query()
	changed := false
	for param := range trackingParams {
		if q.Has(param) {
			q.Del(param)
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}

	// Remove fragment
	u.Fragment = ""

	return u.String()
}

// URLDeduplicator checks URLs against existing artifacts to avoid reprocessing.
type URLDeduplicator struct {
	pool *pgxpool.Pool
}

// NewURLDeduplicator creates a new URL deduplicator.
func NewURLDeduplicator(pool *pgxpool.Pool) *URLDeduplicator {
	return &URLDeduplicator{pool: pool}
}

// FilterNew returns only artifacts whose normalized URL is not already in the database.
// The second return value is the count of duplicates skipped.
func (d *URLDeduplicator) FilterNew(ctx context.Context, artifacts []connector.RawArtifact) ([]connector.RawArtifact, int, error) {
	if d.pool == nil || len(artifacts) == 0 {
		return artifacts, 0, nil
	}

	// Collect normalized URLs
	normalized := make([]string, len(artifacts))
	for i, a := range artifacts {
		normalized[i] = NormalizeURL(a.URL)
	}

	// Batch check against existing artifacts
	rows, err := d.pool.Query(ctx, `
		SELECT source_ref FROM artifacts
		WHERE source_id = 'bookmarks' AND source_ref = ANY($1)
	`, normalized)
	if err != nil {
		return nil, 0, fmt.Errorf("dedup query: %w", err)
	}
	defer rows.Close()

	known := make(map[string]bool)
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			continue
		}
		known[ref] = true
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("dedup rows: %w", err)
	}

	// Filter to new-only
	var result []connector.RawArtifact
	dupes := 0
	for i, a := range artifacts {
		if known[normalized[i]] {
			dupes++
			continue
		}
		// Update the artifact's SourceRef to the normalized URL for consistent storage
		a.SourceRef = normalized[i]
		result = append(result, a)
	}

	if dupes > 0 {
		slog.Info("bookmark dedup filtered duplicates",
			"total", len(artifacts),
			"new", len(result),
			"duplicates", dupes,
		)
	}

	return result, dupes, nil
}

// IsKnown checks if a single normalized URL already exists as an artifact.
func (d *URLDeduplicator) IsKnown(ctx context.Context, normalizedURL string) (bool, error) {
	if d.pool == nil {
		return false, nil
	}

	var exists bool
	err := d.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM artifacts
			WHERE source_id = 'bookmarks' AND source_ref = $1
		)
	`, normalizedURL).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("dedup check: %w", err)
	}
	return exists, nil
}
