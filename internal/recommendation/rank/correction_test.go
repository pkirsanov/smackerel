package rank

import "testing"

func TestActiveCorrectionBlocksMatchingPositiveBoost(t *testing.T) {
	corrections := []PreferenceCorrection{{ID: "corr-1", PreferenceKey: "loves_spicy", CorrectionKind: "remove"}}

	correction, ok := ActiveCorrectionForPreference("loves_spicy", corrections)
	if !ok {
		t.Fatal("matching active correction was not found")
	}
	if correction.ID != "corr-1" {
		t.Fatalf("correction ID = %q, want corr-1", correction.ID)
	}
	if PositiveBoostAllowed("loves_spicy", corrections) {
		t.Fatal("matching active correction allowed the positive boost")
	}
}

func TestActiveCorrectionDoesNotBlockUnrelatedPreference(t *testing.T) {
	corrections := []PreferenceCorrection{{ID: "corr-1", PreferenceKey: "loves_spicy", CorrectionKind: "remove"}}

	if !PositiveBoostAllowed("quiet_cafes", corrections) {
		t.Fatal("unrelated correction blocked quiet_cafes boost")
	}
}
