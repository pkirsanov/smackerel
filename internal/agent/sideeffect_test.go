package agent

import "testing"

// Exhaustive switch: every constant returned by AllSideEffectClasses must
// be Valid(), have a distinct Rank(), and the rank ordering must be
// read < write < external (spec 037 §3.2).
func TestSideEffectClass_Exhaustive(t *testing.T) {
	classes := AllSideEffectClasses()
	if len(classes) != 3 {
		t.Fatalf("expected 3 side-effect classes, got %d", len(classes))
	}

	wantRanks := map[SideEffectClass]int{
		SideEffectRead:     0,
		SideEffectWrite:    1,
		SideEffectExternal: 2,
	}

	seen := make(map[SideEffectClass]bool)
	for _, c := range classes {
		if !c.Valid() {
			t.Errorf("class %q from AllSideEffectClasses() is not Valid()", c)
		}
		if seen[c] {
			t.Errorf("class %q duplicated in AllSideEffectClasses()", c)
		}
		seen[c] = true
		if got, want := c.Rank(), wantRanks[c]; got != want {
			t.Errorf("class %q rank=%d want %d", c, got, want)
		}
	}

	// Ordering invariant.
	if SideEffectRead.Rank() >= SideEffectWrite.Rank() ||
		SideEffectWrite.Rank() >= SideEffectExternal.Rank() {
		t.Errorf("side-effect ordering violated: read=%d write=%d external=%d",
			SideEffectRead.Rank(), SideEffectWrite.Rank(), SideEffectExternal.Rank())
	}

	// Unknown class.
	if SideEffectClass("purge").Valid() {
		t.Error(`SideEffectClass("purge") must not be Valid()`)
	}
	if SideEffectClass("purge").Rank() != -1 {
		t.Errorf(`SideEffectClass("purge").Rank() must be -1`)
	}
}
