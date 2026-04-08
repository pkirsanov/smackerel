package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/smackerel/smackerel/internal/db"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// SearchRequest is the JSON body for POST /api/search.
type SearchRequest struct {
	Query   string        `json:"query"`
	Limit   int           `json:"limit,omitempty"`
	Filters SearchFilters `json:"filters,omitempty"`
}

// SearchFilters are optional filters for search queries.
type SearchFilters struct {
	Type     string `json:"type,omitempty"`
	DateFrom string `json:"date_from,omitempty"`
	DateTo   string `json:"date_to,omitempty"`
	Person   string `json:"person,omitempty"`
	Topic    string `json:"topic,omitempty"`
}

// SearchResponse is the response for POST /api/search.
type SearchResponse struct {
	Results         []SearchResult `json:"results"`
	TotalCandidates int            `json:"total_candidates"`
	SearchTimeMs    int64          `json:"search_time_ms"`
	Message         string         `json:"message,omitempty"`
}

// SearchResult is a single search result.
type SearchResult struct {
	ArtifactID   string   `json:"artifact_id"`
	Title        string   `json:"title"`
	ArtifactType string   `json:"artifact_type"`
	Summary      string   `json:"summary"`
	SourceURL    string   `json:"source_url,omitempty"`
	Relevance    string   `json:"relevance"`
	Explanation  string   `json:"explanation"`
	CreatedAt    string   `json:"created_at"`
	Topics       []string `json:"topics"`
	Connections  int      `json:"connections"`
}

// SearchEngine handles semantic search operations.
type SearchEngine struct {
	Pool *pgxpool.Pool
	NATS *smacknats.Client
}

// SearchHandler handles POST /api/search.
func (d *Dependencies) SearchHandler(w http.ResponseWriter, r *http.Request) {
	if !d.checkAuth(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authentication required")
		return
	}

	var req SearchRequest
	// Limit request body to 1MB to prevent memory exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Invalid JSON body")
		return
	}

	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "EMPTY_QUERY", "Query text is required")
		return
	}

	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 10
	}

	start := time.Now()

	// Get the search engine from dependencies
	engine, ok := d.SearchEngine.(*SearchEngine)
	if !ok || engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ML_UNAVAILABLE", "Search sidecar is not responding")
		return
	}

	results, totalCandidates, err := engine.Search(r.Context(), req)
	if err != nil {
		slog.Error("search failed", "error", err, "query", req.Query)
		writeError(w, http.StatusInternalServerError, "SEARCH_FAILED", "Search processing error")
		return
	}

	elapsed := time.Since(start).Milliseconds()

	resp := SearchResponse{
		Results:         results,
		TotalCandidates: totalCandidates,
		SearchTimeMs:    elapsed,
	}

	if len(results) == 0 {
		resp.Message = "I don't have anything about that yet"
	}

	writeJSON(w, http.StatusOK, resp)
}

// Search performs a semantic search: embed query → pgvector similarity → filters → graph expansion.
func (s *SearchEngine) Search(ctx context.Context, req SearchRequest) ([]SearchResult, int, error) {
	// Step 1: Create a unique inbox for this query to avoid shared-subject races
	replySubject := s.NATS.Conn.NewInbox()

	sub, err := s.NATS.Conn.SubscribeSync(replySubject)
	if err != nil {
		slog.Warn("embedding subscription failed, falling back to text search", "error", err)
		return s.textSearch(ctx, req)
	}
	defer sub.Unsubscribe()
	// Auto-unsubscribe after 1 message — this inbox is single-use
	if err := sub.AutoUnsubscribe(1); err != nil {
		slog.Warn("auto-unsubscribe failed", "error", err)
	}

	// Step 2: Publish embed request with reply subject so ML sidecar responds to our inbox
	queryID := fmt.Sprintf("q-%d", time.Now().UnixNano())
	embedPayload, _ := json.Marshal(map[string]string{
		"query_id":      queryID,
		"text":          req.Query,
		"reply_subject": replySubject,
	})

	if err := s.NATS.Publish(ctx, smacknats.SubjectSearchEmbed, embedPayload); err != nil {
		return nil, 0, fmt.Errorf("publish embed request: %w", err)
	}

	// Step 3: Wait for embedding response on the unique inbox (with timeout)
	embedding, err := s.waitForEmbeddingOnInbox(ctx, sub)
	if err != nil {
		// Fallback: text-based search if embedding fails
		slog.Warn("embedding failed, falling back to text search", "error", err)
		return s.textSearch(ctx, req)
	}

	// Step 3: Vector similarity search with pgvector
	results, total, err := s.vectorSearch(ctx, embedding, req)
	if err != nil {
		return nil, 0, fmt.Errorf("vector search: %w", err)
	}

	return results, total, nil
}

// waitForEmbeddingOnInbox waits for a single embedding response on a unique inbox subscription.
func (s *SearchEngine) waitForEmbeddingOnInbox(ctx context.Context, sub *nats.Subscription) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	msg, err := sub.NextMsgWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("wait for embedding: %w", err)
	}

	var resp struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response: %w", err)
	}
	return resp.Embedding, nil
}

