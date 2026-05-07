//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// TestPhotosHealth_C004_LiveAPIScrubsErrNoRowsForMissingCluster proves
// the chaos C-004 closure end-to-end: GET and POST against a random
// (non-existent) duplicate-group ID MUST return a clean 404 with
// `cluster_not_found` and MUST NOT leak the lib/pq sentinel "no rows
// in result set". Unit tests in internal/api/photos_chaos_closure_test.go
// cover the helper in isolation; this test exercises the full handler
// path with a real *photolib.Store backed by the disposable test DB
// so a future regression that bypasses the helper still trips this
// adversarial regression.
func TestPhotosHealth_C004_LiveAPIScrubsErrNoRowsForMissingCluster(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	cfg := config.PhotosConfig{Enabled: true}
	cfg.Policy.ActionsMaxScopeSize = 50
	cfg.IOLimits.PhotoBinaryMaxBytes = 104857600
	handlers := api.NewPhotosHandlers(store, cfg)

	router := chi.NewRouter()
	router.Get("/v1/photos/health/duplicates/{id}", handlers.HealthDuplicatesGet)
	router.Post("/v1/photos/health/duplicates/{id}/best-pick", handlers.SetClusterBestPick)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	missingID := uuid.NewString()

	// --- GET path ---
	response, err := http.Get(server.URL + "/v1/photos/health/duplicates/" + missingID)
	if err != nil {
		t.Fatalf("GET duplicates/%s: %v", missingID, err)
	}
	body := readResponseBody(t, response)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET status = %d, want 404 (body=%s)", response.StatusCode, body)
	}
	if strings.Contains(body, "no rows in result set") {
		t.Fatalf("GET response leaked lib/pq sentinel; body=%s", body)
	}
	if !strings.Contains(body, "cluster_not_found") {
		t.Fatalf("GET response missing cluster_not_found code; body=%s", body)
	}
	if !strings.Contains(body, "duplicate group not found") {
		t.Fatalf("GET response missing clean message; body=%s", body)
	}

	// --- POST best-pick path ---
	bestPickBody, err := json.Marshal(map[string]string{
		"photo_id":  uuid.NewString(),
		"picked_by": "user",
	})
	if err != nil {
		t.Fatalf("marshal best-pick body: %v", err)
	}
	postResp, err := http.Post(
		server.URL+"/v1/photos/health/duplicates/"+missingID+"/best-pick",
		"application/json",
		bytes.NewReader(bestPickBody),
	)
	if err != nil {
		t.Fatalf("POST best-pick %s: %v", missingID, err)
	}
	postBody := readResponseBody(t, postResp)
	if postResp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST status = %d, want 404 (body=%s)", postResp.StatusCode, postBody)
	}
	if strings.Contains(postBody, "no rows in result set") {
		t.Fatalf("POST response leaked lib/pq sentinel; body=%s", postBody)
	}
	if !strings.Contains(postBody, "cluster_not_found") {
		t.Fatalf("POST response missing cluster_not_found code; body=%s", postBody)
	}
	if !strings.Contains(postBody, "duplicate group not found") {
		t.Fatalf("POST response missing clean message; body=%s", postBody)
	}
}

func readResponseBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(response.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return buf.String()
}
