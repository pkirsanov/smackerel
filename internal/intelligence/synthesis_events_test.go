package intelligence

import "testing"

func TestNonNilSynthesisSourceClasses_NormalizesNilAndPreservesNonNil(t *testing.T) {
	normalized := nonNilSynthesisSourceClasses(nil)
	if normalized == nil || len(normalized) != 0 {
		t.Fatalf("nil source classes normalized to %#v, want non-nil empty slice", normalized)
	}

	original := []string{"canonical-graph", "optional-source"}
	preserved := nonNilSynthesisSourceClasses(original)
	if len(preserved) != len(original) || &preserved[0] != &original[0] {
		t.Fatalf("non-nil source classes were replaced: got %#v, want original slice %#v", preserved, original)
	}
}
