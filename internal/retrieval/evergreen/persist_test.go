// Spec 095 SCOPE-07 / PKT-095-B — tests for the durable persistence shape
// (signed evergreen_score encoding + the Principle-9 NULL-not-excluded reader)
// and for the race-free late judge binding used by cmd/core.
package evergreen

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestPersistedScore(t *testing.T) {
	cases := []struct {
		name string
		sig  EvergreenSignal
		want float64
	}{
		{"evergreen high conf", EvergreenSignal{Evergreen: true, Confidence: 0.9}, 0.9},
		{"evergreen low conf", EvergreenSignal{Evergreen: true, Confidence: 0.5}, 0.5},
		{"ephemeral high conf", EvergreenSignal{Evergreen: false, Confidence: 0.8}, -0.8},
		{"ephemeral zero conf", EvergreenSignal{Evergreen: false, Confidence: 0}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sig.PersistedScore(); got != tc.want {
				t.Errorf("PersistedScore() = %g, want %g", got, tc.want)
			}
		})
	}
}

func TestEvergreenFromPersistedScore(t *testing.T) {
	// Round-trip: a persisted score recovers the judgment direction.
	if !EvergreenFromPersistedScore((EvergreenSignal{Evergreen: true, Confidence: 0.9}).PersistedScore()) {
		t.Error("evergreen signal must round-trip to evergreen=true")
	}
	if EvergreenFromPersistedScore((EvergreenSignal{Evergreen: false, Confidence: 0.9}).PersistedScore()) {
		t.Error("ephemeral signal must round-trip to evergreen=false")
	}
	// Boundary: exactly 0 is evergreen (lean-evergreen, Principle 9).
	if !EvergreenFromPersistedScore(0) {
		t.Error("score 0 must be treated as evergreen (Principle 9 boundary)")
	}
	if EvergreenFromPersistedScore(-0.01) {
		t.Error("a negative score must be ephemeral")
	}
}

func TestPoolExcludedByPersistedScore(t *testing.T) {
	cases := []struct {
		name         string
		scorePresent bool
		score        float64
		exclude      bool
		want         bool
	}{
		{"switch off, ephemeral present", true, -0.9, false, false},
		{"NULL score never excluded (Principle 9)", false, 0, true, false},
		{"present evergreen not excluded", true, 0.7, true, false},
		{"present ephemeral excluded", true, -0.7, true, true},
		{"present boundary 0 not excluded", true, 0, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PoolExcludedByPersistedScore(tc.scorePresent, tc.score, tc.exclude); got != tc.want {
				t.Errorf("PoolExcludedByPersistedScore(present=%t, score=%g, exclude=%t) = %t, want %t",
					tc.scorePresent, tc.score, tc.exclude, got, tc.want)
			}
		})
	}
}

