// Package retrieval registers the spec 061 SCOPE-03 `retrieval_search`
// agent tool. The tool routes the retrieval scenario's queries through
// the existing /api/search SearchEngine so the agent honors the same
// vector + LLM rerank + graph expand contract the PWA Screen 5 search
// surface uses.
//
// The tool is a sibling of internal/agent (rather than internal/agent
// itself or internal/api) because:
//
//   - internal/api already depends on internal/agent (executor wiring);
//     registering a tool inside internal/api would produce an import
//     cycle.
//   - internal/agent must stay substrate-only (no skill-specific code).
//
// Wiring contract: production code in cmd/core constructs a *Services
// and calls SetServices once at startup. Until SetServices is called
// the handler returns a structured `{"error":"retrieval_tools_not_configured"}`
// envelope — failing loudly inside the trace instead of crashing the
// binary, and never silently returning empty hits (which would defeat
// the provenance gate in BS-007).
package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/api"
)

// ToolName is the single tool registered by this package. Wiring and
// allowlist code MUST consult this constant rather than hard-coding
// the string.
const ToolName = "retrieval_search"

// Searcher is the minimum surface this tool needs from the spec 037
// search engine. It is satisfied by *api.SearchEngine in production
// and by a fake in tests.
type Searcher interface {
	Search(ctx context.Context, req api.SearchRequest) ([]api.SearchResult, int, string, error)
}

// Services holds the runtime dependencies required by the retrieval
// tool handler. Production wiring constructs one in cmd/core and
// calls SetServices once before the agent bridge starts dispatching.
// Tests construct their own and override via SetServices /
// ResetForTest.
type Services struct {
	// Engine is the search engine that powers /api/search; in production
	// this is the same singleton wired into the API surface so the agent
	// and PWA share one retrieval substrate.
	Engine Searcher
	// MaxTopK is the SST-derived cap on the per-call `top_k` parameter
	// (assistant.skills.retrieval.top_k). Requests with a larger
	// top_k are silently clamped down to this value before the
	// SearchEngine is invoked. MUST be >= 1.
	MaxTopK int
}

var (
	servicesMu sync.RWMutex
	services   *Services
)

// SetServices wires the production retrieval runtime into the
// retrieval_search handler. Pass nil to clear (test-only). Calling
// SetServices is idempotent; the most recent non-nil call wins.
func SetServices(s *Services) {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	services = s
}

// ResetForTest clears the wired services. Test-only — production code
// MUST NOT call this.
func ResetForTest() {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	services = nil
}

// loadServices returns the wired services or a structured error if
// SetServices has not been called.
func loadServices() (*Services, error) {
	servicesMu.RLock()
	defer servicesMu.RUnlock()
	if services == nil {
		return nil, errors.New("retrieval_tools_not_configured")
	}
	if services.Engine == nil {
		return nil, errors.New("retrieval_tools_engine_not_configured")
	}
	if services.MaxTopK < 1 {
		return nil, fmt.Errorf("retrieval_tools_max_top_k_invalid: %d", services.MaxTopK)
	}
	return services, nil
}

// -------------------- schemas --------------------

var inputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["query", "user_id"],
  "properties": {
    "query":   {"type": "string", "minLength": 1},
    "user_id": {"type": "string", "minLength": 1},
    "top_k":   {"type": "integer", "minimum": 1, "maximum": 50}
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["hits"],
  "properties": {
    "hits": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["artifact_id", "title", "snippet", "captured_at"],
        "properties": {
          "artifact_id": {"type": "string"},
          "title":       {"type": "string"},
          "snippet":     {"type": "string"},
          "captured_at": {"type": "string"}
        }
      }
    }
  }
}`)

// -------------------- registration --------------------

func init() {
	agent.RegisterTool(agent.Tool{
		Name:             ToolName,
		Description:      "Search the user's knowledge graph for artifacts matching a free-text query; returns artifact_id-cited hits with titles, snippets, and capture timestamps.",
		InputSchema:      inputSchema,
		OutputSchema:     outputSchema,
		SideEffectClass:  agent.SideEffectRead,
		OwningPackage:    "internal/agent/tools/retrieval",
		PerCallTimeoutMs: 2500,
		Handler:          handleRetrievalSearch,
	})
}

// -------------------- handler --------------------

type retrievalInput struct {
	Query  string `json:"query"`
	UserID string `json:"user_id"`
	TopK   int    `json:"top_k,omitempty"`
}

type retrievalHit struct {
	ArtifactID string `json:"artifact_id"`
	Title      string `json:"title"`
	Snippet    string `json:"snippet"`
	CapturedAt string `json:"captured_at"`
}

type retrievalOutput struct {
	Hits []retrievalHit `json:"hits"`
}

func handleRetrievalSearch(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	svc, err := loadServices()
	if err != nil {
		return nil, err
	}
	var in retrievalInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("retrieval_search_bad_input: %w", err)
	}
	if in.UserID == "" {
		return nil, errors.New("retrieval_search_missing_user_id")
	}
	if in.Query == "" {
		return nil, errors.New("retrieval_search_empty_query")
	}

	// Cap top_k at SST-derived MaxTopK. Zero/negative requested → use
	// the cap directly so the tool always returns at most MaxTopK hits.
	limit := in.TopK
	if limit < 1 {
		limit = svc.MaxTopK
	}
	if limit > svc.MaxTopK {
		limit = svc.MaxTopK
	}

	results, _, _, err := svc.Engine.Search(ctx, api.SearchRequest{
		Query: in.Query,
		Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("retrieval_search_engine_error: %w", err)
	}

	out := retrievalOutput{Hits: make([]retrievalHit, 0, len(results))}
	for _, r := range results {
		out.Hits = append(out.Hits, retrievalHit{
			ArtifactID: r.ArtifactID,
			Title:      r.Title,
			Snippet:    r.Snippet,
			CapturedAt: r.CreatedAt,
		})
	}
	return json.Marshal(out)
}
