//go:build e2e

package drive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	"github.com/smackerel/smackerel/internal/drive/monitor"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestFolderMoveUpdatesArtifactContextWithoutDuplicateExtractionActivity(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	file := fixtures.File{
		ID:         "scope3-e2e-folder-move",
		Name:       "Pantry list.txt",
		MimeType:   "text/plain",
		SizeBytes:  96,
		FolderPath: []string{"Receipts"},
		RevisionID: "scope3-e2e-folder-rev-1",
		Owner:      "fixture-owner@example.com",
		URL:        "https://drive.example/scope3-e2e-folder-move",
		Content:    []byte("Grocery receipt and pantry list: chickpeas, rice, basil."),
	}
	fixtureServer.AddFile(file)
	provider := newE2EGoogleProvider(fixtureServer, pool)
	connectionID := createE2EConnection(t, pool, fixtureServer, provider, []string{"root"})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := smscan.NewPostgresStore(pool)
	if _, err := smscan.NewService(provider, store).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	processor := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker())
	if _, err := processor.ProcessPending(ctx, connectionID); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}
	beforeFileRequests := fixtureServer.RequestCount("/drive/v3/files")
	file.FolderPath = []string{"Meal Plans"}
	fixtureServer.AddFile(file)
	fixtureServer.AddChange(fixtures.Change{Kind: "move", FileID: file.ID})
	if _, err := monitor.NewService(provider, store, monitor.WithMoveRefresher(processor)).RunOnce(ctx, connectionID); err != nil {
		t.Fatalf("monitor move: %v", err)
	}
	if got := fixtureServer.RequestCount("/drive/v3/files"); got != beforeFileRequests {
		t.Fatalf("move refresh re-fetched bytes: before=%d after=%d", beforeFileRequests, got)
	}

	var metadataBytes []byte
	if err := pool.QueryRow(ctx, `
		SELECT a.metadata
		FROM artifacts a JOIN drive_files f ON f.artifact_id=a.id
		WHERE f.connection_id=$1 AND f.provider_file_id=$2`, connectionID, file.ID).Scan(&metadataBytes); err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	classification := metadata["drive"].(map[string]any)["classification"].(map[string]any)
	if classification["folder_path"] != "Meal Plans" {
		t.Fatalf("classification folder_path = %v, want Meal Plans", classification["folder_path"])
	}
	view := getDriveConnectionView(t, liveConfig, connectionID)
	if int(view["indexed_count"].(float64)) != 1 {
		t.Fatalf("connection view indexed_count = %v, want 1", view["indexed_count"])
	}
	if !strings.Contains(getText(t, liveConfig.CoreURL+"/pwa/connector-detail.js"), "skipped_review") {
		t.Fatalf("connector detail JS is missing skipped review rendering")
	}
}