// TestPoolExclusionSQLPredicate proves the SQL twin of
// PoolExcludedByPersistedScore: OFF (the shipped default) emits an empty
// fragment so the host candidate query is byte-for-byte unchanged; ON emits a
// fragment that keeps NULL (Principle 9) and present-evergreen (>= 0) rows and
// drops only present-ephemeral (< 0) rows (R12). The qualifier handles both
// aliased (synthesis: FROM artifacts a) and unaliased (digest: FROM artifacts)
// host queries.
func TestPoolExclusionSQLPredicate(t *testing.T) {
	// Property 1 — policy off ⇒ "" (default unchanged, safe additive activation).
	if got := PoolExclusionSQLPredicate("a", false); got != "" {
		t.Errorf("exclusion off must emit an empty fragment (byte-for-byte unchanged), got %q", got)
	}
	if got := PoolExclusionSQLPredicate("", false); got != "" {
		t.Errorf("exclusion off (unaliased) must emit an empty fragment, got %q", got)
	}

	// Property 2/3 — policy on ⇒ keep NULL + evergreen, drop ephemeral.
	aliased := PoolExclusionSQLPredicate("a", true)
	if want := " AND (a.evergreen_score IS NULL OR a.evergreen_score >= 0)"; aliased != want {
		t.Errorf("aliased fragment = %q, want %q", aliased, want)
	}
	unaliased := PoolExclusionSQLPredicate("", true)
	if want := " AND (evergreen_score IS NULL OR evergreen_score >= 0)"; unaliased != want {
		t.Errorf("unaliased fragment = %q, want %q", unaliased, want)
	}
	// Property 3 — a NULL score is never excluded (Principle 9): the fragment
	// must whitelist IS NULL.
	if !strings.Contains(aliased, "IS NULL") {
		t.Error("fragment must keep NULL (not-yet-scored ⇒ evergreen, Principle 9)")
	}
	// Property 2 — present-evergreen (>= 0) kept; the boundary is the same
	// `>= 0` the Go reader EvergreenFromPersistedScore uses, so SQL and Go
	// agree (ephemeral is strictly < 0, evergreen is >= 0).
	if !strings.Contains(aliased, ">= 0") {
		t.Error("fragment must keep present-evergreen rows (score >= 0)")
	}
	// No SQL placeholder may appear in the fragment (it must not shift the
	// host query's positional args, e.g. synthesis's $1).
	if strings.Contains(aliased, "$") || strings.Contains(unaliased, "$") {
		t.Error("exclusion fragment must carry no positional placeholders")
	}

	t.Run("would_catch_regression", func(t *testing.T) {
		// If a regression flipped the boundary to drop NULLs (hiding
		// not-yet-scored artifacts), the IS NULL whitelist would vanish.
		if !strings.Contains(PoolExclusionSQLPredicate("a", true), "IS NULL") {
			t.Fatal("regression: NULL no longer whitelisted ⇒ not-yet-scored items wrongly excluded (Principle 9)")
		}
	})
}

// TestSetJudgeLateBinding proves cmd/core's late-binding contract: a scorer
// built with judgment_source=scenario but NO judge falls back to the
// deterministic source; after SetJudge upgrades it to a scripted scenario
// judge, the SAME scorer routes through the judge. Non-tautological: the two
// phases produce DIFFERENT provenance for the same candidate.
func TestSetJudgeLateBinding(t *testing.T) {
	s := NewScorer(EvergreenConfig{JudgmentSource: JudgmentSourceScenario, ConfidenceFloor: 0.6})

	// Before SetJudge: scenario source but no judge ⇒ deterministic fallback.
	before := s.Score(context.Background(), EvergreenCandidate{ArtifactID: "a", SourceKind: "gmail", UserStarred: true})
	if before.Source != provenanceTierSignalsFb {
		t.Fatalf("before SetJudge: source = %q, want %q (deterministic fallback when no judge wired)", before.Source, provenanceTierSignalsFb)
	}

	// After SetJudge: the same scorer routes through the scenario judge.
	s.SetJudge(&fakeJudge{decision: EvergreenDecision{IsEvergreen: false, Confidence: 0.95}})
	after := s.Score(context.Background(), EvergreenCandidate{ArtifactID: "a", SourceKind: "gmail", UserStarred: true})
	if after.Source != provenanceScenario {
		t.Fatalf("after SetJudge: source = %q, want %q (scenario judge now wired)", after.Source, provenanceScenario)
	}
	// The scenario judged ephemeral at 0.95 >= floor 0.6 ⇒ ephemeral; the
	// deterministic fallback judged a starred item evergreen. Proves the
	// judgment actually changed (not a hardcoded path).
	if before.Evergreen == after.Evergreen {
		t.Errorf("late-bound judge did not change the judgment: before=%t after=%t", before.Evergreen, after.Evergreen)
	}
}

// TestSetJudgeRaceFree exercises concurrent Score() + SetJudge() — the exact
// startup window cmd/core has (connector goroutines scoring while the bridge
// judge is late-bound). It MUST be clean under `go test -race`; a plain mutable
// field would trip the race detector here.
func TestSetJudgeRaceFree(t *testing.T) {
	s := NewScorer(EvergreenConfig{JudgmentSource: JudgmentSourceScenario, ConfidenceFloor: 0.6})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = s.Score(context.Background(), EvergreenCandidate{ArtifactID: "x", SourceKind: "gmail"})
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			s.SetJudge(&fakeJudge{decision: EvergreenDecision{IsEvergreen: true, Confidence: 0.9}})
		}
	}()
	wg.Wait()
}
