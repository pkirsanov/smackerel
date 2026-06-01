package tools

// web_search wraps a web.WebSearchProvider behind the
// openknowledge.Tool contract so the agent loop (SCOPE-09) can call
// it through the same interface as the deterministic tools. Snippets
// are mapped 1:1 into ok.Snippet (Text=Title+"\n"+Snippet+"\n"+URL
// is NOT used — the canonical ContentHash form lives in web/searxng.go
// and is preserved verbatim) and ok.Source{Kind: SourceWeb}. Error
// classification is straight pass-through of the typed sentinels
// defined in internal/assistant/openknowledge/web/provider.go.
//
// NO-DEFAULTS (G028): k is a required input; there is no implicit
// upper-bound silently applied. MaxWebSearchK caps planner runaway;
// raising it requires a spec change (mirrors MaxInternalRetrievalK).

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/web"
)

// MaxWebSearchK is the upper bound this tool enforces on a single
// Execute call. It is intentionally smaller than web.MaxK so the
// planner cannot exhaust the provider per-iteration budget; widening
// it requires a spec change (G028).
const MaxWebSearchK = 10

// Typed sentinel errors surfaced through ToolResult.Error.
var (
	ErrWebSearchMalformed     = &ok.ToolError{Code: "malformed_params", Message: "params do not match schema"}
	ErrWebSearchEmptyQuery    = &ok.ToolError{Code: "invalid_query", Message: "query must be non-empty after trim"}
	ErrWebSearchK             = &ok.ToolError{Code: "invalid_k", Message: "k must be > 0 and <= MaxWebSearchK"}
	ErrWebSearchUnreachable   = &ok.ToolError{Code: "provider_unreachable", Message: "web search provider unreachable"}
	ErrWebSearchQuota         = &ok.ToolError{Code: "provider_quota_exceeded", Message: "web search provider quota exceeded"}
	ErrWebSearchCircuitOpen   = &ok.ToolError{Code: "provider_circuit_open", Message: "web search provider circuit breaker is open"}
	ErrWebSearchNotConfigured = &ok.ToolError{Code: "provider_not_configured", Message: "web search provider not configured"}
	ErrWebSearchMalformedResp = &ok.ToolError{Code: "provider_malformed_response", Message: "web search provider returned a malformed response"}
	ErrWebSearchBackend       = &ok.ToolError{Code: "backend_failure", Message: "web search provider returned an error"}
)

const webSearchSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["query", "k"],
  "properties": {
    "query": {"type": "string", "minLength": 1},
    "k": {"type": "integer", "minimum": 1, "maximum": 10}
  }
}`

type webSearchParams struct {
	Query *string `json:"query"`
	K     *int    `json:"k"`
}

// WebSearch is the registry-facing handle.
type WebSearch struct {
	provider web.WebSearchProvider
}

// NewWebSearch wraps a non-nil WebSearchProvider. The provider's
// underlying http.Client owns egress policy; this tool does not
// re-validate URLs.
func NewWebSearch(provider web.WebSearchProvider) *WebSearch {
	if provider == nil {
		panic("openknowledge: web_search requires a non-nil WebSearchProvider")
	}
	return &WebSearch{provider: provider}
}

// Name reports the registry key.
func (WebSearch) Name() string { return "web_search" }

// Description summarises the tool for the planner prompt.
func (WebSearch) Description() string {
	return "Search the public web for up to k snippets matching a free-text query via the operator-configured provider. Returns Source entries with Kind=SourceWeb."
}

// ParamsSchema returns the JSONSchema for Execute params.
func (WebSearch) ParamsSchema() json.RawMessage {
	return json.RawMessage(webSearchSchema)
}

// Execute validates params, calls the provider, and maps WebSnippet
// into the canonical ToolResult envelope. The ContentHash from the
// provider (see web.CanonicalContentHash) is preserved verbatim so
// the cite-back verifier (SCOPE-08) can anchor citations without
// re-hashing.
func (t *WebSearch) Execute(ctx context.Context, params json.RawMessage) (*ok.ToolResult, error) {
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p webSearchParams
	if err := dec.Decode(&p); err != nil {
		return &ok.ToolResult{Error: ErrWebSearchMalformed}, nil
	}
	if p.Query == nil || p.K == nil {
		return &ok.ToolResult{Error: ErrWebSearchMalformed}, nil
	}
	query := strings.TrimSpace(*p.Query)
	if query == "" {
		return &ok.ToolResult{Error: ErrWebSearchEmptyQuery}, nil
	}
	k := *p.K
	if k <= 0 || k > MaxWebSearchK {
		return &ok.ToolResult{Error: ErrWebSearchK}, nil
	}

	snippets, err := t.provider.Search(ctx, query, k)
	if err != nil {
		return &ok.ToolResult{Error: classifyProviderError(err)}, nil
	}

	outSnippets := make([]ok.Snippet, 0, len(snippets))
	outSources := make([]ok.Source, 0, len(snippets))
	for _, s := range snippets {
		// Defensive: web/searxng.go already skips empty-URL rows;
		// re-check here so any future provider that drops the
		// invariant cannot smuggle in unciteable snippets.
		if strings.TrimSpace(s.URL) == "" || strings.TrimSpace(s.ContentHash) == "" {
			continue
		}
		outSnippets = append(outSnippets, ok.Snippet{
			Text:        s.Snippet,
			ContentHash: s.ContentHash,
			SourceRef:   s.URL,
		})
		outSources = append(outSources, ok.Source{
			Kind: ok.SourceWeb,
			Web: &ok.WebSource{
				URL:         s.URL,
				Title:       s.Title,
				Provider:    s.Provider,
				FetchedAt:   s.FetchedAt,
				ContentHash: s.ContentHash,
				Snippet:     s.Snippet,
			},
		})
	}
	return &ok.ToolResult{Snippets: outSnippets, Sources: outSources}, nil
}

// classifyProviderError maps the typed sentinels exported by the web
// package onto the soft ToolError codes the agent loop surfaces. We
// match with errors.Is to avoid string parsing.
func classifyProviderError(err error) *ok.ToolError {
	switch {
	case errors.Is(err, web.ErrProviderNotConfigured):
		return ErrWebSearchNotConfigured
	case errors.Is(err, web.ErrCircuitOpen):
		return ErrWebSearchCircuitOpen
	case errors.Is(err, web.ErrProviderUnreachable):
		return ErrWebSearchUnreachable
	case errors.Is(err, web.ErrQuotaExceeded):
		return ErrWebSearchQuota
	case errors.Is(err, web.ErrInvalidQuery):
		return ErrWebSearchEmptyQuery
	case errors.Is(err, web.ErrMalformedResponse):
		return ErrWebSearchMalformedResp
	default:
		return &ok.ToolError{Code: ErrWebSearchBackend.Code, Message: ErrWebSearchBackend.Message + ": " + err.Error()}
	}
}
