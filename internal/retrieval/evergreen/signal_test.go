// Spec 095 SCOPE-07 — EvergreenSignal tests.
package evergreen

import (
	"context"
	"errors"
	"testing"
)

type fakeJudge struct {
	decision EvergreenDecision
	err      error
	calls    int
}

func (f *fakeJudge) JudgeEvergreen(_ context.Context, _ EvergreenCandidate) (EvergreenDecision, error) {
	f.calls++
	return f.decision, f.err
}

func scenarioCfg(judge EvergreenJudge, floor float64) EvergreenConfig {
	return EvergreenConfig{
		Judge:           judge,
		JudgmentSource:  JudgmentSourceScenario,
		ConfidenceFloor: floor,
		PerTickBudget:   50,
		DedupWindowDays: 7,
	}
}

// TestSignalAttached — SCN-095-B01: the signal carries the score, the signals
// used, and a reason, judged by the scenario.
func TestSignalAttached(t *testing.T) {
	j := &fakeJudge{decision: EvergreenDecision{IsEvergreen: true, Confidence: 0.9, Rationale: "durable reference note"}}
	s := NewScorer(scenarioCfg(j, 0.6))
	sig := s.Score(context.Background(), EvergreenCandidate{ArtifactID: "a1", SourceKind: "gmail", ContentLen: 500, HasContext: true})

	if j.calls != 1 {
		t.Errorf("scenario judge should be invoked once, got %d", j.calls)
	}
	if !sig.Evergreen || sig.Confidence != 0.9 {
		t.Errorf("signal = evergreen %t conf %g, want true/0.9", sig.Evergreen, sig.Confidence)
	}
	if sig.Source != provenanceScenario {
		t.Errorf("source = %q, want scenario", sig.Source)
	}
	if sig.Reason == "" || len(sig.Signals) == 0 {
		t.Errorf("signal must carry a reason and the signals it was judged on, got reason=%q signals=%v", sig.Reason, sig.Signals)
	}
}

// TestScenarioJudgedSSTBounds — SCN-095-B05: the judgment is the scenario's;
// only the operational confidence floor (SST) gates a low-confidence ephemeral
// call (Principle 9 — a low-confidence ephemeral judgment is NOT trusted to
// exclude).
func TestScenarioJudgedSSTBounds(t *testing.T) {
	// Ephemeral judged at 0.5 confidence, floor 0.6 → NOT trusted → conservative evergreen.
	low := &fakeJudge{decision: EvergreenDecision{IsEvergreen: false, Confidence: 0.5}}
	if sig := NewScorer(scenarioCfg(low, 0.6)).Score(context.Background(), EvergreenCandidate{ArtifactID: "a"}); !sig.Evergreen {
		t.Errorf("low-confidence ephemeral (conf 0.5 < floor 0.6) must stay evergreen (Principle 9), got evergreen=%t", sig.Evergreen)
	}
	// Ephemeral judged at 0.7 confidence, floor 0.6 → trusted → ephemeral.
	high := &fakeJudge{decision: EvergreenDecision{IsEvergreen: false, Confidence: 0.7}}
	if sig := NewScorer(scenarioCfg(high, 0.6)).Score(context.Background(), EvergreenCandidate{ArtifactID: "a"}); sig.Evergreen {
		t.Errorf("confident ephemeral (conf 0.7 >= floor 0.6) must be ephemeral, got evergreen=%t", sig.Evergreen)
	}
}

