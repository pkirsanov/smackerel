package tools

// semantic_searcher.go — spec 104 SCOPE-01.
//
// SemanticSearcher is the general, namespace-scoped embedding-cosine search
// over the artifacts table. It resolves the spec 064 SCOPE-06 text-similarity
// deferral (PgxGraphSearcher uses ILIKE) as a GENERAL capability:
//
//   - self_knowledge (SCOPE-04) consumes it scoped to source_id="smackerel_self";
//   - internal_retrieval can adopt it for the user namespace in a follow-on
//     (one embedding-backed path, no fork).
//
// The query is embedded via the injected Embedder (the ML sidecar in
// production; a deterministic fake in tests) and ranked by pgvector cosine
// distance (`embedding <=> $vec`). On embedder failure the searcher returns a
// typed error — it NEVER silently falls back to keyword search, so the agent
// surfaces the honest BUG-061-009 refusal rather than a lower-fidelity guess.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/db"
)

// Embedder embeds a query string into a vector for semantic search.
// Structurally satisfied by the ML-sidecar embedder wired in cmd/core;
// tests substitute a deterministic fake. Implementations MUST return
// vectors of a consistent dimension matching artifacts.embedding (384).
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// rowQuerier is the minimal pgx surface the searcher needs. *pgxpool.Pool
// satisfies it; tests substitute a stub to prove the validation/embed
// short-circuit paths never reach the database.
type rowQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// SemanticSearcher is the general namespace-scoped embedding search contract.
type SemanticSearcher interface {
	// Search returns up to k artifacts in the given source_id namespace,
	// ranked by cosine similarity of artifacts.embedding to the query's
	// embedding. Only artifacts with a non-NULL embedding participate.
	Search(ctx context.Context, namespace, query string, k int) ([]GraphArtifact, error)
}

// Typed sentinel errors. The tool layer maps these to honest refusals.
var (
	ErrSemanticSearchNamespace = errors.New("openknowledge: semantic search namespace must be non-empty")
	ErrSemanticSearchQuery     = errors.New("openknowledge: semantic search query must be non-empty after trim")
	ErrSemanticSearchK         = errors.New("openknowledge: semantic search k must be > 0 and <= MaxInternalRetrievalK")
	ErrSemanticSearchEmbed     = errors.New("openknowledge: semantic search query embedding failed")
	ErrSemanticSearchEmptyVec  = errors.New("openknowledge: semantic search query produced an empty embedding")
)

// PgxSemanticSearcher implements SemanticSearcher against live PostgreSQL +
// pgvector, embedding the query via the injected Embedder.
type PgxSemanticSearcher struct {
	pool     rowQuerier
	embedder Embedder
}

// NewPgxSemanticSearcher wraps a pgx pool + embedder. Both are required;
// the constructor panics on nil rather than allowing a silent no-op path
// (G028: no defaults). Pool ownership stays with the caller.
func NewPgxSemanticSearcher(pool rowQuerier, embedder Embedder) *PgxSemanticSearcher {
	if pool == nil {
		panic("openknowledge: PgxSemanticSearcher requires a non-nil pgx pool")
	}
	if embedder == nil {
		panic("openknowledge: PgxSemanticSearcher requires a non-nil Embedder")
	}
	return &PgxSemanticSearcher{pool: pool, embedder: embedder}
}

// Search validates inputs, embeds the query, and runs a namespace-scoped
// pgvector cosine search. The namespace and query vector are parameterised;
// the only non-parameter SQL is the LIMIT, bounded by MaxInternalRetrievalK.
func (s *PgxSemanticSearcher) Search(ctx context.Context, namespace, query string, k int) ([]GraphArtifact, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, ErrSemanticSearchNamespace
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrSemanticSearchQuery
	}
	if k <= 0 || k > MaxInternalRetrievalK {
		return nil, ErrSemanticSearchK
	}

	vec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSemanticSearchEmbed, err)
	}
	if len(vec) == 0 {
		return nil, ErrSemanticSearchEmptyVec
	}
	embStr := db.FormatEmbedding(vec)

	rows, err := s.pool.Query(ctx, `
		SELECT id, title, COALESCE(summary, '')
		FROM artifacts
		WHERE source_id = $1 AND embedding IS NOT NULL
		ORDER BY embedding <=> $2::vector
		LIMIT $3
	`, namespace, embStr, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]GraphArtifact, 0, k)
	for rows.Next() {
		var a GraphArtifact
		if err := rows.Scan(&a.ID, &a.Title, &a.Summary); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
