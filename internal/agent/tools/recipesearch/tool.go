// Package recipesearch registers the BUG-061-003 `recipe_search`
// agent tool. The tool delegates to the existing /api/search
// SearchEngine with `SearchFilters{Domain: "recipe"}` so it shares
// the same vector + LLM rerank + graph expand substrate as
// retrieval_search while restricting hits to recipe-domain artifacts.
//
// Wiring contract: production code in cmd/core constructs a *Services
// and calls SetServices once at startup. Until SetServices is called
// the handler returns a structured `{"error":"recipe_search_tools_not_configured"}`
// envelope so the trace surface fails loudly.
package recipesearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/api"
)

// ToolName is the single tool registered by this package.
const ToolName = "recipe_search"

// Searcher is the minimum surface this tool needs from the spec 037
// search engine. Satisfied by *api.SearchEngine in production.
type Searcher interface {
	Search(ctx context.Context, req api.SearchRequest) ([]api.SearchResult, int, string, error)
}

// Services holds the runtime dependencies for the recipe_search handler.
type Services struct {
	Engine  Searcher
	MaxTopK int
}

var (
	servicesMu sync.RWMutex
	services   *Services
)

// SetServices wires the production runtime. Pass nil to clear (test-only).
func SetServices(s *Services) {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	services = s
}

// ResetForTest clears the wired services. Test-only.
func ResetForTest() {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	services = nil
}

func loadServices() (*Services, error) {
	servicesMu.RLock()
	defer servicesMu.RUnlock()
	if services == nil {
		return nil, errors.New("recipe_search_tools_not_configured")
	}
	if services.Engine == nil {
		return nil, errors.New("recipe_search_tools_engine_not_configured")
	}
	if services.MaxTopK < 1 {
		return nil, fmt.Errorf("recipe_search_tools_max_top_k_invalid: %d", services.MaxTopK)
	}
	return services, nil
}

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
        "required": ["artifact_id", "title", "ingredient_summary", "score"],
        "properties": {
          "artifact_id":        {"type": "string"},
          "title":              {"type": "string"},
          "ingredient_summary": {"type": "string"},
          "score":              {"type": "number"}
        }
      }
    }
  }
}`)

func init() {
	agent.RegisterTool(agent.Tool{
		Name:             ToolName,
		Description:      "Search the user's knowledge graph for recipe-domain artifacts matching a free-text query; returns artifact_id-cited hits with titles and ingredient summaries.",
		InputSchema:      inputSchema,
		OutputSchema:     outputSchema,
		SideEffectClass:  agent.SideEffectRead,
		OwningPackage:    "internal/agent/tools/recipesearch",
		PerCallTimeoutMs: 2500,
		Handler:          handleRecipeSearch,
	})
}

type recipeSearchInput struct {
	Query  string `json:"query"`
	UserID string `json:"user_id"`
	TopK   int    `json:"top_k,omitempty"`
}

type recipeSearchHit struct {
	ArtifactID        string  `json:"artifact_id"`
	Title             string  `json:"title"`
	IngredientSummary string  `json:"ingredient_summary"`
	Score             float64 `json:"score"`
}

type recipeSearchOutput struct {
	Hits []recipeSearchHit `json:"hits"`
}

func handleRecipeSearch(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	svc, err := loadServices()
	if err != nil {
		return nil, err
	}
	var in recipeSearchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("recipe_search_bad_input: %w", err)
	}
	if in.UserID == "" {
		return nil, errors.New("recipe_search_missing_user_id")
	}
	if in.Query == "" {
		return nil, errors.New("recipe_search_empty_query")
	}

	limit := in.TopK
	if limit < 1 {
		limit = svc.MaxTopK
	}
	if limit > svc.MaxTopK {
		limit = svc.MaxTopK
	}

	results, _, _, err := svc.Engine.Search(ctx, api.SearchRequest{
		Query:   in.Query,
		Limit:   limit,
		Filters: api.SearchFilters{Domain: "recipe"},
	})
	if err != nil {
		return nil, fmt.Errorf("recipe_search_engine_error: %w", err)
	}

	out := recipeSearchOutput{Hits: make([]recipeSearchHit, 0, len(results))}
	for _, r := range results {
		out.Hits = append(out.Hits, recipeSearchHit{
			ArtifactID:        r.ArtifactID,
			Title:             r.Title,
			IngredientSummary: summarizeIngredients(r),
		})
	}
	return json.Marshal(out)
}

// summarizeIngredients extracts a short ingredient teaser from a
// SearchResult. Prefers the snippet (already a search-time excerpt);
// falls back to summary. Bounded at 200 chars.
func summarizeIngredients(r api.SearchResult) string {
	s := r.Snippet
	if s == "" {
		s = r.Summary
	}
	const cap = 200
	if len(s) > cap {
		return s[:cap]
	}
	return s
}
