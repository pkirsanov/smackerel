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
	StarWeight       float64 // weight for explicit star count
	ConnectionWeight float64 // weight for connection count
	DecayFactor      float64 // exponential decay factor
}

// DefaultMomentumConfig returns the default R-208 momentum parameters.
func DefaultMomentumConfig() MomentumConfig {
	return MomentumConfig{
		CaptureWeight30d: 3.0,
		CaptureWeight90d: 1.0,
		SearchWeight30d:  2.0,
		StarWeight:       5.0,
		ConnectionWeight: 0.5,
		DecayFactor:      0.02,
	}
}

// CalculateMomentum computes the momentum score using the R-208 formula.
// momentum = (captures_30d * 3.0 + captures_90d * 1.0 + search_hits_30d * 2.0 +
//
//	star_count * 5.0 + connection_count * 0.5) * exp(-0.02 * days_since_active)
func CalculateMomentum(captures30d, captures90d, searchHits30d, starCount, connectionCount int, daysSinceActive int, cfg MomentumConfig) float64 {
	raw := float64(captures30d)*cfg.CaptureWeight30d +
		float64(captures90d)*cfg.CaptureWeight90d +
		float64(searchHits30d)*cfg.SearchWeight30d +
		float64(starCount)*cfg.StarWeight +
		float64(connectionCount)*cfg.ConnectionWeight

	decay := math.Exp(-cfg.DecayFactor * float64(daysSinceActive))
	return raw * decay
}

// TransitionState determines the next state based on momentum score per R-208.
// Hot: momentum > 50, Active: momentum >= 10, Cooling: previously active but declining,
// Dormant: momentum < 1 for cooling/emerging states.
func TransitionState(current State, momentum float64) State {
	switch {
	case momentum > 50.0:
		return StateHot
	case momentum >= 10.0:
		if current == StateHot {
			return StateActive // Hot → Active: momentum drops below 50
		}
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
		SELECT t.id, t.name, t.state, t.capture_count_30d, t.capture_count_90d, t.search_hit_count_30d,
		       COALESCE(t.star_count, 0),
		       COALESCE((SELECT COUNT(*) FROM edges WHERE src_type = 'topic' AND src_id = t.id
		                  OR dst_type = 'topic' AND dst_id = t.id), 0)::int,
		       EXTRACT(DAY FROM NOW() - COALESCE(t.last_active, t.created_at))::int
		FROM topics t
		WHERE t.state != 'archived'
	`)
	if err != nil {
		return fmt.Errorf("query topics: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, state string
		var cap30, cap90, search30, starCount, connCount, daysSince int

		if err := rows.Scan(&id, &name, &state, &cap30, &cap90, &search30, &starCount, &connCount, &daysSince); err != nil {
			slog.Warn("scan topic row", "error", err)
			continue
		}

		momentum := CalculateMomentum(cap30, cap90, search30, starCount, connCount, daysSince, l.Config)
		newState := TransitionState(State(state), momentum)

		// Update momentum score in DB; skip write when state and momentum are unchanged.
		_, err := l.Pool.Exec(ctx, `
			UPDATE topics SET momentum_score = $2, state = $3, updated_at = NOW()
			WHERE id = $1 AND (state != $3 OR momentum_score IS DISTINCT FROM $2)
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

	return nil
}
