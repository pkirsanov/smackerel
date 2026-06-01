package tools

// internal_retrieval wraps the existing knowledge-graph / pgvector store
// behind the openknowledge.Tool contract. The tool does NOT implement
// search itself: it delegates to a GraphSearcher dependency injected by
// the registry wiring. Unit tests substitute a mock; integration tests
// drive a real pgx-backed adapter against the ephemeral test compose.
//
// SCOPE-06 ships the tool surface and a text-similarity adapter
// (PgxGraphSearcher) sufficient to prove the contract end-to-end against
// live PostgreSQL. Wiring an embedding-backed adapter that re-uses the
// SearchEngine NATS/ML pipeline is deferred — see the SCOPE-06 finding
// in specs/064-open-ended-knowledge-agent/scopes.md.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

// MaxInternalRetrievalK is the documented upper bound on the number of
// snippets the tool will return in a single Execute call. It is a
// package-level constant rather than a tunable so the planner cannot
// inflate retrieval and bypass the agent loop's per-step budget. Raising
// the cap requires a spec change, not a config flip (G028: no defaults).
const MaxInternalRetrievalK = 25

// Typed sentinel errors returned via ToolResult.Error.
var (
	ErrInternalRetrievalMalformed = &ok.ToolError{Code: "malformed_params", Message: "params do not match schema"}
	ErrInternalRetrievalK         = &ok.ToolError{Code: "invalid_k", Message: "k must be > 0 and <= MaxInternalRetrievalK"}
	ErrInternalRetrievalQuery     = &ok.ToolError{Code: "invalid_query", Message: "query must be non-empty after trim"}
	ErrInternalRetrievalBackend   = &ok.ToolError{Code: "backend_failure", Message: "graph searcher returned an error"}
	ErrInvalidArtifact            = &ok.ToolError{Code: "invalid_artifact", Message: "graph searcher returned an artifact with empty id"}
)

// GraphArtifact is the minimum projection the tool needs from the graph
// store. Adapters lift artifacts table rows into this shape; the tool
// itself never touches the SQL layer directly.
type GraphArtifact struct {
	ID      string
	Title   string
	Summary string
}

// GraphSearcher is the dependency boundary. Real implementations hit
// pgvector or the SearchEngine; tests substitute mocks. Implementations
// MUST honour the k bound supplied by the caller (the tool already
// rejects out-of-range k upstream) and MUST return artifacts with
// non-empty IDs.
type GraphSearcher interface {
	Search(ctx context.Context, query string, k int) ([]GraphArtifact, error)
}

const internalRetrievalSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["query", "k"],
  "properties": {
    "query": {"type": "string", "minLength": 1},
    "k": {"type": "integer", "minimum": 1, "maximum": 25}
  }
}`

type internalRetrievalParams struct {
	Query *string `json:"query"`
	K     *int    `json:"k"`
}

// InternalRetrieval is the registry-facing handle.
type InternalRetrieval struct {
	searcher GraphSearcher
}

// NewInternalRetrieval constructs the tool around the given searcher.
// The constructor panics if searcher is nil; the registry MUST inject a
// real backend at boot rather than allowing a silent no-op fallback.
func NewInternalRetrieval(searcher GraphSearcher) *InternalRetrieval {
	if searcher == nil {
		panic("openknowledge: internal_retrieval requires a non-nil GraphSearcher")
	}
	return &InternalRetrieval{searcher: searcher}
}

// Name reports the registry key.
func (InternalRetrieval) Name() string { return "internal_retrieval" }

// Description summarises the tool for the planner prompt.
func (InternalRetrieval) Description() string {
	return "Retrieve up to k grounding snippets from the user's own knowledge graph for a free-text query. Returns Source entries with Kind=SourceArtifact."
}

// ParamsSchema returns the JSONSchema for Execute params.
func (InternalRetrieval) ParamsSchema() json.RawMessage {
	return json.RawMessage(internalRetrievalSchema)
}

// Execute validates params, calls the underlying graph searcher, and
// maps results to the canonical ToolResult envelope.
func (t *InternalRetrieval) Execute(ctx context.Context, params json.RawMessage) (*ok.ToolResult, error) {
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p internalRetrievalParams
	if err := dec.Decode(&p); err != nil {
		return &ok.ToolResult{Error: ErrInternalRetrievalMalformed}, nil
	}
	if p.Query == nil || p.K == nil {
		return &ok.ToolResult{Error: ErrInternalRetrievalMalformed}, nil
	}
	query := strings.TrimSpace(*p.Query)
	if query == "" {
		return &ok.ToolResult{Error: ErrInternalRetrievalQuery}, nil
	}
	k := *p.K
	if k <= 0 || k > MaxInternalRetrievalK {
		return &ok.ToolResult{Error: ErrInternalRetrievalK}, nil
	}

	artifacts, err := t.searcher.Search(ctx, query, k)
	if err != nil {
		return &ok.ToolResult{Error: &ok.ToolError{
			Code:    ErrInternalRetrievalBackend.Code,
			Message: ErrInternalRetrievalBackend.Message + ": " + err.Error(),
		}}, nil
	}

	snippets := make([]ok.Snippet, 0, len(artifacts))
	sources := make([]ok.Source, 0, len(artifacts))
	for _, a := range artifacts {
		if strings.TrimSpace(a.ID) == "" {
			return &ok.ToolResult{Error: ErrInvalidArtifact}, nil
		}
		text := canonicalSnippetText(a.Title, a.Summary)
		hash := snippetHash(text)
		snippets = append(snippets, ok.Snippet{
			Text:        text,
			ContentHash: hash,
			SourceRef:   a.ID,
		})
		sources = append(sources, ok.Source{
			Kind: ok.SourceArtifact,
			Artifact: &ok.ArtifactRef{
				ID:    a.ID,
				Kind:  "artifact",
				Title: a.Title,
			},
		})
	}

	return &ok.ToolResult{Snippets: snippets, Sources: sources}, nil
}

// canonicalSnippetText is the documented canonical form hashed for
// cite-back. Form: trim title, trim summary, join with a single "\n\n"
// separator. Empty summary collapses the trailing separator. No
// filesystem paths, no source URLs, no raw content_raw — those fields
// can carry PII path leakage. The cite-back verifier (SCOPE-08) MUST
// reproduce this exact form when validating planner citations.
func canonicalSnippetText(title, summary string) string {
	title = strings.TrimSpace(title)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return title
	}
	return title + "\n\n" + summary
}

// snippetHash returns the lowercase hex SHA-256 of the canonical text.
func snippetHash(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

// PgxGraphSearcher is a text-similarity adapter against the live
// artifacts table. It deliberately uses ILIKE on title/summary rather
// than the embedding column so it can run without the ML sidecar; the
// embedding-backed adapter is a follow-up (see scopes.md SCOPE-06
// finding).
type PgxGraphSearcher struct {
	pool *pgxpool.Pool
}

// NewPgxGraphSearcher wraps a pgx pool. Pool ownership stays with the
// caller; the searcher does not close it.
func NewPgxGraphSearcher(pool *pgxpool.Pool) *PgxGraphSearcher {
	if pool == nil {
		panic("openknowledge: PgxGraphSearcher requires a non-nil pgx pool")
	}
	return &PgxGraphSearcher{pool: pool}
}

// Search runs an ILIKE match against artifacts.title and artifacts.summary
// and returns up to k rows. The query string is parameterised; the only
// non-parameter SQL is the LIMIT, which is bounded by the tool to
// MaxInternalRetrievalK.
func (s *PgxGraphSearcher) Search(ctx context.Context, query string, k int) ([]GraphArtifact, error) {
	if k <= 0 || k > MaxInternalRetrievalK {
		return nil, errors.New("k out of range")
	}
	pattern := "%" + strings.ReplaceAll(strings.ReplaceAll(query, `\`, `\\`), `%`, `\%`) + "%"
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, COALESCE(summary, '')
		FROM artifacts
		WHERE title ILIKE $1 OR summary ILIKE $1
		ORDER BY created_at DESC
		LIMIT $2
	`, pattern, k)
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
