package contracts

import (
	"strings"
	"testing"
)

// TestCanonicalRefusalBodyFor_EachCauseHasExactBody proves every
// RefusalCause in AllRefusalCauses maps to its exact user-facing body.
//
// BUG-061-009: relocated here from provenance/gate_test.go's
// TestEnforceRefusal_EachCauseHasExactBody when the dead
// provenance.EnforceRefusal path was removed. The cause→body mapping is
// a pure-function contract that belongs at the contracts layer, where
// CanonicalRefusalBodyFor lives — not behind a dead gate wrapper.
//
// Adversarial: if any cause is silently aliased to the default body in a
// refactor, that case fails. Every body is HONEST — none contains the
// band-low "saved as an idea" capture phrase, because a high-band
// refusal is never a capture (INV-HB-REFUSAL).
func TestCanonicalRefusalBodyFor_EachCauseHasExactBody(t *testing.T) {
	wantBodies := map[RefusalCause]string{
		RefusalBudgetExhausted:         "I couldn't complete that within the answer budget.",
		RefusalToolUnavailable:         "A tool I needed isn't available right now.",
		RefusalFabricatedSourceBlocked: "I couldn't verify the sources I would have cited.",
		RefusalInternalOnlyRestricted:  "That requires looking outside your knowledge graph, which is disabled.",
		RefusalAmbiguousNotClarified:   "I couldn't decide what to look up.",
		RefusalDefault:                 "I don't have a sourced answer for that.",
	}
	// Closed-vocabulary guard: the table MUST cover every declared cause
	// exactly (adds/removals to AllRefusalCauses force a table update).
	if len(wantBodies) != len(AllRefusalCauses) {
		t.Fatalf("wantBodies covers %d causes; AllRefusalCauses has %d — update the table when a cause is added/removed", len(wantBodies), len(AllRefusalCauses))
	}
	for _, cause := range AllRefusalCauses {
		want, ok := wantBodies[cause]
		if !ok {
			t.Fatalf("cause %q is in AllRefusalCauses but not covered by wantBodies", cause)
		}
		got := CanonicalRefusalBodyFor(cause)
		if got != want {
			t.Errorf("CanonicalRefusalBodyFor(%q) = %q; want %q", cause, got, want)
		}
		if got == "" {
			t.Errorf("CanonicalRefusalBodyFor(%q) is empty; the contract must be total", cause)
		}
		if strings.Contains(got, "saved as an idea") || strings.Contains(got, "saved as idea") {
			t.Errorf("CanonicalRefusalBodyFor(%q) = %q contains the band-low capture phrase; a high-band refusal must read honestly", cause, got)
		}
	}
}

// TestCanonicalRefusalBodyFor_AdversarialDefault — an unrecognised cause
// MUST fall back to the default honest body; the contract is total and
// never returns "". Relocated from provenance/gate_test.go's
// TestEnforceRefusal_AdversarialDefault.
func TestCanonicalRefusalBodyFor_AdversarialDefault(t *testing.T) {
	got := CanonicalRefusalBodyFor(RefusalCause("definitely_not_a_real_cause"))
	if got == "" {
		t.Fatal("CanonicalRefusalBodyFor returned empty for an unknown cause — contract is not total")
	}
	if got != CanonicalRefusalBodyFor(RefusalDefault) {
		t.Fatalf("unknown cause did not fall back to the default body: got %q", got)
	}
}
