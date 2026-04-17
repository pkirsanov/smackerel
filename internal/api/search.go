package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/smackerel/smackerel/internal/db"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/stringutil"
)

// SearchRequest is the JSON body for POST /api/search.
type SearchRequest struct {
	Query   string        `json:"query"`
	Limit   int           `json:"limit,omitempty"`
	Filters SearchFilters `json:"filters,omitempty"`
}

// SearchFilters are optional filters for search queries.
type SearchFilters struct {
	Type       string `json:"type,omitempty"`
	DateFrom   string `json:"date_from,omitempty"`
	DateTo     string `json:"date_to,omitempty"`
	Person     string `json:"person,omitempty"`
	Topic      string `json:"topic,omitempty"`
	Domain     string `json:"domain,omitempty"`
	Ingredient string `json:"ingredient,omitempty"`
}

// SearchResponse is the response for POST /api/search.
type SearchResponse struct {
	Results         []SearchResult        `json:"results"`
	TotalCandidates int                   `json:"total_candidates"`
	SearchTimeMs    int64                 `json:"search_time_ms"`
	SearchMode      string                `json:"search_mode"`
	Message         string                `json:"message,omitempty"`
	KnowledgeMatch  *ConceptMatchResponse `json:"knowledge_match,omitempty"`
}

