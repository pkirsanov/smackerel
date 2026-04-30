//go:build integration

package drive

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	"github.com/smackerel/smackerel/internal/drive/monitor"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestFolderMoveRefreshesTaxonomyWithoutReextractingContent(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	file := fixtures.File{
		ID:         "scope3-folder-move",
		Name:       "Weekend groceries.txt",
		MimeType:   "text/plain",
		SizeBytes:  128,
		FolderPath: []string{"Receipts", "Household"},
		RevisionID: "same-revision-after-move",
		Owner:      "fixture-owner@example.com",
		URL:        "https://drive.example/scope3-folder-move",
		Content:    []byte("Grocery receipt total 31.45 for tomatoes, basil, pasta."),
	}
	fixtureServer.AddFile(file)
	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := smscan.NewPostgresStore(pool)
	if _, err := smscan.NewService(provider, store).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	processor := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker())
	if _, err := processor.ProcessPending(ctx, connectionID); err != nil {
		t.Fatalf("initial ProcessPending: %v", err)
	}
	beforeFileRequests := fixtureServer.RequestCount("/drive/v3/files")

	file.FolderPath = []string{"Meal Plans", "April"}
	fixtureServer.AddFile(file)
	fixtureServer.AddChange(fixtures.Change{Kind: "move", FileID: file.ID})
	if _, err := monitor.NewService(provider, store, monitor.WithMoveRefresher(processor)).RunOnce(ctx, connectionID); err != nil {
		t.Fatalf("monitor move: %v", err)
	}
	afterFileRequests := fixtureServer.RequestCount("/drive/v3/files")
	if afterFileRequests != beforeFileRequests {
		t.Fatalf("folder move re-fetched file bytes: before=%d after=%d", beforeFileRequests, afterFileRequests)
	}

	var metadataBytes []byte
	var folderPath string
	if err := pool.QueryRow(ctx, `
		SELECT a.metadata, array_to_string(f.folder_path, '/')
		FROM artifacts a
		JOIN drive_files f ON f.artifact_id=a.id
		WHERE f.connection_id=$1 AND f.provider_file_id=$2`, connectionID, file.ID).Scan(&metadataBytes, &folderPath); err != nil {
		t.Fatalf("read moved file metadata: %v", err)
	}
	if folderPath != "Meal Plans/April" {
		t.Fatalf("folder_path = %q, want Meal Plans/April", folderPath)
	}
	var metadata map[string]any
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	classification := metadata["drive"].(map[string]any)["classification"].(map[string]any)
	if classification["folder_path"] != "Meal Plans/April" || classification["classification"] != "recipe" {
		t.Fatalf("refreshed classification = %+v, want recipe with moved folder path", classification)
	}
}
