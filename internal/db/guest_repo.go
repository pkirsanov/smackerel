package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// GuestNode represents a guest in the hospitality knowledge graph.
type GuestNode struct {
	ID             string
	Email          string
	Name           string
	Source         string
	TotalStays     int
	TotalSpend     float64
	AvgRating      *float64
	SentimentScore *float64
	FirstStayAt    *time.Time
	LastStayAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// GuestRepository manages guest nodes in the hospitality graph.
type GuestRepository struct {
	Pool *pgxpool.Pool
}

// NewGuestRepository creates a new GuestRepository.
func NewGuestRepository(pool *pgxpool.Pool) *GuestRepository {
	return &GuestRepository{Pool: pool}
}

// UpsertByEmail inserts or updates a guest by email+source, returning the node.
func (r *GuestRepository) UpsertByEmail(ctx context.Context, email, name, source string) (*GuestNode, error) {
	id := ulid.Make().String()
	var g GuestNode
	err := r.Pool.QueryRow(ctx, `
		INSERT INTO guests (id, email, name, source)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email, source) DO UPDATE SET
			name = CASE WHEN EXCLUDED.name != '' THEN EXCLUDED.name ELSE guests.name END,
			updated_at = NOW()
		RETURNING id, email, name, source, total_stays, total_spend,
		          avg_rating, sentiment_score, first_stay_at, last_stay_at,
		          created_at, updated_at
	`, id, email, name, source).Scan(
		&g.ID, &g.Email, &g.Name, &g.Source,
		&g.TotalStays, &g.TotalSpend,
		&g.AvgRating, &g.SentimentScore,
		&g.FirstStayAt, &g.LastStayAt,
		&g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert guest by email: %w", err)
	}
	return &g, nil
}

// FindByEmail looks up a guest by email address.
func (r *GuestRepository) FindByEmail(ctx context.Context, email string) (*GuestNode, error) {
	var g GuestNode
	err := r.Pool.QueryRow(ctx, `
		SELECT id, email, name, source, total_stays, total_spend,
		       avg_rating, sentiment_score, first_stay_at, last_stay_at,
		       created_at, updated_at
		FROM guests WHERE email = $1
	`, email).Scan(
		&g.ID, &g.Email, &g.Name, &g.Source,
		&g.TotalStays, &g.TotalSpend,
		&g.AvgRating, &g.SentimentScore,
		&g.FirstStayAt, &g.LastStayAt,
		&g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("find guest by email: %w", err)
	}
	return &g, nil
}

// IncrementStay increments stay count and adds spend for a guest.
func (r *GuestRepository) IncrementStay(ctx context.Context, id string, spend float64) error {
	_, err := r.Pool.Exec(ctx, `
		UPDATE guests SET
			total_stays = total_stays + 1,
			total_spend = total_spend + $2,
			last_stay_at = NOW(),
			first_stay_at = COALESCE(first_stay_at, NOW()),
			updated_at = NOW()
		WHERE id = $1
	`, id, spend)
	if err != nil {
		return fmt.Errorf("increment guest stay: %w", err)
	}
	return nil
}

// UpdateSentiment sets the sentiment score for a guest.
func (r *GuestRepository) UpdateSentiment(ctx context.Context, id string, score float64) error {
	_, err := r.Pool.Exec(ctx, `
		UPDATE guests SET sentiment_score = $2, updated_at = NOW()
		WHERE id = $1
	`, id, score)
	if err != nil {
		return fmt.Errorf("update guest sentiment: %w", err)
	}
	return nil
}
