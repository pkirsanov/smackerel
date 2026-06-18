// Spec 095 SCOPE-07 / PKT-095-B — unit tests for the LIVE ingestion-front-door
// evergreen wiring in PublishRawArtifact. These cover the candidate build, the
// boolean-metadata reader, and the nil-safe persistence shape WITHOUT a DB (the
// pure helpers scoreEvergreen / buildEvergreenCandidate take no DB path). The
// actual INSERT persistence against a real Postgres is integration-gated
// (F-095-E2E-LIVE); these assertions pin the (*float64, *string) the INSERT
// binds for the evergreen_score / evergreen_source columns.
package pipeline

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestBuildEvergreenCandidate(t *testing.T) {
	art := connector.RawArtifact{
		SourceID:   "telegram",
		RawContent: "hello world",
		Metadata:   map[string]interface{}{"user_starred": true, "has_context": true},
	}
	cand := buildEvergreenCandidate(art)
	if cand.SourceKind != "telegram" {
		t.Errorf("SourceKind = %q, want telegram (from SourceID)", cand.SourceKind)
	}
	if cand.ContentLen != len("hello world") {
		t.Errorf("ContentLen = %d, want %d (len of RawContent)", cand.ContentLen, len("hello world"))
	}
	if !cand.UserStarred || !cand.HasContext {
		t.Errorf("metadata flags not read: starred=%t context=%t", cand.UserStarred, cand.HasContext)
	}
	// ArtifactID is threaded later by scoreEvergreen, not by the pure build.
	if cand.ArtifactID != "" {
		t.Errorf("buildEvergreenCandidate must not set ArtifactID, got %q", cand.ArtifactID)
	}
}

func TestMetadataBool(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]interface{}
		want bool
	}{
		{"nil metadata", nil, false},
		{"absent key", map[string]interface{}{"other": true}, false},
		{"present true", map[string]interface{}{"user_starred": true}, true},
		{"present false", map[string]interface{}{"user_starred": false}, false},
		{"non-bool value", map[string]interface{}{"user_starred": "true"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := metadataBool(tc.meta, "user_starred"); got != tc.want {
				t.Errorf("metadataBool = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestScoreEvergreen_NilScorerLeavesNull — NFR-3: a nil Scorer leaves both
// columns NULL (nil pointers ⇒ SQL NULL) and changes nothing else.
func TestScoreEvergreen_NilScorerLeavesNull(t *testing.T) {
	p := &RawArtifactPublisher{} // Scorer nil
	score, source := p.scoreEvergreen(context.Background(), "art-1",
		connector.RawArtifact{SourceID: "gmail", RawContent: "x"})
	if score != nil || source != nil {
		t.Errorf("nil scorer must leave both columns NULL, got score=%v source=%v", score, source)
	}
}

// TestScoreEvergreen_WiredScorerPersists — a wired scorer persists the signed
// score (+confidence when evergreen) and the provenance, and threads the
// artifact id + front-door signals into the candidate.
func TestScoreEvergreen_WiredScorerPersists(t *testing.T) {
	sc := &stubScorer{}
	p := &RawArtifactPublisher{Scorer: sc}
	art := connector.RawArtifact{
		SourceID:   "gmail",
		RawContent: "durable note",
		Metadata:   map[string]interface{}{"user_starred": true},
	}
	score, source := p.scoreEvergreen(context.Background(), "art-1", art)
	if score == nil || source == nil {
		t.Fatal("wired scorer must persist a non-nil score+source")
	}
	// gmail (not notification) ⇒ stub judges evergreen=true, conf 0.7 ⇒ +0.7.
	if *score != 0.7 {
		t.Errorf("evergreen score = %g, want 0.7 (+confidence)", *score)
	}
	if *source != "tier_signals" {
		t.Errorf("evergreen source = %q, want tier_signals (stub provenance)", *source)
	}
	if sc.last.ArtifactID != "art-1" {
		t.Errorf("artifact id not threaded to the scorer for trace correlation, got %q", sc.last.ArtifactID)
	}
	if sc.last.SourceKind != "gmail" || !sc.last.UserStarred {
		t.Errorf("front-door signals not forwarded: %+v", sc.last)
	}
}

// TestScoreEvergreen_EphemeralNegativeScore — an ephemeral judgment persists a
// NEGATIVE signed score, so the single column carries the direction.
func TestScoreEvergreen_EphemeralNegativeScore(t *testing.T) {
	sc := &stubScorer{}
	p := &RawArtifactPublisher{Scorer: sc}
	art := connector.RawArtifact{SourceID: "notification", RawContent: "running late"}
	score, _ := p.scoreEvergreen(context.Background(), "art-2", art)
	if score == nil || *score != -0.7 {
		t.Errorf("ephemeral (notification) must persist a negative score -0.7, got %v", score)
	}
}
