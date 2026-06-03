// Spec 076 SCOPE-4a — unit coverage for LookupNLRouting.
//
// Pure-function tests of the NL find / NL rate classifier the facade
// consults between the slash-shortcut step and the reference-resolution
// step. The wiring proof (facade actually consults this and routes the
// turn accordingly) lives in facade_nl_routing_test.go alongside the
// other facade unit tests.

package assistant

import "testing"

func TestLookupNLRouting_NLFindRoutesToRetrievalQA(t *testing.T) {
	cases := []string{
		"find me notes about ACL tags",
		"Find me notes about ACL tags",
		"find my notes from last week",
		"search for the burger place",
		"look for the meeting summary",
		"find ACL tags",
		"search ACL tags",
		"  find me   ACL tags  ",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			hit, ok := LookupNLRouting(in)
			if !ok {
				t.Fatalf("LookupNLRouting(%q) ok=false; want true", in)
			}
			if hit.ScenarioID != "retrieval_qa" {
				t.Errorf("ScenarioID = %q; want retrieval_qa", hit.ScenarioID)
			}
			if hit.RateDisambig {
				t.Errorf("RateDisambig = true; want false for find pattern")
			}
		})
	}
}

func TestLookupNLRouting_NLRateAmbiguousTargetTriggersDisambig(t *testing.T) {
	cases := []string{
		"rate that 8 out of 10",
		"rate this 5/5",
		"rate it amazing",
		"Rate THIS 9/10",
		"rate them all great",
		"rate these",
		"rate those",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			hit, ok := LookupNLRouting(in)
			if !ok {
				t.Fatalf("LookupNLRouting(%q) ok=false; want true", in)
			}
			if hit.ScenarioID != "" {
				t.Errorf("ScenarioID = %q; want empty for rate-disambig", hit.ScenarioID)
			}
			if !hit.RateDisambig {
				t.Errorf("RateDisambig = false; want true for rate pattern")
			}
		})
	}
}

func TestLookupNLRouting_NonRoutedTextReturnsFalse(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"hello there",
		"findings from the report", // findings is a different word
		"finding common ground",    // "finding" not "find"
		"search",                   // bare prefix, no query body
		"find",                     // bare prefix
		"rate",                     // bare rate
		"rate the burger as 8/10",  // named target, not ambiguous
		"please rate this",         // "rate" is not token 0
		"/find ACL tags",           // legacy slash; handled by alias interceptor
		"/rate that 8 out of 10",   // legacy slash
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			hit, ok := LookupNLRouting(in)
			if ok {
				t.Errorf("LookupNLRouting(%q) ok=true (%+v); want false", in, hit)
			}
		})
	}
}
