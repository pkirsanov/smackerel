package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres wraps a pgx connection pool.
type Postgres struct {
	Pool *pgxpool.Pool
}

// Connect creates a new PostgreSQL connection pool.
func Connect(ctx context.Context, databaseURL string, maxConns, minConns int32) (*Postgres, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	config.MaxConns = maxConns
	config.MinConns = minConns
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second // proactively detect stale connections

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	// Use an explicit timeout for the initial ping so startup doesn't hang
	// when the database is unresponsive (the caller's ctx may have no deadline).
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	slog.Info("connected to PostgreSQL", "host", config.ConnConfig.Host, "database", config.ConnConfig.Database)
	return &Postgres{Pool: pool}, nil
}

// Close shuts down the connection pool.
func (p *Postgres) Close() {
	p.Pool.Close()
}

// ArtifactCount returns the estimated number of rows in the artifacts table.
// Uses pg_class.reltuples (updated by ANALYZE/autovacuum) instead of COUNT(*)
// to avoid a full sequential scan on every health check (IMP-022-R30-003).
// The estimate is accurate within a few percent for tables with regular writes.
func (p *Postgres) ArtifactCount(ctx context.Context) (int64, error) {
	var count float64
	err := p.Pool.QueryRow(ctx,
		"SELECT COALESCE(reltuples, 0) FROM pg_class WHERE relname = 'artifacts'",
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("estimate artifact count: %w", err)
	}
	// reltuples can be -1 before the first ANALYZE; treat as 0
	if count < 0 {
		count = 0
	}
	return int64(count), nil
}

// Healthy checks if the database is reachable.
func (p *Postgres) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return p.Pool.Ping(ctx) == nil
}

// ExportResult holds exported artifacts and a cursor for pagination.
type ExportResult struct {
	Artifacts  []ExportedArtifact
	NextCursor time.Time // last created_at value; zero if no results
}

// ExportedArtifact is a type-safe representation of an exported artifact row.
type ExportedArtifact struct {
	ArtifactID   string `json:"artifact_id"`
	Title        string `json:"title"`
	ArtifactType string `json:"artifact_type"`
	Summary      string `json:"summary"`
	SourceURL    string `json:"source_url"`
	Content      string `json:"content"`
	Topics       string `json:"topics"`
	Entities     string `json:"entities"`
	KeyIdeas     string `json:"key_ideas"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// ExportArtifacts returns processed artifacts for data export with cursor-based pagination.
// cursor is the starting point (exclusive); use time.Time{} for the first page.
// limit is capped at 10000.
func (p *Postgres) ExportArtifacts(ctx context.Context, cursor time.Time, limit int) (*ExportResult, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}

	rows, err := p.Pool.Query(ctx, `
		SELECT id, title, artifact_type, COALESCE(summary, ''),
		       COALESCE(source_url, ''), COALESCE(content_raw, ''),
		       COALESCE(topics::text, '[]'),
		       COALESCE(entities::text, '{}'),
		       COALESCE(key_ideas::text, '[]'),
		       created_at, updated_at
		FROM artifacts
		WHERE processing_status = 'processed' AND created_at > $1
		ORDER BY created_at ASC
		LIMIT $2
	`, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("export query: %w", err)
	}
	defer rows.Close()

	var results []ExportedArtifact
	var lastCreatedAt time.Time
	var scanErrors int
	for rows.Next() {
		var id, title, artType, summary, sourceURL, content string
		var topicsStr, entitiesStr, keyIdeasStr string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &title, &artType, &summary, &sourceURL,
			&content, &topicsStr, &entitiesStr, &keyIdeasStr,
			&createdAt, &updatedAt); err != nil {
			scanErrors++
			slog.Warn("export scan error", "error", err, "scan_errors_so_far", scanErrors)
			continue
		}

		lastCreatedAt = createdAt
		results = append(results, ExportedArtifact{
			ArtifactID:   id,
			Title:        title,
			ArtifactType: artType,
			Summary:      summary,
			SourceURL:    sourceURL,
			Content:      content,
			Topics:       topicsStr,
			Entities:     entitiesStr,
			KeyIdeas:     keyIdeasStr,
			CreatedAt:    createdAt.Format(time.RFC3339),
			UpdatedAt:    updatedAt.Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		return &ExportResult{Artifacts: results, NextCursor: lastCreatedAt}, fmt.Errorf("export iteration: %w", err)
	}
	if scanErrors > 0 {
		return &ExportResult{Artifacts: results, NextCursor: lastCreatedAt}, fmt.Errorf("export completed with %d scan errors", scanErrors)
	}
	return &ExportResult{Artifacts: results, NextCursor: lastCreatedAt}, nil
}

// RecentArtifact is a summary of a recently captured artifact.
type RecentArtifact struct {
	ID           string
	Title        string
	ArtifactType string
	Summary      string
	CreatedAt    time.Time
}

// RecentArtifacts returns the most recently captured artifacts, ordered newest first.
func (p *Postgres) RecentArtifacts(ctx context.Context, limit int) ([]RecentArtifact, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := p.Pool.Query(ctx, `
		SELECT id, title, artifact_type, COALESCE(summary, ''), created_at
		FROM artifacts ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("recent artifacts query: %w", err)
	}
	defer rows.Close()

	var items []RecentArtifact
	var scanErrors int
	for rows.Next() {
		var a RecentArtifact
		if err := rows.Scan(&a.ID, &a.Title, &a.ArtifactType, &a.Summary, &a.CreatedAt); err != nil {
			scanErrors++
			slog.Warn("recent artifacts scan error", "error", err, "scan_errors_so_far", scanErrors)
			continue
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return items, fmt.Errorf("recent artifacts iteration: %w", err)
	}
	if scanErrors > 0 {
		return items, fmt.Errorf("recent artifacts completed with %d scan errors", scanErrors)
	}
	return items, nil
}

// ArtifactDetail is the full detail of a single artifact.
type ArtifactDetail struct {
	ID             string
	Title          string
	ArtifactType   string
	Summary        string
	SourceURL      string
	Sentiment      string
	SourceQuality  string
	ProcessingTier string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// GetArtifact retrieves a single artifact by ID.
func (p *Postgres) GetArtifact(ctx context.Context, id string) (*ArtifactDetail, error) {
	var a ArtifactDetail
	err := p.Pool.QueryRow(ctx, `
		SELECT id, title, artifact_type, COALESCE(summary, ''), COALESCE(source_url, ''),
		       COALESCE(sentiment, ''), COALESCE(source_quality, ''), COALESCE(processing_tier, ''),
		       created_at, updated_at
		FROM artifacts WHERE id = $1
	`, id).Scan(&a.ID, &a.Title, &a.ArtifactType, &a.Summary, &a.SourceURL,
		&a.Sentiment, &a.SourceQuality, &a.ProcessingTier,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get artifact: %w", err)
	}
	return &a, nil
}
