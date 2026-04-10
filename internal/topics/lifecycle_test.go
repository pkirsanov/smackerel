package topics

import (
	"testing"
)

func TestCalculateMomentum(t *testing.T) {
	cfg := DefaultMomentumConfig()

	// Active topic: 5 captures in 30d, 10 in 90d, 3 search hits, 1 star, 4 connections, 2 days since active
	m := CalculateMomentum(5, 10, 3, 1, 4, 2, cfg)
	// raw = 5*3 + 10*1 + 3*2 + 1*5 + 4*0.5 = 15+10+6+5+2 = 38
	// decay = exp(-0.02 * 2) ≈ 0.9608
	// momentum ≈ 38 * 0.9608 ≈ 36.51
	if m < 35 || m > 40 {
		t.Errorf("expected momentum between 35-40, got %.2f", m)
	}
}

func TestCalculateMomentum_Dormant(t *testing.T) {
	cfg := DefaultMomentumConfig()

	// Dormant: 0 captures, 0 searches, 0 stars, 0 connections, 100 days inactive
	m := CalculateMomentum(0, 0, 0, 0, 0, 100, cfg)
	if m != 0 {
		t.Errorf("expected momentum 0 for dormant topic, got %.2f", m)
	}
}

func TestCalculateMomentum_Decay(t *testing.T) {
	cfg := DefaultMomentumConfig()

	// Same captures, but 30 days inactive vs 0 days
	m1 := CalculateMomentum(5, 10, 3, 1, 2, 0, cfg)
	m2 := CalculateMomentum(5, 10, 3, 1, 2, 30, cfg)

	if m2 >= m1 {
		t.Errorf("expected decayed momentum (%.2f) to be less than fresh (%.2f)", m2, m1)
	}
}

func TestTransitionState(t *testing.T) {
	tests := []struct {
		name     string
		current  State
		momentum float64
		expected State
	}{
		{"emerging to hot", StateEmerging, 55.0, StateHot},
		{"emerging to active", StateEmerging, 10.0, StateActive},
		{"hot to cooling", StateHot, 3.0, StateCooling},
		{"cooling to dormant", StateCooling, 0.5, StateDormant},
		{"emerging stays emerging", StateEmerging, 3.0, StateEmerging},
		{"hot stays hot", StateHot, 60.0, StateHot},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TransitionState(tt.current, tt.momentum)
			if got != tt.expected {
				t.Errorf("TransitionState(%s, %.1f) = %s, want %s", tt.current, tt.momentum, got, tt.expected)
			}
		})
	}
}

func TestDefaultMomentumConfig(t *testing.T) {
	cfg := DefaultMomentumConfig()
	if cfg.CaptureWeight30d != 3.0 {
		t.Errorf("expected CaptureWeight30d=3.0, got %.1f", cfg.CaptureWeight30d)
	}
	if cfg.DecayFactor != 0.02 {
		t.Errorf("expected DecayFactor=0.02, got %.2f", cfg.DecayFactor)
	}
	if cfg.StarWeight != 5.0 {
		t.Errorf("expected StarWeight=5.0, got %.1f", cfg.StarWeight)
	}
	if cfg.ConnectionWeight != 0.5 {
		t.Errorf("expected ConnectionWeight=0.5, got %.1f", cfg.ConnectionWeight)
	}
}

func TestTransitionState_ActiveToCooling(t *testing.T) {
	got := TransitionState(StateActive, 3.0)
	if got != StateCooling {
		t.Errorf("expected cooling when active drops to 3.0, got %s", got)
	}
}

func TestTransitionState_DormantStays(t *testing.T) {
	got := TransitionState(StateDormant, 0.0)
	if got != StateDormant {
		t.Errorf("expected dormant to stay dormant at 0.0, got %s", got)
	}
}

func TestTransitionState_EmergingToDormant(t *testing.T) {
	got := TransitionState(StateEmerging, 0.0)
	if got != StateDormant {
		t.Errorf("expected emerging to go dormant at 0.0, got %s", got)
	}
}

