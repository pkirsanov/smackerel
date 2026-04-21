//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// testPool returns a pgxpool connected to the test database.
// The pool is closed automatically when the test completes.
// Skips the test if DATABASE_URL is not set.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live stack not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping test database: %v", err)
	}

	t.Cleanup(func() { pool.Close() })
	return pool
}

// testNATSConn returns a NATS connection to the test NATS server.
// The connection is closed automatically when the test completes.
// Skips the test if NATS_URL is not set.
func testNATSConn(t *testing.T) *nats.Conn {
	t.Helper()

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("integration: NATS_URL not set — live stack not available")
	}

	opts := []nats.Option{
		nats.Name("smackerel-integration-test"),
	}

	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if authToken != "" {
		opts = append(opts, nats.Token(authToken))
	}

	nc, err := nats.Connect(natsURL, opts...)
	if err != nil {
		t.Fatalf("connect to test NATS: %v", err)
	}

	t.Cleanup(func() { nc.Close() })
	return nc
}

// testJetStream returns a JetStream context from the test NATS connection.
func testJetStream(t *testing.T) (jetstream.JetStream, *nats.Conn) {
	t.Helper()

	nc := testNATSConn(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}

	return js, nc
}

// testID returns a unique test-scoped identifier.
func testID(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano())
}

// cleanupArtifact registers cleanup to delete a test artifact and its edges.
// CHAOS-031-001: errors are logged instead of silently swallowed so stale
// test data is detectable rather than invisible.
func cleanupArtifact(t *testing.T, pool *pgxpool.Pool, artifactID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		deletes := []struct {
			query string
			desc  string
		}{
			{"DELETE FROM list_items WHERE source_artifact_ids @> ARRAY[$1]::text[]", "list_items"},
			{"DELETE FROM lists WHERE source_artifact_ids @> ARRAY[$1]::text[]", "lists"},
			{"DELETE FROM annotations WHERE artifact_id = $1", "annotations"},
			{"DELETE FROM edges WHERE src_id = $1 OR dst_id = $1", "edges"},
			{"DELETE FROM artifacts WHERE id = $1", "artifacts"},
		}
		for _, d := range deletes {
			if _, err := pool.Exec(ctx, d.query, artifactID); err != nil {
				t.Logf("cleanup %s for %s failed: %v", d.desc, artifactID, err)
			}
		}
	})
}

// cleanupList registers cleanup to delete a test list and its items.
func cleanupList(t *testing.T, pool *pgxpool.Pool, listID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM list_items WHERE list_id = $1", listID); err != nil {
			t.Logf("cleanup list_items for list %s failed: %v", listID, err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM lists WHERE id = $1", listID); err != nil {
			t.Logf("cleanup list %s failed: %v", listID, err)
		}
	})
}

// cleanupAnnotation registers cleanup to delete a test annotation.
func cleanupAnnotation(t *testing.T, pool *pgxpool.Pool, annotationID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM annotations WHERE id = $1", annotationID); err != nil {
			t.Logf("cleanup annotation %s failed: %v", annotationID, err)
		}
	})
}