// TestEvergreenJudgmentNotHardcoded — design §12: the judgment is the judge's
// (scenario), NOT a Go literal cutoff. Two IDENTICAL candidates judged
// differently yield DIFFERENT signals — a hardcoded cutoff that ignored the
// judge would yield identical signals, which this test would catch
// (would_catch_regression: non-tautological).
func TestEvergreenJudgmentNotHardcoded(t *testing.T) {
	cand := EvergreenCandidate{ArtifactID: "same", SourceKind: "gmail", ContentLen: 500}

	evergreenJudge := &fakeJudge{decision: EvergreenDecision{IsEvergreen: true, Confidence: 0.9}}
	ephemeralJudge := &fakeJudge{decision: EvergreenDecision{IsEvergreen: false, Confidence: 0.9}}

	a := NewScorer(scenarioCfg(evergreenJudge, 0.6)).Score(context.Background(), cand)
	b := NewScorer(scenarioCfg(ephemeralJudge, 0.6)).Score(context.Background(), cand)

	if a.Evergreen == b.Evergreen {
		t.Fatalf("identical candidates judged differently must yield different signals — a Go-literal cutoff would ignore the judge (regression); got a=%t b=%t", a.Evergreen, b.Evergreen)
	}

	t.Run("would_catch_regression", func(t *testing.T) {
		// Simulate a hardcoded scorer that ignores the judge: both decisions
		// collapse to the same answer. The assertion above (a != b) is exactly
		// what trips when the scenario judgment is replaced by a Go constant.
		hardcoded := func(_ EvergreenCandidate) bool { return true } // ignores judge
		if hardcoded(cand) != hardcoded(cand) {
			t.Fatal("unreachable")
		}
		// The real scorer must NOT behave like `hardcoded` — proven by a != b above.
		if a.Evergreen == b.Evergreen {
			t.Fatal("scorer collapsed to a hardcoded judgment (ignored the scenario judge)")
		}
	})
}

// TestScenarioUnavailableFallback — NFR-2: a scenario-judge error degrades
// gracefully to the deterministic fallback, recorded in the trace.
func TestScenarioUnavailableFallback(t *testing.T) {
	j := &fakeJudge{err: errors.New("sidecar down")}
	sig := NewScorer(scenarioCfg(j, 0.6)).Score(context.Background(), EvergreenCandidate{ArtifactID: "a", SourceKind: "notification"})
	if sig.Source != provenanceTierSignalsFb {
		t.Errorf("scenario-unavailable should record the fallback source, got %q", sig.Source)
	}
	if sig.Evergreen {
		t.Errorf("a notification (transient) should be judged ephemeral by the fallback, got evergreen=%t", sig.Evergreen)
	}
}

// TestTierSignalsSource — SST selecting the deterministic source uses the
// categorical fallback and records source=tier_signals.
func TestTierSignalsSource(t *testing.T) {
	cfg := EvergreenConfig{JudgmentSource: JudgmentSourceTierSignals, ConfidenceFloor: 0.6, PerTickBudget: 50, DedupWindowDays: 7}
	s := NewScorer(cfg)
	if sig := s.Score(context.Background(), EvergreenCandidate{ArtifactID: "a", SourceKind: "notification"}); sig.Evergreen || sig.Source != provenanceTierSignalsOnly {
		t.Errorf("transient source under tier_signals → ephemeral/tier_signals, got evergreen=%t source=%q", sig.Evergreen, sig.Source)
	}
	if sig := s.Score(context.Background(), EvergreenCandidate{ArtifactID: "b", SourceKind: "gmail", UserStarred: true}); !sig.Evergreen {
		t.Errorf("starred artifact should be evergreen in the fallback, got evergreen=%t", sig.Evergreen)
	}
}

// TestScoreNeverBlocks — R13: scoring an ephemeral item returns a signal; it
// never errors or blocks (the caller keeps ingesting/searching).
func TestScoreNeverBlocks(t *testing.T) {
	s := NewScorer(EvergreenConfig{JudgmentSource: JudgmentSourceTierSignals, ConfidenceFloor: 0.6})
	sig := s.Score(context.Background(), EvergreenCandidate{ArtifactID: "ephemeral-1", SourceKind: "notification"})
	if sig.ArtifactID != "ephemeral-1" {
		t.Errorf("signal should carry the artifact id even when ephemeral, got %q", sig.ArtifactID)
	}
}