func TestTransitionState_CoolingToActive(t *testing.T) {
	// If momentum recovers while cooling
	got := TransitionState(StateCooling, 12.0)
	if got != StateActive {
		t.Errorf("expected cooling to recover to active at 12.0, got %s", got)
	}
}

func TestTransitionState_AllBoundaries(t *testing.T) {
	// Test exact boundary values
	tests := []struct {
		current  State
		momentum float64
		expected State
	}{
		{StateEmerging, 51.0, StateHot},     // above hot threshold (>50)
		{StateEmerging, 50.0, StateActive},  // exactly 50 is not >50
		{StateEmerging, 10.0, StateActive},  // exact active threshold
		{StateEmerging, 9.9, StateEmerging}, // just below active
		{StateEmerging, 3.0, StateEmerging},
		{StateHot, 50.0, StateActive}, // hot drops below 50
		{StateHot, 9.9, StateCooling},
		{StateCooling, 0.9, StateDormant},
	}

	for _, tt := range tests {
		got := TransitionState(tt.current, tt.momentum)
		if got != tt.expected {
			t.Errorf("TransitionState(%s, %.1f) = %s, want %s",
				tt.current, tt.momentum, got, tt.expected)
		}
	}
}

func TestCalculateMomentum_ZeroDecay(t *testing.T) {
	cfg := MomentumConfig{
		CaptureWeight30d: 1.0,
		CaptureWeight90d: 1.0,
		SearchWeight30d:  1.0,
		StarWeight:       0.0,
		ConnectionWeight: 0.0,
		DecayFactor:      0.0, // No decay
	}

	m := CalculateMomentum(5, 5, 5, 0, 0, 100, cfg)
	if m != 15.0 {
		t.Errorf("expected 15.0 with zero decay, got %.2f", m)
	}
}

func TestCalculateMomentum_HighDecay(t *testing.T) {
	cfg := MomentumConfig{
		CaptureWeight30d: 1.0,
		CaptureWeight90d: 1.0,
		SearchWeight30d:  1.0,
		StarWeight:       0.0,
		ConnectionWeight: 0.0,
		DecayFactor:      1.0, // Extreme decay
	}

	m1 := CalculateMomentum(5, 5, 5, 0, 0, 0, cfg)
	m2 := CalculateMomentum(5, 5, 5, 0, 0, 5, cfg)
	if m2 >= m1 {
		t.Errorf("high decay should reduce momentum significantly")
	}
}

func TestCalculateMomentum_StarsAndConnections(t *testing.T) {
	cfg := DefaultMomentumConfig()

	// Same captures but with stars and connections vs without
	mWithout := CalculateMomentum(5, 10, 3, 0, 0, 0, cfg)
	mWith := CalculateMomentum(5, 10, 3, 3, 10, 0, cfg)

	// Stars add 3*5=15, connections add 10*0.5=5 → 20 more raw
	if mWith <= mWithout {
		t.Errorf("expected stars+connections to increase momentum, without=%.2f, with=%.2f", mWithout, mWith)
	}
	diff := mWith - mWithout
	if diff < 19 || diff > 21 {
		t.Errorf("expected ~20 difference from stars+connections, got %.2f", diff)
	}
}

func TestStateConstants(t *testing.T) {
	states := []State{StateEmerging, StateActive, StateHot, StateCooling, StateDormant, StateArchived}
	seen := make(map[State]bool)
	for _, s := range states {
		if s == "" {
			t.Error("state constant should not be empty")
		}
		if seen[s] {
			t.Errorf("duplicate state: %s", s)
		}
		seen[s] = true
	}
	if len(states) != 6 {
		t.Errorf("expected 6 states, got %d", len(states))
	}
}

func TestNewLifecycle(t *testing.T) {
	l := NewLifecycle(nil)
	if l == nil {
		t.Fatal("expected non-nil lifecycle")
	}
	cfg := l.Config
	if cfg.CaptureWeight30d != 3.0 {
		t.Errorf("expected default config")
	}
}
