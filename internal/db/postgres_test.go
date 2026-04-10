package db

import (
	"context"
	"strings"
	"testing"
)

func TestConnect_InvalidURL(t *testing.T) {
	ctx := context.Background()
	_, err := Connect(ctx, "not-a-valid-url")
	if err == nil {
		t.Fatal("expected error for invalid database URL")
	}
	if !strings.Contains(err.Error(), "parse database url") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestConnect_EmptyURL(t *testing.T) {
	ctx := context.Background()
	_, err := Connect(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty database URL")
	}
}

func TestConnect_MalformedPostgresURL(t *testing.T) {
	ctx := context.Background()
	// Valid scheme but unreachable host — should fail on connect/ping, not parse
	_, err := Connect(ctx, "postgres://user:pass@localhost:99999/db")
	if err == nil {
		t.Fatal("expected error for unreachable database")
	}
}

func TestConnect_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err := Connect(ctx, "postgres://user:pass@localhost:5432/testdb")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestPostgres_Close(t *testing.T) {
	// Verify Close doesn't panic on a nil-ish pool scenario.
	// In production, the pool is always non-nil after Connect succeeds,
	// but this documents the contract.
	// We can't create a real pool without a DB, so this is a compile-check.
	var _ interface{ Close() } = &Postgres{}
}

func TestHealthy_Interface(t *testing.T) {
	// Verify Postgres implements the expected health-check interface shape.
	// This is a compile-time contract check.
	type healthChecker interface {
		Healthy(ctx context.Context) bool
	}
	var _ healthChecker = &Postgres{}
}

func TestArtifactCount_Interface(t *testing.T) {
	// Verify Postgres implements the expected artifact count interface.
	type counter interface {
		ArtifactCount(ctx context.Context) (int64, error)
	}
	var _ counter = &Postgres{}
}
