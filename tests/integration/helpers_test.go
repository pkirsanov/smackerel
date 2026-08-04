//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// devStackMarkers are the SST-derived identifiers that appear ONLY in the
// persistent dev/prod stack's connection URLs. config/smackerel.yaml pins dev
// to host ports 40001-40002 (core/ml) and 42001-42006 (infra), while the
// disposable test stack uses 45001-45002 and 47001-47006. These prefixes
// therefore separate the two WITHOUT matching the container-internal ports
// (:5432, :4222, :8080) that every stack shares — the in-network URLs the
// `test integration` lane injects must keep passing.
var devStackMarkers = []string{
	"smackerel-dev",
	"smackerel-prod",
	":4000", // dev core/ml host-port prefix (core_host_port 40001, ml_host_port 40002)
	":4200", // dev infra host-port prefix (postgres 42001, nats 42002-42003)
}

// disposableStackViolation reports a live-stack connection URL that points at
// the persistent dev/prod stack instead of the disposable test stack. Env
// access is injected so the adversarial cases can exercise it without mutating
// process state.
//
// The returned error names the offending variable and the matched marker but
// NEVER the value: DATABASE_URL and NATS_URL embed credentials, and test
// output is not a safe place for them.
func disposableStackViolation(lookup func(string) string) error {
	for _, key := range []string{"DATABASE_URL", "NATS_URL"} {
		value := lookup(key)
		if value == "" {
			continue
		}
		for _, marker := range devStackMarkers {
			if strings.Contains(value, marker) {
				return fmt.Errorf("%s contains persistent dev/prod stack marker %q — refuse to run; live-stack integration tests require the disposable test stack (Compose project smackerel-test, host ports 45001-45002/47001-47006)", key, marker)
			}
		}
	}
	return nil
}

// requireDisposableStack fails loudly when the live-stack env points at the
// persistent dev stack. This package contains committed destructive DDL —
// TestMigrations_TableDropAndRecreate drops lists/list_items with CASCADE and
// COMMITS the drop — so the assertion must run BEFORE any connection is opened.
func requireDisposableStack(t *testing.T) {
	t.Helper()
	if err := disposableStackViolation(os.Getenv); err != nil {
		t.Fatalf("integration: %v", err)
	}
}

// testPool returns a pgxpool connected to the test database.
// The pool is closed automatically when the test completes.
// Skips the test if DATABASE_URL is not set.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	requireDisposableStack(t)

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
	requireDisposableStack(t)

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
