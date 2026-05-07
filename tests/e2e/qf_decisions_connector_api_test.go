//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	qfConnectorID        = "qf-decisions"
	qfDecisionEventsPath = "/api/private/smackerel/v1/decision-events"
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
	service, ok := health.Services["connector:"+qfConnectorID]
	if !ok {
		t.Fatalf("connector:qf-decisions missing from health services: %s", body)
	}
	if service.Status != "error" {
		t.Fatalf("connector:qf-decisions status = %q, want error before QF stub is available", service.Status)
	}
}

func TestQFDecisionsConnectorSchemaMismatchDoesNotPublishTrustedArtifacts(t *testing.T) {
	cfg := loadE2EConfig(t)
	shutdownQFStub := startQFSchemaMismatchStub(t)
	defer shutdownQFStub()

	waitForHealth(t, cfg, 2*time.Minute)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("e2e: DATABASE_URL is required for live-stack artifact assertion")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `DELETE FROM artifacts WHERE source_id = $1`, qfConnectorID); err != nil {
		t.Fatalf("clean qf artifacts before schema mismatch test: %v", err)
	}

	resp, err := apiPostNoBody(cfg, "/settings/connectors/"+qfConnectorID+"/sync")
	if err != nil {
		t.Fatalf("POST /settings/connectors/%s/sync: %v", qfConnectorID, err)
	}
	if resp.StatusCode != http.StatusSeeOther {
		body, readErr := readBody(resp)
		if readErr != nil {
			t.Fatalf("sync status = %d; body read failed: %v", resp.StatusCode, readErr)
		}
		t.Fatalf("sync status = %d, want %d; body = %s", resp.StatusCode, http.StatusSeeOther, body)
	}
	resp.Body.Close()

	lastError := waitForQFConnectorError(t, pool, "packet_version 99 is unsupported")
	if !strings.Contains(lastError, "packet_version 99 is unsupported") {
		t.Fatalf("last qf connector error = %q", lastError)
	}

	serviceStatus := qfConnectorStatus(t, cfg)
	if serviceStatus != "degraded" {
		t.Fatalf("connector:qf-decisions status = %q, want degraded after live supervisor schema mismatch", serviceStatus)
	}

	artifacts := 0
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM artifacts WHERE source_id = $1`, qfConnectorID).Scan(&artifacts); err != nil {
		t.Fatalf("count qf artifacts after schema mismatch: %v", err)
	}
	if artifacts != 0 {
		t.Fatalf("schema mismatch must not publish qf artifacts; found %d", artifacts)
	}
}

func startQFSchemaMismatchStub(t *testing.T) func() {
	t.Helper()

	baseURL := os.Getenv("QF_DECISIONS_BASE_URL")
	if baseURL == "" {
		t.Fatal("e2e: QF_DECISIONS_BASE_URL is required for live QF schema-mismatch stub")
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse QF_DECISIONS_BASE_URL: %v", err)
	}
	port := parsedURL.Port()
	if port == "" {
		t.Fatalf("QF_DECISIONS_BASE_URL must include a port: %s", baseURL)
	}

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		t.Fatalf("start live QF schema-mismatch stub on configured port %s: %v", port, err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != qfDecisionEventsPath {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"code": "unauthorized", "message": "authorization is required"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"code": "invalid_query_parameter", "message": "packet_version 99 is unsupported"})
	})}
	serverErrors := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
		close(serverErrors)
	}()

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("shutdown QF schema-mismatch stub: %v", err)
		}
		if err := <-serverErrors; err != nil {
			t.Fatalf("QF schema-mismatch stub failed: %v", err)
		}
	}
}

func apiPostNoBody(cfg e2eConfig, path string) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodPost, cfg.CoreURL+path, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return client.Do(request)
}

func waitForQFConnectorError(t *testing.T, pool *pgxpool.Pool, want string) string {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	lastObserved := ""
	for time.Now().Before(deadline) {
		queryCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		var lastError string
		err := pool.QueryRow(queryCtx, `SELECT COALESCE(last_error, '') FROM sync_state WHERE source_id = $1`, qfConnectorID).Scan(&lastError)
		cancel()
		if err == nil {
			lastObserved = lastError
		} else {
			lastObserved = err.Error()
		}
		if err == nil && strings.Contains(lastError, want) {
			return lastError
		}
		time.Sleep(500 * time.Millisecond)
	}
	stateRows := describeSyncStateRows(t, pool)
	t.Fatalf("qf connector did not record expected error containing %q; last observed sync_state result: %q; sync_state rows: %s", want, lastObserved, stateRows)
	return ""
}

func describeSyncStateRows(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `SELECT source_id, COALESCE(last_error, '') FROM sync_state ORDER BY source_id`)
	if err != nil {
		return "query failed: " + err.Error()
	}
	defer rows.Close()

	var summaries []string
	for rows.Next() {
		var sourceID string
		var lastError string
		if err := rows.Scan(&sourceID, &lastError); err != nil {
			return "scan failed: " + err.Error()
		}
		summaries = append(summaries, sourceID+"="+lastError)
	}
	if err := rows.Err(); err != nil {
		return "rows failed: " + err.Error()
	}
	if len(summaries) == 0 {
		return "<none>"
	}
	return strings.Join(summaries, "; ")
}

func qfConnectorStatus(t *testing.T, cfg e2eConfig) string {
	t.Helper()

	resp, err := apiGet(cfg, "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health after qf sync: %v", err)
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
	service, ok := health.Services["connector:"+qfConnectorID]
	if !ok {
		t.Fatalf("connector:qf-decisions missing from health services: %s", body)
	}
	return service.Status
}
