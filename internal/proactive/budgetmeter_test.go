package proactive

import "testing"

// TestReadBudgetMeter_ExhaustedIsExplicit covers SCN-107-008: when no budget
// remains the meter reports exhaustion explicitly with an honest "N of M used
// today" render, never a hidden default.
func TestReadBudgetMeter_ExhaustedIsExplicit(t *testing.T) {
	m := ReadBudgetMeter(0, 5)
	if !m.Exhausted {
		t.Fatalf("Exhausted = false, want true when remaining == 0")
	}
	if m.Used != 5 || m.Total != 5 {
		t.Errorf("Used/Total = %d/%d, want 5/5", m.Used, m.Total)
	}
	if m.Display != "5 of 5 used today" {
		t.Errorf("Display = %q, want %q", m.Display, "5 of 5 used today")
	}
}

func TestReadBudgetMeter_PartialUsage(t *testing.T) {
	m := ReadBudgetMeter(3, 5)
	if m.Exhausted {
		t.Errorf("Exhausted = true, want false when budget remains")
	}
	if m.Used != 2 {
		t.Errorf("Used = %d, want 2", m.Used)
	}
	if m.Display != "2 of 5 used today" {
		t.Errorf("Display = %q, want %q", m.Display, "2 of 5 used today")
	}
}

func TestReadBudgetMeter_FreshDay(t *testing.T) {
	m := ReadBudgetMeter(5, 5)
	if m.Exhausted {
		t.Errorf("Exhausted = true on a fresh day, want false")
	}
	if m.Used != 0 {
		t.Errorf("Used = %d, want 0", m.Used)
	}
}

// TestReadBudgetMeter_Clamps proves transient over/under counts never render a
// nonsensical meter.
func TestReadBudgetMeter_Clamps(t *testing.T) {
	// remaining above total clamps to total (0 used).
	over := ReadBudgetMeter(9, 5)
	if over.Used != 0 || over.Exhausted {
		t.Errorf("over-remaining meter = %+v, want Used=0 Exhausted=false", over)
	}
	// negative remaining clamps to 0 (exhausted).
	neg := ReadBudgetMeter(-3, 5)
	if neg.Used != 5 || !neg.Exhausted {
		t.Errorf("negative-remaining meter = %+v, want Used=5 Exhausted=true", neg)
	}
}