// vectorSearch performs pgvector cosine similarity search.
func (s *SearchEngine) vectorSearch(ctx context.Context, embedding []float32, req SearchRequest) ([]SearchResult, int, error) {
	// Format embedding for pgvector
	embStr := db.FormatEmbedding(embedding)

	// Build query with optional filters
	query := `
		SELECT id, title, artifact_type, COALESCE(summary, ''), COALESCE(source_url, ''),
		       COALESCE(topics::text, '[]'), created_at,
		       1 - (embedding <=> $1::vector) AS similarity
		FROM artifacts
		WHERE embedding IS NOT NULL`

	args := []interface{}{embStr}
	argN := 2

	if req.Filters.Type != "" {
		query += fmt.Sprintf(" AND artifact_type = $%d", argN)
		args = append(args, req.Filters.Type)
		argN++
	}

	if req.Filters.DateFrom != "" {
		query += fmt.Sprintf(" AND created_at >= $%d::timestamptz", argN)
		args = append(args, req.Filters.DateFrom)
		argN++
	}

	if req.Filters.Person != "" {
		query += fmt.Sprintf(" AND entities->'people' ? $%d", argN)
		args = append(args, req.Filters.Person)
		argN++
	}

	if req.Filters.Topic != "" {
		query += fmt.Sprintf(" AND topics ? $%d", argN)
		args = append(args, req.Filters.Topic)
		argN++
	}

	query += " ORDER BY embedding <=> $1::vector LIMIT $" + fmt.Sprintf("%d", argN)
	args = append(args, req.Limit*3) // Fetch more for re-ranking

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("vector search query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var topicsStr string
		var similarity float64
		var createdAt time.Time

		if err := rows.Scan(&r.ArtifactID, &r.Title, &r.ArtifactType, &r.Summary,
			&r.SourceURL, &topicsStr, &createdAt, &similarity); err != nil {
			continue
		}

		r.CreatedAt = createdAt.Format(time.RFC3339)

		// Parse topics
		var topics []string
		_ = json.Unmarshal([]byte(topicsStr), &topics)
		r.Topics = topics

		// Set relevance based on similarity score
		switch {
		case similarity > 0.7:
			r.Relevance = "high"
		case similarity > 0.4:
			r.Relevance = "medium"
		default:
			r.Relevance = "low"
		}
		r.Explanation = fmt.Sprintf("Similarity: %.2f", similarity)

		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("vector search row iteration: %w", err)
	}

	// Batch-fetch connection counts for all results (N+1 fix)
	if len(results) > 0 {
		ids := make([]string, len(results))
		for i, r := range results {
			ids[i] = r.ArtifactID
		}
		connRows, err := s.Pool.Query(ctx, `
			SELECT id, COUNT(*) FROM (
				SELECT src_id AS id FROM edges WHERE src_type = 'artifact' AND src_id = ANY($1)
				UNION ALL
				SELECT dst_id AS id FROM edges WHERE dst_type = 'artifact' AND dst_id = ANY($1)
			) sub GROUP BY id
		`, ids)
		if err == nil {
			connMap := make(map[string]int)
			for connRows.Next() {
				var aid string
				var count int
				if connRows.Scan(&aid, &count) == nil {
					connMap[aid] = count
				}
			}
			if err := connRows.Err(); err != nil {
				slog.Warn("connection count row iteration error", "error", err)
			}
			connRows.Close()
			for i := range results {
				results[i].Connections = connMap[results[i].ArtifactID]
			}
		}
	}

	total := len(results)
	if len(results) > req.Limit {
		results = results[:req.Limit]
	}

	return results, total, nil
}

// textSearch is a fallback when embedding is unavailable — uses trigram text search.
func (s *SearchEngine) textSearch(ctx context.Context, req SearchRequest) ([]SearchResult, int, error) {
	// Escape ILIKE metacharacters in user query to prevent wildcard injection
	safeQuery := escapeLikePattern(req.Query)

	rows, err := s.Pool.Query(ctx, `
		SELECT id, title, artifact_type, COALESCE(summary, ''), COALESCE(source_url, ''),
		       COALESCE(topics::text, '[]'), created_at,
		       similarity(title, $1) AS sim
		FROM artifacts
		WHERE title % $1 OR summary ILIKE '%' || $2 || '%'
		ORDER BY sim DESC
		LIMIT $3
	`, req.Query, safeQuery, req.Limit)
	if err != nil {
		return nil, 0, fmt.Errorf("text search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var topicsStr string
		var sim float64
		var createdAt time.Time

		if err := rows.Scan(&r.ArtifactID, &r.Title, &r.ArtifactType, &r.Summary,
			&r.SourceURL, &topicsStr, &createdAt, &sim); err != nil {
			continue
		}

		r.CreatedAt = createdAt.Format(time.RFC3339)
		r.Relevance = "medium"
		r.Explanation = "Text match"

		var topics []string
		_ = json.Unmarshal([]byte(topicsStr), &topics)
		r.Topics = topics

		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("text search row iteration: %w", err)
	}

	return results, len(results), nil
}

// escapeLikePattern escapes ILIKE metacharacters (%, _) in user input
// to prevent wildcard injection in text search fallback queries.
func escapeLikePattern(s string) string {
	r := strings.NewReplacer(
		`%`, `\%`,
		`_`, `\_`,
		`\`, `\\`,
	)
	return r.Replace(s)
}
