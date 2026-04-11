package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// PropertyNode represents a property in the hospitality knowledge graph.
type PropertyNode struct {
	ID            string
	ExternalID    string
	Source        string
	Name          string
	TotalBookings int
	TotalRevenue  float64
	AvgRating     *float64
	IssueCount    int
	Topics        []string
}

// PropertyRepository manages property nodes in the hospitality graph.
type PropertyRepository struct {
	Pool *pgxpool.Pool
}

// NewPropertyRepository creates a new PropertyRepository.
func NewPropertyRepository(pool *pgxpool.Pool) *PropertyRepository {
	return &PropertyRepository{Pool: pool}
}

// UpsertByExternalID inserts or updates a property by external_id+source, returning the node.
func (r *PropertyRepository) UpsertByExternalID(ctx context.Context, externalID, source, name string) (*PropertyNode, error) {
	id := ulid.Make().String()
	var p PropertyNode
	err := r.Pool.QueryRow(ctx, `
		INSERT INTO properties (id, external_id, source, name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (external_id, source) DO UPDATE SET
			name = CASE WHEN EXCLUDED.name != '' THEN EXCLUDED.name ELSE properties.name END,
			updated_at = NOW()
		RETURNING id, external_id, source, name, total_bookings, total_revenue,
		          avg_rating, issue_count, topics
	`, id, externalID, source, name).Scan(
		&p.ID, &p.ExternalID, &p.Source, &p.Name,
		&p.TotalBookings, &p.TotalRevenue,
		&p.AvgRating, &p.IssueCount, &p.Topics,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert property by external_id: %w", err)
	}
	return &p, nil
}

// IncrementBookings increments booking count and adds revenue for a property.
func (r *PropertyRepository) IncrementBookings(ctx context.Context, id string, revenue float64) error {
	_, err := r.Pool.Exec(ctx, `
		UPDATE properties SET
			total_bookings = total_bookings + 1,
			total_revenue = total_revenue + $2,
			updated_at = NOW()
		WHERE id = $1
	`, id, revenue)
	if err != nil {
		return fmt.Errorf("increment property bookings: %w", err)
	}
	return nil
}

// UpdateTopics sets the topic tags for a property.
func (r *PropertyRepository) UpdateTopics(ctx context.Context, id string, topics []string) error {
	_, err := r.Pool.Exec(ctx, `
		UPDATE properties SET topics = $2, updated_at = NOW()
		WHERE id = $1
	`, id, topics)
	if err != nil {
		return fmt.Errorf("update property topics: %w", err)
	}
	return nil
}

// FindByExternalID looks up a property by external_id.
func (r *PropertyRepository) FindByExternalID(ctx context.Context, externalID string) (*PropertyNode, error) {
	var p PropertyNode
	err := r.Pool.QueryRow(ctx, `
		SELECT id, external_id, source, name, total_bookings, total_revenue,
		       avg_rating, issue_count, topics
		FROM properties WHERE external_id = $1
	`, externalID).Scan(
		&p.ID, &p.ExternalID, &p.Source, &p.Name,
		&p.TotalBookings, &p.TotalRevenue,
		&p.AvgRating, &p.IssueCount, &p.Topics,
	)
	if err != nil {
		return nil, fmt.Errorf("find property by external_id: %w", err)
	}
	return &p, nil
}

// UpdateIssueCount adjusts the issue count for a property by a delta.
func (r *PropertyRepository) UpdateIssueCount(ctx context.Context, id string, delta int) error {
	_, err := r.Pool.Exec(ctx, `
		UPDATE properties SET
			issue_count = GREATEST(0, issue_count + $2),
			updated_at = NOW()
		WHERE id = $1
	`, id, delta)
	if err != nil {
		return fmt.Errorf("update property issue count: %w", err)
	}
	return nil
}
