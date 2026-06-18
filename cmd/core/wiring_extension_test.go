// Spec 095 F-095-EXT-INGEST — unit coverage for shareEvergreenScorer, the
// wiring that shares the connector front door's evergreen Scorer with the
// spec-058 Chrome-extension ingest publisher (cmd/core/wiring.go buildAPIDeps).
//
// Construction-time invariants only (mirrors wiring_assistant_facade_test.go):
// a supplied scorer is shared verbatim; a nil scorer leaves the publisher's
// Scorer a nil interface (NFR-3 graceful degrade). No live stack — the
// publisher's pool/nats are nil because PublishRawArtifact is never called; the
// assertions read only the constructed publisher's Scorer field.
package main

import (
	"testing"

	"github.com/smackerel/smackerel/internal/pipeline"
	"github.com/smackerel/smackerel/internal/retrieval/evergreen"
)

// newTestEvergreenScorer builds a real *evergreen.Scorer (deterministic
// tier_signals source; no live judge needed) for the wiring assertions.
func newTestEvergreenScorer() *evergreen.Scorer {
	return evergreen.NewScorer(evergreen.EvergreenConfig{
		JudgmentSource:  evergreen.JudgmentSourceTierSignals,
		ConfidenceFloor: 0.6,
		PerTickBudget:   50,
		DedupWindowDays: 7,
	})
}

// TestShareEvergreenScorer_WiresScorer — F-095-EXT-INGEST: the extension ingest
// publisher MUST share the SAME evergreen Scorer as the connector front door
// (cmd/core/services.go), so extension-captured artifacts are scored at
// ingestion identically to connector ones. Would fail (Scorer nil) if the
// wiring regressed to the pre-fix bare NewRawArtifactPublisher call.
func TestShareEvergreenScorer_WiresScorer(t *testing.T) {
	pub := pipeline.NewRawArtifactPublisher(nil, nil)
	scorer := newTestEvergreenScorer()

	shareEvergreenScorer(pub, scorer)

	if pub.Scorer == nil {
		t.Fatal("extension publisher must carry the evergreen scorer (F-095-EXT-INGEST), got nil Scorer")
	}
	if pub.Scorer != scorer {
		t.Errorf("extension publisher must carry the SAME scorer instance as the connector front door; got a different value")
	}
}

// TestShareEvergreenScorer_NilScorerSafe — NFR-3 / Principle 9: when
// retrieval.evergreen.enabled=false svc.evergreenScorer is nil; the publisher's
// Scorer MUST stay a nil interface so PublishRawArtifact persists a NULL
// evergreen_score and ingestion is byte-for-byte unchanged. Also guards the
// typed-nil-in-interface trap (a typed-nil *evergreen.Scorer in the interface
// field would be != nil and panic in scoreEvergreen).
func TestShareEvergreenScorer_NilScorerSafe(t *testing.T) {
	pub := pipeline.NewRawArtifactPublisher(nil, nil)

	shareEvergreenScorer(pub, nil)

	if pub.Scorer != nil {
		t.Errorf("nil scorer must leave pub.Scorer a nil interface (NFR-3), got non-nil %v", pub.Scorer)
	}
}

// TestShareEvergreenScorer_NilPublisherSafe — defensive: a nil publisher must
// not panic (the helper short-circuits).
func TestShareEvergreenScorer_NilPublisherSafe(t *testing.T) {
	shareEvergreenScorer(nil, newTestEvergreenScorer())
}
