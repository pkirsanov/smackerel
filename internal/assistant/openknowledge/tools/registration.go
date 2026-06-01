package tools

// registration.go is the single entrypoint cmd/core wiring calls to
// populate an openknowledge.Registry with the four production
// tools (internal_retrieval, web_search, unit_convert, calculator).
//
// Why a function and NOT an init():
//
//   - internal_retrieval needs a GraphSearcher backed by the live
//     pgx pool.
//   - web_search needs a WebSearchProvider constructed from
//     OpenKnowledgeConfig.Provider + Endpoint + APIKey at startup.
//
// Both dependencies are unavailable at package load time, so an
// init() would have to register stubs and rebind later — exactly the
// pattern G029 (no stubs) forbids. The startup wiring layer in
// cmd/core composes Deps once and calls RegisterAll exactly once per
// boot. Tests construct Deps in-memory (fake searcher / fake
// provider) and verify the registry is wired correctly.
//
// Allowlist behaviour is owned by ok.NewRegistry; RegisterAll does
// NOT consult or mutate the allowlist. A tool may be registered yet
// not Enabled() if the operator excludes it from
// assistant.open_knowledge.tool_allowlist. The registry's allowlist
// is therefore the single source of truth for which tools the
// planner sees.

import (
	"errors"
	"fmt"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/web"
)

// Deps bundles the runtime dependencies the four production tools
// need. All fields are required; nil fields cause RegisterAll to
// return an error rather than silently registering a partial set.
type Deps struct {
	// GraphSearcher backs internal_retrieval. In production this is
	// *PgxGraphSearcher built from the live pgx pool. Tests inject
	// a fake.
	GraphSearcher GraphSearcher
	// WebSearchProvider backs web_search. In production this is
	// *web.SearxNG (or future Brave/Tavily) built from the operator's
	// OpenKnowledgeConfig. Tests inject a fake.
	WebSearchProvider web.WebSearchProvider
}

// Validate returns a multi-error describing every nil dependency. The
// caller surfaces it directly so the operator sees ALL missing wiring
// in one shot rather than one-at-a-time.
func (d Deps) Validate() error {
	var missing []string
	if d.GraphSearcher == nil {
		missing = append(missing, "GraphSearcher")
	}
	if d.WebSearchProvider == nil {
		missing = append(missing, "WebSearchProvider")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("openknowledge tools: missing required deps: %v", missing)
}

// RegisterAll registers internal_retrieval, web_search, unit_convert
// and calculator with the supplied registry. Order is alphabetic by
// tool name so Registry.Enabled() (which sorts by name) matches the
// registration order, easing debugging.
//
// Returns the first registration error (typically ErrDuplicateTool)
// or the Deps validation error. A nil registry is rejected.
func RegisterAll(registry *ok.Registry, deps Deps) error {
	if registry == nil {
		return errors.New("openknowledge tools: nil registry")
	}
	if err := deps.Validate(); err != nil {
		return err
	}
	tools := []ok.Tool{
		NewCalculator(),
		NewInternalRetrieval(deps.GraphSearcher),
		NewUnitConvert(),
		NewWebSearch(deps.WebSearchProvider),
	}
	for _, t := range tools {
		if err := registry.Register(t); err != nil {
			return fmt.Errorf("register %s: %w", t.Name(), err)
		}
	}
	return nil
}
