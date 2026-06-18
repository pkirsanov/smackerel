// Spec 095 SCOPE-08 — pool-eligibility tests.
package evergreen

import "testing"

func evergreenSig(id string) EvergreenSignal {
	return EvergreenSignal{ArtifactID: id, Evergreen: true, Confidence: 0.9}
}
func ephemeralSig(id string) EvergreenSignal {
	return EvergreenSignal{ArtifactID: id, Evergreen: false, Confidence: 0.8}
}

func bothOn() PoolPolicy {
	return PoolPolicy{SynthesisExcludesLowEvergreen: true, DigestExcludesLowEvergreen: true}
}

// TestSynthesisPoolExcludesLowEvergreen — SCN-095-B03.
func TestSynthesisPoolExcludesLowEvergreen(t *testing.T) {
	p := bothOn()
	if IncludeInSynthesisPool(ephemeralSig("e"), p) {
		t.Error("low-evergreen artifact must be excluded from the synthesis pool")
	}
	if !IncludeInSynthesisPool(evergreenSig("g"), p) {
		t.Error("evergreen artifact must be included in the synthesis pool")
	}
}

// TestDigestPoolExcludesLowEvergreen — SCN-095-B04.
func TestDigestPoolExcludesLowEvergreen(t *testing.T) {
	p := bothOn()
	if IncludeInDigestPool(ephemeralSig("e"), p) {
		t.Error("low-evergreen artifact must be excluded from the digest pool")
	}
	if !IncludeInDigestPool(evergreenSig("g"), p) {
		t.Error("evergreen artifact must be included in the digest pool")
	}
}

// TestEphemeralStaysSearchable — SCN-095-B02 / R13 / Principle 9: an ephemeral
// item is excluded from the pools and routed to aggressive decay, but remains
// fully searchable (never hidden/deleted).
func TestEphemeralStaysSearchable(t *testing.T) {
	p := bothOn()
	e := ephemeralSig("e")

	if IncludeInSynthesisPool(e, p) || IncludeInDigestPool(e, p) {
		t.Error("ephemeral item must be excluded from synthesis + digest pools")
	}
	if !AggressiveDecay(e) {
		t.Error("ephemeral item must be routed to aggressive decay")
	}
	// The crux (R13): excluded from pools, but STILL searchable.
	if !Searchable(e) {
		t.Fatal("ephemeral item must remain fully searchable (Principle 9 — no punishment, no hiding)")
	}

	t.Run("would_catch_regression", func(t *testing.T) {
		// If a regression made Searchable() return false for ephemeral items
		// (hiding them), this assertion trips.
		if !Searchable(ephemeralSig("x")) {
			t.Fatal("regression: ephemeral item was made non-searchable (R13 violated)")
		}
		// An evergreen item is naturally never decayed aggressively.
		if AggressiveDecay(evergreenSig("g")) {
			t.Fatal("regression: an evergreen item was routed to aggressive decay")
		}
	})
}

// TestMixedPoolSelectiveExclusion — adversarial (no tautology): a pool with
// mixed evergreen/ephemeral fixtures proves exclusion is SELECTIVE — not all
// items excluded, not all included.
func TestMixedPoolSelectiveExclusion(t *testing.T) {
	p := bothOn()
	sigs := []EvergreenSignal{
		evergreenSig("g1"), ephemeralSig("e1"), evergreenSig("g2"), ephemeralSig("e2"), evergreenSig("g3"),
	}
	got := FilterSynthesisPool(sigs, p)
	if len(got) != 3 {
		t.Fatalf("expected 3 evergreen survivors, got %d (%v)", len(got), got)
	}
	for _, s := range got {
		if !s.Evergreen {
			t.Errorf("filtered pool should contain only evergreen items, found %s", s.ArtifactID)
		}
	}
	// Non-tautology: at least one was excluded AND at least one survived.
	if len(got) == 0 || len(got) == len(sigs) {
		t.Fatal("filter must be selective — not all-excluded and not all-included")
	}
}

// TestPolicyOffIncludesAll — when the SST switch is off, low-evergreen items
// are NOT excluded (the policy gate is respected, no silent exclusion).
func TestPolicyOffIncludesAll(t *testing.T) {
	off := PoolPolicy{SynthesisExcludesLowEvergreen: false, DigestExcludesLowEvergreen: false}
	if !IncludeInSynthesisPool(ephemeralSig("e"), off) {
		t.Error("with the synthesis switch off, low-evergreen items must NOT be excluded")
	}
	if !IncludeInDigestPool(ephemeralSig("e"), off) {
		t.Error("with the digest switch off, low-evergreen items must NOT be excluded")
	}
}

// TestPoolExclusionTrace — the §14.A pool_excluded token fires only on exclusion.
func TestPoolExclusionTrace(t *testing.T) {
	p := bothOn()
	if tok := PoolExclusionTrace("synthesis", ephemeralSig("e"), p); tok == "" {
		t.Error("excluded ephemeral item should emit a pool_excluded trace token")
	}
	if tok := PoolExclusionTrace("synthesis", evergreenSig("g"), p); tok != "" {
		t.Errorf("included evergreen item should emit no exclusion token, got %q", tok)
	}
}
