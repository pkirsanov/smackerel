// Package web defines the WebSearchProvider interface used by the
// open-ended knowledge agent (spec 064 SCOPE-07) and ships three
// adapter implementations: SearxNG (functional), Brave and Tavily
// (stubs that return ErrProviderNotConfigured).
//
// Boundary: this package produces []WebSnippet, the raw provider
// payload. The agent loop (SCOPE-09) wraps each WebSnippet into a
// openknowledge.Source with Kind = SourceWeb before handing it to the
// cite-back verifier (SCOPE-08). Conversion is performed at the agent
// loop boundary so this package does not depend on the openknowledge
// type surface.
package web

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors returned by every WebSearchProvider implementation.
// They are typed so the agent loop can map them onto soft ToolError
// codes without string matching.
var (
	// ErrProviderNotConfigured indicates the provider has no usable
	// configuration (e.g. Brave/Tavily stub, or a SearxNG instance
	// constructed with an empty endpoint).
	ErrProviderNotConfigured = errors.New("openknowledge/web: provider not configured")

	// ErrProviderUnreachable indicates a transport-level failure or a
	// non-2xx response that is not specifically classified.
	ErrProviderUnreachable = errors.New("openknowledge/web: provider unreachable")

	// ErrInvalidQuery indicates the query failed local validation
	// (empty / whitespace-only / k <= 0 / k above the provider bound).
	ErrInvalidQuery = errors.New("openknowledge/web: invalid query")

	// ErrQuotaExceeded indicates the provider responded with HTTP 429
	// or an equivalent rate-limit signal.
	ErrQuotaExceeded = errors.New("openknowledge/web: provider quota exceeded")

	// ErrInvalidConfig indicates the constructor was called with a
	// structurally invalid argument (nil http.Client, empty endpoint,
	// etc.). Returned from constructors only; Search returns the more
	// specific errors above.
	ErrInvalidConfig = errors.New("openknowledge/web: invalid provider configuration")

	// ErrMalformedResponse indicates the provider returned a body that
	// could not be decoded into the expected schema.
	ErrMalformedResponse = errors.New("openknowledge/web: malformed provider response")
)

// MaxK is the upper bound this package enforces on a single Search
// call. The agent loop (SCOPE-09) caps its own per-iteration request
// well below this value; MaxK is a defence-in-depth bound to keep a
// runaway planner from issuing huge requests against the operator's
// configured provider.
const MaxK = 50

// WebSnippet is the raw provider payload for a single result. The
// ContentHash field is the canonical key used by the cite-back
// verifier (SCOPE-08); see CanonicalContentHash for the exact form.
//
// FetchedAt is stamped by the client, not trusted from the provider
// response, so the agent loop can prove the upper bound on snippet
// freshness without trusting an external clock.
type WebSnippet struct {
	URL         string
	Title       string
	Snippet     string
	ContentHash string
	FetchedAt   time.Time
	Provider    string
}

// WebSearchProvider is the interface every web-search adapter
// implements. Search MUST NOT block on outbound network calls beyond
// the deadline carried by ctx; transport errors MUST be wrapped with
// ErrProviderUnreachable; HTTP 429 MUST be wrapped with
// ErrQuotaExceeded. An empty result set is NOT an error: implementers
// return (nil, nil) or (empty slice, nil).
type WebSearchProvider interface {
	Search(ctx context.Context, query string, k int) ([]WebSnippet, error)
	// Name returns the canonical lowercase provider label
	// ("searxng", "brave", "tavily"). It MUST match the value stamped
	// into WebSnippet.Provider for snippets this provider returns.
	Name() string
}
