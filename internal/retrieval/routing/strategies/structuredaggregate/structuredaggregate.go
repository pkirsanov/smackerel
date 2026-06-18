// Package structuredaggregate implements spec 095 SCOPE-05 — the
// structured_aggregate retrieval strategy (Idea 1b). For aggregate/superlative
// intents over structured data ("which month did I spend the most on
// subscriptions?") it runs a structured query over the EXISTING
// expenses/subscriptions tables and returns the exact extremum, instead of the
// §9.2 path's single most-similar chunk (which misses the actually-highest
// rows).
//
// It is a THIN adapter over the injected routing.SpendAggregator (wired in
// cmd/core over internal/intelligence/{expenses,subscriptions}). It introduces
// NO new SQL/OLAP engine and NO new table (Principle 5). Aggregates over
// financial-markets/QF artifacts return descriptive recall only — no advice
// (Principle 10).
//
// References:
//   - specs/095-retrieval-strategy-routing/spec.md R3, SCN-095-A03, Principle 10
//   - specs/095-retrieval-strategy-routing/design.md §5
//   - specs/095-retrieval-strategy-routing/scopes.md SCOPE-05
package structuredaggregate

import (
	"context"
	"errors"
	"fmt"

	"github.com/smackerel/smackerel/internal/retrieval/routing"
)

// Strategy is the structured_aggregate retrieval strategy. It holds only an
// injected SpendAggregator — it opens no store and runs no SQL itself.
type Strategy struct {
	aggregator routing.SpendAggregator
}

// New constructs the strategy from an injected aggregator over the existing
// intelligence aggregates.
func New(aggregator routing.SpendAggregator) *Strategy {
	return &Strategy{aggregator: aggregator}
}

// Kind reports the strategy kind.
func (s *Strategy) Kind() routing.StrategyKind { return routing.StrategyStructuredAggregate }

// Execute maps the aggregate intent + slots onto the existing aggregate and
// returns the exact computed extremum with structured-table provenance. For
// financial artifacts the answer is descriptive recall only (Principle 10).
func (s *Strategy) Execute(ctx context.Context, req routing.RetrievalRequest) (routing.RetrievalResult, error) {
	if s.aggregator == nil {
		return routing.RetrievalResult{}, errors.New("structuredaggregate: nil SpendAggregator (must be injected)")
	}
	q := req.Aggregate
	if q.Extremum == "" {
		q.Extremum = routing.ExtremumMax
	}
	result, err := s.aggregator.SuperlativeSpend(ctx, q)
	if err != nil {
		return routing.RetrievalResult{}, fmt.Errorf("structuredaggregate: superlative spend: %w", err)
	}

	answer := fmt.Sprintf("%s spend was %s in %s (%.2f), computed from the %s table.",
		q.Category, extremumWord(q.Extremum), result.Bucket, result.Amount, result.Table)
	if q.Financial {
		// Principle 10 — descriptive recall only; never advice.
		answer = fmt.Sprintf("For reference only (no financial advice): %s", answer)
	}

	return routing.RetrievalResult{
		Strategy: routing.StrategyStructuredAggregate,
		Answer:   answer,
		Sources: []routing.RetrievedSource{{
			Kind:       routing.SourceStructuredAggregate,
			ArtifactID: "",
			Detail:     fmt.Sprintf("%s table; %s=%s amount=%.2f", result.Table, q.Period, result.Bucket, result.Amount),
		}},
	}, nil
}

func extremumWord(e routing.AggregateExtremum) string {
	if e == routing.ExtremumMin {
		return "lowest"
	}
	return "highest"
}
