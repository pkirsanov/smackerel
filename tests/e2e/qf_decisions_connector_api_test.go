//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
)

func TestQFDecisionsConnectorHealthAppearsInLiveAPI(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	resp, err := apiGet(cfg, "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, readErr := readBody(resp)
		if readErr != nil {
			t.Fatalf("GET /api/health status = %d; body read failed: %v", resp.StatusCode, readErr)
		}
		t.Fatalf("GET /api/health status = %d; body = %s", resp.StatusCode, body)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read health response: %v", err)
	}

	var health struct {
		Services map[string]struct {
			Status string `json:"status"`
		} `json:"services"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("decode health response: %v; body = %s", err, body)
	}
	service, ok := health.Services["connector:qf-decisions"]
	if !ok {
		t.Fatalf("connector:qf-decisions missing from health services: %s", body)
	}
	if service.Status != "disconnected" {
		t.Fatalf("connector:qf-decisions status = %q, want disconnected", service.Status)
	}
}

func TestQFDecisionsConnectorSchemaMismatchDoesNotPublishTrustedArtifacts(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set - live stack DB not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `DELETE FROM artifacts WHERE source_id = $1`, qfdecisions.DefaultConnectorID); err != nil {
		t.Fatalf("clean qf artifacts before schema mismatch test: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(qfdecisions.BridgeErrorResponse{Code: "invalid_query_parameter", Message: "packet_version 99 is unsupported"})
	}))
	defer server.Close()

	qfConnector := qfdecisions.New(qfdecisions.DefaultConnectorID)
	err = qfConnector.Connect(ctx, connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-service-token"},
		Enabled:      true,
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       server.URL,
			"packet_version": 99,
			"page_size":      25,
		},
	})
	if err == nil {
		t.Fatal("expected schema mismatch")
	}
	var schemaErr qfdecisions.SchemaCompatibilityError
	if !errors.As(err, &schemaErr) {
		t.Fatalf("error = %v, want SchemaCompatibilityError", err)
	}
	if got := qfConnector.Health(ctx); got != connector.HealthDegraded {
		t.Fatalf("health = %s, want %s", got, connector.HealthDegraded)
	}

	artifacts := 0
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM artifacts WHERE source_id = $1`, qfdecisions.DefaultConnectorID).Scan(&artifacts); err != nil {
		t.Fatalf("count qf artifacts after schema mismatch: %v", err)
	}
	if artifacts != 0 {
		t.Fatalf("schema mismatch must not publish qf artifacts; found %d", artifacts)
	}
}