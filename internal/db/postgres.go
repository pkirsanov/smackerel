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
func Connect(ctx context.Context, databaseURL string) (*Postgres, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
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

// ArtifactCount returns the number of rows in the artifacts table.
func (p *Postgres) ArtifactCount(ctx context.Context) (int64, error) {
	var count int64
	err := p.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM artifacts").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count artifacts: %w", err)
	}
	return count, nil
}

// Healthy checks if the database is reachable.
func (p *Postgres) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return p.Pool.Ping(ctx) == nil
}
