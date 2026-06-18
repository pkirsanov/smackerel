// Package vaguerecall implements spec 095 SCOPE-06 — the vague_recall
// retrieval strategy (Idea 1c). It is the router's default and safe fallback:
// a THIN adapter over the EXISTING §9.2 vector → graph-expand → LLM-rerank
// pipeline (injected as routing.VagueRecallExecutor). It changes NOTHING about
// that pipeline (NFR-3 zero regression) — it delegates and stamps the strategy
// kind for the trace.
//
// References:
//   - specs/095-retrieval-strategy-routing/spec.md R4, NFR-3, SCN-095-A04
//   - specs/095-retrieval-strategy-routing/design.md §2
//   - specs/095-retrieval-strategy-routing/scopes.md SCOPE-06
package vaguerecall

import (
	"context"
	"errors"
	"fmt"

	"github.com/smackerel/smackerel/internal/retrieval/routing"
)

// Strategy is the vague_recall retrieval strategy. It holds only the injected
// executor over the existing pipeline — it opens no store.
type Strategy struct {
	exec routing.VagueRecallExecutor
}

// New constructs the strategy from an injected executor over the existing
// §9.2 pipeline.
func New(exec routing.VagueRecallExecutor) *Strategy {
	return &Strategy{exec: exec}
}

// Kind reports the strategy kind.
func (s *Strategy) Kind() routing.StrategyKind { return routing.StrategyVagueRecall }

// Execute delegates to the existing pipeline unchanged and stamps the strategy
// kind. The delegated result's sources and answer are preserved verbatim
// (NFR-3 — the existing vague-recall path behaves exactly as today).
func (s *Strategy) Execute(ctx context.Context, req routing.RetrievalRequest) (routing.RetrievalResult, error) {
	if s.exec == nil {
		return routing.RetrievalResult{}, errors.New("vaguerecall: nil VagueRecallExecutor (must be injected)")
	}
	res, err := s.exec.VagueRecall(ctx, req.Query)
	if err != nil {
		return routing.RetrievalResult{}, fmt.Errorf("vaguerecall: existing pipeline: %w", err)
	}
	// Preserve the delegated result verbatim; only stamp the strategy kind so
	// the trace is attributable. Do NOT mutate sources/answer (NFR-3).
	res.Strategy = routing.StrategyVagueRecall
	return res, nil
}
