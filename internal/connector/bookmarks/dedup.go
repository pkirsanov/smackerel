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
// Path casing is preserved. Invalid URLs are returned as-is (after control-char stripping).
func NormalizeURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}

	// F-CHAOS-R30-001: Strip ASCII control characters (0x00-0x1F, 0x7F) before parsing.
	// Embedded NUL/CR/LF/TAB in URLs would otherwise survive url.Parse and produce:
	//   - log injection via slog when the URL is included in structured log fields
	//   - PostgreSQL INSERT failure on NUL bytes (text columns reject 0x00),
	//     blocking the artifact row and short-circuiting all dedup checks for the file
	//   - dedup miss when an attacker introduces \n/\r/\t variants of the same URL
	rawURL = stripURLControlChars(rawURL)

	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return rawURL
	}

	// Lowercase scheme and host only
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)

	// F-CHAOS-R24-002: Strip userinfo to prevent credential leaks into SourceRef.
	u.User = nil

	// F-CHAOS-R30-002/003: Canonicalise host — strip trailing DNS-root dot,
	// strip www. prefix, and elide default ports (:80 for http, :443 for https,
	// :21 for ftp) so equivalent URLs hash to one SourceRef instead of several.
	hostname := strings.TrimRight(u.Hostname(), ".")
	// IMP-009-R-002: Strip www. prefix for consistent dedup across www/non-www variants.
	if strings.HasPrefix(hostname, "www.") {
		hostname = hostname[4:]
	}
	port := u.Port()
	if port != "" {
		defaultPorts := map[string]string{
			"http":  "80",
			"https": "443",
			"ftp":   "21",
		}
		if def, ok := defaultPorts[u.Scheme]; ok && def == port {
			port = ""
		}
	}
	// Re-add brackets for IPv6 literals when reassembling.
	if strings.Contains(hostname, ":") {
		hostname = "[" + hostname + "]"
	}
	if port != "" {
		u.Host = hostname + ":" + port
	} else {
		u.Host = hostname
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

// stripURLControlChars removes ASCII control characters (0x00-0x1F and 0x7F)
// from a URL string. Used by NormalizeURL (F-CHAOS-R30-001) to prevent
// embedded NUL/CR/LF/TAB bytes from surviving into SourceRef.
func stripURLControlChars(s string) string {
	hasControl := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == 0x7F {
			hasControl = true
			break
		}
	}
	if !hasControl {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == 0x7F {
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
