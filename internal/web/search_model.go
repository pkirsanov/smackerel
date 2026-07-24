package web

import (
	"context"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
)

// SearchState is the closed, mutually-exclusive vocabulary for the outcome
// region of the server-rendered Search surface (BUG-002-006). The server-owned
// terminal states are rendered identically in a complete baseline page and in
// the HTMX fragment; `loading`, `network`, and `retrying` are browser
// transition states and are never claimed by the server.
type SearchState string

const (
	// SearchStateReady is the initial page with no prior outcome implied.
	SearchStateReady SearchState = "ready"
	// SearchStateValidation is a blank/control/whitespace-only submission. The
	// HTTP layer returns 422 and ZERO SearchEngine or downstream search-domain
	// work is dispatched.
	SearchStateValidation SearchState = "validation"
	// SearchStateResults is an authorized query with one or more real results.
	SearchStateResults SearchState = "results"
	// SearchStateEmpty is an authorized query that completed with zero matches;
	// it carries no error or retry language.
	SearchStateEmpty SearchState = "empty"
	// SearchStateFilteredEmpty is an authorized query whose active filters
	// excluded otherwise-eligible rows. (Closed-vocabulary member; produced when
	// canonical filter wiring lands.)
	SearchStateFilteredEmpty SearchState = "filtered_empty"
	// SearchStateDegraded is a verified partial result while one dependency is
	// degraded. (Closed-vocabulary member.)
	SearchStateDegraded SearchState = "degraded"
	// SearchStateUnauthorized is a rejected session; never rendered as empty.
	SearchStateUnauthorized SearchState = "unauthorized"
	// SearchStateTimeout is a search that exceeded its deadline.
	SearchStateTimeout SearchState = "timeout"
	// SearchStateServerError is a search-engine or query failure with a safe
	// reference and Retry.
	SearchStateServerError SearchState = "server_error"
)

// SearchResultView is one projected search result row for the server-rendered
// Search outcome region. It is a presentation projection of api.SearchResult;
// it does not re-implement ranking or search semantics.
type SearchResultView struct {
	ID        string
	Title     string
	Type      string
	Summary   string
	SourceURL string
	QFCard    *qfdecisions.PacketCard
}

// KnowledgeMatchView is the pre-synthesized knowledge-layer concept match shown
// above raw results. It is a named projection so the typed SearchPageModel and
// the shared outcome template bind to one concrete type.
type KnowledgeMatchView struct {
	ConceptID     string
	Title         string
	Summary       string
	CitationCount int
	UpdatedAt     time.Time
}

// SearchPageModel is the single typed projection for both the complete baseline
// page and the HTMX outcome fragment. Exactly one State is set per response and
// the states are mutually exclusive (BUG-002-006 design "closed result-state
// model").
type SearchPageModel struct {
	Title             string
	Query             string
	State             SearchState
	ValidationMessage string
	ErrorMessage      string
	Results           []SearchResultView
	KnowledgeMatch    *KnowledgeMatchView
	ResultCount       int
}

// SearchExecutor is the narrow domain-dispatch seam the Search handler depends
// on. Production injects the real *api.SearchEngine; focused unit tests inject a
// counting fake that proves zero SearchEngine dispatch for blank/control/
// whitespace-only input. It is an observation seam only: no runtime parameter,
// header, cookie, query, route, or UI control selects a test path.
type SearchExecutor interface {
	Search(ctx context.Context, req api.SearchRequest) ([]api.SearchResult, int, string, error)
}
