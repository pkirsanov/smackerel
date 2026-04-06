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
