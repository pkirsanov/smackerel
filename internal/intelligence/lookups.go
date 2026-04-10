package intelligence

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// SearchLogEntry records a single search query for frequency tracking.
type SearchLogEntry struct {
	ID           string    `json:"id"`
	Query        string    `json:"query"`
	QueryHash    string    `json:"query_hash"`
	ResultsCount int       `json:"results_count"`
	TopResultID  string    `json:"top_result_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// FrequentLookup represents a query that was repeated 3+ times in 30 days.
type FrequentLookup struct {
	QueryHash    string `json:"query_hash"`
	SampleQuery  string `json:"sample_query"`
	LookupCount  int    `json:"lookup_count"`
	HasReference bool   `json:"has_reference"`
}

// QuickReference is a compiled reference from frequently looked-up content.
type QuickReference struct {
	ID                string    `json:"id"`
	Concept           string    `json:"concept"`
	Content           string    `json:"content"`
	SourceArtifactIDs []string  `json:"source_artifact_ids"`
	LookupCount       int       `json:"lookup_count"`
	Pinned            bool      `json:"pinned"`
	CreatedAt         time.Time `json:"created_at"`
}

// LogSearch records a search query for frequency tracking per R-507.
func (e *Engine) LogSearch(ctx context.Context, query string, resultsCount int, topResultID string) error {
	if e.Pool == nil {
		return fmt.Errorf("search logging requires a database connection")
	}

	normalizedQuery := normalizeQuery(query)
	queryHash := hashQuery(normalizedQuery)

	_, err := e.Pool.Exec(ctx, `
		INSERT INTO search_log (id, query, query_hash, results_count, top_result_id, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`, ulid.Make().String(), query, queryHash, resultsCount, topResultID)
	return err
}

// DetectFrequentLookups finds queries repeated 3+ times in 30 days per R-507.
func (e *Engine) DetectFrequentLookups(ctx context.Context) ([]FrequentLookup, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("lookup detection requires a database connection")
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT sl.query_hash, MIN(sl.query) AS sample_query, COUNT(*) AS freq,
		       EXISTS(SELECT 1 FROM quick_references qr WHERE qr.concept = MIN(sl.query)) AS has_ref
		FROM search_log sl
		WHERE sl.created_at > NOW() - INTERVAL '30 days'
		GROUP BY sl.query_hash
		HAVING COUNT(*) >= 3
		ORDER BY freq DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query frequent lookups: %w", err)
	}
	defer rows.Close()

	var lookups []FrequentLookup
	for rows.Next() {
		var fl FrequentLookup
		if err := rows.Scan(&fl.QueryHash, &fl.SampleQuery, &fl.LookupCount, &fl.HasReference); err != nil {
			slog.Warn("frequent lookup scan failed", "error", err)
			continue
		}
		lookups = append(lookups, fl)
	}
	return lookups, rows.Err()
}

// CreateQuickReference creates a pinned quick reference from frequently looked-up content.
func (e *Engine) CreateQuickReference(ctx context.Context, concept, content string, sourceIDs []string) (*QuickReference, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("quick reference creation requires a database connection")
	}

	qr := &QuickReference{
		ID:                ulid.Make().String(),
		Concept:           concept,
		Content:           content,
		SourceArtifactIDs: sourceIDs,
		Pinned:            true,
		CreatedAt:         time.Now(),
	}

	_, err := e.Pool.Exec(ctx, `
		INSERT INTO quick_references (id, concept, content, source_artifact_ids, lookup_count, pinned, created_at)
		VALUES ($1, $2, $3, $4::jsonb, 0, $5, $6)
	`, qr.ID, qr.Concept, qr.Content, fmt.Sprintf(`["%s"]`, strings.Join(sourceIDs, `","`)),
		qr.Pinned, qr.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert quick reference: %w", err)
	}
	return qr, nil
}

// GetQuickReferences returns all pinned quick references.
func (e *Engine) GetQuickReferences(ctx context.Context) ([]QuickReference, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("quick references query requires a database connection")
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT id, concept, content, lookup_count, pinned, created_at
		FROM quick_references
		WHERE pinned = TRUE
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query quick references: %w", err)
	}
	defer rows.Close()

	var refs []QuickReference
	for rows.Next() {
		var qr QuickReference
		if err := rows.Scan(&qr.ID, &qr.Concept, &qr.Content, &qr.LookupCount, &qr.Pinned, &qr.CreatedAt); err != nil {
			slog.Warn("quick reference scan failed", "error", err)
			continue
		}
		refs = append(refs, qr)
	}
	return refs, rows.Err()
}

// normalizeQuery prepares a query for hashing: lowercase, trim, collapse spaces.
func normalizeQuery(q string) string {
	return strings.Join(strings.Fields(strings.ToLower(q)), " ")
}

// hashQuery produces a consistent SHA-256 hash for a normalized query.
func hashQuery(normalized string) string {
	h := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", h[:16]) // 128-bit prefix is sufficient
}
