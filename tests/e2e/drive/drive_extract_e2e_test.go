//go:build e2e

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveExtractE2E_MultiFormatFilesBecomeSearchable(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFile(fixtures.File{
		ID:         "scope3-searchable-recipe",
		Name:       "Searchable dinner.txt",
		MimeType:   "text/plain",
		SizeBytes:  128,
		FolderPath: []string{"Meal Plans"},
		RevisionID: "scope3-searchable-rev-1",
		Owner:      "fixture-owner@example.com",
		URL:        "https://drive.example/scope3-searchable-recipe",
		Content:    []byte("Dinner recipe: chickpeas, preserved lemon, parsley, tahini."),
	})
	provider := newE2EGoogleProvider(fixtureServer, pool)
	connectionID := createE2EConnection(t, pool, fixtureServer, provider, []string{"root"})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := smscan.NewService(provider, smscan.NewPostgresStore(pool)).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).ProcessPending(ctx, connectionID); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}

	searchBody := []byte(`{"query":"preserved lemon chickpeas","limit":5}`)
	req, err := http.NewRequest(http.MethodPost, liveConfig.CoreURL+"/api/search", bytes.NewReader(searchBody))
	if err != nil {
		t.Fatalf("build search request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if liveConfig.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+liveConfig.AuthToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/search: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read search response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/search status=%d body=%s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "Searchable dinner") {
		t.Fatalf("search response did not include drive artifact: %s", string(body))
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if decoded["total_candidates"].(float64) < 1 {
		t.Fatalf("search total_candidates = %v, want at least 1", decoded["total_candidates"])
	}
}