// ConceptMatchResponse is the knowledge layer match in a search response.
type ConceptMatchResponse struct {
	ConceptID     string   `json:"concept_id"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	CitationCount int      `json:"citation_count"`
	SourceTypes   []string `json:"source_types"`
	UpdatedAt     string   `json:"updated_at"`
	MatchScore    float64  `json:"match_score"`
}

// SearchResult is a single search result.
type SearchResult struct {
	ArtifactID   string          `json:"artifact_id"`
	Title        string          `json:"title"`
	ArtifactType string          `json:"artifact_type"`
	Summary      string          `json:"summary"`
	SourceURL    string          `json:"source_url,omitempty"`
	Relevance    string          `json:"relevance"`
	Explanation  string          `json:"explanation"`
	CreatedAt    string          `json:"created_at"`
	Topics       []string        `json:"topics"`
	Connections  int             `json:"connections"`
	DomainData   json.RawMessage `json:"domain_data,omitempty"`
}

// SearchEngine handles semantic search operations.
type SearchEngine struct {
	Pool           *pgxpool.Pool
	NATS           *smacknats.Client
	MLSidecarURL   string
	HealthCacheTTL time.Duration

	mlHealthy  atomic.Bool
	mlHealthAt atomic.Int64 // unix nanos of last health check

	// healthClient is a dedicated HTTP client for ML sidecar health probes.
	// Short timeouts prevent health checks from blocking search requests.
	healthClient     *http.Client
	healthClientOnce sync.Once

	// healthProbeMu coalesces concurrent TTL-expired health probes
	// to prevent thundering-herd against a recovering ML sidecar.
	healthProbeMu sync.Mutex
}

// maxQueryLen limits search query length to prevent abuse.
const maxQueryLen = 10000

// SearchHandler handles POST /api/search.
func (d *Dependencies) SearchHandler(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if !decodeJSONBody(w, r, &req, "INVALID_INPUT", "Invalid JSON body") {
		return
	}

	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "EMPTY_QUERY", "Query text is required")
		return
	}

	if len(req.Query) > maxQueryLen {
		writeError(w, http.StatusBadRequest, "QUERY_TOO_LONG", "Query exceeds maximum length")
		return
	}

	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 10
	}

	start := time.Now()

	// Step 0: Knowledge-layer-first — check for pre-synthesized concept page match
	var knowledgeMatch *ConceptMatchResponse
	if d.KnowledgeStore != nil && d.KnowledgeConceptSearchThreshold > 0 {
		match, err := d.KnowledgeStore.SearchConcepts(r.Context(), req.Query, d.KnowledgeConceptSearchThreshold)
		if err != nil {
			slog.Warn("knowledge concept search failed", "error", err, "query", req.Query)
		} else if match != nil {
			knowledgeMatch = &ConceptMatchResponse{
				ConceptID:     match.ConceptID,
				Title:         match.Title,
				Summary:       match.Summary,
				CitationCount: match.CitationCount,
				SourceTypes:   match.SourceTypes,
				UpdatedAt:     match.UpdatedAt.Format(time.RFC3339),
				MatchScore:    match.MatchScore,
			}
		}
	}

	// Get the search engine from dependencies
	if d.SearchEngine == nil {
		writeError(w, http.StatusServiceUnavailable, "ML_UNAVAILABLE", "Search sidecar is not responding")
		return
	}

	results, totalCandidates, searchMode, err := d.SearchEngine.Search(r.Context(), req)
	if err != nil {
		slog.Error("search failed", "error", err, "query", req.Query)
		writeError(w, http.StatusInternalServerError, "SEARCH_FAILED", "Search processing error")
		return
	}

	// Override search_mode when knowledge match is present
	if knowledgeMatch != nil {
		searchMode = "knowledge_first"
	}

	elapsed := time.Since(start).Milliseconds()

	resp := SearchResponse{
		Results:         results,
		TotalCandidates: totalCandidates,
		SearchTimeMs:    elapsed,
		SearchMode:      searchMode,
		KnowledgeMatch:  knowledgeMatch,
	}

	if len(results) == 0 && knowledgeMatch == nil {
		resp.Message = "I don't have anything about that yet"
	}

	// Write the response immediately so search latency is not affected by logging.
	writeJSON(w, http.StatusOK, resp)

	// Log search for frequency tracking in a goroutine (truly non-blocking).
	// Uses a detached context so the log insert completes even if the client disconnects.
	if d.IntelligenceEngine != nil {
		topResultID := ""
		if len(results) > 0 {
			topResultID = results[0].ArtifactID
		}
		engine := d.IntelligenceEngine
		queryStr := req.Query
		resultCount := len(results)
		go func() {
			logCtx, logCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer logCancel()
			if err := engine.LogSearch(logCtx, queryStr, resultCount, topResultID); err != nil {
				slog.Warn("search logging failed", "error", err, "query", queryStr)
			}
		}()
	}
}

// Search performs a semantic search: embed query → pgvector similarity → filters → graph expansion.
func (s *SearchEngine) Search(ctx context.Context, req SearchRequest) ([]SearchResult, int, string, error) {
	// Step 0: Parse temporal intent from query (e.g., "from last week")
	if temporal := parseTemporalIntent(req.Query); temporal != nil {
		if req.Filters.DateFrom == "" {
			req.Filters.DateFrom = temporal.DateFrom
		}
		if req.Filters.DateTo == "" {
			req.Filters.DateTo = temporal.DateTo
		}
		if temporal.Cleaned != "" {
			req.Query = temporal.Cleaned
		}
		slog.Info("temporal intent parsed",
			"original_query", req.Query,
			"date_from", req.Filters.DateFrom,
			"date_to", req.Filters.DateTo,
		)
	}

	// Step 0.5: Parse domain intent (e.g., "recipes with chicken", "cameras under $500")
	if intent := parseDomainIntent(req.Query); intent != nil {
		if req.Filters.Domain == "" {
			req.Filters.Domain = intent.Domain
		}
		if len(intent.Attributes) > 0 && req.Filters.Ingredient == "" {
			req.Filters.Ingredient = intent.Attributes[0]
		}
		if intent.Cleaned != "" {
			req.Query = intent.Cleaned
		}
		slog.Info("domain intent parsed",
			"domain", intent.Domain,
			"attributes", intent.Attributes,
			"price_max", intent.PriceMax,
		)
	}

	// Temporal-only query — use time-range-filtered recency query, skip embedding
	if req.Query == "" {
		results, total, err := s.timeRangeSearch(ctx, req)
		return results, total, "time_range", err
	}

	// Check ML sidecar health cache — skip NATS embed if sidecar is down
	if !s.isMLHealthy(ctx) {
		slog.Info("ML sidecar unhealthy, using text fallback", "query", req.Query)
		results, total, err := s.textSearch(ctx, req)
		return results, total, "text_fallback", err
	}

	// Step 1: Create a unique inbox for this query to avoid shared-subject races
	replySubject := s.NATS.Conn.NewInbox()

	sub, err := s.NATS.Conn.SubscribeSync(replySubject)
	if err != nil {
		slog.Warn("embedding subscription failed, falling back to text search", "error", err)
		results, total, err := s.textSearch(ctx, req)
		return results, total, "text_fallback", err
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
		return nil, 0, "", fmt.Errorf("publish embed request: %w", err)
	}

	// Step 3: Wait for embedding response on the unique inbox (with timeout)
	embedding, err := s.waitForEmbeddingOnInbox(ctx, sub)
	if err != nil {
		// Fallback: text-based search if embedding fails
		slog.Warn("embedding failed, falling back to text search", "error", err)
		results, total, err := s.textSearch(ctx, req)
		return results, total, "text_fallback", err
	}

	// Step 4: Vector similarity search with pgvector
	results, total, err := s.vectorSearch(ctx, embedding, req)
	if err != nil {
		return nil, 0, "", fmt.Errorf("vector search: %w", err)
	}

	// Step 5: Graph expansion — find related artifacts via knowledge graph edges
	if len(results) > 0 && len(results) < req.Limit {
		expanded := s.graphExpand(ctx, results, req.Limit-len(results))
		if len(expanded) > 0 {
			results = append(results, expanded...)
			total += len(expanded)
		}
	}

	// Step 6: LLM re-ranking via ML sidecar (best-effort, skip on failure)
	if len(results) > 1 {
		reranked, err := s.rerankViaML(ctx, req.Query, results)
		if err != nil {
			slog.Warn("LLM re-ranking failed, using similarity order", "error", err)
		} else {
			results = reranked
		}
	}

	return results, total, "semantic", nil
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

// isMLHealthy returns cached ML sidecar health status.
// Refreshes the cache when TTL has expired via a quick HTTP health check.
// Concurrent probe requests are coalesced to prevent thundering-herd.
func (s *SearchEngine) isMLHealthy(ctx context.Context) bool {
	if s.MLSidecarURL == "" {
		return false
	}

	// A zero TTL means SST config was not wired — treat as unhealthy (fail-visible,
	// triggers text fallback) rather than using a hidden default.
	if s.HealthCacheTTL == 0 {
		slog.Warn("ML health cache TTL is zero — SST config ML_HEALTH_CACHE_TTL_S may be missing")
		return false
	}

	now := time.Now().UnixNano()
	lastCheck := s.mlHealthAt.Load()

	if now-lastCheck < int64(s.HealthCacheTTL) {
		return s.mlHealthy.Load()
	}

	// TTL expired — coalesce concurrent probes via mutex.
	// If another goroutine is already probing, use the stale cached value.
	if !s.healthProbeMu.TryLock() {
		return s.mlHealthy.Load()
	}
	defer s.healthProbeMu.Unlock()

	// Re-check after acquiring lock — another goroutine may have refreshed.
	if time.Now().UnixNano()-s.mlHealthAt.Load() < int64(s.HealthCacheTTL) {
		return s.mlHealthy.Load()
	}

	healthy := s.probeMLHealth(ctx)
	s.mlHealthy.Store(healthy)
	s.mlHealthAt.Store(time.Now().UnixNano())
	return healthy
}

// probeMLHealth performs a quick HTTP GET to the ML sidecar health endpoint.
// Uses a detached context (not the caller's request context) because the result
// is cached and shared across all search requests for the TTL period. A cancelled
// request must not taint the shared cache with a false-unhealthy result.
func (s *SearchEngine) probeMLHealth(_ context.Context) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.MLSidecarURL+"/health", nil)
	if err != nil {
		return false
	}

	client := s.getHealthClient()
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// getHealthClient returns the dedicated health-check HTTP client, creating it lazily.
func (s *SearchEngine) getHealthClient() *http.Client {
	s.healthClientOnce.Do(func() {
		s.healthClient = &http.Client{
			Timeout: 2 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        2,
				MaxIdleConnsPerHost: 1,
				IdleConnTimeout:     30 * time.Second,
			},
		}
	})
	return s.healthClient
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

	args := []any{embStr}
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

	if req.Filters.DateTo != "" {
		query += fmt.Sprintf(" AND created_at <= $%d::timestamptz", argN)
		args = append(args, req.Filters.DateTo)
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

	if req.Filters.Domain != "" {
		query += fmt.Sprintf(" AND domain_data->>'domain' = $%d", argN)
		args = append(args, req.Filters.Domain)
		argN++
	}

	if req.Filters.Ingredient != "" {
		query += fmt.Sprintf(` AND domain_data @> jsonb_build_object('ingredients', jsonb_build_array(jsonb_build_object('name', $%d)))`, argN)
		args = append(args, req.Filters.Ingredient)
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
		if err := json.Unmarshal([]byte(topicsStr), &topics); err != nil {
			slog.Debug("failed to unmarshal artifact topics", "artifact_id", r.ArtifactID, "error", err)
		}
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

// rerankViaML sends search candidates to the ML sidecar for LLM-based re-ranking.
// Uses NATS request-reply with a 3-second timeout. Falls back gracefully.
func (s *SearchEngine) rerankViaML(ctx context.Context, query string, candidates []SearchResult) ([]SearchResult, error) {
	// Build candidate summaries for the LLM
	type candidate struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Type    string `json:"type"`
	}
	var cands []candidate
	for _, r := range candidates {
		cands = append(cands, candidate{
			ID:      r.ArtifactID,
			Title:   r.Title,
			Summary: r.Summary,
			Type:    r.ArtifactType,
		})
	}

	// Create unique reply inbox
	replySubject := s.NATS.Conn.NewInbox()
	sub, err := s.NATS.Conn.SubscribeSync(replySubject)
	if err != nil {
		return nil, fmt.Errorf("subscribe for rerank reply: %w", err)
	}
	defer sub.Unsubscribe()
	sub.AutoUnsubscribe(1)

	payload, _ := json.Marshal(map[string]any{
		"query":         query,
		"candidates":    cands,
		"reply_subject": replySubject,
	})

	if err := s.NATS.Publish(ctx, smacknats.SubjectSearchRerank, payload); err != nil {
		return nil, fmt.Errorf("publish rerank request: %w", err)
	}

	// Wait for response with timeout
	rerankCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	msg, err := sub.NextMsgWithContext(rerankCtx)
	if err != nil {
		return nil, fmt.Errorf("wait for rerank response: %w", err)
	}

	var resp struct {
		RankedIDs []string `json:"ranked_ids"`
	}
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal rerank response: %w", err)
	}

	if len(resp.RankedIDs) == 0 {
		return candidates, nil // No re-ranking available
	}

	// Reorder results by the ranked ID order
	resultMap := make(map[string]SearchResult)
	for _, r := range candidates {
		resultMap[r.ArtifactID] = r
	}

	var reranked []SearchResult
	for i, id := range resp.RankedIDs {
		if r, ok := resultMap[id]; ok {
			r.Relevance = "reranked"
			r.Explanation = fmt.Sprintf("LLM rank: #%d", i+1)
			reranked = append(reranked, r)
			delete(resultMap, id)
		}
	}
	// Append any results not covered by re-ranking
	for _, r := range candidates {
		if _, remaining := resultMap[r.ArtifactID]; remaining {
			reranked = append(reranked, r)
		}
	}

	return reranked, nil
}

// graphExpand finds related artifacts via knowledge graph edges from the primary results.
// This enriches search results by discovering connections that vector similarity alone might miss.
func (s *SearchEngine) graphExpand(ctx context.Context, primaryResults []SearchResult, maxExpansion int) []SearchResult {
	if maxExpansion <= 0 || len(primaryResults) == 0 {
		return nil
	}

	// Collect primary artifact IDs to exclude from expansion
	primaryIDs := make(map[string]bool)
	var ids []string
	for _, r := range primaryResults {
		primaryIDs[r.ArtifactID] = true
		ids = append(ids, r.ArtifactID)
	}

	// Find connected artifacts via edges (both directions)
	rows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT a.id, a.title, a.artifact_type, COALESCE(a.summary, ''),
		       COALESCE(a.source_url, ''), a.created_at, e.edge_type, e.weight
		FROM edges e
		JOIN artifacts a ON (
			(e.dst_type = 'artifact' AND e.dst_id = a.id) OR
			(e.src_type = 'artifact' AND e.src_id = a.id)
		)
		WHERE (
			(e.src_type = 'artifact' AND e.src_id = ANY($1)) OR
			(e.dst_type = 'artifact' AND e.dst_id = ANY($1))
		)
		AND a.id != ALL($1)
		AND a.processing_status = 'processed'
		AND e.weight >= 0.3
		ORDER BY e.weight DESC
		LIMIT $2
	`, ids, maxExpansion)
	if err != nil {
		slog.Warn("graph expansion query failed", "error", err)
		return nil
	}
	defer rows.Close()

	var expanded []SearchResult
	seen := make(map[string]bool)
	for rows.Next() {
		var r SearchResult
		var createdAt time.Time
		var edgeType string
		var weight float64

		if err := rows.Scan(&r.ArtifactID, &r.Title, &r.ArtifactType, &r.Summary,
			&r.SourceURL, &createdAt, &edgeType, &weight); err != nil {
			continue
		}

		if primaryIDs[r.ArtifactID] || seen[r.ArtifactID] {
			continue
		}
		seen[r.ArtifactID] = true

		r.CreatedAt = createdAt.Format(time.RFC3339)
		r.Relevance = "graph"
		r.Explanation = fmt.Sprintf("Connected via %s (weight: %.2f)", edgeType, weight)

		expanded = append(expanded, r)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("graph expansion iteration error", "error", err)
	}

	return expanded
}

