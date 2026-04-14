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
	if externalID == "" || len(externalID) > 255 {
		return nil, fmt.Errorf("invalid external_id: must be 1-255 chars, got %d", len(externalID))
	}
	if len(name) > 500 {
		name = name[:500]
	}
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
	if revenue < 0 {
		return fmt.Errorf("revenue must be non-negative: %f", revenue)
	}
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
// FindByExternalID looks up a property by external_id. If source is non-empty, scopes the lookup.
func (r *PropertyRepository) FindByExternalID(ctx context.Context, externalID string, source ...string) (*PropertyNode, error) {
	var p PropertyNode
	var query string
	var args []interface{}
	if len(source) > 0 && source[0] != "" {
		query = `SELECT id, external_id, source, name, total_bookings, total_revenue,
		       avg_rating, issue_count, topics
		FROM properties WHERE external_id = $1 AND source = $2`
		args = []interface{}{externalID, source[0]}
	} else {
		query = `SELECT id, external_id, source, name, total_bookings, total_revenue,
		       avg_rating, issue_count, topics
		FROM properties WHERE external_id = $1`
		args = []interface{}{externalID}
	}
	err := r.Pool.QueryRow(ctx, query, args...).Scan(
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
