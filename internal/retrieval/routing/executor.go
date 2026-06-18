// Spec 095 SCOPE-06 — the retrieval executor: the seam that ties the router to
// the selected strategy overlay. This is the pre-retrieval stage the facade's
// retrieval_qa path consumes (mirroring the LookupNLRouting seam). The actual
// facade call-site wiring is routed to spec 061 as PKT-095-A (the
// internal/assistant/ substrate is read-only under spec 095); the Executor is
// the ready-to-wire capability.
//
// The Executor holds only the RetrievalStrategy interface for each kind — it
// does NOT import the strategy sub-packages (which import this package), so
// there is no import cycle. cmd/core constructs the concrete strategies and
// injects them.
//
// References:
//   - specs/095-retrieval-strategy-routing/design.md §3
//   - specs/095-retrieval-strategy-routing/scopes.md SCOPE-06
package routing

import (
	"context"
	"fmt"

	"github.com/smackerel/smackerel/internal/assistant/intent"
)

// Executor routes an inbound intent to a strategy and executes it. It always
// has a vague_recall strategy registered (the safe fallback).
type Executor struct {
	router     *Router
	strategies map[StrategyKind]RetrievalStrategy
}

// NewExecutor builds an executor from a router and the concrete strategy
// overlays. It REQUIRES a vague_recall strategy (the router's safe fallback
// must always be executable); construction fails loud otherwise.
func NewExecutor(router *Router, strategies ...RetrievalStrategy) (*Executor, error) {
	if router == nil {
		return nil, fmt.Errorf("routing: nil router")
	}
	m := make(map[StrategyKind]RetrievalStrategy, len(strategies))
	for _, s := range strategies {
		if s == nil {
			continue
		}
		if !IsValidStrategyKind(s.Kind()) {
			return nil, fmt.Errorf("routing: strategy reports unknown kind %q", s.Kind())
		}
		m[s.Kind()] = s
	}
	if _, ok := m[StrategyVagueRecall]; !ok {
		return nil, fmt.Errorf("routing: a vague_recall strategy is REQUIRED (the router's safe fallback must be executable)")
	}
	return &Executor{router: router, strategies: m}, nil
}

// Retrieve routes the intent to a strategy and executes it, returning the
// result and the traced selection. If the selected strategy has no registered
// overlay, it degrades to vague_recall (the safe fallback is always present),
// and the selection's trace records the original decision.
func (e *Executor) Retrieve(ctx context.Context, in intent.CompiledIntent, req RetrievalRequest) (RetrievalResult, StrategySelection, error) {
	sel := e.router.Route(in)
	req.Selection = sel

	strat, ok := e.strategies[sel.Strategy]
	if !ok {
		// The selected specialized overlay is not wired; degrade to the
		// always-present safe fallback rather than erroring the user's query.
		strat = e.strategies[StrategyVagueRecall]
		sel.Strategy = StrategyVagueRecall
		sel.FellBack = true
		sel.Reason = ReasonStrategyDisabled
		req.Selection = sel
	}
	res, err := strat.Execute(ctx, req)
	if err != nil {
		return RetrievalResult{}, sel, err
	}
	return res, sel, nil
}

// Selection exposes the router's pure decision without executing — useful for
// the facade to observe the trace (Principle 8) before dispatch.
func (e *Executor) Selection(in intent.CompiledIntent) StrategySelection {
	return e.router.Route(in)
}
