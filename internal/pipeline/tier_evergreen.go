// Spec 095 SCOPE-07 — evergreen scoring seam for the ingestion pipeline.
//
// LIVE WIRING (PKT-095-B, delivered): the real ingestion front door is
// RawArtifactPublisher.PublishRawArtifact (ingest.go), which resolves the tier
// via resolveTierFromMetadata and now ADDITIVELY scores + persists the evergreen
// signal through the injected EvergreenScorer defined in THIS file. That is the
// production path that scores real ingested artifacts.
//
// EvergreenScorer (below) is the abstraction the publisher depends on (not the
// concrete *evergreen.Scorer), so the dependency is one-directional
// (pipeline → evergreen) with no cycle and tests can inject a stub.
//
// History: an earlier additive helper (AssignTierWithEvergreen) sited the signal
// at the AssignTier (capital) tier-assignment heuristic. That heuristic has NO
// live production caller (the live door resolves tier from connector-provided
// metadata via resolveTierFromMetadata), so the helper was superseded by the
// PublishRawArtifact wiring (finding G-095-GAPS-01) and removed as dead code by
// the spec 095 simplify phase. NFR-3 (existing tier outcomes unchanged) is
// proven on the live path: ingest_test.go::TestResolveTierFromMetadata_* +
// ingest_evergreen_test.go (nil scorer => NULL, wired scorer => additive only).
//
// References:
//   - specs/095-retrieval-strategy-routing/spec.md R10, NFR-2, NFR-3
//   - specs/095-retrieval-strategy-routing/design.md §6
//   - specs/095-retrieval-strategy-routing/scopes.md SCOPE-07
package pipeline

import (
	"context"

	"github.com/smackerel/smackerel/internal/retrieval/evergreen"
)

// EvergreenScorer is the seam the evergreen signal plugs into. It is the LIVE
// seam consumed by RawArtifactPublisher at the ingestion front door (ingest.go);
// implemented by *evergreen.Scorer and wired in cmd/core.
type EvergreenScorer interface {
	Score(ctx context.Context, c evergreen.EvergreenCandidate) evergreen.EvergreenSignal
}
