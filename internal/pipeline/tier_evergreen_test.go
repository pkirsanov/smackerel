// Spec 095 SCOPE-07 — shared evergreen test stub for the pipeline package.
// The live ingestion-seam assertions live in ingest_evergreen_test.go (NFR-3:
// the tier outcome is unchanged; the scorer is additive). This file provides
// the stubScorer those tests inject.
package pipeline

import (
	"context"

	"github.com/smackerel/smackerel/internal/retrieval/evergreen"
)

// stubScorer returns a fixed signal derived from the candidate so the ingestion
// tests can assert the live seam attaches a signal without changing the tier.
type stubScorer struct {
	calls int
	last  evergreen.EvergreenCandidate
}

func (s *stubScorer) Score(_ context.Context, c evergreen.EvergreenCandidate) evergreen.EvergreenSignal {
	s.calls++
	s.last = c
	ever := c.SourceKind != "notification"
	return evergreen.EvergreenSignal{ArtifactID: c.ArtifactID, Evergreen: ever, Confidence: 0.7, Source: "tier_signals", Reason: "stub"}
}