// textSearch is a fallback when embedding is unavailable — uses trigram text search.
func (s *SearchEngine) textSearch(ctx context.Context, req SearchRequest) ([]SearchResult, int, error) {
	// Escape ILIKE metacharacters in user query to prevent wildcard injection
	safeQuery := stringutil.EscapeLikePattern(req.Query)

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
		if err := json.Unmarshal([]byte(topicsStr), &topics); err != nil {
			slog.Debug("failed to unmarshal artifact topics", "artifact_id", r.ArtifactID, "error", err)
		}
		r.Topics = topics

		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("text search row iteration: %w", err)
	}

	return results, len(results), nil
}

// timeRangeSearch returns artifacts filtered by DateFrom/DateTo, ordered by created_at DESC.
// Used when a temporal phrase consumed the entire query (e.g., "yesterday", "last week").
func (s *SearchEngine) timeRangeSearch(ctx context.Context, req SearchRequest) ([]SearchResult, int, error) {
	query := `
		SELECT id, title, artifact_type, COALESCE(summary, ''), COALESCE(source_url, ''),
		       COALESCE(topics::text, '[]'), created_at
		FROM artifacts
		WHERE 1=1`

	args := []any{}
	argN := 1

	if req.Filters.DateFrom != "" {
		query += fmt.Sprintf(" AND created_at >= $%d::timestamptz", argN)
		args = append(args, req.Filters.DateFrom)
		argN++
	}

	if req.Filters.DateTo != "" {
		query += fmt.Sprintf(" AND created_at <= $%d::timestamptz", argN)
		args = append(args, req.Filters.DateTo)
		argN++
	}

	if req.Filters.Type != "" {
		query += fmt.Sprintf(" AND artifact_type = $%d", argN)
		args = append(args, req.Filters.Type)
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

	query += " ORDER BY created_at DESC LIMIT $" + fmt.Sprintf("%d", argN)
	args = append(args, req.Limit)

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("time range search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var topicsStr string
		var createdAt time.Time

		if err := rows.Scan(&r.ArtifactID, &r.Title, &r.ArtifactType, &r.Summary,
			&r.SourceURL, &topicsStr, &createdAt); err != nil {
			continue
		}

		r.CreatedAt = createdAt.Format(time.RFC3339)
		r.Relevance = "recent"
		r.Explanation = "Time-range match"

		var topics []string
		if err := json.Unmarshal([]byte(topicsStr), &topics); err != nil {
			slog.Debug("failed to unmarshal artifact topics", "artifact_id", r.ArtifactID, "error", err)
		}
		r.Topics = topics

		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("time range search iteration: %w", err)
	}

	return results, len(results), nil
}
