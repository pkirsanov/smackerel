package topics

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"
)

// State represents a topic lifecycle state.
type State string

const (
	StateEmerging State = "emerging"
	StateActive   State = "active"
	StateHot      State = "hot"
	StateCooling  State = "cooling"
	StateDormant  State = "dormant"
	StateArchived State = "archived"
)

// MomentumConfig holds parameters for the momentum scoring formula (R-208).
type MomentumConfig struct {
	CaptureWeight30d float64 // weight for 30-day capture count
	CaptureWeight90d float64 // weight for 90-day capture count
	SearchWeight30d  float64 // weight for 30-day search hits
	DecayFactor      float64 // exponential decay factor
}

// DefaultMomentumConfig returns the default R-208 momentum parameters.
func DefaultMomentumConfig() MomentumConfig {
	return MomentumConfig{
		CaptureWeight30d: 3.0,
		CaptureWeight90d: 1.0,
		SearchWeight30d:  2.0,
		DecayFactor:      0.1,
	}
}

// CalculateMomentum computes the momentum score using the R-208 formula.
// momentum = (captures_30d * w1 + captures_90d * w2 + search_hits_30d * w3) * decay
func CalculateMomentum(captures30d, captures90d, searchHits30d int, daysSinceActive int, cfg MomentumConfig) float64 {
	raw := float64(captures30d)*cfg.CaptureWeight30d +
		float64(captures90d)*cfg.CaptureWeight90d +
		float64(searchHits30d)*cfg.SearchWeight30d

	decay := math.Exp(-cfg.DecayFactor * float64(daysSinceActive))
	return raw * decay
}

// TransitionState determines the next state based on momentum score.
func TransitionState(current State, momentum float64) State {
	switch {
	case momentum >= 15.0:
		return StateHot
	case momentum >= 8.0:
		return StateActive
	case momentum >= 3.0:
		if current == StateHot || current == StateActive {
			return StateCooling
		}
		return StateEmerging
	case momentum >= 1.0:
		if current == StateCooling || current == StateActive || current == StateHot {
			return StateCooling
		}
		return StateEmerging
	default:
		if current == StateCooling {
			return StateDormant
		}
		if current == StateEmerging {
			return StateDormant
		}
		return current
	}
}

// Lifecycle manages topic state transitions and momentum updates.
type Lifecycle struct {
	Pool   *pgxpool.Pool
	Config MomentumConfig
}

// NewLifecycle creates a new topic lifecycle manager.
func NewLifecycle(pool *pgxpool.Pool) *Lifecycle {
	return &Lifecycle{
		Pool:   pool,
		Config: DefaultMomentumConfig(),
	}
}

// UpdateAllMomentum recalculates momentum scores for all topics.
func (l *Lifecycle) UpdateAllMomentum(ctx context.Context) error {
	rows, err := l.Pool.Query(ctx, `
		SELECT id, name, state, capture_count_30d, capture_count_90d, search_hit_count_30d,
		       EXTRACT(DAY FROM NOW() - COALESCE(last_active, created_at))::int
		FROM topics
		WHERE state != 'archived'
	`)
	if err != nil {
		return fmt.Errorf("query topics: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, state string
		var cap30, cap90, search30, daysSince int

		if err := rows.Scan(&id, &name, &state, &cap30, &cap90, &search30, &daysSince); err != nil {
			slog.Warn("scan topic row", "error", err)
			continue
		}

		momentum := CalculateMomentum(cap30, cap90, search30, daysSince, l.Config)
		newState := TransitionState(State(state), momentum)

		if State(state) != newState || true { // Always update momentum_score
			_, err := l.Pool.Exec(ctx, `
				UPDATE topics SET momentum_score = $2, state = $3, updated_at = NOW()
				WHERE id = $1
			`, id, momentum, string(newState))
			if err != nil {
				slog.Warn("update topic momentum", "id", id, "error", err)
			}

			if State(state) != newState {
				slog.Info("topic state transition",
					"topic", name,
					"from", state,
					"to", newState,
					"momentum", momentum,
				)
			}
		}
	}

	return nil
}
