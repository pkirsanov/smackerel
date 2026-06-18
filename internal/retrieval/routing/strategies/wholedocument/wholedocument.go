// Package wholedocument implements spec 095 SCOPE-04 — the whole_document
// retrieval strategy (Idea 1a). For complete-context intents ("summarize the
// whole March 5th meeting") it fetches the FULL preserved artifact and
// synthesizes from complete context, instead of the §9.2 top-k chunk
// similarity subset that silently biases the answer.
//
// The full artifact is read through the injected routing.ArtifactFetcher over
// the EXISTING store (Principle 5 — no new index). The result reuses the
// full-artifact citation primitive (knowledge.AgentAnswerSource{Kind:
// artifact}) conceptually via routing.SourceFullArtifact.
//
// References:
//   - specs/095-retrieval-strategy-routing/spec.md R2, SCN-095-A02
//   - specs/095-retrieval-strategy-routing/design.md §2
//   - specs/095-retrieval-strategy-routing/scopes.md SCOPE-04
package wholedocument

import (
	"context"
	"errors"
	"fmt"

	"github.com/smackerel/smackerel/internal/retrieval/routing"
)

// Strategy is the whole_document retrieval strategy. It holds only an injected
// ArtifactFetcher — it opens no store.
type Strategy struct {
	fetcher routing.ArtifactFetcher
}

// New constructs the strategy from an injected full-artifact fetcher.
func New(fetcher routing.ArtifactFetcher) *Strategy {
	return &Strategy{fetcher: fetcher}
}

// Kind reports the strategy kind.
func (s *Strategy) Kind() routing.StrategyKind { return routing.StrategyWholeDocument }

// Execute fetches the FULL preserved artifact by id and assembles a
// complete-context result. It never fetches a top-k chunk subset — the whole
// document is the unit of retrieval (R2). The returned RetrievalResult carries
// FullArtifact=true and a full-artifact citation source (Principle 8).
func (s *Strategy) Execute(ctx context.Context, req routing.RetrievalRequest) (routing.RetrievalResult, error) {
	if s.fetcher == nil {
		return routing.RetrievalResult{}, errors.New("wholedocument: nil ArtifactFetcher (must be injected)")
	}
	if req.ArtifactID == "" {
		return routing.RetrievalResult{}, errors.New("wholedocument: empty ArtifactID — cannot fetch the full document")
	}
	art, err := s.fetcher.FetchFullArtifact(ctx, req.ArtifactID)
	if err != nil {
		return routing.RetrievalResult{}, fmt.Errorf("wholedocument: fetch full artifact %s: %w", req.ArtifactID, err)
	}
	return routing.RetrievalResult{
		Strategy:     routing.StrategyWholeDocument,
		FullArtifact: true,
		Answer:       art.Content, // synthesized from the COMPLETE artifact, not a chunk subset
		Sources: []routing.RetrievedSource{{
			Kind:       routing.SourceFullArtifact,
			ArtifactID: art.ID,
			Detail:     fmt.Sprintf("full artifact %q (%d chunks)", art.Title, art.NumChunks),
		}},
	}, nil
}
