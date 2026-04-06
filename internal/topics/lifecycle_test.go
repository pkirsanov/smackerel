package topics

import (
	"testing"
)

func TestCalculateMomentum(t *testing.T) {
	cfg := DefaultMomentumConfig()

	// Active topic: 5 captures in 30d, 10 in 90d, 3 search hits, 2 days since active
	m := CalculateMomentum(5, 10, 3, 2, cfg)
	if m < 20 || m > 35 {
		t.Errorf("expected momentum between 20-35, got %.2f", m)
	}
}

func TestCalculateMomentum_Dormant(t *testing.T) {
	cfg := DefaultMomentumConfig()

	// Dormant: 0 captures, 0 searches, 100 days inactive
	m := CalculateMomentum(0, 0, 0, 100, cfg)
	if m != 0 {
		t.Errorf("expected momentum 0 for dormant topic, got %.2f", m)
	}
}

func TestCalculateMomentum_Decay(t *testing.T) {
	cfg := DefaultMomentumConfig()

	// Same captures, but 30 days inactive vs 0 days
	m1 := CalculateMomentum(5, 10, 3, 0, cfg)
	m2 := CalculateMomentum(5, 10, 3, 30, cfg)

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
		{"emerging to hot", StateEmerging, 15.0, StateHot},
		{"emerging to active", StateEmerging, 8.0, StateActive},
		{"hot to cooling", StateHot, 3.0, StateCooling},
		{"cooling to dormant", StateCooling, 0.5, StateDormant},
		{"emerging stays emerging", StateEmerging, 3.0, StateEmerging},
		{"hot stays hot", StateHot, 20.0, StateHot},
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
	if cfg.DecayFactor != 0.1 {
		t.Errorf("expected DecayFactor=0.1, got %.1f", cfg.DecayFactor)
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
	got := TransitionState(StateCooling, 10.0)
	if got != StateActive {
		t.Errorf("expected cooling to recover to active at 10.0, got %s", got)
	}
}

func TestTransitionState_AllBoundaries(t *testing.T) {
	// Test exact boundary values
	tests := []struct {
		current  State
		momentum float64
		expected State
	}{
		{StateEmerging, 15.0, StateHot},    // exact hot threshold
		{StateEmerging, 14.9, StateActive}, // just below hot
		{StateEmerging, 8.0, StateActive},  // exact active threshold
		{StateEmerging, 7.9, StateEmerging},
		{StateEmerging, 3.0, StateEmerging},
		{StateHot, 14.9, StateActive},
		{StateHot, 7.9, StateCooling},
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
		DecayFactor:      0.0, // No decay
	}

	m := CalculateMomentum(5, 5, 5, 100, cfg)
	if m != 15.0 {
		t.Errorf("expected 15.0 with zero decay, got %.2f", m)
	}
}

func TestCalculateMomentum_HighDecay(t *testing.T) {
	cfg := MomentumConfig{
		CaptureWeight30d: 1.0,
		CaptureWeight90d: 1.0,
		SearchWeight30d:  1.0,
		DecayFactor:      1.0, // Extreme decay
	}

	m1 := CalculateMomentum(5, 5, 5, 0, cfg)
	m2 := CalculateMomentum(5, 5, 5, 5, cfg)
	if m2 >= m1 {
		t.Errorf("high decay should reduce momentum significantly")
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
