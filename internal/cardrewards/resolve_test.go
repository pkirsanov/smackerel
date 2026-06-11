package cardrewards

import "testing"

// fixtureCatalog is a small in-memory catalog used by the resolver unit tests.
// It includes the B02 card (Citi Custom Cash) and two cards that share the
// "freedom" alias token for the B03 ambiguity scenario.
func fixtureCatalog() []CatalogCard {
	return []CatalogCard{
		{ID: "citi-custom-cash", Name: "Citi Custom Cash", Aliases: []string{"citi custom cash", "custom cash"}},
		{ID: "chase-freedom-flex", Name: "Chase Freedom Flex", Aliases: []string{"freedom flex", "chase freedom"}},
		{ID: "chase-freedom-unlimited", Name: "Chase Freedom Unlimited", Aliases: []string{"freedom unlimited", "chase freedom"}},
		{ID: "discover-it", Name: "Discover it Cash Back", Aliases: []string{"discover", "discover it"}},
	}
}

// SCN-083-B02 — resolution returns the catalog top candidate for free text.
func TestResolveCard_TopCandidate_B02(t *testing.T) {
	got := ResolveCard("custom cash", fixtureCatalog())
	if len(got) == 0 {
		t.Fatalf("expected at least one candidate for %q, got none", "custom cash")
	}
	if got[0].CardID != "citi-custom-cash" {
		t.Fatalf("top candidate = %q, want citi-custom-cash; full=%+v", got[0].CardID, got)
	}
	// "custom cash" is an exact alias of Citi Custom Cash → score 1.0.
	if got[0].Score != scoreExact || got[0].MatchType != matchExactAlias {
		t.Fatalf("top candidate match = (%v, %s), want (%v, %s)", got[0].Score, got[0].MatchType, scoreExact, matchExactAlias)
	}
}

// SCN-083-B02 (case/whitespace robustness) — normalization makes mixed case and
// extra whitespace resolve identically.
func TestResolveCard_NormalizationRobust_B02(t *testing.T) {
	got := ResolveCard("  Citi   Custom   Cash  ", fixtureCatalog())
	if len(got) == 0 || got[0].CardID != "citi-custom-cash" {
		t.Fatalf("normalized input did not resolve to citi-custom-cash; got=%+v", got)
	}
	if got[0].Score != scoreExact {
		t.Fatalf("expected exact-match score for normalized exact alias, got %v", got[0].Score)
	}
}

// SCN-083-B03 — ambiguous text matching a shared alias token returns more than
// one candidate for disambiguation.
func TestResolveCard_Ambiguous_B03(t *testing.T) {
	got := ResolveCard("freedom", fixtureCatalog())
	if len(got) < 2 {
		t.Fatalf("expected >= 2 candidates for ambiguous %q, got %d: %+v", "freedom", len(got), got)
	}
	// Both Chase Freedom cards must appear among the candidates.
	seen := map[string]bool{}
	for _, c := range got {
		seen[c.CardID] = true
	}
	if !seen["chase-freedom-flex"] || !seen["chase-freedom-unlimited"] {
		t.Fatalf("ambiguous resolution missing one of the freedom cards; got=%+v", got)
	}
}

// SCN-083-B03 (exact alias still wins under ambiguity) — when an exact alias is
// shared by two cards ("chase freedom"), BOTH surface as exact (1.0) candidates,
// so the caller must disambiguate rather than the resolver guessing.
func TestResolveCard_SharedExactAlias_B03(t *testing.T) {
	got := ResolveCard("chase freedom", fixtureCatalog())
	exactCount := 0
	for _, c := range got {
		if c.Score == scoreExact && c.MatchType == matchExactAlias {
			exactCount++
		}
	}
	if exactCount < 2 {
		t.Fatalf("expected >= 2 exact-alias candidates for shared alias %q, got %d: %+v", "chase freedom", exactCount, got)
	}
}

// Empty / whitespace-only input yields no candidates (boundary).
func TestResolveCard_EmptyInput(t *testing.T) {
	if got := ResolveCard("   ", fixtureCatalog()); got != nil {
		t.Fatalf("expected nil candidates for blank input, got %+v", got)
	}
}

// Unrelated text below the noise floor yields no candidates (adversarial: the
// resolver must NOT mismap a totally unrelated phrase to a real card).
func TestResolveCard_UnrelatedInputDropped(t *testing.T) {
	got := ResolveCard("zzqq totally unrelated phrase", fixtureCatalog())
	if len(got) != 0 {
		t.Fatalf("expected no candidates for unrelated input, got %+v", got)
	}
}

// Candidates are ranked best-first and capped at one per card.
func TestResolveCard_RankedAndDeduped(t *testing.T) {
	got := ResolveCard("discover it", fixtureCatalog())
	if len(got) == 0 || got[0].CardID != "discover-it" {
		t.Fatalf("expected discover-it as top candidate, got %+v", got)
	}
	counts := map[string]int{}
	var prev float64 = 2 // scores are <= 1.0, so this forces a real descending check
	for _, c := range got {
		counts[c.CardID]++
		if c.Score > prev {
			t.Fatalf("candidates not sorted descending by score: %+v", got)
		}
		prev = c.Score
	}
	for id, n := range counts {
		if n != 1 {
			t.Fatalf("card %s produced %d candidates, want exactly 1", id, n)
		}
	}
}
