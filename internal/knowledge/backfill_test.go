package knowledge

import (
	"testing"
	"time"
)

// T-BF-01: normalized() applies documented defaults without clobbering
// explicit non-zero values.
func TestSynthesisBackfillOptions_Normalized_Defaults(t *testing.T) {
	got := SynthesisBackfillOptions{}.normalized()
	if got.BatchSize != 100 {
		t.Errorf("BatchSize = %d, want default 100", got.BatchSize)
	}
	if got.Cooldown != 30*time.Minute {
		t.Errorf("Cooldown = %v, want default 30m", got.Cooldown)
	}
	if got.TriggeredBy != "backfill_cli" {
		t.Errorf("TriggeredBy = %q, want default backfill_cli", got.TriggeredBy)
	}
}

// T-BF-02: explicit values are preserved, not overwritten by defaults.
func TestSynthesisBackfillOptions_Normalized_PreservesExplicitValues(t *testing.T) {
	got := SynthesisBackfillOptions{
		BatchSize:   25,
		Cooldown:    5 * time.Minute,
		TriggeredBy: "custom_trigger",
	}.normalized()
	if got.BatchSize != 25 {
		t.Errorf("BatchSize = %d, want 25", got.BatchSize)
	}
	if got.Cooldown != 5*time.Minute {
		t.Errorf("Cooldown = %v, want 5m", got.Cooldown)
	}
	if got.TriggeredBy != "custom_trigger" {
		t.Errorf("TriggeredBy = %q, want custom_trigger", got.TriggeredBy)
	}
}

// T-BF-03: MaxArtifacts=0 must mean "no cap" per the documented contract —
// normalized() must never turn a real zero into a default, since 0 is a
// meaningful value here (unlike BatchSize/Cooldown/TriggeredBy).
func TestSynthesisBackfillOptions_Normalized_MaxArtifactsZeroMeansUncapped(t *testing.T) {
	got := SynthesisBackfillOptions{MaxArtifacts: 0}.normalized()
	if got.MaxArtifacts != 0 {
		t.Errorf("MaxArtifacts = %d, want 0 (uncapped) preserved by normalized()", got.MaxArtifacts)
	}
}

// T-BF-04: DryRun must be preserved as-is by normalized() (a bool default of
// false must not be conflated with "unset").
func TestSynthesisBackfillOptions_Normalized_PreservesDryRun(t *testing.T) {
	got := SynthesisBackfillOptions{DryRun: true}.normalized()
	if !got.DryRun {
		t.Error("DryRun = false, want true to survive normalized()")
	}
}
